package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/groblegark/kbeads/internal/model"
)

// --- formula dump tests ---

func TestFormulaDump_Stdout(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-f1"] = &model.Bead{
		ID:   "kd-f1",
		Type: "formula",
		Fields: makeFormulaFields(
			[]FormulaVarDef{{Name: "version", Required: true}},
			[]FormulaStep{
				{ID: "build", Title: "Build {{version}}", Type: "task"},
				{ID: "deploy", Title: "Deploy {{version}}", Type: "task", DependsOn: []string{"build"}},
			},
		),
	}
	withMockClient(t, mc)

	out := captureStdout(t, func() {
		if err := runFormulaDump("kd-f1", ""); err != nil {
			t.Fatalf("dump: %v", err)
		}
	})

	var parsed struct {
		Vars  []FormulaVarDef `json:"vars"`
		Steps []FormulaStep   `json:"steps"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, out)
	}
	if len(parsed.Vars) != 1 {
		t.Errorf("vars count = %d, want 1", len(parsed.Vars))
	}
	if len(parsed.Steps) != 2 {
		t.Errorf("steps count = %d, want 2", len(parsed.Steps))
	}
	if parsed.Steps[0].ID != "build" {
		t.Errorf("first step id = %q, want build", parsed.Steps[0].ID)
	}
}

func TestFormulaDump_ToFile(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-f2"] = &model.Bead{
		ID:   "kd-f2",
		Type: "formula",
		Fields: makeFormulaFields(nil, []FormulaStep{
			{ID: "s1", Title: "Step 1"},
		}),
	}
	withMockClient(t, mc)

	outFile := filepath.Join(t.TempDir(), "out.json")
	if err := runFormulaDump("kd-f2", outFile); err != nil {
		t.Fatalf("dump --file: %v", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("reading output file: %v", err)
	}
	var parsed struct {
		Steps []FormulaStep `json:"steps"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("output file is not valid JSON: %v", err)
	}
	if len(parsed.Steps) != 1 {
		t.Errorf("steps count = %d, want 1", len(parsed.Steps))
	}
}

func TestFormulaDump_NotFormula(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-task"] = &model.Bead{ID: "kd-task", Type: "task"}
	withMockClient(t, mc)

	err := runFormulaDump("kd-task", "")
	if err == nil {
		t.Fatal("expected error for non-formula bead")
	}
	if !strings.Contains(err.Error(), "not formula") {
		t.Errorf("error = %q, want mention of 'not formula'", err)
	}
}

func TestFormulaDump_EmptyFields(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-empty"] = &model.Bead{ID: "kd-empty", Type: "formula"}
	withMockClient(t, mc)

	err := runFormulaDump("kd-empty", "")
	if err == nil {
		t.Fatal("expected error for empty fields")
	}
	if !strings.Contains(err.Error(), "no fields") {
		t.Errorf("error = %q, want 'no fields'", err)
	}
}


func TestFormulaDump_GetError(t *testing.T) {
	mc := newMockClient()
	mc.GetErr = fmt.Errorf("connection refused")
	withMockClient(t, mc)

	err := runFormulaDump("kd-missing", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "getting formula") {
		t.Errorf("error = %q, want 'getting formula'", err)
	}
}

// --- formula update tests ---

func TestFormulaUpdate_Basic(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-f1"] = &model.Bead{
		ID:   "kd-f1",
		Type: "formula",
		Fields: makeFormulaFields(nil, []FormulaStep{
			{ID: "old", Title: "Old step"},
		}),
	}
	withMockClient(t, mc)

	newFormula := `{
		"vars": [{"name": "env", "required": true}],
		"steps": [
			{"id": "build", "title": "Build for {{env}}", "type": "task"},
			{"id": "deploy", "title": "Deploy to {{env}}", "type": "task", "depends_on": ["build"]}
		]
	}`
	tmpFile := filepath.Join(t.TempDir(), "formula.json")
	if err := os.WriteFile(tmpFile, []byte(newFormula), 0644); err != nil {
		t.Fatal(err)
	}

	if err := runFormulaUpdate("kd-f1", tmpFile, "", false); err != nil {
		t.Fatalf("update: %v", err)
	}

	if len(mc.UpdateBeadCalls) != 1 {
		t.Fatalf("expected 1 UpdateBead call, got %d", len(mc.UpdateBeadCalls))
	}

	call := mc.UpdateBeadCalls[0]
	if call.ID != "kd-f1" {
		t.Errorf("update ID = %q, want kd-f1", call.ID)
	}

	var fields struct {
		Vars  []FormulaVarDef `json:"vars"`
		Steps []FormulaStep   `json:"steps"`
	}
	if err := json.Unmarshal(call.Req.Fields, &fields); err != nil {
		t.Fatalf("unmarshal update fields: %v", err)
	}
	if len(fields.Steps) != 2 {
		t.Errorf("steps = %d, want 2", len(fields.Steps))
	}
	if len(fields.Vars) != 1 {
		t.Errorf("vars = %d, want 1", len(fields.Vars))
	}
	if fields.Steps[0].ID != "build" {
		t.Errorf("first step id = %q, want build", fields.Steps[0].ID)
	}
}

