import { describe, expect, it } from 'vitest';
import { ItemBound, TradeOutcome } from '../gen/wathena/v1/event_pb';
import {
  NO_TRADE,
  TRADE_ITEMS,
  type TradeEvent,
  type TradeState,
  canConfirm,
  canOffer,
  canOfferZeny,
  offeredAmount,
  tradeMessage,
  tradeReducer,
  tradeRequestText,
  zenyText,
} from './trade';

function run(...events: TradeEvent[]): TradeState {
  return events.reduce(tradeReducer, NO_TRADE);
}

const open: TradeEvent = { type: 'open', partner: 'Bob', baseLevel: 50 };

function item(slot: number, itemId: number, amount: number): TradeEvent {
  return { type: 'offer', mine: true, offer: { case: 'item', value: { slot, itemId, amount } } };
}

describe('tradeReducer', () => {
  it('opens with empty offers', () => {
    const s = run(open);
    expect(s).toMatchObject({ open: true, partner: 'Bob', baseLevel: 50, confirmed: false });
    expect(s.mine).toEqual({ items: [], zeny: 0, locked: false });
    expect(canOffer(s)).toBe(true);
    expect(canOfferZeny(s)).toBe(true);
    expect(canConfirm(s)).toBe(false);
  });

  it('ignores offers while no trade is open', () => {
    expect(run(item(0, 909, 1))).toBe(NO_TRADE);
    expect(run({ type: 'lock', mine: true })).toBe(NO_TRADE);
  });

  it('adds offers to the side that made them', () => {
    const s = run(
      open,
      item(3, 909, 5),
      item(3, 909, 2),
      {
        type: 'offer',
        mine: false,
        offer: {
          case: 'item',
          value: { slot: 0, itemId: 1201, amount: 1, refine: 7, bound: ItemBound.UNSPECIFIED },
        },
      },
      { type: 'offer', mine: false, offer: { case: 'zeny', value: 5000 } },
      { type: 'offer', mine: true, offer: { case: 'zeny', value: 100 } },
    );
    expect(s.mine.items.map((i) => [i.slot, i.amount])).toEqual([
      [3, 5],
      [3, 2],
    ]);
    expect(offeredAmount(s, 3)).toBe(7);
    expect(offeredAmount(s, 4)).toBe(0);
    expect(s.theirs.items[0]).toMatchObject({ itemId: 1201, refine: 7 });
    expect(s.theirs.zeny).toBe(5000);
    expect(s.mine.zeny).toBe(100);
    // Zeny is offered once.
    expect(canOfferZeny(s)).toBe(false);
  });

  it('stops offers at the item limit and after the lock', () => {
    const full = run(open, ...Array.from({ length: TRADE_ITEMS }, (_, i) => item(i, 909, 1)));
    expect(canOffer(full)).toBe(false);
    const locked = run(open, { type: 'lock', mine: true });
    expect(canOffer(locked)).toBe(false);
    expect(canOfferZeny(locked)).toBe(false);
  });

  it('enables Trade when both offers are locked', () => {
    const mine = run(open, { type: 'lock', mine: true });
    expect(canConfirm(mine)).toBe(false);
    // Trade before both locks does nothing.
    expect(tradeReducer(mine, { type: 'confirm' }).confirmed).toBe(false);
    const both = tradeReducer(mine, { type: 'lock', mine: false });
    expect(both.theirs.locked).toBe(true);
    expect(canConfirm(both)).toBe(true);
    const pressed = tradeReducer(both, { type: 'confirm' });
    expect(pressed.confirmed).toBe(true);
    expect(canConfirm(pressed)).toBe(false);
    // WAITING says the server took the press.
    expect(tradeReducer(both, { type: 'result', outcome: TradeOutcome.WAITING }).confirmed).toBe(
      true,
    );
    expect(tradeReducer(both, { type: 'result', outcome: TradeOutcome.LOCKED })).toBe(both);
  });

  it('forgets everything when it closes, and a new trade starts empty', () => {
    const s = run(open, item(0, 909, 1), { type: 'lock', mine: true }, { type: 'close' });
    expect(s).toBe(NO_TRADE);
    expect(run(open, item(0, 909, 1), open).mine.items).toEqual([]);
  });

  it('does not change the state passed in', () => {
    const before = run(open);
    tradeReducer(before, item(0, 909, 1));
    tradeReducer(before, { type: 'lock', mine: true });
    expect(before.mine).toEqual({ items: [], zeny: 0, locked: false });
  });
});

describe('trade texts', () => {
  it('asks to trade', () => {
    expect(tradeRequestText('Bob', 50)).toBe('Bob (level 50) wants to trade with you.');
  });

  it('writes zeny with separators', () => {
    expect(zenyText(1200000)).toBe('1,200,000');
    expect(zenyText(0)).toBe('0');
  });

  it('says how a trade step went', () => {
    expect(tradeMessage(TradeOutcome.TOO_FAR, 'Bob')).toBe('Bob is too far away to trade.');
    expect(tradeMessage(TradeOutcome.TOO_FAR)).toBe('That player is too far away to trade.');
    expect(tradeMessage(TradeOutcome.REFUSED, 'Bob')).toBe('Bob refused to trade.');
    expect(tradeMessage(TradeOutcome.REQUEST_SENT, 'Bob')).toBe('You asked Bob to trade.');
    expect(tradeMessage(TradeOutcome.COMPLETED)).toBe('The trade is complete.');
    expect(tradeMessage(TradeOutcome.RESTRICTED)).toBe('This item cannot be traded.');
    expect(tradeMessage(TradeOutcome.BOUND)).toBe(
      'This bounded item cannot be traded to that character.',
    );
    expect(tradeMessage(TradeOutcome.HOOK, '', 'No trades in this town.')).toBe(
      'No trades in this town.',
    );
    expect(tradeMessage(TradeOutcome.HOOK)).toBe('The trade is not allowed.');
    expect(tradeMessage(TradeOutcome.WAITING, 'Bob')).toBe('Waiting for Bob to press Trade.');
    for (const value of Object.values(TradeOutcome)) {
      if (typeof value === 'number') {
        expect(tradeMessage(value, 'Bob')).toMatch(/^[A-Z].*\.$/);
        expect(tradeMessage(value)).toMatch(/^[A-Z].*\.$/);
      }
    }
  });
});
