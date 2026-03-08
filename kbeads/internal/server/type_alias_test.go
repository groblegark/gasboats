package server

import (
	"encoding/json"
	"testing"

	beadsv1 "github.com/groblegark/kbeads/gen/beads/v1"
	"github.com/groblegark/kbeads/internal/model"
)

func TestCreateBead_BundleAlias(t *testing.T) {
	srv, ms, ctx := testCtx(t)

	// Creating a bead with deprecated type "bundle" should resolve to "molecule".
	resp, err := srv.CreateBead(ctx, &beadsv1.CreateBeadRequest{
		Title: "My bundle", Type: "bundle", CreatedBy: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b := resp.Bead
	if b.Type != "molecule" {
		t.Errorf("type = %q, want 'molecule' (alias resolved)", b.Type)
	}
	if b.Kind != "issue" {
		t.Errorf("kind = %q, want 'issue' (molecule kind)", b.Kind)
	}

	// Verify stored with resolved type.
	stored := ms.beads[b.Id]
	if string(stored.Type) != "molecule" {
		t.Errorf("stored type = %q, want molecule", stored.Type)
	}
}

func TestCreateBead_TemplateAlias(t *testing.T) {
	srv, _, ctx := testCtx(t)

	// Creating a bead with deprecated type "template" should resolve to "formula".
	resp, err := srv.CreateBead(ctx, &beadsv1.CreateBeadRequest{
		Title: "My template", Type: "template", CreatedBy: "test",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b := resp.Bead
	if b.Type != "formula" {
		t.Errorf("type = %q, want 'formula' (alias resolved)", b.Type)
	}
	if b.Kind != "data" {
		t.Errorf("kind = %q, want 'data' (formula kind)", b.Kind)
	}
}

func TestCreateBead_MoleculeType(t *testing.T) {
	srv, _, ctx := testCtx(t)

	// Creating a bead with canonical type "molecule" should work directly.
	resp, err := srv.CreateBead(ctx, &beadsv1.CreateBeadRequest{
		Title:  "My molecule",
		Type:   "molecule",
		Fields: []byte(`{"formula_id":"kd-f-1","applied_vars":{"env":"prod"}}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b := resp.Bead
	if b.Type != "molecule" {
		t.Errorf("type = %q, want molecule", b.Type)
	}
	if b.Kind != "issue" {
		t.Errorf("kind = %q, want issue", b.Kind)
	}

	// Verify fields were stored.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b.Fields, &fields); err != nil {
		t.Fatalf("unmarshal fields: %v", err)
	}
	if string(fields["formula_id"]) != `"kd-f-1"` {
		t.Errorf("formula_id = %s, want \"kd-f-1\"", fields["formula_id"])
	}
}

func TestCreateBead_FormulaType(t *testing.T) {
	srv, _, ctx := testCtx(t)

	resp, err := srv.CreateBead(ctx, &beadsv1.CreateBeadRequest{
		Title:  "Deploy workflow",
		Type:   "formula",
		Fields: []byte(`{"vars":[{"name":"env"}],"steps":[{"id":"s1","title":"Build"}]}`),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b := resp.Bead
	if b.Type != "formula" {
		t.Errorf("type = %q, want formula", b.Type)
	}
	if b.Kind != "data" {
		t.Errorf("kind = %q, want data", b.Kind)
	}
}

func TestListBeads_TypeAliasInFilter(t *testing.T) {
	srv, ms, ctx := testCtx(t)

	// Store some molecule-type beads.
	ms.beads["kd-mol-1"] = &model.Bead{
		ID: "kd-mol-1", Title: "Mol 1", Type: "molecule", Kind: "issue", Status: "open",
	}
	ms.beads["kd-task-1"] = &model.Bead{
		ID: "kd-task-1", Title: "Task 1", Type: "task", Kind: "issue", Status: "open",
	}

	// Filtering by deprecated type "bundle" should resolve to "molecule".
	resp, err := srv.ListBeads(ctx, &beadsv1.ListBeadsRequest{
		Type: []string{"bundle"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should find the molecule bead (alias resolved).
	found := false
	for _, b := range resp.Beads {
		if b.Id == "kd-mol-1" {
			found = true
		}
		if b.Id == "kd-task-1" {
			t.Error("task bead should not be in molecule/bundle results")
		}
	}
	if !found {
		t.Error("molecule bead kd-mol-1 should be found when filtering by 'bundle' alias")
	}
}

func TestListBeads_TemplateAliasFilter(t *testing.T) {
	srv, ms, ctx := testCtx(t)

	ms.beads["kd-f-1"] = &model.Bead{
		ID: "kd-f-1", Title: "Formula 1", Type: "formula", Kind: "data", Status: "open",
	}

	resp, err := srv.ListBeads(ctx, &beadsv1.ListBeadsRequest{
		Type: []string{"template"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, b := range resp.Beads {
		if b.Id == "kd-f-1" {
			found = true
		}
	}
	if !found {
		t.Error("formula bead kd-f-1 should be found when filtering by 'template' alias")
	}
}
