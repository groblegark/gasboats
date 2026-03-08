// Stability test: verify 3D view doesn't reset during periodic data refreshes (bd-7ccyd).
// This test reproduces the "view resets every ~30s" bug by:
// 1. Loading the graph with mock data
// 2. Letting it stabilize
// 3. Simulating multiple poll-cycle refreshes (structure changes each time)
// 4. Verifying camera position and node positions remain stable
//
// The test injects slightly different Graph responses on each poll to simulate
// live beads being created/closed, triggering the structureChanged code path.

import { test, expect } from '@playwright/test';
import { MOCK_GRAPH, MOCK_PING } from './fixtures.js';

// Graph response that changes slightly each call (simulates live system)
function makeGraphResponses() {
  // Base graph from fixtures
  const base = JSON.parse(JSON.stringify(MOCK_GRAPH));

  // Response 1: base graph (initial load)
  const r1 = JSON.parse(JSON.stringify(base));

  // Response 2: add a new node (simulates bead creation)
  const r2 = JSON.parse(JSON.stringify(base));
  r2.nodes.push({
    id: 'bd-new-1',
    title: 'New bead from poll 2',
    status: 'open',
    priority: 2,
    issue_type: 'task',
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  });

  // Response 3: change status of an existing node + add another
  const r3 = JSON.parse(JSON.stringify(r2));
  if (r3.nodes.length > 1) r3.nodes[1].status = 'closed';
  r3.nodes.push({
    id: 'bd-new-2',
    title: 'New bead from poll 3',
    status: 'in_progress',
    priority: 1,
    issue_type: 'task',
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
  });

  // Response 4: remove the first new node (simulates filtering)
  const r4 = JSON.parse(JSON.stringify(r3));
  r4.nodes = r4.nodes.filter(n => n.id !== 'bd-new-1');

  return [r1, r2, r3, r4];
}

