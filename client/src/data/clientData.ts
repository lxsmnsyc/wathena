// The client data the game server serves at /client-data.json (decision
// 0033): the active mods' asset builds, the art that records of mods use,
// by ID, and the client data tables merged with the mods' tables. Version
// 2 adds the names of mods' items, monsters, skills and jobs, and the
// descriptions of items, and moves a job's body sprites under body
// (review decision 40).

import { AssetStore, type Fetch } from '../assets/store';
import { applyTables } from './tables';

/** The client data format this client reads. */
export const CLIENT_DATA_VERSION = 2;

export interface ModBuild {
  id: string;
  version: string;
  /** The build's URL path on the game server, ending in "/". */
  assets: string;
}

export interface ItemArt {
  icon?: string;
  image?: string;
  drop_sprite?: string;
  view_sprite?: string;
  /** The view ID the server gave an item with a view_sprite. */
  view?: number;
  /** The name of a mod's item, or a name a mod set. */
  name?: string;
  /** Card slots, with a name. */
  slots?: number;
  /** Description lines, with RO's color codes. */
  description?: string[];
}

export interface MonsterArt {
  sprite?: string;
  sounds?: Record<string, string>;
  name?: string;
}

export interface SkillArt {
  icon?: string;
  effects?: Record<string, string[]>;
  name?: string;
}

export interface JobArt {
  name?: string;
  /** Body sprites by sex, m and f. */
  body?: Record<string, string>;
  /**
   * The job whose sprites and client tables the job uses beside its body,
   * for a job that looks send by its own ID, such as a mod's job.
   */
  look?: number;
}

export interface ClientData {
  version: number;
  mods: ModBuild[];
  items: Record<string, ItemArt>;
  monsters: Record<string, MonsterArt>;
  npc_sprites: Record<string, string>;
  jobs: Record<string, JobArt>;
  skills: Record<string, SkillArt>;
  companions: Record<string, string>;
  mounts: Record<string, string>;
  tables?: Record<string, unknown>;
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

/** A name a record may use: a string under one of the namespaces given. */
function isName(v: unknown, types: readonly string[]): v is string {
  if (typeof v !== 'string' || v.length === 0 || v.length > 255) {
    return false;
  }
  const rest = v.startsWith('mod/') ? v.split('/').slice(2).join('/') : v;
  return types.some((t) => rest.startsWith(`${t}/`));
}

function names(v: unknown, types: readonly string[]): v is Record<string, string> {
  return isObject(v) && Object.values(v).every((n) => isName(n, types));
}

const TEXTURE = ['icon', 'image', 'texture', 'ui'] as const;
const SPRITE = ['sprite'] as const;

/** The longest name and description line the client keeps, in UTF-16 units. */
const MAX_TEXT = 1024;

/** A record's name: text of 1 to MAX_TEXT units, or none. */
function isText(v: unknown): boolean {
  return v === undefined || (typeof v === 'string' && v.length > 0 && v.length <= MAX_TEXT);
}

function isItemArt(v: unknown): v is ItemArt {
  return (
    isObject(v) &&
    (v.icon === undefined || isName(v.icon, TEXTURE)) &&
    (v.image === undefined || isName(v.image, TEXTURE)) &&
    (v.drop_sprite === undefined || isName(v.drop_sprite, SPRITE)) &&
    (v.view_sprite === undefined || isName(v.view_sprite, SPRITE)) &&
    (v.view === undefined || (Number.isInteger(v.view) && typeof v.view === 'number' && v.view > 0)) &&
    isText(v.name) &&
    (v.slots === undefined || (Number.isInteger(v.slots) && typeof v.slots === 'number' && v.slots >= 0 && v.slots <= 4)) &&
    (v.description === undefined ||
      (Array.isArray(v.description) &&
        v.description.length <= 256 &&
        v.description.every((l) => typeof l === 'string' && l.length <= MAX_TEXT)))
  );
}

function isMonsterArt(v: unknown): v is MonsterArt {
  return (
    isObject(v) &&
    (v.sprite === undefined || isName(v.sprite, SPRITE)) &&
    (v.sounds === undefined || names(v.sounds, ['sound'])) &&
    isText(v.name)
  );
}

function isSkillArt(v: unknown): v is SkillArt {
  return (
    isObject(v) &&
    (v.icon === undefined || isName(v.icon, TEXTURE)) &&
    (v.effects === undefined ||
      (isObject(v.effects) &&
        Object.values(v.effects).every((l) => Array.isArray(l) && l.every((n) => isName(n, ['effect']))))) &&
    isText(v.name)
  );
}

/** A job: its name and body sprites. Version 1's sex keys are refused. */
function isJobArt(v: unknown): v is JobArt {
  return (
    isObject(v) &&
    Object.keys(v).every((k) => k === 'name' || k === 'body' || k === 'look') &&
    isText(v.name) &&
    (v.look === undefined || (typeof v.look === 'number' && Number.isInteger(v.look) && v.look >= 0)) &&
    (v.body === undefined || names(v.body, SPRITE))
  );
}

function isModBuild(v: unknown): v is ModBuild {
  return (
    isObject(v) &&
    typeof v.id === 'string' &&
    /^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)*$/.test(v.id) &&
    typeof v.version === 'string' &&
    typeof v.assets === 'string' &&
    v.assets.startsWith('/') &&
    v.assets.endsWith('/')
  );
}

