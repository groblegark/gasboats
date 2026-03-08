package client

import (
	"context"
	"encoding/json"
	"fmt"

	beadsv1 "github.com/groblegark/kbeads/gen/beads/v1"
	"github.com/groblegark/kbeads/internal/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// GRPCClient implements BeadsClient using the gRPC transport.
type GRPCClient struct {
	conn   *grpc.ClientConn
	client beadsv1.BeadsServiceClient
}

// tokenCredentials implements grpc.PerRPCCredentials to inject a Bearer token.
type tokenCredentials struct {
	token string
}

func (t tokenCredentials) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + t.token}, nil
}

func (t tokenCredentials) RequireTransportSecurity() bool { return false }

// NewGRPCClient connects to the given gRPC address and returns a client.
// When token is non-empty, per-RPC credentials are attached.
func NewGRPCClient(addr, token string) (*GRPCClient, error) {
	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if token != "" {
		opts = append(opts, grpc.WithPerRPCCredentials(tokenCredentials{token: token}))
	}
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, fmt.Errorf("grpc dial: %w", err)
	}
	return &GRPCClient{
		conn:   conn,
		client: beadsv1.NewBeadsServiceClient(conn),
	}, nil
}

func (c *GRPCClient) Close() error {
	return c.conn.Close()
}

// --- Bead CRUD ---

func (c *GRPCClient) CreateBead(ctx context.Context, req *CreateBeadRequest) (*model.Bead, error) {
	pbReq := &beadsv1.CreateBeadRequest{
		Title:       req.Title,
		Kind:        req.Kind,
		Type:        req.Type,
		Description: req.Description,
		Notes:       req.Notes,
		Priority:    int32(req.Priority),
		Assignee:    req.Assignee,
		Owner:       req.Owner,
		Labels:      req.Labels,
		CreatedBy:   req.CreatedBy,
		Fields:      req.Fields,
	}
	if req.DueAt != nil {
		pbReq.DueAt = timestamppb.New(*req.DueAt)
	}
	if req.DeferUntil != nil {
		pbReq.DeferUntil = timestamppb.New(*req.DeferUntil)
	}

	resp, err := c.client.CreateBead(ctx, pbReq)
	if err != nil {
		return nil, err
	}
	return protoBeadToModel(resp.GetBead()), nil
}

func (c *GRPCClient) GetBead(ctx context.Context, id string) (*model.Bead, error) {
	resp, err := c.client.GetBead(ctx, &beadsv1.GetBeadRequest{Id: id})
	if err != nil {
		return nil, err
	}
	return protoBeadToModel(resp.GetBead()), nil
}

func (c *GRPCClient) ListBeads(ctx context.Context, req *ListBeadsRequest) (*ListBeadsResponse, error) {
	pbReq := &beadsv1.ListBeadsRequest{
		Status:   req.Status,
		Type:     req.Type,
		Kind:     req.Kind,
		Labels:   req.Labels,
		Assignee: req.Assignee,
		Search:   req.Search,
		Sort:     req.Sort,
		Limit:    int32(req.Limit),
		Offset:   int32(req.Offset),
	}
	if req.Priority != nil {
		pbReq.Priority = wrapperspb.Int32(int32(*req.Priority))
	}

	resp, err := c.client.ListBeads(ctx, pbReq)
	if err != nil {
		return nil, err
	}

	beads := make([]*model.Bead, 0, len(resp.GetBeads()))
	for _, pb := range resp.GetBeads() {
		beads = append(beads, protoBeadToModel(pb))
	}
	return &ListBeadsResponse{
		Beads: beads,
		Total: int(resp.GetTotal()),
	}, nil
}

