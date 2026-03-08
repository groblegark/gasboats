package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/groblegark/kbeads/internal/model"
)

var depColumns = []string{"bead_id", "depends_on_id", "type", "created_at", "created_by", "metadata"}

func TestQueryGetReverseDependenciesForBeadsEmpty(t *testing.T) {
	result, err := queryGetReverseDependenciesForBeads(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestQueryGetReverseDependenciesForBeads(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()

	rows := sqlmock.NewRows(depColumns).
		AddRow("child-1", "parent-1", "blocks", now, nil, nil).
		AddRow("child-2", "parent-1", "blocks", now, nil, nil)

	mock.ExpectQuery("SELECT .+ FROM deps WHERE depends_on_id").
		WillReturnRows(rows)

	result, err := queryGetReverseDependenciesForBeads(context.Background(), db, []string{"parent-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	deps := result["parent-1"]
	if len(deps) != 2 {
		t.Fatalf("expected 2 reverse deps for parent-1, got %d", len(deps))
	}
	if deps[0].BeadID != "child-1" {
		t.Errorf("dep[0].BeadID = %q, want child-1", deps[0].BeadID)
	}
}

func TestQueryGetReverseDependenciesForBeadsQueryError(t *testing.T) {
	db, mock := newMockDB(t)
	mock.ExpectQuery("SELECT .+ FROM deps WHERE depends_on_id").
		WillReturnError(sql.ErrConnDone)

	_, err := queryGetReverseDependenciesForBeads(context.Background(), db, []string{"parent-1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestQueryGetDependenciesForBeadsEmptyInput(t *testing.T) {
	result, err := queryGetDependenciesForBeads(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestQueryGetDependenciesForBeadsHappyPath(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()

	rows := sqlmock.NewRows(depColumns).
		AddRow("child-1", "parent-1", "blocks", now, "user1", nil)

	mock.ExpectQuery("SELECT .+ FROM deps WHERE bead_id").
		WillReturnRows(rows)

	result, err := queryGetDependenciesForBeads(context.Background(), db, []string{"child-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result["child-1"]) != 1 {
		t.Errorf("expected 1 dep for child-1, got %d", len(result["child-1"]))
	}
}

func TestAddExcludeTypesClauseEmpty(t *testing.T) {
	clauses, args := addExcludeTypesClause(nil, nil, new(int), nil)
	if len(clauses) != 0 {
		t.Errorf("expected no clauses, got %d", len(clauses))
	}
	if len(args) != 0 {
		t.Errorf("expected no args, got %d", len(args))
	}
}

func TestAddExcludeTypesClause(t *testing.T) {
	argIdx := 2
	clauses := []string{"status = $1"}
	args := []any{"open"}

	clauses, args = addExcludeTypesClause(clauses, args, &argIdx, []model.BeadType{"epic", "chore"})

	if len(clauses) != 2 {
		t.Fatalf("expected 2 clauses, got %d", len(clauses))
	}
	if clauses[1] != "type NOT IN ($3, $4)" {
		t.Errorf("clause = %q, want 'type NOT IN ($3, $4)'", clauses[1])
	}
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(args))
	}
	if args[1] != "epic" || args[2] != "chore" {
		t.Errorf("args = %v, want [open epic chore]", args)
	}
	if argIdx != 4 {
		t.Errorf("argIdx = %d, want 4", argIdx)
	}
}

func TestQueryGetDependencyCountsEmptyInput(t *testing.T) {
	result, err := queryGetDependencyCounts(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestQueryGetDependencyCountsForwardAndReverse(t *testing.T) {
	db, mock := newMockDB(t)

	// Forward counts.
	fwdRows := sqlmock.NewRows([]string{"bead_id", "count"}).
		AddRow("bead-1", 3)
	mock.ExpectQuery("SELECT bead_id, COUNT.*FROM deps.*WHERE bead_id").
		WillReturnRows(fwdRows)

	// Reverse counts — bead-1 already exists in result, bead-2 is new.
	revRows := sqlmock.NewRows([]string{"depends_on_id", "count"}).
		AddRow("bead-1", 2).
		AddRow("bead-2", 5)
	mock.ExpectQuery("SELECT depends_on_id, COUNT.*FROM deps.*WHERE depends_on_id").
		WillReturnRows(revRows)

	result, err := queryGetDependencyCounts(context.Background(), db, []string{"bead-1", "bead-2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dc := result["bead-1"]; dc == nil {
		t.Fatal("expected counts for bead-1")
	} else {
		if dc.DependencyCount != 3 {
			t.Errorf("bead-1 DependencyCount = %d, want 3", dc.DependencyCount)
		}
		if dc.DependentCount != 2 {
			t.Errorf("bead-1 DependentCount = %d, want 2", dc.DependentCount)
		}
	}

	// bead-2 only has reverse deps.
	if dc := result["bead-2"]; dc == nil {
		t.Fatal("expected counts for bead-2")
	} else {
		if dc.DependencyCount != 0 {
			t.Errorf("bead-2 DependencyCount = %d, want 0", dc.DependencyCount)
		}
		if dc.DependentCount != 5 {
			t.Errorf("bead-2 DependentCount = %d, want 5", dc.DependentCount)
		}
	}
}

func TestQueryGetLabelsForBeadsEmpty(t *testing.T) {
	result, err := queryGetLabelsForBeads(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestQueryGetBlockedByForBeadsEmpty(t *testing.T) {
	result, err := queryGetBlockedByForBeads(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

func TestQueryGetBeadsByIDsEmptyInput(t *testing.T) {
	beads, err := queryGetBeadsByIDs(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(beads) != 0 {
		t.Errorf("expected empty slice, got %d beads", len(beads))
	}
}

func TestQueryGetBeadsByIDsHappyPath(t *testing.T) {
	db, mock := newMockDB(t)
	now := time.Now().UTC()

	rows := sqlmock.NewRows(beadRowColumns).AddRow(
		"kd-abc", nil, "issue", "task", "test bead", nil, nil,
		"open", 2, nil, nil, now, nil, now,
		nil, nil, nil, nil, nil,
	)
	mock.ExpectQuery("SELECT .+ FROM beads WHERE id").
		WillReturnRows(rows)

	beads, err := queryGetBeadsByIDs(context.Background(), db, []string{"kd-abc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(beads) != 1 {
		t.Fatalf("expected 1 bead, got %d", len(beads))
	}
	if beads[0].ID != "kd-abc" {
		t.Errorf("bead ID = %q, want kd-abc", beads[0].ID)
	}
}
