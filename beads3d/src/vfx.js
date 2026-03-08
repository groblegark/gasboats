// --- VFX / Event Sprite System (bd-7t6nt: extracted from main.js) ---
import * as THREE from 'three';
import { nodeSize } from './colors.js';

/**
 * @typedef {Object} GraphNode
 * @property {string} id - Node identifier
 * @property {number} [x] - X position in 3D space
 * @property {number} [y] - Y position in 3D space
 * @property {number} [z] - Z position in 3D space
 * @property {string} [status] - Node status (e.g. 'open', 'in_progress', 'closed')
 * @property {string} [issue_type] - Issue type (e.g. 'feature', 'bug', 'task', 'epic')
 * @property {number} [priority] - Priority level (0 = highest)
 * @property {boolean} [_hidden] - Whether the node is hidden from view
 * @property {Object} [__threeObj] - Internal Three.js object reference
 */

/**
 * @typedef {Object} VFXConfig
 * @property {number} orbitSpeed - Orbit ring angular speed
 * @property {number} orbitRate - Orbit ring emission interval (seconds)
 * @property {number} orbitSize - Orbit ring particle size
 * @property {number} hoverRate - Hover glow emission interval (seconds)
 * @property {number} hoverSize - Hover glow particle size
 * @property {number} streamRate - Dependency energy stream emission interval (seconds)
 * @property {number} streamSpeed - Energy stream particle velocity
 * @property {number} particleLifetime - Base particle lifetime (seconds)
 * @property {number} selectionGlow - Selection glow intensity multiplier
 * @property {number} intensity - Global VFX intensity multiplier
 * @property {boolean} [claimComet] - Whether claim comet trails are enabled
 */

// --- Dependency injection ---
let _getGraph = null; // () => ForceGraph3D instance
let _getGraphData = null; // () => { nodes, links }
let _getParticlePool = null; // () => GPU particle pool

/**
 * Inject dependencies required by the VFX system.
 * @param {Object} deps - Dependency injection container
 * @param {Function} deps.getGraph - Returns the ForceGraph3D instance
 * @param {Function} deps.getGraphData - Returns { nodes, links } graph data
 * @param {Function} deps.getParticlePool - Returns the GPU particle pool
 * @returns {void}
 */
export function setVfxDeps({ getGraph, getGraphData, getParticlePool }) {
  _getGraph = getGraph;
  _getGraphData = getGraphData;
  _getParticlePool = getParticlePool;
}

// --- VFX Control Panel settings (bd-hr5om) ---
/** @type {VFXConfig} */
export const _vfxConfig = {
  orbitSpeed: 2.5, // orbit ring angular speed
  orbitRate: 0.08, // orbit ring emission interval (seconds)
  orbitSize: 1.5, // orbit ring particle size
  hoverRate: 0.15, // hover glow emission interval (seconds)
  hoverSize: 1.2, // hover glow particle size
  streamRate: 0.12, // dependency energy stream emission interval (seconds)
  streamSpeed: 3.0, // energy stream particle velocity
  particleLifetime: 0.8, // base particle lifetime (seconds)
  selectionGlow: 1.0, // selection glow intensity multiplier
  intensity: 1.0, // global VFX intensity multiplier (bd-epyyu)
};

// VFX intensity presets (bd-epyyu)
const VFX_PRESETS = {
  subtle: 0.25,
  normal: 1.0,
  dramatic: 2.0,
  maximum: 4.0,
};

/**
 * Set the global VFX intensity multiplier (clamped to 0-4).
 * @param {number} v - Intensity value
 * @returns {void}
 */
export function setVfxIntensity(v) {
  _vfxConfig.intensity = Math.max(0, Math.min(4, v));
}

/**
 * Apply a named VFX intensity preset.
 * @param {string} name - Preset name ('subtle', 'normal', 'dramatic', 'maximum')
 * @returns {void}
 */
export function presetVFX(name) {
  const v = VFX_PRESETS[name];
  if (v !== undefined) setVfxIntensity(v);
}

// --- Adaptive quality scaling (bd-dnuky) ---
// Monitors FPS and auto-adjusts particle budget + aura cap to maintain performance.
// Three quality tiers: full → reduced → minimal. Hysteresis prevents oscillation.
const ADAPTIVE_FPS_LOW = 20; // below this: disable particle spawning entirely
const ADAPTIVE_FPS_REDUCE = 30; // below this for 5+ consecutive: reduce budget
const ADAPTIVE_FPS_RECOVER = 50; // above this for 30+ consecutive: restore budget
const ADAPTIVE_CONSECUTIVE_LOW = 5; // frames below REDUCE threshold before downgrade
const ADAPTIVE_CONSECUTIVE_HIGH = 30; // frames above RECOVER threshold before upgrade

const _adaptiveState = {
  tier: 'full', // 'full' | 'reduced' | 'minimal'
  consecutiveLow: 0,
  consecutiveHigh: 0,
  particleBudgetScale: 1.0, // multiplier for particle counts
  auraMaxNodes: 20,
  manualOverride: false, // true when user sets intensity manually
};

/**
 * Feed a frame's FPS into the adaptive quality monitor.
 * Call once per frame from the animation loop.
 * @param {number} fps - Current frame's FPS
 * @returns {void}
 */
export function adaptiveQualityTick(fps) {
  if (_adaptiveState.manualOverride) return;

  if (fps < ADAPTIVE_FPS_REDUCE) {
    _adaptiveState.consecutiveLow++;
    _adaptiveState.consecutiveHigh = 0;
  } else if (fps > ADAPTIVE_FPS_RECOVER) {
    _adaptiveState.consecutiveHigh++;
    _adaptiveState.consecutiveLow = 0;
  } else {
    // In the dead zone — reset both counters
    _adaptiveState.consecutiveLow = 0;
    _adaptiveState.consecutiveHigh = 0;
  }

  if (fps < ADAPTIVE_FPS_LOW) {
    // Emergency: disable particles
    if (_adaptiveState.tier !== 'minimal') {
      _adaptiveState.tier = 'minimal';
      _adaptiveState.particleBudgetScale = 0;
      _adaptiveState.auraMaxNodes = 0;
    }
  } else if (_adaptiveState.consecutiveLow >= ADAPTIVE_CONSECUTIVE_LOW && _adaptiveState.tier === 'full') {
    _adaptiveState.tier = 'reduced';
    _adaptiveState.particleBudgetScale = 0.5;
    _adaptiveState.auraMaxNodes = 10;
  } else if (_adaptiveState.consecutiveHigh >= ADAPTIVE_CONSECUTIVE_HIGH) {
    if (_adaptiveState.tier === 'minimal') {
      _adaptiveState.tier = 'reduced';
      _adaptiveState.particleBudgetScale = 0.5;
      _adaptiveState.auraMaxNodes = 10;
      _adaptiveState.consecutiveHigh = 0; // reset to require sustained recovery
    } else if (_adaptiveState.tier === 'reduced') {
      _adaptiveState.tier = 'full';
      _adaptiveState.particleBudgetScale = 1.0;
      _adaptiveState.auraMaxNodes = 20;
    }
  }
}

