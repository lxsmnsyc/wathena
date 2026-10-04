// Package rules holds a world's ruleset flags in the form the game code
// reads them (strategy section 3.6, decision 0019). server/mod resolves a
// ruleset mod into flag values, and server/gamedata turns them into
// Rules. Each package that branches where rAthena checks RENEWAL reads
// the matching field here, not the base.
//
// A Rules value is shared by every zone of a world. Nothing changes it
// after the world starts.
package rules

// Rules are the flags of one ruleset. A true field is the renewal value,
// or on for an on/off flag.
type Rules struct {
	// Renewal is the base: renewal's formulas and data, or
	// pre-renewal's. It cannot be mixed.
	Renewal bool
	// CastTime is cast_time: renewal's variable and fixed cast time
	// (RENEWAL_CAST), or the DEX-only cast time.
	CastTime bool
	// ASPD is aspd: renewal's ASPD formula, shield penalty and bonus
	// stacking (RENEWAL_ASPD). status.Tables reads it through SetASPD.
	ASPD bool
	// SizeTable is size_table: renewal's size table for knuckles and
	// whips. status.Tables reads it through SetSizeTable.
	SizeTable bool
	// SkillCooldown is skill_cooldown: the cooldowns of renewal's
	// skill_db, or those of pre-renewal's. gamedata applies it to the
	// skill data.
	SkillCooldown bool
	// StatPointCost is stat_point_cost: the status point cost formula
	// (RENEWAL_STAT).
	StatPointCost bool
	// LevelPenaltyExp and LevelPenaltyDrop are level_penalty_exp and
	// level_penalty_drop (RENEWAL_EXP and RENEWAL_DROP).
	LevelPenaltyExp, LevelPenaltyDrop bool
	// LevelDamageMod is level_damage_mod: skill damage grows with base
	// levels above 99 (RENEWAL_LVDMG).
	LevelDamageMod bool
	// ElementTable is element_table: battle_attr_fix with renewal's
	// table and status bonuses, or pre-renewal's.
	ElementTable bool
	// StatusResist is status_resist: status_get_sc_def's formulas.
	StatusResist bool
	// HealFormula is heal_formula: skill_calc_heal and the RENEWAL parts
	// of pc_itemheal.
	HealFormula bool
	// BossProtection is boss_protection: traps that do not catch bosses,
	// and Blade Stop on bosses.
	BossProtection bool
	// EnsembleSongs is ensemble_songs: songs and dances as self buffs,
	// ensemble fatigue, and the other RENEWAL parts of the Bard and
	// Dancer performance code.
	EnsembleSongs bool
	// Homunculus is homunculus. No homunculus system exists yet, so
	// nothing reads it.
	Homunculus bool
	// BusyRestrictions is busy_restrictions: the busy message and the
	// actions renewal refuses while a player talks to an NPC or casts.
	BusyRestrictions bool
	// FourthJobTraits is fourth_job_traits: trait stats, trait points,
	// RES and MRES. Only a renewal base allows it.
	FourthJobTraits bool
	// ItemRolls is item_rolls: items with roll ranges get random values
	// when created. It is off in both core rulesets.
	ItemRolls bool
	// Skills holds the skill_rebalance flags a ruleset set, by skill ID:
	// true for the skill's renewal version. A skill without an entry
	// follows the base.
	Skills map[int]bool
	// Limits are the limits rAthena compiles in, such as MAX_PARTY
	// (limits.go).
	Limits Limits
	// Party holds the party and friend values of rAthena's
	// battle_athena.conf and inter_athena.conf (limits.go).
	Party Party
	// Guild holds the guild values of rAthena's battle conf files
	// (limits.go).
	Guild Guild
}

// Base returns the rules of a core ruleset: every flag at the base's
// value. It is a new value each time, so callers may change it.
func Base(renewal bool) *Rules {
	return &Rules{
		Renewal: renewal, ASPD: renewal, SizeTable: renewal, CastTime: renewal, SkillCooldown: renewal, StatPointCost: renewal,
		LevelPenaltyExp: renewal, LevelPenaltyDrop: renewal, LevelDamageMod: renewal, ElementTable: renewal,
		StatusResist: renewal, HealFormula: renewal, BossProtection: renewal, EnsembleSongs: renewal,
		Homunculus: renewal, BusyRestrictions: renewal, FourthJobTraits: renewal,
		Limits: DefaultLimits(), Party: DefaultParty(), Guild: DefaultGuild(),
	}
}

var (
	renewalRules    = Base(true)
	preRenewalRules = Base(false)
)

// Of returns the shared rules of a core ruleset. Callers must not change
// it.
func Of(renewal bool) *Rules {
	if renewal {
		return renewalRules
	}
	return preRenewalRules
}

// Skill reports whether a skill uses its renewal version: its
// skill_rebalance flag, or the base.
func (r *Rules) Skill(id int) bool {
	if v, ok := r.Skills[id]; ok {
		return v
	}
	return r.Renewal
}

