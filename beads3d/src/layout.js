// --- Layout modes (bd-7t6nt) ---
// Extracted from main.js: layout switching, DAG dragging subtree (beads-6253),
// and agent DAG tether (beads-1gx1).

import * as THREE from 'three';

// Dependency injection — set by main.js before use
let _deps = {};

/**
 * Inject dependencies from main.js.
 *
 * @param {Object} deps
 * @param {Function} deps.getGraph         - () => ForceGraph3D instance
 * @param {Function} deps.getGraphData     - () => { nodes, links }
 * @param {Function} deps.getLayoutGuides  - () => THREE.Object3D[]
 * @param {Function} deps.setLayoutGuides  - (arr) => void
 */
export function setLayoutDeps(deps) {
  _deps = deps;
}

// --- Layout state ---
let currentLayout = 'free';

export function getCurrentLayout() {
  return currentLayout;
}

// Agent tether strength — 0 = off, 1 = max pull (bd-uzj5j)
let _agentTetherStrength = 0.5;

export function getAgentTetherStrength() {
  return _agentTetherStrength;
}

export function setAgentTetherStrength(v) {
  _agentTetherStrength = v;
}

// --- Layout helpers ---

function clearLayoutGuides() {
  const graph = _deps.getGraph();
  const layoutGuides = _deps.getLayoutGuides();
  const scene = graph.scene();
  for (const obj of layoutGuides) {
    scene.remove(obj);
    if (obj.geometry) obj.geometry.dispose();
    if (obj.material) obj.material.dispose();
    // Sprite sheets
    if (obj.material && obj.material.map) obj.material.map.dispose();
  }
  _deps.setLayoutGuides([]);
}

export function makeTextSprite(text, opts = {}) {
  const fontSize = opts.fontSize || 24;
  const color = opts.color || '#4a9eff';
  const bg = opts.background || null; // e.g. 'rgba(10, 10, 18, 0.85)'
  const padding = bg ? 10 : 8;
  const canvas = document.createElement('canvas');
  const ctx = canvas.getContext('2d');
  ctx.font = `${fontSize}px SF Mono, Fira Code, monospace`;
  const metrics = ctx.measureText(text);
  canvas.width = Math.ceil(metrics.width) + padding * 2;
  canvas.height = fontSize + padding * 2;
  // Background (bd-jy0yt)
  if (bg) {
    ctx.fillStyle = bg;
    const r = 4,
      cw = canvas.width,
      ch = canvas.height;
    ctx.beginPath();
    ctx.moveTo(r, 0);
    ctx.lineTo(cw - r, 0);
    ctx.arcTo(cw, 0, cw, r, r);
    ctx.lineTo(cw, ch - r);
    ctx.arcTo(cw, ch, cw - r, ch, r);
    ctx.lineTo(r, ch);
    ctx.arcTo(0, ch, 0, ch - r, r);
    ctx.lineTo(0, r);
    ctx.arcTo(0, 0, r, 0, r);
    ctx.closePath();
    ctx.fill();
  }
  ctx.font = `${fontSize}px SF Mono, Fira Code, monospace`;
  ctx.fillStyle = color;
  ctx.textBaseline = 'top';
  ctx.fillText(text, padding, padding);
  const tex = new THREE.CanvasTexture(canvas);
  tex.minFilter = THREE.LinearFilter;
  const screenSpace = opts.sizeAttenuation === false;
  const mat = new THREE.SpriteMaterial({
    map: tex,
    transparent: true,
    opacity: opts.opacity || 0.6,
    depthWrite: false,
    depthTest: false,
    sizeAttenuation: !screenSpace,
  });
  const sprite = new THREE.Sprite(mat);
  if (screenSpace) {
    const aspect = canvas.width / canvas.height;
    const spriteH = opts.screenHeight || 0.03; // fraction of viewport height
    sprite.scale.set(spriteH * aspect, spriteH, 1);
  } else {
    sprite.scale.set(canvas.width / 4, canvas.height / 4, 1);
  }
  return sprite;
}

