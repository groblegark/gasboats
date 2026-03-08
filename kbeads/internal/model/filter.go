package model

import "time"

// BeadFilter holds criteria for querying beads.
type BeadFilter struct {
	Status      []Status   `json:"status,omitempty"`
	Type        []BeadType `json:"type,omitempty"`
	ExcludeTypes []BeadType `json:"exclude_types,omitempty"`
	Kind        []Kind     `json:"kind,omitempty"`
	Priority    *int       `json:"priority,omitempty"`
	PriorityMin *int       `json:"priority_min,omitempty"`
	PriorityMax *int       `json:"priority_max,omitempty"`
	Assignee    string     `json:"assignee,omitempty"`
	Labels      []string   `json:"labels,omitempty"`
	LabelsAny   []string   `json:"labels_any,omitempty"`
	Search      string            `json:"search,omitempty"` // full-text search on title/description
	Fields      map[string]string `json:"fields,omitempty"` // custom field key=value filters (JSONB)
	NoOpenDeps  bool       `json:"no_open_deps,omitempty"` // only beads with no open/in_progress/deferred dependencies
	IDs         []string   `json:"ids,omitempty"`          // fetch specific bead IDs
	UpdatedAfter *time.Time `json:"updated_after,omitempty"`
	ParentID    string     `json:"parent_id,omitempty"`
	Sort        string            `json:"sort,omitempty"`   // e.g. "-priority", "created_at"; prefix "-" = descending
	Limit       int        `json:"limit,omitempty"`
	Offset      int        `json:"offset,omitempty"`
}
