// SSE live updates, mutation handling, doots, doot popups (bd-03b5v, bd-ki6im, bd-c7723, bd-bwkdk, beads-edy1)
// Extracted from main.js to reduce monolith size (bd-7t6nt).

import { CSS2DRenderer, CSS2DObject } from 'three/examples/jsm/renderers/CSS2DRenderer.js';
import {
  spawnStatusPulse,
  spawnShockwave,
  spawnCollapseEffect,
  triggerClaimComet,
  intensifyAura,
  _pendingFireworks,
} from './vfx.js';

// Callbacks set by main.js to avoid circular imports
let _api = null;
let _getGraphData = null;
let _getGraph = null;
let _refresh = null;
let _handleNodeClick = null;

/**
 * Inject dependencies from main.js to avoid circular imports.
 * @param {Object} deps
 * @param {Object} deps.api - BeadsAPI instance
 * @param {Function} deps.getGraphData - Returns current graph data ({nodes, links})
 * @param {Function} deps.getGraph - Returns ForceGraph3D instance
 * @param {Function} deps.refresh - Trigger a full data refresh
 * @param {Function} deps.handleNodeClick - Callback when a node is clicked
 * @returns {void}
 */
export function setMutationDeps({ api, getGraphData, getGraph, refresh, handleNodeClick }) {
  _api = api;
  _getGraphData = getGraphData;
  _getGraph = getGraph;
  _refresh = refresh;
  _handleNodeClick = handleNodeClick;
}

function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

// --- State ---

/** @type {Array<Object>} Active doot overlay elements: { css2d, el, node, birth, lifetime, jx, jz } */
export const doots = [];

/** @type {Object|null} CSS2DRenderer instance for HTML doots, or null before init */
export let css2dRenderer = null;

/** @type {Map<string, Object>} Doot-triggered issue popups: nodeId to { el, timer, node, lastDoot } */
export const dootPopups = new Map();

// --- SSE live updates (bd-03b5v, bd-ki6im: reconnection, bd-otufd: batching) ---
// Handle incoming mutation events: optimistic property updates for instant feedback,
// debounced full refresh for structural changes (new/deleted beads).
let _refreshTimer;
let _mutationBatchTimer = null; // bd-otufd: batch mutations over 100ms window
let _mutationBatch = []; // bd-otufd: pending mutations to apply
const MUTATION_BATCH_MS = 100; // bd-otufd: batch window for coalescing mutations
// SSE connection state tracking (bd-ki6im)
const _sseState = { mutation: 'connecting', bus: 'connecting' };

function _updateConnectionStatus() {
  const statusEl = document.getElementById('status');
  if (!statusEl) return;
  const states = [_sseState.mutation, _sseState.bus];
  if (states.every((s) => s === 'connected')) {
    statusEl.textContent = 'connected';
    statusEl.className = 'connected';
  } else if (states.some((s) => s === 'disconnected')) {
    const retryBtn = statusEl.querySelector('.retry-btn');
    if (!retryBtn) {
      statusEl.innerHTML = 'disconnected <button class="retry-btn" title="Retry SSE connection">retry</button>';
      statusEl.className = 'error';
      statusEl.querySelector('.retry-btn').addEventListener('click', (e) => {
        e.stopPropagation();
        _api.reconnectAll();
      });
    }
  } else if (states.some((s) => s === 'reconnecting')) {
    const attempt = _sseState._lastAttempt || '?';
    statusEl.textContent = `reconnecting (${attempt})...`;
    statusEl.className = 'reconnecting';
  } else {
    statusEl.textContent = 'connecting...';
    statusEl.className = '';
  }
}

/**
 * Connect to the SSE mutation event stream and apply optimistic updates with debounced refresh.
 * @returns {void}
 */
