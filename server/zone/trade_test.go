package zone

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

func doTrade(z *Zone, p *Player, t TradeAction) []*wathenav1.Event {
	z.handle(command{player: p, seq: 1, trade: &t})
	z.commitTransfers()
	z.stepAt(z.now + 50)
	return events(p)
}

func netIDOf(p, of *Player) uint32 {
	id, _ := p.netIDs.get(of.entity)
	return id
}

func tradeOutcomes(ev []*wathenav1.Event) []wathenav1.TradeOutcome {
	var out []wathenav1.TradeOutcome
	for _, e := range ev {
		switch {
		case e.HasTradeResult():
			out = append(out, e.GetTradeResult().GetOutcome())
		case e.HasTradeClose():
			out = append(out, e.GetTradeClose().GetOutcome())
		}
	}
	return out
}

// openTrade has a ask b, and b accept.
func openTrade(t *testing.T, z *Zone, a, b *Player) {
	t.Helper()
	ev := doTrade(z, a, TradeAction{Request: netIDOf(a, b)})
	if got := tradeOutcomes(ev); len(got) != 1 || got[0] != wathenav1.TradeOutcome_TRADE_OUTCOME_REQUEST_SENT {
		t.Fatalf("request: %v", got)
	}
	if rq := eventsOf(b, (*wathenav1.Event).GetTradeRequest, (*wathenav1.Event).HasTradeRequest); len(rq) != 1 || rq[0].GetFrom() != a.Name {
		t.Fatalf("b got %v", rq)
	}
	doTrade(z, b, TradeAction{Respond: true, Accept: true})
	if a.trade == nil || b.trade == nil {
		t.Fatal("no trade")
	}
	events(a)
}

func TestTrade(t *testing.T) {
	z, a, b, cm := storageZone(t, stack(0, 909, 10), stack(1, 501, 2))
	b.record.Zeny = 100
	openTrade(t, z, a, b)
	ev := doTrade(z, a, TradeAction{AddSlot: 0, AddAmount: 4})
	if of := eventsOf2(ev, (*wathenav1.Event).GetTradeOffer, (*wathenav1.Event).HasTradeOffer); len(of) != 1 || !of[0].GetMine() ||
		of[0].GetItem().GetAmount() != 4 {
		t.Fatalf("offer %v", of)
	}
	if of := eventsOf(b, (*wathenav1.Event).GetTradeOffer, (*wathenav1.Event).HasTradeOffer); len(of) != 1 || of[0].GetMine() ||
		of[0].GetItem().GetItemId() != 909 {
		t.Fatalf("b sees %v", of)
	}
	doTrade(z, a, TradeAction{AddSlot: 0, AddAmount: 2})
	doTrade(z, a, TradeAction{AddSlot: 1, AddAmount: 2})
	doTrade(z, b, TradeAction{Zeny: 60, HasZeny: true})
	// Confirming before locking does nothing.
	doTrade(z, a, TradeAction{Confirm: true})
	doTrade(z, a, TradeAction{Lock: true})
	// A locked offer takes no more.
	if got := tradeOutcomes(doTrade(z, a, TradeAction{AddSlot: 0, AddAmount: 1})); len(got) != 1 || got[0] != wathenav1.TradeOutcome_TRADE_OUTCOME_LOCKED {
		t.Errorf("after lock: %v", got)
	}
	doTrade(z, b, TradeAction{Lock: true})
	if got := tradeOutcomes(doTrade(z, a, TradeAction{Confirm: true})); len(got) != 1 || got[0] != wathenav1.TradeOutcome_TRADE_OUTCOME_WAITING {
		t.Errorf("first confirm: %v", got)
	}
	before := len(cm.got)
	ev = doTrade(z, b, TradeAction{Confirm: true})
	if got := tradeOutcomes(ev); len(got) != 1 || got[0] != wathenav1.TradeOutcome_TRADE_OUTCOME_COMPLETED {
		t.Fatalf("completed: %v", got)
	}
	if a.inventory.Count(909) != 4 || b.inventory.Count(909) != 6 || b.inventory.Count(501) != 2 || a.inventory.Count(501) != 0 ||
		a.record.Zeny != 560 || b.record.Zeny != 40 {
		t.Errorf("after: a %d %d %d, b %d %d %d", a.inventory.Count(909), a.inventory.Count(501), a.record.Zeny,
			b.inventory.Count(909), b.inventory.Count(501), b.record.Zeny)
	}
	if a.trade != nil || b.trade != nil {
		t.Error("the trade did not end")
	}
	// One commit holds both characters and every line.
	if len(cm.got) != before+1 {
		t.Fatalf("%d commits", len(cm.got)-before)
	}
	c := cm.got[len(cm.got)-1]
	if len(c.saves) != 2 || len(c.logs.Items) != 2 || len(c.logs.Zeny) != 1 || c.logs.Items[0].Action != transfer.ActionTrade {
		t.Errorf("commit %+v", c)
	}
	// The whole Red Potion stack kept its serial.
	if c.logs.Items[1].Source != (item.Serial{}) {
		t.Errorf("the whole stack got a new serial")
	}
}

