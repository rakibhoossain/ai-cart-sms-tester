package main

import (
	"crypto/subtle"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"
	"github.com/rakib/ai-cart-sms-tester/internal/api"
	"github.com/rakib/ai-cart-sms-tester/internal/db"
)

//go:embed all:frontend/out
var frontendFiles embed.FS

func main() {
	// Load .env file if it exists
	_ = godotenv.Load()

	// Initialize Database
	storageDir := "storage"
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Fatalf("Failed to create storage directory: %v", err)
	}
	dbPath := storageDir + "/sms.db"
	if err := db.Init(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Start Background Workers (WebSocket Broadcast)
	api.StartBackgroundWorkers()

	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Auth Middleware
	authUser := os.Getenv("BASIC_AUTH_USER")
	authPass := os.Getenv("BASIC_AUTH_PASSWORD")
	apiToken := os.Getenv("API_TOKEN")

	basicAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if authUser == "" || authPass == "" {
				next.ServeHTTP(w, r)
				return
			}
			user, pass, ok := r.BasicAuth()
			if !ok || subtle.ConstantTimeCompare([]byte(user), []byte(authUser)) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte(authPass)) != 1 {
				w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}

	apiTokenOrBasicAuth := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check API Token
			if apiToken != "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					token := strings.TrimPrefix(authHeader, "Bearer ")
					if subtle.ConstantTimeCompare([]byte(token), []byte(apiToken)) == 1 {
						next.ServeHTTP(w, r)
						return
					}
				}
			}

			// Fallback to Basic Auth
			if authUser == "" || authPass == "" {
				if apiToken == "" {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Check Basic Auth
			user, pass, ok := r.BasicAuth()
			if ok && subtle.ConstantTimeCompare([]byte(user), []byte(authUser)) == 1 && subtle.ConstantTimeCompare([]byte(pass), []byte(authPass)) == 1 {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		})
	}

	// API Routes
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(apiTokenOrBasicAuth)
			r.Post("/send", api.SendSMS)
		})

		r.Group(func(r chi.Router) {
			r.Use(basicAuth)
			r.Get("/messages", api.GetMessages)
			r.Delete("/messages", api.DeleteMessages)
			r.Put("/messages/{id}/read", api.MarkAsRead)
			r.Get("/ws", api.HandleWebSocket)
		})
	})

	// Serve Frontend (SPA) with Basic Auth
	// Using embed.FS, stripping "frontend/out" prefix
	fs, err := fs.Sub(frontendFiles, "frontend/out")
	if err != nil {
		log.Fatalf("Failed to create sub filesystem: %v", err)
	}

	r.Group(func(r chi.Router) {
		r.Use(basicAuth)
		FileServer(r, "/", http.FS(fs))
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8025"
	}
	log.Printf("Starting SMS Tester on port %s", port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func FileServer(r chi.Router, public string, root http.FileSystem) {
	if strings.ContainsAny(public, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if public != "/" && public[len(public)-1] != '/' {
		r.Get(public, http.RedirectHandler(public+"/", 301).ServeHTTP)
		public += "/"
	}

	r.Get(public+"*", func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")

		// Extract the file path from the URL
		filePath := strings.TrimPrefix(r.URL.Path, pathPrefix)
		if filePath == "" {
			filePath = "."
		}

		// Helper to serve the root index.html for SPA fallback
		serveRootIndex := func() {
			indexFile, err := root.Open("index.html")
			if err != nil {
				http.NotFound(w, r)
				return
			}
			defer indexFile.Close()
			stat, _ := indexFile.Stat()
			http.ServeContent(w, r, "index.html", stat.ModTime(), indexFile)
		}

		// 1. Try to open the file exactly as requested
		f, err := root.Open(filePath)
		if os.IsNotExist(err) {
			// 2. Try appending .html (Next.js static export often drops extension)
			f, err = root.Open(filePath + ".html")
		}

		if os.IsNotExist(err) {
			// 3. Fallback to SPA root index
			serveRootIndex()
			return
		} else if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 4. If directory, try finding index.html inside
		if stat.IsDir() {
			// Construct path to index.html safely
			indexPath := strings.TrimRight(filePath, "/") + "/index.html"
			if filePath == "." || filePath == "" {
				indexPath = "index.html"
			}

			indexFunc, err := root.Open(indexPath)
			if err == nil {
				defer indexFunc.Close()
				idxStat, _ := indexFunc.Stat()
				http.ServeContent(w, r, "index.html", idxStat.ModTime(), indexFunc)
				return
			}
			// If no index.html in sub-directory, fall back to SPA root
			serveRootIndex()
			return
		}

		// Serve the actual file
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
	})
}
