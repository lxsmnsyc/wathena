package zone

import (
	"slices"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// The monster magic, breath and area skills: the NPC_ skills of rAthena's
// src/map/skills/npc that hit with magic, a breath or an area. Their
// damage formulas are in server/skill/npc_magic.go.

func init() {
	registerSkillImpls(map[int]any{
		skill.IDNPCDarkStrike:       attackSkill{},
		skill.IDNPCDarkThunder:      attackSkill{},
		skill.IDNPCMagicalAttack:    npcMagicalAttack{},
		skill.IDNPCHellJudgement:    npcHellJudgement{},
		skill.IDNPCHellJudgement2:   npcHellJudgement{},
		skill.IDNPCDarkCross:        npcDarkCross{},
		skill.IDNPCDarkBreath:       npcDarkBreath{},
		skill.IDNPCFireBreath:       npcBreath{},
		skill.IDNPCIceBreath:        npcBreath{rate: 70, status: sc.SCFreeze},
		skill.IDNPCIceBreath2:       npcBreath{rate: 100, status: sc.SCFreeze},
		skill.IDNPCThunderBreath:    npcBreath{},
		skill.IDNPCAcidBreath:       npcBreath{rate: 70, status: sc.SCPoison},
		skill.IDNPCDarknessBreath:   npcBreath{},
		skill.IDNPCGrandDarkness:    npcGrandDarkness{},
		skill.IDNPCEvilLand:         npcEvilLand{},
		skill.IDNPCEarthquake:       npcEarthquake{},
		skill.IDNPCStormGust2:       npcStormGust2{},
		skill.IDNPCVenomFog:         npcUnit{},
		skill.IDNPCPsychicWave:      npcUnit{},
		skill.IDNPCIceMine:          npcUnit{},
		skill.IDNPCFlameCross:       npcUnit{},
		skill.IDNPCHellBurning:      npcUnit{},
		skill.IDNPCCaneOfEvilEye:    npcUnit{},
		skill.IDNPCCloudKill:        npcCloudKill{},
		skill.IDNPCComet:            npcComet{},
		skill.IDNPCRainOfMeteor:     npcRainOfMeteor{},
		skill.IDNPCJackFrost:        npcJackFrost{},
		skill.IDNPCArrowStorm:       splashSkill{},
		skill.IDNPCRayOfGenesis:     npcRayOfGenesis{},
		skill.IDNPCPoisonBuster:     npcPoisonBuster{},
		skill.IDNPCIgnitionBreak:    npcIgnitionBreak{},
		skill.IDNPCVenomImpress:     npcVenomImpress{},
		skill.IDNPCDragonBreath:     npcDragonBreath{},
		skill.IDNPCMandragora:       npcMandragora{},
		skill.IDNPCFatalMenace:      npcFatalMenace{},
		skill.IDNPCCursedCircle:     npcCursedCircle{},
		skill.IDNPCMagmaEruption:    npcMagmaEruption{},
		idNCMagmaEruptionDot:        ncMagmaEruptionDot{},
		skill.IDNPCElectricWalk:     npcPropertyWalk{},
		skill.IDNPCFireWalk:         npcPropertyWalk{},
		skill.IDNPCReverberation:    npcUnit{},
		skill.IDNPCReverberationAtk: npcReverberationAtk{},
	})
}

// Statuses that these skills start, which server/statuschange has no
// constants for.
const (
	scMagicalAttack     = 251 // SC_MAGICALATTACK
	scFear              = 309 // SC_FEAR
	scBurning           = 310 // SC_BURNING
	scFreezing          = 311 // SC_FREEZING
	scVenomImpress      = 361 // SC_VENOMIMPRESS
	scPropertyWalk      = 392 // SC_PROPERTYWALK
	scCursedCircleAtker = 414 // SC_CURSEDCIRCLE_ATKER
)

// Skills that these skills use, which server/skill does not name.
const (
	idNPCMagmaEruptionDot = 734  // NPC_MAGMA_ERUPTION_DOTDAMAGE
	idNCMagmaEruptionDot  = 5015 // NC_MAGMA_ERUPTION_DOTDAMAGE
	idSOFireWalk          = 2443 // SO_FIREWALK
	idSOElectricWalk      = 2444 // SO_ELECTRICWALK
)

// etAnger is ET_ANGER, the emotion of NPC_DARKBREATH.
const etAnger = 7

// npcMagicalAttack is SkillDemonShockAttack: a hit of the skill's type,
// then SC_MAGICALATTACK on the caster for Duration1, which turns its
// normal attacks into this skill.
type npcMagicalAttack struct{}

func (npcMagicalAttack) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.skillAttack(d.Type, src, src, target, d, lv, *flag)
	z.scStart(src, src, scMagicalAttack, 100, lv, int64(d.Duration1.At(lv)))
}

