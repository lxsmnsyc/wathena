package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// auditStorage adds up what the audit's personal storages and carts
// hold, the entries named storage:<key>.
func auditStorage(t *testing.T, srv *Server) map[uint32]int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()
	zones, err := srv.Audit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out := map[uint32]int{}
	for _, z := range zones {
		if z.Map != GuildStorageMap {
			continue
		}
		for _, p := range z.Players {
			if strings.HasPrefix(p.Name, "storage:") {
				for _, it := range p.Items {
					out[it.Item] += it.Amount
				}
			}
		}
	}
	if _, twice := TotalsOf(zones); len(twice) > 0 {
		t.Errorf("serials held twice: %v", twice)
	}
	return out
}

// TestStoragePutAndTake: Welcome holds the storage limits with their
// ranges. The Kafra opens the account's storage, of storage_slots. A
// player puts Jellopy in and takes some out, and a script counts and
// takes what it holds. The storage shows the same items when it opens
// again, and the audit counts them.
func TestStoragePutAndTake(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	l := alice.WorldLimits()
	if l.InventorySlots != 100 || l.StorageSlots != 600 || l.CartSlots != 100 || len(l.Ranges) != 11 {
		t.Errorf("limits %+v", l)
	}
	for name, want := range map[string]LimitRange{"inventory_slots": {Base: 100, Max: 200}, "storage_slots": {Base: 600, Max: 600}, "cart_slots": {Base: 100, Max: 100},
		"guild_members": {Base: 16, Max: 76}, "guild_storage": {Base: 600, Max: 600}, "party_members": {Base: 12, Max: 12}} {
		if got := l.Ranges[name]; got != want {
			t.Errorf("limit %s: %+v, want %+v", name, got, want)
		}
	}

	open := alice.OpenKafra("Storage")
	if open.GetSize() != 600 || open.GetName() != "" || open.GetDisplayName() == "" || len(open.GetItems()) != 0 || open.GetPages() != 0 {
		t.Errorf("the storage %v", open)
	}
	if o := alice.TryStorePut(Jellopy, 8); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_UNSPECIFIED {
		t.Fatalf("storing 8 Jellopy: %s", o)
	}
	alice.StoreTake(Jellopy, 3)
	if alice.Count(Jellopy) != 15 || alice.Game().Storage.Count(Jellopy) != 5 {
		t.Errorf("Alice holds %d Jellopy and sees %d stored", alice.Count(Jellopy), alice.Game().Storage.Count(Jellopy))
	}
	// A script cannot read a storage while it is open, as rAthena's -1.
	if got := alice.Command("!count Jellopy storage"); got != "nil" {
		t.Errorf("counting the open storage: %s", got)
	}
	alice.CloseStorage()
	if got := alice.Command("!count Jellopy storage"); got != "5" {
		t.Errorf("counting the storage: %s", got)
	}
	alice.MustCommand("!take Jellopy 1 storage")
	if got := alice.Command("!take Jellopy 9 storage"); got != "false" {
		t.Errorf("taking more than the storage holds: %s", got)
	}
	if got := auditStorage(t, srv); got[Jellopy] != 4 {
		t.Errorf("the audit's storages hold %v", got)
	}

	again := alice.OpenStorage("")
	if items := again.GetItems(); len(items) != 1 || items[0].GetItemId() != Jellopy || items[0].GetAmount() != 4 {
		t.Errorf("the storage opens again with %v", items)
	}
	// One storage is open at a time.
	if got := alice.Command("!storage Vault"); got != "false" {
		t.Errorf("opening the Vault with the storage open: %s", got)
	}
	srv.CheckLogs()
}

// TestExtraStorage: the Kafra opens the fixture's Vault, an extra
// storage of data/storages.yml with 50 slots. Its items stay apart from
// the account's storage. A script gives items into it.
func TestExtraStorage(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	open := alice.OpenKafra("Vault")
	if open.GetSize() != 50 || open.GetName() != VaultName || open.GetDisplayName() != "Vault" {
		t.Errorf("the Vault %v", open)
	}
	alice.StorePut(Jellopy, 4)
	alice.CloseStorage()
	alice.MustCommand("!give Jellopy 2 to=Vault")
	if got := alice.Command("!count Jellopy Vault"); got != "6" {
		t.Errorf("counting the Vault: %s", got)
	}
	if got := alice.Command("!count Jellopy storage"); got != "0" {
		t.Errorf("counting the account's storage: %s", got)
	}
	if items := alice.OpenStorage("Vault").GetItems(); len(items) != 1 || items[0].GetAmount() != 6 {
		t.Errorf("the Vault opens again with %v", items)
	}
	alice.CloseStorage()
	if items := alice.OpenStorage("").GetItems(); len(items) != 0 {
		t.Errorf("the account's storage holds %v", items)
	}
	if got := auditStorage(t, srv); got[Jellopy] != 6 {
		t.Errorf("the audit's storages hold %v", got)
	}
	srv.CheckLogs()
}

