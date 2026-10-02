package battle

// Pre-renewal port of rAthena's normal weapon attack: the #ifndef RENEWAL
// branches of battle.cpp. The helpers that both modes share live in
// weapon.go and player.go.

// Base damage flags that only pre-renewal reads, as rAthena's
// e_base_damage_flag.
const (
	// bdmgArrow is BDMG_ARROW: the attack uses ammunition with a bow or a
	// gun, so the minimum damage scales with weapon ATK and the
	// ammunition's ATK is added.
	bdmgArrow = 0x0002
	// bdmgNoSize is BDMG_NOSIZE: Weapon Perfection skips the size
	// penalty. Status changes are not ported, so it is never set.
	bdmgNoSize = 0x0008
)

// Pre-renewal Double Attack and Chain Action. Their skills do not have
// the Critical damage flag in db/pre-re/skill_db.yml, so a double hit is
// never critical.
const (
	preRenewalDoubleAttackRate    = 5
	preRenewalDoubleAttackCanCrit = false
	preRenewalChainActionCanCrit  = false
)

// AttackPreRenewal is a normal weapon attack on a target in rAthena's
// pre-renewal mode, as battle_calc_attack with BF_WEAPON, no skill and no
// status changes. The attacker is a player when its Player is set, and a
// monster otherwise. It reads the same Status fields as Attack, with these
// differences:
//   - RHW.Atk is the weapon's ATK and RHW.Atk2 the refine bonus for
//     players. LHW holds the left hand's.
//   - RHW.Wlv and LHW.Wlv are the weapon levels of the items in each hand.
//     rAthena reads them from the item, not from the status.
//   - BAtk already holds bonus bBaseAtk, so EAtk is not read.
//   - Player.AtkRate, Player.PreRenewalSizes, Status.Res, Status.CRate
//     and Status.PAtk are renewal only and are not read.
func AttackPreRenewal(attacker, target *Status, r Rand) Result {
	wd := calcWeaponAttackPreRenewal(attacker, target, nil, r)
	return finishAttack(&wd, attacker)
}

// SkillAttackPreRenewal is battle_calc_attack for a skill in pre-renewal:
// the attack type is BF_WEAPON, BF_MAGIC or BF_MISC.
func SkillAttackPreRenewal(typ int, src, target *Status, sk *Skill, r Rand) Result {
	var d damage
	switch typ {
	case TypeWeapon:
		d = calcWeaponAttackPreRenewal(src, target, sk, r)
	case TypeMagic:
		d = calcMagicAttack(src, target, sk, true, r)
	case TypeMisc:
		d = calcMiscAttack(src, target, sk, true, r)
	default:
		return Result{Kind: Miss}
	}
	return finishAttack(&d, src)
}

