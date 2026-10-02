package battle

import "math"

// Player is what a player attacker has that a monster does not, from
// rAthena's map_session_data. Set it on a Status to make Attack and
// AttackPreRenewal use the player formula.
type Player struct {
	// Weapon is the weapon type attacks use, as sd->status.weapon. It is
	// the right hand's type, or the left hand's when the right hand is
	// empty. With a weapon in each hand, rAthena uses its W_DOUBLE_* types
	// (25 and up), which count as STR weapons here.
	Weapon int
	// RightWeapon and LeftWeapon are each hand's weapon type, as
	// weapontype1 and weapontype2. A two-handed weapon sets both. An empty
	// hand or a shield is WeaponFist.
	RightWeapon, LeftWeapon int
	// Skills maps a learned skill ID to its level, as pc_checkskill reads.
	Skills map[int]int
	// Granted marks the skills whose level an item grants, as
	// pc_checkskill_flag's SKILL_FLAG_TEMPORARY and replaced flags.
	Granted map[int]bool
	// ArrowAttack is sd->state.arrow_atk: the attack uses ammunition.
	// rAthena sets it in battle_weapon_attack for bows and guns.
	ArrowAttack bool
	// AmmoAtk and AmmoEle are the equipped ammunition's ATK and element,
	// as bonus.arrow_atk and bonus.arrow_ele. They only apply when
	// ArrowAttack is set. Cannonballs add no ATK to normal attacks, so
	// AmmoAtk must be 0 for them.
	AmmoAtk, AmmoEle int
	// AtkRate is bonus bAtkRate, in percent of weapon and equipment ATK.
	AtkRate int
	// Magic holds the bonuses that raise magic damage (bMagicAddRace and
	// its kin).
	Magic MagicBonus
	// IgnoreMDefEle, IgnoreMDefRace and IgnoreMDefClass are bits by
	// element, race and class: bIgnoreMdefEle and its kin, which make
	// magic ignore MDEF. IgnoreMDefRaceRate and IgnoreMDefClassRate are
	// bIgnoreMdefRaceRate and bIgnoreMdefClassRate, in percent.
	IgnoreMDefEle, IgnoreMDefRace, IgnoreMDefClass uint32
	IgnoreMDefRaceRate, IgnoreMDefClassRate        map[int]int
	// MagicSubDefEle is bMagicSubDefEle and MagicSubSize bMagicSubSize.
	// AddMDef is bAddMDefMonster, by the attacker's monster ID.
	// MagicDefRate and MiscDefRate are bMagicDefRate and bMiscDefRate.
	MagicSubDefEle, MagicSubSize, AddMDef map[int]int
	MagicDefRate, MiscDefRate             int
	// SkillAtk is bSkillAtk: percent more damage by skill ID. SubSkill is
	// bSubSkill: percent less damage taken by skill ID.
	SkillAtk, SubSkill map[int]int
	// Cast holds the bonuses of cast times, delays and costs.
	Cast CastBonus
	// SkillHeal is bSkillHeal: percent more healing done by skill ID.
	// SkillHeal2 is bSkillHeal2: percent more healing received by skill ID.
	SkillHeal, SkillHeal2 map[int]int
	// AddHealRate is bHealPower: percent more healing done by the skills
	// that skill_add_heal_rate names. AddHeal2Rate is bHealPower2:
	// percent more healing received.
	AddHealRate, AddHeal2Rate int
	// CartWeight and CartWeightMax are the pushcart's weight, which Cart
	// Revolution reads.
	CartWeight, CartWeightMax int
	// Right and Left are each hand's weapon bonuses. rAthena counts both
	// on the right hand because left_cardfix_to_right is on. Arrow holds
	// the ammunition's.
	Right, Left Hand
	Arrow       Arrow
	// IgnoreDefRace and IgnoreDefClass are bonus2 bIgnoreDefRaceRate and
	// bIgnoreDefClassRate, in percent of DEF ignored by race and class.
	IgnoreDefRace, IgnoreDefClass map[int]int
	// IgnoreResRace is bIgnoreResRaceRate, in percent of Res ignored by
	// race.
	IgnoreResRace map[int]int
	// WeaponDamageRate is bWeaponDamageRate, in percent by weapon type.
	WeaponDamageRate map[int]int
	// CritAtkRate and NonCritAtkRate are bCritAtkRate and
	// bNonCritAtkRate. ShortAtkRate and LongAtkRate are bShortAtkRate and
	// bLongAtkRate. All are percents of damage.
	CritAtkRate, NonCritAtkRate, ShortAtkRate, LongAtkRate int
	// CritAddRace is bCriticalAddRace, in tenths of a percent of critical
	// chance by race. CriticalRangeAtk is bCriticalLong, in tenths.
	CritAddRace      map[int]int
	CriticalRangeAtk int
	// PerfectHit is the chance of perfect hit, in percent: the highest
	// bPerfectHitRate plus bPerfectHitAddRate.
	PerfectHit int
	// DoubleRate is the chance of a double attack with any weapon, in
	// percent: the highest bDoubleRate plus bDoubleAddRate.
	DoubleRate int
	// NoSizeFix is bNoSizeFix: the weapon size table does not apply.
	NoSizeFix bool
	// PreRenewalSizes picks the pre-renewal size table for knuckles and
	// whips: the size_table ruleset flag set to pre_renewal.
	PreRenewalSizes bool

	// Bonuses that reduce damage the player takes, as indexed_bonus.

	// SubEle is bSubEle, by the hit's element. SubEle2 is bonus3 bSubEle.
	SubEle  map[int]int
	SubEle2 []FlaggedRate
	// SubRace, SubSize, SubClass and SubRace2 are bSubRace, bSubSize,
	// bSubClass and bSubRace2, by the attacker's race, size, class and
	// monster group. SubRace3 is bonus3 bSubRace.
	SubRace, SubSize, SubClass, SubRace2 map[int]int
	SubRace3                             []FlaggedRate
	// WeaponSubSize is bWeaponSubSize, by the attacker's size.
	WeaponSubSize map[int]int
	// SubDefEle is bSubDefEle, by the attacker's own element.
	SubDefEle map[int]int
	// AddDef is bAddDefMonster, by the attacker's monster ID.
	AddDef map[int]int
	// NearAtkDef and LongAtkDef are bNearAtkDef and bLongAtkDef: percent
	// less damage from melee and ranged physical hits.
	NearAtkDef, LongAtkDef int
	// CritDefRate is bCritDefRate: percent less damage from critical
	// hits. CriticalDef is bCriticalDef: percent less chance to be hit
	// critically.
	CritDefRate, CriticalDef int
	// DamageTakenCap is bAbsorbDmgMaxHP2: one hit deals at most this
	// percent of max HP.
	DamageTakenCap int
	// DualWield is the class rule for attacks with a weapon in each hand.
	DualWield DualWield
}

