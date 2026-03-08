import { describe, it, expect, vi, beforeEach } from 'vitest';
import * as THREE from 'three';
import {
  createFresnelMaterial,
  createPulseRingMaterial,
  createStarField,
  createSelectionRingMaterial,
  createMateriaMaterial,
  createParticlePool,
  updateShaderTime,
} from '../../src/shaders.js';

describe('createFresnelMaterial', () => {
  it('returns a ShaderMaterial', () => {
    const mat = createFresnelMaterial('#ff0000');
    expect(mat).toBeInstanceOf(THREE.ShaderMaterial);
  });

  it('sets uniforms from color and options', () => {
    const mat = createFresnelMaterial('#2d8a4e', { opacity: 0.6, power: 3.0 });
    expect(mat.uniforms.glowColor.value).toBeInstanceOf(THREE.Color);
    expect(mat.uniforms.opacity.value).toBe(0.6);
    expect(mat.uniforms.power.value).toBe(3.0);
  });

  it('uses default opacity and power', () => {
    const mat = createFresnelMaterial('#ff0000');
    expect(mat.uniforms.opacity.value).toBe(0.4);
    expect(mat.uniforms.power.value).toBe(2.0);
  });

  it('is transparent and no depth write', () => {
    const mat = createFresnelMaterial('#ff0000');
    expect(mat.transparent).toBe(true);
    expect(mat.depthWrite).toBe(false);
  });

  it('has vertex and fragment shaders', () => {
    const mat = createFresnelMaterial('#ff0000');
    expect(mat.vertexShader).toContain('vNormal');
    expect(mat.fragmentShader).toContain('fresnel');
  });
});

describe('createPulseRingMaterial', () => {
  it('returns a ShaderMaterial', () => {
    const mat = createPulseRingMaterial('#d4a017');
    expect(mat).toBeInstanceOf(THREE.ShaderMaterial);
  });

  it('has time and pulseCycle uniforms', () => {
    const mat = createPulseRingMaterial('#d4a017');
    expect(mat.uniforms.time.value).toBe(0);
    expect(mat.uniforms.pulseCycle.value).toBe(4.0);
  });

  it('uses DoubleSide', () => {
    const mat = createPulseRingMaterial('#d4a017');
    expect(mat.side).toBe(THREE.DoubleSide);
  });
});

describe('createStarField', () => {
  it('returns THREE.Points', () => {
    const stars = createStarField(100, 300);
    expect(stars).toBeInstanceOf(THREE.Points);
  });

  it('creates correct number of particles', () => {
    const count = 50;
    const stars = createStarField(count, 300);
    const posAttr = stars.geometry.getAttribute('position');
    expect(posAttr.count).toBe(count);
  });

  it('sets isStarField userData', () => {
    const stars = createStarField();
    expect(stars.userData.isStarField).toBe(true);
  });

  it('has size and alpha attributes', () => {
    const stars = createStarField(10);
    expect(stars.geometry.getAttribute('size')).toBeTruthy();
    expect(stars.geometry.getAttribute('alpha')).toBeTruthy();
  });

  it('uses additive blending', () => {
    const stars = createStarField(10);
    expect(stars.material.blending).toBe(THREE.AdditiveBlending);
  });
});

describe('createSelectionRingMaterial', () => {
  it('returns ShaderMaterial with visible uniform', () => {
    const mat = createSelectionRingMaterial();
    expect(mat).toBeInstanceOf(THREE.ShaderMaterial);
    expect(mat.uniforms.visible.value).toBe(0.0);
  });

  it('has time uniform', () => {
    const mat = createSelectionRingMaterial();
    expect(mat.uniforms.time.value).toBe(0);
  });

  it('fragment shader uses visible for discard', () => {
    const mat = createSelectionRingMaterial();
    expect(mat.fragmentShader).toContain('discard');
  });
});

