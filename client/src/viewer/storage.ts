// The storage and cart windows, as RO's storage and cart windows. One
// component draws both.
//
// The storage window opens when the server sends StorageOpen, such as
// from a storage or guild storage NPC. Its title is the storage's name:
// "Storage" for the account's storage, "Guild storage", or an extra
// storage's display name. Closing it tells the server.
//
// The cart window shows the cart the server sent at login or when the
// player got one. The player opens and closes it with the Cart button or
// Alt+W, and it shows the cart's weight. It closes when the player loses
// the cart.
//
// Both list their items in pages of STORAGE_PAGE_SIZE rows, so a storage
// of 10,000 slots stays fast. Items move in from the inventory window, by
// dragging a row onto the window or by a right click on the row, and out
// to the inventory with Take, a double click, or by dragging a row onto
// the inventory window. A row's second button moves it between the cart
// and the open storage. A stack of more than one asks how many first.
// The Log button shows the guild storage's last moves. Item names come
// from the build's item table.

import { StorageKind, StorageOutcome } from '../gen/wathena/v1/event_pb';
import { stackLabel } from './equipment';
import {
  type ItemIcons,
  SLOT_DRAG_TYPE,
  type Stack,
  type StackInput,
  itemTooltip,
  parseDropAmount,
  stack,
} from './inventory';
import { type ItemTexts, refinedLabel } from './itemText';

/** The drag data type that carries a storage slot. */
export const STORAGE_DRAG_TYPE = 'application/x-wathena-storage-slot';

/** The drag data type that carries a cart slot. */
export const CART_DRAG_TYPE = 'application/x-wathena-cart-slot';

/** How many rows a page of the storage window shows. */
export const STORAGE_PAGE_SIZE = 100;

/** A storage as StorageOpen describes it. The fields after items may be left out. */
export interface StorageOpenValues {
  kind: StorageKind;
  /** How many slots the storage has. */
  size: number;
  /** The first page of the slots that hold items. */
  items: readonly StackInput[];
  name?: string;
  displayName?: string;
  /** The cart's weight and its limit, in tenths. */
  weight?: number;
  maxWeight?: number;
  /** How many StorageItems pages follow. */
  pages?: number;
}

/** The stored items as the server last described them. */
export class StorageModel {
  private readonly slots = new Map<number, Stack>();
  /** The stacks in slot order, until the next change. */
  private sorted: Stack[] | undefined;
  /**
   * Slots that StorageAdd or StorageRemove changed since StorageOpen. A
   * later page leaves them as they are, since the change is newer.
   */
  private readonly changed = new Set<number>();
  /** The StorageItems pages that have not come yet. */
  private readonly missing = new Set<number>();
  /** The storage the window shows, or UNSPECIFIED when none is open. */
  kind = StorageKind.UNSPECIFIED;
  /** How many slots the storage has. */
  size = 0;
  /** The extra storage's record name, empty for the others. */
  name = '';
  /** The storage's name as players see it, or empty. */
  displayName = '';
  /** The cart's weight and its limit, in tenths. */
  weight = 0;
  maxWeight = 0;

  /** Replaces everything with the first page, as StorageOpen. */
  open(info: StorageOpenValues): void {
    this.slots.clear();
    this.changed.clear();
    this.missing.clear();
    this.sorted = undefined;
    for (const s of info.items) {
      if (s.amount > 0) {
        this.slots.set(s.slot, stack(s));
      }
    }
    this.kind = info.kind;
    this.size = info.size;
    this.name = info.name ?? '';
    this.displayName = info.displayName ?? '';
    this.weight = info.weight ?? 0;
    this.maxWeight = info.maxWeight ?? 0;
    for (let page = 1; page <= (info.pages ?? 0); page++) {
      this.missing.add(page);
    }
  }

  /**
   * Adds a later page of slots, as StorageItems. It returns false when the
   * page is not one the open storage waits for.
   */
  addPage(kind: StorageKind, page: number, items: readonly StackInput[]): boolean {
    if (kind !== this.kind || !this.missing.delete(page)) {
      return false;
    }
    for (const s of items) {
      if (s.amount > 0 && !this.changed.has(s.slot)) {
        this.slots.set(s.slot, stack(s));
      }
    }
    this.sorted = undefined;
    return true;
  }

  /** Whether pages of the open storage have not come yet. */
  get loading(): boolean {
    return this.missing.size > 0;
  }

  /** Forgets everything, as StorageClose. */
  close(): void {
    this.open({ kind: StorageKind.UNSPECIFIED, size: 0, items: [] });
  }

  /** Whether a storage is open. */
  get isOpen(): boolean {
    return this.kind !== StorageKind.UNSPECIFIED;
  }