export function connectLiveUpdates() {
  try {
    _api.connectEvents(
      (evt) => {
        // Batch mutations over MUTATION_BATCH_MS to coalesce rapid updates (bd-otufd)
        _mutationBatch.push(evt);
        if (!_mutationBatchTimer) {
          _mutationBatchTimer = setTimeout(() => {
            const batch = _mutationBatch;
            _mutationBatch = [];
            _mutationBatchTimer = null;
            // Deduplicate: keep last event per issue_id (latest state wins)
            const byId = new Map();
            for (const e of batch) {
              if (e.issue_id) byId.set(e.issue_id, e);
              else byId.set(Math.random(), e); // non-issue events pass through
            }
            let anyStructural = false;
            for (const e of byId.values()) {
              const applied = applyMutationOptimistic(e);
              if (!applied) anyStructural = true;
            }
            clearTimeout(_refreshTimer);
            _refreshTimer = setTimeout(_refresh, anyStructural ? 3000 : 10000);
          }, MUTATION_BATCH_MS);
        }
      },
      {
        onStatus: (state, info) => {
          _sseState.mutation = state;
          if (info.attempt) _sseState._lastAttempt = info.attempt;
          _updateConnectionStatus();
          // On reconnect, refresh to catch missed mutations (bd-ki6im)
          if (state === 'connected' && _sseState._mutationWasDown) {
            _sseState._mutationWasDown = false;
            clearTimeout(_refreshTimer);
            _refreshTimer = setTimeout(_refresh, 500);
          }
          if (state === 'reconnecting' || state === 'disconnected') {
            _sseState._mutationWasDown = true;
          }
        },
      },
    );
  } catch {
    /* polling fallback */
  }
}

/**
 * Apply a mutation event optimistically to the in-memory graph data.
 * Returns true if a visual update was applied (no urgent refresh needed).
 * @param {Object} evt - Mutation event with type, issue_id, and event-specific fields
 * @returns {boolean} Whether a visual update was applied
 */
export function applyMutationOptimistic(evt) {
  const graphData = _getGraphData();
  const graph = _getGraph();
  if (!graphData || !graphData.nodes) return false;

  const id = evt.issue_id;
  if (!id) return false;

  // Find the node in the current graph
  const node = graphData.nodes.find((n) => n.id === id);

  switch (evt.type) {
    case 'status': {
      if (!node) return false;
      const oldStatus = node.status;
      node.status = evt.new_status || node.status;
      // Rebuild THREE.js object if status changed (affects color, pulse ring, etc.)
      if (oldStatus !== node.status && graph) {
        graph.nodeThreeObject(graph.nodeThreeObject());
        // Event sprite: status change pulse (bd-9qeto) + shockwave ring (bd-3fnon)
        spawnStatusPulse(node, oldStatus, node.status);
        spawnShockwave(node, oldStatus, node.status);
        // Implosion/collapse effect on close (bd-1n122)
        if (node.status === 'closed') spawnCollapseEffect(node);
      }
      return true;
    }

    case 'update': {
      if (!node) return false;
      // Update assignee if present in the event
      if (evt.assignee !== undefined) {
        const oldAssignee = node.assignee;
        node.assignee = evt.assignee;
        // Comet trail: agent claiming a bead (bd-t4umc)
        if (evt.assignee && evt.assignee !== oldAssignee) {
          triggerClaimComet(node, evt.assignee);
        }
      }
      if (evt.title) {
        node.title = evt.title;
      }
      // Intensify aura on any update to in-progress bead (bd-ttet4)
      if (node.status === 'in_progress') intensifyAura(node.id);
      return true;
    }

    case 'create':
      // New bead — can't render without full data, need refresh.
      // Record the ID for firework burst after refresh places the node. (bd-4gmot)
      if (evt.issue_id) _pendingFireworks.add(evt.issue_id);
      return false;

    case 'delete':
      // Deleted bead — could remove from graph, but safer to let refresh handle it
      return false;

    default:
      return false;
  }
}

