package e2e

import (
	"context"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// TestDisconnectKeepsItems closes a player's connection in the middle
// of a drop, while another player picks up its earlier drop. The player
// logs in again and has what the server held for it, and nothing is
// lost or held twice.
func TestDisconnectKeepsItems(t *testing.T) {
	t.Parallel()
	disconnectKeepsItems(t, false)
}

// TestDisconnectKeepsItemsInTheDatabase is the same with PostgreSQL, and
// then checks the saved inventories.
func TestDisconnectKeepsItemsInTheDatabase(t *testing.T) {
	t.Parallel()
	disconnectKeepsItems(t, true)
}

func disconnectKeepsItems(t *testing.T, db bool) {
	srv := Start(t, Options{StartItems: "909:20", Database: db})
	alice := srv.Join(t, "Alice")
	bob := srv.Join(t, "Bob")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	d := DupTest{Server: srv, Bots: 2}
	start, err := d.audit(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}

	alice.Drop(Jellopy, 5)
	first := bob.FindItem(Jellopy)
	// Bob's pick-up and Alice's second drop are in flight when her
	// connection closes.
	if err := bob.Bot.PickUp(ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if err := alice.DropItem(ctx, alice.Game().SlotOf(Jellopy), 3); err != nil {
		t.Fatal(err)
	}
	_ = alice.CloseNow()
	bob.Expect("the first drop in the inventory", func(e *wathenav1.Event) bool {
		a := e.GetInventoryAdd()
		return a != nil && a.GetItemId() == Jellopy && a.GetAmount() == 5
	})

	again := srv.Join(t, "Alice")
	zones, err := d.online(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if full, ok := d.check(t, start, zones, "after Alice logged in again"); !full || !ok {
		t.FailNow()
	}
	// Bob picks up what lies on the ground: the second drop, when the
	// server took it before the connection closed.
	for {
		zones, err := srv.Audit(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if now, _ := TotalsOf(zones); now.Ground == 0 {
			break
		}
		bob.PickUp(bob.FindItem(Jellopy))
	}
	zones, err = srv.Audit(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := d.check(t, start, zones, "at the end"); !ok {
		t.FailNow()
	}
	d.checkBots(ctx, t, []*botclient.Bot{again.Bot, bob.Bot})
	t.Logf("Alice holds %d Jellopy, Bob %d", again.Count(Jellopy), bob.Count(Jellopy))
	if again.Count(Jellopy) != 12 && again.Count(Jellopy) != 15 {
		t.Errorf("Alice holds %d Jellopy, want 15, or 12 when the server took her second drop", again.Count(Jellopy))
	}
	srv.CheckLogs()
	if !db {
		return
	}
	held := map[string]int{"Alice": again.Count(Jellopy), "Bob": bob.Count(Jellopy)}
	srv.Stop()
	for name, want := range held {
		var n int
		if err := srv.DB.QueryRow(ctx, `SELECT coalesce(sum(i.amount), 0) FROM inventory i JOIN characters c ON c.id = i.character_id
			WHERE c.name = $1 AND i.item_id = $2`, name, Jellopy).Scan(&n); err != nil || n != want {
			t.Errorf("%s: saved %d Jellopy, %v; the server held %d", name, n, err, want)
		}
	}
}
