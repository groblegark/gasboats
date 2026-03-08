// Detail panel — tiling bead info panels with decision UI (bd-fbmq3, bd-1xskh, bd-7t6nt)
// Extracted from main.js to reduce monolith size.

import { rigColor } from './colors.js';

// Callbacks set by main.js to avoid circular imports
let _api = null;
let _showAgentWindow = null;
let _openAgentTab = null; // bd-bwi52: open tabbed agents view for a node
let _showStatusToast = null;
let _getGraph = null;

/**
 * Map of currently open detail panels, keyed by bead ID.
 *
 * @type {Map<string, HTMLElement>}
 */
const openPanels = new Map(); // beadId → panel element

/**
 * Inject dependencies from main.js.
 *
 * @param {Object} deps
 * @param {Object}   deps.api              - BeadsAPI instance
 * @param {Function} deps.showAgentWindow  - (node) => void
 * @param {Function} deps.openAgentTab     - (node) => void
 * @param {Function} deps.showStatusToast  - (msg, isError?) => void
 * @param {Function} deps.getGraph         - () => ForceGraph3D instance
 * @returns {void}
 */
export function setDetailDeps({ api, showAgentWindow, openAgentTab, showStatusToast, getGraph }) {
  _api = api;
  _showAgentWindow = showAgentWindow;
  _openAgentTab = openAgentTab;
  _showStatusToast = showStatusToast;
  _getGraph = getGraph;
}

export { openPanels };

function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

/**
 * Shows the detail panel for a graph node. If the panel for this node is
 * already open, it is toggled closed instead.
 *
 * @param {Object} node - Graph node object with id, title, status, etc.
 * @returns {Promise<void>}
 */
export async function showDetail(node) {
  const container = document.getElementById('detail');

  // Toggle: if this bead's panel is already open, close it (bd-fbmq3)
  if (openPanels.has(node.id)) {
    closeDetailPanel(node.id);
    return;
  }

  container.style.display = 'block';

  // Create a new panel element
  const panel = document.createElement('div');
  panel.className = 'detail-panel';
  panel.dataset.beadId = node.id;
  container.appendChild(panel);

  // Track it
  openPanels.set(node.id, panel);
  repositionPanels();

  const pLabel = ['P0 CRIT', 'P1 HIGH', 'P2 MED', 'P3 LOW', 'P4 BACKLOG'][node.priority] || '';

  // Show basic info immediately
  panel.innerHTML = `
    <div class="detail-header">
      <span class="detail-id">${escapeHtml(node.id)}</span>
      <button class="detail-close">&times;</button>
    </div>
    <div class="detail-title">${escapeHtml(node.title || node.id)}</div>
    <div class="detail-meta">
      <span class="tag tag-${node.status}">${node.status}</span>
      <span class="tag">${node.issue_type || 'task'}</span>
      <span class="tag">${pLabel}</span>
      ${node.assignee ? `<span class="tag tag-assignee">${escapeHtml(node.assignee)}</span>` : ''}
      ${node.rig ? `<span class="tag" style="color:${rigColor(node.rig)};border-color:${rigColor(node.rig)}33">${escapeHtml(node.rig)}</span>` : ''}
      ${node._blocked ? '<span class="tag tag-blocked">BLOCKED</span>' : ''}
      ${node._jackExpired ? '<span class="tag tag-blocked">EXPIRED</span>' : ''}
    </div>
    <div class="detail-body loading">loading full details...</div>
  `;

  // Close button handler
  panel.querySelector('.detail-close').onclick = () => closeDetailPanel(node.id);

  // Animate open
  requestAnimationFrame(() => panel.classList.add('open'));

  // Agent nodes: open tabbed agents view (bd-kau4k, bd-bwi52)
  if (node.issue_type === 'agent' && node.id.startsWith('agent:')) {
    closeDetailPanel(node.id);
    if (_openAgentTab) _openAgentTab(node);
    else if (_showAgentWindow) _showAgentWindow(node);
    return;
  }

  // Decision/gate nodes: show decision panel with options and resolve UI (bd-1xskh, bd-9gxt1)
  if (node.issue_type === 'gate' || node.issue_type === 'decision') {
    try {
      const resp = await _api.decisionGet(node.id);
      const body = panel.querySelector('.detail-body');
      if (body) {
        body.classList.remove('loading');
        body.innerHTML = renderDecisionDetail(node, resp);
        bindDecisionHandlers(panel, node, resp);
      }
    } catch (err) {
      // Fall back to regular detail
      try {
        const full = await _api.show(node.id);
        const body = panel.querySelector('.detail-body');
        if (body) {
          body.classList.remove('loading');
          body.innerHTML = renderFullDetail(full);
        }
      } catch (err2) {
        const body = panel.querySelector('.detail-body');
        if (body) {
          body.classList.remove('loading');
          body.textContent = `Could not load: ${err2.message}`;
        }
      }
    }
    return;
  }

  // Regular nodes
  try {
    const full = await _api.show(node.id);
    const body = panel.querySelector('.detail-body');
    if (body) {
      body.classList.remove('loading');
      body.innerHTML = renderFullDetail(full);
    }
  } catch (err) {
    const body = panel.querySelector('.detail-body');
    if (body) {
      body.classList.remove('loading');
      body.textContent = `Could not load: ${err.message}`;
    }
  }
}

