import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import type { WMap } from '../formats/wmap';
import type { Sprite } from '../formats/wspr';
import { GroundItemSchema } from '../gen/wathena/v1/event_pb';
import { SpriteAsset, type SpriteCache } from '../render/sprites';
import GroundItems, { DROP_MS, dropHeight, groundItemLabel, itemPosition } from './groundItems';

describe('itemPosition', () => {
  it('places items inside their cell', () => {
    expect(itemPosition(10, 20, 0, 0)).toEqual([10 + 1 / 32, 20 + 1 / 32]);
    expect(itemPosition(10, 20, 15, 15)).toEqual([10 + 31 / 32, 20 + 31 / 32]);
    expect(itemPosition(10, 20, 8, 4)).toEqual([10.53125, 20.28125]);
  });

  it('keeps out-of-range places in the cell', () => {
    const [x, y] = itemPosition(3, 4, 99, 16);
    expect(x).toBeLessThan(4);
    expect(y).toBeLessThan(5);
  });
});

describe('dropHeight', () => {
  it('starts and ends on the ground', () => {
    expect(dropHeight(0)).toBe(0);
    expect(dropHeight(DROP_MS)).toBe(0);
    expect(dropHeight(DROP_MS + 1000)).toBe(0);
    expect(dropHeight(-5)).toBe(0);
  });

  it('jumps, lands, and bounces lower', () => {
    const samples = Array.from({ length: DROP_MS }, (_, i) => dropHeight(i));
    const peak = Math.max(...samples);
    const peakAt = samples.indexOf(peak);
    expect(peak).toBeCloseTo(1.2, 2);
    // After the jump it touches the ground, then rises again, lower.
    const landing = samples.findIndex((h, i) => i > peakAt && h < 0.01);
    expect(landing).toBeGreaterThan(peakAt);
    const bounce = Math.max(...samples.slice(landing + 5));
    expect(bounce).toBeGreaterThan(0.1);
    expect(bounce).toBeLessThan(peak / 2);
    for (const h of samples) {
      expect(h).toBeGreaterThanOrEqual(0);
    }
  });
});

/** A flat 20 × 20 map. */
const map = {
  width: 20,
  height: 20,
  cellFlags: new Uint8Array(400).fill(3),
  cellHeights: new Float32Array(1600),
} as unknown as WMap;

const sprite: Sprite = {
  pages: [{ kind: 0, width: 1, height: 1, pixels: new Uint8Array([1]) }],
  palette: new Uint8Array(1024),
  images: [{ page: 0, x: 0, y: 0, width: 1, height: 1 }],
  actions: [{ intervalMs: 100, frames: [{ event: -1, anchors: [], layers: [] }] }],
  events: [],
};

/** Waits for loads in the background. */
async function settle(): Promise<void> {
  return new Promise((resolve) => {
    setTimeout(resolve, 0);
  });
}

function items(): { items: GroundItems; asked: string[] } {
  const asked: string[] = [];
  const cache = {
    get: async (name: string) => {
      asked.push(name);
      return Promise.resolve(new SpriteAsset(sprite));
    },
  } as unknown as SpriteCache;
  return { items: new GroundItems(cache), asked };
}

describe('groundItemLabel', () => {
  it('shows the amount of a stack', () => {
    expect(groundItemLabel('빨간포션', 1)).toBe('빨간포션');
    expect(groundItemLabel('빨간포션', 5)).toBe('빨간포션: 5 ea.');
  });
});

describe('GroundItems', () => {
  it('draws an item sprite by item ID inside its cell', async () => {
    const { items: g, asked } = items();
    g.add(7, map, 5, 6, create(GroundItemSchema, { itemId: 501, amount: 1, subX: 8, subY: 8 }));
    await settle();
    expect(asked).toEqual(['sprite/item/501']);
    expect(g.cell(7)).toEqual([5, 6]);
    const shown = [...g.shown()];
    expect(shown.map(([id, p]) => [id, p.x, p.y, p.z])).toEqual([[7, 5.53125, 0, -6.53125]]);
    expect(g.info(7)).toMatchObject({ itemId: 501, amount: 1 });
    g.remove(7);
    expect(g.has(7)).toBe(false);
    expect(g.group.children).toHaveLength(0);
  });

  it('plays the drop once', async () => {
    const { items: g } = items();
    g.add(1, map, 5, 6, create(GroundItemSchema, { itemId: 501, dropped: true }));
    await settle();
    const position = () => [...g.shown()][0]![1].y;
    g.update(200);
    expect(position()).toBeCloseTo(dropHeight(200));
    g.update(DROP_MS);
    expect(position()).toBe(0);
  });
});
