// Package poolmanager maintains a pool of prewarmed agent pods ready for
// instant assignment. It periodically reconciles the pool size against the
// desired minimum, creating new prewarmed agents when the pool shrinks and
// recycling idle agents that exceed their TTL.
package poolmanager

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gasboat/controller/internal/beadsapi"
	"gasboat/controller/internal/config"
)

// PoolConfig holds the pool configuration.
type PoolConfig struct {
	MinSize  int
	MaxSize  int
	TTL      time.Duration
	Role     string
	Mode     string
	Project  string
	Interval time.Duration
}

// PoolManager maintains a pool of prewarmed agent beads. The existing
// reconciler handles pod creation for these beads; the pool manager only
// manages the bead lifecycle.
type PoolManager struct {
	daemon *beadsapi.Client
	cfg    PoolConfig
	logger *slog.Logger
	mu     sync.Mutex
	seq    int // monotonic counter for unique agent names
}

// New creates a PoolManager from controller config.
func New(daemon *beadsapi.Client, cfg *config.Config, logger *slog.Logger) *PoolManager {
	return &PoolManager{
		daemon: daemon,
		cfg: PoolConfig{
			MinSize:  cfg.PrewarmedPoolMinSize,
			MaxSize:  cfg.PrewarmedPoolMaxSize,
			TTL:      cfg.PrewarmedPoolTTL,
			Role:     cfg.PrewarmedPoolRole,
			Mode:     cfg.PrewarmedPoolMode,
			Project:  cfg.PrewarmedPoolProject,
			Interval: cfg.PrewarmedPoolInterval,
		},
		logger: logger,
	}
}

// prewarmedAgent represents a prewarmed agent bead from the daemon.
type prewarmedAgent struct {
	ID        string
	AgentName string
	CreatedAt time.Time
}

// Reconcile performs a single pool reconciliation pass:
// 1. List all active agent beads with agent_state=prewarmed
// 2. Recycle agents that exceed TTL
// 3. Create new prewarmed agents to reach min_pool_size
func (pm *PoolManager) Reconcile(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	agents, err := pm.listPrewarmedAgents(ctx)
	if err != nil {
		return fmt.Errorf("listing prewarmed agents: %w", err)
	}

	now := time.Now()
	var active []prewarmedAgent

	// Recycle agents exceeding TTL.
	for _, a := range agents {
		age := now.Sub(a.CreatedAt)
		if pm.cfg.TTL > 0 && age > pm.cfg.TTL {
			pm.logger.Info("recycling prewarmed agent (TTL exceeded)",
				"agent", a.ID, "age", age.Round(time.Second))
			if err := pm.daemon.CloseBead(ctx, a.ID, map[string]string{
				"agent_state": "done",
			}); err != nil {
				pm.logger.Warn("failed to close expired prewarmed agent",
					"agent", a.ID, "error", err)
			}
			continue
		}
		active = append(active, a)
	}

	// Create new agents to reach min size.
	deficit := pm.cfg.MinSize - len(active)
	if deficit <= 0 {
		return nil
	}

	// Cap creation to not exceed max size.
	if len(active)+deficit > pm.cfg.MaxSize {
		deficit = pm.cfg.MaxSize - len(active)
	}
	if deficit <= 0 {
		return nil
	}

	pm.logger.Info("pool below minimum, creating prewarmed agents",
		"current", len(active), "min", pm.cfg.MinSize, "creating", deficit)

	for i := 0; i < deficit; i++ {
		if err := pm.createPrewarmedAgent(ctx); err != nil {
			pm.logger.Warn("failed to create prewarmed agent", "error", err)
			return err
		}
	}

	return nil
}

// listPrewarmedAgents returns all active agent beads with agent_state=prewarmed.
func (pm *PoolManager) listPrewarmedAgents(ctx context.Context) ([]prewarmedAgent, error) {
	beads, err := pm.daemon.ListAgentBeads(ctx)
	if err != nil {
		return nil, err
	}

	var result []prewarmedAgent
	for _, b := range beads {
		if b.AgentState != "prewarmed" {
			continue
		}
		// Filter by project if configured.
		if pm.cfg.Project != "" && b.Project != pm.cfg.Project {
			continue
		}
		result = append(result, prewarmedAgent{
			ID:        b.ID,
			AgentName: b.AgentName,
			// Use metadata created_at if available, otherwise use zero time
			// (will not be recycled by TTL until next sync populates it).
			CreatedAt: parseTime(b.Metadata["created_at"]),
		})
	}

	return result, nil
}