func (c *GRPCClient) UpdateBead(ctx context.Context, id string, req *UpdateBeadRequest) (*model.Bead, error) {
	pbReq := &beadsv1.UpdateBeadRequest{Id: id}
	if req.Title != nil {
		pbReq.Title = req.Title
	}
	if req.Description != nil {
		pbReq.Description = req.Description
	}
	if req.Notes != nil {
		pbReq.Notes = req.Notes
	}
	if req.Status != nil {
		pbReq.Status = req.Status
	}
	if req.Priority != nil {
		p := int32(*req.Priority)
		pbReq.Priority = &p
	}
	if req.Assignee != nil {
		pbReq.Assignee = req.Assignee
	}
	if req.Owner != nil {
		pbReq.Owner = req.Owner
	}
	if req.DueAt != nil {
		pbReq.DueAt = timestamppb.New(*req.DueAt)
	}
	if req.DeferUntil != nil {
		pbReq.DeferUntil = timestamppb.New(*req.DeferUntil)
	}
	if req.Fields != nil {
		pbReq.Fields = req.Fields
	}
	if len(req.Labels) > 0 {
		pbReq.Labels = req.Labels
	}

	resp, err := c.client.UpdateBead(ctx, pbReq)
	if err != nil {
		return nil, err
	}
	return protoBeadToModel(resp.GetBead()), nil
}

func (c *GRPCClient) CloseBead(ctx context.Context, id string, closedBy string) (*model.Bead, error) {
	resp, err := c.client.CloseBead(ctx, &beadsv1.CloseBeadRequest{
		Id:       id,
		ClosedBy: closedBy,
	})
	if err != nil {
		return nil, err
	}
	return protoBeadToModel(resp.GetBead()), nil
}

func (c *GRPCClient) DeleteBead(ctx context.Context, id string) error {
	_, err := c.client.DeleteBead(ctx, &beadsv1.DeleteBeadRequest{Id: id})
	return err
}

// --- Dependencies ---

func (c *GRPCClient) AddDependency(ctx context.Context, req *AddDependencyRequest) (*model.Dependency, error) {
	resp, err := c.client.AddDependency(ctx, &beadsv1.AddDependencyRequest{
		BeadId:      req.BeadID,
		DependsOnId: req.DependsOnID,
		Type:        req.Type,
		CreatedBy:   req.CreatedBy,
	})
	if err != nil {
		return nil, err
	}
	return protoDependencyToModel(resp.GetDependency()), nil
}

func (c *GRPCClient) RemoveDependency(ctx context.Context, beadID, dependsOnID, depType string) error {
	_, err := c.client.RemoveDependency(ctx, &beadsv1.RemoveDependencyRequest{
		BeadId:      beadID,
		DependsOnId: dependsOnID,
		Type:        depType,
	})
	return err
}

func (c *GRPCClient) GetDependencies(ctx context.Context, beadID string) ([]*model.Dependency, error) {
	resp, err := c.client.GetDependencies(ctx, &beadsv1.GetDependenciesRequest{BeadId: beadID})
	if err != nil {
		return nil, err
	}
	deps := make([]*model.Dependency, 0, len(resp.GetDependencies()))
	for _, pb := range resp.GetDependencies() {
		deps = append(deps, protoDependencyToModel(pb))
	}
	return deps, nil
}

func (c *GRPCClient) GetReverseDependencies(_ context.Context, _ string) ([]*model.Dependency, error) {
	return nil, fmt.Errorf("GetReverseDependencies is not supported over gRPC transport; use --transport=http")
}

// --- Labels ---

func (c *GRPCClient) AddLabel(ctx context.Context, beadID, label string) (*model.Bead, error) {
	resp, err := c.client.AddLabel(ctx, &beadsv1.AddLabelRequest{
		BeadId: beadID,
		Label:  label,
	})
	if err != nil {
		return nil, err
	}
	return protoBeadToModel(resp.GetBead()), nil
}

func (c *GRPCClient) RemoveLabel(ctx context.Context, beadID, label string) error {
	_, err := c.client.RemoveLabel(ctx, &beadsv1.RemoveLabelRequest{
		BeadId: beadID,
		Label:  label,
	})
	return err
}

func (c *GRPCClient) GetLabels(ctx context.Context, beadID string) ([]string, error) {
	resp, err := c.client.GetLabels(ctx, &beadsv1.GetLabelsRequest{BeadId: beadID})
	if err != nil {
		return nil, err
	}
	return resp.GetLabels(), nil
}

