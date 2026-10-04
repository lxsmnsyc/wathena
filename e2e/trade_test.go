package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// tradeLine waits for the line the fixture's trade event sends, and
// returns it.
func (p *Player) tradeLine() string {
	p.t.Helper()
	return p.Expect("the fixture's trade line", func(e *wathenav1.Event) bool {
		c := e.GetChat()
		return c != nil && c.GetChannel() == wathenav1.ChatChannel_CHAT_CHANNEL_SYSTEM && strings.HasPrefix(c.GetText(), "e2e trade: ")
	}).GetChat().GetText()
}

// zeny is the zeny the player sees.
func (p *Player) zeny() int64 { return p.Stat(wathenav1.StatId_STAT_ID_ZENY) }

// playTrade gives Alice zeny and runs a trade: Alice gives 5 Jellopy and
// 300 zeny, Bob gives 2 Red Potions. It checks both sides.
func playTrade(t *testing.T, alice, bob *Player) {
	t.Helper()
	alice.MustCommand("!zeny 500")
	alice.Until("500 zeny", func(_ View, g Game) bool { return g.Stats[wathenav1.StatId_STAT_ID_ZENY] == 500 })
	bobZeny := bob.zeny()
	a, b := Trade(alice, bob, Offer{Items: map[uint32]int{Jellopy: 5}, Zeny: 300}, Offer{Items: map[uint32]int{RedPotion: 2}})
	if a != wathenav1.TradeOutcome_TRADE_OUTCOME_COMPLETED || b != wathenav1.TradeOutcome_TRADE_OUTCOME_COMPLETED {
		t.Fatalf("the trade closed with %s and %s", a, b)
	}
	alice.Until("the traded items", func(_ View, g Game) bool {
		return g.Count(Jellopy) == 15 && g.Count(RedPotion) == 12 && g.Stats[wathenav1.StatId_STAT_ID_ZENY] == 200
	})
	bob.Until("the traded items", func(_ View, g Game) bool {
		return g.Count(Jellopy) == 25 && g.Count(RedPotion) == 8 && g.Stats[wathenav1.StatId_STAT_ID_ZENY] == bobZeny+300
	})
	if alice.Seen(func(e *wathenav1.Event) bool {
		return e.GetInventoryRemove().GetReason() == wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_TRADED
	}) == nil {
		t.Error("Alice saw no InventoryRemove with INVENTORY_REMOVE_REASON_TRADED")
	}
	// Bob confirmed last, so the event names him first.
	want := "e2e trade: Bob and Alice, 7 items, 0 and 300 zeny"
	for _, p := range []*Player{alice, bob} {
		if got := p.tradeLine(); got != want {
			t.Errorf("%s got the trade line %q, want %q", p.Name, got, want)
		}
	}
}

// TestTrade: two players trade items and zeny. Both inventories and both
// zeny amounts change, and the fixture's trade event tells both what
// changed hands.
func TestTrade(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	playTrade(t, alice, bob)
	srv.CheckLogs()
}

// TestTradeInTheDatabase: the same trade with PostgreSQL. Its item and
// zeny log lines have the action trade and were written in one
// transaction, and the saved characters hold the result.
func TestTradeInTheDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	playTrade(t, alice, bob)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	type line struct {
		item   uint32
		amount int
		tx     string
		at     time.Time
	}
	var lines []line
	deadline := time.Now().Add(10 * time.Second)
	for len(lines) < 2 && time.Now().Before(deadline) {
		lines = nil
		rows, err := srv.DB.Query(ctx, `SELECT item_id, amount, xmin::text, at FROM item_log WHERE action = 'trade' ORDER BY id`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var l line
			if err := rows.Scan(&l.item, &l.amount, &l.tx, &l.at); err != nil {
				t.Fatal(err)
			}
			lines = append(lines, l)
		}
		rows.Close()
		time.Sleep(100 * time.Millisecond)
	}
	got := map[uint32]int{}
	for _, l := range lines {
		got[l.item] += l.amount
	}
	if len(lines) != 2 || got[Jellopy] != 5 || got[RedPotion] != 2 {
		t.Fatalf("trade item log lines %+v", lines)
	}
	var zenyTx string
	var zeny int64
	var zenyAt time.Time
	if err := srv.DB.QueryRow(ctx, `SELECT xmin::text, amount, at FROM zeny_log WHERE action = 'trade'`).Scan(&zenyTx, &zeny, &zenyAt); err != nil {
		t.Fatal(err)
	}
	if lines[0].tx != lines[1].tx || zenyTx != lines[0].tx || zeny != 300 || !lines[0].at.Equal(lines[1].at) || !zenyAt.Equal(lines[0].at) {
		t.Errorf("the trade's lines: items %+v, zeny %d in transaction %s at %v", lines, zeny, zenyTx, zenyAt)
	}

	srv.Stop()
	for name, want := range map[string][3]int64{"Alice": {15, 12, 200}, "Bob": {25, 8, 300}} {
		var jellopy, potions, z int64
		if err := srv.DB.QueryRow(ctx, `SELECT
			coalesce((SELECT sum(amount) FROM inventory WHERE character_id = c.id AND item_id = $2), 0),
			coalesce((SELECT sum(amount) FROM inventory WHERE character_id = c.id AND item_id = $3), 0), c.zeny
			FROM characters c WHERE c.name = $1`, name, Jellopy, RedPotion).Scan(&jellopy, &potions, &z); err != nil {
			t.Fatal(err)
		}
		if [3]int64{jellopy, potions, z} != want {
			t.Errorf("%s saved %d Jellopy, %d Red Potions and %d zeny, want %v", name, jellopy, potions, z, want)
		}
	}
	srv.CheckLogs()
}

