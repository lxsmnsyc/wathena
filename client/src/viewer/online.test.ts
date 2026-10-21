import { create } from '@bufbuild/protobuf';
import { Texture } from 'three';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { setArt } from '../assets/art';
import type { ClientData } from '../data/clientData';
import type { Sprite } from '../formats/wspr';
import type { WMap } from '../formats/wmap';
import { ChatChannel } from '../gen/wathena/v1/command_pb';
import {
  DigitsKind,
  EntityKind,
  type Event,
  EventSchema,
  LookSchema,
  NpcView,
} from '../gen/wathena/v1/event_pb';
import type { EffectCache } from '../render/effects';
import { SpriteAsset, type SpriteCache } from '../render/sprites';
import type { Pathfinder } from '../world/pathfinder';
import Online, { WARP_SPRITE, lookOf } from './online';

/** Client data with a mod's monster, skill and hat (review decision 40). */
const MOD_DATA: ClientData = {
  version: 2,
  mods: [],
  items: {
    '2000003': {
      name: 'Lantern Hat',
      view_sprite: 'mod/example.lantern/sprite/headgear/Lantern_Hat',
      view: 1000000,
    },
    '2000004': {
      name: 'Lantern Shield',
      view_sprite: 'mod/example.lantern/sprite/shield/Lantern_Shield',
      view: 1000001,
    },
    '2000005': {
      name: 'Lantern Staff',
      view_sprite: 'mod/example.lantern/sprite/weapon/Lantern_Staff',
      view: 1000002,
    },
  },
  monsters: {
    '1002': {
      name: 'Lantern Ghost',
      sounds: {
        attack: 'mod/example.lantern/sound/ghost_attack',
        damage: 'mod/example.lantern/sound/ghost_damage',
        die: 'mod/example.lantern/sound/ghost_die',
      },
    },
  },
  npc_sprites: {},
  jobs: {},
  skills: { '20000': { effects: { hit: ['mod/example.lantern/effect/flare'] } } },
  companions: { '10000': 'mod/example.lantern/sprite/companion/Wisp' },
  mounts: { '10000': 'mod/example.lantern/sprite/mount/Lantern_Strider' },
};

