package bridge

import (
	"context"
	"time"

	"github.com/slack-go/slack"
)

// threadValidationInterval is how often thread bindings are validated.
const threadValidationInterval = 6 * time.Hour

// startPeriodicThreadValidation runs ValidateThreadBindings at a fixed interval
// until ctx is cancelled. Invalid bindings (deleted threads, archived channels)
// are removed from state to prevent unbounded growth.
func (b *Bot) startPeriodicThreadValidation(ctx context.Context) {
	ticker := time.NewTicker(threadValidationInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.validateThreadBindings(ctx)
		}
	}
}

// validateThreadBindings checks all thread-agent mappings against the Slack API
// and removes entries where the thread no longer exists (channel archived,
// thread deleted, or API returns an error indicating inaccessibility).
func (b *Bot) validateThreadBindings(ctx context.Context) {
	if b.state == nil || b.api == nil {
		return
	}

	bindings := b.state.AllThreadAgents()
	if len(bindings) == 0 {
		return
	}

	b.logger.Info("thread-validate: starting scan", "bindings", len(bindings))

	removed := 0
	errors := 0
	for key := range bindings {
		// Parse "channel:thread_ts" key.
		channel, threadTS := parseThreadAgentKey(key)
		if channel == "" || threadTS == "" {
			continue
		}

		// Probe the thread with a minimal API call.
		if !b.isThreadAccessible(ctx, channel, threadTS) {
			// Remove the stale binding and its listen flag.
			if err := b.state.RemoveThreadAgent(channel, threadTS); err != nil {
				b.logger.Warn("thread-validate: failed to remove stale binding",
					"channel", channel, "thread_ts", threadTS, "error", err)
				errors++
				continue
			}
			_ = b.state.RemoveListenThread(channel, threadTS)
			removed++
		}
	}

	if removed > 0 || errors > 0 {
		b.logger.Info("thread-validate: scan complete",
			"checked", len(bindings), "removed", removed, "errors", errors)
	} else {
		b.logger.Info("thread-validate: all bindings valid", "checked", len(bindings))
	}
}

// parseThreadAgentKey splits a "channel:thread_ts" key into its components.
func parseThreadAgentKey(key string) (channel, threadTS string) {
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return key[:i], key[i+1:]
		}
	}
	return "", ""
}

// isThreadAccessible checks if a Slack thread is still accessible by fetching
// exactly one reply. Returns false if the channel is archived, the thread is
// deleted, or the API returns channel_not_found / thread_not_found.
func (b *Bot) isThreadAccessible(ctx context.Context, channel, threadTS string) bool {
	_, _, _, err := b.api.GetConversationRepliesContext(ctx, &slack.GetConversationRepliesParameters{
		ChannelID: channel,
		Timestamp: threadTS,
		Limit:     1,
		Inclusive:  true,
	})
	if err == nil {
		return true
	}

	// Slack API errors that indicate the thread/channel is gone.
	errStr := err.Error()
	switch errStr {
	case "channel_not_found", "thread_not_found", "is_archived",
		"not_in_channel", "missing_scope":
		return false
	}

	// Treat other errors (rate limit, network) as "accessible" to avoid
	// false-positive removals. We'll catch them on the next cycle.
	b.logger.Debug("thread-validate: API error treated as accessible",
		"channel", channel, "thread_ts", threadTS, "error", err)
	return true
}