/**
 * Close a single detail panel by bead ID.
 *
 * @param {string} beadId - The bead/issue identifier whose panel to close.
 * @returns {void}
 */
export function closeDetailPanel(beadId) {
  const panel = openPanels.get(beadId);
  if (!panel) return;
  panel.classList.remove('open');
  openPanels.delete(beadId);
  setTimeout(() => {
    panel.remove();
    repositionPanels();
    if (openPanels.size === 0) {
      document.getElementById('detail').style.display = 'none';
    }
  }, 200); // wait for slide-out animation
}

// Position panels side-by-side from right edge (bd-fbmq3)
function repositionPanels() {
  let offset = 0;
  // Iterate in insertion order (Map preserves order) — newest on right
  const entries = [...openPanels.entries()].reverse();
  for (const [, panel] of entries) {
    panel.style.right = `${offset}px`;
    offset += 384; // 380px width + 4px gap
  }
}

function renderFullDetail(issue) {
  const sections = [];

  if (issue.description) {
    sections.push(`<div class="detail-section"><h4>Description</h4><pre>${escapeHtml(issue.description)}</pre></div>`);
  }
  if (issue.design) {
    sections.push(`<div class="detail-section"><h4>Design</h4><pre>${escapeHtml(issue.design)}</pre></div>`);
  }
  if (issue.notes) {
    sections.push(`<div class="detail-section"><h4>Notes</h4><pre>${escapeHtml(issue.notes)}</pre></div>`);
  }
  if (issue.acceptance_criteria) {
    sections.push(
      `<div class="detail-section"><h4>Acceptance Criteria</h4><pre>${escapeHtml(issue.acceptance_criteria)}</pre></div>`,
    );
  }

  // Dependencies
  if (issue.dependencies && issue.dependencies.length > 0) {
    const deps = issue.dependencies
      .map(
        (d) =>
          `<div class="dep-item">${escapeHtml(d.type || 'dep')} &rarr; ${escapeHtml(d.title || d.depends_on_id || d.id)}</div>`,
      )
      .join('');
    sections.push(`<div class="detail-section"><h4>Dependencies</h4>${deps}</div>`);
  }

  // Blocked by
  if (issue.blocked_by && issue.blocked_by.length > 0) {
    sections.push(
      `<div class="detail-section"><h4>Blocked By</h4>${issue.blocked_by.map((b) => `<div class="dep-item">${escapeHtml(b)}</div>`).join('')}</div>`,
    );
  }

  // Labels
  if (issue.labels && issue.labels.length > 0) {
    const labels = issue.labels.map((l) => `<span class="tag">${escapeHtml(l)}</span>`).join(' ');
    sections.push(`<div class="detail-section"><h4>Labels</h4>${labels}</div>`);
  }

  // Jack-specific metadata (bd-hffzf)
  if (issue.issue_type === 'jack' && issue.metadata) {
    const jm = typeof issue.metadata === 'string' ? JSON.parse(issue.metadata) : issue.metadata;
    const jackRows = [];
    if (jm.jack_target) jackRows.push(`<div><strong>Target:</strong> ${escapeHtml(jm.jack_target)}</div>`);
    if (jm.jack_ttl) jackRows.push(`<div><strong>TTL:</strong> ${escapeHtml(jm.jack_ttl)}</div>`);
    if (jm.jack_revert_plan) jackRows.push(`<div><strong>Revert plan:</strong> ${escapeHtml(jm.jack_revert_plan)}</div>`);
    if (issue.jack_expires_at) {
      const exp = new Date(issue.jack_expires_at);
      const isExpired = exp.getTime() < Date.now();
      jackRows.push(`<div><strong>Expires:</strong> ${exp.toLocaleString()} ${isExpired ? '<span class="tag tag-blocked">EXPIRED</span>' : ''}</div>`);
    }
    if (jm.jack_rig) jackRows.push(`<div><strong>Rig:</strong> ${escapeHtml(jm.jack_rig)}</div>`);
    if (jm.jack_changes && jm.jack_changes.length > 0) {
      jackRows.push(`<div><strong>Changes:</strong> ${jm.jack_changes.length} recorded</div>`);
    }
    if (jackRows.length) {
      sections.push(`<div class="detail-section"><h4>Jack Details</h4>${jackRows.join('')}</div>`);
    }
  }

  // Metadata
  const meta = [];
  if (issue.created_at) meta.push(`created: ${new Date(issue.created_at).toLocaleDateString()}`);
  if (issue.updated_at) meta.push(`updated: ${new Date(issue.updated_at).toLocaleDateString()}`);
  if (issue.owner) meta.push(`owner: ${issue.owner}`);
  if (issue.created_by) meta.push(`by: ${issue.created_by}`);
  if (meta.length) {
    sections.push(`<div class="detail-section detail-timestamps">${meta.join(' &middot; ')}</div>`);
  }

  return sections.join('') || '<em>No additional details</em>';
}

