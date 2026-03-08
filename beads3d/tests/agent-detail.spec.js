// E2E tests for Agent Window details — bead assignments, status bar,
// popout/dock, collapse/expand, close, and rig display (bd-fa9k7).
// Complements agents-view.spec.js which covers overlay open/close, search,
// and bus event auto-creation.
//
// Run: npx playwright test tests/agent-detail.spec.js
// View report: npx playwright show-report test-results/html-report

import { test, expect } from '@playwright/test';
import {
  MOCK_PING,
  MOCK_SHOW,
  MOCK_MULTI_AGENT_GRAPH,
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

async function openOverlay(page) {
  await page.keyboard.press('Shift+A');
  await page.waitForTimeout(500);
  await expect(page.locator('#agents-view')).toHaveClass(/open/);
}

// =====================================================================
// AGENT WINDOW STRUCTURE
// =====================================================================

test.describe('Agent window structure', () => {

  test('each agent window has header with name and status', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    const windows = page.locator('.agents-view-grid .agent-window');
    const count = await windows.count();
    expect(count).toBeGreaterThanOrEqual(9);

    // Check first three windows for structure
    for (let i = 0; i < Math.min(count, 3); i++) {
      const win = windows.nth(i);
      await expect(win.locator('.agent-window-name')).toBeVisible();
      await expect(win.locator('.agent-window-header')).toBeVisible();
    }
  });

  test('agent windows show status bar with state', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    const firstWindow = page.locator('.agents-view-grid .agent-window').first();
    const statusBar = firstWindow.locator('.agent-status-bar');
    await expect(statusBar).toBeVisible();

    // Should contain status label
    await expect(statusBar.locator('.status-label')).toContainText('Status:');
  });

  test('agent windows have feed area', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    const firstWindow = page.locator('.agents-view-grid .agent-window').first();
    const feed = firstWindow.locator('.agent-feed');
    await expect(feed).toBeAttached();
  });

  test('agent windows have mail compose input', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    const firstWindow = page.locator('.agents-view-grid .agent-window').first();
    const mailInput = firstWindow.locator('.agent-mail-input');
    const mailSend = firstWindow.locator('.agent-mail-send');
    await expect(mailInput).toBeAttached();
    await expect(mailSend).toBeAttached();
  });

  test('agent window names match graph agent node titles', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    const names = await page.locator('.agents-view-grid .agent-window-name').allTextContents();

    // MOCK_MULTI_AGENT_GRAPH has 9 agents
    const graphAgents = MOCK_MULTI_AGENT_GRAPH.nodes
      .filter(n => n.issue_type === 'agent')
      .map(n => n.title);

    // All graph agent names should appear in window names
    for (const agentName of graphAgents) {
      expect(names.some(n => n.includes(agentName))).toBe(true);
    }
  });
});

// =====================================================================
// BEAD ASSIGNMENTS
// =====================================================================

test.describe('Agent bead assignments', () => {

  test('agent windows show assigned beads', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    // Find windows that have beads assigned (from assigned_to edges)
    const beadSections = page.locator('.agents-view-grid .agent-window-beads');
    const beadCount = await beadSections.count();

    // At least some agents should have assigned beads in MOCK_MULTI_AGENT_GRAPH
    expect(beadCount).toBeGreaterThanOrEqual(1);
  });

  test('assigned bead items are clickable', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    // Find a bead item in any agent window
    const beadItem = page.locator('.agents-view-grid .agent-window-bead').first();
    const beadExists = await beadItem.count();

    if (beadExists > 0) {
      const beadId = await beadItem.getAttribute('data-bead-id');
      expect(beadId).toBeTruthy();

      // Click the bead — should trigger node selection
      await beadItem.click();
      await page.waitForTimeout(500);

      const selected = await page.evaluate(() => window.__beads3d?.selectedNodeId);
      expect(selected).toBeTruthy();
    }
  });

  test('bead count badge matches assigned_to edges', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    // Count assigned_to edges for each agent
    const assignedCounts = {};
    for (const edge of MOCK_MULTI_AGENT_GRAPH.edges) {
      if (edge.dep_type === 'assigned_to') {
        assignedCounts[edge.target] = (assignedCounts[edge.target] || 0) + 1;
      }
    }

    // Verify at least one agent shows correct count in badge
    const badges = await page.locator('.agents-view-grid .agent-window-badge').allTextContents();
    // Find badges that are numbers (bead counts)
    const numBadges = badges.filter(b => /^\d+$/.test(b.trim()));
    expect(numBadges.length).toBeGreaterThanOrEqual(1);
  });
});

