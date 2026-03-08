// Shared mutable state for beads3d (bd-7t6nt: extracted from main.js monolith)
// All modules import from here to avoid circular dependencies.

import * as THREE from 'three';
import { BeadsAPI } from './api.js';

/**
 * @typedef {Object} Issue
 * @property {string} id - Bead ID (e.g. "bd-abc12")
 * @property {string} title
 * @property {string} status - open|in_progress|closed|blocked|hooked|deferred|review|on_ice|tombstone
 * @property {string} issue_type - epic|feature|bug|task|agent|decision|gate|chore|doc|test
 * @property {number} priority - 0-4 (P0=critical, P4=backlog)
 * @property {string} [assignee]
 * @property {string} [created_at]
 * @property {string} [updated_at]
 * @property {string} [description]
 * @property {string} [notes]
 * @property {string} [design]
 * @property {string[]} [labels]
 * @property {Object[]} [deps]
 */

/**
 * @typedef {Issue & {x:number, y:number, z:number, vx:number, vy:number, vz:number, fx:number|null, fy:number|null, fz:number|null, __threeObj?:THREE.Object3D, _hidden?:boolean, _blocked?:boolean, _decisionState?:string}} GraphNode
 */

/**
 * @typedef {Object} GraphLink
 * @property {string|GraphNode} source
 * @property {string|GraphNode} target
 * @property {string} dep_type - blocks|waits-for|relates-to|parent-child|assigned_to|rig_conflict
 */

/**
 * @typedef {Object} GraphData
 * @property {GraphNode[]} nodes
 * @property {GraphLink[]} links
 */

// --- Config ---
const params = new URLSearchParams(window.location.search);
/** @type {string} API base URL */
export const API_BASE = params.get('api') || '/api';
/** @type {string} Deep-link to a specific bead ID */
export const DEEP_LINK_BEAD = params.get('bead') || ''; // bd-he95o: URL deep-linking
/** @type {string} Deep-link to a specific molecule */
export const DEEP_LINK_MOLECULE = params.get('molecule') || ''; // bd-lwut6: molecule focus view
/** @type {string} Named filter profile from URL */
export const URL_PROFILE = params.get('profile') || ''; // bd-8o2gd phase 4: load named profile from URL
/** @type {string} Assignee filter from URL */
export const URL_ASSIGNEE = params.get('assignee') || ''; // bd-8o2gd phase 4: filter by assignee via URL
/** @type {string} Comma-separated status filters from URL */
export const URL_STATUS = params.get('status') || ''; // bd-8o2gd phase 4: comma-separated statuses
/** @type {string} Comma-separated type filters from URL */
export const URL_TYPES = params.get('types') || ''; // bd-8o2gd phase 4: comma-separated types
/** @type {number} Poll interval in ms */
export const POLL_INTERVAL = 30000; // bd-c1x6p: reduced from 10s to 30s — SSE handles live updates
/** @type {number} Max nodes to display */
export const MAX_NODES = 1000; // bd-04wet: raised from 500 to show more relevant beads

/** @type {BeadsAPI} Daemon API client */
export const api = new BeadsAPI(API_BASE);

// --- Shared geometries (reused across all nodes to reduce GC + draw overhead) ---
/** @type {{sphereHi: THREE.SphereGeometry, sphereLo: THREE.SphereGeometry, torus: THREE.TorusGeometry, octa: THREE.OctahedronGeometry, box: THREE.BoxGeometry}} Reusable geometries */
export const GEO = {
  sphereHi: new THREE.SphereGeometry(1, 12, 12), // unit sphere, scaled per-node
  sphereLo: new THREE.SphereGeometry(1, 6, 6), // low-poly glow shell
  torus: new THREE.TorusGeometry(1, 0.15, 6, 20), // unit torus for rings
  octa: new THREE.OctahedronGeometry(1, 0), // blocked spikes
  box: new THREE.BoxGeometry(1, 1, 1), // descent stage, general purpose
};

