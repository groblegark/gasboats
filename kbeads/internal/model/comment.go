package model

import "time"

// Comment represents a comment on a bead.
type Comment struct {
	ID        int64     `json:"id"`
	BeadID    string    `json:"bead_id"`
	Author    string    `json:"author"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"created_at"`
}
