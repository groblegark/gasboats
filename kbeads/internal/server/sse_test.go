package server

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/groblegark/kbeads/internal/events"
)

func TestSSEHub_BroadcastAndReceive(t *testing.T) {
	hub := newSSEHub()

	client := hub.subscribe(nil) // all topics
	defer hub.unsubscribe(client)

	hub.broadcast("beads.bead.created", []byte(`{"id":"kd-1"}`))

	select {
	case evt := <-client.ch:
		if evt.Topic != "beads.bead.created" {
			t.Fatalf("expected topic=%q, got %q", "beads.bead.created", evt.Topic)
		}
		if string(evt.Data) != `{"id":"kd-1"}` {
			t.Fatalf("expected data=%q, got %q", `{"id":"kd-1"}`, string(evt.Data))
		}
		if evt.ID != 1 {
			t.Fatalf("expected id=1, got %d", evt.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSSEHub_TopicFiltering(t *testing.T) {
	hub := newSSEHub()

	// Client only wants bead events.
	client := hub.subscribe([]string{"beads.bead.*"})
	defer hub.unsubscribe(client)

	hub.broadcast("beads.label.added", []byte(`{"label":"x"}`))
	hub.broadcast("beads.bead.created", []byte(`{"id":"kd-1"}`))

	select {
	case evt := <-client.ch:
		if evt.Topic != "beads.bead.created" {
			t.Fatalf("expected topic=%q, got %q", "beads.bead.created", evt.Topic)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}

	// Ensure no more events (label.added should have been filtered).
	select {
	case evt := <-client.ch:
		t.Fatalf("unexpected event: topic=%q", evt.Topic)
	case <-time.After(50 * time.Millisecond):
		// Good - no extra events.
	}
}

func TestSSEHub_MultipleTopicFilters(t *testing.T) {
	hub := newSSEHub()

	client := hub.subscribe([]string{"beads.bead.*", "beads.label.*"})
	defer hub.unsubscribe(client)

	hub.broadcast("beads.bead.created", []byte(`{}`))
	hub.broadcast("beads.label.added", []byte(`{}`))
	hub.broadcast("beads.comment.added", []byte(`{}`)) // should be filtered

	received := 0
	timeout := time.After(time.Second)
	for received < 2 {
		select {
		case <-client.ch:
			received++
		case <-timeout:
			t.Fatalf("expected 2 events, got %d", received)
		}
	}

	select {
	case <-client.ch:
		t.Fatal("unexpected third event")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSSEHub_Unsubscribe(t *testing.T) {
	hub := newSSEHub()

	client := hub.subscribe(nil)
	hub.unsubscribe(client)

	hub.broadcast("beads.bead.created", []byte(`{}`))

	select {
	case <-client.ch:
		t.Fatal("should not receive events after unsubscribe")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSSEHub_EventsSince(t *testing.T) {
	hub := newSSEHub()

	// Broadcast 5 events.
	for i := range 5 {
		hub.broadcast("beads.bead.created", []byte(`{"n":`+string(rune('0'+i))+`}`))
	}

	// Get events after ID 2 (should return IDs 3, 4, 5).
	evts := hub.eventsSince(2)
	if len(evts) != 3 {
		t.Fatalf("expected 3 events, got %d", len(evts))
	}
	if evts[0].ID != 3 || evts[1].ID != 4 || evts[2].ID != 5 {
		t.Fatalf("expected IDs [3,4,5], got [%d,%d,%d]", evts[0].ID, evts[1].ID, evts[2].ID)
	}
}

func TestSSEHub_EventsSince_Empty(t *testing.T) {
	hub := newSSEHub()
	evts := hub.eventsSince(0)
	if len(evts) != 0 {
		t.Fatalf("expected 0 events, got %d", len(evts))
	}
}

func TestSSEHub_EventsSince_AllNew(t *testing.T) {
	hub := newSSEHub()
	hub.broadcast("beads.bead.created", []byte(`{}`))
	hub.broadcast("beads.bead.updated", []byte(`{}`))

	evts := hub.eventsSince(0)
	if len(evts) != 2 {
		t.Fatalf("expected 2 events, got %d", len(evts))
	}
}

func TestSSEHub_RingBufferWrap(t *testing.T) {
	hub := newSSEHub()

	// Fill the ring buffer and then some to force wrap.
	for range sseRingBufferSize + 100 {
		hub.broadcast("beads.bead.created", []byte(`{}`))
	}

	// The oldest event in the buffer should have ID = 101 (100 were evicted).
	evts := hub.eventsSince(0)
	if len(evts) != sseRingBufferSize {
		t.Fatalf("expected %d events, got %d", sseRingBufferSize, len(evts))
	}
	if evts[0].ID != 101 {
		t.Fatalf("expected oldest event ID=101, got %d", evts[0].ID)
	}
}

func TestMatchTopicPattern(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		topic   string
		want    bool
	}{
		{"beads.bead.created", "beads.bead.created", true},
		{"beads.bead.created", "beads.bead.updated", false},
		{"beads.bead.*", "beads.bead.created", true},
		{"beads.bead.*", "beads.bead.updated", true},
		{"beads.bead.*", "beads.label.added", false},
		{"beads.>", "beads.bead.created", true},
		{"beads.>", "beads.label.added", true},
		{"beads.>", "other.topic", false},
		{"*.*.*", "beads.bead.created", true},
		{"*.*.*", "beads.bead", false},
	} {
		t.Run(tc.pattern+"_"+tc.topic, func(t *testing.T) {
			got := matchTopicPattern(tc.pattern, tc.topic)
			if got != tc.want {
				t.Fatalf("matchTopicPattern(%q, %q) = %v, want %v", tc.pattern, tc.topic, got, tc.want)
			}
		})
	}
}

// TestHandleEventStream_SSE tests the full HTTP SSE endpoint.
func TestHandleEventStream_SSE(t *testing.T) {
	srv, _, handler := newTestServer()

	// Start the SSE request in a goroutine.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest("GET", "/v1/events/stream", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rec, req)
	}()

	// Give the handler time to register the subscription.
	time.Sleep(50 * time.Millisecond)

	// Broadcast an event.
	srv.sseHub.broadcast("beads.bead.created", []byte(`{"id":"kd-sse1"}`))

	// Give it time to be written.
	time.Sleep(50 * time.Millisecond)

	// Cancel the context to end the stream.
	cancel()
	<-done

	// Check response headers.
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected Content-Type=text/event-stream, got %q", ct)
	}

	// Parse the SSE output.
	body := rec.Body.String()
	if !strings.Contains(body, "event:beads.bead.created") {
		t.Fatalf("expected event:beads.bead.created in body, got:\n%s", body)
	}
	if !strings.Contains(body, `data:{"id":"kd-sse1"}`) {
		t.Fatalf("expected data with kd-sse1 in body, got:\n%s", body)
	}
	if !strings.Contains(body, "id:") {
		t.Fatalf("expected id: field in body, got:\n%s", body)
	}
}

// TestHandleEventStream_TopicFilter tests the ?topics= query param.
func TestHandleEventStream_TopicFilter(t *testing.T) {
	srv, _, handler := newTestServer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest("GET", "/v1/events/stream?topics=beads.label.*", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(50 * time.Millisecond)

	// Broadcast a bead event (should be filtered) and a label event (should pass).
	srv.sseHub.broadcast("beads.bead.created", []byte(`{"id":"kd-1"}`))
	srv.sseHub.broadcast("beads.label.added", []byte(`{"label":"urgent"}`))

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if strings.Contains(body, "beads.bead.created") {
		t.Fatalf("expected bead event to be filtered out, got:\n%s", body)
	}
	if !strings.Contains(body, "beads.label.added") {
		t.Fatalf("expected label event in body, got:\n%s", body)
	}
}

// TestHandleEventStream_LastEventID tests reconnection with Last-Event-ID.
func TestHandleEventStream_LastEventID(t *testing.T) {
	srv, _, handler := newTestServer()

	// Pre-broadcast 3 events before connecting.
	srv.sseHub.broadcast("beads.bead.created", []byte(`{"n":1}`))
	srv.sseHub.broadcast("beads.bead.updated", []byte(`{"n":2}`))
	srv.sseHub.broadcast("beads.bead.closed", []byte(`{"n":3}`))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest("GET", "/v1/events/stream", nil)
	req.Header.Set("Last-Event-ID", "1") // Should replay events 2 and 3.
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	// Should contain events 2 and 3 but not event 1.
	if strings.Contains(body, `data:{"n":1}`) {
		t.Fatalf("expected event 1 to be skipped, got:\n%s", body)
	}
	if !strings.Contains(body, `data:{"n":2}`) {
		t.Fatalf("expected event 2 in body, got:\n%s", body)
	}
	if !strings.Contains(body, `data:{"n":3}`) {
		t.Fatalf("expected event 3 in body, got:\n%s", body)
	}
}

// TestHandleEventStream_RecordAndPublish tests that recordAndPublish broadcasts to SSE.
func TestHandleEventStream_RecordAndPublish(t *testing.T) {
	srv, _, handler := newTestServer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest("GET", "/v1/events/stream", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(50 * time.Millisecond)

	// Use recordAndPublish (which the HTTP handlers use) to emit an event.
	srv.recordAndPublish(context.Background(), events.TopicBeadCreated, "kd-sse-rp",
		"alice", events.BeadCreated{})

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "event:beads.bead.created") {
		t.Fatalf("expected SSE event from recordAndPublish, got:\n%s", body)
	}
}

// TestHandleEventStream_MultipleClients verifies fan-out to multiple clients.
func TestHandleEventStream_MultipleClients(t *testing.T) {
	srv, _, handler := newTestServer()

	startClient := func() (*httptest.ResponseRecorder, context.CancelFunc, <-chan struct{}) {
		ctx, cancel := context.WithCancel(context.Background())
		req := httptest.NewRequest("GET", "/v1/events/stream", nil)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		done := make(chan struct{})
		go func() {
			defer close(done)
			handler.ServeHTTP(rec, req)
		}()
		return rec, cancel, done
	}

	rec1, cancel1, done1 := startClient()
	defer cancel1()
	rec2, cancel2, done2 := startClient()
	defer cancel2()

	time.Sleep(50 * time.Millisecond)

	srv.sseHub.broadcast("beads.bead.created", []byte(`{"id":"kd-multi"}`))

	time.Sleep(50 * time.Millisecond)
	cancel1()
	cancel2()
	<-done1
	<-done2

	for i, rec := range []*httptest.ResponseRecorder{rec1, rec2} {
		body := rec.Body.String()
		if !strings.Contains(body, "beads.bead.created") {
			t.Fatalf("client %d: expected bead event, got:\n%s", i+1, body)
		}
	}
}

// TestSSEEventFormat verifies the exact SSE wire format.
func TestSSEEventFormat(t *testing.T) {
	srv, _, handler := newTestServer()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest("GET", "/v1/events/stream", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(50 * time.Millisecond)
	srv.sseHub.broadcast("beads.bead.created", []byte(`{"id":"kd-fmt"}`))
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	// Parse SSE events from body.
	scanner := bufio.NewScanner(strings.NewReader(rec.Body.String()))
	var id, event, data string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "id:") {
			id = strings.TrimPrefix(line, "id:")
		} else if strings.HasPrefix(line, "event:") {
			event = strings.TrimPrefix(line, "event:")
		} else if strings.HasPrefix(line, "data:") {
			data = strings.TrimPrefix(line, "data:")
		}
	}

	if id == "" {
		t.Fatal("expected non-empty id field")
	}
	if event != "beads.bead.created" {
		t.Fatalf("expected event=beads.bead.created, got %q", event)
	}
	if !json.Valid([]byte(data)) {
		t.Fatalf("expected valid JSON data, got %q", data)
	}
	if data != `{"id":"kd-fmt"}` {
		t.Fatalf("expected data=%q, got %q", `{"id":"kd-fmt"}`, data)
	}
}
