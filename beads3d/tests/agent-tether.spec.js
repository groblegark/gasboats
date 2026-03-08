// E2E screenshot-based test for agent-pull tether physics (bd-tzw8d).
// Takes successive screenshots with tether off vs on, verifying that
// agents pull their assigned beads closer when tether is enabled.
//
// The test uses unfixed positions (no fx/fy/fz) so the d3 force simulation
// actually moves nodes. The agent-tether force should pull beads toward
// their assigned agent.

import { test, expect } from '@playwright/test';
import { MOCK_PING, MOCK_SHOW } from './fixtures.js';

// Graph with agents and assigned beads — NO fixed positions so forces work.
// Two agents each assigned 2 beads, with dependency edges between beads.
const TETHER_GRAPH = {
  nodes: [
    // Agent nodes
    { id: 'agent:alpha', title: 'alpha', status: 'open', priority: 2, issue_type: 'agent', assignee: '', created_at: '2026-02-19T00:00:00Z', updated_at: '2026-02-19T12:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
    { id: 'agent:beta', title: 'beta', status: 'open', priority: 2, issue_type: 'agent', assignee: '', created_at: '2026-02-19T00:00:00Z', updated_at: '2026-02-19T12:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
    // Beads assigned to alpha
    { id: 'bd-a1', title: 'Auth RPC layer', status: 'in_progress', priority: 1, issue_type: 'feature', assignee: 'alpha', created_at: '2026-02-10T10:00:00Z', updated_at: '2026-02-19T11:30:00Z', labels: ['auth'], dep_count: 1, dep_by_count: 0, blocked_by: [] },
    { id: 'bd-a2', title: 'Auth integration tests', status: 'open', priority: 2, issue_type: 'task', assignee: 'alpha', created_at: '2026-02-12T09:00:00Z', updated_at: '2026-02-19T11:00:00Z', labels: ['testing'], dep_count: 0, dep_by_count: 1, blocked_by: ['bd-a1'] },
    // Beads assigned to beta
    { id: 'bd-b1', title: 'Storage migration', status: 'in_progress', priority: 1, issue_type: 'task', assignee: 'beta', created_at: '2026-02-08T08:00:00Z', updated_at: '2026-02-19T10:45:00Z', labels: ['storage'], dep_count: 1, dep_by_count: 0, blocked_by: [] },
    { id: 'bd-b2', title: 'Replication health check', status: 'open', priority: 2, issue_type: 'task', assignee: 'beta', created_at: '2026-02-14T11:00:00Z', updated_at: '2026-02-19T09:30:00Z', labels: ['ops'], dep_count: 0, dep_by_count: 1, blocked_by: ['bd-b1'] },
    // Unassigned beads (should NOT be pulled)
    { id: 'bd-free1', title: 'Unassigned bug', status: 'open', priority: 1, issue_type: 'bug', assignee: '', created_at: '2026-02-18T20:00:00Z', updated_at: '2026-02-19T10:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
    { id: 'bd-free2', title: 'Unassigned task', status: 'open', priority: 3, issue_type: 'task', assignee: '', created_at: '2026-02-16T12:00:00Z', updated_at: '2026-02-19T06:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [] },
  ],
  edges: [
    // Agent assignments
    { source: 'agent:alpha', target: 'bd-a1', type: 'assigned_to' },
    { source: 'agent:alpha', target: 'bd-a2', type: 'assigned_to' },
    { source: 'agent:beta', target: 'bd-b1', type: 'assigned_to' },
    { source: 'agent:beta', target: 'bd-b2', type: 'assigned_to' },
    // Dependency edges (for BFS traversal depth testing)
    { source: 'bd-a1', target: 'bd-a2', type: 'blocks' },
    { source: 'bd-b1', target: 'bd-b2', type: 'blocks' },
  ],
  stats: { total_open: 6, total_in_progress: 2, total_blocked: 2, total_closed: 0 },
};

function mockAPI(page) {
  return Promise.all([
    page.route('**/api/bd.v1.BeadsService/Ping', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) })
    ),
    page.route('**/api/bd.v1.BeadsService/Graph', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(TETHER_GRAPH) })
    ),
    page.route('**/api/bd.v1.BeadsService/List', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) })
    ),
    page.route('**/api/bd.v1.BeadsService/Show', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_SHOW) })
    ),
    page.route('**/api/bd.v1.BeadsService/Stats', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(TETHER_GRAPH.stats) })
    ),
    page.route('**/api/bd.v1.BeadsService/Blocked', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) })
    ),
    page.route('**/api/bd.v1.BeadsService/Ready', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) })
    ),
    page.route('**/api/bd.v1.BeadsService/Update', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) })
    ),
    page.route('**/api/bd.v1.BeadsService/Close', route =>
      route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) })
    ),
    page.route('**/api/events', route =>
      route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' })
    ),
  ]);
}

