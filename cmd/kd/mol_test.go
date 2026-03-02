package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/groblegark/kbeads/internal/client"
	"github.com/groblegark/kbeads/internal/model"
)

// mockBeadsClient implements client.BeadsClient for unit tests.
type mockBeadsClient struct {
	beads      map[string]*model.Bead
	revDeps    map[string][]*model.Dependency
	addedDeps  []client.AddDependencyRequest
	comments   []struct{ beadID, author, text string }
	closedIDs  []string
	deletedIDs []string

	// Error injection.
	closeErr  map[string]error
	deleteErr map[string]error
}

func newMockBeadsClient() *mockBeadsClient {
	return &mockBeadsClient{
		beads:     make(map[string]*model.Bead),
		revDeps:   make(map[string][]*model.Dependency),
		closeErr:  make(map[string]error),
		deleteErr: make(map[string]error),
	}
}

func (m *mockBeadsClient) GetBead(_ context.Context, id string) (*model.Bead, error) {
	b, ok := m.beads[id]
	if !ok {
		return nil, fmt.Errorf("bead %s not found", id)
	}
	return b, nil
}

func (m *mockBeadsClient) ListBeads(_ context.Context, req *client.ListBeadsRequest) (*client.ListBeadsResponse, error) {
	var out []*model.Bead
	for _, b := range m.beads {
		if len(req.Type) > 0 {
			match := false
			for _, t := range req.Type {
				if string(b.Type) == t {
					match = true
					break
				}
			}
			if !match {
				continue
			}
		}
		out = append(out, b)
	}
	return &client.ListBeadsResponse{Beads: out, Total: len(out)}, nil
}

func (m *mockBeadsClient) CreateBead(_ context.Context, req *client.CreateBeadRequest) (*model.Bead, error) {
	return &model.Bead{ID: "kd-new", Title: req.Title, Type: model.BeadType(req.Type)}, nil
}

func (m *mockBeadsClient) UpdateBead(_ context.Context, id string, req *client.UpdateBeadRequest) (*model.Bead, error) {
	return m.beads[id], nil
}

func (m *mockBeadsClient) CloseBead(_ context.Context, id string, closedBy string) (*model.Bead, error) {
	if err, ok := m.closeErr[id]; ok {
		return nil, err
	}
	m.closedIDs = append(m.closedIDs, id)
	b := m.beads[id]
	if b != nil {
		b.Status = model.StatusClosed
	}
	return b, nil
}

func (m *mockBeadsClient) DeleteBead(_ context.Context, id string) error {
	if err, ok := m.deleteErr[id]; ok {
		return err
	}
	m.deletedIDs = append(m.deletedIDs, id)
	delete(m.beads, id)
	return nil
}

func (m *mockBeadsClient) AddDependency(_ context.Context, req *client.AddDependencyRequest) (*model.Dependency, error) {
	m.addedDeps = append(m.addedDeps, *req)
	return &model.Dependency{BeadID: req.BeadID, DependsOnID: req.DependsOnID, Type: model.DependencyType(req.Type)}, nil
}

func (m *mockBeadsClient) RemoveDependency(_ context.Context, beadID, dependsOnID, depType string) error {
	return nil
}

func (m *mockBeadsClient) GetDependencies(_ context.Context, beadID string) ([]*model.Dependency, error) {
	return nil, nil
}

func (m *mockBeadsClient) GetReverseDependencies(_ context.Context, beadID string) ([]*model.Dependency, error) {
	return m.revDeps[beadID], nil
}

func (m *mockBeadsClient) AddLabel(_ context.Context, beadID, label string) (*model.Bead, error) {
	return nil, nil
}

func (m *mockBeadsClient) RemoveLabel(_ context.Context, beadID, label string) error {
	return nil
}

func (m *mockBeadsClient) GetLabels(_ context.Context, beadID string) ([]string, error) {
	return nil, nil
}

func (m *mockBeadsClient) AddComment(_ context.Context, beadID, author, text string) (*model.Comment, error) {
	m.comments = append(m.comments, struct{ beadID, author, text string }{beadID, author, text})
	return &model.Comment{BeadID: beadID, Author: author, Text: text}, nil
}

func (m *mockBeadsClient) GetComments(_ context.Context, beadID string) ([]*model.Comment, error) {
	return nil, nil
}

func (m *mockBeadsClient) GetEvents(_ context.Context, beadID string) ([]*model.Event, error) {
	return nil, nil
}