// CastBonus are the bonuses of skill cast times, delays and costs, as
// rAthena's castrate, bonus.varcastrate and their kin. The maps are by
// skill ID.
type CastBonus struct {
	// CastRate is castrate (bCastrate, pre-renewal), 100 by default.
	// VarCastRate is bonus.varcastrate (bVariableCastrate), FixCastRate
	// bonus.fixcastrate (bFixedCastrate, the lowest kept), and AddVarCast
	// and AddFixCast bonus.add_varcast and add_fixcast, in ms.
	CastRate, VarCastRate, FixCastRate, AddVarCast, AddFixCast int
	// SkillCastRate is skillcastrate, SkillVarCast skillvarcast,
	// SkillFixCast skillfixcast and SkillFixCastRate skillfixcastrate.
	SkillCastRate, SkillVarCast, SkillFixCast, SkillFixCastRate map[int]int
	// DelayRate is bonus.delayrate (bDelayrate): percent less after-cast
	// delay. SkillDelay is skilldelay (bSkillDelay) and SkillCooldown
	// skillcooldown, in ms.
	DelayRate                 int
	SkillDelay, SkillCooldown map[int]int
	// SPRate is dsprate (bUseSPrate), 100 by default. SkillUseSPRate is
	// skillusesprate and SkillUseSP skillusesp.
	SPRate                     int
	SkillUseSPRate, SkillUseSP map[int]int
	// NoCastCancel is special_state.no_castcancel and NoCastCancel2
	// no_castcancel2 (bNoCastCancel and bNoCastCancel2).
	NoCastCancel, NoCastCancel2 bool
	// NoGemstone is special_state.no_gemstone (bNoGemStone).
	NoGemstone bool
}

// MagicBonus are the bonuses that raise magic damage, as rAthena's
// magic_addrace, magic_addele, magic_addsize, magic_addclass,
// magic_addrace2, add_mdmg and magic_atk_ele.
type MagicBonus struct {
	AddRace, AddEle, AddSize, AddClass, AddRace2 map[int]int
	// AddDamage is bAddMagicDamageClass, by monster ID.
	AddDamage map[int]int
	// AtkEle is bMagicAtkEle, by the attack's element.
	AtkEle map[int]int
}

// DualWield is a class's rule for damage with a weapon in each hand, from
// battle_calc_attack_left_right_hands.
type DualWield int

const (
	// DualWieldNone applies no rate. Other classes cannot dual wield.
	DualWieldNone DualWield = iota
	// DualWieldThief is for classes that start as a Thief (MAPID_THIEF):
	// AS_RIGHT and AS_LEFT.
	DualWieldThief
	// DualWieldKagerou is for Kagerou and Oboro: KO_RIGHT and KO_LEFT.
	DualWieldKagerou
)

// skill returns the level of a learned skill, as pc_checkskill. It
// returns 0 for units that are not players.
func (s *Status) skill(id int) int {
	if s.Player == nil {
		return 0
	}
	return s.Player.Skills[id]
}

// Weapon types, as rAthena's weapon_type.
const (
	WeaponFist = iota
	WeaponDagger
	Weapon1HSword
	Weapon2HSword
	Weapon1HSpear
	Weapon2HSpear
	Weapon1HAxe
	Weapon2HAxe
	WeaponMace
	Weapon2HMace
	WeaponStaff
	WeaponBow
	WeaponKnuckle
	WeaponMusical
	WeaponWhip
	WeaponBook
	WeaponKatar
	WeaponRevolver
	WeaponRifle
	WeaponGatling
	WeaponShotgun
	WeaponGrenade
	WeaponHuuma
	Weapon2HStaff
	weaponTypeCount
)

