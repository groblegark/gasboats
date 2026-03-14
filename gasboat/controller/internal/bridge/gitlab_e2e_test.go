package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gasboat/controller/internal/beadsapi"
)

// e2eDaemon is a combined mock that satisfies GitLabBeadClient,
// AgentResolverClient, and GitLabSquawkClient for E2E testing.
type e2eDaemon struct {
	mu           sync.Mutex
	beads        map[string]*beadsapi.BeadDetail // bead ID → bead
	agentBeads   map[string]*beadsapi.BeadDetail // agent name → agent bead
	comments     []commentRecord
	spawnedBeads []*beadsapi.BeadDetail
}

func newE2EDaemon() *e2eDaemon {
	return &e2eDaemon{
		beads:      make(map[string]*beadsapi.BeadDetail),
		agentBeads: make(map[string]*beadsapi.BeadDetail),
	}
}

// --- GitLabBeadClient methods ---

func (d *e2eDaemon) ListTaskBeads(_ context.Context) ([]*beadsapi.BeadDetail, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	var result []*beadsapi.BeadDetail
	for _, b := range d.beads {
		if b.Type == "task" {
			result = append(result, b)
		}
	}
	return result, nil
}

func (d *e2eDaemon) UpdateBeadFields(_ context.Context, beadID string, fields map[string]string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	// Check task beads by ID.
	if bead, ok := d.beads[beadID]; ok {
		if bead.Fields == nil {
			bead.Fields = make(map[string]string)
		}
		for k, v := range fields {
			bead.Fields[k] = v
		}
		return nil
	}
	// Check agent beads by ID (agentBeads is keyed by name, so scan by ID).
	for _, bead := range d.agentBeads {
		if bead.ID == beadID {
			if bead.Fields == nil {
				bead.Fields = make(map[string]string)
			}
			for k, v := range fields {
				bead.Fields[k] = v
			}
			return nil
		}
	}
	return fmt.Errorf("bead %s not found", beadID)
}

func (d *e2eDaemon) AddComment(_ context.Context, beadID, author, text string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.beads[beadID]; !ok {
		return fmt.Errorf("bead %s not found", beadID)
	}
	d.comments = append(d.comments, commentRecord{BeadID: beadID, Author: author, Text: text})
	return nil
}

// --- AgentResolverClient methods ---

func (d *e2eDaemon) FindAgentBead(_ context.Context, agentName string) (*beadsapi.BeadDetail, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if b, ok := d.agentBeads[agentName]; ok {
		return b, nil
	}
	return nil, &beadsapi.APIError{StatusCode: 404, Message: "not found"}
}

func (d *e2eDaemon) SpawnAgent(_ context.Context, agentName, project, taskID, role, customPrompt string, extraFields ...map[string]string) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
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
	d.spawnedBeads = append(d.spawnedBeads, bead)
	// Also register as an agent bead so squawk forwarder can find it.
	d.agentBeads[agentName] = bead
	return id, nil
}

// --- Test helpers ---

func (d *e2eDaemon) getComments() []commentRecord {
	d.mu.Lock()
	defer d.mu.Unlock()
	result := make([]commentRecord, len(d.comments))
	copy(result, d.comments)
	return result
}

func (d *e2eDaemon) getBead(id string) *beadsapi.BeadDetail {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.beads[id]
}

func (d *e2eDaemon) getSpawned() []*beadsapi.BeadDetail {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]*beadsapi.BeadDetail{}, d.spawnedBeads...)
}

