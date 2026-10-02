package zone

import (
	"slices"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Ground skill units: rAthena's skill_unit_group and skill_unit. A
// ground skill places a group of units, one per cell of its layout. Each
// unit is an entity, so viewers see it. A timer every 100 ms
// (skill_unit_timer) ends units whose time ran out and runs the effect of
// units with an interval on the units around them. Units without an
// interval, such as Safety Wall and Pneuma, act when a unit enters or
// leaves their cell (skill_unit_move).

// unitTimerInterval is SKILLUNITTIMER_INTERVAL.
const unitTimerInterval = 100

// Battle check flags of units, as rAthena's BCT_ values.
const (
	bctSelf    = 0x010000
	bctEnemy   = 0x020000
	bctParty   = 0x040000
	bctGuild   = 0x080000 | 0x200000
	bctNeutral = 0x100000
	bctAll     = 0x3F0000
)

// unitGroup is skill_unit_group.
type unitGroup struct {
	id  int64
	src *entity
	def *skill.Def
	lv  int
	// unitID is the UNT_ ID that picks the client's effect.
	unitID int
	// start is the group's tick, and limit its lifetime in milliseconds.
	start, limit int64
	// interval is how often a unit acts on a target, or -1 for units that
	// act when a target enters or leaves their cell.
	interval   int
	targetFlag int
	// val2 and val3 are the group's values: Safety Wall's hits and, in
	// renewal, its durability; Fire Wall's hits per cell.
	val2  int
	val3  int64
	units []*entity
	// deleted is set once the group is gone.
	deleted bool
}

// skillUnit is skill_unit: one cell of a group.
type skillUnit struct {
	group *unitGroup
	// val2 is the unit's own value: the hits left on a Fire Wall cell.
	val2 int
	// rng is the unit's range: the area around its cell it acts on.
	rng int
	// limit is the unit's own lifetime from the group's start.
	limit int64
	// hidden units act but are not shown, as the cells of a
	// RangedSingleUnit group besides its center.
	hidden bool
}

// unitLayout is skill_get_unit_layout: the cell offsets of a group. A
// layout of n is the square of radius n around the cell. Fire Wall's
// layout (-1) depends on the direction from the caster.
func unitLayout(d *skill.Def, lv int, src *entity, x, y int) [][2]int {
	n := d.Unit.Layout.At(lv)
	if n >= 0 {
		var cells [][2]int
		for dy := -n; dy <= n; dy++ {
			for dx := -n; dx <= n; dx++ {
				cells = append(cells, [2]int{dx, dy})
			}
		}
		return cells
	}
	if d.ID != skill.IDFireWall {
		return [][2]int{{0, 0}}
	}
	dir := 6
	if src.x != x || src.y != y {
		dir = mapCalcDir(src, x, y)
	}
	switch dir {
	case 0, 4:
		return [][2]int{{-1, 0}, {0, 0}, {1, 0}}
	case 2, 6:
		return [][2]int{{0, -1}, {0, 0}, {0, 1}}
	case 1, 5:
		return [][2]int{{1, 1}, {1, 0}, {0, 0}, {0, -1}, {-1, -1}}
	}
	return [][2]int{{-1, 1}, {-1, 0}, {0, 0}, {0, -1}, {1, -1}}
}

// unitFlag reports whether the skill's unit has a UF_ flag, by name.
func unitFlag(d *skill.Def, name string) bool {
	return d.Unit != nil && slices.Contains(d.Unit.Flags, name)
}

// unitSetting is skill_unitsetting: it places a skill's group at a cell,
// and returns it, or nil when no cell could hold a unit.
func (z *Zone) unitSetting(src *entity, d *skill.Def, lv, x, y int) *unitGroup {
	if d.Unit == nil {
		return nil
	}
	g := &unitGroup{src: src, def: d, lv: lv, unitID: d.Unit.ID, start: z.now,
		limit: int64(d.Duration1.At(lv)), interval: d.Unit.Interval, targetFlag: d.Unit.Target}
	rng := d.Unit.Range.At(lv)
	switch d.ID {
	case skill.IDSafetyWall:
		g.val2 = lv + 1
		if z.renewal() && src.status != nil {
			level := z.unit(src).Level()
			g.val3 = int64(300*lv + 65*(src.status.Int+level) + src.status.MaxSP)
		}
	case skill.IDFireWall:
		if src.sc.Has(scViolentgale) {
			g.limit = g.limit * 3 / 2
		}
		g.val2 = 4 + lv
	}
	z.nextGroup++
	g.id = z.nextGroup
	single := unitFlag(d, "RangedSingleUnit")
	for _, off := range unitLayout(d, lv, src, x, y) {
		ux, uy := x+off[0], y+off[1]
		if ux <= 0 || uy <= 0 || ux >= z.grid.Width || uy >= z.grid.Height || z.grid.NoPass(ux, uy) {
			continue
		}
		if unitFlag(d, "PathCheck") && !z.clearShot(ux, uy, src.x, src.y) {
			continue
		}
		u := &skillUnit{group: g, rng: rng, limit: g.limit, hidden: single && (off[0] != 0 || off[1] != 0)}
		if d.ID == skill.IDFireWall {
			u.val2 = g.val2
		}
		e := &entity{unit: u, x: ux, y: uy}
		z.entities.add(e)
		g.units = append(g.units, e)
	}
	if len(g.units) == 0 {
		return nil
	}
	src.groups = append(src.groups, g)
	z.groups[g.id] = g
	// A unit of range 0 acts at once on whoever stands on its cell.
	if rng == 0 && g.interval == -1 {
		for _, t := range z.unitsOnCells(g) {
			z.unitStatusRefresh(t)
		}
	}
	return g
}

// unitsOnCells are the characters on a group's cells.
func (z *Zone) unitsOnCells(g *unitGroup) []*entity {
	var out []*entity
	for e := range z.entities.all() {
		if (e.player == nil && e.monster == nil) || e.dead {
			continue
		}
		for _, u := range g.units {
			if u.x == e.x && u.y == e.y {
				out = append(out, e)
				break
			}
		}
	}
	return out
}

// deleteUnit is skill_delunit: a unit ends. Its group ends with its last
// unit.
func (z *Zone) deleteUnit(e *entity) {
	u := e.unit
	if u == nil || !z.entities.has(e) {
		return
	}
	g := u.group
	z.entities.remove(e)
	z.removed = append(z.removed, removal{e, wathenav1.VanishReason_VANISH_REASON_EXPIRED})
	g.units = slices.DeleteFunc(g.units, func(x *entity) bool { return x == e })
	// skill_unit_onout for whoever stands on the cell.
	for o := range z.entities.all() {
		if o.x == e.x && o.y == e.y && (o.player != nil || o.monster != nil) {
			z.unitStatusRefresh(o)
		}
	}
	if len(g.units) == 0 {
		z.deleteGroup(g)
	}
}

// deleteGroup is skill_delunitgroup: every unit of a group ends.
func (z *Zone) deleteGroup(g *unitGroup) {
	if g.deleted {
		return
	}
	g.deleted = true
	delete(z.groups, g.id)
	g.src.groups = slices.DeleteFunc(g.src.groups, func(x *unitGroup) bool { return x == g })
	for _, e := range slices.Clone(g.units) {
		z.deleteUnit(e)
	}
}

// clearUnitGroups is skill_clear_unitgroup: a unit's groups end, as when
// it dies or leaves the map.
func (z *Zone) clearUnitGroups(e *entity) {
	for _, g := range slices.Clone(e.groups) {
		z.deleteGroup(g)
	}
}

// advanceUnits is skill_unit_timer: every 100 ms, units whose time ran
// out end, then the units with an interval act on the units in range.
func (z *Zone) advanceUnits() {
	if len(z.groups) == 0 {
		z.nextUnitTimer = z.now + unitTimerInterval
		return
	}
	for z.now >= z.nextUnitTimer {
		z.nextUnitTimer += unitTimerInterval
		if z.nextUnitTimer <= z.now-unitTimerInterval {
			// A zone that slept skips the missed passes.
			z.nextUnitTimer = z.now + unitTimerInterval
		}
		z.unitTimer()
	}
}

// unitTimer is one pass of skill_unit_timer_sub over every unit.
func (z *Zone) unitTimer() {
	var units []*entity
	for e := range z.entities.all() {
		if e.unit != nil {
			units = append(units, e)
		}
	}
	for _, e := range units {
		if !z.entities.has(e) {
			continue
		}
		u := e.unit
		g := u.group
		age := z.now - g.start
		if age >= g.limit || age >= u.limit {
			z.deleteUnit(e)
			continue
		}
		if u.rng < 0 || g.interval == -1 {
			continue
		}
		q := &query{area: areaRadius, x: e.x, y: e.y, radius: u.rng, kinds: queryPlayers | queryMonsters,
			lineOfSight: unitFlag(g.def, "PathCheck")}
		for _, t := range z.findEntities(q) {
			if !z.entities.has(e) {
				break
			}
			if !checkTarget(g.src, t, g.targetFlag) {
				continue
			}
			z.unitOnPlaceTimer(e, t)
		}
	}
}

// checkTarget is battle_check_target on a normal map for the units this
// server has: the caster itself, its enemies, its allies (monsters among
// monsters are BCT_PARTY), and other players, which are neutral.
func checkTarget(src, t *entity, flag int) bool {
	if flag&bctAll == bctAll {
		return t.player != nil || t.monster != nil
	}
	var state int
	switch {
	case t == src:
		state = bctSelf | bctParty | bctGuild
	case isEnemy(src, t):
		state = bctEnemy
	case src.monster != nil && t.monster != nil:
		state = bctParty
	default:
		state = bctNeutral
	}
	return flag&state != 0
}

// unitOnPlaceTimer is skill_unit_onplace_timer: a unit acts on one
// target, at most once per interval for each target and group (or each
// skill, for UF_NOOVERLAP).
func (z *Zone) unitOnPlaceTimer(e, t *entity) {
	u := e.unit
	g := u.group
	if t.dead || g.src == nil {
		return
	}
	key := g.id
	if unitFlag(g.def, "NoOverlap") {
		key = -int64(g.def.ID)
	}
	if t.tickset == nil {
		t.tickset = map[int64]int64{}
	}
	if next, ok := t.tickset[key]; ok && z.now < next {
		return
	}
	t.tickset[key] = z.now + int64(g.interval)
	switch g.def.ID {
	case skill.IDFireWall:
		// Fire Wall hits again in the same pass when its interval is
		// shorter than the timer's, while the target stays on the cell.
		x, y := t.x, t.y
		count := 0
		for {
			z.skillAttack(g.def.Type, g.src, e, t, g.def, g.lv, 0)
			u.val2--
			count++
			if g.interval <= 0 || u.val2 <= 0 || t.x != x || t.y != y || count >= unitTimerInterval/g.interval || t.dead {
				break
			}
		}
		if u.val2 <= 0 {
			z.deleteUnit(e)
		}
	default:
		z.skillAttack(g.def.Type, g.src, e, t, g.def, g.lv, 0)
	}
}

// skillUnitMove is skill_unit_move: a unit entered a cell, so the units
// without an interval on its old and new cells act on it.
func (z *Zone) skillUnitMove(e *entity) {
	if len(z.groups) == 0 && (e.sc.Empty() || (!e.sc.Has(sc.SCSafetywall) && !e.sc.Has(sc.SCPneuma))) {
		return
	}
	z.unitStatusRefresh(e)
}

// unitStatusRefresh is skill_unit_onplace and skill_unit_onout for the
// status units: a unit on a Safety Wall or Pneuma cell gets its status,
// and one that left the cells of its group loses it.
func (z *Zone) unitStatusRefresh(t *entity) {
	if t.dead || (t.player == nil && t.monster == nil) {
		return
	}
	en := z.statusEngine()
	if en == nil {
		return
	}
	for _, typ := range []int{sc.SCSafetywall, sc.SCPneuma} {
		var on *unitGroup
		for _, g := range z.groups {
			if statusOfUnit(g.def.ID) != typ || !checkTarget(g.src, t, g.targetFlag) {
				continue
			}
			for _, u := range g.units {
				if u.x == t.x && u.y == t.y {
					on = g
					break
				}
			}
			if on != nil {
				break
			}
		}
		cur := t.sc.Get(typ)
		if cur != nil && (on == nil || int64(groupOfStatus(cur)) != on.id) {
			if g := z.groups[int64(groupOfStatus(cur))]; g == nil || !groupCovers(g, t) {
				en.End(z.unit(t), typ, sc.ReasonRemoved)
				cur = nil
			}
		}
		if on == nil || cur != nil {
			continue
		}
		left := on.limit - (z.now - on.start)
		if left <= 0 {
			continue
		}
		s := sc.Start{Type: typ, Rate: 10000, Val1: on.lv, Duration: left, Flag: sc.NoAvoid, Reason: sc.ReasonSkill,
			Source: z.unit(on.src).Ref()}
		if typ == sc.SCSafetywall {
			s.Val2, s.Val3 = on.def.ID, int(on.id)
		} else {
			s.Val2 = int(on.id)
		}
		en.Start(z.unit(on.src), z.unit(t), s)
	}
}

// statusOfUnit is the status a unit without an interval starts.
func statusOfUnit(id int) int {
	switch id {
	case skill.IDSafetyWall:
		return sc.SCSafetywall
	case skill.IDPneuma:
		return sc.SCPneuma
	}
	return -1
}

// groupOfStatus is the group ID a status unit's status holds.
func groupOfStatus(e *sc.Entry) int {
	if e.Type == sc.SCSafetywall {
		return e.Val3
	}
	return e.Val2
}

// groupCovers reports whether a unit stands on a cell of a group.
func groupCovers(g *unitGroup, t *entity) bool {
	for _, u := range g.units {
		if u.x == t.x && u.y == t.y {
			return true
		}
	}
	return false
}

// posMaxCount is skill_pos_maxcount_check: a ground skill may not stack
// on a unit it conflicts with (UF_NOREITERATION), and a caster may have
// only ActiveInstance groups of a skill (land_skill_limit, for players).
// It sends the failure when display is set.
func (z *Zone) posMaxCount(src *entity, d *skill.Def, lv, x, y int, display bool) bool {
	if d.Unit != nil && unitFlag(d, "NoReiteration") && z.unitRangeConflict(d, lv, x, y) {
		if display {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		}
		return false
	}
	if d.Unit != nil && unitFlag(d, "NoFootSet") && src.player != nil && z.unitRangeOccupied(d, lv, x, y) {
		if display {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		}
		return false
	}
	if src.player != nil {
		if maxCount := d.ActiveInstance.At(lv); maxCount > 0 {
			for _, g := range src.groups {
				if g.def.ID == d.ID {
					maxCount--
				}
			}
			if maxCount <= 0 {
				if display {
					z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
				}
				return false
			}
		}
	}
	return true
}

// reiterationConflicts are the skills whose units block each other, as
// skill_check_unit_range_sub: Safety Wall, Pneuma and their kind.
var reiterationConflicts = map[int]bool{skill.IDSafetyWall: true, skill.IDPneuma: true}

// unitRangeConflict is skill_check_unit_range: a unit of a conflicting
// skill within the new group's layout and range.
func (z *Zone) unitRangeConflict(d *skill.Def, lv, x, y int) bool {
	r := d.Unit.Range.At(lv) + max(d.Unit.Layout.At(lv), 0)
	for _, g := range z.groups {
		same := g.def.ID == d.ID
		if !same && (!reiterationConflicts[d.ID] || !reiterationConflicts[g.def.ID]) {
			continue
		}
		for _, u := range g.units {
			if abs(u.x-x) <= r && abs(u.y-y) <= r {
				return true
			}
		}
	}
	return false
}

// unitRangeOccupied is skill_check_unit_range2: a character within the
// new group's layout and range.
func (z *Zone) unitRangeOccupied(d *skill.Def, lv, x, y int) bool {
	r := d.Unit.Range.At(lv) + max(d.Unit.Layout.At(lv), 0)
	for e := range z.entities.all() {
		if (e.player != nil || e.monster != nil) && !e.dead && abs(e.x-x) <= r && abs(e.y-y) <= r {
			return true
		}
	}
	return false
}

// unitAppear is the appear event of a ground skill's unit.
func (z *Zone) unitAppear(p *Player, id uint32, e *entity) *wathenav1.Event {
	u := e.unit
	b := wathenav1.SkillUnit_builder{
		UnitId:  uint32(u.group.unitID),  // #nosec G115 -- a UNT_ ID.
		SkillId: shownSkill(u.group.def), // the client shows the icon skill's effect.
	}
	if c, ok := p.netIDs.get(u.group.src); ok {
		b.Caster = c
	}
	return wathenav1.Event_builder{Appear: wathenav1.EntityAppear_builder{
		Id:        id,
		Kind:      wathenav1.EntityKind_ENTITY_KIND_SKILL_UNIT,
		X:         uint32(e.x), // #nosec G115 -- map cells are at most 1024.
		Y:         uint32(e.y), // #nosec G115 -- as above.
		SkillUnit: b.Build(),
	}.Build()}.Build()
}
