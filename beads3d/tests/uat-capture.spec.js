// UAT screenshot capture for Claude vision evaluation.
// Captures screenshots across multiple scenarios for qualitative review.
//
// Run: npx playwright test tests/uat-capture.spec.js
// Output: Screenshots saved to UAT_SCREENSHOT_DIR (default /tmp/beads3d-uat-screenshots)
// Manifest: manifest.json with scenario metadata for downstream evaluation.

import { test } from '@playwright/test';
import { MOCK_GRAPH, MOCK_PING, MOCK_SHOW, MOCK_MULTI_AGENT_GRAPH, MOCK_LARGE_GRAPH } from './fixtures.js';
import * as fs from 'fs';
import * as path from 'path';

const OUTPUT_DIR = process.env.UAT_SCREENSHOT_DIR || '/tmp/beads3d-uat-screenshots';

// Manifest accumulator — written to disk in afterAll
const manifest = [];

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

// Wait for graph to render and settle, then zoom to fit
async function waitForGraphReady(page) {
  await page.waitForSelector('#status.connected', { timeout: 15000 });
  await page.waitForTimeout(3000);
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

// Save screenshot and record in manifest
async function captureScenario(page, scenario) {
  const filename = `${String(scenario.number).padStart(2, '0')}-${scenario.slug}.png`;
  const filepath = path.join(OUTPUT_DIR, filename);
  await page.screenshot({ path: filepath, fullPage: false });
  manifest.push({
    number: scenario.number,
    name: scenario.name,
    slug: scenario.slug,
    filename,
    path: filepath,
    graph: scenario.graph || 'MOCK_GRAPH',
    criteria: scenario.criteria,
  });
}

test.describe('UAT screenshot capture', () => {

  test.beforeAll(() => {
    fs.mkdirSync(OUTPUT_DIR, { recursive: true });
  });

  test.afterAll(() => {
    const manifestPath = path.join(OUTPUT_DIR, 'manifest.json');
    fs.writeFileSync(manifestPath, JSON.stringify(manifest, null, 2));
  });

  test('01 — default view, small graph, labels off', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);
    await captureScenario(page, {
      number: 1,
      name: 'Default view (small graph, labels off)',
      slug: 'default-small-no-labels',
      criteria: [
        'All nodes visible and distinguishable by color/shape',
        'Star field background renders (not solid black)',
        'Nodes are spatially distributed (not clumped in one spot)',
        'HUD stats bar visible at top',
        'No visual artifacts or rendering glitches',
      ],
    });
  });

  test('02 — default view, small graph, labels on', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);
    // Labels are ON by default (bd-oypa2) — wait for LOD pass
    await page.waitForTimeout(500);
    await forceRender(page);
    await page.waitForTimeout(200);
    await captureScenario(page, {
      number: 2,
      name: 'Default view (small graph, labels on)',
      slug: 'default-small-labels-on',
      criteria: [
        'Node labels are visible and readable',
        'No severe label overlap (labels should not obscure each other)',
        'Labels appear near their corresponding nodes',
        'Label text is crisp (no blurring or aliasing artifacts)',
        'LOD system shows labels on important nodes',
      ],
    });
  });

  test('03 — multi-agent graph, labels off', async ({ page }) => {
    await mockAPI(page, MOCK_MULTI_AGENT_GRAPH);
    await page.goto('/');
    await waitForGraphReady(page);
    await captureScenario(page, {
      number: 3,
      name: 'Multi-agent view (23 nodes, labels off)',
      slug: 'multi-agent-no-labels',
      graph: 'MOCK_MULTI_AGENT_GRAPH',
      criteria: [
        'Agent nodes (orange boxes) are visible and distinguishable from issue nodes',
        'Assignment edges connect agents to their beads',
        'Epic node is larger/prominent (purple)',
        'Blocked/open/in_progress nodes have distinct colors',
        'Graph is not overcrowded — nodes are spatially separated',
      ],
    });
  });

  test('04 — multi-agent graph, labels on', async ({ page }) => {
    await mockAPI(page, MOCK_MULTI_AGENT_GRAPH);
    await page.goto('/');
    await waitForGraphReady(page);
    // Labels are ON by default (bd-oypa2) — wait for LOD pass
    await page.waitForTimeout(500);
    await forceRender(page);
    await page.waitForTimeout(200);
    await captureScenario(page, {
      number: 4,
      name: 'Multi-agent view (23 nodes, labels on)',
      slug: 'multi-agent-labels-on',
      graph: 'MOCK_MULTI_AGENT_GRAPH',
      criteria: [
        'LOD budget limits visible labels (not all 23 labeled at once)',
        'Visible labels are readable and not severely overlapping',
        'Agent name labels are distinguishable',
        'Label opacity varies by priority (LOD fade)',
        'Dense areas have fewer labels than sparse areas',
      ],
    });
  });

  test('05 — large graph, labels off', async ({ page }) => {
    await mockAPI(page, MOCK_LARGE_GRAPH);
    await page.goto('/');
    await waitForGraphReady(page);
    await captureScenario(page, {
      number: 5,
      name: 'Large graph (100+ nodes, labels off)',
      slug: 'large-graph-no-labels',
      graph: 'MOCK_LARGE_GRAPH',
      criteria: [
        'Graph renders without visual artifacts despite high node count',
        'Node colors/shapes remain distinguishable',
        'Performance: no visible frame drops or incomplete rendering',
        'Spatial distribution is reasonable (no single massive cluster)',
        'Edge density does not completely obscure nodes',
      ],
    });
  });

  test('06 — large graph, labels on', async ({ page }) => {
    await mockAPI(page, MOCK_LARGE_GRAPH);
    await page.goto('/');
    await waitForGraphReady(page);
    // Labels are ON by default (bd-oypa2) — wait for LOD pass
    await page.waitForTimeout(500);
    await forceRender(page);
    await page.waitForTimeout(200);
    await captureScenario(page, {
      number: 6,
      name: 'Large graph (100+ nodes, labels on)',
      slug: 'large-graph-labels-on',
      graph: 'MOCK_LARGE_GRAPH',
      criteria: [
        'LOD budget aggressively limits labels (far fewer than 100)',
        'Visible labels are still readable',
        'No label rendering causes frame artifacts',
        'Labels do not completely cover the graph',
        'Higher-priority/in-progress nodes get label preference',
      ],
    });
  });

  test('07 — bloom post-processing enabled', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);
    await page.keyboard.press('b');
    await page.waitForTimeout(1000);
    await forceRender(page);
    await forceRender(page);
    await captureScenario(page, {
      number: 7,
      name: 'Bloom post-processing enabled',
      slug: 'bloom-enabled',
      criteria: [
        'Visible glow/halo effect around bright nodes',
        'Bloom does not wash out node details completely',
        'In-progress nodes (amber) glow more prominently',
        'Star field is enhanced by bloom effect',
        'Overall scene has a softer, cinematic quality',
      ],
    });
  });

  test('08 — zoomed-in close-up with labels', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);
    // Labels are ON by default (bd-oypa2) — wait for LOD pass
    await page.waitForTimeout(500);
    // Zoom camera close to Epic 1 cluster
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return;
      const epic = b.graph.graphData().nodes.find(n => n.id === 'bd-epic1');
      if (!epic) return;
      b.graph.cameraPosition(
        { x: (epic.x || 0), y: (epic.y || 0) + 20, z: (epic.z || 0) + 60 },
        { x: (epic.x || 0), y: (epic.y || 0), z: (epic.z || 0) },
        0
      );
      const controls = b.graph.controls();
      if (controls) controls.update();
    });
    await forceRender(page);
    await page.waitForTimeout(300);
    await forceRender(page);
    await captureScenario(page, {
      number: 8,
      name: 'Zoomed-in close-up (Epic 1 cluster, labels on)',
      slug: 'closeup-epic1-labels',
      criteria: [
        'Individual node shapes clearly visible (spheres for issues, box for agents)',
        'Materia shader effect visible on in_progress nodes (translucent glow)',
        'Labels are large and readable at this zoom level',
        'Dependency edges are clearly visible between nodes',
        'Node colors accurately represent status (amber=in_progress, green=open, etc.)',
      ],
    });
  });

  test('09 — node selected with highlight', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);
    // Labels are ON by default (bd-oypa2)
    await page.waitForTimeout(500);
    // Select bd-feat1 via API
    await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-feat1');
      if (target) b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });
    await page.waitForTimeout(1500);
    await forceRender(page);
    await captureScenario(page, {
      number: 9,
      name: 'Node selected with highlight + labels',
      slug: 'node-selected-highlight',
      criteria: [
        'Selected node has a visible selection ring/highlight effect',
        'Connected nodes remain at full brightness',
        'Unconnected nodes are visibly dimmed but still present',
        'Label on selected node is visible',
        'Detail panel or sidebar visible (if applicable)',
      ],
    });
  });

  test('10 — HUD elements visible', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);
    await captureScenario(page, {
      number: 10,
      name: 'HUD overlay elements',
      slug: 'hud-elements',
      criteria: [
        'Stats bar visible at top (total/open/in_progress/blocked/closed counts)',
        'Search box visible and accessible',
        'Connection status indicator visible (green=connected)',
        'Filter/type buttons visible',
        'HUD elements do not obscure the main graph visualization',
      ],
    });
  });

  test('11 — DAG layout mode', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);
    await page.keyboard.press('2');
    await page.waitForTimeout(3000);
    await forceRender(page);
    await captureScenario(page, {
      number: 11,
      name: 'DAG layout mode',
      slug: 'layout-dag',
      criteria: [
        'Nodes arranged in hierarchical/layered structure',
        'Parent-child relationships flow top-to-bottom or left-to-right',
        'Dependency edges follow the hierarchy direction',
        'No excessive node overlap within layers',
        'Layout mode visually distinct from free layout',
      ],
    });
  });

  test('12 — radial layout mode', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);
    await page.keyboard.press('4');
    await page.waitForTimeout(3000);
    await forceRender(page);
    await captureScenario(page, {
      number: 12,
      name: 'Radial layout mode',
      slug: 'layout-radial',
      criteria: [
        'Nodes arranged in concentric rings or radial pattern',
        'Central node(s) represent root/epic',
        'Outer rings contain leaf/dependent nodes',
        'Layout is visually balanced and symmetric',
        'Layout mode visually distinct from free and DAG layouts',
      ],
    });
  });

  test('13 — cluster layout mode', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);
    await page.keyboard.press('5');
    await page.waitForTimeout(3000);
    await forceRender(page);
    await captureScenario(page, {
      number: 13,
      name: 'Cluster layout mode',
      slug: 'layout-cluster',
      criteria: [
        'Nodes grouped into visible clusters (by epic/type)',
        'Cluster boundaries are apparent',
        'Inter-cluster edges are visible',
        'Intra-cluster nodes are close together',
        'Layout is visually distinct from other modes',
      ],
    });
  });

  test('14 — molecule focus view', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/?molecule=bd-epic1');
    await waitForGraphReady(page);
    // Labels are ON by default (bd-oypa2) — wait for LOD pass
    await page.waitForTimeout(500);
    await forceRender(page);
    await captureScenario(page, {
      number: 14,
      name: 'Molecule focus (bd-epic1 subgraph)',
      slug: 'molecule-focus-epic1',
      criteria: [
        'bd-epic1 and its connected nodes are highlighted/zoomed',
        'Non-connected nodes are dimmed or hidden',
        'Labels visible on focused subgraph',
        'Dependency edges within subgraph are clear',
        'Camera positioned to frame the focused subgraph',
      ],
    });
  });

  test('15 — context menu on right-click', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraphReady(page);
    // Trigger context menu via API
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
    await captureScenario(page, {
      number: 15,
      name: 'Context menu (right-click node)',
      slug: 'context-menu',
      criteria: [
        'Context menu is visible and positioned near click location',
        'Menu options are readable text',
        'Menu does not extend off-screen',
        'Menu has clear visual boundary (shadow/border)',
        'Background graph is still partially visible behind menu',
      ],
    });
  });
});
