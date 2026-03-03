package main

import (
	"context"
	"encoding/json"
	"fmt"
	"gasboat/controller/internal/beadsapi"
	"os"
	"sort"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:     "config",
	Short:   "Config bead management",
	GroupID: "session",
}

// configNamespaces are the known config bead namespaces to dump.
// The daemon API requires a namespace parameter, so we enumerate all known ones.
var configNamespaces = []string{
	"claude-settings",
	"claude-hooks",
	"claude-mcp",
	"type",
	"context",
	"view",
}

// configEntry is the dump/load serialization format.
type configEntry struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

// configDumper is the interface needed by dumpConfigs (subset of beadsapi.Client).
type configDumper interface {
	ListConfigs(ctx context.Context, namespace string) ([]beadsapi.ConfigEntry, error)
}

// configLoader is the interface needed by loadConfigs (subset of beadsapi.Client).
type configLoader interface {
	SetConfig(ctx context.Context, key string, value []byte) error
}

// dumpConfigs fetches all config entries across known namespaces.
func dumpConfigs(ctx context.Context, client configDumper, namespaces []string) ([]configEntry, error) {
	var entries []configEntry
	for _, ns := range namespaces {
		configs, err := client.ListConfigs(ctx, ns)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[config] warning: failed to list %s: %v\n", ns, err)
			continue
		}
		for _, c := range configs {
			entries = append(entries, configEntry{Key: c.Key, Value: c.Value})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Key < entries[j].Key
	})

	return entries, nil
}

// loadConfigs restores config entries to the daemon.
func loadConfigs(ctx context.Context, client configLoader, entries []configEntry) (restored int, errors int) {
	for _, e := range entries {
		if err := client.SetConfig(ctx, e.Key, e.Value); err != nil {
			fmt.Fprintf(os.Stderr, "[config] warning: failed to set %s: %v\n", e.Key, err)
			errors++
			continue
		}
		fmt.Fprintf(os.Stderr, "[config] restored %s\n", e.Key)
		restored++
	}
	return restored, errors
}

var configDumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Dump all config beads as JSON (pipe to file for backup)",
	Long: `Fetches config entries from all known namespaces and prints them
as a JSON array. The output is suitable for saving to a file and
restoring later with 'gb config load'.

Example:
  gb config dump > configs-backup.json
  gb config dump | jq .`,
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := dumpConfigs(cmd.Context(), daemon, configNamespaces)
		if err != nil {
			return err
		}

		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return fmt.Errorf("marshalling: %w", err)
		}

		fmt.Fprintln(os.Stdout, string(data))
		fmt.Fprintf(os.Stderr, "[config] dumped %d entries\n", len(entries))
		return nil
	},
}

var configLoadCmd = &cobra.Command{
	Use:   "load <file>",
	Short: "Restore config beads from a dump file",
	Long: `Reads a JSON file produced by 'gb config dump' and creates or
updates each config entry on the daemon.

Example:
  gb config load configs-backup.json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		data, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		var entries []configEntry
		if err := json.Unmarshal(data, &entries); err != nil {
			return fmt.Errorf("parsing JSON: %w", err)
		}

		restored, _ := loadConfigs(cmd.Context(), daemon, entries)
		fmt.Fprintf(os.Stderr, "[config] loaded %d entries\n", restored)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configDumpCmd)
	configCmd.AddCommand(configLoadCmd)
}