afterEach(() => {
  setArt(undefined);
});

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

  it('changes cells and the paths of MapCells', () => {
    let sets = 0;
    const o = new Online(
      1,
      {} as SpriteCache,
      {
        setMap: () => {
          sets++;
        },
      } as unknown as Pathfinder,
    );
    const m = map();
    m.cellFlags[3 * 10 + 2] = 3 | 4;
    o.setMap(m);
    const pack = (x: number, y: number, flags: number): number => x | (y << 10) | (flags << 20);
    o.handle([
      create(EventSchema, {
        body: {
          case: 'mapCells',
          value: { cells: [pack(2, 3, 0), pack(4, 4, 2), pack(20, 1, 0)] },
        },
      }),
    ]);
    // The water bit stays; walkable and shootable follow the event. A
    // cell outside the map is left out.
    expect(m.cellFlags[3 * 10 + 2]).toBe(4);
    expect(m.cellFlags[4 * 10 + 4]).toBe(2);
    expect(sets).toBe(1);
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

  it('loads a monster or a job view of an NPC, and keeps its title', () => {
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
          value: {
            id: 7,
            kind: EntityKind.NPC,
            x: 1,
            y: 1,
            title: '<Society>',
            look: { body: 1002, npcView: NpcView.MONSTER },
          },
        },
      }),
      create(EventSchema, {
        body: {
          case: 'appear',
          value: {
            id: 8,
            kind: EntityKind.NPC,
            x: 2,
            y: 2,
            look: { body: 7, npcView: NpcView.JOB },
          },
        },
      }),
    ]);
    expect(asked[0]).toBe('sprite/mob/1002');
    // A job view loads a character's sprites, not an NPC sprite.
    expect(asked.slice(1).some((n) => n.startsWith('sprite/npc/'))).toBe(false);
    expect(asked.length).toBeGreaterThan(1);
    expect(o.titleOf(7)).toBe('<Society>');
    expect(o.titleOf(8)).toBe('');
  });

  it('passes music, digits, the quest log, sounds and colored server text on', () => {
    const o = online();
    o.setMap(map());
    const got: string[] = [];
    o.onMusic = (name) => {
      got.push(`music ${name}`);
    };
    o.onDigits = (kind, value) => {
      got.push(`digits ${String(kind)} ${String(value)}`);
    };
    o.onQuestLogOpen = (quest) => {
      got.push(`log ${String(quest)}`);
    };
    o.onServerText = (text, color) => {
      got.push(`text ${text} ${String(color)}`);
    };
    o.onSound = (name) => {
      got.push(`sound ${name}`);
    };
    o.handle([
      create(EventSchema, { body: { case: 'music', value: { name: 'music/30' } } }),
      create(EventSchema, {
        body: { case: 'digits', value: { kind: DigitsKind.COUNT_DOWN, value: -10 } },
      }),
      create(EventSchema, { body: { case: 'questLogOpen', value: { quest: 7 } } }),
      create(EventSchema, {
        body: { case: 'chat', value: { channel: ChatChannel.SYSTEM, text: 'Hi', color: 0xff00 } },
      }),
      // The player is not in sight yet, so its sound has no place.
      create(EventSchema, { body: { case: 'sound', value: { name: 'sound/ding' } } }),
    ]);
    expect(got).toEqual([
      'music music/30',
      `digits ${String(DigitsKind.COUNT_DOWN)} -10`,
      'log 7',
      'text Hi 65280',
    ]);
  });

  it('shows NPC progress bars until they are full', () => {
    const o = online();
    o.setMap(map());
    let now = 1000;
    o.clock = () => now;
    o.handle([
      create(EventSchema, {
        body: { case: 'npcProgressBar', value: { npc: 5, durationMs: 2000, color: 0xff0000 } },
      }),
    ]);
    now = 1500;
    expect([...o.npcBars()]).toEqual([[5, { fill: 0.25, color: 0xff0000 }]]);
    now = 3000;
    expect(o.npcBars().size).toBe(0);
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

  it('passes the color of a monster’s chat line to onChat', () => {
    const o = online();
    o.setMap(map());
    const colors: number[] = [];
    o.onChat = (_from, _name, _text, color) => {
      colors.push(color);
    };
    o.handle([
      create(EventSchema, {
        body: {
          case: 'chat',
          value: {
            channel: ChatChannel.LOCAL,
            from: 9,
            fromName: 'Poring',
            text: 'Grr!',
            color: 0xff8000,
          },
        },
      }),
      create(EventSchema, {
        body: {
          case: 'chat',
          value: { channel: ChatChannel.LOCAL, from: 3, fromName: 'Bob', text: 'hi' },
        },
      }),
    ]);
    expect(colors).toEqual([0xff8000, 0]);
  });

  it('passes whispers, channel lines and line IDs on', () => {
    const o = online();
    o.setMap(map());
    const lines: string[] = [];
    const id = new Uint8Array([1, 2]);
    o.onChat = (_from, name, text, _color, lineId) => {
      lines.push(`local ${name}: ${text} ${String(lineId.length)}`);
    };
    o.onWhisper = (from, text, lineId) => {
      lines.push(`whisper ${from}: ${text} ${String(lineId.length)}`);
    };
    o.onChannelChat = (channel, name, text, color) => {
      lines.push(`${channel} ${name}: ${text} ${color.toString(16)}`);
    };
    o.handle([
      create(EventSchema, {
        body: {
          case: 'chat',
          value: { channel: ChatChannel.LOCAL, from: 3, fromName: 'Bob', text: 'hi', lineId: id },
        },
      }),
      create(EventSchema, {
        body: {
          case: 'chat',
          value: { channel: ChatChannel.WHISPER, fromName: 'Eve', text: 'psst', lineId: id },
        },
      }),
      create(EventSchema, {
        body: {
          case: 'chat',
          value: {
            channel: ChatChannel.NAMED,
            channelName: '#trade',
            fromName: 'Ann',
            text: 'S> Knife',
            color: 0xff00,
          },
        },
      }),
    ]);
    expect(lines).toEqual(['local Bob: hi 2', 'whisper Eve: psst 2', '#trade Ann: S> Knife ff00']);
  });

  it('marks whispers and announcements from mods with the mod', () => {
    const o = online();
    o.setMap(map());
    const got: string[] = [];
    o.onWhisper = (from, text) => {
      got.push(`whisper ${from}: ${text}`);
    };
    o.onServerText = (text) => {
      got.push(`server ${text}`);
    };
    o.onAnnouncement = (text) => {
      got.push(`announce ${text}`);
    };
    o.handle([
      create(EventSchema, {
        body: {
          case: 'chat',
          value: {
            channel: ChatChannel.WHISPER,
            fromName: 'Bob',
            fromMod: 'example.a',
            text: 'psst',
          },
        },
      }),
      create(EventSchema, {
        body: {
          case: 'chat',
          value: { channel: ChatChannel.ANNOUNCEMENT, fromMod: 'example.a', text: 'Boss!' },
        },
      }),
      create(EventSchema, {
        body: { case: 'chat', value: { channel: ChatChannel.ANNOUNCEMENT, text: 'Restart' } },
      }),
    ]);
    expect(got).toEqual([
      'server [Mod example.a] Bob: psst',
      'announce [Mod example.a] Boss!',
      'announce Restart',
    ]);
  });

  it('passes channel, block, sanction and report events on', () => {
    const o = online();
    o.setMap(map());
    const got: string[] = [];
    o.onChannelJoined = (c) => {
      got.push(`joined ${c.name}`);
    };
    o.onChannelLeft = (name) => {
      got.push(`left ${name}`);
    };
    o.onBlockList = (names, full) => {
      got.push(`blocked ${names.join(',')} ${String(full)}`);
    };
    o.onSanction = (n) => {
      got.push(`sanction ${String(n.endsAtMs)}`);
    };
    o.onReportResult = (outcome) => {
      got.push(`report ${String(outcome)}`);
    };
    o.handle([
      create(EventSchema, { body: { case: 'channelJoined', value: { name: '#trade' } } }),
      create(EventSchema, { body: { case: 'channelLeft', value: { name: '#map' } } }),
      create(EventSchema, {
        body: { case: 'blockList', value: { names: ['Bob', 'Eve'], full: true } },
      }),
      create(EventSchema, { body: { case: 'sanction', value: { endsAtMs: 5n } } }),
      create(EventSchema, { body: { case: 'reportResult', value: { outcome: 1 } } }),
    ]);
    expect(got).toEqual([
      'joined #trade',
      'left #map',
      'blocked Bob,Eve true',
      'sanction 5',
      'report 1',
    ]);
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

  it('draws the mount, companion and cart of the option bits', () => {
    // A Rune Knight on a dragon, with a cart.
    const rider = lookOf(create(LookSchema, { body: 4054, sex: 1, weapon: 2 }), 0x80000 | 0x8);
    expect(rider).toMatchObject({
      body: 'sprite/body/rune_knight2/m',
      weapon: 'sprite/weapon/rune_knight2/m/2',
      cart: 'sprite/cart/1',
    });
    expect(rider.companion).toBeUndefined();
    // A Hunter on a costume mount, with a falcon. Equipment keeps the job's look.
    const hunter = lookOf(create(LookSchema, { body: 11, sex: 0, weapon: 11 }), 0x20 | 0x10);
    expect(hunter).toMatchObject({
      body: 'sprite/body/ostrich_hunter/f',
      weapon: 'sprite/weapon/hunter/f/11',
      companion: 'sprite/companion/falcon',
    });
    expect(hunter.cart).toBeUndefined();
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

  it('loads a companion and cart on OptionChange, and not for a tint', async () => {
    vi.stubGlobal('reportError', () => {});
    const asked: [string, boolean | undefined][] = [];
    const sprites = {
      get: async (name: string, placeholder?: boolean) => {
        asked.push([name, placeholder]);
        await Promise.resolve();
        return name.startsWith('sprite/body/') ||
          name.startsWith('sprite/companion/') ||
          name.startsWith('sprite/cart/')
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
          value: { id: 1, kind: EntityKind.PLAYER, x: 4, y: 5, dir: 3, look: { body: 11, sex: 1 } },
        },
      }),
    ]);
    await settle();
    const before = o.me?.character;
    // Poison only tints, so the character stays.
    o.handle([
      create(EventSchema, {
        body: { case: 'optionChange', value: { id: 1, options: { health: 1 } } },
      }),
    ]);
    await settle();
    expect(o.me?.character).toBe(before);
    o.handle([
      create(EventSchema, {
        body: { case: 'optionChange', value: { id: 1, options: { option: 0x10 | 0x8 } } },
      }),
    ]);
    await settle();
    expect(asked).toContainEqual(['sprite/companion/falcon', true]);
    expect(asked).toContainEqual(['sprite/cart/1', true]);
    const after = o.me?.character;
    expect(after).not.toBe(before);
    expect(after?.direction).toBe(3);
    expect(o.position(1)?.x).toBe(4.5);
    // A LookChange keeps the companion.
    asked.length = 0;
    o.handle([
      create(EventSchema, {
        body: { case: 'lookChange', value: { id: 1, look: { body: 11, sex: 1, headTop: 17 } } },
      }),
    ]);
    await settle();
    expect(asked).toContainEqual(['sprite/companion/falcon', true]);
    expect(o.me?.character).not.toBe(after);
  });
});

describe('status icons and options', () => {
  it('passes the player status icons on', () => {
    const o = online();
    o.setMap(map());
    const icons: [number, boolean, number][] = [];
    o.onStatusIcon = (icon, on, remainingMs) => {
      icons.push([icon, on, remainingMs]);
    };
    o.handle([
      create(EventSchema, {
        body: { case: 'statusIcon', value: { icon: 12, on: true, remainingMs: 60_000n } },
      }),
      create(EventSchema, {
        body: { case: 'statusIcon', value: { icon: 12, on: false, remainingMs: 0n } },
      }),
    ]);
    expect(icons).toEqual([
      [12, true, 60_000],
      [12, false, 0],
    ]);
  });

  it('tints and hides units by their options, also while they load', async () => {
    vi.stubGlobal('reportError', () => {});
    const sprites = {
      get: async () => {
        await Promise.resolve();
        return sprite();
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
          value: { id: 1, kind: EntityKind.PLAYER, x: 1, y: 1, look: { body: 0 } },
        },
      }),
      create(EventSchema, {
        body: {
          case: 'appear',
          value: {
            id: 2,
            kind: EntityKind.MONSTER,
            x: 2,
            y: 2,
            look: { body: 1002 },
            options: { option: 0x2 },
          },
        },
      }),
      // The player freezes while its sprites load.
      create(EventSchema, {
        body: { case: 'optionChange', value: { id: 1, options: { body: 2 } } },
      }),
    ]);
    await settle();
    const me = o.me;
    expect(me?.appearance.color).toEqual([0.55, 0.75, 1]);
    // The hidden monster does not show.
    expect(o.group.children.some((c) => !c.visible)).toBe(true);
    o.handle([
      create(EventSchema, {
        body: { case: 'optionChange', value: { id: 1, options: { option: 0x4 } } },
      }),
      create(EventSchema, {
        body: { case: 'optionChange', value: { id: 2, options: {} } },
      }),
    ]);
    expect(me?.appearance).toMatchObject({ visible: true, opacity: 0.5 });
    expect(me?.character.group.visible).toBe(true);
    expect(o.group.children.every((c) => c.visible)).toBe(true);
  });
});

