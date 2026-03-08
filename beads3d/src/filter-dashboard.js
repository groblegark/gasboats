// Filter Dashboard — configurable filter panel with profile persistence (bd-8o2gd, bd-7t6nt)
// Extracted from main.js to reduce monolith size.

// Callbacks set by main.js to avoid circular imports
let _applyFilters = null;
let _refresh = null;
let _syncAllRigPills = null;
let _api = null;

// Shared state references — set by main.js at init time
let _state = null;

/**
 * @param {Object} deps
 * @returns {void}
 */
export function setFilterDeps({ applyFilters, refresh, syncAllRigPills, api, state }) {
  _applyFilters = applyFilters;
  _refresh = refresh;
  _syncAllRigPills = syncAllRigPills;
  _api = api;
  _state = state;
}

/**
 * @returns {void}
 */
export function toggleFilterDashboard() {
  const panel = document.getElementById('filter-dashboard');
  if (!panel) return;
  _state.filterDashboardOpen = !_state.filterDashboardOpen;
  panel.classList.toggle('open', _state.filterDashboardOpen);
  if (_state.filterDashboardOpen) syncFilterDashboard();
}

// Sync dashboard button states to match current filter state
/**
 * @returns {void}
 */
export function syncFilterDashboard() {
  // Status buttons
  document.querySelectorAll('.fd-status').forEach((btn) => {
    const status = btn.dataset.status;
    const STATUS_GROUPS = { in_progress: ['in_progress', 'blocked', 'hooked', 'deferred'] };
    const group = STATUS_GROUPS[status] || [status];
    btn.classList.toggle(
      'active',
      group.some((s) => _state.statusFilter.has(s)),
    );
  });

  // Type buttons
  document.querySelectorAll('.fd-type').forEach((btn) => {
    btn.classList.toggle('active', _state.typeFilter.has(btn.dataset.type));
  });

  // Priority buttons
  document.querySelectorAll('.fd-priority').forEach((btn) => {
    btn.classList.toggle('active', _state.priorityFilter.has(btn.dataset.priority));
  });

  // Age buttons
  document.querySelectorAll('.fd-age').forEach((btn) => {
    const days = parseInt(btn.dataset.days, 10);
    btn.classList.toggle('active', days === _state.activeAgeDays);
  });

  // Agent toggles
  const fdShow = document.getElementById('fd-agent-show');
  const fdOrph = document.getElementById('fd-agent-orphaned');
  if (fdShow) fdShow.classList.toggle('active', _state.agentFilterShow);
  if (fdOrph) fdOrph.classList.toggle('active', _state.agentFilterOrphaned);

  // Assignee buttons
  updateAssigneeButtons();
}

// Sync toolbar controls to match dashboard changes
/**
 * @returns {void}
 */
export function syncToolbarControls() {
  // Status
  const STATUS_GROUPS = { in_progress: ['in_progress', 'blocked', 'hooked', 'deferred'] };
  document.querySelectorAll('.filter-status').forEach((btn) => {
    const status = btn.dataset.status;
    const group = STATUS_GROUPS[status] || [status];
    btn.classList.toggle(
      'active',
      group.some((s) => _state.statusFilter.has(s)),
    );
  });
  // Type
  document.querySelectorAll('.filter-type').forEach((btn) => {
    btn.classList.toggle('active', _state.typeFilter.has(btn.dataset.type));
  });
  // Age
  document.querySelectorAll('.filter-age').forEach((btn) => {
    const days = parseInt(btn.dataset.days, 10);
    btn.classList.toggle('active', days === _state.activeAgeDays);
  });
  // Agent toggles
  const btnShow = document.getElementById('btn-agent-show');
  const btnOrph = document.getElementById('btn-agent-orphaned');
  if (btnShow) btnShow.classList.toggle('active', _state.agentFilterShow);
  if (btnOrph) btnOrph.classList.toggle('active', _state.agentFilterOrphaned);
}

/**
 * @returns {void}
 */
