package bridge

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"gasboat/controller/internal/beadsapi"
)

// mockResolverDaemon implements AgentResolverClient for testing.
type mockResolverDaemon struct {
	mu           sync.Mutex
	agentBeads   map[string]*beadsapi.BeadDetail // agent name → bead
	spawnedBeads []*beadsapi.BeadDetail
	updatedBeads map[string]map[string]string // bead ID → fields
	spawnErr     error
}

func newMockResolverDaemon() *mockResolverDaemon {
	return &mockResolverDaemon{
		agentBeads:   make(map[string]*beadsapi.BeadDetail),
		updatedBeads: make(map[string]map[string]string),
	}
}

func (m *mockResolverDaemon) FindAgentBead(_ context.Context, agentName string) (*beadsapi.BeadDetail, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.agentBeads[agentName]; ok {
		return b, nil
	}
	return nil, &beadsapi.APIError{StatusCode: 404, Message: "not found"}
}

func (m *mockResolverDaemon) SpawnAgent(_ context.Context, agentName, project, taskID, role, customPrompt string, extraFields ...map[string]string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.spawnErr != nil {
		return "", m.spawnErr
	}
	id := "bd-spawned-" + agentName
	fields := map[string]string{
		"agent":   agentName,
		"project": project,
		"mode":    "crew",
		"role":    role,
		"prompt":  customPrompt,
	}
	if taskID != "" {
		fields["task_id"] = taskID
	}
	for _, extra := range extraFields {
		for k, v := range extra {
			fields[k] = v
		}
	}
	bead := &beadsapi.BeadDetail{
		ID:     id,
		Title:  agentName,
		Type:   "agent",
		Fields: fields,
	}
	m.spawnedBeads = append(m.spawnedBeads, bead)
	return id, nil
}

func (m *mockResolverDaemon) UpdateBeadFields(_ context.Context, beadID string, fields map[string]string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.updatedBeads[beadID] = fields
	return nil
}

func (m *mockResolverDaemon) getSpawned() []*beadsapi.BeadDetail {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]*beadsapi.BeadDetail{}, m.spawnedBeads...)
}

func (m *mockResolverDaemon) getUpdated(beadID string) map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.updatedBeads[beadID]
}

// newTestAgentResolver creates an AgentResolver with a mock daemon and a GitLab
// test server that returns MR details.
func newTestAgentResolver(daemon *mockResolverDaemon, gitlabServer *httptest.Server) *AgentResolver {
	var gitlab *GitLabClient
	if gitlabServer != nil {
		gitlab = newTestGitLabClient(gitlabServer.URL)
	}
	return NewAgentResolver(AgentResolverConfig{
		Daemon: daemon,
		GitLab: gitlab,
		Client: &http.Client{},
		Logger: slog.Default(),
	})
}

func TestResolveAndNudge_AgentAlive_Nudges(t *testing.T) {
	// Set up a coop server that accepts nudges.
	var nudgeReceived string
	coopServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/nudge" {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			nudgeReceived = body["message"]
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]bool{"delivered": true})
			return
		}
		http.NotFound(w, r)
	}))
	defer coopServer.Close()

	daemon := newMockResolverDaemon()
	daemon.agentBeads["test-agent"] = &beadsapi.BeadDetail{
		ID:    "bd-agent-1",
		Title: "test-agent",
		Type:  "agent",
		Notes: "coop_url: " + coopServer.URL,
		Fields: map[string]string{
			"agent":   "test-agent",
			"project": "gasboat",
		},
	}

	resolver := newTestAgentResolver(daemon, nil)

	taskBead := BeadEvent{
		ID:       "kd-task-1",
		Type:     "task",
		Title:    "Fix auth bug",
		Assignee: "test-agent",
		Labels:   []string{"project:gasboat"},
		Fields: map[string]string{
			"mr_url":            "https://gitlab.com/org/repo/-/merge_requests/42",
			"gitlab_project_id": "99",
			"gitlab_mr_iid":     "42",
		},
	}

	err := resolver.ResolveAndNudge(context.Background(), taskBead, "Review comment: fix the bug")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nudgeReceived != "Review comment: fix the bug" {
		t.Errorf("nudge message = %q, want %q", nudgeReceived, "Review comment: fix the bug")
	}

	// Verify MR binding fields were set on agent bead.
	updated := daemon.getUpdated("bd-agent-1")
	if updated["gitlab_mr_url"] != "https://gitlab.com/org/repo/-/merge_requests/42" {
		t.Errorf("gitlab_mr_url = %q, want MR URL", updated["gitlab_mr_url"])
	}
	if updated["gitlab_project_id"] != "99" {
		t.Errorf("gitlab_project_id = %q, want 99", updated["gitlab_project_id"])
	}
	if updated["gitlab_mr_iid"] != "42" {
		t.Errorf("gitlab_mr_iid = %q, want 42", updated["gitlab_mr_iid"])
	}
}

