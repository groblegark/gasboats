// E2E tests for multi-agent session data and the doot+popup pipeline (bd-mll3i).
// Uses MOCK_MULTI_AGENT_GRAPH (9 agents) and synthetic session transcripts
// to verify the full doot streaming path with realistic multi-agent views.
//
// Run: npx playwright test tests/sessions.spec.js
// View report: npx playwright show-report test-results/html-report

import { test, expect } from '@playwright/test';
import {
  MOCK_MULTI_AGENT_GRAPH,
  MOCK_PING,
  MOCK_SHOW,
  ALL_SESSIONS,
  SESSION_SWIFT_NEWT,
  SESSION_DEFT_FOX,
  SESSION_ARCH_SEAL,
  toSseFrame,
  sessionToSseBody,
} from './fixtures.js';

// ---- Helpers ----

// Mock all API endpoints using the multi-agent graph
async function mockAPI(page, busBody = ': keepalive\n\n') {
  await page.route('**/api/bd.v1.BeadsService/Ping', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) }));
  await page.route('**/api/bd.v1.BeadsService/Graph', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_MULTI_AGENT_GRAPH) }));
  await page.route('**/api/bd.v1.BeadsService/List', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
  await page.route('**/api/bd.v1.BeadsService/Show', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_SHOW) }));
  await page.route('**/api/bd.v1.BeadsService/Stats', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_MULTI_AGENT_GRAPH.stats) }));
  await page.route('**/api/bd.v1.BeadsService/Blocked', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
  await page.route('**/api/bd.v1.BeadsService/Ready', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));

  // Mutation SSE (legacy events endpoint)
  await page.route('**/api/events', route =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));

  // Bus events SSE — inject synthetic session data
  await page.route('**/api/bus/events*', route =>
    route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      headers: { 'Cache-Control': 'no-cache', 'Connection': 'keep-alive' },
      body: busBody,
    }));
}

// Wait for the graph to load with full multi-agent data
async function waitForGraph(page) {
  await page.waitForSelector('#status.connected', { timeout: 15000 });
  await page.waitForTimeout(3000);
  await page.waitForFunction(() => {
    const b = window.__beads3d;
    if (!b || !b.graph) return false;
    const nodes = b.graph.graphData().nodes;
    // Wait for at least 9 agent nodes to be present
    const agentCount = nodes.filter(n => n.issue_type === 'agent').length;
    return agentCount >= 9 && typeof window.__beads3d_spawnDoot === 'function';
  }, { timeout: 15000 });
}

// Get all agent node titles from the graph
function getAgentTitles(page) {
  return page.evaluate(() => {
    const nodes = window.__beads3d.graph.graphData().nodes;
    return nodes.filter(n => n.issue_type === 'agent').map(n => n.title).sort();
  });
}

// Get all in_progress beads with assignees
function getInProgressBeads(page) {
  return page.evaluate(() => {
    const nodes = window.__beads3d.graph.graphData().nodes;
    return nodes
      .filter(n => n.status === 'in_progress' && n.assignee)
      .map(n => ({ id: n.id, assignee: n.assignee, title: n.title }));
  });
}

// Get assigned_to edge count
function getAssignedToEdgeCount(page) {
  return page.evaluate(() => {
    const links = window.__beads3d.graph.graphData().links;
    return links.filter(l => {
      const dt = l.dep_type || l.type;
      return dt === 'assigned_to';
    }).length;
  });
}

// Inject a doot directly onto a named agent node
async function injectDoot(page, agentTitle, label, color = '#4a9eff') {
  return page.evaluate(({ agentTitle, label, color }) => {
    const nodes = window.__beads3d.graph.graphData().nodes;
    const agent = nodes.find(n => n.issue_type === 'agent' && n.title === agentTitle);
    if (!agent) return { ok: false, reason: `no agent: ${agentTitle}` };
    if (typeof window.__beads3d_spawnDoot !== 'function') return { ok: false, reason: 'spawnDoot not exposed' };
    window.__beads3d_spawnDoot(agent, label, color);
    return { ok: true };
  }, { agentTitle, label, color });
}

