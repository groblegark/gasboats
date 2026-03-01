package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/groblegark/kbeads/internal/eventbus"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// --- handleBusStatus tests ---

func TestBusStatusNoBus(t *testing.T) {
	_, _, handler := newTestServer()
	rec := doJSON(t, handler, "GET", "/v1/bus/status", nil)
	requireStatus(t, rec, 200)

	var resp busStatusResponse
	decodeJSON(t, rec, &resp)

	if resp.JetStreamEnabled {
		t.Error("expected jetstream_enabled=false when bus is nil")
	}
	if resp.HandlerCount != 0 {
		t.Errorf("handler_count = %d, want 0", resp.HandlerCount)
	}
	if len(resp.Streams) != len(eventbus.StreamNames) {
		t.Errorf("streams count = %d, want %d", len(resp.Streams), len(eventbus.StreamNames))
	}
	if len(resp.Handlers) != 0 {
		t.Error("handlers should be empty when bus is nil")
	}
}

func TestBusStatusWithBusNoJetStream(t *testing.T) {
	s, _, handler := newTestServer()
	bus := eventbus.New()
	s.SetBus(bus)

	rec := doJSON(t, handler, "GET", "/v1/bus/status", nil)
	requireStatus(t, rec, 200)

	var resp busStatusResponse
	decodeJSON(t, rec, &resp)

	if resp.JetStreamEnabled {
		t.Error("expected jetstream_enabled=false when JetStream not set")
	}
	if resp.HandlerCount != 0 {
		t.Errorf("handler_count = %d, want 0", resp.HandlerCount)
	}
}

func TestBusStatusWithHandlers(t *testing.T) {
	s, _, handler := newTestServer()
	bus := eventbus.New()
	bus.Register(&busStubHandler{id: "handler-a"})
	bus.Register(&busStubHandler{id: "handler-b"})
	s.SetBus(bus)

	rec := doJSON(t, handler, "GET", "/v1/bus/status", nil)
	requireStatus(t, rec, 200)

	var resp busStatusResponse
	decodeJSON(t, rec, &resp)

	if resp.HandlerCount != 2 {
		t.Errorf("handler_count = %d, want 2", resp.HandlerCount)
	}
	if len(resp.Handlers) != 2 {
		t.Errorf("handlers len = %d, want 2", len(resp.Handlers))
	}

	ids := map[string]bool{}
	for _, h := range resp.Handlers {
		ids[h] = true
	}
	if !ids["handler-a"] || !ids["handler-b"] {
		t.Errorf("handlers = %v, want handler-a and handler-b", resp.Handlers)
	}
}

func TestBusStatusStreamsAlwaysPresent(t *testing.T) {
	_, _, handler := newTestServer()
	rec := doJSON(t, handler, "GET", "/v1/bus/status", nil)
	requireStatus(t, rec, 200)

	var resp busStatusResponse
	decodeJSON(t, rec, &resp)

	expected := map[string]bool{
		"hooks": true, "decisions": true, "agents": true,
		"mail": true, "mutations": true, "config": true,
		"gate": true, "inbox": true, "jack": true,
	}
	for _, s := range resp.Streams {
		if !expected[s] {
			t.Errorf("unexpected stream name %q", s)
		}
		delete(expected, s)
	}
	for s := range expected {
		t.Errorf("missing stream name %q", s)
	}
}

// --- handleBusEvents error path tests ---

func TestBusEventsNoBus(t *testing.T) {
	_, _, handler := newTestServer()
	rec := doJSON(t, handler, "GET", "/v1/bus/events", nil)
	requireStatus(t, rec, 503)

	var resp map[string]string
	decodeJSON(t, rec, &resp)
	if !strings.Contains(resp["error"], "JetStream not configured") {
		t.Errorf("error = %q, want JetStream not configured message", resp["error"])
	}
}

func TestBusEventsNoJetStream(t *testing.T) {
	s, _, handler := newTestServer()
	s.SetBus(eventbus.New()) // bus without JetStream

	rec := doJSON(t, handler, "GET", "/v1/bus/events", nil)
	requireStatus(t, rec, 503)
}

