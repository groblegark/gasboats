// E2E tests for Shift+S/D epic cycling navigation (bd-pnngb).
// Tests keyboard-driven epic cycling with camera fly-to and visual emphasis.
//
// Run: npx playwright test tests/epic-cycling.spec.js
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

// MOCK_GRAPH has 2 epics: bd-epic1 "Epic: Platform Overhaul" and bd-epic2 "Epic: Observability Stack"
// Sorted alphabetically: "Epic: Observability Stack" (index 0), "Epic: Platform Overhaul" (index 1)

test.describe('epic cycling (Shift+S/D)', () => {

  test('Shift+D cycles to first epic and shows HUD', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Press Shift+D to cycle to the first epic
    await page.keyboard.press('Shift+D');
    await page.waitForTimeout(500);

    // HUD should appear with "Epic 1/2: ..."
    const hud = page.locator('#epic-hud');
    await expect(hud).toBeVisible();
    const hudText = await hud.textContent();
    expect(hudText).toMatch(/Epic 1\/2:/);
    expect(hudText).toContain('Observability Stack');
  });

  test('Shift+D twice cycles to second epic', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await page.keyboard.press('Shift+D');
    await page.waitForTimeout(300);
    await page.keyboard.press('Shift+D');
    await page.waitForTimeout(500);

    const hud = page.locator('#epic-hud');
    const hudText = await hud.textContent();
    expect(hudText).toMatch(/Epic 2\/2:/);
    expect(hudText).toContain('Platform Overhaul');
  });

  test('Shift+D wraps around at the end', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // 3 presses: epic1, epic2, wrap to epic1
    await page.keyboard.press('Shift+D');
    await page.waitForTimeout(200);
    await page.keyboard.press('Shift+D');
    await page.waitForTimeout(200);
    await page.keyboard.press('Shift+D');
    await page.waitForTimeout(500);

    const hud = page.locator('#epic-hud');
    const hudText = await hud.textContent();
    expect(hudText).toMatch(/Epic 1\/2:/);
  });

  test('Shift+S cycles to previous (last) epic', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Shift+S from no selection should go to the last epic
    await page.keyboard.press('Shift+S');
    await page.waitForTimeout(500);

    const hud = page.locator('#epic-hud');
    const hudText = await hud.textContent();
    expect(hudText).toMatch(/Epic 2\/2:/);
    expect(hudText).toContain('Platform Overhaul');
  });

  test('Shift+S after Shift+D goes back to previous', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await page.keyboard.press('Shift+D');
    await page.waitForTimeout(200);
    await page.keyboard.press('Shift+D');
    await page.waitForTimeout(200);
    // Now at epic 2, go back
    await page.keyboard.press('Shift+S');
    await page.waitForTimeout(500);

    const hud = page.locator('#epic-hud');
    const hudText = await hud.textContent();
    expect(hudText).toMatch(/Epic 1\/2:/);
  });

  test('non-epic nodes are dimmed during highlight', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await page.keyboard.press('Shift+D');
    await page.waitForTimeout(800);

    // Check that non-child nodes have reduced opacity via __threeObj
    const dimmedCount = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return 0;
      const nodes = b.graphData().nodes;
      let dimmed = 0;
      for (const n of nodes) {
        if (!n.__threeObj) continue;
        let hasDimmed = false;
        n.__threeObj.traverse(c => {
          if (c.material && c.material.opacity < 0.5) hasDimmed = true;
        });
        if (hasDimmed) dimmed++;
      }
      return dimmed;
    });

    // There should be some dimmed nodes (non-epic, non-child nodes)
    expect(dimmedCount).toBeGreaterThan(0);
  });

  test('Escape clears epic highlighting and hides HUD', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await page.keyboard.press('Shift+D');
    await page.waitForTimeout(500);

    const hud = page.locator('#epic-hud');
    await expect(hud).toBeVisible();

    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);

    // HUD should be hidden
    await expect(hud).toBeHidden();

    // All node opacities should be restored
    const allRestored = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b) return false;
      const nodes = b.graphData().nodes;
      for (const n of nodes) {
        if (!n.__threeObj) continue;
        let hasDimmed = false;
        n.__threeObj.traverse(c => {
          if (c.material && c.material.opacity < 0.5) hasDimmed = true;
        });
        if (hasDimmed) return false;
      }
      return true;
    });
    expect(allRestored).toBe(true);
  });

  test('cycling is inert when search input is focused', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Focus search input
    await page.keyboard.press('/');
    await page.waitForTimeout(200);

    // Try Shift+D — should not trigger epic cycling
    await page.keyboard.press('Shift+D');
    await page.waitForTimeout(500);

    const hud = page.locator('#epic-hud');
    await expect(hud).toBeHidden();
  });

  test('HUD auto-fades after 3 seconds', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await page.keyboard.press('Shift+D');
    await page.waitForTimeout(500);

    const hud = page.locator('#epic-hud');
    await expect(hud).toBeVisible();

    // Wait for auto-fade (3s + 0.4s transition + buffer)
    await page.waitForTimeout(4000);

    await expect(hud).toBeHidden();
  });
});
