import { describe, expect, it } from 'vitest';
import { ALL_SKILLS, CastBars, Cooldowns, cooldownText } from './cooldowns';

describe('Cooldowns', () => {
  it('counts down a skill until it can be used', () => {
    const c = new Cooldowns();
    c.set(5, 2000, 1000);
    expect(c.remaining(5, 1000)).toBe(2000);
    expect(c.remaining(5, 2500)).toBe(500);
    expect(c.remaining(5, 3000)).toBe(0);
    expect(c.remaining(6, 1000)).toBe(0);
  });

  it('holds back every skill during the delay after a cast', () => {
    const c = new Cooldowns();
    c.set(ALL_SKILLS, 1000, 0);
    c.set(5, 300, 0);
    expect(c.remaining(5, 0)).toBe(1000);
    expect(c.remaining(99, 500)).toBe(500);
    // A longer cooldown of one skill outlasts the delay.
    c.set(7, 5000, 0);
    expect(c.remaining(7, 2000)).toBe(3000);
  });

  it('ends a cooldown of 0 and clears', () => {
    const c = new Cooldowns();
    c.set(5, 2000, 0);
    c.set(5, 0, 10);
    expect(c.remaining(5, 10)).toBe(0);
    c.set(ALL_SKILLS, 2000, 0);
    c.clear();
    expect(c.remaining(5, 0)).toBe(0);
  });
});

describe('cooldownText', () => {
  it('shows tenths under 10 seconds and whole seconds above', () => {
    expect(cooldownText(0)).toBe('');
    expect(cooldownText(-5)).toBe('');
    expect(cooldownText(1)).toBe('0.1');
    expect(cooldownText(1450)).toBe('1.5');
    expect(cooldownText(9999)).toBe('10.0');
    expect(cooldownText(10_000)).toBe('10');
    expect(cooldownText(12_001)).toBe('13');
  });
});

describe('CastBars', () => {
  it('fills a bar over its cast time and removes it at the end', () => {
    const b = new CastBars();
    b.start(7, 2000, 1000);
    expect(b.active(1000)).toEqual(new Map([[7, 0]]));
    expect(b.active(1500)).toEqual(new Map([[7, 0.25]]));
    expect(b.expire(2999)).toEqual([]);
    expect(b.active(3000)).toEqual(new Map());
    expect(b.expire(3000)).toEqual([7]);
  });

  it('cancels a bar early and replaces a caster bar', () => {
    const b = new CastBars();
    b.start(7, 2000, 0);
    b.start(8, 2000, 0);
    b.cancel(7);
    expect([...b.active(100).keys()]).toEqual([8]);
    b.start(8, 1000, 1000);
    expect(b.active(1500).get(8)).toBe(0.5);
    // A cast without a cast time shows no bar.
    b.start(9, 0, 0);
    expect(b.active(0).has(9)).toBe(false);
    b.clear();
    expect(b.active(1500).size).toBe(0);
  });
});
