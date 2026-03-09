package main

import (
	"html/template"
	"net/http"
)

// WebUI serves HTML pages for viewing roles and configuration.
type WebUI struct {
	api   *RolesAPI
	pages map[string]*template.Template
}

// NewWebUI creates a web UI backed by the roles API.
func NewWebUI(api *RolesAPI) *WebUI {
	pageNames := []string{
		"index.html", "role.html", "config_beads.html",
		"advice.html", "projects.html",
	}
	pages := make(map[string]*template.Template, len(pageNames))
	for _, name := range pageNames {
		pages[name] = template.Must(
			template.New("").ParseFS(templateFS, "templates/layout.html", "templates/"+name),
		)
	}
	return &WebUI{api: api, pages: pages}
}

// RegisterRoutes adds HTML page routes to the mux.
func (u *WebUI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", u.handleIndex)
	mux.HandleFunc("GET /roles/{role}", u.handleRole)
	mux.HandleFunc("GET /config-beads", u.handleConfigBeads)
	mux.HandleFunc("GET /advice", u.handleAdvice)
	mux.HandleFunc("GET /projects", u.handleProjects)
}

func (u *WebUI) render(w http.ResponseWriter, name string, data map[string]any) {
	tmpl, ok := u.pages[name]
	if !ok {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "layout", data); err != nil {
		u.api.logger.Error("template render error", "template", name, "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func (u *WebUI) handleIndex(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	roles, err := u.api.fetchRoles(ctx)
	if err != nil {
		u.api.logger.Error("failed to fetch roles for UI", "error", err)
		http.Error(w, "Failed to load roles", http.StatusInternalServerError)
		return
	}
	u.render(w, "index.html", map[string]any{"Roles": roles})
}

func (u *WebUI) handleRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	roleName := r.PathValue("role")
	role, err := u.api.fetchRole(ctx, roleName)
	if err != nil {
		u.api.logger.Error("failed to fetch role for UI", "role", roleName, "error", err)
		http.Error(w, "Failed to load role", http.StatusInternalServerError)
		return
	}
	u.render(w, "role.html", map[string]any{"Role": role})
}

func (u *WebUI) handleConfigBeads(w http.ResponseWriter, r *http.Request) {
	beads, total, err := u.api.fetchConfigBeads(r)
	if err != nil {
		u.api.logger.Error("failed to list config beads for UI", "error", err)
		http.Error(w, "Failed to load config beads", http.StatusInternalServerError)
		return
	}
	u.render(w, "config_beads.html", map[string]any{"ConfigBeads": beads, "Total": total})
}

func (u *WebUI) handleAdvice(w http.ResponseWriter, r *http.Request) {
	beads, total, err := u.api.fetchAdviceBeads(r)
	if err != nil {
		u.api.logger.Error("failed to list advice beads for UI", "error", err)
		http.Error(w, "Failed to load advice beads", http.StatusInternalServerError)
		return
	}
	u.render(w, "advice.html", map[string]any{"AdviceBeads": beads, "Total": total})
}

func (u *WebUI) handleProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := u.api.fetchProjects(r.Context())
	if err != nil {
		u.api.logger.Error("failed to list projects for UI", "error", err)
		http.Error(w, "Failed to load projects", http.StatusInternalServerError)
		return
	}
	u.render(w, "projects.html", map[string]any{"Projects": projects})
}
