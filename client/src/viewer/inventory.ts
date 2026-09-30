import type { AssetStore } from '../assets/store';
import { DropItemFailure, StatId, UseItemFailure } from '../gen/wathena/v1/event_pb';
import { type ItemTexts, renderDescription } from './itemText';

// The inventory window, as RO's Alt+E window: each slot's icon, item and
// amount, and the weight. A double click or Enter uses an item. The Drop
// button, the Delete key, or dragging a row onto the game view drops it,
// after asking how many when the slot holds more than one. Items show by
// name from the build's item table, and a selected item shows its
// description below the list.

/** One inventory slot the server described. */
export interface Stack {
  slot: number;
  itemId: number;
  amount: number;
}

/** The inventory as the server last described it. */
export class InventoryModel {
  private readonly slots = new Map<number, Stack>();
  /** How many slots take new items. */
  size = 0;

  /** Replaces everything, as InventoryUpdate. */
  reset(items: readonly Stack[], size: number): void {
    this.slots.clear();
    for (const s of items) {
      if (s.amount > 0) {
        this.slots.set(s.slot, { slot: s.slot, itemId: s.itemId, amount: s.amount });
      }
    }
    this.size = size;
  }

  /** Adds an amount to a slot, as InventoryAdd. */
  add(slot: number, itemId: number, amount: number): void {
    const s = this.slots.get(slot);
    if (s?.itemId === itemId) {
      s.amount += amount;
    } else {
      this.slots.set(slot, { slot, itemId, amount });
    }
  }

  /** Takes an amount from a slot, as InventoryRemove. */
  remove(slot: number, amount: number): void {
    const s = this.slots.get(slot);
    if (s === undefined) {
      return;
    }
    s.amount -= amount;
    if (s.amount <= 0) {
      this.slots.delete(slot);
    }
  }

  get(slot: number): Stack | undefined {
    return this.slots.get(slot);
  }

  /** The held stacks in slot order. */
  list(): Stack[] {
    return [...this.slots.values()].sort((a, b) => a.slot - b.slot);
  }
}

/**
 * Weight as RO shows it, in whole units: "12 / 2000". The server sends
 * tenths. Values not sent yet show as "-".
 */
export function weightText(stats: ReadonlyMap<StatId, bigint>): string {
  const w = (id: StatId): string => {
    const v = stats.get(id);
    return v === undefined ? '-' : String(v / 10n);
  };
  return `${w(StatId.WEIGHT)} / ${w(StatId.MAX_WEIGHT)}`;
}

/** Weight as a percent of the maximum, rounded down, as rAthena. */
export function weightPercent(stats: ReadonlyMap<StatId, bigint>): number | undefined {
  const w = stats.get(StatId.WEIGHT);
  const max = stats.get(StatId.MAX_WEIGHT);
  if (w === undefined || max === undefined) {
    return undefined;
  }
  return Number((w * 100n) / (max > 0n ? max : 1n));
}

/** From this weight percent, the player cannot attack. */
export const MAJOR_OVERWEIGHT = 90;

/** The chat log line for items entering the inventory, as RO's pick up message. */
export function gotItemMessage(name: string, amount: number): string {
  return amount === 1 ? `You got ${name}.` : `You got ${name} (${amount}).`;
}

/** The chat log line for a used item. */
export function usedItemMessage(name: string): string {
  return `You used ${name}.`;
}

/** What to tell the player when using an item fails. */
export function useItemMessage(reason: UseItemFailure): string {
  switch (reason) {
    case UseItemFailure.EMPTY_SLOT:
      return 'That slot is empty.';
    case UseItemFailure.NOT_USABLE:
      return 'This item cannot be used.';
    case UseItemFailure.NOT_SUPPORTED:
      return 'This item does nothing yet. Only healing items work for now.';
    case UseItemFailure.TOO_SOON:
      return 'You are using items too fast.';
    case UseItemFailure.COOLDOWN:
      return 'This item is cooling down.';
    case UseItemFailure.CANNOT_USE:
      return 'You cannot use this item.';
    case UseItemFailure.OVERWEIGHT:
      return 'You are carrying too much to use this item.';
    case UseItemFailure.INVENTORY_FULL:
      return 'You need more free slots to use this item.';
    default:
      return 'The item was not used.';
  }
}

/** What to tell the player when dropping an item fails. */
export function dropItemMessage(reason: DropItemFailure): string {
  switch (reason) {
    case DropItemFailure.EMPTY_SLOT:
      return 'That slot is empty.';
    case DropItemFailure.BAD_AMOUNT:
      return 'You do not have that many.';
    case DropItemFailure.CANNOT_DROP:
      return 'This item cannot be dropped.';
    default:
      return 'The item was not dropped.';
  }
}

/** Reads a drop amount: a whole number from 1 to max, or undefined. */
export function parseDropAmount(text: string, max: number): number | undefined {
  const t = text.trim();
  if (!/^\d+$/.test(t)) {
    return undefined;
  }
  const n = Number(t);
  return n >= 1 && n <= max ? n : undefined;
}

