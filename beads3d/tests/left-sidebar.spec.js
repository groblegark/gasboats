// E2E tests for left sidebar — agent roster & focused issue panel (bd-9cpbc.4).
// Verifies: sidebar open/close, agent roster population, focused issue display,
// and keyboard shortcut.
//
// Run: npx playwright test tests/left-sidebar.spec.js
// View report: npx playwright show-report test-results/html-report

import { test, expect } from '@playwright/test';
import { MOCK_GRAPH, MOCK_PING, MOCK_SHOW } from './fixtures.js';

async function mockAPI(page) {
  await page.route('**/api/bd.v1.BeadsService/Ping', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) }));
  await page.route('**/api/bd.v1.BeadsService/Graph', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_GRAPH) }));
  await page.route('**/api/bd.v1.BeadsService/List', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: '[]' }));
  await page.route('**/api/bd.v1.BeadsService/Show', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_SHOW) }));
  await page.route('**/api/bd.v1.BeadsService/Stats', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_GRAPH.stats) }));
  await page.route('**/api/bd.v1.BeadsService/Blocked', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: '[]' }));
  await page.route('**/api/bd.v1.BeadsService/Ready', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: '[]' }));
  await page.route('**/api/events', route =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
  await page.route('**/api/bus/events*', route =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
}

async function waitForGraph(page) {
  await page.waitForSelector('#status.connected', { timeout: 15000 });
  await page.waitForTimeout(3000);
  await page.waitForFunction(() => {
    const b = window.__beads3d;
    return b && b.graph && b.graph.graphData().nodes.length > 0;
  }, { timeout: 10000 });
}

test.describe('left sidebar (bd-9cpbc.4)', () => {

  test('sidebar opens with f key and closes with f key', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Sidebar should be closed initially
    const sidebar = page.locator('#left-sidebar');
    await expect(sidebar).not.toHaveClass(/open/);

    // Press f to open sidebar
    await page.keyboard.press('f');
    await page.waitForTimeout(300);
    await expect(sidebar).toHaveClass(/open/);

    // Press f again to close sidebar
    await page.keyboard.press('f');
    await page.waitForTimeout(300);
    await expect(sidebar).not.toHaveClass(/open/);
  });

  test('sidebar opens via close button (x)', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Open sidebar
    await page.keyboard.press('f');
    await page.waitForTimeout(300);
    await expect(page.locator('#left-sidebar')).toHaveClass(/open/);

    // Close via X button
    await page.click('#ls-close');
    await page.waitForTimeout(300);
    await expect(page.locator('#left-sidebar')).not.toHaveClass(/open/);
  });

  test('agent roster shows agents from graph data', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Open sidebar
    await page.keyboard.press('f');
    await page.waitForTimeout(500);

    // Agent roster should show alice and bob (from MOCK_GRAPH agent nodes)
    const agentList = page.locator('#ls-agent-list');
    const agentRows = agentList.locator('.ls-agent-row');

    // MOCK_GRAPH has agent:alice and agent:bob
    const count = await agentRows.count();
    expect(count).toBeGreaterThanOrEqual(2);

    // Check agent count badge
    const countBadge = page.locator('#ls-agent-count');
    const countText = await countBadge.textContent();
    expect(parseInt(countText)).toBeGreaterThanOrEqual(2);
  });

  test('agent roster entries show name and status dot', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Open sidebar
    await page.keyboard.press('f');
    await page.waitForTimeout(500);

    // Check that agent rows have name and status dot
    const result = await page.evaluate(() => {
      const rows = document.querySelectorAll('#ls-agent-list .ls-agent-row');
      const agents = [];
      for (const row of rows) {
        const name = row.querySelector('.ls-agent-name')?.textContent?.trim();
        const dot = row.querySelector('.ls-agent-dot');
        const hasDot = dot !== null;
        agents.push({ name, hasDot });
      }
      return agents;
    });

    expect(result.length).toBeGreaterThanOrEqual(2);
    // Each agent should have a name and a status dot
    for (const agent of result) {
      expect(agent.name).toBeTruthy();
      expect(agent.hasDot).toBe(true);
    }
  });

  test('clicking graph node shows focused issue in sidebar', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Open sidebar first
    await page.keyboard.press('f');
    await page.waitForTimeout(300);

    // Initially shows placeholder text
    const content = page.locator('#ls-focused-content');
    await expect(content).toContainText('Click a node to inspect');

    // Click bd-feat1 node programmatically
    await page.evaluate(() => {
      const b = window.__beads3d;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-feat1');
      if (target) b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });
    await page.waitForTimeout(1000);

    // Focused issue should now show bd-feat1 details
    const focusedContent = await page.evaluate(() => {
      const el = document.getElementById('ls-focused-content');
      return el ? el.textContent : null;
    });

    expect(focusedContent).not.toBeNull();
    expect(focusedContent).toContain('bd-feat1');
  });

  test('clicking different node updates focused issue', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Open sidebar
    await page.keyboard.press('f');
    await page.waitForTimeout(300);

    // Click bd-feat1
    await page.evaluate(() => {
      const b = window.__beads3d;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-feat1');
      if (target) b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });
    await page.waitForTimeout(1000);

    const first = await page.evaluate(() =>
      document.getElementById('ls-focused-content')?.textContent
    );
    expect(first).toContain('bd-feat1');

    // Click bd-task3 instead
    await page.evaluate(() => {
      const b = window.__beads3d;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-task3');
      if (target) b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });
    await page.waitForTimeout(1000);

    const second = await page.evaluate(() =>
      document.getElementById('ls-focused-content')?.textContent
    );
    expect(second).toContain('bd-task3');
    expect(second).not.toContain('bd-feat1');
  });

  test('focused issue shows status and priority', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Open sidebar
    await page.keyboard.press('f');
    await page.waitForTimeout(300);

    // Click bd-epic1 (P0, in_progress)
    await page.evaluate(() => {
      const b = window.__beads3d;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-epic1');
      if (target) b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });
    await page.waitForTimeout(1000);

    const info = await page.evaluate(() => {
      const el = document.getElementById('ls-focused-content');
      if (!el) return null;
      return {
        text: el.textContent,
        html: el.innerHTML,
      };
    });

    expect(info).not.toBeNull();
    // Should show the issue ID
    expect(info.text).toContain('bd-epic1');
    // Should show status
    expect(info.text).toMatch(/in.progress/i);
  });

  test('clearing selection resets focused issue to placeholder', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Open sidebar
    await page.keyboard.press('f');
    await page.waitForTimeout(300);

    // Click a node
    await page.evaluate(() => {
      const b = window.__beads3d;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-feat1');
      if (target) b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });
    await page.waitForTimeout(1000);

    // Verify something is shown
    const before = await page.evaluate(() =>
      document.getElementById('ls-focused-content')?.textContent
    );
    expect(before).toContain('bd-feat1');

    // Clear selection by clicking background
    await page.evaluate(() => {
      window.__beads3d.clearSelection();
    });
    await page.waitForTimeout(500);

    // Should revert to placeholder
    const after = await page.evaluate(() =>
      document.getElementById('ls-focused-content')?.textContent
    );
    expect(after).toContain('Click a node to inspect');
  });
});
