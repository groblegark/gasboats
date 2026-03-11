package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"gasboat/controller/internal/beadsapi"
)

// --- helpers ---

func TestParseIntField(t *testing.T) {
	tests := []struct {
		input  string
		defVal int
		want   int
	}{
		{"", 5, 5},
		{"0", 5, 0},
		{"3", 5, 3},
		{"10", 0, 10},
		{"-1", 5, 5},
		{"abc", 5, 5},
		{"100", 0, 100},
	}
	for _, tt := range tests {
		got := parseIntField(tt.input, tt.defVal)
		if got != tt.want {
			t.Errorf("parseIntField(%q, %d) = %d, want %d", tt.input, tt.defVal, got, tt.want)
		}
	}
}

func TestParseDurationField(t *testing.T) {
	tests := []struct {
		input  string
		defVal time.Duration
		want   time.Duration
	}{
		{"", 30 * time.Minute, 30 * time.Minute},
		{"10m", 30 * time.Minute, 10 * time.Minute},
		{"1h", 30 * time.Minute, 1 * time.Hour},
		{"45s", 30 * time.Minute, 45 * time.Second},
		{"2h30m", 30 * time.Minute, 2*time.Hour + 30*time.Minute},
		// Plain integer → minutes.
		{"15", 30 * time.Minute, 15 * time.Minute},
		{"1", 30 * time.Minute, 1 * time.Minute},
		// Invalid.
		{"abc", 30 * time.Minute, 30 * time.Minute},
		{"-5m", 30 * time.Minute, 30 * time.Minute},
		{"0", 30 * time.Minute, 30 * time.Minute},   // plain int 0 → invalid
		{"0s", 30 * time.Minute, 30 * time.Minute},   // zero duration → default
		{"-1", 30 * time.Minute, 30 * time.Minute},   // negative plain int
	}
	for _, tt := range tests {
		got := parseDurationField(tt.input, tt.defVal)
		if got != tt.want {
			t.Errorf("parseDurationField(%q, %v) = %v, want %v", tt.input, tt.defVal, got, tt.want)
		}
	}
}

// --- mock daemon ---

// mockBeads is a simple in-memory mock of the beads daemon API.
type mockBeads struct {
	mu     sync.Mutex
	beads  map[string]*beadsapi.BeadDetail
	closed map[string]bool
	// Track calls for assertions.
	updateCalls []updateCall
	closeCalls  []string
	createCalls int
}

type updateCall struct {
	id     string
	fields map[string]string
}

func newMockBeads() *mockBeads {
	return &mockBeads{
		beads:  make(map[string]*beadsapi.BeadDetail),
		closed: make(map[string]bool),
	}
}

func (m *mockBeads) handler() http.Handler {
	mux := http.NewServeMux()

	// GET /v1/beads?type=schedule&status=open,in_progress → list
	mux.HandleFunc("/v1/beads", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// CreateBead
			m.mu.Lock()
			m.createCalls++
			id := fmt.Sprintf("mock-bead-%d", m.createCalls)
			m.beads[id] = &beadsapi.BeadDetail{ID: id, Status: "open", Fields: map[string]string{}}
			m.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"id": id})
			return
		}
		// ListBeadsFiltered
		m.mu.Lock()
		var result []any
		for _, b := range m.beads {
			if b.Type != "schedule" {
				continue
			}
			result = append(result, map[string]any{
				"id":     b.ID,
				"title":  b.Title,
				"type":   b.Type,
				"status": b.Status,
				"fields": b.Fields,
			})
		}
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"beads": result, "total": len(result)})
	})

	// /v1/beads/{id} and sub-resources
	mux.HandleFunc("/v1/beads/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/v1/beads/"):]

		// Sub-resources: /v1/beads/{id}/close, /labels, /deps
		for _, suffix := range []string{"/close", "/labels", "/deps"} {
			if idx := len(path) - len(suffix); idx > 0 && path[idx:] == suffix {
				beadID := path[:idx]
				switch suffix {
				case "/close":
					m.mu.Lock()
					m.closeCalls = append(m.closeCalls, beadID)
					if b, ok := m.beads[beadID]; ok {
						b.Status = "closed"
					}
					m.mu.Unlock()
				}
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
				return
			}
		}

		// Bare bead path: /v1/beads/{id}
		beadID := path
		if r.Method == http.MethodPatch {
			// UpdateBeadFields: PATCH /v1/beads/{id} with {"fields": {...}}
			var body map[string]json.RawMessage
			_ = json.NewDecoder(r.Body).Decode(&body)
			var fields map[string]any
			if raw, ok := body["fields"]; ok {
				_ = json.Unmarshal(raw, &fields)
			}
			// Convert to string map for tracking.
			strFields := make(map[string]string)
			for k, v := range fields {
				switch val := v.(type) {
				case string:
					strFields[k] = val
				case bool:
					strFields[k] = fmt.Sprintf("%v", val)
				default:
					strFields[k] = fmt.Sprintf("%v", val)
				}
			}
			m.mu.Lock()
			m.updateCalls = append(m.updateCalls, updateCall{id: beadID, fields: strFields})
			if b, ok := m.beads[beadID]; ok {
				for k, v := range strFields {
					b.Fields[k] = v
				}
			}
			m.mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
			return
		}

		// GET /v1/beads/{id}
		m.mu.Lock()
		b, ok := m.beads[beadID]
		m.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     b.ID,
			"title":  b.Title,
			"type":   b.Type,
			"status": b.Status,
			"fields": b.Fields,
		})
	})

	return mux
}

