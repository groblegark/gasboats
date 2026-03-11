package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"gasboat/controller/internal/beadsapi"
)

// configBeadCategories are the known config bead categories (title values).
var configBeadCategories = []string{
	"claude-settings",
	"claude-hooks",
	"claude-mcp",
	"claude-instructions",
	"type",
	"view",
	"context",
}

// templateSet holds all parsed HTML templates.
type templateSet struct {
	t *template.Template
}

func newTemplateSet() *templateSet {
	allTemplates := navTmpl + indexTmpl +
		configBeadFormTmpl + configBeadListTmpl + configBeadDeleteConfirmTmpl +
		adviceListTmpl + adviceFormTmpl + adviceDeleteConfirmTmpl +
		instructionsFormTmpl +
		rolesListTmpl + rolePreviewTmpl
	t := template.Must(template.New("").Funcs(template.FuncMap{
		"join":     strings.Join,
		"toJSON":   toJSONString,
		"contains": sliceContains,
	}).Parse(allTemplates))
	return &templateSet{t: t}
}

// WebUI serves HTML pages for managing config beads.
type WebUI struct {
	client *beadsapi.Client
	logger *slog.Logger
	tmpl   *templateSet
}

// NewWebUI creates a new web UI handler.
func NewWebUI(client *beadsapi.Client, logger *slog.Logger, ts *templateSet) *WebUI {
	return &WebUI{client: client, logger: logger, tmpl: ts}
}

// RegisterRoutes registers web UI routes on the given mux.
func (ui *WebUI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /config-beads", ui.handleListConfigBeadsPage)
	mux.HandleFunc("GET /config-beads/new", ui.handleNewConfigBead)
	mux.HandleFunc("POST /config-beads/new", ui.handleCreateConfigBead)
	mux.HandleFunc("GET /config-beads/{id}/edit", ui.handleEditConfigBead)
	mux.HandleFunc("POST /config-beads/{id}/edit", ui.handleUpdateConfigBead)
	mux.HandleFunc("GET /config-beads/{id}/delete", ui.handleDeleteConfirm)
	mux.HandleFunc("POST /config-beads/{id}/delete", ui.handleDeleteConfigBead)
}

type formData struct {
	ID         string
	Title      string
	Labels     string
	Value      string
	Categories []string
	Error      string
	IsEdit     bool
}

