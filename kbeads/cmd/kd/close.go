package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var closeCmd = &cobra.Command{
	Use:     "close <id>...",
	Short:   "Close one or more beads",
	GroupID: "workflow",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		for _, id := range args {
			bead, err := beadsClient.CloseBead(context.Background(), id, actor)
			if err != nil {
				return fmt.Errorf("closing %s: %w", id, err)
			}

			if jsonOutput {
				printBeadJSON(bead)
			} else {
				if len(args) > 1 {
					fmt.Printf("Closed %s\n", bead.ID)
				} else {
					printBeadTable(bead)
				}
			}
		}
		return nil
	},
}
