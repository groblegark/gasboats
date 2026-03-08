package events

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/groblegark/kbeads/internal/model"
	"github.com/nats-io/nats.go"
)

func TestNoopPublisher_Publish(t *testing.T) {
	pub := &NoopPublisher{}
	err := pub.Publish(context.Background(), TopicBeadCreated, BeadCreated{})
	if err != nil {
		t.Fatalf("NoopPublisher.Publish returned unexpected error: %v", err)
	}
}

func TestNoopPublisher_Close(t *testing.T) {
	pub := &NoopPublisher{}
	err := pub.Close()
	if err != nil {
		t.Fatalf("NoopPublisher.Close returned unexpected error: %v", err)
	}
}

func TestNoopPublisher_ImplementsPublisher(t *testing.T) {
	var _ Publisher = (*NoopPublisher)(nil)
}

func TestNATSPublisher_ImplementsPublisher(t *testing.T) {
	var _ Publisher = (*NATSPublisher)(nil)
}

func TestNATSPublisher_Publish(t *testing.T) {
	url := startTestNATS(t)

	pub, err := NewNATSPublisher(url)
	if err != nil {
		t.Fatalf("creating publisher: %v", err)
	}
	defer pub.Close()

	// Subscribe to capture published messages.
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connecting subscriber: %v", err)
	}
	defer nc.Close()

	ch := make(chan *nats.Msg, 1)
	sub, err := nc.ChanSubscribe(TopicBeadCreated, ch)
	if err != nil {
		t.Fatalf("subscribing: %v", err)
	}
	defer sub.Unsubscribe() //nolint:errcheck
	nc.Flush()

	event := BeadCreated{Bead: &model.Bead{ID: "kd-pub1", Title: "Test"}}
	if err := pub.Publish(context.Background(), TopicBeadCreated, event); err != nil {
		t.Fatalf("Publish error: %v", err)
	}
	pub.conn.Flush()

	select {
	case msg := <-ch:
		var got BeadCreated
		if err := json.Unmarshal(msg.Data, &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Bead.ID != "kd-pub1" {
			t.Errorf("got bead ID=%q, want %q", got.Bead.ID, "kd-pub1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for published message")
	}
}

func TestNATSPublisher_PublishMultipleTopics(t *testing.T) {
	url := startTestNATS(t)

	pub, err := NewNATSPublisher(url)
	if err != nil {
		t.Fatalf("creating publisher: %v", err)
	}
	defer pub.Close()

	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connecting subscriber: %v", err)
	}
	defer nc.Close()

	ch := make(chan *nats.Msg, 4)
	sub, err := nc.ChanSubscribe("beads.>", ch)
	if err != nil {
		t.Fatalf("subscribing: %v", err)
	}
	defer sub.Unsubscribe() //nolint:errcheck
	nc.Flush()

	for _, tc := range []struct {
		topic string
		event any
	}{
		{TopicBeadCreated, BeadCreated{Bead: &model.Bead{ID: "kd-1"}}},
		{TopicBeadDeleted, BeadDeleted{BeadID: "kd-2"}},
		{TopicLabelAdded, LabelAdded{BeadID: "kd-1", Label: "urgent"}},
		{TopicCommentAdded, CommentAdded{Comment: &model.Comment{ID: 1, BeadID: "kd-1"}}},
	} {
		if err := pub.Publish(context.Background(), tc.topic, tc.event); err != nil {
			t.Fatalf("Publish(%s): %v", tc.topic, err)
		}
	}
	pub.conn.Flush()

	for i := 0; i < 4; i++ {
		select {
		case <-ch:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for message %d", i)
		}
	}
}

func TestNATSPublisher_Close(t *testing.T) {
	url := startTestNATS(t)

	pub, err := NewNATSPublisher(url)
	if err != nil {
		t.Fatalf("creating publisher: %v", err)
	}

	if err := pub.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}

	// Publishing after close should fail.
	err = pub.Publish(context.Background(), TopicBeadCreated, BeadCreated{})
	if err == nil {
		t.Error("expected error publishing after close")
	}
}
