// E2E tests for connected-component subgraph highlighting and camera (beads-cfee).
// Verifies: click highlights full component, camera frames subgraph, labels visible
// and non-overlapping in the final view.
//
// Run: npx playwright test tests/subgraph-highlight.spec.js
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

test.describe('subgraph highlight and camera (beads-cfee)', () => {

  test('clicking a node highlights the full connected component', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Click bd-feat1 — part of Epic1 cluster
    await page.evaluate(() => {
      const b = window.__beads3d;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-feat1');
      b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });
    await page.waitForTimeout(1000);

    // Check that the full connected component is in highlightNodes
    const result = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return null;
      const hl = b.highlightNodes();
      return {
        highlighted: [...hl],
        size: hl.size,
      };
    });

    expect(result).not.toBeNull();
    expect(result.size).toBeGreaterThan(3); // at least the Epic1 cluster

    // Epic1 cluster: bd-epic1, bd-feat1, bd-feat2, bd-task1, bd-task2, bd-bug1
    // Plus agents connected: agent:alice, agent:bob (via assigned_to edges)
    // And bd-bug2 (agent:alice → bd-bug2)
    const expectedHighlighted = ['bd-epic1', 'bd-feat1', 'bd-feat2', 'bd-task1', 'bd-task2', 'bd-bug1'];
    for (const id of expectedHighlighted) {
      expect(result.highlighted, `${id} should be in highlightNodes`).toContain(id);
    }

    // Isolated nodes should NOT be highlighted
    expect(result.highlighted).not.toContain('bd-task5');
    expect(result.highlighted).not.toContain('bd-feat3');
  });

  test('camera moves to frame the highlighted subgraph', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const camBefore = await getCameraPos(page);

    // Click bd-task3 (part of Epic2 cluster on the right side)
    await page.evaluate(() => {
      const b = window.__beads3d;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-task3');
      b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });

    // Wait for camera animation (1000ms)
    await page.waitForTimeout(2000);

    const camAfter = await getCameraPos(page);

    // Camera should have moved significantly toward the Epic2 cluster
    expect(dist3d(camBefore, camAfter)).toBeGreaterThan(20);
  });

  test('highlighted labels are visible after click', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Click bd-feat1 to select Epic1 component
    await page.evaluate(() => {
      const b = window.__beads3d;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-feat1');
      b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });

    // Wait for animation loop to set label visibility on highlighted nodes.
    // Use waitForFunction instead of fixed timeout — rAF may not fire reliably
    // in SwiftShader headless mode (bd-hh4s7).
    await page.waitForFunction(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return false;
      const hl = b.highlightNodes();
      const nodes = b.graph.graphData().nodes;
      for (const node of nodes) {
        if (!hl.has(node.id)) continue;
        const obj = node.__threeObj;
        if (!obj) continue;
        let found = false;
        obj.traverse(child => {
          if (child.userData.nodeLabel && child.visible) found = true;
        });
        if (found) return true;
      }
      return false;
    }, { timeout: 10000 });

    // Now collect full label info
    const labelInfo = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;
      const hl = b.highlightNodes();
      const nodes = b.graph.graphData().nodes;
      const results = { highlighted: [], visible: [] };
      for (const node of nodes) {
        if (!hl.has(node.id)) continue;
        results.highlighted.push(node.id);
        const obj = node.__threeObj;
        if (!obj) continue;
        obj.traverse(child => {
          if (child.userData.nodeLabel && child.visible) {
            results.visible.push(node.id);
          }
        });
      }
      return results;
    });

    expect(labelInfo).not.toBeNull();
    expect(labelInfo.highlighted.length).toBeGreaterThan(3);
    // At least the selected node and some neighbors should have visible labels
    // (LOD system may not show all labels, but highlighted nodes get priority boost)
    expect(labelInfo.visible.length).toBeGreaterThan(0);
  });

  test('label overlap area is bounded after subgraph highlight', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Click bd-feat1 to trigger component highlight + spread
    await page.evaluate(() => {
      const b = window.__beads3d;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-feat1');
      b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });

    // Wait for spread force (2.5s) + camera animation + LOD pass
    await page.waitForTimeout(4000);
    await forceRender(page);
    await page.waitForTimeout(500);

    // Project all visible labels to screen space and measure overlap
    const overlapResult = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return { error: 'no graph' };

      const camera = b.graph.camera();
      const renderer = b.graph.renderer();
      const width = renderer.domElement.clientWidth;
      const height = renderer.domElement.clientHeight;
      const THREE = window.__THREE;

      const rects = [];
      for (const node of b.graph.graphData().nodes) {
        const obj = node.__threeObj;
        if (!obj) continue;
        obj.traverse(child => {
          if (!child.userData.nodeLabel || !child.visible) return;
          const worldPos = new THREE.Vector3();
          child.getWorldPosition(worldPos);
          const ndc = worldPos.clone().project(camera);
          if (ndc.z > 1) return;
          const sx = (ndc.x * 0.5 + 0.5) * width;
          const sy = (-ndc.y * 0.5 + 0.5) * height;
          const lw = child.scale.x * height;
          const lh = child.scale.y * height;
          rects.push({
            id: node.id,
            left: sx - lw / 2,
            right: sx + lw / 2,
            top: sy - lh / 2,
            bottom: sy + lh / 2,
          });
        });
      }

      // Check pairwise overlaps
      let totalOverlapArea = 0;
      const overlaps = [];
      for (let i = 0; i < rects.length; i++) {
        for (let j = i + 1; j < rects.length; j++) {
          const a = rects[i], b = rects[j];
          const overlapX = Math.max(0, Math.min(a.right, b.right) - Math.max(a.left, b.left));
          const overlapY = Math.max(0, Math.min(a.bottom, b.bottom) - Math.max(a.top, b.top));
          const area = overlapX * overlapY;
          if (area > 0) {
            totalOverlapArea += area;
            overlaps.push({ a: a.id, b: b.id, area });
          }
        }
      }

      return { labelCount: rects.length, overlaps, totalOverlapArea };
    });

    // Quality gate: total overlap should be bounded.
    // Pairwise spread force (beads-8g56) targets < 3000px².
    // Perfect = 0, but force-directed layout with fixed positions means some residual overlap.
    console.log(`[quality-gate] labels=${overlapResult.labelCount} totalOverlap=${Math.round(overlapResult.totalOverlapArea)}px² pairs=${overlapResult.overlaps.length}`);
    expect(overlapResult.totalOverlapArea).toBeLessThan(3000);
  });

  test('deep-link URL highlights full subgraph', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/?bead=bd-task3');
    await waitForGraph(page);
    await page.waitForTimeout(2000);

    // Verify bd-task3's component is highlighted
    const result = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return null;
      const hl = b.highlightNodes();
      return [...hl];
    });

    expect(result).not.toBeNull();
    // Epic2 cluster: bd-task3, bd-epic2, bd-task4, plus bd-feat3 and bd-old2
    expect(result).toContain('bd-task3');
    expect(result).toContain('bd-epic2');
    expect(result).toContain('bd-task4');
  });

  test('clearing selection removes spread force and restores view', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Click to select
    await page.evaluate(() => {
      const b = window.__beads3d;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-feat1');
      b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });
    await page.waitForTimeout(2000);

    // Verify selection is active
    const hlBefore = await page.evaluate(() => window.__beads3d.highlightNodes().size);
    expect(hlBefore).toBeGreaterThan(0);

    // Click background to clear
    await page.evaluate(() => {
      const b = window.__beads3d;
      b.graph.onBackgroundClick()();
    });
    await page.waitForTimeout(1000);

    // Verify spread force is removed
    const hasSpread = await page.evaluate(() => {
      const b = window.__beads3d;
      return b.graph.d3Force('subgraphSpread') !== undefined &&
             b.graph.d3Force('subgraphSpread') !== null;
    });
    expect(hasSpread).toBe(false);

    // Verify highlight is cleared
    const hlAfter = await page.evaluate(() => window.__beads3d.highlightNodes().size);
    expect(hlAfter).toBe(0);
  });

  test('visual: subgraph highlight screenshot', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Click bd-feat1 for Epic1 component
    await page.evaluate(() => {
      const b = window.__beads3d;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-feat1');
      b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });
    await page.waitForTimeout(3000);
    await forceRender(page);
    await forceRender(page);

    await expect(page).toHaveScreenshot('subgraph-highlight-epic1.png', {
      animations: 'disabled',
      maxDiffPixelRatio: 0.45,
    });
  });
});
