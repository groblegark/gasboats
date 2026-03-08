package model

import (
	"encoding/json"
	"time"
)

// Event is a persisted event record, mirroring what is published to NATS.
type Event struct {
	ID        int64           `json:"id"`
	Topic     string          `json:"topic"`
	BeadID    string          `json:"bead_id"`
	Actor     string          `json:"actor,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt time.Time       `json:"created_at"`
}
