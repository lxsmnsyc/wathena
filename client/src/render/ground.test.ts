import { Mesh, Texture } from 'three';
import { describe, expect, it } from 'vitest';
import type { WMap } from '../formats/wmap';
import buildGround from './ground';
import { defaultLight } from './light';
import type { TextureSource } from './textures';

const textures: TextureSource = { uniform: () => ({ value: new Texture() }) };

/** A 2 × 1 ground with one top tile per surface and one east wall. */
function ground(): WMap {
  return {
    width: 4,
    height: 2,
    cellFlags: new Uint8Array(8),
    cellHeights: new Float32Array(32),
    ground: {
      width: 2,
      height: 1,
      textures: ['texture/a', 'texture/b'],
      tiles: {
        count: 2,
        u: new Float32Array([0, 1, 0, 1, 0, 1, 0, 1]),
        v: new Float32Array([0, 0, 1, 1, 0, 0, 1, 1]),
        texture: new Uint16Array([0, 1]),
        lightmap: new Uint32Array([0, 1]),
        color: new Uint8Array(8).fill(255),
      },
      surfaceHeights: new Float32Array([0, 0, 0, 0, 1, 1, 1, 1]),
      top: new Int32Array([0, 0]),
      north: new Int32Array([-1, -1]),
      east: new Int32Array([1, -1]),
      lightmap: 'lightmap/test',
      lightmapCount: 2,
      lightmapColumns: 2,
    },
    light: {
      longitude: 0,
      latitude: 0,
      diffuse: [1, 1, 1],
      ambient: [0, 0, 0],
      opacity: 1,
      direction: [0, 1, 0],
    },
    water: undefined,
    objects: [],
    sounds: [],
    effects: [],
    emitters: [],
    deps: [],
  };
}

describe('buildGround', () => {
  it('builds one mesh per texture', () => {
    const g = buildGround(ground(), textures, defaultLight());
    const meshes = g.children.filter((c) => c instanceof Mesh);
    expect(meshes).toHaveLength(2);
    const [tops, walls] = meshes;
    // Two top tiles use texture a, and one wall uses texture b.
    expect(tops?.geometry.getAttribute('position').count).toBe(12);
    expect(walls?.geometry.getAttribute('position').count).toBe(6);
  });

  it('places tiles and faces tops up', () => {
    const g = buildGround(ground(), textures, defaultLight());
    const tops = g.children[0] as Mesh;
    const pos = [...tops.geometry.getAttribute('position').array];
    // The first triangle is corners 0, 1, and 3 of surface (0, 0).
    expect(pos.slice(0, 9)).toEqual([0, 0, -0, 2, 0, -0, 2, 0, -2]);
    const normal = [...tops.geometry.getAttribute('normal').array].slice(0, 3);
    expect(normal[1]).toBeCloseTo(1);
    // The east wall joins surface 0's east edge at height 0 to surface
    // 1's west edge at height 1.
    const walls = g.children[1] as Mesh;
    const wy = [...walls.geometry.getAttribute('position').array].filter((_, i) => i % 3 === 1);
    expect(new Set(wy)).toEqual(new Set([0, 1]));
    // Lightmap block 1 starts 8 texels across a 16 × 8 atlas.
    const uv1 = [...walls.geometry.getAttribute('uv1').array];
    expect(uv1.slice(0, 2)).toEqual([9 / 16, 1 / 8]);
  });
});
