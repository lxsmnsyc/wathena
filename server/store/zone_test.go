package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// testZone runs a zone with renewal's job tables and item database, and
// two Red Potions and an Apple for each new character. Cancel ctx to stop
// it.
func testZone(t *testing.T, ctx context.Context, interval time.Duration) *zone.Zone {
	t.Helper()
	z := newTestZone(t, "map/test", interval)
	go z.Run(ctx)
	return z
}

// newTestZone makes a zone as testZone does, without running it.
func newTestZone(t *testing.T, name string, interval time.Duration) *zone.Zone {
	t.Helper()
	g := &path.Grid{Width: 40, Height: 40, Cells: make([]uint8, 1600)}
	for i := range g.Cells {
		g.Cells[i] = path.Walkable | path.Shootable
	}
	z := zone.New(name, g, rng.FromKey([32]byte{9}).Stream(), nil)
	var jobs status.Data
	if err := reference.LoadFile("extracts/renewal/job_stats.json", &jobs); err != nil {
		t.Fatal(err)
	}
	tables, err := status.New(&jobs)
	if err != nil {
		t.Fatal(err)
	}
	var prog progression.Data
	if err := reference.LoadFile("extracts/renewal/progression.json", &prog); err != nil {
		t.Fatal(err)
	}
	var items item.Data
	if err := reference.LoadFile("extracts/renewal/items.json", &items); err != nil {
		t.Fatal(err)
	}
	z.Status = tables
	z.Start = status.Character{Job: 0, Male: true, Stats: status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}}
	z.Progress = progression.New(&prog, nil, progression.DefaultConfig)
	z.Items = item.New(&items)
	z.StartItems = []zone.StartItem{{ID: 501, Amount: 2}, {ID: 512, Amount: 1}}
	z.SaveInterval = interval
	return z
}

// join loads a character from the store into the zone, as the gateway
// does, making it first when it does not exist.
func join(t *testing.T, ctx context.Context, s *store.Store, z *zone.Zone, name string) (*zone.Session, *character.Character) {
	t.Helper()
	fresh := z.NewCharacter(name, zone.Look{Sex: 1, Hair: 2})
	id, err := s.DevCharacter(ctx, &fresh)
	if err != nil {
		t.Fatal(err)
	}
	c, sess, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	p, err := z.JoinCharacter(ctx, c, sess)
	if err != nil {
		t.Fatal(err)
	}
	return p, c
}

// waitFor reads a player's messages until an event matches.
func waitFor(t *testing.T, p *zone.Session, match func(*wathenav1.Event) bool) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case m, ok := <-p.Out:
			if !ok {
				t.Fatal("the zone dropped the player")
			}
			for _, e := range m.GetFrame().GetEvents() {
				if match(e) {
					return
				}
			}
		case <-deadline:
			t.Fatal("no matching event")
		}
	}
}

