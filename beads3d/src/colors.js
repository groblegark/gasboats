// Color scheme — biological cell aesthetic
// Dark background, bioluminescent nodes, organic feel

/** @type {Record<string, string>} Map of issue status to CSS hex color */
export const STATUS_COLORS = {
  open: '#2d8a4e', // green — alive, ready
  in_progress: '#d4a017', // amber — active metabolism
  blocked: '#d04040', // red — blocked by dependency (bd-7haep)
  hooked: '#c06020', // burnt orange — waiting on hook (bd-7haep)
  deferred: '#3a5a7a', // muted blue — deferred/dormant (bd-7haep)
  review: '#4a9eff', // blue — signaling
  on_ice: '#3a5a7a', // muted blue — dormant
  closed: '#333340', // dark — inert
  tombstone: '#1a1a22', // near-black — dead
};

/** @type {Record<string, string>} Map of issue type to CSS hex color */
export const TYPE_COLORS = {
  epic: '#8b45a6', // purple — organelle
  feature: '#2d8a4e', // green
  bug: '#d04040', // red — pathogen
  task: '#4a9eff', // blue
  agent: '#ff6b35', // orange — ribosome
  decision: '#d4a017', // amber — signal molecule
  gate: '#d4a017',
  jack: '#e06830', // orange-red — hydraulic jack, temporary infra mod
  chore: '#666',
  doc: '#5a8a5a',
  test: '#4a7a9e',
};

/** @type {Record<number, number>} Map of priority level (0-4) to base node size in pixels */
export const PRIORITY_SIZES = {
  0: 16, // P0 critical — largest (bd-d8dfd: increased)
  1: 11, // P1 high
  2: 8, // P2 medium
  3: 6, // P3 low
  4: 5, // P4 backlog — smallest
};

/** @type {Record<string, string>} Map of dependency type to CSS hex color (with optional alpha) */
export const DEP_TYPE_COLORS = {
  blocks: '#d04040',
  'waits-for': '#d4a017',
  'relates-to': '#4a9eff88',
  'parent-child': '#8b45a688',
  'child-of': '#6a6a8a88', // muted gray — hierarchy links (kd-XGgiokgQBH)
  'action-item': '#e0842088', // orange — derived action items (kd-XGgiokgQBH)
  escalate: '#d4a017', // yellow — escalation arrows (kd-XGgiokgQBH)
  duplicate: '#66666688', // gray — dedup cluster (kd-XGgiokgQBH)
  'jira-link': '#4a7a9e88', // muted blue — cross-project Jira links (kd-XGgiokgQBH)
  assigned_to: '#ff6b3566', // reduced opacity — high density in large graphs (bd-ld2fa)
  rig_conflict: '#ff3030', // bright red — agents on same rig+branch (bd-90ikf)
  default: '#3a3a5a',
};

/** @type {Record<string, string>} Map of decision state to CSS hex color (bd-zr374) */
export const DECISION_COLORS = {
  pending: '#d4a017', // amber — awaiting response
  resolved: '#2d8a4e', // green — answered
  expired: '#d04040', // red — timed out
  canceled: '#666', // gray — canceled
};

/**
 * Determine the display color for an issue node in the 3D graph.
 * Checks blocked state, issue type, decision state, and status in priority order.
 * Respects runtime color overrides from the control panel (window.__beads3d_colorOverrides).
 * @param {{status: string, issue_type: string, _blocked: boolean, _decisionState?: string}} issue - The issue object
 * @returns {string} CSS hex color string
 */
