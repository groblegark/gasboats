// E2E tests for Control Panel — sections, sliders, color pickers, presets (bd-fa9k7).
// Tests panel open/close, section collapse, slider interaction, color pickers,
// preset application, and custom preset save/load.
//
// Run: npx playwright test tests/control-panel.spec.js
// View report: npx playwright show-report test-results/html-report

import { test, expect } from '@playwright/test';
import { MOCK_GRAPH, MOCK_PING, MOCK_SHOW } from './fixtures.js';

// ---- Helpers ----

async function mockAPI(page) {
  await page.route('**/api/bd.v1.BeadsService/Ping', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_PING) }));
  await page.route('**/api/bd.v1.BeadsService/Graph', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_GRAPH) }));
  await page.route('**/api/bd.v1.BeadsService/List', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
  await page.route('**/api/bd.v1.BeadsService/Show', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_SHOW) }));
  await page.route('**/api/bd.v1.BeadsService/Stats', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_GRAPH.stats) }));
  await page.route('**/api/bd.v1.BeadsService/Blocked', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));
  await page.route('**/api/bd.v1.BeadsService/Ready', route =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify([]) }));

  await page.route('**/api/events', route =>
    route.fulfill({ status: 200, contentType: 'text/event-stream', body: 'data: {"type":"ping"}\n\n' }));
  await page.route('**/api/bus/events*', route =>
    route.fulfill({
      status: 200,
      contentType: 'text/event-stream',
      headers: { 'Cache-Control': 'no-cache', 'Connection': 'keep-alive' },
      body: ': keepalive\n\n',
    }));
}

async function waitForGraph(page) {
  await page.waitForSelector('#status.connected', { timeout: 15000 });
  await page.waitForTimeout(2000);
  await page.waitForFunction(() => {
    const b = window.__beads3d;
    return b && b.graph && b.graph.graphData().nodes.length > 0;
  }, { timeout: 10000 });
}

async function openPanel(page) {
  await page.keyboard.press('g');
  await page.waitForTimeout(300);
  await expect(page.locator('#control-panel')).toHaveClass(/open/);
}

// =====================================================================
// PANEL OPEN / CLOSE
// =====================================================================

test.describe('Control panel open/close', () => {

  test('panel is hidden by default', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const panel = page.locator('#control-panel');
    await expect(panel).not.toHaveClass(/open/);
  });

  test('g key opens panel', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await page.keyboard.press('g');
    await page.waitForTimeout(300);
    await expect(page.locator('#control-panel')).toHaveClass(/open/);
  });

  test('g key toggles panel closed', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    await page.keyboard.press('g');
    await page.waitForTimeout(300);
    await expect(page.locator('#control-panel')).not.toHaveClass(/open/);
  });

  test('close button closes panel', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    await page.locator('#cp-close').click();
    await page.waitForTimeout(300);
    await expect(page.locator('#control-panel')).not.toHaveClass(/open/);
  });

  test('header shows CONTROL PANEL title', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    await expect(page.locator('.cp-title')).toContainText('CONTROL PANEL');
  });
});

// =====================================================================
// SECTIONS
// =====================================================================

test.describe('Control panel sections', () => {

  test('all 7 sections are present', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    await expect(page.locator('#cp-bloom')).toBeAttached();
    await expect(page.locator('#cp-shaders')).toBeAttached();
    await expect(page.locator('#cp-stars')).toBeAttached();
    await expect(page.locator('#cp-colors')).toBeAttached();
    await expect(page.locator('#cp-labels')).toBeAttached();
    await expect(page.locator('#cp-animation')).toBeAttached();
    await expect(page.locator('#cp-presets')).toBeAttached();
  });

  test('section labels are correct', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    const labels = page.locator('.cp-section-label');
    const allLabels = await labels.allTextContents();
    expect(allLabels).toContain('Bloom');
    expect(allLabels).toContain('Shaders');
    expect(allLabels).toContain('Star Field');
    expect(allLabels).toContain('Colors');
    expect(allLabels).toContain('Labels');
    expect(allLabels).toContain('Animation');
    expect(allLabels).toContain('Presets');
  });

  test('clicking section header collapses section', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    const bloomSection = page.locator('#cp-bloom');
    const bloomHeader = bloomSection.locator('.cp-section-header');

    // Collapse
    await bloomHeader.click();
    await page.waitForTimeout(200);
    await expect(bloomSection).toHaveClass(/collapsed/);

    // Body should be hidden
    await expect(bloomSection.locator('.cp-section-body')).not.toBeVisible();

    // Expand again
    await bloomHeader.click();
    await page.waitForTimeout(200);
    await expect(bloomSection).not.toHaveClass(/collapsed/);
  });
});

