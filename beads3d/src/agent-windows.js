// Agent activity feed windows — rich session transcripts, tabbed overlay, unified feed
// (bd-kau4k, bd-7h9sd, bd-bwi52, bd-jgvas, bd-9ndk0.3, bd-5ok9s, bd-7t6nt)
// Extracted from main.js to reduce monolith size.

import { rigColor } from './colors.js';
import { formatToolLabel, TOOL_ICONS } from './event-format.js';
import { formatDuration, escapeStatusText } from './left-sidebar.js';

// Callbacks set by main.js to avoid circular imports
let _api = null;
let _getGraphData = null;
let _handleNodeClick = null;

/**
 * Inject dependencies from main.js to avoid circular imports.
 * @param {Object} deps
 * @param {Object} deps.api - BeadsAPI instance
 * @param {Function} deps.getGraphData - Returns current graph data ({nodes, links})
 * @param {Function} deps.handleNodeClick - Callback when a node is clicked
 * @returns {void}
 */
export function setAgentWindowDeps({ api, getGraphData, handleNodeClick }) {
  _api = api;
  _getGraphData = getGraphData;
  _handleNodeClick = handleNodeClick;
}

function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

// --- State ---

const AGENT_WINDOW_MAX = 20; // max simultaneous agent windows (bd-kkd9y)
const AGENT_FEED_MAX = 200; // max entries per agent feed (bd-kkd9y)

/** @type {Map<string, Object>} Map of agent ID to agent window state objects */
export const agentWindows = new Map();
let agentsViewOpen = false;
let _selectedAgentTab = null; // currently selected agent ID in tabbed view

/**
 * Returns whether the agents overlay view is currently open.
 * @returns {boolean}
 */
export function getAgentsViewOpen() {
  return agentsViewOpen;
}

/**
 * Returns the ID of the currently selected agent tab, or null if none.
 * @returns {string|null}
 */
export function getSelectedAgentTab() {
  return _selectedAgentTab;
}

// --- Assigned beads helpers (bd-tgg70) ---

function getAssignedBeads(agentId) {
  const graphData = _getGraphData();
  if (!graphData || !graphData.links) return [];
  return graphData.links
    .filter((l) => l.dep_type === 'assigned_to' && (typeof l.source === 'object' ? l.source.id : l.source) === agentId)
    .map((l) => {
      const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      const tgtNode = graphData.nodes.find((n) => n.id === tgtId);
      return tgtNode ? { id: tgtId, title: tgtNode.title || tgtId } : null;
    })
    .filter(Boolean);
}

function renderBeadsListHtml(assigned) {
  return assigned
    .map(
      (b) =>
        `<div class="agent-window-bead" data-bead-id="${escapeHtml(b.id)}" title="${escapeHtml(b.id)}: ${escapeHtml(b.title)}" style="cursor:pointer">${escapeHtml(b.id.replace(/^[a-z]+-/, ''))}: ${escapeHtml(b.title)}</div>`,
    )
    .join('');
}

/**
 * Refresh the assigned beads list for all open agent windows.
 * @returns {void}
 */
export function refreshAgentWindowBeads() {
  const graphData = _getGraphData();
  for (const [agentId, win] of agentWindows) {
    const assigned = getAssignedBeads(agentId);
    const beadsList = renderBeadsListHtml(assigned);
    const badges = win.el.querySelectorAll('.agent-window-header .agent-window-badge');
    for (const b of badges) {
      if (!b.style.color) {
        b.textContent = assigned.length;
        break;
      }
    }
    let beadsContainer = win.el.querySelector('.agent-window-beads');
    if (beadsList) {
      if (!beadsContainer) {
        beadsContainer = document.createElement('div');
        beadsContainer.className = 'agent-window-beads';
        const feed = win.el.querySelector('.agent-feed');
        if (feed) win.el.insertBefore(beadsContainer, feed);
      }
      beadsContainer.innerHTML = beadsList;
      beadsContainer.onclick = (e) => {
        const beadEl = e.target.closest('.agent-window-bead');
        if (!beadEl) return;
        const beadId = beadEl.dataset.beadId;
        if (!beadId) return;
        e.stopPropagation();
        if (graphData) {
          const beadNode = graphData.nodes.find((n) => n.id === beadId);
          if (beadNode) _handleNodeClick(beadNode);
        }
      };
    } else if (beadsContainer) {
      beadsContainer.remove();
    }
  }
}

// --- Agent window creation (bd-7h9sd, bd-kau4k) ---

/**
 * Show (or focus) an agent window for the given graph node.
 * Creates the window DOM element and registers it if not already open.
 * @param {Object} node - Graph node representing the agent
 * @returns {void}
 */
