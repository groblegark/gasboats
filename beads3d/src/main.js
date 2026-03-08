import ForceGraph3D from '3d-force-graph';
import * as THREE from 'three';
import { UnrealBloomPass } from 'three/examples/jsm/postprocessing/UnrealBloomPass.js';
// CSS2DRenderer, CSS2DObject moved to mutations.js (bd-7t6nt)
import { BeadsAPI } from './api.js';
import { nodeColor, nodeSize, linkColor, colorToHex, rigColor } from './colors.js';
import {
  createFresnelMaterial,
  createStarField,
  updateShaderTime,
  createMateriaMaterial,
  createMateriaHaloTexture,
  createParticlePool,
  createFairyLights,
} from './shaders.js';
import { LINK_ICON_MATERIALS, LINK_ICON_DEFAULT, LINK_ICON_SCALE } from './link-icons.js';
import { updateRightSidebar, updateEpicProgress, updateDepHealth } from './right-sidebar.js';
import { updateDecisionList, showDecisionLightbox } from './decision-lightbox.js';
import { dootLabel, dootColor, resolveAgentIdLoose } from './event-format.js';
import { updateAssigneeButtons, updateFilterCount } from './filter-dashboard.js';
import { showDetail, hideDetail } from './detail-panel.js';
import {
  setLeftSidebarDeps,
  initLeftSidebar,
  updateAgentRosterFromEvent,
  updateLeftSidebarFocus,
  setLeftSidebarOpen,
  startLeftSidebarIdleTimer,
} from './left-sidebar.js';
import {
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
  appendAgentEvent,
  startAgentWindowIdleTimer,
} from './agent-windows.js';
import {
  setVfxDeps,
  _vfxConfig,
  _auraEmitters,
  _pendingFireworks,
  _activeCollapses,
  getCameraShake,
  clearCameraShake,
  eventSprites,
  EVENT_SPRITE_MAX,
  spawnStatusPulse,
  spawnFireworkBurst,
  spawnShockwave,
  spawnCollapseEffect,
  spawnEdgeSpark,
  spawnCometTrail,
  spawnEnergyBeam,
  triggerClaimComet,
  updateEventSprites,
  updateInProgressAura,
  intensifyAura,
  setVfxIntensity,
  presetVFX,
  adaptiveQualityTick,
  getAdaptiveState,
  applyVfxPreset,
} from './vfx.js';
// setControlPanelDeps, toggleControlPanel, initControlPanel moved to camera.js (bd-7t6nt)
import {
  doots,
  css2dRenderer,
  dootPopups,
  connectLiveUpdates,
  initCSS2DRenderer,
  updateDoots,
  findAgentNode,
  spawnDoot,
  showDootPopup,
  dismissDootPopup,
  applyMutationOptimistic,
  updateBusConnectionState,
} from './mutations.js';
import { setContextMenuDeps, handleNodeRightClick, hideContextMenu, showStatusToast } from './context-menu.js';
import {
  setCameraDeps,
  _keysDown,
  _camVelocity,
  cameraFrozen,
  isBoxSelecting,
  setupControls,
  setupBoxSelect,
  captureScreenshot,
  exportGraphJSON,
  centerCameraOnSelection,
  unfreezeCamera,
  hideBulkMenu,
  showBulkMenu,
  updateCameraMovement,
} from './camera.js';
import {
  setLayoutDeps,
  setLayout,
  getDragSubtree,
  setupAgentTether,
  setupEpicCluster,
  makeTextSprite,
  getCurrentLayout,
  getAgentTetherStrength,
  setAgentTetherStrength,
} from './layout.js';
import {
  setMinimapDeps,
  renderMinimap,
  toggleMinimap,
  getMinimapVisible,
  setMinimapVisible,
} from './minimap.js';

// --- Config ---
const params = new URLSearchParams(window.location.search);
const API_BASE = params.get('api') || '/api';
const API_MODE = params.get('mode') || ''; // 'rest' (kbeads) or 'rpc' (bd-daemon Connect)
const DEEP_LINK_BEAD = params.get('bead') || ''; // bd-he95o: URL deep-linking
const DEEP_LINK_MOLECULE = params.get('molecule') || ''; // bd-lwut6: molecule focus view
const URL_PROFILE = params.get('profile') || ''; // bd-8o2gd phase 4: load named profile from URL
const URL_ASSIGNEE = params.get('assignee') || ''; // bd-8o2gd phase 4: filter by assignee via URL
const URL_STATUS = params.get('status') || ''; // bd-8o2gd phase 4: comma-separated statuses
const URL_TYPES = params.get('types') || ''; // bd-8o2gd phase 4: comma-separated types
const POLL_INTERVAL = 30000; // bd-c1x6p: reduced from 10s to 30s — SSE handles live updates
const MAX_NODES = 1000; // bd-04wet: raised from 500 to show more relevant beads

const api = new BeadsAPI(API_BASE, API_MODE ? { mode: API_MODE } : {});

// --- Shared geometries (reused across all nodes to reduce GC + draw overhead) ---
const GEO = {
  sphereHi: new THREE.SphereGeometry(1, 12, 12), // unit sphere, scaled per-node
  sphereLo: new THREE.SphereGeometry(1, 6, 6), // low-poly glow shell
  torus: new THREE.TorusGeometry(1, 0.15, 6, 20), // unit torus for rings
  octa: new THREE.OctahedronGeometry(1, 0), // blocked spikes
  box: new THREE.BoxGeometry(1, 1, 1), // descent stage, general purpose
  // Agent-specific shared geometries (bd-lzojw: previously created per-node)
  cone: new THREE.ConeGeometry(0.3, 0.5, 6), // thruster nozzle
  legCyl: new THREE.CylinderGeometry(0.04, 0.04, 1, 4), // landing leg
  legPad: new THREE.SphereGeometry(0.12, 4, 4), // landing pad
  antennaCyl: new THREE.CylinderGeometry(0.02, 0.02, 1, 3), // antenna
  // Jack-specific shared geometries (bd-lzojw)
  boltHead: new THREE.CylinderGeometry(1, 1, 0.4, 6), // hex bolt head
  boltShaft: new THREE.CylinderGeometry(0.4, 0.4, 1.2, 6), // bolt shaft
};

// --- Material cache: reuse materials by color+params key (bd-lzojw) ---
// Avoids creating thousands of ShaderMaterial/MeshBasicMaterial instances.
// The `time` uniform is shared across all materia materials (updated in animation loop).
// `selected` defaults to 0.0; selection highlight is handled separately.
const _materialCache = new Map();

function getCachedMateriaMaterial(hexColor, opts = {}) {
  const key = `materia:${hexColor}:${opts.opacity ?? 0.85}:${opts.coreIntensity ?? 1.4}:${opts.breathSpeed ?? 0.0}`;
  if (_materialCache.has(key)) return _materialCache.get(key);
  const mat = createMateriaMaterial(hexColor, opts);
  _materialCache.set(key, mat);
  return mat;
}

function getCachedBasicMaterial(hexColor, opts = {}) {
  const key = `basic:${hexColor}:${opts.opacity ?? 1.0}:${opts.wireframe ? 'w' : ''}`;
  if (_materialCache.has(key)) return _materialCache.get(key);
  const mat = new THREE.MeshBasicMaterial({
    color: hexColor,
    transparent: true,
    opacity: opts.opacity ?? 1.0,
    wireframe: !!opts.wireframe,
  });
  _materialCache.set(key, mat);
  return mat;
}

function getCachedSpriteMaterial(hexColor, opts = {}) {
  const blendKey = opts.blending === 'additive' ? 'add' : 'norm';
  const key = `sprite:${hexColor}:${opts.opacity ?? 1.0}:${blendKey}`;
  if (_materialCache.has(key)) return _materialCache.get(key);
  const mat = new THREE.SpriteMaterial({
    map: opts.map || null,
    color: hexColor,
    transparent: true,
    opacity: opts.opacity ?? 1.0,
    blending: opts.blending === 'additive' ? THREE.AdditiveBlending : THREE.NormalBlending,
    depthWrite: false,
  });
  _materialCache.set(key, mat);
  return mat;
}

// Shared materia halo texture (bd-c7d5z) — lazy-initialized on first use
let _materiaHaloTex = null;

// Link icon textures moved to link-icons.js (bd-7t6nt)

// --- State ---
let graphData = { nodes: [], links: [] };
let graph;
const searchFilter = '';
const statusFilter = new Set(); // empty = show all
const typeFilter = new Set();
const priorityFilter = new Set(); // empty = show all priorities (bd-8o2gd phase 2)
const assigneeFilter = ''; // empty = show all assignees (bd-8o2gd phase 2)
const filterDashboardOpen = false; // slide-out filter panel state (bd-8o2gd phase 2)
const startTime = performance.now();
let selectedNode = null;
const highlightNodes = new Set();
const highlightLinks = new Set();
let bloomPass = null;
const bloomEnabled = false;
let layoutGuides = []; // THREE objects added as layout visual aids (cleaned up on layout switch)

// Search navigation state
let searchResults = []; // ordered list of matching node ids
let searchResultIdx = -1; // current position in results (-1 = none)
// minimapVisible moved to minimap.js (bd-7t6nt)

// Multi-selection state (rubber-band / shift+drag)
const multiSelected = new Set(); // set of node IDs currently multi-selected
const revealedNodes = new Set(); // node IDs force-shown by click-to-reveal (hq-vorf47)
const collapsedEpics = new Set(); // epic IDs whose children are collapsed (kd-XGgiokgQBH)
let focusedMoleculeNodes = new Set(); // node IDs in the focused molecule (bd-lwut6)
// isBoxSelecting, boxSelectStart, cameraFrozen, _keysDown, _camVelocity, CAM_* moved to camera.js (bd-7t6nt)
let labelsVisible = true; // true when persistent info labels are shown on all nodes (bd-1o2f7, bd-oypa2: default on)
// openPanels moved to detail-panel.js (bd-7t6nt)
const activeAgeDays = 7; // age filter: show beads updated within N days (0 = all) (bd-uc0mw)

// Agent filter state (bd-8o2gd: configurable filter dashboard, phase 1)
const agentFilterShow = true; // master toggle — show/hide all agent nodes
const agentFilterOrphaned = false; // show agents with no visible connected beads
const agentFilterRigExclude = new Set(); // hide agents on these rigs (exact match)
const agentFilterNameExclude = []; // glob patterns to hide agents by name (bd-8o2gd phase 4)

// Edge type filter (bd-a0vbd): hide specific edge types to reduce graph density
const depTypeHidden = new Set(['rig_conflict']); // default: hide rig conflict edges

// _pendingFireworks and _activeCollapses moved to vfx.js (bd-7t6nt)
const maxEdgesPerNode = 0; // 0 = unlimited; cap edges per node to reduce hub hairballs (bd-ke2xc)

// Simple glob matcher: supports * (any chars) and ? (single char) (bd-8o2gd phase 4)
function globMatch(pattern, str) {
  const re = new RegExp(
    '^' +
      pattern
        .replace(/[.+^${}()|[\]\\]/g, '\\$&')
        .replace(/\*/g, '.*')
        .replace(/\?/g, '.') +
      '$',
    'i',
  );
  return re.test(str);
}

// Check if a text input element is focused — suppress keyboard shortcuts (beads-lznc)
function isTextInputFocused() {
  const el = document.activeElement;
  if (!el) return false;
  const tag = el.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || el.contentEditable === 'true';
}

// Resource cleanup refs (bd-7n4g8)
let _bloomResizeHandler = null;
let _pollIntervalId = null;
const _searchDebounceTimer = null;

// Live event doots — HTML overlay elements via CSS2DRenderer (bd-bwkdk)
// doots, css2dRenderer, dootPopups moved to mutations.js (bd-7t6nt)

// Doot-triggered issue popups — auto-dismissing cards when doots fire (beads-edy1)

// Agent windows, agents view state moved to agent-windows.js (bd-7t6nt)

// Left Sidebar state (bd-nnr22)
// leftSidebarOpen, _agentRoster moved to left-sidebar.js (bd-7t6nt)

// Epic cycling state — Shift+S/D navigation (bd-pnngb)
let _epicNodes = []; // sorted array of epic nodes, rebuilt on refresh
let _epicCycleIndex = -1; // current position in _epicNodes (-1 = none)

// --- GPU Particle Pool + Selection VFX (bd-m9525) ---
let _particlePool = null; // GPU particle pool instance
let _hoveredNode = null; // currently hovered node for glow warmup
let _hoverGlowTimer = 0; // accumulator for hover glow particle emission
let _selectionOrbitTimer = 0; // accumulator for orbit ring particle emission
let _energyStreamTimer = 0; // accumulator for dependency energy stream particles
const _flyToTrailActive = false; // true during camera fly-to for particle trail

// --- VFX system moved to vfx.js (bd-7t6nt) ---
// _vfxConfig, eventSprites, spawn functions, updateEventSprites, updateInProgressAura, intensifyAura
// are all imported from './vfx.js'

// --- FPS counter + performance monitoring overlay (bd-8nx79) ---
let _perfOverlayVisible = false;
let _perfGraphVisible = false;
const _frameTimes = []; // last 120 frame times (ms)
const _fpsHistory = []; // last 60 FPS values
let _lastFrameTime = 0;
let _perfOverlayEl = null;
let _perfCanvasEl = null;

function createPerfOverlay() {
  if (_perfOverlayEl) return;
  const el = document.createElement('div');
  el.id = 'perf-overlay';
  el.style.cssText =
    'position:fixed;top:8px;right:8px;z-index:10000;font-family:monospace;font-size:11px;background:rgba(0,0,5,0.85);color:#ccc;padding:6px 10px;border-radius:4px;border:1px solid #2a2a3a;pointer-events:none;display:none;min-width:120px;';
  el.innerHTML =
    '<div id="perf-fps" style="font-size:14px;font-weight:bold">-- FPS</div><canvas id="perf-graph" width="120" height="30" style="display:none;margin-top:4px;border:1px solid #222"></canvas><div id="perf-stats" style="margin-top:4px;font-size:9px;color:#888;line-height:1.4"></div>';
  document.body.appendChild(el);
  _perfOverlayEl = el;
  _perfCanvasEl = document.getElementById('perf-graph');
}

function updatePerfOverlay(t) {
  if (!_perfOverlayVisible || !_perfOverlayEl) return;
  const now = performance.now();
  const frameMs = _lastFrameTime > 0 ? now - _lastFrameTime : 16.67;
  _lastFrameTime = now;
  _frameTimes.push(frameMs);
  if (_frameTimes.length > 120) _frameTimes.shift();
  const fps = Math.round(1000 / frameMs);
  _fpsHistory.push(fps);
  if (_fpsHistory.length > 60) _fpsHistory.shift();
  const avgFps = Math.round(_fpsHistory.reduce((a, b) => a + b, 0) / _fpsHistory.length);

  // FPS display with color coding
  const fpsEl = document.getElementById('perf-fps');
  if (fpsEl) {
    const color = avgFps > 50 ? '#2d8a4e' : avgFps > 30 ? '#d4a017' : '#d04040';
    fpsEl.textContent = avgFps + ' FPS';
    fpsEl.style.color = color;
  }

  // Frame time sparkline graph
  if (_perfGraphVisible && _perfCanvasEl) {
    _perfCanvasEl.style.display = 'block';
    const ctx = _perfCanvasEl.getContext('2d');
    const w = 120,
      h = 30;
    ctx.clearRect(0, 0, w, h);
    ctx.fillStyle = 'rgba(0,0,5,0.5)';
    ctx.fillRect(0, 0, w, h);
    const maxMs = 50;
    for (let i = 0; i < _frameTimes.length; i++) {
      const x = (i / 120) * w;
      const barH = Math.min(_frameTimes[i] / maxMs, 1) * h;
      ctx.fillStyle = _frameTimes[i] > 33 ? '#d04040' : '#2d8a4e';
      ctx.fillRect(x, h - barH, Math.max(w / 120, 1), barH);
    }
    // 30fps threshold line
    ctx.strokeStyle = '#d4a01744';
    ctx.beginPath();
    ctx.moveTo(0, h - (33 / maxMs) * h);
    ctx.lineTo(w, h - (33 / maxMs) * h);
    ctx.stroke();
  } else if (_perfCanvasEl) {
    _perfCanvasEl.style.display = 'none';
  }

  // Memory + object stats (every 30 frames to avoid thrashing)
  if (_fpsHistory.length % 30 === 0) {
    const statsEl = document.getElementById('perf-stats');
    if (statsEl) {
      const lines = [];
      // Frame time percentiles (bd-ak9s9)
      if (_frameTimes.length >= 10) {
        const sorted = [..._frameTimes].sort((a, b) => a - b);
        const p50 = sorted[Math.floor(sorted.length * 0.5)].toFixed(1);
        const p95 = sorted[Math.floor(sorted.length * 0.95)].toFixed(1);
        const p99 = sorted[Math.floor(sorted.length * 0.99)].toFixed(1);
        lines.push('p50: ' + p50 + '  p95: ' + p95 + '  p99: ' + p99 + ' ms');
      }
      const mem = performance.memory;
      if (mem)
        lines.push(
          'heap: ' +
            (mem.usedJSHeapSize / 1048576).toFixed(1) +
            ' / ' +
            (mem.totalJSHeapSize / 1048576).toFixed(1) +
            ' MB',
        );
      if (_particlePool) lines.push('particles: ' + _particlePool.activeCount + ' / 2000');
      if (graphData) lines.push('nodes: ' + graphData.nodes.length + '  links: ' + graphData.links.length);
      lines.push('sprites: ' + eventSprites.length);
      if (_auraEmitters) lines.push('auras: ' + _auraEmitters.size);
      const aq = getAdaptiveState();
      lines.push('quality: ' + aq.tier + (aq.manualOverride ? ' (manual)' : ''));
      statsEl.innerHTML = lines.join('<br>');
    }
  }
}

function togglePerfOverlay() {
  createPerfOverlay();
  _perfOverlayVisible = !_perfOverlayVisible;
  _perfOverlayEl.style.display = _perfOverlayVisible ? 'block' : 'none';
  if (!_perfOverlayVisible) _lastFrameTime = 0;
}

function togglePerfGraph() {
  _perfGraphVisible = !_perfGraphVisible;
}

// _agentTetherStrength moved to layout.js (bd-7t6nt)
// Minimap moved to minimap.js (bd-7t6nt)

