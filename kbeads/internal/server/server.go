package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	beadsv1 "github.com/groblegark/kbeads/gen/beads/v1"
	"github.com/groblegark/kbeads/internal/eventbus"
	"github.com/groblegark/kbeads/internal/events"
	"github.com/groblegark/kbeads/internal/hooks"
	"github.com/groblegark/kbeads/internal/model"
	"github.com/groblegark/kbeads/internal/presence"
	"github.com/groblegark/kbeads/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// BeadsServer implements the beadsv1.BeadsServiceServer interface.
type BeadsServer struct {
	beadsv1.UnimplementedBeadsServiceServer
	store        store.Store
	publisher    events.Publisher
	bus          *eventbus.Bus
	sseHub       *sseHub
	hooksHandler *hooks.Handler
	Presence     *presence.Tracker
}

// NewBeadsServer returns a new BeadsServer backed by the given store and publisher.
func NewBeadsServer(s store.Store, p events.Publisher) *BeadsServer {
	return &BeadsServer{
		store:        s,
		publisher:    p,
		sseHub:       newSSEHub(),
		hooksHandler: hooks.NewHandler(s, slog.Default()),
		Presence:     presence.New(),
	}
}

// SetBus attaches an event bus for JetStream publishing. When set, mutation
// events are published to JetStream streams in addition to basic NATS pub/sub.
func (s *BeadsServer) SetBus(bus *eventbus.Bus) {
	s.bus = bus
}

// Bus returns the event bus, or nil if not configured.
func (s *BeadsServer) Bus() *eventbus.Bus {
	return s.bus
}

// recordAndPublish persists an event to the store and publishes it to NATS.
// Both operations are best-effort; failures are logged but do not block the caller.
func (s *BeadsServer) recordAndPublish(ctx context.Context, topic, beadID, actor string, event any) {
	payload, err := json.Marshal(event)
	if err != nil {
		slog.Warn("failed to marshal event", "topic", topic, "bead_id", beadID, "error", err)
		return
	}
	if err := s.store.RecordEvent(ctx, &model.Event{
		Topic:   topic,
		BeadID:  beadID,
		Actor:   actor,
		Payload: payload,
	}); err != nil {
		slog.Warn("failed to record event", "topic", topic, "bead_id", beadID, "error", err)
	}
	if err := s.publisher.Publish(ctx, topic, event); err != nil {
		slog.Warn("failed to publish event", "topic", topic, "bead_id", beadID, "error", err)
	}
	s.broadcastEvent(topic, event)

	// Publish to JetStream mutation stream if bus is configured.
	if s.bus != nil {
		if mutationType := topicToMutationType(topic); mutationType != "" {
			s.bus.PublishRaw(
				eventbus.SubjectMutationPrefix+mutationType,
				payload,
			)
		}
	}
}

// topicToMutationType maps a legacy event topic to a JetStream mutation event
// type suffix. Returns "" for topics that are not mutation events.
func topicToMutationType(topic string) string {
	switch topic {
	case events.TopicBeadCreated:
		return string(eventbus.EventMutationCreate)
	case events.TopicBeadUpdated:
		return string(eventbus.EventMutationUpdate)
	case events.TopicBeadClosed:
		return string(eventbus.EventMutationStatus)
	case events.TopicBeadDeleted:
		return string(eventbus.EventMutationDelete)
	case events.TopicCommentAdded:
		return string(eventbus.EventMutationComment)
	}
	return ""
}

// inputError indicates invalid user input.
// Transport layers map this to 400 / InvalidArgument.
type inputError string

func (e inputError) Error() string { return string(e) }

// AddDependency creates a dependency between two beads.
func (s *BeadsServer) AddDependency(ctx context.Context, req *beadsv1.AddDependencyRequest) (*beadsv1.AddDependencyResponse, error) {
	if req.GetBeadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "bead_id is required")
	}
	if req.GetDependsOnId() == "" {
		return nil, status.Error(codes.InvalidArgument, "depends_on_id is required")
	}

	now := time.Now().UTC()
	dep := &model.Dependency{
		BeadID:      req.GetBeadId(),
		DependsOnID: req.GetDependsOnId(),
		Type:        model.DependencyType(req.GetType()),
		CreatedAt:   now,
		CreatedBy:   req.GetCreatedBy(),
	}

	if err := s.store.AddDependency(ctx, dep); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to add dependency: %v", err)
	}

	s.recordAndPublish(ctx, events.TopicDependencyAdded, dep.BeadID, dep.CreatedBy, events.DependencyAdded{Dependency: dep})

	return &beadsv1.AddDependencyResponse{
		Dependency: dependencyToProto(dep),
	}, nil
}

// RemoveDependency removes a dependency between two beads.
func (s *BeadsServer) RemoveDependency(ctx context.Context, req *beadsv1.RemoveDependencyRequest) (*beadsv1.RemoveDependencyResponse, error) {
	if req.GetBeadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "bead_id is required")
	}
	if req.GetDependsOnId() == "" {
		return nil, status.Error(codes.InvalidArgument, "depends_on_id is required")
	}

	if err := s.store.RemoveDependency(ctx, req.GetBeadId(), req.GetDependsOnId(), model.DependencyType(req.GetType())); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to remove dependency: %v", err)
	}

	s.recordAndPublish(ctx, events.TopicDependencyRemoved, req.GetBeadId(), "", events.DependencyRemoved{
		BeadID:      req.GetBeadId(),
		DependsOnID: req.GetDependsOnId(),
		Type:        req.GetType(),
	})

	return &beadsv1.RemoveDependencyResponse{}, nil
}