/**
 * Get the current adaptive quality state for diagnostics.
 * @returns {{tier: string, particleBudgetScale: number, auraMaxNodes: number, manualOverride: boolean}}
 */
export function getAdaptiveState() {
  return { ..._adaptiveState };
}

/**
 * Apply a VFX preset that also scales particle budgets (bd-dnuky).
 * Presets override adaptive quality until FPS issues are detected.
 * @param {string} name - Preset name ('subtle', 'normal', 'dramatic', 'maximum')
 * @returns {void}
 */
export function applyVfxPreset(name) {
  const v = VFX_PRESETS[name];
  if (v === undefined) return;
  setVfxIntensity(v);
  // Scale particle budget with intensity
  _adaptiveState.manualOverride = name !== 'normal';
  if (name === 'subtle') {
    _adaptiveState.particleBudgetScale = 0.25;
    _adaptiveState.auraMaxNodes = 5;
    _adaptiveState.tier = 'reduced';
  } else if (name === 'normal') {
    _adaptiveState.particleBudgetScale = 1.0;
    _adaptiveState.auraMaxNodes = 20;
    _adaptiveState.tier = 'full';
    _adaptiveState.manualOverride = false; // allow adaptive scaling
  } else if (name === 'dramatic') {
    _adaptiveState.particleBudgetScale = 1.5;
    _adaptiveState.auraMaxNodes = 20;
    _adaptiveState.tier = 'full';
  } else if (name === 'maximum') {
    _adaptiveState.particleBudgetScale = 2.0;
    _adaptiveState.auraMaxNodes = 20;
    _adaptiveState.tier = 'full';
  }
}

// --- Ambient particle aura for in-progress beads (bd-ttet4) ---
/** @type {Map<string, {lastEmit: number, lastSpark: number, nextSpark: number, intensifyUntil: number}>} */
export const _auraEmitters = new Map(); // nodeId → { lastEmit, lastSpark, intensifyUntil }
let AURA_MAX_NODES = 20;
const AURA_ORBIT_PERIOD = 2.0; // seconds for full orbit
const AURA_SPARK_INTERVAL_MIN = 3.0;
const AURA_SPARK_INTERVAL_MAX = 5.0;

/**
 * Update ambient particle aura for in-progress bead nodes.
 * @param {number} t - Current animation time in seconds
 * @returns {void}
 */
export function updateInProgressAura(t) {
  const _particlePool = _getParticlePool && _getParticlePool();
  const graphData = _getGraphData && _getGraphData();
  const graph = _getGraph && _getGraph();
  if (!_particlePool || !graphData || !graph) return;
  const dt = 0.016;

  // Collect in-progress nodes (budget: adaptive cap, bd-dnuky)
  const auraMax = _adaptiveState.auraMaxNodes;
  if (auraMax === 0) return; // minimal tier: skip all aura work
  const inProgressNodes = [];
  for (const n of graphData.nodes) {
    if (n.status === 'in_progress' && !n._hidden && n.x !== undefined) {
      inProgressNodes.push(n);
      if (inProgressNodes.length >= auraMax) break;
    }
  }

  // Clean up emitters for nodes no longer in-progress
  for (const [id] of _auraEmitters) {
    if (!inProgressNodes.find((n) => n.id === id)) _auraEmitters.delete(id);
  }

  for (const node of inProgressNodes) {
    let state = _auraEmitters.get(node.id);
    if (!state) {
      state = {
        lastEmit: t,
        lastSpark: t,
        nextSpark: AURA_SPARK_INTERVAL_MIN + Math.random() * (AURA_SPARK_INTERVAL_MAX - AURA_SPARK_INTERVAL_MIN),
        intensifyUntil: 0,
      };
      _auraEmitters.set(node.id, state);
    }

    const pos = { x: node.x || 0, y: node.y || 0, z: node.z || 0 };
    const size = nodeSize({ priority: node.priority, issue_type: node.issue_type });
    const isIntensified = t < state.intensifyUntil;
    const budgetScale = _adaptiveState.particleBudgetScale;
    const baseInterval = isIntensified ? 0.05 : 0.1;
    const emitInterval = budgetScale > 0 ? baseInterval / budgetScale : 999; // scale emission rate (bd-dnuky)
    const color = 0xd4a017; // in_progress amber

    // Aura pattern by type
    const isBug = node.issue_type === 'bug';
    const isEpic = node.issue_type === 'epic';

    // Continuous orbit emission
    if (t - state.lastEmit > emitInterval) {
      state.lastEmit = t;
      const count = isIntensified ? 3 : 1;
      for (let i = 0; i < count; i++) {
        const angle = (t / AURA_ORBIT_PERIOD + i * 0.3) * Math.PI * 2;
        const radius = size * 1.5;
        // Orbit position
        const ox = pos.x + Math.cos(angle) * radius;
        const oy = pos.y + Math.sin(angle * 0.5) * radius * 0.3; // slight vertical bob
        const oz = pos.z + Math.sin(angle) * radius;
        // Tangential velocity (orbit direction)
        let vx = -Math.sin(angle) * 1.5;
        let vy = 0.3;
        let vz = Math.cos(angle) * 1.5;
        // Bug: erratic jitter
        if (isBug) {
          vx += (Math.random() - 0.5) * 4;
          vy += (Math.random() - 0.5) * 4;
          vz += (Math.random() - 0.5) * 4;
        }
        const pSize = isEpic ? 2.0 : 1.2;
        // Brightness variation (sine wave)
        const brightness = 0.7 + 0.3 * Math.sin(t * 3 + i);
        _particlePool.emit({ x: ox, y: oy, z: oz }, color, 1, {
          velocity: [vx, vy, vz],
          spread: isBug ? 1.5 : 0.3,
          lifetime: isEpic ? 1.2 : 0.8,
          size: pSize * brightness,
        });
      }
    }

    // Spark ejection every 3-5 seconds
    if (t - state.lastSpark > state.nextSpark) {
      state.lastSpark = t;
      state.nextSpark = AURA_SPARK_INTERVAL_MIN + Math.random() * (AURA_SPARK_INTERVAL_MAX - AURA_SPARK_INTERVAL_MIN);
      const sparkAngle = Math.random() * Math.PI * 2;
      const sparkPhi = Math.acos(2 * Math.random() - 1);
      const speed = 8 + Math.random() * 4;
      _particlePool.emit(pos, color, 1, {
        velocity: [
          speed * Math.sin(sparkPhi) * Math.cos(sparkAngle),
          speed * Math.sin(sparkPhi) * Math.sin(sparkAngle),
          speed * Math.cos(sparkPhi),
        ],
        spread: 0.5,
        lifetime: 1.0,
        size: 2.5,
      });
    }
  }
}

// Intensify aura briefly when a bead receives an update (bd-ttet4)
/**
 * Briefly intensify the aura of a bead node on update.
 * @param {string} nodeId - ID of the node to intensify
 * @returns {void}
 */
export function intensifyAura(nodeId) {
  const state = _auraEmitters.get(nodeId);
  if (state) state.intensifyUntil = performance.now() / 1000 + 1.0;
}

