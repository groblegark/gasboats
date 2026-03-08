// --- Camera, Controls, Box Select, Screenshot/Export ---
// Extracted from main.js (bd-7t6nt)
// Contains: camera freeze/center, Quake-style smooth camera, setupControls,
// rubber-band box selection, bulk menu, screenshot & export.

import * as THREE from 'three';
import {
  setFilterDeps,
  toggleFilterDashboard,
  syncFilterDashboard,
  initFilterDashboard,
  updateAssigneeButtons,
  updateFilterCount,
} from './filter-dashboard.js';
import { setDetailDeps, showDetail, hideDetail } from './detail-panel.js';
import {
  setLeftSidebarDeps,
  toggleLeftSidebar,
  initLeftSidebar,
  getLeftSidebarOpen,
  setLeftSidebarOpen,
} from './left-sidebar.js';
import {
  setAgentWindowDeps,
  agentWindows,
  getAgentsViewOpen,
  getSelectedAgentTab,
  refreshAgentWindowBeads,
  showAgentWindow,
  closeAgentWindow,
  toggleAgentsView,
  openAgentsView,
  closeAgentsView,
  selectAgentTab,
  createAgentWindowInGrid,
  initUnifiedFeed,
} from './agent-windows.js';
import { _vfxConfig } from './vfx.js';
import { setControlPanelDeps, toggleControlPanel, initControlPanel, getControlPanelOpen } from './control-panel.js';
import { setOnNodeClick, initRightSidebar } from './right-sidebar.js';
import { setMutationDeps, dootPopups, dismissDootPopup } from './mutations.js';
import { showStatusToast, hideContextMenu, ctxMenu } from './context-menu.js';
import { setDecisionLightboxDeps, initDecisionLightbox } from './decision-lightbox.js';

// Dependency injection — set by main.js before setupControls()
let _deps = {};

/**
 * Inject dependencies from main.js.
 *
 * @param {Object} deps
 * @param {Object}   deps.api                  - BeadsAPI instance
 * @param {Function} deps.getGraph             - () => ForceGraph3D instance
 * @param {Function} deps.getGraphData         - () => { nodes, links }
 * @param {Function} deps.getBloomPass         - () => bloomPass
 * @param {Function} deps.setBloomEnabled      - (v) => void
 * @param {Function} deps.getBloomEnabled      - () => boolean
 * @param {Function} deps.refresh              - () => void
 * @param {Function} deps.applyFilters         - () => void
 * @param {Function} deps.setLayout            - (mode) => void
 * @param {Function} deps.toggleLabels         - () => void
 * @param {Function} deps.getLabelsVisible     - () => boolean
 * @param {Function} deps.toggleMinimap        - () => void
 * @param {Function} deps.handleNodeClick      - (node) => void
 * @param {Function} deps.clearSelection       - () => void
 * @param {Function} deps.clearEpicHighlight   - () => void
 * @param {Function} deps.hideTooltip          - () => void
 * @param {Function} deps.isTextInputFocused   - () => boolean
 * @param {Function} deps.flyToSearchResult    - () => void
 * @param {Function} deps.nextSearchResult     - () => void
 * @param {Function} deps.prevSearchResult     - () => void
 * @param {Function} deps.togglePerfOverlay    - () => void
 * @param {Function} deps.togglePerfGraph      - () => void
 * @param {Function} deps.setVfxIntensity      - (v) => void
 * @param {Function} deps.presetVFX            - (name) => void
 * @param {Function} deps.cycleEpic            - (delta) => void
 * @param {Function} deps.linkKey              - (link) => string
 * @param {Function} deps._syncAllRigPills     - () => void
 * @param {Object}   deps.state                - reactive state accessors (see below)
 */
export function setCameraDeps(deps) {
  _deps = deps;
}

// --- Camera constants (bd-zab4q) ---
// Quake-style smooth camera movement

/** @type {Set<string>} Currently held arrow/WASD keys */
export const _keysDown = new Set();

/** @type {{x: number, y: number, z: number}} World-space camera velocity */
export const _camVelocity = { x: 0, y: 0, z: 0 };

/** @type {number} Acceleration per frame while key held */
export const CAM_ACCEL = 1.2;

/** @type {number} Max strafe speed (units/frame) */
export const CAM_MAX_SPEED = 16;

/** @type {number} Velocity multiplier per frame when no key held (lower = more friction) */
export const CAM_FRICTION = 0.88;

// Camera freeze state (bd-casin)

/** @type {boolean} Whether orbit controls are frozen (after multi-select zoom) */
export let cameraFrozen = false;

// Box selection state

/** @type {boolean} Whether a rubber-band box selection is in progress */
export let isBoxSelecting = false;

/** @type {{x: number, y: number}|null} Screen coords of the box selection start point */
export let boxSelectStart = null;

// --- Camera freeze / center on multi-select (bd-casin) ---

/**
 * Fly camera to center on the selected nodes and their immediate connections,
 * then freeze orbit controls. Call unfreezeCamera() (Escape) to restore.
 * @returns {void}
 */
