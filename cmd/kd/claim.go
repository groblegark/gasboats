package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/groblegark/kbeads/internal/client"
	"github.com/spf13/cobra"
)

var claimForce bool

var claimCmd = &cobra.Command{
	Use:     "claim <id>",
	Short:   "Claim a bead by assigning it to yourself",
	GroupID: "workflow",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		ctx := context.Background()

		if !claimForce {
			// Enforce single-claim limit: agent must not already have a claimed bead.
			if err := checkNoClaimed(ctx); err != nil {
				return err
			}

			// Block claiming epics — epics are coordination containers, not actionable work.
			if err := checkNotEpic(ctx, id); err != nil {
				return err
			}

			// Enforce project-scoped claiming: bead must belong to agent's project.
			if err := checkProjectMatch(ctx, id); err != nil {
				return err
			}
		}

		inProgress := "in_progress"
		bead, err := beadsClient.UpdateBead(ctx, id, &client.UpdateBeadRequest{
			Assignee: &actor,
			Status:   &inProgress,
		})
		if err != nil {
			return fmt.Errorf("claiming bead %s: %w", id, err)
		}

		if jsonOutput {
			printBeadJSON(bead)
		} else {
			printBeadTable(bead)
		}

		// Print advice diff: show any new advice items that become relevant
		// because of the claimed bead's labels (e.g., role-specific advice).
		printAdviceDiff(ctx, bead)

		return nil
	},
}

func init() {
	claimCmd.Flags().BoolVar(&claimForce, "force", false, "bypass project-scope and single-claim checks")
}

// checkNoClaimed returns an error if the actor already has an in-progress bead
// claimed. Agents should claim at most one actionable bead at a time to keep
// their focus clear and avoid work-item confusion.
func checkNoClaimed(ctx context.Context) error {
	resp, err := beadsClient.ListBeads(ctx, &client.ListBeadsRequest{
		Status:   []string{"in_progress"},
		Assignee: actor,
		Limit:    5,
	})
	if err != nil {
		// Fail open: don't block claiming if we can't check.
		return nil
	}
	for _, b := range resp.Beads {
		// Skip non-issue beads — only actionable work (kind=issue) counts.
		if b.Kind != "issue" {
			continue
		}
		return fmt.Errorf("you already have claimed bead %s %q — unclaim it first with `kd unclaim %s`, or use --force to override", b.ID, b.Title, b.ID)
	}
	return nil
}

// checkNotEpic returns an error if the target bead is an epic. Epics are
// coordination containers, not actionable work items — claim an individual task,
// feature, or bug from the epic instead.
func checkNotEpic(ctx context.Context, beadID string) error {
	bead, err := beadsClient.GetBead(ctx, beadID)
	if err != nil {
		return nil // Fail open if we can't fetch the bead.
	}
	if bead.Type == "epic" {
		return fmt.Errorf("cannot claim an epic — epics are planning containers, not actionable work.\nClaim an individual task, feature, or bug from this epic instead.\nUse `kd dep list %s` to see the child work items, or `--force` to override.", beadID)
	}
	return nil
}

// checkProjectMatch verifies that the target bead belongs to the same project
// as the actor. The agent's project is read from the BOAT_PROJECT env var.
// Beads carry their project in a "project:<name>" label.
// If no project is configured for the agent, the check is skipped.
func checkProjectMatch(ctx context.Context, beadID string) error {
	agentProject := agentProject()
	if agentProject == "" {
		return nil // No project configured — skip check.
	}

	bead, err := beadsClient.GetBead(ctx, beadID)
	if err != nil {
		return nil // Fail open if we can't fetch the bead.
	}

	// Find project:<name> label on the target bead.
	beadProject := ""
	for _, label := range bead.Labels {
		if strings.HasPrefix(label, "project:") {
			beadProject = strings.TrimPrefix(label, "project:")
			break
		}
	}

	if beadProject == "" {
		return nil // Bead has no project label — allow claiming.
	}

	if beadProject != agentProject {
		return fmt.Errorf("bead %s belongs to project %q, but you are in project %q — only claim beads in your own project, or use --force to override", beadID, beadProject, agentProject)
	}
	return nil
}

