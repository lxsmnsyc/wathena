package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// Trade (phase 5 step 6, decision 0023), as rAthena's trade.cpp:
//   - A player asks another on the map, within 2 cells, to trade. A
//     player who blocks the other refuses at once, as invites do
//     (decision 0018); rAthena has no such check.
//   - Each side offers up to 10 items and an amount of zeny, then locks
//     its offer. Once both locked, each confirms. The trade completes
//     when both confirmed.
//   - The offers are checked as they come, as trade_tradeadditem, and
//     again at the end on copies of both inventories, as trade_check.
//     Then the trade hook may refuse it.
//   - Every item and zeny moves in one transfer, so the trade happens
//     whole or not at all, and joins one commit with both characters.
//   - A map change, a logout or a new request ends the trade, as
//     unit_remove_map does. Moving does not.

// tradeDistance is TRADE_DISTANCE.
const tradeDistance = 2

// maxTradeItems is the 10 deal slots of trade_tradeadditem.
const maxTradeItems = 10

// skillBasic is NV_BASIC.
const skillBasic = 1

// trade is one side of a trade.
type trade struct {
	partner *Player
	// items are the offered slots, in the order they were offered.
	items []tradeItem
	zeny  int64
	// zenySet is set once zeny was offered: the client offers it once.
	zenySet bool
	// locked is deal_locked 1, confirmed deal_locked 2.
	locked, confirmed bool
}

type tradeItem struct {
	slot   int
	serial item.Serial
	amount int
}

// TradeAction is one trade command, for the gateway.
type TradeAction struct {
	// Request is the entity ID of the player to ask.
	Request uint32
	// Respond answers a request, with Accept.
	Respond, Accept bool
	// AddSlot and AddAmount offer an amount of an inventory slot, when
	// AddAmount is above 0.
	AddSlot, AddAmount int
	// Zeny offers zeny, when HasZeny is set.
	Zeny    int64
	HasZeny bool
	Lock    bool
	Confirm bool
	Cancel  bool
}

// Trade runs a trade command.
func (s *Session) Trade(seq uint32, t TradeAction) bool {
	return s.send(command{seq: seq, trade: &t})
}

func tradeResult(o wathenav1.TradeOutcome, name string) *wathenav1.Event {
	return wathenav1.Event_builder{TradeResult: wathenav1.TradeResult_builder{Outcome: o, Name: name}.Build()}.Build()
}

func tradeClose(o wathenav1.TradeOutcome) *wathenav1.Event {
	return wathenav1.Event_builder{TradeClose: wathenav1.TradeClose_builder{Outcome: o}.Build()}.Build()
}

// tradeCommand runs a trade command.
func (z *Zone) tradeCommand(p *Player, c *TradeAction) {
	switch {
	case c.Request != 0:
		z.tradeRequest(p, c.Request)
	case c.Respond:
		z.tradeAck(p, c.Accept)
	case c.AddAmount > 0:
		z.tradeAddItem(p, c.AddSlot, c.AddAmount)
	case c.HasZeny:
		z.tradeAddZeny(p, c.Zeny)
	case c.Lock:
		z.tradeLock(p)
	case c.Confirm:
		z.tradeConfirm(p)
	case c.Cancel:
		z.tradeCancel(p, wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED)
	}
}

// busyForTrade is the part of pc_istrading and trade_tradeack's checks
// that wAthena has: a conversation, a shop window or an open storage.
func busyForTrade(p *Player) bool {
	return p.talk != nil || p.shop != nil || p.storage != nil || p.guild.storage != nil
}

// inTradeRange is check_distance_bl with TRADE_DISTANCE.
func inTradeRange(a, b *Player) bool {
	return a.entity != nil && b.entity != nil && abs(a.entity.x-b.entity.x) <= tradeDistance && abs(a.entity.y-b.entity.y) <= tradeDistance
}

