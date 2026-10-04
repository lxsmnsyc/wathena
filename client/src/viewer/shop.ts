import type { ShopOpen } from '../gen/wathena/v1/event_pb';
import { ShopOutcome, StatId } from '../gen/wathena/v1/event_pb';
import { zenyLabel } from './forms';
import {
  type CurrencyBalance,
  type InventoryModel,
  type ItemIcons,
  currencyBalance,
} from './inventory';
import type { ItemTexts } from './itemText';

// The NPC shop window, as RO's buy and sell windows in one: what the NPC
// sells with its cost and an amount to buy, and what the player may sell
// with its price and an amount. A shop of data/npcs.yml may cost items
// and currencies beside zeny, and may limit its stock. Each line shows
// its whole cost, with how much of each the player has. The totals show
// the zeny and the weight, and the window refuses a purchase that the
// player cannot pay or carry, as the server would. The sell part shows
// only for a shop that buys from players.

/** An item a purchase takes, for one bought. */
export interface CostItem {
  itemId: number;
  amount: number;
}

/** A currency a purchase takes, for one bought. */
export interface CostCurrency {
  /** The record's full name, such as cash or example.arena:tokens. */
  currency: string;
  displayName: string;
  amount: number;
}

/** One line of a purchase or a sale. */
export interface ShopLine {
  /** An item ID to buy, or an inventory slot to sell. */
  key: number;
  amount: number;
  /** Zeny for one. */
  price: number;
  /** Weight of one, in tenths. Sales lose weight, which the window does not check. */
  weight: number;
  /** The items one bought takes. */
  costItems?: readonly CostItem[];
  /** The currencies one bought takes. */
  costCurrencies?: readonly CostCurrency[];
  /** How many the shop has left, when it limits the item. */
  stock?: number;
}

/** What the player has to pay with, beside zeny. */
export interface ShopWallet {
  /** How many of an item the inventory holds, leaving out worn items. */
  itemCount: (itemId: number) => number;
  /** The player's balance of a currency by its full name. */
  currency: (name: string) => bigint;
  /** An item's name, for messages. */
  label: (itemId: number) => string;
}

/** A wallet with no items and no currencies. */
export const EMPTY_WALLET: ShopWallet = {
  itemCount: () => 0,
  currency: () => 0n,
  label: (id) => `Item ${String(id)}`,
};

/** The zeny and weight of lines. */
export function shopTotals(lines: readonly ShopLine[]): { zeny: number; weight: number } {
  let zeny = 0;
  let weight = 0;
  for (const l of lines) {
    zeny += l.price * l.amount;
    weight += l.weight * l.amount;
  }
  return { zeny, weight };
}

/** A currency's name as players see it. */
function currencyLabel(c: { currency: string; displayName: string }): string {
  return c.displayName === '' ? c.currency : c.displayName;
}

/** The items and currencies lines take, summed by item ID and by currency name. */
export function shopCosts(lines: readonly ShopLine[]): {
  items: Map<number, number>;
  currencies: Map<string, CostCurrency>;
} {
  const items = new Map<number, number>();
  const currencies = new Map<string, CostCurrency>();
  for (const l of lines) {
    for (const c of l.costItems ?? []) {
      items.set(c.itemId, (items.get(c.itemId) ?? 0) + c.amount * l.amount);
    }
    for (const c of l.costCurrencies ?? []) {
      const had = currencies.get(c.currency)?.amount ?? 0;
      currencies.set(c.currency, {
        currency: c.currency,
        displayName: c.displayName,
        amount: had + c.amount * l.amount,
      });
    }
  }
  return { items, currencies };
}

