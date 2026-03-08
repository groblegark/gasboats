// Left Sidebar — agent roster + focused issue inspector (bd-nnr22, bd-7t6nt)
// Extracted from main.js to reduce monolith size.

import { resolveAgentIdLoose } from './event-format.js';

// Callbacks set by main.js to avoid circular imports
let _api = null;
let _getGraph = null;
let _getGraphData = null;
let _getSelectedNode = null;
let _selectNode = null;
let _showDetail = null;

// State references
let _leftSidebarOpen = false;
const _agentRoster = new Map(); // agent name → { status, task, tool, idleSince, crashError, nodeId }

/**
 * @param {Object} deps
 * @returns {void}
 */
export function setLeftSidebarDeps({ api, getGraph, getGraphData, getSelectedNode, selectNode, showDetail }) {
  _api = api;
  _getGraph = getGraph;
  _getGraphData = getGraphData;
  _getSelectedNode = getSelectedNode;
  _selectNode = selectNode;
  _showDetail = showDetail;
}

/**
 * @returns {boolean}
 */
export function getLeftSidebarOpen() {
  return _leftSidebarOpen;
}
/**
 * @param {boolean} v
 * @returns {void}
 */
export function setLeftSidebarOpen(v) {
  _leftSidebarOpen = v;
}
/**
 * @returns {Map<string, Object>}
 */
export function getAgentRoster() {
  return _agentRoster;
}

// Utility helpers (also used by agent windows in main.js)
/**
 * @param {number} seconds
 * @returns {string}
 */
export function formatDuration(seconds) {
  if (seconds < 60) return seconds + 's';
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  if (m < 60) return m + 'm' + (s > 0 ? s + 's' : '');
  const h = Math.floor(m / 60);
  return h + 'h' + (m % 60) + 'm';
}

/**
 * @param {string} str
 * @returns {string}
 */
export function escapeStatusText(str) {
  const d = document.createElement('div');
  d.textContent = str;
  return d.innerHTML;
}

function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

/**
 * @returns {void}
 */
export function toggleLeftSidebar() {
  const panel = document.getElementById('left-sidebar');
  if (!panel) return;
  _leftSidebarOpen = !_leftSidebarOpen;
  panel.classList.toggle('open', _leftSidebarOpen);
  if (_leftSidebarOpen) {
    renderAgentRoster();
    const sel = _getSelectedNode();
    if (sel) updateLeftSidebarFocus(sel);
  }
}

/**
 * @returns {void}
 */
export function initLeftSidebar() {
  const closeBtn = document.getElementById('ls-close');
  if (closeBtn)
    closeBtn.onclick = () => {
      _leftSidebarOpen = false;
      document.getElementById('left-sidebar')?.classList.remove('open');
    };
}

// Update agent roster from SSE events
/**
 * @param {Object} evt
 * @returns {void}
 */
export function updateAgentRosterFromEvent(evt) {
  const agentId = resolveAgentIdLoose(evt);
  if (!agentId) return;

  const type = evt.type || '';
  const p = evt.payload || {};
  const agentName = agentId.replace('agent:', '');

  let entry = _agentRoster.get(agentName);
  if (!entry) {
    entry = { status: 'active', task: '', tool: '', idleSince: null, crashError: null, nodeId: agentId };
    _agentRoster.set(agentName, entry);
  }

  if (type === 'AgentStarted' || type === 'SessionStart') {
    entry.status = 'active';
    entry.idleSince = null;
    entry.crashError = null;
  } else if (type === 'AgentIdle' || type === 'OjAgentIdle') {
    entry.status = 'idle';
    entry.idleSince = evt.ts ? new Date(evt.ts).getTime() : Date.now();
  } else if (type === 'AgentCrashed') {
    entry.status = 'crashed';
    entry.crashError = p.error || 'unknown';
    entry.idleSince = null;
  } else if (type === 'AgentStopped') {
    entry.status = 'stopped';
    entry.idleSince = null;
  } else if (type === 'PreToolUse') {
    entry.status = 'active';
    entry.tool = p.tool_name || p.toolName || '';
    entry.idleSince = null;
    entry.crashError = null;
  }

  // Update task from MutationUpdate with assignee claim (bd-23dv2: exact match only)
  if (type === 'MutationUpdate' && p.new_status === 'in_progress' && p.assignee) {
    const e = _agentRoster.get(p.assignee);
    if (e) e.task = p.title || p.issue_id || '';
  }

  // Clear stale task when a bead is closed (bd-23dv2)
  if (type === 'MutationClose' && p.assignee) {
    const e = _agentRoster.get(p.assignee);
    if (e && e.task) {
      // Only clear if the closed bead matches the displayed task
      const closedTitle = p.title || p.issue_id || '';
      if (e.task === closedTitle || e.task === p.issue_id) {
        e.task = '';
      }
    }
  }

  if (_leftSidebarOpen) renderAgentRoster();
}