func (m *mockBeadsClient) SetConfig(_ context.Context, key string, value json.RawMessage) (*model.Config, error) {
	return nil, nil
}

func (m *mockBeadsClient) GetConfig(_ context.Context, key string) (*model.Config, error) {
	return nil, nil
}

func (m *mockBeadsClient) ListConfigs(_ context.Context, namespace string) ([]*model.Config, error) {
	return nil, nil
}

func (m *mockBeadsClient) DeleteConfig(_ context.Context, key string) error {
	return nil
}

func (m *mockBeadsClient) EmitHook(_ context.Context, req *client.EmitHookRequest) (*client.EmitHookResponse, error) {
	return nil, nil
}

func (m *mockBeadsClient) ListGates(_ context.Context, agentBeadID string) ([]model.GateRow, error) {
	return nil, nil
}

func (m *mockBeadsClient) SatisfyGate(_ context.Context, agentBeadID, gateID string) error {
	return nil
}

func (m *mockBeadsClient) ClearGate(_ context.Context, agentBeadID, gateID string) error {
	return nil
}

func (m *mockBeadsClient) GetAgentRoster(_ context.Context, staleThresholdSecs int) (*client.AgentRosterResponse, error) {
	return nil, nil
}

func (m *mockBeadsClient) Health(_ context.Context) (string, error) {
	return "ok", nil
}

func (m *mockBeadsClient) Close() error {
	return nil
}

// withMockClient sets the global beadsClient to a mock for the duration of a test.
func withMockClient(t *testing.T, mock *mockBeadsClient) {
	t.Helper()
	old := beadsClient
	beadsClient = mock
	t.Cleanup(func() { beadsClient = old })
}

// --- molFormulaID tests ---

func TestMolFormulaID_FormulaID(t *testing.T) {
	b := &model.Bead{
		Fields: json.RawMessage(`{"formula_id":"kd-f1","template_id":"kd-t1"}`),
	}
	got := molFormulaID(b)
	if got != "kd-f1" {
		t.Errorf("molFormulaID() = %q, want kd-f1 (formula_id takes precedence)", got)
	}
}

func TestMolFormulaID_TemplateIDFallback(t *testing.T) {
	b := &model.Bead{
		Fields: json.RawMessage(`{"template_id":"kd-t1"}`),
	}
	got := molFormulaID(b)
	if got != "kd-t1" {
		t.Errorf("molFormulaID() = %q, want kd-t1 (template_id fallback)", got)
	}
}

func TestMolFormulaID_NoFields(t *testing.T) {
	b := &model.Bead{}
	got := molFormulaID(b)
	if got != "" {
		t.Errorf("molFormulaID() = %q, want empty", got)
	}
}

func TestMolFormulaID_EmptyFields(t *testing.T) {
	b := &model.Bead{
		Fields: json.RawMessage(`{}`),
	}
	got := molFormulaID(b)
	if got != "" {
		t.Errorf("molFormulaID() = %q, want empty", got)
	}
}

// --- printMolList tests ---

func TestPrintMolList_NoMolecules(t *testing.T) {
	out := captureStdout(t, func() {
		printMolList(nil, 0)
	})
	if !strings.Contains(out, "No molecules found") {
		t.Errorf("expected 'No molecules found', got:\n%s", out)
	}
}

func TestPrintMolList_WithMolecules(t *testing.T) {
	beads := []*model.Bead{
		{
			ID:     "kd-m1",
			Status: "open",
			Title:  "Auth molecule",
			Type:   "molecule",
			Fields: json.RawMessage(`{"formula_id":"kd-f1"}`),
		},
	}
	out := captureStdout(t, func() {
		printMolList(beads, 1)
	})
	if !strings.Contains(out, "kd-m1") {
		t.Error("output missing molecule ID")
	}
	if !strings.Contains(out, "kd-f1") {
		t.Error("output missing formula ID")
	}
	if !strings.Contains(out, "1 molecules (1 total)") {
		t.Errorf("output missing total count, got:\n%s", out)
	}
}

func TestPrintMolList_TruncatesLongTitle(t *testing.T) {
	longTitle := strings.Repeat("x", 50)
	beads := []*model.Bead{
		{ID: "kd-m1", Title: longTitle, Type: "molecule"},
	}
	out := captureStdout(t, func() {
		printMolList(beads, 1)
	})
	if strings.Contains(out, longTitle) {
		t.Error("expected long title to be truncated")
	}
	if !strings.Contains(out, "...") {
		t.Error("expected truncated title to have ellipsis")
	}
}

