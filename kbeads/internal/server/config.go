package server

import (
	"context"
	"encoding/json"

	"github.com/groblegark/kbeads/internal/model"
)

// builtinConfigs provides the built-in type definitions for all known bead types.
var builtinConfigs = map[string]json.RawMessage{
	"type:epic":    json.RawMessage(`{"kind":"issue","fields":[]}`),
	"type:task":    json.RawMessage(`{"kind":"issue","fields":[]}`),
	"type:feature": json.RawMessage(`{"kind":"issue","fields":[]}`),
	"type:chore":   json.RawMessage(`{"kind":"issue","fields":[]}`),
	"type:bug":     json.RawMessage(`{"kind":"issue","fields":[]}`),
	"type:advice": json.RawMessage(`{
		"kind": "data",
		"fields": [
			{"name": "hook_command",   "type": "string"},
			{"name": "hook_trigger",   "type": "enum", "values": ["session-end", "before-commit", "before-push", "before-handoff"]},
			{"name": "hook_timeout",   "type": "integer"},
			{"name": "hook_on_failure","type": "enum", "values": ["block", "warn", "ignore"]},
			{"name": "subscriptions",  "type": "string[]"},
			{"name": "subscriptions_exclude", "type": "string[]"}
		]
	}`),
	"type:jack": json.RawMessage(`{
		"kind": "data",
		"fields": [
			{"name": "jack_target",          "type": "string",  "required": true},
			{"name": "jack_reason",          "type": "string",  "required": true},
			{"name": "jack_revert_plan",     "type": "string",  "required": true},
			{"name": "jack_ttl",             "type": "string"},
			{"name": "jack_expires_at",      "type": "string"},
			{"name": "jack_original_ttl",    "type": "string"},
			{"name": "jack_extension_count", "type": "integer"},
			{"name": "jack_cumulative_ttl",  "type": "string"},
			{"name": "jack_reverted",        "type": "boolean"},
			{"name": "jack_closed_reason",   "type": "string"},
			{"name": "jack_closed_at",       "type": "string"},
			{"name": "jack_escalated",       "type": "boolean"},
			{"name": "jack_escalated_at",    "type": "string"},
			{"name": "jack_changes",         "type": "json"},
			{"name": "jack_rig",             "type": "string"}
		]
	}`),
	"type:mail": json.RawMessage(`{"kind":"data","fields":[]}`),
	"type:agent": json.RawMessage(`{
		"kind": "config",
		"fields": [
			{"name": "agent",         "type": "string", "required": true},
			{"name": "role",          "type": "string", "required": true},
			{"name": "project",       "type": "string", "required": true},
			{"name": "mode",          "type": "string"},
			{"name": "agent_state",   "type": "string"},
			{"name": "mock_scenario", "type": "string"},
			{"name": "stop_requested",    "type": "string"},
			{"name": "gate_satisfied_by", "type": "string"},
			{"name": "advice_subscriptions",         "type": "string[]"},
			{"name": "advice_subscriptions_exclude",  "type": "string[]"}
		]
	}`),
	"type:decision": json.RawMessage(`{
		"kind": "data",
		"fields": [
			{"name": "prompt",                  "type": "string", "required": true},
			{"name": "options",                 "type": "json"},
			{"name": "context",                 "type": "string"},
			{"name": "session",                 "type": "string"},
			{"name": "requested_by",            "type": "string"},
			{"name": "requesting_agent_bead_id","type": "string"},
			{"name": "chosen",                  "type": "string"},
			{"name": "rationale",               "type": "string"},
			{"name": "response_text",           "type": "string"},
			{"name": "responded_by",            "type": "string"},
			{"name": "responded_at",            "type": "string"},
			{"name": "required_artifact",       "type": "string"},
			{"name": "artifact_status",         "type": "enum", "values": ["pending", "submitted", "accepted"]}
		]
	}`),
	"type:report": json.RawMessage(`{
		"kind": "data",
		"fields": [
			{"name": "decision_id", "type": "string", "required": true},
			{"name": "report_type", "type": "string", "required": true},
			{"name": "content",     "type": "string", "required": true},
			{"name": "format",      "type": "string"}
		]
	}`),

	"type:project": json.RawMessage(`{
		"kind": "config",
		"fields": [
			{"name": "prefix",          "type": "string"},
			{"name": "git_url",         "type": "string"},
			{"name": "default_branch",  "type": "string"},
			{"name": "image",           "type": "string"},
			{"name": "storage_class",   "type": "string"},
			{"name": "service_account", "type": "string"},
			{"name": "rtk_enabled",     "type": "boolean"},
			{"name": "docker",          "type": "boolean"},
			{"name": "cpu_request",     "type": "string"},
			{"name": "cpu_limit",       "type": "string"},
			{"name": "memory_request",  "type": "string"},
			{"name": "memory_limit",    "type": "string"},
			{"name": "secrets",         "type": "json"},
			{"name": "env",             "type": "json"},
			{"name": "env_json",        "type": "json"},
			{"name": "repos",           "type": "json"},
			{"name": "slack_channel",   "type": "string"},
			{"name": "channel_roles",   "type": "json"},
			{"name": "auto_assign",     "type": "string"},
			{"name": "prewarmed_pool",  "type": "json"},
			{"name": "nudge_prompts",   "type": "json"}
		]
	}`),

	// Infrastructure types — config kind.
	"type:role":    json.RawMessage(`{"kind":"config","fields":[]}`),
	"type:rig":     json.RawMessage(`{"kind":"config","fields":[]}`),
	"type:convoy":  json.RawMessage(`{"kind":"config","fields":[]}`),
	"type:config":  json.RawMessage(`{"kind":"config","fields":[]}`),

	// Infrastructure types — data kind.
	"type:event":    json.RawMessage(`{"kind":"data","fields":[]}`),
	"type:gate":     json.RawMessage(`{"kind":"data","fields":[]}`),
	"type:message":  json.RawMessage(`{"kind":"data","fields":[]}`),
	"type:formula": json.RawMessage(`{
		"kind": "data",
		"fields": [
			{"name": "vars",  "type": "json"},
			{"name": "steps", "type": "json"},
			{"name": "default_roles", "type": "json"},
			{"name": "assigned_agent", "type": "string"}
		]
	}`),
	"type:molecule": json.RawMessage(`{
		"kind": "issue",
		"fields": [
			{"name": "formula_id",   "type": "string"},
			{"name": "applied_vars", "type": "json"},
			{"name": "ephemeral",    "type": "boolean"}
		]
	}`),
	"type:mention":  json.RawMessage(`{"kind":"data","fields":[]}`),
	"type:artifact": json.RawMessage(`{"kind":"data","fields":[]}`),
	"type:runbook":  json.RawMessage(`{"kind":"data","fields":[]}`),
}

// resolveTypeConfig looks up the type config for a bead type from the
// builtin type definitions. Returns nil, nil if not found.
func (s *BeadsServer) resolveTypeConfig(_ context.Context, beadType model.BeadType) (*model.TypeConfig, error) {
	key := "type:" + string(beadType)

	raw, ok := builtinConfigs[key]
	if !ok {
		return nil, nil
	}

	var tc model.TypeConfig
	if err := json.Unmarshal(raw, &tc); err != nil {
		return nil, err
	}
	return &tc, nil
}
