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

// FlagLong is BF_LONG in Result.Flag: the attack was ranged.
const FlagLong = bfLong

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

// damage is rAthena's Damage struct, with the fields a normal attack
// uses. The renewal ATK parts (statusAtk, weaponAtk and so on) are only
// used by players. Monsters get their damage directly.
type damage struct {
	damage, damage2 int64
	typ             damageType
	div             int
	dmgLv           damageLevel
	flag            int

	statusAtk, statusAtk2   int64
	weaponAtk, weaponAtk2   int64
	equipAtk, equipAtk2     int64
	masteryAtk, masteryAtk2 int64
	percentAtk, percentAtk2 int64
}

// calcWeaponAttack is battle_calc_weapon_attack for a normal attack. The
// random draws happen in rAthena's order: lucky dodge, double attack,
// critical, hit, then the damage rolls.
func calcWeaponAttack(src, target *Status, r Rand) damage {
	sd := src.Player
	wd := initializeWeaponData(src)

	rightElement := weaponElement(src, handRight)
	leftElement := weaponElement(src, handLeft)

	// Check for Lucky Dodge.
	if target.Flee2 != 0 && r.IntN(1000) < target.Flee2 {
		wd.typ = dmgLuckyDodge
		wd.dmgLv = atkLucky
		return wd
	}

	// Multiple hits come first, so a double attack can override a
	// critical hit.
	calcMultiAttack(&wd, src, r)

	// Critical hits always hit, so the critical check comes next.
	if isAttackCritical(&wd, src, target, r) {
		if wd.typ == dmgMultiHit {
			wd.typ = dmgMultiHitCritical
		} else {
			wd.typ = dmgCritical
		}
	}
	critical := wd.typ == dmgCritical || wd.typ == dmgMultiHitCritical

	infdef := false
	if !isAttackHitting(&wd, src, target, critical, r) {
		wd.dmgLv = atkFlee
	} else if infdef = isInfiniteDefense(target, wd.flag); !infdef {
		calcSkillBaseDamage(&wd, src, target, critical, r)

		if sd != nil {
			// Renewal applies the attacker's cards to weapon and
			// equipment ATK only.
			wd.weaponAtk += cardfixAttacker(src, target, wd.weaponAtk, 2, wd.flag)
			wd.equipAtk += cardfixAttacker(src, target, wd.equipAtk, 2, wd.flag)
			// left_cardfix_to_right moves the left hand's bonuses to the
			// right hand, so the left hand only gets bAddDamageClass.
			if isAttackLeftHanded(src) {
				wd.weaponAtk2 += cardfixAttacker(src, target, wd.weaponAtk2, 3, wd.flag)
				wd.equipAtk2 += cardfixAttacker(src, target, wd.equipAtk2, 3, wd.flag)
			}
		}
		// A player target's cards apply to the ATK parts when a player
		// attacks a player. Players attacking players is not ported.

		// battle_attack_sc_bonus only changes damage through status
		// changes, spirit charms, Star Gladiator's anger against a marked
		// monster and the Summoner's Power of Life. None of them are
		// ported.

		// Players sum their ATK parts. Monsters already have their
		// damage from battle_calc_base_damage.
		if sd != nil {
			sumPlayerDamage(&wd, src)
		}

		// Skill ratio: battle_calc_attack_skill_ratio is 100 with no
		// skill and no status changes.
		wd.rate(src, 100)

		// battle_calc_skill_constant_addition is 0 with no skill.

		// Advanced Katar Mastery applies after the skill ratio.
		if lv := src.skill(skillAdvKatarMastery); sd != nil && sd.Weapon == WeaponKatar && lv > 0 {
			atkAddRate(src, &wd.damage, &wd.damage2, int64(10+2*lv))
		}

		// Res reduces physical damage by a percent before DEF.
		if wd.damage+wd.damage2 != 0 && target.Res > 0 {
			// Players can ignore part of Res with bonuses that are not
			// ported.
			res := float64(target.Res)
			part := float64(float32(res / (res + 400.0)))
			wd.damage -= int64(part * 80.0 / 100.0 * float64(wd.damage))
			wd.damage2 -= int64(part * 80.0 / 100.0 * float64(wd.damage2))
		}

		if wd.damage+wd.damage2 != 0 {
			if !attackIgnoresDef(src, target, handLeft) || !attackIgnoresDef(src, target, handRight) {
				calcDefenseReduction(&wd, src, target)
			}
			calcAttackPostDefense(&wd, src)
		}
	}

	calcElementDamage(&wd, src, target, rightElement, leftElement)

	// Renewal critical damage, with the attacker's C.Rate.
	if critical {
		if sd != nil {
			rate := float32(1.4) + float32(0.01)*float32(src.CRate)
			wd.damage = int64(math.Floor(float64(float32(wd.damage) * rate)))
			if isAttackLeftHanded(src) {
				wd.damage2 = int64(math.Floor(float64(float32(wd.damage2) * rate)))
			}
		} else {
			wd.damage = int64(math.Floor(float64(float32(wd.damage) * 1.4)))
		}
		if tsd := target.Player; tsd != nil && tsd.CritDefRate != 0 {
			atkAddRate(src, &wd.damage, &wd.damage2, int64(-tsd.CritDefRate))
		}
	}

	// Renewal applies the target's cards here only when a monster
	// attacks a player.
	if target.Player != nil && sd == nil {
		wd.damage += cardfixTarget(src, target, wd.damage, rightElement, wd.flag)
		if isAttackLeftHanded(src) {
			wd.damage2 += cardfixTarget(src, target, wd.damage2, rightElement, wd.flag)
		}
	}

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
	// battle_do_reflect does not change the attacker's damage.
	absorbDamage(&wd, target)
	return wd
}

