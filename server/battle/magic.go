package battle

// Magic and misc attacks, and the skill ratio of weapon skills, as
// battle_calc_magic_attack, battle_calc_misc_attack and
// battle_calc_attack_skill_ratio in renewal.

// Skill IDs that only these functions read.
const (
	skillFreezingTrap = 121 // HT_FREEZINGTRAP
	skillPhantasmic   = 380 // HT_PHANTASMIC
	skillGroundDrift  = 525 // GS_GROUNDDRIFT
	skillResearchTrap = 2248
)

// BaseDamager is implemented by a SkillImpl whose magic or misc damage
// does not start from MATK, such as Heal's damage to the undead, which is
// skill_calc_heal with heal false.
type BaseDamager interface {
	// BaseDamage returns the skill's base damage.
	BaseDamage(a *SkillContext, r Rand) int64
}

// atkPercent is battle_get_atkpercent: the ATK percent of status changes.
// A player's skill that takes no masteries takes no ATK percent either.
func atkPercent(src *Status, sk *Skill) int64 {
	if sk != nil && src.Player != nil && !skillStacksMasteries(sk.ID) {
		return 100
	}
	return src.atkPercent()
}

// skillRatio is battle_calc_attack_skill_ratio: in renewal it starts from
// the ATK percent of status changes, then the skill adds its own ratio.
// Overthrust, Berserk, Poison React and the other statuses it reads are
// not ported.
func skillRatio(wd *damage, src, target *Status, renewal bool) int {
	ratio := 100
	if wd.skillID() != skillSacrifice && renewal {
		ratio = int(atkPercent(src, wd.sk))
	}
	if wd.sk != nil && wd.sk.Impl != nil {
		wd.sk.Impl.SkillRatio(wd.context(src, target), &ratio)
	}
	return ratio
}

// skillConstantAddition is battle_calc_skill_constant_addition in
// renewal: damage that a skill adds after its ratio.
func skillConstantAddition(wd *damage, src, _ *Status) int64 {
	switch wd.skillID() {
	case skillExtremityFist:
		return int64(250 + 150*wd.skillLv())
	case skillFreezingTrap:
		if src.Player != nil {
			return int64(40 * src.skill(skillResearchTrap))
		}
	}
	return 0
}

// magicElement is battle_get_magic_element: the skill's element, the
// weapon's for ElementWeapon, or a random one.
func magicElement(src *Status, sk *Skill, r Rand) int {
	switch sk.Element {
	case ElementWeapon:
		return src.RHW.Ele
	case ElementEndowed:
		// status_get_attack_sc_element: no endowment is ported.
		return Neutral
	case ElementRandom:
		return r.IntN(AllElements)
	}
	return sk.Element
}

// miscElement is battle_get_misc_element: weapon and endowed elements are
// neutral for misc attacks.
func miscElement(sk *Skill, r Rand) int {
	switch sk.Element {
	case ElementWeapon, ElementEndowed:
		return Neutral
	case ElementRandom:
		return r.IntN(AllElements)
	}
	return sk.Element
}

