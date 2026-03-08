// Custom shaders for beads3d visual effects
import * as THREE from 'three';

/**
 * @typedef {Object} ParticlePool
 * @property {Function} emit - Emit particles at a position with given properties
 * @property {Function} update - Update all active particles (call every frame)
 * @property {Function} dispose - (not currently implemented) Clean up GPU resources
 * @property {Function} activeCount - Returns the number of currently alive particles
 */

// --- Fresnel Glow Shader ---
// Creates a rim-lighting effect: transparent at center, glowing at edges.
// Used for the outer glow shell around each node.
/**
 * Create a Fresnel rim-lighting shader material.
 * @param {number} color - Hex color for the glow
 * @param {Object} [opts] - Optional parameters
 * @param {number} [opts.opacity=0.4] - Base opacity
 * @param {number} [opts.power=2.0] - Fresnel exponent (higher = tighter rim)
 * @returns {THREE.ShaderMaterial}
 */
export function createFresnelMaterial(color, { opacity = 0.4, power = 2.0 } = {}) {
  return new THREE.ShaderMaterial({
    uniforms: {
      glowColor: { value: new THREE.Color(color) },
      opacity: { value: opacity },
      power: { value: power },
    },
    vertexShader: `
      varying vec3 vNormal;
      varying vec3 vViewDir;
      void main() {
        vNormal = normalize(normalMatrix * normal);
        vec4 mvPos = modelViewMatrix * vec4(position, 1.0);
        vViewDir = normalize(-mvPos.xyz);
        gl_Position = projectionMatrix * mvPos;
      }
    `,
    fragmentShader: `
      uniform vec3 glowColor;
      uniform float opacity;
      uniform float power;
      varying vec3 vNormal;
      varying vec3 vViewDir;
      void main() {
        float fresnel = pow(1.0 - abs(dot(vNormal, vViewDir)), power);
        gl_FragColor = vec4(glowColor, fresnel * opacity);
      }
    `,
    transparent: true,
    depthWrite: false,
    side: THREE.FrontSide,
  });
}

// --- Pulsing Ring Shader ---
// Animated pulsing with color cycling for in-progress nodes.
/**
 * Create an animated pulsing ring shader material for in-progress nodes.
 * @param {number} color - Hex color for the ring
 * @returns {THREE.ShaderMaterial}
 */
export function createPulseRingMaterial(color) {
  return new THREE.ShaderMaterial({
    uniforms: {
      ringColor: { value: new THREE.Color(color) },
      time: { value: 0 },
      pulseCycle: { value: 4.0 }, // bd-b3ujw: controllable pulse speed
    },
    vertexShader: `
      varying vec2 vUv;
      void main() {
        vUv = uv;
        gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
      }
    `,
    fragmentShader: `
      uniform vec3 ringColor;
      uniform float time;
      uniform float pulseCycle;
      varying vec2 vUv;
      void main() {
        // Intermittent pulse: brief flash every ~Ns, fades quickly (bd-s9b4v, bd-b3ujw)
        float cycle = mod(time, pulseCycle);
        float pulse = smoothstep(0.0, 0.3, cycle) * smoothstep(1.0, 0.3, cycle) * 0.3;
        // Soft edges along the torus cross-section
        float dist = abs(vUv.y - 0.5) * 2.0;
        float softEdge = smoothstep(1.0, 0.3, dist);
        gl_FragColor = vec4(ringColor, pulse * softEdge);
      }
    `,
    transparent: true,
    depthWrite: false,
    side: THREE.DoubleSide,
  });
}

// --- Background Star Field ---
// Creates a GPU particle system of tiny stars for depth and atmosphere.
/**
 * Create a GPU particle system of tiny twinkling stars.
 * @param {number} [count=2000] - Number of star particles
 * @param {number} [radius=600] - Outer radius of the star sphere shell
 * @returns {THREE.Points}
 */