// Pending firework burst targets — IDs of beads from create events awaiting refresh (bd-4gmot)
/** @type {Set<string>} */
export const _pendingFireworks = new Set();
// Active collapse animations — Map<nodeId, { startTime, node, origPos }> (bd-1n122)
/** @type {Map<string, {startTime: number, node: GraphNode, origPos: {x: number, y: number, z: number}, phase: string, ghostStart?: number}>} */
export const _activeCollapses = new Map();

// --- Event sprites: pop-up animations for status changes + new associations (bd-9qeto) ---
/** @type {Array<{mesh: THREE.Object3D, birth: number, lifetime: number, type: string, node?: GraphNode, [key: string]: *}>} */
export const eventSprites = []; // { mesh, birth, lifetime, type, ... }
/** @type {number} */
export const EVENT_SPRITE_MAX = 80; // bd-k9cqt: increased for energy beam effects

// Status pulse colors by transition
const STATUS_PULSE_COLORS = {
  in_progress: 0xd4a017, // amber — just started
  closed: 0x2d8a4e, // green — completed
  open: 0x4a9eff, // blue — reopened
  review: 0x4a9eff, // blue
  on_ice: 0x3a5a7a, // muted blue
};

// Spawn an expanding ring burst when a bead changes status (bd-9qeto)
/**
 * Spawn expanding ring burst when a bead changes status.
 * @param {GraphNode} node - The node that changed status
 * @param {string} oldStatus - Previous status value
 * @param {string} newStatus - New status value
 * @returns {void}
 */
export function spawnStatusPulse(node, oldStatus, newStatus) {
  const graph = _getGraph && _getGraph();
  if (!node || !graph) return;
  const color = STATUS_PULSE_COLORS[newStatus] || 0x4a9eff;
  const size = nodeSize({ priority: node.priority, issue_type: node.issue_type });

  // Ring 1: fast expanding ring
  const ringGeo = new THREE.RingGeometry(size * 0.8, size * 1.0, 24);
  const ringMat = new THREE.MeshBasicMaterial({
    color,
    transparent: true,
    opacity: 0.7,
    side: THREE.DoubleSide,
    depthWrite: false,
  });
  const ring = new THREE.Mesh(ringGeo, ringMat);
  ring.position.set(node.x || 0, node.y || 0, node.z || 0);
  ring.lookAt(graph.camera().position);
  graph.scene().add(ring);

  eventSprites.push({
    mesh: ring,
    node,
    birth: performance.now() / 1000,
    lifetime: 1.5,
    type: 'status-pulse',
    startScale: 1.0,
    endScale: 4.0,
  });

  // Ring 2: slower, wider, dimmer secondary pulse
  const ring2Geo = new THREE.RingGeometry(size * 0.6, size * 0.75, 24);
  const ring2Mat = new THREE.MeshBasicMaterial({
    color,
    transparent: true,
    opacity: 0.4,
    side: THREE.DoubleSide,
    depthWrite: false,
  });
  const ring2 = new THREE.Mesh(ring2Geo, ring2Mat);
  ring2.position.set(node.x || 0, node.y || 0, node.z || 0);
  ring2.lookAt(graph.camera().position);
  graph.scene().add(ring2);

  eventSprites.push({
    mesh: ring2,
    node,
    birth: performance.now() / 1000 + 0.15,
    lifetime: 2.0,
    type: 'status-pulse',
    startScale: 1.0,
    endScale: 5.0,
  });

  // Prune oldest
  while (eventSprites.length > EVENT_SPRITE_MAX) {
    const old = eventSprites.shift();
    graph.scene().remove(old.mesh);
    old.mesh.geometry.dispose();
    old.mesh.material.dispose();
  }
}

// Firework burst colors by issue type (bd-4gmot)
const FIREWORK_COLORS = {
  feature: 0x00e5ff, // cyan
  bug: 0xd04040, // red
  task: 0xd4a017, // amber
  epic: 0x8b45a6, // purple
  agent: 0xff6b35, // orange
  decision: 0xd4a017, // amber
  chore: 0x999999, // gray
};

// Spawn firework burst at a node position when a new bead is created (bd-4gmot)
/**
 * Spawn a radial firework burst at a node when a new bead is created.
 * @param {GraphNode} node - The newly created node
 * @returns {void}
 */
export function spawnFireworkBurst(node) {
  const _particlePool = _getParticlePool && _getParticlePool();
  const graph = _getGraph && _getGraph();
  if (!node || !_particlePool || !graph) return;
  if (_adaptiveState.particleBudgetScale === 0) return; // minimal tier: skip (bd-dnuky)
  const pos = { x: node.x || 0, y: node.y || 0, z: node.z || 0 };
  const color = FIREWORK_COLORS[node.issue_type] || 0x4a9eff;
  const size = nodeSize({ priority: node.priority, issue_type: node.issue_type });
  const scale = _adaptiveState.particleBudgetScale;

  // Wave 1: primary radial burst — scaled by budget (bd-dnuky)
  const count1 = Math.round((80 + Math.floor(Math.random() * 40)) * scale);
  for (let i = 0; i < count1; i++) {
    const theta = Math.random() * Math.PI * 2;
    const phi = Math.acos(2 * Math.random() - 1);
    const speed = 8 + Math.random() * 4; // 8-12 units/s
    _particlePool.emit(pos, color, 1, {
      velocity: [
        speed * Math.sin(phi) * Math.cos(theta),
        speed * Math.sin(phi) * Math.sin(theta),
        speed * Math.cos(phi),
      ],
      spread: size * 0.3,
      lifetime: 1.2 + Math.random() * 0.3,
      size: 2.0,
    });
  }

  // Wave 2: secondary slower ring — staggered 100ms, 40 particles
  setTimeout(() => {
    const pool = _getParticlePool && _getParticlePool();
    if (!pool) return;
    const wave2Count = Math.round(40 * scale);
    for (let i = 0; i < wave2Count; i++) {
      const angle = (i / 40) * Math.PI * 2;
      const speed = 3 + Math.random() * 2;
      pool.emit(pos, color, 1, {
        velocity: [speed * Math.cos(angle), (Math.random() - 0.5) * 2, speed * Math.sin(angle)],
        spread: size * 0.2,
        lifetime: 1.5 + Math.random() * 0.5,
        size: 2.5,
      });
    }
  }, 100);

  // Center flash: bright bloom spike (0.3s duration, additive blending)
  const flashGeo = new THREE.SphereGeometry(size * 0.6, 12, 12);
  const flashMat = new THREE.MeshBasicMaterial({
    color: 0xffffff,
    transparent: true,
    opacity: 0.9,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });
  const flash = new THREE.Mesh(flashGeo, flashMat);
  flash.position.set(pos.x, pos.y, pos.z);
  graph.scene().add(flash);
  eventSprites.push({
    mesh: flash,
    node,
    birth: performance.now() / 1000,
    lifetime: 0.3,
    type: 'creation-flash',
    startScale: 1.0,
    endScale: 3.0,
  });

  // Expanding ring (concentric pulse from center)
  const ringGeo = new THREE.RingGeometry(size * 0.5, size * 0.8, 32);
  const ringMat = new THREE.MeshBasicMaterial({
    color,
    transparent: true,
    opacity: 0.8,
    side: THREE.DoubleSide,
    depthWrite: false,
  });
  const ring = new THREE.Mesh(ringGeo, ringMat);
  ring.position.set(pos.x, pos.y, pos.z);
  ring.lookAt(graph.camera().position);
  graph.scene().add(ring);
  eventSprites.push({
    mesh: ring,
    node,
    birth: performance.now() / 1000,
    lifetime: 1.5,
    type: 'status-pulse',
    startScale: 1.0,
    endScale: 6.0,
  });

  // Prune oldest sprites
  while (eventSprites.length > EVENT_SPRITE_MAX) {
    const old = eventSprites.shift();
    graph.scene().remove(old.mesh);
    old.mesh.geometry.dispose();
    old.mesh.material.dispose();
  }
}