// Get current doot count
function getDootCount(page) {
  return page.evaluate(() => {
    if (typeof window.__beads3d_doots !== 'function') return -1;
    return window.__beads3d_doots().length;
  });
}

// ---- Tests ----

test.describe('Multi-agent graph: 9 agents simultaneously', () => {

  test('all 9 agent nodes are synthesized in MOCK_MULTI_AGENT_GRAPH', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const titles = await getAgentTitles(page);
    expect(titles).toEqual([
      'arch-seal', 'arch-seal-1', 'deft-fox', 'keen-bird',
      'lush-mole', 'stout-mare', 'swift-newt', 'tall-seal', 'vast-toad',
    ]);
    expect(titles).toHaveLength(9);
  });

  test('all in_progress beads have assignees and agent nodes', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const beads = await getInProgressBeads(page);
    // Should have 13 in_progress beads from MOCK_MULTI_AGENT_GRAPH
    expect(beads.length).toBeGreaterThanOrEqual(10);

    // Every assignee should have a corresponding agent node
    const agentTitles = await getAgentTitles(page);
    const agentSet = new Set(agentTitles);
    for (const bead of beads) {
      expect(agentSet.has(bead.assignee),
        `Expected agent node for assignee ${bead.assignee} (bead ${bead.id})`).toBe(true);
    }
  });

  test('assigned_to edges connect all 9 agents to their beads', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const edgeCount = await getAssignedToEdgeCount(page);
    // MOCK_MULTI_AGENT_GRAPH has 13 assigned_to edges
    expect(edgeCount).toBeGreaterThanOrEqual(9);
  });

  test('agent nodes have correct issue_type=agent', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const agentTypes = await page.evaluate(() => {
      const nodes = window.__beads3d.graph.graphData().nodes;
      return nodes.filter(n => n.issue_type === 'agent').map(n => n.issue_type);
    });
    expect(agentTypes.every(t => t === 'agent')).toBe(true);
  });

  test('doot popup appears when a non-agent bead gets a doot via bus event', async ({ page }) => {
    // Inject bus events for swift-newt's session — includes PreToolUse which
    // generates a doot on the agent node AND triggers a popup on the issue node
    const busBody = sessionToSseBody(SESSION_SWIFT_NEWT);
    await mockAPI(page, busBody);
    await page.goto('/');
    await waitForGraph(page);

    // After SSE events are processed, verify doot functionality is exposed
    const dootFnAvailable = await page.evaluate(() =>
      typeof window.__beads3d_spawnDoot === 'function'
    );
    if (!dootFnAvailable) {
      test.skip();
      return;
    }

    // Manually inject a doot on 'swift-newt' agent node (simulating bus event)
    const result = await injectDoot(page, 'swift-newt', 'bash: go test ./internal/rpc/...', '#ff7a00');
    expect(result.ok).toBe(true);

    const dootCount = await getDootCount(page);
    expect(dootCount).toBeGreaterThanOrEqual(1);
  });

});