// calcWeaponAttackPreRenewal is the pre-renewal battle_calc_weapon_attack
// for a normal attack (sk is nil) or a skill. The random draws happen in
// rAthena's order: lucky dodge, double attack, critical, hit, the damage
// rolls, then the random part of soft DEF.
func calcWeaponAttackPreRenewal(src, target *Status, sk *Skill, r Rand) damage {
	sd := src.Player
	wd := initializeWeaponData(src, sk)
	wd.pre = true

	rightElement := weaponElement(src, handRight, sk, r)
	leftElement := weaponElement(src, handLeft, sk, r)

	// Check for Lucky Dodge. Skills cannot be lucky dodged.
	if sk == nil && target.Flee2 != 0 && r.IntN(1000) < target.Flee2 {
		wd.typ = dmgLuckyDodge
		wd.dmgLv = atkLucky
		return wd
	}

	// Multiple hits come first, so a double attack can override a
	// critical hit.
	calcMultiAttack(&wd, src, preRenewalDoubleAttackRate, r)

	// Critical hits always hit, so the critical check comes next.
	if isAttackCritical(&wd, src, target, preRenewalDoubleAttackCanCrit, preRenewalChainActionCanCrit, r) {
		if wd.typ == dmgMultiHit {
			wd.typ = dmgMultiHitCritical
		} else {
			wd.typ = dmgCritical
		}
	}
	critical := wd.typ == dmgCritical || wd.typ == dmgMultiHitCritical

	infdef := false
	if !isAttackHitting(&wd, src, target, critical, true, r) {
		wd.dmgLv = atkFlee
	} else if infdef = isInfiniteDefense(target, wd.flag); !infdef {
		// The base damage of a normal attack comes first, for the bonuses
		// that read it. A skill then rolls its own.
		calcSkillBaseDamagePreRenewal(&wd, src, target, critical, r)
		wd.basedamage = wd.damage
		if sk != nil {
			calcSkillBaseDamagePreRenewal(&wd, src, target, critical, r)
		}

		// Skill ratio and the additive damage bonus.
		wd.rate(src, int64(skillRatio(&wd, src, target, false)))
		wd.atkAdd(src, &wd.damage, &wd.damage2, skillConstantAddition(&wd, src, target))

		// pc_skillatk_bonus and pc_sub_skillatk_bonus.
		if sd != nil && sk != nil {
			if i := int64(sd.SkillAtk[sk.ID]); i != 0 {
				wd.atkAddRate(src, &wd.damage, &wd.damage2, i)
			}
		}
		if tsd := target.Player; tsd != nil && sk != nil {
			if i := int64(tsd.SubSkill[sk.ID]); i != 0 {
				wd.atkAddRate(src, &wd.damage, &wd.damage2, -i)
			}
		}

		// battle_attack_sc_bonus: pre-renewal applies the ATK percent of
		// status changes after the skill ratio. Its other parts need status
		// changes or spirit charms, which are not ported.
		wd.rate(src, atkPercent(src, sk))
		wd.basedamage = wd.basedamage * atkPercent(src, nil) / 100

		if wd.damage+wd.damage2 != 0 {
			calcDefenseReductionPreRenewal(&wd, src, target, critical, r)
			calcAttackPostDefensePreRenewal(&wd, src, target)
		}
	}

	calcElementDamagePreRenewal(&wd, src, target, rightElement, leftElement)

	if sd != nil {
		// Weapon Research and Hilt Binding add ATK after the element
		// table in pre-renewal.
		if lv := src.skill(skillWeaponResearch); lv > 0 {
			wd.atkAdd(src, &wd.damage, &wd.damage2, int64(lv)*2)
		}
		if wd.skillID() == skillGroundDrift {
			wd.atkAdd(src, &wd.damage, &wd.damage2, int64(50*wd.skillLv()))
		}
		if wd.skillID() != skillCartRevolution && src.skill(skillHiltBinding) > 0 {
			wd.atkAdd(src, &wd.damage, &wd.damage2, 4)
		}

		// Pre-renewal applies the attacker's cards to the whole damage.
		wd.damage += cardfixAttackerPreRenewal(src, target, wd.damage, 2, wd.flag, wd.nk)
		if isAttackLeftHanded(src, sk) {
			wd.damage2 += cardfixAttackerPreRenewal(src, target, wd.damage2, 3, wd.flag, wd.nk)
		}
	}

	// The target's cards apply to every attacker in pre-renewal.
	if target.Player != nil {
		wd.damage += cardfixTarget(src, target, wd.damage, rightElement, leftElement, 0, wd.flag, wd.nk)
		if isAttackLeftHanded(src, sk) {
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

// calcSkillBaseDamagePreRenewal is the default case of the pre-renewal
// battle_calc_skill_base_damage: battle_calc_base_damage for each hand,
// a skill's split among its targets, then the player's critical and
// non-critical damage bonuses.
func calcSkillBaseDamagePreRenewal(wd *damage, src, target *Status, critical bool, r Rand) {
	sd := src.Player
	bflag := 0
	if critical {
		bflag |= bdmgCrit
	}
	if sd != nil && isSkillUsingArrow(src, wd.sk) {
		switch sd.Weapon {
		case WeaponBow:
			bflag |= bdmgArrow
		case WeaponRevolver, WeaponRifle, WeaponGatling, WeaponShotgun, WeaponGrenade:
			// Critical hits with guns work like melee critical hits.
			if bflag&bdmgCrit == 0 {
				bflag |= bdmgArrow
			}
		}
		// Other weapons that use ammunition work like melee attacks.
	}
	wd.damage = calcBaseDamagePreRenewal(src, handRight, target.Size, bflag, r)
	if isAttackLeftHanded(src, wd.sk) {
		wd.damage2 = calcBaseDamagePreRenewal(src, handLeft, target.Size, bflag, r)
	}
	if wd.nk.Has(NKSplashSplit) && wd.miscflag > 0 {
		wd.damage /= int64(wd.miscflag)
	}

	if sd != nil {
		if bflag&bdmgCrit != 0 {
			if sd.CritAtkRate > 0 {
				wd.atkAddRate(src, &wd.damage, &wd.damage2, int64(sd.CritAtkRate))
			}
		} else if sd.NonCritAtkRate > 0 && (wd.sk == nil || !wd.sk.IgnoreNonCritBonus) {
			wd.atkAddRate(src, &wd.damage, &wd.damage2, int64(sd.NonCritAtkRate))
		}
		// Taekwon Mission (TK_POWER) needs party members on the map. It
		// is not ported.
	}
	if tsd := target.Player; tsd != nil && tsd.CritDefRate != 0 && wd.sk == nil && bflag&bdmgCrit != 0 {
		wd.atkAddRate(src, &wd.damage, &wd.damage2, int64(-tsd.CritDefRate))
	}
	// Acid Terror ignores DEF but loses the target's soft DEF instead.
	if wd.skillID() == skillAcidTerror {
		wd.atkAdd(src, &wd.damage, &wd.damage2, int64(-target.Def2))
	}
}

// calcBaseDamagePreRenewal is battle_calc_base_damage in pre-renewal, for
// a hand's weapon attack. A monster rolls between its atk and atk2. A
// player rolls between a DEX based minimum and the weapon's ATK, and the
// size penalty applies. Both then add base ATK. rAthena keeps atkmin and
// atkmax in uint32, so the port does too.
func calcBaseDamagePreRenewal(st *Status, h hand, size, flag int, r Rand) int64 {
	sd := st.Player
	wa := &st.RHW
	if h == handLeft {
		wa = &st.LHW
	}

	var atkmin, atkmax uint32
	if sd == nil {
		// Mental Charge (SC_CHANGE) makes monsters use their MATK. Status
		// changes are not ported.
		atkmin, atkmax = toUint32(wa.Atk), toUint32(wa.Atk2)
		atkmin = min(atkmin, atkmax)
	} else {
		atkmax = toUint32(wa.Atk)
		if flag&bdmgCrit == 0 || flag&bdmgArrow != 0 {
			atkmin = toUint32(st.Dex)
			// rAthena checks that the hand holds an item of type weapon.
			if sd.weapon(h) != WeaponFist {
				atkmin = atkmin * toUint32(80+wa.Wlv*20) / 100
			}
			atkmin = min(atkmin, atkmax)
			if flag&bdmgArrow != 0 {
				atkmin = atkmin * atkmax / 100
				atkmax = max(atkmax, atkmin)
			}
		}
	}

	// Maximize Power also takes the maximum. Status changes are not
	// ported.

	var dmg int64
	if flag&bdmgCrit == 0 {
		if atkmax > atkmin {
			dmg = int64(r.IntN(int(atkmax - atkmin)))
		}
		dmg += int64(atkmin)
	} else {
		dmg = int64(atkmax)
	}

	if sd != nil {
		// The ammunition adds 0 to its ATK - 1, or all of it on a
		// critical hit.
		if flag&bdmgArrow != 0 && sd.AmmoAtk != 0 {
			if flag&bdmgCrit != 0 {
				dmg += int64(sd.AmmoAtk)
			} else {
				dmg += int64(r.IntN(absInt(sd.AmmoAtk)))
			}
		}
		// The size penalty only applies to players.
		if !sd.NoSizeFix && flag&bdmgNoSize == 0 {
			dmg = dmg * int64(PreRenewalSizeFix(sd.weapon(h), size)) / 100
		}
	}
	// Elementals get bonuses from insignia status changes here.

	dmg += int64(st.BAtk)

	if sd != nil {
		dmg = addWeaponDamage(sd, dmg, h, r)
	}
	return dmg
}

// calcDefenseReductionPreRenewal is battle_calc_defense_reduction in
// pre-renewal: hard DEF reduces damage by a percent, at most 100, and
// soft DEF, partly random, is subtracted. A critical hit or a hand that
// ignores DEF keeps its damage. A hand that pierces DEF deals a percent
// equal to both DEF values instead.
func calcDefenseReductionPreRenewal(wd *damage, src, target *Status, critical bool, r Rand) {
	// status_get_def lowers DEF while the target casts, and caps it to a
	// signed char, the pre-renewal defType. Sample monsters do not cast.
	def1 := toInt8(int64(min(max(target.Def, -128), 127)))
	def2 := toInt16(int64(target.Def2))

	if sd := src.Player; sd != nil {
		// bIgnoreDefRaceRate and bIgnoreDefClassRate ignore a percent of
		// both DEF values.
		i := int64(sd.IgnoreDefRace[target.Race] + sd.IgnoreDefRace[RaceAll])
		i += int64(sd.IgnoreDefClass[target.Class] + sd.IgnoreDefClass[ClassAll])
		if i != 0 {
			i = min(i, 100)
			def1 = toInt8(def1 - def1*i/100)
			def2 = toInt16(def2 - def2*i/100)
		}
	}

	// The defense penalty for many attackers: vit_penalty_type is 1 and
	// vit_penalty_target is players. It counts the units that target the
	// player, which is 0 for a direct attack, below vit_penalty_count. So
	// only its floor applies: a player's soft DEF is at least 1.
	if target.Player != nil && def2 < 1 {
		def2 = 1
	}

	// vit_def is an int16 in rAthena.
	var vitDef int64
	if tsd := target.Player; tsd != nil {
		// Players: [VIT * 0.3] + rnd(0, [VIT^2 / 150] - [VIT * 0.3] - 1)
		// + [VIT * 0.5].
		vitDef = toInt16(3 * def2 / 10)
		vitDef = toInt16(vitDef + int64(rndValue(r, 0, int(max(0, def2*def2/150-3*def2/10-1)))))
		vitDef = toInt16(vitDef + def2/2)
		// Divine Protection, Ranger Main and Research Fire/Earth add soft
		// DEF against monsters.
		if src.Player == nil {
			if lv := target.skill(skillDivineProtection); lv > 0 && (isUndead(src.DefEle) || src.Race == RaceDemon) {
				vitDef = toInt16(vitDef + int64((float64(target.Level)/25.0+3.0)*float64(lv)+0.5))
			}
			if lv := target.skill(skillRangerMain); lv > 0 && (src.Race == RaceBrute || src.Race == RacePlayerDoram || src.Race == RaceFish || src.Race == RacePlant) {
				vitDef = toInt16(vitDef + int64(lv)*5)
			}
			if lv := target.skill(skillResearchFE); lv > 0 && (src.DefEle == Fire || src.DefEle == Earth) {
				vitDef = toInt16(vitDef + int64(lv)*10)
			}
		}
	} else {
		// Monsters: VIT + rnd(0, [VIT / 20]^2 - 1). Eska (SC_SKA) sets the
		// random part to 90. Status changes are not ported.
		vitDef = toInt16((def2 / 20) * (def2 / 20))
		if vitDef > 0 {
			vitDef = toInt16(def2 + int64(r.IntN(int(vitDef))))
		} else {
			vitDef = def2
		}
	}

	if weaponDefenseType != 0 {
		vitDef = toInt16(vitDef + def1*weaponDefenseType)
		def1 = 0
	}

	def1 = min(def1, 100)

	// rate is each hand's percent of damage: 100 when it ignores DEF,
	// both DEF values when it pierces DEF, and 100 - DEF otherwise.
	rate := func(h hand) int64 {
		switch {
		case attackIgnoresDefPreRenewal(wd, src, target, h, critical):
			return 100
		case isAttackPiercingPreRenewal(wd, src, target, h, critical):
			return def1 + vitDef
		}
		return 100 - def1
	}
	// soft is each hand's soft DEF reduction.
	soft := func(h hand) int64 {
		if attackIgnoresDefPreRenewal(wd, src, target, h, critical) || isAttackPiercingPreRenewal(wd, src, target, h, critical) {
			return 0
		}
		return -vitDef
	}
	wd.atkRate2(src, &wd.damage, &wd.damage2, rate(handRight), rate(handLeft))
	wd.basedamage = wd.basedamage * (100 - def1) / 100
	wd.atkAdd2(src, &wd.damage, &wd.damage2, soft(handRight), soft(handLeft))
	wd.basedamage -= vitDef
}

// attackIgnoresDefPreRenewal is attack_ignores_def in pre-renewal: a
// critical hit always ignores DEF.
func attackIgnoresDefPreRenewal(wd *damage, src, target *Status, h hand, critical bool) bool {
	return critical || attackIgnoresDef(wd, src, target, h)
}

// isAttackPiercingPreRenewal is is_attack_piercing in pre-renewal: a
// critical hit never pierces DEF.
func isAttackPiercingPreRenewal(wd *damage, src, target *Status, h hand, critical bool) bool {
	return !critical && isAttackPiercing(wd, src, target, h)
}

// calcAttackPostDefensePreRenewal is battle_calc_attack_post_defense in
// pre-renewal: the refine bonus is added after DEF, damage is raised to
// at least 1, then masteries are added.
func calcAttackPostDefensePreRenewal(wd *damage, src, target *Status) {
	if src.Player != nil {
		if skillStacksMasteries(wd.skillID()) {
			wd.atkAdd2(src, &wd.damage, &wd.damage2, int64(src.RHW.Atk2), int64(src.LHW.Atk2))
		}
		wd.basedamage += int64(src.RHW.Atk2)
	}
	// Damage can be negative after DEF. The refine bonus counts against
	// that value before the minimum of 1.
	minDamage(wd, src, 1)

	calcAttackMasteriesPreRenewal(wd, src, target)

	// Aura Blade adds damage here. Status changes are not ported.
	minDamage(wd, src, 1)
}

// calcAttackMasteriesPreRenewal is battle_calc_attack_masteries in
// pre-renewal: mastery ATK is added to each hand's damage, unless the
// skill takes none.
//
// Camouflage and other status changes also add damage here. They are not
// ported.
func calcAttackMasteriesPreRenewal(wd *damage, src, target *Status) {
	if src.Player == nil {
		return
	}
	// The base damage of a normal attack always gets the masteries.
	wd.basedamage += addMastery(src, target, handRight, true)
	if !skillStacksMasteries(wd.skillID()) {
		return
	}
	wd.damage += addMastery(src, target, handRight, true)
	if isAttackLeftHanded(src, wd.sk) {
		wd.damage2 += addMastery(src, target, handLeft, true)
	}
}

// calcElementDamagePreRenewal is battle_calc_element_damage in
// pre-renewal: the element table applies to the whole damage of each
// hand. Envenom adds its damage and applies the table again, and skills
// forced to neutral apply it again with neutral.
//
// Star Crumbs and Spirit Spheres add damage after the element table. They
// are not ported. Of the status bonuses here, SC_WATK_ELEMENT (Magnum
// Break's fire bonus) is ported and EDP is not.
func calcElementDamagePreRenewal(wd *damage, src, target *Status, rightElement, leftElement int) {
	if !wd.nk.Has(NKIgnoreElement) && (wd.damage > 0 || wd.damage2 > 0) {
		// Normal attacks by monsters are non-elemental and deal 100%
		// against every defense element (attack_attr_none). rAthena
		// returns here, so the damage is not raised to 0 either.
		if src.Player == nil && wd.sk == nil && monsterAttackAttrNone {
			return
		}
		wd.damage = AttrFixPreRenewal(wd.damage, rightElement, target.DefEle, target.EleLv, 1)
		if isAttackLeftHanded(src, wd.sk) {
			wd.damage2 = AttrFixPreRenewal(wd.damage2, leftElement, target.DefEle, target.EleLv, 1)
		}
		switch wd.skillID() {
		case skillCartRevolution, skillMagicCrasher:
			wd.damage = AttrFixPreRenewal(wd.damage, Neutral, target.DefEle, target.EleLv, 1)
			if isAttackLeftHanded(src, wd.sk) {
				wd.damage2 = AttrFixPreRenewal(wd.damage2, Neutral, target.DefEle, target.EleLv, 1)
			}
		}
	}
	if skillStacksMasteries(wd.skillID()) && wd.skillID() == skillEnvenom {
		// Envenom adds 15 per level, and the element table applies to the
		// base damage and again to the final damage.
		wd.atkAdd(src, &wd.damage, &wd.damage2, int64(15*wd.skillLv()))
		wd.damage = AttrFixPreRenewal(wd.damage, rightElement, target.DefEle, target.EleLv, 1)
	}
	// SC_WATK_ELEMENT adds a percent of the damage a normal attack of its
	// element would deal. Only players get it, and not on skills that
	// ignore the element or attack cards, or when the damage is 0 or
	// less. Star Crumbs and Spirit Spheres would add to the base damage
	// first; they are not ported.
	if c := src.Changes; c != nil && src.Player != nil && c.WatkElementRate != 0 &&
		!wd.nk.Has(NKIgnoreElement) && !wd.nk.Has(NKIgnoreAtkCard) && (wd.damage > 0 || wd.damage2 > 0) {
		wd.basedamage = AttrFixPreRenewal(wd.basedamage, c.WatkElement, target.DefEle, target.EleLv, 1)
		wd.damage += wd.basedamage * int64(c.WatkElementRate) / 100
	}
	if !attrRecover {
		minDamage(wd, src, 0)
	}
}

// cardfixAttackerPreRenewal is cardfixAttacker in pre-renewal. Advanced
// Katar Mastery works as a card bonus on melee attacks, and bShortAtkRate
// and bLongAtkRate count here too. Both only change the right hand's rate.
func cardfixAttackerPreRenewal(src, target *Status, dmg int64, left, flag int, nk NK) int64 {
	if dmg == 0 || nk.Has(NKIgnoreAtkCard) {
		return 0
	}
	sd := src.Player
	cardfix := cardfixHands(sd, target, flag, nk)
	if lv := src.skill(skillAdvKatarMastery); !sd.ArrowAttack && sd.Weapon == WeaponKatar && lv > 0 {
		cardfix = cardfix * int64(100+10+2*lv) / 100
	}
	cardfix, cardfixLeft := cardfixAddDamage(sd, target, cardfix, left)
	if flag&bfShort != 0 {
		cardfix = cardfix * int64(100+sd.ShortAtkRate) / 100
	}
	if flag&bfLong != 0 {
		cardfix = cardfix * int64(100+sd.LongAtkRate) / 100
	}
	return applyCardfix(dmg, chooseCardfix(cardfix, cardfixLeft, left))
}

// atkRate2 is rAthena's ATK_RATE2 macro: each hand's damage times its own
// percent.
func (wd *damage) atkRate2(src *Status, v, v2 *int64, pct, pct2 int64) {
	*v = *v * pct / 100
	if isAttackLeftHanded(src, wd.sk) {
		*v2 = *v2 * pct2 / 100
	}
}

// atkAdd2 is rAthena's ATK_ADD2 macro: adds each hand's own amount.
func (wd *damage) atkAdd2(src *Status, v, v2 *int64, n, n2 int64) {
	*v += n
	if isAttackLeftHanded(src, wd.sk) {
		*v2 += n2
	}
}

// toUint32 converts as C does when it stores an int in a uint32.
func toUint32(v int) uint32 {
	return uint32(v) // #nosec G115 -- rAthena stores these values in a uint32.
}

// toInt16 converts as C does when it stores a value in an int16.
func toInt16(v int64) int64 {
	return int64(int16(v)) // #nosec G115 -- rAthena stores these values in an int16.
}

// toInt8 converts as C does when it stores a value in a signed char.
func toInt8(v int64) int64 {
	return int64(int8(v)) // #nosec G115 -- rAthena stores pre-renewal DEF in a signed char.
}

// absInt returns the absolute value of n. C's rnd() % n with a negative n
// gives a result between 0 and |n| - 1 because rnd() is not negative.
func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