// npcHellJudgement is SkillHellsJudgement and SkillHellsJudgement2: a
// splash around the caster (SkillImplRecursiveDamageSplash), which
// castendNoDamageId starts with the caster as the target.
type npcHellJudgement struct{ splashSkill }

// castendNoDamage is castendNoDamageId: skill_castend_damage_id on the
// caster. It runs the splash itself, so the end of the cast runs once.
func (h npcHellJudgement) castendNoDamage(z *Zone, src, _ *entity, d *skill.Def, lv int, flag *int) {
	h.castendDamage(z, src, src, d, lv, flag)
}

// additionalEffect is applyAdditionalEffects:
//   - NPC_HELLJUDGEMENT curses.
//   - NPC_HELLJUDGEMENT2 starts one of six statuses at random.
func (npcHellJudgement) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	typ := sc.SCCurse
	if d.ID == skill.IDNPCHellJudgement2 {
		switch z.rand.IntN(6) {
		case 0:
			typ = sc.SCSleep
		case 1:
			typ = sc.SCConfusion
		case 2:
			typ = sc.SCHallucination
		case 3:
			typ = sc.SCStun
		case 4:
			typ = scFear
		}
	}
	z.scStart(src, target, typ, 100, lv, int64(d.Duration2.At(lv)))
}

// npcDarkCross is SkillCrossOfDarkness: a weapon hit that may blind.
type npcDarkCross struct{ attackSkill }

// additionalEffect is SkillCrossOfDarkness::applyAdditionalEffects.
func (npcDarkCross) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart(src, target, sc.SCBlind, 3*lv, lv, int64(d.Duration2.At(lv)))
}

// npcDarkBreath is SkillDarkBreath: the caster shows anger, then hits
// on half of the rolls.
type npcDarkBreath struct{}

func (npcDarkBreath) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.emotions = append(z.emotions, npcEmotion{src, etAnger})
	if z.rand.IntN(2) == 0 {
		return
	}
	z.skillAttack(d.Type, src, src, target, d, lv, *flag)
}

// npcBreath is the breath skills: SkillFireBreath, SkillIceBreath,
// SkillIceBreath2, SkillThunderBreath, SkillAcidBreath and
// SkillDarknessBreath. A hit may start a status at a rate.
type npcBreath struct {
	rate, status int
}

// castendDamage is the breaths' castendDamageId with
// skill_eightpath_algorithm: the units in a line toward the target take
// a hit (map_foreachindir with skill_attack_area). When the line deals
// no damage, the target takes one more.
func (npcBreath) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.area.main = target
	var total int64
	for _, t := range z.inDir(src.x, src.y, target.x, target.y, d.SplashFor(lv), d.ActiveInstance.At(lv), 0) {
		total += z.breathAttackArea(src, t, d, lv, *flag)
	}
	if total == 0 {
		z.skillAttack(d.Type, src, src, target, d, lv, *flag)
	}
}

// breathAttackArea is skill_attack_area for the breaths: other units
// than the main target must be enemies the skill can reach. The breaths
// keep their animation on every unit.
func (z *Zone) breathAttackArea(src, t *entity, d *skill.Def, lv, flag int) int64 {
	if t.dead {
		return 0
	}
	if t != z.area.main && (!checkTarget(src, t, bctEnemy) || !z.statusCheckSkillUse(nil, t, d, 2)) {
		return 0
	}
	return z.skillAttack(d.Type, src, src, t, d, lv, flag)
}

