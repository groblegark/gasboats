// Event formatting utilities for doots, agent feeds, and activity streams (bd-7t6nt)
// Pure functions — no global state dependencies.

/**
 * A bus or mutation event from the NATS event stream.
 * @typedef {Object} BusEvent
 * @property {string} type - The event type identifier (e.g. 'AgentStarted', 'PreToolUse', 'MutationUpdate')
 * @property {Object} payload - Event-specific data payload
 */

// Generate a concise label for a bus/mutation event (bd-c7723, bd-wn5he)
const _lastHeartbeat = {};
/**
 * Generate a concise human-readable label for a bus/mutation event.
 * Returns null for events that should be suppressed (e.g. noisy heartbeats).
 * @param {BusEvent} evt - The bus event to label
 * @returns {string|null} A short display label, or null if the event should be suppressed
 */
export function dootLabel(evt) {
  const type = evt.type || '';
  const p = evt.payload || {};

  // Agent lifecycle
  if (type === 'AgentStarted') return 'started';
  if (type === 'AgentStopped') return 'stopped';
  if (type === 'AgentCrashed') return 'crashed!';
  if (type === 'AgentIdle') return 'idle';
  if (type === 'AgentHeartbeat') return null; // too noisy

  // Hook events (tool use, session, etc.) — show full command context (bd-wn5he)
  if (type === 'PreToolUse' || type === 'PostToolUse') {
    const tool = p.tool_name || p.toolName || '';
    const input = p.tool_input || {};
    if (tool === 'Bash' || tool === 'bash') {
      const cmd = input.command || input.cmd || '';
      const short = cmd
        .replace(/^cd [^ ]+ && /, '')
        .split('\n')[0]
        .slice(0, 60);
      return short || 'bash';
    }
    if (tool === 'Read' || tool === 'read') {
      const fp = input.file_path || input.path || '';
      return fp ? `read ${fp.split('/').pop()}` : 'read';
    }
    if (tool === 'Edit' || tool === 'edit') {
      const fp = input.file_path || input.path || '';
      return fp ? `edit ${fp.split('/').pop()}` : 'edit';
    }
    if (tool === 'Write' || tool === 'write') {
      const fp = input.file_path || input.path || '';
      return fp ? `write ${fp.split('/').pop()}` : 'write';
    }
    if (tool === 'Grep' || tool === 'grep') {
      const pat = input.pattern || '';
      return pat ? `grep ${pat.slice(0, 30)}` : 'grep';
    }
    if (tool === 'Glob' || tool === 'glob') {
      const pat = input.pattern || '';
      return pat ? `glob ${pat.slice(0, 30)}` : 'glob';
    }
    if (tool === 'Task' || tool === 'task') {
      const desc = input.description || '';
      return desc ? `task: ${desc.slice(0, 40)}` : 'task';
    }
    return tool ? tool.toLowerCase() : 'tool';
  }
  if (type === 'SessionStart') return 'session start';
  if (type === 'SessionEnd') return 'session end';
  if (type === 'Stop') return 'stop';
  if (type === 'UserPromptSubmit') return 'prompt';
  if (type === 'PreCompact') return 'compacting...';

  // OddJobs
  if (type === 'OjJobCreated') return 'job created';
  if (type === 'OjStepAdvanced') return 'step';
  if (type === 'OjAgentSpawned') return 'spawned';
  if (type === 'OjAgentIdle') return 'idle';
  if (type === 'OjJobCompleted') return 'job done';
  if (type === 'OjJobFailed') return 'job failed!';

  // Mail events (bd-t76aw)
  if (type === 'MailSent') return `✉ ${(p.subject || 'mail').slice(0, 40)}`;
  if (type === 'MailRead') return '✉ read';

  // Mutations (bd-wn5he: rate-limit noisy heartbeat updates, show meaningful ones)
  if (type === 'MutationCreate') return `new: ${(p.title || p.issue_id || 'bead').slice(0, 40)}`;
  if (type === 'MutationUpdate') {
    if (p.type === 'rpc_audit') return null; // daemon-token bookkeeping noise
    // Rate-limit agent heartbeats: one doot per agent per 10s
    if (p.agent_state && !p.new_status) {
      const key = p.issue_id || p.actor || '';
      const now = Date.now();
      if (now - (_lastHeartbeat[key] || 0) < 10000) return null;
      _lastHeartbeat[key] = now;
      return p.agent_state; // "working", "idle", etc.
    }
    // Show assignee claims
    if (p.assignee && p.type === 'update') return `claimed by ${p.assignee}`;
    return p.title ? p.title.slice(0, 50) : 'updated';
  }
  if (type === 'MutationStatus') return p.new_status || 'status';
  if (type === 'MutationClose') return 'closed';

  // Decisions (bd-0j7hr: show decision events as doots + graph updates)
  if (type === 'DecisionCreated') return `? ${(p.question || 'decision').slice(0, 35)}`;
  if (type === 'DecisionResponded') return `✓ ${(p.chosen_label || 'resolved').slice(0, 35)}`;
  if (type === 'DecisionEscalated') return 'escalated';
  if (type === 'DecisionExpired') return 'expired';

  // Jack advisories (beads-xjol: risky command warnings)
  if (type === 'jack-reminder') {
    const cmd = (p.command || '').slice(0, 40);
    return cmd ? `jack? ${cmd}` : 'jack advisory';
  }

  return type
    .replace(/([A-Z])/g, ' $1')
    .trim()
    .toLowerCase()
    .slice(0, 20);
}

