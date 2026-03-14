// Package bridge provides the GitLab MR merge watcher.
//
// GitLabSync subscribes to kbeads SSE bead events, watches for beads with
// mr_url fields pointing to GitLab MRs, and queries GitLab to detect merges.
// When an MR is merged, it sets mr_merged=true on the bead.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"gasboat/controller/internal/beadsapi"
)

// GitLabBeadClient is the subset of beadsapi.Client used by GitLabSync.
type GitLabBeadClient interface {
	ListTaskBeads(ctx context.Context) ([]*beadsapi.BeadDetail, error)
	UpdateBeadFields(ctx context.Context, beadID string, fields map[string]string) error
	AddComment(ctx context.Context, beadID, author, text string) error
}

// GitLabSync watches for MR merges and updates bead fields.
type GitLabSync struct {
	gitlab   *GitLabClient
	daemon   GitLabBeadClient
	logger   *slog.Logger
	nudge    NudgeFunc
	resolver *AgentResolver // optional; when set, handles agent lookup/reuse for review nudges

	mu   sync.Mutex
	seen map[string]time.Time // dedup key → last check time
}

// NudgeFunc is an optional callback for sending nudges to agents.
type NudgeFunc func(ctx context.Context, agentName, message string) error

// GitLabSyncConfig holds configuration for the GitLab sync watcher.
type GitLabSyncConfig struct {
	GitLab   *GitLabClient
	Daemon   GitLabBeadClient
	Logger   *slog.Logger
	Nudge    NudgeFunc      // optional; if nil, nudges are skipped
	Resolver *AgentResolver // optional; when set, handles agent lookup/reuse for review nudges
}

// NewGitLabSync creates a new GitLab MR sync watcher.
func NewGitLabSync(cfg GitLabSyncConfig) *GitLabSync {
	return &GitLabSync{
		gitlab:   cfg.GitLab,
		daemon:   cfg.Daemon,
		logger:   cfg.Logger,
		nudge:    cfg.Nudge,
		resolver: cfg.Resolver,
		seen:     make(map[string]time.Time),
	}
}

// RegisterHandlers registers SSE event handlers on the given stream.
// Watches for bead updates where mr_url is set — triggers MR status check,
// MR description sync, and review comment nudges.
func (s *GitLabSync) RegisterHandlers(stream *SSEStream) {
	stream.On("beads.bead.updated", s.handleUpdated)
	stream.On("beads.bead.updated", s.handleDescriptionSync)
	stream.On("beads.bead.updated", s.handleReviewNudge)
	s.logger.Info("GitLab sync watcher registered SSE handlers",
		"topics", []string{"beads.bead.updated"})
}

// handleUpdated checks if a bead's mr_url points to a GitLab MR and queries
// its merge status. If merged, sets mr_merged=true on the bead.
func (s *GitLabSync) handleUpdated(ctx context.Context, data []byte) {
	bead := ParseBeadEvent(data)
	if bead == nil {
		return
	}

	mrURL := bead.Fields["mr_url"]
	if mrURL == "" {
		return
	}

	// Only process GitLab MR URLs.
	ref := ParseMRURL(mrURL)
	if ref == nil {
		return
	}

	// Skip if already marked as merged.
	if bead.Fields["mr_merged"] == "true" {
		return
	}

	// Dedup: don't re-check the same MR too frequently.
	dedupKey := "gitlab-mr:" + bead.ID + ":" + mrURL
	if s.isDuplicate(dedupKey) {
		return
	}

	s.logger.Info("checking GitLab MR status",
		"bead", bead.ID, "mr_url", mrURL, "project", ref.ProjectPath, "iid", ref.IID)

	mr, err := s.gitlab.GetMergeRequestByPath(ctx, ref.ProjectPath, ref.IID)
	if err != nil {
		s.logger.Error("failed to get GitLab MR",
			"bead", bead.ID, "mr_url", mrURL, "error", err)
		return
	}

	// Update bead with MR state regardless of merge status.
	fields := map[string]string{
		"mr_state":          mr.State,
		"gitlab_mr_iid":     strconv.Itoa(mr.IID),
		"gitlab_project_id": strconv.Itoa(mr.ProjectID),
	}
	if mr.HeadPipeline != nil {
		fields["mr_pipeline_status"] = mr.HeadPipeline.Status
		if mr.HeadPipeline.WebURL != "" {
			fields["mr_pipeline_url"] = mr.HeadPipeline.WebURL
		}
	}
	if mr.State == "merged" {
		fields["mr_merged"] = "true"
		s.logger.Info("MR merged — updating bead",
			"bead", bead.ID, "mr_url", mrURL)
	}

	if err := s.daemon.UpdateBeadFields(ctx, bead.ID, fields); err != nil {
		s.logger.Error("failed to update bead fields",
			"bead", bead.ID, "error", err)
	}
}

