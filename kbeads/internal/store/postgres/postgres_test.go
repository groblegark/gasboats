package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/groblegark/kbeads/internal/model"
)

// newMockDB creates a sqlmock database with automatic cleanup and expectation checking.
func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unfulfilled expectations: %v", err)
		}
		db.Close()
	})
	return db, mock
}

// beadWithTotalColumns is the column list for queryListBeads results (total_count + bead columns).
var beadWithTotalColumns = []string{
	"total_count",
	"id", "slug", "kind", "type", "title", "description", "notes",
	"status", "priority", "assignee", "owner", "created_at", "created_by", "updated_at",
	"closed_at", "closed_by", "due_at", "defer_until", "fields",
}

// beadRowColumns is the column list for scanBead results (standard bead columns).
var beadRowColumns = []string{
	"id", "slug", "kind", "type", "title", "description", "notes",
	"status", "priority", "assignee", "owner", "created_at", "created_by", "updated_at",
	"closed_at", "closed_by", "due_at", "defer_until", "fields",
}

// addBeadWithTotalRow adds a minimal bead row with a leading total_count to a sqlmock.Rows.
func addBeadWithTotalRow(rows *sqlmock.Rows, total int, id, kind, typ, title, status string, priority int, now time.Time) *sqlmock.Rows {
	return rows.AddRow(
		total,
		id, nil, kind, typ, title, nil, nil,
		status, priority, nil, nil, now, nil, now,
		nil, nil, nil, nil, nil,
	)
}

// emptyRelationalExpectations sets up sqlmock expectations for the three relational
// queries (labels, deps, comments) that follow a bead query, returning empty results.
func emptyRelationalExpectations(mock sqlmock.Sqlmock, id string) {
	mock.ExpectQuery("SELECT label FROM labels WHERE bead_id = \\$1").WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"label"}))
	mock.ExpectQuery("SELECT .+ FROM deps WHERE bead_id = \\$1").WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"bead_id", "depends_on_id", "type", "created_at", "created_by", "metadata"}))
	mock.ExpectQuery("SELECT .+ FROM comments WHERE bead_id = \\$1").WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "bead_id", "author", "text", "created_at"}))
}

