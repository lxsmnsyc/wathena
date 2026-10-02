import { describe, expect, it } from 'vitest';
import { auras, effects, skillEffects, unitEffects } from '../data/tables';
import {
  HEAL_SKILLS,
  Recent,
  castAura,
  skillEffectIds,
  splitHits,
  unitEffect,
} from './skillEffects';

describe('skill effect lookup', () => {
  it('finds the effects of each moment', () => {
    expect(skillEffectIds(5, 'caster')).toEqual([16]);
    expect(skillEffectIds(5, 'hit')).toEqual([1]);
    expect(skillEffectIds(5, 'effect')).toEqual([]);
    expect(skillEffectIds(28, 'effect')).toEqual([312]);
    expect(skillEffectIds(20, 'effect')).toEqual([29]);
    expect(skillEffectIds(99999, 'hit')).toEqual([]);
  });

  it('lists every first class skill', () => {
    const ids = [
      5, 6, 7, 8, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 24, 25, 26, 27, 28, 29, 30, 31,
      32, 33, 34, 35, 42, 45, 46, 47, 51, 52, 53, 142, 143, 146, 148, 149, 150, 152, 153, 155, 156,
      157,
    ];
    for (const id of ids) {
      expect(skillEffects.has(id), String(id)).toBe(true);
    }
  });

  it('refers only to effects in effects.yml', () => {
    const used = [
      ...[...skillEffects.values()].flatMap((s) => [
        ...(s.caster ?? []),
        ...(s.effect ?? []),
        ...(s.hit ?? []),
      ]),
      ...[...unitEffects.values()].flatMap((u) => (u.effect === undefined ? [] : [u.effect])),
      ...[...auras.values()].map((a) => a.effect),
    ];
    for (const id of used) {
      expect(effects.has(id), String(id)).toBe(true);
    }
  });

  it('picks the cast aura by element', () => {
    expect(castAura(14, 1)).toBe(54);
    expect(castAura(19, 3)).toBe(55);
    expect(castAura(28, 6)).toBe(58);
    expect(castAura(5, 0)).toBe(12);
    // An unknown element shows the neutral aura.
    expect(castAura(5, 42)).toBe(12);
    // Arrow Repel shows none.
    expect(castAura(148, 0)).toBeUndefined();
  });

  it('shows ground units by UNT_ ID', () => {
    expect(unitEffect(0x7e, 12)).toEqual({ stay: 315 });
    expect(unitEffect(0x7f, 18)).toEqual({ stay: 25 });
    expect(unitEffect(0x80, 27)).toEqual({ stay: 316 });
    expect(unitEffect(0x81, 27)).toEqual({ stay: 317 });
    expect(unitEffect(0x85, 25)).toEqual({ stay: 141 });
    // Thunder Storm's unit plays its skill's effect once.
    expect(unitEffect(0x86, 21)).toEqual({ once: [30] });
    expect(unitEffect(0x99, 21)).toBeUndefined();
  });

  it('shows heal numbers for Heal and First Aid', () => {
    expect(HEAL_SKILLS.has(28)).toBe(true);
    expect(HEAL_SKILLS.has(142)).toBe(true);
    expect(HEAL_SKILLS.has(5)).toBe(false);
  });
});

describe('splitHits', () => {
  it('shows equal shares that add up to the damage', () => {
    expect(splitHits(100, 1)).toEqual([100]);
    expect(splitHits(100, 0)).toEqual([100]);
    expect(splitHits(100, 2)).toEqual([50, 50]);
    expect(splitHits(100, 3)).toEqual([33, 33, 34]);
    expect(splitHits(0, 2)).toEqual([0, 0]);
  });
});

describe('Recent', () => {
  it('reports a key seen within the window', () => {
    const r = new Recent();
    expect(r.check('a', 0, 100)).toBe(false);
    expect(r.check('a', 50, 100)).toBe(true);
    expect(r.check('b', 50, 100)).toBe(false);
    expect(r.check('a', 150, 100)).toBe(false);
    expect(r.check('a', 200, 100)).toBe(true);
  });
});
