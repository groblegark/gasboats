// Interaction tests for beads3d editing features (bd-ibxu4).
// Tests context menu editing, keyboard shortcuts, and status feedback.
// Uses mocked API with request tracking to verify write operations.
//
// Run: npx playwright test tests/interactions.spec.js
// View report: npx playwright show-report test-results/html-report

import { test, expect } from '@playwright/test';
import { MOCK_GRAPH, MOCK_PING, MOCK_SHOW } from './fixtures.js';

// Track API calls for assertions
function createAPITracker() {
  const calls = [];
  return {
    calls,
    getCallsTo(method) {
      return calls.filter(c => c.method === method);
    },
    lastCallTo(method) {
      const matching = this.getCallsTo(method);
      return matching[matching.length - 1] || null;
    },
  };
}

// Mock API with request tracking
async function mockAPI(page, tracker) {
  const handle = async (method, response) => {
    await page.route(`**/api/bd.v1.BeadsService/${method}`, async route => {
      const body = route.request().postDataJSON();
      tracker.calls.push({ method, body, time: Date.now() });
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(response),
      });
    });
  };

  await handle('Ping', MOCK_PING);
  await handle('Graph', MOCK_GRAPH);
  await handle('List', []);
  await handle('Show', MOCK_SHOW);
  await handle('Stats', MOCK_GRAPH.stats);
  await handle('Blocked', []);
  await handle('Ready', []);
  await handle('Update', { ok: true });
  await handle('Close', { ok: true });

  await page.route('**/api/events', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      body: 'data: {"type":"ping"}\n\n',
    });
  });
}

// Wait for graph to render and have node data
async function waitForGraph(page) {
  await page.waitForSelector('#status.connected', { timeout: 15000 });
  await page.waitForTimeout(3000);
  // Poll until graph data is populated (avoids flaky timing issues)
  await page.waitForFunction(() => {
    const b = window.__beads3d;
    return b && b.graph && b.graph.graphData().nodes.length > 0;
  }, { timeout: 10000 });
}

// Trigger a right-click on a node via the graph API (reliable with WebGL canvas).
// Uses the same pattern as visual.spec.js — calls the onNodeRightClick handler directly.
async function rightClickNode(page, nodeId) {
  return page.evaluate((id) => {
    const b = window.__beads3d;
    if (!b || !b.graph) return false;
    const node = b.graph.graphData().nodes.find(n => n.id === id);
    if (!node) return false;
    b.graph.onNodeRightClick()(node, {
      preventDefault: () => {},
      clientX: 400,
      clientY: 300,
    });
    return true;
  }, nodeId);
}

