package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// registerConnectShim registers POST /bd.v1.BeadsService/{method} routes that
// translate beads3d Connect-protocol calls to the native REST handlers.
// beads3d sends all requests as POST with JSON bodies; this shim adapts them.
func (s *BeadsServer) registerConnectShim(mux *http.ServeMux) {
	mux.HandleFunc("POST /bd.v1.BeadsService/{method}", s.handleConnect)
}

func (s *BeadsServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	method := r.PathValue("method")

	switch method {
	case "List":
		s.connectList(w, r)
	case "Graph":
		s.handleGraph(w, r)
	case "Stats":
		s.connectStats(w, r)
	case "GetConfig":
		s.connectGetConfig(w, r)
	case "ConfigList":
		s.connectListConfigs(w, r)
	case "Blocked":
		s.connectBlocked(w, r)
	case "Create":
		s.handleCreateBead(w, r)
	case "Update":
		s.connectUpdate(w, r)
	default:
		writeError(w, http.StatusNotFound, "unknown method: "+method)
	}
}

// connectList translates Connect List to GET /v1/beads with query params.
func (s *BeadsServer) connectList(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Status   []string `json:"status"`
		Type     []string `json:"type"`
		Kind     []string `json:"kind"`
		Assignee string   `json:"assignee"`
		Search   string   `json:"search"`
		Sort     string   `json:"sort"`
		Limit    int      `json:"limit"`
		Offset   int      `json:"offset"`
		Labels   []string `json:"labels"`
		Fields   []string `json:"fields"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)

	q := url.Values{}
	if len(body.Status) > 0 {
		q.Set("status", strings.Join(body.Status, ","))
	}
	if len(body.Type) > 0 {
		q.Set("type", strings.Join(body.Type, ","))
	}
	if len(body.Kind) > 0 {
		q.Set("kind", strings.Join(body.Kind, ","))
	}
	if body.Assignee != "" {
		q.Set("assignee", body.Assignee)
	}
	if body.Search != "" {
		q.Set("search", body.Search)
	}
	if body.Sort != "" {
		q.Set("sort", body.Sort)
	}
	if body.Limit > 0 {
		q.Set("limit", strconv.Itoa(body.Limit))
	}
	if body.Offset > 0 {
		q.Set("offset", strconv.Itoa(body.Offset))
	}
	for _, l := range body.Labels {
		q.Add("label", l)
	}
	for _, f := range body.Fields {
		q.Add("field", f)
	}
	r.URL.RawQuery = q.Encode()
	r.Method = http.MethodGet
	s.handleListBeads(w, r)
}

// connectStats translates Connect Stats to the internal stats handler.
func (s *BeadsServer) connectStats(w http.ResponseWriter, r *http.Request) {
	r.Method = http.MethodGet
	s.handleGetStats(w, r)
}

// connectGetConfig translates Connect GetConfig.
func (s *BeadsServer) connectGetConfig(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Key string `json:"key"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Key != "" {
		r.SetPathValue("key", body.Key)
		r.Method = http.MethodGet
		s.handleGetConfig(w, r)
	} else {
		r.Method = http.MethodGet
		s.handleListConfigs(w, r)
	}
}

// connectListConfigs translates Connect ConfigList.
func (s *BeadsServer) connectListConfigs(w http.ResponseWriter, r *http.Request) {
	r.Method = http.MethodGet
	s.handleListConfigs(w, r)
}

// connectBlocked translates Connect Blocked.
func (s *BeadsServer) connectBlocked(w http.ResponseWriter, r *http.Request) {
	r.Method = http.MethodGet
	s.handleGetBlocked(w, r)
}

// connectUpdate translates Connect Update to PATCH /v1/beads/{id}.
func (s *BeadsServer) connectUpdate(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	id, _ := body["id"].(string)
	if id == "" {
		writeError(w, http.StatusBadRequest, "missing id field")
		return
	}
	r.SetPathValue("id", id)
	delete(body, "id")

	// Re-encode without id and pipe through the PATCH handler.
	patchBytes, _ := json.Marshal(body)
	r.Body = io.NopCloser(bytes.NewReader(patchBytes))
	r.ContentLength = int64(len(patchBytes))
	r.Method = http.MethodPatch
	s.handleUpdateBead(w, r)
}
