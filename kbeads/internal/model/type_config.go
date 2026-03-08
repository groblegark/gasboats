package model

// FieldType identifies the JSON schema type of a typed field.
type FieldType string

const (
	FieldTypeString    FieldType = "string"
	FieldTypeInteger   FieldType = "integer"
	FieldTypeFloat     FieldType = "float"
	FieldTypeBoolean   FieldType = "boolean"
	FieldTypeTimestamp FieldType = "timestamp"
	FieldTypeEnum      FieldType = "enum"
	FieldTypeStrings   FieldType = "string[]"
	FieldTypeEnums     FieldType = "enum[]"
	FieldTypeJSON      FieldType = "json"
)

// FieldDef describes a single typed field on a bead type.
type FieldDef struct {
	Name     string    `json:"name"`
	Type     FieldType `json:"type"`
	Required bool      `json:"required,omitempty"`
	Values   []string  `json:"values,omitempty"` // allowed values for enum / enum[]
}

// TypeConfig defines the kind and typed fields for a bead type.
type TypeConfig struct {
	Kind   Kind       `json:"kind"`
	Fields []FieldDef `json:"fields,omitempty"`
}
