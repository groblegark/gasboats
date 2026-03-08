package model

import (
	"encoding/json"
	"fmt"
	"math"
	"time"
)

// ValidateFields checks that the given fields JSON conforms to the provided
// field definitions. It rejects unknown keys, validates types, and enforces
// required constraints. Returns a *ValidationError on failure, nil on success.
func ValidateFields(fields json.RawMessage, defs []FieldDef) error {
	if len(fields) == 0 {
		// No fields provided — only fail if any field is required.
		for _, d := range defs {
			if d.Required {
				return &ValidationError{Errors: []FieldError{{
					Field:   d.Name,
					Message: "is required",
				}}}
			}
		}
		return nil
	}

	var m map[string]any
	if err := json.Unmarshal(fields, &m); err != nil {
		return &ValidationError{Errors: []FieldError{{
			Field:   "fields",
			Message: "must be a JSON object",
		}}}
	}

	// Build lookup for known field names.
	defsByName := make(map[string]*FieldDef, len(defs))
	for i := range defs {
		defsByName[defs[i].Name] = &defs[i]
	}

	var ve ValidationError

	// Reject unknown keys.
	for key := range m {
		if _, ok := defsByName[key]; !ok {
			ve.Errors = append(ve.Errors, FieldError{
				Field:   key,
				Message: "unknown field",
			})
		}
	}

	// Validate each defined field.
	for _, d := range defs {
		val, present := m[d.Name]
		if !present || val == nil {
			if d.Required {
				ve.Errors = append(ve.Errors, FieldError{
					Field:   d.Name,
					Message: "is required",
				})
			}
			continue
		}
		if err := validateFieldValue(d, val); err != nil {
			ve.Errors = append(ve.Errors, FieldError{
				Field:   d.Name,
				Message: err.Error(),
			})
		}
	}

	if ve.HasErrors() {
		return &ve
	}
	return nil
}

func validateFieldValue(d FieldDef, val any) error {
	switch d.Type {
	case FieldTypeString:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("must be a string")
		}
	case FieldTypeInteger:
		n, ok := val.(float64)
		if !ok {
			return fmt.Errorf("must be an integer")
		}
		if n != math.Trunc(n) {
			return fmt.Errorf("must be an integer")
		}
	case FieldTypeFloat:
		if _, ok := val.(float64); !ok {
			return fmt.Errorf("must be a number")
		}
	case FieldTypeBoolean:
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("must be a boolean")
		}
	case FieldTypeTimestamp:
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("must be an RFC 3339 timestamp string")
		}
		if _, err := time.Parse(time.RFC3339, s); err != nil {
			return fmt.Errorf("must be an RFC 3339 timestamp string")
		}
	case FieldTypeEnum:
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("must be a string")
		}
		if !contains(d.Values, s) {
			return fmt.Errorf("must be one of %v", d.Values)
		}
	case FieldTypeStrings:
		arr, ok := val.([]any)
		if !ok {
			return fmt.Errorf("must be an array of strings")
		}
		for _, elem := range arr {
			if _, ok := elem.(string); !ok {
				return fmt.Errorf("must be an array of strings")
			}
		}
	case FieldTypeEnums:
		arr, ok := val.([]any)
		if !ok {
			return fmt.Errorf("must be an array of strings")
		}
		for _, elem := range arr {
			s, ok := elem.(string)
			if !ok {
				return fmt.Errorf("must be an array of strings")
			}
			if !contains(d.Values, s) {
				return fmt.Errorf("array element %q must be one of %v", s, d.Values)
			}
		}
	case FieldTypeJSON:
		// Any valid JSON value is accepted.
	default:
		return fmt.Errorf("unknown field type %q", d.Type)
	}
	return nil
}

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
