package battle

// Skill attacks: battle_calc_attack with a skill, for BF_WEAPON,
// BF_MAGIC and BF_MISC. The skill's data comes from skill_db, and its
// own formulas from a SkillImpl, as rAthena's SkillImpl methods that
// battle.cpp calls.

// NK is a set of rAthena's e_skill_nk damage flags, as bits.
type NK uint16

// Damage flags of skill_db.yml, as e_skill_nk.
const (
	NKNoDamage NK = 1 << iota
	NKSplash
	NKSplashSplit
	NKIgnoreAtkCard
	NKIgnoreElement
	NKIgnoreDefense
	NKIgnoreFlee
	NKIgnoreDefCard
	NKCritical
	NKIgnoreLongCard
	NKSimpleDefense
)

// Has reports whether the set holds a flag.
func (n NK) Has(f NK) bool { return n&f != 0 }

// Skill elements beyond the ten elements, as rAthena's ELE_WEAPON,
// ELE_ENDOWED and ELE_RANDOM.
const (
	ElementWeapon  = 12
	ElementEndowed = 13
	ElementRandom  = 14
)

// Attack types of a skill, as rAthena's BF_WEAPON, BF_MAGIC and BF_MISC.
const (
	TypeWeapon = bfWeapon
	TypeMagic  = bfMagic
	TypeMisc   = bfMisc
)

// Battle flags that callers read from Result.Flag, as e_battle_flag.
const (
	FlagWeapon = bfWeapon
	FlagMagic  = bfMagic
	FlagMisc   = bfMisc
	FlagShort  = bfShort
	FlagSkill  = bfSkill
	FlagNormal = bfNormal
	// FlagRangeMask, FlagWeaponMask and FlagSkillMask are BF_RANGEMASK,
	// BF_WEAPONMASK and BF_SKILLMASK.
	FlagRangeMask  = 0x00f0
	FlagWeaponMask = 0x000f
	FlagSkillMask  = 0x0f00
)

// Skill is a skill attack: rAthena's skill_id and skill_lv, with the
// skill_db values that damage reads at that level.
type Skill struct {
	ID, Level int
	// Hits is skill_get_num: the number of hits. A negative count shows
	// that many hits and divides the damage among them (DAMAGE_DIV_FIX).
	Hits int
	// Element is skill_get_ele: an element, or ElementWeapon,
	// ElementEndowed or ElementRandom.
	Element int
	// NK are the skill's damage flags.
	NK NK
	// Ground is set for a ground skill (INF_GROUND_SKILL). Its hits show
	// no attack motion.
	Ground bool
	// Range is skill_get_range2 for the caster. Below 4 the attack is
	// melee (battle_range_type).
	Range int
	// Distance is the distance from the caster to the target. A caster
	// that is not a player is melee within 3 cells, as
	// skillrange_by_distance.
	Distance int
	// Knockback is skill_get_blewcount.
	Knockback int
	// Ammo is set when the skill uses ammunition (skill_get_ammotype).
	Ammo bool
	// Trap is INF2_ISTRAP: traps always count as melee.
	Trap bool
	// IgnoreNonCritBonus is INF2_IGNORENONCRITATKBONUS.
	IgnoreNonCritBonus bool
	// Flag is wflag or mflag: for a skill that splits its damage, the
	// number of targets, and other skill specific values, such as the
	// ring of Magnum Break.
	Flag int
	// Impl holds the skill's own formulas. It may be nil.
	Impl SkillImpl
	// Self is set when the target is the caster itself (src == target),
	// as for Grand Cross, whose area hits its caster.
	Self bool
	// Renewal is set for an attack in renewal: SkillAttack sets it and
	// SkillAttackPreRenewal clears it. Skills whose range type differs by
	// mode read it.
	Renewal bool
}

