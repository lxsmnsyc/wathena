import { EquipFailure } from '../gen/wathena/v1/event_pb';
import type { InventoryModel, Stack } from './inventory';
import { type ItemTexts, refinedLabel } from './itemText';

// The equipment window, as RO's Alt+Q window: each equipment position with
// the item worn there, its refine level, and its cards. Clicking a worn
// item takes it off. Costumes and shadow gear share one row at the end,
// shown only when something is worn there.

/** rAthena's EQP_ bits (mmo.hpp) for the positions the window shows. */
export const EQP = {
  headLow: 0x1,
  rightHand: 0x2,
  garment: 0x4,
  rightAccessory: 0x8,
  armor: 0x10,
  leftHand: 0x20,
  shoes: 0x40,
  leftAccessory: 0x80,
  headTop: 0x100,
  headMid: 0x200,
  ammo: 0x8000,
} as const;

/** The positions the window lists, in order. */
export const POSITIONS: readonly { name: string; mask: number }[] = [
  { name: 'Head top', mask: EQP.headTop },
  { name: 'Head mid', mask: EQP.headMid },
  { name: 'Head low', mask: EQP.headLow },
  { name: 'Armor', mask: EQP.armor },
  { name: 'Right hand', mask: EQP.rightHand },
  { name: 'Left hand', mask: EQP.leftHand },
  { name: 'Garment', mask: EQP.garment },
  { name: 'Shoes', mask: EQP.shoes },
  { name: 'Right accessory', mask: EQP.rightAccessory },
  { name: 'Left accessory', mask: EQP.leftAccessory },
  { name: 'Ammo', mask: EQP.ammo },
];

/** Every bit of the listed positions. */
const LISTED = POSITIONS.reduce((m, p) => m | p.mask, 0);

/** One row of the window: a position and the items worn there. */
export interface EquipmentRow {
  name: string;
  items: Stack[];
}

/**
 * The window's rows. An item worn in several positions, such as a hat over
 * head top and head mid, shows in each. Items worn only in other positions,
 * such as costumes and shadow gear, share a last row, which is left out
 * when empty.
 */
export function equipmentRows(stacks: readonly Stack[]): EquipmentRow[] {
  const rows = POSITIONS.map((p) => ({
    name: p.name,
    items: stacks.filter((s) => (s.equip & p.mask) !== 0).slice(0, 1),
  }));
  const other = stacks.filter((s) => s.equip !== 0 && (s.equip & LISTED) === 0);
  if (other.length > 0) {
    rows.push({ name: 'Costume and shadow', items: other });
  }
  return rows;
}

/** Whether the inventory window offers to put an item on. */
export function canEquip(s: Stack): boolean {
  return s.location !== 0 && !s.card && s.equip === 0;
}

/**
 * The items a card can go into: not worn, not a card, worn in a position
 * the card fits, and with a free card slot. slots gives an item's card
 * slots from the item table.
 */
export function cardTargets(
  stacks: readonly Stack[],
  card: Stack,
  slots: (itemId: number) => number,
): Stack[] {
  return stacks.filter(
    (s) =>
      s.slot !== card.slot &&
      s.equip === 0 &&
      !s.card &&
      (s.location & card.location) !== 0 &&
      s.cards.length < slots(s.itemId),
  );
}

/** How an item shows: its label with its refine level, such as "+7 Knife [3]". */
export function stackLabel(s: Stack, label: (itemId: number) => string): string {
  return refinedLabel(label(s.itemId), s.refine);
}

/** The names of an item's cards, in slot order. */
export function cardNames(s: Stack, label: (itemId: number) => string): string[] {
  return s.cards.map(label);
}

/** What to tell the player when putting an item on fails. */
export function equipMessage(reason: EquipFailure): string {
  switch (reason) {
    case EquipFailure.EMPTY_SLOT:
      return 'That slot is empty.';
    case EquipFailure.CANNOT_EQUIP:
      return 'You cannot equip this item.';
    case EquipFailure.LEVEL:
      return 'Your base level is too low or too high for this item.';
    case EquipFailure.WRONG_AMMUNITION:
      return 'This ammunition does not fit your weapon.';
    default:
      return 'The item was not equipped.';
  }
}

/** What to tell the player when a card does not go in. */
export const INSERT_CARD_FAILED = 'The card cannot go into that item.';

/** What to tell the player when no item can take a card. */
export const NO_CARD_TARGET = 'No item can take this card.';

/** What the window asks the server to do. */
export interface EquipmentActions {
  unequip: (slot: number) => void;
}

/** The equipment window on the page. */
export class EquipmentWindow {
  private readonly table: HTMLTableElement;
  /** What the rows last showed, to skip writing them again. */
  private shown = '';

  constructor(
    private readonly root: HTMLElement,
    private readonly model: InventoryModel,
    private readonly actions: EquipmentActions,
    private readonly texts: ItemTexts,
  ) {
    this.table = document.createElement('table');
    root.append(this.table);
    texts.onLoad(() => {
      this.shown = '';
      this.update();
    });
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  toggle(): void {
    this.root.hidden = !this.root.hidden;
  }

  /** Shows what is worn. The rows are written again only when they change. */
  update(): void {
    const label = (id: number): string => this.texts.label(id);
    const rows = equipmentRows(this.model.list()).map((r) => ({
      name: r.name,
      items: r.items.map((s) => ({
        slot: s.slot,
        label: stackLabel(s, label),
        cards: cardNames(s, label),
      })),
    }));
    const key = JSON.stringify(rows);
    if (key === this.shown) {
      return;
    }
    this.shown = key;
    const body = document.createElement('tbody');
    for (const r of rows) {
      const row = body.insertRow();
      const name = row.insertCell();
      name.className = 'position';
      name.textContent = r.name;
      const cell = row.insertCell();
      for (const it of r.items) {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'worn';
        button.textContent = it.label;
        button.title = `${it.label}: click to take off`;
        button.addEventListener('click', () => {
          this.actions.unequip(it.slot);
        });
        cell.append(button);
        if (it.cards.length > 0) {
          const cards = document.createElement('div');
          cards.className = 'cards';
          cards.textContent = it.cards.join(', ');
          cell.append(cards);
        }
      }
    }
    this.table.replaceChildren(body);
  }
}
