import { create } from '@bufbuild/protobuf';
import { Texture } from 'three';
import { describe, expect, it } from 'vitest';
import type { WMap } from '../formats/wmap';
import { EntityKind, type Event, EventSchema } from '../gen/wathena/v1/event_pb';
import type { EffectCache } from '../render/effects';
import type { SpriteCache } from '../render/sprites';
import type { Pathfinder } from '../world/pathfinder';
import Online, { WARP_SPRITE } from './online';

function map(): WMap {
  const n = 10;
  return {
    width: n,
    height: n,
    cellFlags: new Uint8Array(n * n).fill(3),
    cellHeights: new Float32Array(n * n * 4),
  } as unknown as WMap;
}

function portal(id: number, x: number, y: number): Event {
  return create(EventSchema, {
    body: {
      case: 'appear',
      value: { id, kind: EntityKind.NPC, x, y, look: { body: WARP_SPRITE } },
    },
  });
}

function portals(o: Online): number {
  return o.group.children.filter((c) => c.name === 'portal').length;
}

function online(): Online {
  const o = new Online(1, {} as SpriteCache, {} as Pathfinder);
  o.effects = {
    cache: {} as EffectCache,
    textures: { uniform: () => ({ value: new Texture() }) },
  };
  return o;
}

describe('Online', () => {
  it('shows warp portals as the warp effect', () => {
    const o = online();
    o.setMap(map());
    o.handle([portal(5, 2, 3)]);
    expect(portals(o)).toBe(1);
    const g = o.group.children.find((c) => c.name === 'portal');
    expect(g?.position.x).toBe(2.5);
    expect(g?.position.z).toBe(-3.5);
    // Two cylinders of four copies each.
    expect(g?.children).toHaveLength(2);
    o.update(1500, 0, g!.position.clone());
    o.handle([create(EventSchema, { body: { case: 'vanish', value: { id: 5 } } })]);
    expect(portals(o)).toBe(0);
  });

  it('clears the old map on a map change and waits for the new one', () => {
    const o = online();
    o.setMap(map());
    o.handle([portal(5, 2, 3)]);
    const changes: [string, number, number][] = [];
    o.onMapChange = (m, x, y) => {
      changes.push([m, x, y]);
    };
    o.handle([
      create(EventSchema, {
        body: { case: 'mapChange', value: { map: 'map/b', x: 3, y: 4, netId: 9 } },
      }),
      portal(5, 7, 7),
    ]);
    expect(changes).toEqual([['map/b', 3, 4]]);
    expect(o.self).toBe(9);
    expect(portals(o)).toBe(0);
    // Events after the change wait for the new map.
    o.handle([portal(6, 1, 1)]);
    expect(portals(o)).toBe(0);
    o.setMap(map());
    expect(portals(o)).toBe(2);
  });
});
