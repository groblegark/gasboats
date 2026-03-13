// Package bridge provides the GitLab MR squawk forwarder.
//
// GitLabSquawkForwarder watches for closed squawk message beads from agents
// that are bound to a GitLab MR (have gitlab_mr_url metadata) and posts the
// squawk text as MR notes. This gives agents a way to communicate with MR
// reviewers without direct GitLab API access.
package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"gasboat/controller/internal/beadsapi"
)

// GitLabSquawkClient is the subset of beadsapi.Client used by GitLabSquawkForwarder.
type GitLabSquawkClient interface {
	FindAgentBead(ctx context.Context, agentName string) (*beadsapi.BeadDetail, error)
}

// GitLabSquawkForwarder watches for squawk beads from MR-bound agents and
// posts them as GitLab MR notes.
type GitLabSquawkForwarder struct {
	daemon GitLabSquawkClient
	gitlab *GitLabClient
	logger *slog.Logger

	mu   sync.Mutex
	seen map[string]bool // dedup on SSE reconnect
}

// GitLabSquawkForwarderConfig holds configuration for the forwarder.
type GitLabSquawkForwarderConfig struct {
	Daemon GitLabSquawkClient
	GitLab *GitLabClient
	Logger *slog.Logger
}

// NewGitLabSquawkForwarder creates a new GitLab squawk forwarder.
func NewGitLabSquawkForwarder(cfg GitLabSquawkForwarderConfig) *GitLabSquawkForwarder {
	return &GitLabSquawkForwarder{
		daemon: cfg.Daemon,
		gitlab: cfg.GitLab,
		logger: cfg.Logger,
		seen:   make(map[string]bool),
	}
}

// RegisterHandlers registers SSE event handlers on the given stream for
// closed message beads (squawk events).
func (f *GitLabSquawkForwarder) RegisterHandlers(stream *SSEStream) {
	stream.On("beads.bead.closed", f.handleClosed)
	f.logger.Info("GitLab squawk forwarder registered SSE handlers",
		"topics", []string{"beads.bead.closed"})
}

func (f *GitLabSquawkForwarder) handleClosed(ctx context.Context, data []byte) {
	bead := ParseBeadEvent(data)
	if bead == nil {
		return
	}

	// Only handle message beads with the "squawk" or "say" label.
	if bead.Type != "message" || (!hasLabel(bead.Labels, "squawk") && !hasLabel(bead.Labels, "say")) {
		return
	}

	// Dedup on SSE reconnect.
	if f.alreadySeen(bead.ID) {
		return
	}

	// Extract the source agent.
	agent := extractSquawkAgent(*bead)
	if agent == "" {
		return
	}

	// Look up the agent bead to find MR binding metadata.
	agentBead, err := f.daemon.FindAgentBead(ctx, agent)
	if err != nil {
		// Agent not found — not MR-bound, skip silently.
		f.logger.Debug("gitlab-squawk: agent bead not found, skipping",
			"agent", agent, "squawk_id", bead.ID)
		return
	}

	// Check if the agent has MR binding metadata.
	mrURL := agentBead.Fields["gitlab_mr_url"]
	if mrURL == "" {
		// Also check notes for gitlab_mr_url (runtime state).
		if notes := beadsapi.ParseNotes(agentBead.Notes); notes != nil {
			mrURL = notes["gitlab_mr_url"]
		}
	}
	if mrURL == "" {
		// Agent is not bound to a GitLab MR, skip.
		return
	}

	// Get the message text.
	text := bead.Fields["text"]
	if text == "" {
		text = bead.Title
	}

	// Resolve MR project path and IID.
	projectPath, mrIID, err := f.resolveMR(agentBead, mrURL)
	if err != nil {
		f.logger.Warn("gitlab-squawk: failed to resolve MR",
			"agent", agent, "mr_url", mrURL, "error", err)
		return
	}

	// Format the message for GitLab.
	body := fmt.Sprintf("**gasboat agent** `%s`:\n\n%s", agent, text)

	// Check if there's a discussion ID to reply to.
	discussionID := agentBead.Fields["gitlab_discussion_id"]
	if discussionID == "" {
		if notes := beadsapi.ParseNotes(agentBead.Notes); notes != nil {
			discussionID = notes["gitlab_discussion_id"]
		}
	}

	if discussionID != "" {
		// Reply to the specific discussion thread.
		if _, err := f.gitlab.PostMRDiscussionReply(ctx, projectPath, mrIID, discussionID, body); err != nil {
			f.logger.Error("gitlab-squawk: failed to reply to MR discussion",
				"agent", agent, "mr_url", mrURL, "discussion", discussionID, "error", err)
			// Fall through to post as a new note.
		} else {
			f.logger.Info("gitlab-squawk: posted discussion reply",
				"agent", agent, "mr_url", mrURL, "discussion", discussionID)
			return
		}
	}

	// Post as a new MR note.
	if _, err := f.gitlab.PostMRNote(ctx, projectPath, mrIID, body); err != nil {
		f.logger.Error("gitlab-squawk: failed to post MR note",
			"agent", agent, "mr_url", mrURL, "error", err)
	} else {
		f.logger.Info("gitlab-squawk: posted MR note",
			"agent", agent, "mr_url", mrURL, "text_length", len(text))
	}
}

// resolveMR extracts the GitLab project path and MR IID from agent bead
// metadata, falling back to parsing the MR URL.
func (f *GitLabSquawkForwarder) resolveMR(agentBead *beadsapi.BeadDetail, mrURL string) (string, int, error) {
	// Try agent bead fields first.
	if iidStr := agentBead.Fields["gitlab_mr_iid"]; iidStr != "" {
		iid, err := strconv.Atoi(iidStr)
		if err == nil {
			// Parse the project path from the MR URL.
			ref := ParseMRURL(mrURL)
			if ref != nil {
				return ref.ProjectPath, iid, nil
			}
		}
	}

	// Fall back to parsing the MR URL.
	ref := ParseMRURL(mrURL)
	if ref == nil {
		return "", 0, fmt.Errorf("cannot parse MR URL: %s", mrURL)
	}
	return ref.ProjectPath, ref.IID, nil
}

// alreadySeen returns true if this bead has already been processed.
func (f *GitLabSquawkForwarder) alreadySeen(beadID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.seen[beadID] {
		return true
	}
	f.seen[beadID] = true
	return false
}
