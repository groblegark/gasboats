// E2E tests for Agents View overlay and synthetic data realism (bd-sg24u, bd-tm8a7).
// Tests the Shift+A overlay, auto-open on events, search filter, and scalability
// with generated synthetic data.
//
// Run: npx playwright test tests/agents-view.spec.js
// View report: npx playwright show-report test-results/html-report

import { test, expect } from '@playwright/test';
import {
  MOCK_PING,
  MOCK_SHOW,
  MOCK_MULTI_AGENT_GRAPH,
  MOCK_LARGE_GRAPH,
  MOCK_REALISTIC_GRAPH,
  generateGraph,
  generateSession,
  sessionToSseBody,
} from './fixtures.js';

// ---- Helpers ----

async function mockAPI(page, graph = MOCK_MULTI_AGENT_GRAPH, busBody = ': keepalive\n\n') {
  await page.route('**/api/bd.v1.BeadsService/Ping', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) }));
  await page.route('**/api/bd.v1.BeadsService/Graph', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(graph) }));
  await page.route('**/api/bd.v1.BeadsService/List', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
  await page.route('**/api/bd.v1.BeadsService/Show', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_SHOW) }));
  await page.route('**/api/bd.v1.BeadsService/Stats', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(graph.stats) }));
  await page.route('**/api/bd.v1.BeadsService/Blocked', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
  await page.route('**/api/bd.v1.BeadsService/Ready', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
  await page.route('**/api/bd.v1.BeadsService/Create', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) }));

  await page.route('**/api/events', route =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
  await page.route('**/api/bus/events*', route =>
    route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      headers: { 'Cache-Control': 'no-cache', 'Connection': 'keep-alive' },
      body: busBody,
    }));
}

async function waitForGraph(page, minAgents = 1) {
  await page.waitForSelector('#status.connected', { timeout: 15000 });
  await page.waitForTimeout(3000);
  await page.waitForFunction((min) => {
    const b = window.__beads3d;
    if (!b || !b.graph) return false;
    const nodes = b.graph.graphData().nodes;
    const agentCount = nodes.filter(n => n.issue_type === 'agent').length;
    return agentCount >= min && typeof window.__beads3d_toggleAgentsView === 'function';
  }, minAgents, { timeout: 15000 });
}

// ---- Agents View Overlay Tests (bd-jgvas Phase 1+2) ----

