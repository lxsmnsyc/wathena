package main

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/economy"
	"github.com/lxsmnsyc/wathena/server/fields"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/server/rotation"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/world"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Rotating dungeons and world changes (decision 0045): the world starts
// in the state the stored one leads to by now, and a rotation.Manager
// makes the later changes through changeHost.

// retireAfter is how long a removed map's zone keeps running after its
// players left, so a player who took a warp to it a moment before goes
// on to the town.
const retireAfter = 30 * time.Second

// warnColor is the color of the countdown's lines, as announcements.
const warnColor = 0xFFFF00

// changeKeys are the secret nodes of the spawns of rotating floors and
// changed regions (review decision 47), from the database's derived keys
// or a dev world's secret.
type changeKeys struct {
	rotation  map[string]rng.Node
	season    rng.Node
	hasSeason bool
}

// changeSetup is a procedural world's rotations and seasons at its
// start.
type changeSetup struct {
	opts  rotation.Options
	state rotation.State
	// due are the changes that came while the server was stopped, which
	// the start records.
	due  []rotation.Change
	keys changeKeys
}

// setupChanges reads a world's rotating dungeons and seasons, and the
// state it runs at its start (decision 0045):
//
//   - A stored world: the dungeon wactl world new marked, the derived
//     keys, and the stored dates and seasons.
//   - A dev world with --layout: the dungeon a new world would mark, and
//     keys from the dev secret. Its state lives in memory.
//   - A dev world of --fields, --towns and --dungeons: none, so its
//     floors keep their IDs.
func setupChanges(cfg settings, proc *proceduralWorld, l *layout.Layout, styles []layout.StyleRecord, now time.Time, log *slog.Logger) (changeSetup, error) {
	var out changeSetup
	out.keys.rotation = map[string]rng.Node{}
	out.state = rotation.State{State: layout.State{Dates: map[string]string{}, Seasons: map[string]int{}}}
	switch {
	case proc.stored != nil:
		for _, r := range proc.changes.Rotations {
			if len(r.Key) != 32 {
				return out, fmt.Errorf("the rotating dungeon %s has no key", r.Dungeon)
			}
			out.opts.Rotations = append(out.opts.Rotations, rotation.Rotation{Dungeon: r.Dungeon, Rule: r.Rule})
			out.keys.rotation[r.Dungeon] = rng.FromKey([32]byte(r.Key))
			if r.ActiveDate != "" {
				out.state.Dates[r.Dungeon] = r.ActiveDate
			}
		}
		if len(proc.changes.SeasonKey) == 32 {
			out.keys.season, out.keys.hasSeason = rng.FromKey([32]byte(proc.changes.SeasonKey)), true
		}
		maps.Copy(out.state.Seasons, proc.changes.Regions)
		out.state.SeasonsStarted, out.state.SeasonEnds = proc.changes.SeasonsStarted, proc.changes.SeasonEnds
	case cfg.Procedural.DevLayout && proc.devSecret != nil:
		if r, ok := layout.PickRotation(l, styles); ok {
			out.opts.Rotations = append(out.opts.Rotations, rotation.Rotation{Dungeon: r.Dungeon, Rule: r.Rule})
			out.keys.rotation[r.Dungeon] = rotation.RotationKey(*proc.devSecret, r.Dungeon)
		}
		out.keys.season, out.keys.hasSeason = rotation.SeasonKey(*proc.devSecret), true
	}
	for _, r := range out.opts.Rotations {
		if l.Map(gen.DungeonMapID(r.Dungeon, 1)) == nil {
			return out, fmt.Errorf("the rotating dungeon %s is not in the world layout", r.Dungeon)
		}
		log.Info("rotating dungeon", "dungeon", r.Dungeon, "rotation", r.Rule.String(), "date", out.state.Dates[r.Dungeon])
	}
	for _, r := range layout.Regions(l) {
		out.opts.Regions = append(out.opts.Regions, r.Town)
	}
	if proc.stored != nil || cfg.Procedural.DevLayout {
		out.opts.SeasonDays = cfg.Procedural.Seasons.Days
	}
	if out.opts.SeasonDays > 0 && !out.keys.hasSeason {
		log.Warn("seasons stay off: the world has no season key; make it again with wactl world new")
		out.opts.SeasonDays = 0
	}
	out.opts.Location = cfg.Location
	var err error
	out.state, out.due, err = rotation.Due(out.opts, out.state, now)
	return out, err
}