/**
 * Determine the display color for an event based on its type.
 * Maps event categories to colors: red for crashes/failures, amber for decisions,
 * gray for stops/ends, green for starts/creates, blue for tools, orange default.
 * @param {BusEvent} evt - The bus event to colorize
 * @returns {string} CSS hex color string
 */
export function dootColor(evt) {
  const type = evt.type || '';
  if (type.includes('Crash') || type.includes('Failed')) return '#ff3333';
  if (type.includes('Decision')) return '#d4a017'; // before Created check — DecisionCreated is yellow
  if (type === 'jack-reminder') return '#ff6b35'; // orange wrench for jack advisories
  if (type.includes('Stop') || type.includes('End')) return '#888888';
  if (type.includes('Started') || type.includes('Spawned') || type.includes('Created')) return '#2d8a4e';
  if (type.includes('Tool')) return '#4a9eff';
  if (type.includes('Idle')) return '#666666';
  return '#ff6b35'; // agent orange default
}

/**
 * Format a tool use event payload into a concise label for agent feed entries.
 * Extracts the most relevant info (command, file path, pattern) from the tool input.
 * @param {Object} payload - The event payload containing tool_name and tool_input
 * @param {string} [payload.tool_name] - Name of the tool (e.g. 'Bash', 'Read', 'Edit')
 * @param {Object} [payload.tool_input] - Tool-specific input parameters
 * @returns {string} A short human-readable label for the tool invocation
 */
export function formatToolLabel(payload) {
  const toolName = payload.tool_name || 'tool';
  const input = payload.tool_input || {};
  if (toolName === 'Bash' && input.command) {
    const cmd = input.command.length > 40 ? input.command.slice(0, 40) + '…' : input.command;
    return cmd;
  }
  if (toolName === 'Read' && input.file_path) {
    return `read ${input.file_path.split('/').pop()}`;
  }
  if (toolName === 'Edit' && input.file_path) {
    return `edit ${input.file_path.split('/').pop()}`;
  }
  if (toolName === 'Write' && input.file_path) {
    return `write ${input.file_path.split('/').pop()}`;
  }
  if (toolName === 'Grep' && input.pattern) {
    return `grep ${input.pattern.length > 30 ? input.pattern.slice(0, 30) + '…' : input.pattern}`;
  }
  if (toolName === 'Task' && input.description) {
    return `task: ${input.description.length > 30 ? input.description.slice(0, 30) + '…' : input.description}`;
  }
  return toolName.toLowerCase();
}

/**
 * Map a bus event to the agent node ID it belongs to (bd-kau4k, bd-t76aw, bd-jgvas).
 * Extracts the actor from the event payload and formats it as an agent node ID.
 * Does NOT require a window to already exist -- used for auto-open (bd-jgvas Phase 2).
 * @param {BusEvent} evt - The bus event to resolve
 * @returns {string|null} Agent node ID (e.g. 'agent:cool-trout') or null if not attributable
 */
export function resolveAgentIdLoose(evt) {
  const p = evt.payload || {};

  if (evt.type === 'MailSent' || evt.type === 'MailRead') {
    const to = (p.to || '').replace(/^@/, '');
    return to ? `agent:${to}` : null;
  }

  if (evt.type && evt.type.startsWith('Decision') && p.requested_by) {
    return `agent:${p.requested_by}`;
  }

  const actor = p.actor;
  return actor && actor !== 'daemon' ? `agent:${actor}` : null;
}

/** @type {Record<string, string>} Map of tool name to single-character icon for agent feed entries */
export const TOOL_ICONS = {
  Read: 'R',
  Edit: 'E',
  Bash: '$',
  Grep: '?',
  Write: 'W',
  Task: 'T',
  Glob: 'G',
  WebFetch: 'F',
  WebSearch: 'S',
  NotebookEdit: 'N',
};
