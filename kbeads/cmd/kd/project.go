package main

import (
	"os"
	"strings"
)

// defaultProject returns KD_PROJECT > BOAT_PROJECT > "".
func defaultProject() string {
	if p := os.Getenv("KD_PROJECT"); p != "" {
		return p
	}
	return os.Getenv("BOAT_PROJECT")
}

// agentProject returns the agent's project name from KD_PROJECT, BOAT_PROJECT,
// or by parsing the first component of BEADS_AGENT_NAME (e.g., "gasboat/gb-zeta" → "gasboat").
func agentProject() string {
	if p := defaultProject(); p != "" {
		return p
	}
	if name := os.Getenv("BEADS_AGENT_NAME"); name != "" {
		parts := strings.SplitN(name, "/", 2)
		if len(parts) == 2 && parts[0] != "" {
			return parts[0]
		}
	}
	return ""
}
