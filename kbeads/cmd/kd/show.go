package main

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:     "show <id>",
	Short:   "Show details of a bead",
	GroupID: "beads",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		bead, err := beadsClient.GetBead(context.Background(), id)
		if err != nil {
			return fmt.Errorf("getting bead %s: %w", id, err)
		}

		if jsonOutput {
			printBeadJSON(bead)
		} else {
			printBeadTable(bead)
			if len(bead.Dependencies) > 0 {
				fmt.Println()
				fmt.Println("Depends On:")
				for _, d := range bead.Dependencies {
					fmt.Printf("  %s (%s)\n", d.DependsOnID, d.Type)
				}
			}
			if len(bead.Comments) > 0 {
				fmt.Println()
				fmt.Println("Comments:")
				for _, c := range bead.Comments {
					ts := c.CreatedAt.Format("2006-01-02 15:04:05")
					fmt.Printf("  [%s] %s: %s\n", ts, c.Author, c.Text)
				}
			}
		}
		return nil
	},
}
