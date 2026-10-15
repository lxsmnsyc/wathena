import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  dropSprite,
  itemIcon,
  jobBody,
  modItem,
  modItemNames,
  modJobName,
  modMonsterName,
  modSkillName,
  monsterSound,
  monsterSprite,
  npcSprite,
  setArt,
  skillArtEffects,
  skillIcon,
  viewSprite,
} from '../assets/art';
import { AssetStore, type Entry, type Fetch, modOf, sha256, shardOf } from '../assets/store';
import { type ClientData, httpOrigin, loadClientData, parseClientData } from './clientData';
import { skillEffects } from './tables';

const encoder = new TextEncoder();

/** Files of a build in memory, by path from the build's URL. */
async function buildFiles(assets: Record<string, { t: Entry['t']; data: Uint8Array<ArrayBuffer> }>) {
  const ext: Record<string, string> = { sprite: 'wspr', texture: 'webp' };
  const files = new Map<string, Uint8Array<ArrayBuffer>>();
  const shards: Record<string, Entry>[] = Array.from({ length: 256 }, () => ({}));
  for (const [name, a] of Object.entries(assets)) {
    const h = await sha256(a.data);
    files.set(`${h.slice(0, 2)}/${h}.${ext[a.t]}`, a.data);
    shards[await shardOf(name)]![name] = { h, t: a.t, s: a.data.byteLength };
  }
  const hashes: string[] = [];
  for (const s of shards) {
    const data = encoder.encode(JSON.stringify(s));
    const h = await sha256(data);
    hashes.push(h);
    files.set(`${h.slice(0, 2)}/${h}.json`, data);
  }
  files.set(
    'manifest.json',
    encoder.encode(JSON.stringify({ contract: 'assets-v1', build: 'test', shards: hashes })),
  );
  return files;
}

const icon = encoder.encode('lantern icon');
const ghost = encoder.encode('ghost sprite');
const placeholder = encoder.encode('placeholder');

function doc(over: Partial<ClientData> = {}): ClientData {
  return {
    version: 2,
    mods: [{ id: 'example.lantern', version: '0.1.0', assets: '/assets/mods/example.lantern/0.1.0/' }],
    items: {
      '2000000': {
        icon: 'mod/example.lantern/icon/item/Paper_Lantern',
        name: 'Paper Lantern',
        description: ['A lantern of paper.', '^0000ffWeight:^000000 1'],
      },
      '2000003': {
        name: 'Lantern Hat',
        slots: 1,
        view_sprite: 'mod/example.lantern/sprite/headgear/Lantern_Hat',
        view: 1000000,
      },
    },
    monsters: {
      '100000': {
        sprite: 'mod/example.lantern/sprite/mob/Lantern_Ghost',
        sounds: { die: 'mod/example.lantern/sound/Lantern_Ghost_die' },
        name: 'Lantern Ghost',
      },
    },
    npc_sprites: { '1000000': 'mod/example.lantern/sprite/npc/Lamplighter' },
    jobs: { '10000': { name: 'Desperado', body: { m: 'mod/example.jobs/sprite/body/Desperado/m' } } },
    skills: {
      '20000': {
        name: 'Lantern Flare',
        icon: 'mod/example.lantern/icon/skill/LANTERN_FLARE',
        effects: { hit: ['mod/example.lantern/effect/flare'] },
      },
    },
    companions: {},
    mounts: {},
    ...over,
  };
}

/** A game server at http://game: core's build at /assets/, a mod's, and the client data. */
async function server(clientData: unknown) {
  const core = await buildFiles({ 'placeholder/texture': { t: 'texture', data: placeholder } });
  const mod = await buildFiles({
    'mod/example.lantern/icon/item/Paper_Lantern': { t: 'texture', data: icon },
    'mod/example.lantern/sprite/mob/Lantern_Ghost': { t: 'sprite', data: ghost },
  });
  const requests: string[] = [];
  const fetcher: Fetch = async (url) => {
    requests.push(url);
    let data: Uint8Array<ArrayBuffer> | undefined;
    if (url === 'http://game/client-data.json') {
      if (clientData === undefined) {
        return new Response('not found', { status: 404 });
      }
      data = encoder.encode(JSON.stringify(clientData));
    } else if (url.startsWith('http://game/assets/mods/example.lantern/0.1.0/')) {
      data = mod.get(url.slice('http://game/assets/mods/example.lantern/0.1.0/'.length));
    } else if (url.startsWith('/assets/')) {
      data = core.get(url.slice('/assets/'.length));
    }
    return Promise.resolve(
      data === undefined ? new Response('not found', { status: 404 }) : new Response(data.slice()),
    );
  };
  return { fetcher, requests };
}