// TestStorageInTheDatabase: with PostgreSQL, the account's storage, the
// Vault and the cart come back after the player logs in again and after
// a restart, and their rows and item log lines are saved.
func TestStorageInTheDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true, StartJob: MerchantJob})
	alice := srv.Join(t, "Alice")
	alice.GetCart()
	alice.OpenKafra("Storage")
	alice.StorePut(Jellopy, 5)
	alice.CloseStorage()
	alice.OpenKafra("Vault")
	alice.StorePut(Jellopy, 3)
	alice.CloseStorage()
	if o := alice.TryCartPut(Jellopy, 2); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_UNSPECIFIED {
		t.Fatalf("putting Jellopy in the cart: %s", o)
	}
	check := func(when string, p *Player) {
		t.Helper()
		if c := p.Game().Cart; c.Count(Jellopy) != 2 {
			t.Errorf("%s: the cart holds %+v", when, c)
		}
		if items := p.OpenStorage("").GetItems(); len(items) != 1 || items[0].GetAmount() != 5 {
			t.Errorf("%s: the storage holds %v", when, items)
		}
		p.CloseStorage()
		if items := p.OpenStorage("Vault").GetItems(); len(items) != 1 || items[0].GetAmount() != 3 {
			t.Errorf("%s: the Vault holds %v", when, items)
		}
		p.CloseStorage()
		if p.Count(Jellopy) != 10 {
			t.Errorf("%s: Alice holds %d Jellopy", when, p.Count(Jellopy))
		}
	}
	_ = alice.Close()
	alice = srv.Join(t, "Alice")
	alice.Until("the cart comes back", func(_ View, g Game) bool { return g.Cart != nil })
	check("logged in again", alice)
	_ = alice.Close()
	srv.Restart()
	alice = srv.Join(t, "Alice")
	alice.Until("the cart comes back", func(_ View, g Game) bool { return g.Cart != nil })
	check("after a restart", alice)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := srv.DB.Query(ctx, `SELECT holder, storage, sum(amount)::int FROM storage_items WHERE item_id = $1 GROUP BY holder, storage`, Jellopy)
	if err != nil {
		t.Fatal(err)
	}
	saved := map[string]int{}
	for rows.Next() {
		var holder string
		var storage, n int
		if err := rows.Scan(&holder, &storage, &n); err != nil {
			t.Fatal(err)
		}
		if storage != 0 {
			holder += ":extra"
		}
		saved[holder] = n
	}
	rows.Close()
	if saved["account"] != 5 || saved["character:extra"] != 3 || saved["cart"] != 2 || len(saved) != 3 {
		t.Errorf("saved storage rows %v", saved)
	}
	var stored, carted int
	if err := srv.DB.QueryRow(ctx, `SELECT count(*) FILTER (WHERE action = 'storage'), count(*) FILTER (WHERE action = 'cart') FROM item_log`).Scan(&stored, &carted); err != nil {
		t.Fatal(err)
	}
	if stored != 2 || carted != 1 {
		t.Errorf("item log: %d storage lines and %d cart lines", stored, carted)
	}
	srv.CheckLogs()
}

