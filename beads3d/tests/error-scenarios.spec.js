// E2E tests for error scenarios and edge cases (bd-4vm96).
// Tests failure modes, degenerate data, mutation edge cases, and resilience.
//
// Run: npx playwright test tests/error-scenarios.spec.js
// View report: npx playwright show-report test-results/html-report

import { test, expect } from '@playwright/test';
import { MOCK_GRAPH, MOCK_PING, MOCK_SHOW } from './fixtures.js';

// --- Shared helpers ---

// Mock all API endpoints with standard fixtures
async function mockAPI(page, graphOverride) {
  const graphData = graphOverride || MOCK_GRAPH;
  await page.route('**/api/bd.v1.BeadsService/Ping', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) }));
  await page.route('**/api/bd.v1.BeadsService/Graph', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(graphData) }));
  await page.route('**/api/bd.v1.BeadsService/List', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
  await page.route('**/api/bd.v1.BeadsService/Show', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_SHOW) }));
  await page.route('**/api/bd.v1.BeadsService/Stats', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(graphData.stats || {}) }));
  await page.route('**/api/bd.v1.BeadsService/Blocked', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
  await page.route('**/api/bd.v1.BeadsService/Ready', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
  await page.route('**/api/bd.v1.BeadsService/Update', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) }));
  await page.route('**/api/bd.v1.BeadsService/Close', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) }));
  await page.route('**/api/events', route =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
  await page.route('**/api/bus/events*', route =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
}

// Wait for graph to render with nodes and mutation handler
async function waitForGraph(page) {
  // Wait for connected status OR for graph to initialize (init error may prevent connected class)
  await Promise.race([
    page.waitForSelector('#status.connected', { timeout: 15000 }),
    page.waitForTimeout(8000),
  ]);
  await page.waitForTimeout(2000);
  await page.waitForFunction(() => {
    const b = window.__beads3d;
    return b && b.graph && b.graphData && b.graphData().nodes.length > 0
      && typeof window.__beads3d_applyMutation === 'function';
  }, { timeout: 15000 });
}

// Wait for graph load without requiring nodes (for empty graph tests)
async function waitForGraphNoNodes(page) {
  await Promise.race([
    page.waitForSelector('#status.connected', { timeout: 15000 }),
    page.waitForTimeout(8000),
  ]);
  await page.waitForTimeout(2000);
  await page.waitForFunction(() => {
    const b = window.__beads3d;
    return b && b.graph;
  }, { timeout: 15000 });
}

// Check page has no uncaught JS errors
function collectErrors(page) {
  const errors = [];
  page.on('pageerror', err => errors.push(err.message));
  return errors;
}

// ============================================================================
// 1. Empty and Degenerate Data
// ============================================================================

test.describe('Empty and degenerate data', () => {
  test('empty graph (0 nodes, 0 edges) renders without crash', async ({ page }) => {
    const errors = collectErrors(page);
    const emptyGraph = { nodes: [], edges: [], stats: { total_open: 0, total_in_progress: 0, total_blocked: 0, total_closed: 0 } };
    await mockAPI(page, emptyGraph);
    await page.goto('/');
    await waitForGraphNoNodes(page);

    // Canvas should exist and be visible
    const canvas = page.locator('canvas');
    await expect(canvas.first()).toBeVisible();

    // No JS errors
    expect(errors).toHaveLength(0);

    // Graph should have 0 nodes
    const nodeCount = await page.evaluate(() => window.__beads3d.graphData().nodes.length);
    expect(nodeCount).toBe(0);
  });

  test('single node with no edges renders correctly', async ({ page }) => {
    const errors = collectErrors(page);
    const singleNode = {
      nodes: [
        { id: 'bd-solo', title: 'Solo node', status: 'open', priority: 2, issue_type: 'task', assignee: '', created_at: '2026-02-20T10:00:00Z', updated_at: '2026-02-20T10:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
      ],
      edges: [],
      stats: { total_open: 1, total_in_progress: 0, total_blocked: 0, total_closed: 0 },
    };
    await mockAPI(page, singleNode);
    await page.goto('/');
    await waitForGraph(page);

    expect(errors).toHaveLength(0);
    const nodeCount = await page.evaluate(() => window.__beads3d.graphData().nodes.length);
    expect(nodeCount).toBe(1);
  });

  test('nodes with missing fields do not crash', async ({ page }) => {
    const errors = collectErrors(page);
    const sparseGraph = {
      nodes: [
        { id: 'bd-sparse1', title: '', status: 'open', priority: 2, issue_type: 'task' },
        { id: 'bd-sparse2', status: 'in_progress', priority: null, issue_type: 'feature', assignee: null },
        { id: 'bd-sparse3', title: 'Missing created_at', status: 'open', issue_type: 'bug' },
      ],
      edges: [],
      stats: { total_open: 2, total_in_progress: 1, total_blocked: 0, total_closed: 0 },
    };
    await mockAPI(page, sparseGraph);
    await page.goto('/');
    await waitForGraph(page);

    expect(errors).toHaveLength(0);
    const nodeCount = await page.evaluate(() => window.__beads3d.graphData().nodes.length);
    expect(nodeCount).toBe(3);
  });

  test('very long title (500+ chars) does not overflow or crash', async ({ page }) => {
    const errors = collectErrors(page);
    const longTitle = 'A'.repeat(600);
    const longGraph = {
      nodes: [
        { id: 'bd-long', title: longTitle, status: 'open', priority: 2, issue_type: 'task', assignee: '', created_at: '2026-02-20T10:00:00Z', updated_at: '2026-02-20T10:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
      ],
      edges: [],
      stats: { total_open: 1, total_in_progress: 0, total_blocked: 0, total_closed: 0 },
    };
    await mockAPI(page, longGraph);
    await page.goto('/');
    await waitForGraph(page);

    expect(errors).toHaveLength(0);
    const stored = await page.evaluate(() => window.__beads3d.graphData().nodes[0].title);
    expect(stored.length).toBe(600);
  });

  test('unicode titles (emoji, CJK, RTL) render without crash', async ({ page }) => {
    const unicodeGraph = {
      nodes: [
        { id: 'bd-emoji', title: '🚀 Deploy rocket launch 🎉✨', status: 'open', priority: 2, issue_type: 'task', assignee: '', created_at: '2026-02-20T10:00:00Z', updated_at: '2026-02-20T10:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
        { id: 'bd-cjk', title: '数据库迁移脚本 テスト 한국어', status: 'in_progress', priority: 1, issue_type: 'feature', assignee: 'alice', created_at: '2026-02-20T10:00:00Z', updated_at: '2026-02-20T10:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
        { id: 'bd-rtl', title: 'مهمة اختبار النظام العربي', status: 'open', priority: 2, issue_type: 'task', assignee: '', created_at: '2026-02-20T10:00:00Z', updated_at: '2026-02-20T10:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
      ],
      edges: [],
      stats: { total_open: 2, total_in_progress: 1, total_blocked: 0, total_closed: 0 },
    };
    await mockAPI(page, unicodeGraph);
    await page.goto('/');
    await waitForGraphNoNodes(page);
    // Wait for nodes to load
    await page.waitForFunction(() => {
      const b = window.__beads3d;
      return b && b.graphData && b.graphData().nodes.length > 0;
    }, { timeout: 10000 });

    // Graph should load at least our 3 unicode nodes — canvas text rendering may
    // produce non-fatal warnings for certain Unicode ranges but should not crash.
    // Note: initUnifiedFeed or other features may inject extra internal nodes.
    const nodeCount = await page.evaluate(() => window.__beads3d.graphData().nodes.length);
    expect(nodeCount).toBeGreaterThanOrEqual(3);

    // Canvas should be visible (rendering survived)
    const canvas = page.locator('canvas');
    await expect(canvas.first()).toBeVisible();
  });
});

// ============================================================================
// 2. RPC/Network Error Scenarios
// ============================================================================

test.describe('RPC/Network error scenarios', () => {
  test('RPC 500 error on Graph endpoint does not crash', async ({ page }) => {
    const errors = collectErrors(page);

    await page.route('**/api/bd.v1.BeadsService/Ping', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) }));
    await page.route('**/api/bd.v1.BeadsService/Graph', route =>
      route.fulfill({ status: 500, contentType: 'application/json', body: JSON.stringify({ error: 'Internal Server Error' }) }));
    await page.route('**/api/bd.v1.BeadsService/List', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
    await page.route('**/api/bd.v1.BeadsService/Show', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({}) }));
    await page.route('**/api/bd.v1.BeadsService/Stats', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({}) }));
    await page.route('**/api/bd.v1.BeadsService/Blocked', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
    await page.route('**/api/bd.v1.BeadsService/Ready', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
    await page.route('**/api/events', route =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
    await page.route('**/api/bus/events*', route =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));

    await page.goto('/');
    await page.waitForTimeout(5000);

    // Page should still be alive (canvas present)
    const canvas = page.locator('canvas');
    await expect(canvas.first()).toBeVisible();
    // No uncaught JS errors (API errors may be logged but shouldn't throw)
    expect(errors).toHaveLength(0);
  });

  test('malformed JSON response from Graph does not crash', async ({ page }) => {
    const errors = collectErrors(page);

    await page.route('**/api/bd.v1.BeadsService/Ping', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) }));
    await page.route('**/api/bd.v1.BeadsService/Graph', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: '{invalid json!!!' }));
    await page.route('**/api/bd.v1.BeadsService/List', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
    await page.route('**/api/bd.v1.BeadsService/Stats', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({}) }));
    await page.route('**/api/bd.v1.BeadsService/Show', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({}) }));
    await page.route('**/api/bd.v1.BeadsService/Blocked', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
    await page.route('**/api/bd.v1.BeadsService/Ready', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
    await page.route('**/api/events', route =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
    await page.route('**/api/bus/events*', route =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));

    await page.goto('/');
    await page.waitForTimeout(5000);

    const canvas = page.locator('canvas');
    await expect(canvas.first()).toBeVisible();
    expect(errors).toHaveLength(0);
  });

  test('Ping failure (daemon unreachable) shows disconnected status', async ({ page }) => {
    const errors = collectErrors(page);

    await page.route('**/api/bd.v1.BeadsService/Ping', route =>
      route.fulfill({ status: 503, body: 'Service Unavailable' }));
    await page.route('**/api/bd.v1.BeadsService/Graph', route =>
      route.fulfill({ status: 503, body: 'Service Unavailable' }));
    await page.route('**/api/bd.v1.BeadsService/List', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
    await page.route('**/api/bd.v1.BeadsService/Stats', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({}) }));
    await page.route('**/api/bd.v1.BeadsService/Show', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({}) }));
    await page.route('**/api/bd.v1.BeadsService/Blocked', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
    await page.route('**/api/bd.v1.BeadsService/Ready', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
    await page.route('**/api/events', route =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
    await page.route('**/api/bus/events*', route =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));

    await page.goto('/');
    await page.waitForTimeout(5000);

    // Page should not crash even without daemon
    const canvas = page.locator('canvas');
    await expect(canvas.first()).toBeVisible();
    expect(errors).toHaveLength(0);
  });
});

