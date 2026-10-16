import { create } from '@bufbuild/protobuf';
import { describe, expect, it } from 'vitest';
import { WorldMapSchema } from '../gen/wathena/v1/event_pb';
import {
  CELL,
  type WorldMapData,
  type WorldMapNodeView,
  bandText,
  layoutWorldMap,
  mapDisplayName,
  nodeLabel,
  worldMapOf,
  worldMapSummary,
} from './worldMap';

function node(id: string, values: Partial<WorldMapNodeView> = {}): WorldMapNodeView {
  return {
    id,
    kind: 'field',
    name: '',
    x: 0,
    y: 0,
    floor: 0,
    visited: false,
    minLevel: 0,
    maxLevel: 0,
    ...values,
  };
}

/** A seat field with a town, a home map, a dungeon of 3 floors, and a field north of it. */
const WORLD: WorldMapData = {
  maps: [
    node('gen:field:0:0', { name: 'Valheim Fields', visited: true, minLevel: 1, maxLevel: 10 }),
    node('gen:town:t1', {
      kind: 'town',
      name: 'Valheim',
      visited: true,
      minLevel: 1,
      maxLevel: 10,
    }),
    node('gen:home:haven', {
      kind: 'home',
      name: 'Haven',
      visited: true,
      minLevel: 1,
      maxLevel: 5,
    }),
    node('gen:dungeon:crypt:1', {
      kind: 'dungeon',
      floor: 1,
      name: 'Crypt of Valheim F1',
      visited: true,
      minLevel: 12,
      maxLevel: 18,
    }),
    node('gen:dungeon:crypt:2', {
      kind: 'dungeon',
      floor: 2,
      name: 'Crypt of Valheim F2',
      visited: true,
      minLevel: 15,
      maxLevel: 21,
    }),
    node('gen:dungeon:crypt:3', { kind: 'dungeon', floor: 3 }),
    node('gen:field:0:1', { y: 1 }),
  ],
  links: [
    { a: 'gen:field:0:0', b: 'gen:town:t1' },
    { a: 'gen:field:0:0', b: 'gen:field:0:1' },
    { a: 'gen:dungeon:crypt:1', b: 'gen:dungeon:crypt:2' },
    { a: 'gen:dungeon:crypt:2', b: 'gen:dungeon:crypt:3' },
    { a: 'gen:field:0:0', b: 'gen:field:9:9' },
  ],
};

describe('worldMapOf', () => {
  it('copies the event into plain data', () => {
    const m = create(WorldMapSchema, {
      maps: [
        {
          id: 'gen:town:t1',
          kind: 'town',
          name: 'Valheim',
          x: -2,
          y: 3,
          visited: true,
          minLevel: 1,
          maxLevel: 10,
        },
      ],
      links: [{ a: 'gen:town:t1', b: 'gen:field:-2:3' }],
    });
    const data = worldMapOf(m);
    expect(data).toEqual({
      maps: [
        {
          id: 'gen:town:t1',
          kind: 'town',
          name: 'Valheim',
          x: -2,
          y: 3,
          floor: 0,
          visited: true,
          minLevel: 1,
          maxLevel: 10,
        },
      ],
      links: [{ a: 'gen:town:t1', b: 'gen:field:-2:3' }],
    });
    // Plain data has no message fields, so a browser check can read it.
    expect(JSON.parse(JSON.stringify(data))).toEqual(data);
  });
});

describe('labels', () => {
  it('writes the name and the level band of visited maps', () => {
    expect(bandText(1, 10)).toBe('1-10');
    expect(bandText(5, 5)).toBe('5');
    expect(bandText(0, 0)).toBe('');
    expect(nodeLabel(WORLD.maps[1]!)).toBe('Valheim 1-10');
    expect(nodeLabel(node('gen:field:1:1', { visited: true, name: 'East Fields' }))).toBe(
      'East Fields',
    );
    // A visited map without a name shows its ID.
    expect(nodeLabel(node('gen:field:1:1', { visited: true }))).toBe('gen:field:1:1');
    expect(nodeLabel(node('gen:field:1:1'))).toBe('');
  });

  it('picks the current map display name', () => {
    expect(mapDisplayName('gen:town:t1', 'New Valheim', WORLD)).toBe('New Valheim');
    expect(mapDisplayName('gen:town:t1', undefined, WORLD)).toBe('Valheim');
    expect(mapDisplayName('gen:field:0:1', '', WORLD)).toBe('gen:field:0:1');
    expect(mapDisplayName('prontera', undefined, undefined)).toBe('prontera');
  });

  it('counts the maps found and visited', () => {
    expect(worldMapSummary(WORLD)).toBe('Visited 5 of the 7 maps found.');
  });
});