export function nodeColor(issue) {
  // bd-sz1ha: check control panel color overrides
  const ov = typeof window !== 'undefined' && window.__beads3d_colorOverrides;

  // Blocked nodes glow red
  if (issue._blocked) return (ov && ov.blocked) || '#d04040';
  // Agents get special orange
  if (issue.issue_type === 'agent') return (ov && ov.agent) || '#ff6b35';
  // Jacks get orange-red; expired jacks flash red (handled in animation loop)
  if (issue.issue_type === 'jack') return (ov && ov.jack) || '#e06830';
  // Epics always purple
  if (issue.issue_type === 'epic') return (ov && ov.epic) || '#8b45a6';
  // Decision/gate nodes colored by decision state (bd-zr374)
  if (issue.issue_type === 'gate' || issue.issue_type === 'decision') {
    const ds = issue._decisionState || (issue.status === 'closed' ? 'resolved' : 'pending');
    return DECISION_COLORS[ds] || DECISION_COLORS.pending;
  }
  // Otherwise by status — check override for the status key
  if (ov && ov[issue.status]) return ov[issue.status];
  return STATUS_COLORS[issue.status] || '#555';
}

/**
 * Determine the display size for an issue node in the 3D graph.
 * Size is based on priority, with multipliers for epics (largest), agents, and regular beads.
 * @param {{priority: number, issue_type: string}} issue - The issue object
 * @returns {number} Node size in pixels
 */
export function nodeSize(issue) {
  const base = PRIORITY_SIZES[issue.priority] ?? 4;
  // Epics are the largest — prominent organizers (bd-7iju8)
  if (issue.issue_type === 'epic') return base * 2.2;
  // Agents are smaller — supporting elements, not the focus (bd-7iju8)
  if (issue.issue_type === 'agent') return Math.max(base, 6) * 1.2;
  // Jacks: slightly larger than beads — visible infrastructure markers (bd-hffzf)
  if (issue.issue_type === 'jack') return base * 1.6;
  // Beads (work items) are the visual focus — boosted 1.5x (bd-7iju8)
  return base * 1.5;
}

/**
 * Determine the display color for a dependency link in the 3D graph.
 * @param {{dep_type: string}} dep - The dependency object
 * @returns {string} CSS hex color string
 */
export function linkColor(dep) {
  return DEP_TYPE_COLORS[dep.dep_type] || DEP_TYPE_COLORS.default;
}

// Rig colors — deterministic hash-based palette (bd-90ikf)
// Each rig gets a unique, distinct color for badge and conflict edges.
/** @type {string[]} Palette of distinct CSS hex colors for rig assignment badges */
const RIG_PALETTE = [
  '#e06090', // pink
  '#40c0a0', // teal
  '#c070e0', // violet
  '#e0a030', // gold
  '#50b0e0', // sky blue
  '#a0d050', // lime
  '#e07050', // coral
  '#70a0e0', // periwinkle
  '#d0b060', // sand
  '#60d0c0', // mint
];

/** @type {Record<string, string>} Cache of rig name to assigned color */
const rigColorCache = {};
/**
 * Get a deterministic color for a rig name using hash-based palette selection.
 * Results are cached for consistent coloring across renders.
 * @param {string} rigName - The rig name to colorize
 * @returns {string} CSS hex color string
 */
export function rigColor(rigName) {
  if (!rigName) return '#666';
  if (rigColorCache[rigName]) return rigColorCache[rigName];
  let hash = 0;
  for (let i = 0; i < rigName.length; i++) hash = ((hash << 5) - hash + rigName.charCodeAt(i)) | 0;
  rigColorCache[rigName] = RIG_PALETTE[Math.abs(hash) % RIG_PALETTE.length];
  return rigColorCache[rigName];
}

/**
 * Convert a CSS color string to a THREE.js hex number.
 * Passes through values that are already numbers. Only handles '#rrggbb' format.
 * @param {string|number} cssColor - CSS hex color string (e.g. '#ff6b35') or numeric hex value
 * @returns {number} Numeric hex color value for THREE.js (e.g. 0xff6b35)
 */
export function colorToHex(cssColor) {
  if (typeof cssColor === 'number') return cssColor;
  if (cssColor.startsWith('#')) {
    return parseInt(cssColor.slice(1, 7), 16);
  }
  return 0x555555;
}