// mrDescFields are the bead fields that feed into the MR description section.
var mrDescFields = map[string]bool{
	"jira_key":            true,
	"jira_status":         true,
	"mr_state":            true,
	"mr_pipeline_status":  true,
	"mr_pipeline_url":     true,
	"mr_approved":         true,
	"mr_approvers":        true,
}

// handleDescriptionSync updates the gasboat-managed section of the GitLab MR
// description when relevant bead fields change.
func (s *GitLabSync) handleDescriptionSync(ctx context.Context, data []byte) {
	if s.gitlab == nil {
		return
	}

	bead := ParseBeadEvent(data)
	if bead == nil || bead.Type != "task" {
		return
	}

	mrURL := bead.Fields["mr_url"]
	if mrURL == "" {
		return
	}

	ref := ParseMRURL(mrURL)
	if ref == nil {
		return
	}

	// Only sync when a relevant field changed.
	if bead.Changes != nil {
		relevant := false
		for k := range bead.Changes {
			if mrDescFields[k] {
				relevant = true
				break
			}
		}
		if !relevant {
			return
		}
	}

	// Dedup: don't re-sync too frequently.
	dedupKey := "gitlab-mrdesc:" + bead.ID
	if s.isDuplicate(dedupKey) {
		return
	}

	section := MRDescriptionSection{
		BeadID:         bead.ID,
		JIRAKey:        bead.Fields["jira_key"],
		JIRAStatus:     bead.Fields["jira_status"],
		PipelineStatus: bead.Fields["mr_pipeline_status"],
		PipelineURL:    bead.Fields["mr_pipeline_url"],
		Approved:       bead.Fields["mr_approved"],
		Approvers:      bead.Fields["mr_approvers"],
		MRState:        bead.Fields["mr_state"],
	}

	if err := syncMRDescription(ctx, s.gitlab, ref.ProjectPath, ref.IID, section, s.logger); err != nil {
		s.logger.Error("failed to sync MR description",
			"bead", bead.ID, "mr_url", mrURL, "error", err)
	}
}

// handleReviewNudge nudges the assigned agent when mr_has_review_comments
// changes to true on a bead. When an AgentResolver is configured, it handles
// agent lookup/reuse: if the original agent is alive it gets nudged; if dead,
// a new agent is spawned with the MR context.
func (s *GitLabSync) handleReviewNudge(ctx context.Context, data []byte) {
	if s.nudge == nil && s.resolver == nil {
		return
	}

	bead := ParseBeadEvent(data)
	if bead == nil || bead.Type != "task" {
		return
	}

	// Only nudge when mr_has_review_comments just changed.
	if bead.Changes == nil {
		return
	}
	if _, ok := bead.Changes["mr_has_review_comments"]; !ok {
		return
	}
	if bead.Fields["mr_has_review_comments"] != "true" {
		return
	}

	assignee := bead.Assignee
	if assignee == "" {
		return
	}

	// Dedup: don't nudge the same agent too frequently for the same bead.
	dedupKey := "gitlab-review-nudge:" + bead.ID
	if s.isDuplicate(dedupKey) {
		return
	}

	message := fmt.Sprintf("MR has new review comments — address them: %s", bead.Fields["mr_url"])

	// When the resolver is available, use it for agent lookup/reuse.
	// This handles dead agents by spawning new ones with MR context.
	if s.resolver != nil {
		if err := s.resolver.ResolveAndNudge(ctx, *bead, message); err != nil {
			s.logger.Error("failed to resolve/nudge agent for review comments",
				"bead", bead.ID, "agent", assignee, "error", err)
		} else {
			s.logger.Info("resolved and nudged agent for MR review comments",
				"bead", bead.ID, "agent", assignee)
		}
		return
	}

	// Fallback: simple nudge by agent name (no agent reuse).
	if err := s.nudge(ctx, assignee, message); err != nil {
		s.logger.Error("failed to nudge agent for review comments",
			"bead", bead.ID, "agent", assignee, "error", err)
	} else {
		s.logger.Info("nudged agent for MR review comments",
			"bead", bead.ID, "agent", assignee)
	}
}