// Force-render the Three.js scene
async function forceRender(page) {
  await page.evaluate(() => {
    const b = window.__beads3d;
    if (!b || !b.graph) return;
    const renderer = b.graph.renderer();
    const scene = b.graph.scene();
    const camera = b.graph.camera();
    if (renderer && scene && camera) {
      renderer.render(scene, camera);
      const composer = b.graph.postProcessingComposer();
      if (composer) composer.render();
    }
  });
}

// Wait for graph to render and force layout to settle
async function waitForGraphReady(page) {
  await page.waitForSelector('#status.connected', { timeout: 15000 });
  // Let force simulation settle
  await page.waitForTimeout(4000);
  // Zoom to fit all nodes
  await page.evaluate(() => {
    const b = window.__beads3d;
    if (!b || !b.graph) return;
    const nodes = b.graph.graphData().nodes.filter(n => n.x !== undefined);
    if (nodes.length === 0) return;
    let maxR = 0;
    for (const n of nodes) {
      const r = Math.sqrt((n.x || 0) ** 2 + (n.y || 0) ** 2 + (n.z || 0) ** 2);
      if (r > maxR) maxR = r;
    }
    const zoom = Math.max(maxR * 3, 120);
    b.graph.cameraPosition({ x: 0, y: zoom * 0.3, z: zoom }, { x: 0, y: 0, z: 0 }, 0);
    const controls = b.graph.controls();
    if (controls) controls.update();
  });
  await forceRender(page);
  await page.waitForTimeout(100);
  await forceRender(page);
  await page.waitForTimeout(200);
}

// Open the control panel if it's collapsed
async function openControlPanel(page) {
  const panel = page.locator('#control-panel');
  const isVisible = await panel.evaluate(el => !el.classList.contains('collapsed'));
  if (!isVisible) {
    await page.keyboard.press('c');
    await page.waitForTimeout(300);
  }
}

// Set the agent tether slider to a value (0-1) and wait for force to settle
async function setTetherStrength(page, value) {
  await openControlPanel(page);
  const slider = page.locator('#cp-agent-tether');
  await slider.fill(String(value));
  await slider.dispatchEvent('input');
  // Wait for force simulation to run and settle with new tether strength
  await page.waitForTimeout(3000);
  // Reheat simulation to ensure force changes propagate
  await page.evaluate(() => {
    const b = window.__beads3d;
    if (b && b.graph) {
      b.graph.d3ReheatSimulation();
    }
  });
  await page.waitForTimeout(3000);
  await forceRender(page);
  await page.waitForTimeout(200);
  await forceRender(page);
}

// Measure average distance between each agent and its assigned beads
async function measureAgentBeadDistances(page) {
  return page.evaluate(() => {
    const b = window.__beads3d;
    if (!b || !b.graph) return null;
    const data = b.graph.graphData();
    const nodeById = new Map();
    for (const n of data.nodes) nodeById.set(n.id, n);

    // Build agent→beads from assigned_to edges
    const agentBeads = new Map();
    for (const l of data.links) {
      const depType = l.dep_type || l.type;
      if (depType !== 'assigned_to') continue;
      const srcId = typeof l.source === 'object' ? l.source.id : l.source;
      const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      const agent = nodeById.get(srcId);
      const bead = nodeById.get(tgtId);
      if (!agent || !bead || agent.issue_type !== 'agent') continue;
      if (!agentBeads.has(srcId)) agentBeads.set(srcId, []);
      agentBeads.get(srcId).push(bead);
    }

    // Measure distances
    const results = {};
    let totalDist = 0;
    let count = 0;
    for (const [agentId, beads] of agentBeads) {
      const agent = nodeById.get(agentId);
      if (!agent || agent.x === undefined) continue;
      const dists = [];
      for (const bead of beads) {
        if (bead.x === undefined) continue;
        const dx = (agent.x || 0) - (bead.x || 0);
        const dy = (agent.y || 0) - (bead.y || 0);
        const dz = (agent.z || 0) - (bead.z || 0);
        const dist = Math.sqrt(dx * dx + dy * dy + dz * dz);
        dists.push(dist);
        totalDist += dist;
        count++;
      }
      results[agentId] = { avgDist: dists.reduce((a, b) => a + b, 0) / dists.length, dists };
    }
    results._avgAll = count > 0 ? totalDist / count : 0;
    return results;
  });
}

