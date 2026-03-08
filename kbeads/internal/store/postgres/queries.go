package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/groblegark/kbeads/internal/model"
	"github.com/lib/pq"
)

// beadColumns is the column list used for SELECT statements on the beads table.
const beadColumns = `id, slug, kind, type, title, description, notes,
	status, priority, assignee, owner, created_at, created_by, updated_at,
	closed_at, closed_by, due_at, defer_until, fields`

// executor is the interface satisfied by both *sql.DB and *sql.Tx.
type executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func queryCreateBead(ctx context.Context, db executor, b *model.Bead) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO beads (
			id, slug, kind, type, title, description, notes,
			status, priority, assignee, owner, created_at, created_by, updated_at,
			closed_at, closed_by, due_at, defer_until, fields
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13, $14,
			$15, $16, $17, $18, $19
		)`,
		b.ID,
		nullString(b.Slug),
		string(b.Kind),
		string(b.Type),
		b.Title,
		b.Description,
		b.Notes,
		string(b.Status),
		b.Priority,
		b.Assignee,
		b.Owner,
		b.CreatedAt,
		b.CreatedBy,
		b.UpdatedAt,
		nullTimePtr(b.ClosedAt),
		b.ClosedBy,
		nullTimePtr(b.DueAt),
		nullTimePtr(b.DeferUntil),
		jsonbBytes(b.Fields),
	)
	return err
}

func queryGetBead(ctx context.Context, db executor, id string) (*model.Bead, error) {
	row := db.QueryRowContext(ctx, `SELECT `+beadColumns+` FROM beads WHERE id = $1`, id)
	b, err := scanBead(row)
	if err != nil {
		return nil, err
	}

	// Fetch labels.
	labels, err := queryGetLabels(ctx, db, id)
	if err != nil {
		return nil, err
	}
	b.Labels = labels

	// Fetch dependencies.
	deps, err := queryGetDependencies(ctx, db, id)
	if err != nil {
		return nil, err
	}
	b.Dependencies = deps

	// Fetch comments.
	comments, err := queryGetComments(ctx, db, id)
	if err != nil {
		return nil, err
	}
	b.Comments = comments

	return b, nil
}

func queryListBeads(ctx context.Context, db executor, filter model.BeadFilter) ([]*model.Bead, int, error) {
	var (
		whereClauses []string
		args         []any
		argIdx       int
	)

	nextArg := func() string {
		argIdx++
		return fmt.Sprintf("$%d", argIdx)
	}

	if len(filter.Status) > 0 {
		placeholders := make([]string, len(filter.Status))
		for i, s := range filter.Status {
			placeholders[i] = nextArg()
			args = append(args, string(s))
		}
		whereClauses = append(whereClauses, "status IN ("+strings.Join(placeholders, ", ")+")")
	}

	if len(filter.Type) > 0 {
		placeholders := make([]string, len(filter.Type))
		for i, t := range filter.Type {
			placeholders[i] = nextArg()
			args = append(args, string(t))
		}
		whereClauses = append(whereClauses, "type IN ("+strings.Join(placeholders, ", ")+")")
	}

	if len(filter.ExcludeTypes) > 0 {
		placeholders := make([]string, len(filter.ExcludeTypes))
		for i, t := range filter.ExcludeTypes {
			placeholders[i] = nextArg()
			args = append(args, string(t))
		}
		whereClauses = append(whereClauses, "type NOT IN ("+strings.Join(placeholders, ", ")+")")
	}

	if len(filter.Kind) > 0 {
		placeholders := make([]string, len(filter.Kind))
		for i, k := range filter.Kind {
			placeholders[i] = nextArg()
			args = append(args, string(k))
		}
		whereClauses = append(whereClauses, "kind IN ("+strings.Join(placeholders, ", ")+")")
	}

	if filter.Priority != nil {
		whereClauses = append(whereClauses, "priority = "+nextArg())
		args = append(args, *filter.Priority)
	}

	if filter.PriorityMin != nil {
		whereClauses = append(whereClauses, "priority >= "+nextArg())
		args = append(args, *filter.PriorityMin)
	}

	if filter.PriorityMax != nil {
		whereClauses = append(whereClauses, "priority <= "+nextArg())
		args = append(args, *filter.PriorityMax)
	}

	if filter.Assignee != "" {
		whereClauses = append(whereClauses, "assignee = "+nextArg())
		args = append(args, filter.Assignee)
	}

	if len(filter.Labels) > 0 {
		for _, label := range filter.Labels {
			p := nextArg()
			whereClauses = append(whereClauses,
				fmt.Sprintf("EXISTS (SELECT 1 FROM labels WHERE labels.bead_id = beads.id AND labels.label = %s)", p))
			args = append(args, label)
		}
	}

	if len(filter.LabelsAny) > 0 {
		p := nextArg()
		whereClauses = append(whereClauses,
			fmt.Sprintf("EXISTS (SELECT 1 FROM labels WHERE labels.bead_id = beads.id AND labels.label = ANY(%s::text[]))", p))
		args = append(args, pq.Array(filter.LabelsAny))
	}

	if len(filter.IDs) > 0 {
		p := nextArg()
		whereClauses = append(whereClauses, "id = ANY("+p+"::text[])")
		args = append(args, pq.Array(filter.IDs))
	}

	if filter.UpdatedAfter != nil {
		whereClauses = append(whereClauses, "updated_at > "+nextArg())
		args = append(args, *filter.UpdatedAfter)
	}

	if filter.ParentID != "" {
		p := nextArg()
		whereClauses = append(whereClauses,
			fmt.Sprintf("EXISTS (SELECT 1 FROM deps WHERE deps.bead_id = beads.id AND deps.depends_on_id = %s AND deps.type = 'parent-child')", p))
		args = append(args, filter.ParentID)
	}

	if filter.Search != "" {
		p := nextArg()
		whereClauses = append(whereClauses,
			fmt.Sprintf("(title ILIKE '%%' || %s || '%%' OR description ILIKE '%%' || %s || '%%')", p, p))
		args = append(args, filter.Search)
	}

	for key, val := range filter.Fields {
		kp := nextArg()
		vp := nextArg()
		whereClauses = append(whereClauses, fmt.Sprintf("fields->>%s = %s", kp, vp))
		args = append(args, key, val)
	}

	if filter.NoOpenDeps {
		whereClauses = append(whereClauses,
			"NOT EXISTS (SELECT 1 FROM deps d JOIN beads dep ON d.depends_on_id = dep.id "+
				"WHERE d.bead_id = beads.id AND dep.status IN ('open', 'in_progress', 'deferred'))")
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = " WHERE " + strings.Join(whereClauses, " AND ")
	}

	// Single query with COUNT(*) OVER() to get total and rows atomically.
	dataQuery := "SELECT COUNT(*) OVER() AS total_count, " + beadColumns + " FROM beads" + whereSQL + " ORDER BY " + parseSortClause(filter.Sort)

	if filter.Limit > 0 {
		dataQuery += " LIMIT " + nextArg()
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		dataQuery += " OFFSET " + nextArg()
		args = append(args, filter.Offset)
	}

	rows, err := db.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list beads: %w", err)
	}
	defer rows.Close()

	var beads []*model.Bead
	var total int
	for rows.Next() {
		b, t, err := scanBeadWithTotal(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("scan beads: %w", err)
		}
		total = t
		beads = append(beads, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("scan beads: %w", err)
	}

	// Populate labels for all returned beads in one query.
	if len(beads) > 0 {
		idPlaceholders := make([]string, len(beads))
		labelArgs := make([]any, len(beads))
		for i, b := range beads {
			idPlaceholders[i] = fmt.Sprintf("$%d", i+1)
			labelArgs[i] = b.ID
		}
		labelRows, err := db.QueryContext(ctx,
			"SELECT bead_id, label FROM labels WHERE bead_id IN ("+strings.Join(idPlaceholders, ", ")+")",
			labelArgs...)
		if err != nil {
			return nil, 0, fmt.Errorf("list beads labels: %w", err)
		}
		defer labelRows.Close()

		labelMap := make(map[string][]string, len(beads))
		for labelRows.Next() {
			var beadID, label string
			if err := labelRows.Scan(&beadID, &label); err != nil {
				return nil, 0, fmt.Errorf("scan bead label: %w", err)
			}
			labelMap[beadID] = append(labelMap[beadID], label)
		}
		if err := labelRows.Err(); err != nil {
			return nil, 0, fmt.Errorf("list beads labels: %w", err)
		}
		for _, b := range beads {
			b.Labels = labelMap[b.ID] // nil if no labels (same as before for unlabelled beads)
		}
	}

	return beads, total, nil
}

func queryUpdateBead(ctx context.Context, db executor, b *model.Bead) error {
	return db.QueryRowContext(ctx, `
		UPDATE beads SET
			slug = $2,
			kind = $3,
			type = $4,
			title = $5,
			description = $6,
			notes = $7,
			status = $8,
			priority = $9,
			assignee = $10,
			owner = $11,
			updated_at = NOW(),
			closed_at = $12,
			closed_by = $13,
			due_at = $14,
			defer_until = $15,
			fields = $16
		WHERE id = $1
		RETURNING updated_at`,
		b.ID,
		nullString(b.Slug),
		string(b.Kind),
		string(b.Type),
		b.Title,
		b.Description,
		b.Notes,
		string(b.Status),
		b.Priority,
		b.Assignee,
		b.Owner,
		nullTimePtr(b.ClosedAt),
		b.ClosedBy,
		nullTimePtr(b.DueAt),
		nullTimePtr(b.DeferUntil),
		jsonbBytes(b.Fields),
	).Scan(&b.UpdatedAt)
}

func queryCloseBead(ctx context.Context, db executor, id string, closedBy string) (*model.Bead, error) {
	row := db.QueryRowContext(ctx, `
		UPDATE beads
		SET status = 'closed', closed_at = NOW(), closed_by = $2, updated_at = NOW()
		WHERE id = $1
		RETURNING `+beadColumns,
		id, closedBy,
	)
	b, err := scanBead(row)
	if err != nil {
		return nil, err
	}

	// Fetch relational data.
	labels, err := queryGetLabels(ctx, db, id)
	if err != nil {
		return nil, err
	}
	b.Labels = labels

	deps, err := queryGetDependencies(ctx, db, id)
	if err != nil {
		return nil, err
	}
	b.Dependencies = deps

	comments, err := queryGetComments(ctx, db, id)
	if err != nil {
		return nil, err
	}
	b.Comments = comments

	return b, nil
}

func queryDeleteBead(ctx context.Context, db executor, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM beads WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func queryAddDependency(ctx context.Context, db executor, dep *model.Dependency) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO deps (bead_id, depends_on_id, type, created_at, created_by, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		dep.BeadID,
		dep.DependsOnID,
		string(dep.Type),
		dep.CreatedAt,
		dep.CreatedBy,
		dep.Metadata,
	)
	return err
}

func queryRemoveDependency(ctx context.Context, db executor, beadID, dependsOnID string, depType model.DependencyType) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM deps
		WHERE bead_id = $1 AND depends_on_id = $2 AND type = $3`,
		beadID, dependsOnID, string(depType),
	)
	return err
}

