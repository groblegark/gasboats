// Beads API client — supports both bd-daemon (Connect-RPC) and kbeads (REST)
// Uses Vite proxy in dev (/api → backend), direct URL in prod

/** @type {string} Default base URL for API requests */
const DEFAULT_BASE = '/api';

// SSE reconnection with exponential backoff (bd-ki6im)
/** @type {number} Initial delay in ms before first SSE reconnection attempt */
const SSE_INITIAL_DELAY = 1000;
/** @type {number} Maximum delay in ms between SSE reconnection attempts */
const SSE_MAX_DELAY = 30000;
/** @type {number} Maximum number of SSE reconnection attempts before giving up */
const SSE_MAX_RETRIES = 50;
/** @type {number} Multiplier applied to delay after each failed reconnection attempt */
const SSE_BACKOFF_FACTOR = 2;

/**
 * HTTP client for beads backends.
 * Supports two modes:
 * - 'rpc' (default): Connect-RPC JSON via bd-daemon (POST /api/bd.v1.BeadsService/<Method>)
 * - 'rest': kbeads REST API (GET/POST/PATCH /api/v1/...)
 * @class
 */
export class BeadsAPI {
  /**
   * Create a new BeadsAPI client.
   * @param {string} [baseUrl='/api'] - Base URL for API requests
   * @param {Object} [opts={}] - Options
   * @param {string} [opts.mode='rpc'] - API mode: 'rpc' for bd-daemon, 'rest' for kbeads
   */
  constructor(baseUrl = DEFAULT_BASE, opts = {}) {
    this.baseUrl = baseUrl;
    this.mode = opts.mode || 'rest';
    this._eventSources = [];
    this._reconnectManagers = [];
  }

  // ────────── Transport helpers ──────────

  /**
   * Make a Connect-RPC JSON call to the beads daemon.
   * @param {string} method - The RPC method name (e.g. 'List', 'Show')
   * @param {Object} [body={}] - The request body to send as JSON
   * @returns {Promise<Object>} The parsed JSON response
   * @throws {Error} If the HTTP response is not ok
   * @private
   */
  async _rpc(method, body = {}) {
    const headers = {
      'Content-Type': 'application/json',
      'Connect-Protocol-Version': '1',
    };

    const resp = await fetch(`${this.baseUrl}/bd.v1.BeadsService/${method}`, {
      method: 'POST',
      headers,
      body: JSON.stringify(body),
    });

    if (!resp.ok) {
      const text = await resp.text().catch(() => '');
      throw new Error(`RPC ${method}: ${resp.status} ${text.slice(0, 100)}`);
    }

    return resp.json();
  }

  /**
   * Make a REST call to the kbeads API.
   * @param {string} method - HTTP method (GET, POST, PATCH, DELETE)
   * @param {string} path - URL path relative to baseUrl (e.g. '/v1/beads')
   * @param {Object} [body] - Optional JSON body for POST/PATCH/PUT
   * @returns {Promise<Object>} The parsed JSON response
   * @throws {Error} If the HTTP response is not ok
   * @private
   */
  async _rest(method, path, body) {
    const opts = { method, headers: {} };
    if (body !== undefined) {
      opts.headers['Content-Type'] = 'application/json';
      opts.body = JSON.stringify(body);
    }

    const resp = await fetch(`${this.baseUrl}${path}`, opts);

    if (!resp.ok) {
      const text = await resp.text().catch(() => '');
      throw new Error(`REST ${method} ${path}: ${resp.status} ${text.slice(0, 100)}`);
    }

    return resp.json();
  }

  // ────────── Read operations ──────────

  /**
   * Ping/health check.
   * @returns {Promise<Object>} Status response
   */
  async ping() {
    if (this.mode === 'rest') return this._rest('GET', '/v1/health');
    return this._rpc('Ping', {});
  }

  /**
   * Fetch the full graph for 3D visualization.
   * @param {Object} [opts={}] - Graph query parameters
   * @returns {Promise<{nodes: Object[], edges: Object[], stats: Object}>} Graph data
   */
  async graph(opts = {}) {
    const defaults = { limit: 500, include_deps: true, include_agents: true };
    if (this.mode === 'rest') return this._rest('POST', '/v1/graph', { ...defaults, ...opts });
    return this._rpc('Graph', { ...defaults, ...opts });
  }

