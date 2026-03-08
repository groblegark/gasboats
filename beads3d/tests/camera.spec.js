// E2E tests for Quake-style smooth camera movement (bd-zab4q).
// Tests velocity-based arrow key scrolling with acceleration/deceleration.
//
// Run: npx playwright test tests/camera.spec.js
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

async function getCameraPos(page) {
  return page.evaluate(() => {
    const b = window.__beads3d;
    if (!b || !b.graph) return null;
    const cam = b.graph.camera();
    return { x: cam.position.x, y: cam.position.y, z: cam.position.z };
  });
}

// Simulate holding a key via the exposed _keysDown set
async function holdKey(page, key) {
  await page.evaluate((k) => window.__beads3d_keysDown.add(k), key);
}
async function releaseKey(page, key) {
  await page.evaluate((k) => window.__beads3d_keysDown.delete(k), key);
}

function dist3d(a, b) {
  return Math.sqrt((a.x - b.x) ** 2 + (a.y - b.y) ** 2 + (a.z - b.z) ** 2);
}

test.describe('smooth camera movement (bd-zab4q)', () => {

  test('ArrowUp accelerates camera upward', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const before = await getCameraPos(page);

    await holdKey(page, 'ArrowUp');
    await page.waitForTimeout(500);
    await releaseKey(page, 'ArrowUp');
    await page.waitForTimeout(200);

    const after = await getCameraPos(page);
    // ArrowUp applies positive Y velocity
    expect(after.y).toBeGreaterThan(before.y + 2);
  });

  test('ArrowRight strafes camera in camera-relative direction', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const before = await getCameraPos(page);

    await holdKey(page, 'ArrowRight');
    await page.waitForTimeout(400);
    await releaseKey(page, 'ArrowRight');
    await page.waitForTimeout(200);

    const after = await getCameraPos(page);
    expect(dist3d(before, after)).toBeGreaterThan(3);
  });

  test('velocity decays after key release (momentum + friction)', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Build velocity upward
    await holdKey(page, 'ArrowUp');
    await page.waitForTimeout(300);
    const posAtRelease = await getCameraPos(page);
    await releaseKey(page, 'ArrowUp');

    // Let momentum coast
    await page.waitForTimeout(400);
    const posCoasted = await getCameraPos(page);

    // Should have coasted further after release
    expect(posCoasted.y).toBeGreaterThan(posAtRelease.y);

    // Wait for friction to fully stop
    await page.waitForTimeout(1500);
    const posStopped = await getCameraPos(page);
    await page.waitForTimeout(300);
    const posFinal = await getCameraPos(page);

    // Should be fully stopped (friction decays exponentially)
    expect(dist3d(posStopped, posFinal)).toBeLessThan(1);
  });

  test('longer hold = more distance (acceleration curve)', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Short hold: 100ms
    const b1 = await getCameraPos(page);
    await holdKey(page, 'ArrowUp');
    await page.waitForTimeout(100);
    await releaseKey(page, 'ArrowUp');
    await page.waitForTimeout(800);
    const a1 = await getCameraPos(page);
    const shortDist = dist3d(b1, a1);

    // Long hold: 500ms
    const b2 = await getCameraPos(page);
    await holdKey(page, 'ArrowUp');
    await page.waitForTimeout(500);
    await releaseKey(page, 'ArrowUp');
    await page.waitForTimeout(800);
    const a2 = await getCameraPos(page);
    const longDist = dist3d(b2, a2);

    // Long hold should cover significantly more distance
    expect(longDist).toBeGreaterThan(shortDist * 1.5);
  });

  test('opposing keys reduce net velocity', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Press up to build velocity
    await holdKey(page, 'ArrowUp');
    await page.waitForTimeout(300);

    // Press down to oppose
    await holdKey(page, 'ArrowDown');
    await page.waitForTimeout(300);

    // Release both
    await releaseKey(page, 'ArrowUp');
    await releaseKey(page, 'ArrowDown');

    // Wait for decay
    await page.waitForTimeout(500);

    // Residual velocity should be much less than if only one key were held
    const pos1 = await getCameraPos(page);
    await page.waitForTimeout(300);
    const pos2 = await getCameraPos(page);
    // Even with imperfect cancellation, movement should be small
    expect(dist3d(pos1, pos2)).toBeLessThan(15);
  });

  // WASD camera controls (bd-pwaen)

  test('W moves camera forward along look direction (XZ plane)', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const before = await getCameraPos(page);

    await holdKey(page, 'w');
    await page.waitForTimeout(500);
    await releaseKey(page, 'w');
    await page.waitForTimeout(200);

    const after = await getCameraPos(page);
    // W moves in XZ plane (forward), Y should stay roughly the same
    const xzDist = Math.sqrt((after.x - before.x) ** 2 + (after.z - before.z) ** 2);
    expect(xzDist).toBeGreaterThan(2);
    expect(Math.abs(after.y - before.y)).toBeLessThan(1);
  });

  test('S moves camera backward along look direction (XZ plane)', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const before = await getCameraPos(page);

    await holdKey(page, 's');
    await page.waitForTimeout(500);
    await releaseKey(page, 's');
    await page.waitForTimeout(200);

    const after = await getCameraPos(page);
    // S moves backward in XZ plane
    const xzDist = Math.sqrt((after.x - before.x) ** 2 + (after.z - before.z) ** 2);
    expect(xzDist).toBeGreaterThan(2);
    expect(Math.abs(after.y - before.y)).toBeLessThan(1);
  });

  test('W and S move in opposite directions', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Move forward with W
    const start = await getCameraPos(page);
    await holdKey(page, 'w');
    await page.waitForTimeout(400);
    await releaseKey(page, 'w');
    await page.waitForTimeout(800);
    const afterW = await getCameraPos(page);

    // Move backward with S
    await holdKey(page, 's');
    await page.waitForTimeout(400);
    await releaseKey(page, 's');
    await page.waitForTimeout(800);
    const afterS = await getCameraPos(page);

    // After W then equal S, should be close to start position
    const returnDist = dist3d(start, afterS);
    const forwardDist = dist3d(start, afterW);
    expect(returnDist).toBeLessThan(forwardDist);
  });

  test('A strafes camera left (same as ArrowLeft)', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const before = await getCameraPos(page);

    await holdKey(page, 'a');
    await page.waitForTimeout(400);
    await releaseKey(page, 'a');
    await page.waitForTimeout(200);

    const after = await getCameraPos(page);
    expect(dist3d(before, after)).toBeGreaterThan(3);
  });

  test('D strafes camera right (same as ArrowRight)', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const before = await getCameraPos(page);

    await holdKey(page, 'd');
    await page.waitForTimeout(400);
    await releaseKey(page, 'd');
    await page.waitForTimeout(200);

    const after = await getCameraPos(page);
    expect(dist3d(before, after)).toBeGreaterThan(3);
  });

  test('A and D match ArrowLeft and ArrowRight movement', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Strafe right with D
    const b1 = await getCameraPos(page);
    await holdKey(page, 'd');
    await page.waitForTimeout(300);
    await releaseKey(page, 'd');
    await page.waitForTimeout(800);
    const a1 = await getCameraPos(page);
    const dDist = dist3d(b1, a1);

    // Strafe right with ArrowRight
    const b2 = await getCameraPos(page);
    await holdKey(page, 'ArrowRight');
    await page.waitForTimeout(300);
    await releaseKey(page, 'ArrowRight');
    await page.waitForTimeout(800);
    const a2 = await getCameraPos(page);
    const arrowDist = dist3d(b2, a2);

    // Should be similar distances (both use same physics)
    expect(Math.abs(dDist - arrowDist)).toBeLessThan(dDist * 0.5);
  });

  test('WASD keys inert when search input focused', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Focus search input
    await page.keyboard.press('/');
    await page.waitForTimeout(100);

    const before = await getCameraPos(page);

    // Press WASD via keyboard (not holdKey helper which bypasses focus check)
    await page.keyboard.down('w');
    await page.waitForTimeout(300);
    await page.keyboard.up('w');
    await page.waitForTimeout(200);

    const after = await getCameraPos(page);
    // Camera should not have moved
    expect(dist3d(before, after)).toBeLessThan(1);
  });

  test('speed is clamped at CAM_MAX_SPEED', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Hold key for a long time to hit max speed
    await holdKey(page, 'ArrowUp');
    await page.waitForTimeout(1000);

    // Sample two positions 100ms apart to measure speed
    const p1 = await getCameraPos(page);
    await page.waitForTimeout(100);
    const p2 = await getCameraPos(page);
    await releaseKey(page, 'ArrowUp');

    // Speed per ~100ms should be bounded (CAM_MAX_SPEED=16 per frame × ~6 frames = ~96 max)
    const speed = dist3d(p1, p2);
    expect(speed).toBeLessThan(150); // reasonable upper bound
    expect(speed).toBeGreaterThan(5); // should be moving fast
  });
});