function addRadialGuides() {
  const graph = _deps.getGraph();
  const layoutGuides = _deps.getLayoutGuides();
  const scene = graph.scene();
  const radiusScale = 80; // match radial layout force (bd-22dga)
  const labels = ['P0', 'P1', 'P2', 'P3', 'P4'];
  for (let p = 0; p <= 4; p++) {
    const r = (p + 0.5) * radiusScale;
    // Ring in XZ plane
    const ringGeo = new THREE.RingGeometry(r - 0.3, r + 0.3, 64);
    const ringMat = new THREE.MeshBasicMaterial({
      color: 0x1a2a3a,
      transparent: true,
      opacity: 0.15,
      side: THREE.DoubleSide,
    });
    const ring = new THREE.Mesh(ringGeo, ringMat);
    ring.rotation.x = -Math.PI / 2; // lay flat in XZ
    scene.add(ring);
    layoutGuides.push(ring);

    // Priority label
    const label = makeTextSprite(labels[p], { fontSize: 20, color: '#2a3a4a', opacity: 0.4 });
    label.position.set(r + 8, 2, 0);
    scene.add(label);
    layoutGuides.push(label);
  }
}

function addClusterGuides(nodes) {
  const graph = _deps.getGraph();
  const layoutGuides = _deps.getLayoutGuides();
  const scene = graph.scene();
  const assignees = [...new Set(nodes.map((n) => n.assignee || '(unassigned)'))];
  const clusterRadius = Math.max(assignees.length * 40, 150);

  assignees.forEach((a, i) => {
    const angle = (i / assignees.length) * Math.PI * 2;
    const x = Math.cos(angle) * clusterRadius;
    const z = Math.sin(angle) * clusterRadius;

    // Assignee label
    const label = makeTextSprite(a, { fontSize: 22, color: '#ff6b35', opacity: 0.5 });
    label.position.set(x, 15, z);
    scene.add(label);
    layoutGuides.push(label);

    // Small anchor ring at cluster center
    const ringGeo = new THREE.RingGeometry(8, 10, 24);
    const ringMat = new THREE.MeshBasicMaterial({
      color: 0xff6b35,
      transparent: true,
      opacity: 0.08,
      side: THREE.DoubleSide,
    });
    const ring = new THREE.Mesh(ringGeo, ringMat);
    ring.position.set(x, 0, z);
    ring.rotation.x = -Math.PI / 2;
    scene.add(ring);
    layoutGuides.push(ring);
  });
}

