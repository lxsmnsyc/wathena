import { Texture, type Vector2 } from 'three';
import { describe, expect, it } from 'vitest';
import type { Effect } from '../formats/weff';
import { EffectView } from './effects';
import type { TextureSource } from './textures';

const textures: TextureSource = { uniform: () => ({ value: new Texture() }) };

/** One layer over three frames at 10 fps: at x = 0, then 1, then hidden. */
function effect(): Effect {
  const n = 3;
  const e: Effect = {
    fps: 10,
    frames: n,
    textures: ['texture/effect/a'],
    layers: 1,
    visible: new Uint8Array([1, 1, 0]),
    texture: new Uint16Array(n),
    srcBlend: new Uint8Array([5, 5, 0]),
    dstBlend: new Uint8Array([2, 2, 0]),
    color: new Uint8Array([255, 255, 255, 0, 255, 255, 255, 200, 0, 0, 0, 0]),
    offset: new Float32Array([0, 0, 1, 2, 0, 0]),
    angle: new Float32Array([0, 90, 0]),
    corners: new Float32Array(n * 8),
  };
  return e;
}

function state(v: EffectView) {
  const mesh = v.group.children[0]!;
  const u = (mesh as unknown as { material: { uniforms: Record<string, { value: unknown }> } })
    .material.uniforms;
  const o = u.offset!.value as Vector2;
  return { visible: mesh.visible, x: o.x, y: o.y, angle: u.angle!.value as number };
}

describe('EffectView', () => {
  it('interpolates between frames', () => {
    const v = new EffectView(effect(), textures);
    v.update(50);
    const s = state(v);
    expect(s.visible).toBe(true);
    expect(s.x).toBeCloseTo(0.5);
    expect(s.y).toBeCloseTo(1);
    expect(s.angle).toBeCloseTo(Math.PI / 4);
  });

  it('holds a frame whose next frame is hidden, then hides', () => {
    const v = new EffectView(effect(), textures);
    v.update(150);
    expect(state(v)).toMatchObject({ visible: true, x: 1, y: 2 });
    v.update(100);
    expect(state(v).visible).toBe(false);
  });

  it('stops after the last frame unless it loops', () => {
    const v = new EffectView(effect(), textures);
    v.loop = false;
    v.update(350);
    expect(v.done).toBe(true);
    expect(state(v).visible).toBe(false);
  });
});