// calcMagicAttack is battle_calc_magic_attack, in pre-renewal when pre is
// set and in renewal otherwise.
func calcMagicAttack(src, target *Status, sk *Skill, pre bool, r Rand) damage {
	ad := damage{damage: 1, div: sk.Hits, blewcount: sk.Knockback, flag: bfMagic | bfSkill, miscflag: sk.Flag,
		dmgLv: atkDef, sk: sk, nk: sk.NK, pre: pre}
	imdef := ad.nk.Has(NKIgnoreDefense)
	ele := magicElement(src, sk, r)
	if sd := src.Player; sd != nil {
		// Magic never uses ammunition.
		sd.ArrowAttack = false
	}
	ad.flag |= rangeType(src, sk)

	if !isInfiniteDefense(target, ad.flag) {
		ratio := 100
		hasRatio := false
		ad.damage = 0
		if b, ok := sk.Impl.(BaseDamager); ok {
			// Heal and the other skills with a base damage of their own.
			ad.damage = b.BaseDamage(ad.context(src, target), r)
		} else {
			if src.MAtkMax > src.MAtkMin {
				ad.damage += int64(src.MAtkMin + r.IntN(src.MAtkMax-src.MAtkMin))
			} else {
				ad.damage += int64(src.MAtkMin)
			}
			// Divide MATK among the targets of a skill that splits it.
			if ad.nk.Has(NKSplashSplit) && ad.miscflag > 0 {
				ad.damage /= int64(ad.miscflag)
			}
			if sk.Impl != nil {
				sk.Impl.SkillRatio(ad.context(src, target), &ratio)
			}
			hasRatio = true
			if pre {
				ad.damage = ad.damage * int64(ratio) / 100
			}
		}

		if !pre {
			ad.damage += cardfixMagic(src, target, ad.damage, ele, ad.flag, ad.nk, false)
			if sd := src.Player; sd != nil && sd.Weapon == Weapon2HStaff {
				if lv := src.skill(skillTwoHandStaff); lv > 0 {
					ad.damage += ad.damage * int64(lv) / 100
				}
			}
			if hasRatio {
				if src.Player != nil && src.SMatk > 0 {
					ad.damage += ad.damage * int64(src.SMatk) / 100
				}
				ad.damage = ad.damage * int64(ratio) / 100
			}
		} else if sd := src.Player; sd != nil {
			// pc_skillatk_bonus, in pre-renewal before MDEF.
			if i := int64(sd.SkillAtk[sk.ID]); i != 0 {
				ad.damage += ad.damage * i / 100
			}
		}

		if sd := src.Player; sd != nil && !imdef {
			if handBits(sd.IgnoreMDefEle, sd.IgnoreMDefRace, sd.IgnoreMDefClass, target) {
				imdef = true
			}
		}
		if tsd := target.Player; tsd != nil {
			if i := int64(tsd.SubSkill[sk.ID]); i != 0 {
				ad.damage -= ad.damage * i / 100
			}
		}

		// MRes reduces magic damage by a percent before MDEF.
		if !pre && ad.damage != 0 && target.MRes > 0 {
			mres := float64(target.MRes)
			part := float64(float32(mres / (mres + 400.0)))
			ad.damage -= int64(part * 80.0 / 100.0 * float64(ad.damage))
		}

		if !imdef {
			mdef := int64(target.MDef)
			mdef2 := int64(target.MDef2)
			i := 0
			if sd := src.Player; sd != nil {
				i += sd.IgnoreMDefRaceRate[target.Race] + sd.IgnoreMDefRaceRate[RaceAll]
				i += sd.IgnoreMDefClassRate[target.Class] + sd.IgnoreMDefClassRate[ClassAll]
			}
			i = min(max(i, 0), 100)
			if pre {
				// Pre-renewal MDEF is a signed char, as status_get_mdef
				// keeps it, and reduces damage by a percent
				// (magic_defense_type 0).
				mdef = toInt8(int64(min(max(target.MDef, -128), 127)))
				if i > 0 {
					mdef -= mdef * int64(i) / 100
				}
				ad.damage = ad.damage*(100-mdef)/100 - mdef2
			} else {
				if i > 0 {
					mdef = int64(float32(float64(mdef) - float64(mdef)*float64(i)/100.0))
				}
				// Negative equipment MDEF counts as 0.
				mdef = max(mdef, 0)
				ad.damage = ad.damage*(1000+mdef)/(1000+mdef*10) - mdef2
			}
		}

		// pc_skillatk_bonus, in renewal after MDEF.
		if sd := src.Player; sd != nil && !pre {
			if i := sd.SkillAtk[sk.ID]; i != 0 {
				ad.damage = int64(float64(ad.damage) + float64(ad.damage)*float64(i)/100.0)
			}
		}

		if ad.damage < 1 {
			ad.damage = 1
		}
		// The elemental spirits that add to bolts are not ported.

		if !ad.nk.Has(NKIgnoreElement) {
			if pre {
				ad.damage = AttrFixPreRenewal(ad.damage, ele, target.DefEle, target.EleLv, 0)
			} else {
				ad.damage = AttrFix(ad.damage, ele, target.DefEle, target.EleLv, 0)
			}
		}
		if pre {
			// Pre-renewal applies a player target's magic cards after the
			// element table.
			ad.damage += cardfixMagic(src, target, ad.damage, ele, ad.flag, ad.nk, true)
		}
	}
	// Against a target that takes 1 damage from each hit, damage is
	// still 1 here.

	applyDivFix(&ad)
	ad.damage = calcDamage(&ad, src, target, ad.damage)
	absorbDamage(&ad, target)
	return ad
}

