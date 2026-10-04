package botclient

import (
	"context"
	"maps"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Trade (phase 5 step 6): what a bot knows of its trade window from the
// server's events, and the commands it sends.

// Trade is the bot's open trade window.
type Trade struct {
	// Partner is the other player's name.
	Partner string
	// Mine are the bot's offered amounts by inventory slot. Theirs are
	// the partner's offered items by the order of its offers, with the
	// amounts added up.
	Mine   map[int]int
	Theirs map[int]Item
	// MyZeny and TheirZeny are the zeny offers.
	MyZeny, TheirZeny int64
	// Locked and PartnerLocked are set once each side locked its offer.
	Locked, PartnerLocked bool
}

// tradeState is what a bot knows of its trade. The caller of apply holds
// the bot's mu.
type tradeState struct {
	// request is the last trade request the bot has not answered.
	request *wathenav1.TradeRequest
	trade   *Trade
	// closed is the outcome of the last TradeClose.
	closed wathenav1.TradeOutcome
}

func (s *tradeState) apply(e *wathenav1.Event) {
	switch {
	case e.HasTradeRequest():
		s.request = e.GetTradeRequest()
	case e.HasTradeOpen():
		s.request = nil
		s.trade = &Trade{Partner: e.GetTradeOpen().GetPartner(), Mine: map[int]int{}, Theirs: map[int]Item{}}
	case e.HasTradeOffer():
		o := e.GetTradeOffer()
		if s.trade == nil {
			return
		}
		switch {
		case o.HasZeny() && o.GetMine():
			s.trade.MyZeny = int64(o.GetZeny())
		case o.HasZeny():
			s.trade.TheirZeny = int64(o.GetZeny())
		case o.GetMine():
			s.trade.Mine[int(o.GetItem().GetSlot())] += int(o.GetItem().GetAmount())
		default:
			it := itemOf(o.GetItem())
			old := s.trade.Theirs[it.Slot]
			it.Amount += old.Amount
			s.trade.Theirs[it.Slot] = it
		}
	case e.HasTradeLock():
		if s.trade == nil {
			return
		}
		if e.GetTradeLock().GetMine() {
			s.trade.Locked = true
		} else {
			s.trade.PartnerLocked = true
		}
	case e.HasTradeClose():
		// A request that was taken back closes too.
		s.request, s.trade, s.closed = nil, nil, e.GetTradeClose().GetOutcome()
	}
}

func (s *tradeState) clone() (*wathenav1.TradeRequest, *Trade) {
	if s.trade == nil {
		return s.request, nil
	}
	c := *s.trade
	c.Mine = maps.Clone(s.trade.Mine)
	c.Theirs = maps.Clone(s.trade.Theirs)
	return s.request, &c
}

func (b *Bot) trade(ctx context.Context, c wathenav1.TradeCommand_builder) error {
	return b.command(ctx, wathenav1.Command_builder{Trade: c.Build()})
}

// TradeRequest asks the player with an entity ID, as the bot sees it, to
// trade.
func (b *Bot) TradeRequest(ctx context.Context, entity uint32) error {
	return b.trade(ctx, wathenav1.TradeCommand_builder{Request: &entity})
}

// TradeRespond answers the last trade request. The bot forgets it.
func (b *Bot) TradeRespond(ctx context.Context, accept bool) error {
	b.mu.Lock()
	b.tradeSt.request = nil
	b.mu.Unlock()
	return b.trade(ctx, wathenav1.TradeCommand_builder{Respond: &accept})
}

// TradeAddItem offers an amount of an inventory slot.
func (b *Bot) TradeAddItem(ctx context.Context, slot, amount int) error {
	return b.trade(ctx, wathenav1.TradeCommand_builder{AddItem: storageMove(slot, amount)})
}

// TradeAddZeny offers zeny. A trade takes one zeny offer.
func (b *Bot) TradeAddZeny(ctx context.Context, zeny uint32) error {
	return b.trade(ctx, wathenav1.TradeCommand_builder{AddZeny: &zeny})
}

// TradeLock ends the bot's offer.
func (b *Bot) TradeLock(ctx context.Context) error {
	return b.trade(ctx, wathenav1.TradeCommand_builder{Lock: new(true)})
}

// TradeConfirm completes the trade once both sides locked.
func (b *Bot) TradeConfirm(ctx context.Context) error {
	return b.trade(ctx, wathenav1.TradeCommand_builder{Confirm: new(true)})
}

// TradeCancel ends the trade, or takes back a request.
func (b *Bot) TradeCancel(ctx context.Context) error {
	return b.trade(ctx, wathenav1.TradeCommand_builder{Cancel: new(true)})
}