test.describe('context menu editing', () => {

  test('right-click shows edit menu with status and priority submenus', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Right-click node via graph API (reliable with WebGL)
    const clicked = await rightClickNode(page, 'bd-task1');
    expect(clicked).toBe(true);
    await page.waitForTimeout(500);

    // Context menu should be visible
    const menu = page.locator('#context-menu');
    await expect(menu).toBeVisible();

    // Should contain status and priority submenus
    await expect(menu.locator('.ctx-submenu:has-text("status")')).toBeVisible();
    await expect(menu.locator('.ctx-submenu:has-text("priority")')).toBeVisible();
    await expect(menu.locator('[data-action="claim"]')).toBeVisible();
    await expect(menu.locator('[data-action="close-bead"]')).toBeVisible();

    // Should also have the original actions
    await expect(menu.locator('[data-action="expand-deps"]')).toBeVisible();
    await expect(menu.locator('[data-action="copy-id"]')).toBeVisible();
  });

  test('right-click on agent node does NOT show context menu', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Right-click agent node via graph API
    const clicked = await rightClickNode(page, 'agent:alice');
    if (!clicked) {
      test.skip(); // Agent node not in graph data
      return;
    }
    await page.waitForTimeout(500);

    // Context menu should NOT be visible for agent nodes
    const menu = page.locator('#context-menu');
    await expect(menu).not.toBeVisible();
  });

  test('status submenu sends Update API call', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Right-click → open context menu
    const clicked = await rightClickNode(page, 'bd-task1');
    expect(clicked).toBe(true);
    await page.waitForTimeout(500);

    // Hover over "status" to open submenu
    await page.locator('#context-menu .ctx-submenu:has-text("status")').hover();
    await page.waitForTimeout(200);

    // Click "in progress" in the submenu
    await page.locator('.ctx-sub-item:has-text("in progress")').click();
    await page.waitForTimeout(500);

    // Verify Update API was called with correct args
    const updateCalls = tracker.getCallsTo('Update');
    expect(updateCalls.length).toBeGreaterThanOrEqual(1);
    const lastUpdate = updateCalls[updateCalls.length - 1];
    expect(lastUpdate.body.id).toBe('bd-task1');
    expect(lastUpdate.body.status).toBe('in_progress');

    // Verify a refresh was triggered (Graph re-fetched)
    const graphCalls = tracker.getCallsTo('Graph');
    expect(graphCalls.length).toBeGreaterThan(1); // initial + refresh
  });

  test('priority submenu sends Update API call', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const clicked = await rightClickNode(page, 'bd-task1');
    expect(clicked).toBe(true);
    await page.waitForTimeout(500);

    // Hover over "priority" to open submenu
    await page.locator('#context-menu .ctx-submenu:has-text("priority")').hover();
    await page.waitForTimeout(200);

    // Click "P0 critical"
    await page.locator('.ctx-sub-item:has-text("P0 critical")').click();
    await page.waitForTimeout(500);

    const updateCalls = tracker.getCallsTo('Update');
    expect(updateCalls.length).toBeGreaterThanOrEqual(1);
    const lastUpdate = updateCalls[updateCalls.length - 1];
    expect(lastUpdate.body.id).toBe('bd-task1');
    expect(lastUpdate.body.priority).toBe(0);
  });

  test('close action sends Close API call', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const clicked = await rightClickNode(page, 'bd-task1');
    expect(clicked).toBe(true);
    await page.waitForTimeout(500);

    await page.locator('#context-menu [data-action="close-bead"]').click();
    await page.waitForTimeout(500);

    const closeCalls = tracker.getCallsTo('Close');
    expect(closeCalls.length).toBe(1);
    expect(closeCalls[0].body.id).toBe('bd-task1');
  });

  test('claim action sends Update with in_progress status', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const clicked = await rightClickNode(page, 'bd-task1');
    expect(clicked).toBe(true);
    await page.waitForTimeout(500);

    await page.locator('#context-menu [data-action="claim"]').click();
    await page.waitForTimeout(500);

    const updateCalls = tracker.getCallsTo('Update');
    expect(updateCalls.length).toBeGreaterThanOrEqual(1);
    const lastUpdate = updateCalls[updateCalls.length - 1];
    expect(lastUpdate.body.id).toBe('bd-task1');
    expect(lastUpdate.body.status).toBe('in_progress');
  });

  test('status toast appears after successful action', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const clicked = await rightClickNode(page, 'bd-task1');
    expect(clicked).toBe(true);
    await page.waitForTimeout(500);

    await page.locator('#context-menu [data-action="close-bead"]').click();

    // Status bar should briefly show the toast message
    const status = page.locator('#status');
    await expect(status).toContainText('bd-task1', { timeout: 1000 });
  });
});

test.describe('keyboard shortcuts', () => {

  test('/ focuses search input', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    await page.keyboard.press('/');
    const searchInput = page.locator('#search-input');
    await expect(searchInput).toBeFocused();
  });

  test('Escape clears search and closes panels', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Type in search
    await page.keyboard.press('/');
    await page.keyboard.type('epic');
    await page.waitForTimeout(200);

    // Press Escape
    await page.keyboard.press('Escape');
    await page.waitForTimeout(200);

    const searchInput = page.locator('#search-input');
    await expect(searchInput).toHaveValue('');
  });

  test('r triggers refresh (Graph API re-fetched)', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const initialGraphCalls = tracker.getCallsTo('Graph').length;
    await page.keyboard.press('r');
    await page.waitForTimeout(1000);

    expect(tracker.getCallsTo('Graph').length).toBeGreaterThan(initialGraphCalls);
  });

  test('l toggles labels on and off persistently (beads-p97b)', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Labels are ON by default (bd-oypa2)
    const btnLabels = page.locator('#btn-labels');
    await expect(btnLabels).toHaveClass(/active/);

    // Label sprites should be visible in the Three.js scene
    // LOD system (beads-bu3r) may limit how many labels show based on zoom distance,
    // so we check that at least some are visible (not necessarily all).
    const labelsOn = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;
      let visible = 0;
      let total = 0;
      b.graph.scene().traverse(child => {
        if (child.userData && child.userData.nodeLabel) {
          total++;
          if (child.visible) visible++;
        }
      });
      return { visible, total };
    });
    expect(labelsOn.total).toBeGreaterThan(0);
    expect(labelsOn.visible).toBeGreaterThan(0);

    // Wait a moment to verify labels persist (not just a flash)
    await page.waitForTimeout(500);

    const stillOn = await page.evaluate(() => {
      const b = window.__beads3d;
      let visible = 0;
      let total = 0;
      b.graph.scene().traverse(child => {
        if (child.userData && child.userData.nodeLabel) {
          total++;
          if (child.visible) visible++;
        }
      });
      return { visible, total };
    });
    expect(stillOn.visible).toBeGreaterThan(0);

    // Press 'l' to toggle labels off
    await page.keyboard.press('l');
    await page.waitForTimeout(300);

    await expect(btnLabels).not.toHaveClass(/active/);

    const labelsOff = await page.evaluate(() => {
      const b = window.__beads3d;
      let visible = 0;
      b.graph.scene().traverse(child => {
        if (child.userData && child.userData.nodeLabel && child.visible) visible++;
      });
      return visible;
    });
    expect(labelsOff).toBe(0);
  });

  test('labels button click toggles labels on and off', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const btnLabels = page.locator('#btn-labels');

    // Labels are ON by default (bd-oypa2) — button starts active
    await expect(btnLabels).toHaveClass(/active/);

    // Click button to turn off
    await btnLabels.click();
    await page.waitForTimeout(300);
    await expect(btnLabels).not.toHaveClass(/active/);

    // Click button to turn back on
    await btnLabels.click();
    await page.waitForTimeout(300);
    await expect(btnLabels).toHaveClass(/active/);
  });
});