  /**
   * List beads/issues with optional filtering.
   * @param {Object} [opts={}] - List query parameters
   * @returns {Promise<Object>} List response
   */
  async list(opts = {}) {
    if (this.mode === 'rest') {
      const params = new URLSearchParams();
      const merged = { limit: 500, ...opts };
      if (merged.limit) params.set('limit', String(merged.limit));
      if (merged.status) {
        for (const s of [].concat(merged.status)) params.append('status', s);
      }
      if (merged.type) params.set('type', merged.type);
      if (merged.assignee) params.set('assignee', merged.assignee);
      if (merged.search) params.set('search', merged.search);
      const qs = params.toString();
      return this._rest('GET', `/v1/beads${qs ? '?' + qs : ''}`);
    }
    return this._rpc('List', { limit: 500, exclude_status: ['tombstone'], ...opts });
  }

  /**
   * Fetch full details for a single bead/issue.
   * @param {string} id - The bead/issue ID
   * @returns {Promise<Object>} The bead object
   */
  async show(id) {
    if (this.mode === 'rest') return this._rest('GET', `/v1/beads/${id}`);
    return this._rpc('Show', { id });
  }

  /**
   * Fetch aggregate statistics.
   * @returns {Promise<Object>} Stats object
   */
  async stats() {
    if (this.mode === 'rest') return this._rest('GET', '/v1/stats');
    return this._rpc('Stats', {});
  }

  /**
   * Fetch beads that are ready to work on (unblocked, open).
   * @returns {Promise<Object>} Response with ready beads
   */
  async ready() {
    if (this.mode === 'rest') return this._rest('GET', '/v1/ready?limit=200');
    return this._rpc('Ready', { limit: 200 });
  }

  /**
   * Fetch beads that are currently blocked.
   * @returns {Promise<Object>} Response with blocked beads
   */
  async blocked() {
    if (this.mode === 'rest') return this._rest('GET', '/v1/blocked');
    return this._rpc('Blocked', {});
  }

  /**
   * Fetch the dependency tree rooted at a given bead.
   * @param {string} id - The root bead ID
   * @param {number} [maxDepth=5] - Maximum depth to traverse
   * @returns {Promise<Object>} Tree structure
   */
  async depTree(id, maxDepth = 5) {
    if (this.mode === 'rest') {
      return this._rest('GET', `/v1/beads/${id}/dependencies?max_depth=${maxDepth}`);
    }
    return this._rpc('DepTree', { id, max_depth: maxDepth });
  }

  /**
   * Fetch an overview of all epics with progress summaries.
   * @returns {Promise<Object>} Epic overview
   */
  async epicOverview() {
    if (this.mode === 'rest') {
      return this._rest('GET', '/v1/beads?type=epic&limit=500');
    }
    return this._rpc('EpicOverview', {});
  }

  // ────────── Write operations ──────────

  /**
   * Update fields on an existing bead.
   * @param {string} id - The bead ID to update
   * @param {Object} fields - Key-value pairs of fields to update
   * @returns {Promise<Object>} The updated bead
   */
  async update(id, fields) {
    if (this.mode === 'rest') return this._rest('PATCH', `/v1/beads/${id}`, fields);
    return this._rpc('Update', { id, ...fields });
  }

  /**
   * Close a bead by ID.
   * @param {string} id - The bead ID to close
   * @returns {Promise<Object>} The closed bead
   */
  async close(id) {
    if (this.mode === 'rest') return this._rest('POST', `/v1/beads/${id}/close`);
    return this._rpc('Close', { id });
  }

  /**
   * Check if the Graph endpoint is available.
   * Probes once and caches the result.
   * @returns {Promise<boolean>} True if Graph is supported
   */
  async hasGraph() {
    if (this._hasGraphCached !== undefined) return this._hasGraphCached;
    try {
      if (this.mode === 'rest') {
        await this._rest('POST', '/v1/graph', { limit: 1 });
      } else {
        await this._rpc('Graph', { limit: 1 });
      }
      this._hasGraphCached = true;
      return true;
    } catch {
      this._hasGraphCached = false;
      return false;
    }
  }

  // ────────── SSE streaming ──────────

