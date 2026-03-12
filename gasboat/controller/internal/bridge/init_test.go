package bridge

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"gasboat/controller/internal/beadsapi"
)

type mockConfigSetter struct {
	beads []*beadsapi.BeadDetail
}

func (m *mockConfigSetter) CreateBead(_ context.Context, req beadsapi.CreateBeadRequest) (string, error) {
	id := "kd-mock-" + req.Title
	m.beads = append(m.beads, &beadsapi.BeadDetail{
		ID:          id,
		Title:       req.Title,
		Type:        req.Type,
		Description: req.Description,
		Labels:      req.Labels,
		Status:      "open",
	})
	return id, nil
}

func (m *mockConfigSetter) ListBeadsFiltered(_ context.Context, _ beadsapi.ListBeadsQuery) (*beadsapi.ListBeadsResult, error) {
	return &beadsapi.ListBeadsResult{Beads: m.beads, Total: len(m.beads)}, nil
}

func (m *mockConfigSetter) UpdateBeadDescription(_ context.Context, beadID, description string) error {
	for _, b := range m.beads {
		if b.ID == beadID {
			b.Description = description
			break
		}
	}
	return nil
}

// findBead returns the first config bead matching the given title and labels.
func findBead(beads []*beadsapi.BeadDetail, title string, labels []string) *beadsapi.BeadDetail {
	for _, b := range beads {
		if b.Title == title && labelsMatch(b.Labels, labels) {
			return b
		}
	}
	return nil
}

// findConfigBeadDescription finds a config bead by title and labels and returns
// its description. Calls t.Fatal if not found.
func findConfigBeadDescription(t *testing.T, beads []*beadsapi.BeadDetail, title string, labels []string) string {
	t.Helper()
	for _, b := range beads {
		if b.Title == title && labelsMatch(b.Labels, labels) {
			return b.Description
		}
	}
	t.Fatalf("expected config bead with title=%q labels=%v", title, labels)
	return ""
}

func TestEnsureConfigs_SeedsNudgePrompts(t *testing.T) {
	setter := &mockConfigSetter{}
	if err := EnsureConfigs(context.Background(), setter, slog.Default()); err != nil {
		t.Fatalf("EnsureConfigs: %v", err)
	}

	// config:* entries are written as config beads, not to KV.
	raw := findConfigBeadDescription(t, setter.beads, "nudge-prompts", []string{"global"})

	var prompts map[string]string
	if err := json.Unmarshal([]byte(raw), &prompts); err != nil {
		t.Fatalf("unmarshal nudge-prompts: %v", err)
	}

	for _, key := range []string{"thread", "adhoc", "default", "prewarmed"} {
		if prompts[key] == "" {
			t.Errorf("nudge-prompts missing key %q", key)
		}
	}

	// Adhoc prompt should use template variable, not hardcoded "Create a bead".
	if val := prompts["adhoc"]; val != "" {
		if strings.Contains(val, "Create a bead") {
			t.Error("adhoc prompt should not say 'Create a bead' — should use {{.TaskHint}}")
		}
		if !strings.Contains(val, "{{.TaskHint}}") {
			t.Error("adhoc prompt should contain {{.TaskHint}} template variable")
		}
		if !strings.Contains(val, "{{.BoatPrompt}}") {
			t.Error("adhoc prompt should contain {{.BoatPrompt}} template variable")
		}
	}
}

func TestEnsureConfigs_SeedsClaudeInstructions(t *testing.T) {
	setter := &mockConfigSetter{}
	if err := EnsureConfigs(context.Background(), setter, slog.Default()); err != nil {
		t.Fatalf("EnsureConfigs: %v", err)
	}

	// config:* entries are written as config beads, not to KV.
	raw := findConfigBeadDescription(t, setter.beads, "claude-instructions", []string{"global"})

	var sections map[string]string
	if err := json.Unmarshal([]byte(raw), &sections); err != nil {
		t.Fatalf("unmarshal claude-instructions: %v", err)
	}

	expectedKeys := []string{
		"prime_header", "session_close", "core_rules", "commands",
		"workflows", "decisions", "session_resumption", "lifecycle",
		"stop_gate", "stop_gate_blocked",
	}
	for _, key := range expectedKeys {
		if sections[key] == "" {
			t.Errorf("claude-instructions missing key %q", key)
		}
	}

	// stop_gate_blocked should contain system-reminder tags.
	if val := sections["stop_gate_blocked"]; !strings.Contains(val, "<system-reminder>") {
		t.Error("stop_gate_blocked should contain <system-reminder> tags")
	}
}

func TestEnsureConfigs_SeedsRoleOverrides(t *testing.T) {
	setter := &mockConfigSetter{}
	if err := EnsureConfigs(context.Background(), setter, slog.Default()); err != nil {
		t.Fatalf("EnsureConfigs: %v", err)
	}

	// config:* entries are written as config beads, not to KV.
	for _, role := range []string{"thread", "polecat"} {
		raw := findConfigBeadDescription(t, setter.beads, "claude-instructions", []string{"role:" + role})

		var sections map[string]string
		if err := json.Unmarshal([]byte(raw), &sections); err != nil {
			t.Errorf("unmarshal role:%s: %v", role, err)
			continue
		}

		if sections["commands"] == "" {
			t.Errorf("role:%s missing commands section", role)
		}
		if sections["lifecycle"] == "" {
			t.Errorf("role:%s missing lifecycle section", role)
		}
	}

	// Thread lifecycle should mention "stay alive" / not gb done.
	raw := findConfigBeadDescription(t, setter.beads, "claude-instructions", []string{"role:thread"})
	var threadSections map[string]string
	_ = json.Unmarshal([]byte(raw), &threadSections)
	if !strings.Contains(threadSections["lifecycle"], "stay alive") {
		t.Error("thread lifecycle should mention staying alive")
	}

	// Polecat lifecycle should mention single-task.
	raw = findConfigBeadDescription(t, setter.beads, "claude-instructions", []string{"role:polecat"})
	var polecatSections map[string]string
	_ = json.Unmarshal([]byte(raw), &polecatSections)
	if !strings.Contains(polecatSections["lifecycle"], "single-task") {
		t.Error("polecat lifecycle should mention single-task")
	}
}