describe('skills', () => {
  function skillUnit(id: number, unitId: number, skillId: number, x: number, y: number): Event {
    return create(EventSchema, {
      body: {
        case: 'appear',
        value: {
          id,
          kind: EntityKind.SKILL_UNIT,
          x,
          y,
          skillUnit: { unitId, skillId, caster: 1 },
        },
      },
    });
  }

  function units(o: Online): number {
    return o.group.children.filter((c) => c.name === 'skill-unit').length;
  }

  it('shows ground skill units until they vanish', () => {
    const o = online();
    o.setMap(map());
    const sounds: string[] = [];
    o.onSound = (name) => {
      sounds.push(name);
    };
    // Safety Wall: an STR effect that waits for the cache, and three cylinders.
    o.effects = {
      cache: { get: async () => Promise.resolve(undefined) } as unknown as EffectCache,
      textures: { uniform: () => ({ value: new Texture() }) },
    };
    o.handle([skillUnit(20, 0x7e, 12, 3, 4), skillUnit(21, 0x99, 12, 4, 4)]);
    expect(units(o)).toBe(1);
    const g = o.group.children.find((c) => c.name === 'skill-unit');
    expect(g?.position.x).toBe(3.5);
    expect(g?.children).toHaveLength(3);
    expect(sounds).toEqual(['sound/effect/ef_glasswall']);
    o.update(60_000, 0, g!.position.clone());
    // Cylinders repeat while the unit stays.
    expect(g?.children.some((c) => c.children.some((m) => m.visible))).toBe(true);
    o.handle([create(EventSchema, { body: { case: 'vanish', value: { id: 20 } } })]);
    expect(units(o)).toBe(0);
  });

  it('plays Thunder Storm once for its cell effect and its unit', () => {
    const o = online();
    o.setMap(map());
    o.clock = () => 1000;
    const sounds: string[] = [];
    o.onSound = (name) => {
      sounds.push(name);
    };
    o.effects = {
      cache: { get: async () => Promise.resolve(undefined) } as unknown as EffectCache,
      textures: { uniform: () => ({ value: new Texture() }) },
    };
    o.handle([
      create(EventSchema, {
        body: {
          case: 'skillEffect',
          value: {
            source: 1,
            skillId: 21,
            level: 3,
            target: { case: 'cell', value: { x: 5, y: 5 } },
            ok: true,
          },
        },
      }),
      skillUnit(30, 0x86, 21, 5, 5),
    ]);
    expect(sounds).toEqual(['sound/effect/magician_thunderstorm']);
    expect(units(o)).toBe(0);
  });

  it('passes the skill list, failures, and cooldowns on', () => {
    const o = online();
    o.setMap(map());
    const got: string[] = [];
    o.onSkillList = (list) => {
      got.push(`list ${list.map((s) => s.skillId).join(',')}`);
    };
    o.onSkillFailed = (id, reason) => {
      got.push(`failed ${id} ${reason}`);
    };
    o.onSkillCooldown = (id, ms) => {
      got.push(`cooldown ${id} ${ms}`);
    };
    o.handle([
      create(EventSchema, {
        body: { case: 'skillList', value: { skills: [{ skillId: 5 }, { skillId: 6 }] } },
      }),
      create(EventSchema, { body: { case: 'skillFailed', value: { skillId: 5, reason: 2 } } }),
      create(EventSchema, { body: { case: 'skillCooldown', value: { skillId: 0, ms: 700 } } }),
    ]);
    expect(got).toEqual(['list 5,6', 'failed 5 2', 'cooldown 0 700']);
  });

  it('starts and cancels casts with their aura', () => {
    vi.stubGlobal('reportError', () => {});
    const o = new Online(
      1,
      { get: async () => Promise.resolve(undefined) } as unknown as SpriteCache,
      {} as Pathfinder,
    );
    o.effects = {
      cache: {} as EffectCache,
      textures: { uniform: () => ({ value: new Texture() }) },
    };
    o.setMap(map());
    const casts: string[] = [];
    o.onSkillCast = (source, skill, ms) => {
      casts.push(`cast ${source} ${skill} ${ms}`);
    };
    o.onSkillCastCancel = (source) => {
      casts.push(`cancel ${source}`);
    };
    // The caster is still loading, so the aura plays on its cell.
    o.handle([
      create(EventSchema, {
        body: {
          case: 'appear',
          value: { id: 9, kind: EntityKind.MONSTER, x: 2, y: 2, look: { body: 1002 } },
        },
      }),
    ]);
    const before = o.group.children.length;
    o.handle([
      create(EventSchema, {
        body: {
          case: 'skillCast',
          value: {
            source: 9,
            skillId: 19,
            level: 1,
            castMs: 1500,
            element: 3,
            target: { case: 'entity', value: 1 },
          },
        },
      }),
    ]);
    // The fire aura: three cylinders.
    expect(o.group.children.length).toBe(before + 3);
    o.handle([create(EventSchema, { body: { case: 'skillCastCancel', value: { source: 9 } } })]);
    expect(o.group.children.length).toBe(before);
    expect(casts).toEqual(['cast 9 19 1500', 'cancel 9']);
  });

  it('uses the effects of a mod skill icon', () => {
    const o = online();
    o.setMap(map());
    o.skillIcon = (id) => (id === 3001 ? 28 : id);
    const sounds: string[] = [];
    o.onSound = (name) => {
      sounds.push(name);
    };
    o.handle([
      create(EventSchema, {
        body: {
          case: 'skillEffect',
          value: {
            source: 1,
            skillId: 3001,
            level: 1,
            target: { case: 'cell', value: { x: 1, y: 1 } },
            ok: true,
          },
        },
      }),
    ]);
    expect(sounds).toEqual(['sound/_heal_effect']);
  });
});

