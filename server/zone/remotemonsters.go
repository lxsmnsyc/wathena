package zone

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/modscript"
)

// Monsters by game ID and entities of other maps (review decision 59):
//   - Each monster has a game ID, as rAthena's GID: the zone's serial in
//     the high 32 bits, and the monster's serial in the zone in the low
//     ones. maps.find_monster turns one into a handle, as map_id2md, so
//     scripts keep monsters in variables, as $@mobid.
//   - A handle of a monster of another zone reads the monster from the
//     list its zone shares at the end of each step. Its area B8 actions
//     go to its zone as messages, which run after the current step.
//   - A spawn on another map reserves the monsters' serials at once, so
//     it returns their handles, as monster fills $@mobid.
//   - maps.find_entities on another map reads the players, monsters and
//     NPCs that the world and the zone share.

// remoteMonsterBase is the first handle of a monster of another zone.
const remoteMonsterBase = int64(1) << 43

// zoneSerials gives each zone its serial, for game IDs.
var zoneSerials atomic.Int64

// mobSnap is what other zones read of a living monster.
type mobSnap struct {
	serial    int64
	mob       uint32
	name      string
	level     int
	x, y      int
	hp, maxHP int64
}

// gameID is a monster's game ID.
func (z *Zone) gameID(e *entity) int64 { return z.serial<<32 | e.monster.serial }

// splitGameID is a game ID's zone serial and monster serial.
func splitGameID(id int64) (zoneSerial, serial int64) { return id >> 32, id & (1<<32 - 1) }

// zoneBySerial finds a zone of the world by its serial.
func (z *Zone) zoneBySerial(serial int64) *Zone {
	if serial == z.serial {
		return z
	}
	if l, ok := z.World.(ZoneLister); ok {
		for _, o := range l.Zones() {
			if o.serial == serial {
				return o
			}
		}
	}
	return nil
}

// newMonsterSerial is the serial of a monster that spawns: one that a
// spawn of another zone reserved, or a new one.
func (z *Zone) newMonsterSerial() (serial int64, reserved bool) {
	if s := z.presetSerial; s != 0 {
		z.presetSerial = 0
		return s, true
	}
	return z.mobSerials.Add(1), false
}

// reserveSerials reserves n serials of the zone's monsters for a spawn
// that another zone asked for. Any goroutine may call it.
func (z *Zone) reserveSerials(n int) []int64 {
	last := z.mobSerials.Add(int64(n))
	out := make([]int64, n)
	for i := range n {
		out[i] = last - int64(n) + 1 + int64(i)
	}
	return out
}

// publishMonsters copies the zone's living monsters into what other
// zones read, at the end of a step.
func (z *Zone) publishMonsters() {
	s := &z.shared
	s.mu.Lock()
	s.mobs = s.mobs[:0]
	for e := range z.entities.all() {
		m := e.monster
		if m == nil || e.dead {
			continue
		}
		snap := mobSnap{serial: m.serial, mob: m.spawn.Mob, name: m.spawn.Name, x: e.x, y: e.y, hp: max(e.hp, 0)}
		if e.status != nil {
			snap.maxHP, snap.level = int64(e.status.MaxHP), e.status.Level
		}
		s.mobs = append(s.mobs, snap)
	}
	s.mu.Unlock()
}

// sharedMonster reads a living monster of a zone from what it shares.
func (z *Zone) sharedMonster(serial int64) (mobSnap, bool) {
	s := &z.shared
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, m := range s.mobs {
		if m.serial == serial {
			return m, true
		}
	}
	return mobSnap{}, false
}

// remoteMonsterHandle is this zone's handle of a monster of another
// zone, by game ID.
func (z *Zone) remoteMonsterHandle(gid int64) int64 {
	if id, ok := z.remoteMonsterIDs[gid]; ok {
		return id
	}
	if z.remoteMonsters == nil {
		z.remoteMonsters, z.remoteMonsterIDs = map[int64]int64{}, map[int64]int64{}
	}
	id := remoteMonsterBase + int64(len(z.remoteMonsters))
	z.remoteMonsters[id], z.remoteMonsterIDs[gid] = gid, id
	return id
}