// secretOf returns the secret of a map's spawns that the stored lists do
// not hold, by group: a rotating floor's date, or a changed region's
// season. ok is false for a map that runs its stored list.
func (k changeKeys) secretOf(m layout.Map, region string) (group string, node rng.Node, ok bool) {
	switch {
	case m.Kind == gen.KindDungeon && m.Date != "":
		key, found := k.rotation[m.Dungeon]
		if !found {
			return "", rng.Node{}, false
		}
		return "rotation:" + m.Dungeon + ":" + m.Date, rotation.RotationSecret(key, m.Date), true
	case m.Kind == gen.KindField && m.Season > 0 && k.hasSeason && region != "":
		return "season:" + region + ":" + strconv.Itoa(m.Season), rotation.SeasonSecret(k.season, region, m.Season), true
	}
	return "", rng.Node{}, false
}

// changeHost makes rotations and region changes in the running world
// (rotation.Host). The rotation.Manager calls it from one goroutine.
type changeHost struct {
	g      gen.Generator
	public rng.Node
	atlas  *layout.Atlas
	set    *fields.Set
	places *places.Index
	moved  *zone.Relocations
	keys   changeKeys
	// regions are each field's region, by map ID.
	regions   map[string]string
	pool      *spawns.Pool
	settings  spawns.Resolved
	monsters  map[uint32]gamedata.Monster
	placeNPCs []gamedata.PlaceNPC
	log       *slog.Logger

	// world, newZone and retire are set once the world exists.
	world   *world.World
	newZone func(n string, grid *path.Grid, sp []zone.Spawn, ws []zone.Warp, npcs []zone.NPC) *zone.Zone
	retire  func(z *zone.Zone)

	mu sync.Mutex
	// warps are the generated maps' warps that run, by map ID.
	warps map[string][]zone.Warp
	// built are maps that Prepare made, by buildKey.
	built map[string]*fields.Field
	// old are the zones each change took out, by the change's key.
	old map[string][]*zone.Zone
}

// buildKey names a generated map with what makes it: its ID, date,
// season and biome.
func buildKey(m layout.Map) string {
	return m.ID + "|" + m.Date + "|" + strconv.Itoa(m.Season) + "|" + strconv.Itoa(m.Biome)
}

// changeKey names a change for the zones it took out.
func changeKey(c rotation.Change) string {
	if c.Kind == rotation.KindRotation {
		return c.Kind + ":" + c.Dungeon + ":" + c.Date
	}
	return c.Kind + ":" + c.Region + ":" + strconv.Itoa(c.Season)
}

// regionsOf maps each field to its region.
func regionsOf(l *layout.Layout) map[string]string {
	out := map[string]string{}
	for _, r := range layout.Regions(l) {
		for _, f := range r.Fields {
			out[f] = r.Town
		}
	}
	return out
}

// diff returns the maps of next that cur lacks or runs otherwise, and
// the maps of cur that next lacks.
func diff(cur, next *layout.Layout) (made []*layout.Map, removed []string) {
	for i := range next.Maps {
		m := &next.Maps[i]
		c := cur.Map(m.ID)
		if c == nil || c.Season != m.Season || c.Biome != m.Biome || c.Date != m.Date {
			made = append(made, m)
		}
	}
	for _, m := range cur.Maps {
		if next.Map(m.ID) == nil {
			removed = append(removed, m.ID)
		}
	}
	return made, removed
}

// generate makes maps, from what Prepare built when it can.
func (h *changeHost) generate(list []*layout.Map) ([]*fields.Field, error) {
	out := make([]*fields.Field, len(list))
	var todo []*layout.Map
	var at []int
	for i, m := range list {
		if f := h.built[buildKey(*m)]; f != nil {
			out[i] = f
			continue
		}
		todo, at = append(todo, m), append(at, i)
	}
	if len(todo) > 0 {
		made, err := fields.GenerateLayoutMaps(h.g, h.public, todo, nil)
		if err != nil {
			return nil, err
		}
		for k, f := range made {
			out[at[k]] = f
		}
	}
	return out, nil
}

// plan computes the spawn lists of maps that do not run stored lists,
// grouped by their secret (changeKeys.secretOf).
func (h *changeHost) plan(next *layout.Layout, warps map[string][]zone.Warp, get func(string) *fields.Field, ids []string) map[string][]spawns.Spawn {
	return planChanged(next, warps, get, ids, h.keys, h.regions, h.pool, h.settings)
}