/**
 * The effect each potion and herb plays on its user, from the comments of
 * roBrowserLegacy's EffectTable for EF_POTION1 to EF_POTION6. Other items
 * play none.
 */
export const USE_EFFECTS: ReadonlyMap<number, number> = new Map([
  [501, 204],
  [507, 204],
  [502, 205],
  [503, 206],
  [508, 206],
  [504, 207],
  [509, 207],
  [505, 208],
  [510, 208],
  [506, 209],
  [511, 209],
]);

/** The drag data type that carries a slot. */
export const SLOT_DRAG_TYPE = 'application/x-wathena-slot';

/** What the window asks the server to do. */
export interface InventoryActions {
  use: (slot: number) => void;
  drop: (slot: number, amount: number) => void;
}

interface Row {
  li: HTMLLIElement;
  icon: HTMLImageElement;
  label: HTMLSpanElement;
  amount: HTMLSpanElement;
  drop: HTMLButtonElement;
  itemId: number;
}

/** The inventory window on the page. */
export class InventoryWindow {
  private readonly list: HTMLUListElement;
  private readonly weight: HTMLOutputElement;
  private readonly count: HTMLOutputElement;
  private readonly form: HTMLFormElement;
  private readonly prompt: HTMLSpanElement;
  private readonly detail: HTMLDivElement;
  private readonly detailName: HTMLDivElement;
  private readonly detailText: HTMLDivElement;
  /** The slot whose description shows. */
  private selected: number | undefined;
  private readonly amountInput: HTMLInputElement;
  private readonly rows = new Map<number, Row>();
  private readonly icons = new Map<number, Promise<string | undefined>>();
  /** The slot the drop form asks about. */
  private dropping: number | undefined;