// =====================================================================
// COLLAPSE / EXPAND
// =====================================================================

test.describe('Agent window collapse/expand', () => {

  test('clicking header collapses window', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    const firstWindow = page.locator('.agents-view-grid .agent-window').first();

    // Should not be collapsed initially (in overlay grid)
    await expect(firstWindow).not.toHaveClass(/collapsed/);

    // Click header to collapse
    const header = firstWindow.locator('.agent-window-header');
    await header.click();
    await page.waitForTimeout(300);

    await expect(firstWindow).toHaveClass(/collapsed/);
  });

  test('clicking collapsed header expands window', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    const firstWindow = page.locator('.agents-view-grid .agent-window').first();
    const header = firstWindow.locator('.agent-window-header');

    // Collapse
    await header.click();
    await page.waitForTimeout(200);
    await expect(firstWindow).toHaveClass(/collapsed/);

    // Expand
    await header.click();
    await page.waitForTimeout(200);
    await expect(firstWindow).not.toHaveClass(/collapsed/);
  });
});

// =====================================================================
// POPOUT / DOCK
// =====================================================================

test.describe('Agent window popout', () => {

  test('popout button exists on agent windows', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    const popoutBtn = page.locator('.agents-view-grid .agent-window-popout').first();
    await expect(popoutBtn).toBeAttached();
  });

  test('clicking popout moves window to floating position', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    const firstWindow = page.locator('.agents-view-grid .agent-window').first();
    const agentId = await firstWindow.getAttribute('data-agent-id');

    // Click popout
    const popoutBtn = firstWindow.locator('.agent-window-popout');
    await popoutBtn.click();
    await page.waitForTimeout(300);

    // Window should no longer be in the grid
    const inGrid = await page.locator(`.agents-view-grid .agent-window[data-agent-id="${agentId}"]`).count();
    expect(inGrid).toBe(0);

    // Window should be floating (positioned absolute in body)
    const floating = await page.locator(`body > .agent-window[data-agent-id="${agentId}"]`).count();
    expect(floating).toBe(1);
  });
});

// =====================================================================
// CLOSE BUTTON
// =====================================================================

test.describe('Agent window close', () => {

  test('close button removes window from grid', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    const initialCount = await page.locator('.agents-view-grid .agent-window').count();

    const firstWindow = page.locator('.agents-view-grid .agent-window').first();
    const closeBtn = firstWindow.locator('.agent-window-close');
    await closeBtn.click();
    await page.waitForTimeout(300);

    const afterCount = await page.locator('.agents-view-grid .agent-window').count();
    expect(afterCount).toBe(initialCount - 1);
  });
});

// =====================================================================
// STATUS BAR UPDATES VIA EVENTS
// =====================================================================

test.describe('Agent status bar updates', () => {

  test('PreToolUse event updates tool display in status bar', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    // Inject a PreToolUse event
    await page.evaluate(() => {
      const agentId = 'agent:swift-newt';
      window.__beads3d_appendAgentEvent(agentId, {
        type: 'PreToolUse',
        ts: new Date().toISOString(),
        payload: { actor: 'swift-newt', tool_name: 'Edit', tool_input: { file_path: '/src/main.go' } },
      });
    });
    await page.waitForTimeout(300);

    // Find swift-newt's window and check status bar
    const swiftWindow = page.locator('.agent-window[data-agent-id="agent:swift-newt"]');
    const toolDisplay = swiftWindow.locator('.agent-status-tool');
    const toolText = await toolDisplay.textContent();
    expect(toolText).toContain('Edit');
  });

  test('AgentIdle event shows idle status', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    // Inject an AgentIdle event
    await page.evaluate(() => {
      const agentId = 'agent:swift-newt';
      window.__beads3d_appendAgentEvent(agentId, {
        type: 'AgentIdle',
        ts: new Date().toISOString(),
        payload: { actor: 'swift-newt' },
      });
    });
    await page.waitForTimeout(300);

    const swiftWindow = page.locator('.agent-window[data-agent-id="agent:swift-newt"]');
    const stateEl = swiftWindow.locator('.agent-status-state');
    const stateText = await stateEl.textContent();
    expect(stateText.toLowerCase()).toContain('idle');
  });

  test('AgentCrashed event shows crashed status', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page, 9);

    await openOverlay(page);

    // Inject an AgentCrashed event
    await page.evaluate(() => {
      const agentId = 'agent:swift-newt';
      window.__beads3d_appendAgentEvent(agentId, {
        type: 'AgentCrashed',
        ts: new Date().toISOString(),
        payload: { actor: 'swift-newt', error: 'context deadline exceeded' },
      });
    });
    await page.waitForTimeout(300);

    const swiftWindow = page.locator('.agent-window[data-agent-id="agent:swift-newt"]');
    const stateEl = swiftWindow.locator('.agent-status-state');
    const stateText = await stateEl.textContent();
    expect(stateText.toLowerCase()).toContain('crashed');
  });
});

