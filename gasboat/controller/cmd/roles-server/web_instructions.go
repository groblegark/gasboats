package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"gasboat/controller/internal/beadsapi"
)

// instructionSections are the known claude-instructions value keys,
// in display order.
var instructionSections = []instructionSection{
	{Key: "identity", Label: "Identity", Hint: "Agent identity description"},
	{Key: "lifecycle", Label: "Lifecycle", Hint: "Agent lifecycle (ephemeral/persistent/single-task)"},
	{Key: "core_rules", Label: "Core Rules", Hint: "Fundamental rules (use kd, no TodoWrite)"},
	{Key: "commands", Label: "Commands", Hint: "CLI command reference"},
	{Key: "workflows", Label: "Workflows", Hint: "Common workflow patterns"},
	{Key: "decisions", Label: "Decisions", Hint: "Human decision protocol"},
	{Key: "session_resumption", Label: "Session Resumption", Hint: "How to resume after interruption"},
	{Key: "session_close", Label: "Session Close", Hint: "Git push checklist"},
	{Key: "stop_gate", Label: "Stop Gate", Hint: "Stop gate contract"},
	{Key: "prime_header", Label: "Prime Header", Hint: "Context recovery header for gb prime"},
	{Key: "stop_gate_blocked", Label: "Stop Gate Blocked Text", Hint: "Text injected by stop-gate.sh hook"},
	{Key: "claude_md", Label: "CLAUDE.md", Hint: "CLAUDE.md content written by gb setup"},
}

type instructionSection struct {
	Key   string
	Label string
	Hint  string
}

// InstructionsUI serves HTML pages for editing claude-instructions config beads.
type InstructionsUI struct {
	client *beadsapi.Client
	logger *slog.Logger
	tmpl   *templateSet
}

// NewInstructionsUI creates a new instructions UI handler.
func NewInstructionsUI(client *beadsapi.Client, logger *slog.Logger, ts *templateSet) *InstructionsUI {
	return &InstructionsUI{client: client, logger: logger, tmpl: ts}
}

// RegisterRoutes registers instruction editing routes.
func (ui *InstructionsUI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /config-beads/{id}/instructions", ui.handleEditInstructions)
	mux.HandleFunc("POST /config-beads/{id}/instructions", ui.handleUpdateInstructions)
}

type instructionsFormData struct {
	ID       string
	Labels   string
	Sections []sectionFormData
	Error    string
}

type sectionFormData struct {
	Key   string
	Label string
	Hint  string
	Value string
}

func (ui *InstructionsUI) handleEditInstructions(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	bead, err := ui.client.GetBead(r.Context(), beadID)
	if err != nil {
		ui.logger.Error("failed to get config bead", "id", beadID, "error", err)
		http.Error(w, "config bead not found", http.StatusNotFound)
		return
	}

	if bead.Title != "claude-instructions" {
		http.Error(w, "this editor is only for claude-instructions config beads", http.StatusBadRequest)
		return
	}

	// Parse the value JSON into a map.
	values := make(map[string]string)
	if raw, ok := bead.Fields["value"]; ok && raw != "" {
		var parsed map[string]any
		if json.Unmarshal([]byte(raw), &parsed) == nil {
			for k, v := range parsed {
				switch val := v.(type) {
				case string:
					values[k] = val
				default:
					b, _ := json.MarshalIndent(val, "", "  ")
					values[k] = string(b)
				}
			}
		}
	}

	sections := make([]sectionFormData, len(instructionSections))
	for i, s := range instructionSections {
		sections[i] = sectionFormData{
			Key:   s.Key,
			Label: s.Label,
			Hint:  s.Hint,
			Value: values[s.Key],
		}
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := ui.tmpl.t.ExecuteTemplate(w, "instructions_form", instructionsFormData{
		ID:       beadID,
		Labels:   strings.Join(bead.Labels, ", "),
		Sections: sections,
	}); err != nil {
		ui.logger.Error("template execution failed", "error", err)
	}
}

func (ui *InstructionsUI) handleUpdateInstructions(w http.ResponseWriter, r *http.Request) {
	beadID := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form data", http.StatusBadRequest)
		return
	}

	// Build the value JSON from individual section fields.
	valueMap := make(map[string]string)
	for _, s := range instructionSections {
		val := r.FormValue("section_" + s.Key)
		if strings.TrimSpace(val) != "" {
			valueMap[s.Key] = val
		}
	}

	valueJSON, err := json.Marshal(valueMap)
	if err != nil {
		ui.logger.Error("failed to marshal instructions", "error", err)
		http.Error(w, "failed to marshal value", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()
	if err := ui.client.UpdateBeadFields(ctx, beadID, map[string]string{
		"value": string(valueJSON),
	}); err != nil {
		ui.logger.Error("failed to update instructions", "id", beadID, "error", err)

		// Re-render form with error.
		sections := make([]sectionFormData, len(instructionSections))
		for i, s := range instructionSections {
			sections[i] = sectionFormData{
				Key:   s.Key,
				Label: s.Label,
				Hint:  s.Hint,
				Value: r.FormValue("section_" + s.Key),
			}
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = ui.tmpl.t.ExecuteTemplate(w, "instructions_form", instructionsFormData{
			ID:       beadID,
			Labels:   r.FormValue("labels"),
			Sections: sections,
			Error:    fmt.Sprintf("failed to update: %v", err),
		})
		return
	}

	http.Redirect(w, r, "/config-beads", http.StatusSeeOther)
}