// Shockwave ring colors by status (bd-3fnon)
const SHOCKWAVE_COLORS = {
  open: 0x2d8a4e, // green
  in_progress: 0xd4a017, // amber
  blocked: 0xd04040, // red
  hooked: 0xc06020, // burnt orange
  review: 0x4a9eff, // blue
  on_ice: 0x3a5a7a, // muted blue
  closed: 0x2d8a4e, // green (completed)
  deferred: 0x3a5a7a, // muted blue
};

// Camera shake state (bd-3fnon)
/** @type {?{startTime: number, duration: number, intensity: number, origPos: THREE.Vector3}} */
export let _cameraShake = null; // { startTime, duration, intensity, origPos }
/**
 * Get the current camera shake state.
 * @returns {?{startTime: number, duration: number, intensity: number, origPos: THREE.Vector3}}
 */
export function getCameraShake() {
  return _cameraShake;
}
/**
 * Clear the camera shake state.
 * @returns {void}
 */
export function clearCameraShake() {
  _cameraShake = null;
}

// Spawn dramatic shockwave ring on status change (bd-3fnon)
/**
 * Spawn a dramatic expanding shockwave torus ring on status change.
 * @param {GraphNode} node - The node that changed status
 * @param {string} oldStatus - Previous status value
 * @param {string} newStatus - New status value
 * @returns {void}
 */
export function spawnShockwave(node, oldStatus, newStatus) {
  const graph = _getGraph && _getGraph();
  const _particlePool = _getParticlePool && _getParticlePool();
  const graphData = _getGraphData && _getGraphData();
  if (!node || !graph || !_particlePool) return;
  if (_adaptiveState.particleBudgetScale === 0) return; // minimal tier: skip (bd-dnuky)
  const color = SHOCKWAVE_COLORS[newStatus] || 0x4a9eff;
  const pos = { x: node.x || 0, y: node.y || 0, z: node.z || 0 };

  // Primary expanding torus ring — radius 0→40 over 1.0s
  const torusGeo = new THREE.TorusGeometry(1, 0.3, 8, 48);
  const torusMat = new THREE.MeshBasicMaterial({
    color,
    transparent: true,
    opacity: 1.0,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });
  const torus = new THREE.Mesh(torusGeo, torusMat);
  torus.position.set(pos.x, pos.y, pos.z);
  torus.lookAt(graph.camera().position);
  graph.scene().add(torus);
  eventSprites.push({
    mesh: torus,
    node,
    birth: performance.now() / 1000,
    lifetime: 1.0,
    type: 'shockwave',
    startScale: 0.1,
    endScale: 40.0,
  });

  // Secondary inner ring — staggered 0.15s, half expansion speed
  const torus2Geo = new THREE.TorusGeometry(1, 0.2, 8, 48);
  const torus2Mat = new THREE.MeshBasicMaterial({
    color,
    transparent: true,
    opacity: 0.7,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });
  const torus2 = new THREE.Mesh(torus2Geo, torus2Mat);
  torus2.position.set(pos.x, pos.y, pos.z);
  torus2.lookAt(graph.camera().position);
  graph.scene().add(torus2);
  eventSprites.push({
    mesh: torus2,
    node,
    birth: performance.now() / 1000 + 0.15,
    lifetime: 1.5,
    type: 'shockwave',
    startScale: 0.1,
    endScale: 20.0,
  });

  // 20-30 particles ejected along ring plane (camera-facing)
  const cam = graph.camera();
  const forward = new THREE.Vector3();
  cam.getWorldDirection(forward);
  // Compute ring plane axes perpendicular to camera forward
  const up = new THREE.Vector3(0, 1, 0);
  const right = new THREE.Vector3().crossVectors(forward, up).normalize();
  const planeUp = new THREE.Vector3().crossVectors(right, forward).normalize();

  const ringCount = Math.round((20 + Math.floor(Math.random() * 10)) * _adaptiveState.particleBudgetScale);
  for (let i = 0; i < ringCount; i++) {
    const angle = (i / ringCount) * Math.PI * 2 + Math.random() * 0.3;
    const speed = 5 + Math.random() * 3;
    const vx = right.x * Math.cos(angle) * speed + planeUp.x * Math.sin(angle) * speed;
    const vy = right.y * Math.cos(angle) * speed + planeUp.y * Math.sin(angle) * speed;
    const vz = right.z * Math.cos(angle) * speed + planeUp.z * Math.sin(angle) * speed;
    _particlePool.emit(pos, color, 1, {
      velocity: [vx, vy, vz],
      spread: 0.3,
      lifetime: 0.8 + Math.random() * 0.4,
      size: 1.5 + Math.random(),
    });
  }

  // Energy ripple: flash connected nodes when shockwave passes
  if (graphData && graphData.links) {
    const neighbors = [];
    for (const l of graphData.links) {
      const src = typeof l.source === 'object' ? l.source : null;
      const tgt = typeof l.target === 'object' ? l.target : null;
      if (!src || !tgt) continue;
      if (src.id === node.id && tgt) neighbors.push(tgt);
      else if (tgt.id === node.id && src) neighbors.push(src);
    }
    // Stagger flash by distance
    for (const nb of neighbors) {
      const dx = (nb.x || 0) - pos.x;
      const dy = (nb.y || 0) - pos.y;
      const dz = (nb.z || 0) - pos.z;
      const dist = Math.sqrt(dx * dx + dy * dy + dz * dz);
      const delay = Math.min(dist / 40, 0.8); // shockwave travel time (40 units/s)
      setTimeout(() => {
        const pool = _getParticlePool && _getParticlePool();
        const g = _getGraph && _getGraph();
        if (!pool || !g) return;
        const nbPos = { x: nb.x || 0, y: nb.y || 0, z: nb.z || 0 };
        pool.emit(nbPos, color, 8, {
          velocity: [0, 2, 0],
          spread: 2.0,
          lifetime: 0.4,
          size: 1.5,
        });
      }, delay * 1000);
    }
  }

  // Camera shake for high-priority beads (P0/P1)
  if (node.priority <= 1) {
    const cam2 = graph.camera();
    _cameraShake = {
      startTime: performance.now() / 1000,
      duration: 0.3,
      intensity: 0.5,
      origPos: cam2.position.clone(),
    };
  }

  // Prune oldest sprites
  while (eventSprites.length > EVENT_SPRITE_MAX) {
    const old = eventSprites.shift();
    graph.scene().remove(old.mesh);
    old.mesh.geometry.dispose();
    old.mesh.material.dispose();
  }
}