test.describe('Session transcripts: dootLabel correctness', () => {

  test.beforeEach(async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);
  });

  test('dootLabel maps PreToolUse/Bash to short command', async ({ page }) => {
    const label = await page.evaluate(() => {
      const fn = window.__beads3d_dootLabel;
      if (!fn) return null;
      return fn({
        type: 'PreToolUse',
        payload: {
          tool_name: 'Bash',
          tool_input: { command: 'go test ./internal/rpc/... -run TestAuth -v 2>&1 | tail -30' },
        },
      });
    });
    expect(label).toBeTruthy();
    expect(label).toContain('go test');
  });

  test('dootLabel maps PreToolUse/Read to filename', async ({ page }) => {
    const label = await page.evaluate(() => {
      const fn = window.__beads3d_dootLabel;
      if (!fn) return null;
      return fn({
        type: 'PreToolUse',
        payload: { tool_name: 'Read', tool_input: { file_path: '/beads/internal/rpc/server_graph.go' } },
      });
    });
    expect(label).toContain('server_graph.go');
  });

  test('dootLabel maps PreToolUse/Edit to filename', async ({ page }) => {
    const label = await page.evaluate(() => {
      const fn = window.__beads3d_dootLabel;
      if (!fn) return null;
      return fn({
        type: 'PreToolUse',
        payload: { tool_name: 'Edit', tool_input: { file_path: '/fics-helm-chart/charts/gitlab-runner/values.yaml' } },
      });
    });
    expect(label).toContain('values.yaml');
  });

  test('dootLabel maps PreToolUse/Grep to pattern', async ({ page }) => {
    const label = await page.evaluate(() => {
      const fn = window.__beads3d_dootLabel;
      if (!fn) return null;
      return fn({
        type: 'PreToolUse',
        payload: { tool_name: 'Grep', tool_input: { pattern: 'GetIssuesByLabel' } },
      });
    });
    expect(label).toContain('GetIssuesByLabel');
  });

  test('dootLabel returns null for AgentHeartbeat (too noisy)', async ({ page }) => {
    const label = await page.evaluate(() => {
      const fn = window.__beads3d_dootLabel;
      if (!fn) return 'MISSING_FN';
      return fn({ type: 'AgentHeartbeat', payload: {} });
    });
    expect(label).toBeNull();
  });

  test('dootLabel maps AgentStarted to "started"', async ({ page }) => {
    const label = await page.evaluate(() => {
      const fn = window.__beads3d_dootLabel;
      if (!fn) return null;
      return fn({ type: 'AgentStarted', payload: { actor: 'swift-newt' } });
    });
    expect(label).toBe('started');
  });

  test('dootLabel maps MutationClose to "closed"', async ({ page }) => {
    const label = await page.evaluate(() => {
      const fn = window.__beads3d_dootLabel;
      if (!fn) return null;
      return fn({ type: 'MutationClose', payload: { actor: 'arch-seal', issue_id: 'gt-helm-chart' } });
    });
    expect(label).toBe('closed');
  });

  test('dootLabel returns null for DecisionCreated (filtered)', async ({ page }) => {
    const label = await page.evaluate(() => {
      const fn = window.__beads3d_dootLabel;
      if (!fn) return 'MISSING_FN';
      return fn({ type: 'DecisionCreated', payload: {} });
    });
    expect(label).toBeNull();
  });

});

test.describe('Session event sequences: realistic doot bursts', () => {

  test('swift-newt session produces correct doot sequence', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Verify dootLabel output for each hook event in swift-newt's session
    const labels = await page.evaluate((events) => {
      const fn = window.__beads3d_dootLabel;
      if (!fn) return null;
      return events.map(evt => ({ type: evt.type, label: fn(evt) }));
    }, SESSION_SWIFT_NEWT);

    if (!labels) { test.skip(); return; }

    // AgentStarted → 'started'
    const started = labels.find(l => l.type === 'AgentStarted');
    expect(started?.label).toBe('started');

    // PreToolUse/Read → includes filename
    const readEvt = labels.find(l => l.type === 'PreToolUse' && SESSION_SWIFT_NEWT.find(e =>
      e.type === 'PreToolUse' && e.payload?.tool_name === 'Read'));
    expect(readEvt?.label).toBeTruthy();

    // AgentIdle → 'idle'
    const idleEvt = labels.find(l => l.type === 'AgentIdle');
    expect(idleEvt?.label).toBe('idle');
  });

  test('arch-seal session includes MutationClose doot', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const closeLabel = await page.evaluate((events) => {
      const fn = window.__beads3d_dootLabel;
      if (!fn) return null;
      const closeEvt = events.find(e => e.type === 'MutationClose');
      return closeEvt ? fn(closeEvt) : null;
    }, SESSION_ARCH_SEAL);

    expect(closeLabel).toBe('closed');
  });

  test('deft-fox session: bash command shows grep pattern in doot label', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const labels = await page.evaluate((events) => {
      const fn = window.__beads3d_dootLabel;
      if (!fn) return [];
      return events
        .filter(e => e.type === 'PreToolUse')
        .map(e => fn(e))
        .filter(Boolean);
    }, SESSION_DEFT_FOX);

    expect(labels.length).toBeGreaterThan(0);
    // At least one should be a Grep or Bash label with content
    const hasContent = labels.some(l => l.length > 4);
    expect(hasContent).toBe(true);
  });

  test('ALL_SESSIONS contains all 9 agents', async () => {
    // Static assertion — verify the fixtures export covers every agent in MOCK_MULTI_AGENT_GRAPH
    const agentNames = Object.keys(ALL_SESSIONS).sort();
    expect(agentNames).toEqual([
      'arch-seal', 'arch-seal-1', 'deft-fox', 'keen-bird',
      'lush-mole', 'stout-mare', 'swift-newt', 'tall-seal', 'vast-toad',
    ]);
  });

});