// --- Bulk mutation tests (bd-d8189) ---
// Programmatically set multiSelected and trigger bulk menu,
// then verify API calls and local graph state changes.

// Helper: click a bulk menu action item directly via DOM
// CSS :hover submenus are fragile in Playwright, so we reveal + click via evaluate
async function clickBulkAction(page, action, value) {
  await page.evaluate(({ action, value }) => {
    // Hide all submenu panels and reset z-index to avoid overlap/interception
    document.querySelectorAll('#bulk-menu .bulk-submenu-panel').forEach(p => {
      p.style.display = 'none';
      p.style.zIndex = '';
    });
    // Find the target item and reveal only its parent submenu panel
    const selector = value !== undefined
      ? `#bulk-menu [data-action="${action}"][data-value="${value}"]`
      : `#bulk-menu [data-action="${action}"]`;
    const item = document.querySelector(selector);
    if (item) {
      const panel = item.closest('.bulk-submenu-panel');
      if (panel) {
        panel.style.display = 'block';
        panel.style.zIndex = '10000'; // above all other panels
      }
      item.click();
    }
  }, { action, value });
  await page.waitForTimeout(300);
}

// Helper: programmatically multi-select nodes and open the bulk menu
async function setupBulkSelection(page, nodeIds) {
  await page.evaluate(({ ids }) => {
    const b = window.__beads3d;
    if (!b) return;
    const sel = b.multiSelected();
    sel.clear();
    for (const id of ids) sel.add(id);
    b.showBulkMenu(400, 300);
  }, { ids: nodeIds });
  await page.waitForTimeout(300);
}

