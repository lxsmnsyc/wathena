import { auras, skillEffects, unitEffects } from '../data/tables';

// Which effects a skill plays, from data/skills.yml, and the timing of
// multi-hit damage numbers.

/** AL_HEAL and NV_FIRSTAID, whose SkillEffect value shows as a heal number. */
export const HEAL_SKILLS: ReadonlySet<number> = new Set([28, 142]);

/** Milliseconds between the numbers of a multi-hit skill, as roBrowser. */
export const HIT_GAP_MS = 200;

/** The moments a skill plays effects at. data/skills.yml explains them. */
export type SkillPhase = 'caster' | 'effect' | 'hit';

/** The RO effect IDs a skill plays at a moment. Unknown skills play none. */
export function skillEffectIds(skillId: number, phase: SkillPhase): readonly number[] {
  return skillEffects.get(skillId)?.[phase] ?? [];
}

/** The cast aura for an element, or undefined for skills that show none. */
export function castAura(skillId: number, element: number): number | undefined {
  if (skillEffects.get(skillId)?.aura === false) {
    return undefined;
  }
  return (auras.get(element) ?? auras.get(0))?.effect;
}

/**
 * What a skill unit shows: an effect that stays while the unit does, or
 * its skill's one-time effects, or nothing for units the table lacks.
 */
export function unitEffect(
  unitId: number,
  skillId: number,
): { stay: number } | { once: readonly number[] } | undefined {
  const u = unitEffects.get(unitId);
  if (u === undefined) {
    return undefined;
  }
  if (u.effect !== undefined) {
    return { stay: u.effect };
  }
  return { once: skillEffectIds(skillId, 'effect') };
}

/**
 * Splits damage into the numbers of its hits, as RO shows a multi-hit
 * skill: each hit shows an equal share, and the last one takes what is
 * left, so the numbers add up to the damage.
 */
export function splitHits(amount: number, hits: number): number[] {
  const n = Math.max(1, Math.floor(hits));
  if (n === 1) {
    return [amount];
  }
  const each = Math.floor(amount / n);
  const out = Array.from({ length: n }, () => each);
  out[n - 1] = amount - each * (n - 1);
  return out;
}

/**
 * Remembers keys for a short time, so an effect that two events ask for
 * plays once. Magnum Break hits each target with its own Damage, and its
 * caster effect plays once. Thunder Storm sends both a SkillEffect at its
 * cell and a skill unit there.
 */
export class Recent {
  private readonly seen = new Map<string, number>();

  /**
   * Whether key was marked within windowMs before now. Marks it when it
   * was not.
   */
  check(key: string, now: number, windowMs: number): boolean {
    const last = this.seen.get(key);
    if (last !== undefined && now - last < windowMs) {
      return true;
    }
    this.seen.set(key, now);
    // Old keys go, so the map stays small.
    if (this.seen.size > 256) {
      for (const [k, t] of this.seen) {
        if (now - t >= 10_000) {
          this.seen.delete(k);
        }
      }
    }
    return false;
  }
}
