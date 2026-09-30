import { Texture } from 'three';
import { describe, expect, it } from 'vitest';
import { DEPTH_TEST, type Emitter } from '../formats/wmap';
import ParticleEmitter from './particles';
import type { TextureSource } from './textures';

const textures: TextureSource = { uniform: () => ({ value: new Texture() }) };

function emitter(): Emitter {
  return {
    position: [10, 1, -10],
    texture: 'texture/effect/smoke1',
    srcBlend: 5,
    dstBlend: 2,
    color: [255, 255, 255, 255],
    dir1: [0, 1, 0],
    dir2: [0, 1, 0],
    gravity: [0, 0, 0],
    radius: [0, 0, 0],
    life: [4, 4],
    rate: [2, 2],
    size: [1, 1],
    speed: 1,
    maxCount: 5,
    flags: DEPTH_TEST,
    scale: [1, 1],
    rotate: [0, 0, 0],
  };
}

describe('ParticleEmitter', () => {
  it('starts full and keeps at most maxCount particles', () => {
    const p = new ParticleEmitter(emitter(), textures, () => 0.5);
    const g = p.mesh.geometry as unknown as { instanceCount: number };
    // Two per second over a four-second life is eight, capped at five.
    expect(g.instanceCount).toBe(5);
    p.update(10);
    expect(g.instanceCount).toBeLessThanOrEqual(5);
  });

  it('moves particles along their direction and fades them', () => {
    const p = new ParticleEmitter(emitter(), textures, () => 0.5);
    const g = p.mesh.geometry;
    const count = (g as unknown as { instanceCount: number }).instanceCount;
    const offset = g.getAttribute('offset');
    const fade = g.getAttribute('fade');
    let highest = 0;
    for (let i = 0; i < count; i++) {
      // Particles only rise, at one cell per second, for at most 4 s.
      expect(offset.getX(i)).toBe(0);
      expect(offset.getY(i)).toBeGreaterThanOrEqual(0);
      expect(offset.getY(i)).toBeLessThanOrEqual(4);
      expect(fade.getX(i)).toBeGreaterThanOrEqual(0);
      expect(fade.getX(i)).toBeLessThanOrEqual(1);
      highest = Math.max(highest, offset.getY(i));
    }
    expect(highest).toBeGreaterThan(1);
  });
});