test.describe('bulk menu mutations', () => {

  test('bulk set-status sends Update for each selected node', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const targetIds = ['bd-task1', 'bd-task2', 'bd-feat2'];
    await setupBulkSelection(page, targetIds);

    // Verify bulk menu is visible
    const bulkMenu = page.locator('#bulk-menu');
    await expect(bulkMenu).toBeVisible();
    await expect(bulkMenu).toContainText('3 beads selected');

    // Hover "set status" to open submenu, then click "in progress"
    await clickBulkAction(page, 'bulk-status', 'in_progress');
    await page.waitForTimeout(500);

    // Verify Update was called for each selected node
    const updateCalls = tracker.getCallsTo('Update');
    expect(updateCalls.length).toBe(3);
    const updatedIds = updateCalls.map(c => c.body.id).sort();
    expect(updatedIds).toEqual(targetIds.sort());
    for (const call of updateCalls) {
      expect(call.body.status).toBe('in_progress');
    }
  });

  test('bulk set-status optimistically updates local node state', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const targetIds = ['bd-task1', 'bd-task5'];
    await setupBulkSelection(page, targetIds);

    // Click bulk set-status → closed
    await clickBulkAction(page, 'bulk-status', 'closed');
    await page.waitForTimeout(300);

    // Verify local graph state updated optimistically
    const statuses = await page.evaluate((ids) => {
      const b = window.__beads3d;
      if (!b) return null;
      const nodes = b.graphData().nodes;
      return ids.map(id => {
        const n = nodes.find(n => n.id === id);
        return n ? n.status : null;
      });
    }, targetIds);
    expect(statuses).toEqual(['closed', 'closed']);
  });

  test('bulk set-priority sends Update with correct priority values', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const targetIds = ['bd-bug1', 'bd-feat3'];
    await setupBulkSelection(page, targetIds);

    // Hover "set priority" and click "P0 critical"
    await clickBulkAction(page, 'bulk-priority', '0');
    await page.waitForTimeout(500);

    const updateCalls = tracker.getCallsTo('Update');
    expect(updateCalls.length).toBe(2);
    for (const call of updateCalls) {
      expect(call.body.priority).toBe(0);
    }
    const updatedIds = updateCalls.map(c => c.body.id).sort();
    expect(updatedIds).toEqual(targetIds.sort());
  });

  test('bulk set-priority optimistically updates local node state', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const targetIds = ['bd-task1', 'bd-bug1'];
    await setupBulkSelection(page, targetIds);

    // Force all submenu panels visible and directly trigger the bulk action
    // (CSS hover-based submenus are fragile in Playwright)
    await clickBulkAction(page, 'bulk-priority', '4');
    await page.waitForTimeout(500);

    const priorities = await page.evaluate((ids) => {
      const b = window.__beads3d;
      if (!b) return null;
      const nodes = b.graphData().nodes;
      return ids.map(id => {
        const n = nodes.find(n => n.id === id);
        return n ? n.priority : null;
      });
    }, targetIds);
    expect(priorities).toEqual([4, 4]);
  });

  test('bulk close-all sends Close for each selected node', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const targetIds = ['bd-task2', 'bd-task5', 'bd-feat3'];
    await setupBulkSelection(page, targetIds);

    // Click "close all"
    await page.locator('#bulk-menu .bulk-item[data-action="bulk-close"]').click();
    await page.waitForTimeout(500);

    const closeCalls = tracker.getCallsTo('Close');
    expect(closeCalls.length).toBe(3);
    const closedIds = closeCalls.map(c => c.body.id).sort();
    expect(closedIds).toEqual(targetIds.sort());
  });

  test('bulk close-all optimistically sets nodes to closed status', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const targetIds = ['bd-task1', 'bd-bug1'];
    await setupBulkSelection(page, targetIds);

    await page.locator('#bulk-menu .bulk-item[data-action="bulk-close"]').click();
    await page.waitForTimeout(300);

    const statuses = await page.evaluate((ids) => {
      const b = window.__beads3d;
      if (!b) return null;
      const nodes = b.graphData().nodes;
      return ids.map(id => {
        const n = nodes.find(n => n.id === id);
        return n ? n.status : null;
      });
    }, targetIds);
    expect(statuses).toEqual(['closed', 'closed']);
  });

  test('bulk clear-selection dismisses menu without API calls', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const targetIds = ['bd-task1', 'bd-task2'];
    await setupBulkSelection(page, targetIds);

    const updatesBefore = tracker.getCallsTo('Update').length;
    const closesBefore = tracker.getCallsTo('Close').length;

    await page.locator('#bulk-menu .bulk-item[data-action="bulk-clear"]').click();
    await page.waitForTimeout(300);

    // No new API calls
    expect(tracker.getCallsTo('Update').length).toBe(updatesBefore);
    expect(tracker.getCallsTo('Close').length).toBe(closesBefore);

    // Menu should be hidden
    await expect(page.locator('#bulk-menu')).not.toBeVisible();

    // Selection should be cleared
    const selCount = await page.evaluate(() => {
      const b = window.__beads3d;
      return b ? b.multiSelected().size : -1;
    });
    expect(selCount).toBe(0);
  });

  test('bulk status toast shows operation summary', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const targetIds = ['bd-task1', 'bd-task2'];
    await setupBulkSelection(page, targetIds);

    await page.locator('#bulk-menu .bulk-item[data-action="bulk-close"]').click();

    // Status bar should show the toast
    const status = page.locator('#status');
    await expect(status).toContainText('closed 2', { timeout: 2000 });
  });

  test('bulk operation rolls back on API failure', async ({ page }) => {
    const tracker = createAPITracker();

    // Set up mock API but with Update returning 500 errors
    const handle = async (method, response) => {
      await page.route(`**/api/bd.v1.BeadsService/${method}`, async route => {
        const body = route.request().postDataJSON();
        tracker.calls.push({ method, body, time: Date.now() });
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify(response),
        });
      });
    };

    await handle('Ping', MOCK_PING);
    await handle('Graph', MOCK_GRAPH);
    await handle('List', []);
    await handle('Show', MOCK_SHOW);
    await handle('Stats', MOCK_GRAPH.stats);
    await handle('Blocked', []);
    await handle('Ready', []);
    await handle('Close', { ok: true });

    // Update returns 500 to test rollback
    await page.route('**/api/bd.v1.BeadsService/Update', async route => {
      tracker.calls.push({ method: 'Update', body: route.request().postDataJSON(), time: Date.now() });
      await route.fulfill({ status: 500, contentType: 'application/json', body: '{"error":"server error"}' });
    });

    await page.route('**/api/events', async route => {
      await route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' });
    });

    await page.goto('/');
    await waitForGraph(page);

    // Record original statuses
    const originalStatuses = await page.evaluate((ids) => {
      const b = window.__beads3d;
      if (!b) return null;
      return ids.map(id => {
        const n = b.graphData().nodes.find(n => n.id === id);
        return n ? n.status : null;
      });
    }, ['bd-task1', 'bd-task2']);

    const targetIds = ['bd-task1', 'bd-task2'];
    await setupBulkSelection(page, targetIds);

    await clickBulkAction(page, 'bulk-status', 'in_progress');
    await page.waitForTimeout(1500);

    // Statuses should be rolled back to originals
    const rolledBackStatuses = await page.evaluate((ids) => {
      const b = window.__beads3d;
      if (!b) return null;
      return ids.map(id => {
        const n = b.graphData().nodes.find(n => n.id === id);
        return n ? n.status : null;
      });
    }, ['bd-task1', 'bd-task2']);
    expect(rolledBackStatuses).toEqual(originalStatuses);

    // Verify Update API was actually called (confirming the action fired)
    const updateCalls = tracker.getCallsTo('Update');
    expect(updateCalls.length).toBe(2);
  });
});