// absorbDamage is battle_absorb_damage: a player target's
// bAbsorbDmgMaxHP2 caps a hit at a percent of its max HP.
func absorbDamage(wd *damage, target *Status) {
	tsd := target.Player
	if tsd == nil || tsd.DamageTakenCap == 0 || wd.damage+wd.damage2 == 0 {
		return
	}
	orig := wd.damage + wd.damage2
	hp := int64(int32(int64(tsd.DamageTakenCap) * int64(target.MaxHP) / 100)) // #nosec G115 -- rAthena keeps the cap in an int32.
	if orig <= hp {
		return
	}
	switch {
	case wd.damage2 == 0:
		wd.damage = hp
	case wd.damage == 0:
		wd.damage2 = hp
	default:
		wd.damage = hp
		wd.damage2 = max(hp*wd.damage2/orig/100, 1)
		wd.damage -= wd.damage2
	}
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

// isSkillUsingArrow is is_skill_using_arrow for a normal attack. A player
// uses ammunition when ArrowAttack is set. For other units, attacks with a
// range over 3 cells count as ranged.
func isSkillUsingArrow(src *Status) bool {
	if src.Player != nil {
		return src.Player.ArrowAttack
	}
	return src.RHW.Range > 3
}

// isAttackRightHanded is is_attack_right_handed. It is only false for
// players with a weapon in the left hand only.
func isAttackRightHanded(src *Status) bool {
	if sd := src.Player; sd != nil && sd.RightWeapon == WeaponFist && sd.LeftWeapon != WeaponFist {
		return false
	}
	return true
}

// isAttackLeftHanded is is_attack_left_handed for a normal attack: a
// player with a weapon in the left hand only or with a katar, or any unit
// with left-hand ATK.
func isAttackLeftHanded(src *Status) bool {
	if sd := src.Player; sd != nil {
		if sd.RightWeapon == WeaponFist && sd.LeftWeapon != WeaponFist {
			return true
		}
		if sd.Weapon == WeaponKatar {
			return true
		}
	}
	return src.LHW.Atk != 0
}

// isAttackCritical is is_attack_critical on its first call, for a normal
// attack. Later calls in rAthena only read the damage type.
func isAttackCritical(wd *damage, src, target *Status, r Rand) bool {
	if src.Cri == 0 {
		return false
	}
	// Double hits can only be critical when the skill has the Critical
	// damage flag.
	if wd.typ == dmgMultiHit {
		if src.skill(skillChainAction) > 0 && !chainActionCanCrit {
			return false
		}
		if src.skill(skillDoubleAttack) > 0 && !doubleAttackCanCrit {
			return false
		}
	}
	cri := src.Cri
	if sd := src.Player; sd != nil {
		cri += sd.CritAddRace[target.Race] + sd.CritAddRace[RaceAll]
		if isSkillUsingArrow(src) {
			cri += sd.Arrow.Cri + sd.CriticalRangeAtk
		}
	}

	// The official factor is 2. rAthena uses 3 when a unit that is not a
	// player attacks a player.
	factor := 2
	if src.Player == nil && target.Player != nil {
		factor = 3
	}
	cri -= target.Luk * factor
	// Status changes (Sleep, Auto Counter, Camouflage) are not ported.
	if tsd := target.Player; tsd != nil && tsd.CriticalDef != 0 {
		cri = cri * (100 - tsd.CriticalDef) / 100
	}
	return r.IntN(1000) < cri
}

// isAttackHitting is is_attack_hitting on its first call, for a normal
// attack.
func isAttackHitting(wd *damage, src, target *Status, critical bool, r Rand) bool {
	if critical {
		return true
	}
	if sd := src.Player; sd != nil && sd.PerfectHit > 0 && r.IntN(100) < sd.PerfectHit {
		return true
	}

	flee := target.Flee
	// The flee penalty for many attackers (agi_penalty_target) only
	// applies to player targets, which are not ported.

	// Renewal has no base hit rate.
	hitrate := src.Hit - flee

	if src.Player != nil {
		if isSkillUsingArrow(src) {
			hitrate += src.Player.Arrow.Hit
		}

		// Renewal: Vulture's Eye adds hit that the status does not show.
		hitrate += src.skill(skillVultureEye)

		// A double attack gets 1 hit per Double Attack level.
		if wd.typ&dmgMultiHit != 0 && wd.div == 2 {
			hitrate += src.skill(skillDoubleAttack)
		}

		// Weapon Research adds a hidden 2% per level.
		if lv := src.skill(skillWeaponResearch); lv > 0 {
			hitrate += hitrate * 2 * lv / 100
		}
		if lv := src.skill(skillTrainingSword); lv > 0 && (src.Player.Weapon == Weapon1HSword || src.Player.Weapon == WeaponDagger) {
			hitrate += 3 * lv
		}
	}

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
// battle_calc_skill_base_damage. In renewal, players use
// battle_calc_damage_parts, and other units use battle_calc_base_damage.
func calcSkillBaseDamage(wd *damage, src, target *Status, critical bool, r Rand) {
	bflag := 0
	if critical {
		bflag |= bdmgCrit
	}
	if src.Player == nil {
		wd.damage = calcBaseDamage(src, &src.RHW, target.Size, bflag, r)
		if isAttackLeftHanded(src) {
			wd.damage2 = calcBaseDamage(src, &src.LHW, target.Size, bflag, r)
		}
		return
	}
	calcDamageParts(wd, src, target, critical, r)

	// Renewal: Taekwon Mission (TK_POWER) adds 20% per level to every ATK
	// part, with or without a party.
	if lv := src.skill(skillTKPower); lv > 0 {
		pct := int64(20 * lv)
		atkAddRate(src, &wd.damage, &wd.damage2, pct)
		atkAddRate(src, &wd.statusAtk, &wd.statusAtk2, pct)
		atkAddRate(src, &wd.weaponAtk, &wd.weaponAtk2, pct)
		atkAddRate(src, &wd.equipAtk, &wd.equipAtk2, pct)
		atkAddRate(src, &wd.masteryAtk, &wd.masteryAtk2, pct)
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

// calcDefenseReduction is battle_calc_defense_reduction in renewal:
// damage * (4000 + DEF) / (4000 + DEF * 10) minus soft DEF. A hand that
// ignores DEF keeps its damage, and a hand that pierces DEF gains DEF / 2.
func calcDefenseReduction(wd *damage, src, target *Status) {
	// status_get_def lowers DEF while the target casts. Sample monsters
	// do not cast.
	def1 := int64(target.Def)
	def2 := int64(target.Def2)

	if sd := src.Player; sd != nil {
		// bIgnoreDefRaceRate and bIgnoreDefClassRate ignore a percent of
		// both DEF values.
		i := int64(sd.IgnoreDefRace[target.Race] + sd.IgnoreDefRace[RaceAll])
		i += int64(sd.IgnoreDefClass[target.Class] + sd.IgnoreDefClass[ClassAll])
		if i != 0 {
			i = min(i, 100)
			def1 -= def1 * i / 100
			def2 -= def2 * i / 100
		}
	}

	// The defense penalty for many attackers (vit_penalty_target) counts
	// the units that target the player, which the recorder's direct
	// attacks do not.

	// Soft DEF is def2 for players and monsters in renewal.
	vitDef := def2
	if tsd := target.Player; tsd != nil && src.Player == nil {
		// Divine Protection, Ranger Main and Research Fire/Earth add soft
		// DEF against monsters.
		if lv := target.skill(skillDivineProtection); lv > 0 && (isUndead(src.DefEle) || src.Race == RaceDemon) {
			vitDef += int64((float32(target.Level)/25.0+3.0)*float32(lv) + 0.5)
		}
		if lv := target.skill(skillRangerMain); lv > 0 && (src.Race == RaceBrute || src.Race == RacePlayerDoram || src.Race == RaceFish || src.Race == RacePlant) {
			vitDef += int64(lv) * 5
		}
		if lv := target.skill(skillResearchFE); lv > 0 && (src.DefEle == Fire || src.DefEle == Earth) {
			vitDef += int64(lv) * 10
		}
	}

	if weaponDefenseType != 0 {
		vitDef += def1 * weaponDefenseType
		def1 = 0
	}

	// -400 would divide by zero.
	if def1 == -400 {
		def1 = -399
	}
	// A piercing hand gains DEF * skill ratio / 200, with a ratio of 100.
	if isAttackPiercing(src, target, handRight) {
		wd.damage += def1 * 100 / 200
	}
	if isAttackLeftHanded(src) && isAttackPiercing(src, target, handLeft) {
		wd.damage2 += def1 * 100 / 200
	}
	if !attackIgnoresDef(src, target, handRight) && !isAttackPiercing(src, target, handRight) {
		wd.damage = wd.damage*(4000+def1)/(4000+10*def1) - vitDef
	}
	if isAttackLeftHanded(src) && !attackIgnoresDef(src, target, handLeft) && !isAttackPiercing(src, target, handLeft) {
		wd.damage2 = wd.damage2*(4000+def1)/(4000+10*def1) - vitDef
	}
}

// handBits is a hand's flag test for attackIgnoresDef and
// isAttackPiercing: whether its bits name the target's element, race or
// class, or every one.
func handBits(ele, race, class uint32, target *Status) bool {
	return ele&(1<<target.DefEle) != 0 || ele&(1<<AllElements) != 0 ||
		race&(1<<target.Race) != 0 || race&(1<<RaceAll) != 0 ||
		class&(1<<target.Class) != 0 || class&(1<<ClassAll) != 0
}

// attackIgnoresDef is attack_ignores_def for a normal attack in renewal:
// bIgnoreDefEle, bIgnoreDefRace and bIgnoreDefClass. With
// left_cardfix_to_right, the left hand's flags count for the right hand.
func attackIgnoresDef(src, target *Status, h hand) bool {
	sd := src.Player
	if sd == nil {
		return false
	}
	if h == handRight && handBits(sd.Right.IgnoreDefEle, sd.Right.IgnoreDefRace, sd.Right.IgnoreDefClass, target) {
		return true
	}
	if handBits(sd.Left.IgnoreDefEle, sd.Left.IgnoreDefRace, sd.Left.IgnoreDefClass, target) {
		if isAttackRightHanded(src) {
			return h == handRight
		}
		return h == handLeft
	}
	return false
}

// isAttackPiercing is is_attack_piercing for a normal attack in renewal:
// bDefRatioAtkEle, bDefRatioAtkRace and bDefRatioAtkClass.
func isAttackPiercing(src, target *Status, h hand) bool {
	sd := src.Player
	if sd == nil {
		return false
	}
	if h == handRight && handBits(sd.Right.DefRatioEle, sd.Right.DefRatioRace, sd.Right.DefRatioClass, target) {
		return true
	}
	if handBits(sd.Left.DefRatioEle, sd.Left.DefRatioRace, sd.Left.DefRatioClass, target) {
		if isAttackRightHanded(src) {
			return h == handRight
		}
		return h == handLeft
	}
	return false
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
	// Renewal players apply the element table to each ATK part in
	// battle_calc_damage_parts instead.
	if (wd.damage > 0 || wd.damage2 > 0) && src.Player == nil {
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
		if sd := src.Player; sd != nil && sd.Weapon == WeaponKatar {
			// Katars deal no second hit to plants.
			wd.damage2 = 0
		} else {
			wd.damage2 = 1
		}
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
	if src.Player != nil {
		calcLeftRightHands(wd, src)
	}
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

// atkAddRate is rAthena's ATK_ADDRATE macro: adds a percent to a value and
// to its left-hand value.
func atkAddRate(src *Status, v, v2 *int64, pct int64) {
	*v += *v * pct / 100
	if isAttackLeftHanded(src) {
		*v2 += *v2 * pct / 100
	}
}

// atkAdd is rAthena's ATK_ADD macro: adds an amount to a value and to its
// left-hand value.
func atkAdd(src *Status, v, v2 *int64, n int64) {
	*v += n
	if isAttackLeftHanded(src) {
		*v2 += n
	}
}