// calcMiscAttack is battle_calc_misc_attack, in pre-renewal when pre is
// set and in renewal otherwise, for the misc skills that are ported: Throw
// Stone, and the skills whose SkillImpl gives a base damage.
func calcMiscAttack(src, target *Status, sk *Skill, pre bool, r Rand) damage {
	md := damage{div: sk.Hits, blewcount: sk.Knockback, dmgLv: atkDef, flag: bfMisc | bfSkill, miscflag: sk.Flag,
		sk: sk, nk: sk.NK, pre: pre}
	if sd := src.Player; sd != nil {
		sd.ArrowAttack = false
	}
	ele := miscElement(sk, r)
	md.flag |= rangeType(src, sk)

	switch sk.ID {
	case skillThrowStone:
		if src.Player != nil {
			md.damage = 50
		} else {
			md.damage = 30
		}
		md.flag |= bfWeapon
	default:
		if b, ok := sk.Impl.(BaseDamager); ok {
			md.damage = b.BaseDamage(md.context(src, target), r)
		}
	}

	if md.nk.Has(NKSplashSplit) && md.miscflag > 0 {
		md.damage /= int64(md.miscflag)
	}

	if !md.nk.Has(NKIgnoreFlee) {
		hit := false
		if tc := target.Changes; tc != nil && tc.Opt1 != 0 && tc.Opt1 != opt1StoneWait && tc.Opt1 != opt1Burning {
			hit = true
		} else {
			hitrate := src.Hit - target.Flee
			if pre {
				hitrate += 80
			} else if src.Player != nil {
				// Vulture's Eye adds hit that the status does not show.
				hitrate += src.skill(skillVultureEye)
			}
			hitrate = min(max(hitrate, minHitrate), maxHitrate)
			if r.IntN(100) < hitrate {
				hit = true
			}
		}
		if !hit {
			md.damage = 0
			md.dmgLv = atkFlee
		}
	}

	md.damage += cardfixMisc(src, target, md.damage, ele, md.flag, md.nk)

	if sd := src.Player; sd != nil {
		if i := int64(sd.SkillAtk[sk.ID]); i != 0 {
			md.damage += md.damage * i / 100
		}
	}
	if tsd := target.Player; tsd != nil {
		if i := int64(tsd.SubSkill[sk.ID]); i != 0 {
			md.damage -= md.damage * i / 100
		}
	}

	if !md.nk.Has(NKIgnoreElement) {
		if pre {
			md.damage = AttrFixPreRenewal(md.damage, ele, target.DefEle, target.EleLv, 0)
		} else {
			md.damage = AttrFix(md.damage, ele, target.DefEle, target.EleLv, 0)
		}
	}

	// Plant damage.
	if md.damage < 0 {
		md.damage = 0
	} else if md.damage != 0 && isInfiniteDefense(target, md.flag) {
		md.damage = 1
	}

	applyDivFix(&md)
	md.damage = calcDamage(&md, src, target, md.damage)
	absorbDamage(&md, target)
	return md
}

