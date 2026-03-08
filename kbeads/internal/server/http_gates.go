package server

import (
	"net/http"

	"github.com/groblegark/kbeads/internal/model"
)

// handleListGates handles GET /v1/agents/{id}/gates.
// Returns all gate rows for an agent bead.
func (s *BeadsServer) handleListGates(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	if agentID == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	gates, err := s.store.ListGates(r.Context(), agentID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if gates == nil {
		gates = []model.GateRow{}
	}
	writeJSON(w, http.StatusOK, gates)
}

// handleSatisfyGate handles POST /v1/agents/{id}/gates/{gate_id}/satisfy.
// Manually satisfies a gate for an agent bead.
func (s *BeadsServer) handleSatisfyGate(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	gateID := r.PathValue("gate_id")
	if agentID == "" || gateID == "" {
		writeError(w, http.StatusBadRequest, "id and gate_id are required")
		return
	}
	// Upsert first to ensure row exists.
	if err := s.store.UpsertGate(r.Context(), agentID, gateID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.store.MarkGateSatisfied(r.Context(), agentID, gateID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "satisfied"})
}

// handleClearGate handles DELETE /v1/agents/{id}/gates/{gate_id}.
// Resets a gate back to pending status.
func (s *BeadsServer) handleClearGate(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("id")
	gateID := r.PathValue("gate_id")
	if agentID == "" || gateID == "" {
		writeError(w, http.StatusBadRequest, "id and gate_id are required")
		return
	}
	if err := s.store.ClearGate(r.Context(), agentID, gateID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "pending"})
}