// Render decision detail panel content (bd-1xskh)
function renderDecisionDetail(node, resp) {
  const dec = resp.decision || {};
  const issue = resp.issue || {};
  const sections = [];

  // State badge
  const state = dec.selected_option ? 'resolved' : node.status === 'closed' ? 'resolved' : 'pending';
  const stateColor = state === 'resolved' ? '#2d8a4e' : state === 'expired' ? '#d04040' : '#d4a017';
  sections.push(
    `<div class="decision-state" style="color:${stateColor};font-weight:bold;margin-bottom:8px">${state.toUpperCase()}</div>`,
  );

  // Prompt
  if (dec.prompt) {
    sections.push(
      `<div class="detail-section"><h4>Question</h4><pre class="decision-prompt">${escapeHtml(dec.prompt)}</pre></div>`,
    );
  }

  // Context
  if (dec.context) {
    sections.push(`<div class="detail-section"><h4>Context</h4><pre>${escapeHtml(dec.context)}</pre></div>`);
  }

  // Options (DecisionPoint.Options is a JSON string in Go, must parse)
  const opts =
    typeof dec.options === 'string'
      ? (() => {
          try {
            return JSON.parse(dec.options);
          } catch {
            return [];
          }
        })()
      : dec.options || [];
  if (opts.length > 0) {
    const optHtml = opts
      .map((opt, i) => {
        const selected = dec.selected_option === opt.id;
        const cls = selected ? 'decision-opt selected' : 'decision-opt';
        const label = opt.label || opt.short || opt.id;
        const beadRef = opt.bead_id ? ` <span class="decision-opt-bead">(${escapeHtml(opt.bead_id)})</span>` : '';
        return `<button class="${cls}" data-opt-id="${escapeHtml(opt.id)}" data-opt-idx="${i}">${escapeHtml(label)}${beadRef}</button>`;
      })
      .join('');
    sections.push(`<div class="detail-section"><h4>Options</h4><div class="decision-options">${optHtml}</div></div>`);
  }

  // Resolution result
  if (dec.selected_option) {
    const selectedOpt = opts.find((o) => o.id === dec.selected_option);
    const selectedLabel = selectedOpt ? selectedOpt.label || selectedOpt.short || selectedOpt.id : dec.selected_option;
    let resolvedInfo = `<div class="decision-selected">${escapeHtml(selectedLabel)}</div>`;
    if (dec.responded_by) resolvedInfo += `<div style="color:#888;font-size:11px">by ${escapeHtml(dec.responded_by)}`;
    if (dec.responded_at) resolvedInfo += ` at ${new Date(dec.responded_at).toLocaleString()}`;
    if (dec.responded_by) resolvedInfo += `</div>`;
    sections.push(`<div class="detail-section"><h4>Selected</h4>${resolvedInfo}</div>`);
    if (dec.response_text) {
      sections.push(`<div class="detail-section"><h4>Response</h4><pre>${escapeHtml(dec.response_text)}</pre></div>`);
    }
  }

  // Custom response input (only for pending decisions)
  if (state === 'pending') {
    sections.push(`<div class="detail-section decision-respond-section">
      <h4>Respond</h4>
      <input type="text" class="decision-response-input" placeholder="Custom response text..." />
      <button class="decision-send-btn">Send</button>
    </div>`);
  }

  // Iteration info
  if (dec.iteration > 0 || dec.max_iterations > 0) {
    sections.push(
      `<div class="detail-section detail-timestamps">iteration ${dec.iteration || 0}/${dec.max_iterations || 3}</div>`,
    );
  }

  // Metadata
  const meta = [];
  if (dec.requested_by) meta.push(`by: ${dec.requested_by}`);
  if (dec.urgency) meta.push(`urgency: ${dec.urgency}`);
  if (issue.created_at) meta.push(`created: ${new Date(issue.created_at).toLocaleDateString()}`);
  if (meta.length) {
    sections.push(`<div class="detail-section detail-timestamps">${meta.join(' &middot; ')}</div>`);
  }

  return sections.join('');
}

