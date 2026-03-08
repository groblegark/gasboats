// --- Minimap (bd-7t6nt) ---
// Extracted from main.js: minimap rendering, click-to-teleport, toggle.

import { nodeColor } from './colors.js';

// Dependency injection — set by main.js before use
let _deps = {};

/**
 * Inject dependencies from main.js.
 *
 * @param {Object} deps
 * @param {Function} deps.getGraph           - () => ForceGraph3D instance
 * @param {Function} deps.getGraphData       - () => { nodes, links }
 * @param {Function} deps.getHighlightNodes  - () => Set
 * @param {Function} deps.getSelectedNode    - () => node | null
 */
export function setMinimapDeps(deps) {
  _deps = deps;
}

// --- Minimap state ---
let minimapVisible = true;

export function getMinimapVisible() {
  return minimapVisible;
}

export function setMinimapVisible(v) {
  minimapVisible = v;
}

// --- Minimap DOM refs ---
const minimapCanvas = document.getElementById('minimap');
const minimapCtx = minimapCanvas.getContext('2d');
const minimapLabel = document.getElementById('minimap-label');

export function renderMinimap() {
  const graph = _deps.getGraph?.();
  const graphData = _deps.getGraphData?.();
  if (!minimapVisible || !graph || graphData.nodes.length === 0) return;

  const w = minimapCanvas.width;
  const h = minimapCanvas.height;
  const ctx = minimapCtx;

  ctx.clearRect(0, 0, w, h);

  // Background
  ctx.fillStyle = 'rgba(10, 10, 20, 0.6)';
  ctx.fillRect(0, 0, w, h);

  // Compute bounding box of all visible nodes (top-down: X, Z)
  let minX = Infinity,
    maxX = -Infinity;
  let minZ = Infinity,
    maxZ = -Infinity;
  const visibleNodes = graphData.nodes.filter((n) => !n._hidden && n.x !== undefined);

  for (const n of visibleNodes) {
    if (n.x < minX) minX = n.x;
    if (n.x > maxX) maxX = n.x;
    if ((n.z || 0) < minZ) minZ = n.z || 0;
    if ((n.z || 0) > maxZ) maxZ = n.z || 0;
  }

  if (!isFinite(minX)) return; // no positioned nodes

  // Add padding
  const pad = 40;
  const rangeX = maxX - minX || 1;
  const rangeZ = maxZ - minZ || 1;
  const scale = Math.min((w - pad * 2) / rangeX, (h - pad * 2) / rangeZ);

  // Map world coords to minimap coords
  const toMiniX = (wx) => pad + (wx - minX) * scale;
  const toMiniY = (wz) => pad + (wz - minZ) * scale;

  // Store mapping for click-to-teleport
  minimapCanvas._mapState = { minX, minZ, scale, pad };

  // Draw links (thin lines)
  ctx.globalAlpha = 0.15;
  ctx.lineWidth = 0.5;
  for (const l of graphData.links) {
    const src = typeof l.source === 'object' ? l.source : null;
    const tgt = typeof l.target === 'object' ? l.target : null;
    if (!src || !tgt || src._hidden || tgt._hidden) continue;
    if (src.x === undefined || tgt.x === undefined) continue;

    ctx.strokeStyle = l.dep_type === 'blocks' ? '#d04040' : '#2a2a3a';
    ctx.beginPath();
    ctx.moveTo(toMiniX(src.x), toMiniY(src.z || 0));
    ctx.lineTo(toMiniX(tgt.x), toMiniY(tgt.z || 0));
    ctx.stroke();
  }
  ctx.globalAlpha = 1;

  // Draw nodes (dots)
  const highlightNodes = _deps.getHighlightNodes();
  for (const n of visibleNodes) {
    const mx = toMiniX(n.x);
    const my = toMiniY(n.z || 0);
    const r = n.issue_type === 'epic' ? 3 : n._blocked ? 2.5 : 1.5;
    const color = nodeColor(n);

    ctx.fillStyle = color;
    ctx.globalAlpha = highlightNodes.size > 0 ? (highlightNodes.has(n.id) ? 1 : 0.2) : 0.8;
    ctx.beginPath();
    ctx.arc(mx, my, r, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.globalAlpha = 1;

  // Draw selected node marker
  const selectedNode = _deps.getSelectedNode();
  if (selectedNode && selectedNode.x !== undefined) {
    const sx = toMiniX(selectedNode.x);
    const sy = toMiniY(selectedNode.z || 0);
    ctx.strokeStyle = '#4a9eff';
    ctx.lineWidth = 1.5;
    ctx.beginPath();
    ctx.arc(sx, sy, 6, 0, Math.PI * 2);
    ctx.stroke();
  }

  // Draw camera viewport indicator (frustum projected to XZ plane)
  const camera = graph.camera();
  const camPos = camera.position;
  // Camera footprint: show camera position as a small diamond
  const cx = toMiniX(camPos.x);
  const cy = toMiniY(camPos.z);
  ctx.strokeStyle = '#4a9eff';
  ctx.lineWidth = 1;
  ctx.globalAlpha = 0.6;

  // Viewport rectangle (approximate based on camera height and FOV)
  const fovRad = ((camera.fov / 2) * Math.PI) / 180;
  const camHeight = Math.abs(camPos.y) || 200;
  const halfW = Math.tan(fovRad) * camHeight * camera.aspect;
  const halfH = Math.tan(fovRad) * camHeight;
  const vw = halfW * scale;
  const vh = halfH * scale;

  ctx.strokeRect(cx - vw, cy - vh, vw * 2, vh * 2);

  // Camera dot
  ctx.fillStyle = '#4a9eff';
  ctx.globalAlpha = 0.8;
  ctx.beginPath();
  ctx.arc(cx, cy, 2, 0, Math.PI * 2);
  ctx.fill();

  ctx.globalAlpha = 1;
}

// Click on minimap -> teleport camera
minimapCanvas.addEventListener('click', (e) => {
  const graph = _deps.getGraph();
  const rect = minimapCanvas.getBoundingClientRect();
  const scaleX = minimapCanvas.width / rect.width;
  const scaleY = minimapCanvas.height / rect.height;
  const mx = (e.clientX - rect.left) * scaleX;
  const my = (e.clientY - rect.top) * scaleY;

  const state = minimapCanvas._mapState;
  if (!state) return;

  // Convert minimap coords back to world XZ
  const wx = (mx - state.pad) / state.scale + state.minX;
  const wz = (my - state.pad) / state.scale + state.minZ;

  // Keep current camera height (Y), move to clicked world position
  const camY = graph.camera().position.y || 200;
  graph.cameraPosition(
    { x: wx, y: camY, z: wz + camY * 0.3 }, // offset Z slightly to look down
    { x: wx, y: 0, z: wz },
    600,
  );
});

export function toggleMinimap() {
  minimapVisible = !minimapVisible;
  minimapCanvas.style.display = minimapVisible ? 'block' : 'none';
  minimapLabel.style.display = minimapVisible ? 'block' : 'none';
}