// tradeRequest is trade_traderequest.
func (z *Zone) tradeRequest(p *Player, netID uint32) {
	if p.entity == nil || p.entity.dead {
		return
	}
	if rules.ItemsOrDefault(z.worldRules()).BasicSkillCheck && p.skillLevel(skillBasic) < 1 {
		// clif_parse_TradeRequest: Basic Skill level 1.
		p.pending = append(p.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_FAILED, ""))
		return
	}
	var t *Player
	for e, id := range p.netIDs.all() {
		if id == netID && e.player != nil && z.players[e.player.charID] == e.player {
			t = e.player
			break
		}
	}
	if t == nil || t == p || t.gone() {
		p.pending = append(p.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_NOT_FOUND, ""))
		return
	}
	// A new request ends the requester's trade, as trade_traderequest.
	if p.trade != nil {
		z.tradeCancel(p, wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED)
	}
	if t.blocking(p.Name) {
		p.pending = append(p.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_REFUSED, t.Name))
		return
	}
	if t.trade != nil || t.tradeAsk != nil && t.tradeAsk != p || busyForTrade(t) {
		p.pending = append(p.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_BUSY, t.Name))
		return
	}
	if !inTradeRange(p, t) {
		p.pending = append(p.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_TOO_FAR, t.Name))
		return
	}
	t.tradeAsk = p
	t.pending = append(t.pending, wathenav1.Event_builder{TradeRequest: wathenav1.TradeRequest_builder{
		From: p.Name, BaseLevel: uint32(max(p.progress.BaseLevel, 0)), // #nosec G115 -- a level.
	}.Build()}.Build())
	p.pending = append(p.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_REQUEST_SENT, t.Name))
}

// tradeAck is trade_tradeack: the asked player answers.
func (z *Zone) tradeAck(p *Player, accept bool) {
	a := p.tradeAsk
	p.tradeAsk = nil
	if a == nil || p.trade != nil {
		return
	}
	if z.players[a.charID] != a || a.gone() {
		p.pending = append(p.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_NOT_FOUND, a.Name))
		return
	}
	if !accept {
		a.pending = append(a.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_REFUSED, p.Name))
		return
	}
	switch {
	case a.trade != nil:
		p.pending = append(p.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_BUSY, a.Name))
		return
	case !inTradeRange(p, a):
		p.pending = append(p.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_TOO_FAR, a.Name))
		a.pending = append(a.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_TOO_FAR, p.Name))
		return
	case busyForTrade(p) || busyForTrade(a):
		p.pending = append(p.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_BUSY, a.Name))
		a.pending = append(a.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_BUSY, p.Name))
		return
	}
	p.trade, a.trade = &trade{partner: a}, &trade{partner: p}
	for _, x := range []*Player{p, a} {
		o := x.trade.partner
		x.pending = append(x.pending, wathenav1.Event_builder{TradeOpen: wathenav1.TradeOpen_builder{
			Partner: o.Name, BaseLevel: uint32(max(o.progress.BaseLevel, 0)), // #nosec G115 -- a level.
		}.Build()}.Build())
	}
}

// tradeWeight is the weight of an offer's items.
func (z *Zone) tradeWeight(p *Player) int {
	w := 0
	for _, it := range p.trade.items {
		w += z.Items.Get(p.inventory.Slots[it.slot].ID).Weight * it.amount
	}
	return w
}

// sameGuild reports whether two players are in one guild, for
// guild-bound items.
func sameGuild(a, b *Player) bool {
	return a.guild.view != nil && b.guild.view != nil && a.guild.view.ID == b.guild.view.ID
}

// tradeAddItem is trade_tradeadditem.
func (z *Zone) tradeAddItem(p *Player, slot, amount int) {
	tr := p.trade
	if tr == nil || tr.locked {
		if tr != nil {
			p.pending = append(p.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_LOCKED, ""))
		}
		return
	}
	t := tr.partner
	refuse := func(o wathenav1.TradeOutcome) {
		p.pending = append(p.pending, wathenav1.Event_builder{TradeResult: wathenav1.TradeResult_builder{
			Outcome: o, Slot: uint32(slot), // #nosec G115 -- below MaxInventory.
		}.Build()}.Build())
	}
	if slot < 0 || slot >= len(p.inventory.Slots) {
		return
	}
	s := &p.inventory.Slots[slot]
	offered := 0
	at := -1
	for i, it := range tr.items {
		if it.slot == slot {
			offered, at = it.amount, i
		}
	}
	if s.ID == 0 || amount < 1 || offered+amount > s.Amount {
		return
	}
	d := z.Items.Get(s.ID)
	switch {
	case s.Equip != 0:
		// impossible_trade_check drops equipped items; the web client
		// is told at once.
		refuse(wathenav1.TradeOutcome_TRADE_OUTCOME_EQUIPPED)
		return
	case z.Items.Restricted(s, item.NoTrade):
		// itemdb_cantrade: msg 260.
		refuse(wathenav1.TradeOutcome_TRADE_OUTCOME_RESTRICTED)
		return
	case !item.BoundAllows(s.Bound, item.PlaceTrade, sameGuild(p, t)):
		// msg 293.
		refuse(wathenav1.TradeOutcome_TRADE_OUTCOME_BOUND)
		return
	case at < 0 && len(tr.items) >= maxTradeItems:
		refuse(wathenav1.TradeOutcome_TRADE_OUTCOME_TOO_MANY)
		return
	}
	// trade_count_stackable is on: each new deal slot needs a free slot.
	if at < 0 && t.inventory.Blank() < len(tr.items)+1 {
		refuse(wathenav1.TradeOutcome_TRADE_OUTCOME_TOO_MANY)
		return
	}
	if t.inventory.Weight+z.tradeWeight(p)+d.Weight*amount > t.maxWeight {
		refuse(wathenav1.TradeOutcome_TRADE_OUTCOME_OVERWEIGHT)
		return
	}
	if at >= 0 {
		tr.items[at].amount += amount
	} else {
		tr.items = append(tr.items, tradeItem{slot: slot, serial: s.Serial, amount: amount})
		at = len(tr.items) - 1
	}
	mine := z.itemMessage(slot, s, amount)
	theirs := z.itemMessage(at, s, amount)
	p.pending = append(p.pending, wathenav1.Event_builder{TradeOffer: wathenav1.TradeOffer_builder{Mine: true, Item: mine}.Build()}.Build())
	t.pending = append(t.pending, wathenav1.Event_builder{TradeOffer: wathenav1.TradeOffer_builder{Item: theirs}.Build()}.Build())
}

