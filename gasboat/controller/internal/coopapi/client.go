// Package coopapi provides an HTTP client for the coop REST API.
//
// Coop is the terminal session manager that wraps Claude Code.
// It exposes a REST API on localhost:8080 for agent state, screen
// content, input injection, nudging, and shutdown.
package coopapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client talks to the coop HTTP API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New creates a coop API client. baseURL is typically "http://localhost:8080".
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// AgentState is the response from GET /api/v1/agent.
type AgentState struct {
	State         string       `json:"state"`
	ErrorCategory string       `json:"error_category,omitempty"`
	LastMessage   string       `json:"last_message,omitempty"`
	Prompt        *AgentPrompt `json:"prompt,omitempty"`
}

// AgentPrompt describes a pending prompt from the agent.
type AgentPrompt struct {
	Type    string `json:"type,omitempty"`
	Subtype string `json:"subtype,omitempty"`
}

// NudgeResponse is the response from POST /api/v1/agent/nudge.
type NudgeResponse struct {
	Delivered bool   `json:"delivered"`
	Reason    string `json:"reason,omitempty"`
}

// GetAgentState returns the current agent state.
func (c *Client) GetAgentState(ctx context.Context) (*AgentState, error) {
	var state AgentState
	if err := c.getJSON(ctx, "/api/v1/agent", &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// GetScreenText returns the current screen content as plain text.
func (c *Client) GetScreenText(ctx context.Context) (string, error) {
	return c.getText(ctx, "/api/v1/screen/text")
}

// GetScreen returns the current screen content (may include formatting).
func (c *Client) GetScreen(ctx context.Context) (string, error) {
	return c.getText(ctx, "/api/v1/screen")
}

// SendKeys sends keyboard input to the agent.
func (c *Client) SendKeys(ctx context.Context, keys []string) error {
	body := map[string][]string{"keys": keys}
	return c.postJSON(ctx, "/api/v1/input/keys", body, nil)
}

// Respond selects an option on a prompt dialog.
func (c *Client) Respond(ctx context.Context, option int) error {
	body := map[string]int{"option": option}
	return c.postJSON(ctx, "/api/v1/agent/respond", body, nil)
}

// Nudge injects a message into the agent's input.
func (c *Client) Nudge(ctx context.Context, message string) (*NudgeResponse, error) {
	body := map[string]string{"message": message}
	var resp NudgeResponse
	if err := c.postJSON(ctx, "/api/v1/agent/nudge", body, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Shutdown requests a graceful coop shutdown.
func (c *Client) Shutdown(ctx context.Context) error {
	return c.postJSON(ctx, "/api/v1/shutdown", nil, nil)
}

// getJSON performs a GET request and decodes the JSON response.
func (c *Client) getJSON(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, path)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// getText performs a GET request and returns the body as a string.
func (c *Client) getText(ctx context.Context, path string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return "", fmt.Errorf("creating request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, path)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading response: %w", err)
	}
	return string(data), nil
}

// postJSON performs a POST request with a JSON body and optionally decodes the response.
func (c *Client) postJSON(ctx context.Context, path string, body interface{}, out interface{}) error {
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling body: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, path)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
