package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"gasboat/controller/internal/beadsapi"
)

// --- Mock implementations ---

type mockBeadLister struct {
	beads []*beadsapi.BeadDetail
	err   error
}

func (m *mockBeadLister) ListBeadsFiltered(_ context.Context, q beadsapi.ListBeadsQuery) (*beadsapi.ListBeadsResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &beadsapi.ListBeadsResult{Beads: m.beads, Total: len(m.beads)}, nil
}

// --- Advice dump tests ---

func TestDumpAdvice_CollectsAndSorts(t *testing.T) {
	lister := &mockBeadLister{
		beads: []*beadsapi.BeadDetail{
			{Title: "Zebra rule", Description: "z desc", Labels: []string{"global"}, Priority: 1},
			{Title: "Alpha rule", Description: "a desc", Labels: []string{"role:captain"}, Priority: 2},
		},
	}

	entries, err := dumpAdvice(context.Background(), lister)
	if err != nil {
		t.Fatalf("dumpAdvice: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2, got %d", len(entries))
	}
	if entries[0].Title != "Alpha rule" {
		t.Errorf("expected sorted, first=%s", entries[0].Title)
	}
	if entries[1].Title != "Zebra rule" {
		t.Errorf("expected sorted, second=%s", entries[1].Title)
	}
	if entries[0].Description != "a desc" {
		t.Errorf("wrong description: %s", entries[0].Description)
	}
}

func TestDumpAdvice_Empty(t *testing.T) {
	lister := &mockBeadLister{beads: []*beadsapi.BeadDetail{}}
	entries, err := dumpAdvice(context.Background(), lister)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0, got %d", len(entries))
	}
}

func TestDumpAdvice_Error(t *testing.T) {
	lister := &mockBeadLister{err: fmt.Errorf("server down")}
	_, err := dumpAdvice(context.Background(), lister)
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Config bead dump/load tests ---

func TestDumpConfigBeads_CollectsAndSorts(t *testing.T) {
	lister := &mockBeadLister{
		beads: []*beadsapi.BeadDetail{
			{Title: "claude-mcp", Labels: []string{"global"}, Description: `{"mcpServers":{}}`},
			{Title: "claude-settings", Labels: []string{"role:captain"}, Description: `{"model":"opus"}`},
			{Title: "claude-settings", Labels: []string{"global"}, Description: `{"model":"sonnet"}`},
		},
	}

	entries, err := dumpConfigBeads(context.Background(), lister)
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 3 {
		t.Fatalf("expected 3, got %d", len(entries))
	}
	// Sorted by title then labels.
	if entries[0].Title != "claude-mcp" {
		t.Errorf("expected first=claude-mcp, got %s", entries[0].Title)
	}
	if entries[1].Title != "claude-settings" || entries[1].Labels[0] != "global" {
		t.Errorf("expected second=claude-settings [global], got %s %v", entries[1].Title, entries[1].Labels)
	}
	if entries[2].Title != "claude-settings" || entries[2].Labels[0] != "role:captain" {
		t.Errorf("expected third=claude-settings [role:captain], got %s %v", entries[2].Title, entries[2].Labels)
	}
}

func TestDumpConfigBeads_Empty(t *testing.T) {
	lister := &mockBeadLister{beads: []*beadsapi.BeadDetail{}}
	entries, err := dumpConfigBeads(context.Background(), lister)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0, got %d", len(entries))
	}
}

type mockConfigBeadCreator struct {
	beads   []*beadsapi.BeadDetail
	created []beadsapi.CreateBeadRequest
	err     error
}

func (m *mockConfigBeadCreator) ListBeadsFiltered(_ context.Context, q beadsapi.ListBeadsQuery) (*beadsapi.ListBeadsResult, error) {
	return &beadsapi.ListBeadsResult{Beads: m.beads, Total: len(m.beads)}, nil
}

func (m *mockConfigBeadCreator) CreateBead(_ context.Context, req beadsapi.CreateBeadRequest) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	m.created = append(m.created, req)
	return fmt.Sprintf("kd-load-%d", len(m.created)), nil
}

func TestLoadConfigBeads_CreatesNew(t *testing.T) {
	client := &mockConfigBeadCreator{beads: []*beadsapi.BeadDetail{}}
	entries := []configBeadEntry{
		{Title: "claude-settings", Labels: []string{"global"}, Value: json.RawMessage(`{"model":"opus"}`)},
		{Title: "claude-mcp", Labels: []string{"global"}, Value: json.RawMessage(`{"mcpServers":{}}`)},
	}

	created, skipped, errors := loadConfigBeads(context.Background(), client, entries)
	if created != 2 || skipped != 0 || errors != 0 {
		t.Errorf("expected 2/0/0, got %d/%d/%d", created, skipped, errors)
	}
	if len(client.created) != 2 {
		t.Fatalf("expected 2 creates, got %d", len(client.created))
	}
	if client.created[0].Type != "config" {
		t.Errorf("wrong type: %s", client.created[0].Type)
	}
}

func TestLoadConfigBeads_SkipsDuplicates(t *testing.T) {
	client := &mockConfigBeadCreator{
		beads: []*beadsapi.BeadDetail{
			{Title: "claude-settings", Labels: []string{"global"}, Description: `{"model":"opus"}`},
		},
	}
	entries := []configBeadEntry{
		{Title: "claude-settings", Labels: []string{"global"}, Value: json.RawMessage(`{"model":"sonnet"}`)},
		{Title: "claude-mcp", Labels: []string{"global"}, Value: json.RawMessage(`{}`)},
	}

	created, skipped, errors := loadConfigBeads(context.Background(), client, entries)
	if created != 1 || skipped != 1 || errors != 0 {
		t.Errorf("expected 1/1/0, got %d/%d/%d", created, skipped, errors)
	}
}

func TestConfigDump_IncludesConfigBeads(t *testing.T) {
	dump := configDump{
		Configs: []configEntry{
			{Key: "claude-settings:global", Value: json.RawMessage(`{"model":"opus"}`)},
		},
		ConfigBeads: []configBeadEntry{
			{Title: "claude-settings", Labels: []string{"global"}, Value: json.RawMessage(`{"model":"opus"}`)},
		},
		Advice: []adviceEntry{
			{Title: "Rule", Description: "desc", Labels: []string{"global"}, Priority: 1},
		},
	}

	data, err := json.Marshal(dump)
	if err != nil {
		t.Fatal(err)
	}

	var loaded configDump
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}

	if len(loaded.Configs) != 1 {
		t.Errorf("expected 1 config, got %d", len(loaded.Configs))
	}
	if len(loaded.ConfigBeads) != 1 {
		t.Errorf("expected 1 config bead, got %d", len(loaded.ConfigBeads))
	}
	if len(loaded.Advice) != 1 {
		t.Errorf("expected 1 advice, got %d", len(loaded.Advice))
	}
}

func TestConfigDump_OmitsEmptyConfigBeads(t *testing.T) {
	dump := configDump{
		Configs: []configEntry{{Key: "a", Value: json.RawMessage(`{}`)}},
	}

	data, err := json.Marshal(dump)
	if err != nil {
		t.Fatal(err)
	}

	// config_beads should be omitted when nil.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["config_beads"]; ok {
		t.Error("expected config_beads to be omitted when empty")
	}
}

