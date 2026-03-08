package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/groblegark/kbeads/internal/model"
)

func TestExportJSONL_Empty(t *testing.T) {
	ms := newMockStore()
	var buf bytes.Buffer
	if err := ExportJSONL(context.Background(), ms, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := nonEmptyLines(buf.String())
	if len(lines) != 1 {
		t.Fatalf("expected 1 line (header only), got %d", len(lines))
	}

	var h header
	if err := json.Unmarshal([]byte(lines[0]), &h); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if h.Version != "1" || h.Type != "header" || h.BeadCount != 0 || h.ConfigCount != 0 {
		t.Fatalf("unexpected header: %+v", h)
	}
}

func TestExportJSONL_WithBeadsAndConfigs(t *testing.T) {
	ms := newMockStore()
	now := time.Now().UTC()

	// Add beads out of ID order to verify sorting.
	ms.beads["kd-zzz"] = &model.Bead{ID: "kd-zzz", Kind: model.KindIssue, Type: model.TypeTask, Title: "Second", Status: model.StatusOpen, CreatedAt: now, UpdatedAt: now}
	ms.beads["kd-aaa"] = &model.Bead{ID: "kd-aaa", Kind: model.KindIssue, Type: model.TypeBug, Title: "First", Status: model.StatusOpen, CreatedAt: now, UpdatedAt: now}

	// Add relational data for kd-aaa.
	ms.labels["kd-aaa"] = []string{"urgent", "frontend"}
	ms.deps["kd-aaa"] = []*model.Dependency{{BeadID: "kd-aaa", DependsOnID: "kd-zzz", Type: model.DepBlocks, CreatedAt: now}}
	ms.comments["kd-aaa"] = []*model.Comment{{ID: 1, BeadID: "kd-aaa", Author: "alice", Text: "Fix this", CreatedAt: now}}

	// Add a config.
	ms.configs["view:inbox"] = &model.Config{Key: "view:inbox", Value: json.RawMessage(`{"filter":{}}`), CreatedAt: now, UpdatedAt: now}

	var buf bytes.Buffer
	if err := ExportJSONL(context.Background(), ms, &buf); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	lines := nonEmptyLines(buf.String())
	// 1 header + 2 beads + 1 config = 4 lines
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines, got %d:\n%s", len(lines), buf.String())
	}

	// Verify header.
	var h header
	if err := json.Unmarshal([]byte(lines[0]), &h); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if h.BeadCount != 2 || h.ConfigCount != 1 {
		t.Fatalf("header counts: bead=%d config=%d", h.BeadCount, h.ConfigCount)
	}

	// Verify beads are sorted by ID (kd-aaa before kd-zzz).
	var rec1, rec2 record
	if err := json.Unmarshal([]byte(lines[1]), &rec1); err != nil {
		t.Fatalf("unmarshal line 1: %v", err)
	}
	if err := json.Unmarshal([]byte(lines[2]), &rec2); err != nil {
		t.Fatalf("unmarshal line 2: %v", err)
	}
	if rec1.Type != "bead" || rec2.Type != "bead" {
		t.Fatalf("expected bead types, got %q and %q", rec1.Type, rec2.Type)
	}

	// Parse bead data to check IDs.
	data1, _ := json.Marshal(rec1.Data)
	data2, _ := json.Marshal(rec2.Data)
	var b1, b2 model.Bead
	if err := json.Unmarshal(data1, &b1); err != nil {
		t.Fatalf("unmarshal b1: %v", err)
	}
	if err := json.Unmarshal(data2, &b2); err != nil {
		t.Fatalf("unmarshal b2: %v", err)
	}

	if b1.ID != "kd-aaa" || b2.ID != "kd-zzz" {
		t.Fatalf("beads not sorted: got %q, %q", b1.ID, b2.ID)
	}

	// Verify kd-aaa has embedded relations.
	if len(b1.Labels) != 2 {
		t.Fatalf("expected 2 labels for kd-aaa, got %d", len(b1.Labels))
	}
	if len(b1.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency for kd-aaa, got %d", len(b1.Dependencies))
	}
	if len(b1.Comments) != 1 {
		t.Fatalf("expected 1 comment for kd-aaa, got %d", len(b1.Comments))
	}

	// Verify config line.
	var rec3 record
	if err := json.Unmarshal([]byte(lines[3]), &rec3); err != nil {
		t.Fatalf("unmarshal line 3: %v", err)
	}
	if rec3.Type != "config" {
		t.Fatalf("expected config type, got %q", rec3.Type)
	}
}

func nonEmptyLines(s string) []string {
	var result []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			result = append(result, line)
		}
	}
	return result
}
