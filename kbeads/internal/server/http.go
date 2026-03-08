package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// NewHTTPHandler returns an http.Handler with all routes registered.
// When authToken is non-empty, requests (except GET /v1/health) must include
// a valid Authorization: Bearer <token> header.
func (s *BeadsServer) NewHTTPHandler(authToken string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/beads", s.handleCreateBead)
	mux.HandleFunc("GET /v1/beads", s.handleListBeads)
	mux.HandleFunc("GET /v1/beads/{id}", s.handleGetBead)
	mux.HandleFunc("PATCH /v1/beads/{id}", s.handleUpdateBead)
	mux.HandleFunc("POST /v1/beads/{id}/close", s.handleCloseBead)
	mux.HandleFunc("DELETE /v1/beads/{id}", s.handleDeleteBead)
	mux.HandleFunc("GET /v1/beads/{id}/dependencies", s.handleGetDependencies)
	mux.HandleFunc("GET /v1/beads/{id}/dependents", s.handleGetReverseDependencies)
	mux.HandleFunc("POST /v1/beads/{id}/dependencies", s.handleAddDependency)
	mux.HandleFunc("DELETE /v1/beads/{id}/dependencies", s.handleRemoveDependency)
	mux.HandleFunc("GET /v1/beads/{id}/labels", s.handleGetLabels)
	mux.HandleFunc("POST /v1/beads/{id}/labels", s.handleAddLabel)
	mux.HandleFunc("DELETE /v1/beads/{id}/labels/{label}", s.handleRemoveLabel)
	mux.HandleFunc("GET /v1/beads/{id}/comments", s.handleGetComments)
	mux.HandleFunc("POST /v1/beads/{id}/comments", s.handleAddComment)
	mux.HandleFunc("GET /v1/beads/{id}/events", s.handleGetEvents)
	mux.HandleFunc("PUT /v1/configs/{key...}", s.handleSetConfig)
	mux.HandleFunc("GET /v1/configs/{key...}", s.handleGetConfig)
	mux.HandleFunc("GET /v1/configs", s.handleListConfigs)
	mux.HandleFunc("DELETE /v1/configs/{key...}", s.handleDeleteConfig)
	mux.HandleFunc("GET /v1/graph", s.handleGraph)
	mux.HandleFunc("POST /v1/graph", s.handleGraph)
	mux.HandleFunc("GET /v1/stats", s.handleGetStats)
	mux.HandleFunc("GET /v1/ready", s.handleGetReady)
	mux.HandleFunc("GET /v1/blocked", s.handleGetBlocked)
	mux.HandleFunc("POST /v1/hooks/execute", s.handleExecuteHooks)
	mux.HandleFunc("POST /v1/hooks/emit", s.handleHookEmit)
	mux.HandleFunc("POST /v1/hooks/publish", s.handleHookPublish)
	mux.HandleFunc("GET /v1/events/stream", s.handleEventStream)
	mux.HandleFunc("GET /v1/bus/events", s.handleBusEvents)
	mux.HandleFunc("GET /v1/bus/status", s.handleBusStatus)
	mux.HandleFunc("GET /v1/decisions/{id}", s.handleGetDecision)
	mux.HandleFunc("GET /v1/decisions", s.handleListDecisions)
	mux.HandleFunc("POST /v1/decisions/{id}/resolve", s.handleResolveDecision)
	mux.HandleFunc("POST /v1/decisions/{id}/cancel", s.handleCancelDecision)
	mux.HandleFunc("GET /v1/health", s.handleHealth)
	mux.HandleFunc("GET /v1/agents/{id}/gates", s.handleListGates)
	mux.HandleFunc("POST /v1/agents/{id}/gates/{gate_id}/satisfy", s.handleSatisfyGate)
	mux.HandleFunc("DELETE /v1/agents/{id}/gates/{gate_id}", s.handleClearGate)
	mux.HandleFunc("GET /v1/agents/roster", s.handleAgentRoster)
	s.registerConnectShim(mux)
	return AuthMiddleware(authToken, mux)
}

// handleHealth handles GET /v1/health.
func (s *BeadsServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// mergeBeadFields merges extra fields into an existing bead's fields JSON.
func (s *BeadsServer) mergeBeadFields(ctx context.Context, id string, extra map[string]any) error {
	bead, err := s.store.GetBead(ctx, id)
	if err != nil {
		return fmt.Errorf("get bead: %w", err)
	}

	// Parse existing fields.
	existing := make(map[string]any)
	if len(bead.Fields) > 0 {
		_ = json.Unmarshal(bead.Fields, &existing)
	}

	// Merge extra fields.
	for k, v := range extra {
		existing[k] = v
	}

	// Marshal back.
	merged, err := json.Marshal(existing)
	if err != nil {
		return fmt.Errorf("marshal fields: %w", err)
	}
	bead.Fields = merged

	return s.store.UpdateBead(ctx, bead)
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