func TestBusEventsInvalidStream(t *testing.T) {
	ns, js := startTestNATS(t)
	_ = ns
	s, _, handler := newTestServer()
	bus := eventbus.New()
	bus.SetJetStream(js)
	s.SetBus(bus)

	rec := doJSON(t, handler, "GET", "/v1/bus/events?stream=bogus,invalid", nil)
	requireStatus(t, rec, 400)

	var resp map[string]string
	decodeJSON(t, rec, &resp)
	if !strings.Contains(resp["error"], "no valid stream names") {
		t.Errorf("error = %q, want 'no valid stream names'", resp["error"])
	}
}

// --- handleBusEvents streaming tests (require embedded NATS) ---

func TestBusEventsStreamHookEvent(t *testing.T) {
	ns, js := startTestNATS(t)
	_ = ns
	if err := eventbus.EnsureStreams(js); err != nil {
		t.Fatalf("EnsureStreams: %v", err)
	}

	s, _, handler := newTestServer()
	bus := eventbus.New()
	bus.SetJetStream(js)
	s.SetBus(bus)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/v1/bus/events?stream=hooks", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rec, req)
	}()

	// Wait for subscriptions to be set up.
	time.Sleep(100 * time.Millisecond)

	// Publish an event to the hooks stream.
	_, err := js.Publish("hooks.test-agent.SessionStart", []byte(`{"type":"SessionStart"}`))
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Wait for the event to propagate.
	time.Sleep(200 * time.Millisecond)

	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "event:hooks") {
		t.Errorf("SSE body should contain event:hooks, got:\n%s", body)
	}
	if !strings.Contains(body, `"stream":"hooks"`) {
		t.Errorf("SSE body should contain stream:hooks, got:\n%s", body)
	}
	if !strings.Contains(body, `"type":"SessionStart"`) {
		t.Errorf("SSE body should contain type:SessionStart, got:\n%s", body)
	}
}

func TestBusEventsStreamFilter(t *testing.T) {
	ns, js := startTestNATS(t)
	_ = ns
	if err := eventbus.EnsureStreams(js); err != nil {
		t.Fatalf("EnsureStreams: %v", err)
	}

	s, _, handler := newTestServer()
	bus := eventbus.New()
	bus.SetJetStream(js)
	s.SetBus(bus)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/v1/bus/events?stream=hooks&filter=Stop", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(100 * time.Millisecond)

	// Publish two events: one that matches the filter, one that doesn't.
	_, _ = js.Publish("hooks.agent1.SessionStart", []byte(`{"type":"SessionStart"}`))
	_, _ = js.Publish("hooks.agent1.Stop", []byte(`{"type":"Stop"}`))

	time.Sleep(200 * time.Millisecond)

	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, `"type":"Stop"`) {
		t.Errorf("SSE body should contain filtered event Stop, got:\n%s", body)
	}
	if strings.Contains(body, `"type":"SessionStart"`) {
		t.Errorf("SSE body should NOT contain SessionStart (filtered out), got:\n%s", body)
	}
}

func TestBusEventsMultipleStreams(t *testing.T) {
	ns, js := startTestNATS(t)
	_ = ns
	if err := eventbus.EnsureStreams(js); err != nil {
		t.Fatalf("EnsureStreams: %v", err)
	}

	s, _, handler := newTestServer()
	bus := eventbus.New()
	bus.SetJetStream(js)
	s.SetBus(bus)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/v1/bus/events?stream=hooks,decisions", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(100 * time.Millisecond)

	_, _ = js.Publish("hooks.agent1.SessionStart", []byte(`{"type":"SessionStart"}`))
	_, _ = js.Publish("decisions.agent1.DecisionCreated", []byte(`{"type":"DecisionCreated"}`))

	time.Sleep(200 * time.Millisecond)

	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "event:hooks") {
		t.Errorf("SSE body should contain event:hooks, got:\n%s", body)
	}
	if !strings.Contains(body, "event:decisions") {
		t.Errorf("SSE body should contain event:decisions, got:\n%s", body)
	}
}