// --- Persistent node labels (bd-1o2f7) ---
// Creates a THREE.Sprite with canvas-rendered text showing bead info.
// Positioned above the node, billboard-aligned to camera.
function createNodeLabelSprite(node) {
  const pLabel = ['P0', 'P1', 'P2', 'P3', 'P4'][node.priority] || '';
  const title = (node.title || node.id).slice(0, 40) + ((node.title || node.id).length > 40 ? '...' : '');
  // Build lines based on content toggles (bd-xnh54)
  const showId = window.__beads3d_labelShowId !== false;
  const showTitle = window.__beads3d_labelShowTitle !== false;
  const showStatus = window.__beads3d_labelShowStatus !== false;
  const lines = [];
  if (showId) lines.push({ text: node.id, bold: true, color: '#4a9eff' });
  if (showTitle) lines.push({ text: title, bold: false, color: '#e0e0e0' });
  if (showStatus)
    lines.push({
      text: `${node.status || ''}${node.assignee ? ' · ' + node.assignee : ''} · ${pLabel}`,
      bold: false,
      color: '#888',
    });
  if (lines.length === 0) lines.push({ text: node.id, bold: true, color: '#4a9eff' });

  const canvas = document.createElement('canvas');
  const ctx = canvas.getContext('2d');
  // Scale canvas font from control panel slider (6-24 → canvas px via 2.5x multiplier)
  const userSize = window.__beads3d_labelSize || 11;
  const fontSize = Math.round(userSize * 2.5);
  const lineHeight = fontSize * 1.3;
  const padding = Math.round(fontSize * 0.5);

  // Measure text widths for all visible lines
  let textW = 0;
  for (const l of lines) {
    ctx.font = `${l.bold ? 'bold ' : ''}${fontSize}px "SF Mono", "Fira Code", monospace`;
    textW = Math.max(textW, ctx.measureText(l.text).width);
  }

  canvas.width = Math.ceil(textW + padding * 2);
  canvas.height = Math.ceil(lineHeight * lines.length + padding * 2);

  // Background with rounded corners (bd-j8ala: configurable bg opacity)
  const bgOpacity = window.__beads3d_labelBgOpacity ?? 0.85;
  ctx.fillStyle = `rgba(10, 10, 18, ${bgOpacity})`;
  const r = 6,
    cw = canvas.width,
    ch = canvas.height;
  ctx.beginPath();
  ctx.moveTo(r, 0);
  ctx.lineTo(cw - r, 0);
  ctx.arcTo(cw, 0, cw, r, r);
  ctx.lineTo(cw, ch - r);
  ctx.arcTo(cw, ch, cw - r, ch, r);
  ctx.lineTo(r, ch);
  ctx.arcTo(0, ch, 0, ch - r, r);
  ctx.lineTo(0, r);
  ctx.arcTo(0, 0, r, 0, r);
  ctx.closePath();
  ctx.fill();

  // Border (bd-j8ala: configurable)
  if (window.__beads3d_labelBorder !== false) {
    ctx.strokeStyle = 'rgba(74, 158, 255, 0.3)';
    ctx.lineWidth = 1;
    ctx.stroke();
  }

  // Render each visible line
  for (let i = 0; i < lines.length; i++) {
    const l = lines[i];
    ctx.font = `${l.bold ? 'bold ' : ''}${fontSize}px "SF Mono", "Fira Code", monospace`;
    ctx.fillStyle = l.color;
    ctx.fillText(l.text, padding, padding + fontSize + lineHeight * i);
  }

  const texture = new THREE.CanvasTexture(canvas);
  texture.minFilter = THREE.LinearFilter;
  const material = new THREE.SpriteMaterial({
    map: texture,
    transparent: true,
    depthWrite: false,
    depthTest: false,
    sizeAttenuation: false, // flat screen-space labels — no perspective zoom
  });
  const sprite = new THREE.Sprite(material);

  // Scale in screen pixels (sizeAttenuation=false) — constant size regardless of zoom
  // bd-oypa2: read label size from control panel slider (default 11 → spriteH 0.06)
  const aspect = canvas.width / canvas.height;
  const labelSizeVal = window.__beads3d_labelSize || 11;
  const spriteH = labelSizeVal * 0.00545; // 11→0.06, 6→0.033, 24→0.131
  sprite.scale.set(spriteH * aspect, spriteH, 1);

  // Position above the node
  const size = nodeSize(node);
  sprite.position.y = size * 2.5 + spriteH / 2 + 2;

  sprite.userData.nodeLabel = true;
  sprite.userData.baseLabelY = sprite.position.y; // base Y for anti-overlap reset (beads-rgmh)
  sprite.visible = labelsVisible;
  return sprite;
}

function toggleLabels() {
  labelsVisible = !labelsVisible;
  // Immediately run LOD pass to show/hide correct labels (beads-bu3r)
  resolveOverlappingLabels();
  const btn = document.getElementById('btn-labels');
  if (btn) btn.classList.toggle('active', labelsVisible);
}

// --- Label anti-overlap with LOD (beads-rgmh, beads-bu3r) ---
// Priority-based level-of-detail: only the top N labels are visible, where N
// scales with zoom level.  Visible labels get multi-pass screen-space repulsion
// to resolve overlaps.  Lower-priority visible labels fade to reduced opacity.
function resolveOverlappingLabels() {
  if (!graph) return;
  const camera = graph.camera();
  const renderer = graph.renderer();
  if (!camera || !renderer) return;

  const width = renderer.domElement.clientWidth;
  const height = renderer.domElement.clientHeight;
  if (width === 0 || height === 0) return;

  // bd-q9d6h: When there's an active selection, the animation loop manages label
  // visibility (show highlighted, hide rest). LOD should NOT override that.
  const hasActiveSelection = !!(selectedNode || multiSelected.size > 0);

  // --- Phase 1: Collect all label sprites and compute priority scores ---
  const allLabels = [];
  for (const node of graphData.nodes) {
    const threeObj = node.__threeObj;
    if (!threeObj) continue;
    threeObj.traverse((child) => {
      if (!child.userData.nodeLabel) return;
      // Reset to base position before computing new offsets
      if (child.userData.baseLabelY !== undefined) {
        child.position.y = child.userData.baseLabelY;
      }
      allLabels.push({ sprite: child, node });
    });
  }

  if (allLabels.length === 0) return;

  // Compute priority for each label
  for (const l of allLabels) {
    l.pri = _labelPriority(l);
  }

  // --- Phase 2: LOD — determine how many labels to show (beads-bu3r) ---
  // Camera distance to scene center drives the label budget.
  // Close zoom: show many; far zoom: show fewer.
  const camPos = camera.position;
  const camDist = camPos.length(); // distance from origin
  // Budget: at distance 100 show ~40 labels, at 500 show ~12, at 1000 show ~6
  // Selected/multi-selected always shown (budget doesn't apply).
  // bd-lwut6: when a molecule is focused, increase budget to show all its labels.
  const BASE_BUDGET = 40;
  const moleculeBudgetBoost = focusedMoleculeNodes.size > 0 ? focusedMoleculeNodes.size : 0;
  const labelBudget = Math.max(6, Math.round(BASE_BUDGET * (100 / Math.max(camDist, 50)))) + moleculeBudgetBoost;

  // Sort by priority descending — highest priority labels shown first
  allLabels.sort((a, b) => b.pri - a.pri);

  // Always-show: selected, multi-selected, and agents with in_progress tasks
  let budgetUsed = 0;
  for (const l of allLabels) {
    const isForced = l.pri >= 500; // selected or multi-selected
    if (isForced) {
      l.show = true;
    } else if (budgetUsed < labelBudget) {
      l.show = true;
      budgetUsed++;
    } else {
      l.show = false;
    }
  }

  // Apply visibility with hysteresis to prevent blinking (bd-q9d6h)
  // Once shown, a label stays visible for at least 8 LOD cycles (~32 frames)
  // before being hidden. This prevents rapid toggling when camera moves.
  // When there's an active selection, skip LOD visibility — animation loop manages it.
  const userOpacity = window.__beads3d_labelOpacity ?? 0.8;
  for (const l of allLabels) {
    if (!labelsVisible) {
      l.sprite.visible = false;
      l.sprite.userData._lodShownAt = 0;
      continue;
    }
    // bd-q9d6h: Don't fight animation loop during selection — it controls visibility
    if (hasActiveSelection) {
      // Still apply opacity to visible labels, but don't change visibility
      if (l.sprite.visible && l.sprite.material) {
        l.sprite.material.opacity = userOpacity;
      }
      continue;
    }
    // Hysteresis: if label was recently shown, keep it visible even if budget says hide
    if (!l.show && l.sprite.userData._lodShownAt > 0) {
      const framesSinceShown = (animate._labelFrame || 0) - l.sprite.userData._lodShownAt;
      if (framesSinceShown < 8) {
        l.show = true; // keep visible — hysteresis grace period
      }
    }
    if (l.show) {
      l.sprite.userData._lodShownAt = animate._labelFrame || 0;
    } else {
      l.sprite.userData._lodShownAt = 0;
    }
    l.sprite.visible = l.show;
    // Opacity fade: top labels get full opacity, lower ones fade (beads-bu3r)
    // Multiply by user opacity slider so both LOD fade and slider work together
    if (l.show && l.sprite.material) {
      const rank = allLabels.indexOf(l);
      const fadeStart = Math.max(6, labelBudget * 0.6);
      let lodOpacity;
      if (rank < fadeStart || l.pri >= 500) {
        lodOpacity = 1.0;
      } else {
        // Fade from 1.0 down to 0.35 for the lowest-ranked visible labels
        const fadeRange = Math.max(1, labelBudget - fadeStart);
        const t = Math.min(1, (rank - fadeStart) / fadeRange);
        lodOpacity = 1.0 - t * 0.65;
      }
      l.sprite.material.opacity = lodOpacity * userOpacity;
    }
  }

  // --- Phase 3: Project visible labels to screen space ---
  const visibleLabels = [];
  for (const l of allLabels) {
    if (!l.show || !labelsVisible) continue;
    const worldPos = new THREE.Vector3();
    l.sprite.getWorldPosition(worldPos);
    const ndc = worldPos.clone().project(camera);
    if (ndc.z > 1) continue; // behind camera
    const sx = (ndc.x * 0.5 + 0.5) * width;
    const sy = (-ndc.y * 0.5 + 0.5) * height;
    const lw = l.sprite.scale.x * height;
    const lh = l.sprite.scale.y * height;
    visibleLabels.push({ ...l, sx, sy, lw, lh, offsetY: 0 });
  }

  if (visibleLabels.length < 2) return;

  // --- Phase 4: Multi-pass overlap resolution (beads-bu3r, bd-5rwn3) ---
  // Run up to 8 passes of pairwise repulsion. Higher-priority labels hold
  // position; lower-priority labels are pushed away from the overlap.
  // Direction is determined by relative screen-Y: if the lower-priority
  // label's center is below (or equal), push it down; otherwise push up.
  // This prevents labels from piling in one direction.
  const PADDING = 14; // wider gap between labels (bd-w0hbr: agent labels)
  const MAX_OFFSET = height * 0.3; // allow more spread to separate dense clusters
  for (let pass = 0; pass < 12; pass++) {
    // Sort by screen X for sweep-and-prune
    visibleLabels.sort((a, b) => a.sx - b.sx);
    let anyMoved = false;

    for (let i = 0; i < visibleLabels.length; i++) {
      const a = visibleLabels[i];
      const aRight = a.sx + a.lw / 2;

      for (let j = i + 1; j < visibleLabels.length; j++) {
        const b = visibleLabels[j];
        const bLeft = b.sx - b.lw / 2;
        if (bLeft > aRight + PADDING) break;

        const aCy = a.sy + a.offsetY;
        const bCy = b.sy + b.offsetY;
        const aTop = aCy - a.lh / 2;
        const aBot = aCy + a.lh / 2;
        const bTop = bCy - b.lh / 2;
        const bBot = bCy + b.lh / 2;

        if (aTop > bBot + PADDING || bTop > aBot + PADDING) continue;

        // Full separation needed to clear the overlap + padding
        const overlapY = Math.min(aBot, bBot) - Math.max(aTop, bTop) + PADDING;
        // Push the lower-priority label away from the higher-priority one
        if (a.pri >= b.pri) {
          // Push b away from a: if b is below or same, push down; else up
          const dir = bCy >= aCy ? 1 : -1;
          b.offsetY += overlapY * dir;
          if (Math.abs(b.offsetY) > MAX_OFFSET) b.offsetY = MAX_OFFSET * Math.sign(b.offsetY);
        } else {
          const dir = aCy >= bCy ? 1 : -1;
          a.offsetY += overlapY * dir;
          if (Math.abs(a.offsetY) > MAX_OFFSET) a.offsetY = MAX_OFFSET * Math.sign(a.offsetY);
        }
        anyMoved = true;
      }
    }
    // Early exit if no overlaps remain
    if (!anyMoved) break;
  }

  // Apply offsets back to sprite world positions (bd-5rwn3).
  // Since sizeAttenuation=false, sprite.scale is in viewport fractions but
  // sprite.position is in the parent group's world space. We must convert
  // screen-pixel offsets to world-space Y offsets using the camera projection.
  for (const l of visibleLabels) {
    if (l.offsetY === 0) continue;
    // Compute world-to-screen Y scale at this sprite's depth
    const worldPos = new THREE.Vector3();
    l.sprite.getWorldPosition(worldPos);
    const camDist = camera.position.distanceTo(worldPos);
    // For perspective camera: world units per pixel = 2 * dist * tan(fov/2) / height
    const fovRad = (camera.fov * Math.PI) / 180;
    const worldPerPx = (2 * camDist * Math.tan(fovRad / 2)) / height;
    l.sprite.position.y -= l.offsetY * worldPerPx; // screen Y is inverted vs world Y
  }
}

function _labelPriority(label) {
  const n = label.node;
  // Higher = wins position contest (stays in place) and survives LOD culling
  let pri = 0;
  if (selectedNode && n.id === selectedNode.id) pri += 1000;
  if (multiSelected.has(n.id)) pri += 500;
  // bd-lwut6: boost labels in focused molecule — always show at full opacity
  if (focusedMoleculeNodes.has(n.id)) pri += 500;
  if (n.issue_type === 'agent') {
    pri += 100;
    // Spread agent priorities by name hash to break ties in overlap resolution (bd-w0hbr)
    let h = 0;
    for (let i = 0; i < (n.id?.length || 0); i++) h = ((h << 3) - h + n.id.charCodeAt(i)) | 0;
    pri += Math.abs(h) % 20;
  }
  // In-progress beads are more important than open/closed
  if (n.status === 'in_progress') pri += 50;
  // Lower priority number = more important
  pri += (4 - (n.priority || 2)) * 10;
  return pri;
}

