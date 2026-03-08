// Package client provides a transport-agnostic interface for the kbeads service
// and an HTTP/JSON implementation that talks to the kbeads REST API.
package client

import (
	"context"
	"encoding/json"
	"time"

	"github.com/groblegark/kbeads/internal/model"
)

// BeadsClient is the interface that all kbeads CLI commands use to communicate
// with the beads server. It is implemented by HTTPClient (default) and can be
// backed by any transport.
type BeadsClient interface {
	// Bead CRUD
	CreateBead(ctx context.Context, req *CreateBeadRequest) (*model.Bead, error)
	GetBead(ctx context.Context, id string) (*model.Bead, error)
	ListBeads(ctx context.Context, req *ListBeadsRequest) (*ListBeadsResponse, error)
	UpdateBead(ctx context.Context, id string, req *UpdateBeadRequest) (*model.Bead, error)
	CloseBead(ctx context.Context, id string, closedBy string) (*model.Bead, error)
	DeleteBead(ctx context.Context, id string) error

	// Dependencies
	AddDependency(ctx context.Context, req *AddDependencyRequest) (*model.Dependency, error)
	RemoveDependency(ctx context.Context, beadID, dependsOnID, depType string) error
	GetDependencies(ctx context.Context, beadID string) ([]*model.Dependency, error)
	GetReverseDependencies(ctx context.Context, beadID string) ([]*model.Dependency, error)

	// Labels
	AddLabel(ctx context.Context, beadID, label string) (*model.Bead, error)
	RemoveLabel(ctx context.Context, beadID, label string) error
	GetLabels(ctx context.Context, beadID string) ([]string, error)

	// Comments
	AddComment(ctx context.Context, beadID, author, text string) (*model.Comment, error)
	GetComments(ctx context.Context, beadID string) ([]*model.Comment, error)

	// Events
	GetEvents(ctx context.Context, beadID string) ([]*model.Event, error)

	// Config
	SetConfig(ctx context.Context, key string, value json.RawMessage) (*model.Config, error)
	GetConfig(ctx context.Context, key string) (*model.Config, error)
	ListConfigs(ctx context.Context, namespace string) ([]*model.Config, error)
	DeleteConfig(ctx context.Context, key string) error

	// Hooks
	EmitHook(ctx context.Context, req *EmitHookRequest) (*EmitHookResponse, error)

	// Gates
	ListGates(ctx context.Context, agentBeadID string) ([]model.GateRow, error)
	SatisfyGate(ctx context.Context, agentBeadID, gateID string) error
	ClearGate(ctx context.Context, agentBeadID, gateID string) error

	// Roster
	GetAgentRoster(ctx context.Context, staleThresholdSecs int) (*AgentRosterResponse, error)

	// Health
	Health(ctx context.Context) (string, error)

	// Lifecycle
	Close() error
}

// EmitHookRequest holds parameters for emitting a hook event.
type EmitHookRequest struct {
	AgentBeadID     string `json:"agent_bead_id"`
	HookType        string `json:"hook_type"`
	ClaudeSessionID string `json:"claude_session_id,omitempty"`
	CWD             string `json:"cwd,omitempty"`
	Actor           string `json:"actor,omitempty"`
	ToolName        string `json:"tool_name,omitempty"` // tool name from Claude Code (e.g. "Bash", "Read")
}

// EmitHookResponse is the response from EmitHook.
type EmitHookResponse struct {
	Block    bool     `json:"block,omitempty"`
	Reason   string   `json:"reason,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Inject   string   `json:"inject,omitempty"`
}

// AgentRosterResponse is the response from GetAgentRoster.
type AgentRosterResponse struct {
	Actors         []RosterEntry  `json:"actors"`
	UnclaimedTasks []UnclaimedTask `json:"unclaimed_tasks"`
}

// RosterEntry represents one agent in the roster.
type RosterEntry struct {
	Actor     string  `json:"actor"`
	TaskID    string  `json:"task_id,omitempty"`
	TaskTitle string  `json:"task_title,omitempty"`
	EpicID    string  `json:"epic_id,omitempty"`
	EpicTitle string  `json:"epic_title,omitempty"`
	IdleSecs  float64 `json:"idle_secs"`
	LastEvent string  `json:"last_event,omitempty"`
	ToolName  string  `json:"tool_name,omitempty"`
	SessionID string  `json:"session_id,omitempty"`
	CWD       string  `json:"cwd,omitempty"`
	Reaped    bool    `json:"reaped,omitempty"`
}

// UnclaimedTask is an in_progress bead with no assignee.
type UnclaimedTask struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Priority int    `json:"priority"`
}

// CreateBeadRequest holds parameters for creating a bead.
type CreateBeadRequest struct {
	Title       string          `json:"title"`
	Kind        string          `json:"kind,omitempty"`
	Type        string          `json:"type"`
	Description string          `json:"description,omitempty"`
	Notes       string          `json:"notes,omitempty"`
	Priority    int             `json:"priority"`
	Assignee    string          `json:"assignee,omitempty"`
	Owner       string          `json:"owner,omitempty"`
	Labels      []string        `json:"labels,omitempty"`
	CreatedBy   string          `json:"created_by,omitempty"`
	Fields      json.RawMessage `json:"fields,omitempty"`
	DueAt       *time.Time      `json:"due_at,omitempty"`
	DeferUntil  *time.Time      `json:"defer_until,omitempty"`
}

// ListBeadsRequest holds parameters for listing beads.
type ListBeadsRequest struct {
	Status       []string          `json:"status,omitempty"`
	Type         []string          `json:"type,omitempty"`
	Kind         []string          `json:"kind,omitempty"`
	Labels       []string          `json:"labels,omitempty"`
	Assignee     string            `json:"assignee,omitempty"`
	Search       string            `json:"search,omitempty"`
	Sort         string            `json:"sort,omitempty"`
	Priority     *int              `json:"priority,omitempty"`
	NoOpenDeps   bool              `json:"no_open_deps,omitempty"` // only beads with no open/in_progress/deferred dependencies
	Limit        int               `json:"limit,omitempty"`
	Offset       int               `json:"offset,omitempty"`
	FieldFilters map[string]string `json:"field_filters,omitempty"`
}

// ListBeadsResponse is the response from ListBeads.
type ListBeadsResponse struct {
	Beads []*model.Bead `json:"beads"`
	Total int           `json:"total"`
}

// UpdateBeadRequest holds optional parameters for updating a bead.
// Nil pointer fields mean "don't change".
type UpdateBeadRequest struct {
	Title       *string         `json:"title,omitempty"`
	Description *string         `json:"description,omitempty"`
	Notes       *string         `json:"notes,omitempty"`
	Status      *string         `json:"status,omitempty"`
	Priority    *int            `json:"priority,omitempty"`
	Assignee    *string         `json:"assignee,omitempty"`
	Owner       *string         `json:"owner,omitempty"`
	DueAt       *time.Time      `json:"due_at,omitempty"`
	DeferUntil  *time.Time      `json:"defer_until,omitempty"`
	Fields      json.RawMessage `json:"fields,omitempty"`
	Labels      []string        `json:"labels,omitempty"`
}

// AddDependencyRequest holds parameters for adding a dependency.
type AddDependencyRequest struct {
	BeadID      string `json:"bead_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"`
	CreatedBy   string `json:"created_by,omitempty"`
}
