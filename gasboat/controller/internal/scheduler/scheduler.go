// Package scheduler evaluates cron-based schedule beads and spawns agents
// on matching triggers. It reads schedule beads from the daemon, evaluates
// cron expressions against wall-clock time, and calls SpawnAgent for any
// schedule whose next-fire time has passed since last_run.
package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"gasboat/controller/internal/beadsapi"
)

// Scheduler evaluates schedule beads and spawns agents on cron triggers.
type Scheduler struct {
	daemon *beadsapi.Client
	logger *slog.Logger
	mu     sync.Mutex
}

// New creates a Scheduler.
func New(daemon *beadsapi.Client, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		daemon: daemon,
		logger: logger,
	}
}

// Schedule represents a parsed schedule bead.
type Schedule struct {
	ID           string
	Title        string
	Cron         string
	Project      string
	Role         string
	Prompt       string
	Enabled      bool
	Timezone     string
	SlackChannel string // optional override: Slack channel for notifications
	LastRun      time.Time
	LastAgentID  string

	// Guard rails.
	MaxConcurrent       int           // max concurrent agents (default 2)
	Timeout             time.Duration // max agent run duration (default 30m)
	RetryOnFailure      bool          // retry on next cycle if agent fails
	MaxRetries          int           // max consecutive failures before auto-disable (default 3)
	ConsecutiveFailures int           // current consecutive failure count
}

// Guard rail defaults.
const (
	defaultMaxConcurrent = 2
	defaultTimeout       = 30 * time.Minute
	defaultMaxRetries    = 3
)

// Reconcile performs a single evaluation pass. For each enabled schedule bead:
//  1. Check and record completion of the last spawned agent
//  2. Parse the cron expression
//  3. Calculate the most recent fire time before now
//  4. Check guard rails (concurrency, failure tracking)
//  5. If fire time is after last_run and guard rails pass, spawn an agent
//  6. Enforce timeouts on running scheduled agents
func (s *Scheduler) Reconcile(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	schedules, err := s.listSchedules(ctx)
	if err != nil {
		return fmt.Errorf("listing schedules: %w", err)
	}

	if len(schedules) == 0 {
		return nil
	}

	now := time.Now()

	for i := range schedules {
		sched := &schedules[i]

		// Track last agent status for failure counting and record run history.
		s.trackLastAgentStatus(ctx, sched)

		if !sched.Enabled {
			continue
		}
		if sched.Cron == "" {
			s.logger.Warn("schedule has empty cron expression", "id", sched.ID, "title", sched.Title)
			continue
		}

		loc := time.UTC
		if sched.Timezone != "" {
			parsed, err := time.LoadLocation(sched.Timezone)
			if err != nil {
				s.logger.Warn("invalid timezone on schedule, using UTC",
					"id", sched.ID, "timezone", sched.Timezone, "error", err)
			} else {
				loc = parsed
			}
		}

		expr, err := ParseCron(sched.Cron)
		if err != nil {
			s.logger.Warn("invalid cron expression on schedule",
				"id", sched.ID, "cron", sched.Cron, "error", err)
			continue
		}

		nowLocal := now.In(loc)
		lastFire := expr.Prev(nowLocal)
		if lastFire.IsZero() {
			continue
		}

		// If last_run is after or equal to the most recent fire time, skip.
		if !sched.LastRun.IsZero() && !sched.LastRun.Before(lastFire) {
			continue
		}

		// Check concurrency limit.
		if s.isLastAgentRunning(ctx, *sched) {
			s.logger.Warn("schedule at concurrency limit, skipping",
				"id", sched.ID, "title", sched.Title,
				"max_concurrent", sched.MaxConcurrent,
				"last_agent", sched.LastAgentID)
			continue
		}

		s.logger.Info("schedule triggered",
			"id", sched.ID, "title", sched.Title,
			"cron", sched.Cron, "last_fire", lastFire.Format(time.RFC3339))

		agentID, err := s.spawnScheduledAgent(ctx, *sched)
		if err != nil {
			s.logger.Error("failed to spawn scheduled agent",
				"schedule", sched.ID, "error", err)
			_ = s.daemon.AddComment(ctx, sched.ID, "scheduler",
				fmt.Sprintf("Run failed to start: %v", err))
			s.recordFailure(ctx, sched)
			continue
		}

		// Update last_run and last_agent_id on the schedule bead.
		if err := s.daemon.UpdateBeadFields(ctx, sched.ID, map[string]string{
			"last_run":      now.UTC().Format(time.RFC3339),
			"last_agent_id": agentID,
		}); err != nil {
			s.logger.Warn("failed to update schedule last_run",
				"schedule", sched.ID, "error", err)
		}

		// Record spawn in run history.
		_ = s.daemon.AddComment(ctx, sched.ID, "scheduler",
			fmt.Sprintf("Run started → %s", agentID))
	}

	// Enforce timeouts on all running scheduled agents.
	s.enforceTimeouts(ctx, schedules, now)

	return nil
}

