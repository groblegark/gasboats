package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/groblegark/kbeads/internal/model"
)

// handleAgentRoster handles GET /v1/agents/roster.
// Returns the live agent roster from the presence tracker.
func (s *BeadsServer) handleAgentRoster(w http.ResponseWriter, r *http.Request) {
	if s.Presence == nil {
		writeJSON(w, http.StatusOK, map[string]any{"actors": []any{}, "unclaimed_tasks": []any{}})
		return
	}

	// Parse optional stale_threshold_secs query param (default: 30 min).
	staleThreshold := 30 * time.Minute
	if v := r.URL.Query().Get("stale_threshold_secs"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			staleThreshold = time.Duration(secs) * time.Second
		}
	}

	entries := s.Presence.Roster(staleThreshold)

	// Enrich with task data from the store.
	type rosterEntry struct {
		Actor     string  `json:"actor"`
		TaskID    string  `json:"task_id,omitempty"`
		TaskTitle string  `json:"task_title,omitempty"`
		EpicID    string  `json:"epic_id,omitempty"`
		EpicTitle string  `json:"epic_title,omitempty"`
		IdleSecs  float64 `json:"idle_secs"`
		LastEvent string  `json:"last_event,omitempty"`
		ToolName  string  `json:"tool_name,omitempty"`
		SessionID string  `json:"session_id,omitempty"`
		CWD       string  `json:"cwd,omitempty"`
		Reaped    bool    `json:"reaped,omitempty"`
	}

	actors := make([]rosterEntry, 0, len(entries))
	for _, e := range entries {
		re := rosterEntry{
			Actor:     e.Actor,
			IdleSecs:  e.IdleSecs,
			LastEvent: e.LastEvent,
			ToolName:  e.ToolName,
			SessionID: e.SessionID,
			CWD:       e.CWD,
			Reaped:    e.Reaped,
		}

		// Look up the agent's current in_progress task.
		ctx := r.Context()
		beads, _, err := s.store.ListBeads(ctx, model.BeadFilter{
			Assignee: e.Actor,
			Status:   []model.Status{model.StatusInProgress},
			Limit:    1,
		})
		if err == nil && len(beads) > 0 {
			re.TaskID = beads[0].ID
			re.TaskTitle = beads[0].Title
		}

		actors = append(actors, re)
	}

	// Find unclaimed in_progress beads (no assignee).
	type unclaimedTask struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Priority int    `json:"priority"`
	}

	var unclaimed []unclaimedTask
	allInProgress, _, err := s.store.ListBeads(r.Context(), model.BeadFilter{
		Status: []model.Status{model.StatusInProgress},
		Limit:  50,
	})
	if err == nil {
		for _, b := range allInProgress {
			if b.Assignee == "" {
				unclaimed = append(unclaimed, unclaimedTask{
					ID:       b.ID,
					Title:    b.Title,
					Priority: b.Priority,
				})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"actors":          actors,
		"unclaimed_tasks": unclaimed,
	})
}
