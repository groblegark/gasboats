// Decision lightbox — floating window for decision interactions (beads-zuc3)
// Replaces sidebar decision queue with a spacious lightbox overlay.

let _api = null;
let _showStatusToast = null;
let _getGraph = null; // reserved for graph node refresh after resolve

let _visible = false;
let _selectedId = null;
let _decisions = []; // cached decision nodes from graph data
let _dragState = null; // { startX, startY, origLeft, origTop }
let _resizeState = null; // { startX, startY, origW, origH }
let _customPos = null; // { left, top } — remembers position during session
let _customSize = null; // { width, height } — remembers size during session

/**
 * Inject dependencies from main.js.
 * @param {Object} deps
 * @param {Object} deps.api - BeadsAPI instance
 * @param {Function} deps.showStatusToast - (msg, isError?) => void
 * @param {Function} deps.getGraph - () => ForceGraph3D instance
 * @returns {void}
 */
export function setDecisionLightboxDeps({ api, showStatusToast, getGraph }) {
  _api = api;
  _showStatusToast = showStatusToast;
  _getGraph = getGraph;
}

function escapeHtml(str) {
  if (!str) return '';
  const div = document.createElement('div');
  div.textContent = str;
  return div.innerHTML;
}

function timeAgo(dateStr) {
  if (!dateStr) return '';
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 1) return 'just now';
  if (mins < 60) return `${mins}m ago`;
  const hours = Math.floor(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  return `${Math.floor(hours / 24)}d ago`;
}

/**
 * Initialize the decision lightbox (event listeners, keyboard, drag).
 * @returns {void}
 */
export function initDecisionLightbox() {
  const backdrop = document.getElementById('decision-lightbox-backdrop');
  const lightbox = document.getElementById('decision-lightbox');
  const closeBtn = document.getElementById('dlb-close');
  const header = document.getElementById('dlb-header');
  const hudBtn = document.getElementById('hud-btn-decisions');

  if (!lightbox) return;

  // Close handlers
  if (closeBtn) closeBtn.addEventListener('click', hideDecisionLightbox);
  if (backdrop) backdrop.addEventListener('click', hideDecisionLightbox);
  if (hudBtn) hudBtn.addEventListener('click', toggleDecisionLightbox);

  // Keyboard
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && _visible) {
      hideDecisionLightbox();
      e.stopPropagation();
    }
  });

  // Global 'd' shortcut (when not in input)
  document.addEventListener('keydown', (e) => {
    if (e.target.tagName === 'INPUT' || e.target.tagName === 'TEXTAREA' || e.target.isContentEditable) return;
    if (e.key === 'd' && !e.ctrlKey && !e.metaKey && !e.altKey) {
      toggleDecisionLightbox();
    }
  });

  // Drag support on header
  if (header) {
    header.addEventListener('mousedown', (e) => {
      if (e.target.closest('button')) return; // Don't drag when clicking close
      const rect = lightbox.getBoundingClientRect();
      _dragState = {
        startX: e.clientX,
        startY: e.clientY,
        origLeft: rect.left,
        origTop: rect.top,
      };
      e.preventDefault();
    });

    document.addEventListener('mousemove', (e) => {
      if (!_dragState) return;
      const dx = e.clientX - _dragState.startX;
      const dy = e.clientY - _dragState.startY;
      const newLeft = Math.max(0, Math.min(window.innerWidth - 100, _dragState.origLeft + dx));
      const newTop = Math.max(0, Math.min(window.innerHeight - 50, _dragState.origTop + dy));
      lightbox.style.left = `${newLeft}px`;
      lightbox.style.top = `${newTop}px`;
      lightbox.style.transform = 'none';
      _customPos = { left: newLeft, top: newTop };
    });

    document.addEventListener('mouseup', () => {
      _dragState = null;
    });

    // Touch drag support (beads-7kez)
    header.addEventListener('touchstart', (e) => {
      if (e.target.closest('button')) return;
      const touch = e.touches[0];
      const rect = lightbox.getBoundingClientRect();
      _dragState = {
        startX: touch.clientX,
        startY: touch.clientY,
        origLeft: rect.left,
        origTop: rect.top,
      };
    }, { passive: true });

    document.addEventListener('touchmove', (e) => {
      if (!_dragState) return;
      const touch = e.touches[0];
      const dx = touch.clientX - _dragState.startX;
      const dy = touch.clientY - _dragState.startY;
      const newLeft = Math.max(0, Math.min(window.innerWidth - 100, _dragState.origLeft + dx));
      const newTop = Math.max(0, Math.min(window.innerHeight - 50, _dragState.origTop + dy));
      lightbox.style.left = `${newLeft}px`;
      lightbox.style.top = `${newTop}px`;
      lightbox.style.transform = 'none';
      _customPos = { left: newLeft, top: newTop };
    }, { passive: true });

    document.addEventListener('touchend', () => {
      _dragState = null;
    });

    // Double-click to reset position and size
    header.addEventListener('dblclick', () => {
      lightbox.style.left = '50%';
      lightbox.style.top = '12%';
      lightbox.style.transform = 'translateX(-50%)';
      lightbox.style.width = '';
      lightbox.style.height = '';
      _customPos = null;
      _customSize = null;
    });
  }

  // Resize handle (beads-7kez)
  const resizeHandle = document.getElementById('dlb-resize-handle');
  if (resizeHandle && lightbox) {
    const startResize = (startX, startY) => {
      const rect = lightbox.getBoundingClientRect();
      _resizeState = { startX, startY, origW: rect.width, origH: rect.height };
    };

    resizeHandle.addEventListener('mousedown', (e) => {
      startResize(e.clientX, e.clientY);
      e.preventDefault();
    });
    resizeHandle.addEventListener('touchstart', (e) => {
      const touch = e.touches[0];
      startResize(touch.clientX, touch.clientY);
    }, { passive: true });

    document.addEventListener('mousemove', (e) => {
      if (!_resizeState) return;
      applyResize(e.clientX, e.clientY, lightbox);
    });
    document.addEventListener('touchmove', (e) => {
      if (!_resizeState) return;
      const touch = e.touches[0];
      applyResize(touch.clientX, touch.clientY, lightbox);
    }, { passive: true });

    const endResize = () => { _resizeState = null; };
    document.addEventListener('mouseup', endResize);
    document.addEventListener('touchend', endResize);
  }
}

