package eventbus

import "testing"

func TestStreamForSubject(t *testing.T) {
	tests := []struct {
		subject string
		want    string
	}{
		{"hooks.SessionStart", "hooks"},
		{"hooks._global.SessionStart", "hooks"},
		{"decisions.DecisionCreated", "decisions"},
		{"decisions._global.DecisionCreated", "decisions"},
		{"agents.AgentStarted", "agents"},
		{"mail.MailSent", "mail"},
		{"mutations.MutationCreate", "mutations"},
		{"config.ConfigSet", "config"},
		{"gate.GateSatisfied", "gate"},
		{"inbox.something", "inbox"},
		{"jack.jack.on", "jack"},
		{"unknown.something", ""},
		{"noseparator", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := StreamForSubject(tt.subject)
		if got != tt.want {
			t.Errorf("StreamForSubject(%q) = %q, want %q", tt.subject, got, tt.want)
		}
	}
}

func TestEventTypeFromSubject(t *testing.T) {
	tests := []struct {
		subject string
		want    string
	}{
		{"hooks.SessionStart", "SessionStart"},
		{"hooks._global.SessionStart", "SessionStart"},
		{"decisions._global.DecisionCreated", "DecisionCreated"},
		{"mutations.MutationCreate", "MutationCreate"},
		{"jack.jack.on", "on"},
		{"noseparator", "noseparator"},
		{"", ""},
	}
	for _, tt := range tests {
		got := EventTypeFromSubject(tt.subject)
		if got != tt.want {
			t.Errorf("EventTypeFromSubject(%q) = %q, want %q", tt.subject, got, tt.want)
		}
	}
}

func TestSubjectPrefixForStream(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"hooks", "hooks."},
		{"decisions", "decisions."},
		{"agents", "agents."},
		{"mail", "mail."},
		{"mutations", "mutations."},
		{"config", "config."},
		{"gate", "gate."},
		{"inbox", "inbox."},
		{"jack", "jack."},
		{"unknown", ""},
	}
	for _, tt := range tests {
		got := SubjectPrefixForStream(tt.name)
		if got != tt.want {
			t.Errorf("SubjectPrefixForStream(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestStreamNameForJetStream(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"hooks", StreamHookEvents},
		{"decisions", StreamDecisionEvents},
		{"agents", StreamAgentEvents},
		{"mail", StreamMailEvents},
		{"mutations", StreamMutationEvents},
		{"config", StreamConfigEvents},
		{"gate", StreamGateEvents},
		{"inbox", StreamInboxEvents},
		{"jack", StreamJackEvents},
		{"unknown", ""},
	}
	for _, tt := range tests {
		got := StreamNameForJetStream(tt.name)
		if got != tt.want {
			t.Errorf("StreamNameForJetStream(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestSubjectForEvent(t *testing.T) {
	tests := []struct {
		eventType EventType
		wantPfx   string
	}{
		{EventDecisionCreated, "decisions."},
		{EventDecisionResponded, "decisions."},
		{EventAgentStarted, "agents."},
		{EventAgentCrashed, "agents."},
		{EventMailSent, "mail."},
		{EventMutationCreate, "mutations."},
		{EventMutationStatus, "mutations."},
		{EventConfigSet, "config."},
		{EventGateSatisfied, "gate."},
		{EventJackOn, "jack."},
		{EventSessionStart, "hooks."},
		{EventPreToolUse, "hooks."},
		{EventStop, "hooks."},
	}
	for _, tt := range tests {
		got := SubjectForEvent(tt.eventType)
		if got != tt.wantPfx+string(tt.eventType) {
			t.Errorf("SubjectForEvent(%s) = %q, want %q", tt.eventType, got, tt.wantPfx+string(tt.eventType))
		}
	}
}

func TestSubjectForDecisionEvent(t *testing.T) {
	tests := []struct {
		eventType   EventType
		requestedBy string
		want        string
	}{
		{EventDecisionCreated, "sharp-seal", "decisions.sharp-seal.DecisionCreated"},
		{EventDecisionResponded, "", "decisions._global.DecisionResponded"},
		{EventDecisionExpired, "bright-hog", "decisions.bright-hog.DecisionExpired"},
	}
	for _, tt := range tests {
		got := SubjectForDecisionEvent(tt.eventType, tt.requestedBy)
		if got != tt.want {
			t.Errorf("SubjectForDecisionEvent(%s, %q) = %q, want %q", tt.eventType, tt.requestedBy, got, tt.want)
		}
	}
}

func TestSubjectForHookEvent(t *testing.T) {
	tests := []struct {
		eventType EventType
		actor     string
		want      string
	}{
		{EventStop, "bright-hog", "hooks.bright-hog.Stop"},
		{EventSessionStart, "", "hooks._global.SessionStart"},
		{EventPreToolUse, "sharp-seal", "hooks.sharp-seal.PreToolUse"},
	}
	for _, tt := range tests {
		got := SubjectForHookEvent(tt.eventType, tt.actor)
		if got != tt.want {
			t.Errorf("SubjectForHookEvent(%s, %q) = %q, want %q", tt.eventType, tt.actor, got, tt.want)
		}
	}
}

func TestStreamNamesContainsAllStreams(t *testing.T) {
	// Verify every stream in StreamNames has a valid prefix mapping.
	for _, name := range StreamNames {
		if SubjectPrefixForStream(name) == "" {
			t.Errorf("StreamNames contains %q but SubjectPrefixForStream returns empty", name)
		}
		if StreamNameForJetStream(name) == "" {
			t.Errorf("StreamNames contains %q but StreamNameForJetStream returns empty", name)
		}
	}
}