// TestTradeRefused: the fixture's trade hook refuses a trade with
// Contraband, and both players get its message while nothing changes
// hands. A player who blocks the requester refuses at once, and sees no
// request. A player 3 cells away is too far.
func TestTradeRefused(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	carol := srv.Join(t, "Carol")
	alice.MustCommand("!give e2e.fixture:Contraband 2")

	if r := alice.RequestTrade(bob); r.GetOutcome() != wathenav1.TradeOutcome_TRADE_OUTCOME_REQUEST_SENT {
		t.Fatalf("asking Bob: %v", r)
	}
	bob.TradeRequested("Alice")
	bob.AcceptTrade()
	alice.TradeOpened()
	if o := alice.OfferItem(Contraband, 2); o != wathenav1.TradeOutcome_TRADE_OUTCOME_UNSPECIFIED {
		t.Fatalf("offering Contraband: %s", o)
	}
	if o := bob.OfferItem(Jellopy, 1); o != wathenav1.TradeOutcome_TRADE_OUTCOME_UNSPECIFIED {
		t.Fatalf("offering a Jellopy: %s", o)
	}
	alice.LockTrade()
	bob.LockTrade()
	alice.ConfirmTrade()
	bob.ConfirmTrade()
	for _, p := range []*Player{alice, bob} {
		r := p.Expect("the hook's refusal", func(e *wathenav1.Event) bool {
			return e.GetTradeResult().GetOutcome() == wathenav1.TradeOutcome_TRADE_OUTCOME_HOOK
		}).GetTradeResult()
		if r.GetMessage() != "No contraband here." {
			t.Errorf("%s got the hook's message %q", p.Name, r.GetMessage())
		}
		if o := p.TradeClosed(); o != wathenav1.TradeOutcome_TRADE_OUTCOME_FAILED {
			t.Errorf("%s: the trade closed with %s", p.Name, o)
		}
	}
	if alice.Count(Contraband) != 2 || bob.Count(Jellopy) != 20 || alice.Count(Jellopy) != 20 {
		t.Errorf("after the refused trade Alice holds %d Contraband and %d Jellopy, Bob %d Jellopy",
			alice.Count(Contraband), alice.Count(Jellopy), bob.Count(Jellopy))
	}

	// A block list refuses at once.
	bob.BlockName("Alice", false)
	if r := alice.RequestTrade(bob); r.GetOutcome() != wathenav1.TradeOutcome_TRADE_OUTCOME_REFUSED || r.GetName() != "Bob" {
		t.Errorf("asking a player who blocks Alice: %v", r)
	}
	// Carol's request comes after Alice's, so Bob saw Alice's by then if
	// it came.
	if r := carol.RequestTrade(bob); r.GetOutcome() != wathenav1.TradeOutcome_TRADE_OUTCOME_REQUEST_SENT {
		t.Fatalf("Carol asks Bob: %v", r)
	}
	bob.TradeRequested("Carol")
	// Bob saw Alice's first request, before the block, and no other.
	n := 0
	events, _ := bob.Events(0)
	for _, e := range events {
		if e.GetTradeRequest().GetFrom() == "Alice" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("Bob saw %d trade requests from Alice, want only the one before the block", n)
	}
	bob.send("refuse", func(ctx context.Context) error { return bob.TradeRespond(ctx, false) })
	if r := carol.Expect("the refusal", (*wathenav1.Event).HasTradeResult).GetTradeResult(); r.GetOutcome() != wathenav1.TradeOutcome_TRADE_OUTCOME_REFUSED {
		t.Errorf("Bob refused Carol: %v", r)
	}

	// Trade range is 2 cells.
	v := carol.View()
	carol.WalkTo(v.X+3, v.Y)
	if r := carol.RequestTrade(alice); r.GetOutcome() != wathenav1.TradeOutcome_TRADE_OUTCOME_TOO_FAR {
		t.Errorf("asking from 3 cells away: %v", r)
	}
	srv.CheckLogs()
}