// --- Build graph ---
function initGraph() {
  graph = ForceGraph3D({ rendererConfig: { preserveDrawingBuffer: true } })(document.getElementById('graph'))
    .backgroundColor('#0a0a0f')
    .showNavInfo(false)

    // Custom node rendering — organic vacuole look (shared geometries for perf)
    .nodeThreeObject((n) => {
      if (n._hidden) return new THREE.Group();

      // Revealed-but-filtered nodes render as ghosts — reduced opacity (hq-vorf47)
      const isGhost = !!n._revealed;
      const ghostFade = isGhost ? 0.4 : 1.0;

      const size = nodeSize(n);
      const hexColor = colorToHex(nodeColor(n));
      const group = new THREE.Group();

      // Materia orb core — FFVII-style inner glow (bd-c7d5z, bd-lzojw: cached materials)
      const breathSpeed = n.status === 'in_progress' ? 2.0 : 0.0; // bd-pe8k2: only in_progress pulses
      const coreIntensity = n.status === 'closed' ? 0.5 : n.status === 'in_progress' ? 1.8 : 1.4;
      const coreOpacity = n.status === 'closed' ? 0.4 : 0.85;
      const core = new THREE.Mesh(
        GEO.sphereHi,
        getCachedMateriaMaterial(hexColor, {
          opacity: coreOpacity * ghostFade,
          coreIntensity,
          breathSpeed,
        }),
      );
      core.scale.setScalar(size);
      group.add(core);

      // Materia halo sprite — soft radial gradient billboard (bd-c7d5z, bd-lzojw: cached material)
      if (!_materiaHaloTex) _materiaHaloTex = createMateriaHaloTexture(64);
      const halo = new THREE.Sprite(
        getCachedSpriteMaterial(hexColor, {
          map: _materiaHaloTex,
          opacity: 0.2 * ghostFade,
          blending: 'additive',
        }),
      );
      halo.scale.setScalar(size * 3.0);
      group.add(halo);

      // Agent: retro lunar lander — cute spaceship with landing legs (beads-yp2y)
      if (n.issue_type === 'agent') {
        group.remove(core);
        group.remove(halo);
        const s = size;
        // bd-lzojw: cached materials instead of new per-agent allocations
        const matOrange = getCachedBasicMaterial(0xff6b35, { opacity: 0.85 });
        const matDark = getCachedBasicMaterial(0x2a2a3a, { opacity: 0.9 });
        const matGold = getCachedBasicMaterial(0xd4a017, { opacity: 0.7 });
        const matWindow = getCachedBasicMaterial(0x88ccff, { opacity: 0.8 });

        // Cabin — squat octahedron (angular Apollo LM shape)
        const cabin = new THREE.Mesh(GEO.octa, matOrange);
        cabin.scale.set(s * 1.0, s * 0.7, s * 1.0);
        group.add(cabin);

        // Viewport window — small sphere on front face
        const window = new THREE.Mesh(GEO.sphereHi, matWindow);
        window.scale.setScalar(s * 0.25);
        window.position.set(0, s * 0.15, s * 0.55);
        group.add(window);

        // Descent stage — wider box below cabin
        const descent = new THREE.Mesh(GEO.box, matGold);
        descent.scale.set(s * 1.4, s * 0.35, s * 1.4);
        descent.position.y = -s * 0.55;
        group.add(descent);

        // Thruster nozzle — cone below descent stage (bd-lzojw: shared geometry)
        const nozzle = new THREE.Mesh(GEO.cone, matDark);
        nozzle.scale.setScalar(s);
        nozzle.position.y = -s * 1.0;
        nozzle.rotation.x = Math.PI; // point down
        group.add(nozzle);

        // Landing legs — 4 angled cylinders (bd-lzojw: shared geometries)
        for (let i = 0; i < 4; i++) {
          const angle = (i / 4) * Math.PI * 2 + Math.PI / 4;
          const leg = new THREE.Mesh(GEO.legCyl, matOrange);
          leg.scale.setScalar(s);
          leg.scale.y = s * 1.2;
          leg.position.set(Math.cos(angle) * s * 0.7, -s * 0.9, Math.sin(angle) * s * 0.7);
          leg.rotation.z = Math.cos(angle) * 0.4;
          leg.rotation.x = -Math.sin(angle) * 0.4;
          group.add(leg);
          // Landing pad at foot
          const pad = new THREE.Mesh(GEO.legPad, matGold);
          pad.scale.setScalar(s);
          pad.position.set(Math.cos(angle) * s * 1.1, -s * 1.5, Math.sin(angle) * s * 1.1);
          group.add(pad);
        }

        // Antenna — thin cylinder on top (bd-lzojw: shared geometry)
        const antenna = new THREE.Mesh(GEO.antennaCyl, matOrange);
        antenna.scale.setScalar(s);
        antenna.scale.y = s * 0.8;
        antenna.position.y = s * 0.8;
        group.add(antenna);
        // Antenna tip
        const tip = new THREE.Mesh(GEO.sphereHi, matOrange);
        tip.scale.setScalar(s * 0.1);
        tip.position.y = s * 1.3;
        group.add(tip);

        // Outer glow — orange fresnel shell (bd-s9b4v, bd-lzojw: cached material)
        const _fresnelKey = 'fresnel:0xff6b35:0.12:3.5';
        let fresnelMat = _materialCache.get(_fresnelKey);
        if (!fresnelMat) {
          fresnelMat = createFresnelMaterial(0xff6b35, { opacity: 0.12, power: 3.5 });
          _materialCache.set(_fresnelKey, fresnelMat);
        }
        const agentGlow = new THREE.Mesh(GEO.sphereLo, fresnelMat);
        agentGlow.scale.setScalar(size * 1.3);
        agentGlow.userData.agentGlow = true;
        agentGlow.userData.baseScale = size * 1.5;
        group.add(agentGlow);

        // Wake trail — elongated sprite behind agent's direction of travel (beads-v0wa)
        const trailMat = new THREE.SpriteMaterial({
          color: 0xff6b35,
          transparent: true,
          opacity: 0.0, // starts invisible
        });
        const trail = new THREE.Sprite(trailMat);
        trail.scale.set(size * 0.4, size * 3, 1);
        trail.userData.agentTrail = true;
        trail.userData.prevPos = { x: 0, y: 0, z: 0 };
        group.add(trail);

        // Rig badge — colored label below landing pads (bd-90ikf)
        if (n.rig) {
          const rc = rigColor(n.rig);
          const rigSprite = makeTextSprite(n.rig, {
            fontSize: 18,
            color: rc,
            background: 'rgba(8, 8, 16, 0.85)',
            sizeAttenuation: false,
            screenHeight: 0.025,
          });
          rigSprite.position.y = -size * 2.2;
          rigSprite.renderOrder = 999;
          rigSprite.userData.rigBadge = true;
          group.add(rigSprite);
        }
      }

      // Decision/gate: diamond shape with "?" marker, only pending visible (bd-zr374, bd-lzojw: cached)
      if (n.issue_type === 'gate' || n.issue_type === 'decision') {
        // Replace sphere core with elongated octahedron (diamond)
        group.remove(core);
        const diamond = new THREE.Mesh(
          GEO.octa,
          getCachedBasicMaterial(hexColor, { opacity: 0.9 * ghostFade }),
        );
        diamond.scale.set(size * 0.8, size * 1.4, size * 0.8); // tall diamond
        group.add(diamond);

        // "?" question mark above node — screen-space for readability
        const qSprite = makeTextSprite('?', {
          fontSize: 32,
          color: '#d4a017',
          opacity: 0.95 * ghostFade,
          background: 'rgba(10, 10, 18, 0.85)',
          sizeAttenuation: false,
          screenHeight: 0.04,
        });
        qSprite.position.y = size * 2.5;
        qSprite.renderOrder = 998;
        group.add(qSprite);

        // Pulsing glow wireframe for pending decisions
        const pulseGlow = new THREE.Mesh(
          GEO.octa,
          getCachedBasicMaterial(0xd4a017, { opacity: 0.25 * ghostFade, wireframe: true }),
        );
        pulseGlow.scale.set(size * 1.2, size * 2.0, size * 1.2);
        pulseGlow.userData.decisionPulse = true;
        group.add(pulseGlow);
      }

      // Jack: hexagonal bolt — temporary infrastructure modification (bd-hffzf, bd-lzojw: shared geo + cached mat)
      if (n.issue_type === 'jack') {
        group.remove(core);
        const s = size;
        const jackColor = n._jackExpired ? 0xd04040 : 0xe06830; // red if expired, orange-red if active

        // Hexagonal bolt head — wide, flat cylinder with 6 sides (bd-lzojw: shared GEO.boltHead)
        const boltHead = new THREE.Mesh(GEO.boltHead, getCachedBasicMaterial(jackColor, { opacity: 0.85 * ghostFade }));
        boltHead.scale.set(s * 1.0, s * 1.0, s * 1.0);
        group.add(boltHead);

        // Bolt shaft — narrower cylinder below the head (bd-lzojw: shared GEO.boltShaft)
        const shaft = new THREE.Mesh(GEO.boltShaft, getCachedBasicMaterial(jackColor, { opacity: 0.7 * ghostFade }));
        shaft.scale.setScalar(s);
        shaft.position.y = -s * 0.8;
        group.add(shaft);

        // Active jack: pulsing glow ring (wireframe hexagonal torus, bd-lzojw: shared geo + cached mat)
        if (n.status === 'in_progress' && !n._jackExpired) {
          const pulseRing = new THREE.Mesh(
            GEO.boltHead,
            getCachedBasicMaterial(0xe06830, { opacity: 0.2 * ghostFade, wireframe: true }),
          );
          pulseRing.scale.set(s * 1.4, s * 0.6, s * 1.4);
          pulseRing.userData.jackPulse = true;
          group.add(pulseRing);
        }

        // Expired jack: flashing red warning ring (bd-lzojw: shared geo + cached mat)
        if (n._jackExpired) {
          const warnRing = new THREE.Mesh(
            GEO.boltHead,
            getCachedBasicMaterial(0xff2020, { opacity: 0.3 * ghostFade, wireframe: true }),
          );
          warnRing.scale.set(s * 1.6, s * 0.8, s * 1.6);
          warnRing.userData.jackExpiredFlash = true;
          group.add(warnRing);

          // "!" warning above expired jack
          const warnSprite = makeTextSprite('!', {
            fontSize: 28,
            color: '#ff2020',
            opacity: 0.9 * ghostFade,
            background: 'rgba(10, 10, 18, 0.85)',
            sizeAttenuation: false,
            screenHeight: 0.035,
          });
          warnSprite.position.y = size * 2.0;
          warnSprite.renderOrder = 998;
          group.add(warnSprite);
        }
      }

      // Blocked: spiky octahedron (bd-lzojw: cached material)
      if (n._blocked) {
        const spike = new THREE.Mesh(
          GEO.octa,
          getCachedBasicMaterial(0xd04040, { opacity: 0.2, wireframe: true }),
        );
        spike.scale.setScalar(size * 2.4);
        group.add(spike);
      }

      // Pending decision badge — small amber dot with count (bd-o6tgy, bd-lzojw: cached material)
      if (n._pendingDecisions > 0 && n.issue_type !== 'gate' && n.issue_type !== 'decision') {
        const badge = new THREE.Mesh(
          GEO.sphereHi,
          getCachedBasicMaterial(0xd4a017, { opacity: 0.9 }),
        );
        badge.scale.setScalar(Math.min(3 + n._pendingDecisions, 6));
        badge.position.set(size * 1.2, size * 1.2, 0); // top-right offset
        badge.userData.decisionBadge = true;
        group.add(badge);
      }

      // Commit count badge — small number below-left of node (bd-90ikf)
      // Lights up when backend adds commit_count field to GraphNode
      if (n.commit_count > 0 && n.issue_type !== 'agent') {
        const ccSprite = makeTextSprite(`${n.commit_count}`, {
          fontSize: 16,
          color: '#a0d050',
          background: 'rgba(8, 8, 16, 0.85)',
          sizeAttenuation: false,
          screenHeight: 0.02,
        });
        ccSprite.position.set(-size * 1.2, -size * 1.2, 0); // bottom-left
        ccSprite.renderOrder = 998;
        ccSprite.userData.commitBadge = true;
        group.add(ccSprite);
      }

      // Attachment/media badge — paperclip icon below-right of node (kd-XGgiokgQBH)
      // Lights up when backend provides jira_attachment_count or jira_has_media fields
      const attachCount = parseInt(n.jira_attachment_count, 10) || 0;
      const hasMedia = n.jira_has_media === 'true' || n.jira_has_media === true;
      if ((attachCount > 0 || hasMedia) && n.issue_type !== 'agent') {
        const label = hasMedia ? `\u{1F4CE}${attachCount || ''}` : `\u{1F4CE}${attachCount}`;
        const attachSprite = makeTextSprite(label, {
          fontSize: 14,
          color: '#60d0c0',
          background: 'rgba(8, 8, 16, 0.85)',
          sizeAttenuation: false,
          screenHeight: 0.02,
        });
        attachSprite.position.set(size * 1.2, -size * 1.2, 0); // bottom-right
        attachSprite.renderOrder = 998;
        attachSprite.userData.attachBadge = true;
        group.add(attachSprite);
      }

      // Epic collapsed indicator — "+" badge when children are hidden (kd-XGgiokgQBH)
      if (n.issue_type === 'epic' && collapsedEpics.has(n.id)) {
        const collapseSprite = makeTextSprite('+', {
          fontSize: 22,
          color: '#8b45a6',
          background: 'rgba(8, 8, 16, 0.9)',
          sizeAttenuation: false,
          screenHeight: 0.025,
        });
        collapseSprite.position.set(0, size * 1.8, 0); // above node
        collapseSprite.renderOrder = 999;
        collapseSprite.userData.collapseBadge = true;
        group.add(collapseSprite);
      }

      // Selection glow — materia intensification instead of orbiting ring (bd-c7d5z)
      // The core materia material has a `selected` uniform (0=off, 1=on)
      // Updated in animation loop to boost glow when selected
      core.userData.materiaCore = true;

      // Persistent info label sprite (bd-1o2f7) — hidden until 'l' toggles labels on
      const labelSprite = createNodeLabelSprite(n);
      group.add(labelSprite);

      return group;
    })
    .nodeLabel(() => '')
    .nodeVisibility((n) => !n._hidden)

    // Link rendering — width responds to selection state
    .linkColor((l) => linkColor(l))
    .linkOpacity(0.55)
    .linkWidth((l) => {
      if (selectedNode) {
        const lk = linkKey(l);
        return highlightLinks.has(lk) ? (l.dep_type === 'blocks' ? 2.0 : 1.2) : 0.15;
      }
      if (l.dep_type === 'rig_conflict') return 2.5; // thick red conflict edge (bd-90ikf)
      // Thinner assignment edges to reduce visual clutter in dense graphs (bd-ld2fa)
      return l.dep_type === 'blocks' ? 1.2 : l.dep_type === 'assigned_to' ? 0.6 : 0.5;
    })
    .linkDirectionalArrowLength(5)
    .linkDirectionalArrowRelPos(1)
    .linkDirectionalArrowColor((l) => linkColor(l))
    .linkVisibility((l) => {
      const src = typeof l.source === 'object' ? l.source : graphData.nodes.find((n) => n.id === l.source);
      const tgt = typeof l.target === 'object' ? l.target : graphData.nodes.find((n) => n.id === l.target);
      return src && tgt && !src._hidden && !tgt._hidden && !depTypeHidden.has(l.dep_type);
    })

    // Link icons — sprite at midpoint showing dep type (shield=blocks, clock=waits, chain=parent)
    .linkThreeObjectExtend(true)
    .linkThreeObject((l) => {
      const baseMat = LINK_ICON_MATERIALS[l.dep_type] || LINK_ICON_DEFAULT;
      const sprite = new THREE.Sprite(baseMat.clone());
      sprite.scale.setScalar(LINK_ICON_SCALE);

      if (l.dep_type === 'assigned_to') {
        // Agent link: icon sprite + pulsing glow tube (beads-v0wa)
        const group = new THREE.Group();
        group.userData.isAgentLink = true;
        group.add(sprite);
        const tubeMat = new THREE.MeshBasicMaterial({
          color: 0xff6b35,
          transparent: true,
          opacity: 0.12,
        });
        const tube = new THREE.Mesh(new THREE.CylinderGeometry(0.6, 0.6, 1, 6, 1, true), tubeMat);
        tube.userData.isGlowTube = true;
        group.add(tube);
        return group;
      }
      return sprite;
    })
    .linkPositionUpdate((obj, { start, end }, l) => {
      const mid = {
        x: (start.x + end.x) / 2,
        y: (start.y + end.y) / 2,
        z: (start.z + end.z) / 2,
      };
      if (obj && obj.userData.isAgentLink) {
        // Agent link group: icon at midpoint, glow tube stretched between endpoints
        const t = (performance.now() - startTime) / 1000;
        const dimTarget = selectedNode ? (highlightLinks.has(linkKey(l)) ? 1.0 : 0.08) : 1.0;
        for (const child of obj.children) {
          if (child.isSprite) {
            child.position.set(mid.x, mid.y, mid.z);
            child.material.opacity = 0.85 * dimTarget;
          } else if (child.userData.isGlowTube) {
            child.position.set(mid.x, mid.y, mid.z);
            const dx = end.x - start.x,
              dy = end.y - start.y,
              dz = end.z - start.z;
            const dist = Math.sqrt(dx * dx + dy * dy + dz * dz) || 1;
            child.scale.set(1, dist, 1);
            child.lookAt(end.x, end.y, end.z);
            child.rotateX(Math.PI / 2);
            child.material.opacity = (0.08 + Math.sin(t * 3) * 0.06) * dimTarget;
          }
        }
      } else if (obj && obj.isSprite) {
        obj.position.set(mid.x, mid.y, mid.z);
        if (selectedNode) {
          obj.material.opacity = highlightLinks.has(linkKey(l)) ? 0.85 : 0.08;
        } else {
          obj.material.opacity = 0.85;
        }
      }
    })

    // Directional particles — blocking links + agent tethers (beads-1gx1)
    .linkDirectionalParticles((l) => (l.dep_type === 'blocks' ? 2 : l.dep_type === 'assigned_to' ? 3 : 0))
    .linkDirectionalParticleWidth((l) => (l.dep_type === 'assigned_to' ? 1.8 : 1.0))
    .linkDirectionalParticleSpeed((l) => (l.dep_type === 'assigned_to' ? 0.008 : 0.003))
    .linkDirectionalParticleColor((l) => linkColor(l))

    // Interaction
    .onNodeHover(handleNodeHover)
    .onNodeClick(handleNodeClick)
    .onNodeRightClick(handleNodeRightClick)
    .onBackgroundClick(() => {
      clearSelection();
      hideTooltip();
      hideDetail();
      hideContextMenu();
    })
    // DAG dragging: dragged node pulls its subtree with spring physics (beads-6253)
    .onNodeDrag((node) => {
      if (!node || node._hidden) return;
      if (!node._dragSubtree) {
        node._dragSubtree = getDragSubtree(node.id);
      }
      // Apply velocity impulses to subtree proportional to drag delta
      // Agents get stronger, snappier coupling to their assigned beads (beads-mxhq)
      const isAgent = node.issue_type === 'agent';
      const subtree = node._dragSubtree;
      for (const { node: child, depth } of subtree) {
        if (child === node || child.fx !== undefined) continue;
        // Agents: stronger base pull (0.6 vs 0.3), slower decay (0.65 vs 0.5)
        const baseStrength = isAgent ? 0.6 : 0.3;
        const decay = isAgent ? 0.65 : 0.5;
        const strength = baseStrength * Math.pow(decay, depth - 1);
        const dx = node.x - child.x;
        const dy = node.y - child.y;
        const dz = (node.z || 0) - (child.z || 0);
        const dist = Math.sqrt(dx * dx + dy * dy + dz * dz) || 1;
        const restDist = (isAgent ? 20 : 30) * depth;
        if (dist > restDist) {
          // Agents: higher impulse multiplier for snappy response
          const impulse = isAgent ? 0.12 : 0.05;
          child.vx = (child.vx || 0) + dx * strength * impulse;
          child.vy = (child.vy || 0) + dy * strength * impulse;
          child.vz = (child.vz || 0) + dz * strength * impulse;
        }
      }
    })
    .onNodeDragEnd((node) => {
      if (node) delete node._dragSubtree;
    });

  // Force tuning — applied by setLayout()
  const nodeCount = graphData.nodes.length || 100;

  // Warm up faster then cool (reduces CPU after initial layout)
  graph.cooldownTime(4000).warmupTicks(nodeCount > 200 ? 50 : 0);

  // Apply default layout forces
  setLayout('free');

  // Scene extras
  const scene = graph.scene();
  scene.fog = new THREE.FogExp2(0x0a0a0f, 0.0001);

  // Fairy lights — drifting luminous particles (bd-52izs, replaces geodesic dome nucleus/membrane)
  const fairyLights = createFairyLights(300, 250);
  scene.add(fairyLights);

  scene.add(new THREE.AmbientLight(0x404060, 0.5));

  // Star field — subtle background particles for depth
  const stars = createStarField(1500, 500);
  scene.add(stars);

  // GPU particle pool for all VFX (bd-m9525)
  _particlePool = createParticlePool(2000);
  _particlePool.mesh.userData.isParticlePool = true;
  scene.add(_particlePool.mesh);

  // Extend camera draw distance
  const camera = graph.camera();
  camera.far = 50000;
  camera.updateProjectionMatrix();

  // Bloom post-processing
  bloomPass = new UnrealBloomPass(
    new THREE.Vector2(window.innerWidth, window.innerHeight),
    0.7, // strength — subtle glow (bd-s9b4v: reduced from 1.2)
    0.4, // radius — tighter spread (bd-s9b4v: reduced from 0.6)
    0.35, // threshold — higher so only bright nodes bloom (bd-s9b4v: raised from 0.2)
  );
  bloomPass.enabled = bloomEnabled;
  const composer = graph.postProcessingComposer();
  composer.addPass(bloomPass);

  // Handle window resize for bloom (bd-7n4g8: store ref for cleanup)
  if (_bloomResizeHandler) window.removeEventListener('resize', _bloomResizeHandler);
  _bloomResizeHandler = () => bloomPass.resolution.set(window.innerWidth, window.innerHeight);
  window.addEventListener('resize', _bloomResizeHandler);

  // CSS2D overlay renderer for HTML doots (bd-bwkdk)
  initCSS2DRenderer();

  // Start animation loop for pulsing effects
  startAnimation();

  return graph;
}

// --- Selection VFX (bd-m9525) ---
// Continuous particle effects for selected/hovered nodes.

