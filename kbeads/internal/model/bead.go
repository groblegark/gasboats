package model

import (
	"encoding/json"
	"time"
)

// Kind is a two-level classification for beads.
type Kind string

const (
	KindIssue  Kind = "issue"
	KindData   Kind = "data"
	KindConfig Kind = "config"
)

// String returns the string representation of the kind.
func (k Kind) String() string {
	return string(k)
}

// IsValid checks whether the kind is a known value.
func (k Kind) IsValid() bool {
	switch k {
	case KindIssue, KindData, KindConfig:
		return true
	}
	return false
}

// BeadType categorizes the kind of bead.
// Well-known constants are provided below, but bead types are extensible;
// custom types (e.g. "message", "gate", "workflow") are valid.
type BeadType string

// Issue-kind types.
const (
	TypeEpic    BeadType = "epic"
	TypeTask    BeadType = "task"
	TypeFeature BeadType = "feature"
	TypeChore   BeadType = "chore"
	TypeBug     BeadType = "bug"
)

// Data-kind types.
const (
	TypeAdvice   BeadType = "advice"
	TypeJack     BeadType = "jack"
	TypeDecision BeadType = "decision"
	TypeReport   BeadType = "report"
	TypeFormula  BeadType = "formula"
)

// Molecule is an issue-kind type (instantiated formula, acts as an epic).
const TypeMolecule BeadType = "molecule"

// String returns the string representation of the bead type.
func (t BeadType) String() string {
	return string(t)
}

// IsValid reports whether the bead type is a non-empty string.
// Bead types are extensible, so any non-empty value is accepted.
func (t BeadType) IsValid() bool {
	return t != ""
}

// KindFor returns the expected kind for a given well-known bead type.
// For unknown types it returns an empty string; the caller decides how to handle that.
func KindFor(t BeadType) Kind {
	switch t {
	case TypeEpic, TypeTask, TypeFeature, TypeChore, TypeBug, TypeMolecule:
		return KindIssue
	case TypeAdvice, TypeJack, TypeFormula:
		return KindData
	default:
		return ""
	}
}

// TypeAliases maps deprecated type names to their canonical replacements.
// "template" → "formula", "bundle" → "molecule".
var TypeAliases = map[BeadType]BeadType{
	"template": TypeFormula,
	"bundle":   TypeMolecule,
}

// Status represents the current state of a bead.
type Status string

const (
	StatusOpen       Status = "open"
	StatusInProgress Status = "in_progress"
	StatusDeferred   Status = "deferred"
	StatusClosed     Status = "closed"
	StatusBlocked    Status = "blocked"
)

// String returns the string representation of the status.
func (s Status) String() string {
	return string(s)
}

// IsValid checks whether the status is a known value.
func (s Status) IsValid() bool {
	switch s {
	case StatusOpen, StatusInProgress, StatusDeferred, StatusClosed, StatusBlocked:
		return true
	}
	return false
}

// Bead is the core work-item record.
type Bead struct {
	ID                 string          `json:"id"`
	Slug               string          `json:"slug,omitempty"`
	Kind               Kind            `json:"kind"`
	Type               BeadType        `json:"type"`
	Title              string          `json:"title"`
	Description string `json:"description,omitempty"`
	Notes       string `json:"notes,omitempty"`
	Status             Status          `json:"status"`
	Priority           int             `json:"priority"`
	Assignee           string          `json:"assignee,omitempty"`
	Owner              string          `json:"owner,omitempty"`
	CreatedAt          time.Time       `json:"created_at"`
	CreatedBy          string          `json:"created_by,omitempty"`
	UpdatedAt          time.Time       `json:"updated_at"`
	ClosedAt           *time.Time      `json:"closed_at,omitempty"`
	ClosedBy           string          `json:"closed_by,omitempty"`
	DueAt              *time.Time      `json:"due_at,omitempty"`
	DeferUntil         *time.Time      `json:"defer_until,omitempty"`
	Fields json.RawMessage `json:"fields,omitempty"`

	// Relational data -- populated by queries, not stored in the beads table.
	Labels       []string      `json:"labels,omitempty"`
	Dependencies []*Dependency `json:"dependencies,omitempty"`
	Comments     []*Comment    `json:"comments,omitempty"`
}
