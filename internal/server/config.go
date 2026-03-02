package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	beadsv1 "github.com/groblegark/kbeads/gen/beads/v1"
	"github.com/groblegark/kbeads/internal/model"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// builtinConfigs provides default config values that are returned when no
// user-defined config exists for a key.  The namespace index groups them by
// prefix so ListConfigs can merge them in.
var builtinConfigs = map[string]*model.Config{
	"view:ready": {
		Key:   "view:ready",
		Value: json.RawMessage(`{"filter":{"status":["open","in_progress"],"kind":["issue"]},"sort":"priority","limit":5}`),
	},
	"type:epic":    {Key: "type:epic", Value: json.RawMessage(`{"kind":"issue","fields":[]}`)},
	"type:task":    {Key: "type:task", Value: json.RawMessage(`{"kind":"issue","fields":[]}`)},
	"type:feature": {Key: "type:feature", Value: json.RawMessage(`{"kind":"issue","fields":[]}`)},
	"type:chore":   {Key: "type:chore", Value: json.RawMessage(`{"kind":"issue","fields":[]}`)},
	"type:bug":     {Key: "type:bug", Value: json.RawMessage(`{"kind":"issue","fields":[]}`)},
	"type:advice": {Key: "type:advice", Value: json.RawMessage(`{
		"kind": "data",
		"fields": [
			{"name": "hook_command",   "type": "string"},
			{"name": "hook_trigger",   "type": "enum", "values": ["session-end", "before-commit", "before-push", "before-handoff"]},
			{"name": "hook_timeout",   "type": "integer"},
			{"name": "hook_on_failure","type": "enum", "values": ["block", "warn", "ignore"]},
			{"name": "subscriptions",  "type": "string[]"},
			{"name": "subscriptions_exclude", "type": "string[]"}
		]
	}`)},
	"type:jack": {Key: "type:jack", Value: json.RawMessage(`{
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
	}`)},
	"type:mail": {Key: "type:mail", Value: json.RawMessage(`{"kind":"data","fields":[]}`)},
	"type:agent": {Key: "type:agent", Value: json.RawMessage(`{
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
	}`)},
	"type:decision": {Key: "type:decision", Value: json.RawMessage(`{
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
	}`)},
	"type:report": {Key: "type:report", Value: json.RawMessage(`{
		"kind": "data",
		"fields": [
			{"name": "decision_id", "type": "string", "required": true},
			{"name": "report_type", "type": "string", "required": true},
			{"name": "content",     "type": "string", "required": true},
			{"name": "format",      "type": "string"}
		]
	}`)},

	// Infrastructure types — config kind.
	"type:role":    {Key: "type:role", Value: json.RawMessage(`{"kind":"config","fields":[]}`)},
	"type:rig":     {Key: "type:rig", Value: json.RawMessage(`{"kind":"config","fields":[]}`)},
	"type:convoy":  {Key: "type:convoy", Value: json.RawMessage(`{"kind":"config","fields":[]}`)},
	"type:config":  {Key: "type:config", Value: json.RawMessage(`{"kind":"config","fields":[]}`)},

	// Infrastructure types — data kind.
	"type:event":    {Key: "type:event", Value: json.RawMessage(`{"kind":"data","fields":[]}`)},
	"type:gate":     {Key: "type:gate", Value: json.RawMessage(`{"kind":"data","fields":[]}`)},
	"type:message":  {Key: "type:message", Value: json.RawMessage(`{"kind":"data","fields":[]}`)},
	"type:formula": {Key: "type:formula", Value: json.RawMessage(`{
		"kind": "data",
		"fields": [
			{"name": "vars",  "type": "json"},
			{"name": "steps", "type": "json"}
		]
	}`)},
	"type:molecule": {Key: "type:molecule", Value: json.RawMessage(`{
		"kind": "issue",
		"fields": [
			{"name": "formula_id",   "type": "string"},
			{"name": "template_id",  "type": "string"},
			{"name": "applied_vars", "type": "json"},
			{"name": "ephemeral",    "type": "boolean"}
		]
	}`)},
	"type:template": {Key: "type:template", Value: json.RawMessage(`{
		"kind": "data",
		"fields": [
			{"name": "vars",  "type": "json"},
			{"name": "steps", "type": "json"}
		]
	}`)},
	"type:bundle": {Key: "type:bundle", Value: json.RawMessage(`{
		"kind": "issue",
		"fields": [
			{"name": "formula_id",   "type": "string"},
			{"name": "template_id",  "type": "string"},
			{"name": "applied_vars", "type": "json"},
			{"name": "ephemeral",    "type": "boolean"}
		]
	}`)},
	"type:mention":  {Key: "type:mention", Value: json.RawMessage(`{"kind":"data","fields":[]}`)},
	"type:artifact": {Key: "type:artifact", Value: json.RawMessage(`{"kind":"data","fields":[]}`)},
	"type:runbook":  {Key: "type:runbook", Value: json.RawMessage(`{"kind":"data","fields":[]}`)},
}