test.describe('agent-pull tether physics', () => {

  test('tether pulls assigned beads closer to agents', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Step 1: Set tether to 0 (off) — beads spread by normal forces only
    await setTetherStrength(page, 0);
    const distOff = await measureAgentBeadDistances(page);

    // Screenshot: tether off — beads should be spread out
    await expect(page).toHaveScreenshot('tether-off.png', {
      animations: 'disabled',
      maxDiffPixelRatio: 0.40,
    });

    // Step 2: Set tether to max (1.0) — agents should pull beads close
    await setTetherStrength(page, 1.0);
    const distOn = await measureAgentBeadDistances(page);

    // Screenshot: tether on — beads should cluster around agents
    await expect(page).toHaveScreenshot('tether-on.png', {
      animations: 'disabled',
      maxDiffPixelRatio: 0.40,
    });

    // Step 3: Verify clustering — average distance should decrease
    expect(distOff).toBeTruthy();
    expect(distOn).toBeTruthy();
    expect(distOff._avgAll).toBeGreaterThan(0);
    expect(distOn._avgAll).toBeGreaterThan(0);

    // Tether-on distances should be meaningfully less than tether-off
    const ratio = distOn._avgAll / distOff._avgAll;
    expect(ratio).toBeLessThan(0.9); // at least 10% closer with tether on
  });

  test('successive frames show convergence with tether enabled', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Enable tether at moderate strength
    await setTetherStrength(page, 0.7);

    // Take measurement immediately after enabling
    const distT0 = await measureAgentBeadDistances(page);

    // Screenshot: initial state after tether enabled
    await expect(page).toHaveScreenshot('tether-convergence-t0.png', {
      animations: 'disabled',
      maxDiffPixelRatio: 0.40,
    });

    // Reheat and let simulation run more
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (b && b.graph) b.graph.d3ReheatSimulation();
    });
    await page.waitForTimeout(4000);
    await forceRender(page);
    await forceRender(page);

    const distT1 = await measureAgentBeadDistances(page);

    // Screenshot: after more simulation time
    await expect(page).toHaveScreenshot('tether-convergence-t1.png', {
      animations: 'disabled',
      maxDiffPixelRatio: 0.40,
    });

    // Distances should either decrease or stay stable (not increase)
    expect(distT0).toBeTruthy();
    expect(distT1).toBeTruthy();
    expect(distT1._avgAll).toBeLessThanOrEqual(distT0._avgAll * 1.1); // allow 10% tolerance
  });

  test('unassigned beads are not pulled by agents', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Set tether to 0 — measure free positions
    await setTetherStrength(page, 0);
    const freeDistOff = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;
      const data = b.graph.graphData();
      const nodeById = new Map();
      for (const n of data.nodes) nodeById.set(n.id, n);

      // Measure distance from free beads to nearest agent
      const agents = data.nodes.filter(n => n.issue_type === 'agent' && n.x !== undefined);
      const free = ['bd-free1', 'bd-free2'].map(id => nodeById.get(id)).filter(n => n && n.x !== undefined);
      let totalDist = 0;
      for (const bead of free) {
        let minDist = Infinity;
        for (const agent of agents) {
          const dx = (agent.x || 0) - (bead.x || 0);
          const dy = (agent.y || 0) - (bead.y || 0);
          const dz = (agent.z || 0) - (bead.z || 0);
          const dist = Math.sqrt(dx * dx + dy * dy + dz * dz);
          if (dist < minDist) minDist = dist;
        }
        totalDist += minDist;
      }
      return totalDist / free.length;
    });

    // Set tether to max
    await setTetherStrength(page, 1.0);
    const freeDistOn = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;
      const data = b.graph.graphData();
      const nodeById = new Map();
      for (const n of data.nodes) nodeById.set(n.id, n);

      const agents = data.nodes.filter(n => n.issue_type === 'agent' && n.x !== undefined);
      const free = ['bd-free1', 'bd-free2'].map(id => nodeById.get(id)).filter(n => n && n.x !== undefined);
      let totalDist = 0;
      for (const bead of free) {
        let minDist = Infinity;
        for (const agent of agents) {
          const dx = (agent.x || 0) - (bead.x || 0);
          const dy = (agent.y || 0) - (bead.y || 0);
          const dz = (agent.z || 0) - (bead.z || 0);
          const dist = Math.sqrt(dx * dx + dy * dy + dz * dz);
          if (dist < minDist) minDist = dist;
        }
        totalDist += minDist;
      }
      return totalDist / free.length;
    });

    // Unassigned beads should NOT be pulled significantly closer
    // Allow some movement from other forces, but no strong pull
    expect(freeDistOff).toBeGreaterThan(0);
    expect(freeDistOn).toBeGreaterThan(0);
    // Free beads should not cluster toward agents (ratio should be close to 1.0)
    const ratio = freeDistOn / freeDistOff;
    expect(ratio).toBeGreaterThan(0.5); // not pulled more than 50% closer
  });

  test('tether strength slider produces proportional clustering', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Measure at three strength levels: 0.2, 0.5, 1.0
    const distances = [];
    for (const strength of [0.2, 0.5, 1.0]) {
      await setTetherStrength(page, strength);
      const dist = await measureAgentBeadDistances(page);
      distances.push({ strength, avgDist: dist._avgAll });
    }

    // Higher strength should produce tighter clustering (smaller distances)
    // Allow some noise but general trend should hold
    expect(distances[0].avgDist).toBeGreaterThan(0);
    expect(distances[2].avgDist).toBeLessThan(distances[0].avgDist); // 1.0 < 0.2
  });
});
