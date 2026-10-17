package world

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

func testZone(name string) *zone.Zone {
	g := &path.Grid{Width: 30, Height: 30, Cells: make([]uint8, 900)}
	for i := range g.Cells {
		g.Cells[i] = path.Walkable | path.Shootable
	}
	return zone.New(name, g, rng.FromKey([32]byte{3}).Stream(), nil)
}

func TestNew(t *testing.T) {
	if _, err := New("map/a", testZone("map/a"), testZone("map/a")); err == nil {
		t.Error("a map listed twice is accepted")
	}
	if _, err := New("map/c", testZone("map/a")); err == nil {
		t.Error("a start map that does not run is accepted")
	}
	a, b := testZone("map/a"), testZone("map/b")
	w, err := New("map/a", b, a)
	if err != nil {
		t.Fatal(err)
	}
	if w.Zone("map/b") != b || w.Zone("map/none") != nil || w.Start() != a || a.World != w || b.World != w {
		t.Error("zones are not found")
	}
	if z := w.Zones(); len(z) != 2 || z[0] != a || z[1] != b {
		t.Error("Zones is not sorted by map")
	}
}

func TestJoinGoesToTheCharactersMap(t *testing.T) {
	a, b := testZone("map/a"), testZone("map/b")
	w, err := New("map/a", a, b)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)
	c := w.NewCharacter("Mover", zone.Look{Sex: 1})
	if c.Map != "map/a" {
		t.Fatalf("a new character starts on %s", c.Map)
	}
	c.Map, c.X, c.Y = "map/b", 7, 8
	s, err := w.JoinCharacter(ctx, &c, nil)
	if err != nil || s.Map != "map/b" {
		t.Fatalf("joined %v, %v", s, err)
	}
	lost := w.NewCharacter("Lost", zone.Look{})
	lost.Map = "map/gone"
	s2, err := w.JoinCharacter(ctx, &lost, nil)
	if err != nil || s2.Map != "map/a" {
		t.Fatalf("a character on a map that does not run joined %v, %v", s2, err)
	}
	cancel()
	select {
	case <-w.Stopped():
	case <-time.After(5 * time.Second):
		t.Fatal("the world did not stop")
	}
	// Both sessions ended with the world.
	for range s.Out {
	}
	for range s2.Out {
	}
}

func TestLoadWarps(t *testing.T) {
	for _, mode := range reference.Modes {
		warps, err := LoadWarps(mode)
		if err != nil {
			t.Fatal(err)
		}
		// prontera's south gate leads to prt_fild08 in both modes.
		found := false
		for _, w := range warps["map/prontera"] {
			if w.X == 156 && w.Y == 22 {
				found = true
				if w.To != "map/prt_fild08" || w.ToX != 170 || w.ToY != 375 || w.XS != 3 || w.YS != 2 {
					t.Errorf("%s: prontera's south gate %+v", mode, w)
				}
			}
		}
		if !found {
			t.Errorf("%s: no warp at prontera 156,22", mode)
		}
		if len(warps) < 400 {
			t.Errorf("%s: warps on %d maps", mode, len(warps))
		}
	}
}

func TestByMapKeepsEnabledWarpsInOrder(t *testing.T) {
	var d WarpData
	add := func(m string, x int, state string) {
		d.Warps = append(d.Warps, struct {
			Map   string `json:"map"`
			X     int    `json:"x"`
			Y     int    `json:"y"`
			Name  string `json:"name"`
			XS    int    `json:"xs"`
			YS    int    `json:"ys"`
			ToMap string `json:"to_map"`
			ToX   int    `json:"to_x"`
			ToY   int    `json:"to_y"`
			Kind  string `json:"kind"`
			State string `json:"state"`
		}{Map: m, X: x, Y: 1, XS: 1, YS: 1, ToMap: "b", State: state})
	}
	add("a", 3, "enabled")
	add("a", 1, "disabled")
	add("a", 2, "enabled")
	add("c", 5, "hidden")
	got := d.ByMap()
	if len(got) != 1 || len(got["map/a"]) != 2 || got["map/a"][0].X != 3 || got["map/a"][1].X != 2 || got["map/a"][0].To != "map/b" {
		t.Errorf("by map %+v", got)
	}
}

