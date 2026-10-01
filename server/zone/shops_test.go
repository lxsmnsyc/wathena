package zone

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// shopItems are items with rAthena's prices: a Red Potion sells for 50
// and buys back at 25.
func shopItems() *item.Table {
	return item.New(&item.Data{Mode: "renewal", Items: []item.Def{
		{ID: 501, AegisName: "Red_Potion", Type: "Healing", Weight: 70, Stackable: true, Buy: 50, Sell: 25},
		{ID: 909, AegisName: "Jellopy", Type: "Etc", Weight: 10, Stackable: true, Buy: 6, Sell: 3},
		{ID: 1201, AegisName: "Knife", Type: "Weapon", Weight: 400, Buy: 50, Sell: 25},
		{ID: 7777, AegisName: "Untradable", Type: "Etc", Weight: 1, Stackable: true, NoSell: true, Buy: 10, Sell: 5},
		{ID: 715, AegisName: "Heavy", Type: "Etc", Weight: 30000, Stackable: true, Buy: 1, Sell: 1},
	}})
}

var toolShop = &Shop{Name: "core:Tool Dealer", Discount: true, Items: []ShopItem{{ID: 501, Price: 50}, {ID: 1201, Price: 50}, {ID: 715, Price: 1}}}

// shopZone is a zone with a shop NPC at 20,20 and a player next to it
// with 1000 zeny. The player's first frame is already read.
func shopZone(t *testing.T) (*Zone, *Player, *entity) {
	t.Helper()
	z := New("map/test", open(40), rng.FromKey([32]byte{6}).Stream(), nil)
	z.Items = shopItems()
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	z.Mods = &Mods{Log: slog.New(slog.DiscardHandler), Shops: map[string]*Shop{toolShop.Name: toolShop}}
	z.SetNPCs([]NPC{{Name: "core:Tool Dealer", DisplayName: "Tool Dealer", X: 20, Y: 20, Sprite: 53, Shop: toolShop}})
	p := add(z, "a")
	p.entity.x, p.entity.y = 21, 20
	p.record.Zeny = 1000
	p.maxWeight = 20000
	z.stepAt(50)
	events(p)
	var n *entity
	for e := range z.entities.all() {
		if e.npc != nil {
			n = e
		}
	}
	return z, p, n
}

func shopOutcome(ev []*wathenav1.Event) wathenav1.ShopOutcome {
	for _, e := range ev {
		if r := e.GetShopResult(); r != nil {
			return r.GetOutcome()
		}
	}
	return wathenav1.ShopOutcome_SHOP_OUTCOME_UNSPECIFIED
}

func shopOpen(ev []*wathenav1.Event) *wathenav1.ShopOpen {
	for _, e := range ev {
		if o := e.GetShopOpen(); o != nil {
			return o
		}
	}
	return nil
}

func buy(z *Zone, p *Player, n *entity, lines ...buyLine) []*wathenav1.Event {
	id, _ := p.netIDs.get(n)
	z.handle(command{player: p, shopBuy: &shopBuyCommand{npc: id, lines: lines}})
	z.stepAt(z.now + 50)
	return events(p)
}

func sell(z *Zone, p *Player, n *entity, lines ...sellLine) []*wathenav1.Event {
	id, _ := p.netIDs.get(n)
	z.handle(command{player: p, shopSell: &shopSellCommand{npc: id, lines: lines}})
	z.stepAt(z.now + 50)
	return events(p)
}

// TestShopPrices checks pc_modifybuyvalue and pc_modifysellvalue: rates
// of 5 + 2 per level for Discount and Overcharge, one less at level 10,
// 5 + 4 per level for Compulsion Discount, the better of the two, and
// rounding down.
func TestShopPrices(t *testing.T) {
	for _, c := range []struct {
		skills    map[int]int
		value     int
		buy, sell int
	}{
		{nil, 50, 50, 50},
		{map[int]int{skillDiscount: 1}, 50, 46, 50},                        // 7%: 46.5
		{map[int]int{skillDiscount: 10}, 50, 38, 50},                       // 24%
		{map[int]int{skillDiscount: 10}, 1, 1, 1},                          // min_shop_buy
		{map[int]int{skillCompulsion: 5}, 100, 75, 100},                    // 25%
		{map[int]int{skillDiscount: 10, skillCompulsion: 1}, 100, 76, 100}, // Discount's 24% beats 9%
		{map[int]int{skillOvercharge: 10}, 25, 25, 31},                     // 24%: 31
		{map[int]int{skillOvercharge: 3}, 25, 25, 27},                      // 11%: 27.75
		{map[int]int{skillOvercharge: 10}, 0, 1, 0},
	} {
		if got := buyPrice(c.skills, c.value); got != c.buy {
			t.Errorf("buy %d with %v: %d, want %d", c.value, c.skills, got, c.buy)
		}
		if got := sellPrice(c.skills, c.value); got != c.sell {
			t.Errorf("sell %d with %v: %d, want %d", c.value, c.skills, got, c.sell)
		}
	}
}

