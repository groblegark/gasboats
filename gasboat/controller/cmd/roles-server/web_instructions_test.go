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

func mockDaemonWithInstructions(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/beads", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"beads": []any{}, "total": 0})
	})

	mux.HandleFunc("GET /v1/beads/{id}", func(w http.ResponseWriter, r *http.Request) {
		beadID := r.PathValue("id")
		w.Header().Set("Content-Type", "application/json")

		switch beadID {
		case "instr-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "instr-1",
				"title":  "claude-instructions",
				"type":   "config",
				"kind":   "config",
				"status": "open",
				"labels": []string{"role:crew"},
				"fields": map[string]any{
					"value": `{"identity":"You are a crew agent","lifecycle":"persistent","core_rules":"Use kd for CRUD"}`,
				},
			})
		case "not-instructions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":     "not-instructions",
				"title":  "claude-settings",
				"type":   "config",
				"kind":   "config",
				"status": "open",
				"labels": []string{"global"},
				"fields": map[string]any{
					"value": `{"model":"sonnet"}`,
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
		}
	})

	mux.HandleFunc("PATCH /v1/beads/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	return httptest.NewServer(mux)
}

func setupTestInstructionsUI(t *testing.T) (*InstructionsUI, *http.ServeMux) {
	t.Helper()
	daemon := mockDaemonWithInstructions(t)
	client, err := beadsapi.New(beadsapi.Config{HTTPAddr: daemon.URL})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	t.Cleanup(func() {
		client.Close()
		daemon.Close()
	})
	tmpl := newTemplateSet()
	ui := NewInstructionsUI(client, setupLogger("error"), tmpl)
	mux := http.NewServeMux()
	ui.RegisterRoutes(mux)
	return ui, mux
}

func TestEditInstructionsForm(t *testing.T) {
	_, mux := setupTestInstructionsUI(t)

	req := httptest.NewRequest("GET", "/config-beads/instr-1/instructions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Edit Claude Instructions") {
		t.Error("expected page to contain 'Edit Claude Instructions'")
	}
	if !strings.Contains(body, "You are a crew agent") {
		t.Error("expected form to contain identity value")
	}
	if !strings.Contains(body, "persistent") {
		t.Error("expected form to contain lifecycle value")
	}
	if !strings.Contains(body, "section_identity") {
		t.Error("expected form to contain section_identity field")
	}
}

func TestEditInstructionsNotFound(t *testing.T) {
	_, mux := setupTestInstructionsUI(t)

	req := httptest.NewRequest("GET", "/config-beads/nonexistent/instructions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestEditInstructionsWrongType(t *testing.T) {
	_, mux := setupTestInstructionsUI(t)

	req := httptest.NewRequest("GET", "/config-beads/not-instructions/instructions", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestUpdateInstructions(t *testing.T) {
	_, mux := setupTestInstructionsUI(t)

	form := url.Values{}
	form.Set("section_identity", "Updated identity")
	form.Set("section_lifecycle", "ephemeral")
	form.Set("section_core_rules", "New rules")

	req := httptest.NewRequest("POST", "/config-beads/instr-1/instructions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/config-beads" {
		t.Errorf("expected redirect to /config-beads, got %q", loc)
	}
}

func TestUpdateInstructionsEmptySections(t *testing.T) {
	_, mux := setupTestInstructionsUI(t)

	// Submit with all sections empty — should still succeed.
	form := url.Values{}
	req := httptest.NewRequest("POST", "/config-beads/instr-1/instructions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}
}
