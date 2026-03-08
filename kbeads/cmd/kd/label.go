package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var labelCmd = &cobra.Command{
	Use:     "label",
	Short:   "Manage bead labels",
	GroupID: "beads",
}

var labelAddCmd = &cobra.Command{
	Use:   "add <bead-id> <label>...",
	Short: "Add labels to a bead",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		beadID := args[0]
		labels := args[1:]

		for _, label := range labels {
			_, err := beadsClient.AddLabel(context.Background(), beadID, label)
			if err != nil {
				return fmt.Errorf("adding label %q: %w", label, err)
			}
		}

		fmt.Printf("Added label(s) to %s\n", beadID)
		return nil
	},
}

var labelRemoveCmd = &cobra.Command{
	Use:   "remove <bead-id> <label>...",
	Short: "Remove labels from a bead",
	Args:  cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		beadID := args[0]
		labels := args[1:]

		for _, label := range labels {
			if err := beadsClient.RemoveLabel(context.Background(), beadID, label); err != nil {
				return fmt.Errorf("removing label %q: %w", label, err)
			}
		}

		fmt.Printf("Removed label(s) from %s\n", beadID)
		return nil
	},
}

func init() {
	labelCmd.AddCommand(labelAddCmd)
	labelCmd.AddCommand(labelRemoveCmd)
}
