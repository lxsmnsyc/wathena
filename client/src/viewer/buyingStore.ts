// Buying stores, as RO's buying stores. player:open_buying_store makes
// the server send BuyingStoreSetup, which opens the setup window: a
// title, the most zeny the store spends, and up to `slots` lines of an
// item the player owns, an amount and a price. Open opens the store.
// While it is open, the owner's window shows what the store still buys
// and the zeny left, with a Close store button, and the chat log tells
// each purchase.
//
// Another player's store shows its title as a board over the player. A
// click on the player, the board, or Browse store in the player menu
// asks for the store. The browse window lists what the store buys, and
// under each line the inventory slots that hold that item, with an
// amount input each. Sell sells them. User text is only ever set with
// textContent.

import {
  type BuyingStoreItem,
  type BuyingStoreList,
  BuyingStoreOutcome,
  ItemBound,
  ItemRestriction,
} from '../gen/wathena/v1/event_pb';
import {
  button,
  bytesProblem,
  element,
  field,
  keepKeys,
  numberInput,
  parseWhole,
  textInput,
  utf8Length,
  zenyLabel,
} from './forms';
import { type ItemIcons, type Stack, stackRow } from './inventory';
import type { ItemTexts } from './itemText';

/** The longest store title, in bytes. */
export const BUYING_TITLE_MAX = 79;
/** The most of one item a store buys, as BUYINGSTORE_MAX_AMOUNT. */
export const BUYING_MAX_AMOUNT = 9999;
/** The highest price, as BUYINGSTORE_MAX_PRICE. */
export const BUYING_MAX_PRICE = 99_990_000;

/** A line of a store being set up. */
export interface BuyingDraftLine {
  itemId: number;
  amount: number;
  price: number;
}

/** What the setup checks against. */
export interface BuyingOpenContext {
  /** The lines the store may have, BuyingStoreSetup.slots. */
  slots: number;
  /** The zeny the player has. */
  zeny: number;
  /** Whether the inventory holds an item, which rAthena requires. */
  owns: (itemId: number) => boolean;
}

/** Why a store cannot open, or undefined when it can. */
export function buyingOpenProblem(
  title: string,
  zenyLimit: number,
  lines: readonly BuyingDraftLine[],
  c: BuyingOpenContext,
): string | undefined {
  const t = bytesProblem('The title', title.trim(), 1, BUYING_TITLE_MAX);
  if (t !== undefined) {
    return t;
  }
  if (zenyLimit < 1) {
    return 'Enter the most zeny the store spends.';
  }
  if (zenyLimit > c.zeny) {
    return 'The zeny limit is more than you have.';
  }
  if (lines.length === 0) {
    return 'Choose at least one item to buy.';
  }
  if (lines.length > c.slots) {
    return `Your store buys at most ${String(c.slots)} items.`;
  }
  const seen = new Set<number>();
  for (const l of lines) {
    if (seen.has(l.itemId)) {
      return 'Each item goes on one line.';
    }
    seen.add(l.itemId);
    if (!c.owns(l.itemId)) {
      return 'You must have one of each item the store buys.';
    }
    if (l.amount < 1 || l.amount > BUYING_MAX_AMOUNT) {
      return `An amount is 1 to ${BUYING_MAX_AMOUNT.toLocaleString('en-US')}.`;
    }
    if (l.price < 1 || l.price > BUYING_MAX_PRICE) {
      return `A price is 1 to ${BUYING_MAX_PRICE.toLocaleString('en-US')} zeny.`;
    }
  }
  return undefined;
}

/** Whether an inventory stack can be sold to a buying store. */
export function canSellToStore(s: Stack): boolean {
  return (
    s.equip === 0 &&
    s.bound === ItemBound.UNSPECIFIED &&
    (s.restrictions & ItemRestriction.NO_TRADE) === 0
  );
}

/** The inventory stacks that hold an item a store buys, and can be sold. */
export function sellCandidates(stacks: readonly Stack[], itemId: number): Stack[] {
  return stacks.filter((s) => s.itemId === itemId && canSellToStore(s));
}

/** A line of a sale to a buying store. */
export interface BuyingSellLine {
  slot: number;
  itemId: number;
  amount: number;
}

