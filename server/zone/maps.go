package zone

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// Maps, areas and monsters of review decision 58, area B8 (decision
// 0065):
//   - Monsters spawn on any map, with a name, a size, an AI and a label.
//     A spawn on another map runs in its zone after the current step.
//   - A label is rAthena's event label of monster: the NPC's event runs
//     when each monster dies, unless a script killed it without labels,
//     as killmonster's npc_killmonster (mob.cpp 3600).
//   - A zone shares what other zones read of its map: the counts of
//     players and monsters, the monsters of each label, map flags, cell
//     marks, its cells and where its players stand. Another zone reads
//     the state after the zone's last step.
//   - Cell changes of scripts go to the map's players, and to each
//     player who enters, as clif_changemapcell does for setwall.

// mapShared is what other zones read of a zone's map. The counters are
// atomic, and the rest is under mu.
type mapShared struct {
	players  atomic.Int64
	monsters atomic.Int64
	grid     atomic.Pointer[path.Grid]

	mu sync.Mutex
	// labels counts the living monsters of each label. pending counts
	// the monsters that other zones asked this zone to spawn, by label,
	// with "" for every monster, until the zone ran the spawn.
	labels  map[string]int
	pending map[string]int
	// flags are the map flags that scripts set, and marks the cell marks
	// (no_vending and no_buying_store) by cell index.
	flags map[string]bool
	marks map[int]uint8
	// cells are the players' cells at the end of the last step, and mobs
	// the living monsters (remotemonsters.go).
	cells [][2]int
	mobs  []mobSnap
}

// addLabelLocked and addPendingLocked change the counts of a label and of
// the spawns that wait. The caller holds mu.
func (s *mapShared) addLabelLocked(key string, n int) {
	if s.labels == nil {
		s.labels = map[string]int{}
	}
	if s.labels[key] += n; s.labels[key] <= 0 {
		delete(s.labels, key)
	}
}

func (s *mapShared) addPendingLocked(key string, n int) {
	if s.pending == nil {
		s.pending = map[string]int{}
	}
	if s.pending[key] += n; s.pending[key] <= 0 {
		delete(s.pending, key)
	}
}

func (s *mapShared) addPending(key string, n int) {
	s.mu.Lock()
	s.addPendingLocked(key, n)
	s.mu.Unlock()
}

// countMonsters counts the living monsters of a label, or every monster
// for "", with the spawns that wait.
func (s *mapShared) countMonsters(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	// monsters changes under mu too, with the spawns that wait.
	if key == "" {
		return int(s.monsters.Load()) + s.pending[""]
	}
	return s.labels[key] + s.pending[key]
}

// msgNoShopCell is msg_txt 204, for a shop on a cell that refuses it.
const msgNoShopCell = "You can't open a shop on this cell."

// Cell marks of scripts, as rAthena's mapcell bits.
const (
	markNoVending uint8 = 1 << iota
	markNoBuyingStore
)

// monsterLabel is a resolved label of spawned monsters.
type monsterLabel struct {
	key string
	// zone and handle are the NPC's, or nil and 0 for an NPC that does
	// not exist. event is the label in lower case.
	zone   *Zone
	handle int64
	event  string
}

// Effects of a spawned monster's size, as clif_spawn_unit's
// clif_specialeffect (clif.cpp 1737).
const (
	efBabyBody2  = 421
	efGiantBody2 = 423
)

// scriptAI is the zone's AI of rAthena's AI_ value: rAthena numbers
// AI_SPHERE 2 and AI_FLORA 3.
func scriptAI(n int) int {
	switch n {
	case 2:
		return aiSphere
	case 3:
		return aiFlora
	}
	return n
}

// Sizes of spawned monsters, as SZ_MEDIUM and SZ_BIG: rAthena's monster
// command reads size 1 as small.
const (
	sizeSmall = 1
	sizeBig   = 2
)

// sharedCells publishes the players' cells, for count_players with a
// rectangle in other zones.
func (z *Zone) sharedCells() {
	s := &z.shared
	s.mu.Lock()
	s.cells = s.cells[:0]
	for _, p := range z.players {
		if e := p.entity; e != nil && !p.gone() {
			s.cells = append(s.cells, [2]int{e.x, e.y})
		}
	}
	s.mu.Unlock()
	s.players.Store(int64(len(z.players)))
}

// mapZone finds the zone of a map by name, such as prontera.
func (z *Zone) mapZone(name string) (*Zone, error) {
	to := z.zoneOf("map/" + strings.TrimPrefix(name, "map/"))
	if to == nil {
		return nil, fmt.Errorf("the map %s does not run in this world", name)
	}
	return to, nil
}

// LabelOwner finds a label's NPC, as world.get_npc does.
func (h scriptHost) LabelOwner(name, modID string) int64 {
	id, _ := h.FindNPC(name, modID)
	return id
}

// resolveLabel turns a label of the runtime into the zone's.
func (h scriptHost) resolveLabel(l *modscript.MonsterLabel) (*monsterLabel, error) {
	if l == nil {
		return nil, nil
	}
	out := &monsterLabel{key: l.Key, event: l.Event}
	if l.NPC == 0 {
		return out, nil
	}
	t, err := h.npcTarget(l.NPC)
	if err != nil {
		return nil, err
	}
	if t.local != nil {
		out.zone, out.handle = h.z, t.local.npc.handle
	} else {
		out.zone, out.handle = t.entry.zone, t.entry.handle
	}
	return out, nil
}

