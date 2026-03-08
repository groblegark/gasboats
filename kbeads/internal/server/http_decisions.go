package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/groblegark/kbeads/internal/events"
	"github.com/groblegark/kbeads/internal/model"
)

// handleGetDecision handles GET /v1/decisions/{id}.
// Returns the bead as both a decision fields object and the full issue data,
// matching the shape expected by the kbeads3d frontend.
func (s *BeadsServer) handleGetDecision(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	bead, err := s.store.GetBead(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) || bead == nil {
		writeError(w, http.StatusNotFound, "decision not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get decision")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"decision": extractDecisionFields(bead),
		"issue":    bead,
	})
}

// handleListDecisions handles GET /v1/decisions.
// Lists beads of type=decision, with optional status filter.
func (s *BeadsServer) handleListDecisions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := model.BeadFilter{
		Type: []model.BeadType{model.TypeDecision},
		Sort: "-created_at",
	}
	if v := q.Get("status"); v != "" {
		for _, st := range strings.Split(v, ",") {
			filter.Status = append(filter.Status, model.Status(st))
		}
	} else {
		filter.Status = []model.Status{model.StatusOpen, model.StatusInProgress}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			filter.Limit = n
		}
	}
	if filter.Limit == 0 {
		filter.Limit = 50
	}

	beads, _, err := s.store.ListBeads(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list decisions")
		return
	}

	decisions := make([]map[string]any, 0, len(beads))
	for _, b := range beads {
		decisions = append(decisions, map[string]any{
			"decision": extractDecisionFields(b),
			"issue":    b,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"decisions": decisions})
}

// resolveDecisionRequest is the JSON body for POST /v1/decisions/{id}/resolve.
type resolveDecisionRequest struct {
	SelectedOption string `json:"selected_option"`
	ResponseText   string `json:"response_text"`
	RespondedBy    string `json:"responded_by"`
}

// handleResolveDecision handles POST /v1/decisions/{id}/resolve.
// Merges response fields into the bead, then closes it.
func (s *BeadsServer) handleResolveDecision(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	var req resolveDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.SelectedOption == "" && req.ResponseText == "" {
		writeError(w, http.StatusBadRequest, "selected_option or response_text is required")
		return
	}

	// Merge resolution fields.
	extra := map[string]any{
		"responded_at": time.Now().UTC().Format(time.RFC3339),
	}
	if req.SelectedOption != "" {
		extra["chosen"] = req.SelectedOption
	}
	if req.ResponseText != "" {
		extra["response_text"] = req.ResponseText
	}
	if req.RespondedBy != "" {
		extra["responded_by"] = req.RespondedBy
	}

	if err := s.mergeBeadFields(r.Context(), id, extra); err != nil {
		slog.Warn("failed to merge decision fields", "bead", id, "error", err)
	}

	closedBy := req.RespondedBy
	bead, err := s.store.CloseBead(r.Context(), id, closedBy)
	if errors.Is(err, sql.ErrNoRows) || bead == nil {
		writeError(w, http.StatusNotFound, "decision not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to close decision")
		return
	}

	s.recordAndPublish(r.Context(), events.TopicBeadClosed, bead.ID, closedBy, events.BeadClosed{
		Bead:     bead,
		ClosedBy: closedBy,
	})

	resp := map[string]any{
		"decision": extractDecisionFields(bead),
		"issue":    bead,
	}
	if rt := reportTypeFromLabels(bead.Labels); rt != "" {
		resp["report_required"] = true
		resp["report_type"] = rt
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleCancelDecision handles POST /v1/decisions/{id}/cancel.
func (s *BeadsServer) handleCancelDecision(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	var body struct {
		Reason     string `json:"reason"`
		CanceledBy string `json:"canceled_by"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	extra := map[string]any{
		"canceled": true,
	}
	if body.Reason != "" {
		extra["cancel_reason"] = body.Reason
	}
	if err := s.mergeBeadFields(r.Context(), id, extra); err != nil {
		slog.Warn("failed to merge cancel fields", "bead", id, "error", err)
	}

	closedBy := body.CanceledBy
	bead, err := s.store.CloseBead(r.Context(), id, closedBy)
	if errors.Is(err, sql.ErrNoRows) || bead == nil {
		writeError(w, http.StatusNotFound, "decision not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to cancel decision")
		return
	}

	s.recordAndPublish(r.Context(), events.TopicBeadClosed, bead.ID, closedBy, events.BeadClosed{
		Bead:     bead,
		ClosedBy: closedBy,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"decision": extractDecisionFields(bead),
		"issue":    bead,
	})
}

// extractDecisionFields extracts decision-specific fields from a bead's Fields JSON.
// Returns a map with keys like prompt, options, context, selected_option (mapped from chosen),
// requested_by, responded_by, responded_at, response_text, urgency, iteration, max_iterations.
func extractDecisionFields(b *model.Bead) map[string]any {
	dec := make(map[string]any)
	if len(b.Fields) == 0 {
		return dec
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b.Fields, &fields); err != nil {
		return dec
	}

	// String fields — extract and assign.
	stringKeys := []string{"prompt", "context", "requested_by", "responded_by", "responded_at", "response_text", "urgency"}
	for _, k := range stringKeys {
		if raw, ok := fields[k]; ok {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				dec[k] = s
			} else {
				dec[k] = strings.TrimSpace(string(raw))
			}
		}
	}

	// "chosen" in storage maps to "selected_option" in the frontend.
	if raw, ok := fields["chosen"]; ok {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			dec["selected_option"] = s
		}
	}

	// "options" — pass through as-is (may be JSON array string or raw array).
	if raw, ok := fields["options"]; ok {
		dec["options"] = json.RawMessage(raw)
	}

	// Numeric fields.
	numKeys := []string{"iteration", "max_iterations"}
	for _, k := range numKeys {
		if raw, ok := fields[k]; ok {
			var n float64
			if json.Unmarshal(raw, &n) == nil {
				dec[k] = int(n)
			}
		}
	}

	// Report requirement derived from bead labels.
	if rt := reportTypeFromLabels(b.Labels); rt != "" {
		dec["report_required"] = true
		dec["report_type"] = rt
	}

	return dec
}