// additionalEffect is the breaths' applyAdditionalEffects.
func (b npcBreath) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	if b.rate > 0 {
		z.scStart(src, target, b.status, b.rate, lv, int64(d.Duration2.At(lv)))
	}
}

// npcSplashHit is the recursive part of
// SkillImplRecursiveDamageSplash::castendDamageId, which
// skill_castend_damage_id runs with flag 1 on each target of a search:
// the hit takes the count and the main target that skill_area_temp
// holds. The original target and NPC skills (INF2_ISNPC) show no splash
// animation.
func (z *Zone) npcSplashHit(src, t *entity, d *skill.Def, lv, flag int) {
	sflag := z.area.count & 0xFFF
	if flag&sdLevel != 0 {
		sflag |= sdLevel
	}
	if z.area.main != t && !d.Has("IsNpc") {
		sflag |= sdAnimation
	}
	z.skillAttack(d.Type, src, src, t, d, lv, sflag)
}

// npcGrandDarkness is SkillGrandCrossOfDarkness: Grand Cross's cross of
// dark cells around the caster. Each hit may blind.
type npcGrandDarkness struct{ unitSkillImpl }

// additionalEffect is SkillGrandCrossOfDarkness::applyAdditionalEffects.
func (npcGrandDarkness) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart(src, target, sc.SCBlind, 100, lv, int64(d.Duration2.At(lv)))
}

// npcEvilLand is SkillEvilLand: a square of cells (skill_db's layout 1,
// so not Sanctuary's, which skill_init_unit_layout lists for a layout of
// -1 only) that hurt and may blind, and heal demons and the undead.
type npcEvilLand struct{ unitSkillImpl }

// additionalEffect is SkillEvilLand::applyAdditionalEffects.
func (npcEvilLand) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart(src, target, sc.SCBlind, 5*lv, lv, int64(d.Duration2.At(lv)))
}

// evilLandOnPlace is UNT_EVILLAND in skill_unit_onplace_timer: a misc
// hit on players and on monsters that are neither demons nor undead,
// when they are enemies. Other monsters are healed as by Sanctuary.
func (z *Zone) evilLandOnPlace(e, t *entity) {
	g := e.unit.group
	st := t.status
	if st == nil {
		return
	}
	if t.player != nil || !battle.IsUndead(st.Race, st.DefEle) && st.Race != battle.RaceDemon {
		if checkTarget(g.src, t, bctEnemy) {
			z.skillAttack(battle.TypeMisc, g.src, e, t, g.def, g.lv, 0)
		}
		return
	}
	srcSt := g.src.status
	if srcSt == nil {
		srcSt = &npcStatus
	}
	heal := skill.CalcHeal(srcSt, st, g.def.ID, g.lv, true, z.renewal(), z.rand)
	if t.hp >= int64(st.MaxHP) {
		return
	}
	if statusIsImmune(t) {
		heal = 0
	}
	z.skillEffect(e, t, z.skillsGet(skill.IDHeal), int(heal), heal, true)
	z.healHP(t, heal)
}

// npcEarthquake is SkillEarthquake: a unit on the caster that hits every
// enemy in its range on each interval.
type npcEarthquake struct{}

func (npcEarthquake) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, _ *int) {
	z.unitSetting(src, d, lv, x, y)
}

// earthquakeOnPlace is UNT_EARTHQUAKE in skill_unit_onplace_timer: the
// group counts its hits, and each hit splits its damage among the
// enemies of the unit in the skill's splash (map_foreachinallrange with
// skill_area_sub_count). The group's first hit carries
// NPC_EARTHQUAKE_FLAG.
func (z *Zone) earthquakeOnPlace(e, t *entity) {
	g := e.unit.group
	g.val1++
	n := 0
	q := &query{area: areaRadius, x: e.x, y: e.y, radius: g.def.SplashFor(g.lv), kinds: queryPlayers | queryMonsters}
	for _, o := range z.findEntities(q) {
		if checkTarget(g.src, o, bctEnemy) {
			n++
		}
	}
	flag := n
	if g.val1 == 1 {
		flag |= npcEarthquakeFlag
	}
	z.skillAttack(g.def.Type, g.src, e, t, g.def, g.lv, flag)
}

