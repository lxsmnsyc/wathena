// Package status computes a player's stats as rAthena's status.cpp does,
// so results match the recorded reference in
// reference/rathena/<mode>/pc_status.json, item_bonuses.json and
// refine.json.
//
// This is a port of rAthena's status.cpp and parts of pc.cpp (GPL-3.0).
// It covers a player with equipment, refines, cards and their bonuses,
// status changes (changes.go), and no mount and no cart, in both renewal
// and pre-renewal, and the status changes of monsters. Function comments
// name the rAthena functions they port. The settings are the defaults
// from rAthena's conf/battle files.
//
// Stats are totals of modifiers (decision 0009): Calc starts from the
// job tables and adds each equipped item, its refine and its bonuses in
// rAthena's order every time. It keeps no state between calls.
//
// What it does not port yet:
//   - The effects of status changes that only skills start, item
//     combos, random options, enchant grades and forged weapons.
//   - Summoners (pc_checkskill_summoner and Soul Attack's range), which
//     Calc refuses.
//   - Taekwon rankers, which need the fame list. Calc treats every
//     Taekwon as unranked, as rAthena does for a character not on it.
//   - Bonuses that only skills, procs or other systems read. Calc keeps
//     them out of the stats, as rAthena keeps them in fields that stats do
//     not read.
package status

