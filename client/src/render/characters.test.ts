import { type Mesh, type ShaderMaterial, Vector3 } from 'three';
import { describe, expect, it } from 'vitest';
import type { Sprite } from '../formats/wspr';
import { Character, type SpriteSource } from './characters';
import { SpriteAsset } from './sprites';

/** A sprite whose frames all have one layer at (0, 0) and one anchor. */
function part(anchor: [number, number]): SpriteAsset {
  const s: Sprite = {
    pages: [{ kind: 0, width: 1, height: 1, pixels: new Uint8Array([1]) }],
    palette: new Uint8Array(1024),
    images: [{ page: 0, x: 0, y: 0, width: 1, height: 1 }],
    actions: Array.from({ length: 16 }, () => ({
      intervalMs: 100,
      frames: [
        {
          event: -1,
          anchors: [anchor],
          layers: [
            {
              image: 0,
              x: 0,
              y: 0,
              mirror: false,
              color: [255, 255, 255, 255] as [number, number, number, number],
              scaleX: 1,
              scaleY: 1,
              rotation: 0,
            },
          ],
        },
      ],
    })),
    events: [],
  };
  return new SpriteAsset(s);
}

const assets: Record<string, SpriteAsset> = {
  body: part([10, -40]),
  head: part([0, -5]),
  shield: part([3, 3]),
  garment: part([0, 0]),
};
const source: SpriteSource = {
  get: async (name) => Promise.resolve(assets[name]),
  palette: async () => Promise.resolve(undefined),
};

function offsetOf(c: Character, index: number): [number, number] {
  const mesh = c.group.children[index]?.children[0] as Mesh;
  const o = (mesh.material as ShaderMaterial).uniforms.offset?.value as { x: number; y: number };
  return [o.x, o.y];
}

describe('Character', () => {
  it('attaches the head at the body anchor', async () => {
    const c = (await Character.load(source, { body: 'body', head: 'head', shield: 'shield' }))!;
    c.update(0, 0, new Vector3(0, 0, 1));
    // Children are body, head, then shield. The shield has no anchor to
    // attach, so it stays at the origin.
    expect(offsetOf(c, 0)).toEqual([0, 0]);
    expect(offsetOf(c, 1)).toEqual([10, -35]);
    expect(offsetOf(c, 2)).toEqual([0, 0]);
  });

  it('puts the shield behind the body when facing away', async () => {
    const c = (await Character.load(source, { body: 'body', head: 'head', shield: 'shield' }))!;
    const toward = new Vector3(0, 0, 1);
    const [body, , shield] = c.group.children;
    c.update(0, 0, toward);
    expect(shield!.position.z).toBeGreaterThan(body!.position.z);
    c.direction = 4;
    c.update(0, 0, toward);
    expect(shield!.position.z).toBeLessThan(body!.position.z);
  });

  it('covers the body with the garment only when facing away', async () => {
    const c = (await Character.load(source, { body: 'body', garment: 'garment' }))!;
    const toward = new Vector3(0, 0, 1);
    const [body, garment] = c.group.children;
    c.update(0, 0, toward);
    expect(garment!.position.z).toBeLessThan(body!.position.z);
    c.direction = 2;
    c.update(0, 0, toward);
    expect(garment!.position.z).toBeLessThan(body!.position.z);
    c.direction = 3;
    c.update(0, 0, toward);
    expect(garment!.position.z).toBeGreaterThan(body!.position.z);
  });
});