// countSaves counts the saves of one session.
type countSaves struct {
	mu          sync.Mutex
	saves, last int
}

func (c *countSaves) Save(_ *character.Character, final bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if final {
		c.last++
	} else {
		c.saves++
	}
}

// Many players cross between two running maps at once. Each ends in one
// zone, and each session saves once per map change and once at the end.
func TestPlayersCrossMapsConcurrently(t *testing.T) {
	a, b := testZone("map/a"), testZone("map/b")
	// Each map's center is a warp to the other map, 5 cells south of
	// its center.
	a.SetWarps([]zone.Warp{{Name: "ab", X: 15, Y: 15, To: "map/b", ToX: 15, ToY: 20}})
	b.SetWarps([]zone.Warp{{Name: "ba", X: 15, Y: 15, To: "map/a", ToX: 15, ToY: 20}})
	w, err := New("map/a", a, b)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	const n = 40
	sessions := make([]*zone.Session, n)
	savers := make([]*countSaves, n)
	for i := range n {
		c := w.NewCharacter("P", zone.Look{})
		c.X, c.Y = 15, 20
		savers[i] = &countSaves{}
		s, err := w.JoinCharacter(ctx, &c, savers[i])
		if err != nil {
			t.Fatal(err)
		}
		sessions[i] = s
		go func() {
			for range s.Out {
			}
		}()
	}
	// Each round, everyone walks back and forth through the center of
	// whichever map they are on, and so crosses to the other map. New
	// characters start on the center, where walking off the warp does
	// not trigger it, so the walks alternate north and south. Each walk
	// gets 1 second, longer than the 5 cells between the warp and either
	// end take, so it crosses the warp before it turns back. A round ends when
	// every player has changed maps once more. Waiting for that, rather
	// than for a fixed time, keeps the test stable on a busy machine.
	saves := func(i int) int {
		savers[i].mu.Lock()
		defer savers[i].mu.Unlock()
		return savers[i].saves
	}
	const rounds = 3
	seq := uint32(2)
	for round := 1; round <= rounds; round++ {
		// A command can be dropped while a player is still arriving on a
		// map, so walks are sent until that player changes maps.
		deadline := time.Now().Add(30 * time.Second)
		for try := 0; ; try++ {
			y := 10
			if try%2 == 1 {
				y = 20
			}
			waiting := 0
			for i, s := range sessions {
				if saves(i) < round {
					s.WalkTo(seq, 15, y)
					waiting++
				}
			}
			seq++
			if waiting == 0 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("round %d: %d players did not change maps", round, waiting)
			}
			time.Sleep(time.Second)
		}
	}
	for _, s := range sessions {
		s.Leave()
	}
	for i, c := range savers {
		c.mu.Lock()
		if c.last != 1 {
			t.Errorf("player %d: %d final saves", i, c.last)
		}
		if c.saves != rounds {
			t.Errorf("player %d: %d map changes, want %d", i, c.saves, rounds)
		}
		c.mu.Unlock()
	}
	cancel()
	<-w.Stopped()
}

// recorder keeps a character's saves.
type recorder struct {
	mu    sync.Mutex
	saves []character.Character
}

func (r *recorder) Save(c *character.Character, _ bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saves = append(r.saves, *c)
}

func (r *recorder) last() character.Character {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.saves) == 0 {
		return character.Character{}
	}
	return r.saves[len(r.saves)-1]
}