  /**
   * Takes a slot as StorageAdd gives it: the slot after the change. weight
   * is the cart's weight after it, and is kept for the cart only.
   */
  add(item: StackInput, weight?: number): void {
    this.changed.add(item.slot);
    this.sorted = undefined;
    this.setWeight(weight);
    if (item.amount <= 0) {
      this.slots.delete(item.slot);
      return;
    }
    this.slots.set(item.slot, stack(item));
  }

  /**
   * Takes an amount from a slot, as StorageRemove. A slot left with
   * nothing is empty. weight is as for add.
   */
  remove(slot: number, amount: number, weight?: number): void {
    this.setWeight(weight);
    const s = this.slots.get(slot);
    if (s === undefined) {
      return;
    }
    this.changed.add(slot);
    s.amount -= amount;
    if (s.amount <= 0) {
      this.slots.delete(slot);
      this.sorted = undefined;
    }
  }

  private setWeight(weight: number | undefined): void {
    if (weight !== undefined && this.kind === StorageKind.CART) {
      this.weight = weight;
    }
  }

  get(slot: number): Stack | undefined {
    return this.slots.get(slot);
  }

  /** How many slots hold items. */
  get count(): number {
    return this.slots.size;
  }

  /** The stored items in slot order. */
  list(): readonly Stack[] {
    this.sorted ??= [...this.slots.values()].sort((a, b) => a.slot - b.slot);
    return this.sorted;
  }

  /** The items of a page, from 0, in slot order. */
  page(index: number, pageSize = STORAGE_PAGE_SIZE): readonly Stack[] {
    return this.list().slice(index * pageSize, (index + 1) * pageSize);
  }

  /** The slots used, such as "12 / 500". */
  get countText(): string {
    return `${String(this.slots.size)} / ${String(this.size)}`;
  }
}

/** How many pages a list of items fills. An empty list has one page. */
export function pageCount(items: number, pageSize = STORAGE_PAGE_SIZE): number {
  return Math.max(1, Math.ceil(items / pageSize));
}

/** A page index kept inside the pages there are. */
export function clampPage(index: number, pages: number): number {
  return Math.min(Math.max(0, index), Math.max(0, pages - 1));
}

/**
 * The title of a storage window: its display name when the server sends
 * one, or "Storage", "Guild storage" or "Cart".
 */
export function storageTitle(kind: StorageKind, displayName = ''): string {
  if (displayName !== '') {
    return displayName;
  }
  switch (kind) {
    case StorageKind.GUILD:
      return 'Guild storage';
    case StorageKind.CART:
      return 'Cart';
    default:
      return 'Storage';
  }
}

/** The cart's weight in whole units, such as "Weight 120 / 8000". The server sends tenths. */
export function cartWeightText(weight: number, maxWeight: number): string {
  return `Weight ${String(Math.floor(weight / 10))} / ${String(Math.floor(maxWeight / 10))}`;
}

/** What to tell the player when a storage or cart action fails. */
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
      return 'This item cannot go there.';
    case StorageOutcome.CANNOT_CARRY:
      return 'You cannot carry that item.';
    case StorageOutcome.RESTRICTED:
      return 'This item cannot be stored.';
    case StorageOutcome.BOUND:
      return 'This bounded item cannot be stored there.';
    case StorageOutcome.CART_OVERWEIGHT:
      return 'The cart cannot carry that much weight.';
    case StorageOutcome.NO_CART:
      return 'You do not have a cart.';
    case StorageOutcome.BUSY:
      return 'You cannot do that while trading or talking to an NPC.';
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

/**
 * A move the window asks about: put moves an inventory slot in, take
 * moves a slot out to the inventory, and send moves a slot to the other
 * window, the cart for the storage and the storage for the cart.
 */
export type StorageMoveKind = 'put' | 'take' | 'send';

/** What the window asks the server to do. */
export interface StorageActions {
  /** Moves an amount from an inventory slot into the window. */
  put: (slot: number, amount: number) => void;
  /** Moves an amount from a slot of the window into the inventory. */
  take: (slot: number, amount: number) => void;
  /** Moves an amount from a slot of the window into the other window. */
  send: (slot: number, amount: number) => void;
  /** Called when the player closes the window. */
  close: () => void;
  log: () => void;
}

/** How the window differs for the storage and the cart. */
export interface StorageWindowOptions {
  /** The drag data type of the window's rows. */
  dragType: string;
  /** Whether the window shows the cart, with its weight. */
  cart: boolean;
  /** Whether the other window can take items now, which shows each row's send button. */
  canSend: () => boolean;
}

/** An inventory stack the window may put in. */
type InventoryLookup = (slot: number) => Stack | undefined;