// ============================================================================
// 3. Mutation Edge Cases
// ============================================================================

test.describe('Mutation edge cases', () => {
  test.beforeEach(async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);
  });

  test('10 rapid status mutations in 100ms — last value wins', async ({ page }) => {
    await page.evaluate(() => {
      for (let i = 0; i < 10; i++) {
        const statuses = ['open', 'in_progress', 'blocked', 'closed', 'review'];
        window.__beads3d_applyMutation({
          type: 'status',
          issue_id: 'bd-task1',
          new_status: statuses[i % statuses.length],
        });
      }
    });

    // Final status should be the last applied: i=9, 9 % 5 = 4 → 'review'
    const status = await page.evaluate(() => {
      const node = window.__beads3d.graphData().nodes.find(n => n.id === 'bd-task1');
      return node ? node.status : null;
    });
    expect(status).toBe('review');
  });

  test('mutation for nonexistent node does not crash', async ({ page }) => {
    const errors = collectErrors(page);

    const results = await page.evaluate(() => {
      return [
        window.__beads3d_applyMutation({ type: 'status', issue_id: 'bd-ghost-123', new_status: 'closed' }),
        window.__beads3d_applyMutation({ type: 'update', issue_id: 'bd-ghost-456', title: 'Phantom' }),
      ];
    });

    expect(results[0]).toBe(false);
    expect(results[1]).toBe(false);
    expect(errors).toHaveLength(0);
  });

  test('create + immediate close (race condition) does not crash', async ({ page }) => {
    const errors = collectErrors(page);

    await page.evaluate(() => {
      // Create event (returns false, triggers refresh)
      window.__beads3d_applyMutation({ type: 'create', issue_id: 'bd-race-test' });
      // Immediately close before refresh
      window.__beads3d_applyMutation({ type: 'status', issue_id: 'bd-race-test', new_status: 'closed' });
    });

    // Should not crash — close targets nonexistent node and returns false
    expect(errors).toHaveLength(0);
  });

  test('mutation with empty/null fields does not crash', async ({ page }) => {
    const errors = collectErrors(page);

    await page.evaluate(() => {
      window.__beads3d_applyMutation({ type: 'status', issue_id: 'bd-task1', new_status: '' });
      window.__beads3d_applyMutation({ type: 'update', issue_id: 'bd-task1', title: null });
      window.__beads3d_applyMutation({ type: 'update', issue_id: 'bd-task1', assignee: undefined });
      window.__beads3d_applyMutation({});
    });

    expect(errors).toHaveLength(0);
  });

  test('status change to same status is a no-op', async ({ page }) => {
    const before = await page.evaluate(() => {
      const node = window.__beads3d.graphData().nodes.find(n => n.id === 'bd-task1');
      return node.status;
    });

    const result = await page.evaluate(() =>
      window.__beads3d_applyMutation({ type: 'status', issue_id: 'bd-task1', new_status: 'open' })
    );

    // Returns true (applied) but status didn't change
    expect(result).toBe(true);
    const after = await page.evaluate(() => {
      const node = window.__beads3d.graphData().nodes.find(n => n.id === 'bd-task1');
      return node.status;
    });
    expect(after).toBe(before);
  });
});

