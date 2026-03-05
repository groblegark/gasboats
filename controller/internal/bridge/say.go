// Package bridge provides the say message watcher.
//
// Say watches the kbeads SSE event stream for closed message beads with the
// "say" label and relays them to the originating agent's Slack thread.
// This gives agents a simple way to post informational updates without
// knowing anything about Slack.
package bridge

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// SayConfig holds configuration for the Say watcher.
type SayConfig struct {
	Daemon BeadClient
	Bot    *Bot
	Logger *slog.Logger
}

// Say watches for closed message beads and relays them to Slack.
type Say struct {
	daemon BeadClient
	bot    *Bot
	logger *slog.Logger

	mu   sync.Mutex
	seen map[string]bool // dedup on SSE reconnect
}

// NewSay creates a new say message watcher.
func NewSay(cfg SayConfig) *Say {
	return &Say{
		daemon: cfg.Daemon,
		bot:    cfg.Bot,
		logger: cfg.Logger,
		seen:   make(map[string]bool),
	}
}

// RegisterHandlers registers SSE event handlers on the given stream for
// message bead closed events.
func (s *Say) RegisterHandlers(stream *SSEStream) {
	stream.On("beads.bead.closed", s.handleClosed)
	s.logger.Info("say watcher registered SSE handlers",
		"topics", []string{"beads.bead.closed"})
}

func (s *Say) handleClosed(ctx context.Context, data []byte) {
	bead := ParseBeadEvent(data)
	if bead == nil {
		return
	}

	// Only handle message beads with the "say" label.
	if bead.Type != "message" || !hasLabel(bead.Labels, "say") {
		return
	}

	// Dedup on SSE reconnect.
	if s.alreadySeen(bead.ID) {
		return
	}

	// Extract the source agent from labels or fields.
	agent := extractSayAgent(*bead)
	if agent == "" {
		s.logger.Warn("say bead has no source agent",
			"id", bead.ID)
		return
	}

	// Get the message text from fields (falls back to title).
	text := bead.Fields["text"]
	if text == "" {
		text = bead.Title
	}

	s.logger.Info("say message received",
		"id", bead.ID,
		"agent", agent,
		"text_length", len(text))

	// Format and post to the agent's Slack thread.
	if s.bot != nil {
		formatted := fmt.Sprintf(":speech_balloon: *%s*: %s", agent, text)
		s.bot.postAgentThreadMessage(ctx, agent, formatted)
	}
}

// extractSayAgent extracts the source agent name from a say bead.
// Checks fields first, then falls back to from: label.
func extractSayAgent(bead BeadEvent) string {
	if agent := bead.Fields["source_agent"]; agent != "" {
		return extractAgentName(agent)
	}
	for _, label := range bead.Labels {
		if strings.HasPrefix(label, "from:") {
			return extractAgentName(strings.TrimPrefix(label, "from:"))
		}
	}
	return extractAgentName(bead.CreatedBy)
}

// alreadySeen returns true if this bead has already been processed.
func (s *Say) alreadySeen(beadID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[beadID] {
		return true
	}
	s.seen[beadID] = true
	return false
}