func TestEnsureConfigs_SeedsAllExpectedTypes(t *testing.T) {
	setter := &mockConfigSetter{}
	if err := EnsureConfigs(context.Background(), setter, slog.Default()); err != nil {
		t.Fatalf("EnsureConfigs: %v", err)
	}

	// type:* and view:* entries become global config beads with the full key as title.
	// context:* entries become config beads with title="context" and role labels.
	expected := []struct {
		title  string
		labels []string
	}{
		{"type:agent", []string{"global"}},
		{"type:project", []string{"global"}},
		{"type:task", []string{"global"}},
		{"view:agents:active", []string{"global"}},
		{"view:decisions:pending", []string{"global"}},
		{"context", []string{"role:crew"}},
	}

	for _, e := range expected {
		if findBead(setter.beads, e.title, e.labels) == nil {
			t.Errorf("expected config bead with title=%q labels=%v", e.title, e.labels)
		}
	}
}

func TestEnsureConfigs_CreatesConfigBeads(t *testing.T) {
	setter := &mockConfigSetter{}
	if err := EnsureConfigs(context.Background(), setter, slog.Default()); err != nil {
		t.Fatalf("EnsureConfigs: %v", err)
	}

	// Should have created config beads for all entries.
	expectedBeads := []struct {
		title  string
		labels []string
	}{
		{"nudge-prompts", []string{"global"}},
		{"claude-instructions", []string{"global"}},
		{"claude-instructions", []string{"role:thread"}},
		{"claude-instructions", []string{"role:polecat"}},
		{"type:agent", []string{"global"}},
		{"type:task", []string{"global"}},
		{"view:agents:active", []string{"global"}},
		{"context", []string{"role:captain"}},
	}

	for _, e := range expectedBeads {
		b := findBead(setter.beads, e.title, e.labels)
		if b == nil {
			t.Errorf("expected config bead with title=%q labels=%v", e.title, e.labels)
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(b.Description), &m); err != nil {
			t.Errorf("config bead %s %v has invalid JSON description: %v", e.title, e.labels, err)
		}
	}
}

func TestEnsureConfigs_UpdatesExistingConfigBead(t *testing.T) {
	setter := &mockConfigSetter{}

	// Pre-seed a config bead that EnsureConfigs should update.
	setter.beads = append(setter.beads, &beadsapi.BeadDetail{
		ID:          "kd-existing",
		Title:       "nudge-prompts",
		Type:        "config",
		Description: `{"old":"value"}`,
		Labels:      []string{"global"},
		Status:      "open",
	})

	if err := EnsureConfigs(context.Background(), setter, slog.Default()); err != nil {
		t.Fatalf("EnsureConfigs: %v", err)
	}

	// Should have updated the existing bead, not created a new one.
	nudgeCount := 0
	for _, b := range setter.beads {
		if b.Title == "nudge-prompts" && labelsMatch(b.Labels, []string{"global"}) {
			nudgeCount++
		}
	}
	if nudgeCount != 1 {
		t.Errorf("expected 1 nudge-prompts bead (upsert), got %d", nudgeCount)
	}

	// The description should be updated to the current value.
	for _, b := range setter.beads {
		if b.ID == "kd-existing" {
			var prompts map[string]string
			if err := json.Unmarshal([]byte(b.Description), &prompts); err != nil {
				t.Fatalf("unmarshal updated description: %v", err)
			}
			if prompts["thread"] == "" {
				t.Error("expected thread prompt in updated description")
			}
			break
		}
	}
}

func TestParseEntryKey(t *testing.T) {
	tests := []struct {
		key        string
		wantTitle  string
		wantLabels []string
	}{
		// config:* keys.
		{"config:nudge-prompts:global", "nudge-prompts", []string{"global"}},
		{"config:claude-instructions:global", "claude-instructions", []string{"global"}},
		{"config:claude-instructions:role:thread", "claude-instructions", []string{"role:thread"}},
		{"config:claude-instructions:role:polecat", "claude-instructions", []string{"role:polecat"}},
		// Role-keyed namespaces.
		{"context:captain", "context", []string{"role:captain"}},
		{"context:crew", "context", []string{"role:crew"}},
		{"context:global", "context", []string{"global"}},
		// Definition namespaces (full key = title, always global).
		{"type:agent", "type:agent", []string{"global"}},
		{"type:task", "type:task", []string{"global"}},
		{"view:agents:active", "view:agents:active", []string{"global"}},
		{"view:ready", "view:ready", []string{"global"}},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			title, labels := parseEntryKey(tt.key)
			if title != tt.wantTitle {
				t.Errorf("title: got %q, want %q", title, tt.wantTitle)
			}
			if !labelsMatch(labels, tt.wantLabels) {
				t.Errorf("labels: got %v, want %v", labels, tt.wantLabels)
			}
		})
	}
}