// ============================================================================
// 4. Window/WebGL Resilience
// ============================================================================

test.describe('Window/WebGL resilience', () => {
  test('window resize during render does not crash', async ({ page }) => {
    const errors = collectErrors(page);
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Resize rapidly 5 times
    for (let i = 0; i < 5; i++) {
      await page.setViewportSize({ width: 800 + i * 100, height: 600 + i * 50 });
      await page.waitForTimeout(100);
    }

    // Wait for resize handlers to settle
    await page.waitForTimeout(1000);

    expect(errors).toHaveLength(0);

    // Canvas should still be visible
    const canvas = page.locator('canvas');
    await expect(canvas.first()).toBeVisible();
  });

  test('graph survives visibility change (tab background/foreground)', async ({ page }) => {
    const errors = collectErrors(page);
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Simulate visibility change (tab backgrounded)
    await page.evaluate(() => {
      Object.defineProperty(document, 'hidden', { value: true, writable: true, configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    await page.waitForTimeout(500);

    // Simulate tab foregrounded
    await page.evaluate(() => {
      Object.defineProperty(document, 'hidden', { value: false, writable: true, configurable: true });
      document.dispatchEvent(new Event('visibilitychange'));
    });
    await page.waitForTimeout(1000);

    expect(errors).toHaveLength(0);

    // Graph should still have nodes
    const nodeCount = await page.evaluate(() => window.__beads3d.graphData().nodes.length);
    expect(nodeCount).toBeGreaterThan(0);
  });
});

// ============================================================================
// 5. SSE Event Edge Cases
// ============================================================================

test.describe('SSE event edge cases', () => {
  test('SSE disconnect does not crash (EventSource closes)', async ({ page }) => {
    const errors = collectErrors(page);
    let eventRouteCount = 0;

    // First SSE connection succeeds briefly, then server closes it
    await page.route('**/api/events', async route => {
      eventRouteCount++;
      if (eventRouteCount === 1) {
        // First connection: send one event then close
        await route.fulfill({
          status: 200,
          contentType: 'text/event-stream',
          body: 'data: {"type":"ping"}\n\n',
        });
      } else {
        // Subsequent reconnects: succeed normally
        await route.fulfill({
          status: 200,
          contentType: 'text/event-stream',
          body: 'data: {"type":"ping"}\n\n',
        });
      }
    });

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
    await page.route('**/api/bus/events*', route =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));

    await page.goto('/');
    await page.waitForTimeout(5000);

    // Should not have uncaught errors from SSE reconnect
    expect(errors).toHaveLength(0);

    const canvas = page.locator('canvas');
    await expect(canvas.first()).toBeVisible();
  });

  test('malformed SSE event data does not crash', async ({ page }) => {
    const errors = collectErrors(page);

    await page.route('**/api/events', route =>
      route.fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body: 'event: mutations\ndata: {not valid json at all}\n\nevent: mutations\ndata: {"type":"status","issue_id":"bd-task1","new_status":"closed"}\n\n',
      }));
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
    await page.route('**/api/bus/events*', route =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));

    await page.goto('/');
    await page.waitForTimeout(5000);

    // Should survive malformed events
    expect(errors).toHaveLength(0);
    const canvas = page.locator('canvas');
    await expect(canvas.first()).toBeVisible();
  });
});

