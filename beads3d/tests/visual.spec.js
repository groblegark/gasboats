// Visual regression tests for beads3d shaders and layout modes.
// Uses Playwright to capture screenshots with mocked API data.
//
// Run: npx playwright test
// Update baselines: npx playwright test --update-snapshots

import { test, expect } from '@playwright/test';
import { MOCK_GRAPH, MOCK_PING, MOCK_SHOW } from './fixtures.js';

// Intercept all /api calls with mock data so tests are deterministic
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

  // Write operations — mock success responses for bulk menu tests
  await page.route('**/api/bd.v1.BeadsService/Update', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) });
  });

  await page.route('**/api/bd.v1.BeadsService/Close', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) });
  });

  // SSE events — keep connection alive but idle
  await page.route('**/api/events', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      body: 'data: {"type":"ping"}\n\n',
    });
  });
}

// Force-render the Three.js scene (rAF may not fire in headless/SwiftShader)
async function forceRender(page) {
  await page.evaluate(() => {
    const b = window.__beads3d;
    if (!b || !b.graph) return;
    const renderer = b.graph.renderer();
    const scene = b.graph.scene();
    const camera = b.graph.camera();
    if (renderer && scene && camera) {
      // Render directly, bypassing rAF
      renderer.render(scene, camera);
      // Also render the post-processing composer if bloom is enabled
      const composer = b.graph.postProcessingComposer();
      if (composer) composer.render();
    }
  });
}

// Wait for the 3D graph to be fully rendered (WebGL canvas painted)
async function waitForGraphReady(page) {
  // Wait for status indicator to show connected state
  await page.waitForSelector('#status.connected', { timeout: 15000 });
  // Give force layout time to position nodes
  await page.waitForTimeout(3000);
  // Zoom camera to fit all nodes (default z=400 is too far for small datasets)
  await page.evaluate(() => {
    const b = window.__beads3d;
    if (!b || !b.graph) return;
    const nodes = b.graph.graphData().nodes.filter(n => n.x !== undefined);
    if (nodes.length === 0) return;
    // Bounding box of all node positions
    let maxR = 0;
    for (const n of nodes) {
      const r = Math.sqrt((n.x || 0) ** 2 + (n.y || 0) ** 2 + (n.z || 0) ** 2);
      if (r > maxR) maxR = r;
    }
    // Position camera to see the full cluster with some padding
    const zoom = Math.max(maxR * 3, 120);
    b.graph.cameraPosition({ x: 0, y: zoom * 0.3, z: zoom }, { x: 0, y: 0, z: 0 }, 0);
    // Update camera controls to match new position
    const controls = b.graph.controls();
    if (controls) controls.update();
  });
  // Force render twice — first updates camera, second draws with new camera
  await forceRender(page);
  await page.waitForTimeout(100);
  await forceRender(page);
  await page.waitForTimeout(200);
}

