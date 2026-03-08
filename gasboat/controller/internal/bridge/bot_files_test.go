package bridge

import (
	"strings"
	"testing"

	"github.com/slack-go/slack"
)

func TestFormatAttachmentsSection_Empty(t *testing.T) {
	result := formatAttachmentsSection(nil)
	if result != "" {
		t.Errorf("expected empty string for nil files, got %q", result)
	}
}

func TestFormatAttachmentsSection_WithFiles(t *testing.T) {
	files := []slack.File{
		{ID: "F1", Name: "screenshot.png", Mimetype: "image/png", Size: 1024 * 512},
		{ID: "F2", Name: "report.pdf", Mimetype: "application/pdf", Size: 1024 * 1024 * 2},
	}

	result := formatAttachmentsSection(files)
	if result == "" {
		t.Fatal("expected non-empty result")
	}

	if !strings.Contains(result, "## Attachments") {
		t.Error("missing ## Attachments header")
	}
	if !strings.Contains(result, "screenshot.png") {
		t.Error("missing file name screenshot.png")
	}
	if !strings.Contains(result, "/api/slack/files/F1") {
		t.Error("missing proxy URL for F1")
	}
	if !strings.Contains(result, "/api/slack/files/F2") {
		t.Error("missing proxy URL for F2")
	}
}

func TestSlackFilesToFields_NoFiles(t *testing.T) {
	fields := slackFilesToFields(nil)
	if fields != nil {
		t.Errorf("expected nil for no files, got %v", fields)
	}
}

func TestSlackFilesToFields_WithImages(t *testing.T) {
	files := []slack.File{
		{ID: "F1", Mimetype: "image/png"},
		{ID: "F2", Mimetype: "application/pdf"},
	}

	fields := slackFilesToFields(files)
	if fields == nil {
		t.Fatal("expected non-nil fields")
	}
	if fields["slack_attachment_count"] != "2" {
		t.Errorf("attachment_count = %q, want %q", fields["slack_attachment_count"], "2")
	}
	if fields["slack_has_images"] != "true" {
		t.Errorf("has_images = %q, want %q", fields["slack_has_images"], "true")
	}
}

func TestSlackFilesToFields_NoImages(t *testing.T) {
	files := []slack.File{
		{ID: "F1", Mimetype: "application/pdf"},
	}

	fields := slackFilesToFields(files)
	if fields == nil {
		t.Fatal("expected non-nil fields")
	}
	if fields["slack_attachment_count"] != "1" {
		t.Errorf("attachment_count = %q, want %q", fields["slack_attachment_count"], "1")
	}
	if _, ok := fields["slack_has_images"]; ok {
		t.Error("expected no slack_has_images field for non-image files")
	}
}

func TestFormatFileSize(t *testing.T) {
	tests := []struct {
		bytes int
		want  string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1024 * 1024, "1.0 MB"},
		{1024 * 1024 * 5, "5.0 MB"},
	}
	for _, tt := range tests {
		got := formatFileSize(tt.bytes)
		if got != tt.want {
			t.Errorf("formatFileSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}
