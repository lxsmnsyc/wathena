// Vending shops, as RO's merchant shops. The Vending skill (ID 41) makes
// the server send VendingSetup, which opens the setup window: a title,
// and up to max_items cart slots with an amount and a price each. Open
// opens the shop, and Cancel closes the window. While the shop is open,
// the player's own shop window shows what is left, with a Close shop
// button, and the chat log tells each sale.
//
// Another player's shop shows its title as a board over the player. A
// click on the player, the board, or Browse shop in the player menu asks
// for the shop, and the browse window lists its items with an amount
// input each, the total, and Buy. User text is only ever set with
// textContent.

import { ItemBound, ItemRestriction, VendingOutcome } from '../gen/wathena/v1/event_pb';
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
import { type ItemIcons, type Stack, itemRuleLines, stackRow } from './inventory';
import type { ItemTexts } from './itemText';
import type { VendingItem, VendingShop } from './online';

/** The Vending skill, MC_VENDING. */
export const VENDING_SKILL = 41;
/** The longest shop title, in bytes, as MESSAGE_SIZE - 1. */
export const VENDING_TITLE_MAX = 79;
/** The highest price, as rAthena's vending_max_value. */
export const VENDING_MAX_PRICE = 1_000_000_000;

/** A line of a shop being set up. */
export interface VendingDraftLine {
  cartSlot: number;
  amount: number;
  price: number;
}

/** Why a cart stack cannot be vended, or undefined when it can. */
export function vendProblem(s: Pick<Stack, 'bound' | 'restrictions'>): string | undefined {
  if ((s.restrictions & ItemRestriction.NO_TRADE) !== 0) {
    return 'This item cannot be vended.';
  }
  if (s.bound !== ItemBound.UNSPECIFIED) {
    return 'A bound item cannot be vended.';
  }
  return undefined;
}

/** Why a shop cannot open, or undefined when it can. */
export function vendingOpenProblem(
  title: string,
  lines: readonly VendingDraftLine[],
  maxItems: number,
  cart: (slot: number) => Stack | undefined,
): string | undefined {
  const t = bytesProblem('The title', title.trim(), 1, VENDING_TITLE_MAX);
  if (t !== undefined) {
    return t;
  }
  if (lines.length === 0) {
    return 'Choose at least one item to sell.';
  }
  if (lines.length > maxItems) {
    return `Your shop holds at most ${String(maxItems)} items.`;
  }
  const seen = new Set<number>();
  for (const l of lines) {
    const s = cart(l.cartSlot);
    if (s === undefined || seen.has(l.cartSlot)) {
      return 'An item is no longer in your cart.';
    }
    seen.add(l.cartSlot);
    const problem = vendProblem(s);
    if (problem !== undefined) {
      return problem;
    }
    if (l.amount < 1 || l.amount > s.amount) {
      return `You have ${String(s.amount)} of that item in the cart.`;
    }
    if (l.price < 1 || l.price > VENDING_MAX_PRICE) {
      return `A price is 1 to ${VENDING_MAX_PRICE.toLocaleString('en-US')} zeny.`;
    }
  }
  return undefined;
}

/** A line of a purchase from a shop. */
export interface VendingBuyLine {
  index: number;
  amount: number;
}

/** The zeny lines of a purchase cost. Lines not in the shop cost nothing. */
export function vendingTotal(
  items: readonly Pick<VendingItem, 'index' | 'price'>[],
  lines: readonly VendingBuyLine[],
): number {
  let total = 0;
  for (const l of lines) {
    const it = items.find((i) => i.index === l.index);
    if (it !== undefined) {
      total += it.price * l.amount;
    }
  }
  return total;
}

/** Why a purchase from a shop cannot go, or undefined when it can. */
export function vendingBuyProblem(
  items: readonly VendingItem[],
  lines: readonly VendingBuyLine[],
  zeny: number,
): string | undefined {
  if (lines.length === 0) {
    return 'Choose what to buy.';
  }
  for (const l of lines) {
    const it = items.find((i) => i.index === l.index);
    if (it === undefined) {
      return 'That item is no longer for sale.';
    }
    if (l.amount < 1 || l.amount > it.item.amount) {
      return `The shop has ${String(it.item.amount)} of that item.`;
    }
  }
  if (vendingTotal(items, lines) > zeny) {
    return 'You do not have enough zeny.';
  }
  return undefined;
}