var builtinConfigsByNamespace = func() map[string][]*model.Config {
	m := map[string][]*model.Config{}
	for key, cfg := range builtinConfigs {
		if i := strings.Index(key, ":"); i > 0 {
			ns := key[:i]
			m[ns] = append(m[ns], cfg)
		}
	}
	return m
}()

// resolveTypeConfig looks up the type config for a bead type, first from the
// store, then from builtin defaults. Returns nil, nil if not found.
func (s *BeadsServer) resolveTypeConfig(ctx context.Context, beadType model.BeadType) (*model.TypeConfig, error) {
	key := "type:" + string(beadType)

	// Try user-defined config in the store first.
	config, err := s.store.GetConfig(ctx, key)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if config == nil {
		// Fall back to builtin.
		config = builtinConfigs[key]
	}
	if config == nil {
		return nil, nil
	}

	var tc model.TypeConfig
	if err := json.Unmarshal(config.Value, &tc); err != nil {
		return nil, err
	}
	return &tc, nil
}

// SetConfig creates or updates a config entry.
func (s *BeadsServer) SetConfig(ctx context.Context, req *beadsv1.SetConfigRequest) (*beadsv1.SetConfigResponse, error) {
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}

	config := &model.Config{
		Key:   req.GetKey(),
		Value: json.RawMessage(req.GetValue()),
	}

	if err := s.store.SetConfig(ctx, config); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to set config: %v", err)
	}

	return &beadsv1.SetConfigResponse{Config: configToProto(config)}, nil
}

// GetConfig retrieves a config by key.
func (s *BeadsServer) GetConfig(ctx context.Context, req *beadsv1.GetConfigRequest) (*beadsv1.GetConfigResponse, error) {
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}

	config, err := s.store.GetConfig(ctx, req.GetKey())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if builtin, ok := builtinConfigs[req.GetKey()]; ok {
				return &beadsv1.GetConfigResponse{Config: configToProto(builtin)}, nil
			}
		}
		return nil, storeError(err, "config")
	}

	return &beadsv1.GetConfigResponse{Config: configToProto(config)}, nil
}

// listConfigsWithBuiltins fetches configs from the store and merges in builtin
// defaults that haven't been overridden.
func (s *BeadsServer) listConfigsWithBuiltins(ctx context.Context, namespace string) ([]*model.Config, error) {
	configs, err := s.store.ListConfigs(ctx, namespace)
	if err != nil {
		return nil, err
	}

	stored := make(map[string]struct{}, len(configs))
	for _, c := range configs {
		stored[c.Key] = struct{}{}
	}
	for _, b := range builtinConfigsByNamespace[namespace] {
		if _, ok := stored[b.Key]; !ok {
			configs = append(configs, b)
		}
	}

	return configs, nil
}

// ListConfigs returns configs matching a namespace prefix, merging in any
// builtin defaults that haven't been overridden by user-defined configs.
func (s *BeadsServer) ListConfigs(ctx context.Context, req *beadsv1.ListConfigsRequest) (*beadsv1.ListConfigsResponse, error) {
	if req.GetNamespace() == "" {
		return nil, status.Error(codes.InvalidArgument, "namespace is required")
	}

	configs, err := s.listConfigsWithBuiltins(ctx, req.GetNamespace())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list configs: %v", err)
	}

	pbConfigs := make([]*beadsv1.Config, 0, len(configs))
	for _, c := range configs {
		pbConfigs = append(pbConfigs, configToProto(c))
	}

	return &beadsv1.ListConfigsResponse{Configs: pbConfigs}, nil
}

// DeleteConfig removes a config by key.
func (s *BeadsServer) DeleteConfig(ctx context.Context, req *beadsv1.DeleteConfigRequest) (*beadsv1.DeleteConfigResponse, error) {
	if req.GetKey() == "" {
		return nil, status.Error(codes.InvalidArgument, "key is required")
	}

	if err := s.store.DeleteConfig(ctx, req.GetKey()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "config not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to delete config: %v", err)
	}

	return &beadsv1.DeleteConfigResponse{}, nil
}