export function createStarField(count = 2000, radius = 600) {
  const positions = new Float32Array(count * 3);
  const sizes = new Float32Array(count);
  const alphas = new Float32Array(count);

  for (let i = 0; i < count; i++) {
    // Distribute in a sphere shell (inner radius 200, outer radius)
    const theta = Math.random() * Math.PI * 2;
    const phi = Math.acos(2 * Math.random() - 1);
    const r = 200 + Math.random() * (radius - 200);
    positions[i * 3] = r * Math.sin(phi) * Math.cos(theta);
    positions[i * 3 + 1] = r * Math.sin(phi) * Math.sin(theta);
    positions[i * 3 + 2] = r * Math.cos(phi);
    sizes[i] = 0.5 + Math.random() * 1.5;
    alphas[i] = 0.1 + Math.random() * 0.4;
  }

  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
  geometry.setAttribute('size', new THREE.BufferAttribute(sizes, 1));
  geometry.setAttribute('alpha', new THREE.BufferAttribute(alphas, 1));

  const material = new THREE.ShaderMaterial({
    uniforms: {
      time: { value: 0 },
      color: { value: new THREE.Color(0x6688aa) },
      twinkleSpeed: { value: 1.0 }, // bd-b3ujw: controllable twinkle speed
    },
    vertexShader: `
      attribute float size;
      attribute float alpha;
      varying float vAlpha;
      uniform float time;
      uniform float twinkleSpeed;
      void main() {
        vAlpha = alpha;
        vec4 mvPos = modelViewMatrix * vec4(position, 1.0);
        // Subtle twinkle: size oscillates per-particle (bd-b3ujw: speed controllable)
        float twinkle = 1.0 + 0.3 * sin(time * 1.5 * twinkleSpeed + position.x * 0.1);
        gl_PointSize = size * twinkle * (300.0 / -mvPos.z);
        gl_Position = projectionMatrix * mvPos;
      }
    `,
    fragmentShader: `
      uniform vec3 color;
      varying float vAlpha;
      void main() {
        // Circular soft point
        float d = length(gl_PointCoord - vec2(0.5));
        float a = smoothstep(0.5, 0.1, d);
        gl_FragColor = vec4(color, a * vAlpha);
      }
    `,
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });

  const points = new THREE.Points(geometry, material);
  points.userData.isStarField = true;
  return points;
}

// --- Fairy Lights (bd-52izs) ---
// Drifting luminous particles that float organically through the scene.
// Replaces the old geodesic dome nucleus/membrane with living light.
/**
 * Create drifting luminous fairy light particles.
 * @param {number} [count=300] - Number of fairy light particles
 * @param {number} [radius=250] - Outer radius of the particle sphere
 * @returns {THREE.Points}
 */
export function createFairyLights(count = 300, radius = 250) {
  const positions = new Float32Array(count * 3);
  const sizes = new Float32Array(count);
  const alphas = new Float32Array(count);
  const phases = new Float32Array(count); // per-particle phase offset
  const speeds = new Float32Array(count); // drift speed multiplier
  const hueShifts = new Float32Array(count); // slight color variation

  for (let i = 0; i < count; i++) {
    const theta = Math.random() * Math.PI * 2;
    const phi = Math.acos(2 * Math.random() - 1);
    const r = 20 + Math.random() * radius;
    positions[i * 3] = r * Math.sin(phi) * Math.cos(theta);
    positions[i * 3 + 1] = r * Math.sin(phi) * Math.sin(theta);
    positions[i * 3 + 2] = r * Math.cos(phi);
    sizes[i] = 1.0 + Math.random() * 3.0;
    alphas[i] = 0.15 + Math.random() * 0.5;
    phases[i] = Math.random() * Math.PI * 2;
    speeds[i] = 0.3 + Math.random() * 0.7;
    hueShifts[i] = Math.random();
  }

  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
  geometry.setAttribute('size', new THREE.BufferAttribute(sizes, 1));
  geometry.setAttribute('alpha', new THREE.BufferAttribute(alphas, 1));
  geometry.setAttribute('phase', new THREE.BufferAttribute(phases, 1));
  geometry.setAttribute('speed', new THREE.BufferAttribute(speeds, 1));
  geometry.setAttribute('hueShift', new THREE.BufferAttribute(hueShifts, 1));

  const material = new THREE.ShaderMaterial({
    uniforms: {
      time: { value: 0 },
      baseColor: { value: new THREE.Color(0x6688cc) },
      warmColor: { value: new THREE.Color(0xcc8866) },
      brightness: { value: 1.0 },
    },
    vertexShader: `
      attribute float size;
      attribute float alpha;
      attribute float phase;
      attribute float speed;
      attribute float hueShift;
      varying float vAlpha;
      varying float vHue;
      uniform float time;
      void main() {
        vHue = hueShift;
        // Organic drift: each particle orbits lazily on its own path
        float t = time * speed * 0.15;
        float p = phase;
        vec3 drift = vec3(
          sin(t + p) * 8.0 + cos(t * 0.7 + p * 2.0) * 4.0,
          cos(t * 0.9 + p * 1.3) * 6.0 + sin(t * 0.4 + p) * 3.0,
          sin(t * 0.6 + p * 0.8) * 7.0 + cos(t * 1.1 + p * 1.5) * 3.0
        );
        vec3 pos = position + drift;
        vec4 mvPos = modelViewMatrix * vec4(pos, 1.0);
        // Pulsing brightness: slow breathe per-particle
        float pulse = 0.6 + 0.4 * sin(time * 1.2 * speed + phase * 3.0);
        vAlpha = alpha * pulse;
        gl_PointSize = size * pulse * (250.0 / -mvPos.z);
        gl_Position = projectionMatrix * mvPos;
      }
    `,
    fragmentShader: `
      uniform vec3 baseColor;
      uniform vec3 warmColor;
      uniform float brightness;
      varying float vAlpha;
      varying float vHue;
      void main() {
        // Soft circular glow with warm core
        float d = length(gl_PointCoord - vec2(0.5));
        float a = smoothstep(0.5, 0.05, d);
        float core = smoothstep(0.3, 0.0, d); // bright center
        vec3 col = mix(baseColor, warmColor, vHue);
        col += vec3(core * 0.4); // white-hot center
        gl_FragColor = vec4(col * brightness, a * vAlpha);
      }
    `,
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });

  const points = new THREE.Points(geometry, material);
  points.userData.isFairyLights = true;
  points.frustumCulled = false;
  return points;
}

