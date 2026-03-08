// Label overlap quality gate (beads-o4ao).
// Verifies that the LOD label system prevents excessive text overlap.
// Uses mock API data so tests are deterministic.

import { test, expect } from '@playwright/test';
import { MOCK_GRAPH, MOCK_PING, MOCK_SHOW, MOCK_MULTI_AGENT_GRAPH } from './fixtures.js';

// Intercept all /api calls with mock data
async function mockAPI(page, graphData = MOCK_GRAPH) {
  await page.route('**/api/bd.v1.BeadsService/Ping', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) });
  });
  await page.route('**/api/bd.v1.BeadsService/Graph', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(graphData) });
  });
  await page.route('**/api/bd.v1.BeadsService/List', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
  });
  await page.route('**/api/bd.v1.BeadsService/Show', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_SHOW) });
  });
  await page.route('**/api/bd.v1.BeadsService/Stats', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(graphData.stats || MOCK_GRAPH.stats) });
  });
  await page.route('**/api/bd.v1.BeadsService/Blocked', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
  });
  await page.route('**/api/bd.v1.BeadsService/Ready', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
  });
  await page.route('**/api/bd.v1.BeadsService/Update', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) });
  });
  await page.route('**/api/bd.v1.BeadsService/Close', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) });
  });
  await page.route('**/api/events', async route => {
    await route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' });
  });
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

// Wait for graph to render and settle
async function waitForGraphReady(page) {
  await page.waitForSelector('#status.connected', { timeout: 15000 });
  await page.waitForTimeout(3000);
  // Zoom camera to fit nodes
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

// Extract visible label bounding boxes from Three.js scene (runs in browser context)
async function getVisibleLabelBounds(page) {
  return page.evaluate(() => {
    const b = window.__beads3d;
    if (!b || !b.graph) return [];
    const THREE = window.__THREE;
    const camera = b.graph.camera();
    const renderer = b.graph.renderer();
    if (!camera || !renderer) return [];

    const width = renderer.domElement.clientWidth;
    const height = renderer.domElement.clientHeight;
    if (width === 0 || height === 0) return [];

    const labels = [];
    const graphData = b.graph.graphData();
    for (const node of graphData.nodes) {
      const threeObj = node.__threeObj;
      if (!threeObj) continue;
      threeObj.traverse(child => {
        if (!child.userData || !child.userData.nodeLabel || !child.visible) return;
        // Get world position and project to screen
        const worldPos = new THREE.Vector3();
        child.getWorldPosition(worldPos);
        const ndc = worldPos.clone().project(camera);
        if (ndc.z > 1) return; // behind camera
        const sx = (ndc.x * 0.5 + 0.5) * width;
        const sy = (-ndc.y * 0.5 + 0.5) * height;
        const lw = child.scale.x * height;
        const lh = child.scale.y * height;
        labels.push({
          nodeId: node.id,
          left: sx - lw / 2,
          top: sy - lh / 2,
          right: sx + lw / 2,
          bottom: sy + lh / 2,
          width: lw,
          height: lh,
          opacity: child.material ? child.material.opacity : 1.0,
        });
      });
    }
    return labels;
  });
}

// Compute overlap ratio between two rectangles (0 = no overlap, 1 = complete overlap)
function overlapRatio(a, b) {
  const overlapX = Math.max(0, Math.min(a.right, b.right) - Math.max(a.left, b.left));
  const overlapY = Math.max(0, Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top));
  const overlapArea = overlapX * overlapY;
  if (overlapArea === 0) return 0;
  const smallerArea = Math.min(a.width * a.height, b.width * b.height);
  return smallerArea > 0 ? overlapArea / smallerArea : 0;
}

