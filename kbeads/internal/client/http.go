package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/groblegark/kbeads/internal/model"
)

// HTTPClient implements BeadsClient using the kbeads HTTP/JSON REST API.
type HTTPClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewHTTPClient creates a new HTTP client targeting the given base URL
// (e.g. "http://localhost:8080"). When token is non-empty, an Authorization
// header is set on every request.
func NewHTTPClient(baseURL, token string) *HTTPClient {
	return &HTTPClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{},
	}
}

// Close is a no-op for the HTTP client.
func (c *HTTPClient) Close() error { return nil }

// --- Bead CRUD ---

func (c *HTTPClient) CreateBead(ctx context.Context, req *CreateBeadRequest) (*model.Bead, error) {
	var bead model.Bead
	if err := c.doJSON(ctx, http.MethodPost, "/v1/beads", req, &bead); err != nil {
		return nil, err
	}
	return &bead, nil
}

func (c *HTTPClient) GetBead(ctx context.Context, id string) (*model.Bead, error) {
	var bead model.Bead
	if err := c.doJSON(ctx, http.MethodGet, "/v1/beads/"+url.PathEscape(id), nil, &bead); err != nil {
		return nil, err
	}
	return &bead, nil
}

func (c *HTTPClient) ListBeads(ctx context.Context, req *ListBeadsRequest) (*ListBeadsResponse, error) {
	q := url.Values{}
	if len(req.Status) > 0 {
		q.Set("status", strings.Join(req.Status, ","))
	}
	if len(req.Type) > 0 {
		q.Set("type", strings.Join(req.Type, ","))
	}
	if len(req.Kind) > 0 {
		q.Set("kind", strings.Join(req.Kind, ","))
	}
	if len(req.Labels) > 0 {
		q.Set("labels", strings.Join(req.Labels, ","))
	}
	if req.Assignee != "" {
		q.Set("assignee", req.Assignee)
	}
	if req.Search != "" {
		q.Set("search", req.Search)
	}
	if req.Sort != "" {
		q.Set("sort", req.Sort)
	}
	if req.Priority != nil {
		q.Set("priority", fmt.Sprintf("%d", *req.Priority))
	}
	if req.NoOpenDeps {
		q.Set("no_open_deps", "true")
	}
	if req.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", req.Limit))
	}
	if req.Offset > 0 {
		q.Set("offset", fmt.Sprintf("%d", req.Offset))
	}
	for k, v := range req.FieldFilters {
		q.Add("field", k+"="+v)
	}

	path := "/v1/beads"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}

	var resp ListBeadsResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *HTTPClient) UpdateBead(ctx context.Context, id string, req *UpdateBeadRequest) (*model.Bead, error) {
	var bead model.Bead
	if err := c.doJSON(ctx, http.MethodPatch, "/v1/beads/"+url.PathEscape(id), req, &bead); err != nil {
		return nil, err
	}
	return &bead, nil
}

func (c *HTTPClient) CloseBead(ctx context.Context, id string, closedBy string) (*model.Bead, error) {
	body := map[string]string{}
	if closedBy != "" {
		body["closed_by"] = closedBy
	}
	var bead model.Bead
	if err := c.doJSON(ctx, http.MethodPost, "/v1/beads/"+url.PathEscape(id)+"/close", body, &bead); err != nil {
		return nil, err
	}
	return &bead, nil
}

func (c *HTTPClient) DeleteBead(ctx context.Context, id string) error {
	return c.doJSON(ctx, http.MethodDelete, "/v1/beads/"+url.PathEscape(id), nil, nil)
}

// --- Dependencies ---

func (c *HTTPClient) AddDependency(ctx context.Context, req *AddDependencyRequest) (*model.Dependency, error) {
	body := map[string]string{
		"depends_on_id": req.DependsOnID,
		"type":          req.Type,
		"created_by":    req.CreatedBy,
	}
	var dep model.Dependency
	if err := c.doJSON(ctx, http.MethodPost, "/v1/beads/"+url.PathEscape(req.BeadID)+"/dependencies", body, &dep); err != nil {
		return nil, err
	}
	return &dep, nil
}

func (c *HTTPClient) RemoveDependency(ctx context.Context, beadID, dependsOnID, depType string) error {
	q := url.Values{}
	q.Set("depends_on_id", dependsOnID)
	if depType != "" {
		q.Set("type", depType)
	}
	path := "/v1/beads/" + url.PathEscape(beadID) + "/dependencies?" + q.Encode()
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil)
}