export function updateAssigneeButtons() {
  const body = document.getElementById('fd-assignee-body');
  if (!body) return;

  // Collect unique assignees from visible graph data
  const assignees = new Set();
  for (const n of _state.graphData.nodes) {
    if (n.assignee && n.issue_type !== 'agent') assignees.add(n.assignee);
  }
  const sorted = [...assignees].sort();

  // Only rebuild if set changed
  const current = [...body.querySelectorAll('.fd-btn')].map((b) => b.dataset.assignee);
  if (current.length === sorted.length && current.every((a, i) => a === sorted[i])) {
    body.querySelectorAll('.fd-btn').forEach((btn) => {
      btn.classList.toggle('active', _state.assigneeFilter === btn.dataset.assignee);
    });
    return;
  }

  body.innerHTML = '';
  for (const name of sorted) {
    const btn = document.createElement('button');
    btn.className = 'fd-btn';
    btn.dataset.assignee = name;
    btn.textContent = name;
    if (_state.assigneeFilter === name) btn.classList.add('active');
    btn.addEventListener('click', () => {
      if (_state.assigneeFilter === name) {
        _state.assigneeFilter = '';
      } else {
        _state.assigneeFilter = name;
      }
      body.querySelectorAll('.fd-btn').forEach((b) => {
        b.classList.toggle('active', _state.assigneeFilter === b.dataset.assignee);
      });
      _applyFilters();
    });
    body.appendChild(btn);
  }
}

// ── Filter profile persistence (bd-8o2gd phase 3) ───────────────────────────

const PROFILE_KEY_PREFIX = 'beads3d.view.';

function _currentFilterState() {
  return {
    status: [..._state.statusFilter],
    types: [..._state.typeFilter],
    priority: [..._state.priorityFilter],
    age_days: _state.activeAgeDays,
    assignee: _state.assigneeFilter,
    agents: {
      show: _state.agentFilterShow,
      orphaned: _state.agentFilterOrphaned,
      rig_exclude: [..._state.agentFilterRigExclude],
      name_exclude: _state.agentFilterNameExclude.length > 0 ? [..._state.agentFilterNameExclude] : [],
    },
  };
}

function _applyFilterState(state) {
  _state.statusFilter.clear();
  (state.status || []).forEach((s) => _state.statusFilter.add(s));
  _state.typeFilter.clear();
  (state.types || []).forEach((t) => _state.typeFilter.add(t));
  _state.priorityFilter.clear();
  (state.priority || []).forEach((p) => _state.priorityFilter.add(String(p)));
  _state.activeAgeDays = state.age_days ?? 7;
  _state.assigneeFilter = state.assignee || '';
  if (state.agents) {
    _state.agentFilterShow = state.agents.show !== false;
    _state.agentFilterOrphaned = !!state.agents.orphaned;
    _state.agentFilterRigExclude.clear();
    (state.agents.rig_exclude || []).forEach((r) => _state.agentFilterRigExclude.add(r));
    _state.agentFilterNameExclude = state.agents.name_exclude || [];
    // Update the exclude input field
    const excludeInput = document.getElementById('fd-agent-exclude');
    if (excludeInput) excludeInput.value = _state.agentFilterNameExclude.join(', ');
  }
  syncFilterDashboard();
  syncToolbarControls();
  _syncAllRigPills();
  // Age changes need re-fetch; for simplicity always refresh
  _refresh();
}

/**
 * @returns {Promise<void>}
 */
export async function loadFilterProfiles() {
  const select = document.getElementById('fd-profile-select');
  if (!select) return;

  try {
    const resp = await _api.configList();
    const config = resp.config || {};
    // Clear existing options except default
    select.innerHTML = '<option value="">— default —</option>';
    const profiles = Object.keys(config)
      .filter((k) => k.startsWith(PROFILE_KEY_PREFIX))
      .map((k) => k.slice(PROFILE_KEY_PREFIX.length))
      .sort();
    for (const name of profiles) {
      const opt = document.createElement('option');
      opt.value = name;
      opt.textContent = name;
      select.appendChild(opt);
    }
    // Restore last selected profile from localStorage
    const lastProfile = localStorage.getItem('beads3d-filter-profile');
    if (lastProfile && profiles.includes(lastProfile)) {
      select.value = lastProfile;
    }
  } catch (e) {
    console.warn('[beads3d] failed to load filter profiles:', e);
  }
}