// Sizes, as rAthena's e_size. SizeAll is SZ_ALL, which item bonuses use
// for every size.
const (
	SizeSmall = iota
	SizeMedium
	SizeLarge
	sizeCount
	SizeAll = sizeCount
)

// Monster classes, as rAthena's e_classAjust. ClassAll is CLASS_ALL.
const (
	ClassNormal = 0
	ClassBoss   = 1
	ClassAll    = 6
)

// Races that attacks read, as rAthena's e_race.
const (
	RaceFormless    = 0
	RaceUndead      = 1
	RaceBrute       = 2
	RacePlant       = 3
	RaceInsect      = 4
	RaceFish        = 5
	RaceDemon       = 6
	RaceDemiHuman   = 7
	RacePlayerDoram = 11
	RaceAll         = 12
)

// Skill IDs that normal attacks read, from rAthena's db/re/skill_db.yml.
const (
	skillSwordMastery       = 2    // SM_SWORD
	skillTwoHandMastery     = 3    // SM_TWOHAND
	skillDivineProtection   = 22   // AL_DP
	skillDemonBane          = 23   // AL_DEMONBANE
	skillVultureEye         = 44   // AC_VULTURE
	skillDoubleAttack       = 48   // TF_DOUBLE
	skillSpearMastery       = 55   // KN_SPEARMASTERY
	skillMaceMastery        = 65   // PR_MACEMASTERY
	skillHiltBinding        = 105  // BS_HILTBINDING
	skillWeaponResearch     = 107  // BS_WEAPONRESEARCH
	skillBeastBane          = 126  // HT_BEASTBANE
	skillRightHandMastery   = 132  // AS_RIGHT
	skillLeftHandMastery    = 133  // AS_LEFT
	skillKatarMastery       = 134  // AS_KATAR
	skillAxeMastery         = 226  // AM_AXEMASTERY
	skillIronFists          = 259  // MO_IRONHAND
	skillAdvancedBook       = 274  // SA_ADVANCEDBOOK
	skillMusicalLesson      = 315  // BA_MUSICALLESSON
	skillDancingLesson      = 323  // DC_DANCINGLESSON
	skillAdvKatarMastery    = 376  // ASC_KATAR
	skillTKRun              = 411  // TK_RUN
	skillTKPower            = 424  // TK_POWER
	skillChainAction        = 511  // GS_CHAINACTION
	skillDragonTraining     = 2007 // RK_DRAGONTRAINING
	skillRangerMain         = 2235 // RA_RANGERMAIN
	skillMadoLicence        = 2255 // NC_MADOLICENCE
	skillTrainingAxe        = 2276 // NC_TRAININGAXE
	skillResearchFE         = 2277 // NC_RESEARCHFE
	skillTrainingSword      = 2474 // GN_TRAINING_SWORD
	skillKORightHand        = 3002 // KO_RIGHT
	skillKOLeftHand         = 3003 // KO_LEFT
	skillSoulAttack         = 5044 // SU_SOULATTACK
	skillTwoHandStaff       = 5228 // AG_TWOHANDSTAFF
	skillNoviceBreakthrough = 5075 // NV_BREAKTHROUGH
)

// Double Attack and Chain Action hit twice (HitCount in skill_db.yml) and
// have the Critical damage flag, so a double hit can still be critical.
const (
	doubleAttackHits    = 2
	doubleAttackCanCrit = true
	chainActionHits     = 2
	chainActionCanCrit  = true
)

// doubleAttackRate is Double Attack's chance per level in renewal, in
// percent.
const doubleAttackRate = 7

// sizeFixTable is each weapon type's damage percent against small, medium
// and large targets, from rAthena's db/size_fix.yml with the renewal
// changes in db/re/size_fix.yml.
var sizeFixTable = [weaponTypeCount][sizeCount]int{
	WeaponFist:     {100, 100, 100},
	WeaponDagger:   {100, 75, 50},
	Weapon1HSword:  {75, 100, 75},
	Weapon2HSword:  {75, 75, 100},
	Weapon1HSpear:  {75, 75, 100},
	Weapon2HSpear:  {75, 75, 100},
	Weapon1HAxe:    {50, 75, 100},
	Weapon2HAxe:    {50, 75, 100},
	WeaponMace:     {75, 100, 100},
	Weapon2HMace:   {100, 100, 100},
	WeaponStaff:    {100, 100, 100},
	WeaponBow:      {100, 100, 75},
	WeaponKnuckle:  {100, 100, 75},
	WeaponMusical:  {75, 100, 75},
	WeaponWhip:     {75, 100, 75},
	WeaponBook:     {100, 100, 50},
	WeaponKatar:    {75, 100, 75},
	WeaponRevolver: {100, 100, 100},
	WeaponRifle:    {100, 100, 100},
	WeaponGatling:  {100, 100, 100},
	WeaponShotgun:  {100, 100, 100},
	WeaponGrenade:  {100, 100, 100},
	WeaponHuuma:    {100, 100, 100},
	Weapon2HStaff:  {100, 100, 100},
}

