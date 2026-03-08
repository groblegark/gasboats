package postgres

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/groblegark/kbeads/internal/model"
)

// scannable is the interface satisfied by both *sql.Row and *sql.Rows.
type scannable interface {
	Scan(dest ...any) error
}

// scanBead scans a single row into a model.Bead.
// The row must contain columns in the order defined by beadColumns.
func scanBead(row scannable) (*model.Bead, error) {
	var b model.Bead
	var (
		slug        sql.NullString
		description sql.NullString
		notes       sql.NullString
		assignee    sql.NullString
		owner       sql.NullString
		createdBy   sql.NullString
		closedAt    sql.NullTime
		closedBy    sql.NullString
		dueAt       sql.NullTime
		deferUntil  sql.NullTime
		fields      []byte
	)

	err := row.Scan(
		&b.ID,
		&slug,
		&b.Kind,
		&b.Type,
		&b.Title,
		&description,
		&notes,
		&b.Status,
		&b.Priority,
		&assignee,
		&owner,
		&b.CreatedAt,
		&createdBy,
		&b.UpdatedAt,
		&closedAt,
		&closedBy,
		&dueAt,
		&deferUntil,
		&fields,
	)
	if err != nil {
		return nil, err
	}

	b.Slug = slug.String
	b.Description = description.String
	b.Notes = notes.String
	b.Assignee = assignee.String
	b.Owner = owner.String
	b.CreatedBy = createdBy.String
	b.ClosedBy = closedBy.String

	if closedAt.Valid {
		t := closedAt.Time
		b.ClosedAt = &t
	}
	if dueAt.Valid {
		t := dueAt.Time
		b.DueAt = &t
	}
	if deferUntil.Valid {
		t := deferUntil.Time
		b.DeferUntil = &t
	}
	if len(fields) > 0 {
		b.Fields = json.RawMessage(fields)
	}

	return &b, nil
}

// scanBeadWithTotal scans a row that has a leading total_count column
// followed by the standard bead columns. Used by queryListBeads with
// COUNT(*) OVER().
func scanBeadWithTotal(row scannable) (*model.Bead, int, error) {
	var total int
	var b model.Bead
	var (
		slug        sql.NullString
		description sql.NullString
		notes       sql.NullString
		assignee    sql.NullString
		owner       sql.NullString
		createdBy   sql.NullString
		closedAt    sql.NullTime
		closedBy    sql.NullString
		dueAt       sql.NullTime
		deferUntil  sql.NullTime
		fields      []byte
	)

	err := row.Scan(
		&total,
		&b.ID,
		&slug,
		&b.Kind,
		&b.Type,
		&b.Title,
		&description,
		&notes,
		&b.Status,
		&b.Priority,
		&assignee,
		&owner,
		&b.CreatedAt,
		&createdBy,
		&b.UpdatedAt,
		&closedAt,
		&closedBy,
		&dueAt,
		&deferUntil,
		&fields,
	)
	if err != nil {
		return nil, 0, err
	}

	b.Slug = slug.String
	b.Description = description.String
	b.Notes = notes.String
	b.Assignee = assignee.String
	b.Owner = owner.String
	b.CreatedBy = createdBy.String
	b.ClosedBy = closedBy.String

	if closedAt.Valid {
		t := closedAt.Time
		b.ClosedAt = &t
	}
	if dueAt.Valid {
		t := dueAt.Time
		b.DueAt = &t
	}
	if deferUntil.Valid {
		t := deferUntil.Time
		b.DeferUntil = &t
	}
	if len(fields) > 0 {
		b.Fields = json.RawMessage(fields)
	}

	return &b, total, nil
}

// scanDependency scans a single row into a model.Dependency.
func scanDependency(row scannable) (*model.Dependency, error) {
	var d model.Dependency
	var (
		createdBy sql.NullString
		metadata  sql.NullString
	)
	err := row.Scan(
		&d.BeadID,
		&d.DependsOnID,
		&d.Type,
		&d.CreatedAt,
		&createdBy,
		&metadata,
	)
	if err != nil {
		return nil, err
	}
	d.CreatedBy = createdBy.String
	d.Metadata = metadata.String
	return &d, nil
}

// scanDependencies scans multiple rows into a slice of model.Dependency pointers.
func scanDependencies(rows *sql.Rows) ([]*model.Dependency, error) {
	var deps []*model.Dependency
	for rows.Next() {
		d, err := scanDependency(rows)
		if err != nil {
			return nil, err
		}
		deps = append(deps, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return deps, nil
}

// scanComment scans a single row into a model.Comment.
func scanComment(row scannable) (*model.Comment, error) {
	var c model.Comment
	var author sql.NullString
	err := row.Scan(
		&c.ID,
		&c.BeadID,
		&author,
		&c.Text,
		&c.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	c.Author = author.String
	return &c, nil
}

// scanComments scans multiple rows into a slice of model.Comment pointers.
func scanComments(rows *sql.Rows) ([]*model.Comment, error) {
	var comments []*model.Comment
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return comments, nil
}

// scanEvent scans a single row into a model.Event.
func scanEvent(row scannable) (*model.Event, error) {
	var e model.Event
	var (
		actor   sql.NullString
		payload []byte
	)
	err := row.Scan(&e.ID, &e.Topic, &e.BeadID, &actor, &payload, &e.CreatedAt)
	if err != nil {
		return nil, err
	}
	e.Actor = actor.String
	if len(payload) > 0 {
		e.Payload = json.RawMessage(payload)
	}
	return &e, nil
}

// scanEvents scans multiple rows into a slice of model.Event pointers.
func scanEvents(rows *sql.Rows) ([]*model.Event, error) {
	var events []*model.Event
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

// scanConfig scans a single row into a model.Config.
func scanConfig(row scannable) (*model.Config, error) {
	var c model.Config
	var value []byte
	err := row.Scan(&c.Key, &value, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.Value = json.RawMessage(value)
	return &c, nil
}

// scanConfigs scans multiple rows into a slice of model.Config pointers.
func scanConfigs(rows *sql.Rows) ([]*model.Config, error) {
	var configs []*model.Config
	for rows.Next() {
		c, err := scanConfig(rows)
		if err != nil {
			return nil, err
		}
		configs = append(configs, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return configs, nil
}

// nullTimePtr converts a *time.Time to a sql.NullTime.
func nullTimePtr(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// nullString converts a string to sql.NullString; empty string is null.
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// jsonbBytes converts json.RawMessage to a []byte suitable for JSONB columns.
func jsonbBytes(m json.RawMessage) []byte {
	if len(m) == 0 {
		return nil
	}
	return []byte(m)
}