function applyResize(clientX, clientY, lightbox) {
  if (!_resizeState) return;
  const dw = clientX - _resizeState.startX;
  const dh = clientY - _resizeState.startY;
  const newW = Math.max(300, Math.min(window.innerWidth - 40, _resizeState.origW + dw));
  const newH = Math.max(200, Math.min(window.innerHeight - 40, _resizeState.origH + dh));
  lightbox.style.width = `${newW}px`;
  lightbox.style.height = `${newH}px`;
  _customSize = { width: newW, height: newH };
}

/**
 * Show the decision lightbox.
 * @param {string} [focusId] - Optional decision ID to focus on.
 * @returns {void}
 */
export function showDecisionLightbox(focusId) {
  const backdrop = document.getElementById('decision-lightbox-backdrop');
  const lightbox = document.getElementById('decision-lightbox');
  if (!lightbox) return;

  backdrop.style.display = 'block';
  lightbox.style.display = 'flex';
  lightbox.setAttribute('aria-hidden', 'false');
  backdrop.setAttribute('aria-hidden', 'false');

  // Restore custom position and size if set (beads-7kez)
  if (_customPos) {
    lightbox.style.left = `${_customPos.left}px`;
    lightbox.style.top = `${_customPos.top}px`;
    lightbox.style.transform = 'none';
  }
  if (_customSize) {
    lightbox.style.width = `${_customSize.width}px`;
    lightbox.style.height = `${_customSize.height}px`;
  }

  // Trigger animation
  requestAnimationFrame(() => {
    backdrop.classList.add('show');
    lightbox.classList.add('show');
  });

  _visible = true;
  document.body.classList.add('dlb-active'); // suppress popups/toasts (bd-7m6td)

  // Render list
  renderList();

  // Focus requested decision or first pending
  if (focusId) {
    selectDecision(focusId);
  } else if (!_selectedId || !_decisions.find((d) => d.id === _selectedId)) {
    const firstPending = _decisions.find((d) => d.status !== 'closed');
    if (firstPending) selectDecision(firstPending.id);
    else if (_decisions.length > 0) selectDecision(_decisions[0].id);
    else renderEmptyDetail();
  }

  // Focus the close button for keyboard nav
  const closeBtn = document.getElementById('dlb-close');
  if (closeBtn) closeBtn.focus();
}

