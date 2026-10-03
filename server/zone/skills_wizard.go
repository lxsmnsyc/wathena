package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// The Wizard and High Wizard skills: rAthena's SkillImpl classes of
// src/map/skills/mage, and the parts of skill.cpp and status.cpp that
// they need. Soul Drain is passive: its SP on a kill is soulDrain, and
// its max SP bonus is in server/status.

func init() {
	registerSkillImpls(map[int]any{
		skill.IDFirePillar:   firePillar{},
		skill.IDSightRasher:  sightRasher{},
		skill.IDMeteor:       meteorStorm{},
		skill.IDJupitel:      jupitel{},
		skill.IDVermilion:    vermilion{},
		skill.IDWaterBall:    waterBall{},
		skill.IDIceWall:      iceWall{},
		skill.IDFrostNova:    frostNova{},
		skill.IDStormGust:    stormGust{},
		skill.IDEarthSpike:   earthSpike{},
		skill.IDHeavenDrive:  heavenDrive{},
		skill.IDQuagmire:     unitSkillImpl{},
		skill.IDEstimation:   sense{},
		skill.IDMagicCrasher: attackSkill{},
		skill.IDMagicPower:   statusSkill{},
		skill.IDNapalmVulcan: napalmVulcan{},
		skill.IDGanbantein:   ganbantein{},
		skill.IDGravitation:  gravitation{},
		skill.IDSightBlaster: sightSkill{},
	})
}

// Statuses that these skills read, which server/statuschange has no
// constants for, and the battle check flag of a pre-renewal Gravitation
// Field's caster.
const (
	scVolcano     = 100 // SC_VOLCANO
	scDeluge      = 101 // SC_DELUGE
	scSuiton      = 236 // SC_SUITON
	scSvRoottwist = 632 // SC_SV_ROOTTWIST
	scGravitation = sc.SCGravitation
)

// Skills of other classes whose units these skills read. The names
// differ from the ones those classes' files may use.
const (
	idSADeluge        = 285 // SA_DELUGE
	idSALandProtector = 288 // SA_LANDPROTECTOR
	idNJSuiton        = 538 // NJ_SUITON
)

// firePillar is SkillFirePillar: a trap that rises when an enemy comes
// near (UNT_FIREPILLAR_WAITING), then hits every enemy around it
// (UNT_FIREPILLAR_ACTIVE). The unit timer runs both.
type firePillar struct{ unitSkillImpl }

// additionalEffect is SkillFirePillar::applyAdditionalEffects: the
// target cannot walk for Duration2.
func (firePillar) additionalEffect(z *Zone, _, target *entity, d *skill.Def, lv int) {
	z.setWalkDelay(target, int64(d.Duration2.At(lv)))
}

// firePillarSplash is UNT_FIREPILLAR_ACTIVE's part of
// skill_unit_onplace_timer: skill_trap_splash on every enemy in the
// skill's splash around the pillar.
func (z *Zone) firePillarSplash(e *entity) {
	g := e.unit.group
	q := &query{area: areaRadius, x: e.x, y: e.y, radius: g.def.SplashFor(g.lv), kinds: queryPlayers | queryMonsters,
		lineOfSight: true}
	for _, t := range z.findEntities(q) {
		if !checkTarget(g.src, t, g.targetFlag) {
			continue
		}
		z.skillAttack(g.def.Type, g.src, e, t, g.def, g.lv, 0)
	}
}

// waterCell is CELL_CHKWATER. The map data has no water cells yet.
func (z *Zone) waterCell(int, int) bool { return false }

// sightRasher is SkillSightRasher: the caster's Sight bursts, a hit on
// every enemy in its splash that it can see.
type sightRasher struct{}

func (sightRasher) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.scEnd(src, sc.SCSight)
	z.skillEffect(src, target, d, lv, int64(lv), true)
	q := &query{area: areaRadius, x: src.x, y: src.y, radius: d.SplashFor(lv), kinds: queryPlayers | queryMonsters,
		enemiesOf: src, lineOfSight: true}
	for _, t := range z.findEntities(q) {
		// skill_area_sub calls skill_castend_damage_id, which is
		// SkillSightRasher::castendDamageId.
		z.skillAttack(d.Type, src, src, t, d, lv, *flag|sdAnimation|1)
	}
}

