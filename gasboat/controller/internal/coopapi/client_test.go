package coopapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetAgentState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent" || r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(AgentState{
			State:         "idle",
			ErrorCategory: "",
			LastMessage:   "hello",
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	state, err := c.GetAgentState(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.State != "idle" {
		t.Errorf("expected state=idle, got %q", state.State)
	}
	if state.LastMessage != "hello" {
		t.Errorf("expected last_message=hello, got %q", state.LastMessage)
	}
}

func TestGetAgentState_WithPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"state": "starting",
			"prompt": map[string]string{
				"type":    "setup",
				"subtype": "project",
			},
		})
	}))
	defer srv.Close()

	c := New(srv.URL)
	state, err := c.GetAgentState(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.Prompt == nil {
		t.Fatal("expected prompt to be non-nil")
	}
	if state.Prompt.Type != "setup" {
		t.Errorf("expected prompt type=setup, got %q", state.Prompt.Type)
	}
}

func TestSendKeys(t *testing.T) {
	var received struct {
		Keys []string `json:"keys"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/input/keys" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.SendKeys(context.Background(), []string{"Escape", "Return"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(received.Keys) != 2 || received.Keys[0] != "Escape" || received.Keys[1] != "Return" {
		t.Errorf("unexpected keys sent: %v", received.Keys)
	}
}

func TestRespond(t *testing.T) {
	var received struct {
		Option int `json:"option"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/respond" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.Respond(context.Background(), 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if received.Option != 2 {
		t.Errorf("expected option=2, got %d", received.Option)
	}
}

func TestNudge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/nudge" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		var req map[string]string
		json.NewDecoder(r.Body).Decode(&req)
		if req["message"] != "do work" {
			t.Errorf("expected message='do work', got %q", req["message"])
		}
		json.NewEncoder(w).Encode(NudgeResponse{Delivered: true})
	}))
	defer srv.Close()

	c := New(srv.URL)
	resp, err := c.Nudge(context.Background(), "do work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Delivered {
		t.Error("expected delivered=true")
	}
}

func TestShutdown(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/shutdown" || r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := New(srv.URL)
	err := c.Shutdown(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("expected shutdown endpoint to be called")
	}
}

func TestGetScreenText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/screen/text" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Write([]byte("Resume Session\n> Latest"))
	}))
	defer srv.Close()

	c := New(srv.URL)
	text, err := c.GetScreenText(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "Resume Session\n> Latest" {
		t.Errorf("unexpected text: %q", text)
	}
}

func TestGetAgentState_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := New(srv.URL)
	_, err := c.GetAgentState(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestGetAgentState_ConnectionRefused(t *testing.T) {
	c := New("http://localhost:1") // nothing listening
	_, err := c.GetAgentState(context.Background())
	if err == nil {
		t.Fatal("expected connection error")
	}
}