// SkillImpl is what a skill changes in a damage calculation, as the
// methods of rAthena's SkillImpl that battle.cpp calls. Each method may
// do nothing.
type SkillImpl interface {
	// SkillRatio is calculateSkillRatio: it changes the skill's damage
	// percent, which starts at 100 (or the ATK percent in renewal).
	SkillRatio(a *SkillContext, ratio *int)
	// HitRate is modifyHitRate: it changes the chance to hit, in
	// percent, before the cap.
	HitRate(a *SkillContext, hit *int)
}

// DamageData is the part of rAthena's Damage that a skill may change
// before the calculation: the hit count (div_), the knockback
// (blewcount) and the battle flags.
type DamageData struct {
	Div, Knockback, Flag int
}

// DamageDataModifier is implemented by a SkillImpl that changes the hit
// count, knockback or battle flags of its attack, as
// SkillImpl::modifyDamageData. battle_calc_weapon_attack calls it after
// initialize_weapon_data.
type DamageDataModifier interface {
	ModifyDamageData(a *SkillContext, d *DamageData)
}

// ElementModifier is implemented by a SkillImpl that changes the element
// of its weapon attack, as SkillImpl::modifyElement, which
// battle_get_weapon_element calls last.
type ElementModifier interface {
	ModifyElement(a *SkillContext, element *int)
}

// modifyDamageData runs the skill's DamageDataModifier on a weapon
// attack.
func modifyDamageData(wd *damage, src, target *Status) {
	if wd.sk == nil {
		return
	}
	m, ok := wd.sk.Impl.(DamageDataModifier)
	if !ok {
		return
	}
	d := DamageData{Div: wd.div, Knockback: wd.blewcount, Flag: wd.flag}
	m.ModifyDamageData(wd.context(src, target), &d)
	wd.div, wd.blewcount, wd.flag = d.Div, d.Knockback, d.Flag
}

// modifyElement runs the skill's ElementModifier on a hand's element.
func modifyElement(wd *damage, src, target *Status, element int) int {
	if wd.sk == nil {
		return element
	}
	if m, ok := wd.sk.Impl.(ElementModifier); ok {
		m.ModifyElement(wd.context(src, target), &element)
	}
	return element
}

// SkillContext is what a SkillImpl reads of an attack.
type SkillContext struct {
	Src, Target *Status
	Skill       *Skill
	// Flag is the attack's wflag or mflag (the Damage's miscflag).
	Flag int
	// Renewal is set in renewal mode.
	Renewal bool
}

// Level is the skill level.
func (a *SkillContext) Level() int { return a.Skill.Level }

// IsUndead is battle_check_undead with undead_detect_type 0.
func IsUndead(race, element int) bool { return isUndead(element) }

// SkillAttack is battle_calc_attack for a skill in renewal: the attack
// type is BF_WEAPON, BF_MAGIC or BF_MISC.
func SkillAttack(typ int, src, target *Status, sk *Skill, r Rand) Result {
	sk.Renewal = true
	var d damage
	switch typ {
	case TypeWeapon:
		d = calcWeaponAttack(src, target, sk, r)
	case TypeMagic:
		d = calcMagicAttack(src, target, sk, false, r)
	case TypeMisc:
		d = calcMiscAttack(src, target, sk, false, r)
	default:
		return Result{Kind: Miss}
	}
	return finishAttack(&d, src)
}

// finishAttack is the end of battle_calc_attack: an attack that deals no
// damage misses.
func finishAttack(d *damage, src *Status) Result {
	if d.damage+d.damage2 < 1 {
		// A hit that deals no damage counts as a miss.
		if d.dmgLv == atkDef {
			d.dmgLv = atkMiss
		}
		if src.Player != nil {
			d.div = 1
		}
		// With a weapon in the left hand only, rAthena does not raise the
		// right hand's damage to 1, so it can stay negative after DEF.
		// rAthena keeps the negative total, and status_damage would heal
		// the target by it. The attack reports no damage instead.
		if d.damage+d.damage2 < 0 {
			d.damage, d.damage2 = 0, 0
		}
	} else {
		// Some damage lands even when the attack was dodged.
		d.dmgLv = atkDef
	}
	return d.result()
}