// PreRenewalSizeFix is SizeFix with pre-renewal's size table: rAthena's
// db/size_fix.yml with the changes in db/pre-re/size_fix.yml, which make
// knuckles 75% against medium and 50% against large targets, and whips
// 50% against large targets.
func PreRenewalSizeFix(weaponType, size int) int {
	switch {
	case weaponType == WeaponKnuckle && size == SizeMedium:
		return 75
	case (weaponType == WeaponKnuckle || weaponType == WeaponWhip) && size == SizeLarge:
		return 50
	}
	return SizeFix(weaponType, size)
}

// SizeFix returns a weapon type's damage percent against a target size, as
// the atkmods that status_calc_pc sets from size_fix_db. It returns 100 for
// values outside the table.
func SizeFix(weaponType, size int) int {
	if weaponType < 0 || weaponType >= weaponTypeCount || size < 0 || size >= sizeCount {
		return 100
	}
	return sizeFixTable[weaponType][size]
}

// hand is a weapon hand, as rAthena's EQI_HAND_R and EQI_HAND_L.
type hand int

const (
	handRight hand = iota
	handLeft
)

// weapon returns a hand's weapon type.
func (p *Player) weapon(h hand) int {
	if h == handLeft {
		return p.LeftWeapon
	}
	return p.RightWeapon
}

// calcMultiAttack is battle_calc_multi_attack for a normal attack by a
// player: Double Attack with a dagger, or with any weapon when an item
// grants it or with bDoubleRate, and Chain Action with a revolver.
// ratePerLevel is Double Attack's chance per level: doubleAttackRate in
// renewal and preRenewalDoubleAttackRate in pre-renewal. Skills have no
// multiple hits here.
//
// Fear Breeze and Shadow Warrior are not ported.
func calcMultiAttack(wd *damage, src *Status, ratePerLevel int, r Rand) {
	sd := src.Player
	if sd == nil || wd.sk != nil {
		return
	}
	lv := src.skill(skillDoubleAttack)
	// Double Attack from an item works with any weapon.
	granted := sd.Granted[skillDoubleAttack] && sd.RightWeapon != WeaponFist
	if wd.div == 1 && (lv > 0 && sd.RightWeapon == WeaponDagger || granted || sd.DoubleRate > 0 && sd.RightWeapon != WeaponFist) {
		// The rate per level, or bDoubleRate when it is higher. The rates
		// are not added.
		if r.IntN(100) < max(ratePerLevel*lv, sd.DoubleRate) {
			wd.div = doubleAttackHits
			wd.typ = dmgMultiHit
		}
	}
	if lv := src.skill(skillChainAction); wd.div == 1 && sd.RightWeapon == WeaponRevolver && lv > 0 && r.IntN(100) < 5*lv {
		wd.div = chainActionHits
		wd.typ = dmgMultiHit
		// rAthena also starts SC_QD_SHOT_READY on the player here.
	}
}

// calcDamageParts is the renewal battle_calc_damage_parts for a player:
// status ATK, weapon ATK, equipment ATK and mastery ATK, each hand apart.
func calcDamageParts(wd *damage, src, target *Status, critical bool, r Rand) {
	rightElement := weaponElement(src, handRight, wd.sk, r)
	leftElement := weaponElement(src, handLeft, wd.sk, r)

	wd.statusAtk += int64(src.BAtk)
	wd.statusAtk2 += int64(src.BAtk)

	// Status ATK is neutral on normal attacks.
	wd.statusAtk = AttrFix(wd.statusAtk, Neutral, target.DefEle, target.EleLv, 0)
	wd.statusAtk2 = AttrFix(wd.statusAtk2, Neutral, target.DefEle, target.EleLv, 0)

	// Right-hand status ATK is doubled after the element table.
	wd.statusAtk *= 2

	wd.weaponAtk += calcBaseWeaponAttack(src, target, handRight, critical, r)
	wd.weaponAtk2 += calcBaseWeaponAttack(src, target, handLeft, critical, r)

	// SC_SUB_WEAPONPROPERTY adds a percent of weapon ATK as damage of its
	// element. The weapon's element then applies to the sum too.
	if c := src.Changes; c != nil && c.SubWeaponRate != 0 {
		rate := int64(c.SubWeaponRate)
		bonus := int64(math.Floor(float64(float32(wd.weaponAtk * rate / 100))))
		bonus2 := int64(math.Floor(float64(float32(wd.weaponAtk2 * rate / 100))))
		bonus = AttrFix(bonus, c.SubWeaponElement, target.DefEle, target.EleLv, 0)
		bonus2 = AttrFix(bonus2, c.SubWeaponElement, target.DefEle, target.EleLv, 0)
		wd.weaponAtk += bonus
		wd.weaponAtk2 += bonus2
	}

	wd.weaponAtk = AttrFix(wd.weaponAtk, rightElement, target.DefEle, target.EleLv, 0)
	wd.weaponAtk2 = AttrFix(wd.weaponAtk2, leftElement, target.DefEle, target.EleLv, 0)

	wd.equipAtk += calcEquipAttack(src, wd.sk)
	wd.equipAtk = AttrFix(wd.equipAtk, rightElement, target.DefEle, target.EleLv, 0)

	wd.equipAtk2 += calcEquipAttack(src, wd.sk)
	wd.equipAtk2 = AttrFix(wd.equipAtk2, leftElement, target.DefEle, target.EleLv, 0)

	// AtkRate adds a percent of weapon and equipment ATK.
	if rate := int64(src.Player.AtkRate); rate != 0 {
		wd.percentAtk = (wd.weaponAtk + wd.equipAtk) * rate / 100
		wd.percentAtk2 = (wd.weaponAtk2 + wd.equipAtk2) * rate / 100
	}

	// Mastery ATK has no element.
	calcAttackMasteries(wd, src, target)

	wd.damage = 0
	wd.damage2 = 0
}

