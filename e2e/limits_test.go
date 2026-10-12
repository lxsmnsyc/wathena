package e2e

import (
	"context"
	"maps"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// LimitRuleset is the ruleset mod in testdata/mods that lets the
// account's storage grow to 700 slots and the cart to 150, and whose
// block lists do not refuse trades.
const LimitRuleset = "e2e.limitrules"

// limitsAre waits until the player's PlayerLimits hold want.
func (p *Player) limitsAre(what string, want map[string]int) {
	p.t.Helper()
	p.Until(what, func(_ View, g Game) bool {
		for name, v := range want {
			if g.Limits[name] != v {
				return false
			}
		}
		return true
	})
}

// TestExpandLimits: the fixture's "!expand" calls player:expand. The
// player's inventory slots grow up to the max and fall back to the base,
// and the player gets PlayerLimits after each change. The account's
// storage is at its max by default.
func TestExpandLimits(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	alice.limitsAre("the limits at login", map[string]int{"inventory_slots": 100, "storage_slots": 600, "cart_slots": 100})
	for _, c := range []struct{ command, want string }{
		{"!expand inventory_slots 20", "120"},
		{"!expand inventory_slots 500", "200"},
		{"!expand inventory_slots -150", "100"},
		{"!expand inventory_slots 10", "110"},
		{"!expand storage_slots 50", "600"},
	} {
		if got := alice.Command(c.command); got != c.want {
			t.Errorf("%q answered %q, want %q", c.command, got, c.want)
		}
	}
	alice.limitsAre("the expanded inventory", map[string]int{"inventory_slots": 110, "storage_slots": 600})
	srv.CheckLogs()
}

// TestExpandLimitsInTheDatabase: with PostgreSQL and e2e.limitrules, the
// expanded inventory, account storage and cart come back after the player
// logs in again and after a restart, and their rows are saved. The
// storage window opens with the expanded size.
func TestExpandLimitsInTheDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true, Ruleset: LimitRuleset})
	alice := srv.Join(t, "Alice")
	for _, c := range []struct{ command, want string }{
		{"!expand inventory_slots 20", "120"},
		{"!expand storage_slots 50", "650"},
		{"!expand cart_slots 1000", "150"},
		{"!expand cart_slots -30", "120"},
	} {
		if got := alice.Command(c.command); got != c.want {
			t.Fatalf("%q answered %q, want %q", c.command, got, c.want)
		}
	}
	want := map[string]int{"inventory_slots": 120, "storage_slots": 650, "cart_slots": 120}
	check := func(when string, p *Player) {
		t.Helper()
		p.limitsAre(when, want)
		if got := p.Game().Limits; !maps.Equal(got, want) {
			t.Errorf("%s: limits %v, want %v", when, got, want)
		}
		if o := p.OpenStorage(""); o.GetSize() != 650 {
			t.Errorf("%s: the storage opens with %d slots", when, o.GetSize())
		}
		p.CloseStorage()
	}
	check("after the expansions", alice)
	_ = alice.Close()
	alice = srv.Join(t, "Alice")
	check("logged in again", alice)
	_ = alice.Close()
	srv.Restart()
	alice = srv.Join(t, "Alice")
	check("after a restart", alice)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var inventory, account, cart int
	if err := srv.DB.QueryRow(ctx, `SELECT c.inventory_size,
		(SELECT slots FROM storage_sizes WHERE holder = 'account' AND account_id = c.account_id),
		(SELECT slots FROM storage_sizes WHERE holder = 'cart' AND character_id = c.id)
		FROM characters c WHERE c.name = 'Alice'`).Scan(&inventory, &account, &cart); err != nil {
		t.Fatal(err)
	}
	if inventory != 120 || account != 650 || cart != 120 {
		t.Errorf("saved inventory %d, storage %d, cart %d", inventory, account, cart)
	}
	srv.CheckLogs()
}

// TestTradeBlockListRule: with e2e.limitrules, whose
// block_list_refuses_trade is no, a player who blocks the requester still
// gets the request, as rAthena. TestTradeRefused covers the default.
func TestTradeBlockListRule(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Ruleset: LimitRuleset})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	bob.BlockName("Alice", false)
	if r := alice.RequestTrade(bob); r.GetOutcome() != wathenav1.TradeOutcome_TRADE_OUTCOME_REQUEST_SENT {
		t.Fatalf("asking a player who blocks Alice: %v", r)
	}
	bob.TradeRequested("Alice")
	srv.CheckLogs()
}