func TestTradeRefusals(t *testing.T) {
	items := []character.Item{stack(0, 7003, 1), stack(1, 1201, 1), stack(2, 909, 1), stack(3, 909, 1), stack(4, 7005, 30), stack(5, 1201, 1)}
	items[1].Cards = []uint32{7004}
	items[2].Bound = item.BoundAccount
	items[3].Bound = item.BoundGuild
	items[5].Equip = 2
	z, a, b, _ := storageZone(t, items...)
	openTrade(t, z, a, b)
	for _, c := range []struct {
		name string
		slot int
		want wathenav1.TradeOutcome
	}{
		{"no_trade", 0, wathenav1.TradeOutcome_TRADE_OUTCOME_RESTRICTED},
		{"a no_trade card", 1, wathenav1.TradeOutcome_TRADE_OUTCOME_RESTRICTED},
		{"account-bound", 2, wathenav1.TradeOutcome_TRADE_OUTCOME_BOUND},
		{"guild-bound outside the guild", 3, wathenav1.TradeOutcome_TRADE_OUTCOME_BOUND},
		{"too heavy", 4, wathenav1.TradeOutcome_TRADE_OUTCOME_OVERWEIGHT},
		{"equipped", 5, wathenav1.TradeOutcome_TRADE_OUTCOME_EQUIPPED},
	} {
		amount := 1
		if c.slot == 4 {
			amount = 30
			b.maxWeight = 3000 * 29
		}
		if got := tradeOutcomes(doTrade(z, a, TradeAction{AddSlot: c.slot, AddAmount: amount})); len(got) != 1 || got[0] != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	// No item, no drop, no use while trading.
	z.dropItem(a, 2, 1)
	if a.inventory.Count(909) != 2 {
		t.Error("dropped while trading")
	}
	// Zeny the player lacks ends the trade.
	got := tradeOutcomes(doTrade(z, a, TradeAction{Zeny: 999999, HasZeny: true}))
	if len(got) != 2 || got[0] != wathenav1.TradeOutcome_TRADE_OUTCOME_ZENY || a.trade != nil || b.trade != nil {
		t.Errorf("zeny: %v", got)
	}

	// A player who blocks the asker refuses at once.
	b.blocks = []string{a.Name}
	if got := tradeOutcomes(doTrade(z, a, TradeAction{Request: netIDOf(a, b)})); len(got) != 1 || got[0] != wathenav1.TradeOutcome_TRADE_OUTCOME_REFUSED {
		t.Errorf("blocked: %v", got)
	}
	if rq := eventsOf(b, (*wathenav1.Event).GetTradeRequest, (*wathenav1.Event).HasTradeRequest); len(rq) != 0 {
		t.Errorf("a blocking player saw the request")
	}
	b.blocks = nil
	// Too far: more than 2 cells.
	b.entity.x = a.entity.x + 3
	if got := tradeOutcomes(doTrade(z, a, TradeAction{Request: netIDOf(a, b)})); len(got) != 1 || got[0] != wathenav1.TradeOutcome_TRADE_OUTCOME_TOO_FAR {
		t.Errorf("far: %v", got)
	}
	b.entity.x = a.entity.x + 1
	// A declined request.
	doTrade(z, a, TradeAction{Request: netIDOf(a, b)})
	doTrade(z, b, TradeAction{Respond: true, Accept: false})
	z.stepAt(z.now + 50)
	if got := tradeOutcomes(events(a)); len(got) != 1 || got[0] != wathenav1.TradeOutcome_TRADE_OUTCOME_REFUSED {
		t.Errorf("declined: %v", got)
	}
}

func TestTradeBasicSkill(t *testing.T) {
	z, a, b, _ := storageZone(t)
	a.char.Skills = map[int]int{}
	if got := tradeOutcomes(doTrade(z, a, TradeAction{Request: netIDOf(a, b)})); len(got) != 1 || got[0] != wathenav1.TradeOutcome_TRADE_OUTCOME_FAILED {
		t.Errorf("without Basic Skill: %v", got)
	}
	r := rules.Base(true)
	r.Items, _ = rules.ItemsOf(map[string]int{"basic_skill_check": 0})
	z.Mods = &Mods{Rules: r}
	if got := tradeOutcomes(doTrade(z, a, TradeAction{Request: netIDOf(a, b)})); len(got) != 1 || got[0] != wathenav1.TradeOutcome_TRADE_OUTCOME_REQUEST_SENT {
		t.Errorf("with basic_skill_check off: %v", got)
	}
}

// TestTradeBlockListRule checks block_list_refuses_trade: on, a player who
// blocks the asker refuses at once; off, the request goes through, as
// rAthena.
func TestTradeBlockListRule(t *testing.T) {
	z, a, b, _ := storageZone(t)
	b.blocks = []string{a.Name}
	if got := tradeOutcomes(doTrade(z, a, TradeAction{Request: netIDOf(a, b)})); len(got) != 1 || got[0] != wathenav1.TradeOutcome_TRADE_OUTCOME_REFUSED {
		t.Errorf("with the rule on: %v", got)
	}
	r := rules.Base(true)
	r.Items, _ = rules.ItemsOf(map[string]int{"block_list_refuses_trade": 0})
	z.Mods = &Mods{Rules: r}
	if got := tradeOutcomes(doTrade(z, a, TradeAction{Request: netIDOf(a, b)})); len(got) != 1 || got[0] != wathenav1.TradeOutcome_TRADE_OUTCOME_REQUEST_SENT {
		t.Errorf("with the rule off: %v", got)
	}
	if rq := eventsOf(b, (*wathenav1.Event).GetTradeRequest, (*wathenav1.Event).HasTradeRequest); len(rq) != 1 {
		t.Errorf("with the rule off, the blocking player saw %d requests, want 1", len(rq))
	}
}

// TestTradeEndsAtLogout checks that a player who leaves in the middle of a
// trade ends it for the other, and nothing moves.
func TestTradeEndsAtLogout(t *testing.T) {
	z, a, b, _ := storageZone(t, stack(0, 909, 5))
	openTrade(t, z, a, b)
	doTrade(z, a, TradeAction{AddSlot: 0, AddAmount: 5})
	doTrade(z, a, TradeAction{Lock: true})
	doTrade(z, b, TradeAction{Lock: true})
	doTrade(z, a, TradeAction{Confirm: true})
	z.leave(a, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	z.stepAt(z.now + 50)
	if got := tradeOutcomes(events(b)); len(got) != 1 || got[0] != wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED || b.trade != nil {
		t.Errorf("b: %v", got)
	}
	if b.inventory.Count(909) != 0 || a.inventory.Count(909) != 5 {
		t.Error("items moved")
	}
	// b's confirm does nothing now.
	doTrade(z, b, TradeAction{Confirm: true})
	if b.inventory.Count(909) != 0 {
		t.Error("items moved after the cancel")
	}
}

// TestPartyBoundLeave checks that a player who leaves its party loses its
// party-bound items.
func TestPartyBoundLeave(t *testing.T) {
	items := []character.Item{stack(0, 909, 3), stack(1, 909, 2)}
	items[0].Bound = item.BoundParty
	z, a, _, _ := storageZone(t, items...)
	z.partyBoundLeft(a)
	if a.inventory.Count(909) != 2 || a.inventory.Slots[0].ID != 0 {
		t.Errorf("after leaving: %d", a.inventory.Count(909))
	}
}