func TestParseSortClause(t *testing.T) {
	for _, tc := range []struct {
		input string
		want  string
	}{
		{"", "created_at DESC"},
		{"priority", "priority ASC"},
		{"-priority", "priority DESC"},
		{"evil_column", "created_at DESC"},
		{"-evil_column", "created_at DESC"},
	} {
		if got := parseSortClause(tc.input); got != tc.want {
			t.Errorf("parseSortClause(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
	// All allowed columns.
	for _, col := range []string{"priority", "created_at", "updated_at", "title", "status", "type"} {
		if got := parseSortClause(col); got != col+" ASC" {
			t.Errorf("parseSortClause(%q) = %q, want %q", col, got, col+" ASC")
		}
		if got := parseSortClause("-" + col); got != col+" DESC" {
			t.Errorf("parseSortClause(-%q) = %q, want %q", col, got, col+" DESC")
		}
	}
}

func TestScanHelpers(t *testing.T) {
	// nullTimePtr
	if nullTimePtr(nil).Valid {
		t.Error("nullTimePtr(nil) should be invalid")
	}
	now := time.Now()
	if nt := nullTimePtr(&now); !nt.Valid || !nt.Time.Equal(now) {
		t.Errorf("nullTimePtr(now) = %v", nt)
	}

	// nullString
	if nullString("").Valid {
		t.Error("nullString(\"\") should be invalid")
	}
	if ns := nullString("hello"); !ns.Valid || ns.String != "hello" {
		t.Errorf("nullString(\"hello\") = %v", ns)
	}

	// jsonbBytes
	if jsonbBytes(nil) != nil {
		t.Error("jsonbBytes(nil) should be nil")
	}
	if jsonbBytes(json.RawMessage{}) != nil {
		t.Error("jsonbBytes({}) should be nil")
	}
	input := json.RawMessage(`{"key":"value"}`)
	if string(jsonbBytes(input)) != `{"key":"value"}` {
		t.Errorf("jsonbBytes = %s", jsonbBytes(input))
	}
}

func TestQueryCreateBead(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	bead := &model.Bead{
		ID: "kd-test1", Kind: model.KindIssue, Type: model.TypeTask,
		Title: "Test bead", Status: model.StatusOpen, CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectExec("INSERT INTO beads").
		WithArgs(
			"kd-test1", sqlmock.AnyArg(), "issue", "task", "Test bead", "", "",
			"open", 0, "", "", now, "", now,
			sqlmock.AnyArg(), "", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := queryCreateBead(context.Background(), db, bead); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQueryGetBead(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()

	rows := sqlmock.NewRows([]string{
		"id", "slug", "kind", "type", "title", "description", "notes",
		"status", "priority", "assignee", "owner", "created_at", "created_by", "updated_at",
		"closed_at", "closed_by", "due_at", "defer_until", "fields",
	}).AddRow(
		"kd-test1", nil, "issue", "task", "Test bead", nil, nil,
		"open", 0, nil, nil, now, nil, now, nil, nil, nil, nil, nil,
	)
	mock.ExpectQuery("SELECT .+ FROM beads WHERE id = \\$1").WithArgs("kd-test1").WillReturnRows(rows)
	mock.ExpectQuery("SELECT label FROM labels WHERE bead_id = \\$1").WithArgs("kd-test1").
		WillReturnRows(sqlmock.NewRows([]string{"label"}).AddRow("urgent"))
	mock.ExpectQuery("SELECT .+ FROM deps WHERE bead_id = \\$1").WithArgs("kd-test1").
		WillReturnRows(sqlmock.NewRows([]string{"bead_id", "depends_on_id", "type", "created_at", "created_by", "metadata"}))
	mock.ExpectQuery("SELECT .+ FROM comments WHERE bead_id = \\$1").WithArgs("kd-test1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "bead_id", "author", "text", "created_at"}))

	bead, err := queryGetBead(context.Background(), db, "kd-test1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bead.ID != "kd-test1" || bead.Title != "Test bead" {
		t.Fatalf("got id=%q title=%q", bead.ID, bead.Title)
	}
	if len(bead.Labels) != 1 || bead.Labels[0] != "urgent" {
		t.Fatalf("expected labels=[urgent], got %v", bead.Labels)
	}
}

func TestQueryGetBead_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT .+ FROM beads WHERE id = \\$1").WithArgs("nonexistent").WillReturnError(sql.ErrNoRows)

	_, err := queryGetBead(context.Background(), db, "nonexistent")
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestQueryDeleteBead(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM beads WHERE id = \\$1").WithArgs("kd-del1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := queryDeleteBead(context.Background(), db, "kd-del1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQueryDeleteBead_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM beads WHERE id = \\$1").WithArgs("nonexistent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := queryDeleteBead(context.Background(), db, "nonexistent"); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestQueryUpdateBead(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	bead := &model.Bead{
		ID: "kd-test1", Kind: model.KindIssue, Type: model.TypeTask,
		Title: "Updated bead", Status: model.StatusOpen, CreatedAt: now, UpdatedAt: now,
	}
	mock.ExpectQuery("UPDATE beads SET").
		WithArgs(
			"kd-test1", sqlmock.AnyArg(), "issue", "task", "Updated bead", "", "",
			"open", 0, "", "",
			sqlmock.AnyArg(), "", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"updated_at"}).AddRow(now))

	if err := queryUpdateBead(context.Background(), db, bead); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQueryUpdateBead_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	bead := &model.Bead{ID: "nonexistent", Kind: model.KindIssue, Type: model.TypeTask, Title: "Test", Status: model.StatusOpen}
	mock.ExpectQuery("UPDATE beads SET").
		WithArgs(
			"nonexistent", sqlmock.AnyArg(), "issue", "task", "Test", "", "",
			"open", 0, "", "",
			sqlmock.AnyArg(), "", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnError(sql.ErrNoRows)

	if err := queryUpdateBead(context.Background(), db, bead); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestQueryAddDependency(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	dep := &model.Dependency{
		BeadID: "kd-a", DependsOnID: "kd-b", Type: model.DepBlocks, CreatedAt: now, CreatedBy: "alice",
	}
	mock.ExpectExec("INSERT INTO deps").
		WithArgs("kd-a", "kd-b", "blocks", now, "alice", "").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := queryAddDependency(context.Background(), db, dep); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQueryGetDependencies(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{"bead_id", "depends_on_id", "type", "created_at", "created_by", "metadata"}).
		AddRow("kd-a", "kd-b", "blocks", now, nil, nil).
		AddRow("kd-a", "kd-c", "related", now, "alice", nil)
	mock.ExpectQuery("SELECT .+ FROM deps WHERE bead_id = \\$1").WithArgs("kd-a").WillReturnRows(rows)

	deps, err := queryGetDependencies(context.Background(), db, "kd-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(deps))
	}
	if deps[0].DependsOnID != "kd-b" || deps[1].CreatedBy != "alice" {
		t.Fatalf("got deps[0].DependsOnID=%q deps[1].CreatedBy=%q", deps[0].DependsOnID, deps[1].CreatedBy)
	}
}

func TestQueryRemoveDependency(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM deps").
		WithArgs("kd-a", "kd-b", "blocks").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := queryRemoveDependency(context.Background(), db, "kd-a", "kd-b", model.DepBlocks); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQueryAddLabel(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("INSERT INTO labels").WithArgs("kd-a", "urgent").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := queryAddLabel(context.Background(), db, "kd-a", "urgent"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQueryRemoveLabel(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM labels").WithArgs("kd-a", "urgent").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := queryRemoveLabel(context.Background(), db, "kd-a", "urgent"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQueryGetLabels(t *testing.T) {
	db, mock := newMockDB(t)
	rows := sqlmock.NewRows([]string{"label"}).AddRow("urgent").AddRow("frontend")
	mock.ExpectQuery("SELECT label FROM labels WHERE bead_id = \\$1").WithArgs("kd-a").WillReturnRows(rows)

	labels, err := queryGetLabels(context.Background(), db, "kd-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 2 || labels[0] != "urgent" || labels[1] != "frontend" {
		t.Fatalf("expected [urgent, frontend], got %v", labels)
	}
}

func TestQueryRecordEvent(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	event := &model.Event{
		Topic: "beads.bead.created", BeadID: "kd-a", Actor: "alice",
		Payload: json.RawMessage(`{"bead":{"id":"kd-a"}}`),
	}
	mock.ExpectQuery("INSERT INTO events").
		WithArgs("beads.bead.created", "kd-a", "alice", []byte(`{"bead":{"id":"kd-a"}}`)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(1, now))

	if err := queryRecordEvent(context.Background(), db, event); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.ID != 1 {
		t.Fatalf("expected id=1, got %d", event.ID)
	}
}

func TestQueryGetEvents(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{"id", "topic", "bead_id", "actor", "payload", "created_at"}).
		AddRow(1, "beads.bead.created", "kd-a", "alice", []byte(`{}`), now).
		AddRow(2, "beads.bead.updated", "kd-a", nil, []byte(`{}`), now)
	mock.ExpectQuery("SELECT .+ FROM events WHERE bead_id = \\$1").WithArgs("kd-a").WillReturnRows(rows)

	evts, err := queryGetEvents(context.Background(), db, "kd-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evts) != 2 {
		t.Fatalf("expected 2 events, got %d", len(evts))
	}
	if evts[0].Actor != "alice" || evts[1].Actor != "" {
		t.Fatalf("got actors=%q %q", evts[0].Actor, evts[1].Actor)
	}
}

func TestQuerySetConfig(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	config := &model.Config{Key: "view:inbox", Value: json.RawMessage(`{"filter":{"status":["open"]}}`)}
	mock.ExpectQuery("INSERT INTO configs").
		WithArgs("view:inbox", []byte(`{"filter":{"status":["open"]}}`)).
		WillReturnRows(sqlmock.NewRows([]string{"created_at", "updated_at"}).AddRow(now, now))

	if err := querySetConfig(context.Background(), db, config); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be set")
	}
}

func TestQueryGetConfig(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT .+ FROM configs WHERE key = \\$1").WithArgs("view:inbox").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "created_at", "updated_at"}).
			AddRow("view:inbox", []byte(`{}`), now, now))

	config, err := queryGetConfig(context.Background(), db, "view:inbox")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Key != "view:inbox" {
		t.Fatalf("got key=%q", config.Key)
	}
}

func TestQueryGetConfig_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT .+ FROM configs WHERE key = \\$1").WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	if _, err := queryGetConfig(context.Background(), db, "nonexistent"); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestQueryListConfigs(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT .+ FROM configs WHERE key LIKE").WithArgs("view").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "created_at", "updated_at"}).
			AddRow("view:inbox", []byte(`{}`), now, now).
			AddRow("view:my-tasks", []byte(`{}`), now, now))

	configs, err := queryListConfigs(context.Background(), db, "view")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}
}

func TestQueryListAllConfigs(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	mock.ExpectQuery("SELECT .+ FROM configs ORDER BY key").
		WillReturnRows(sqlmock.NewRows([]string{"key", "value", "created_at", "updated_at"}).
			AddRow("context:prime", []byte(`{}`), now, now).
			AddRow("view:inbox", []byte(`{}`), now, now))

	configs, err := queryListAllConfigs(context.Background(), db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}
	if configs[0].Key != "context:prime" || configs[1].Key != "view:inbox" {
		t.Fatalf("unexpected keys: %q, %q", configs[0].Key, configs[1].Key)
	}
}

func TestQueryDeleteConfig(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM configs WHERE key = \\$1").WithArgs("view:inbox").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := queryDeleteConfig(context.Background(), db, "view:inbox"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQueryDeleteConfig_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectExec("DELETE FROM configs WHERE key = \\$1").WithArgs("nonexistent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := queryDeleteConfig(context.Background(), db, "nonexistent"); err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestQueryAddComment(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	comment := &model.Comment{BeadID: "kd-a", Author: "alice", Text: "Hello world"}
	mock.ExpectQuery("INSERT INTO comments").
		WithArgs("kd-a", "alice", "Hello world").
		WillReturnRows(sqlmock.NewRows([]string{"id", "created_at"}).AddRow(int64(1), now))

	if err := queryAddComment(context.Background(), db, comment); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if comment.ID != 1 || comment.CreatedAt.IsZero() {
		t.Fatalf("got id=%d created_at=%v", comment.ID, comment.CreatedAt)
	}
}

func TestQueryGetComments(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	rows := sqlmock.NewRows([]string{"id", "bead_id", "author", "text", "created_at"}).
		AddRow(int64(1), "kd-a", "alice", "First", now).
		AddRow(int64(2), "kd-a", nil, "Second", now)
	mock.ExpectQuery("SELECT .+ FROM comments WHERE bead_id = \\$1").WithArgs("kd-a").WillReturnRows(rows)

	comments, err := queryGetComments(context.Background(), db, "kd-a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(comments))
	}
	if comments[0].Author != "alice" || comments[1].Author != "" {
		t.Fatalf("got authors=%q %q", comments[0].Author, comments[1].Author)
	}
}

func TestQueryListBeads(t *testing.T) {
	now := time.Now().UTC()
	pri := func(v int) *int { return &v }

	for _, tc := range []struct {
		name      string
		filter    model.BeadFilter
		queryPat  string
		args      []driver.Value
		wantCount int
		wantTotal int
	}{
		{
			name:      "NoFilter",
			filter:    model.BeadFilter{},
			queryPat:  "SELECT COUNT\\(\\*\\) OVER\\(\\) AS total_count, .+ FROM beads ORDER BY created_at DESC",
			wantCount: 2,
			wantTotal: 2,
		},
		{
			name:      "FilterByStatus",
			filter:    model.BeadFilter{Status: []model.Status{model.StatusOpen, model.StatusDeferred}},
			queryPat:  "SELECT .+ FROM beads WHERE status IN \\(\\$1, \\$2\\) ORDER BY",
			args:      []driver.Value{"open", "deferred"},
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name:      "FilterByType",
			filter:    model.BeadFilter{Type: []model.BeadType{model.TypeBug}},
			queryPat:  "SELECT .+ FROM beads WHERE type IN \\(\\$1\\) ORDER BY",
			args:      []driver.Value{"bug"},
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name:     "FilterByKind",
			filter:   model.BeadFilter{Kind: []model.Kind{model.KindData}},
			queryPat: "SELECT .+ FROM beads WHERE kind IN \\(\\$1\\) ORDER BY",
			args:     []driver.Value{"data"},
		},
		{
			name:      "FilterByPriority",
			filter:    model.BeadFilter{Priority: pri(3)},
			queryPat:  "SELECT .+ FROM beads WHERE priority = \\$1 ORDER BY",
			args:      []driver.Value{3},
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name:      "FilterByAssignee",
			filter:    model.BeadFilter{Assignee: "alice"},
			queryPat:  "SELECT .+ FROM beads WHERE assignee = \\$1 ORDER BY",
			args:      []driver.Value{"alice"},
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name:      "FilterByLabels",
			filter:    model.BeadFilter{Labels: []string{"urgent"}},
			queryPat:  "SELECT .+ FROM beads WHERE EXISTS \\(SELECT 1 FROM labels .+\\) ORDER BY",
			args:      []driver.Value{"urgent"},
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name:      "FilterBySearch",
			filter:    model.BeadFilter{Search: "login"},
			queryPat:  "SELECT .+ FROM beads WHERE \\(title ILIKE .+\\) ORDER BY",
			args:      []driver.Value{"login"},
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name:      "WithLimitAndOffset",
			filter:    model.BeadFilter{Limit: 10, Offset: 5},
			queryPat:  "SELECT .+ FROM beads ORDER BY .+ LIMIT \\$1 OFFSET \\$2",
			args:      []driver.Value{10, 5},
			wantCount: 1,
			wantTotal: 20,
		},
		{
			name:     "WithSort",
			filter:   model.BeadFilter{Sort: "-priority"},
			queryPat: "SELECT .+ FROM beads ORDER BY priority DESC",
		},
		{
			name:      "FilterByField",
			filter:    model.BeadFilter{Fields: map[string]string{"sprint": "3"}},
			queryPat:  "SELECT .+ FROM beads WHERE fields->>\\$1 = \\$2 ORDER BY",
			args:      []driver.Value{"sprint", "3"},
			wantCount: 1,
			wantTotal: 1,
		},
		{
			name:      "CombinedFilters",
			filter:    model.BeadFilter{Status: []model.Status{model.StatusOpen}, Assignee: "bob", Limit: 5},
			queryPat:  "SELECT .+ FROM beads WHERE status IN \\(\\$1\\) AND assignee = \\$2 ORDER BY .+ LIMIT \\$3",
			args:      []driver.Value{"open", "bob", 5},
			wantCount: 1,
			wantTotal: 3,
		},
		{
			name:      "FilterNoOpenDeps",
			filter:    model.BeadFilter{NoOpenDeps: true},
			queryPat:  "SELECT .+ FROM beads WHERE NOT EXISTS .+deps.+dep\\.status IN.+ ORDER BY",
			wantCount: 1,
			wantTotal: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock := newMockDB(t)
			eq := mock.ExpectQuery(tc.queryPat)
			if len(tc.args) > 0 {
				eq.WithArgs(tc.args...)
			}
			r := sqlmock.NewRows(beadWithTotalColumns)
			for i := range tc.wantCount {
				addBeadWithTotalRow(r, tc.wantTotal, fmt.Sprintf("kd-%d", i+1), "issue", "task", "T", "open", 0, now)
			}
			eq.WillReturnRows(r)

			// queryListBeads now fetches labels in a second query for non-empty results.
			if tc.wantCount > 0 {
				labelArgs := make([]driver.Value, tc.wantCount)
				for i := range tc.wantCount {
					labelArgs[i] = fmt.Sprintf("kd-%d", i+1)
				}
				mock.ExpectQuery("SELECT bead_id, label FROM labels WHERE bead_id IN").
					WithArgs(labelArgs...).
					WillReturnRows(sqlmock.NewRows([]string{"bead_id", "label"}))
			}

			beads, total, err := queryListBeads(context.Background(), db, tc.filter)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(beads) != tc.wantCount {
				t.Fatalf("expected %d beads, got %d", tc.wantCount, len(beads))
			}
			if total != tc.wantTotal {
				t.Fatalf("expected total=%d, got %d", tc.wantTotal, total)
			}
		})
	}
}

func TestQueryCloseBead(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()

	rows := sqlmock.NewRows(beadRowColumns)
	rows.AddRow(
		"kd-cls1", nil, "issue", "task", "Close me", nil, nil,
		"closed", 0, nil, nil, now, nil, now,
		now, "alice", nil, nil, nil,
	)
	mock.ExpectQuery("UPDATE beads SET").WithArgs("kd-cls1", "alice").WillReturnRows(rows)
	emptyRelationalExpectations(mock, "kd-cls1")

	bead, err := queryCloseBead(context.Background(), db, "kd-cls1", "alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bead.ID != "kd-cls1" || bead.Status != "closed" || bead.ClosedBy != "alice" {
		t.Fatalf("got id=%q status=%q closed_by=%q", bead.ID, bead.Status, bead.ClosedBy)
	}
	if bead.ClosedAt == nil {
		t.Fatal("expected closed_at to be set")
	}
}

func TestQueryCloseBead_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("UPDATE beads SET").WithArgs("nonexistent", "").WillReturnError(sql.ErrNoRows)

	_, err := queryCloseBead(context.Background(), db, "nonexistent", "")
	if err != sql.ErrNoRows {
		t.Fatalf("expected sql.ErrNoRows, got %v", err)
	}
}

