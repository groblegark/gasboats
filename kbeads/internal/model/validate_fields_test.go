package model

import (
	"encoding/json"
	"testing"
)

func TestValidateFields_EmptyDefsEmptyFields(t *testing.T) {
	if err := ValidateFields(nil, nil); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateFields_RequiredMissing(t *testing.T) {
	defs := []FieldDef{{Name: "severity", Type: FieldTypeString, Required: true}}
	err := ValidateFields(nil, defs)
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
	errs := fieldErrors(t, err)
	if !hasFieldError(errs, "severity") {
		t.Error("expected error on 'severity'")
	}
}

func TestValidateFields_RequiredNullValue(t *testing.T) {
	defs := []FieldDef{{Name: "severity", Type: FieldTypeString, Required: true}}
	err := ValidateFields(json.RawMessage(`{"severity":null}`), defs)
	if err == nil {
		t.Fatal("expected error for null required field")
	}
	errs := fieldErrors(t, err)
	if !hasFieldError(errs, "severity") {
		t.Error("expected error on 'severity'")
	}
}

func TestValidateFields_OptionalMissing(t *testing.T) {
	defs := []FieldDef{{Name: "notes", Type: FieldTypeString}}
	if err := ValidateFields(json.RawMessage(`{}`), defs); err != nil {
		t.Fatalf("optional missing field should pass, got %v", err)
	}
}

func TestValidateFields_UnknownField(t *testing.T) {
	defs := []FieldDef{{Name: "severity", Type: FieldTypeString}}
	err := ValidateFields(json.RawMessage(`{"severity":"high","bogus":123}`), defs)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	errs := fieldErrors(t, err)
	if !hasFieldError(errs, "bogus") {
		t.Error("expected error on 'bogus'")
	}
}

func TestValidateFields_StringType(t *testing.T) {
	defs := []FieldDef{{Name: "name", Type: FieldTypeString}}
	if err := ValidateFields(json.RawMessage(`{"name":"hello"}`), defs); err != nil {
		t.Fatalf("expected pass for string, got %v", err)
	}
	err := ValidateFields(json.RawMessage(`{"name":123}`), defs)
	if err == nil {
		t.Fatal("expected error for non-string")
	}
}

func TestValidateFields_IntegerType(t *testing.T) {
	defs := []FieldDef{{Name: "count", Type: FieldTypeInteger}}
	if err := ValidateFields(json.RawMessage(`{"count":42}`), defs); err != nil {
		t.Fatalf("expected pass for integer, got %v", err)
	}
	err := ValidateFields(json.RawMessage(`{"count":3.14}`), defs)
	if err == nil {
		t.Fatal("expected error for float in integer field")
	}
	err = ValidateFields(json.RawMessage(`{"count":"five"}`), defs)
	if err == nil {
		t.Fatal("expected error for string in integer field")
	}
}

func TestValidateFields_FloatType(t *testing.T) {
	defs := []FieldDef{{Name: "weight", Type: FieldTypeFloat}}
	if err := ValidateFields(json.RawMessage(`{"weight":3.14}`), defs); err != nil {
		t.Fatalf("expected pass for float, got %v", err)
	}
	if err := ValidateFields(json.RawMessage(`{"weight":42}`), defs); err != nil {
		t.Fatalf("expected pass for integer in float field, got %v", err)
	}
	err := ValidateFields(json.RawMessage(`{"weight":"heavy"}`), defs)
	if err == nil {
		t.Fatal("expected error for string in float field")
	}
}

func TestValidateFields_EnumType(t *testing.T) {
	defs := []FieldDef{{Name: "priority", Type: FieldTypeEnum, Values: []string{"low", "medium", "high"}}}
	if err := ValidateFields(json.RawMessage(`{"priority":"high"}`), defs); err != nil {
		t.Fatalf("expected pass for valid enum, got %v", err)
	}
	err := ValidateFields(json.RawMessage(`{"priority":"critical"}`), defs)
	if err == nil {
		t.Fatal("expected error for invalid enum value")
	}
	err = ValidateFields(json.RawMessage(`{"priority":1}`), defs)
	if err == nil {
		t.Fatal("expected error for non-string enum")
	}
}

func TestValidateFields_StringArrayType(t *testing.T) {
	defs := []FieldDef{{Name: "tags", Type: FieldTypeStrings}}
	if err := ValidateFields(json.RawMessage(`{"tags":["a","b"]}`), defs); err != nil {
		t.Fatalf("expected pass for string array, got %v", err)
	}
	if err := ValidateFields(json.RawMessage(`{"tags":[]}`), defs); err != nil {
		t.Fatalf("expected pass for empty array, got %v", err)
	}
	err := ValidateFields(json.RawMessage(`{"tags":[1,2]}`), defs)
	if err == nil {
		t.Fatal("expected error for non-string elements")
	}
	err = ValidateFields(json.RawMessage(`{"tags":"single"}`), defs)
	if err == nil {
		t.Fatal("expected error for non-array")
	}
}

func TestValidateFields_EnumArrayType(t *testing.T) {
	defs := []FieldDef{{Name: "roles", Type: FieldTypeEnums, Values: []string{"admin", "user", "viewer"}}}
	if err := ValidateFields(json.RawMessage(`{"roles":["admin","user"]}`), defs); err != nil {
		t.Fatalf("expected pass for valid enum array, got %v", err)
	}
	err := ValidateFields(json.RawMessage(`{"roles":["admin","superuser"]}`), defs)
	if err == nil {
		t.Fatal("expected error for invalid enum array value")
	}
}

func TestValidateFields_BooleanType(t *testing.T) {
	defs := []FieldDef{{Name: "active", Type: FieldTypeBoolean}}
	if err := ValidateFields(json.RawMessage(`{"active":true}`), defs); err != nil {
		t.Fatalf("expected pass for boolean true, got %v", err)
	}
	if err := ValidateFields(json.RawMessage(`{"active":false}`), defs); err != nil {
		t.Fatalf("expected pass for boolean false, got %v", err)
	}
	err := ValidateFields(json.RawMessage(`{"active":"yes"}`), defs)
	if err == nil {
		t.Fatal("expected error for string in boolean field")
	}
	err = ValidateFields(json.RawMessage(`{"active":1}`), defs)
	if err == nil {
		t.Fatal("expected error for number in boolean field")
	}
}

func TestValidateFields_TimestampType(t *testing.T) {
	defs := []FieldDef{{Name: "due", Type: FieldTypeTimestamp}}
	if err := ValidateFields(json.RawMessage(`{"due":"2024-01-15T10:30:00Z"}`), defs); err != nil {
		t.Fatalf("expected pass for valid RFC3339 timestamp, got %v", err)
	}
	err := ValidateFields(json.RawMessage(`{"due":"not-a-timestamp"}`), defs)
	if err == nil {
		t.Fatal("expected error for invalid timestamp string")
	}
	err = ValidateFields(json.RawMessage(`{"due":1234567890}`), defs)
	if err == nil {
		t.Fatal("expected error for number in timestamp field")
	}
}

func TestValidateFields_JSONType(t *testing.T) {
	defs := []FieldDef{{Name: "extra", Type: FieldTypeJSON}}
	if err := ValidateFields(json.RawMessage(`{"extra":{"nested":"value"}}`), defs); err != nil {
		t.Fatalf("expected pass for JSON object, got %v", err)
	}
	if err := ValidateFields(json.RawMessage(`{"extra":42}`), defs); err != nil {
		t.Fatalf("expected pass for JSON number, got %v", err)
	}
	if err := ValidateFields(json.RawMessage(`{"extra":"string"}`), defs); err != nil {
		t.Fatalf("expected pass for JSON string, got %v", err)
	}
}

func TestValidateFields_InvalidJSON(t *testing.T) {
	defs := []FieldDef{{Name: "x", Type: FieldTypeString}}
	err := ValidateFields(json.RawMessage(`{not json}`), defs)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestValidateFields_MultipleErrors(t *testing.T) {
	defs := []FieldDef{
		{Name: "name", Type: FieldTypeString, Required: true},
		{Name: "count", Type: FieldTypeInteger, Required: true},
	}
	err := ValidateFields(json.RawMessage(`{}`), defs)
	if err == nil {
		t.Fatal("expected errors")
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if len(ve.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(ve.Errors))
	}
}
