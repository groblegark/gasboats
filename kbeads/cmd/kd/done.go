package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var doneCmd = &cobra.Command{
	Use:     "done <id>...",
	Short:   "Mark beads as done and close them",
	GroupID: "workflow",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		comment, _ := cmd.Flags().GetString("comment")

		for _, id := range args {
			if comment != "" {
				_, err := beadsClient.AddComment(context.Background(), id, actor, comment)
				if err != nil {
					return fmt.Errorf("adding comment to %s: %w", id, err)
				}
			}

			bead, err := beadsClient.CloseBead(context.Background(), id, actor)
			if err != nil {
				return fmt.Errorf("closing %s: %w", id, err)
			}

			if jsonOutput {
				printBeadJSON(bead)
			} else {
				if len(args) > 1 {
					fmt.Printf("Done %s\n", bead.ID)
				} else {
					printBeadTable(bead)
				}
			}
		}
		return nil
	},
}

func init() {
	doneCmd.Flags().StringP("comment", "m", "", "completion comment to add before closing")
}
