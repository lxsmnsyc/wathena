package zone

import (
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Monster skills that cause statuses: the NPC_ skills of rAthena's
// src/map/skills/npc that hit with a status, start a status on the
// target or on every enemy around the monster, or drain. Their damage
// formulas are in server/skill/npc_status.go.

// Skill IDs of the monster skills that cause statuses, from rAthena's
// skill_db.yml. Some exist only in renewal.
const (
	idNPCPoison            = 176 // NPC_POISON
	idNPCBlindAttack       = 177 // NPC_BLINDATTACK
	idNPCSilenceAttack     = 178 // NPC_SILENCEATTACK
	idNPCStunAttack        = 179 // NPC_STUNATTACK
	idNPCPetrifyAttack     = 180 // NPC_PETRIFYATTACK
	idNPCCurseAttack       = 181 // NPC_CURSEATTACK
	idNPCSleepAttack       = 182 // NPC_SLEEPATTACK
	idNPCLick              = 206 // NPC_LICK
	idNPCHallucination     = 207 // NPC_HALLUCINATION
	idNPCStop              = 342 // NPC_STOP
	idNPCDragonFear        = 659 // NPC_DRAGONFEAR
	idNPCBleeding          = 660 // NPC_BLEEDING
	idNPCWideSilence       = 663 // NPC_WIDESILENCE
	idNPCWideFreeze        = 664 // NPC_WIDEFREEZE
	idNPCWideBleeding      = 665 // NPC_WIDEBLEEDING
	idNPCWideStone         = 666 // NPC_WIDESTONE
	idNPCWideConfuse       = 667 // NPC_WIDECONFUSE
	idNPCWideSleep         = 668 // NPC_WIDESLEEP
	idNPCSlowCast          = 672 // NPC_SLOWCAST
	idNPCWideCurse         = 677 // NPC_WIDECURSE
	idNPCWideStun          = 678 // NPC_WIDESTUN
	idNPCWideSoulDrain     = 680 // NPC_WIDESOULDRAIN
	idNPCWideHellDignity   = 684 // NPC_WIDEHELLDIGNITY
	idNPCWideWeb           = 721 // NPC_WIDEWEB
	idNPCWideSuck          = 722 // NPC_WIDESUCK
	idNPCLexAeterna        = 727 // NPC_LEX_AETERNA
	idNPCHallucinationWalk = 743 // NPC_HALLUCINATIONWALK
	idNPCAllStatDown       = 751 // NPC_ALL_STAT_DOWN
	idNPCGradualGravity    = 752 // NPC_GRADUAL_GRAVITY
	idNPCWideBleeding2     = 756 // NPC_WIDEBLEEDING2
	idNPCWideSilence2      = 757 // NPC_WIDESILENCE2
	idNPCWideStone2        = 759 // NPC_WIDESTONE2
	idNPCWideCurse2        = 761 // NPC_WIDECURSE2
	idNPCWideConfuse2      = 762 // NPC_WIDECONFUSE2
	idNPCWideFreeze2       = 763 // NPC_WIDEFREEZE2
	idNPCBleeding2         = 764 // NPC_BLEEDING2
	idNPCDeadlyCurse2      = 779 // NPC_DEADLYCURSE2
)

// npcEarthquakeID is NPC_EARTHQUAKE, whose hits on several targets
// SC_NPC_HALLUCINATIONWALK does not evade.
const npcEarthquakeID = 653

// scNPCHallucinationWalk is SC_NPC_HALLUCINATIONWALK, whose val3 is the
// chance to evade a magic hit.
const scNPCHallucinationWalk = 758

func init() {
	registerSkillImpls(map[int]any{
		idNPCPoison:            npcStatusAttack{},
		idNPCBlindAttack:       npcStatusAttack{},
		idNPCSilenceAttack:     npcStatusAttack{},
		idNPCStunAttack:        npcStatusAttack{},
		idNPCPetrifyAttack:     npcStatusAttack{},
		idNPCCurseAttack:       npcStatusAttack{},
		idNPCSleepAttack:       npcStatusAttack{},
		idNPCBleeding:          npcStatusAttack{},
		idNPCBleeding2:         npcStatusAttack{},
		idNPCLick:              npcLick{},
		idNPCHallucination:     npcHallucination{},
		idNPCStop:              npcStop{},
		idNPCDragonFear:        npcDragonFear{},
		idNPCSlowCast:          npcWideStatus{},
		idNPCWideSilence:       npcWideStatus{},
		idNPCWideSilence2:      npcWideStatus{},
		idNPCWideFreeze:        npcWideStatus{},
		idNPCWideFreeze2:       npcWideStatus{},
		idNPCWideCurse:         npcWideStatus{},
		idNPCWideCurse2:        npcWideStatus{},
		idNPCWideConfuse:       npcWideStatus{},
		idNPCWideConfuse2:      npcWideStatus{},
		idNPCWideBleeding:      npcWideStatus{},
		idNPCWideBleeding2:     npcWideStatus{},
		idNPCWideStone:         npcWideStatus{delay: true},
		idNPCWideStone2:        npcWideStatus{delay: true},
		idNPCWideStun:          npcWideStatus{},
		idNPCWideSleep:         npcWideStatus{},
		idNPCWideHellDignity:   npcWideStatus{},
		idNPCWideWeb:           npcWideStatus{},
		idNPCDeadlyCurse2:      npcWideStatus{},
		idNPCWideSoulDrain:     npcWideSoulDrain{},
		idNPCWideSuck:          npcWideSuck{},
		idNPCLexAeterna:        npcLexAeterna{},
		idNPCHallucinationWalk: npcStatusSkill{},
		idNPCGradualGravity:    npcForcedStatus{},
		idNPCAllStatDown:       npcForcedStatus{},
	})
}

// npcAttackStatus is the status each status attack may cause. rAthena
// names it in each class rather than reading skill_get_sc.
var npcAttackStatus = map[int]int{
	idNPCPoison:        sc.SCPoison,
	idNPCBlindAttack:   sc.SCBlind,
	idNPCSilenceAttack: sc.SCSilence,
	idNPCStunAttack:    sc.SCStun,
	idNPCPetrifyAttack: sc.SCStonewait,
	idNPCCurseAttack:   sc.SCCurse,
	idNPCSleepAttack:   sc.SCSleep,
	idNPCBleeding:      sc.SCBleeding,
	idNPCBleeding2:     sc.SCBleeding,
}

// npcStatusAttack is the weapon attacks that may cause a status, as
// WeaponSkillImpl with applyAdditionalEffects: SkillPoisonAttack,
// SkillBlindAttack, SkillSilenceAttack, SkillStunAttack,
// SkillPetrifyAttack, SkillCurseAttack, SkillSleepAttack, SkillBleeding
// and SkillBleeding2.
type npcStatusAttack struct{ attackSkill }

func (npcStatusAttack) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	typ := npcAttackStatus[d.ID]
	switch d.ID {
	case idNPCPetrifyAttack:
		// sc_start2 of SC_STONEWAIT: stone after Duration1.
		z.npcScStartDelay(src, target, typ, 20*lv, lv, int64(d.Duration2.At(lv)), int64(d.Duration1.At(lv)))
	case idNPCBleeding2:
		z.scStart(src, target, typ, 50+10*lv, lv, int64(d.Duration2.At(lv)))
	default:
		z.scStart(src, target, typ, 20*lv, lv, int64(d.Duration2.At(lv)))
	}
}

