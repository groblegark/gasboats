package model

import (
	"testing"
)

func TestMatchesSubscriptions(t *testing.T) {
	tests := []struct {
		name         string
		adviceLabels []string
		subs         []string
		want         bool
	}{
		{
			name:         "global matches any agent",
			adviceLabels: []string{"global"},
			subs:         []string{"global", "rig:beads"},
			want:         true,
		},
		{
			name:         "rig match",
			adviceLabels: []string{"rig:beads"},
			subs:         []string{"global", "rig:beads"},
			want:         true,
		},
		{
			name:         "rig mismatch blocks",
			adviceLabels: []string{"rig:gastown"},
			subs:         []string{"global", "rig:beads"},
			want:         false,
		},
		{
			name:         "agent match",
			adviceLabels: []string{"agent:arch-eel"},
			subs:         []string{"global", "agent:arch-eel"},
			want:         true,
		},
		{
			name:         "agent mismatch blocks",
			adviceLabels: []string{"agent:cool-rook"},
			subs:         []string{"global", "agent:arch-eel"},
			want:         false,
		},
		{
			name:         "compound AND within group",
			adviceLabels: []string{"g0:role:polecat", "g0:rig:beads"},
			subs:         []string{"global", "rig:beads", "role:polecat"},
			want:         true,
		},
		{
			name:         "compound AND partial mismatch",
			adviceLabels: []string{"g0:role:polecat", "g0:rig:beads"},
			subs:         []string{"global", "role:polecat"},
			want:         false, // missing rig:beads
		},
		{
			name:         "OR across groups",
			adviceLabels: []string{"g0:role:polecat", "g1:role:crew"},
			subs:         []string{"global", "role:crew"},
			want:         true, // matches group 1
		},
		{
			name:         "OR across groups neither matches",
			adviceLabels: []string{"g0:role:polecat", "g1:role:crew"},
			subs:         []string{"global", "role:witness"},
			want:         false,
		},
		{
			name:         "rig required even with group match",
			adviceLabels: []string{"rig:gastown", "g0:role:polecat"},
			subs:         []string{"global", "rig:beads", "role:polecat"},
			want:         false, // rig:gastown not in subs
		},
		{
			name:         "unprefixed labels are OR",
			adviceLabels: []string{"role:polecat", "role:crew"},
			subs:         []string{"global", "role:crew"},
			want:         true,
		},
		{
			name:         "empty advice labels",
			adviceLabels: []string{},
			subs:         []string{"global"},
			want:         false, // no groups to match
		},
		{
			name:         "empty subscriptions",
			adviceLabels: []string{"global"},
			subs:         []string{},
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesSubscriptions(tt.adviceLabels, tt.subs)
			if got != tt.want {
				t.Errorf("MatchesSubscriptions(%v, %v) = %v, want %v",
					tt.adviceLabels, tt.subs, got, tt.want)
			}
		})
	}
}

func TestParseGroups(t *testing.T) {
	labels := []string{"g0:role:polecat", "g0:rig:beads", "g1:role:crew", "global"}
	groups := ParseGroups(labels)

	// Group 0 should have 2 labels
	if len(groups[0]) != 2 {
		t.Errorf("group 0: got %d labels, want 2", len(groups[0]))
	}

	// Group 1 should have 1 label
	if len(groups[1]) != 1 {
		t.Errorf("group 1: got %d labels, want 1", len(groups[1]))
	}

	// "global" should be in its own high-numbered group
	found := false
	for k, v := range groups {
		if k >= 1000 && len(v) == 1 && v[0] == "global" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'global' in its own unprefixed group")
	}
}

func TestStripGroupPrefix(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"g0:role:polecat", "role:polecat"},
		{"g12:rig:beads", "rig:beads"},
		{"global", "global"},
		{"rig:beads", "rig:beads"},
		{"g:bad", "g:bad"}, // g without digits
	}

	for _, tt := range tests {
		got := StripGroupPrefix(tt.input)
		if got != tt.want {
			t.Errorf("StripGroupPrefix(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildAgentSubscriptions(t *testing.T) {
	subs := BuildAgentSubscriptions("beads/polecats/quartz", nil)

	expected := map[string]bool{
		"global":          true,
		"agent:beads/polecats/quartz": true,
		"rig:beads":       true,
		"role:polecats":   true,
		"role:polecat":    true,
	}

	for _, s := range subs {
		if !expected[s] {
			t.Errorf("unexpected subscription: %q", s)
		}
		delete(expected, s)
	}
	for s := range expected {
		t.Errorf("missing subscription: %q", s)
	}
}