/** The zeny a sale brings. */
export function buyingSellTotal(
  items: readonly Pick<BuyingStoreItem, 'itemId' | 'price'>[],
  lines: readonly BuyingSellLine[],
): number {
  let total = 0;
  for (const l of lines) {
    total += (items.find((i) => i.itemId === l.itemId)?.price ?? 0) * l.amount;
  }
  return total;
}

/** Why a sale to a store cannot go, or undefined when it can. */
export function buyingSellProblem(
  items: readonly Pick<BuyingStoreItem, 'itemId' | 'amount' | 'price'>[],
  zenyLimit: number,
  lines: readonly BuyingSellLine[],
  inventory: (slot: number) => Stack | undefined,
): string | undefined {
  if (lines.length === 0) {
    return 'Choose what to sell.';
  }
  const perItem = new Map<number, number>();
  const slots = new Set<number>();
  for (const l of lines) {
    const s = inventory(l.slot);
    if (s?.itemId !== l.itemId || slots.has(l.slot)) {
      return 'An item is no longer in your inventory.';
    }
    slots.add(l.slot);
    if (!canSellToStore(s)) {
      return 'This item cannot be sold to a store.';
    }
    if (l.amount < 1 || l.amount > s.amount) {
      return `You have ${String(s.amount)} of that item.`;
    }
    perItem.set(l.itemId, (perItem.get(l.itemId) ?? 0) + l.amount);
  }
  for (const [itemId, n] of perItem) {
    const want = items.find((i) => i.itemId === itemId);
    if (want === undefined) {
      return 'The store does not buy that item.';
    }
    if (n > want.amount) {
      return `The store buys only ${String(want.amount)} of that item.`;
    }
  }
  if (buyingSellTotal(items, lines) > zenyLimit) {
    return 'The store does not have that much zeny left.';
  }
  return undefined;
}

/** The lines of a store after it bought an amount of an item. A line at 0 goes. */
export function applyBought<T extends Pick<BuyingStoreItem, 'itemId' | 'amount'>>(
  items: readonly T[],
  itemId: number,
  amount: number,
): T[] {
  return items.flatMap((i) => {
    if (i.itemId !== itemId) {
      return [i];
    }
    const left = i.amount - amount;
    return left > 0 ? [{ ...i, amount: left }] : [];
  });
}

/** The chat log line for a purchase of the player's store. */
export function buyingBoughtText(
  seller: string,
  amount: number,
  item: string,
  zeny: number,
): string {
  return `${seller} sold you ${String(amount)} x ${item} for ${zeny.toLocaleString('en-US')} zeny.`;
}

/** What to tell the player about a buying store step. */
export function buyingStoreMessage(o: BuyingStoreOutcome, item = '', message = ''): string {
  switch (o) {
    case BuyingStoreOutcome.OPENED:
      return 'Your buying store is open.';
    case BuyingStoreOutcome.SOLD:
      return 'The sale went through.';
    case BuyingStoreOutcome.FAILED:
      return 'The buying store action failed.';
    case BuyingStoreOutcome.OVERWEIGHT:
      return 'You could not carry what the store buys.';
    case BuyingStoreOutcome.COUNT:
      return item === ''
        ? 'The store does not buy that many.'
        : `The store does not buy that many ${item}.`;
    case BuyingStoreOutcome.ZENY:
      return 'The store does not have enough zeny.';
    case BuyingStoreOutcome.ALL_BOUGHT:
      return 'Your buying store bought everything and closed.';
    case BuyingStoreOutcome.LIMIT_REACHED:
      return 'Your buying store reached its zeny limit and closed.';
    case BuyingStoreOutcome.HOOK:
      return message === '' ? 'The sale is not allowed.' : message;
    case BuyingStoreOutcome.CLOSED:
      return 'The buying store is closed.';
    default:
      return 'The buying store action did nothing.';
  }
}

/** Whether a buying store outcome closes the owner's store. */
export function closesOwnStore(o: BuyingStoreOutcome): boolean {
  return (
    o === BuyingStoreOutcome.ALL_BOUGHT ||
    o === BuyingStoreOutcome.LIMIT_REACHED ||
    o === BuyingStoreOutcome.CLOSED
  );
}

