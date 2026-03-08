// Performance baseline tests for beads3d (bd-ak9s9)
// Measures FPS, frame times (p50/p95/p99), memory, and render cost at various node counts.
//
// Run: npx playwright test tests/perf-baselines.spec.js
// Output: perf-baseline.md in project root with tabulated results.

import { test, expect } from '@playwright/test';
import { generateGraph, MOCK_PING, MOCK_SHOW } from './fixtures.js';
import * as fs from 'fs';
import * as path from 'path';

const NODE_COUNTS = [100, 500, 1000, 5000];
const WARMUP_MS = 4000;  // let force layout settle
const MEASURE_MS = 5000; // collect frames for 5 seconds
const OUTPUT_FILE = path.join(process.cwd(), 'perf-baseline.md');
const RESULTS_FILE = path.join(process.cwd(), 'test-results', 'perf-results.jsonl');

// Intercept all /api calls with synthetic graph data
async function mockAPI(page, graphData) {
  await page.route('**/api/bd.v1.BeadsService/Ping', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) }));
  await page.route('**/api/bd.v1.BeadsService/Graph', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(graphData) }));
  await page.route('**/api/bd.v1.BeadsService/List', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
  await page.route('**/api/bd.v1.BeadsService/Show', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_SHOW) }));
  await page.route('**/api/bd.v1.BeadsService/Stats', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(graphData.stats) }));
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

// Collect frame times using performance.now() polling (not rAF monkey-patch)
// This runs a dedicated rAF loop that measures frame-to-frame timing
async function startCollection(page) {
  await page.evaluate(() => {
    window.__perfFrameTimes = [];
    window.__perfCollecting = true;
    window.__perfLastTime = performance.now();
    function perfTick() {
      if (!window.__perfCollecting) return;
      const now = performance.now();
      window.__perfFrameTimes.push(now - window.__perfLastTime);
      window.__perfLastTime = now;
      requestAnimationFrame(perfTick);
    }
    requestAnimationFrame(perfTick);
  });
}

// Stop collecting and return results
async function stopCollection(page) {
  return page.evaluate(() => {
    window.__perfCollecting = false;
    const times = window.__perfFrameTimes.slice();
    // Drop first frame (startup spike)
    if (times.length > 1) times.shift();
    const sorted = [...times].sort((a, b) => a - b);
    const len = sorted.length;
    if (len === 0) return null;

    const sum = sorted.reduce((a, b) => a + b, 0);
    const avgMs = sum / len;
    const avgFps = 1000 / avgMs;
    const p50 = sorted[Math.floor(len * 0.5)];
    const p95 = sorted[Math.floor(len * 0.95)];
    const p99 = sorted[Math.floor(len * 0.99)];
    const minMs = sorted[0];
    const maxMs = sorted[len - 1];

    // Memory (Chrome only)
    const mem = performance.memory;
    const heapUsedMB = mem ? (mem.usedJSHeapSize / 1048576).toFixed(1) : 'N/A';
    const heapTotalMB = mem ? (mem.totalJSHeapSize / 1048576).toFixed(1) : 'N/A';

    // Object counts from beads3d internals
    const b = window.__beads3d;
    let nodeCount = 0, linkCount = 0;
    if (b && b.graph) {
      const gd = b.graph.graphData();
      nodeCount = gd.nodes.length;
      linkCount = gd.links.length;
    }

    return {
      frameCount: len,
      avgFps: Math.round(avgFps * 10) / 10,
      avgMs: Math.round(avgMs * 100) / 100,
      p50: Math.round(p50 * 100) / 100,
      p95: Math.round(p95 * 100) / 100,
      p99: Math.round(p99 * 100) / 100,
      minMs: Math.round(minMs * 100) / 100,
      maxMs: Math.round(maxMs * 100) / 100,
      heapUsedMB,
      heapTotalMB,
      nodeCount,
      linkCount,
    };
  });
}