afterEach(() => {
  setArt(undefined);
  vi.restoreAllMocks();
});

describe('loadClientData', () => {
  it('loads core, then each mod build, and resolves mod names', async () => {
    const s = await server(doc());
    const store = await AssetStore.open('/assets/', s.fetcher);
    const data = await loadClientData('ws://game/ws', store, s.fetcher);
    expect(data?.mods[0]?.id).toBe('example.lantern');
    expect(store.modIds()).toEqual(['example.lantern']);
    // The mod's manifest loads after core's.
    const manifests = s.requests.filter((r) => r.endsWith('manifest.json'));
    expect(manifests).toEqual(['/assets/manifest.json', 'http://game/assets/mods/example.lantern/0.1.0/manifest.json']);
    expect(await store.bytes('mod/example.lantern/icon/item/Paper_Lantern', 'texture')).toEqual(icon);
    expect(await store.bytes('mod/example.lantern/sprite/mob/Lantern_Ghost', 'sprite')).toEqual(ghost);
    // Core's names still resolve in core's build.
    expect(await store.bytes('placeholder/texture', 'texture')).toEqual(placeholder);
  });

  it('logs a missing mod name once', async () => {
    const s = await server(doc());
    const store = await AssetStore.open('/assets/', s.fetcher);
    await loadClientData('ws://game/ws', store, s.fetcher);
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    expect(await store.bytes('mod/example.lantern/icon/item/Unlit_Lantern', 'texture')).toBeUndefined();
    expect(await store.bytes('mod/example.lantern/icon/item/Unlit_Lantern', 'texture')).toBeUndefined();
    // A mod without a build is missing too.
    expect(await store.bytes('mod/example.other/icon/item/X', 'texture')).toBeUndefined();
    expect(warn).toHaveBeenCalledTimes(2);
  });

  it('applies the merged tables', async () => {
    const tables = {
      'skills.yml': {
        skills: [{ id: 5, constant: 'SM_BASH', caster: [16], hit: [1, 17] }],
        units: [],
        auras: [{ id: 0, constant: 'ELE_NEUTRAL', effect: 54 }],
      },
    };
    const before = skillEffects.get(5);
    const s = await server(doc({ tables }));
    const store = await AssetStore.open('/assets/', s.fetcher);
    await loadClientData('ws://game/ws', store, s.fetcher);
    expect(skillEffects.get(5)?.hit).toEqual([1, 17]);
    // Put core's table back for other tests.
    expect(before).toBeDefined();
  });

  it('leaves everything as it is without client data', async () => {
    const s = await server(undefined);
    const store = await AssetStore.open('/assets/', s.fetcher);
    expect(await loadClientData('ws://game/ws', store, s.fetcher)).toBeUndefined();
    expect(store.modIds()).toEqual([]);
  });

  it('refuses client data of another shape', async () => {
    const s = await server({ ...doc(), version: 1 });
    const store = await AssetStore.open('/assets/', s.fetcher);
    await expect(loadClientData('ws://game/ws', store, s.fetcher)).rejects.toThrow('unknown version');
  });
});