export function showAgentWindow(node) {
  if (!node || !node.id) return;
  if (agentWindows.has(node.id)) {
    toggleAgentDropdown(node.id);
    return;
  }

  // Evict oldest idle/crashed windows if at capacity (bd-kkd9y)
  if (agentWindows.size >= AGENT_WINDOW_MAX) {
    // Prefer evicting crashed, then idle, then oldest
    let victim = null;
    for (const [id, w] of agentWindows) {
      if (w.lastStatus === 'crashed') { victim = id; break; }
      if (!victim && w.lastStatus === 'idle') victim = id;
    }
    if (!victim) victim = agentWindows.keys().next().value; // oldest
    if (victim) closeAgentWindow(victim);
  }

  const el = document.createElement('div');
  el.className = 'agent-window';
  el.dataset.agentId = node.id;
  el.style.display = 'none';
  const agentName = node.title || node.id.replace('agent:', '');
  const initStatus = (node.status || '').toLowerCase();
  const initStatusClass =
    initStatus === 'active'
      ? 'status-active'
      : initStatus === 'idle'
        ? 'status-idle'
        : initStatus === 'crashed'
          ? 'status-crashed'
          : '';
  el.innerHTML = `
    <div class="agent-window-header">
      <span class="agent-window-name">${escapeHtml(agentName)}</span>
    </div>
    <div class="agent-status-bar">
      <span><span class="status-label">Status:</span> <span class="agent-status-state ${initStatusClass}">${initStatus || '?'}</span></span>
      <span class="agent-status-idle-dur"></span>
      <span class="agent-status-tool"></span>
    </div>
    <div class="agent-feed"><div class="agent-window-empty">waiting for events...</div></div>
    <div class="agent-mail-compose">
      <input class="agent-mail-input" type="text" placeholder="Send message to ${escapeHtml(agentName)}..." />
      <button class="agent-mail-send">&#x2709;</button>
    </div>
  `;
  const container = document.getElementById('agent-windows');
  if (container) container.appendChild(el);
  const feedEl = el.querySelector('.agent-feed');
  const statusEl = el.querySelector('.agent-status-bar');
  agentWindows.set(node.id, {
    el,
    feedEl,
    statusEl,
    node,
    entries: [],
    pendingTool: null,
    collapsed: false,
    lastStatus: initStatus || null,
    lastTool: null,
    idleSince: initStatus === 'idle' ? Date.now() : null,
    crashError: null,
  });
  _addBottomTrayChip(node);
}

let _activeDropdownAgent = null;

function _addBottomTrayChip(node) {
  const topBar = document.getElementById('agent-top-bar');
  if (!topBar) return;
  const agentName = node.title || node.id.replace('agent:', '');
  const status = (node.status || '').toLowerCase();
  const dotColor =
    status === 'active' ? '#2d8a4e' : status === 'idle' ? '#d4a017' : status === 'crashed' ? '#d04040' : '#666';
  const chip = document.createElement('button');
  chip.className = 'agent-tray-chip';
  chip.dataset.agentId = node.id;
  chip.innerHTML = `<span class="agent-tray-dot" style="background:${dotColor}"></span>${escapeHtml(agentName)}`;
  chip.onclick = (e) => {
    e.stopPropagation();
    toggleAgentDropdown(node.id);
  };
  topBar.appendChild(chip);
}

/**
 * Toggle the dropdown panel for a specific agent.
 * @param {string} agentId - The agent ID to show/hide
 * @returns {void}
 */
export function toggleAgentDropdown(agentId) {
  const dropdown = document.getElementById('agent-dropdown');
  if (!dropdown) return;
  // If clicking same agent, close dropdown
  if (_activeDropdownAgent === agentId && dropdown.classList.contains('open')) {
    closeAgentDropdown();
    return;
  }
  // Position dropdown near the chip
  const chip = document.querySelector(`.agent-tray-chip[data-agent-id="${agentId}"]`);
  if (chip) {
    const rect = chip.getBoundingClientRect();
    // Align right edge of dropdown with right edge of chip, but keep on screen
    const rightEdge = Math.min(rect.right, window.innerWidth - 12);
    const leftEdge = Math.max(12, rightEdge - 480);
    dropdown.style.left = leftEdge + 'px';
    dropdown.style.right = 'auto';
    dropdown.style.width = Math.min(480, window.innerWidth - 24) + 'px';
  }
  // Move the agent window element into the dropdown
  const win = agentWindows.get(agentId);
  if (!win) return;
  // Remove any previous window from dropdown
  dropdown.innerHTML = '';
  win.el.style.display = '';
  win.el.classList.remove('collapsed');
  dropdown.appendChild(win.el);
  dropdown.classList.add('open');
  // Update chip active states
  for (const c of document.querySelectorAll('.agent-tray-chip')) {
    c.classList.toggle('active', c.dataset.agentId === agentId);
  }
  _activeDropdownAgent = agentId;
  autoScroll(win);
  // Close on click outside
  setTimeout(() => {
    document.addEventListener('click', _closeDropdownOnClickOutside, { once: true });
  }, 0);
}

function _closeDropdownOnClickOutside(e) {
  const dropdown = document.getElementById('agent-dropdown');
  const topBar = document.getElementById('agent-top-bar');
  if (!dropdown) return;
  if (dropdown.contains(e.target) || (topBar && topBar.contains(e.target))) {
    // Re-attach listener since we're inside
    document.addEventListener('click', _closeDropdownOnClickOutside, { once: true });
    return;
  }
  closeAgentDropdown();
}

/**
 * Close the agent dropdown panel.
 * @returns {void}
 */
export function closeAgentDropdown() {
  const dropdown = document.getElementById('agent-dropdown');
  if (!dropdown) return;
  // Move window back to hidden container
  if (_activeDropdownAgent) {
    const win = agentWindows.get(_activeDropdownAgent);
    if (win) {
      const hiddenContainer = document.getElementById('agent-windows');
      if (hiddenContainer) {
        win.el.style.display = 'none';
        hiddenContainer.appendChild(win.el);
      }
    }
  }
  dropdown.classList.remove('open');
  dropdown.innerHTML = '';
  for (const c of document.querySelectorAll('.agent-tray-chip')) {
    c.classList.remove('active');
  }
  _activeDropdownAgent = null;
}

/**
 * Close and remove an agent window by agent ID.
 * @param {string} agentId - The agent ID whose window to close
 * @returns {void}
 */
export function closeAgentWindow(agentId) {
  if (_activeDropdownAgent === agentId) closeAgentDropdown();
  const win = agentWindows.get(agentId);
  if (!win) return;
  if (win._dragCleanup) {
    win._dragCleanup();
    win._dragCleanup = null;
  }
  win.el.remove();
  agentWindows.delete(agentId);
  const chip = document.querySelector(`.agent-tray-chip[data-agent-id="${agentId}"]`);
  if (chip) chip.remove();
}

