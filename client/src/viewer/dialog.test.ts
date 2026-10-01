import { describe, expect, it } from 'vitest';
import { dialogRuns, parseNumber } from './dialog';

describe('dialogRuns', () => {
  it('keeps plain text as one run', () => {
    expect(dialogRuns('[Kafra Employee]')).toEqual([
      { text: '[Kafra Employee]', color: undefined },
    ]);
  });

  it('splits at color codes', () => {
    expect(dialogRuns('Welcome. ^ff0000Prontera^000000 is here.')).toEqual([
      { text: 'Welcome. ', color: undefined },
      { text: 'Prontera', color: '#ff0000' },
      { text: ' is here.', color: '#000000' },
    ]);
  });

  it('reads item links as runs that name their item', () => {
    expect(
      dialogRuns('Bring ^0000FF<ITEMLINK>Red Potion<INFO>501</INFO></ITEMLINK>^000000 to me.'),
    ).toEqual([
      { text: 'Bring ', color: undefined },
      { text: 'Red Potion', color: '#0000ff', item: 501 },
      { text: ' to me.', color: '#000000' },
    ]);
  });

  it('keeps markup as text', () => {
    expect(dialogRuns('<b>bold</b> <ITEMLINK>x</ITEMLINK>')).toEqual([
      { text: '<b>bold</b> <ITEMLINK>x</ITEMLINK>', color: undefined },
    ]);
  });
});

describe('parseNumber', () => {
  it('accepts whole numbers in the range', () => {
    expect(parseNumber(' 7 ', 1n, 10n)).toBe(7n);
    expect(parseNumber('-3', -5n, 0n)).toBe(-3n);
  });

  it('refuses the rest', () => {
    expect(parseNumber('11', 1n, 10n)).toBeUndefined();
    expect(parseNumber('1.5', 1n, 10n)).toBeUndefined();
    expect(parseNumber('', 1n, 10n)).toBeUndefined();
    expect(parseNumber('1e3', 0n, 10000n)).toBeUndefined();
  });
});