// Update all selection-related VFX each frame.
function updateSelectionVFX(t) {
  const dt = 0.016; // ~60fps

  // 1. Hover glow warmup — gentle particle emission on hovered node
  if (_hoveredNode && _hoveredNode !== selectedNode) {
    _hoverGlowTimer += dt;
    if (_hoverGlowTimer > _vfxConfig.hoverRate) {
      _hoverGlowTimer = 0;
      const pos = { x: _hoveredNode.x || 0, y: _hoveredNode.y || 0, z: _hoveredNode.z || 0 };
      const size = nodeSize({ priority: _hoveredNode.priority, issue_type: _hoveredNode.issue_type });
      _particlePool.emit(pos, 0x4a9eff, 2, {
        velocity: [0, 0.5, 0],
        spread: size * 0.6,
        lifetime: _vfxConfig.particleLifetime * 0.75,
        size: _vfxConfig.hoverSize,
      });
    }
  }

  // 2. Selection orbit ring — particles orbiting the selected node
  if (selectedNode) {
    _selectionOrbitTimer += dt;
    if (_selectionOrbitTimer > _vfxConfig.orbitRate) {
      _selectionOrbitTimer = 0;
      const pos = { x: selectedNode.x || 0, y: selectedNode.y || 0, z: selectedNode.z || 0 };
      const size = nodeSize({ priority: selectedNode.priority, issue_type: selectedNode.issue_type });
      const radius = size * 1.8;
      const angle = t * _vfxConfig.orbitSpeed;
      // Emit at orbit position with tangential velocity
      const orbitPos = {
        x: pos.x + Math.cos(angle) * radius,
        y: pos.y + (Math.random() - 0.5) * size * 0.5,
        z: pos.z + Math.sin(angle) * radius,
      };
      _particlePool.emit(orbitPos, 0x4a9eff, 1, {
        velocity: [-Math.sin(angle) * 1.5, 0.3, Math.cos(angle) * 1.5],
        spread: 0.3,
        lifetime: _vfxConfig.particleLifetime,
        size: _vfxConfig.orbitSize,
      });
      // Second particle at opposite side
      const orbitPos2 = {
        x: pos.x + Math.cos(angle + Math.PI) * radius,
        y: pos.y + (Math.random() - 0.5) * size * 0.5,
        z: pos.z + Math.sin(angle + Math.PI) * radius,
      };
      _particlePool.emit(orbitPos2, 0x4a9eff, 1, {
        velocity: [-Math.sin(angle + Math.PI) * 1.5, 0.3, Math.cos(angle + Math.PI) * 1.5],
        spread: 0.3,
        lifetime: _vfxConfig.particleLifetime,
        size: _vfxConfig.orbitSize,
      });
    }
  }

  // 3. Dependency energy streams — particle flow along highlighted links
  if (selectedNode && highlightLinks.size > 0) {
    _energyStreamTimer += dt;
    if (_energyStreamTimer > _vfxConfig.streamRate) {
      _energyStreamTimer = 0;
      for (const l of graphData.links) {
        if (!highlightLinks.has(linkKey(l))) continue;
        const src = typeof l.source === 'object' ? l.source : graphData.nodes.find((n) => n.id === l.source);
        const tgt = typeof l.target === 'object' ? l.target : graphData.nodes.find((n) => n.id === l.target);
        if (!src || !tgt) continue;
        // Spawn particle at random point along the link
        const progress = Math.random();
        const pos = {
          x: (src.x || 0) + ((tgt.x || 0) - (src.x || 0)) * progress,
          y: (src.y || 0) + ((tgt.y || 0) - (src.y || 0)) * progress,
          z: (src.z || 0) + ((tgt.z || 0) - (src.z || 0)) * progress,
        };
        // Velocity towards target
        const dx = (tgt.x || 0) - (src.x || 0);
        const dy = (tgt.y || 0) - (src.y || 0);
        const dz = (tgt.z || 0) - (src.z || 0);
        const len = Math.sqrt(dx * dx + dy * dy + dz * dz) || 1;
        const speed = _vfxConfig.streamSpeed;
        const linkColorHex = l.dep_type === 'blocks' ? 0xd04040 : l.dep_type === 'assigned_to' ? 0xff6b35 : 0x4a9eff;
        _particlePool.emit(pos, linkColorHex, 1, {
          velocity: [(dx / len) * speed, (dy / len) * speed, (dz / len) * speed],
          spread: 0.5,
          lifetime: Math.min(len / (speed * 60), _vfxConfig.particleLifetime * 1.9),
          size: 1.0,
        });
      }
    }
  }

  // 4. Connected materia pulse — highlighted nodes pulse particles in unison
  if (selectedNode && highlightNodes.size > 1) {
    const pulseBeat = Math.sin(t * 3) > 0.95; // brief pulse every ~2 seconds
    if (pulseBeat && !updateSelectionVFX._lastPulse) {
      for (const nodeId of highlightNodes) {
        if (nodeId === selectedNode.id) continue;
        const node = graphData.nodes.find((n) => n.id === nodeId);
        if (!node) continue;
        const pos = { x: node.x || 0, y: node.y || 0, z: node.z || 0 };
        const color = nodeColor(node);
        _particlePool.emit(pos, color, 4, {
          velocity: [0, 1.5, 0],
          spread: 2,
          lifetime: _vfxConfig.particleLifetime,
          size: _vfxConfig.orbitSize,
        });
      }
    }
    updateSelectionVFX._lastPulse = pulseBeat;
  }
}
updateSelectionVFX._lastPulse = false;

// Spawn selection burst — enhanced materia intensification on click (bd-m9525)
function spawnSelectionBurst(node) {
  if (!_particlePool || !node) return;
  const pos = { x: node.x || 0, y: node.y || 0, z: node.z || 0 };
  const size = nodeSize({ priority: node.priority, issue_type: node.issue_type });
  const color = nodeColor(node);

  // Inner materia burst — bright core particles
  _particlePool.emit(pos, 0xffffff, Math.round(6 * _vfxConfig.selectionGlow), {
    velocity: [0, 2, 0],
    spread: size * 0.3,
    lifetime: 0.5 * _vfxConfig.selectionGlow,
    size: 2.0 * _vfxConfig.selectionGlow,
  });

  // Outer colored burst — expanding ring of node-colored particles
  for (let i = 0; i < 12; i++) {
    const angle = (i / 12) * Math.PI * 2;
    const ringPos = {
      x: pos.x + Math.cos(angle) * size * 0.5,
      y: pos.y,
      z: pos.z + Math.sin(angle) * size * 0.5,
    };
    _particlePool.emit(ringPos, color, 1, {
      velocity: [Math.cos(angle) * 3, 0.5 + Math.random(), Math.sin(angle) * 3],
      spread: 0.2,
      lifetime: 1.0,
      size: 1.8,
    });
  }
}

// Camera fly-to particle trail (bd-m9525)
function spawnFlyToTrail(fromPos, toPos) {
  if (!_particlePool) return;
  // Spawn particles along the path between camera start and target
  const steps = 8;
  for (let i = 0; i < steps; i++) {
    const progress = i / steps;
    const pos = {
      x: fromPos.x + (toPos.x - fromPos.x) * progress,
      y: fromPos.y + (toPos.y - fromPos.y) * progress,
      z: fromPos.z + (toPos.z - fromPos.z) * progress,
    };
    // Delayed emission for trail effect
    setTimeout(() => {
      if (!_particlePool) return;
      _particlePool.emit(pos, 0x4a9eff, 3, {
        velocity: [0, 0.5, 0],
        spread: 2,
        lifetime: 1.2,
        size: 1.0,
      });
    }, progress * 400);
  }
}

// --- Selection logic ---

// Unique key for a link (handles both object and string source/target)
function linkKey(l) {
  const s = typeof l.source === 'object' ? l.source.id : l.source;
  const t = typeof l.target === 'object' ? l.target.id : l.target;
  return `${s}->${t}`;
}

// Select a node: highlight it and its entire connected component (beads-1sqr)
function selectNode(node, componentIds) {
  selectedNode = node;
  highlightNodes.clear();
  highlightLinks.clear();

  if (!node) return;

  // If a pre-computed connected component is provided, highlight the full subgraph.
  // Otherwise fall back to direct neighbors only (e.g. for keyboard navigation).
  const targetIds = componentIds || new Set([node.id]);
  if (!componentIds) {
    // Legacy: direct neighbors only
    for (const l of graphData.links) {
      const srcId = typeof l.source === 'object' ? l.source.id : l.source;
      const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      if (srcId === node.id || tgtId === node.id) {
        targetIds.add(srcId);
        targetIds.add(tgtId);
      }
    }
  }

  for (const id of targetIds) highlightNodes.add(id);

  // Highlight all links between nodes in the component
  for (const l of graphData.links) {
    const srcId = typeof l.source === 'object' ? l.source.id : l.source;
    const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
    if (highlightNodes.has(srcId) && highlightNodes.has(tgtId)) {
      highlightLinks.add(linkKey(l));
    }
  }

  // Force link width recalculation
  graph.linkWidth(graph.linkWidth());

  // Materia selection burst VFX (bd-m9525)
  spawnSelectionBurst(node);

  // bd-nnr22: update left sidebar focused issue
  if (typeof updateLeftSidebarFocus === 'function') updateLeftSidebarFocus(node);
}

// Temporarily spread out highlighted subgraph nodes for readability (beads-k38a).
// Uses pairwise repulsion between component nodes (not just radial from centroid)
// so nearby nodes push apart in all directions.  Also applies a gentle centroid
// expansion to prevent the subgraph from collapsing inward.
let _spreadTimeout = null;
function spreadSubgraph(componentIds) {
  // Remove any previous spread force
  if (graph.d3Force('subgraphSpread')) {
    graph.d3Force('subgraphSpread', null);
  }
  if (_spreadTimeout) {
    clearTimeout(_spreadTimeout);
    _spreadTimeout = null;
  }

  if (!componentIds || componentIds.size < 2) return;

  // Collect component nodes for O(n²) pairwise check (n is small — typically < 20)
  const componentNodeList = graphData.nodes.filter((n) => componentIds.has(n.id));
  const count = componentNodeList.length;
  if (count < 2) return;

  // Minimum pairwise distance — labels are ~80-120px wide at typical zoom.
  // At camera distance ~200 (zoomToNodes default for small clusters), 1 world unit ≈ 3-4px.
  // So 40 world units ≈ 120-160px — enough clearance for side-by-side labels.
  const MIN_PAIR_DIST = Math.max(40, count * 5);
  // Radial expansion: push nodes to at least this distance from centroid
  const MIN_RADIAL_DIST = Math.max(30, count * 6);

  graph.d3Force('subgraphSpread', (alpha) => {
    // Phase 1: Pairwise repulsion — push overlapping node pairs apart
    const strength = alpha * 0.15;
    for (let i = 0; i < componentNodeList.length; i++) {
      const a = componentNodeList[i];
      for (let j = i + 1; j < componentNodeList.length; j++) {
        const b = componentNodeList[j];
        const dx = (a.x || 0) - (b.x || 0);
        const dy = (a.y || 0) - (b.y || 0);
        const dz = (a.z || 0) - (b.z || 0);
        const dist = Math.sqrt(dx * dx + dy * dy + dz * dz) || 0.1;
        if (dist < MIN_PAIR_DIST) {
          // Inverse-distance force: stronger when closer
          const push = (strength * (MIN_PAIR_DIST - dist)) / dist;
          // Add small random jitter to break symmetry for overlapping nodes
          const jx = (Math.random() - 0.5) * 0.1;
          const jy = (Math.random() - 0.5) * 0.1;
          a.vx += (dx + jx) * push;
          a.vy += (dy + jy) * push;
          a.vz += dz * push;
          b.vx -= (dx + jx) * push;
          b.vy -= (dy + jy) * push;
          b.vz -= dz * push;
        }
      }
    }

    // Phase 2: Radial expansion from live centroid — prevents collapse
    let cx = 0,
      cy = 0,
      cz = 0;
    for (const n of componentNodeList) {
      cx += n.x || 0;
      cy += n.y || 0;
      cz += n.z || 0;
    }
    cx /= count;
    cy /= count;
    cz /= count;

    const radialStrength = alpha * 0.08;
    for (const n of componentNodeList) {
      const dx = (n.x || 0) - cx;
      const dy = (n.y || 0) - cy;
      const dz = (n.z || 0) - cz;
      const dist = Math.sqrt(dx * dx + dy * dy + dz * dz) || 0.1;
      if (dist < MIN_RADIAL_DIST) {
        const push = (radialStrength * (MIN_RADIAL_DIST - dist)) / dist;
        n.vx += dx * push;
        n.vy += dy * push;
        n.vz += dz * push;
      }
    }
  });

  // Reheat simulation to apply the force
  graph.d3ReheatSimulation();

  // Remove the spread force after layout settles (3s — slightly longer for pairwise)
  _spreadTimeout = setTimeout(() => {
    graph.d3Force('subgraphSpread', null);
    _spreadTimeout = null;
  }, 3000);
}

function clearSelection() {
  selectedNode = null;
  highlightNodes.clear();
  highlightLinks.clear();
  multiSelected.clear();
  // Remove subgraph spread force (beads-k38a)
  if (graph.d3Force('subgraphSpread')) {
    graph.d3Force('subgraphSpread', null);
  }
  // Clear revealed subgraph and re-apply filters (hq-vorf47)
  if (revealedNodes.size > 0) {
    revealedNodes.clear();
    graphData.nodes.forEach((n) => {
      n._revealed = false;
    });
    applyFilters();
  }
  // Clear molecule focus state (bd-lwut6)
  focusedMoleculeNodes.clear();
  hideBulkMenu();
  unfreezeCamera(); // bd-casin: restore orbit controls
  restoreAllNodeOpacity();
  updateBeadURL(null); // bd-he95o: clear URL deep-link on deselect
  // Force link width recalculation
  graph.linkWidth(graph.linkWidth());
  // bd-nnr22: clear left sidebar focused issue
  if (typeof updateLeftSidebarFocus === 'function') updateLeftSidebarFocus(null);
}

// --- URL deep-linking (bd-he95o) ---
// Focus on a bead specified via ?bead=<id> URL parameter.
// Selects the node, highlights its connected subgraph, flies camera to it,
// and opens the detail panel.
function focusDeepLinkBead(beadId) {
  const node = graphData.nodes.find((n) => n.id === beadId);
  if (!node) {
    console.warn(`[beads3d] Deep-link bead "${beadId}" not found in graph`);
    return;
  }

  // Select and highlight the full connected component (beads-1sqr)
  const component = getConnectedComponent(node.id);
  selectNode(node, component);

  // Reveal the subgraph, spread for readability, and zoom to it
  revealedNodes.clear();
  for (const id of component) revealedNodes.add(id);
  applyFilters();

  spreadSubgraph(component); // beads-k38a: push nodes apart for readability
  zoomToNodes(component);

  // Show detail panel after camera starts moving
  setTimeout(() => showDetail(node), 500);

  // Update URL hash for shareability without triggering reload
  if (window.history.replaceState) {
    const url = new URL(window.location.href);
    url.searchParams.set('bead', beadId);
    window.history.replaceState(null, '', url.toString());
  }

  console.log(`[beads3d] Deep-linked to bead: ${beadId}`);
}

// --- Molecule focus view (bd-lwut6) ---
// Brings a molecule's connected subgraph into view with all labels readable.
// Triggered via ?molecule=<id> URL parameter or programmatically.
function focusMolecule(moleculeId) {
  const node = graphData.nodes.find((n) => n.id === moleculeId);
  if (!node) {
    console.warn(`[beads3d] Molecule "${moleculeId}" not found in graph`);
    return;
  }

  // Find the full connected component
  const component = getConnectedComponent(node.id);

  // Store focused molecule nodes for label LOD override
  focusedMoleculeNodes = new Set(component);

  // Select and highlight the subgraph
  selectNode(node, component);

  // Reveal all nodes in the component (override filters)
  revealedNodes.clear();
  for (const id of component) revealedNodes.add(id);
  applyFilters();

  // Enable labels if not already visible
  if (!labelsVisible) {
    labelsVisible = true;
    const btn = document.getElementById('btn-labels');
    if (btn) btn.classList.toggle('active', true);
  }

  // Spread nodes apart for readability, then zoom to fit
  spreadSubgraph(component);
  zoomToNodes(component);

  // Show detail panel for the molecule node
  setTimeout(() => showDetail(node), 500);

  // Update URL for shareability
  if (window.history.replaceState) {
    const url = new URL(window.location.href);
    url.searchParams.set('molecule', moleculeId);
    url.searchParams.delete('bead');
    window.history.replaceState(null, '', url.toString());
  }

  console.log(`[beads3d] Molecule focus: ${moleculeId} (${component.size} nodes)`);
}

// centerCameraOnSelection, unfreezeCamera moved to camera.js (bd-7t6nt)

// --- Node opacity helpers ---

// Save the original opacity on a material (first time only)
function saveBaseOpacity(mat) {
  if (mat.uniforms && mat.uniforms.opacity) {
    if (mat._baseUniformOpacity === undefined) mat._baseUniformOpacity = mat.uniforms.opacity.value;
  } else if (!mat.uniforms) {
    if (mat._baseOpacity === undefined) mat._baseOpacity = mat.opacity;
  }
}

// Set material opacity to base * factor
function setMaterialDim(mat, factor) {
  if (mat.uniforms && mat.uniforms.opacity) {
    mat.uniforms.opacity.value = (mat._baseUniformOpacity ?? 0.4) * factor;
  } else if (!mat.uniforms) {
    mat.opacity = (mat._baseOpacity ?? mat.opacity) * factor;
  }
}

// Restore material to saved base opacity
function restoreMaterialOpacity(mat) {
  if (mat.uniforms && mat.uniforms.opacity && mat._baseUniformOpacity !== undefined) {
    mat.uniforms.opacity.value = mat._baseUniformOpacity;
  } else if (!mat.uniforms && mat._baseOpacity !== undefined) {
    mat.opacity = mat._baseOpacity;
  }
}

// Restore all nodes that were dimmed during selection
function restoreAllNodeOpacity() {
  for (const node of graphData.nodes) {
    const threeObj = node.__threeObj;
    if (!threeObj) continue;

    // Revert selection-shown labels (bd-xk0tx): LOD pass will re-apply budget (beads-bu3r)
    threeObj.traverse((child) => {
      if (child.userData.nodeLabel) {
        child.visible = false; // LOD pass (resolveOverlappingLabels) re-shows the right ones
      }
    });

    if (!node._wasDimmed) continue;
    threeObj.traverse((child) => {
      if (
        !child.material ||
        child.userData.selectionRing ||
        child.userData.materiaCore ||
        child.userData.decisionPulse ||
        child.userData.nodeLabel
      )
        return;
      restoreMaterialOpacity(child.material);
    });
    node._wasDimmed = false;
  }
}

// --- Animation loop: pulsing effects + selection dimming ---