// TestE2E_GitLabReviewComment_AgentAlive_ResponsePostedAsMRNote validates:
// 1. GitLab note webhook → bead comment + mr_has_review_comments flag
// 2. SSE bead update → agent resolver nudges alive agent via coop
// 3. Agent squawk → squawk forwarder posts MR note
func TestE2E_GitLabReviewComment_AgentAlive_ResponsePostedAsMRNote(t *testing.T) {
	const mrURL = "https://gitlab.com/org/repo/-/merge_requests/42"
	const agentName = "review-agent"

	// Track nudge messages received by the coop server.
	var nudgeMessages []string
	var nudgeMu sync.Mutex
	coopServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/agent/nudge" {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			nudgeMu.Lock()
			nudgeMessages = append(nudgeMessages, body["message"])
			nudgeMu.Unlock()
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]bool{"delivered": true})
			return
		}
		http.NotFound(w, r)
	}))
	defer coopServer.Close()

	// Track MR notes posted to GitLab.
	var postedNotes []string
	var notesMu sync.Mutex
	gitlabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/notes") {
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			notesMu.Lock()
			postedNotes = append(postedNotes, body["body"])
			notesMu.Unlock()
			resp := GitLabNote{ID: 5001, Body: body["body"]}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		// Return MR details for GetMergeRequestByPath.
		if strings.Contains(r.URL.Path, "/merge_requests/") {
			resp := GitLabMR{IID: 42, ProjectID: 99, SourceBranch: "fix/auth", State: "opened"}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer gitlabServer.Close()

	gitlabClient := newTestGitLabClient(gitlabServer.URL)
	logger := slog.Default()

	// Set up the combined mock daemon.
	daemon := newE2EDaemon()

	// Seed: task bead with mr_url assigned to an agent.
	daemon.beads["kd-task-1"] = &beadsapi.BeadDetail{
		ID:       "kd-task-1",
		Title:    "Fix auth token refresh",
		Type:     "task",
		Assignee: agentName,
		Fields:   map[string]string{"mr_url": mrURL},
	}

	// Seed: agent bead with coop_url (alive).
	daemon.agentBeads[agentName] = &beadsapi.BeadDetail{
		ID:    "bd-agent-1",
		Title: agentName,
		Type:  "agent",
		Notes: "coop_url: " + coopServer.URL,
		Fields: map[string]string{
			"agent":   agentName,
			"project": "gasboat",
		},
	}

	ctx := context.Background()

	// ──────────────────────────────────────────────────────────────────
	// STEP 1: GitLab note webhook → bead comment + review flag
	// ──────────────────────────────────────────────────────────────────
	webhookHandler := GitLabWebhookHandlerWithConfig(GitLabWebhookConfig{
		GitLab:        gitlabClient,
		Daemon:        daemon,
		WebhookSecret: "test-secret",
		BotUsername:    "gasboat-bot",
		Nudge: func(ctx context.Context, agent, msg string) error {
			// Webhook-level nudge (simple, not resolver).
			return nil
		},
		Logger: logger,
	})

	noteEvent := map[string]any{
		"object_kind": "note",
		"user":        map[string]any{"username": "reviewer-alice"},
		"object_attributes": map[string]any{
			"note":          "Please add error handling on line 42",
			"noteable_type": "MergeRequest",
			"system":        false,
			"discussion_id": "disc-e2e-abc",
			"position": map[string]any{
				"new_path": "pkg/auth/handler.go",
				"new_line": 42,
			},
		},
		"merge_request": map[string]any{
			"iid":   42,
			"url":   mrURL,
			"title": "Fix auth token refresh",
		},
	}
	body, _ := json.Marshal(noteEvent)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Token", "test-secret")
	w := httptest.NewRecorder()
	webhookHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("step 1: webhook returned %d, want 200", w.Code)
	}

	// Verify comment was added to the bead.
	comments := daemon.getComments()
	if len(comments) != 1 {
		t.Fatalf("step 1: expected 1 comment, got %d", len(comments))
	}
	if comments[0].BeadID != "kd-task-1" {
		t.Errorf("step 1: comment bead ID = %q, want kd-task-1", comments[0].BeadID)
	}
	if !strings.Contains(comments[0].Text, "reviewer-alice") {
		t.Errorf("step 1: comment should mention reviewer, got: %s", comments[0].Text)
	}
	if !strings.Contains(comments[0].Text, "pkg/auth/handler.go:42") {
		t.Errorf("step 1: comment should contain file:line, got: %s", comments[0].Text)
	}

	// Verify mr_has_review_comments flag was set.
	taskBead := daemon.getBead("kd-task-1")
	if taskBead.Fields["mr_has_review_comments"] != "true" {
		t.Errorf("step 1: mr_has_review_comments = %q, want true", taskBead.Fields["mr_has_review_comments"])
	}

	// ──────────────────────────────────────────────────────────────────
	// STEP 2: SSE bead update → agent resolver nudges alive agent
	// ──────────────────────────────────────────────────────────────────
	resolver := NewAgentResolver(AgentResolverConfig{
		Daemon: daemon,
		GitLab: gitlabClient,
		Client: &http.Client{},
		Logger: logger,
	})

	gitlabSync := NewGitLabSync(GitLabSyncConfig{
		Daemon:   daemon,
		GitLab:   gitlabClient,
		Logger:   logger,
		Resolver: resolver,
	})

	// Simulate the SSE bead.updated event that fires when mr_has_review_comments is set.
	sseData, _ := json.Marshal(map[string]any{
		"bead": map[string]any{
			"id":       "kd-task-1",
			"type":     "task",
			"title":    "Fix auth token refresh",
			"assignee": agentName,
			"fields": map[string]string{
				"mr_has_review_comments": "true",
				"mr_url":                mrURL,
			},
		},
		"changes": map[string]any{
			"mr_has_review_comments": "true",
		},
	})
	gitlabSync.handleReviewNudge(ctx, sseData)

	// Verify the nudge was delivered to the coop server.
	nudgeMu.Lock()
	nudgeCount := len(nudgeMessages)
	nudgeMu.Unlock()
	if nudgeCount != 1 {
		t.Fatalf("step 2: expected 1 nudge, got %d", nudgeCount)
	}
	nudgeMu.Lock()
	nudgeMsg := nudgeMessages[0]
	nudgeMu.Unlock()
	if !strings.Contains(nudgeMsg, "review comments") {
		t.Errorf("step 2: nudge should mention review comments, got: %s", nudgeMsg)
	}
	if !strings.Contains(nudgeMsg, mrURL) {
		t.Errorf("step 2: nudge should contain MR URL, got: %s", nudgeMsg)
	}

	// Verify MR binding fields were set on agent bead.
	daemon.mu.Lock()
	agentBead := daemon.agentBeads[agentName]
	agentMRURL := agentBead.Fields["gitlab_mr_url"]
	daemon.mu.Unlock()
	if agentMRURL != mrURL {
		t.Errorf("step 2: agent gitlab_mr_url = %q, want %q", agentMRURL, mrURL)
	}

	// ──────────────────────────────────────────────────────────────────
	// STEP 3: Agent squawk → squawk forwarder posts MR note
	// ──────────────────────────────────────────────────────────────────
	squawkFwd := NewGitLabSquawkForwarder(GitLabSquawkForwarderConfig{
		Daemon: daemon,
		GitLab: gitlabClient,
		Logger: logger,
	})

	// Simulate a closed squawk bead from the agent.
	squawkData := marshalSSEBeadPayload(BeadEvent{
		ID:     "bd-squawk-e2e",
		Type:   "message",
		Labels: []string{"squawk", "from:" + agentName},
		Fields: map[string]string{
			"source_agent": agentName,
			"text":         "Fixed the error handling, please re-review",
		},
	})
	squawkFwd.handleClosed(ctx, squawkData)

	// Verify the squawk was posted as a GitLab MR note.
	notesMu.Lock()
	noteCount := len(postedNotes)
	notesMu.Unlock()
	if noteCount != 1 {
		t.Fatalf("step 3: expected 1 MR note, got %d", noteCount)
	}
	notesMu.Lock()
	noteBody := postedNotes[0]
	notesMu.Unlock()
	if !strings.Contains(noteBody, agentName) {
		t.Errorf("step 3: MR note should contain agent name, got: %s", noteBody)
	}
	if !strings.Contains(noteBody, "Fixed the error handling") {
		t.Errorf("step 3: MR note should contain squawk text, got: %s", noteBody)
	}
}