// npcEarthquakeFlag is NPC_EARTHQUAKE_FLAG.
const npcEarthquakeFlag = 0x800

// npcUnit is the monster skills that only place their unit, whose
// cells hit the enemies in range: SkillVenomFog, SkillNpcPsychicWave,
// SkillIceMine, SkillFlameCross, SkillHellBurning and
// SkillCaneOfEvilEye. flag 1 keeps ammunition.
type npcUnit struct{ unitSkillImpl }

// npcStormGust2 is SkillStormGust2: a Storm Gust unit whose hits may
// freeze.
type npcStormGust2 struct{ unitSkillImpl }

// additionalEffect is SkillStormGust2::applyAdditionalEffects: 10% at
// level 1, 7% at level 2, 3% above.
func (npcStormGust2) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	rate := 3
	switch lv {
	case 1:
		rate = 10
	case 2:
		rate = 7
	}
	z.scStart(src, target, sc.SCFreeze, rate, lv, int64(d.Duration2.At(lv)))
}

// npcCloudKill is SkillNpcCloudKill: one poison cloud per caster.
type npcCloudKill struct{}

// castendPos is SkillNpcCloudKill::castendPos2. skill_unitsetting first
// ends the caster's other clouds (skill_clear_group with flag 4).
func (npcCloudKill) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	*flag |= 4
	for _, g := range slices.Clone(src.groups) {
		if g.def.ID == skill.IDNPCCloudKill || g.def.ID == idSOCloudKill {
			z.deleteGroup(g)
		}
	}
	z.unitSetting(src, d, lv, x, y)
}

// idSOCloudKill is SO_CLOUD_KILL, which shares the cloud limit.
const idSOCloudKill = 2450

// cloudKillOnPlace is UNT_CLOUD_KILL in skill_unit_onplace_timer: the
// hit comes from the caster, at level -1 (SD_LEVEL), without animation.
func (z *Zone) cloudKillOnPlace(e, t *entity) {
	g := e.unit.group
	z.skillAttack(g.def.Type, g.src, g.src, t, g.def, g.lv, sdAnimation|sdLevel)
}

// npcComet is SkillComet2: a comet whose damage falls with the distance
// from its center, and burns.
type npcComet struct{ unitSkillImpl }

// additionalEffect is SkillComet2::applyAdditionalEffects.
func (npcComet) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart4(src, target, scBurning, 100, lv, 1000, int(z.unit(src).Ref().ID), 0, int64(d.Duration2.At(lv)))
}

// bindRatio gives SkillComet2's formula the distance from the target to
// the caster's last comet (sc->comet_x and comet_y). A caster without
// one counts 8.
func (npcComet) bindRatio(_ *Zone, src, _, target *entity) battle.SkillImpl {
	dist := 8
	if c := src.comet; c != nil {
		dist = max(abs(target.x-c[0]), abs(target.y-c[1]))
	}
	return skill.NPCComet{CenterDistance: dist}
}

// cometOnPlace is NPC_COMET in skill_unit_onplace_timer: nothing happens
// to a target on Magnetic Earth.
func (z *Zone) cometOnPlace(e, t *entity) {
	if z.landProtected(t.x, t.y) {
		return
	}
	g := e.unit.group
	z.skillAttack(g.def.Type, g.src, e, t, g.def, g.lv, 0)
}

// npcRainOfMeteor is SkillRainOfMeteor: meteors over the area, two in
// the first interval and one in each after. Each falls as Meteor Storm's
// do, one interval after it was placed.
type npcRainOfMeteor struct{}

