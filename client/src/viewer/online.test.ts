import { create } from '@bufbuild/protobuf';
import { Texture } from 'three';
import { describe, expect, it, vi } from 'vitest';
import type { WMap } from '../formats/wmap';
import { ChatChannel } from '../gen/wathena/v1/command_pb';
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

  it('loads an NPC sprite by its sprite ID', () => {
    // Node has no reportError, which the browser has.
    vi.stubGlobal('reportError', () => {});
    const asked: string[] = [];
    const sprites = {
      get: async (name: string) => {
        asked.push(name);
        await Promise.resolve();
        return undefined;
      },
    } as unknown as SpriteCache;
    const o = new Online(1, sprites, {} as Pathfinder);
    o.setMap(map());
    o.handle([
      create(EventSchema, {
        body: {
          case: 'appear',
          value: { id: 7, kind: EntityKind.NPC, x: 1, y: 1, look: { body: 112 } },
        },
      }),
      create(EventSchema, {
        body: {
          case: 'appear',
          value: { id: 8, kind: EntityKind.NPC, x: 2, y: 2, look: { body: 999999 } },
        },
      }),
    ]);
    expect(asked).toEqual(['sprite/npc/112', 'sprite/npc/999999']);
  });

  it('passes server text on, and local chat to onChat', () => {
    const o = online();
    o.setMap(map());
    const server: string[] = [];
    const local: string[] = [];
    o.onServerText = (text) => {
      server.push(text);
    };
    o.onChat = (_from, name, text) => {
      local.push(`${name}: ${text}`);
    };
    o.handle([
      create(EventSchema, {
        body: { case: 'chat', value: { channel: ChatChannel.SYSTEM, text: 'Welcome' } },
      }),
      create(EventSchema, {
        body: {
          case: 'chat',
          value: { channel: ChatChannel.LOCAL, from: 3, fromName: 'Bob', text: 'hi' },
        },
      }),
    ]);
    expect(server).toEqual(['Welcome']);
    expect(local).toEqual(['Bob: hi']);
  });
});
