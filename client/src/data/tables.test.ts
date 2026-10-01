import { describe, expect, it } from 'vitest';
import { effects, partEffect, weapons } from './tables';

describe('tables', () => {
  it('loads the YAML tables', () => {
    expect(weapons.get(11)?.constant).toBe('W_BOW');
    expect(effects.get(109)?.parts[0]).toEqual({
      variants: ['effect/bubble1', 'effect/bubble2', 'effect/bubble3', 'effect/bubble4'],
    });
  });

  it('lists level up effects with their sound', () => {
    expect(effects.get(371)?.parts).toEqual([{ effect: 'effect/angel' }, { sound: 'sound/levelup' }]);
    expect(effects.get(158)?.parts).toEqual([{ effect: 'effect/joblvup' }]);
  });

  it('lists the warp portal as two cylinders', () => {
    const parts = effects.get(321)?.parts ?? [];
    expect(parts).toHaveLength(2);
    for (const p of parts) {
      expect('cylinder' in p && p.cylinder.texture).toBe('texture/effect/ring_blue');
      expect('cylinder' in p && p.cylinder.repeat).toBe(true);
    }
  });

  it('names the effect a part shows', () => {
    expect(partEffect({ effect: 'effect/a' }, () => 0)).toBe('effect/a');
    expect(partEffect({ variants: ['effect/a', 'effect/b'] }, () => 1)).toBe('effect/b');
    expect(partEffect({ sound: 'sound/a' }, () => 0)).toBeUndefined();
    const cylinder = effects.get(321)?.parts[0];
    expect(cylinder === undefined ? 'none' : partEffect(cylinder, () => 0)).toBeUndefined();
  });
});