// Shared materia halo texture (bd-c7d5z) — lazy-initialized on first use
/** @type {THREE.Texture|null} Materia halo texture, lazy-initialized */
export let _materiaHaloTex = null;
/** @param {THREE.Texture} tex */
export function setMateriaHaloTex(tex) {
  _materiaHaloTex = tex;
}

// --- Graph State ---
/** @type {GraphData} Current graph data */
export let graphData = { nodes: [], links: [] };
/** @param {GraphData} data */
export function setGraphData(data) {
  graphData = data;
}
/** @type {Object|null} 3d-force-graph instance */
export let graph = null;
/** @param {Object} g */
export function setGraph(g) {
  graph = g;
}

/** @type {string} Current search filter text */
export let searchFilter = '';
/** @param {string} v */
export function setSearchFilter(v) {
  searchFilter = v;
}
/** @type {Set<string>} Active status filters (empty = show all) */
export const statusFilter = new Set(); // empty = show all
/** @type {Set<string>} Active issue type filters */
export const typeFilter = new Set();
/** @type {Set<number>} Active priority filters (empty = show all) */
export const priorityFilter = new Set(); // empty = show all priorities (bd-8o2gd phase 2)
/** @type {string} Assignee filter (empty = show all) */
export let assigneeFilter = ''; // empty = show all assignees (bd-8o2gd phase 2)
/** @param {string} v */
export function setAssigneeFilter(v) {
  assigneeFilter = v;
}
/** @type {boolean} Whether filter dashboard panel is open */
export let filterDashboardOpen = false; // slide-out filter panel state (bd-8o2gd phase 2)
/** @param {boolean} v */
export function setFilterDashboardOpen(v) {
  filterDashboardOpen = v;
}
/** @type {number} App start timestamp (performance.now) */
export const startTime = performance.now();
/** @type {GraphNode|null} Currently selected node */
export let selectedNode = null;
/** @param {GraphNode|null} n */
export function setSelectedNode(n) {
  selectedNode = n;
}
/** @type {Set<GraphNode>} Nodes highlighted by hover/selection */
export const highlightNodes = new Set();
/** @type {Set<GraphLink>} Links highlighted by hover/selection */
export const highlightLinks = new Set();
/** @type {Object|null} Three.js UnrealBloomPass instance */
export let bloomPass = null;
/** @param {Object} bp */
export function setBloomPass(bp) {
  bloomPass = bp;
}
/** @type {boolean} Whether bloom post-processing is enabled */
export let bloomEnabled = false;
/** @param {boolean} v */
export function setBloomEnabled(v) {
  bloomEnabled = v;
}
/** @type {THREE.Object3D[]} Layout visual aid objects (cleaned up on layout switch) */
export let layoutGuides = []; // THREE objects added as layout visual aids (cleaned up on layout switch)
/** @param {THREE.Object3D[]} arr */
export function setLayoutGuides(arr) {
  layoutGuides = arr;
}

// Search navigation state
/** @type {string[]} Ordered list of matching node IDs */
export let searchResults = []; // ordered list of matching node ids
/** @param {string[]} arr */
export function setSearchResults(arr) {
  searchResults = arr;
}
/** @type {number} Current position in search results (-1 = none) */
export let searchResultIdx = -1; // current position in results (-1 = none)
/** @param {number} v */
export function setSearchResultIdx(v) {
  searchResultIdx = v;
}
/** @type {boolean} Whether minimap is visible */
export let minimapVisible = true;
/** @param {boolean} v */
export function setMinimapVisible(v) {
  minimapVisible = v;
}