// --- Comments ---

func (c *GRPCClient) AddComment(ctx context.Context, beadID, author, text string) (*model.Comment, error) {
	resp, err := c.client.AddComment(ctx, &beadsv1.AddCommentRequest{
		BeadId: beadID,
		Author: author,
		Text:   text,
	})
	if err != nil {
		return nil, err
	}
	return protoCommentToModel(resp.GetComment()), nil
}

func (c *GRPCClient) GetComments(ctx context.Context, beadID string) ([]*model.Comment, error) {
	resp, err := c.client.GetComments(ctx, &beadsv1.GetCommentsRequest{BeadId: beadID})
	if err != nil {
		return nil, err
	}
	comments := make([]*model.Comment, 0, len(resp.GetComments()))
	for _, pb := range resp.GetComments() {
		comments = append(comments, protoCommentToModel(pb))
	}
	return comments, nil
}

// --- Events ---

func (c *GRPCClient) GetEvents(ctx context.Context, beadID string) ([]*model.Event, error) {
	resp, err := c.client.GetEvents(ctx, &beadsv1.GetEventsRequest{BeadId: beadID})
	if err != nil {
		return nil, err
	}
	events := make([]*model.Event, 0, len(resp.GetEvents()))
	for _, pb := range resp.GetEvents() {
		events = append(events, protoEventToModel(pb))
	}
	return events, nil
}

// --- Config ---

func (c *GRPCClient) SetConfig(ctx context.Context, key string, value json.RawMessage) (*model.Config, error) {
	resp, err := c.client.SetConfig(ctx, &beadsv1.SetConfigRequest{
		Key:   key,
		Value: value,
	})
	if err != nil {
		return nil, err
	}
	return protoConfigToModel(resp.GetConfig()), nil
}

func (c *GRPCClient) GetConfig(ctx context.Context, key string) (*model.Config, error) {
	resp, err := c.client.GetConfig(ctx, &beadsv1.GetConfigRequest{Key: key})
	if err != nil {
		return nil, err
	}
	return protoConfigToModel(resp.GetConfig()), nil
}

func (c *GRPCClient) ListConfigs(ctx context.Context, namespace string) ([]*model.Config, error) {
	resp, err := c.client.ListConfigs(ctx, &beadsv1.ListConfigsRequest{Namespace: namespace})
	if err != nil {
		return nil, err
	}
	configs := make([]*model.Config, 0, len(resp.GetConfigs()))
	for _, pb := range resp.GetConfigs() {
		configs = append(configs, protoConfigToModel(pb))
	}
	return configs, nil
}

func (c *GRPCClient) DeleteConfig(ctx context.Context, key string) error {
	_, err := c.client.DeleteConfig(ctx, &beadsv1.DeleteConfigRequest{Key: key})
	return err
}

// --- Hooks ---

// EmitHook is not implemented over gRPC (no proto definition exists).
// Returns an error directing callers to use the HTTP transport.
func (c *GRPCClient) EmitHook(_ context.Context, _ *EmitHookRequest) (*EmitHookResponse, error) {
	return nil, fmt.Errorf("EmitHook is not supported over gRPC transport; use --transport=http")
}

// --- Roster ---

func (c *GRPCClient) GetAgentRoster(_ context.Context, _ int) (*AgentRosterResponse, error) {
	return nil, fmt.Errorf("GetAgentRoster is not supported over gRPC transport; use --transport=http")
}

// --- Gates ---

func (c *GRPCClient) ListGates(_ context.Context, _ string) ([]model.GateRow, error) {
	return nil, fmt.Errorf("not supported over gRPC")
}

func (c *GRPCClient) SatisfyGate(_ context.Context, _, _ string) error {
	return fmt.Errorf("not supported over gRPC")
}

func (c *GRPCClient) ClearGate(_ context.Context, _, _ string) error {
	return fmt.Errorf("not supported over gRPC")
}

// --- Health ---

