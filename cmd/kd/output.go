package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/groblegark/kbeads/internal/client"
	"github.com/groblegark/kbeads/internal/model"
)

func printBeadJSON(bead *model.Bead) {
	data, err := json.MarshalIndent(bead, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func printBeadTable(bead *model.Bead) {
	fmt.Printf("ID:          %s\n", bead.ID)
	fmt.Printf("Slug:        %s\n", bead.Slug)
	fmt.Printf("Title:       %s\n", bead.Title)
	fmt.Printf("Type:        %s\n", bead.Type)
	fmt.Printf("Kind:        %s\n", bead.Kind)
	fmt.Printf("Status:      %s\n", bead.Status)
	fmt.Printf("Priority:    %d\n", bead.Priority)
	fmt.Printf("Assignee:    %s\n", bead.Assignee)
	fmt.Printf("Owner:       %s\n", bead.Owner)
	if bead.Description != "" {
		fmt.Printf("Description: %s\n", bead.Description)
	}
	if len(bead.Labels) > 0 {
		fmt.Printf("Labels:      %s\n", strings.Join(bead.Labels, ", "))
	}
	if len(bead.Fields) > 0 {
		var fields map[string]string
		if json.Unmarshal(bead.Fields, &fields) == nil && len(fields) > 0 {
			keys := make([]string, 0, len(fields))
			for k := range fields {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			fmt.Println("Fields:")
			for _, k := range keys {
				fmt.Printf("  %-24s %s\n", k+":", fields[k])
			}
		}
	}
	fmt.Printf("Created By:  %s\n", bead.CreatedBy)
	if !bead.CreatedAt.IsZero() {
		fmt.Printf("Created At:  %s\n", bead.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if !bead.UpdatedAt.IsZero() {
		fmt.Printf("Updated At:  %s\n", bead.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
}

func printBeadListJSON(beads []*model.Bead) {
	data, err := json.MarshalIndent(beads, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func printBeadListTable(beads []*model.Bead, total int) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tTYPE\tPRIORITY\tTITLE\tASSIGNEE")
	for _, b := range beads {
		title := b.Title
		if len(title) > 50 {
			title = title[:47] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n",
			b.ID,
			b.Status,
			b.Type,
			b.Priority,
			title,
			b.Assignee,
		)
	}
	w.Flush()
	fmt.Printf("\n%d beads (%d total)\n", len(beads), total)
}

// resolvedDep pairs a dependency with its optionally-resolved target bead.
type resolvedDep struct {
	Dep  *model.Dependency
	Bead *model.Bead // nil if fetch failed
}

// fetchAndResolveDeps fetches dependencies for a bead and resolves each target.
// If types is non-empty, only dependencies matching one of the given types are included.
func fetchAndResolveDeps(ctx context.Context, c client.BeadsClient, beadID string, types []string) ([]resolvedDep, error) {
	deps, err := c.GetDependencies(ctx, beadID)
	if err != nil {
		return nil, err
	}
	return resolveBeadDeps(ctx, c, deps, types), nil
}

// resolveBeadDeps takes an existing dependency slice, filters by type, and resolves each target bead.
func resolveBeadDeps(ctx context.Context, c client.BeadsClient, deps []*model.Dependency, types []string) []resolvedDep {
	typeSet := make(map[string]bool, len(types))
	for _, t := range types {
		typeSet[t] = true
	}

	var resolved []resolvedDep
	for _, d := range deps {
		if len(typeSet) > 0 && !typeSet[string(d.Type)] {
			continue
		}
		rd := resolvedDep{Dep: d}
		fetchedBead, err := c.GetBead(ctx, d.DependsOnID)
		if err == nil {
			rd.Bead = fetchedBead
		}
		resolved = append(resolved, rd)
	}
	return resolved
}

// printDepSubSection prints resolved dependencies as indented lines.
func printDepSubSection(deps []resolvedDep, fields []string) {
	if len(deps) == 0 {
		return
	}
	if len(fields) == 0 {
		fields = []string{"id", "title", "status"}
	}
	for _, rd := range deps {
		if rd.Bead != nil {
			vals := make([]string, len(fields))
			for i, f := range fields {
				vals[i] = beadField(rd.Bead, f)
			}
			fmt.Printf("    %s: %s\n", string(rd.Dep.Type), strings.Join(vals, " | "))
		} else {
			fmt.Printf("    %s: %s (unresolved)\n", string(rd.Dep.Type), rd.Dep.DependsOnID)
		}
	}
}

// printBeadTableFiltered prints bead detail fields, restricted to the given whitelist.
// If fields is nil or empty, all fields are printed (delegates to printBeadTable).
func printBeadTableFiltered(bead *model.Bead, fields []string) {
	if len(fields) == 0 {
		printBeadTable(bead)
		return
	}
	fieldSet := make(map[string]bool, len(fields))
	for _, f := range fields {
		fieldSet[strings.ToLower(f)] = true
	}
	type fieldRow struct {
		label string
		key   string
		value string
	}
	rows := []fieldRow{
		{"ID", "id", bead.ID},
		{"Slug", "slug", bead.Slug},
		{"Title", "title", bead.Title},
		{"Type", "type", string(bead.Type)},
		{"Kind", "kind", string(bead.Kind)},
		{"Status", "status", string(bead.Status)},
		{"Priority", "priority", fmt.Sprintf("%d", bead.Priority)},
		{"Assignee", "assignee", bead.Assignee},
		{"Owner", "owner", bead.Owner},
	}
	for _, r := range rows {
		if fieldSet[r.key] {
			fmt.Printf("%-13s%s\n", r.label+":", r.value)
		}
	}
	if fieldSet["description"] && bead.Description != "" {
		fmt.Printf("%-13s%s\n", "Description:", bead.Description)
	}
	if fieldSet["labels"] && len(bead.Labels) > 0 {
		fmt.Printf("%-13s%s\n", "Labels:", strings.Join(bead.Labels, ", "))
	}
	if fieldSet["created_by"] {
		fmt.Printf("%-13s%s\n", "Created By:", bead.CreatedBy)
	}
	if fieldSet["created_at"] && !bead.CreatedAt.IsZero() {
		fmt.Printf("%-13s%s\n", "Created At:", bead.CreatedAt.Format("2006-01-02 15:04:05"))
	}
	if fieldSet["updated_at"] && !bead.UpdatedAt.IsZero() {
		fmt.Printf("%-13s%s\n", "Updated At:", bead.UpdatedAt.Format("2006-01-02 15:04:05"))
	}
}

// printComments prints bead comments in a standard format.
func printComments(comments []*model.Comment) {
	if len(comments) == 0 {
		return
	}
	fmt.Println()
	fmt.Println("Comments:")
	for _, c := range comments {
		ts := ""
		if !c.CreatedAt.IsZero() {
			ts = c.CreatedAt.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("  [%s] %s: %s\n", ts, c.Author, c.Text)
	}
}
