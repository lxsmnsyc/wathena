// The client data tables in data/ (strategy section 10.5). The Go package
// in data/ checks them, and the build imports them as JSON.

import effectsTable from '../../data/effects.yml';
import jobsTable from '../../data/jobs.yml';
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

/** One thing an effect shows: an STR effect, or one of several at random. */
export type EffectPart = { effect: string } | { variants: string[] };

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

function isPart(v: unknown): v is EffectPart {
  return isObject(v) && (typeof v.effect === 'string' || strings(v.variants));
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