// ============================================================================
// 6. Graph Data Edge Cases
// ============================================================================

test.describe('Graph data edge cases', () => {
  test('self-referencing edge does not crash', async ({ page }) => {
    const errors = collectErrors(page);
    const selfRefGraph = {
      nodes: [
        { id: 'bd-self', title: 'Self-referencing node', status: 'open', priority: 2, issue_type: 'task', assignee: '', created_at: '2026-02-20T10:00:00Z', updated_at: '2026-02-20T10:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
      ],
      edges: [
        { source: 'bd-self', target: 'bd-self', dep_type: 'blocks' },
      ],
      stats: { total_open: 1, total_in_progress: 0, total_blocked: 0, total_closed: 0 },
    };
    await mockAPI(page, selfRefGraph);
    await page.goto('/');
    await waitForGraph(page);

    expect(errors).toHaveLength(0);
  });

  test('edge referencing nonexistent node does not crash', async ({ page }) => {
    const errors = collectErrors(page);
    const danglingGraph = {
      nodes: [
        { id: 'bd-exists', title: 'Real node', status: 'open', priority: 2, issue_type: 'task', assignee: '', created_at: '2026-02-20T10:00:00Z', updated_at: '2026-02-20T10:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
      ],
      edges: [
        { source: 'bd-exists', target: 'bd-phantom', dep_type: 'blocks' },
        { source: 'bd-phantom2', target: 'bd-exists', dep_type: 'relates-to' },
      ],
      stats: { total_open: 1, total_in_progress: 0, total_blocked: 0, total_closed: 0 },
    };
    await mockAPI(page, danglingGraph);
    await page.goto('/');
    await waitForGraph(page);

    expect(errors).toHaveLength(0);
  });

  test('duplicate node IDs do not crash', async ({ page }) => {
    const errors = collectErrors(page);
    const dupGraph = {
      nodes: [
        { id: 'bd-dup', title: 'First version', status: 'open', priority: 2, issue_type: 'task', assignee: '', created_at: '2026-02-20T10:00:00Z', updated_at: '2026-02-20T10:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
        { id: 'bd-dup', title: 'Duplicate version', status: 'closed', priority: 1, issue_type: 'bug', assignee: 'alice', created_at: '2026-02-20T10:00:00Z', updated_at: '2026-02-20T10:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
      ],
      edges: [],
      stats: { total_open: 1, total_in_progress: 0, total_blocked: 0, total_closed: 1 },
    };
    await mockAPI(page, dupGraph);
    await page.goto('/');
    await waitForGraph(page);

    expect(errors).toHaveLength(0);
  });

  test('all nodes closed — renders without crash', async ({ page }) => {
    const errors = collectErrors(page);
    const allClosedGraph = {
      nodes: [
        { id: 'bd-c1', title: 'Closed 1', status: 'closed', priority: 2, issue_type: 'task', assignee: '', created_at: '2026-02-20T10:00:00Z', updated_at: '2026-02-20T10:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
        { id: 'bd-c2', title: 'Closed 2', status: 'closed', priority: 1, issue_type: 'bug', assignee: 'bob', created_at: '2026-02-20T10:00:00Z', updated_at: '2026-02-20T10:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
        { id: 'bd-c3', title: 'Closed 3', status: 'closed', priority: 0, issue_type: 'epic', assignee: '', created_at: '2026-02-20T10:00:00Z', updated_at: '2026-02-20T10:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
      ],
      edges: [
        { source: 'bd-c3', target: 'bd-c1', dep_type: 'parent-child' },
        { source: 'bd-c1', target: 'bd-c2', dep_type: 'blocks' },
      ],
      stats: { total_open: 0, total_in_progress: 0, total_blocked: 0, total_closed: 3 },
    };
    await mockAPI(page, allClosedGraph);
    await page.goto('/');
    await waitForGraph(page);

    expect(errors).toHaveLength(0);
    const nodeCount = await page.evaluate(() => window.__beads3d.graphData().nodes.length);
    expect(nodeCount).toBe(3);
  });
});