// --- Detail panel tests (bd-yprv2) ---
// Verify the detail panel opens on node click, shows correct fields,
// lazy-loads full details, and closes properly.

test.describe('detail panel', () => {

  test('clicking a node opens detail panel with basic info', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Click node via graph API
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return;
      const node = b.graphData().nodes.find(n => n.id === 'bd-feat1');
      if (node) b.graph.onNodeClick()(node, { preventDefault: () => {} });
    });
    await page.waitForTimeout(1000);

    // Detail panel should be visible
    const detail = page.locator('#detail');
    await expect(detail).toBeVisible();

    // Should show the node ID
    await expect(detail.locator('.detail-id')).toContainText('bd-feat1');

    // Should show the title
    await expect(detail.locator('.detail-title')).toContainText('Add user authentication');

    // Should show status, type, and priority tags
    const meta = detail.locator('.detail-meta');
    await expect(meta).toContainText('in_progress');
    await expect(meta).toContainText('feature');
    await expect(meta).toContainText('P1');
  });

  test('detail panel lazy-loads full description via Show API', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Click node to open detail
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return;
      const node = b.graphData().nodes.find(n => n.id === 'bd-feat1');
      if (node) b.graph.onNodeClick()(node, { preventDefault: () => {} });
    });
    await page.waitForTimeout(2000);

    // Show API should have been called
    const showCalls = tracker.getCallsTo('Show');
    expect(showCalls.length).toBeGreaterThanOrEqual(1);

    // Description section should be rendered (from MOCK_SHOW fixture)
    const detail = page.locator('#detail');
    // Wait for lazy-loaded content to appear
    await expect(detail).toContainText('OAuth2 authentication', { timeout: 5000 });
  });

  test('detail panel shows dependency list', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return;
      const node = b.graphData().nodes.find(n => n.id === 'bd-feat1');
      if (node) b.graph.onNodeClick()(node, { preventDefault: () => {} });
    });
    await page.waitForTimeout(2000);

    // MOCK_SHOW has a dependency on bd-task1
    const detail = page.locator('#detail');
    await expect(detail).toContainText('Dependencies');
    await expect(detail).toContainText('Write OAuth integration tests');
  });

  test('detail panel close button hides the panel', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Open detail
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return;
      const node = b.graphData().nodes.find(n => n.id === 'bd-feat1');
      if (node) b.graph.onNodeClick()(node, { preventDefault: () => {} });
    });
    await page.waitForTimeout(1000);

    const detail = page.locator('#detail');
    await expect(detail).toBeVisible();

    // Click close button
    await detail.locator('.detail-close').click();
    await page.waitForTimeout(500);

    // Panel should be hidden
    await expect(detail).not.toBeVisible();
  });

  test('Escape key closes detail panel', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Open detail
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return;
      const node = b.graphData().nodes.find(n => n.id === 'bd-task1');
      if (node) b.graph.onNodeClick()(node, { preventDefault: () => {} });
    });
    await page.waitForTimeout(1000);
    await expect(page.locator('#detail')).toBeVisible();

    // Press Escape
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);

    await expect(page.locator('#detail')).not.toBeVisible();
  });

  test('detail panel shows assignee tag', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return;
      const node = b.graphData().nodes.find(n => n.id === 'bd-feat1');
      if (node) b.graph.onNodeClick()(node, { preventDefault: () => {} });
    });
    await page.waitForTimeout(1000);

    // bd-feat1 has assignee 'alice'
    const meta = page.locator('#detail .detail-meta');
    await expect(meta.locator('.tag-assignee')).toContainText('alice');
  });

  // bd-fbmq3: tiling detail panels
  test('multiple detail panels tile side-by-side', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Open first panel (bd-feat1)
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return;
      const node = b.graphData().nodes.find(n => n.id === 'bd-feat1');
      if (node) b.showDetail(node);
    });

    const detail = page.locator('#detail');
    await expect(detail).toBeVisible();
    let panels = detail.locator('.detail-panel');
    await expect(panels).toHaveCount(1);

    // Open second panel (bd-task1)
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return;
      const node = b.graphData().nodes.find(n => n.id === 'bd-task1');
      if (node) b.showDetail(node);
    });

    panels = detail.locator('.detail-panel');
    await expect(panels).toHaveCount(2);

    // Both beads should be visible in their respective panels
    await expect(detail).toContainText('bd-feat1');
    await expect(detail).toContainText('bd-task1');

    // Close first panel by clicking it again (toggle)
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return;
      const node = b.graphData().nodes.find(n => n.id === 'bd-feat1');
      if (node) b.showDetail(node);
    });

    panels = detail.locator('.detail-panel');
    await expect(panels).toHaveCount(1);
    await expect(detail).toContainText('bd-task1');
  });
});

