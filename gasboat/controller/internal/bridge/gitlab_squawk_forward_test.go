package bridge

import (
	"context"
	"log/slog"
	"testing"

	"gasboat/controller/internal/beadsapi"
)

func TestGitLabSquawkForwarder_IgnoresNonMessageBeads(t *testing.T) {
	f := &GitLabSquawkForwarder{
		daemon: newMockDaemon(),
		gitlab: NewGitLabClient(GitLabClientConfig{Logger: slog.Default()}),
		logger: slog.Default(),
		seen:   make(map[string]bool),
	}

	data := marshalSSEBeadPayload(BeadEvent{
		ID:     "bd-task1",
		Type:   "task",
		Labels: []string{"bug"},
	})
	f.handleClosed(context.Background(), data)

	if f.alreadySeen("bd-task1") {
		t.Error("expected task bead to be ignored, not marked as seen")
	}
}

func TestGitLabSquawkForwarder_IgnoresMessageWithoutSquawkLabel(t *testing.T) {
	f := &GitLabSquawkForwarder{
		daemon: newMockDaemon(),
		gitlab: NewGitLabClient(GitLabClientConfig{Logger: slog.Default()}),
		logger: slog.Default(),
		seen:   make(map[string]bool),
	}

	data := marshalSSEBeadPayload(BeadEvent{
		ID:     "bd-msg1",
		Type:   "message",
		Labels: []string{"from:test-agent"},
	})
	f.handleClosed(context.Background(), data)

	if f.alreadySeen("bd-msg1") {
		t.Error("expected non-squawk message bead to be ignored")
	}
}

func TestGitLabSquawkForwarder_SkipsAgentWithoutMRBinding(t *testing.T) {
	mock := newMockDaemon()
	// Agent exists but has no gitlab_mr_url metadata.
	mock.mu.Lock()
	bd := beadsapiBeadDetail("test-agent", "agent", nil)
	mock.beads["test-agent"] = &bd
	mock.mu.Unlock()

	f := &GitLabSquawkForwarder{
		daemon: mock,
		gitlab: NewGitLabClient(GitLabClientConfig{Logger: slog.Default()}),
		logger: slog.Default(),
		seen:   make(map[string]bool),
	}

	data := marshalSSEBeadPayload(BeadEvent{
		ID:     "bd-squawk1",
		Type:   "message",
		Labels: []string{"squawk", "from:test-agent"},
		Fields: map[string]string{
			"source_agent": "test-agent",
			"text":         "Build complete",
		},
	})
	f.handleClosed(context.Background(), data)

	// Should be marked as seen (was processed) but no MR post attempted.
	if !f.alreadySeen("bd-squawk1") {
		t.Error("expected bead to be marked as seen after processing")
	}
}

func TestGitLabSquawkForwarder_SkipsUnknownAgent(t *testing.T) {
	mock := newMockDaemon()
	// No agent seeded — FindAgentBead will return error.

	f := &GitLabSquawkForwarder{
		daemon: mock,
		gitlab: NewGitLabClient(GitLabClientConfig{Logger: slog.Default()}),
		logger: slog.Default(),
		seen:   make(map[string]bool),
	}

	data := marshalSSEBeadPayload(BeadEvent{
		ID:     "bd-squawk2",
		Type:   "message",
		Labels: []string{"squawk", "from:unknown-agent"},
		Fields: map[string]string{
			"source_agent": "unknown-agent",
			"text":         "Hello",
		},
	})
	f.handleClosed(context.Background(), data)

	// Agent not found → silently skipped, bead still marked seen.
	if !f.alreadySeen("bd-squawk2") {
		t.Error("expected bead to be marked as seen")
	}
}