func (npcRainOfMeteor) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	area := d.SplashFor(lv)
	interval := d.Unit.Interval
	if interval <= 0 {
		return
	}
	for i := 1; i <= d.Duration1.At(lv)/interval; i++ {
		if i == 1 {
			// The first meteor is at the center, the second near it.
			z.unitSettingFlag(src, d, lv, x, y, *flag+interval)
			tx := x - 1 + z.rand.IntN(3)
			ty := y - 1 + z.rand.IntN(3)
			z.unitSettingFlag(src, d, lv, tx, ty, *flag+interval)
			continue
		}
		tx := x - area + z.rand.IntN(area*2+1)
		ty := y - area + z.rand.IntN(area*2+1)
		z.unitSettingFlag(src, d, lv, tx, ty, *flag+i*interval)
	}
}

// npcJackFrost is SkillJackFrost2: a splash of the enemies around the
// target, which may freeze.
type npcJackFrost struct{}

// castendNoDamage is SkillJackFrost2::castendNoDamageId: every enemy in
// the splash around the target takes the recursive hit.
func (npcJackFrost) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	for _, t := range z.areaTargets(src, target.x, target.y, d.SplashFor(lv), true) {
		z.npcSplashHit(src, t, d, lv, *flag|1)
	}
}

// additionalEffect is SkillJackFrost2::applyAdditionalEffects: freeze at
// a rate of 200%, for Duration1.
func (npcJackFrost) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart(src, target, sc.SCFreeze, 200, lv, int64(d.Duration1.At(lv)))
}

// bindRatio gives SkillJackFrost2's formula whether the target has
// SC_FREEZING.
func (npcJackFrost) bindRatio(_ *Zone, _, _, target *entity) battle.SkillImpl {
	return skill.NPCJackFrost{Freezing: target.sc.Has(scFreezing)}
}

// npcRayOfGenesis is SkillNpcRayOfGenesis: castendNoDamageId runs
// skill_castend_damage_id on its target, the splash of
// SkillImplRecursiveDamageSplash. It runs the splash itself, so the end
// of the cast runs once.
type npcRayOfGenesis struct{ splashSkill }

func (r npcRayOfGenesis) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	r.castendDamage(z, src, target, d, lv, flag)
}

// additionalEffect is SkillNpcRayOfGenesis::applyAdditionalEffects:
// blindness at 50% below level 8 and 100% from it, for Duration1.
func (npcRayOfGenesis) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	rate := 100
	if lv < 8 {
		rate = 50
	}
	z.scStart(src, target, sc.SCBlind, rate, lv, int64(d.Duration1.At(lv)))
}

// npcPoisonBuster is SkillNpcPoisonBuster: a hit on a poisoned target,
// which ends its poison.
type npcPoisonBuster struct{}

func (npcPoisonBuster) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if target.sc.Has(sc.SCPoison) {
		z.skillAttack(d.Type, src, src, target, d, lv, *flag)
		z.scEnd(target, sc.SCPoison)
		return
	}
	if src.player != nil {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
	}
}

// npcIgnitionBreak is SkillNpcIgnitionBreak: a splash around the
// target, whose damage falls with the distance from the caster.
type npcIgnitionBreak struct{}

// castendNoDamage is SkillNpcIgnitionBreak::castendNoDamageId: there is
// no main target (skill_area_temp[1] is 0), and every enemy in the
// splash takes the recursive hit.
func (npcIgnitionBreak) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.area.main = nil
	z.skillEffect(src, target, d, lv, int64(lv), true)
	for _, t := range z.areaTargets(src, target.x, target.y, d.SplashFor(lv), true) {
		z.npcSplashHit(src, t, d, lv, *flag|sdSplash|1)
	}
}

// npcVenomImpress is SkillNpcVenomImpress: a weapon hit (the skill has
// no type of its own) that starts SC_VENOMIMPRESS for Duration1.
type npcVenomImpress struct{}

func (npcVenomImpress) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, *flag)
}

func (npcVenomImpress) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart(src, target, scVenomImpress, 100, lv, int64(d.Duration1.At(lv)))
}

