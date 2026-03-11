package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/groblegark/kbeads/internal/client"
	"github.com/groblegark/kbeads/internal/model"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search beads by text query with field filters",
	Long: `Search beads by text query, field filters, and structured output.

Examples:
  kd search "deployment issue"                    # text search
  kd search -t agent --where project=gasboat      # field equality
  kd search -t agent --where project=             # field is empty/absent
  kd search -t agent --fields project,role,mode   # custom columns
  kd search -s '!closed'                          # exclude closed beads`,
	GroupID: "beads",
	Args:    cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		statusFlags, _ := cmd.Flags().GetStringSlice("status")
		beadType, _ := cmd.Flags().GetStringSlice("type")
		kind, _ := cmd.Flags().GetStringSlice("kind")
		limit, _ := cmd.Flags().GetInt("limit")
		whereFlags, _ := cmd.Flags().GetStringSlice("where")
		fieldsFlag, _ := cmd.Flags().GetString("fields")
		labels, _ := cmd.Flags().GetStringSlice("label")
		assignee, _ := cmd.Flags().GetString("assignee")
		sort, _ := cmd.Flags().GetString("sort")

		// Process status negation: "!closed" → all statuses except "closed".
		statuses, err := resolveStatuses(statusFlags)
		if err != nil {
			return err
		}

		// Parse --where flags into field filters.
		fieldFilters := parseWhereFlags(whereFlags)

		req := &client.ListBeadsRequest{
			Search:       query,
			Status:       statuses,
			Type:         beadType,
			Kind:         kind,
			Labels:       labels,
			Assignee:     assignee,
			Sort:         sort,
			FieldFilters: fieldFilters,
			Limit:        limit,
		}

		resp, err := beadsClient.ListBeads(context.Background(), req)
		if err != nil {
			return fmt.Errorf("searching beads: %w", err)
		}

		if jsonOutput {
			printBeadListJSON(resp.Beads)
		} else if fieldsFlag != "" {
			columns := strings.Split(fieldsFlag, ",")
			printBeadListCustom(resp.Beads, resp.Total, columns)
		} else {
			printBeadListTable(resp.Beads, resp.Total)
		}
		return nil
	},
}

func init() {
	searchCmd.Flags().StringSliceP("status", "s", nil, "filter by status (repeatable, prefix ! to negate)")
	searchCmd.Flags().StringSliceP("type", "t", nil, "filter by type (repeatable)")
	searchCmd.Flags().StringSliceP("kind", "k", nil, "filter by kind (repeatable)")
	searchCmd.Flags().StringSliceP("label", "l", nil, "filter by label (repeatable)")
	searchCmd.Flags().StringSliceP("where", "w", nil, "field filter as key=value (repeatable, empty value matches absent)")
	searchCmd.Flags().String("fields", "", "comma-separated columns to display (e.g. id,title,project,role)")
	searchCmd.Flags().String("assignee", "", "filter by assignee")
	searchCmd.Flags().String("sort", "", "sort by column (prefix - for descending, e.g. -priority)")
	searchCmd.Flags().Int("limit", 20, "maximum number of results")
}

// allStatuses is the set of valid bead statuses.
var allStatuses = []string{"open", "in_progress", "deferred", "closed"}

// resolveStatuses handles status negation. A status prefixed with "!" means
// "all statuses except this one". Multiple negations are AND'd together.
func resolveStatuses(flags []string) ([]string, error) {
	var positive, negative []string
	for _, s := range flags {
		if strings.HasPrefix(s, "!") {
			negative = append(negative, strings.TrimPrefix(s, "!"))
		} else {
			positive = append(positive, s)
		}
	}
	if len(negative) > 0 && len(positive) > 0 {
		return nil, fmt.Errorf("cannot mix positive and negated statuses")
	}
	if len(negative) > 0 {
		excluded := make(map[string]bool, len(negative))
		for _, n := range negative {
			excluded[n] = true
		}
		var result []string
		for _, s := range allStatuses {
			if !excluded[s] {
				result = append(result, s)
			}
		}
		return result, nil
	}
	return positive, nil
}

// parseWhereFlags parses "key=value" pairs from --where flags.
// An empty value (e.g. "project=") matches beads where the field is absent or empty.
func parseWhereFlags(flags []string) map[string]string {
	if len(flags) == 0 {
		return nil
	}
	m := make(map[string]string, len(flags))
	for _, f := range flags {
		if idx := strings.IndexByte(f, '='); idx >= 0 {
			m[f[:idx]] = f[idx+1:]
		}
	}
	return m
}

// printBeadListCustom prints beads with user-selected columns.
// Supported column names: id, title, status, type, kind, priority, assignee,
// owner, created_by, labels, plus any custom field name (from bead.Fields).
func printBeadListCustom(beads []*model.Bead, total int, columns []string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	headers := make([]string, len(columns))
	for i, c := range columns {
		headers[i] = strings.ToUpper(strings.TrimSpace(c))
	}
	fmt.Fprintln(w, strings.Join(headers, "\t"))

	for _, b := range beads {
		vals := make([]string, len(columns))
		for i, c := range columns {
			vals[i] = beadColumnValue(b, strings.TrimSpace(c))
		}
		fmt.Fprintln(w, strings.Join(vals, "\t"))
	}
	w.Flush()
	fmt.Printf("\n%d beads (%d total)\n", len(beads), total)
}

// beadColumnValue extracts a display value for a named column from a bead.
func beadColumnValue(b *model.Bead, col string) string {
	switch col {
	case "id":
		return b.ID
	case "title":
		title := b.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		return title
	case "status":
		return string(b.Status)
	case "type":
		return string(b.Type)
	case "kind":
		return string(b.Kind)
	case "priority":
		return fmt.Sprintf("%d", b.Priority)
	case "assignee":
		return b.Assignee
	case "owner":
		return b.Owner
	case "created_by":
		return b.CreatedBy
	case "labels":
		return strings.Join(b.Labels, ",")
	default:
		// Try custom field from bead.Fields JSON.
		return beadFieldValue(b, col)
	}
}

// beadFieldValue extracts a custom field value from the bead's Fields JSON.
func beadFieldValue(b *model.Bead, key string) string {
	if len(b.Fields) == 0 {
		return ""
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(b.Fields, &fields) != nil {
		return ""
	}
	raw, ok := fields[key]
	if !ok {
		return ""
	}
	// Try to unquote string values; fall back to raw JSON.
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}