function windowHead(title: HTMLElement, onClose: () => void, closeLabel = 'Close'): HTMLDivElement {
  const head = element('div', 'store-head');
  head.append(title, button(closeLabel, onClose));
  return head;
}

/** What the setup window asks the server to do. */
export interface BuyingSetupActions {
  open: (title: string, zenyLimit: number, lines: BuyingDraftLine[]) => void;
  cancel: () => void;
}

/** What the buying store windows read from the inventory. */
export interface BuyingSource {
  stacks: () => readonly Stack[];
  inventory: (slot: number) => Stack | undefined;
  zeny: () => number;
}

/**
 * The setup window, section#buying-setup: a title, the zeny limit, and
 * one line per slot with an item picked from the inventory, an amount
 * and a price.
 */
export class BuyingSetupWindow {
  private readonly title = textInput('Store title', 30);
  private readonly titleCount = element('output', 'store-count');
  private readonly limit = numberInput('Zeny limit', 12);
  private readonly list = element('ol', 'store-lines');
  private readonly problem = element('output', 'store-problem');
  private rows: { item: HTMLSelectElement; amount: HTMLInputElement; price: HTMLInputElement }[] =
    [];
  private slots = 0;

  constructor(
    private readonly root: HTMLElement,
    private readonly source: BuyingSource,
    private readonly actions: BuyingSetupActions,
    private readonly texts: ItemTexts,
  ) {
    this.title.maxLength = BUYING_TITLE_MAX;
    const buttons = element('div', 'store-buttons');
    buttons.append(
      button('Open', () => {
        this.submit();
      }),
      button('Cancel', () => {
        this.cancel();
      }),
    );
    root.replaceChildren(
      windowHead(element('h2', '', 'Open a buying store'), () => {
        this.cancel();
      }),
      field('Title', this.title),
      this.titleCount,
      field('Zeny limit', this.limit),
      this.list,
      this.problem,
      buttons,
    );
    root.addEventListener('input', () => {
      this.titleCount.textContent = `${String(utf8Length(this.title.value))} / ${String(BUYING_TITLE_MAX)} bytes`;
    });
    keepKeys(root, () => {
      this.cancel();
    });
    root.hidden = true;
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /** Opens the window, as BuyingStoreSetup. */
  show(slots: number): void {
    this.slots = slots;
    // One choice per item the inventory holds, since the store needs one.
    const ids = [
      ...new Set(
        this.source
          .stacks()
          .filter((s) => s.equip === 0)
          .map((s) => s.itemId),
      ),
    ];
    const label = (id: number): string => this.texts.label(id);
    this.rows = [];
    const lines: HTMLLIElement[] = [];
    for (let i = 0; i < slots; i++) {
      const item = document.createElement('select');
      item.setAttribute('aria-label', `Item of line ${String(i + 1)}`);
      const none = document.createElement('option');
      none.value = '';
      none.textContent = '(none)';
      item.append(
        none,
        ...ids.map((id) => {
          const o = document.createElement('option');
          o.value = String(id);
          o.textContent = label(id);
          return o;
        }),
      );
      const amount = numberInput('Amount', 5);
      amount.setAttribute(
        'aria-label',
        `Amount of line ${String(i + 1)}, 1 to ${String(BUYING_MAX_AMOUNT)}`,
      );
      const price = numberInput('Price', 10);
      price.setAttribute('aria-label', `Price of line ${String(i + 1)}`);
      const li = document.createElement('li');
      li.append(item, amount, price);
      lines.push(li);
      this.rows.push({ item, amount, price });
    }
    this.list.replaceChildren(...lines);
    this.problem.textContent =
      ids.length === 0 ? 'Your inventory has no items to buy more of.' : '';
    this.titleCount.textContent = `${String(utf8Length(this.title.value))} / ${String(BUYING_TITLE_MAX)} bytes`;
    this.root.hidden = false;
    this.title.focus();
  }

  hide(): void {
    this.root.hidden = true;
  }

  private cancel(): void {
    if (this.root.hidden) {
      return;
    }
    this.hide();
    this.actions.cancel();
  }

  private submit(): void {
    const lines: BuyingDraftLine[] = [];
    for (const r of this.rows) {
      if (r.item.value === '') {
        continue;
      }
      const amount = parseWhole(r.amount.value, 1, BUYING_MAX_AMOUNT);
      const price = parseWhole(r.price.value, 1, BUYING_MAX_PRICE);
      if (amount === undefined) {
        this.problem.textContent = `An amount is 1 to ${BUYING_MAX_AMOUNT.toLocaleString('en-US')}.`;
        return;
      }
      if (price === undefined) {
        this.problem.textContent = `A price is 1 to ${BUYING_MAX_PRICE.toLocaleString('en-US')} zeny.`;
        return;
      }
      lines.push({ itemId: Number(r.item.value), amount, price });
    }
    const limit = parseWhole(this.limit.value, 0, Number.MAX_SAFE_INTEGER) ?? 0;
    const stacks = this.source.stacks();
    const problem = buyingOpenProblem(this.title.value, limit, lines, {
      slots: this.slots,
      zeny: this.source.zeny(),
      owns: (id) => stacks.some((s) => s.itemId === id),
    });
    if (problem !== undefined) {
      this.problem.textContent = problem;
      return;
    }
    this.actions.open(this.title.value.trim(), limit, lines);
    this.hide();
  }
}

/** A buying store line as the windows keep it. */
type StoreLine = Pick<BuyingStoreItem, 'itemId' | 'amount' | 'price'>;

/**
 * The owner's open store, section#buying-own: its title, the zeny it may
 * still spend, what it still buys, and Close store.
 */
export class BuyingOwnWindow {
  private readonly title = element('h2');
  private readonly limit = element('output', 'store-info');
  private readonly list = element('ul', 'store-items');
  private items: StoreLine[] = [];
  private zenyLimit = 0;

