package zone

import (
	"math"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// Monster skills that only monsters use: the NPC_ self skills and
// utilities of rAthena's src/map/skills/npc. They heal, buff or change
// the monster, kill it, or move it and its target. Their formulas are in
// server/skill/npc_buff.go.

// Skill IDs of the monster self skills.
const (
	idNpcAttriChange       = 161 // NPC_ATTRICHANGE
	idNpcChangeWater       = 162 // NPC_CHANGEWATER
	idNpcChangeGround      = 163 // NPC_CHANGEGROUND
	idNpcChangeFire        = 164 // NPC_CHANGEFIRE
	idNpcChangeWind        = 165 // NPC_CHANGEWIND
	idNpcChangePoison      = 166 // NPC_CHANGEPOISON
	idNpcChangeHoly        = 167 // NPC_CHANGEHOLY
	idNpcChangeDarkness    = 168 // NPC_CHANGEDARKNESS
	idNpcChangeTelekinesis = 169 // NPC_CHANGETELEKINESIS
	idNpcSelfDestruction   = 173 // NPC_SELFDESTRUCTION
	idNpcSuicide           = 175 // NPC_SUICIDE
	idNpcProvocation       = 194 // NPC_PROVOCATION
	idNpcSmoking           = 195 // NPC_SMOKING
	idNpcKeeping           = 201 // NPC_KEEPING
	idNpcDarkBlessing      = 203 // NPC_DARKBLESSING
	idNpcBarrier           = 204 // NPC_BARRIER
	idNpcDefender          = 205 // NPC_DEFENDER
	idNpcChangeUndead      = 348 // NPC_CHANGEUNDEAD
	idNpcPowerUp           = 349 // NPC_POWERUP
	idNpcAgiUp             = 350 // NPC_AGIUP
	idNpcInvisible         = 353 // NPC_INVISIBLE
	idNpcRun               = 354 // NPC_RUN
	idNpcMagicMirror       = 671 // NPC_MAGICMIRROR
	idNpcExpulsion         = 674 // NPC_EXPULSION
	idNpcStoneSkin         = 675 // NPC_STONESKIN
	idNpcAntiMagic         = 676 // NPC_ANTIMAGIC
	idNpcVampireGift       = 679 // NPC_VAMPIRE_GIFT
	idNpcTalk              = 682 // NPC_TALK
	idNpcHellPower         = 683 // NPC_HELLPOWER
	idNpcInvincible        = 685 // NPC_INVINCIBLE
	idNpcAllHeal           = 687 // NPC_ALLHEAL
	idNpcMillenniumShield  = 707 // NPC_MILLENNIUMSHIELD
	idNpcMaxPain           = 716 // NPC_MAXPAIN
	idNpcCHeal             = 729 // NPC_CHEAL
	idNpcDamageHeal        = 753 // NPC_DAMAGE_HEAL, renewal only
	idNpcImmuneProperty    = 754 // NPC_IMMUNE_PROPERTY, renewal only
	idNpcMoveCoordinate    = 755 // NPC_MOVE_COORDINATE, renewal only
)

// Statuses of the monster self skills that the zone reads.
const (
	scMillenniumshield = 314 // SC_MILLENNIUMSHIELD
	scDamageHeal       = 916 // SC_DAMAGE_HEAL
)

// immuneProperty are SkillPropertyImmune's statuses by level, from
// SC_IMMUNE_PROPERTY_NOTHING to SC_IMMUNE_PROPERTY_UNDEAD.
var immuneProperty = [10]int{917, 918, 919, 920, 921, 924, 923, 922, 925, 926}

func init() {
	registerSkillImpls(map[int]any{
		idNpcAllHeal:           npcFullHeal{},
		idNpcCHeal:             npcColuceoHeal{},
		idNpcAgiUp:             npcStatus2{val1: 50, val2: 100},
		idNpcPowerUp:           npcStatus2{val1: 200, val2: 100},
		idNpcInvisible:         npcInvisible{},
		idNpcDefender:          statusSkill{},
		idNpcStoneSkin:         npcArmorChange{},
		idNpcAntiMagic:         npcArmorChange{},
		idNpcMaxPain:           statusSkill{},
		idNpcMagicMirror:       statusSkill{},
		idNpcKeeping:           statusSkill{},
		idNpcBarrier:           statusSkill{},
		idNpcInvincible:        statusSkill{},
		idNpcDamageHeal:        statusSkill{},
		idNpcMillenniumShield:  npcMilleniumShield2{},
		idNpcAttriChange:       npcAttributeChange{},
		idNpcChangeWater:       npcAttributeChange{},
		idNpcChangeGround:      npcAttributeChange{},
		idNpcChangeFire:        npcAttributeChange{},
		idNpcChangeWind:        npcAttributeChange{},
		idNpcChangePoison:      npcAttributeChange{},
		idNpcChangeHoly:        npcAttributeChange{},
		idNpcChangeDarkness:    npcAttributeChange{},
		idNpcChangeTelekinesis: npcAttributeChange{},
		idNpcChangeUndead:      npcUndeadAttributeChange{},
		idNpcImmuneProperty:    npcPropertyImmune{},
		idNpcSelfDestruction:   npcSuicideBombing{},
		idNpcSuicide:           npcSuicide{},
		idNpcVampireGift:       npcVampireGift{},
		idNpcSmoking:           npcSmoking{},
		idNpcMoveCoordinate:    npcChangeLocation{},
		idNpcRun:               npcRun{},
		idNpcProvocation:       npcProvocation{},
		idNpcTalk:              npcTalk{},
		idNpcExpulsion:         npcExpulsion{},
		idNpcDarkBlessing:      npcDarkBlessing{},
		idNpcHellPower:         npcHellPower{},
	})
}

// npcFullHeal is SkillFullHeal (NPC_ALLHEAL): the target heals to full
// HP, and a monster forgets who hurt it.
type npcFullHeal struct{}

func (npcFullHeal) castendNoDamage(z *Zone, _, target *entity, _ *skill.Def, _ int, _ *int) {
	if statusIsImmune(target) || target.status == nil {
		return
	}
	// status_percent_heal: status_heal of the max HP, which returns what
	// it healed.
	before := target.hp
	z.healHP(target, int64(target.status.MaxHP))
	heal := target.hp - before
	// clif_skill_nodamage without a source shows AL_HEAL on the target.
	if d := z.skillsGet(skill.IDHeal); d != nil {
		z.skillEffect(target, target, d, int(heal), heal, true)
	}
	if m := target.monster; m != nil {
		m.dmglog.Entries = nil
	}
}

// npcColuceoHeal is SkillNpcColuceoHeal (NPC_CHEAL): Heal at level 10 on
// the caster and every monster on its side within the splash.
type npcColuceoHeal struct{}

func (npcColuceoHeal) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if *flag&1 != 0 {
		tst := target.status
		// rAthena also needs the target's status change data, which
		// every unit of the zone has.
		if tst == nil || src.status == nil || battle.IsUndead(tst.Race, tst.DefEle) || target.sc.Has(sc.SCBerserk) {
			return
		}
		i := skill.CalcHeal(src.status, tst, skill.IDHeal, 10, true, z.rules().HealFormula, z.rand)
		if statusIsImmune(target) {
			i = 0
		}
		z.skillEffect(src, target, d, lv, i, true)
		// SC_AKAITSUKI, which turns the heal into damage, is not ported.
		z.healHP(target, i)
		return
	}
	// map_foreachinallrange over BL_MOB with BCT_PARTY: the caster and
	// the monsters of its side, without the wall check.
	for _, t := range z.findEntities(&query{area: areaRadius, x: src.x, y: src.y, radius: d.SplashFor(lv),
		kinds: queryMonsters, alliesOf: src}) {
		z.castendNoDamage(src, t, d, lv, *flag|1)
	}
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// npcStatus2 is SkillAgilityUp (NPC_AGIUP) and SkillPowerUp
// (NPC_POWERUP): sc_start2 with fixed values.
//   - NPC_AGIUP: 50% more speed and 100% more flee.
//   - NPC_POWERUP: 200% more ATK and 100% more hit.
type npcStatus2 struct{ val1, val2 int }

func (s npcStatus2) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	ok := z.scStart4(src, target, d.Status, 100, s.val1, s.val2, 0, 0, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// npcInvisible is SkillInvisible (NPC_INVISIBLE): SC_CLOAKING with val4
// 6, a cloak that attacks and skills do not end.
type npcInvisible struct{}

func (npcInvisible) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	ok := z.scStart4(src, target, d.Status, 100, lv, 0, 0, 6, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// npcArmorChange is SkillStoneSkin (NPC_STONESKIN) and SkillAntiMagic
// (NPC_ANTIMAGIC): SC_ARMORCHANGE with the skill ID as val2, which tells
// the status which damage it reduces.
type npcArmorChange struct{}

func (npcArmorChange) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	ok := z.scStart4(src, target, d.Status, 100, lv, d.ID, 0, 0, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// npcMilleniumShield2 is SkillMilleniumShield2 (NPC_MILLENNIUMSHIELD):
// the effect shows only when the status starts.
type npcMilleniumShield2 struct{}

func (npcMilleniumShield2) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if z.scStart(src, target, d.Status, 100, lv, int64(d.Duration1.At(lv))) {
		z.skillEffect(src, target, d, lv, int64(lv), true)
	}
}

// npcAttributeChange is SkillAttributeChange (NPC_ATTRICHANGE) and the
// element changes of one element, such as SkillWaterAttributeChange:
// SC_ELEMENTALCHANGE with the skill's element. NPC_ATTRICHANGE has none,
// so the status picks one.
type npcAttributeChange struct{}

func (npcAttributeChange) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	ok := z.scStart4(src, target, d.Status, 100, lv, d.Element.At(lv), 0, 0, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// npcUndeadAttributeChange is SkillUndeadAttributeChange
// (NPC_CHANGEUNDEAD): a weapon hit that may give the target the undead
// element.
type npcUndeadAttributeChange struct{ attackSkill }

func (npcUndeadAttributeChange) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart(src, target, sc.SCChangeundead, 10*lv, lv, int64(d.Duration2.At(lv)))
}

// npcPropertyImmune is SkillPropertyImmune (NPC_IMMUNE_PROPERTY): the
// level picks the element the unit takes no damage from.
type npcPropertyImmune struct{}

func (npcPropertyImmune) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	typ := d.Status
	if lv >= 1 && lv <= len(immuneProperty) {
		typ = immuneProperty[lv-1]
	}
	ok := z.scStart(src, target, typ, 100, lv, int64(d.Duration1.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// npcSuicideBombing is SkillSuicideBombing (NPC_SELFDESTRUCTION): the
// caster hits every character around it for its HP, then dies, with its
// exp and drops.
type npcSuicideBombing struct{}

func (npcSuicideBombing) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	if target.sc.Has(sc.SCHiding) {
		return
	}
	if src != target {
		// Bit 1 only marks the call from the area loop.
		z.skillAttack(d.Type, src, src, target, d, lv, *flag&^1)
	}
}

func (npcSuicideBombing) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	// Monsters hit everyone (BCT_ALL). A player and a marine sphere hit
	// enemies only, outside versus maps, which do not exist yet.
	// alchemist_summon_setting 8, which adds the caster's slaves, is off.
	q := &query{area: areaRadius, x: target.x, y: target.y, radius: d.SplashFor(lv),
		kinds: queryPlayers | queryMonsters, lineOfSight: true, exclude: map[*entity]bool{src: true}}
	if m := src.monster; m == nil || m.specialAI == aiSphere {
		q.enemiesOf = src
	}
	z.skillEffect(src, src, d, lv, int64(lv), true)
	// map_delblock: the caster is out of its own search, so a chain of
	// self destructions does not hit back. Skill units (BL_SKILL), which
	// rAthena hits too, take no damage in the zone yet.
	for _, t := range z.findEntities(q) {
		z.castendDamage(src, t, d, lv, *flag|1)
	}
	// status_zap: no damage shows, but the monster drops its items and
	// gives exp.
	if !src.dead {
		z.zap(src, src.hp, 0)
	}
}

// npcSuicide is SkillNpcSuicide (NPC_SUICIDE): the monster dies without
// exp or drops (status_kill).
type npcSuicide struct{}

func (npcSuicide) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	if m := src.monster; m != nil {
		m.noRewards = true
	}
	z.zap(src, src.hp, 0)
}

// npcVampireGift is SkillVampireGift (NPC_VAMPIRE_GIFT): a weapon splash
// around the caster that heals it by the damage of each hit.
type npcVampireGift struct{}

// castendDamage is SkillImplRecursiveDamageSplash::castendDamageId.
func (npcVampireGift) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.recursiveSplash(src, target, d, lv, *flag)
}

// castendNoDamage is skill_castend_damage_id on the caster. The splash
// runs here, so the end of the dispatch runs once.
func (s npcVampireGift) castendNoDamage(z *Zone, src, _ *entity, d *skill.Def, lv int, flag *int) {
	s.castendDamage(z, src, src, d, lv, flag)
}

func (npcVampireGift) splashDamage(z *Zone, src, target *entity, d *skill.Def, lv, flag int) {
	heal := int64(int32(min(z.skillAttack(d.Type, src, src, target, d, lv, flag), math.MaxInt32))) // #nosec G115 -- capped to int32 above.
	if heal <= 0 {
		return
	}
	// clif_skill_nodamage without a source shows AL_HEAL on the caster.
	if h := z.skillsGet(skill.IDHeal); h != nil {
		z.skillEffect(src, src, h, int(heal), heal, true)
	}
	z.healHP(src, heal)
}

// npcSmoking is SkillSmoking (NPC_SMOKING): 3 misc damage to the target,
// the caster itself.
type npcSmoking struct{}

func (npcSmoking) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.skillAttack(d.Type, src, src, target, d, lv, *flag)
}