// Spawn implosion/collapse effect when a bead is closed (bd-1n122)
/**
 * Spawn an implosion/collapse effect when a bead is closed.
 * @param {GraphNode} node - The node being closed
 * @returns {void}
 */
export function spawnCollapseEffect(node) {
  const graph = _getGraph && _getGraph();
  const _particlePool = _getParticlePool && _getParticlePool();
  const graphData = _getGraphData && _getGraphData();
  if (!node || !graph || !_particlePool) return;
  if (_adaptiveState.particleBudgetScale === 0) return; // minimal tier: skip (bd-dnuky)
  const pos = { x: node.x || 0, y: node.y || 0, z: node.z || 0 };
  const color = FIREWORK_COLORS[node.issue_type] || 0x4a9eff;
  const size = nodeSize({ priority: node.priority, issue_type: node.issue_type });
  const now = performance.now() / 1000;

  // Track this collapse for node scale animation
  _activeCollapses.set(node.id, {
    startTime: now,
    node,
    origPos: { ...pos },
    phase: 'collapsing',
  });

  // Phase 1: Spiral-inward particles sucked toward center (0-0.5s)
  for (let i = 0; i < 50; i++) {
    const theta = Math.random() * Math.PI * 2;
    const phi = Math.acos(2 * Math.random() - 1);
    const radius = 6 + Math.random() * 4;
    const spawnPos = {
      x: pos.x + radius * Math.sin(phi) * Math.cos(theta),
      y: pos.y + radius * Math.sin(phi) * Math.sin(theta),
      z: pos.z + radius * Math.cos(phi),
    };
    const speed = -(10 + Math.random() * 5);
    _particlePool.emit(spawnPos, color, 1, {
      velocity: [
        speed * Math.sin(phi) * Math.cos(theta),
        speed * Math.sin(phi) * Math.sin(theta),
        speed * Math.cos(phi),
      ],
      spread: 0.5,
      lifetime: 0.5 + Math.random() * 0.2,
      size: 1.5,
    });
  }

  // Phase 2: Contracting ring — visible collapse boundary
  const ringGeo = new THREE.RingGeometry(size * 4.0, size * 4.5, 32);
  const ringMat = new THREE.MeshBasicMaterial({
    color: 0x666666,
    transparent: true,
    opacity: 0.6,
    side: THREE.DoubleSide,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });
  const ring = new THREE.Mesh(ringGeo, ringMat);
  ring.position.set(pos.x, pos.y, pos.z);
  ring.lookAt(graph.camera().position);
  graph.scene().add(ring);
  eventSprites.push({
    mesh: ring,
    node,
    birth: now,
    lifetime: 0.8,
    type: 'collapse-ring',
    startScale: 1.0,
    endScale: 0.05,
  });

  // Phase 3 (delayed 0.8s): Death burst — flash + scatter particles
  setTimeout(() => {
    const pool = _getParticlePool && _getParticlePool();
    const g = _getGraph && _getGraph();
    if (!pool || !g) return;
    const burstPos = { x: node.x || pos.x, y: node.y || pos.y, z: node.z || pos.z };
    const flashGeo = new THREE.SphereGeometry(size * 0.8, 12, 12);
    const flashMat = new THREE.MeshBasicMaterial({
      color: 0xffffff,
      transparent: true,
      opacity: 1.0,
      depthWrite: false,
      blending: THREE.AdditiveBlending,
    });
    const flash = new THREE.Mesh(flashGeo, flashMat);
    flash.position.set(burstPos.x, burstPos.y, burstPos.z);
    g.scene().add(flash);
    eventSprites.push({
      mesh: flash,
      node,
      birth: performance.now() / 1000,
      lifetime: 0.4,
      type: 'creation-flash',
      startScale: 1.0,
      endScale: 2.5,
    });
    for (let i = 0; i < 30; i++) {
      const t = Math.random() * Math.PI * 2;
      const p = Math.acos(2 * Math.random() - 1);
      const s = 5 + Math.random() * 3;
      pool.emit(burstPos, 0x888888, 1, {
        velocity: [s * Math.sin(p) * Math.cos(t), s * Math.sin(p) * Math.sin(t), s * Math.cos(p)],
        spread: 0.3,
        lifetime: 0.8 + Math.random() * 0.4,
        size: 1.2,
      });
    }
    const collapse = _activeCollapses.get(node.id);
    if (collapse) {
      collapse.phase = 'ghost';
      collapse.ghostStart = performance.now() / 1000;
      const ghostGeo = new THREE.SphereGeometry(size, 12, 12);
      const ghostMat = new THREE.MeshBasicMaterial({
        color: 0x444466,
        transparent: true,
        opacity: 0.35,
        depthWrite: false,
      });
      const ghost = new THREE.Mesh(ghostGeo, ghostMat);
      ghost.position.set(burstPos.x, burstPos.y, burstPos.z);
      g.scene().add(ghost);
      eventSprites.push({
        mesh: ghost,
        node,
        birth: performance.now() / 1000,
        lifetime: 2.0,
        type: 'collapse-ghost',
        startScale: 1.0,
        endScale: 0.5,
      });
    }
  }, 800);

  // Edge dissolution: spawn sparks at connected link endpoints
  if (graphData && graphData.links) {
    for (const l of graphData.links) {
      const src = typeof l.source === 'object' ? l.source : null;
      const tgt = typeof l.target === 'object' ? l.target : null;
      if (!src || !tgt) continue;
      if (src.id === node.id) spawnEdgeSpark(tgt, src, 0x666666);
      else if (tgt.id === node.id) spawnEdgeSpark(src, tgt, 0x666666);
    }
  }

  while (eventSprites.length > EVENT_SPRITE_MAX) {
    const old = eventSprites.shift();
    graph.scene().remove(old.mesh);
    old.mesh.geometry.dispose();
    old.mesh.material.dispose();
  }
}

// Comet trail effect: agent -> claimed bead arc with particle trail (bd-t4umc)
/**
 * Spawn a comet trail arc from a source position to a target node.
 * @param {{x: number, y: number, z: number}} sourcePos - Starting position
 * @param {GraphNode} targetNode - Target node the comet flies toward
 * @param {number} color - Hex color for the comet trail
 * @returns {void}
 */
