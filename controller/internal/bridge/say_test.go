package bridge

import (
	"context"
	"log/slog"
	"testing"
)

func TestSay_HandleClosed_IgnoresNonMessageBeads(t *testing.T) {
	s := &Say{
		daemon: newMockDaemon(),
		logger: slog.Default(),
		seen:   make(map[string]bool),
	}

	// Task bead should be ignored.
	data := marshalSSEBeadPayload(BeadEvent{
		ID:     "bd-task1",
		Type:   "task",
		Labels: []string{"bug"},
	})
	s.handleClosed(context.Background(), data)
	// No panic, no action — just verify it doesn't crash.
}

func TestSay_HandleClosed_IgnoresMessageWithoutSayLabel(t *testing.T) {
	s := &Say{
		daemon: newMockDaemon(),
		logger: slog.Default(),
		seen:   make(map[string]bool),
	}

	// Message bead without "say" label should be ignored.
	data := marshalSSEBeadPayload(BeadEvent{
		ID:     "bd-msg1",
		Type:   "message",
		Labels: []string{"from:test-agent"},
	})
	s.handleClosed(context.Background(), data)
	// No crash, no action.
}

func TestSay_HandleClosed_ProcessesSayMessage(t *testing.T) {
	s := &Say{
		daemon: newMockDaemon(),
		logger: slog.Default(),
		seen:   make(map[string]bool),
		// bot is nil — Slack post will be skipped, but processing still happens.
	}

	data := marshalSSEBeadPayload(BeadEvent{
		ID:     "bd-say1",
		Type:   "message",
		Labels: []string{"say", "from:test-agent"},
		Fields: map[string]string{
			"source_agent": "test-agent",
			"text":         "Build complete, MR ready for review",
		},
	})
	s.handleClosed(context.Background(), data)

	// Verify dedup: same bead should be skipped on replay.
	if !s.alreadySeen("bd-say1") {
		t.Error("expected bead to be marked as seen after processing")
	}
}

func TestSay_HandleClosed_Dedup(t *testing.T) {
	s := &Say{
		daemon: newMockDaemon(),
		logger: slog.Default(),
		seen:   make(map[string]bool),
	}

	data := marshalSSEBeadPayload(BeadEvent{
		ID:     "bd-say2",
		Type:   "message",
		Labels: []string{"say", "from:agent-1"},
		Fields: map[string]string{
			"source_agent": "agent-1",
			"text":         "First message",
		},
	})

	// First call processes it.
	s.handleClosed(context.Background(), data)
	if !s.alreadySeen("bd-say2") {
		t.Error("expected bead to be seen after first call")
	}

	// Second call should be deduped (no panic, no double processing).
	s.handleClosed(context.Background(), data)
}

func TestExtractSayAgent(t *testing.T) {
	tests := []struct {
		name string
		bead BeadEvent
		want string
	}{
		{
			name: "from fields",
			bead: BeadEvent{
				Fields: map[string]string{"source_agent": "gasboat/crew/my-agent"},
				Labels: []string{"from:other"},
			},
			want: "my-agent",
		},
		{
			name: "from label",
			bead: BeadEvent{
				Labels: []string{"say", "from:label-agent"},
			},
			want: "label-agent",
		},
		{
			name: "from created_by",
			bead: BeadEvent{
				CreatedBy: "gasboat/crew/creator-agent",
			},
			want: "creator-agent",
		},
		{
			name: "empty",
			bead: BeadEvent{},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSayAgent(tt.bead)
			if got != tt.want {
				t.Errorf("extractSayAgent() = %q, want %q", got, tt.want)
			}
		})
	}
}
