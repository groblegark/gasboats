package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/groblegark/kbeads/internal/client"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:     "search <query>",
	Short:   "Search beads by text query",
	GroupID: "beads",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		status, _ := cmd.Flags().GetStringSlice("status")
		beadType, _ := cmd.Flags().GetStringSlice("type")
		kind, _ := cmd.Flags().GetStringSlice("kind")
		limit, _ := cmd.Flags().GetInt("limit")

		req := &client.ListBeadsRequest{
			Search: query,
			Status: status,
			Type:   beadType,
			Kind:   kind,
			Limit:  limit,
		}

		resp, err := beadsClient.ListBeads(context.Background(), req)
		if err != nil {
			return fmt.Errorf("searching beads: %w", err)
		}

		if jsonOutput {
			printBeadListJSON(resp.Beads)
		} else {
			printBeadListTable(resp.Beads, resp.Total)
		}
		return nil
	},
}

func init() {
	searchCmd.Flags().StringSliceP("status", "s", nil, "filter by status (repeatable)")
	searchCmd.Flags().StringSliceP("type", "t", nil, "filter by type (repeatable)")
	searchCmd.Flags().StringSliceP("kind", "k", nil, "filter by kind (repeatable)")
	searchCmd.Flags().Int("limit", 20, "maximum number of results")
}