func queryGetDependencies(ctx context.Context, db executor, beadID string) ([]*model.Dependency, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT bead_id, depends_on_id, type, created_at, created_by, metadata
		FROM deps
		WHERE bead_id = $1`,
		beadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDependencies(rows)
}

func queryGetReverseDependencies(ctx context.Context, db executor, beadID string) ([]*model.Dependency, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT bead_id, depends_on_id, type, created_at, created_by, metadata
		FROM deps
		WHERE depends_on_id = $1`,
		beadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDependencies(rows)
}

func queryAddLabel(ctx context.Context, db executor, beadID, label string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO labels (bead_id, label)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING`,
		beadID, label,
	)
	return err
}

func queryRemoveLabel(ctx context.Context, db executor, beadID, label string) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM labels
		WHERE bead_id = $1 AND label = $2`,
		beadID, label,
	)
	return err
}

func queryGetLabels(ctx context.Context, db executor, beadID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT label FROM labels WHERE bead_id = $1`,
		beadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var labels []string
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, rows.Err()
}

func queryAddComment(ctx context.Context, db executor, c *model.Comment) error {
	return db.QueryRowContext(ctx, `
		INSERT INTO comments (bead_id, author, text)
		VALUES ($1, $2, $3)
		RETURNING id, created_at`,
		c.BeadID, c.Author, c.Text,
	).Scan(&c.ID, &c.CreatedAt)
}

func queryGetComments(ctx context.Context, db executor, beadID string) ([]*model.Comment, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, bead_id, author, text, created_at
		FROM comments
		WHERE bead_id = $1
		ORDER BY created_at ASC`,
		beadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanComments(rows)
}

