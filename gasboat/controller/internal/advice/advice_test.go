package advice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gasboat/controller/internal/beadsapi"
)

func TestListOpenAdvice_ExcludesClosed(t *testing.T) {
	// Simulate a server that returns both open and closed advice beads
	// (e.g., if the server-side status filter has a bug).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"beads": []map[string]any{
				{"id": "kd-open1", "title": "Open advice", "status": "open", "type": "advice", "kind": "data", "labels": []string{"global"}},
				{"id": "kd-closed1", "title": "Closed advice", "status": "closed", "type": "advice", "kind": "data", "labels": []string{"global"}},
				{"id": "kd-open2", "title": "Another open", "status": "open", "type": "advice", "kind": "data", "labels": []string{"global"}},
			},
			"total": 3,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client, err := beadsapi.New(beadsapi.Config{
		HTTPAddr: srv.URL,
	})
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	defer client.Close()

	beads, err := ListOpenAdvice(context.Background(), client)
	if err != nil {
		t.Fatalf("ListOpenAdvice: %v", err)
	}

	// Should only return the 2 open beads, filtering out the closed one.
	if len(beads) != 2 {
		t.Errorf("expected 2 open beads, got %d", len(beads))
	}
	for _, b := range beads {
		if b.Status == "closed" {
			t.Errorf("closed bead %s should not be returned by ListOpenAdvice", b.ID)
		}
	}
}

func TestListOpenAdvice_AllOpen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request includes status=open filter.
		status := r.URL.Query().Get("status")
		if status != "open" {
			t.Errorf("expected status=open query param, got %q", status)
		}

		resp := map[string]any{
			"beads": []map[string]any{
				{"id": "kd-1", "title": "Advice 1", "status": "open", "type": "advice", "kind": "data", "labels": []string{"global"}},
				{"id": "kd-2", "title": "Advice 2", "status": "open", "type": "advice", "kind": "data", "labels": []string{"role:crew"}},
			},
			"total": 2,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client, err := beadsapi.New(beadsapi.Config{
		HTTPAddr: srv.URL,
	})
	if err != nil {
		t.Fatalf("creating client: %v", err)
	}
	defer client.Close()

	beads, err := ListOpenAdvice(context.Background(), client)
	if err != nil {
		t.Fatalf("ListOpenAdvice: %v", err)
	}

	if len(beads) != 2 {
		t.Errorf("expected 2 beads, got %d", len(beads))
	}
}