// Bind click handlers for decision option buttons and custom response (bd-9gxt1)
function bindDecisionHandlers(panel, node, resp) {
  const dec = resp.decision || {};
  const state = dec.selected_option ? 'resolved' : 'pending';
  if (state !== 'pending') return; // Already resolved — no interaction

  const graph = _getGraph();

  // Option buttons
  panel.querySelectorAll('.decision-opt').forEach((btn) => {
    btn.addEventListener('click', async () => {
      const optId = btn.dataset.optId;
      btn.classList.add('selected');
      btn.disabled = true;
      try {
        await _api.decisionResolve(node.id, optId, '');
        // Optimistic state update
        node._decisionState = 'resolved';
        const stateEl = panel.querySelector('.decision-state');
        if (stateEl) {
          stateEl.textContent = 'RESOLVED';
          stateEl.style.color = '#2d8a4e';
        }
        // Disable all buttons
        panel.querySelectorAll('.decision-opt').forEach((b) => {
          b.disabled = true;
        });
        const respondSection = panel.querySelector('.decision-respond-section');
        if (respondSection) respondSection.remove();
        _showStatusToast(`resolved ${node.id}: ${optId}`);
        // Rebuild graph node
        if (graph) graph.nodeThreeObject(graph.nodeThreeObject());
      } catch (err) {
        btn.classList.remove('selected');
        btn.disabled = false;
        _showStatusToast(`resolve failed: ${err.message}`, true);
        console.error('[beads3d] decision resolve failed:', err);
      }
    });
  });

  // Custom response send button
  const sendBtn = panel.querySelector('.decision-send-btn');
  const input = panel.querySelector('.decision-response-input');
  if (sendBtn && input) {
    const doSend = async () => {
      const text = input.value.trim();
      if (!text) return;
      sendBtn.disabled = true;
      try {
        await _api.decisionResolve(node.id, '', text);
        node._decisionState = 'resolved';
        const stateEl = panel.querySelector('.decision-state');
        if (stateEl) {
          stateEl.textContent = 'RESOLVED';
          stateEl.style.color = '#2d8a4e';
        }
        panel.querySelectorAll('.decision-opt').forEach((b) => {
          b.disabled = true;
        });
        input.value = 'Sent!';
        input.disabled = true;
        _showStatusToast(`resolved ${node.id}`);
        if (graph) graph.nodeThreeObject(graph.nodeThreeObject());
      } catch (err) {
        sendBtn.disabled = false;
        _showStatusToast(`response failed: ${err.message}`, true);
        console.error('[beads3d] decision response failed:', err);
      }
    };
    sendBtn.addEventListener('click', doSend);
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') doSend();
    });
  }
}

/**
 * Closes all currently open detail panels.
 *
 * @returns {void}
 */
export function hideDetail() {
  // Close all open panels (bd-fbmq3)
  for (const [beadId] of openPanels) {
    closeDetailPanel(beadId);
  }
}