// --- Pop-out / dock-back (bd-dqe6k) ---

/**
 * Toggle an agent window between docked and popped-out (floating) state.
 * @param {string} agentId - The agent ID whose window to toggle
 * @returns {void}
 */
export function togglePopout(agentId) {
  const win = agentWindows.get(agentId);
  if (!win) return;
  const el = win.el;
  const btn = el.querySelector('.agent-window-popout');
  if (el.classList.contains('popped-out')) {
    el.classList.remove('popped-out');
    el.style.left = '';
    el.style.top = '';
    el.style.width = '';
    el.style.height = '';
    btn.innerHTML = '&#x2197;';
    btn.title = 'Pop out to floating window';
    const tray = document.getElementById('agent-windows');
    const tabContent = document.querySelector('.agents-tab-content');
    if (tabContent && document.getElementById('agents-view')?.classList.contains('open')) {
      tabContent.appendChild(el);
    } else if (tray) {
      tray.appendChild(el);
    }
    if (win._dragCleanup) {
      win._dragCleanup();
      win._dragCleanup = null;
    }
  } else {
    const rect = el.getBoundingClientRect();
    el.classList.add('popped-out');
    el.style.left = Math.min(rect.left, window.innerWidth - 440) + 'px';
    el.style.top = Math.max(20, rect.top - 60) + 'px';
    btn.innerHTML = '&#x2199;';
    btn.title = 'Dock back to tray';
    document.body.appendChild(el);
    win._dragCleanup = enableHeaderDrag(el);
  }
}

function enableHeaderDrag(el) {
  const header = el.querySelector('.agent-window-header');
  let dragging = false,
    startX = 0,
    startY = 0,
    origLeft = 0,
    origTop = 0;
  function onMouseDown(e) {
    if (e.target.closest('button') || e.target.closest('.agent-window-name')) return;
    dragging = true;
    startX = e.clientX;
    startY = e.clientY;
    origLeft = parseInt(el.style.left) || 0;
    origTop = parseInt(el.style.top) || 0;
    e.preventDefault();
  }
  function onMouseMove(e) {
    if (!dragging) return;
    el.style.left = origLeft + e.clientX - startX + 'px';
    el.style.top = origTop + e.clientY - startY + 'px';
  }
  function onMouseUp() {
    dragging = false;
  }
  header.addEventListener('mousedown', onMouseDown);
  document.addEventListener('mousemove', onMouseMove);
  document.addEventListener('mouseup', onMouseUp);
  return () => {
    header.removeEventListener('mousedown', onMouseDown);
    document.removeEventListener('mousemove', onMouseMove);
    document.removeEventListener('mouseup', onMouseUp);
  };
}

/**
 * Enable top-edge resize handle on an agent window element.
 * @param {HTMLElement} el - The agent window DOM element to enable resizing on
 * @returns {void}
 */
export function enableTopResize(el) {
  const handle = el.querySelector('.agent-window-resize-handle');
  if (!handle) return;
  let resizing = false,
    startY = 0,
    origHeight = 0;
  handle.addEventListener('mousedown', (e) => {
    resizing = true;
    startY = e.clientY;
    origHeight = el.offsetHeight;
    handle.classList.add('active');
    el.style.transition = 'none';
    e.preventDefault();
  });
  document.addEventListener('mousemove', (e) => {
    if (!resizing) return;
    el.style.height = Math.max(100, Math.min(window.innerHeight - 40, origHeight + startY - e.clientY)) + 'px';
  });
  document.addEventListener('mouseup', () => {
    if (!resizing) return;
    resizing = false;
    handle.classList.remove('active');
    el.style.transition = '';
  });
}

// --- Agents View overlay (bd-jgvas, bd-bwi52) ---

/**
 * Toggle the agents overlay view open or closed.
 * @returns {void}
 */
export function toggleAgentsView() {
  if (agentsViewOpen) closeAgentsView();
  else openAgentsView();
}

/**
 * Open the agents overlay view, populating tabs for all visible agent nodes.
 * @returns {void}
 */
export function openAgentsView() {
  const overlay = document.getElementById('agents-view');
  const graphData = _getGraphData();
  if (!overlay || !graphData) return;
  const agentNodes = graphData.nodes.filter((n) => n.issue_type === 'agent' && !n._hidden);
  if (agentNodes.length === 0) return;
  const statusOrder = { active: 0, idle: 1 };
  agentNodes.sort((a, b) => {
    const sa = statusOrder[a.status] ?? 2;
    const sb = statusOrder[b.status] ?? 2;
    if (sa !== sb) return sa - sb;
    return (a.title || a.id).localeCompare(b.title || b.id);
  });
  const counts = { active: 0, idle: 0, crashed: 0 };
  for (const n of agentNodes) {
    if (n.status === 'active') counts.active++;
    else if (n.status === 'idle') counts.idle++;
    else if (n.status === 'crashed') counts.crashed++;
  }
  overlay.innerHTML = `
    <div class="agents-view-header">
      <span class="agents-view-title">AGENTS</span>
      <input class="agents-view-search" type="text" placeholder="filter agents..." />
      <div class="agents-view-stats">
        <span class="active">${counts.active} active</span>
        <span class="idle">${counts.idle} idle</span>
        ${counts.crashed ? `<span class="crashed">${counts.crashed} crashed</span>` : ''}
        <span>${agentNodes.length} total</span>
      </div>
      <button class="agents-view-close">ESC close</button>
    </div>
    <div class="agents-tab-bar"></div>
    <div class="agents-tab-content"></div>
  `;
  overlay.querySelector('.agents-view-close').onclick = () => closeAgentsView();
  const searchEl = overlay.querySelector('.agents-view-search');
  searchEl.addEventListener('input', () => {
    const q = searchEl.value.toLowerCase().trim();
    const tabBar = overlay.querySelector('.agents-tab-bar');
    if (!tabBar) return;
    for (const tab of tabBar.children) {
      const name = (tab.dataset.agentName || '').toLowerCase();
      tab.style.display = !q || name.includes(q) ? '' : 'none';
    }
  });
  setTimeout(() => searchEl.focus(), 100);
  const tabBar = overlay.querySelector('.agents-tab-bar');
  const contentArea = overlay.querySelector('.agents-tab-content');
  for (const node of agentNodes) {
    _ensureAgentWindow(node, contentArea);
    const agentName = node.title || node.id.replace('agent:', '');
    const agentStatus = (node.status || '').toLowerCase();
    const tab = document.createElement('button');
    tab.className = 'agent-tab';
    tab.dataset.agentId = node.id;
    tab.dataset.agentName = agentName;
    const dotColor =
      agentStatus === 'active'
        ? '#2d8a4e'
        : agentStatus === 'idle'
          ? '#d4a017'
          : agentStatus === 'crashed'
            ? '#d04040'
            : '#666';
    tab.innerHTML = `<span class="agent-tab-dot" style="background:${dotColor}"></span>${escapeHtml(agentName)}`;
    tab.onclick = () => selectAgentTab(node.id);
    tabBar.appendChild(tab);
  }
  const firstId =
    _selectedAgentTab && agentNodes.find((n) => n.id === _selectedAgentTab) ? _selectedAgentTab : agentNodes[0].id;
  selectAgentTab(firstId);
  overlay.classList.add('open');
  agentsViewOpen = true;
}