// meteorStorm is SkillMeteorStorm: one meteor per interval of the
// skill's time, each on a random cell of the splash. Each meteor is a
// group whose lifetime is its fall time; the unit timer drops it.
type meteorStorm struct{}

func (meteorStorm) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	area := d.SplashFor(lv)
	interval := d.Unit.Interval
	for i := 1; i <= d.Duration1.At(lv)/interval; i++ {
		tmpx := x - area + z.rand.IntN(area*2+1)
		tmpy := y - area + z.rand.IntN(area*2+1)
		z.unitSettingFlag(src, d, lv, tmpx, tmpy, *flag+i*interval)
	}
}

// additionalEffect is SkillMeteorStorm::applyAdditionalEffects.
func (meteorStorm) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart(src, target, sc.SCStun, 3*lv, lv, int64(d.Duration2.At(lv)))
}

// jupitel is SkillJupitelThunder: the hit comes 150 ms after the cast,
// so another spell may start before the knockback.
type jupitel struct{}

func (jupitel) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.addTimerSkill(src, z.now+timerSkillInterval, target, 0, 0, d, lv, 1, *flag)
}

func (jupitel) timerSkill(z *Zone, t *timerSkill) { z.jupitelHit(t) }

// jupitelHit is the WZ_JUPITEL case of skill_timerskill, which Water
// Ball shares: a hit while there is a line of sight, at any distance.
func (z *Zone) jupitelHit(t *timerSkill) {
	src, target := t.src, t.target
	if t.typ <= 0 || target.dead {
		return
	}
	if clear, _ := path.Shoot(z.grid, src.x, src.y, target.x, target.y); !clear {
		return
	}
	// The act delay here stops unlimited casting.
	if src.status != nil {
		src.canAct = max(z.now+int64(src.status.AMotion), src.canAct)
	}
	z.skillAttack(battle.TypeMagic, src, src, target, t.def, t.lv, t.flag)
}

// vermilion is SkillLordOfVermilion: a ground unit that may blind.
type vermilion struct{ unitSkillImpl }

// additionalEffect is SkillLordOfVermilion::applyAdditionalEffects.
func (vermilion) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	rate := min(4*lv, 40)
	if z.skillRenewal(skill.IDVermilion) {
		rate = 10 + 5*lv
	}
	z.scStart(src, target, sc.SCBlind, rate, lv, int64(d.Duration2.At(lv)))
}

// waterBall is SkillWaterBall: water cells around the caster turn into
// Water Ball units, and a timer skill throws them one by one.
type waterBall struct{}

func (waterBall) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.unitSetting(src, d, lv, src.x, src.y)
	z.addTimerSkill(src, z.now, target, src.x, src.y, d, lv, 0, *flag)
}

// timerSkill is the WZ_WATERBALL case of skill_timerskill: the next
// Water Ball unit of the layout is used for the next throw. The first
// run only takes a unit; each later one throws a ball, and the last one
// throws without a unit.
func (waterBall) timerSkill(z *Zone, t *timerSkill) {
	src, target := t.src, t.target
	var unit *entity
	layout := unitLayout(t.def, t.lv, src, t.x, t.y)
	for i := t.typ; i >= 0 && i < len(layout); i++ {
		if u := z.unitOnCell(src, t.x+layout[i][0], t.y+layout[i][1], skill.IDWaterBall, false); u != nil {
			unit = u
			break
		}
	}
	z.jupitelHit(t)
	if unit != nil && !target.dead && !src.dead {
		// The unit is used up. The timer goes on until the target dies,
		// even without a line of sight, and the caster cannot walk.
		z.deleteUnit(unit)
		z.setWalkDelay(src, timerSkillInterval)
		z.addTimerSkill(src, z.now+timerSkillInterval, target, t.x, t.y, t.def, t.lv, t.typ+1, t.flag)
	}
}