// calcBaseWeaponAttack is the renewal battle_calc_base_weapon_attack: the
// weapon ATK roll with its variance and stat bonus, then the size
// penalty.
func calcBaseWeaponAttack(src, target *Status, h hand, critical bool, r Rand) int64 {
	sd := src.Player
	wa, atkmin := &src.RHW, src.WAtk
	if h == handLeft {
		wa, atkmin = &src.LHW, src.WAtk2
	}
	atkmax := atkmin
	dmg := int64(atkmin)

	// rAthena checks that the hand holds an item. An empty hand, or a
	// shield, has no weapon ATK, so both give the same result.
	if sd.weapon(h) != WeaponFist {
		baseStat := src.Str
		switch sd.Weapon {
		case WeaponBow, WeaponMusical, WeaponWhip, WeaponRevolver, WeaponRifle, WeaponGatling, WeaponShotgun, WeaponGrenade:
			if src.skill(skillSoulAttack) == 0 {
				baseStat = src.Dex
			}
		}

		// rAthena computes these in float. Each product is converted to
		// float32, so Go cannot fuse it with the next addition.
		variance := float32(float32(float32(5)*float32(wa.Atk))*float32(wa.Wlv)) / 100
		statBonus := float32(wa.Atk*baseStat) / 200

		atkmin = max(0, int(float32(float32(atkmin)-variance)+statBonus))
		atkmax = min(math.MaxUint16, int(float32(float32(atkmax)+variance)+statBonus))

		// Maximize Power also takes the maximum. Status changes are not
		// ported.
		if critical {
			dmg = int64(atkmax)
		} else {
			dmg = int64(rndValue(r, atkmin, atkmax))
		}
	}

	dmg = addWeaponDamage(sd, dmg, h, r)

	// battle_calc_sizefix. Weapon Perfection also skips the size penalty.
	// Status changes are not ported.
	if !sd.NoSizeFix {
		fix := SizeFix(sd.weapon(h), target.Size)
		if sd.PreRenewalSizes {
			fix = PreRenewalSizeFix(sd.weapon(h), target.Size)
		}
		dmg = dmg * int64(fix) / 100
	}
	return min(max(dmg, math.MinInt32), math.MaxInt32)
}

// addWeaponDamage is battle_add_weapon_damage: the over-refine bonus and
// bWeaponDamageRate of a hand.
func addWeaponDamage(sd *Player, dmg int64, h hand, r Rand) int64 {
	hd := &sd.Right
	if h == handLeft {
		hd = &sd.Left
	}
	if hd.Overrefine > 0 {
		dmg += int64(r.IntN(hd.Overrefine) + 1)
	}
	if rate := sd.WeaponDamageRate[sd.weapon(h)]; rate != 0 {
		dmg += dmg * int64(rate) / 100
	}
	return dmg
}

// rndValue is rAthena's rnd_value: a number in [lo, hi]. A range of one
// value draws nothing.
func rndValue(r Rand, lo, hi int) int {
	if lo > hi {
		lo, hi = hi, lo
	}
	if lo == hi {
		return lo
	}
	return lo + r.IntN(hi-lo+1)
}

// calcEquipAttack is the renewal battle_calc_equip_attack: equipment ATK,
// plus the ammunition's ATK when the attack uses it.
func calcEquipAttack(src *Status, sk *Skill) int64 {
	eatk := 0
	if src.Player != nil && isSkillUsingArrow(src, sk) {
		eatk += src.Player.AmmoAtk
	}
	return int64(eatk + src.EAtk)
}

// calcAttackMasteries is the renewal battle_calc_attack_masteries for a
// player. A few skills take no mastery ATK.
//
// Star Crumbs and Spirit Spheres also add mastery ATK. They are not
// ported.
func calcAttackMasteries(wd *damage, src, target *Status) {
	if !skillStacksMasteries(wd.skillID()) {
		return
	}
	wd.masteryAtk = addMastery(src, target, handRight, false)
	if isAttackLeftHanded(src, wd.sk) {
		wd.masteryAtk2 = addMastery(src, target, handLeft, false)
	}
	// Envenom's added ATK counts as mastery ATK.
	if wd.skillID() == skillEnvenom {
		wd.atkAdd(src, &wd.masteryAtk, &wd.masteryAtk2, int64(15*wd.skillLv()))
	}
	if wd.skillID() != skillCartRevolution && src.skill(skillHiltBinding) > 0 {
		wd.atkAdd(src, &wd.masteryAtk, &wd.masteryAtk2, 4)
	}
}