// planChanged computes the spawn lists of maps that do not run stored
// lists, grouped by their secret (changeKeys.secretOf).
func planChanged(next *layout.Layout, warps map[string][]zone.Warp, get func(string) *fields.Field, ids []string,
	keys changeKeys, regions map[string]string, pool *spawns.Pool, st spawns.Resolved) map[string][]spawns.Spawn {
	groups := map[string][]string{}
	nodes := map[string]rng.Node{}
	for _, id := range ids {
		m := next.Map(id)
		if m == nil {
			continue
		}
		g, node, ok := keys.secretOf(*m, regions[id])
		if !ok {
			continue
		}
		groups[g], nodes[g] = append(groups[g], id), node
	}
	out := map[string][]spawns.Spawn{}
	for _, g := range slices.Sorted(maps.Keys(groups)) {
		only := groups[g]
		sm, err := fields.SpawnMapsOf(next, warps, get, func(id string) bool { return slices.Contains(only, id) })
		if err != nil {
			continue
		}
		for _, p := range spawns.Plan(nodes[g], sm, pool, st) {
			out[p.Map] = p.Spawns
		}
	}
	return out
}

// destination is where a map's players and points go: the nearest town
// by the map graph of l, at its save spot.
func (h *changeHost) destination(a *layout.Atlas, id string) (zone.Relocation, bool) {
	t, ok := a.NearestTown(id)
	if !ok {
		return zone.Relocation{}, false
	}
	f := h.set.Get(t.ID)
	if f == nil {
		return zone.Relocation{}, false
	}
	x, y := fields.SaveSpot(f, h.warps[t.ID])
	return zone.Relocation{Map: t.ID, X: x, Y: y}, true
}

// movesOf returns the moves of the points of the maps a change removes
// or replaces, by the layout that runs before it.
func (h *changeHost) movesOf(cur *layout.Layout, ids []string) []rotation.Move {
	a := layout.NewAtlas(cur, layout.AtlasOptions{})
	var out []rotation.Move
	for _, id := range ids {
		if to, ok := h.destination(a, id); ok {
			out = append(out, rotation.Move{From: id, To: to.Map, X: to.X, Y: to.Y})
		}
	}
	return out
}

// Prepare builds the maps a change adds before it happens.
func (h *changeHost) Prepare(_ context.Context, _, to rotation.State, _ rotation.Change) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	next := h.atlas.Base().Apply(h.public, len(h.g.Biomes), to.State)
	made, _ := diff(h.atlas.Layout(), next)
	list, err := h.generate(made)
	if err != nil {
		return err
	}
	for i, m := range made {
		h.built[buildKey(*m)] = list[i]
	}
	return nil
}

// affected returns the maps a change removes or replaces, by the layout
// that runs.
func (h *changeHost) affected(c rotation.Change) []layout.Map {
	var out []layout.Map
	for _, m := range h.atlas.Layout().Maps {
		switch {
		case c.Kind == rotation.KindRotation && m.Kind == gen.KindDungeon && m.Dungeon == c.Dungeon && m.Date != "":
			out = append(out, m)
		case c.Kind == rotation.KindRegion && m.Kind == gen.KindField && h.regions[m.ID] == c.Region:
			out = append(out, m)
		}
	}
	return out
}

// Warn tells the players on the maps a change removes or replaces how
// long is left, and where they will go.
func (h *changeHost) Warn(_ context.Context, c rotation.Change, left time.Duration) {
	if h.world == nil {
		return
	}
	for _, m := range h.affected(c) {
		z := h.world.Zone(m.ID)
		if z == nil {
			continue
		}
		town := "the nearest town"
		if t, ok := h.atlas.NearestTown(m.ID); ok && t.Name != "" {
			town = t.Name
		}
		what := "changes with the season"
		if c.Kind == rotation.KindRotation {
			what = "closes for new floors"
		}
		z.Announce(nameOr(m)+" "+what+" in "+humanLeft(left)+". Anyone still here then goes to "+town+".", warnColor)
	}
}

func nameOr(m layout.Map) string {
	if m.Name != "" {
		return m.Name
	}
	return m.ID
}

