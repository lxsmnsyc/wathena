// Skill cooldowns and cast bars, timed with a clock such as
// performance.now(). The server sends SkillCooldown for each skill that
// cannot be used for a time, and skill ID 0 for the delay after a cast,
// which holds back every skill. SkillCast starts a cast bar over the
// caster, and SkillCastCancel ends it early.

/** The skill ID of the delay after a cast, which applies to every skill. */
export const ALL_SKILLS = 0;

/** When each skill can be used again. */
export class Cooldowns {
  private readonly ends = new Map<number, number>();

  /** Starts a cooldown of ms for a skill, or for every skill with ALL_SKILLS. */
  set(skillId: number, ms: number, now: number): void {
    if (ms <= 0) {
      this.ends.delete(skillId);
      return;
    }
    this.ends.set(skillId, now + ms);
  }

  /** The milliseconds left before a skill can be used, 0 when it can. */
  remaining(skillId: number, now: number): number {
    const own = this.ends.get(skillId) ?? 0;
    const all = this.ends.get(ALL_SKILLS) ?? 0;
    return Math.max(0, own - now, all - now);
  }

  clear(): void {
    this.ends.clear();
  }
}

/**
 * The time left as RO shows it on a skill: tenths of a second under 10
 * seconds, whole seconds rounded up from there, and nothing at 0.
 */
export function cooldownText(ms: number): string {
  if (ms <= 0) {
    return '';
  }
  if (ms < 10_000) {
    return (Math.ceil(ms / 100) / 10).toFixed(1);
  }
  return String(Math.ceil(ms / 1000));
}

/** The cast bars shown over casting entities, by entity ID. */
export class CastBars {
  private readonly bars = new Map<number, { start: number; ms: number }>();

  /** Starts a bar of ms over an entity, replacing its last one. */
  start(id: number, ms: number, now: number): void {
    if (ms > 0) {
      this.bars.set(id, { start: now, ms });
    }
  }

  /** Ends an entity's bar before its time. */
  cancel(id: number): void {
    this.bars.delete(id);
  }

  clear(): void {
    this.bars.clear();
  }

  /** The bars still shown at now, with how full each is, from 0 to 1. */
  active(now: number): Map<number, number> {
    const out = new Map<number, number>();
    for (const [id, b] of this.bars) {
      if (now - b.start < b.ms) {
        out.set(id, Math.max(0, (now - b.start) / b.ms));
      }
    }
    return out;
  }

  /** Forgets the bars that ended by now, and returns their IDs. */
  expire(now: number): number[] {
    const gone: number[] = [];
    for (const [id, b] of this.bars) {
      if (now - b.start >= b.ms) {
        gone.push(id);
        this.bars.delete(id);
      }
    }
    return gone;
  }
}