// --- mol list command tests ---

func TestMolListCmd_FiltersTypeMolecule(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{ID: "kd-m1", Title: "Mol 1", Type: "molecule", Status: "open"}
	mock.beads["kd-t1"] = &model.Bead{ID: "kd-t1", Title: "Task 1", Type: "task", Status: "open"}
	withMockClient(t, mock)

	out := captureStdout(t, func() {
		molListCmd.SetArgs([]string{})
		_ = molListCmd.RunE(molListCmd, []string{})
	})
	if !strings.Contains(out, "kd-m1") {
		t.Error("expected molecule in output")
	}
}

// --- mol show command tests ---

func TestMolShowCmd_AcceptsMoleculeType(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{
		ID:     "kd-m1",
		Title:  "Test Molecule",
		Type:   "molecule",
		Kind:   "issue",
		Status: "open",
		Fields: json.RawMessage(`{"formula_id":"kd-f1"}`),
	}
	withMockClient(t, mock)

	out := captureStdout(t, func() {
		_ = molShowCmd.RunE(molShowCmd, []string{"kd-m1"})
	})
	if !strings.Contains(out, "kd-m1") {
		t.Error("show output missing molecule ID")
	}
	if !strings.Contains(out, "kd-f1") {
		t.Error("show output missing formula ID")
	}
}

func TestMolShowCmd_AcceptsBundleType(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-b1"] = &model.Bead{
		ID:     "kd-b1",
		Title:  "Legacy Bundle",
		Type:   "bundle",
		Kind:   "issue",
		Status: "open",
		Fields: json.RawMessage(`{"template_id":"kd-t1"}`),
	}
	withMockClient(t, mock)

	out := captureStdout(t, func() {
		_ = molShowCmd.RunE(molShowCmd, []string{"kd-b1"})
	})
	if !strings.Contains(out, "kd-b1") {
		t.Error("show output missing bundle ID")
	}
	if !strings.Contains(out, "kd-t1") {
		t.Error("show output missing template ID (fallback)")
	}
}

func TestMolShowCmd_RejectsNonMoleculeType(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-t1"] = &model.Bead{
		ID:   "kd-t1",
		Title: "Task",
		Type: "task",
	}
	withMockClient(t, mock)

	err := molShowCmd.RunE(molShowCmd, []string{"kd-t1"})
	if err == nil {
		t.Fatal("expected error for non-molecule type")
	}
	if !strings.Contains(err.Error(), "not molecule") {
		t.Errorf("expected 'not molecule' in error, got: %v", err)
	}
}

func TestMolShowCmd_RendersChildren(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{
		ID:     "kd-m1",
		Title:  "Parent Molecule",
		Type:   "molecule",
		Kind:   "issue",
		Status: "open",
	}
	mock.beads["kd-c1"] = &model.Bead{
		ID:     "kd-c1",
		Title:  "Child Task",
		Type:   "task",
		Status: "open",
	}
	mock.revDeps["kd-m1"] = []*model.Dependency{
		{BeadID: "kd-c1", DependsOnID: "kd-m1", Type: "parent-child"},
	}
	withMockClient(t, mock)

	out := captureStdout(t, func() {
		_ = molShowCmd.RunE(molShowCmd, []string{"kd-m1"})
	})
	if !strings.Contains(out, "Steps:") {
		t.Error("expected 'Steps:' header")
	}
	if !strings.Contains(out, "kd-c1") {
		t.Error("expected child bead ID in output")
	}
	if !strings.Contains(out, "Child Task") {
		t.Error("expected child title in output")
	}
}

func TestMolShowCmd_SkipsNonParentChildDeps(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{
		ID:     "kd-m1",
		Title:  "Molecule",
		Type:   "molecule",
		Kind:   "issue",
		Status: "open",
	}
	mock.revDeps["kd-m1"] = []*model.Dependency{
		{BeadID: "kd-r1", DependsOnID: "kd-m1", Type: "related"},
	}
	withMockClient(t, mock)

	out := captureStdout(t, func() {
		_ = molShowCmd.RunE(molShowCmd, []string{"kd-m1"})
	})
	if strings.Contains(out, "kd-r1") {
		t.Error("non-parent-child dep should not be rendered in Steps")
	}
}