func isRemoteMonster(id int64) bool { return id >= remoteMonsterBase && id < remoteMonsterBase<<1 }

// remoteMonster is the zone and serial of a remote monster handle.
func (h scriptHost) remoteMonster(id int64) (*Zone, int64, error) {
	gid, ok := h.z.remoteMonsters[id]
	if !ok {
		return nil, 0, errors.New("not a monster handle")
	}
	zs, serial := splitGameID(gid)
	to := h.z.zoneBySerial(zs)
	if to == nil {
		return nil, 0, errors.New("the monster's map is gone")
	}
	return to, serial, nil
}

// FindMonster is map_id2md for scripts: the handle of a living monster
// of any map by its game ID.
func (h scriptHost) FindMonster(gid int64) (int64, bool) {
	z := h.z
	zs, serial := splitGameID(gid)
	if zs == z.serial {
		e := z.entities.mobs[serial]
		if e == nil || e.dead || !z.entities.has(e) {
			return 0, false
		}
		return z.monsterHandle(e), true
	}
	to := z.zoneBySerial(zs)
	if to == nil {
		return 0, false
	}
	if _, ok := to.sharedMonster(serial); !ok {
		return 0, false
	}
	return z.remoteMonsterHandle(gid), true
}

// remoteMonsterField reads a field of a monster of another zone.
func (h scriptHost) remoteMonsterField(id int64, field string) (script.Value, error) {
	to, serial, err := h.remoteMonster(id)
	if err != nil {
		return script.Nil, err
	}
	m, alive := to.sharedMonster(serial)
	switch field {
	case "kind":
		return script.String("monster"), nil
	case "game_id":
		return script.Int(h.z.remoteMonsters[id]), nil
	case "map":
		return script.String(strings.TrimPrefix(to.Map, "map/")), nil
	case "is_alive":
		return script.Bool(alive), nil
	}
	if _, known := monsterFields[field]; !known {
		return script.Nil, fmt.Errorf("monster has no field %q", field)
	}
	if !alive {
		return script.Nil, errors.New("the monster is gone from its map")
	}
	switch field {
	case "id":
		return script.Int(int64(m.mob)), nil
	case "name":
		return script.String(m.name), nil
	case "level":
		return intValue(m.level), nil
	case "hp":
		return script.Int(m.hp), nil
	case "max_hp":
		return script.Int(m.maxHP), nil
	case "x":
		return intValue(m.x), nil
	case "y":
		return intValue(m.y), nil
	}
	return script.Nil, fmt.Errorf("monster.%s: the monster is on another map: read it on its own map", field)
}

// remoteMonsterAct sends an area B8 action to the zone of a monster of
// another map. It reports true, as the action runs there later.
func (h scriptHost) remoteMonsterAct(id int64, a modscript.MonsterAction) (bool, error) {
	to, serial, err := h.remoteMonster(id)
	if err != nil {
		return false, err
	}
	h.z.postNPC(to, npcMessage{kind: npcZoneMessage, zoneRun: func(t *Zone) {
		e := t.entities.mobs[serial]
		if e == nil || e.dead || !t.entities.has(e) {
			return
		}
		_, _ = scriptHost{t}.MonsterAct(t.monsterHandle(e), a)
	}})
	return true, nil
}

