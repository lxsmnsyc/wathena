import type { ShopOpen } from '../gen/wathena/v1/event_pb';
import { ShopOutcome, StatId } from '../gen/wathena/v1/event_pb';
import type { InventoryModel } from './inventory';
import type { ItemTexts } from './itemText';

// The NPC shop window, as RO's buy and sell windows in one: what the NPC
// sells with its price and an amount to buy, and what the player may
// sell with its price and an amount. The totals show the zeny and the
// weight, and the window refuses a purchase that the player cannot pay
// or carry, as the server would.

/** One line of a purchase or a sale. */
export interface ShopLine {
  /** An item ID to buy, or an inventory slot to sell. */
  key: number;
  amount: number;
  /** Zeny for one. */
  price: number;
  /** Weight of one, in tenths. Sales lose weight, which the window does not check. */
  weight: number;
}

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

/** Why a purchase cannot go, or undefined when it can. */
export function buyProblem(
  lines: readonly ShopLine[],
  stats: ReadonlyMap<StatId, bigint>,
): string | undefined {
  if (lines.length === 0) {
    return 'Choose what to buy.';
  }
  const t = shopTotals(lines);
  const zeny = Number(stats.get(StatId.ZENY) ?? 0n);
  if (t.zeny > zeny) {
    return 'You do not have enough zeny.';
  }
  const weight = Number(stats.get(StatId.WEIGHT) ?? 0n);
  const max = stats.get(StatId.MAX_WEIGHT);
  if (max !== undefined && weight + t.weight > Number(max)) {
    return 'You cannot carry that much.';
  }
  return undefined;
}

/** What to tell the player about a shop's answer. */
export function shopMessage(o: ShopOutcome): string | undefined {
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

/** What the window asks the server to do. */
export interface ShopActions {
  buy: (npc: number, lines: { itemId: number; amount: number }[]) => void;
  sell: (npc: number, lines: { slot: number; amount: number }[]) => void;
  close: (npc: number) => void;
}

/** The shop window on the page. */
export class ShopWindow {
  private shop: ShopOpen | undefined;
  private readonly buyList: HTMLTableSectionElement;
  private readonly sellList: HTMLTableSectionElement;
  private readonly total: HTMLOutputElement;
  private readonly problem: HTMLOutputElement;
  private buyInputs: { input: HTMLInputElement; line: ShopLine }[] = [];
  private sellInputs: { input: HTMLInputElement; line: ShopLine }[] = [];
  /** The player's stats, for the totals. */
  stats: ReadonlyMap<StatId, bigint> = new Map();

  constructor(
    private readonly root: HTMLElement,
    private readonly inventory: InventoryModel,
    private readonly texts: ItemTexts,
    private readonly actions: ShopActions,
  ) {
    const table = (caption: string): HTMLTableSectionElement => {
      const t = document.createElement('table');
      const c = t.createCaption();
      c.textContent = caption;
      const head = t.createTHead().insertRow();
      for (const h of ['Item', 'Price', 'Amount']) {
        const th = document.createElement('th');
        th.textContent = h;
        head.append(th);
      }
      root.append(t);
      return t.createTBody();
    };
    this.buyList = table('Buy');
    this.sellList = table('Sell');
    this.total = document.createElement('output');
    this.total.className = 'shop-total';
    this.problem = document.createElement('output');
    this.problem.className = 'shop-problem';
    const buttons = document.createElement('div');
    for (const [label, f] of [
      [
        'Buy',
        () => {
          this.buy();
        },
      ],
      [
        'Sell',
        () => {
          this.sell();
        },
      ],
      [
        'Close',
        () => {
          this.close(true);
        },
      ],
    ] as const) {
      const b = document.createElement('button');
      b.type = 'button';
      b.textContent = label;
      b.addEventListener('click', f);
      buttons.append(b);
    }
    root.append(this.total, this.problem, buttons);
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

  /** Opens the window for a shop the server described. */
  show(s: ShopOpen): void {
    this.shop = s;
    this.buyList.replaceChildren();
    this.sellList.replaceChildren();
    this.buyInputs = [];
    this.sellInputs = [];
    for (const it of s.items) {
      const line = { key: it.itemId, amount: 0, price: it.price, weight: it.weight };
      this.buyInputs.push({ input: this.row(this.buyList, it.itemId, it.price, 30000), line });
    }
    for (const it of s.sellable) {
      const stack = this.inventory.get(it.slot);
      if (stack === undefined) {
        continue;
      }
      const line = { key: it.slot, amount: 0, price: it.price, weight: 0 };
      this.sellInputs.push({
        input: this.row(this.sellList, stack.itemId, it.price, stack.amount),
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

  private row(
    body: HTMLTableSectionElement,
    itemId: number,
    price: number,
    max: number,
  ): HTMLInputElement {
    const r = body.insertRow();
    r.insertCell().textContent = this.texts.label(itemId);
    const p = r.insertCell();
    p.className = 'value';
    p.textContent = `${price.toLocaleString('en-US')} z`;
    const input = document.createElement('input');
    input.inputMode = 'numeric';
    input.placeholder = '0';
    input.size = 5;
    input.setAttribute('aria-label', `Amount of ${this.texts.label(itemId)}, 0 to ${max}`);
    input.dataset.max = String(max);
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
    this.total.value = `Zeny ${zeny.toLocaleString('en-US')} · Buy ${b.zeny.toLocaleString('en-US')} z, weight ${b.weight / 10} · Sell ${s.zeny.toLocaleString('en-US')} z`;
    this.problem.value = buy.length > 0 ? (buyProblem(buy, this.stats) ?? '') : '';
  }

  private buy(): void {
    const lines = readLines(this.buyInputs);
    if (this.shop === undefined || lines === undefined) {
      return;
    }
    const problem = buyProblem(lines, this.stats);
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
