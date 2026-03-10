package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gasboat/controller/internal/beadsapi"
)

func TestFormatDuration(t *testing.T) {
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
		if got := formatDuration(tc.d); got != tc.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

func TestNotifyAgentSpawn_ScheduledAgent_PostsScheduleNotification(t *testing.T) {
	daemon := newMockDaemon()
	daemon.beads["sched-agent-1"] = &beadsapi.BeadDetail{
		ID: "sched-agent-1", Type: "agent", Status: "open",
		Title: "sched-1234", Fields: map[string]string{
			"agent":          "sched-1234",
			"schedule_id":    "kd-sched-1",
			"schedule_title": "Daily Release Notes",
			"schedule_cron":  "0 9 * * 1-5",
		},
	}

	var mu sync.Mutex
	var postedTexts []string
	slackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat.postMessage" {
			_ = r.ParseForm()
			mu.Lock()
			postedTexts = append(postedTexts, r.FormValue("text"))
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": "C123", "ts": "1234.5678"})
	}))
	defer slackSrv.Close()

	bot := newTestBot(daemon, slackSrv)
	bot.channel = "C123"

	bot.NotifyAgentSpawn(context.Background(), BeadEvent{
		ID:       "sched-agent-1",
		Type:     "agent",
		Title:    "sched-1234",
		Assignee: "sched-1234",
		Fields: map[string]string{
			"agent":          "sched-1234",
			"schedule_id":    "kd-sched-1",
			"schedule_title": "Daily Release Notes",
			"schedule_cron":  "0 9 * * 1-5",
		},
	})

	// Verify schedule title is cached.
	if got := bot.agentScheduleTitle["sched-1234"]; got != "Daily Release Notes" {
		t.Errorf("expected schedule title cached, got %q", got)
	}

	// Verify spawn time is recorded.
	if bot.agentSpawnedAt["sched-1234"].IsZero() {
		t.Error("expected agentSpawnedAt to be set")
	}

	mu.Lock()
	defer mu.Unlock()

	// Should post a schedule-specific message.
	found := false
	for _, text := range postedTexts {
		if strings.Contains(text, "Scheduled task") && strings.Contains(text, "Daily Release Notes") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected schedule notification, got texts: %v", postedTexts)
	}
}

func TestNotifyAgentSpawn_NonScheduled_NoScheduleFields(t *testing.T) {
	daemon := newMockDaemon()
	daemon.beads["normal-agent"] = &beadsapi.BeadDetail{
		ID: "normal-agent", Type: "agent", Status: "open",
		Title: "my-agent", Fields: map[string]string{"agent": "my-agent"},
	}

	slackSrv := newFakeSlackServer(t)
	defer slackSrv.Close()

	bot := newTestBot(daemon, slackSrv)
	bot.channel = "C123"

	bot.NotifyAgentSpawn(context.Background(), BeadEvent{
		ID:       "normal-agent",
		Type:     "agent",
		Title:    "my-agent",
		Assignee: "my-agent",
		Fields:   map[string]string{"agent": "my-agent"},
	})

	// No schedule title should be cached for a non-scheduled agent.
	if got := bot.agentScheduleTitle["my-agent"]; got != "" {
		t.Errorf("expected no schedule title for normal agent, got %q", got)
	}
}

func TestNotifyAgentState_ScheduledDone_PostsCompletionNotification(t *testing.T) {
	daemon := newMockDaemon()

	var mu sync.Mutex
	var postedTexts []string
	slackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat.postMessage" {
			_ = r.ParseForm()
			mu.Lock()
			postedTexts = append(postedTexts, r.FormValue("text"))
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": "C123", "ts": "5678.9012"})
	}))
	defer slackSrv.Close()

	bot := newTestBot(daemon, slackSrv)
	bot.channel = "C123"
	bot.agentState["sched-done"] = "working"
	bot.agentScheduleTitle["sched-done"] = "Daily Release Notes"
	bot.agentSpawnedAt["sched-done"] = time.Now().Add(-4*time.Minute - 12*time.Second)

	bot.NotifyAgentState(context.Background(), BeadEvent{
		Assignee: "sched-done",
		Fields:   map[string]string{"agent_state": "done", "agent": "sched-done"},
	})

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, text := range postedTexts {
		if strings.Contains(text, "Scheduled task") && strings.Contains(text, "completed") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected schedule completion notification, got texts: %v", postedTexts)
	}
}