describe('createMateriaMaterial', () => {
  it('returns a ShaderMaterial', () => {
    const mat = createMateriaMaterial('#8b45a6');
    expect(mat).toBeInstanceOf(THREE.ShaderMaterial);
  });

  it('sets default options', () => {
    const mat = createMateriaMaterial('#ff0000');
    expect(mat.uniforms.opacity.value).toBe(0.85);
    expect(mat.uniforms.coreIntensity.value).toBe(1.4);
    expect(mat.uniforms.breathSpeed.value).toBe(0.0);
  });

  it('accepts custom options', () => {
    const mat = createMateriaMaterial('#ff0000', { opacity: 0.5, coreIntensity: 2.0, breathSpeed: 1.5 });
    expect(mat.uniforms.opacity.value).toBe(0.5);
    expect(mat.uniforms.coreIntensity.value).toBe(2.0);
    expect(mat.uniforms.breathSpeed.value).toBe(1.5);
  });

  it('has selected uniform for selection boost', () => {
    const mat = createMateriaMaterial('#ff0000');
    expect(mat.uniforms.selected.value).toBe(0.0);
  });
});

describe('createParticlePool', () => {
  it('returns object with mesh, emit, update, activeCount', () => {
    const pool = createParticlePool(100);
    expect(pool.mesh).toBeInstanceOf(THREE.Points);
    expect(typeof pool.emit).toBe('function');
    expect(typeof pool.update).toBe('function');
    expect(typeof pool.activeCount).toBe('number');
  });

  it('starts with zero active particles', () => {
    const pool = createParticlePool(100);
    expect(pool.activeCount).toBe(0);
  });

  it('emit increases active count', () => {
    const pool = createParticlePool(100);
    pool.emit({ x: 0, y: 0, z: 0 }, '#ff0000', 5);
    expect(pool.activeCount).toBe(5);
  });

  it('update decreases life over time', () => {
    const pool = createParticlePool(100);
    pool.emit({ x: 0, y: 0, z: 0 }, '#ff0000', 3, { lifetime: 0.5 });
    expect(pool.activeCount).toBe(3);

    // Simulate time passing — dt is clamped to 0.1 per frame, so need many frames
    // lifetime=0.5, need 0.5s of accumulated dt to kill all particles
    for (let t = 0.1; t <= 2.0; t += 0.1) {
      pool.update(t);
    }
    expect(pool.activeCount).toBe(0);
  });

  it('wraps around when exceeding max particles', () => {
    const pool = createParticlePool(5);
    pool.emit({ x: 0, y: 0, z: 0 }, '#ff0000', 5);
    expect(pool.activeCount).toBe(5);
    // Emit 3 more — overwrites oldest 3
    pool.emit({ x: 1, y: 1, z: 1 }, '#00ff00', 3);
    expect(pool.activeCount).toBe(5); // still max 5 slots
  });

  it('has correct geometry attributes', () => {
    const pool = createParticlePool(50);
    const geo = pool.mesh.geometry;
    expect(geo.getAttribute('position').count).toBe(50);
    expect(geo.getAttribute('aVelocity').count).toBe(50);
    expect(geo.getAttribute('aColor').count).toBe(50);
    expect(geo.getAttribute('aLife').count).toBe(50);
    expect(geo.getAttribute('aMaxLife').count).toBe(50);
    expect(geo.getAttribute('aSize').count).toBe(50);
  });
});

describe('updateShaderTime', () => {
  it('updates time uniform on objects with shader materials', () => {
    const scene = new THREE.Scene();
    const mat = new THREE.ShaderMaterial({
      uniforms: { time: { value: 0 } },
      vertexShader: '',
      fragmentShader: '',
    });
    const mesh = new THREE.Mesh(new THREE.BoxGeometry(), mat);
    scene.add(mesh);

    updateShaderTime(scene, 5.0);
    expect(mat.uniforms.time.value).toBe(5.0);
  });

  it('skips objects without time uniform', () => {
    const scene = new THREE.Scene();
    const mat = new THREE.MeshBasicMaterial({ color: 0xff0000 });
    const mesh = new THREE.Mesh(new THREE.BoxGeometry(), mat);
    scene.add(mesh);

    // Should not throw
    updateShaderTime(scene, 3.0);
  });
});
