package main

import (
	"context"
	"fmt"

	"github.com/groblegark/kbeads/internal/client"
	"github.com/spf13/cobra"
)

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

var updateCmd = &cobra.Command{
	Use:     "update <id>",
	Short:   "Update a bead",
	GroupID: "beads",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		req := &client.UpdateBeadRequest{}

		if cmd.Flags().Changed("title") {
			v, _ := cmd.Flags().GetString("title")
			req.Title = strPtr(v)
		}
		if cmd.Flags().Changed("description") {
			v, _ := cmd.Flags().GetString("description")
			req.Description = strPtr(v)
		}
		if cmd.Flags().Changed("status") {
			v, _ := cmd.Flags().GetString("status")
			req.Status = strPtr(v)
		}
		if cmd.Flags().Changed("priority") {
			v, _ := cmd.Flags().GetInt("priority")
			req.Priority = intPtr(v)
		}
		if cmd.Flags().Changed("assignee") {
			v, _ := cmd.Flags().GetString("assignee")
			req.Assignee = strPtr(v)
		}
		if cmd.Flags().Changed("owner") {
			v, _ := cmd.Flags().GetString("owner")
			req.Owner = strPtr(v)
		}
		if cmd.Flags().Changed("notes") {
			v, _ := cmd.Flags().GetString("notes")
			req.Notes = strPtr(v)
		}
		if cmd.Flags().Changed("field") {
			fieldPairs, _ := cmd.Flags().GetStringArray("field")
			fieldsJSON, err := parseFields(fieldPairs)
			if err != nil {
				return fmt.Errorf("parsing fields: %w", err)
			}
			req.Fields = fieldsJSON
		}

		bead, err := beadsClient.UpdateBead(context.Background(), id, req)
		if err != nil {
			return fmt.Errorf("updating bead %s: %w", id, err)
		}

		if jsonOutput {
			printBeadJSON(bead)
		} else {
			printBeadTable(bead)
		}
		return nil
	},
}

func init() {
	updateCmd.Flags().String("title", "", "bead title")
	updateCmd.Flags().StringP("description", "d", "", "bead description")
	updateCmd.Flags().StringP("status", "s", "", "bead status")
	updateCmd.Flags().IntP("priority", "p", 0, "bead priority")
	updateCmd.Flags().String("assignee", "", "assignee")
	updateCmd.Flags().String("owner", "", "owner")
	updateCmd.Flags().String("notes", "", "notes")
	updateCmd.Flags().StringArrayP("field", "f", nil, "typed field (key=value, repeatable)")
}
