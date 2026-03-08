package sync

import (
	"context"
	"database/sql"
	"sort"
	"strings"

	"github.com/groblegark/kbeads/internal/model"
	"github.com/groblegark/kbeads/internal/store"
)

// mockStore is a minimal in-memory store for sync tests.
type mockStore struct {
	beads    map[string]*model.Bead
	configs  map[string]*model.Config
	labels   map[string][]string
	deps     map[string][]*model.Dependency
	comments map[string][]*model.Comment
}

func newMockStore() *mockStore {
	return &mockStore{
		beads:    make(map[string]*model.Bead),
		configs:  make(map[string]*model.Config),
		labels:   make(map[string][]string),
		deps:     make(map[string][]*model.Dependency),
		comments: make(map[string][]*model.Comment),
	}
}

func (m *mockStore) CreateBead(_ context.Context, bead *model.Bead) error {
	m.beads[bead.ID] = bead
	return nil
}

func (m *mockStore) GetBead(_ context.Context, id string) (*model.Bead, error) {
	b, ok := m.beads[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return b, nil
}

func (m *mockStore) ListBeads(_ context.Context, _ model.BeadFilter) ([]*model.Bead, int, error) {
	var result []*model.Bead
	for _, b := range m.beads {
		result = append(result, b)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID
	})
	return result, len(result), nil
}

func (m *mockStore) UpdateBead(_ context.Context, bead *model.Bead) error {
	m.beads[bead.ID] = bead
	return nil
}

func (m *mockStore) CloseBead(_ context.Context, _ string, _ string) (*model.Bead, error) {
	return nil, nil
}

func (m *mockStore) DeleteBead(_ context.Context, id string) error {
	delete(m.beads, id)
	return nil
}

func (m *mockStore) GetDependenciesForBeads(_ context.Context, _ []string) (map[string][]*model.Dependency, error) {
	return make(map[string][]*model.Dependency), nil
}

func (m *mockStore) GetReverseDependenciesForBeads(_ context.Context, _ []string) (map[string][]*model.Dependency, error) {
	return make(map[string][]*model.Dependency), nil
}

func (m *mockStore) GetDependencyCounts(_ context.Context, _ []string) (map[string]*model.DependencyCounts, error) {
	return make(map[string]*model.DependencyCounts), nil
}

func (m *mockStore) GetLabelsForBeads(_ context.Context, _ []string) (map[string][]string, error) {
	return make(map[string][]string), nil
}

func (m *mockStore) GetBlockedByForBeads(_ context.Context, _ []string) (map[string][]string, error) {
	return make(map[string][]string), nil
}

func (m *mockStore) GetBeadsByIDs(_ context.Context, ids []string) ([]*model.Bead, error) {
	var result []*model.Bead
	for _, id := range ids {
		if b, ok := m.beads[id]; ok {
			result = append(result, b)
		}
	}
	return result, nil
}

func (m *mockStore) GetStats(_ context.Context) (*model.GraphStats, error) {
	return &model.GraphStats{}, nil
}

func (m *mockStore) AddDependency(_ context.Context, dep *model.Dependency) error {
	m.deps[dep.BeadID] = append(m.deps[dep.BeadID], dep)
	return nil
}

func (m *mockStore) RemoveDependency(_ context.Context, _ string, _ string, _ model.DependencyType) error {
	return nil
}

func (m *mockStore) GetDependencies(_ context.Context, beadID string) ([]*model.Dependency, error) {
	return m.deps[beadID], nil
}

func (m *mockStore) GetReverseDependencies(_ context.Context, beadID string) ([]*model.Dependency, error) {
	var result []*model.Dependency
	for _, deps := range m.deps {
		for _, d := range deps {
			if d.DependsOnID == beadID {
				result = append(result, d)
			}
		}
	}
	return result, nil
}

func (m *mockStore) AddLabel(_ context.Context, beadID string, label string) error {
	m.labels[beadID] = append(m.labels[beadID], label)
	return nil
}

func (m *mockStore) RemoveLabel(_ context.Context, _ string, _ string) error {
	return nil
}

func (m *mockStore) GetLabels(_ context.Context, beadID string) ([]string, error) {
	return m.labels[beadID], nil
}

func (m *mockStore) AddComment(_ context.Context, comment *model.Comment) error {
	m.comments[comment.BeadID] = append(m.comments[comment.BeadID], comment)
	return nil
}

func (m *mockStore) GetComments(_ context.Context, beadID string) ([]*model.Comment, error) {
	return m.comments[beadID], nil
}

func (m *mockStore) RecordEvent(_ context.Context, _ *model.Event) error {
	return nil
}

func (m *mockStore) GetEvents(_ context.Context, _ string) ([]*model.Event, error) {
	return nil, nil
}

func (m *mockStore) SetConfig(_ context.Context, config *model.Config) error {
	m.configs[config.Key] = config
	return nil
}

func (m *mockStore) GetConfig(_ context.Context, key string) (*model.Config, error) {
	c, ok := m.configs[key]
	if !ok {
		return nil, sql.ErrNoRows
	}
	return c, nil
}

func (m *mockStore) ListConfigs(_ context.Context, namespace string) ([]*model.Config, error) {
	prefix := namespace + ":"
	var result []*model.Config
	for k, c := range m.configs {
		if strings.HasPrefix(k, prefix) {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *mockStore) ListAllConfigs(_ context.Context) ([]*model.Config, error) {
	var result []*model.Config
	for _, c := range m.configs {
		result = append(result, c)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Key < result[j].Key
	})
	return result, nil
}

func (m *mockStore) DeleteConfig(_ context.Context, key string) error {
	delete(m.configs, key)
	return nil
}

func (m *mockStore) RunInTransaction(_ context.Context, fn func(tx store.Store) error) error {
	return fn(m)
}

func (m *mockStore) Close() error {
	return nil
}

func (m *mockStore) UpsertGate(_ context.Context, _, _ string) error { return nil }

func (m *mockStore) MarkGateSatisfied(_ context.Context, _, _ string) error { return nil }

func (m *mockStore) ClearGate(_ context.Context, _, _ string) error { return nil }

func (m *mockStore) IsGateSatisfied(_ context.Context, _, _ string) (bool, error) {
	return false, nil
}

func (m *mockStore) ListGates(_ context.Context, _ string) ([]model.GateRow, error) {
	return nil, nil
}
