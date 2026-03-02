package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/groblegark/kbeads/internal/eventbus"
	"github.com/groblegark/kbeads/internal/model"
	"github.com/nats-io/nats.go"
)

// publishedMsg records a message published to the mock JetStream.
type publishedMsg struct {
	subject string
	data    []byte
}

// mockJetStream is a minimal mock of nats.JetStreamContext that records
// Publish calls. Unimplemented methods will panic if called.
type mockJetStream struct {
	nats.JetStreamContext
	published []publishedMsg
}

func (m *mockJetStream) Publish(subject string, data []byte, _ ...nats.PubOpt) (*nats.PubAck, error) {
	m.published = append(m.published, publishedMsg{subject: subject, data: data})
	return &nats.PubAck{Stream: "HOOK_EVENTS", Sequence: uint64(len(m.published))}, nil
}

// newTestBusWithJS creates an event bus backed by a mock JetStream, returning
// both so tests can inspect published messages.
func newTestBusWithJS() (*eventbus.Bus, *mockJetStream) {
	js := &mockJetStream{}
	bus := eventbus.New()
	bus.SetJetStream(js)
	return bus, js
}

// TestHandleHookEmit_NoAgentBeadID verifies that a hook emit without
// agent_bead_id returns immediately with no block (no gates to check).
func TestHandleHookEmit_NoAgentBeadID(t *testing.T) {
	_, _, h := newTestServer()

	rec := doJSON(t, h, "POST", "/v1/hooks/emit", map[string]any{
		"hook_type":         "PreToolUse",
		"claude_session_id": "sess-1",
		"cwd":               "/workspace",
		"actor":             "test-agent",
		"tool_name":         "Bash",
	})
	requireStatus(t, rec, 200)

	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["block"] == true {
		t.Fatalf("expected no block when agent_bead_id is empty, got %v", resp)
	}
}

// TestHandleHookEmit_PreToolUse verifies that non-Stop hooks with an
// agent_bead_id skip gate evaluation and return without blocking.
func TestHandleHookEmit_PreToolUse(t *testing.T) {
	_, _, h := newTestServer()

	rec := doJSON(t, h, "POST", "/v1/hooks/emit", map[string]any{
		"agent_bead_id":     "kd-agent-1",
		"hook_type":         "PreToolUse",
		"claude_session_id": "sess-1",
		"cwd":               "/workspace",
		"actor":             "test-agent",
		"tool_name":         "Read",
	})
	requireStatus(t, rec, 200)

	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["block"] == true {
		t.Fatalf("expected no block for PreToolUse hook, got %v", resp)
	}
}

// TestHandleHookEmit_RecordsPresence verifies that a hook emit with an actor
// records presence for the agent roster.
func TestHandleHookEmit_RecordsPresence(t *testing.T) {
	srv, _, h := newTestServer()

	doJSON(t, h, "POST", "/v1/hooks/emit", map[string]any{
		"hook_type":         "PostToolUse",
		"claude_session_id": "sess-1",
		"cwd":               "/workspace",
		"actor":             "wise-newt",
		"tool_name":         "Bash",
	})

	roster := srv.Presence.Roster(0)
	if len(roster) != 1 {
		t.Fatalf("expected 1 roster entry, got %d", len(roster))
	}
	if roster[0].Actor != "wise-newt" {
		t.Fatalf("expected actor=wise-newt, got %q", roster[0].Actor)
	}
}

// TestHandleHookEmit_InvalidJSON verifies that an invalid JSON body
// returns 400.
func TestHandleHookEmit_InvalidJSON(t *testing.T) {
	_, _, h := newTestServer()

	req := httptest.NewRequest("POST", "/v1/hooks/emit", strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	requireStatus(t, rec, 400)
}

// TestHandleHookEmit_StopBlocks verifies that Stop hook blocks when
// no gate is satisfied (using the base mockStore which always returns false).
func TestHandleHookEmit_StopBlocks(t *testing.T) {
	_, _, h := newTestServer()

	rec := doJSON(t, h, "POST", "/v1/hooks/emit", map[string]any{
		"agent_bead_id":     "kd-agent-1",
		"hook_type":         "Stop",
		"claude_session_id": "sess-1",
		"actor":             "test-agent",
	})
	requireStatus(t, rec, 200)

	var resp map[string]any
	decodeJSON(t, rec, &resp)
	if resp["block"] != true {
		t.Fatalf("expected block=true for Stop with unsatisfied gate, got %v", resp)
	}
}

// TestHandleHookEmit_NilPresence verifies that a hook emit works when
// Presence is nil (no presence tracking).
func TestHandleHookEmit_NilPresence(t *testing.T) {
	srv, _, h := newTestServer()
	srv.Presence = nil

	rec := doJSON(t, h, "POST", "/v1/hooks/emit", map[string]any{
		"hook_type":         "PreToolUse",
		"claude_session_id": "sess-1",
		"actor":             "test-agent",
	})
	requireStatus(t, rec, 200)
}

// TestHandleHookPublish_Success verifies that a valid publish request
// returns 204 and publishes to JetStream.
func TestHandleHookPublish_Success(t *testing.T) {
	srv, _, h := newTestServer()

	bus, js := newTestBusWithJS()
	srv.SetBus(bus)

	rec := doJSON(t, h, "POST", "/v1/hooks/publish", map[string]any{
		"subject": "hooks.worker-1.PreToolUse",
		"payload": map[string]any{
			"agent": "worker-1",
			"event": "PreToolUse",
			"ts":    "2026-03-02T07:30:00Z",
		},
	})
	requireStatus(t, rec, 204)

	if len(js.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(js.published))
	}
	if js.published[0].subject != "hooks.worker-1.PreToolUse" {
		t.Errorf("expected subject hooks.worker-1.PreToolUse, got %s", js.published[0].subject)
	}
}