// npcScStartDelay is sc_start2 with a delay: val2 is the caster's ID,
// and the status starts after delay ms, as SC_STONEWAIT.
func (z *Zone) npcScStartDelay(src, target *entity, typ, rate, lv int, duration, delay int64) bool {
	en := z.statusEngine()
	if en == nil || typ < 0 {
		return false
	}
	su := z.unit(src)
	return en.Start(su, z.unit(target), sc.Start{Type: typ, Rate: rate * 100, Val1: lv, Val2: int(su.Ref().ID),
		Duration: duration, Delay: delay, Reason: sc.ReasonSkill, Source: su.Ref()})
}

// npcLick is SkillLick: 100 SP gone, and the skill's status.
type npcLick struct{}

func (npcLick) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	z.zap(target, 0, 100)
	ok := z.scStart(src, target, d.Status, lv*20, lv, int64(d.Duration2.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// npcHallucination is SkillHallucination: the skill's status at 20% per
// level.
type npcHallucination struct{}

func (npcHallucination) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	ok := z.scStart(src, target, d.Status, lv*20, lv, int64(d.Duration2.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// npcStop is SkillStop: the target and the monster both stop. Each
// SC_STOP's val2 names the other, so that the end of one ends the other.
type npcStop struct{}

func (npcStop) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	en := z.statusEngine()
	if en == nil {
		return
	}
	su, tu := z.unit(src), z.unit(target)
	dur := int64(d.Duration1.At(lv))
	ok := en.Start(su, tu, sc.Start{Type: d.Status, Rate: 10000, Val1: lv, Val2: int(su.Ref().ID), Duration: dur,
		Reason: sc.ReasonSkill, Source: su.Ref()})
	z.skillEffect(src, target, d, lv, int64(lv), ok)
	if ok {
		// rAthena finds the partner by val2. The status's source names it
		// here, since unit IDs are per kind (see SC_STOP in
		// statuschange's end).
		en.Start(su, su, sc.Start{Type: d.Status, Rate: 10000, Val1: lv, Val2: int(tu.Ref().ID), Duration: dur,
			Reason: sc.ReasonSkill, Source: tu.Ref()})
	}
}

// npcDragonFear is SkillDragonFear: every enemy in the splash gets one
// of four statuses, picked at random. When the pick fails, the next one
// in the list is tried, until one starts or all failed. The duration is
// Duration2 at the pick's place in the list, not at the skill's level.
type npcDragonFear struct{}

var npcDragonFearStatuses = [...]int{sc.SCStun, sc.SCSilence, sc.SCConfusion, sc.SCBleeding}

func (npcDragonFear) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if *flag&1 != 0 {
		n := len(npcDragonFearStatuses)
		i := z.rand.IntN(n)
		j := i
		id := int(z.unit(src).Ref().ID)
		for !z.scStart4(src, target, npcDragonFearStatuses[i], 100, lv, id, 0, 0, int64(d.Duration2.At(i+1))) {
			i++
			if i == n {
				i = 0
			}
			if i == j {
				break
			}
		}
		return
	}
	z.npcWideArea(src, target, d, lv, flag)
}

// npcWideArea is the area half of the wide NPC skills:
// clif_skill_nodamage on the target, then skill_castend_nodamage_id with
// flag 1 on every enemy in the splash around it (map_foreachinallrange,
// no wall check).
func (z *Zone) npcWideArea(src, target *entity, d *skill.Def, lv int, flag *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	for _, t := range z.areaTargets(src, target.x, target.y, d.SplashFor(lv), false) {
		z.castendNoDamage(src, t, d, lv, *flag|bctEnemy|sdPreamble|1)
	}
}

// npcWideStatus is the wide status skills that start the skill's status
// at 100% on every enemy in the splash: SkillSlowCast, SkillWideSilence,
// SkillWideFreeze, SkillWideCurse, SkillWideConfusion,
// SkillWideBleeding, SkillWidePetrify, SkillWideStun, SkillWideSleep,
// SkillHellDignity, SkillWideWeb, SkillDeadlyCurse2 and their second
// versions. With delay set (SkillWidePetrify), the status starts after
// Duration1, as SC_STONEWAIT. SkillDeadlyCurse2's SC_DEADLY_DEFEASANCE
// starts, but its effects wait for bNoMagicDamage and magic reflection,
// which are not ported.
type npcWideStatus struct{ delay bool }

func (w npcWideStatus) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if *flag&1 == 0 {
		z.npcWideArea(src, target, d, lv, flag)
		return
	}
	if w.delay {
		z.npcScStartDelay(src, target, d.Status, 100, lv, int64(d.Duration2.At(lv)), int64(d.Duration1.At(lv)))
		return
	}
	z.scStart4(src, target, d.Status, 100, lv, int(z.unit(src).Ref().ID), 0, 0, int64(d.Duration2.At(lv)))
}

// npcWideSoulDrain is SkillWideSoulDrain: every enemy in the splash
// loses 20% of its SP per level, from 1 to 5 and again from 6.
// rAthena also lets it reach dead targets; the area search here skips
// them.
type npcWideSoulDrain struct{}

func (npcWideSoulDrain) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if *flag&1 == 0 {
		z.npcWideArea(src, target, d, lv, flag)
		return
	}
	// status_percent_damage of SP: a percent of the current SP.
	z.takeSP(target, z.percentSP(target, ((lv-1)%5+1)*20))
}

