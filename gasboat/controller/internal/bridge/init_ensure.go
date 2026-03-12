package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"gasboat/controller/internal/beadsapi"
)

// EnsureConfigs upserts all gasboat-managed type, view, context, and config
// entries as config beads in the beads daemon. It is safe to call on every
// startup; existing beads with matching (title, labels) are updated in place.
func EnsureConfigs(ctx context.Context, setter ConfigSetter, logger *slog.Logger) error {
	for key, value := range configs() {
		valueJSON, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("marshalling config %s: %w", key, err)
		}

		if err := ensureConfigBead(ctx, setter, key, valueJSON, logger); err != nil {
			logger.Warn("failed to ensure config bead", "key", key, "error", err)
		}

		logger.Info("ensured config bead", "key", key)
	}

	return nil
}

// roleKeyedNamespaces are key prefixes where the suffix represents a role
// (or "global"). Other namespaces (type, view) use the full key as a
// global-scoped title.
var roleKeyedNamespaces = map[string]bool{
	"context": true,
}

// parseEntryKey converts a config entry key into (title, labels) for
// config bead storage.
//
// Key patterns:
//
//	"config:nudge-prompts:global"            → ("nudge-prompts", ["global"])
//	"config:claude-instructions:role:thread" → ("claude-instructions", ["role:thread"])
//	"context:captain"                        → ("context", ["role:captain"])
//	"type:agent"                             → ("type:agent", ["global"])
//	"view:agents:active"                     → ("view:agents:active", ["global"])
func parseEntryKey(key string) (title string, labels []string) {
	// config:* keys: strip prefix, parse category:scope.
	if strings.HasPrefix(key, "config:") {
		rest := strings.TrimPrefix(key, "config:")
		idx := strings.Index(rest, ":")
		if idx < 0 {
			return rest, []string{"global"}
		}
		return rest[:idx], []string{rest[idx+1:]}
	}

	// Check for role-keyed namespaces (e.g. context:captain).
	if idx := strings.Index(key, ":"); idx > 0 {
		namespace := key[:idx]
		if roleKeyedNamespaces[namespace] {
			suffix := key[idx+1:]
			if suffix == "global" {
				return namespace, []string{"global"}
			}
			return namespace, []string{"role:" + suffix}
		}
	}

	// Everything else (type:*, view:*): full key is title, always global.
	return key, []string{"global"}
}

// ensureConfigBead creates or updates a config bead for the given config key.
func ensureConfigBead(ctx context.Context, setter ConfigSetter, key string, valueJSON []byte, logger *slog.Logger) error {
	category, labels := parseEntryKey(key)

	// Search for an existing open config bead with matching title.
	result, err := setter.ListBeadsFiltered(ctx, beadsapi.ListBeadsQuery{
		Types:    []string{"config"},
		Statuses: []string{"open"},
		Limit:    500,
	})
	if err != nil {
		return fmt.Errorf("listing config beads: %w", err)
	}

	// Find existing bead with same title and labels.
	description := string(valueJSON)
	for _, bead := range result.Beads {
		if bead.Title == category && labelsMatch(bead.Labels, labels) {
			// Update existing bead's description.
			if bead.Description != description {
				if err := setter.UpdateBeadDescription(ctx, bead.ID, description); err != nil {
					return fmt.Errorf("updating config bead %s: %w", bead.ID, err)
				}
				logger.Info("updated config bead", "key", key, "bead_id", bead.ID)
			}
			return nil
		}
	}

	// Create new config bead.
	id, err := setter.CreateBead(ctx, beadsapi.CreateBeadRequest{
		Title:       category,
		Type:        "config",
		Kind:        "config",
		Description: description,
		Labels:      labels,
		CreatedBy:   "gasboat",
	})
	if err != nil {
		return fmt.Errorf("creating config bead for %s: %w", key, err)
	}
	logger.Info("created config bead", "key", key, "bead_id", id)
	return nil
}

// labelsMatch checks if two label slices contain the same elements.
func labelsMatch(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	set := make(map[string]bool, len(a))
	for _, l := range a {
		set[l] = true
	}
	for _, l := range b {
		if !set[l] {
			return false
		}
	}
	return true
}