// spawnSpec is a spawn of spawning.monsters that the target zone runs.
type spawnSpec struct {
	s     modscript.SpawnSpec
	label *monsterLabel
	owner int64
	// serials are the monsters' serials that the asking zone reserved,
	// so it has their handles at once (remotemonsters.go).
	serials []int64
}

// SpawnMonsters places monsters that a script context owns
// (spawning.monsters), as rAthena's monster and areamonster. Monsters
// of another map spawn in its zone after the current step and belong to
// no context. Their serials are reserved now, so the handles come back
// at once, as monster fills $@mobid (script.cpp 11312). A monster that
// finds no cell leaves its handle without a monster.
func (h scriptHost) SpawnMonsters(owner int64, s modscript.SpawnSpec) ([]int64, error) {
	z := h.z
	if z.Mods == nil || z.Mods.Monsters == nil {
		return nil, errors.New("there are no monster records")
	}
	if _, ok := z.Mods.Monsters[s.Monster]; !ok {
		return nil, fmt.Errorf("there is no monster %d", s.Monster)
	}
	label, err := h.resolveLabel(s.Label)
	if err != nil {
		return nil, err
	}
	to := z
	if s.Map != "" {
		if to, err = z.mapZone(s.Map); err != nil {
			return nil, err
		}
	}
	if s.HasCell {
		g := to.shared.grid.Load()
		if g == nil {
			g = to.grid
		}
		if s.X < 0 || s.Y < 0 || s.X >= g.Width || s.Y >= g.Height {
			return nil, fmt.Errorf("%d,%d is outside the map", s.X, s.Y)
		}
		if s.Area == 0 && g.NoPass(s.X, s.Y) {
			return nil, fmt.Errorf("%d,%d is not a walkable cell", s.X, s.Y)
		}
	}
	spec := spawnSpec{s: s, label: label, owner: owner}
	if to == z {
		return z.spawnScripted(spec), nil
	}
	// The counts of the target count the monsters at once, as rAthena's
	// mobcount right after monster does.
	key := ""
	if label != nil {
		key = label.key
	}
	to.shared.addPending("", s.Amount)
	if key != "" {
		to.shared.addPending(key, s.Amount)
	}
	spec.owner = 0
	spec.serials = to.reserveSerials(s.Amount)
	undo := func(t *Zone, n int) {
		t.shared.addPending("", -n)
		if key != "" {
			t.shared.addPending(key, -n)
		}
	}
	if !to.npcIn.post(npcMessage{kind: npcZoneMessage, zoneRun: func(t *Zone) {
		// Each monster that spawns takes its place in the counts from the
		// spawns that wait, in one step (entitySet.add), so another zone
		// never counts it twice or not at all. Those that found no cell
		// leave the counts after.
		t.entities.reservedPlaced = 0
		t.spawnScripted(spec)
		undo(t, s.Amount-t.entities.reservedPlaced)
	}}) {
		undo(to, s.Amount)
		return nil, fmt.Errorf("the zone of %s has too many messages waiting", s.Map)
	}
	ids := make([]int64, len(spec.serials))
	for i, serial := range spec.serials {
		ids[i] = z.remoteMonsterHandle(to.serial<<32 | serial)
	}
	return ids, nil
}

// spawnScripted places the monsters of a spawn in this zone, as
// mob_once_spawn and mob_once_spawn_area.
func (z *Zone) spawnScripted(spec spawnSpec) []int64 {
	s := spec.s
	if z.Mods == nil || z.Mods.Monsters == nil {
		return nil
	}
	tmpl, ok := z.Mods.Monsters[s.Monster]
	if !ok {
		return nil
	}
	sp := tmpl
	sp.Amount, sp.owner = s.Amount, spec.owner
	sp.X, sp.Y, sp.W, sp.H = 0, 0, 0, 0
	sp.label, sp.size, sp.ai = spec.label, s.Size, scriptAI(s.AI)
	if s.DisplayName != nil {
		// mob_once_spawn_sub copies the name as it is; --en-- and --ja--
		// are the monster's own (mob_parse_dataset).
		if n := *s.DisplayName; n != "--en--" && n != "--ja--" {
			sp.Name = n
		}
	}
	if sp.ai != aiNone {
		z.aiMonsters = true
	}
	if sp.ai != aiNone && sp.Reward != nil && sp.ai != aiSphere {
		// mob_dead: only a monster without an AI, or a marine sphere,
		// drops items (alchemist_summon_reward 1). One without a master
		// still gives exp.
		r := *sp.Reward
		r.Drops = nil
		sp.Reward = &r
	}
	switch {
	case s.Rectangle != nil:
		rc := s.Rectangle
		// Monsters that come back use the rectangle's middle and size.
		sp.X, sp.Y = (rc.X1+rc.X2)/2, (rc.Y1+rc.Y2)/2
		sp.W, sp.H = (rc.X2-rc.X1+1)/2, (rc.Y2-rc.Y1+1)/2
	case s.HasCell:
		sp.X, sp.Y, sp.W, sp.H = s.X, s.Y, s.Area, s.Area
		// spawnCell reads a center of 0,0 as anywhere.
		if sp.X == 0 && sp.Y == 0 {
			sp.X = 1
		}
	}
	if s.Respawn {
		// As a spawn line: its delay, or rAthena's default of 5 seconds.
		// mob_setdelayspawn's spawn delay rates are 100% by default, and
		// die keeps the 1 second floor.
		sp.RespawnMs = cmp.Or(s.RespawnMs, modscript.DefaultRespawnMs)
	} else {
		sp.noRespawn = true
	}
	if spec.owner != 0 {
		if z.owned == nil {
			z.owned = map[int64][]*Spawn{}
		}
		z.owned[spec.owner] = append(z.owned[spec.owner], &sp)
	}
	ids := make([]int64, 0, s.Amount)
	// The serials another zone reserved go to the monsters of the spawn,
	// in order. Those of monsters that find no cell stay unused.
	next := 0
	preset := func() {
		if next < len(spec.serials) {
			z.presetSerial = spec.serials[next]
		}
		next++
	}
	defer func() { z.presetSerial = 0 }()
	place := func(x, y int) {
		preset()
		if e := z.spawnAt(&sp, x, y); e != nil && spec.owner != 0 {
			ids = append(ids, z.monsterHandle(e))
		}
	}
	w, h := z.grid.Width, z.grid.Height
	lx, ly := -1, -1
	for range s.Amount {
		switch {
		case s.Rectangle != nil:
			// mob_once_spawn_area (mob.cpp 732): up to the rectangle's
			// cells times 3 tries, at most 1000, for a walkable cell. A
			// failure uses the last good cell, and fails the rest when
			// there is none.
			rc := s.Rectangle
			tries := min((rc.Y2-rc.Y1+1)*(rc.X2-rc.X1+1)*3, 1000)
			var x, y int
			j := 0
			for {
				x, y = z.rndValue(rc.X1, rc.X2), z.rndValue(rc.Y1, rc.Y2)
				j++
				if !z.grid.NoPass(x, y) || j >= tries {
					break
				}
			}
			if j == tries && z.grid.NoPass(x, y) {
				if lx == -1 && ly == -1 {
					return ids
				}
				x, y = lx, ly
			}
			lx, ly = x, y
			// mob_once_spawn_sub: a cell outside the map, or on its first
			// row or column, is a random one.
			if x <= 0 || x >= w || y <= 0 || y >= h {
				var ok bool
				if x, y, ok = z.searchFreeCell(x, y, -1, -1); !ok {
					next++
					continue
				}
			}
			place(x, y)
		case s.HasCell && s.Area == 0:
			place(s.X, s.Y)
		case s.HasCell:
			preset()
			if e := z.spawnOne(&sp); e != nil && spec.owner != 0 {
				ids = append(ids, z.monsterHandle(e))
			}
			z.presetSerial = 0
		default:
			// mob_once_spawn_sub: no cell is a random one, as
			// map_search_freecell with the whole map (map.cpp).
			x, y, ok := z.searchFreeCell(0, 0, -1, -1)
			if !ok {
				next++
				continue
			}
			place(x, y)
		}
	}
	return ids
}

