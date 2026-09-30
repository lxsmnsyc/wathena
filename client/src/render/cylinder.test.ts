import { Texture } from 'three';
import { describe, expect, it } from 'vitest';
import { type Cylinder, effects } from '../data/tables';
import { CylinderView, cylinderState } from './cylinder';
import type { TextureSource } from './textures';

const textures: TextureSource = { uniform: () => ({ value: new Texture() }) };

function warpRing(): Cylinder {
  const part = effects.get(321)?.parts[0];
  if (part === undefined || !('cylinder' in part)) {
    throw new Error('no warp portal in effects.yml');
  }
  return part.cylinder;
}

describe('cylinderState', () => {
  it('shrinks the warp ring and raises then lowers it', () => {
    const c = warpRing();
    expect(cylinderState(c, 0)).toEqual({ bottom: 2, top: 3.3, height: 0, alpha: 0 });
    const mid = cylinderState(c, 2000);
    expect(mid.height).toBeCloseTo(1.1);
    expect(mid.bottom).toBeCloseTo(1);
    expect(mid.alpha).toBeCloseTo(0.4);
    // Fading in over the first quarter and out over the last.
    expect(cylinderState(c, 500).alpha).toBeCloseTo(0.2);
    expect(cylinderState(c, 3500).alpha).toBeCloseTo(0.2);
    expect(cylinderState(c, 3000).height).toBeCloseTo(0.55);
  });

  it('grows for animation 4 and keeps its shape for 0', () => {
    const c = { ...warpRing(), animation: 4, fade: false };
    expect(cylinderState(c, 1000).top).toBeCloseTo(3.3 / 4);
    expect(cylinderState({ ...c, animation: 0 }, 1000)).toEqual({
      bottom: 2,
      top: 3.3,
      height: 1.1,
      alpha: 0.4,
    });
  });
});

describe('CylinderView', () => {
  it('starts each copy a second after the one before, and repeats', () => {
    const v = new CylinderView(warpRing(), textures);
    expect(v.group.children).toHaveLength(4);
    v.update(1500);
    expect(v.group.children.map((m) => m.visible)).toEqual([true, true, false, false]);
    v.update(3000);
    expect(v.group.children.every((m) => m.visible)).toBe(true);
    expect(v.done).toBe(false);
  });

  it('ends a part that does not repeat', () => {
    const v = new CylinderView({ ...warpRing(), repeat: false }, textures);
    v.update(6999);
    expect(v.done).toBe(false);
    v.update(1);
    expect(v.done).toBe(true);
    expect(v.group.children.every((m) => !m.visible)).toBe(true);
  });
});