describe('skill numbers', () => {
  async function loaded(): Promise<Online> {
    vi.stubGlobal('reportError', () => {});
    const sprites = {
      get: async () => {
        await Promise.resolve();
        return sprite();
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
          value: { id: 1, kind: EntityKind.PLAYER, x: 1, y: 1, look: { body: 0 } },
        },
      }),
      create(EventSchema, {
        body: {
          case: 'appear',
          value: { id: 2, kind: EntityKind.MONSTER, x: 3, y: 3, look: { body: 1002 } },
        },
      }),
    ]);
    await settle();
    return o;
  }

  it('passes a repeated sound on, and shows a transformed player as its monster', async () => {
    const o = await loaded();
    const sounds: string[] = [];
    o.onSound = (name, _at, repeat) => {
      sounds.push(`${name} ${String(repeat)}`);
    };
    o.handle([
      create(EventSchema, { body: { case: 'sound', value: { name: 'sound/bell', repeat: true } } }),
      create(EventSchema, { body: { case: 'sound', value: { name: 'sound/ding' } } }),
    ]);
    expect(sounds).toEqual(['sound/bell true', 'sound/ding false']);
    const before = o.position(1);
    o.handle([
      create(EventSchema, {
        body: {
          case: 'lookChange',
          value: { id: 1, look: { body: 1002, npcView: NpcView.MONSTER } },
        },
      }),
    ]);
    await settle();
    // The player's unit now draws one sprite, the monster's, where it stood.
    expect(o.position(1)?.equals(before!)).toBe(true);
  });

  it('splits a multi-hit skill into its hits, then the total', async () => {
    const o = await loaded();
    vi.useFakeTimers();
    try {
      const hits: string[] = [];
      o.onHit = (h) => {
        hits.push(`${h.amount}${h.combo === true ? ' total' : ''}`);
      };
      o.handle([
        create(EventSchema, {
          body: {
            case: 'damage',
            value: {
              source: 1,
              target: 2,
              amount: 101n,
              hits: 2,
              kind: 1,
              delayMs: 100,
              skillId: 46,
            },
          },
        }),
      ]);
      vi.advanceTimersByTime(100);
      expect(hits).toEqual(['50']);
      vi.advanceTimersByTime(200);
      expect(hits).toEqual(['50', '51']);
      vi.advanceTimersByTime(200);
      expect(hits).toEqual(['50', '51', '101 total']);
    } finally {
      vi.useRealTimers();
    }
  });

  it('shows the HP Heal heals as a heal number', async () => {
    const o = await loaded();
    const hits: string[] = [];
    o.onHit = (h) => {
      hits.push(`${h.amount} ${h.kind} ${h.mine}`);
    };
    o.handle([
      create(EventSchema, {
        body: {
          case: 'skillEffect',
          value: {
            source: 1,
            skillId: 28,
            level: 3,
            target: { case: 'entity', value: 1 },
            value: 120n,
            ok: true,
          },
        },
      }),
      // A buff shows no number.
      create(EventSchema, {
        body: {
          case: 'skillEffect',
          value: {
            source: 1,
            skillId: 34,
            level: 3,
            target: { case: 'entity', value: 1 },
            value: 3n,
            ok: true,
          },
        },
      }),
    ]);
    expect(hits).toEqual(['120 5 true']);
  });

  it('names a mod monster, plays its sounds, and plays the effects a mod skill names', async () => {
    setArt(MOD_DATA);
    const o = await loaded();
    const asked: string[] = [];
    o.effects = {
      cache: {
        get: async (name: string) => {
          asked.push(name);
          return Promise.resolve(undefined);
        },
      } as unknown as EffectCache,
      textures: { uniform: () => ({ value: new Texture() }) },
    };
    const sounds: string[] = [];
    o.onSound = (name) => {
      sounds.push(name);
    };
    expect(o.nameOf(2)).toBe('Lantern Ghost');
    vi.useFakeTimers();
    try {
      o.handle([
        create(EventSchema, {
          body: {
            case: 'damage',
            value: {
              source: 1,
              target: 2,
              amount: 10n,
              hits: 1,
              kind: 1,
              delayMs: 0,
              skillId: 20000,
            },
          },
        }),
        create(EventSchema, {
          body: {
            case: 'damage',
            value: { source: 2, target: 1, amount: 3n, hits: 1, kind: 1, delayMs: 0 },
          },
        }),
      ]);
      vi.advanceTimersByTime(10);
      o.handle([create(EventSchema, { body: { case: 'vanish', value: { id: 2, reason: 2 } } })]);
    } finally {
      vi.useRealTimers();
    }
    expect(sounds).toEqual([
      'mod/example.lantern/sound/ghost_attack',
      'mod/example.lantern/sound/ghost_damage',
      'mod/example.lantern/sound/ghost_die',
    ]);
    expect(asked).toEqual(['mod/example.lantern/effect/flare']);
  });
});

