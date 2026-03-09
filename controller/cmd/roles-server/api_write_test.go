package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gasboat/controller/internal/beadsapi"
)

// mockWriteDaemon extends the read-only mock with write endpoints.
func mockWriteDaemon(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// Bead listing (same as read-only mock).
	mux.HandleFunc("GET /v1/beads", func(w http.ResponseWriter, r *http.Request) {
		beadType := r.URL.Query().Get("type")
		w.Header().Set("Content-Type", "application/json")
		switch beadType {
		case "config":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"beads": []map[string]any{
					{
						"id": "cfg-1", "title": "claude-settings", "type": "config",
						"kind": "config", "status": "open", "labels": []string{"global"},
						"fields": map[string]any{"value": `{"model":"sonnet"}`},
					},
				},
				"total": 1,
			})
		case "advice":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"beads": []map[string]any{
					{
						"id": "adv-1", "title": "Use gb prime", "type": "advice",
						"kind": "data", "status": "open", "labels": []string{"global"},
						"description": "Run gb prime after compaction",
					},
				},
				"total": 1,
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"beads": []any{}, "total": 0})
		}
	})

	// Create bead.
	mux.HandleFunc("POST /v1/beads", func(w http.ResponseWriter, r *http.Request) {
		var req beadsapi.CreateBeadRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "new-1"})
	})

	// Get single bead.
	mux.HandleFunc("GET /v1/beads/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		w.Header().Set("Content-Type", "application/json")
		switch id {
		case "cfg-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "cfg-1", "title": "claude-settings", "type": "config",
				"kind": "config", "status": "open", "labels": []string{"global"},
				"description": `{"model":"sonnet"}`,
				"fields":      map[string]any{"value": `{"model":"sonnet"}`},
			})
		case "adv-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "adv-1", "title": "Use gb prime", "type": "advice",
				"kind": "data", "status": "open", "labels": []string{"global"},
				"description": "Run gb prime after compaction",
			})
		case "new-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "new-1", "title": "new bead", "type": "config",
				"kind": "config", "status": "open", "labels": []string{},
				"fields": map[string]any{},
			})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	// Update bead (PATCH).
	mux.HandleFunc("PATCH /v1/beads/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Close bead.
	mux.HandleFunc("POST /v1/beads/{id}/close", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Add label.
	mux.HandleFunc("POST /v1/beads/{id}/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Remove label.
	mux.HandleFunc("DELETE /v1/beads/{id}/labels/{label}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Update fields (PUT).
	mux.HandleFunc("PUT /v1/beads/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	return httptest.NewServer(mux)
}

func setupWriteAPI(t *testing.T) (*RolesAPI, *http.ServeMux) {
	t.Helper()
	daemon := mockWriteDaemon(t)
	client, err := beadsapi.New(beadsapi.Config{HTTPAddr: daemon.URL})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	t.Cleanup(func() {
		client.Close()
		daemon.Close()
	})
	api := NewRolesAPI(client, setupLogger("error"))
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	return api, mux
}

func TestGetConfigBead(t *testing.T) {
	_, mux := setupWriteAPI(t)

	req := httptest.NewRequest("GET", "/api/config-beads/cfg-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp configBead
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != "cfg-1" {
		t.Errorf("expected id cfg-1, got %q", resp.ID)
	}
}

func TestCreateConfigBead(t *testing.T) {
	_, mux := setupWriteAPI(t)

	body := `{"title":"claude-hooks","labels":["global"],"value":"{\"hooks\":[]}"}`
	req := httptest.NewRequest("POST", "/api/config-beads", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["id"] == "" {
		t.Error("expected non-empty id in response")
	}
}

func TestCreateConfigBeadMissingTitle(t *testing.T) {
	_, mux := setupWriteAPI(t)

	body := `{"labels":["global"]}`
	req := httptest.NewRequest("POST", "/api/config-beads", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateConfigBead(t *testing.T) {
	_, mux := setupWriteAPI(t)

	body := `{"title":"claude-settings-v2","value":"{\"model\":\"opus\"}"}`
	req := httptest.NewRequest("PUT", "/api/config-beads/cfg-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteConfigBead(t *testing.T) {
	_, mux := setupWriteAPI(t)

	req := httptest.NewRequest("DELETE", "/api/config-beads/cfg-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetAdviceBead(t *testing.T) {
	_, mux := setupWriteAPI(t)

	req := httptest.NewRequest("GET", "/api/advice/adv-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp adviceBead
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ID != "adv-1" {
		t.Errorf("expected id adv-1, got %q", resp.ID)
	}
}

func TestCreateAdviceBead(t *testing.T) {
	_, mux := setupWriteAPI(t)

	body := `{"title":"New advice","description":"Some advice content","labels":["global"]}`
	req := httptest.NewRequest("POST", "/api/advice", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateAdviceBead(t *testing.T) {
	_, mux := setupWriteAPI(t)

	title := "Updated advice"
	body := `{"title":"` + title + `","description":"Updated content"}`
	req := httptest.NewRequest("PUT", "/api/advice/adv-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteAdviceBead(t *testing.T) {
	_, mux := setupWriteAPI(t)

	req := httptest.NewRequest("DELETE", "/api/advice/adv-1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUpdateConfigBeadLabels(t *testing.T) {
	_, mux := setupWriteAPI(t)

	// Change labels from ["global"] to ["role:crew"]
	body := `{"labels":["role:crew"]}`
	req := httptest.NewRequest("PUT", "/api/config-beads/cfg-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateConfigBeadInvalidJSON(t *testing.T) {
	_, mux := setupWriteAPI(t)

	req := httptest.NewRequest("POST", "/api/config-beads", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