// createPrewarmedAgent creates a new agent bead in prewarmed state.
// The existing reconciler will create the corresponding pod.
func (pm *PoolManager) createPrewarmedAgent(ctx context.Context) error {
	pm.seq++
	agentName := fmt.Sprintf("prewarmed-%d-%d", time.Now().Unix(), pm.seq)

	fields := map[string]string{
		"agent":       agentName,
		"mode":        pm.cfg.Mode,
		"role":        pm.cfg.Role,
		"project":     pm.cfg.Project,
		"agent_state": "prewarmed",
	}
	fieldsJSON, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("marshalling agent fields: %w", err)
	}

	labels := []string{"prewarmed"}
	if pm.cfg.Project != "" {
		labels = append(labels, "project:"+pm.cfg.Project)
	}

	beadID, err := pm.daemon.CreateBead(ctx, beadsapi.CreateBeadRequest{
		Title:       agentName,
		Type:        "agent",
		Description: "Prewarmed agent ready for assignment",
		Fields:      json.RawMessage(fieldsJSON),
		Labels:      labels,
	})
	if err != nil {
		return fmt.Errorf("creating prewarmed agent bead: %w", err)
	}

	// Add role label for advice matching.
	_ = pm.daemon.AddLabel(ctx, beadID, "role:"+pm.cfg.Role)

	pm.logger.Info("created prewarmed agent",
		"bead", beadID, "agent", agentName,
		"project", pm.cfg.Project, "role", pm.cfg.Role)
	return nil
}

// AssignRequest holds the parameters for assigning a prewarmed agent.
type AssignRequest struct {
	// Thread context for the assigned work.
	Channel     string `json:"channel"`
	ThreadTS    string `json:"thread_ts"`
	Description string `json:"description"`
	Project     string `json:"project"`
}

// AssignResult is returned when a prewarmed agent is successfully assigned.
type AssignResult struct {
	BeadID    string `json:"bead_id"`
	AgentName string `json:"agent_name"`
}

// ErrPoolEmpty is returned when no prewarmed agents are available for assignment.
var ErrPoolEmpty = fmt.Errorf("no prewarmed agents available")

// AssignPrewarmed atomically picks a prewarmed agent from the pool and
// transitions it to "assigning" state with the given thread context.
// Returns ErrPoolEmpty if no prewarmed agents are available.
func (pm *PoolManager) AssignPrewarmed(ctx context.Context, req AssignRequest) (*AssignResult, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	agents, err := pm.listPrewarmedAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing prewarmed agents: %w", err)
	}
	if len(agents) == 0 {
		return nil, ErrPoolEmpty
	}

	// Pick the oldest prewarmed agent (FIFO).
	pick := agents[0]
	for _, a := range agents[1:] {
		if !a.CreatedAt.IsZero() && (pick.CreatedAt.IsZero() || a.CreatedAt.Before(pick.CreatedAt)) {
			pick = a
		}
	}

	// Update the bead: set agent_state to "assigning" and add thread context.
	fields := map[string]string{
		"agent_state":  "assigning",
		"spawn_source": "slack-thread-pool",
	}
	if req.Channel != "" {
		fields["slack_thread_channel"] = req.Channel
	}
	if req.ThreadTS != "" {
		fields["slack_thread_ts"] = req.ThreadTS
	}
	if req.Project != "" {
		fields["project"] = req.Project
	}
	if err := pm.daemon.UpdateBeadFields(ctx, pick.ID, fields); err != nil {
		return nil, fmt.Errorf("updating prewarmed agent %s: %w", pick.ID, err)
	}

	// Update the bead description with thread context.
	if req.Description != "" {
		desc := req.Description
		if err := pm.daemon.UpdateBead(ctx, pick.ID, beadsapi.UpdateBeadRequest{
			Description: &desc,
		}); err != nil {
			pm.logger.Warn("failed to set description on assigned agent",
				"agent", pick.ID, "error", err)
		}
	}

	pm.logger.Info("assigned prewarmed agent",
		"bead", pick.ID, "agent", pick.AgentName,
		"channel", req.Channel, "thread_ts", req.ThreadTS)

	return &AssignResult{
		BeadID:    pick.ID,
		AgentName: pick.AgentName,
	}, nil
}

// RunLoop runs the pool reconciler periodically until the context is cancelled.
func (pm *PoolManager) RunLoop(ctx context.Context) {
	pm.logger.Info("pool manager started",
		"min_size", pm.cfg.MinSize, "max_size", pm.cfg.MaxSize,
		"ttl", pm.cfg.TTL, "interval", pm.cfg.Interval)

	ticker := time.NewTicker(pm.cfg.Interval)
	defer ticker.Stop()

	// Run immediately on start.
	if err := pm.Reconcile(ctx); err != nil {
		pm.logger.Warn("initial pool reconcile failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			pm.logger.Info("pool manager stopped")
			return
		case <-ticker.C:
			if err := pm.Reconcile(ctx); err != nil {
				pm.logger.Warn("pool reconcile failed", "error", err)
			}
		}
	}
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