// iceWall is SkillIceWall: five cells of ice with HP, across the way
// from the caster, that units cannot walk through.
type iceWall struct{}

func (iceWall) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	*flag |= 1
	if z.unitSetting(src, d, lv, x, y) != nil {
		z.skillPosEffect(src, d, lv, x, y)
	}
}

// iceWallLayout is skill_init_unit_layout's Ice Wall layouts, by
// rAthena's direction from the caster to the cell.
func iceWallLayout(dir int) [][2]int {
	if dir&1 != 0 {
		if dir&2 != 0 {
			return [][2]int{{-2, 2}, {-1, 1}, {0, 0}, {1, -1}, {2, -2}}
		}
		return [][2]int{{2, 2}, {1, 1}, {0, 0}, {-1, -1}, {-2, -2}}
	}
	if dir%4 == 0 {
		return [][2]int{{-2, 0}, {-1, 0}, {0, 0}, {1, 0}, {2, 0}}
	}
	return [][2]int{{0, -2}, {0, -1}, {0, 0}, {0, 1}, {0, 2}}
}

// setIceWallCell is map_setgatcell for an Ice Wall cell: on, the cell
// becomes gat type 5, which units cannot walk on but spells pass;
// off, the cell gets back the flags it had. The zone's grid may be shared
// with other zones of the map, so the first change copies it.
func (z *Zone) setIceWallCell(x, y int, on bool, old int) {
	if !z.ownGrid {
		g := *z.grid
		g.Cells = append([]uint8(nil), z.grid.Cells...)
		z.grid = &g
		z.ownGrid = true
	}
	i := x + y*z.grid.Width
	if on {
		z.grid.Cells[i] = path.Shootable
		return
	}
	z.grid.Cells[i] = uint8(old) // #nosec G115 -- the cell's own flags.
}

// frostNova is SkillFrostNova: a hit on every enemy in its splash that
// the caster can see, but not on the caster's own cell.
type frostNova struct{}

func (frostNova) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	z.area = areaTemp{}
	q := &query{area: areaRadius, x: src.x, y: src.y, radius: d.SplashFor(lv), kinds: queryPlayers | queryMonsters,
		enemiesOf: src, lineOfSight: true}
	for _, t := range z.findEntities(q) {
		// skill_attack_area: the target checks, then a hit that keeps its
		// animation.
		if t.x == src.x && t.y == src.y || z.landProtected(t.x, t.y) {
			continue
		}
		if !z.statusCheckSkillUse(nil, t, d, 2) {
			continue
		}
		z.skillAttack(battle.TypeMagic, src, src, t, d, lv, *flag)
	}
}

// additionalEffect is SkillFrostNova::applyAdditionalEffects.
func (frostNova) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	rate := lv*3 + 35
	if src.player != nil {
		rate = lv*5 + 33
	}
	z.scStart(src, target, sc.SCFreeze, rate, lv, int64(d.Duration2.At(lv)))
}

// stormGust is SkillStormGust: a ground unit that freezes. The hit
// counter of pre-renewal is in skill_unit_onplace_timer.
type stormGust struct{ unitSkillImpl }

// additionalEffect is SkillStormGust::applyAdditionalEffects: renewal
// freezes by chance; pre-renewal freezes on the third hit in a row.
func (stormGust) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	if z.skillRenewal(skill.IDStormGust) {
		z.scStart(src, target, sc.SCFreeze, 65-5*lv, lv, int64(d.Duration2.At(lv)))
		return
	}
	switch {
	case target.sgCounter >= 3 && z.scStart(src, target, sc.SCFreeze, 150, lv, int64(d.Duration2.At(lv))):
		target.sgCounter = 0
	case target.sgCounter > 250:
		// It only resets on a freeze, so it would grow forever on bosses.
		target.sgCounter = 0
	}
}