// trackLastAgentStatus checks the status of the last spawned agent and updates
// the schedule's consecutive failure count and run history. If the agent
// completed successfully, the failure count is reset. If it failed, the count
// is incremented and the schedule may be auto-disabled. Uses last_checked_agent
// on the schedule bead to avoid re-processing the same agent's result.
func (s *Scheduler) trackLastAgentStatus(ctx context.Context, sched *Schedule) {
	if sched.LastAgentID == "" {
		return
	}

	detail, err := s.daemon.GetBead(ctx, sched.LastAgentID)
	if err != nil {
		return
	}

	// Only process closed agents (terminal state).
	if detail.Status != "closed" {
		return
	}

	// Skip if we already processed this agent's result.
	lastChecked, _ := s.daemon.GetBead(ctx, sched.ID)
	if lastChecked != nil && lastChecked.Fields["last_checked_agent"] == sched.LastAgentID {
		return
	}

	agentState := detail.Fields["agent_state"]

	// Record run history comment with duration.
	var durationStr string
	if !sched.LastRun.IsZero() {
		durationStr = formatRunDuration(time.Since(sched.LastRun))
	}
	var comment string
	switch agentState {
	case "done":
		comment = fmt.Sprintf("Run completed → %s done", sched.LastAgentID)
	case "failed":
		reason := detail.Fields["close_reason"]
		if reason != "" {
			comment = fmt.Sprintf("Run failed → %s failed (%s)", sched.LastAgentID, reason)
		} else {
			comment = fmt.Sprintf("Run failed → %s failed", sched.LastAgentID)
		}
	default:
		comment = fmt.Sprintf("Run ended → %s closed (state: %s)", sched.LastAgentID, agentState)
	}
	if durationStr != "" {
		comment += " [" + durationStr + "]"
	}
	_ = s.daemon.AddComment(ctx, sched.ID, "scheduler", comment)

	// Mark this agent as checked and update failure tracking.
	updateFields := map[string]string{
		"last_checked_agent": sched.LastAgentID,
	}

	if agentState == "failed" {
		sched.ConsecutiveFailures++
		updateFields["consecutive_failures"] = fmt.Sprintf("%d", sched.ConsecutiveFailures)
		if sched.ConsecutiveFailures >= sched.MaxRetries {
			sched.Enabled = false
			updateFields["enabled"] = "false"
			s.logger.Warn("schedule auto-disabled after max consecutive failures",
				"id", sched.ID, "title", sched.Title,
				"consecutive_failures", sched.ConsecutiveFailures,
				"max_retries", sched.MaxRetries)
		}
	} else if agentState == "done" {
		if sched.ConsecutiveFailures > 0 {
			sched.ConsecutiveFailures = 0
			updateFields["consecutive_failures"] = "0"
		}
	}

	_ = s.daemon.UpdateBeadFields(ctx, sched.ID, updateFields)
}