// npcDragonBreath is SkillNpcDragonBreath: a ground skill whose area
// takes a weapon hit each, which may burn (levels 1 to 5) or freeze
// (levels 6 to 10).
type npcDragonBreath struct{}

// castendPos is SkillNpcDragonBreath::castendPos2: map_foreachinarea
// over the splash around the cell, each enemy through
// skill_castend_damage_id with flag 1.
func (b npcDragonBreath) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	z.area.x, z.area.y = x, y
	for _, t := range z.areaTargets(src, x, y, d.SplashFor(lv), false) {
		f := *flag | 1
		b.castendDamage(z, src, t, d, lv, &f)
	}
}

// castendDamage is SkillNpcDragonBreath::castendDamageId: a hiding
// target is not hit.
func (npcDragonBreath) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if target.sc.Has(sc.SCHiding) {
		z.skillEffect(src, src, d, lv, int64(lv), true)
		return
	}
	z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, *flag)
}

// additionalEffect is SkillNpcDragonBreath::applyAdditionalEffects, for
// Duration1.
func (npcDragonBreath) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	typ := scBurning
	if lv > 5 {
		typ = scFreezing
	}
	z.scStart4(src, target, typ, 50, lv, 1000, int(z.unit(src).Ref().ID), 0, int64(d.Duration1.At(lv)))
}

// npcMandragora is SkillNpcHowlingOfMandragora: players around the
// caster may get SC_MANDRAGORA and lose SP.
type npcMandragora struct{}

func (m npcMandragora) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if *flag&1 == 0 {
		// map_foreachinallrange: no wall check.
		for _, t := range z.areaTargets(src, target.x, target.y, d.SplashFor(lv), false) {
			f := *flag | 1
			z.castendNoDamage(src, t, d, lv, f)
		}
		z.skillEffect(src, src, d, lv, int64(lv), true)
		return
	}
	st := target.status
	if st == nil {
		return
	}
	rate := max(20*lv-(st.Vit+st.Luk)/5, 10)
	// It skips monsters, and a target that has the status already.
	if target.monster != nil || target.sc.Has(d.Status) {
		return
	}
	if z.rand.IntN(100) < rate {
		z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
		z.zap(target, 0, int64(st.MaxSP)*int64(25+5*lv)/100)
	}
}

// npcFatalMenace is SkillNpcFatalMenace: a weapon hit on the enemies
// around the target, then the caster warps to a random cell 800 ms
// later.
type npcFatalMenace struct{}

func (m npcFatalMenace) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if *flag&1 != 0 {
		z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, *flag)
		return
	}
	// map_search_freecell over the whole map.
	x, y, _ := z.searchFreeCell(-1, -1, -1, -1)
	z.area.x, z.area.y = x, y
	for _, t := range z.areaTargets(src, target.x, target.y, d.SplashFor(lv), true) {
		f := *flag | 1
		m.castendDamage(z, src, t, d, lv, &f)
	}
	z.addTimerSkill(src, z.now+800, src, x, y, d, lv, 0, *flag)
	z.skillEffect(src, src, d, lv, int64(lv), true)
}

// timerSkill is NPC_FATALMENACE in skill_timerskill: unit_warp to the
// cell.
func (npcFatalMenace) timerSkill(z *Zone, t *timerSkill) {
	z.unitWarp(t.src, t.x, t.y)
}

// npcCursedCircle is SkillNpcCursedCircle: enemies around the caster
// may be held (SC_CURSEDCIRCLE_TARGET), and the caster holds them
// (SC_CURSEDCIRCLE_ATKER).
type npcCursedCircle struct{}

