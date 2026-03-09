package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func setupTestWebUI(t *testing.T) (*WebUI, *http.ServeMux) {
	t.Helper()
	api, _ := setupTestAPI(t)
	webUI := NewWebUI(api)
	mux := http.NewServeMux()
	api.RegisterRoutes(mux)
	webUI.RegisterRoutes(mux)
	return webUI, mux
}

func TestWebIndex(t *testing.T) {
	_, mux := setupTestWebUI(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Roles") {
		t.Error("expected page to contain 'Roles'")
	}
	if !strings.Contains(body, "global") {
		t.Error("expected page to contain 'global' role")
	}
	if !strings.Contains(body, "crew") {
		t.Error("expected page to contain 'crew' role")
	}
}

func TestWebRole(t *testing.T) {
	_, mux := setupTestWebUI(t)

	req := httptest.NewRequest("GET", "/roles/crew", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "crew") {
		t.Error("expected page to contain 'crew'")
	}
}

func TestWebConfigBeads(t *testing.T) {
	_, mux := setupTestWebUI(t)

	req := httptest.NewRequest("GET", "/config-beads", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Config Beads") {
		t.Error("expected page to contain 'Config Beads'")
	}
}

func TestWebAdvice(t *testing.T) {
	_, mux := setupTestWebUI(t)

	req := httptest.NewRequest("GET", "/advice", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Advice Beads") {
		t.Error("expected page to contain 'Advice Beads'")
	}
}

func TestWebProjects(t *testing.T) {
	_, mux := setupTestWebUI(t)

	req := httptest.NewRequest("GET", "/projects", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Projects") {
		t.Error("expected page to contain 'Projects'")
	}
}

func TestWebContentType(t *testing.T) {
	_, mux := setupTestWebUI(t)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("expected text/html content type, got %q", ct)
	}
}