describe('view sprites', () => {
  it('draws the view sprite the client data names for an item view', () => {
    setArt(MOD_DATA);
    const look = lookOf(create(LookSchema, { body: 0, sex: 1, headTop: 1000000, headMid: 17 }));
    expect(look.headgearTop).toBe('mod/example.lantern/sprite/headgear/Lantern_Hat');
    expect(look.headgearMid).toBe('sprite/headgear/m/17');
  });

  it('passes the world map on as plain data', () => {
    const o = online();
    o.setMap(map());
    const got: unknown[] = [];
    o.onWorldMap = (m) => {
      got.push(m);
    };
    o.handle([
      create(EventSchema, {
        body: {
          case: 'worldMap',
          value: {
            maps: [{ id: 'gen:field:0:0', kind: 'field', x: 0, y: -1 }],
            links: [{ a: 'gen:field:0:0', b: 'gen:town:t1' }],
          },
        },
      }),
    ]);
    expect(got).toEqual([
      {
        maps: [
          {
            id: 'gen:field:0:0',
            kind: 'field',
            name: '',
            x: 0,
            y: -1,
            floor: 0,
            visited: false,
            minLevel: 0,
            maxLevel: 0,
          },
        ],
        links: [{ a: 'gen:field:0:0', b: 'gen:town:t1' }],
      },
    ]);
  });

  it('passes quest events on as plain data', () => {
    const o = online();
    o.clock = () => 2000;
    o.setMap(map());
    const got: unknown[] = [];
    o.onQuestList = (quests) => {
      got.push(['list', quests]);
    };
    o.onQuestUpdate = (quest) => {
      got.push(['update', quest]);
    };
    o.onQuestRemove = (id) => {
      got.push(['remove', id]);
    };
    o.handle([
      create(EventSchema, {
        body: {
          case: 'questList',
          value: { quests: [{ id: 7, title: 'Hunt', active: true, limitMs: -1n }] },
        },
      }),
      create(EventSchema, {
        body: {
          case: 'questUpdate',
          value: {
            quest: {
              id: 7,
              title: 'Hunt',
              limitMs: 5000n,
              objectives: [{ label: 'Poring', monster: 1002, count: 1, goal: 3 }],
            },
          },
        },
      }),
      create(EventSchema, { body: { case: 'questRemove', value: { id: 7 } } }),
    ]);
    expect(got).toEqual([
      [
        'list',
        [{ id: 7, title: 'Hunt', active: true, limitMs: -1, receivedAt: 2000, objectives: [] }],
      ],
      [
        'update',
        {
          id: 7,
          title: 'Hunt',
          active: false,
          limitMs: 5000,
          receivedAt: 2000,
          objectives: [
            { label: 'Poring', monster: 1002, count: 1, goal: 3, minLevel: 0, maxLevel: 0 },
          ],
        },
      ],
      ['remove', 7],
    ]);
  });

  it('keeps quest icons until removed, until the NPC goes, or until the map changes', () => {
    const o = online();
    o.setMap(map());
    const icon = (npc: number, name: string): Event =>
      create(EventSchema, { body: { case: 'questIcon', value: { npc, icon: name } } });
    o.handle([icon(5, 'quest'), icon(6, 'job_question'), icon(7, 'warg')]);
    expect([...o.questIcons()]).toEqual([
      [5, 'quest'],
      [6, 'job_question'],
      [7, 'warg'],
    ]);
    // An empty name removes the icon, and a vanish takes it away.
    o.handle([
      icon(5, ''),
      create(EventSchema, { body: { case: 'vanish', value: { id: 6 } } }),
      icon(7, 'warg_face'),
    ]);
    expect([...o.questIcons()]).toEqual([[7, 'warg_face']]);
    o.handle([
      create(EventSchema, {
        body: { case: 'mapChange', value: { map: 'map/b', x: 3, y: 4, netId: 9 } },
      }),
    ]);
    expect(o.questIcons().size).toBe(0);
  });

  it('draws the view sprites of a weapon and a shield', () => {
    setArt(MOD_DATA);
    const look = lookOf(
      create(LookSchema, { body: 1, sex: 1, weapon: 2, weaponView: 1000002, shield: 1000001 }),
    );
    expect(look.weapon).toBe('mod/example.lantern/sprite/weapon/Lantern_Staff');
    expect(look.shield).toBe('mod/example.lantern/sprite/shield/Lantern_Shield');
    // Without a view of art, the weapon type and the shield's view show.
    const plain = lookOf(create(LookSchema, { body: 1, sex: 1, weapon: 2, shield: 4 }));
    expect(plain.weapon).toBe('sprite/weapon/swordman/m/2');
    expect(plain.shield).toBe('sprite/shield/swordman/m/4');
  });

  it('draws the art of companion and mount records', () => {
    setArt(MOD_DATA);
    // A Knight on a mount of art that acts as a Peco Peco, with a
    // companion of art that acts as a falcon: the body stays on foot.
    const look = lookOf(
      create(LookSchema, { body: 7, sex: 1, companion: 10000, mount: 10000 }),
      0x20 | 0x10,
    );
    expect(look).toMatchObject({
      body: 'sprite/body/knight/m',
      companion: 'mod/example.lantern/sprite/companion/Wisp',
      mount: 'mod/example.lantern/sprite/mount/Lantern_Strider',
    });
    // Records without art keep the option bits' looks.
    const core = lookOf(
      create(LookSchema, { body: 11, sex: 0, companion: 1, mount: 1 }),
      0x20 | 0x10,
    );
    expect(core).toMatchObject({
      body: 'sprite/body/ostrich_hunter/f',
      companion: 'sprite/companion/falcon',
    });
    expect(core.mount).toBeUndefined();
  });
});

