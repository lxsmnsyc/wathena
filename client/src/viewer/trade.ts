// The trade window, as RO's trade window. A trade starts from the player
// menu: the other player gets a prompt to accept or decline, which
// declines by itself after 30 seconds. When both agree, the window opens
// with two panes, the player's offer and the partner's, of up to
// TRADE_ITEMS items and an amount of zeny each.
//
// Items are offered by dragging an inventory row onto the window, or by a
// right click on the row. A stack of more than one asks how many first.
// Zeny is offered once. Lock (OK) ends the player's offer. When both
// offers are locked, Trade completes the trade once both players pressed
// it. Cancel ends the trade. The chat log says how each step went. User
// text is only ever set with textContent.

import { TradeOutcome } from '../gen/wathena/v1/event_pb';
import { stackLabel } from './equipment';
import {
  type ItemIcons,
  SLOT_DRAG_TYPE,
  type Stack,
  itemTooltip,
  parseDropAmount,
  stack,
} from './inventory';
import type { ItemTexts } from './itemText';
import type { TradeOfferValue } from './online';

/** The most items each player offers, as rAthena's 10 trade slots. */
export const TRADE_ITEMS = 10;

/** One player's offer. */
export interface TradeSide {
  /**
   * The offered items in the order they were offered. For the player's
   * own offer, slot is the inventory slot. amount is the amount offered.
   */
  items: readonly Stack[];
  zeny: number;
  locked: boolean;
}

/** The trade as the server last described it. */
export interface TradeState {
  open: boolean;
  partner: string;
  baseLevel: number;
  mine: TradeSide;
  theirs: TradeSide;
  /** Whether the player pressed Trade. */
  confirmed: boolean;
}

const EMPTY_SIDE: TradeSide = { items: [], zeny: 0, locked: false };

/** No trade. */
export const NO_TRADE: TradeState = {
  open: false,
  partner: '',
  baseLevel: 0,
  mine: EMPTY_SIDE,
  theirs: EMPTY_SIDE,
  confirmed: false,
};

/** What changes a trade: a server event, or the player's Trade press. */
export type TradeEvent =
  | { type: 'open'; partner: string; baseLevel: number }
  | { type: 'offer'; mine: boolean; offer: TradeOfferValue }
  | { type: 'lock'; mine: boolean }
  | { type: 'confirm' }
  | { type: 'result'; outcome: TradeOutcome }
  | { type: 'close' };

/** The trade after an event. The state passed in is not changed. */
export function tradeReducer(s: TradeState, e: TradeEvent): TradeState {
  switch (e.type) {
    case 'open':
      return { ...NO_TRADE, open: true, partner: e.partner, baseLevel: e.baseLevel };
    case 'close':
      return NO_TRADE;
    case 'offer': {
      if (!s.open) {
        return s;
      }
      const side = e.mine ? s.mine : s.theirs;
      const next: TradeSide =
        e.offer.case === 'item'
          ? { ...side, items: [...side.items, stack(e.offer.value)] }
          : { ...side, zeny: e.offer.value };
      return e.mine ? { ...s, mine: next } : { ...s, theirs: next };
    }
    case 'lock':
      if (!s.open) {
        return s;
      }
      return e.mine
        ? { ...s, mine: { ...s.mine, locked: true } }
        : { ...s, theirs: { ...s.theirs, locked: true } };
    case 'confirm':
      return s.open && canConfirm(s) ? { ...s, confirmed: true } : s;
    case 'result':
      return s.open && e.outcome === TradeOutcome.WAITING ? { ...s, confirmed: true } : s;
    default:
      return s;
  }
}

/** How much of an inventory slot the player offered. */
export function offeredAmount(s: TradeState, slot: number): number {
  let n = 0;
  for (const i of s.mine.items) {
    if (i.slot === slot) {
      n += i.amount;
    }
  }
  return n;
}

/** Whether the player can offer another item. */
export function canOffer(s: TradeState): boolean {
  return s.open && !s.mine.locked && s.mine.items.length < TRADE_ITEMS;
}