func (c npcCursedCircle) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if *flag&1 != 0 {
		if target.status != nil && target.status.Class == battle.ClassBoss {
			return
		}
		if z.scStart4(src, target, d.Status, 50, lv, int(z.unit(src).Ref().ID), 0, 0, int64(d.Duration1.At(lv))) {
			if target.monster != nil {
				z.mobUnlockTarget(target)
			}
			*flag |= skillNoConsumeReq
		}
		return
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
	// map_forcountinrange: at most 15 targets, as monsters have no
	// spirit spheres.
	count := 0
	for _, t := range z.areaTargets(src, src.x, src.y, d.SplashFor(lv), true) {
		if count >= 15 {
			break
		}
		f := *flag | 1
		c.castendNoDamage(z, src, t, d, lv, &f)
		count++
	}
	ok := z.scStart4(src, src, scCursedCircleAtker, 50, lv, count, 0, 0, int64(d.Duration1.At(lv)))
	z.skillEffect(src, src, d, lv, int64(lv), ok)
}

// npcMagmaEruption is SkillNpcMagmaEruption: a weapon slam on the area,
// which may stun, then an eruption unit two attack motions later.
type npcMagmaEruption struct{}

// castendPos is SkillNpcMagmaEruption::castendPos2: map_foreachinarea
// over the splash, each enemy through skill_castend_damage_id with
// flag 1 and the splash animation.
func (npcMagmaEruption) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	for _, t := range z.areaTargets(src, x, y, d.SplashFor(lv), false) {
		z.skillAttack(battle.TypeWeapon, src, src, t, d, lv, *flag|sdAnimation|1)
	}
	amotion := 0
	if src.status != nil {
		amotion = src.status.AMotion
	}
	z.addTimerSkill(src, z.now+int64(amotion)*2, nil, x, y, d, lv, 0, *flag)
}

// timerSkill is NPC_MAGMA_ERUPTION in skill_timerskill: the eruption
// unit.
func (npcMagmaEruption) timerSkill(z *Zone, t *timerSkill) {
	z.unitSetting(t.src, t.def, t.lv, t.x, t.y)
}

// additionalEffect is SkillNpcMagmaEruption::applyAdditionalEffects: the
// slam may stun.
func (npcMagmaEruption) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart(src, target, sc.SCStun, 90, lv, int64(d.Duration2.At(lv)))
}

// magmaEruptionOnPlace is UNT_MAGMA_ERUPTION in skill_unit_onplace_timer
// for NPC_MAGMA_ERUPTION: a hit of NC_MAGMA_ERUPTION_DOTDAMAGE, then one
// of NPC_MAGMA_ERUPTION_DOTDAMAGE.
func (z *Zone) magmaEruptionOnPlace(e, t *entity) {
	g := e.unit.group
	for _, id := range []int{idNCMagmaEruptionDot, idNPCMagmaEruptionDot} {
		if d := z.skillsGet(id); d != nil {
			z.skillAttack(d.Type, g.src, e, t, d, g.lv, 0)
		}
	}
}

// ncMagmaEruptionDot is SkillMagmaEruptionDotDamage, the eruption hit of
// NC_MAGMA_ERUPTION, which NPC_MAGMA_ERUPTION's unit also deals.
type ncMagmaEruptionDot struct{}

// additionalEffect is SkillMagmaEruptionDotDamage::applyAdditionalEffects:
// it may burn.
func (ncMagmaEruptionDot) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart4(src, target, scBurning, 10*lv, lv, 1000, int(z.unit(src).Ref().ID), 0, int64(d.Duration2.At(lv)))
}

// npcPropertyWalk is SkillNpcElectricWalk and SkillNpcFireWalk:
// SC_PROPERTYWALK on the caster, which leaves the skill's unit on the
// cells it walks from.
type npcPropertyWalk struct{}

// castendPos is castendPos2: the status starts again, with the skill in
// val1 and the level in val2, for Duration1.
func (npcPropertyWalk) castendPos(z *Zone, src *entity, _, _ int, d *skill.Def, lv int, _ *int) {
	if src.sc.Has(d.Status) {
		z.scEnd(src, d.Status)
	}
	z.scStart4(src, src, d.Status, 100, d.ID, lv, 0, 0, int64(d.Duration1.At(lv)))
}

