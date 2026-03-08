package model

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidationError holds a list of field-level validation errors.
type ValidationError struct {
	Errors []FieldError
}

// FieldError represents a single validation failure on a named field.
type FieldError struct {
	Field   string
	Message string
}

// Error formats the validation error as a semicolon-separated list of field messages.
func (e *ValidationError) Error() string {
	parts := make([]string, len(e.Errors))
	for i, fe := range e.Errors {
		parts[i] = fe.Field + ": " + fe.Message
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// HasErrors reports whether the validation error contains any field errors.
func (e *ValidationError) HasErrors() bool {
	return len(e.Errors) > 0
}

// ValidateBead checks a Bead for constraint violations.
// It returns a *ValidationError if any rules fail, or nil if the bead is valid.
func ValidateBead(b *Bead) error {
	var ve ValidationError

	// Title: required and at most 500 characters.
	title := strings.TrimSpace(b.Title)
	if title == "" {
		ve.Errors = append(ve.Errors, FieldError{Field: "title", Message: "is required"})
	} else if len([]rune(title)) > 500 {
		ve.Errors = append(ve.Errors, FieldError{Field: "title", Message: "must be 500 characters or fewer"})
	}

	// Priority: must be 0-4.
	if b.Priority < 0 || b.Priority > 4 {
		ve.Errors = append(ve.Errors, FieldError{
			Field:   "priority",
			Message: fmt.Sprintf("must be between 0 and 4, got %d", b.Priority),
		})
	}

	// Status: must be a valid enum value (closed set).
	if !b.Status.IsValid() {
		ve.Errors = append(ve.Errors, FieldError{
			Field:   "status",
			Message: fmt.Sprintf("invalid value %q", b.Status),
		})
	}

	// Type: must be non-empty (bead types are extensible).
	if strings.TrimSpace(string(b.Type)) == "" {
		ve.Errors = append(ve.Errors, FieldError{
			Field:   "type",
			Message: "is required",
		})
	}

	// Kind: must be a valid enum value (closed set).
	if !b.Kind.IsValid() {
		ve.Errors = append(ve.Errors, FieldError{
			Field:   "kind",
			Message: fmt.Sprintf("invalid value %q", b.Kind),
		})
	}

	// ClosedAt consistency with Status.
	if b.Status == StatusClosed && b.ClosedAt == nil {
		ve.Errors = append(ve.Errors, FieldError{
			Field:   "closed_at",
			Message: "is required when status is closed",
		})
	}
	if b.Status != StatusClosed && b.ClosedAt != nil {
		ve.Errors = append(ve.Errors, FieldError{
			Field:   "closed_at",
			Message: "must be nil when status is not closed",
		})
	}

	// Fields: must be valid JSON if present.
	if len(b.Fields) > 0 && !json.Valid(b.Fields) {
		ve.Errors = append(ve.Errors, FieldError{
			Field:   "fields",
			Message: "contains invalid JSON",
		})
	}

	if ve.HasErrors() {
		return &ve
	}
	return nil
}
