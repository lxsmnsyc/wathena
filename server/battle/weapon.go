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

// damage is rAthena's Damage struct, with the fields attacks use. The
// renewal ATK parts (statusAtk, weaponAtk and so on) are only used by
// players. Monsters get their damage directly.
type damage struct {
	damage, damage2 int64
	typ             damageType
	div             int
	dmgLv           damageLevel
	flag            int
	// blewcount is the knockback, and miscflag the wflag or mflag.
	blewcount, miscflag int
	// sk is the skill, or nil for a normal attack, and nk its damage
	// flags.
	sk *Skill
	nk NK
	// pre is set for an attack in pre-renewal.
	pre bool
	// basedamage is pre-renewal's damage of a normal attack with the
	// right hand, which SC_WATK_ELEMENT reads.
	basedamage int64

	statusAtk, statusAtk2   int64
	weaponAtk, weaponAtk2   int64
	equipAtk, equipAtk2     int64
	masteryAtk, masteryAtk2 int64
	percentAtk, percentAtk2 int64
}

// calcWeaponAttack is battle_calc_weapon_attack in renewal, for a normal
// attack (sk is nil) or a skill. The random draws happen in rAthena's
// order: lucky dodge, double attack, critical, hit, then the damage rolls.
func calcWeaponAttack(src, target *Status, sk *Skill, r Rand) damage {
	sd := src.Player
	wd := initializeWeaponData(src, sk)

	rightElement := weaponElement(src, handRight, wd.sk, r)
	leftElement := weaponElement(src, handLeft, wd.sk, r)

	// Check for Lucky Dodge. Skills cannot be lucky dodged.
	if sk == nil && target.Flee2 != 0 && r.IntN(1000) < target.Flee2 {
		wd.typ = dmgLuckyDodge
		wd.dmgLv = atkLucky
		if wd.div < 0 {
			wd.div = -wd.div
		}
		return wd
	}

	// Multiple hits come first, so a double attack can override a
	// critical hit.
	calcMultiAttack(&wd, src, doubleAttackRate, r)

	// Critical hits always hit, so the critical check comes next.
	if isAttackCritical(&wd, src, target, doubleAttackCanCrit, chainActionCanCrit, r) {
		if wd.typ == dmgMultiHit {
			wd.typ = dmgMultiHitCritical
		} else {
			wd.typ = dmgCritical
		}
	}
	critical := wd.typ == dmgCritical || wd.typ == dmgMultiHitCritical

	infdef := false
	if !isAttackHitting(&wd, src, target, critical, false, r) {
		wd.dmgLv = atkFlee
	} else if infdef = isInfiniteDefense(target, wd.flag); !infdef {
		calcSkillBaseDamage(&wd, src, target, critical, r)

		if sd != nil {
			// Renewal applies the attacker's cards to weapon and
			// equipment ATK only.
			wd.weaponAtk += cardfixAttacker(src, target, wd.weaponAtk, 2, wd.flag, wd.nk)
			wd.equipAtk += cardfixAttacker(src, target, wd.equipAtk, 2, wd.flag, wd.nk)
			// left_cardfix_to_right moves the left hand's bonuses to the
			// right hand, so the left hand only gets bAddDamageClass.
			if isAttackLeftHanded(src, wd.sk) {
				wd.weaponAtk2 += cardfixAttacker(src, target, wd.weaponAtk2, 3, wd.flag, wd.nk)
				wd.equipAtk2 += cardfixAttacker(src, target, wd.equipAtk2, 3, wd.flag, wd.nk)
			}
		}
		// A player target's cards apply to the ATK parts when a player
		// attacks a player. Players attacking players is not ported.

		// battle_attack_sc_bonus: SC_WATK_ELEMENT raises weapon ATK by
		// its percent, except for Meteor Assault. Its other parts are
		// other status changes, spirit charms, Star Gladiator's anger
		// against a marked monster and the Summoner's Power of Life, which
		// are not ported.
		if c := src.Changes; c != nil && c.WatkElementRate != 0 && wd.skillID() != skillMeteorAssault {
			wd.atkAddRate(src, &wd.weaponAtk, &wd.weaponAtk2, int64(c.WatkElementRate))
		}

		// Players sum their ATK parts. Monsters already have their
		// damage from battle_calc_base_damage.
		if sd != nil {
			sumPlayerDamage(&wd, src)
		}

		// Skill ratio: battle_calc_attack_skill_ratio, which starts from
		// the ATK percent of status changes in renewal.
		ratio := skillRatio(&wd, src, target, true)
		wd.rate(src, int64(ratio))

		// Additive damage bonus.
		wd.atkAdd(src, &wd.damage, &wd.damage2, skillConstantAddition(&wd, src, target))

		// Advanced Katar Mastery applies after the skill ratio.
		if lv := src.skill(skillAdvKatarMastery); sd != nil && sd.Weapon == WeaponKatar && lv > 0 {
			wd.atkAddRate(src, &wd.damage, &wd.damage2, int64(10+2*lv))
		}

		// Res reduces physical damage by a percent before DEF. Skills with
		// the simple defense formula ignore Res.
		if wd.damage+wd.damage2 != 0 && target.Res > 0 && !wd.nk.Has(NKSimpleDefense) {
			// Players can ignore part of Res with bonuses that are not
			// ported.
			res := float64(target.Res)
			part := float64(float32(res / (res + 400.0)))
			wd.damage -= int64(part * 80.0 / 100.0 * float64(wd.damage))
			wd.damage2 -= int64(part * 80.0 / 100.0 * float64(wd.damage2))
		}

		if wd.damage+wd.damage2 != 0 {
			if !attackIgnoresDef(&wd, src, target, handLeft) || !attackIgnoresDef(&wd, src, target, handRight) {
				calcDefenseReduction(&wd, src, target, ratio)
			}
			calcAttackPostDefense(&wd, src)
		}

		// pc_skillatk_bonus and pc_sub_skillatk_bonus.
		if sd != nil && sk != nil {
			if i := int64(sd.SkillAtk[sk.ID]); i != 0 {
				wd.atkAddRate(src, &wd.damage, &wd.damage2, i)
				wd.allAtkAddRate(src, i)
			}
		}
		if tsd := target.Player; tsd != nil && sk != nil {
			if i := int64(tsd.SubSkill[sk.ID]); i != 0 {
				wd.atkAddRate(src, &wd.damage, &wd.damage2, -i)
				wd.allAtkAddRate(src, -i)
			}
		}
	}

	calcElementDamage(&wd, src, target, rightElement, leftElement)

	// Renewal critical damage, with the attacker's C.Rate.
	if critical {
		if sd != nil {
			rate := float32(1.4) + float32(0.01)*float32(src.CRate)
			wd.damage = int64(math.Floor(float64(float32(wd.damage) * rate)))
			if isAttackLeftHanded(src, wd.sk) {
				wd.damage2 = int64(math.Floor(float64(float32(wd.damage2) * rate)))
			}
		} else {
			wd.damage = int64(math.Floor(float64(float32(wd.damage) * 1.4)))
		}
		if tsd := target.Player; tsd != nil && tsd.CritDefRate != 0 {
			wd.atkAddRate(src, &wd.damage, &wd.damage2, int64(-tsd.CritDefRate))
		}
	}

	// Renewal applies the target's cards here only when a monster
	// attacks a player.
	if target.Player != nil && sd == nil {
		wd.damage += cardfixTarget(src, target, wd.damage, rightElement, leftElement, 0, wd.flag, wd.nk)
		if isAttackLeftHanded(src, wd.sk) {
			wd.damage2 += cardfixTarget(src, target, wd.damage2, rightElement, leftElement, 1, wd.flag, wd.nk)
		}
	}

	// A target that takes 1 damage from every hit.
	if infdef {
		calcAttackPlant(&wd, src, target)
		return wd
	}

	applyDivFix(&wd)
	calcAttackLeftRightHands(&wd, src)
	calcAttackGvgBg(&wd, src, target)

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

// initializeWeaponData is initialize_weapon_data: the hit count, the
// knockback and the battle flags of a normal attack or a skill.
func initializeWeaponData(src *Status, sk *Skill) damage {
	wd := damage{typ: dmgNormal, div: 1, dmgLv: atkDef, flag: bfWeapon, sk: sk}
	if sk != nil {
		wd.div = sk.Hits
		wd.blewcount = sk.Knockback
		wd.miscflag = sk.Flag
		wd.nk = sk.NK
	}
	if sk != nil || wd.miscflag != 0 {
		wd.flag |= bfSkill
	} else {
		wd.flag |= bfNormal
	}
	if sk != nil {
		wd.flag |= rangeType(src, sk)
	} else if isSkillUsingArrow(src, sk) {
		wd.flag |= bfLong
	} else {
		wd.flag |= bfShort
	}
	return wd
}

// isSkillUsingArrow is is_skill_using_arrow. A player uses ammunition
// when ArrowAttack is set (state.arrow_atk). Another unit uses it with a
// skill that needs ammunition, or when its attack range is over 3 cells.
func isSkillUsingArrow(src *Status, sk *Skill) bool {
	if src.Player != nil {
		if src.Player.ArrowAttack {
			return true
		}
	} else {
		if sk != nil && sk.Ammo {
			return true
		}
		if src.RHW.Range > 3 {
			return true
		}
	}
	if sk != nil {
		switch sk.ID {
		case skillFreezingTrap, skillPhantasmic, skillGroundDrift:
			return true
		}
	}
	return false
}

// isAttackRightHanded is is_attack_right_handed. Skills always use the
// right hand. A normal attack is only left handed for a player with a
// weapon in the left hand only.
func isAttackRightHanded(src *Status, sk *Skill) bool {
	if sd := src.Player; sk == nil && sd != nil && sd.RightWeapon == WeaponFist && sd.LeftWeapon != WeaponFist {
		return false
	}
	return true
}

// isAttackLeftHanded is is_attack_left_handed: for a normal attack, a
// player with a weapon in the left hand only or with a katar, or any unit
// with left-hand ATK. Skills never use the left hand.
func isAttackLeftHanded(src *Status, sk *Skill) bool {
	if sk != nil {
		return false
	}
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

// isAttackCritical is is_attack_critical on its first call. Later calls
// in rAthena only read the damage type. A skill is only critical with the
// Critical damage flag. doubleCanCrit and chainCanCrit tell whether
// Double Attack and Chain Action have the Critical damage flag in the
// mode's skill_db.yml.
func isAttackCritical(wd *damage, src, target *Status, doubleCanCrit, chainCanCrit bool, r Rand) bool {
	if wd.sk != nil && !wd.nk.Has(NKCritical) {
		return false
	}
	if src.Cri == 0 {
		return false
	}
	// Double hits can only be critical when the skill has the Critical
	// damage flag.
	if wd.typ == dmgMultiHit {
		if src.skill(skillChainAction) > 0 && !chainCanCrit {
			return false
		}
		if src.skill(skillDoubleAttack) > 0 && !doubleCanCrit {
			return false
		}
	}
	cri := src.Cri
	if sd := src.Player; sd != nil {
		cri += sd.CritAddRace[target.Race] + sd.CritAddRace[RaceAll]
		if wd.sk == nil && isSkillUsingArrow(src, wd.sk) {
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
	// Sleep doubles it. Auto Counter and Camouflage come with their
	// skills.
	if tc := target.Changes; tc != nil && tc.Sleep {
		cri *= 2
	}
	if tsd := target.Player; tsd != nil && tsd.CriticalDef != 0 {
		cri = cri * (100 - tsd.CriticalDef) / 100
	}
	return r.IntN(1000) < cri
}

// isAttackHitting is is_attack_hitting on its first call. pre selects the
// pre-renewal rules: a base hit rate of 80, and no hidden hit from
// Vulture's Eye.
func isAttackHitting(wd *damage, src, target *Status, critical, pre bool, r Rand) bool {
	if critical {
		return true
	}
	if sd := src.Player; sd != nil && sd.PerfectHit > 0 && r.IntN(100) < sd.PerfectHit {
		return true
	}
	// A target that is stunned, frozen, asleep or stone is always hit.
	if tc := target.Changes; tc != nil && tc.Opt1 != 0 && tc.Opt1 != opt1StoneWait && tc.Opt1 != opt1Burning {
		return true
	}
	if wd.nk.Has(NKIgnoreFlee) {
		return true
	}

	flee := target.Flee
	// The flee penalty for many attackers (agi_penalty_target) only
	// applies to player targets, which are not ported.

	// Renewal has no base hit rate. Pre-renewal starts from 80.
	hitrate := src.Hit - flee
	if pre {
		hitrate += 80
	}

	if src.Player != nil {
		if isSkillUsingArrow(src, wd.sk) {
			hitrate += src.Player.Arrow.Hit
		}

		// Renewal: Vulture's Eye adds hit that the status does not show.
		if !pre {
			hitrate += src.skill(skillVultureEye)
		}
	}

	if wd.sk != nil {
		if impl := wd.sk.Impl; impl != nil {
			impl.HitRate(wd.context(src, target), &hitrate)
		}
	} else if src.Player != nil && wd.typ&dmgMultiHit != 0 && wd.div == 2 {
		// A double attack gets 1 hit per Double Attack level.
		hitrate += src.skill(skillDoubleAttack)
	}

	if src.Player != nil {
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

// calcSkillBaseDamage is battle_calc_skill_base_damage in renewal. No
// first class skill has a base damage of its own, so every skill takes
// the default case: players use battle_calc_damage_parts, and other units
// use battle_calc_base_damage. A skill that splits its damage divides it
// among its targets.
func calcSkillBaseDamage(wd *damage, src, target *Status, critical bool, r Rand) {
	bflag := 0
	if critical {
		bflag |= bdmgCrit
	}
	if src.Player == nil {
		wd.damage = calcBaseDamage(src, &src.RHW, target.Size, bflag, r)
		if isAttackLeftHanded(src, wd.sk) {
			wd.damage2 = calcBaseDamage(src, &src.LHW, target.Size, bflag, r)
		}
	} else {
		calcDamageParts(wd, src, target, critical, r)
	}

	if wd.nk.Has(NKSplashSplit) && wd.miscflag > 0 {
		n := int64(wd.miscflag)
		wd.damage /= n
		wd.statusAtk /= n
		wd.weaponAtk /= n
		wd.equipAtk /= n
		wd.masteryAtk /= n
	}

	// Renewal: Taekwon Mission (TK_POWER) adds 20% per level to every ATK
	// part, with or without a party.
	if lv := src.skill(skillTKPower); lv > 0 {
		pct := int64(20 * lv)
		wd.atkAddRate(src, &wd.damage, &wd.damage2, pct)
		wd.allAtkAddRate(src, pct)
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
// ignores DEF keeps its damage, and a hand that pierces DEF gains DEF times
// the skill ratio / 200. Skills with the simple defense formula subtract
// both DEF values instead.
func calcDefenseReduction(wd *damage, src, target *Status, ratio int) {
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

	if wd.skillID() == skillAcidTerror {
		// Acid Terror ignores only status DEF.
		def2 = 0
	}

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

	if wd.nk.Has(NKSimpleDefense) {
		// A flat reduction that bypasses the renewal formula.
		wd.damage -= def1 + vitDef
		if isAttackLeftHanded(src, wd.sk) {
			wd.damage2 -= def1 + vitDef
		}
		return
	}

	// -400 would divide by zero.
	if def1 == -400 {
		def1 = -399
	}
	if isAttackPiercing(wd, src, target, handRight) {
		wd.damage += def1 * int64(ratio) / 200
	}
	if isAttackLeftHanded(src, wd.sk) && isAttackPiercing(wd, src, target, handLeft) {
		wd.damage2 += def1 * int64(ratio) / 200
	}
	if !attackIgnoresDef(wd, src, target, handRight) && !isAttackPiercing(wd, src, target, handRight) {
		wd.damage = wd.damage*(4000+def1)/(4000+10*def1) - vitDef
	}
	if isAttackLeftHanded(src, wd.sk) && !attackIgnoresDef(wd, src, target, handLeft) && !isAttackPiercing(wd, src, target, handLeft) {
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

// attackIgnoresDef is attack_ignores_def in renewal: bIgnoreDefEle,
// bIgnoreDefRace and bIgnoreDefClass, and the Ignore Defense damage flag.
// With left_cardfix_to_right, the left hand's flags count for the right
// hand.
func attackIgnoresDef(wd *damage, src, target *Status, h hand) bool {
	if sd := src.Player; sd != nil {
		if h == handRight && handBits(sd.Right.IgnoreDefEle, sd.Right.IgnoreDefRace, sd.Right.IgnoreDefClass, target) {
			return true
		}
		if handBits(sd.Left.IgnoreDefEle, sd.Left.IgnoreDefRace, sd.Left.IgnoreDefClass, target) {
			if isAttackRightHanded(src, wd.sk) {
				if h == handRight {
					return true
				}
			} else if h == handLeft {
				return true
			}
		}
	}
	return wd.nk.Has(NKIgnoreDefense)
}

// isAttackPiercing is is_attack_piercing in renewal: bDefRatioAtkEle,
// bDefRatioAtkRace and bDefRatioAtkClass. Investigate always pierces,
// and a few skills never do.
func isAttackPiercing(wd *damage, src, target *Status, h hand) bool {
	switch wd.skillID() {
	case skillInvestigate:
		return true
	case skillSacrifice, skillShieldChain:
		return false
	}
	sd := src.Player
	if sd == nil {
		return false
	}
	if h == handRight && handBits(sd.Right.DefRatioEle, sd.Right.DefRatioRace, sd.Right.DefRatioClass, target) {
		return true
	}
	if handBits(sd.Left.DefRatioEle, sd.Left.DefRatioRace, sd.Left.DefRatioClass, target) {
		if isAttackRightHanded(src, wd.sk) {
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
// least minimum, and left-hand damage to at least 0. Pre-renewal raises
// the base damage of a normal attack too.
func minDamage(wd *damage, src *Status, minimum int64) {
	if isAttackRightHanded(src, wd.sk) {
		wd.damage = max(wd.damage, minimum)
		if wd.pre {
			wd.basedamage = max(wd.basedamage, minimum)
		}
	}
	if isAttackLeftHanded(src, wd.sk) {
		wd.damage2 = max(wd.damage2, 0)
	}
}

// calcElementDamage is battle_calc_element_damage in renewal.
func calcElementDamage(wd *damage, src, target *Status, rightElement, leftElement int) {
	if !wd.nk.Has(NKIgnoreElement) && (wd.damage > 0 || wd.damage2 > 0) {
		// Renewal players apply the element table to each ATK part in
		// battle_calc_damage_parts instead.
		if src.Player == nil {
			// Normal attacks by monsters are non-elemental and deal 100%
			// against every defense element (attack_attr_none).
			if wd.sk == nil && monsterAttackAttrNone {
				return
			}
			wd.damage = AttrFix(wd.damage, rightElement, target.DefEle, target.EleLv, 1)
			if isAttackLeftHanded(src, wd.sk) {
				wd.damage2 = AttrFix(wd.damage2, leftElement, target.DefEle, target.EleLv, 1)
			}
		}
		// Skills forced to neutral gain from the weapon's element, and the
		// element table applies again with neutral.
		switch wd.skillID() {
		case skillInvestigate, skillExtremityFist, skillShieldBoomerang, skillShieldChain, skillCartRevolution, skillMagicCrasher:
			wd.damage = AttrFix(wd.damage, Neutral, target.DefEle, target.EleLv, 1)
			if isAttackLeftHanded(src, wd.sk) {
				wd.damage2 = AttrFix(wd.damage2, Neutral, target.DefEle, target.EleLv, 1)
			}
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
func calcAttackPlant(wd *damage, src, target *Status) {
	// Only a hit reaches here, so the attack always counts as hitting.
	wd.damage = 1
	if isAttackLeftHanded(src, wd.sk) {
		if sd := src.Player; sd != nil && sd.Weapon == WeaponKatar {
			// Katars deal no second hit to plants.
			wd.damage2 = 0
		} else {
			wd.damage2 = 1
		}
	}
	// A monster's statuses that block the hit, such as Safety Wall.
	if tc := target.Changes; target.Player == nil && tc != nil && tc.Block != nil && tc.Block(wd.flag, wd.skillID(), 1) {
		wd.damage, wd.damage2 = 0, 0
		wd.dmgLv = atkBlock
		return
	}

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
	if !isAttackRightHanded(src, wd.sk) && !isAttackLeftHanded(src, wd.sk) && wd.damage != 0 {
		wd.damage = 0
	}
	if !isAttackLeftHanded(src, wd.sk) && wd.damage2 != 0 {
		wd.damage2 = 0
	}
}

// calcAttackGvgBg is battle_calc_attack_gvg_bg outside GvG and
// battlegrounds. Damage returned by reflection does not change the
// attack's damage.
func calcAttackGvgBg(wd *damage, src, target *Status) {
	if wd.damage+wd.damage2 == 0 {
		return
	}
	switch {
	case wd.damage2 == 0:
		wd.damage = calcDamage(wd, src, target, wd.damage)
	case wd.damage == 0:
		wd.damage2 = calcDamage(wd, src, target, wd.damage2)
	default:
		wd.damage = calcDamage(wd, src, target, wd.damage)
		wd.damage2 = calcDamage(wd, src, target, wd.damage2)
		if wd.damage > 1 && wd.damage2 < 1 {
			wd.damage2 = 1
		}
	}
}

// calcDamage is battle_calc_damage outside PK maps: the target's statuses
// that block or reduce the hit, then the target's damage taken rate.
func calcDamage(wd *damage, src, target *Status, dmg int64) int64 {
	if dmg == 0 {
		return 0
	}
	// ksprotection is off by default.

	tc := target.Changes
	// Energy Coat reduces physical damage, and magic damage in renewal,
	// by 6% per 20% of SP, and costs SP.
	if tc != nil && tc.EnergyCoat != nil && (wd.flag&bfWeapon != 0 || !wd.pre && wd.flag&bfMagic != 0) {
		dmg -= dmg * int64(tc.EnergyCoat()) / 100
	}

	if skillMinDamage != 0 && dmg > 0 && dmg < int64(wd.div) && wd.flag&bfWeapon != 0 && skillMinDamage&1 != 0 {
		dmg = int64(wd.div)
	}

	// Some monsters, such as MVPs with a green aura, take less damage.
	if taken := target.DamageTaken; taken != 0 && taken != 100 {
		dmg = max(dmg*int64(taken)/100, 1)
	}

	// battle_status_block_damage, last: Safety Wall, Pneuma and the other
	// statuses that block a hit. Safety Wall's durability takes the
	// damage left after the reductions above.
	if tc != nil && tc.Block != nil && tc.Block(wd.flag, wd.skillID(), dmg) {
		wd.dmgLv = atkBlock
		return 0
	}
	return dmg
}

// rate is rAthena's ATK_RATE macro: both hands' damage times a percent.
func (wd *damage) rate(src *Status, pct int64) {
	wd.damage = wd.damage * pct / 100
	if isAttackLeftHanded(src, wd.sk) {
		wd.damage2 = wd.damage2 * pct / 100
	}
}

// atkAddRate is rAthena's ATK_ADDRATE macro: adds a percent to a value and
// to its left-hand value.
func (wd *damage) atkAddRate(src *Status, v, v2 *int64, pct int64) {
	*v += *v * pct / 100
	if isAttackLeftHanded(src, wd.sk) {
		*v2 += *v2 * pct / 100
	}
}

// atkAdd is rAthena's ATK_ADD macro: adds an amount to a value and to its
// left-hand value.
func (wd *damage) atkAdd(src *Status, v, v2 *int64, n int64) {
	*v += n
	if isAttackLeftHanded(src, wd.sk) {
		*v2 += n
	}
}

// allAtkAddRate is rAthena's RE_ALLATK_ADDRATE macro: ATK_ADDRATE on
// status, weapon, equipment and mastery ATK.
func (wd *damage) allAtkAddRate(src *Status, pct int64) {
	wd.atkAddRate(src, &wd.statusAtk, &wd.statusAtk2, pct)
	wd.atkAddRate(src, &wd.weaponAtk, &wd.weaponAtk2, pct)
	wd.atkAddRate(src, &wd.equipAtk, &wd.equipAtk2, pct)
	wd.atkAddRate(src, &wd.masteryAtk, &wd.masteryAtk2, pct)
}

// context is the SkillContext of the attack.
func (wd *damage) context(src, target *Status) *SkillContext {
	return &SkillContext{Src: src, Target: target, Skill: wd.sk, Flag: wd.miscflag, Renewal: !wd.pre}
}