function _ensureAgentWindow(node, contentArea) {
  const existing = agentWindows.get(node.id);
  if (existing) {
    existing.collapsed = false;
    existing.el.classList.remove('collapsed');
    contentArea.appendChild(existing.el);
    return;
  }
  const graphData = _getGraphData();
  const el = document.createElement('div');
  el.className = 'agent-window';
  el.dataset.agentId = node.id;
  const agentName = node.title || node.id.replace('agent:', '');
  const agentStatus = (node.status || '').toLowerCase();
  const statusColor =
    agentStatus === 'active'
      ? '#2d8a4e'
      : agentStatus === 'idle'
        ? '#d4a017'
        : agentStatus === 'crashed'
          ? '#d04040'
          : '#666';
  const assigned = getAssignedBeads(node.id);
  const beadsList = renderBeadsListHtml(assigned);
  const avRigBadge = node.rig
    ? `<span class="agent-window-rig" style="color:${rigColor(node.rig)};border-color:${rigColor(node.rig)}33">${escapeHtml(node.rig)}</span>`
    : '';
  const avStatusClass =
    agentStatus === 'active'
      ? 'status-active'
      : agentStatus === 'idle'
        ? 'status-idle'
        : agentStatus === 'crashed'
          ? 'status-crashed'
          : '';
  el.innerHTML = `
    <div class="agent-window-header">
      <span class="agent-window-name" style="cursor:pointer" title="Click to zoom to agent">${escapeHtml(agentName)}</span>
      ${avRigBadge}
      <span class="agent-window-badge" style="color:${statusColor}">${agentStatus || '?'}</span>
      <span class="agent-window-badge">${assigned.length}</span>
    </div>
    <div class="agent-status-bar">
      <span><span class="status-label">Status:</span> <span class="agent-status-state ${avStatusClass}">${agentStatus || '?'}</span></span>
      <span class="agent-status-idle-dur"></span>
      <span class="agent-status-tool"></span>
    </div>
    ${beadsList ? `<div class="agent-window-beads">${beadsList}</div>` : ''}
    <div class="agent-feed"><div class="agent-window-empty">waiting for events...</div></div>
    <div class="agent-mail-compose">
      <input class="agent-mail-input" type="text" placeholder="Send message to ${escapeHtml(agentName)}..." />
      <button class="agent-mail-send">&#x2709;</button>
    </div>
  `;
  el.querySelector('.agent-window-name').onclick = (e) => {
    e.stopPropagation();
    if (graphData) {
      const agentNode = graphData.nodes.find((n) => n.id === node.id);
      if (agentNode) _handleNodeClick(agentNode);
    }
  };
  const beadsContainer = el.querySelector('.agent-window-beads');
  if (beadsContainer) {
    beadsContainer.onclick = (e) => {
      const beadEl = e.target.closest('.agent-window-bead');
      if (!beadEl) return;
      const beadId = beadEl.dataset.beadId;
      if (!beadId) return;
      e.stopPropagation();
      if (graphData) {
        const beadNode = graphData.nodes.find((n) => n.id === beadId);
        if (beadNode) _handleNodeClick(beadNode);
      }
    };
  }
  const mailInput = el.querySelector('.agent-mail-input');
  const mailSend = el.querySelector('.agent-mail-send');
  const doSend = async () => {
    const text = mailInput.value.trim();
    if (!text) return;
    mailInput.value = '';
    mailInput.disabled = true;
    mailSend.disabled = true;
    try {
      await _api.sendMail(agentName, text);
      const win = agentWindows.get(node.id);
      if (win) {
        const empty = win.feedEl.querySelector('.agent-window-empty');
        if (empty) empty.remove();
        const ts = new Date().toTimeString().slice(0, 8);
        win.feedEl.appendChild(createEntry(ts, '\u25b6', `sent: ${text}`, 'mail mail-sent'));
        autoScroll(win);
      }
    } catch (err) {
      console.error('[beads3d] mail send failed:', err);
      mailInput.value = text;
      const compose = el.querySelector('.agent-mail-compose');
      compose.classList.add('send-error');
      setTimeout(() => compose.classList.remove('send-error'), 2000);
    }
    mailInput.disabled = false;
    mailSend.disabled = false;
    mailInput.focus();
  };
  mailSend.onclick = doSend;
  mailInput.onkeydown = (e) => {
    if (e.key === 'Enter') doSend();
  };
  contentArea.appendChild(el);
  const feedEl = el.querySelector('.agent-feed');
  const statusEl = el.querySelector('.agent-status-bar');
  agentWindows.set(node.id, {
    el,
    feedEl,
    statusEl,
    node,
    entries: [],
    pendingTool: null,
    collapsed: false,
    lastStatus: agentStatus || null,
    lastTool: null,
    idleSince: agentStatus === 'idle' ? Date.now() : null,
    crashError: null,
  });
}