func (ui *WebUI) handleListConfigBeadsPage(w http.ResponseWriter, r *http.Request) {
	result, err := ui.client.ListBeadsFiltered(r.Context(), beadsapi.ListBeadsQuery{
		Types: []string{"config"},
	})
	if err != nil {
		ui.logger.Error("failed to list config beads", "error", err)
		http.Error(w, "failed to list config beads", http.StatusInternalServerError)
		return
	}

	beads := make([]configBead, 0, len(result.Beads))
	for _, b := range result.Beads {
		beads = append(beads, toConfigBead(b))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.tmpl.t.ExecuteTemplate(w, "config_bead_list", beads); err != nil {
		ui.logger.Error("template execution failed", "error", err)
	}
}

func (ui *WebUI) handleNewConfigBead(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.tmpl.t.ExecuteTemplate(w, "config_bead_form", formData{
		Categories: configBeadCategories,
	}); err != nil {
		ui.logger.Error("template execution failed", "error", err)
	}
}

func (ui *WebUI) handleCreateConfigBead(w http.ResponseWriter, r *http.Request) {
	fd := formData{Categories: configBeadCategories}

	if err := r.ParseForm(); err != nil {
		ui.renderFormError(w, fd, "invalid form data")
		return
	}

	fd.Title = strings.TrimSpace(r.FormValue("title"))
	fd.Labels = strings.TrimSpace(r.FormValue("labels"))
	fd.Value = strings.TrimSpace(r.FormValue("value"))

	if fd.Title == "" {
		ui.renderFormError(w, fd, "title (category) is required")
		return
	}

	if fd.Value != "" && !json.Valid([]byte(fd.Value)) {
		ui.renderFormError(w, fd, "value must be valid JSON")
		return
	}

	labels := parseLabels(fd.Labels)

	// Build fields with the value.
	fields := make(map[string]any)
	if fd.Value != "" {
		fields["value"] = fd.Value
	}
	fieldsJSON, _ := json.Marshal(fields)

	_, err := ui.client.CreateBead(r.Context(), beadsapi.CreateBeadRequest{
		Title:     fd.Title,
		Type:      "config",
		Kind:      "config",
		Labels:    labels,
		Fields:    fieldsJSON,
		CreatedBy: "roles-server",
	})
	if err != nil {
		ui.logger.Error("failed to create config bead", "error", err)
		ui.renderFormError(w, fd, fmt.Sprintf("failed to create: %v", err))
		return
	}

	http.Redirect(w, r, "/config-beads", http.StatusSeeOther)
}

func (ui *WebUI) handleEditConfigBead(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	bead, err := ui.client.GetBead(r.Context(), beadID)
	if err != nil {
		ui.logger.Error("failed to get config bead", "id", beadID, "error", err)
		http.Error(w, "config bead not found", http.StatusNotFound)
		return
	}

	valueStr := ""
	if raw, ok := bead.Fields["value"]; ok && raw != "" {
		var parsed any
		if json.Unmarshal([]byte(raw), &parsed) == nil {
			pretty, _ := json.MarshalIndent(parsed, "", "  ")
			valueStr = string(pretty)
		} else {
			valueStr = raw
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.tmpl.t.ExecuteTemplate(w, "config_bead_form", formData{
		ID:         beadID,
		Title:      bead.Title,
		Labels:     strings.Join(bead.Labels, ", "),
		Value:      valueStr,
		Categories: configBeadCategories,
		IsEdit:     true,
	}); err != nil {
		ui.logger.Error("template execution failed", "error", err)
	}
}

func (ui *WebUI) handleUpdateConfigBead(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	fd := formData{ID: beadID, IsEdit: true, Categories: configBeadCategories}

	if err := r.ParseForm(); err != nil {
		ui.renderFormError(w, fd, "invalid form data")
		return
	}

	fd.Title = strings.TrimSpace(r.FormValue("title"))
	fd.Labels = strings.TrimSpace(r.FormValue("labels"))
	fd.Value = strings.TrimSpace(r.FormValue("value"))

	if fd.Title == "" {
		ui.renderFormError(w, fd, "title (category) is required")
		return
	}

	if fd.Value != "" && !json.Valid([]byte(fd.Value)) {
		ui.renderFormError(w, fd, "value must be valid JSON")
		return
	}

	ctx := r.Context()

	if err := ui.client.UpdateBead(ctx, beadID, beadsapi.UpdateBeadRequest{
		Title: &fd.Title,
	}); err != nil {
		ui.logger.Error("failed to update config bead title", "id", beadID, "error", err)
		ui.renderFormError(w, fd, fmt.Sprintf("failed to update: %v", err))
		return
	}

	if err := ui.client.UpdateBeadFields(ctx, beadID, map[string]string{
		"value": fd.Value,
	}); err != nil {
		ui.logger.Error("failed to update config bead value", "id", beadID, "error", err)
		ui.renderFormError(w, fd, fmt.Sprintf("failed to update value: %v", err))
		return
	}

	// Sync labels: fetch current, remove stale, add new.
	bead, err := ui.client.GetBead(ctx, beadID)
	if err == nil {
		syncLabels(ctx, ui.client, ui.logger, beadID, bead.Labels, parseLabels(fd.Labels))
	}

	http.Redirect(w, r, "/config-beads", http.StatusSeeOther)
}

func (ui *WebUI) handleDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	bead, err := ui.client.GetBead(r.Context(), beadID)
	if err != nil {
		ui.logger.Error("failed to get config bead for delete", "id", beadID, "error", err)
		http.Error(w, "config bead not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.tmpl.t.ExecuteTemplate(w, "config_bead_delete_confirm", toConfigBead(bead)); err != nil {
		ui.logger.Error("template execution failed", "error", err)
	}
}

func (ui *WebUI) handleDeleteConfigBead(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	if err := ui.client.DeleteBead(r.Context(), beadID); err != nil {
		ui.logger.Error("failed to delete config bead", "id", beadID, "error", err)
		http.Error(w, fmt.Sprintf("failed to delete: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/config-beads", http.StatusSeeOther)
}

func (ui *WebUI) renderFormError(w http.ResponseWriter, data formData, errMsg string) {
	data.Error = errMsg
	if data.Categories == nil {
		data.Categories = configBeadCategories
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	if err := ui.tmpl.t.ExecuteTemplate(w, "config_bead_form", data); err != nil {
		ui.logger.Error("template execution failed", "error", err)
	}
}

// parseLabels splits a comma-separated label string into trimmed, non-empty labels.
func parseLabels(raw string) []string {
	var labels []string
	for _, l := range strings.Split(raw, ",") {
		l = strings.TrimSpace(l)
		if l != "" {
			labels = append(labels, l)
		}
	}
	return labels
}

// syncLabels reconciles bead labels: removes stale, adds new.
func syncLabels(ctx context.Context, client *beadsapi.Client, logger *slog.Logger, beadID string, current, desired []string) {
	currentSet := make(map[string]bool, len(current))
	for _, l := range current {
		currentSet[l] = true
	}
	desiredSet := make(map[string]bool, len(desired))
	for _, l := range desired {
		desiredSet[l] = true
	}
	for _, l := range current {
		if !desiredSet[l] {
			if err := client.RemoveLabel(ctx, beadID, l); err != nil {
				logger.Warn("failed to remove label", "bead", beadID, "label", l, "error", err)
			}
		}
	}
	for _, l := range desired {
		if !currentSet[l] {
			if err := client.AddLabel(ctx, beadID, l); err != nil {
				logger.Warn("failed to add label", "bead", beadID, "label", l, "error", err)
			}
		}
	}
}

func toJSONString(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func sliceContains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