/** The lines of a shop after a sale: the sold line loses amount, and goes at 0. */
export function applySale(
  items: readonly VendingItem[],
  index: number,
  amount: number,
): VendingItem[] {
  return items.flatMap((i) => {
    if (i.index !== index) {
      return [i];
    }
    const left = i.item.amount - amount;
    return left > 0 ? [{ ...i, item: { ...i.item, amount: left } }] : [];
  });
}

/** The chat log line for a sale of the player's shop. */
export function vendingSoldText(buyer: string, amount: number, item: string, zeny: number): string {
  return `${buyer} bought ${String(amount)} x ${item} for ${zeny.toLocaleString('en-US')} zeny.`;
}

/** What to tell the player about a vending step. message is a mod's reason. */
export function vendingMessage(o: VendingOutcome, message = ''): string {
  switch (o) {
    case VendingOutcome.OPENED:
      return 'Your shop is open.';
    case VendingOutcome.BOUGHT:
      return 'Thank you for your purchase.';
    case VendingOutcome.FAILED:
      return 'The shop action failed.';
    case VendingOutcome.NOT_ENOUGH_ZENY:
      return 'You do not have enough zeny.';
    case VendingOutcome.OVERWEIGHT:
      return 'You cannot carry that much.';
    case VendingOutcome.OUT_OF_STOCK:
      return 'The shop does not have that many left.';
    case VendingOutcome.STORE_CHANGED:
      return 'The shop changed. Browse it again.';
    case VendingOutcome.RESTRICTED:
      return 'An item cannot be vended.';
    case VendingOutcome.HOOK:
      return message === '' ? 'The sale is not allowed.' : message;
    case VendingOutcome.CLOSED:
      return 'The shop is closed.';
    case VendingOutcome.TOO_CLOSE:
      return 'You are too close to an NPC to open a shop.';
    default:
      return 'The shop action did nothing.';
  }
}

/** The head of a store window: its title and a Close button. */
function windowHead(title: HTMLElement, onClose: () => void, closeLabel = 'Close'): HTMLDivElement {
  const head = element('div', 'store-head');
  head.append(title, button(closeLabel, onClose));
  return head;
}

/** What the setup window asks the server to do. */
export interface VendingSetupActions {
  open: (title: string, lines: VendingDraftLine[]) => void;
  cancel: () => void;
}

/**
 * The setup window, section#vending-setup. It lists the cart's items with
 * an amount and a price input each.
 */
export class VendingSetupWindow {
  private readonly title = textInput('Shop title', 30);
  private readonly titleCount = element('output', 'store-count');
  private readonly list = element('ul', 'store-items');
  private readonly info = element('output', 'store-info');
  private readonly problem = element('output', 'store-problem');
  private rows: { slot: number; amount: HTMLInputElement; price: HTMLInputElement }[] = [];
  private maxItems = 0;