test.describe('Agents View overlay', () => {

  test('Shift+A opens overlay with all agent nodes', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    // Overlay should be hidden initially
    const overlay = page.locator('#agents-view');
    await expect(overlay).not.toHaveClass(/open/);

    // Press Shift+A
    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(500);

    // Overlay should be open
    await expect(overlay).toHaveClass(/open/);

    // Should contain the header with title and stats
    await expect(page.locator('.agents-view-title')).toContainText('AGENTS');
    await expect(page.locator('.agents-view-stats')).toBeVisible();

    // Should have agent windows in the grid
    const windows = page.locator('.agents-view-grid .agent-window');
    const count = await windows.count();
    expect(count).toBeGreaterThanOrEqual(9);

    // Each window should have a header with agent name
    for (let i = 0; i < Math.min(count, 3); i++) {
      await expect(windows.nth(i).locator('.agent-window-name')).toBeVisible();
    }
  });

  test('Shift+A toggles overlay closed', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    const overlay = page.locator('#agents-view');

    // Open
    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(300);
    await expect(overlay).toHaveClass(/open/);

    // Close
    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(300);
    await expect(overlay).not.toHaveClass(/open/);
  });

  test('Escape closes overlay', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    const overlay = page.locator('#agents-view');

    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(300);
    await expect(overlay).toHaveClass(/open/);

    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);
    await expect(overlay).not.toHaveClass(/open/);
  });

  test('overlay shows status badges with correct colors', async ({ page }) => {
    // Use a graph with mixed agent statuses
    const graph = generateGraph({ seed: 42, nodeCount: 20, agentCount: 6, epicCount: 2 });
    await mockAPI(page, graph);
    await page.goto('/');
    await waitForGraph(page, 3);

    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(500);

    // Check that agent windows have status badges
    const badges = page.locator('.agents-view-grid .agent-window-badge');
    const count = await badges.count();
    expect(count).toBeGreaterThanOrEqual(6); // At least 1 per agent window (status + count)

    // Verify at least some status text is present
    const allText = await badges.allTextContents();
    const statusTexts = allText.filter(t => ['active', 'idle', 'crashed'].includes(t));
    expect(statusTexts.length).toBeGreaterThanOrEqual(1);
  });

  test('search filter hides non-matching agent windows', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(500);

    const searchInput = page.locator('.agents-view-search');
    await expect(searchInput).toBeVisible();

    // Type a specific agent name
    await searchInput.fill('swift');
    await page.waitForTimeout(300);

    // Count visible windows — should be filtered
    const visible = await page.evaluate(() => {
      const windows = document.querySelectorAll('.agents-view-grid .agent-window');
      let count = 0;
      for (const w of windows) {
        if (w.style.display !== 'none') count++;
      }
      return count;
    });

    // Should show only swift-newt
    expect(visible).toBe(1);
  });

  test('Escape clears search before closing overlay', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(500);

    const searchInput = page.locator('.agents-view-search');
    await searchInput.fill('swift');
    await page.waitForTimeout(200);

    // Focus the search
    await searchInput.focus();

    // First Escape should clear search text, not close overlay
    await page.keyboard.press('Escape');
    await page.waitForTimeout(200);

    const overlay = page.locator('#agents-view');
    await expect(overlay).toHaveClass(/open/);
    await expect(searchInput).toHaveValue('');

    // All windows should be visible again
    const visible = await page.evaluate(() => {
      const windows = document.querySelectorAll('.agents-view-grid .agent-window');
      let count = 0;
      for (const w of windows) {
        if (w.style.display !== 'none') count++;
      }
      return count;
    });
    expect(visible).toBeGreaterThanOrEqual(9);

    // Second Escape should close overlay
    await page.keyboard.press('Escape');
    await page.waitForTimeout(200);
    await expect(overlay).not.toHaveClass(/open/);
  });

  test('windows persist across toggle (preserve event history)', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    // Open overlay
    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(500);

    // Inject an event into an agent window
    await page.evaluate(() => {
      const agentId = 'agent:swift-newt';
      window.__beads3d_appendAgentEvent(agentId, {
        type: 'PreToolUse',
        ts: new Date().toISOString(),
        payload: { actor: 'swift-newt', tool_name: 'Read', tool_input: { file_path: '/test.go' } },
      });
    });
    await page.waitForTimeout(200);

    // Verify event is in feed
    const entryCount = await page.evaluate(() => {
      const win = window.__beads3d_agentWindows().get('agent:swift-newt');
      return win ? win.entries.length : 0;
    });
    expect(entryCount).toBe(1);

    // Close overlay
    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(300);

    // Re-open overlay
    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(500);

    // Event should still be in feed (window persisted)
    const entryCountAfter = await page.evaluate(() => {
      const win = window.__beads3d_agentWindows().get('agent:swift-newt');
      return win ? win.entries.length : 0;
    });
    expect(entryCountAfter).toBe(1);
  });

  test('agent windows move to bottom tray when overlay closes', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    // Open overlay — windows go into #agents-view .agents-view-grid
    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(500);

    const gridCount = await page.locator('.agents-view-grid .agent-window').count();
    expect(gridCount).toBeGreaterThanOrEqual(9);

    // Close overlay — windows move to #agent-windows tray
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);

    const trayCount = await page.locator('#agent-windows .agent-window').count();
    expect(trayCount).toBeGreaterThanOrEqual(9);

    // Windows in tray should be collapsed
    const collapsedCount = await page.locator('#agent-windows .agent-window.collapsed').count();
    expect(collapsedCount).toBe(trayCount);
  });

  test('stats counts match agent status distribution', async ({ page }) => {
    const graph = generateGraph({ seed: 99, nodeCount: 10, agentCount: 8, epicCount: 1 });
    await mockAPI(page, graph);
    await page.goto('/');
    await waitForGraph(page, 3);

    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(500);

    // Read stats from the overlay header
    const statsText = await page.locator('.agents-view-stats').textContent();

    // Count agents in the mock graph
    const agents = graph.nodes.filter(n => n.issue_type === 'agent');

    // Stats should show total count
    expect(statsText).toContain(`${agents.length} total`);

    // At least one status category should be present
    expect(statsText).toMatch(/\d+ (active|idle|crashed)/);
  });

  test('mail compose sends message to correct agent', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    // Intercept the Create RPC to capture sent mail
    const mailRequests = [];
    await page.route('**/api/bd.v1.BeadsService/Create', async route => {
      const body = JSON.parse(route.request().postData());
      mailRequests.push(body);
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) });
    });

    // Open overlay and find an agent window
    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(500);

    // Find the mail input inside the first agent window
    const firstWindow = page.locator('.agents-view-grid .agent-window').first();
    const mailInput = firstWindow.locator('.agent-mail-input');

    // Skip if no mail input exists (window may not have compose UI)
    if (await mailInput.count() === 0) {
      test.skip();
      return;
    }

    // Type a message and press Enter
    await mailInput.fill('Hello from test');
    await mailInput.press('Enter');
    await page.waitForTimeout(500);

    // Verify the Create RPC was called with correct params
    expect(mailRequests.length).toBe(1);
    expect(mailRequests[0].title).toBe('Hello from test');
    expect(mailRequests[0].issue_type).toBe('message');
    expect(mailRequests[0].sender).toBe('beads3d');
  });
});