async function saveFilterProfile(name) {
  if (!name) return;
  const state = _currentFilterState();
  try {
    await _api.configSet(PROFILE_KEY_PREFIX + name, JSON.stringify(state));
    localStorage.setItem('beads3d-filter-profile', name);
    await loadFilterProfiles();
    const select = document.getElementById('fd-profile-select');
    if (select) select.value = name;
    console.log(`[beads3d] saved filter profile: ${name}`);
  } catch (e) {
    console.warn('[beads3d] failed to save filter profile:', e);
  }
}

/**
 * @param {string} name
 * @returns {Promise<void>}
 */
export async function loadFilterProfile(name) {
  if (!name) {
    // Default profile — clear all filters
    _state.statusFilter.clear();
    _state.typeFilter.clear();
    _state.priorityFilter.clear();
    _state.assigneeFilter = '';
    _state.agentFilterShow = true;
    _state.agentFilterOrphaned = false;
    _state.agentFilterRigExclude.clear();
    _state.activeAgeDays = 7;
    syncFilterDashboard();
    syncToolbarControls();
    _syncAllRigPills();
    localStorage.removeItem('beads3d-filter-profile');
    _refresh();
    return;
  }

  try {
    const resp = await _api.configGet(PROFILE_KEY_PREFIX + name);
    const state = JSON.parse(resp.value);
    _applyFilterState(state);
    localStorage.setItem('beads3d-filter-profile', name);
    console.log(`[beads3d] loaded filter profile: ${name}`);
  } catch (e) {
    console.warn(`[beads3d] failed to load profile ${name}:`, e);
  }
}

async function deleteFilterProfile(name) {
  if (!name) return;
  try {
    await _api.configUnset(PROFILE_KEY_PREFIX + name);
    localStorage.removeItem('beads3d-filter-profile');
    await loadFilterProfiles();
    const select = document.getElementById('fd-profile-select');
    if (select) select.value = '';
    console.log(`[beads3d] deleted filter profile: ${name}`);
  } catch (e) {
    console.warn(`[beads3d] failed to delete profile ${name}:`, e);
  }
}

// Apply URL query params to filter state (bd-8o2gd phase 4)
/**
 * @returns {Promise<void>}
 */
export async function applyUrlFilterParams() {
  let needRefresh = false;

  // ?profile=<name> — load a named profile
  if (_state.URL_PROFILE) {
    await loadFilterProfile(_state.URL_PROFILE);
    const select = document.getElementById('fd-profile-select');
    if (select) select.value = _state.URL_PROFILE;
    return; // profile applies all settings; skip individual params
  }

  // ?status=open,in_progress — comma-separated status filter
  if (_state.URL_STATUS) {
    _state.statusFilter.clear();
    _state.URL_STATUS.split(',').forEach((s) => _state.statusFilter.add(s.trim()));
    needRefresh = true;
  }

  // ?types=epic,bug — comma-separated type filter
  if (_state.URL_TYPES) {
    _state.typeFilter.clear();
    _state.URL_TYPES.split(',').forEach((t) => _state.typeFilter.add(t.trim()));
    needRefresh = true;
  }

  // ?assignee=cool-trout — filter by assignee
  if (_state.URL_ASSIGNEE) {
    _state.assigneeFilter = _state.URL_ASSIGNEE;
    needRefresh = true;
  }

  if (needRefresh) {
    syncFilterDashboard();
    syncToolbarControls();
    _applyFilters();
  }
}

// Generate a shareable URL with current filter state (bd-8o2gd phase 4)
/**
 * @returns {string}
 */