  constructor(
    private readonly root: HTMLElement,
    private readonly cart: () => readonly Stack[],
    private readonly actions: VendingSetupActions,
    private readonly texts: ItemTexts,
    private readonly icons: ItemIcons | undefined,
  ) {
    this.title.maxLength = VENDING_TITLE_MAX;
    const buttons = element('div', 'store-buttons');
    buttons.append(
      button('Open', () => {
        this.submit();
      }),
      button('Cancel', () => {
        this.cancel();
      }),
    );
    this.list.setAttribute('aria-label', 'Cart items');
    root.replaceChildren(
      windowHead(element('h2', '', 'Open a shop'), () => {
        this.cancel();
      }),
      field('Title', this.title),
      this.titleCount,
      this.info,
      this.list,
      this.problem,
      buttons,
    );
    root.addEventListener('input', () => {
      this.refresh();
    });
    keepKeys(root, () => {
      this.cancel();
    });
    root.hidden = true;
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /** Opens the window, as VendingSetup. */
  show(maxItems: number): void {
    this.maxItems = maxItems;
    this.rows = [];
    const label = (id: number): string => this.texts.label(id);
    const items = this.cart().map((s) => {
      const amount = numberInput('0', 5);
      amount.setAttribute('aria-label', `Amount of ${label(s.itemId)}, 0 to ${String(s.amount)}`);
      const price = numberInput('Price', 10);
      price.setAttribute('aria-label', `Price of one ${label(s.itemId)}`);
      const rules = itemRuleLines(s);
      const problem = vendProblem(s);
      amount.disabled = problem !== undefined;
      price.disabled = problem !== undefined;
      const li = stackRow(s, label, this.icons, amount, price);
      if (problem !== undefined) {
        li.classList.add('unavailable');
        li.append(element('span', 'store-rule', rules.join(', ')));
      }
      this.rows.push({ slot: s.slot, amount, price });
      return li;
    });
    if (items.length === 0) {
      items.push(element('li', 'empty', 'Your cart is empty.'));
    }
    this.list.replaceChildren(...items);
    this.problem.textContent = '';
    this.root.hidden = false;
    this.refresh();
    this.title.focus();
  }

  /** Hides the window without telling the server. */
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

  /** The lines the inputs give, or a problem. */
  private lines(): VendingDraftLine[] | string {
    const out: VendingDraftLine[] = [];
    for (const r of this.rows) {
      if (r.amount.value.trim() === '' || r.amount.value.trim() === '0') {
        continue;
      }
      const amount = parseWhole(r.amount.value, 1, 30000);
      const price = parseWhole(r.price.value, 1, VENDING_MAX_PRICE);
      if (amount === undefined) {
        return 'Amounts are whole numbers.';
      }
      if (price === undefined) {
        return `A price is 1 to ${VENDING_MAX_PRICE.toLocaleString('en-US')} zeny.`;
      }
      out.push({ cartSlot: r.slot, amount, price });
    }
    return out;
  }

  private refresh(): void {
    this.titleCount.textContent = `${String(utf8Length(this.title.value))} / ${String(VENDING_TITLE_MAX)} bytes`;
    const lines = this.lines();
    const count = typeof lines === 'string' ? 0 : lines.length;
    this.info.textContent = `Items ${String(count)} / ${String(this.maxItems)}`;
  }

  private submit(): void {
    const lines = this.lines();
    if (typeof lines === 'string') {
      this.problem.textContent = lines;
      return;
    }
    const all = this.cart();
    const problem = vendingOpenProblem(this.title.value, lines, this.maxItems, (slot) =>
      all.find((s) => s.slot === slot),
    );
    if (problem !== undefined) {
      this.problem.textContent = problem;
      return;
    }
    this.actions.open(this.title.value.trim(), lines);
    this.hide();
  }
}

/**
 * The player's own open shop, section#vending-own: its title, what is
 * left with the prices, and Close shop.
 */
export class VendingOwnWindow {
  private readonly title = element('h2');
  private readonly list = element('ul', 'store-items');
  private items: VendingItem[] = [];

  constructor(
    private readonly root: HTMLElement,
    closeShop: () => void,
    private readonly texts: ItemTexts,
    private readonly icons: ItemIcons | undefined,
  ) {
    const hide = button('Hide', () => {
      this.root.hidden = true;
    });
    const head = windowHead(this.title, closeShop, 'Close shop');
    head.append(hide);
    this.list.setAttribute('aria-label', 'Items for sale');
    root.replaceChildren(head, this.list);
    keepKeys(root, () => {
      this.root.hidden = true;
    });
    root.hidden = true;
  }

  /** Whether the player's shop is open. */
  get shopOpen(): boolean {
    return this.title.textContent !== '';
  }

  /** Shows the shop, as VendingOpened. */
  show(title: string, items: readonly VendingItem[]): void {
    this.title.textContent = title;
    this.items = [...items];
    this.root.hidden = false;
    this.render();
  }

  /** Takes a sale off the list, as VendingSold. Returns the sold item's ID, or 0. */
  sold(index: number, amount: number): number {
    const itemId = this.items.find((i) => i.index === index)?.item.itemId ?? 0;
    this.items = applySale(this.items, index, amount);
    this.render();
    return itemId;
  }

  /** Forgets the shop, as when it closes. */
  closed(): void {
    this.title.textContent = '';
    this.items = [];
    this.root.hidden = true;
  }