func TestQueryCloseBead_WithRelationalData(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()

	rows := sqlmock.NewRows(beadRowColumns)
	rows.AddRow(
		"kd-cls2", nil, "issue", "task", "Close me", nil, nil,
		"closed", 0, nil, nil, now, nil, now,
		now, "bob", nil, nil, nil,
	)
	mock.ExpectQuery("UPDATE beads SET").WithArgs("kd-cls2", "bob").WillReturnRows(rows)

	// Labels
	mock.ExpectQuery("SELECT label FROM labels WHERE bead_id = \\$1").WithArgs("kd-cls2").
		WillReturnRows(sqlmock.NewRows([]string{"label"}).AddRow("urgent").AddRow("backend"))
	// Dependencies
	mock.ExpectQuery("SELECT .+ FROM deps WHERE bead_id = \\$1").WithArgs("kd-cls2").
		WillReturnRows(sqlmock.NewRows([]string{"bead_id", "depends_on_id", "type", "created_at", "created_by", "metadata"}).
			AddRow("kd-cls2", "kd-other", "blocks", now, nil, nil))
	// Comments
	mock.ExpectQuery("SELECT .+ FROM comments WHERE bead_id = \\$1").WithArgs("kd-cls2").
		WillReturnRows(sqlmock.NewRows([]string{"id", "bead_id", "author", "text", "created_at"}).
			AddRow(int64(1), "kd-cls2", "alice", "Done!", now))

	bead, err := queryCloseBead(context.Background(), db, "kd-cls2", "bob")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bead.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(bead.Labels))
	}
	if len(bead.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(bead.Dependencies))
	}
	if len(bead.Comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(bead.Comments))
	}
}

