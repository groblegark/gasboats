package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetAgentState_NullStateNoPanic(t *testing.T) {
	// Regression test: coop API returning {"state": null} must not cause
	// a panic when callers use bare type assertion state["state"].(string).
	// getAgentState must normalize null → "" so all call sites are safe.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"state": nil})
	}))
	defer srv.Close()

	client := &http.Client{}
	state, err := getAgentState(client, srv.URL+"/api/v1")
	if err != nil {
		t.Fatalf("getAgentState returned error: %v", err)
	}

	// Must be a string (not nil) so .(string) assertions don't panic.
	val, ok := state["state"].(string)
	if !ok {
		t.Fatalf("state[\"state\"] is %T, want string", state["state"])
	}
	if val != "" {
		t.Errorf("state[\"state\"] = %q, want empty string", val)
	}
}

func TestGetAgentState_MissingStateKey(t *testing.T) {
	// When JSON has no "state" key, getAgentState defaults to "".
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"version": "1.0"})
	}))
	defer srv.Close()

	client := &http.Client{}
	state, err := getAgentState(client, srv.URL+"/api/v1")
	if err != nil {
		t.Fatalf("getAgentState error: %v", err)
	}

	val, ok := state["state"].(string)
	if !ok {
		t.Errorf("state[\"state\"] should be string, got %T", state["state"])
	}
	if val != "" {
		t.Errorf("state[\"state\"] = %q, want empty string", val)
	}
}

func TestGetAgentState_ValidState(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"state": "idle"})
	}))
	defer srv.Close()

	client := &http.Client{}
	state, err := getAgentState(client, srv.URL+"/api/v1")
	if err != nil {
		t.Fatalf("getAgentState error: %v", err)
	}

	val, ok := state["state"].(string)
	if !ok {
		t.Fatalf("state[\"state\"] is %T, want string", state["state"])
	}
	if val != "idle" {
		t.Errorf("state[\"state\"] = %q, want \"idle\"", val)
	}
}