// GetDependencies returns all dependencies for a bead.
func (s *BeadsServer) GetDependencies(ctx context.Context, req *beadsv1.GetDependenciesRequest) (*beadsv1.GetDependenciesResponse, error) {
	if req.GetBeadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "bead_id is required")
	}

	deps, err := s.store.GetDependencies(ctx, req.GetBeadId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get dependencies: %v", err)
	}

	pbDeps := make([]*beadsv1.Dependency, 0, len(deps))
	for _, d := range deps {
		pbDeps = append(pbDeps, dependencyToProto(d))
	}

	return &beadsv1.GetDependenciesResponse{Dependencies: pbDeps}, nil
}

// AddLabel adds a label to a bead.
func (s *BeadsServer) AddLabel(ctx context.Context, req *beadsv1.AddLabelRequest) (*beadsv1.AddLabelResponse, error) {
	if req.GetBeadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "bead_id is required")
	}
	if req.GetLabel() == "" {
		return nil, status.Error(codes.InvalidArgument, "label is required")
	}

	if err := s.store.AddLabel(ctx, req.GetBeadId(), req.GetLabel()); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to add label: %v", err)
	}

	s.recordAndPublish(ctx, events.TopicLabelAdded, req.GetBeadId(), "", events.LabelAdded{
		BeadID: req.GetBeadId(),
		Label:  req.GetLabel(),
	})

	// Fetch the updated bead to return.
	bead, err := s.store.GetBead(ctx, req.GetBeadId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get bead after adding label: %v", err)
	}
	if bead == nil {
		return nil, status.Error(codes.NotFound, "bead not found")
	}

	return &beadsv1.AddLabelResponse{Bead: beadToProto(bead)}, nil
}

// RemoveLabel removes a label from a bead.
func (s *BeadsServer) RemoveLabel(ctx context.Context, req *beadsv1.RemoveLabelRequest) (*beadsv1.RemoveLabelResponse, error) {
	if req.GetBeadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "bead_id is required")
	}
	if req.GetLabel() == "" {
		return nil, status.Error(codes.InvalidArgument, "label is required")
	}

	if err := s.store.RemoveLabel(ctx, req.GetBeadId(), req.GetLabel()); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to remove label: %v", err)
	}

	s.recordAndPublish(ctx, events.TopicLabelRemoved, req.GetBeadId(), "", events.LabelRemoved{
		BeadID: req.GetBeadId(),
		Label:  req.GetLabel(),
	})

	return &beadsv1.RemoveLabelResponse{}, nil
}

// GetLabels returns all labels for a bead.
func (s *BeadsServer) GetLabels(ctx context.Context, req *beadsv1.GetLabelsRequest) (*beadsv1.GetLabelsResponse, error) {
	if req.GetBeadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "bead_id is required")
	}

	labels, err := s.store.GetLabels(ctx, req.GetBeadId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get labels: %v", err)
	}

	return &beadsv1.GetLabelsResponse{Labels: labels}, nil
}

// AddComment adds a comment to a bead.
func (s *BeadsServer) AddComment(ctx context.Context, req *beadsv1.AddCommentRequest) (*beadsv1.AddCommentResponse, error) {
	if req.GetBeadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "bead_id is required")
	}
	if req.GetText() == "" {
		return nil, status.Error(codes.InvalidArgument, "text is required")
	}

	now := time.Now().UTC()
	comment := &model.Comment{
		BeadID:    req.GetBeadId(),
		Author:    req.GetAuthor(),
		Text:      req.GetText(),
		CreatedAt: now,
	}

	if err := s.store.AddComment(ctx, comment); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to add comment: %v", err)
	}

	s.recordAndPublish(ctx, events.TopicCommentAdded, comment.BeadID, comment.Author, events.CommentAdded{Comment: comment})

	return &beadsv1.AddCommentResponse{
		Comment: commentToProto(comment),
	}, nil
}

// GetComments returns all comments for a bead.
func (s *BeadsServer) GetComments(ctx context.Context, req *beadsv1.GetCommentsRequest) (*beadsv1.GetCommentsResponse, error) {
	if req.GetBeadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "bead_id is required")
	}

	comments, err := s.store.GetComments(ctx, req.GetBeadId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get comments: %v", err)
	}

	pbComments := make([]*beadsv1.Comment, 0, len(comments))
	for _, c := range comments {
		pbComments = append(pbComments, commentToProto(c))
	}

	return &beadsv1.GetCommentsResponse{Comments: pbComments}, nil
}

// GetEvents returns all persisted events for a bead.
func (s *BeadsServer) GetEvents(ctx context.Context, req *beadsv1.GetEventsRequest) (*beadsv1.GetEventsResponse, error) {
	if req.GetBeadId() == "" {
		return nil, status.Error(codes.InvalidArgument, "bead_id is required")
	}

	evts, err := s.store.GetEvents(ctx, req.GetBeadId())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get events: %v", err)
	}

	pbEvents := make([]*beadsv1.Event, 0, len(evts))
	for _, e := range evts {
		pbEvents = append(pbEvents, eventToProto(e))
	}

	return &beadsv1.GetEventsResponse{Events: pbEvents}, nil
}

// Health returns the service health status.
func (s *BeadsServer) Health(_ context.Context, _ *beadsv1.HealthRequest) (*beadsv1.HealthResponse, error) {
	return &beadsv1.HealthResponse{Status: "ok"}, nil
}
