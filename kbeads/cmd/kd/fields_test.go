package main

import (
	"encoding/json"
	"testing"
)

func TestParseFields(t *testing.T) {
	tests := []struct {
		name    string
		pairs   []string
		want    string
		wantErr bool
	}{
		{
			name:  "nil input",
			pairs: nil,
			want:  "",
		},
		{
			name:  "plain strings",
			pairs: []string{"question=What next?", "requestor=agent-1"},
			want:  `{"question":"What next?","requestor":"agent-1"}`,
		},
		{
			name:  "json array value",
			pairs: []string{`options=[{"id":"done","label":"Done"}]`},
			want:  `{"options":[{"id":"done","label":"Done"}]}`,
		},
		{
			name:  "json object value",
			pairs: []string{`meta={"key":"val"}`},
			want:  `{"meta":{"key":"val"}}`,
		},
		{
			name:  "boolean and number",
			pairs: []string{"active=true", "count=42", "ratio=3.14"},
			want:  `{"active":true,"count":42,"ratio":3.14}`,
		},
		{
			name:  "null value",
			pairs: []string{"cleared=null"},
			want:  `{"cleared":null}`,
		},
		{
			name:  "quoted json string",
			pairs: []string{`name="hello"`},
			want:  `{"name":"hello"}`,
		},
		{
			name:  "plain number-like string that is not valid json",
			pairs: []string{"version=1.2.3"},
			want:  `{"version":"1.2.3"}`,
		},
		{
			name:    "missing equals",
			pairs:   []string{"noequals"},
			wantErr: true,
		},
		{
			name:    "empty key",
			pairs:   []string{"=value"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFields(tt.pairs)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.want == "" {
				if got != nil {
					t.Fatalf("expected nil, got %s", got)
				}
				return
			}
			// Compare as unmarshaled maps to ignore key ordering.
			var wantMap, gotMap map[string]any
			if err := json.Unmarshal([]byte(tt.want), &wantMap); err != nil {
				t.Fatalf("bad test want: %v", err)
			}
			if err := json.Unmarshal(got, &gotMap); err != nil {
				t.Fatalf("result is not valid JSON: %s", got)
			}
			// Re-marshal both to canonical form for comparison.
			wantJSON, _ := json.Marshal(wantMap)
			gotJSON, _ := json.Marshal(gotMap)
			if string(wantJSON) != string(gotJSON) {
				t.Errorf("got  %s\nwant %s", gotJSON, wantJSON)
			}
		})
	}
}