/** Why a purchase cannot go, or undefined when it can. */
export function buyProblem(
  lines: readonly ShopLine[],
  stats: ReadonlyMap<StatId, bigint>,
  wallet: ShopWallet = EMPTY_WALLET,
): string | undefined {
  if (lines.length === 0) {
    return 'Choose what to buy.';
  }
  for (const l of lines) {
    if (l.stock !== undefined && l.amount > l.stock) {
      return l.stock === 0
        ? `${wallet.label(l.key)} is sold out.`
        : `Only ${String(l.stock)} ${wallet.label(l.key)} are left.`;
    }
  }
  const t = shopTotals(lines);
  const zeny = Number(stats.get(StatId.ZENY) ?? 0n);
  if (t.zeny > zeny) {
    return 'You do not have enough zeny.';
  }
  const costs = shopCosts(lines);
  for (const [itemId, need] of costs.items) {
    const have = wallet.itemCount(itemId);
    if (have < need) {
      return `You need ${String(need)} ${wallet.label(itemId)} and have ${String(have)}.`;
    }
  }
  for (const c of costs.currencies.values()) {
    const have = wallet.currency(c.currency);
    if (have < BigInt(c.amount)) {
      return `You need ${String(c.amount)} ${currencyLabel(c)} and have ${have.toString()}.`;
    }
  }
  const weight = Number(stats.get(StatId.WEIGHT) ?? 0n);
  const max = stats.get(StatId.MAX_WEIGHT);
  if (max !== undefined && weight + t.weight > Number(max)) {
    return 'You cannot carry that much.';
  }
  return undefined;
}

/**
 * The parts of one item's cost, such as "50 z", "Jellopy × 2 (you have
 * 5)" and "Arena tokens × 3 (you have 10)". A price of 0 with other costs
 * leaves the zeny out.
 */
export function costParts(
  line: Pick<ShopLine, 'price' | 'costItems' | 'costCurrencies'>,
  wallet: ShopWallet,
): string[] {
  const out: string[] = [];
  const items = line.costItems ?? [];
  const currencies = line.costCurrencies ?? [];
  if (line.price > 0 || (items.length === 0 && currencies.length === 0)) {
    out.push(zenyLabel(line.price));
  }
  for (const c of items) {
    out.push(
      `${wallet.label(c.itemId)} × ${String(c.amount)} (you have ${String(wallet.itemCount(c.itemId))})`,
    );
  }
  for (const c of currencies) {
    out.push(
      `${currencyLabel(c)} × ${String(c.amount)} (you have ${wallet.currency(c.currency).toLocaleString('en-US')})`,
    );
  }
  return out;
}

/** A line's stock, such as "12 left", or empty when the shop does not limit it. */
export function stockText(stock: number | undefined): string {
  if (stock === undefined) {
    return '';
  }
  return stock === 0 ? 'Sold out' : `${String(stock)} left`;
}

/** What to tell the player about a shop's answer. item names the item it is about, if any. */
export function shopMessage(o: ShopOutcome, item = ''): string | undefined {
  switch (o) {
    case ShopOutcome.BOUGHT:
      return 'Thank you for your purchase.';
    case ShopOutcome.SOLD:
      return 'Thank you for your sale.';
    case ShopOutcome.NOT_ENOUGH_ZENY:
      return 'You do not have enough zeny.';
    case ShopOutcome.OVERWEIGHT:
      return 'You cannot carry that much.';
    case ShopOutcome.FAILED:
      return 'The trade did not go through.';
    case ShopOutcome.NOT_ENOUGH_ITEMS:
      return item === ''
        ? 'You do not have the items the purchase costs.'
        : `You do not have enough ${item} for the purchase.`;
    case ShopOutcome.NOT_ENOUGH_CURRENCY:
      return 'You do not have enough of a currency the purchase costs.';
    case ShopOutcome.OUT_OF_STOCK:
      return item === ''
        ? 'The shop does not have that many left.'
        : `The shop does not have that many ${item} left.`;
    default:
      return undefined;
  }
}

/** Reads an amount input: a whole number from 0 to max. */
export function parseAmount(text: string, max: number): number | undefined {
  const t = text.trim();
  if (t === '') {
    return 0;
  }
  if (!/^\d{1,5}$/.test(t)) {
    return undefined;
  }
  const n = Number(t);
  return n <= max ? n : undefined;
}

/** The most of one item a purchase line asks for. */
const MAX_BUY = 30000;

/** What the window asks the server to do. */
export interface ShopActions {
  buy: (npc: number, lines: { itemId: number; amount: number }[]) => void;
  sell: (npc: number, lines: { slot: number; amount: number }[]) => void;
  close: (npc: number) => void;
}

/**
 * The shop window on the page, section#shop-window. It holds h2 with the
 * shop's name, table.shop-buy and table.shop-sell, the totals, and the
 * Buy, Sell and Close buttons.
 */
