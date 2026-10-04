package e2e

import (
	"context"
	"fmt"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Trade steps (phase 5 step 6).

// RequestTrade asks another player in sight to trade, and returns the
// TradeResult: TRADE_OUTCOME_REQUEST_SENT, or why it was refused.
func (p *Player) RequestTrade(other *Player) *wathenav1.TradeResult {
	p.t.Helper()
	id := p.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, other.Name).ID
	p.send("ask to trade", func(ctx context.Context) error { return p.TradeRequest(ctx, id) })
	return p.Expect("the trade request's result", func(e *wathenav1.Event) bool {
		r := e.GetTradeResult()
		return r != nil && (r.GetName() == other.Name || r.GetName() == "")
	}).GetTradeResult()
}

// TradeRequested waits for a trade request from a player.
func (p *Player) TradeRequested(from string) *wathenav1.TradeRequest {
	p.t.Helper()
	return p.Expect("a trade request from "+from, func(e *wathenav1.Event) bool {
		return e.GetTradeRequest().GetFrom() == from
	}).GetTradeRequest()
}

// AcceptTrade answers the trade request and waits for the trade window.
func (p *Player) AcceptTrade() *wathenav1.TradeOpen {
	p.t.Helper()
	p.send("accept the trade", func(ctx context.Context) error { return p.TradeRespond(ctx, true) })
	return p.TradeOpened()
}

// TradeOpened waits for the trade window.
func (p *Player) TradeOpened() *wathenav1.TradeOpen {
	p.t.Helper()
	return p.Expect("the trade window", func(e *wathenav1.Event) bool {
		if r := e.GetTradeResult(); r != nil && r.GetOutcome() != wathenav1.TradeOutcome_TRADE_OUTCOME_REQUEST_SENT {
			p.t.Fatalf("%s: the trade did not open: %v", p.Name, r)
		}
		return e.HasTradeOpen()
	}).GetTradeOpen()
}

// OfferSlot offers an amount of an inventory slot, and returns the
// outcome: TRADE_OUTCOME_UNSPECIFIED when the offer shows.
func (p *Player) OfferSlot(slot, amount int) wathenav1.TradeOutcome {
	p.t.Helper()
	p.send("offer an item", func(ctx context.Context) error { return p.TradeAddItem(ctx, slot, amount) })
	e := p.Expect(fmt.Sprintf("the offer of slot %d", slot), func(e *wathenav1.Event) bool {
		o := e.GetTradeOffer()
		return e.HasTradeResult() || o != nil && o.GetMine() && o.HasItem() && int(o.GetItem().GetSlot()) == slot
	})
	return e.GetTradeResult().GetOutcome()
}

// OfferItem offers an amount of the first slot that holds an item.
func (p *Player) OfferItem(id uint32, amount int) wathenav1.TradeOutcome {
	p.t.Helper()
	slot := p.Game().SlotOf(id)
	if slot < 0 {
		p.t.Fatalf("%s has no item %d to offer", p.Name, id)
	}
	return p.OfferSlot(slot, amount)
}

// OfferZeny offers zeny and waits for the offer.
func (p *Player) OfferZeny(zeny uint32) {
	p.t.Helper()
	p.send("offer zeny", func(ctx context.Context) error { return p.TradeAddZeny(ctx, zeny) })
	p.Expect("the zeny offer", func(e *wathenav1.Event) bool {
		if r := e.GetTradeResult(); r != nil {
			p.t.Fatalf("%s: offering %d zeny: %v", p.Name, zeny, r)
		}
		o := e.GetTradeOffer()
		return o != nil && o.GetMine() && o.HasZeny() && o.GetZeny() == zeny
	})
}

// LockTrade locks the player's offer.
func (p *Player) LockTrade() {
	p.t.Helper()
	p.send("lock the trade", func(ctx context.Context) error { return p.TradeLock(ctx) })
	p.Expect("the lock", func(e *wathenav1.Event) bool { return e.GetTradeLock().GetMine() && e.HasTradeLock() })
}

// ConfirmTrade confirms the trade. It does not wait.
func (p *Player) ConfirmTrade() {
	p.t.Helper()
	p.send("confirm the trade", func(ctx context.Context) error { return p.TradeConfirm(ctx) })
}

// TradeClosed waits for the trade window to close, and returns why.
func (p *Player) TradeClosed() wathenav1.TradeOutcome {
	p.t.Helper()
	return p.Expect("the trade to close", (*wathenav1.Event).HasTradeClose).GetTradeClose().GetOutcome()
}

// Offer is what one side of Trade offers: amounts by item ID, and zeny.
type Offer struct {
	Items map[uint32]int
	Zeny  uint32
}

// Trade runs a whole trade between two players in sight of each other:
// a asks, b accepts, both offer, lock and confirm. It returns how the
// trade window closed for both.
func Trade(a, b *Player, offerA, offerB Offer) (wathenav1.TradeOutcome, wathenav1.TradeOutcome) {
	a.t.Helper()
	if r := a.RequestTrade(b); r.GetOutcome() != wathenav1.TradeOutcome_TRADE_OUTCOME_REQUEST_SENT {
		a.t.Fatalf("%s asks %s to trade: %v", a.Name, b.Name, r)
	}
	b.TradeRequested(a.Name)
	b.AcceptTrade()
	a.TradeOpened()
	for _, side := range []struct {
		p *Player
		o Offer
	}{{a, offerA}, {b, offerB}} {
		for id, n := range side.o.Items {
			if o := side.p.OfferItem(id, n); o != wathenav1.TradeOutcome_TRADE_OUTCOME_UNSPECIFIED {
				side.p.t.Fatalf("%s offers %d of item %d: %s", side.p.Name, n, id, o)
			}
		}
		if side.o.Zeny > 0 {
			side.p.OfferZeny(side.o.Zeny)
		}
	}
	a.LockTrade()
	b.LockTrade()
	a.ConfirmTrade()
	a.Expect("waiting for the partner", func(e *wathenav1.Event) bool {
		return e.GetTradeResult().GetOutcome() == wathenav1.TradeOutcome_TRADE_OUTCOME_WAITING
	})
	b.ConfirmTrade()
	return a.TradeClosed(), b.TradeClosed()
}