  constructor(
    private readonly root: HTMLElement,
    private readonly store: AssetStore,
    private readonly model: InventoryModel,
    private readonly actions: InventoryActions,
    private readonly texts: ItemTexts,
  ) {
    const head = document.createElement('div');
    head.className = 'inventory-head';
    this.count = document.createElement('output');
    this.weight = document.createElement('output');
    head.append(this.count, this.weight);
    this.list = document.createElement('ul');
    this.list.setAttribute('aria-label', 'Items');
    this.form = document.createElement('form');
    this.form.className = 'drop-form';
    this.form.hidden = true;
    const label = document.createElement('label');
    this.prompt = document.createElement('span');
    this.amountInput = document.createElement('input');
    this.amountInput.type = 'number';
    this.amountInput.min = '1';
    this.amountInput.required = true;
    label.append(this.prompt, this.amountInput);
    const ok = document.createElement('button');
    ok.type = 'submit';
    ok.textContent = 'Drop';
    const cancel = document.createElement('button');
    cancel.type = 'button';
    cancel.textContent = 'Cancel';
    cancel.addEventListener('click', () => {
      this.closeDrop();
    });
    this.form.append(label, ok, cancel);
    this.form.addEventListener('submit', (e) => {
      e.preventDefault();
      const slot = this.dropping;
      const s = slot === undefined ? undefined : this.model.get(slot);
      if (slot === undefined || s === undefined) {
        this.closeDrop();
        return;
      }
      const n = parseDropAmount(this.amountInput.value, s.amount);
      if (n === undefined) {
        this.amountInput.setCustomValidity(`Enter 1 to ${s.amount}.`);
        this.amountInput.reportValidity();
        return;
      }
      this.actions.drop(slot, n);
      this.closeDrop();
    });
    this.amountInput.addEventListener('input', () => {
      this.amountInput.setCustomValidity('');
    });
    this.form.addEventListener('keydown', (e) => {
      // Keys typed here are for the form, not game shortcuts.
      e.stopPropagation();
      if (e.key === 'Escape') {
        this.closeDrop();
      }
    });
    this.detail = document.createElement('div');
    this.detail.className = 'item-detail';
    this.detail.hidden = true;
    this.detailName = document.createElement('div');
    this.detailName.className = 'item-detail-name';
    this.detailText = document.createElement('div');
    this.detail.append(this.detailName, this.detailText);
    root.append(head, this.list, this.form, this.detail);
    texts.onLoad(() => {
      this.relabel();
    });
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  toggle(): void {
    this.root.hidden = !this.root.hidden;
    if (this.root.hidden) {
      this.closeDrop();
    }
  }

  /**
   * Starts a drop: a slot with one item drops at once, and a larger stack
   * asks how many, with the whole stack filled in.
   */
  requestDrop(slot: number): void {
    const s = this.model.get(slot);
    if (s === undefined) {
      return;
    }
    if (s.amount === 1) {
      this.actions.drop(slot, 1);
      return;
    }
    if (this.root.hidden) {
      this.toggle();
    }
    this.dropping = slot;
    this.prompt.textContent = `Drop how many ${this.texts.label(s.itemId)}? (1 to ${s.amount}) `;
    this.amountInput.max = String(s.amount);
    this.amountInput.value = String(s.amount);
    this.form.hidden = false;
    this.amountInput.focus();
    this.amountInput.select();
  }

  private closeDrop(): void {
    this.dropping = undefined;
    this.form.hidden = true;
  }

  /** Shows the inventory and weight. Only changed parts are written. */
  update(stats: ReadonlyMap<StatId, bigint>): void {
    const stacks = this.model.list();
    set(this.count, `${stacks.length} / ${this.model.size}`);
    const percent = weightPercent(stats);
    set(this.weight, `Weight ${weightText(stats)}${percent === undefined ? '' : ` (${percent}%)`}`);
    this.weight.classList.toggle('heavy', percent !== undefined && percent >= MAJOR_OVERWEIGHT);
    const seen = new Set<number>();
    let previous: HTMLLIElement | undefined;
    for (const s of stacks) {
      seen.add(s.slot);
      let row = this.rows.get(s.slot);
      if (row !== undefined && row.itemId !== s.itemId) {
        row.li.remove();
        this.rows.delete(s.slot);
        row = undefined;
      }
      if (row === undefined) {
        row = this.row(s.slot, s.itemId);
        this.rows.set(s.slot, row);
      }
      set(row.amount, `× ${s.amount}`);
      // Rows stay in slot order.
      const next = previous === undefined ? this.list.firstChild : previous.nextSibling;
      if (next !== row.li) {
        this.list.insertBefore(row.li, next);
      }
      previous = row.li;
    }
    for (const [slot, row] of this.rows) {
      if (!seen.has(slot)) {
        row.li.remove();
        this.rows.delete(slot);
      }
    }
    if (this.dropping !== undefined && !seen.has(this.dropping)) {
      this.closeDrop();
    }
    if (this.selected !== undefined && !seen.has(this.selected)) {
      this.select(undefined);
    }
  }

  /** Writes every row's name again, such as when the names load. */
  private relabel(): void {
    for (const row of this.rows.values()) {
      this.label(row);
    }
    if (this.selected !== undefined) {
      this.select(this.selected);
    }
  }

  private label(row: Row): void {
    const name = this.texts.label(row.itemId);
    set(row.label, name);
    row.li.title = `${name}: double-click to use, drag out to drop`;
    row.drop.setAttribute('aria-label', `Drop ${name}`);
  }

  /** Shows the description of the item in a slot, or hides it. */
  private select(slot: number | undefined): void {
    this.selected = slot;
    const s = slot === undefined ? undefined : this.model.get(slot);
    if (slot === undefined || s === undefined) {
      this.detail.hidden = true;
      return;
    }
    const id = s.itemId;
    this.detailName.textContent = this.texts.label(id);
    this.detail.hidden = false;
    this.texts
      .description(id)
      .then((lines) => {
        // Another item may be selected by now.
        if (this.selected === slot && this.model.get(slot)?.itemId === id) {
          renderDescription(this.detailText, lines ?? []);
        }
      })
      .catch((error: unknown) => {
        console.error(`description ${id}:`, error);
      });
  }

  private row(slot: number, itemId: number): Row {
    const li = document.createElement('li');
    li.tabIndex = 0;
    li.draggable = true;
    const icon = document.createElement('img');
    icon.alt = '';
    icon.width = 24;
    icon.height = 24;
    icon.draggable = false;
    const label = document.createElement('span');
    label.className = 'name';
    const amount = document.createElement('span');
    amount.className = 'amount';
    const drop = document.createElement('button');
    drop.type = 'button';
    drop.textContent = 'Drop';
    drop.addEventListener('click', () => {
      this.requestDrop(slot);
    });
    li.append(icon, label, amount, drop);
    li.addEventListener('focus', () => {
      this.select(slot);
    });
    li.addEventListener('dblclick', () => {
      this.actions.use(slot);
    });
    li.addEventListener('keydown', (e) => {
      if (e.target !== li) {
        return;
      }
      if (e.key === 'Enter') {
        e.preventDefault();
        e.stopPropagation();
        this.actions.use(slot);
      } else if (e.key === 'Delete') {
        e.preventDefault();
        this.requestDrop(slot);
      }
    });
    li.addEventListener('dragstart', (e) => {
      e.dataTransfer?.setData(SLOT_DRAG_TYPE, String(slot));
      if (e.dataTransfer !== null) {
        e.dataTransfer.effectAllowed = 'move';
      }
    });
    this.icon(itemId, icon);
    const row = { li, icon, label, amount, drop, itemId };
    this.label(row);
    return row;
  }

  /** Loads an item's icon, icon/item/<id>. Items without one show none. */
  private icon(id: number, img: HTMLImageElement): void {
    let url = this.icons.get(id);
    if (url === undefined) {
      url = this.store
        .bytes(`icon/item/${id}`, 'texture')
        .then((data) =>
          data === undefined
            ? undefined
            : URL.createObjectURL(new Blob([data], { type: 'image/webp' })),
        );
      this.icons.set(id, url);
    }
    url
      .then((src) => {
        if (src === undefined) {
          img.style.visibility = 'hidden';
        } else {
          img.src = src;
        }
      })
      .catch((error: unknown) => {
        img.style.visibility = 'hidden';
        console.error(`icon ${id}:`, error);
      });
  }
}

function set(el: HTMLElement, text: string): void {
  if (el.textContent !== text) {
    el.textContent = text;
  }
}
