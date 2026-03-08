// E2E tests for HUD connection status, stats display, and keyboard hints (bd-9cpbc.6).
// Tests status indicator transitions, stats accuracy, keyboard hints bar,
// and disconnect/reconnect scenarios.
//
// Run: npx playwright test tests/hud-status.spec.js
// View report: npx playwright show-report test-results/html-report

import { test, expect } from '@playwright/test';
import { MOCK_GRAPH, MOCK_PING, MOCK_SHOW } from './fixtures.js';

// ---- Helpers ----

async function mockAPI(page, { pingFail = false } = {}) {
  if (pingFail) {
    await page.route('**/api/bd.v1.BeadsService/Ping', route =>
      route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'connection refused' }) }));
  } else {
    await page.route('**/api/bd.v1.BeadsService/Ping', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) }));
  }
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

// =====================================================================
// STATUS INDICATOR
// =====================================================================

test.describe('Status indicator', () => {

  test('shows "connecting..." initially before data loads', async ({ page }) => {
    // Don't set up mocks yet — check initial state
    await page.route('**/api/**', route => {
      // Delay all API responses to capture the initial connecting state
      return new Promise(resolve => setTimeout(() => {
        route.fulfill({ status: 200, contentType: 'application/json', body: '{}' });
        resolve();
      }, 5000));
    });

    await page.goto('/');

    // Status should show connecting text before any API responds
    const status = page.locator('#status');
    await expect(status).toContainText('connecting');
  });

  test('transitions to "connected" class after successful Ping and Graph', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const status = page.locator('#status');
    await expect(status).toHaveClass(/connected/);
    // Should contain node count info
    const text = await status.textContent();
    expect(text).toContain('beads');
  });

  test('shows error state when Graph API fails', async ({ page }) => {
    // Mock everything except Graph which fails
    await page.route('**/api/bd.v1.BeadsService/Ping', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) }));
    await page.route('**/api/bd.v1.BeadsService/Graph', route =>
      route.fulfill({ status: 500, contentType: 'text/plain', body: 'Internal Server Error' }));
    await page.route('**/api/bd.v1.BeadsService/List', route =>
      route.fulfill({ status: 500, contentType: 'text/plain', body: 'Internal Server Error' }));
    await page.route('**/api/events', route =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
    await page.route('**/api/bus/events*', route =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': keepalive\n\n' }));

    await page.goto('/');
    await page.waitForTimeout(5000);

    const status = page.locator('#status');
    await expect(status).toHaveClass(/error/);
  });

  test('status text includes node and link counts', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const status = page.locator('#status');
    const text = await status.textContent();
    // Format: "graph api · N beads · M links"
    expect(text).toMatch(/\d+\s*beads/);
    expect(text).toMatch(/\d+\s*links/);
  });
});

// =====================================================================
// STATS DISPLAY
// =====================================================================

test.describe('Stats display', () => {

  test('stats element shows node counts', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const stats = page.locator('#stats');
    await expect(stats).toBeVisible();
    const text = await stats.textContent();

    // Should show open, active, and shown counts
    expect(text).toContain('open');
    expect(text).toContain('active');
    expect(text).toContain('shown');
  });

  test('stats values reflect MOCK_GRAPH data', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const stats = page.locator('#stats');
    const text = await stats.textContent();

    // MOCK_GRAPH: total_open=8, total_in_progress=3, total_blocked=3
    expect(text).toContain('8');  // open
    expect(text).toContain('3');  // active
  });

  test('stats and project pulse show matching open/active counts', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Extract open count from #stats
    const statsText = await page.locator('#stats').textContent();

    // Extract open count from project pulse
    const pulseValues = await page.locator('#hud-project-pulse .pulse-stat-value').allTextContents();

    // Both should report the same open count
    const pulseOpen = pulseValues[0]; // first metric is 'open'
    expect(statsText).toContain(pulseOpen);
  });
});

// =====================================================================
// KEYBOARD HINTS
// =====================================================================

test.describe('Keyboard hints', () => {

  test('keyhints bar is visible in bottom-right section', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const hints = page.locator('#keyhints');
    await expect(hints).toBeVisible();
  });

  test('shows all documented keyboard shortcuts', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const hints = page.locator('#keyhints');
    const text = await hints.textContent();

    // Check for all shortcuts mentioned in the HTML
    expect(text).toContain('search');
    expect(text).toContain('fly');
    expect(text).toContain('cycle');
    expect(text).toContain('close');
    expect(text).toContain('refresh');
    expect(text).toContain('bloom');
    expect(text).toContain('labels');
    expect(text).toContain('minimap');
    expect(text).toContain('sidebar');
    expect(text).toContain('controls');
    expect(text).toContain('layout');
    expect(text).toContain('agents');
  });

  test('keyhints are inside the bottom-hud-right section', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Verify parent relationship
    const parent = page.locator('#bottom-hud-right #keyhints');
    await expect(parent).toBeVisible();
  });
});

// =====================================================================
// DISCONNECT/RECONNECT (STRESS)
// =====================================================================

test.describe('Connection resilience', () => {

  test('recovers from initial error to connected state on retry', async ({ page }) => {
    let callCount = 0;

    // First Graph call fails, second succeeds
    await page.route('**/api/bd.v1.BeadsService/Ping', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) }));
    await page.route('**/api/bd.v1.BeadsService/Graph', route => {
      callCount++;
      if (callCount <= 1) {
        return route.fulfill({ status: 500, contentType: 'text/plain', body: 'Server Error' });
      }
      return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_GRAPH) });
    });
    await page.route('**/api/bd.v1.BeadsService/List', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
    await page.route('**/api/bd.v1.BeadsService/Stats', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_GRAPH.stats) }));
    await page.route('**/api/bd.v1.BeadsService/Blocked', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
    await page.route('**/api/bd.v1.BeadsService/Ready', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
    await page.route('**/api/bd.v1.BeadsService/Show', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_SHOW) }));
    await page.route('**/api/events', route =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
    await page.route('**/api/bus/events*', route =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: ': keepalive\n\n' }));

    await page.goto('/');

    // Initially may show error
    await page.waitForTimeout(3000);

    // Trigger refresh (which retries the Graph call)
    await page.keyboard.press('r');
    await page.waitForTimeout(5000);

    // After retry, should be connected
    const status = page.locator('#status');
    await expect(status).toHaveClass(/connected/);
  });
});
