package main

import (
	"context"
	"testing"

	"gasboat/controller/internal/beadsapi"
)

// --- Mock for config bead resolution ---

type mockConfigBeadLister struct {
	beads []*beadsapi.BeadDetail
	err   error
}

func (m *mockConfigBeadLister) ListBeadsFiltered(_ context.Context, q beadsapi.ListBeadsQuery) (*beadsapi.ListBeadsResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &beadsapi.ListBeadsResult{Beads: m.beads, Total: len(m.beads)}, nil
}

// --- ResolveConfigBeads tests ---

func TestResolveConfigBeads_GlobalOnly(t *testing.T) {
	lister := &mockConfigBeadLister{
		beads: []*beadsapi.BeadDetail{
			{
				Title:       "claude-settings",
				Labels:      []string{"global"},
				Description: `{"model":"sonnet","permissions":{"allow":["Bash(*)"]}}`,
			},
		},
	}

	subs := []string{"global", "role:captain"}
	merged, count := ResolveConfigBeads(context.Background(), lister, "claude-settings", subs)

	if count != 1 {
		t.Fatalf("expected 1 layer, got %d", count)
	}
	if merged["model"] != "sonnet" {
		t.Errorf("expected model=sonnet, got %v", merged["model"])
	}
}

func TestResolveConfigBeads_RoleOverridesGlobal(t *testing.T) {
	lister := &mockConfigBeadLister{
		beads: []*beadsapi.BeadDetail{
			{
				Title:       "claude-settings",
				Labels:      []string{"global"},
				Description: `{"model":"sonnet","alwaysThinkingEnabled":true}`,
			},
			{
				Title:       "claude-settings",
				Labels:      []string{"role:captain"},
				Description: `{"model":"opus"}`,
			},
		},
	}

	subs := []string{"global", "role:captain"}
	merged, count := ResolveConfigBeads(context.Background(), lister, "claude-settings", subs)

	if count != 2 {
		t.Fatalf("expected 2 layers, got %d", count)
	}
	if merged["model"] != "opus" {
		t.Errorf("expected model=opus (role override), got %v", merged["model"])
	}
	if merged["alwaysThinkingEnabled"] != true {
		t.Error("expected alwaysThinkingEnabled from global layer")
	}
}

func TestResolveConfigBeads_HooksConcat(t *testing.T) {
	lister := &mockConfigBeadLister{
		beads: []*beadsapi.BeadDetail{
			{
				Title:       "claude-hooks",
				Labels:      []string{"global"},
				Description: `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"gb hook prime"}]}]}}`,
			},
			{
				Title:       "claude-hooks",
				Labels:      []string{"role:captain"},
				Description: `{"hooks":{"SessionStart":[{"matcher":"","hooks":[{"type":"command","command":"gb hook extra"}]}]}}`,
			},
		},
	}

	subs := []string{"global", "role:captain"}
	merged, count := ResolveConfigBeads(context.Background(), lister, "claude-hooks", subs)

	if count != 2 {
		t.Fatalf("expected 2 layers, got %d", count)
	}
	hooks, ok := merged["hooks"].(map[string]any)
	if !ok {
		t.Fatal("expected hooks key")
	}
	sessionStart, ok := hooks["SessionStart"].([]any)
	if !ok {
		t.Fatal("expected SessionStart array")
	}
	if len(sessionStart) != 2 {
		t.Errorf("expected 2 SessionStart hooks (concatenated), got %d", len(sessionStart))
	}
}

func TestResolveConfigBeads_FiltersNonMatching(t *testing.T) {
	lister := &mockConfigBeadLister{
		beads: []*beadsapi.BeadDetail{
			{
				Title:       "claude-settings",
				Labels:      []string{"global"},
				Description: `{"model":"sonnet"}`,
			},
			{
				Title:       "claude-settings",
				Labels:      []string{"role:engineer"},
				Description: `{"model":"haiku"}`,
			},
		},
	}

	// Agent is a captain, should NOT get engineer settings.
	subs := []string{"global", "role:captain"}
	merged, count := ResolveConfigBeads(context.Background(), lister, "claude-settings", subs)

	if count != 1 {
		t.Fatalf("expected 1 matching layer (global only), got %d", count)
	}
	if merged["model"] != "sonnet" {
		t.Errorf("expected model=sonnet (global only), got %v", merged["model"])
	}
}

