import { describe, expect, it } from 'vitest';
import { BuyingStoreOutcome, ItemBound } from '../gen/wathena/v1/event_pb';
import {
  applyBought,
  buyingBoughtText,
  buyingOpenProblem,
  buyingSellProblem,
  buyingSellTotal,
  buyingStoreMessage,
  closesOwnStore,
  sellCandidates,
} from './buyingStore';
import plainStack from './testStacks';

const context = { slots: 2, zeny: 100000, owns: (id: number) => id === 909 || id === 501 };

describe('buyingOpenProblem', () => {
  const line = { itemId: 909, amount: 100, price: 10 };

  it('lets a store that fits open', () => {
    expect(buyingOpenProblem('Buying jellopy', 1000, [line], context)).toBeUndefined();
  });

  it('checks the title, the zeny limit and the lines', () => {
    expect(buyingOpenProblem('', 1000, [line], context)).toBe('The title is empty.');
    expect(buyingOpenProblem('Buy', 0, [line], context)).toBe(
      'Enter the most zeny the store spends.',
    );
    expect(buyingOpenProblem('Buy', 100001, [line], context)).toBe(
      'The zeny limit is more than you have.',
    );
    expect(buyingOpenProblem('Buy', 1000, [], context)).toBe('Choose at least one item to buy.');
    expect(
      buyingOpenProblem(
        'Buy',
        1000,
        [line, { ...line, itemId: 501 }, { ...line, itemId: 502 }],
        context,
      ),
    ).toBe('Your store buys at most 2 items.');
    expect(buyingOpenProblem('Buy', 1000, [line, line], context)).toBe(
      'Each item goes on one line.',
    );
    expect(buyingOpenProblem('Buy', 1000, [{ ...line, itemId: 502 }], context)).toBe(
      'You must have one of each item the store buys.',
    );
    expect(buyingOpenProblem('Buy', 1000, [{ ...line, amount: 10000 }], context)).toBe(
      'An amount is 1 to 9,999.',
    );
    expect(buyingOpenProblem('Buy', 1000, [{ ...line, price: 99990001 }], context)).toBe(
      'A price is 1 to 99,990,000 zeny.',
    );
  });
});

describe('selling to a store', () => {
  const stacks = [
    plainStack(1, 909, 5),
    plainStack(2, 909, 3),
    { ...plainStack(3, 909, 2), bound: ItemBound.ACCOUNT },
    { ...plainStack(4, 909, 1), equip: 1 },
  ];
  const inventory = (slot: number) => stacks.find((s) => s.slot === slot);
  const store = [{ itemId: 909, amount: 6, price: 10 }];

  it('picks the slots that can be sold', () => {
    expect(sellCandidates(stacks, 909).map((s) => s.slot)).toEqual([1, 2]);
  });

  it('checks the amounts and the zeny left', () => {
    const lines = [
      { slot: 1, itemId: 909, amount: 4 },
      { slot: 2, itemId: 909, amount: 2 },
    ];
    expect(buyingSellTotal(store, lines)).toBe(60);
    expect(buyingSellProblem(store, 60, lines, inventory)).toBeUndefined();
    expect(buyingSellProblem(store, 59, lines, inventory)).toBe(
      'The store does not have that much zeny left.',
    );
    expect(
      buyingSellProblem(store, 1000, [...lines, { slot: 2, itemId: 909, amount: 1 }], inventory),
    ).toBe('An item is no longer in your inventory.');
    expect(
      buyingSellProblem(
        store,
        1000,
        [
          { slot: 1, itemId: 909, amount: 5 },
          { slot: 2, itemId: 909, amount: 2 },
        ],
        inventory,
      ),
    ).toBe('The store buys only 6 of that item.');
    expect(buyingSellProblem(store, 1000, [{ slot: 3, itemId: 909, amount: 1 }], inventory)).toBe(
      'This item cannot be sold to a store.',
    );
    expect(buyingSellProblem(store, 1000, [], inventory)).toBe('Choose what to sell.');
  });
});

describe('the own store', () => {
  it('takes purchases off the list', () => {
    const items = [
      { itemId: 909, amount: 6, price: 10 },
      { itemId: 501, amount: 1, price: 50 },
    ];
    expect(applyBought(items, 909, 2)[0]?.amount).toBe(4);
    expect(applyBought(items, 501, 1).map((i) => i.itemId)).toEqual([909]);
    expect(buyingBoughtText('Bob', 2, 'Jellopy', 20)).toBe('Bob sold you 2 x Jellopy for 20 zeny.');
  });
});

describe('buyingStoreMessage', () => {
  it('names every outcome', () => {
    for (const o of Object.values(BuyingStoreOutcome).filter((v) => typeof v === 'number')) {
      expect(buyingStoreMessage(o)).not.toBe('');
    }
    expect(buyingStoreMessage(BuyingStoreOutcome.COUNT, 'Jellopy')).toBe(
      'The store does not buy that many Jellopy.',
    );
    expect(buyingStoreMessage(BuyingStoreOutcome.HOOK, '', 'Not here.')).toBe('Not here.');
    expect(closesOwnStore(BuyingStoreOutcome.ALL_BOUGHT)).toBe(true);
    expect(closesOwnStore(BuyingStoreOutcome.SOLD)).toBe(false);
  });
});