// monsterLabelEvent runs a dead monster's label, as mob_dead's
// npc_event (mob.cpp 3600 to 3609): with the player that killed it, or
// else the player that dealt the most damage (first_sd), on any map, as
// npc_event attaches the player wherever the NPC is. It runs after the
// current step, with the monster counts of the moment it died, so a
// label that reads them sees what rAthena's sees inside mob_dead and
// inside killmonster's loop (script.cpp 11463), where the monsters not
// killed yet still count.
func (z *Zone) monsterLabelEvent(e, killer *entity) {
	m := e.monster
	l := m.label
	if l == nil || l.zone == nil || m.labelOff {
		return
	}
	var p *Player
	if killer != nil && killer.player != nil {
		p = killer.player
	} else {
		p = z.topDamage(e)
	}
	msg := npcMessage{kind: npcEventMessage, handle: l.handle, label: l.event,
		counts: &labelCounts{zone: z, key: l.key, total: z.shared.countMonsters(""), label: z.shared.countMonsters(l.key)}}
	if p != nil {
		if l.zone == z {
			msg.player = int64(p.charID)
		} else {
			// The NPC's zone gets a handle of the player of this map.
			msg.playerName = p.Name
		}
	}
	z.postNPC(l.zone, msg)
}

// labelCounts are the monster counts of a map when a monster of a label
// died: every monster, and those of the label.
type labelCounts struct {
	zone         *Zone
	key          string
	total, label int
}

// topDamage is the player of the zone that dealt a monster the most
// damage, or nil.
func (z *Zone) topDamage(e *entity) *Player {
	var best *Player
	var most int64 = -1
	for _, d := range e.monster.dmglog.Entries {
		p := z.players[d.CharID]
		if p == nil || p.gone() || d.Damage <= most {
			continue
		}
		best, most = p, d.Damage
	}
	return best
}

// scriptKill is status_kill on a monster: it dies with no exp and no
// drops (mob_dead's type 3). Without labels, its label does not run, as
// killmonster's npc_killmonster.
func (z *Zone) scriptKill(e *entity, labels bool) {
	m := e.monster
	if m == nil || e.dead || !z.entities.has(e) {
		return
	}
	m.noRewards = true
	m.labelOff = !labels
	e.hp = 0
	e.hpChanged = true
	z.die(e, nil)
}

// CountPlayers is getmapusers and getareausers.
func (h scriptHost) CountPlayers(mapName string, area *modscript.Rect) (int, bool) {
	to, err := h.z.mapZone(mapName)
	if err != nil {
		return 0, false
	}
	if area == nil {
		if to == h.z {
			return len(h.z.players), true
		}
		return int(to.shared.players.Load()), true
	}
	n := 0
	if to == h.z {
		for _, p := range h.z.players {
			if e := p.entity; e != nil && !p.gone() && area.Contains(e.x, e.y) {
				n++
			}
		}
		return n, true
	}
	to.shared.mu.Lock()
	for _, c := range to.shared.cells {
		if area.Contains(c[0], c[1]) {
			n++
		}
	}
	to.shared.mu.Unlock()
	return n, true
}