function startAnimation() {
  function animate() {
    requestAnimationFrame(animate);
    const t = (performance.now() - startTime) / 1000;
    const hasSelection = selectedNode !== null;

    // Update all shader uniforms (star field twinkle, Fresnel, fairy lights drift, selection ring sweep)
    updateShaderTime(graph.scene(), t);

    // Per-node visual feedback — only iterate when needed
    for (const node of graphData.nodes) {
      const threeObj = node.__threeObj;
      if (!threeObj) continue;

      const isMultiSelected = multiSelected.has(node.id);
      const isHighlighted = !hasSelection || highlightNodes.has(node.id) || isMultiSelected;
      const isSelected = (hasSelection && node.id === selectedNode.id) || isMultiSelected;
      const dimFactor = isHighlighted ? 1.0 : 0.35;

      // Skip traversal when nothing to update (agents always animate — beads-v0wa)
      if (
        !hasSelection &&
        !isMultiSelected &&
        node.status !== 'in_progress' &&
        node.issue_type !== 'agent' &&
        !labelsVisible
      )
        continue;

      // Track dimmed nodes for restoration in clearSelection()
      if (hasSelection && !isHighlighted) node._wasDimmed = true;

      threeObj.traverse((child) => {
        if (!child.material) return;

        // Label sprites: show on highlighted nodes when selected (bd-xk0tx)
        // When no selection, LOD pass (resolveOverlappingLabels) manages visibility (beads-bu3r)
        // bd-q9d6h: only update if state actually changed, to reduce blink
        if (child.userData.nodeLabel) {
          if (hasSelection) {
            const shouldShow = isHighlighted;
            if (child.visible !== shouldShow) child.visible = shouldShow;
          }
          // else: LOD pass handles visibility via label budget
          return;
        }

        if (child.userData.materiaCore) {
          // Materia selection boost — glow intensification (bd-c7d5z, bd-lzojw: safe with cached materials)
          // Cached materials are shared, so we clone on select and restore on deselect.
          if (child.material.uniforms && child.material.uniforms.selected) {
            if (isSelected && child.material.uniforms.selected.value !== 1.0) {
              child.userData._cachedMaterial = child.material;
              child.material = child.material.clone();
              child.material.uniforms.selected.value = 1.0;
            } else if (!isSelected && child.userData._cachedMaterial) {
              child.material = child.userData._cachedMaterial;
              delete child.userData._cachedMaterial;
            }
          }
        } else if (child.userData.selectionRing) {
          // Legacy selection ring (kept for backward compat)
          if (child.material.uniforms && child.material.uniforms.visible) {
            child.material.uniforms.visible.value = isSelected ? 1.0 : 0.0;
          }
        } else if (child.userData.agentGlow) {
          // Agent glow: breathing pulse (beads-v0wa)
          const base = child.userData.baseScale || 1;
          const pulse = 1.0 + Math.sin(t * 2) * 0.08;
          child.scale.setScalar(base * pulse);
          if (child.material.uniforms && child.material.uniforms.opacity) {
            child.material.uniforms.opacity.value = (0.1 + Math.sin(t * 2.5) * 0.05) * dimFactor;
          }
        } else if (child.userData.agentTrail) {
          // Wake trail: orient behind movement direction, fade based on speed (beads-v0wa)
          const prev = child.userData.prevPos;
          const dx = (node.x || 0) - prev.x;
          const dy = (node.y || 0) - prev.y;
          const dz = (node.z || 0) - prev.z;
          const speed = Math.sqrt(dx * dx + dy * dy + dz * dz);
          // Smoothly update previous position
          prev.x += dx * 0.1;
          prev.y += dy * 0.1;
          prev.z += dz * 0.1;
          // Trail visible only when moving (speed > threshold)
          const trailOpacity = Math.min(speed * 0.15, 0.35) * dimFactor;
          child.material.opacity = trailOpacity;
          // Position trail behind the agent (opposite of travel direction)
          if (speed > 0.5) {
            const nx = -dx / speed,
              ny = -dy / speed,
              nz = -dz / speed;
            child.position.set(nx * 6, ny * 6, nz * 6);
          }
        } else if (child.userData.decisionPulse) {
          // Decision pending pulse: breathe scale + rotate (bd-zr374)
          const pulse = 1.0 + Math.sin(t * 2.5) * 0.15;
          child.scale.set(child.scale.x, child.scale.y, child.scale.z);
          child.scale.multiplyScalar(pulse);
          child.rotation.y = t * 0.3;
          child.material.opacity = (0.2 + Math.sin(t * 2.5) * 0.1) * dimFactor;
        } else if (child.userData.jackPulse) {
          // Active jack: slow breathing pulse + gentle rotation (bd-hffzf)
          const pulse = 1.0 + Math.sin(t * 1.8) * 0.12;
          child.scale.multiplyScalar(pulse);
          child.rotation.y = t * 0.4;
          child.material.opacity = (0.15 + Math.sin(t * 1.8) * 0.08) * dimFactor;
        } else if (child.userData.jackExpiredFlash) {
          // Expired jack: rapid red flash — urgent attention (bd-hffzf)
          const flash = Math.abs(Math.sin(t * 4.0));
          child.material.opacity = (0.1 + flash * 0.35) * dimFactor;
          child.rotation.y = t * 0.6;
        } else if (hasSelection) {
          saveBaseOpacity(child.material);
          setMaterialDim(child.material, dimFactor);
        }
      });
    }

    // Quake-style smooth camera movement — delegated to camera.js (bd-zab4q, bd-7t6nt)
    updateCameraMovement();

    // Update live event doots — HTML overlay via CSS2DRenderer (bd-bwkdk)
    updateDoots(t);
    // Only render CSS2D overlay when doots are active — avoids DOM projection overhead (bd-kkd9y)
    if (css2dRenderer && doots.length > 0) css2dRenderer.render(graph.scene(), graph.camera());

    // Update event sprites — status pulses + edge sparks (bd-9qeto)
    updateEventSprites(t);
    updatePerfOverlay(t);

    // Adaptive quality scaling — feed FPS to auto-tune particle budget (bd-dnuky)
    {
      const now = performance.now();
      if (!animate._lastAdaptiveTime) animate._lastAdaptiveTime = now;
      const dt = now - animate._lastAdaptiveTime;
      animate._lastAdaptiveTime = now;
      if (dt > 0) adaptiveQualityTick(1000 / dt);
    }

    // Camera shake for shockwave effects (bd-3fnon)
    const shake = getCameraShake();
    if (shake) {
      const elapsed = t - shake.startTime;
      if (elapsed > shake.duration) {
        graph.camera().position.copy(shake.origPos);
        clearCameraShake();
      } else {
        const dampen = 1 - elapsed / shake.duration;
        const jitter = shake.intensity * dampen;
        graph
          .camera()
          .position.set(
            shake.origPos.x + (Math.random() - 0.5) * jitter * 2,
            shake.origPos.y + (Math.random() - 0.5) * jitter * 2,
            shake.origPos.z + (Math.random() - 0.5) * jitter * 2,
          );
      }
    }

    // GPU particle pool update + selection VFX (bd-m9525)
    if (_particlePool) {
      _particlePool.update(t);
      updateSelectionVFX(t);
      updateInProgressAura(t);
    }

    // Label anti-overlap: run every 4th frame for perf (beads-rgmh)
    if (!animate._labelFrame) animate._labelFrame = 0;
    if (++animate._labelFrame % 4 === 0) resolveOverlappingLabels();

    // Minimap: render every 3rd frame for perf
    if (!animate._frame) animate._frame = 0;
    if (++animate._frame % 3 === 0) renderMinimap();
  }
  animate();
}

// --- Data fetching ---
async function fetchGraphData() {
  const statusEl = document.getElementById('status');
  try {
    // Try Graph API first (single optimized endpoint)
    const hasGraph = await api.hasGraph();
    if (hasGraph) {
      return await fetchViaGraph(statusEl);
    }
    // Fallback: combine List endpoints
    return await fetchViaList(statusEl);
  } catch (err) {
    statusEl.textContent = `error: ${err.message}`;
    statusEl.className = 'error';
    console.error('Fetch failed:', err);
    return null;
  }
}

async function fetchViaGraph(statusEl) {
  // bd-a0vbd: default to active statuses only — no closed beads.
  // Closed beads add noise and bridge separate clusters into one hairball.
  const graphArgs = {
    limit: MAX_NODES,
    status: ['open', 'in_progress', 'blocked', 'hooked', 'deferred'], // bd-a0vbd: no closed by default
    include_deps: true,
    include_body: true,
    include_agents: true,
    exclude_types: DEEP_LINK_MOLECULE
      ? ['message', 'config', 'gate', 'wisp', 'convoy', 'formula', 'advice', 'role'] // bd-lwut6: include molecules when focusing one
      : ['message', 'config', 'gate', 'wisp', 'convoy', 'molecule', 'formula', 'advice', 'role'], // bd-04wet, bd-t25i1, bd-uqkpq: filter noise types
  };
  const result = await api.graph(graphArgs);

  const now = Date.now();
  let nodes = (result.nodes || []).map((n) => ({
    id: n.id,
    ...n,
    _blocked: !!(n.blocked_by && n.blocked_by.length > 0),
    _jackExpired: n.issue_type === 'jack' && n.jack_expires_at && new Date(n.jack_expires_at).getTime() < now,
  }));

  // Graph API edges: { source, target, type } → links: { source, target, dep_type }
  // Promote "blocks" edges where target is an epic to "parent-child" so they render
  // with the chain glyph instead of the shield. Most epics use "blocks" deps rather
  // than explicit "parent-child" deps. (bd-uqkpq)
  const nodeIds = new Set(nodes.map((n) => n.id));
  const nodeMap = Object.fromEntries(nodes.map((n) => [n.id, n]));
  const links = [];
  for (const e of result.edges || []) {
    const hasSrc = nodeIds.has(e.source);
    const hasTgt = nodeIds.has(e.target);
    let depType = e.type || 'blocks';

    // Promote blocks edges to parent-child when target is an epic (bd-uqkpq)
    if (depType === 'blocks' && hasTgt && nodeMap[e.target]?.issue_type === 'epic') {
      depType = 'parent-child';
    }

    if (hasSrc && hasTgt) {
      links.push({ source: e.source, target: e.target, dep_type: depType });
    } else if (depType === 'parent-child' && (hasSrc || hasTgt)) {
      // Create ghost node for the missing endpoint so DAG links remain visible
      const missingId = hasSrc ? e.target : e.source;
      if (!nodeIds.has(missingId)) {
        nodes.push({
          id: missingId,
          title: missingId,
          status: 'open',
          priority: 3,
          issue_type: 'epic',
          _placeholder: true,
          _blocked: false,
        });
        nodeIds.add(missingId);
        nodeMap[missingId] = nodes[nodes.length - 1];
      }
      links.push({ source: e.source, target: e.target, dep_type: 'parent-child' });
    }
  }

  // Client-side agent→bead linkage (beads-zq8a): synthesize agent nodes and
  // assigned_to links from node assignee fields. The server Graph API should
  // return these, but currently doesn't — so we build them client-side.
  const agentNodes = new Map(); // assignee name → agent node
  for (const n of nodes) {
    if (n.assignee && n.status === 'in_progress') {
      const agentId = `agent:${n.assignee}`;
      if (!agentNodes.has(n.assignee)) {
        agentNodes.set(n.assignee, {
          id: agentId,
          title: n.assignee,
          status: 'active',
          priority: 3,
          issue_type: 'agent',
          _blocked: false,
        });
      }
      // Only add edge if not already present from server
      const edgeExists = links.some(
        (l) =>
          (l.source === agentId || l.source?.id === agentId) &&
          (l.target === n.id || l.target?.id === n.id) &&
          l.dep_type === 'assigned_to',
      );
      if (!edgeExists) {
        links.push({ source: agentId, target: n.id, dep_type: 'assigned_to' });
      }
    }
  }
  for (const [, agentNode] of agentNodes) {
    if (!nodeIds.has(agentNode.id)) {
      nodes.push(agentNode);
      nodeIds.add(agentNode.id);
      nodeMap[agentNode.id] = agentNode;
    }
  }

  // Filter disconnected decisions and molecules: only show if they have at least
  // one edge connecting them to a visible bead (bd-t25i1)
  const LINKED_ONLY = new Set(['decision', 'gate', 'molecule']); // bd-zbyn7: include gate (decisions are type=gate)
  const connectedIds = new Set();
  for (const l of links) {
    connectedIds.add(l.source);
    connectedIds.add(l.target);
  }
  nodes = nodes.filter((n) => !LINKED_ONLY.has(n.issue_type) || connectedIds.has(n.id));

  // Max-edges-per-node cap: prune low-priority edges from hub nodes (bd-ke2xc)
  if (maxEdgesPerNode > 0) {
    const DEP_RANK = { blocks: 0, 'parent-child': 1, 'waits-for': 2, 'relates-to': 3, assigned_to: 4, rig_conflict: 5 };
    // Count edges per node
    const edgeCounts = new Map(); // nodeId → [link, ...]
    for (const l of links) {
      const src = typeof l.source === 'object' ? l.source.id : l.source;
      const tgt = typeof l.target === 'object' ? l.target.id : l.target;
      if (!edgeCounts.has(src)) edgeCounts.set(src, []);
      if (!edgeCounts.has(tgt)) edgeCounts.set(tgt, []);
      edgeCounts.get(src).push(l);
      edgeCounts.get(tgt).push(l);
    }
    // Find over-connected nodes and mark edges for removal
    const removedLinks = new Set();
    for (const [, nodeLinks] of edgeCounts) {
      if (nodeLinks.length <= maxEdgesPerNode) continue;
      // Sort by priority (keep high-priority edges)
      nodeLinks.sort((a, b) => (DEP_RANK[a.dep_type] ?? 3) - (DEP_RANK[b.dep_type] ?? 3));
      for (let i = maxEdgesPerNode; i < nodeLinks.length; i++) {
        removedLinks.add(nodeLinks[i]);
      }
    }
    if (removedLinks.size > 0) {
      const before = links.length;
      links.splice(0, links.length, ...links.filter((l) => !removedLinks.has(l)));
      console.log(`[beads3d] Edge cap: pruned ${before - links.length} edges (max ${maxEdgesPerNode}/node)`);
    }
  }

  statusEl.textContent = `graph api · ${nodes.length} beads · ${links.length} links`;
  statusEl.className = 'connected';
  updateStats(result.stats, nodes);
  console.log(`[beads3d] Graph API: ${nodes.length} nodes, ${links.length} links`);
  return { nodes, links };
}

async function fetchViaList(statusEl) {
  const SKIP_TYPES = new Set(
    DEEP_LINK_MOLECULE
      ? ['message', 'config', 'wisp', 'convoy', 'formula', 'advice', 'role'] // bd-lwut6: include molecules; bd-zbyn7: include gate/decision
      : ['message', 'config', 'wisp', 'convoy', 'molecule', 'formula', 'advice', 'role'],
  );

  // Parallel fetch: open/active beads + blocked + stats (bd-7haep: include all active statuses)
  const [openIssues, inProgress, hookedIssues, deferredIssues, blocked, stats] = await Promise.all([
    api.list({
      limit: MAX_NODES * 2, // over-fetch to compensate for client-side filtering
      status: 'open',
      exclude_status: ['tombstone', 'closed'],
    }),
    api.list({
      limit: 100,
      status: 'in_progress',
    }),
    api.list({ limit: 100, status: 'hooked' }).catch(() => []),
    api.list({ limit: 100, status: 'deferred' }).catch(() => []),
    api.blocked().catch(() => []),
    api.stats().catch(() => null),
  ]);

  // Merge all issues, dedup by id, filter out noise
  const issueMap = new Map();
  const addIssues = (arr) => {
    if (!Array.isArray(arr)) return;
    for (const i of arr) {
      if (i.ephemeral) continue;
      if (SKIP_TYPES.has(i.issue_type)) continue;
      if (i.id.includes('-wisp-')) continue;
      issueMap.set(i.id, i);
    }
  };

  addIssues(openIssues);
  addIssues(inProgress);
  addIssues(hookedIssues);
  addIssues(deferredIssues);
  addIssues(blocked);

  const issues = [...issueMap.values()].slice(0, MAX_NODES);

  statusEl.textContent = `list api · ${issues.length} beads`;
  statusEl.className = 'connected';
  updateStats(stats, issues);
  return buildGraphData(issues);
}

function buildGraphData(issues) {
  const issueMap = new Map();
  issues.forEach((i) => issueMap.set(i.id, i));

  const buildNow = Date.now();
  const nodes = issues.map((issue) => ({
    id: issue.id,
    ...issue,
    _blocked: !!(issue.blocked_by && issue.blocked_by.length > 0),
    _jackExpired: issue.issue_type === 'jack' && issue.jack_expires_at && new Date(issue.jack_expires_at).getTime() < buildNow,
  }));

  const links = [];
  const seenLinks = new Set();

  issues.forEach((issue) => {
    // blocked_by → create links
    if (issue.blocked_by && Array.isArray(issue.blocked_by)) {
      for (const blockerId of issue.blocked_by) {
        const key = `${issue.id}<-${blockerId}`;
        if (seenLinks.has(key)) continue;
        seenLinks.add(key);

        if (!issueMap.has(blockerId)) {
          const placeholder = {
            id: blockerId,
            title: blockerId,
            status: 'open',
            priority: 3,
            issue_type: 'task',
            _placeholder: true,
          };
          issueMap.set(blockerId, placeholder);
          nodes.push({ ...placeholder, _blocked: false });
        }

        links.push({ source: blockerId, target: issue.id, dep_type: 'blocks' });
      }
    }

    // Dependencies
    if (issue.dependencies && Array.isArray(issue.dependencies)) {
      for (const dep of issue.dependencies) {
        const fromId = dep.issue_id || issue.id;
        const toId = dep.depends_on_id || dep.id;
        if (!toId) continue;
        const key = `${fromId}->${toId}`;
        if (seenLinks.has(key)) continue;
        seenLinks.add(key);

        if (!issueMap.has(toId)) {
          const placeholder = {
            id: toId,
            title: dep.title || toId,
            status: dep.status || 'open',
            priority: dep.priority ?? 3,
            issue_type: dep.issue_type || 'task',
            _placeholder: true,
          };
          issueMap.set(toId, placeholder);
          nodes.push({ ...placeholder, _blocked: false });
        }

        links.push({
          source: fromId,
          target: toId,
          dep_type: dep.type || dep.dependency_type || 'blocks',
        });
      }
    }

    // Parent-child — create placeholder if parent not loaded (bd-uqkpq)
    if (issue.parent) {
      const key = `${issue.id}->parent:${issue.parent}`;
      if (!seenLinks.has(key)) {
        seenLinks.add(key);
        if (!issueMap.has(issue.parent)) {
          const placeholder = {
            id: issue.parent,
            title: issue.parent,
            status: 'open',
            priority: 3,
            issue_type: 'epic',
            _placeholder: true,
          };
          issueMap.set(issue.parent, placeholder);
          nodes.push({ ...placeholder, _blocked: false });
        }
        links.push({ source: issue.id, target: issue.parent, dep_type: 'parent-child' });
      }
    }
  });

  // Client-side agent→bead linkage (beads-zq8a): same as fetchViaGraph path
  const agentMap = new Map();
  for (const n of nodes) {
    const assignee = n.assigned_to || n.assignee;
    if (assignee && n.status === 'in_progress') {
      const agentId = `agent:${assignee}`;
      if (!agentMap.has(assignee)) {
        agentMap.set(assignee, {
          id: agentId,
          title: assignee,
          status: 'active',
          priority: 3,
          issue_type: 'agent',
          _blocked: false,
        });
      }
      links.push({ source: agentId, target: n.id, dep_type: 'assigned_to' });
    }
  }
  for (const [, agentNode] of agentMap) {
    if (!issueMap.has(agentNode.id)) {
      nodes.push(agentNode);
      issueMap.set(agentNode.id, agentNode);
    }
  }

  console.log(`[beads3d] ${nodes.length} nodes, ${links.length} links`);
  return { nodes, links };
}

// bd-9cpbc.1: live-update project pulse from bus mutation events
function _liveUpdateProjectPulse() {
  if (!graphData) return;
  const pulseEl = document.getElementById('hud-project-pulse');
  if (!pulseEl) return;
  const nodes = graphData.nodes.filter((n) => !n._hidden);
  let open = 0,
    active = 0,
    blocked = 0,
    agentCount = 0,
    pendingDecisions = 0;
  for (const n of nodes) {
    if (n.issue_type === 'agent') {
      agentCount++;
      continue;
    }
    if (
      (n.issue_type === 'decision' || (n.issue_type === 'gate' && n.await_type === 'decision')) &&
      n.status !== 'closed'
    )
      pendingDecisions++;
    if (n._blocked) blocked++;
    else if (n.status === 'in_progress') active++;
    else if (n.status === 'open' || n.status === 'hooked' || n.status === 'deferred') open++;
  }
  pulseEl.innerHTML = `
    <div class="pulse-stat"><span class="pulse-stat-label">open</span><span class="pulse-stat-value">${open}</span></div>
    <div class="pulse-stat"><span class="pulse-stat-label">active</span><span class="pulse-stat-value good">${active}</span></div>
    <div class="pulse-stat"><span class="pulse-stat-label">blocked</span><span class="pulse-stat-value${blocked ? ' bad' : ''}">${blocked}</span></div>
    <div class="pulse-stat"><span class="pulse-stat-label">agents</span><span class="pulse-stat-value${agentCount ? ' warn' : ''}">${agentCount}</span></div>
    <div class="pulse-stat"><span class="pulse-stat-label">decisions</span><span class="pulse-stat-value${pendingDecisions ? ' warn' : ''}">${pendingDecisions}</span></div>
    <div class="pulse-stat"><span class="pulse-stat-label">shown</span><span class="pulse-stat-value">${nodes.length}</span></div>
  `;
}

