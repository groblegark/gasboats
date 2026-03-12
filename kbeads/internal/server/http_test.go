package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/groblegark/kbeads/internal/events"
	"github.com/groblegark/kbeads/internal/model"
	"github.com/groblegark/kbeads/internal/store"
)

type mockStore struct {
	beads         map[string]*model.Bead
	events        []*model.Event
	deps          map[string][]*model.Dependency
	labels        map[string][]string
	comments      map[string][]*model.Comment
	commentNextID int64

	// addLabelErr, when non-nil, is returned by AddLabel (for testing rollback).
	addLabelErr error
}

func newMockStore() *mockStore {
	return &mockStore{
		beads:    make(map[string]*model.Bead),
		deps:     make(map[string][]*model.Dependency),
		labels:   make(map[string][]string),
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
		return nil, nil
	}
	// Clone and attach labels so callers see the latest label state.
	clone := *b
	clone.Labels = m.labels[id]
	return &clone, nil
}

func (m *mockStore) ListBeads(_ context.Context, filter model.BeadFilter) ([]*model.Bead, int, error) {
	var result []*model.Bead
outer:
	for _, b := range m.beads {
		if len(filter.Status) > 0 {
			found := false
			for _, s := range filter.Status {
				if b.Status == s {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if len(filter.Type) > 0 {
			found := false
			for _, t := range filter.Type {
				if b.Type == t {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if len(filter.Kind) > 0 {
			found := false
			for _, k := range filter.Kind {
				if b.Kind == k {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if filter.Priority != nil && b.Priority != *filter.Priority {
			continue
		}
		if filter.Assignee != "" && b.Assignee != filter.Assignee {
			continue
		}
		if len(filter.Labels) > 0 {
			beadLabels := m.labels[b.ID]
			for _, want := range filter.Labels {
				found := false
				for _, have := range beadLabels {
					if have == want {
						found = true
						break
					}
				}
				if !found {
					continue outer
				}
			}
		}
		if filter.Search != "" {
			if !strings.Contains(strings.ToLower(b.Title), strings.ToLower(filter.Search)) &&
				!strings.Contains(strings.ToLower(b.Description), strings.ToLower(filter.Search)) {
				continue
			}
		}
		if len(filter.Fields) > 0 {
			if b.Fields == nil {
				continue
			}
			var fm map[string]string
			if json.Unmarshal(b.Fields, &fm) != nil {
				continue
			}
			match := true
			for k, v := range filter.Fields {
				if fm[k] != v {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}
		result = append(result, b)
	}
	return result, len(result), nil
}

func (m *mockStore) UpdateBead(_ context.Context, bead *model.Bead) error {
	m.beads[bead.ID] = bead
	return nil
}

func (m *mockStore) CloseBead(_ context.Context, id string, closedBy string) (*model.Bead, error) {
	b, ok := m.beads[id]
	if !ok {
		return nil, nil
	}
	now := time.Now().UTC()
	b.Status = model.StatusClosed
	b.ClosedAt = &now
	b.ClosedBy = closedBy
	b.UpdatedAt = now
	b.Labels = m.labels[id]
	return b, nil
}

func (m *mockStore) DeleteBead(_ context.Context, id string) error {
	if _, ok := m.beads[id]; !ok {
		return sql.ErrNoRows
	}
	delete(m.beads, id)
	delete(m.labels, id)
	return nil
}

func (m *mockStore) GetStats(_ context.Context) (*model.GraphStats, error) {
	stats := &model.GraphStats{}
	for _, b := range m.beads {
		switch b.Status {
		case model.StatusOpen:
			stats.TotalOpen++
		case model.StatusInProgress:
			stats.TotalInProgress++
		case model.StatusBlocked:
			stats.TotalBlocked++
		case model.StatusClosed:
			stats.TotalClosed++
		case model.StatusDeferred:
			stats.TotalDeferred++
		}
	}
	return stats, nil
}

func (m *mockStore) GetDependenciesForBeads(_ context.Context, beadIDs []string) (map[string][]*model.Dependency, error) {
	result := make(map[string][]*model.Dependency)
	idSet := make(map[string]bool, len(beadIDs))
	for _, id := range beadIDs {
		idSet[id] = true
	}
	for id, deps := range m.deps {
		if idSet[id] {
			result[id] = deps
		}
	}
	return result, nil
}

func (m *mockStore) GetReverseDependenciesForBeads(_ context.Context, beadIDs []string) (map[string][]*model.Dependency, error) {
	idSet := make(map[string]bool, len(beadIDs))
	for _, id := range beadIDs {
		idSet[id] = true
	}
	result := make(map[string][]*model.Dependency)
	for _, deps := range m.deps {
		for _, d := range deps {
			if idSet[d.DependsOnID] {
				result[d.DependsOnID] = append(result[d.DependsOnID], d)
			}
		}
	}
	return result, nil
}

func (m *mockStore) GetDependencyCounts(_ context.Context, beadIDs []string) (map[string]*model.DependencyCounts, error) {
	result := make(map[string]*model.DependencyCounts)
	for _, id := range beadIDs {
		result[id] = &model.DependencyCounts{
			DependencyCount: len(m.deps[id]),
		}
	}
	for _, deps := range m.deps {
		for _, d := range deps {
			if dc, ok := result[d.DependsOnID]; ok {
				dc.DependentCount++
			}
		}
	}
	return result, nil
}

func (m *mockStore) GetLabelsForBeads(_ context.Context, beadIDs []string) (map[string][]string, error) {
	result := make(map[string][]string)
	for _, id := range beadIDs {
		if labels, ok := m.labels[id]; ok {
			result[id] = labels
		}
	}
	return result, nil
}

func (m *mockStore) GetBlockedByForBeads(_ context.Context, beadIDs []string) (map[string][]string, error) {
	idSet := make(map[string]bool, len(beadIDs))
	for _, id := range beadIDs {
		idSet[id] = true
	}
	result := make(map[string][]string)
	for id, deps := range m.deps {
		if !idSet[id] {
			continue
		}
		for _, d := range deps {
			if d.Type == model.DepBlocks {
				result[id] = append(result[id], d.DependsOnID)
			}
		}
	}
	return result, nil
}

func (m *mockStore) GetBeadsByIDs(_ context.Context, ids []string) ([]*model.Bead, error) {
	var result []*model.Bead
	for _, id := range ids {
		if b, ok := m.beads[id]; ok {
			clone := *b
			clone.Labels = m.labels[id]
			result = append(result, &clone)
		}
	}
	return result, nil
}

func (m *mockStore) AddDependency(_ context.Context, dep *model.Dependency) error {
	m.deps[dep.BeadID] = append(m.deps[dep.BeadID], dep)
	return nil
}

func (m *mockStore) RemoveDependency(_ context.Context, beadID, dependsOnID string, depType model.DependencyType) error {
	deps := m.deps[beadID]
	for i, d := range deps {
		if d.DependsOnID == dependsOnID && d.Type == depType {
			m.deps[beadID] = append(deps[:i], deps[i+1:]...)
			return nil
		}
	}
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
	if m.addLabelErr != nil {
		return m.addLabelErr
	}
	// Skip duplicates (mirrors ON CONFLICT DO NOTHING).
	for _, l := range m.labels[beadID] {
		if l == label {
			return nil
		}
	}
	m.labels[beadID] = append(m.labels[beadID], label)
	return nil
}

func (m *mockStore) RemoveLabel(_ context.Context, beadID string, label string) error {
	labels := m.labels[beadID]
	for i, l := range labels {
		if l == label {
			m.labels[beadID] = append(labels[:i], labels[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockStore) GetLabels(_ context.Context, beadID string) ([]string, error) {
	return m.labels[beadID], nil
}

func (m *mockStore) AddComment(_ context.Context, comment *model.Comment) error {
	m.commentNextID++
	comment.ID = m.commentNextID
	m.comments[comment.BeadID] = append(m.comments[comment.BeadID], comment)
	return nil
}

func (m *mockStore) GetComments(_ context.Context, beadID string) ([]*model.Comment, error) {
	return m.comments[beadID], nil
}

func (m *mockStore) RecordEvent(_ context.Context, event *model.Event) error {
	event.ID = int64(len(m.events) + 1)
	m.events = append(m.events, event)
	return nil
}

func (m *mockStore) GetEvents(_ context.Context, beadID string) ([]*model.Event, error) {
	var result []*model.Event
	for _, e := range m.events {
		if e.BeadID == beadID {
			result = append(result, e)
		}
	}
	return result, nil
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

// newTestServer returns a fresh server, its mock store, and an HTTP handler.
func newTestServer() (*BeadsServer, *mockStore, http.Handler) {
	ms := newMockStore()
	s := NewBeadsServer(ms, &events.NoopPublisher{})
	return s, ms, s.NewHTTPHandler("")
}

// doJSON performs an HTTP request with an optional JSON body and returns the recorder.
func doJSON(t *testing.T, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body != nil {
		b, _ := json.Marshal(body)
		req = httptest.NewRequest(method, path, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// requireStatus asserts the recorder has the expected HTTP status code.
func requireStatus(t *testing.T, rec *httptest.ResponseRecorder, code int) {
	t.Helper()
	if rec.Code != code {
		t.Fatalf("expected status %d, got %d; body: %s", code, rec.Code, rec.Body.String())
	}
}

// decodeJSON decodes the recorder's response body into v.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
}

func TestHandleHTTPErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		method    string
		path      string
		body      any
		code      int
		wantError string
	}{
		{"CreateBead/MissingTitle", "POST", "/v1/beads", map[string]any{"type": "task"}, 400, "title is required"},
		{"GetBead/NotFound", "GET", "/v1/beads/nonexistent", nil, 404, "bead not found"},
		{"DeleteBead/NotFound", "DELETE", "/v1/beads/nonexistent", nil, 404, ""},
		{"AddComment/MissingText", "POST", "/v1/beads/kd-x/comments", map[string]any{"author": "alice"}, 400, ""},
		{"AddDependency/MissingDependsOnID", "POST", "/v1/beads/kd-a/dependencies", map[string]any{"type": "blocks"}, 400, ""},
		{"AddLabel/MissingLabel", "POST", "/v1/beads/kd-a/labels", map[string]any{}, 400, ""},
		{"RemoveDependency/MissingDependsOnID", "DELETE", "/v1/beads/kd-a/dependencies?type=blocks", nil, 400, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, h := newTestServer()
			rec := doJSON(t, h, tc.method, tc.path, tc.body)
			requireStatus(t, rec, tc.code)
			if tc.wantError != "" {
				var body map[string]string
				decodeJSON(t, rec, &body)
				if body["error"] != tc.wantError {
					t.Fatalf("expected error=%q, got %q", tc.wantError, body["error"])
				}
			}
		})
	}
}

func TestHandleHealth(t *testing.T) {
	_, _, h := newTestServer()
	rec := doJSON(t, h, "GET", "/v1/health", nil)
	requireStatus(t, rec, 200)
	var body map[string]string
	decodeJSON(t, rec, &body)
	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %q", body["status"])
	}
}

func TestHandleCreateBead(t *testing.T) {
	_, _, h := newTestServer()
	rec := doJSON(t, h, "POST", "/v1/beads", map[string]any{"title": "Test bead", "type": "task"})
	requireStatus(t, rec, 201)
	var bead model.Bead
	decodeJSON(t, rec, &bead)
	if bead.ID == "" {
		t.Fatal("expected bead to have an ID")
	}
	if bead.Title != "Test bead" || bead.Status != model.StatusOpen || bead.Kind != model.KindIssue {
		t.Fatalf("got title=%q status=%q kind=%q", bead.Title, bead.Status, bead.Kind)
	}
}

func TestHandleListBeads(t *testing.T) {
	_, ms, h := newTestServer()
	ms.beads["kd-abc123"] = &model.Bead{ID: "kd-abc123", Title: "Bead one", Status: model.StatusOpen}
	ms.beads["kd-def456"] = &model.Bead{ID: "kd-def456", Title: "Bead two", Status: model.StatusOpen}

	rec := doJSON(t, h, "GET", "/v1/beads", nil)
	requireStatus(t, rec, 200)
	var result struct {
		Beads []model.Bead `json:"beads"`
		Total int          `json:"total"`
	}
	decodeJSON(t, rec, &result)
	if result.Total != 2 || len(result.Beads) != 2 {
		t.Fatalf("expected 2 beads, got total=%d len=%d", result.Total, len(result.Beads))
	}
}

func TestHandleListBeads_WithFilters(t *testing.T) {
	_, ms, h := newTestServer()
	ms.beads["kd-f1"] = &model.Bead{ID: "kd-f1", Title: "Open test bead", Kind: model.KindIssue, Type: model.TypeTask, Status: model.StatusOpen, Assignee: "alice"}
	ms.beads["kd-f2"] = &model.Bead{ID: "kd-f2", Title: "Closed bead", Kind: model.KindIssue, Type: model.TypeTask, Status: model.StatusClosed, Assignee: "alice"}

	rec := doJSON(t, h, "GET", "/v1/beads?status=open&type=task&kind=issue&limit=10&offset=0&sort=-priority&assignee=alice&search=test", nil)
	requireStatus(t, rec, 200)
	var result struct {
		Beads []model.Bead `json:"beads"`
		Total int          `json:"total"`
	}
	decodeJSON(t, rec, &result)
	if result.Total != 1 {
		t.Fatalf("expected total=1 (only open matching), got %d", result.Total)
	}
}

func TestHandleGetBead(t *testing.T) {
	_, ms, h := newTestServer()
	ms.beads["kd-test1"] = &model.Bead{ID: "kd-test1", Title: "Test bead", Status: model.StatusOpen}

	rec := doJSON(t, h, "GET", "/v1/beads/kd-test1", nil)
	requireStatus(t, rec, 200)
	var bead model.Bead
	decodeJSON(t, rec, &bead)
	if bead.ID != "kd-test1" || bead.Title != "Test bead" {
		t.Fatalf("got id=%q title=%q", bead.ID, bead.Title)
	}
}

func TestHandleGetDependencies(t *testing.T) {
	_, ms, h := newTestServer()
	ms.deps["kd-dep1"] = []*model.Dependency{
		{BeadID: "kd-dep1", DependsOnID: "kd-dep2", Type: "blocks"},
		{BeadID: "kd-dep1", DependsOnID: "kd-dep3", Type: "related"},
	}
	rec := doJSON(t, h, "GET", "/v1/beads/kd-dep1/dependencies", nil)
	requireStatus(t, rec, 200)
	var result struct {
		Dependencies []model.Dependency `json:"dependencies"`
	}
	decodeJSON(t, rec, &result)
	if len(result.Dependencies) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(result.Dependencies))
	}
}

func TestHandleGetLabels(t *testing.T) {
	_, ms, h := newTestServer()
	ms.labels["kd-lbl1"] = []string{"urgent", "frontend"}

	rec := doJSON(t, h, "GET", "/v1/beads/kd-lbl1/labels", nil)
	requireStatus(t, rec, 200)
	var result struct {
		Labels []string `json:"labels"`
	}
	decodeJSON(t, rec, &result)
	if len(result.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(result.Labels))
	}
}

func TestHandleGetComments(t *testing.T) {
	_, ms, h := newTestServer()
	ms.comments["kd-cmt1"] = []*model.Comment{
		{ID: 1, BeadID: "kd-cmt1", Author: "alice", Text: "First"},
		{ID: 2, BeadID: "kd-cmt1", Author: "bob", Text: "Second"},
	}
	rec := doJSON(t, h, "GET", "/v1/beads/kd-cmt1/comments", nil)
	requireStatus(t, rec, 200)
	var result struct {
		Comments []model.Comment `json:"comments"`
	}
	decodeJSON(t, rec, &result)
	if len(result.Comments) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(result.Comments))
	}
}

func TestHandleGetEvents(t *testing.T) {
	_, ms, h := newTestServer()
	ms.events = []*model.Event{
		{ID: 1, Topic: "beads.bead.created", BeadID: "kd-abc123", Actor: "alice", Payload: json.RawMessage(`{}`)},
		{ID: 2, Topic: "beads.label.added", BeadID: "kd-abc123", Payload: json.RawMessage(`{}`)},
		{ID: 3, Topic: "beads.bead.created", BeadID: "kd-other", Payload: json.RawMessage(`{}`)},
	}
	rec := doJSON(t, h, "GET", "/v1/beads/kd-abc123/events", nil)
	requireStatus(t, rec, 200)
	var result struct {
		Events []model.Event `json:"events"`
	}
	decodeJSON(t, rec, &result)
	if len(result.Events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(result.Events))
	}
}

func TestHandleEmptyLists(t *testing.T) {
	_, _, h := newTestServer()
	for _, tc := range []struct {
		name string
		path string
	}{
		{"Dependencies", "/v1/beads/kd-nope/dependencies"},
		{"Labels", "/v1/beads/kd-nope/labels"},
		{"Comments", "/v1/beads/kd-nope/comments"},
		{"Events", "/v1/beads/kd-nonexistent/events"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doJSON(t, h, "GET", tc.path, nil)
			requireStatus(t, rec, 200)
		})
	}
}

func TestHandleAddComment_Response(t *testing.T) {
	_, ms, h := newTestServer()
	ms.beads["kd-cmt2"] = &model.Bead{ID: "kd-cmt2", Title: "Bead for comment", Status: model.StatusOpen}

	rec := doJSON(t, h, "POST", "/v1/beads/kd-cmt2/comments", map[string]any{"author": "alice", "text": "A new comment"})
	requireStatus(t, rec, 201)
	var comment model.Comment
	decodeJSON(t, rec, &comment)
	if comment.Text != "A new comment" || comment.Author != "alice" || comment.BeadID != "kd-cmt2" {
		t.Fatalf("got text=%q author=%q bead_id=%q", comment.Text, comment.Author, comment.BeadID)
	}
}
