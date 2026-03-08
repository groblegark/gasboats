package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testHandler captures the incoming request details and returns a canned response.
type testHandler struct {
	// captured from the request
	method      string
	path        string
	rawPath     string // URL-encoded path (for testing PathEscape)
	requestURI  string
	query       string
	body        string
	contentType string

	// canned response
	statusCode   int
	responseBody string
}

func (h *testHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.method = r.Method
	h.path = r.URL.Path
	h.rawPath = r.URL.RawPath
	h.requestURI = r.RequestURI
	h.query = r.URL.RawQuery
	h.contentType = r.Header.Get("Content-Type")
	if r.Body != nil {
		data, _ := io.ReadAll(r.Body)
		h.body = string(data)
	}

	w.Header().Set("Content-Type", "application/json")
	if h.statusCode != 0 {
		w.WriteHeader(h.statusCode)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	if h.responseBody != "" {
		_, _ = w.Write([]byte(h.responseBody))
	}
}

// newTestClient creates an HTTPClient pointed at a test server with the given handler.
func newTestClient(h http.Handler) (*HTTPClient, *httptest.Server) {
	srv := httptest.NewServer(h)
	c := NewHTTPClient(srv.URL, "")
	return c, srv
}

// --- SetConfig ---

func TestHTTPClient_SetConfig(t *testing.T) {
	h := &testHandler{
		responseBody: `{
			"key": "view:inbox",
			"value": {"columns": ["title", "status"]},
			"created_at": "2026-01-15T10:00:00Z",
			"updated_at": "2026-01-15T10:00:00Z"
		}`,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	val := json.RawMessage(`{"columns": ["title", "status"]}`)
	cfg, err := c.SetConfig(context.Background(), "view:inbox", val)
	if err != nil {
		t.Fatalf("SetConfig() error = %v", err)
	}

	if h.method != http.MethodPut {
		t.Errorf("method = %q, want PUT", h.method)
	}
	if h.path != "/v1/configs/view:inbox" {
		t.Errorf("path = %q, want /v1/configs/view:inbox", h.path)
	}

	var reqBody map[string]json.RawMessage
	if err := json.Unmarshal([]byte(h.body), &reqBody); err != nil {
		t.Fatalf("unmarshaling request body: %v", err)
	}
	var gotVal, wantVal interface{}
	if err := json.Unmarshal(reqBody["value"], &gotVal); err != nil {
		t.Fatalf("unmarshaling request value: %v", err)
	}
	if err := json.Unmarshal([]byte(`{"columns": ["title", "status"]}`), &wantVal); err != nil {
		t.Fatalf("unmarshaling expected value: %v", err)
	}
	gotJSON, _ := json.Marshal(gotVal)
	wantJSON, _ := json.Marshal(wantVal)
	if string(gotJSON) != string(wantJSON) {
		t.Errorf("request body value = %s, want %s", gotJSON, wantJSON)
	}

	if cfg.Key != "view:inbox" {
		t.Errorf("cfg.Key = %q, want 'view:inbox'", cfg.Key)
	}
	if cfg.Value == nil {
		t.Error("cfg.Value is nil, want non-nil")
	}
}

// --- GetConfig ---

func TestHTTPClient_GetConfig(t *testing.T) {
	h := &testHandler{
		responseBody: `{
			"key": "type:decision",
			"value": {"fields": [{"name": "status", "type": "enum"}]},
			"created_at": "2026-01-15T10:00:00Z",
			"updated_at": "2026-01-16T10:00:00Z"
		}`,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	cfg, err := c.GetConfig(context.Background(), "type:decision")
	if err != nil {
		t.Fatalf("GetConfig() error = %v", err)
	}

	if h.method != http.MethodGet {
		t.Errorf("method = %q, want GET", h.method)
	}
	if h.path != "/v1/configs/type:decision" {
		t.Errorf("path = %q, want /v1/configs/type:decision", h.path)
	}

	if cfg.Key != "type:decision" {
		t.Errorf("cfg.Key = %q, want 'type:decision'", cfg.Key)
	}
}

// --- ListConfigs ---

func TestHTTPClient_ListConfigs(t *testing.T) {
	h := &testHandler{
		responseBody: `{
			"configs": [
				{"key": "view:inbox", "value": {}, "created_at": "2026-01-15T10:00:00Z", "updated_at": "2026-01-15T10:00:00Z"},
				{"key": "view:board", "value": {}, "created_at": "2026-01-15T10:00:00Z", "updated_at": "2026-01-15T10:00:00Z"}
			]
		}`,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	configs, err := c.ListConfigs(context.Background(), "view")
	if err != nil {
		t.Fatalf("ListConfigs() error = %v", err)
	}

	if h.method != http.MethodGet {
		t.Errorf("method = %q, want GET", h.method)
	}
	if h.path != "/v1/configs" {
		t.Errorf("path = %q, want /v1/configs", h.path)
	}
	if !strings.Contains(h.query, "namespace=view") {
		t.Errorf("query %q missing namespace=view", h.query)
	}

	if len(configs) != 2 {
		t.Fatalf("len(configs) = %d, want 2", len(configs))
	}
	if configs[0].Key != "view:inbox" {
		t.Errorf("configs[0].Key = %q, want 'view:inbox'", configs[0].Key)
	}
}

func TestHTTPClient_ListConfigs_Empty(t *testing.T) {
	h := &testHandler{
		responseBody: `{"configs": []}`,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	configs, err := c.ListConfigs(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("ListConfigs() error = %v", err)
	}
	if len(configs) != 0 {
		t.Errorf("len(configs) = %d, want 0", len(configs))
	}
}

// --- DeleteConfig ---

func TestHTTPClient_DeleteConfig(t *testing.T) {
	h := &testHandler{
		statusCode: http.StatusNoContent,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	err := c.DeleteConfig(context.Background(), "view:inbox")
	if err != nil {
		t.Fatalf("DeleteConfig() error = %v", err)
	}

	if h.method != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", h.method)
	}
	if h.path != "/v1/configs/view:inbox" {
		t.Errorf("path = %q, want /v1/configs/view:inbox", h.path)
	}
}

// --- Health ---

func TestHTTPClient_Health(t *testing.T) {
	h := &testHandler{
		responseBody: `{"status": "ok"}`,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	status, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}

	if h.method != http.MethodGet {
		t.Errorf("method = %q, want GET", h.method)
	}
	if h.path != "/v1/health" {
		t.Errorf("path = %q, want /v1/health", h.path)
	}

	if status != "ok" {
		t.Errorf("status = %q, want 'ok'", status)
	}
}

// --- Error handling ---

func TestHTTPClient_Error_JSONBody(t *testing.T) {
	h := &testHandler{
		statusCode:   http.StatusBadRequest,
		responseBody: `{"error": "bead title is required"}`,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	_, err := c.CreateBead(context.Background(), &CreateBeadRequest{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", apiErr.StatusCode)
	}
	if apiErr.Message != "bead title is required" {
		t.Errorf("message = %q, want 'bead title is required'", apiErr.Message)
	}
}

func TestHTTPClient_Error_NonJSONBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL, "")
	_, err := c.GetBead(context.Background(), "bead-123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", apiErr.StatusCode)
	}
	if apiErr.Message != "internal server error" {
		t.Errorf("message = %q, want 'internal server error'", apiErr.Message)
	}
}

func TestHTTPClient_Error_404(t *testing.T) {
	h := &testHandler{
		statusCode:   http.StatusNotFound,
		responseBody: `{"error": "bead not found"}`,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	_, err := c.GetBead(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", apiErr.StatusCode)
	}
	if apiErr.Message != "bead not found" {
		t.Errorf("message = %q, want 'bead not found'", apiErr.Message)
	}
}

func TestHTTPClient_Error_500(t *testing.T) {
	h := &testHandler{
		statusCode:   http.StatusInternalServerError,
		responseBody: `{"error": "database connection lost"}`,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	err := c.DeleteBead(context.Background(), "bead-123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", apiErr.StatusCode)
	}
}

func TestHTTPClient_Error_FormatString(t *testing.T) {
	apiErr := &APIError{StatusCode: 403, Message: "forbidden"}
	want := "HTTP 403: forbidden"
	if apiErr.Error() != want {
		t.Errorf("Error() = %q, want %q", apiErr.Error(), want)
	}
}

func TestHTTPClient_Error_EmptyJSONError(t *testing.T) {
	h := &testHandler{
		statusCode:   http.StatusUnprocessableEntity,
		responseBody: `{"error": ""}`,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	_, err := c.GetBead(context.Background(), "bead-123")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", apiErr.StatusCode)
	}
	if apiErr.Message != `{"error": ""}` {
		t.Errorf("message = %q, want raw body", apiErr.Message)
	}
}

func TestHTTPClient_Error_CanceledContext(t *testing.T) {
	h := &testHandler{
		responseBody: `{"status": "ok"}`,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Health(ctx)
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("error = %q, want to contain 'context canceled'", err.Error())
	}
}

// --- 204 No Content handling ---

func TestHTTPClient_204NoContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL, "")

	err := c.DeleteBead(context.Background(), "bead-del")
	if err != nil {
		t.Fatalf("DeleteBead() with 204 error = %v", err)
	}

	err = c.DeleteConfig(context.Background(), "some:key")
	if err != nil {
		t.Fatalf("DeleteConfig() with 204 error = %v", err)
	}

	err = c.RemoveLabel(context.Background(), "bead-x", "label")
	if err != nil {
		t.Fatalf("RemoveLabel() with 204 error = %v", err)
	}

	err = c.RemoveDependency(context.Background(), "bead-a", "bead-b", "blocks")
	if err != nil {
		t.Fatalf("RemoveDependency() with 204 error = %v", err)
	}
}

// --- Close ---

func TestHTTPClient_Close(t *testing.T) {
	c := NewHTTPClient("http://localhost:9999", "")
	if err := c.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
}

// --- NewHTTPClient base URL trimming ---

func TestNewHTTPClient_TrimsTrailingSlash(t *testing.T) {
	c := NewHTTPClient("http://localhost:8080/", "")
	if c.baseURL != "http://localhost:8080" {
		t.Errorf("baseURL = %q, want 'http://localhost:8080'", c.baseURL)
	}
}

func TestNewHTTPClient_NoTrailingSlash(t *testing.T) {
	c := NewHTTPClient("http://localhost:8080", "")
	if c.baseURL != "http://localhost:8080" {
		t.Errorf("baseURL = %q, want 'http://localhost:8080'", c.baseURL)
	}
}

// --- Interface compliance ---

func TestHTTPClient_ImplementsBeadsClient(t *testing.T) {
	var _ BeadsClient = (*HTTPClient)(nil)
}

// --- Concurrent requests ---

func TestHTTPClient_ConcurrentRequests(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status": "ok"}`))
	}))
	defer srv.Close()

	c := NewHTTPClient(srv.URL, "")

	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := c.Health(context.Background())
			errs <- err
		}()
	}

	for i := 0; i < 10; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent Health() error = %v", err)
		}
	}
}

