package main

import (
	"testing"
)

func TestParseSimpleOption_ThreeParts(t *testing.T) {
	opt, err := parseSimpleOption("yes:Deploy now:plan")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opt["id"] != "yes" {
		t.Errorf("id = %v, want yes", opt["id"])
	}
	if opt["short"] != "Deploy now" {
		t.Errorf("short = %v, want 'Deploy now'", opt["short"])
	}
	if opt["artifact_type"] != "plan" {
		t.Errorf("artifact_type = %v, want plan", opt["artifact_type"])
	}
}

func TestParseSimpleOption_TwoParts_DefaultsToReport(t *testing.T) {
	opt, err := parseSimpleOption("skip:Skip this")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opt["id"] != "skip" {
		t.Errorf("id = %v, want skip", opt["id"])
	}
	if opt["short"] != "Skip this" {
		t.Errorf("short = %v, want 'Skip this'", opt["short"])
	}
	if opt["artifact_type"] != "report" {
		t.Errorf("artifact_type = %v, want report", opt["artifact_type"])
	}
}

func TestParseSimpleOption_OnePart_UsesAsID(t *testing.T) {
	opt, err := parseSimpleOption("approve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opt["id"] != "approve" {
		t.Errorf("id = %v, want approve", opt["id"])
	}
	if opt["artifact_type"] != "report" {
		t.Errorf("artifact_type = %v, want report", opt["artifact_type"])
	}
}

func TestParseSimpleOption_EmptyArtifactType_DefaultsToReport(t *testing.T) {
	opt, err := parseSimpleOption("yes:Approve:")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opt["artifact_type"] != "report" {
		t.Errorf("artifact_type = %v, want report", opt["artifact_type"])
	}
}

func TestParseSimpleOption_InvalidArtifactType(t *testing.T) {
	_, err := parseSimpleOption("yes:Approve:invalid")
	if err == nil {
		t.Fatal("expected error for invalid artifact_type")
	}
}

func TestParseSimpleOption_AllValidArtifactTypes(t *testing.T) {
	for _, at := range []string{"report", "plan", "checklist", "diff-summary", "epic", "bug"} {
		opt, err := parseSimpleOption("id:label:" + at)
		if err != nil {
			t.Errorf("unexpected error for artifact_type %q: %v", at, err)
		}
		if got := opt["artifact_type"]; got != at {
			t.Errorf("artifact_type = %v, want %v", got, at)
		}
	}
}
