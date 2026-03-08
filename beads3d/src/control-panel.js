// --- bd-69y6v: Control Panel ---
// Extracted from main.js to reduce monolith size.

import { Color } from 'three';
import { createStarField } from './shaders.js';
import { _vfxConfig, setVfxIntensity, applyVfxPreset } from './vfx.js';
import { setLeftSidebarOpen } from './left-sidebar.js';

// Dependency injection — set by main.js before initControlPanel()
let _deps = {};

/**
 * Inject dependencies from main.js.
 *
 * @param {Object} deps
 * @param {Function} deps.getGraph        - () => ForceGraph3D instance
 * @param {Function} deps.getGraphData    - () => { nodes, links }
 * @param {Function} deps.getBloomPass    - () => UnrealBloomPass instance
 * @param {Function} deps.setMaxEdgesPerNode - (v: number) => void
 * @param {Function} deps.setAgentTetherStrength - (v: number) => void
 * @param {Function} deps.setMinimapVisible - (v: boolean) => void
 * @param {Function} deps.getDepTypeHidden - () => Set
 * @param {Function} deps.applyFilters    - () => void
 * @param {Function} deps.refresh         - () => void
 * @param {Function} deps.toggleLabels    - () => void
 * @param {Function} deps.getLabelsVisible - () => boolean
 * @param {Function} deps.setLayout       - (mode: string) => void
 * @param {Object}   deps.api             - BeadsAPI instance (configGet, configSet)
 */
export function setControlPanelDeps(deps) {
  _deps = deps;
}

let controlPanelOpen = false;

/**
 * Returns whether the control panel is currently open.
 *
 * @returns {boolean}
 */
export function getControlPanelOpen() {
  return controlPanelOpen;
}

/**
 * Toggles the control panel open/closed state.
 *
 * @returns {void}
 */
export function toggleControlPanel() {
  const panel = document.getElementById('control-panel');
  if (!panel) return;
  controlPanelOpen = !controlPanelOpen;
  panel.classList.toggle('open', controlPanelOpen);
}

/**
 * Initializes the control panel: wires up all sliders, toggles, presets,
 * theme import/export, and config bead persistence.
 *
 * @returns {void}
 */