// TestRestrictions: the fixture's items with one restriction each show
// it in InventoryAdd. A no_trade item cannot be offered, a no_storage
// item cannot be stored, by a player or a script, and a no_drop item
// cannot be dropped. An equipped item cannot be offered or stored.
func TestRestrictions(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	for name, want := range map[string]uint32{"Heirloom": 2, "Ledger": 16, "Anchor": 1, "Contraband": 0} {
		mark := alice.mark
		alice.MustCommand("!give e2e.fixture:" + name + " 2")
		alice.mark = mark
		add := alice.Expect(name, func(e *wathenav1.Event) bool { return e.HasInventoryAdd() }).GetInventoryAdd()
		if add.GetItemId() != map[string]uint32{"Heirloom": Heirloom, "Ledger": Ledger, "Anchor": Anchor, "Contraband": Contraband}[name] || add.GetRestrictions() != want {
			t.Errorf("%s came as item %d with restrictions %d, want %d", name, add.GetItemId(), add.GetRestrictions(), want)
		}
		alice.Skip()
	}
	alice.MustCommand("!give e2e.fixture:Charm 1")
	slot := alice.Game().SlotOf(Charm)
	alice.send("equip", func(ctx context.Context) error { return alice.EquipItem(ctx, slot) })
	alice.Expect("the charm on", func(e *wathenav1.Event) bool { return e.HasItemEquipped() })

	if o := alice.TryDrop(Anchor, 1); o != wathenav1.DropItemFailure_DROP_ITEM_FAILURE_CANNOT_DROP {
		t.Errorf("dropping an Anchor: %s", o)
	}
	alice.OpenKafra("Storage")
	if o := alice.TryStorePut(Ledger, 1); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_RESTRICTED {
		t.Errorf("storing a Ledger: %s", o)
	}
	if o := alice.TryStorePut(Charm, 1); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_NOT_STORABLE {
		t.Errorf("storing an equipped charm: %s", o)
	}
	if o := alice.TryStorePut(Heirloom, 1); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_UNSPECIFIED {
		t.Errorf("storing an Heirloom, which only no_trade keeps: %s", o)
	}
	alice.CloseStorage()
	if got := alice.Command("!give e2e.fixture:Ledger 1 to=storage"); got != "false" {
		t.Errorf("a script stores a Ledger: %s", got)
	}

	if r := alice.RequestTrade(bob); r.GetOutcome() != wathenav1.TradeOutcome_TRADE_OUTCOME_REQUEST_SENT {
		t.Fatalf("asking Bob: %v", r)
	}
	bob.TradeRequested("Alice")
	bob.AcceptTrade()
	alice.TradeOpened()
	if o := alice.OfferItem(Heirloom, 1); o != wathenav1.TradeOutcome_TRADE_OUTCOME_RESTRICTED {
		t.Errorf("offering an Heirloom: %s", o)
	}
	if o := alice.OfferSlot(slot, 1); o != wathenav1.TradeOutcome_TRADE_OUTCOME_EQUIPPED {
		t.Errorf("offering the equipped charm: %s", o)
	}
	if o := alice.OfferItem(Anchor, 1); o != wathenav1.TradeOutcome_TRADE_OUTCOME_UNSPECIFIED {
		t.Errorf("offering an Anchor, which only no_drop keeps: %s", o)
	}
	alice.send("cancel", func(ctx context.Context) error { return alice.TradeCancel(ctx) })
	for _, p := range []*Player{alice, bob} {
		if o := p.TradeClosed(); o != wathenav1.TradeOutcome_TRADE_OUTCOME_CANCELLED {
			t.Errorf("%s: the trade closed with %s", p.Name, o)
		}
	}
	if alice.Count(Anchor) != 2 || bob.Count(Anchor) != 0 {
		t.Errorf("after the cancel Alice holds %d Anchors, Bob %d", alice.Count(Anchor), bob.Count(Anchor))
	}
	srv.CheckLogs()
}