// tradeAddZeny is trade_tradeaddzeny. An amount the player lacks, or one
// that would take the partner past the zeny cap, ends the trade.
func (z *Zone) tradeAddZeny(p *Player, amount int64) {
	tr := p.trade
	if tr == nil || tr.locked || tr.zenySet {
		return
	}
	t := tr.partner
	if amount < 0 || amount > p.record.Zeny || amount > transfer.MaxZeny-t.record.Zeny {
		p.pending = append(p.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_ZENY, ""))
		z.tradeCancel(p, wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED)
		return
	}
	tr.zeny, tr.zenySet = amount, true
	p.pending = append(p.pending, wathenav1.Event_builder{TradeOffer: wathenav1.TradeOffer_builder{Mine: true,
		Zeny: new(uint32(amount))}.Build()}.Build()) // #nosec G115 -- at most MAX_ZENY.
	t.pending = append(t.pending, wathenav1.Event_builder{TradeOffer: wathenav1.TradeOffer_builder{
		Zeny: new(uint32(amount))}.Build()}.Build()) // #nosec G115 -- at most MAX_ZENY.
}

// tradeLock is trade_tradeok.
func (z *Zone) tradeLock(p *Player) {
	tr := p.trade
	if tr == nil || tr.locked {
		return
	}
	tr.locked = true
	p.pending = append(p.pending, wathenav1.Event_builder{TradeLock: wathenav1.TradeLock_builder{Mine: true}.Build()}.Build())
	tr.partner.pending = append(tr.partner.pending, wathenav1.Event_builder{TradeLock: wathenav1.TradeLock_builder{}.Build()}.Build())
}

// tradeCancel is trade_tradecancel: it ends the player's trade, or its
// request, for both players.
func (z *Zone) tradeCancel(p *Player, o wathenav1.TradeOutcome) {
	if a := p.tradeAsk; a != nil {
		p.tradeAsk = nil
		a.pending = append(a.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED, p.Name))
	}
	for _, x := range z.players {
		if x.tradeAsk == p {
			x.tradeAsk = nil
			x.pending = append(x.pending, tradeClose(wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED))
		}
	}
	tr := p.trade
	if tr == nil {
		return
	}
	t := tr.partner
	p.trade = nil
	p.pending = append(p.pending, tradeClose(o))
	if t != nil && t.trade != nil && t.trade.partner == p {
		t.trade = nil
		t.pending = append(t.pending, tradeClose(o))
	}
}

// tradeConfirm is trade_tradecommit.
func (z *Zone) tradeConfirm(p *Player) {
	tr := p.trade
	if tr == nil || !tr.locked || tr.confirmed {
		return
	}
	t := tr.partner
	tr.confirmed = true
	if t.trade == nil || !t.trade.confirmed {
		p.pending = append(p.pending, tradeResult(wathenav1.TradeOutcome_TRADE_OUTCOME_WAITING, t.Name))
		return
	}
	if ok, msg := z.runTradeHook(p, t); !ok {
		for _, x := range []*Player{p, t} {
			x.pending = append(x.pending, wathenav1.Event_builder{TradeResult: wathenav1.TradeResult_builder{
				Outcome: wathenav1.TradeOutcome_TRADE_OUTCOME_HOOK, Message: msg}.Build()}.Build())
		}
		z.tradeCancel(p, wathenav1.TradeOutcome_TRADE_OUTCOME_FAILED)
		return
	}
	if !z.tradeCommit(p, t) {
		z.tradeCancel(p, wathenav1.TradeOutcome_TRADE_OUTCOME_FAILED)
		return
	}
	p.trade, t.trade = nil, nil
	p.pending = append(p.pending, tradeClose(wathenav1.TradeOutcome_TRADE_OUTCOME_COMPLETED))
	t.pending = append(t.pending, tradeClose(wathenav1.TradeOutcome_TRADE_OUTCOME_COMPLETED))
}

