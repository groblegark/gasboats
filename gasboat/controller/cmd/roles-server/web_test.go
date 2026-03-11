package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gasboat/controller/internal/beadsapi"
)

func setupTestWebUI(t *testing.T) (*WebUI, *http.ServeMux, *httptest.Server) {
	t.Helper()
	daemon := mockDaemonWithMutations(t)
	client, err := beadsapi.New(beadsapi.Config{HTTPAddr: daemon.URL})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	t.Cleanup(func() {
		client.Close()
		daemon.Close()
	})
	tmpl := newTemplateSet()
	ui := NewWebUI(client, setupLogger("error"), tmpl)
	mux := http.NewServeMux()
	ui.RegisterRoutes(mux)
	return ui, mux, daemon
}

// mockDaemonWithMutations extends the mock daemon with POST/PATCH/DELETE handlers.
func mockDaemonWithMutations(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	// Bead listing (reuse from existing mock).
	mux.HandleFunc("GET /v1/beads", func(w http.ResponseWriter, r *http.Request) {
		beadType := r.URL.Query().Get("type")
		w.Header().Set("Content-Type", "application/json")

		switch beadType {
		case "config":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"beads": []map[string]any{
					{
						"id":     "cfg-1",
						"title":  "claude-settings",
						"type":   "config",
						"kind":   "config",
						"status": "open",
						"labels": []string{"global"},
						"fields": map[string]any{
							"value": `{"model":"sonnet"}`,
						},
					},
					{
						"id":     "cfg-2",
						"title":  "claude-instructions",
						"type":   "config",
						"kind":   "config",
						"status": "open",
						"labels": []string{"role:crew"},
						"fields": map[string]any{
							"value": `{"lifecycle":"persistent"}`,
						},
					},
				},
				"total": 2,
			})
		case "advice":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"beads": []map[string]any{
					{
						"id":          "adv-1",
						"title":       "Use gb prime",
						"type":        "advice",
						"kind":        "data",
						"status":      "open",
						"labels":      []string{"global"},
						"description": "Run gb prime after compaction",
					},
				},
				"total": 1,
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"beads": []any{}, "total": 0})
		}
	})

	// Get single bead.
	mux.HandleFunc("GET /v1/beads/{id}", func(w http.ResponseWriter, r *http.Request) {
		beadID := r.PathValue("id")
		w.Header().Set("Content-Type", "application/json")

		switch beadID {
		case "cfg-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "cfg-1",
				"title":  "claude-settings",
				"type":   "config",
				"kind":   "config",
				"status": "open",
				"labels": []string{"global"},
				"fields": map[string]any{
					"value": `{"model":"sonnet"}`,
				},
			})
		case "adv-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":          "adv-1",
				"title":       "Use gb prime",
				"type":        "advice",
				"kind":        "data",
				"status":      "open",
				"labels":      []string{"global"},
				"description": "Run gb prime after compaction",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		}
	})

	// Create bead.
	mux.HandleFunc("POST /v1/beads", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "cfg-new"})
	})

	// Update bead (PATCH).
	mux.HandleFunc("PATCH /v1/beads/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Update fields.
	mux.HandleFunc("PUT /v1/beads/{id}/fields", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Delete bead.
	mux.HandleFunc("DELETE /v1/beads/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// Labels.
	mux.HandleFunc("POST /v1/beads/{id}/labels", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})
	mux.HandleFunc("DELETE /v1/beads/{id}/labels/{label}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	return httptest.NewServer(mux)
}


func TestListConfigBeadsPage(t *testing.T) {
	_, mux, _ := setupTestWebUI(t)

	req := httptest.NewRequest("GET", "/config-beads", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Config Beads") {
		t.Error("expected page to contain 'Config Beads' heading")
	}
	if !strings.Contains(body, "cfg-1") {
		t.Error("expected page to contain bead ID cfg-1")
	}
	if !strings.Contains(body, "claude-settings") {
		t.Error("expected page to contain category 'claude-settings'")
	}
}

func TestNewConfigBeadForm(t *testing.T) {
	_, mux, _ := setupTestWebUI(t)

	req := httptest.NewRequest("GET", "/config-beads/new", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "New Config Bead") {
		t.Error("expected page to contain 'New Config Bead'")
	}
	if !strings.Contains(body, "claude-settings") {
		t.Error("expected category dropdown to contain 'claude-settings'")
	}
}

func TestCreateConfigBead(t *testing.T) {
	_, mux, _ := setupTestWebUI(t)

	form := url.Values{}
	form.Set("title", "claude-settings")
	form.Set("labels", "global, role:crew")
	form.Set("value", `{"model":"opus"}`)

	req := httptest.NewRequest("POST", "/config-beads/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/config-beads" {
		t.Errorf("expected redirect to /config-beads, got %q", loc)
	}
}

func TestCreateConfigBeadValidation(t *testing.T) {
	_, mux, _ := setupTestWebUI(t)

	tests := []struct {
		name      string
		title     string
		value     string
		wantError string
	}{
		{"empty title", "", `{}`, "title (category) is required"},
		{"invalid JSON", "claude-settings", `{bad`, "value must be valid JSON"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := url.Values{}
			form.Set("title", tt.title)
			form.Set("value", tt.value)

			req := httptest.NewRequest("POST", "/config-beads/new", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, req)

			if w.Code != http.StatusUnprocessableEntity {
				t.Fatalf("expected 422, got %d", w.Code)
			}
			body := w.Body.String()
			if !strings.Contains(body, tt.wantError) {
				t.Errorf("expected error %q in body, got: %s", tt.wantError, body)
			}
		})
	}
}

func TestEditConfigBeadForm(t *testing.T) {
	_, mux, _ := setupTestWebUI(t)

	req := httptest.NewRequest("GET", "/config-beads/cfg-1/edit", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Edit Config Bead") {
		t.Error("expected page to contain 'Edit Config Bead'")
	}
	if !strings.Contains(body, "claude-settings") {
		t.Error("expected form to have category pre-selected")
	}
}

func TestEditConfigBeadNotFound(t *testing.T) {
	_, mux, _ := setupTestWebUI(t)

	req := httptest.NewRequest("GET", "/config-beads/nonexistent/edit", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUpdateConfigBead(t *testing.T) {
	_, mux, _ := setupTestWebUI(t)

	form := url.Values{}
	form.Set("title", "claude-settings")
	form.Set("labels", "global")
	form.Set("value", `{"model":"opus"}`)

	req := httptest.NewRequest("POST", "/config-beads/cfg-1/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteConfirm(t *testing.T) {
	_, mux, _ := setupTestWebUI(t)

	req := httptest.NewRequest("GET", "/config-beads/cfg-1/delete", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Delete Config Bead") {
		t.Error("expected page to contain 'Delete Config Bead'")
	}
	if !strings.Contains(body, "cfg-1") {
		t.Error("expected page to show bead ID")
	}
}

func TestDeleteConfigBead(t *testing.T) {
	_, mux, _ := setupTestWebUI(t)

	req := httptest.NewRequest("POST", "/config-beads/cfg-1/delete", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/config-beads" {
		t.Errorf("expected redirect to /config-beads, got %q", loc)
	}
}

func TestDeleteConfirmNotFound(t *testing.T) {
	_, mux, _ := setupTestWebUI(t)

	req := httptest.NewRequest("GET", "/config-beads/nonexistent/delete", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