// humanLeft writes a time left, such as "10 minutes" or "30 seconds".
func humanLeft(d time.Duration) string {
	if d >= time.Minute {
		n := int((d + 30*time.Second) / time.Minute)
		if n == 1 {
			return "1 minute"
		}
		return strconv.Itoa(n) + " minutes"
	}
	n := max(int((d+time.Second/2)/time.Second), 1)
	if n == 1 {
		return "1 second"
	}
	return strconv.Itoa(n) + " seconds"
}

// zoneWait is the longest the host waits on one zone.
const zoneWait = 10 * time.Second

// Apply makes a change in the running world, as strategy section 8.10
// says: the new maps and their zones, the portals to them in the same
// step, the players of the removed and replaced maps moved to their
// nearest town, and the points of the players online. It returns the
// moves for the store.
func (h *changeHost) Apply(ctx context.Context, _, to rotation.State, c rotation.Change) ([]rotation.Move, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.world == nil || h.newZone == nil {
		return nil, fmt.Errorf("the world does not run yet")
	}
	cur := h.atlas.Layout()
	next := h.atlas.Base().Apply(h.public, len(h.g.Biomes), to.State)
	made, removed := diff(cur, next)
	list, err := h.generate(made)
	if err != nil {
		return nil, err
	}
	newSet := h.set.With(list, removed)
	warps, err := newSet.LayoutWarps(next)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(made))
	madeSet := map[string]bool{}
	var replaced []string
	for i, m := range made {
		ids[i], madeSet[m.ID] = m.ID, true
		if cur.Map(m.ID) != nil {
			replaced = append(replaced, m.ID)
		}
	}
	planned := h.plan(next, warps, newSet.Get, ids)
	// The new maps' places, for NPCs placed by tag on them.
	var newMaps []places.Map
	for _, f := range list {
		newMaps = append(newMaps, places.Map{ID: f.ID, Places: f.AllPlaces()})
	}
	tmp := places.New(newMaps, nil)
	npcs := placeNPCs(tmp, h.placeNPCs, h.log)
	zones := make([]*zone.Zone, len(list))
	for i, f := range list {
		zs, missing := gamedata.ZoneSpawns(planned[f.ID], h.monsters)
		for _, m := range missing {
			h.log.Warn("a spawn names a monster that does not load; its line is left out", "map", f.ID, "monster", m)
		}
		zones[i] = h.newZone(f.ID, f.Grid(), zs, warps[f.ID], npcs[f.ID])
	}
	moves := h.movesOf(cur, slices.Concat(removed, replaced))

	// From here the world changes, in the order of strategy section
	// 8.10: the maps, the portals, then the players and the points.
	h.set.Update(newSet)
	h.atlas.Replace(next)
	var links []places.Link
	for _, id := range slices.Sorted(maps.Keys(warps)) {
		for _, w := range warps[id] {
			links = append(links, places.Link{From: id, X: w.X, Y: w.Y, To: w.To, ToX: w.ToX, ToY: w.ToY})
		}
	}
	h.places.Update(removed, newMaps, links)
	// A removed map's points move at every later join too. A replaced
	// map runs again with its ID, so its points move this once.
	once := &zone.Relocations{}
	for _, mv := range moves {
		to := zone.Relocation{Map: mv.To, X: mv.X, Y: mv.Y}
		once.Add(mv.From, to)
		if slices.Contains(removed, mv.From) {
			h.moved.Add(mv.From, to)
		}
	}
	var old []*zone.Zone
	for _, z := range zones {
		if slices.Contains(replaced, z.Map) {
			if o := h.world.Swap(z); o != nil {
				old = append(old, o)
			}
			continue
		}
		if err := h.world.Add(z); err != nil {
			h.log.Error("a new map's zone did not start", "map", z.Map, "err", err)
		}
	}
	for _, id := range removed {
		if o := h.world.Remove(id); o != nil {
			old = append(old, o)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(warps)) {
		if madeSet[id] || slices.EqualFunc(h.warps[id], warps[id], func(a, b zone.Warp) bool { return a == b }) {
			continue
		}
		if z := h.world.Zone(id); z != nil {
			wctx, cancel := context.WithTimeout(ctx, zoneWait)
			if err := z.ReplaceWarps(wctx, warps[id]); err != nil {
				h.log.Error("a map's portals did not change", "map", id, "err", err)
			}
			cancel()
		}
	}
	h.warps = warps
	for _, o := range old {
		to, ok := once.Lookup(o.Map)
		if !ok {
			continue
		}
		wctx, cancel := context.WithTimeout(ctx, zoneWait)
		n, err := o.Evacuate(wctx, to.Map, to.X, to.Y)
		cancel()
		if err != nil {
			h.log.Error("the players of a removed map did not move", "map", o.Map, "err", err)
		} else if n > 0 {
			h.log.Info("players moved off a removed map", "map", o.Map, "to", to.Map, "players", n)
		}
	}
	for _, z := range h.world.Zones() {
		wctx, cancel := context.WithTimeout(ctx, zoneWait)
		if err := z.Relocate(wctx, once); err != nil {
			h.log.Warn("a zone did not move its players' points; their next login does", "map", z.Map, "err", err)
		}
		cancel()
	}
	h.old[changeKey(c)] = old
	for _, m := range made {
		delete(h.built, buildKey(*m))
	}
	return moves, nil
}

