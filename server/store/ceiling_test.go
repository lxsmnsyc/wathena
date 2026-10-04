package store_test

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// TestCeilingSaves measures save and load times at rAthena's limits and
// at the engine's ceilings (decision 0023): a character with a full
// inventory and block list, a full storage and cart, and a party and a
// guild at their member limits. It runs only with
// WATHENA_CEILING_BENCH=1 and a test database, and logs a table:
//
//	WATHENA_CEILING_BENCH=1 go test ./server/store -run TestCeilingSaves -v
func TestCeilingSaves(t *testing.T) {
	if os.Getenv("WATHENA_CEILING_BENCH") != "1" {
		t.Skip("set WATHENA_CEILING_BENCH=1 to measure the ceilings")
	}
	_, game := migrated(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	s := newStore(t, game, store.Options{RetryDelay: time.Second})
	const rounds = 20
	gearItem := func(slot int) character.Item {
		return character.Item{Slot: slot, ID: 1101, Amount: 1, Serial: item.NewSerial(), Refine: 7, Cards: []uint32{4001, 4002, 4003, 4004},
			Extra: item.Extra{Options: []item.Option{{Name: "var_maxhp", Value: 500}}}}
	}
	report := func(what string, d time.Duration) { t.Logf("%-44s %10.2f ms", what, float64(d.Microseconds())/1000) }

	// Inventories and block lists: a full save when the character is
	// made, and a save that changes one slot, as most saves do.
	for i, n := range []int{200, 1000} {
		c := newCharacter(fmt.Sprintf("Inv%d", n))
		c.Items, c.InventorySize = nil, n
		for slot := range n {
			c.Items = append(c.Items, gearItem(slot))
		}
		for b := range n {
			c.Blocks = append(c.Blocks, fmt.Sprintf("Blocked%04d", b))
		}
		start := time.Now()
		id, err := s.DevCharacter(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		report(fmt.Sprintf("character, %d items and %d blocks, first save", n, n), time.Since(start))
		start = time.Now()
		got, sess, err := s.Load(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		report(fmt.Sprintf("character, %d items, load", n), time.Since(start))
		start = time.Now()
		for r := range rounds {
			next := copyOf(got)
			next.Items[r%n].Amount = 1
			next.Items[r%n].Refine = r % 10
			next.Zeny = int64(r + i)
			sess.Save(next, false)
			if err := sess.Flush(ctx); err != nil {
				t.Fatal(err)
			}
			got = next
		}
		report(fmt.Sprintf("character, %d items, one-slot save", n), time.Since(start)/rounds)
	}

	// Storages and carts, through commits as the zones write them.
	reg := storage.NewRegistry(s)
	owner := newCharacter("Owner")
	id, err := s.DevCharacter(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	loaded, _, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		key   storage.Key
		slots int
	}{
		{"storage", storage.Key{Holder: storage.HolderAccount, ID: loaded.AccountID}, 600},
		{"storage", storage.Key{Holder: storage.HolderAccount, ID: loaded.AccountID, Storage: 1}, 10000},
		{"cart", storage.Key{Holder: storage.HolderCart, ID: id}, 100},
		{"cart", storage.Key{Holder: storage.HolderCart, ID: id, Storage: 0}, 1000},
	} {
		sts, err := reg.Acquire(ctx, []storage.Key{c.key})
		if err != nil {
			t.Fatal(err)
		}
		st := sts[0]
		st.Lock()
		box := st.Box()
		box.Slots = nil
		for slot := range c.slots {
			it := gearItem(slot)
			box.Slots = append(box.Slots, item.Slot{ID: it.ID, Amount: 1, Serial: it.Serial, Refine: it.Refine, Cards: it.Cards, Extra: it.Extra})
		}
		base, cur := st.Commit()
		st.Unlock()
		start := time.Now()
		s.CommitWith(nil, []store.StorageWrite{{Key: c.key, Base: base, Cur: cur}}, transfer.Logs{})
		if err := s.Drain(ctx); err != nil {
			t.Fatal(err)
		}
		report(fmt.Sprintf("%s of %d, first commit", c.name, c.slots), time.Since(start))
		start = time.Now()
		for r := range rounds {
			st.Lock()
			box.Slots[r%c.slots].Refine = r % 10
			base, cur := st.Commit()
			st.Unlock()
			s.CommitWith(nil, []store.StorageWrite{{Key: c.key, Base: base, Cur: cur}}, transfer.Logs{})
			if err := s.Drain(ctx); err != nil {
				t.Fatal(err)
			}
		}
		report(fmt.Sprintf("%s of %d, one-slot commit", c.name, c.slots), time.Since(start)/rounds)
		start = time.Now()
		if items, err := s.LoadStorage(ctx, c.key); err != nil || len(items) != c.slots {
			t.Fatalf("load: %d %v", len(items), err)
		}
		report(fmt.Sprintf("%s of %d, load", c.name, c.slots), time.Since(start))
		reg.Release(st)
	}

	// A party and a guild at their member limits: made with every member,
	// then every saved party and guild loaded, as at start.
	soc := store.NewSocialStore(game, slog.New(slog.DiscardHandler))
	defer func() { _ = soc.Close(ctx) }()
	for _, n := range []int{12, 256} {
		party := partyRecord(t, ctx, s, n)
		start := time.Now()
		soc.CreateParty(party)
		if err := soc.Flush(ctx); err != nil {
			t.Fatal(err)
		}
		report(fmt.Sprintf("party of %d, made", n), time.Since(start))
	}
	start := time.Now()
	if _, err := store.LoadParties(ctx, game); err != nil {
		t.Fatal(err)
	}
	report("parties of 12 and 256, load", time.Since(start))
	for i, n := range []int{76, 512} {
		g := guildRecord(t, ctx, s, n, int64(i+1))
		start := time.Now()
		soc.CreateGuild(g)
		if err := soc.Flush(ctx); err != nil {
			t.Fatal(err)
		}
		report(fmt.Sprintf("guild of %d, made", n), time.Since(start))
	}
	start = time.Now()
	if _, _, err := store.LoadGuilds(ctx, game); err != nil {
		t.Fatal(err)
	}
	report("guilds of 76 and 512, load", time.Since(start))
}

// members makes n characters for a party or a guild.
func members(t *testing.T, ctx context.Context, s *store.Store, prefix string, n int) []int64 {
	t.Helper()
	var ids []int64
	for i := range n {
		c := newCharacter(fmt.Sprintf("%s%03d", prefix, i))
		c.Items = nil
		id, err := s.DevCharacter(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func partyRecord(t *testing.T, ctx context.Context, s *store.Store, n int) social.PartyRecord {
	p := social.PartyRecord{ID: int64(n), Name: fmt.Sprintf("Party%d", n)}
	for i, id := range members(t, ctx, s, fmt.Sprintf("P%d_", n), n) {
		p.Members = append(p.Members, social.MemberRecord{Slot: i, CharID: id, Name: fmt.Sprintf("P%d_%03d", n, i), Level: 50})
	}
	p.Leader = p.Members[0].Name
	return p
}

func guildRecord(t *testing.T, ctx context.Context, s *store.Store, n int, id int64) social.GuildRecord {
	g := social.GuildRecord{ID: id, Name: fmt.Sprintf("Guild%d", n), Level: 1, Positions: social.DefaultPositions(20)}
	for i, cid := range members(t, ctx, s, fmt.Sprintf("G%d_", n), n) {
		g.Members = append(g.Members, social.GuildMemberRecord{Slot: i, CharID: cid, Name: fmt.Sprintf("G%d_%03d", n, i), Position: 19, Level: 50})
	}
	g.Members[0].Position = 0
	return g
}