// Multi-selection state (rubber-band / shift+drag)
/** @type {Set<string>} Node IDs currently multi-selected */
export const multiSelected = new Set(); // set of node IDs currently multi-selected
/** @type {Set<string>} Node IDs force-shown by click-to-reveal */
export const revealedNodes = new Set(); // node IDs force-shown by click-to-reveal (hq-vorf47)
/** @type {Set<string>} Node IDs in the focused molecule */
export const focusedMoleculeNodes = new Set(); // node IDs in the focused molecule (bd-lwut6)
/** @type {boolean} Whether rubber-band box selection is active */
export let isBoxSelecting = false;
/** @param {boolean} v */
export function setIsBoxSelecting(v) {
  isBoxSelecting = v;
}
/** @type {{x:number, y:number}|null} Box selection start screen coords */
export let boxSelectStart = null; // {x, y} screen coords
/** @param {{x:number, y:number}|null} v */
export function setBoxSelectStart(v) {
  boxSelectStart = v;
}
/** @type {boolean} Whether orbit controls are locked by multi-select */
export let cameraFrozen = false; // true when multi-select has locked orbit controls (bd-casin)
/** @param {boolean} v */
export function setCameraFrozen(v) {
  cameraFrozen = v;
}
/** @type {boolean} Whether persistent info labels are shown on all nodes */
export let labelsVisible = true; // true when persistent info labels are shown on all nodes (bd-1o2f7, bd-oypa2: default on)
/** @param {boolean} v */
export function setLabelsVisible(v) {
  labelsVisible = v;
}

// Quake-style smooth camera movement (bd-zab4q)
/** @type {Set<string>} Currently held arrow/movement keys */
export const _keysDown = new Set(); // currently held arrow keys
/** @type {{x:number, y:number, z:number}} World-space camera velocity */
export const _camVelocity = { x: 0, y: 0, z: 0 }; // world-space camera velocity
/** @type {number} Camera acceleration per frame while key held */
export const CAM_ACCEL = 1.2; // acceleration per frame while key held
/** @type {number} Max camera strafe speed (units/frame) */
export const CAM_MAX_SPEED = 16; // max strafe speed (units/frame)
/** @type {number} Camera velocity friction multiplier per frame */
export const CAM_FRICTION = 0.88; // velocity multiplier per frame when no key held (lower = more friction)
/** @type {Map<string, HTMLElement>} Open detail panels keyed by bead ID */
export const openPanels = new Map(); // beadId → panel element (bd-fbmq3: tiling detail panels)
/** @type {number} Age filter: show beads updated within N days (0 = all) */
export let activeAgeDays = 7; // age filter: show beads updated within N days (0 = all) (bd-uc0mw)
/** @param {number} v */
export function setActiveAgeDays(v) {
  activeAgeDays = v;
}

// Agent filter state (bd-8o2gd: configurable filter dashboard, phase 1)
/** @type {boolean} Master toggle to show/hide all agent nodes */
export let agentFilterShow = true; // master toggle — show/hide all agent nodes
/** @param {boolean} v */
export function setAgentFilterShow(v) {
  agentFilterShow = v;
}
/** @type {boolean} Whether to show agents with no visible connected beads */
export let agentFilterOrphaned = false; // show agents with no visible connected beads
/** @param {boolean} v */
export function setAgentFilterOrphaned(v) {
  agentFilterOrphaned = v;
}
/** @type {Set<string>} Rig names to exclude from agent display */
export const agentFilterRigExclude = new Set(); // hide agents on these rigs (exact match)
/** @type {string[]} Glob patterns to hide agents by name */
export let agentFilterNameExclude = []; // glob patterns to hide agents by name (bd-8o2gd phase 4)
/** @param {string[]} v */
export function setAgentFilterNameExclude(v) {
  agentFilterNameExclude = v;
}

// Edge type filter (bd-a0vbd): hide specific edge types to reduce graph density
/** @type {Set<string>} Edge dep_types to hide from the graph */
export const depTypeHidden = new Set(['rig_conflict']); // default: hide rig conflict edges
/** @type {number} Max edges per node (0 = unlimited) */
export let maxEdgesPerNode = 0; // 0 = unlimited; cap edges per node to reduce hub hairballs (bd-ke2xc)
/** @param {number} v */
export function setMaxEdgesPerNode(v) {
  maxEdgesPerNode = v;
}