  /**
   * Create an SSE EventSource with automatic exponential-backoff reconnection.
   * @private
   */
  _connectWithReconnect(url, label, setupFn, callbacks = {}) {
    const mgr = {
      url,
      label,
      _es: null,
      _retries: 0,
      _delay: SSE_INITIAL_DELAY,
      _timer: null,
      _stopped: false,

      connect: () => {
        if (mgr._stopped) return;

        const isReconnect = mgr._retries > 0;
        if (isReconnect) {
          console.log(`[beads3d] ${label} SSE reconnecting (attempt ${mgr._retries}/${SSE_MAX_RETRIES})...`);
          callbacks.onStatus?.('reconnecting', { attempt: mgr._retries, maxRetries: SSE_MAX_RETRIES });
        } else {
          callbacks.onStatus?.('connecting', {});
        }

        const es = new EventSource(url);
        mgr._es = es;

        es.onopen = () => {
          console.log(`[beads3d] ${label} SSE connected`);
          mgr._retries = 0;
          mgr._delay = SSE_INITIAL_DELAY;
          callbacks.onStatus?.('connected', {});
        };

        es.onerror = () => {
          if (mgr._stopped) return;
          if (es.readyState === EventSource.CLOSED) {
            console.warn(`[beads3d] ${label} SSE closed, scheduling reconnect`);
            es.close();
            mgr._scheduleReconnect();
          }
        };

        setupFn(es);

        // Track for cleanup
        const idx = this._eventSources.indexOf(mgr._prevEs);
        if (idx >= 0) this._eventSources.splice(idx, 1);
        mgr._prevEs = es;
        this._eventSources.push(es);
      },

      _scheduleReconnect: () => {
        if (mgr._stopped) return;
        mgr._retries++;
        if (mgr._retries > SSE_MAX_RETRIES) {
          console.error(`[beads3d] ${label} SSE gave up after ${SSE_MAX_RETRIES} retries`);
          callbacks.onStatus?.('disconnected', {});
          return;
        }
        const jitter = Math.random() * 0.3 * mgr._delay;
        const wait = mgr._delay + jitter;
        console.log(`[beads3d] ${label} SSE reconnect in ${Math.round(wait)}ms`);
        mgr._timer = setTimeout(() => {
          mgr.connect();
        }, wait);
        mgr._delay = Math.min(mgr._delay * SSE_BACKOFF_FACTOR, SSE_MAX_DELAY);
      },

      stop: () => {
        mgr._stopped = true;
        clearTimeout(mgr._timer);
        if (mgr._es) mgr._es.close();
      },

      retry: () => {
        mgr._stopped = false;
        mgr._retries = 0;
        mgr._delay = SSE_INITIAL_DELAY;
        clearTimeout(mgr._timer);
        if (mgr._es) mgr._es.close();
        mgr.connect();
      },
    };

    this._reconnectManagers.push(mgr);
    mgr.connect();
    return mgr;
  }

  /**
   * Connect to the mutation SSE event stream.
   * @param {Function} onEvent - Callback invoked with each parsed event object
   * @param {Object} [callbacks={}] - SSE lifecycle callbacks
   * @returns {Object} Reconnection manager
   */
  connectEvents(onEvent, callbacks = {}) {
    const url = this.mode === 'rest'
      ? `${this.baseUrl}/v1/events/stream`
      : `${this.baseUrl}/events`;
    return this._connectWithReconnect(
      url,
      'mutation',
      (es) => {
        es.onmessage = (e) => {
          try {
            const data = JSON.parse(e.data);
            onEvent(data);
          } catch {
            /* skip malformed */
          }
        };
      },
      callbacks,
    );
  }

  /**
   * Connect to the NATS bus SSE event stream.
   * @param {string} streams - Comma-separated stream names or 'all'
   * @param {Function} onEvent - Callback invoked with each parsed event object
   * @param {Object} [callbacks={}] - SSE lifecycle callbacks
   * @returns {Object} Reconnection manager
   */
  connectBusEvents(streams, onEvent, callbacks = {}) {
    const busPath = this.mode === 'rest' ? '/v1/bus/events' : '/bus/events';
    const url = `${this.baseUrl}${busPath}?stream=${encodeURIComponent(streams)}`;
    const eventTypes = ['agents', 'hooks', 'oj', 'mutations', 'decisions', 'mail'];
    return this._connectWithReconnect(
      url,
      'bus',
      (es) => {
        for (const type of eventTypes) {
          es.addEventListener(type, (e) => {
            try {
              onEvent(JSON.parse(e.data));
            } catch {
              /* skip */
            }
          });
        }
      },
      callbacks,
    );
  }