  /** Shows or hides the window while the shop is open. */
  toggle(): void {
    if (this.shopOpen) {
      this.root.hidden = !this.root.hidden;
    }
  }

  private render(): void {
    const label = (id: number): string => this.texts.label(id);
    const rows = this.items.map((i) =>
      stackRow(i.item, label, this.icons, element('span', 'value', zenyLabel(i.price))),
    );
    if (rows.length === 0) {
      rows.push(element('li', 'empty', 'Everything sold.'));
    }
    this.list.replaceChildren(...rows);
  }
}

/** What the browse window asks the server to do. */
export interface VendingBrowseActions {
  buy: (vendor: number, shop: number, lines: VendingBuyLine[]) => void;
}

/**
 * Another player's shop, section#vending-browse: its title and seller,
 * its items with a price and an amount input each, the total, and Buy.
 */
export class VendingBrowseWindow {
  private readonly title = element('h2');
  private readonly seller = element('div', 'store-owner');
  private readonly list = element('ul', 'store-items');
  private readonly total = element('output', 'store-total');
  private readonly problem = element('output', 'store-problem');
  private shop: VendingShop | undefined;
  private inputs: { index: number; input: HTMLInputElement; max: number }[] = [];

  constructor(
    private readonly root: HTMLElement,
    private readonly zeny: () => number,
    private readonly actions: VendingBrowseActions,
    private readonly texts: ItemTexts,
    private readonly icons: ItemIcons | undefined,
  ) {
    const buttons = element('div', 'store-buttons');
    buttons.append(
      button('Buy', () => {
        this.buy();
      }),
    );
    this.list.setAttribute('aria-label', 'Items for sale');
    root.replaceChildren(
      windowHead(this.title, () => {
        this.hide();
      }),
      this.seller,
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

  /** Shows a shop, as VendingList. */
  show(shop: VendingShop): void {
    this.shop = shop;
    this.title.textContent = shop.title;
    this.seller.textContent = `Seller: ${shop.seller}`;
    const label = (id: number): string => this.texts.label(id);
    this.inputs = [];
    const rows = shop.items.map((i) => {
      const input = numberInput('0', 5);
      input.setAttribute(
        'aria-label',
        `Amount of ${label(i.item.itemId)}, 0 to ${String(i.item.amount)}`,
      );
      this.inputs.push({ index: i.index, input, max: i.item.amount });
      return stackRow(
        i.item,
        label,
        this.icons,
        element('span', 'value', zenyLabel(i.price)),
        input,
      );
    });
    if (rows.length === 0) {
      rows.push(element('li', 'empty', 'Nothing is for sale.'));
    }
    this.list.replaceChildren(...rows);
    this.problem.textContent = '';
    this.root.hidden = false;
    this.refresh();
  }

  hide(): void {
    this.root.hidden = true;
    this.shop = undefined;
  }

  /** The lines the inputs give, or undefined when one is not a whole number in range. */
  private lines(): VendingBuyLine[] | undefined {
    const out: VendingBuyLine[] = [];
    for (const { index, input, max } of this.inputs) {
      const t = input.value.trim();
      if (t === '' || t === '0') {
        continue;
      }
      const n = parseWhole(t, 1, max);
      if (n === undefined) {
        return undefined;
      }
      out.push({ index, amount: n });
    }
    return out;
  }

  private refresh(): void {
    const lines = this.lines();
    if (lines === undefined || this.shop === undefined) {
      this.problem.textContent = 'Amounts are whole numbers, up to what the shop has.';
      return;
    }
    this.total.textContent = `Total ${zenyLabel(vendingTotal(this.shop.items, lines))} · You have ${zenyLabel(this.zeny())}`;
    this.problem.textContent =
      lines.length > 0 ? (vendingBuyProblem(this.shop.items, lines, this.zeny()) ?? '') : '';
  }

  private buy(): void {
    const lines = this.lines();
    const shop = this.shop;
    if (lines === undefined || shop === undefined) {
      return;
    }
    const problem = vendingBuyProblem(shop.items, lines, this.zeny());
    if (problem !== undefined) {
      this.problem.textContent = problem;
      return;
    }
    this.actions.buy(shop.vendor, shop.shop, lines);
    this.hide();
  }
}