// addMastery is battle_addmastery: ATK from mastery skills, by target and
// by the hand's weapon type. It returns the ATK to add. Renewal starts
// from 0 and pre-renewal from the hand's damage, so the caller adds it to
// the right value. pre selects the pre-renewal rules: Weapon Research
// adds nothing here, and Axe Mastery does not work with one-handed
// swords.
//
// Spirit charms and the Hunter spirit are not ported.
func addMastery(src, target *Status, h hand, pre bool) int64 {
	var dmg int64
	lv := func(id int) int64 { return int64(src.skill(id)) }

	// Demon Bane does not work against players.
	if l := lv(skillDemonBane); l > 0 && target.Player == nil && (isUndead(target.DefEle) || target.Race == RaceDemon) {
		dmg += int64(float64(l) * (float64(src.Level)/20.0 + 3.0))
	}
	if l := lv(skillRangerMain); l > 0 && (target.Race == RaceBrute || target.Race == RacePlayerDoram || target.Race == RacePlant || target.Race == RaceFish) {
		dmg += l * 5
	}
	if l := lv(skillResearchFE); l > 0 && (target.DefEle == Fire || target.DefEle == Earth) {
		dmg += l * 10
	}
	// Mado License adds ATK even without a Madogear.
	dmg += 15 * lv(skillMadoLicence)
	if l := lv(skillBeastBane); l > 0 && (target.Race == RaceInsect || target.Race == RaceBrute || target.Race == RacePlayerDoram) {
		dmg += l * 4
	}
	// Renewal: Weapon Research works with every weapon. Pre-renewal adds
	// it in battle_calc_weapon_attack instead.
	if !pre {
		dmg += 2 * lv(skillWeaponResearch)
	}
	if l := lv(skillNoviceBreakthrough); l > 0 {
		dmg += 15 * l
		if l > 4 {
			dmg += 25
		}
	}

	switch src.Player.weapon(h) {
	case Weapon1HSword, WeaponDagger:
		// Renewal: Axe Mastery also works with one-handed swords.
		if src.Player.weapon(h) == Weapon1HSword && !pre {
			dmg += 3 * lv(skillAxeMastery)
		}
		dmg += 4 * lv(skillSwordMastery)
		dmg += 10 * lv(skillTrainingSword)
	case Weapon2HSword:
		dmg += 4 * lv(skillTwoHandMastery)
	case Weapon1HSpear, Weapon2HSpear:
		if l := lv(skillSpearMastery); l > 0 {
			// Riding adds 5 per level instead of 4. Mounts are not ported.
			dmg += 4 * l
			if lv(skillDragonTraining) > 0 {
				dmg += 10 * l
			}
		}
	case Weapon1HAxe, Weapon2HAxe:
		dmg += 3 * lv(skillAxeMastery)
		dmg += 5 * lv(skillTrainingAxe)
	case WeaponMace, Weapon2HMace:
		dmg += 3 * lv(skillMaceMastery)
		dmg += 4 * lv(skillTrainingAxe)
	case WeaponFist, WeaponKnuckle:
		if src.Player.weapon(h) == WeaponFist {
			dmg += 10 * lv(skillTKRun)
		}
		dmg += 3 * lv(skillIronFists)
	case WeaponMusical:
		dmg += 3 * lv(skillMusicalLesson)
	case WeaponWhip:
		dmg += 3 * lv(skillDancingLesson)
	case WeaponBook:
		dmg += 3 * lv(skillAdvancedBook)
	case WeaponKatar:
		dmg += 3 * lv(skillKatarMastery)
	}
	return dmg
}

// isUndead is battle_check_undead with undead_detect_type 0: only the
// Undead element counts, not the race.
func isUndead(element int) bool {
	return element == Undead
}

// weaponElement is battle_get_weapon_element: the skill's element, or for
// a normal attack or a skill of the weapon's element, the hand's weapon
// element, or the ammunition's element for the right hand when the attack
// uses ammunition.
//
// Endowments and spirit charms change the element. They are not ported.
func weaponElement(src *Status, h hand, sk *Skill, r Rand) int {
	if sk != nil && sk.Element != ElementWeapon {
		switch sk.Element {
		case ElementEndowed:
			// status_get_attack_sc_element: no endowment is ported.
			return Neutral
		case ElementRandom:
			return r.IntN(AllElements)
		}
		return sk.Element
	}
	if h == handLeft {
		return src.LHW.Ele
	}
	element := src.RHW.Ele
	if sd := src.Player; sd != nil && isSkillUsingArrow(src, sk) && sd.AmmoEle != Neutral {
		element = sd.AmmoEle
	}
	return element
}

// cardfixAttacker is battle_calc_cardfix for BF_WEAPON with the attacker's
// bonuses. It returns the damage to add. left is the left flag: 2 for the
// right hand, and 3 for the left hand. With left_cardfix_to_right on, both
// hands' bonuses count on the right hand, and the left hand only gets its
// bAddDamageClass. A skill that ignores attack cards takes none.
func cardfixAttacker(src, target *Status, dmg int64, left, flag int, nk NK) int64 {
	if dmg == 0 || nk.Has(NKIgnoreAtkCard) {
		return 0
	}
	cardfix, cardfixLeft := cardfixAddDamage(src.Player, target, cardfixHands(src.Player, target, flag, nk), left)
	return applyCardfix(dmg, chooseCardfix(cardfix, cardfixLeft, left))
}

