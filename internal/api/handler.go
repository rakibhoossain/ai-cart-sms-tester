package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/rakib/ai-cart-sms-tester/internal/db"
	"github.com/rakib/ai-cart-sms-tester/internal/model"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

var clients = make(map[*websocket.Conn]bool)
var clientsMu sync.Mutex
var broadcast = make(chan model.SMS, 1000) // Buffered channel

func StartBackgroundWorkers() {
	go handleMessages()
}

func SetupRoutes(r chi.Router) {
	r.Get("/messages", GetMessages)
	r.Post("/send", SendSMS)
	r.Delete("/messages", DeleteMessages)
	r.Put("/messages/{id}/read", MarkAsRead)
	r.Get("/ws", HandleWebSocket)

	StartBackgroundWorkers()
}

func GetMessages(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")
	search := r.URL.Query().Get("search")

	page := 1
	if pageStr != "" {
		fmt.Sscanf(pageStr, "%d", &page)
	}
	limit := 50
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}

	messages, err := db.GetMessages(page, limit, search)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	total, unread, err := db.GetStats(search)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	resp := map[string]interface{}{
		"data": messages,
		"meta": map[string]interface{}{
			"total":  total,
			"unread": unread,
			"page":   page,
			"limit":  limit,
		},
	}
	json.NewEncoder(w).Encode(resp)
}

func MarkAsRead(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	var id int64
	fmt.Sscanf(idStr, "%d", &id)

	if err := db.MarkAsRead(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func SendSMS(w http.ResponseWriter, r *http.Request) {
	var req struct {
		From string `json:"from"`
		To   string `json:"to"`
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sms := model.SMS{
		From:      req.From,
		To:        req.To,
		Body:      req.Body,
		IsRead:    false,
		CreatedAt: time.Now(),
	}

	id, err := db.SaveSMS(&sms)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sms.ID = id

	// Non-blocking send to broadcast channel
	select {
	case broadcast <- sms:
	default:
		// Channel full, drop message for real-time clients but it's saved in DB
		// This prevents the API from hanging
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(sms)
}

func DeleteMessages(w http.ResponseWriter, r *http.Request) {
	if err := db.DeleteAllMessages(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	clientsMu.Lock()
	clients[ws] = true
	clientsMu.Unlock()

	// Ensure connection references are cleaned up
	defer func() {
		clientsMu.Lock()
		delete(clients, ws)
		clientsMu.Unlock()
		ws.Close()
	}()

	// Ping/Pong loop to keep connection alive and detect dead clients
	ws.SetReadLimit(512)
	ws.SetReadDeadline(time.Now().Add(60 * time.Second))
	ws.SetPongHandler(func(string) error { ws.SetReadDeadline(time.Now().Add(60 * time.Second)); return nil })

	for {
		_, _, err := ws.ReadMessage()
		if err != nil {
			break
		}
	}
}

func handleMessages() {
	ticker := time.NewTicker(50 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg := <-broadcast:
			clientsMu.Lock()
			for client := range clients {
				client.SetWriteDeadline(time.Now().Add(10 * time.Second))
				err := client.WriteJSON(msg)
				if err != nil {
					client.Close()
					delete(clients, client)
				}
			}
			clientsMu.Unlock()
		case <-ticker.C:
			// Send Pings
			clientsMu.Lock()
			for client := range clients {
				client.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := client.WriteMessage(websocket.PingMessage, nil); err != nil {
					client.Close()
					delete(clients, client)
				}
			}
			clientsMu.Unlock()
		}
	}
}