// TestE2E_GitLabReviewComment_AgentDead_SpawnsNewAgent validates:
// 1. GitLab note webhook → bead update
// 2. SSE bead update → agent resolver spawns a new agent (original is dead)
// 3. The spawned agent has MR context metadata
func TestE2E_GitLabReviewComment_AgentDead_SpawnsNewAgent(t *testing.T) {
	const mrURL = "https://gitlab.com/org/repo/-/merge_requests/10"
	const agentName = "dead-agent"

	gitlabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := GitLabMR{IID: 10, ProjectID: 55, SourceBranch: "fix/deadcode", State: "opened"}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer gitlabServer.Close()

	gitlabClient := newTestGitLabClient(gitlabServer.URL)
	logger := slog.Default()

	daemon := newE2EDaemon()

	// Seed: task bead assigned to agent (no agent bead → agent is dead).
	daemon.beads["kd-task-2"] = &beadsapi.BeadDetail{
		ID:       "kd-task-2",
		Title:    "Remove dead code",
		Type:     "task",
		Assignee: agentName,
		Labels:   []string{"project:gasboat"},
		Fields:   map[string]string{"mr_url": mrURL},
	}

	ctx := context.Background()

	// Step 1: Webhook sets review flag.
	webhookHandler := GitLabWebhookHandlerWithConfig(GitLabWebhookConfig{
		Daemon:        daemon,
		WebhookSecret: "secret",
		Logger:        logger,
	})

	noteEvent := map[string]any{
		"object_kind": "note",
		"user":        map[string]any{"username": "bob"},
		"object_attributes": map[string]any{
			"note":          "This is still referenced",
			"noteable_type": "MergeRequest",
			"system":        false,
		},
		"merge_request": map[string]any{
			"iid":   10,
			"url":   mrURL,
			"title": "Remove dead code",
		},
	}
	body, _ := json.Marshal(noteEvent)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Token", "secret")
	w := httptest.NewRecorder()
	webhookHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("webhook returned %d", w.Code)
	}

	// Step 2: SSE triggers resolver → agent is dead → spawn.
	resolver := NewAgentResolver(AgentResolverConfig{
		Daemon: daemon,
		GitLab: gitlabClient,
		Client: &http.Client{},
		Logger: logger,
	})

	gitlabSync := NewGitLabSync(GitLabSyncConfig{
		Daemon:   daemon,
		GitLab:   gitlabClient,
		Logger:   logger,
		Resolver: resolver,
	})

	sseData, _ := json.Marshal(map[string]any{
		"bead": map[string]any{
			"id":       "kd-task-2",
			"type":     "task",
			"title":    "Remove dead code",
			"assignee": agentName,
			"labels":   []string{"project:gasboat"},
			"fields": map[string]string{
				"mr_has_review_comments": "true",
				"mr_url":                mrURL,
			},
		},
		"changes": map[string]any{
			"mr_has_review_comments": "true",
		},
	})
	gitlabSync.handleReviewNudge(ctx, sseData)

	// Verify agent was spawned.
	spawned := daemon.getSpawned()
	if len(spawned) != 1 {
		t.Fatalf("expected 1 spawned agent, got %d", len(spawned))
	}
	agent := spawned[0]
	if agent.Fields["agent"] != agentName {
		t.Errorf("spawned agent name = %q, want %q", agent.Fields["agent"], agentName)
	}
	if agent.Fields["task_id"] != "kd-task-2" {
		t.Errorf("task_id = %q, want kd-task-2", agent.Fields["task_id"])
	}
	if agent.Fields["gitlab_mr_url"] != mrURL {
		t.Errorf("gitlab_mr_url = %q, want %q", agent.Fields["gitlab_mr_url"], mrURL)
	}
	if agent.Fields["gitlab_mr_source_branch"] != "fix/deadcode" {
		t.Errorf("gitlab_mr_source_branch = %q, want fix/deadcode", agent.Fields["gitlab_mr_source_branch"])
	}
	if agent.Fields["spawn_source"] != "gitlab-mr-review" {
		t.Errorf("spawn_source = %q, want gitlab-mr-review", agent.Fields["spawn_source"])
	}
	if agent.Fields["project"] != "gasboat" {
		t.Errorf("project = %q, want gasboat", agent.Fields["project"])
	}
}