export function spawnCometTrail(sourcePos, targetNode, color) {
  const graph = _getGraph && _getGraph();
  const _particlePool = _getParticlePool && _getParticlePool();
  if (!graph || !_particlePool) return;
  const tgtPos = { x: targetNode.x || 0, y: targetNode.y || 0, z: targetNode.z || 0 };
  const mid = {
    x: (sourcePos.x + tgtPos.x) / 2,
    y: (sourcePos.y + tgtPos.y) / 2 + 20,
    z: (sourcePos.z + tgtPos.z) / 2,
  };
  const curve = new THREE.QuadraticBezierCurve3(
    new THREE.Vector3(sourcePos.x, sourcePos.y, sourcePos.z),
    new THREE.Vector3(mid.x, mid.y, mid.z),
    new THREE.Vector3(tgtPos.x, tgtPos.y, tgtPos.z),
  );
  const headGeo = new THREE.SphereGeometry(3, 10, 10);
  const headMat = new THREE.MeshBasicMaterial({
    color,
    transparent: true,
    opacity: 1.0,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });
  const head = new THREE.Mesh(headGeo, headMat);
  head.position.copy(curve.getPointAt(0));
  graph.scene().add(head);
  const startTime = performance.now() / 1000;
  eventSprites.push({
    mesh: head,
    node: targetNode,
    birth: startTime,
    lifetime: 0.8,
    type: 'comet-head',
    curve,
    startTime,
    duration: 0.8,
    trailCount: 55,
    _emitted: 0,
    color,
  });
  while (eventSprites.length > EVENT_SPRITE_MAX) {
    const old = eventSprites.shift();
    graph.scene().remove(old.mesh);
    old.mesh.geometry.dispose();
    old.mesh.material.dispose();
  }
}

/**
 * Trigger a claim comet trail from an agent node to a claimed bead.
 * @param {GraphNode} node - The bead being claimed
 * @param {string} newAssignee - Agent name of the new assignee
 * @returns {void}
 */
export function triggerClaimComet(node, newAssignee) {
  const graph = _getGraph && _getGraph();
  const graphData = _getGraphData && _getGraphData();
  if (!graph || !newAssignee || !_vfxConfig.claimComet) return;
  const color = 0xd4a017;
  const agentId = 'agent:' + newAssignee;
  const agentNode = graphData && graphData.nodes.find((n) => n.id === agentId);
  let sourcePos;
  if (agentNode && agentNode.x !== undefined) {
    sourcePos = { x: agentNode.x, y: agentNode.y, z: agentNode.z };
  } else {
    const controls = graph.controls();
    const target = controls && controls.target ? controls.target : new THREE.Vector3(0, 0, 0);
    sourcePos = {
      x: target.x + (Math.random() - 0.5) * 20,
      y: target.y + 50,
      z: target.z + (Math.random() - 0.5) * 20,
    };
  }
  spawnCometTrail(sourcePos, node, color);
}

// Spawn sparks that travel along a new edge between two nodes (bd-9qeto)
/**
 * Spawn sparks that travel along a new edge between two nodes.
 * @param {GraphNode} sourceNode - Source node of the edge
 * @param {GraphNode} targetNode - Target node of the edge
 * @param {number} [color] - Hex color for the sparks (defaults to 0x4a9eff)
 * @returns {void}
 */
export function spawnEdgeSpark(sourceNode, targetNode, color) {
  const graph = _getGraph && _getGraph();
  if (!sourceNode || !targetNode || !graph) return;
  const sparkColor = color || 0x4a9eff;

  // Create 3 small sphere sparks that travel from source to target
  for (let i = 0; i < 3; i++) {
    const sparkGeo = new THREE.SphereGeometry(0.8, 6, 6);
    const sparkMat = new THREE.MeshBasicMaterial({
      color: sparkColor,
      transparent: true,
      opacity: 0.9,
      depthWrite: false,
    });
    const spark = new THREE.Mesh(sparkGeo, sparkMat);
    spark.position.set(sourceNode.x || 0, sourceNode.y || 0, sourceNode.z || 0);
    graph.scene().add(spark);

    eventSprites.push({
      mesh: spark,
      birth: performance.now() / 1000 + i * 0.2,
      lifetime: 1.2,
      type: 'edge-spark',
      sourceNode,
      targetNode,
      jitter: { x: (Math.random() - 0.5) * 2, y: (Math.random() - 0.5) * 2, z: (Math.random() - 0.5) * 2 },
    });
  }

  // Burst particles at connection point (midpoint)
  const mx = ((sourceNode.x || 0) + (targetNode.x || 0)) / 2;
  const my = ((sourceNode.y || 0) + (targetNode.y || 0)) / 2;
  const mz = ((sourceNode.z || 0) + (targetNode.z || 0)) / 2;

  for (let i = 0; i < 5; i++) {
    const pGeo = new THREE.SphereGeometry(0.4, 4, 4);
    const pMat = new THREE.MeshBasicMaterial({
      color: sparkColor,
      transparent: true,
      opacity: 0.8,
      depthWrite: false,
    });
    const p = new THREE.Mesh(pGeo, pMat);
    p.position.set(mx, my, mz);
    graph.scene().add(p);

    eventSprites.push({
      mesh: p,
      birth: performance.now() / 1000 + 0.3,
      lifetime: 1.0,
      type: 'burst',
      velocity: {
        x: (Math.random() - 0.5) * 15,
        y: (Math.random() - 0.5) * 15,
        z: (Math.random() - 0.5) * 15,
      },
    });
  }

  // Prune oldest
  while (eventSprites.length > EVENT_SPRITE_MAX) {
    const old = eventSprites.shift();
    graph.scene().remove(old.mesh);
    old.mesh.geometry.dispose();
    old.mesh.material.dispose();
  }
}

// bd-k9cqt: Energy beam effect for dependency creation
/**
 * Spawn an energy beam effect between two nodes for dependency creation.
 * @param {GraphNode} sourceNode - Source node of the beam
 * @param {GraphNode} targetNode - Target node of the beam
 * @param {number} color - Hex color for the beam
 * @returns {void}
 */
