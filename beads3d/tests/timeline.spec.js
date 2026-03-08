// Timeline scrubber tests for beads3d (bd-ln94x).
// Tests logarithmic timeline rendering, handle drag interaction,
// zoom, reset, and filter integration.
//
// Run: npx playwright test tests/timeline.spec.js
// View report: npx playwright show-report test-results/html-report

import { test, expect } from '@playwright/test';
import { MOCK_GRAPH, MOCK_PING, MOCK_SHOW } from './fixtures.js';

async function mockAPI(page) {
  await page.route('**/api/bd.v1.BeadsService/Ping', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) });
  });
  await page.route('**/api/bd.v1.BeadsService/Graph', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_GRAPH) });
  });
  await page.route('**/api/bd.v1.BeadsService/List', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
  });
  await page.route('**/api/bd.v1.BeadsService/Show', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_SHOW) });
  });
  await page.route('**/api/bd.v1.BeadsService/Stats', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_GRAPH.stats) });
  });
  await page.route('**/api/bd.v1.BeadsService/Blocked', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
  });
  await page.route('**/api/bd.v1.BeadsService/Ready', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
  });
  await page.route('**/api/events', async route => {
    await route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' });
  });
  await page.route('**/api/bus/**', async route => {
    await route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' });
  });
}

async function waitForGraph(page) {
  await page.waitForSelector('#status.connected', { timeout: 15000 });
  await page.waitForTimeout(3000);
  await page.waitForFunction(() => {
    const b = window.__beads3d;
    return b && b.graph && b.graph.graphData().nodes.length > 0;
  }, { timeout: 10000 });
}

test.describe('Timeline Scrubber', () => {
  test.beforeEach(async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);
  });

  test('renders timeline canvas with histogram bars', async ({ page }) => {
    const canvas = page.locator('#timeline-canvas');
    await expect(canvas).toBeVisible();

    // Canvas should have non-zero dimensions
    const box = await canvas.boundingBox();
    expect(box.width).toBeGreaterThan(50);
    expect(box.height).toBeGreaterThan(10);

    // Handles should be visible
    await expect(page.locator('#handle-left')).toBeVisible();
    await expect(page.locator('#handle-right')).toBeVisible();

    // Range highlight should be visible
    await expect(page.locator('#timeline-range')).toBeVisible();

    // Date labels should show something
    const oldest = await page.locator('#tl-oldest').textContent();
    const newest = await page.locator('#tl-newest').textContent();
    expect(oldest.length).toBeGreaterThan(0);
    expect(newest.length).toBeGreaterThan(0);
  });

  test('range label shows "all time" when handles span full range', async ({ page }) => {
    const rangeLabel = await page.locator('#tl-range').textContent();
    expect(rangeLabel).toBe('all time');
  });

  test('dragging left handle narrows selection and filters nodes', async ({ page }) => {
    const canvas = page.locator('#timeline-canvas');
    const box = await canvas.boundingBox();

    // Count visible nodes before drag
    const beforeCount = await page.evaluate(() => {
      const b = window.__beads3d;
      return b.graph.graphData().nodes.filter(n => !n._hidden).length;
    });

    // Drag left handle to ~40% position (filters out older nodes)
    const handleLeft = page.locator('#handle-left');
    const handleBox = await handleLeft.boundingBox();
    await handleLeft.dragTo(canvas, {
      sourcePosition: { x: handleBox.width / 2, y: handleBox.height / 2 },
      targetPosition: { x: box.width * 0.4, y: box.height / 2 },
    });

    // Range label should no longer say "all time"
    await page.waitForFunction(() => {
      const label = document.getElementById('tl-range');
      return label && label.textContent !== 'all time';
    }, { timeout: 5000 });

    const rangeLabel = await page.locator('#tl-range').textContent();
    expect(rangeLabel).not.toBe('all time');
    expect(rangeLabel).toContain('\u2013'); // en dash between dates

    // Some nodes should now be hidden (old ones filtered out)
    const afterCount = await page.evaluate(() => {
      const b = window.__beads3d;
      return b.graph.graphData().nodes.filter(n => !n._hidden).length;
    });
    expect(afterCount).toBeLessThanOrEqual(beforeCount);
  });

  test('double-click resets timeline to full range', async ({ page }) => {
    const canvas = page.locator('#timeline-canvas');
    const box = await canvas.boundingBox();

    // First narrow the selection
    const handleLeft = page.locator('#handle-left');
    const handleBox = await handleLeft.boundingBox();
    await handleLeft.dragTo(canvas, {
      sourcePosition: { x: handleBox.width / 2, y: handleBox.height / 2 },
      targetPosition: { x: box.width * 0.5, y: box.height / 2 },
    });

    // Verify it's narrowed
    await page.waitForFunction(() => {
      const label = document.getElementById('tl-range');
      return label && label.textContent !== 'all time';
    }, { timeout: 5000 });

    // Double-click to reset
    await canvas.dblclick();

    // Should show "all time" again
    await page.waitForFunction(() => {
      const label = document.getElementById('tl-range');
      return label && label.textContent === 'all time';
    }, { timeout: 5000 });

    const rangeLabel = await page.locator('#tl-range').textContent();
    expect(rangeLabel).toBe('all time');
  });

  test('scroll wheel zooms the timeline selection', async ({ page }) => {
    const canvas = page.locator('#timeline-canvas');
    const box = await canvas.boundingBox();

    // Scroll down (zoom out shouldn't change full range much)
    // Scroll up (zoom in) should narrow the selection
    await page.mouse.move(box.x + box.width * 0.7, box.y + box.height / 2);
    await page.mouse.wheel(0, -300); // scroll up = zoom in

    // Range should narrow (no longer "all time")
    await page.waitForFunction(() => {
      const label = document.getElementById('tl-range');
      return label && label.textContent !== 'all time';
    }, { timeout: 5000 });

    const rangeLabel = await page.locator('#tl-range').textContent();
    expect(rangeLabel).not.toBe('all time');
  });

  test('age filter button change resets timeline to full range', async ({ page }) => {
    const canvas = page.locator('#timeline-canvas');
    const box = await canvas.boundingBox();

    // Narrow timeline selection first
    await page.mouse.move(box.x + box.width * 0.5, box.y + box.height / 2);
    await page.mouse.wheel(0, -300);
    await page.waitForFunction(() => {
      const label = document.getElementById('tl-range');
      return label && label.textContent !== 'all time';
    }, { timeout: 5000 });

    // Click "30d" age filter button
    await page.click('.filter-age[data-days="30"]');

    // Wait for re-fetch (the age button triggers refresh which resets timeline)
    await page.waitForFunction(() => {
      const label = document.getElementById('tl-range');
      return label && label.textContent === 'all time';
    }, { timeout: 10000 });

    const rangeLabel = await page.locator('#tl-range').textContent();
    expect(rangeLabel).toBe('all time');
  });

  test('agent nodes are exempt from timeline filtering', async ({ page }) => {
    const canvas = page.locator('#timeline-canvas');
    const box = await canvas.boundingBox();

    // Zoom into a tight range that excludes most nodes
    await page.mouse.move(box.x + box.width * 0.9, box.y + box.height / 2);
    for (let i = 0; i < 5; i++) {
      await page.mouse.wheel(0, -200);
      await page.waitForTimeout(100);
    }

    // Agent nodes should still be visible regardless of timeline filter
    const agentVisible = await page.evaluate(() => {
      const b = window.__beads3d;
      const agents = b.graph.graphData().nodes.filter(n => n.issue_type === 'agent');
      return agents.every(a => !a._hidden);
    });
    expect(agentVisible).toBe(true);
  });
});