// npcWideSuck is SkillWideSuck: it places the skill's ground unit,
// whose hits heal the monster (skill_unit_onplace_timer). flag 1 keeps
// ammunition. skill_db.yml gives the skill no unit, so no unit is placed.
type npcWideSuck struct{}

func (npcWideSuck) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	*flag |= 1
	z.unitSetting(src, d, lv, x, y)
}

// npcWideSuckOnPlace is skill_unit_onplace_timer for NPC_WIDESUCK: the
// hit's damage heals the caster.
func (z *Zone) npcWideSuckOnPlace(e, t *entity) {
	g := e.unit.group
	heal := int64(int32(z.skillAttack(g.def.Type, g.src, e, t, g.def, g.lv, 0))) // #nosec G115 -- rAthena keeps the heal in an int32.
	if heal > 0 {
		z.skillEffect(g.src, t, g.def, g.lv, int64(g.lv), true)
		z.healHP(g.src, heal)
	}
}

// npcLexAeterna is SkillLexAeterna2: Lex Aeterna level 1 on every enemy
// in the square around the cell (map_foreachinallarea).
type npcLexAeterna struct{}

func (npcLexAeterna) castendPos(z *Zone, src *entity, x, y int, d *skill.Def, lv int, flag *int) {
	lex := z.Skills.Get(skill.IDLexAeterna)
	if lex == nil {
		return
	}
	i := d.SplashFor(lv)
	for _, t := range z.findEntities(&query{area: areaRect, x0: x - i, y0: y - i, x1: x + i, y1: y + i,
		kinds: queryPlayers | queryMonsters, enemiesOf: src}) {
		z.castendNoDamage(src, t, lex, 1, *flag|bctEnemy|1)
	}
}