// --- bond command tests ---

func TestBondCmd_SequentialDefault(t *testing.T) {
	mock := newMockBeadsClient()
	withMockClient(t, mock)

	cmd := newBondCmd()
	out := captureStdout(t, func() {
		_ = cmd.RunE(cmd, []string{"kd-a", "kd-b"})
	})

	if len(mock.addedDeps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(mock.addedDeps))
	}
	dep := mock.addedDeps[0]
	if dep.BeadID != "kd-b" {
		t.Errorf("expected BeadID=kd-b, got %s", dep.BeadID)
	}
	if dep.DependsOnID != "kd-a" {
		t.Errorf("expected DependsOnID=kd-a, got %s", dep.DependsOnID)
	}
	if dep.Type != "blocks" {
		t.Errorf("expected Type=blocks (sequential default), got %s", dep.Type)
	}
	if !strings.Contains(out, "Bonded") {
		t.Error("expected 'Bonded' in output")
	}
}

func TestBondCmd_SequentialExplicit(t *testing.T) {
	mock := newMockBeadsClient()
	withMockClient(t, mock)

	cmd := newBondCmd()
	cmd.SetArgs([]string{"--type", "sequential"})
	_ = cmd.Flags().Set("type", "sequential")
	_ = cmd.RunE(cmd, []string{"kd-a", "kd-b"})

	if len(mock.addedDeps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(mock.addedDeps))
	}
	if mock.addedDeps[0].Type != "blocks" {
		t.Errorf("sequential should map to blocks, got %s", mock.addedDeps[0].Type)
	}
}

func TestBondCmd_Parallel(t *testing.T) {
	mock := newMockBeadsClient()
	withMockClient(t, mock)

	cmd := newBondCmd()
	_ = cmd.Flags().Set("type", "parallel")
	_ = cmd.RunE(cmd, []string{"kd-a", "kd-b"})

	if len(mock.addedDeps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(mock.addedDeps))
	}
	if mock.addedDeps[0].Type != "related" {
		t.Errorf("parallel should map to related, got %s", mock.addedDeps[0].Type)
	}
}

func TestBondCmd_UnknownTypeError(t *testing.T) {
	mock := newMockBeadsClient()
	withMockClient(t, mock)

	cmd := newBondCmd()
	_ = cmd.Flags().Set("type", "invalid")
	err := cmd.RunE(cmd, []string{"kd-a", "kd-b"})

	if err == nil {
		t.Fatal("expected error for unknown bond type")
	}
	if !strings.Contains(err.Error(), "unknown bond type") {
		t.Errorf("expected 'unknown bond type' in error, got: %v", err)
	}
}

// --- squash command tests ---

func TestSquashCmd_TypeValidation(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-t1"] = &model.Bead{ID: "kd-t1", Title: "Task", Type: "task"}
	withMockClient(t, mock)

	cmd := newSquashCmd()
	err := cmd.RunE(cmd, []string{"kd-t1"})
	if err == nil {
		t.Fatal("expected error for non-molecule type")
	}
	if !strings.Contains(err.Error(), "not molecule") {
		t.Errorf("expected 'not molecule' in error, got: %v", err)
	}
}

func TestSquashCmd_AcceptsBundleType(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-b1"] = &model.Bead{ID: "kd-b1", Title: "Bundle", Type: "bundle", Status: "open"}
	withMockClient(t, mock)

	cmd := newSquashCmd()
	out := captureStdout(t, func() {
		_ = cmd.RunE(cmd, []string{"kd-b1"})
	})
	if !strings.Contains(out, "Squashed") {
		t.Errorf("expected squash success output, got:\n%s", out)
	}
}

func TestSquashCmd_DigestFromChildTitles(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{ID: "kd-m1", Title: "My Molecule", Type: "molecule", Status: "open"}
	mock.beads["kd-c1"] = &model.Bead{ID: "kd-c1", Title: "Step 1", Type: "task", Status: "open"}
	mock.beads["kd-c2"] = &model.Bead{ID: "kd-c2", Title: "Step 2", Type: "task", Status: "closed"}
	mock.revDeps["kd-m1"] = []*model.Dependency{
		{BeadID: "kd-c1", DependsOnID: "kd-m1", Type: "parent-child"},
		{BeadID: "kd-c2", DependsOnID: "kd-m1", Type: "parent-child"},
	}
	withMockClient(t, mock)

	cmd := newSquashCmd()
	captureStdout(t, func() {
		_ = cmd.RunE(cmd, []string{"kd-m1"})
	})

	if len(mock.comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(mock.comments))
	}
	digest := mock.comments[0].text
	if !strings.Contains(digest, "Step 1") || !strings.Contains(digest, "Step 2") {
		t.Errorf("digest should contain child titles, got: %s", digest)
	}
	if !strings.Contains(digest, "My Molecule") {
		t.Errorf("digest should reference molecule title, got: %s", digest)
	}
}