export function setLayout(mode) {
  const graph = _deps.getGraph?.();
  const graphData = _deps.getGraphData?.();
  currentLayout = mode;

  // Highlight active button (bd-7zczp: sync aria-checked for radio group)
  document.querySelectorAll('#layout-controls button').forEach((b) => {
    b.classList.remove('active');
    b.setAttribute('aria-checked', 'false');
  });
  const btn = document.getElementById(`btn-layout-${mode}`);
  if (btn) {
    btn.classList.add('active');
    btn.setAttribute('aria-checked', 'true');
  }
  const layoutSel = document.getElementById('cp-layout-mode');
  if (layoutSel && layoutSel.value !== mode) layoutSel.value = mode;

  // Guard: deps not wired yet (called before setLayoutDeps during init)
  if (!graph || !graphData) return;

  // Clear all custom forces and visual guides
  clearLayoutGuides();
  graph.dagMode(null);
  graph.d3Force('radialPriority', null);
  graph.d3Force('clusterAssignee', null);
  graph.d3Force('flattenY', null);
  graph.d3Force('flattenZ', null);

  // Restore default forces
  const nodeCount = graphData.nodes.length || 100;

  switch (mode) {
    case 'free':
      graph
        .d3Force('charge')
        .strength(nodeCount > 200 ? -60 : -120)
        .distanceMax(400);
      graph.d3Force('link').distance(nodeCount > 200 ? 40 : 60);
      break;

    case 'dag': {
      // Hierarchical top-down: dagMode positions Y by depth.
      // Add stronger charge repulsion in X/Z to spread nodes within layers. (bd-22dga)
      graph.dagMode('td');
      graph.d3Force('charge').strength(-80).distanceMax(500);
      graph.d3Force('link').distance(50);
      graph.dagLevelDistance(60);
      // Flatten Z so layers are clearly visible in the X-Y plane.
      graph.d3Force('flattenZ', (alpha) => {
        for (const node of graphData.nodes) {
          if (node._hidden) continue;
          node.vz += (0 - (node.z || 0)) * alpha * 0.3;
        }
      });
      graph.cameraPosition({ x: 0, y: 0, z: 500 }, { x: 0, y: 0, z: 0 }, 1200);
      break;
    }

    case 'radial': {
      // Radial: distance from center = priority (P0 center, P4 outer). (bd-22dga)
      // Weaker charge so radial force dominates; stronger damping for distinct rings.
      graph.d3Force('charge').strength(-8).distanceMax(120);
      graph.d3Force('link').distance(15);

      const radiusScale = 80; // pixels per priority level (wider rings)
      graph.d3Force('radialPriority', (alpha) => {
        for (const node of graphData.nodes) {
          if (node._hidden) continue;
          const targetR = (node.priority + 0.5) * radiusScale;
          const x = node.x || 0;
          const z = node.z || 0;
          const currentR = Math.sqrt(x * x + z * z) || 1;
          const factor = (targetR / currentR - 1) * alpha * 0.5;
          node.vx += x * factor;
          node.vz += z * factor;
        }
      });
      // Flatten Y for a disc layout
      graph.d3Force('flattenY', (alpha) => {
        for (const node of graphData.nodes) {
          if (node._hidden) continue;
          node.vy += (0 - (node.y || 0)) * alpha * 0.5;
        }
      });
      addRadialGuides();
      // Top-down camera for disc view
      graph.cameraPosition({ x: 0, y: 500, z: 50 }, { x: 0, y: 0, z: 0 }, 1200);
      break;
    }

    case 'cluster': {
      // Cluster by assignee: each assignee gets an anchor point on a circle. (bd-22dga)
      // Weaker charge within clusters; stronger anchor damping for distinct grouping.
      graph.d3Force('charge').strength(-10).distanceMax(100);
      graph.d3Force('link').distance(15);

      // Build assignee -> anchor position map (wider circle for clear separation)
      const assignees = [...new Set(graphData.nodes.map((n) => n.assignee || '(unassigned)'))];
      const anchorMap = {};
      const clusterRadius = Math.max(assignees.length * 60, 200);
      assignees.forEach((a, i) => {
        const angle = (i / assignees.length) * Math.PI * 2;
        anchorMap[a] = {
          x: Math.cos(angle) * clusterRadius,
          z: Math.sin(angle) * clusterRadius,
        };
      });

      graph.d3Force('clusterAssignee', (alpha) => {
        for (const node of graphData.nodes) {
          if (node._hidden) continue;
          const anchor = anchorMap[node.assignee || '(unassigned)'];
          if (!anchor) continue;
          node.vx += (anchor.x - (node.x || 0)) * alpha * 0.4;
          node.vz += (anchor.z - (node.z || 0)) * alpha * 0.4;
        }
      });
      // Flatten Y for a disc layout
      graph.d3Force('flattenY', (alpha) => {
        for (const node of graphData.nodes) {
          if (node._hidden) continue;
          node.vy += (0 - (node.y || 0)) * alpha * 0.5;
        }
      });
      addClusterGuides(graphData.nodes);
      // Top-down camera for cluster disc view
      graph.cameraPosition({ x: 0, y: 500, z: 50 }, { x: 0, y: 0, z: 0 }, 1200);
      break;
    }
  }

  // Reheat simulation to animate the transition
  graph.d3ReheatSimulation();

  // Re-apply agent tether force (survives layout changes)
  setupAgentTether();
  // Re-apply epic clustering force (kd-XGgiokgQBH)
  setupEpicCluster();
}

