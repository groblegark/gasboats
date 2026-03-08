// Fixed mock data for deterministic screenshot tests.
// Provides a small but representative graph with different node types,
// statuses, priorities, and dependency types.

// Fixed positions (fx/fy/fz) make force layout deterministic for screenshot tests.
// Layout: Epic1 cluster on left, Epic2 cluster on right, isolates in between.
export const MOCK_GRAPH = {
  nodes: [
    // --- Epic 1 cluster (left) ---
    { id: 'bd-epic1', title: 'Epic: Platform Overhaul', status: 'in_progress', priority: 0, issue_type: 'epic', assignee: 'alice', created_at: '2026-01-15T10:00:00Z', updated_at: '2026-02-10T12:00:00Z', labels: ['platform'], dep_count: 3, dep_by_count: 0, blocked_by: [], fx: -80, fy: 0, fz: 0 },
    { id: 'bd-feat1', title: 'Add user authentication', status: 'in_progress', priority: 1, issue_type: 'feature', assignee: 'alice', created_at: '2026-01-20T09:00:00Z', updated_at: '2026-02-18T15:00:00Z', labels: ['auth'], dep_count: 1, dep_by_count: 1, blocked_by: [], fx: -120, fy: -50, fz: 0 },
    { id: 'bd-task1', title: 'Write OAuth integration tests', status: 'open', priority: 2, issue_type: 'task', assignee: 'bob', created_at: '2026-02-01T08:00:00Z', updated_at: '2026-02-17T10:00:00Z', labels: ['testing'], dep_count: 0, dep_by_count: 1, blocked_by: ['bd-feat1'], fx: -160, fy: -90, fz: 0 },
    { id: 'bd-bug1', title: 'Token refresh race condition', status: 'open', priority: 1, issue_type: 'bug', assignee: 'charlie', created_at: '2026-02-05T14:00:00Z', updated_at: '2026-02-19T09:00:00Z', labels: ['auth', 'critical'], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: -70, fy: -80, fz: 0 },
    { id: 'bd-task2', title: 'Database migration script', status: 'open', priority: 2, issue_type: 'task', assignee: 'bob', created_at: '2026-02-10T11:00:00Z', updated_at: '2026-02-19T08:00:00Z', labels: [], dep_count: 0, dep_by_count: 1, blocked_by: [], fx: -40, fy: -50, fz: 0 },
    { id: 'bd-feat2', title: 'API rate limiting', status: 'open', priority: 2, issue_type: 'feature', assignee: '', created_at: '2026-02-12T16:00:00Z', updated_at: '2026-02-18T14:00:00Z', labels: ['api'], dep_count: 1, dep_by_count: 0, blocked_by: ['bd-task2'], fx: -40, fy: 50, fz: 0 },
    // --- Epic 2 cluster (right) ---
    { id: 'bd-epic2', title: 'Epic: Observability Stack', status: 'open', priority: 1, issue_type: 'epic', assignee: 'charlie', created_at: '2026-01-10T08:00:00Z', updated_at: '2026-02-15T11:00:00Z', labels: ['infra'], dep_count: 2, dep_by_count: 0, blocked_by: [], fx: 80, fy: 0, fz: 0 },
    { id: 'bd-task3', title: 'Add structured logging', status: 'in_progress', priority: 2, issue_type: 'task', assignee: 'charlie', created_at: '2026-02-08T10:00:00Z', updated_at: '2026-02-19T07:00:00Z', labels: ['logging'], dep_count: 0, dep_by_count: 1, blocked_by: [], fx: 50, fy: -50, fz: 0 },
    { id: 'bd-task4', title: 'Metrics dashboard Helm chart', status: 'open', priority: 3, issue_type: 'task', assignee: '', created_at: '2026-02-14T09:00:00Z', updated_at: '2026-02-18T16:00:00Z', labels: ['helm', 'infra'], dep_count: 0, dep_by_count: 1, blocked_by: ['bd-task3'], fx: 120, fy: -50, fz: 0 },
    // --- Isolated nodes (center/bottom) ---
    { id: 'bd-bug2', title: 'Memory leak in event bus', status: 'open', priority: 0, issue_type: 'bug', assignee: 'alice', created_at: '2026-02-18T20:00:00Z', updated_at: '2026-02-19T10:00:00Z', labels: ['critical'], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 0, fy: 80, fz: 0 },
    { id: 'bd-task5', title: 'Refactor config loader', status: 'open', priority: 3, issue_type: 'task', assignee: 'bob', created_at: '2026-02-16T12:00:00Z', updated_at: '2026-02-19T06:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 50, fy: 80, fz: 0 },
    { id: 'bd-feat3', title: 'WebSocket event streaming', status: 'open', priority: 2, issue_type: 'feature', assignee: 'charlie', created_at: '2026-02-13T15:00:00Z', updated_at: '2026-02-19T11:00:00Z', labels: ['api', 'realtime'], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 130, fy: 60, fz: 0 },
    // Closed old nodes — for age filter testing (bd-uc0mw)
    { id: 'bd-old1', title: 'Legacy config migration', status: 'closed', priority: 3, issue_type: 'task', assignee: 'bob', created_at: '2025-11-01T10:00:00Z', updated_at: '2025-12-15T14:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 0, fy: -100, fz: 0 },
    { id: 'bd-old2', title: 'Deprecated API cleanup', status: 'closed', priority: 4, issue_type: 'task', assignee: 'charlie', created_at: '2025-10-01T08:00:00Z', updated_at: '2025-11-20T11:00:00Z', labels: [], dep_count: 0, dep_by_count: 1, blocked_by: [], fx: 50, fy: -100, fz: 0 },
    // Agent nodes (synthetic — added by Graph API when include_agents=true)
    { id: 'agent:alice', title: 'alice', status: 'open', priority: 2, issue_type: 'agent', assignee: '', created_at: '2026-02-19T00:00:00Z', updated_at: '2026-02-19T12:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: -150, fy: 30, fz: 0 },
    { id: 'agent:bob', title: 'bob', status: 'open', priority: 2, issue_type: 'agent', assignee: '', created_at: '2026-02-19T00:00:00Z', updated_at: '2026-02-19T12:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: -150, fy: -30, fz: 0 },
  ],
  edges: [
    { source: 'bd-epic1', target: 'bd-feat1', type: 'parent-child' },
    { source: 'bd-epic1', target: 'bd-feat2', type: 'parent-child' },
    { source: 'bd-feat1', target: 'bd-task1', type: 'blocks' },
    { source: 'bd-task2', target: 'bd-feat2', type: 'blocks' },
    { source: 'bd-epic2', target: 'bd-task3', type: 'parent-child' },
    { source: 'bd-epic2', target: 'bd-task4', type: 'parent-child' },
    { source: 'bd-task3', target: 'bd-task4', type: 'blocks' },
    { source: 'bd-feat1', target: 'bd-bug1', type: 'relates-to' },
    { source: 'bd-epic1', target: 'bd-task2', type: 'parent-child' },
    { source: 'bd-feat3', target: 'bd-epic2', type: 'waits-for' },
    // Agent assignment edges
    { source: 'agent:alice', target: 'bd-feat1', type: 'assigned_to' },
    { source: 'agent:alice', target: 'bd-bug2', type: 'assigned_to' },
    { source: 'agent:bob', target: 'bd-task1', type: 'assigned_to' },
    { source: 'agent:bob', target: 'bd-task2', type: 'assigned_to' },
    // Old closed node connected to active task (for age filter rescue test)
    { source: 'bd-old2', target: 'bd-task3', type: 'blocks' },
  ],
  stats: {
    total_open: 8,
    total_in_progress: 3,
    total_blocked: 3,
    total_closed: 2,
  },
};

// Minimal response for Ping
export const MOCK_PING = { version: '0.62.6', uptime: 3600 };

// --- Multi-agent mock graph (bd-mll3i) ---
// 9 active agents working on a realistic platform overhaul project.
// Each agent has 1-3 in_progress beads. Fixed positions cluster agents
// around their beads for clean simultaneous 9-agent view tests.
export const MOCK_MULTI_AGENT_GRAPH = {
  nodes: [
    // --- EPIC: Platform Overhaul (top-left anchor) ---
    { id: 'gt-epic-platform', title: 'Epic: Platform Overhaul', status: 'in_progress', priority: 0, issue_type: 'epic', assignee: '', created_at: '2026-01-10T09:00:00Z', updated_at: '2026-02-18T08:00:00Z', labels: ['platform'], dep_count: 6, dep_by_count: 0, blocked_by: [], fx: -200, fy: 0, fz: 0 },

    // --- Agent: swift-newt (auth cluster) ---
    { id: 'gt-auth-rpc', title: 'Implement auth RPC layer', status: 'in_progress', priority: 1, issue_type: 'feature', assignee: 'swift-newt', created_at: '2026-02-10T10:00:00Z', updated_at: '2026-02-19T11:30:00Z', labels: ['auth', 'rpc'], dep_count: 1, dep_by_count: 0, blocked_by: [], fx: -300, fy: -100, fz: 0 },
    { id: 'gt-auth-tests', title: 'Auth integration tests', status: 'in_progress', priority: 2, issue_type: 'task', assignee: 'swift-newt', created_at: '2026-02-12T09:00:00Z', updated_at: '2026-02-19T11:00:00Z', labels: ['auth', 'testing'], dep_count: 0, dep_by_count: 1, blocked_by: ['gt-auth-rpc'], fx: -350, fy: -150, fz: 0 },

    // --- Agent: keen-bird (storage cluster) ---
    { id: 'gt-dolt-schema', title: 'Dolt schema migration v3', status: 'in_progress', priority: 1, issue_type: 'task', assignee: 'keen-bird', created_at: '2026-02-08T08:00:00Z', updated_at: '2026-02-19T10:45:00Z', labels: ['dolt', 'schema'], dep_count: 0, dep_by_count: 2, blocked_by: [], fx: -300, fy: 100, fz: 0 },
    { id: 'gt-dolt-repl', title: 'Dolt replication health check', status: 'in_progress', priority: 2, issue_type: 'task', assignee: 'keen-bird', created_at: '2026-02-14T11:00:00Z', updated_at: '2026-02-19T09:30:00Z', labels: ['dolt', 'ops'], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: -350, fy: 60, fz: 0 },

    // --- Agent: vast-toad (visualization cluster) ---
    { id: 'gt-graph-api', title: 'Graph API: agent synthesis', status: 'in_progress', priority: 1, issue_type: 'feature', assignee: 'vast-toad', created_at: '2026-02-15T10:00:00Z', updated_at: '2026-02-19T11:50:00Z', labels: ['api', 'graph'], dep_count: 0, dep_by_count: 1, blocked_by: [], fx: -100, fy: -200, fz: 0 },

    // --- Agent: arch-seal (infra cluster) ---
    { id: 'gt-k8s-deploy', title: 'K8s deployment manifests update', status: 'in_progress', priority: 1, issue_type: 'task', assignee: 'arch-seal', created_at: '2026-02-16T09:00:00Z', updated_at: '2026-02-19T10:00:00Z', labels: ['k8s', 'infra'], dep_count: 1, dep_by_count: 0, blocked_by: [], fx: -100, fy: 200, fz: 0 },
    { id: 'gt-helm-chart', title: 'Helm chart: raise memory limits', status: 'in_progress', priority: 1, issue_type: 'task', assignee: 'arch-seal', created_at: '2026-02-17T08:00:00Z', updated_at: '2026-02-19T11:20:00Z', labels: ['helm', 'infra'], dep_count: 0, dep_by_count: 1, blocked_by: [], fx: -50, fy: 250, fz: 0 },

    // --- Agent: lush-mole (test data cluster) ---
    { id: 'gt-e2e-fixtures', title: 'E2E fixture data: multi-agent sessions', status: 'in_progress', priority: 1, issue_type: 'task', assignee: 'lush-mole', created_at: '2026-02-18T09:00:00Z', updated_at: '2026-02-19T11:55:00Z', labels: ['testing', 'e2e'], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 100, fy: -200, fz: 0 },

    // --- Agent: deft-fox (UI/bugs cluster) ---
    { id: 'gt-label-toggle', title: 'Fix label toggle key (l key bug)', status: 'in_progress', priority: 2, issue_type: 'bug', assignee: 'deft-fox', created_at: '2026-02-19T08:00:00Z', updated_at: '2026-02-19T11:40:00Z', labels: ['ui', 'bug'], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 100, fy: 0, fz: 0 },
    { id: 'gt-doot-popup', title: 'Doot-triggered issue popup', status: 'in_progress', priority: 1, issue_type: 'feature', assignee: 'deft-fox', created_at: '2026-02-19T09:00:00Z', updated_at: '2026-02-19T11:45:00Z', labels: ['ui', 'doots'], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 150, fy: -50, fz: 0 },

    // --- Agent: stout-mare (ops cluster) ---
    { id: 'gt-oomkill', title: 'Fix OOMKilled gitlab-runner jobs', status: 'in_progress', priority: 0, issue_type: 'bug', assignee: 'stout-mare', created_at: '2026-02-17T10:00:00Z', updated_at: '2026-02-19T11:00:00Z', labels: ['ops', 'critical'], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 100, fy: 150, fz: 0 },

    // --- Agent: tall-seal (coverage cluster) ---
    { id: 'gt-coverage-e2e', title: 'Collect E2E coverage from Playwright', status: 'in_progress', priority: 2, issue_type: 'task', assignee: 'tall-seal', created_at: '2026-02-18T10:00:00Z', updated_at: '2026-02-19T10:30:00Z', labels: ['testing', 'coverage'], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 200, fy: 100, fz: 0 },
    { id: 'gt-coverage-unit', title: 'Unit test coverage baseline', status: 'in_progress', priority: 2, issue_type: 'task', assignee: 'tall-seal', created_at: '2026-02-18T11:00:00Z', updated_at: '2026-02-19T10:00:00Z', labels: ['testing', 'coverage'], dep_count: 0, dep_by_count: 1, blocked_by: [], fx: 250, fy: 60, fz: 0 },

    // --- Agent: arch-seal-1 (popup cluster) ---
    { id: 'gt-popup-detect', title: 'Detect doot events on issue nodes', status: 'in_progress', priority: 1, issue_type: 'task', assignee: 'arch-seal-1', created_at: '2026-02-19T09:00:00Z', updated_at: '2026-02-19T11:30:00Z', labels: ['ui', 'doots'], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 300, fy: -100, fz: 0 },

    // --- Blocked / open work (background context) ---
    { id: 'gt-nats-bench', title: 'NATS throughput benchmark', status: 'open', priority: 3, issue_type: 'task', assignee: '', created_at: '2026-02-16T14:00:00Z', updated_at: '2026-02-19T08:00:00Z', labels: ['nats', 'perf'], dep_count: 0, dep_by_count: 1, blocked_by: ['gt-dolt-schema'], fx: -250, fy: 150, fz: 0 },
    { id: 'gt-redis-ttl', title: 'Tune Redis cache TTL', status: 'open', priority: 3, issue_type: 'task', assignee: '', created_at: '2026-02-17T09:00:00Z', updated_at: '2026-02-18T14:00:00Z', labels: ['redis', 'perf'], dep_count: 0, dep_by_count: 1, blocked_by: ['gt-graph-api'], fx: -50, fy: -250, fz: 0 },

    // --- Agent nodes (synthetic — one per active agent) ---
    { id: 'agent:swift-newt', title: 'swift-newt', status: 'active', priority: 3, issue_type: 'agent', assignee: '', created_at: '2026-02-19T08:00:00Z', updated_at: '2026-02-19T11:55:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: -390, fy: -130, fz: 0 },
    { id: 'agent:keen-bird', title: 'keen-bird', status: 'active', priority: 3, issue_type: 'agent', assignee: '', created_at: '2026-02-19T08:00:00Z', updated_at: '2026-02-19T11:55:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: -390, fy: 80, fz: 0 },
    { id: 'agent:vast-toad', title: 'vast-toad', status: 'active', priority: 3, issue_type: 'agent', assignee: '', created_at: '2026-02-19T08:00:00Z', updated_at: '2026-02-19T11:55:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: -130, fy: -240, fz: 0 },
    { id: 'agent:arch-seal', title: 'arch-seal', status: 'active', priority: 3, issue_type: 'agent', assignee: '', created_at: '2026-02-19T08:00:00Z', updated_at: '2026-02-19T11:55:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: -130, fy: 220, fz: 0 },
    { id: 'agent:lush-mole', title: 'lush-mole', status: 'active', priority: 3, issue_type: 'agent', assignee: '', created_at: '2026-02-19T08:00:00Z', updated_at: '2026-02-19T11:55:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 80, fy: -240, fz: 0 },
    { id: 'agent:deft-fox', title: 'deft-fox', status: 'active', priority: 3, issue_type: 'agent', assignee: '', created_at: '2026-02-19T08:00:00Z', updated_at: '2026-02-19T11:55:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 80, fy: 20, fz: 0 },
    { id: 'agent:stout-mare', title: 'stout-mare', status: 'active', priority: 3, issue_type: 'agent', assignee: '', created_at: '2026-02-19T08:00:00Z', updated_at: '2026-02-19T11:55:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 80, fy: 180, fz: 0 },
    { id: 'agent:tall-seal', title: 'tall-seal', status: 'active', priority: 3, issue_type: 'agent', assignee: '', created_at: '2026-02-19T08:00:00Z', updated_at: '2026-02-19T11:55:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 230, fy: 80, fz: 0 },
    { id: 'agent:arch-seal-1', title: 'arch-seal-1', status: 'active', priority: 3, issue_type: 'agent', assignee: '', created_at: '2026-02-19T08:00:00Z', updated_at: '2026-02-19T11:55:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 320, fy: -130, fz: 0 },
  ],
  edges: [
    // Epic → features
    { source: 'gt-epic-platform', target: 'gt-auth-rpc', dep_type: 'parent-child' },
    { source: 'gt-epic-platform', target: 'gt-graph-api', dep_type: 'parent-child' },
    { source: 'gt-epic-platform', target: 'gt-k8s-deploy', dep_type: 'parent-child' },
    { source: 'gt-epic-platform', target: 'gt-e2e-fixtures', dep_type: 'parent-child' },
    // Task dependency chains
    { source: 'gt-auth-rpc', target: 'gt-auth-tests', dep_type: 'blocks' },
    { source: 'gt-helm-chart', target: 'gt-k8s-deploy', dep_type: 'blocks' },
    { source: 'gt-dolt-schema', target: 'gt-nats-bench', dep_type: 'blocks' },
    { source: 'gt-graph-api', target: 'gt-redis-ttl', dep_type: 'blocks' },
    { source: 'gt-coverage-unit', target: 'gt-coverage-e2e', dep_type: 'blocks' },
    // Agent → bead assignments (assigned_to edges)
    { source: 'agent:swift-newt', target: 'gt-auth-rpc', dep_type: 'assigned_to' },
    { source: 'agent:swift-newt', target: 'gt-auth-tests', dep_type: 'assigned_to' },
    { source: 'agent:keen-bird', target: 'gt-dolt-schema', dep_type: 'assigned_to' },
    { source: 'agent:keen-bird', target: 'gt-dolt-repl', dep_type: 'assigned_to' },
    { source: 'agent:vast-toad', target: 'gt-graph-api', dep_type: 'assigned_to' },
    { source: 'agent:arch-seal', target: 'gt-k8s-deploy', dep_type: 'assigned_to' },
    { source: 'agent:arch-seal', target: 'gt-helm-chart', dep_type: 'assigned_to' },
    { source: 'agent:lush-mole', target: 'gt-e2e-fixtures', dep_type: 'assigned_to' },
    { source: 'agent:deft-fox', target: 'gt-label-toggle', dep_type: 'assigned_to' },
    { source: 'agent:deft-fox', target: 'gt-doot-popup', dep_type: 'assigned_to' },
    { source: 'agent:stout-mare', target: 'gt-oomkill', dep_type: 'assigned_to' },
    { source: 'agent:tall-seal', target: 'gt-coverage-e2e', dep_type: 'assigned_to' },
    { source: 'agent:tall-seal', target: 'gt-coverage-unit', dep_type: 'assigned_to' },
    { source: 'agent:arch-seal-1', target: 'gt-popup-detect', dep_type: 'assigned_to' },
  ],
  stats: {
    total_open: 4,
    total_in_progress: 13,
    total_blocked: 4,
    total_closed: 0,
  },
};

// --- Synthetic bus event sessions (bd-mll3i) ---
// Realistic tool call sequences for each agent, exercising the full doot pipeline.
// Each session is an array of bus event objects (stream, type, payload).
// Use with sseFrame() to inject into Playwright tests via /bus/events SSE.

const T0 = '2026-02-19T11:00:00.000Z';

// Build a bus event object (matches SSE payload format)
function busEvent(stream, type, payload = {}, offsetMs = 0) {
  const ts = new Date(new Date(T0).getTime() + offsetMs).toISOString();
  return {
    stream,
    type,
    subject: `${stream}.${type}`,
    seq: Math.floor(Math.random() * 1000000),
    ts,
    payload,
  };
}

// swift-newt: Auth RPC work session
// - Session starts, reads proto files, edits server.go, runs tests
export const SESSION_SWIFT_NEWT = [
  busEvent('agents', 'AgentStarted',   { actor: 'swift-newt', issue_id: 'gt-auth-rpc' },               0),
  busEvent('hooks',  'SessionStart',   { actor: 'swift-newt', session_id: 'sess-swift-001' },          200),
  busEvent('hooks',  'PreToolUse',     { actor: 'swift-newt', tool_name: 'Read', tool_input: { file_path: '/beads/internal/rpc/protocol.go' } }, 800),
  busEvent('hooks',  'PostToolUse',    { actor: 'swift-newt', tool_name: 'Read' },                     1200),
  busEvent('hooks',  'PreToolUse',     { actor: 'swift-newt', tool_name: 'Grep', tool_input: { pattern: 'AuthService', path: '/beads/internal/rpc' } }, 1500),
  busEvent('hooks',  'PostToolUse',    { actor: 'swift-newt', tool_name: 'Grep' },                     1900),
  busEvent('hooks',  'PreToolUse',     { actor: 'swift-newt', tool_name: 'Edit', tool_input: { file_path: '/beads/internal/rpc/server_auth.go' } }, 2200),
  busEvent('hooks',  'PostToolUse',    { actor: 'swift-newt', tool_name: 'Edit' },                     3500),
  busEvent('hooks',  'PreToolUse',     { actor: 'swift-newt', tool_name: 'Bash', tool_input: { command: 'go test ./internal/rpc/... -run TestAuth -v 2>&1 | tail -30' } }, 3800),
  busEvent('hooks',  'PostToolUse',    { actor: 'swift-newt', tool_name: 'Bash' },                     8200),
  busEvent('hooks',  'PreToolUse',     { actor: 'swift-newt', tool_name: 'Edit', tool_input: { file_path: '/beads/internal/rpc/server_auth.go' } }, 8500),
  busEvent('hooks',  'PostToolUse',    { actor: 'swift-newt', tool_name: 'Edit' },                     9200),
  busEvent('hooks',  'PreToolUse',     { actor: 'swift-newt', tool_name: 'Bash', tool_input: { command: 'go test ./internal/rpc/... -run TestAuth -count=1 2>&1 | tail -20' } }, 9500),
  busEvent('hooks',  'PostToolUse',    { actor: 'swift-newt', tool_name: 'Bash' },                     12000),
  busEvent('mutations', 'MutationUpdate', { actor: 'swift-newt', issue_id: 'gt-auth-rpc', assignee: 'swift-newt', type: 'update' }, 12500),
  busEvent('agents', 'AgentIdle',      { actor: 'swift-newt', issue_id: 'gt-auth-rpc' },               13000),
];

// keen-bird: Dolt schema migration session
// - Reads existing schema, writes migration SQL, applies it, verifies
export const SESSION_KEEN_BIRD = [
  busEvent('agents', 'AgentStarted',   { actor: 'keen-bird', issue_id: 'gt-dolt-schema' },             0),
  busEvent('hooks',  'SessionStart',   { actor: 'keen-bird', session_id: 'sess-keen-001' },            300),
  busEvent('hooks',  'PreToolUse',     { actor: 'keen-bird', tool_name: 'Bash', tool_input: { command: 'dolt schema show --host gastown-next.app.e2e.dev.fics.ai' } }, 900),
  busEvent('hooks',  'PostToolUse',    { actor: 'keen-bird', tool_name: 'Bash' },                      2000),
  busEvent('hooks',  'PreToolUse',     { actor: 'keen-bird', tool_name: 'Write', tool_input: { file_path: '/gastown/migrations/v3_add_inbox_priority.sql' } }, 2400),
  busEvent('hooks',  'PostToolUse',    { actor: 'keen-bird', tool_name: 'Write' },                     3100),
  busEvent('hooks',  'PreToolUse',     { actor: 'keen-bird', tool_name: 'Bash', tool_input: { command: 'dolt sql -f /gastown/migrations/v3_add_inbox_priority.sql' } }, 3400),
  busEvent('hooks',  'PostToolUse',    { actor: 'keen-bird', tool_name: 'Bash' },                      5800),
  busEvent('hooks',  'PreToolUse',     { actor: 'keen-bird', tool_name: 'Bash', tool_input: { command: 'dolt schema show inbox_items | grep priority' } },          6000),
  busEvent('hooks',  'PostToolUse',    { actor: 'keen-bird', tool_name: 'Bash' },                      6500),
  busEvent('mutations', 'MutationUpdate', { actor: 'keen-bird', issue_id: 'gt-dolt-schema', agent_state: 'working' }, 7000),
  busEvent('agents', 'AgentIdle',      { actor: 'keen-bird', issue_id: 'gt-dolt-schema' },             7500),
];

// vast-toad: Graph API work session
// - Reads server_graph.go, adds agent synthesis logic, writes tests
export const SESSION_VAST_TOAD = [
  busEvent('agents', 'AgentStarted',   { actor: 'vast-toad', issue_id: 'gt-graph-api' },               0),
  busEvent('hooks',  'SessionStart',   { actor: 'vast-toad', session_id: 'sess-vast-001' },            400),
  busEvent('hooks',  'PreToolUse',     { actor: 'vast-toad', tool_name: 'Read', tool_input: { file_path: '/beads/internal/rpc/server_graph.go' } }, 1000),
  busEvent('hooks',  'PostToolUse',    { actor: 'vast-toad', tool_name: 'Read' },                      1800),
  busEvent('hooks',  'PreToolUse',     { actor: 'vast-toad', tool_name: 'Grep', tool_input: { pattern: 'GetIssuesByLabel', path: '/beads/internal/storage' } }, 2100),
  busEvent('hooks',  'PostToolUse',    { actor: 'vast-toad', tool_name: 'Grep' },                      2600),
  busEvent('hooks',  'PreToolUse',     { actor: 'vast-toad', tool_name: 'Edit', tool_input: { file_path: '/beads/internal/rpc/server_graph.go' } }, 3000),
  busEvent('hooks',  'PostToolUse',    { actor: 'vast-toad', tool_name: 'Edit' },                      5200),
  busEvent('hooks',  'PreToolUse',     { actor: 'vast-toad', tool_name: 'Bash', tool_input: { command: 'go test ./internal/rpc/... -run TestGraph -v -count=1 2>&1 | tail -40' } }, 5500),
  busEvent('hooks',  'PostToolUse',    { actor: 'vast-toad', tool_name: 'Bash' },                      9000),
  busEvent('agents', 'AgentIdle',      { actor: 'vast-toad', issue_id: 'gt-graph-api' },               9500),
];

// arch-seal: Helm chart memory limit fix session
// - Reads values.yaml, edits limits, runs helm lint, commits
export const SESSION_ARCH_SEAL = [
  busEvent('agents', 'AgentStarted',   { actor: 'arch-seal', issue_id: 'gt-helm-chart' },              0),
  busEvent('hooks',  'SessionStart',   { actor: 'arch-seal', session_id: 'sess-arch-001' },            200),
  busEvent('hooks',  'PreToolUse',     { actor: 'arch-seal', tool_name: 'Bash', tool_input: { command: 'kubectl --context devops get pods -n gitlab-runner | grep -v Running' } }, 700),
  busEvent('hooks',  'PostToolUse',    { actor: 'arch-seal', tool_name: 'Bash' },                      2100),
  busEvent('hooks',  'PreToolUse',     { actor: 'arch-seal', tool_name: 'Read', tool_input: { file_path: '/fics-helm-chart/charts/gitlab-runner/values.yaml' } }, 2400),
  busEvent('hooks',  'PostToolUse',    { actor: 'arch-seal', tool_name: 'Read' },                      2900),
  busEvent('hooks',  'PreToolUse',     { actor: 'arch-seal', tool_name: 'Edit', tool_input: { file_path: '/fics-helm-chart/charts/gitlab-runner/values.yaml' } }, 3200),
  busEvent('hooks',  'PostToolUse',    { actor: 'arch-seal', tool_name: 'Edit' },                      4000),
  busEvent('hooks',  'PreToolUse',     { actor: 'arch-seal', tool_name: 'Bash', tool_input: { command: 'helm lint charts/gitlab-runner --strict' } },              4300),
  busEvent('hooks',  'PostToolUse',    { actor: 'arch-seal', tool_name: 'Bash' },                      6800),
  busEvent('hooks',  'PreToolUse',     { actor: 'arch-seal', tool_name: 'Bash', tool_input: { command: 'git add charts/gitlab-runner/values.yaml && git commit -m "fix: raise deploy job memory limits to 8Gi"' } }, 7000),
  busEvent('hooks',  'PostToolUse',    { actor: 'arch-seal', tool_name: 'Bash' },                      7800),
  busEvent('mutations', 'MutationUpdate', { actor: 'arch-seal', issue_id: 'gt-helm-chart', new_status: 'closed', type: 'close' }, 8200),
  busEvent('mutations', 'MutationClose',  { actor: 'arch-seal', issue_id: 'gt-helm-chart' },           8400),
  busEvent('agents', 'AgentIdle',      { actor: 'arch-seal', issue_id: 'gt-k8s-deploy' },              8800),
];

// lush-mole: E2E fixture writing session (meta!)
// - Glob test files, read fixtures.js, write expanded fixtures, run playwright
export const SESSION_LUSH_MOLE = [
  busEvent('agents', 'AgentStarted',   { actor: 'lush-mole', issue_id: 'gt-e2e-fixtures' },            0),
  busEvent('hooks',  'SessionStart',   { actor: 'lush-mole', session_id: 'sess-lush-001' },            300),
  busEvent('hooks',  'PreToolUse',     { actor: 'lush-mole', tool_name: 'Glob', tool_input: { pattern: 'tests/**/*.spec.js' } }, 900),
  busEvent('hooks',  'PostToolUse',    { actor: 'lush-mole', tool_name: 'Glob' },                      1300),
  busEvent('hooks',  'PreToolUse',     { actor: 'lush-mole', tool_name: 'Read', tool_input: { file_path: '/beads3d/tests/fixtures.js' } }, 1600),
  busEvent('hooks',  'PostToolUse',    { actor: 'lush-mole', tool_name: 'Read' },                      2400),
  busEvent('hooks',  'PreToolUse',     { actor: 'lush-mole', tool_name: 'Read', tool_input: { file_path: '/beads3d/src/main.js' } }, 2700),
  busEvent('hooks',  'PostToolUse',    { actor: 'lush-mole', tool_name: 'Read' },                      4200),
  busEvent('hooks',  'PreToolUse',     { actor: 'lush-mole', tool_name: 'Edit', tool_input: { file_path: '/beads3d/tests/fixtures.js' } }, 4500),
  busEvent('hooks',  'PostToolUse',    { actor: 'lush-mole', tool_name: 'Edit' },                      7800),
  busEvent('hooks',  'PreToolUse',     { actor: 'lush-mole', tool_name: 'Write', tool_input: { file_path: '/beads3d/tests/sessions.spec.js' } }, 8100),
  busEvent('hooks',  'PostToolUse',    { actor: 'lush-mole', tool_name: 'Write' },                     9500),
  busEvent('hooks',  'PreToolUse',     { actor: 'lush-mole', tool_name: 'Bash', tool_input: { command: 'npx playwright test tests/sessions.spec.js --reporter=list 2>&1 | tail -30' } }, 9800),
  busEvent('hooks',  'PostToolUse',    { actor: 'lush-mole', tool_name: 'Bash' },                      25000),
  busEvent('mutations', 'MutationUpdate', { actor: 'lush-mole', issue_id: 'gt-e2e-fixtures', agent_state: 'working' }, 25500),
  busEvent('agents', 'AgentIdle',      { actor: 'lush-mole', issue_id: 'gt-e2e-fixtures' },            26000),
];

// deft-fox: Label toggle bug fix session
// - Reads main.js label handling, finds bug, fixes it, verifies
export const SESSION_DEFT_FOX = [
  busEvent('agents', 'AgentStarted',   { actor: 'deft-fox', issue_id: 'gt-label-toggle' },             0),
  busEvent('hooks',  'SessionStart',   { actor: 'deft-fox', session_id: 'sess-deft-001' },             200),
  busEvent('hooks',  'PreToolUse',     { actor: 'deft-fox', tool_name: 'Grep', tool_input: { pattern: "key.*'l'|keyCode.*76", path: '/beads3d/src/main.js' } }, 800),
  busEvent('hooks',  'PostToolUse',    { actor: 'deft-fox', tool_name: 'Grep' },                       1400),
  busEvent('hooks',  'PreToolUse',     { actor: 'deft-fox', tool_name: 'Read', tool_input: { file_path: '/beads3d/src/main.js', offset: 3300, limit: 80 } }, 1700),
  busEvent('hooks',  'PostToolUse',    { actor: 'deft-fox', tool_name: 'Read' },                       2200),
  busEvent('hooks',  'PreToolUse',     { actor: 'deft-fox', tool_name: 'Edit', tool_input: { file_path: '/beads3d/src/main.js' } }, 2500),
  busEvent('hooks',  'PostToolUse',    { actor: 'deft-fox', tool_name: 'Edit' },                       3600),
  busEvent('hooks',  'PreToolUse',     { actor: 'deft-fox', tool_name: 'Bash', tool_input: { command: 'npx playwright test tests/interactions.spec.js --grep "label" 2>&1 | tail -20' } }, 3900),
  busEvent('hooks',  'PostToolUse',    { actor: 'deft-fox', tool_name: 'Bash' },                       18000),
  busEvent('mutations', 'MutationClose',  { actor: 'deft-fox', issue_id: 'gt-label-toggle' },          18500),
  busEvent('agents', 'AgentIdle',      { actor: 'deft-fox', issue_id: 'gt-doot-popup' },               19000),
];

// stout-mare: OOMKill investigation session
// - Checks pod events, memory metrics, edits resource limits
export const SESSION_STOUT_MARE = [
  busEvent('agents', 'AgentStarted',   { actor: 'stout-mare', issue_id: 'gt-oomkill' },                0),
  busEvent('hooks',  'SessionStart',   { actor: 'stout-mare', session_id: 'sess-stout-001' },          200),
  busEvent('hooks',  'PreToolUse',     { actor: 'stout-mare', tool_name: 'Bash', tool_input: { command: 'kubectl --context devops get events -n gitlab-runner --field-selector reason=OOMKilling --sort-by=.lastTimestamp | tail -20' } }, 800),
  busEvent('hooks',  'PostToolUse',    { actor: 'stout-mare', tool_name: 'Bash' },                     3500),
  busEvent('hooks',  'PreToolUse',     { actor: 'stout-mare', tool_name: 'Bash', tool_input: { command: 'kubectl --context devops top pod -n gitlab-runner --sort-by=memory | head -15' } }, 3800),
  busEvent('hooks',  'PostToolUse',    { actor: 'stout-mare', tool_name: 'Bash' },                     5200),
  busEvent('hooks',  'PreToolUse',     { actor: 'stout-mare', tool_name: 'Read', tool_input: { file_path: '/fics-helm-chart/charts/gitlab-runner/values.yaml' } }, 5600),
  busEvent('hooks',  'PostToolUse',    { actor: 'stout-mare', tool_name: 'Read' },                     6100),
  busEvent('mutations', 'MutationUpdate', { actor: 'stout-mare', issue_id: 'gt-oomkill', agent_state: 'working' }, 6500),
  busEvent('agents', 'AgentIdle',      { actor: 'stout-mare', issue_id: 'gt-oomkill' },                7000),
];

// tall-seal: Coverage collection session
// - Runs playwright with coverage, collects lcov, merges with unit
export const SESSION_TALL_SEAL = [
  busEvent('agents', 'AgentStarted',   { actor: 'tall-seal', issue_id: 'gt-coverage-e2e' },            0),
  busEvent('hooks',  'SessionStart',   { actor: 'tall-seal', session_id: 'sess-tall-001' },            400),
  busEvent('hooks',  'PreToolUse',     { actor: 'tall-seal', tool_name: 'Bash', tool_input: { command: 'npx playwright test --reporter=line --coverage 2>&1 | tail -40' } }, 1100),
  busEvent('hooks',  'PostToolUse',    { actor: 'tall-seal', tool_name: 'Bash' },                      28000),
  busEvent('hooks',  'PreToolUse',     { actor: 'tall-seal', tool_name: 'Bash', tool_input: { command: 'ls coverage/ && cat coverage/coverage-summary.json | python3 -c "import sys,json; d=json.load(sys.stdin); print(d[\"total\"])"' } }, 28400),
  busEvent('hooks',  'PostToolUse',    { actor: 'tall-seal', tool_name: 'Bash' },                      29000),
  busEvent('mutations', 'MutationUpdate', { actor: 'tall-seal', issue_id: 'gt-coverage-e2e', agent_state: 'working' }, 29500),
  busEvent('agents', 'AgentIdle',      { actor: 'tall-seal', issue_id: 'gt-coverage-e2e' },            30000),
];

// arch-seal-1: Popup detection session
// - Reads doot handling, finds issue node, adds event trigger
export const SESSION_ARCH_SEAL_1 = [
  busEvent('agents', 'AgentStarted',   { actor: 'arch-seal-1', issue_id: 'gt-popup-detect' },          0),
  busEvent('hooks',  'SessionStart',   { actor: 'arch-seal-1', session_id: 'sess-arch1-001' },         300),
  busEvent('hooks',  'PreToolUse',     { actor: 'arch-seal-1', tool_name: 'Grep', tool_input: { pattern: 'spawnDoot|findAgentNode', path: '/beads3d/src/main.js' } }, 900),
  busEvent('hooks',  'PostToolUse',    { actor: 'arch-seal-1', tool_name: 'Grep' },                    1500),
  busEvent('hooks',  'PreToolUse',     { actor: 'arch-seal-1', tool_name: 'Read', tool_input: { file_path: '/beads3d/src/main.js', offset: 3690, limit: 100 } }, 1800),
  busEvent('hooks',  'PostToolUse',    { actor: 'arch-seal-1', tool_name: 'Read' },                    2600),
  busEvent('hooks',  'PreToolUse',     { actor: 'arch-seal-1', tool_name: 'Edit', tool_input: { file_path: '/beads3d/src/main.js' } }, 2900),
  busEvent('hooks',  'PostToolUse',    { actor: 'arch-seal-1', tool_name: 'Edit' },                    4500),
  busEvent('hooks',  'PreToolUse',     { actor: 'arch-seal-1', tool_name: 'Bash', tool_input: { command: 'npx playwright test tests/doots.spec.js --reporter=list 2>&1 | tail -25' } }, 4800),
  busEvent('hooks',  'PostToolUse',    { actor: 'arch-seal-1', tool_name: 'Bash' },                    20000),
  busEvent('mutations', 'MutationUpdate', { actor: 'arch-seal-1', issue_id: 'gt-popup-detect', agent_state: 'working' }, 20500),
  busEvent('agents', 'AgentIdle',      { actor: 'arch-seal-1', issue_id: 'gt-popup-detect' },          21000),
];

// All 9 agent sessions for convenience
export const ALL_SESSIONS = {
  'swift-newt':  SESSION_SWIFT_NEWT,
  'keen-bird':   SESSION_KEEN_BIRD,
  'vast-toad':   SESSION_VAST_TOAD,
  'arch-seal':   SESSION_ARCH_SEAL,
  'lush-mole':   SESSION_LUSH_MOLE,
  'deft-fox':    SESSION_DEFT_FOX,
  'stout-mare':  SESSION_STOUT_MARE,
  'tall-seal':   SESSION_TALL_SEAL,
  'arch-seal-1': SESSION_ARCH_SEAL_1,
};

// Helper: serialize a bus event to an SSE frame string
// stream: the SSE event name (matches EventSource.addEventListener stream)
export function toSseFrame(evt) {
  return `event: ${evt.stream}\ndata: ${JSON.stringify(evt)}\n\n`;
}

// Helper: build a complete SSE response body from an array of events
export function sessionToSseBody(events) {
  return events.map(toSseFrame).join('');
}

// Minimal response for Show (bd-feat1 as example)
export const MOCK_SHOW = {
  id: 'bd-feat1',
  title: 'Add user authentication',
  description: 'Implement OAuth2 authentication flow with PKCE support.\nIntegrate with Claude OAuth provider.\nSupport refresh token rotation.',
  status: 'in_progress',
  priority: 1,
  issue_type: 'feature',
  assignee: 'alice',
  labels: ['auth'],
  design: 'Use authorization code flow with PKCE. Store tokens in secure HTTP-only cookies.',
  notes: 'Blocked by Cloudflare device code endpoint from K8s pods.',
  created_at: '2026-01-20T09:00:00Z',
  updated_at: '2026-02-18T15:00:00Z',
  dependencies: [
    { depends_on_id: 'bd-task1', title: 'Write OAuth integration tests', type: 'blocks', status: 'open', priority: 2 },
  ],
  blocked_by: [],
};

// --- Synthetic data generator (bd-sg24u) ---
// Generates parameterized graph data for scalability and realism testing.
// Uses a seeded PRNG for deterministic output across test runs.

// Simple seeded PRNG (mulberry32) — deterministic from seed
function mulberry32(seed) {
  return function() {
    seed |= 0; seed = seed + 0x6D2B79F5 | 0;
    let t = Math.imul(seed ^ seed >>> 15, 1 | seed);
    t = t + Math.imul(t ^ t >>> 7, 61 | t) ^ t;
    return ((t ^ t >>> 14) >>> 0) / 4294967296;
  };
}

const AGENT_NAMES = [
  'swift-newt', 'keen-bird', 'vast-toad', 'arch-seal', 'lush-mole',
  'deft-fox', 'stout-mare', 'tall-seal', 'cool-trout', 'brave-rat',
  'wise-rook', 'neat-crab', 'tame-yak', 'true-crab', 'bold-hawk',
  'warm-dove', 'sharp-eel', 'dark-moth', 'quick-sole', 'avid-ant',
];

const ISSUE_TITLES = [
  'Implement auth RPC layer', 'Fix token refresh race', 'Add rate limiting middleware',
  'Dolt schema migration v3', 'NATS throughput benchmark', 'Redis cache TTL tuning',
  'Helm chart memory limits', 'K8s deployment manifests', 'E2E fixture expansion',
  'Graph API agent synthesis', 'Coverage collection pipeline', 'Label toggle key fix',
  'Doot popup detection', 'OOMKilled job investigation', 'WebSocket event streaming',
  'Structured logging pipeline', 'Metrics dashboard chart', 'Config loader refactor',
  'API gateway routing', 'Session registry cleanup', 'Credential rotation flow',
  'Slack bot reauth', 'Agent pod lifecycle', 'Coop broker merge',
  'Docker image optimization', 'CI pipeline parallelization', 'Visual regression baselines',
  'Accessibility audit', 'Mobile viewport support', 'Theme customization',
  'Bulk edit performance', 'Search index rebuild', 'Dependency cycle detection',
  'Epic progress tracking', 'Timeline scrubber polish', 'Camera smoothing',
  'Node clustering algorithm', 'Edge bundling', 'Minimap interaction',
  'Export format options', 'Import validation', 'Keyboard shortcut help',
];

const ISSUE_TYPES = ['task', 'task', 'task', 'feature', 'feature', 'bug', 'bug'];
const STATUSES = ['open', 'open', 'in_progress', 'in_progress', 'blocked', 'closed'];
const AGENT_STATUSES = ['active', 'active', 'active', 'idle', 'idle', 'crashed'];
const LABELS = ['auth', 'rpc', 'testing', 'dolt', 'k8s', 'helm', 'ui', 'api', 'ops', 'perf', 'infra', 'critical'];
const TOOL_NAMES = ['Read', 'Edit', 'Bash', 'Grep', 'Write', 'Glob', 'Task'];

/**
 * Generate a realistic graph dataset.
 * @param {Object} opts
 * @param {number} opts.seed - PRNG seed for determinism (default: 42)
 * @param {number} opts.nodeCount - Total issue nodes (default: 50)
 * @param {number} opts.agentCount - Number of agents (default: 8)
 * @param {number} opts.epicCount - Number of epics (default: 3)
 * @param {number} opts.closedRatio - Fraction of issues that are closed (default: 0.15)
 * @param {boolean} opts.fixedPositions - Add fx/fy/fz for deterministic layout (default: true)
 * @returns {{ nodes: Array, edges: Array, stats: Object }}
 */
export function generateGraph(opts = {}) {
  const {
    seed = 42,
    nodeCount = 50,
    agentCount = 8,
    epicCount = 3,
    closedRatio = 0.15,
    fixedPositions = true,
  } = opts;

  const rng = mulberry32(seed);
  const pick = (arr) => arr[Math.floor(rng() * arr.length)];

  const nodes = [];
  const edges = [];
  const issueIds = [];
  const baseDate = new Date('2026-02-01T08:00:00Z');

  // Create epics
  for (let i = 0; i < epicCount; i++) {
    const id = `syn-epic-${i}`;
    const angle = (i / epicCount) * 2 * Math.PI;
    nodes.push({
      id, title: `Epic: ${ISSUE_TITLES[i % ISSUE_TITLES.length]}`,
      status: 'in_progress', priority: 0, issue_type: 'epic', assignee: '',
      created_at: new Date(baseDate.getTime() - 30 * 86400000).toISOString(),
      updated_at: new Date(baseDate.getTime() + i * 86400000).toISOString(),
      labels: [pick(LABELS)], dep_count: 0, dep_by_count: 0, blocked_by: [],
      ...(fixedPositions && { fx: Math.cos(angle) * 200, fy: Math.sin(angle) * 200, fz: 0 }),
    });
    issueIds.push(id);
  }

  // Create issues
  for (let i = 0; i < nodeCount; i++) {
    const id = `syn-${String(i).padStart(3, '0')}`;
    const isClosed = rng() < closedRatio;
    const status = isClosed ? 'closed' : pick(STATUSES.filter(s => s !== 'closed'));
    const type = pick(ISSUE_TYPES);
    const title = ISSUE_TITLES[i % ISSUE_TITLES.length];
    const epicIdx = i % epicCount;
    const angle = (i / nodeCount) * 2 * Math.PI;
    const radius = 80 + rng() * 150;

    nodes.push({
      id, title, status, priority: Math.floor(rng() * 4),
      issue_type: type, assignee: '',
      created_at: new Date(baseDate.getTime() + i * 3600000).toISOString(),
      updated_at: new Date(baseDate.getTime() + (i + nodeCount) * 3600000).toISOString(),
      labels: rng() > 0.5 ? [pick(LABELS)] : [],
      dep_count: 0, dep_by_count: 0, blocked_by: [],
      ...(fixedPositions && { fx: Math.cos(angle) * radius, fy: Math.sin(angle) * radius, fz: (rng() - 0.5) * 40 }),
    });
    issueIds.push(id);

    // Parent-child edge to epic
    edges.push({ source: `syn-epic-${epicIdx}`, target: id, dep_type: 'parent-child' });
  }

  // Add dependency edges (~20% of issues block another)
  for (let i = 0; i < nodeCount; i++) {
    if (rng() < 0.2 && i + 1 < nodeCount) {
      const src = `syn-${String(i).padStart(3, '0')}`;
      const tgt = `syn-${String(i + 1 + Math.floor(rng() * 3)).padStart(3, '0')}`;
      if (issueIds.includes(tgt) && src !== tgt) {
        edges.push({ source: src, target: tgt, dep_type: 'blocks' });
      }
    }
  }

  // Create agents with mixed statuses
  const agents = [];
  for (let i = 0; i < agentCount; i++) {
    const name = AGENT_NAMES[i % AGENT_NAMES.length];
    const status = pick(AGENT_STATUSES);
    const angle = (i / agentCount) * 2 * Math.PI;
    nodes.push({
      id: `agent:${name}`, title: name, status, priority: 3,
      issue_type: 'agent', assignee: '',
      created_at: baseDate.toISOString(),
      updated_at: new Date(baseDate.getTime() + nodeCount * 3600000).toISOString(),
      labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [],
      ...(fixedPositions && { fx: Math.cos(angle) * 350, fy: Math.sin(angle) * 350, fz: 0 }),
    });
    agents.push(name);
  }

  // Assign agents to in_progress issues
  const inProgress = nodes.filter(n => n.status === 'in_progress' && n.issue_type !== 'agent' && n.issue_type !== 'epic');
  for (const issue of inProgress) {
    const agent = pick(agents);
    issue.assignee = agent;
    edges.push({ source: `agent:${agent}`, target: issue.id, dep_type: 'assigned_to' });
  }

  // Compute stats
  const stats = { total_open: 0, total_in_progress: 0, total_blocked: 0, total_closed: 0 };
  for (const n of nodes) {
    if (n.issue_type === 'agent') continue;
    if (n.status === 'open') stats.total_open++;
    else if (n.status === 'in_progress') stats.total_in_progress++;
    else if (n.status === 'blocked') stats.total_blocked++;
    else if (n.status === 'closed') stats.total_closed++;
  }

  return { nodes, edges, stats };
}

/**
 * Generate synthetic bus events for an agent session.
 * @param {string} agentName
 * @param {string} issueId - The bead the agent is working on
 * @param {Object} opts
 * @param {number} opts.seed - PRNG seed (default: hash of agentName)
 * @param {number} opts.toolCount - Number of tool use pairs (default: 4-8 random)
 * @param {boolean} opts.includeDecision - Add a decision event (default: false)
 * @param {boolean} opts.includeMail - Add a mail event (default: false)
 * @param {boolean} opts.crash - End with crash instead of idle (default: false)
 * @returns {Array} Array of bus event objects
 */
export function generateSession(agentName, issueId, opts = {}) {
  const nameSeed = agentName.split('').reduce((a, c) => a + c.charCodeAt(0), 0);
  const {
    seed = nameSeed,
    includeDecision = false,
    includeMail = false,
    crash = false,
  } = opts;

  const rng = mulberry32(seed);
  const pick = (arr) => arr[Math.floor(rng() * arr.length)];
  const toolCount = opts.toolCount || (4 + Math.floor(rng() * 5));

  const events = [];
  let offset = 0;

  // Agent started
  events.push(busEvent('agents', 'AgentStarted', { actor: agentName, issue_id: issueId }, offset));
  offset += 200 + Math.floor(rng() * 300);

  events.push(busEvent('hooks', 'SessionStart', { actor: agentName, session_id: `sess-${agentName}-${seed}` }, offset));
  offset += 300 + Math.floor(rng() * 500);

  // Tool use pairs
  const TOOL_INPUTS = {
    Read: () => ({ file_path: `/beads/internal/rpc/server_${pick(['graph', 'auth', 'config', 'stats'])}.go` }),
    Edit: () => ({ file_path: `/beads/internal/rpc/server_${pick(['graph', 'auth', 'config'])}.go` }),
    Bash: () => ({ command: pick(['go test ./...', 'make build', 'git status', 'kubectl get pods', 'helm lint']) }),
    Grep: () => ({ pattern: pick(['TODO', 'FIXME', 'func Test', 'import']), path: '/beads/internal' }),
    Write: () => ({ file_path: `/beads/internal/rpc/server_${pick(['new', 'test'])}.go` }),
    Glob: () => ({ pattern: pick(['**/*.go', '**/*.spec.js', 'tests/**']) }),
    Task: () => ({ description: 'Research implementation approach' }),
  };

  for (let i = 0; i < toolCount; i++) {
    const tool = pick(TOOL_NAMES);
    const input = TOOL_INPUTS[tool]();

    events.push(busEvent('hooks', 'PreToolUse', { actor: agentName, tool_name: tool, tool_input: input }, offset));
    offset += 300 + Math.floor(rng() * 2000);

    events.push(busEvent('hooks', 'PostToolUse', { actor: agentName, tool_name: tool }, offset));
    offset += 200 + Math.floor(rng() * 500);
  }

  // Optional decision event
  if (includeDecision) {
    events.push(busEvent('decisions', 'DecisionCreated', {
      actor: agentName, decision_id: `dec-${agentName}`, requested_by: agentName,
      prompt: 'Deploy changes to staging?',
      options: [{ id: 'y', label: 'Yes' }, { id: 'n', label: 'No' }],
    }, offset));
    offset += 1000;
  }

  // Optional mail event
  if (includeMail) {
    events.push(busEvent('mail', 'MailSent', {
      actor: agentName, from: agentName, to: pick(AGENT_NAMES),
      subject: 'Status update', body: 'Work is progressing well.',
    }, offset));
    offset += 500;
  }

  // Mutation update
  events.push(busEvent('mutations', 'MutationUpdate', { actor: agentName, issue_id: issueId, type: 'update' }, offset));
  offset += 500;

  // End state
  if (crash) {
    events.push(busEvent('agents', 'AgentCrashed', { actor: agentName, issue_id: issueId, error: 'context deadline exceeded' }, offset));
  } else {
    events.push(busEvent('agents', 'AgentIdle', { actor: agentName, issue_id: issueId }, offset));
  }

  return events;
}

// Pre-built large graph for scalability tests (100 nodes, 12 agents)
export const MOCK_LARGE_GRAPH = generateGraph({ seed: 99, nodeCount: 100, agentCount: 12, epicCount: 4 });

// Pre-built medium graph with decisions and mail events
export const MOCK_REALISTIC_GRAPH = generateGraph({ seed: 7, nodeCount: 40, agentCount: 6, epicCount: 2, closedRatio: 0.25 });
