package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/groblegark/kbeads/internal/events"
	"github.com/groblegark/kbeads/internal/model"
)

// handleCreateBead handles POST /v1/beads.
func (s *BeadsServer) handleCreateBead(w http.ResponseWriter, r *http.Request) {
	var in createBeadInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	bead, err := s.createBead(r.Context(), in)
	if err != nil {
		var ie inputError
		if errors.As(err, &ie) {
			writeError(w, http.StatusBadRequest, ie.Error())
		} else {
			writeError(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	writeJSON(w, http.StatusCreated, bead)
}

// handleListBeads handles GET /v1/beads.
func (s *BeadsServer) handleListBeads(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := model.BeadFilter{
		Assignee: q.Get("assignee"),
		Search:   q.Get("search"),
		Sort:     q.Get("sort"),
	}

	if v := q.Get("status"); v != "" {
		for _, s := range strings.Split(v, ",") {
			filter.Status = append(filter.Status, model.Status(s))
		}
	}
	if v := q.Get("type"); v != "" {
		for _, t := range strings.Split(v, ",") {
			filter.Type = append(filter.Type, model.BeadType(t))
		}
	}
	if v := q.Get("kind"); v != "" {
		for _, k := range strings.Split(v, ",") {
			filter.Kind = append(filter.Kind, model.Kind(k))
		}
	}
	if v := q.Get("labels"); v != "" {
		filter.Labels = strings.Split(v, ",")
	}
	if v := q.Get("priority"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Priority = &n
		}
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Limit = n
		}
	}
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			filter.Offset = n
		}
	}
	if q.Get("no_open_deps") == "true" {
		filter.NoOpenDeps = true
	}
	if ff := q["field"]; len(ff) > 0 {
		filter.Fields = make(map[string]string, len(ff))
		for _, f := range ff {
			if k, v, ok := strings.Cut(f, "="); ok {
				filter.Fields[k] = v
			}
		}
	}

	beads, total, err := s.store.ListBeads(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list beads")
		return
	}

	// Ensure beads is never null in JSON output.
	if beads == nil {
		beads = []*model.Bead{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"beads": beads,
		"total": total,
	})
}

// handleGetBead handles GET /v1/beads/{id}.
func (s *BeadsServer) handleGetBead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	bead, err := s.store.GetBead(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "bead not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get bead")
		return
	}
	if bead == nil {
		writeError(w, http.StatusNotFound, "bead not found")
		return
	}

	writeJSON(w, http.StatusOK, bead)
}

// handleDeleteBead handles DELETE /v1/beads/{id}.
func (s *BeadsServer) handleDeleteBead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := s.store.DeleteBead(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bead not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete bead")
		return
	}

	s.recordAndPublish(r.Context(), events.TopicBeadDeleted, id, "", events.BeadDeleted{BeadID: id})

	w.WriteHeader(http.StatusNoContent)
}

// handleUpdateBead handles PATCH /v1/beads/{id}.
func (s *BeadsServer) handleUpdateBead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	var in updateBeadInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	// For HTTP/JSON, DueAt/DeferUntil/Labels presence is inferred from non-nil/non-empty.
	if in.DueAt != nil {
		in.dueAtSet = true
	}
	if in.DeferUntil != nil {
		in.deferUntilSet = true
	}
	if in.Labels != nil {
		in.labelsSet = true
	}

	bead, err := s.updateBead(r.Context(), id, in)
	if err != nil {
		var ie inputError
		if errors.As(err, &ie) {
			writeError(w, http.StatusBadRequest, ie.Error())
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "bead not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, bead)
}

// handleCloseBead handles POST /v1/beads/{id}/close.
// Accepts optional JSON body with "closed_by" and any extra fields to merge
// into the bead's fields before closing (e.g., "chosen", "rationale" for decisions).
func (s *BeadsServer) handleCloseBead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	// Decode body as generic map to capture both closed_by and extra fields.
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)

	closedBy, _ := body["closed_by"].(string)

	// Merge extra fields (anything except closed_by) into the bead before closing.
	extraFields := make(map[string]any)
	for k, v := range body {
		if k != "closed_by" {
			extraFields[k] = v
		}
	}
	if len(extraFields) > 0 {
		if err := s.mergeBeadFields(r.Context(), id, extraFields); err != nil {
			// Non-fatal — log and continue with close.
			slog.Warn("failed to merge close fields", "bead", id, "error", err)
		}
	}

	bead, err := s.store.CloseBead(r.Context(), id, closedBy)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "bead not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to close bead")
		return
	}
	if bead == nil {
		writeError(w, http.StatusNotFound, "bead not found")
		return
	}

	s.recordAndPublish(r.Context(), events.TopicBeadClosed, bead.ID, closedBy, events.BeadClosed{
		Bead:     bead,
		ClosedBy: closedBy,
	})

	writeJSON(w, http.StatusOK, bead)
}