/** The words of each move, for the amount prompt. */
export function moveWords(cart: boolean): Record<StorageMoveKind, string> {
  return cart
    ? { put: 'put in the cart', take: 'take out', send: 'move to the storage' }
    : { put: 'store', take: 'take out', send: 'move to the cart' };
}

/** The amount prompt, such as "How many Jellopy to store? (1 to 10)". */
export function movePrompt(words: string, name: string, max: number): string {
  return `How many ${name} to ${words}? (1 to ${String(max)}) `;
}

/**
 * The window on the page, such as section#storage-window or
 * section#cart-window. It holds h2 with the storage's name,
 * output.storage-count, div.storage-weight for the cart, ul.storage-items
 * with one li[data-slot] per slot of the shown page, div.storage-pager,
 * form.storage-amount for amounts, and ol.storage-log for the log.
 */
export class StorageWindow {
  private readonly title = document.createElement('h2');
  private readonly count = document.createElement('output');
  private readonly weightBox = document.createElement('div');
  private readonly weightBar = document.createElement('meter');
  private readonly weightText = document.createElement('output');
  private readonly logButton = document.createElement('button');
  private readonly list = document.createElement('ul');
  private readonly pager = document.createElement('div');
  private readonly pageText = document.createElement('output');
  private readonly previous = document.createElement('button');
  private readonly next = document.createElement('button');
  private readonly loadingText = document.createElement('p');
  private readonly form = document.createElement('form');
  private readonly prompt = document.createElement('span');
  private readonly amountInput = document.createElement('input');
  private readonly logList = document.createElement('ol');
  private readonly logBox = document.createElement('div');
  private readonly words: Record<StorageMoveKind, string>;
  /** The page shown, from 0. */
  private pageIndex = 0;
  /** The move the amount form asks about. */
  private moving: { kind: StorageMoveKind; slot: number } | undefined;