// CountMonsters is mobcount: the living monsters of a label, or of the
// map. A monster label's event reads the counts of the moment its
// monster died (monsterLabelEvent).
func (h scriptHost) CountMonsters(mapName, label string) (int, bool) {
	to, err := h.z.mapZone(mapName)
	if err != nil {
		return 0, false
	}
	if v := h.z.countView; v != nil && v.zone == to {
		switch label {
		case "":
			return v.total, true
		case v.key:
			return v.label, true
		}
	}
	return to.shared.countMonsters(label), true
}

// RemoveMonsters is killmonster and killmonsterall, in the map's zone.
func (h scriptHost) RemoveMonsters(mapName string, r modscript.RemoveMonsters) error {
	return h.z.onMap(strings.TrimPrefix(mapName, "map/"), func(z *Zone) {
		var list []*entity
		for e := range z.entities.all() {
			m := e.monster
			if m == nil || e.dead {
				continue
			}
			switch {
			case r.Label != "":
				// buildin_killmonster_sub: the label matches exactly.
				if m.label == nil || m.label.key != r.Label {
					continue
				}
			case r.Scripted:
				// "All": monsters that no spawn line placed (!md->spawn).
				if m.spawn.line {
					continue
				}
			}
			list = append(list, e)
		}
		for _, e := range list {
			z.scriptKill(e, r.RunLabels)
		}
	})
}

// WarpPlayers is mapwarp and areawarp: the players of the map, of a
// rectangle, a party or a guild, go to a cell, to a random walkable cell
// of a rectangle of the target, or with To "random" to a random cell of
// their own map, as areawarp's "Random" (buildin_areawarp_sub's
// pc_randomwarp, script.cpp 5670). Dead players go too: neither
// buildin_areawarp_sub nor mapwarp's party and guild loops check
// pc_isdead (script.cpp 5659 and 14340).
func (h scriptHost) WarpPlayers(mapName string, w modscript.WarpPlayers) error {
	z := h.z
	var dest *Zone
	if w.To != "random" {
		var err error
		if dest, err = z.mapZone(w.To); err != nil {
			return err
		}
	}
	return z.onMap(strings.TrimPrefix(mapName, "map/"), func(z *Zone) {
		for _, p := range z.playersInOrder() {
			e := p.entity
			switch {
			case e == nil:
				continue
			case w.Area != nil && !w.Area.Contains(e.x, e.y):
				continue
			case w.Party != 0 && p.partyID() != w.Party:
				continue
			case w.Guild != 0 && p.guildID() != w.Guild:
				continue
			}
			if dest == nil {
				if x, y, ok := z.randomCell(); ok {
					z.scriptWarpDead(p, z, x, y)
				}
				continue
			}
			x, y := w.X, w.Y
			if w.ToArea != nil {
				x, y = z.randomIn(dest, *w.ToArea)
			}
			z.scriptWarpDead(p, dest, x, y)
		}
	})
}

// randomIn is buildin_areawarp_sub's cell of a rectangle of a map: up to
// its cells times 3 tries, at most 1000, for a walkable cell. The last
// try is used when none is.
func (z *Zone) randomIn(dest *Zone, r modscript.Rect) (int, int) {
	g := dest.shared.grid.Load()
	if g == nil {
		g = dest.grid
	}
	tries := min((r.Y2-r.Y1+1)*(r.X2-r.X1+1)*3, 1000)
	var x, y int
	for j := 0; j < tries; j++ {
		x, y = z.rndValue(r.X1, r.X2), z.rndValue(r.Y1, r.Y2)
		if !g.NoPass(x, y) {
			break
		}
	}
	return x, y
}

// scriptWarp warps a player of the zone once no script runs, as
// pc_setpos: a talk ends, and a cell that is not on the map is a random
// one (arrivalCell). A dead player stays.
func (z *Zone) scriptWarp(p *Player, to *Zone, x, y int) {
	p.warpTo = &scriptWarp{to: to, x: x, y: y}
	z.later(func() { z.afterTalk(p) })
}

// scriptWarpDead is scriptWarp for the warps that move dead players too,
// as pc_setpos does when its caller does not check pc_isdead: the
// player arrives dead and returns to its save point after the rest of
// its wait.
func (z *Zone) scriptWarpDead(p *Player, to *Zone, x, y int) {
	p.warpTo = &scriptWarp{to: to, x: x, y: y, dead: true}
	z.later(func() { z.afterTalk(p) })
}

