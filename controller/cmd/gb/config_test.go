package main

import (
	"context"
	"encoding/json"
	"fmt"
	"gasboat/controller/internal/beadsapi"
	"os"
	"path/filepath"
	"testing"
)

// mockDumper implements configDumper for testing.
type mockDumper struct {
	configs map[string][]beadsapi.ConfigEntry // namespace → entries
	err     error                             // if set, all calls return this
}

func (m *mockDumper) ListConfigs(_ context.Context, namespace string) ([]beadsapi.ConfigEntry, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.configs[namespace], nil
}

// mockLoader implements configLoader for testing.
type mockLoader struct {
	stored map[string]json.RawMessage // key → value
	err    error                      // if set, all calls return this
}

func (m *mockLoader) SetConfig(_ context.Context, key string, value []byte) error {
	if m.err != nil {
		return m.err
	}
	m.stored[key] = json.RawMessage(value)
	return nil
}

func TestDumpConfigs_CollectsAllNamespaces(t *testing.T) {
	dumper := &mockDumper{
		configs: map[string][]beadsapi.ConfigEntry{
			"claude-settings": {
				{Key: "claude-settings:global", Value: json.RawMessage(`{"model":"opus"}`)},
			},
			"claude-mcp": {
				{Key: "claude-mcp:global", Value: json.RawMessage(`{"mcpServers":{}}`)},
			},
			"type": {
				{Key: "type:agent", Value: json.RawMessage(`{"kind":"agent"}`)},
				{Key: "type:task", Value: json.RawMessage(`{"kind":"task"}`)},
			},
		},
	}

	entries, err := dumpConfigs(context.Background(), dumper, []string{"claude-settings", "claude-mcp", "type"})
	if err != nil {
		t.Fatalf("dumpConfigs: %v", err)
	}

	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}
}

func TestDumpConfigs_SortsByKey(t *testing.T) {
	dumper := &mockDumper{
		configs: map[string][]beadsapi.ConfigEntry{
			"b-ns": {{Key: "b-ns:zebra", Value: json.RawMessage(`{}`)}},
			"a-ns": {{Key: "a-ns:alpha", Value: json.RawMessage(`{}`)}},
		},
	}

	entries, err := dumpConfigs(context.Background(), dumper, []string{"b-ns", "a-ns"})
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Key != "a-ns:alpha" {
		t.Errorf("expected first key a-ns:alpha, got %s", entries[0].Key)
	}
	if entries[1].Key != "b-ns:zebra" {
		t.Errorf("expected second key b-ns:zebra, got %s", entries[1].Key)
	}
}

func TestDumpConfigs_EmptyNamespace(t *testing.T) {
	dumper := &mockDumper{
		configs: map[string][]beadsapi.ConfigEntry{},
	}

	entries, err := dumpConfigs(context.Background(), dumper, []string{"claude-settings"})
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestDumpConfigs_ContinuesOnError(t *testing.T) {
	// First namespace errors, second succeeds.
	callCount := 0
	dumper := &mockDumper{
		configs: map[string][]beadsapi.ConfigEntry{
			"good-ns": {{Key: "good-ns:one", Value: json.RawMessage(`{}`)}},
		},
	}
	// Wrap to fail on first call.
	wrapper := &failFirstDumper{inner: dumper, failNs: "bad-ns"}

	entries, err := dumpConfigs(context.Background(), wrapper, []string{"bad-ns", "good-ns"})
	if err != nil {
		t.Fatal(err)
	}
	_ = callCount

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (skipping failed ns), got %d", len(entries))
	}
	if entries[0].Key != "good-ns:one" {
		t.Errorf("expected good-ns:one, got %s", entries[0].Key)
	}
}

type failFirstDumper struct {
	inner  *mockDumper
	failNs string
}

func (f *failFirstDumper) ListConfigs(ctx context.Context, namespace string) ([]beadsapi.ConfigEntry, error) {
	if namespace == f.failNs {
		return nil, fmt.Errorf("connection refused")
	}
	return f.inner.ListConfigs(ctx, namespace)
}

func TestLoadConfigs_RestoresAll(t *testing.T) {
	loader := &mockLoader{stored: make(map[string]json.RawMessage)}

	entries := []configEntry{
		{Key: "claude-settings:global", Value: json.RawMessage(`{"model":"opus"}`)},
		{Key: "claude-mcp:global", Value: json.RawMessage(`{"mcpServers":{}}`)},
	}

	restored, errors := loadConfigs(context.Background(), loader, entries)
	if restored != 2 {
		t.Errorf("expected 2 restored, got %d", restored)
	}
	if errors != 0 {
		t.Errorf("expected 0 errors, got %d", errors)
	}

	if string(loader.stored["claude-settings:global"]) != `{"model":"opus"}` {
		t.Errorf("wrong value for claude-settings:global: %s", loader.stored["claude-settings:global"])
	}
	if string(loader.stored["claude-mcp:global"]) != `{"mcpServers":{}}` {
		t.Errorf("wrong value for claude-mcp:global: %s", loader.stored["claude-mcp:global"])
	}
}

func TestLoadConfigs_ContinuesOnError(t *testing.T) {
	loader := &mockLoader{
		stored: make(map[string]json.RawMessage),
		err:    fmt.Errorf("server error"),
	}

	entries := []configEntry{
		{Key: "a", Value: json.RawMessage(`{}`)},
		{Key: "b", Value: json.RawMessage(`{}`)},
	}

	restored, errors := loadConfigs(context.Background(), loader, entries)
	if restored != 0 {
		t.Errorf("expected 0 restored, got %d", restored)
	}
	if errors != 2 {
		t.Errorf("expected 2 errors, got %d", errors)
	}
}

func TestDumpLoad_Roundtrip(t *testing.T) {
	// Dump from a mock, serialize to JSON, write to file, read back, load into another mock.
	dumper := &mockDumper{
		configs: map[string][]beadsapi.ConfigEntry{
			"ns": {
				{Key: "ns:one", Value: json.RawMessage(`{"a":1}`)},
				{Key: "ns:two", Value: json.RawMessage(`{"b":2}`)},
			},
		},
	}

	entries, err := dumpConfigs(context.Background(), dumper, []string{"ns"})
	if err != nil {
		t.Fatal(err)
	}

	// Serialize (what gb config dump does).
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	// Write to temp file.
	tmpFile := filepath.Join(t.TempDir(), "dump.json")
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Read back and parse (what gb config load does).
	readData, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	var loaded []configEntry
	if err := json.Unmarshal(readData, &loaded); err != nil {
		t.Fatal(err)
	}

	// Load into mock.
	loader := &mockLoader{stored: make(map[string]json.RawMessage)}
	restored, errors := loadConfigs(context.Background(), loader, loaded)

	if restored != 2 {
		t.Errorf("expected 2 restored, got %d", restored)
	}
	if errors != 0 {
		t.Errorf("expected 0 errors, got %d", errors)
	}
	// Compare semantically — MarshalIndent reformats the JSON.
	var got1, want1 any
	json.Unmarshal(loader.stored["ns:one"], &got1)
	json.Unmarshal([]byte(`{"a":1}`), &want1)
	if fmt.Sprint(got1) != fmt.Sprint(want1) {
		t.Errorf("wrong value for ns:one: %s", loader.stored["ns:one"])
	}

	var got2, want2 any
	json.Unmarshal(loader.stored["ns:two"], &got2)
	json.Unmarshal([]byte(`{"b":2}`), &want2)
	if fmt.Sprint(got2) != fmt.Sprint(want2) {
		t.Errorf("wrong value for ns:two: %s", loader.stored["ns:two"])
	}
}