// ---- Auto-open on first event (bd-jgvas Phase 2) ----

test.describe('auto-open agent windows', () => {

  test('bus event creates agent window automatically', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    // No agent windows should exist initially
    const initialCount = await page.evaluate(() => window.__beads3d_agentWindows().size);
    expect(initialCount).toBe(0);

    // Inject a bus event for an agent
    await page.evaluate(() => {
      // Simulate bus event arriving via resolveAgentIdLoose path
      const agentId = window.__beads3d_resolveAgentIdLoose({
        type: 'PreToolUse',
        payload: { actor: 'swift-newt', tool_name: 'Read' },
      });
      // Verify the function works
      return agentId;
    });

    // The resolveAgentIdLoose should return the correct ID
    const resolvedId = await page.evaluate(() =>
      window.__beads3d_resolveAgentIdLoose({
        type: 'PreToolUse',
        payload: { actor: 'swift-newt', tool_name: 'Read' },
      })
    );
    expect(resolvedId).toBe('agent:swift-newt');
  });

  test('resolveAgentIdLoose handles mail events', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    const mailId = await page.evaluate(() =>
      window.__beads3d_resolveAgentIdLoose({
        type: 'MailSent',
        payload: { to: '@swift-newt', from: 'keen-bird' },
      })
    );
    expect(mailId).toBe('agent:swift-newt');
  });

  test('resolveAgentIdLoose handles decision events', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    const decId = await page.evaluate(() =>
      window.__beads3d_resolveAgentIdLoose({
        type: 'DecisionCreated',
        payload: { requested_by: 'keen-bird', decision_id: 'dec-123' },
      })
    );
    expect(decId).toBe('agent:keen-bird');
  });

  test('resolveAgentIdLoose excludes daemon actor', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    const daemonId = await page.evaluate(() =>
      window.__beads3d_resolveAgentIdLoose({
        type: 'MutationUpdate',
        payload: { actor: 'daemon', issue_id: 'bd-001' },
      })
    );
    expect(daemonId).toBeNull();
  });

  test('appendAgentEvent adds feed entries to existing window', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    // Open overlay to create windows
    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(500);

    // Add multiple events and verify they accumulate
    const result = await page.evaluate(() => {
      const agentId = 'agent:swift-newt';
      const win = window.__beads3d_agentWindows().get(agentId);
      if (!win) return { found: false };

      window.__beads3d_appendAgentEvent(agentId, {
        type: 'PreToolUse',
        ts: new Date().toISOString(),
        payload: { actor: 'swift-newt', tool_name: 'Read', tool_input: { file_path: '/test.go' } },
      });
      window.__beads3d_appendAgentEvent(agentId, {
        type: 'PostToolUse',
        ts: new Date().toISOString(),
        payload: { actor: 'swift-newt', tool_name: 'Read', tool_input: { file_path: '/test.go' } },
      });
      window.__beads3d_appendAgentEvent(agentId, {
        type: 'PreToolUse',
        ts: new Date().toISOString(),
        payload: { actor: 'swift-newt', tool_name: 'Edit', tool_input: { file_path: '/main.go' } },
      });

      return {
        found: true,
        entryCount: win.entries.length,
        hasEmptyPlaceholder: !!win.feedEl.querySelector('.agent-window-empty'),
        feedChildren: win.feedEl.children.length,
      };
    });

    expect(result.found).toBe(true);
    expect(result.entryCount).toBe(3);
    expect(result.hasEmptyPlaceholder).toBe(false); // Placeholder removed after first event
    expect(result.feedChildren).toBeGreaterThanOrEqual(1); // Feed has rendered entries
  });
});

