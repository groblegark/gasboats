package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gasboat/controller/internal/beadsapi"
)

// --- mock daemon for execution tracking tests ---

type trackingMock struct {
	mu       sync.Mutex
	beads    map[string]*beadsapi.BeadDetail
	comments []mockComment // recorded comments
	updates  []mockUpdate  // recorded field updates
	creates  int           // bead creation count
}

type mockComment struct {
	beadID string
	author string
	text   string
}

type mockUpdate struct {
	beadID string
	fields map[string]string
}

func newTrackingMock() *trackingMock {
	return &trackingMock{
		beads: make(map[string]*beadsapi.BeadDetail),
	}
}

func (m *trackingMock) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/v1/beads", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			m.mu.Lock()
			m.creates++
			id := fmt.Sprintf("mock-bead-%d", m.creates)
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
				"id": b.ID, "title": b.Title, "type": b.Type,
				"status": b.Status, "fields": b.Fields,
			})
		}
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"beads": result, "total": len(result)})
	})

	mux.HandleFunc("/v1/beads/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path[len("/v1/beads/"):]

		// /v1/beads/{id}/comments
		for _, suffix := range []string{"/comments", "/close", "/labels", "/deps"} {
			idx := len(path) - len(suffix)
			if idx > 0 && path[idx:] == suffix {
				beadID := path[:idx]
				if suffix == "/comments" && r.Method == http.MethodPost {
					var body map[string]string
					_ = json.NewDecoder(r.Body).Decode(&body)
					m.mu.Lock()
					m.comments = append(m.comments, mockComment{
						beadID: beadID,
						author: body["author"],
						text:   body["text"],
					})
					m.mu.Unlock()
				}
				if suffix == "/close" {
					m.mu.Lock()
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

		beadID := path
		if r.Method == http.MethodPatch {
			var body map[string]json.RawMessage
			_ = json.NewDecoder(r.Body).Decode(&body)
			var fields map[string]any
			if raw, ok := body["fields"]; ok {
				_ = json.Unmarshal(raw, &fields)
			}
			strFields := make(map[string]string)
			for k, v := range fields {
				strFields[k] = fmt.Sprintf("%v", v)
			}
			m.mu.Lock()
			m.updates = append(m.updates, mockUpdate{beadID: beadID, fields: strFields})
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

		m.mu.Lock()
		b, ok := m.beads[beadID]
		m.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": b.ID, "title": b.Title, "type": b.Type,
			"status": b.Status, "fields": b.Fields,
		})
	})

	return mux
}

func makeTrackingScheduler(srv *httptest.Server) *Scheduler {
	client, _ := beadsapi.New(beadsapi.Config{HTTPAddr: srv.URL})
	return New(client, slog.Default())
}

// --- tests ---

func TestFormatRunDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "0s"},
		{30 * time.Second, "30s"},
		{59 * time.Second, "59s"},
		{60 * time.Second, "1m"},
		{90 * time.Second, "1m 30s"},
		{5*time.Minute + 12*time.Second, "5m 12s"},
		{60 * time.Minute, "1h"},
		{90 * time.Minute, "1h 30m"},
		{2*time.Hour + 15*time.Minute, "2h 15m"},
	}
	for _, tc := range tests {
		if got := formatRunDuration(tc.d); got != tc.want {
			t.Errorf("formatRunDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestRecordRunCompletion_AgentDone(t *testing.T) {
	mock := newTrackingMock()
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Fields: map[string]string{"last_agent_id": "agent-1"},
	}
	mock.beads["agent-1"] = &beadsapi.BeadDetail{
		ID: "agent-1", Type: "agent", Status: "closed",
		Fields: map[string]string{"agent_state": "done"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := makeTrackingScheduler(srv)
	sched := Schedule{
		ID:          "sched-1",
		LastAgentID: "agent-1",
		LastRun:     time.Now().Add(-5 * time.Minute),
	}

	s.trackLastAgentStatus(context.Background(), &sched)

	mock.mu.Lock()
	defer mock.mu.Unlock()

	// Should have added a completion comment.
	found := false
	for _, c := range mock.comments {
		if c.beadID == "sched-1" && strings.Contains(c.text, "completed") && strings.Contains(c.text, "agent-1") {
			found = true
			if c.author != "scheduler" {
				t.Errorf("expected author=scheduler, got %q", c.author)
			}
			break
		}
	}
	if !found {
		t.Errorf("expected completion comment, got: %v", mock.comments)
	}

	// Should have set last_checked_agent.
	checkedFound := false
	for _, u := range mock.updates {
		if u.beadID == "sched-1" && u.fields["last_checked_agent"] == "agent-1" {
			checkedFound = true
			break
		}
	}
	if !checkedFound {
		t.Error("expected last_checked_agent to be set")
	}
}

func TestRecordRunCompletion_AgentFailed(t *testing.T) {
	mock := newTrackingMock()
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Fields: map[string]string{"last_agent_id": "agent-fail"},
	}
	mock.beads["agent-fail"] = &beadsapi.BeadDetail{
		ID: "agent-fail", Type: "agent", Status: "closed",
		Fields: map[string]string{
			"agent_state":  "failed",
			"close_reason": "timeout after 10m",
		},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := makeTrackingScheduler(srv)
	sched := Schedule{
		ID:          "sched-1",
		LastAgentID: "agent-fail",
		LastRun:     time.Now().Add(-10 * time.Minute),
	}

	s.trackLastAgentStatus(context.Background(), &sched)

	mock.mu.Lock()
	defer mock.mu.Unlock()

	found := false
	for _, c := range mock.comments {
		if c.beadID == "sched-1" && strings.Contains(c.text, "failed") && strings.Contains(c.text, "timeout") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected failure comment with reason, got: %v", mock.comments)
	}
}

func TestRecordRunCompletion_SkipsAlreadyChecked(t *testing.T) {
	mock := newTrackingMock()
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Fields: map[string]string{
			"last_agent_id":      "agent-1",
			"last_checked_agent": "agent-1", // Already checked.
		},
	}
	mock.beads["agent-1"] = &beadsapi.BeadDetail{
		ID: "agent-1", Type: "agent", Status: "closed",
		Fields: map[string]string{"agent_state": "done"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := makeTrackingScheduler(srv)
	sched := Schedule{
		ID:          "sched-1",
		LastAgentID: "agent-1",
		LastRun:     time.Now().Add(-5 * time.Minute),
	}

	s.trackLastAgentStatus(context.Background(), &sched)

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.comments) > 0 {
		t.Errorf("expected no comments for already-checked agent, got: %v", mock.comments)
	}
}

func TestRecordRunCompletion_SkipsRunningAgent(t *testing.T) {
	mock := newTrackingMock()
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Fields: map[string]string{"last_agent_id": "agent-running"},
	}
	mock.beads["agent-running"] = &beadsapi.BeadDetail{
		ID: "agent-running", Type: "agent", Status: "open",
		Fields: map[string]string{"agent_state": "working"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := makeTrackingScheduler(srv)
	sched := Schedule{
		ID:          "sched-1",
		LastAgentID: "agent-running",
	}

	s.trackLastAgentStatus(context.Background(), &sched)

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.comments) > 0 {
		t.Errorf("expected no comments for running agent, got: %v", mock.comments)
	}
}

func TestRecordRunCompletion_IncludesDuration(t *testing.T) {
	mock := newTrackingMock()
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Fields: map[string]string{"last_agent_id": "agent-1"},
	}
	mock.beads["agent-1"] = &beadsapi.BeadDetail{
		ID: "agent-1", Type: "agent", Status: "closed",
		Fields: map[string]string{"agent_state": "done"},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := makeTrackingScheduler(srv)
	sched := Schedule{
		ID:          "sched-1",
		LastAgentID: "agent-1",
		LastRun:     time.Now().Add(-4*time.Minute - 12*time.Second),
	}

	s.trackLastAgentStatus(context.Background(), &sched)

	mock.mu.Lock()
	defer mock.mu.Unlock()

	found := false
	for _, c := range mock.comments {
		// Should contain duration in brackets.
		if c.beadID == "sched-1" && strings.Contains(c.text, "[4m") {
			found = true
			break
		}
	}
	if !found {
		texts := make([]string, len(mock.comments))
		for i, c := range mock.comments {
			texts[i] = c.text
		}
		t.Errorf("expected comment with duration, got: %v", texts)
	}
}

func TestReconcile_RecordsSpawnComment(t *testing.T) {
	mock := newTrackingMock()
	mock.beads["sched-1"] = &beadsapi.BeadDetail{
		ID: "sched-1", Type: "schedule", Status: "open",
		Title: "Daily Task",
		Fields: map[string]string{
			"cron":    "* * * * *", // every minute
			"enabled": "true",
			"prompt":  "do something",
		},
	}
	srv := httptest.NewServer(mock.handler())
	defer srv.Close()

	s := makeTrackingScheduler(srv)
	err := s.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	found := false
	for _, c := range mock.comments {
		if c.beadID == "sched-1" && strings.Contains(c.text, "Run started") {
			found = true
			if c.author != "scheduler" {
				t.Errorf("expected author=scheduler, got %q", c.author)
			}
			break
		}
	}
	if !found {
		texts := make([]string, len(mock.comments))
		for i, c := range mock.comments {
			texts[i] = fmt.Sprintf("[%s] %s", c.beadID, c.text)
		}
		t.Errorf("expected spawn comment, got: %v", texts)
	}
}
