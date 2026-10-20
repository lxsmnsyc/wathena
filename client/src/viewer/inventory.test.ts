import { describe, expect, it } from 'vitest';
import {
  DropItemFailure,
  ItemBound,
  ItemRestriction,
  StatId,
  UseItemFailure,
} from '../gen/wathena/v1/event_pb';
import {
  InventoryModel,
  type Stack,
  USE_EFFECTS,
  actionWord,
  countItem,
  currenciesText,
  currencyBalance,
  dropItemMessage,
  gotItemMessage,
  itemRuleLines,
  itemTooltip,
  parseDropAmount,
  rollLines,
  stackRolls,
  useItemMessage,
  usedItemMessage,
  weightPercent,
  weightText,
  zenyLine,
} from './inventory';

/** A slot with no equipment details. */
function plain(slot: number, itemId: number, amount: number): Stack {
  return {
    slot,
    itemId,
    amount,
    equip: 0,
    refine: 0,
    cards: [],
    location: 0,
    card: false,
    rolls: [],
    cardRolls: [],
    options: [],
    bound: 0,
    restrictions: 0,
    broken: false,
    expiresAt: 0,
    signedBy: '',
  };
}

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
    expect(m.list()).toEqual([plain(1, 1201, 1), plain(3, 909, 1)]);
    // A list replaces everything.
    m.reset([], 100);
    expect(m.list()).toEqual([]);
  });

  it('replaces a slot that held another item', () => {
    const m = new InventoryModel();
    m.add(0, 501, 1);
    m.remove(0, 1);
    m.add(0, 909, 3);
    expect(m.get(0)).toEqual(plain(0, 909, 3));
  });

  it('keeps equipment details', () => {
    const m = new InventoryModel();
    const cards = [4001];
    m.reset(
      [{ slot: 2, itemId: 1201, amount: 1, equip: 0x2, refine: 7, cards, location: 0x2 }],
      100,
    );
    // The model keeps its own copy of the cards.
    cards.push(4002);
    expect(m.get(2)).toEqual({
      slot: 2,
      itemId: 1201,
      amount: 1,
      equip: 0x2,
      refine: 7,
      cards: [4001],
      location: 0x2,
      card: false,
      rolls: [],
      cardRolls: [],
      options: [],
      bound: 0,
      restrictions: 0,
      broken: false,
      expiresAt: 0,
      signedBy: '',
    });
    m.add(3, 4001, 1, { location: 0x2, card: true });
    expect(m.get(3)?.card).toBe(true);
    expect(m.get(3)?.location).toBe(0x2);
    m.add(4, 2301, 1, { refine: 4, cards: [4003] });
    expect(m.get(4)).toMatchObject({ equip: 0, refine: 4, cards: [4003] });
  });

  it('follows equips, unequips and inserted cards', () => {
    const m = new InventoryModel();
    m.reset(
      [
        { slot: 0, itemId: 1201, amount: 1, location: 0x2 },
        { slot: 1, itemId: 1116, amount: 1, location: 0x22 },
        { slot: 2, itemId: 4001, amount: 2, location: 0x2, card: true },
      ],
      100,
    );
    m.equipped(0, 0x2);
    expect(m.get(0)?.equip).toBe(0x2);
    // A two-handed weapon takes both hands. The knife comes off even if
    // its ItemUnequipped was missed.
    m.equipped(1, 0x22);
    expect(m.get(1)?.equip).toBe(0x22);
    expect(m.get(0)?.equip).toBe(0);
    m.unequipped(1);
    expect(m.get(1)?.equip).toBe(0);
    m.cardInserted(0, 4001);
    m.remove(2, 1);
    expect(m.get(0)?.cards).toEqual([4001]);
    expect(m.get(2)?.amount).toBe(1);
    // Slots that are empty change nothing.
    m.equipped(9, 0x2);
    m.unequipped(9);
    m.cardInserted(9, 4001);
    expect(m.list()).toHaveLength(3);
  });
});

