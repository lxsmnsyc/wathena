package e2e

import (
	"context"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// TestTransfersInTheDatabase plays the shop and loot scenarios on a
// server with PostgreSQL, and checks the item and zeny logs and the
// saved character. The logs are written while the player plays, with
// the character, not only when it leaves.
func TestTransfersInTheDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true})
	p := srv.Join(t, "Ledger")
	shopping(t, p)
	dummy := p.Find(wathenav1.EntityKind_ENTITY_KIND_MONSTER, "Dummy")
	p.Kill(dummy)
	p.PickUp(p.FindItem(Charm))
	p.Use(RedPotion)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	// Every transfer is in the log before the player leaves.
	want := map[string]int{"give": 1, "buy": 1, "sell": 1, "take": 1, "pick_up": 1, "use": 1}
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := map[string]int{}
		rows, err := srv.DB.Query(ctx, `SELECT action, count(*) FROM item_log GROUP BY action`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var a string
			var n int
			if err := rows.Scan(&a, &n); err != nil {
				t.Fatal(err)
			}
			got[a] = n
		}
		rows.Close()
		if equalCounts(got, want) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("item log lines by action: %v, want %v", got, want)
		}
		time.Sleep(100 * time.Millisecond)
	}
	var fromKind string
	var fromID, amount int64
	if err := srv.DB.QueryRow(ctx, `SELECT from_kind, from_id, (rolls->>'luck')::bigint FROM item_log WHERE action = 'pick_up'`).Scan(&fromKind, &fromID, &amount); err != nil {
		t.Fatal(err)
	}
	if fromKind != "monster" || fromID < 100_000 || amount != 3 {
		t.Errorf("the pick-up came from %s %d with luck %d", fromKind, fromID, amount)
	}
	zeny := map[string]int64{}
	rows, err := srv.DB.Query(ctx, `SELECT action, sum(amount)::bigint FROM zeny_log GROUP BY action`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var a string
		var n int64
		if err := rows.Scan(&a, &n); err != nil {
			t.Fatal(err)
		}
		zeny[a] = n
	}
	rows.Close()
	final := p.Stat(wathenav1.StatId_STAT_ID_ZENY)
	if zeny["give"] != 1000 || zeny["buy"] != 100 || zeny["give"]-zeny["buy"]+zeny["sell"] != final {
		t.Errorf("zeny log %v, the player holds %d", zeny, final)
	}

	// The saved character is what the player saw.
	view := p.Game()
	srv.Stop()
	var saved int64
	if err := srv.DB.QueryRow(ctx, `SELECT zeny FROM characters WHERE name = 'Ledger'`).Scan(&saved); err != nil || saved != final {
		t.Errorf("saved zeny %d, %v; the player saw %d", saved, err, final)
	}
	for _, id := range []uint32{RedPotion, Jellopy, Charm} {
		var n int
		if err := srv.DB.QueryRow(ctx, `SELECT coalesce(sum(i.amount), 0) FROM inventory i JOIN characters c ON c.id = i.character_id
			WHERE c.name = 'Ledger' AND i.item_id = $1`, id).Scan(&n); err != nil || n != view.Count(id) {
			t.Errorf("item %d: saved %d, %v; the player saw %d", id, n, err, view.Count(id))
		}
	}
	srv.CheckLogs()
}

func equalCounts(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
