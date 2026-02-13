package model

import (
	"time"
)

type SMS struct {
	ID         int64     `json:"id"`
	From       string    `json:"from"`
	To         string    `json:"to"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
	CheckCount int       `json:"check_count"` // To simulate read status if needed
	IsRead     bool      `json:"is_read"`
}