describe('actionWord', () => {
  it('names what a double click does', () => {
    expect(actionWord(plain(0, 501, 1))).toBe('use');
    expect(actionWord({ ...plain(0, 1201, 1), location: 0x2 })).toBe('equip');
    expect(actionWord({ ...plain(0, 4001, 1), location: 0x2, card: true })).toBe('insert');
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
    expect(useItemMessage(UseItemFailure.NOT_SUPPORTED)).toBe('This item does not work yet.');
    expect(dropItemMessage(DropItemFailure.CANNOT_DROP)).toBe('This item cannot be dropped.');
    expect(dropItemMessage(DropItemFailure.BOUND)).toBe('This item cannot be dropped.');
    expect(dropItemMessage(DropItemFailure.UNSPECIFIED)).toBe('The item was not dropped.');
  });

  it('names items that were picked up or used', () => {
    expect(gotItemMessage('빨간포션', 5)).toBe('You got 빨간포션 (5).');
    expect(gotItemMessage('나이프 [3]', 1)).toBe('You got 나이프 [3].');
    expect(usedItemMessage('사과')).toBe('You used 사과.');
  });
});

describe('USE_EFFECTS', () => {
  it('plays the potion effect of the potion color', () => {
    expect(USE_EFFECTS.get(501)).toBe(204);
    expect(USE_EFFECTS.get(505)).toBe(208);
    expect(USE_EFFECTS.get(512)).toBeUndefined();
  });
});

describe('rolls', () => {
  const roll = {
    label: 'Damage against bosses',
    value: 31,
    known: true,
    min: 15,
    max: 35,
    unit: '%',
  };

  it('copies the roll fields of a message', () => {
    const got = stackRolls({
      rolls: [{ ...roll, name: 'boss_damage' } as typeof roll],
      cardRolls: [{ rolls: [{ ...roll, label: 'LUK', value: 2, min: 1, max: 5, unit: '' }] }],
      options: [{ label: 'MaxHP', value: 100, unit: '', flag: false }],
    });
    expect(got.rolls).toEqual([roll]);
    expect(got.cardRolls[0]?.[0]?.value).toBe(2);
    expect(got.options).toEqual([{ label: 'MaxHP', value: 100, unit: '', flag: false }]);
  });

  it('lists rolls, card rolls and options for the description', () => {
    const m = new InventoryModel();
    m.reset([{ slot: 3, itemId: 1201, amount: 1, cards: [4001], rolls: [roll] }], 100);
    m.rollsChanged(3, {
      rolls: [{ ...roll, value: 20 }],
      cardRolls: [[{ ...roll, label: 'LUK', value: 2, min: 1, max: 5, unit: '' }]],
      options: [{ label: 'Attack element: Fire', value: 0, unit: '', flag: true }],
    });
    const s = m.get(3);
    expect(s === undefined ? [] : rollLines(s, (id) => `Item ${id}`)).toEqual([
      'Damage against bosses +20% (15 to 35)',
      'Item 4001: LUK +2 (1 to 5)',
      'Attack element: Fire',
    ]);
  });
});

