// --- Context menu (right-click) --- extracted from main.js (bd-7t6nt)
// Handles right-click context menu on nodes: status/priority changes,
// claim, close, expand deps, show deps/blockers, copy ID.

// Dependency injection — set by main.js before use
let _deps = {};

/**
 * Inject dependencies from main.js.
 *
 * @param {Object} deps
 * @param {Object}   deps.api             - BeadsAPI instance
 * @param {Function} deps.getGraph        - () => ForceGraph3D instance
 * @param {Function} deps.getGraphData    - () => { nodes, links }
 * @param {Function} deps.escapeHtml      - (str) => escaped HTML string
 * @param {Function} deps.hideTooltip     - () => void
 * @param {Function} deps.expandDepTree   - (node) => void
 * @param {Function} deps.highlightSubgraph - (node, direction) => void
 */
export function setContextMenuDeps(deps) {
  _deps = deps;
}

// --- Context menu (right-click) ---
const ctxMenu = document.getElementById('context-menu');
let ctxNode = null;

function buildStatusSubmenu(currentStatus) {
  const statuses = [
    { value: 'open', label: 'open', color: '#2d8a4e' },
    { value: 'in_progress', label: 'in progress', color: '#d4a017' },
    { value: 'closed', label: 'closed', color: '#333340' },
  ];
  return statuses
    .map(
      (s) =>
        `<div class="ctx-sub-item${s.value === currentStatus ? ' active' : ''}" data-action="set-status" data-value="${s.value}">` +
        `<span class="ctx-dot" style="background:${s.color}"></span>${s.label}</div>`,
    )
    .join('');
}

function buildPrioritySubmenu(currentPriority) {
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
        `<div class="ctx-sub-item${p.value === currentPriority ? ' active' : ''}" data-action="set-priority" data-value="${p.value}">` +
        `<span class="ctx-dot" style="background:${p.color}"></span>${p.label}</div>`,
    )
    .join('');
}

/**
 * Handles a right-click on a graph node by building and displaying
 * the context menu with status, priority, and action options.
 *
 * @param {Object} node - The graph node that was right-clicked.
 * @param {MouseEvent} event - The contextmenu mouse event.
 * @returns {void}
 */
export function handleNodeRightClick(node, event) {
  event.preventDefault();
  if (!node || node._hidden) return;
  // Skip agent pseudo-nodes — they're not real beads
  if (node.issue_type === 'agent') return;
  ctxNode = node;
  _deps.hideTooltip();

  ctxMenu.innerHTML = `
    <div class="ctx-header">${_deps.escapeHtml(node.id)}</div>
    <div class="ctx-item ctx-submenu">status
      <div class="ctx-submenu-panel">${buildStatusSubmenu(node.status)}</div>
    </div>
    <div class="ctx-item ctx-submenu">priority
      <div class="ctx-submenu-panel">${buildPrioritySubmenu(node.priority)}</div>
    </div>
    <div class="ctx-item" data-action="claim">claim (assign to me)</div>
    <div class="ctx-item" data-action="close-bead">close</div>
    <div class="ctx-sep"></div>
    <div class="ctx-item" data-action="expand-deps">expand dep tree<span class="ctx-key">e</span></div>
    <div class="ctx-item" data-action="show-deps">show dependencies<span class="ctx-key">d</span></div>
    <div class="ctx-item" data-action="show-blockers">show blockers<span class="ctx-key">b</span></div>
    <div class="ctx-sep"></div>
    <div class="ctx-item" data-action="copy-id">copy ID<span class="ctx-key">c</span></div>
    <div class="ctx-item" data-action="copy-show">copy bd show ${_deps.escapeHtml(node.id)}</div>
  `;

  // Position menu, keeping it on screen
  ctxMenu.style.display = 'block';
  const rect = ctxMenu.getBoundingClientRect();
  let x = event.clientX;
  let y = event.clientY;
  if (x + rect.width > window.innerWidth) x = window.innerWidth - rect.width - 8;
  if (y + rect.height > window.innerHeight) y = window.innerHeight - rect.height - 8;
  ctxMenu.style.left = x + 'px';
  ctxMenu.style.top = y + 'px';

  // Handle clicks on menu items (including submenu items, bd-9g7f0)
  ctxMenu.onclick = (e) => {
    // Check submenu items first (they're nested inside .ctx-item)
    const subItem = e.target.closest('.ctx-sub-item');
    if (subItem) {
      handleContextAction(subItem.dataset.action, node, subItem);
      return;
    }
    const item = e.target.closest('.ctx-item');
    if (!item || item.classList.contains('ctx-submenu')) return; // skip submenu parents
    handleContextAction(item.dataset.action, node, item);
  };
}

