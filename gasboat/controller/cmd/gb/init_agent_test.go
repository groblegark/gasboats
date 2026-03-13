package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gasboat/controller/internal/coopapi"
)

func TestBypassStartup_IdleAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(coopapi.AgentState{State: "idle"})
	}))
	defer srv.Close()

	coop := coopapi.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := bypassStartup(ctx, coop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBypassStartup_WorkingAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(coopapi.AgentState{State: "working"})
	}))
	defer srv.Close()

	coop := coopapi.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := bypassStartup(ctx, coop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBypassStartup_SetupPromptWithExit(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.URL.Path {
		case "/api/v1/agent":
			if callCount <= 3 {
				json.NewEncoder(w).Encode(map[string]interface{}{
					"state": "starting",
					"prompt": map[string]string{
						"type": "setup",
					},
				})
			} else {
				json.NewEncoder(w).Encode(coopapi.AgentState{State: "idle"})
			}
		case "/api/v1/screen":
			w.Write([]byte("Please select:\n1. Yes\n2. No, exit"))
		case "/api/v1/agent/respond":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	coop := coopapi.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := bypassStartup(ctx, coop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBypassStartup_Cancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(coopapi.AgentState{State: "starting"})
	}))
	defer srv.Close()

	coop := coopapi.New(srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled

	err := bypassStartup(ctx, coop)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestWaitForCoop_Ready(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(coopapi.AgentState{State: "idle"})
	}))
	defer srv.Close()

	initCoopURL = srv.URL
	coop := coopapi.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := waitForCoop(ctx, coop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestContainsCI(t *testing.T) {
	tests := []struct {
		s, substr string
		want      bool
	}{
		{"Resume Session", "resume", true},
		{"RESUME SESSION", "session", true},
		{"hello world", "Resume", false},
		{"Resume a session", "resume", true},
	}
	for _, tt := range tests {
		got := containsCI(tt.s, tt.substr)
		if got != tt.want {
			t.Errorf("containsCI(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
		}
	}
}

func TestEnvDuration_Default(t *testing.T) {
	d := envDuration("NONEXISTENT_GB_TEST_VAR_12345", 42*time.Second)
	if d != 42*time.Second {
		t.Errorf("expected 42s, got %v", d)
	}
}

func TestEnvDuration_Set(t *testing.T) {
	t.Setenv("TEST_GB_INIT_DUR", "10")
	d := envDuration("TEST_GB_INIT_DUR", 42*time.Second)
	if d != 10*time.Second {
		t.Errorf("expected 10s, got %v", d)
	}
}

func TestInjectPrompt_WorkingAgent_StillDelivers(t *testing.T) {
	// Regression test: injectPrompt must NOT skip when the agent is "working".
	// Coop accepts nudges during working state — the agent picks up the
	// message when the current generation finishes.
	nudgeDelivered := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent":
			json.NewEncoder(w).Encode(coopapi.AgentState{State: "working"})
		case "/api/v1/agent/nudge":
			nudgeDelivered = true
			json.NewEncoder(w).Encode(coopapi.NudgeResponse{Delivered: true})
		}
	}))
	defer srv.Close()

	coop := coopapi.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := injectPrompt(ctx, coop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !nudgeDelivered {
		t.Fatal("nudge was not delivered — injectPrompt should not skip when agent is working")
	}
}

func TestInjectPrompt_IdleAgent_Delivers(t *testing.T) {
	nudgeDelivered := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent":
			json.NewEncoder(w).Encode(coopapi.AgentState{State: "idle"})
		case "/api/v1/agent/nudge":
			nudgeDelivered = true
			json.NewEncoder(w).Encode(coopapi.NudgeResponse{Delivered: true})
		}
	}))
	defer srv.Close()

	coop := coopapi.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := injectPrompt(ctx, coop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !nudgeDelivered {
		t.Fatal("nudge was not delivered to idle agent")
	}
}

func TestInjectPrompt_RetryOnFailure(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/agent":
			json.NewEncoder(w).Encode(coopapi.AgentState{State: "idle"})
		case "/api/v1/agent/nudge":
			attempts++
			if attempts < 3 {
				json.NewEncoder(w).Encode(coopapi.NudgeResponse{Delivered: false, Reason: "agent busy"})
			} else {
				json.NewEncoder(w).Encode(coopapi.NudgeResponse{Delivered: true})
			}
		}
	}))
	defer srv.Close()

	coop := coopapi.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := injectPrompt(ctx, coop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts < 3 {
		t.Fatalf("expected at least 3 nudge attempts, got %d", attempts)
	}
}

func TestMonitorExit_AgentExited(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.URL.Path {
		case "/api/v1/agent":
			if callCount <= 2 {
				json.NewEncoder(w).Encode(coopapi.AgentState{State: "working"})
			} else {
				json.NewEncoder(w).Encode(coopapi.AgentState{State: "exited"})
			}
		case "/api/v1/shutdown":
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	coop := coopapi.New(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Reduce sleeps for test speed — monitorExit has 10s initial sleep + 5s poll.
	// We test the logic, not the timing, by using a context that'll be faster.
	err := monitorExit(ctx, coop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
