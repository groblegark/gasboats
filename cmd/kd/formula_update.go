package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/groblegark/kbeads/internal/client"
	"github.com/spf13/cobra"
)

// runFormulaUpdate replaces a formula's fields from a JSON file.
func runFormulaUpdate(id, filePath string) error {
	if filePath == "" {
		return fmt.Errorf("--file is required: provide a JSON file path or - for stdin")
	}

	// Validate the bead is a formula.
	ctx := context.Background()
	bead, err := beadsClient.GetBead(ctx, id)
	if err != nil {
		return fmt.Errorf("getting formula: %w", err)
	}
	if string(bead.Type) != "formula" && string(bead.Type) != "template" {
		return fmt.Errorf("bead %s is type %q, not formula", id, bead.Type)
	}

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

	// Build fields and update.
	fields := map[string]any{
		"vars":  content.Vars,
		"steps": content.Steps,
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
		fmt.Printf("{\"id\":%q,\"steps\":%d,\"vars\":%d}\n", id, len(content.Steps), len(content.Vars))
	} else {
		fmt.Printf("Updated formula %s: %d steps, %d vars\n", id, len(content.Steps), len(content.Vars))
	}
	return nil
}

var formulaUpdateCmd = &cobra.Command{
	Use:   "update <formula-id>",
	Short: "Replace formula definition from a JSON file",
	Long: `Replace the vars and steps of an existing formula from a JSON file.

The file format is the same as 'kd formula create --file':

  {
    "vars": [...],
    "steps": [...]
  }

Round-trip workflow:

  kd formula dump kd-abc123 > formula.json
  $EDITOR formula.json                       # add/remove/edit steps
  kd formula update kd-abc123 --file formula.json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filePath, _ := cmd.Flags().GetString("file")
		return runFormulaUpdate(args[0], filePath)
	},
}

func init() {
	formulaUpdateCmd.Flags().StringP("file", "f", "", "JSON file with formula definition (use - for stdin)")
}