describe('itemRuleLines', () => {
  it('says who a copy is bound to and what it cannot do', () => {
    expect(itemRuleLines({ bound: ItemBound.UNSPECIFIED, restrictions: 0 })).toEqual([]);
    expect(
      itemRuleLines({
        bound: ItemBound.ACCOUNT,
        restrictions: ItemRestriction.NO_TRADE | ItemRestriction.NO_DROP,
      }),
    ).toEqual(['Bound to account', 'Cannot be dropped', 'Cannot be traded']);
    expect(itemRuleLines({ bound: ItemBound.CHARACTER, restrictions: 0 })).toEqual([
      'Bound to character',
    ]);
    expect(itemRuleLines({ bound: ItemBound.UNSPECIFIED, restrictions: 0x7f })).toEqual([
      'Cannot be dropped',
      'Cannot be traded',
      'Cannot be sold',
      'Cannot be put in a cart',
      'Cannot be stored',
      'Cannot be put in guild storage',
      'Cannot be mailed',
    ]);
  });

  it('says whether a copy is named, broken or rented', () => {
    expect(
      itemRuleLines({
        bound: ItemBound.UNSPECIFIED,
        restrictions: 0,
        signedBy: 'Alice',
        broken: true,
      }),
    ).toEqual(['Named for Alice', 'Broken: repair it to wear it']);
    const lines = itemRuleLines({
      bound: ItemBound.UNSPECIFIED,
      restrictions: 0,
      expiresAt: 1_893_456_000,
    });
    expect(lines).toHaveLength(1);
    expect(lines[0]?.startsWith('Rental until ')).toBe(true);
  });

  it('adds the lines to a tooltip', () => {
    expect(itemTooltip('Knife', { bound: ItemBound.GUILD, restrictions: 0 })).toBe(
      'Knife\nBound to guild',
    );
    expect(itemTooltip('Knife', { bound: ItemBound.UNSPECIFIED, restrictions: 0 })).toBe('Knife');
  });

  it('keeps bound and restrictions from the server', () => {
    const m = new InventoryModel();
    m.reset([{ slot: 0, itemId: 501, amount: 1, bound: ItemBound.PARTY, restrictions: 2 }], 100);
    expect(m.get(0)).toMatchObject({ bound: ItemBound.PARTY, restrictions: 2 });
  });
});

describe('InventoryModel pages', () => {
  it('adds the pages after the first InventoryUpdate', () => {
    const m = new InventoryModel();
    m.reset([{ slot: 0, itemId: 501, amount: 1 }], 1000);
    m.addPage([
      { slot: 600, itemId: 909, amount: 3 },
      { slot: 601, itemId: 502, amount: 0 },
    ]);
    expect(m.list().map((s) => s.slot)).toEqual([0, 600]);
    expect(m.size).toBe(1000);
    // A new first page replaces everything.
    m.reset([{ slot: 2, itemId: 501, amount: 1 }], 1000);
    expect(m.list().map((s) => s.slot)).toEqual([2]);
  });
});

describe('InventoryModel bound', () => {
  it('keeps bound and restrictions from InventoryAdd', () => {
    const m = new InventoryModel();
    m.reset([], 100);
    m.add(0, 501, 1, { bound: ItemBound.ACCOUNT, restrictions: 2 });
    expect(m.get(0)).toMatchObject({ bound: ItemBound.ACCOUNT, restrictions: 2 });
    m.add(0, 501, 2, { bound: ItemBound.ACCOUNT, restrictions: 6 });
    expect(m.get(0)).toMatchObject({ amount: 3, bound: ItemBound.ACCOUNT, restrictions: 6 });
    // Details left out keep what the slot has.
    m.add(0, 501, 1);
    expect(m.get(0)).toMatchObject({ amount: 4, restrictions: 6 });
  });
});

describe('currencies', () => {
  const list = [
    { name: 'cash', displayName: 'Cash', amount: 1500n },
    { name: 'example.arena:tokens', displayName: '', amount: 25n },
  ];

  it('finds a balance by its full name', () => {
    expect(currencyBalance(list, 'cash')).toBe(1500n);
    expect(currencyBalance(list, 'missing')).toBe(0n);
  });

  it('shows each balance by its display name, or its full name', () => {
    expect(currenciesText(list)).toBe('Currencies: Cash 1,500, example.arena:tokens 25');
    expect(currenciesText([])).toBe('Currencies: none');
  });

  it('shows the zeny', () => {
    expect(zenyLine(new Map([[StatId.ZENY, 1234567n]]))).toBe('Zeny 1,234,567');
    expect(zenyLine(new Map())).toBe('Zeny -');
  });
});

describe('countItem', () => {
  it('counts the held stacks of an item, leaving out worn ones', () => {
    const worn = { ...plain(3, 2301, 1), equip: 16 };
    const stacks = [plain(1, 909, 3), plain(2, 909, 4), plain(4, 2301, 1), worn];
    expect(countItem(stacks, 909)).toBe(7);
    expect(countItem(stacks, 2301)).toBe(1);
    const m = new InventoryModel();
    m.reset(stacks, 100);
    expect(m.count(909)).toBe(7);
  });
});