func TestResolveAndNudge_AgentAlive_CoopURLInFields(t *testing.T) {
	// Agent has coop_url as a field instead of in notes.
	var nudgeReceived bool
	coopServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/nudge" {
			nudgeReceived = true
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]bool{"delivered": true})
			return
		}
		http.NotFound(w, r)
	}))
	defer coopServer.Close()

	daemon := newMockResolverDaemon()
	daemon.agentBeads["test-agent"] = &beadsapi.BeadDetail{
		ID:    "bd-agent-1",
		Title: "test-agent",
		Type:  "agent",
		Fields: map[string]string{
			"agent":    "test-agent",
			"coop_url": coopServer.URL,
		},
	}

	resolver := newTestAgentResolver(daemon, nil)

	taskBead := BeadEvent{
		ID:       "kd-task-1",
		Type:     "task",
		Assignee: "test-agent",
		Fields: map[string]string{
			"mr_url": "https://gitlab.com/org/repo/-/merge_requests/42",
		},
	}

	err := resolver.ResolveAndNudge(context.Background(), taskBead, "test message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !nudgeReceived {
		t.Error("expected nudge to be delivered via coop")
	}
}

func TestResolveAndNudge_AgentDead_SpawnsNew(t *testing.T) {
	daemon := newMockResolverDaemon()
	// No agent bead → agent is dead.

	// GitLab server returns MR details with source branch.
	gitlabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := GitLabMR{
			IID:          42,
			State:        "opened",
			ProjectID:    99,
			SourceBranch: "fix/auth-bug",
			TargetBranch: "main",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer gitlabServer.Close()

	resolver := newTestAgentResolver(daemon, gitlabServer)

	taskBead := BeadEvent{
		ID:       "kd-task-1",
		Type:     "task",
		Title:    "Fix auth bug",
		Assignee: "test-agent",
		Labels:   []string{"project:gasboat"},
		Fields: map[string]string{
			"mr_url": "https://gitlab.com/org/repo/-/merge_requests/42",
		},
	}

	err := resolver.ResolveAndNudge(context.Background(), taskBead, "Review comment")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify agent was spawned.
	spawned := daemon.getSpawned()
	if len(spawned) != 1 {
		t.Fatalf("expected 1 spawned agent, got %d", len(spawned))
	}

	agent := spawned[0]
	if agent.Fields["agent"] != "test-agent" {
		t.Errorf("agent name = %q, want test-agent", agent.Fields["agent"])
	}
	if agent.Fields["project"] != "gasboat" {
		t.Errorf("project = %q, want gasboat", agent.Fields["project"])
	}
	if agent.Fields["task_id"] != "kd-task-1" {
		t.Errorf("task_id = %q, want kd-task-1", agent.Fields["task_id"])
	}
	if agent.Fields["spawn_source"] != "gitlab-mr-review" {
		t.Errorf("spawn_source = %q, want gitlab-mr-review", agent.Fields["spawn_source"])
	}
	if agent.Fields["gitlab_mr_url"] != "https://gitlab.com/org/repo/-/merge_requests/42" {
		t.Errorf("gitlab_mr_url = %q, want MR URL", agent.Fields["gitlab_mr_url"])
	}
	if agent.Fields["gitlab_mr_source_branch"] != "fix/auth-bug" {
		t.Errorf("gitlab_mr_source_branch = %q, want fix/auth-bug", agent.Fields["gitlab_mr_source_branch"])
	}
	if agent.Fields["gitlab_project_id"] != "99" {
		t.Errorf("gitlab_project_id = %q, want 99", agent.Fields["gitlab_project_id"])
	}
	if agent.Fields["gitlab_mr_iid"] != "42" {
		t.Errorf("gitlab_mr_iid = %q, want 42", agent.Fields["gitlab_mr_iid"])
	}
}

func TestResolveAndNudge_AgentExistsButNoCoopURL_SpawnsNew(t *testing.T) {
	daemon := newMockResolverDaemon()
	// Agent bead exists but has no coop_url (not running).
	daemon.agentBeads["test-agent"] = &beadsapi.BeadDetail{
		ID:    "bd-agent-old",
		Title: "test-agent",
		Type:  "agent",
		Fields: map[string]string{
			"agent":   "test-agent",
			"project": "gasboat",
		},
	}

	resolver := newTestAgentResolver(daemon, nil)

	taskBead := BeadEvent{
		ID:       "kd-task-1",
		Type:     "task",
		Title:    "Fix it",
		Assignee: "test-agent",
		Labels:   []string{"project:gasboat"},
		Fields: map[string]string{
			"mr_url": "https://gitlab.com/org/repo/-/merge_requests/5",
		},
	}

	err := resolver.ResolveAndNudge(context.Background(), taskBead, "Review comment")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	spawned := daemon.getSpawned()
	if len(spawned) != 1 {
		t.Fatalf("expected 1 spawned agent, got %d", len(spawned))
	}
	if spawned[0].Fields["agent"] != "test-agent" {
		t.Errorf("spawned agent name = %q, want test-agent", spawned[0].Fields["agent"])
	}
}

func TestResolveAndNudge_NoAssignee_ReturnsError(t *testing.T) {
	daemon := newMockResolverDaemon()
	resolver := newTestAgentResolver(daemon, nil)

	taskBead := BeadEvent{
		ID:       "kd-task-1",
		Type:     "task",
		Assignee: "", // no assignee
		Fields:   map[string]string{"mr_url": "https://gitlab.com/org/repo/-/merge_requests/1"},
	}

	err := resolver.ResolveAndNudge(context.Background(), taskBead, "msg")
	if err == nil {
		t.Fatal("expected error for empty assignee")
	}
}

func TestBuildMRFields_PopulatesFromTaskBead(t *testing.T) {
	resolver := newTestAgentResolver(newMockResolverDaemon(), nil)

	taskBead := BeadEvent{
		Fields: map[string]string{
			"mr_url":            "https://gitlab.com/org/repo/-/merge_requests/42",
			"gitlab_project_id": "99",
			"gitlab_mr_iid":     "42",
		},
	}

	fields := resolver.buildMRFields(taskBead)
	if fields["gitlab_mr_url"] != "https://gitlab.com/org/repo/-/merge_requests/42" {
		t.Errorf("gitlab_mr_url = %q", fields["gitlab_mr_url"])
	}
	if fields["gitlab_project_id"] != "99" {
		t.Errorf("gitlab_project_id = %q", fields["gitlab_project_id"])
	}
	if fields["gitlab_mr_iid"] != "42" {
		t.Errorf("gitlab_mr_iid = %q", fields["gitlab_mr_iid"])
	}
}

func TestBuildMRFields_FetchesSourceBranch(t *testing.T) {
	gitlabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := GitLabMR{
			IID:          42,
			ProjectID:    99,
			SourceBranch: "feature/new-thing",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer gitlabServer.Close()

	resolver := newTestAgentResolver(newMockResolverDaemon(), gitlabServer)

	taskBead := BeadEvent{
		Fields: map[string]string{
			"mr_url": "https://gitlab.com/org/repo/-/merge_requests/42",
		},
	}

	fields := resolver.buildMRFields(taskBead)
	if fields["gitlab_mr_source_branch"] != "feature/new-thing" {
		t.Errorf("gitlab_mr_source_branch = %q, want feature/new-thing", fields["gitlab_mr_source_branch"])
	}
	// Should backfill project_id and iid from the GitLab response.
	if fields["gitlab_project_id"] != "99" {
		t.Errorf("gitlab_project_id = %q, want 99", fields["gitlab_project_id"])
	}
	if fields["gitlab_mr_iid"] != "42" {
		t.Errorf("gitlab_mr_iid = %q, want 42", fields["gitlab_mr_iid"])
	}
}

func TestBuildMRFields_NoMRURL_ReturnsEmpty(t *testing.T) {
	resolver := newTestAgentResolver(newMockResolverDaemon(), nil)

	taskBead := BeadEvent{
		Fields: map[string]string{}, // no mr_url
	}

	fields := resolver.buildMRFields(taskBead)
	if len(fields) != 0 {
		t.Errorf("expected empty fields, got %v", fields)
	}
}

func TestHandleReviewNudge_WithResolver(t *testing.T) {
	// Test that handleReviewNudge uses the resolver when configured.
	var nudgeReceived string
	coopServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/nudge" {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			nudgeReceived = body["message"]
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]bool{"delivered": true})
			return
		}
		http.NotFound(w, r)
	}))
	defer coopServer.Close()

	resolverDaemon := newMockResolverDaemon()
	resolverDaemon.agentBeads["my-agent"] = &beadsapi.BeadDetail{
		ID:    "bd-agent-1",
		Title: "my-agent",
		Type:  "agent",
		Notes: "coop_url: " + coopServer.URL,
		Fields: map[string]string{
			"agent": "my-agent",
		},
	}

	resolver := newTestAgentResolver(resolverDaemon, nil)

	// Build a mock GitLabBeadClient that returns the task bead.
	syncDaemon := &mockGitLabBeadClient{
		beads: []*beadsapi.BeadDetail{},
	}

	sync := NewGitLabSync(GitLabSyncConfig{
		Daemon:   syncDaemon,
		Logger:   slog.Default(),
		Resolver: resolver,
	})

	// Simulate a bead update event with mr_has_review_comments changed.
	// Changes are at the wrapper level in SSE format, not inside the bead.
	data, _ := json.Marshal(map[string]any{
		"bead": map[string]any{
			"id":       "kd-task-1",
			"type":     "task",
			"title":    "Fix bug",
			"assignee": "my-agent",
			"fields": map[string]string{
				"mr_has_review_comments": "true",
				"mr_url":                "https://gitlab.com/org/repo/-/merge_requests/42",
			},
		},
		"changes": map[string]any{
			"mr_has_review_comments": "true",
		},
	})
	sync.handleReviewNudge(context.Background(), data)

	if nudgeReceived == "" {
		t.Error("expected nudge to be delivered via resolver")
	}
	if nudgeReceived != "MR has new review comments — address them: https://gitlab.com/org/repo/-/merge_requests/42" {
		t.Errorf("unexpected nudge message: %q", nudgeReceived)
	}
}

// mockGitLabBeadClient implements GitLabBeadClient for sync tests.
type mockGitLabBeadClient struct {
	beads []*beadsapi.BeadDetail
}

func (m *mockGitLabBeadClient) ListTaskBeads(_ context.Context) ([]*beadsapi.BeadDetail, error) {
	return m.beads, nil
}

func (m *mockGitLabBeadClient) UpdateBeadFields(_ context.Context, _ string, _ map[string]string) error {
	return nil
}

func (m *mockGitLabBeadClient) AddComment(_ context.Context, _, _, _ string) error {
	return nil
}