func TestNotifyAgentState_ScheduledFailed_PostsFailureNotification(t *testing.T) {
	daemon := newMockDaemon()

	var mu sync.Mutex
	var postedTexts []string
	slackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat.postMessage" {
			_ = r.ParseForm()
			mu.Lock()
			postedTexts = append(postedTexts, r.FormValue("text"))
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": "C123", "ts": "5678.9012"})
	}))
	defer slackSrv.Close()

	bot := newTestBot(daemon, slackSrv)
	bot.channel = "C123"
	bot.agentState["sched-fail"] = "working"
	bot.agentScheduleTitle["sched-fail"] = "Nightly Backup"
	bot.agentSpawnedAt["sched-fail"] = time.Now().Add(-10 * time.Minute)

	bot.NotifyAgentState(context.Background(), BeadEvent{
		Assignee: "sched-fail",
		Fields: map[string]string{
			"agent_state":  "failed",
			"agent":        "sched-fail",
			"close_reason": "timeout after 10m",
		},
	})

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, text := range postedTexts {
		if strings.Contains(text, "Scheduled task") && strings.Contains(text, "failed") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected schedule failure notification, got texts: %v", postedTexts)
	}
}

func TestNotifyAgentState_NonScheduled_NoScheduleNotification(t *testing.T) {
	daemon := newMockDaemon()

	var mu sync.Mutex
	var postedTexts []string
	slackSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat.postMessage" {
			_ = r.ParseForm()
			mu.Lock()
			postedTexts = append(postedTexts, r.FormValue("text"))
			mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "channel": "C123", "ts": "5678.9012"})
	}))
	defer slackSrv.Close()

	bot := newTestBot(daemon, slackSrv)
	bot.channel = "C123"
	bot.agentState["normal-agent"] = "working"
	// No agentScheduleTitle set — this is a non-scheduled agent.

	bot.NotifyAgentState(context.Background(), BeadEvent{
		Assignee: "normal-agent",
		Fields:   map[string]string{"agent_state": "done", "agent": "normal-agent"},
	})

	mu.Lock()
	defer mu.Unlock()

	for _, text := range postedTexts {
		if strings.Contains(text, "Scheduled task") {
			t.Errorf("unexpected schedule notification for non-scheduled agent: %q", text)
		}
	}
}

func TestSpawnAgent_ExtraFields_MergedIntoMock(t *testing.T) {
	daemon := newMockDaemon()
	extra := map[string]string{
		"schedule_id":    "kd-sched-1",
		"schedule_title": "Daily Cleanup",
	}
	id, err := daemon.SpawnAgent(context.Background(), "sched-bot", "gasboat", "", "crew", "", extra)
	if err != nil {
		t.Fatalf("SpawnAgent: %v", err)
	}

	bead := daemon.beads[id]
	if bead == nil {
		t.Fatal("expected bead to be created")
	}
	if got := bead.Fields["schedule_id"]; got != "kd-sched-1" {
		t.Errorf("expected schedule_id=kd-sched-1, got %q", got)
	}
	if got := bead.Fields["schedule_title"]; got != "Daily Cleanup" {
		t.Errorf("expected schedule_title=Daily Cleanup, got %q", got)
	}
	// Standard fields should still be present.
	if got := bead.Fields["agent"]; got != "sched-bot" {
		t.Errorf("expected agent=sched-bot, got %q", got)
	}
}