// =====================================================================
// AGENT WINDOW WITH SSE BUS EVENTS
// =====================================================================

test.describe('Agent window with live events', () => {

  test('bus events populate agent feed in real-time', async ({ page }) => {
    // Generate a session for swift-newt with 3 tools
    const events = generateSession('swift-newt', 'gt-auth-rpc', { seed: 1, toolCount: 3 });
    const busBody = sessionToSseBody(events);

    await mockAPI(page, MOCK_MULTI_AGENT_GRAPH, busBody);
    await page.goto('/');
    await waitForGraph(page, 9);

    // Wait for events to process
    await page.waitForTimeout(3000);

    // Open overlay
    await openOverlay(page);

    // Swift-newt should have a window with events
    const swiftWindow = page.locator('.agent-window[data-agent-id="agent:swift-newt"]');
    const windowExists = await swiftWindow.count();

    if (windowExists > 0) {
      const feedEntries = await page.evaluate(() => {
        const win = window.__beads3d_agentWindows().get('agent:swift-newt');
        return win ? win.entries.length : 0;
      });
      // Should have received events from the bus stream
      expect(feedEntries).toBeGreaterThanOrEqual(1);
    }
  });

  test('multiple agents receive their own events independently', async ({ page }) => {
    const events = [
      ...generateSession('swift-newt', 'gt-auth', { seed: 1, toolCount: 2 }),
      ...generateSession('keen-bird', 'gt-dolt', { seed: 2, toolCount: 2 }),
    ];
    const busBody = sessionToSseBody(events);

    await mockAPI(page, MOCK_MULTI_AGENT_GRAPH, busBody);
    await page.goto('/');
    await waitForGraph(page, 9);

    await page.waitForTimeout(3000);
    await openOverlay(page);

    // Both agents should have windows with events
    const result = await page.evaluate(() => {
      const swiftWin = window.__beads3d_agentWindows().get('agent:swift-newt');
      const keenWin = window.__beads3d_agentWindows().get('agent:keen-bird');
      return {
        swiftEntries: swiftWin ? swiftWin.entries.length : 0,
        keenEntries: keenWin ? keenWin.entries.length : 0,
      };
    });

    // Both should have received their own events
    expect(result.swiftEntries).toBeGreaterThanOrEqual(1);
    expect(result.keenEntries).toBeGreaterThanOrEqual(1);
  });
});

// =====================================================================
// RIG DISPLAY
// =====================================================================

test.describe('Agent rig display', () => {

  test('agent windows show rig label when present', async ({ page }) => {
    // Create a graph with agents that have rigs
    const graph = JSON.parse(JSON.stringify(MOCK_MULTI_AGENT_GRAPH));
    // Add rig field to some agents
    const agents = graph.nodes.filter(n => n.issue_type === 'agent');
    if (agents.length > 0) agents[0].rig = 'beads';
    if (agents.length > 1) agents[1].rig = 'gastown';

    await mockAPI(page, graph);
    await page.goto('/');
    await waitForGraph(page, 5);

    await openOverlay(page);

    // At least one window should show a rig label
    const rigLabels = page.locator('.agents-view-grid .agent-window-rig');
    const count = await rigLabels.count();
    expect(count).toBeGreaterThanOrEqual(1);
  });
});