// npcStatusSkill is StatusSkillImpl for NPC_HALLUCINATIONWALK: the
// skill's status for Duration1. A skill without a status (SC_NONE, as in
// pre-renewal) does nothing.
type npcStatusSkill struct{}

func (npcStatusSkill) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if d.Status < 0 {
		return
	}
	ok := z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// npcForcedStatus is SkillIncreasedGravity and SkillDecreaseAllStats:
// status_change_start of the skill's status for Duration1 with
// SCSTART_NOAVOID, SCSTART_NOTICKDEF and SCSTART_NORATEDEF.
// SkillDecreaseAllStats also sends clif_skill_damage without damage,
// which is only a motion.
type npcForcedStatus struct{}

func (npcForcedStatus) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	en := z.statusEngine()
	if en == nil || d.Status < 0 {
		return
	}
	su := z.unit(src)
	en.Start(su, z.unit(target), sc.Start{Type: d.Status, Rate: 10000, Val1: lv, Duration: int64(d.Duration1.At(lv)),
		Flag: sc.NoAvoid | sc.NoTickDef | sc.NoRateDef, Reason: sc.ReasonSkill, Source: su.Ref()})
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// npcHallucinationWalkEvade is the part of skill_attack for a magic hit
// on a unit under SC_NPC_HALLUCINATIONWALK: val3 percent of hits miss.
func (z *Zone) npcHallucinationWalkEvade(target *entity, damage int64) bool {
	e := target.sc.Get(scNPCHallucinationWalk)
	return damage != 0 && e != nil && z.rand.IntN(100) < e.Val3
}