function all<T>(v: unknown, check: (x: unknown) => x is T): v is Record<string, T> {
  return isObject(v) && Object.values(v).every(check);
}

/** Checks the client data's shape. Anything else is an error. */
export function parseClientData(v: unknown): ClientData {
  if (!isObject(v) || v.version !== CLIENT_DATA_VERSION) {
    throw new Error('client data: unknown version');
  }
  const { mods, items, monsters, jobs, skills, tables } = v;
  const invalid = new Error('client data: invalid');
  if (!Array.isArray(mods) || !mods.every(isModBuild)) {
    throw invalid;
  }
  if (!all(items, isItemArt) || !all(monsters, isMonsterArt) || !all(skills, isSkillArt)) {
    throw invalid;
  }
  if (!all(jobs, isJobArt)) {
    throw invalid;
  }
  const sprites = (x: unknown): Record<string, string> => {
    if (!names(x, SPRITE)) {
      throw invalid;
    }
    return x;
  };
  if (tables !== undefined && !isObject(tables)) {
    throw invalid;
  }
  return {
    version: CLIENT_DATA_VERSION,
    mods,
    items,
    monsters,
    npc_sprites: sprites(v.npc_sprites),
    jobs,
    skills,
    companions: sprites(v.companions),
    mounts: sprites(v.mounts),
    ...(tables === undefined ? {} : { tables }),
  };
}

/**
 * The game server's HTTP origin from its WebSocket URL: ws becomes http
 * and wss becomes https. Undefined for a URL that cannot be read.
 */
export function httpOrigin(server: string): string | undefined {
  let u: URL;
  try {
    u = new URL(server);
  } catch {
    return undefined;
  }
  if (u.protocol === 'wss:' || u.protocol === 'https:') {
    return `https://${u.host}`;
  }
  if (u.protocol === 'ws:' || u.protocol === 'http:') {
    return `http://${u.host}`;
  }
  return undefined;
}

/**
 * Loads the client data from a game server, opens each active mod's build
 * and adds it to store, and applies the merged tables. A server without
 * client data, such as an older one, leaves everything as it is. A mod
 * build that fails to open is logged, and its art shows placeholders.
 */
export async function loadClientData(
  server: string,
  store: AssetStore,
  fetcher: Fetch = async (url, init) => fetch(url, init),
): Promise<ClientData | undefined> {
  const origin = httpOrigin(server);
  if (origin === undefined) {
    return undefined;
  }
  const res = await fetcher(`${origin}/client-data.json`, { cache: 'no-cache' });
  if (res.status === 404) {
    return undefined;
  }
  if (!res.ok) {
    throw new Error(`client data: HTTP ${res.status}`);
  }
  const data = parseClientData(await res.json());
  // Core's manifest is open already. Each mod's loads after it.
  for (const m of data.mods) {
    try {
      store.addMod(m.id, await AssetStore.open(`${origin}${m.assets}`, fetcher));
    } catch (error) {
      console.error(`mod ${m.id}: assets:`, error);
    }
  }
  if (data.tables !== undefined) {
    applyTables(data.tables);
  }
  return data;
}