import (
	"fmt"
	"math"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Settings from rAthena's conf/battle files and headers.
const (
	// maxHPLv99, maxHPLv150 and maxHP cap max HP by base level
	// (max_hp_lv99, max_hp_lv150, max_hp).
	maxHPLv99  = 330000
	maxHPLv150 = 660000
	maxHP      = 1100000
	// maxSP is max_sp.
	maxSP = 1000000
	// maxASPD, maxThirdASPD, maxExtendedASPD and maxSummonerASPD are
	// max_aspd and its variants, after battle_adjust_conf turns them into
	// the shortest attack delay: (2000 - aspd * 10) * 2.
	maxASPD         = (amotionZeroASPD - 190*amotionInterval) * amotionDividerPC
	maxThirdASPD    = (amotionZeroASPD - 193*amotionInterval) * amotionDividerPC
	maxExtendedASPD = (amotionZeroASPD - 193*amotionInterval) * amotionDividerPC
	maxSummonerASPD = (amotionZeroASPD - 193*amotionInterval) * amotionDividerPC
	// maxWalkSpeed is max_walk_speed (300) after battle_adjust_conf:
	// 100 * DEFAULT_WALK_SPEED / 300.
	maxWalkSpeed = 100 * defaultWalkSpeed / 300
	// maxDef and overDefBonus are max_def and over_def_bonus: pre-renewal
	// caps equipment DEF and MDEF at 99, and DEF over it adds nothing.
	maxDef       = 99
	overDefBonus = 0
	// maxLevel is MAX_LEVEL.
	maxLevel = 275
	// weaponDefenseType is weapon_defense_type.
	weaponDefenseType = 0

	defaultWalkSpeed = 150
	minWalkSpeed     = 20
	maxWalkSpeedCap  = 1000
	// minASPD is MIN_ASPD, the longest attack delay.
	minASPD          = 8000
	amotionDividerPC = 2
	amotionZeroASPD  = 2000
	amotionInterval  = 10

	// playerMode is the mode status_calc_pc_sub gives players: MD_MASK
	// without MD_ANGRY and MD_TARGETWEAK. The other excluded bits are
	// outside MD_MASK.
	playerMode = 0xffff &^ (0x800 | 0x4000)

	sizeSmall  = 0
	sizeMedium = 1
	// racePlayerHuman is RC_PLAYER_HUMAN.
	racePlayerHuman = 10

	// weaponTypes is MAX_WEAPON_TYPE. Renewal's ASPD table has a shield
	// column after it.
	weaponTypes = 24
	weaponFist  = 0
)

// Skill IDs this package reads, from rAthena's skill_db.yml.
const (
	skillIncCarry          = 36   // MC_INCCARRY
	skillOwlEye            = 43   // AC_OWL
	skillVultureEye        = 44   // AC_VULTURE
	skillMiss              = 49   // TF_MISS
	skillMaceMastery       = 65   // PR_MACEMASTERY
	skillPushCart          = 39   // MC_PUSHCART
	skillHiltBinding       = 105  // BS_HILTBINDING
	skillWeaponResearch    = 107  // BS_WEAPONRESEARCH
	skillSkinTemper        = 109  // BS_SKINTEMPER
	skillTrust             = 248  // CR_TRUST
	skillDodge             = 265  // MO_DODGE
	skillAdvancedBook      = 274  // SA_ADVANCEDBOOK
	skillDragonology       = 284  // SA_DRAGONOLOGY
	skillFreeCast          = 278  // SA_FREECAST
	skillMusicalLesson     = 315  // BA_MUSICALLESSON
	skillDancingLesson     = 323  // DC_DANCINGLESSON
	skillMeditatio         = 363  // HP_MEDITATIO
	skillManaRecharge      = 481  // HP_MANARECHARGE
	skillSoulDrain         = 364  // HW_SOULDRAIN
	skillDevil             = 441  // SG_DEVIL
	skillKaina             = 466  // SL_KAINA
	skillSingleAction      = 509  // GS_SINGLEACTION
	skillRiding            = 63   // KN_RIDING
	skillCavalierMastery   = 64   // KN_CAVALIERMASTERY
	skillSnakeEye          = 510  // GS_SNAKEEYE
	skillAllIncCarry       = 681  // ALL_INCCARRY
	skillEucharistica      = 2049 // AB_EUCHARISTICA
	skillResearchTrap      = 2248 // RA_RESEARCHTRAP
	skillTrainingAxe       = 2276 // NC_TRAININGAXE
	skillLesson            = 2412 // WM_LESSON
	skillBreakthrough      = 5075 // NV_BREAKTHROUGH
	skillTranscendence     = 5077 // NV_TRANSCENDENCE
	skillTwoHandStaff      = 5228 // AG_TWOHANDSTAFF
	skillShadowSense       = 5288 // SHC_SHADOW_SENSE
	skillStageManner       = 5349 // TR_STAGE_MANNER
	skillPFI               = 5401 // NW_P_F_I
	skillGrenadeMastery    = 5402 // NW_GRENADE_MASTERY
	skillTalismanMastery   = 5416 // SOA_TALISMAN_MASTERY
	skillSoulMastery       = 5417 // SOA_SOUL_MASTERY
	skillMysticalCreatureM = 5433 // SH_MYSTICAL_CREATURE_MASTERY
	skillSelfStudyTactics  = 5449 // HN_SELFSTUDY_TATICS
	skillSelfStudySorcery  = 5450 // HN_SELFSTUDY_SOCERY
	skillWarBookMastery    = 5464 // SKE_WAR_BOOK_MASTERY
)

// Weapon types this package reads, as rAthena's weapon_type.
const (
	weaponDagger   = 1
	weapon1HAxe    = 6
	weapon2HAxe    = 7
	weaponMace     = 8
	weapon2HMace   = 9
	weaponBow      = 11
	weaponMusical  = 13
	weaponWhip     = 14
	weaponBook     = 15
	weaponKatar    = 16
	weaponRevolver = 17
	weaponRifle    = 18
	weaponGatling  = 19
	weaponShotgun  = 20
	weaponGrenade  = 21
	weapon2HStaff  = 23
)

// Elements and races this package reads, as rAthena's e_element and
// e_race.
const (
	eleNeutral = 0
	eleFire    = 3
	eleHoly    = 6
	eleDark    = 7
	raceDemon  = 6
	raceDragon = 9
)

// Stats are the six stats a player raised with status points, as
// rAthena's mmo_charstatus str to luk. They do not include job bonuses.
type Stats struct {
	Str, Agi, Vit, Int, Dex, Luk int
}

// Character is what a player's stats come from.
type Character struct {
	// Job is the job ID, as rAthena's e_job.
	Job int
	// Male is the character's sex. It picks the job whose bonus stats and
	// weight apply to Bard and Dancer style jobs, as pc_mapid2jobid.
	Male                bool
	BaseLevel, JobLevel int
	Stats               Stats
	// Skills maps a learned skill ID to its level, as pc_checkskill
	// reads.
	Skills map[int]int
	// Deaths is die_counter. A Super Novice who never died gets +10 to
	// every stat.
	Deaths int
	// Equipment is what the player wears, or nil for nothing.
	Equipment *Equipment
	// Changes are the player's active statuses, its status_change, or
	// nil for none.
	Changes *sc.List
	// Riding is pc_isriding: the player rides a Peco Peco (OPTION_RIDING).
	// It slows attacks less with Cavalier Mastery, carries more with
	// Riding, walks faster and adds to Spear Mastery.
	Riding bool
	// Cart is pc_iscarton: the player pushes a cart, which slows it
	// down unless Pushcart is at level 10.
	Cart bool
	// StatusBonuses are the bonuses of active statuses: the scripts of
	// status.yml and the bonuses of mod statuses, in status order, as
	// status_calc_pc_sub runs status scripts. TimedBonuses are the timed
	// bonuses, in the order they started (pc_bonus_script).
	StatusBonuses, TimedBonuses []Bonus
	// Casting is set while the player casts a skill
	// (ud.skilltimer != INVALID_TIMER), which Free Cast reads.
	Casting bool
}

// Result is a player's stats.
type Result struct {
	// Base is base_status: stats before status changes.
	Base battle.Status
	// Battle is battle_status, what attacks use. Its Player is set, with
	// the character's skills, weapons and battle bonuses.
	Battle battle.Status
	// MaxWeight is max_weight, in tenths of a weight unit.
	MaxWeight int
	// Window is what rAthena's status window shows.
	Window Window
	// Bonuses are the equipment bonuses that systems other than attacks
	// read.
	Bonuses Bonuses
}

// Bonuses are equipment bonuses outside the battle status.
type Bonuses struct {
	// HPRecoverRate and SPRecoverRate are hprecov_rate and
	// sprecov_rate: natural regen in percent, 100 by default.
	HPRecoverRate, SPRecoverRate int
	// NoHPRegen and NoSPRegen are bNoRegen: natural regen is off.
	NoHPRegen, NoSPRegen bool
	// ResEff is reseff: the resistance to each status, per 10,000.
	ResEff map[int]int
	// AlwaysEndure is bNoWalkDelay.
	AlwaysEndure bool
	// AddEff, AddEffWhenHit and AddEffOnSkill are addeff, addeff_atked
	// and addeff_onskill: the statuses that attacks may cause.
	AddEff, AddEffWhenHit, AddEffOnSkill []AddEffect
	// Regen are hp_regen, sp_regen, percent_hp_regen, percent_sp_regen,
	// hp_loss and sp_loss: HP and SP gained or lost over time.
	Regen []RegenBonus
	// AutoSpell, AutoSpellWhenHit and AutoSpellOnSkill are autospell,
	// autospell2 and autospell3: the skills that attacks, hits taken and
	// skills used may cast (bAutoSpell, bAutoSpellWhenHit and
	// bAutoSpellOnSkill).
	AutoSpell, AutoSpellWhenHit, AutoSpellOnSkill []AutoSpell
	// NoKnockback is special_state.no_knockback (bNoKnockback): skills
	// cannot knock the player back.
	NoKnockback bool
	// Unstripable is bonus.unstripable_equip: the EQP_ positions that
	// Divest skills cannot take off.
	Unstripable uint32
	// AddStealRate is bonus.add_steal_rate (bAddStealRate): the steal
	// chance rises by this much, per 10,000.
	AddStealRate int
	// AddHealRate is bonus.add_heal_rate (bHealPower): the player's heal
	// skills heal this percent more. AddHeal2Rate is
	// bonus.add_heal2_rate (bHealPower2): heals the player receives from
	// skills heal this percent more.
	AddHealRate, AddHeal2Rate int
}

// AutoSpell is s_autospell: a skill that a trigger may cast.
type AutoSpell struct {
	// Skill and Level are the skill cast and its level.
	Skill, Level int
	// Rate is the chance, per 1,000, from -1,000 to 1,000.
	Rate int
	// BattleFlag holds the BF_ bits of the hits that trigger it: a kind
	// (BF_WEAPON, BF_MAGIC or BF_MISC), a range (BF_SHORT, BF_LONG) and
	// an attack type (BF_NORMAL, BF_SKILL). AutoSpellOnSkill entries
	// have none.
	BattleFlag int
	// Flag holds rAthena's AUTOSPELL_FORCE_ bits.
	//   - AutoSpellForceTarget: in AutoSpell and AutoSpellWhenHit, the
	//     skill is cast on the enemy (the target hit, or the attacker).
	//     Without it, it is cast on the player. In AutoSpellOnSkill the
	//     bit means the reverse: the skill is cast on the player, and
	//     without it on the target of the trigger skill.
	//   - AutoSpellForceRandomLevel: the level is a random one from 1 to
	//     Level.
	Flag int
	// TriggerSkill is the skill whose use triggers an AutoSpellOnSkill
	// entry.
	TriggerSkill int
	// Card is the card whose script added the entry, or 0 for an item's
	// script. Entries of the same card do not stack (autospell_stacking
	// is off).
	Card int
}

// AUTOSPELL_FORCE_ bits of AutoSpell.Flag, as rAthena's
// e_autospell_flags.
const (
	AutoSpellForceTarget      = 0x1
	AutoSpellForceRandomLevel = 0x2
)

// AddEffect is s_addeffect: a status an attack may cause.
type AddEffect struct {
	Status int
	// Rate and ArrowRate are per 10,000; ArrowRate adds to ranged
	// attacks.
	Rate, ArrowRate int
	// Flag holds rAthena's ATF_ bits.
	Flag int
	// Duration is the duration in milliseconds.
	Duration int
	// Skill is the skill of an addeff_onskill entry.
	Skill int
}

// ATF_ bits of AddEffect, as rAthena's e_additional_effect.
const (
	ATFSelf   = 0x01
	ATFTarget = 0x02
	ATFShort  = 0x04
	ATFLong   = 0x08
	ATFWeapon = 0x10
	ATFMagic  = 0x20
	ATFMisc   = 0x40
)

// RegenBonus is s_regen: an amount every Rate milliseconds.
type RegenBonus struct {
	// Kind is hp_gain_over_time, sp_gain_over_time, hp_loss_over_time or
	// sp_loss_over_time. Percent is set for a percent of max HP or SP.
	Kind    string
	Percent bool
	Value   int
	Rate    int
}

// Window holds the two numbers rAthena's status window shows for ATK,
// MATK, DEF and MDEF, from the pc_leftside_* and pc_rightside_* macros
// that clif_updatestatus sends. In renewal the window shows them as
// "left + right". In pre-renewal it shows ATK, DEF and MDEF as
// "left + right" and MATK as the range "left ~ right".
type Window struct {
	ATK, ATK2   int
	MATK, MATK2 int
	DEF, DEF2   int
	MDEF, MDEF2 int
}

// Tables are the job tables of one rAthena mode.
type Tables struct {
	renewal   bool
	jobs      map[int]*job
	mapIDJobs map[uint64][2]int
	refine    *refineTable
	// renewalASPD is the aspd ruleset flag: renewal's ASPD formula, shield
	// penalty and bonus stacking (RENEWAL_ASPD). aspdBase holds the ASPD
	// table of the flag's mode by job, when it differs from the base's.
	renewalASPD bool
	aspdBase    map[int][]int
	// preRenewalSizes is the size_table ruleset flag set to pre_renewal:
	// attacks use pre-renewal's size table for knuckles and whips.
	preRenewalSizes bool
	// statuses is the status database, for the default duration of
	// cause_status. Without it, that duration is 0.
	statuses *sc.DB
	// autoSpellSelf picks the target of an autospell that names none.
	// Without it, such an autospell targets the enemy.
	autoSpellSelf func(skill int) bool
}

// SetAutoSpellSelf sets how a "cast" bonus without a target option picks
// one, as rAthena's bAutoSpell with no flag: self reports whether a
// skill is a support skill, or a self skill without NoTargetSelf
// (skill_get_inf and INF2_NOTARGETSELF). Such skills are cast on the
// player, and others on the enemy.
func (t *Tables) SetAutoSpellSelf(self func(skill int) bool) { t.autoSpellSelf = self }

// SetStatuses sets the status database that bonuses read.
func (t *Tables) SetStatuses(db *sc.DB) { t.statuses = db }

// SetSizeTable sets the size_table ruleset flag: pre-renewal's size
// table, or renewal's.
func (t *Tables) SetSizeTable(preRenewal bool) { t.preRenewalSizes = preRenewal }

// New builds the tables from job_stats.json.
func New(d *Data) (*Tables, error) {
	var renewal bool
	switch d.Mode {
	case "renewal":
		renewal = true
	case "pre-renewal":
	default:
		return nil, fmt.Errorf("status: unknown mode %q", d.Mode)
	}
	t := &Tables{renewal: renewal, renewalASPD: renewal, jobs: map[int]*job{}, mapIDJobs: map[uint64][2]int{}}
	for i := range d.Jobs {
		t.jobs[d.Jobs[i].ID] = newJob(&d.Jobs[i], renewal)
	}
	for _, m := range d.MapIDJobs {
		t.mapIDJobs[uint64(m.MapID)] = [2]int{m.Female, m.Male} // #nosec G115 -- map job IDs are positive.
	}
	return t, nil
}

// SetASPD sets the aspd ruleset flag. other is job_stats.json of the
// other mode, whose ASPD table the flag needs when it differs from the
// base. It is not read when the flag matches the base.
func (t *Tables) SetASPD(renewal bool, other *Data) error {
	t.renewalASPD = renewal
	t.aspdBase = nil
	if renewal == t.renewal {
		return nil
	}
	if other == nil || (other.Mode == "renewal") != renewal {
		return fmt.Errorf("status: the aspd flag needs the job table of its own mode")
	}
	t.aspdBase = map[int][]int{}
	for i := range other.Jobs {
		t.aspdBase[other.Jobs[i].ID] = other.Jobs[i].ASPDBase
	}
	return nil
}

// Renewal reports whether the tables are renewal's.
func (t *Tables) Renewal() bool { return t.renewal }

// data is rAthena's status_data with the fields battle.Status lacks.
type data struct {
	battle.Status
	pow, sta, wis, spl, con, crt int
	patk, smatk, hplus, crate    int
	aspdRate                     int
}

// pc is the part of map_session_data this package reads.
type pc struct {
	t     *Tables
	c     *Character
	job   *job
	mapID uint64
	// statsJob is job_db.find(pc_mapid2jobid(class_, sex)), which gives
	// job bonuses and weight.
	statsJob *job
	// eq is the equipment, never nil. worn is its weapon types.
	eq   *Equipment
	worn item.Worn
	// b holds the bonuses of the equipment's scripts.
	b *pcBonus
	// granted are the skills item scripts grant, by ID, as rAthena's
	// skill command with ADDSKILL_TEMP.
	granted map[int]int
}

// skill is pc_checkskill: the learned level, or the level an item grants
// when it is higher.
func (p *pc) skill(id int) int { return max(p.c.Skills[id], p.granted[id]) }

// noEquipment is a player with nothing equipped.
var noEquipment = func() *Equipment {
	e := &Equipment{}
	for i := range e.Worn.Index {
		e.Worn.Index[i] = -1
	}
	return e
}()

// Calc is status_calc_pc with SCO_FORCE: status_calc_pc_sub fills the
// base status, then status_calc_bl_main makes the battle status with no
// status changes.
func (t *Tables) Calc(c Character) (Result, error) {
	j, err := t.job(c.Job)
	if err != nil {
		return Result{}, err
	}
	p := &pc{t: t, c: &c, job: j, mapID: j.mapID, eq: c.Equipment}
	if p.eq == nil {
		p.eq = noEquipment
	}
	p.worn = p.eq.Worn
	if p.mapID&mapidFirstMask == mapidSummoner {
		return Result{}, fmt.Errorf("status: job %d is a summoner, which is not ported", c.Job)
	}
	statsID, ok := t.mapIDToJob(p.mapID, c.Male)
	if !ok {
		return Result{}, fmt.Errorf("status: job %d has no map job ID", c.Job)
	}
	if p.statsJob, err = t.job(statsID); err != nil {
		return Result{}, err
	}
	if c.BaseLevel < 1 || c.BaseLevel > maxLevel || c.JobLevel < 1 || c.JobLevel > len(p.statsJob.bonus) {
		return Result{}, fmt.Errorf("status: job %d cannot have base level %d and job level %d", c.Job, c.BaseLevel, c.JobLevel)
	}
	base := p.calcBase()
	bat := p.calcBattle(&base)
	res := Result{Base: base.Status, Battle: bat.Status, MaxWeight: p.maxWeight(), Window: p.window(&bat)}
	res.Base.Level, res.Battle.Level = c.BaseLevel, c.BaseLevel
	res.Base.ID, res.Battle.ID = c.Job, c.Job
	res.Base.CRate, res.Battle.CRate = base.crate, bat.crate
	res.Base.SMatk, res.Battle.SMatk = base.smatk, bat.smatk
	res.Base.Spl, res.Battle.Spl = base.spl, bat.spl
	res.Base.HPlus, res.Battle.HPlus = base.hplus, bat.hplus
	res.Base.PAtk, res.Battle.PAtk = base.patk, bat.patk
	res.Battle.Player = p.b.player
	res.Battle.Changes = BattleChanges(c.Changes)
	res.Bonuses = Bonuses{HPRecoverRate: p.b.hprecovRate, SPRecoverRate: p.b.sprecovRate, NoHPRegen: p.b.noHPRegen, NoSPRegen: p.b.noSPRegen,
		ResEff: p.b.resEff, AlwaysEndure: p.b.noWalkDelay, AddEff: p.b.addEff, AddEffWhenHit: p.b.addEffWhenHit,
		AddEffOnSkill: p.b.addEffOnSkill, Regen: p.b.regen, AutoSpell: p.b.autoSpell, AutoSpellWhenHit: p.b.autoSpellWhenHit,
		AutoSpellOnSkill: p.b.autoSpellOnSkill, NoKnockback: p.b.noKnockback, Unstripable: p.b.unstripable, AddStealRate: p.b.addStealRate,
		AddHealRate: p.b.addHealRate, AddHeal2Rate: p.b.addHeal2Rate}
	return res, nil
}

// lr flags, as rAthena's e_lr_flag: which hand a script's bonuses go to.
const (
	lrNone = iota
	lrWeapon
	lrShield
	lrArrow
)

// calcBase is status_calc_pc_sub for a player with no status changes.
func (p *pc) calcBase() data {
	var b data
	c := p.c
	renewal := p.t.renewal
	b.Speed = defaultWalkSpeed
	b.Mode = playerMode
	b.Size = sizeMedium
	if p.mapID&joblBaby != 0 {
		b.Size = sizeSmall
	}
	b.aspdRate = 1000
	b.EleLv = 1
	b.Race = racePlayerHuman

	p.b = newPCBonus(p)
	pb := p.b
	pb.plan(p.eq)

	// Parse equipment: each item's DEF, weapon ATK and refine, then its
	// script.
	refinedef := 0
	for i := range item.EqiMax {
		e := p.eq.Items[i]
		if e == nil || i == item.EqiAmmo || p.sameEquipIndex(i) {
			continue
		}
		d := e.Def
		p.addDef(&b.Def, d.Defense)
		refine := min(e.Refine, p.t.MaxRefine())
		info := p.t.currentRefine(d, refine)
		switch {
		case d.IsWeapon():
			wlv := min(d.WeaponLevel, maxWeaponLevel(renewal))
			left := i == item.EqiHandL && e.Position == item.EqpHandL
			wa, wd := &b.RHW, &pb.player.Right
			if left {
				wa, wd = &b.LHW, &pb.player.Left
			}
			wa.Atk += d.Atk
			if info != nil {
				wa.Atk2 += info.bonus / 100
				if renewal && wlv == 5 {
					b.patk += refine * 2
					b.smatk += refine * 2
				}
			}
			if renewal {
				if pb.weaponAtkRate != 0 {
					wa.Atk += wa.Atk * pb.weaponAtkRate / 100
				}
				wa.Matk += d.Matk
				wa.Wlv = wlv
				if info != nil && p.worn.WeaponType1 != weaponBow {
					wa.Matk += info.bonus / 100
				}
			} else {
				// Pre-renewal's battle_calc_base_damage reads the weapon
				// level of the item in the hand, which server/battle
				// takes from Wlv.
				wa.Wlv = wlv
			}
			if info != nil {
				wd.Overrefine = info.randomBonus / 100
			}
			wa.Range += d.Range
			lr := lrNone
			if left {
				lr = lrWeapon
			}
			if !p.restricted(d) {
				pb.run(e.Bonuses, lr, &b)
			}
			if renewal && pb.weaponMatkRate != 0 {
				wa.Matk += wa.Matk * pb.weaponMatkRate / 100
			}
		case d.IsArmor():
			if info != nil {
				refinedef += info.bonus
				if renewal && d.ArmorLevel == 2 {
					b.Res += refine * 2
					b.MRes += refine * 2
				}
			}
			lr := lrNone
			if i == item.EqiHandL {
				lr = lrShield
			}
			if !p.restricted(d) {
				pb.run(e.Bonuses, lr, &b)
			}
		case d.IsShadowGear():
			if !p.restricted(d) {
				pb.run(e.Bonuses, lrNone, &b)
			}
		}
	}

	// Ammunition: its ATK, then its script on the arrow. Throwing weapons
	// run no script; the caller leaves their bonuses out.
	if e := p.eq.Items[item.EqiAmmo]; e != nil {
		pb.arrowAtk += e.Def.Atk
		pb.run(e.Bonuses, lrArrow, &b)
	}

	// Equipment script stat bonuses are kept apart from cards'.
	pb.paramEquip, pb.paramBonus = pb.paramBonus, traits{}

	p.addDef(&b.Def, (refinedef+50)/100)

	// Cards, by the slot of the item that holds them.
	for i := range item.EqiMax {
		e := p.eq.Items[i]
		if e == nil || i == item.EqiAmmo || p.sameEquipIndex(i) {
			continue
		}
		lr := lrNone
		if i == item.EqiHandL && e.Position == item.EqpHandL {
			lr = lrWeapon
		}
		for _, cd := range e.Cards {
			if !p.restricted(cd.Def) {
				// current_equip_card_id: autospells of one card do not
				// stack.
				if cd.Def != nil {
					pb.cardID = int(cd.Def.ID)
				}
				pb.run(cd.Bonuses, lr, &b)
				pb.cardID = 0
			}
		}
	}

	// Status scripts, then timed bonuses (pc_bonus_script).
	pb.run(c.StatusBonuses, lrNone, &b)
	pb.run(c.TimedBonuses, lrNone, &b)

	// rhw.range and lhw.range become at least 1, and the right hand's
	// range is the longer of the two.
	b.RHW.Range, b.LHW.Range = max(b.RHW.Range, 1), max(b.LHW.Range, 1)
	b.RHW.Range = max(b.RHW.Range, b.LHW.Range)

	pb.doubleRate += pb.doubleAddRate
	pb.perfectHit += pb.perfectHitAdd

	// Job bonuses.
	bonus := p.statsJob.bonus[c.JobLevel-1]
	b.Str, b.Agi, b.Vit, b.Int, b.Dex, b.Luk = bonus[paramStr], bonus[paramAgi], bonus[paramVit], bonus[paramInt], bonus[paramDex], bonus[paramLuk]
	b.pow, b.sta, b.wis, b.spl, b.con, b.crt = bonus[paramPow], bonus[paramSta], bonus[paramWis], bonus[paramSpl], bonus[paramCon], bonus[paramCrt]

	// A Super Novice who never died, at job level 70 or a third job.
	if p.mapID&mapidSecondMask == mapidSuperNovice && (c.JobLevel >= 70 || p.mapID&joblThird != 0) && c.Deaths == 0 {
		b.Str += 10
		b.Agi += 10
		b.Vit += 10
		b.Int += 10
		b.Dex += 10
		b.Luk += 10
	}

	// Absolute modifiers from passive skills.
	if p.skill(skillHiltBinding) > 0 {
		b.Str++
	}
	if lv := p.skill(skillDragonology); lv > 0 {
		b.Int += (lv + 1) / 2
	}
	b.Dex += p.skill(skillOwlEye)
	b.Int += p.skill(skillResearchTrap)

	// Raised stats and the bonuses of equipment and cards, capped to
	// uint16.
	raised := [6]int{c.Stats.Str, c.Stats.Agi, c.Stats.Vit, c.Stats.Int, c.Stats.Dex, c.Stats.Luk}
	for i, v := range []*int{&b.Str, &b.Agi, &b.Vit, &b.Int, &b.Dex, &b.Luk, &b.pow, &b.sta, &b.wis, &b.spl, &b.con, &b.crt} {
		r := 0
		if i < len(raised) {
			r = raised[i]
		}
		*v = capValue(*v+r+pb.paramBonus[i]+pb.paramEquip[i], 0, math.MaxUint16)
	}

	// bNoWalkDelay shows Endure and adds 1 MDEF.
	if pb.noWalkDelay {
		b.MDef++
	}

	b.con += p.skill(skillGrenadeMastery)
	b.spl += p.skill(skillSoulMastery)

	// Attack. Pre-renewal adds bWeaponAtk of the weapon type to base ATK.
	weapon := p.worn.Weapon
	if !renewal {
		if weapon < weaponTypes {
			b.BAtk += pb.weaponAtk[weapon]
		}
		if p.skill(skillHiltBinding) > 0 {
			b.BAtk += 4
		}
	} else {
		b.WAtk = b.RHW.Atk + b.RHW.Atk2
		b.WAtk2 = b.LHW.Atk + b.LHW.Atk2
		b.EAtk = pb.eatk
	}

	b.MaxHP = p.capMaxHP(p.calcMaxHP(b.Vit))
	b.MaxSP = capValue(p.calcMaxSP(b.Int), 1, maxSP)

	p.calcMisc(&b)

	// Equipment rate modifiers.
	pb.matkRate = max(pb.matkRate, 0)
	if pb.matkRate != 100 {
		b.MAtkMax = b.MAtkMax * pb.matkRate / 100
		b.MAtkMin = b.MAtkMin * pb.matkRate / 100
	}
	applyRate(&b.Hit, &pb.hitRate)
	applyRate(&b.Flee, &pb.fleeRate)
	applyRate(&b.Def2, &pb.def2Rate)
	applyRate(&b.MDef2, &pb.mdef2Rate)
	pb.criticalRate = max(pb.criticalRate, 0)
	if pb.criticalRate != 100 {
		b.Cri = capValue(b.Cri*pb.criticalRate/100, math.MinInt16, math.MaxInt16)
	}
	applyRate(&b.Flee2, &pb.flee2Rate)
	applyRate(&b.patk, &pb.patkRate)
	applyRate(&b.smatk, &pb.smatkRate)
	applyRate(&b.Res, &pb.resRate)
	applyRate(&b.MRes, &pb.mresRate)
	applyRate(&b.hplus, &pb.hplusRate)
	applyRate(&b.crate, &pb.crateRate)

	// Hit.
	if !renewal {
		b.Hit += p.skill(skillWeaponResearch) * 2
	}
	if lv := p.skill(skillVultureEye); lv > 0 {
		if !renewal {
			b.Hit += lv
		}
		if weapon == weaponBow {
			b.RHW.Range += lv
		}
	}
	if weapon >= weaponRevolver && weapon <= weaponGrenade {
		b.Hit += 2 * p.skill(skillSingleAction)
		if lv := p.skill(skillSnakeEye); lv > 0 {
			b.Hit += lv
			b.RHW.Range += lv
		}
	}
	if lv := p.skill(skillTrainingAxe); lv > 0 {
		switch weapon {
		case weapon1HAxe, weapon2HAxe:
			b.Hit += lv * 3
		case weaponMace, weapon2HMace:
			b.Hit += lv * 2
		}
	}
	if weapon == weaponBook {
		b.Hit += p.skill(skillWarBookMastery) * 3
	}

	// Flee.
	if lv := p.skill(skillMiss); lv > 0 {
		if p.mapID&joblSecond != 0 && p.mapID&mapidFirstMask == mapidThief {
			b.Flee += lv * 4
		} else {
			b.Flee += lv * 3
		}
	}
	b.Flee += p.skill(skillDodge) * 3 / 2
	b.Flee += p.skill(skillShadowSense) * 10

	// Critical.
	if renewal {
		b.Cri += p.skill(skillDancingLesson) * 10
		if weapon == weaponMace || weapon == weapon2HMace {
			b.Cri += p.skill(skillMaceMastery) * 10
		}
	}
	if lv := p.skill(skillShadowSense); lv > 0 {
		switch weapon {
		case weaponDagger, item.WeaponDoubleDD, item.WeaponDoubleDS, item.WeaponDoubleDA:
			b.Cri += 100 + lv*40
		case weaponKatar:
			b.Cri += 50 + lv*20
		}
	}

	// P.Atk and S.Matk.
	if lv := p.skill(skillStageManner); lv > 0 && (weapon == weaponBow || weapon == weaponMusical || weapon == weaponWhip) {
		b.patk += lv * 3
		b.smatk += lv * 3
	}
	b.patk += p.skill(skillSelfStudyTactics)
	b.smatk += p.skill(skillSelfStudySorcery)
	if lv := p.skill(skillPFI); lv > 0 && weapon >= weaponRevolver && weapon <= weaponGrenade {
		b.patk += lv + 2
	}
	b.smatk += p.skill(skillTalismanMastery)
	if lv := p.skill(skillWarBookMastery); lv > 0 && weapon == weaponBook {
		b.patk += lv + 2
	}
	if weapon == weapon2HStaff {
		b.smatk += p.skill(skillTwoHandStaff) * 2
	}
	if lv := p.skill(skillMysticalCreatureM); lv > 0 {
		b.smatk += lv * 15 / 10
		b.patk += lv * 15 / 10
	}

	// Equipment DEF and MDEF rates, and pre-renewal's cap.
	pb.defRate = max(pb.defRate, 0)
	if pb.defRate != 100 {
		b.Def = p.capDef(b.Def * pb.defRate / 100)
	}
	if !renewal && weaponDefenseType == 0 && b.Def > maxDef {
		b.Def2 += overDefBonus * (b.Def - maxDef)
		b.Def = maxDef
	}
	pb.mdefRate = max(pb.mdefRate, 0)
	if pb.mdefRate != 100 {
		b.MDef = p.capDef(b.MDef * pb.mdefRate / 100)
	}
	if !renewal && b.MDef > maxDef {
		b.MDef2 += overDefBonus * (b.MDef - maxDef)
		b.MDef = maxDef
	}

	// ASPD.
	i := p.baseAmotion(&b)
	if p.t.renewalASPD {
		// Renewal's base value is ASPD, not amotion.
		i = amotionZeroASPD - i*amotionInterval
	}
	b.AMotion = capValue(i, p.maxASPD()/amotionDividerPC, minASPD/amotionDividerPC)
	if !p.t.renewalASPD {
		if lv := p.skill(skillAdvancedBook); lv > 0 && weapon == weaponBook {
			b.aspdRate -= 5 * lv
		}
		if lv := p.skill(skillDevil); lv > 0 && (p.mapID&mapidThirdMask == mapidStarEmperor || c.JobLevel >= p.job.maxJobLevel) {
			b.aspdRate -= 30 * lv
		}
		if lv := p.skill(skillSingleAction); lv > 0 && weapon >= weaponRevolver && weapon <= weaponGrenade {
			b.aspdRate -= (lv + 1) / 2 * 10
		}
		if c.Riding {
			b.aspdRate += 500 - 100*p.skill(skillCavalierMastery)
		}
		b.aspdRate += pb.aspdRate
	}
	b.ADelay = amotionDividerPC * b.AMotion

	b.DMotion = capValue(800-b.Agi*4, 400, 800)

	pl := pb.player
	// Skill SP cost: Mana Recharge lowers dsprate by 4% per level, which
	// does not go below 0.
	if lv := p.skill(skillManaRecharge); lv > 0 {
		pl.Cast.SPRate -= 4 * lv
	}
	pl.Cast.SPRate -= c.Changes.Val3(sc.SCService4u)
	pl.Cast.SPRate = max(pl.Cast.SPRate, 0)

	// Element and race resistance and damage from skills.
	if lv := p.skill(skillTrust); lv > 0 {
		battle.Add(&pl.SubEle, eleHoly, lv*5)
	}
	if lv := p.skill(skillSkinTemper); lv > 0 {
		battle.Add(&pl.SubEle, eleNeutral, lv)
		battle.Add(&pl.SubEle, eleFire, lv*5)
	}
	if lv := p.skill(skillDragonology); lv > 0 {
		// left_cardfix_to_right is on, so only the right hand gets it.
		battle.Add(&pl.Right.AddRace, raceDragon, lv*4)
		battle.Add(&pl.Magic.AddRace, raceDragon, lv*2)
		battle.Add(&pl.SubRace, raceDragon, lv*4)
	}
	if lv := p.skill(skillEucharistica); lv > 0 {
		battle.Add(&pl.Right.AddRace, raceDemon, lv)
		battle.Add(&pl.Right.AddEle, eleDark, lv)
		battle.Add(&pl.SubRace, raceDemon, lv)
		battle.Add(&pl.SubEle, eleDark, lv)
	}

	// The player's remaining bonuses for attacks.
	pl.Skills = c.Skills
	if len(p.granted) > 0 {
		pl.Skills = map[int]int{}
		for id, lv := range c.Skills {
			pl.Skills[id] = lv
		}
		for id, lv := range p.granted {
			if lv > pl.Skills[id] {
				pl.Skills[id] = lv
				if pl.Granted == nil {
					pl.Granted = map[int]bool{}
				}
				pl.Granted[id] = true
			}
		}
	}
	pl.AtkRate = pb.atkRate
	pl.PerfectHit = pb.perfectHit
	pl.DoubleRate = pb.doubleRate
	pl.Weapon, pl.RightWeapon, pl.LeftWeapon = weapon, p.worn.WeaponType1, p.worn.WeaponType2
	pl.AmmoAtk, pl.AmmoEle = pb.arrowAtk, pb.arrowEle
	pl.PreRenewalSizes = p.t.preRenewalSizes
	pl.Riding = c.Riding
	pl.JobLevel = c.JobLevel
	if e := p.eq.Items[item.EqiHandR]; e != nil && e.Def != nil && e.Def.IsWeapon() {
		pl.WeaponWeight = e.Def.Weight
	}
	// The shield that Shield Boomerang and Shield Chain read.
	if e := p.eq.Items[item.EqiHandL]; e != nil && e.Def != nil && e.Def.IsArmor() {
		pl.Shield, pl.ShieldWeight, pl.ShieldRefine = true, e.Def.Weight, e.Refine
	}
	// The statuses at the end of status_calc_pc_sub that change bonuses.
	if l := c.Changes; !l.Empty() {
		if e := l.Get(sc.SCConcentrate); e != nil {
			// Improve Concentration leaves out the AGI and DEX of cards.
			e.Val3 = pb.paramBonus[paramAgi]
			e.Val4 = pb.paramBonus[paramDex]
		}
		if e := l.Get(sc.SCSiegfried); e != nil {
			// Siegfried's element resistance: the four elements, and in
			// pre-renewal five more.
			eles := []int{eleWater, eleEarth, eleFire, eleWind}
			if !p.t.renewal {
				eles = append(eles, elePoison, eleHoly, eleDark, eleGhost, eleUndead)
			}
			for _, ele := range eles {
				battle.Add(&pl.SubEle, ele, e.Val2)
			}
		}
		if e := l.Get(sc.SCFortune); e != nil && p.t.renewal {
			pl.CritAtkRate += 2 * e.Val1
		}
		if e := l.Get(sc.SCBasilica); e != nil && p.t.renewal {
			// Renewal's Basilica: 5% more damage per level against the
			// dark and undead elements, on the right hand only
			// (left_cardfix_to_right is on), and 3% more holy magic per
			// level.
			battle.Add(&pl.Right.AddEle, eleDark, e.Val1*5)
			battle.Add(&pl.Right.AddEle, eleUndead, e.Val1*5)
			battle.Add(&pl.Magic.AtkEle, eleHoly, e.Val1*3)
		}
		if p.t.renewal {
			// Endows raise magic of their element.
			battle.Add(&pl.Magic.AtkEle, eleFire, l.Val1(sc.SCFireweapon))
			battle.Add(&pl.Magic.AtkEle, eleWind, l.Val1(sc.SCWindweapon))
			battle.Add(&pl.Magic.AtkEle, eleWater, l.Val1(sc.SCWaterweapon))
			battle.Add(&pl.Magic.AtkEle, eleEarth, l.Val1(sc.SCEarthweapon))
		}
		pb.matkRate += l.Val1(sc.SCIncmatkrate)
		pb.matkRate += l.Val2(sc.SCMindbreaker)
		if e := l.Get(sc.SCProvidence); e != nil {
			// Resistant Souls: holy and demon resistance.
			battle.Add(&pl.SubEle, eleHoly, e.Val2)
			battle.Add(&pl.SubRace, battle.RaceDemon, e.Val2)
		}
		if l.Val2(sc.SCNibelungen) == ringMatkRate {
			pb.matkRate += 20
		}
	}
	return b
}

// sameEquipIndex is pc_is_same_equip_index: an item in two positions
// is parsed once, at the later index.
func (p *pc) sameEquipIndex(i int) bool {
	idx := &p.eq.Items
	same := func(j int) bool { return idx[j] != nil && idx[j] == idx[i] }
	switch i {
	case item.EqiHandR:
		return same(item.EqiHandL)
	case item.EqiHeadMid:
		return same(item.EqiHeadLow)
	case item.EqiHeadTop:
		return same(item.EqiHeadMid) || same(item.EqiHeadLow)
	case item.EqiCostumeHeadMid:
		return same(item.EqiCostumeHeadLow)
	case item.EqiCostumeHeadTop:
		return same(item.EqiCostumeHeadMid) || same(item.EqiCostumeHeadLow)
	}
	return false
}

// maxWeaponLevel is MAX_WEAPON_LEVEL.
func maxWeaponLevel(renewal bool) int {
	if renewal {
		return 5
	}
	return 4
}

// addDef adds to DEF or MDEF as rAthena's defType: int16 in renewal, and
// a signed char in pre-renewal, which wraps.
func (p *pc) addDef(v *int, n int) {
	if p.t.renewal {
		*v = int(int16(*v + n)) // #nosec G115 -- defType is int16 in renewal.
		return
	}
	*v = int(int8(*v + n)) // #nosec G115 -- defType is a signed char in pre-renewal.
}

// capDef caps DEF or MDEF to DEFTYPE_MIN and DEFTYPE_MAX. Pre-renewal's
// limits are CHAR_MIN and CHAR_MAX. The reference was recorded on arm64
// Linux, where char is unsigned, so they are 0 and 255 there, and the
// value is then stored in a signed char. wAthena follows the recording on
// every platform, as it does for the fused multiply of calcBaseHP.
func (p *pc) capDef(v int) int {
	if p.t.renewal {
		return capValue(v, math.MinInt16, math.MaxInt16)
	}
	return int(int8(capValue(v, 0, math.MaxUint8))) // #nosec G115 -- defType is a signed char in pre-renewal.
}

// applyRate is one of status_calc_pc_sub's "Equipment modifiers for misc
// settings": a rate below 0 becomes 0, and a rate other than 100 scales
// the stat.
func applyRate(v, rate *int) {
	*rate = max(*rate, 0)
	if *rate != 100 {
		*v = *v * *rate / 100
	}
}

// calcMisc is status_calc_misc for a player.
func (p *pc) calcMisc(b *data) {
	level := p.c.BaseLevel
	if p.t.renewal {
		b.Hit = capValue(b.Hit+level+b.Dex+b.Luk/3+175+2*b.con, 1, math.MaxInt16)
		b.Flee = capValue(b.Flee+level+b.Agi+b.Luk/5+100+2*b.con, 1, math.MaxInt16)
		b.Def2 = capValue(b.Def2+int((float32(level)+float32(b.Vit))/2+float32(b.Agi)/5), 0, math.MaxInt16)
		b.MDef2 = capValue(b.MDef2+int(float32(b.Int)+float32(level)/4+float32(b.Dex+b.Vit)/5), 0, math.MaxInt16)
		b.patk = capValue(b.patk+b.pow/3+b.con/5, 0, math.MaxInt16)
		b.smatk = capValue(b.smatk+b.spl/3+b.con/5, 0, math.MaxInt16)
		b.Res = capValue(b.Res+b.sta+b.sta/3*5, 0, math.MaxInt16)
		b.MRes = capValue(b.MRes+b.wis+b.wis/3*5, 0, math.MaxInt16)
		b.hplus = capValue(b.hplus+b.crt, 0, math.MaxInt16)
		b.crate = capValue(b.crate+b.crt/3, 0, math.MaxInt16)
		b.MAtkMin = p.baseMatk(b)
		b.MAtkMax = p.baseMatk(b)
	} else {
		b.MAtkMin = int(uint16(b.Int + (b.Int/7)*(b.Int/7))) // #nosec G115 -- status_base_matk_min returns uint16.
		b.MAtkMax = int(uint16(b.Int + (b.Int/5)*(b.Int/5))) // #nosec G115 -- status_base_matk_max returns uint16.
		b.Hit = capValue(b.Hit+level+b.Dex, 1, math.MaxInt16)
		b.Flee = capValue(b.Flee+level+b.Agi, 1, math.MaxInt16)
		b.Def2 = capValue(b.Def2+b.Vit, 0, math.MaxInt16)
		b.MDef2 = capValue(b.MDef2+b.Int+b.Vit/2, 0, math.MaxInt16)
	}
	// enable_critical and enable_perfect_flee include players.
	if p.t.renewal {
		b.Cri = capValue(b.Cri+level/10+10+b.Luk*3, 1, math.MaxInt16)
	} else {
		b.Cri = capValue(b.Cri+10+b.Luk*10/3, 1, math.MaxInt16)
	}
	b.Flee2 = capValue(b.Flee2+b.Luk+10, 0, math.MaxInt16)
	b.BAtk += p.baseAtk(b)
}

// baseMatk is status_base_matk_min and status_base_matk_max for a
// player in renewal. Both are the same.
func (p *pc) baseMatk(b *data) int {
	return int(uint16(b.Int + b.Int/2 + b.Dex/5 + b.Luk/3 + p.c.BaseLevel/4 + 5*b.spl)) // #nosec G115 -- rAthena returns uint16.
}

// rangedWeapon reports whether a weapon type uses DEX for base ATK and
// ASPD, as status_base_atk and status_base_amotion_pc check.
func rangedWeapon(weapon int) bool {
	switch weapon {
	case weaponBow, weaponMusical, weaponWhip, weaponRevolver, weaponRifle, weaponGatling, weaponShotgun, weaponGrenade:
		return true
	}
	return false
}

// baseAtk is status_base_atk for a player. Ranged weapons swap STR and
// DEX.
func (p *pc) baseAtk(b *data) int {
	str, dex := b.Str, b.Dex
	if rangedWeapon(p.worn.Weapon) {
		str, dex = b.Dex, b.Str
	}
	var v int
	if p.t.renewal {
		v = (str*10+dex*10/5+b.Luk*10/3+p.c.BaseLevel*10/4)/10 + 5*b.pow
	} else {
		dstr := str / 10
		v = str + dstr*dstr + dex/5 + b.Luk/5
	}
	return capValue(v, 0, math.MaxUint16)
}

// aspdTable returns the job's ASPD table for the aspd flag's mode.
func (p *pc) aspdTable() []int {
	if p.t.aspdBase != nil {
		return p.t.aspdBase[p.c.Job]
	}
	return p.job.aspdBase
}

// aspdAt returns the table's entry for a weapon type, or 0 when the
// table lacks it.
func aspdAt(table []int, i int) int {
	if i < 0 || i >= len(table) {
		return 0
	}
	return table[i]
}

// baseAmotion is status_base_amotion_pc. With the renewal aspd flag it
// returns ASPD, otherwise amotion.
func (p *pc) baseAmotion(b *data) int {
	table := p.aspdTable()
	w := &p.worn
	if p.t.renewalASPD {
		aspd := aspdAt(table, w.WeaponType1)
		if w.Shield != 0 {
			aspd += aspdAt(table, weaponTypes)
		} else if w.WeaponType2 != weaponFist && w.Index[item.EqiHandR] != w.Index[item.EqiHandL] {
			// Dual wield.
			aspd += aspdAt(table, w.WeaponType2) / 4
		}
		var temp float32
		if rangedWeapon(w.Weapon) {
			temp = float32(b.Dex*b.Dex)/7 + float32(b.Agi*b.Agi)*0.5
		} else {
			temp = float32(b.Dex*b.Dex)/5 + float32(b.Agi*b.Agi)*0.5
		}
		temp = float32(math.Sqrt(float64(temp)))*0.25 + 196
		val := 0
		if lv := p.skill(skillAdvancedBook); lv > 0 && w.Weapon == weaponBook {
			val += (lv-1)/2 + 1
		}
		if lv := p.skill(skillDevil); lv > 0 && (p.mapID&mapidThirdMask == mapidStarEmperor || p.c.JobLevel >= p.job.maxJobLevel) {
			val += 1 + lv
		}
		if lv := p.skill(skillSingleAction); lv > 0 && w.Weapon >= weaponRevolver && w.Weapon <= weaponGrenade {
			val += (lv + 1) / 2
		}
		if p.c.Riding {
			val -= 50 - 10*p.skill(skillCavalierMastery)
		}
		fixed := calcAspdBonus(p.c.Changes, true, true, nil)
		return int(temp+float32(fixed+val)*float32(b.Agi)/200) - min(aspd, 200)
	}
	var amotion int
	if w.Weapon < weaponTypes {
		amotion = aspdAt(table, w.Weapon)
	} else {
		// Dual wield.
		amotion = (aspdAt(table, w.WeaponType1) + aspdAt(table, w.WeaponType2)) * 7 / 10
	}
	amotion -= amotion * (4*b.Agi + b.Dex) / 1000
	// The raw delay of bAspd.
	amotion += p.b.aspdAdd
	return amotion
}

// maxASPD is pc_maxaspd.
func (p *pc) maxASPD() int {
	switch {
	case p.mapID&joblThird != 0:
		return maxThirdASPD
	case p.mapID&mapidSecondMask == mapidKagerouOboro || p.mapID&mapidSecondMask == mapidRebellion:
		return maxExtendedASPD
	case p.mapID&mapidFirstMask == mapidSummoner:
		return maxSummonerASPD
	}
	return maxASPD
}

// calcMaxHP is status_calc_maxhp_pc.
func (p *pc) calcMaxHP(vit int) int {
	dmax := float64(tableAt(p.job.baseHP, p.c.BaseLevel))
	if vit > 0 {
		// Fused as the arm64 recording is. See calcBaseHP.
		dmax *= math.FMA(float64(vit), 0.01, 1.0)
	}
	if p.mapID&joblUpper != 0 || isPrimaryFourth(p.mapID) {
		dmax *= 1.25
	}
	// VIT from equipment scripts gives 1 more HP each.
	dmax += float64(p.b.paramEquip[paramVit])
	dmax += float64(p.hpBonus())
	// status_get_hpbonus_equip: bMaxHPrate. status_get_hpbonus_item has
	// nothing to add.
	dmax += dmax * float64(p.b.hprate-100) / 100
	// status_get_hpbonus with STATUS_BONUS_RATE, from statuses.
	if rate := hpRateSC(p.c.Changes); rate != 0 {
		dmax += float64(int64(dmax * float64(rate) / 100))
	}
	return max(int(uint32(max(dmax, 1))), 1)
}

// tableAt reads a job's HP or SP table at a base level. rAthena sizes the
// tables to MAX_LEVEL while it loads them, then shrinks them to the job's
// maximum base level, so a level past the maximum reads 0.
func tableAt(table []uint32, level int) uint32 {
	if level < 1 || level > len(table) {
		return 0
	}
	return table[level-1]
}

// capMaxHP caps max HP by base level.
func (p *pc) capMaxHP(hp int) int {
	switch {
	case p.c.BaseLevel < 100:
		return capValue(hp, 1, maxHPLv99)
	case p.c.BaseLevel < 151:
		return capValue(hp, 1, maxHPLv150)
	}
	return capValue(hp, 1, maxHP)
}

// hpBonus is status_get_hpbonus with STATUS_BONUS_FIX: bMaxHP, skills
// and statuses.
func (p *pc) hpBonus() int {
	bonus := p.b.hp + hpBonusSC(p.c.Changes, p.t.renewal)
	bonus += p.skill(skillTrust) * 200
	for _, id := range []int{skillBreakthrough, skillTranscendence} {
		if lv := p.skill(id); lv > 0 {
			bonus += 350 * lv
			if lv > 4 {
				bonus += 250
			}
		}
	}
	return bonus
}

// calcMaxSP is status_calc_maxsp_pc.
func (p *pc) calcMaxSP(intel int) int {
	dmax := float64(tableAt(p.job.baseSP, p.c.BaseLevel))
	if intel > 0 {
		// Fused as in calcMaxHP.
		dmax *= math.FMA(float64(intel), 0.01, 1.0)
	}
	if p.mapID&joblUpper != 0 || isPrimaryFourth(p.mapID) {
		dmax *= 1.25
	}
	// INT from equipment scripts gives 1 more SP each.
	dmax += float64(p.b.paramEquip[paramInt])
	dmax += float64(p.spBonus())
	// status_get_spbonus_equip and status_get_spbonus_item.
	equip := dmax * float64(p.b.sprate-100) / 100
	if item := spItemSC(p.c.Changes); item != 0 {
		dmax += equip + dmax*float64(item)/100
	} else {
		dmax += equip
	}
	// "Aegis accuracy": the rate bonus is truncated before it is added.
	dmax += float64(int64(dmax * float64(p.spRate()) / 100))
	return max(int(uint32(max(dmax, 1))), 1)
}

// spBonus is status_get_spbonus with STATUS_BONUS_FIX: bMaxSP and
// skills.
func (p *pc) spBonus() int {
	bonus := p.b.sp
	bonus += p.skill(skillKaina) * 30
	if lv := p.skill(skillResearchTrap); lv > 0 {
		bonus += 200 + 20*lv
	}
	bonus += p.skill(skillLesson) * 30
	for _, id := range []int{skillBreakthrough, skillTranscendence} {
		if lv := p.skill(id); lv > 0 {
			bonus += 30 * lv
			if lv > 4 {
				bonus += 50
			}
		}
	}
	return bonus
}

// spRate is status_get_spbonus with STATUS_BONUS_RATE, from skills.
func (p *pc) spRate() int {
	bonus := p.skill(skillMeditatio) + 2*p.skill(skillSoulDrain)
	if p.t.renewal {
		lesson := skillDancingLesson
		if p.c.Male {
			lesson = skillMusicalLesson
		}
		bonus += p.skill(lesson)
		if p.c.Changes.Val2(sc.SCNibelungen) == ringSPRate {
			bonus += 30
		}
	}
	bonus += spRateSC(p.c.Changes)
	return max(bonus, -100)
}

// maxWeight is status_calc_weight with CALCWT_MAXBONUS. It reads the
// raised STR, not the total.
func (p *pc) maxWeight() int {
	w := p.statsJob.maxWeight + p.c.Stats.Str*300
	w += p.b.addMaxWeight
	w += 2000 * p.skill(skillIncCarry)
	if p.c.Riding && p.skill(skillRiding) > 0 {
		w += 10000
	}
	w += 2000 * p.skill(skillAllIncCarry)
	return w
}

// calcBattle is status_calc_bl_main with every flag, for a player: the
// battle status from the base status and the player's statuses.
func (p *pc) calcBattle(b *data) data {
	s := *b
	pb := p.b
	l := p.c.Changes
	renewal := p.t.renewal

	s.Str = calcStr(l, b.Str)
	s.Agi = calcAgi(l, b.Agi)
	s.Vit = calcVit(l, b.Vit)
	s.Int = calcInt(l, b.Int)
	s.Dex = calcDex(l, b.Dex)
	s.Luk = calcLuk(l, b.Luk)

	// BATK: the difference that equipment made to the base status stays.
	if b.BAtk != 0 {
		s.BAtk = p.baseAtk(&s) + b.BAtk - p.baseAtk(b)
		s.BAtk = calcBatk(l, s.BAtk, renewal)
	}

	// Pre-renewal's status_calc_watk on each hand's weapon ATK, then
	// bWeaponAtkRate on the right hand's. A player's refine ATK is left
	// out.
	if !renewal {
		s.RHW.Atk = calcWatk(l, b.RHW.Atk, renewal, b.RHW.Wlv == 4)
		if pb.weaponAtkRate != 0 {
			s.RHW.Atk += s.RHW.Atk * pb.weaponAtkRate / 100
		}
		if b.LHW.Atk != 0 {
			s.LHW.Atk = calcWatk(l, b.LHW.Atk, renewal, b.LHW.Wlv == 4)
		}
	}

	// HIT and FLEE follow the change of the stats they come from.
	if s.Dex == b.Dex && (!renewal || (s.Luk == b.Luk && s.con == b.con)) {
		s.Hit = calcHit(l, b.Hit, renewal)
	} else {
		d := s.Dex - b.Dex
		if renewal {
			d += s.Luk/3 - b.Luk/3 + 2*(s.con-b.con)
		}
		s.Hit = calcHit(l, b.Hit+d, renewal)
	}
	if s.Agi == b.Agi && (!renewal || (s.Luk == b.Luk && s.con == b.con)) {
		s.Flee = calcFlee(l, b.Flee, renewal)
	} else {
		d := s.Agi - b.Agi
		if renewal {
			d += s.Luk/5 - b.Luk/5 + 2*(s.con-b.con)
		}
		s.Flee = calcFlee(l, b.Flee+d, renewal)
	}

	s.Def = calcDef(l, b.Def, renewal, true, p.capDef)
	switch {
	case s.Vit == b.Vit && (!renewal || s.Agi == b.Agi):
		s.Def2 = calcDef2(l, b.Def2, renewal, s.Vit)
	case renewal:
		s.Def2 = calcDef2(l, b.Def2+int((float32(s.Vit)/2-float32(b.Vit)/2)+(float32(s.Agi)/5-float32(b.Agi)/5)), renewal, s.Vit)
	default:
		s.Def2 = calcDef2(l, b.Def2+s.Vit-b.Vit, renewal, s.Vit)
	}
	s.MDef = calcMdef(l, b.MDef, renewal, p.capDef)
	switch {
	case s.Int == b.Int && s.Vit == b.Vit && (!renewal || s.Dex == b.Dex):
		s.MDef2 = calcMdef2(l, b.MDef2, renewal)
	case renewal:
		s.MDef2 = calcMdef2(l, b.MDef2+s.Int-b.Int+int((float32(s.Dex)/5-float32(b.Dex)/5)+(float32(s.Vit)/5-float32(b.Vit)/5)), renewal)
	default:
		s.MDef2 = calcMdef2(l, b.MDef2+s.Int-b.Int+(s.Vit-b.Vit)/2, renewal)
	}

	// status_calc_speed. Item speed bonuses and the haste of Improve
	// Dodge apply even with no status changes, since a player's status
	// change list always exists.
	in := speedInput{player: true, itemSpeed: pb.speedRate + pb.speedAddRate, tunnel: p.skill(skillTunnelDrive),
		riding: p.c.Riding, cart: p.c.Cart, pushCart: p.skill(skillPushCart),
		danceLesson: p.skill(skillDancingLesson)}
	if p.c.Casting {
		in.freeCast = p.skill(skillFreeCast)
	}
	if p.c.Male {
		in.danceLesson = p.skill(skillMusicalLesson)
	}
	if p.mapID&mapidSecondMask == mapidAssassin {
		in.assassinTF = p.skill(skillMiss)
	}
	s.Speed = calcSpeed(l, b.Speed, in)
	if s.Speed < maxWalkSpeed {
		s.Speed = maxWalkSpeed
	}

	// Critical is at least 1%, and katars double it after status changes.
	if b.Cri != 0 {
		switch {
		case s.Luk == b.Luk:
			s.Cri = calcCritical(l, b.Cri, renewal)
		case renewal:
			s.Cri = calcCritical(l, b.Cri+3*(s.Luk-b.Luk), renewal)
		default:
			s.Cri = calcCritical(l, b.Cri+(s.Luk-b.Luk)*10/3, renewal)
		}
		if p.worn.Weapon == weaponKatar {
			s.Cri *= 2
		}
	}
	if b.Flee2 != 0 {
		s.Flee2 = calcFlee2(l, b.Flee2+s.Luk-b.Luk)
	}

	// Elements.
	s.RHW.Ele = calcAttackElement(l, b.RHW.Ele)
	s.LHW.Ele = calcAttackElement(l, b.LHW.Ele)
	s.DefEle = calcElement(l, b.DefEle)
	s.EleLv = calcElementLv(l, b.EleLv)

	// Max HP and SP from the battle VIT and INT.
	s.MaxHP = p.capMaxHP(p.calcMaxHP(s.Vit))
	s.MaxSP = capValue(p.calcMaxSP(s.Int), 1, maxSP)

	if renewal {
		// status_calc_watk, on the left hand's weapon ATK with only a
		// left-hand weapon.
		if b.WAtk == 0 {
			s.WAtk = 0
			s.WAtk2 = calcWatk(l, b.WAtk2, renewal, true)
		} else {
			s.WAtk = calcWatk(l, b.WAtk, renewal, true)
		}
	}

	// MATK.
	recognized := l.Has(sc.SCRecognizedspell)
	var matkMin, matkMax int
	if renewal {
		// MATK = StatusMATK + WeaponMATK + ExtraMATK.
		matkMin, matkMax = p.baseMatk(&s), p.baseMatk(&s)
		wMatk, variance := 0, 0
		if s.RHW.Matk > 0 {
			wMatk = s.RHW.Matk
			variance = s.RHW.Matk * s.RHW.Wlv / 10
		}
		if s.LHW.Matk > 0 {
			wMatk += s.LHW.Matk
			variance += s.LHW.Matk * s.LHW.Wlv / 10
		}
		matkMin += wMatk - variance
		matkMax += wMatk + variance
		if recognized {
			matkMin = max(matkMin, matkMax)
		}
		if o := pb.player.Right.Overrefine; o > 0 {
			matkMin++
			matkMax += o - 1
		}
		// Mystical Amplification.
		matkMin, matkMax = magicPowerMatk(l, matkMin, matkMax, true)
		matkMin += pb.ematk + pb.ematkHidden
		matkMax += pb.ematk + pb.ematkHidden
		matkMin = calcPseudobuffMatk(l, matkMin, renewal)
		matkMax = calcPseudobuffMatk(l, matkMax, renewal)
		if pb.matkRate != 100 {
			matkMin = matkMin * pb.matkRate / 100
			matkMax = matkMax * pb.matkRate / 100
		}
	} else {
		matkMin = int(uint16(s.Int + (s.Int/7)*(s.Int/7))) // #nosec G115 -- status_base_matk_min returns uint16.
		matkMax = int(uint16(s.Int + (s.Int/5)*(s.Int/5))) // #nosec G115 -- status_base_matk_max returns uint16.
		matkMin += pb.ematk + pb.ematkHidden
		matkMax += pb.ematk + pb.ematkHidden
		if pb.matkRate != 100 {
			matkMin = matkMin * pb.matkRate / 100
			matkMax = matkMax * pb.matkRate / 100
		}
		if recognized {
			matkMin = max(matkMin, matkMax)
		}
		matkMin = calcPseudobuffMatk(l, matkMin, renewal)
		matkMax = calcPseudobuffMatk(l, matkMax, renewal)
		matkMin, matkMax = magicPowerMatk(l, matkMin, matkMax, false)
	}
	s.MAtkMin = capValue(matkMin, 0, math.MaxUint16)
	s.MAtkMax = capValue(matkMax, 0, math.MaxUint16)

	// ASPD.
	amotion := p.baseAmotion(&s)
	if !p.t.renewalASPD {
		s.aspdRate = calcAspdRateOf(l, b.aspdRate, p.worn.Weapon)
	}
	amotion = amotion * s.aspdRate / 1000
	if lv := p.skill(skillFreeCast); lv > 0 && p.c.Casting {
		// Free Cast slows the attacks of a player who casts.
		if p.t.renewalASPD {
			amotion = amotion * 5 * (lv + 10) / 100
		} else {
			amotion += (2000 - amotion) * (55 - 5*(lv+1)) / 100
		}
	}
	if p.t.renewalASPD {
		// The renewal rate modifier, from bAspdRate and statuses.
		bonus := calcAspdBonus(l, false, true, p.skill)
		amotion += max(195-amotion, 2) * (pb.aspdRate2 + bonus) / 100
		amotion = amotionZeroASPD - amotion*amotionInterval
		// The raw change of bAspd.
		amotion += pb.aspdAdd
	}
	amotion = calcFixAspd(l, amotion)
	s.AMotion = capValue(amotion, p.maxASPD()/amotionDividerPC, minASPD/amotionDividerPC)
	s.ADelay = amotionDividerPC * s.AMotion

	// Damage motion follows a changed AGI.
	if b.Agi != s.Agi {
		s.DMotion = capValue(800-s.Agi*4, 400, 800)
	}
	return s
}

// window is pc_leftside_atk to pc_rightside_matk, with the clamp that
// clif_updatestatus applies to pre-renewal's SP_MDEF2.
func (p *pc) window(s *data) Window {
	if p.t.renewal {
		return Window{
			ATK: s.BAtk, ATK2: s.WAtk + s.WAtk2 + s.EAtk,
			MATK: p.baseMatk(s), MATK2: s.RHW.Matk + s.LHW.Matk + p.b.ematk,
			DEF: s.Def2, DEF2: s.Def,
			MDEF: s.MDef2, MDEF2: s.MDef,
		}
	}
	return Window{
		ATK: s.BAtk + s.RHW.Atk + s.LHW.Atk, ATK2: s.RHW.Atk2 + s.LHW.Atk2,
		MATK: s.MAtkMin, MATK2: s.MAtkMax,
		DEF: s.Def, DEF2: s.Def2,
		MDEF: s.MDef, MDEF2: max(s.MDef2-s.Vit/2, 0),
	}
}

// capValue is rAthena's cap_value.
func capValue(v, lo, hi int) int { return min(max(v, lo), hi) }
