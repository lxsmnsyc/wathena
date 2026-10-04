package zone

import (
	"cmp"
	"slices"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
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
	bctSelf  = 0x010000
	bctEnemy = 0x020000
	bctParty = 0x040000
	bctGuild = bctGuildAlly | bctSameGuild
	// bctGuildAlly is allies, not guildmates, and bctSameGuild guildmates,
	// not allies.
	bctGuildAlly = 0x080000
	bctSameGuild = 0x200000
	bctNeutral   = 0x100000
	bctAll       = 0x3F0000
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
	// val1, val2 and val3 are the group's values: Sanctuary's hits left
	// on the undead and Fire Pillar's val1; Safety Wall's hits and, in
	// renewal, its durability; Fire Wall's hits per cell; whether a Meteor
	// Storm meteor started to fall.
	val1 int
	val2 int
	val3 int64
	// text is the group's valstr: the message of Graffiti.
	text  string
	units []*entity
	// linkGroup is link_group_id: a group that ends with this one, as a
	// pre-renewal Gravitation Field cast under another.
	linkGroup int64
	// deleted is set once the group is gone.
	deleted bool
	// trap holds a trap's own values, or nil for other skills.
	trap *trapState
	// target is the unit that Fiber Lock holds (sg->val2).
	target *entity
	// songDance is group->state.song_dance: 1 for a song or a dance, 2
	// for an ensemble. x and y are the cell the group was placed on,
	// which a song moves from as its performer walks.
	songDance int
	x, y      int
}

// skillUnit is skill_unit: one cell of a group.
type skillUnit struct {
	group *unitGroup
	// val1 and val2 are the unit's own values: an Ice Wall cell's HP and
	// the cell flags it covers; the hits left on a Fire Wall cell.
	val1 int
	val2 int
	// rng is the unit's range: the area around its cell it acts on.
	rng int
	// limit is the unit's own lifetime from the group's start.
	limit int64
	// hidden units act but are not shown, as the cells of a
	// RangedSingleUnit group besides its center.
	hidden bool
	// overlap is val2's UF_ENSEMBLE mark: a song or dance cell on another
	// of its kind, which acts as Dissonance or Ugly Dance.
	overlap bool
}

