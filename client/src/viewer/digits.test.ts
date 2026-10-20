import { describe, expect, it } from 'vitest';
import { DigitsKind } from '../gen/wathena/v1/event_pb';
import { clockText, digitsText } from './digits';

describe('clockText', () => {
  it('leaves out the parts on the left that are 0', () => {
    expect(clockText(5)).toBe('05');
    expect(clockText(65)).toBe('01:05');
    expect(clockText(3661)).toBe('01:01:01');
    expect(clockText(90061)).toBe('01:01:01:01');
  });
});

describe('digitsText', () => {
  it('shows a value for 5 seconds', () => {
    expect(digitsText(DigitsKind.UNSPECIFIED, 30, 0)).toBe('30');
    expect(digitsText(DigitsKind.UNSPECIFIED, 30, 4999)).toBe('30');
    expect(digitsText(DigitsKind.UNSPECIFIED, 30, 5000)).toBeUndefined();
  });

  it('counts up from the negated value', () => {
    expect(digitsText(DigitsKind.COUNT_UP, -10, 0)).toBe('10');
    expect(digitsText(DigitsKind.COUNT_UP, -10, 2500)).toBe('12');
  });

  it('counts down from the negated value and goes at 0', () => {
    expect(digitsText(DigitsKind.COUNT_DOWN, -70, 0)).toBe('01:10');
    expect(digitsText(DigitsKind.COUNT_DOWN, -2, 2000)).toBe('00');
    expect(digitsText(DigitsKind.COUNT_DOWN, -2, 3000)).toBeUndefined();
  });

  it('counts two digits down to 0 and stops there', () => {
    expect(digitsText(DigitsKind.COUNT_DOWN_TO_ZERO, 99, 0)).toBe('99');
    expect(digitsText(DigitsKind.COUNT_DOWN_TO_ZERO, 3, 3000)).toBe('00');
    expect(digitsText(DigitsKind.COUNT_DOWN_TO_ZERO, 3, 4000)).toBe('00');
    expect(digitsText(DigitsKind.COUNT_DOWN_TO_ZERO, 3, 5000)).toBeUndefined();
  });
});
