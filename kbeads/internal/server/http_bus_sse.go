package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/groblegark/kbeads/internal/eventbus"
	"github.com/nats-io/nats.go"
)

// busSSEEvent is the JSON payload sent in each SSE data field.
type busSSEEvent struct {
	Stream  string          `json:"stream"`  // Short stream name (e.g., "hooks")
	Type    string          `json:"type"`    // Event type extracted from subject
	Subject string          `json:"subject"` // Full NATS subject
	Seq     uint64          `json:"seq"`     // JetStream sequence number
	TS      string          `json:"ts"`      // ISO 8601 timestamp
	Payload json.RawMessage `json:"payload"` // Raw event JSON
}

// handleBusEvents handles GET /v1/bus/events (JetStream SSE endpoint).
//
// Query parameters:
//   - stream: comma-separated stream names (hooks, decisions, agents, mail,
//     mutations, config, gate, inbox, jack) or "all" (default: all)
//   - filter: event type filter (e.g., "Stop", "MutationCreate")
func (s *BeadsServer) handleBusEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	if s.bus == nil || !s.bus.JetStreamEnabled() {
		writeError(w, http.StatusServiceUnavailable, "JetStream not configured (set BEADS_NATS_URL)")
		return
	}

	js := s.bus.JetStream()

	// Parse stream selection.
	streams := eventbus.StreamNames // default: all
	if q := r.URL.Query().Get("stream"); q != "" && q != "all" {
		streams = nil
		for _, name := range strings.Split(q, ",") {
			name = strings.TrimSpace(name)
			if name != "" && eventbus.SubjectPrefixForStream(name) != "" {
				streams = append(streams, name)
			}
		}
		if len(streams) == 0 {
			writeError(w, http.StatusBadRequest, "no valid stream names provided")
			return
		}
	}

	filter := r.URL.Query().Get("filter")

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Create ephemeral push subscriptions for each requested stream.
	events := make(chan *nats.Msg, 128)
	var subs []*nats.Subscription
	for _, name := range streams {
		prefix := eventbus.SubjectPrefixForStream(name)
		sub, err := js.Subscribe(prefix+">", func(msg *nats.Msg) {
			select {
			case events <- msg:
			default:
				// Drop if channel is full.
			}
		}, nats.DeliverNew())
		if err != nil {
			slog.Warn("bus SSE: failed to subscribe to stream", "stream", name, "err", err)
			continue
		}
		subs = append(subs, sub)
	}

	if len(subs) == 0 {
		writeError(w, http.StatusServiceUnavailable, "failed to subscribe to any streams")
		return
	}

	// Clean up subscriptions on disconnect.
	defer func() {
		for _, sub := range subs {
			_ = sub.Unsubscribe()
		}
	}()

	ctx := r.Context()
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()

	var seq atomic.Uint64

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-events:
			streamName := eventbus.StreamForSubject(msg.Subject)
			eventType := eventbus.EventTypeFromSubject(msg.Subject)

			// Apply event type filter.
			if filter != "" && eventType != filter {
				continue
			}

			meta, _ := msg.Metadata()
			var jsSeq uint64
			var ts string
			if meta != nil {
				jsSeq = meta.Sequence.Stream
				ts = meta.Timestamp.UTC().Format(time.RFC3339)
			} else {
				ts = time.Now().UTC().Format(time.RFC3339)
			}

			evt := busSSEEvent{
				Stream:  streamName,
				Type:    eventType,
				Subject: msg.Subject,
				Seq:     jsSeq,
				TS:      ts,
				Payload: msg.Data,
			}

			data, err := json.Marshal(evt)
			if err != nil {
				continue
			}

			id := seq.Add(1)
			fmt.Fprintf(w, "id:%d\n", id)
			fmt.Fprintf(w, "event:%s\n", streamName)
			fmt.Fprintf(w, "data:%s\n\n", data)
			flusher.Flush()

		case <-keepalive.C:
			fmt.Fprintf(w, ":keepalive\n\n")
			flusher.Flush()
		}
	}
}