// PartyWarp is warpparty (script.cpp 5781): the members of a party that
// live, on every map or only on From, go to a map and a cell, with a
// random cell within a range of it, or to one of warpparty's targets.
// buildin_warpparty skips dead members (script.cpp 5870). Map flags that
// refuse warps, and pc_job_can_entermap, do not exist yet.
func (h scriptHost) PartyWarp(party int64, w modscript.PartyWarp) error {
	z := h.z
	info, err := h.partyInfo(party)
	if err != nil {
		return err
	}
	var dest *Zone
	x, y := w.X, w.Y
	caller := ""
	switch w.Target {
	case "":
		if dest, err = z.mapZone(w.Map); err != nil {
			return err
		}
	case "save_point", "random_all":
		// The attached player's save point, or a random cell of its map.
		p, err := h.player(w.Player)
		if err != nil {
			return fmt.Errorf("player: %w", err)
		}
		caller = p.Name
		if w.Target == "save_point" {
			if dest, x, y = z.savePoint(p); dest == nil {
				return nil
			}
			break
		}
		dest = z
		var ok bool
		if x, y, ok = z.randomAllCell(); !ok {
			// buildin_warpparty: "moving player failed".
			return nil
		}
	case "leader":
		// The leader's map and cell. A leader who is offline moves nobody.
		d := z.playerDir()
		if !info.LeaderOnline {
			return nil
		}
		var en *playerEntry
		if d != nil {
			en = d.find(info.Leader)
		}
		if en == nil {
			p := z.playerByName(info.Leader)
			if p == nil {
				return nil
			}
			dest, x, y = z, p.entity.x, p.entity.y
			break
		}
		var s playerSnap
		var ok bool
		if dest, _, s, ok = d.read(en); !ok {
			return nil
		}
		x, y = s.x, s.y
		if p := z.playerByName(info.Leader); p != nil {
			x, y = p.entity.x, p.entity.y
		}
	}
	z.everyZone(func(z *Zone) {
		for _, p := range z.playersInOrder() {
			if p.partyID() != party || p.entity == nil || p.entity.dead {
				continue
			}
			if w.From != "" && strings.TrimPrefix(z.Map, "map/") != w.From {
				continue
			}
			switch w.Target {
			case "random":
				if rx, ry, ok := z.randomCell(); ok {
					z.scriptWarp(p, z, rx, ry)
				}
				continue
			case "save_point_all":
				if to, sx, sy := z.savePoint(p); to != nil {
					z.scriptWarp(p, to, sx, sy)
				}
				continue
			case "save_point":
				z.scriptWarp(p, dest, x, y)
				continue
			case "leader":
				if p.Name == info.Leader {
					continue
				}
			case "random_all":
				if p.Name == caller {
					z.scriptWarp(p, dest, x, y)
					continue
				}
			}
			nx, ny := x, y
			if w.RangeX != 0 || w.RangeY != 0 {
				nx, ny = z.partyRangeCell(dest, x, y, w.RangeX, w.RangeY)
			}
			z.scriptWarp(p, dest, nx, ny)
		}
	})
	return nil
}

// partyRangeCell is buildin_warpparty's cell within a range (script.cpp
// 5903): up to 10 tries for a walkable cell, and the cell itself when the
// tries run out, as the loop's count reaches 0 on its tenth try whatever
// that try found.
func (z *Zone) partyRangeCell(dest *Zone, x, y, rx, ry int) (int, int) {
	g := dest.shared.grid.Load()
	if g == nil || dest == z {
		g = dest.grid
	}
	attempts := 10
	var nx, ny int
	for {
		nx, ny = x+z.rndValue(-rx, rx), y+z.rndValue(-ry, ry)
		attempts--
		if attempts <= 0 || nx >= 0 && ny >= 0 && nx < g.Width && ny < g.Height && !g.NoPass(nx, ny) {
			break
		}
	}
	if attempts == 0 {
		return x, y
	}
	return nx, ny
}

// randomAllCell is warpparty's RandomAll cell (script.cpp 5840): up to
// 1000 tries of a cell from 1 to the map's size less one, which units
// can walk on and is not next to a warp (teleport_on_portal is off). The
// loop's count fails a cell found on a later try.
func (z *Zone) randomAllCell() (int, int, bool) {
	w, h := z.grid.Width, z.grid.Height
	for range 1000 {
		x, y := z.rndValue(1, w-1), z.rndValue(1, h-1)
		if x < w && y < h && !z.grid.NoPass(x, y) && !z.warpNear(x, y, 1) {
			return x, y, true
		}
	}
	return 0, 0, false
}

// AnnounceArea is areaannounce: the line goes to the players of a
// rectangle of the map.
func (h scriptHost) AnnounceArea(mapName, mod, text string, area modscript.Rect) error {
	return h.z.onMap(strings.TrimPrefix(mapName, "map/"), func(z *Zone) {
		for _, p := range z.playersInOrder() {
			if e := p.entity; e != nil && area.Contains(e.x, e.y) {
				p.pending = append(p.pending, wathenav1.Event_builder{Chat: wathenav1.ChatMessage_builder{
					Channel: wathenav1.ChatChannel_CHAT_CHANNEL_ANNOUNCEMENT,
					Text:    text,
					FromMod: mod,
				}.Build()}.Build())
			}
		}
	})
}

// cellRefusals are the cell kinds that scripts cannot set or check, and
// why: their systems do not exist, or keep no cell flag.
var cellRefusals = map[string]string{
	"water":          "the server's map data has no water cells yet",
	"basilica":       "wAthena's Basilica is a status of its caster, not a cell flag, and battlegrounds do not exist",
	"land_protector": "Land Protector does not exist",
	"no_chat":        "chat rooms do not exist",
	"maelstrom":      "Maelstrom does not exist",
	"ice_wall":       "wAthena's Ice Wall is a skill unit, not a cell flag",
}

// SetCells is setcell, in the map's zone. Walkable and shootable change
// the map's cells. no_vending and no_buying_store refuse shops on the
// cells. rAthena's setcell sends nothing to the players (script.cpp
// 20631; map_setcell, map.cpp 3404, has no clif call), so their clients
// keep the old cells.
// With send, the map's players get the cells at once and players who
// enter get them then, as setwall's map_iwall_set and map_iwall_get send
// a wall's cells (clif_changemapcell, map.cpp 3511 and 3538).
func (h scriptHost) SetCells(mapName string, area modscript.Rect, kind string, on, send bool) error {
	if why, ok := cellRefusals[kind]; ok {
		return fmt.Errorf("%s cells cannot be set: %s", kind, why)
	}
	if kind == "npc" {
		return errors.New("npc cells cannot be set: wAthena's touch areas read their NPCs, not a cell flag")
	}
	return h.z.onMap(strings.TrimPrefix(mapName, "map/"), func(z *Zone) { z.setCells(area, kind, on, send) })
}

