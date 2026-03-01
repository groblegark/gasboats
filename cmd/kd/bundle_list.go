package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/groblegark/kbeads/internal/client"
	"github.com/groblegark/kbeads/internal/model"
	"github.com/spf13/cobra"
)

var bundleListCmd = &cobra.Command{
	Use:   "list",
	Short: "List bundles",
	Long: `List bundles (instantiated templates).

Examples:
  kd bundle list
  kd bundle list --label project:gasboat
  kd bundle list --status closed`,
	RunE: func(cmd *cobra.Command, args []string) error {
		labels, _ := cmd.Flags().GetStringSlice("label")
		statuses, _ := cmd.Flags().GetStringSlice("status")

		if len(statuses) == 0 {
			statuses = []string{"open", "in_progress"}
		}

		req := &client.ListBeadsRequest{
			Type:   []string{"bundle"},
			Status: statuses,
			Labels: labels,
			Limit:  100,
		}

		resp, err := beadsClient.ListBeads(context.Background(), req)
		if err != nil {
			return fmt.Errorf("listing bundles: %w", err)
		}

		if jsonOutput {
			printBeadListJSON(resp.Beads)
		} else {
			printBundleList(resp.Beads, resp.Total)
		}
		return nil
	},
}

func printBundleList(beads []*model.Bead, total int) {
	if len(beads) == 0 {
		fmt.Println("No bundles found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tTITLE\tTEMPLATE\tASSIGNEE")
	for _, b := range beads {
		title := b.Title
		if len(title) > 40 {
			title = title[:37] + "..."
		}

		templateID := bundleTemplateID(b)

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			b.ID, b.Status, title, templateID, b.Assignee)
	}
	w.Flush()
	fmt.Printf("\n%d bundles (%d total)\n", len(beads), total)
}

// bundleTemplateID extracts the template_id from a bundle bead's fields.
func bundleTemplateID(b *model.Bead) string {
	if len(b.Fields) == 0 {
		return ""
	}
	var f struct {
		TemplateID string `json:"template_id"`
	}
	if json.Unmarshal(b.Fields, &f) == nil {
		return f.TemplateID
	}
	return ""
}

func init() {
	bundleListCmd.Flags().StringSliceP("label", "l", nil, "filter by label (repeatable)")
	bundleListCmd.Flags().StringSliceP("status", "s", nil, "filter by status (default: open, in_progress)")
}