/**
 * @returns {void}
 */
export function renderAgentRoster() {
  const list = document.getElementById('ls-agent-list');
  const count = document.getElementById('ls-agent-count');
  if (!list) return;

  const graphData = _getGraphData();

  // Seed roster from graph data and cross-reference assigned_to edges (bd-23dv2)
  if (graphData && graphData.nodes) {
    // Build set of agents that have assigned_to edges (actual current assignments)
    const agentsWithTasks = new Map(); // agentId → { id, title }
    if (graphData.links) {
      for (const l of graphData.links) {
        if (l.dep_type !== 'assigned_to') continue;
        const src = typeof l.source === 'object' ? l.source.id : l.source;
        const tgt = typeof l.target === 'object' ? l.target.id : l.target;
        const tgtNode = graphData.nodes.find((n) => n.id === tgt);
        if (tgtNode) agentsWithTasks.set(src, { id: tgt, title: tgtNode.title || tgt });
      }
    }

    for (const n of graphData.nodes) {
      if (n.issue_type === 'agent' && !n._hidden) {
        const name = n.title || n.id.replace('agent:', '');
        const assignedBead = agentsWithTasks.get(n.id);
        const taskFromGraph = assignedBead ? assignedBead.title : '';
        if (!_agentRoster.has(name)) {
          _agentRoster.set(name, {
            status: n._agentState || 'active',
            task: taskFromGraph,
            tool: '',
            idleSince: null,
            crashError: null,
            nodeId: n.id,
          });
        } else {
          const e = _agentRoster.get(name);
          e.nodeId = n.id;
          // Sync task from graph assigned_to edges — authoritative source (bd-23dv2)
          e.task = taskFromGraph;
        }
      }
    }
  }

  if (_agentRoster.size === 0) {
    list.innerHTML = '<div class="ls-agent-empty">No agents connected</div>';
    if (count) count.textContent = '0';
    return;
  }

  // Sort: active first, then idle, then crashed, then stopped
  const order = { active: 0, idle: 1, crashed: 2, stopped: 3 };
  const sorted = [..._agentRoster.entries()].sort((a, b) => (order[a[1].status] || 9) - (order[b[1].status] || 9));

  if (count) count.textContent = String(sorted.length);

  const graph = _getGraph();

  list.innerHTML = sorted
    .map(([name, e]) => {
      const dotClass =
        e.status === 'active' ? 'active' : e.status === 'idle' ? 'idle' : e.status === 'crashed' ? 'crashed' : 'idle';
      const idle =
        e.status === 'idle' && e.idleSince ? formatDuration(Math.floor((Date.now() - e.idleSince) / 1000)) : '';
      const toolText = e.tool ? escapeStatusText(e.tool) : '';
      return `<div class="ls-agent-row" data-agent-id="${escapeStatusText(e.nodeId)}" title="${escapeStatusText(name)}${e.task ? ': ' + escapeStatusText(e.task) : ''}">
      <span class="ls-agent-dot ${dotClass}"></span>
      <span class="ls-agent-name">${escapeStatusText(name)}</span>
      <span class="ls-agent-task">${e.task ? escapeStatusText(e.task.slice(0, 30)) : ''}</span>
      <span class="ls-agent-meta">${idle || toolText}</span>
    </div>`;
    })
    .join('');

  // Click handler: fly to agent node
  list.querySelectorAll('.ls-agent-row').forEach((row) => {
    row.onclick = () => {
      const nodeId = row.dataset.agentId;
      if (!nodeId || !graphData) return;
      const node = graphData.nodes.find((n) => n.id === nodeId);
      if (node && node.x !== undefined && graph) {
        const dist = 150;
        graph.cameraPosition(
          { x: node.x + dist, y: node.y + dist * 0.3, z: node.z + dist },
          { x: node.x, y: node.y, z: node.z },
          1000,
        );
      }
    };
  });
}

