// E2E tests for materia node rendering (bd-c7d5z, bd-xnudd).
// Verifies: materia ShaderMaterial on cores, halo sprites, selection boost,
// breathing on in-progress nodes, agent nodes use lunar lander (not materia),
// and visual screenshot of materia orbs.
//
// Run: npx playwright test tests/materia-rendering.spec.js
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

test.describe('materia node rendering (bd-c7d5z)', () => {

  test('non-agent nodes use materia ShaderMaterial with uniforms', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;
      const nodes = b.graph.graphData().nodes;
      const info = [];

      for (const n of nodes) {
        if (n.issue_type === 'agent') continue;
        const obj = n.__threeObj;
        if (!obj) continue;

        obj.traverse(child => {
          if (child.userData.materiaCore && child.material) {
            const u = child.material.uniforms;
            if (u && u.materiaColor && u.coreIntensity && u.breathSpeed !== undefined && u.selected !== undefined) {
              info.push({
                id: n.id,
                type: n.issue_type,
                status: n.status,
                hasMateriaColor: !!u.materiaColor,
                coreIntensity: u.coreIntensity.value,
                breathSpeed: u.breathSpeed.value,
                selected: u.selected.value,
              });
            }
          }
        });
      }
      return { count: info.length, nodes: info };
    });

    expect(result).not.toBeNull();
    // MOCK_GRAPH has 14 non-agent nodes (epics, features, tasks, bugs, closed)
    expect(result.count).toBeGreaterThanOrEqual(10);

    // Each should have materia uniforms
    for (const n of result.nodes) {
      expect(n.hasMateriaColor).toBe(true);
      expect(typeof n.coreIntensity).toBe('number');
      expect(n.coreIntensity).toBeGreaterThan(0);
    }
  });

  test('in-progress nodes have breathing animation (breathSpeed > 0)', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;
      const nodes = b.graph.graphData().nodes;
      const breathing = [];
      const notBreathing = [];

      for (const n of nodes) {
        if (n.issue_type === 'agent') continue;
        const obj = n.__threeObj;
        if (!obj) continue;

        obj.traverse(child => {
          if (!child.userData.materiaCore || !child.material?.uniforms) return;
          const speed = child.material.uniforms.breathSpeed?.value;
          if (speed > 0) {
            breathing.push({ id: n.id, status: n.status, breathSpeed: speed });
          } else {
            notBreathing.push({ id: n.id, status: n.status, breathSpeed: speed });
          }
        });
      }
      return { breathing, notBreathing };
    });

    expect(result).not.toBeNull();
    // MOCK_GRAPH has bd-epic1, bd-feat1, bd-task3 as in_progress
    expect(result.breathing.length).toBeGreaterThanOrEqual(2);

    // All breathing nodes should be in_progress
    for (const n of result.breathing) {
      expect(n.status).toBe('in_progress');
      expect(n.breathSpeed).toBeGreaterThan(0);
    }

    // Non in_progress nodes should not breathe (bd-pe8k2: only in_progress pulses)
    for (const n of result.notBreathing) {
      expect(n.breathSpeed).toBe(0);
    }
  });

  test('halo sprites are present with additive blending', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;
      const THREE = window.__THREE;
      const nodes = b.graph.graphData().nodes;
      let haloCount = 0;
      let additiveCount = 0;
      let nodeCount = 0;

      for (const n of nodes) {
        if (n.issue_type === 'agent') continue;
        nodeCount++;
        const obj = n.__threeObj;
        if (!obj) continue;

        obj.traverse(child => {
          // Halo sprites are THREE.Sprite children (not label sprites)
          if (child.isSprite && child.material && !child.userData.nodeLabel) {
            haloCount++;
            if (child.material.blending === THREE.AdditiveBlending) {
              additiveCount++;
            }
          }
        });
      }
      return { nodeCount, haloCount, additiveCount };
    });

    expect(result).not.toBeNull();
    // Each non-agent node should have a halo sprite
    expect(result.haloCount).toBeGreaterThanOrEqual(10);
    // All halos should use additive blending
    expect(result.additiveCount).toBe(result.haloCount);
  });

  test('selecting a node sets materia selected uniform to 1.0', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    // Click bd-feat1 to select it
    await page.evaluate(() => {
      const b = window.__beads3d;
      const target = b.graph.graphData().nodes.find(n => n.id === 'bd-feat1');
      if (target) b.graph.onNodeClick()(target, { preventDefault: () => {} });
    });
    await page.waitForTimeout(1000);

    const result = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;
      const nodes = b.graph.graphData().nodes;
      const selected = [];
      const unselected = [];

      for (const n of nodes) {
        if (n.issue_type === 'agent') continue;
        const obj = n.__threeObj;
        if (!obj) continue;

        obj.traverse(child => {
          if (!child.userData.materiaCore || !child.material?.uniforms?.selected) return;
          const val = child.material.uniforms.selected.value;
          if (n.id === 'bd-feat1') {
            selected.push({ id: n.id, selected: val });
          } else {
            unselected.push({ id: n.id, selected: val });
          }
        });
      }
      return { selected, unselected };
    });

    expect(result).not.toBeNull();
    // Selected node should have selected=1.0
    expect(result.selected.length).toBe(1);
    expect(result.selected[0].selected).toBe(1.0);

    // Unselected nodes should have selected=0.0
    for (const n of result.unselected) {
      expect(n.selected).toBe(0.0);
    }
  });

  test('agent nodes use lunar lander mesh, not materia', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;
      const nodes = b.graph.graphData().nodes;
      const agents = [];

      for (const n of nodes) {
        if (n.issue_type !== 'agent') continue;
        const obj = n.__threeObj;
        if (!obj) continue;

        let hasMateriaCore = false;
        let hasAgentGlow = false;
        let meshCount = 0;

        obj.traverse(child => {
          if (child.userData.materiaCore) hasMateriaCore = true;
          if (child.userData.agentGlow) hasAgentGlow = true;
          if (child.isMesh) meshCount++;
        });

        agents.push({
          id: n.id,
          hasMateriaCore,
          hasAgentGlow,
          meshCount,
        });
      }
      return agents;
    });

    // MOCK_GRAPH has agent:alice and agent:bob (+ possible synthesized agents)
    expect(result.length).toBeGreaterThanOrEqual(2);

    for (const agent of result) {
      // Agent nodes should NOT have materia cores
      expect(agent.hasMateriaCore).toBe(false);
      // Agent nodes should have Fresnel agentGlow
      expect(agent.hasAgentGlow).toBe(true);
      // Agent nodes should have multiple meshes (cabin, window, descent, nozzle, legs, antenna, tip)
      expect(agent.meshCount).toBeGreaterThanOrEqual(5);
    }
  });

  test('closed nodes have dimmer materia (lower intensity and opacity)', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);

    const result = await page.evaluate(() => {
      const b = window.__beads3d;
      if (!b || !b.graph) return null;
      const nodes = b.graph.graphData().nodes;
      const closed = [];
      const open = [];

      for (const n of nodes) {
        if (n.issue_type === 'agent') continue;
        const obj = n.__threeObj;
        if (!obj) continue;

        obj.traverse(child => {
          if (!child.userData.materiaCore || !child.material?.uniforms) return;
          const u = child.material.uniforms;
          const info = {
            id: n.id,
            status: n.status,
            coreIntensity: u.coreIntensity?.value,
            opacity: u.opacity?.value,
          };
          if (n.status === 'closed') {
            closed.push(info);
          } else if (n.status !== 'blocked') {
            open.push(info);
          }
        });
      }
      return { closed, open };
    });

    expect(result).not.toBeNull();
    // MOCK_GRAPH has bd-old1 and bd-old2 as closed (some may be filtered by age)
    expect(result.closed.length).toBeGreaterThanOrEqual(1);
    expect(result.open.length).toBeGreaterThanOrEqual(5);

    // Closed nodes should have lower intensity and opacity than open nodes
    const avgClosedIntensity = result.closed.reduce((s, n) => s + n.coreIntensity, 0) / result.closed.length;
    const avgOpenIntensity = result.open.reduce((s, n) => s + n.coreIntensity, 0) / result.open.length;
    expect(avgClosedIntensity).toBeLessThan(avgOpenIntensity);

    const avgClosedOpacity = result.closed.reduce((s, n) => s + n.opacity, 0) / result.closed.length;
    const avgOpenOpacity = result.open.reduce((s, n) => s + n.opacity, 0) / result.open.length;
    expect(avgClosedOpacity).toBeLessThan(avgOpenOpacity);
  });

  test('visual: materia rendering screenshot', async ({ page }) => {
    await mockAPI(page);
    await page.goto('/');
    await waitForGraph(page);
    await page.waitForTimeout(1000);

    // Force a couple render frames for WebGL stability
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
    await page.waitForTimeout(500);

    await expect(page).toHaveScreenshot('materia-rendering.png', {
      animations: 'disabled',
      maxDiffPixelRatio: 0.45,
    });
  });
});
