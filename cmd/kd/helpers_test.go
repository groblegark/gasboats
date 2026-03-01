package main

import (
	"strings"
	"testing"
	"time"

	"github.com/groblegark/kbeads/internal/model"
)

// --- hasTargetingLabel ---

func TestHasTargetingLabel(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   bool
	}{
		{"nil labels", nil, false},
		{"empty labels", []string{}, false},
		{"no targeting", []string{"project:foo", "priority:high"}, false},
		{"global", []string{"project:foo", "global"}, true},
		{"rig prefix", []string{"rig:my-rig"}, true},
		{"role prefix", []string{"role:crew"}, true},
		{"agent prefix", []string{"agent:my-agent"}, true},
		{"partial rig (too short)", []string{"rig:"}, false},
		{"partial role (too short)", []string{"role:"}, false},
		{"partial agent (too short)", []string{"agent"}, false},
		{"global among others", []string{"project:x", "global", "tag:y"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasTargetingLabel(tt.labels)
			if got != tt.want {
				t.Errorf("hasTargetingLabel(%v) = %v, want %v", tt.labels, got, tt.want)
			}
		})
	}
}

// --- filterJackLabels ---

func TestFilterJackLabels(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   int
	}{
		{"nil", nil, 0},
		{"no jack labels", []string{"project:foo", "role:crew"}, 0},
		{"one jack label", []string{"jack:my-jack", "project:foo"}, 1},
		{"multiple jack labels", []string{"jack:a", "jack:b", "other"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterJackLabels(tt.labels)
			if len(got) != tt.want {
				t.Errorf("filterJackLabels returned %d labels, want %d", len(got), tt.want)
			}
			for _, l := range got {
				if !strings.HasPrefix(l, "jack:") {
					t.Errorf("expected jack: prefix, got %q", l)
				}
			}
		})
	}
}

// --- truncate ---

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello..."},
		{"abcdefgh", 5, "ab..."},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := truncate(tt.input, tt.max)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
			}
		})
	}
}

// --- strPtr / intPtr ---

func TestStrPtr(t *testing.T) {
	p := strPtr("hello")
	if *p != "hello" {
		t.Errorf("*strPtr = %q, want hello", *p)
	}
}

func TestIntPtr(t *testing.T) {
	p := intPtr(42)
	if *p != 42 {
		t.Errorf("*intPtr = %d, want 42", *p)
	}
}

// --- agentProject / defaultProject ---

func TestDefaultProject(t *testing.T) {
	t.Run("KD_PROJECT set", func(t *testing.T) {
		t.Setenv("KD_PROJECT", "myproj")
		t.Setenv("BOAT_PROJECT", "other")
		if got := defaultProject(); got != "myproj" {
			t.Errorf("defaultProject() = %q, want myproj", got)
		}
	})
	t.Run("BOAT_PROJECT fallback", func(t *testing.T) {
		t.Setenv("KD_PROJECT", "")
		t.Setenv("BOAT_PROJECT", "boatproj")
		if got := defaultProject(); got != "boatproj" {
			t.Errorf("defaultProject() = %q, want boatproj", got)
		}
	})
	t.Run("both empty", func(t *testing.T) {
		t.Setenv("KD_PROJECT", "")
		t.Setenv("BOAT_PROJECT", "")
		if got := defaultProject(); got != "" {
			t.Errorf("defaultProject() = %q, want empty", got)
		}
	})
}