// isDuplicate returns true if the key was seen within the last 5 minutes.
func (s *GitLabSync) isDuplicate(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	// Periodic cleanup.
	for k, t := range s.seen {
		if now.Sub(t) > 5*time.Minute {
			delete(s.seen, k)
		}
	}

	if t, ok := s.seen[key]; ok && now.Sub(t) < 5*time.Minute {
		return true
	}
	s.seen[key] = now
	return false
}

// GitLabPoller periodically polls GitLab for recently merged MRs and updates
// matching beads. This is the fallback for when webhooks don't fire.
type GitLabPoller struct {
	gitlab       *GitLabClient
	daemon       GitLabBeadClient
	logger       *slog.Logger
	groupID      int
	pollInterval time.Duration
	lastPoll     time.Time
}

// GitLabPollerConfig holds configuration for the GitLab poller.
type GitLabPollerConfig struct {
	GitLab       *GitLabClient
	Daemon       GitLabBeadClient
	Logger       *slog.Logger
	GroupID      int
	PollInterval time.Duration
}

// NewGitLabPoller creates a new GitLab MR polling fallback.
func NewGitLabPoller(cfg GitLabPollerConfig) *GitLabPoller {
	return &GitLabPoller{
		gitlab:       cfg.GitLab,
		daemon:       cfg.Daemon,
		logger:       cfg.Logger,
		groupID:      cfg.GroupID,
		pollInterval: cfg.PollInterval,
		lastPoll:     time.Now().Add(-cfg.PollInterval), // poll immediately on first run
	}
}