// --- DAG Dragging Subtree (beads-6253) ---
// Returns array of {node, depth} for all nodes reachable from startId.
// Agents: follow assigned_to edges downstream only.
// Beads: follow all edge types bidirectionally (full connected component).
export function getDragSubtree(startId) {
  const graphData = _deps.getGraphData();
  const nodeById = new Map();
  for (const n of graphData.nodes) {
    if (!n._hidden) nodeById.set(n.id, n);
  }

  const result = [];
  const visited = new Set();
  const queue = [{ id: startId, depth: 0 }];
  const startNode = nodeById.get(startId);
  const isAgent = startNode && startNode.issue_type === 'agent';

  while (queue.length > 0) {
    const { id, depth } = queue.shift();
    if (visited.has(id)) continue;
    visited.add(id);
    const node = nodeById.get(id);
    if (!node) continue;
    result.push({ node, depth });

    // Max 4 hops to limit subtree size
    if (depth >= 4) continue;

    for (const l of graphData.links) {
      const srcId = typeof l.source === 'object' ? l.source.id : l.source;
      const tgtId = typeof l.target === 'object' ? l.target.id : l.target;

      if (isAgent) {
        // Agents: only follow outgoing assigned_to edges, then deps downstream
        if (srcId === id && !visited.has(tgtId)) {
          queue.push({ id: tgtId, depth: depth + 1 });
        }
      } else {
        // Beads: follow edges bidirectionally
        if (srcId === id && !visited.has(tgtId)) {
          queue.push({ id: tgtId, depth: depth + 1 });
        }
        if (tgtId === id && !visited.has(srcId)) {
          queue.push({ id: srcId, depth: depth + 1 });
        }
      }
    }
  }
  return result;
}

// --- Agent DAG Tether (beads-1gx1) ---
// Strong elastic coupling between agent nodes and their claimed bead subtrees.
// When an agent moves (drag or force), its beads follow like a kite tail.
// Force propagates: agent -> assigned bead -> bead's dependencies (with decay).
export function setupAgentTether() {
  const graph = _deps.getGraph();
  const graphData = _deps.getGraphData();
  graph.d3Force('agentTether', (alpha) => {
    // Build agent -> bead adjacency from current links
    const nodeById = new Map();
    for (const n of graphData.nodes) {
      if (!n._hidden) nodeById.set(n.id, n);
    }

    // Collect agent->bead assignments
    const agentBeads = new Map(); // agentId -> [beadNode, ...]
    for (const l of graphData.links) {
      if (l.dep_type !== 'assigned_to') continue;
      const srcId = typeof l.source === 'object' ? l.source.id : l.source;
      const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      const agent = nodeById.get(srcId);
      const bead = nodeById.get(tgtId);
      if (!agent || !bead || agent.issue_type !== 'agent') continue;
      if (!agentBeads.has(srcId)) agentBeads.set(srcId, []);
      agentBeads.get(srcId).push(bead);
    }

    // Build dep adjacency for subtree traversal (bead -> its deps)
    const deps = new Map(); // nodeId -> [depNode, ...]
    for (const l of graphData.links) {
      if (l.dep_type === 'assigned_to') continue;
      const srcId = typeof l.source === 'object' ? l.source.id : l.source;
      const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      // parent->child and blocks edges: target is the dependent
      const parent = nodeById.get(srcId);
      const child = nodeById.get(tgtId);
      if (!parent || !child) continue;
      if (!deps.has(srcId)) deps.set(srcId, []);
      deps.get(srcId).push(child);
    }

    // Skip entirely when tether disabled (bd-uzj5j)
    if (_agentTetherStrength <= 0) return;

    // Apply spring force: agent pulls beads, beads pull deps (with decay)
    const BASE_STRENGTH = 0.3; // max tether pull (scaled by slider)
    const TETHER_STRENGTH = BASE_STRENGTH * _agentTetherStrength;
    const DECAY = 0.5; // force halves per hop
    const REST_DIST = 20; // desired agent->bead distance

    for (const [agentId, beads] of agentBeads) {
      const agent = nodeById.get(agentId);
      if (!agent || agent.x === undefined) continue;

      // BFS from agent through beads and their deps
      const queue = beads.map((b) => ({ node: b, depth: 1 }));
      const visited = new Set([agentId]);

      while (queue.length > 0) {
        const { node, depth } = queue.shift();
        if (visited.has(node.id)) continue;
        visited.add(node.id);
        if (node.x === undefined) continue;

        const strength = TETHER_STRENGTH * Math.pow(DECAY, depth - 1);
        const dx = agent.x - node.x;
        const dy = agent.y - node.y;
        const dz = (agent.z || 0) - (node.z || 0);
        const dist = Math.sqrt(dx * dx + dy * dy + dz * dz) || 1;

        // Only apply pull when beyond rest distance
        if (dist > REST_DIST * depth) {
          const pull = strength * alpha;
          node.vx += dx * pull;
          node.vy += dy * pull;
          node.vz += dz * pull;
          // Counter-force on agent (Newton's 3rd, dampened)
          const counterPull = pull * 0.15;
          agent.vx -= dx * counterPull;
          agent.vy -= dy * counterPull;
          agent.vz -= dz * counterPull;
        }

        // Enqueue this node's deps (up to 3 hops)
        if (depth < 3) {
          const children = deps.get(node.id) || [];
          for (const child of children) {
            if (!visited.has(child.id)) {
              queue.push({ node: child, depth: depth + 1 });
            }
          }
        }
      }
    }
  });
}

