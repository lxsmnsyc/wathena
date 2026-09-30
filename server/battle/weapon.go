package battle

import "math"

// damageType is rAthena's e_damage_type, the hit animation.
type damageType int

const (
	dmgNormal           damageType = 0
	dmgMultiHit         damageType = 8
	dmgCritical         damageType = 10
	dmgLuckyDodge       damageType = 11
	dmgMultiHitCritical damageType = 13
)

// damageLevel is rAthena's damage_lv, the state of one attack.
type damageLevel int

const (
	atkNone damageLevel = iota
	atkLucky
	atkFlee
	atkMiss
	atkBlock
	atkDef
)

// Attack flags, as rAthena's e_battle_flag.
const (
	bfWeapon = 0x0001
	bfMagic  = 0x0002
	bfMisc   = 0x0004
	bfShort  = 0x0010
	bfLong   = 0x0040
	bfSkill  = 0x0100
	bfNormal = 0x0200
)

// Base damage flags, as rAthena's e_base_damage_flag.
const (
	bdmgCrit  = 0x0001
	bdmgMagic = 0x0004
)

// damage is rAthena's Damage struct, with the fields a normal monster
// attack uses. The renewal parts (statusAtk, weaponAtk and so on) are
// only used by players, so they are left out.
type damage struct {
	damage, damage2 int64
	typ             damageType
	div             int
	dmgLv           damageLevel
	flag            int
}

// calcWeaponAttack is battle_calc_weapon_attack for a normal attack by a
// monster. The random draws happen in rAthena's order: lucky dodge,
// critical, hit, then the damage roll.
func calcWeaponAttack(src, target *Status, r Rand) damage {
	wd := initializeWeaponData(src)

	// battle_get_weapon_element: a normal attack uses the weapon's
	// element.
	rightElement := src.RHW.Ele
	leftElement := src.LHW.Ele

	// Check for Lucky Dodge.
	if target.Flee2 != 0 && r.IntN(1000) < target.Flee2 {
		wd.typ = dmgLuckyDodge
		wd.dmgLv = atkLucky
		return wd
	}

	// battle_calc_multi_attack only applies to players (double attack,
	// chain action).

	// Critical hits always hit, so the critical check comes first.
	if isAttackCritical(src, target, r) {
		wd.typ = dmgCritical
	}
	critical := wd.typ == dmgCritical || wd.typ == dmgMultiHitCritical

	infdef := false
	if !isAttackHitting(src, target, critical, r) {
		wd.dmgLv = atkFlee
	} else if infdef = isInfiniteDefense(target, wd.flag); !infdef {
		calcSkillBaseDamage(&wd, src, target, critical, r)

		// Players add card bonuses and battle_attack_sc_bonus here, and
		// sum their ATK parts. Monsters have their damage already.

		// Skill ratio: battle_calc_attack_skill_ratio is 100 with no
		// skill and no status changes.
		wd.rate(src, 100)

		// battle_calc_skill_constant_addition is 0 with no skill.

		// Res reduces physical damage by a percent before DEF.
		if wd.damage+wd.damage2 != 0 && target.Res > 0 {
			// Players can ignore part of Res. Monsters cannot.
			res := float64(target.Res)
			part := float64(float32(res / (res + 400.0)))
			wd.damage -= int64(part * 80.0 / 100.0 * float64(wd.damage))
			wd.damage2 -= int64(part * 80.0 / 100.0 * float64(wd.damage2))
		}

		if wd.damage+wd.damage2 != 0 {
			// attack_ignores_def is false for a monster's normal attack.
			calcDefenseReduction(&wd, src, target)
			calcAttackPostDefense(&wd, src)
		}
	}

	calcElementDamage(&wd, src, target, rightElement, leftElement)

	// Renewal critical damage. Monsters have no critical damage bonus.
	if critical {
		wd.damage = int64(math.Floor(float64(float32(wd.damage) * 1.4)))
	}

	// Card fixes only apply when a player attacks or is attacked.

	// A target that takes 1 damage from every hit.
	if infdef {
		calcAttackPlant(&wd, src)
		return wd
	}

	applyDivFix(&wd)
	calcAttackLeftRightHands(&wd, src)
	calcAttackGvgBg(&wd, target)

	// battle_calc_weapon_final_atk_modifiers only changes damage through
	// status changes and skill damage adjustments, which do not apply.
	// battle_absorb_damage only applies to players, and battle_do_reflect
	// does not change the attacker's damage.
	return wd
}

// initializeWeaponData is initialize_weapon_data for a normal attack.
func initializeWeaponData(src *Status) damage {
	wd := damage{typ: dmgNormal, div: 1, dmgLv: atkDef, flag: bfWeapon | bfNormal}
	if isSkillUsingArrow(src) {
		wd.flag |= bfLong
	} else {
		wd.flag |= bfShort
	}
	return wd
}