  constructor(
    private readonly root: HTMLElement,
    closeStore: () => void,
    private readonly texts: ItemTexts,
    private readonly icons: ItemIcons | undefined,
  ) {
    const head = windowHead(this.title, closeStore, 'Close store');
    head.append(
      button('Hide', () => {
        this.root.hidden = true;
      }),
    );
    this.list.setAttribute('aria-label', 'Items the store buys');
    root.replaceChildren(head, this.limit, this.list);
    keepKeys(root, () => {
      this.root.hidden = true;
    });
    root.hidden = true;
  }

  /** Whether the player's store is open. */
  get storeOpen(): boolean {
    return this.title.textContent !== '';
  }

  /** Shows the store, as BuyingStoreOpened. */
  show(title: string, zenyLimit: number, items: readonly StoreLine[]): void {
    this.title.textContent = title;
    this.zenyLimit = zenyLimit;
    this.items = items.map((i) => ({ itemId: i.itemId, amount: i.amount, price: i.price }));
    this.root.hidden = false;
    this.render();
  }

  /** Takes a purchase off the list, as BuyingStoreBought. */
  bought(itemId: number, amount: number, zenyLimit: number): void {
    this.items = applyBought(this.items, itemId, amount);
    this.zenyLimit = zenyLimit;
    this.render();
  }

  /** Forgets the store, as when it closes. */
  closed(): void {
    this.title.textContent = '';
    this.items = [];
    this.root.hidden = true;
  }

  /** Shows or hides the window while the store is open. */
  toggle(): void {
    if (this.storeOpen) {
      this.root.hidden = !this.root.hidden;
    }
  }

  private render(): void {
    this.limit.textContent = `Zeny left ${zenyLabel(this.zenyLimit)}`;
    const label = (id: number): string => this.texts.label(id);
    const rows = this.items.map((i) => storeRow(i, label, this.icons));
    if (rows.length === 0) {
      rows.push(element('li', 'empty', 'The store bought everything.'));
    }
    this.list.replaceChildren(...rows);
  }
}

/** A row of what a store buys: the item, how many, and the price. */
function storeRow(
  i: StoreLine,
  label: (id: number) => string,
  icons: ItemIcons | undefined,
): HTMLLIElement {
  const li = document.createElement('li');
  if (icons !== undefined) {
    const img = document.createElement('img');
    img.alt = '';
    img.width = 24;
    img.height = 24;
    icons.show(i.itemId, img);
    li.append(img);
  }
  li.append(
    element('span', 'name', label(i.itemId)),
    element('span', 'amount', `× ${String(i.amount)}`),
    element('span', 'value', zenyLabel(i.price)),
  );
  return li;
}

/** What the browse window asks the server to do. */
export interface BuyingBrowseActions {
  sell: (buyer: number, store: number, lines: BuyingSellLine[]) => void;
}

/**
 * Another player's buying store, section#buying-browse: what it buys,
 * and under each line the inventory slots that hold the item, with an
 * amount input each.
 */
export class BuyingBrowseWindow {
  private readonly title = element('h2');
  private readonly owner = element('div', 'store-owner');
  private readonly list = element('ul', 'store-items');
  private readonly total = element('output', 'store-total');
  private readonly problem = element('output', 'store-problem');
  private store: BuyingStoreList | undefined;
  private inputs: { slot: number; itemId: number; max: number; input: HTMLInputElement }[] = [];