/**
 * Hide the decision lightbox.
 * @returns {void}
 */
export function hideDecisionLightbox() {
  const backdrop = document.getElementById('decision-lightbox-backdrop');
  const lightbox = document.getElementById('decision-lightbox');
  if (!lightbox) return;

  backdrop.classList.remove('show');
  lightbox.classList.remove('show');
  _visible = false;
  document.body.classList.remove('dlb-active'); // re-enable popups/toasts (bd-7m6td)

  setTimeout(() => {
    backdrop.style.display = 'none';
    lightbox.style.display = 'none';
    lightbox.setAttribute('aria-hidden', 'true');
    backdrop.setAttribute('aria-hidden', 'true');
  }, 250);
}

/**
 * Toggle the decision lightbox visibility.
 * @returns {void}
 */
export function toggleDecisionLightbox() {
  if (_visible) hideDecisionLightbox();
  else showDecisionLightbox();
}

/**
 * Update the decision list from graph data. Called on every graph refresh.
 * @param {Object} graphData - Graph data with nodes array.
 * @returns {void}
 */
export function updateDecisionList(graphData) {
  if (!graphData) return;

  // Find decision nodes
  _decisions = graphData.nodes.filter(
    (n) =>
      (n.issue_type === 'decision' || (n.issue_type === 'gate' && n.await_type === 'decision')) && !n._hidden,
  );

  // Sort: pending first, then by created_at descending
  _decisions.sort((a, b) => {
    const aResolved = a.status === 'closed' ? 1 : 0;
    const bResolved = b.status === 'closed' ? 1 : 0;
    if (aResolved !== bResolved) return aResolved - bResolved;
    return (b.created_at || '').localeCompare(a.created_at || '');
  });

  // Update HUD badge
  const pendingCount = _decisions.filter((d) => d.status !== 'closed').length;
  updateBadge(pendingCount);

  // Re-render list if visible
  if (_visible) {
    renderList();
  }
}

function updateBadge(count) {
  const hudBadge = document.getElementById('hud-decisions-badge');
  const dlbBadge = document.getElementById('dlb-badge');
  if (hudBadge) {
    hudBadge.textContent = count;
    hudBadge.classList.toggle('zero', count === 0);
  }
  if (dlbBadge) {
    dlbBadge.textContent = count;
    dlbBadge.classList.toggle('zero', count === 0);
  }
}

function renderList() {
  const listEl = document.getElementById('dlb-list');
  if (!listEl) return;

  if (_decisions.length === 0) {
    listEl.innerHTML = '<div class="dlb-empty">No decisions</div>';
    renderEmptyDetail();
    return;
  }

  listEl.innerHTML = _decisions
    .map((d) => {
      const isPending = d.status !== 'closed';
      const cls = `dlb-list-item ${isPending ? 'pending' : 'resolved'} ${d.id === _selectedId ? 'active' : ''}`;
      const prompt = d.title || d.id;
      const truncated = prompt.length > 60 ? prompt.slice(0, 57) + '...' : prompt;
      const agent = d.assignee || d._requestedBy || '';
      return `<div class="${cls}" data-decision-id="${escapeHtml(d.id)}" role="option" tabindex="0" aria-selected="${d.id === _selectedId}">
        <div>${escapeHtml(truncated)}</div>
        ${agent ? `<div class="dlb-list-agent">${escapeHtml(agent)}</div>` : ''}
        <div class="dlb-list-time">${timeAgo(d.created_at)}</div>
      </div>`;
    })
    .join('');

  // Click handlers
  listEl.querySelectorAll('.dlb-list-item').forEach((el) => {
    el.addEventListener('click', () => selectDecision(el.dataset.decisionId));
    el.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        selectDecision(el.dataset.decisionId);
      }
    });
  });
}

