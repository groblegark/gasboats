package main

import (
	"context"
	"fmt"
	"os"

	"github.com/groblegark/kbeads/internal/client"
	"github.com/spf13/cobra"
)

var (
	listProjectFlag     string
	listAllProjectsFlag bool
)

var listCmd = &cobra.Command{
	Use:     "list",
	Short:   "List beads",
	Long:    "List beads. By default only issue-kind beads are shown (task, feature, bug, epic, chore). Use --all-types to include infrastructure beads, or --type/--kind to filter explicitly.",
	GroupID: "beads",
	RunE: func(cmd *cobra.Command, args []string) error {
		status, _ := cmd.Flags().GetStringSlice("status")
		beadType, _ := cmd.Flags().GetStringSlice("type")
		kind, _ := cmd.Flags().GetStringSlice("kind")
		limit, _ := cmd.Flags().GetInt("limit")
		assignee, _ := cmd.Flags().GetString("assignee")
		offset, _ := cmd.Flags().GetInt("offset")
		fieldFlags, _ := cmd.Flags().GetStringArray("field")
		noBlockers, _ := cmd.Flags().GetBool("no-blockers")
		sort, _ := cmd.Flags().GetString("sort")
		allTypes, _ := cmd.Flags().GetBool("all-types")
		labelFlags, _ := cmd.Flags().GetStringSlice("label")

		// Default to kind=issue when no explicit type, kind, or all-types flag is set.
		if !allTypes && len(beadType) == 0 && len(kind) == 0 {
			kind = []string{"issue"}
		}

		req := &client.ListBeadsRequest{
			Status:     status,
			Type:       beadType,
			Kind:       kind,
			Limit:      limit,
			Assignee:   assignee,
			Offset:     offset,
			NoOpenDeps: noBlockers,
			Sort:       sort,
		}

		if !listAllProjectsFlag && listProjectFlag != "" {
			req.Labels = append(req.Labels, "project:"+listProjectFlag)
		}
		req.Labels = append(req.Labels, labelFlags...)

		if len(fieldFlags) > 0 {
			req.FieldFilters = make(map[string]string, len(fieldFlags))
			for _, f := range fieldFlags {
				k, v, ok := splitField(f)
				if !ok {
					fmt.Fprintf(os.Stderr, "Error: invalid field filter %q (expected key=value)\n", f)
					os.Exit(1)
				}
				req.FieldFilters[k] = v
			}
		}

		resp, err := beadsClient.ListBeads(context.Background(), req)
		if err != nil {
			return fmt.Errorf("listing beads: %w", err)
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
	listCmd.Flags().StringSliceP("status", "s", nil, "filter by status (repeatable)")
	listCmd.Flags().StringSliceP("type", "t", nil, "filter by type (repeatable)")
	listCmd.Flags().StringSliceP("kind", "k", nil, "filter by kind (repeatable; default: issue)")
	listCmd.Flags().Int("limit", 20, "maximum number of beads to return")
	listCmd.Flags().String("assignee", "", "filter by assignee")
	listCmd.Flags().Int("offset", 0, "offset for pagination")
	listCmd.Flags().StringArrayP("field", "f", nil, "filter by custom field (key=value, repeatable)")
	listCmd.Flags().StringSliceP("label", "l", nil, "filter by label (repeatable, e.g. --label source:jira --label jira-has-media)")
	listCmd.Flags().Bool("no-blockers", false, "only show beads with no open/in_progress/deferred dependencies")
	listCmd.Flags().String("sort", "", "sort column: priority, created_at, updated_at, title, status, type (prefix with - for descending, e.g. -priority)")
	listCmd.Flags().StringVar(&listProjectFlag, "project", defaultProject(), "filter by project label (default: $KD_PROJECT or $BOAT_PROJECT)")
	listCmd.Flags().BoolVar(&listAllProjectsFlag, "all-projects", false, "show beads from all projects (disables project filter)")
	listCmd.Flags().Bool("all-types", false, "show all bead kinds including infrastructure (default: only issue-kind beads)")
}