// Run starts the polling loop. It blocks until ctx is canceled.
func (p *GitLabPoller) Run(ctx context.Context) error {
	p.logger.Info("starting GitLab MR poller",
		"group_id", p.groupID, "poll_interval", p.pollInterval)

	// Initial poll.
	p.poll(ctx)

	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

// poll checks GitLab for recently merged MRs and updates matching beads.
func (p *GitLabPoller) poll(ctx context.Context) {
	since := p.lastPoll
	p.lastPoll = time.Now()

	mrs, err := p.gitlab.ListMergedMRs(ctx, p.groupID, since)
	if err != nil {
		p.logger.Error("GitLab poll failed", "error", err)
		return
	}

	if len(mrs) == 0 {
		p.logger.Debug("no recently merged MRs", "since", since)
		return
	}

	p.logger.Info("found recently merged MRs", "count", len(mrs), "since", since)

	// Load beads with mr_url to match against.
	beads, err := p.daemon.ListTaskBeads(ctx)
	if err != nil {
		p.logger.Error("failed to list task beads", "error", err)
		return
	}

	// Build mr_url → bead index.
	urlIndex := make(map[string]*beadsapi.BeadDetail)
	for _, b := range beads {
		if u := b.Fields["mr_url"]; u != "" {
			urlIndex[u] = b
		}
	}

	// Match merged MRs to beads.
	for _, mr := range mrs {
		bead, ok := urlIndex[mr.WebURL]
		if !ok {
			continue
		}

		// Skip already-merged beads.
		if bead.Fields["mr_merged"] == "true" {
			continue
		}

		p.logger.Info("poll: MR merged — updating bead",
			"bead", bead.ID, "mr_url", mr.WebURL, "mr_iid", mr.IID)

		fields := map[string]string{
			"mr_merged":         "true",
			"mr_state":          "merged",
			"gitlab_mr_iid":     strconv.Itoa(mr.IID),
			"gitlab_project_id": strconv.Itoa(mr.ProjectID),
		}
		if mr.HeadPipeline != nil {
			fields["mr_pipeline_status"] = mr.HeadPipeline.Status
			if mr.HeadPipeline.WebURL != "" {
				fields["mr_pipeline_url"] = mr.HeadPipeline.WebURL
			}
		}

		if err := p.daemon.UpdateBeadFields(ctx, bead.ID, fields); err != nil {
			p.logger.Error("failed to update bead fields from poll",
				"bead", bead.ID, "error", err)
		}
	}
}

// GitLabWebhookConfig holds configuration for the GitLab webhook handler.
type GitLabWebhookConfig struct {
	GitLab        *GitLabClient
	Daemon        GitLabBeadClient
	WebhookSecret string
	BotUsername   string            // GitLab username of the bot; notes from this user are ignored to prevent loops
	Nudge         NudgeFunc         // optional; if set, review comments nudge the assigned agent with rich context
	AgentResolver *AgentResolver    // optional; if set, spawns agents for review comments when no running agent found
	Logger        *slog.Logger
}

// GitLabWebhookHandler returns an http.Handler that processes GitLab webhook
// events for merge request merges.
func GitLabWebhookHandler(gitlab *GitLabClient, daemon GitLabBeadClient, webhookSecret string, logger *slog.Logger) http.Handler {
	return GitLabWebhookHandlerWithConfig(GitLabWebhookConfig{
		GitLab:        gitlab,
		Daemon:        daemon,
		WebhookSecret: webhookSecret,
		Logger:        logger,
	})
}

// GitLabWebhookHandlerWithConfig returns an http.Handler using the full config.
func GitLabWebhookHandlerWithConfig(cfg GitLabWebhookConfig) http.Handler {
	daemon := cfg.Daemon
	logger := cfg.Logger
	webhookSecret := cfg.WebhookSecret
	botUsername := cfg.BotUsername
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify webhook secret.
		if r.Header.Get("X-Gitlab-Token") != webhookSecret {
			logger.Warn("webhook: invalid X-Gitlab-Token")
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		var event struct {
			ObjectKind string `json:"object_kind"`
			User       struct {
				Username string `json:"username"`
			} `json:"user"`
			ObjectAttr struct {
				IID       int    `json:"iid"`
				State     string `json:"state"`
				Action    string `json:"action"`
				URL       string `json:"url"`
				ProjectID int    `json:"target_project_id"`
				// Pipeline-specific fields (when object_kind=pipeline).
				ID     int    `json:"id"`
				Status string `json:"status"`
				// Note-specific fields (when object_kind=note).
				Note         string `json:"note"`
				NoteableType string `json:"noteable_type"`
				System       bool   `json:"system"`
				DiscussionID string `json:"discussion_id"`
				Position     *struct {
					NewPath string `json:"new_path"`
					NewLine int    `json:"new_line"`
					OldPath string `json:"old_path"`
					OldLine int    `json:"old_line"`
				} `json:"position"`
			} `json:"object_attributes"`
			MergeRequest *struct {
				IID   int    `json:"iid"`
				URL   string `json:"url"`
				Title string `json:"title"`
			} `json:"merge_request"`
		}

		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			logger.Error("webhook: failed to decode body", "error", err)
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		switch event.ObjectKind {
		case "note":
			var pos *notePosition
			if event.ObjectAttr.Position != nil {
				pos = &notePosition{
					NewPath: event.ObjectAttr.Position.NewPath,
					NewLine: event.ObjectAttr.Position.NewLine,
					OldPath: event.ObjectAttr.Position.OldPath,
					OldLine: event.ObjectAttr.Position.OldLine,
				}
			}
			nc := noteContext{
				NoteableType: event.ObjectAttr.NoteableType,
				Note:         event.ObjectAttr.Note,
				System:       event.ObjectAttr.System,
				Author:       event.User.Username,
				BotUsername:   botUsername,
				Position:     pos,
				DiscussionID: event.ObjectAttr.DiscussionID,
				MR:           event.MergeRequest,
			}
			handleNoteWebhook(r.Context(), nc, cfg.Nudge, cfg.AgentResolver, daemon, logger)
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"status":"processed","kind":"note"}`)
			return
		case "pipeline":
			handlePipelineWebhook(r.Context(), event.ObjectAttr.ID, event.ObjectAttr.Status, event.ObjectAttr.URL, event.MergeRequest, daemon, logger)
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"status":"processed","kind":"pipeline"}`)
			return
		case "merge_request":
			switch event.ObjectAttr.Action {
			case "merge":
				// Fall through to merge handling below.
			case "approved", "unapproved":
				handleApprovalWebhook(r.Context(), event.ObjectAttr.URL, event.ObjectAttr.Action, event.User.Username, daemon, logger)
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, `{"status":"processed","action":"%s"}`, event.ObjectAttr.Action)
				return
			default:
				w.WriteHeader(http.StatusOK)
				fmt.Fprintf(w, `{"status":"ignored","reason":"action=%s"}`, event.ObjectAttr.Action)
				return
			}
		default:
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"status":"ignored","reason":"kind=%s"}`, event.ObjectKind)
			return
		}

		logger.Info("webhook: MR merged",
			"mr_url", event.ObjectAttr.URL,
			"iid", event.ObjectAttr.IID,
			"project_id", event.ObjectAttr.ProjectID)

		// Find matching bead by mr_url.
		ctx := r.Context()
		beads, err := daemon.ListTaskBeads(ctx)
		if err != nil {
			logger.Error("webhook: failed to list beads", "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		matched := false
		for _, bead := range beads {
			if bead.Fields["mr_url"] == event.ObjectAttr.URL {
				if bead.Fields["mr_merged"] == "true" {
					continue // already processed
				}
				logger.Info("webhook: updating bead for merged MR",
					"bead", bead.ID, "mr_url", event.ObjectAttr.URL)

				fields := map[string]string{
					"mr_merged":         "true",
					"mr_state":          "merged",
					"gitlab_mr_iid":     strconv.Itoa(event.ObjectAttr.IID),
					"gitlab_project_id": strconv.Itoa(event.ObjectAttr.ProjectID),
				}
				if err := daemon.UpdateBeadFields(ctx, bead.ID, fields); err != nil {
					logger.Error("webhook: failed to update bead",
						"bead", bead.ID, "error", err)
				}
				matched = true
			}
		}

		w.WriteHeader(http.StatusOK)
		if matched {
			fmt.Fprintf(w, `{"status":"processed"}`)
		} else {
			fmt.Fprintf(w, `{"status":"no_match","mr_url":"%s"}`, event.ObjectAttr.URL)
		}
	})
}

// handlePipelineWebhook processes a GitLab pipeline webhook event. It matches
// the pipeline's MR URL to a bead and updates the pipeline status fields.
func handlePipelineWebhook(ctx context.Context, pipelineID int, status, pipelineURL string, mr *struct {
	IID   int    `json:"iid"`
	URL   string `json:"url"`
	Title string `json:"title"`
}, daemon GitLabBeadClient, logger *slog.Logger) {
	if mr == nil || mr.URL == "" {
		logger.Debug("webhook: pipeline event has no merge_request, skipping")
		return
	}

	logger.Info("webhook: pipeline status update",
		"pipeline_id", pipelineID, "status", status, "mr_url", mr.URL)

	beads, err := daemon.ListTaskBeads(ctx)
	if err != nil {
		logger.Error("webhook: failed to list beads for pipeline event", "error", err)
		return
	}

	for _, bead := range beads {
		if bead.Fields["mr_url"] != mr.URL {
			continue
		}

		fields := map[string]string{
			"mr_pipeline_status": status,
		}
		if pipelineURL != "" {
			fields["mr_pipeline_url"] = pipelineURL
		}

		if err := daemon.UpdateBeadFields(ctx, bead.ID, fields); err != nil {
			logger.Error("webhook: failed to update pipeline status on bead",
				"bead", bead.ID, "error", err)
		} else {
			logger.Info("webhook: updated pipeline status on bead",
				"bead", bead.ID, "status", status, "pipeline_id", pipelineID)
		}
	}
}

// handleApprovalWebhook processes MR approved/unapproved webhook events.
func handleApprovalWebhook(ctx context.Context, mrURL, action, username string, daemon GitLabBeadClient, logger *slog.Logger) {
	if mrURL == "" {
		return
	}

	logger.Info("webhook: MR approval event",
		"mr_url", mrURL, "action", action, "user", username)

	beads, err := daemon.ListTaskBeads(ctx)
	if err != nil {
		logger.Error("webhook: failed to list beads for approval event", "error", err)
		return
	}

	for _, bead := range beads {
		if bead.Fields["mr_url"] != mrURL {
			continue
		}

		fields := map[string]string{}

		switch action {
		case "approved":
			fields["mr_approved"] = "true"
			// Append approver to comma-separated list.
			existing := bead.Fields["mr_approvers"]
			if existing == "" {
				fields["mr_approvers"] = username
			} else if !containsApprover(existing, username) {
				fields["mr_approvers"] = existing + "," + username
			}
		case "unapproved":
			fields["mr_approved"] = "false"
			// Remove approver from list.
			fields["mr_approvers"] = removeApprover(bead.Fields["mr_approvers"], username)
		}

		if err := daemon.UpdateBeadFields(ctx, bead.ID, fields); err != nil {
			logger.Error("webhook: failed to update approval status on bead",
				"bead", bead.ID, "error", err)
		} else {
			logger.Info("webhook: updated approval status on bead",
				"bead", bead.ID, "action", action, "user", username)
		}
	}
}

// notePosition mirrors the GitLab note position object for inline review comments.
type notePosition struct {
	NewPath string `json:"new_path"`
	NewLine int    `json:"new_line"`
	OldPath string `json:"old_path"`
	OldLine int    `json:"old_line"`
}

// noteContext holds parsed data from a GitLab note webhook event.
type noteContext struct {
	NoteableType string
	Note         string
	System       bool
	Author       string
	BotUsername  string
	Position     *notePosition
	DiscussionID string
	MR           *struct {
		IID   int    `json:"iid"`
		URL   string `json:"url"`
		Title string `json:"title"`
	}
}

// handleNoteWebhook processes a GitLab note webhook event. It matches
// MR review comments to beads, creates bead comments with the review feedback,
// and nudges the assigned agent with a rich message containing file path, line,
// diff context, and discussion ID. If no running agent is found and an
// AgentResolver is configured, it spawns a new agent to handle the review.
// Notes from the bot user are skipped to prevent feedback loops.
func handleNoteWebhook(ctx context.Context, nc noteContext, nudge NudgeFunc, resolver *AgentResolver, daemon GitLabBeadClient, logger *slog.Logger) {
	// Only process MR discussion notes.
	if nc.NoteableType != "MergeRequest" {
		logger.Debug("webhook: note event is not for MergeRequest, skipping",
			"noteable_type", nc.NoteableType)
		return
	}

	// Skip system-generated notes (merge status changes, etc.).
	if nc.System {
		logger.Debug("webhook: skipping system note")
		return
	}

	// Skip notes from the bot user to prevent feedback loops.
	if nc.BotUsername != "" && nc.Author == nc.BotUsername {
		logger.Debug("webhook: skipping note from bot user",
			"author", nc.Author, "bot_username", nc.BotUsername)
		return
	}

	if nc.MR == nil || nc.MR.URL == "" {
		logger.Debug("webhook: note event has no merge_request, skipping")
		return
	}

	logger.Info("webhook: MR review comment",
		"mr_url", nc.MR.URL, "author", nc.Author)

	beads, err := daemon.ListTaskBeads(ctx)
	if err != nil {
		logger.Error("webhook: failed to list beads for note event", "error", err)
		return
	}

	// Format the comment text for the bead.
	var commentText strings.Builder
	commentText.WriteString(fmt.Sprintf("**GitLab review comment** by @%s:\n\n", nc.Author))
	if nc.Position != nil && nc.Position.NewPath != "" {
		commentText.WriteString(fmt.Sprintf("`%s:%d`\n\n", nc.Position.NewPath, nc.Position.NewLine))
	}
	commentText.WriteString(nc.Note)

	for _, bead := range beads {
		if bead.Fields["mr_url"] != nc.MR.URL {
			continue
		}

		// Add comment to the bead.
		if err := daemon.AddComment(ctx, bead.ID, "gitlab-bridge", commentText.String()); err != nil {
			logger.Error("webhook: failed to add review comment to bead",
				"bead", bead.ID, "error", err)
			continue
		}

		// Set mr_has_review_comments flag.
		fields := map[string]string{
			"mr_has_review_comments": "true",
		}
		if err := daemon.UpdateBeadFields(ctx, bead.ID, fields); err != nil {
			logger.Error("webhook: failed to set review comment flag on bead",
				"bead", bead.ID, "error", err)
		} else {
			logger.Info("webhook: added review comment to bead",
				"bead", bead.ID, "author", nc.Author, "mr_url", nc.MR.URL)
		}

		// Nudge the assigned agent with rich context.
		nudged := false
		if nudge != nil && bead.Assignee != "" {
			msg := buildReviewNudgeMessage(nc, bead.ID)
			if err := nudge(ctx, bead.Assignee, msg); err != nil {
				logger.Error("webhook: failed to nudge agent for review comment",
					"bead", bead.ID, "agent", bead.Assignee, "error", err)
			} else {
				logger.Info("webhook: nudged agent for review comment",
					"bead", bead.ID, "agent", bead.Assignee, "author", nc.Author)
				nudged = true
			}
		}

		// Fallback: if nudge didn't work and we have an agent resolver,
		// try to find or spawn an agent for this MR review comment.
		if !nudged && resolver != nil {
			taskEvent := BeadEvent{
				ID:       bead.ID,
				Type:     bead.Type,
				Title:    bead.Title,
				Assignee: bead.Assignee,
				Labels:   bead.Labels,
				Fields:   bead.Fields,
			}
			msg := buildReviewNudgeMessage(nc, bead.ID)
			if err := resolver.ResolveAndNudge(ctx, taskEvent, msg); err != nil {
				logger.Warn("webhook: agent resolver failed for review comment",
					"bead", bead.ID, "mr_url", nc.MR.URL, "error", err)
			} else {
				logger.Info("webhook: dispatched review via agent resolver",
					"bead", bead.ID, "mr_url", nc.MR.URL, "author", nc.Author)
			}
		}
	}
}

// buildReviewNudgeMessage constructs a rich nudge message for an agent from a
// GitLab review comment, including file path, line number, diff context,
// discussion ID, and MR reference.
func buildReviewNudgeMessage(nc noteContext, beadID string) string {
	var b strings.Builder
	b.WriteString("GitLab review comment on MR")
	if nc.MR.Title != "" {
		b.WriteString(fmt.Sprintf(" \"%s\"", nc.MR.Title))
	}
	b.WriteString(fmt.Sprintf(" by @%s", nc.Author))
	if nc.Position != nil && nc.Position.NewPath != "" {
		b.WriteString(fmt.Sprintf("\nFile: %s", nc.Position.NewPath))
		if nc.Position.NewLine > 0 {
			b.WriteString(fmt.Sprintf(":%d", nc.Position.NewLine))
		}
		if nc.Position.OldPath != "" && nc.Position.OldPath != nc.Position.NewPath {
			b.WriteString(fmt.Sprintf(" (was %s)", nc.Position.OldPath))
		}
		if nc.Position.OldLine > 0 {
			b.WriteString(fmt.Sprintf(" [old line %d]", nc.Position.OldLine))
		}
	}
	b.WriteString(fmt.Sprintf("\nComment: %s", nc.Note))
	if nc.DiscussionID != "" {
		b.WriteString(fmt.Sprintf("\nDiscussion ID: %s", nc.DiscussionID))
	}
	if nc.MR.URL != "" {
		b.WriteString(fmt.Sprintf("\nMR: %s", nc.MR.URL))
	}
	b.WriteString(fmt.Sprintf("\nBead: %s", beadID))
	return b.String()
}

// containsApprover checks if username is in the comma-separated approvers list.
func containsApprover(approvers, username string) bool {
	for _, a := range strings.Split(approvers, ",") {
		if strings.TrimSpace(a) == username {
			return true
		}
	}
	return false
}

// removeApprover removes username from the comma-separated approvers list.
func removeApprover(approvers, username string) string {
	var result []string
	for _, a := range strings.Split(approvers, ",") {
		a = strings.TrimSpace(a)
		if a != "" && a != username {
			result = append(result, a)
		}
	}
	return strings.Join(result, ",")
}
