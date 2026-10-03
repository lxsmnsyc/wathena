package zone

import (
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// The monster weapon attacks: the NPC_ skills of rAthena's
// src/map/skills/npc that hit, drain, break or chain hits. The damage
// formulas are in server/skill/npc_weapon.go.

// Skill IDs of the monster weapon attacks, from rAthena's skill_db.yml.
const (
	idNpcPiercingAtt        = 158 // NPC_PIERCINGATT
	idNpcMentalBreaker      = 159 // NPC_MENTALBREAKER
	idNpcRangeAttack        = 160 // NPC_RANGEATTACK
	idNpcCriticalSlash      = 170 // NPC_CRITICALSLASH
	idNpcComboAttack        = 171 // NPC_COMBOATTACK
	idNpcGuidedAttack       = 172 // NPC_GUIDEDATTACK
	idNpcSplashAttack       = 174 // NPC_SPLASHATTACK
	idNpcRandomAttack       = 183 // NPC_RANDOMATTACK
	idNpcWaterAttack        = 184 // NPC_WATERATTACK
	idNpcGroundAttack       = 185 // NPC_GROUNDATTACK
	idNpcFireAttack         = 186 // NPC_FIREATTACK
	idNpcWindAttack         = 187 // NPC_WINDATTACK
	idNpcPoisonAttack       = 188 // NPC_POISONATTACK
	idNpcHolyAttack         = 189 // NPC_HOLYATTACK
	idNpcDarknessAttack     = 190 // NPC_DARKNESSATTACK
	idNpcTelekinesisAttack  = 191 // NPC_TELEKINESISATTACK
	idNpcBloodDrain         = 199 // NPC_BLOODDRAIN
	idNpcEnergyDrain        = 200 // NPC_ENERGYDRAIN
	idNpcRevenge            = 333 // NPC_REVENGE
	idNpcWeaponBraker       = 343 // NPC_WEAPONBRAKER
	idNpcArmorBrake         = 344 // NPC_ARMORBRAKE
	idNpcHelmBrake          = 345 // NPC_HELMBRAKE
	idNpcShieldBrake        = 346 // NPC_SHIELDBRAKE
	idNpcUndeadAttack       = 347 // NPC_UNDEADATTACK
	idNpcPulseStrike        = 661 // NPC_PULSESTRIKE
	idNpcCriticalWound      = 673 // NPC_CRITICALWOUND
	idNpcPulseStrike2       = 712 // NPC_PULSESTRIKE2
	idNpcDancingBlade       = 713 // NPC_DANCINGBLADE
	idNpcDancingBladeAttack = 714 // NPC_DANCINGBLADE_ATK
	idNpcDarkPiercing       = 715 // NPC_DARKPIERCING
	idNpcPhantomThrust      = 741 // NPC_PHANTOMTHRUST
)

// scWeaponBreaker is SC_WEAPONBREAKER, NPC_WEAPONBRAKER's status: its
// val2 is a chance per 10,000 that the unit's weapon hits break the
// target's weapon (skill_additional_effect).
const scWeaponBreaker = 909 // SC_WEAPONBREAKER

func init() {
	registerSkillImpls(map[int]any{
		// WeaponSkillImpl and the classes that only add formulas to it.
		idNpcGroundAttack:      attackSkill{},
		idNpcWaterAttack:       attackSkill{},
		idNpcFireAttack:        attackSkill{},
		idNpcWindAttack:        attackSkill{},
		idNpcDarknessAttack:    attackSkill{},
		idNpcPoisonAttack:      attackSkill{},
		idNpcHolyAttack:        attackSkill{},
		idNpcTelekinesisAttack: attackSkill{},
		idNpcUndeadAttack:      attackSkill{},
		idNpcComboAttack:       attackSkill{},
		idNpcCriticalSlash:     attackSkill{},
		idNpcGuidedAttack:      attackSkill{},
		idNpcPiercingAtt:       attackSkill{},
		idNpcRangeAttack:       attackSkill{},
		idNpcRandomAttack:      attackSkill{},

		idNpcSplashAttack:       npcSplashAttack{},
		idNpcBloodDrain:         npcDrain{typ: battle.TypeWeapon},
		idNpcEnergyDrain:        npcDrain{typ: battle.TypeMagic},
		idNpcMentalBreaker:      npcMentalBreaker{},
		idNpcPulseStrike:        npcPulseStrike{},
		idNpcCriticalWound:      npcCriticalWound{},
		idNpcArmorBrake:         npcBreakEquip{where: item.EqpArmor},
		idNpcHelmBrake:          npcBreakEquip{where: item.EqpHelm},
		idNpcShieldBrake:        npcBreakEquip{where: item.EqpShield},
		idNpcWeaponBraker:       statusSkill{},
		idNpcPhantomThrust:      npcPhantomThrust{},
		idNpcDarkPiercing:       npcDarkPiercing{},
		idNpcDancingBlade:       npcDancingBlade{},
		idNpcDancingBladeAttack: npcDancingBladeAttack{},
		idNpcRevenge:            npcRevenge{},
		idNpcPulseStrike2:       npcPulseStrike2{},
	})
}

// npcSplashAttack is SkillSplashAttack: the splash of
// SkillImplRecursiveDamageSplash, with a fake packet for the first
// target (SD_PREAMBLE).
type npcSplashAttack struct{}

func (npcSplashAttack) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	*flag |= sdPreamble
	z.recursiveSplash(src, target, d, lv, *flag)
}

