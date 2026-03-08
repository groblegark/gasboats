package main

import (
	"encoding/json"
	"strings"
	"unicode"
)

// splitField splits "key=value" into (key, value, true).
// Returns ("", "", false) if there is no '=' or key is empty.
func splitField(s string) (string, string, bool) {
	i := strings.IndexByte(s, '=')
	if i <= 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

// rawOrString returns a json.RawMessage if v looks like a JSON literal
// (object, array, quoted string, boolean, null, or number). Otherwise it
// returns v as a plain Go string so json.Marshal will quote it.
func rawOrString(v string) any {
	if len(v) == 0 {
		return v
	}
	switch v[0] {
	case '{', '[', '"':
		if json.Valid([]byte(v)) {
			return json.RawMessage(v)
		}
	default:
		// true, false, null, or a number
		if v == "true" || v == "false" || v == "null" {
			return json.RawMessage(v)
		}
		if len(v) > 0 && (v[0] == '-' || unicode.IsDigit(rune(v[0]))) {
			if json.Valid([]byte(v)) {
				return json.RawMessage(v)
			}
		}
	}
	return v // will be JSON-quoted as a string
}

// jsonMarshal is a thin wrapper so callers don't need to import encoding/json.
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}