function renderEmptyDetail() {
  const detailEl = document.getElementById('dlb-detail');
  if (detailEl) {
    detailEl.innerHTML = '<div class="dlb-empty">Select a decision from the list</div>';
  }
}

async function selectDecision(id) {
  _selectedId = id;

  // Update list active state
  const listEl = document.getElementById('dlb-list');
  if (listEl) {
    listEl.querySelectorAll('.dlb-list-item').forEach((el) => {
      const isActive = el.dataset.decisionId === id;
      el.classList.toggle('active', isActive);
      el.setAttribute('aria-selected', isActive);
    });
  }

  const detailEl = document.getElementById('dlb-detail');
  if (!detailEl) return;

  detailEl.innerHTML = '<div class="dlb-empty">Loading...</div>';

  const node = _decisions.find((d) => d.id === id);
  if (!node) {
    detailEl.innerHTML = '<div class="dlb-empty">Decision not found</div>';
    return;
  }

  // Fetch full decision data from API
  try {
    const resp = await _api.decisionGet(node.id);
    renderDecisionDetail(detailEl, node, resp);
  } catch (err) {
    detailEl.innerHTML = `<div class="dlb-empty">Could not load: ${escapeHtml(err.message)}</div>`;
  }
}

function renderDecisionDetail(container, node, resp) {
  const dec = resp.decision || {};
  const sections = [];

  // State
  const state = dec.selected_option ? 'resolved' : node.status === 'closed' ? 'resolved' : 'pending';
  sections.push(`<div class="dlb-state ${state}">${state.toUpperCase()}</div>`);

  // Update border color
  const lightbox = document.getElementById('decision-lightbox');
  if (lightbox) lightbox.classList.toggle('resolved-border', state === 'resolved');

  // Prompt
  if (dec.prompt) {
    sections.push(`<div class="dlb-section"><h4>Question</h4><div class="dlb-prompt">${escapeHtml(dec.prompt)}</div></div>`);
  }

  // Context
  if (dec.context) {
    sections.push(`<div class="dlb-section"><h4>Context</h4><div class="dlb-context">${escapeHtml(dec.context)}</div></div>`);
  }

  // Options
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
      .map((opt) => {
        const selected = dec.selected_option === opt.id;
        const cls = selected ? 'dlb-opt selected' : 'dlb-opt';
        const disabled = state !== 'pending' ? 'disabled' : '';
        const label = opt.label || opt.short || opt.id;
        const beadRef = opt.bead_id ? `<span class="dlb-opt-bead">(${escapeHtml(opt.bead_id)})</span>` : '';
        return `<button class="${cls}" data-opt-id="${escapeHtml(opt.id)}" ${disabled}>${escapeHtml(label)} ${beadRef}</button>`;
      })
      .join('');
    sections.push(`<div class="dlb-section"><h4>Options</h4><div class="dlb-options">${optHtml}</div></div>`);
  }

  // Resolution info
  if (dec.selected_option) {
    const selectedOpt = opts.find((o) => o.id === dec.selected_option);
    const selectedLabel = selectedOpt ? selectedOpt.label || selectedOpt.short || selectedOpt.id : dec.selected_option;
    let info = `<div class="dlb-selected-label">${escapeHtml(selectedLabel)}</div>`;
    if (dec.responded_by) info += `<div class="dlb-meta">by ${escapeHtml(dec.responded_by)}`;
    if (dec.responded_at) info += ` at ${new Date(dec.responded_at).toLocaleString()}`;
    if (dec.responded_by) info += '</div>';
    sections.push(`<div class="dlb-section"><h4>Selected</h4>${info}</div>`);
    if (dec.response_text) {
      sections.push(`<div class="dlb-section"><h4>Response</h4><div class="dlb-context">${escapeHtml(dec.response_text)}</div></div>`);
    }
  }

  // Custom response (pending only)
  if (state === 'pending') {
    sections.push(`<div class="dlb-section"><h4>Respond</h4>
      <div class="dlb-respond">
        <textarea class="dlb-response-input" placeholder="Custom response text..." rows="2"></textarea>
        <button class="dlb-send-btn">Send</button>
      </div>
    </div>`);
  }

  // Metadata
  const meta = [];
  if (dec.requested_by) meta.push(`by: ${dec.requested_by}`);
  if (dec.urgency) meta.push(`urgency: ${dec.urgency}`);
  if (dec.iteration > 0 || dec.max_iterations > 0) {
    meta.push(`iteration ${dec.iteration || 0}/${dec.max_iterations || 3}`);
  }
  if (meta.length) {
    sections.push(`<div class="dlb-section"><div class="dlb-meta">${meta.join(' &middot; ')}</div></div>`);
  }

  container.innerHTML = sections.join('');

  // Bind handlers
  if (state === 'pending') {
    bindOptionHandlers(container, node);
  }
}

