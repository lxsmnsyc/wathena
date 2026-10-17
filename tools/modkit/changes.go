package modkit

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/fields"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/server/rotation"
	"github.com/lxsmnsyc/wathena/server/schedule"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Rotations and world changes in test worlds (proposal 0002, review
// decision 47, decision 0045). world:rotate(dungeon) and
// world:change_region(x, y) change a test world's generated maps as
// staff force a change, at once:
//
//   - Each test world has its own copy of the generated world's set,
//     atlas and places, so a change stays in its case.
//   - A rotation gives the dungeon's floors the date of its style's next
//     rotation after the world's time, or after the date it rotated to
//     before, with their spawns from the test world's rotation key.
//   - A region's change gives its fields their next season and a new
//     biome, with their spawns from the season key.
//   - The points on the maps that went move to their nearest town's save
//     spot, and the zone's dungeon_rotated or region_changed handlers
//     run.
//   - A test world runs one map, so a case that runs on a map the change
//     removes or replaces cannot make it.

// testChanges are one test world's copy of the generated maps.
type testChanges struct {
	g      *generated
	set    *fields.Set
	atlas  *layout.Atlas
	places *places.Index
	warps  map[string][]zone.Warp
	state  layout.State
	// planned are the spawn lists of the maps a change made.
	planned   map[string][]spawns.Spawn
	spawnMaps map[string]spawns.Map
}

// newTestChanges copies a generated world for one test world.
func newTestChanges(g *generated) *testChanges {
	a := layout.NewAtlas(g.atlas.Base(), layout.AtlasOptions{})
	a.Replace(g.atlas.Layout())
	return &testChanges{g: g, set: g.set.With(nil, nil), atlas: a, places: g.places.Clone(), warps: g.warps,
		state: layout.State{Dates: map[string]string{}, Seasons: map[string]int{}}, planned: map[string][]spawns.Spawn{},
		spawnMaps: map[string]spawns.Map{}}
}

// apply runs the test world in a new state. ids are the maps whose
// spawn lists come from secret.
func (c *testChanges) apply(w *testWorld, st layout.State, secret func(m layout.Map) (spawnsSecret, bool)) (map[string]zone.Relocation, error) {
	g := c.g
	cur := c.atlas.Layout()
	next := c.atlas.Base().Apply(g.public, len(g.gen.Biomes), st)
	var made []*layout.Map
	var removed, replaced []string
	for i := range next.Maps {
		m := &next.Maps[i]
		o := cur.Map(m.ID)
		if o == nil || o.Season != m.Season || o.Biome != m.Biome || o.Date != m.Date {
			made = append(made, m)
			if o != nil {
				replaced = append(replaced, m.ID)
			}
		}
	}
	for _, m := range cur.Maps {
		if next.Map(m.ID) == nil {
			removed = append(removed, m.ID)
		}
	}
	if bound, ok := w.h.Bound(); ok && (slices.Contains(removed, bound) || slices.Contains(replaced, bound)) {
		return nil, fmt.Errorf("the case runs on %s, which the change removes; a test world runs one map, so run the case on another map", bound)
	}
	list, err := fields.GenerateLayoutMaps(g.gen, g.public, made, nil)
	if err != nil {
		return nil, err
	}
	set := c.set.With(list, removed)
	warps, err := set.LayoutWarps(next)
	if err != nil {
		return nil, err
	}
	// The moves go by the graph before the change.
	before := layout.NewAtlas(cur, layout.AtlasOptions{})
	moves := map[string]zone.Relocation{}
	for _, id := range slices.Concat(removed, replaced) {
		t, ok := before.NearestTown(id)
		if !ok {
			continue
		}
		if f := c.set.Get(t.ID); f != nil {
			x, y := fields.SaveSpot(f, c.warps[t.ID])
			moves[id] = zone.Relocation{Map: t.ID, X: x, Y: y}
		}
	}
	groups := map[string][]string{}
	nodes := map[string]spawnsSecret{}
	for _, m := range made {
		if s, ok := secret(*m); ok {
			groups[s.group] = append(groups[s.group], m.ID)
			nodes[s.group] = s
		}
	}
	for grp, ids := range groups {
		sm, err := fields.SpawnMapsOf(next, warps, set.Get, func(id string) bool { return slices.Contains(ids, id) })
		if err != nil {
			return nil, err
		}
		planned := spawns.Plan(nodes[grp].node, sm, spawns.NewPool(w.l.SpawnMonsters, w.l.SpawnThemes), g.settings)
		for i, p := range planned {
			c.planned[p.Map], c.spawnMaps[p.Map] = p.Spawns, sm[i]
		}
	}
	var newMaps []places.Map
	for _, f := range list {
		newMaps = append(newMaps, places.Map{ID: f.ID, Places: f.AllPlaces()})
	}
	var links []places.Link
	for _, f := range set.Fields() {
		for _, wp := range warps[f.ID] {
			links = append(links, places.Link{From: f.ID, X: wp.X, Y: wp.Y, To: wp.To, ToX: wp.ToX, ToY: wp.ToY})
		}
	}
	c.set.Update(set)
	c.atlas.Replace(next)
	c.places.Update(removed, newMaps, links)
	c.warps, c.state = warps, st
	return moves, nil
}

// spawnsSecret is the secret of a group of maps' spawns.
type spawnsSecret struct {
	group string
	node  rng.Node
}

// rotate is world:rotate(dungeon).
func (w *testWorld) rotate(args []script.Value) ([]script.Value, error) {
	const what = "world:rotate"
	c, err := w.testChanges(what)
	if err != nil {
		return nil, err
	}
	dungeon, _ := args[0].AsString()
	first := c.atlas.Base().Map(gen.DungeonMapID(dungeon, 1))
	if first == nil {
		return nil, fmt.Errorf("%s: the test world has no dungeon %q; its dungeons are named by their styles, such as crypt", what, dungeon)
	}
	style, ok := fields.FindStyle(w.l.DungeonStyles, first.Record)
	if !ok || style.Rotation == nil {
		return nil, fmt.Errorf("%s: the dungeon %s's style %s has no rotation", what, dungeon, first.Record)
	}
	spec, err := style.Rotation.Spec()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	if err := w.bindDefault(what); err != nil {
		return nil, err
	}
	loc := w.h.Location()
	after := w.h.Now()
	if d := c.state.Dates[dungeon]; d != "" {
		if t, err := time.ParseInLocation("2006-01-02", d, loc); err == nil {
			if at := schedule.Resolve(t.Year(), t.Month(), t.Day(), spec.Hour, spec.Minute, loc); at.After(after) {
				after = at
			}
		}
	}
	date := layout.DateOf(schedule.Next(spec, after, loc), loc)
	st := c.state.Clone()
	st.Dates[dungeon] = date
	key := rotation.RotationKey(c.g.secret, dungeon)
	moves, err := c.apply(w, st, func(m layout.Map) (spawnsSecret, bool) {
		if m.Dungeon != dungeon || m.Date == "" {
			return spawnsSecret{}, false
		}
		return spawnsSecret{group: dungeon + ":" + m.Date, node: rotation.RotationSecret(key, m.Date)}, true
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	// The old floors are gone for good.
	w.h.WorldChanged(modscript.WorldChange{Dungeon: dungeon, Date: date}, moves, slices.Collect(maps.Keys(moves)))
	return []script.Value{script.String(date)}, nil
}

// changeRegion is world:change_region(x, y).
func (w *testWorld) changeRegion(args []script.Value) ([]script.Value, error) {
	const what = "world:change_region"
	c, err := w.testChanges(what)
	if err != nil {
		return nil, err
	}
	x, _ := args[0].AsInt()
	y, _ := args[1].AsInt()
	region, ok := layout.RegionOf(layout.Regions(c.atlas.Base()), x, y)
	if !ok {
		return nil, fmt.Errorf("%s: the test world has no field %s in a region; its field is gen:field:0:0", what, gen.FieldID(x, y))
	}
	if err := w.bindDefault(what); err != nil {
		return nil, err
	}
	st := c.state.Clone()
	st.Seasons[region]++
	season := st.Seasons[region]
	key := rotation.SeasonKey(c.g.secret)
	moves, err := c.apply(w, st, func(m layout.Map) (spawnsSecret, bool) {
		if m.Kind != gen.KindField || m.Season == 0 {
			return spawnsSecret{}, false
		}
		return spawnsSecret{group: region, node: rotation.SeasonSecret(key, region, m.Season)}, true
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	// The region's fields run again with their IDs.
	w.h.WorldChanged(modscript.WorldChange{Region: region, Season: season}, moves, nil)
	return []script.Value{script.String(region), script.Int(int64(season))}, nil
}

// testChanges returns the test world's copy of the generated maps.
func (w *testWorld) testChanges(what string) (*testChanges, error) {
	if w.changes == nil {
		return nil, errors.New(what + ": " + errNoGenerator.Error())
	}
	return w.changes, nil
}

// bindDefault gives a world without a map its default map, as
// world:plan_restart does, so the handlers run.
func (w *testWorld) bindDefault(what string) error {
	if _, ok := w.h.Bound(); ok {
		return nil
	}
	if err := w.bind(""); err != nil {
		return fmt.Errorf("%s: %w", what, err)
	}
	return nil
}