export function centerCameraOnSelection() {
  const multiSelected = _deps.state.multiSelected;
  const graphData = _deps.getGraphData();
  const graph = _deps.getGraph();
  if (multiSelected.size === 0) return;

  // Collect selected node IDs + their direct neighbors
  const relevantIds = new Set(multiSelected);
  for (const l of graphData.links) {
    const srcId = typeof l.source === 'object' ? l.source.id : l.source;
    const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
    if (multiSelected.has(srcId) || multiSelected.has(tgtId)) {
      relevantIds.add(srcId);
      relevantIds.add(tgtId);
    }
  }

  // Calculate bounding-box center of relevant nodes
  let cx = 0,
    cy = 0,
    cz = 0,
    count = 0;
  for (const node of graphData.nodes) {
    if (!relevantIds.has(node.id)) continue;
    cx += node.x || 0;
    cy += node.y || 0;
    cz += node.z || 0;
    count++;
  }
  if (count === 0) return;
  cx /= count;
  cy /= count;
  cz /= count;

  // Calculate radius (max distance from center) to set camera distance
  let maxDist = 0;
  for (const node of graphData.nodes) {
    if (!relevantIds.has(node.id)) continue;
    const dx = (node.x || 0) - cx;
    const dy = (node.y || 0) - cy;
    const dz = (node.z || 0) - cz;
    const d = Math.sqrt(dx * dx + dy * dy + dz * dz);
    if (d > maxDist) maxDist = d;
  }

  // Camera distance: enough to see the whole cluster with some padding
  const distance = Math.max(maxDist * 2.5, 120);
  const lookAt = { x: cx, y: cy, z: cz };
  // Position camera along the current camera direction, but at the right distance
  const cam = graph.camera();
  const dir = new THREE.Vector3(cam.position.x - cx, cam.position.y - cy, cam.position.z - cz).normalize();
  const camPos = {
    x: cx + dir.x * distance,
    y: cy + dir.y * distance,
    z: cz + dir.z * distance,
  };

  graph.cameraPosition(camPos, lookAt, 1000);

  // Freeze orbit controls + pin all node positions after the fly animation
  cameraFrozen = true;
  setTimeout(() => {
    const controls = graph.controls();
    if (controls) controls.enabled = false;
    // Pin every node so forces don't drift them (bd-casin)
    for (const node of graphData.nodes) {
      node.fx = node.x;
      node.fy = node.y;
      node.fz = node.z;
    }
  }, 1050);
}

/**
 * Unfreeze camera orbit controls and unpin all nodes so forces resume.
 * @returns {void}
 */
export function unfreezeCamera() {
  if (!cameraFrozen) return;
  cameraFrozen = false;
  const graph = _deps.getGraph();
  const graphData = _deps.getGraphData();
  const controls = graph.controls();
  if (controls) controls.enabled = true;
  // Unpin all nodes so forces resume (bd-casin)
  for (const node of graphData.nodes) {
    node.fx = undefined;
    node.fy = undefined;
    node.fz = undefined;
  }
}

/**
 * Update Quake-style smooth camera movement based on currently held keys.
 * Called from the animation loop in main.js each frame.
 * @returns {void}
 */
export function updateCameraMovement() {
  const graph = _deps.getGraph?.();
  if (!graph) return;

  if (
    _keysDown.size > 0 ||
    Math.abs(_camVelocity.x) > 0.01 ||
    Math.abs(_camVelocity.y) > 0.01 ||
    Math.abs(_camVelocity.z) > 0.01
  ) {
    const camera = graph.camera();
    const controls = graph.controls();
    // Build desired direction from held keys
    const right = new THREE.Vector3();
    camera.getWorldDirection(new THREE.Vector3());
    right.setFromMatrixColumn(camera.matrixWorld, 0).normalize();

    // Forward vector for W/S: camera look direction projected onto XZ plane (bd-pwaen)
    const forward = new THREE.Vector3();
    camera.getWorldDirection(forward);
    forward.y = 0; // project to XZ plane — no diving into ground
    forward.normalize();

    // Accelerate in held directions
    // Strafe: ArrowLeft/A = left, ArrowRight/D = right
    if (_keysDown.has('ArrowLeft') || _keysDown.has('a')) {
      _camVelocity.x -= right.x * CAM_ACCEL;
      _camVelocity.y -= right.y * CAM_ACCEL;
      _camVelocity.z -= right.z * CAM_ACCEL;
    }
    if (_keysDown.has('ArrowRight') || _keysDown.has('d')) {
      _camVelocity.x += right.x * CAM_ACCEL;
      _camVelocity.y += right.y * CAM_ACCEL;
      _camVelocity.z += right.z * CAM_ACCEL;
    }
    // Vertical: ArrowUp/Down (unchanged — moves camera up/down in Y)
    if (_keysDown.has('ArrowUp')) {
      _camVelocity.y += CAM_ACCEL;
    }
    if (_keysDown.has('ArrowDown')) {
      _camVelocity.y -= CAM_ACCEL;
    }
    // Forward/back: W/S move along camera look direction in XZ plane (bd-pwaen)
    if (_keysDown.has('w')) {
      _camVelocity.x += forward.x * CAM_ACCEL;
      _camVelocity.z += forward.z * CAM_ACCEL;
    }
    if (_keysDown.has('s')) {
      _camVelocity.x -= forward.x * CAM_ACCEL;
      _camVelocity.z -= forward.z * CAM_ACCEL;
    }

    // Clamp speed
    const speed = Math.sqrt(_camVelocity.x ** 2 + _camVelocity.y ** 2 + _camVelocity.z ** 2);
    if (speed > CAM_MAX_SPEED) {
      const s = CAM_MAX_SPEED / speed;
      _camVelocity.x *= s;
      _camVelocity.y *= s;
      _camVelocity.z *= s;
    }

    // Apply velocity to camera + orbit target
    const delta = new THREE.Vector3(_camVelocity.x, _camVelocity.y, _camVelocity.z);
    camera.position.add(delta);
    if (controls && controls.target) controls.target.add(delta);

    // Friction: decelerate when no keys held (or gentle drag when keys held)
    const friction = _keysDown.size > 0 ? 0.95 : CAM_FRICTION;
    _camVelocity.x *= friction;
    _camVelocity.y *= friction;
    _camVelocity.z *= friction;

    // Full stop below threshold
    if (speed < 0.01) {
      _camVelocity.x = 0;
      _camVelocity.y = 0;
      _camVelocity.z = 0;
    }
  }
}

