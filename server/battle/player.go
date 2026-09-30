package battle

import "math"

// Player is what a player attacker has that a monster does not, from
// rAthena's map_session_data. Set it on a Status to make Attack use the
// player formula.
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
	// AddRace is bonus2 bAddRace from both hands, in percent by race.
	// rAthena merges both hands because left_cardfix_to_right is on.
	AddRace map[int]int
	// IgnoreDefRace is bonus2 bIgnoreDefRaceRate, in percent of DEF
	// ignored by race.
	IgnoreDefRace map[int]int
	// DualWield is the class rule for attacks with a weapon in each hand.
	DualWield DualWield
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

// Sizes, as rAthena's e_size.
const (
	SizeSmall = iota
	SizeMedium
	SizeLarge
	sizeCount
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
// player: Double Attack with a dagger, and Chain Action with a revolver.
//
// Fear Breeze, Shadow Warrior and the bDoubleRate bonus are not ported.
func calcMultiAttack(wd *damage, src *Status, r Rand) {
	sd := src.Player
	if sd == nil {
		return
	}
	if lv := src.skill(skillDoubleAttack); wd.div == 1 && lv > 0 && sd.RightWeapon == WeaponDagger {
		// Renewal: 7% per level.
		if r.IntN(100) < 7*lv {
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
	rightElement := weaponElement(src, handRight)
	leftElement := weaponElement(src, handLeft)

	wd.statusAtk += int64(src.BAtk)
	wd.statusAtk2 += int64(src.BAtk)

	// Status ATK is neutral on normal attacks.
	wd.statusAtk = AttrFix(wd.statusAtk, Neutral, target.DefEle, target.EleLv, 0)
	wd.statusAtk2 = AttrFix(wd.statusAtk2, Neutral, target.DefEle, target.EleLv, 0)

	// Right-hand status ATK is doubled after the element table.
	wd.statusAtk *= 2

	wd.weaponAtk += calcBaseWeaponAttack(src, target, handRight, critical, r)
	wd.weaponAtk2 += calcBaseWeaponAttack(src, target, handLeft, critical, r)

	wd.weaponAtk = AttrFix(wd.weaponAtk, rightElement, target.DefEle, target.EleLv, 0)
	wd.weaponAtk2 = AttrFix(wd.weaponAtk2, leftElement, target.DefEle, target.EleLv, 0)

	wd.equipAtk += calcEquipAttack(src)
	wd.equipAtk = AttrFix(wd.equipAtk, rightElement, target.DefEle, target.EleLv, 0)

	wd.equipAtk2 += calcEquipAttack(src)
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

	// battle_add_weapon_damage adds the over-refine bonus and the
	// bWeaponDamageRate bonus. Neither is ported.

	// Weapon Perfection skips the size penalty. Status changes are not
	// ported.
	dmg = dmg * int64(SizeFix(sd.weapon(h), target.Size)) / 100
	return min(max(dmg, math.MinInt32), math.MaxInt32)
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
func calcEquipAttack(src *Status) int64 {
	eatk := 0
	if src.Player != nil && isSkillUsingArrow(src) {
		eatk += src.Player.AmmoAtk
	}
	return int64(eatk + src.EAtk)
}

// calcAttackMasteries is the renewal battle_calc_attack_masteries for a
// normal attack by a player.
//
// Star Crumbs and Spirit Spheres also add mastery ATK. They are not
// ported.
func calcAttackMasteries(wd *damage, src, target *Status) {
	wd.masteryAtk = addMastery(src, target, handRight)
	if isAttackLeftHanded(src) {
		wd.masteryAtk2 = addMastery(src, target, handLeft)
	}
	if src.skill(skillHiltBinding) > 0 {
		atkAdd(src, &wd.masteryAtk, &wd.masteryAtk2, 4)
	}
}

// addMastery is the renewal battle_addmastery: ATK from mastery skills,
// by target and by the hand's weapon type.
//
// Spirit charms and the Hunter spirit are not ported.
func addMastery(src, target *Status, h hand) int64 {
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
	// Renewal: Weapon Research works with every weapon.
	dmg += 2 * lv(skillWeaponResearch)
	if l := lv(skillNoviceBreakthrough); l > 0 {
		dmg += 15 * l
		if l > 4 {
			dmg += 25
		}
	}

	switch src.Player.weapon(h) {
	case Weapon1HSword, WeaponDagger:
		// Renewal: Axe Mastery also works with one-handed swords.
		if src.Player.weapon(h) == Weapon1HSword {
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

// weaponElement is battle_get_weapon_element for a normal attack: the
// hand's weapon element, or the ammunition's element for the right hand
// when the attack uses ammunition.
//
// Endowments and spirit charms change the element. They are not ported.
func weaponElement(src *Status, h hand) int {
	if h == handLeft {
		return src.LHW.Ele
	}
	element := src.RHW.Ele
	if sd := src.Player; sd != nil && isSkillUsingArrow(src) && sd.AmmoEle != Neutral {
		element = sd.AmmoEle
	}
	return element
}

// cardfixAttacker is battle_calc_cardfix for BF_WEAPON with the attacker's
// bonuses on the right hand (left flag 2). It returns the damage to add.
// With left_cardfix_to_right on, both hands' bonuses count together, and
// the left hand (left flag 3) gets no change.
//
// Only bAddRace is ported. Element, size, class and monster bonuses are
// not.
func cardfixAttacker(src, target *Status, dmg int64) int64 {
	if dmg == 0 {
		return 0
	}
	sd := src.Player
	cardfix := int64(1000)
	cardfix = cardfix * int64(100+sd.AddRace[target.Race]+sd.AddRace[RaceAll]) / 100
	fixed := dmg - dmg*(1000-max(0, cardfix))/1000
	return min(max(fixed-dmg, math.MinInt32), math.MaxInt32)
}

// sumPlayerDamage is the part of the renewal battle_calc_weapon_attack
// that sums a player's ATK parts into damage.
func sumPlayerDamage(wd *damage, src *Status) {
	wd.damage = wd.statusAtk + wd.weaponAtk + wd.equipAtk + wd.percentAtk
	if isAttackLeftHanded(src) {
		wd.damage2 = wd.statusAtk2 + wd.weaponAtk2 + wd.equipAtk2 + wd.percentAtk2
	}
	// P.ATK is 0 before the fourth classes, which are not ported. rAthena
	// still rounds through float here.
	wd.damage = int64(math.Floor(float64(float32(wd.damage))))
	if isAttackLeftHanded(src) {
		wd.damage2 = int64(math.Floor(float64(float32(wd.damage2))))
	}
	wd.damage += wd.masteryAtk
	if isAttackLeftHanded(src) {
		wd.damage2 += wd.masteryAtk2
	}
	// bCritAtkRate, bNonCritAtkRate, bShortAtkRate and bLongAtkRate are
	// not ported.
}

// calcLeftRightHands is the player part of
// battle_calc_attack_left_right_hands: the katar's second hit and the
// dual-wield rates.
func calcLeftRightHands(wd *damage, src *Status) {
	sd := src.Player
	if sd.Weapon == WeaponKatar {
		// Katars hit a second time for 1% plus 2% per Double Attack level.
		wd.damage2 = wd.damage * int64(1+src.skill(skillDoubleAttack)*2) / 100
		return
	}
	if !isAttackLeftHanded(src) {
		return
	}
	// With only a left-hand weapon, the right hand deals the damage of an
	// unarmed attack.
	if isAttackRightHanded(src) && wd.damage != 0 {
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