// TestE2E_SquawkForwarder_DiscussionReply validates that when an agent has
// gitlab_discussion_id set, the squawk is posted as a discussion reply.
func TestE2E_SquawkForwarder_DiscussionReply(t *testing.T) {
	const mrURL = "https://gitlab.com/org/repo/-/merge_requests/7"
	const agentName = "reply-agent"

	var postedPath string
	var postedBody string
	var mu sync.Mutex
	gitlabServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			mu.Lock()
			postedPath = r.URL.Path
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			postedBody = body["body"]
			mu.Unlock()
			resp := GitLabNote{ID: 3001, Body: body["body"]}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.NotFound(w, r)
	}))
	defer gitlabServer.Close()

	daemon := newE2EDaemon()
	daemon.agentBeads[agentName] = &beadsapi.BeadDetail{
		ID:    "bd-agent-reply",
		Title: agentName,
		Type:  "agent",
		Fields: map[string]string{
			"agent":                 agentName,
			"gitlab_mr_url":         mrURL,
			"gitlab_mr_iid":         "7",
			"gitlab_discussion_id":  "disc-reply-123",
		},
	}

	squawkFwd := NewGitLabSquawkForwarder(GitLabSquawkForwarderConfig{
		Daemon: daemon,
		GitLab: newTestGitLabClient(gitlabServer.URL),
		Logger: slog.Default(),
	})

	squawkData := marshalSSEBeadPayload(BeadEvent{
		ID:     "bd-squawk-reply",
		Type:   "message",
		Labels: []string{"squawk", "from:" + agentName},
		Fields: map[string]string{
			"source_agent": agentName,
			"text":         "Done, error handling added",
		},
	})
	squawkFwd.handleClosed(context.Background(), squawkData)

	mu.Lock()
	path := postedPath
	noteBody := postedBody
	mu.Unlock()

	// Verify it was posted as a discussion reply (path contains /discussions/).
	if !strings.Contains(path, "/discussions/disc-reply-123/notes") {
		t.Errorf("expected discussion reply path, got: %s", path)
	}
	if !strings.Contains(noteBody, agentName) {
		t.Errorf("reply should contain agent name, got: %s", noteBody)
	}
	if !strings.Contains(noteBody, "error handling added") {
		t.Errorf("reply should contain squawk text, got: %s", noteBody)
	}
}

