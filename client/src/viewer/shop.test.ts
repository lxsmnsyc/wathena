import { describe, expect, it } from 'vitest';
import { ShopOutcome, StatId } from '../gen/wathena/v1/event_pb';
import { buyProblem, parseAmount, shopMessage, shopTotals } from './shop';

const stats = new Map<StatId, bigint>([
  [StatId.ZENY, 1000n],
  [StatId.WEIGHT, 500n],
  [StatId.MAX_WEIGHT, 1000n],
]);

describe('shopTotals', () => {
  it('sums zeny and weight', () => {
    expect(
      shopTotals([
        { key: 501, amount: 3, price: 50, weight: 70 },
        { key: 1201, amount: 1, price: 50, weight: 400 },
      ]),
    ).toEqual({ zeny: 200, weight: 610 });
  });
});

describe('buyProblem', () => {
  it('lets a purchase the player can pay and carry go', () => {
    expect(buyProblem([{ key: 501, amount: 3, price: 50, weight: 70 }], stats)).toBeUndefined();
  });

  it('refuses one over the zeny or the weight', () => {
    expect(buyProblem([{ key: 501, amount: 21, price: 50, weight: 1 }], stats)).toBe(
      'You do not have enough zeny.',
    );
    expect(buyProblem([{ key: 501, amount: 8, price: 1, weight: 70 }], stats)).toBe(
      'You cannot carry that much.',
    );
    expect(buyProblem([], stats)).toBe('Choose what to buy.');
  });
});

describe('parseAmount', () => {
  it('reads whole numbers up to the maximum, and empty as 0', () => {
    expect(parseAmount('', 5)).toBe(0);
    expect(parseAmount('5', 5)).toBe(5);
    expect(parseAmount('6', 5)).toBeUndefined();
    expect(parseAmount('-1', 5)).toBeUndefined();
  });
});

describe('shopMessage', () => {
  it('names each outcome the player should hear of', () => {
    expect(shopMessage(ShopOutcome.BOUGHT)).toBe('Thank you for your purchase.');
    expect(shopMessage(ShopOutcome.CLOSED)).toBeUndefined();
  });
});