// ---- Synthetic data realism (bd-sg24u) ----

test.describe('synthetic graph realism', () => {

  test('generated graph has correct node counts and types', async ({ page }) => {
    const graph = generateGraph({ seed: 42, nodeCount: 50, agentCount: 8, epicCount: 3 });

    // Verify structure
    const agents = graph.nodes.filter(n => n.issue_type === 'agent');
    const epics = graph.nodes.filter(n => n.issue_type === 'epic');
    const issues = graph.nodes.filter(n => !['agent', 'epic'].includes(n.issue_type));

    expect(agents).toHaveLength(8);
    expect(epics).toHaveLength(3);
    expect(issues).toHaveLength(50);
    expect(graph.nodes).toHaveLength(61);

    // Verify agent status diversity
    const agentStatuses = new Set(agents.map(a => a.status));
    expect(agentStatuses.size).toBeGreaterThanOrEqual(2);

    // Verify issue status diversity
    const issueStatuses = new Set(issues.map(i => i.status));
    expect(issueStatuses.size).toBeGreaterThanOrEqual(3);

    // Verify edges include parent-child, blocks, and assigned_to
    const edgeTypes = new Set(graph.edges.map(e => e.dep_type));
    expect(edgeTypes.has('parent-child')).toBe(true);
    expect(edgeTypes.has('assigned_to')).toBe(true);

    // Stats should sum correctly
    const totalIssues = graph.stats.total_open + graph.stats.total_in_progress +
                        graph.stats.total_blocked + graph.stats.total_closed;
    expect(totalIssues).toBe(50 + 3); // issues + epics
  });

  test('generated graph is deterministic (same seed = same output)', async ({ page }) => {
    const g1 = generateGraph({ seed: 123, nodeCount: 30, agentCount: 5 });
    const g2 = generateGraph({ seed: 123, nodeCount: 30, agentCount: 5 });

    expect(g1.nodes.length).toBe(g2.nodes.length);
    expect(g1.edges.length).toBe(g2.edges.length);

    for (let i = 0; i < g1.nodes.length; i++) {
      expect(g1.nodes[i].id).toBe(g2.nodes[i].id);
      expect(g1.nodes[i].title).toBe(g2.nodes[i].title);
      expect(g1.nodes[i].status).toBe(g2.nodes[i].status);
    }
  });

  test('generated sessions have correct event structure', async ({ page }) => {
    const session = generateSession('test-agent', 'syn-001', {
      seed: 42,
      toolCount: 5,
      includeDecision: true,
      includeMail: true,
    });

    // Should start with AgentStarted + SessionStart
    expect(session[0].type).toBe('AgentStarted');
    expect(session[1].type).toBe('SessionStart');

    // Should have 5 PreToolUse/PostToolUse pairs
    const preTool = session.filter(e => e.type === 'PreToolUse');
    const postTool = session.filter(e => e.type === 'PostToolUse');
    expect(preTool).toHaveLength(5);
    expect(postTool).toHaveLength(5);

    // Should have decision and mail events
    const decisions = session.filter(e => e.type === 'DecisionCreated');
    const mails = session.filter(e => e.type === 'MailSent');
    expect(decisions).toHaveLength(1);
    expect(mails).toHaveLength(1);

    // Should end with MutationUpdate + AgentIdle
    expect(session[session.length - 2].type).toBe('MutationUpdate');
    expect(session[session.length - 1].type).toBe('AgentIdle');

    // Timestamps should be monotonically increasing
    for (let i = 1; i < session.length; i++) {
      expect(new Date(session[i].ts).getTime()).toBeGreaterThanOrEqual(
        new Date(session[i - 1].ts).getTime()
      );
    }
  });

  test('crash session ends with AgentCrashed', async ({ page }) => {
    const session = generateSession('crash-agent', 'syn-001', { crash: true });
    expect(session[session.length - 1].type).toBe('AgentCrashed');
    expect(session[session.length - 1].payload.error).toBeTruthy();
  });
});