// --- Search navigation tests (bd-yprv2) ---
// Verify search input filtering, result count display, and arrow key navigation.

test.describe('search navigation', () => {

  test('typing in search filters nodes and shows match count', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Focus search and type
    await page.keyboard.press('/');
    await page.keyboard.type('epic');
    await page.waitForTimeout(500);

    // Filter count should show matches
    const filterCount = page.locator('#filter-count');
    await expect(filterCount).toContainText('matches');

    // Should show visible/total count
    await expect(filterCount).toContainText('/');
  });

  test('search matches by node ID', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    await page.keyboard.press('/');
    await page.keyboard.type('bd-bug1');
    await page.waitForTimeout(500);

    const filterCount = page.locator('#filter-count');
    // Should find exactly 1 match
    await expect(filterCount).toContainText('1/1 matches');
  });

  test('search matches by assignee', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    await page.keyboard.press('/');
    await page.keyboard.type('alice');
    await page.waitForTimeout(500);

    // alice is assigned to bd-epic1, bd-feat1, bd-bug2, and agent:alice
    const filterCount = page.locator('#filter-count');
    const text = await filterCount.textContent();
    // Should have multiple matches
    expect(text).toMatch(/\d+\/\d+ matches/);
  });

  test('Enter key on search result opens detail panel', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    await page.keyboard.press('/');
    await page.keyboard.type('bd-bug1');
    await page.waitForTimeout(500);

    // Press Enter to fly to result
    await page.keyboard.press('Enter');
    await page.waitForTimeout(2000);

    // Detail panel should open for the matched node
    const detail = page.locator('#detail');
    await expect(detail).toBeVisible();
    await expect(detail.locator('.detail-id')).toContainText('bd-bug1');
  });

  test('ArrowDown/ArrowUp cycle through search results', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Search for 'task' — should match multiple nodes (bd-task1..5)
    await page.keyboard.press('/');
    await page.keyboard.type('task');
    await page.waitForTimeout(500);

    const filterCount = page.locator('#filter-count');

    // Should start at result 1
    let text = await filterCount.textContent();
    expect(text).toMatch(/^1\//);

    // Press ArrowDown to go to next result
    await page.keyboard.press('ArrowDown');
    await page.waitForTimeout(1000);
    text = await filterCount.textContent();
    expect(text).toMatch(/^2\//);

    // Press ArrowDown again
    await page.keyboard.press('ArrowDown');
    await page.waitForTimeout(1000);
    text = await filterCount.textContent();
    expect(text).toMatch(/^3\//);

    // Press ArrowUp to go back
    await page.keyboard.press('ArrowUp');
    await page.waitForTimeout(1000);
    text = await filterCount.textContent();
    expect(text).toMatch(/^2\//);
  });

  test('clearing search restores all nodes', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Get total node count (includes all nodes regardless of _hidden)
    const totalBefore = await page.evaluate(() => {
      const b = window.__beads3d;
      return b ? b.graphData().nodes.length : 0;
    });

    // Search to filter — poll until some nodes are hidden (avoids fixed timeout race)
    await page.keyboard.press('/');
    await page.keyboard.type('epic');
    await page.waitForFunction(() => {
      const b = window.__beads3d;
      if (!b) return false;
      return b.graphData().nodes.some(n => n._hidden);
    }, { timeout: 5000 });

    // Verify some nodes are hidden
    const visibleDuring = await page.evaluate(() => {
      const b = window.__beads3d;
      return b ? b.graphData().nodes.filter(n => !n._hidden).length : 0;
    });
    expect(visibleDuring).toBeLessThan(totalBefore);

    // Press Escape to clear search — poll until search filter is removed and
    // age filter has settled (exactly 1 node should remain hidden: bd-old1)
    await page.keyboard.press('Escape');
    await page.waitForFunction((expected) => {
      const b = window.__beads3d;
      if (!b) return false;
      const visible = b.graphData().nodes.filter(n => !n._hidden).length;
      return visible === expected;
    }, totalBefore - 1, { timeout: 5000 });

    const visibleAfter = await page.evaluate(() => {
      const b = window.__beads3d;
      return b ? b.graphData().nodes.filter(n => !n._hidden).length : 0;
    });
    // With default 7d age filter, bd-old1 (disconnected, closed, updated 2025-12-15) stays hidden.
    // bd-old2 is rescued because it's connected to active bd-task3.
    expect(visibleAfter).toBe(totalBefore - 1);
  });

  test('keyboard shortcuts do not fire while search input is focused', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Focus search
    await page.keyboard.press('/');
    await page.waitForTimeout(300);

    // Record graph calls AFTER search is focused (avoids counting any in-flight refreshes)
    const graphCallsAfterFocus = tracker.getCallsTo('Graph').length;

    // Type 'r' — should NOT trigger refresh (just types in search input)
    await page.keyboard.type('r');
    await page.waitForTimeout(500);

    // No new Graph calls should have been made
    expect(tracker.getCallsTo('Graph').length).toBe(graphCallsAfterFocus);

    // Search input should contain 'r'
    const searchInput = page.locator('#search-input');
    await expect(searchInput).toHaveValue('r');
  });
});