// propertyWalkMoved is SC_PROPERTYWALK's part of map_moveblock, after a
// unit moved from (x0, y0):
//   - When it placed its count of cells, the status ends.
//   - Otherwise, unless a walk unit is on its new cell, it places the
//     skill's unit on the cell it left, and counts it in val3.
func (z *Zone) propertyWalkMoved(e *entity, x0, y0 int) {
	en := e.sc.Get(scPropertyWalk)
	if en == nil {
		return
	}
	d := z.skillsGet(en.Val1)
	if d == nil {
		return
	}
	if en.Val3 >= d.ActiveInstance.At(en.Val2) {
		z.scEnd(e, scPropertyWalk)
		return
	}
	for _, id := range []int{idSOElectricWalk, skill.IDNPCElectricWalk, idSOFireWalk, skill.IDNPCFireWalk} {
		if z.unitOnCell(e, e.x, e.y, id, false) != nil {
			return
		}
	}
	if z.unitSetting(e, d, en.Val2, x0, y0) != nil {
		en.Val3++
	}
}

// ratioBinder is a skill whose formula reads the zone: calcSkillAttack
// asks it for the SkillImpl of each hit. dsrc is where the hit comes
// from.
type ratioBinder interface {
	bindRatio(z *Zone, src, dsrc, target *entity) battle.SkillImpl
}

// flameCrossLayout is NPC_FLAMECROSS's layout, from
// skill_init_unit_layout: a cross of 8 cells 2 out on each axis, without
// its center.
var flameCrossLayout = layoutOf(
	[]int{-2, -1, 1, 2, 0, 0, 0, 0},
	[]int{0, 0, 0, 0, -2, -1, 1, 2})

// magicalAttack is SC_MAGICALATTACK's part of battle_weapon_attack: the
// normal attack is NPC_MAGICALATTACK at the status's level instead. It
// reports whether the attack was replaced.
func (z *Zone) magicalAttack(src, target *entity) bool {
	e := src.sc.Get(scMagicalAttack)
	if e == nil {
		return false
	}
	if d := z.skillsGet(skill.IDNPCMagicalAttack); d != nil {
		z.skillAttack(battle.TypeMagic, src, src, target, d, e.Val1, 0)
	}
	return true
}

// reverberationOnPlace is UNT_REVERBERATION in skill_unit_onplace_timer:
// the trap goes off. Each target in its splash (skill_trap_splash) takes
// NPC_REVERBERATION_ATK 50 ms later, and the trap ends a second later.
//
// rAthena also sets it off from skill_unit_onplace, when a unit steps
// on it, with 1.5 seconds to end. The zone's units act from the unit
// timer only, so the trap goes off within 100 ms instead. Hits on the
// trap (its val1) need units that can be attacked, which the zone does
// not have.
func (z *Zone) reverberationOnPlace(e *entity) {
	g := e.unit.group
	atk := z.skillsGet(skill.IDNPCReverberationAtk)
	q := &query{area: areaRadius, x: e.x, y: e.y, radius: g.def.SplashFor(g.lv), kinds: queryPlayers | queryMonsters,
		lineOfSight: true}
	for _, t := range z.findEntities(q) {
		if atk == nil || !checkTarget(g.src, t, g.targetFlag) {
			continue
		}
		z.addTimerSkill(g.src, z.now+50, t, 0, 0, atk, g.lv, battle.TypeWeapon, 0)
	}
	g.limit = z.now - g.start + 1000
	g.unitID = untUsedTraps
}

// npcReverberationAtk is SkillReverberationAttack: a splash around each
// target of the trap, which splits its damage.
type npcReverberationAtk struct{ splashSkill }

// timerSkill is NPC_REVERBERATION_ATK in skill_timerskill:
// skill_castend_damage_id with SD_LEVEL and SD_ANIMATION, which runs the
// splash. Its splashSearch clears the main target, which only changes
// the animation, and NPC skills show none.
func (r npcReverberationAtk) timerSkill(z *Zone, t *timerSkill) {
	if t.src.dead || t.target.dead {
		return
	}
	flag := t.flag | sdLevel | sdAnimation
	r.castendDamage(z, t.src, t.target, t.def, t.lv, &flag)
}
