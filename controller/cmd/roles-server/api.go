package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"

	"gasboat/controller/internal/beadsapi"
)

// RolesAPI serves the read-only roles API endpoints.
type RolesAPI struct {
	client *beadsapi.Client
	logger *slog.Logger
}

// NewRolesAPI creates a new roles API handler.
func NewRolesAPI(client *beadsapi.Client, logger *slog.Logger) *RolesAPI {
	return &RolesAPI{client: client, logger: logger}
}

// RegisterRoutes registers API routes on the given mux.
func (a *RolesAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/roles", a.handleListRoles)
	mux.HandleFunc("GET /api/roles/{role}", a.handleGetRole)
	mux.HandleFunc("GET /api/config-beads", a.handleListConfigBeads)
	mux.HandleFunc("GET /api/advice", a.handleListAdvice)
	mux.HandleFunc("GET /api/projects", a.handleListProjects)
}

// roleInfo is the JSON representation of a role.
type roleInfo struct {
	Name         string       `json:"name"`
	ConfigBeads  []configBead `json:"config_beads"`
	AdviceBeads  []adviceBead `json:"advice_beads"`
	AgentCount   int          `json:"agent_count"`
	ActiveAgents []string     `json:"active_agents,omitempty"`
}

// configBead is a simplified config bead for the API response.
type configBead struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Labels []string `json:"labels"`
	Value  any      `json:"value,omitempty"`
}

// adviceBead is a simplified advice bead for the API response.
type adviceBead struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Labels      []string `json:"labels"`
	Description string   `json:"description,omitempty"`
	Status      string   `json:"status"`
}

// fetchRoles fetches all roles derived from config beads, advice beads,
// and active agent beads. A role is any unique "role:X" label found across
// config and advice beads.
func (a *RolesAPI) fetchRoles(ctx context.Context) ([]roleInfo, error) {
	// Fetch config beads, advice beads, and agents in parallel.
	var (
		configResult *beadsapi.ListBeadsResult
		adviceResult *beadsapi.ListBeadsResult
		agents       []beadsapi.AgentBead
		configErr    error
		adviceErr    error
		agentsErr    error
		wg           sync.WaitGroup
	)

	wg.Add(3)
	go func() {
		defer wg.Done()
		configResult, configErr = a.client.ListBeadsFiltered(ctx, beadsapi.ListBeadsQuery{
			Types: []string{"config"},
		})
	}()
	go func() {
		defer wg.Done()
		adviceResult, adviceErr = a.client.ListBeadsFiltered(ctx, beadsapi.ListBeadsQuery{
			Types:    []string{"advice"},
			Statuses: []string{"open", "in_progress"},
		})
	}()
	go func() {
		defer wg.Done()
		agents, agentsErr = a.client.ListAgentBeads(ctx)
	}()
	wg.Wait()

	if configErr != nil {
		return nil, fmt.Errorf("list config beads: %w", configErr)
	}
	if adviceErr != nil {
		return nil, fmt.Errorf("list advice beads: %w", adviceErr)
	}
	if agentsErr != nil {
		return nil, fmt.Errorf("list agent beads: %w", agentsErr)
	}

	// Extract unique roles from all label sources.
	roles := make(map[string]*roleInfo)
	ensureRole := func(name string) *roleInfo {
		ri, ok := roles[name]
		if !ok {
			ri = &roleInfo{Name: name}
			roles[name] = ri
		}
		return ri
	}

	// Always include "global" as a pseudo-role for global config.
	ensureRole("global")

	for _, b := range configResult.Beads {
		cb := toConfigBead(b)
		for _, label := range b.Labels {
			if name, ok := strings.CutPrefix(label, "role:"); ok {
				ri := ensureRole(name)
				ri.ConfigBeads = append(ri.ConfigBeads, cb)
			}
			if label == "global" {
				ri := ensureRole("global")
				ri.ConfigBeads = append(ri.ConfigBeads, cb)
			}
		}
	}

	for _, b := range adviceResult.Beads {
		ab := toAdviceBead(b)
		for _, label := range b.Labels {
			if name, ok := strings.CutPrefix(label, "role:"); ok {
				ri := ensureRole(name)
				ri.AdviceBeads = append(ri.AdviceBeads, ab)
			}
			if label == "global" {
				ri := ensureRole("global")
				ri.AdviceBeads = append(ri.AdviceBeads, ab)
			}
		}
	}

	for _, ag := range agents {
		if ag.Role != "" {
			ri := ensureRole(ag.Role)
			ri.AgentCount++
			ri.ActiveAgents = append(ri.ActiveAgents, ag.AgentName)
		}
	}

	// Sort roles by name.
	result := make([]roleInfo, 0, len(roles))
	for _, ri := range roles {
		result = append(result, *ri)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Name == "global" {
			return true
		}
		if result[j].Name == "global" {
			return false
		}
		return result[i].Name < result[j].Name
	})

	return result, nil
}

