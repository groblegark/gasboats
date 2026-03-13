// Package bridge provides the GitLab MR agent resolver.
//
// AgentResolver handles agent lookup and reuse for MR review comments.
// When a review comment arrives on an agent-authored MR, it finds the original
// agent (or spawns a new one) and delivers the review context.
package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"gasboat/controller/internal/beadsapi"
)

// AgentResolverClient is the subset of beadsapi.Client used by AgentResolver.
type AgentResolverClient interface {
	FindAgentBead(ctx context.Context, agentName string) (*beadsapi.BeadDetail, error)
	SpawnAgent(ctx context.Context, agentName, project, taskID, role, customPrompt string, extraFields ...map[string]string) (string, error)
	UpdateBeadFields(ctx context.Context, beadID string, fields map[string]string) error
}

// AgentResolver looks up or spawns agents for MR review comment handling.
type AgentResolver struct {
	daemon AgentResolverClient
	gitlab *GitLabClient
	client *http.Client
	logger *slog.Logger
}

// AgentResolverConfig holds configuration for the AgentResolver.
type AgentResolverConfig struct {
	Daemon AgentResolverClient
	GitLab *GitLabClient
	Client *http.Client // HTTP client for nudge delivery
	Logger *slog.Logger
}

// NewAgentResolver creates a new AgentResolver.
func NewAgentResolver(cfg AgentResolverConfig) *AgentResolver {
	return &AgentResolver{
		daemon: cfg.Daemon,
		gitlab: cfg.GitLab,
		client: cfg.Client,
		logger: cfg.Logger,
	}
}

// ResolveAndNudge finds the agent assigned to the given task bead and delivers
// a nudge with the review comment context. If the agent is dead (no active
// agent bead found), it spawns a new agent pre-assigned to the task with the
// MR branch context.
//
// The taskBead must have a non-empty Assignee field (the agent name) and an
// mr_url field pointing to the GitLab MR.
func (r *AgentResolver) ResolveAndNudge(ctx context.Context, taskBead BeadEvent, message string) error {
	agentName := taskBead.Assignee
	if agentName == "" {
		return fmt.Errorf("task bead %s has no assignee", taskBead.ID)
	}

	mrURL := taskBead.Fields["mr_url"]

	// Try to find the existing agent bead.
	agentBead, err := r.daemon.FindAgentBead(ctx, agentName)
	if err == nil && agentBead != nil {
		// Agent exists — check if it has a coop_url (alive).
		coopURL := beadsapi.ParseNotes(agentBead.Notes)["coop_url"]
		if coopURL == "" {
			// Also check fields (coop_url may be stored as a field).
			coopURL = agentBead.Fields["coop_url"]
		}
		if coopURL != "" {
			r.logger.Info("agent alive, nudging for MR review",
				"agent", agentName, "task", taskBead.ID, "mr_url", mrURL)

			// Store MR binding metadata on the agent bead (best-effort).
			r.setMRBindingFields(ctx, agentBead.ID, taskBead)

			return nudgeCoop(ctx, r.client, coopURL, message)
		}
		r.logger.Info("agent bead exists but has no coop_url, treating as dead",
			"agent", agentName, "task", taskBead.ID)
	}

	// Agent is dead or not found — spawn a new one.
	r.logger.Info("spawning new agent for MR review comment",
		"agent", agentName, "task", taskBead.ID, "mr_url", mrURL)

	return r.spawnAgentForMR(ctx, agentName, taskBead)
}

// spawnAgentForMR creates a new agent bead pre-assigned to the task with MR
// context. The agent is spawned with the same name so the entrypoint can find
// existing session data (PVC, JSONL) for session continuity.
func (r *AgentResolver) spawnAgentForMR(ctx context.Context, agentName string, taskBead BeadEvent) error {
	// Resolve project from task bead labels.
	project := projectFromLabels(taskBead.Labels)

	// Build the custom prompt with MR context.
	mrURL := taskBead.Fields["mr_url"]
	prompt := fmt.Sprintf(
		"You have been respawned to address review comments on MR %s (task %s: %s). "+
			"Check out the MR branch, read the review comments, and address the feedback. "+
			"Use `kd show %s` to see the full task context and comments.",
		mrURL, taskBead.ID, taskBead.Title, taskBead.ID)

	// Build extra fields with MR binding metadata.
	extraFields := r.buildMRFields(taskBead)
	extraFields["spawn_source"] = "gitlab-mr-review"

	agentBeadID, err := r.daemon.SpawnAgent(ctx, agentName, project, taskBead.ID, "crew", prompt, extraFields)
	if err != nil {
		return fmt.Errorf("spawn agent %q for MR review: %w", agentName, err)
	}

	r.logger.Info("spawned agent for MR review",
		"agent", agentName, "agent_bead", agentBeadID,
		"task", taskBead.ID, "mr_url", mrURL, "project", project)
	return nil
}

// setMRBindingFields stores GitLab MR metadata on the agent bead so the agent
// knows which MR/branch to work on. Best-effort: errors are logged but not
// returned.
func (r *AgentResolver) setMRBindingFields(ctx context.Context, agentBeadID string, taskBead BeadEvent) {
	fields := r.buildMRFields(taskBead)
	if len(fields) == 0 {
		return
	}
	if err := r.daemon.UpdateBeadFields(ctx, agentBeadID, fields); err != nil {
		r.logger.Warn("failed to set MR binding fields on agent bead",
			"agent_bead", agentBeadID, "error", err)
	}
}

// buildMRFields extracts GitLab MR metadata from a task bead into a field map
// suitable for storing on the agent bead.
func (r *AgentResolver) buildMRFields(taskBead BeadEvent) map[string]string {
	fields := make(map[string]string)

	if mrURL := taskBead.Fields["mr_url"]; mrURL != "" {
		fields["gitlab_mr_url"] = mrURL
	}
	if pid := taskBead.Fields["gitlab_project_id"]; pid != "" {
		fields["gitlab_project_id"] = pid
	}
	if iid := taskBead.Fields["gitlab_mr_iid"]; iid != "" {
		fields["gitlab_mr_iid"] = iid
	}

	// Fetch MR details from GitLab to get the source branch.
	if r.gitlab != nil {
		if mrURL := taskBead.Fields["mr_url"]; mrURL != "" {
			ref := ParseMRURL(mrURL)
			if ref != nil {
				mr, err := r.gitlab.GetMergeRequestByPath(context.Background(), ref.ProjectPath, ref.IID)
				if err == nil && mr != nil {
					fields["gitlab_mr_source_branch"] = mr.SourceBranch
					// Backfill project_id and iid if not already set.
					if fields["gitlab_project_id"] == "" {
						fields["gitlab_project_id"] = strconv.Itoa(mr.ProjectID)
					}
					if fields["gitlab_mr_iid"] == "" {
						fields["gitlab_mr_iid"] = strconv.Itoa(mr.IID)
					}
				} else if err != nil {
					r.logger.Warn("failed to fetch MR details for source branch",
						"mr_url", mrURL, "error", err)
				}
			}
		}
	}

	return fields
}