// boundSlot is the player's first slot of an item bound as b.
func (p *Player) boundSlot(id uint32, b wathenav1.ItemBound) int {
	p.t.Helper()
	slot := p.SlotWhere(func(it Item) bool { return it.ID == id && it.Bound == b })
	if slot < 0 {
		p.t.Fatalf("%s has no item %d bound as %s", p.Name, id, b)
	}
	return slot
}

// TestBoundItems: a script gives bound Jellopy, which do not stack with
// the others. An account-bound copy goes into the account's storage. A
// character-bound copy cannot be stored, offered or dropped. A
// party-bound copy goes away when its owner leaves the party.
func TestBoundItems(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	alice.MustCommand("!give Jellopy 2 bound=account")
	alice.MustCommand("!give Jellopy 3 bound=character")
	account := alice.boundSlot(Jellopy, wathenav1.ItemBound_ITEM_BOUND_ACCOUNT)
	character := alice.boundSlot(Jellopy, wathenav1.ItemBound_ITEM_BOUND_CHARACTER)
	plain := alice.boundSlot(Jellopy, wathenav1.ItemBound_ITEM_BOUND_UNSPECIFIED)
	if account == character || account == plain || character == plain || alice.Count(Jellopy) != 25 {
		t.Fatalf("bound Jellopy in slots %d, %d and %d: %v", account, character, plain, alice.Game().Items)
	}

	alice.OpenKafra("Storage")
	put := func(slot int) wathenav1.StorageOutcome {
		alice.send("store", func(ctx context.Context) error { return alice.StoragePut(ctx, slot, 1) })
		return alice.storageResult("the bound Jellopy in the storage", func(e *wathenav1.Event) bool {
			return e.GetStorageAdd().GetKind() == wathenav1.StorageKind_STORAGE_KIND_PERSONAL && e.HasStorageAdd()
		})
	}
	if o := put(account); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_UNSPECIFIED {
		t.Errorf("storing an account-bound Jellopy: %s", o)
	}
	if o := put(character); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_BOUND {
		t.Errorf("storing a character-bound Jellopy: %s", o)
	}
	if b := alice.Game().Storage.Items; len(b) != 1 {
		t.Errorf("the storage holds %v", b)
	} else {
		for _, it := range b {
			if it.Bound != wathenav1.ItemBound_ITEM_BOUND_ACCOUNT {
				t.Errorf("the stored Jellopy is bound as %s", it.Bound)
			}
		}
	}
	alice.CloseStorage()

	alice.send("drop", func(ctx context.Context) error { return alice.DropItem(ctx, character, 1) })
	if f := alice.Expect("the drop's failure", (*wathenav1.Event).HasDropItemFailed).GetDropItemFailed(); f.GetReason() != wathenav1.DropItemFailure_DROP_ITEM_FAILURE_BOUND {
		t.Errorf("dropping a character-bound Jellopy: %v", f)
	}
	if r := alice.RequestTrade(bob); r.GetOutcome() != wathenav1.TradeOutcome_TRADE_OUTCOME_REQUEST_SENT {
		t.Fatalf("asking Bob: %v", r)
	}
	bob.TradeRequested("Alice")
	bob.AcceptTrade()
	alice.TradeOpened()
	if o := alice.OfferSlot(character, 1); o != wathenav1.TradeOutcome_TRADE_OUTCOME_BOUND {
		t.Errorf("offering a character-bound Jellopy: %s", o)
	}
	if o := alice.OfferSlot(account, 1); o != wathenav1.TradeOutcome_TRADE_OUTCOME_BOUND {
		t.Errorf("offering an account-bound Jellopy: %s", o)
	}
	alice.send("cancel", func(ctx context.Context) error { return alice.TradeCancel(ctx) })
	alice.TradeClosed()
	bob.TradeClosed()

	// Party-bound copies go with the party.
	MakeParty("Crew", false, false, alice, bob)
	alice.MustCommand("!give Jellopy 4 bound=party")
	party := alice.boundSlot(Jellopy, wathenav1.ItemBound_ITEM_BOUND_PARTY)
	before := alice.Count(Jellopy)
	alice.send("leave", func(ctx context.Context) error { return alice.PartyLeave(ctx) })
	r := alice.Expect("the party-bound Jellopy to go", func(e *wathenav1.Event) bool {
		r := e.GetInventoryRemove()
		return r != nil && r.GetReason() == wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_BOUND
	}).GetInventoryRemove()
	if int(r.GetSlot()) != party || r.GetAmount() != 4 {
		t.Errorf("removed %v, the party-bound Jellopy were in slot %d", r, party)
	}
	alice.Until("4 fewer Jellopy", func(_ View, g Game) bool { return g.Count(Jellopy) == before-4 })
	srv.CheckLogs()
}
