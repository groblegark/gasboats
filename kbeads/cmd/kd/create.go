package main

import (
	"context"
	"fmt"

	"github.com/groblegark/kbeads/internal/client"
	"github.com/spf13/cobra"
)

// agentRequiredFields lists the fields the gasboat controller requires for
// type=agent beads.  Without them the SSE-triggered spawn silently skips the
// bead (buildEvent returns false when role=='' || name=='').
var agentRequiredFields = []string{"agent", "role", "project"}

// validateAgentFields returns an error if any required agent field is missing
// from the provided key=value pairs.
func validateAgentFields(pairs []string) error {
	present := make(map[string]bool, len(pairs))
	for _, p := range pairs {
		k, _, ok := splitField(p)
		if ok {
			present[k] = true
		}
	}
	var missing []string
	for _, f := range agentRequiredFields {
		if !present[f] {
			missing = append(missing, f)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"type=agent requires fields: %v\n"+
				"provide them with -f flags (or --role for the role field), e.g.:\n"+
				"  kd create <title> --type agent --role crew -f agent=<name> -f project=<project>",
			missing,
		)
	}
	return nil
}

// parseFields converts -f key=value pairs into a JSON object (bytes).
// Values that look like JSON (start with { [ " or are true/false/null/number)
// are embedded as-is; everything else is quoted as a string.
func parseFields(pairs []string) ([]byte, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	m := make(map[string]any, len(pairs))
	for _, p := range pairs {
		k, v, ok := splitField(p)
		if !ok {
			return nil, fmt.Errorf("invalid field %q: expected key=value", p)
		}
		m[k] = rawOrString(v)
	}
	b, err := jsonMarshal(m)
	if err != nil {
		return nil, fmt.Errorf("encoding fields: %w", err)
	}
	return b, nil
}

var createCmd = &cobra.Command{
	Use:     "create <title>",
	Short:   "Create a new bead",
	GroupID: "beads",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		title := args[0]

		description, _ := cmd.Flags().GetString("description")
		beadType, _ := cmd.Flags().GetString("type")
		kind, _ := cmd.Flags().GetString("kind")
		priority, _ := cmd.Flags().GetInt("priority")
		labels, _ := cmd.Flags().GetStringSlice("label")
		assignee, _ := cmd.Flags().GetString("assignee")
		owner, _ := cmd.Flags().GetString("owner")
		role, _ := cmd.Flags().GetString("role")

		fieldPairs, _ := cmd.Flags().GetStringArray("field")

		if role != "" {
			labels = append(labels, "role:"+role)
			// For agent beads, --role also satisfies the required "role" field
			// so the user doesn't need to pass both --role and -f role=<value>.
			if beadType == "agent" {
				hasRoleField := false
				for _, p := range fieldPairs {
					k, _, ok := splitField(p)
					if ok && k == "role" {
						hasRoleField = true
						break
					}
				}
				if !hasRoleField {
					fieldPairs = append(fieldPairs, "role="+role)
				}
			}
		}

		if beadType == "agent" {
			if err := validateAgentFields(fieldPairs); err != nil {
				return err
			}
		}

		// Apply --project label and cross-project guard.
		project, _ := cmd.Flags().GetString("project")
		force, _ := cmd.Flags().GetBool("force")
		if project != "" {
			labels = append(labels, "project:"+project)
			if !force {
				if ap := agentProject(); ap != "" && ap != project {
					return fmt.Errorf("cross-project create: you are in project %q but creating in %q — use --force to override", ap, project)
				}
			}
		}

		fieldsJSON, err := parseFields(fieldPairs)
		if err != nil {
			return fmt.Errorf("parsing fields: %w", err)
		}

		req := &client.CreateBeadRequest{
			Title:       title,
			Description: description,
			Type:        beadType,
			Kind:        kind,
			Priority:    priority,
			Labels:      labels,
			Assignee:    assignee,
			Owner:       owner,
			CreatedBy:   actor,
			Fields:      fieldsJSON,
		}

		bead, err := beadsClient.CreateBead(context.Background(), req)
		if err != nil {
			return fmt.Errorf("creating bead: %w", err)
		}

		if jsonOutput {
			printBeadJSON(bead)
		} else {
			printBeadTable(bead)
		}
		return nil
	},
}

func init() {
	createCmd.Flags().StringP("description", "d", "", "bead description")
	createCmd.Flags().StringP("type", "t", "task", "bead type")
	createCmd.Flags().StringP("kind", "k", "", "bead kind (optional, inferred from type)")
	createCmd.Flags().IntP("priority", "p", 2, "bead priority")
	createCmd.Flags().StringSliceP("label", "l", nil, "labels (repeatable)")
	createCmd.Flags().String("assignee", "", "assignee")
	createCmd.Flags().String("owner", "", "owner")
	createCmd.Flags().String("role", "", "role label to assign (adds role:<value> label; for --type agent also sets the role field)")
	createCmd.Flags().StringArrayP("field", "f", nil, "typed field (key=value, repeatable)")
	createCmd.Flags().String("project", defaultProject(), "project label (default: $KD_PROJECT or $BOAT_PROJECT)")
	createCmd.Flags().Bool("force", false, "bypass cross-project check")
}
