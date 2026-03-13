package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestJiraADFToText(t *testing.T) {
	tests := []struct {
		name string
		adf  string
		want string
	}{
		{
			name: "simple paragraph",
			adf:  `{"version":1,"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello world"}]}]}`,
			want: "Hello world",
		},
		{
			name: "bold text",
			adf:  `{"version":1,"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"bold","marks":[{"type":"strong"}]}]}]}`,
			want: "**bold**",
		},
		{
			name: "heading",
			adf:  `{"version":1,"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Title"}]}]}`,
			want: "## Title",
		},
		{
			name: "bullet list",
			adf:  `{"version":1,"type":"doc","content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"item one"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"item two"}]}]}]}]}`,
			want: "- item one\n- item two",
		},
		{
			name: "empty",
			adf:  ``,
			want: "",
		},
		{
			name: "invalid json",
			adf:  `{not valid}`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jiraADFToText(json.RawMessage(tt.adf))
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatJiraTime(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"2026-03-13T02:33:54.000+0000", "2026-03-13 02:33"},
		{"invalid", "invalid"},
		{"", ""},
	}
	for _, tt := range tests {
		got := formatJiraTime(tt.input)
		if got != tt.want {
			t.Errorf("formatJiraTime(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestNameOrEmpty(t *testing.T) {
	if got := nameOrEmpty(nil); got != "(none)" {
		t.Errorf("got %q, want (none)", got)
	}
	ref := &jiraNamedRef{Name: "Bug"}
	if got := nameOrEmpty(ref); got != "Bug" {
		t.Errorf("got %q, want Bug", got)
	}
}

func TestUserOrEmpty(t *testing.T) {
	if got := userOrEmpty(nil); got != "(unassigned)" {
		t.Errorf("got %q, want (unassigned)", got)
	}
	ref := &jiraUserRef{DisplayName: "Alice"}
	if got := userOrEmpty(ref); got != "Alice" {
		t.Errorf("got %q, want Alice", got)
	}
}

func TestRunJiraShow_MissingEnvVars(t *testing.T) {
	// Clear JIRA env vars.
	os.Unsetenv("JIRA_BASE_URL")
	os.Unsetenv("JIRA_EMAIL")
	os.Unsetenv("JIRA_API_TOKEN")

	err := runJiraShow(jiraShowCmd, []string{"PE-1234"})
	if err == nil {
		t.Fatal("expected error when env vars are missing")
	}
	if !strings.Contains(err.Error(), "JIRA_BASE_URL") {
		t.Errorf("error should mention JIRA_BASE_URL, got: %v", err)
	}
}

func TestRunJiraShow_Integration(t *testing.T) {
	// Mock JIRA API server.
	issueResp := map[string]any{
		"key": "PE-1234",
		"id":  "12345",
		"fields": map[string]any{
			"summary": "Test bug",
			"status":  map[string]string{"name": "To Do"},
			"issuetype": map[string]string{"name": "Bug"},
			"priority":  map[string]string{"name": "High"},
			"labels":    []string{"frontend"},
			"created":   "2026-03-13T02:33:54.000+0000",
			"updated":   "2026-03-13T10:00:00.000+0000",
			"description": map[string]any{
				"version": 1,
				"type":    "doc",
				"content": []any{
					map[string]any{
						"type": "paragraph",
						"content": []any{
							map[string]any{"type": "text", "text": "This is the description"},
						},
					},
				},
			},
		},
	}

	commentsResp := map[string]any{
		"comments": []any{
			map[string]any{
				"author":  map[string]string{"displayName": "Alice"},
				"created": "2026-03-13T03:00:00.000+0000",
				"body": map[string]any{
					"version": 1,
					"type":    "doc",
					"content": []any{
						map[string]any{
							"type": "paragraph",
							"content": []any{
								map[string]any{"type": "text", "text": "A comment"},
							},
						},
					},
				},
			},
		},
		"total": 1,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/comment") {
			json.NewEncoder(w).Encode(commentsResp)
		} else {
			json.NewEncoder(w).Encode(issueResp)
		}
	}))
	defer srv.Close()

	t.Setenv("JIRA_BASE_URL", srv.URL)
	t.Setenv("JIRA_EMAIL", "test@example.com")
	t.Setenv("JIRA_API_TOKEN", "fake-token")

	err := runJiraShow(jiraShowCmd, []string{"PE-1234"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestJiraHTTPClient_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"errorMessages":["Issue does not exist"]}`))
	}))
	defer srv.Close()

	client := &jiraHTTPClient{
		baseURL:    srv.URL,
		authHeader: "Basic dGVzdDp0ZXN0",
		http:       &http.Client{},
	}

	_, err := client.getIssue(t.Context(), "INVALID-999")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404, got: %v", err)
	}
}
