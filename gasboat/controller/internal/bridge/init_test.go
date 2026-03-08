package bridge

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

type mockConfigSetter struct {
	configs map[string][]byte
}

func (m *mockConfigSetter) SetConfig(_ context.Context, key string, value []byte) error {
	m.configs[key] = value
	return nil
}

func TestEnsureConfigs_SeedsNudgePrompts(t *testing.T) {
	setter := &mockConfigSetter{configs: make(map[string][]byte)}
	if err := EnsureConfigs(context.Background(), setter, slog.Default()); err != nil {
		t.Fatalf("EnsureConfigs: %v", err)
	}

	raw, ok := setter.configs["config:nudge-prompts:global"]
	if !ok {
		t.Fatal("expected config:nudge-prompts:global to be seeded")
	}

	var prompts map[string]string
	if err := json.Unmarshal(raw, &prompts); err != nil {
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
	setter := &mockConfigSetter{configs: make(map[string][]byte)}
	if err := EnsureConfigs(context.Background(), setter, slog.Default()); err != nil {
		t.Fatalf("EnsureConfigs: %v", err)
	}

	raw, ok := setter.configs["config:claude-instructions:global"]
	if !ok {
		t.Fatal("expected config:claude-instructions:global to be seeded")
	}

	var sections map[string]string
	if err := json.Unmarshal(raw, &sections); err != nil {
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
	setter := &mockConfigSetter{configs: make(map[string][]byte)}
	if err := EnsureConfigs(context.Background(), setter, slog.Default()); err != nil {
		t.Fatalf("EnsureConfigs: %v", err)
	}

	for _, role := range []string{"thread", "polecat"} {
		key := "config:claude-instructions:role:" + role
		raw, ok := setter.configs[key]
		if !ok {
			t.Errorf("expected %s to be seeded", key)
			continue
		}

		var sections map[string]string
		if err := json.Unmarshal(raw, &sections); err != nil {
			t.Errorf("unmarshal %s: %v", key, err)
			continue
		}

		if sections["commands"] == "" {
			t.Errorf("%s missing commands section", key)
		}
		if sections["lifecycle"] == "" {
			t.Errorf("%s missing lifecycle section", key)
		}
	}

	// Thread lifecycle should mention "stay alive" / not gb done.
	raw := setter.configs["config:claude-instructions:role:thread"]
	var threadSections map[string]string
	_ = json.Unmarshal(raw, &threadSections)
	if !strings.Contains(threadSections["lifecycle"], "stay alive") {
		t.Error("thread lifecycle should mention staying alive")
	}

	// Polecat lifecycle should mention single-task.
	raw = setter.configs["config:claude-instructions:role:polecat"]
	var polecatSections map[string]string
	_ = json.Unmarshal(raw, &polecatSections)
	if !strings.Contains(polecatSections["lifecycle"], "single-task") {
		t.Error("polecat lifecycle should mention single-task")
	}
}

func TestEnsureConfigs_SeedsAllExpectedTypes(t *testing.T) {
	setter := &mockConfigSetter{configs: make(map[string][]byte)}
	if err := EnsureConfigs(context.Background(), setter, slog.Default()); err != nil {
		t.Fatalf("EnsureConfigs: %v", err)
	}

	for _, key := range []string{
		"type:agent", "type:project", "type:task",
		"view:agents:active", "view:decisions:pending",
		"context:crew",
	} {
		if _, ok := setter.configs[key]; !ok {
			t.Errorf("expected config %q to be seeded", key)
		}
	}
}
