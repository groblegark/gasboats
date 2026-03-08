package model

// GraphNode is a projection of a Bead for graph visualization.
// Lighter than a full Bead — only the fields the frontend needs.
type GraphNode struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	IssueType string   `json:"issue_type"`
	ParentID  string   `json:"parent_id,omitempty"`
	Assignee  string   `json:"assignee,omitempty"`
	Priority  int      `json:"priority"`
	Labels    []string `json:"labels,omitempty"`
	BlockedBy []string `json:"blocked_by,omitempty"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`

	// Populated only when IncludeBody is set.
	Description string `json:"description,omitempty"`
	Notes       string `json:"notes,omitempty"`

	DepCount   int  `json:"dep_count"`
	DepByCount int  `json:"dep_by_count"`
	Ephemeral  bool `json:"ephemeral,omitempty"`
}

// GraphArgs are the request parameters for the graph endpoint.
type GraphArgs struct {
	Status       []string `json:"status,omitempty"`
	ExcludeTypes []string `json:"exclude_types,omitempty"`
	Types        []string `json:"types,omitempty"`
	Labels       []string `json:"labels,omitempty"`
	LabelsAny    []string `json:"labels_any,omitempty"`
	Priority     *int     `json:"priority,omitempty"`
	PriorityMin  *int     `json:"priority_min,omitempty"`
	PriorityMax  *int     `json:"priority_max,omitempty"`
	ParentID     string   `json:"parent_id,omitempty"`
	Assignee     string   `json:"assignee,omitempty"`
	Query        string   `json:"query,omitempty"`
	MaxAgeDays   int      `json:"max_age_days,omitempty"`
	Limit        int      `json:"limit,omitempty"`

	IncludeDeps            bool `json:"include_deps,omitempty"`
	IncludeBody            bool `json:"include_body,omitempty"`
	IncludeAgents          bool `json:"include_agents,omitempty"`
	IncludeConnectedClosed *bool `json:"include_connected_closed,omitempty"`
}

// DependencyCounts holds forward and reverse dependency counts for a bead.
type DependencyCounts struct {
	DependencyCount int `json:"dependency_count"`
	DependentCount  int `json:"dependent_count"`
}

// GraphEdge represents a dependency relationship as a graph edge.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Type   string `json:"type"`
}

// GraphStats holds aggregate bead counts by status.
type GraphStats struct {
	TotalOpen       int `json:"total_open"`
	TotalInProgress int `json:"total_in_progress"`
	TotalBlocked    int `json:"total_blocked"`
	TotalClosed     int `json:"total_closed"`
	TotalDeferred   int `json:"total_deferred"`
}

// GraphResponse is the response for the graph visualization endpoint.
type GraphResponse struct {
	Nodes []GraphNode  `json:"nodes"`
	Edges []*GraphEdge `json:"edges"`
	Stats *GraphStats  `json:"stats"`
}
