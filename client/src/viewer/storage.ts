// The storage window, as RO's storage window. It opens when the server
// sends StorageOpen, such as from a guild storage NPC, and lists the
// stored items with the slots used. Items move in from the inventory
// window, by dragging a row onto the storage window or by a right click
// on the row, and out to the inventory with Take, a double click, or by
// dragging a row onto the inventory window. A stack of more than one
// asks how many first. The Log button shows the guild storage's last
// moves. Closing the window tells the server. Item names come from the
// build's item table.

import { StorageKind, StorageOutcome } from '../gen/wathena/v1/event_pb';
import { stackLabel } from './equipment';
import {
  type ItemIcons,
  SLOT_DRAG_TYPE,
  type Stack,
  type StackInput,
  parseDropAmount,
  stack,
} from './inventory';
import { type ItemTexts, refinedLabel } from './itemText';

/** The drag data type that carries a storage slot. */
export const STORAGE_DRAG_TYPE = 'application/x-wathena-storage-slot';

/** The stored items as the server last described them. */
export class StorageModel {
  private readonly slots = new Map<number, Stack>();
  /** The storage the window shows, or UNSPECIFIED when none is open. */
  kind = StorageKind.UNSPECIFIED;
  /** How many slots hold items. */
  size = 0;

  /** Replaces everything, as StorageOpen. */
  open(kind: StorageKind, items: readonly StackInput[], size: number): void {
    this.slots.clear();
    for (const s of items) {
      if (s.amount > 0) {
        this.slots.set(s.slot, stack(s));
      }
    }
    this.kind = kind;
    this.size = size;
  }

  /** Forgets everything, as StorageClose. */
  close(): void {
    this.slots.clear();
    this.kind = StorageKind.UNSPECIFIED;
    this.size = 0;
  }

  /** Whether a storage is open. */
  get isOpen(): boolean {
    return this.kind !== StorageKind.UNSPECIFIED;
  }

  /** Takes a slot as StorageAdd gives it: the slot after the change. */
  add(item: StackInput): void {
    if (item.amount <= 0) {
      this.slots.delete(item.slot);
      return;
    }
    this.slots.set(item.slot, stack(item));
  }

  /** Takes an amount from a slot, as StorageRemove. A slot left with nothing is empty. */
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

  /** The stored items in slot order. */
  list(): Stack[] {
    return [...this.slots.values()].sort((a, b) => a.slot - b.slot);
  }

  /** The slots used, such as "12 / 500". */
  get countText(): string {
    return `${String(this.slots.size)} / ${String(this.size)}`;
  }
}

/** The title of a storage window. */
export function storageTitle(kind: StorageKind): string {
  return kind === StorageKind.GUILD ? 'Guild storage' : 'Storage';
}

/** What to tell the player when a storage action fails. */
export function storageMessage(outcome: StorageOutcome): string {
  switch (outcome) {
    case StorageOutcome.IN_USE:
      return 'Another guild member is using the guild storage.';
    case StorageOutcome.NO_GUILD:
      return 'You are not in a guild.';
    case StorageOutcome.NO_STORAGE:
      return 'Your guild has not learned Guild Storage Expansion.';
    case StorageOutcome.NO_PERMISSION:
      return 'Your guild position does not allow the guild storage.';
    case StorageOutcome.ALREADY_OPEN:
      return 'You have a storage open already.';
    case StorageOutcome.FULL:
      return 'The storage is full.';
    case StorageOutcome.NOT_STORABLE:
      return 'This item cannot go into the storage.';
    case StorageOutcome.CANNOT_CARRY:
      return 'You cannot carry that item.';
    default:
      return 'The storage action did nothing.';
  }
}

/** One move of the guild storage log, as GuildStorageLogEntry. */
export interface StorageLogEntry {
  atMs: bigint;
  name: string;
  itemId: number;
  amount: number;
  refine: number;
}

/**
 * A guild storage log line, such as "Bob put in Jellopy (3)" or "Bob
 * took out +7 Knife". The time is shown before it.
 */
export function storageLogLine(e: StorageLogEntry, label: (itemId: number) => string): string {
  const item = refinedLabel(label(e.itemId), e.refine);
  const amount = Math.abs(e.amount);
  const what = amount > 1 ? `${item} (${String(amount)})` : item;
  return e.amount >= 0 ? `${e.name} put in ${what}` : `${e.name} took out ${what}`;
}

/** A log time in the player's local time. */
export function logTime(atMs: bigint): string {
  return new Date(Number(atMs)).toLocaleString();
}

/** What the storage window asks the server to do. */
export interface StorageActions {
  /** Moves an amount from an inventory slot into the storage. */
  put: (slot: number, amount: number) => void;
  /** Moves an amount from a storage slot into the inventory. */
  take: (slot: number, amount: number) => void;
  close: () => void;
  log: () => void;
}