// isSkillUsingArrow is is_skill_using_arrow for a monster's normal
// attack: attacks with a range over 3 cells count as ranged.
func isSkillUsingArrow(src *Status) bool {
	return src.RHW.Range > 3
}

// isAttackRightHanded is is_attack_right_handed. It is only false for
// players with a weapon in the left hand only.
func isAttackRightHanded(*Status) bool { return true }

// isAttackLeftHanded is is_attack_left_handed for a normal attack by a
// unit that is not a player.
func isAttackLeftHanded(src *Status) bool { return src.LHW.Atk != 0 }

// isAttackCritical is is_attack_critical on its first call, for a normal
// attack by a monster on a monster. Later calls in rAthena only read the
// damage type.
func isAttackCritical(src, target *Status, r Rand) bool {
	if src.Cri == 0 {
		return false
	}
	cri := src.Cri
	// The official factor is 2. rAthena uses 3 when a monster attacks a
	// player, which Status cannot tell apart, so this uses 2.
	cri -= target.Luk * 2
	// Status changes (Sleep, Auto Counter, Camouflage) do not apply.
	return r.IntN(1000) < cri
}

// isAttackHitting is is_attack_hitting on its first call, for a normal
// attack by a monster.
func isAttackHitting(src, target *Status, critical bool, r Rand) bool {
	if critical {
		return true
	}
	flee := target.Flee
	// The flee penalty for many attackers (agi_penalty_target) only
	// applies to players.

	// Renewal has no base hit rate.
	hitrate := src.Hit - flee
	hitrate = min(max(hitrate, minHitrate), maxHitrate)
	return r.IntN(100) < hitrate
}

// isInfiniteDefense is is_infinite_defense: targets that ignore an
// attack's type take 1 damage from each hit.
func isInfiniteDefense(target *Status, flag int) bool {
	if target.hasMode(ModeIgnoreMelee) && flag&(bfWeapon|bfShort) == bfWeapon|bfShort {
		return true
	}
	if target.hasMode(ModeIgnoreMagic) && flag&bfMagic != 0 {
		return true
	}
	if target.hasMode(ModeIgnoreRanged) && flag&(bfWeapon|bfLong) == bfWeapon|bfLong {
		return true
	}
	return target.hasMode(ModeIgnoreMisc) && flag&bfMisc != 0
}

// calcSkillBaseDamage is the default case of
// battle_calc_skill_base_damage. In renewal, units that are not players
// use battle_calc_base_damage.
func calcSkillBaseDamage(wd *damage, src, target *Status, critical bool, r Rand) {
	bflag := 0
	if critical {
		bflag |= bdmgCrit
	}
	wd.damage = calcBaseDamage(src, &src.RHW, target.Size, bflag, r)
	if isAttackLeftHanded(src) {
		wd.damage2 = calcBaseDamage(src, &src.LHW, target.Size, bflag, r)
	}
}

// calcBaseDamage is battle_calc_base_damage for a unit that is not a
// player: a roll between the weapon's atk and atk2, plus base ATK. The
// roll is rnd() % (atk2 - atk), so atk2 itself is only reached by a
// critical hit.
func calcBaseDamage(st *Status, wa *Weapon, _ int, flag int, r Rand) int64 {
	var atkmin, atkmax int
	if flag&bdmgMagic != 0 {
		atkmin, atkmax = st.MAtkMin, st.MAtkMax
	} else {
		atkmin, atkmax = wa.Atk, wa.Atk2
	}
	atkmin = min(atkmin, atkmax)

	var dmg int64
	if flag&bdmgCrit == 0 {
		if atkmax > atkmin {
			dmg = int64(r.IntN(atkmax - atkmin))
		}
		dmg += int64(atkmin)
	} else {
		dmg = int64(atkmax)
	}

	// Elementals get bonuses from insignia status changes here.

	if flag&bdmgMagic == 0 {
		dmg += int64(st.BAtk)
	}
	if flag&bdmgCrit != 0 {
		dmg = dmg * 14 / 10
	}
	return dmg
}

// calcDefenseReduction is battle_calc_defense_reduction in renewal, for
// a target that is not a player: damage * (4000 + DEF) / (4000 + DEF * 10)
// minus soft DEF.
func calcDefenseReduction(wd *damage, src, target *Status) {
	// status_get_def lowers DEF while the target casts. Sample monsters
	// do not cast.
	def1 := int64(target.Def)
	def2 := int64(target.Def2)

	// The defense penalty for many attackers (vit_penalty_target) only
	// applies to players.

	// A monster's soft DEF is its def2.
	vitDef := def2

	if weaponDefenseType != 0 {
		vitDef += def1 * weaponDefenseType
		def1 = 0
	}

	// -400 would divide by zero.
	if def1 == -400 {
		def1 = -399
	}
	// Monsters' attacks never pierce defense (is_attack_piercing).
	wd.damage = wd.damage*(4000+def1)/(4000+10*def1) - vitDef
	if isAttackLeftHanded(src) {
		wd.damage2 = wd.damage2*(4000+def1)/(4000+10*def1) - vitDef
	}
}

