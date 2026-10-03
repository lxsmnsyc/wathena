import { describe, expect, it } from 'vitest';
import { EquipFailure } from '../gen/wathena/v1/event_pb';
import {
  EQP,
  POSITIONS,
  canEquip,
  cardNames,
  cardTargets,
  equipMessage,
  equipmentRows,
  stackLabel,
} from './equipment';
import type { Stack } from './inventory';

function item(slot: number, itemId: number, more: Partial<Stack> = {}): Stack {
  return {
    slot,
    itemId,
    amount: 1,
    equip: 0,
    refine: 0,
    cards: [],
    location: 0,
    card: false,
    rolls: [],
    cardRolls: [],
    options: [],
    ...more,
  };
}

const names = new Map([
  [1201, 'Knife [3]'],
  [2301, 'Cotton Shirt [1]'],
  [4001, 'Poring Card'],
  [4002, 'Fabre Card'],
]);
const label = (id: number): string => names.get(id) ?? `Item ${id}`;

describe('equipmentRows', () => {
  it('lists every position with what is worn there', () => {
    const rows = equipmentRows([
      item(0, 1201, { equip: EQP.rightHand, location: EQP.rightHand }),
      item(1, 501, { amount: 5 }),
      // A hat over head top and head mid shows in both.
      item(2, 5001, { equip: EQP.headTop | EQP.headMid }),
    ]);
    expect(rows.map((r) => r.name)).toEqual(POSITIONS.map((p) => p.name));
    const worn = Object.fromEntries(rows.map((r) => [r.name, r.items.map((s) => s.slot)]));
    expect(worn['Right hand']).toEqual([0]);
    expect(worn['Head top']).toEqual([2]);
    expect(worn['Head mid']).toEqual([2]);
    expect(worn.Armor).toEqual([]);
  });

  it('shows a two-handed weapon in both hands', () => {
    const rows = equipmentRows([item(0, 1116, { equip: 0x22, location: 0x22 })]);
    const hands = rows.filter((r) => r.name.endsWith('hand'));
    expect(hands.map((r) => r.items.length)).toEqual([1, 1]);
  });

  it('adds a row for costumes and shadow gear only when worn', () => {
    expect(equipmentRows([]).some((r) => r.name === 'Costume and shadow')).toBe(false);
    const rows = equipmentRows([
      item(3, 19500, { equip: 0x400 }),
      item(4, 24000, { equip: 0x10000 }),
    ]);
    expect(rows.at(-1)).toMatchObject({ name: 'Costume and shadow' });
    expect(rows.at(-1)?.items.map((s) => s.slot)).toEqual([3, 4]);
  });
});

describe('labels', () => {
  it('shows the refine level and card names', () => {
    const knife = item(0, 1201, { refine: 7, cards: [4001, 4002] });
    expect(stackLabel(knife, label)).toBe('+7 Knife [3]');
    expect(cardNames(knife, label)).toEqual(['Poring Card', 'Fabre Card']);
    expect(stackLabel(item(1, 2301), label)).toBe('Cotton Shirt [1]');
  });
});

describe('canEquip', () => {
  it('offers to put on equipment that is not worn', () => {
    expect(canEquip(item(0, 1201, { location: EQP.rightHand }))).toBe(true);
    expect(canEquip(item(0, 1201, { location: EQP.rightHand, equip: EQP.rightHand }))).toBe(false);
    expect(canEquip(item(0, 501))).toBe(false);
    expect(canEquip(item(0, 4001, { location: EQP.rightHand, card: true }))).toBe(false);
  });
});

describe('cardTargets', () => {
  const slots = (id: number): number => {
    const m = /\[(\d)\]/.exec(names.get(id) ?? '');
    return m === null ? 0 : Number(m[1]);
  };
  const card = item(9, 4001, { location: EQP.rightHand, card: true });

  it('lists items that can take the card', () => {
    const stacks = [
      item(0, 1201, { location: EQP.rightHand }),
      // Worn.
      item(1, 1201, { location: EQP.rightHand, equip: EQP.rightHand }),
      // Another position.
      item(2, 2301, { location: EQP.armor }),
      // Every slot is full.
      item(3, 1201, { location: EQP.rightHand, cards: [4001, 4001, 4002] }),
      // No slots.
      item(4, 1116, { location: 0x22 }),
      // Another card.
      item(5, 4002, { location: EQP.rightHand, card: true }),
      // A free slot left.
      item(6, 1201, { location: EQP.rightHand, cards: [4002] }),
      card,
    ];
    expect(cardTargets(stacks, card, slots).map((s) => s.slot)).toEqual([0, 6]);
  });

  it('lists nothing when no item fits', () => {
    expect(cardTargets([item(2, 2301, { location: EQP.armor }), card], card, slots)).toEqual([]);
  });
});

describe('equipMessage', () => {
  it('explains failures', () => {
    expect(equipMessage(EquipFailure.CANNOT_EQUIP)).toBe('You cannot equip this item.');
    expect(equipMessage(EquipFailure.LEVEL)).toBe(
      'Your base level is too low or too high for this item.',
    );
    expect(equipMessage(EquipFailure.WRONG_AMMUNITION)).toBe(
      'This ammunition does not fit your weapon.',
    );
    expect(equipMessage(EquipFailure.UNSPECIFIED)).toBe('The item was not equipped.');
  });
});
