package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/groblegark/kbeads/internal/model"
)

// setConfigRequest is the JSON body for PUT /v1/configs/{key}.
type setConfigRequest struct {
	Value json.RawMessage `json:"value"`
}

// handleSetConfig handles PUT /v1/configs/{key}.
func (s *BeadsServer) handleSetConfig(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}

	var req setConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	config := &model.Config{
		Key:   key,
		Value: req.Value,
	}

	if err := s.store.SetConfig(r.Context(), config); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to set config")
		return
	}

	writeJSON(w, http.StatusOK, config)
}

// handleGetConfig handles GET /v1/configs/{key}.
func (s *BeadsServer) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}

	config, err := s.store.GetConfig(r.Context(), key)
	if errors.Is(err, sql.ErrNoRows) {
		if builtin, ok := builtinConfigs[key]; ok {
			writeJSON(w, http.StatusOK, builtin)
			return
		}
		writeError(w, http.StatusNotFound, "config not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get config")
		return
	}

	writeJSON(w, http.StatusOK, config)
}

// handleListConfigs handles GET /v1/configs?namespace=...
func (s *BeadsServer) handleListConfigs(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		writeError(w, http.StatusBadRequest, "namespace query parameter is required")
		return
	}

	configs, err := s.listConfigsWithBuiltins(r.Context(), namespace)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list configs")
		return
	}

	if configs == nil {
		configs = []*model.Config{}
	}

	writeJSON(w, http.StatusOK, map[string]any{"configs": configs})
}

// handleDeleteConfig handles DELETE /v1/configs/{key}.
func (s *BeadsServer) handleDeleteConfig(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}

	if err := s.store.DeleteConfig(r.Context(), key); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "config not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete config")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