func TestResolveConfigBeads_FiltersWrongCategory(t *testing.T) {
	lister := &mockConfigBeadLister{
		beads: []*beadsapi.BeadDetail{
			{
				Title:       "claude-mcp",
				Labels:      []string{"global"},
				Description: `{"mcpServers":{}}`,
			},
		},
	}

	subs := []string{"global"}
	merged, count := ResolveConfigBeads(context.Background(), lister, "claude-settings", subs)

	if count != 0 || merged != nil {
		t.Errorf("expected no match for wrong category, got %d layers", count)
	}
}

func TestResolveConfigBeads_NoBeads(t *testing.T) {
	lister := &mockConfigBeadLister{beads: []*beadsapi.BeadDetail{}}

	subs := []string{"global"}
	merged, count := ResolveConfigBeads(context.Background(), lister, "claude-settings", subs)

	if count != 0 || merged != nil {
		t.Errorf("expected 0 layers, got %d", count)
	}
}

func TestResolveConfigBeads_InvalidJSON(t *testing.T) {
	lister := &mockConfigBeadLister{
		beads: []*beadsapi.BeadDetail{
			{
				Title:       "claude-settings",
				Labels:      []string{"global"},
				Description: `not valid json`,
			},
			{
				Title:       "claude-settings",
				Labels:      []string{"role:captain"},
				Description: `{"model":"opus"}`,
			},
		},
	}

	subs := []string{"global", "role:captain"}
	merged, count := ResolveConfigBeads(context.Background(), lister, "claude-settings", subs)

	if count != 1 {
		t.Fatalf("expected 1 valid layer (skipping invalid), got %d", count)
	}
	if merged["model"] != "opus" {
		t.Errorf("expected model=opus, got %v", merged["model"])
	}
}

func TestResolveConfigBeads_UnknownCategory(t *testing.T) {
	lister := &mockConfigBeadLister{beads: []*beadsapi.BeadDetail{}}

	merged, count := ResolveConfigBeads(context.Background(), lister, "nonexistent", []string{"global"})
	if count != 0 || merged != nil {
		t.Error("expected nil for unknown category")
	}
}

func TestResolveConfigBeads_MultiRolePrecedence(t *testing.T) {
	// When an agent has multiple roles (e.g. "thread,crew"), the first role
	// should have higher precedence (its values override later roles).
	lister := &mockConfigBeadLister{
		beads: []*beadsapi.BeadDetail{
			{
				Title:       "claude-settings",
				Labels:      []string{"global"},
				Description: `{"model":"sonnet","base":"yes"}`,
			},
			{
				Title:       "claude-settings",
				Labels:      []string{"role:crew"},
				Description: `{"model":"haiku","crew_setting":"yes"}`,
			},
			{
				Title:       "claude-settings",
				Labels:      []string{"role:thread"},
				Description: `{"model":"opus","thread_setting":"yes"}`,
			},
		},
	}

	// thread,crew -> thread is first (higher precedence)
	subs := []string{"global", "role:thread", "role:crew"}
	merged, count := ResolveConfigBeads(context.Background(), lister, "claude-settings", subs)

	if count != 3 {
		t.Fatalf("expected 3 layers, got %d", count)
	}
	// role:thread should win for model (first role = highest precedence)
	if merged["model"] != "opus" {
		t.Errorf("expected model=opus (first role thread wins), got %v", merged["model"])
	}
	// Both role settings should be present
	if merged["crew_setting"] != "yes" {
		t.Error("expected crew_setting=yes from role:crew layer")
	}
	if merged["thread_setting"] != "yes" {
		t.Error("expected thread_setting=yes from role:thread layer")
	}
	if merged["base"] != "yes" {
		t.Error("expected base=yes from global layer")
	}
}