// --- Selection Pulse Shader ---
// A bright, pulsing ring for the selected node (replaces basic material).
// Set `visible` uniform to 1.0 to show, 0.0 to hide.
/**
 * Create a bright pulsing selection ring shader material.
 * @returns {THREE.ShaderMaterial}
 */
export function createSelectionRingMaterial() {
  return new THREE.ShaderMaterial({
    uniforms: {
      ringColor: { value: new THREE.Color(0x4a9eff) },
      time: { value: 0 },
      visible: { value: 0.0 },
    },
    vertexShader: `
      varying vec2 vUv;
      void main() {
        vUv = uv;
        gl_Position = projectionMatrix * modelViewMatrix * vec4(position, 1.0);
      }
    `,
    fragmentShader: `
      uniform vec3 ringColor;
      uniform float time;
      uniform float visible;
      varying vec2 vUv;
      void main() {
        if (visible < 0.5) discard;
        float pulse = 0.5 + 0.3 * sin(time * 4.0);
        // Animated sweep around the ring
        float sweep = sin(vUv.x * 6.2832 + time * 2.0) * 0.5 + 0.5;
        float dist = abs(vUv.y - 0.5) * 2.0;
        float softEdge = smoothstep(1.0, 0.2, dist);
        float alpha = pulse * softEdge * (0.6 + 0.4 * sweep);
        gl_FragColor = vec4(ringColor, alpha);
      }
    `,
    transparent: true,
    depthWrite: false,
    side: THREE.DoubleSide,
  });
}

// --- Materia Orb Shader (bd-1038x) ---
// FFVII-inspired translucent sphere: bright inner core, absorbed edges.
// Opposite of Fresnel rim-light — glow radiates from center outward.
// Breathing pulse for in-progress nodes, intensity for selection.
/**
 * Create a materia orb shader material with inner core glow.
 * @param {number} color - Hex color for the materia
 * @param {Object} [opts] - Optional parameters
 * @param {number} [opts.opacity=0.85] - Base opacity
 * @param {number} [opts.coreIntensity=1.4] - Core glow brightness
 * @param {number} [opts.breathSpeed=0.0] - Breathing pulse speed in Hz (0 = static)
 * @returns {THREE.ShaderMaterial}
 */