  constructor(
    private readonly root: HTMLElement,
    private readonly source: BuyingSource,
    private readonly actions: BuyingBrowseActions,
    private readonly texts: ItemTexts,
    private readonly icons: ItemIcons | undefined,
  ) {
    const buttons = element('div', 'store-buttons');
    buttons.append(
      button('Sell', () => {
        this.sell();
      }),
    );
    this.list.setAttribute('aria-label', 'Items the store buys');
    root.replaceChildren(
      windowHead(this.title, () => {
        this.hide();
      }),
      this.owner,
      this.list,
      this.total,
      this.problem,
      buttons,
    );
    root.addEventListener('input', () => {
      this.refresh();
    });
    keepKeys(root, () => {
      this.hide();
    });
    root.hidden = true;
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /** Shows a store, as BuyingStoreList. */
  show(store: BuyingStoreList): void {
    this.store = store;
    this.title.textContent = store.title;
    this.owner.textContent = `Buyer: ${store.owner} · Zeny left ${zenyLabel(store.zenyLimit)}`;
    const label = (id: number): string => this.texts.label(id);
    this.inputs = [];
    const stacks = this.source.stacks();
    const rows = store.items.map((i) => {
      const li = storeRow(i, label, this.icons);
      const mine = element('ul', 'store-candidates');
      const candidates = sellCandidates(stacks, i.itemId);
      for (const s of candidates) {
        const input = numberInput('0', 5);
        input.setAttribute(
          'aria-label',
          `Amount to sell from slot ${String(s.slot)}, 0 to ${String(s.amount)}`,
        );
        this.inputs.push({ slot: s.slot, itemId: s.itemId, max: s.amount, input });
        mine.append(stackRow(s, label, this.icons, input));
      }
      if (candidates.length === 0) {
        mine.append(element('li', 'empty', 'You have none to sell.'));
      }
      li.append(mine);
      return li;
    });
    if (rows.length === 0) {
      rows.push(element('li', 'empty', 'The store buys nothing.'));
    }
    this.list.replaceChildren(...rows);
    this.problem.textContent = '';
    this.root.hidden = false;
    this.refresh();
  }

  hide(): void {
    this.root.hidden = true;
    this.store = undefined;
  }

  private lines(): BuyingSellLine[] | undefined {
    const out: BuyingSellLine[] = [];
    for (const { slot, itemId, max, input } of this.inputs) {
      const t = input.value.trim();
      if (t === '' || t === '0') {
        continue;
      }
      const n = parseWhole(t, 1, max);
      if (n === undefined) {
        return undefined;
      }
      out.push({ slot, itemId, amount: n });
    }
    return out;
  }

  private refresh(): void {
    const lines = this.lines();
    const store = this.store;
    if (lines === undefined || store === undefined) {
      this.problem.textContent = 'Amounts are whole numbers, up to what you have.';
      return;
    }
    this.total.textContent = `You get ${zenyLabel(buyingSellTotal(store.items, lines))}`;
    this.problem.textContent =
      lines.length > 0
        ? (buyingSellProblem(store.items, store.zenyLimit, lines, this.source.inventory) ?? '')
        : '';
  }

  private sell(): void {
    const lines = this.lines();
    const store = this.store;
    if (lines === undefined || store === undefined) {
      return;
    }
    const problem = buyingSellProblem(store.items, store.zenyLimit, lines, this.source.inventory);
    if (problem !== undefined) {
      this.problem.textContent = problem;
      return;
    }
    this.actions.sell(store.buyer, store.store, lines);
    this.hide();
  }
}