/** An inventory stack the window may put into the storage. */
type InventoryLookup = (slot: number) => Stack | undefined;

/**
 * The window on the page, section#storage-window. It holds h2 with the
 * storage's name and output.storage-count, ul.storage-items with one
 * li[data-slot] per slot, form.storage-amount for amounts, and
 * ol.storage-log for the log.
 */
export class StorageWindow {
  private readonly title = document.createElement('h2');
  private readonly count = document.createElement('output');
  private readonly list = document.createElement('ul');
  private readonly form = document.createElement('form');
  private readonly prompt = document.createElement('span');
  private readonly amountInput = document.createElement('input');
  private readonly logList = document.createElement('ol');
  private readonly logBox = document.createElement('div');
  /** The move the amount form asks about. */
  private moving: { put: boolean; slot: number } | undefined;

  constructor(
    private readonly root: HTMLElement,
    private readonly model: StorageModel,
    private readonly inventory: InventoryLookup,
    private readonly actions: StorageActions,
    private readonly texts: ItemTexts,
    private readonly icons: ItemIcons,
  ) {
    const head = document.createElement('div');
    head.className = 'storage-head';
    this.count.className = 'storage-count';
    const logButton = document.createElement('button');
    logButton.type = 'button';
    logButton.className = 'storage-log-button';
    logButton.textContent = 'Log';
    logButton.title = 'Show the last moves of the guild storage';
    logButton.addEventListener('click', () => {
      this.actions.log();
    });
    const close = document.createElement('button');
    close.type = 'button';
    close.className = 'storage-close';
    close.textContent = 'Close';
    close.addEventListener('click', () => {
      this.requestClose();
    });
    head.append(this.title, this.count, logButton, close);
    this.list.className = 'storage-items';
    this.list.setAttribute('aria-label', 'Stored items');

    this.form.className = 'storage-amount';
    this.form.hidden = true;
    const label = document.createElement('label');
    this.amountInput.type = 'number';
    this.amountInput.min = '1';
    this.amountInput.required = true;
    label.append(this.prompt, this.amountInput);
    const ok = document.createElement('button');
    ok.type = 'submit';
    ok.textContent = 'OK';
    const cancel = document.createElement('button');
    cancel.type = 'button';
    cancel.textContent = 'Cancel';
    cancel.addEventListener('click', () => {
      this.closeAmount();
    });
    this.form.append(label, ok, cancel);
    this.form.addEventListener('submit', (e) => {
      e.preventDefault();
      const m = this.moving;
      const s = m === undefined ? undefined : this.source(m.put, m.slot);
      if (m === undefined || s === undefined) {
        this.closeAmount();
        return;
      }
      const n = parseDropAmount(this.amountInput.value, s.amount);
      if (n === undefined) {
        this.amountInput.setCustomValidity(`Enter 1 to ${String(s.amount)}.`);
        this.amountInput.reportValidity();
        return;
      }
      this.move(m.put, m.slot, n);
      this.closeAmount();
    });
    this.amountInput.addEventListener('input', () => {
      this.amountInput.setCustomValidity('');
    });
    this.form.addEventListener('keydown', (e) => {
      // Keys typed here are for the form, not game shortcuts.
      e.stopPropagation();
      if (e.key === 'Escape') {
        this.closeAmount();
      }
    });

    this.logBox.className = 'storage-log';
    this.logBox.hidden = true;
    const logHead = document.createElement('div');
    logHead.className = 'storage-log-head';
    const logTitle = document.createElement('h3');
    logTitle.textContent = 'Storage log';
    const logClose = document.createElement('button');
    logClose.type = 'button';
    logClose.textContent = 'Hide';
    logClose.addEventListener('click', () => {
      this.logBox.hidden = true;
    });
    logHead.append(logTitle, logClose);
    this.logBox.append(logHead, this.logList);

    // A row from the inventory window dropped here puts the item in.
    this.root.addEventListener('dragover', (e) => {
      if (e.dataTransfer?.types.includes(SLOT_DRAG_TYPE) === true) {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
      }
    });
    this.root.addEventListener('drop', (e) => {
      if (e.dataTransfer?.types.includes(SLOT_DRAG_TYPE) !== true) {
        return;
      }
      const slot = Number(e.dataTransfer.getData(SLOT_DRAG_TYPE));
      if (Number.isInteger(slot)) {
        e.preventDefault();
        this.requestPut(slot);
      }
    });
    this.root.replaceChildren(head, this.list, this.form, this.logBox);
    texts.onLoad(() => {
      this.render();
    });
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /** Shows the window after StorageOpen. */
  show(): void {
    this.root.hidden = false;
    this.logBox.hidden = true;
    this.render();
  }

  /** Hides the window without telling the server, as after StorageClose. */
  hide(): void {
    this.root.hidden = true;
    this.closeAmount();
    this.logBox.hidden = true;
  }

  /** Closes the window and tells the server. */
  requestClose(): void {
    if (this.root.hidden) {
      return;
    }
    this.hide();
    this.actions.close();
  }

  /**
   * Starts putting an inventory slot into the storage. A slot with one
   * item goes at once, and a larger stack asks how many. Worn items stay.
   */
  requestPut(slot: number): void {
    const s = this.inventory(slot);
    if (s?.equip !== 0 || this.root.hidden) {
      return;
    }
    this.request(true, s);
  }

  /** Starts taking a storage slot out, as requestPut. */
  requestTake(slot: number): void {
    const s = this.model.get(slot);
    if (s === undefined || this.root.hidden) {
      return;
    }
    this.request(false, s);
  }

  /** Shows the guild storage log. */
  showLog(entries: readonly StorageLogEntry[]): void {
    const label = (id: number): string => this.texts.label(id);
    const rows = entries.map((e) => {
      const li = document.createElement('li');
      const time = document.createElement('time');
      time.dateTime = new Date(Number(e.atMs)).toISOString();
      time.textContent = logTime(e.atMs);
      const text = document.createElement('bdi');
      text.textContent = storageLogLine(e, label);
      li.classList.toggle('taken', e.amount < 0);
      li.append(time, ' ', text);
      return li;
    });
    if (rows.length === 0) {
      const li = document.createElement('li');
      li.className = 'empty';
      li.textContent = 'Nothing was moved yet.';
      rows.push(li);
    }
    this.logList.replaceChildren(...rows);
    this.logBox.hidden = false;
  }

  /** Draws the stored items again. */
  render(): void {
    this.title.textContent = storageTitle(this.model.kind);
    this.count.textContent = this.model.countText;
    const label = (id: number): string => this.texts.label(id);
    const rows = this.model.list().map((s) => this.row(s, label));
    this.list.replaceChildren(...rows);
    if (this.moving !== undefined && this.source(this.moving.put, this.moving.slot) === undefined) {
      this.closeAmount();
    }
  }

  private row(s: Stack, label: (id: number) => string): HTMLLIElement {
    const li = document.createElement('li');
    li.dataset.slot = String(s.slot);
    li.tabIndex = 0;
    li.draggable = true;
    const icon = document.createElement('img');
    icon.alt = '';
    icon.width = 24;
    icon.height = 24;
    icon.draggable = false;
    this.icons.show(s.itemId, icon);
    const name = document.createElement('span');
    name.className = 'name';
    name.textContent = stackLabel(s, label);
    const amount = document.createElement('span');
    amount.className = 'amount';
    amount.textContent = `× ${String(s.amount)}`;
    const take = document.createElement('button');
    take.type = 'button';
    take.textContent = 'Take';
    take.setAttribute('aria-label', `Take ${name.textContent}`);
    take.addEventListener('click', () => {
      this.requestTake(s.slot);
    });
    li.title = `${name.textContent}: double-click to take, or drag onto the inventory`;
    li.append(icon, name, amount, take);
    li.addEventListener('dblclick', () => {
      this.requestTake(s.slot);
    });
    li.addEventListener('keydown', (e) => {
      if (e.target === li && e.key === 'Enter') {
        e.preventDefault();
        e.stopPropagation();
        this.requestTake(s.slot);
      }
    });
    li.addEventListener('dragstart', (e) => {
      e.dataTransfer?.setData(STORAGE_DRAG_TYPE, String(s.slot));
      if (e.dataTransfer !== null) {
        e.dataTransfer.effectAllowed = 'move';
      }
    });
    return li;
  }

  private source(put: boolean, slot: number): Stack | undefined {
    return put ? this.inventory(slot) : this.model.get(slot);
  }

  private move(put: boolean, slot: number, amount: number): void {
    if (put) {
      this.actions.put(slot, amount);
    } else {
      this.actions.take(slot, amount);
    }
  }

  private request(put: boolean, s: Stack): void {
    if (s.amount === 1) {
      this.move(put, s.slot, 1);
      return;
    }
    this.moving = { put, slot: s.slot };
    const name = this.texts.label(s.itemId);
    this.prompt.textContent = `${put ? 'Store' : 'Take'} how many ${name}? (1 to ${String(s.amount)}) `;
    this.amountInput.max = String(s.amount);
    this.amountInput.value = String(s.amount);
    this.form.hidden = false;
    this.amountInput.focus();
    this.amountInput.select();
  }

  private closeAmount(): void {
    this.moving = undefined;
    this.form.hidden = true;
  }
}