/**
 * Update the SSE bus connection state and refresh the status indicator.
 * Called from connectBusStream in main.js.
 * @param {string} state - Connection state ('connecting', 'connected', 'reconnecting', 'disconnected')
 * @param {Object} info - Additional info, e.g. { attempt: number }
 * @returns {void}
 */
export function updateBusConnectionState(state, info) {
  _sseState.bus = state;
  if (info && info.attempt) _sseState._lastAttempt = info.attempt;
  _updateConnectionStatus();
  // On reconnect, refresh to catch missed events (bd-ki6im)
  if (state === 'connected' && _sseState._busWasDown) {
    _sseState._busWasDown = false;
    clearTimeout(_refreshTimer);
    _refreshTimer = setTimeout(_refresh, 500);
  }
  if (state === 'reconnecting' || state === 'disconnected') {
    _sseState._busWasDown = true;
  }
}

// --- Live event doots — HTML overlay via CSS2DRenderer (bd-c7723, bd-bwkdk) ---

const DOOT_LIFETIME = 4.0; // seconds before fully faded
const DOOT_RISE_SPEED = 8; // units per second upward
const DOOT_MAX = 30; // max active doots (oldest get pruned)
let _dootLastSpawn = 0; // timestamp of last doot spawn (bd-otufd throttle)
const DOOT_MIN_INTERVAL = 50; // minimum ms between doot spawns under load

/**
 * Initialize the CSS2D overlay renderer for HTML doots and append it to the graph element.
 * @returns {void}
 */
export function initCSS2DRenderer() {
  css2dRenderer = new CSS2DRenderer();
  css2dRenderer.setSize(window.innerWidth, window.innerHeight);
  css2dRenderer.domElement.id = 'css2d-overlay';
  css2dRenderer.domElement.style.position = 'absolute';
  css2dRenderer.domElement.style.top = '0';
  css2dRenderer.domElement.style.left = '0';
  css2dRenderer.domElement.style.pointerEvents = 'none';
  css2dRenderer.domElement.style.zIndex = '1';
  document.getElementById('graph').appendChild(css2dRenderer.domElement);
  window.addEventListener('resize', () => {
    css2dRenderer.setSize(window.innerWidth, window.innerHeight);
  });
}

/**
 * Find a graph node to attach a doot to for a bus event.
 * Pass 1: Prefer dedicated agent nodes (issue_type=agent).
 * Pass 2: Fall back to any visible node by issue_id or assignee.
 * @param {Object} evt - Bus event with type and payload fields
 * @returns {Object|null} The matching graph node, or null if not found
 */
export function findAgentNode(evt) {
  const graphData = _getGraphData();
  const p = evt.payload || {};

  // Mail events: find recipient agent node (bd-t76aw, bd-gal6f: prefer visible)
  if (evt.type === 'MailSent' || evt.type === 'MailRead') {
    const to = (p.to || '').replace(/^@/, '');
    if (to && graphData) {
      const visible = graphData.nodes.filter((n) => n.issue_type === 'agent' && !n._hidden);
      for (const node of visible) {
        if (node.title === to || node.id === `agent:${to}`) return node;
      }
      // Fall back to hidden agents
      const hidden = graphData.nodes.filter((n) => n.issue_type === 'agent' && n._hidden);
      for (const node of hidden) {
        if (node.title === to || node.id === `agent:${to}`) return node;
      }
    }
    return null;
  }

  const candidates = [
    p.issue_id, // mutation events: the bead being mutated
    p.agent_id, // agent lifecycle events
    p.agentID, // alternate casing
    p.assignee, // mutation events: the agent assigned to the bead
    p.requested_by, // decision events: requesting agent (bd-0j7hr)
    p.actor, // hook events (short agent name) or mutations ("daemon")
  ].filter((c) => c && c !== 'daemon');

  if (candidates.length === 0) return null;

  // Pass 1a: Prefer VISIBLE agent nodes (bd-gal6f: avoid doots on hidden agents)
  const visibleAgents = graphData.nodes.filter((n) => n.issue_type === 'agent' && !n._hidden);
  for (const candidate of candidates) {
    for (const node of visibleAgents) {
      if (node.id === candidate || node.title === candidate || node.assignee === candidate) return node;
    }
    for (const node of visibleAgents) {
      if (node.id === `agent:${candidate}`) return node;
    }
  }
  // Pass 1b: Fall back to hidden agent nodes (still better than random bead)
  const allAgents = graphData.nodes.filter((n) => n.issue_type === 'agent' && n._hidden);
  for (const candidate of candidates) {
    for (const node of allAgents) {
      if (node.id === candidate || node.title === candidate || node.assignee === candidate) return node;
    }
    for (const node of allAgents) {
      if (node.id === `agent:${candidate}`) return node;
    }
  }

  // Pass 2: Fall back to any visible node (bd-5knqx live doot fix)
  const allVisible = graphData.nodes.filter((n) => !n._hidden);
  if (p.issue_id) {
    const byId = allVisible.find((n) => n.id === p.issue_id);
    if (byId) return byId;
  }
  const actor = p.actor;
  if (actor && actor !== 'daemon') {
    const byAssignee = allVisible.find((n) => n.assignee === actor && n.status === 'in_progress');
    if (byAssignee) return byAssignee;
  }
  return null;
}

