package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/groblegark/kbeads/internal/model"
	"github.com/lib/pq"
)

// queryGetDependenciesForBeads returns forward dependencies keyed by bead ID.
func queryGetDependenciesForBeads(ctx context.Context, db executor, beadIDs []string) (map[string][]*model.Dependency, error) {
	if len(beadIDs) == 0 {
		return make(map[string][]*model.Dependency), nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT bead_id, depends_on_id, type, created_at, created_by, metadata
		FROM deps
		WHERE bead_id = ANY($1::text[])`,
		pq.Array(beadIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("batch deps: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]*model.Dependency)
	for rows.Next() {
		d, err := scanDependency(rows)
		if err != nil {
			return nil, fmt.Errorf("scan batch dep: %w", err)
		}
		result[d.BeadID] = append(result[d.BeadID], d)
	}
	return result, rows.Err()
}

// queryGetReverseDependenciesForBeads returns reverse dependencies keyed by target bead ID.
func queryGetReverseDependenciesForBeads(ctx context.Context, db executor, beadIDs []string) (map[string][]*model.Dependency, error) {
	if len(beadIDs) == 0 {
		return make(map[string][]*model.Dependency), nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT bead_id, depends_on_id, type, created_at, created_by, metadata
		FROM deps
		WHERE depends_on_id = ANY($1::text[])`,
		pq.Array(beadIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("batch reverse deps: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]*model.Dependency)
	for rows.Next() {
		d, err := scanDependency(rows)
		if err != nil {
			return nil, fmt.Errorf("scan batch reverse dep: %w", err)
		}
		result[d.DependsOnID] = append(result[d.DependsOnID], d)
	}
	return result, rows.Err()
}

// queryGetDependencyCounts returns forward and reverse dep counts per bead.
func queryGetDependencyCounts(ctx context.Context, db executor, beadIDs []string) (map[string]*model.DependencyCounts, error) {
	if len(beadIDs) == 0 {
		return make(map[string]*model.DependencyCounts), nil
	}

	result := make(map[string]*model.DependencyCounts, len(beadIDs))

	// Forward counts (how many things this bead depends on)
	fwdRows, err := db.QueryContext(ctx, `
		SELECT bead_id, COUNT(*) FROM deps
		WHERE bead_id = ANY($1::text[])
		GROUP BY bead_id`,
		pq.Array(beadIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("dep counts forward: %w", err)
	}
	defer fwdRows.Close()

	for fwdRows.Next() {
		var id string
		var count int
		if err := fwdRows.Scan(&id, &count); err != nil {
			return nil, fmt.Errorf("scan dep count: %w", err)
		}
		result[id] = &model.DependencyCounts{DependencyCount: count}
	}
	if err := fwdRows.Err(); err != nil {
		return nil, err
	}

	// Reverse counts (how many things depend on this bead)
	revRows, err := db.QueryContext(ctx, `
		SELECT depends_on_id, COUNT(*) FROM deps
		WHERE depends_on_id = ANY($1::text[])
		GROUP BY depends_on_id`,
		pq.Array(beadIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("dep counts reverse: %w", err)
	}
	defer revRows.Close()

	for revRows.Next() {
		var id string
		var count int
		if err := revRows.Scan(&id, &count); err != nil {
			return nil, fmt.Errorf("scan reverse dep count: %w", err)
		}
		if dc, ok := result[id]; ok {
			dc.DependentCount = count
		} else {
			result[id] = &model.DependencyCounts{DependentCount: count}
		}
	}
	return result, revRows.Err()
}

// queryGetLabelsForBeads returns labels keyed by bead ID.
func queryGetLabelsForBeads(ctx context.Context, db executor, beadIDs []string) (map[string][]string, error) {
	if len(beadIDs) == 0 {
		return make(map[string][]string), nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT bead_id, label FROM labels
		WHERE bead_id = ANY($1::text[])`,
		pq.Array(beadIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("batch labels: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var beadID, label string
		if err := rows.Scan(&beadID, &label); err != nil {
			return nil, fmt.Errorf("scan batch label: %w", err)
		}
		result[beadID] = append(result[beadID], label)
	}
	return result, rows.Err()
}

// queryGetBlockedByForBeads returns open blocker IDs keyed by bead ID.
func queryGetBlockedByForBeads(ctx context.Context, db executor, beadIDs []string) (map[string][]string, error) {
	if len(beadIDs) == 0 {
		return make(map[string][]string), nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT d.bead_id, d.depends_on_id
		FROM deps d
		JOIN beads blocker ON d.depends_on_id = blocker.id
		WHERE d.bead_id = ANY($1::text[])
		  AND d.type = 'blocks'
		  AND blocker.status IN ('open', 'in_progress', 'blocked', 'deferred')`,
		pq.Array(beadIDs),
	)
	if err != nil {
		return nil, fmt.Errorf("batch blocked-by: %w", err)
	}
	defer rows.Close()

	result := make(map[string][]string)
	for rows.Next() {
		var beadID, blockerID string
		if err := rows.Scan(&beadID, &blockerID); err != nil {
			return nil, fmt.Errorf("scan blocked-by: %w", err)
		}
		result[beadID] = append(result[beadID], blockerID)
	}
	return result, rows.Err()
}

// queryGetBeadsByIDs fetches beads by their IDs without relational data.
func queryGetBeadsByIDs(ctx context.Context, db executor, ids []string) ([]*model.Bead, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx,
		`SELECT `+beadColumns+` FROM beads WHERE id = ANY($1::text[])`,
		pq.Array(ids),
	)
	if err != nil {
		return nil, fmt.Errorf("get beads by ids: %w", err)
	}
	defer rows.Close()

	var beads []*model.Bead
	for rows.Next() {
		b, err := scanBead(rows)
		if err != nil {
			return nil, fmt.Errorf("scan bead by id: %w", err)
		}
		beads = append(beads, b)
	}
	return beads, rows.Err()
}

// addExcludeTypesClause appends an exclude-types WHERE clause using a subquery-free approach.
func addExcludeTypesClause(whereClauses []string, args []any, argIdx *int, excludeTypes []model.BeadType) ([]string, []any) {
	if len(excludeTypes) == 0 {
		return whereClauses, args
	}
	placeholders := make([]string, len(excludeTypes))
	for i, t := range excludeTypes {
		*argIdx++
		placeholders[i] = fmt.Sprintf("$%d", *argIdx)
		args = append(args, string(t))
	}
	whereClauses = append(whereClauses, "type NOT IN ("+strings.Join(placeholders, ", ")+")")
	return whereClauses, args
}
