package main

import (
	"strings"
	"testing"
)

func TestDetectNudgeType(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		expected string
	}{
		{"default", nil, "default"},
		{"thread", map[string]string{"SLACK_THREAD_CHANNEL": "C123"}, "thread"},
		{"adhoc", map[string]string{"BOAT_PROMPT": "do something"}, "adhoc"},
		{"prewarmed", map[string]string{"BOAT_AGENT_STATE": "prewarmed"}, "prewarmed"},
		{"prewarmed over thread", map[string]string{
			"BOAT_AGENT_STATE":     "prewarmed",
			"SLACK_THREAD_CHANNEL": "C123",
		}, "prewarmed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear relevant env vars.
			for _, k := range []string{"SLACK_THREAD_CHANNEL", "BOAT_PROMPT", "BOAT_AGENT_STATE"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			got := detectNudgeType()
			if got != tt.expected {
				t.Errorf("detectNudgeType() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSubstituteNudgeVars(t *testing.T) {
	vars := nudgeVars{
		Project:      "myproject",
		Role:         "captain",
		ProjectHint:  " focus on myproject",
		TaskHint:     " task kd-123",
		MonorepoHint: " repos hint",
		BoatPrompt:   "do the thing",
	}

	tmpl := "Project: {{.Project}}, role:{{.Role}}, hint:{{.ProjectHint}}, task:{{.TaskHint}}, mono:{{.MonorepoHint}}, prompt:{{.BoatPrompt}}"
	got := substituteNudgeVars(tmpl, vars)
	expected := "Project: myproject, role:captain, hint: focus on myproject, task: task kd-123, mono: repos hint, prompt:do the thing"

	if got != expected {
		t.Errorf("substituteNudgeVars() =\n%s\nwant:\n%s", got, expected)
	}
}

func TestBuildNudgeVars_RoleFromEnv(t *testing.T) {
	t.Setenv("BOAT_ROLE", "worker")
	t.Setenv("BOAT_TASK_ID", "")
	t.Setenv("BOAT_PROMPT", "")
	t.Setenv("BOAT_REFERENCE_REPOS", "")
	vars := buildNudgeVars()
	if vars.Role != "worker" {
		t.Errorf("Role = %q, want %q", vars.Role, "worker")
	}
}

func TestBuildNudgeVars_TaskHintFromEnv(t *testing.T) {
	t.Setenv("BOAT_TASK_ID", "kd-abc123")
	t.Setenv("BOAT_PROMPT", "")
	t.Setenv("BOAT_REFERENCE_REPOS", "")
	vars := buildNudgeVars()
	if !strings.Contains(vars.TaskHint, "kd-abc123") {
		t.Error("TaskHint should include BOAT_TASK_ID")
	}
}
