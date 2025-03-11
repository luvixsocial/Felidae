package types

import "time"

// 📌 Post struct
type Post struct {
	ID        int      `json:"id"`
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
}