func queryRecordEvent(ctx context.Context, db executor, e *model.Event) error {
	return db.QueryRowContext(ctx, `
		INSERT INTO events (topic, bead_id, actor, payload)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		e.Topic, e.BeadID, e.Actor, []byte(e.Payload),
	).Scan(&e.ID, &e.CreatedAt)
}

func queryGetEvents(ctx context.Context, db executor, beadID string) ([]*model.Event, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, topic, bead_id, actor, payload, created_at
		FROM events
		WHERE bead_id = $1
		ORDER BY created_at ASC`,
		beadID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func querySetConfig(ctx context.Context, db executor, c *model.Config) error {
	return db.QueryRowContext(ctx, `
		INSERT INTO configs (key, value)
		VALUES ($1, $2)
		ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = NOW()
		RETURNING created_at, updated_at`,
		c.Key, []byte(c.Value),
	).Scan(&c.CreatedAt, &c.UpdatedAt)
}

func queryGetConfig(ctx context.Context, db executor, key string) (*model.Config, error) {
	row := db.QueryRowContext(ctx, `
		SELECT key, value, created_at, updated_at
		FROM configs WHERE key = $1`, key)
	return scanConfig(row)
}

func queryListConfigs(ctx context.Context, db executor, namespace string) ([]*model.Config, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT key, value, created_at, updated_at
		FROM configs WHERE key LIKE $1 || ':%'
		ORDER BY key`, namespace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConfigs(rows)
}

func queryListAllConfigs(ctx context.Context, db executor) ([]*model.Config, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT key, value, created_at, updated_at
		FROM configs ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanConfigs(rows)
}

func queryDeleteConfig(ctx context.Context, db executor, key string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM configs WHERE key = $1`, key)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func queryGetStats(ctx context.Context, db executor) (*model.GraphStats, error) {
	stats := &model.GraphStats{}
	err := db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN status = 'open' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'in_progress' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'closed' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN status = 'deferred' THEN 1 ELSE 0 END), 0)
		FROM beads`).Scan(
		&stats.TotalOpen,
		&stats.TotalInProgress,
		&stats.TotalBlocked,
		&stats.TotalClosed,
		&stats.TotalDeferred,
	)
	if err != nil {
		return nil, fmt.Errorf("stats: %w", err)
	}
	return stats, nil
}

func parseSortClause(sort string) string {
	if sort == "" {
		return "created_at DESC"
	}
	desc := strings.HasPrefix(sort, "-")
	col := strings.TrimPrefix(sort, "-")
	allowed := map[string]bool{
		"priority": true, "created_at": true, "updated_at": true,
		"title": true, "status": true, "type": true,
	}
	if !allowed[col] {
		return "created_at DESC"
	}
	if desc {
		return col + " DESC"
	}
	return col + " ASC"
}