// --- Epic Clustering Force (kd-XGgiokgQBH) ---
// Pulls child-of and parent-child children toward their epic parent node,
// creating visual clusters around epics.
export function setupEpicCluster() {
  const graph = _deps.getGraph();
  const graphData = _deps.getGraphData();
  graph.d3Force('epicCluster', (alpha) => {
    const nodeById = new Map();
    for (const n of graphData.nodes) {
      if (!n._hidden) nodeById.set(n.id, n);
    }

    // Build epic → children mapping from parent-child and child-of edges
    const epicChildren = new Map(); // epicId → [childNode, ...]
    for (const l of graphData.links) {
      if (l.dep_type !== 'parent-child' && l.dep_type !== 'child-of') continue;
      const srcId = typeof l.source === 'object' ? l.source.id : l.source;
      const tgtId = typeof l.target === 'object' ? l.target.id : l.target;
      const src = nodeById.get(srcId);
      const tgt = nodeById.get(tgtId);
      if (!src || !tgt) continue;
      // parent-child: source is parent, target is child
      // child-of: target depends on (is child of) source
      const parent = src;
      const child = tgt;
      if (parent.issue_type !== 'epic') continue;
      if (!epicChildren.has(srcId)) epicChildren.set(srcId, []);
      epicChildren.get(srcId).push(child);
    }

    // Pull children toward their epic parent
    const CLUSTER_STRENGTH = 0.15;
    const REST_DIST = 30; // desired child distance from epic
    for (const [epicId, children] of epicChildren) {
      const epic = nodeById.get(epicId);
      if (!epic || epic.x === undefined || epic._epicCollapsed) continue;
      for (const child of children) {
        if (child.x === undefined || child._hidden) continue;
        const dx = epic.x - child.x;
        const dy = epic.y - child.y;
        const dz = epic.z - child.z;
        const dist = Math.sqrt(dx * dx + dy * dy + dz * dz) || 1;
        if (dist < REST_DIST) continue; // already close enough
        const pull = CLUSTER_STRENGTH * alpha * Math.min((dist - REST_DIST) / dist, 1);
        child.vx += dx * pull;
        child.vy += dy * pull;
        child.vz += dz * pull;
        // Gentle counter-force on epic
        epic.vx -= dx * pull * 0.05;
        epic.vy -= dy * pull * 0.05;
        epic.vz -= dz * pull * 0.05;
      }
    }
  });
}
