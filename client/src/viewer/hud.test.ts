import { describe, expect, it } from 'vitest';
import { PickUpFailure, StatId } from '../gen/wathena/v1/event_pb';
import { expLine, expPercent, hpLine, pickUpMessage } from './hud';

describe('expPercent', () => {
  it('rounds down to a tenth of a percent', () => {
    expect(expPercent(0n, 100n)).toBe('0.0%');
    expect(expPercent(1n, 3n)).toBe('33.3%');
    expect(expPercent(2n, 3n)).toBe('66.6%');
    expect(expPercent(999n, 1000n)).toBe('99.9%');
  });

  it('shows max at the maximum level', () => {
    expect(expPercent(5n, 0n)).toBe('max');
  });

  it('stays between 0 and 100 percent', () => {
    expect(expPercent(200n, 100n)).toBe('100.0%');
    expect(expPercent(-1n, 100n)).toBe('0.0%');
  });

  it('handles exp beyond 2^53', () => {
    expect(expPercent(2n ** 60n, 2n ** 61n)).toBe('50.0%');
  });
});

describe('status lines', () => {
  it('waits for the server', () => {
    expect(hpLine(new Map())).toBeUndefined();
    expect(expLine(new Map())).toBeUndefined();
  });

  it('shows HP, levels, and exp', () => {
    const stats = new Map<StatId, bigint>([
      [StatId.HP, -3n],
      [StatId.MAX_HP, 40n],
      [StatId.BASE_LEVEL, 12n],
      [StatId.BASE_EXP, 345n],
      [StatId.NEXT_BASE_EXP, 1000n],
      [StatId.JOB_LEVEL, 50n],
      [StatId.JOB_EXP, 0n],
      [StatId.NEXT_JOB_EXP, 0n],
    ]);
    expect(hpLine(stats)).toBe('HP 0 / 40');
    expect(expLine(stats)).toBe('Base Lv 12 (34.5%), Job Lv 50 (max)');
  });

  it('shows levels before exp arrives', () => {
    expect(expLine(new Map([[StatId.BASE_LEVEL, 1n]]))).toBe('Base Lv 1');
  });
});

describe('pickUpMessage', () => {
  it('explains why a pick up failed', () => {
    expect(pickUpMessage(PickUpFailure.NOT_YOURS)).toBe('You cannot pick up this item yet.');
    expect(pickUpMessage(PickUpFailure.GONE)).toBe('The item is no longer there.');
    expect(pickUpMessage(PickUpFailure.OVERWEIGHT)).toBe(
      'You are carrying too much to pick this up.',
    );
    expect(pickUpMessage(PickUpFailure.INVENTORY_FULL)).toBe('Your inventory is full.');
    expect(pickUpMessage(PickUpFailure.TOO_MANY)).toBe('You cannot carry more of this item.');
  });
});
