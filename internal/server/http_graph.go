package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/groblegark/kbeads/internal/model"
)

// handleGraph handles both GET /v1/graph and POST /v1/graph.
// Returns nodes and edges for 3D force-directed graph visualization.
func (s *BeadsServer) handleGraph(w http.ResponseWriter, r *http.Request) {
	var args model.GraphArgs

	switch r.Method {
	case http.MethodPost:
		if r.Body != nil {
			if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
				writeError(w, http.StatusBadRequest, "invalid graph args: "+err.Error())
				return
			}
		}
	case http.MethodGet:
		args = graphArgsFromQuery(r)
	}

	ctx := r.Context()

	// Defaults
	if args.Limit == 0 {
		args.Limit = 500
	}
	if len(args.Status) == 0 {
		args.Status = []string{"open", "in_progress", "blocked", "deferred"}
	}
	if len(args.ExcludeTypes) == 0 {
		args.ExcludeTypes = []string{"message", "config", "advice", "role", "event"}
	}

	// Build base filter
	filter := model.BeadFilter{
		Limit: args.Limit,
	}
	if args.Assignee != "" {
		filter.Assignee = args.Assignee
	}
	if args.Priority != nil {
		filter.Priority = args.Priority
	}
	if args.PriorityMin != nil {
		filter.PriorityMin = args.PriorityMin
	}
	if args.PriorityMax != nil {
		filter.PriorityMax = args.PriorityMax
	}
	if len(args.Labels) > 0 {
		filter.Labels = args.Labels
	}
	if len(args.LabelsAny) > 0 {
		filter.LabelsAny = args.LabelsAny
	}
	if args.ParentID != "" {
		filter.ParentID = args.ParentID
	}

	// Apply type exclusions
	for _, t := range args.ExcludeTypes {
		filter.ExcludeTypes = append(filter.ExcludeTypes, model.BeadType(t))
	}

	// Multi-status: query each status separately and merge.
	beadMap := make(map[string]*model.Bead)
	query := args.Query

	for _, statusStr := range args.Status {
		f := filter
		f.Status = []model.Status{model.Status(statusStr)}

		if statusStr == "closed" && args.MaxAgeDays > 0 {
			cutoff := time.Now().AddDate(0, 0, -args.MaxAgeDays)
			f.UpdatedAfter = &cutoff
		}

		if len(args.Types) > 0 {
			for _, t := range args.Types {
				tf := f
				tf.Type = []model.BeadType{model.BeadType(t)}
				if query != "" {
					tf.Search = query
				}
				beads, _, err := s.store.ListBeads(ctx, tf)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "failed to list beads")
					return
				}
				for _, b := range beads {
					beadMap[b.ID] = b
				}
			}
			continue
		}

		if query != "" {
			f.Search = query
		}
		beads, _, err := s.store.ListBeads(ctx, f)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list beads")
			return
		}
		for _, b := range beads {
			beadMap[b.ID] = b
		}
	}

	// Enforce limit via relevance-based sorting.
	if len(beadMap) > args.Limit {
		sorted := make([]*model.Bead, 0, len(beadMap))
		for _, b := range beadMap {
			sorted = append(sorted, b)
		}
		sort.Slice(sorted, func(i, j int) bool {
			ri, rj := beadRelevance(sorted[i]), beadRelevance(sorted[j])
			if ri != rj {
				return ri < rj
			}
			return sorted[i].Priority < sorted[j].Priority
		})
		beadMap = make(map[string]*model.Bead, args.Limit)
		for i := 0; i < args.Limit && i < len(sorted); i++ {
			beadMap[sorted[i].ID] = sorted[i]
		}
	}

	// Collect IDs for batch queries.
	beadIDs := make([]string, 0, len(beadMap))
	for id := range beadMap {
		beadIDs = append(beadIDs, id)
	}

	// Concurrent batch queries.
	var (
		depRecords    map[string][]*model.Dependency
		revDepRecords map[string][]*model.Dependency
		depCounts     map[string]*model.DependencyCounts
		labelsMap     map[string][]string
		blockedMap    map[string][]string
		graphStats    *model.GraphStats

		depErr, revDepErr, countErr, labelErr, blockedErr, statsErr error
		wg                                                          sync.WaitGroup
	)

	wg.Add(6)

	go func() {
		defer wg.Done()
		depRecords, depErr = s.store.GetDependenciesForBeads(ctx, beadIDs)
	}()

	go func() {
		defer wg.Done()
		revDepRecords, revDepErr = s.store.GetReverseDependenciesForBeads(ctx, beadIDs)
	}()

	go func() {
		defer wg.Done()
		depCounts, countErr = s.store.GetDependencyCounts(ctx, beadIDs)
	}()

	go func() {
		defer wg.Done()
		labelsMap, labelErr = s.store.GetLabelsForBeads(ctx, beadIDs)
		if labelErr != nil {
			labelsMap = make(map[string][]string)
		}
	}()

	go func() {
		defer wg.Done()
		blockedMap, blockedErr = s.store.GetBlockedByForBeads(ctx, beadIDs)
		if blockedErr != nil {
			blockedMap = make(map[string][]string)
		}
	}()

	go func() {
		defer wg.Done()
		graphStats, statsErr = s.store.GetStats(ctx)
	}()

	wg.Wait()

	if depErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to get dependencies")
		return
	}
	if countErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to get dependency counts")
		return
	}

	// Build edges from forward deps.
	edges := make([]*model.GraphEdge, 0)
	seenEdges := make(map[string]bool)
	missingDepIDs := make(map[string]bool)

	for beadID, deps := range depRecords {
		for _, dep := range deps {
			edgeKey := fmt.Sprintf("%s->%s:%s", beadID, dep.DependsOnID, dep.Type)
			if seenEdges[edgeKey] {
				continue
			}
			seenEdges[edgeKey] = true
			edges = append(edges, &model.GraphEdge{
				Source: beadID,
				Target: dep.DependsOnID,
				Type:   string(dep.Type),
			})
			if args.IncludeDeps {
				if _, exists := beadMap[dep.DependsOnID]; !exists {
					missingDepIDs[dep.DependsOnID] = true
				}
			}
		}
	}

	// Build edges from reverse deps.
	if revDepErr == nil && revDepRecords != nil {
		for _, deps := range revDepRecords {
			for _, dep := range deps {
				edgeKey := fmt.Sprintf("%s->%s:%s", dep.BeadID, dep.DependsOnID, dep.Type)
				if seenEdges[edgeKey] {
					continue
				}
				seenEdges[edgeKey] = true
				edges = append(edges, &model.GraphEdge{
					Source: dep.BeadID,
					Target: dep.DependsOnID,
					Type:   string(dep.Type),
				})
				if args.IncludeDeps {
					if _, exists := beadMap[dep.BeadID]; !exists {
						missingDepIDs[dep.BeadID] = true
					}
				}
			}
		}
	}

	// Batch-fetch missing dep targets.
	excludeTypeSet := make(map[model.BeadType]bool, len(args.ExcludeTypes))
	for _, t := range args.ExcludeTypes {
		excludeTypeSet[model.BeadType(t)] = true
	}
	if len(missingDepIDs) > 0 {
		missingIDs := make([]string, 0, len(missingDepIDs))
		for id := range missingDepIDs {
			missingIDs = append(missingIDs, id)
		}
		depBeads, err := s.store.GetBeadsByIDs(ctx, missingIDs)
		if err == nil {
			for _, b := range depBeads {
				if excludeTypeSet[b.Type] {
					continue
				}
				beadMap[b.ID] = b
			}
		}
	}

	// Include connected closed items.
	includeConnectedClosed := args.IncludeConnectedClosed != nil && *args.IncludeConnectedClosed
	if includeConnectedClosed {
		closedCandidateIDs := make(map[string]bool)
		for _, e := range edges {
			if _, exists := beadMap[e.Source]; !exists {
				closedCandidateIDs[e.Source] = true
			}
			if _, exists := beadMap[e.Target]; !exists {
				closedCandidateIDs[e.Target] = true
			}
		}
		for _, deps := range depRecords {
			for _, dep := range deps {
				if _, exists := beadMap[dep.DependsOnID]; !exists {
					closedCandidateIDs[dep.DependsOnID] = true
				}
			}
		}
		if len(closedCandidateIDs) > 0 {
			candidateIDs := make([]string, 0, len(closedCandidateIDs))
			for id := range closedCandidateIDs {
				candidateIDs = append(candidateIDs, id)
			}
			candidates, err := s.store.GetBeadsByIDs(ctx, candidateIDs)
			if err == nil {
				for _, b := range candidates {
					if excludeTypeSet[b.Type] {
						continue
					}
					if b.Status == model.StatusClosed {
						if args.MaxAgeDays > 0 {
							cutoff := time.Now().AddDate(0, 0, -args.MaxAgeDays)
							if b.UpdatedAt.Before(cutoff) {
								continue
							}
						}
						beadMap[b.ID] = b
					}
				}
			}
		}
	}

	// Build parent-child lookup from dep records.
	parentOf := make(map[string]string)
	for beadID, deps := range depRecords {
		for _, dep := range deps {
			if dep.Type == model.DepParentChild {
				parentOf[beadID] = dep.DependsOnID
			}
		}
	}
	for _, deps := range revDepRecords {
		for _, dep := range deps {
			if dep.Type == model.DepParentChild {
				parentOf[dep.BeadID] = dep.DependsOnID
			}
		}
	}

	// Infer parent-child from "blocks" deps where target is an epic.
	for beadID, deps := range depRecords {
		if _, hasParent := parentOf[beadID]; hasParent {
			continue
		}
		for _, dep := range deps {
			if dep.Type == model.DepBlocks {
				if target, ok := beadMap[dep.DependsOnID]; ok && target.Type == model.TypeEpic {
					parentOf[beadID] = dep.DependsOnID
					break
				}
			}
		}
	}

	// Promote "blocks" edges to "parent-child" where parentOf says so.
	for i, e := range edges {
		if e.Type == string(model.DepBlocks) {
			if parentOf[e.Source] == e.Target {
				edges[i].Type = string(model.DepParentChild)
			}
		}
	}

	// Build nodes.
	nodes := make([]model.GraphNode, 0, len(beadMap))
	for _, bead := range beadMap {
		node := model.GraphNode{
			ID:        bead.ID,
			Title:     bead.Title,
			Status:    string(bead.Status),
			Priority:  bead.Priority,
			IssueType: string(bead.Type),
			ParentID:  parentOf[bead.ID],
			Assignee:  bead.Assignee,
			Labels:    labelsMap[bead.ID],
			CreatedAt: bead.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt: bead.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			BlockedBy: blockedMap[bead.ID],
		}

		if counts, ok := depCounts[bead.ID]; ok {
			node.DepCount = counts.DependencyCount
			node.DepByCount = counts.DependentCount
		}

		if args.IncludeBody {
			node.Description = bead.Description
			node.Notes = bead.Notes
		}

		nodes = append(nodes, node)
	}

	// Add agent nodes with assignment edges.
	if args.IncludeAgents {
		agentNodes := make(map[string]*model.GraphNode)

		// Source 1: Live roster from presence tracker.
		if s.Presence != nil {
			for _, e := range s.Presence.Roster(0) {
				if e.Reaped {
					continue
				}
				status := "idle"
				if e.IdleSecs < 30 {
					status = "active"
				}
				agentNodes[e.Actor] = &model.GraphNode{
					ID:        "agent:" + e.Actor,
					Title:     e.Actor,
					IssueType: "agent",
					Status:    status,
				}
			}
		}

		// Source 2: Issue assignees — create fallback agent nodes.
		for _, bead := range beadMap {
			if bead.Assignee == "" || bead.Status != model.StatusInProgress {
				continue
			}
			if _, exists := agentNodes[bead.Assignee]; !exists {
				agentNodes[bead.Assignee] = &model.GraphNode{
					ID:        "agent:" + bead.Assignee,
					Title:     bead.Assignee,
					IssueType: "agent",
				}
			}
		}

		// Create assigned_to edges only for in_progress issues.
		for _, bead := range beadMap {
			if bead.Assignee == "" || bead.Status != model.StatusInProgress {
				continue
			}
			if _, exists := agentNodes[bead.Assignee]; exists {
				edges = append(edges, &model.GraphEdge{
					Source: "agent:" + bead.Assignee,
					Target: bead.ID,
					Type:   "assigned_to",
				})
			}
		}

		// Only add agent nodes that have at least one edge.
		agentsWithEdges := make(map[string]bool)
		for _, e := range edges {
			if e.Type == "assigned_to" || e.Type == "requested_by" {
				agentsWithEdges[e.Source] = true
			}
		}
		for _, node := range agentNodes {
			if agentsWithEdges[node.ID] {
				nodes = append(nodes, *node)
			}
		}
	}

	// Filter disconnected nodes.
	connectedIDs := make(map[string]bool)
	for _, e := range edges {
		connectedIDs[e.Source] = true
		connectedIDs[e.Target] = true
	}

	openActiveSet := make(map[string]bool)
	neighbors := make(map[string][]string)
	for _, n := range nodes {
		if n.Status == "open" || n.Status == "in_progress" || n.Status == "blocked" || n.Status == "deferred" ||
			n.Status == "active" || n.Status == "idle" {
			openActiveSet[n.ID] = true
		}
	}
	for _, e := range edges {
		neighbors[e.Source] = append(neighbors[e.Source], e.Target)
		neighbors[e.Target] = append(neighbors[e.Target], e.Source)
	}

	closedConnected := make(map[string]bool)
	for _, n := range nodes {
		if n.Status != "closed" {
			continue
		}
		for _, neighborID := range neighbors[n.ID] {
			if openActiveSet[neighborID] {
				closedConnected[n.ID] = true
				break
			}
		}
	}

	filteredNodes := make([]model.GraphNode, 0, len(nodes))
	for _, n := range nodes {
		switch n.IssueType {
		case "gate", "decision":
			if !connectedIDs[n.ID] {
				continue
			}
		case "agent":
			if !connectedIDs[n.ID] && n.Status != "active" && n.Status != "idle" {
				continue
			}
		}
		if n.Status == "closed" && !closedConnected[n.ID] {
			continue
		}
		filteredNodes = append(filteredNodes, n)
	}
	nodes = filteredNodes

	// Clean up edges pointing to pruned nodes.
	nodeIDSet := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		nodeIDSet[n.ID] = true
	}
	filteredEdges := make([]*model.GraphEdge, 0, len(edges))
	for _, e := range edges {
		if nodeIDSet[e.Source] && nodeIDSet[e.Target] {
			filteredEdges = append(filteredEdges, e)
		}
	}
	edges = filteredEdges

	resp := &model.GraphResponse{
		Nodes: nodes,
		Edges: edges,
	}

	if statsErr == nil && graphStats != nil {
		resp.Stats = graphStats
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleGetStats handles GET /v1/stats.
func (s *BeadsServer) handleGetStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.GetStats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

// handleGetReady handles GET /v1/ready.
// Returns beads that are open and have no unsatisfied blocking dependencies.
func (s *BeadsServer) handleGetReady(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	beads, _, err := s.store.ListBeads(r.Context(), model.BeadFilter{
		Status: []model.Status{model.StatusOpen},
		Sort:   "priority",
		Limit:  limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list beads")
		return
	}

	// Filter out beads that have unsatisfied blocking dependencies.
	var ready []*model.Bead
	for _, b := range beads {
		deps, err := s.store.GetDependencies(r.Context(), b.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to get dependencies")
			return
		}
		blocked := false
		for _, d := range deps {
			if d.Type == model.DepBlocks {
				// Check if the blocking bead is still open.
				blocker, err := s.store.GetBead(r.Context(), d.DependsOnID)
				if err != nil {
					continue
				}
				if blocker != nil && blocker.Status != model.StatusClosed {
					blocked = true
					break
				}
			}
		}
		if !blocked {
			ready = append(ready, b)
		}
	}

	if ready == nil {
		ready = []*model.Bead{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"beads": ready,
		"total": len(ready),
	})
}

// handleGetBlocked handles GET /v1/blocked.
// Returns beads with status=blocked, enriched with blocked_by dependency info.
func (s *BeadsServer) handleGetBlocked(w http.ResponseWriter, r *http.Request) {
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	beads, _, err := s.store.ListBeads(r.Context(), model.BeadFilter{
		Status: []model.Status{model.StatusBlocked},
		Sort:   "priority",
		Limit:  limit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list beads")
		return
	}

	// Enrich each bead with its dependencies.
	for _, b := range beads {
		deps, err := s.store.GetDependencies(r.Context(), b.ID)
		if err != nil {
			continue
		}
		b.Dependencies = deps
	}

	if beads == nil {
		beads = []*model.Bead{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"beads": beads,
		"total": len(beads),
	})
}

// beadRelevance returns a sort key for relevance-based ordering.
func beadRelevance(b *model.Bead) int {
	switch b.Status {
	case model.StatusInProgress:
		return 0
	case model.StatusBlocked:
		return 1
	case model.StatusOpen:
		return 2
	case model.StatusDeferred:
		return 3
	case model.StatusClosed:
		if !b.UpdatedAt.IsZero() && time.Since(b.UpdatedAt) < 7*24*time.Hour {
			return 4
		}
		return 5
	default:
		return 6
	}
}

// graphArgsFromQuery parses GraphArgs from URL query parameters (GET requests).
func graphArgsFromQuery(r *http.Request) model.GraphArgs {
	q := r.URL.Query()
	var args model.GraphArgs

	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			args.Limit = n
		}
	}
	if v := q.Get("assignee"); v != "" {
		args.Assignee = v
	}
	if v := q.Get("query"); v != "" {
		args.Query = v
	}
	if v := q.Get("parent_id"); v != "" {
		args.ParentID = v
	}
	if v := q.Get("max_age_days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			args.MaxAgeDays = n
		}
	}
	if v := q.Get("include_deps"); v == "true" || v == "1" {
		args.IncludeDeps = true
	}
	if v := q.Get("include_body"); v == "true" || v == "1" {
		args.IncludeBody = true
	}
	if v := q.Get("include_agents"); v == "true" || v == "1" {
		args.IncludeAgents = true
	}
	if v := q.Get("include_connected_closed"); v == "true" || v == "1" {
		t := true
		args.IncludeConnectedClosed = &t
	}
	if vs := q["status"]; len(vs) > 0 {
		args.Status = vs
	}
	if vs := q["types"]; len(vs) > 0 {
		args.Types = vs
	}
	if vs := q["exclude_types"]; len(vs) > 0 {
		args.ExcludeTypes = vs
	}
	if vs := q["labels"]; len(vs) > 0 {
		args.Labels = vs
	}
	if vs := q["labels_any"]; len(vs) > 0 {
		args.LabelsAny = vs
	}

	return args
}