// TestHandleHookPublish_MissingSubject verifies that a missing subject
// returns 400.
func TestHandleHookPublish_MissingSubject(t *testing.T) {
	srv, _, h := newTestServer()
	bus, _ := newTestBusWithJS()
	srv.SetBus(bus)

	rec := doJSON(t, h, "POST", "/v1/hooks/publish", map[string]any{
		"payload": map[string]any{"event": "Stop"},
	})
	requireStatus(t, rec, 400)
}

// TestHandleHookPublish_MissingPayload verifies that a missing payload
// returns 400.
func TestHandleHookPublish_MissingPayload(t *testing.T) {
	srv, _, h := newTestServer()
	bus, _ := newTestBusWithJS()
	srv.SetBus(bus)

	rec := doJSON(t, h, "POST", "/v1/hooks/publish", map[string]any{
		"subject": "hooks.worker-1.Stop",
	})
	requireStatus(t, rec, 400)
}

// TestHandleHookPublish_InvalidJSON verifies that invalid JSON returns 400.
func TestHandleHookPublish_InvalidJSON(t *testing.T) {
	_, _, h := newTestServer()

	req := httptest.NewRequest("POST", "/v1/hooks/publish", strings.NewReader("{bad json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	requireStatus(t, rec, 400)
}

// TestHandleHookPublish_NoBus verifies that publishing without a configured
// event bus returns 503.
func TestHandleHookPublish_NoBus(t *testing.T) {
	_, _, h := newTestServer()

	rec := doJSON(t, h, "POST", "/v1/hooks/publish", map[string]any{
		"subject": "hooks.worker-1.Stop",
		"payload": map[string]any{"event": "Stop"},
	})
	requireStatus(t, rec, 503)
}

// TestHandleExecuteHooks_Valid verifies a valid request returns 200.
func TestHandleExecuteHooks_Valid(t *testing.T) {
	_, _, h := newTestServer()

	rec := doJSON(t, h, "POST", "/v1/hooks/execute", map[string]any{
		"agent_id": "test-agent",
		"trigger":  "session-end",
		"cwd":      "/workspace",
	})
	requireStatus(t, rec, 200)

	var resp map[string]any
	decodeJSON(t, rec, &resp)
	// Should not block when no advice beads exist.
	if resp["block"] == true {
		t.Fatalf("expected no block when no advice beads exist, got %v", resp)
	}
}

// TestHandleExecuteHooks_MissingAgentID verifies that a missing agent_id
// returns 400.
func TestHandleExecuteHooks_MissingAgentID(t *testing.T) {
	_, _, h := newTestServer()

	rec := doJSON(t, h, "POST", "/v1/hooks/execute", map[string]any{
		"trigger": "session-end",
	})
	requireStatus(t, rec, 400)
}

// TestHandleExecuteHooks_MissingTrigger verifies that a missing trigger
// returns 400.
func TestHandleExecuteHooks_MissingTrigger(t *testing.T) {
	_, _, h := newTestServer()

	rec := doJSON(t, h, "POST", "/v1/hooks/execute", map[string]any{
		"agent_id": "test-agent",
	})
	requireStatus(t, rec, 400)
}

// TestHandleExecuteHooks_InvalidJSON verifies that invalid JSON returns 400.
func TestHandleExecuteHooks_InvalidJSON(t *testing.T) {
	_, _, h := newTestServer()

	req := httptest.NewRequest("POST", "/v1/hooks/execute", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	requireStatus(t, rec, 400)
}

// TestHandleExecuteHooks_WithAdvice verifies that the hooks handler evaluates
// advice beads that match the agent.
func TestHandleExecuteHooks_WithAdvice(t *testing.T) {
	_, ms, h := newTestServer()

	// Create a bead with labels for targeting.
	ms.beads["test-agent"] = &model.Bead{
		ID:     "test-agent",
		Type:   model.TypeTask,
		Kind:   model.KindIssue,
		Status: model.StatusOpen,
	}
	ms.labels["test-agent"] = []string{"role:developer"}

	rec := doJSON(t, h, "POST", "/v1/hooks/execute", map[string]any{
		"agent_id": "test-agent",
		"trigger":  "session-end",
	})
	requireStatus(t, rec, 200)
}