// TestE2E_BotComment_Ignored validates that comments from the bot user
// do not trigger the review comment flow.
func TestE2E_BotComment_Ignored(t *testing.T) {
	daemon := newE2EDaemon()
	daemon.beads["kd-task-3"] = &beadsapi.BeadDetail{
		ID:       "kd-task-3",
		Type:     "task",
		Assignee: "some-agent",
		Fields:   map[string]string{"mr_url": "https://gitlab.com/org/repo/-/merge_requests/5"},
	}

	handler := GitLabWebhookHandlerWithConfig(GitLabWebhookConfig{
		Daemon:        daemon,
		WebhookSecret: "secret",
		BotUsername:    "gasboat-bot",
		Logger:        slog.Default(),
	})

	event := map[string]any{
		"object_kind": "note",
		"user":        map[string]any{"username": "gasboat-bot"},
		"object_attributes": map[string]any{
			"note":          "I've addressed this in the latest commit",
			"noteable_type": "MergeRequest",
			"system":        false,
		},
		"merge_request": map[string]any{
			"iid": 5,
			"url": "https://gitlab.com/org/repo/-/merge_requests/5",
		},
	}
	body, _ := json.Marshal(event)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	req.Header.Set("X-Gitlab-Token", "secret")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Bot comments should not create bead comments or set the review flag.
	if len(daemon.getComments()) != 0 {
		t.Errorf("expected 0 comments for bot user, got %d", len(daemon.getComments()))
	}
	if daemon.getBead("kd-task-3").Fields["mr_has_review_comments"] != "" {
		t.Error("mr_has_review_comments should be empty for bot note")
	}
}