function bindOptionHandlers(container, node) {
  // Option buttons
  container.querySelectorAll('.dlb-opt').forEach((btn) => {
    btn.addEventListener('click', async () => {
      const optId = btn.dataset.optId;
      btn.classList.add('selected');
      container.querySelectorAll('.dlb-opt').forEach((b) => {
        b.disabled = true;
      });

      try {
        await _api.decisionResolve(node.id, optId, '');
        node._decisionState = 'resolved';

        // Update state badge
        const stateEl = container.querySelector('.dlb-state');
        if (stateEl) {
          stateEl.textContent = 'RESOLVED';
          stateEl.className = 'dlb-state resolved';
        }

        // Remove respond section
        const respondSection = container.querySelector('.dlb-respond');
        if (respondSection) respondSection.closest('.dlb-section').remove();

        // Update border
        const lightbox = document.getElementById('decision-lightbox');
        if (lightbox) lightbox.classList.add('resolved-border');

        _showStatusToast(`resolved ${node.id}: ${optId}`);

        // Refresh list
        const listItem = document.querySelector(`.dlb-list-item[data-decision-id="${node.id}"]`);
        if (listItem) {
          listItem.classList.remove('pending');
          listItem.classList.add('resolved');
        }
      } catch (err) {
        btn.classList.remove('selected');
        container.querySelectorAll('.dlb-opt').forEach((b) => {
          b.disabled = false;
        });
        _showStatusToast(`resolve failed: ${err.message}`, true);
      }
    });
  });

  // Custom response
  const sendBtn = container.querySelector('.dlb-send-btn');
  const input = container.querySelector('.dlb-response-input');
  if (sendBtn && input) {
    const doSend = async () => {
      const text = input.value.trim();
      if (!text) return;
      sendBtn.disabled = true;

      try {
        await _api.decisionResolve(node.id, '', text);
        node._decisionState = 'resolved';

        const stateEl = container.querySelector('.dlb-state');
        if (stateEl) {
          stateEl.textContent = 'RESOLVED';
          stateEl.className = 'dlb-state resolved';
        }

        container.querySelectorAll('.dlb-opt').forEach((b) => {
          b.disabled = true;
        });

        input.value = 'Sent!';
        input.disabled = true;

        const lightbox = document.getElementById('decision-lightbox');
        if (lightbox) lightbox.classList.add('resolved-border');

        _showStatusToast(`resolved ${node.id}`);

        const listItem = document.querySelector(`.dlb-list-item[data-decision-id="${node.id}"]`);
        if (listItem) {
          listItem.classList.remove('pending');
          listItem.classList.add('resolved');
        }
      } catch (err) {
        sendBtn.disabled = false;
        _showStatusToast(`response failed: ${err.message}`, true);
      }
    };

    sendBtn.addEventListener('click', doSend);
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        doSend();
      }
    });
  }
}