func (c *GRPCClient) Health(ctx context.Context) (string, error) {
	resp, err := c.client.Health(ctx, &beadsv1.HealthRequest{})
	if err != nil {
		return "", err
	}
	return resp.GetStatus(), nil
}

// --- proto to model conversions ---

func protoBeadToModel(pb *beadsv1.Bead) *model.Bead {
	if pb == nil {
		return nil
	}
	b := &model.Bead{
		ID:          pb.GetId(),
		Slug:        pb.GetSlug(),
		Kind:        model.Kind(pb.GetKind()),
		Type:        model.BeadType(pb.GetType()),
		Title:       pb.GetTitle(),
		Description: pb.GetDescription(),
		Notes:       pb.GetNotes(),
		Status:      model.Status(pb.GetStatus()),
		Priority:    int(pb.GetPriority()),
		Assignee:    pb.GetAssignee(),
		Owner:       pb.GetOwner(),
		CreatedBy:   pb.GetCreatedBy(),
		Labels:      pb.GetLabels(),
	}
	if pb.GetCreatedAt() != nil {
		b.CreatedAt = pb.GetCreatedAt().AsTime()
	}
	if pb.GetUpdatedAt() != nil {
		b.UpdatedAt = pb.GetUpdatedAt().AsTime()
	}
	if pb.GetClosedAt() != nil {
		t := pb.GetClosedAt().AsTime()
		b.ClosedAt = &t
	}
	if pb.GetDueAt() != nil {
		t := pb.GetDueAt().AsTime()
		b.DueAt = &t
	}
	if pb.GetDeferUntil() != nil {
		t := pb.GetDeferUntil().AsTime()
		b.DeferUntil = &t
	}
	if len(pb.GetFields()) > 0 {
		b.Fields = json.RawMessage(pb.GetFields())
	}

	for _, d := range pb.GetDependencies() {
		b.Dependencies = append(b.Dependencies, protoDependencyToModel(d))
	}
	for _, c := range pb.GetComments() {
		b.Comments = append(b.Comments, protoCommentToModel(c))
	}

	return b
}

func protoDependencyToModel(pb *beadsv1.Dependency) *model.Dependency {
	if pb == nil {
		return nil
	}
	d := &model.Dependency{
		BeadID:      pb.GetBeadId(),
		DependsOnID: pb.GetDependsOnId(),
		Type:        model.DependencyType(pb.GetType()),
		CreatedBy:   pb.GetCreatedBy(),
	}
	if pb.GetCreatedAt() != nil {
		d.CreatedAt = pb.GetCreatedAt().AsTime()
	}
	return d
}

func protoCommentToModel(pb *beadsv1.Comment) *model.Comment {
	if pb == nil {
		return nil
	}
	c := &model.Comment{
		ID:     int64(pb.GetId()),
		BeadID: pb.GetBeadId(),
		Author: pb.GetAuthor(),
		Text:   pb.GetText(),
	}
	if pb.GetCreatedAt() != nil {
		c.CreatedAt = pb.GetCreatedAt().AsTime()
	}
	return c
}

func protoEventToModel(pb *beadsv1.Event) *model.Event {
	if pb == nil {
		return nil
	}
	e := &model.Event{
		ID:      int64(pb.GetId()),
		Topic:   pb.GetTopic(),
		BeadID:  pb.GetBeadId(),
		Actor:   pb.GetActor(),
		Payload: json.RawMessage(pb.GetPayload()),
	}
	if pb.GetCreatedAt() != nil {
		e.CreatedAt = pb.GetCreatedAt().AsTime()
	}
	return e
}

func protoConfigToModel(pb *beadsv1.Config) *model.Config {
	if pb == nil {
		return nil
	}
	c := &model.Config{
		Key:   pb.GetKey(),
		Value: json.RawMessage(pb.GetValue()),
	}
	if pb.GetCreatedAt() != nil {
		c.CreatedAt = pb.GetCreatedAt().AsTime()
	}
	if pb.GetUpdatedAt() != nil {
		c.UpdatedAt = pb.GetUpdatedAt().AsTime()
	}
	return c
}