test.describe('beads3d view stability (bd-7ccyd)', () => {

  test('camera position remains stable across multiple poll-cycle refreshes', async ({ page }) => {
    const responses = makeGraphResponses();
    let callCount = 0;

    // Mock API — Graph endpoint returns different data each call
    await page.route('**/api/bd.v1.BeadsService/Graph', async route => {
      const idx = Math.min(callCount, responses.length - 1);
      callCount++;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(responses[idx]),
      });
    });

    await page.route('**/api/bd.v1.BeadsService/Ping', async route => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) });
    });
    await page.route('**/api/bd.v1.BeadsService/List', async route => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
    });
    await page.route('**/api/bd.v1.BeadsService/Show', async route => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({}) });
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
    await page.route('**/api/bd.v1.BeadsService/Update', async route => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) });
    });
    await page.route('**/api/bd.v1.BeadsService/Close', async route => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ ok: true }) });
    });
    await page.route('**/api/events', async route => {
      await route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' });
    });
    await page.route('**/api/bus/events*', async route => {
      await route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' });
    });

    // Load the page
    await page.goto('/');
    await page.waitForSelector('#status.connected', { timeout: 15000 });

    // Wait for initial layout to stabilize (force simulation cooldown)
    await page.waitForTimeout(5000);

    // Record initial camera position and node positions
    const snapshot1 = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;
      const cam = b.graph.camera();
      const nodes = b.graphData().nodes
        .filter(n => n.x !== undefined && !n._hidden)
        .map(n => ({ id: n.id, x: n.x, y: n.y, z: n.z || 0 }));
      return {
        camera: { x: cam.position.x, y: cam.position.y, z: cam.position.z },
        nodeCount: nodes.length,
        nodes,
      };
    });

    expect(snapshot1).not.toBeNull();
    expect(snapshot1.nodeCount).toBeGreaterThan(0);
    console.log(`[stability] Initial: ${snapshot1.nodeCount} nodes, camera at (${snapshot1.camera.x.toFixed(1)}, ${snapshot1.camera.y.toFixed(1)}, ${snapshot1.camera.z.toFixed(1)})`);

    // Wait for 4 poll cycles (40 seconds) — each one delivers different Graph data,
    // triggering structureChanged + graph.graphData() reheat
    await page.waitForTimeout(45000);

    // At least 3 additional Graph calls should have happened
    expect(callCount).toBeGreaterThanOrEqual(3);
    console.log(`[stability] After 45s: ${callCount} Graph API calls`);

    // Record final camera position and node positions
    const snapshot2 = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;
      const cam = b.graph.camera();
      const nodes = b.graphData().nodes
        .filter(n => n.x !== undefined && !n._hidden)
        .map(n => ({ id: n.id, x: n.x, y: n.y, z: n.z || 0 }));
      return {
        camera: { x: cam.position.x, y: cam.position.y, z: cam.position.z },
        nodeCount: nodes.length,
        nodes,
      };
    });

    expect(snapshot2).not.toBeNull();
    console.log(`[stability] Final: ${snapshot2.nodeCount} nodes, camera at (${snapshot2.camera.x.toFixed(1)}, ${snapshot2.camera.y.toFixed(1)}, ${snapshot2.camera.z.toFixed(1)})`);

    // ASSERT: Camera should NOT have moved significantly
    const cameraDrift = Math.sqrt(
      (snapshot2.camera.x - snapshot1.camera.x) ** 2 +
      (snapshot2.camera.y - snapshot1.camera.y) ** 2 +
      (snapshot2.camera.z - snapshot1.camera.z) ** 2
    );
    console.log(`[stability] Camera drift: ${cameraDrift.toFixed(2)} units`);
    expect(cameraDrift).toBeLessThan(5); // camera should be rock-solid (< 5 units)

    // ASSERT: Existing nodes should NOT have scattered far from original positions
    const nodeMap1 = new Map(snapshot1.nodes.map(n => [n.id, n]));
    let maxDrift = 0;
    let totalDrift = 0;
    let matchedCount = 0;
    for (const n2 of snapshot2.nodes) {
      const n1 = nodeMap1.get(n2.id);
      if (!n1) continue; // new node, skip
      const drift = Math.sqrt((n2.x - n1.x) ** 2 + (n2.y - n1.y) ** 2 + (n2.z - n1.z) ** 2);
      maxDrift = Math.max(maxDrift, drift);
      totalDrift += drift;
      matchedCount++;
    }
    const avgDrift = matchedCount > 0 ? totalDrift / matchedCount : 0;
    console.log(`[stability] Node drift — max: ${maxDrift.toFixed(2)}, avg: ${avgDrift.toFixed(2)} (${matchedCount} matched nodes)`);

    // Nodes should stay roughly in place (< 50 units average drift)
    // Before the fix, drift was 100-300+ units per refresh cycle
    expect(avgDrift).toBeLessThan(50);
    expect(maxDrift).toBeLessThan(100);
  });

  test('user camera zoom/pan survives refresh cycles', async ({ page }) => {
    const responses = makeGraphResponses();
    let callCount = 0;

    await page.route('**/api/bd.v1.BeadsService/Graph', async route => {
      const idx = Math.min(callCount, responses.length - 1);
      callCount++;
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(responses[idx]) });
    });
    await page.route('**/api/bd.v1.BeadsService/Ping', async route => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) });
    });
    await page.route('**/api/bd.v1.BeadsService/List', async route => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) });
    });
    await page.route('**/api/bd.v1.BeadsService/Show', async route => {
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({}) });
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
    await page.route('**/api/events', async route => {
      await route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' });
    });
    await page.route('**/api/bus/events*', async route => {
      await route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' });
    });

    await page.goto('/');
    await page.waitForSelector('#status.connected', { timeout: 15000 });
    await page.waitForTimeout(5000);

    // Move camera to a custom position (simulating user zoom/pan)
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return;
      b.graph.cameraPosition({ x: 200, y: 100, z: 300 }, { x: 50, y: 0, z: 0 }, 0);
      const controls = b.graph.controls();
      if (controls) controls.update();
    });
    await page.waitForTimeout(500);

    // Record the custom camera position
    const customCam = await page.evaluate(() => {
      const cam = window.__beads3d.graph.camera();
      return { x: cam.position.x, y: cam.position.y, z: cam.position.z };
    });

    console.log(`[stability] Custom camera: (${customCam.x.toFixed(1)}, ${customCam.y.toFixed(1)}, ${customCam.z.toFixed(1)})`);

    // Wait for 3 poll cycles
    await page.waitForTimeout(35000);

    // Camera should still be at the custom position
    const finalCam = await page.evaluate(() => {
      const cam = window.__beads3d.graph.camera();
      return { x: cam.position.x, y: cam.position.y, z: cam.position.z };
    });

    console.log(`[stability] Final camera: (${finalCam.x.toFixed(1)}, ${finalCam.y.toFixed(1)}, ${finalCam.z.toFixed(1)})`);

    const drift = Math.sqrt(
      (finalCam.x - customCam.x) ** 2 +
      (finalCam.y - customCam.y) ** 2 +
      (finalCam.z - customCam.z) ** 2
    );
    console.log(`[stability] Camera drift after pan: ${drift.toFixed(2)} units`);
    expect(drift).toBeLessThan(5);
  });
});
