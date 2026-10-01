// The client data tables in data/ (strategy section 10.5). The Go package
// in data/ checks them, and the build imports them as JSON.

import effectsTable from '../../data/effects.yml';
import jobsTable from '../../data/jobs.yml';
import npcSpritesTable from '../../data/npc_sprites.yml';
import weaponsTable from '../../data/weapons.yml';

export interface Weapon {
  id: number;
  constant: string;
  name: string;
  /** The weapon sprite for items without their own view. 0 draws none. */
  view: number;
  attack: string[];
  hit: string[];
}

/**
 * A textured cone the client draws, as roBrowser's CYLINDER effects.
 * data/effects.yml explains the fields.
 */
export interface Cylinder {
  texture: string;
  bottom: number;
  top: number;
  height: number;
  color: [number, number, number];
  alpha: number;
  duration_ms: number;
  copies: number;
  copy_ms: number;
  animation: number;
  fade: boolean;
  repeat: boolean;
}

/**
 * One thing an effect shows: an STR effect, one of several STR effects at
 * random, a sound, or a cylinder.
 */
export type EffectPart =
  | { effect: string }
  | { variants: string[] }
  | { sound: string }
  | { cylinder: Cylinder };

/**
 * The STR effect a part shows, or undefined for a sound. random(n) picks
 * one of n variants.
 */
export function partEffect(part: EffectPart, random: (n: number) => number): string | undefined {
  if ('effect' in part) {
    return part.effect;
  }
  if ('variants' in part) {
    return part.variants[random(part.variants.length)];
  }
  return undefined;
}

export interface EffectEntry {
  id: number;
  constant: string;
  parts: EffectPart[];
}

function isObject(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

function strings(v: unknown): v is string[] {
  return Array.isArray(v) && v.every((s) => typeof s === 'string');
}

function isWeapon(v: unknown): v is Weapon {
  return (
    isObject(v) &&
    typeof v.id === 'number' &&
    typeof v.constant === 'string' &&
    typeof v.name === 'string' &&
    typeof v.view === 'number' &&
    strings(v.attack) &&
    strings(v.hit)
  );
}

function isCylinder(v: unknown): v is Cylinder {
  return (
    isObject(v) &&
    typeof v.texture === 'string' &&
    ['bottom', 'top', 'height', 'alpha', 'duration_ms', 'copies', 'copy_ms', 'animation'].every(
      (k) => typeof v[k] === 'number',
    ) &&
    Array.isArray(v.color) &&
    v.color.length === 3 &&
    v.color.every((c) => typeof c === 'number') &&
    typeof v.fade === 'boolean' &&
    typeof v.repeat === 'boolean'
  );
}

function isPart(v: unknown): v is EffectPart {
  return (
    isObject(v) &&
    (typeof v.effect === 'string' ||
      strings(v.variants) ||
      typeof v.sound === 'string' ||
      isCylinder(v.cylinder))
  );
}

function isEffect(v: unknown): v is EffectEntry {
  return (
    isObject(v) &&
    typeof v.id === 'number' &&
    typeof v.constant === 'string' &&
    Array.isArray(v.parts) &&
    v.parts.every(isPart)
  );
}

/** Reads a table's list, keyed by id. A table of the wrong shape is an error. */
function table<T extends { id: number }>(
  name: string,
  data: unknown,
  key: string,
  check: (v: unknown) => v is T,
): ReadonlyMap<number, T> {
  const list = isObject(data) ? data[key] : undefined;
  if (!Array.isArray(list) || !list.every(check)) {
    throw new Error(`${name}: invalid table`);
  }
  return new Map(list.map((v) => [v.id, v]));
}

export interface JobLook {
  look: string;
  race: 'human' | 'doram';
  /** Set for looks that exist for one sex only. */
  sex?: 'm' | 'f';
}

export interface Job {
  id: number;
  constant: string;
  look: string;
  mount?: string;
}

function isJobLook(v: unknown): v is JobLook & { id: string } {
  return (
    isObject(v) &&
    typeof v.look === 'string' &&
    (v.race === 'human' || v.race === 'doram') &&
    (v.sex === undefined || v.sex === 'm' || v.sex === 'f')
  );
}

function isJob(v: unknown): v is Job {
  return isObject(v) && typeof v.id === 'number' && typeof v.constant === 'string' && typeof v.look === 'string';
}

function looksTable(data: unknown): ReadonlyMap<string, JobLook> {
  const list = isObject(data) ? data.looks : undefined;
  if (!Array.isArray(list) || !list.every(isJobLook)) {
    throw new Error('jobs.yml: invalid looks');
  }
  return new Map(list.map((l) => [l.look, l]));
}

/** Job looks by name, and jobs by rAthena job ID. */
export const looks = looksTable(jobsTable);
export const jobs = table('jobs.yml', jobsTable, 'jobs', isJob);

/** Weapon types by rAthena ID. */
export const weapons = table('weapons.yml', weaponsTable, 'weapons', isWeapon);

/** Effects by RO effect ID. */
export const effects = table('effects.yml', effectsTable, 'effects', isEffect);

export interface NpcSprite {
  id: number;
  /** The sprite's asset name is sprite/npc/<sprite>. */
  sprite: string;
}

function isNpcSprite(v: unknown): v is NpcSprite {
  return isObject(v) && typeof v.id === 'number' && typeof v.sprite === 'string';
}

/** NPC sprites by the ID the server sends as an NPC's Look.body. */
export const npcSprites = table('npc_sprites.yml', npcSpritesTable, 'sprites', isNpcSprite);