// formatRunDuration formats a duration for run history display.
func formatRunDuration(d time.Duration) string {
	d = d.Truncate(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		m := int(d.Minutes())
		s := int(d.Seconds()) % 60
		if s == 0 {
			return fmt.Sprintf("%dm", m)
		}
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if m == 0 {
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dh %dm", h, m)
}

// recordFailure increments the consecutive failure count and auto-disables
// the schedule if max_retries is reached. Used for spawn failures and
// timeout kills (agent completion failures are tracked in trackLastAgentStatus).
func (s *Scheduler) recordFailure(ctx context.Context, sched *Schedule) {
	sched.ConsecutiveFailures++
	fields := map[string]string{
		"consecutive_failures": fmt.Sprintf("%d", sched.ConsecutiveFailures),
	}

	if sched.ConsecutiveFailures >= sched.MaxRetries {
		sched.Enabled = false
		fields["enabled"] = "false"
		s.logger.Warn("schedule auto-disabled after max consecutive failures",
			"id", sched.ID, "title", sched.Title,
			"consecutive_failures", sched.ConsecutiveFailures,
			"max_retries", sched.MaxRetries)
	}

	if err := s.daemon.UpdateBeadFields(ctx, sched.ID, fields); err != nil {
		s.logger.Warn("failed to update consecutive_failures",
			"schedule", sched.ID, "error", err)
	}
}

// isLastAgentRunning checks if the last spawned agent for this schedule is
// still active (not closed). Returns true if the schedule is at its
// concurrency limit.
func (s *Scheduler) isLastAgentRunning(ctx context.Context, sched Schedule) bool {
	if sched.LastAgentID == "" {
		return false
	}

	detail, err := s.daemon.GetBead(ctx, sched.LastAgentID)
	if err != nil {
		// Can't determine status — allow the spawn.
		return false
	}

	// If the last agent's bead is still open/in_progress, it's running.
	return detail.Status != "closed"
}

// enforceTimeouts checks all schedules for running agents that have exceeded
// their timeout duration. Timed-out agents are closed with agent_state=failed.
func (s *Scheduler) enforceTimeouts(ctx context.Context, schedules []Schedule, now time.Time) {
	for i := range schedules {
		sched := &schedules[i]
		if sched.LastAgentID == "" || sched.LastRun.IsZero() || sched.Timeout <= 0 {
			continue
		}

		// Check if the agent has exceeded the timeout.
		elapsed := now.Sub(sched.LastRun)
		if elapsed <= sched.Timeout {
			continue
		}

		// Verify the agent is still running before killing it.
		detail, err := s.daemon.GetBead(ctx, sched.LastAgentID)
		if err != nil || detail.Status == "closed" {
			continue
		}

		s.logger.Warn("schedule agent timed out, killing",
			"schedule", sched.ID, "title", sched.Title,
			"agent", sched.LastAgentID,
			"elapsed", elapsed.String(), "timeout", sched.Timeout.String())

		// Mark agent as failed and close.
		_ = s.daemon.UpdateBeadFields(ctx, sched.LastAgentID, map[string]string{
			"agent_state":  "failed",
			"close_reason": fmt.Sprintf("timeout after %s (limit: %s)", elapsed.Truncate(time.Second), sched.Timeout),
		})
		_ = s.daemon.CloseBead(ctx, sched.LastAgentID, map[string]string{
			"agent_state": "failed",
		})

		// Record the timeout as a failure.
		s.recordFailure(ctx, sched)
	}
}

// RunLoop runs the scheduler periodically until the context is cancelled.
func (s *Scheduler) RunLoop(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 60 * time.Second
	}

	s.logger.Info("scheduler started", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Run immediately on start.
	if err := s.Reconcile(ctx); err != nil {
		s.logger.Warn("initial schedule reconcile failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler stopped")
			return
		case <-ticker.C:
			if err := s.Reconcile(ctx); err != nil {
				s.logger.Warn("schedule reconcile failed", "error", err)
			}
		}
	}
}

// listSchedules queries the daemon for active schedule beads.
func (s *Scheduler) listSchedules(ctx context.Context) ([]Schedule, error) {
	result, err := s.daemon.ListBeadsFiltered(ctx, beadsapi.ListBeadsQuery{
		Types:    []string{"schedule"},
		Statuses: []string{"open", "in_progress"},
	})
	if err != nil {
		return nil, err
	}

	var schedules []Schedule
	for _, b := range result.Beads {
		enabled := b.Fields["enabled"] == "true"
		var lastRun time.Time
		if raw := b.Fields["last_run"]; raw != "" {
			for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
				if t, err := time.Parse(layout, raw); err == nil {
					lastRun = t
					break
				}
			}
		}
		schedules = append(schedules, Schedule{
			ID:                  b.ID,
			Title:               b.Title,
			Cron:                b.Fields["cron"],
			Project:             b.Fields["project"],
			Role:                b.Fields["role"],
			Prompt:              b.Fields["prompt"],
			Enabled:             enabled,
			Timezone:            b.Fields["timezone"],
			SlackChannel:        b.Fields["slack_channel"],
			LastRun:             lastRun,
			LastAgentID:         b.Fields["last_agent_id"],
			MaxConcurrent:       parseIntField(b.Fields["max_concurrent"], defaultMaxConcurrent),
			Timeout:             parseDurationField(b.Fields["timeout"], defaultTimeout),
			RetryOnFailure:      b.Fields["retry_on_failure"] == "true",
			MaxRetries:          parseIntField(b.Fields["max_retries"], defaultMaxRetries),
			ConsecutiveFailures: parseIntField(b.Fields["consecutive_failures"], 0),
		})
	}
	return schedules, nil
}