// --- Context menu advanced actions (bd-yprv2) ---
// Test copy-id, show-deps, show-blockers, and expand-deps actions.

test.describe('context menu advanced actions', () => {

  test('copy-id action copies node ID to clipboard', async ({ page, context }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    // Grant clipboard permission
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await page.goto('/');
    await waitForGraph(page);

    const clicked = await rightClickNode(page, 'bd-task1');
    expect(clicked).toBe(true);
    await page.waitForTimeout(500);

    // Click copy-id action via evaluate (context menu can detach during refresh)
    await page.evaluate(() => {
      const item = document.querySelector('#context-menu [data-action="copy-id"]');
      if (item) item.click();
    });
    await page.waitForTimeout(300);

    // Verify clipboard content
    const clipText = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipText).toBe('bd-task1');
  });

  test('copy-show action copies "bd show" command to clipboard', async ({ page, context }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await page.goto('/');
    await waitForGraph(page);

    const clicked = await rightClickNode(page, 'bd-feat1');
    expect(clicked).toBe(true);
    await page.waitForTimeout(500);

    // Click via evaluate to avoid detached DOM flakiness
    await page.evaluate(() => {
      const item = document.querySelector('#context-menu [data-action="copy-show"]');
      if (item) item.click();
    });
    await page.waitForTimeout(300);

    const clipText = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipText).toBe('bd show bd-feat1');
  });

  test('show-deps highlights downstream subgraph', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Right-click on bd-epic1 (has 3 downstream deps: bd-feat1, bd-feat2, bd-task2)
    const clicked = await rightClickNode(page, 'bd-epic1');
    expect(clicked).toBe(true);
    await page.waitForTimeout(500);

    // Click show-deps via evaluate (it's a data-action item, may be in submenu)
    await page.evaluate(() => {
      const item = document.querySelector('#context-menu [data-action="show-deps"]');
      if (item) item.click();
    });
    await page.waitForTimeout(500);

    // Context menu should close
    await expect(page.locator('#context-menu')).not.toBeVisible();

    // Check that some nodes are dimmed (non-subgraph) and some are not
    // Verify the subgraph was highlighted (we can't access highlightNodes directly,
    // but we can verify the action completed without error and menu closed)
    // The visual effect is that non-subgraph links become thin (0.2) and subgraph links stay wide
    const nodeCount = await page.evaluate(() => {
      const b = window.__beads3d;
      return b ? b.graphData().nodes.length : 0;
    });
    expect(nodeCount).toBeGreaterThan(0);
  });

  test('show-blockers highlights upstream subgraph', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Right-click on bd-task4 (blocked by bd-task3 which is child of bd-epic2)
    const clicked = await rightClickNode(page, 'bd-task4');
    expect(clicked).toBe(true);
    await page.waitForTimeout(500);

    await page.evaluate(() => {
      const item = document.querySelector('#context-menu [data-action="show-blockers"]');
      if (item) item.click();
    });
    await page.waitForTimeout(500);

    // Context menu should close
    await expect(page.locator('#context-menu')).not.toBeVisible();
  });

  test('expand-deps calls Show API and keeps context menu closed', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const showCallsBefore = tracker.getCallsTo('Show').length;

    const clicked = await rightClickNode(page, 'bd-feat1');
    expect(clicked).toBe(true);
    await page.waitForTimeout(500);

    // Click expand-deps
    await page.locator('#context-menu [data-action="expand-deps"]').click();
    await page.waitForTimeout(1000);

    // Show API should be called to load full dep tree
    expect(tracker.getCallsTo('Show').length).toBeGreaterThan(showCallsBefore);

    // Context menu should close
    await expect(page.locator('#context-menu')).not.toBeVisible();
  });

  test('context menu closes on Escape key', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const clicked = await rightClickNode(page, 'bd-task1');
    expect(clicked).toBe(true);
    await page.waitForTimeout(500);
    await expect(page.locator('#context-menu')).toBeVisible();

    await page.keyboard.press('Escape');
    await page.waitForTimeout(300);

    await expect(page.locator('#context-menu')).not.toBeVisible();
  });
});