func newTestScheduler(srv *httptest.Server) *Scheduler {
	client, _ := beadsapi.New(beadsapi.Config{HTTPAddr: srv.URL})
	return New(client, slog.Default())
}

// --- guard rail tests ---

func TestIsLastAgentRunning_NoAgent(t *testing.T) {
	mock := newMockBeads()
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	// No last agent → not running.
	got := s.isLastAgentRunning(context.Background(), Schedule{})
	if got {
		t.Error("expected false for schedule with no last agent")
	}
}

func TestIsLastAgentRunning_AgentClosed(t *testing.T) {
	mock := newMockBeads()
	mock.beads["agent-1"] = &beadsapi.BeadDetail{
		ID: "agent-1", Type: "agent", Status: "closed",
		Fields: map[string]string{},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	got := s.isLastAgentRunning(context.Background(), Schedule{LastAgentID: "agent-1"})
	if got {
		t.Error("expected false for closed agent")
	}
}

func TestIsLastAgentRunning_AgentOpen(t *testing.T) {
	mock := newMockBeads()
	mock.beads["agent-1"] = &beadsapi.BeadDetail{
		ID: "agent-1", Type: "agent", Status: "open",
		Fields: map[string]string{},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	got := s.isLastAgentRunning(context.Background(), Schedule{LastAgentID: "agent-1"})
	if !got {
		t.Error("expected true for open agent")
	}
}

func TestIsLastAgentRunning_AgentNotFound(t *testing.T) {
	mock := newMockBeads()
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	// Unknown agent → can't determine status → allow spawn.
	got := s.isLastAgentRunning(context.Background(), Schedule{LastAgentID: "nonexistent"})
	if got {
		t.Error("expected false when agent not found (permissive)")
	}
}

func TestRecordFailure_IncrementsCount(t *testing.T) {
	mock := newMockBeads()
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Fields: map[string]string{"consecutive_failures": "0"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	sched := &Schedule{
		ID:                  "sched-1",
		Enabled:             true,
		MaxRetries:          3,
		ConsecutiveFailures: 0,
	}

	s.recordFailure(context.Background(), sched)

	if sched.ConsecutiveFailures != 1 {
		t.Errorf("expected 1 consecutive failure, got %d", sched.ConsecutiveFailures)
	}
	if !sched.Enabled {
		t.Error("should not be disabled after 1 failure")
	}
}

func TestRecordFailure_AutoDisablesAtMaxRetries(t *testing.T) {
	mock := newMockBeads()
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Fields: map[string]string{"consecutive_failures": "2"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	sched := &Schedule{
		ID:                  "sched-1",
		Enabled:             true,
		MaxRetries:          3,
		ConsecutiveFailures: 2, // Already at 2, one more → auto-disable.
	}

	s.recordFailure(context.Background(), sched)

	if sched.ConsecutiveFailures != 3 {
		t.Errorf("expected 3 consecutive failures, got %d", sched.ConsecutiveFailures)
	}
	if sched.Enabled {
		t.Error("expected schedule to be auto-disabled at max retries")
	}

	// Check that enabled=false was written to the bead.
	mock.mu.Lock()
	defer mock.mu.Unlock()
	found := false
	for _, call := range mock.updateCalls {
		if call.id == "sched-1" && call.fields["enabled"] == "false" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected UpdateBeadFields to set enabled=false")
	}
}

func TestTrackLastAgentStatus_SuccessResetsFailures(t *testing.T) {
	mock := newMockBeads()
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Fields: map[string]string{
			"consecutive_failures": "2",
			"last_agent_id":        "agent-done",
		},
	}
	mock.beads["agent-done"] = &beadsapi.BeadDetail{
		ID: "agent-done", Type: "agent", Status: "closed",
		Fields: map[string]string{"agent_state": "done"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	sched := &Schedule{
		ID:                  "sched-1",
		LastAgentID:         "agent-done",
		ConsecutiveFailures: 2,
	}

	s.trackLastAgentStatus(context.Background(), sched)

	if sched.ConsecutiveFailures != 0 {
		t.Errorf("expected failures reset to 0, got %d", sched.ConsecutiveFailures)
	}
}

func TestTrackLastAgentStatus_FailureIncrements(t *testing.T) {
	mock := newMockBeads()
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Fields: map[string]string{
			"consecutive_failures": "1",
			"last_agent_id":        "agent-fail",
		},
	}
	mock.beads["agent-fail"] = &beadsapi.BeadDetail{
		ID: "agent-fail", Type: "agent", Status: "closed",
		Fields: map[string]string{"agent_state": "failed"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	sched := &Schedule{
		ID:                  "sched-1",
		LastAgentID:         "agent-fail",
		ConsecutiveFailures: 1,
		MaxRetries:          5,
		Enabled:             true,
	}

	s.trackLastAgentStatus(context.Background(), sched)

	if sched.ConsecutiveFailures != 2 {
		t.Errorf("expected 2 consecutive failures, got %d", sched.ConsecutiveFailures)
	}
}

func TestTrackLastAgentStatus_AutoDisablesAtMaxRetries(t *testing.T) {
	mock := newMockBeads()
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Fields: map[string]string{
			"consecutive_failures": "2",
			"last_agent_id":        "agent-fail",
		},
	}
	mock.beads["agent-fail"] = &beadsapi.BeadDetail{
		ID: "agent-fail", Type: "agent", Status: "closed",
		Fields: map[string]string{"agent_state": "failed"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	sched := &Schedule{
		ID:                  "sched-1",
		LastAgentID:         "agent-fail",
		ConsecutiveFailures: 2,
		MaxRetries:          3,
		Enabled:             true,
	}

	s.trackLastAgentStatus(context.Background(), sched)

	if sched.ConsecutiveFailures != 3 {
		t.Errorf("expected 3 failures, got %d", sched.ConsecutiveFailures)
	}
	if sched.Enabled {
		t.Error("expected schedule to be auto-disabled")
	}
}

func TestTrackLastAgentStatus_SkipsAlreadyCheckedAgent(t *testing.T) {
	mock := newMockBeads()
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Fields: map[string]string{
			"last_agent_id":        "agent-1",
			"last_checked_agent":   "agent-1", // Already checked.
			"consecutive_failures": "1",
		},
	}
	mock.beads["agent-1"] = &beadsapi.BeadDetail{
		ID: "agent-1", Type: "agent", Status: "closed",
		Fields: map[string]string{"agent_state": "failed"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	sched := &Schedule{
		ID:                  "sched-1",
		LastAgentID:         "agent-1",
		ConsecutiveFailures: 1,
		MaxRetries:          5,
	}

	s.trackLastAgentStatus(context.Background(), sched)

	// Should not have changed because this agent was already checked.
	if sched.ConsecutiveFailures != 1 {
		t.Errorf("expected failures unchanged at 1, got %d", sched.ConsecutiveFailures)
	}
}

func TestTrackLastAgentStatus_SkipsNonClosedAgent(t *testing.T) {
	mock := newMockBeads()
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Fields: map[string]string{"last_agent_id": "agent-running"},
	}
	mock.beads["agent-running"] = &beadsapi.BeadDetail{
		ID: "agent-running", Type: "agent", Status: "in_progress",
		Fields: map[string]string{"agent_state": "working"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	sched := &Schedule{
		ID:                  "sched-1",
		LastAgentID:         "agent-running",
		ConsecutiveFailures: 0,
	}

	s.trackLastAgentStatus(context.Background(), sched)

	// Should not have changed — agent still running.
	if sched.ConsecutiveFailures != 0 {
		t.Errorf("expected failures unchanged at 0, got %d", sched.ConsecutiveFailures)
	}
}

func TestEnforceTimeouts_KillsTimedOutAgent(t *testing.T) {
	mock := newMockBeads()
	mock.beads["agent-slow"] = &beadsapi.BeadDetail{
		ID: "agent-slow", Type: "agent", Status: "open",
		Fields: map[string]string{"agent_state": "working"},
	}
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Fields: map[string]string{"consecutive_failures": "0"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	now := time.Now()
	schedules := []Schedule{
		{
			ID:                  "sched-1",
			LastAgentID:         "agent-slow",
			LastRun:             now.Add(-45 * time.Minute),
			Timeout:             30 * time.Minute,
			MaxRetries:          3,
			ConsecutiveFailures: 0,
			Enabled:             true,
		},
	}

	s.enforceTimeouts(context.Background(), schedules, now)

	mock.mu.Lock()
	defer mock.mu.Unlock()

	// Should have closed the agent.
	if len(mock.closeCalls) == 0 {
		t.Fatal("expected agent to be closed")
	}
	if mock.closeCalls[0] != "agent-slow" {
		t.Errorf("expected agent-slow to be closed, got %q", mock.closeCalls[0])
	}

	// Should have set agent_state=failed.
	found := false
	for _, call := range mock.updateCalls {
		if call.id == "agent-slow" && call.fields["agent_state"] == "failed" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected agent_state=failed to be set on timed out agent")
	}

	// Should have recorded the failure on the schedule.
	if schedules[0].ConsecutiveFailures != 1 {
		t.Errorf("expected 1 failure after timeout, got %d", schedules[0].ConsecutiveFailures)
	}
}

func TestEnforceTimeouts_SkipsAgentWithinTimeout(t *testing.T) {
	mock := newMockBeads()
	mock.beads["agent-ok"] = &beadsapi.BeadDetail{
		ID: "agent-ok", Type: "agent", Status: "open",
		Fields: map[string]string{"agent_state": "working"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	now := time.Now()
	schedules := []Schedule{
		{
			ID:          "sched-1",
			LastAgentID: "agent-ok",
			LastRun:     now.Add(-10 * time.Minute),
			Timeout:     30 * time.Minute,
		},
	}

	s.enforceTimeouts(context.Background(), schedules, now)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.closeCalls) > 0 {
		t.Error("agent within timeout should not be closed")
	}
}

func TestEnforceTimeouts_SkipsAlreadyClosedAgent(t *testing.T) {
	mock := newMockBeads()
	mock.beads["agent-done"] = &beadsapi.BeadDetail{
		ID: "agent-done", Type: "agent", Status: "closed",
		Fields: map[string]string{"agent_state": "done"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	now := time.Now()
	schedules := []Schedule{
		{
			ID:          "sched-1",
			LastAgentID: "agent-done",
			LastRun:     now.Add(-45 * time.Minute),
			Timeout:     30 * time.Minute,
		},
	}

	s.enforceTimeouts(context.Background(), schedules, now)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.closeCalls) > 0 {
		t.Error("already-closed agent should not be closed again")
	}
}

func TestReconcile_SkipsDisabledSchedule(t *testing.T) {
	mock := newMockBeads()
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Title: "Disabled Schedule",
		Fields: map[string]string{
			"cron":    "* * * * *",
			"enabled": "false",
			"prompt":  "do something",
		},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}

	// Should not have spawned any agents.
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.createCalls > 0 {
		t.Error("disabled schedule should not spawn agents")
	}
}

func TestReconcile_SkipsConcurrencyLimitReached(t *testing.T) {
	mock := newMockBeads()
	mock.beads["agent-running"] = &beadsapi.BeadDetail{
		ID: "agent-running", Type: "agent", Status: "open",
		Fields: map[string]string{"agent_state": "working"},
	}
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Title: "Busy Schedule",
		Fields: map[string]string{
			"cron":           "* * * * *",
			"enabled":        "true",
			"prompt":         "do something",
			"last_agent_id":  "agent-running",
			"max_concurrent": "1",
		},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := newTestScheduler(srv)
	err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}

	// The only create calls should be 0 because the last agent is still running.
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.createCalls > 0 {
		t.Error("should not spawn agent when concurrency limit reached")
	}
}