// SongSkills are the Bard and Dancer songs and ensembles, and the
// performance skills that go with them, by skill_db name. The
// ensemble_songs flag picks their code and their skill_db records, so a
// ruleset cannot set skill_rebalance for them.
var SongSkills = map[string]int{
	"BA_APPLEIDUN": 322, "BA_ASSASSINCROSS": 320, "BA_DISSONANCE": 317, "BA_POEMBRAGI": 321, "BA_WHISTLE": 319,
	"BD_ADAPTATION": 304, "BD_DRUMBATTLEFIELD": 309, "BD_ENCORE": 305, "BD_ETERNALCHAOS": 308, "BD_INTOABYSS": 312, "BD_LULLABY": 306,
	"BD_RICHMANKIM": 307, "BD_RINGNIBELUNGEN": 310, "BD_ROKISWEIL": 311, "BD_SIEGFRIED": 313,
	"CG_HERMODE": 488, "CG_LONGINGFREEDOM": 487, "CG_MOONLIT": 395,
	"DC_DONTFORGETME": 328, "DC_FORTUNEKISS": 329, "DC_HUMMING": 327, "DC_SERVICEFORYOU": 330, "DC_UGLYDANCE": 325,
}

// RebalancedSkills are the skills whose RENEWAL branches the server has
// ported, by skill_db name, so their skill_rebalance flags change
// something. Each flag picks the skill's own code (its SkillImpl and
// skill.cpp parts), the start values and effects of its own status, and
// its skill_db record. The damage formulas that the battle code shares
// stay with the base.
var RebalancedSkills = map[string]int{
	// Swordman, Knight, Lord Knight, Crusader and Paladin.
	"SM_MAGNUM": 7, "KN_TWOHANDQUICKEN": 60, "KN_BRANDISHSPEAR": 57, "KN_BOWLINGBASH": 62, "KN_CHARGEATK": 1001,
	"LK_SPIRALPIERCE": 397, "LK_CONCENTRATION": 357, "CR_AUTOGUARD": 249, "CR_SHIELDBOOMERANG": 251, "CR_HOLYCROSS": 253,
	"CR_SHRINK": 1002, "CR_SPEARQUICKEN": 258, "PA_PRESSURE": 367, "PA_SHIELDCHAIN": 480,
	// Mage, Wizard, High Wizard, Sage and Professor.
	"MG_SAFETYWALL": 12, "MG_FIREBALL": 17, "MG_THUNDERSTORM": 21, "MG_ENERGYCOAT": 157,
	"WZ_METEOR": 83, "WZ_VERMILION": 85, "WZ_FROSTNOVA": 88, "WZ_STORMGUST": 89, "WZ_EARTHSPIKE": 90, "WZ_HEAVENDRIVE": 91,
	"WZ_SIGHTBLASTER": 1006, "HW_NAPALMVULCAN": 400, "HW_GRAVITATION": 484, "HW_MAGICPOWER": 366,
	"SA_MAGICROD": 276, "SA_SPELLBREAKER": 277, "SA_AUTOSPELL": 279, "SA_FLAMELAUNCHER": 280, "SA_FROSTWEAPON": 281,
	"SA_LIGHTNINGLOADER": 282, "SA_SEISMICWEAPON": 283, "SA_VOLCANO": 285, "SA_DELUGE": 286, "SA_VIOLENTGALE": 287, "PF_SOULCHANGE": 374,
	// Archer, Hunter, Sniper, Bard and Dancer skills that are not songs.
	"AC_SHOWER": 47, "HT_SKIDTRAP": 115, "HT_LANDMINE": 116, "HT_BLASTMINE": 122, "HT_CLAYMORETRAP": 123, "HT_BLITZBEAT": 129,
	"HT_PHANTASMIC": 1009, "SN_FALCONASSAULT": 381, "SN_SHARPSHOOTING": 382,
	"BA_MUSICALSTRIKE": 316, "DC_THROWARROW": 324, "BA_PANGVOICE": 1010, "DC_WINKCHARM": 1011, "CG_ARROWVULCAN": 394, "CG_TAROTCARD": 489,
	// Acolyte, Priest, High Priest, Monk and Champion.
	"AL_ANGELUS": 33, "AL_BLESSING": 34, "PR_IMPOSITIO": 66, "PR_SUFFRAGIUM": 67, "PR_TURNUNDEAD": 77, "PR_MAGNUS": 79,
	"PR_REDEMPTIO": 1014, "HP_ASSUMPTIO": 361, "HP_BASILICA": 362,
	"MO_TRIPLEATTACK": 263, "MO_INVESTIGATE": 266, "MO_FINGEROFFENSIVE": 267, "MO_STEELBODY": 268, "MO_EXPLOSIONSPIRITS": 270,
	"MO_EXTREMITYFIST": 271, "MO_CHAINCOMBO": 272, "MO_COMBOFINISH": 273, "MO_BALKYOUNG": 1016,
	"CH_PALMSTRIKE": 370, "CH_TIGERFIST": 371, "CH_CHAINCRUSH": 372,
	// Merchant, Blacksmith, Whitesmith, Alchemist and Creator.
	"MC_MAMMONITE": 42, "MC_LOUD": 155, "BS_ADRENALINE": 111, "BS_OVERTHRUST": 113, "WS_OVERTHRUSTMAX": 486, "WS_CARTTERMINATION": 485,
	"AM_DEMONSTRATION": 229, "AM_ACIDTERROR": 230, "CR_ACIDDEMONSTRATION": 490,
	// Thief, Assassin, Assassin Cross, Rogue and Stalker.
	"TF_STEAL": 50, "AS_SONICBLOW": 136, "AS_SPLASHER": 141, "AS_VENOMKNIFE": 1004, "ASC_EDP": 378, "ASC_BREAKER": 379,
	"ASC_METEORASSAULT": 406, "RG_BACKSTAP": 212, "RG_RAID": 214, "RG_CLOSECONFINE": 1005,
}