func TestSquashCmd_CustomSummary(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{ID: "kd-m1", Title: "Mol", Type: "molecule", Status: "open"}
	withMockClient(t, mock)

	cmd := newSquashCmd()
	_ = cmd.Flags().Set("summary", "Custom digest text")
	captureStdout(t, func() {
		_ = cmd.RunE(cmd, []string{"kd-m1"})
	})

	if len(mock.comments) != 1 {
		t.Fatalf("expected 1 comment, got %d", len(mock.comments))
	}
	if mock.comments[0].text != "Custom digest text" {
		t.Errorf("expected custom summary, got: %s", mock.comments[0].text)
	}
}

func TestSquashCmd_ClosesOpenChildren(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{ID: "kd-m1", Title: "Mol", Type: "molecule", Status: "open"}
	mock.beads["kd-c1"] = &model.Bead{ID: "kd-c1", Title: "Open", Type: "task", Status: "open"}
	mock.beads["kd-c2"] = &model.Bead{ID: "kd-c2", Title: "Closed", Type: "task", Status: "closed"}
	mock.revDeps["kd-m1"] = []*model.Dependency{
		{BeadID: "kd-c1", DependsOnID: "kd-m1", Type: "parent-child"},
		{BeadID: "kd-c2", DependsOnID: "kd-m1", Type: "parent-child"},
	}
	withMockClient(t, mock)

	cmd := newSquashCmd()
	captureStdout(t, func() {
		_ = cmd.RunE(cmd, []string{"kd-m1"})
	})

	// Only the open child should be closed.
	if len(mock.closedIDs) != 1 || mock.closedIDs[0] != "kd-c1" {
		t.Errorf("expected only kd-c1 closed, got: %v", mock.closedIDs)
	}
}

func TestSquashCmd_PartialCloseFailure(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{ID: "kd-m1", Title: "Mol", Type: "molecule", Status: "open"}
	mock.beads["kd-c1"] = &model.Bead{ID: "kd-c1", Title: "Will fail", Type: "task", Status: "open"}
	mock.beads["kd-c2"] = &model.Bead{ID: "kd-c2", Title: "Will succeed", Type: "task", Status: "open"}
	mock.revDeps["kd-m1"] = []*model.Dependency{
		{BeadID: "kd-c1", DependsOnID: "kd-m1", Type: "parent-child"},
		{BeadID: "kd-c2", DependsOnID: "kd-m1", Type: "parent-child"},
	}
	mock.closeErr["kd-c1"] = fmt.Errorf("close failed")
	withMockClient(t, mock)

	cmd := newSquashCmd()
	out := captureStdout(t, func() {
		err := cmd.RunE(cmd, []string{"kd-m1"})
		if err != nil {
			t.Fatalf("squash should not return error on partial failure, got: %v", err)
		}
	})

	// Should warn about the failure.
	if !strings.Contains(out, "warning") {
		t.Errorf("expected warning about failed close, got:\n%s", out)
	}
	// The successful close should still happen.
	if len(mock.closedIDs) != 1 || mock.closedIDs[0] != "kd-c2" {
		t.Errorf("expected kd-c2 closed despite kd-c1 failure, got: %v", mock.closedIDs)
	}
}

func TestSquashCmd_CloseMolFlag(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{ID: "kd-m1", Title: "Mol", Type: "molecule", Status: "open"}
	withMockClient(t, mock)

	cmd := newSquashCmd()
	_ = cmd.Flags().Set("close-mol", "true")
	out := captureStdout(t, func() {
		_ = cmd.RunE(cmd, []string{"kd-m1"})
	})

	found := false
	for _, id := range mock.closedIDs {
		if id == "kd-m1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected molecule itself to be closed with --close-mol")
	}
	if !strings.Contains(out, "Molecule closed") {
		t.Error("expected 'Molecule closed' in output")
	}
}

