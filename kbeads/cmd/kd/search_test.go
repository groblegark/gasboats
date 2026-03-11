package main

import (
	"encoding/json"
	"testing"

	"github.com/groblegark/kbeads/internal/model"
)

func TestResolveStatuses_Positive(t *testing.T) {
	got, err := resolveStatuses([]string{"open", "in_progress"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 || got[0] != "open" || got[1] != "in_progress" {
		t.Errorf("resolveStatuses = %v, want [open in_progress]", got)
	}
}

func TestResolveStatuses_Negated(t *testing.T) {
	got, err := resolveStatuses([]string{"!closed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := map[string]bool{"open": true, "in_progress": true, "deferred": true}
	if len(got) != 3 {
		t.Fatalf("resolveStatuses = %v, want 3 statuses", got)
	}
	for _, s := range got {
		if !expected[s] {
			t.Errorf("unexpected status %q in result", s)
		}
	}
}

func TestResolveStatuses_MultipleNegated(t *testing.T) {
	got, err := resolveStatuses([]string{"!closed", "!deferred"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := map[string]bool{"open": true, "in_progress": true}
	if len(got) != 2 {
		t.Fatalf("resolveStatuses = %v, want 2 statuses", got)
	}
	for _, s := range got {
		if !expected[s] {
			t.Errorf("unexpected status %q in result", s)
		}
	}
}

func TestResolveStatuses_MixedError(t *testing.T) {
	_, err := resolveStatuses([]string{"open", "!closed"})
	if err == nil {
		t.Error("expected error for mixed positive and negated statuses")
	}
}

func TestResolveStatuses_Empty(t *testing.T) {
	got, err := resolveStatuses(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("resolveStatuses(nil) = %v, want empty", got)
	}
}

func TestParseWhereFlags(t *testing.T) {
	tests := []struct {
		name  string
		flags []string
		want  map[string]string
	}{
		{"nil", nil, nil},
		{"single", []string{"project=gasboat"}, map[string]string{"project": "gasboat"}},
		{"empty_value", []string{"project="}, map[string]string{"project": ""}},
		{"multiple", []string{"project=gasboat", "role=crew"}, map[string]string{"project": "gasboat", "role": "crew"}},
		{"value_with_equals", []string{"url=https://example.com/path?a=1"}, map[string]string{"url": "https://example.com/path?a=1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWhereFlags(tt.flags)
			if tt.want == nil {
				if got != nil {
					t.Errorf("parseWhereFlags = %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseWhereFlags = %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseWhereFlags[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

func TestBeadFieldValue(t *testing.T) {
	b := &beadForFieldTest{
		fields: `{"project":"gasboat","count":42,"nested":{"a":"b"}}`,
	}
	bead := b.toBead()

	if got := beadFieldValue(bead, "project"); got != "gasboat" {
		t.Errorf("beadFieldValue(project) = %q, want %q", got, "gasboat")
	}
	if got := beadFieldValue(bead, "count"); got != "42" {
		t.Errorf("beadFieldValue(count) = %q, want %q", got, "42")
	}
	if got := beadFieldValue(bead, "missing"); got != "" {
		t.Errorf("beadFieldValue(missing) = %q, want empty", got)
	}
}

// beadForFieldTest is a helper to create test beads with JSON fields.
type beadForFieldTest struct {
	fields string
}

func (b *beadForFieldTest) toBead() *model.Bead {
	return &model.Bead{
		Fields: json.RawMessage(b.fields),
	}
}