func TestAgentProject(t *testing.T) {
	t.Run("from KD_PROJECT", func(t *testing.T) {
		t.Setenv("KD_PROJECT", "proj1")
		t.Setenv("BOAT_PROJECT", "")
		t.Setenv("BEADS_AGENT_NAME", "")
		if got := agentProject(); got != "proj1" {
			t.Errorf("agentProject() = %q, want proj1", got)
		}
	})
	t.Run("from BEADS_AGENT_NAME", func(t *testing.T) {
		t.Setenv("KD_PROJECT", "")
		t.Setenv("BOAT_PROJECT", "")
		t.Setenv("BEADS_AGENT_NAME", "gasboat/gb-zeta")
		if got := agentProject(); got != "gasboat" {
			t.Errorf("agentProject() = %q, want gasboat", got)
		}
	})
	t.Run("BEADS_AGENT_NAME no slash", func(t *testing.T) {
		t.Setenv("KD_PROJECT", "")
		t.Setenv("BOAT_PROJECT", "")
		t.Setenv("BEADS_AGENT_NAME", "standalone")
		if got := agentProject(); got != "" {
			t.Errorf("agentProject() = %q, want empty (no slash)", got)
		}
	})
	t.Run("all empty", func(t *testing.T) {
		t.Setenv("KD_PROJECT", "")
		t.Setenv("BOAT_PROJECT", "")
		t.Setenv("BEADS_AGENT_NAME", "")
		if got := agentProject(); got != "" {
			t.Errorf("agentProject() = %q, want empty", got)
		}
	})
}

// --- filterDepsByType ---

func TestFilterDepsByType(t *testing.T) {
	deps := []*model.Dependency{
		{BeadID: "a", DependsOnID: "b", Type: "blocks"},
		{BeadID: "a", DependsOnID: "c", Type: "parent-child"},
		{BeadID: "a", DependsOnID: "d", Type: "related"},
	}

	t.Run("filter blocks", func(t *testing.T) {
		got := filterDepsByType(deps, []string{"blocks"})
		if len(got) != 1 {
			t.Fatalf("expected 1, got %d", len(got))
		}
		if got[0].DependsOnID != "b" {
			t.Errorf("expected dep on b, got %s", got[0].DependsOnID)
		}
	})

	t.Run("filter multiple types", func(t *testing.T) {
		got := filterDepsByType(deps, []string{"blocks", "related"})
		if len(got) != 2 {
			t.Fatalf("expected 2, got %d", len(got))
		}
	})

	t.Run("no match", func(t *testing.T) {
		got := filterDepsByType(deps, []string{"nonexistent"})
		if len(got) != 0 {
			t.Errorf("expected 0, got %d", len(got))
		}
	})
}

// --- printBeadJSON ---

func TestPrintBeadJSON(t *testing.T) {
	bead := &model.Bead{
		ID:    "kd-abc",
		Title: "Test bead",
		Type:  "task",
	}
	out := captureStdout(t, func() {
		printBeadJSON(bead)
	})
	if !strings.Contains(out, `"id": "kd-abc"`) {
		t.Errorf("printBeadJSON output missing id, got:\n%s", out)
	}
	if !strings.Contains(out, `"title": "Test bead"`) {
		t.Errorf("printBeadJSON output missing title, got:\n%s", out)
	}
}

// --- printBeadListJSON ---

func TestPrintBeadListJSON(t *testing.T) {
	beads := []*model.Bead{
		{ID: "kd-1", Title: "First"},
		{ID: "kd-2", Title: "Second"},
	}
	out := captureStdout(t, func() {
		printBeadListJSON(beads)
	})
	if !strings.Contains(out, "kd-1") || !strings.Contains(out, "kd-2") {
		t.Errorf("printBeadListJSON missing bead IDs, got:\n%s", out)
	}
}

// --- printBeadListTable ---

func TestPrintBeadListTable(t *testing.T) {
	beads := []*model.Bead{
		{ID: "kd-1", Title: "First", Status: "open", Type: "task", Priority: 2},
		{ID: "kd-2", Title: "Second", Status: "closed", Type: "bug", Priority: 1},
	}
	out := captureStdout(t, func() {
		printBeadListTable(beads, 10)
	})
	if !strings.Contains(out, "kd-1") {
		t.Errorf("output missing kd-1, got:\n%s", out)
	}
	if !strings.Contains(out, "2 beads (10 total)") {
		t.Errorf("output missing total count, got:\n%s", out)
	}
}