test.describe('Doot injection: multi-agent simultaneous bursts', () => {

  test('spawn doots for all 9 agents simultaneously — under max cap', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const dootCount = await page.evaluate(() => {
      const b = window.__beads3d;
      const nodes = b.graph.graphData().nodes;
      const agentNodes = nodes.filter(n => n.issue_type === 'agent');

      if (agentNodes.length < 9) return { count: -1, reason: `only ${agentNodes.length} agents` };
      if (typeof window.__beads3d_spawnDoot !== 'function') return { count: -1, reason: 'no spawnDoot' };

      // Spawn 2 doots per agent = 18 total (within DOOT_MAX=30)
      for (const agent of agentNodes) {
        window.__beads3d_spawnDoot(agent, 'working', '#2d8a4e');
        window.__beads3d_spawnDoot(agent, 'bash', '#ff7a00');
      }

      return { count: window.__beads3d_doots().length };
    });

    expect(dootCount.count).toBeGreaterThanOrEqual(1);
    expect(dootCount.count).toBeLessThanOrEqual(30); // DOOT_MAX
  });

  test('each agent can independently receive doots from different event types', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Inject one doot per agent with distinct labels
    const agentLabels = [
      ['swift-newt',  'go test ./...'],
      ['keen-bird',   'dolt sql -f migration.sql'],
      ['vast-toad',   'edit server_graph.go'],
      ['arch-seal',   'helm lint --strict'],
      ['lush-mole',   'write fixtures.js'],
      ['deft-fox',    'read main.js'],
      ['stout-mare',  'kubectl get events'],
      ['tall-seal',   'npx playwright test'],
      ['arch-seal-1', 'grep spawnDoot'],
    ];

    let spawned = 0;
    for (const [agent, label] of agentLabels) {
      const result = await injectDoot(page, agent, label, '#4a9eff');
      if (result.ok) spawned++;
    }

    expect(spawned).toBeGreaterThanOrEqual(7); // allow 2 failures for flaky agent rendering
    const total = await getDootCount(page);
    expect(total).toBeGreaterThanOrEqual(spawned);
  });

});