function updateStats(stats, issues) {
  const el = document.getElementById('stats');
  const parts = [];
  if (stats) {
    // Handle both Graph API (total_open) and Stats API (open_issues) formats
    const open = stats.total_open ?? stats.open_issues ?? 0;
    const active = stats.total_in_progress ?? stats.in_progress_issues ?? 0;
    const blocked = stats.total_blocked ?? stats.blocked_issues ?? 0;
    parts.push(`<span>${open}</span> open`);
    parts.push(`<span>${active}</span> active`);
    if (blocked) parts.push(`<span>${blocked}</span> blocked`);

    // Update Bottom HUD project pulse (bd-ddj44, bd-9ndk0.1)
    const pulseEl = document.getElementById('hud-project-pulse');
    if (pulseEl) {
      const agentCount = issues.filter((n) => n.issue_type === 'agent').length;
      const pendingDecisions = issues.filter(
        (n) =>
          (n.issue_type === 'decision' || (n.issue_type === 'gate' && n.await_type === 'decision')) &&
          n.status !== 'closed',
      ).length;
      pulseEl.innerHTML = `
        <div class="pulse-stat"><span class="pulse-stat-label">open</span><span class="pulse-stat-value">${open}</span></div>
        <div class="pulse-stat"><span class="pulse-stat-label">active</span><span class="pulse-stat-value good">${active}</span></div>
        <div class="pulse-stat"><span class="pulse-stat-label">blocked</span><span class="pulse-stat-value${blocked ? ' bad' : ''}">${blocked}</span></div>
        <div class="pulse-stat"><span class="pulse-stat-label">agents</span><span class="pulse-stat-value${agentCount ? ' warn' : ''}">${agentCount}</span></div>
        <div class="pulse-stat"><span class="pulse-stat-label">decisions</span><span class="pulse-stat-value${pendingDecisions ? ' warn' : ''}">${pendingDecisions}</span></div>
        <div class="pulse-stat"><span class="pulse-stat-label">shown</span><span class="pulse-stat-value">${issues.length}</span></div>
      `;
    }
  }
  parts.push(`<span>${issues.length}</span> shown`);
  el.innerHTML = parts.join(' &middot; ');
}

// --- Tooltip ---
const tooltip = document.getElementById('tooltip');

function handleNodeHover(node) {
  document.body.style.cursor = node ? 'pointer' : 'default';
  // Track hovered node for VFX glow warmup (bd-m9525)
  _hoveredNode = node && !node._hidden ? node : null;
  _hoverGlowTimer = 0;
  if (!node || node._hidden) {
    hideTooltip();
    return;
  }

  const pLabel = ['P0 CRIT', 'P1', 'P2', 'P3', 'P4'][node.priority] || '';
  const assignee = node.assignee ? `<br>assignee: ${escapeHtml(node.assignee)}` : '';

  tooltip.innerHTML = `
    <div class="id">${escapeHtml(node.id)} &middot; ${node.issue_type || 'task'} &middot; ${pLabel}</div>
    <div class="title">${escapeHtml(node.title || node.id)}</div>
    <div class="meta">
      ${node.status}${node._blocked ? ' &middot; BLOCKED' : ''}${node._jackExpired ? ' &middot; EXPIRED' : ''}${node._placeholder ? ' &middot; (ref)' : ''}
      ${assignee}
      ${node.blocked_by ? '<br>blocked by: ' + node.blocked_by.map(escapeHtml).join(', ') : ''}
    </div>
    <div class="hint">click for details</div>
  `;
  // Respect HUD visibility toggle (bd-4hggh)
  if (window.__beads3d_hudHidden && window.__beads3d_hudHidden['tooltip']) return;
  tooltip.style.display = 'block';
  document.addEventListener('mousemove', positionTooltip);
}

function positionTooltip(e) {
  const pad = 15;
  let x = e.clientX + pad;
  let y = e.clientY + pad;
  // Keep tooltip on screen
  const rect = tooltip.getBoundingClientRect();
  if (x + rect.width > window.innerWidth) x = e.clientX - rect.width - pad;
  if (y + rect.height > window.innerHeight) y = e.clientY - rect.height - pad;
  tooltip.style.left = x + 'px';
  tooltip.style.top = y + 'px';
}

function hideTooltip() {
  tooltip.style.display = 'none';
  document.removeEventListener('mousemove', positionTooltip);
}

// --- Detail panel (click to open) ---
function handleNodeClick(node) {
  if (!node) return;
  // Allow clicking revealed nodes even when they'd normally be hidden (hq-vorf47)
  if (node._hidden && !revealedNodes.has(node.id)) return;

  // Epic collapse/expand: toggle children visibility on epic click (kd-XGgiokgQBH)
  if (node.issue_type === 'epic') {
    if (collapsedEpics.has(node.id)) {
      collapsedEpics.delete(node.id);
      node._epicCollapsed = false;
    } else {
      collapsedEpics.add(node.id);
      node._epicCollapsed = true;
    }
    applyFilters();
    // Regenerate node objects to update epic visual state (collapsed indicator)
    if (graph) graph.nodeThreeObject(graph.nodeThreeObject());
  }

  // Compute full connected component first, then highlight it all (beads-1sqr)
  const component = getConnectedComponent(node.id);
  selectNode(node, component);

  // Reveal entire connected subgraph regardless of filters (hq-vorf47).
  revealedNodes.clear();
  for (const id of component) {
    revealedNodes.add(id);
  }
  applyFilters(); // re-run filters to un-hide revealed nodes

  spreadSubgraph(component); // beads-k38a: push nodes apart for readability
  zoomToNodes(component);

  // Route decision/gate nodes through lightbox instead of detail panel (beads-zuc3)
  if (node.issue_type === 'decision' || (node.issue_type === 'gate' && node.await_type === 'decision')) {
    showDecisionLightbox(node.id);
  } else {
    showDetail(node);
  }

  // Update URL for deep-linking (bd-he95o) — enables copy/paste sharing
  updateBeadURL(node.id);
}

// Update URL ?bead= parameter without page reload (bd-he95o)
function updateBeadURL(beadId) {
  if (!window.history.replaceState) return;
  const url = new URL(window.location.href);
  if (beadId) {
    url.searchParams.set('bead', beadId);
  } else {
    url.searchParams.delete('bead');
    url.searchParams.delete('molecule'); // bd-lwut6: clear molecule param on deselect
  }
  window.history.replaceState(null, '', url.toString());
}

// BFS to find the full connected component (both directions) for a node (bd-tr0en)
function getConnectedComponent(startId) {
  const visited = new Set();
  const queue = [startId];
  while (queue.length > 0) {
    const current = queue.shift();
    if (visited.has(current)) continue;
    visited.add(current);
    for (const l of graphData.links) {
      const srcId = typeof l.source === 'object' ? l.source.id : l.source;
      const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      if (srcId === current && !visited.has(tgtId)) queue.push(tgtId);
      if (tgtId === current && !visited.has(srcId)) queue.push(srcId);
    }
  }
  return visited;
}

// Fly camera to fit a set of node IDs with padding (bd-tr0en)
function zoomToNodes(nodeIds) {
  let cx = 0,
    cy = 0,
    cz = 0,
    count = 0;
  for (const node of graphData.nodes) {
    if (!nodeIds.has(node.id)) continue;
    cx += node.x || 0;
    cy += node.y || 0;
    cz += node.z || 0;
    count++;
  }
  if (count === 0) return;
  cx /= count;
  cy /= count;
  cz /= count;

  // For single-node components, use the original close-up zoom
  if (count === 1) {
    const distance = 150;
    const distRatio = 1 + distance / Math.hypot(cx, cy, cz);
    const camFrom = graph.camera().position.clone();
    const camTo = { x: cx * distRatio, y: cy * distRatio, z: cz * distRatio };
    spawnFlyToTrail(camFrom, { x: cx, y: cy, z: cz }); // bd-m9525: particle trail
    graph.cameraPosition(camTo, { x: cx, y: cy, z: cz }, 1000);
    return;
  }

  // Calculate radius (max distance from center)
  let maxDist = 0;
  for (const node of graphData.nodes) {
    if (!nodeIds.has(node.id)) continue;
    const dx = (node.x || 0) - cx;
    const dy = (node.y || 0) - cy;
    const dz = (node.z || 0) - cz;
    const d = Math.sqrt(dx * dx + dy * dy + dz * dz);
    if (d > maxDist) maxDist = d;
  }

  const distance = Math.max(maxDist * 2.5, 150);
  const lookAt = { x: cx, y: cy, z: cz };
  const cam = graph.camera();
  const dir = new THREE.Vector3(cam.position.x - cx, cam.position.y - cy, cam.position.z - cz).normalize();
  const camPos = {
    x: cx + dir.x * distance,
    y: cy + dir.y * distance,
    z: cz + dir.z * distance,
  };
  spawnFlyToTrail(graph.camera().position.clone(), lookAt); // bd-m9525: particle trail
  graph.cameraPosition(camPos, lookAt, 1000);
}

// showDetail, closeDetailPanel, hideDetail, renderFullDetail, renderDecisionDetail,
// bindDecisionHandlers, repositionPanels moved to detail-panel.js (bd-7t6nt)

function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

// Context menu (right-click) moved to context-menu.js (bd-7t6nt)
// handleNodeRightClick, hideContextMenu, showStatusToast, buildStatusSubmenu,
// buildPrioritySubmenu, optimisticUpdate, handleContextAction imported from context-menu.js

// Floating toast notification for decision events (bd-tausm)
function showDecisionToast(evt) {
  const p = evt.payload || {};
  const type = evt.type;
  let text, cls;
  if (type === 'DecisionCreated') {
    const agent = p.requested_by || 'agent';
    const q = (p.question || 'decision').slice(0, 60);
    text = `? ${agent}: ${q}`;
    cls = p.urgency === 'high' ? 'decision-toast urgent' : 'decision-toast';
  } else if (type === 'DecisionResponded') {
    text = `✓ Decided: ${(p.chosen_label || 'resolved').slice(0, 40)}`;
    cls = 'decision-toast resolved';
  } else {
    return; // Only toast for created and responded
  }

  const toast = document.createElement('div');
  toast.className = cls;
  toast.textContent = text;
  // Click to focus the decision node
  if (p.decision_id) {
    toast.style.cursor = 'pointer';
    toast.addEventListener('click', () => {
      toast.remove();
      if (graphData) {
        const decNode = graphData.nodes.find((n) => n.id === p.decision_id);
        if (decNode) handleNodeClick(decNode);
      }
    });
  }
  document.body.appendChild(toast);
  requestAnimationFrame(() => toast.classList.add('show'));
  setTimeout(() => {
    toast.classList.remove('show');
    setTimeout(() => toast.remove(), 300);
  }, 8000);
}

// Walk the dependency graph in one direction to build a subgraph highlight
function highlightSubgraph(node, direction) {
  selectedNode = node;
  highlightNodes.clear();
  highlightLinks.clear();

  const visited = new Set();
  const queue = [node.id];

  while (queue.length > 0) {
    const current = queue.shift();
    if (visited.has(current)) continue;
    visited.add(current);
    highlightNodes.add(current);

    for (const l of graphData.links) {
      const srcId = typeof l.source === 'object' ? l.source.id : l.source;
      const tgtId = typeof l.target === 'object' ? l.target.id : l.target;

      if (direction === 'downstream') {
        // Dependencies: this node depends on (source=node → target=dep)
        if (srcId === current) {
          highlightLinks.add(linkKey(l));
          if (!visited.has(tgtId)) queue.push(tgtId);
        }
      } else {
        // Blockers: what blocks this node (target=node ← source=blocker)
        if (tgtId === current) {
          highlightLinks.add(linkKey(l));
          if (!visited.has(srcId)) queue.push(srcId);
        }
      }
    }
  }

  graph.linkWidth(graph.linkWidth());
}

// --- Dep tree expansion: load full deps for a node via Show API ---
const expandedNodes = new Set(); // track which nodes have been expanded

async function expandDepTree(node) {
  if (expandedNodes.has(node.id)) {
    console.log(`[beads3d] ${node.id} already expanded`);
    return;
  }
  expandedNodes.add(node.id);

  const statusEl = document.getElementById('status');
  statusEl.textContent = `expanding ${node.id}...`;

  try {
    const full = await api.show(node.id);
    const existingIds = new Set(graphData.nodes.map((n) => n.id));
    const existingLinks = new Set(graphData.links.map((l) => linkKey(l)));
    let addedNodes = 0;
    let addedLinks = 0;

    // Process dependencies from Show response
    const deps = full.dependencies || [];
    for (const dep of deps) {
      const depId = dep.depends_on_id || dep.id;
      if (!depId) continue;

      const depType = dep.type || dep.dependency_type || 'blocks';

      // Add node if not already in graph
      if (!existingIds.has(depId)) {
        graphData.nodes.push({
          id: depId,
          title: dep.title || depId,
          status: dep.status || 'open',
          priority: dep.priority ?? 3,
          issue_type: dep.issue_type || 'task',
          assignee: dep.assignee || '',
          _blocked: false,
          _expanded: true,
        });
        existingIds.add(depId);
        addedNodes++;
      }

      // Add link if not already present
      const lk = `${node.id}->${depId}`;
      if (!existingLinks.has(lk)) {
        graphData.links.push({ source: node.id, target: depId, dep_type: depType });
        existingLinks.add(lk);
        addedLinks++;
      }
    }

    // Process blocked_by
    const blockedBy = full.blocked_by || [];
    for (const blockerId of blockedBy) {
      if (!blockerId) continue;

      if (!existingIds.has(blockerId)) {
        graphData.nodes.push({
          id: blockerId,
          title: blockerId,
          status: 'open',
          priority: 3,
          issue_type: 'task',
          _blocked: false,
          _expanded: true,
        });
        existingIds.add(blockerId);
        addedNodes++;
      }

      const lk = `${blockerId}->${node.id}`;
      if (!existingLinks.has(lk)) {
        graphData.links.push({ source: blockerId, target: node.id, dep_type: 'blocks' });
        existingLinks.add(lk);
        addedLinks++;
      }
    }

    // Fetch titles for newly added placeholder nodes (max 10, parallel)
    const untitledNodes = graphData.nodes.filter((n) => n._expanded && n.title === n.id);
    await Promise.all(
      untitledNodes.slice(0, 10).map(async (n) => {
        try {
          const detail = await api.show(n.id);
          n.title = detail.title || n.id;
          n.status = detail.status || n.status;
          n.issue_type = detail.issue_type || n.issue_type;
          n.priority = detail.priority ?? n.priority;
          n.assignee = detail.assignee || n.assignee;
          n._blocked = !!(detail.blocked_by && detail.blocked_by.length > 0);
        } catch {
          /* placeholder stays as-is */
        }
      }),
    );

    // Update the graph — save/restore camera to prevent library auto-reposition (bd-7ccyd)
    const cam = graph.camera();
    const savedPos = cam.position.clone();
    const ctl = graph.controls();
    const savedTgt = ctl?.target?.clone();
    graph.graphData(graphData);
    cam.position.copy(savedPos);
    if (ctl && savedTgt) {
      ctl.target.copy(savedTgt);
      ctl.update();
    }

    // Highlight the expanded subtree
    selectNode(node);

    statusEl.textContent = `expanded ${node.id}: +${addedNodes} nodes, +${addedLinks} links`;
    statusEl.className = 'connected';
    console.log(`[beads3d] Expanded ${node.id}: +${addedNodes} nodes, +${addedLinks} links`);
  } catch (err) {
    statusEl.textContent = `expand failed: ${err.message}`;
    statusEl.className = 'error';
    console.error(`[beads3d] Expand failed for ${node.id}:`, err);
  }
}

// copyToClipboard, showCtxToast, context menu event listeners moved to context-menu.js (bd-7t6nt)

