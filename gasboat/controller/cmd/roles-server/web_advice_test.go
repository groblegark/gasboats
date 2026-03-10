package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gasboat/controller/internal/beadsapi"
)

func setupTestAdviceUI(t *testing.T) (*AdviceUI, *http.ServeMux) {
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
	ui := NewAdviceUI(client, setupLogger("error"), tmpl)
	mux := http.NewServeMux()
	ui.RegisterRoutes(mux)
	return ui, mux
}

func TestListAdvicePage(t *testing.T) {
	_, mux := setupTestAdviceUI(t)

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

func TestNewAdviceForm(t *testing.T) {
	_, mux := setupTestAdviceUI(t)

	req := httptest.NewRequest("GET", "/advice/new", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "New Advice") {
		t.Error("expected page to contain 'New Advice'")
	}
}

func TestCreateAdvice(t *testing.T) {
	_, mux := setupTestAdviceUI(t)

	form := url.Values{}
	form.Set("title", "Test advice")
	form.Set("labels", "global, role:crew")
	form.Set("description", "Some advice content")

	req := httptest.NewRequest("POST", "/advice/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/advice" {
		t.Errorf("expected redirect to /advice, got %q", loc)
	}
}

func TestCreateAdviceValidation(t *testing.T) {
	_, mux := setupTestAdviceUI(t)

	form := url.Values{}
	form.Set("title", "")
	form.Set("description", "Some content")

	req := httptest.NewRequest("POST", "/advice/new", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "title is required") {
		t.Error("expected validation error for empty title")
	}
}

func TestEditAdviceForm(t *testing.T) {
	_, mux := setupTestAdviceUI(t)

	req := httptest.NewRequest("GET", "/advice/adv-1/edit", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Edit Advice") {
		t.Error("expected page to contain 'Edit Advice'")
	}
}

func TestEditAdviceNotFound(t *testing.T) {
	_, mux := setupTestAdviceUI(t)

	req := httptest.NewRequest("GET", "/advice/nonexistent/edit", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestUpdateAdvice(t *testing.T) {
	_, mux := setupTestAdviceUI(t)

	form := url.Values{}
	form.Set("title", "Updated advice")
	form.Set("labels", "global")
	form.Set("description", "Updated content")

	req := httptest.NewRequest("POST", "/advice/adv-1/edit", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDeleteAdviceConfirm(t *testing.T) {
	_, mux := setupTestAdviceUI(t)

	req := httptest.NewRequest("GET", "/advice/adv-1/delete", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Delete Advice") {
		t.Error("expected page to contain 'Delete Advice'")
	}
	if !strings.Contains(body, "adv-1") {
		t.Error("expected page to show bead ID")
	}
}

func TestDeleteAdvice(t *testing.T) {
	_, mux := setupTestAdviceUI(t)

	req := httptest.NewRequest("POST", "/advice/adv-1/delete", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d: %s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/advice" {
		t.Errorf("expected redirect to /advice, got %q", loc)
	}
}

func TestDeleteAdviceNotFound(t *testing.T) {
	_, mux := setupTestAdviceUI(t)

	req := httptest.NewRequest("GET", "/advice/nonexistent/delete", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
