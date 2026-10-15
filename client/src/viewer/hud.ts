import { modJobName } from '../assets/art';
import { jobs } from '../data/tables';
import { PickUpFailure, StatId } from '../gen/wathena/v1/event_pb';

// The player's status on the page: HP, levels, and exp.

/**
 * Exp toward the next level as a percent with one decimal, rounded down,
 * as RO shows it. next is 0 at the maximum level, which shows "max".
 */
export function expPercent(exp: bigint, next: bigint): string {
  if (next <= 0n) {
    return 'max';
  }
  let tenths = (exp * 1000n) / next;
  if (tenths < 0n) {
    tenths = 0n;
  }
  if (tenths > 1000n) {
    tenths = 1000n;
  }
  return `${(Number(tenths) / 10).toFixed(1)}%`;
}

/** The HP line, or undefined until the server sends HP. */
export function hpLine(stats: ReadonlyMap<StatId, bigint>): string | undefined {
  const hp = stats.get(StatId.HP);
  const max = stats.get(StatId.MAX_HP);
  if (hp === undefined || max === undefined) {
    return undefined;
  }
  return `HP ${hp < 0n ? 0n : hp} / ${max}`;
}

/** Job names that do not follow from their constants. */
const JOB_NAMES = new Map<string, string>([
  ['JOB_MAGE', 'Mage'],
  ['JOB_NOVICE_HIGH', 'High Novice'],
  ['JOB_SUPER_NOVICE', 'Super Novice'],
]);

/**
 * A job's name from its rAthena constant: JOB_LORD_KNIGHT is Lord Knight,
 * JOB_SWORDMAN_HIGH is High Swordman, and the mounted JOB_KNIGHT2 is
 * Knight. A mod's job has the name the server's client data gives it.
 * Unknown jobs show their ID.
 */
export function jobName(id: number): string {
  const named = modJobName(id);
  if (named !== undefined) {
    return named;
  }
  const constant = jobs.get(id)?.constant;
  if (constant === undefined) {
    return `Job ${id}`;
  }
  const known = JOB_NAMES.get(constant);
  if (known !== undefined) {
    return known;
  }
  let words = constant.replace(/^JOB_/, '').replace(/2$/, '').toLowerCase().split('_');
  if (words.length > 1 && words[words.length - 1] === 'high') {
    words = ['high', ...words.slice(0, -1)];
  }
  return words.map((w) => w.charAt(0).toUpperCase() + w.slice(1)).join(' ');
}

/**
 * The levels and exp line, or undefined until the server sends levels.
 * names holds the job names the server sent, which win over the names
 * made from the client tables, since mods add jobs the tables lack.
 */
export function expLine(
  stats: ReadonlyMap<StatId, bigint>,
  names: ReadonlyMap<number, string> = new Map(),
): string | undefined {
  const level = (name: string, lv: StatId, exp: StatId, next: StatId): string | undefined => {
    const l = stats.get(lv);
    if (l === undefined) {
      return undefined;
    }
    const e = stats.get(exp);
    const n = stats.get(next);
    return e === undefined || n === undefined
      ? `${name} ${l}`
      : `${name} ${l} (${expPercent(e, n)})`;
  };
  const base = level('Base Lv', StatId.BASE_LEVEL, StatId.BASE_EXP, StatId.NEXT_BASE_EXP);
  const job = level('Job Lv', StatId.JOB_LEVEL, StatId.JOB_EXP, StatId.NEXT_JOB_EXP);
  const parts = [base, job].filter((p) => p !== undefined);
  if (parts.length === 0) {
    return undefined;
  }
  const id = stats.get(StatId.JOB);
  const line = parts.join(', ');
  if (id === undefined) {
    return line;
  }
  const name = names.get(Number(id)) ?? jobName(Number(id));
  return `${name}: ${line}`;
}

/** What to tell the player when a pick up fails. */
export function pickUpMessage(reason: PickUpFailure): string {
  switch (reason) {
    case PickUpFailure.NOT_YOURS:
      return 'You cannot pick up this item yet.';
    case PickUpFailure.OVERWEIGHT:
      return 'You are carrying too much to pick this up.';
    case PickUpFailure.INVENTORY_FULL:
      return 'Your inventory is full.';
    case PickUpFailure.TOO_MANY:
      return 'You cannot carry more of this item.';
    default:
      return 'The item is no longer there.';
  }
}