// --- Filtering ---
function applyFilters() {
  const q = searchFilter.toLowerCase();
  graphData.nodes.forEach((n) => {
    n._revealed = false; // reset before re-evaluating (hq-vorf47)
    let hidden = false;

    // Text search
    if (
      q &&
      !(n.id || '').toLowerCase().includes(q) &&
      !(n.title || '').toLowerCase().includes(q) &&
      !(n.assignee || '').toLowerCase().includes(q)
    ) {
      hidden = true;
    }

    // Agent visibility controls (bd-n0971, bd-8o2gd)
    if (n.issue_type === 'agent') {
      const agentStatus = (n.status || '').toLowerCase();
      // Always hide closed/tombstone agents
      if (agentStatus === 'closed' || agentStatus === 'tombstone') {
        hidden = true;
      }
      // Master toggle: hide all agents (bd-8o2gd)
      if (!agentFilterShow) {
        hidden = true;
      }
      // Rig exclusion: hide agents on excluded rigs (bd-8o2gd)
      if (agentFilterRigExclude.size > 0 && n.rig && agentFilterRigExclude.has(n.rig)) {
        hidden = true;
      }
      // Name exclusion: hide agents matching glob patterns (bd-8o2gd phase 4)
      if (agentFilterNameExclude.length > 0) {
        const name = (n.id || '').toLowerCase();
        if (agentFilterNameExclude.some((p) => globMatch(p, name))) hidden = true;
      }
    }

    // Status filter — agent nodes are exempt from user status filters (bd-keeha)
    if (statusFilter.size > 0 && n.issue_type !== 'agent' && !statusFilter.has(n.status)) {
      hidden = true;
    }

    // Type filter — agent nodes are always visible (bd-keeha)
    if (typeFilter.size > 0 && n.issue_type !== 'agent' && !typeFilter.has(n.issue_type)) {
      hidden = true;
    }

    // Priority filter (bd-8o2gd phase 2) — agents exempt
    if (priorityFilter.size > 0 && n.issue_type !== 'agent') {
      const p = n.priority != null ? String(n.priority) : null;
      if (p === null || !priorityFilter.has(p)) hidden = true;
    }

    // Assignee filter (bd-8o2gd phase 2) — agents exempt
    if (assigneeFilter && n.issue_type !== 'agent') {
      if ((n.assignee || '').toLowerCase() !== assigneeFilter.toLowerCase()) hidden = true;
    }

    // Age filter (bd-uc0mw): hide old closed beads, always show active/open/blocked/agent
    if (!hidden && activeAgeDays > 0 && n.status === 'closed') {
      const updatedAt = n.updated_at ? new Date(n.updated_at) : null;
      if (updatedAt) {
        const cutoff = Date.now() - activeAgeDays * 86400000;
        if (updatedAt.getTime() < cutoff) {
          hidden = true;
          n._ageFiltered = true; // mark so we can rescue connected nodes below
        }
      }
    }

    // Hide resolved/expired/closed decisions — only show pending (bd-zr374)
    if (!hidden && (n.issue_type === 'gate' || n.issue_type === 'decision')) {
      const ds = n._decisionState || (n.status === 'closed' ? 'resolved' : 'pending');
      if (ds !== 'pending') hidden = true;
    }

    n._hidden = hidden;
    n._searchMatch = !hidden && !!q;
  });

  // Epic collapse: hide children of collapsed epics (kd-XGgiokgQBH)
  if (collapsedEpics.size > 0) {
    // Build set of child node IDs for collapsed epics
    const collapsedChildren = new Set();
    for (const l of graphData.links) {
      if (l.dep_type !== 'parent-child' && l.dep_type !== 'child-of') continue;
      const srcId = typeof l.source === 'object' ? l.source.id : l.source;
      const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      if (collapsedEpics.has(srcId)) {
        collapsedChildren.add(tgtId);
      }
    }
    for (const n of graphData.nodes) {
      if (collapsedChildren.has(n.id)) {
        n._hidden = true;
        n._epicCollapsed = true; // mark for potential rescue
      }
    }
  }

  // Hide orphaned agents (bd-n0971, bd-8o2gd): if all of an agent's connected
  // beads are hidden, hide the agent too — unless agentFilterOrphaned is true.
  // Exception (bd-ixx3d): never hide agents with active/idle status — these are
  // live agents from the roster and must always be visible even without edges.
  for (const n of graphData.nodes) {
    if (n.issue_type !== 'agent' || n._hidden) continue;
    if (agentFilterOrphaned) continue; // bd-8o2gd: user wants to see orphaned agents
    const agentStatus = (n.status || '').toLowerCase();
    if (agentStatus === 'active' || agentStatus === 'idle') continue;
    const hasVisibleBead = graphData.links.some((l) => {
      const srcId = typeof l.source === 'object' ? l.source.id : l.source;
      const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      if (srcId !== n.id) return false;
      const bead = graphData.nodes.find((nd) => nd.id === tgtId);
      return bead && !bead._hidden;
    });
    if (!hasVisibleBead) n._hidden = true;
  }

  // Rescue age-filtered nodes that are directly connected to visible nodes (bd-uc0mw).
  // This ensures dependency chains remain visible even when old closed beads are culled.
  if (activeAgeDays > 0) {
    const visibleIds = new Set(graphData.nodes.filter((n) => !n._hidden).map((n) => n.id));
    for (const n of graphData.nodes) {
      if (!n._ageFiltered) continue;
      // Check if this age-filtered node has an edge to/from any visible node
      const connected = graphData.links.some((l) => {
        const srcId = typeof l.source === 'object' ? l.source.id : l.source;
        const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
        return (srcId === n.id && visibleIds.has(tgtId)) || (tgtId === n.id && visibleIds.has(srcId));
      });
      if (connected) {
        n._hidden = false;
        n._ageFiltered = false;
      }
    }
  }

  // Click-to-reveal: force-show nodes in the revealed subgraph (hq-vorf47).
  // This overrides all filters for the connected component of the clicked node.
  if (revealedNodes.size > 0) {
    for (const nodeId of revealedNodes) {
      const rn = graphData.nodes.find((nd) => nd.id === nodeId);
      if (rn) {
        rn._hidden = false;
        rn._revealed = true;
      }
    }
  }

  // Build ordered search results for navigation
  if (q) {
    searchResults = graphData.nodes
      .filter((n) => n._searchMatch)
      .sort((a, b) => {
        const aId = (a.id || '').toLowerCase().includes(q) ? 0 : 1;
        const bId = (b.id || '').toLowerCase().includes(q) ? 0 : 1;
        if (aId !== bId) return aId - bId;
        const aTitle = (a.title || '').toLowerCase().includes(q) ? 0 : 1;
        const bTitle = (b.title || '').toLowerCase().includes(q) ? 0 : 1;
        if (aTitle !== bTitle) return aTitle - bTitle;
        return (a.priority ?? 9) - (b.priority ?? 9);
      });
    if (searchResults.length > 0) {
      searchResultIdx = Math.min(Math.max(searchResultIdx, 0), searchResults.length - 1);
    } else {
      searchResultIdx = -1;
    }
  } else {
    searchResults = [];
    searchResultIdx = -1;
  }

  // Trigger re-render
  graph.nodeVisibility((n) => !n._hidden);
  graph.linkVisibility((l) => {
    const src = typeof l.source === 'object' ? l.source : graphData.nodes.find((n) => n.id === l.source);
    const tgt = typeof l.target === 'object' ? l.target : graphData.nodes.find((n) => n.id === l.target);
    return src && tgt && !src._hidden && !tgt._hidden && !depTypeHidden.has(l.dep_type);
  });

  // Rebuild node objects when reveal state changes (ghost opacity) (hq-vorf47)
  if (revealedNodes.size > 0 || graphData.nodes.some((n) => n._revealed)) {
    graph.nodeThreeObject(graph.nodeThreeObject());
  }

  updateFilterCount();
}

// Navigate search results: fly camera to the current match
function flyToSearchResult() {
  if (searchResults.length === 0 || searchResultIdx < 0) return;
  const node = searchResults[searchResultIdx];
  if (!node) return;

  selectNode(node);

  const distance = 150;
  const dx = node.x || 0,
    dy = node.y || 0,
    dz = node.z || 0;
  const distRatio = 1 + distance / (Math.hypot(dx, dy, dz) || 1);
  graph.cameraPosition({ x: dx * distRatio, y: dy * distRatio, z: dz * distRatio }, node, 800);

  showDetail(node);
}

function nextSearchResult() {
  if (searchResults.length === 0) return;
  searchResultIdx = (searchResultIdx + 1) % searchResults.length;
  updateFilterCount();
  flyToSearchResult();
}

function prevSearchResult() {
  if (searchResults.length === 0) return;
  searchResultIdx = (searchResultIdx - 1 + searchResults.length) % searchResults.length;
  updateFilterCount();
  flyToSearchResult();
}

// --- Epic cycling: Shift+S/D navigation (bd-pnngb) ---

function rebuildEpicIndex() {
  const prev = _epicNodes.map((n) => n.id).join(',');
  _epicNodes = graphData.nodes
    .filter((n) => n.issue_type === 'epic' && !n._hidden)
    .sort((a, b) => (a.title || a.id).localeCompare(b.title || b.id));
  const curr = _epicNodes.map((n) => n.id).join(',');
  // Reset index if the set of epics changed
  if (prev !== curr) _epicCycleIndex = -1;
}

function cycleEpic(delta) {
  if (_epicNodes.length === 0) return;
  if (_epicCycleIndex < 0) {
    _epicCycleIndex = delta > 0 ? 0 : _epicNodes.length - 1;
  } else {
    _epicCycleIndex = (_epicCycleIndex + delta + _epicNodes.length) % _epicNodes.length;
  }
  highlightEpic(_epicNodes[_epicCycleIndex]);
}

function highlightEpic(epicNode) {
  if (!epicNode) return;

  // Select the epic and fly camera to it
  selectNode(epicNode);
  const distance = 160;
  const dx = epicNode.x || 0,
    dy = epicNode.y || 0,
    dz = epicNode.z || 0;
  const distRatio = 1 + distance / (Math.hypot(dx, dy, dz) || 1);
  graph.cameraPosition({ x: dx * distRatio, y: dy * distRatio, z: dz * distRatio }, epicNode, 800);

  // Find all child/descendant node IDs via parent-child edges.
  // Direction is inconsistent: raw edges have source=parent, target=child;
  // promoted blocks edges have source=child, target=parent (epic).
  // So we check both directions.
  const childIds = new Set();
  const queue = [epicNode.id];
  while (queue.length > 0) {
    const parentId = queue.shift();
    for (const l of graphData.links) {
      if (l.dep_type !== 'parent-child') continue;
      const srcId = typeof l.source === 'object' ? l.source.id : l.source;
      const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      let childId = null;
      if (srcId === parentId && !childIds.has(tgtId)) childId = tgtId;
      else if (tgtId === parentId && !childIds.has(srcId)) childId = srcId;
      if (childId) {
        childIds.add(childId);
        queue.push(childId);
      }
    }
  }

  // Dim non-descendant nodes, emphasize descendants
  for (const n of graphData.nodes) {
    const obj = n.__threeObj;
    if (!obj) continue;
    if (n.id === epicNode.id || childIds.has(n.id)) {
      obj.traverse((c) => {
        if (c.material) c.material.opacity = 1.0;
      });
    } else {
      obj.traverse((c) => {
        if (c.material) c.material.opacity = 0.15;
      });
    }
  }

  // Show detail panel
  showDetail(epicNode);

  // Show HUD indicator
  showEpicHUD(_epicCycleIndex, _epicNodes.length, epicNode.title || epicNode.id);
}

function clearEpicHighlight() {
  _epicCycleIndex = -1;
  restoreAllNodeOpacity();
  hideEpicHUD();
}

let _epicHUDEl = null;
let _epicHUDTimer = null;

function showEpicHUD(index, total, title) {
  if (!_epicHUDEl) {
    _epicHUDEl = document.createElement('div');
    _epicHUDEl.id = 'epic-hud';
    document.body.appendChild(_epicHUDEl);
  }
  _epicHUDEl.textContent = `Epic ${index + 1}/${total}: ${title}`;
  _epicHUDEl.style.display = 'block';
  _epicHUDEl.style.opacity = '1';

  // Auto-fade after 3 seconds of no cycling
  clearTimeout(_epicHUDTimer);
  _epicHUDTimer = setTimeout(() => {
    _epicHUDEl.style.opacity = '0';
    setTimeout(() => {
      if (_epicHUDEl) _epicHUDEl.style.display = 'none';
    }, 400);
  }, 3000);
}

function hideEpicHUD() {
  clearTimeout(_epicHUDTimer);
  if (_epicHUDEl) {
    _epicHUDEl.style.display = 'none';
  }
}

// Populate rig filter pills in a container — clickable pills for each discovered rig (bd-8o2gd)
function _buildRigPillsIn(container, nodes) {
  if (!container) return;

  const rigs = new Set();
  for (const n of nodes) {
    if (n.issue_type === 'agent' && n.rig) rigs.add(n.rig);
  }
  const sortedRigs = [...rigs].sort();

  // Only rebuild if rig set changed
  const currentRigs = [...container.querySelectorAll('.rig-pill')].map((p) => p.dataset.rig);
  if (currentRigs.length === sortedRigs.length && currentRigs.every((r, i) => r === sortedRigs[i])) {
    for (const pill of container.querySelectorAll('.rig-pill')) {
      pill.classList.toggle('excluded', agentFilterRigExclude.has(pill.dataset.rig));
    }
    return;
  }

  container.innerHTML = '';
  for (const rig of sortedRigs) {
    const pill = document.createElement('span');
    pill.className = 'rig-pill';
    pill.dataset.rig = rig;
    pill.textContent = rig;
    pill.style.color = rigColor(rig);
    pill.style.borderColor = rigColor(rig) + '66';
    pill.style.background = rigColor(rig) + '18';
    if (agentFilterRigExclude.has(rig)) pill.classList.add('excluded');
    pill.title = `Click to ${agentFilterRigExclude.has(rig) ? 'show' : 'hide'} agents on ${rig}`;
    pill.addEventListener('click', () => {
      if (agentFilterRigExclude.has(rig)) {
        agentFilterRigExclude.delete(rig);
      } else {
        agentFilterRigExclude.add(rig);
      }
      // Sync all rig pill containers
      _syncAllRigPills();
      applyFilters();
    });
    container.appendChild(pill);
  }
}

function _syncAllRigPills() {
  document.querySelectorAll('.rig-pill').forEach((pill) => {
    const rig = pill.dataset.rig;
    pill.classList.toggle('excluded', agentFilterRigExclude.has(rig));
    pill.title = `Click to ${agentFilterRigExclude.has(rig) ? 'show' : 'hide'} agents on ${rig}`;
  });
}

function updateRigPills(nodes) {
  _buildRigPillsIn(document.getElementById('agent-rig-pills'), nodes);
  _buildRigPillsIn(document.getElementById('fd-rig-pills'), nodes);
}

// Filter Dashboard moved to filter-dashboard.js (bd-7t6nt)

// Layout modes, DAG dragging subtree (beads-6253), Agent DAG tether (beads-1gx1)
// moved to layout.js (bd-7t6nt)

// Rubber-band selection moved to camera.js (bd-7t6nt)

// --- Refresh ---
// Merge new data into the existing graph, preserving node positions to avoid layout jumps.
// Only triggers a full graph.graphData() call when nodes are added or removed.
// Uses a low alpha reheat to gently integrate new nodes without scattering the layout.
async function refresh() {
  const data = await fetchGraphData();
  if (!data) return;

  const currentNodes = graphData.nodes;
  const currentLinks = graphData.links;
  const existingById = new Map(currentNodes.map((n) => [n.id, n]));
  const newIds = new Set(data.nodes.map((n) => n.id));

  // Position-related keys to preserve across refreshes
  const POSITION_KEYS = ['x', 'y', 'z', 'vx', 'vy', 'vz', 'fx', 'fy', 'fz', '__threeObj', '_wasDimmed'];

  let nodesAdded = 0;
  let nodesRemoved = 0;

  // Update existing nodes in-place, detect additions
  const mergedNodes = data.nodes.map((incoming) => {
    const existing = existingById.get(incoming.id);
    if (existing) {
      // Update properties in-place (preserving position/velocity/three.js object)
      for (const key of Object.keys(incoming)) {
        if (!POSITION_KEYS.includes(key)) {
          existing[key] = incoming[key];
        }
      }
      existing._blocked = !!(incoming.blocked_by && incoming.blocked_by.length > 0);
      existing._jackExpired = incoming.issue_type === 'jack' && incoming.jack_expires_at && new Date(incoming.jack_expires_at).getTime() < Date.now();
      return existing;
    }
    // New node — place near a connected neighbor if possible, else near origin
    nodesAdded++;
    const newNode = {
      ...incoming,
      _blocked: !!(incoming.blocked_by && incoming.blocked_by.length > 0),
      _jackExpired: incoming.issue_type === 'jack' && incoming.jack_expires_at && new Date(incoming.jack_expires_at).getTime() < Date.now(),
    };
    // Seed new nodes near a connected existing node to reduce layout shock
    const neighborId = (incoming.blocked_by || [])[0] || incoming.assignee_id;
    const neighbor = neighborId && existingById.get(neighborId);
    if (neighbor && neighbor.x !== undefined) {
      newNode.x = neighbor.x + (Math.random() - 0.5) * 30;
      newNode.y = neighbor.y + (Math.random() - 0.5) * 30;
      newNode.z = neighbor.z + (Math.random() - 0.5) * 30;
    }
    return newNode;
  });

  // Fire firework bursts for newly created beads that were pending from SSE events (bd-4gmot)
  if (_pendingFireworks.size > 0) {
    for (const node of mergedNodes) {
      if (_pendingFireworks.has(node.id) && !existingById.has(node.id)) {
        spawnFireworkBurst(node);
        _pendingFireworks.delete(node.id);
      }
    }
    _pendingFireworks.clear(); // clear any stale entries
  }

  // Detect removed nodes
  for (const n of currentNodes) {
    if (!newIds.has(n.id)) nodesRemoved++;
  }

  // Build link key for comparison (includes dep_type)
  // Exclude rig_conflict edges — they're re-synthesized every refresh and would
  // always trigger structureChanged even when nothing meaningful changed (bd-c1x6p).
  const refreshLinkKey = (l) =>
    `${typeof l.source === 'object' ? l.source.id : l.source}→${typeof l.target === 'object' ? l.target.id : l.target}:${l.dep_type}`;
  const existingLinkKeys = new Set(currentLinks.filter((l) => l.dep_type !== 'rig_conflict').map(refreshLinkKey));
  const newLinkKeys = new Set(data.links.filter((l) => l.dep_type !== 'rig_conflict').map(refreshLinkKey));

  let linksChanged = false;
  // Detect genuinely new links for edge spark animations (bd-9qeto)
  const brandNewLinks = data.links.filter((l) => !existingLinkKeys.has(refreshLinkKey(l)));
  if (
    data.links.length !== currentLinks.length ||
    brandNewLinks.length > 0 ||
    currentLinks.some((l) => !newLinkKeys.has(refreshLinkKey(l)))
  ) {
    linksChanged = true;
  }

  // Spawn edge sparks for new associations (bd-9qeto)
  // Only fire for non-assigned_to links (assigned_to already has glow tubes)
  for (const nl of brandNewLinks) {
    if (nl.dep_type === 'assigned_to') continue;
    const srcId = typeof nl.source === 'object' ? nl.source.id : nl.source;
    const tgtId = typeof nl.target === 'object' ? nl.target.id : nl.target;
    const srcNode = mergedNodes.find((n) => n.id === srcId);
    const tgtNode = mergedNodes.find((n) => n.id === tgtId);
    if (srcNode && tgtNode && srcNode.x !== undefined && tgtNode.x !== undefined) {
      const beamColor =
        nl.dep_type === 'blocks'
          ? 0xd04040
          : nl.dep_type === 'parent-child'
            ? 0x8b45a6
            : nl.dep_type === 'waits-for'
              ? 0xd4a017
              : 0x4a9eff;
      spawnEnergyBeam(srcNode, tgtNode, beamColor);
    }
  }

  const structureChanged = nodesAdded > 0 || nodesRemoved > 0 || linksChanged;

  graphData = { nodes: mergedNodes, links: data.links };

  // Populate rig filter pills from agent nodes (bd-8o2gd)
  updateRigPills(mergedNodes);
  // Update assignee buttons in filter dashboard (bd-8o2gd phase 2)
  updateAssigneeButtons();

  applyFilters();
  rebuildEpicIndex();
  updateRightSidebar(graphData); // bd-inqge
  updateDecisionList(graphData); // beads-zuc3: refresh decision lightbox

  // Compute pending decision badge counts per parent node (bd-o6tgy)
  const nodeById = new Map(mergedNodes.map((n) => [n.id, n]));
  for (const n of mergedNodes) n._pendingDecisions = 0;
  for (const link of data.links) {
    if (link.dep_type !== 'parent-child') continue;
    const childId = typeof link.target === 'object' ? link.target.id : link.target;
    const parentId = typeof link.source === 'object' ? link.source.id : link.source;
    const child = nodeById.get(childId);
    const parent = nodeById.get(parentId);
    if (child && parent && (child.issue_type === 'gate' || child.issue_type === 'decision')) {
      const ds = child._decisionState || (child.status === 'closed' ? 'resolved' : 'pending');
      if (ds === 'pending') parent._pendingDecisions++;
    }
  }

  // Rig conflict edges — red links between agents sharing the same rig (bd-90ikf)
  // Synthesized client-side: group agents by rig, create conflict edges between pairs.
  const rigGroups = new Map(); // rig -> [agentId, ...]
  for (const n of mergedNodes) {
    if (n.issue_type === 'agent' && n.rig) {
      if (!rigGroups.has(n.rig)) rigGroups.set(n.rig, []);
      rigGroups.get(n.rig).push(n.id);
    }
  }
  // Remove stale conflict edges from previous update
  graphData.links = graphData.links.filter((l) => l.dep_type !== 'rig_conflict');
  for (const [, agents] of rigGroups) {
    if (agents.length < 2) continue;
    for (let i = 0; i < agents.length; i++) {
      for (let j = i + 1; j < agents.length; j++) {
        graphData.links.push({
          source: agents[i],
          target: agents[j],
          dep_type: 'rig_conflict',
        });
      }
    }
  }

  if (structureChanged) {
    // graph.graphData() reheats d3-force to alpha=1, which scatters positioned nodes.
    // Fix (bd-7ccyd): pin ALL existing nodes at their current positions during the
    // graphData() call. Only new nodes (without positions) float freely. After a brief
    // settling period, unpin so the layout can gently adjust.
    const pinnedNodes = [];
    for (const n of mergedNodes) {
      if (n.x !== undefined && n.fx === undefined) {
        n.fx = n.x;
        n.fy = n.y;
        n.fz = n.z || 0;
        pinnedNodes.push(n);
      }
    }

    // Save camera state — graphData() triggers the library's onUpdate which
    // auto-repositions the camera when it detects a (0,0,Z) default position.
    // We restore immediately after to prevent any camera jump (bd-7ccyd).
    const cam = graph.camera();
    const savedCamPos = cam.position.clone();
    const controls = graph.controls();
    const savedTarget = controls?.target?.clone();

    graph.graphData(graphData);

    // Counter the force reheat: graphData() sets alpha=1 which causes violent
    // node scattering. Temporarily set high alphaDecay so the simulation cools
    // down much faster (settles in ~50 ticks instead of ~300). Restore normal
    // decay after settling period (bd-c1x6p).
    const normalDecay = 0.0228; // d3 default
    graph.d3AlphaDecay(0.1); // 4x faster cooldown
    setTimeout(() => graph.d3AlphaDecay(normalDecay), 2000);

    // Restore camera position immediately (prevents library auto-reposition)
    cam.position.copy(savedCamPos);
    if (controls && savedTarget) {
      controls.target.copy(savedTarget);
      controls.update();
    }

    // Release pins after simulation has mostly cooled down. With the faster
    // alphaDecay, alpha drops below 0.1 within ~1s. Unpin after 2s to be
    // safe — remaining alpha is negligible so nodes barely drift (bd-c1x6p).
    setTimeout(() => {
      for (const n of pinnedNodes) {
        delete n.fx;
        delete n.fy;
        delete n.fz;
      }
    }, 2000);
  }
  // If only properties changed (status, title, etc.), the existing three.js
  // objects pick up the changes via the animation tick — no layout reset needed.

  // bd-tgg70: Update beads lists in all open agent windows after graph refresh
  refreshAgentWindowBeads();
}