func TestGitLabSquawkForwarder_Dedup(t *testing.T) {
	f := &GitLabSquawkForwarder{
		daemon: newMockDaemon(),
		gitlab: NewGitLabClient(GitLabClientConfig{Logger: slog.Default()}),
		logger: slog.Default(),
		seen:   make(map[string]bool),
	}

	data := marshalSSEBeadPayload(BeadEvent{
		ID:     "bd-squawk-dup",
		Type:   "message",
		Labels: []string{"squawk", "from:agent-1"},
		Fields: map[string]string{
			"source_agent": "agent-1",
			"text":         "First call",
		},
	})

	f.handleClosed(context.Background(), data)
	if !f.alreadySeen("bd-squawk-dup") {
		t.Error("expected bead to be seen after first call")
	}

	// Second call should be deduped — no panic, no double processing.
	f.handleClosed(context.Background(), data)
}

func TestGitLabSquawkForwarder_AcceptsSayLabel(t *testing.T) {
	mock := newMockDaemon()
	// No agent bead seeded, so FindAgentBead will fail — that's fine,
	// we just verify the "say" label passes the filter.

	f := &GitLabSquawkForwarder{
		daemon: mock,
		gitlab: NewGitLabClient(GitLabClientConfig{Logger: slog.Default()}),
		logger: slog.Default(),
		seen:   make(map[string]bool),
	}

	data := marshalSSEBeadPayload(BeadEvent{
		ID:     "bd-say1",
		Type:   "message",
		Labels: []string{"say", "from:test-agent"},
		Fields: map[string]string{
			"source_agent": "test-agent",
			"text":         "Legacy say message",
		},
	})
	f.handleClosed(context.Background(), data)

	if !f.alreadySeen("bd-say1") {
		t.Error("expected legacy 'say' label to be accepted")
	}
}

func TestGitLabSquawkForwarder_ResolveMR_FromURL(t *testing.T) {
	f := &GitLabSquawkForwarder{
		logger: slog.Default(),
	}

	agentBead := &beadsapi.BeadDetail{
		Fields: map[string]string{},
	}
	mrURL := "https://gitlab.com/PiHealth/CoreFICS/monorepo/-/merge_requests/42"

	projectPath, iid, err := f.resolveMR(agentBead, mrURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if projectPath != "PiHealth/CoreFICS/monorepo" {
		t.Errorf("projectPath = %q, want %q", projectPath, "PiHealth/CoreFICS/monorepo")
	}
	if iid != 42 {
		t.Errorf("iid = %d, want 42", iid)
	}
}

func TestGitLabSquawkForwarder_ResolveMR_FromBeadFieldIID(t *testing.T) {
	f := &GitLabSquawkForwarder{
		logger: slog.Default(),
	}

	agentBead := &beadsapi.BeadDetail{
		Fields: map[string]string{
			"gitlab_mr_iid": "99",
		},
	}
	mrURL := "https://gitlab.com/PiHealth/CoreFICS/monorepo/-/merge_requests/42"

	// Should prefer the bead field IID over the URL IID.
	projectPath, iid, err := f.resolveMR(agentBead, mrURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if projectPath != "PiHealth/CoreFICS/monorepo" {
		t.Errorf("projectPath = %q, want %q", projectPath, "PiHealth/CoreFICS/monorepo")
	}
	if iid != 99 {
		t.Errorf("iid = %d, want 99 (from bead field)", iid)
	}
}

func TestGitLabSquawkForwarder_ResolveMR_InvalidURL(t *testing.T) {
	f := &GitLabSquawkForwarder{
		logger: slog.Default(),
	}

	agentBead := &beadsapi.BeadDetail{
		Fields: map[string]string{},
	}
	_, _, err := f.resolveMR(agentBead, "not-a-valid-url")
	if err == nil {
		t.Error("expected error for invalid MR URL")
	}
}

// beadsapiBeadDetail is a test helper to create a BeadDetail with optional fields.
func beadsapiBeadDetail(name, beadType string, fields map[string]string) beadsapi.BeadDetail {
	return beadsapi.BeadDetail{
		ID:     "bd-" + name,
		Title:  name,
		Type:   beadType,
		Fields: fields,
	}
}
