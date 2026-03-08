// E2E tests for live graph mutation updates (bd-4j1l8).
// Tests optimistic mutation handling: status changes, title/assignee updates,
// create/delete triggering full refresh, and debounce behavior.
//
// Run: npx playwright test tests/mutations.spec.js
// View report: npx playwright show-report test-results/html-report

import { test, expect } from '@playwright/test';
import { MOCK_GRAPH, MOCK_PING, MOCK_SHOW } from './fixtures.js';

// Mock all API endpoints
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
  await page.route('**/api/bd.v1.BeadsService/Update', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) }));
  await page.route('**/api/bd.v1.BeadsService/Close', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) }));

  // Mock /events SSE — keep alive but don't send anything (tests inject events manually)
  await page.route('**/api/events', route =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));

  // Mock /bus/events SSE for doots
  await page.route('**/api/bus/events*', route =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
}

// Wait for graph to load with node data and mutation handler exposed
async function waitForGraph(page) {
  await page.waitForSelector('#status.connected', { timeout: 15000 });
  await page.waitForTimeout(3000);
  await page.waitForFunction(() => {
    const b = window.__beads3d;
    return b && b.graph && b.graph.graphData().nodes.length > 0
      && typeof window.__beads3d_applyMutation === 'function';
  }, { timeout: 10000 });
}

// Helper: get a node's status from the graph
function getNodeStatus(page, nodeId) {
  return page.evaluate((id) => {
    const nodes = window.__beads3d.graphData().nodes;
    const node = nodes.find(n => n.id === id);
    return node ? node.status : null;
  }, nodeId);
}

// Helper: get a node's title from the graph
function getNodeTitle(page, nodeId) {
  return page.evaluate((id) => {
    const nodes = window.__beads3d.graphData().nodes;
    const node = nodes.find(n => n.id === id);
    return node ? node.title : null;
  }, nodeId);
}

// Helper: get a node's assignee from the graph
function getNodeAssignee(page, nodeId) {
  return page.evaluate((id) => {
    const nodes = window.__beads3d.graphData().nodes;
    const node = nodes.find(n => n.id === id);
    return node ? node.assignee : null;
  }, nodeId);
}

test.describe('Live mutation updates', () => {
  test.beforeEach(async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);
  });

  test('status mutation updates node status', async ({ page }) => {
    // Verify initial status
    const before = await getNodeStatus(page, 'bd-task1');
    expect(before).toBe('open');

    // Apply a status mutation
    const result = await page.evaluate(() => {
      return window.__beads3d_applyMutation({
        type: 'status',
        issue_id: 'bd-task1',
        old_status: 'open',
        new_status: 'in_progress',
      });
    });
    expect(result).toBe(true);

    // Verify status changed
    const after = await getNodeStatus(page, 'bd-task1');
    expect(after).toBe('in_progress');
  });

  test('status mutation returns false for unknown node', async ({ page }) => {
    const result = await page.evaluate(() => {
      return window.__beads3d_applyMutation({
        type: 'status',
        issue_id: 'bd-nonexistent',
        old_status: 'open',
        new_status: 'closed',
      });
    });
    expect(result).toBe(false);
  });

  test('update mutation changes title', async ({ page }) => {
    const before = await getNodeTitle(page, 'bd-feat1');
    expect(before).toBe('Add user authentication');

    const result = await page.evaluate(() => {
      return window.__beads3d_applyMutation({
        type: 'update',
        issue_id: 'bd-feat1',
        title: 'Add OAuth2 authentication',
      });
    });
    expect(result).toBe(true);

    const after = await getNodeTitle(page, 'bd-feat1');
    expect(after).toBe('Add OAuth2 authentication');
  });

  test('update mutation changes assignee', async ({ page }) => {
    const before = await getNodeAssignee(page, 'bd-task1');
    expect(before).toBe('bob');

    const result = await page.evaluate(() => {
      return window.__beads3d_applyMutation({
        type: 'update',
        issue_id: 'bd-task1',
        assignee: 'charlie',
      });
    });
    expect(result).toBe(true);

    const after = await getNodeAssignee(page, 'bd-task1');
    expect(after).toBe('charlie');
  });

  test('create mutation returns false (needs full refresh)', async ({ page }) => {
    const result = await page.evaluate(() => {
      return window.__beads3d_applyMutation({
        type: 'create',
        issue_id: 'bd-new-task',
        title: 'New task',
      });
    });
    expect(result).toBe(false);
  });

  test('delete mutation returns false (needs full refresh)', async ({ page }) => {
    const result = await page.evaluate(() => {
      return window.__beads3d_applyMutation({
        type: 'delete',
        issue_id: 'bd-task1',
      });
    });
    expect(result).toBe(false);
  });

  test('mutation with no issue_id returns false', async ({ page }) => {
    const result = await page.evaluate(() => {
      return window.__beads3d_applyMutation({
        type: 'status',
        new_status: 'closed',
      });
    });
    expect(result).toBe(false);
  });

  test('unknown mutation type returns false', async ({ page }) => {
    const result = await page.evaluate(() => {
      return window.__beads3d_applyMutation({
        type: 'comment',
        issue_id: 'bd-task1',
      });
    });
    expect(result).toBe(false);
  });

  test('multiple rapid status mutations are applied sequentially', async ({ page }) => {
    // Apply three status changes rapidly
    await page.evaluate(() => {
      window.__beads3d_applyMutation({
        type: 'status', issue_id: 'bd-task1', new_status: 'in_progress',
      });
      window.__beads3d_applyMutation({
        type: 'status', issue_id: 'bd-task1', new_status: 'closed',
      });
      window.__beads3d_applyMutation({
        type: 'status', issue_id: 'bd-task1', new_status: 'open',
      });
    });

    // Final status should be the last one applied
    const status = await getNodeStatus(page, 'bd-task1');
    expect(status).toBe('open');
  });

  test('status mutation preserves other node properties', async ({ page }) => {
    // Get initial properties
    const before = await page.evaluate(() => {
      const node = window.__beads3d.graphData().nodes.find(n => n.id === 'bd-task1');
      return { title: node.title, priority: node.priority, assignee: node.assignee, issue_type: node.issue_type };
    });

    // Apply status change
    await page.evaluate(() => {
      window.__beads3d_applyMutation({
        type: 'status', issue_id: 'bd-task1', new_status: 'in_progress',
      });
    });

    // Verify other properties unchanged
    const after = await page.evaluate(() => {
      const node = window.__beads3d.graphData().nodes.find(n => n.id === 'bd-task1');
      return { title: node.title, priority: node.priority, assignee: node.assignee, issue_type: node.issue_type };
    });

    expect(after.title).toBe(before.title);
    expect(after.priority).toBe(before.priority);
    expect(after.assignee).toBe(before.assignee);
    expect(after.issue_type).toBe(before.issue_type);
  });
});