func TestShopOpensOnClick(t *testing.T) {
	z, p, n := shopZone(t)
	p.char.Skills = map[int]int{skillDiscount: 10, skillOvercharge: 10}
	p.inventory.Add(z.Items.Get(909), 5, p.maxWeight)
	p.inventory.Add(z.Items.Get(7777), 1, p.maxWeight)
	o := shopOpen(click(z, p, n))
	if o == nil || len(o.GetItems()) != 3 || o.GetItems()[0].GetPrice() != 38 {
		t.Fatalf("shop %v", o)
	}
	// The sell list leaves out what the shop does not buy, and pays with
	// Overcharge: 3 * 1.24 = 3.
	if s := o.GetSellable(); len(s) != 1 || s[0].GetSlot() != 0 || s[0].GetPrice() != 3 {
		t.Fatalf("sellable %v", s)
	}
}

func TestShopBuy(t *testing.T) {
	z, p, n := shopZone(t)
	click(z, p, n)
	ev := buy(z, p, n, buyLine{501, 3})
	if shopOutcome(ev) != wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT || p.record.Zeny != 850 || p.inventory.Count(501) != 3 {
		t.Fatalf("outcome %v, zeny %d, potions %d", shopOutcome(ev), p.record.Zeny, p.inventory.Count(501))
	}
	// The window closes after a purchase.
	if ev := buy(z, p, n, buyLine{501, 1}); shopOutcome(ev) != wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED || p.record.Zeny != 850 {
		t.Fatalf("a purchase without an open shop: %v, zeny %d", shopOutcome(ev), p.record.Zeny)
	}
	for _, c := range []struct {
		name  string
		lines []buyLine
		want  wathenav1.ShopOutcome
	}{
		{"too little zeny", []buyLine{{501, 20}}, wathenav1.ShopOutcome_SHOP_OUTCOME_NOT_ENOUGH_ZENY},
		{"not sold here", []buyLine{{909, 1}}, wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED},
		{"too heavy", []buyLine{{715, 1}}, wathenav1.ShopOutcome_SHOP_OUTCOME_OVERWEIGHT},
		{"over the stack", []buyLine{{501, 30000}}, wathenav1.ShopOutcome_SHOP_OUTCOME_OVERWEIGHT},
	} {
		click(z, p, n)
		ev := buy(z, p, n, c.lines...)
		if got := shopOutcome(ev); got != c.want || p.record.Zeny != 850 || p.inventory.Count(501) != 3 {
			t.Errorf("%s: %v, zeny %d, potions %d", c.name, got, p.record.Zeny, p.inventory.Count(501))
		}
	}
	// Equipment buys one at a time: a hexed amount of 3 buys 1.
	click(z, p, n)
	if ev := buy(z, p, n, buyLine{1201, 3}); shopOutcome(ev) != wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT || p.inventory.Count(1201) != 1 || p.record.Zeny != 800 {
		t.Fatalf("knife: %v, knives %d, zeny %d", shopOutcome(ev), p.inventory.Count(1201), p.record.Zeny)
	}
	// Two lines that fit one by one and not together change nothing.
	p.maxWeight = 1 << 30
	p.inventory.Add(z.Items.Get(501), 29990, p.maxWeight)
	p.record.Zeny = 10000
	click(z, p, n)
	if ev := buy(z, p, n, buyLine{501, 6}, buyLine{501, 6}); shopOutcome(ev) != wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED || p.record.Zeny != 10000 {
		t.Fatalf("two lines over the stack: %v, zeny %d", shopOutcome(ev), p.record.Zeny)
	}
}

func TestShopSell(t *testing.T) {
	z, p, n := shopZone(t)
	p.inventory.Add(z.Items.Get(909), 10, p.maxWeight)
	p.inventory.Add(z.Items.Get(7777), 1, p.maxWeight)
	click(z, p, n)
	ev := sell(z, p, n, sellLine{0, 4})
	if shopOutcome(ev) != wathenav1.ShopOutcome_SHOP_OUTCOME_SOLD || p.record.Zeny != 1012 || p.inventory.Count(909) != 6 {
		t.Fatalf("outcome %v, zeny %d, jellopies %d", shopOutcome(ev), p.record.Zeny, p.inventory.Count(909))
	}
	for _, c := range []struct {
		name  string
		lines []sellLine
	}{
		{"an item the shop does not buy", []sellLine{{0, 1}, {1, 1}}},
		{"the same slot twice", []sellLine{{0, 3}, {0, 3}}},
		{"more than the slot", []sellLine{{0, 7}}},
		{"an empty slot", []sellLine{{5, 1}}},
	} {
		click(z, p, n)
		if ev := sell(z, p, n, c.lines...); shopOutcome(ev) != wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED || p.record.Zeny != 1012 || p.inventory.Count(909) != 6 {
			t.Errorf("%s: %v, zeny %d, jellopies %d", c.name, shopOutcome(ev), p.record.Zeny, p.inventory.Count(909))
		}
	}
	// Zeny over MAX_ZENY is lost, as pc_getzeny.
	p.record.Zeny = character.MaxZeny - 5
	click(z, p, n)
	if ev := sell(z, p, n, sellLine{0, 6}); shopOutcome(ev) != wathenav1.ShopOutcome_SHOP_OUTCOME_SOLD || p.record.Zeny != character.MaxZeny {
		t.Fatalf("zeny %d after selling near the cap", p.record.Zeny)
	}
	// A player who walked away cannot sell.
	p.inventory.Add(z.Items.Get(909), 1, p.maxWeight)
	click(z, p, n)
	p.entity.x = n.x + npcClickRange + 1
	if ev := sell(z, p, n, sellLine{0, 1}); shopOutcome(ev) != wathenav1.ShopOutcome_SHOP_OUTCOME_FAILED || p.inventory.Count(909) != 1 {
		t.Fatalf("a sale from too far: %v", shopOutcome(ev))
	}
}

