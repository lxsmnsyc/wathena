package store_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/store"
)

func TestModIDs(t *testing.T) {
	_, game := migrated(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s := store.NewModIDs(game)

	// A first start gives two items IDs from the start of the range.
	ids, err := mod.LoadIDs(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	a, err := ids.ID(ctx, mod.ItemIDs, "example.a:Hat", "example.a")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ids.ID(ctx, mod.ItemIDs, "example.b:Cake", "example.b")
	if a != 2_000_000 || b != 2_000_001 || len(ids.Assigned) != 2 {
		t.Fatalf("ids %d %d, assigned %v", a, b, ids.Assigned)
	}

	// A restart without example.a keeps example.b's ID, and the next new
	// item never gets example.a's.
	ids, err = mod.LoadIDs(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	b2, _ := ids.ID(ctx, mod.ItemIDs, "example.b:Cake", "example.b")
	c, _ := ids.ID(ctx, mod.ItemIDs, "example.c:Pie", "example.c")
	if b2 != b || c != 2_000_002 || len(ids.Assigned) != 1 {
		t.Fatalf("after a restart: %d %d, assigned %v", b2, c, ids.Assigned)
	}
	// example.a returns with its old ID, and kinds have their own range.
	a2, _ := ids.ID(ctx, mod.ItemIDs, "example.a:Hat", "example.a")
	m, _ := ids.ID(ctx, mod.MonsterIDs, "example.a:Hat", "example.a")
	if a2 != a || m != 100_000 {
		t.Fatalf("returning item %d, monster %d", a2, m)
	}

	// Servers that assign at the same moment never share an ID.
	var wg sync.WaitGroup
	got := make([]uint32, 20)
	for i := range got {
		wg.Go(func() {
			id, err := s.AssignModID(ctx, "skill", "example.d:Skill"+string(rune('A'+i)), "example.d", 20_000)
			if err != nil {
				t.Error(err)
			}
			got[i] = id
		})
	}
	wg.Wait()
	seen := map[uint32]bool{}
	for _, id := range got {
		if seen[id] || id < 20_000 || id >= 20_020 {
			t.Fatalf("skill IDs %v", got)
		}
		seen[id] = true
	}

	// The game role cannot change or delete an ID.
	for _, sql := range []string{"UPDATE mod_ids SET id = 5", "DELETE FROM mod_ids"} {
		if _, err := game.Exec(ctx, sql); err == nil {
			t.Errorf("the game role ran %q", sql)
		}
	}
}