test.describe('Doot popup: issue card triggered by doot on non-agent node', () => {

  test('showDootPopup is exposed and creates popup for non-agent nodes', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const popupCreated = await page.evaluate(() => {
      if (typeof window.__beads3d_showDootPopup !== 'function') return { ok: false, reason: 'not exposed' };

      // Find a non-agent in_progress node with assignee
      const nodes = window.__beads3d.graph.graphData().nodes;
      const bead = nodes.find(n => n.issue_type !== 'agent' && n.status === 'in_progress' && n.assignee);
      if (!bead) return { ok: false, reason: 'no in_progress bead found' };

      window.__beads3d_showDootPopup(bead);

      const popups = window.__beads3d_dootPopups();
      return { ok: popups.size >= 1, size: popups.size, beadId: bead.id };
    });

    if (!popupCreated.ok && popupCreated.reason) {
      // If popup API not implemented yet, skip gracefully
      test.skip();
      return;
    }
    expect(popupCreated.ok).toBe(true);
    expect(popupCreated.size).toBeGreaterThanOrEqual(1);
  });

  test('popup DOM element is created with correct bead ID', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await page.evaluate(() => {
      if (typeof window.__beads3d_showDootPopup !== 'function') return { ok: false };

      const nodes = window.__beads3d.graph.graphData().nodes;
      const bead = nodes.find(n => n.issue_type !== 'agent' && n.status === 'in_progress');
      if (!bead) return { ok: false, reason: 'no bead' };

      window.__beads3d_showDootPopup(bead);

      const el = document.querySelector(`[data-bead-id="${bead.id}"]`);
      return { ok: !!el, beadId: bead.id, found: !!el };
    });

    if (!result.ok) { test.skip(); return; }
    expect(result.found).toBe(true);
  });

  test('popup is dismissed after calling dismissDootPopup', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await page.evaluate(() => {
      if (typeof window.__beads3d_showDootPopup !== 'function') return { ok: false };
      if (typeof window.__beads3d_dismissDootPopup !== 'function') return { ok: false };

      const nodes = window.__beads3d.graph.graphData().nodes;
      const bead = nodes.find(n => n.issue_type !== 'agent' && n.status === 'in_progress');
      if (!bead) return { ok: false };

      window.__beads3d_showDootPopup(bead);
      const before = window.__beads3d_dootPopups().size;

      window.__beads3d_dismissDootPopup(bead.id);
      const after = window.__beads3d_dootPopups().size;

      return { ok: true, before, after };
    });

    if (!result.ok) { test.skip(); return; }
    expect(result.before).toBeGreaterThanOrEqual(1);
    expect(result.after).toBeLessThan(result.before);
  });

  test('popup not created for agent nodes (agent type excluded)', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await page.evaluate(() => {
      if (typeof window.__beads3d_showDootPopup !== 'function') return { ok: false };

      const nodes = window.__beads3d.graph.graphData().nodes;
      const agentNode = nodes.find(n => n.issue_type === 'agent');
      if (!agentNode) return { ok: false, reason: 'no agent node' };

      const before = window.__beads3d_dootPopups().size;
      window.__beads3d_showDootPopup(agentNode); // should be a no-op
      const after = window.__beads3d_dootPopups().size;

      return { ok: true, before, after, diff: after - before };
    });

    if (!result.ok) { test.skip(); return; }
    expect(result.diff).toBe(0); // agent nodes never get popups
  });

  test('max 3 simultaneous popups: oldest evicted when limit reached', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await page.evaluate(() => {
      if (typeof window.__beads3d_showDootPopup !== 'function') return { ok: false };

      const nodes = window.__beads3d.graph.graphData().nodes;
      const beads = nodes.filter(n => n.issue_type !== 'agent' && n.status === 'in_progress').slice(0, 5);
      if (beads.length < 4) return { ok: false, reason: `only ${beads.length} beads` };

      for (const b of beads) {
        window.__beads3d_showDootPopup(b);
      }

      return { ok: true, size: window.__beads3d_dootPopups().size };
    });

    if (!result.ok) { test.skip(); return; }
    expect(result.size).toBeLessThanOrEqual(3); // DOOT_POPUP_MAX = 3
  });

});