  constructor(
    private readonly root: HTMLElement,
    private readonly model: StorageModel,
    private readonly inventory: InventoryLookup,
    private readonly actions: StorageActions,
    private readonly texts: ItemTexts,
    private readonly icons: ItemIcons,
    private readonly options: StorageWindowOptions = {
      dragType: STORAGE_DRAG_TYPE,
      cart: false,
      canSend: () => false,
    },
  ) {
    this.words = moveWords(options.cart);
    const head = document.createElement('div');
    head.className = 'storage-head';
    this.count.className = 'storage-count';
    this.logButton.type = 'button';
    this.logButton.className = 'storage-log-button';
    this.logButton.textContent = 'Log';
    this.logButton.title = 'Show the last moves of the guild storage';
    this.logButton.addEventListener('click', () => {
      this.actions.log();
    });
    const close = document.createElement('button');
    close.type = 'button';
    close.className = 'storage-close';
    close.textContent = 'Close';
    close.addEventListener('click', () => {
      this.requestClose();
    });
    head.append(this.title, this.count, this.logButton, close);

    this.weightBox.className = 'storage-weight';
    this.weightBox.hidden = !options.cart;
    this.weightBar.min = 0;
    this.weightBar.setAttribute('aria-label', 'Cart weight');
    this.weightBox.append(this.weightBar, this.weightText);

    this.list.className = 'storage-items';
    this.list.setAttribute('aria-label', options.cart ? 'Cart items' : 'Stored items');

    this.loadingText.className = 'storage-loading';
    this.loadingText.textContent = 'Loading items...';
    this.loadingText.hidden = true;

    this.pager.className = 'storage-pager';
    this.previous.type = 'button';
    this.previous.textContent = 'Previous';
    this.previous.addEventListener('click', () => {
      this.showPage(this.pageIndex - 1);
    });
    this.next.type = 'button';
    this.next.textContent = 'Next';
    this.next.addEventListener('click', () => {
      this.showPage(this.pageIndex + 1);
    });
    this.pager.append(this.previous, this.pageText, this.next);

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
      const s = m === undefined ? undefined : this.source(m.kind, m.slot);
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
      this.actions[m.kind](m.slot, n);
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
    this.acceptDrop(SLOT_DRAG_TYPE, (slot) => {
      this.requestPut(slot);
    });
    this.root.replaceChildren(
      head,
      this.weightBox,
      this.loadingText,
      this.list,
      this.pager,
      this.form,
      this.logBox,
    );
    texts.onLoad(() => {
      this.render();
    });
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /**
   * Calls onDrop with the slot of a row of another window dropped here,
   * such as a cart row on the storage window.
   */
  acceptDrop(type: string, onDrop: (slot: number) => void): void {
    this.root.addEventListener('dragover', (e) => {
      if (e.dataTransfer?.types.includes(type) === true) {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
      }
    });
    this.root.addEventListener('drop', (e) => {
      if (e.dataTransfer?.types.includes(type) !== true) {
        return;
      }
      const slot = Number(e.dataTransfer.getData(type));
      if (Number.isInteger(slot)) {
        e.preventDefault();
        onDrop(slot);
      }
    });
  }

  /** Shows the window, on its first page. */
  show(): void {
    this.root.hidden = false;
    this.logBox.hidden = true;
    this.pageIndex = 0;
    this.render();
  }

  /** Hides the window without telling the server, as after StorageClose. */
  hide(): void {
    this.root.hidden = true;
    this.closeAmount();
    this.logBox.hidden = true;
  }

  /** Shows or hides the window, as the Cart button does. */
  toggle(): void {
    if (this.root.hidden) {
      this.show();
    } else {
      this.requestClose();
    }
  }

  /** Closes the window and calls the close action. */
  requestClose(): void {
    if (this.root.hidden) {
      return;
    }
    this.hide();
    this.actions.close();
  }

  /**
   * Starts putting an inventory slot in. A slot with one item goes at
   * once, and a larger stack asks how many. Worn items stay.
   */
  requestPut(slot: number): void {
    const s = this.inventory(slot);
    if (s?.equip !== 0 || this.root.hidden || !this.model.isOpen) {
      return;
    }
    this.request('put', s);
  }

  /** Starts taking a slot out to the inventory, as requestPut. */
  requestTake(slot: number): void {
    const s = this.model.get(slot);
    if (s === undefined || this.root.hidden) {
      return;
    }
    this.request('take', s);
  }

  /** Starts moving a slot to the other window, as requestPut. */
  requestSend(slot: number): void {
    const s = this.model.get(slot);
    if (s === undefined || this.root.hidden || !this.options.canSend()) {
      return;
    }
    this.request('send', s);
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

  /** Shows a page, from 0. */
  private showPage(index: number): void {
    this.pageIndex = index;
    this.render();
    this.list.scrollTop = 0;
  }

  /** Draws the shown page again. Only that page's rows are made. */
  render(): void {
    const m = this.model;
    this.title.textContent = storageTitle(m.kind, m.displayName);
    this.count.textContent = m.countText;
    this.logButton.hidden = m.kind !== StorageKind.GUILD;
    if (this.options.cart) {
      const max = Math.max(1, m.maxWeight);
      this.weightBar.max = max;
      this.weightBar.high = max * 0.9;
      this.weightBar.value = m.weight;
      this.weightText.textContent = cartWeightText(m.weight, m.maxWeight);
    }
    this.loadingText.hidden = !m.loading;
    const pages = pageCount(m.count);
    this.pageIndex = clampPage(this.pageIndex, pages);
    this.pager.hidden = pages <= 1;
    this.pageText.textContent = `Page ${String(this.pageIndex + 1)} / ${String(pages)}`;
    this.previous.disabled = this.pageIndex === 0;
    this.next.disabled = this.pageIndex >= pages - 1;
    if (this.root.hidden) {
      return;
    }
    const label = (id: number): string => this.texts.label(id);
    const send = this.options.canSend();
    const rows = m.page(this.pageIndex).map((s) => this.row(s, label, send));
    this.list.replaceChildren(...rows);
    if (
      this.moving !== undefined &&
      this.source(this.moving.kind, this.moving.slot) === undefined
    ) {
      this.closeAmount();
    }
  }

  private row(s: Stack, label: (id: number) => string, send: boolean): HTMLLIElement {
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
    li.title = itemTooltip(
      `${name.textContent}: double-click to take, or drag onto the inventory`,
      s,
    );
    li.append(icon, name, amount, take);
    if (send) {
      const other = document.createElement('button');
      other.type = 'button';
      other.className = 'storage-send';
      other.textContent = this.options.cart ? 'To storage' : 'To cart';
      other.setAttribute(
        'aria-label',
        `Move ${name.textContent} to the ${this.options.cart ? 'storage' : 'cart'}`,
      );
      other.addEventListener('click', () => {
        this.requestSend(s.slot);
      });
      li.append(other);
    }
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
      e.dataTransfer?.setData(this.options.dragType, String(s.slot));
      if (e.dataTransfer !== null) {
        e.dataTransfer.effectAllowed = 'move';
      }
    });
    return li;
  }

  private source(kind: StorageMoveKind, slot: number): Stack | undefined {
    return kind === 'put' ? this.inventory(slot) : this.model.get(slot);
  }

  private request(kind: StorageMoveKind, s: Stack): void {
    if (s.amount === 1) {
      this.actions[kind](s.slot, 1);
      return;
    }
    this.moving = { kind, slot: s.slot };
    this.prompt.textContent = movePrompt(this.words[kind], this.texts.label(s.itemId), s.amount);
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
