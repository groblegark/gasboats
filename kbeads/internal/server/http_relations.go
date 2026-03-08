package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/groblegark/kbeads/internal/events"
	"github.com/groblegark/kbeads/internal/model"
)

// handleGetDependencies handles GET /v1/beads/{id}/dependencies.
func (s *BeadsServer) handleGetDependencies(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	if beadID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	deps, err := s.store.GetDependencies(r.Context(), beadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get dependencies")
		return
	}

	if deps == nil {
		deps = []*model.Dependency{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"dependencies": deps})
}

// handleGetReverseDependencies handles GET /v1/beads/{id}/dependents.
func (s *BeadsServer) handleGetReverseDependencies(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	if beadID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	deps, err := s.store.GetReverseDependencies(r.Context(), beadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get reverse dependencies")
		return
	}

	if deps == nil {
		deps = []*model.Dependency{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"dependencies": deps})
}

// addDependencyRequest is the JSON body for POST /v1/beads/{id}/dependencies.
type addDependencyRequest struct {
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"`
	CreatedBy   string `json:"created_by"`
}

// handleAddDependency handles POST /v1/beads/{id}/dependencies.
func (s *BeadsServer) handleAddDependency(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	if beadID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	var req addDependencyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.DependsOnID == "" {
		writeError(w, http.StatusBadRequest, "depends_on_id is required")
		return
	}

	now := time.Now().UTC()
	dep := &model.Dependency{
		BeadID:      beadID,
		DependsOnID: req.DependsOnID,
		Type:        model.DependencyType(req.Type),
		CreatedAt:   now,
		CreatedBy:   req.CreatedBy,
	}

	if err := s.store.AddDependency(r.Context(), dep); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add dependency")
		return
	}

	s.recordAndPublish(r.Context(), events.TopicDependencyAdded, dep.BeadID, dep.CreatedBy, events.DependencyAdded{Dependency: dep})

	writeJSON(w, http.StatusCreated, dep)
}

// handleRemoveDependency handles DELETE /v1/beads/{id}/dependencies.
// depends_on_id and type are taken from query parameters.
func (s *BeadsServer) handleRemoveDependency(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	if beadID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	q := r.URL.Query()
	dependsOnID := q.Get("depends_on_id")
	if dependsOnID == "" {
		writeError(w, http.StatusBadRequest, "depends_on_id query parameter is required")
		return
	}
	depType := q.Get("type")

	if err := s.store.RemoveDependency(r.Context(), beadID, dependsOnID, model.DependencyType(depType)); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove dependency")
		return
	}

	s.recordAndPublish(r.Context(), events.TopicDependencyRemoved, beadID, "", events.DependencyRemoved{
		BeadID:      beadID,
		DependsOnID: dependsOnID,
		Type:        depType,
	})

	w.WriteHeader(http.StatusNoContent)
}

// handleGetLabels handles GET /v1/beads/{id}/labels.
func (s *BeadsServer) handleGetLabels(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	if beadID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	labels, err := s.store.GetLabels(r.Context(), beadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get labels")
		return
	}

	if labels == nil {
		labels = []string{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"labels": labels})
}

// addLabelRequest is the JSON body for POST /v1/beads/{id}/labels.
type addLabelRequest struct {
	Label string `json:"label"`
}

// handleAddLabel handles POST /v1/beads/{id}/labels.
func (s *BeadsServer) handleAddLabel(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	if beadID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	var req addLabelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Label == "" {
		writeError(w, http.StatusBadRequest, "label is required")
		return
	}

	if err := s.store.AddLabel(r.Context(), beadID, req.Label); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add label")
		return
	}

	s.recordAndPublish(r.Context(), events.TopicLabelAdded, beadID, "", events.LabelAdded{
		BeadID: beadID,
		Label:  req.Label,
	})

	// Fetch the updated bead to return.
	bead, err := s.store.GetBead(r.Context(), beadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get bead after adding label")
		return
	}
	if bead == nil {
		writeError(w, http.StatusNotFound, "bead not found")
		return
	}

	writeJSON(w, http.StatusCreated, bead)
}

// handleRemoveLabel handles DELETE /v1/beads/{id}/labels/{label}.
func (s *BeadsServer) handleRemoveLabel(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	if beadID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	label := r.PathValue("label")
	if label == "" {
		writeError(w, http.StatusBadRequest, "label is required")
		return
	}

	if err := s.store.RemoveLabel(r.Context(), beadID, label); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove label")
		return
	}

	s.recordAndPublish(r.Context(), events.TopicLabelRemoved, beadID, "", events.LabelRemoved{
		BeadID: beadID,
		Label:  label,
	})

	w.WriteHeader(http.StatusNoContent)
}

// handleGetComments handles GET /v1/beads/{id}/comments.
func (s *BeadsServer) handleGetComments(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	if beadID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	comments, err := s.store.GetComments(r.Context(), beadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get comments")
		return
	}

	if comments == nil {
		comments = []*model.Comment{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"comments": comments})
}

// addCommentRequest is the JSON body for POST /v1/beads/{id}/comments.
type addCommentRequest struct {
	Author string `json:"author"`
	Text   string `json:"text"`
}

// handleAddComment handles POST /v1/beads/{id}/comments.
func (s *BeadsServer) handleAddComment(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	if beadID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	var req addCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	now := time.Now().UTC()
	comment := &model.Comment{
		BeadID:    beadID,
		Author:    req.Author,
		Text:      req.Text,
		CreatedAt: now,
	}

	if err := s.store.AddComment(r.Context(), comment); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to add comment")
		return
	}

	s.recordAndPublish(r.Context(), events.TopicCommentAdded, comment.BeadID, comment.Author, events.CommentAdded{Comment: comment})

	writeJSON(w, http.StatusCreated, comment)
}

// handleGetEvents handles GET /v1/beads/{id}/events.
func (s *BeadsServer) handleGetEvents(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	if beadID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	evts, err := s.store.GetEvents(r.Context(), beadID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get events")
		return
	}

	if evts == nil {
		evts = []*model.Event{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"events": evts})
}