/**
 * Select and display the agent tab for the given agent ID in the overlay.
 * @param {string} agentId - The agent ID to select
 * @returns {void}
 */
export function selectAgentTab(agentId) {
  _selectedAgentTab = agentId;
  const overlay = document.getElementById('agents-view');
  if (!overlay) return;
  const tabs = overlay.querySelectorAll('.agent-tab');
  for (const tab of tabs) {
    const isActive = tab.dataset.agentId === agentId;
    tab.classList.toggle('active', isActive);
    if (isActive) {
      const badge = tab.querySelector('.agent-tab-unread');
      if (badge) badge.remove();
    }
  }
  const contentArea = overlay.querySelector('.agents-tab-content');
  if (contentArea) {
    for (const child of contentArea.children) {
      child.style.display = child.dataset.agentId === agentId ? '' : 'none';
    }
  }
  const win = agentWindows.get(agentId);
  if (win) autoScroll(win);
}

/**
 * Update the status indicator dot color for an agent's tab and tray chip.
 * @param {string} agentId - The agent ID to update
 * @param {string} status - The new status ('active', 'idle', 'crashed', etc.)
 * @returns {void}
 */
export function _updateTabStatus(agentId, status) {
  const color =
    status === 'active' ? '#2d8a4e' : status === 'idle' ? '#d4a017' : status === 'crashed' ? '#d04040' : '#666';
  if (agentsViewOpen) {
    const overlay = document.getElementById('agents-view');
    if (overlay) {
      const dot = overlay.querySelector(`.agent-tab[data-agent-id="${agentId}"] .agent-tab-dot`);
      if (dot) dot.style.background = color;
    }
  }
  const chip = document.querySelector(`.agent-tray-chip[data-agent-id="${agentId}"] .agent-tray-dot`);
  if (chip) chip.style.background = color;
}

function _incrementTabUnread(agentId) {
  if (!agentsViewOpen || agentId === _selectedAgentTab) return;
  const overlay = document.getElementById('agents-view');
  if (!overlay) return;
  const tab = overlay.querySelector(`.agent-tab[data-agent-id="${agentId}"]`);
  if (!tab) return;
  let badge = tab.querySelector('.agent-tab-unread');
  if (!badge) {
    badge = document.createElement('span');
    badge.className = 'agent-tab-unread';
    badge.textContent = '1';
    tab.appendChild(badge);
  } else {
    const count = parseInt(badge.textContent || '0', 10) + 1;
    badge.textContent = count > 99 ? '99+' : String(count);
  }
}

/**
 * Close the agents overlay view and move all agent windows back to the tray.
 * @returns {void}
 */
export function closeAgentsView() {
  const overlay = document.getElementById('agents-view');
  if (!overlay) return;
  const tray = document.getElementById('agent-windows');
  const dropdown = document.getElementById('agent-dropdown');
  if (tray) {
    for (const [, win] of agentWindows) {
      // Don't move windows that are currently in the top dropdown
      if (dropdown && dropdown.contains(win.el)) continue;
      if (win.el.parentElement !== tray) {
        win.el.style.display = 'none';
        tray.appendChild(win.el);
      }
    }
  }
  overlay.classList.remove('open');
  overlay.innerHTML = '';
  agentsViewOpen = false;
}

/**
 * Create an agent window and add it to the agents overlay grid with a new tab.
 * @param {Object} node - Graph node representing the agent
 * @returns {void}
 */
export function createAgentWindowInGrid(node) {
  if (!node || agentWindows.has(node.id)) return;
  const overlay = document.getElementById('agents-view');
  if (!overlay) return;
  const contentArea = overlay.querySelector('.agents-tab-content');
  if (!contentArea) return;
  _ensureAgentWindow(node, contentArea);
  const tabBar = overlay.querySelector('.agents-tab-bar');
  if (tabBar) {
    const agentName = node.title || node.id.replace('agent:', '');
    const agentStatus = (node.status || '').toLowerCase();
    const dotColor =
      agentStatus === 'active'
        ? '#2d8a4e'
        : agentStatus === 'idle'
          ? '#d4a017'
          : agentStatus === 'crashed'
            ? '#d04040'
            : '#666';
    const tab = document.createElement('button');
    tab.className = 'agent-tab';
    tab.dataset.agentId = node.id;
    tab.dataset.agentName = agentName;
    tab.innerHTML = `<span class="agent-tab-dot" style="background:${dotColor}"></span>${escapeHtml(agentName)}`;
    tab.onclick = () => selectAgentTab(node.id);
    tabBar.appendChild(tab);
  }
  const win = agentWindows.get(node.id);
  if (win && _selectedAgentTab && _selectedAgentTab !== node.id) {
    win.el.style.display = 'none';
  }
  updateAgentsViewStats();
}

/**
 * Update the active/idle/crashed/total stats display in the agents overlay header.
 * @returns {void}
 */