// handleListRoles lists all known roles.
func (a *RolesAPI) handleListRoles(w http.ResponseWriter, r *http.Request) {
	roles, err := a.fetchRoles(r.Context())
	if err != nil {
		a.logger.Error("failed to fetch roles", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch roles")
		return
	}
	writeJSON(w, map[string]any{"roles": roles})
}

// fetchRole returns the detailed configuration for a specific role,
// including its resolved config beads, advice beads, and active agents.
func (a *RolesAPI) fetchRole(ctx context.Context, roleName string) (*roleInfo, error) {
	var labelFilter []string
	if roleName == "global" {
		labelFilter = []string{"global"}
	} else {
		labelFilter = []string{"role:" + roleName}
	}

	// Fetch config beads, advice beads, and agents in parallel.
	var (
		configResult *beadsapi.ListBeadsResult
		adviceResult *beadsapi.ListBeadsResult
		agents       []beadsapi.AgentBead
		configErr    error
		adviceErr    error
		agentsErr    error
		wg           sync.WaitGroup
	)

	wg.Add(3)
	go func() {
		defer wg.Done()
		configResult, configErr = a.client.ListBeadsFiltered(ctx, beadsapi.ListBeadsQuery{
			Types:  []string{"config"},
			Labels: labelFilter,
		})
	}()
	go func() {
		defer wg.Done()
		adviceResult, adviceErr = a.client.ListBeadsFiltered(ctx, beadsapi.ListBeadsQuery{
			Types:    []string{"advice"},
			Statuses: []string{"open", "in_progress"},
			Labels:   labelFilter,
		})
	}()
	go func() {
		defer wg.Done()
		agents, agentsErr = a.client.ListAgentBeads(ctx)
	}()
	wg.Wait()

	if configErr != nil {
		return nil, fmt.Errorf("list config beads: %w", configErr)
	}
	if adviceErr != nil {
		return nil, fmt.Errorf("list advice beads: %w", adviceErr)
	}
	if agentsErr != nil {
		return nil, fmt.Errorf("list agent beads: %w", agentsErr)
	}

	configs := make([]configBead, 0, len(configResult.Beads))
	for _, b := range configResult.Beads {
		configs = append(configs, toConfigBead(b))
	}

	advices := make([]adviceBead, 0, len(adviceResult.Beads))
	for _, b := range adviceResult.Beads {
		advices = append(advices, toAdviceBead(b))
	}

	var activeAgents []string
	for _, ag := range agents {
		if ag.Role == roleName {
			activeAgents = append(activeAgents, ag.AgentName)
		}
	}

	return &roleInfo{
		Name:         roleName,
		ConfigBeads:  configs,
		AdviceBeads:  advices,
		AgentCount:   len(activeAgents),
		ActiveAgents: activeAgents,
	}, nil
}

// handleGetRole returns the detailed configuration for a specific role.
func (a *RolesAPI) handleGetRole(w http.ResponseWriter, r *http.Request) {
	roleName := r.PathValue("role")
	role, err := a.fetchRole(r.Context(), roleName)
	if err != nil {
		a.logger.Error("failed to fetch role", "role", roleName, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch role")
		return
	}
	writeJSON(w, role)
}

// listConfigQuery builds a ListBeadsQuery for config beads from HTTP request params.
func listConfigQuery(r *http.Request) beadsapi.ListBeadsQuery {
	q := beadsapi.ListBeadsQuery{
		Types: []string{"config"},
	}
	if labels := r.URL.Query().Get("labels"); labels != "" {
		q.Labels = strings.Split(labels, ",")
	}
	return q
}

// listAdviceQuery builds a ListBeadsQuery for advice beads from HTTP request params.
func listAdviceQuery(r *http.Request) beadsapi.ListBeadsQuery {
	q := beadsapi.ListBeadsQuery{
		Types:    []string{"advice"},
		Statuses: []string{"open", "in_progress"},
	}
	if labels := r.URL.Query().Get("labels"); labels != "" {
		q.Labels = strings.Split(labels, ",")
	}
	return q
}

// fetchConfigBeads fetches config beads using query params from the request.
func (a *RolesAPI) fetchConfigBeads(r *http.Request) ([]configBead, int, error) {
	result, err := a.client.ListBeadsFiltered(r.Context(), listConfigQuery(r))
	if err != nil {
		return nil, 0, err
	}
	beads := make([]configBead, 0, len(result.Beads))
	for _, b := range result.Beads {
		beads = append(beads, toConfigBead(b))
	}
	return beads, result.Total, nil
}

// fetchAdviceBeads fetches advice beads using query params from the request.
func (a *RolesAPI) fetchAdviceBeads(r *http.Request) ([]adviceBead, int, error) {
	result, err := a.client.ListBeadsFiltered(r.Context(), listAdviceQuery(r))
	if err != nil {
		return nil, 0, err
	}
	beads := make([]adviceBead, 0, len(result.Beads))
	for _, b := range result.Beads {
		beads = append(beads, toAdviceBead(b))
	}
	return beads, result.Total, nil
}

// fetchProjects fetches all registered projects.
func (a *RolesAPI) fetchProjects(ctx context.Context) (map[string]beadsapi.ProjectInfo, error) {
	return a.client.ListProjectBeads(ctx)
}

// handleListConfigBeads lists all config beads, optionally filtered by label.
func (a *RolesAPI) handleListConfigBeads(w http.ResponseWriter, r *http.Request) {
	beads, total, err := a.fetchConfigBeads(r)
	if err != nil {
		a.logger.Error("failed to list config beads", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list config beads")
		return
	}
	writeJSON(w, map[string]any{"config_beads": beads, "total": total})
}

// handleListAdvice lists advice beads, optionally filtered by label.
func (a *RolesAPI) handleListAdvice(w http.ResponseWriter, r *http.Request) {
	beads, total, err := a.fetchAdviceBeads(r)
	if err != nil {
		a.logger.Error("failed to list advice beads", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list advice beads")
		return
	}
	writeJSON(w, map[string]any{"advice_beads": beads, "total": total})
}

// handleListProjects lists all registered projects.
func (a *RolesAPI) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := a.fetchProjects(r.Context())
	if err != nil {
		a.logger.Error("failed to list projects", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}
	writeJSON(w, map[string]any{"projects": projects})
}

// toConfigBead converts a BeadDetail to a configBead response.
func toConfigBead(b *beadsapi.BeadDetail) configBead {
	cb := configBead{
		ID:     b.ID,
		Title:  b.Title,
		Labels: b.Labels,
	}
	if raw, ok := b.Fields["value"]; ok && raw != "" {
		var parsed any
		if json.Unmarshal([]byte(raw), &parsed) == nil {
			cb.Value = parsed
		} else {
			cb.Value = raw
		}
	}
	return cb
}

// toAdviceBead converts a BeadDetail to an adviceBead response.
func toAdviceBead(b *beadsapi.BeadDetail) adviceBead {
	return adviceBead{
		ID:          b.ID,
		Title:       b.Title,
		Labels:      b.Labels,
		Description: b.Description,
		Status:      b.Status,
	}
}

// writeJSON writes a JSON response with status 200.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