func TestProgressSurvivesReconnect(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	zctx, stop := context.WithCancel(ctx)
	defer stop()
	z := testZone(t, zctx, 0)

	p, first := join(t, ctx, s, z, "Erin")
	if len(first.Items) != 2 || first.StatusPoints != 48 {
		t.Fatalf("new character %+v", first)
	}
	// Raise STR by 2, use a Red Potion, and walk away from the start.
	p.RaiseStat(2, status.Str, 2)
	p.UseItem(3, 0)
	p.WalkTo(4, first.X+3, first.Y)
	waitFor(t, p, func(e *wathenav1.Event) bool { return e.HasInventoryRemove() })
	time.Sleep(time.Second)
	p.Leave()

	got, sess, err := s.Load(ctx, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stats.Str != 3 || got.StatusPoints != 44 {
		t.Errorf("STR %d with %d status points, want 3 and 44 (2 points per raise)", got.Stats.Str, got.StatusPoints)
	}
	if len(got.Items) != 2 || got.Items[0].ID != 501 || got.Items[0].Amount != 1 || got.Items[0].Serial != first.Items[0].Serial {
		t.Errorf("items %+v, want one Red Potion left with its serial", got.Items)
	}
	if got.X != first.X+3 || got.Y != first.Y {
		t.Errorf("at %d,%d, want %d,%d", got.X, got.Y, first.X+3, first.Y)
	}
	// Close the loaded session, then play again: the zone restores the
	// saved character.
	sess.Save(got, true)
	p, again := join(t, ctx, s, z, "erin")
	if again.ID != first.ID || again.Stats.Str != 3 {
		t.Fatalf("second login %+v", again)
	}
	waitFor(t, p, func(e *wathenav1.Event) bool {
		for _, st := range e.GetStats().GetStats() {
			if st.GetId() == wathenav1.StatId_STAT_ID_STR && st.GetValue() == 3 {
				return true
			}
		}
		return false
	})
}

// readStr reads a character's STR with the admin role, without starting
// a session.
func readStr(t *testing.T, admin *pgxpool.Pool, id int64) int {
	t.Helper()
	var str int
	if err := admin.QueryRow(timeout(t), "SELECT str FROM characters WHERE id = $1", id).Scan(&str); err != nil {
		t.Fatal(err)
	}
	return str
}

func TestAutosaveAndShutdownSave(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	zctx, stop := context.WithCancel(ctx)
	defer stop()
	z := testZone(t, zctx, 300*time.Millisecond)

	p, c := join(t, ctx, s, z, "Fay")
	p.RaiseStat(2, status.Str, 4)
	// The autosave writes the change while the player stays.
	deadline := time.Now().Add(5 * time.Second)
	for readStr(t, admin, c.ID) != 5 {
		if time.Now().After(deadline) {
			t.Fatal("the autosave did not write the raise")
		}
		time.Sleep(50 * time.Millisecond)
	}
	// A change right before shutdown is saved by the shutdown.
	p.RaiseStat(3, status.Agi, 1)
	p.Ack(4)
	waitFor(t, p, func(e *wathenav1.Event) bool {
		for _, st := range e.GetStats().GetStats() {
			if st.GetId() == wathenav1.StatId_STAT_ID_AGI && st.GetValue() == 2 {
				return true
			}
		}
		return false
	})
	stop()
	<-z.Stopped()
	if err := s.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.Load(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stats.Str != 5 || got.Stats.Agi != 2 {
		t.Errorf("stats %+v, want STR 5 and AGI 2", got.Stats)
	}
}

func TestStaleSessionIsDisconnected(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	zctx, stop := context.WithCancel(ctx)
	defer stop()
	z := testZone(t, zctx, 200*time.Millisecond)

	p, c := join(t, ctx, s, z, "Gil")
	// Another session loads the character, as a login on another server
	// would.
	if _, _, err := s.Load(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	// The next autosave is rejected as stale, and the zone drops the
	// player with the reason.
	p.RaiseStat(2, status.Str, 1)
	var reason wathenav1.DisconnectReason
	deadline := time.After(5 * time.Second)
	for open := true; open; {
		select {
		case m, ok := <-p.Out:
			open = ok
			if ok && m.HasDisconnect() {
				reason = m.GetDisconnect().GetReason()
			}
		case <-deadline:
			t.Fatal("the player was not disconnected")
		}
	}
	if reason != wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_IN_ELSEWHERE {
		t.Errorf("reason %v", reason)
	}
	if got := readStr(t, game, c.ID); got != 1 {
		t.Errorf("STR %d in the database, want the stale raise not saved", got)
	}
}

func TestMapChangeSavesTheNewMap(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	// Autosaves wait an hour, so only the map change saves.
	a := newTestZone(t, "map/test", time.Hour)
	b := newTestZone(t, "map/field", time.Hour)
	c0 := a.NewCharacter("Hana", zone.Look{Sex: 1})
	a.SetWarps([]zone.Warp{{Name: "out", X: c0.X + 3, Y: c0.Y, To: "map/field", ToX: 7, ToY: 9}})
	w, err := world.New("map/test", a, b)
	if err != nil {
		t.Fatal(err)
	}
	wctx, stop := context.WithCancel(ctx)
	defer stop()
	go w.Run(wctx)

	fresh := w.NewCharacter("Hana", zone.Look{Sex: 1})
	id, err := s.DevCharacter(ctx, &fresh)
	if err != nil {
		t.Fatal(err)
	}
	c, sess, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	p, err := w.JoinCharacter(ctx, c, sess)
	if err != nil {
		t.Fatal(err)
	}
	p.UseItem(2, 0)
	waitFor(t, p, func(e *wathenav1.Event) bool { return e.HasInventoryRemove() })
	p.WalkTo(3, c.X+5, c.Y)
	waitFor(t, p, func(e *wathenav1.Event) bool { return e.GetMapChange().GetMap() == "map/field" })
	// The save at the map change reaches the database while the player
	// stays, with the new map and cell and the potion used on the old map.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var m string
		var x, y, potions int
		err := admin.QueryRow(ctx, `SELECT c.map, c.x, c.y, coalesce((SELECT amount FROM inventory i WHERE i.character_id = c.id AND i.item_id = 501), 0)
			FROM characters c WHERE c.id = $1`, id).Scan(&m, &x, &y, &potions)
		if err != nil {
			t.Fatal(err)
		}
		if m == "map/field" && x == 7 && y == 9 && potions == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("saved %s %d,%d with %d potions, want map/field 7,9 with 1", m, x, y, potions)
		}
		time.Sleep(50 * time.Millisecond)
	}
	p.Leave()
	got, _, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Map != "map/field" || got.X != 7 || got.Y != 9 {
		t.Errorf("after logout at %s %d,%d", got.Map, got.X, got.Y)
	}
}