export function getShareableUrl() {
  const url = new URL(window.location.href);
  // Clear old filter params
  url.searchParams.delete('profile');
  url.searchParams.delete('status');
  url.searchParams.delete('types');
  url.searchParams.delete('assignee');

  // Check if current state matches a saved profile
  const select = document.getElementById('fd-profile-select');
  if (select?.value) {
    url.searchParams.set('profile', select.value);
  } else {
    // Encode individual filter params
    if (_state.statusFilter.size > 0) url.searchParams.set('status', [..._state.statusFilter].join(','));
    if (_state.typeFilter.size > 0) url.searchParams.set('types', [..._state.typeFilter].join(','));
    if (_state.assigneeFilter) url.searchParams.set('assignee', _state.assigneeFilter);
  }

  return url.toString();
}

/**
 * @returns {void}
 */
export function initFilterDashboard() {
  const panel = document.getElementById('filter-dashboard');
  if (!panel) return;

  // Close button
  document.getElementById('fd-close')?.addEventListener('click', toggleFilterDashboard);

  // Collapsible sections (bd-7zczp: add keyboard + ARIA)
  panel.querySelectorAll('.fd-section-header').forEach((header) => {
    header.setAttribute('role', 'button');
    header.setAttribute('tabindex', '0');
    const section = header.parentElement;
    header.setAttribute('aria-expanded', !section.classList.contains('collapsed'));
    const toggle = () => {
      section.classList.toggle('collapsed');
      header.setAttribute('aria-expanded', !section.classList.contains('collapsed'));
    };
    header.addEventListener('click', toggle);
    header.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        toggle();
      }
    });
  });

  const STATUS_GROUPS = {
    in_progress: ['in_progress', 'blocked', 'hooked', 'deferred'],
  };

  // Status buttons — sync with toolbar
  panel.querySelectorAll('.fd-status').forEach((btn) => {
    btn.addEventListener('click', () => {
      const status = btn.dataset.status;
      const group = STATUS_GROUPS[status] || [status];
      btn.classList.toggle('active');
      if (_state.statusFilter.has(status)) {
        group.forEach((s) => _state.statusFilter.delete(s));
      } else {
        group.forEach((s) => _state.statusFilter.add(s));
      }
      syncToolbarControls();
      _applyFilters();
    });
  });

  // Type buttons — sync with toolbar
  panel.querySelectorAll('.fd-type').forEach((btn) => {
    btn.addEventListener('click', () => {
      const type = btn.dataset.type;
      btn.classList.toggle('active');
      if (_state.typeFilter.has(type)) {
        _state.typeFilter.delete(type);
      } else {
        _state.typeFilter.add(type);
      }
      syncToolbarControls();
      _applyFilters();
    });
  });

  // Priority buttons (bd-8o2gd phase 2)
  panel.querySelectorAll('.fd-priority').forEach((btn) => {
    btn.addEventListener('click', () => {
      const p = btn.dataset.priority;
      btn.classList.toggle('active');
      if (_state.priorityFilter.has(p)) {
        _state.priorityFilter.delete(p);
      } else {
        _state.priorityFilter.add(p);
      }
      _applyFilters();
    });
  });

  // Age buttons — sync with toolbar (triggers re-fetch)
  panel.querySelectorAll('.fd-age').forEach((btn) => {
    btn.addEventListener('click', () => {
      const newDays = parseInt(btn.dataset.days, 10);
      if (newDays === _state.activeAgeDays) return;
      panel.querySelectorAll('.fd-age').forEach((b) => b.classList.remove('active'));
      btn.classList.add('active');
      _state.activeAgeDays = newDays;
      syncToolbarControls();
      _refresh();
    });
  });

  // Agent show/orphaned toggles — sync with toolbar
  document.getElementById('fd-agent-show')?.addEventListener('click', () => {
    _state.agentFilterShow = !_state.agentFilterShow;
    document.getElementById('fd-agent-show')?.classList.toggle('active', _state.agentFilterShow);
    syncToolbarControls();
    _applyFilters();
  });

  document.getElementById('fd-agent-orphaned')?.addEventListener('click', () => {
    _state.agentFilterOrphaned = !_state.agentFilterOrphaned;
    document.getElementById('fd-agent-orphaned')?.classList.toggle('active', _state.agentFilterOrphaned);
    syncToolbarControls();
    _applyFilters();
  });

  // Agent name exclusion input — glob patterns, comma-separated (bd-8o2gd phase 4)
  const agentExcludeInput = document.getElementById('fd-agent-exclude');
  if (agentExcludeInput) {
    agentExcludeInput.addEventListener('input', () => {
      const val = agentExcludeInput.value.trim();
      _state.agentFilterNameExclude = val
        ? val
            .split(',')
            .map((p) => p.trim().toLowerCase())
            .filter(Boolean)
        : [];
      _applyFilters();
    });
  }

  // Share button — copy shareable URL to clipboard (bd-8o2gd phase 4)
  document.getElementById('fd-share')?.addEventListener('click', () => {
    const url = getShareableUrl();
    navigator.clipboard
      .writeText(url)
      .then(() => {
        const btn = document.getElementById('fd-share');
        if (btn) {
          btn.textContent = 'copied!';
          setTimeout(() => {
            btn.textContent = 'share';
          }, 1500);
        }
      })
      .catch(() => {
        prompt('Copy this URL:', url);
      });
  });

  // Reset button
  document.getElementById('fd-reset')?.addEventListener('click', () => {
    _state.statusFilter.clear();
    _state.typeFilter.clear();
    _state.priorityFilter.clear();
    _state.assigneeFilter = '';
    _state.agentFilterShow = true;
    _state.agentFilterOrphaned = false;
    _state.agentFilterRigExclude.clear();
    _state.agentFilterNameExclude = [];
    const excludeInput = document.getElementById('fd-agent-exclude');
    if (excludeInput) excludeInput.value = '';
    _state.activeAgeDays = 7;
    syncFilterDashboard();
    syncToolbarControls();
    _syncAllRigPills();
    _refresh();
  });

  // ── Profile persistence (bd-8o2gd phase 3) ──────────────────────────────

  const profileSelect = document.getElementById('fd-profile-select');
  const btnSave = document.getElementById('fd-profile-save');
  const btnSaveAs = document.getElementById('fd-profile-save-as');
  const btnDelete = document.getElementById('fd-profile-delete');

  // Load profile list, then apply URL params (bd-8o2gd phase 4)
  loadFilterProfiles().then(() => {
    applyUrlFilterParams();
  });

  // Profile dropdown change — load selected profile
  profileSelect?.addEventListener('change', () => {
    loadFilterProfile(profileSelect.value);
  });

  // Save — overwrite currently selected profile
  btnSave?.addEventListener('click', () => {
    const name = profileSelect?.value;
    if (!name) {
      // No profile selected — prompt for name
      const newName = prompt('Profile name:');
      if (newName) saveFilterProfile(newName.trim());
    } else {
      saveFilterProfile(name);
    }
  });

  // Save As — always prompt for new name
  btnSaveAs?.addEventListener('click', () => {
    const newName = prompt('New profile name:');
    if (newName) saveFilterProfile(newName.trim());
  });

  // Delete — remove currently selected profile
  btnDelete?.addEventListener('click', () => {
    const name = profileSelect?.value;
    if (!name) return;
    if (confirm(`Delete profile "${name}"?`)) {
      deleteFilterProfile(name);
    }
  });
}

/**
 * @returns {void}
 */
export function updateFilterCount() {
  const visible = _state.graphData.nodes.filter((n) => !n._hidden).length;
  const total = _state.graphData.nodes.length;
  const el = document.getElementById('filter-count');
  if (el) {
    if (_state.searchResults.length > 0) {
      el.textContent = `${_state.searchResultIdx + 1}/${_state.searchResults.length} matches · ${visible}/${total}`;
    } else if (visible < total) {
      el.textContent = `${visible}/${total}`;
    } else {
      el.textContent = `${total}`;
    }
  }
  // Update filter dashboard node count (bd-8o2gd phase 2)
  const fdCount = document.getElementById('fd-node-count');
  if (fdCount) fdCount.textContent = `${visible}/${total} nodes`;
}