/**
 * Spawn an HTML doot (floating text label) via CSS2DObject above a graph node.
 * @param {Object} node - Graph node to anchor the doot to
 * @param {string} text - Text content to display in the doot
 * @param {string} color - CSS color string for the doot text
 * @returns {void}
 */
export function spawnDoot(node, text, color) {
  const graph = _getGraph();
  if (!node || !text || !graph) return;

  // Throttle doot spawning under burst load — skip if too rapid (bd-otufd)
  const now = performance.now();
  if (now - _dootLastSpawn < DOOT_MIN_INTERVAL) return;
  _dootLastSpawn = now;

  // Trigger doot popup for non-agent nodes (beads-edy1)
  showDootPopup(node);

  // Create HTML element for the doot
  const el = document.createElement('div');
  el.className = 'doot-text';
  el.textContent = text;
  el.style.color = color || '#ff6b35';
  el.style.setProperty('--doot-color', color || '#ff6b35');

  // Wrap in CSS2DObject for 3D positioning
  const css2d = new CSS2DObject(el);
  css2d.userData.isDoot = true;

  // Random horizontal jitter so overlapping doots spread out
  const jx = (Math.random() - 0.5) * 6;
  const jz = (Math.random() - 0.5) * 6;
  css2d.position.set(
    (node.x || 0) + jx,
    (node.y || 0) + 10, // start just above node
    (node.z || 0) + jz,
  );
  graph.scene().add(css2d);

  doots.push({
    css2d,
    el,
    node,
    birth: performance.now() / 1000,
    lifetime: DOOT_LIFETIME,
    jx,
    jz,
  });

  // Prune oldest if over limit
  while (doots.length > DOOT_MAX) {
    const old = doots.shift();
    graph.scene().remove(old.css2d);
  }
}

/**
 * Update doot positions (rising animation) and opacity (fade out) each frame.
 * CSS2DRenderer handles screen projection; this updates world Y for rising.
 * @param {number} t - Current time in seconds (from performance.now() / 1000)
 * @returns {void}
 */
export function updateDoots(t) {
  const graph = _getGraph?.();
  for (let i = doots.length - 1; i >= 0; i--) {
    const d = doots[i];
    const age = t - d.birth;

    if (age > d.lifetime) {
      // Remove expired doot
      if (graph) graph.scene().remove(d.css2d);
      doots.splice(i, 1);
      continue;
    }

    // Rise upward, follow node position (nodes can move during force layout)
    const rise = age * DOOT_RISE_SPEED;
    d.css2d.position.set((d.node.x || 0) + d.jx, (d.node.y || 0) + 10 + rise, (d.node.z || 0) + d.jz);

    // Fade out over last 40% of lifetime — only update DOM when value changes (bd-kkd9y)
    const fadeStart = d.lifetime * 0.6;
    const opacity = age < fadeStart ? 0.9 : 0.9 * (1 - (age - fadeStart) / (d.lifetime - fadeStart));
    const opStr = Math.max(0, opacity).toFixed(2);
    if (d._lastOpacity !== opStr) {
      d.el.style.opacity = opStr;
      d._lastOpacity = opStr;
    }
  }
}