// cardfixHands is the part of battle_calc_cardfix for BF_WEAPON that
// sums the attacker's race, element, size, monster group and class
// bonuses: the ammunition's and the right hand's for an attack with
// ammunition, and both hands' otherwise. It returns the rate, on a base of
// 1000. Element bonuses of an attack with ammunition leave out a skill
// that ignores the element.
func cardfixHands(sd *Player, target *Status, flag int, nk NK) int64 {
	r, l := &sd.Right, &sd.Left
	cardfix := int64(1000)
	if sd.ArrowAttack {
		a := &sd.Arrow
		cardfix = cardfix * int64(100+r.AddRace[target.Race]+a.AddRace[target.Race]+r.AddRace[RaceAll]+a.AddRace[RaceAll]) / 100
		if !nk.Has(NKIgnoreElement) {
			ele := r.AddEle[target.DefEle] + a.AddEle[target.DefEle] + r.AddEle[AllElements] + a.AddEle[AllElements]
			ele += sumFlagged(r.AddEle2, target.DefEle, AllElements, flag)
			cardfix = cardfix * int64(100+ele) / 100
		}
		cardfix = cardfix * int64(100+r.AddSize[target.Size]+a.AddSize[target.Size]+r.AddSize[SizeAll]+a.AddSize[SizeAll]) / 100
		race2 := 0
		for _, g := range target.Race2 {
			race2 += r.AddRace2[g]
		}
		cardfix = cardfix * int64(100+race2) / 100
		cardfix = cardfix * int64(100+r.AddClass[target.Class]+a.AddClass[target.Class]+r.AddClass[ClassAll]+a.AddClass[ClassAll]) / 100
	} else {
		ele := r.AddEle[target.DefEle] + l.AddEle[target.DefEle] + r.AddEle[AllElements] + l.AddEle[AllElements]
		ele += sumFlagged(r.AddEle2, target.DefEle, AllElements, flag)
		ele += sumFlagged(l.AddEle2, target.DefEle, AllElements, flag)
		cardfix = cardfix * int64(100+ele) / 100
		cardfix = cardfix * int64(100+r.AddRace[target.Race]+l.AddRace[target.Race]+r.AddRace[RaceAll]+l.AddRace[RaceAll]) / 100
		cardfix = cardfix * int64(100+r.AddSize[target.Size]+l.AddSize[target.Size]+r.AddSize[SizeAll]+l.AddSize[SizeAll]) / 100
		for _, g := range target.Race2 {
			cardfix = cardfix * int64(100+r.AddRace2[g]+l.AddRace2[g]) / 100
		}
		cardfix = cardfix * int64(100+r.AddClass[target.Class]+l.AddClass[target.Class]+r.AddClass[ClassAll]+l.AddClass[ClassAll]) / 100
	}
	return cardfix
}

// cardfixAddDamage adds bAddDamageClass to the rate of cardfixHands. It is
// by the target's monster ID, or job for a player. It also returns the
// left hand's rate, which with the left flag (left&1) only has the left
// hand's bAddDamageClass. Without the flag, the left rate stays 1000.
func cardfixAddDamage(sd *Player, target *Status, cardfix int64, left int) (int64, int64) {
	cardfixLeft := int64(1000)
	if v, ok := sd.Right.AddDamage[target.ID]; ok {
		cardfix = cardfix * int64(100+v) / 100
	}
	if left&1 != 0 {
		if v, ok := sd.Left.AddDamage[target.ID]; ok {
			cardfixLeft = cardfixLeft * int64(100+v) / 100
		}
	}
	return cardfix, cardfixLeft
}

// chooseCardfix is the end of battle_calc_cardfix's attacker part: the
// left hand's rate with the left flag, and the right hand's otherwise.
func chooseCardfix(cardfix, cardfixLeft int64, left int) int64 {
	if left&1 != 0 {
		return cardfixLeft
	}
	return cardfix
}

// cardfixTarget is battle_calc_cardfix for BF_WEAPON with the target's
// bonuses, for an attack on a player. It returns the damage to add. rhEle
// and lhEle are the elements of the attack's hands. left is the left
// flag: 0 for the right hand and 1 for the left hand. The left hand also
// gets bSubEle for its own element when it differs.
func cardfixTarget(src, target *Status, dmg int64, rhEle, lhEle, left, flag int, nk NK) int64 {
	if dmg == 0 || nk.Has(NKIgnoreDefCard) {
		return 0
	}
	tsd := target.Player
	cardfix := int64(1000)
	if !nk.Has(NKIgnoreElement) {
		ele := tsd.SubEle[rhEle] + tsd.SubEle[AllElements] + sumFlagged(tsd.SubEle2, rhEle, AllElements, flag)
		cardfix = cardfix * int64(100-ele) / 100
		if left&1 != 0 && lhEle != rhEle {
			ele := tsd.SubEle[lhEle] + tsd.SubEle[AllElements] + sumFlagged(tsd.SubEle2, lhEle, AllElements, flag)
			cardfix = cardfix * int64(100-ele) / 100
		}
	}
	// s_defele is the attacker's own element when the target is a player.
	cardfix = cardfix * int64(100-tsd.SubDefEle[src.DefEle]-tsd.SubDefEle[AllElements]) / 100
	cardfix = cardfix * int64(100-tsd.SubSize[src.Size]-tsd.SubSize[SizeAll]) / 100
	cardfix = cardfix * int64(100-tsd.WeaponSubSize[src.Size]-tsd.WeaponSubSize[SizeAll]) / 100
	race2 := 0
	for _, g := range src.Race2 {
		race2 += tsd.SubRace2[g]
	}
	cardfix = cardfix * int64(100-race2) / 100
	race := tsd.SubRace[src.Race] + tsd.SubRace[RaceAll] + sumFlagged(tsd.SubRace3, src.Race, RaceAll, flag)
	cardfix = cardfix * int64(100-race) / 100
	cardfix = cardfix * int64(100-tsd.SubClass[src.Class]-tsd.SubClass[ClassAll]) / 100
	if v, ok := tsd.AddDef[src.ID]; ok {
		cardfix = cardfix * int64(100-v) / 100
	}
	if flag&bfShort != 0 {
		cardfix = cardfix * int64(100-tsd.NearAtkDef) / 100
	} else if !nk.Has(NKIgnoreLongCard) {
		cardfix = cardfix * int64(100-tsd.LongAtkDef) / 100
	}
	if tc := target.Changes; tc != nil && tc.DefRate != 0 {
		cardfix = cardfix * int64(100-tc.DefRate) / 100
	}
	return applyCardfix(dmg, cardfix)
}