// tradeCommit moves both offers in one transfer, as trade_tradecommit
// after trade_check. Each offered slot must still hold its serial and
// the amount, and be unequipped. It returns false, with nothing moved,
// when a check fails.
func (z *Zone) tradeCommit(p, t *Player) bool {
	pb, tb := z.box(p), z.box(t)
	pw, tw := z.wallet(p), z.wallet(t)
	tx := transfer.New(transfer.ActionTrade)
	if z.Clock != nil {
		tx.Now = z.Clock()
	}
	for _, side := range []struct {
		from     *Player
		fb, toBx *transfer.InventoryBox
	}{{p, pb, tb}, {t, tb, pb}} {
		for _, it := range side.from.trade.items {
			s := &side.from.inventory.Slots[it.slot]
			if s.Serial != it.serial || s.Amount < it.amount || s.Equip != 0 {
				return false
			}
			tx.Move(side.fb, it.slot, it.serial, it.amount, side.toBx)
		}
	}
	if p.trade.zeny > 0 {
		tx.MoveZeny(pw, p.trade.zeny, tw)
	}
	if t.trade.zeny > 0 {
		tx.MoveZeny(tw, t.trade.zeny, pw)
	}
	var items []modscript.TradeItem
	for _, side := range []*Player{p, t} {
		for _, it := range side.trade.items {
			items = append(items, modscript.TradeItem{From: int64(side.charID), Item: side.inventory.Slots[it.slot].ID, Amount: it.amount})
		}
	}
	zeny := [2]int64{p.trade.zeny, t.trade.zeny}
	logs, err := tx.Apply(func(c transfer.Change) {
		who := p
		if c.Box == transfer.Box(tb) || c.Wallet == transfer.Wallet(tw) {
			who = t
		}
		switch {
		case c.Wallet != nil:
			if s := z.battleStats(who); s != nil {
				who.pending = append(who.pending, s)
			}
		case c.Amount > 0:
			z.sendAdd(who, c.Slot, c.Amount)
		default:
			who.pending = append(who.pending, wathenav1.Event_builder{InventoryRemove: wathenav1.InventoryRemove_builder{
				Slot: uint32(c.Slot), Amount: uint32(-c.Amount), // #nosec G115 -- below MaxInventory and a stack.
				Reason: wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_TRADED,
			}.Build()}.Build())
			z.updateWeight(who)
		}
	})
	if err != nil {
		return false
	}
	z.recordTransfer(p, logs)
	if !t.transferred {
		t.transferred = true
		z.txPlayers = append(z.txPlayers, t)
	}
	z.scriptEvents = append(z.scriptEvents, scriptEvent{name: "trade", trade: &modscript.TradeEvent{
		Players: [2]int64{int64(p.charID), int64(t.charID)}, Items: items, Zeny: zeny}})
	return true
}

// runTradeHook runs the trade hooks before a trade completes. It returns
// false and a message when one refuses the trade.
func (z *Zone) runTradeHook(p, t *Player) (bool, string) {
	r := z.scripts
	if r == nil || !r.HasHooks("trade") {
		return true, ""
	}
	var items []modscript.TradeItem
	for _, side := range []*Player{p, t} {
		for _, it := range side.trade.items {
			items = append(items, modscript.TradeItem{From: int64(side.charID), Item: side.inventory.Slots[it.slot].ID, Amount: it.amount})
		}
	}
	return r.RunTrade(modscript.TradeEvent{Players: [2]int64{int64(p.charID), int64(t.charID)}, Items: items,
		Zeny: [2]int64{p.trade.zeny, t.trade.zeny}})
}

// Mail runs a mail command (mail.go).
func (s *Session) Mail(seq uint32, c *wathenav1.MailCommand) bool {
	return s.send(command{seq: seq, mail: c})
}

// Vending runs a vending command (vending.go).
func (s *Session) Vending(seq uint32, c *wathenav1.VendingCommand) bool {
	return s.send(command{seq: seq, vending: c})
}

// BuyingStore runs a buying store command (vending.go).
func (s *Session) BuyingStore(seq uint32, c *wathenav1.BuyingStoreCommand) bool {
	return s.send(command{seq: seq, buying: c})
}

// StoreSearch runs a store search command (vending.go).
func (s *Session) StoreSearch(seq uint32, c *wathenav1.StoreSearchCommand) bool {
	return s.send(command{seq: seq, search: c})
}
