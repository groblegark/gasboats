package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"github.com/groblegark/kbeads/internal/client"
	"github.com/groblegark/kbeads/internal/model"
)

// mockBeadsClient implements client.BeadsClient for testing CLI commands.
type mockBeadsClient struct {
	mu sync.Mutex

	// Call tracking.
	CreateBeadCalls    []*client.CreateBeadRequest
	UpdateBeadCalls    []struct {
		ID  string
		Req *client.UpdateBeadRequest
	}
	GetBeadCalls       []string
	AddDepCalls        []*client.AddDependencyRequest
	CloseBeadCalls     []struct{ ID, ClosedBy string }
	DeleteBeadCalls    []string
	AddCommentCalls    []struct{ BeadID, Author, Text string }
	GetRevDepCalls     []string
	ListBeadsCalls     []*client.ListBeadsRequest

	// Configurable return values.
	Beads           map[string]*model.Bead // keyed by ID
	ListBeadsResult []*model.Bead          // if set, ListBeads returns these
	RevDeps     map[string][]*model.Dependency
	CreateIDs   []string // sequential IDs for CreateBead calls
	createIndex int

	// Error injection.
	CreateErr    error
	GetErr       error
	UpdateErr    error
	AddDepErr    error
	CloseErr     error
	DeleteErr    error
	CommentErr   error
	RevDepErr    error
	ListErr      error

	// Per-call error injection (keyed by call argument).
	DeleteErrs map[string]error
	CloseErrs  map[string]error
}

func newMockClient() *mockBeadsClient {
	return &mockBeadsClient{
		Beads:      make(map[string]*model.Bead),
		RevDeps:    make(map[string][]*model.Dependency),
		DeleteErrs: make(map[string]error),
		CloseErrs:  make(map[string]error),
	}
}

func (m *mockBeadsClient) CreateBead(_ context.Context, req *client.CreateBeadRequest) (*model.Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CreateBeadCalls = append(m.CreateBeadCalls, req)
	if m.CreateErr != nil {
		return nil, m.CreateErr
	}
	id := fmt.Sprintf("kd-new-%d", m.createIndex)
	if m.createIndex < len(m.CreateIDs) {
		id = m.CreateIDs[m.createIndex]
	}
	m.createIndex++
	return &model.Bead{
		ID:          id,
		Title:       req.Title,
		Type:        model.BeadType(req.Type),
		Description: req.Description,
		Priority:    req.Priority,
		Status:      model.StatusOpen,
		Labels:      req.Labels,
		Assignee:    req.Assignee,
		Fields:      req.Fields,
	}, nil
}

func (m *mockBeadsClient) GetBead(_ context.Context, id string) (*model.Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetBeadCalls = append(m.GetBeadCalls, id)
	if m.GetErr != nil {
		return nil, m.GetErr
	}
	if b, ok := m.Beads[id]; ok {
		return b, nil
	}
	return nil, fmt.Errorf("bead %s not found", id)
}

func (m *mockBeadsClient) ListBeads(_ context.Context, req *client.ListBeadsRequest) (*client.ListBeadsResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ListBeadsCalls = append(m.ListBeadsCalls, req)
	if m.ListErr != nil {
		return nil, m.ListErr
	}
	if m.ListBeadsResult != nil {
		return &client.ListBeadsResponse{Beads: m.ListBeadsResult, Total: len(m.ListBeadsResult)}, nil
	}
	return &client.ListBeadsResponse{}, nil
}

func (m *mockBeadsClient) UpdateBead(_ context.Context, id string, req *client.UpdateBeadRequest) (*model.Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UpdateBeadCalls = append(m.UpdateBeadCalls, struct {
		ID  string
		Req *client.UpdateBeadRequest
	}{id, req})
	if m.UpdateErr != nil {
		return nil, m.UpdateErr
	}
	if b, ok := m.Beads[id]; ok {
		return b, nil
	}
	return &model.Bead{ID: id}, nil
}

func (m *mockBeadsClient) CloseBead(_ context.Context, id, closedBy string) (*model.Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CloseBeadCalls = append(m.CloseBeadCalls, struct{ ID, ClosedBy string }{id, closedBy})
	if err, ok := m.CloseErrs[id]; ok {
		return nil, err
	}
	if m.CloseErr != nil {
		return nil, m.CloseErr
	}
	return &model.Bead{ID: id, Status: model.StatusClosed}, nil
}