// Done runs the dungeon_rotated or region_changed handlers in every
// zone, and stops the old zones after retireAfter.
func (h *changeHost) Done(_ context.Context, c rotation.Change) {
	h.mu.Lock()
	old := h.old[changeKey(c)]
	delete(h.old, changeKey(c))
	h.mu.Unlock()
	ev := modscript.WorldChange{Region: c.Region, Season: c.Season}
	if c.Kind == rotation.KindRotation {
		ev = modscript.WorldChange{Dungeon: c.Dungeon, Date: c.Date}
	}
	if h.world != nil {
		for _, z := range h.world.Zones() {
			z.WorldChanged(ev)
		}
	}
	if len(old) > 0 && h.retire != nil {
		time.AfterFunc(retireAfter, func() {
			for _, z := range old {
				h.retire(z)
			}
		})
	}
}

// changeStore records changes in the database, or nowhere for a dev
// world without one.
type changeStore struct{ pool *pgxpool.Pool }

// Commit records a change with its moves.
func (s changeStore) Commit(ctx context.Context, c rotation.Change, moves []rotation.Move) error {
	if s.pool == nil {
		return nil
	}
	w := store.WorldChange{}
	if c.Kind == rotation.KindRotation {
		w.Dungeon, w.Date = c.Dungeon, c.Date
	} else {
		w.Region, w.Season, w.SeasonEnds = c.Region, c.Season, c.SeasonEnd
	}
	for _, m := range moves {
		w.Moves = append(w.Moves, store.Move{From: m.From, To: m.To, X: m.X, Y: m.Y})
	}
	return store.CommitWorldChange(ctx, s.pool, w)
}

// StartSeasons records the date seasons started.
func (s changeStore) StartSeasons(ctx context.Context, date string) error {
	if s.pool == nil {
		return nil
	}
	return store.StartSeasons(ctx, s.pool, date)
}

// startMoves returns the moves of the changes that came while the server
// was stopped: the old floors of each rotation, and the fields of each
// changed region, to their nearest town's save spot in the layout that
// runs now.
func (h *changeHost) startMoves(from rotation.State, due []rotation.Change) [][]rotation.Move {
	a := layout.NewAtlas(h.atlas.Layout(), layout.AtlasOptions{})
	st := from.Clone()
	out := make([][]rotation.Move, len(due))
	for i, c := range due {
		for _, m := range a.Maps() {
			var old string
			switch {
			case c.Kind == rotation.KindRotation && m.Kind == gen.KindDungeon && m.Dungeon == c.Dungeon && st.Dates[c.Dungeon] != "":
				old = layout.FloorID(c.Dungeon, st.Dates[c.Dungeon], m.Floor)
			case c.Kind == rotation.KindRegion && m.Kind == gen.KindField && h.regions[m.ID] == c.Region:
				old = m.ID
			default:
				continue
			}
			if to, ok := h.destination(a, m.ID); ok {
				out[i] = append(out[i], rotation.Move{From: old, To: to.Map, X: to.X, Y: to.Y})
			}
		}
		st = c.Next(st)
	}
	return out
}

