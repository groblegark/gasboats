// E2E tests for molecule focus view (bd-lwut6).
// Verifies: ?molecule=<id> deep-link, connected component highlight,
// camera framing, all labels visible in the molecule, and screenshot.
//
// Run: npx playwright test tests/molecule-focus.spec.js
// View report: npx playwright show-report test-results/html-report

import { test, expect } from '@playwright/test';
import { MOCK_PING, MOCK_SHOW } from './fixtures.js';

// Mock graph with a molecule node connected to several beads.
// The molecule acts as a grouping node — clicking it or deep-linking
// should reveal its entire connected component with all labels visible.
const MOCK_MOLECULE_GRAPH = {
  nodes: [
    // --- Molecule cluster ---
    { id: 'mol-auth', title: 'Auth subsystem', status: 'open', priority: 2, issue_type: 'molecule', assignee: '', created_at: '2026-02-15T10:00:00Z', updated_at: '2026-02-19T12:00:00Z', labels: ['auth'], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 0, fy: 0, fz: 0 },
    { id: 'bd-oauth', title: 'Implement OAuth flow', status: 'in_progress', priority: 1, issue_type: 'feature', assignee: 'alice', created_at: '2026-02-10T09:00:00Z', updated_at: '2026-02-19T11:00:00Z', labels: ['auth'], dep_count: 1, dep_by_count: 0, blocked_by: [], fx: -60, fy: -50, fz: 0 },
    { id: 'bd-token', title: 'Token refresh logic', status: 'open', priority: 2, issue_type: 'task', assignee: 'bob', created_at: '2026-02-12T08:00:00Z', updated_at: '2026-02-19T10:00:00Z', labels: ['auth'], dep_count: 0, dep_by_count: 1, blocked_by: ['bd-oauth'], fx: -60, fy: 50, fz: 0 },
    { id: 'bd-session', title: 'Session management', status: 'open', priority: 2, issue_type: 'task', assignee: '', created_at: '2026-02-13T10:00:00Z', updated_at: '2026-02-19T09:00:00Z', labels: ['auth'], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 60, fy: -50, fz: 0 },
    { id: 'bd-rbac', title: 'Role-based access control', status: 'open', priority: 1, issue_type: 'feature', assignee: '', created_at: '2026-02-14T11:00:00Z', updated_at: '2026-02-19T08:00:00Z', labels: ['auth'], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 60, fy: 50, fz: 0 },
    // --- Isolated nodes (should NOT be in molecule component) ---
    { id: 'bd-infra', title: 'Infrastructure cleanup', status: 'open', priority: 3, issue_type: 'task', assignee: '', created_at: '2026-02-16T12:00:00Z', updated_at: '2026-02-19T07:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: 200, fy: 0, fz: 0 },
    { id: 'bd-docs', title: 'Update API docs', status: 'open', priority: 4, issue_type: 'task', assignee: '', created_at: '2026-02-17T09:00:00Z', updated_at: '2026-02-19T06:00:00Z', labels: [], dep_count: 0, dep_by_count: 0, blocked_by: [], fx: -200, fy: 0, fz: 0 },
  ],
  edges: [
    // Molecule → beads (parent-child or blocks)
    { source: 'mol-auth', target: 'bd-oauth', type: 'parent-child' },
    { source: 'mol-auth', target: 'bd-token', type: 'parent-child' },
    { source: 'mol-auth', target: 'bd-session', type: 'parent-child' },
    { source: 'mol-auth', target: 'bd-rbac', type: 'parent-child' },
    // Intra-molecule dependency
    { source: 'bd-oauth', target: 'bd-token', type: 'blocks' },
  ],
  stats: {
    total_open: 5,
    total_in_progress: 1,
    total_blocked: 1,
    total_closed: 0,
  },
};

async function mockAPI(page) {
  await page.route('**/api/bd.v1.BeadsService/Ping', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) }));
  await page.route('**/api/bd.v1.BeadsService/Graph', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_MOLECULE_GRAPH) }));
  await page.route('**/api/bd.v1.BeadsService/List', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: '[]' }));
  await page.route('**/api/bd.v1.BeadsService/Show', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_SHOW) }));
  await page.route('**/api/bd.v1.BeadsService/Stats', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_MOLECULE_GRAPH.stats) }));
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

async function getCameraPos(page) {
  return page.evaluate(() => {
    const b = window.__beads3d;
    if (!b || !b.graph) return null;
    const cam = b.graph.camera();
    return { x: cam.position.x, y: cam.position.y, z: cam.position.z };
  });
}

function dist3d(a, b) {
  return Math.sqrt((a.x - b.x) ** 2 + (a.y - b.y) ** 2 + (a.z - b.z) ** 2);
}