// --- Doot-triggered issue popup (beads-edy1) ---
const DOOT_POPUP_DURATION = 30000; // 30s auto-dismiss
const DOOT_POPUP_MAX = 3; // max simultaneous popups

/**
 * Show (or refresh) an auto-dismissing popup card for a node when a doot fires.
 * @param {Object} node - Graph node to show the popup for
 * @returns {void}
 */
export function showDootPopup(node) {
  if (!node || !node.id || node.issue_type === 'agent') return;

  const existing = dootPopups.get(node.id);
  if (existing) {
    // Reset timer — activity keeps it alive
    clearTimeout(existing.timer);
    existing.timer = setTimeout(() => dismissDootPopup(node.id), DOOT_POPUP_DURATION);
    existing.lastDoot = Date.now();
    // Pulse animation
    existing.el.classList.remove('doot-pulse');
    void existing.el.offsetWidth; // force reflow
    existing.el.classList.add('doot-pulse');
    return;
  }

  // Prune oldest if at max
  if (dootPopups.size >= DOOT_POPUP_MAX) {
    const oldest = [...dootPopups.entries()].sort((a, b) => a[1].lastDoot - b[1].lastDoot)[0];
    if (oldest) dismissDootPopup(oldest[0]);
  }

  // Create popup element
  const container = document.getElementById('doot-popups') || createDootPopupContainer();
  const el = document.createElement('div');
  el.className = 'doot-popup';
  el.dataset.beadId = node.id;

  const pLabel = ['P0', 'P1', 'P2', 'P3', 'P4'][node.priority] || '';
  el.innerHTML = `
    <div class="doot-popup-header">
      <span class="doot-popup-id">${escapeHtml(node.id)}</span>
      <span class="tag tag-${node.status}">${node.status}</span>
      <span class="tag">${pLabel}</span>
      <button class="doot-popup-close">&times;</button>
    </div>
    <div class="doot-popup-title">${escapeHtml(node.title || node.id)}</div>
    ${node.assignee ? `<div class="doot-popup-assignee">@ ${escapeHtml(node.assignee)}</div>` : ''}
    <div class="doot-popup-bar"></div>
  `;

  el.querySelector('.doot-popup-close').onclick = () => dismissDootPopup(node.id);
  el.onclick = (e) => {
    if (e.target.classList.contains('doot-popup-close')) return;
    _handleNodeClick(node);
  };

  container.appendChild(el);
  requestAnimationFrame(() => el.classList.add('open'));

  const timer = setTimeout(() => dismissDootPopup(node.id), DOOT_POPUP_DURATION);
  dootPopups.set(node.id, { el, timer, node, lastDoot: Date.now() });

  // Start countdown bar animation
  const bar = el.querySelector('.doot-popup-bar');
  if (bar) bar.style.animationDuration = `${DOOT_POPUP_DURATION}ms`;
}

/**
 * Dismiss and remove a doot popup by node ID.
 * @param {string} nodeId - The node ID whose popup to dismiss
 * @returns {void}
 */
export function dismissDootPopup(nodeId) {
  const popup = dootPopups.get(nodeId);
  if (!popup) return;
  clearTimeout(popup.timer);
  popup.el.classList.remove('open');
  dootPopups.delete(nodeId);
  setTimeout(() => popup.el.remove(), 300);
}

function createDootPopupContainer() {
  const c = document.createElement('div');
  c.id = 'doot-popups';
  document.body.appendChild(c);
  return c;
}
