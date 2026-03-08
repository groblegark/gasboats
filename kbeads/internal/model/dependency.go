package model

import "time"

// DependencyType categorizes the relationship between two beads.
// Well-known constants are provided below, but dependency types are extensible.
type DependencyType string

const (
	DepBlocks      DependencyType = "blocks"
	DepParentChild DependencyType = "parent-child"
	DepRelated     DependencyType = "related"
	DepDuplicates  DependencyType = "duplicates"
	DepSupersedes  DependencyType = "supersedes"
)

// IsValid reports whether the dependency type is a non-empty string of at most 50 characters.
// Dependency types are extensible, so any non-empty value within the length limit is accepted.
func (d DependencyType) IsValid() bool {
	return len(d) > 0 && len(d) <= 50
}

// Dependency represents a directional relationship between two beads.
type Dependency struct {
	BeadID      string         `json:"bead_id"`
	DependsOnID string         `json:"depends_on_id"`
	Type        DependencyType `json:"type"`
	CreatedAt   time.Time      `json:"created_at"`
	CreatedBy   string         `json:"created_by,omitempty"`
	Metadata    string         `json:"metadata,omitempty"`
}