test.describe('molecule focus view (bd-lwut6)', () => {

  test('?molecule=<id> deep-link highlights full connected component', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/?molecule=mol-auth');
    await waitForGraph(page);
    // Wait for focusMolecule (2s delay) + animation
    await page.waitForTimeout(4000);

    const result = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return null;
      const hl = b.highlightNodes();
      const focused = b.focusedMoleculeNodes();
      return {
        highlighted: [...hl],
        focused: [...focused],
        selectedId: b.selectedNode?.id,
      };
    });

    expect(result).not.toBeNull();
    // The molecule + its 4 connected beads + synthesized agent:alice should all be highlighted.
    // agent:alice is synthesized because bd-oauth has assignee:'alice' and status:'in_progress'.
    expect(result.highlighted).toContain('mol-auth');
    expect(result.highlighted).toContain('bd-oauth');
    expect(result.highlighted).toContain('bd-token');
    expect(result.highlighted).toContain('bd-session');
    expect(result.highlighted).toContain('bd-rbac');

    // Focused molecule nodes: 5 beads + 1 synthesized agent = 6
    expect(result.focused).toContain('mol-auth');
    expect(result.focused.length).toBe(6);

    // Isolated nodes should NOT be highlighted
    expect(result.highlighted).not.toContain('bd-infra');
    expect(result.highlighted).not.toContain('bd-docs');

    // Selected node should be the molecule itself
    expect(result.selectedId).toBe('mol-auth');
  });

  test('camera moves to frame the molecule subgraph', async ({ page }) => {
    await mockAPI(page);
    // Use ?molecule= so molecules are in graph, but capture camera before auto-focus
    await page.goto('/?molecule=mol-auth');
    await page.waitForSelector('#status.connected', { timeout: 15000 });
    // Capture camera right after graph loads but before the 2s focusMolecule fires
    await page.waitForTimeout(500);
    const camBefore = await getCameraPos(page);

    // Wait for auto-focus animation to complete (2s delay + 1s animation)
    await page.waitForTimeout(4000);

    const camAfter = await getCameraPos(page);

    // Camera should have moved from initial position toward the molecule cluster
    // Initial camera is at {0,0,400}, molecule cluster is near origin
    expect(dist3d(camBefore, camAfter)).toBeGreaterThan(20);
  });

  test('all labels in molecule are visible after focus', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/?molecule=mol-auth');
    await waitForGraph(page);
    // Wait for focusMolecule (2s delay) + animation, then poll for label visibility
    // instead of fixed timeout — rAF may not fire reliably in SwiftShader (bd-hh4s7).
    await page.waitForFunction(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return false;
      const focused = b.focusedMoleculeNodes();
      if (focused.size === 0) return false;
      const nodes = b.graph.graphData().nodes;
      let visCount = 0;
      for (const node of nodes) {
        if (!focused.has(node.id)) continue;
        const obj = node.__threeObj;
        if (!obj) continue;
        obj.traverse(child => {
          if (child.userData.nodeLabel && child.visible) visCount++;
        });
      }
      return visCount >= 6;
    }, { timeout: 15000 });

    const labelInfo = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;

      const focused = b.focusedMoleculeNodes();
      const nodes = b.graph.graphData().nodes;
      const results = { moleculeNodes: [], visibleLabels: [], hiddenLabels: [] };

      for (const node of nodes) {
        if (!focused.has(node.id)) continue;
        results.moleculeNodes.push(node.id);
        const obj = node.__threeObj;
        if (!obj) continue;
        obj.traverse(child => {
          if (!child.userData.nodeLabel) return;
          if (child.visible) {
            results.visibleLabels.push(node.id);
          } else {
            results.hiddenLabels.push(node.id);
          }
        });
      }
      return results;
    });

    expect(labelInfo).not.toBeNull();
    // 5 beads + 1 synthesized agent:alice = 6 molecule nodes
    expect(labelInfo.moleculeNodes.length).toBe(6);
    // ALL molecule labels should be visible (the key feature)
    expect(labelInfo.visibleLabels.length).toBe(6);
    expect(labelInfo.hiddenLabels.length).toBe(0);
  });

  test('programmatic focusMolecule works', async ({ page }) => {
    await mockAPI(page);
    // Navigate with ?molecule= so molecules are included in the graph query
    await page.goto('/?molecule=mol-auth');
    await waitForGraph(page);
    // Wait for initial auto-focus to complete, then clear and re-focus programmatically
    await page.waitForTimeout(3000);
    await page.evaluate(() => window.__beads3d.clearSelection());
    await page.waitForTimeout(500);

    // Call focusMolecule programmatically
    await page.evaluate(() => {
      window.__beads3d.focusMolecule('mol-auth');
    });
    await page.waitForTimeout(2000);

    const result = await page.evaluate(() => {
      const b = window.__beads3d;
      return {
        focusedSize: b.focusedMoleculeNodes().size,
        highlightSize: b.highlightNodes().size,
        selectedId: b.selectedNode?.id,
      };
    });

    expect(result.focusedSize).toBe(6); // 5 beads + agent:alice
    expect(result.highlightSize).toBeGreaterThanOrEqual(6);
    expect(result.selectedId).toBe('mol-auth');
  });

  test('clearing selection clears molecule focus', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/?molecule=mol-auth');
    await waitForGraph(page);
    await page.waitForTimeout(4000);

    // Verify focus is active
    const before = await page.evaluate(() => window.__beads3d.focusedMoleculeNodes().size);
    expect(before).toBe(6); // 5 beads + agent:alice

    // Clear selection
    await page.evaluate(() => window.__beads3d.clearSelection());
    await page.waitForTimeout(500);

    const after = await page.evaluate(() => ({
      focusedSize: window.__beads3d.focusedMoleculeNodes().size,
      highlightSize: window.__beads3d.highlightNodes().size,
    }));

    expect(after.focusedSize).toBe(0);
    expect(after.highlightSize).toBe(0);

    // URL should no longer have molecule param
    const url = await page.evaluate(() => window.location.search);
    expect(url).not.toContain('molecule=');
  });

  test('visual: molecule focus screenshot', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/?molecule=mol-auth');
    await waitForGraph(page);
    await page.waitForTimeout(4000);
    await forceRender(page);
    await forceRender(page);

    await expect(page).toHaveScreenshot('molecule-focus-auth.png', {
      animations: 'disabled',
      maxDiffPixelRatio: 0.45,
    });
  });
});