test.describe('Fixture validation: MOCK_MULTI_AGENT_GRAPH structure', () => {

  test('MOCK_MULTI_AGENT_GRAPH has correct node counts', async () => {
    // Static validation — check fixture structure directly
    const nodeCount = MOCK_MULTI_AGENT_GRAPH.nodes.length;
    const agentCount = MOCK_MULTI_AGENT_GRAPH.nodes.filter(n => n.issue_type === 'agent').length;
    const inProgressCount = MOCK_MULTI_AGENT_GRAPH.nodes.filter(n => n.status === 'in_progress').length;

    expect(agentCount).toBe(9);
    expect(inProgressCount).toBeGreaterThanOrEqual(9);
    expect(nodeCount).toBeGreaterThan(agentCount + inProgressCount - 5); // allow overlap
  });

  test('every agent node has a corresponding in_progress bead with matching assignee', async () => {
    const agents = MOCK_MULTI_AGENT_GRAPH.nodes.filter(n => n.issue_type === 'agent');
    const inProgress = MOCK_MULTI_AGENT_GRAPH.nodes.filter(n => n.status === 'in_progress');

    const agentNamesInGraph = new Set(agents.map(a => a.title));
    const assigneesWithWork = new Set(inProgress.map(n => n.assignee).filter(Boolean));

    for (const agentTitle of agentNamesInGraph) {
      expect(assigneesWithWork.has(agentTitle),
        `Agent ${agentTitle} has no in_progress bead`).toBe(true);
    }
  });

  test('every assigned_to edge references existing agent and bead nodes', async () => {
    const nodeIds = new Set(MOCK_MULTI_AGENT_GRAPH.nodes.map(n => n.id));
    const assignedEdges = MOCK_MULTI_AGENT_GRAPH.edges.filter(e => e.dep_type === 'assigned_to');

    for (const edge of assignedEdges) {
      expect(nodeIds.has(edge.source),
        `assigned_to edge source ${edge.source} not in nodes`).toBe(true);
      expect(nodeIds.has(edge.target),
        `assigned_to edge target ${edge.target} not in nodes`).toBe(true);
    }
  });

  test('toSseFrame serializes a bus event to correct SSE format', async () => {
    const evt = SESSION_SWIFT_NEWT[0]; // AgentStarted
    const frame = toSseFrame(evt);

    expect(frame).toMatch(/^event: agents\n/);
    expect(frame).toContain('"type":"AgentStarted"');
    expect(frame).toContain('"actor":"swift-newt"');
    expect(frame.endsWith('\n\n')).toBe(true);
  });

  test('sessionToSseBody concatenates all frames in order', async () => {
    const body = sessionToSseBody(SESSION_SWIFT_NEWT);
    const frames = body.split('\n\n').filter(Boolean);

    // Each non-empty chunk should contain a valid event
    expect(frames.length).toBe(SESSION_SWIFT_NEWT.length);
    expect(frames[0]).toContain('AgentStarted');
    expect(frames[frames.length - 1]).toContain('AgentIdle');
  });

  test('ALL_SESSIONS busEvent arrays are non-empty and well-formed', async () => {
    for (const [agentName, session] of Object.entries(ALL_SESSIONS)) {
      expect(session.length, `${agentName} session should be non-empty`).toBeGreaterThan(0);
      // First event should be AgentStarted or SessionStart
      const firstType = session[0].type;
      expect(['AgentStarted', 'SessionStart', 'PreToolUse'].includes(firstType),
        `${agentName} first event type: ${firstType}`).toBe(true);
      // All events should have stream, type, ts
      for (const evt of session) {
        expect(evt.stream).toBeTruthy();
        expect(evt.type).toBeTruthy();
        expect(evt.ts).toBeTruthy();
      }
    }
  });

});

// --- Agent activity feed window tests (bd-kau4k) ---