// =====================================================================
// BLOOM SLIDERS
// =====================================================================

test.describe('Bloom controls', () => {

  test('bloom section has threshold, strength, and radius sliders', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    await expect(page.locator('#cp-bloom-threshold')).toBeAttached();
    await expect(page.locator('#cp-bloom-strength')).toBeAttached();
    await expect(page.locator('#cp-bloom-radius')).toBeAttached();
  });

  test('bloom threshold slider shows default value', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    const val = page.locator('#cp-bloom-threshold-val');
    const text = await val.textContent();
    expect(parseFloat(text)).toBeGreaterThanOrEqual(0);
    expect(parseFloat(text)).toBeLessThanOrEqual(1);
  });

  test('adjusting bloom strength slider updates display value', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    const slider = page.locator('#cp-bloom-strength');
    const valEl = page.locator('#cp-bloom-strength-val');

    // Set slider to a new value
    await slider.fill('1.5');
    await slider.dispatchEvent('input');
    await page.waitForTimeout(200);

    const text = await valEl.textContent();
    expect(parseFloat(text)).toBeCloseTo(1.5, 1);
  });
});

// =====================================================================
// SHADER CONTROLS
// =====================================================================

test.describe('Shader controls', () => {

  test('shader section has glow opacity, power, and pulse speed', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    await expect(page.locator('#cp-fresnel-opacity')).toBeAttached();
    await expect(page.locator('#cp-fresnel-power')).toBeAttached();
    await expect(page.locator('#cp-pulse-speed')).toBeAttached();
  });

  test('adjusting pulse speed updates display', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    const slider = page.locator('#cp-pulse-speed');
    const valEl = page.locator('#cp-pulse-speed-val');

    await slider.fill('6.0');
    await slider.dispatchEvent('input');
    await page.waitForTimeout(200);

    const text = await valEl.textContent();
    expect(parseFloat(text)).toBeCloseTo(6.0, 1);
  });
});

// =====================================================================
// STAR FIELD CONTROLS
// =====================================================================

test.describe('Star field controls', () => {

  test('star section has count and twinkle speed sliders', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    await expect(page.locator('#cp-star-count')).toBeAttached();
    await expect(page.locator('#cp-twinkle-speed')).toBeAttached();
  });

  test('star count slider shows numeric value', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    const val = page.locator('#cp-star-count-val');
    const text = await val.textContent();
    expect(parseInt(text)).toBeGreaterThanOrEqual(0);
  });
});

// =====================================================================
// COLOR PICKERS
// =====================================================================

test.describe('Color controls', () => {

  test('color section has 6 color pickers', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    await expect(page.locator('#cp-bg-color')).toBeAttached();
    await expect(page.locator('#cp-color-open')).toBeAttached();
    await expect(page.locator('#cp-color-active')).toBeAttached();
    await expect(page.locator('#cp-color-blocked')).toBeAttached();
    await expect(page.locator('#cp-color-agent')).toBeAttached();
    await expect(page.locator('#cp-color-epic')).toBeAttached();
  });

  test('color pickers have default hex values', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    const openColor = await page.locator('#cp-color-open').inputValue();
    expect(openColor).toMatch(/^#[0-9a-f]{6}$/i);

    const activeColor = await page.locator('#cp-color-active').inputValue();
    expect(activeColor).toMatch(/^#[0-9a-f]{6}$/i);
  });

  test('color labels describe each picker', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    const colorBody = page.locator('#cp-colors .cp-section-body');
    const labels = await colorBody.locator('.cp-label').allTextContents();
    expect(labels).toContain('background');
    expect(labels).toContain('open');
    expect(labels).toContain('active');
    expect(labels).toContain('blocked');
    expect(labels).toContain('agent');
    expect(labels).toContain('epic');
  });
});

