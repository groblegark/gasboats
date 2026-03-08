package eventbus

import (
	"fmt"

	"github.com/nats-io/nats.go"
)

const (
	StreamHookEvents     = "HOOK_EVENTS"
	StreamDecisionEvents = "DECISION_EVENTS"
	StreamAgentEvents    = "AGENT_EVENTS"
	StreamMailEvents     = "MAIL_EVENTS"
	StreamMutationEvents = "MUTATION_EVENTS"
	StreamConfigEvents   = "CONFIG_EVENTS"
	StreamGateEvents     = "GATE_EVENTS"
	StreamInboxEvents    = "INBOX_EVENTS"
	StreamJackEvents     = "JACK_EVENTS"

	SubjectHookPrefix     = "hooks."
	SubjectDecisionPrefix = "decisions."
	SubjectAgentPrefix    = "agents."
	SubjectMailPrefix     = "mail."
	SubjectMutationPrefix = "mutations."
	SubjectConfigPrefix   = "config."
	SubjectGatePrefix     = "gate."
	SubjectInboxPrefix    = "inbox."
	SubjectJackPrefix     = "jack."
)

// StreamNames lists all known stream short names.
var StreamNames = []string{
	"hooks", "decisions", "agents", "mail",
	"mutations", "config", "gate", "inbox", "jack",
}

var streamToPrefix = map[string]string{
	"hooks":     SubjectHookPrefix,
	"decisions": SubjectDecisionPrefix,
	"agents":    SubjectAgentPrefix,
	"mail":      SubjectMailPrefix,
	"mutations": SubjectMutationPrefix,
	"config":    SubjectConfigPrefix,
	"gate":      SubjectGatePrefix,
	"inbox":     SubjectInboxPrefix,
	"jack":      SubjectJackPrefix,
}

var prefixToStream = map[string]string{
	"hooks":     "hooks",
	"decisions": "decisions",
	"agents":    "agents",
	"mail":      "mail",
	"mutations": "mutations",
	"config":    "config",
	"gate":      "gate",
	"inbox":     "inbox",
	"jack":      "jack",
}

// StreamForSubject returns the short stream name for a NATS subject.
func StreamForSubject(subject string) string {
	for i := 0; i < len(subject); i++ {
		if subject[i] == '.' {
			if name, ok := prefixToStream[subject[:i]]; ok {
				return name
			}
			break
		}
	}
	return ""
}

// EventTypeFromSubject extracts the event type from a NATS subject (last segment).
func EventTypeFromSubject(subject string) string {
	for i := len(subject) - 1; i >= 0; i-- {
		if subject[i] == '.' {
			return subject[i+1:]
		}
	}
	return subject
}

// SubjectPrefixForStream returns the NATS subject prefix for a short stream name.
func SubjectPrefixForStream(name string) string {
	return streamToPrefix[name]
}

// StreamNameForJetStream returns the JetStream stream constant for a short name.
func StreamNameForJetStream(name string) string {
	switch name {
	case "hooks":
		return StreamHookEvents
	case "decisions":
		return StreamDecisionEvents
	case "agents":
		return StreamAgentEvents
	case "mail":
		return StreamMailEvents
	case "mutations":
		return StreamMutationEvents
	case "config":
		return StreamConfigEvents
	case "gate":
		return StreamGateEvents
	case "inbox":
		return StreamInboxEvents
	case "jack":
		return StreamJackEvents
	}
	return ""
}

// SubjectForEvent returns the NATS subject for a given event type.
func SubjectForEvent(eventType EventType) string {
	if eventType.IsDecisionEvent() {
		return SubjectDecisionPrefix + string(eventType)
	}
	if eventType.IsAgentEvent() {
		return SubjectAgentPrefix + string(eventType)
	}
	if eventType.IsMailEvent() {
		return SubjectMailPrefix + string(eventType)
	}
	if eventType.IsMutationEvent() {
		return SubjectMutationPrefix + string(eventType)
	}
	if eventType.IsConfigEvent() {
		return SubjectConfigPrefix + string(eventType)
	}
	if eventType.IsGateEvent() {
		return SubjectGatePrefix + string(eventType)
	}
	if eventType.IsJackEvent() {
		return SubjectJackPrefix + string(eventType)
	}
	return SubjectHookPrefix + string(eventType)
}

// SubjectForDecisionEvent returns a scoped NATS subject for a decision event.
func SubjectForDecisionEvent(eventType EventType, requestedBy string) string {
	scope := "_global"
	if requestedBy != "" {
		scope = requestedBy
	}
	return SubjectDecisionPrefix + scope + "." + string(eventType)
}

// SubjectForHookEvent returns an agent-scoped NATS subject for a hook event.
func SubjectForHookEvent(eventType EventType, actor string) string {
	scope := "_global"
	if actor != "" {
		scope = actor
	}
	return SubjectHookPrefix + scope + "." + string(eventType)
}

// EnsureStreams creates the required JetStream streams if they don't already exist.
func EnsureStreams(js nats.JetStreamContext) error {
	streams := []struct {
		name    string
		subject string
	}{
		{StreamHookEvents, SubjectHookPrefix + ">"},
		{StreamDecisionEvents, SubjectDecisionPrefix + ">"},
		{StreamAgentEvents, SubjectAgentPrefix + ">"},
		{StreamMailEvents, SubjectMailPrefix + ">"},
		{StreamMutationEvents, SubjectMutationPrefix + ">"},
		{StreamConfigEvents, SubjectConfigPrefix + ">"},
		{StreamGateEvents, SubjectGatePrefix + ">"},
		{StreamInboxEvents, SubjectInboxPrefix + ">"},
		{StreamJackEvents, SubjectJackPrefix + ">"},
	}

	for _, s := range streams {
		if _, err := js.StreamInfo(s.name); err != nil {
			_, err = js.AddStream(&nats.StreamConfig{
				Name:     s.name,
				Subjects: []string{s.subject},
				Storage:  nats.FileStorage,
				MaxMsgs:  10000,
				MaxBytes: 100 << 20, // 100MB
			})
			if err != nil {
				return fmt.Errorf("create %s stream: %w", s.name, err)
			}
		}
	}
	return nil
}
