package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gasboat/controller/internal/beadsapi"
)

func TestRolesListPage(t *testing.T) {
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
	ui := NewRolesUI(client, setupLogger("error"), tmpl)

	req := httptest.NewRequest("GET", "/roles", nil)
	w := httptest.NewRecorder()
	ui.handleListRoles(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Roles") {
		t.Error("expected page to contain 'Roles' heading")
	}
	if !strings.Contains(body, "global") {
		t.Error("expected page to list 'global' role")
	}
}

func TestRolePreviewPage(t *testing.T) {
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
	ui := NewRolesUI(client, setupLogger("error"), tmpl)

	req := httptest.NewRequest("GET", "/roles/crew", nil)
	req.SetPathValue("role", "crew")
	w := httptest.NewRecorder()
	ui.handleRolePreview(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Role: crew") {
		t.Error("expected page to show role name")
	}
	if !strings.Contains(body, "Config Beads") {
		t.Error("expected page to contain Config Beads section")
	}
	if !strings.Contains(body, "Advice Beads") {
		t.Error("expected page to contain Advice Beads section")
	}
}

func TestRolePreviewGlobal(t *testing.T) {
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
	ui := NewRolesUI(client, setupLogger("error"), tmpl)

	req := httptest.NewRequest("GET", "/roles/global", nil)
	req.SetPathValue("role", "global")
	w := httptest.NewRecorder()
	ui.handleRolePreview(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Role: global") {
		t.Error("expected page to show 'Role: global'")
	}
}