export class ShopWindow {
  private shop: ShopOpen | undefined;
  private readonly title = document.createElement('h2');
  private readonly buyList: HTMLTableSectionElement;
  private readonly sellTable: HTMLTableElement;
  private readonly sellList: HTMLTableSectionElement;
  private readonly sellButton = document.createElement('button');
  private readonly total: HTMLOutputElement;
  private readonly problem: HTMLOutputElement;
  private buyInputs: { input: HTMLInputElement; line: ShopLine }[] = [];
  private sellInputs: { input: HTMLInputElement; line: ShopLine }[] = [];
  /** The player's stats, for the totals. */
  stats: ReadonlyMap<StatId, bigint> = new Map();
  /** The player's currencies, from the last CurrencyUpdate. */
  currencies: readonly CurrencyBalance[] = [];

  constructor(
    private readonly root: HTMLElement,
    private readonly inventory: InventoryModel,
    private readonly texts: ItemTexts,
    private readonly actions: ShopActions,
    private readonly icons?: ItemIcons,
  ) {
    const table = (caption: string, heads: string[]): HTMLTableElement => {
      const t = document.createElement('table');
      const c = t.createCaption();
      c.textContent = caption;
      const head = t.createTHead().insertRow();
      for (const h of heads) {
        const th = document.createElement('th');
        th.textContent = h;
        head.append(th);
      }
      return t;
    };
    this.title.className = 'shop-name';
    const buyTable = table('Buy', ['Item', 'Cost', 'Stock', 'Amount']);
    buyTable.className = 'shop-buy';
    this.buyList = buyTable.createTBody();
    this.sellTable = table('Sell', ['Item', 'Price', 'Amount']);
    this.sellTable.className = 'shop-sell';
    this.sellList = this.sellTable.createTBody();
    this.total = document.createElement('output');
    this.total.className = 'shop-total';
    this.problem = document.createElement('output');
    this.problem.className = 'shop-problem';
    const buttons = document.createElement('div');
    const make = (b: HTMLButtonElement, label: string, f: () => void): HTMLButtonElement => {
      b.type = 'button';
      b.textContent = label;
      b.addEventListener('click', f);
      return b;
    };
    buttons.append(
      make(document.createElement('button'), 'Buy', () => {
        this.buy();
      }),
      make(this.sellButton, 'Sell', () => {
        this.sell();
      }),
      make(document.createElement('button'), 'Close', () => {
        this.close(true);
      }),
    );
    root.append(this.title, buyTable, this.sellTable, this.total, this.problem, buttons);
    root.hidden = true;
    root.addEventListener('keydown', (e) => {
      e.stopPropagation();
      if (e.key === 'Escape') {
        this.close(true);
      }
    });
    root.addEventListener('input', () => {
      this.refresh();
    });
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /** What the player has, for the cost checks. */
  private wallet(): ShopWallet {
    return {
      itemCount: (id) => this.inventory.count(id),
      currency: (name) => currencyBalance(this.currencies, name),
      label: (id) => this.texts.label(id),
    };
  }

  /** Opens the window for a shop the server described. */
  show(s: ShopOpen): void {
    this.shop = s;
    this.title.textContent = s.name === '' ? 'Shop' : s.name;
    this.buyList.replaceChildren();
    this.sellList.replaceChildren();
    this.buyInputs = [];
    this.sellInputs = [];
    const wallet = this.wallet();
    for (const it of s.items) {
      const line: ShopLine = {
        key: it.itemId,
        amount: 0,
        price: it.price,
        weight: it.weight,
        costItems: it.costItems.map((c) => ({ itemId: c.itemId, amount: c.amount })),
        costCurrencies: it.costCurrencies.map((c) => ({
          currency: c.currency,
          displayName: c.displayName,
          amount: c.amount,
        })),
      };
      if (it.stock !== undefined) {
        line.stock = it.stock;
      }
      const max = Math.min(MAX_BUY, it.stock ?? MAX_BUY);
      this.buyInputs.push({ input: this.buyRow(line, wallet, max), line });
    }
    // The sell part shows for a shop that buys from players.
    this.sellTable.hidden = !s.buysFromPlayers;
    this.sellButton.hidden = !s.buysFromPlayers;
    for (const it of s.buysFromPlayers ? s.sellable : []) {
      const stack = this.inventory.get(it.slot);
      if (stack === undefined) {
        continue;
      }
      const line = { key: it.slot, amount: 0, price: it.price, weight: 0 };
      this.sellInputs.push({
        input: this.sellRow(stack.itemId, it.price, stack.amount),
        line,
      });
    }
    this.root.hidden = false;
    this.refresh();
  }

  /** Closes the window. With tell, the server hears of it. */
  close(tell: boolean): void {
    if (this.shop !== undefined && tell) {
      this.actions.close(this.shop.npc);
    }
    this.shop = undefined;
    this.root.hidden = true;
  }

  private amountInput(itemId: number, max: number): HTMLInputElement {
    const input = document.createElement('input');
    input.inputMode = 'numeric';
    input.placeholder = '0';
    input.size = 5;
    input.setAttribute('aria-label', `Amount of ${this.texts.label(itemId)}, 0 to ${max}`);
    input.dataset.max = String(max);
    input.disabled = max === 0;
    return input;
  }

  private itemCell(row: HTMLTableRowElement, itemId: number): void {
    const cell = row.insertCell();
    cell.className = 'shop-item';
    if (this.icons !== undefined) {
      const img = document.createElement('img');
      img.alt = '';
      img.width = 24;
      img.height = 24;
      this.icons.show(itemId, img);
      cell.append(img);
    }
    cell.append(this.texts.label(itemId));
  }

  private buyRow(line: ShopLine, wallet: ShopWallet, max: number): HTMLInputElement {
    const r = this.buyList.insertRow();
    this.itemCell(r, line.key);
    const cost = r.insertCell();
    cost.className = 'shop-cost';
    for (const part of costParts(line, wallet)) {
      const div = document.createElement('div');
      div.textContent = part;
      cost.append(div);
    }
    const stock = r.insertCell();
    stock.className = 'value';
    stock.textContent = stockText(line.stock);
    const input = this.amountInput(line.key, max);
    r.insertCell().append(input);
    return input;
  }

  private sellRow(itemId: number, price: number, max: number): HTMLInputElement {
    const r = this.sellList.insertRow();
    this.itemCell(r, itemId);
    const p = r.insertCell();
    p.className = 'value';
    p.textContent = zenyLabel(price);
    const input = this.amountInput(itemId, max);
    r.insertCell().append(input);
    return input;
  }

  private refresh(): void {
    const buy = readLines(this.buyInputs);
    const sell = readLines(this.sellInputs);
    if (buy === undefined || sell === undefined) {
      this.problem.value = 'Amounts are whole numbers, up to what you have.';
      return;
    }
    const b = shopTotals(buy);
    const s = shopTotals(sell);
    const zeny = Number(this.stats.get(StatId.ZENY) ?? 0n);
    const costs = shopCosts(buy);
    const buyText = [
      zenyLabel(b.zeny),
      ...[...costs.items].map(([id, n]) => `${this.texts.label(id)} × ${String(n)}`),
      ...[...costs.currencies.values()].map((c) => `${currencyLabel(c)} × ${String(c.amount)}`),
    ].join(' + ');
    const sellText = this.sellTable.hidden ? '' : ` · Sell ${zenyLabel(s.zeny)}`;
    this.total.value = `Zeny ${zeny.toLocaleString('en-US')} · Buy ${buyText}, weight ${b.weight / 10}${sellText}`;
    this.problem.value = buy.length > 0 ? (buyProblem(buy, this.stats, this.wallet()) ?? '') : '';
  }

  private buy(): void {
    const lines = readLines(this.buyInputs);
    if (this.shop === undefined || lines === undefined) {
      return;
    }
    const problem = buyProblem(lines, this.stats, this.wallet());
    if (problem !== undefined) {
      this.problem.value = problem;
      return;
    }
    this.actions.buy(
      this.shop.npc,
      lines.map((l) => ({ itemId: l.key, amount: l.amount })),
    );
    this.close(false);
  }

  private sell(): void {
    const lines = readLines(this.sellInputs);
    if (this.shop === undefined || lines === undefined || lines.length === 0) {
      this.problem.value = 'Choose what to sell.';
      return;
    }
    this.actions.sell(
      this.shop.npc,
      lines.map((l) => ({ slot: l.key, amount: l.amount })),
    );
    this.close(false);
  }
}

/** Reads the amounts of a window's rows into lines, or undefined when one is not a whole number in range. */
function readLines(inputs: { input: HTMLInputElement; line: ShopLine }[]): ShopLine[] | undefined {
  const out: ShopLine[] = [];
  for (const { input, line } of inputs) {
    const n = parseAmount(input.value, Number(input.dataset.max));
    if (n === undefined) {
      return undefined;
    }
    if (n > 0) {
      out.push({ ...line, amount: n });
    }
  }
  return out;
}
