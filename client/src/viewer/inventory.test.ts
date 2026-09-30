import { describe, expect, it } from 'vitest';
import { DropItemFailure, StatId, UseItemFailure } from '../gen/wathena/v1/event_pb';
import {
  InventoryModel,
  USE_EFFECTS,
  dropItemMessage,
  itemLabel,
  parseDropAmount,
  useItemMessage,
  weightPercent,
  weightText,
} from './inventory';

describe('InventoryModel', () => {
  it('follows the server', () => {
    const m = new InventoryModel();
    m.reset(
      [
        { slot: 3, itemId: 909, amount: 2 },
        { slot: 0, itemId: 501, amount: 5 },
      ],
      100,
    );
    expect(m.size).toBe(100);
    expect(m.list().map((s) => s.slot)).toEqual([0, 3]);
    m.add(0, 501, 2);
    m.add(1, 1201, 1);
    expect(m.get(0)?.amount).toBe(7);
    m.remove(0, 7);
    m.remove(3, 1);
    m.remove(9, 1);
    expect(m.list()).toEqual([
      { slot: 1, itemId: 1201, amount: 1 },
      { slot: 3, itemId: 909, amount: 1 },
    ]);
    // A list replaces everything.
    m.reset([], 100);
    expect(m.list()).toEqual([]);
  });

  it('replaces a slot that held another item', () => {
    const m = new InventoryModel();
    m.add(0, 501, 1);
    m.remove(0, 1);
    m.add(0, 909, 3);
    expect(m.get(0)).toEqual({ slot: 0, itemId: 909, amount: 3 });
  });
});

describe('weight', () => {
  it('shows whole units and a rounded down percent', () => {
    const stats = new Map<StatId, bigint>([
      [StatId.WEIGHT, 1799n],
      [StatId.MAX_WEIGHT, 2000n],
    ]);
    expect(weightText(stats)).toBe('179 / 200');
    expect(weightPercent(stats)).toBe(89);
  });

  it('waits for the server', () => {
    expect(weightText(new Map())).toBe('- / -');
    expect(weightPercent(new Map())).toBeUndefined();
  });
});

describe('parseDropAmount', () => {
  it('takes whole numbers from 1 to the stack', () => {
    expect(parseDropAmount('3', 5)).toBe(3);
    expect(parseDropAmount(' 5 ', 5)).toBe(5);
    expect(parseDropAmount('0', 5)).toBeUndefined();
    expect(parseDropAmount('6', 5)).toBeUndefined();
    expect(parseDropAmount('2.5', 5)).toBeUndefined();
    expect(parseDropAmount('-1', 5)).toBeUndefined();
    expect(parseDropAmount('', 5)).toBeUndefined();
  });
});

describe('messages', () => {
  it('explains failures', () => {
    expect(useItemMessage(UseItemFailure.NOT_USABLE)).toBe('This item cannot be used.');
    expect(useItemMessage(UseItemFailure.TOO_SOON)).toBe('You are using items too fast.');
    expect(dropItemMessage(DropItemFailure.CANNOT_DROP)).toBe('This item cannot be dropped.');
    expect(dropItemMessage(DropItemFailure.UNSPECIFIED)).toBe('The item was not dropped.');
  });

  it('labels items by ID', () => {
    expect(itemLabel(501)).toBe('Item 501');
  });
});

describe('USE_EFFECTS', () => {
  it('plays the potion effect of the potion color', () => {
    expect(USE_EFFECTS.get(501)).toBe(204);
    expect(USE_EFFECTS.get(505)).toBe(208);
    expect(USE_EFFECTS.get(512)).toBeUndefined();
  });
});