// cardfixMagic is battle_calc_cardfix for BF_MAGIC: in renewal, the
// attacker's magic bonuses, each rounded down in rAthena's order, then a
// player target's reductions. Pre-renewal calls it once for the target's
// reductions only (targetOnly), with the near and long reductions too.
// It returns the damage to add.
func cardfixMagic(src, target *Status, dmg int64, ele, flag int, nk NK, targetOnly bool) int64 {
	if dmg == 0 {
		return 0
	}
	orig := dmg
	re := func(fix int) {
		dmg -= dmg * int64(100-max(0, 100+fix)) / 100
	}
	if sd := src.Player; sd != nil && !nk.Has(NKIgnoreAtkCard) && !targetOnly {
		m := &sd.Magic
		re(m.AddSize[target.Size] + m.AddSize[SizeAll])
		race2 := 0
		for _, g := range target.Race2 {
			race2 += m.AddRace2[g]
		}
		race2 += m.AddDamage[target.ID]
		re(race2)
		if !nk.Has(NKIgnoreElement) {
			re(m.AddEle[target.DefEle] + m.AddEle[AllElements])
		}
		// Statuses of the target that change its element are not ported.
		if !nk.Has(NKIgnoreElement) {
			re(m.AtkEle[ele] + m.AtkEle[AllElements])
		}
		re(m.AddRace[target.Race] + m.AddRace[RaceAll])
		re(m.AddClass[target.Class] + m.AddClass[ClassAll])
	}
	if tsd := target.Player; tsd != nil && !nk.Has(NKIgnoreDefCard) {
		cardfix := int64(1000)
		if !nk.Has(NKIgnoreElement) {
			fix := tsd.SubEle[ele] + tsd.SubEle[AllElements] + sumFlagged(tsd.SubEle2, ele, AllElements, flag)
			fix += tsd.MagicSubDefEle[src.DefEle] + tsd.MagicSubDefEle[AllElements]
			cardfix = cardfix * int64(100-fix) / 100
		}
		cardfix = cardfix * int64(100-tsd.SubSize[src.Size]-tsd.SubSize[SizeAll]) / 100
		cardfix = cardfix * int64(100-tsd.MagicSubSize[src.Size]-tsd.MagicSubSize[SizeAll]) / 100
		race2 := 0
		for _, g := range src.Race2 {
			race2 += tsd.SubRace2[g]
		}
		cardfix = cardfix * int64(100-race2) / 100
		race := tsd.SubRace[src.Race] + tsd.SubRace[RaceAll] + sumFlagged(tsd.SubRace3, src.Race, RaceAll, flag)
		cardfix = cardfix * int64(100-race) / 100
		cardfix = cardfix * int64(100-tsd.SubClass[src.Class]-tsd.SubClass[ClassAll]) / 100
		if v, ok := tsd.AddMDef[src.ID]; ok {
			cardfix = cardfix * int64(100-v) / 100
		}
		if targetOnly {
			// Pre-renewal: ranged defense counts against magic too.
			if flag&bfShort != 0 {
				cardfix = cardfix * int64(100-tsd.NearAtkDef) / 100
			} else if !nk.Has(NKIgnoreLongCard) {
				cardfix = cardfix * int64(100-tsd.LongAtkDef) / 100
			}
		}
		cardfix = cardfix * int64(100-tsd.MagicDefRate) / 100
		dmg += applyCardfix(dmg, cardfix)
	}
	return dmg - orig
}

// cardfixMisc is battle_calc_cardfix for BF_MISC: only a player target's
// reductions apply.
func cardfixMisc(src, target *Status, dmg int64, ele, flag int, nk NK) int64 {
	tsd := target.Player
	if dmg == 0 || tsd == nil || nk.Has(NKIgnoreDefCard) {
		return 0
	}
	cardfix := int64(1000)
	if !nk.Has(NKIgnoreElement) {
		fix := tsd.SubEle[ele] + tsd.SubEle[AllElements]
		for _, e := range tsd.SubEle2 {
			if e.Key == ele && matchesFlag(e.Flag, flag) {
				fix += e.Rate
			}
		}
		fix += tsd.SubDefEle[src.DefEle] + tsd.SubDefEle[AllElements]
		cardfix = cardfix * int64(100-fix) / 100
	}
	race := tsd.SubRace[src.Race] + tsd.SubRace[RaceAll] + sumFlagged(tsd.SubRace3, src.Race, RaceAll, flag)
	cardfix = cardfix * int64(100-race) / 100
	cardfix = cardfix * int64(100-tsd.SubSize[src.Size]-tsd.SubSize[SizeAll]) / 100
	race2 := 0
	for _, g := range src.Race2 {
		race2 += tsd.SubRace2[g]
	}
	cardfix = cardfix * int64(100-race2) / 100
	cardfix = cardfix * int64(100-tsd.SubClass[src.Class]-tsd.SubClass[ClassAll]) / 100
	cardfix = cardfix * int64(100-tsd.MiscDefRate) / 100
	if flag&bfShort != 0 {
		cardfix = cardfix * int64(100-tsd.NearAtkDef) / 100
	} else if !nk.Has(NKIgnoreLongCard) {
		cardfix = cardfix * int64(100-tsd.LongAtkDef) / 100
	}
	return applyCardfix(dmg, cardfix)
}
