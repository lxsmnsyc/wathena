import { describe, expect, it } from 'vitest';
import { ShopOutcome, StatId } from '../gen/wathena/v1/event_pb';
import {
  type ShopWallet,
  buyProblem,
  costParts,
  parseAmount,
  shopCosts,
  shopMessage,
  shopTotals,
  stockText,
} from './shop';

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

const wallet: ShopWallet = {
  itemCount: (id) => (id === 909 ? 5 : 0),
  currency: (name) => (name === 'example.arena:tokens' ? 10n : 0n),
  label: (id) => (id === 909 ? 'Jellopy' : `Item ${String(id)}`),
};

const tokens = { currency: 'example.arena:tokens', displayName: 'Arena tokens', amount: 3 };

describe('shopCosts', () => {
  it('sums the items and currencies of every line', () => {
    const c = shopCosts([
      { key: 501, amount: 2, price: 0, weight: 0, costItems: [{ itemId: 909, amount: 2 }] },
      { key: 502, amount: 1, price: 0, weight: 0, costItems: [{ itemId: 909, amount: 1 }] },
      { key: 503, amount: 2, price: 0, weight: 0, costCurrencies: [tokens] },
    ]);
    expect(c.items).toEqual(new Map([[909, 5]]));
    expect(c.currencies.get('example.arena:tokens')?.amount).toBe(6);
  });
});

describe('buyProblem with costs', () => {
  it('lets a purchase the player can pay in items and currencies go', () => {
    expect(
      buyProblem(
        [
          {
            key: 501,
            amount: 2,
            price: 10,
            weight: 0,
            costItems: [{ itemId: 909, amount: 2 }],
            costCurrencies: [tokens],
          },
        ],
        stats,
        wallet,
      ),
    ).toBeUndefined();
  });

  it('refuses one short of items, a currency, or stock', () => {
    expect(
      buyProblem(
        [{ key: 501, amount: 3, price: 0, weight: 0, costItems: [{ itemId: 909, amount: 2 }] }],
        stats,
        wallet,
      ),
    ).toBe('You need 6 Jellopy and have 5.');
    expect(
      buyProblem(
        [{ key: 501, amount: 4, price: 0, weight: 0, costCurrencies: [tokens] }],
        stats,
        wallet,
      ),
    ).toBe('You need 12 Arena tokens and have 10.');
    expect(
      buyProblem([{ key: 909, amount: 3, price: 0, weight: 0, stock: 2 }], stats, wallet),
    ).toBe('Only 2 Jellopy are left.');
    expect(
      buyProblem([{ key: 909, amount: 1, price: 0, weight: 0, stock: 0 }], stats, wallet),
    ).toBe('Jellopy is sold out.');
  });
});

describe('costParts', () => {
  it('lists the zeny, items and currencies with what the player has', () => {
    expect(
      costParts(
        { price: 50, costItems: [{ itemId: 909, amount: 2 }], costCurrencies: [tokens] },
        wallet,
      ),
    ).toEqual(['50 z', 'Jellopy × 2 (you have 5)', 'Arena tokens × 3 (you have 10)']);
    // A free item with another cost leaves the zeny out.
    expect(costParts({ price: 0, costCurrencies: [tokens] }, wallet)).toEqual([
      'Arena tokens × 3 (you have 10)',
    ]);
    expect(costParts({ price: 0 }, wallet)).toEqual(['0 z']);
  });
});

describe('stockText', () => {
  it('shows what is left, or nothing for an unlimited item', () => {
    expect(stockText(undefined)).toBe('');
    expect(stockText(12)).toBe('12 left');
    expect(stockText(0)).toBe('Sold out');
  });
});

describe('shopMessage for costs and stock', () => {
  it('names the new outcomes', () => {
    expect(shopMessage(ShopOutcome.NOT_ENOUGH_ITEMS, 'Jellopy')).toBe(
      'You do not have enough Jellopy for the purchase.',
    );
    expect(shopMessage(ShopOutcome.NOT_ENOUGH_CURRENCY)).toBe(
      'You do not have enough of a currency the purchase costs.',
    );
    expect(shopMessage(ShopOutcome.OUT_OF_STOCK, 'Jellopy')).toBe(
      'The shop does not have that many Jellopy left.',
    );
    expect(shopMessage(ShopOutcome.OUT_OF_STOCK)).toBe('The shop does not have that many left.');
  });
});