/**
 * Simple glob matcher: supports * (any chars) and ? (single char).
 * @param {string} pattern - Glob pattern
 * @param {string} str - String to test
 * @returns {boolean}
 */
export function globMatch(pattern, str) {
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

/**
 * Check if a text input element is focused (suppress keyboard shortcuts).
 * @returns {boolean}
 */
export function isTextInputFocused() {
  const el = document.activeElement;
  if (!el) return false;
  const tag = el.tagName;
  return tag === 'INPUT' || tag === 'TEXTAREA' || el.contentEditable === 'true';
}

// Resource cleanup refs (bd-7n4g8)
/** @type {Function|null} Bloom resize event handler ref */
export let _bloomResizeHandler = null;
/** @param {Function|null} v */
export function setBloomResizeHandler(v) {
  _bloomResizeHandler = v;
}
/** @type {number|null} Poll setInterval ID */
export let _pollIntervalId = null;
/** @param {number|null} v */
export function setPollIntervalId(v) {
  _pollIntervalId = v;
}
/** @type {number|null} Search input debounce timer ID */
export let _searchDebounceTimer = null;
/** @param {number|null} v */
export function setSearchDebounceTimer(v) {
  _searchDebounceTimer = v;
}

// Live event doots — HTML overlay elements via CSS2DRenderer (bd-bwkdk)
/** @type {Array<{css2d:Object, el:HTMLElement, node:GraphNode, birth:number, lifetime:number, jx:number, jz:number}>} Active doot animations */
export const doots = []; // { css2d, el, node, birth, lifetime, jx, jz }
/** @type {Object|null} CSS2DRenderer instance */
export let css2dRenderer = null; // CSS2DRenderer instance
/** @param {Object} v */
export function setCss2dRenderer(v) {
  css2dRenderer = v;
}

// Doot-triggered issue popups — auto-dismissing cards when doots fire (beads-edy1)
/** @type {Map<string, {el:HTMLElement, timer:number, node:GraphNode, lastDoot:number}>} Active doot popup cards by node ID */
export const dootPopups = new Map(); // nodeId → { el, timer, node, lastDoot }

// Agent windows state moved to agent-windows.js (bd-7t6nt)

// Left Sidebar state (bd-nnr22)
/** @type {boolean} Whether left sidebar is open */
export let leftSidebarOpen = false;
/** @param {boolean} v */
export function setLeftSidebarOpen(v) {
  leftSidebarOpen = v;
}
/** @type {Map<string, {status:string, task:string, tool:string, idleSince:number, crashError:string, nodeId:string}>} Agent roster by name */
export const _agentRoster = new Map(); // agent name → { status, task, tool, idleSince, crashError, nodeId }

// Epic cycling state — Shift+S/D navigation (bd-pnngb)
/** @type {GraphNode[]} Sorted array of epic nodes, rebuilt on refresh */
export let _epicNodes = []; // sorted array of epic nodes, rebuilt on refresh
/** @param {GraphNode[]} arr */
export function setEpicNodes(arr) {
  _epicNodes = arr;
}
/** @type {number} Current position in _epicNodes (-1 = none) */
export let _epicCycleIndex = -1; // current position in _epicNodes (-1 = none)
/** @param {number} v */
export function setEpicCycleIndex(v) {
  _epicCycleIndex = v;
}

// --- GPU Particle Pool + Selection VFX (bd-m9525) ---
/** @type {Object|null} GPU particle pool instance */
export let _particlePool = null; // GPU particle pool instance
/** @param {Object} p */
export function setParticlePool(p) {
  _particlePool = p;
}
/** @type {GraphNode|null} Currently hovered node for glow warmup */
export let _hoveredNode = null; // currently hovered node for glow warmup
/** @param {GraphNode|null} n */
export function setHoveredNode(n) {
  _hoveredNode = n;
}
/** @type {number} Accumulator for hover glow particle emission */
export let _hoverGlowTimer = 0; // accumulator for hover glow particle emission
/** @param {number} v */
export function setHoverGlowTimer(v) {
  _hoverGlowTimer = v;
}
/** @type {number} Accumulator for orbit ring particle emission */
export let _selectionOrbitTimer = 0; // accumulator for orbit ring particle emission
/** @param {number} v */
export function setSelectionOrbitTimer(v) {
  _selectionOrbitTimer = v;
}
/** @type {number} Accumulator for dependency energy stream particles */
export let _energyStreamTimer = 0; // accumulator for dependency energy stream particles
/** @param {number} v */
export function setEnergyStreamTimer(v) {
  _energyStreamTimer = v;
}
/** @type {boolean} Whether camera fly-to particle trail is active */
export let _flyToTrailActive = false; // true during camera fly-to for particle trail
/** @param {boolean} v */
export function setFlyToTrailActive(v) {
  _flyToTrailActive = v;
}

// --- VFX Control Panel settings (bd-hr5om) ---
/** @type {{orbitSpeed:number, orbitRate:number, orbitSize:number, hoverRate:number, hoverSize:number, streamRate:number, streamSpeed:number, particleLifetime:number, selectionGlow:number}} VFX tuning parameters */
export const _vfxConfig = {
  orbitSpeed: 2.5, // orbit ring angular speed
  orbitRate: 0.08, // orbit ring emission interval (seconds)
  orbitSize: 1.5, // orbit ring particle size
  hoverRate: 0.15, // hover glow emission interval (seconds)
  hoverSize: 1.2, // hover glow particle size
  streamRate: 0.12, // dependency energy stream emission interval (seconds)
  streamSpeed: 3.0, // energy stream particle velocity
  particleLifetime: 0.8, // base particle lifetime (seconds)
  selectionGlow: 1.0, // selection glow intensity multiplier
};

// Agent tether strength — 0 = off, 1 = max pull (bd-uzj5j)
/** @type {number} Agent tether pull strength (0=off, 1=max) */
export let _agentTetherStrength = 0.5;
/** @param {number} v */
export function setAgentTetherStrength(v) {
  _agentTetherStrength = v;
}

// --- Event sprites: pop-up animations for status changes + new associations (bd-9qeto) ---
/** @type {Array<{mesh:THREE.Mesh, birth:number, lifetime:number, type:string}>} Active event sprite animations */
export const eventSprites = []; // { mesh, birth, lifetime, type, ... }
/** @type {number} Max concurrent event sprites */
export const EVENT_SPRITE_MAX = 40;

// Status pulse colors by transition
/** @type {Object<string, number>} Hex colors for status transition pulses */
export const STATUS_PULSE_COLORS = {
  in_progress: 0xd4a017, // amber — just started
  closed: 0x2d8a4e, // green — completed
  open: 0x4a9eff, // blue — reopened
  review: 0x4a9eff, // blue
  on_ice: 0x3a5a7a, // muted blue
};

// --- SSE connection state tracking (bd-ki6im) ---
/** @type {{mutation:string, bus:string}} SSE connection status per stream */
export const _sseState = { mutation: 'connecting', bus: 'connecting' };
/** @type {number|null} Refresh debounce timer ID */
export let _refreshTimer = null;
/** @param {number|null} v */
export function setRefreshTimer(v) {
  _refreshTimer = v;
}

// --- Expanded nodes ---
/** @type {Set<string>} Node IDs with expanded detail views */
export const expandedNodes = new Set();

// --- Epic collapse/expand (kd-XGgiokgQBH) ---
/** @type {Set<string>} Epic node IDs whose children are collapsed (hidden) */
export const collapsedEpics = new Set();

/**
 * Escape HTML for safe rendering.
 * @param {string} str
 * @returns {string}
 */
export function escapeHtml(str) {
  return String(str || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}