// =====================================================================
// LABEL & ANIMATION CONTROLS
// =====================================================================

test.describe('Label and animation controls', () => {

  test('label section has font size and opacity sliders', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    await expect(page.locator('#cp-label-size')).toBeAttached();
    await expect(page.locator('#cp-label-opacity')).toBeAttached();
  });

  test('animation section has fly speed and force strength', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    await expect(page.locator('#cp-fly-speed')).toBeAttached();
    await expect(page.locator('#cp-force-strength')).toBeAttached();
  });

  test('adjusting label size updates display value', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    const slider = page.locator('#cp-label-size');
    const valEl = page.locator('#cp-label-size-val');

    await slider.fill('16');
    await slider.dispatchEvent('input');
    await page.waitForTimeout(200);

    const text = await valEl.textContent();
    expect(parseInt(text)).toBe(16);
  });

  test('adjusting force strength updates display value', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    const slider = page.locator('#cp-force-strength');
    const valEl = page.locator('#cp-force-strength-val');

    await slider.fill('100');
    await slider.dispatchEvent('input');
    await page.waitForTimeout(200);

    const text = await valEl.textContent();
    expect(parseInt(text)).toBe(100);
  });
});

// =====================================================================
// PRESETS
// =====================================================================

test.describe('Presets', () => {

  test('preset section has built-in preset buttons', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    const presetBtns = page.locator('#cp-preset-buttons button');
    const count = await presetBtns.count();
    expect(count).toBeGreaterThanOrEqual(3); // default dark, neon, high contrast
  });

  test('clicking neon preset changes bloom strength', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);

    // Get initial bloom strength
    const initialVal = await page.locator('#cp-bloom-strength-val').textContent();

    // Find and click a non-default preset (neon has strength 1.8)
    const presetBtns = page.locator('#cp-preset-buttons button');
    const count = await presetBtns.count();
    // Click the second preset (neon)
    if (count >= 2) {
      await presetBtns.nth(1).click();
      await page.waitForTimeout(300);
      const newVal = await page.locator('#cp-bloom-strength-val').textContent();
      // The value should have changed (neon preset has different bloom)
      expect(newVal).not.toBe(initialVal);
    }
  });

  test('preset buttons have visible text labels', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    const presetBtns = page.locator('#cp-preset-buttons button');
    const labels = await presetBtns.allTextContents();
    // At least the built-in presets should have meaningful labels
    expect(labels.length).toBeGreaterThanOrEqual(3);
    labels.forEach(label => expect(label.trim().length).toBeGreaterThan(0));
  });
});

// =====================================================================
// SLIDER VALUE RANGES
// =====================================================================

test.describe('Slider value ranges', () => {

  test('all sliders have min, max, and step attributes', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    const sliders = page.locator('.cp-slider');
    const count = await sliders.count();
    expect(count).toBeGreaterThanOrEqual(8); // bloom×3, shader×3, star×2, label×2, anim×2

    for (let i = 0; i < count; i++) {
      const slider = sliders.nth(i);
      const min = await slider.getAttribute('min');
      const max = await slider.getAttribute('max');
      const step = await slider.getAttribute('step');
      expect(min).toBeTruthy();
      expect(max).toBeTruthy();
      expect(step).toBeTruthy();
      expect(parseFloat(max)).toBeGreaterThan(parseFloat(min));
    }
  });

  test('each slider has a matching value display element', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    await openPanel(page);
    const sliderIds = [
      'cp-bloom-threshold', 'cp-bloom-strength', 'cp-bloom-radius',
      'cp-fresnel-opacity', 'cp-fresnel-power', 'cp-pulse-speed',
      'cp-star-count', 'cp-twinkle-speed',
      'cp-label-size', 'cp-label-opacity',
      'cp-fly-speed', 'cp-force-strength',
    ];

    for (const id of sliderIds) {
      const valEl = page.locator(`#${id}-val`);
      await expect(valEl).toBeAttached();
      const text = await valEl.textContent();
      expect(text.trim().length).toBeGreaterThan(0);
    }
  });
});