// castendNoDamage is skill_castend_nodamage_id's case for NPC_SMOKING: a
// self skill that ends in skill_castend_damage_id.
func (s npcSmoking) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	s.castendDamage(z, src, target, d, lv, flag)
}

// npcChangeLocation is SkillChangeLocation (NPC_MOVE_COORDINATE): the
// target moves to the caster's cell, and a caster that is not a boss
// moves to the target's.
type npcChangeLocation struct{}

func (npcChangeLocation) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	px, py := target.x, target.y
	// skill_check_unit_movepos: battlegrounds and guild wars do not
	// exist yet.
	if !z.movePos(target, src.x, src.y, true) {
		*flag |= skillNoConsumeReq
		return
	}
	// clif_skill_damage with DMGVAL_IGNORE shows only the animation,
	// which the effect stands for.
	z.skillEffect(src, target, d, lv, int64(lv), true)
	if src.status == nil || src.status.Class != battle.ClassBoss {
		if !z.movePos(src, px, py, true) {
			*flag |= skillNoConsumeReq
		}
	}
}

// npcRun is SkillNpcRun (NPC_RUN): the monster drops its target and runs
// away from it, 7 cells or the level's cells above level 1, and does not
// think until it arrives.
type npcRun struct{}

func (npcRun) castendNoDamage(z *Zone, src, _ *entity, _ *skill.Def, lv int, _ *int) {
	m := src.monster
	if m == nil || m.target == nil || !z.entities.has(m.target) {
		return
	}
	tbl := m.target
	// md->state.can_escape keeps damage from stopping the walk. The zone
	// has no damage walk delay, so it needs no flag.
	z.mobUnlockTarget(src)
	dist := 7
	if lv > 1 {
		dist = lv
	}
	z.unitEscape(src, tbl, dist)
	if t := path.PathTime(src.path, src.speed); len(src.path) > 0 && t > 0 {
		m.ai.SetState(mob.StateWalk, src.status.Mode)
		m.nextThink = z.now + int64(t)
	}
}

