import { create } from '@bufbuild/protobuf';
import { Texture } from 'three';
import { describe, expect, it, vi } from 'vitest';
import type { Sprite } from '../formats/wspr';
import type { WMap } from '../formats/wmap';
import { ChatChannel } from '../gen/wathena/v1/command_pb';
import { EntityKind, type Event, EventSchema, LookSchema } from '../gen/wathena/v1/event_pb';
import type { EffectCache } from '../render/effects';
import { SpriteAsset, type SpriteCache } from '../render/sprites';
import type { Pathfinder } from '../world/pathfinder';
import Online, { WARP_SPRITE, lookOf } from './online';

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

describe('lookOf', () => {
  it('draws no equipment for a bare look', () => {
    expect(lookOf(create(LookSchema, { body: 0, hair: 2, sex: 1 }))).toEqual({
      body: 'sprite/body/novice/m',
      head: 'sprite/head/human/m/2',
      hairPalette: 'palette/hair/human/m/2/0',
    });
  });

  it('draws the weapon type view, shield, headgear and garment', () => {
    const look = lookOf(
      create(LookSchema, {
        body: 1,
        hair: 3,
        sex: 0,
        weapon: 2,
        shield: 4,
        headTop: 17,
        headMid: 12,
        headBottom: 5,
        robe: 9,
      }),
    );
    expect(look).toMatchObject({
      body: 'sprite/body/swordman/f',
      weapon: 'sprite/weapon/swordman/f/2',
      shield: 'sprite/shield/swordman/f/4',
      headgearTop: 'sprite/headgear/f/17',
      headgearMid: 'sprite/headgear/f/12',
      headgearBottom: 'sprite/headgear/f/5',
      garment: 'sprite/garment/swordman/f/9',
    });
  });

  it('draws no weapon for bare hands or an unknown type', () => {
    expect(lookOf(create(LookSchema, { weapon: 0 })).weapon).toBeUndefined();
    expect(lookOf(create(LookSchema, { weapon: 9999 })).weapon).toBeUndefined();
  });
});

/** A sprite of one frame per action, for characters that load. */
function sprite(): SpriteAsset {
  const s: Sprite = {
    pages: [{ kind: 0, width: 1, height: 1, pixels: new Uint8Array([1]) }],
    palette: new Uint8Array(1024),
    images: [{ page: 0, x: 0, y: 0, width: 1, height: 1 }],
    actions: Array.from({ length: 16 }, () => ({
      intervalMs: 100,
      frames: [{ event: -1, anchors: [[0, 0] as [number, number]], layers: [] }],
    })),
    events: [],
  };
  return new SpriteAsset(s);
}

async function settle(): Promise<void> {
  for (let i = 0; i < 5; i++) {
    await new Promise<void>((resolve) => {
      setTimeout(resolve, 0);
    });
  }
}

describe('LookChange', () => {
  it('loads the new look and keeps the player in place', async () => {
    vi.stubGlobal('reportError', () => {});
    const asked: string[] = [];
    const sprites = {
      get: async (name: string) => {
        asked.push(name);
        await Promise.resolve();
        return name.startsWith('sprite/body/') || name.startsWith('sprite/headgear/')
          ? sprite()
          : undefined;
      },
      palette: async () => {
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
          value: { id: 1, kind: EntityKind.PLAYER, x: 4, y: 5, dir: 3, look: { body: 0, sex: 1 } },
        },
      }),
    ]);
    await settle();
    const before = o.me?.character;
    expect(before).toBeDefined();
    o.handle([
      create(EventSchema, {
        body: {
          case: 'lookChange',
          value: { id: 1, look: { body: 0, sex: 1, headTop: 17, weapon: 1 } },
        },
      }),
    ]);
    await settle();
    expect(asked).toContain('sprite/headgear/m/17');
    expect(asked).toContain('sprite/weapon/novice/m/1');
    const after = o.me?.character;
    expect(after).toBeDefined();
    expect(after).not.toBe(before);
    expect(after?.direction).toBe(3);
    expect(o.position(1)?.x).toBe(4.5);
    expect(o.group.children).toContain(after?.group);
    expect(o.group.children).not.toContain(before?.group);
  });
});
