package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/groblegark/kbeads/internal/client"
	"github.com/spf13/cobra"
)

// runFormulaUpdate replaces a formula's fields from a JSON file and/or updates
// the assigned agent.
func runFormulaUpdate(id, filePath, assignee string, clearAssignee bool) error {
	if filePath == "" && assignee == "" && !clearAssignee {
		return fmt.Errorf("--file or --assignee is required")
	}

	// Validate the bead is a formula.
	ctx := context.Background()
	bead, err := beadsClient.GetBead(ctx, id)
	if err != nil {
		return fmt.Errorf("getting formula: %w", err)
	}
	if string(bead.Type) != "formula" {
		return fmt.Errorf("bead %s is type %q, not formula", id, bead.Type)
	}

	// Start from existing fields when no file is provided.
	var fields map[string]any

	if filePath != "" {
		// Read input.
		var data []byte
		if filePath == "-" {
			data, err = os.ReadFile("/dev/stdin")
		} else {
			data, err = os.ReadFile(filePath)
		}
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		// Parse and validate.
		var content struct {
			Vars  []FormulaVarDef `json:"vars"`
			Steps []FormulaStep   `json:"steps"`
		}
		if err := json.Unmarshal(data, &content); err != nil {
			return fmt.Errorf("parsing formula JSON: %w", err)
		}

		if len(content.Steps) == 0 {
			return fmt.Errorf("formula must have at least one step")
		}

		stepIDs := make(map[string]bool, len(content.Steps))
		for _, s := range content.Steps {
			if s.ID == "" {
				return fmt.Errorf("every step must have an id")
			}
			if s.Title == "" {
				return fmt.Errorf("step %q must have a title", s.ID)
			}
			if stepIDs[s.ID] {
				return fmt.Errorf("duplicate step id %q", s.ID)
			}
			stepIDs[s.ID] = true
		}
		for _, s := range content.Steps {
			for _, dep := range s.DependsOn {
				if !stepIDs[dep] {
					return fmt.Errorf("step %q depends_on unknown step %q", s.ID, dep)
				}
			}
		}

		fields = map[string]any{
			"vars":  content.Vars,
			"steps": content.Steps,
		}
	} else {
		// Preserve existing fields.
		fields = make(map[string]any)
		if len(bead.Fields) > 0 {
			if err := json.Unmarshal(bead.Fields, &fields); err != nil {
				return fmt.Errorf("parsing existing fields: %w", err)
			}
		}
	}

	// Update assigned_agent.
	if clearAssignee {
		delete(fields, "assigned_agent")
	} else if assignee != "" {
		fields["assigned_agent"] = assignee
	}

	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("encoding fields: %w", err)
	}

	req := &client.UpdateBeadRequest{
		Fields: fieldsJSON,
	}

	_, err = beadsClient.UpdateBead(ctx, id, req)
	if err != nil {
		return fmt.Errorf("updating formula: %w", err)
	}

	if jsonOutput {
		fmt.Printf("{\"id\":%q}\n", id)
	} else {
		fmt.Printf("Updated formula %s\n", id)
	}
	return nil
}

var formulaUpdateCmd = &cobra.Command{
	Use:   "update <formula-id>",
	Short: "Update formula definition or assignment",
	Long: `Update an existing formula's definition and/or agent assignment.

Use --file to replace the vars and steps. Use --assignee to set the
default agent assignment for molecules created from this formula.

Examples:

  kd formula update kd-abc123 --file formula.json
  kd formula update kd-abc123 --assignee my-agent
  kd formula update kd-abc123 --assignee ""`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		assignee, _ := cmd.Flags().GetString("assignee")
		clearAssignee := cmd.Flags().Changed("assignee") && assignee == ""
		return runFormulaUpdate(args[0], filePath, assignee, clearAssignee)
	},
}

func init() {
	formulaUpdateCmd.Flags().StringP("file", "f", "", "JSON file with formula definition (use - for stdin)")
	formulaUpdateCmd.Flags().String("assignee", "", "agent to assign molecules to when this formula is poured (empty string to clear)")
}