// TestShopNoDuplication sends purchases, sales, drops and uses of the
// same slot at once, from many goroutines, through a running zone. Every
// item and every zeny is accounted for: what the player holds, what lies
// on the ground, and what changed hands.
func TestShopNoDuplication(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{7}).Stream(), nil)
	z.Items = shopItems()
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	shop := &Shop{Name: "core:Jellopy Trader", Items: []ShopItem{{ID: 909, Price: 6}}}
	z.Mods = &Mods{Log: slog.New(slog.DiscardHandler), Shops: map[string]*Shop{shop.Name: shop}}
	z.SetNPCs([]NPC{{Name: shop.Name, DisplayName: "Trader", X: 20, Y: 20, Sprite: 53, Shop: shop}})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go z.Run(ctx)
	c := z.NewCharacter("a", Look{})
	c.X, c.Y, c.Zeny = 21, 20, 100_000
	c.Items = []character.Item{{Slot: 0, ID: 909, Amount: 500, Serial: item.NewSerial()}}
	s, err := z.JoinCharacter(ctx, &c, nil)
	if err != nil {
		t.Fatal(err)
	}
	z.control <- func() { z.players[1].maxWeight = 1 << 30 }
	// The player learns the NPC's ID in its first frames.
	var npc uint32
	var mu sync.Mutex
	sold, bought := 0, 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		for m := range s.Out {
			for _, e := range m.GetFrame().GetEvents() {
				if a := e.GetAppear(); a != nil && a.GetKind() == wathenav1.EntityKind_ENTITY_KIND_NPC {
					mu.Lock()
					npc = a.GetId()
					mu.Unlock()
				}
				if r := e.GetInventoryRemove(); r != nil && r.GetReason() == wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_SOLD {
					mu.Lock()
					sold += int(r.GetAmount())
					mu.Unlock()
				}
				if a := e.GetInventoryAdd(); a != nil {
					mu.Lock()
					bought += int(a.GetAmount())
					mu.Unlock()
				}
			}
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		id := npc
		mu.Unlock()
		if id != 0 {
			npc = id
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the NPC never appeared")
		}
		time.Sleep(10 * time.Millisecond)
	}
	var wg sync.WaitGroup
	seq := uint32(1)
	var seqMu sync.Mutex
	next := func() uint32 {
		seqMu.Lock()
		defer seqMu.Unlock()
		seq++
		return seq
	}
	for g := range 8 {
		wg.Go(func() {
			for i := range 25 {
				switch (g + i) % 4 {
				case 0:
					s.NpcClick(next(), npc)
					s.ShopSell(next(), npc, []SellLine{{Slot: 0, Amount: 3}})
				case 1:
					s.NpcClick(next(), npc)
					s.ShopBuy(next(), npc, []BuyLine{{Item: 909, Amount: 2}})
				case 2:
					s.DropItem(next(), 0, 1)
				case 3:
					s.UseItem(next(), 0)
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
	wg.Wait()
	time.Sleep(200 * time.Millisecond)
	// Read the final state on the zone's goroutine.
	final := make(chan [3]int64, 1)
	z.control <- func() {
		p := z.players[1]
		ground := int64(0)
		for e := range z.entities.all() {
			if e.item != nil {
				ground += int64(e.item.amount)
			}
		}
		final <- [3]int64{int64(p.inventory.Count(909)), ground, p.record.Zeny}
	}
	got := <-final
	s.Leave()
	<-done
	mu.Lock()
	defer mu.Unlock()
	held, ground, zeny := got[0], got[1], got[2]
	if held+ground != int64(500+bought-sold) {
		t.Errorf("items: %d held + %d on the ground, want 500 + %d bought - %d sold", held, ground, bought, sold)
	}
	if zeny != int64(100_000-6*bought+3*sold) {
		t.Errorf("zeny %d, want 100000 - 6 * %d + 3 * %d", zeny, bought, sold)
	}
	if sold == 0 || bought == 0 {
		t.Errorf("sold %d, bought %d: the race did not trade", sold, bought)
	}
}