/**
 * Capture a screenshot of the current 3D graph view and download it as a PNG.
 * @returns {void}
 */
export function captureScreenshot() {
  const graph = _deps.getGraph();
  const renderer = graph.renderer();
  const canvas = renderer.domElement;

  // Force a render to ensure the buffer is fresh
  renderer.render(graph.scene(), graph.camera());

  const dataUrl = canvas.toDataURL('image/png');
  const link = document.createElement('a');
  link.download = `beads3d-${new Date().toISOString().slice(0, 19).replace(/:/g, '')}.png`;
  link.href = dataUrl;
  link.click();

  const statusEl = document.getElementById('status');
  statusEl.textContent = 'screenshot saved';
  statusEl.className = 'connected';
}

/**
 * Export the visible graph data (nodes, links, filters) as a downloadable JSON file.
 * @returns {void}
 */
export function exportGraphJSON() {
  const graphData = _deps.getGraphData();
  const state = _deps.state;
  const visibleNodes = graphData.nodes.filter((n) => !n._hidden);
  const visibleIds = new Set(visibleNodes.map((n) => n.id));
  const visibleLinks = graphData.links.filter((l) => {
    const srcId = typeof l.source === 'object' ? l.source.id : l.source;
    const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
    return visibleIds.has(srcId) && visibleIds.has(tgtId);
  });

  const exportData = {
    exported_at: new Date().toISOString(),
    filters: {
      search: state.searchFilter || null,
      status: state.statusFilter.size > 0 ? [...state.statusFilter] : null,
      type: state.typeFilter.size > 0 ? [...state.typeFilter] : null,
      agents: {
        show: state.agentFilterShow,
        orphaned: state.agentFilterOrphaned,
        rig_exclude: state.agentFilterRigExclude.size > 0 ? [...state.agentFilterRigExclude] : null,
      },
    },
    stats: {
      total_nodes: graphData.nodes.length,
      visible_nodes: visibleNodes.length,
      visible_links: visibleLinks.length,
    },
    nodes: visibleNodes.map((n) => ({
      id: n.id,
      title: n.title,
      status: n.status,
      priority: n.priority,
      issue_type: n.issue_type,
      assignee: n.assignee || null,
      blocked: !!n._blocked,
      x: n.x ? Math.round(n.x * 10) / 10 : null,
      y: n.y ? Math.round(n.y * 10) / 10 : null,
      z: n.z ? Math.round(n.z * 10) / 10 : null,
    })),
    links: visibleLinks.map((l) => ({
      source: typeof l.source === 'object' ? l.source.id : l.source,
      target: typeof l.target === 'object' ? l.target.id : l.target,
      dep_type: l.dep_type,
    })),
  };

  const json = JSON.stringify(exportData, null, 2);
  const blob = new Blob([json], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.download = `beads3d-${new Date().toISOString().slice(0, 19).replace(/:/g, '')}.json`;
  link.href = url;
  link.click();
  URL.revokeObjectURL(url);

  const statusEl = document.getElementById('status');
  statusEl.textContent = `exported ${visibleNodes.length} nodes, ${visibleLinks.length} links`;
  statusEl.className = 'connected';
}

// --- Rubber-band selection (shift+drag) ---
const selectOverlay = document.getElementById('select-overlay');
const selectCtx = selectOverlay.getContext('2d');
const bulkMenu = document.getElementById('bulk-menu');

function resizeSelectOverlay() {
  selectOverlay.width = window.innerWidth;
  selectOverlay.height = window.innerHeight;
}
window.addEventListener('resize', resizeSelectOverlay);
resizeSelectOverlay();

// Project a 3D node position to 2D screen coordinates
function nodeToScreen(node) {
  const graph = _deps.getGraph();
  const camera = graph.camera();
  const renderer = graph.renderer();
  const { width, height } = renderer.domElement.getBoundingClientRect();
  const vec = new THREE.Vector3(node.x || 0, node.y || 0, node.z || 0);
  vec.project(camera);
  return {
    x: (vec.x * 0.5 + 0.5) * width,
    y: (-vec.y * 0.5 + 0.5) * height,
  };
}

/**
 * Set up shift+drag rubber-band box selection on the graph element.
 * @returns {void}
 */
export function setupBoxSelect() {
  const graph = _deps.getGraph();
  const graphData = _deps.getGraphData;
  const graphEl = document.getElementById('graph');

  graphEl.addEventListener('mousedown', (e) => {
    if (!e.shiftKey || e.button !== 0) return;
    e.preventDefault();
    e.stopPropagation();

    isBoxSelecting = true;
    boxSelectStart = { x: e.clientX, y: e.clientY };
    selectOverlay.style.display = 'block';
    selectOverlay.style.pointerEvents = 'auto';

    // Disable orbit controls during box select
    const controls = _deps.getGraph().controls();
    if (controls) controls.enabled = false;
  });

  // Use document-level listeners so drag works even if mouse leaves graph
  document.addEventListener('mousemove', (e) => {
    if (!isBoxSelecting) return;
    e.preventDefault();

    const x0 = Math.min(boxSelectStart.x, e.clientX);
    const y0 = Math.min(boxSelectStart.y, e.clientY);
    const w = Math.abs(e.clientX - boxSelectStart.x);
    const h = Math.abs(e.clientY - boxSelectStart.y);

    selectCtx.clearRect(0, 0, selectOverlay.width, selectOverlay.height);

    // Draw selection rectangle
    selectCtx.fillStyle = 'rgba(74, 158, 255, 0.08)';
    selectCtx.fillRect(x0, y0, w, h);
    selectCtx.strokeStyle = 'rgba(74, 158, 255, 0.6)';
    selectCtx.lineWidth = 1;
    selectCtx.setLineDash([4, 4]);
    selectCtx.strokeRect(x0, y0, w, h);
    selectCtx.setLineDash([]);

    // Live preview: highlight nodes inside the rectangle
    const previewSet = new Set();
    const gd = _deps.getGraphData();
    for (const node of gd.nodes) {
      if (node._hidden || node.issue_type === 'agent') continue;
      const screen = nodeToScreen(node);
      if (screen.x >= x0 && screen.x <= x0 + w && screen.y >= y0 && screen.y <= y0 + h) {
        previewSet.add(node.id);
        // Draw a small indicator dot on the overlay
        selectCtx.beginPath();
        selectCtx.arc(screen.x, screen.y, 4, 0, Math.PI * 2);
        selectCtx.fillStyle = 'rgba(74, 158, 255, 0.5)';
        selectCtx.fill();
      }
    }
    _deps.state.multiSelected = previewSet;
  });

  document.addEventListener('mouseup', (e) => {
    if (!isBoxSelecting) return;
    isBoxSelecting = false;
    selectOverlay.style.display = 'none';
    selectOverlay.style.pointerEvents = 'none';
    selectCtx.clearRect(0, 0, selectOverlay.width, selectOverlay.height);

    // Re-enable orbit controls (will be re-frozen by centerCameraOnSelection if multi-select)
    const controls = _deps.getGraph().controls();
    if (controls) controls.enabled = true;

    const multiSelected = _deps.state.multiSelected;
    const highlightNodes = _deps.state.highlightNodes;
    const highlightLinks = _deps.state.highlightLinks;

    // Finalize selection
    if (multiSelected.size > 0) {
      // Highlight connected nodes/links for the selection
      highlightNodes.clear();
      highlightLinks.clear();
      for (const id of multiSelected) highlightNodes.add(id);
      const gd = _deps.getGraphData();
      for (const l of gd.links) {
        const srcId = typeof l.source === 'object' ? l.source.id : l.source;
        const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
        if (multiSelected.has(srcId) || multiSelected.has(tgtId)) {
          highlightNodes.add(srcId);
          highlightNodes.add(tgtId);
          highlightLinks.add(_deps.linkKey(l));
        }
      }
      _deps.getGraph().linkWidth(_deps.getGraph().linkWidth());

      // Center camera on selection and freeze controls (bd-casin)
      centerCameraOnSelection();

      showBulkMenu(e.clientX, e.clientY);
    }
  });
}

function buildBulkStatusSubmenu() {
  const statuses = [
    { value: 'open', label: 'open', color: '#2d8a4e' },
    { value: 'in_progress', label: 'in progress', color: '#d4a017' },
    { value: 'closed', label: 'closed', color: '#333340' },
  ];
  return statuses
    .map(
      (s) =>
        `<div class="bulk-item" data-action="bulk-status" data-value="${s.value}">` +
        `<span class="ctx-dot" style="background:${s.color}"></span>${s.label}</div>`,
    )
    .join('');
}

function buildBulkPrioritySubmenu() {
  const priorities = [
    { value: 0, label: 'P0 critical', color: '#ff3333' },
    { value: 1, label: 'P1 high', color: '#ff8833' },
    { value: 2, label: 'P2 medium', color: '#d4a017' },
    { value: 3, label: 'P3 low', color: '#4a9eff' },
    { value: 4, label: 'P4 backlog', color: '#666' },
  ];
  return priorities
    .map(
      (p) =>
        `<div class="bulk-item" data-action="bulk-priority" data-value="${p.value}">` +
        `<span class="ctx-dot" style="background:${p.color}"></span>${p.label}</div>`,
    )
    .join('');
}

/**
 * Display the bulk action menu at the given screen coordinates.
 * @param {number} x - Screen X position for the menu
 * @param {number} y - Screen Y position for the menu
 * @returns {void}
 */
export function showBulkMenu(x, y) {
  const multiSelected = _deps.state.multiSelected;
  const count = multiSelected.size;
  bulkMenu.innerHTML = `
    <div class="bulk-header">${count} bead${count !== 1 ? 's' : ''} selected</div>
    <div class="bulk-item bulk-submenu">set status
      <div class="bulk-submenu-panel">${buildBulkStatusSubmenu()}</div>
    </div>
    <div class="bulk-item bulk-submenu">set priority
      <div class="bulk-submenu-panel">${buildBulkPrioritySubmenu()}</div>
    </div>
    <div class="bulk-sep"></div>
    <div class="bulk-item" data-action="bulk-close">close all</div>
    <div class="bulk-sep"></div>
    <div class="bulk-item" data-action="bulk-clear">clear selection</div>
  `;

  bulkMenu.style.display = 'block';
  const rect = bulkMenu.getBoundingClientRect();
  if (x + rect.width > window.innerWidth) x = window.innerWidth - rect.width - 8;
  if (y + rect.height > window.innerHeight) y = window.innerHeight - rect.height - 8;
  bulkMenu.style.left = x + 'px';
  bulkMenu.style.top = y + 'px';

  bulkMenu.onclick = (e) => {
    const item = e.target.closest('.bulk-item');
    if (!item) return;
    const action = item.dataset.action;
    const value = item.dataset.value;
    handleBulkAction(action, value);
  };
}

/**
 * Hide the bulk action menu and remove its click handler.
 * @returns {void}
 */
export function hideBulkMenu() {
  bulkMenu.style.display = 'none';
  bulkMenu.onclick = null;
}

async function handleBulkAction(action, value) {
  const multiSelected = _deps.state.multiSelected;
  const graphData = _deps.getGraphData();
  const graph = _deps.getGraph();
  const api = _deps.api;
  const ids = [...multiSelected];
  hideBulkMenu();

  // Build snapshot for rollback and apply optimistic changes
  const nodeMap = new Map(graphData.nodes.map((n) => [n.id, n]));
  const snapshots = new Map();

  switch (action) {
    case 'bulk-status': {
      for (const id of ids) {
        const n = nodeMap.get(id);
        if (n) {
          snapshots.set(id, { status: n.status });
          n.status = value;
        }
      }
      graph.nodeThreeObject(graph.nodeThreeObject());
      showStatusToast(`${ids.length} → ${value}`);
      const results = await Promise.allSettled(ids.map((id) => api.update(id, { status: value })));
      const failed = results.filter((r) => r.status === 'rejected').length;
      if (failed > 0) {
        showStatusToast(`${failed}/${ids.length} failed`, true);
        for (const [id, snap] of snapshots) {
          const n = nodeMap.get(id);
          if (n) Object.assign(n, snap);
        }
        graph.nodeThreeObject(graph.nodeThreeObject());
      }
      break;
    }
    case 'bulk-priority': {
      const p = parseInt(value, 10);
      for (const id of ids) {
        const n = nodeMap.get(id);
        if (n) {
          snapshots.set(id, { priority: n.priority });
          n.priority = p;
        }
      }
      graph.nodeThreeObject(graph.nodeThreeObject());
      showStatusToast(`${ids.length} → P${p}`);
      const results = await Promise.allSettled(ids.map((id) => api.update(id, { priority: p })));
      const failed = results.filter((r) => r.status === 'rejected').length;
      if (failed > 0) {
        showStatusToast(`${failed}/${ids.length} failed`, true);
        for (const [id, snap] of snapshots) {
          const n = nodeMap.get(id);
          if (n) Object.assign(n, snap);
        }
        graph.nodeThreeObject(graph.nodeThreeObject());
      }
      break;
    }
    case 'bulk-close': {
      for (const id of ids) {
        const n = nodeMap.get(id);
        if (n) {
          snapshots.set(id, { status: n.status });
          n.status = 'closed';
        }
      }
      graph.nodeThreeObject(graph.nodeThreeObject());
      showStatusToast(`closed ${ids.length}`);
      const results = await Promise.allSettled(ids.map((id) => api.close(id)));
      const failed = results.filter((r) => r.status === 'rejected').length;
      if (failed > 0) {
        showStatusToast(`${failed}/${ids.length} failed`, true);
        for (const [id, snap] of snapshots) {
          const n = nodeMap.get(id);
          if (n) Object.assign(n, snap);
        }
        graph.nodeThreeObject(graph.nodeThreeObject());
      }
      break;
    }
    case 'bulk-clear':
      break;
  }

  multiSelected.clear();
  unfreezeCamera(); // bd-casin: restore orbit controls after bulk action
}

/**
 * Wire up all UI controls: toolbar buttons, keyboard shortcuts, search, filters,
 * and sub-module dependency injection (filter dashboard, detail panel, etc.).
 * @returns {void}
 */
export function setupControls() {
  const graph = _deps.getGraph();
  const api = _deps.api;
  const state = _deps.state;
  const btnRefresh = document.getElementById('btn-refresh');
  const searchInput = document.getElementById('search-input');

  const btnBloom = document.getElementById('btn-bloom');

  // Layout buttons
  document.getElementById('btn-layout-free').onclick = () => _deps.setLayout('free');
  document.getElementById('btn-layout-dag').onclick = () => _deps.setLayout('dag');
  // Timeline layout removed (bd-t9unh)
  document.getElementById('btn-layout-radial').onclick = () => _deps.setLayout('radial');
  document.getElementById('btn-layout-cluster').onclick = () => _deps.setLayout('cluster');

  btnRefresh.onclick = () => _deps.refresh();

  // Screenshot & export buttons
  document.getElementById('btn-screenshot').onclick = () => captureScreenshot();
  document.getElementById('btn-export').onclick = () => exportGraphJSON();

  // Bloom toggle
  btnBloom.onclick = () => {
    _deps.setBloomEnabled(!_deps.getBloomEnabled());
    const bloomPass = _deps.getBloomPass();
    if (bloomPass) bloomPass.enabled = _deps.getBloomEnabled();
    btnBloom.classList.toggle('active', _deps.getBloomEnabled());
  };

  // Labels toggle (bd-1o2f7, bd-oypa2: start active since labels default on)
  const btnLabelsEl = document.getElementById('btn-labels');
  btnLabelsEl.onclick = () => _deps.toggleLabels();
  if (_deps.getLabelsVisible()) btnLabelsEl.classList.add('active');

  // Bottom HUD bar quick-action buttons (bd-ddj44, bd-9ndk0.1)
  const hudBtnRefresh = document.getElementById('hud-btn-refresh');
  const hudBtnLabels = document.getElementById('hud-btn-labels');
  const hudBtnAgents = document.getElementById('hud-btn-agents');
  const hudBtnBloom = document.getElementById('hud-btn-bloom');
  const hudBtnSearch = document.getElementById('hud-btn-search');
  const hudBtnMinimap = document.getElementById('hud-btn-minimap');
  const hudBtnSidebar = document.getElementById('hud-btn-sidebar');
  const hudBtnControls = document.getElementById('hud-btn-controls');
  if (hudBtnRefresh) hudBtnRefresh.onclick = () => _deps.refresh();
  if (hudBtnLabels) hudBtnLabels.onclick = () => _deps.toggleLabels();
  if (hudBtnAgents) hudBtnAgents.onclick = () => toggleAgentsView();
  if (hudBtnBloom)
    hudBtnBloom.onclick = () => {
      _deps.setBloomEnabled(!_deps.getBloomEnabled());
      const bloomPass = _deps.getBloomPass();
      if (bloomPass) bloomPass.enabled = _deps.getBloomEnabled();
      hudBtnBloom.classList.toggle('active', _deps.getBloomEnabled());
    };
  if (hudBtnSearch) hudBtnSearch.onclick = () => searchInput.focus();
  if (hudBtnMinimap) hudBtnMinimap.onclick = () => _deps.toggleMinimap();
  if (hudBtnSidebar) hudBtnSidebar.onclick = () => toggleLeftSidebar();
  if (hudBtnControls) hudBtnControls.onclick = () => toggleControlPanel();

  // bd-69y6v: Control panel — wire dependencies and init
  setControlPanelDeps({
    getGraph: _deps.getGraph,
    getGraphData: _deps.getGraphData,
    getBloomPass: _deps.getBloomPass,
    setMaxEdgesPerNode: (v) => {
      state.maxEdgesPerNode = v;
    },
    setAgentTetherStrength: (v) => {
      state._agentTetherStrength = v;
    },
    setMinimapVisible: (v) => {
      state.minimapVisible = v;
    },
    getDepTypeHidden: () => state.depTypeHidden,
    applyFilters: _deps.applyFilters,
    refresh: _deps.refresh,
    toggleLabels: _deps.toggleLabels,
    getLabelsVisible: _deps.getLabelsVisible,
    setLayout: _deps.setLayout,
    api,
  });
  initControlPanel();

  // bd-inqge: Right sidebar
  setOnNodeClick(_deps.handleNodeClick); // register callback for right-sidebar.js (bd-7t6nt)
  initRightSidebar();

  // bd-9ndk0.3: Unified activity stream
  initUnifiedFeed();

  // Search — debounced input updates filter, Enter/arrows navigate results (bd-7n4g8)
  searchInput.addEventListener('input', (e) => {
    state.searchFilter = e.target.value;
    state.searchResultIdx = 0; // reset to first result on new input
    clearTimeout(state._searchDebounceTimer);
    state._searchDebounceTimer = setTimeout(() => _deps.applyFilters(), 150);
  });
  searchInput.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      if (state.searchResults.length > 0) {
        _deps.flyToSearchResult();
      }
    } else if (e.key === 'ArrowDown') {
      e.preventDefault();
      _deps.nextSearchResult();
    } else if (e.key === 'ArrowUp') {
      e.preventDefault();
      _deps.prevSearchResult();
    }
  });

  // Status filter toggles
  // "active" button covers in_progress + blocked + hooked + deferred (bd-7haep)
  const STATUS_GROUPS = {
    in_progress: ['in_progress', 'blocked', 'hooked', 'deferred'],
  };
  document.querySelectorAll('.filter-status').forEach((btn) => {
    btn.addEventListener('click', () => {
      const status = btn.dataset.status;
      const group = STATUS_GROUPS[status] || [status];
      btn.classList.toggle('active');
      btn.setAttribute('aria-pressed', btn.classList.contains('active'));
      if (state.statusFilter.has(status)) {
        group.forEach((s) => state.statusFilter.delete(s));
      } else {
        group.forEach((s) => state.statusFilter.add(s));
      }
      syncFilterDashboard();
      _deps.applyFilters();
    });
  });

  // Type filter toggles
  document.querySelectorAll('.filter-type').forEach((btn) => {
    btn.addEventListener('click', () => {
      const type = btn.dataset.type;
      btn.classList.toggle('active');
      btn.setAttribute('aria-pressed', btn.classList.contains('active'));
      if (state.typeFilter.has(type)) {
        state.typeFilter.delete(type);
      } else {
        state.typeFilter.add(type);
      }
      syncFilterDashboard();
      _deps.applyFilters();
    });
  });

  // Agent filter controls (bd-8o2gd)
  const btnAgentShow = document.getElementById('btn-agent-show');
  const btnAgentOrphaned = document.getElementById('btn-agent-orphaned');

  if (btnAgentShow) {
    btnAgentShow.addEventListener('click', () => {
      state.agentFilterShow = !state.agentFilterShow;
      btnAgentShow.classList.toggle('active', state.agentFilterShow);
      syncFilterDashboard();
      _deps.applyFilters();
    });
  }

  if (btnAgentOrphaned) {
    btnAgentOrphaned.addEventListener('click', () => {
      state.agentFilterOrphaned = !state.agentFilterOrphaned;
      btnAgentOrphaned.classList.toggle('active', state.agentFilterOrphaned);
      syncFilterDashboard();
      _deps.applyFilters();
    });
  }

  // Age filter (bd-uc0mw): radio-style — only one active at a time.
  // Triggers a full re-fetch because the server uses max_age_days to limit
  // which closed issues are returned (avoids pulling thousands of stale beads).
  document.querySelectorAll('.filter-age').forEach((btn) => {
    btn.addEventListener('click', () => {
      const newDays = parseInt(btn.dataset.days, 10);
      if (newDays === state.activeAgeDays) return; // no change
      document.querySelectorAll('.filter-age').forEach((b) => {
        b.classList.remove('active');
        b.setAttribute('aria-pressed', 'false');
      });
      btn.classList.add('active');
      btn.setAttribute('aria-pressed', 'true');
      state.activeAgeDays = newDays;
      syncFilterDashboard();
      _deps.refresh(); // re-fetch with new age cutoff (bd-uc0mw)
    });
  });

  // Filter dashboard panel (bd-8o2gd phase 2) — wire dependencies
  setFilterDeps({
    applyFilters: _deps.applyFilters,
    refresh: _deps.refresh,
    syncAllRigPills: _deps._syncAllRigPills,
    api,
    state: {
      get filterDashboardOpen() {
        return state.filterDashboardOpen;
      },
      set filterDashboardOpen(v) {
        state.filterDashboardOpen = v;
      },
      get statusFilter() {
        return state.statusFilter;
      },
      get typeFilter() {
        return state.typeFilter;
      },
      get priorityFilter() {
        return state.priorityFilter;
      },
      get activeAgeDays() {
        return state.activeAgeDays;
      },
      set activeAgeDays(v) {
        state.activeAgeDays = v;
      },
      get assigneeFilter() {
        return state.assigneeFilter;
      },
      set assigneeFilter(v) {
        state.assigneeFilter = v;
      },
      get agentFilterShow() {
        return state.agentFilterShow;
      },
      set agentFilterShow(v) {
        state.agentFilterShow = v;
      },
      get agentFilterOrphaned() {
        return state.agentFilterOrphaned;
      },
      set agentFilterOrphaned(v) {
        state.agentFilterOrphaned = v;
      },
      get agentFilterRigExclude() {
        return state.agentFilterRigExclude;
      },
      get agentFilterNameExclude() {
        return state.agentFilterNameExclude;
      },
      set agentFilterNameExclude(v) {
        state.agentFilterNameExclude = v;
      },
      get graphData() {
        return _deps.getGraphData();
      },
      get searchResults() {
        return state.searchResults;
      },
      get searchResultIdx() {
        return state.searchResultIdx;
      },
      URL_PROFILE: state.URL_PROFILE,
      URL_STATUS: state.URL_STATUS,
      URL_TYPES: state.URL_TYPES,
      URL_ASSIGNEE: state.URL_ASSIGNEE,
    },
  });
  initFilterDashboard();

  // Detail panel (bd-fbmq3, bd-7t6nt) — wire dependencies
  setDetailDeps({
    api,
    showAgentWindow,
    openAgentTab: (node) => {
      // bd-bwi52: open agents view + select tab for this agent
      if (getAgentsViewOpen()) {
        selectAgentTab(node.id);
      } else {
        openAgentsView();
        selectAgentTab(node.id);
      }
    },
    showStatusToast,
    getGraph: _deps.getGraph,
  });

  // Decision lightbox (beads-zuc3) — wire dependencies
  setDecisionLightboxDeps({
    api,
    showStatusToast,
    getGraph: _deps.getGraph,
  });
  initDecisionLightbox();

  // Agent windows (bd-7t6nt) — wire dependencies
  setAgentWindowDeps({
    api,
    getGraphData: _deps.getGraphData,
    handleNodeClick: _deps.handleNodeClick,
  });

  // Mutations/doots (bd-7t6nt) — wire dependencies
  setMutationDeps({
    api,
    getGraphData: _deps.getGraphData,
    getGraph: _deps.getGraph,
    refresh: _deps.refresh,
    handleNodeClick: _deps.handleNodeClick,
  });

  // Keyboard shortcuts
  document.addEventListener('keydown', (e) => {
    // '/' to focus search
    if (e.key === '/' && !_deps.isTextInputFocused()) {
      e.preventDefault();
      searchInput.focus();
    }
    // Escape to clear search, close detail, close context/bulk menu, and deselect
    if (e.key === 'Escape') {
      // Always unfreeze camera on Escape (bd-casin)
      unfreezeCamera();

      // Close control panel if open (bd-69y6v)
      if (getControlPanelOpen()) {
        toggleControlPanel();
        return;
      }

      // Close left sidebar if open (bd-nnr22)
      if (getLeftSidebarOpen()) {
        toggleLeftSidebar();
        return;
      }

      // Close filter dashboard if open (bd-8o2gd phase 2)
      if (state.filterDashboardOpen) {
        toggleFilterDashboard();
        return;
      }

      // Close Agents View if open (bd-jgvas)
      if (getAgentsViewOpen()) {
        // If search is focused and has text, clear it first
        const avSearch = document.querySelector('.agents-view-search');
        if (avSearch && document.activeElement === avSearch && avSearch.value) {
          avSearch.value = '';
          avSearch.dispatchEvent(new Event('input'));
          return;
        }
        closeAgentsView();
        return;
      }

      if (bulkMenu.style.display === 'block') {
        hideBulkMenu();
        _deps.state.multiSelected.clear();
        return;
      }
      if (ctxMenu.style.display === 'block') {
        hideContextMenu();
        return;
      }
      if (document.activeElement === searchInput) {
        searchInput.value = '';
        state.searchFilter = '';
        searchInput.blur();
        _deps.applyFilters();
      }
      _deps.clearSelection();
      _deps.clearEpicHighlight();
      hideDetail();
      _deps.hideTooltip();
      // Dismiss all doot popups (beads-799l)
      for (const [id] of dootPopups) dismissDootPopup(id);
      // Close all agent windows (bd-kau4k)
      for (const [id] of agentWindows) closeAgentWindow(id);
    }
    // 'r' to refresh
    if (e.key === 'r' && !_deps.isTextInputFocused()) {
      _deps.refresh();
    }
    // Performance overlay: backtick toggles, ~ toggles frame graph (bd-8nx79)
    if (e.key === '`' && !e.shiftKey && !_deps.isTextInputFocused()) {
      _deps.togglePerfOverlay();
      return;
    }
    if (e.key === '~' && !_deps.isTextInputFocused()) {
      _deps.togglePerfGraph();
      return;
    }
    // VFX intensity: [ ] to adjust, Shift+1-4 for presets (bd-epyyu)
    if (e.key === '[' && !_deps.isTextInputFocused()) {
      _deps.setVfxIntensity(_vfxConfig.intensity - 0.25);
      return;
    }
    if (e.key === ']' && !_deps.isTextInputFocused()) {
      _deps.setVfxIntensity(_vfxConfig.intensity + 0.25);
      return;
    }
    if (e.shiftKey && e.key === '!' && !_deps.isTextInputFocused()) {
      _deps.presetVFX('subtle');
      return;
    }
    if (e.shiftKey && e.key === '@' && !_deps.isTextInputFocused()) {
      _deps.presetVFX('normal');
      return;
    }
    if (e.shiftKey && e.key === '#' && !_deps.isTextInputFocused()) {
      _deps.presetVFX('dramatic');
      return;
    }
    if (e.shiftKey && e.key === '$' && !_deps.isTextInputFocused()) {
      _deps.presetVFX('maximum');
      return;
    }
    // 'b' to toggle bloom (ignore key repeat to prevent rapid on/off — beads-p97b)
    if (e.key === 'b' && !e.repeat && !_deps.isTextInputFocused()) {
      btnBloom.click();
    }
    // 'm' to toggle minimap
    if (e.key === 'm' && !e.repeat && !_deps.isTextInputFocused()) {
      _deps.toggleMinimap();
    }
    // 'l' for labels toggle (bd-1o2f7, beads-p97b: ignore key repeat)
    if (e.key === 'l' && !e.repeat && !_deps.isTextInputFocused()) {
      _deps.toggleLabels();
    }
    // 'f' for left sidebar (bd-nnr22, was filter dashboard bd-8o2gd)
    if (e.key === 'f' && !e.repeat && !_deps.isTextInputFocused()) {
      toggleLeftSidebar();
    }
    // 'g' for control panel (bd-69y6v)
    if (e.key === 'g' && !e.repeat && !_deps.isTextInputFocused()) {
      toggleControlPanel();
    }
    // 'p' for screenshot
    if (e.key === 'p' && !_deps.isTextInputFocused()) {
      captureScreenshot();
    }
    // 'x' for export
    if (e.key === 'x' && !_deps.isTextInputFocused()) {
      exportGraphJSON();
    }
    // Shift+D / Shift+S for epic cycling (bd-pnngb)
    if (e.shiftKey && e.key === 'D' && !_deps.isTextInputFocused()) {
      e.preventDefault();
      _deps.cycleEpic(1);
      return;
    }
    if (e.shiftKey && e.key === 'S' && !_deps.isTextInputFocused()) {
      e.preventDefault();
      _deps.cycleEpic(-1);
      return;
    }
    // Shift+A for Agents View overlay (bd-jgvas)
    if (e.shiftKey && e.key === 'A' && !_deps.isTextInputFocused()) {
      e.preventDefault();
      toggleAgentsView();
      return;
    }
    // 1-5 for layout modes
    const layoutKeys = { 1: 'free', 2: 'dag', 3: 'radial', 4: 'cluster' };
    if (layoutKeys[e.key] && !_deps.isTextInputFocused()) {
      _deps.setLayout(layoutKeys[e.key]);
    }

    // Arrow + WASD keys: track held keys for Quake-style smooth camera (bd-zab4q, bd-pwaen)
    if (
      ['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'w', 'a', 's', 'd'].includes(e.key) &&
      !e.shiftKey &&
      !_deps.isTextInputFocused()
    ) {
      e.preventDefault();
      _keysDown.add(e.key);
    }
  });

  // Release arrow keys — velocity decays via friction in animation loop (bd-zab4q)
  document.addEventListener('keyup', (e) => {
    _keysDown.delete(e.key);
  });
}
