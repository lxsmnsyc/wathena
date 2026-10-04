package storage

import (
	"context"
	"errors"
	"testing"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

type fakeLoader struct {
	loads map[Key]int
	items map[Key][]character.Item
	fail  bool
}

func (f *fakeLoader) LoadStorage(_ context.Context, k Key) ([]character.Item, error) {
	if f.fail {
		return nil, errors.New("down")
	}
	f.loads[k]++
	return f.items[k], nil
}

func TestKeys(t *testing.T) {
	recs := []*Record{{ID: 10000, Name: "example.bank:vault", Account: true}, {ID: 10001, Name: "example.bank:locker"}}
	keys := Keys(&character.Info{ID: 7, AccountID: 3, Name: "Ann"}, recs)
	want := []Key{{Holder: HolderAccount, ID: 3}, {Holder: HolderCart, ID: 7}, {Holder: HolderAccount, ID: 3, Storage: 10000},
		{Holder: HolderCharacter, ID: 7, Storage: 10001}}
	if len(keys) != len(want) {
		t.Fatalf("%v", keys)
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Errorf("key %d: %+v, want %+v", i, keys[i], want[i])
		}
	}
	// A character in memory is its own account, by name.
	mem := Keys(&character.Info{Name: "Bob"}, nil)
	if mem[0].Name != "bob" || mem[0].Saved() || mem[1].Name != "bob" {
		t.Errorf("memory keys %v", mem)
	}
	if o := keys[3].Owner(); o.Kind != transfer.KindStorage || o.ID != 7 || o.Storage != 10001 {
		t.Errorf("owner %+v", o)
	}
	if o := keys[1].Owner(); o.Kind != transfer.KindCart || o.ID != 7 {
		t.Errorf("cart owner %+v", o)
	}
}

func TestAcquireAndRelease(t *testing.T) {
	k := Key{Holder: HolderAccount, ID: 3}
	f := &fakeLoader{loads: map[Key]int{}, items: map[Key][]character.Item{k: {{Slot: 2, ID: 909, Amount: 4, Serial: item.NewSerial()}}}}
	r := NewRegistry(f)
	a, err := r.Acquire(context.Background(), []Key{k})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := r.Acquire(context.Background(), []Key{k})
	if a[0] != b[0] || f.loads[k] != 1 {
		t.Fatalf("two characters of one account: %d loads", f.loads[k])
	}
	st := a[0]
	st.Lock()
	if s := st.Box().Slot(2); s == nil || s.Amount != 4 || st.Used() != 1 {
		t.Errorf("loaded slots %v", st.Slots())
	}
	st.Unlock()
	// One opener at a time.
	if !st.Open("ann") || st.Open("bob") || !st.Open("ann") {
		t.Error("opener rules")
	}
	st.Close("bob")
	if st.Open("bob") {
		t.Error("bob closed ann's opening")
	}
	st.Close("ann")
	if !st.Open("bob") {
		t.Error("bob cannot open after ann closed")
	}
	// The storage stays while one user remains, and leaves after the
	// last.
	r.Release(a[0])
	if c, _ := r.Acquire(context.Background(), []Key{k}); c[0] != st || f.loads[k] != 1 {
		t.Error("the storage left with a user online")
	}
	r.Release(st)
	r.Release(st)
	if c, _ := r.Acquire(context.Background(), []Key{k}); c[0] == st || f.loads[k] != 2 {
		t.Error("the storage stayed after its last user")
	}
	// A failed load takes nothing.
	f.fail = true
	if _, err := r.Acquire(context.Background(), []Key{{Holder: HolderCart, ID: 9}}); err == nil {
		t.Error("no error")
	}
}

func TestMemoryRegistryKeeps(t *testing.T) {
	r := NewRegistry(nil)
	k := Key{Holder: HolderAccount, Name: "ann"}
	a, _ := r.Acquire(context.Background(), []Key{k})
	r.Release(a[0])
	b, _ := r.Acquire(context.Background(), []Key{k})
	if a[0] != b[0] {
		t.Error("a storage in memory left the registry")
	}
	if len(r.Audit()) != 1 {
		t.Errorf("audit %v", r.Audit())
	}
}

func TestPrepareCountsWeight(t *testing.T) {
	tb := item.New(&item.Data{Items: []item.Def{{ID: 909, Weight: 10, Stackable: true}}})
	st := newStorage(Key{Holder: HolderCart, ID: 1}, []character.Item{{Slot: 0, ID: 909, Amount: 5}})
	st.Lock()
	defer st.Unlock()
	b := st.Prepare(tb, 100, 8000)
	if b.Weight != 50 || b.Size != 100 || b.MaxWeight != 8000 {
		t.Errorf("box %+v", b)
	}
	// A second prepare keeps the weight the moves changed.
	b.Weight = 70
	if st.Prepare(tb, 100, 8000).Weight != 70 {
		t.Error("weight counted again")
	}
}
