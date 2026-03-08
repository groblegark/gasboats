package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/groblegark/kbeads/internal/model"
	"github.com/spf13/cobra"
)

var childrenCmd = &cobra.Command{
	Use:     "children <bead-id>",
	Short:   "List beads that depend on this bead (reverse dependencies)",
	GroupID: "views",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		beadID := args[0]
		filterType, _ := cmd.Flags().GetString("type")

		ctx := context.Background()
		deps, err := beadsClient.GetReverseDependencies(ctx, beadID)
		if err != nil {
			return fmt.Errorf("fetching reverse dependencies: %w", err)
		}

		var types []string
		if filterType != "" {
			types = []string{filterType}
		}

		// For reverse deps, resolve the BeadID (the bead that depends on us).
		typeSet := make(map[string]bool, len(types))
		for _, t := range types {
			typeSet[t] = true
		}

		type resolvedChild struct {
			Dep  *model.Dependency
			Bead *model.Bead
		}
		var resolved []resolvedChild
		for _, d := range deps {
			if len(typeSet) > 0 && !typeSet[string(d.Type)] {
				continue
			}
			rc := resolvedChild{Dep: d}
			fetchedBead, err := beadsClient.GetBead(ctx, d.BeadID)
			if err == nil {
				rc.Bead = fetchedBead
			}
			resolved = append(resolved, rc)
		}

		if len(resolved) == 0 {
			fmt.Println("No dependents found.")
			return nil
		}

		if jsonOutput {
			type jsonChild struct {
				BeadID string `json:"bead_id"`
				Type   string `json:"type"`
				Status string `json:"status,omitempty"`
				Title  string `json:"title,omitempty"`
			}
			var out []jsonChild
			for _, rd := range resolved {
				jc := jsonChild{
					BeadID: rd.Dep.BeadID,
					Type:   string(rd.Dep.Type),
				}
				if rd.Bead != nil {
					jc.Status = string(rd.Bead.Status)
					jc.Title = rd.Bead.Title
				}
				out = append(out, jc)
			}
			data, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling JSON: %w", err)
			}
			fmt.Println(string(data))
		} else {
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "BEAD_ID\tTYPE\tSTATUS\tTITLE")
			for _, rd := range resolved {
				status := "(unknown)"
				title := "(error fetching)"
				if rd.Bead != nil {
					status = string(rd.Bead.Status)
					title = rd.Bead.Title
					if len(title) > 50 {
						title = title[:47] + "..."
					}
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
					rd.Dep.BeadID,
					string(rd.Dep.Type),
					status,
					title,
				)
			}
			w.Flush()
		}
		return nil
	},
}

func init() {
	childrenCmd.Flags().StringP("type", "t", "", "filter by dependency type")
}