export function updateAgentsViewStats() {
  const overlay = document.getElementById('agents-view');
  if (!overlay) return;
  const statsEl = overlay.querySelector('.agents-view-stats');
  const graphData = _getGraphData();
  if (!statsEl || !graphData) return;
  const agentNodes = graphData.nodes.filter((n) => n.issue_type === 'agent' && !n._hidden);
  const counts = { active: 0, idle: 0, crashed: 0 };
  for (const n of agentNodes) {
    if (n.status === 'active') counts.active++;
    else if (n.status === 'idle') counts.idle++;
    else if (n.status === 'crashed') counts.crashed++;
  }
  statsEl.innerHTML = `<span class="active">${counts.active} active</span><span class="idle">${counts.idle} idle</span>${counts.crashed ? `<span class="crashed">${counts.crashed} crashed</span>` : ''}<span>${agentNodes.length} total</span>`;
}

// --- Unified Activity Stream (bd-9ndk0.3) ---

const _unifiedFeed = { el: null, entries: [], maxEntries: 500, pendingTools: new Map() };

/**
 * Initialize the unified activity feed DOM element and toggle button.
 * @returns {void}
 */
export function initUnifiedFeed() {
  _unifiedFeed.el = document.getElementById('unified-feed');
  if (!_unifiedFeed.el) return;
  _unifiedFeed.el.innerHTML = '<div class="uf-empty">waiting for agent events...</div>';
  const toggleBtn = document.getElementById('unified-feed-toggle');
  if (toggleBtn) {
    toggleBtn.onclick = () => {
      const active = _unifiedFeed.el.classList.toggle('active');
      toggleBtn.textContent = active ? 'split' : 'unified';
    };
  }
}

function appendUnifiedEntry(agentId, evt) {
  if (!_unifiedFeed.el) return;
  const type = evt.type;
  const p = evt.payload || {};
  const ts = evt.ts ? new Date(evt.ts) : new Date();
  const timeStr = ts.toTimeString().slice(0, 8);
  const agentName = agentId.replace(/^agent:/, '');
  if (type === 'PreToolUse') {
    const label = formatToolLabel(p);
    const toolClass = `tool-${(p.tool_name || 'tool').toLowerCase()}`;
    const entry = createUnifiedEntry(timeStr, agentName, TOOL_ICONS[p.tool_name] || '·', label, toolClass + ' running');
    _unifiedFeed.el.appendChild(entry);
    _unifiedFeed.entries.push(entry);
    _unifiedFeed.pendingTools.set(agentId, { entry, startTs: ts.getTime() });
    trimUnifiedFeed();
    autoScrollUnified();
    return;
  }
  if (type === 'PostToolUse') {
    const pending = _unifiedFeed.pendingTools.get(agentId);
    if (pending) {
      const dur = (ts.getTime() - pending.startTs) / 1000;
      pending.entry.classList.remove('running');
      const durEl = pending.entry.querySelector('.uf-entry-dur');
      if (durEl && dur > 0.1) durEl.textContent = `${dur.toFixed(1)}s`;
      const iconEl = pending.entry.querySelector('.uf-entry-icon');
      if (iconEl) iconEl.textContent = '✓';
      _unifiedFeed.pendingTools.delete(agentId);
    }
    return;
  }
  let icon, text, classes;
  if (type === 'AgentStarted') {
    icon = '●';
    text = 'started';
    classes = 'lifecycle lifecycle-started';
  } else if (type === 'AgentIdle') {
    icon = '◌';
    text = 'idle';
    classes = 'lifecycle lifecycle-idle';
  } else if (type === 'AgentCrashed') {
    icon = '✕';
    text = 'crashed!';
    classes = 'lifecycle lifecycle-crashed';
  } else if (type === 'AgentStopped') {
    icon = '○';
    text = 'stopped';
    classes = 'lifecycle lifecycle-stopped';
  } else if (type === 'SessionStart') {
    icon = '▸';
    text = 'session start';
    classes = 'lifecycle';
  } else if (type === 'MutationCreate') {
    icon = '+';
    text = `new: ${p.title || 'bead'}`;
    classes = 'mutation';
  } else if (type === 'MutationClose') {
    icon = '✓';
    text = `closed ${p.issue_id || ''}`;
    classes = 'mutation mutation-close';
  } else if (type === 'MutationStatus') {
    icon = '~';
    text = p.new_status || 'updated';
    classes = 'mutation';
  } else if (type === 'MutationUpdate') {
    if (p.assignee) {
      icon = '→';
      text = `claimed by ${p.assignee}`;
      classes = 'mutation';
    } else return;
  } else if (type === 'DecisionCreated') {
    icon = '?';
    text = (p.question || 'decision').slice(0, 50);
    classes = 'decision decision-pending';
  } else if (type === 'DecisionResponded') {
    icon = '✓';
    text = `decided: ${p.chosen_label || 'resolved'}`;
    classes = 'decision decision-resolved';
  } else if (type === 'DecisionExpired') {
    icon = '⏰';
    text = 'decision expired';
    classes = 'decision decision-expired';
  } else if (type === 'MailSent') {
    icon = '✉';
    text = `from ${p.from || '?'}: ${p.subject || ''}`;
    classes = 'mail mail-received';
  } else if (type === 'MailRead') {
    icon = '✉';
    text = 'mail read';
    classes = 'mail';
  } else if (type === 'UserPromptSubmit') {
    const prompt = p.prompt || p.Prompt || '';
    if (!prompt) return;
    icon = '▸';
    text = prompt.length > 120 ? prompt.slice(0, 120) + '…' : prompt;
    classes = 'prompt';
  } else if (type === 'Stop') {
    const msg = p.last_assistant_message || '';
    if (msg) {
      const msgDisplay = msg.length > 200 ? msg.slice(0, 200) + '…' : msg;
      const msgEntry = createUnifiedEntry(timeStr, agentName, '💬', msgDisplay, 'assistant-msg');
      _unifiedFeed.el.appendChild(msgEntry);
      _unifiedFeed.entries.push(msgEntry);
    }
    icon = '⏸';
    text = 'checkpoint';
    classes = 'lifecycle';
  } else return;
  const entry = createUnifiedEntry(timeStr, agentName, icon, text, classes);
  _unifiedFeed.el.appendChild(entry);
  _unifiedFeed.entries.push(entry);
  trimUnifiedFeed();
  autoScrollUnified();
}