func TestScanBead_WithOptionalFields(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()
	closedAt := now.Add(-time.Hour)
	dueAt := now.Add(24 * time.Hour)

	rows := sqlmock.NewRows([]string{
		"id", "slug", "kind", "type", "title", "description", "notes",
		"status", "priority", "assignee", "owner", "created_at", "created_by", "updated_at",
		"closed_at", "closed_by", "due_at", "defer_until", "fields",
	}).AddRow(
		"kd-full", "test-slug", "issue", "task", "Full bead", "A description", "Some notes",
		"closed", 2, "bob", "alice", now, "carol", now,
		closedAt, "dave", dueAt, nil, []byte(`{"foo":"bar"}`),
	)
	mock.ExpectQuery("SELECT").WillReturnRows(rows)

	bead, err := scanBead(db.QueryRow("SELECT"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bead.Slug != "test-slug" || bead.Description != "A description" {
		t.Fatalf("got slug=%q description=%q", bead.Slug, bead.Description)
	}
	if bead.Notes != "Some notes" {
		t.Fatalf("got notes=%q", bead.Notes)
	}
	if bead.Priority != 2 || bead.Assignee != "bob" || bead.Owner != "alice" || bead.CreatedBy != "carol" {
		t.Fatalf("got priority=%d assignee=%q owner=%q created_by=%q", bead.Priority, bead.Assignee, bead.Owner, bead.CreatedBy)
	}
	if bead.ClosedAt == nil || bead.DueAt == nil || bead.DeferUntil != nil {
		t.Fatalf("got closed_at=%v due_at=%v defer_until=%v", bead.ClosedAt, bead.DueAt, bead.DeferUntil)
	}
	if string(bead.Fields) != `{"foo":"bar"}` {
		t.Fatalf("got fields=%s", bead.Fields)
	}
}

func TestQueryGetDependenciesForBeads(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()

	depRows := sqlmock.NewRows([]string{"bead_id", "depends_on_id", "type", "created_at", "created_by", "metadata"}).
		AddRow("kd-b1", "kd-b2", "blocks", now, nil, nil).
		AddRow("kd-b1", "kd-b3", "related", now, nil, nil)
	mock.ExpectQuery("SELECT .+ FROM deps WHERE bead_id = ANY").
		WillReturnRows(depRows)

	result, err := queryGetDependenciesForBeads(context.Background(), db, []string{"kd-b1", "kd-b2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result["kd-b1"]) != 2 {
		t.Fatalf("expected 2 deps for kd-b1, got %d", len(result["kd-b1"]))
	}
}

func TestQueryGetDependencyCounts(t *testing.T) {
	db, mock := newMockDB(t)

	fwdRows := sqlmock.NewRows([]string{"bead_id", "count"}).
		AddRow("kd-c1", 3)
	mock.ExpectQuery("SELECT bead_id, COUNT\\(\\*\\) FROM deps WHERE bead_id = ANY").
		WillReturnRows(fwdRows)

	revRows := sqlmock.NewRows([]string{"depends_on_id", "count"}).
		AddRow("kd-c1", 1)
	mock.ExpectQuery("SELECT depends_on_id, COUNT\\(\\*\\) FROM deps WHERE depends_on_id = ANY").
		WillReturnRows(revRows)

	result, err := queryGetDependencyCounts(context.Background(), db, []string{"kd-c1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["kd-c1"].DependencyCount != 3 {
		t.Fatalf("expected dep count 3, got %d", result["kd-c1"].DependencyCount)
	}
	if result["kd-c1"].DependentCount != 1 {
		t.Fatalf("expected dep-by count 1, got %d", result["kd-c1"].DependentCount)
	}
}

func TestQueryGetLabelsForBeads(t *testing.T) {
	db, mock := newMockDB(t)

	labelRows := sqlmock.NewRows([]string{"bead_id", "label"}).
		AddRow("kd-l1", "urgent").
		AddRow("kd-l1", "frontend").
		AddRow("kd-l2", "backend")
	mock.ExpectQuery("SELECT bead_id, label FROM labels WHERE bead_id = ANY").
		WillReturnRows(labelRows)

	result, err := queryGetLabelsForBeads(context.Background(), db, []string{"kd-l1", "kd-l2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result["kd-l1"]) != 2 {
		t.Fatalf("expected 2 labels for kd-l1, got %d", len(result["kd-l1"]))
	}
	if len(result["kd-l2"]) != 1 {
		t.Fatalf("expected 1 label for kd-l2, got %d", len(result["kd-l2"]))
	}
}

func TestQueryGetBlockedByForBeads(t *testing.T) {
	db, mock := newMockDB(t)

	blockedRows := sqlmock.NewRows([]string{"bead_id", "depends_on_id"}).
		AddRow("kd-blocked1", "kd-blocker1").
		AddRow("kd-blocked1", "kd-blocker2")
	mock.ExpectQuery("SELECT d.bead_id, d.depends_on_id FROM deps").
		WillReturnRows(blockedRows)

	result, err := queryGetBlockedByForBeads(context.Background(), db, []string{"kd-blocked1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result["kd-blocked1"]) != 2 {
		t.Fatalf("expected 2 blockers, got %d", len(result["kd-blocked1"]))
	}
}

func TestQueryGetBeadsByIDs(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()

	beadRows := sqlmock.NewRows(beadRowColumns).
		AddRow("kd-id1", nil, "issue", "task", "Bead 1", nil, nil, "open", 0, nil, nil, now, nil, now, nil, nil, nil, nil, nil)
	mock.ExpectQuery("SELECT .+ FROM beads WHERE id = ANY").
		WillReturnRows(beadRows)

	result, err := queryGetBeadsByIDs(context.Background(), db, []string{"kd-id1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || result[0].ID != "kd-id1" {
		t.Fatalf("expected 1 bead with id=kd-id1, got %v", result)
	}
}

func TestQueryGetDependenciesForBeads_Empty(t *testing.T) {
	result, err := queryGetDependenciesForBeads(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(result))
	}
}

func TestQueryGetStats(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	mock.ExpectQuery(`SELECT`).WillReturnRows(
		sqlmock.NewRows([]string{"open", "in_progress", "blocked", "closed", "deferred"}).
			AddRow(5, 3, 2, 10, 1),
	)

	stats, err := queryGetStats(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalOpen != 5 {
		t.Fatalf("expected total_open=5, got %d", stats.TotalOpen)
	}
	if stats.TotalInProgress != 3 {
		t.Fatalf("expected total_in_progress=3, got %d", stats.TotalInProgress)
	}
	if stats.TotalBlocked != 2 {
		t.Fatalf("expected total_blocked=2, got %d", stats.TotalBlocked)
	}
	if stats.TotalClosed != 10 {
		t.Fatalf("expected total_closed=10, got %d", stats.TotalClosed)
	}
	if stats.TotalDeferred != 1 {
		t.Fatalf("expected total_deferred=1, got %d", stats.TotalDeferred)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