// changedSpawns gives the maps that do not run their stored spawn lists
// theirs, from the derived keys: the rotating floors of the active dates
// and the fields of changed regions (review decision 47). It drops the
// stored lists of maps that do not run, and makes the host.
func changedSpawns(cfg settings, proc *proceduralWorld, all *gamedata.Mods, log *slog.Logger) error {
	if proc.atlas == nil {
		return nil
	}
	core, err := spawns.LoadCore()
	if err != nil {
		return err
	}
	st, err := cfg.Spawns.Resolve(core.Densities)
	if err != nil {
		return err
	}
	if st.Economy && all.JobTables != nil && all.Items != nil {
		// Changed regions' fields pass the economy checks, as `wactl
		// world new` picks fields (decision 0046).
		ecfg, err := economy.LoadConfig()
		if err != nil {
			return err
		}
		ts, err := economy.LoadTargets()
		if err != nil {
			return err
		}
		w := gamedata.EconomyWorld(ecfg, item.New(all.Items.Data), all.JobTables, all.Tables, all.Rules, all.Monsters)
		st.Rate = gamedata.EconomyRater(w, ts)
	}
	run := proc.atlas.Layout()
	regions := regionsOf(proc.atlas.Base())
	pool := spawns.NewPool(all.SpawnMonsters, all.SpawnThemes)
	var ids []string
	for _, m := range run.Maps {
		if _, _, ok := proc.setup.keys.secretOf(m, regions[m.ID]); ok {
			ids = append(ids, m.ID)
		}
	}
	planned := planChanged(run, proc.warps, proc.set.Get, ids, proc.setup.keys, regions, pool, st)
	out := map[string][]spawns.Spawn{}
	for _, m := range run.Maps {
		if list, ok := planned[m.ID]; ok {
			out[m.ID] = list
		} else if list, ok := proc.spawns[m.ID]; ok && m.Date == "" && m.Season == 0 {
			out[m.ID] = list
		}
	}
	if len(planned) > 0 {
		log.Info("spawns of rotating floors and changed regions computed", "maps", len(planned))
	}
	proc.spawns = out
	proc.host = &changeHost{g: proc.g, public: proc.public, atlas: proc.atlas, set: proc.set, keys: proc.setup.keys,
		regions: regions, pool: pool, settings: st, monsters: all.Monsters, placeNPCs: all.PlaceNPCs, log: log,
		warps: proc.warps, built: map[string]*fields.Field{}, old: map[string][]*zone.Zone{}}
	return nil
}

// startChanges records the changes that came while the server was
// stopped, with the moves of their old maps' points, and makes the
// manager of the later ones. A change the store cannot record stops the
// start, so it happens again at the next one.
func startChanges(ctx context.Context, proc proceduralWorld, pool *pgxpool.Pool, moved *zone.Relocations, log *slog.Logger) (*rotation.Manager, error) {
	h := proc.host
	cs := changeStore{pool: pool}
	stored := rotation.State{State: layout.State{Dates: map[string]string{}, Seasons: maps.Clone(proc.changes.Regions)},
		SeasonsStarted: proc.changes.SeasonsStarted, SeasonEnds: proc.changes.SeasonEnds}
	for _, r := range proc.changes.Rotations {
		if r.ActiveDate != "" {
			stored.Dates[r.Dungeon] = r.ActiveDate
		}
	}
	if proc.setup.state.SeasonsStarted != "" && stored.SeasonsStarted == "" {
		if err := cs.StartSeasons(ctx, proc.setup.state.SeasonsStarted); err != nil {
			return nil, fmt.Errorf("seasons: %w", err)
		}
	}
	for i, moves := range h.startMoves(stored, proc.setup.due) {
		c := proc.setup.due[i]
		for _, mv := range moves {
			// A changed region's fields run again with their IDs: the
			// store moves their points, and later points there stay.
			if _, runs := proc.atlas.Map(mv.From); !runs {
				moved.Add(mv.From, zone.Relocation{Map: mv.To, X: mv.X, Y: mv.Y})
			}
		}
		if err := cs.Commit(ctx, c, moves); err != nil {
			return nil, fmt.Errorf("the world change %s could not be recorded: %w", c.String(), err)
		}
		log.Info("world changed while the server was stopped", "change", c.String(), "moves", len(moves), "first", c.First)
	}
	opts := proc.setup.opts
	opts.Host, opts.Store, opts.Log = h, cs, log
	return rotation.New(opts, proc.setup.state)
}

// background runs a function on its own goroutine until ctx ends, and
// returns a function that waits for it. The wait calls stop first, which
// ends ctx: before it did, a start that failed after the server
// variables began to run, such as one whose event named a map that does
// not run, waited for a stop that only a signal would bring.
func background(ctx context.Context, stop context.CancelFunc, run func(context.Context)) (wait func()) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		run(ctx)
	}()
	return func() {
		stop()
		<-done
	}
}