// applyCardfix is rAthena's APPLY_CARDFIX, returning the damage to add.
func applyCardfix(dmg, cardfix int64) int64 {
	fixed := dmg - dmg*(1000-max(0, cardfix))/1000
	return min(max(fixed-dmg, math.MinInt32), math.MaxInt32)
}

// sumPlayerDamage is the part of the renewal battle_calc_weapon_attack
// that sums a player's ATK parts into damage, with the critical and
// range rates.
func sumPlayerDamage(wd *damage, src *Status) {
	sd := src.Player
	wd.damage = wd.statusAtk + wd.weaponAtk + wd.equipAtk + wd.percentAtk
	if isAttackLeftHanded(src, wd.sk) {
		wd.damage2 = wd.statusAtk2 + wd.weaponAtk2 + wd.equipAtk2 + wd.percentAtk2
	}
	// P.ATK, rounded through float as rAthena does.
	patk := int64(100 + src.PAtk)
	wd.damage = int64(math.Floor(float64(float32(wd.damage * patk / 100))))
	if isAttackLeftHanded(src, wd.sk) {
		wd.damage2 = int64(math.Floor(float64(float32(wd.damage2 * patk / 100))))
	}
	wd.damage += wd.masteryAtk
	if isAttackLeftHanded(src, wd.sk) {
		wd.damage2 += wd.masteryAtk2
	}
	if wd.typ == dmgCritical || wd.typ == dmgMultiHitCritical {
		// bCritAtkRate, in full on a normal attack and in half on a skill.
		div := int64(100)
		if wd.sk != nil {
			div = 200
		}
		wd.damage += int64(math.Floor(float64(float32(wd.damage * int64(sd.CritAtkRate) / div))))
		if isAttackLeftHanded(src, wd.sk) {
			wd.damage2 += int64(math.Floor(float64(float32(wd.damage2 * int64(sd.CritAtkRate) / div))))
		}
	} else if wd.sk == nil || !wd.sk.IgnoreNonCritBonus {
		wd.atkAddRate(src, &wd.damage, &wd.damage2, int64(sd.NonCritAtkRate))
	}
	if wd.flag&bfShort != 0 {
		wd.atkAddRate(src, &wd.damage, &wd.damage2, int64(sd.ShortAtkRate))
	}
	if wd.flag&bfLong != 0 {
		wd.atkAddRate(src, &wd.damage, &wd.damage2, int64(sd.LongAtkRate))
	}
}

// calcLeftRightHands is the player part of
// battle_calc_attack_left_right_hands: the katar's second hit and the
// dual-wield rates.
func calcLeftRightHands(wd *damage, src *Status) {
	sd := src.Player
	if sd.Weapon == WeaponKatar && wd.sk == nil {
		// Katars hit a second time for 1% plus 2% per Double Attack level.
		wd.damage2 = wd.damage * int64(1+src.skill(skillDoubleAttack)*2) / 100
		return
	}
	if !isAttackLeftHanded(src, wd.sk) {
		return
	}
	// With only a left-hand weapon, the right hand deals the damage of an
	// unarmed attack.
	if isAttackRightHanded(src, wd.sk) && wd.damage != 0 {
		switch sd.DualWield {
		case DualWieldThief:
			wd.damage = wd.damage * int64(50+src.skill(skillRightHandMastery)*10) / 100
		case DualWieldKagerou:
			wd.damage = wd.damage * int64(70+src.skill(skillKORightHand)*10) / 100
		}
		wd.damage = max(wd.damage, 1)
	}
	// The left hand is always adjusted, even with an empty right hand.
	if wd.damage2 != 0 {
		switch sd.DualWield {
		case DualWieldThief:
			wd.damage2 = wd.damage2 * int64(30+src.skill(skillLeftHandMastery)*10) / 100
		case DualWieldKagerou:
			wd.damage2 = wd.damage2 * int64(50+src.skill(skillKOLeftHand)*10) / 100
		}
		wd.damage2 = max(wd.damage2, 1)
	}
}
