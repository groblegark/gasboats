package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/groblegark/kbeads/internal/model"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:     "config",
	Short:   "Manage configs (type, view, context definitions)",
	GroupID: "system",
}

var configCreateCmd = &cobra.Command{
	Use:     "create <key> <json-value>",
	Aliases: []string{"new"},
	Short:   "Create or update a config",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		value := []byte(args[1])

		// Validate that value is valid JSON.
		if !json.Valid(value) {
			return fmt.Errorf("value must be valid JSON")
		}

		config, err := beadsClient.SetConfig(context.Background(), key, value)
		if err != nil {
			return fmt.Errorf("setting config %q: %w", key, err)
		}

		printConfigJSON(config)
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a config by key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := beadsClient.GetConfig(context.Background(), args[0])
		if err != nil {
			return fmt.Errorf("getting config %q: %w", args[0], err)
		}

		printConfigJSON(config)
		return nil
	},
}

var configListCmd = &cobra.Command{
	Use:   "list [namespace]",
	Short: "List configs by namespace (e.g. view, type, context)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		namespace := ""
		if len(args) > 0 {
			namespace = args[0]
		}
		if namespace == "" {
			return fmt.Errorf("namespace argument is required")
		}

		configs, err := beadsClient.ListConfigs(context.Background(), namespace)
		if err != nil {
			return fmt.Errorf("listing configs in %q: %w", namespace, err)
		}

		for _, c := range configs {
			printConfigJSON(c)
		}
		if len(configs) == 0 {
			fmt.Println("No configs found.")
		}
		return nil
	},
}

var configDeleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a config by key",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := beadsClient.DeleteConfig(context.Background(), args[0]); err != nil {
			return fmt.Errorf("deleting config %q: %w", args[0], err)
		}

		fmt.Printf("Deleted config %q\n", args[0])
		return nil
	},
}

func printConfigJSON(c *model.Config) {
	// Pretty-print by unmarshalling the value bytes so they render as JSON, not base64.
	var valueObj any
	_ = json.Unmarshal(c.Value, &valueObj)

	out := map[string]any{
		"key":   c.Key,
		"value": valueObj,
	}
	if !c.CreatedAt.IsZero() {
		out["created_at"] = c.CreatedAt.Format("2006-01-02T15:04:05Z")
	}
	if !c.UpdatedAt.IsZero() {
		out["updated_at"] = c.UpdatedAt.Format("2006-01-02T15:04:05Z")
	}

	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(data))
}

func init() {
	configCmd.AddCommand(configCreateCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configDeleteCmd)
}