func (z *Zone) setCells(area modscript.Rect, kind string, on, send bool) {
	w, h := z.grid.Width, z.grid.Height
	x1, y1 := max(area.X1, 0), max(area.Y1, 0)
	x2, y2 := min(area.X2, w-1), min(area.Y2, h-1)
	if x1 > x2 || y1 > y2 {
		return
	}
	switch kind {
	case "walkable", "shootable":
		bit := uint8(path.Walkable)
		if kind == "shootable" {
			bit = path.Shootable
		}
		var changed []uint32
		for y := y1; y <= y2; y++ {
			for x := x1; x <= x2; x++ {
				i := x + y*w
				old := z.grid.Cells[i]
				v := old &^ bit
				if on {
					v |= bit
				}
				if v == old {
					continue
				}
				z.ensureOwnGrid()
				z.grid.Cells[i] = v
				if send {
					if z.cellChanges == nil {
						z.cellChanges = map[int]uint8{}
					}
					z.cellChanges[i] = v
				}
				changed = append(changed, packCell(x, y, v))
			}
		}
		if len(changed) == 0 {
			return
		}
		g := *z.grid
		g.Cells = slices.Clone(z.grid.Cells)
		z.shared.grid.Store(&g)
		if !send {
			return
		}
		for _, p := range z.players {
			p.pending = append(p.pending, cellEvents(changed)...)
		}
	case "no_vending", "no_buying_store":
		bit := markNoVending
		if kind == "no_buying_store" {
			bit = markNoBuyingStore
		}
		s := &z.shared
		s.mu.Lock()
		if s.marks == nil {
			s.marks = map[int]uint8{}
		}
		for y := y1; y <= y2; y++ {
			for x := x1; x <= x2; x++ {
				i := x + y*w
				v := s.marks[i] &^ bit
				if on {
					v |= bit
				}
				if v == 0 {
					delete(s.marks, i)
				} else {
					s.marks[i] = v
				}
			}
		}
		s.mu.Unlock()
	}
}

// ensureOwnGrid copies the zone's grid before a change, as the map's
// grid may be shared with other zones.
func (z *Zone) ensureOwnGrid() {
	if z.ownGrid {
		return
	}
	g := *z.grid
	g.Cells = slices.Clone(z.grid.Cells)
	z.grid = &g
	z.ownGrid = true
}

// cellMark reports whether a cell of the zone has a mark.
func (z *Zone) cellMark(x, y int, bit uint8) bool {
	s := &z.shared
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.marks[x+y*z.grid.Width]&bit != 0
}

// packCell packs a cell of the MapCells event.
func packCell(x, y int, flags uint8) uint32 {
	return uint32(x&0x3FF) | uint32(y&0x3FF)<<10 | uint32(flags&(path.Walkable|path.Shootable))<<20 // #nosec G115 -- masked.
}

// maxCellsPerEvent is the most cells a MapCells event holds.
const maxCellsPerEvent = 4096

// cellEvents are MapCells events of packed cells.
func cellEvents(cells []uint32) []*wathenav1.Event {
	var out []*wathenav1.Event
	for len(cells) > 0 {
		n := min(len(cells), maxCellsPerEvent)
		out = append(out, wathenav1.Event_builder{MapCells: wathenav1.MapCells_builder{Cells: slices.Clone(cells[:n])}.Build()}.Build())
		cells = cells[n:]
	}
	return out
}

// changedCells are the events of every cell that scripts changed, for a
// player who enters the map, as map_iwall_get.
func (z *Zone) changedCells() []*wathenav1.Event {
	if len(z.cellChanges) == 0 {
		return nil
	}
	keys := make([]int, 0, len(z.cellChanges))
	for i := range z.cellChanges {
		keys = append(keys, i)
	}
	sort.Ints(keys)
	cells := make([]uint32, len(keys))
	for n, i := range keys {
		cells[n] = packCell(i%z.grid.Width, i/z.grid.Width, z.cellChanges[i])
	}
	return cellEvents(cells)
}

// CheckCell is checkcell (map_getcellp, map.cpp 3318).
func (h scriptHost) CheckCell(mapName string, x, y int, kind string) (bool, error) {
	if why, ok := cellRefusals[kind]; ok {
		return false, fmt.Errorf("%s cells cannot be read: %s", kind, why)
	}
	to, err := h.z.mapZone(mapName)
	if err != nil {
		return false, err
	}
	g := to.shared.grid.Load()
	if to == h.z || g == nil {
		g = to.grid
	}
	inside := x >= 0 && x < g.Width-1 && y >= 0 && y < g.Height-1
	var flags uint8
	if inside {
		flags = g.Cells[x+y*g.Width]
	}
	walkable, shootable := flags&path.Walkable != 0, flags&path.Shootable != 0
	switch kind {
	case "pass", "reach":
		return inside && walkable, nil
	case "no_pass", "no_reach":
		return !inside || !walkable, nil
	case "wall":
		return inside && !walkable && !shootable, nil
	case "cliff":
		return inside && !walkable && shootable, nil
	case "stack":
		// CELL_NOSTACK is off by default.
		return false, nil
	case "npc":
		if to != h.z {
			return false, errors.New("npc cells of another map cannot be read: touch areas are not shared between zones")
		}
		for _, n := range h.z.touchNPCs {
			if !n.npc.removed && n.npc.touches(n, x, y) {
				return true, nil
			}
		}
		return false, nil
	case "no_vending":
		return inside && to.cellMark(x, y, markNoVending), nil
	case "no_buying_store":
		return inside && to.cellMark(x, y, markNoBuyingStore), nil
	}
	return false, fmt.Errorf("unknown cell kind %q", kind)
}