// npcDrain is SkillSuckingBlood (a weapon hit) and SkillEnergyDrain (a
// magic hit): the caster heals the damage it dealt, and shows it as a
// Heal.
type npcDrain struct{ typ int }

func (n npcDrain) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	// rAthena casts the damage to int32.
	heal := int64(int32(z.skillAttack(n.typ, src, src, target, d, lv, *flag))) // #nosec G115 -- as rAthena's cast.
	if heal > 0 {
		z.skillEffect(src, src, z.skillsGet(skill.IDHeal), int(heal), heal, true)
		z.healHP(src, heal)
	}
}

// npcMentalBreaker is SkillSpiritDestruction: a weapon hit that takes
// 12%, 16%, 25%, 50% or 100% of the target's maximum SP, by level.
type npcMentalBreaker struct{ attackSkill }

func (npcMentalBreaker) additionalEffect(z *Zone, src, target *entity, _ *skill.Def, lv int) {
	var rate int
	switch lv {
	case 1:
		rate = 12
	case 2:
		rate = 16
	case 3:
		rate = 25
	case 4:
		rate = 50
	case 5:
		rate = 100
	default:
		rate = lv
	}
	z.percentDamageSP(src, target, rate)
}

// percentDamageSP is status_percent_damage with no HP and a negative SP
// rate, which may not kill: rate percent of the target's maximum SP, at
// least 1, all of it from 100%. The damage is passive, so it ends no
// status. Monsters have no SP to lose.
func (z *Zone) percentDamageSP(_, target *entity, rate int) {
	p := target.player
	if p == nil || target.status == nil || rate == 0 {
		return
	}
	maxSP := int64(target.status.MaxSP)
	var sp int64
	if rate > 99 {
		sp = maxSP
	} else {
		// apply_rate.
		sp = maxSP * int64(rate) / 100
	}
	if sp == 0 {
		sp = 1
	}
	scEnv{z}.Damage(z.unit(target), 0, sp, true, false)
}

// npcPulseStrike is SkillPulseStrike: a self skill that runs the splash
// of skill_castend_damage_id around the caster.
type npcPulseStrike struct{ splashSkill }

func (npcPulseStrike) castendNoDamage(z *Zone, src, _ *entity, d *skill.Def, lv int, flag *int) {
	z.castendDamage(src, src, d, lv, *flag)
	// The inner call already ended the skill (afterSkill), so the outer
	// one does not end it again.
	*flag |= 1
}

// npcCriticalWound is SkillCriticalWounds: a weapon hit that starts
// SC_CRITICALWOUND for Duration2.
type npcCriticalWound struct{ attackSkill }

func (npcCriticalWound) additionalEffect(z *Zone, src, target *entity, d *skill.Def, lv int) {
	z.scStart(src, target, sc.SCCriticalwound, 100, lv, int64(d.Duration2.At(lv)))
}

// npcBreakEquip is SkillBreakArmor, SkillBreakHelm and SkillBreakShield:
// a weapon hit that breaks a part with 1.5% per level (skill_break_equip,
// per 10,000).
type npcBreakEquip struct {
	attackSkill
	where uint32
}

func (n npcBreakEquip) additionalEffect(z *Zone, src, target *entity, _ *skill.Def, lv int) {
	z.breakEquip(src, target, n.where, 150*lv, bctEnemy)
}

// npcPhantomThrust is SkillNpcPhantomThrust: the caster turns to the
// target and pulls it next to itself, then hits it when it is an enemy.
type npcPhantomThrust struct{}

func (npcPhantomThrust) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	// unit_setdir.
	src.dir = protocolDir(mapCalcDir(src, target.x, target.y))
	z.skillEffect(src, target, d, lv, int64(lv), true)
	// skill_blown toward the caster: the opposite of the caster's facing,
	// which points at the target, by the distance less one.
	z.skillBlown(src, target, chebyshev(src, target)-1, rDir(src), false)
	if checkTarget(src, target, bctEnemy) {
		z.skillAttack(battle.TypeWeapon, src, src, target, d, lv, *flag)
	}
}

// npcDarkPiercing is SkillDarkPiercing with skill_eightpath_algorithm
// on: every enemy in a line toward the target (map_foreachindir), the
// splash wide and ActiveInstance long. When the line deals no damage,
// the target is hit once.
type npcDarkPiercing struct{}

func (npcDarkPiercing) castendDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	z.area = areaTemp{main: target}
	var total int64
	for _, t := range z.inDir(src.x, src.y, target.x, target.y, d.SplashFor(lv), d.ActiveInstance.At(lv), 0) {
		total += z.skillAttackArea(d.Type, src, src, t, d, lv, *flag)
	}
	if total == 0 {
		z.skillAttack(d.Type, src, src, target, d, lv, *flag)
	}
}