// remoteEntities is maps.find_entities on another zone's map: the
// players of the world's directory, the monsters the zone shares and the
// NPCs of the world's directory, as of the zone's last step.
func (h scriptHost) remoteEntities(to *Zone, q modscript.EntityQuery) ([]modscript.Entity, error) {
	z := h.z
	switch {
	case q.Area == modscript.AreaAround || q.Area == modscript.AreaLine:
		return nil, errors.New("on another map, the area is x and y with radius, a rectangle or cells")
	case q.EnemiesOf != nil || q.AlliesOf != nil || q.LineOfSight || len(q.Exclude) > 0:
		return nil, errors.New("on another map, a query has no enemies_of, allies_of, line_of_sight or exclude")
	}
	zq := &query{radius: q.Radius, circle: q.Circle, cells: q.Cells, x: q.X, y: q.Y}
	switch q.Area {
	case modscript.AreaCell:
		zq.area = areaRadius
	case modscript.AreaRectangle:
		zq.area = areaRect
		zq.x0, zq.y0, zq.x1, zq.y1 = q.Corners[0][0], q.Corners[0][1], q.Corners[1][0], q.Corners[1][1]
	case modscript.AreaCells:
		zq.area = areaCells
	}
	want := func(k string) bool { return len(q.Kinds) == 0 || slices.Contains(q.Kinds, k) }
	type found struct {
		ref    modscript.Entity
		order  int
		key    string
		serial int64
		x, y   int
		hp     int64
	}
	var out []found
	if d := z.playerDir(); d != nil && want("player") {
		d.mu.Lock()
		for _, en := range d.byName {
			if en.zone != to || en.removed {
				continue
			}
			s := en.snap
			if (s.hp > 0) != q.Alive || !z.inArea(zq, s.x, s.y) {
				continue
			}
			out = append(out, found{order: 0, key: en.name, x: s.x, y: s.y, hp: s.hp})
		}
		d.mu.Unlock()
		for i := range out {
			en := d.find(out[i].key)
			if en == nil {
				continue
			}
			out[i].ref = modscript.Entity{Kind: "player", ID: z.handleOf(en)}
		}
	}
	if want("monster") && q.Alive {
		to.shared.mu.Lock()
		for _, m := range to.shared.mobs {
			if z.inArea(zq, m.x, m.y) {
				out = append(out, found{order: 1, serial: m.serial, x: m.x, y: m.y, hp: m.hp})
			}
		}
		to.shared.mu.Unlock()
		for i := range out {
			if out[i].order == 1 {
				out[i].ref = modscript.Entity{Kind: "monster", ID: z.remoteMonsterHandle(to.serial<<32 | out[i].serial)}
			}
		}
	}
	if d := z.npcDir(); d != nil && want("npc") && q.Alive {
		var list []*npcEntry
		d.mu.Lock()
		for _, en := range d.byZone[to] {
			if !en.removed && !en.floating && z.inArea(zq, en.x, en.y) {
				list = append(list, en)
			}
		}
		d.mu.Unlock()
		for _, en := range list {
			_, x, y := d.place(en)
			out = append(out, found{ref: modscript.Entity{Kind: "npc", ID: z.remoteHandle(en)}, order: 2, serial: en.handle, x: x, y: y})
		}
	}
	out = slices.DeleteFunc(out, func(f found) bool { return f.ref.Kind == "" })
	slices.SortFunc(out, func(a, b found) int {
		return cmp.Or(cmp.Compare(a.order, b.order), strings.Compare(a.key, b.key), cmp.Compare(a.serial, b.serial))
	})
	cx, cy := zq.center()
	dist := func(f found) int {
		dx, dy := f.x-cx, f.y-cy
		if q.Circle {
			return dx*dx + dy*dy
		}
		return max(abs(dx), abs(dy))
	}
	switch q.Sort {
	case modscript.SortNearest:
		slices.SortStableFunc(out, func(a, b found) int { return cmp.Compare(dist(a), dist(b)) })
	case modscript.SortFarthest:
		slices.SortStableFunc(out, func(a, b found) int { return cmp.Compare(dist(b), dist(a)) })
	case modscript.SortLowestHP:
		slices.SortStableFunc(out, func(a, b found) int { return cmp.Compare(a.hp, b.hp) })
	case modscript.SortRandom:
		for i := len(out) - 1; i > 0; i-- {
			j := z.rand.IntN(i + 1)
			out[i], out[j] = out[j], out[i]
		}
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	refs := make([]modscript.Entity, len(out))
	for i, f := range out {
		refs[i] = f.ref
	}
	return refs, nil
}
