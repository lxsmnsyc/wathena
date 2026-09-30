import { Mesh, type ShaderMaterial } from 'three';
import { describe, expect, it } from 'vitest';
import type { Sprite } from '../formats/wspr';
import { SpriteAsset, SpriteView } from './sprites';

/** A sprite with one image and 16 actions whose frames place it at x = action. */
function sprite(): Sprite {
  return {
    pages: [{ kind: 0, width: 2, height: 2, pixels: new Uint8Array([1, 1, 1, 1]) }],
    palette: new Uint8Array(1024),
    images: [{ page: 0, x: 0, y: 0, width: 2, height: 2 }],
    actions: Array.from({ length: 16 }, (_, a) => ({
      intervalMs: 100,
      frames: [0, 1].map((f) => ({
        event: -1,
        anchors: [],
        layers: [
          {
            image: 0,
            x: a,
            y: f,
            mirror: false,
            color: [255, 255, 255, 255] as [number, number, number, number],
            scaleX: 1,
            scaleY: 1,
            rotation: 0,
          },
        ],
      })),
    })),
    events: [],
  };
}

function shown(v: SpriteView): [number, number] {
  const mesh = v.group.children.find((c) => c instanceof Mesh && c.visible) as Mesh;
  const offset = (mesh.material as ShaderMaterial).uniforms.offset?.value as {
    x: number;
    y: number;
  };
  return [offset.x, offset.y];
}

describe('SpriteView', () => {
  it('picks the action for the facing and the camera', () => {
    const v = new SpriteView(new SpriteAsset(sprite()));
    v.direction = 2;
    v.update(0, 0);
    expect(shown(v)).toEqual([2, 0]);
    // A camera turned three eighths shows the facing three steps on.
    v.update(0, 3);
    expect(shown(v)).toEqual([5, 0]);
    v.update(0, -3);
    expect(shown(v)).toEqual([7, 0]);
    v.play(1);
    v.update(0, 0);
    expect(shown(v)).toEqual([10, 0]);
  });

  it('advances frames by the action interval', () => {
    const v = new SpriteView(new SpriteAsset(sprite()));
    v.update(99, 0);
    expect(shown(v)).toEqual([0, 0]);
    v.update(1, 0);
    expect(shown(v)).toEqual([0, 1]);
    v.update(100, 0);
    expect(shown(v)).toEqual([0, 0]);
  });
});