// SSE, mutations, doots, doot popups moved to mutations.js (bd-7t6nt)
// showAgentWindow through enableTopResize moved to agent-windows.js (bd-7t6nt)

// Right sidebar moved to right-sidebar.js (bd-7t6nt)

// Control panel moved to control-panel.js (bd-69y6v)

// openAgentsView through autoScroll moved to agent-windows.js (bd-7t6nt)

function connectBusStream() {
  try {
    let _dootDrops = 0;
    // bd-eyvbw: debounced refresh for new agents not yet in the graph.
    // When bus events arrive for an agent before the graph has fetched its node,
    // schedule a quick refresh so the node appears without a manual page reload.
    let _newAgentRefreshTimer = null;
    const _pendingNewAgents = new Set(); // agent IDs awaiting graph refresh
    api.connectBusEvents(
      'agents,hooks,oj,mutations,mail,decisions,advisory',
      (evt) => {
        const label = dootLabel(evt);
        if (!label) return;

        const node = findAgentNode(evt);
        if (!node) {
          // bd-eyvbw: If we can identify the agent but it's not in the graph yet,
          // schedule a refresh to fetch the new node instead of silently dropping.
          const pendingAgentId = resolveAgentIdLoose(evt);
          if (pendingAgentId && !_pendingNewAgents.has(pendingAgentId)) {
            _pendingNewAgents.add(pendingAgentId);
            clearTimeout(_newAgentRefreshTimer);
            _newAgentRefreshTimer = setTimeout(async () => {
              await refresh();
              // After refresh, create windows for any newly-appeared agents
              for (const pid of _pendingNewAgents) {
                if (!agentWindows.has(pid) && graphData) {
                  const agentNode = graphData.nodes.find((n) => n.id === pid);
                  if (agentNode) {
                    if (getAgentsViewOpen()) {
                      createAgentWindowInGrid(agentNode);
                    } else {
                      showAgentWindow(agentNode);
                    }
                  }
                }
              }
              _pendingNewAgents.clear();
            }, 1500);
          }
          if (++_dootDrops <= 5) {
            const dp = evt.payload || {};
            console.debug(
              '[beads3d] doot drop %d: type=%s actor=%s issue=%s',
              _dootDrops,
              evt.type,
              dp.actor,
              dp.issue_id,
            );
          }
          // bd-eyvbw: still process agent window + roster updates below (skip doot/VFX only)
        }

        if (node) spawnDoot(node, label, dootColor(evt));

        // Event sprites: status change pulse + close burst (bd-9qeto)
        const p = evt.payload || {};
        if (node && (evt.type === 'MutationStatus' || evt.type === 'MutationClose') && p.issue_id && graphData) {
          const issueNode = graphData.nodes.find((n) => n.id === p.issue_id);
          if (issueNode) {
            const newStatus = p.new_status || (evt.type === 'MutationClose' ? 'closed' : '');
            spawnStatusPulse(issueNode, p.old_status || '', newStatus);
          }
        }

        // Edge pulse: spark along assigned_to edge from agent to task (bd-kc7r1)
        // Rate-limited to 1 spark per agent per 500ms to avoid overwhelming the scene.
        if (node && node.issue_type === 'agent' && graphData) {
          if (!connectBusStream._lastSpark) connectBusStream._lastSpark = {};
          const now = Date.now();
          const lastSpark = connectBusStream._lastSpark[node.id] || 0;
          if (now - lastSpark > 500) {
            connectBusStream._lastSpark[node.id] = now;
            const agentNodeId = node.id;
            const assignedLinks = graphData.links.filter(
              (l) =>
                l.dep_type === 'assigned_to' && (typeof l.source === 'object' ? l.source.id : l.source) === agentNodeId,
            );
            for (const link of assignedLinks) {
              const tgtId = typeof link.target === 'object' ? link.target.id : link.target;
              const taskNode = graphData.nodes.find((n) => n.id === tgtId);
              if (taskNode && !taskNode._hidden) {
                const sparkHex = parseInt((dootColor(evt) || '#ff6b35').replace('#', ''), 16);
                spawnEdgeSpark(node, taskNode, sparkHex);
                break; // one pulse per event
              }
            }
          }
        }

        // Decision event: update graph node state, rebuild Three.js, spark edges (bd-0j7hr, bd-fbcbd)
        if (evt.type && evt.type.startsWith('Decision') && p.decision_id && graphData) {
          const decNode = graphData.nodes.find((n) => n.id === p.decision_id);
          if (decNode) {
            if (evt.type === 'DecisionCreated') decNode._decisionState = 'pending';
            else if (evt.type === 'DecisionResponded') decNode._decisionState = 'resolved';
            else if (evt.type === 'DecisionExpired') decNode._decisionState = 'expired';
            // Re-apply filters: resolved/expired decisions disappear from graph (bd-zr374)
            applyFilters();
            // Rebuild node Three.js object to reflect new color/shape
            graph.nodeThreeObject(graph.nodeThreeObject());

            // Edge spark: agent ↔ decision node (bd-fbcbd)
            if (p.requested_by && !decNode._hidden) {
              const agentNode = graphData.nodes.find(
                (n) => n.issue_type === 'agent' && (n.title === p.requested_by || n.id === `agent:${p.requested_by}`),
              );
              if (agentNode && !agentNode._hidden) {
                const sparkColor = evt.type === 'DecisionResponded' ? 0x2d8a4e : 0xd4a017;
                spawnEdgeSpark(agentNode, decNode, sparkColor);
              }
            }
          }
          // Toast notification for new/resolved decisions (bd-tausm)
          showDecisionToast(evt);
        }

        // Feed agent activity windows (bd-kau4k, bd-jgvas Phase 2: auto-open)
        const agentId = resolveAgentIdLoose(evt);
        if (agentId) {
          // Auto-create window if it doesn't exist yet (bd-jgvas)
          if (!agentWindows.has(agentId) && graphData) {
            const agentNode = graphData.nodes.find((n) => n.id === agentId);
            if (agentNode) {
              if (getAgentsViewOpen()) {
                // Create window inside the overlay grid
                createAgentWindowInGrid(agentNode);
              } else {
                showAgentWindow(agentNode);
              }
            }
          }
          if (agentWindows.has(agentId)) {
            appendAgentEvent(agentId, evt);
            // bd-bwi52: Flash tab for non-selected agents when events arrive
            if (getAgentsViewOpen() && agentId !== getSelectedAgentTab()) {
              const overlay = document.getElementById('agents-view');
              if (overlay) {
                const tab = overlay.querySelector(`.agent-tab[data-agent-id="${agentId}"]`);
                if (tab && !tab.classList.contains('active')) {
                  tab.classList.add('tab-flash');
                  setTimeout(() => tab.classList.remove('tab-flash'), 800);
                }
              }
            }
          }
        }

        // bd-9cpbc.1: live-update decision lightbox from bus events (beads-zuc3)
        if (evt.type && evt.type.startsWith('Decision')) {
          updateDecisionList(graphData);
        }
        if (evt.type === 'MutationStatus' || evt.type === 'MutationClose' || evt.type === 'MutationUpdate') {
          updateEpicProgress(graphData);
          updateDepHealth(graphData);
          // Live-update project pulse stats
          _liveUpdateProjectPulse(evt);
        }

        // bd-nnr22: update left sidebar agent roster from all SSE events
        updateAgentRosterFromEvent(evt);
      },
      {
        // bd-ki6im: bus SSE reconnection status (handler in mutations.js)
        onStatus: (state, info) => updateBusConnectionState(state, info),
      },
    );
  } catch {
    /* SSE not available — degrade gracefully */
  }
}

// --- Init ---
async function main() {
  try {
    initGraph();
    // Seed with empty data so the force layout initializes before first tick
    graph.graphData({ nodes: [], links: [] });

    // Context menu (bd-7t6nt) — wire dependencies
    setContextMenuDeps({
      api,
      getGraph: () => graph,
      getGraphData: () => graphData,
      escapeHtml,
      hideTooltip,
      expandDepTree,
      highlightSubgraph,
    });

    // Layout modes (bd-7t6nt) — wire dependencies
    setLayoutDeps({
      getGraph: () => graph,
      getGraphData: () => graphData,
      getLayoutGuides: () => layoutGuides,
      setLayoutGuides: (arr) => { layoutGuides = arr; },
    });

    // Minimap (bd-7t6nt) — wire dependencies
    setMinimapDeps({
      getGraph: () => graph,
      getGraphData: () => graphData,
      getHighlightNodes: () => highlightNodes,
      getSelectedNode: () => selectedNode,
    });

    // Camera, Controls, Box Select (bd-7t6nt) — wire dependencies
    setCameraDeps({
      api,
      getGraph: () => graph,
      getGraphData: () => graphData,
      getBloomPass: () => bloomPass,
      setBloomEnabled: v => { bloomEnabled = v; },
      getBloomEnabled: () => bloomEnabled,
      refresh,
      applyFilters,
      setLayout,
      toggleLabels,
      getLabelsVisible: () => labelsVisible,
      toggleMinimap,
      handleNodeClick,
      clearSelection,
      clearEpicHighlight,
      hideTooltip,
      isTextInputFocused,
      flyToSearchResult,
      nextSearchResult,
      prevSearchResult,
      togglePerfOverlay,
      togglePerfGraph,
      setVfxIntensity,
      presetVFX,
      cycleEpic,
      linkKey,
      _syncAllRigPills,
      state: {
        get multiSelected() { return multiSelected; },
        set multiSelected(v) { multiSelected = v; },
        get highlightNodes() { return highlightNodes; },
        get highlightLinks() { return highlightLinks; },
        get searchFilter() { return searchFilter; },
        set searchFilter(v) { searchFilter = v; },
        get statusFilter() { return statusFilter; },
        get typeFilter() { return typeFilter; },
        get priorityFilter() { return priorityFilter; },
        get assigneeFilter() { return assigneeFilter; },
        set assigneeFilter(v) { assigneeFilter = v; },
        get filterDashboardOpen() { return filterDashboardOpen; },
        set filterDashboardOpen(v) { filterDashboardOpen = v; },
        get activeAgeDays() { return activeAgeDays; },
        set activeAgeDays(v) { activeAgeDays = v; },
        get agentFilterShow() { return agentFilterShow; },
        set agentFilterShow(v) { agentFilterShow = v; },
        get agentFilterOrphaned() { return agentFilterOrphaned; },
        set agentFilterOrphaned(v) { agentFilterOrphaned = v; },
        get agentFilterRigExclude() { return agentFilterRigExclude; },
        get agentFilterNameExclude() { return agentFilterNameExclude; },
        set agentFilterNameExclude(v) { agentFilterNameExclude = v; },
        get depTypeHidden() { return depTypeHidden; },
        get maxEdgesPerNode() { return maxEdgesPerNode; },
        set maxEdgesPerNode(v) { maxEdgesPerNode = v; },
        get _agentTetherStrength() { return getAgentTetherStrength(); },
        set _agentTetherStrength(v) { setAgentTetherStrength(v); },
        get minimapVisible() { return getMinimapVisible(); },
        set minimapVisible(v) { setMinimapVisible(v); },
        get searchResults() { return searchResults; },
        get searchResultIdx() { return searchResultIdx; },
        set searchResultIdx(v) { searchResultIdx = v; },
        get _searchDebounceTimer() { return _searchDebounceTimer; },
        set _searchDebounceTimer(v) { _searchDebounceTimer = v; },
        URL_PROFILE, URL_STATUS, URL_TYPES, URL_ASSIGNEE,
      },
    });

    setupControls();
    setupBoxSelect();
    await refresh();
    connectLiveUpdates();
    connectBusStream(); // bd-c7723: live NATS event doots on agent nodes
    // VFX system (bd-7t6nt) — wire dependencies
    setVfxDeps({
      getGraph: () => graph,
      getGraphData: () => graphData,
      getParticlePool: () => _particlePool,
    });
    // Left sidebar (bd-nnr22, bd-7t6nt) — wire dependencies
    setLeftSidebarDeps({
      api,
      getGraph: () => graph,
      getGraphData: () => graphData,
      getSelectedNode: () => selectedNode,
      selectNode,
      showDetail,
    });
    initLeftSidebar();
    startLeftSidebarIdleTimer();
    startAgentWindowIdleTimer();
    if (_pollIntervalId) clearInterval(_pollIntervalId);
    _pollIntervalId = setInterval(refresh, POLL_INTERVAL);
    graph.cameraPosition({ x: 0, y: 0, z: 400 });

    // URL deep-linking (bd-he95o): ?bead=<id> highlights and focuses a specific bead.
    // Delay to let force layout settle so camera can fly to stable positions.
    if (DEEP_LINK_BEAD) {
      setTimeout(() => focusDeepLinkBead(DEEP_LINK_BEAD), 2000);
    }
    // Molecule focus view (bd-lwut6): ?molecule=<id> focuses on a molecule's subgraph.
    if (DEEP_LINK_MOLECULE) {
      setTimeout(() => focusMolecule(DEEP_LINK_MOLECULE), 2000);
    }

    // Expose for Playwright tests
    window.__THREE = THREE;
    window.__beads3d = {
      graph,
      graphData: () => graphData,
      multiSelected: () => multiSelected,
      highlightNodes: () => highlightNodes,
      showBulkMenu,
      showDetail,
      hideDetail,
      selectNode,
      highlightSubgraph,
      clearSelection,
      focusMolecule,
      focusedMoleculeNodes: () => focusedMoleculeNodes,
      get selectedNode() {
        return selectedNode;
      },
      get cameraFrozen() {
        return cameraFrozen;
      },
    };
    // Expose doot internals for testing (bd-pg7vy)
    window.__beads3d_spawnDoot = spawnDoot;
    window.__beads3d_doots = () => doots;
    window.__beads3d_dootLabel = dootLabel;
    window.__beads3d_dootColor = dootColor;
    window.__beads3d_findAgentNode = findAgentNode;
    // Expose mutation handler for testing (bd-03b5v)
    window.__beads3d_applyMutation = applyMutationOptimistic;
    // Expose popup internals for testing (beads-xmix)
    window.__beads3d_showDootPopup = showDootPopup;
    window.__beads3d_dismissDootPopup = dismissDootPopup;
    window.__beads3d_dootPopups = () => dootPopups;
    // Expose agent window internals for testing (bd-kau4k)
    window.__beads3d_showAgentWindow = showAgentWindow;
    window.__beads3d_closeAgentWindow = closeAgentWindow;
    window.__beads3d_appendAgentEvent = appendAgentEvent;
    window.__beads3d_agentWindows = () => agentWindows;
    // Expose agents view overlay for testing (bd-jgvas)
    window.__beads3d_toggleAgentsView = toggleAgentsView;
    window.__beads3d_openAgentsView = openAgentsView;
    window.__beads3d_closeAgentsView = closeAgentsView;
    window.__beads3d_agentsViewOpen = () => getAgentsViewOpen();
    window.__beads3d_selectAgentTab = selectAgentTab;
    window.__beads3d_selectedAgentTab = () => getSelectedAgentTab();
    window.__beads3d_resolveAgentIdLoose = resolveAgentIdLoose;
    // Expose event sprite internals for testing (bd-9qeto)
    window.__beads3d_spawnStatusPulse = spawnStatusPulse;
    window.__beads3d_spawnEdgeSpark = spawnEdgeSpark;
    window.__beads3d_spawnEnergyBeam = spawnEnergyBeam;
    window.__beads3d_eventSprites = () => eventSprites;
    // Expose camera velocity system for testing (bd-zab4q)
    window.__beads3d_keysDown = _keysDown;
    window.__beads3d_camVelocity = _camVelocity;

    // Cleanup on page unload (bd-7n4g8): close SSE, clear intervals
    window.addEventListener('beforeunload', () => {
      api.destroy();
      if (_pollIntervalId) clearInterval(_pollIntervalId);
      if (_bloomResizeHandler) window.removeEventListener('resize', _bloomResizeHandler);
    });
  } catch (err) {
    console.error('Init failed:', err);
    document.getElementById('status').textContent = `init error: ${err.message}`;
    document.getElementById('status').className = 'error';
  }
}

main();

// _updateAgentStatusBar, idle timer moved to agent-windows.js (bd-7t6nt)