func (c *HTTPClient) GetDependencies(ctx context.Context, beadID string) ([]*model.Dependency, error) {
	var resp struct {
		Dependencies []*model.Dependency `json:"dependencies"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/beads/"+url.PathEscape(beadID)+"/dependencies", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Dependencies, nil
}

func (c *HTTPClient) GetReverseDependencies(ctx context.Context, beadID string) ([]*model.Dependency, error) {
	var resp struct {
		Dependencies []*model.Dependency `json:"dependencies"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/beads/"+url.PathEscape(beadID)+"/dependents", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Dependencies, nil
}

// --- Labels ---

func (c *HTTPClient) AddLabel(ctx context.Context, beadID, label string) (*model.Bead, error) {
	body := map[string]string{"label": label}
	var bead model.Bead
	if err := c.doJSON(ctx, http.MethodPost, "/v1/beads/"+url.PathEscape(beadID)+"/labels", body, &bead); err != nil {
		return nil, err
	}
	return &bead, nil
}

func (c *HTTPClient) RemoveLabel(ctx context.Context, beadID, label string) error {
	return c.doJSON(ctx, http.MethodDelete, "/v1/beads/"+url.PathEscape(beadID)+"/labels/"+url.PathEscape(label), nil, nil)
}

func (c *HTTPClient) GetLabels(ctx context.Context, beadID string) ([]string, error) {
	var resp struct {
		Labels []string `json:"labels"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/beads/"+url.PathEscape(beadID)+"/labels", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Labels, nil
}

// --- Comments ---

func (c *HTTPClient) AddComment(ctx context.Context, beadID, author, text string) (*model.Comment, error) {
	body := map[string]string{"author": author, "text": text}
	var comment model.Comment
	if err := c.doJSON(ctx, http.MethodPost, "/v1/beads/"+url.PathEscape(beadID)+"/comments", body, &comment); err != nil {
		return nil, err
	}
	return &comment, nil
}

func (c *HTTPClient) GetComments(ctx context.Context, beadID string) ([]*model.Comment, error) {
	var resp struct {
		Comments []*model.Comment `json:"comments"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/beads/"+url.PathEscape(beadID)+"/comments", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Comments, nil
}

// --- Events ---

func (c *HTTPClient) GetEvents(ctx context.Context, beadID string) ([]*model.Event, error) {
	var resp struct {
		Events []*model.Event `json:"events"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/beads/"+url.PathEscape(beadID)+"/events", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Events, nil
}

// --- Config ---

func (c *HTTPClient) SetConfig(ctx context.Context, key string, value json.RawMessage) (*model.Config, error) {
	body := map[string]json.RawMessage{"value": value}
	var config model.Config
	if err := c.doJSON(ctx, http.MethodPut, "/v1/configs/"+key, body, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (c *HTTPClient) GetConfig(ctx context.Context, key string) (*model.Config, error) {
	var config model.Config
	if err := c.doJSON(ctx, http.MethodGet, "/v1/configs/"+key, nil, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (c *HTTPClient) ListConfigs(ctx context.Context, namespace string) ([]*model.Config, error) {
	var resp struct {
		Configs []*model.Config `json:"configs"`
	}
	path := "/v1/configs?namespace=" + url.QueryEscape(namespace)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Configs, nil
}

func (c *HTTPClient) DeleteConfig(ctx context.Context, key string) error {
	return c.doJSON(ctx, http.MethodDelete, "/v1/configs/"+key, nil, nil)
}

// --- Hooks ---

func (c *HTTPClient) EmitHook(ctx context.Context, req *EmitHookRequest) (*EmitHookResponse, error) {
	var resp EmitHookResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/hooks/emit", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// --- Gates ---

func (c *HTTPClient) ListGates(ctx context.Context, agentBeadID string) ([]model.GateRow, error) {
	var gates []model.GateRow
	if err := c.doJSON(ctx, http.MethodGet, "/v1/agents/"+url.PathEscape(agentBeadID)+"/gates", nil, &gates); err != nil {
		return nil, err
	}
	return gates, nil
}

func (c *HTTPClient) SatisfyGate(ctx context.Context, agentBeadID, gateID string) error {
	path := "/v1/agents/" + url.PathEscape(agentBeadID) + "/gates/" + url.PathEscape(gateID) + "/satisfy"
	return c.doJSON(ctx, http.MethodPost, path, nil, nil)
}

func (c *HTTPClient) ClearGate(ctx context.Context, agentBeadID, gateID string) error {
	path := "/v1/agents/" + url.PathEscape(agentBeadID) + "/gates/" + url.PathEscape(gateID)
	return c.doJSON(ctx, http.MethodDelete, path, nil, nil)
}

// --- Roster ---

func (c *HTTPClient) GetAgentRoster(ctx context.Context, staleThresholdSecs int) (*AgentRosterResponse, error) {
	path := fmt.Sprintf("/v1/agents/roster?stale_threshold_secs=%d", staleThresholdSecs)
	var resp AgentRosterResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// --- Health ---

func (c *HTTPClient) Health(ctx context.Context) (string, error) {
	var resp struct {
		Status string `json:"status"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/v1/health", nil, &resp); err != nil {
		return "", err
	}
	return resp.Status, nil
}

// --- internal helpers ---

// APIError represents an error response from the server.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Message)
}

// doJSON performs an HTTP request with optional JSON body and decodes the JSON response.
// If result is nil, the response body is discarded (for DELETE/204 responses).
func (c *HTTPClient) doJSON(ctx context.Context, method, path string, body any, result any) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("performing request: %w", err)
	}
	defer resp.Body.Close()

	// 204 No Content — success with no body.
	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error != "" {
			return &APIError{StatusCode: resp.StatusCode, Message: errResp.Error}
		}
		return &APIError{StatusCode: resp.StatusCode, Message: string(respBody)}
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}

	return nil
}
