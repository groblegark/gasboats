package bridge

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestGitLabClient creates a GitLabClient pointing at a test server.
func newTestGitLabClient(url string) *GitLabClient {
	return NewGitLabClient(GitLabClientConfig{
		BaseURL: url, Token: "test-token", Logger: slog.Default(),
	})
}

func TestGitLabClient_GetMergeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/merge_requests/211" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("PRIVATE-TOKEN") != "test-token" {
			t.Error("missing or wrong PRIVATE-TOKEN header")
		}
		resp := GitLabMR{
			ID:           1001,
			IID:          211,
			Title:        "Fix auth bug",
			State:        "merged",
			ProjectID:    42,
			SourceBranch: "fix/auth",
			TargetBranch: "main",
			WebURL:       "https://gitlab.com/org/repo/-/merge_requests/211",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestGitLabClient(server.URL)
	mr, err := client.GetMergeRequest(context.Background(), 42, 211)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mr.IID != 211 {
		t.Errorf("IID = %d, want 211", mr.IID)
	}
	if mr.State != "merged" {
		t.Errorf("State = %q, want merged", mr.State)
	}
	if mr.Title != "Fix auth bug" {
		t.Errorf("Title = %q, want Fix auth bug", mr.Title)
	}
}

func TestGitLabClient_GetMergeRequestByPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// httptest receives the raw (un-decoded) request; Go's net/http
		// decodes %2F to / in r.URL.Path. Use RawPath to check encoding.
		wantRaw := "/api/v4/projects/PiHealth%2FCoreFICS%2Ffics-helm-chart/merge_requests/211"
		if r.URL.RawPath != wantRaw && r.URL.Path != "/api/v4/projects/PiHealth/CoreFICS/fics-helm-chart/merge_requests/211" {
			t.Errorf("unexpected path: raw=%s decoded=%s", r.URL.RawPath, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		resp := GitLabMR{IID: 211, State: "opened", ProjectID: 99}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestGitLabClient(server.URL)
	mr, err := client.GetMergeRequestByPath(context.Background(), "PiHealth/CoreFICS/fics-helm-chart", 211)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mr.State != "opened" {
		t.Errorf("State = %q, want opened", mr.State)
	}
}

func TestGitLabClient_ListMergedMRs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/groups/10/merge_requests" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("state") != "merged" {
			t.Error("expected state=merged query param")
		}
		if r.URL.Query().Get("updated_after") == "" {
			t.Error("expected updated_after query param")
		}
		resp := []GitLabMR{
			{IID: 210, State: "merged", ProjectID: 42, Title: "MR 210"},
			{IID: 211, State: "merged", ProjectID: 42, Title: "MR 211"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestGitLabClient(server.URL)
	after, _ := time.Parse(time.RFC3339, "2026-03-01T00:00:00Z")
	mrs, err := client.ListMergedMRs(context.Background(), 10, after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mrs) != 2 {
		t.Fatalf("expected 2 MRs, got %d", len(mrs))
	}
	if mrs[0].IID != 210 {
		t.Errorf("first MR IID = %d, want 210", mrs[0].IID)
	}
}

func TestGitLabClient_GetPipeline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/42/pipelines/5000" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		resp := GitLabPipeline{ID: 5000, Status: "success", Ref: "main"}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestGitLabClient(server.URL)
	pip, err := client.GetPipeline(context.Background(), 42, 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pip.Status != "success" {
		t.Errorf("Status = %q, want success", pip.Status)
	}
}

func TestGitLabClient_ErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Not Found"}`))
	}))
	defer server.Close()

	client := newTestGitLabClient(server.URL)
	_, err := client.GetMergeRequest(context.Background(), 42, 999)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestParseMRURL(t *testing.T) {
	tests := []struct {
		name        string
		url         string
		wantPath    string
		wantIID     int
		wantNil     bool
	}{
		{
			name:    "standard MR URL",
			url:     "https://gitlab.com/PiHealth/CoreFICS/fics-helm-chart/-/merge_requests/211",
			wantPath: "PiHealth/CoreFICS/fics-helm-chart",
			wantIID:  211,
		},
		{
			name:    "simple project",
			url:     "https://gitlab.com/org/repo/-/merge_requests/42",
			wantPath: "org/repo",
			wantIID:  42,
		},
		{
			name:    "deeply nested project",
			url:     "https://gitlab.example.com/a/b/c/d/-/merge_requests/1",
			wantPath: "a/b/c/d",
			wantIID:  1,
		},
		{
			name:    "not a MR URL",
			url:     "https://gitlab.com/org/repo/-/issues/42",
			wantNil: true,
		},
		{
			name:    "empty string",
			url:     "",
			wantNil: true,
		},
		{
			name:    "github URL",
			url:     "https://github.com/org/repo/pull/42",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref := ParseMRURL(tt.url)
			if tt.wantNil {
				if ref != nil {
					t.Errorf("expected nil, got %+v", ref)
				}
				return
			}
			if ref == nil {
				t.Fatal("expected non-nil MRRef")
			}
			if ref.ProjectPath != tt.wantPath {
				t.Errorf("ProjectPath = %q, want %q", ref.ProjectPath, tt.wantPath)
			}
			if ref.IID != tt.wantIID {
				t.Errorf("IID = %d, want %d", ref.IID, tt.wantIID)
			}
		})
	}
}

