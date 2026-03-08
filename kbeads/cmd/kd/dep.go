package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/groblegark/kbeads/internal/client"
	"github.com/spf13/cobra"
)

var depCmd = &cobra.Command{
	Use:     "dep",
	Short:   "Manage bead dependencies",
	GroupID: "beads",
}

var depAddCmd = &cobra.Command{
	Use:   "add <bead-id> <depends-on-id>",
	Short: "Add a dependency between beads",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		beadID := args[0]
		dependsOnID := args[1]
		depType, _ := cmd.Flags().GetString("type")

		dep, err := beadsClient.AddDependency(context.Background(), &client.AddDependencyRequest{
			BeadID:      beadID,
			DependsOnID: dependsOnID,
			Type:        depType,
			CreatedBy:   actor,
		})
		if err != nil {
			return fmt.Errorf("adding dependency: %w", err)
		}

		if jsonOutput {
			data, err := json.MarshalIndent(dep, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling JSON: %w", err)
			}
			fmt.Println(string(data))
		} else {
			fmt.Printf("Bead:        %s\n", dep.BeadID)
			fmt.Printf("Depends On:  %s\n", dep.DependsOnID)
			fmt.Printf("Type:        %s\n", dep.Type)
			fmt.Printf("Created By:  %s\n", dep.CreatedBy)
			if !dep.CreatedAt.IsZero() {
				fmt.Printf("Created At:  %s\n", dep.CreatedAt.Format("2006-01-02 15:04:05"))
			}
		}
		return nil
	},
}

var depRemoveCmd = &cobra.Command{
	Use:   "remove <bead-id> <depends-on-id>",
	Short: "Remove a dependency between beads",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		beadID := args[0]
		dependsOnID := args[1]
		depType, _ := cmd.Flags().GetString("type")

		if err := beadsClient.RemoveDependency(context.Background(), beadID, dependsOnID, depType); err != nil {
			return fmt.Errorf("removing dependency: %w", err)
		}

		fmt.Println("Removed dependency")
		return nil
	},
}

var depListCmd = &cobra.Command{
	Use:   "list <bead-id>",
	Short: "List dependencies of a bead",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		beadID := args[0]

		deps, err := beadsClient.GetDependencies(context.Background(), beadID)
		if err != nil {
			return fmt.Errorf("listing dependencies: %w", err)
		}

		if jsonOutput {
			data, err := json.MarshalIndent(deps, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling JSON: %w", err)
			}
			fmt.Println(string(data))
		} else {
			if len(deps) == 0 {
				fmt.Println("No dependencies found.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "DEPENDS_ON\tTYPE\tCREATED_BY\tCREATED_AT")
			for _, d := range deps {
				createdAt := ""
				if !d.CreatedAt.IsZero() {
					createdAt = d.CreatedAt.Format("2006-01-02 15:04:05")
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					d.DependsOnID,
					d.Type,
					d.CreatedBy,
					createdAt,
				)
			}
			w.Flush()
		}
		return nil
	},
}

func init() {
	depAddCmd.Flags().StringP("type", "t", "blocks", "dependency type")
	depRemoveCmd.Flags().StringP("type", "t", "blocks", "dependency type")

	depCmd.AddCommand(depAddCmd)
	depCmd.AddCommand(depRemoveCmd)
	depCmd.AddCommand(depListCmd)
}