// --- GetAgentRoster ---

func TestHTTPClient_GetAgentRoster(t *testing.T) {
	h := &testHandler{
		responseBody: `{
			"actors": [
				{
					"actor": "wise-newt",
					"task_id": "bd-abc12",
					"task_title": "Fix login bug",
					"idle_secs": 30.5,
					"last_event": "Bash",
					"session_id": "sess-1"
				},
				{
					"actor": "ripe-elk",
					"idle_secs": 120.0,
					"reaped": true
				}
			],
			"unclaimed_tasks": [
				{"id": "bd-xyz99", "title": "Write docs", "priority": 2}
			]
		}`,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	resp, err := c.GetAgentRoster(context.Background(), 600)
	if err != nil {
		t.Fatalf("GetAgentRoster() error = %v", err)
	}

	// Verify request.
	if h.method != http.MethodGet {
		t.Errorf("method = %q, want GET", h.method)
	}
	if h.path != "/v1/agents/roster" {
		t.Errorf("path = %q, want /v1/agents/roster", h.path)
	}
	if h.query != "stale_threshold_secs=600" {
		t.Errorf("query = %q, want stale_threshold_secs=600", h.query)
	}

	// Verify parsed response.
	if len(resp.Actors) != 2 {
		t.Fatalf("len(Actors) = %d, want 2", len(resp.Actors))
	}
	a0 := resp.Actors[0]
	if a0.Actor != "wise-newt" {
		t.Errorf("Actors[0].Actor = %q, want wise-newt", a0.Actor)
	}
	if a0.TaskID != "bd-abc12" {
		t.Errorf("Actors[0].TaskID = %q, want bd-abc12", a0.TaskID)
	}
	if a0.TaskTitle != "Fix login bug" {
		t.Errorf("Actors[0].TaskTitle = %q, want Fix login bug", a0.TaskTitle)
	}
	if a0.IdleSecs != 30.5 {
		t.Errorf("Actors[0].IdleSecs = %f, want 30.5", a0.IdleSecs)
	}
	if a0.LastEvent != "Bash" {
		t.Errorf("Actors[0].LastEvent = %q, want Bash", a0.LastEvent)
	}
	if a0.SessionID != "sess-1" {
		t.Errorf("Actors[0].SessionID = %q, want sess-1", a0.SessionID)
	}

	a1 := resp.Actors[1]
	if a1.Actor != "ripe-elk" {
		t.Errorf("Actors[1].Actor = %q, want ripe-elk", a1.Actor)
	}
	if !a1.Reaped {
		t.Error("Actors[1].Reaped = false, want true")
	}
	if a1.TaskID != "" {
		t.Errorf("Actors[1].TaskID = %q, want empty", a1.TaskID)
	}

	if len(resp.UnclaimedTasks) != 1 {
		t.Fatalf("len(UnclaimedTasks) = %d, want 1", len(resp.UnclaimedTasks))
	}
	ut := resp.UnclaimedTasks[0]
	if ut.ID != "bd-xyz99" || ut.Title != "Write docs" || ut.Priority != 2 {
		t.Errorf("UnclaimedTasks[0] = %+v, want {bd-xyz99, Write docs, 2}", ut)
	}
}

func TestHTTPClient_GetAgentRoster_Empty(t *testing.T) {
	h := &testHandler{
		responseBody: `{"actors": [], "unclaimed_tasks": []}`,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	resp, err := c.GetAgentRoster(context.Background(), 300)
	if err != nil {
		t.Fatalf("GetAgentRoster() error = %v", err)
	}
	if len(resp.Actors) != 0 {
		t.Errorf("len(Actors) = %d, want 0", len(resp.Actors))
	}
	if len(resp.UnclaimedTasks) != 0 {
		t.Errorf("len(UnclaimedTasks) = %d, want 0", len(resp.UnclaimedTasks))
	}
}

func TestHTTPClient_GetAgentRoster_ServerError(t *testing.T) {
	h := &testHandler{
		statusCode:   http.StatusInternalServerError,
		responseBody: `{"error": "database unavailable"}`,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	_, err := c.GetAgentRoster(context.Background(), 600)
	if err == nil {
		t.Fatal("GetAgentRoster() expected error for 500 response")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 500 {
		t.Errorf("APIError.StatusCode = %d, want 500", apiErr.StatusCode)
	}
}

func TestHTTPClient_GetAgentRoster_DefaultThreshold(t *testing.T) {
	h := &testHandler{
		responseBody: `{"actors": [], "unclaimed_tasks": []}`,
	}
	c, srv := newTestClient(h)
	defer srv.Close()

	_, err := c.GetAgentRoster(context.Background(), 0)
	if err != nil {
		t.Fatalf("GetAgentRoster() error = %v", err)
	}
	if h.query != "stale_threshold_secs=0" {
		t.Errorf("query = %q, want stale_threshold_secs=0", h.query)
	}
}