// earthSpike is SkillEarthSpike: one magic hit.
type earthSpike struct{ attackSkill }

// heavenDrive is SkillHeavensDrive: a ground unit that hits once.
type heavenDrive struct{ unitSkillImpl }

// additionalEffect is SkillHeavensDrive::applyAdditionalEffects.
func (heavenDrive) additionalEffect(z *Zone, _, target *entity, _ *skill.Def, _ int) {
	z.scEnd(target, scSvRoottwist)
}

// sense is SkillSense (WZ_ESTIMATION): it shows a monster's data. The
// client has no window for it yet, so the skill only checks its target
// and succeeds.
type sense struct{}

func (sense) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if src.player == nil {
		return
	}
	if target.player != nil {
		// It fails on players.
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	// clif_skill_estimation would send the monster's data here.
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// napalmVulcan is SkillNapalmVulcan: a splash that may curse.
type napalmVulcan struct{ splashSkill }

// additionalEffect is SkillNapalmVulcan::applyAdditionalEffects.
func (napalmVulcan) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart(src, target, sc.SCCurse, 5*lv, lv, int64(d.Duration2.At(lv)))
}

// ganbantein is SkillGanbantein: four times in five, every skill unit in
// the 3x3 square around the cell ends. Otherwise the skill fails and
// keeps its gemstones.
type ganbantein struct{}

func (ganbantein) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	if z.rand.IntN(100) >= 80 {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		*flag |= skillNoConsumeReq
		return
	}
	z.skillPosEffect(src, d, lv, x, y)
	// rAthena keeps the splash in a bool, so the square is 3x3 at
	// every level.
	i := 0
	if d.SplashFor(lv) != 0 {
		i = 1
	}
	for cy := y - i; cy <= y+i; cy++ {
		for cx := x - i; cx <= x+i; cx++ {
			z.cellOverlap(src, d, cx, cy)
		}
	}
}

// gravitation is SkillGravitationField. In renewal it is a ground unit
// that hits from the caster. In pre-renewal the caster keeps the field
// (SC_GRAVITATION with BCT_SELF): it cannot move, use items or deal other
// damage, and damage ends the field.
type gravitation struct{}

func (gravitation) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	*flag |= 1
	g := z.unitSetting(src, d, lv, x, y)
	if z.skillRenewal(skill.IDGravitation) || g == nil {
		return
	}
	z.scStart4(src, src, d.Status, 100, lv, 0, bctSelf, int(g.id), int64(d.Duration1.At(lv)))
}

// gravitationDamaged is status_damage's pre-renewal Gravitation Field
// part: damage to the caster ends its field.
func (z *Zone) gravitationDamaged(e *entity) {
	ge := e.sc.Get(scGravitation)
	if z.skillRenewal(skill.IDGravitation) || ge == nil || ge.Val3 != bctSelf {
		return
	}
	if g := z.groups[int64(ge.Val4)]; g != nil {
		z.deleteGroup(g)
		ge.Val4 = 0
		z.scEnd(e, scGravitation)
	}
}

// wizardFieldRefresh is skill_unit_onplace and skill_unit_onleft for the
// fields that give a status to the units on them: Quagmire, and
// pre-renewal's Gravitation Field. A unit that enters a cell of a field
// that targets it gets the status for the field's whole time. A player
// that leaves every cell of Quagmire loses it; monsters keep it. Any unit
// that leaves Gravitation Field loses it, but its caster.
func (z *Zone) wizardFieldRefresh(t *entity) {
	if t.dead || z.statusEngine() == nil {
		return
	}
	if g := z.fieldOn(t, skill.IDQuagmire); g != nil {
		if !t.sc.Has(sc.SCQuagmire) {
			z.scStart4(g.src, t, sc.SCQuagmire, 100, g.lv, int(g.id), 0, 0, g.limit)
		}
	} else if t.player != nil && t.sc.Has(sc.SCQuagmire) {
		z.scEnd(t, sc.SCQuagmire)
	}
	if z.skillRenewal(skill.IDGravitation) {
		return
	}
	if g := z.fieldOn(t, skill.IDGravitation); g != nil {
		if !t.sc.Has(scGravitation) {
			z.scStart4(g.src, t, scGravitation, 100, g.lv, 0, bctEnemy, int(g.id), g.limit)
		}
	} else if e := t.sc.Get(scGravitation); e != nil && e.Val3 != bctSelf {
		z.scEnd(t, scGravitation)
	}
}