// mapFlagRefusals are rAthena's map flags that scripts cannot set, and
// why. nomemo is the one map flag wAthena has.
var mapFlagRefusals = map[string]string{}

func init() {
	refuse := func(why string, flags ...string) {
		for _, f := range flags {
			mapFlagRefusals[f] = why
		}
	}
	refuse("PvP is not ported", "pvp", "pvp_noparty", "pvp_noguild", "pvp_nightmaredrop", "pvp_nocalcrank")
	refuse("sieges do not exist", "gvg", "gvg_noparty", "gvg_castle", "gvg_dungeon", "gvg_te", "gvg_te_castle")
	refuse("battlegrounds do not exist", "battleground")
	refuse("the client draws no weather", "snow", "fog", "sakura", "leaves", "rain", "clouds", "clouds2", "fireworks")
	refuse("day and night do not exist", "nightenabled")
	refuse("chat rooms do not exist", "nochat")
	refuse("private airships do not exist", "privateairship_source", "privateairship_destination")
	refuse("the bank does not exist", "nobank")
	refuse("the cash shop window does not exist", "nocashshop")
	refuse("pets do not exist", "nopetcapture")
	refuse("dynamic NPCs do not exist", "nodynamicnpc")
	refuse("the macro checker does not exist", "nomacrochecker")
	refuse("the special popup does not exist", "specialpopup")
	refuse("map channels do not exist", "nomapchannelautojoin")
	refuse("the flag's effect is not built yet", "noteleport", "nosave", "nobranch", "nopenalty", "nozenypenalty",
		"notrade", "noskill", "nowarp", "partylock", "noicewall", "nogo", "nobaseexp", "nojobexp", "nomobloot",
		"nomvploot", "noreturn", "nowarpto", "restricted", "nocommand", "nodrop", "jexp", "bexp", "novending",
		"loadevent", "noexppenalty", "guildlock", "town", "autotrade", "allowks", "monster_noteleport", "reset",
		"nousecart", "noitemconsumption", "nosunmoonstarmiracle", "forcemineffect", "nolockon", "notomb",
		"skill_damage", "nocostume", "hidemobhpbar", "noloot", "noexp", "skill_duration", "norodex",
		"norenewalexppenalty", "norenewaldroppenalty", "nobuyingstore", "invincible_time")
}

// checkFlag reads a map flag's name: nil for nomemo, an error for one
// that cannot be set.
func checkFlag(flag string) (refused string, err error) {
	if flag == "nomemo" {
		return "", nil
	}
	if why, ok := mapFlagRefusals[flag]; ok {
		return why, nil
	}
	return "", fmt.Errorf("%q is not a map flag; the flags are rAthena's names without MF_, such as nomemo", flag)
}

// SetMapFlag is setmapflag and removemapflag, in the map's zone.
func (h scriptHost) SetMapFlag(mapName, flag string, on bool) error {
	why, err := checkFlag(flag)
	if err != nil {
		return err
	}
	if why != "" {
		return fmt.Errorf("the map flag %s cannot be set: %s", flag, why)
	}
	if _, err := h.z.mapZone(mapName); err != nil {
		return err
	}
	return h.z.onMap(strings.TrimPrefix(mapName, "map/"), func(z *Zone) {
		s := &z.shared
		s.mu.Lock()
		if s.flags == nil {
			s.flags = map[string]bool{}
		}
		s.flags[flag] = on
		s.mu.Unlock()
	})
}

// MapFlag is getmapflag. A flag that cannot be set is off.
func (h scriptHost) MapFlag(mapName, flag string) (bool, error) {
	why, err := checkFlag(flag)
	if err != nil {
		return false, err
	}
	to, err := h.z.mapZone(mapName)
	if err != nil {
		return false, err
	}
	if why != "" {
		return false, nil
	}
	return to.mapFlag(flag), nil
}

// mapFlag is a map flag of the zone: what a script set, or else the
// world layout's. Other zones may call it.
func (z *Zone) mapFlag(flag string) bool {
	s := &z.shared
	s.mu.Lock()
	on, set := s.flags[flag]
	s.mu.Unlock()
	if set {
		return on
	}
	if flag == "nomemo" {
		if m, ok := z.atlas().Map(z.Map); ok {
			return m.NoMemo
		}
	}
	return false
}

// FindFreeCell is getfreecell's map_search_freecell with flag 1 (map.cpp
// 2790): 50 tries for a walkable cell within the range of the center, or
// anywhere away from the map's edges, never the center itself. With
// From, the cell must be reachable by a direct walk, as flag 2's
// unit_can_reach_pos.
func (h scriptHost) FindFreeCell(mapName string, q modscript.FreeCell) (int, int, bool, error) {
	z := h.z
	to, err := z.mapZone(mapName)
	if err != nil {
		return 0, 0, false, err
	}
	g := to.shared.grid.Load()
	if to == z || g == nil {
		g = to.grid
	}
	var from *entity
	if q.From != nil {
		if from, err = h.entityOf(*q.From); err != nil {
			return 0, 0, false, err
		}
		if to != z {
			return 0, 0, false, errors.New("reachable_from must be on the map")
		}
	}
	reach := func(x, y int) bool {
		if g.NoPass(x, y) {
			return false
		}
		if from == nil {
			return true
		}
		_, ok := path.Search(g, from.x, from.y, x, y, true)
		return ok
	}
	if q.RangeX == 0 && q.RangeY == 0 {
		// No range: the center itself.
		return q.X, q.Y, reach(q.X, q.Y), nil
	}
	w, hh := g.Width, g.Height
	edgeValid := min(mapEdgeSize, 5)
	for range 50 {
		var x, y int
		if q.RangeX >= 0 {
			x = z.rndValue(q.X-q.RangeX, q.X+q.RangeX)
		} else {
			x = z.rndValue(mapEdgeSize, w-mapEdgeSize-1)
		}
		if q.RangeY >= 0 {
			y = z.rndValue(q.Y-q.RangeY, q.Y+q.RangeY)
		} else {
			y = z.rndValue(mapEdgeSize, hh-mapEdgeSize-1)
		}
		if x == q.X && y == q.Y {
			continue
		}
		if x < edgeValid || x > w-edgeValid || y < edgeValid || y > hh-edgeValid {
			continue
		}
		if x < w && y < hh && reach(x, y) {
			return x, y, true, nil
		}
	}
	return q.X, q.Y, false, nil
}

