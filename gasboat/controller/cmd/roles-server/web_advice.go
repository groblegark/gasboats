package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"gasboat/controller/internal/beadsapi"
)

// AdviceUI serves HTML pages for managing advice beads.
type AdviceUI struct {
	client *beadsapi.Client
	logger *slog.Logger
	tmpl   *templateSet
}

// NewAdviceUI creates a new advice UI handler.
func NewAdviceUI(client *beadsapi.Client, logger *slog.Logger, ts *templateSet) *AdviceUI {
	return &AdviceUI{client: client, logger: logger, tmpl: ts}
}

// RegisterRoutes registers advice UI routes on the given mux.
func (ui *AdviceUI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /advice", ui.handleListAdvicePage)
	mux.HandleFunc("GET /advice/new", ui.handleNewAdvice)
	mux.HandleFunc("POST /advice/new", ui.handleCreateAdvice)
	mux.HandleFunc("GET /advice/{id}/edit", ui.handleEditAdvice)
	mux.HandleFunc("POST /advice/{id}/edit", ui.handleUpdateAdvice)
	mux.HandleFunc("GET /advice/{id}/delete", ui.handleDeleteConfirm)
	mux.HandleFunc("POST /advice/{id}/delete", ui.handleDeleteAdvice)
}

type adviceFormData struct {
	ID          string
	Title       string
	Labels      string
	Description string
	Error       string
	IsEdit      bool
}

func (ui *AdviceUI) handleListAdvicePage(w http.ResponseWriter, r *http.Request) {
	result, err := ui.client.ListBeadsFiltered(r.Context(), beadsapi.ListBeadsQuery{
		Types:    []string{"advice"},
		Statuses: []string{"open", "in_progress"},
	})
	if err != nil {
		ui.logger.Error("failed to list advice beads", "error", err)
		http.Error(w, "failed to list advice beads", http.StatusInternalServerError)
		return
	}

	beads := make([]adviceBead, 0, len(result.Beads))
	for _, b := range result.Beads {
		beads = append(beads, toAdviceBead(b))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.tmpl.t.ExecuteTemplate(w, "advice_list", beads); err != nil {
		ui.logger.Error("template execution failed", "error", err)
	}
}

func (ui *AdviceUI) handleNewAdvice(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.tmpl.t.ExecuteTemplate(w, "advice_form", adviceFormData{}); err != nil {
		ui.logger.Error("template execution failed", "error", err)
	}
}

func (ui *AdviceUI) handleCreateAdvice(w http.ResponseWriter, r *http.Request) {
	fd := adviceFormData{}

	if err := r.ParseForm(); err != nil {
		ui.renderFormError(w, fd, "invalid form data")
		return
	}

	fd.Title = strings.TrimSpace(r.FormValue("title"))
	fd.Labels = strings.TrimSpace(r.FormValue("labels"))
	fd.Description = strings.TrimSpace(r.FormValue("description"))

	if fd.Title == "" {
		ui.renderFormError(w, fd, "title is required")
		return
	}

	labels := parseLabels(fd.Labels)

	_, err := ui.client.CreateBead(r.Context(), beadsapi.CreateBeadRequest{
		Title:       fd.Title,
		Type:        "advice",
		Kind:        "data",
		Labels:      labels,
		Description: fd.Description,
		CreatedBy:   "roles-server",
	})
	if err != nil {
		ui.logger.Error("failed to create advice bead", "error", err)
		ui.renderFormError(w, fd, fmt.Sprintf("failed to create: %v", err))
		return
	}

	http.Redirect(w, r, "/advice", http.StatusSeeOther)
}

func (ui *AdviceUI) handleEditAdvice(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	bead, err := ui.client.GetBead(r.Context(), beadID)
	if err != nil {
		ui.logger.Error("failed to get advice bead", "id", beadID, "error", err)
		http.Error(w, "advice bead not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.tmpl.t.ExecuteTemplate(w, "advice_form", adviceFormData{
		ID:          beadID,
		Title:       bead.Title,
		Labels:      strings.Join(bead.Labels, ", "),
		Description: bead.Description,
		IsEdit:      true,
	}); err != nil {
		ui.logger.Error("template execution failed", "error", err)
	}
}

func (ui *AdviceUI) handleUpdateAdvice(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	fd := adviceFormData{ID: beadID, IsEdit: true}

	if err := r.ParseForm(); err != nil {
		ui.renderFormError(w, fd, "invalid form data")
		return
	}

	fd.Title = strings.TrimSpace(r.FormValue("title"))
	fd.Labels = strings.TrimSpace(r.FormValue("labels"))
	fd.Description = strings.TrimSpace(r.FormValue("description"))

	if fd.Title == "" {
		ui.renderFormError(w, fd, "title is required")
		return
	}

	ctx := r.Context()

	if err := ui.client.UpdateBead(ctx, beadID, beadsapi.UpdateBeadRequest{
		Title:       &fd.Title,
		Description: &fd.Description,
	}); err != nil {
		ui.logger.Error("failed to update advice bead", "id", beadID, "error", err)
		ui.renderFormError(w, fd, fmt.Sprintf("failed to update: %v", err))
		return
	}

	// Sync labels.
	bead, err := ui.client.GetBead(ctx, beadID)
	if err == nil {
		syncLabels(ctx, ui.client, ui.logger, beadID, bead.Labels, parseLabels(fd.Labels))
	}

	http.Redirect(w, r, "/advice", http.StatusSeeOther)
}

func (ui *AdviceUI) handleDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	bead, err := ui.client.GetBead(r.Context(), beadID)
	if err != nil {
		ui.logger.Error("failed to get advice bead for delete", "id", beadID, "error", err)
		http.Error(w, "advice bead not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.tmpl.t.ExecuteTemplate(w, "advice_delete_confirm", toAdviceBead(bead)); err != nil {
		ui.logger.Error("template execution failed", "error", err)
	}
}

func (ui *AdviceUI) handleDeleteAdvice(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	if err := ui.client.DeleteBead(r.Context(), beadID); err != nil {
		ui.logger.Error("failed to delete advice bead", "id", beadID, "error", err)
		http.Error(w, fmt.Sprintf("failed to delete: %v", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/advice", http.StatusSeeOther)
}

func (ui *AdviceUI) renderFormError(w http.ResponseWriter, data adviceFormData, errMsg string) {
	data.Error = errMsg
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnprocessableEntity)
	if err := ui.tmpl.t.ExecuteTemplate(w, "advice_form", data); err != nil {
		ui.logger.Error("template execution failed", "error", err)
	}
}
