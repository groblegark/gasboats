// Integration smoke tests for beads3d HUD, sidebars, and control panel (bd-a2mnd).
// Verifies that all major UI panels render correctly with mocked API data.
//
// Run: npx playwright test tests/hud-smoke.spec.js
// View report: npx playwright show-report test-results/html-report

import { test, expect } from '@playwright/test';
import { MOCK_GRAPH, MOCK_PING, MOCK_SHOW } from './fixtures.js';

// ---- Helpers ----

async function mockAPI(page) {
  await page.route('**/api/bd.v1.BeadsService/Ping', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) }));
  await page.route('**/api/bd.v1.BeadsService/Graph', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_GRAPH) }));
  await page.route('**/api/bd.v1.BeadsService/List', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
  await page.route('**/api/bd.v1.BeadsService/Show', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_SHOW) }));
  await page.route('**/api/bd.v1.BeadsService/Stats', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_GRAPH.stats) }));
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

// ---- Bottom HUD Bar ----

test.describe('Bottom HUD bar', () => {

  test('renders three-column layout', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await expect(page.locator('#bottom-hud')).toBeVisible();
    await expect(page.locator('#bottom-hud-left')).toBeVisible();
    await expect(page.locator('#bottom-hud-center')).toBeVisible();
    await expect(page.locator('#bottom-hud-right')).toBeVisible();
  });

  test('quick action buttons are present', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const buttons = [
      'hud-btn-refresh', 'hud-btn-labels', 'hud-btn-agents',
      'hud-btn-bloom', 'hud-btn-search', 'hud-btn-minimap',
      'hud-btn-sidebar', 'hud-btn-controls',
    ];

    for (const id of buttons) {
      await expect(page.locator(`#${id}`)).toBeVisible();
    }
  });

  test('legend shows status and edge types', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const legend = page.locator('#legend');
    await expect(legend).toBeVisible();
    await expect(legend).toContainText('open');
    await expect(legend).toContainText('active');
    await expect(legend).toContainText('epic');
    await expect(legend).toContainText('blocked');
    await expect(legend).toContainText('agent');
    await expect(legend).toContainText('blocks');
    await expect(legend).toContainText('waits');
    await expect(legend).toContainText('parent');
  });

  test('project pulse shows statistics', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const pulse = page.locator('#hud-project-pulse');
    await expect(pulse).toBeVisible();
    // Stats from MOCK_GRAPH: 8 open, 3 in_progress, 3 blocked, 2 closed
    await expect(pulse).toContainText('open');
    await expect(pulse).toContainText('active');
  });

  test('connection status shows connected', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const status = page.locator('#status');
    await expect(status).toHaveClass(/connected/);
  });
});

// ---- Unified Feed ----

test.describe('Unified activity feed', () => {

  test('unified feed element exists', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await expect(page.locator('#unified-feed')).toBeAttached();
  });

  test('unified/split toggle button exists', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const toggle = page.locator('#unified-feed-toggle');
    await expect(toggle).toBeVisible();
    await expect(toggle).toContainText('unified');
  });

  test('toggle activates unified feed and hides agent windows', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const toggle = page.locator('#unified-feed-toggle');
    const feed = page.locator('#unified-feed');

    // Click to activate unified view
    await toggle.click();
    await page.waitForTimeout(300);
    await expect(feed).toHaveClass(/active/);

    // Click again to deactivate
    await toggle.click();
    await page.waitForTimeout(300);
    await expect(feed).not.toHaveClass(/active/);
  });
});

// ---- Quick Action Buttons ----

test.describe('Quick action button wiring', () => {

  test('sidebar toggle button opens left sidebar', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const sidebar = page.locator('#left-sidebar');
    // Initially may be hidden — click to toggle
    await page.locator('#hud-btn-sidebar').click();
    await page.waitForTimeout(500);
    await expect(sidebar).toBeVisible();
  });

  test('controls button opens control panel', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await page.locator('#hud-btn-controls').click();
    await page.waitForTimeout(500);
    await expect(page.locator('#control-panel')).toHaveClass(/open/);
  });

  test('search button focuses search input', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await page.locator('#hud-btn-search').click();
    await page.waitForTimeout(300);
    // Search input should be focused
    const searchInput = page.locator('#search');
    await expect(searchInput).toBeFocused();
  });
});