// monsterOf finds a monster of the zone that lives, by its handle.
func (h scriptHost) monsterOf(id int64) (*entity, error) {
	e := h.z.monsterHandles[id]
	if e == nil || e.dead || !h.z.entities.has(e) {
		return nil, errors.New("the monster is gone from this map")
	}
	return e, nil
}

// MonsterAct runs the monster methods of area B8.
func (h scriptHost) MonsterAct(id int64, a modscript.MonsterAction) (bool, error) {
	z := h.z
	if isRemoteMonster(id) {
		return h.remoteMonsterAct(id, a)
	}
	e, err := h.monsterOf(id)
	if err != nil {
		return false, err
	}
	m := e.monster
	switch a.Op {
	case "kill", "remove":
		// unitkill runs the label; killmonster's default does not.
		z.scriptKill(e, a.Op == "kill")
		return true, nil
	case "set_damage_immune":
		// UMOB_DMGIMMUNE: ud.immune_attack, which battle_check_target
		// reads (battle.cpp 7928).
		m.immune = a.On
		return true, nil
	}
	if e.status == nil {
		return false, errors.New("the monster has no stats")
	}
	own := func() {
		st := *e.status
		e.status = &st
	}
	switch a.Op {
	case "set_hp":
		// UMOB_HP's status_set_hp (status.cpp 1299): 0 does nothing, and
		// the HP is at most the maximum, so it never kills.
		if a.Value < 1 {
			return false, nil
		}
		e.hp = min(a.Value, int64(e.status.MaxHP))
		e.hpChanged = true
		return true, nil
	case "set_modes":
		// UMOB_MODE replaces the base modes.
		own()
		e.status.Mode = a.Modes
		return true, nil
	case "set_speed":
		// UMOB_SPEED: the next step walks at the new speed.
		own()
		e.status.Speed = int(a.Value)
		e.speed = int(a.Value)
		return true, nil
	case "use_skill_at":
		return z.monsterSkillAt(e, a.Skill, a.Level, a.X, a.Y)
	case "warp_to":
		// unitwarp on the monster's own map.
		return z.unitWarp(e, a.X, a.Y), nil
	}
	return false, fmt.Errorf("unknown monster action %q", a.Op)
}

// monsterSkillAt is unitskillusepos: a ground skill on a cell, with its
// own cast time. Other skills fail.
func (z *Zone) monsterSkillAt(e *entity, skillID, level int64, x, y int) (bool, error) {
	if z.Skills == nil {
		return false, errors.New("this zone has no skills")
	}
	d := z.Skills.Get(int(skillID))
	if d == nil {
		return false, fmt.Errorf("there is no skill %d", skillID)
	}
	if d.Target != skill.TargetGround {
		return false, nil
	}
	lv := int(level)
	e.monster.skillIdx = -1
	o := &castOverride{ms: d.CastTime.At(lv), cancel: d.CastCancel}
	return z.skillUsePosWith(e, d, lv, x, y, o), nil
}

// MonsterInfo is getmonsterinfo: a monster's record, with the exp rates
// as rAthena applies them when it loads mob_db.
func (h scriptHost) MonsterInfo(id uint32) (modscript.MonsterInfo, bool) {
	z := h.z
	if z.Mods == nil || z.Mods.Monsters == nil {
		return modscript.MonsterInfo{}, false
	}
	s, ok := z.Mods.Monsters[id]
	if !ok || s.Status == nil {
		return modscript.MonsterInfo{}, false
	}
	st := s.Status
	in := modscript.MonsterInfo{
		ID: id, Name: s.Name, Level: st.Level, HP: st.MaxHP, SP: st.MaxSP,
		AttackMin: st.RHW.Atk, AttackMax: st.RHW.Atk2, MagicAttackMin: st.MAtkMin, MagicAttackMax: st.MAtkMax,
		Defense: st.Def, MagicDefense: st.MDef, Resistance: st.Res, MagicResistance: st.MRes,
		Str: st.Str, Agi: st.Agi, Vit: st.Vit, Int: st.Int, Dex: st.Dex, Luk: st.Luk,
		Speed: st.Speed, AttackRange: st.RHW.Range, SkillRange: s.ViewRange, Chase: s.ChaseRange,
		Size: st.Size, Race: st.Race, Element: st.DefEle, ElementLevel: st.EleLv, Modes: st.Mode,
		Damage: cmp.Or(st.DamageTaken, 100),
	}
	if r := s.Reward; r != nil {
		in.BaseExp, in.JobExp, in.MvpExp = r.BaseExp, r.JobExp, r.MvpExp
	}
	return in, true
}

var _ modscript.MapHost = scriptHost{}
