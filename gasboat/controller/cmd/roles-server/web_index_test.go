package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gasboat/controller/internal/beadsapi"
)

func TestIndexPage(t *testing.T) {
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
	handler := handleIndex(client, setupLogger("error"), tmpl)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Dashboard") {
		t.Error("expected page to contain 'Dashboard'")
	}
	if !strings.Contains(body, "Config Beads") {
		t.Error("expected page to contain 'Config Beads' card")
	}
	if !strings.Contains(body, "Advice Beads") {
		t.Error("expected page to contain 'Advice Beads' card")
	}
}

func TestIndexPageNavigation(t *testing.T) {
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
	handler := handleIndex(client, setupLogger("error"), tmpl)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "Roles Server") {
		t.Error("expected nav to contain 'Roles Server' brand")
	}
	if !strings.Contains(body, "/config-beads") {
		t.Error("expected nav to contain link to config-beads")
	}
	if !strings.Contains(body, "/advice") {
		t.Error("expected nav to contain link to advice")
	}
	if !strings.Contains(body, "/roles") {
		t.Error("expected nav to contain link to roles")
	}
}

func TestIndexPage404(t *testing.T) {
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
	handler := handleIndex(client, setupLogger("error"), tmpl)

	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown path, got %d", w.Code)
	}
}
