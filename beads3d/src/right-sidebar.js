// Right sidebar: epic progress, dependency health (bd-7t6nt, bd-9cpbc)
// Extracted from main.js to reduce monolith size.
// Decision queue moved to decision-lightbox.js (beads-zuc3).

// Callback for node click — set by main.js to avoid circular import
let _onNodeClick = null;
/**
 * @param {Function} fn
 * @returns {void}
 */
export function setOnNodeClick(fn) {
  _onNodeClick = fn;
}

function escapeHtml(str) {
  return String(str || '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

let rightSidebarCollapsed = false;

/**
 * @returns {void}
 */
export function toggleRightSidebar() {
  const sidebar = document.getElementById('right-sidebar');
  if (!sidebar) return;
  rightSidebarCollapsed = !rightSidebarCollapsed;
  sidebar.classList.toggle('collapsed', rightSidebarCollapsed);
  const btn = document.getElementById('rs-collapse');
  if (btn) btn.innerHTML = rightSidebarCollapsed ? '&#x25C0;' : '&#x25B6;';
  // Shift controls bar when sidebar collapses/expands (bd-kj8e5)
  const controls = document.getElementById('controls');
  if (controls) controls.classList.toggle('sidebar-collapsed', rightSidebarCollapsed);
}

/**
 * @returns {void}
 */
export function initRightSidebar() {
  const sidebar = document.getElementById('right-sidebar');
  if (!sidebar) return;

  // Collapse button
  const collapseBtn = document.getElementById('rs-collapse');
  if (collapseBtn) collapseBtn.onclick = () => toggleRightSidebar();

  // Collapsible sections (bd-7zczp: add keyboard + ARIA)
  sidebar.querySelectorAll('.rs-section-header').forEach((header) => {
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
}

/**
 * @param {Object} graphData
 * @returns {void}
 */
export function updateRightSidebar(graphData) {
  if (!graphData || rightSidebarCollapsed) return;
  updateEpicProgress(graphData);
  updateDepHealth(graphData);
}

/**
 * @param {Object} graphData
 * @returns {void}
 */
export function updateEpicProgress(graphData) {
  const body = document.getElementById('rs-epics-body');
  if (!body || !graphData) return;

  // Find all epic nodes
  const epics = graphData.nodes.filter((n) => n.issue_type === 'epic' && !n._hidden);
  if (epics.length === 0) {
    body.innerHTML = '<div class="rs-empty">no epics</div>';
    return;
  }

  // For each epic, find children via parent-child links
  const html = epics
    .map((epic) => {
      const children = graphData.links
        .filter(
          (l) => l.dep_type === 'parent-child' && (typeof l.source === 'object' ? l.source.id : l.source) === epic.id,
        )
        .map((l) => {
          const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
          return graphData.nodes.find((n) => n.id === tgtId);
        })
        .filter(Boolean);

      const total = children.length;
      if (total === 0) return '';

      const closed = children.filter((c) => c.status === 'closed').length;
      const active = children.filter((c) => c.status === 'in_progress').length;
      const blocked = children.filter((c) => c._blocked).length;
      const pct = Math.round((closed / total) * 100);

      const closedW = (closed / total) * 100;
      const activeW = (active / total) * 100;
      const blockedW = (blocked / total) * 100;

      const name = epic.title || epic.id.replace(/^[a-z]+-/, '');
      return `<div class="rs-epic-item" data-node-id="${escapeHtml(epic.id)}" title="${escapeHtml(epic.id)}: ${escapeHtml(epic.title || '')}">
      <div class="rs-epic-name">${escapeHtml(name)} <span class="rs-epic-pct">${pct}%</span></div>
      <div class="rs-epic-bar">
        <span style="width:${closedW}%;background:#2d8a4e"></span>
        <span style="width:${activeW}%;background:#d4a017"></span>
        <span style="width:${blockedW}%;background:#d04040"></span>
      </div>
    </div>`;
    })
    .filter(Boolean)
    .join('');

  body.innerHTML = html || '<div class="rs-empty">no epics with children</div>';

  // Click to fly to epic
  body.querySelectorAll('.rs-epic-item').forEach((el) => {
    el.onclick = () => {
      const nodeId = el.dataset.nodeId;
      const node = graphData.nodes.find((n) => n.id === nodeId);
      if (node && _onNodeClick) _onNodeClick(node);
    };
  });
}

/**
 * @param {Object} graphData
 * @returns {void}
 */
export function updateDepHealth(graphData) {
  const body = document.getElementById('rs-health-body');
  if (!body || !graphData) return;

  const blocked = graphData.nodes.filter((n) => n._blocked && !n._hidden && n.status !== 'closed');
  if (blocked.length === 0) {
    body.innerHTML = '<div class="rs-empty">no blocked items</div>';
    return;
  }

  const html = blocked
    .slice(0, 15)
    .map((n) => {
      const name = n.title || n.id.replace(/^[a-z]+-/, '');
      return `<div class="rs-blocked-item" data-node-id="${escapeHtml(n.id)}" title="${escapeHtml(n.id)}">${escapeHtml(name)}</div>`;
    })
    .join('');

  body.innerHTML = `<div style="font-size:9px;color:#d04040;margin-bottom:4px">${blocked.length} blocked</div>${html}`;

  body.querySelectorAll('.rs-blocked-item').forEach((el) => {
    el.onclick = () => {
      const node = graphData.nodes.find((n) => n.id === el.dataset.nodeId);
      if (node && _onNodeClick) _onNodeClick(node);
    };
  });
}

// updateDecisionQueue removed — decisions now rendered by decision-lightbox.js (beads-zuc3)
/** @deprecated Use decision-lightbox.js updateDecisionList() instead */
export function updateDecisionQueue() {}