func TestPrintBeadListTableTruncatesLongTitle(t *testing.T) {
	longTitle := strings.Repeat("x", 60)
	beads := []*model.Bead{
		{ID: "kd-1", Title: longTitle, Status: "open", Type: "task"},
	}
	out := captureStdout(t, func() {
		printBeadListTable(beads, 1)
	})
	if strings.Contains(out, longTitle) {
		t.Error("expected long title to be truncated")
	}
	if !strings.Contains(out, "...") {
		t.Error("expected truncated title to have ellipsis")
	}
}

// --- printBeadTable ---

func TestPrintBeadTable(t *testing.T) {
	now := time.Now()
	bead := &model.Bead{
		ID:          "kd-abc",
		Title:       "Test bead",
		Type:        "task",
		Kind:        "issue",
		Status:      "open",
		Priority:    2,
		Assignee:    "alice",
		Description: "A description",
		Labels:      []string{"project:foo"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	out := captureStdout(t, func() {
		printBeadTable(bead)
	})
	for _, want := range []string{"kd-abc", "Test bead", "task", "issue", "open", "alice", "A description", "project:foo"} {
		if !strings.Contains(out, want) {
			t.Errorf("printBeadTable output missing %q", want)
		}
	}
}

// --- printConfigJSON ---

func TestPrintConfigJSON(t *testing.T) {
	c := &model.Config{
		Key:       "test.key",
		Value:     []byte(`{"enabled":true}`),
		CreatedAt: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
	}
	out := captureStdout(t, func() {
		printConfigJSON(c)
	})
	if !strings.Contains(out, "test.key") {
		t.Errorf("output missing key, got:\n%s", out)
	}
	if !strings.Contains(out, "enabled") {
		t.Errorf("output missing value, got:\n%s", out)
	}
	if !strings.Contains(out, "2025-06-01") {
		t.Errorf("output missing created_at, got:\n%s", out)
	}
}

// --- printComments ---

func TestPrintCommentsNilEmpty(t *testing.T) {
	out := captureStdout(t, func() {
		printComments(nil)
	})
	if out != "" {
		t.Errorf("expected empty output for nil comments, got %q", out)
	}
}

func TestPrintCommentsOutput(t *testing.T) {
	comments := []*model.Comment{
		{Author: "alice", Text: "looks good", CreatedAt: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)},
		{Author: "bob", Text: "needs work"},
	}
	out := captureStdout(t, func() {
		printComments(comments)
	})
	if !strings.Contains(out, "Comments:") {
		t.Error("expected Comments: header")
	}
	if !strings.Contains(out, "alice: looks good") {
		t.Error("expected alice's comment")
	}
	if !strings.Contains(out, "bob: needs work") {
		t.Error("expected bob's comment")
	}
}

// --- printDepSubSection ---

func TestPrintDepSubSectionEmpty(t *testing.T) {
	out := captureStdout(t, func() {
		printDepSubSection(nil, nil)
	})
	if out != "" {
		t.Errorf("expected empty output for nil deps, got %q", out)
	}
}

func TestPrintDepSubSectionResolved(t *testing.T) {
	deps := []resolvedDep{
		{
			Dep:  &model.Dependency{Type: "blocks", DependsOnID: "kd-target"},
			Bead: &model.Bead{ID: "kd-target", Title: "Target bead", Status: "open"},
		},
	}
	out := captureStdout(t, func() {
		printDepSubSection(deps, []string{"id", "title", "status"})
	})
	if !strings.Contains(out, "blocks:") {
		t.Error("expected blocks: prefix")
	}
	if !strings.Contains(out, "kd-target") {
		t.Error("expected target bead ID")
	}
}

func TestPrintDepSubSectionUnresolved(t *testing.T) {
	deps := []resolvedDep{
		{
			Dep:  &model.Dependency{Type: "parent-child", DependsOnID: "kd-missing"},
			Bead: nil,
		},
	}
	out := captureStdout(t, func() {
		printDepSubSection(deps, nil)
	})
	if !strings.Contains(out, "unresolved") {
		t.Error("expected (unresolved) for nil bead")
	}
	if !strings.Contains(out, "kd-missing") {
		t.Error("expected missing bead ID")
	}
}