// skillID is the attack's skill ID, or 0 for a normal attack.
func (wd *damage) skillID() int {
	if wd.sk == nil {
		return 0
	}
	return wd.sk.ID
}

// skillLv is the attack's skill level, or 0.
func (wd *damage) skillLv() int {
	if wd.sk == nil {
		return 0
	}
	return wd.sk.Level
}

// rangeType is battle_range_type: whether a skill attack is melee
// (BF_SHORT) or ranged (BF_LONG). skillrange_by_distance is 0, so it
// follows the skill's range.
func rangeType(src *Status, sk *Skill) int {
	if sk.Trap {
		return bfShort
	}
	switch sk.ID {
	case skillArrowShower, skillBomb:
		// Monsters' Arrow Shower and Bomb are melee.
		if src.Player == nil {
			return bfShort
		}
	case skillBrandishSpear:
		// Renewal's Brandish Spear is ranged.
		if sk.Renewal {
			return bfLong
		}
	}
	// skillrange_by_distance is every unit but players: they are melee
	// within 3 cells of the target.
	if src.Player == nil {
		if sk.Distance <= 3 {
			return bfShort
		}
		return bfLong
	}
	if sk.Range < 4 {
		return bfShort
	}
	return bfLong
}

// skillStacksMasteries is battle_skill_stacks_masteries_vvs: whether
// masteries, refine bonus, Star Crumbs and Spirit Spheres add to a skill.
// No first class skill is left out.
func skillStacksMasteries(id int) bool {
	switch id {
	case skillShieldChain, skillShieldBoomerang, skillAcidTerror, skillInvestigate, skillExtremityFist, skillSacrifice,
		skillNPCDragonBreath:
		return false
	}
	return true
}

// Skill IDs that battle reads, from rAthena's skill_db.yml.
const (
	skillBash            = 5   // SM_BASH
	skillMagnumBreak     = 7   // SM_MAGNUM
	skillNapalmBeat      = 11  // MG_NAPALMBEAT
	skillSoulStrike      = 13  // MG_SOULSTRIKE
	skillFireWall        = 18  // MG_FIREWALL
	skillHeal            = 28  // AL_HEAL
	skillMammonite       = 42  // MC_MAMMONITE
	skillArrowShower     = 47  // AC_SHOWER
	skillEnvenom         = 52  // TF_POISON
	skillShieldBoomerang = 251 // CR_SHIELDBOOMERANG
	skillAcidTerror      = 230 // AM_ACIDTERROR
	skillBomb            = 229 // AM_DEMONSTRATION
	skillInvestigate     = 266 // MO_INVESTIGATE
	skillExtremityFist   = 271 // MO_EXTREMITYFIST
	skillTripleAttack    = 263 // MO_TRIPLEATTACK
	skillFingerOffensive = 267 // MO_FINGEROFFENSIVE
	skillShieldChain     = 480 // PA_SHIELDCHAIN
	skillSacrifice       = 368 // PA_SACRIFICE
	skillCartRevolution  = 153 // MC_CARTREVOLUTION
	skillThrowStone      = 152 // TF_THROWSTONE
	skillMagicCrasher    = 365 // HW_MAGICCRASHER
	skillMeteorAssault   = 406 // ASC_METEORASSAULT
	skillBrandishSpear   = 57  // KN_BRANDISHSPEAR
	skillAutoCounter     = 61  // KN_AUTOCOUNTER
	skillSpiralPierce    = 397 // LK_SPIRALPIERCE
	skillBlitzBeat       = 129 // HT_BLITZBEAT
	skillFalconAssault   = 381 // SN_FALCONASSAULT
	skillSharpShooting   = 382 // SN_SHARPSHOOTING
	skillGrandCross      = 254 // CR_GRANDCROSS
	skillPressure        = 367 // PA_PRESSURE
	skillGospel          = 369 // PA_GOSPEL
)