function createUnifiedEntry(time, agent, icon, text, classes) {
  if (_unifiedFeed.el) {
    const empty = _unifiedFeed.el.querySelector('.uf-empty');
    if (empty) empty.remove();
  }
  const el = document.createElement('div');
  el.className = `uf-entry ${classes}`;
  el.innerHTML = `<span class="uf-entry-time">${escapeHtml(time)}</span><span class="uf-entry-agent">${escapeHtml(agent)}</span><span class="uf-entry-icon">${escapeHtml(icon)}</span><span class="uf-entry-text">${escapeHtml(text)}</span><span class="uf-entry-dur"></span>`;
  return el;
}
function trimUnifiedFeed() {
  while (_unifiedFeed.entries.length > _unifiedFeed.maxEntries) {
    const old = _unifiedFeed.entries.shift();
    if (old && old.parentNode) old.parentNode.removeChild(old);
  }
}
function autoScrollUnified() {
  if (!_unifiedFeed.el) return;
  const isNear = _unifiedFeed.el.scrollTop + _unifiedFeed.el.clientHeight >= _unifiedFeed.el.scrollHeight - 30;
  if (isNear || _unifiedFeed.entries.length <= 1) {
    requestAnimationFrame(() => {
      _unifiedFeed.el.scrollTop = _unifiedFeed.el.scrollHeight;
    });
  }
}

// --- Agent event routing (bd-kau4k, bd-5ok9s) ---

/**
 * Route an agent event to both the unified feed and the agent's individual window.
 * @param {string} agentId - The agent ID that produced the event
 * @param {Object} evt - The event object with type, payload, and ts fields
 * @returns {void}
 */
export function appendAgentEvent(agentId, evt) {
  appendUnifiedEntry(agentId, evt);
  const win = agentWindows.get(agentId);
  if (!win) return;
  const empty = win.feedEl.querySelector('.agent-window-empty');
  if (empty) empty.remove();
  const type = evt.type;
  const p = evt.payload || {};
  const ts = evt.ts ? new Date(evt.ts) : new Date();
  const timeStr = ts.toTimeString().slice(0, 8);
  if (type === 'PreToolUse') {
    const toolName = p.tool_name || 'tool';
    const icon = TOOL_ICONS[toolName] || '·';
    const label = formatToolLabel(p);
    const entry = createEntry(timeStr, icon, label, `tool-${toolName.toLowerCase()} running`);
    win.feedEl.appendChild(entry);
    win.entries.push(entry);
    win.pendingTool = { toolName, startTs: ts.getTime(), entry };
    win.lastTool = toolName;
    win.lastStatus = 'active';
    win.idleSince = null;
    win.crashError = null;
    _updateAgentStatusBar(win);
    autoScroll(win);
    return;
  }
  if (type === 'PostToolUse') {
    if (win.pendingTool) {
      const dur = (ts.getTime() - win.pendingTool.startTs) / 1000;
      win.pendingTool.entry.classList.remove('running');
      const durEl = win.pendingTool.entry.querySelector('.agent-entry-dur');
      if (durEl && dur > 0.1) durEl.textContent = `${dur.toFixed(1)}s`;
      const iconEl = win.pendingTool.entry.querySelector('.agent-entry-icon');
      if (iconEl) iconEl.textContent = '✓';
      win.pendingTool = null;
    }
    return;
  }
  if (type === 'AgentStarted') {
    win.feedEl.appendChild(createEntry(timeStr, '●', 'started', 'lifecycle lifecycle-started'));
    win.lastStatus = 'active';
    win.idleSince = null;
    win.crashError = null;
    win.lastTool = null;
  } else if (type === 'AgentIdle') {
    win.feedEl.appendChild(createEntry(timeStr, '◌', 'idle', 'lifecycle lifecycle-idle'));
    win.lastStatus = 'idle';
    win.idleSince = ts.getTime();
    win.crashError = null;
  } else if (type === 'AgentCrashed') {
    win.feedEl.appendChild(createEntry(timeStr, '✕', 'crashed!', 'lifecycle lifecycle-crashed'));
    win.lastStatus = 'crashed';
    win.idleSince = null;
    win.crashError = p.error || 'unknown error';
  } else if (type === 'AgentStopped') {
    win.feedEl.appendChild(createEntry(timeStr, '○', 'stopped', 'lifecycle lifecycle-stopped'));
    win.lastStatus = 'stopped';
    win.idleSince = null;
  } else if (type === 'SessionStart') {
    win.feedEl.appendChild(createEntry(timeStr, '▸', 'session start', 'lifecycle'));
    win.lastStatus = 'active';
    win.idleSince = null;
    win.crashError = null;
  } else if (type === 'MutationCreate') {
    win.feedEl.appendChild(createEntry(timeStr, '+', `new: ${p.title || 'new bead'}`, 'mutation'));
  } else if (type === 'MutationClose') {
    win.feedEl.appendChild(createEntry(timeStr, '✓', `closed ${p.issue_id || ''}`, 'mutation mutation-close'));
  } else if (type === 'MutationStatus') {
    win.feedEl.appendChild(createEntry(timeStr, '~', p.new_status || 'updated', 'mutation'));
  } else if (type === 'MutationUpdate') {
    if (p.assignee) {
      win.feedEl.appendChild(createEntry(timeStr, '→', `claimed by ${p.assignee}`, 'mutation'));
    }
    return;
  } else if (type === 'OjJobCreated') {
    win.feedEl.appendChild(createEntry(timeStr, '⚙', 'job created', 'lifecycle'));
  } else if (type === 'OjJobCompleted') {
    win.feedEl.appendChild(createEntry(timeStr, '✓', 'job done', 'lifecycle lifecycle-started'));
  } else if (type === 'OjJobFailed') {
    win.feedEl.appendChild(createEntry(timeStr, '✕', 'job failed!', 'lifecycle lifecycle-crashed'));
  } else if (type === 'MailSent') {
    win.feedEl.appendChild(
      createEntry(timeStr, '✉', `from ${p.from || 'unknown'}: ${p.subject || 'no subject'}`, 'mail mail-received'),
    );
  } else if (type === 'MailRead') {
    win.feedEl.appendChild(createEntry(timeStr, '✉', 'mail read', 'mail'));
  } else if (type === 'DecisionCreated') {
    win.feedEl.appendChild(
      createEntry(timeStr, '?', (p.question || 'decision').slice(0, 50), 'decision decision-pending'),
    );
  } else if (type === 'DecisionResponded') {
    win.feedEl.appendChild(
      createEntry(timeStr, '✓', `decided: ${p.chosen_label || 'resolved'}`, 'decision decision-resolved'),
    );
  } else if (type === 'DecisionExpired') {
    win.feedEl.appendChild(createEntry(timeStr, '⏰', 'decision expired', 'decision decision-expired'));
  } else if (type === 'DecisionEscalated') {
    win.feedEl.appendChild(createEntry(timeStr, '!', 'decision escalated', 'decision decision-escalated'));
  } else if (type === 'UserPromptSubmit') {
    const prompt = p.prompt || p.Prompt || '';
    if (prompt) {
      win.feedEl.appendChild(
        createEntry(timeStr, '▸', prompt.length > 120 ? prompt.slice(0, 120) + '…' : prompt, 'prompt'),
      );
    }
  } else if (type === 'Stop') {
    const msg = p.last_assistant_message || '';
    if (msg) {
      win.feedEl.appendChild(
        createEntry(timeStr, '💬', msg.length > 200 ? msg.slice(0, 200) + '…' : msg, 'assistant-msg'),
      );
    }
    win.feedEl.appendChild(createEntry(timeStr, '⏸', 'checkpoint', 'lifecycle'));
  } else {
    return;
  }
  _updateAgentStatusBar(win);
  _updateTabStatus(agentId, win.lastStatus);
  _incrementTabUnread(agentId);
  trimAgentFeed(win); // prevent unbounded DOM growth (bd-kkd9y)
  autoScroll(win);
}

