package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// validBead returns a Bead that passes all validation rules.
func validBead() Bead {
	return Bead{
		Title:    "Implement login flow",
		Priority: 2,
		Status:   StatusOpen,
		Type:     TypeTask,
		Kind:     KindIssue,
	}
}

// fieldErrors extracts a *ValidationError from err or fails the test.
func fieldErrors(t *testing.T, err error) []FieldError {
	t.Helper()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	return ve.Errors
}

// hasFieldError reports whether the error list contains an error for the given field.
func hasFieldError(errs []FieldError, field string) bool {
	for _, fe := range errs {
		if fe.Field == field {
			return true
		}
	}
	return false
}

func TestValidate_TitleRequired(t *testing.T) {
	b := validBead()
	b.Title = ""
	errs := fieldErrors(t, ValidateBead(&b))
	if !hasFieldError(errs, "title") {
		t.Error("expected error on field 'title' for empty title")
	}
}

func TestValidate_TitleWhitespaceOnly(t *testing.T) {
	b := validBead()
	b.Title = "   \t\n  "
	errs := fieldErrors(t, ValidateBead(&b))
	if !hasFieldError(errs, "title") {
		t.Error("expected error on field 'title' for whitespace-only title")
	}
}

func TestValidate_TitleTooLong(t *testing.T) {
	b := validBead()
	b.Title = strings.Repeat("a", 501)
	errs := fieldErrors(t, ValidateBead(&b))
	if !hasFieldError(errs, "title") {
		t.Error("expected error on field 'title' for title exceeding 500 chars")
	}
}

func TestValidate_TitleExactly500(t *testing.T) {
	b := validBead()
	b.Title = strings.Repeat("a", 500)
	if err := ValidateBead(&b); err != nil {
		t.Errorf("title with exactly 500 chars should be valid, got: %v", err)
	}
}

func TestValidate_TitleValid(t *testing.T) {
	b := validBead()
	b.Title = "A perfectly normal title"
	if err := ValidateBead(&b); err != nil {
		t.Errorf("expected no error for valid title, got: %v", err)
	}
}

func TestValidate_PriorityTooLow(t *testing.T) {
	b := validBead()
	b.Priority = -1
	errs := fieldErrors(t, ValidateBead(&b))
	if !hasFieldError(errs, "priority") {
		t.Error("expected error on field 'priority' for value -1")
	}
}

func TestValidate_PriorityTooHigh(t *testing.T) {
	b := validBead()
	b.Priority = 5
	errs := fieldErrors(t, ValidateBead(&b))
	if !hasFieldError(errs, "priority") {
		t.Error("expected error on field 'priority' for value 5")
	}
}

func TestValidate_PriorityZero(t *testing.T) {
	b := validBead()
	b.Priority = 0
	if err := ValidateBead(&b); err != nil {
		t.Errorf("priority 0 should be valid, got: %v", err)
	}
}

func TestValidate_PriorityFour(t *testing.T) {
	b := validBead()
	b.Priority = 4
	if err := ValidateBead(&b); err != nil {
		t.Errorf("priority 4 should be valid, got: %v", err)
	}
}

func TestValidate_InvalidStatus(t *testing.T) {
	b := validBead()
	b.Status = Status("bogus")
	errs := fieldErrors(t, ValidateBead(&b))
	if !hasFieldError(errs, "status") {
		t.Error("expected error on field 'status' for invalid value")
	}
}

func TestValidate_EmptyType(t *testing.T) {
	b := validBead()
	b.Type = BeadType("")
	errs := fieldErrors(t, ValidateBead(&b))
	if !hasFieldError(errs, "type") {
		t.Error("expected error on field 'type' for empty value")
	}
}

func TestValidate_CustomTypeValid(t *testing.T) {
	b := validBead()
	b.Type = BeadType("workflow")
	if err := ValidateBead(&b); err != nil {
		t.Errorf("custom bead type 'workflow' should be valid, got: %v", err)
	}
}

func TestValidate_InvalidKind(t *testing.T) {
	b := validBead()
	b.Kind = Kind("bogus")
	errs := fieldErrors(t, ValidateBead(&b))
	if !hasFieldError(errs, "kind") {
		t.Error("expected error on field 'kind' for invalid value")
	}
}

func TestValidate_ClosedWithoutClosedAt(t *testing.T) {
	b := validBead()
	b.Status = StatusClosed
	b.ClosedAt = nil
	errs := fieldErrors(t, ValidateBead(&b))
	if !hasFieldError(errs, "closed_at") {
		t.Error("expected error on field 'closed_at' when status is closed but ClosedAt is nil")
	}
}

func TestValidate_ClosedWithClosedAt(t *testing.T) {
	b := validBead()
	b.Status = StatusClosed
	now := time.Now()
	b.ClosedAt = &now
	if err := ValidateBead(&b); err != nil {
		t.Errorf("closed bead with ClosedAt set should be valid, got: %v", err)
	}
}

func TestValidate_OpenWithClosedAt(t *testing.T) {
	b := validBead()
	b.Status = StatusOpen
	now := time.Now()
	b.ClosedAt = &now
	errs := fieldErrors(t, ValidateBead(&b))
	if !hasFieldError(errs, "closed_at") {
		t.Error("expected error on field 'closed_at' when status is open but ClosedAt is set")
	}
}

func TestValidate_FieldsInvalidJSON(t *testing.T) {
	b := validBead()
	b.Fields = json.RawMessage(`{not json}`)
	errs := fieldErrors(t, ValidateBead(&b))
	if !hasFieldError(errs, "fields") {
		t.Error("expected error on field 'fields' for invalid JSON")
	}
}

func TestValidate_FieldsValidJSON(t *testing.T) {
	b := validBead()
	b.Fields = json.RawMessage(`{"key": "value"}`)
	if err := ValidateBead(&b); err != nil {
		t.Errorf("valid JSON fields should pass, got: %v", err)
	}
}

func TestValidate_FieldsNil(t *testing.T) {
	b := validBead()
	b.Fields = nil
	if err := ValidateBead(&b); err != nil {
		t.Errorf("nil fields should pass, got: %v", err)
	}
}

func TestValidate_FullyValidBead(t *testing.T) {
	b := validBead()
	if err := ValidateBead(&b); err != nil {
		t.Errorf("expected no error for a fully valid bead, got: %v", err)
	}
}

func TestValidationError_Error(t *testing.T) {
	ve := &ValidationError{
		Errors: []FieldError{
			{Field: "title", Message: "is required"},
			{Field: "priority", Message: "must be between 0 and 4, got 9"},
		},
	}
	got := ve.Error()
	want := "validation failed: title: is required; priority: must be between 0 and 4, got 9"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestValidationError_HasErrors(t *testing.T) {
	ve := &ValidationError{}
	if ve.HasErrors() {
		t.Error("HasErrors() should be false for empty Errors slice")
	}
	ve.Errors = append(ve.Errors, FieldError{Field: "x", Message: "y"})
	if !ve.HasErrors() {
		t.Error("HasErrors() should be true when Errors is non-empty")
	}
}