// spawnScheduledAgent creates a new agent bead for a triggered schedule.
func (s *Scheduler) spawnScheduledAgent(ctx context.Context, sched Schedule) (string, error) {
	project := sched.Project
	if project == "" {
		project = "gasboat"
	}
	role := sched.Role
	if role == "" {
		role = "crew"
	}
	prompt := sched.Prompt
	if prompt == "" {
		prompt = "Scheduled task: " + sched.Title
	}

	agentName := fmt.Sprintf("sched-%d", time.Now().Unix())

	// Create a task bead for the scheduled run.
	taskFields := map[string]string{
		"schedule_id": sched.ID,
	}
	taskFieldsJSON, err := json.Marshal(taskFields)
	if err != nil {
		return "", fmt.Errorf("marshalling task fields: %w", err)
	}
	taskID, err := s.daemon.CreateBead(ctx, beadsapi.CreateBeadRequest{
		Title:       fmt.Sprintf("Scheduled: %s", sched.Title),
		Type:        "task",
		Description: prompt,
		Labels:      []string{"project:" + project, "scheduled"},
		Fields:      taskFieldsJSON,
	})
	if err != nil {
		return "", fmt.Errorf("creating scheduled task bead: %w", err)
	}

	// Spawn the agent with the task pre-assigned, including schedule metadata
	// so the bridge can identify this as a scheduled run and customize notifications.
	schedFields := map[string]string{
		"schedule_id":    sched.ID,
		"schedule_title": sched.Title,
		"schedule_cron":  sched.Cron,
	}
	if sched.SlackChannel != "" {
		schedFields["schedule_slack_channel"] = sched.SlackChannel
	}
	beadID, err := s.daemon.SpawnAgent(ctx, agentName, project, taskID, role, prompt, schedFields)
	if err != nil {
		return "", fmt.Errorf("spawning agent for schedule %s: %w", sched.ID, err)
	}

	// Best-effort: label agent as scheduled.
	_ = s.daemon.AddLabel(ctx, beadID, "scheduled")

	s.logger.Info("spawned scheduled agent",
		"schedule", sched.ID, "agent", agentName,
		"bead", beadID, "task", taskID,
		"project", project, "role", role)

	return beadID, nil
}

// parseIntField parses a string as an integer, returning defVal if empty or invalid.
func parseIntField(s string, defVal int) int {
	if s == "" {
		return defVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return defVal
	}
	return v
}

// parseDurationField parses a string as a duration, returning defVal if empty or invalid.
// Supports Go duration strings (e.g., "30m", "1h", "45s") and plain integers as minutes.
func parseDurationField(s string, defVal time.Duration) time.Duration {
	if s == "" {
		return defVal
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		// Try as plain minutes.
		if mins, err := strconv.Atoi(s); err == nil && mins > 0 {
			return time.Duration(mins) * time.Minute
		}
		return defVal
	}
	if d <= 0 {
		return defVal
	}
	return d
}