// calcAttackPostDefense is battle_calc_attack_post_defense in renewal.
// With no status changes, it only raises damage to at least 1.
func calcAttackPostDefense(wd *damage, src *Status) {
	minDamage(wd, src, 1)
}

// minDamage is battle_min_damage: right-hand damage is raised to at
// least minimum, and left-hand damage to at least 0.
func minDamage(wd *damage, src *Status, minimum int64) {
	if isAttackRightHanded(src) {
		wd.damage = max(wd.damage, minimum)
	}
	if isAttackLeftHanded(src) {
		wd.damage2 = max(wd.damage2, 0)
	}
}

// calcElementDamage is battle_calc_element_damage for a normal attack.
func calcElementDamage(wd *damage, src, target *Status, rightElement, leftElement int) {
	if wd.damage > 0 || wd.damage2 > 0 {
		// Normal attacks by monsters are non-elemental and deal 100%
		// against every defense element (attack_attr_none).
		if monsterAttackAttrNone {
			return
		}
		wd.damage = AttrFix(wd.damage, rightElement, target.DefEle, target.EleLv, 1)
		if isAttackLeftHanded(src) {
			wd.damage2 = AttrFix(wd.damage2, leftElement, target.DefEle, target.EleLv, 1)
		}
	}
	if !attrRecover {
		minDamage(wd, src, 0)
	}
}

// calcAttackPlant is battle_calc_attack_plant: the attack hit a target
// that takes 1 damage from each hit.
//
// rAthena also handles the Emperium here. That needs the monster ID and
// guild castle state, so it is left out.
func calcAttackPlant(wd *damage, src *Status) {
	// Only a hit reaches here, so the attack always counts as hitting.
	wd.damage = 1
	if isAttackLeftHanded(src) {
		wd.damage2 = 1
	}
	// Status changes that block damage do not apply.

	applyDivFix(wd)

	// With left-hand damage, the total never exceeds 2.
	if wd.damage > 1 && wd.damage2 > 0 {
		wd.damage = 1
		wd.damage2 = 1
	}
}

// applyDivFix is battle_apply_div_fix: DAMAGE_DIV_FIX and the minimum
// damage for multi-hit attacks.
func applyDivFix(wd *damage) {
	if wd.damage != 0 {
		if wd.div < 0 {
			wd.div = -wd.div
			wd.damage /= int64(wd.div)
		}
		wd.damage *= int64(wd.div)
		if wd.damage < int64(wd.div) && skillMinDamage&wd.flag != 0 {
			wd.damage = int64(wd.div)
		}
	} else if wd.div < 0 {
		wd.div = -wd.div
	}
}

// calcAttackLeftRightHands is battle_calc_attack_left_right_hands. The
// dual-wield and katar rates only apply to players.
func calcAttackLeftRightHands(wd *damage, src *Status) {
	if !isAttackRightHanded(src) && !isAttackLeftHanded(src) && wd.damage != 0 {
		wd.damage = 0
	}
	if !isAttackLeftHanded(src) && wd.damage2 != 0 {
		wd.damage2 = 0
	}
}

// calcAttackGvgBg is battle_calc_attack_gvg_bg outside GvG and
// battlegrounds. Damage returned by reflection does not change the
// attack's damage.
func calcAttackGvgBg(wd *damage, target *Status) {
	if wd.damage+wd.damage2 == 0 {
		return
	}
	switch {
	case wd.damage2 == 0:
		wd.damage = calcDamage(wd, target, wd.damage)
	case wd.damage == 0:
		wd.damage2 = calcDamage(wd, target, wd.damage2)
	default:
		wd.damage = calcDamage(wd, target, wd.damage)
		wd.damage2 = calcDamage(wd, target, wd.damage2)
		if wd.damage > 1 && wd.damage2 < 1 {
			wd.damage2 = 1
		}
	}
}

// calcDamage is battle_calc_damage for a normal attack with no status
// changes on either side, outside PK maps.
func calcDamage(wd *damage, target *Status, dmg int64) int64 {
	if dmg == 0 {
		return 0
	}
	// ksprotection is off by default.

	if skillMinDamage != 0 && dmg > 0 && dmg < int64(wd.div) && wd.flag&bfWeapon != 0 && skillMinDamage&1 != 0 {
		dmg = int64(wd.div)
	}

	// Some monsters, such as MVPs with a green aura, take less damage.
	if taken := target.DamageTaken; taken != 0 && taken != 100 {
		dmg = max(dmg*int64(taken)/100, 1)
	}
	return dmg
}

// rate is rAthena's ATK_RATE macro: both hands' damage times a percent.
func (wd *damage) rate(src *Status, pct int64) {
	wd.damage = wd.damage * pct / 100
	if isAttackLeftHanded(src) {
		wd.damage2 = wd.damage2 * pct / 100
	}
}