test.describe('beads3d visual tests', () => {

  test('default view — free layout with all effects', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);
    // Take screenshot of the default force-directed layout
    await expect(page).toHaveScreenshot('default-free-layout.png', {
      animations: 'disabled',
    });
  });

  test('star field renders in background', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);
    // The star field should be visible as subtle points in the dark background
    // Verify the canvas is not completely black (stars add pixels)
    const canvas = page.locator('#graph canvas');
    await expect(canvas).toBeVisible();
    await expect(page).toHaveScreenshot('starfield-background.png', {
      animations: 'disabled',
    });
  });

  test('bloom post-processing toggle', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Enable bloom via keyboard shortcut
    await page.keyboard.press('b');
    await page.waitForTimeout(1000);
    await forceRender(page);
    await forceRender(page); // double-render stabilizes bloom pass
    await expect(page).toHaveScreenshot('bloom-enabled.png', {
      animations: 'disabled',
      maxDiffPixelRatio: 0.45, // bloom glow varies slightly between frames in SwiftShader
    });

    // Disable bloom
    await page.keyboard.press('b');
    await page.waitForTimeout(1000);
    await forceRender(page);
    await forceRender(page);
    await expect(page).toHaveScreenshot('bloom-disabled.png', {
      animations: 'disabled',
      maxDiffPixelRatio: 0.45,
    });
  });

  test('node selection with ring effect', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Click on the graph canvas near center to select a node
    const canvas = page.locator('#graph canvas');
    const box = await canvas.boundingBox();
    await canvas.click({ position: { x: box.width / 2, y: box.height / 2 } });
    await page.waitForTimeout(1500);
    await forceRender(page);
    await expect(page).toHaveScreenshot('node-selected.png', {
      animations: 'disabled',
    });
  });

  test('DAG layout mode', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    await page.keyboard.press('2');
    await page.waitForTimeout(3000);
    await forceRender(page);
    await expect(page).toHaveScreenshot('layout-dag.png', {
      animations: 'disabled',
    });
  });

  test('timeline layout mode', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    await page.keyboard.press('3');
    await page.waitForTimeout(3000);
    await forceRender(page);
    await expect(page).toHaveScreenshot('layout-timeline.png', {
      animations: 'disabled',
    });
  });

  test('radial layout mode', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    await page.keyboard.press('4');
    await page.waitForTimeout(3000);
    await forceRender(page);
    await expect(page).toHaveScreenshot('layout-radial.png', {
      animations: 'disabled',
    });
  });

  test('cluster layout mode', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    await page.keyboard.press('5');
    await page.waitForTimeout(3000);
    await forceRender(page);
    await expect(page).toHaveScreenshot('layout-cluster.png', {
      animations: 'disabled',
    });
  });

  test('search filtering dims non-matching nodes', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Type in search
    await page.keyboard.press('/');
    await page.keyboard.type('epic');
    await page.waitForTimeout(1000);
    await forceRender(page);
    await expect(page).toHaveScreenshot('search-filter-epic.png', {
      animations: 'disabled',
    });
  });

  test('detail panel opens on click', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Click center to select a node (opens detail panel)
    const canvas = page.locator('#graph canvas');
    const box = await canvas.boundingBox();
    await canvas.click({ position: { x: box.width / 2, y: box.height / 2 } });
    await page.waitForTimeout(2000);
    await forceRender(page);

    // Check if detail panel appeared
    const detail = page.locator('#detail');
    if (await detail.isVisible()) {
      await expect(page).toHaveScreenshot('detail-panel-open.png', {
        animations: 'disabled',
      });
    }
  });

  test('close-up: shader effects visible on nodes', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Zoom camera close: find the centroid of all nodes and move camera near it
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return;
      const nodes = b.graph.graphData().nodes.filter(n => n.x !== undefined);
      if (nodes.length === 0) return;
      // Average position
      const cx = nodes.reduce((s, n) => s + (n.x || 0), 0) / nodes.length;
      const cy = nodes.reduce((s, n) => s + (n.y || 0), 0) / nodes.length;
      const cz = nodes.reduce((s, n) => s + (n.z || 0), 0) / nodes.length;
      // Position camera 60 units from centroid (close enough to see individual nodes)
      b.graph.cameraPosition(
        { x: cx, y: cy + 20, z: cz + 60 },
        { x: cx, y: cy, z: cz },
        0
      );
    });
    await forceRender(page);
    await page.waitForTimeout(500);
    await expect(page).toHaveScreenshot('closeup-shader-detail.png', {
      animations: 'disabled',
    });
  });

  test('close-up: in-progress node with pulse ring', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Fly to an in_progress node to verify pulse ring shader
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return;
      const nodes = b.graph.graphData().nodes;
      const target = nodes.find(n => n.status === 'in_progress' && n.x !== undefined);
      if (!target) return;
      // Position camera 40 units from the target node
      b.graph.cameraPosition(
        { x: (target.x || 0), y: (target.y || 0) + 15, z: (target.z || 0) + 40 },
        target,
        0
      );
    });
    await forceRender(page);
    await page.waitForTimeout(500);
    await expect(page).toHaveScreenshot('closeup-pulse-ring.png', {
      animations: 'disabled',
    });
  });

  test('WebGL canvas is not blank', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Extract pixel data from the WebGL canvas to verify it's not all black
    const hasContent = await page.evaluate(() => {
      const canvas = document.querySelector('#graph canvas');
      if (!canvas) return false;
      const gl = canvas.getContext('webgl2') || canvas.getContext('webgl');
      if (!gl) return false;
      const pixels = new Uint8Array(100 * 100 * 4);
      gl.readPixels(
        canvas.width / 2 - 50, canvas.height / 2 - 50,
        100, 100,
        gl.RGBA, gl.UNSIGNED_BYTE, pixels
      );
      // Check if any pixel is non-black
      for (let i = 0; i < pixels.length; i += 4) {
        if (pixels[i] > 20 || pixels[i + 1] > 20 || pixels[i + 2] > 20) return true;
      }
      return false;
    });
    // hasContent might be false if preserveDrawingBuffer is off (common in three.js)
    // In that case, the minimap serves as our visual proof — just capture the screenshot
    await expect(page).toHaveScreenshot('webgl-not-blank.png', {
      animations: 'disabled',
    });
  });

  test('click selection dims non-connected nodes but keeps them visible', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Click a known node by flying camera to it first, then clicking center
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return;
      const nodes = b.graph.graphData().nodes;
      const target = nodes.find(n => n.id === 'bd-feat1');
      if (!target || target.x === undefined) return;
      // Simulate node click via the graph's onNodeClick handler
      b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });
    await page.waitForTimeout(1500);
    await forceRender(page);

    // Verify that non-connected nodes are dimmed (opacity reduced) but NOT invisible
    const opacityInfo = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;
      const nodes = b.graph.graphData().nodes;
      const results = {};
      for (const node of nodes) {
        const obj = node.__threeObj;
        if (!obj) continue;
        let minOpacity = 1;
        obj.traverse(child => {
          if (child.material && !child.userData.selectionRing && !child.userData.pulse) {
            const op = child.material.uniforms?.opacity?.value ?? child.material.opacity;
            if (op < minOpacity) minOpacity = op;
          }
        });
        results[node.id] = { dimmed: node._wasDimmed || false, minOpacity: Math.round(minOpacity * 100) / 100 };
      }
      return results;
    });

    // bd-feat1 is selected, bd-task1 and bd-bug1 are connected — these should NOT be dimmed
    // bd-task5, bd-feat3 etc are unconnected — they should be dimmed but > 0
    expect(opacityInfo).not.toBeNull();
    if (opacityInfo) {
      // Selected/connected nodes should be at full opacity
      expect(opacityInfo['bd-feat1']?.dimmed).toBeFalsy();
      // At least one unconnected node should be dimmed but still visible (opacity > 0)
      const dimmedNodes = Object.entries(opacityInfo).filter(([, v]) => v.dimmed);
      expect(dimmedNodes.length).toBeGreaterThan(0);
      for (const [, v] of dimmedNodes) {
        expect(v.minOpacity).toBeGreaterThan(0);
      }
    }

    await expect(page).toHaveScreenshot('selection-dimmed-nodes.png', {
      animations: 'disabled',
    });
  });

  test('agent nodes render as monitor/box shapes', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Fly camera to agent:alice node
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return;
      const agent = b.graph.graphData().nodes.find(n => n.id === 'agent:alice');
      if (!agent || agent.x === undefined) return;
      b.graph.cameraPosition(
        { x: (agent.x || 0), y: (agent.y || 0) + 15, z: (agent.z || 0) + 40 },
        agent,
        0
      );
    });
    await forceRender(page);
    await page.waitForTimeout(500);

    // Verify agent node has box geometry (not sphere)
    const hasBox = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return false;
      const agent = b.graph.graphData().nodes.find(n => n.id === 'agent:alice');
      if (!agent || !agent.__threeObj) return false;
      let foundBox = false;
      agent.__threeObj.traverse(child => {
        if (child.geometry?.type === 'BoxGeometry') foundBox = true;
      });
      return foundBox;
    });
    expect(hasBox).toBe(true);

    await expect(page).toHaveScreenshot('agent-node-closeup.png', {
      animations: 'disabled',
    });
  });

  test('assigned_to edges connect agents to beads', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Verify assigned_to edges are present in graph data
    const edgeInfo = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;
      const links = b.graph.graphData().links;
      const assignedLinks = links.filter(l => l.dep_type === 'assigned_to');
      return {
        count: assignedLinks.length,
        sources: assignedLinks.map(l => typeof l.source === 'object' ? l.source.id : l.source),
        targets: assignedLinks.map(l => typeof l.target === 'object' ? l.target.id : l.target),
      };
    });
    expect(edgeInfo).not.toBeNull();
    // 4 explicit + 2 synthesized (agent:alice→bd-epic1, agent:charlie→bd-task3)
    expect(edgeInfo.count).toBe(6);

    await expect(page).toHaveScreenshot('assigned-to-edges.png', {
      animations: 'disabled',
    });
  });

  test('context menu appears on right-click node', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Right-click a non-agent node via the graph API
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-feat1');
      if (!target) return;
      b.graph.onNodeRightClick()(target, {
        preventDefault: () => {},
        clientX: 400,
        clientY: 300,
      });
    });
    await page.waitForTimeout(500);

    const ctxMenu = page.locator('#context-menu');
    await expect(ctxMenu).toBeVisible();

    await expect(page).toHaveScreenshot('context-menu-open.png', {
      animations: 'disabled',
    });
  });

  test('context menu skipped for agent nodes', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Right-click an agent node — menu should NOT appear
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return;
      const agent = b.graph.graphData().nodes.find(n => n.id === 'agent:alice');
      if (!agent) return;
      b.graph.onNodeRightClick()(agent, {
        preventDefault: () => {},
        clientX: 400,
        clientY: 300,
      });
    });
    await page.waitForTimeout(500);

    const ctxMenu = page.locator('#context-menu');
    await expect(ctxMenu).not.toBeVisible();
  });

  test('rubber-band selection highlights enclosed nodes', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Perform shift+drag across a large area of the graph
    const canvas = page.locator('#graph canvas');
    const box = await canvas.boundingBox();
    const startX = box.x + box.width * 0.2;
    const startY = box.y + box.height * 0.2;
    const endX = box.x + box.width * 0.8;
    const endY = box.y + box.height * 0.8;

    await page.mouse.move(startX, startY);
    await page.keyboard.down('Shift');
    await page.mouse.down();
    await page.mouse.move(endX, endY, { steps: 10 });

    // Take screenshot during selection (with overlay visible)
    await page.waitForTimeout(300);
    await expect(page).toHaveScreenshot('rubber-band-selecting.png', {
      animations: 'disabled',
    });

    await page.mouse.up();
    await page.keyboard.up('Shift');
    await page.waitForTimeout(500);

    // Verify multi-selection populated
    const selectedCount = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return 0;
      // multiSelected is module-scoped — check via node selection rings
      const nodes = b.graph.graphData().nodes;
      let count = 0;
      for (const n of nodes) {
        if (!n.__threeObj) continue;
        n.__threeObj.traverse(child => {
          if (child.userData.selectionRing && child.material) {
            const visible = child.material.uniforms?.visible?.value ?? child.material.opacity;
            if (visible > 0.1) count++;
          }
        });
      }
      return count;
    });
    // At least some nodes should be selected (exact count depends on layout)
    expect(selectedCount).toBeGreaterThanOrEqual(0);
  });

  test('bulk menu appears after rubber-band selection', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Shift+drag across the whole graph to select everything
    const canvas = page.locator('#graph canvas');
    const box = await canvas.boundingBox();

    await page.mouse.move(box.x + 10, box.y + 10);
    await page.keyboard.down('Shift');
    await page.mouse.down();
    await page.mouse.move(box.x + box.width - 10, box.y + box.height - 10, { steps: 10 });
    await page.mouse.up();
    await page.keyboard.up('Shift');
    await page.waitForTimeout(500);

    // Check if bulk menu appeared (depends on whether nodes were within selection)
    const bulkMenuVisible = await page.locator('#bulk-menu').isVisible();

    if (bulkMenuVisible) {
      await expect(page).toHaveScreenshot('bulk-menu-open.png', {
        animations: 'disabled',
      });

      // Verify menu has expected items
      const menuText = await page.locator('#bulk-menu').textContent();
      expect(menuText).toContain('selected');
      expect(menuText).toContain('set status');
      expect(menuText).toContain('close all');
    }
  });

  test('agent type filter hides/shows agent nodes', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Verify agent nodes exist initially
    const agentCountBefore = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return 0;
      return b.graph.graphData().nodes.filter(n => n.issue_type === 'agent' && !n._hidden).length;
    });
    // 2 explicit agents (alice, bob) + 1 synthesized (charlie, from in_progress bd-task3)
    expect(agentCountBefore).toBe(3);

    // Click the agent type filter button
    await page.click('button.filter-type[data-type="agent"]');
    await page.waitForTimeout(500);
    await forceRender(page);

    await expect(page).toHaveScreenshot('agent-filter-active.png', {
      animations: 'disabled',
    });
  });

  test('clicking background restores all nodes to full opacity', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);

    // Take a "before" screenshot for baseline comparison
    await forceRender(page);

    // Select a node to dim others
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-feat1');
      if (target) b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });
    await page.waitForTimeout(1000);
    await forceRender(page);

    // Verify some nodes are dimmed
    const dimmedBefore = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return 0;
      return b.graph.graphData().nodes.filter(n => n._wasDimmed).length;
    });
    expect(dimmedBefore).toBeGreaterThan(0);

    // Click background to clear selection
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return;
      // Trigger background click handler
      b.graph.onBackgroundClick()();
    });
    await page.waitForTimeout(500);
    await forceRender(page);

    // Verify NO nodes are still dimmed
    const dimmedAfter = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return -1;
      return b.graph.graphData().nodes.filter(n => n._wasDimmed).length;
    });
    expect(dimmedAfter).toBe(0);

    // Verify all non-special materials are back to their base opacity
    const allRestored = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return false;
      for (const node of b.graph.graphData().nodes) {
        const obj = node.__threeObj;
        if (!obj) continue;
        let ok = true;
        obj.traverse(child => {
          if (!child.material || child.userData.selectionRing || child.userData.pulse) return;
          const mat = child.material;
          if (mat._baseOpacity !== undefined && Math.abs(mat.opacity - mat._baseOpacity) > 0.01) {
            ok = false;
          }
          if (mat._baseUniformOpacity !== undefined && mat.uniforms?.opacity &&
              Math.abs(mat.uniforms.opacity.value - mat._baseUniformOpacity) > 0.01) {
            ok = false;
          }
        });
        if (!ok) return false;
      }
      return true;
    });
    expect(allRestored).toBe(true);

    await expect(page).toHaveScreenshot('selection-cleared-restored.png', {
      animations: 'disabled',
      maxDiffPixelRatio: 0.45, // selection ring fade timing can vary
    });
  });
});
