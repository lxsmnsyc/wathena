package main

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/rotation"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestBackground: a function that runs until the server stops ends when
// runAll returns before it serves. A start that failed after the server
// variables began to run, such as one whose event named a map that does
// not run, used to wait for a signal that never came (decision 0045).
func TestBackground(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	ran := make(chan struct{})
	wait := background(ctx, stop, func(ctx context.Context) {
		close(ran)
		<-ctx.Done()
	})
	<-ran
	done := make(chan struct{})
	go func() {
		wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the wait blocked")
	}
	if ctx.Err() == nil {
		t.Error("the wait did not end the context")
	}
}

// changeWorld is a dev world with --layout and its zones, as runAll
// makes them, without the network.
type changeWorld struct {
	proc  proceduralWorld
	world *world.World
	moved *zone.Relocations
	ctx   context.Context
}

func newChangeWorld(t *testing.T) *changeWorld {
	t.Helper()
	if !gen.Procedural {
		t.Skip("a build without the procedural tag")
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := settings{options: defaultOptions()}
	cfg.Dev, cfg.Seed = true, "change-host"
	cfg.Procedural.FieldSize, cfg.Procedural.DevLayout = 64, true
	towns, dungeons := 2, 1
	cfg.Procedural.Layout = layout.Options{Grid: []int{2, 2}, Towns: &towns, Dungeons: &dungeons}
	cfg.Location = time.UTC
	p, err := fieldMaps(cfg, store.World{}, log)
	if err != nil {
		t.Fatal(err)
	}
	if err := addMaps(cfg, &p, coreSet(t), nil, log); err != nil {
		t.Fatal(err)
	}
	if err := changedSpawns(cfg, &p, &gamedata.Mods{}, log); err != nil {
		t.Fatal(err)
	}
	moved := &zone.Relocations{}
	idx := p.set.Places(p.warps)
	p.host.places, p.host.moved = idx, moved
	newZone := func(n string, grid *path.Grid, sp []zone.Spawn, ws []zone.Warp, _ []zone.NPC) *zone.Zone {
		z := zone.New(n, grid, rng.FromKey([32]byte{9}).SecretStream(), sp)
		z.SaveMap, z.Moved = p.start, moved
		z.SetWarps(ws)
		return z
	}
	var zones []*zone.Zone
	for _, f := range p.set.Fields() {
		zones = append(zones, newZone(f.ID, f.Grid(), nil, p.warps[f.ID], nil))
	}
	w, err := world.New(p.start, zones...)
	if err != nil {
		t.Fatal(err)
	}
	p.host.world, p.host.newZone, p.host.retire = w, newZone, w.Retire
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go w.Run(ctx)
	return &changeWorld{proc: p, world: w, moved: moved, ctx: ctx}
}

// mapChange waits for a session's map change to a map.
func mapChange(t *testing.T, s *zone.Session, m string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
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

// TestChangeHost: a dev world with --layout rotates its dungeon as a new
// world does. A rotation adds the new floors' zones, points the entrance
// at the new first floor in the same step, moves the player inside to
// the nearest town's save spot, moves the points of a player elsewhere,
// and takes the old floors out; a region's change replaces its fields'
// zones with the same IDs (strategy section 8.10, decision 0045).
func TestChangeHost(t *testing.T) {
	cw := newChangeWorld(t)
	p, h := cw.proc, cw.proc.host
	if len(p.setup.opts.Rotations) != 1 {
		t.Fatalf("rotations %+v", p.setup.opts.Rotations)
	}
	d := p.setup.opts.Rotations[0].Dungeon
	from := p.setup.state
	date := from.Dates[d]
	if date == "" || len(p.setup.due) != 1 || !p.setup.due[0].First {
		t.Fatalf("the start: %+v %+v", from, p.setup.due)
	}
	old := layout.FloorID(d, date, 1)
	if cw.world.Zone(old) == nil || cw.world.Zone(gen.DungeonMapID(d, 1)) != nil {
		t.Fatalf("the first floor %s does not run", old)
	}
	if m, ok := p.atlas.Map(old); !ok || !m.NoMemo {
		t.Errorf("the floor %+v", m)
	}
	// A player on the first floor, and one elsewhere with points there.
	inside := cw.world.NewCharacter("Inside", zone.Look{})
	inside.Map, inside.X, inside.Y = old, 1, 1
	si, err := cw.world.JoinCharacter(cw.ctx, &inside, nil)
	if err != nil || si.Map != old {
		t.Fatalf("joined %v %v", si, err)
	}
	saves := make(chan *character.Character, 64)
	outside := cw.world.NewCharacter("Outside", zone.Look{})
	outside.SaveMap, outside.Memos = old, []character.Memo{{Map: old, X: 3, Y: 3}}
	if _, err := cw.world.JoinCharacter(cw.ctx, &outside, saverFunc(func(c *character.Character) { saves <- c })); err != nil {
		t.Fatal(err)
	}

	next := "2099-01-01"
	c := rotation.Change{Kind: rotation.KindRotation, Dungeon: d, Date: next}
	to := c.Next(from)
	if err := h.Prepare(cw.ctx, from, to, c); err != nil {
		t.Fatal(err)
	}
	moves, err := h.Apply(cw.ctx, from, to, c)
	if err != nil {
		t.Fatal(err)
	}
	town, ok := p.atlas.NearestTown(layout.FloorID(d, next, 1))
	if !ok || len(moves) == 0 || moves[0].From != old || moves[0].To != town.ID {
		t.Fatalf("moves %+v, town %s", moves, town.ID)
	}
	mapChange(t, si, town.ID)
	fresh := layout.FloorID(d, next, 1)
	if cw.world.Zone(fresh) == nil || cw.world.Zone(old) != nil || p.set.Get(fresh) == nil || p.set.Get(old) != nil {
		t.Fatal("the floors did not change")
	}
	// The entrance leads to the new floor.
	entrance := ""
	for _, k := range p.atlas.Links() {
		if k.B.Map == fresh && k.B.Tag == layout.TagEntrance {
			entrance = k.A.Map
		}
	}
	if !slices.ContainsFunc(h.warps[entrance], func(w zone.Warp) bool { return w.To == fresh }) ||
		slices.ContainsFunc(h.warps[entrance], func(w zone.Warp) bool { return strings.Contains(w.To, date) }) {
		t.Errorf("the entrance's warps %+v", h.warps[entrance])
	}
	if to, ok := cw.moved.Lookup(old); !ok || to.Map != town.ID {
		t.Errorf("relocation %+v", to)
	}
	h.Done(cw.ctx, c)
	// The outside player's points moved; its next save holds them.
	z := cw.world.Zone(p.start)
	if err := z.Relocate(cw.ctx, cw.moved); err != nil {
		t.Fatal(err)
	}

	// A region's change: its fields get new zones with the same IDs.
	region := p.setup.opts.Regions[0]
	field := layout.Regions(p.atlas.Base())[0].Fields[0]
	before := cw.world.Zone(field)
	rc := rotation.Change{Kind: rotation.KindRegion, Region: region, Season: 1}
	if _, err := h.Apply(cw.ctx, to, rc.Next(to), rc); err != nil {
		t.Fatal(err)
	}
	if z := cw.world.Zone(field); z == nil || z == before {
		t.Error("the field's zone did not change")
	}
	// The field runs again with its ID, so points made there later stay.
	if _, ok := cw.moved.Lookup(field); ok {
		t.Error("a replaced field's later points would move")
	}
	if m, _ := p.atlas.Map(field); m.Season != 1 {
		t.Errorf("the field %+v", m)
	}
	if f := p.set.Get(field); f == nil || f.Key != layout.SeasonNode(p.public, f.X, f.Y, 1).Key() {
		t.Error("the field is not its season's")
	}
	h.Done(cw.ctx, rc)
}

type saverFunc func(c *character.Character)

func (f saverFunc) Save(c *character.Character, _ bool) { f(c) }