// Wait for graph to load
async function waitForGraph(page) {
  await page.waitForSelector('#status', { timeout: 15000 });
  try {
    await page.waitForSelector('#status.connected', { timeout: 10000 });
  } catch {
    // May not get connected state with mock SSE, proceed anyway
  }
}

// Ensure results directory and clear previous results
test.beforeAll(async () => {
  const dir = path.dirname(RESULTS_FILE);
  if (!fs.existsSync(dir)) fs.mkdirSync(dir, { recursive: true });
  if (fs.existsSync(RESULTS_FILE)) fs.unlinkSync(RESULTS_FILE);
});

test.describe.serial('Performance baselines', () => {
  for (const nodeCount of NODE_COUNTS) {
    test(`${nodeCount} nodes`, async ({ page }) => {
      test.setTimeout(90000);

      const agentCount = Math.min(Math.floor(nodeCount * 0.1), 20);
      const epicCount = Math.max(2, Math.floor(nodeCount / 50));
      const graphData = generateGraph({
        seed: nodeCount,
        nodeCount,
        agentCount,
        epicCount,
        fixedPositions: false, // let force layout run — more realistic
      });

      await mockAPI(page, graphData);
      await page.goto('/');
      await waitForGraph(page);

      // Warmup: let force layout settle
      await page.waitForTimeout(WARMUP_MS);

      // Measure steady-state performance
      await startCollection(page);
      await page.waitForTimeout(MEASURE_MS);
      const results = await stopCollection(page);

      expect(results).not.toBeNull();
      results.targetNodes = nodeCount;

      // Append to JSONL file for afterAll to read
      fs.appendFileSync(RESULTS_FILE, JSON.stringify(results) + '\n');

      // Basic sanity — SwiftShader is 5-10x slower than real GPU, so only check frame count
      expect(results.frameCount).toBeGreaterThan(20);
    });
  }
});

test.afterAll(async () => {
  if (!fs.existsSync(RESULTS_FILE)) return;
  const allResults = fs.readFileSync(RESULTS_FILE, 'utf8')
    .split('\n')
    .filter(Boolean)
    .map(line => JSON.parse(line));

  if (allResults.length === 0) return;

  const lines = [
    '# beads3d Performance Baselines',
    '',
    `Generated: ${new Date().toISOString().slice(0, 19)}Z`,
    `Environment: Playwright Chromium (SwiftShader), viewport 1280x800`,
    `Warmup: ${WARMUP_MS}ms, Measurement: ${MEASURE_MS}ms`,
    '',
    '## Results',
    '',
    '| Nodes | Links | Avg FPS | Avg ms | p50 ms | p95 ms | p99 ms | Min ms | Max ms | Heap MB | Frames |',
    '|------:|------:|--------:|-------:|-------:|-------:|-------:|-------:|-------:|--------:|-------:|',
  ];

  for (const r of allResults.sort((a, b) => a.targetNodes - b.targetNodes)) {
    lines.push(
      `| ${r.targetNodes} | ${r.linkCount} | ${r.avgFps} | ${r.avgMs} | ${r.p50} | ${r.p95} | ${r.p99} | ${r.minMs} | ${r.maxMs} | ${r.heapUsedMB} | ${r.frameCount} |`
    );
  }

  lines.push('');
  lines.push('## Interpretation');
  lines.push('');
  lines.push('- **Target**: 30+ FPS at 500 nodes, 15+ FPS at 1000 nodes');
  lines.push('- **p95**: Frame time below 33ms means smooth at 30fps');
  lines.push('- **p99**: Occasional spikes above 50ms are acceptable (GC, layout settling)');
  lines.push('- **SwiftShader**: Software renderer — real GPU will be 2-4x faster');
  lines.push('- **Heap**: Watch for linear growth with node count (indicates memory leak)');
  lines.push('');

  const report = lines.join('\n') + '\n';
  fs.writeFileSync(OUTPUT_FILE, report);
  console.log(`\nPerformance report written to: ${OUTPUT_FILE}\n`);
  console.log(report);
});