describe('layoutWorldMap', () => {
  const layout = layoutWorldMap(WORLD, 'gen:town:t1');
  const at = (id: string) => layout.nodes.find((n) => n.id === id)!;

  it('puts fields at their grid cells with north up', () => {
    expect(at('gen:field:0:0')).toMatchObject({ cx: 0, cy: 0 });
    expect(at('gen:field:0:1')).toMatchObject({ cx: 0, cy: -CELL });
  });

  it('keeps towns, the home map and floors inside the cell without overlap', () => {
    const inCell = layout.nodes.filter((n) => n.id !== 'gen:field:0:1');
    for (const n of inCell) {
      expect(Math.abs(n.cx) + n.r).toBeLessThan(CELL / 2);
      expect(Math.abs(n.cy) + n.r).toBeLessThan(CELL / 2);
    }
    for (const [i, p] of inCell.entries()) {
      for (const q of inCell.slice(i + 1)) {
        expect(Math.hypot(p.cx - q.cx, p.cy - q.cy)).toBeGreaterThan(p.r + q.r);
      }
    }
    // Towns and the home map sit above the field, floors below it, in
    // order and in one column.
    expect(at('gen:town:t1').cy).toBeLessThan(0);
    expect(at('gen:home:haven').cy).toBeLessThan(0);
    const floors = [1, 2, 3].map((f) => at(`gen:dungeon:crypt:${f}`));
    expect(new Set(floors.map((f) => f.cx)).size).toBe(1);
    expect(floors[0]!.cy).toBeGreaterThan(0);
    expect(floors[1]!.cy).toBeGreaterThan(floors[0]!.cy);
    expect(floors[2]!.cy).toBeGreaterThan(floors[1]!.cy);
  });

  it('labels visited maps and marks the others with "?"', () => {
    expect(at('gen:town:t1')).toMatchObject({ label: 'Valheim 1-10', text: 'Valheim 1-10' });
    expect(at('gen:dungeon:crypt:1').text).toBe('Crypt of Valheim F1 12-18');
    expect(at('gen:dungeon:crypt:2')).toMatchObject({
      label: 'Crypt of Valheim F2 15-21',
      text: 'F2 15-21',
      anchor: 'start',
    });
    expect(at('gen:dungeon:crypt:3')).toMatchObject({ label: '', text: '?', visited: false });
    expect(at('gen:field:0:1')).toMatchObject({ label: '', text: '?' });
  });

  it('marks the current map', () => {
    expect(layout.nodes.filter((n) => n.current).map((n) => n.id)).toEqual(['gen:town:t1']);
    expect(layoutWorldMap(WORLD).nodes.some((n) => n.current)).toBe(false);
  });

  it('draws links between known maps only', () => {
    expect(layout.links).toHaveLength(4);
    expect(layout.links[1]).toEqual({
      a: 'gen:field:0:0',
      b: 'gen:field:0:1',
      x1: 0,
      y1: 0,
      x2: 0,
      y2: -CELL,
    });
  });

  it('bounds every map and label', () => {
    const b = layout.bounds;
    for (const n of layout.nodes) {
      expect(n.cx - n.r).toBeGreaterThanOrEqual(b.x);
      expect(n.cx + n.r).toBeLessThanOrEqual(b.x + b.width);
      expect(n.cy - n.r).toBeGreaterThanOrEqual(b.y);
      expect(n.cy + n.r).toBeLessThanOrEqual(b.y + b.height);
    }
    // The town's label is centered under it.
    const t = at('gen:town:t1');
    expect(t.anchor).toBe('middle');
    expect(t.ty).toBeGreaterThan(t.cy + t.r);
    expect(layoutWorldMap({ maps: [], links: [] }).bounds).toEqual({
      x: 0,
      y: 0,
      width: 1,
      height: 1,
    });
  });

  it('spreads the towns and dungeons that share a cell', () => {
    const shared = layoutWorldMap({
      maps: [
        node('gen:town:t1', { kind: 'town' }),
        node('gen:town:t2', { kind: 'town' }),
        node('gen:home:haven', { kind: 'home' }),
        node('gen:dungeon:a:1', { kind: 'dungeon', floor: 1 }),
        node('gen:dungeon:b:1', { kind: 'dungeon', floor: 1 }),
      ],
      links: [],
    });
    const xs = shared.nodes.map((n) => n.cx);
    expect(xs.slice(0, 3)).toEqual([-48, 0, 48]);
    expect(xs.slice(3)).toEqual([-48, 48]);
  });
});