/** Whether the player can offer zeny: once, before locking. */
export function canOfferZeny(s: TradeState): boolean {
  return s.open && !s.mine.locked && s.mine.zeny === 0;
}

/** Whether Trade is enabled: both offers locked and the player has not pressed it. */
export function canConfirm(s: TradeState): boolean {
  return s.open && s.mine.locked && s.theirs.locked && !s.confirmed;
}

/** The prompt for a TradeRequest, such as "Bob (level 50) wants to trade with you." */
export function tradeRequestText(from: string, baseLevel: number): string {
  return `${from} (level ${String(baseLevel)}) wants to trade with you.`;
}

/** Zeny with thousands separators, such as "1,200,000". */
export function zenyText(zeny: number): string {
  return zeny.toLocaleString('en-US');
}

/**
 * What to tell the player about a trade step or its end. name is the
 * player the outcome is about, and message a mod's reason for HOOK.
 */
export function tradeMessage(outcome: TradeOutcome, name = '', message = ''): string {
  const who = name === '' ? 'That player' : name;
  switch (outcome) {
    case TradeOutcome.TOO_FAR:
      return `${who} is too far away to trade.`;
    case TradeOutcome.NOT_FOUND:
      return name === '' ? 'That player is not here.' : `${name} is not here.`;
    case TradeOutcome.BUSY:
      return `${who} is busy.`;
    case TradeOutcome.REFUSED:
      return name === '' ? 'The trade was refused.' : `${name} refused to trade.`;
    case TradeOutcome.CANCELLED:
      return 'The trade was cancelled.';
    case TradeOutcome.COMPLETED:
      return 'The trade is complete.';
    case TradeOutcome.FAILED:
      return 'The trade failed. Nothing changed hands.';
    case TradeOutcome.OVERWEIGHT:
      return `${who} cannot carry that much.`;
    case TradeOutcome.RESTRICTED:
      return 'This item cannot be traded.';
    case TradeOutcome.BOUND:
      return 'This bounded item cannot be traded to that character.';
    case TradeOutcome.TOO_MANY:
      return 'No more items can be added to the trade.';
    case TradeOutcome.LOCKED:
      return 'The offer is locked.';
    case TradeOutcome.ZENY:
      return 'That amount of zeny cannot be traded.';
    case TradeOutcome.HOOK:
      return message === '' ? 'The trade is not allowed.' : message;
    case TradeOutcome.WAITING:
      return name === ''
        ? 'Waiting for the other player to press Trade.'
        : `Waiting for ${name} to press Trade.`;
    case TradeOutcome.EQUIPPED:
      return 'An equipped item cannot be traded.';
    case TradeOutcome.REQUEST_SENT:
      return name === '' ? 'You asked to trade.' : `You asked ${name} to trade.`;
    default:
      return 'The trade did nothing.';
  }
}

/** What the trade window asks the server to do. */
export interface TradeActions {
  addItem: (slot: number, amount: number) => void;
  addZeny: (zeny: number) => void;
  lock: () => void;
  /** Completes the trade, and marks that the player pressed Trade. */
  confirm: () => void;
  cancel: () => void;
  /** Shows a short message, such as for an equipped item. */
  message: (text: string) => void;
}

/** What the window reads: the trade, an inventory slot, and the zeny the player has. */
export interface TradeSource {
  state: () => TradeState;
  inventory: (slot: number) => Stack | undefined;
  zeny: () => number;
}

interface Pane {
  root: HTMLDivElement;
  title: HTMLHeadingElement;
  locked: HTMLSpanElement;
  list: HTMLUListElement;
  zeny: HTMLOutputElement;
}

/**
 * The window on the page, section#trade-window. It holds h2 with the
 * partner's name, div.trade-pane.mine and div.trade-pane.theirs with
 * ul.trade-items and output.trade-zeny, form.trade-zeny-form,
 * form.trade-amount for amounts, and the Lock, Trade and Cancel buttons.
 */