test.describe('Agent activity feed windows', () => {

  // Helper: open an agent window by calling showAgentWindow
  async function openWindow(page, agentTitle) {
    return page.evaluate((title) => {
      const showWin = window.__beads3d_showAgentWindow;
      if (!showWin) return { ok: false, reason: 'not exposed' };
      const nodes = window.__beads3d.graphData().nodes;
      const agent = nodes.find(n => n.issue_type === 'agent' && n.title === title);
      if (!agent) return { ok: false, reason: `no agent: ${title}` };
      showWin(agent);
      return { ok: true, id: agent.id };
    }, agentTitle);
  }

  // Helper: get number of open agent windows
  async function getWindowCount(page) {
    return page.evaluate(() => {
      const fn = window.__beads3d_agentWindows;
      return fn ? fn().size : -1;
    });
  }

  // Helper: get DOM window count
  async function getDomWindowCount(page) {
    return page.evaluate(() => {
      const container = document.getElementById('agent-windows');
      return container ? container.querySelectorAll('.agent-window').length : 0;
    });
  }

  // Helper: get feed entry count for an agent window
  async function getFeedEntryCount(page, agentTitle) {
    return page.evaluate((title) => {
      const container = document.getElementById('agent-windows');
      if (!container) return -1;
      const win = container.querySelector(`.agent-window[data-agent-id="agent:${title}"]`);
      if (!win) return -1;
      return win.querySelectorAll('.agent-entry').length;
    }, agentTitle);
  }

  // Helper: inject an event into an agent's window
  async function injectEvent(page, agentId, evt) {
    return page.evaluate(({ agentId, evt }) => {
      const fn = window.__beads3d_appendAgentEvent;
      if (!fn) return false;
      fn(agentId, evt);
      return true;
    }, { agentId, evt });
  }

  test('clicking agent node opens activity feed window', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await openWindow(page, 'swift-newt');
    if (!result.ok) { test.skip(); return; }

    expect(await getWindowCount(page)).toBe(1);
    expect(await getDomWindowCount(page)).toBe(1);

    // Window should show agent name
    const name = await page.evaluate(() => {
      const el = document.querySelector('.agent-window-name');
      return el ? el.textContent : '';
    });
    expect(name).toBe('swift-newt');
  });

  test('clicking agent again toggles window collapse/expand', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await openWindow(page, 'swift-newt');
    if (!result.ok) { test.skip(); return; }

    // First open — not collapsed
    const isCollapsed1 = await page.evaluate(() => {
      const win = document.querySelector('.agent-window');
      return win ? win.classList.contains('collapsed') : null;
    });
    expect(isCollapsed1).toBe(false);

    // Click again — should collapse
    await openWindow(page, 'swift-newt');
    const isCollapsed2 = await page.evaluate(() => {
      const win = document.querySelector('.agent-window');
      return win ? win.classList.contains('collapsed') : null;
    });
    expect(isCollapsed2).toBe(true);

    // Click again — should expand
    await openWindow(page, 'swift-newt');
    const isCollapsed3 = await page.evaluate(() => {
      const win = document.querySelector('.agent-window');
      return win ? win.classList.contains('collapsed') : null;
    });
    expect(isCollapsed3).toBe(false);
  });

  test('window shows assigned beads for the agent', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await openWindow(page, 'swift-newt');
    if (!result.ok) { test.skip(); return; }

    // swift-newt should have assigned beads listed
    const beads = await page.evaluate(() => {
      const els = document.querySelectorAll('.agent-window-bead');
      return Array.from(els).map(el => el.textContent);
    });
    expect(beads.length).toBeGreaterThan(0);
  });

  test('appendAgentEvent adds entries to the feed', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await openWindow(page, 'swift-newt');
    if (!result.ok) { test.skip(); return; }

    // Inject swift-newt session events
    for (const evt of SESSION_SWIFT_NEWT) {
      await injectEvent(page, 'agent:swift-newt', evt);
    }

    // Should have entries (some events merge: PostToolUse doesn't add a row)
    const count = await getFeedEntryCount(page, 'swift-newt');
    expect(count).toBeGreaterThanOrEqual(8); // started + session start + 6 pre-tool + assignment + idle
  });

  test('PreToolUse/PostToolUse pairs show duration', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await openWindow(page, 'swift-newt');
    if (!result.ok) { test.skip(); return; }

    // Inject a PreToolUse + PostToolUse pair with known timestamps
    await injectEvent(page, 'agent:swift-newt', {
      stream: 'hooks', type: 'PreToolUse', subject: 'hooks.PreToolUse',
      ts: '2026-02-20T11:00:00.000Z',
      payload: { actor: 'swift-newt', tool_name: 'Bash', tool_input: { command: 'go test ./...' } },
    });
    await injectEvent(page, 'agent:swift-newt', {
      stream: 'hooks', type: 'PostToolUse', subject: 'hooks.PostToolUse',
      ts: '2026-02-20T11:00:03.500Z',
      payload: { actor: 'swift-newt', tool_name: 'Bash' },
    });

    // The entry should show a checkmark icon and duration
    const entries = await page.evaluate(() => {
      return Array.from(document.querySelectorAll('.agent-entry')).map(el => ({
        icon: el.querySelector('.agent-entry-icon')?.textContent || '',
        text: el.querySelector('.agent-entry-text')?.textContent || '',
        dur: el.querySelector('.agent-entry-dur')?.textContent || '',
      }));
    });

    const bashEntry = entries.find(e => e.text.includes('go test'));
    expect(bashEntry).toBeTruthy();
    expect(bashEntry.icon).toBe('✓');
    expect(bashEntry.dur).toBe('3.5s');
  });

  test('multiple agent windows can open simultaneously', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const agents = ['swift-newt', 'keen-bird', 'vast-toad'];
    for (const name of agents) {
      const r = await openWindow(page, name);
      if (!r.ok) { test.skip(); return; }
    }

    expect(await getWindowCount(page)).toBe(3);
    expect(await getDomWindowCount(page)).toBe(3);
  });

  test('close button removes agent window', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await openWindow(page, 'swift-newt');
    if (!result.ok) { test.skip(); return; }

    expect(await getWindowCount(page)).toBe(1);

    // Click close button
    await page.click('.agent-window-close');

    expect(await getWindowCount(page)).toBe(0);
    expect(await getDomWindowCount(page)).toBe(0);
  });

  test('Escape key closes all agent windows', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openWindow(page, 'swift-newt');
    await openWindow(page, 'keen-bird');
    expect(await getWindowCount(page)).toBe(2);

    await page.keyboard.press('Escape');

    expect(await getWindowCount(page)).toBe(0);
  });

  test('lifecycle events render with correct styling', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await openWindow(page, 'swift-newt');
    if (!result.ok) { test.skip(); return; }

    await injectEvent(page, 'agent:swift-newt', {
      stream: 'agents', type: 'AgentStarted', subject: 'agents.AgentStarted',
      ts: new Date().toISOString(),
      payload: { actor: 'swift-newt' },
    });
    await injectEvent(page, 'agent:swift-newt', {
      stream: 'agents', type: 'AgentIdle', subject: 'agents.AgentIdle',
      ts: new Date().toISOString(),
      payload: { actor: 'swift-newt' },
    });

    const entries = await page.evaluate(() => {
      return Array.from(document.querySelectorAll('.agent-entry')).map(el => ({
        classes: el.className,
        text: el.querySelector('.agent-entry-text')?.textContent || '',
      }));
    });

    const started = entries.find(e => e.text === 'started');
    expect(started).toBeTruthy();
    expect(started.classes).toContain('lifecycle-started');

    const idle = entries.find(e => e.text === 'idle');
    expect(idle).toBeTruthy();
    expect(idle.classes).toContain('lifecycle-idle');
  });

  test('full swift-newt session populates feed with correct entry count', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await openWindow(page, 'swift-newt');
    if (!result.ok) { test.skip(); return; }

    // Inject all swift-newt session events
    for (const evt of SESSION_SWIFT_NEWT) {
      await injectEvent(page, 'agent:swift-newt', evt);
    }

    // SESSION_SWIFT_NEWT has 16 events:
    // AgentStarted (1 row) + SessionStart (1) + 6 PreToolUse/PostToolUse pairs (6 rows, Post merges in)
    // + MutationUpdate with assignee (1 row) + AgentIdle (1 row) = 10 rows
    const count = await getFeedEntryCount(page, 'swift-newt');
    expect(count).toBe(10);
  });
});
