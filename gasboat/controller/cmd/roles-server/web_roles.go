package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"

	"gasboat/controller/internal/beadsapi"
)

// RolesUI serves HTML pages for viewing roles and previewing their configuration.
type RolesUI struct {
	client *beadsapi.Client
	logger *slog.Logger
	tmpl   *templateSet
}

// NewRolesUI creates a new roles UI handler.
func NewRolesUI(client *beadsapi.Client, logger *slog.Logger, ts *templateSet) *RolesUI {
	return &RolesUI{client: client, logger: logger, tmpl: ts}
}

// RegisterRoutes registers roles UI routes on the given mux.
func (ui *RolesUI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /roles", ui.handleListRoles)
	mux.HandleFunc("GET /roles/{role}", ui.handleRolePreview)
}

type rolesListData struct {
	Roles []roleSummary
}

type roleSummary struct {
	Name        string
	ConfigCount int
	AdviceCount int
	AgentCount  int
}

type rolePreviewData struct {
	Name         string
	ConfigBeads  []roleConfigBead
	AdviceBeads  []roleAdviceBead
	ActiveAgents []string
}

type roleConfigBead struct {
	ID     string
	Title  string
	Labels []string
	Value  string
}

type roleAdviceBead struct {
	ID          string
	Title       string
	Labels      []string
	Description string
}

func (ui *RolesUI) handleListRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

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
		configResult, configErr = ui.client.ListBeadsFiltered(ctx, beadsapi.ListBeadsQuery{
			Types: []string{"config"},
		})
	}()
	go func() {
		defer wg.Done()
		adviceResult, adviceErr = ui.client.ListBeadsFiltered(ctx, beadsapi.ListBeadsQuery{
			Types:    []string{"advice"},
			Statuses: []string{"open", "in_progress"},
		})
	}()
	go func() {
		defer wg.Done()
		agents, agentsErr = ui.client.ListAgentBeads(ctx)
	}()
	wg.Wait()

	if configErr != nil {
		ui.logger.Error("failed to list config beads", "error", configErr)
	}
	if adviceErr != nil {
		ui.logger.Error("failed to list advice beads", "error", adviceErr)
	}
	if agentsErr != nil {
		ui.logger.Error("failed to list agents", "error", agentsErr)
	}

	roles := make(map[string]*roleSummary)
	ensureRole := func(name string) *roleSummary {
		rs, ok := roles[name]
		if !ok {
			rs = &roleSummary{Name: name}
			roles[name] = rs
		}
		return rs
	}
	ensureRole("global")

	if configResult != nil {
		for _, b := range configResult.Beads {
			for _, label := range b.Labels {
				if name, ok := strings.CutPrefix(label, "role:"); ok {
					ensureRole(name).ConfigCount++
				}
				if label == "global" {
					ensureRole("global").ConfigCount++
				}
			}
		}
	}

	if adviceResult != nil {
		for _, b := range adviceResult.Beads {
			for _, label := range b.Labels {
				if name, ok := strings.CutPrefix(label, "role:"); ok {
					ensureRole(name).AdviceCount++
				}
				if label == "global" {
					ensureRole("global").AdviceCount++
				}
			}
		}
	}

	for _, ag := range agents {
		if ag.Role != "" {
			ensureRole(ag.Role).AgentCount++
		}
	}

	result := make([]roleSummary, 0, len(roles))
	for _, rs := range roles {
		result = append(result, *rs)
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

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.tmpl.t.ExecuteTemplate(w, "roles_list", rolesListData{Roles: result}); err != nil {
		ui.logger.Error("template execution failed", "error", err)
	}
}

func (ui *RolesUI) handleRolePreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	roleName := r.PathValue("role")

	var labelFilter []string
	if roleName == "global" {
		labelFilter = []string{"global"}
	} else {
		labelFilter = []string{"role:" + roleName}
	}

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
		configResult, configErr = ui.client.ListBeadsFiltered(ctx, beadsapi.ListBeadsQuery{
			Types:  []string{"config"},
			Labels: labelFilter,
		})
	}()
	go func() {
		defer wg.Done()
		adviceResult, adviceErr = ui.client.ListBeadsFiltered(ctx, beadsapi.ListBeadsQuery{
			Types:    []string{"advice"},
			Statuses: []string{"open", "in_progress"},
			Labels:   labelFilter,
		})
	}()
	go func() {
		defer wg.Done()
		agents, agentsErr = ui.client.ListAgentBeads(ctx)
	}()
	wg.Wait()

	if configErr != nil {
		ui.logger.Error("failed to list config beads for role", "role", roleName, "error", configErr)
		http.Error(w, "failed to fetch config beads", http.StatusInternalServerError)
		return
	}
	if adviceErr != nil {
		ui.logger.Error("failed to list advice beads for role", "role", roleName, "error", adviceErr)
		http.Error(w, "failed to fetch advice beads", http.StatusInternalServerError)
		return
	}
	if agentsErr != nil {
		ui.logger.Error("failed to list agents", "error", agentsErr)
	}

	configs := make([]roleConfigBead, 0, len(configResult.Beads))
	for _, b := range configResult.Beads {
		valueStr := ""
		if raw, ok := b.Fields["value"]; ok && raw != "" {
			var parsed any
			if json.Unmarshal([]byte(raw), &parsed) == nil {
				pretty, _ := json.MarshalIndent(parsed, "", "  ")
				valueStr = string(pretty)
			} else {
				valueStr = raw
			}
		}
		configs = append(configs, roleConfigBead{
			ID:     b.ID,
			Title:  b.Title,
			Labels: b.Labels,
			Value:  valueStr,
		})
	}

	advices := make([]roleAdviceBead, 0, len(adviceResult.Beads))
	for _, b := range adviceResult.Beads {
		advices = append(advices, roleAdviceBead{
			ID:          b.ID,
			Title:       b.Title,
			Labels:      b.Labels,
			Description: b.Description,
		})
	}

	var activeAgents []string
	for _, ag := range agents {
		if ag.Role == roleName {
			activeAgents = append(activeAgents, ag.AgentName)
		}
	}

	data := rolePreviewData{
		Name:         roleName,
		ConfigBeads:  configs,
		AdviceBeads:  advices,
		ActiveAgents: activeAgents,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.tmpl.t.ExecuteTemplate(w, "role_preview", data); err != nil {
		ui.logger.Error("template execution failed", "error", err)
	}
}
