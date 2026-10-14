// The client data tables in data/ (strategy section 10.5). The Go package
// in data/ checks them, and the build imports them as JSON.

import effectsTable from '../../data/effects.yml';
import jobsTable from '../../data/jobs.yml';
import skillsTable from '../../data/skills.yml';
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
): Map<number, T> {
  const list = isObject(data) ? data[key] : undefined;
  if (!Array.isArray(list) || !list.every(check)) {
    throw new Error(`${name}: invalid table`);
  }
  return new Map(list.map((v) => [v.id, v]));
}

/** Replaces a map's entries with another's. */
function replace<K, V>(into: Map<K, V>, from: Map<K, V>): void {
  into.clear();
  for (const [k, v] of from) {
    into.set(k, v);
  }
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

function looksTable(data: unknown): Map<string, JobLook> {
  const list = isObject(data) ? data.looks : undefined;
  if (!Array.isArray(list) || !list.every(isJobLook)) {
    throw new Error('jobs.yml: invalid looks');
  }
  return new Map(list.map((l) => [l.look, l]));
}

const looksMap = looksTable(jobsTable);
const jobsMap = table('jobs.yml', jobsTable, 'jobs', isJob);
const weaponsMap = table('weapons.yml', weaponsTable, 'weapons', isWeapon);
const effectsMap = table('effects.yml', effectsTable, 'effects', isEffect);

/** Job looks by name, and jobs by rAthena job ID. */
export const looks: ReadonlyMap<string, JobLook> = looksMap;
export const jobs: ReadonlyMap<number, Job> = jobsMap;

/** Weapon types by rAthena ID. */
export const weapons: ReadonlyMap<number, Weapon> = weaponsMap;

/** Effects by RO effect ID. */
export const effects: ReadonlyMap<number, EffectEntry> = effectsMap;

/**
 * The effects a skill plays, as RO effect IDs. data/skills.yml explains
 * when each list plays.
 */
export interface SkillEffects {
  id: number;
  constant: string;
  caster?: number[];
  effect?: number[];
  hit?: number[];
  /** False for skills that show no cast aura. */
  aura?: boolean;
}

/** A skill unit's effect by UNT_ ID. Without one, the unit plays its skill's effect. */
export interface UnitEffect {
  id: number;
  constant: string;
  effect?: number;
}

/** The cast aura of an element. */
export interface Aura {
  id: number;
  constant: string;
  effect: number;
}

function numbers(v: unknown): boolean {
  return v === undefined || (Array.isArray(v) && v.every((n) => typeof n === 'number'));
}

function isSkillEffects(v: unknown): v is SkillEffects {
  return (
    isObject(v) &&
    typeof v.id === 'number' &&
    typeof v.constant === 'string' &&
    numbers(v.caster) &&
    numbers(v.effect) &&
    numbers(v.hit) &&
    (v.aura === undefined || typeof v.aura === 'boolean')
  );
}

function isUnitEffect(v: unknown): v is UnitEffect {
  return (
    isObject(v) &&
    typeof v.id === 'number' &&
    typeof v.constant === 'string' &&
    (v.effect === undefined || typeof v.effect === 'number')
  );
}

function isAura(v: unknown): v is Aura {
  return (
    isObject(v) &&
    typeof v.id === 'number' &&
    typeof v.constant === 'string' &&
    typeof v.effect === 'number'
  );
}

const skillEffectsMap = table('skills.yml', skillsTable, 'skills', isSkillEffects);
const unitEffectsMap = table('skills.yml', skillsTable, 'units', isUnitEffect);
const aurasMap = table('skills.yml', skillsTable, 'auras', isAura);

/** Skill effects by rAthena skill ID. */
export const skillEffects: ReadonlyMap<number, SkillEffects> = skillEffectsMap;

/** Skill unit effects by rAthena UNT_ ID. */
export const unitEffects: ReadonlyMap<number, UnitEffect> = unitEffectsMap;

/** Cast auras by rAthena ELE_ element. */
export const auras: ReadonlyMap<number, Aura> = aurasMap;

/**
 * Replaces the tables with the server's, which are core's merged with
 * the mods' client/*.yml tables (decision 0033), keyed by file name, such
 * as jobs.yml. A table the server does not send stays as built in. Every
 * table is checked before any is replaced, so a bad table changes
 * nothing.
 */
export function applyTables(tables: unknown): void {
  if (!isObject(tables)) {
    throw new Error('client data tables: invalid');
  }
  const steps: (() => void)[] = [];
  const jobsData = tables['jobs.yml'];
  if (jobsData !== undefined) {
    const l = looksTable(jobsData);
    const j = table('jobs.yml', jobsData, 'jobs', isJob);
    steps.push(() => {
      replace(looksMap, l);
      replace(jobsMap, j);
    });
  }
  if (tables['weapons.yml'] !== undefined) {
    const w = table('weapons.yml', tables['weapons.yml'], 'weapons', isWeapon);
    steps.push(() => {
      replace(weaponsMap, w);
    });
  }
  if (tables['effects.yml'] !== undefined) {
    const e = table('effects.yml', tables['effects.yml'], 'effects', isEffect);
    steps.push(() => {
      replace(effectsMap, e);
    });
  }
  const skillsData = tables['skills.yml'];
  if (skillsData !== undefined) {
    const s = table('skills.yml', skillsData, 'skills', isSkillEffects);
    const u = table('skills.yml', skillsData, 'units', isUnitEffect);
    const a = table('skills.yml', skillsData, 'auras', isAura);
    steps.push(() => {
      replace(skillEffectsMap, s);
      replace(unitEffectsMap, u);
      replace(aurasMap, a);
    });
  }
  for (const step of steps) {
    step();
  }
}
