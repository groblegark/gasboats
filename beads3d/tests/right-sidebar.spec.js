// E2E tests for Right sidebar — epic progress, dep health, decision queue (bd-9cpbc.5).
// Tests all three sidebar sections with mocked graph data and live SSE events.
//
// Run: npx playwright test tests/right-sidebar.spec.js
// View report: npx playwright show-report test-results/html-report

import { test, expect } from '@playwright/test';
import { MOCK_GRAPH, MOCK_PING, MOCK_SHOW } from './fixtures.js';

// Build a graph variant with decision/gate nodes for decision queue tests
function graphWithDecisions() {
  const graph = JSON.parse(JSON.stringify(MOCK_GRAPH));
  graph.nodes.push(
    { id: 'bd-dec1', title: 'Deploy to prod?', status: 'open', priority: 2, issue_type: 'decision', assignee: '', created_at: '2026-02-19T12:00:00Z', updated_at: '2026-02-19T12:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 0, fy: 120, fz: 0 },
    { id: 'bd-gate1', title: 'Approve release v3', status: 'open', priority: 1, issue_type: 'gate', assignee: '', created_at: '2026-02-19T13:00:00Z', updated_at: '2026-02-19T13:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 60, fy: 120, fz: 0 },
    { id: 'bd-dec2', title: 'Resolved question', status: 'closed', priority: 3, issue_type: 'decision', assignee: '', created_at: '2026-02-18T10:00:00Z', updated_at: '2026-02-19T10:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: -60, fy: 120, fz: 0 },
  );
  return graph;
}

// ---- Helpers ----

async function mockAPI(page, graph = MOCK_GRAPH) {
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

  await page.route('**/api/events', route =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
  await page.route('**/api/bus/events*', route =>
    route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      headers: { 'Cache-Control': 'no-cache', 'Connection': 'keep-alive' },
      body: ': keepalive\n\n',
    }));
}

async function waitForGraph(page) {
  await page.waitForSelector('#status.connected', { timeout: 15000 });
  await page.waitForTimeout(2000);
  await page.waitForFunction(() => {
    const b = window.__beads3d;
    return b && b.graph && b.graph.graphData().nodes.length > 0;
  }, { timeout: 10000 });
}

// =====================================================================
// SIDEBAR VISIBILITY AND TOGGLE
// =====================================================================

test.describe('Right sidebar visibility', () => {

  test('sidebar is visible on page load', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const sidebar = page.locator('#right-sidebar');
    await expect(sidebar).toBeVisible();
  });

  test('collapse button toggles sidebar collapsed state', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const sidebar = page.locator('#right-sidebar');
    const btn = page.locator('#rs-collapse');

    // Collapse
    await btn.click();
    await page.waitForTimeout(300);
    await expect(sidebar).toHaveClass(/collapsed/);

    // Sections should be hidden when collapsed
    await expect(page.locator('#rs-epics .rs-section-body')).not.toBeVisible();

    // Expand
    await btn.click();
    await page.waitForTimeout(300);
    await expect(sidebar).not.toHaveClass(/collapsed/);
  });

  test('all three sections are present', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await expect(page.locator('#rs-epics')).toBeAttached();
    await expect(page.locator('#rs-decisions')).toBeAttached();
    await expect(page.locator('#rs-health')).toBeAttached();
  });

  test('section headers are clickable to collapse individual sections', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const epicSection = page.locator('#rs-epics');
    const epicHeader = epicSection.locator('.rs-section-header');

    // Click header to collapse
    await epicHeader.click();
    await page.waitForTimeout(200);
    await expect(epicSection).toHaveClass(/collapsed/);

    // Click again to expand
    await epicHeader.click();
    await page.waitForTimeout(200);
    await expect(epicSection).not.toHaveClass(/collapsed/);
  });
});

// =====================================================================
// EPIC PROGRESS
// =====================================================================

test.describe('Epic Progress section', () => {

  test('shows epic nodes from MOCK_GRAPH with completion bars', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const body = page.locator('#rs-epics-body');
    await expect(body).toBeVisible();

    // MOCK_GRAPH has 2 epics: bd-epic1 and bd-epic2
    const epicItems = body.locator('.rs-epic-item');
    await expect(epicItems).toHaveCount(2);

    // Each should have a name and progress bar
    const firstEpic = epicItems.first();
    await expect(firstEpic.locator('.rs-epic-name')).toBeVisible();
    await expect(firstEpic.locator('.rs-epic-bar')).toBeVisible();
    await expect(firstEpic.locator('.rs-epic-pct')).toBeVisible();
  });

  test('epic child counts are correct', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const body = page.locator('#rs-epics-body');

    // MOCK_GRAPH bd-epic1 has 3 children: feat1(in_progress), feat2(open, blocked), task2(open)
    // closed: 0, total: 3, pct: 0%
    // MOCK_GRAPH bd-epic2 has 2 children: task3(in_progress), task4(open, blocked)
    // closed: 0, total: 2, pct: 0%

    const pcts = body.locator('.rs-epic-pct');
    const allPcts = await pcts.allTextContents();
    // Both should be 0% since no children are closed
    expect(allPcts.every(p => p === '0%')).toBe(true);
  });

  test('epic item shows correct title text', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const body = page.locator('#rs-epics-body');
    const names = body.locator('.rs-epic-name');
    const allNames = await names.allTextContents();

    // Should contain epic titles
    expect(allNames.some(n => n.includes('Platform Overhaul'))).toBe(true);
    expect(allNames.some(n => n.includes('Observability Stack'))).toBe(true);
  });

  test('clicking epic item flies to epic node', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const epicItem = page.locator('#rs-epics-body .rs-epic-item').first();
    await epicItem.click();
    await page.waitForTimeout(500);

    // Should select the epic node — check if detail panel shows or node is selected
    // The handleNodeClick typically updates selectedNodeId
    const selected = await page.evaluate(() => window.__beads3d?.selectedNodeId);
    expect(selected).toBeTruthy();
  });
});

