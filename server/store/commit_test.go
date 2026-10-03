package store_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// copyOf copies a loaded character, so a test can change it.
func copyOf(c *character.Character) *character.Character {
	n := *c
	n.Items = slices.Clone(c.Items)
	return &n
}

// TestCommitMovesBetweenCharacters writes one transfer that moves part of
// a stack from Alice to Bob and zeny back: both characters and the log
// lines in one transaction.
func TestCommitMovesBetweenCharacters(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	ids := make([]int64, 2)
	chars := make([]*character.Character, 2)
	sessions := make([]*store.Session, 2)
	for i, name := range []string{"Alice", "Bob"} {
		c := newCharacter(name)
		if i == 1 {
			c.Items, c.Zeny = nil, 500
		}
		id, err := s.DevCharacter(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id
		if chars[i], sessions[i], err = s.Load(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	// The zone's side of the transfer, on copies of what was loaded.
	alice := &transfer.InventoryBox{Inv: item.NewInventory(), Items: nil, MaxWeight: 1 << 30, Who: transfer.Inventory(ids[0])}
	for _, it := range chars[0].Items {
		alice.Inv.Slots[it.Slot] = item.Slot{ID: it.ID, Amount: it.Amount, Serial: it.Serial, Equip: it.Equip, Refine: it.Refine, Cards: it.Cards}
	}
	bob := &transfer.InventoryBox{Inv: item.NewInventory(), MaxWeight: 1 << 30, Who: transfer.Inventory(ids[1])}
	aliceZeny, bobZeny := chars[0].Zeny, chars[1].Zeny
	potions := chars[0].Items[0].Serial
	tx := transfer.New(transfer.ActionMove).Move(alice, 0, potions, 2, bob).
		MoveZeny(&transfer.ZenyWallet{Who: bob.Who, Z: &bobZeny}, 300, &transfer.ZenyWallet{Who: alice.Who, Z: &aliceZeny})
	logs, err := tx.Apply(nil)
	if err != nil {
		t.Fatal(err)
	}
	next := []*character.Character{copyOf(chars[0]), copyOf(chars[1])}
	next[0].Items[0].Amount, next[0].Zeny = 3, aliceZeny
	split := bob.Inv.Slots[0].Serial
	next[1].Items = []character.Item{{Slot: 0, ID: 501, Amount: 2, Serial: split}}
	next[1].Zeny = bobZeny
	s.Commit([]store.Entry{{Session: sessions[0], Char: next[0]}, {Session: sessions[1], Char: next[1]}}, logs)
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		got, _, err := s.Load(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		want := *next[i]
		want.ID, want.AccountID = id, got.AccountID
		if !got.Equal(&want) {
			t.Errorf("character %d: %+v\nwant %+v", i, got, &want)
		}
	}
	var serial, source pgtype.UUID
	var from, to string
	var amount int
	err = admin.QueryRow(ctx, `SELECT serial, source, from_kind || ':' || from_id, to_kind || ':' || to_id, amount FROM item_log`).
		Scan(&serial, &source, &from, &to, &amount)
	if err != nil {
		t.Fatal(err)
	}
	if serial.Bytes != split || source.Bytes != potions || from != fmt.Sprint("inventory:", ids[0]) || to != fmt.Sprint("inventory:", ids[1]) || amount != 2 {
		t.Errorf("item log: %v from %v, %s to %s, %d", serial, source, from, to, amount)
	}
	var zeny int64
	if err := admin.QueryRow(ctx, `SELECT amount FROM zeny_log WHERE from_id = $1 AND to_id = $2`, ids[1], ids[0]).Scan(&zeny); err != nil || zeny != 300 {
		t.Errorf("zeny log: %d, %v", zeny, err)
	}
	// The rows are in this month's partition.
	var part string
	month := time.Now().UTC().Format("2006_01")
	if err := admin.QueryRow(ctx, `SELECT tableoid::regclass::text FROM item_log`).Scan(&part); err != nil || part != "item_log_"+month {
		t.Errorf("partition %q, %v", part, err)
	}
}

// TestCommitOrdersSaves checks that a save queued before a commit is
// dropped, and one queued after it waits for it.
func TestCommitOrdersSaves(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	id, err := s.DevCharacter(ctx, newCharacter("Carol"))
	if err != nil {
		t.Fatal(err)
	}
	c, sess, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for round := range 20 {
		older, committed, newer := copyOf(c), copyOf(c), copyOf(c)
		older.Zeny, committed.Zeny, newer.Zeny = int64(round*3+1), int64(round*3+2), int64(round*3+3)
		older.Items = older.Items[:1]
		committed.Items = committed.Items[:2]
		sess.Save(older, false)
		s.Commit([]store.Entry{{Session: sess, Char: committed}}, transfer.Logs{Zeny: []transfer.ZenyLog{{
			At: time.Now(), Action: transfer.ActionGive, From: transfer.End(transfer.KindScript), To: transfer.Inventory(id), Amount: 1,
		}}})
		sess.Save(newer, false)
		if err := sess.Flush(ctx); err != nil {
			t.Fatal(err)
		}
		if err := s.Drain(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := sess.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	sess.Save(copyOf(c), true)
	if err := sess.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Zeny != c.Zeny || len(got.Items) != len(c.Items) {
		t.Errorf("zeny %d with %d items, want %d with %d", got.Zeny, len(got.Items), c.Zeny, len(c.Items))
	}
}

// TestCommitWithStaleSession writes the rest of a commit when one of its
// characters was loaded by a newer session, and ends the old session.
func TestCommitWithStaleSession(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	a, err := s.DevCharacter(ctx, newCharacter("Dan"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.DevCharacter(ctx, newCharacter("Eve"))
	if err != nil {
		t.Fatal(err)
	}
	ca, old, err := s.Load(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	cb, sb, err := s.Load(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	staled := make(chan struct{})
	old.OnStale(func() { close(staled) })
	if _, _, err := s.Load(ctx, a); err != nil {
		t.Fatal(err)
	}
	na, nb := copyOf(ca), copyOf(cb)
	na.Zeny, nb.Zeny = 1, 2
	s.Commit([]store.Entry{{Session: old, Char: na}, {Session: sb, Char: nb}}, transfer.Logs{Zeny: []transfer.ZenyLog{{
		At: time.Now(), Action: transfer.ActionMove, From: transfer.Inventory(a), To: transfer.Inventory(b), Amount: 1,
	}}})
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-staled:
	case <-ctx.Done():
		t.Fatal("the old session was not told it is stale")
	}
	var za, zb int64
	if err := admin.QueryRow(ctx, `SELECT (SELECT zeny FROM characters WHERE id = $1), (SELECT zeny FROM characters WHERE id = $2)`, a, b).Scan(&za, &zb); err != nil {
		t.Fatal(err)
	}
	if za != ca.Zeny || zb != 2 {
		t.Errorf("zeny %d and %d, want %d and 2", za, zb, ca.Zeny)
	}
	var n int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM zeny_log`).Scan(&n); err != nil || n != 1 {
		t.Errorf("%d zeny log lines, %v", n, err)
	}
}

// TestLogPartitions checks that the store makes the partitions of this
// month and the next, and that a second call changes nothing.
func TestLogPartitions(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	newStore(t, game, store.Options{})
	now := time.Now().UTC()
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	want := []string{
		"item_log_" + first.Format("2006_01"), "item_log_" + first.AddDate(0, 1, 0).Format("2006_01"),
		"zeny_log_" + first.Format("2006_01"), "zeny_log_" + first.AddDate(0, 1, 0).Format("2006_01"),
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		var got []string
		rows, err := admin.Query(ctx, `SELECT c.relname FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
			WHERE i.inhparent IN ('item_log'::regclass, 'zeny_log'::regclass) AND c.relname NOT LIKE '%_default' ORDER BY 1`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var n string
			if err := rows.Scan(&n); err != nil {
				t.Fatal(err)
			}
			got = append(got, n)
		}
		rows.Close()
		if slices.Equal(got, want) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("partitions %v, want %v", got, want)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := game.Exec(ctx, `SELECT ensure_log_partitions(now() + interval '40 days')`); err != nil {
		t.Errorf("the game role cannot make partitions: %v", err)
	}
}
