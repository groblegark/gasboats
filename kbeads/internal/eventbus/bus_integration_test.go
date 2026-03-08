package eventbus

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// startEmbeddedNATS creates an embedded NATS server with JetStream for testing.
func startEmbeddedNATS(t *testing.T) (*server.Server, nats.JetStreamContext) {
	t.Helper()
	opts := &server.Options{
		Host:      "127.0.0.1",
		Port:      -1, // random port
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("failed to create NATS server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server not ready")
	}
	t.Cleanup(func() { ns.Shutdown() })

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("failed to connect to NATS: %v", err)
	}
	t.Cleanup(func() { nc.Close() })

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("failed to create JetStream context: %v", err)
	}

	return ns, js
}

func TestEnsureStreams(t *testing.T) {
	_, js := startEmbeddedNATS(t)

	if err := EnsureStreams(js); err != nil {
		t.Fatalf("EnsureStreams failed: %v", err)
	}

	// Verify all streams were created.
	for _, name := range StreamNames {
		jsName := StreamNameForJetStream(name)
		info, err := js.StreamInfo(jsName)
		if err != nil {
			t.Errorf("stream %s (%s) not found: %v", name, jsName, err)
			continue
		}
		if info.Config.MaxMsgs != 10000 {
			t.Errorf("stream %s MaxMsgs = %d, want 10000", name, info.Config.MaxMsgs)
		}
	}

	// Calling EnsureStreams again should be idempotent.
	if err := EnsureStreams(js); err != nil {
		t.Fatalf("EnsureStreams (second call) failed: %v", err)
	}
}

func TestBusSetJetStream(t *testing.T) {
	_, js := startEmbeddedNATS(t)

	bus := New()
	if bus.JetStreamEnabled() {
		t.Error("new bus should not have JetStream")
	}

	bus.SetJetStream(js)
	if !bus.JetStreamEnabled() {
		t.Error("bus should have JetStream after SetJetStream")
	}
	if bus.JetStream() == nil {
		t.Error("JetStream() should not be nil")
	}

	bus.SetJetStream(nil)
	if bus.JetStreamEnabled() {
		t.Error("bus should not have JetStream after SetJetStream(nil)")
	}
}

func TestBusDispatchPublishesToJetStream(t *testing.T) {
	_, js := startEmbeddedNATS(t)
	if err := EnsureStreams(js); err != nil {
		t.Fatalf("EnsureStreams: %v", err)
	}

	bus := New()
	bus.SetJetStream(js)

	event := &Event{
		Type:      EventSessionStart,
		SessionID: "test-session",
		Actor:     "bright-hog",
	}

	result, err := bus.Dispatch(context.Background(), event)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if result == nil {
		t.Fatal("Dispatch returned nil result")
	}

	// Give JetStream a moment to persist.
	time.Sleep(50 * time.Millisecond)

	// Verify the event was published to the hooks stream.
	info, err := js.StreamInfo(StreamHookEvents)
	if err != nil {
		t.Fatalf("StreamInfo: %v", err)
	}
	if info.State.Msgs == 0 {
		t.Error("expected at least 1 message in HOOK_EVENTS stream")
	}
}

func TestPublishRaw(t *testing.T) {
	_, js := startEmbeddedNATS(t)
	if err := EnsureStreams(js); err != nil {
		t.Fatalf("EnsureStreams: %v", err)
	}

	bus := New()
	bus.SetJetStream(js)

	payload := MutationEventPayload{
		Type:    "MutationCreate",
		IssueID: "kd-abc123",
		Title:   "Test issue",
	}
	data, _ := json.Marshal(payload)

	bus.PublishRaw(SubjectMutationPrefix+"MutationCreate", data)

	time.Sleep(50 * time.Millisecond)

	info, err := js.StreamInfo(StreamMutationEvents)
	if err != nil {
		t.Fatalf("StreamInfo: %v", err)
	}
	if info.State.Msgs == 0 {
		t.Error("expected at least 1 message in MUTATION_EVENTS stream")
	}
}

func TestPublishRawNoJetStream(t *testing.T) {
	bus := New()
	// Should not panic when JetStream is not configured.
	bus.PublishRaw("mutations.MutationCreate", []byte(`{"test":true}`))
}

func TestDispatchWithDecisionEventScoping(t *testing.T) {
	_, js := startEmbeddedNATS(t)
	if err := EnsureStreams(js); err != nil {
		t.Fatalf("EnsureStreams: %v", err)
	}

	bus := New()
	bus.SetJetStream(js)

	// Subscribe to the scoped decision subject.
	sub, err := js.SubscribeSync("decisions.bright-hog.>")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	raw := json.RawMessage(`{"requested_by":"bright-hog","decision_id":"d-123"}`)
	event := &Event{
		Type: EventDecisionCreated,
		Raw:  raw,
	}

	_, err = bus.Dispatch(context.Background(), event)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("NextMsg: %v (event may not have been published to scoped subject)", err)
	}
	if msg.Subject != "decisions.bright-hog.DecisionCreated" {
		t.Errorf("subject = %q, want %q", msg.Subject, "decisions.bright-hog.DecisionCreated")
	}
}

func TestDispatchWithHookEventActorScoping(t *testing.T) {
	_, js := startEmbeddedNATS(t)
	if err := EnsureStreams(js); err != nil {
		t.Fatalf("EnsureStreams: %v", err)
	}

	bus := New()
	bus.SetJetStream(js)

	// Subscribe to agent-scoped hook events.
	sub, err := js.SubscribeSync("hooks.sharp-seal.>")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	event := &Event{
		Type:  EventStop,
		Actor: "sharp-seal",
	}

	_, err = bus.Dispatch(context.Background(), event)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("NextMsg: %v", err)
	}
	if msg.Subject != "hooks.sharp-seal.Stop" {
		t.Errorf("subject = %q, want %q", msg.Subject, "hooks.sharp-seal.Stop")
	}
}
