package server

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	// sseRingBufferSize is the number of recent events kept in memory for
	// Last-Event-ID reconnection support.
	sseRingBufferSize = 1000

	// sseKeepaliveInterval is how often keepalive comments are sent to
	// prevent connection timeouts.
	sseKeepaliveInterval = 15 * time.Second
)

// sseEvent is a single event stored in the ring buffer and sent to SSE clients.
type sseEvent struct {
	ID    uint64 // monotonically increasing sequence number
	Topic string
	Data  []byte // JSON-encoded payload
}

// sseHub fans out events from recordAndPublish to connected SSE clients.
// It maintains an in-memory ring buffer for Last-Event-ID reconnection.
type sseHub struct {
	mu      sync.RWMutex
	clients map[*sseClient]struct{}
	nextID  atomic.Uint64

	// Ring buffer for replay on reconnection.
	ringMu  sync.RWMutex
	ring    [sseRingBufferSize]sseEvent
	ringPos int  // next write position (wraps around)
	ringLen int  // number of valid entries (up to sseRingBufferSize)
}

// sseClient represents a single connected SSE consumer.
type sseClient struct {
	topics []string       // topic glob patterns to match (empty = all)
	ch     chan *sseEvent // buffered channel for event delivery
}

func newSSEHub() *sseHub {
	return &sseHub{
		clients: make(map[*sseClient]struct{}),
	}
}

// broadcast sends an event to all connected clients whose topic filters match.
func (h *sseHub) broadcast(topic string, payload []byte) {
	id := h.nextID.Add(1)
	evt := &sseEvent{
		ID:    id,
		Topic: topic,
		Data:  payload,
	}

	// Store in ring buffer.
	h.ringMu.Lock()
	h.ring[h.ringPos] = *evt
	h.ringPos = (h.ringPos + 1) % sseRingBufferSize
	if h.ringLen < sseRingBufferSize {
		h.ringLen++
	}
	h.ringMu.Unlock()

	// Fan out to connected clients.
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		if c.matchesTopic(topic) {
			select {
			case c.ch <- evt:
			default:
				// Drop if client is slow — prevents blocking the publisher.
			}
		}
	}
}

// subscribe registers a new SSE client and returns it. Call unsubscribe when done.
func (h *sseHub) subscribe(topics []string) *sseClient {
	c := &sseClient{
		topics: topics,
		ch:     make(chan *sseEvent, 64),
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	return c
}

// unsubscribe removes a client from the hub.
func (h *sseHub) unsubscribe(c *sseClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

// eventsSince returns buffered events with ID > lastID, in order.
// Returns nil if lastID is too old (no longer in buffer).
func (h *sseHub) eventsSince(lastID uint64) []*sseEvent {
	h.ringMu.RLock()
	defer h.ringMu.RUnlock()

	if h.ringLen == 0 {
		return nil
	}

	var result []*sseEvent

	// Walk the ring buffer from oldest to newest.
	start := h.ringPos - h.ringLen
	if start < 0 {
		start += sseRingBufferSize
	}
	for i := range h.ringLen {
		idx := (start + i) % sseRingBufferSize
		evt := &h.ring[idx]
		if evt.ID > lastID {
			result = append(result, evt)
		}
	}

	return result
}

// matchesTopic checks whether the client's topic filters match the given topic.
// An empty filter list matches all topics.
// Supports simple glob patterns: "beads.bead.*" matches "beads.bead.created".
func (c *sseClient) matchesTopic(topic string) bool {
	if len(c.topics) == 0 {
		return true
	}
	for _, pattern := range c.topics {
		if matchTopicPattern(pattern, topic) {
			return true
		}
	}
	return false
}

// matchTopicPattern matches a dot-separated topic against a pattern.
// Supports "*" as a single-segment wildcard and ">" as a multi-segment
// suffix wildcard (NATS-style).
func matchTopicPattern(pattern, topic string) bool {
	if pattern == topic {
		return true
	}

	patParts := strings.Split(pattern, ".")
	topParts := strings.Split(topic, ".")

	for i, pp := range patParts {
		if pp == ">" {
			// ">" matches one or more remaining segments.
			return i < len(topParts)
		}
		if i >= len(topParts) {
			return false
		}
		if pp != "*" && pp != topParts[i] {
			return false
		}
	}

	return len(patParts) == len(topParts)
}

// handleEventStream handles GET /v1/events/stream (SSE endpoint).
func (s *BeadsServer) handleEventStream(w http.ResponseWriter, r *http.Request) {
	// Ensure response supports flushing (required for SSE).
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	// Parse optional topic filters from query params.
	var topics []string
	if q := r.URL.Query().Get("topics"); q != "" {
		for _, t := range strings.Split(q, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				topics = append(topics, t)
			}
		}
	}

	// Subscribe to the hub.
	client := s.sseHub.subscribe(topics)
	defer s.sseHub.unsubscribe(client)

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering.
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// If the client sent Last-Event-ID, replay buffered events.
	if lastIDStr := r.Header.Get("Last-Event-ID"); lastIDStr != "" {
		if lastID, err := strconv.ParseUint(lastIDStr, 10, 64); err == nil {
			replayed := s.sseHub.eventsSince(lastID)
			for _, evt := range replayed {
				if client.matchesTopic(evt.Topic) {
					writeSSEEvent(w, evt)
				}
			}
			flusher.Flush()
		}
	}

	// Stream events until client disconnects.
	ctx := r.Context()
	keepalive := time.NewTicker(sseKeepaliveInterval)
	defer keepalive.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-client.ch:
			writeSSEEvent(w, evt)
			flusher.Flush()
		case <-keepalive.C:
			// Send a comment line as keepalive.
			fmt.Fprintf(w, ":keepalive\n\n")
			flusher.Flush()
		}
	}
}

// writeSSEEvent writes a single SSE event to the writer.
func writeSSEEvent(w http.ResponseWriter, evt *sseEvent) {
	fmt.Fprintf(w, "id:%d\n", evt.ID)
	fmt.Fprintf(w, "event:%s\n", evt.Topic)
	fmt.Fprintf(w, "data:%s\n\n", evt.Data)
}

// broadcastEvent is called by recordAndPublish to fan out events to SSE clients.
func (s *BeadsServer) broadcastEvent(topic string, event any) {
	if s.sseHub == nil {
		return
	}
	payload, err := json.Marshal(event)
	if err != nil {
		slog.Warn("failed to marshal event for SSE broadcast", "topic", topic, "error", err)
		return
	}
	s.sseHub.broadcast(topic, payload)
}