// npcProvocation is SkillProvocation (NPC_PROVOCATION): the monster
// drops its target.
type npcProvocation struct{}

func (npcProvocation) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	if src.monster != nil {
		z.mobUnlockTarget(src)
	}
}

// npcTalk is SkillTalk (NPC_TALK): the effect only. The line the monster
// says comes from its skills entry when mobskill_use picks it.
type npcTalk struct{}

func (npcTalk) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// npcExpulsion is SkillExpulsion (NPC_EXPULSION): unit_warp of the target
// to a random cell of the map.
type npcExpulsion struct{}

func (npcExpulsion) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	z.skillEffect(src, target, d, lv, int64(lv), true)
	x, y, ok := z.randomCell()
	if !ok {
		return
	}
	if p := target.player; p != nil {
		p.warpTo = &scriptWarp{to: z, x: x, y: y}
		z.finishWarp(p)
		return
	}
	z.moveTo(target, x, y)
}

// npcDarkBlessing is SkillDarkBlessing (NPC_DARKBLESSING): SC_COMA by a
// chance of 50% plus 5% per level.
type npcDarkBlessing struct{}

func (npcDarkBlessing) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	ok := z.scStart4(src, target, d.Status, 50+lv*5, lv, lv, 0, 0, int64(d.Duration2.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// npcHellPower is SkillHellPower (NPC_HELLPOWER): SC_HELLPOWER by a
// chance of 20% per level.
type npcHellPower struct{}

func (npcHellPower) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	ok := z.scStart(src, target, d.Status, lv*20, lv, int64(d.Duration2.At(lv)))
	z.skillEffect(src, target, d, lv, int64(lv), ok)
}

// keepingStarted is status_change_start's case for SC_KEEPING and
// SC_BARRIER: the unit cannot attack, act or move, and does not flinch,
// until the status ends.
func (z *Zone) keepingStarted(e *entity, en *sc.Entry) {
	if en.Type != sc.SCKeeping && en.Type != sc.SCBarrier {
		return
	}
	end := z.now + en.Remaining(z.now)
	e.nextAttack, e.canAct, e.canMove, e.endure = end, end, end, end
}

// keepingEnded is status_change_end's case for SC_KEEPING and
// SC_BARRIER: the unit may attack, act and move again at once.
func (z *Zone) keepingEnded(e *entity, en *sc.Entry) {
	if en.Type != sc.SCKeeping && en.Type != sc.SCBarrier {
		return
	}
	e.nextAttack, e.canAct, e.canMove, e.endure = z.now, z.now, z.now, z.now
}

// millenniumShield is SC_MILLENNIUMSHIELD's part of
// battle_status_block_damage: a shield takes the hit. A shield that
// breaks stuns the unit for a second, and the status ends with the last
// shield. src is the attacker, or nil.
func (z *Zone) millenniumShield(src, t *entity, dmg int64) bool {
	e := t.sc.Get(scMillenniumshield)
	if e == nil || e.Val2 <= 0 || dmg <= 0 {
		return false
	}
	e.Val3 -= int(min(dmg, math.MaxInt32))
	if e.Val3 > 0 {
		return true
	}
	e.Val2--
	if e.Val2 > 0 {
		// clif_millenniumshield has no event on the client yet.
		e.Val3 = 1000
	} else {
		z.scEnd(t, scMillenniumshield)
	}
	if en := z.statusEngine(); en != nil {
		var su sc.Unit
		if src != nil {
			su = z.unit(src)
		}
		en.Start(su, z.unit(t), sc.Start{Type: sc.SCStun, Rate: 10000, Duration: 1000, Flag: sc.NoTickDef,
			Reason: sc.ReasonSkill})
	}
	return true
}

// damageHeal is SC_DAMAGE_HEAL's part of battle_status_block_damage: a
// hit of the status's attack type heals the unit instead. The heal
// numbers that rAthena shows (clif_specialeffect_value) have no event on
// the client yet. rAthena leaves the hit's dmg_lv as it was, where the
// zone's block marks it blocked.
func (z *Zone) damageHeal(t *entity, flag int, dmg int64) bool {
	e := t.sc.Get(scDamageHeal)
	if e == nil || dmg <= 0 || flag&e.Val2 == 0 {
		return false
	}
	z.healHP(t, dmg)
	return true
}