export function createMateriaMaterial(color, { opacity = 0.85, coreIntensity = 1.4, breathSpeed = 0.0 } = {}) {
  return new THREE.ShaderMaterial({
    uniforms: {
      materiaColor: { value: new THREE.Color(color) },
      opacity: { value: opacity },
      coreIntensity: { value: coreIntensity },
      breathSpeed: { value: breathSpeed },
      time: { value: 0 },
      selected: { value: 0.0 },
    },
    vertexShader: `
      varying vec3 vNormal;
      varying vec3 vViewDir;
      varying vec3 vWorldPos;
      void main() {
        vNormal = normalize(normalMatrix * normal);
        vec4 mvPos = modelViewMatrix * vec4(position, 1.0);
        vViewDir = normalize(-mvPos.xyz);
        vWorldPos = position;
        gl_Position = projectionMatrix * mvPos;
      }
    `,
    fragmentShader: `
      uniform vec3 materiaColor;
      uniform float opacity;
      uniform float coreIntensity;
      uniform float breathSpeed;
      uniform float time;
      uniform float selected;
      varying vec3 vNormal;
      varying vec3 vViewDir;
      varying vec3 vWorldPos;
      void main() {
        // Core glow: brightest at center, absorbed at edges (inverted Fresnel)
        float facing = abs(dot(vNormal, vViewDir));
        float core = pow(facing, 0.8) * coreIntensity;

        // Subsurface scattering approximation
        float sss = 0.3 + 0.7 * facing;

        // Breathing pulse (0 = no breathing, >0 = speed in Hz)
        float breath = 1.0;
        if (breathSpeed > 0.0) {
          breath = 0.85 + 0.15 * sin(time * breathSpeed * 6.2832);
        }

        // Selection boost
        float sel = 1.0 + selected * 0.8;

        // Edge absorption: darken at extreme grazing angles
        float edgeAbsorb = smoothstep(0.0, 0.15, facing);

        // Inner color variation: slight hue shift toward white at core
        vec3 innerColor = mix(materiaColor, vec3(1.0), 0.2 * core);

        // Combine
        vec3 finalColor = innerColor * sss * core * breath * sel;
        float finalAlpha = opacity * edgeAbsorb * breath;

        gl_FragColor = vec4(finalColor, finalAlpha);
      }
    `,
    transparent: true,
    depthWrite: false,
    side: THREE.FrontSide,
  });
}

// --- Materia Halo Sprite (bd-1038x) ---
// Soft radial gradient billboard behind each node (replaces Fresnel shell).
// Works with bloom pass for natural light bleed.
/**
 * Create a soft radial gradient halo canvas texture.
 * @param {number} [size=64] - Canvas texture size in pixels
 * @returns {THREE.CanvasTexture}
 */
export function createMateriaHaloTexture(size = 64) {
  const canvas = document.createElement('canvas');
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext('2d');
  const cx = size / 2;
  const gradient = ctx.createRadialGradient(cx, cx, 0, cx, cx, cx);
  gradient.addColorStop(0, 'rgba(255,255,255,0.6)');
  gradient.addColorStop(0.3, 'rgba(255,255,255,0.15)');
  gradient.addColorStop(0.7, 'rgba(255,255,255,0.03)');
  gradient.addColorStop(1.0, 'rgba(255,255,255,0.0)');
  ctx.fillStyle = gradient;
  ctx.fillRect(0, 0, size, size);
  const texture = new THREE.CanvasTexture(canvas);
  texture.needsUpdate = true;
  return texture;
}

// --- GPU Particle Pool (bd-1038x) ---
// Pre-allocated particle system for all visual effects.
// Single draw call via THREE.Points. Particles managed via life attribute.
/**
 * Create a pre-allocated GPU particle pool for visual effects.
 * @param {number} [maxParticles=2000] - Maximum number of concurrent particles
 * @returns {ParticlePool}
 */
