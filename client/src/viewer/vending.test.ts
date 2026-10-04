import { describe, expect, it } from 'vitest';
import { ItemBound, ItemRestriction, VendingOutcome } from '../gen/wathena/v1/event_pb';
import {
  VENDING_MAX_PRICE,
  applySale,
  vendProblem,
  vendingBuyProblem,
  vendingMessage,
  vendingOpenProblem,
  vendingSoldText,
  vendingTotal,
} from './vending';
import plainStack from './testStacks';

const cart = new Map([
  [0, plainStack(0, 909, 10)],
  [1, plainStack(1, 501, 3)],
  [2, { ...plainStack(2, 502, 1), restrictions: ItemRestriction.NO_TRADE }],
  [3, { ...plainStack(3, 503, 1), bound: ItemBound.CHARACTER }],
]);
const lookup = (slot: number) => cart.get(slot);

describe('vendingOpenProblem', () => {
  it('lets a shop that fits open', () => {
    expect(
      vendingOpenProblem('Cheap jellopy', [{ cartSlot: 0, amount: 10, price: 5 }], 3, lookup),
    ).toBeUndefined();
  });

  it('checks the title, the lines, the amounts and the prices', () => {
    const line = { cartSlot: 0, amount: 1, price: 5 };
    expect(vendingOpenProblem(' ', [line], 3, lookup)).toBe('The title is empty.');
    expect(vendingOpenProblem('a'.repeat(80), [line], 3, lookup)).toBe(
      'The title is too long: 80 of at most 79 bytes.',
    );
    expect(vendingOpenProblem('Shop', [], 3, lookup)).toBe('Choose at least one item to sell.');
    expect(vendingOpenProblem('Shop', [line, { ...line, cartSlot: 1 }], 1, lookup)).toBe(
      'Your shop holds at most 1 items.',
    );
    expect(vendingOpenProblem('Shop', [{ ...line, amount: 11 }], 3, lookup)).toBe(
      'You have 10 of that item in the cart.',
    );
    expect(vendingOpenProblem('Shop', [{ ...line, price: VENDING_MAX_PRICE + 1 }], 3, lookup)).toBe(
      'A price is 1 to 1,000,000,000 zeny.',
    );
    expect(vendingOpenProblem('Shop', [{ ...line, cartSlot: 2 }], 3, lookup)).toBe(
      'This item cannot be vended.',
    );
    expect(vendingOpenProblem('Shop', [{ ...line, cartSlot: 9 }], 3, lookup)).toBe(
      'An item is no longer in your cart.',
    );
  });

  it('refuses untradeable and bound items', () => {
    expect(vendProblem(cart.get(0)!)).toBeUndefined();
    expect(vendProblem(cart.get(3)!)).toBe('A bound item cannot be vended.');
  });
});

const shop = [
  { index: 0, item: plainStack(0, 909, 10), price: 5 },
  { index: 1, item: plainStack(1, 501, 3), price: 100 },
];

describe('buying from a shop', () => {
  it('totals the lines and checks them', () => {
    const lines = [
      { index: 0, amount: 4 },
      { index: 1, amount: 2 },
    ];
    expect(vendingTotal(shop, lines)).toBe(220);
    expect(vendingBuyProblem(shop, lines, 220)).toBeUndefined();
    expect(vendingBuyProblem(shop, lines, 219)).toBe('You do not have enough zeny.');
    expect(vendingBuyProblem(shop, [], 1000)).toBe('Choose what to buy.');
    expect(vendingBuyProblem(shop, [{ index: 1, amount: 4 }], 1000)).toBe(
      'The shop has 3 of that item.',
    );
    expect(vendingBuyProblem(shop, [{ index: 7, amount: 1 }], 1000)).toBe(
      'That item is no longer for sale.',
    );
  });
});

describe('the own shop', () => {
  it('takes sales off the list', () => {
    const after = applySale(shop, 0, 4);
    expect(after[0]?.item.amount).toBe(6);
    expect(applySale(after, 1, 3).map((i) => i.index)).toEqual([0]);
    expect(vendingSoldText('Alice', 3, 'Jellopy', 1500)).toBe(
      'Alice bought 3 x Jellopy for 1,500 zeny.',
    );
  });
});

describe('vendingMessage', () => {
  it('names every outcome, with the hook reason', () => {
    for (const o of Object.values(VendingOutcome).filter((v) => typeof v === 'number')) {
      expect(vendingMessage(o)).not.toBe('');
    }
    expect(vendingMessage(VendingOutcome.HOOK, 'No vending here.')).toBe('No vending here.');
    expect(vendingMessage(VendingOutcome.STORE_CHANGED)).toBe('The shop changed. Browse it again.');
  });
});
