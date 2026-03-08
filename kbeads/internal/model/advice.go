package model

import (
	"fmt"
	"strings"
)

// MatchesSubscriptions checks if an advice bead should be delivered to an agent
// based on the agent's subscription labels.
//
// Matching rules:
//   - If advice has rig:X label, agent MUST be subscribed to rig:X (required match)
//   - If advice has agent:X label, agent MUST be subscribed to agent:X (required match)
//   - For other labels: AND within groups (gN: prefix), OR across groups
//
// This prevents rig-scoped advice from leaking to other rigs via role matches.
func MatchesSubscriptions(adviceLabels, subscriptions []string) bool {
	subSet := make(map[string]bool, len(subscriptions))
	for _, s := range subscriptions {
		subSet[s] = true
	}

	// Check required labels: rig:X and agent:X must be in subscriptions.
	for _, l := range adviceLabels {
		clean := StripGroupPrefix(l)
		if strings.HasPrefix(clean, "rig:") && !subSet[clean] {
			return false
		}
		if strings.HasPrefix(clean, "agent:") && !subSet[clean] {
			return false
		}
	}

	// Parse label groups for AND/OR matching.
	groups := ParseGroups(adviceLabels)

	// OR across groups: if any group fully matches, advice applies.
	for _, groupLabels := range groups {
		if len(groupLabels) == 0 {
			continue
		}
		allMatch := true
		for _, label := range groupLabels {
			if !subSet[label] {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	return false
}

// ParseGroups extracts group numbers from label prefixes.
// Labels with gN: prefix are grouped together (AND within group).
// Labels without prefix are treated as separate groups (backward compat - OR behavior).
func ParseGroups(labels []string) map[int][]string {
	groups := make(map[int][]string)
	nextUnprefixed := 1000

	for _, label := range labels {
		if strings.HasPrefix(label, "g") {
			idx := strings.Index(label, ":")
			if idx > 1 {
				var groupNum int
				if _, err := fmt.Sscanf(label[:idx], "g%d", &groupNum); err == nil {
					groups[groupNum] = append(groups[groupNum], label[idx+1:])
					continue
				}
			}
		}
		// No valid gN: prefix — treat as its own group (OR behavior).
		groups[nextUnprefixed] = append(groups[nextUnprefixed], label)
		nextUnprefixed++
	}
	return groups
}

// StripGroupPrefix removes the gN: prefix from a label if present.
// "g0:role:polecat" → "role:polecat", "global" → "global".
func StripGroupPrefix(label string) string {
	if len(label) >= 3 && label[0] == 'g' {
		for i := 1; i < len(label); i++ {
			if label[i] == ':' && i > 1 {
				return label[i+1:]
			}
			if label[i] < '0' || label[i] > '9' {
				break
			}
		}
	}
	return label
}

// BuildAgentSubscriptions creates auto-subscription labels for an agent.
// It always includes "global" and "agent:<agentID>", plus rig/role labels
// parsed from the agent ID (format: rig/role_plural/name).
func BuildAgentSubscriptions(agentID string, extra []string) []string {
	subs := make([]string, 0, len(extra)+4)
	subs = append(subs, extra...)
	subs = append(subs, "global")
	subs = append(subs, "agent:"+agentID)

	parts := strings.Split(agentID, "/")
	if len(parts) >= 1 && parts[0] != "" {
		subs = append(subs, "rig:"+parts[0])
	}
	if len(parts) >= 2 {
		rolePlural := parts[1]
		subs = append(subs, "role:"+rolePlural)
		if roleSingular := singularize(rolePlural); roleSingular != rolePlural {
			subs = append(subs, "role:"+roleSingular)
		}
	}
	return subs
}

// singularize converts a plural role name to singular by stripping a trailing "s".
func singularize(plural string) string {
	if strings.HasSuffix(plural, "s") {
		return strings.TrimSuffix(plural, "s")
	}
	return plural
}