describe('parseClientData', () => {
  it('checks names against their namespaces', () => {
    expect(() => parseClientData(doc())).not.toThrow();
    for (const bad of [
      doc({ items: { '1': { icon: 'sprite/mob/1002' } } }),
      doc({ monsters: { '1': { sprite: 'icon/item/501' } } }),
      doc({ npc_sprites: { '1': '' } }),
      doc({ mods: [{ id: 'Bad', version: '1', assets: '/x/' }] }),
      doc({ mods: [{ id: 'a', version: '1', assets: 'http://elsewhere/' }] }),
      doc({ items: { '1': { name: '' } } }),
      doc({ items: { '1': { description: 'one line' as unknown as string[] } } }),
      doc({ items: { '1': { slots: 9 } } }),
      doc({ items: { '1': { view: -1 } } }),
      doc({ monsters: { '1': { name: 7 as unknown as string } } }),
      // Version 1's job shape.
      doc({ jobs: { '1': { m: 'sprite/body/x/m' } as unknown as { name: string } } }),
      doc({ skills: { '1': { effects: { hit: ['sprite/mob/1'] } } } }),
    ]) {
      expect(() => parseClientData(bad)).toThrow('invalid');
    }
  });
});

describe('art', () => {
  it('names records by ID until the server names their art', () => {
    expect(itemIcon(2000000)).toBe('icon/item/2000000');
    expect(monsterSprite(1002)).toBe('sprite/mob/1002');
    setArt(doc());
    expect(itemIcon(2000000)).toBe('mod/example.lantern/icon/item/Paper_Lantern');
    expect(itemIcon(501)).toBe('icon/item/501');
    expect(dropSprite(2000000)).toBe('sprite/item/2000000');
    expect(monsterSprite(100000)).toBe('mod/example.lantern/sprite/mob/Lantern_Ghost');
    expect(npcSprite(1000000)).toBe('mod/example.lantern/sprite/npc/Lamplighter');
    expect(npcSprite(117)).toBe('sprite/npc/117');
    expect(jobBody(10000, 'm')).toBe('mod/example.jobs/sprite/body/Desperado/m');
    expect(jobBody(10000, 'f')).toBeUndefined();
    expect(skillIcon(20000)).toBe('mod/example.lantern/icon/skill/LANTERN_FLARE');
    expect(skillArtEffects(20000, 'hit')).toEqual(['mod/example.lantern/effect/flare']);
    expect(skillArtEffects(20000, 'caster')).toBeUndefined();
    expect(skillArtEffects(5, 'hit')).toBeUndefined();
    expect(monsterSound(100000, 'die')).toBe('mod/example.lantern/sound/Lantern_Ghost_die');
    expect(monsterSound(100000, 'attack')).toBeUndefined();
    expect(viewSprite(1000000)).toBe('mod/example.lantern/sprite/headgear/Lantern_Hat');
    expect(viewSprite(17)).toBeUndefined();
  });

  it('names the records of mods', () => {
    expect(modItem(2000000)).toBeUndefined();
    expect(modJobName(10000)).toBeUndefined();
    setArt(doc());
    expect(modItem(2000000)).toEqual({
      name: 'Paper Lantern',
      slots: 0,
      description: ['A lantern of paper.', '^0000ffWeight:^000000 1'],
    });
    expect(modItem(2000003)).toEqual({ name: 'Lantern Hat', slots: 1 });
    expect(modItem(501)).toBeUndefined();
    expect(modItemNames()).toEqual([
      [2000000, 'Paper Lantern'],
      [2000003, 'Lantern Hat'],
    ]);
    expect(modMonsterName(100000)).toBe('Lantern Ghost');
    expect(modMonsterName(1002)).toBeUndefined();
    expect(modSkillName(20000)).toBe('Lantern Flare');
    expect(modJobName(10000)).toBe('Desperado');
  });

  it('finds the mod of a name', () => {
    expect(modOf('mod/example.lantern/icon/item/X')).toBe('example.lantern');
    expect(modOf('icon/item/501')).toBeUndefined();
    expect(modOf('mod/')).toBeUndefined();
  });

  it('reads the game server origin', () => {
    expect(httpOrigin('wss://play.example.com/ws')).toBe('https://play.example.com');
    expect(httpOrigin('ws://localhost:8300/ws')).toBe('http://localhost:8300');
    expect(httpOrigin('ftp://x')).toBeUndefined();
  });
});