/**
 * @param {Object} node
 * @returns {Promise<void>}
 */
export async function updateLeftSidebarFocus(node) {
  const content = document.getElementById('ls-focused-content');
  if (!content) return;

  if (!node) {
    content.innerHTML = '<div class="ls-placeholder">Click a node to inspect</div>';
    return;
  }

  // Show basic info immediately
  const pLabel = ['P0', 'P1', 'P2', 'P3', 'P4'][node.priority] || '';
  const statusClass = node._blocked ? 'blocked' : node.status || 'open';
  content.innerHTML = `
    <div class="ls-issue-header">
      <span class="ls-issue-id">${escapeHtml(node.id)}</span>
      <span class="ls-issue-status ${statusClass}">${node._blocked ? 'blocked' : node.status || 'open'}</span>
      ${pLabel ? `<span class="ls-issue-priority">${pLabel}</span>` : ''}
    </div>
    <div class="ls-issue-title">${escapeHtml(node.title || node.id)}</div>
    <div style="color:#555;font-size:9px;font-style:italic">loading...</div>
  `;

  // Agent nodes: show agent info instead
  if (node.issue_type === 'agent') {
    const name = node.title || node.id.replace('agent:', '');
    const e = _agentRoster.get(name);
    content.innerHTML = `
      <div class="ls-issue-header">
        <span class="ls-issue-id">${escapeHtml(node.id)}</span>
        <span class="ls-issue-status in_progress">agent</span>
      </div>
      <div class="ls-issue-title">${escapeHtml(name)}</div>
      <div class="ls-issue-field">
        <div class="ls-issue-field-label">Status</div>
        <div class="ls-issue-field-value">${e ? e.status : 'unknown'}</div>
      </div>
      ${e && e.task ? `<div class="ls-issue-field"><div class="ls-issue-field-label">Current Task</div><div class="ls-issue-field-value">${escapeHtml(e.task)}</div></div>` : ''}
      ${e && e.tool ? `<div class="ls-issue-field"><div class="ls-issue-field-label">Last Tool</div><div class="ls-issue-field-value">${escapeHtml(e.tool)}</div></div>` : ''}
    `;
    return;
  }

  const graphData = _getGraphData();
  const graph = _getGraph();

  // Fetch full details
  try {
    const full = await _api.show(node.id);
    // Re-check that this node is still selected
    if (_getSelectedNode() !== node) return;

    let html = `
      <div class="ls-issue-header">
        <span class="ls-issue-id">${escapeHtml(node.id)}</span>
        <span class="ls-issue-status ${statusClass}">${node._blocked ? 'blocked' : node.status || 'open'}</span>
        ${pLabel ? `<span class="ls-issue-priority">${pLabel}</span>` : ''}
      </div>
      <div class="ls-issue-title">${escapeHtml(full.title || node.title || node.id)}</div>
    `;

    // Type + assignee
    const metaParts = [];
    if (full.issue_type || node.issue_type) metaParts.push(full.issue_type || node.issue_type);
    if (full.assignee) metaParts.push('@ ' + full.assignee);
    if (metaParts.length) {
      html += `<div class="ls-issue-field"><div class="ls-issue-field-label">Info</div><div class="ls-issue-field-value">${escapeHtml(metaParts.join(' · '))}</div></div>`;
    }

    // Description
    if (full.description) {
      const desc = full.description.length > 300 ? full.description.slice(0, 300) + '...' : full.description;
      html += `<div class="ls-issue-field"><div class="ls-issue-field-label">Description</div><div class="ls-issue-field-value">${escapeHtml(desc)}</div></div>`;
    }

    // Dependencies (blocks / blocked_by)
    if (full.dependencies && full.dependencies.length > 0) {
      const deps = full.dependencies
        .map((d) => {
          const depId = d.depends_on_id || d.id || '';
          return `<span class="ls-dep-link" data-dep-id="${escapeHtml(depId)}">${escapeHtml(d.title || depId)}</span>`;
        })
        .join('<br>');
      html += `<div class="ls-issue-field"><div class="ls-issue-field-label">Depends On</div><div class="ls-issue-field-value">${deps}</div></div>`;
    }
    if (full.blocked_by && full.blocked_by.length > 0) {
      const blockers = full.blocked_by
        .map((b) => `<span class="ls-dep-link" data-dep-id="${escapeHtml(b)}">${escapeHtml(b)}</span>`)
        .join('<br>');
      html += `<div class="ls-issue-field"><div class="ls-issue-field-label">Blocked By</div><div class="ls-issue-field-value">${blockers}</div></div>`;
    }

    // Labels
    if (full.labels && full.labels.length > 0) {
      html += `<div class="ls-issue-field"><div class="ls-issue-field-label">Labels</div><div class="ls-issue-field-value">${full.labels.map((l) => escapeHtml(l)).join(', ')}</div></div>`;
    }

    content.innerHTML = html;

    // Bind dep link click handlers
    content.querySelectorAll('.ls-dep-link').forEach((link) => {
      link.onclick = () => {
        const depId = link.dataset.depId;
        if (!depId || !graphData) return;
        const depNode = graphData.nodes.find((n) => n.id === depId);
        if (depNode) {
          _selectNode(depNode);
          _showDetail(depNode);
          if (depNode.x !== undefined && graph) {
            graph.cameraPosition(
              { x: depNode.x + 100, y: depNode.y + 30, z: depNode.z + 100 },
              { x: depNode.x, y: depNode.y, z: depNode.z },
              800,
            );
          }
        }
      };
    });
  } catch (err) {
    content.innerHTML = `
      <div class="ls-issue-header">
        <span class="ls-issue-id">${escapeHtml(node.id)}</span>
      </div>
      <div class="ls-issue-title">${escapeHtml(node.title || node.id)}</div>
      <div class="ls-issue-field-value">Could not load details</div>
    `;
  }
}

// Update agent roster idle durations (runs alongside bd-5ok9s status bar updates)
/**
 * @returns {void}
 */
export function startLeftSidebarIdleTimer() {
  setInterval(() => {
    if (_leftSidebarOpen) {
      // Update idle durations in-place
      const list = document.getElementById('ls-agent-list');
      if (!list) return;
      list.querySelectorAll('.ls-agent-row').forEach((row) => {
        const agentId = row.dataset.agentId;
        if (!agentId) return;
        const name = agentId.replace('agent:', '');
        const e = _agentRoster.get(name);
        if (e && e.status === 'idle' && e.idleSince) {
          const meta = row.querySelector('.ls-agent-meta');
          if (meta) meta.textContent = formatDuration(Math.floor((Date.now() - e.idleSince) / 1000));
        }
      });
    }
  }, 1000);
}