func TestSquashCmd_DryRun(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{ID: "kd-m1", Title: "Mol", Type: "molecule", Status: "open"}
	mock.beads["kd-c1"] = &model.Bead{ID: "kd-c1", Title: "Child", Type: "task", Status: "open"}
	mock.revDeps["kd-m1"] = []*model.Dependency{
		{BeadID: "kd-c1", DependsOnID: "kd-m1", Type: "parent-child"},
	}
	withMockClient(t, mock)

	cmd := newSquashCmd()
	_ = cmd.Flags().Set("dry-run", "true")
	out := captureStdout(t, func() {
		_ = cmd.RunE(cmd, []string{"kd-m1"})
	})

	if !strings.Contains(out, "Would squash") {
		t.Errorf("dry-run output missing 'Would squash', got:\n%s", out)
	}
	if len(mock.comments) > 0 {
		t.Error("dry-run should not add comments")
	}
	if len(mock.closedIDs) > 0 {
		t.Error("dry-run should not close anything")
	}
}

func TestSquashCmd_AddCommentVerification(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{ID: "kd-m1", Title: "Mol", Type: "molecule", Status: "open"}
	withMockClient(t, mock)

	cmd := newSquashCmd()
	_ = cmd.Flags().Set("summary", "Final summary")
	captureStdout(t, func() {
		_ = cmd.RunE(cmd, []string{"kd-m1"})
	})

	if len(mock.comments) != 1 {
		t.Fatalf("expected 1 AddComment call, got %d", len(mock.comments))
	}
	if mock.comments[0].beadID != "kd-m1" {
		t.Errorf("expected comment on kd-m1, got %s", mock.comments[0].beadID)
	}
}

// --- burn command tests ---

func TestBurnCmd_TypeValidation(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-t1"] = &model.Bead{ID: "kd-t1", Title: "Task", Type: "task"}
	withMockClient(t, mock)

	cmd := newBurnCmd()
	err := cmd.RunE(cmd, []string{"kd-t1"})
	if err == nil {
		t.Fatal("expected error for non-molecule type")
	}
	if !strings.Contains(err.Error(), "not molecule") {
		t.Errorf("expected 'not molecule' in error, got: %v", err)
	}
}

func TestBurnCmd_AcceptsBundleType(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-b1"] = &model.Bead{ID: "kd-b1", Title: "Bundle", Type: "bundle", Status: "open"}
	withMockClient(t, mock)

	cmd := newBurnCmd()
	_ = cmd.Flags().Set("force", "true")
	captureStdout(t, func() {
		_ = cmd.RunE(cmd, []string{"kd-b1"})
	})
	// Should succeed without error — bundle type is accepted.
	found := false
	for _, id := range mock.deletedIDs {
		if id == "kd-b1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected bundle to be deleted")
	}
}

func TestBurnCmd_CascadeDeleteChildren(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{ID: "kd-m1", Title: "Mol", Type: "molecule", Status: "open"}
	mock.beads["kd-c1"] = &model.Bead{ID: "kd-c1", Title: "Child 1", Type: "task"}
	mock.beads["kd-c2"] = &model.Bead{ID: "kd-c2", Title: "Child 2", Type: "task"}
	mock.revDeps["kd-m1"] = []*model.Dependency{
		{BeadID: "kd-c1", DependsOnID: "kd-m1", Type: "parent-child"},
		{BeadID: "kd-c2", DependsOnID: "kd-m1", Type: "parent-child"},
	}
	withMockClient(t, mock)

	cmd := newBurnCmd()
	_ = cmd.Flags().Set("force", "true")
	captureStdout(t, func() {
		_ = cmd.RunE(cmd, []string{"kd-m1"})
	})

	// Children should be deleted first, then the molecule.
	if len(mock.deletedIDs) != 3 {
		t.Fatalf("expected 3 deletes (2 children + 1 molecule), got %d: %v", len(mock.deletedIDs), mock.deletedIDs)
	}
	// Molecule should be last.
	if mock.deletedIDs[len(mock.deletedIDs)-1] != "kd-m1" {
		t.Errorf("molecule should be deleted last, got order: %v", mock.deletedIDs)
	}
}