func TestResolveConfigBeads_SpecificityOrder(t *testing.T) {
	// Verify that global < rig < role in merge order.
	lister := &mockConfigBeadLister{
		beads: []*beadsapi.BeadDetail{
			{
				Title:       "claude-settings",
				Labels:      []string{"role:captain"},
				Description: `{"model":"opus"}`,
			},
			{
				Title:       "claude-settings",
				Labels:      []string{"global"},
				Description: `{"model":"sonnet","extra":"yes"}`,
			},
			{
				Title:       "claude-settings",
				Labels:      []string{"project:gasboat"},
				Description: `{"model":"haiku","project":"gasboat"}`,
			},
		},
	}

	subs := []string{"global", "project:gasboat", "role:captain"}
	merged, count := ResolveConfigBeads(context.Background(), lister, "claude-settings", subs)

	if count != 3 {
		t.Fatalf("expected 3 layers, got %d", count)
	}
	// role:captain is most specific, should win for model.
	if merged["model"] != "opus" {
		t.Errorf("expected model=opus (role wins), got %v", merged["model"])
	}
	// global's "extra" should survive.
	if merged["extra"] != "yes" {
		t.Error("expected extra=yes from global layer")
	}
	// project's "project" should survive (not overridden by role).
	if merged["project"] != "gasboat" {
		t.Error("expected project=gasboat from project layer")
	}
}

func TestResolveConfigBeads_ProjectInlineOverridesRole(t *testing.T) {
	// Project bead inline config (extra layer at "2~:" specificity) should
	// override role config beads but be overridden by agent config beads.
	lister := &mockConfigBeadLister{
		beads: []*beadsapi.BeadDetail{
			{
				Title:       "claude-settings",
				Labels:      []string{"global"},
				Description: `{"model":"sonnet","base":"yes"}`,
			},
			{
				Title:       "claude-settings",
				Labels:      []string{"role:crew"},
				Description: `{"model":"haiku","role_key":"yes"}`,
			},
		},
	}

	// Inject project inline config as an extra layer.
	extras := []resolvedConfig{{
		value:       []byte(`{"model":"opus","project_inline":"yes"}`),
		specificity: projectInlineSpecificity,
	}}

	subs := []string{"global", "role:crew"}
	merged, count := ResolveConfigBeads(context.Background(), lister, "claude-settings", subs, extras...)

	if count != 3 {
		t.Fatalf("expected 3 layers (global + role + inline), got %d", count)
	}
	// Project inline should override role for "model".
	if merged["model"] != "opus" {
		t.Errorf("expected model=opus (project inline wins over role), got %v", merged["model"])
	}
	// Role key should survive.
	if merged["role_key"] != "yes" {
		t.Error("expected role_key=yes from role layer")
	}
	// Project inline key should be present.
	if merged["project_inline"] != "yes" {
		t.Error("expected project_inline=yes from inline layer")
	}
	// Global base should survive.
	if merged["base"] != "yes" {
		t.Error("expected base=yes from global layer")
	}
}

func TestResolveConfigBeads_AgentOverridesProjectInline(t *testing.T) {
	// Agent-level config beads (3:) should override project inline (2~:).
	lister := &mockConfigBeadLister{
		beads: []*beadsapi.BeadDetail{
			{
				Title:       "claude-settings",
				Labels:      []string{"global"},
				Description: `{"model":"sonnet"}`,
			},
			{
				Title:       "claude-settings",
				Labels:      []string{"agent:kd-test-agent"},
				Description: `{"model":"opus"}`,
			},
		},
	}

	extras := []resolvedConfig{{
		value:       []byte(`{"model":"haiku","inline_key":"yes"}`),
		specificity: projectInlineSpecificity,
	}}

	subs := []string{"global", "agent:kd-test-agent"}
	merged, count := ResolveConfigBeads(context.Background(), lister, "claude-settings", subs, extras...)

	if count != 3 {
		t.Fatalf("expected 3 layers, got %d", count)
	}
	// Agent should win for "model".
	if merged["model"] != "opus" {
		t.Errorf("expected model=opus (agent wins over project inline), got %v", merged["model"])
	}
	// Project inline key should survive.
	if merged["inline_key"] != "yes" {
		t.Error("expected inline_key=yes from project inline layer")
	}
}