func TestFormulaUpdate_NotFormula(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-task"] = &model.Bead{ID: "kd-task", Type: "task"}
	withMockClient(t, mc)

	tmpFile := filepath.Join(t.TempDir(), "f.json")
	os.WriteFile(tmpFile, []byte(`{"steps":[{"id":"s","title":"S"}]}`), 0644)

	err := runFormulaUpdate("kd-task", tmpFile, "", false)
	if err == nil {
		t.Fatal("expected error for non-formula bead")
	}
	if !strings.Contains(err.Error(), "not formula") {
		t.Errorf("error = %q, want 'not formula'", err)
	}
}

func TestFormulaUpdate_MissingFile(t *testing.T) {
	err := runFormulaUpdate("kd-f", "", "", false)
	if err == nil {
		t.Fatal("expected error for missing --file")
	}
	if !strings.Contains(err.Error(), "--file or --assignee is required") {
		t.Errorf("error = %q, want '--file or --assignee is required'", err)
	}
}

func TestFormulaUpdate_NoSteps(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-f"] = &model.Bead{ID: "kd-f", Type: "formula"}
	withMockClient(t, mc)

	tmpFile := filepath.Join(t.TempDir(), "f.json")
	os.WriteFile(tmpFile, []byte(`{"vars":[],"steps":[]}`), 0644)

	err := runFormulaUpdate("kd-f", tmpFile, "", false)
	if err == nil {
		t.Fatal("expected error for empty steps")
	}
	if !strings.Contains(err.Error(), "at least one step") {
		t.Errorf("error = %q, want 'at least one step'", err)
	}
}

func TestFormulaUpdate_DuplicateStepID(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-f"] = &model.Bead{ID: "kd-f", Type: "formula"}
	withMockClient(t, mc)

	tmpFile := filepath.Join(t.TempDir(), "f.json")
	os.WriteFile(tmpFile, []byte(`{"steps":[{"id":"s","title":"A"},{"id":"s","title":"B"}]}`), 0644)

	err := runFormulaUpdate("kd-f", tmpFile, "", false)
	if err == nil {
		t.Fatal("expected error for duplicate step id")
	}
	if !strings.Contains(err.Error(), "duplicate step id") {
		t.Errorf("error = %q, want 'duplicate step id'", err)
	}
}

func TestFormulaUpdate_BadDependency(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-f"] = &model.Bead{ID: "kd-f", Type: "formula"}
	withMockClient(t, mc)

	tmpFile := filepath.Join(t.TempDir(), "f.json")
	os.WriteFile(tmpFile, []byte(`{"steps":[{"id":"s1","title":"A","depends_on":["missing"]}]}`), 0644)

	err := runFormulaUpdate("kd-f", tmpFile, "", false)
	if err == nil {
		t.Fatal("expected error for bad dependency")
	}
	if !strings.Contains(err.Error(), "unknown step") {
		t.Errorf("error = %q, want 'unknown step'", err)
	}
}

func TestFormulaUpdate_ServerError(t *testing.T) {
	mc := newMockClient()
	mc.Beads["kd-f"] = &model.Bead{ID: "kd-f", Type: "formula"}
	mc.UpdateErr = fmt.Errorf("server unavailable")
	withMockClient(t, mc)

	tmpFile := filepath.Join(t.TempDir(), "f.json")
	os.WriteFile(tmpFile, []byte(`{"steps":[{"id":"s","title":"S"}]}`), 0644)

	err := runFormulaUpdate("kd-f", tmpFile, "", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "updating formula") {
		t.Errorf("error = %q, want 'updating formula'", err)
	}
}

func TestFormulaUpdate_RoundTrip(t *testing.T) {
	// Dump a formula then update it back — fields should round-trip.
	mc := newMockClient()
	mc.Beads["kd-rt"] = &model.Bead{
		ID:   "kd-rt",
		Type: "formula",
		Fields: makeFormulaFields(
			[]FormulaVarDef{{Name: "env", Required: true, Default: "staging"}},
			[]FormulaStep{
				{ID: "build", Title: "Build {{env}}", Type: "task"},
				{ID: "deploy", Title: "Deploy {{env}}", Type: "task", DependsOn: []string{"build"}},
			},
		),
	}
	withMockClient(t, mc)

	// Dump to file.
	dumpFile := filepath.Join(t.TempDir(), "rt.json")
	if err := runFormulaDump("kd-rt", dumpFile); err != nil {
		t.Fatalf("dump: %v", err)
	}

	// Update from that same file.
	if err := runFormulaUpdate("kd-rt", dumpFile, "", false); err != nil {
		t.Fatalf("update: %v", err)
	}

	if len(mc.UpdateBeadCalls) != 1 {
		t.Fatalf("expected 1 UpdateBead call, got %d", len(mc.UpdateBeadCalls))
	}

	var fields struct {
		Vars  []FormulaVarDef `json:"vars"`
		Steps []FormulaStep   `json:"steps"`
	}
	if err := json.Unmarshal(mc.UpdateBeadCalls[0].Req.Fields, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(fields.Steps) != 2 {
		t.Errorf("round-trip steps = %d, want 2", len(fields.Steps))
	}
	if len(fields.Vars) != 1 {
		t.Errorf("round-trip vars = %d, want 1", len(fields.Vars))
	}
}