// unitLayout is skill_get_unit_layout: the cell offsets of a group. A
// layout of n is the square of radius n around the cell. Fire Wall's
// layout (-1) depends on the direction from the caster.
func unitLayout(d *skill.Def, lv int, src *entity, x, y int) [][2]int {
	n := d.Unit.Layout.At(lv)
	// Monsters sometimes deploy more units on level 10.
	if src.monster != nil && lv >= 10 && d.ID == skill.IDWaterBall {
		n = 4
	}
	if n >= 0 {
		var cells [][2]int
		for dy := -n; dy <= n; dy++ {
			for dx := -n; dx <= n; dx++ {
				cells = append(cells, [2]int{dx, dy})
			}
		}
		return cells
	}
	switch d.ID {
	case skill.IDSanctuary:
		return sanctuaryLayout
	case skill.IDMagnus, skill.IDGospel:
		// Battle Chant's cross is Magnus Exorcismus's.
		return magnusLayout
	case skill.IDGrandCross, skill.IDNPCGrandDarkness:
		return grandCrossLayout
	case skill.IDNPCFlameCross:
		return flameCrossLayout
	case skill.IDVenomDust:
		// skill_init_unit_layout: Venom Dust's cross of 5 cells.
		return [][2]int{{-1, 0}, {0, -1}, {0, 0}, {0, 1}, {1, 0}}
	case skill.IDFogWall:
		// Blinding Mist's 5x3 layout, the same in every direction.
		var cells [][2]int
		for dy := -1; dy <= 1; dy++ {
			for dx := -2; dx <= 2; dx++ {
				cells = append(cells, [2]int{dx, dy})
			}
		}
		return cells
	}
	if d.ID != skill.IDFireWall && d.ID != skill.IDIceWall {
		return [][2]int{{0, 0}}
	}
	dir := 6
	if src.x != x || src.y != y {
		dir = mapCalcDir(src, x, y)
	}
	if d.ID == skill.IDIceWall {
		return iceWallLayout(dir)
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
	return z.unitSettingFlag(src, d, lv, x, y, 0)
}

// unitSettingFlag is skill_unitsetting with its flag: flag&1 places the
// skill's second unit, as an active Fire Pillar. Meteor Storm passes its
// lifetime instead.
func (z *Zone) unitSettingFlag(src *entity, d *skill.Def, lv, x, y, flag int) *unitGroup {
	if d.Unit == nil {
		return nil
	}
	limit := int64(d.Duration1.At(lv))
	var val1 int
	var link int64
	switch d.ID {
	case skill.IDMeteor, skill.IDNPCRainOfMeteor:
		limit = int64(flag)
		// The flag should not change anything else for these skills.
		flag = 0
	case skill.IDFirePillar:
		if z.landProtected(x, y) {
			return nil
		}
		if flag&1 != 0 {
			limit = 1000
		}
		val1 = lv + 2
	case skill.IDGravitation:
		// Pre-renewal: a field cast under another links to it.
		if e := src.sc.Get(scGravitation); !z.skillRenewal(skill.IDGravitation) && e != nil && e.Val3 == bctSelf {
			link = int64(e.Val4)
		}
	}
	// Quagmire's target turns to all on PvP maps (vs_traps_bctall),
	// which this server does not have.
	unitID := d.Unit.ID
	if flag&1 != 0 {
		unitID = d.Unit.AlternateID
	}
	g := &unitGroup{src: src, def: d, lv: lv, unitID: unitID, start: z.now,
		limit: limit, interval: d.Unit.Interval, targetFlag: d.Unit.Target, val1: val1, linkGroup: link}
	rng := d.Unit.Range.At(lv)
	switch d.ID {
	case skill.IDSafetyWall:
		g.val2 = lv + 1
		if z.skillRenewal(skill.IDSafetyWall) && src.status != nil {
			level := z.unit(src).Level()
			g.val3 = int64(300*lv + 65*(src.status.Int+level) + src.status.MaxSP)
		}
	case skill.IDFireWall:
		if src.sc.Has(scViolentgale) {
			g.limit = g.limit * 3 / 2
		}
		g.val2 = 4 + lv
	case skill.IDSanctuary:
		// The undead and demons it may still hit.
		g.val1 = lv + 3
	case skill.IDNPCComet:
		// The caster's last comet, which the damage reads.
		src.comet = &[2]int{x, y}
	case skill.IDSkidTrap, skill.IDAnkleSnare, skill.IDShockwave, skill.IDSandman, skill.IDClaymoreTrap,
		skill.IDLandMine, skill.IDFlasher, skill.IDFreezingTrap, skill.IDBlastMine, skill.IDTalkieBox:
		g.trap = z.trapSetting(src, d, lv)
	case skill.IDGraffiti:
		// A player's message comes with the cast (sd->message), which the
		// protocol does not carry yet, so it is empty. Other units write
		// "Boo!".
		if src.player == nil {
			g.text = "Boo!"
		}
	case skill.IDLandProtector, skill.IDVolcano, skill.IDDeluge, skill.IDViolentGale:
		// A caster has one element field. A new one replaces the old,
		// and a new Volcano, Deluge or Violent Gale keeps the time the
		// old one had left.
		if old := z.elementField(src); old != nil {
			if isElementField(old.def.ID) && old.limit > 0 {
				g.limit = old.limit - (z.now - old.start)
				if g.limit < 0 {
					g.limit = int64(d.Duration1.At(lv))
				}
			}
			z.clearGroup(src)
		}
	case skill.IDWhistle, skill.IDHumming, skill.IDPoemBragi, skill.IDDontForgetMe, skill.IDServiceForYou,
		skill.IDAssassinCross, skill.IDFortuneKiss, skill.IDDrumBattlefield, skill.IDRingNibelungen,
		skill.IDRichmanKim, skill.IDSiegfried:
		g.val1, g.val2 = z.songValues(src, d.ID, lv)
	}
	z.nextGroup++
	g.id = z.nextGroup
	// A song, dance or ensemble starts SC_DANCING on its performer.
	g.songDance, g.x, g.y = songDanceOf(d), x, y
	z.danceStart(src, g)
	single := unitFlag(d, "RangedSingleUnit")
	for _, off := range unitLayout(d, lv, src, x, y) {
		ux, uy := x+off[0], y+off[1]
		if ux <= 0 || uy <= 0 || ux >= z.grid.Width || uy >= z.grid.Height {
			continue
		}
		// Songs and dances may stand on walls.
		if g.songDance == 0 && z.grid.NoPass(ux, uy) {
			continue
		}
		if unitFlag(d, "PathCheck") && !z.clearShot(ux, uy, src.x, src.y) {
			continue
		}
		// skill_cell_overlap: the cell may refuse the unit or lose its
		// own.
		alive := z.cellOverlap(src, d, ux, uy)
		if alive == 0 {
			continue
		}
		u := &skillUnit{group: g, rng: rng, limit: g.limit, hidden: single && (off[0] != 0 || off[1] != 0)}
		if d.ID == skill.IDFogWall && alive == 2 {
			// Blinding Mist on Deluge lasts twice as long.
			u.limit *= 2
			g.limit = u.limit
		}
		switch d.ID {
		case skill.IDFireWall:
			u.val2 = g.val2
		case skill.IDIceWall:
			u.val1 = 200 + 200*lv
			u.val2 = int(z.grid.Cells[ux+uy*z.grid.Width])
		case skill.IDGrandCross, skill.IDNPCGrandDarkness:
			// The hits each cell deals: 950 / 300 = 3.
			if g.interval > 0 {
				u.val1 = d.Duration1.At(lv) / g.interval
			}
		case skill.IDNPCReverberation:
			// The hits the cell takes before it goes off.
			u.val1 = 1 + lv
		case skill.IDWaterBall:
			// Water, Deluge or Suiton turn into Water Ball cells. A
			// monster needs none.
			if src.player != nil && !z.waterCell(ux, uy) && z.unitOnCell(src, ux, uy, idSADeluge, true) == nil &&
				z.unitOnCell(src, ux, uy, idNJSuiton, true) == nil {
				continue
			}
		}
		e := &entity{unit: u, x: ux, y: uy}
		z.entities.add(e)
		g.units = append(g.units, e)
		// skill_initunit: a song or dance cell may overlap another.
		z.danceOverlapSet(e)
		// skill_initunit's oninit actions.
		if d.ID == skill.IDIceWall {
			z.setIceWallCell(ux, uy, true, 0)
		}
	}
	if len(g.units) == 0 {
		// skill_delunitgroup of a group without cells.
		z.danceGroupDeleted(g)
		return nil
	}
	src.groups = append(src.groups, g)
	if len(z.groups) == 0 {
		// rAthena's skill_unit_timer always runs, so a new unit acts
		// within 100 ms. The zone's timer rests while there are no groups,
		// so it starts again at the next zone tick.
		z.nextUnitTimer = min(z.nextUnitTimer, z.now+unitTimerInterval/2)
	}
	z.groups[g.id] = g
	// A unit of range 0 acts at once on whoever stands on its cell.
	if rng == 0 {
		for _, t := range z.unitsOnCells(g) {
			z.unitStatusRefresh(t)
			if d.ID == skill.IDBasilica {
				z.basilicaRefresh(t, false)
			}
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
	// skill_delunit: the cell's overlap ends.
	z.danceOverlapRemove(e)
	z.entities.remove(e)
	z.removed = append(z.removed, removal{e, wathenav1.VanishReason_VANISH_REASON_EXPIRED})
	g.units = slices.DeleteFunc(g.units, func(x *entity) bool { return x == e })
	// skill_delunit's ondelete actions.
	if g.def.ID == skill.IDIceWall {
		z.setIceWallCell(e.x, e.y, false, u.val2)
	}
	// Ankle Snare lets go of the unit it holds.
	if g.def.ID == skill.IDAnkleSnare && g.trap != nil && g.trap.caught != nil {
		z.scEnd(g.trap.caught, sc.SCAnkle)
	}
	// skill_unit_onout for whoever stands on the cell.
	for o := range z.entities.all() {
		if o.x == e.x && o.y == e.y && (o.player != nil || o.monster != nil) {
			z.unitStatusRefresh(o)
			if g.def.ID == skill.IDBasilica {
				z.basilicaRefresh(o, false)
			}
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
	z.danceGroupDeleted(g)
	delete(z.groups, g.id)
	if g.def.ID == skill.IDSpiderWeb {
		z.spiderWebGroupDeleted(g)
	}
	g.src.groups = slices.DeleteFunc(g.src.groups, func(x *unitGroup) bool { return x == g })
	for _, e := range slices.Clone(g.units) {
		z.deleteUnit(e)
	}
	// The caster's Battle Chant ends with its group.
	if e := g.src.sc.Get(sc.SCGospel); g.unitID == untGospel && e != nil && int64(e.Val3) == g.id {
		e.Val3 = 0
		z.scEnd(g.src, sc.SCGospel)
	}
	// A linked group ends with this one.
	if l := z.groups[g.linkGroup]; g.linkGroup != 0 && l != nil {
		z.deleteGroup(l)
	}
}

// clearUnitGroups is skill_clear_unitgroup: a unit's groups end, as when
// it dies or leaves the map.
func (z *Zone) clearUnitGroups(e *entity) {
	for _, g := range slices.Clone(e.groups) {
		z.deleteGroup(g)
	}
	// skill_cleartimerskill comes with it in rAthena's death and
	// unit_remove_map.
	z.clearTimerSkills(e)
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
		meteor := g.def.ID == skill.IDMeteor || g.def.ID == skill.IDNPCRainOfMeteor
		if age >= g.limit || age >= u.limit {
			// The unit expired (skill_unit_onlimit).
			switch {
			case g.unitID == untIceWall:
				// An Ice Wall cell loses 50 HP every second.
				u.val1 -= 50
				g.limit = z.now + int64(g.interval) - g.start
				u.limit = z.now + int64(g.interval) - g.start
				if u.val1 <= 0 {
					z.deleteUnit(e)
				}
			case meteor && g.val2 == 1:
				// A meteor deals its damage before it ends.
			case g.trap != nil && z.trapExpired(e):
				// A trap that never went off, or a used trap.
			default:
				z.deleteUnit(e)
			}
		} else if meteor {
			// The meteor starts to fall one interval before it expires.
			if g.val2 == 0 && (age >= g.limit-int64(g.interval) || age >= u.limit-int64(g.interval)) && g.src != nil {
				z.skillPosEffect(g.src, g.def, g.lv, e.x, e.y)
				g.val2 = 1
				// Every meteor of Meteor Storm hits 700 ms after its
				// effect starts.
				if g.def.ID == skill.IDMeteor {
					g.limit = g.limit - int64(g.interval) + 700
				}
			}
			// No damage until it expires.
			continue
		}
		if g.def.ID == skill.IDSanctuary && g.val1 <= 0 {
			// Sanctuary ends once it hit its count of the undead.
			z.deleteGroup(g)
			continue
		}
		// skill_dance_switch: an overlapped song or dance cell acts as
		// Dissonance or Ugly Dance.
		restore := z.danceSwitch(e)
		if !z.entities.has(e) || u.rng < 0 || g.interval == -1 {
			restore()
			continue
		}
		kinds := queryPlayers | queryMonsters
		if unitFlag(g.def, "NoMob") {
			// UF_NOMOB: monsters do not set the unit off.
			kinds = queryPlayers
		}
		q := &query{area: areaRadius, x: e.x, y: e.y, radius: u.rng, kinds: kinds,
			lineOfSight: unitFlag(g.def, "PathCheck")}
		// skill_unit_effect: ground skills do nothing on Magnetic
		// Earth, except songs and traps.
		if !g.def.Has("IsSong") && !g.def.Has("IsTrap") && !g.def.Has("IgnoreLandProtector") &&
			z.landProtected(e.x, e.y) {
			continue
		}
		for _, t := range z.findEntities(q) {
			if !z.entities.has(e) {
				break
			}
			if g.trap != nil && !z.trapCatches(g, t) {
				continue
			}
			// A skill with TargetSelf, as pre-renewal Grand Cross, also
			// hits its caster.
			if !checkTarget(g.src, t, g.targetFlag) && (t != g.src || !g.def.Has("TargetSelf")) {
				continue
			}
			z.unitOnPlaceTimer(e, t)
		}
		// A meteor ends once it fell.
		if meteor && z.entities.has(e) {
			z.deleteUnit(e)
		}
		restore()
	}
}

// UNT_ IDs that units read.
const (
	untFirePillarWaiting = 0x87 // UNT_FIREPILLAR_WAITING
	untFirePillarActive  = 0x88 // UNT_FIREPILLAR_ACTIVE
	untUsedTraps         = 0x8c // UNT_USED_TRAPS
	untIceWall           = 0x8d // UNT_ICEWALL
	untGravitation       = 0xb8 // UNT_GRAVITATION
	untGospel            = 0xb3 // UNT_GOSPEL
)

// checkTarget is battle_check_target on a normal map for the units this
// server has: the caster itself, its enemies, its allies (monsters among
// monsters are BCT_PARTY), and other players, which are neutral. Outside
// PvP and GvG, a unit whose master shares the source master's party is
// BCT_PARTY too, when the flag asks for it or the unit is an enemy. The
// party state then takes the place of the enemy state, except for a
// summon that players may hit.
func checkTarget(src, t *entity, flag int) bool {
	if flag&bctAll == bctAll {
		return t.player != nil || t.monster != nil
	}
	var state int
	switch {
	case t == src || t.monster != nil && t.monster.master == src:
		// The unit itself, or its own summon, whose master it is. A
		// summon that players may hit stays an enemy too.
		state = bctSelf | bctParty | bctGuild
		if isEnemy(src, t) {
			state |= bctEnemy
		}
		return flag&state != 0
	case isEnemy(src, t):
		state = bctEnemy
	case src.monster != nil && t.monster != nil:
		state = bctParty
	}
	if (flag&bctParty != 0 || state&bctEnemy != 0) && sameParty(battleMaster(src), battleMaster(t)) {
		state |= bctParty
		if state&bctEnemy != 0 && (t.monster == nil || !t.monster.playersHit) {
			state &^= bctEnemy
		}
	}
	// The same guild, or an allied one unless the flag has
	// BCT_SAMEGUILD, which BCT_GUILD holds.
	if (flag&bctGuild != 0 || state&bctEnemy != 0) && sameGuildOrAlly(src, t, flag&bctSameGuild != 0) {
		state |= bctGuild
		if state&bctEnemy != 0 && (t.monster == nil || !t.monster.playersHit) {
			state &^= bctEnemy
		}
	}
	if state == 0 {
		state = bctNeutral
	}
	return flag&state != 0
}

// battleMaster is battle_get_master: the unit that summoned a monster,
// up the chain of masters, or the unit itself.
func battleMaster(e *entity) *entity {
	for range 8 {
		if e.monster == nil || e.monster.master == nil {
			break
		}
		e = e.monster.master
	}
	return e
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
	switch g.unitID {
	case untIceWall, untUsedTraps:
		// Ice Wall does nothing to the units on it, and a used trap
		// waits to end.
		return
	case untFirePillarWaiting:
		// The pillar rises: an active pillar takes the waiting one's
		// place.
		z.unitSettingFlag(g.src, g.def, g.lv, e.x, e.y, 1)
		z.deleteUnit(e)
		return
	case untFirePillarActive:
		z.firePillarSplash(e)
		// It ends 1.5 seconds later. multi_trigger_trap is off, so it
		// does not go off again for the other units in range.
		g.limit = z.now - g.start + 1500
		g.unitID = untUsedTraps
		return
	case untGravitation:
		if z.skillRenewal(skill.IDGravitation) {
			// Renewal's field hits from the caster.
			z.skillAttack(g.def.Type, g.src, g.src, t, g.def, g.lv, 0)
		} else {
			z.skillAttack(g.def.Type, g.src, e, t, g.def, g.lv, 0)
		}
		return
	}
	switch g.def.ID {
	case skill.IDStormGust:
		if z.skillRenewal(skill.IDStormGust) {
			z.skillAttack(g.def.Type, g.src, e, t, g.def, g.lv, 0)
			return
		}
		// Pre-renewal's Storm Gust counts its hits on each unit, over
		// every Storm Gust. An absorbed hit resets the count.
		t.sgCounter++
		if z.skillAttack(g.def.Type, g.src, e, t, g.def, g.lv, 0) <= 0 {
			t.sgCounter = 0
		}
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
	case skill.IDSanctuary:
		z.sanctuaryOnPlace(e, t)
	case skill.IDMagnus:
		// Pre-renewal Magnus Exorcismus hits only the undead and demons.
		if st := t.status; !z.skillRenewal(skill.IDMagnus) && (st == nil || !battle.IsUndead(st.Race, st.DefEle) && st.Race != battle.RaceDemon) {
			break
		}
		z.skillAttack(g.def.Type, g.src, e, t, g.def, g.lv, 0)
	case skill.IDBasilica:
		z.basilicaOnPlace(e, t)
	case idNPCWideSuck:
		// The hit heals the caster (skills_npc_status.go).
		z.npcWideSuckOnPlace(e, t)
	case skill.IDGrandCross, skill.IDNPCGrandDarkness:
		z.grandCrossOnPlace(e, t)
	case skill.IDNPCEvilLand:
		z.evilLandOnPlace(e, t)
	case skill.IDNPCEarthquake:
		z.earthquakeOnPlace(e, t)
	case skill.IDNPCComet:
		z.cometOnPlace(e, t)
	case skill.IDNPCCloudKill:
		z.cloudKillOnPlace(e, t)
	case skill.IDNPCMagmaEruption:
		z.magmaEruptionOnPlace(e, t)
	case skill.IDNPCReverberation:
		z.reverberationOnPlace(e)
	case skill.IDGospel:
		z.gospelOnPlace(e, t)
	case skill.IDLullaby, skill.IDUglyDance, skill.IDDissonance, skill.IDAppleIdun:
		z.songOnPlaceTimer(e, t)
	case skill.IDVenomDust:
		// UNT_VENOMDUST: poison on a target without it.
		if en := z.statusEngine(); en != nil && !t.sc.Has(sc.SCPoison) {
			su := z.unit(g.src)
			en.Start(su, z.unit(t), sc.Start{Type: sc.SCPoison, Rate: 10000, Val1: g.lv, Val2: int(su.Ref().ID),
				Duration: int64(g.def.Duration2.At(g.lv)), Reason: sc.ReasonSkill, Source: su.Ref()})
		}
	case skill.IDSkidTrap, skill.IDAnkleSnare, skill.IDShockwave, skill.IDSandman, skill.IDClaymoreTrap,
		skill.IDLandMine, skill.IDFlasher, skill.IDFreezingTrap, skill.IDBlastMine, skill.IDTalkieBox:
		z.trapOnPlaceTimer(e, t)
	default:
		z.skillAttack(g.def.Type, g.src, e, t, g.def, g.lv, 0)
	}
}

// skillUnitMove is skill_unit_move: a unit entered a cell, so the units
// without an interval on its old and new cells act on it.
func (z *Zone) skillUnitMove(e *entity) {
	z.cloakingMoved(e)
	if len(z.groups) == 0 && (e.sc.Empty() || (!e.sc.Has(sc.SCSafetywall) && !e.sc.Has(sc.SCPneuma) &&
		!e.sc.Has(sc.SCBasilica) && !e.sc.Has(sc.SCQuagmire) && !e.sc.Has(scGravitation))) {
		return
	}
	z.unitStatusRefresh(e)
	z.basilicaRefresh(e, true)
	z.danceMoved(e)
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
			if statusOfUnit(g.def.ID) != typ || !checkTarget(g.src, t, g.targetFlag) || z.unitBlocked(g, t) {
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
	z.fieldStatusRefresh(t)
	z.wizardFieldRefresh(t)
	z.spiderWebOnPlace(t)
	z.songRefresh(t)
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
		// Traps do not stack on each other, nor on Venom Dust.
		trap := d.Has("IsTrap") && (g.def.Has("IsTrap") || g.def.ID == idVenomDust)
		if !same && !trap && (!reiterationConflicts[d.ID] || !reiterationConflicts[g.def.ID]) {
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
		UnitId:  uint32(z.shownUnitID(e)), // #nosec G115 -- a UNT_ ID.
		SkillId: shownSkill(u.group.def),  // the client shows the icon skill's effect.
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

// unitsAt are the skill units on a cell, in the order the zone added
// them.
func (z *Zone) unitsAt(x, y int) []*entity {
	var out []*entity
	for _, g := range z.groups {
		for _, e := range g.units {
			if e.x == x && e.y == y {
				out = append(out, e)
			}
		}
	}
	slices.SortFunc(out, func(a, b *entity) int { return cmp.Compare(a.uid, b.uid) })
	return out
}

// unitOnCell is map_find_skill_unit_oncell: a unit of a skill on a cell,
// or nil. With check set, only a unit whose group targets target counts.
func (z *Zone) unitOnCell(target *entity, x, y, skillID int, check bool) *entity {
	for _, e := range z.unitsAt(x, y) {
		g := e.unit.group
		if g.def.ID != skillID {
			continue
		}
		if !check || checkTarget(g.src, target, g.targetFlag) {
			return e
		}
	}
	return nil
}

// cellOverlap is skill_cell_overlap over the units on a cell, for a new
// unit of a skill: it removes the units the skill removes, and returns 0
// when the new unit cannot be placed, 2 for Blinding Mist on Deluge,
// whose time doubles, and 1 otherwise.
func (z *Zone) cellOverlap(src *entity, d *skill.Def, x, y int) int {
	alive := 1
	for _, e := range z.unitsAt(x, y) {
		if alive == 0 {
			break
		}
		if !z.entities.has(e) || e.unit.group.deleted {
			continue
		}
		u, g := e.unit, e.unit.group
		switch d.ID {
		case skill.IDGanbantein:
			// Every unit goes. A ranged single unit's group goes with its
			// center.
			if unitFlag(g.def, "RangedSingleUnit") {
				if !u.hidden {
					z.deleteGroup(g)
				}
			} else {
				z.deleteUnit(e)
			}
			continue
		case skill.IDWaterBall:
			// Water Ball takes Deluge and Suiton.
			if g.def.ID == idSADeluge || g.def.ID == idNJSuiton {
				z.deleteUnit(e)
				continue
			}
		case skill.IDIceWall, skill.IDGravitation:
			// These cannot be placed on themselves. Gravitation Field only
			// in pre-renewal.
			if (d.ID != skill.IDGravitation || !z.skillRenewal(skill.IDGravitation)) && g.def.ID == d.ID {
				return 0
			}
		case skill.IDLandProtector:
			if g.def.ID == skill.IDLandProtector {
				// Two Magnetic Earths cancel each other.
				alive = 0
				z.deleteUnit(e)
				continue
			}
			// It removes everything but traps and barriers.
			if !g.def.Has("IsTrap") && !g.def.Has("IgnoreLandProtector") || g.def.ID == idFirePillar {
				if unitFlag(g.def, "RangedSingleUnit") {
					if !u.hidden {
						z.deleteGroup(g)
					}
				} else {
					z.deleteUnit(e)
				}
				continue
			}
		case skill.IDVolcano, skill.IDDeluge, skill.IDViolentGale:
			// They cannot be placed on top of other units.
			if u.rng <= 0 && (g.def.Unit == nil || g.def.Unit.ID != untDummySkill) {
				alive = 0
				continue
			}
		case skill.IDFogWall:
			switch g.def.ID {
			case skill.IDVolcano, skill.IDViolentGale:
				alive = 0
				continue
			case skill.IDDeluge:
				alive = 2
			}
		}
		// Magnetic Earth blocks every new unit but traps and barriers.
		if g.def.ID == skill.IDLandProtector && !d.Has("IsTrap") && !d.Has("IgnoreLandProtector") {
			alive = 0
		}
	}
	return alive
}