export function createParticlePool(maxParticles = 2000) {
  // Per-particle attributes: position(3), velocity(3), color(3), life(1), maxLife(1), size(1)
  const positions = new Float32Array(maxParticles * 3);
  const velocities = new Float32Array(maxParticles * 3);
  const colors = new Float32Array(maxParticles * 3);
  const lives = new Float32Array(maxParticles); // current life (0 = dead)
  const maxLives = new Float32Array(maxParticles); // initial life (for fade calc)
  const sizes = new Float32Array(maxParticles);

  const geometry = new THREE.BufferGeometry();
  geometry.setAttribute('position', new THREE.BufferAttribute(positions, 3));
  geometry.setAttribute('aVelocity', new THREE.BufferAttribute(velocities, 3));
  geometry.setAttribute('aColor', new THREE.BufferAttribute(colors, 3));
  geometry.setAttribute('aLife', new THREE.BufferAttribute(lives, 1));
  geometry.setAttribute('aMaxLife', new THREE.BufferAttribute(maxLives, 1));
  geometry.setAttribute('aSize', new THREE.BufferAttribute(sizes, 1));

  const material = new THREE.ShaderMaterial({
    uniforms: {
      time: { value: 0 },
      dt: { value: 0 },
      pointTexture: { value: null }, // set to halo texture for soft circles
    },
    vertexShader: `
      attribute vec3 aVelocity;
      attribute vec3 aColor;
      attribute float aLife;
      attribute float aMaxLife;
      attribute float aSize;
      varying vec3 vColor;
      varying float vAlpha;
      void main() {
        vColor = aColor;
        // Fade out over last 40% of lifetime
        float progress = 1.0 - (aLife / max(aMaxLife, 0.001));
        vAlpha = aLife > 0.0 ? smoothstep(1.0, 0.6, progress) : 0.0;
        vec4 mvPos = modelViewMatrix * vec4(position, 1.0);
        gl_PointSize = aLife > 0.0 ? aSize * (200.0 / -mvPos.z) : 0.0;
        gl_Position = projectionMatrix * mvPos;
      }
    `,
    fragmentShader: `
      uniform sampler2D pointTexture;
      varying vec3 vColor;
      varying float vAlpha;
      void main() {
        if (vAlpha < 0.001) discard;
        // Soft circular falloff
        float d = length(gl_PointCoord - vec2(0.5));
        float a = smoothstep(0.5, 0.1, d);
        gl_FragColor = vec4(vColor, a * vAlpha);
      }
    `,
    transparent: true,
    depthWrite: false,
    blending: THREE.AdditiveBlending,
  });

  const points = new THREE.Points(geometry, material);
  points.frustumCulled = false; // particles spread everywhere
  let nextIdx = 0;
  let lastTime = 0;

  return {
    mesh: points,
    // Emit particles at a position with given properties
    emit(pos, color, count, { velocity = [0, 2, 0], spread = 1.0, lifetime = 1.5, size = 2.0 } = {}) {
      const c = new THREE.Color(color);
      for (let i = 0; i < count; i++) {
        const idx = nextIdx % maxParticles;
        nextIdx++;
        positions[idx * 3] = pos.x + (Math.random() - 0.5) * spread;
        positions[idx * 3 + 1] = pos.y + (Math.random() - 0.5) * spread;
        positions[idx * 3 + 2] = pos.z + (Math.random() - 0.5) * spread;
        velocities[idx * 3] = velocity[0] + (Math.random() - 0.5) * spread * 2;
        velocities[idx * 3 + 1] = velocity[1] + (Math.random() - 0.5) * spread * 2;
        velocities[idx * 3 + 2] = velocity[2] + (Math.random() - 0.5) * spread * 2;
        colors[idx * 3] = c.r;
        colors[idx * 3 + 1] = c.g;
        colors[idx * 3 + 2] = c.b;
        lives[idx] = lifetime;
        maxLives[idx] = lifetime;
        sizes[idx] = size * (0.5 + Math.random());
      }
      geometry.attributes.position.needsUpdate = true;
      geometry.attributes.aVelocity.needsUpdate = true;
      geometry.attributes.aColor.needsUpdate = true;
      geometry.attributes.aLife.needsUpdate = true;
      geometry.attributes.aMaxLife.needsUpdate = true;
      geometry.attributes.aSize.needsUpdate = true;
    },
    // Call every frame with current time
    update(t) {
      const dt = lastTime > 0 ? Math.min(t - lastTime, 0.1) : 0.016;
      lastTime = t;
      material.uniforms.time.value = t;
      material.uniforms.dt.value = dt;
      let changed = false;
      for (let i = 0; i < maxParticles; i++) {
        if (lives[i] <= 0) continue;
        lives[i] -= dt;
        if (lives[i] <= 0) {
          lives[i] = 0;
          sizes[i] = 0;
          changed = true;
          continue;
        }
        // Integrate velocity (CPU-side for simplicity; GPU upgrade later)
        positions[i * 3] += velocities[i * 3] * dt;
        positions[i * 3 + 1] += velocities[i * 3 + 1] * dt;
        positions[i * 3 + 2] += velocities[i * 3 + 2] * dt;
        // Damping
        velocities[i * 3] *= 0.98;
        velocities[i * 3 + 1] *= 0.98;
        velocities[i * 3 + 2] *= 0.98;
        changed = true;
      }
      if (changed) {
        geometry.attributes.position.needsUpdate = true;
        geometry.attributes.aLife.needsUpdate = true;
        geometry.attributes.aSize.needsUpdate = true;
      }
    },
    // Active particle count (for diagnostics)
    get activeCount() {
      let n = 0;
      for (let i = 0; i < maxParticles; i++) if (lives[i] > 0) n++;
      return n;
    },
  };
}

// --- Update all shader uniforms ---
// Call this in the animation loop to advance time-based effects.
/**
 * Update all shader time uniforms in the scene.
 * @param {THREE.Scene} scene - The Three.js scene to traverse
 * @param {number} time - Current animation time in seconds
 * @returns {void}
 */
export function updateShaderTime(scene, time) {
  scene.traverse((obj) => {
    if (obj.material && obj.material.uniforms && obj.material.uniforms.time) {
      obj.material.uniforms.time.value = time;
    }
  });
}
