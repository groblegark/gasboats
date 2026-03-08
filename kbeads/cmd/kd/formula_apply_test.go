package main

import (
	"testing"
)

func TestSubstituteVars(t *testing.T) {
	vars := map[string]string{
		"component": "auth",
		"assignee":  "alice",
	}

	tests := []struct {
		input string
		want  string
	}{
		{"Design {{component}}", "Design auth"},
		{"{{component}} for {{assignee}}", "auth for alice"},
		{"No vars here", "No vars here"},
		{"{{unknown}} stays", "{{unknown}} stays"},
		{"", ""},
		{"{{component}}{{component}}", "authauth"},
	}

	for _, tt := range tests {
		got := substituteVars(tt.input, vars)
		if got != tt.want {
			t.Errorf("substituteVars(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEvaluateCondition(t *testing.T) {
	vars := map[string]string{
		"feature":   "auth",
		"empty":     "",
		"flag":      "true",
		"flagfalse": "false",
		"zero":      "0",
	}

	tests := []struct {
		cond string
		want bool
	}{
		// Truthy
		{"{{feature}}", true},
		{"{{empty}}", false},
		{"{{missing}}", false},
		{"{{flag}}", true},
		{"{{flagfalse}}", false},
		{"{{zero}}", false},

		// Negation
		{"!{{feature}}", false},
		{"!{{empty}}", true},
		{"!{{missing}}", true},

		// Equality
		{"{{feature}} == auth", true},
		{"{{feature}} == notauth", false},
		{"{{feature}} != notauth", true},
		{"{{feature}} != auth", false},
	}

	for _, tt := range tests {
		got := evaluateCondition(tt.cond, vars)
		if got != tt.want {
			t.Errorf("evaluateCondition(%q) = %v, want %v", tt.cond, got, tt.want)
		}
	}
}

func TestMergeLabels(t *testing.T) {
	tests := []struct {
		base  []string
		extra []string
		want  int // expected count
	}{
		{[]string{"a", "b"}, []string{"c"}, 3},
		{[]string{"a", "b"}, []string{"b", "c"}, 3}, // dedup
		{[]string{"a"}, nil, 1},
		{nil, []string{"a"}, 1},
	}

	for _, tt := range tests {
		got := mergeLabels(tt.base, tt.extra)
		if len(got) != tt.want {
			t.Errorf("mergeLabels(%v, %v) len = %d, want %d", tt.base, tt.extra, len(got), tt.want)
		}
	}
}

func TestPriorityOrDefault(t *testing.T) {
	p := 1
	if got := priorityOrDefault(&p, 2); got != 1 {
		t.Errorf("priorityOrDefault(&1, 2) = %d, want 1", got)
	}
	if got := priorityOrDefault(nil, 2); got != 2 {
		t.Errorf("priorityOrDefault(nil, 2) = %d, want 2", got)
	}
}