func TestBurnCmd_FiltersParentChildDeps(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{ID: "kd-m1", Title: "Mol", Type: "molecule", Status: "open"}
	mock.revDeps["kd-m1"] = []*model.Dependency{
		{BeadID: "kd-c1", DependsOnID: "kd-m1", Type: "parent-child"},
		{BeadID: "kd-r1", DependsOnID: "kd-m1", Type: "related"},
	}
	withMockClient(t, mock)

	cmd := newBurnCmd()
	_ = cmd.Flags().Set("force", "true")
	captureStdout(t, func() {
		_ = cmd.RunE(cmd, []string{"kd-m1"})
	})

	// Only parent-child child (kd-c1) should be deleted, not related (kd-r1).
	for _, id := range mock.deletedIDs {
		if id == "kd-r1" {
			t.Error("related dep should not be deleted — only parent-child")
		}
	}
}

func TestBurnCmd_DryRun(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{ID: "kd-m1", Title: "Mol", Type: "molecule", Status: "open"}
	mock.revDeps["kd-m1"] = []*model.Dependency{
		{BeadID: "kd-c1", DependsOnID: "kd-m1", Type: "parent-child"},
	}
	withMockClient(t, mock)

	cmd := newBurnCmd()
	_ = cmd.Flags().Set("dry-run", "true")
	out := captureStdout(t, func() {
		_ = cmd.RunE(cmd, []string{"kd-m1"})
	})

	if !strings.Contains(out, "Would burn") {
		t.Errorf("dry-run output missing 'Would burn', got:\n%s", out)
	}
	if !strings.Contains(out, "kd-c1") {
		t.Error("dry-run output should list children to be deleted")
	}
	if len(mock.deletedIDs) > 0 {
		t.Error("dry-run should not delete anything")
	}
}

func TestBurnCmd_RequiresForceWithChildren(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{ID: "kd-m1", Title: "Mol", Type: "molecule", Status: "open"}
	mock.revDeps["kd-m1"] = []*model.Dependency{
		{BeadID: "kd-c1", DependsOnID: "kd-m1", Type: "parent-child"},
	}
	withMockClient(t, mock)

	cmd := newBurnCmd()
	// No --force flag.
	out := captureStdout(t, func() {
		_ = cmd.RunE(cmd, []string{"kd-m1"})
	})

	if !strings.Contains(out, "--force") {
		t.Errorf("expected prompt about --force, got:\n%s", out)
	}
	if len(mock.deletedIDs) > 0 {
		t.Error("should not delete without --force")
	}
}

func TestBurnCmd_NoForceNeededWithoutChildren(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{ID: "kd-m1", Title: "Mol", Type: "molecule", Status: "open"}
	withMockClient(t, mock)

	cmd := newBurnCmd()
	// No --force, but also no children.
	captureStdout(t, func() {
		err := cmd.RunE(cmd, []string{"kd-m1"})
		if err != nil {
			t.Fatalf("burn without children should not require --force, got: %v", err)
		}
	})

	if len(mock.deletedIDs) != 1 || mock.deletedIDs[0] != "kd-m1" {
		t.Errorf("expected molecule deleted, got: %v", mock.deletedIDs)
	}
}

func TestBurnCmd_PartialDeleteFailure(t *testing.T) {
	mock := newMockBeadsClient()
	mock.beads["kd-m1"] = &model.Bead{ID: "kd-m1", Title: "Mol", Type: "molecule", Status: "open"}
	mock.revDeps["kd-m1"] = []*model.Dependency{
		{BeadID: "kd-c1", DependsOnID: "kd-m1", Type: "parent-child"},
		{BeadID: "kd-c2", DependsOnID: "kd-m1", Type: "parent-child"},
	}
	mock.deleteErr["kd-c1"] = fmt.Errorf("delete failed")
	withMockClient(t, mock)

	cmd := newBurnCmd()
	_ = cmd.Flags().Set("force", "true")
	out := captureStdout(t, func() {
		_ = cmd.RunE(cmd, []string{"kd-m1"})
	})

	if !strings.Contains(out, "warning") {
		t.Errorf("expected warning about failed child delete, got:\n%s", out)
	}
	// kd-c2 and kd-m1 should still be deleted.
	deletedSet := map[string]bool{}
	for _, id := range mock.deletedIDs {
		deletedSet[id] = true
	}
	if !deletedSet["kd-c2"] {
		t.Error("kd-c2 should still be deleted despite kd-c1 failure")
	}
	if !deletedSet["kd-m1"] {
		t.Error("molecule should still be deleted despite child failure")
	}
}