  // ────────── Decision operations ──────────

  async decisionGet(issueId) {
    if (this.mode === 'rest') return this._rest('GET', `/v1/decisions/${issueId}`);
    return this._rpc('DecisionGet', { issue_id: issueId });
  }

  async decisionList(opts = {}) {
    if (this.mode === 'rest') {
      const params = new URLSearchParams();
      if (opts.status) params.set('status', opts.status);
      const qs = params.toString();
      return this._rest('GET', `/v1/decisions${qs ? '?' + qs : ''}`);
    }
    return this._rpc('DecisionList', opts);
  }

  async decisionListRecent(since, requestedBy) {
    if (this.mode === 'rest') {
      const params = new URLSearchParams({ since });
      if (requestedBy) params.set('requested_by', requestedBy);
      return this._rest('GET', `/v1/decisions?${params.toString()}`);
    }
    const args = { since };
    if (requestedBy) args.requested_by = requestedBy;
    return this._rpc('DecisionListRecent', args);
  }

  async decisionResolve(issueId, selectedOption, responseText, respondedBy = 'beads3d') {
    if (this.mode === 'rest') {
      return this._rest('POST', `/v1/decisions/${issueId}/resolve`, {
        selected_option: selectedOption,
        response_text: responseText,
        responded_by: respondedBy,
      });
    }
    return this._rpc('DecisionResolve', {
      issue_id: issueId,
      selected_option: selectedOption,
      response_text: responseText,
      responded_by: respondedBy,
    });
  }

  async decisionCancel(issueId, reason, canceledBy = 'beads3d') {
    if (this.mode === 'rest') {
      return this._rest('POST', `/v1/decisions/${issueId}/cancel`, {
        reason,
        canceled_by: canceledBy,
      });
    }
    return this._rpc('DecisionCancel', {
      issue_id: issueId,
      reason,
      canceled_by: canceledBy,
    });
  }

  async decisionRemind(issueId, force = false) {
    // No kbeads REST equivalent — RPC-only for now
    return this._rpc('DecisionRemind', { issue_id: issueId, force });
  }

  /**
   * Send mail to an agent by creating a message bead.
   */
  async sendMail(toAgent, subject, body = '') {
    if (this.mode === 'rest') {
      return this._rest('POST', '/v1/beads', {
        title: subject,
        description: body,
        type: 'message',
        assignee: toAgent,
        created_by: 'beads3d',
        priority: 2,
      });
    }
    return this._rpc('Create', {
      title: subject,
      description: body,
      issue_type: 'message',
      assignee: toAgent,
      sender: 'beads3d',
      priority: 2,
    });
  }

  // ────────── Config operations ──────────

  async configList() {
    if (this.mode === 'rest') return this._rest('GET', '/v1/configs');
    return this._rpc('ConfigList', {});
  }

  async configGet(key) {
    if (this.mode === 'rest') return this._rest('GET', `/v1/configs/${key}`);
    return this._rpc('GetConfig', { key });
  }

  async configSet(key, value) {
    if (this.mode === 'rest') return this._rest('PUT', `/v1/configs/${key}`, { value });
    return this._rpc('ConfigSet', { key, value });
  }

  async configUnset(key) {
    if (this.mode === 'rest') return this._rest('DELETE', `/v1/configs/${key}`);
    return this._rpc('ConfigUnset', { key });
  }

  // ────────── Lifecycle ──────────

  /**
   * Close all SSE connections and stop all reconnection managers.
   */
  destroy() {
    for (const mgr of this._reconnectManagers) {
      mgr.stop();
    }
    this._reconnectManagers.length = 0;
    for (const es of this._eventSources) {
      es.close();
    }
    this._eventSources.length = 0;
  }

  /**
   * Manually reconnect all SSE streams.
   */
  reconnectAll() {
    for (const mgr of this._reconnectManagers) {
      mgr.retry();
    }
  }
}
