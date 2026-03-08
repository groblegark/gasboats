package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/groblegark/kbeads/internal/model"
)

func TestBurnCmd_BasicForce(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-mol-1"] = &model.Bead{
		ID: "kd-mol-1", Title: "Test mol", Type: "molecule", Status: model.StatusOpen,
	}
	mc.Beads["kd-child-1"] = &model.Bead{ID: "kd-child-1", Type: "task"}
	mc.Beads["kd-child-2"] = &model.Bead{ID: "kd-child-2", Type: "task"}
	mc.RevDeps["kd-mol-1"] = []*model.Dependency{
		{BeadID: "kd-child-1", DependsOnID: "kd-mol-1", Type: "parent-child"},
		{BeadID: "kd-child-2", DependsOnID: "kd-mol-1", Type: "parent-child"},
		{BeadID: "kd-other", DependsOnID: "kd-mol-1", Type: "related"}, // Not a child.
	}
	withMockClient(t, mc)

	cmd := newBurnCmd()
	cmd.SetArgs([]string{"kd-mol-1", "--force"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("burn: %v", err)
		}
	})

	// Should delete 2 children + molecule.
	if got := len(mc.DeleteBeadCalls); got != 3 {
		t.Fatalf("expected 3 DeleteBead calls, got %d", got)
	}
	if mc.DeleteBeadCalls[0] != "kd-child-1" {
		t.Errorf("first delete = %q, want kd-child-1", mc.DeleteBeadCalls[0])
	}
	if mc.DeleteBeadCalls[1] != "kd-child-2" {
		t.Errorf("second delete = %q, want kd-child-2", mc.DeleteBeadCalls[1])
	}
	if mc.DeleteBeadCalls[2] != "kd-mol-1" {
		t.Errorf("third delete = %q, want kd-mol-1", mc.DeleteBeadCalls[2])
	}

	if !strings.Contains(out, "Burned molecule kd-mol-1") {
		t.Errorf("output missing confirmation, got:\n%s", out)
	}
	if !strings.Contains(out, "2 children deleted") {
		t.Errorf("output missing child count, got:\n%s", out)
	}
}

func TestBurnCmd_DryRun(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-mol-1"] = &model.Bead{
		ID: "kd-mol-1", Title: "Dry mol", Type: "molecule",
	}
	mc.RevDeps["kd-mol-1"] = []*model.Dependency{
		{BeadID: "kd-child-1", DependsOnID: "kd-mol-1", Type: "parent-child"},
	}
	withMockClient(t, mc)

	cmd := newBurnCmd()
	cmd.SetArgs([]string{"kd-mol-1", "--dry-run"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("dry-run: %v", err)
		}
	})

	// Should not delete anything.
	if len(mc.DeleteBeadCalls) != 0 {
		t.Errorf("dry-run deleted %d beads, want 0", len(mc.DeleteBeadCalls))
	}
	if !strings.Contains(out, "Would burn molecule kd-mol-1") {
		t.Errorf("output missing dry-run preview, got:\n%s", out)
	}
	if !strings.Contains(out, "kd-child-1") {
		t.Errorf("output missing child ID, got:\n%s", out)
	}
}

func TestBurnCmd_RequiresForce(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-mol-1"] = &model.Bead{
		ID: "kd-mol-1", Title: "Protected mol", Type: "molecule",
	}
	mc.RevDeps["kd-mol-1"] = []*model.Dependency{
		{BeadID: "kd-child-1", DependsOnID: "kd-mol-1", Type: "parent-child"},
	}
	withMockClient(t, mc)

	cmd := newBurnCmd()
	cmd.SetArgs([]string{"kd-mol-1"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("burn without force: %v", err)
		}
	})

	// Should not delete anything without --force.
	if len(mc.DeleteBeadCalls) != 0 {
		t.Errorf("burn without --force deleted %d beads, want 0", len(mc.DeleteBeadCalls))
	}
	if !strings.Contains(out, "--force") {
		t.Errorf("output should mention --force, got:\n%s", out)
	}
}

func TestBurnCmd_NotMolecule(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-task"] = &model.Bead{ID: "kd-task", Type: "task"}
	withMockClient(t, mc)

	cmd := newBurnCmd()
	cmd.SetArgs([]string{"kd-task"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for non-molecule")
	}
	if !strings.Contains(err.Error(), "not molecule") {
		t.Errorf("error = %q, want 'not molecule'", err)
	}
}


func TestBurnCmd_ChildDeleteError(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-mol-1"] = &model.Bead{
		ID: "kd-mol-1", Title: "Fail mol", Type: "molecule",
	}
	mc.RevDeps["kd-mol-1"] = []*model.Dependency{
		{BeadID: "kd-child-1", DependsOnID: "kd-mol-1", Type: "parent-child"},
		{BeadID: "kd-child-2", DependsOnID: "kd-mol-1", Type: "parent-child"},
	}
	mc.DeleteErrs["kd-child-1"] = fmt.Errorf("permission denied")
	withMockClient(t, mc)

	cmd := newBurnCmd()
	cmd.SetArgs([]string{"kd-mol-1", "--force"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("burn with partial failure: %v", err)
		}
	})

	// Should still attempt to delete child-2 and the molecule despite child-1 failure.
	if !strings.Contains(out, "warning") {
		t.Errorf("output should warn about child-1 failure, got:\n%s", out)
	}
	if len(mc.DeleteBeadCalls) != 3 {
		t.Errorf("expected 3 delete calls (even with failure), got %d", len(mc.DeleteBeadCalls))
	}
}

func TestBurnCmd_NoChildren(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-mol-1"] = &model.Bead{
		ID: "kd-mol-1", Title: "Empty mol", Type: "molecule",
	}
	mc.RevDeps["kd-mol-1"] = []*model.Dependency{}
	withMockClient(t, mc)

	// No children means no --force required.
	cmd := newBurnCmd()
	cmd.SetArgs([]string{"kd-mol-1"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("burn empty mol: %v", err)
		}
	})

	if len(mc.DeleteBeadCalls) != 1 {
		t.Errorf("expected 1 delete call (just molecule), got %d", len(mc.DeleteBeadCalls))
	}
	if !strings.Contains(out, "0 children deleted") {
		t.Errorf("output = %q, want '0 children deleted'", out)
	}
}