// waitMap reads a session's messages until a map change to a map.
func waitMap(t *testing.T, s *zone.Session, m string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case msg, ok := <-s.Out:
			if !ok {
				t.Fatalf("the session ended before %s", m)
			}
			for _, e := range msg.GetFrame().GetEvents() {
				if e.GetMapChange().GetMap() == m {
					return
				}
			}
		case <-deadline:
			t.Fatalf("no map change to %s", m)
		}
	}
}

// TestRemoveAndEvacuate: a rotation adds a new map's zone while the
// world runs, takes the old one out, moves its player to the town's
// save spot, and moves the save and memo points of a player elsewhere
// and of a player who logs in later (decision 0045).
func TestRemoveAndEvacuate(t *testing.T) {
	moved := &zone.Relocations{}
	town, old := testZone("map/town"), testZone("map/old")
	town.Moved, old.Moved = moved, moved
	w, err := New("map/town", town, old)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)
	inside := w.NewCharacter("Inside", zone.Look{})
	inside.Map, inside.X, inside.Y = "map/old", 5, 5
	si, err := w.JoinCharacter(ctx, &inside, nil)
	if err != nil || si.Map != "map/old" {
		t.Fatalf("joined %v, %v", si, err)
	}
	saver := &recorder{}
	outside := w.NewCharacter("Outside", zone.Look{})
	outside.SaveMap, outside.SaveX, outside.SaveY = "map/old", 3, 3
	outside.Memos = []character.Memo{{Map: "map/old", X: 1, Y: 1}, {Map: "map/town", X: 9, Y: 9}, {Map: "map/far", X: 2, Y: 2}}
	if _, err := w.JoinCharacter(ctx, &outside, saver); err != nil {
		t.Fatal(err)
	}

	// The new map's zone joins the running world.
	fresh := testZone("map/new")
	fresh.Moved = moved
	if err := w.Add(fresh); err != nil {
		t.Fatal(err)
	}
	if err := w.Add(testZone("map/new")); err == nil {
		t.Error("a map added twice")
	}
	moved.Add("map/old", zone.Relocation{Map: "map/town", X: 12, Y: 13})
	gone := w.Remove("map/old")
	if gone != old || w.Zone("map/old") != nil || w.Remove("map/town") != nil {
		t.Fatal("remove")
	}
	if n, err := old.Evacuate(ctx, "map/town", 12, 13); err != nil || n != 1 {
		t.Fatalf("evacuated %d, %v", n, err)
	}
	waitMap(t, si, "map/town")
	for _, z := range w.Zones() {
		if err := z.Relocate(ctx, moved); err != nil {
			t.Fatal(err)
		}
	}
	// The new map runs, with nobody on it yet.
	n, err := fresh.Players(ctx)
	if err != nil || n != 0 {
		t.Errorf("players on the new map %d %v", n, err)
	}
	w.Retire(old)
	select {
	case <-old.Stopped():
	case <-time.After(5 * time.Second):
		t.Fatal("the old zone did not stop")
	}
	// A login whose character was saved on the old map goes to the town.
	late := w.NewCharacter("Late", zone.Look{})
	late.Map, late.X, late.Y = "map/old", 5, 5
	late.SaveMap = "map/old"
	sl, err := w.JoinCharacter(ctx, &late, nil)
	if err != nil || sl.Map != "map/town" {
		t.Fatalf("a late login joined %v, %v", sl, err)
	}
	cancel()
	select {
	case <-w.Stopped():
	case <-time.After(5 * time.Second):
		t.Fatal("the world did not stop")
	}
	c := saver.last()
	want := []character.Memo{{Map: "map/town", X: 12, Y: 13}, {Map: "map/far", X: 2, Y: 2}}
	if c.SaveMap != "map/town" || c.SaveX != 12 || c.SaveY != 13 || len(c.Memos) != 2 || c.Memos[0] != want[0] || c.Memos[1] != want[1] {
		t.Errorf("the last save: %s %d,%d %+v", c.SaveMap, c.SaveX, c.SaveY, c.Memos)
	}
}