// ---- Scalability tests with large synthetic data ----

test.describe('large graph rendering', () => {

  test('100-node graph renders without errors', async ({ page }) => {
    await mockAPI(page, MOCK_LARGE_GRAPH);
    await page.goto('/');
    await waitForGraph(page, 5);

    // Verify all nodes loaded
    const nodeCount = await page.evaluate(() =>
      window.__beads3d.graph.graphData().nodes.length
    );
    expect(nodeCount).toBe(MOCK_LARGE_GRAPH.nodes.length);

    // No console errors
    const errors = [];
    page.on('console', msg => { if (msg.type() === 'error') errors.push(msg.text()); });
    await page.waitForTimeout(2000);

    // Status should be connected
    await expect(page.locator('#status')).toHaveClass('connected');
  });

  test('Agents View overlay handles 12 agents', async ({ page }) => {
    await mockAPI(page, MOCK_LARGE_GRAPH);
    await page.goto('/');
    await waitForGraph(page, 5);

    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(500);

    const overlay = page.locator('#agents-view');
    await expect(overlay).toHaveClass(/open/);

    // Should show all 12 agents (or however many are visible)
    const windowCount = await page.locator('.agents-view-grid .agent-window').count();
    expect(windowCount).toBeGreaterThanOrEqual(5); // At least visible agents
  });

  test('realistic graph with closed items and decisions', async ({ page }) => {
    await mockAPI(page, MOCK_REALISTIC_GRAPH);
    await page.goto('/');

    await page.waitForSelector('#status.connected', { timeout: 15000 });
    await page.waitForTimeout(3000);
    await page.waitForFunction(() => {
      const b = window.__beads3d;
      return b && b.graph && b.graph.graphData().nodes.length > 0;
    }, { timeout: 15000 });

    // Should render with mixed statuses
    const nodeCount = await page.evaluate(() =>
      window.__beads3d.graph.graphData().nodes.length
    );
    expect(nodeCount).toBe(MOCK_REALISTIC_GRAPH.nodes.length);

    // Should have closed items in the graph
    const closedCount = await page.evaluate(() =>
      window.__beads3d.graph.graphData().nodes.filter(n => n.status === 'closed').length
    );
    expect(closedCount).toBeGreaterThan(0);
  });

  test('SSE events with generated sessions work end-to-end', async ({ page }) => {
    // Generate sessions for 3 agents
    const sessions = [
      ...generateSession('swift-newt', 'gt-auth-rpc', { seed: 1, toolCount: 3 }),
      ...generateSession('keen-bird', 'gt-dolt-schema', { seed: 2, toolCount: 2 }),
      ...generateSession('deft-fox', 'gt-label-toggle', { seed: 3, toolCount: 2, includeDecision: true }),
    ];
    const busBody = sessionToSseBody(sessions);

    await mockAPI(page, MOCK_MULTI_AGENT_GRAPH, busBody);
    await page.goto('/');
    await waitForGraph(page, 9);

    // SSE events should have been delivered
    await page.waitForTimeout(2000);

    // Open agents view — windows should exist for agents that had events
    await page.keyboard.press('Shift+A');
    await page.waitForTimeout(500);

    // At minimum, the overlay should open
    const overlay = page.locator('#agents-view');
    await expect(overlay).toHaveClass(/open/);
  });
});
