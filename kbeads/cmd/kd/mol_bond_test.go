package main

import (
	"strings"
	"testing"
)

func TestBondCmd_Sequential(t *testing.T) {
	mc := newMockClient()
	withMockClient(t, mc)

	cmd := newBondCmd()
	cmd.SetArgs([]string{"kd-mol-a", "kd-mol-b"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("bond: %v", err)
		}
	})

	if len(mc.AddDepCalls) != 1 {
		t.Fatalf("expected 1 AddDependency call, got %d", len(mc.AddDepCalls))
	}
	dep := mc.AddDepCalls[0]
	if dep.BeadID != "kd-mol-b" {
		t.Errorf("dep.BeadID = %q, want kd-mol-b", dep.BeadID)
	}
	if dep.DependsOnID != "kd-mol-a" {
		t.Errorf("dep.DependsOnID = %q, want kd-mol-a", dep.DependsOnID)
	}
	if dep.Type != "blocks" {
		t.Errorf("dep.Type = %q, want blocks (sequential default)", dep.Type)
	}
	if dep.CreatedBy != "test-actor" {
		t.Errorf("dep.CreatedBy = %q, want test-actor", dep.CreatedBy)
	}

	if !strings.Contains(out, "Bonded kd-mol-a") {
		t.Errorf("output missing bond confirmation, got:\n%s", out)
	}
}

func TestBondCmd_SequentialExplicit(t *testing.T) {
	mc := newMockClient()
	withMockClient(t, mc)

	cmd := newBondCmd()
	cmd.SetArgs([]string{"kd-a", "kd-b", "--type", "sequential"})
	captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("bond sequential: %v", err)
		}
	})

	if mc.AddDepCalls[0].Type != "blocks" {
		t.Errorf("explicit sequential type = %q, want blocks", mc.AddDepCalls[0].Type)
	}
}

func TestBondCmd_Parallel(t *testing.T) {
	mc := newMockClient()
	withMockClient(t, mc)

	cmd := newBondCmd()
	cmd.SetArgs([]string{"kd-a", "kd-b", "--type", "parallel"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("bond parallel: %v", err)
		}
	})

	dep := mc.AddDepCalls[0]
	if dep.Type != "related" {
		t.Errorf("parallel type = %q, want related", dep.Type)
	}
	if !strings.Contains(out, "related") {
		t.Errorf("output should mention 'related', got:\n%s", out)
	}
}

func TestBondCmd_UnknownType(t *testing.T) {
	mc := newMockClient()
	withMockClient(t, mc)

	cmd := newBondCmd()
	cmd.SetArgs([]string{"kd-a", "kd-b", "--type", "invalid"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown bond type")
	}
	if !strings.Contains(err.Error(), "unknown bond type") {
		t.Errorf("error = %q, want 'unknown bond type'", err)
	}
}

func TestBondCmd_Direction(t *testing.T) {
	mc := newMockClient()
	withMockClient(t, mc)

	// bond kd-first kd-second: kd-second depends on kd-first.
	cmd := newBondCmd()
	cmd.SetArgs([]string{"kd-first", "kd-second"})
	captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("bond: %v", err)
		}
	})

	dep := mc.AddDepCalls[0]
	// B (second arg) is the bead that has the dependency.
	if dep.BeadID != "kd-second" {
		t.Errorf("dep.BeadID = %q, want kd-second", dep.BeadID)
	}
	// A (first arg) is the one it depends on.
	if dep.DependsOnID != "kd-first" {
		t.Errorf("dep.DependsOnID = %q, want kd-first", dep.DependsOnID)
	}
}