// npcDancingBlade is SkillDancingBlade: the first blade hits after the
// caster's attack motion, as NPC_DANCINGBLADE_ATK.
type npcDancingBlade struct{}

func (npcDancingBlade) castendNoDamage(z *Zone, src, target *entity, _ *skill.Def, lv int, _ *int) {
	atk := z.skillsGet(idNpcDancingBladeAttack)
	if atk == nil {
		return
	}
	amotion := 0
	if src.status != nil {
		amotion = src.status.AMotion
	}
	z.addTimerSkill(src, z.now+int64(amotion), target, 0, 0, atk, lv, 0, 0)
}

// npcDancingBladeAttack is skill_timerskill's NPC_DANCINGBLADE_ATK: a
// weapon hit, then up to four more blades 650 ms apart, each on a random
// enemy within 5 cells of the last target, or the same target when there
// is none. typ counts the blades after the first.
type npcDancingBladeAttack struct{}

func (npcDancingBladeAttack) timerSkill(z *Zone, t *timerSkill) {
	src, target := t.src, t.target
	if target == nil {
		return
	}
	z.skillAttack(battle.TypeWeapon, src, src, target, t.def, t.lv, t.flag)
	if t.typ < 4 {
		next := z.enemyArea(src, target.x, target.y, 5, src)
		if next == nil {
			next = target
		}
		z.addTimerSkill(src, z.now+650, next, t.x, 0, t.def, t.lv, t.typ+1, 0)
	}
}

// enemyArea is battle_getenemyarea: a random enemy of src in the square
// of the range around a cell, of the first 32 found. It leaves out src
// and ignore, and the dead.
func (z *Zone) enemyArea(src *entity, x, y, rng int, ignore *entity) *entity {
	var list []*entity
	for _, o := range z.findEntities(&query{area: areaRadius, x: x, y: y, radius: rng,
		kinds: queryPlayers | queryMonsters}) {
		if o == src || o == ignore {
			continue
		}
		if len(list) >= maxEnemySearch {
			break
		}
		if checkTarget(src, o, bctEnemy) {
			list = append(list, o)
		}
	}
	if len(list) == 0 {
		return nil
	}
	return list[z.rand.IntN(len(list))]
}

// npcRevenge is SkillRevenge: a slave turns on a random unit that
// attacks or casts at its master (battle_gettargeted), as if provoked.
type npcRevenge struct{}

func (npcRevenge) castendNoDamage(z *Zone, src, _ *entity, _ *skill.Def, _ int, _ *int) {
	m := src.monster
	if m == nil || m.master == nil {
		return
	}
	master := m.master
	if !z.entities.has(master) {
		return
	}
	t := z.getTargeted(master)
	if t == nil {
		return
	}
	m.provoke = t
	z.mobTarget(src, t)
}

// getTargeted is battle_gettargeted: a random unit within AREA_SIZE
// (ViewRange) of e, of the first 32 found, whose attack or skill targets
// e.
func (z *Zone) getTargeted(e *entity) *entity {
	var list []*entity
	for _, o := range z.findEntities(&query{area: areaRadius, x: e.x, y: e.y, radius: ViewRange,
		kinds: queryPlayers | queryMonsters, dead: true}) {
		if o == e {
			continue
		}
		if len(list) >= maxEnemySearch {
			break
		}
		if o.target == e || o.cast != nil && o.cast.target == e {
			list = append(list, o)
		}
	}
	if len(list) == 0 {
		return nil
	}
	return list[z.rand.IntN(len(list))]
}

// npcPulseStrike2 is SkillPulseStrike2: three splashes around the
// caster, Duration1 apart, each a skill_castend_damage_id from
// skill_timerskill.
type npcPulseStrike2 struct{ splashSkill }

func (npcPulseStrike2) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, flag *int) {
	for i := range 3 {
		z.addTimerSkill(src, z.now+int64(d.Duration1.At(lv))*int64(i), target, 0, 0, d, lv, d.Type, *flag)
	}
}

func (npcPulseStrike2) timerSkill(z *Zone, t *timerSkill) {
	if t.target != nil {
		z.castendDamage(t.src, t.target, t.def, t.lv, t.flag)
	}
}

// itemHealPenalty is pc_itemheal's penalty: SC_CRITICALWOUND's val2 and
// Death Hurt with val3 1 (20) take a part of an item's heal, and
// SC_NORECOVER_STATE takes all of it. Vitality Activation, Water
// Insignia and Bite Scar are not ported.
func itemHealPenalty(e *entity, hp, sp int) (int, int) {
	if e.sc.Empty() {
		return hp, sp
	}
	penalty := 0
	if x := e.sc.Get(sc.SCCriticalwound); x != nil {
		penalty += x.Val2
	}
	if x := e.sc.Get(sc.SCDeathhurt); x != nil && x.Val3 == 1 {
		penalty += 20
	}
	if e.sc.Has(sc.SCNorecoverState) {
		penalty = 100
	}
	if penalty > 0 {
		hp -= hp * penalty / 100
		sp -= sp * penalty / 100
	}
	return hp, sp
}