func TestBusEventsSSEHeaders(t *testing.T) {
	ns, js := startTestNATS(t)
	_ = ns
	if err := eventbus.EnsureStreams(js); err != nil {
		t.Fatalf("EnsureStreams: %v", err)
	}

	s, _, handler := newTestServer()
	bus := eventbus.New()
	bus.SetJetStream(js)
	s.SetBus(bus)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/v1/bus/events?stream=hooks", nil)
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

	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	if xab := rec.Header().Get("X-Accel-Buffering"); xab != "no" {
		t.Errorf("X-Accel-Buffering = %q, want no", xab)
	}
}

func TestBusEventsSSEFormat(t *testing.T) {
	ns, js := startTestNATS(t)
	_ = ns
	if err := eventbus.EnsureStreams(js); err != nil {
		t.Fatalf("EnsureStreams: %v", err)
	}

	s, _, handler := newTestServer()
	bus := eventbus.New()
	bus.SetJetStream(js)
	s.SetBus(bus)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/v1/bus/events?stream=hooks", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(100 * time.Millisecond)

	_, _ = js.Publish("hooks.agent1.Stop", []byte(`{"stopped":true}`))

	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()

	// Verify SSE format: id:<n>\nevent:<stream>\ndata:<json>\n\n
	if !strings.Contains(body, "id:1\n") {
		t.Errorf("SSE body should contain id:1, got:\n%s", body)
	}
	if !strings.Contains(body, "event:hooks\n") {
		t.Errorf("SSE body should contain event:hooks, got:\n%s", body)
	}
	if !strings.Contains(body, "data:") {
		t.Errorf("SSE body should contain data: prefix, got:\n%s", body)
	}

	// Extract and verify the JSON data payload.
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "data:") {
			var evt busSSEEvent
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data:")), &evt); err != nil {
				t.Fatalf("failed to parse SSE data payload: %v", err)
			}
			if evt.Stream != "hooks" {
				t.Errorf("payload stream = %q, want hooks", evt.Stream)
			}
			if evt.Type != "Stop" {
				t.Errorf("payload type = %q, want Stop", evt.Type)
			}
			if evt.Subject != "hooks.agent1.Stop" {
				t.Errorf("payload subject = %q, want hooks.agent1.Stop", evt.Subject)
			}
			return
		}
	}
	t.Error("no data: line found in SSE body")
}

func TestBusEventsStreamAllDefault(t *testing.T) {
	ns, js := startTestNATS(t)
	_ = ns
	if err := eventbus.EnsureStreams(js); err != nil {
		t.Fatalf("EnsureStreams: %v", err)
	}

	s, _, handler := newTestServer()
	bus := eventbus.New()
	bus.SetJetStream(js)
	s.SetBus(bus)

	ctx, cancel := context.WithCancel(context.Background())
	// No stream param = all streams.
	req := httptest.NewRequest("GET", "/v1/bus/events", nil)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.ServeHTTP(rec, req)
	}()

	time.Sleep(100 * time.Millisecond)

	// Publish to mutations stream (should be subscribed by default).
	_, _ = js.Publish("mutations.MutationCreate", []byte(`{"issue_id":"kd-123"}`))

	time.Sleep(200 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "event:mutations") {
		t.Errorf("SSE body should contain event:mutations when subscribing to all, got:\n%s", body)
	}
}

// --- helpers ---

// startTestNATS creates an embedded NATS server with JetStream for testing.
func startTestNATS(t *testing.T) (*server.Server, nats.JetStreamContext) {
	t.Helper()
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("failed to create NATS server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server not ready")
	}
	t.Cleanup(func() { ns.Shutdown() })

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("failed to connect to NATS: %v", err)
	}
	t.Cleanup(func() { nc.Close() })

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("failed to create JetStream context: %v", err)
	}
	return ns, js
}

// busStubHandler is a minimal eventbus.Handler for testing bus status.
type busStubHandler struct {
	id string
}

func (h *busStubHandler) ID() string                    { return h.id }
func (h *busStubHandler) Handles() []eventbus.EventType { return nil }
func (h *busStubHandler) Priority() int                 { return 0 }
func (h *busStubHandler) Handle(_ context.Context, _ *eventbus.Event, _ *eventbus.Result) error {
	return fmt.Errorf("not implemented")
}