// fieldOn is the group of a skill with a unit on a unit's cell that
// targets it, the oldest when there are several, or nil. Hidden units
// are immune to fields without TargetHidden, as skill_unit_onplace.
func (z *Zone) fieldOn(t *entity, skillID int) *unitGroup {
	var on *unitGroup
	for _, g := range z.groups {
		if g.def.ID != skillID || g.src == nil || on != nil && on.id < g.id || !groupCovers(g, t) {
			continue
		}
		if !checkTarget(g.src, t, g.targetFlag) {
			continue
		}
		if !t.sc.Empty() && t.sc.Option&sc.OptionHide != 0 && !g.def.Has("TargetHidden") {
			continue
		}
		on = g
	}
	return on
}

// toggleMagicPower is skill_toggle_magicpower, in pre-renewal: the first
// damaging spell after Mystical Amplification turns its MATK on, and the
// next one ends it.
func (z *Zone) toggleMagicPower(e *entity, d *skill.Def) {
	if d.NK.Has(battle.NKNoDamage) || d.Type != battle.TypeMagic {
		return
	}
	mp := e.sc.Get(sc.SCMagicpower)
	if mp == nil {
		return
	}
	if mp.Val4 != 0 {
		z.scEnd(e, sc.SCMagicpower)
		return
	}
	mp.Val4 = 1
	scEnv{z}.Recalc(z.unit(e), z.Statuses.Get(sc.SCMagicpower))
}

// soulDrain is the Soul Drain part of skill_counter_additional_effect: a
// player's targeted spell that kills gives back SP, by the target's
// level.
func (z *Zone) soulDrain(src, target *entity, d *skill.Def, flag int) {
	p := src.player
	if p == nil || d == nil || flag&battle.FlagMagic == 0 || !target.dead || z.Skills == nil {
		return
	}
	if d.Target == skill.TargetGround || d.Target == skill.TargetSelf {
		return
	}
	lv := p.skillLevel(skill.IDSoulDrain)
	if lv <= 0 {
		return
	}
	z.skillEffect(src, target, z.Skills.Get(skill.IDSoulDrain), lv, int64(lv), true)
	sp := int64(z.unit(target).Level() * (95 + 15*lv) / 100)
	if _, got := z.statusHeal(p, 0, sp); got > 0 {
		if s := z.battleStats(p); s != nil {
			p.pending = append(p.pending, s)
		}
	}
}

// sightBlasterTick is status_change_timer_sub for SC_SIGHTBLASTER: the
// first enemy next to the caster that a hit damages ends the status.
func (z *Zone) sightBlasterTick() {
	if z.Skills == nil {
		return
	}
	var casters []*entity
	for e := range z.entities.all() {
		if !e.dead && !e.sc.Empty() && e.sc.Has(sc.SCSightblaster) {
			casters = append(casters, e)
		}
	}
	d := z.Skills.Get(skill.IDSightBlaster)
	if d == nil {
		return
	}
	for _, e := range casters {
		st := e.sc.Get(sc.SCSightblaster)
		if !z.entities.has(e) || e.dead || st == nil {
			continue
		}
		q := &query{area: areaRadius, x: e.x, y: e.y, radius: st.Val3, kinds: queryPlayers | queryMonsters}
		for _, t := range z.findEntities(q) {
			if !isEnemy(e, t) || !z.statusCheckSkillUse(e, t, d, 2) {
				continue
			}
			if z.skillAttack(d.Type, e, e, t, d, st.Val1, 0) > 0 {
				z.scEnd(e, sc.SCSightblaster)
				break
			}
		}
	}
}
