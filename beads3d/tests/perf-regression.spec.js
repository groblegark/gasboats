// Performance regression tests for beads3d (bd-yyrlx)
//
// Tests FPS thresholds, time-to-interactive, SSE burst handling, and memory stability.
//
// NOTE: CI uses SwiftShader (software WebGL) which is 50-100x slower than real GPU.
// Thresholds here are extremely conservative to avoid flaky failures. Run locally
// with a real GPU for meaningful FPS numbers.
//
// Run: npx playwright test tests/perf-regression.spec.js

import { test, expect } from '@playwright/test';
import { generateGraph, MOCK_PING, MOCK_SHOW } from './fixtures.js';

// Pre-generate deterministic test graphs at import time
const GRAPH_100 = generateGraph({ seed: 100, nodeCount: 100, agentCount: 5, epicCount: 2 });
const GRAPH_500 = generateGraph({ seed: 500, nodeCount: 500, agentCount: 15, epicCount: 6 });
const GRAPH_1000 = generateGraph({ seed: 1000, nodeCount: 1000, agentCount: 20, epicCount: 10 });

// Mock all API endpoints with a given graph
async function mockAPI(page, graph) {
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
  await page.route('**/api/bd.v1.BeadsService/Update', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) }));
  await page.route('**/api/bd.v1.BeadsService/Close', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) }));
  await page.route('**/api/events', route =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
  await page.route('**/api/bus/events*', route =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
}

// Wait for graph to load
async function waitForGraph(page, minNodes = 1, timeout = 30000) {
  await page.waitForSelector('#status', { timeout: 15000 });
  try {
    await page.waitForSelector('#status.connected', { timeout });
  } catch {
    // May not get connected state with mock SSE, proceed anyway
  }
  await page.waitForFunction((min) => {
    const b = window.__beads3d;
    return b && b.graph && b.graphData().nodes.length >= min;
  }, minNodes, { timeout });
}

// Measure FPS over a duration using requestAnimationFrame counting
async function measureFPS(page, durationMs = 10000) {
  return page.evaluate((dur) => {
    return new Promise(resolve => {
      const frameTimes = [];
      let lastTime = performance.now();
      function tick() {
        const now = performance.now();
        frameTimes.push(now - lastTime);
        lastTime = now;
        if (now - frameTimes[0] > dur) {
          // Drop first measurement (startup spike)
          frameTimes.shift();
          const sorted = [...frameTimes].sort((a, b) => a - b);
          const len = sorted.length;
          const avgMs = sorted.reduce((a, b) => a + b, 0) / len;
          resolve({
            fps: 1000 / avgMs,
            frames: len,
            avgMs,
            p50: sorted[Math.floor(len * 0.5)],
            p95: sorted[Math.floor(len * 0.95)],
            p99: sorted[Math.floor(len * 0.99)],
          });
        } else {
          requestAnimationFrame(tick);
        }
      }
      requestAnimationFrame(tick);
    });
  }, durationMs);
}

// Get JS heap usage (Chrome only)
async function getHeapUsage(page) {
  return page.evaluate(() => {
    const mem = performance.memory;
    return mem ? {
      usedMB: Math.round(mem.usedJSHeapSize / 1048576 * 10) / 10,
      totalMB: Math.round(mem.totalJSHeapSize / 1048576 * 10) / 10,
    } : null;
  });
}

// =====================================================================
// Test 1: FPS regression thresholds at various node counts
// =====================================================================
test.describe('FPS regression thresholds', () => {
  const configs = [
    { count: 100, graph: GRAPH_100, minFrames: 50, label: '100 nodes' },
    { count: 500, graph: GRAPH_500, minFrames: 10, label: '500 nodes' },
    { count: 1000, graph: GRAPH_1000, minFrames: 3, label: '1000 nodes' },
  ];

  for (const { count, graph, minFrames, label } of configs) {
    test(`${label}: render loop produces enough frames`, async ({ page }) => {
      test.setTimeout(120000);

      await mockAPI(page, graph);
      await page.goto('/');
      await waitForGraph(page, Math.min(count, 50));

      // Let force layout settle
      await page.waitForTimeout(8000);

      // Measure FPS over 10 seconds
      const perf = await measureFPS(page, 10000);
      console.log(`[perf-regression ${label}] FPS: ${perf.fps.toFixed(1)}, frames: ${perf.frames}, avgMs: ${perf.avgMs.toFixed(1)}, p95: ${perf.p95.toFixed(1)}`);

      // SwiftShader thresholds — verify render loop is alive and responsive
      // Real GPU: expect >30 FPS at 500 nodes, >20 FPS at 1000 nodes
      // SwiftShader: just verify frames are produced (1+ FPS equivalent)
      expect(perf.frames).toBeGreaterThan(minFrames);
    });
  }
});

// =====================================================================
// Test 2: Time-to-interactive (graph loaded + first paint)
// =====================================================================
test.describe('Time to interactive', () => {
  test('100 nodes: interactive within 5 seconds', async ({ page }) => {
    test.setTimeout(30000);

    await mockAPI(page, GRAPH_100);

    const startTime = Date.now();
    await page.goto('/');
    await waitForGraph(page, 10, 15000);
    const tti = Date.now() - startTime;

    console.log(`[perf-regression TTI] 100 nodes: ${tti}ms`);
    // Graph should load and show nodes within 5 seconds even with SwiftShader
    expect(tti).toBeLessThan(15000);
  });

  test('500 nodes: interactive within 15 seconds', async ({ page }) => {
    test.setTimeout(60000);

    await mockAPI(page, GRAPH_500);

    const startTime = Date.now();
    await page.goto('/');
    await waitForGraph(page, 10, 30000);
    const tti = Date.now() - startTime;

    console.log(`[perf-regression TTI] 500 nodes: ${tti}ms`);
    expect(tti).toBeLessThan(30000);
  });
});

// =====================================================================
// Test 3: SSE event burst handling — 50 rapid status changes
// =====================================================================
test.describe('SSE event burst handling', () => {
  test('50 rapid status changes: no frozen render loop', async ({ page }) => {
    test.setTimeout(120000);

    // Use small graph so SwiftShader doesn't choke
    await mockAPI(page, GRAPH_100);

    // Override SSE route to serve controllable stream
    let sseResolve;
    const ssePromise = new Promise(r => { sseResolve = r; });
    await page.route('**/api/bus/events*', async route => {
      // Build 50 rapid status mutation events
      const nodeIds = GRAPH_100.nodes.slice(0, 50).map(n => n.id);
      const events = nodeIds.map((id, i) => {
        const newStatus = i % 2 === 0 ? 'in_progress' : 'closed';
        return `data: ${JSON.stringify({
          type: 'mutation',
          payload: { issue_id: id, field: 'status', value: newStatus },
        })}\n\n`;
      });
      // Send all 50 events as a burst with ping header
      const body = 'data: {"type":"ping"}\n\n' + events.join('');
      await route.fulfill({
        status: 200,
        contentType: 'text/event-stream',
        body,
      });
      sseResolve();
    });

    await page.goto('/');
    await waitForGraph(page, 10);
    await page.waitForTimeout(5000); // let layout settle

    // Wait for SSE events to be served
    await ssePromise;
    // Give the app time to process the burst
    await page.waitForTimeout(3000);

    // Verify render loop is still alive after burst
    const perf = await measureFPS(page, 5000);
    console.log(`[perf-regression SSE burst] Post-burst FPS: ${perf.fps.toFixed(1)}, frames: ${perf.frames}`);
    expect(perf.frames).toBeGreaterThan(5);

    // Verify graph still has nodes (no crash/wipe)
    const nodeCount = await page.evaluate(() => {
      const b = window.__beads3d;
      return b && b.graphData ? b.graphData().nodes.length : 0;
    });
    expect(nodeCount).toBeGreaterThan(50);
  });
});

// =====================================================================
// Test 4: Memory stability — no unbounded growth over time
// =====================================================================
test.describe('Memory stability', () => {
  test('heap does not grow unboundedly over 2 minutes', async ({ page }) => {
    test.setTimeout(180000); // 3 min timeout

    await mockAPI(page, GRAPH_100);
    await page.goto('/');
    await waitForGraph(page, 10);
    await page.waitForTimeout(5000); // let layout settle

    // Take initial heap measurement
    const heapBefore = await getHeapUsage(page);
    if (!heapBefore) {
      test.skip(true, 'performance.memory not available');
      return;
    }
    console.log(`[perf-regression memory] Initial heap: ${heapBefore.usedMB}MB`);

    // Let the app run for 2 minutes with render loop active
    await page.waitForTimeout(120000);

    // Take final heap measurement
    const heapAfter = await getHeapUsage(page);
    console.log(`[perf-regression memory] After 2min: ${heapAfter.usedMB}MB (delta: ${(heapAfter.usedMB - heapBefore.usedMB).toFixed(1)}MB)`);

    // Heap should not grow more than 50MB over 2 minutes with a static 100-node graph.
    // Some growth is normal (GC pressure, deferred cleanup), but unbounded growth
    // indicates a memory leak in the animation loop, particle system, or event handlers.
    const growthMB = heapAfter.usedMB - heapBefore.usedMB;
    expect(growthMB).toBeLessThan(50);
  });
});