// =====================================================================
// DEP HEALTH
// =====================================================================

test.describe('Dep Health section', () => {

  test('shows blocked issues from MOCK_GRAPH', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const body = page.locator('#rs-health-body');
    await expect(body).toBeVisible();

    // MOCK_GRAPH has 3 blocked items: bd-task1, bd-feat2, bd-task4
    const blockedItems = body.locator('.rs-blocked-item');
    await expect(blockedItems).toHaveCount(3);
  });

  test('blocked count header shows correct number', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const body = page.locator('#rs-health-body');
    // Header shows "N blocked"
    await expect(body).toContainText('3 blocked');
  });

  test('blocked item titles match graph data', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const items = page.locator('#rs-health-body .rs-blocked-item');
    const allTexts = await items.allTextContents();

    // Should contain titles of blocked nodes
    expect(allTexts.some(t => t.includes('OAuth integration tests'))).toBe(true);
    expect(allTexts.some(t => t.includes('API rate limiting'))).toBe(true);
    expect(allTexts.some(t => t.includes('Metrics dashboard Helm chart'))).toBe(true);
  });

  test('clicking blocked item navigates to that node', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const item = page.locator('#rs-health-body .rs-blocked-item').first();
    await item.click();
    await page.waitForTimeout(500);

    const selected = await page.evaluate(() => window.__beads3d?.selectedNodeId);
    expect(selected).toBeTruthy();
  });

  test('shows "no blocked items" when graph has none', async ({ page }) => {
    // Create a graph with no blocked items
    const unblockedGraph = JSON.parse(JSON.stringify(MOCK_GRAPH));
    unblockedGraph.nodes.forEach(n => { n.blocked_by = []; });

    await mockAPI(page, unblockedGraph);
    await page.goto('/');
    await waitForGraph(page);

    const body = page.locator('#rs-health-body');
    await expect(body).toContainText('no blocked items');
  });
});

// =====================================================================
// DECISION QUEUE
// =====================================================================

test.describe('Decision Queue section', () => {

  test('shows "no pending decisions" when graph has no decision nodes', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const body = page.locator('#rs-decisions-body');
    await expect(body).toContainText('no pending decisions');
  });

  test('shows pending decisions from graph (gate and decision types)', async ({ page }) => {
    const graph = graphWithDecisions();
    await mockAPI(page, graph);
    await page.goto('/');
    await waitForGraph(page);

    const body = page.locator('#rs-decisions-body');

    // Should show 2 pending decisions (bd-dec1 open, bd-gate1 open; bd-dec2 is closed)
    const items = body.locator('.rs-decision-item');
    await expect(items).toHaveCount(2);

    // Check prompts
    const prompts = body.locator('.rs-decision-prompt');
    const allPrompts = await prompts.allTextContents();
    expect(allPrompts.some(p => p.includes('Deploy to prod?'))).toBe(true);
    expect(allPrompts.some(p => p.includes('Approve release v3'))).toBe(true);
  });

  test('clicking decision item navigates to decision node', async ({ page }) => {
    const graph = graphWithDecisions();
    await mockAPI(page, graph);
    await page.goto('/');
    await waitForGraph(page);

    const item = page.locator('#rs-decisions-body .rs-decision-item').first();
    await item.click();
    await page.waitForTimeout(500);

    const selected = await page.evaluate(() => window.__beads3d?.selectedNodeId);
    expect(selected).toBeTruthy();
  });

  test('closed decision nodes are excluded from queue', async ({ page }) => {
    const graph = graphWithDecisions();
    await mockAPI(page, graph);
    await page.goto('/');
    await waitForGraph(page);

    // bd-dec2 is closed — should not appear
    const prompts = page.locator('#rs-decisions-body .rs-decision-prompt');
    const allPrompts = await prompts.allTextContents();
    expect(allPrompts.every(p => !p.includes('Resolved question'))).toBe(true);
  });
});