export function spawnEnergyBeam(sourceNode, targetNode, color) {
  const graph = _getGraph && _getGraph();
  if (!sourceNode || !targetNode || !graph) return;
  const scene = graph.scene();
  const now = performance.now() / 1000;
  const sx = sourceNode.x || 0,
    sy = sourceNode.y || 0,
    sz = sourceNode.z || 0;
  const tx = targetNode.x || 0,
    ty = targetNode.y || 0,
    tz = targetNode.z || 0;
  const dx = tx - sx,
    dy = ty - sy,
    dz = tz - sz;
  const dist = Math.sqrt(dx * dx + dy * dy + dz * dz);
  const beamGeo = new THREE.CylinderGeometry(0.3, 0.3, 1, 6, 1, true);
  const beamMat = new THREE.MeshBasicMaterial({
    color,
    transparent: true,
    opacity: 0.9,
    depthWrite: false,
    side: THREE.DoubleSide,
  });
  const beam = new THREE.Mesh(beamGeo, beamMat);
  beam.position.set(sx, sy, sz);
  scene.add(beam);
  eventSprites.push({ mesh: beam, birth: now, lifetime: 0.6, type: 'energy-beam', sourceNode, targetNode, dist });
  for (let i = 0; i < 10; i++) {
    const pGeo = new THREE.SphereGeometry(0.5, 4, 4);
    const pMat = new THREE.MeshBasicMaterial({ color, transparent: true, opacity: 0.7, depthWrite: false });
    const p = new THREE.Mesh(pGeo, pMat);
    p.position.set(sx, sy, sz);
    scene.add(p);
    eventSprites.push({
      mesh: p,
      birth: now + i * 0.04,
      lifetime: 0.8,
      type: 'beam-trail',
      sourceNode,
      targetNode,
      jitter: { x: (Math.random() - 0.5) * 3, y: (Math.random() - 0.5) * 3, z: (Math.random() - 0.5) * 3 },
    });
  }
  for (let li = 0; li < 2; li++) {
    const points = [];
    const segs = 6;
    for (let j = 0; j <= segs; j++) {
      const frac = j / segs;
      points.push(
        new THREE.Vector3(
          sx + dx * frac + (j > 0 && j < segs ? (Math.random() - 0.5) * 4 : 0),
          sy + dy * frac + (j > 0 && j < segs ? (Math.random() - 0.5) * 4 : 0),
          sz + dz * frac + (j > 0 && j < segs ? (Math.random() - 0.5) * 4 : 0),
        ),
      );
    }
    const lineGeo = new THREE.BufferGeometry().setFromPoints(points);
    const lineMat = new THREE.LineBasicMaterial({
      color: 0xffffff,
      transparent: true,
      opacity: 0.6,
      depthWrite: false,
    });
    const line = new THREE.Line(lineGeo, lineMat);
    scene.add(line);
    eventSprites.push({
      mesh: line,
      birth: now,
      lifetime: 0.5,
      type: 'lightning',
      sourceNode,
      targetNode,
      segments: segs,
    });
  }
  for (let i = 0; i < 12; i++) {
    const pGeo = new THREE.SphereGeometry(0.4, 4, 4);
    const pMat = new THREE.MeshBasicMaterial({ color, transparent: true, opacity: 0.8, depthWrite: false });
    const p = new THREE.Mesh(pGeo, pMat);
    p.position.set(tx, ty, tz);
    scene.add(p);
    eventSprites.push({
      mesh: p,
      birth: now + 0.45,
      lifetime: 0.8,
      type: 'burst',
      velocity: { x: (Math.random() - 0.5) * 20, y: (Math.random() - 0.5) * 20, z: (Math.random() - 0.5) * 20 },
    });
  }
  const flashGeo = new THREE.SphereGeometry(2, 8, 8);
  const flashMat = new THREE.MeshBasicMaterial({ color: 0xffffff, transparent: true, opacity: 0.9, depthWrite: false });
  const flash = new THREE.Mesh(flashGeo, flashMat);
  flash.position.set(tx, ty, tz);
  scene.add(flash);
  eventSprites.push({
    mesh: flash,
    birth: now + 0.45,
    lifetime: 0.4,
    type: 'creation-flash',
    node: targetNode,
    startScale: 1,
    endScale: 4,
  });
  while (eventSprites.length > EVENT_SPRITE_MAX) {
    const old = eventSprites.shift();
    scene.remove(old.mesh);
    old.mesh.geometry.dispose();
    old.mesh.material.dispose();
  }
}

// Update event sprites each frame (bd-9qeto)
/**
 * Update all active event sprites (expanding rings, sparks, comets, etc.).
 * @param {number} t - Current animation time in seconds
 * @returns {void}
 */
