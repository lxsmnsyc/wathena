package battle

import (
	"math"

	"github.com/lxsmnsyc/wathena/server/rules"
)

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
	// rules are the world's ruleset flags. nil follows the base of the
	// attack's mode.
	rules *rules.Rules
	// basedamage is pre-renewal's damage of a normal attack with the
	// right hand, which SC_WATK_ELEMENT reads.
	basedamage int64
	// rejected is the damage Reject Sword returned to the attacker.
	rejected int64

	statusAtk, statusAtk2   int64
	weaponAtk, weaponAtk2   int64
	equipAtk, equipAtk2     int64
	masteryAtk, masteryAtk2 int64
	percentAtk, percentAtk2 int64
}

// calcWeaponAttack is battle_calc_weapon_attack in renewal, for a normal
// attack (sk is nil) or a skill. The random draws happen in rAthena's
// order: lucky dodge, double attack, critical, hit, then the damage rolls.
func calcWeaponAttack(src, target *Status, sk *Skill, rl *rules.Rules, r Rand) damage {
	sd := src.Player
	wd := initializeWeaponData(src, sk)
	if rl != nil {
		wd.rules = rl
	}
	modifyDamageData(&wd, src, target)

	rightElement := modifyElement(&wd, src, target, weaponElement(src, handRight, wd.sk, r))
	leftElement := modifyElement(&wd, src, target, weaponElement(src, handLeft, wd.sk, r))

	// Check for Lucky Dodge. Skills cannot be lucky dodged, except
	// Martyr's Reckoning.
	if (sk == nil || wd.skillID() == skillSacrifice) && target.Flee2 != 0 && r.IntN(1000) < target.Flee2 {
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

		// Magic Crasher adds a MATK roll to weapon ATK.
		if wd.skillID() == skillMagicCrasher {
			matk := int64(src.MAtkMin)
			if src.MAtkMax > src.MAtkMin {
				matk += int64(r.IntN(src.MAtkMax - src.MAtkMin))
			}
			wd.atkAdd(src, &wd.weaponAtk, &wd.weaponAtk2, matk)
		}

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
		// SC_DRUMBATTLE adds its ATK to equipment ATK.
		if c := src.Changes; c != nil && c.DrumBattle != 0 {
			wd.atkAdd(src, &wd.equipAtk, &wd.equipAtk2, int64(c.DrumBattle))
		}
		edpBonus(&wd, src)

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
		wd.atkAdd(src, &wd.damage, &wd.damage2, skillConstantAddition(&wd, src, target, r))

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

	calcWeaponFinalAtkModifiers(&wd, src, target, r)
	// battle_do_reflect does not change the attacker's damage.
	absorbDamage(&wd, target)
	return wd
}

// calcWeaponFinalAtkModifiers is battle_calc_weapon_final_atk_modifiers,
// for the statuses ported so far: Reject Sword halves a hit of a monster,
// or of a player with a dagger or a sword, and returns the halved damage
// to the attacker. The zone deals the returned damage and counts the
// reflection.
func calcWeaponFinalAtkModifiers(wd *damage, src, target *Status, r Rand) {
	tc := target.Changes
	if wd.damage == 0 || tc == nil || tc.RejectSword <= 0 {
		return
	}
	if sd := src.Player; sd != nil && sd.RightWeapon != WeaponDagger && sd.RightWeapon != Weapon1HSword &&
		sd.Weapon != Weapon2HSword {
		return
	}
	if r.IntN(100) >= tc.RejectSword {
		return
	}
	wd.damage = wd.damage * 50 / 100
	wd.rejected = wd.damage
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
		wd.rules = sk.Rules
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
	// NPC_CRITICALSLASH is always critical. LG_PINPOINTATTACK is not
	// ported.
	if wd.skillID() == skillNpcCriticalSlash {
		return true
	}
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
	// Sleep doubles it. Camouflage comes with its skill.
	if tc := target.Changes; tc != nil && tc.Sleep {
		cri *= 2
	}
	// Auto Counter's counter is always critical: auto_counter_type is 15,
	// every unit type. A normal attack under SC_AUTOCOUNTER would end it
	// and be critical too, but the status keeps its unit from attacking
	// (NoAttack).
	if wd.skillID() == skillAutoCounter {
		return true
	}
	// Renewal's Soul Breaker has half the critical chance.
	if wd.skillID() == skillSoulBreaker && !wd.pre {
		cri /= 2
	}
	if wd.skillID() == skillSharpShooting {
		// Focused Arrow Strike is critical more often.
		if wd.pre {
			cri += 200
		} else {
			cri += 300
		}
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
	// Venom Splasher's own target is always hit.
	if wd.skillID() == skillVenomSplasher && wd.miscflag == 0 {
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
	// Blinding Mist's hit penalty is only for ranged normal attacks.
	if tc := target.Changes; tc != nil && tc.FogWall && wd.flag&(bfLong|bfMagic) == bfLong && wd.sk == nil {
		hitrate -= 50
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
	// Rapid Smiting gives a flat 20 hit after the cap.
	if wd.skillID() == skillShieldChain {
		hitrate += 20
	}
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
	if target.hasMode(ModeIgnoreMisc) && flag&bfMisc != 0 {
		return true
	}
	// SC_INVINCIBLE.
	return target.Changes != nil && target.Changes.Invincible
}

// calcSkillBaseDamage is battle_calc_skill_base_damage in renewal. No
// first class skill has a base damage of its own, so every skill takes
// the default case: players use battle_calc_damage_parts, and other units
// use battle_calc_base_damage. A skill that splits its damage divides it
// among its targets.
func calcSkillBaseDamage(wd *damage, src, target *Status, critical bool, r Rand) {
	switch wd.skillID() {
	case skillSpiralPierce:
		calcSpiralPierceBase(wd, src, target, r)
		return
	case skillSacrifice:
		// Martyr's Reckoning: 9% of max HP, as weapon ATK.
		wd.damage = int64(src.MaxHP * 9 / 100)
		wd.damage2 = 0
		wd.weaponAtk, wd.weaponAtk2 = wd.damage, wd.damage2
		return
	case skillShieldBoomerang, skillShieldChain:
		calcShieldBase(wd, src, true)
		return
	}
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

// calcShieldBase is the Shield Boomerang and Shield Chain case of
// battle_calc_skill_base_damage: base ATK, plus 4 per refine and a tenth
// of the weight of a player's shield, which renewal counts as weapon ATK
// too. A unit that is not a player adds its atk2 instead. Pre-renewal's
// DEF reduction, which comes next for players, is the caller's.
func calcShieldBase(wd *damage, src *Status, renewal bool) {
	wd.damage = int64(src.BAtk)
	sd := src.Player
	if sd == nil {
		wd.atkAdd(src, &wd.damage, &wd.damage2, int64(src.RHW.Atk2))
		return
	}
	if !sd.Shield {
		return
	}
	wd.atkAdd(src, &wd.damage, &wd.damage2, int64(4*sd.ShieldRefine))
	wd.atkAdd(src, &wd.damage, &wd.damage2, int64(sd.ShieldWeight/10))
	if renewal {
		wd.atkAdd(src, &wd.weaponAtk, &wd.weaponAtk2, int64(4*sd.ShieldRefine))
		wd.atkAdd(src, &wd.weaponAtk, &wd.weaponAtk2, int64(sd.ShieldWeight/10))
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
	// Maximize Power takes the maximum.
	if c := st.Changes; c != nil && c.MaximizePower {
		atkmin = atkmax
	}

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
	// Grand Cross and NPC_GRANDDARKNESS skip the bonuses that ignore DEF.
	if sd := src.Player; sd != nil && wd.skillID() != skillGrandCross && wd.skillID() != skillGrandDarkness {
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
	case skillSacrifice, skillShieldChain, skillGrandCross, skillGrandDarkness:
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

// calcAttackPostDefense is battle_calc_attack_post_defense in renewal:
// Aura Blade adds (3 + its level) times the base level, then damage is
// at least 1.
func calcAttackPostDefense(wd *damage, src *Status) {
	if c := src.Changes; c != nil && c.AuraBlade > 0 {
		wd.atkAdd(src, &wd.damage, &wd.damage2, int64((3+c.AuraBlade)*src.Level))
	}
	minDamage(wd, src, 1)
	// Sonic Acceleration adds 90% to Sonic Blow.
	if wd.skillID() == skillSonicBlow && src.skill(skillSonicAcceleration) > 0 {
		wd.atkAddRate(src, &wd.damage, &wd.damage2, 90)
	}
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
		// battle_calc_damage_parts instead. Martyr's Reckoning has no ATK
		// parts, so every unit applies it here.
		if wd.skillID() == skillSacrifice {
			wd.damage = wd.attrFixPlain(wd.damage, rightElement, target.DefEle, target.EleLv, 1)
			if isAttackLeftHanded(src, wd.sk) {
				wd.damage2 = wd.attrFixPlain(wd.damage2, leftElement, target.DefEle, target.EleLv, 1)
			}
		} else if src.Player == nil {
			// Normal attacks by monsters are non-elemental and deal 100%
			// against every defense element (attack_attr_none).
			if wd.sk == nil && monsterAttackAttrNone {
				return
			}
			wd.damage = wd.attrFix(src, target, wd.damage, rightElement, target.DefEle, target.EleLv, 1)
			if isAttackLeftHanded(src, wd.sk) {
				wd.damage2 = wd.attrFix(src, target, wd.damage2, leftElement, target.DefEle, target.EleLv, 1)
			}
		}
		// Skills forced to neutral gain from the weapon's element, and the
		// element table applies again with neutral.
		switch wd.skillID() {
		case skillInvestigate, skillExtremityFist, skillShieldBoomerang, skillShieldChain, skillCartRevolution, skillMagicCrasher:
			wd.damage = wd.attrFix(src, target, wd.damage, Neutral, target.DefEle, target.EleLv, 1)
			if isAttackLeftHanded(src, wd.sk) {
				wd.damage2 = wd.attrFix(src, target, wd.damage2, Neutral, target.DefEle, target.EleLv, 1)
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

	// Triple Attack and Finger Offensive do not split their damage on a
	// plant. In pre-renewal the target needs exactly 100 DEF.
	if id := wd.skillID(); (id == skillTripleAttack || id == skillFingerOffensive) && wd.div < 0 && (!wd.pre || target.Def == 100) {
		wd.div = -wd.div
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

	// Pre-renewal Gravitation Field and Gloria Domini bypass everything
	// else. From here on, each status's own skill_rebalance flag picks
	// its RENEWAL branch.
	rl := wd.ruleset()
	if (wd.skillID() == skillGravitation || wd.skillID() == skillPressure) && !rl.Skill(wd.skillID()) {
		return dmg
	}

	tc := target.Changes
	// SC_BARRIER: a hit that deals damage deals 1 per hit, and nothing
	// else changes it.
	if tc != nil && tc.Barrier && dmg > 0 {
		return int64(wd.div)
	}
	if tc != nil && tc.Aeterna != nil && tc.Aeterna(wd.flag, wd.skillID()) {
		// Lex Aeterna doubles the damage and ends.
		dmg *= 2
	}
	// Assumptio halves the damage in pre-renewal, outside versus maps.
	if tc != nil && tc.Assumptio > 0 && !rl.Skill(skillAssumptio) {
		dmg /= 2
	}
	// Renewal's Sightless Mind: the target takes 30% more damage, or 15%
	// for a boss.
	if tc != nil && tc.Raid && rl.Skill(skillRaid) {
		if target.Class == ClassBoss {
			dmg += dmg * 15 / 100
		} else {
			dmg += dmg * 30 / 100
		}
	}
	// Blinding Mist: skills deal 25% less, and ranged weapon hits 75%
	// less.
	if tc != nil && tc.FogWall {
		if wd.flag&bfSkill != 0 {
			dmg -= dmg * 25 / 100
		} else if wd.flag&(bfLong|bfWeapon) == bfLong|bfWeapon {
			dmg /= 4
		}
	}
	// Defending Aura reduces ranged weapon damage. Throw Zeny and its
	// kin, which it skips, are not ported.
	if tc != nil && tc.Defender != 0 && wd.flag&(bfLong|bfWeapon) == bfLong|bfWeapon {
		dmg -= dmg * int64(tc.Defender) / 100
	}
	// SC_ARMORCHANGE changes the damage of weapon and magic hits.
	if tc != nil && tc.ArmorChangeWeapon|tc.ArmorChangeMagic != 0 {
		if wd.flag&bfWeapon != 0 {
			dmg -= dmg * int64(tc.ArmorChangeWeapon) / 100
		} else if wd.flag&bfMagic != 0 {
			dmg -= dmg * int64(tc.ArmorChangeMagic) / 100
		}
	}
	// SC_ARMOR (NPC_DEFENDER) divides the damage of ranged weapon and
	// misc hits.
	if tc != nil && tc.ArmorDiv != 0 && tc.ArmorRange&wd.flag != 0 && tc.ArmorType&wd.flag != 0 {
		dmg /= int64(tc.ArmorDiv)
	}
	// Energy Coat reduces physical damage, and magic damage in renewal,
	// by 6% per 20% of SP, and costs SP. Cart Termination goes through.
	if tc != nil && tc.EnergyCoat != nil && (wd.flag&bfWeapon != 0 || rl.Skill(skillEnergyCoat) && wd.flag&bfMagic != 0) &&
		wd.skillID() != skillCartTermination {
		dmg -= dmg * int64(tc.EnergyCoat()) / 100
	}
	// Renewal: Steel Body reduces every hit to a tenth.
	if tc != nil && tc.SteelBody && rl.Skill(skillSteelBody) {
		if dmg > 10 {
			dmg /= 10
		} else {
			dmg = 1
		}
	}

	if skillMinDamage != 0 && dmg > 0 && dmg < int64(wd.div) && wd.flag&bfWeapon != 0 && skillMinDamage&1 != 0 {
		dmg = int64(wd.div)
	}

	// Some monsters, such as MVPs with a green aura, take less damage.
	if taken := target.DamageTaken; taken != 0 && taken != 100 {
		dmg = max(dmg*int64(taken)/100, 1)
	}

	// battle_status_block_damage, last: Kyrie Eleison first, then Safety
	// Wall, Pneuma and the other statuses that block a hit. Safety Wall's
	// durability takes the damage left after the reductions above.
	if tc != nil && tc.Kyrie != nil && dmg > 0 {
		dmg = tc.Kyrie(wd.flag, wd.skillID(), dmg)
	}
	if tc != nil && tc.Block != nil && tc.Block(wd.flag, wd.skillID(), dmg) {
		wd.dmgLv = atkBlock
		return 0
	}
	// Pre-renewal Basilica blocks every hit from a unit that is not a
	// boss, with Pneuma's check.
	if tc != nil && tc.Basilica && !rl.Skill(skillBasilica) && !src.hasMode(ModeStatusImmune) {
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
	rl := wd.ruleset()
	return &SkillContext{Src: src, Target: target, Skill: wd.sk, Flag: wd.miscflag, Renewal: rl.Skill(wd.skillID()), Rules: rl}
}

// calcSpiralPierceBase is Spiral Pierce's case of the renewal
// battle_calc_skill_base_damage. A player's status, weapon and equipment
// ATK all count as equipment ATK, which adds a tenth of the weapon's
// weight, then takes 70%, then a size fix of its own: 130% on small and
// 115% on medium targets. Other units use their ATK.
func calcSpiralPierceBase(wd *damage, src, target *Status, r Rand) {
	sd := src.Player
	if sd == nil {
		wd.damage = calcBaseDamage(src, &src.RHW, target.Size, 0, r)
		return
	}
	calcDamageParts(wd, src, target, false, r)
	wd.atkAdd2(src, &wd.equipAtk, &wd.equipAtk2, wd.statusAtk+wd.weaponAtk, wd.statusAtk2+wd.weaponAtk2)
	wd.atkRate2(src, &wd.statusAtk, &wd.statusAtk2, 0, 0)
	wd.atkRate2(src, &wd.weaponAtk, &wd.weaponAtk2, 0, 0)
	if sd.RightWeapon != WeaponFist {
		wd.equipAtk += int64(sd.WeaponWeight / 10)
	}
	wd.atkRate2(src, &wd.equipAtk, &wd.equipAtk2, 70, 70)
	switch target.Size {
	case SizeSmall:
		wd.atkRate2(src, &wd.equipAtk, &wd.equipAtk2, 130, 130)
	case SizeMedium:
		wd.atkRate2(src, &wd.equipAtk, &wd.equipAtk2, 115, 115)
	}
}

// skillCartTermination is WS_CARTTERMINATION, which Energy Coat does not
// reduce.
const skillCartTermination = 485

// The skills whose statuses battle_calc_damage reads in RENEWAL
// branches, which their skill_rebalance flags pick.
const (
	skillEnergyCoat = 157 // MG_ENERGYCOAT
	skillRaid       = 214 // RG_RAID
	skillSteelBody  = 268 // MO_STEELBODY
	skillAssumptio  = 361 // HP_ASSUMPTIO
	skillBasilica   = 362 // HP_BASILICA
)