// TestCart: a Merchant learns Pushcart and gets a cart from a script. It
// puts items in and takes them out, moves them between the cart and the
// storage, and the cart refuses what passes max_cart_weight. A player
// without a cart gets STORAGE_OUTCOME_NO_CART.
func TestCart(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartJob: MerchantJob})
	alice := srv.Join(t, "Alice")
	if got := alice.Command("!cart"); got != "false" {
		t.Errorf("a cart without Pushcart: %s", got)
	}
	if o := alice.TryCartPut(Jellopy, 1); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_NO_CART {
		t.Errorf("putting into a cart Alice does not have: %s", o)
	}
	cart := alice.GetCart()
	if cart.GetSize() != 100 || cart.GetMaxWeight() != CartMaxWeight || cart.GetWeight() != 0 || len(cart.GetItems()) != 0 {
		t.Errorf("the cart %v", cart)
	}
	if o := alice.TryCartPut(Jellopy, 6); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_UNSPECIFIED {
		t.Fatalf("putting 6 Jellopy in the cart: %s", o)
	}
	perJellopy := alice.Game().Cart.Weight / 6
	if perJellopy <= 0 {
		t.Errorf("the cart weighs %d with 6 Jellopy", alice.Game().Cart.Weight)
	}
	if o := alice.TryCartTake(Jellopy, 2); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_UNSPECIFIED {
		t.Fatalf("taking 2 Jellopy from the cart: %s", o)
	}
	if c := alice.Game().Cart; c.Count(Jellopy) != 4 || c.Weight != 4*perJellopy || alice.Count(Jellopy) != 16 {
		t.Errorf("the cart holds %+v, Alice %d Jellopy", c, alice.Count(Jellopy))
	}
	if got := alice.Command("!count Jellopy cart"); got != "4" {
		t.Errorf("counting the cart: %s", got)
	}

	// The cart and the open storage.
	alice.OpenKafra("Storage")
	if o := alice.TryCartToStorage(Jellopy, 3); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_UNSPECIFIED {
		t.Fatalf("moving 3 Jellopy from the cart to the storage: %s", o)
	}
	if o := alice.TryStorageToCart(Jellopy, 1); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_UNSPECIFIED {
		t.Fatalf("moving 1 Jellopy from the storage to the cart: %s", o)
	}
	g := alice.Game()
	if g.Cart.Count(Jellopy) != 2 || g.Storage.Count(Jellopy) != 2 || g.Count(Jellopy) != 16 {
		t.Errorf("cart %d, storage %d, inventory %d Jellopy", g.Cart.Count(Jellopy), g.Storage.Count(Jellopy), g.Count(Jellopy))
	}
	alice.CloseStorage()

	// max_cart_weight: 3 Anvils of 2,500 fit beside the Jellopy, a 4th
	// does not.
	alice.MustCommand("!give e2e.fixture:Anvil 4")
	if o := alice.TryCartPut(Anvil, 3); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_UNSPECIFIED {
		t.Fatalf("putting 3 Anvils in the cart: %s", o)
	}
	if o := alice.TryCartPut(Anvil, 1); o != wathenav1.StorageOutcome_STORAGE_OUTCOME_CART_OVERWEIGHT {
		t.Errorf("a 4th Anvil: %s", o)
	}
	if c := alice.Game().Cart; c.Count(Anvil) != 3 || c.Weight != 7500+c.Count(Jellopy)*perJellopy || alice.Count(Anvil) != 1 {
		t.Errorf("the cart holds %+v, Alice %d Anvils", c, alice.Count(Anvil))
	}

	// The cart comes back at login, with what it holds.
	_ = alice.Close()
	alice = srv.Join(t, "Alice")
	alice.Until("the cart at login", func(_ View, g Game) bool { return g.Cart != nil && g.Cart.Complete() })
	if c := alice.Game().Cart; c.Count(Anvil) != 3 || c.MaxWeight != CartMaxWeight {
		t.Errorf("the cart at login %+v", c)
	}
	if got := auditStorage(t, srv); got[Anvil] != 3 {
		t.Errorf("the audit's storages and carts hold %v", got)
	}
	srv.CheckLogs()
}

// TestStoragePages: a script fills a storage with 250 Test Charms, which
// do not stack. The storage's list comes in pages after StorageOpen, each
// in its own frame, every frame under the 64 KiB limit, and every charm
// is listed.
func TestStoragePages(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{})
	alice := srv.Join(t, "Alice")
	alice.MustCommand("!give e2e.fixture:Charm 250 to=storage")
	if got := alice.Command("!count e2e.fixture:Charm storage"); got != "250" {
		t.Fatalf("the storage holds %s charms", got)
	}
	open := alice.OpenStorage("")
	if open.GetPages() < 2 || len(open.GetItems()) == 0 || len(open.GetItems()) > 100 {
		t.Errorf("StorageOpen holds %d slots and says %d pages follow", len(open.GetItems()), open.GetPages())
	}
	st := alice.Game().Storage
	if len(st.Items) != 250 || st.Count(Charm) != 250 || st.PagesSeen != int(open.GetPages()) {
		t.Errorf("the storage lists %d slots, %d charms, %d of %d pages", len(st.Items), st.Count(Charm), st.PagesSeen, open.GetPages())
	}
	pages := 0
	events, _ := alice.Events(0)
	for _, e := range events {
		if p := e.GetStorageItems(); p != nil {
			pages++
			if int(p.GetPage()) != pages || len(p.GetItems()) > 100 {
				t.Errorf("page %d holds %d slots, in place %d", p.GetPage(), len(p.GetItems()), pages)
			}
		}
	}
	if pages != int(open.GetPages()) {
		t.Errorf("%d pages came, StorageOpen said %d", pages, open.GetPages())
	}
	if n := alice.Counts().MaxMessage; n > schema.MaxMessageBytes || n == 0 {
		t.Errorf("the largest message is %d bytes, the limit %d", n, schema.MaxMessageBytes)
	}
	// Moves work once the pages came.
	alice.StoreTake(Charm, 1)
	if got := alice.Game().Storage.Count(Charm); got != 249 {
		t.Errorf("the storage shows %d charms after a take", got)
	}
	srv.CheckLogs()
}