export function initControlPanel() {
  const panel = document.getElementById('control-panel');
  if (!panel) return;

  // Convenience accessors from injected deps
  const getGraph = () => _deps.getGraph?.();
  const getGraphData = () => _deps.getGraphData?.();
  const getBloomPass = () => _deps.getBloomPass?.();

  // Toggle button
  const btn = document.getElementById('btn-control-panel');
  if (btn) btn.onclick = () => toggleControlPanel();

  // Close button
  const closeBtn = document.getElementById('cp-close');
  if (closeBtn)
    closeBtn.onclick = () => {
      controlPanelOpen = false;
      panel.classList.remove('open');
    };

  // bd-7zczp: Focus trap + Escape to close
  panel.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && controlPanelOpen) {
      e.stopPropagation();
      controlPanelOpen = false;
      panel.classList.remove('open');
      const opener = document.getElementById('btn-control-panel');
      if (opener) opener.focus();
      return;
    }
    if (e.key === 'Tab' && controlPanelOpen) {
      const focusable = panel.querySelectorAll('button, [href], input, select, [tabindex]:not([tabindex="-1"])');
      if (!focusable.length) return;
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    }
  });

  // Collapsible sections
  panel.querySelectorAll('.cp-section-header').forEach((header) => {
    header.setAttribute('role', 'button');
    header.setAttribute('tabindex', '0');
    const section = header.parentElement;
    header.setAttribute('aria-expanded', !section.classList.contains('collapsed'));
    const toggle = () => {
      section.classList.toggle('collapsed');
      header.setAttribute('aria-expanded', !section.classList.contains('collapsed'));
    };
    header.onclick = toggle;
    header.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        toggle();
      }
    });
  });

  // bd-7zczp: ARIA + keyboard support for toggle switches
  panel.querySelectorAll('.cp-toggle').forEach((toggle) => {
    toggle.setAttribute('role', 'switch');
    toggle.setAttribute('tabindex', '0');
    toggle.setAttribute('aria-checked', toggle.classList.contains('on'));
    // Watch for class changes from click handlers to keep aria-checked in sync
    const observer = new MutationObserver(() => {
      toggle.setAttribute('aria-checked', toggle.classList.contains('on'));
    });
    observer.observe(toggle, { attributes: true, attributeFilter: ['class'] });
    // Keyboard activation
    toggle.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        toggle.click();
      }
    });
  });

  // Helper: wire a slider to its value display and a callback
  function wireSlider(id, cb) {
    const slider = document.getElementById(id);
    const valEl = document.getElementById(id + '-val');
    if (!slider) return;
    slider.addEventListener('input', () => {
      const v = parseFloat(slider.value);
      if (valEl) valEl.textContent = Number.isInteger(v) ? v : v.toFixed(2);
      cb(v);
    });
  }

  // Bloom controls
  wireSlider('cp-bloom-threshold', (v) => {
    const bp = getBloomPass();
    if (bp) bp.threshold = v;
  });
  wireSlider('cp-bloom-strength', (v) => {
    const bp = getBloomPass();
    if (bp) bp.strength = v;
  });
  wireSlider('cp-bloom-radius', (v) => {
    const bp = getBloomPass();
    if (bp) bp.radius = v;
  });

  // Shader controls — update fresnel materials on all glow shells
  wireSlider('cp-fresnel-opacity', (v) => {
    const graph = getGraph();
    if (!graph) return;
    graph.scene().traverse((obj) => {
      if (obj.material?.uniforms?.opacity && obj.material?.uniforms?.power) {
        obj.material.uniforms.opacity.value = v;
      }
    });
  });
  wireSlider('cp-fresnel-power', (v) => {
    const graph = getGraph();
    if (!graph) return;
    graph.scene().traverse((obj) => {
      if (obj.material?.uniforms?.opacity && obj.material?.uniforms?.power) {
        obj.material.uniforms.power.value = v;
      }
    });
  });
  wireSlider('cp-pulse-speed', (v) => {
    // Update breathSpeed on in-progress materia cores (bd-pe8k2, bd-b3ujw)
    const graph = getGraph();
    if (!graph) return;
    graph.scene().traverse((obj) => {
      if (obj.material?.uniforms?.breathSpeed && obj.material.uniforms.breathSpeed.value > 0) {
        obj.material.uniforms.breathSpeed.value = v;
      }
      if (obj.material?.uniforms?.pulseCycle) {
        obj.material.uniforms.pulseCycle.value = v;
      }
    });
  });

  // Star field controls
  wireSlider('cp-star-count', (v) => {
    const graph = getGraph();
    if (!graph) return;
    const scene = graph.scene();
    // Remove existing star field
    scene.traverse((obj) => {
      if (obj.userData?.isStarField) scene.remove(obj);
    });
    // Add new one with updated count
    if (v > 0) {
      const stars = createStarField(v, 500);
      scene.add(stars);
    }
  });
  wireSlider('cp-twinkle-speed', (v) => {
    // Update twinkleSpeed uniform on star field (bd-b3ujw)
    const graph = getGraph();
    if (!graph) return;
    graph.scene().traverse((obj) => {
      if (obj.userData?.isStarField && obj.material?.uniforms?.twinkleSpeed) {
        obj.material.uniforms.twinkleSpeed.value = v;
      }
    });
  });

  // Background color
  const bgColor = document.getElementById('cp-bg-color');
  if (bgColor) {
    bgColor.addEventListener('input', () => {
      const graph = getGraph();
      if (!graph) return;
      graph.scene().background = new Color(bgColor.value);
    });
  }

  // Node color overrides — stored in a config object
  window.__beads3d_colorOverrides = {};
  let _colorDebounce = null;
  const colorMap = {
    'cp-color-open': 'open',
    'cp-color-active': 'in_progress',
    'cp-color-blocked': 'blocked',
    'cp-color-agent': 'agent',
    'cp-color-epic': 'epic',
    'cp-color-jack': 'jack',
  };
  for (const [elId, key] of Object.entries(colorMap)) {
    const el = document.getElementById(elId);
    if (!el) continue;
    el.addEventListener('input', () => {
      window.__beads3d_colorOverrides[key] = el.value;
      // Debounced re-render — color pickers fire rapidly
      clearTimeout(_colorDebounce);
      _colorDebounce = setTimeout(() => {
        const graph = getGraph();
        if (graph) graph.nodeThreeObject(graph.nodeThreeObject());
      }, 150);
    });
  }

  // Label controls (bd-oypa2: wire to actual label rendering)
  let _labelSizeDebounce;
  wireSlider('cp-label-size', (v) => {
    window.__beads3d_labelSize = v;
    // Regenerate all labels with new size (debounced — slider fires rapidly)
    clearTimeout(_labelSizeDebounce);
    _labelSizeDebounce = setTimeout(() => {
      const graph = getGraph();
      if (graph) graph.nodeThreeObject(graph.nodeThreeObject());
    }, 200);
  });
  wireSlider('cp-label-opacity', (v) => {
    window.__beads3d_labelOpacity = v;
    // Apply opacity to all existing label sprites immediately
    const graph = getGraph();
    if (graph) {
      graph.scene().traverse((child) => {
        if (child.userData?.nodeLabel && child.material) {
          child.material.opacity = v;
        }
      });
    }
  });

  // Label show/hide toggle (bd-oypa2)
  const labelToggle = document.getElementById('cp-label-toggle');
  if (labelToggle) {
    labelToggle.addEventListener('click', () => {
      labelToggle.classList.toggle('on');
      _deps.toggleLabels?.();
      // Sync the toolbar button
      const btn = document.getElementById('btn-labels');
      if (btn) btn.classList.toggle('active', _deps.getLabelsVisible?.());
    });
  }

  // Label content toggles (bd-xnh54) — choose which fields to show
  let _labelContentDebounce;
  for (const field of ['id', 'title', 'status']) {
    const toggleEl = document.getElementById(`cp-label-show-${field}`);
    if (!toggleEl) continue;
    toggleEl.addEventListener('click', () => {
      toggleEl.classList.toggle('on');
      const isOn = toggleEl.classList.contains('on');
      window[`__beads3d_labelShow${field.charAt(0).toUpperCase() + field.slice(1)}`] = isOn;
      // Regenerate labels with new content (debounced)
      clearTimeout(_labelContentDebounce);
      _labelContentDebounce = setTimeout(() => {
        const graph = getGraph();
        if (graph) graph.nodeThreeObject(graph.nodeThreeObject());
      }, 200);
    });
  }

  // Label style controls (bd-j8ala) — background opacity and border
  wireSlider('cp-label-bg-opacity', (v) => {
    window.__beads3d_labelBgOpacity = v;
    clearTimeout(_labelContentDebounce);
    _labelContentDebounce = setTimeout(() => {
      const graph = getGraph();
      if (graph) graph.nodeThreeObject(graph.nodeThreeObject());
    }, 200);
  });
  {
    const borderToggle = document.getElementById('cp-label-border');
    if (borderToggle)
      borderToggle.addEventListener('click', () => {
        borderToggle.classList.toggle('on');
        window.__beads3d_labelBorder = borderToggle.classList.contains('on');
        clearTimeout(_labelContentDebounce);
        _labelContentDebounce = setTimeout(() => {
          const graph = getGraph();
          if (graph) graph.nodeThreeObject(graph.nodeThreeObject());
        }, 200);
      });
  }

  // Edge type toggles (bd-a0vbd): show/hide specific edge types to reduce graph density
  const EDGE_TOGGLE_MAP = {
    'cp-edge-blocks': 'blocks',
    'cp-edge-parent-child': 'parent-child',
    'cp-edge-child-of': 'child-of', // kd-XGgiokgQBH
    'cp-edge-waits-for': 'waits-for',
    'cp-edge-relates-to': 'relates-to',
    'cp-edge-action-item': 'action-item', // kd-XGgiokgQBH
    'cp-edge-escalate': 'escalate', // kd-XGgiokgQBH
    'cp-edge-duplicate': 'duplicate', // kd-XGgiokgQBH
    'cp-edge-jira-link': 'jira-link', // kd-XGgiokgQBH
    'cp-edge-assigned-to': 'assigned_to',
    'cp-edge-rig-conflict': 'rig_conflict',
  };
  for (const [elId, depType] of Object.entries(EDGE_TOGGLE_MAP)) {
    const toggleEl = document.getElementById(elId);
    if (!toggleEl) continue;
    toggleEl.addEventListener('click', () => {
      toggleEl.classList.toggle('on');
      const depTypeHidden = _deps.getDepTypeHidden?.();
      if (depTypeHidden) {
        if (toggleEl.classList.contains('on')) {
          depTypeHidden.delete(depType);
        } else {
          depTypeHidden.add(depType);
        }
      }
      _deps.applyFilters?.();
    });
  }

  // Max edges per node slider (bd-ke2xc)
  let _edgeCapDebounce;
  const edgeCapSlider = document.getElementById('cp-edge-max-per-node');
  const edgeCapVal = document.getElementById('cp-edge-max-per-node-val');
  if (edgeCapSlider) {
    edgeCapSlider.addEventListener('input', () => {
      const v = parseInt(edgeCapSlider.value, 10);
      _deps.setMaxEdgesPerNode?.(v);
      if (edgeCapVal) edgeCapVal.textContent = v === 0 ? 'off' : String(v);
      // Debounced re-fetch to apply edge cap
      clearTimeout(_edgeCapDebounce);
      _edgeCapDebounce = setTimeout(() => _deps.refresh?.(), 500);
    });
  }

  // Layout controls (bd-a1odd)
  wireSlider('cp-force-strength', (v) => {
    const graph = getGraph();
    if (graph) {
      graph.d3Force('charge')?.strength(-v);
      graph.d3ReheatSimulation();
    }
  });
  wireSlider('cp-link-distance', (v) => {
    const graph = getGraph();
    if (graph) {
      graph.d3Force('link')?.distance(v);
      graph.d3ReheatSimulation();
    }
  });
  wireSlider('cp-center-force', (v) => {
    const graph = getGraph();
    if (graph) {
      graph.d3Force('center')?.strength(v);
      graph.d3ReheatSimulation();
    }
  });
  wireSlider('cp-collision-radius', (v) => {
    const graph = getGraph();
    if (graph) {
      if (v > 0) {
        graph.d3Force('collision', (alpha) => {
          const graphData = getGraphData();
          const nodes = graphData.nodes;
          for (let i = 0; i < nodes.length; i++) {
            for (let j = i + 1; j < nodes.length; j++) {
              const a = nodes[i],
                b = nodes[j];
              if (a._hidden || b._hidden) continue;
              const dx = (b.x || 0) - (a.x || 0),
                dy = (b.y || 0) - (a.y || 0),
                dz = (b.z || 0) - (a.z || 0);
              const dist = Math.sqrt(dx * dx + dy * dy + dz * dz) || 1;
              if (dist < v * 2) {
                const f = ((v * 2 - dist) / dist) * alpha * 0.5;
                a.vx -= dx * f;
                a.vy -= dy * f;
                a.vz -= dz * f;
                b.vx += dx * f;
                b.vy += dy * f;
                b.vz += dz * f;
              }
            }
          }
        });
      } else {
        graph.d3Force('collision', null);
      }
      graph.d3ReheatSimulation();
    }
  });
  wireSlider('cp-alpha-decay', (v) => {
    const graph = getGraph();
    if (graph) graph.d3AlphaDecay(v);
  });
  // Agent tether: slider controls pull strength (bd-uzj5j)
  wireSlider('cp-agent-tether', (v) => {
    _deps.setAgentTetherStrength?.(v);
    const graph = getGraph();
    if (graph && v > 0) graph.d3ReheatSimulation();
  });
  // Layout mode dropdown (bd-a1odd)
  {
    const sel = document.getElementById('cp-layout-mode');
    if (sel) sel.addEventListener('change', () => _deps.setLayout?.(sel.value));
  }

  // Animation controls
  wireSlider('cp-fly-speed', (v) => {
    window.__beads3d_flySpeed = v;
  });

  // Particles / VFX controls (bd-hr5om)
  wireSlider('cp-orbit-speed', (v) => {
    _vfxConfig.orbitSpeed = v;
  });
  wireSlider('cp-orbit-rate', (v) => {
    _vfxConfig.orbitRate = v;
  });
  wireSlider('cp-orbit-size', (v) => {
    _vfxConfig.orbitSize = v;
  });
  wireSlider('cp-hover-rate', (v) => {
    _vfxConfig.hoverRate = v;
  });
  wireSlider('cp-stream-rate', (v) => {
    _vfxConfig.streamRate = v;
  });
  wireSlider('cp-stream-speed', (v) => {
    _vfxConfig.streamSpeed = v;
  });
  wireSlider('cp-particle-lifetime', (v) => {
    _vfxConfig.particleLifetime = v;
  });
  wireSlider('cp-selection-glow', (v) => {
    _vfxConfig.selectionGlow = v;
  });

  // VFX intensity slider + preset buttons (bd-dnuky)
  wireSlider('cp-vfx-intensity', (v) => {
    setVfxIntensity(v);
    _vfxConfig.intensity = v;
  });
  const presetButtons = {
    'cp-vfx-subtle': 'subtle',
    'cp-vfx-normal': 'normal',
    'cp-vfx-dramatic': 'dramatic',
    'cp-vfx-max': 'maximum',
  };
  for (const [id, preset] of Object.entries(presetButtons)) {
    const btn = document.getElementById(id);
    if (btn) btn.addEventListener('click', () => {
      applyVfxPreset(preset);
      // Sync slider to match preset intensity
      const slider = document.getElementById('cp-vfx-intensity');
      const valEl = document.getElementById('cp-vfx-intensity-val');
      const presetVal = { subtle: 0.25, normal: 1.0, dramatic: 2.0, maximum: 4.0 }[preset];
      if (slider) slider.value = presetVal;
      if (valEl) valEl.textContent = presetVal.toFixed(2);
    });
  }

  // Camera controls (bd-bz1ba)
  wireSlider('cp-camera-fov', (v) => {
    const graph = getGraph();
    if (!graph) return;
    const camera = graph.camera();
    camera.fov = v;
    camera.updateProjectionMatrix();
  });
  wireSlider('cp-camera-rotate-speed', (v) => {
    const graph = getGraph();
    if (!graph) return;
    const controls = graph.controls();
    if (controls) controls.autoRotateSpeed = v;
  });
  wireSlider('cp-camera-zoom-speed', (v) => {
    const graph = getGraph();
    if (!graph) return;
    const controls = graph.controls();
    if (controls) controls.zoomSpeed = v;
  });
  wireSlider('cp-camera-near', (v) => {
    const graph = getGraph();
    if (!graph) return;
    const camera = graph.camera();
    camera.near = v;
    camera.updateProjectionMatrix();
  });
  wireSlider('cp-camera-far', (v) => {
    const graph = getGraph();
    if (!graph) return;
    const camera = graph.camera();
    camera.far = v;
    camera.updateProjectionMatrix();
  });
  // Auto-rotate toggle
  const autoRotateToggle = document.getElementById('cp-camera-autorotate');
  if (autoRotateToggle) {
    autoRotateToggle.onclick = () => {
      autoRotateToggle.classList.toggle('on');
      const graph = getGraph();
      if (!graph) return;
      const controls = graph.controls();
      if (controls) controls.autoRotate = autoRotateToggle.classList.contains('on');
    };
  }

  // bd-4hggh: HUD Visibility toggles
  // Track which HUD elements the user has hidden via toggles (prevents
  // other code from re-showing them, e.g. tooltip on hover).
  const _hudHidden = {};

  // Local state for right sidebar collapsed (mirrors right-sidebar.js internal state)
  let rightSidebarCollapsed = false;

  panel.querySelectorAll('.cp-toggle[data-target]').forEach((toggle) => {
    const targetId = toggle.dataset.target;
    toggle.addEventListener('click', () => {
      const isOn = toggle.classList.toggle('on');
      _hudHidden[targetId] = !isOn;
      const el = document.getElementById(targetId);
      if (!el) return;

      if (targetId === 'minimap') {
        // Minimap has both canvas and label
        el.style.display = isOn ? 'block' : 'none';
        const label = document.getElementById('minimap-label');
        if (label) label.style.display = isOn ? 'block' : 'none';
        _deps.setMinimapVisible?.(isOn);
      } else if (targetId === 'left-sidebar') {
        if (isOn) {
          el.classList.add('open');
          setLeftSidebarOpen(true);
        } else {
          el.classList.remove('open');
          setLeftSidebarOpen(false);
        }
      } else if (targetId === 'right-sidebar') {
        if (isOn) {
          el.classList.remove('collapsed');
          rightSidebarCollapsed = false;
        } else {
          el.classList.add('collapsed');
          rightSidebarCollapsed = true;
        }
        // Shift controls bar
        const controls = document.getElementById('controls');
        if (controls) controls.classList.toggle('sidebar-collapsed', !isOn);
      } else {
        el.style.display = isOn ? '' : 'none';
      }
    });
  });

  // Expose _hudHidden so tooltip code can check it
  window.__beads3d_hudHidden = _hudHidden;

  // bd-krh7y: Theme presets
  const BUILT_IN_PRESETS = {
    'Default Dark': {
      'cp-bloom-threshold': 0.35,
      'cp-bloom-strength': 0.7,
      'cp-bloom-radius': 0.4,
      'cp-fresnel-opacity': 0.4,
      'cp-fresnel-power': 2.0,
      'cp-pulse-speed': 4.0,
      'cp-star-count': 2000,
      'cp-twinkle-speed': 1.0,
      'cp-bg-color': '#000005',
      'cp-color-open': '#2d8a4e',
      'cp-color-active': '#d4a017',
      'cp-color-blocked': '#d04040',
      'cp-color-agent': '#ff6b35',
      'cp-color-epic': '#8b45a6',
      'cp-color-jack': '#e06830',
      'cp-label-toggle': 1,
      'cp-label-size': 11,
      'cp-label-opacity': 0.8,
      'cp-label-show-id': 1,
      'cp-label-show-title': 1,
      'cp-label-show-status': 1,
      'cp-label-bg-opacity': 0.85,
      'cp-label-border': 1,
      'cp-force-strength': 60,
      'cp-link-distance': 60,
      'cp-center-force': 1,
      'cp-collision-radius': 0,
      'cp-alpha-decay': 0.023,
      'cp-agent-tether': 0.5,
      'cp-fly-speed': 1000,
      'cp-orbit-speed': 2.5,
      'cp-orbit-rate': 0.08,
      'cp-orbit-size': 1.5,
      'cp-hover-rate': 0.15,
      'cp-stream-rate': 0.12,
      'cp-stream-speed': 3.0,
      'cp-particle-lifetime': 0.8,
      'cp-selection-glow': 1.0,
      'cp-camera-fov': 75,
      'cp-camera-rotate-speed': 2.0,
      'cp-camera-zoom-speed': 1.0,
      'cp-camera-near': 0.1,
      'cp-camera-far': 50000,
      'cp-hud-stats': 1,
      'cp-hud-bottom': 1,
      'cp-hud-controls': 1,
      'cp-hud-left-sidebar': 1,
      'cp-hud-right-sidebar': 1,
      'cp-hud-minimap': 1,
      'cp-hud-tooltip': 1,
      'cp-edge-blocks': 1,
      'cp-edge-parent-child': 1,
      'cp-edge-waits-for': 1,
      'cp-edge-relates-to': 1,
      'cp-edge-child-of': 1,
      'cp-edge-action-item': 1,
      'cp-edge-escalate': 1,
      'cp-edge-duplicate': 1,
      'cp-edge-jira-link': 1,
      'cp-edge-assigned-to': 1,
      'cp-edge-rig-conflict': 0,
      'cp-edge-max-per-node': 0,
    },
    Neon: {
      'cp-bloom-threshold': 0.15,
      'cp-bloom-strength': 1.8,
      'cp-bloom-radius': 0.6,
      'cp-fresnel-opacity': 0.7,
      'cp-fresnel-power': 1.5,
      'cp-pulse-speed': 2.0,
      'cp-star-count': 3000,
      'cp-twinkle-speed': 2.0,
      'cp-bg-color': '#050510',
      'cp-color-open': '#00ff88',
      'cp-color-active': '#ffee00',
      'cp-color-blocked': '#ff2050',
      'cp-color-agent': '#ff8800',
      'cp-color-epic': '#cc44ff',
      'cp-color-jack': '#ff5522',
      'cp-label-toggle': 1,
      'cp-label-size': 12,
      'cp-label-opacity': 0.9,
      'cp-label-show-id': 1,
      'cp-label-show-title': 1,
      'cp-label-show-status': 1,
      'cp-label-bg-opacity': 0.85,
      'cp-label-border': 1,
      'cp-force-strength': 80,
      'cp-link-distance': 50,
      'cp-center-force': 1,
      'cp-collision-radius': 0,
      'cp-alpha-decay': 0.023,
      'cp-agent-tether': 0.5,
      'cp-fly-speed': 800,
      'cp-orbit-speed': 4.0,
      'cp-orbit-rate': 0.05,
      'cp-orbit-size': 2.0,
      'cp-hover-rate': 0.08,
      'cp-stream-rate': 0.06,
      'cp-stream-speed': 5.0,
      'cp-particle-lifetime': 1.2,
      'cp-selection-glow': 1.5,
      'cp-camera-fov': 60,
      'cp-camera-rotate-speed': 3.0,
      'cp-camera-zoom-speed': 1.5,
      'cp-camera-near': 0.1,
      'cp-camera-far': 50000,
      'cp-hud-stats': 1,
      'cp-hud-bottom': 1,
      'cp-hud-controls': 1,
      'cp-hud-left-sidebar': 1,
      'cp-hud-right-sidebar': 1,
      'cp-hud-minimap': 1,
      'cp-hud-tooltip': 1,
      'cp-edge-blocks': 1,
      'cp-edge-parent-child': 1,
      'cp-edge-waits-for': 1,
      'cp-edge-relates-to': 1,
      'cp-edge-child-of': 1,
      'cp-edge-action-item': 1,
      'cp-edge-escalate': 1,
      'cp-edge-duplicate': 1,
      'cp-edge-jira-link': 1,
      'cp-edge-assigned-to': 1,
      'cp-edge-rig-conflict': 0,
      'cp-edge-max-per-node': 0,
    },
    'High Contrast': {
      'cp-bloom-threshold': 0.8,
      'cp-bloom-strength': 0.3,
      'cp-bloom-radius': 0.2,
      'cp-fresnel-opacity': 0.2,
      'cp-fresnel-power': 3.0,
      'cp-pulse-speed': 4.0,
      'cp-star-count': 500,
      'cp-twinkle-speed': 0.5,
      'cp-bg-color': '#000000',
      'cp-color-open': '#00cc44',
      'cp-color-active': '#ffcc00',
      'cp-color-blocked': '#ff0000',
      'cp-color-agent': '#ff8844',
      'cp-color-epic': '#aa44cc',
      'cp-color-jack': '#dd5520',
      'cp-label-toggle': 1,
      'cp-label-size': 13,
      'cp-label-opacity': 1.0,
      'cp-label-show-id': 1,
      'cp-label-show-title': 1,
      'cp-label-show-status': 1,
      'cp-label-bg-opacity': 0.85,
      'cp-label-border': 1,
      'cp-force-strength': 60,
      'cp-link-distance': 60,
      'cp-center-force': 1,
      'cp-collision-radius': 0,
      'cp-alpha-decay': 0.023,
      'cp-agent-tether': 0.5,
      'cp-fly-speed': 1000,
      'cp-orbit-speed': 1.5,
      'cp-orbit-rate': 0.15,
      'cp-orbit-size': 1.0,
      'cp-hover-rate': 0.25,
      'cp-stream-rate': 0.2,
      'cp-stream-speed': 2.0,
      'cp-particle-lifetime': 0.5,
      'cp-selection-glow': 0.6,
      'cp-camera-fov': 75,
      'cp-camera-rotate-speed': 2.0,
      'cp-camera-zoom-speed': 1.0,
      'cp-camera-near': 0.1,
      'cp-camera-far': 50000,
      'cp-hud-stats': 1,
      'cp-hud-bottom': 1,
      'cp-hud-controls': 1,
      'cp-hud-left-sidebar': 1,
      'cp-hud-right-sidebar': 1,
      'cp-hud-minimap': 1,
      'cp-hud-tooltip': 1,
      'cp-edge-blocks': 1,
      'cp-edge-parent-child': 1,
      'cp-edge-waits-for': 1,
      'cp-edge-relates-to': 1,
      'cp-edge-child-of': 1,
      'cp-edge-action-item': 1,
      'cp-edge-escalate': 1,
      'cp-edge-duplicate': 1,
      'cp-edge-jira-link': 1,
      'cp-edge-assigned-to': 1,
      'cp-edge-rig-conflict': 0,
      'cp-edge-max-per-node': 0,
    },
  };

  function applyPreset(settings) {
    for (const [id, val] of Object.entries(settings)) {
      const el = document.getElementById(id);
      if (!el) continue;
      if (el.classList.contains('cp-toggle')) {
        // HUD visibility toggle: val=1 means on, val=0 means off
        const shouldBeOn = val === 1 || val === true;
        if (el.classList.contains('on') !== shouldBeOn) {
          el.click(); // triggers the toggle handler
        }
      } else {
        el.value = val;
        el.dispatchEvent(new Event('input')); // triggers all wired handlers
      }
    }
  }

  function getCurrentSettings() {
    const settings = {};
    const ids = Object.keys(BUILT_IN_PRESETS['Default Dark']);
    for (const id of ids) {
      const el = document.getElementById(id);
      if (!el) continue;
      if (el.classList.contains('cp-toggle')) {
        settings[id] = el.classList.contains('on') ? 1 : 0;
      } else {
        settings[id] = el.type === 'color' ? el.value : parseFloat(el.value);
      }
    }
    return settings;
  }

  // Render preset buttons
  const presetContainer = document.getElementById('cp-preset-buttons');
  if (presetContainer) {
    function renderPresetButtons() {
      presetContainer.innerHTML = '';
      // Built-in presets
      for (const name of Object.keys(BUILT_IN_PRESETS)) {
        const btn = document.createElement('button');
        btn.className = 'cp-preset-btn';
        btn.textContent = name;
        btn.onclick = () => applyPreset(BUILT_IN_PRESETS[name]);
        presetContainer.appendChild(btn);
      }
      // Custom presets from localStorage
      const custom = JSON.parse(localStorage.getItem('beads3d-custom-presets') || '{}');
      for (const name of Object.keys(custom)) {
        const btn = document.createElement('button');
        btn.className = 'cp-preset-btn';
        btn.textContent = name;
        btn.onclick = () => applyPreset(custom[name]);
        btn.oncontextmenu = (e) => {
          e.preventDefault();
          delete custom[name];
          localStorage.setItem('beads3d-custom-presets', JSON.stringify(custom));
          renderPresetButtons();
        };
        btn.title = 'Click to load, right-click to delete';
        presetContainer.appendChild(btn);
      }
      // Save button
      const saveBtn = document.createElement('button');
      saveBtn.className = 'cp-preset-btn';
      saveBtn.textContent = '+ save';
      saveBtn.style.color = '#39c5cf';
      saveBtn.onclick = () => {
        const name = prompt('Preset name:');
        if (!name) return;
        custom[name] = getCurrentSettings();
        localStorage.setItem('beads3d-custom-presets', JSON.stringify(custom));
        renderPresetButtons();
      };
      presetContainer.appendChild(saveBtn);
    }
    renderPresetButtons();
  }

  // --- Preset import/export (bd-n0g9q) ---
  const exportBtn = document.getElementById('cp-preset-export');
  if (exportBtn) {
    exportBtn.onclick = () => {
      const settings = getCurrentSettings();
      const blob = new Blob([JSON.stringify(settings, null, 2)], { type: 'application/json' });
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = 'beads3d-theme.json';
      a.click();
      URL.revokeObjectURL(a.href);
    };
  }
  const importBtn = document.getElementById('cp-preset-import');
  const fileInput = document.getElementById('cp-preset-file-input');
  if (importBtn && fileInput) {
    importBtn.onclick = () => fileInput.click();
    fileInput.onchange = () => {
      const file = fileInput.files?.[0];
      if (!file) return;
      const reader = new FileReader();
      reader.onload = () => {
        try {
          const settings = JSON.parse(reader.result);
          if (settings && typeof settings === 'object') applyPreset(settings);
        } catch {
          console.warn('[beads3d] failed to import preset');
        }
      };
      reader.readAsText(file);
      fileInput.value = '';
    };
  }
  const copyUrlBtn = document.getElementById('cp-preset-copy-url');
  if (copyUrlBtn) {
    copyUrlBtn.onclick = () => {
      const settings = getCurrentSettings();
      const encoded = btoa(JSON.stringify(settings));
      const url = `${location.origin}${location.pathname}#preset=${encoded}`;
      navigator.clipboard
        .writeText(url)
        .then(() => {
          copyUrlBtn.textContent = 'copied!';
          setTimeout(() => {
            copyUrlBtn.textContent = 'copy URL';
          }, 1500);
        })
        .catch(() => {
          prompt('Copy this URL:', url);
        });
    };
  }
  // Apply preset from URL fragment on load
  if (location.hash.startsWith('#preset=')) {
    try {
      const settings = JSON.parse(atob(location.hash.slice('#preset='.length)));
      if (settings && typeof settings === 'object') applyPreset(settings);
    } catch {
      /* ignore invalid fragment */
    }
  }
  // --- Config bead persistence (bd-ljy5v) ---
  // Load saved settings from daemon on startup, save changes back with debounce.
  const CONFIG_KEY = 'beads3d-control-panel-settings';
  let _persistDebounce = null;

  function persistSettings() {
    clearTimeout(_persistDebounce);
    _persistDebounce = setTimeout(() => {
      const settings = getCurrentSettings();
      _deps.api?.configSet(CONFIG_KEY, JSON.stringify(settings)).catch((err) => {
        console.warn('[beads3d] failed to persist settings:', err.message);
      });
    }, 1000);
  }

  // Wire persistence to all control panel inputs
  panel.querySelectorAll('.cp-slider, input[type="color"]').forEach((input) => {
    input.addEventListener('input', persistSettings);
  });

  // Load saved settings from config bead
  _deps.api
    ?.configGet(CONFIG_KEY)
    .then((resp) => {
      const val = resp?.value;
      if (!val) return;
      try {
        const settings = JSON.parse(val);
        if (settings && typeof settings === 'object') {
          applyPreset(settings);
          console.log('[beads3d] loaded control panel settings from config bead');
        }
      } catch {
        console.warn('[beads3d] failed to parse saved settings');
      }
    })
    .catch(() => {
      // Config bead not available — silently fall back to defaults
    });
}