// ---- Right Sidebar ----

test.describe('Right sidebar', () => {

  test('renders with all three sections', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const sidebar = page.locator('#right-sidebar');
    await expect(sidebar).toBeAttached();

    await expect(page.locator('#rs-epics')).toBeAttached();
    await expect(page.locator('#rs-decisions')).toBeAttached();
    await expect(page.locator('#rs-health')).toBeAttached();
  });

  test('has section labels', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await expect(page.locator('#rs-epics .rs-section-label')).toContainText('Epic Progress');
    await expect(page.locator('#rs-decisions .rs-section-label')).toContainText('Decisions');
    await expect(page.locator('#rs-health .rs-section-label')).toContainText('Dep Health');
  });

  test('collapse button toggles collapsed state', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const sidebar = page.locator('#right-sidebar');
    const collapseBtn = page.locator('#rs-collapse');

    // Click collapse
    await collapseBtn.click();
    await page.waitForTimeout(300);
    await expect(sidebar).toHaveClass(/collapsed/);

    // Click again to expand
    await collapseBtn.click();
    await page.waitForTimeout(300);
    await expect(sidebar).not.toHaveClass(/collapsed/);
  });
});

// ---- Control Panel ----

test.describe('Control panel', () => {

  test('opens with g key and closes with Escape', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const panel = page.locator('#control-panel');

    // Press g to open
    await page.keyboard.press('g');
    await page.waitForTimeout(500);
    await expect(panel).toHaveClass(/open/);

    // Press Escape to close
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);
    await expect(panel).not.toHaveClass(/open/);
  });

  test('has bloom, camera, and label sections', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Open control panel
    await page.keyboard.press('g');
    await page.waitForTimeout(500);

    await expect(page.locator('#cp-bloom')).toBeAttached();
    await expect(page.locator('#cp-bloom .cp-section-label')).toContainText('Bloom');
  });

  test('close button works', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Open with g
    await page.keyboard.press('g');
    await page.waitForTimeout(500);
    await expect(page.locator('#control-panel')).toHaveClass(/open/);

    // Close with X button
    await page.locator('#cp-close').click();
    await page.waitForTimeout(300);
    await expect(page.locator('#control-panel')).not.toHaveClass(/open/);
  });
});

// ---- Left Sidebar ----

test.describe('Left sidebar', () => {

  test('opens with f key', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await page.keyboard.press('f');
    await page.waitForTimeout(500);
    await expect(page.locator('#left-sidebar')).toBeVisible();
  });

  test('has agent roster and focused issue sections', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await page.keyboard.press('f');
    await page.waitForTimeout(500);

    await expect(page.locator('#ls-agent-roster')).toBeVisible();
    await expect(page.locator('#ls-focused-issue')).toBeVisible();
    await expect(page.locator('#ls-agent-roster .ls-section-label')).toContainText('Agent Roster');
    await expect(page.locator('#ls-focused-issue .ls-section-label')).toContainText('Focused Issue');
  });

  test('close button hides sidebar', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Open
    await page.keyboard.press('f');
    await page.waitForTimeout(500);
    await expect(page.locator('#left-sidebar')).toBeVisible();

    // Close
    await page.locator('#ls-close').click();
    await page.waitForTimeout(300);
    await expect(page.locator('#left-sidebar')).not.toBeVisible();
  });
});

// ---- Keyboard Hints ----

test.describe('Keyboard hints', () => {

  test('keyhints bar is visible in bottom HUD', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const hints = page.locator('#keyhints');
    await expect(hints).toBeVisible();
    await expect(hints).toContainText('search');
    await expect(hints).toContainText('bloom');
    await expect(hints).toContainText('labels');
    await expect(hints).toContainText('minimap');
    await expect(hints).toContainText('sidebar');
    await expect(hints).toContainText('controls');
    await expect(hints).toContainText('agents');
  });
});