func (m *mockBeadsClient) DeleteBead(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DeleteBeadCalls = append(m.DeleteBeadCalls, id)
	if err, ok := m.DeleteErrs[id]; ok {
		return err
	}
	return m.DeleteErr
}

func (m *mockBeadsClient) AddDependency(_ context.Context, req *client.AddDependencyRequest) (*model.Dependency, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AddDepCalls = append(m.AddDepCalls, req)
	if m.AddDepErr != nil {
		return nil, m.AddDepErr
	}
	return &model.Dependency{
		BeadID:      req.BeadID,
		DependsOnID: req.DependsOnID,
		Type:        model.DependencyType(req.Type),
	}, nil
}

func (m *mockBeadsClient) RemoveDependency(context.Context, string, string, string) error {
	return nil
}

func (m *mockBeadsClient) GetDependencies(_ context.Context, beadID string) ([]*model.Dependency, error) {
	return nil, nil
}

func (m *mockBeadsClient) GetReverseDependencies(_ context.Context, beadID string) ([]*model.Dependency, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.GetRevDepCalls = append(m.GetRevDepCalls, beadID)
	if m.RevDepErr != nil {
		return nil, m.RevDepErr
	}
	return m.RevDeps[beadID], nil
}

func (m *mockBeadsClient) AddLabel(context.Context, string, string) (*model.Bead, error) {
	return &model.Bead{}, nil
}

func (m *mockBeadsClient) RemoveLabel(context.Context, string, string) error { return nil }

func (m *mockBeadsClient) GetLabels(context.Context, string) ([]string, error) {
	return nil, nil
}

func (m *mockBeadsClient) AddComment(_ context.Context, beadID, author, text string) (*model.Comment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AddCommentCalls = append(m.AddCommentCalls, struct{ BeadID, Author, Text string }{beadID, author, text})
	if m.CommentErr != nil {
		return nil, m.CommentErr
	}
	return &model.Comment{BeadID: beadID, Author: author, Text: text}, nil
}

func (m *mockBeadsClient) GetComments(context.Context, string) ([]*model.Comment, error) {
	return nil, nil
}

func (m *mockBeadsClient) GetEvents(context.Context, string) ([]*model.Event, error) {
	return nil, nil
}

func (m *mockBeadsClient) SetConfig(_ context.Context, key string, value json.RawMessage) (*model.Config, error) {
	return &model.Config{Key: key, Value: value}, nil
}

func (m *mockBeadsClient) GetConfig(_ context.Context, key string) (*model.Config, error) {
	return &model.Config{Key: key}, nil
}

func (m *mockBeadsClient) ListConfigs(context.Context, string) ([]*model.Config, error) {
	return nil, nil
}

func (m *mockBeadsClient) DeleteConfig(context.Context, string) error { return nil }

func (m *mockBeadsClient) EmitHook(context.Context, *client.EmitHookRequest) (*client.EmitHookResponse, error) {
	return &client.EmitHookResponse{}, nil
}

func (m *mockBeadsClient) ListGates(context.Context, string) ([]model.GateRow, error) {
	return nil, nil
}

func (m *mockBeadsClient) SatisfyGate(context.Context, string, string) error { return nil }
func (m *mockBeadsClient) ClearGate(context.Context, string, string) error   { return nil }

func (m *mockBeadsClient) GetAgentRoster(context.Context, int) (*client.AgentRosterResponse, error) {
	return &client.AgentRosterResponse{}, nil
}

func (m *mockBeadsClient) Health(context.Context) (string, error) { return "ok", nil }
func (m *mockBeadsClient) Close() error                          { return nil }

// withMockClient swaps beadsClient and actor for the duration of a test,
// restoring the originals on cleanup.
func withMockClient(t *testing.T, mc *mockBeadsClient) {
	t.Helper()
	origClient := beadsClient
	origActor := actor
	origJSON := jsonOutput
	t.Cleanup(func() {
		beadsClient = origClient
		actor = origActor
		jsonOutput = origJSON
	})
	beadsClient = mc
	actor = "test-actor"
	jsonOutput = false
}
