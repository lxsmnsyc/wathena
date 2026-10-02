import { describe, expect, it } from 'vitest';
import { StatusIconList, remainingText } from './statusIcons';

describe('remainingText', () => {
  it('shows seconds under a minute, rounded up', () => {
    expect(remainingText(59_001)).toBe('60s');
    expect(remainingText(1500)).toBe('2s');
    expect(remainingText(0)).toBe('0s');
    expect(remainingText(-20)).toBe('0s');
  });

  it('shows minutes from a minute up, rounded up', () => {
    expect(remainingText(60_000)).toBe('1m');
    expect(remainingText(90_000)).toBe('2m');
    expect(remainingText(240_000)).toBe('4m');
  });
});

describe('StatusIconList', () => {
  it('counts down icons that end, and not those that do not', () => {
    const l = new StatusIconList();
    l.set(10, true, 5000, 1000);
    l.set(20, true, -1, 1000);
    expect(l.shown(1000)).toEqual([
      { icon: 10, left: '5s' },
      { icon: 20, left: undefined },
    ]);
    expect(l.shown(3500)).toEqual([
      { icon: 10, left: '3s' },
      { icon: 20, left: undefined },
    ]);
  });

  it('replaces an icon with the same ID, and hides it when it goes', () => {
    const l = new StatusIconList();
    l.set(10, true, 5000, 0);
    l.set(20, true, 1000, 0);
    l.set(10, true, 120_000, 0);
    expect(l.shown(0)).toEqual([
      { icon: 10, left: '2m' },
      { icon: 20, left: '1s' },
    ]);
    l.set(10, false, 0, 10);
    expect(l.shown(10)).toEqual([{ icon: 20, left: '1s' }]);
    l.set(99, false, 0, 10);
    expect(l.shown(10)).toHaveLength(1);
  });

  it('clears every icon', () => {
    const l = new StatusIconList();
    l.set(10, true, -1, 0);
    l.clear();
    expect(l.shown(0)).toEqual([]);
  });
});