/**
 * Hides the context menu and clears associated state.
 *
 * @returns {void}
 */
export function hideContextMenu() {
  ctxMenu.style.display = 'none';
  ctxMenu.onclick = null;
  ctxNode = null;
}

// Apply an optimistic update to a node: immediately update local data + visuals,
// fire the API call, and revert on failure.
async function optimisticUpdate(node, changes, apiCall) {
  // Snapshot current values for rollback
  const snapshot = {};
  for (const key of Object.keys(changes)) {
    snapshot[key] = node[key];
  }

  // Apply changes immediately
  Object.assign(node, changes);

  // Force Three.js object rebuild for this node (picks up new color, size, status effects)
  const graph = _deps.getGraph();
  graph.nodeThreeObject(graph.nodeThreeObject());

  try {
    await apiCall();
  } catch (err) {
    // Revert on failure
    Object.assign(node, snapshot);
    graph.nodeThreeObject(graph.nodeThreeObject());
    showStatusToast(`error: ${err.message}`, true);
  }
}

async function handleContextAction(action, node, el) {
  const api = _deps.api;
  switch (action) {
    case 'set-status': {
      const value = el?.dataset.value;
      if (!value || value === node.status) break;
      hideContextMenu();
      showStatusToast(`${node.id} → ${value}`);
      await optimisticUpdate(node, { status: value }, () => api.update(node.id, { status: value }));
      break;
    }
    case 'set-priority': {
      const value = parseInt(el?.dataset.value, 10);
      if (isNaN(value) || value === node.priority) break;
      hideContextMenu();
      showStatusToast(`${node.id} → P${value}`);
      await optimisticUpdate(node, { priority: value }, () => api.update(node.id, { priority: value }));
      break;
    }
    case 'claim':
      hideContextMenu();
      showStatusToast(`claimed ${node.id}`);
      await optimisticUpdate(node, { status: 'in_progress' }, () => api.update(node.id, { status: 'in_progress' }));
      break;
    case 'close-bead':
      hideContextMenu();
      showStatusToast(`closed ${node.id}`);
      await optimisticUpdate(node, { status: 'closed' }, () => api.close(node.id));
      break;
    case 'expand-deps':
      _deps.expandDepTree(node);
      hideContextMenu();
      break;
    case 'show-deps':
      _deps.highlightSubgraph(node, 'downstream');
      hideContextMenu();
      break;
    case 'show-blockers':
      _deps.highlightSubgraph(node, 'upstream');
      hideContextMenu();
      break;
    case 'copy-id':
      copyToClipboard(node.id);
      showCtxToast('copied!');
      break;
    case 'copy-show':
      copyToClipboard(`bd show ${node.id}`);
      showCtxToast('copied!');
      break;
  }
}

// Brief toast message overlaid on the status bar (bd-9g7f0)
let _toastTimer = null;
let _toastOrigText = null;
let _toastOrigClass = null;
/**
 * Displays a brief toast message overlaid on the status bar.
 *
 * @param {string} msg - The message to display.
 * @param {boolean} [isError=false] - Whether to style the toast as an error.
 * @returns {void}
 */
export function showStatusToast(msg, isError = false) {
  const el = document.getElementById('status');
  // Save the base state only on first (non-nested) toast
  if (_toastTimer === null) {
    _toastOrigText = el.textContent;
    _toastOrigClass = el.className;
  } else {
    clearTimeout(_toastTimer);
  }
  el.textContent = msg;
  el.className = isError ? 'error' : '';
  _toastTimer = setTimeout(() => {
    el.textContent = _toastOrigText;
    el.className = _toastOrigClass;
    _toastTimer = null;
  }, 2000);
}

function copyToClipboard(text) {
  navigator.clipboard.writeText(text).catch(() => {
    // Fallback for non-HTTPS contexts
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    document.body.removeChild(ta);
  });
}

function showCtxToast(msg) {
  const items = ctxMenu.querySelectorAll('.ctx-item');
  // Brief flash on the clicked item, then close
  setTimeout(() => hideContextMenu(), 400);
}

// Close context menu on any click elsewhere or Escape
document.addEventListener('click', (e) => {
  if (ctxMenu.style.display === 'block' && !ctxMenu.contains(e.target)) {
    hideContextMenu();
  }
});

// Suppress browser context menu on the graph canvas
document.getElementById('graph').addEventListener('contextmenu', (e) => e.preventDefault());

/**
 * The context menu DOM element, exported for keyboard shortcut checks in camera.js.
 *
 * @type {HTMLElement}
 */
export { ctxMenu };
