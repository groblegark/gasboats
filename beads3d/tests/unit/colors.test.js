import { describe, it, expect, beforeEach } from 'vitest';
import {
  STATUS_COLORS,
  TYPE_COLORS,
  PRIORITY_SIZES,
  DEP_TYPE_COLORS,
  DECISION_COLORS,
  nodeColor,
  nodeSize,
  linkColor,
  rigColor,
  colorToHex,
} from '../../src/colors.js';

describe('nodeColor', () => {
  beforeEach(() => {
    // Clear any color overrides between tests
    if (typeof globalThis.window !== 'undefined') {
      delete globalThis.window.__beads3d_colorOverrides;
    }
  });

  it('returns red for blocked nodes', () => {
    expect(nodeColor({ _blocked: true, status: 'open' })).toBe('#d04040');
  });

  it('returns orange for agent nodes', () => {
    expect(nodeColor({ issue_type: 'agent', status: 'open' })).toBe('#ff6b35');
  });

  it('returns purple for epic nodes', () => {
    expect(nodeColor({ issue_type: 'epic', status: 'open' })).toBe('#8b45a6');
  });

  it('returns status color for regular nodes', () => {
    expect(nodeColor({ status: 'open' })).toBe(STATUS_COLORS.open);
    expect(nodeColor({ status: 'in_progress' })).toBe(STATUS_COLORS.in_progress);
    expect(nodeColor({ status: 'closed' })).toBe(STATUS_COLORS.closed);
  });

  it('returns fallback for unknown status', () => {
    expect(nodeColor({ status: 'unknown_status' })).toBe('#555');
  });

  it('returns decision color for gate/decision nodes', () => {
    expect(nodeColor({ issue_type: 'gate', status: 'open' })).toBe(DECISION_COLORS.pending);
    expect(nodeColor({ issue_type: 'decision', status: 'closed' })).toBe(DECISION_COLORS.resolved);
  });

  it('uses _decisionState when set', () => {
    expect(nodeColor({ issue_type: 'gate', _decisionState: 'expired', status: 'open' })).toBe(DECISION_COLORS.expired);
  });

  it('blocked takes priority over agent/epic', () => {
    expect(nodeColor({ _blocked: true, issue_type: 'agent', status: 'open' })).toBe('#d04040');
    expect(nodeColor({ _blocked: true, issue_type: 'epic', status: 'open' })).toBe('#d04040');
  });
});

describe('nodeSize', () => {
  it('returns priority-based size scaled 1.5x for regular nodes', () => {
    expect(nodeSize({ priority: 0 })).toBe(16 * 1.5);
    expect(nodeSize({ priority: 2 })).toBe(8 * 1.5);
    expect(nodeSize({ priority: 4 })).toBe(5 * 1.5);
  });

  it('returns 2.2x for epics', () => {
    expect(nodeSize({ priority: 0, issue_type: 'epic' })).toBe(16 * 2.2);
    expect(nodeSize({ priority: 3, issue_type: 'epic' })).toBe(6 * 2.2);
  });

  it('returns 1.2x (min 6) for agents', () => {
    expect(nodeSize({ priority: 0, issue_type: 'agent' })).toBe(16 * 1.2);
    // priority 4 base=5, Math.max(5,6)=6, 6*1.2=7.2
    expect(nodeSize({ priority: 4, issue_type: 'agent' })).toBe(6 * 1.2);
  });

  it('uses fallback size 4 for undefined priority', () => {
    expect(nodeSize({})).toBe(4 * 1.5);
  });
});

describe('linkColor', () => {
  it('returns correct color for known dep types', () => {
    expect(linkColor({ dep_type: 'blocks' })).toBe('#d04040');
    expect(linkColor({ dep_type: 'waits-for' })).toBe('#d4a017');
    expect(linkColor({ dep_type: 'relates-to' })).toBe('#4a9eff88');
  });

  it('returns default for unknown dep type', () => {
    expect(linkColor({ dep_type: 'something-else' })).toBe(DEP_TYPE_COLORS.default);
    expect(linkColor({})).toBe(DEP_TYPE_COLORS.default);
  });
});

describe('rigColor', () => {
  it('returns gray for falsy rig name', () => {
    expect(rigColor(null)).toBe('#666');
    expect(rigColor('')).toBe('#666');
    expect(rigColor(undefined)).toBe('#666');
  });

  it('returns a color from the palette for valid rig names', () => {
    const palette = [
      '#e06090', '#40c0a0', '#c070e0', '#e0a030', '#50b0e0',
      '#a0d050', '#e07050', '#70a0e0', '#d0b060', '#60d0c0',
    ];
    const color = rigColor('gastown');
    expect(palette).toContain(color);
  });

  it('returns deterministic color for same rig name', () => {
    const c1 = rigColor('test-rig');
    const c2 = rigColor('test-rig');
    expect(c1).toBe(c2);
  });

  it('returns different colors for different rig names (usually)', () => {
    // Not guaranteed but very likely with different strings
    const c1 = rigColor('alpha');
    const c2 = rigColor('beta');
    // At minimum, the function shouldn't crash
    expect(typeof c1).toBe('string');
    expect(typeof c2).toBe('string');
  });
});

describe('colorToHex', () => {
  it('returns number directly if input is already a number', () => {
    expect(colorToHex(0xff0000)).toBe(0xff0000);
  });

  it('converts CSS hex to THREE.js hex number', () => {
    expect(colorToHex('#ff0000')).toBe(0xff0000);
    expect(colorToHex('#2d8a4e')).toBe(0x2d8a4e);
    expect(colorToHex('#000000')).toBe(0x000000);
  });

  it('handles hex with alpha (ignores extra chars)', () => {
    expect(colorToHex('#4a9eff88')).toBe(0x4a9eff);
  });

  it('returns fallback for non-hex strings', () => {
    expect(colorToHex('rgb(255,0,0)')).toBe(0x555555);
  });
});

describe('constant maps', () => {
  it('STATUS_COLORS has expected keys', () => {
    expect(STATUS_COLORS).toHaveProperty('open');
    expect(STATUS_COLORS).toHaveProperty('in_progress');
    expect(STATUS_COLORS).toHaveProperty('closed');
    expect(STATUS_COLORS).toHaveProperty('blocked');
  });

  it('TYPE_COLORS has expected keys', () => {
    expect(TYPE_COLORS).toHaveProperty('epic');
    expect(TYPE_COLORS).toHaveProperty('bug');
    expect(TYPE_COLORS).toHaveProperty('agent');
    expect(TYPE_COLORS).toHaveProperty('decision');
  });

  it('PRIORITY_SIZES maps 0-4', () => {
    for (let i = 0; i <= 4; i++) {
      expect(PRIORITY_SIZES[i]).toBeGreaterThan(0);
    }
    // P0 is largest
    expect(PRIORITY_SIZES[0]).toBeGreaterThan(PRIORITY_SIZES[4]);
  });
});
