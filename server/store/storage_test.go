package store_test

import (
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// TestStorageCommits writes a whole stack from Alice's inventory into her
// account's storage, then from the storage into Bob's inventory, then
// Bob's whole stack to Alice's cart, with their characters, and reads the
// storages back. Each serial is in one place after each commit.
func TestStorageCommits(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{RetryDelay: time.Second})
	ids := make([]int64, 2)
	chars := make([]*character.Character, 2)
	sessions := make([]*store.Session, 2)
	for i, name := range []string{"Alice", "Bob"} {
		c := newCharacter(name)
		if i == 1 {
			c.Items = nil
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
	account := chars[0].AccountID
	reg := storage.NewRegistry(s)
	keys := storage.Keys(&chars[0].Info, []*storage.Record{{ID: 10000, Name: "example.bank:vault", Slots: 50, Account: false}})
	sts, err := reg.Acquire(ctx, keys)
	if err != nil {
		t.Fatal(err)
	}
	main, cart := sts[0], sts[1]
	if k := main.Key(); k.Holder != storage.HolderAccount || k.ID != account {
		t.Fatalf("main key %+v", k)
	}
	stack := chars[0].Items[0]
	alice := copyOf(chars[0])
	alice.Items = alice.Items[1:]
	main.Lock()
	b := main.Box()
	b.Slots = []item.Slot{{ID: stack.ID, Amount: stack.Amount, Serial: stack.Serial}}
	base, cur := main.Commit()
	main.Unlock()
	line := transfer.ItemLog{Action: transfer.ActionStorage, From: transfer.Inventory(ids[0]), To: main.Key().Owner(), Serial: stack.Serial,
		ItemID: stack.ID, Amount: stack.Amount}
	s.CommitWith([]store.Entry{{Session: sessions[0], Char: alice}}, []store.StorageWrite{{Key: main.Key(), Base: base, Cur: cur}},
		transfer.Logs{Items: []transfer.ItemLog{line}})
	// The storage leaves memory and comes back: the load waits for the
	// commit.
	reg.Release(main)
	again, err := reg.Acquire(ctx, keys[:1])
	if err != nil {
		t.Fatal(err)
	}
	if again[0] == main {
		t.Fatal("the storage stayed in memory")
	}
	main = again[0]
	main.Lock()
	got := main.Slots()
	main.Unlock()
	if len(got) != 1 || got[0].Serial != stack.Serial || got[0].Amount != stack.Amount {
		t.Fatalf("storage %+v", got)
	}
	// Storage to Bob, whole stack, and Bob's stack to Alice's cart in the
	// next commit: the serial passes through two tables and back.
	bob := copyOf(chars[1])
	bob.Items = []character.Item{{Slot: 0, ID: stack.ID, Amount: stack.Amount, Serial: stack.Serial}}
	main.Lock()
	main.Box().Slots = nil
	base, cur = main.Commit()
	main.Unlock()
	s.CommitWith([]store.Entry{{Session: sessions[1], Char: bob}}, []store.StorageWrite{{Key: main.Key(), Base: base, Cur: cur}}, transfer.Logs{})
	bob2 := copyOf(bob)
	bob2.Items = nil
	cart.Lock()
	cart.Box().Slots = []item.Slot{{ID: stack.ID, Amount: stack.Amount, Serial: stack.Serial, Extra: item.Extra{Bound: item.BoundAccount}}}
	base, cur = cart.Commit()
	cart.Unlock()
	s.CommitWith([]store.Entry{{Session: sessions[1], Char: bob2}}, []store.StorageWrite{{Key: cart.Key(), Base: base, Cur: cur}}, transfer.Logs{})
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	var where []string
	rows, err := admin.Query(ctx, `SELECT 'inventory' FROM inventory WHERE serial = $1
		UNION ALL SELECT holder FROM storage_items WHERE serial = $1`, stack.Serial[:])
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			t.Fatal(err)
		}
		where = append(where, w)
	}
	if len(where) != 1 || where[0] != "cart" {
		t.Errorf("the serial is in %v", where)
	}
	items, err := s.LoadStorage(ctx, cart.Key())
	if err != nil || len(items) != 1 || items[0].Bound != item.BoundAccount {
		t.Errorf("cart %+v %v", items, err)
	}
	var storageID int
	if err := admin.QueryRow(ctx, `SELECT to_storage FROM item_log WHERE action = 'storage'`).Scan(&storageID); err != nil || storageID != 0 {
		t.Errorf("log storage %d %v", storageID, err)
	}
}

// TestDevCharacterSkills checks that a --dev character keeps its start
// skills and block list, as --start-job gives Basic Skill.
func TestDevCharacterSkills(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	c := newCharacter("Starter")
	c.Skills, c.Blocks = map[int]int{1: 9, 36: 5}, []string{"Pest"}
	id, err := s.DevCharacter(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := s.Load(ctx, id)
	if err != nil || got.Skills[1] != 9 || got.Skills[36] != 5 || len(got.Blocks) != 1 {
		t.Errorf("loaded %v %v %v", got.Skills, got.Blocks, err)
	}
}

// TestStorageSizes saves the sizes player:expand sets (review decision
// 32): an account's storage and a cart. Each loads back when the storage
// comes into memory again, and a size of 0 deletes the row.
func TestStorageSizes(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{RetryDelay: time.Second})
	id, err := s.DevCharacter(ctx, newCharacter("Alice"))
	if err != nil {
		t.Fatal(err)
	}
	c, _, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	reg := storage.NewRegistry(s)
	keys := storage.Keys(&c.Info, nil)
	acquire := func() []*storage.Storage {
		t.Helper()
		sts, err := reg.Acquire(ctx, keys)
		if err != nil {
			t.Fatal(err)
		}
		return sts
	}
	sizes := func(sts []*storage.Storage) [2]int {
		var out [2]int
		for i, st := range sts {
			st.Lock()
			out[i] = st.Size()
			st.Unlock()
		}
		return out
	}
	commit := func(sts []*storage.Storage, account, cart int) {
		t.Helper()
		var ws []store.StorageWrite
		for i, n := range []int{account, cart} {
			st := sts[i]
			st.Lock()
			st.SetSize(n)
			base, cur := st.Commit()
			sb, sc := st.CommitSize()
			st.Unlock()
			ws = append(ws, store.StorageWrite{Key: st.Key(), Base: base, Cur: cur, SizeBase: sb, Size: sc})
		}
		s.CommitWith(nil, ws, transfer.Logs{})
		for _, st := range sts {
			reg.Release(st)
		}
	}
	sts := acquire()
	if got := sizes(sts); got != [2]int{0, 0} {
		t.Fatalf("new sizes %v", got)
	}
	commit(sts, 650, 120)
	sts = acquire()
	if got := sizes(sts); got != [2]int{650, 120} {
		t.Fatalf("saved sizes %v", got)
	}
	commit(sts, 0, 130)
	sts = acquire()
	if got := sizes(sts); got != [2]int{0, 130} {
		t.Fatalf("sizes after lowering %v", got)
	}
	var rows int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM storage_sizes WHERE holder = 'account'`).Scan(&rows); err != nil || rows != 0 {
		t.Errorf("account storage size rows %d, %v", rows, err)
	}
}