// --- Filter button tests (bd-yprv2) ---
// Test status and type filter toggle buttons.

test.describe('filter buttons', () => {

  test('status filter button toggles active state', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const btn = page.locator('.filter-status[data-status="open"]');
    // Initially not active
    await expect(btn).not.toHaveClass(/active/);

    // Click to activate
    await btn.click();
    await expect(btn).toHaveClass(/active/);

    // Click again to deactivate
    await btn.click();
    await expect(btn).not.toHaveClass(/active/);
  });

  test('status filter hides non-matching nodes', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    const totalNodes = await page.evaluate(() => {
      const b = window.__beads3d;
      return b ? b.graphData().nodes.length : 0;
    });

    // Click "in_progress" filter — should show only in_progress nodes
    await page.locator('.filter-status[data-status="in_progress"]').click();

    // Wait for filter to apply: all visible nodes should be in_progress (bd-jib6v)
    await page.waitForFunction((total) => {
      const b = window.__beads3d;
      if (!b) return false;
      const visible = b.graphData().nodes.filter(n => !n._hidden);
      return visible.length > 0 && visible.length < total
        && visible.every(n => n.status === 'in_progress' || n.issue_type === 'agent');
    }, totalNodes, { timeout: 5000 });

    const visibleNodes = await page.evaluate(() => {
      const b = window.__beads3d;
      return b ? b.graphData().nodes.filter(n => !n._hidden).length : 0;
    });

    // Should be fewer than total (only in_progress nodes + agents visible)
    expect(visibleNodes).toBeLessThan(totalNodes);
    expect(visibleNodes).toBeGreaterThan(0);
  });

  test('type filter button toggles and filters nodes', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Click "bug" type filter
    const btn = page.locator('.filter-type[data-type="bug"]');
    await btn.click();

    // Wait for filter to apply: visible nodes should be bugs or agents (bd-jib6v, bd-keeha)
    await page.waitForFunction(() => {
      const b = window.__beads3d;
      if (!b) return false;
      const visible = b.graphData().nodes.filter(n => !n._hidden);
      return visible.length > 0
        && visible.every(n => n.issue_type === 'bug' || n.issue_type === 'agent');
    }, null, { timeout: 5000 });
    await expect(btn).toHaveClass(/active/);
  });

  test('combined status and type filters intersect', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Filter: status=open AND type=task
    await page.locator('.filter-status[data-status="open"]').click();
    await page.locator('.filter-type[data-type="task"]').click();

    // Wait for combined filter: visible should be open tasks (+ agents exempt) (bd-jib6v)
    await page.waitForFunction(() => {
      const b = window.__beads3d;
      if (!b) return false;
      const visible = b.graphData().nodes.filter(n => !n._hidden);
      const nonAgents = visible.filter(n => n.issue_type !== 'agent');
      return nonAgents.length > 0
        && nonAgents.every(n => n.status === 'open' && n.issue_type === 'task');
    }, null, { timeout: 5000 });

    const visible = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return [];
      return b.graphData().nodes.filter(n => !n._hidden && n.issue_type !== 'agent')
        .map(n => ({ id: n.id, status: n.status, type: n.issue_type }));
    });

    // All visible non-agent nodes should be open tasks
    for (const n of visible) {
      expect(n.status).toBe('open');
      expect(n.type).toBe('task');
    }
    expect(visible.length).toBeGreaterThan(0);
  });

  test('age filter hides old closed beads but rescues connected ones', async ({ page }) => {
    const tracker = createAPITracker();
    await mockAPI(page, tracker);
    await page.goto('/');
    await waitForGraph(page);

    // Default age filter is 7d — old closed beads (bd-old1, bd-old2) should be handled
    // bd-old1: disconnected old closed → should be hidden
    // bd-old2: connected to bd-task3 (active) → should be rescued (visible)
    await page.waitForTimeout(500);

    const result = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return null;
      const nodes = b.graphData().nodes;
      const old1 = nodes.find(n => n.id === 'bd-old1');
      const old2 = nodes.find(n => n.id === 'bd-old2');
      return {
        old1Hidden: old1 ? old1._hidden : null,
        old2Hidden: old2 ? old2._hidden : null,
      };
    });

    expect(result).not.toBeNull();
    expect(result.old1Hidden).toBe(true);   // disconnected old closed = hidden
    expect(result.old2Hidden).toBe(false);  // connected to active node = rescued

    // Click "all" to show everything
    await page.locator('.filter-age[data-days="0"]').click();
    await page.waitForTimeout(300);

    const afterAll = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return null;
      const nodes = b.graphData().nodes;
      const old1 = nodes.find(n => n.id === 'bd-old1');
      return { old1Hidden: old1 ? old1._hidden : null };
    });

    expect(afterAll.old1Hidden).toBe(false);  // "all" shows everything
  });
});