export class TradeWindow {
  private readonly title = document.createElement('h2');
  private readonly mine: Pane;
  private readonly theirs: Pane;
  private readonly zenyForm = document.createElement('form');
  private readonly zenyInput = document.createElement('input');
  private readonly form = document.createElement('form');
  private readonly prompt = document.createElement('span');
  private readonly amountInput = document.createElement('input');
  private readonly lockButton = document.createElement('button');
  private readonly confirmButton = document.createElement('button');
  /** The inventory slot the amount form asks about. */
  private adding: number | undefined;

  constructor(
    private readonly root: HTMLElement,
    private readonly source: TradeSource,
    private readonly actions: TradeActions,
    private readonly texts: ItemTexts,
    private readonly icons: ItemIcons,
  ) {
    const head = document.createElement('div');
    head.className = 'trade-head';
    head.append(this.title);

    this.mine = TradeWindow.pane('mine');
    this.theirs = TradeWindow.pane('theirs');
    const panes = document.createElement('div');
    panes.className = 'trade-panes';

    this.zenyForm.className = 'trade-zeny-form';
    const zenyLabel = document.createElement('label');
    zenyLabel.textContent = 'Zeny ';
    this.zenyInput.type = 'number';
    this.zenyInput.min = '1';
    this.zenyInput.required = true;
    zenyLabel.append(this.zenyInput);
    const zenyOk = document.createElement('button');
    zenyOk.type = 'submit';
    zenyOk.textContent = 'Offer';
    this.zenyForm.append(zenyLabel, zenyOk);
    this.zenyForm.addEventListener('submit', (e) => {
      e.preventDefault();
      const have = this.source.zeny();
      const n = parseDropAmount(this.zenyInput.value, have);
      if (n === undefined) {
        this.zenyInput.setCustomValidity(
          have > 0 ? `Enter 1 to ${zenyText(have)}.` : 'You have no zeny.',
        );
        this.zenyInput.reportValidity();
        return;
      }
      this.actions.addZeny(n);
      this.zenyInput.value = '';
    });
    this.zenyInput.addEventListener('input', () => {
      this.zenyInput.setCustomValidity('');
    });
    this.mine.zeny.after(this.zenyForm);
    panes.append(this.mine.root, this.theirs.root);

    this.form.className = 'trade-amount';
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
      const slot = this.adding;
      const left = slot === undefined ? 0 : this.available(slot);
      if (slot === undefined || left <= 0) {
        this.closeAmount();
        return;
      }
      const n = parseDropAmount(this.amountInput.value, left);
      if (n === undefined) {
        this.amountInput.setCustomValidity(`Enter 1 to ${String(left)}.`);
        this.amountInput.reportValidity();
        return;
      }
      this.actions.addItem(slot, n);
      this.closeAmount();
    });
    this.amountInput.addEventListener('input', () => {
      this.amountInput.setCustomValidity('');
    });
    for (const f of [this.form, this.zenyForm]) {
      f.addEventListener('keydown', (e) => {
        // Keys typed here are for the form, not game shortcuts.
        e.stopPropagation();
        if (e.key === 'Escape') {
          this.closeAmount();
        }
      });
    }

    const buttons = document.createElement('div');
    buttons.className = 'trade-buttons';
    this.lockButton.type = 'button';
    this.lockButton.className = 'trade-lock';
    this.lockButton.textContent = 'OK';
    this.lockButton.title = 'Lock your offer';
    this.lockButton.addEventListener('click', () => {
      this.closeAmount();
      this.actions.lock();
    });
    this.confirmButton.type = 'button';
    this.confirmButton.className = 'trade-confirm';
    this.confirmButton.textContent = 'Trade';
    this.confirmButton.title = 'Complete the trade once both offers are locked';
    this.confirmButton.addEventListener('click', () => {
      if (canConfirm(this.source.state())) {
        this.actions.confirm();
      }
    });
    const cancelTrade = document.createElement('button');
    cancelTrade.type = 'button';
    cancelTrade.className = 'trade-cancel';
    cancelTrade.textContent = 'Cancel';
    cancelTrade.addEventListener('click', () => {
      this.actions.cancel();
    });
    buttons.append(this.lockButton, this.confirmButton, cancelTrade);

    const hint = document.createElement('p');
    hint.className = 'trade-hint';
    hint.textContent = 'Drag items from the inventory here, or right-click them.';

    // A row from the inventory window dropped here offers the item.
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
        this.requestAdd(slot);
      }
    });
    this.root.replaceChildren(head, panes, this.form, hint, buttons);
    texts.onLoad(() => {
      this.render();
    });
  }

  get open(): boolean {
    return !this.root.hidden;
  }

  /**
   * Starts offering an inventory slot. A slot with one item left to offer
   * goes at once, and a larger stack asks how many. A worn item says it
   * cannot be traded.
   */
  requestAdd(slot: number): void {
    const state = this.source.state();
    const s = this.source.inventory(slot);
    if (s === undefined || !canOffer(state)) {
      return;
    }
    if (s.equip !== 0) {
      this.actions.message(tradeMessage(TradeOutcome.EQUIPPED));
      return;
    }
    const left = this.available(slot);
    if (left <= 0) {
      return;
    }
    if (left === 1) {
      this.actions.addItem(slot, 1);
      return;
    }
    this.adding = slot;
    this.prompt.textContent = `How many ${this.texts.label(s.itemId)} to offer? (1 to ${String(left)}) `;
    this.amountInput.max = String(left);
    this.amountInput.value = String(left);
    this.form.hidden = false;
    this.amountInput.focus();
    this.amountInput.select();
  }

  /** Shows the trade, or hides the window when none is open. */
  render(): void {
    const s = this.source.state();
    this.root.hidden = !s.open;
    if (!s.open) {
      this.closeAmount();
      return;
    }
    this.title.textContent = `Trade with ${s.partner}`;
    this.title.title = `${s.partner}, level ${String(s.baseLevel)}`;
    this.mine.title.textContent = 'You';
    this.theirs.title.textContent = s.partner;
    this.fill(this.mine, s.mine);
    this.fill(this.theirs, s.theirs);
    this.zenyForm.hidden = !canOfferZeny(s);
    this.lockButton.disabled = s.mine.locked;
    this.confirmButton.disabled = !canConfirm(s);
    this.confirmButton.textContent = s.confirmed ? 'Waiting' : 'Trade';
    if (!canOffer(s) || (this.adding !== undefined && this.available(this.adding) <= 0)) {
      this.closeAmount();
    }
  }

  /** How much of an inventory slot is left to offer. */
  private available(slot: number): number {
    const s = this.source.inventory(slot);
    return s === undefined ? 0 : s.amount - offeredAmount(this.source.state(), slot);
  }

  private static pane(kind: 'mine' | 'theirs'): Pane {
    const div = document.createElement('div');
    div.className = `trade-pane ${kind}`;
    const head = document.createElement('div');
    head.className = 'trade-pane-head';
    const title = document.createElement('h3');
    const locked = document.createElement('span');
    locked.className = 'trade-locked';
    locked.textContent = 'Locked';
    head.append(title, locked);
    const list = document.createElement('ul');
    list.className = 'trade-items';
    list.setAttribute('aria-label', kind === 'mine' ? 'Your offer' : "The partner's offer");
    const zeny = document.createElement('output');
    zeny.className = 'trade-zeny';
    div.append(head, list, zeny);
    return { root: div, title, locked, list, zeny };
  }

  private fill(pane: Pane, side: TradeSide): void {
    pane.locked.hidden = !side.locked;
    pane.root.classList.toggle('locked', side.locked);
    const label = (id: number): string => this.texts.label(id);
    const rows = side.items.map((s) => {
      const li = document.createElement('li');
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
      li.title = itemTooltip(name.textContent, s);
      li.append(icon, name, amount);
      return li;
    });
    for (let i = side.items.length; i < TRADE_ITEMS; i++) {
      const li = document.createElement('li');
      li.className = 'empty';
      rows.push(li);
    }
    pane.list.replaceChildren(...rows);
    pane.zeny.textContent = `Zeny ${zenyText(side.zeny)}`;
  }

  private closeAmount(): void {
    this.adding = undefined;
    this.form.hidden = true;
  }
}
