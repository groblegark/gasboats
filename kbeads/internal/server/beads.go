package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	beadsv1 "github.com/groblegark/kbeads/gen/beads/v1"
	"github.com/groblegark/kbeads/internal/events"
	"github.com/groblegark/kbeads/internal/idgen"
	"github.com/groblegark/kbeads/internal/model"
	"github.com/groblegark/kbeads/internal/store"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// createBeadInput holds transport-agnostic parameters for creating a bead.
type createBeadInput struct {
	Title       string          `json:"title"`
	Kind        string          `json:"kind"`
	Type        string          `json:"type"`
	Description string          `json:"description"`
	Notes       string          `json:"notes"`
	Priority    int             `json:"priority"`
	Assignee    string          `json:"assignee"`
	Owner       string          `json:"owner"`
	Labels      []string        `json:"labels"`
	CreatedBy   string          `json:"created_by"`
	Fields      json.RawMessage `json:"fields"`
	DueAt       *time.Time      `json:"due_at,omitempty"`
	DeferUntil  *time.Time      `json:"defer_until,omitempty"`
}

// createBead validates input, persists a new bead with labels, and publishes
// a BeadCreated event. Returns inputError for validation failures.
func (s *BeadsServer) createBead(ctx context.Context, in createBeadInput) (*model.Bead, error) {
	if in.Title == "" {
		return nil, inputError("title is required")
	}

	now := time.Now().UTC()
	id, err := idgen.Generate()
	if err != nil {
		return nil, fmt.Errorf("failed to generate ID: %w", err)
	}

	beadType := model.BeadType(in.Type)

	// Resolve deprecated type aliases (template→formula, bundle→molecule).
	if canonical, ok := model.TypeAliases[beadType]; ok {
		beadType = canonical
	}

	// Resolve type config to determine kind and field definitions.
	tc, err := s.resolveTypeConfig(ctx, beadType)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve type config: %w", err)
	}
	if tc == nil {
		return nil, inputError("unknown bead type " + string(beadType))
	}

	bead := &model.Bead{
		ID:          id,
		Kind:        tc.Kind,
		Type:        beadType,
		Title:       in.Title,
		Description: in.Description,
		Notes:       in.Notes,
		Status:      model.StatusOpen,
		Priority:    in.Priority,
		Assignee:    in.Assignee,
		Owner:       in.Owner,
		CreatedAt:   now,
		CreatedBy:   in.CreatedBy,
		UpdatedAt:   now,
		DueAt:       in.DueAt,
		DeferUntil:  in.DeferUntil,
		Labels:      in.Labels,
	}

	if len(in.Fields) > 0 {
		bead.Fields = in.Fields
	}

	if err := model.ValidateBead(bead); err != nil {
		return nil, inputError("invalid bead: " + err.Error())
	}

	if err := model.ValidateFields(bead.Fields, tc.Fields); err != nil {
		return nil, inputError("invalid fields: " + err.Error())
	}

	// Bug 5 fix: wrap CreateBead + label inserts in a transaction.
	err = s.store.RunInTransaction(ctx, func(tx store.Store) error {
		if err := tx.CreateBead(ctx, bead); err != nil {
			return fmt.Errorf("failed to create bead: %w", err)
		}
		for _, label := range bead.Labels {
			if err := tx.AddLabel(ctx, bead.ID, label); err != nil {
				return fmt.Errorf("failed to add label %q: %w", label, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	s.recordAndPublish(ctx, events.TopicBeadCreated, bead.ID, bead.CreatedBy, events.BeadCreated{Bead: bead})

	if bead.Type == model.TypeAdvice {
		s.recordAndPublish(ctx, events.TopicAdviceCreated, bead.ID, bead.CreatedBy, events.AdviceCreated{Bead: bead})
	}

	return bead, nil
}

// CreateBead validates the request, persists a new bead, publishes a BeadCreated event,
// and returns the full bead.
func (s *BeadsServer) CreateBead(ctx context.Context, req *beadsv1.CreateBeadRequest) (*beadsv1.CreateBeadResponse, error) {
	bead, err := s.createBead(ctx, createBeadInput{
		Title:       req.GetTitle(),
		Kind:        req.GetKind(),
		Type:        req.GetType(),
		Description: req.GetDescription(),
		Notes:       req.GetNotes(),
		Priority:    int(req.GetPriority()),
		Assignee:    req.GetAssignee(),
		Owner:       req.GetOwner(),
		Labels:      req.GetLabels(),
		CreatedBy:   req.GetCreatedBy(),
		Fields:      json.RawMessage(req.GetFields()),
		DueAt:       protoTimestamp(req.GetDueAt()),
		DeferUntil:  protoTimestamp(req.GetDeferUntil()),
	})
	if err != nil {
		var ie inputError
		if errors.As(err, &ie) {
			return nil, status.Error(codes.InvalidArgument, ie.Error())
		}
		return nil, status.Errorf(codes.Internal, "%v", err)
	}

	return &beadsv1.CreateBeadResponse{Bead: beadToProto(bead)}, nil
}

// GetBead retrieves a single bead by ID.
func (s *BeadsServer) GetBead(ctx context.Context, req *beadsv1.GetBeadRequest) (*beadsv1.GetBeadResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	bead, err := s.store.GetBead(ctx, req.GetId())
	if err != nil {
		return nil, storeError(err, "bead")
	}
	if bead == nil {
		return nil, status.Error(codes.NotFound, "bead not found")
	}

	return &beadsv1.GetBeadResponse{Bead: beadToProto(bead)}, nil
}

// ListBeads returns a filtered, paginated list of beads.
func (s *BeadsServer) ListBeads(ctx context.Context, req *beadsv1.ListBeadsRequest) (*beadsv1.ListBeadsResponse, error) {
	filter := model.BeadFilter{
		Assignee: req.GetAssignee(),
		Labels:   req.GetLabels(),
		Search:   req.GetSearch(),
		Sort:     req.GetSort(),
		Limit:    int(req.GetLimit()),
		Offset:   int(req.GetOffset()),
	}

	for _, st := range req.GetStatus() {
		filter.Status = append(filter.Status, model.Status(st))
	}
	for _, t := range req.GetType() {
		bt := model.BeadType(t)
		// Resolve deprecated type aliases (template→formula, bundle→molecule).
		if canonical, ok := model.TypeAliases[bt]; ok {
			bt = canonical
		}
		filter.Type = append(filter.Type, bt)
	}
	for _, k := range req.GetKind() {
		filter.Kind = append(filter.Kind, model.Kind(k))
	}
	if req.GetPriority() != nil {
		p := int(req.GetPriority().GetValue())
		filter.Priority = &p
	}
	if len(req.GetFieldFilters()) > 0 {
		filter.Fields = req.GetFieldFilters()
	}

	beads, total, err := s.store.ListBeads(ctx, filter)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to list beads: %v", err)
	}

	pbBeads := make([]*beadsv1.Bead, 0, len(beads))
	for _, b := range beads {
		pbBeads = append(pbBeads, beadToProto(b))
	}

	return &beadsv1.ListBeadsResponse{
		Beads: pbBeads,
		Total: int32(total),
	}, nil
}

// updateBeadInput holds transport-agnostic parameters for updating a bead.
// Pointer fields indicate optionality: nil means "don't change".
type updateBeadInput struct {
	Title       *string         `json:"title,omitempty"`
	Description *string         `json:"description,omitempty"`
	Notes       *string         `json:"notes,omitempty"`
	Status      *string         `json:"status,omitempty"`
	Priority    *int            `json:"priority,omitempty"`
	Assignee    *string         `json:"assignee,omitempty"`
	Owner       *string         `json:"owner,omitempty"`
	DueAt       *time.Time      `json:"due_at,omitempty"`
	DeferUntil  *time.Time      `json:"defer_until,omitempty"`
	Fields      json.RawMessage `json:"fields,omitempty"`
	Labels      []string        `json:"labels,omitempty"`

	// dueAtSet / deferUntilSet track whether the field was provided at all
	// (since a nil *time.Time means "clear the field", distinct from "not provided").
	dueAtSet      bool
	deferUntilSet bool
	labelsSet     bool
}

// updateBead applies partial updates to an existing bead, persists them,
// and publishes a BeadUpdated event. Returns inputError for validation failures.
func (s *BeadsServer) updateBead(ctx context.Context, id string, in updateBeadInput) (*model.Bead, error) {
	bead, err := s.store.GetBead(ctx, id)
	if err != nil {
		return nil, err
	}
	if bead == nil {
		return nil, sql.ErrNoRows
	}

	changes := make(map[string]any)

	if in.Title != nil {
		bead.Title = *in.Title
		changes["title"] = bead.Title
	}
	if in.Description != nil {
		bead.Description = *in.Description
		changes["description"] = bead.Description
	}
	if in.Notes != nil {
		bead.Notes = *in.Notes
		changes["notes"] = bead.Notes
	}
	if in.Status != nil {
		bead.Status = model.Status(*in.Status)
		changes["status"] = bead.Status
	}
	if in.Priority != nil {
		bead.Priority = *in.Priority
		changes["priority"] = bead.Priority
	}
	if in.Assignee != nil {
		bead.Assignee = *in.Assignee
		changes["assignee"] = bead.Assignee
	}
	if in.Owner != nil {
		bead.Owner = *in.Owner
		changes["owner"] = bead.Owner
	}

	// Bug 3 fix: treat zero time as "clear the field".
	if in.dueAtSet {
		if in.DueAt != nil && in.DueAt.IsZero() {
			bead.DueAt = nil
		} else {
			bead.DueAt = in.DueAt
		}
		changes["due_at"] = bead.DueAt
	}
	if in.deferUntilSet {
		if in.DeferUntil != nil && in.DeferUntil.IsZero() {
			bead.DeferUntil = nil
		} else {
			bead.DeferUntil = in.DeferUntil
		}
		changes["defer_until"] = bead.DeferUntil
	}

	if in.Fields != nil {
		// Merge incoming fields into existing fields (patch semantics).
		existing := make(map[string]any)
		if len(bead.Fields) > 0 {
			_ = json.Unmarshal(bead.Fields, &existing)
		}
		var patch map[string]any
		if err := json.Unmarshal(in.Fields, &patch); err == nil {
			for k, v := range patch {
				existing[k] = v
			}
		}
		merged, mergeErr := json.Marshal(existing)
		if mergeErr != nil {
			return nil, fmt.Errorf("failed to merge fields: %w", mergeErr)
		}
		bead.Fields = merged
		changes["fields"] = bead.Fields
	}
	if in.labelsSet {
		bead.Labels = in.Labels
		changes["labels"] = bead.Labels
	}

	// Reconcile ClosedAt with Status changes.
	if bead.Status == model.StatusClosed && bead.ClosedAt == nil {
		now := time.Now().UTC()
		bead.ClosedAt = &now
		changes["closed_at"] = bead.ClosedAt
	}
	if bead.Status != model.StatusClosed && bead.ClosedAt != nil {
		bead.ClosedAt = nil
		changes["closed_at"] = bead.ClosedAt
	}

	bead.UpdatedAt = time.Now().UTC()

	if err := model.ValidateBead(bead); err != nil {
		return nil, inputError("invalid bead: " + err.Error())
	}

	// Validate fields against type config if fields were changed.
	if _, ok := changes["fields"]; ok {
		tc, err := s.resolveTypeConfig(ctx, bead.Type)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve type config: %w", err)
		}
		if tc != nil {
			if err := model.ValidateFields(bead.Fields, tc.Fields); err != nil {
				return nil, inputError("invalid fields: " + err.Error())
			}
		}
	}

	if err := s.store.UpdateBead(ctx, bead); err != nil {
		return nil, fmt.Errorf("failed to update bead: %w", err)
	}

	// Bug 1 fix: reconcile labels in the store.
	if _, ok := changes["labels"]; ok {
		if err := s.reconcileLabels(ctx, bead.ID, bead.Labels); err != nil {
			return nil, fmt.Errorf("failed to reconcile labels: %w", err)
		}
	}

	s.recordAndPublish(ctx, events.TopicBeadUpdated, bead.ID, "", events.BeadUpdated{
		Bead:    bead,
		Changes: changes,
	})

	if bead.Type == model.TypeAdvice {
		s.recordAndPublish(ctx, events.TopicAdviceUpdated, bead.ID, "", events.AdviceUpdated{
			Bead:    bead,
			Changes: changes,
		})
	}

	return bead, nil
}

// reconcileLabels compares the desired labels with the existing labels in
// the store and adds/removes as needed.
func (s *BeadsServer) reconcileLabels(ctx context.Context, beadID string, newLabels []string) error {
	existing, err := s.store.GetLabels(ctx, beadID)
	if err != nil {
		return err
	}

	existingSet := make(map[string]struct{}, len(existing))
	for _, l := range existing {
		existingSet[l] = struct{}{}
	}
	newSet := make(map[string]struct{}, len(newLabels))
	for _, l := range newLabels {
		newSet[l] = struct{}{}
	}

	// Remove labels that are no longer desired.
	for _, l := range existing {
		if _, ok := newSet[l]; !ok {
			if err := s.store.RemoveLabel(ctx, beadID, l); err != nil {
				return err
			}
		}
	}
	// Add labels that are new.
	for _, l := range newLabels {
		if _, ok := existingSet[l]; !ok {
			if err := s.store.AddLabel(ctx, beadID, l); err != nil {
				return err
			}
		}
	}

	return nil
}

// UpdateBead applies partial updates to an existing bead.
func (s *BeadsServer) UpdateBead(ctx context.Context, req *beadsv1.UpdateBeadRequest) (*beadsv1.UpdateBeadResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	in := updateBeadInput{}
	if req.Title != nil {
		in.Title = req.Title
	}
	if req.Description != nil {
		in.Description = req.Description
	}
	if req.Notes != nil {
		in.Notes = req.Notes
	}
	if req.Status != nil {
		in.Status = req.Status
	}
	if req.Priority != nil {
		p := int(*req.Priority)
		in.Priority = &p
	}
	if req.Assignee != nil {
		in.Assignee = req.Assignee
	}
	if req.Owner != nil {
		in.Owner = req.Owner
	}
	if req.DueAt != nil {
		in.DueAt = protoTimestamp(req.DueAt)
		in.dueAtSet = true
	}
	if req.DeferUntil != nil {
		in.DeferUntil = protoTimestamp(req.DeferUntil)
		in.deferUntilSet = true
	}
	if req.Fields != nil {
		in.Fields = json.RawMessage(req.Fields)
	}
	if len(req.Labels) > 0 {
		in.Labels = req.Labels
		in.labelsSet = true
	}

	bead, err := s.updateBead(ctx, req.GetId(), in)
	if err != nil {
		var ie inputError
		if errors.As(err, &ie) {
			return nil, status.Error(codes.InvalidArgument, ie.Error())
		}
		if errors.Is(err, sql.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "bead not found")
		}
		return nil, status.Errorf(codes.Internal, "%v", err)
	}

	return &beadsv1.UpdateBeadResponse{Bead: beadToProto(bead)}, nil
}

// CloseBead marks a bead as closed.
func (s *BeadsServer) CloseBead(ctx context.Context, req *beadsv1.CloseBeadRequest) (*beadsv1.CloseBeadResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	bead, err := s.store.CloseBead(ctx, req.GetId(), req.GetClosedBy())
	if err != nil {
		return nil, storeError(err, "bead")
	}
	if bead == nil {
		return nil, status.Error(codes.NotFound, "bead not found")
	}

	s.recordAndPublish(ctx, events.TopicBeadClosed, bead.ID, req.GetClosedBy(), events.BeadClosed{
		Bead:     bead,
		ClosedBy: req.GetClosedBy(),
	})

	if bead.Type == model.TypeAdvice {
		s.recordAndPublish(ctx, events.TopicAdviceDeleted, bead.ID, req.GetClosedBy(), events.AdviceDeleted{BeadID: bead.ID})
	}

	return &beadsv1.CloseBeadResponse{Bead: beadToProto(bead)}, nil
}

// DeleteBead removes a bead by ID.
func (s *BeadsServer) DeleteBead(ctx context.Context, req *beadsv1.DeleteBeadRequest) (*beadsv1.DeleteBeadResponse, error) {
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	if err := s.store.DeleteBead(ctx, req.GetId()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, status.Error(codes.NotFound, "bead not found")
		}
		var nfErr interface{ NotFound() bool }
		if errors.As(err, &nfErr) && nfErr.NotFound() {
			return nil, status.Error(codes.NotFound, "bead not found")
		}
		return nil, status.Errorf(codes.Internal, "failed to delete bead: %v", err)
	}

	s.recordAndPublish(ctx, events.TopicBeadDeleted, req.GetId(), "", events.BeadDeleted{BeadID: req.GetId()})

	return &beadsv1.DeleteBeadResponse{}, nil
}

// reportTypeFromLabels extracts the report template name from labels.
// E.g., "report:summary" → "summary". Returns "" if no report label found.
func reportTypeFromLabels(labels []string) string {
	for _, l := range labels {
		if strings.HasPrefix(l, "report:") {
			return strings.TrimPrefix(l, "report:")
		}
	}
	return ""
}

