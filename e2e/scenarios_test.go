package e2e

import (
	"os"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

func TestMain(m *testing.M) { os.Exit(Main(m)) }

// TestLoginWalkAndChat: two players log in and see each other, one
// walks and the other sees the walk, and both hear a chat line.
func TestLoginWalkAndChat(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	if v := alice.View(); v.Map != StartMap || v.X != 12 || v.Y != 12 {
		t.Errorf("Alice entered on %s at %d,%d, want %s at 12,12", v.Map, v.X, v.Y, StartMap)
	}
	if got := alice.Stat(wathenav1.StatId_STAT_ID_JOB); got != 1 {
		t.Errorf("Alice's job is %d, want the start job 1", got)
	}
	if alice.Count(RedPotion) != 10 || alice.Count(Jellopy) != 20 {
		t.Errorf("Alice's start items: %v", alice.Game().Items)
	}
	seen := bob.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, "Alice")
	alice.Find(wathenav1.EntityKind_ENTITY_KIND_PLAYER, "Bob")

	alice.WalkTo(14, 10)
	bob.Until("Alice's walk ends on 14,10", func(v View, _ Game) bool {
		for _, e := range v.Entities {
			if e.ID == seen.ID {
				return e.DestX == 14 && e.DestY == 10
			}
		}
		return false
	})

	alice.Say("hello from Alice")
	bob.Hears("Alice", "hello from Alice")
	srv.CheckLogs()
}

// TestCombatLootAndItems: a player kills a monster, picks up both of
// its drops, one with a roll, and uses a potion.
func TestCombatLootAndItems(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	p := srv.Join(t, "Fighter")
	dummy := p.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Dummy")
	if dealt := p.Kill(dummy); dealt <= 0 {
		t.Errorf("the kill dealt %d damage", dealt)
	}
	jellopy := p.PickUp(p.FindItem(Jellopy))
	if jellopy.GetAmount() != 1 || p.Count(Jellopy) != 21 {
		t.Errorf("picked up %d Jellopy, holding %d", jellopy.GetAmount(), p.Count(Jellopy))
	}
	charm := p.PickUp(p.FindItem(Charm))
	// item_rolls is off in core's ruleset, so the roll is its default.
	if r := charm.GetRolls(); len(r) != 1 || r[0].GetName() != "luck" || r[0].GetValue() != 3 || r[0].GetMin() != 1 || r[0].GetMax() != 5 {
		t.Errorf("the charm's rolls: %v", r)
	}
	p.Use(RedPotion)
	if p.Count(RedPotion) != 9 {
		t.Errorf("%d Red Potions after one use", p.Count(RedPotion))
	}
	srv.CheckLogs()
}

// TestSkillAndJobChange: a Swordman learns Bash, casts it on a monster,
// and becomes a Knight at the Mentor.
func TestSkillAndJobChange(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	p := srv.Join(t, "Sword")
	points := p.Stat(wathenav1.StatId_STAT_ID_SKILL_POINTS)
	if level := p.Learn(SkillBash); level != 1 {
		t.Fatalf("Bash level %d", level)
	}
	p.Until("a skill point spent", func(_ View, g Game) bool {
		return g.Stats[wathenav1.StatId_STAT_ID_SKILL_POINTS] == points-1
	})
	dummy := p.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Dummy")
	p.WalkTo(dummy.X, dummy.Y-1)
	if d := p.Cast(SkillBash, 1, dummy); d.GetHits() < 1 {
		t.Errorf("Bash: %v", d)
	}

	c := p.Talk("Mentor")
	c.Says("You are a Knight now.")
	c.Close()
	p.Until("the job is Knight", func(_ View, g Game) bool { return g.Stats[wathenav1.StatId_STAT_ID_JOB] == JobKnight })
	if e := p.Seen(func(e *wathenav1.Event) bool { return e.GetJobName().GetJob() == JobKnight }); e.GetJobName().GetName() == "" {
		t.Errorf("the Knight's name: %v", e)
	}
	if got := p.Stat(wathenav1.StatId_STAT_ID_JOB_LEVEL); got != 1 {
		t.Errorf("job level %d after the change", got)
	}
	srv.CheckLogs()
}

// TestShopAndScriptItems: a script gives items and zeny, a shop buys and
// sells, and a script takes items back.
func TestShopAndScriptItems(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	p := srv.Join(t, "Trader")
	shopping(t, p)
	srv.CheckLogs()
}

// shopping is the trading part of the tests: the Quartermaster gives 5
// Jellopy and 1,000 zeny, the player buys 2 Red Potions and sells 5
// Jellopy, and the Quartermaster takes 2 Jellopy.
func shopping(t *testing.T, p *Player) {
	t.Helper()
	c := p.Talk("Quartermaster")
	c.Choose("Give")
	c.Says("Here you are.")
	c.Close()
	p.Until("1,000 zeny and 25 Jellopy", func(_ View, g Game) bool {
		return g.Stats[wathenav1.StatId_STAT_ID_ZENY] == 1000 && g.Count(Jellopy) == 25
	})

	shop := p.OpenShop("Tool Dealer")
	var price int64
	for _, it := range shop.GetItems() {
		if it.GetItemId() == RedPotion {
			price = int64(it.GetPrice())
		}
	}
	if price != 50 {
		t.Fatalf("Red Potion costs %d in %v", price, shop.GetItems())
	}
	if out := p.Buy(shop, map[uint32]int{RedPotion: 2}); out != wathenav1.ShopOutcome_SHOP_OUTCOME_BOUGHT {
		t.Fatalf("buy: %s", out)
	}
	p.Until("900 zeny and 12 Red Potions", func(_ View, g Game) bool {
		return g.Stats[wathenav1.StatId_STAT_ID_ZENY] == 900 && g.Count(RedPotion) == 12
	})

	shop = p.OpenShop("Tool Dealer")
	var sell int64
	for _, s := range shop.GetSellable() {
		if p.Game().Items[int(s.GetSlot())].ID == Jellopy {
			sell = int64(s.GetPrice())
		}
	}
	if sell <= 0 {
		t.Fatalf("the shop does not buy Jellopy: %v", shop.GetSellable())
	}
	if out := p.Sell(shop, map[uint32]int{Jellopy: 5}); out != wathenav1.ShopOutcome_SHOP_OUTCOME_SOLD {
		t.Fatalf("sell: %s", out)
	}
	p.Until("the sale's zeny and 20 Jellopy", func(_ View, g Game) bool {
		return g.Stats[wathenav1.StatId_STAT_ID_ZENY] == 900+5*sell && g.Count(Jellopy) == 20
	})

	c = p.Talk("Quartermaster")
	c.Choose("Take")
	c.Says("Thank you.")
	c.Close()
	p.Until("18 Jellopy", func(_ View, g Game) bool { return g.Count(Jellopy) == 18 })
}

// TestWarp: the Guide sends the player to the other map.
func TestWarp(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	p := srv.Join(t, "Walker")
	p.Talk("Guide").Pick("Go to the field")
	m := p.MapChange(FieldMap)
	if m.GetX() != 12 || m.GetY() != 12 {
		t.Errorf("arrived at %d,%d", m.GetX(), m.GetY())
	}
	p.Until("standing on the new map", func(v View, _ Game) bool { return v.Map == FieldMap && v.X == 12 && v.Y == 12 })
	if c := p.Counts(); c.MapChanges != 1 {
		t.Errorf("%d map changes", c.MapChanges)
	}
	srv.CheckLogs()
}