describe('body size effects', () => {
  function effect(entity: number, effectId: number): Event {
    return create(EventSchema, {
      body: { case: 'effect', value: { effectId, at: { case: 'entity', value: entity } } },
    });
  }

  it('scales small and big monsters and plays no particles for them', async () => {
    vi.stubGlobal('reportError', () => {});
    const sprites = {
      get: async () => {
        await Promise.resolve();
        return sprite();
      },
    } as unknown as SpriteCache;
    const o = new Online(1, sprites, {} as Pathfinder);
    o.setMap(map());
    o.handle([
      create(EventSchema, {
        body: {
          case: 'appear',
          value: { id: 1, kind: EntityKind.MONSTER, x: 1, y: 1, look: { body: 1002 } },
        },
      }),
      // The effect comes while the sprite loads.
      effect(1, 421),
    ]);
    await settle();
    const unit = o.me;
    expect(unit?.scale).toBe(0.75);
    const children = o.group.children.length;
    o.handle([effect(1, 423)]);
    expect(unit?.scale).toBe(1.25);
    expect(o.group.children.length).toBe(children);
  });

  it('leaves the size alone for other effects', async () => {
    vi.stubGlobal('reportError', () => {});
    const sprites = {
      get: async () => {
        await Promise.resolve();
        return sprite();
      },
    } as unknown as SpriteCache;
    const o = new Online(1, sprites, {} as Pathfinder);
    o.setMap(map());
    o.effects = {
      cache: {} as EffectCache,
      textures: { uniform: () => ({ value: new Texture() }) },
    };
    o.handle([
      create(EventSchema, {
        body: {
          case: 'appear',
          value: { id: 1, kind: EntityKind.MONSTER, x: 1, y: 1, look: { body: 1002 } },
        },
      }),
    ]);
    await settle();
    o.handle([effect(1, 422), effect(1, 16)]);
    expect(o.me?.scale).toBe(1);
  });
});