function createEntry(time, icon, text, classes) {
  const el = document.createElement('div');
  el.className = `agent-entry ${classes}`;
  el.innerHTML = `<span class="agent-entry-time">${escapeHtml(time)}</span><span class="agent-entry-icon">${escapeHtml(icon)}</span><span class="agent-entry-text">${escapeHtml(text)}</span><span class="agent-entry-dur"></span>`;
  return el;
}
function autoScroll(win) {
  const feed = win.feedEl;
  const isNearBottom = feed.scrollTop + feed.clientHeight >= feed.scrollHeight - 30;
  if (isNearBottom || win.entries.length <= 1) {
    requestAnimationFrame(() => {
      feed.scrollTop = feed.scrollHeight;
    });
  }
}

// Trim per-agent feed DOM to prevent unbounded growth (bd-kkd9y)
function trimAgentFeed(win) {
  const feed = win.feedEl;
  while (feed.childNodes.length > AGENT_FEED_MAX) {
    const old = feed.firstChild;
    feed.removeChild(old);
    // Also clean from entries array if tracked
    const idx = win.entries.indexOf(old);
    if (idx !== -1) win.entries.splice(idx, 1);
  }
}

// --- Status bar updates (bd-5ok9s) ---

function _updateAgentStatusBar(win) {
  if (!win.statusEl) return;
  const stateEl = win.statusEl.querySelector('.agent-status-state');
  const idleDurEl = win.statusEl.querySelector('.agent-status-idle-dur');
  const toolEl = win.statusEl.querySelector('.agent-status-tool');
  if (stateEl) {
    const s = win.lastStatus || '?';
    stateEl.textContent = s;
    stateEl.className =
      'agent-status-state ' +
      (s === 'active' ? 'status-active' : s === 'idle' ? 'status-idle' : s === 'crashed' ? 'status-crashed' : '');
  }
  if (idleDurEl) {
    if (win.lastStatus === 'idle' && win.idleSince) {
      idleDurEl.innerHTML =
        '<span class="status-label">Idle:</span> <span class="status-idle-dur">' +
        formatDuration(Math.floor((Date.now() - win.idleSince) / 1000)) +
        '</span>';
    } else if (win.lastStatus === 'crashed' && win.crashError) {
      idleDurEl.innerHTML = '<span class="status-crashed">' + escapeStatusText(win.crashError) + '</span>';
    } else {
      idleDurEl.textContent = '';
    }
  }
  if (toolEl) {
    if (win.lastTool) {
      toolEl.innerHTML =
        '<span class="status-label">Last:</span> <span class="status-tool">' +
        escapeStatusText(win.lastTool) +
        '</span>';
    } else {
      toolEl.textContent = '';
    }
  }
}

/**
 * Start a 1-second interval that refreshes idle duration displays in agent status bars.
 * @returns {void}
 */
export function startAgentWindowIdleTimer() {
  setInterval(() => {
    for (const [, win] of agentWindows) {
      if (win.lastStatus === 'idle' && win.idleSince) {
        _updateAgentStatusBar(win);
      }
    }
  }, 1000);
}