test.describe('label overlap quality gate (beads-o4ao)', () => {

  test('labels enabled — no pair exceeds 20% overlap (standard graph)', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Labels are ON by default (bd-oypa2) — wait for LOD pass
    await page.waitForTimeout(500);
    await forceRender(page);
    await page.waitForTimeout(200);

    const labels = await getVisibleLabelBounds(page);
    expect(labels.length).toBeGreaterThan(0);

    // Check all pairs for excessive overlap
    const overlaps = [];
    for (let i = 0; i < labels.length; i++) {
      for (let j = i + 1; j < labels.length; j++) {
        const ratio = overlapRatio(labels[i], labels[j]);
        if (ratio > 0.20) {
          overlaps.push({
            a: labels[i].nodeId,
            b: labels[j].nodeId,
            ratio: Math.round(ratio * 100),
          });
        }
      }
    }

    if (overlaps.length > 0) {
      const detail = overlaps.map(o => `${o.a} ↔ ${o.b}: ${o.ratio}%`).join(', ');
      expect(overlaps.length, `Label pairs with >20% overlap: ${detail}`).toBe(0);
    }
  });

  test('LOD budget limits visible labels on far zoom', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Labels are ON by default (bd-oypa2) — wait for LOD pass
    await page.waitForTimeout(500);

    // Zoom camera far out (simulate wide zoom)
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return;
      b.graph.cameraPosition({ x: 0, y: 200, z: 800 }, { x: 0, y: 0, z: 0 }, 0);
      const controls = b.graph.controls();
      if (controls) controls.update();
    });
    await forceRender(page);
    await page.waitForTimeout(500); // wait for LOD pass
    await forceRender(page);

    const labels = await getVisibleLabelBounds(page);
    const totalNodes = await page.evaluate(() => {
      const b = window.__beads3d;
      return b ? b.graph.graphData().nodes.filter(n => !n._hidden).length : 0;
    });

    // At far zoom, LOD should show fewer labels than total visible nodes
    // Budget formula: max(6, 40 * (100 / max(camDist, 50)))
    // At distance ~824: budget = max(6, 40 * 100/824) ≈ 6
    expect(labels.length).toBeLessThanOrEqual(Math.max(8, totalNodes));
    // At far zoom the budget should be significantly less than total
    if (totalNodes > 10) {
      expect(labels.length).toBeLessThan(totalNodes);
    }
  });

  test('selected node label always visible regardless of LOD budget', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Labels are ON by default (bd-oypa2)
    await page.waitForTimeout(500);

    // Zoom far out to minimize budget
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return;
      b.graph.cameraPosition({ x: 0, y: 500, z: 1500 }, { x: 0, y: 0, z: 0 }, 0);
      const controls = b.graph.controls();
      if (controls) controls.update();
    });
    await forceRender(page);
    await page.waitForTimeout(300);

    // Select a specific node
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.selectNode) return;
      const node = b.graph.graphData().nodes.find(n => n.id === 'bd-task5');
      if (node) b.selectNode(node);
    });
    await page.waitForTimeout(500);
    await forceRender(page);

    // The selected node's label should be visible (selection shows highlighted labels)
    const labels = await getVisibleLabelBounds(page);
    // When a node is selected, the animation loop shows labels for highlighted nodes
    // which includes the selected node and its connected component
    expect(labels.length).toBeGreaterThan(0);
    expect(labels.some(l => l.nodeId === 'bd-task5')).toBeTruthy();
  });

  test('multi-agent graph — labels readable at default zoom', async ({ page }) => {
    await mockAPI(page, MOCK_MULTI_AGENT_GRAPH);
    await page.goto('/');
    await waitForGraphReady(page);

    // Labels are ON by default (bd-oypa2) — wait for LOD pass
    await page.waitForTimeout(500);
    await forceRender(page);
    await page.waitForTimeout(200);

    const labels = await getVisibleLabelBounds(page);
    expect(labels.length).toBeGreaterThan(0);

    // Check overlap — allow up to 2 pairs with minor overlap in dense graphs
    let severeOverlaps = 0;
    for (let i = 0; i < labels.length; i++) {
      for (let j = i + 1; j < labels.length; j++) {
        const ratio = overlapRatio(labels[i], labels[j]);
        if (ratio > 0.20) severeOverlaps++;
      }
    }

    // In a dense multi-agent graph, LOD should prevent most overlaps
    // Allow at most 2 minor overlapping pairs (3D projection can still cause some)
    expect(severeOverlaps, `${severeOverlaps} label pairs have >20% overlap`).toBeLessThanOrEqual(2);
  });

  test('labels off — no visible labels after toggle', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Labels are ON by default (bd-oypa2) — press 'l' to turn them off
    await page.keyboard.press('l');
    await page.waitForTimeout(500);
    const labels = await getVisibleLabelBounds(page);
    expect(labels.length).toBe(0);
  });

  test('lower-priority labels have reduced opacity', async ({ page }) => {
    await mockAPI(page, MOCK_MULTI_AGENT_GRAPH);
    await page.goto('/');
    await waitForGraphReady(page);

    // Labels are ON by default (bd-oypa2) — wait for LOD pass
    await page.waitForTimeout(500);
    await forceRender(page);
    await page.waitForTimeout(200);

    const labels = await getVisibleLabelBounds(page);
    if (labels.length > 6) {
      // Some labels should have reduced opacity (the LOD fade)
      const fadedLabels = labels.filter(l => l.opacity < 1.0);
      // Not all labels should be full opacity when we have many
      expect(fadedLabels.length).toBeGreaterThan(0);
    }
  });
});
