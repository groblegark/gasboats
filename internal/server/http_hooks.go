package server

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/groblegark/kbeads/internal/eventbus"
	"github.com/groblegark/kbeads/internal/hooks"
	"github.com/groblegark/kbeads/internal/presence"
)

// hookEmitRequest is the JSON body for POST /v1/hooks/emit.
type hookEmitRequest struct {
	AgentBeadID     string `json:"agent_bead_id"`
	HookType        string `json:"hook_type"`        // "Stop", "PreToolUse", etc.
	ClaudeSessionID string `json:"claude_session_id"`
	CWD             string `json:"cwd"`
	Actor           string `json:"actor"`
	ToolName        string `json:"tool_name,omitempty"` // tool name from Claude Code (e.g. "Bash", "Read")
}

// hookEmitResponse is the JSON response from POST /v1/hooks/emit.
type hookEmitResponse struct {
	Block    bool     `json:"block,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Inject   string   `json:"inject,omitempty"`
}

// handleHookEmit handles POST /v1/hooks/emit.
// It evaluates gate state and soft auto-checks, returning block/warn/inject signals.
func (s *BeadsServer) handleHookEmit(w http.ResponseWriter, r *http.Request) {
	var req hookEmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Record presence for agent roster tracking.
	if s.Presence != nil && req.Actor != "" {
		s.Presence.RecordHookEvent(presence.HookEvent{
			Actor:     req.Actor,
			HookType:  req.HookType,
			ToolName:  req.ToolName,
			SessionID: req.ClaudeSessionID,
			CWD:       req.CWD,
		})
	}

	ctx := r.Context()
	var resp hookEmitResponse

	// If no agent_bead_id, there are no gates to check — allow.
	if req.AgentBeadID == "" {
		writeJSON(w, http.StatusOK, resp)
		return
	}

	// Evaluate strict gates for Stop hook.
	if req.HookType == "Stop" {
		// Upsert the decision gate for this agent (creates pending row if not exists).
		if err := s.store.UpsertGate(ctx, req.AgentBeadID, "decision"); err != nil {
			slog.Warn("hookEmit: failed to upsert decision gate", "agent", req.AgentBeadID, "err", err)
		}

		satisfied, err := s.store.IsGateSatisfied(ctx, req.AgentBeadID, "decision")
		if err != nil {
			slog.Warn("hookEmit: failed to check decision gate", "agent", req.AgentBeadID, "err", err)
		}
		if !satisfied {
			resp.Block = true
			resp.Reason = "decision: decision point offered before session end"
			writeJSON(w, http.StatusOK, resp)
			return
		}

		// Gate is satisfied — verify it was satisfied via 'gb yield' (not a manual mark).
		// gb yield sets gate_satisfied_by=yield; gb gate mark --force sets gate_satisfied_by=operator
		// (or the legacy value manual-force for backward compatibility). An empty or unrecognized
		// value means the gate was bypassed without going through the proper yield flow, which
		// breaks the Slack bridge.
		agentBead, beadErr := s.store.GetBead(ctx, req.AgentBeadID)
		var satisfiedBy string
		if beadErr == nil && agentBead != nil && len(agentBead.Fields) > 0 {
			var fields map[string]any
			if json.Unmarshal(agentBead.Fields, &fields) == nil {
				satisfiedBy, _ = fields["gate_satisfied_by"].(string)
			}
		}
		if satisfiedBy != "yield" && satisfiedBy != "operator" && satisfiedBy != "manual-force" {
			resp.Block = true
			resp.Reason = "decision: gate was not satisfied via 'gb yield' — create a decision with 'gb decision create' then call 'gb yield'"
			writeJSON(w, http.StatusOK, resp)
			return
		}

		// Consume the gate (reset to pending) so the next Stop blocks again.
		if err := s.store.ClearGate(ctx, req.AgentBeadID, "decision"); err != nil {
			slog.Warn("hookEmit: failed to clear decision gate after consume", "agent", req.AgentBeadID, "err", err)
		}
		// Clear gate_satisfied_by field so it doesn't carry over to the next session.
		if err := s.mergeBeadFields(ctx, req.AgentBeadID, map[string]any{"gate_satisfied_by": nil}); err != nil {
			slog.Warn("hookEmit: failed to clear gate_satisfied_by field", "agent", req.AgentBeadID, "err", err)
		}
	}

	// Soft gate AutoChecks — always warn, never block.
	if warning := hooks.CheckCommitPush(req.CWD); warning != "" {
		resp.Warnings = append(resp.Warnings, warning)
	}

	// TODO: bead-update soft check — requires checking KD_HOOK_BEAD env var from the
	// client side. Skip server-side check for now; the CLI can handle this in future.

	// Run existing advice hook logic for session-end trigger on Stop hook.
	if req.HookType == "Stop" {
		agentID := req.AgentBeadID
		if req.Actor != "" {
			agentID = req.Actor
		}
		hookResp := s.hooksHandler.HandleSessionEvent(ctx, hooks.SessionEvent{
			AgentID: agentID,
			Trigger: hooks.TriggerSessionEnd,
			CWD:     req.CWD,
		})
		if hookResp.Block && !resp.Block {
			resp.Block = true
			resp.Reason = hookResp.Reason
		}
		resp.Warnings = append(resp.Warnings, hookResp.Warnings...)
	}

	// Dispatch hook event through the bus for JetStream publishing and handler chain.
	if s.bus != nil {
		event := &eventbus.Event{
			Type:      eventbus.EventType(req.HookType),
			SessionID: req.ClaudeSessionID,
			CWD:       req.CWD,
			Actor:     req.Actor,
			ToolName:  req.ToolName,
		}
		if busResult, err := s.bus.Dispatch(ctx, event); err != nil {
			slog.Warn("hookEmit: bus dispatch error", "hook_type", req.HookType, "err", err)
		} else if busResult.Block && !resp.Block {
			resp.Block = busResult.Block
			resp.Reason = busResult.Reason
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// hookPublishRequest is the JSON body for POST /v1/hooks/publish.
type hookPublishRequest struct {
	Subject string          `json:"subject"`
	Payload json.RawMessage `json:"payload"`
}

// handleHookPublish handles POST /v1/hooks/publish.
// It publishes a pre-built hook event payload to the NATS HOOK_EVENTS stream
// on the given subject. This allows the hook relay to publish events through
// the daemon's persistent NATS connection instead of opening a new connection
// per event.
func (s *BeadsServer) handleHookPublish(w http.ResponseWriter, r *http.Request) {
	var req hookPublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Subject == "" {
		writeError(w, http.StatusBadRequest, "subject is required")
		return
	}
	if len(req.Payload) == 0 {
		writeError(w, http.StatusBadRequest, "payload is required")
		return
	}

	if s.bus == nil {
		writeError(w, http.StatusServiceUnavailable, "event bus not configured")
		return
	}

	s.bus.PublishRaw(req.Subject, req.Payload)
	w.WriteHeader(http.StatusNoContent)
}

// executeHooksRequest is the JSON body for POST /v1/hooks/execute.
type executeHooksRequest struct {
	AgentID string `json:"agent_id"`
	Trigger string `json:"trigger"`
	CWD     string `json:"cwd,omitempty"`
}

// handleExecuteHooks handles POST /v1/hooks/execute.
// Agents call this to evaluate advice hooks for a given lifecycle trigger.
func (s *BeadsServer) handleExecuteHooks(w http.ResponseWriter, r *http.Request) {
	var req executeHooksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.AgentID == "" {
		writeError(w, http.StatusBadRequest, "agent_id is required")
		return
	}
	if req.Trigger == "" {
		writeError(w, http.StatusBadRequest, "trigger is required")
		return
	}

	resp := s.hooksHandler.HandleSessionEvent(r.Context(), hooks.SessionEvent{
		AgentID: req.AgentID,
		Trigger: req.Trigger,
		CWD:     req.CWD,
	})

	writeJSON(w, http.StatusOK, resp)
}