export function updateEventSprites(t) {
  const graph = _getGraph && _getGraph();
  const _particlePool = _getParticlePool && _getParticlePool();
  for (let i = eventSprites.length - 1; i >= 0; i--) {
    const s = eventSprites[i];
    const age = t - s.birth;

    // Not born yet (staggered spawns)
    if (age < 0) continue;

    if (age > s.lifetime) {
      if (graph) graph.scene().remove(s.mesh);
      s.mesh.geometry.dispose();
      s.mesh.material.dispose();
      eventSprites.splice(i, 1);
      continue;
    }

    const progress = age / s.lifetime; // 0→1

    if (s.type === 'status-pulse') {
      // Expanding ring that fades out
      const scale = s.startScale + (s.endScale - s.startScale) * progress;
      s.mesh.scale.setScalar(scale);
      s.mesh.material.opacity = (1 - progress) * 0.7;
      // Follow node position
      if (s.node && graph) {
        s.mesh.position.set(s.node.x || 0, s.node.y || 0, s.node.z || 0);
        s.mesh.lookAt(graph.camera().position);
      }
    } else if (s.type === 'edge-spark') {
      // Interpolate from source to target with slight jitter
      const sx = s.sourceNode.x || 0,
        sy = s.sourceNode.y || 0,
        sz = s.sourceNode.z || 0;
      const tx = s.targetNode.x || 0,
        ty = s.targetNode.y || 0,
        tz = s.targetNode.z || 0;
      const wobble = Math.sin(progress * Math.PI * 3) * (1 - progress);
      s.mesh.position.set(
        sx + (tx - sx) * progress + s.jitter.x * wobble,
        sy + (ty - sy) * progress + s.jitter.y * wobble,
        sz + (tz - sz) * progress + s.jitter.z * wobble,
      );
      // Shrink and fade near the end
      const sparkScale = 1 - progress * 0.7;
      s.mesh.scale.setScalar(sparkScale);
      s.mesh.material.opacity = (1 - progress * progress) * 0.9;
    } else if (s.type === 'creation-flash') {
      // Bright bloom spike: rapid expand then fade (bd-4gmot)
      const scale = s.startScale + (s.endScale - s.startScale) * progress;
      s.mesh.scale.setScalar(scale);
      s.mesh.material.opacity = (1 - progress * progress) * 0.9;
      // Follow node position
      if (s.node) {
        s.mesh.position.set(s.node.x || 0, s.node.y || 0, s.node.z || 0);
      }
    } else if (s.type === 'shockwave') {
      // Expanding torus ring with inverse-square opacity falloff (bd-3fnon)
      const scale = s.startScale + (s.endScale - s.startScale) * progress;
      s.mesh.scale.setScalar(scale);
      // Inverse square falloff for opacity
      const falloff = 1 / (1 + progress * progress * 4);
      s.mesh.material.opacity = falloff * 0.9;
      // Follow node position and face camera
      if (s.node && graph) {
        s.mesh.position.set(s.node.x || 0, s.node.y || 0, s.node.z || 0);
        s.mesh.lookAt(graph.camera().position);
      }
    } else if (s.type === 'burst') {
      // Outward burst particles with gravity-like deceleration
      const decel = 1 - progress * 0.8; // slow down over time
      s.mesh.position.x += s.velocity.x * 0.016 * decel;
      s.mesh.position.y += s.velocity.y * 0.016 * decel;
      s.mesh.position.z += s.velocity.z * 0.016 * decel;
      s.mesh.material.opacity = (1 - progress) * 0.8;
      s.mesh.scale.setScalar(1 - progress * 0.5);
    } else if (s.type === 'comet-head') {
      // Comet flying along bezier arc with particle trail (bd-t4umc)
      s.mesh.position.copy(s.curve.getPointAt(Math.min(progress, 1.0)));
      const expectedEmitted = Math.floor(progress * s.trailCount);
      while (s._emitted < expectedEmitted && _particlePool) {
        const trailT = s._emitted / s.trailCount;
        const tp = s.curve.getPointAt(Math.min(trailT, 1.0));
        const tangent = s.curve.getTangentAt(Math.min(trailT, 1.0));
        _particlePool.emit({ x: tp.x, y: tp.y, z: tp.z }, s.color, 1, {
          velocity: [
            -tangent.x * 3 + (Math.random() - 0.5) * 2,
            -tangent.y * 3 + (Math.random() - 0.5) * 2,
            -tangent.z * 3 + (Math.random() - 0.5) * 2,
          ],
          spread: 0.5,
          lifetime: 0.6 + Math.random() * 0.3,
          size: 2.5 * (1 - trailT * 0.5),
        });
        s._emitted++;
      }
      s.mesh.material.opacity = Math.max(0.3, 1 - progress * 0.7);
      if (progress >= 0.95 && !s._arrived) {
        s._arrived = true;
        const tgt = s.node;
        if (tgt && _particlePool) {
          const tp2 = { x: tgt.x || 0, y: tgt.y || 0, z: tgt.z || 0 };
          for (let j = 0; j < 20; j++) {
            const a = (j / 20) * Math.PI * 2;
            _particlePool.emit(tp2, s.color, 1, {
              velocity: [Math.cos(a) * 4, (Math.random() - 0.5) * 2, Math.sin(a) * 4],
              spread: 0.3,
              lifetime: 0.6,
              size: 1.5,
            });
          }
        }
        if (tgt && graph) {
          const pulseGeo = new THREE.SphereGeometry(4, 10, 10);
          const pulseMat = new THREE.MeshBasicMaterial({
            color: 0xd4a017,
            transparent: true,
            opacity: 0.6,
            depthWrite: false,
            blending: THREE.AdditiveBlending,
          });
          const pulse = new THREE.Mesh(pulseGeo, pulseMat);
          pulse.position.set(tgt.x || 0, tgt.y || 0, tgt.z || 0);
          graph.scene().add(pulse);
          eventSprites.push({
            mesh: pulse,
            node: tgt,
            birth: performance.now() / 1000,
            lifetime: 0.5,
            type: 'creation-flash',
            startScale: 1.0,
            endScale: 1.3,
          });
        }
      }
    } else if (s.type === 'collapse-ring') {
      // bd-1n122: Contracting ring — cubic-in easing
      const ease = progress * progress * progress;
      const scale = s.startScale + (s.endScale - s.startScale) * ease;
      s.mesh.scale.setScalar(Math.max(scale, 0.01));
      s.mesh.material.opacity = (1 - ease) * 0.6;
      if (s.node && graph) {
        s.mesh.position.set(s.node.x || 0, s.node.y || 0, s.node.z || 0);
        s.mesh.lookAt(graph.camera().position);
      }
    } else if (s.type === 'collapse-ghost') {
      // bd-1n122: Ghostly afterimage — slowly shrinks and fades
      const scale = s.startScale + (s.endScale - s.startScale) * progress;
      s.mesh.scale.setScalar(scale);
      s.mesh.material.opacity = 0.35 * (1 - progress * progress);
    } else if (s.type === 'energy-beam') {
      // bd-k9cqt: Beam cylinder stretches from source toward target
      const src = s.sourceNode,
        tgt = s.targetNode;
      const sx = src.x || 0,
        sy = src.y || 0,
        sz = src.z || 0;
      const tx = tgt.x || 0,
        ty = tgt.y || 0,
        tz = tgt.z || 0;
      const reach = Math.min(progress / 0.75, 1);
      const dx = (tx - sx) * reach,
        dy = (ty - sy) * reach,
        dz = (tz - sz) * reach;
      const len = Math.sqrt(dx * dx + dy * dy + dz * dz) || 0.01;
      s.mesh.scale.set(1, len, 1);
      s.mesh.position.set(sx + dx * 0.5, sy + dy * 0.5, sz + dz * 0.5);
      s.mesh.lookAt(sx + dx, sy + dy, sz + dz);
      s.mesh.rotateX(Math.PI / 2);
      s.mesh.material.opacity = progress > 0.75 ? (1 - (progress - 0.75) / 0.25) * 0.9 : 0.9;
    } else if (s.type === 'beam-trail') {
      // bd-k9cqt: Trail particles along beam path, drift outward
      const src = s.sourceNode,
        tgt = s.targetNode;
      const sx = src.x || 0,
        sy = src.y || 0,
        sz = src.z || 0;
      const tx = tgt.x || 0,
        ty = tgt.y || 0,
        tz = tgt.z || 0;
      const travel = Math.min(progress * 1.5, 1);
      s.mesh.position.set(
        sx + (tx - sx) * travel + (progress > 0.5 ? s.jitter.x * (progress - 0.5) * 2 : 0),
        sy + (ty - sy) * travel + (progress > 0.5 ? s.jitter.y * (progress - 0.5) * 2 : 0),
        sz + (tz - sz) * travel + (progress > 0.5 ? s.jitter.z * (progress - 0.5) * 2 : 0),
      );
      s.mesh.material.opacity = (1 - progress) * 0.7;
      s.mesh.scale.setScalar(1 - progress * 0.6);
    } else if (s.type === 'lightning') {
      // bd-k9cqt: Jagged lines with vertex regeneration for flicker
      const src = s.sourceNode,
        tgt = s.targetNode;
      const sx = src.x || 0,
        sy = src.y || 0,
        sz = src.z || 0;
      const tx = tgt.x || 0,
        ty = tgt.y || 0,
        tz = tgt.z || 0;
      const dx = tx - sx,
        dy = ty - sy,
        dz = tz - sz;
      if (!s._lastRegen || t - s._lastRegen > 0.05) {
        s._lastRegen = t;
        const positions = s.mesh.geometry.attributes.position;
        const segs = s.segments;
        for (let j = 0; j <= segs; j++) {
          const frac = j / segs;
          const jitterAmt = j > 0 && j < segs ? 4 * (1 - progress) : 0;
          positions.setXYZ(
            j,
            sx + dx * frac + (Math.random() - 0.5) * jitterAmt,
            sy + dy * frac + (Math.random() - 0.5) * jitterAmt,
            sz + dz * frac + (Math.random() - 0.5) * jitterAmt,
          );
        }
        positions.needsUpdate = true;
      }
      s.mesh.material.opacity = (1 - progress * progress) * 0.6;
    }
  }

  // bd-1n122: Animate node scale during active collapses
  for (const [id, collapse] of _activeCollapses) {
    const elapsed = t - collapse.startTime;
    if (collapse.phase === 'collapsing') {
      const p = Math.min(elapsed / 0.8, 1);
      const ease = p * p * p;
      const threeObj = collapse.node.__threeObj;
      if (threeObj) threeObj.scale.setScalar(Math.max(1 - ease, 0.01));
    }
    if (elapsed > 3.0) _activeCollapses.delete(id);
  }
}
