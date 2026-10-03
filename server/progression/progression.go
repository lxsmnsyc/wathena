// Package progression ports what rAthena gives for a kill: experience
// shares, level ups, the renewal level penalty, item drops, and where
// dropped items land and who may pick them up.
//
// This is a port of parts of rAthena's pc.cpp, mob.cpp and map.cpp
// (GPL-3.0). Function comments name the rAthena functions they port. The
// settings are the defaults from rAthena's conf/battle files.
package progression

import "strings"

// Limits from rAthena's headers.
const (
	// MaxLevel is MAX_LEVEL, the highest level any job can have.
	MaxLevel = 275
	// MaxLevelBaseExp is MAX_LEVEL_BASE_EXP: the most base exp a player
	// at the maximum base level can hold.
	MaxLevelBaseExp = 99999999
	// MaxLevelJobExp is MAX_LEVEL_JOB_EXP, the same for job exp.
	MaxLevelJobExp = 999999999
	// MaxExp is MAX_EXP for clients from 2017-08-30 on.
	MaxExp = int64(1<<63 - 1)
	// DamageLogSize is DAMAGELOG_SIZE, the most attackers a monster
	// remembers.
	DamageLogSize = 20
	// startStatusPoints is inter_athena.conf's start_status_points.
	startStatusPoints = 48
)

// Rand is the random source rolls draw from. IntN returns a number in
// [0, n), as rAthena's rnd() % n. It must draw once even when n is 1,
// as rnd_value does.
type Rand interface {
	IntN(n int) int
}

// ItemRate is the drop rate setting for one kind of item, from
// drops.conf: the rate in percent for normal monsters, bosses and MVPs,
// and the bounds of the adjusted rate, out of 10000.
type ItemRate struct {
	Rate, Boss, MVP int
	Min, Max        int
}

// Config holds the battle settings this package reads. DefaultConfig has
// rAthena's defaults.
type Config struct {
	// BaseExpRate, JobExpRate and MvpExpRate scale monster exp when
	// loading, in percent (base_exp_rate, job_exp_rate, mvp_exp_rate).
	BaseExpRate, JobExpRate, MvpExpRate int
	// MultiLevelUp lets one kill give more than one level
	// (multi_level_up). MultiLevelUpBase and MultiLevelUpJob stop it from
	// those levels on, when not 0.
	MultiLevelUp                      bool
	MultiLevelUpBase, MultiLevelUpJob int
	// MaxExpGainRate caps the exp from one kill, in tenths of a percent of
	// the current level's exp (max_exp_gain_rate). 0 is no cap.
	MaxExpGainRate int
	// ExpCalcType is exp_calc_type: 0 shares by damage over total damage,
	// 1 by damage over max HP, 2 as 0 with the first attacker counted
	// twice.
	ExpCalcType int
	// ExpBonusAttacker is the percent more exp for each attacker after
	// the first, up to ExpBonusMaxAttacker attackers.
	ExpBonusAttacker, ExpBonusMaxAttacker int
	// FirstAttackLootBonus is the percent of total damage added to the
	// first attacker for loot priority (first_attack_loot_bonus).
	FirstAttackLootBonus int

	// Drop rates by item kind (item_rate_* and item_drop_*_min/max).
	Common, Heal, Use, Equip, Card ItemRate
	// LogarithmicDrops is item_logarithmic_drops.
	LogarithmicDrops bool
	// DropRateIncrease adds 0.01% to drops below 50% (drop_rateincrease).
	DropRateIncrease bool
	// DropRate0Item lets a drop rate reach 0 (drop_rate0item).
	DropRate0Item bool
	// DropsByLuk and DropsByLuk2 add the killer's LUK to drop rates
	// (drops_by_luk, drops_by_luk2).
	DropsByLuk, DropsByLuk2 int
	// DropRateCap is drop_rate_cap, out of 10000.
	DropRateCap int

	// FloorItemLifetime is how long an item stays on the ground, in
	// milliseconds (flooritem_lifetime).
	FloorItemLifetime int64
	// ItemGetTime holds item_first_get_time, item_second_get_time and
	// item_third_get_time. MVPItemGetTime holds the mvp_ versions, used
	// for drops of bosses and MVPs.
	ItemGetTime, MVPItemGetTime [3]int64
	// DropDelay is how long after a kill the items fall, in milliseconds:
	// 0 when delay_battle_damage is on, as by default, and 500 otherwise.
	DropDelay int64
}

// DefaultConfig is rAthena's default conf/battle settings.
var DefaultConfig = Config{
	BaseExpRate:          100,
	JobExpRate:           100,
	MvpExpRate:           100,
	ExpBonusAttacker:     25,
	ExpBonusMaxAttacker:  12,
	FirstAttackLootBonus: 30,
	Common:               ItemRate{Rate: 100, Boss: 100, MVP: 100, Min: 1, Max: 10000},
	Heal:                 ItemRate{Rate: 100, Boss: 100, MVP: 100, Min: 1, Max: 10000},
	Use:                  ItemRate{Rate: 100, Boss: 100, MVP: 100, Min: 1, Max: 10000},
	Equip:                ItemRate{Rate: 100, Boss: 100, MVP: 100, Min: 1, Max: 10000},
	Card:                 ItemRate{Rate: 100, Boss: 100, MVP: 100, Min: 1, Max: 10000},
	DropRateCap:          9000,
	FloorItemLifetime:    60000,
	ItemGetTime:          [3]int64{3000, 2000, 2000},
	MVPItemGetTime:       [3]int64{10000, 10000, 2000},
}

// Data is the part of an extracted progression.json this package reads.
type Data struct {
	Exp        []ExpGroup `json:"exp"`
	StatPoints []struct {
		Level  int `json:"level"`
		Points int `json:"points"`
	} `json:"stat_points"`
	// LevelPenalty maps a penalty type, such as "exp" or "drop", to its
	// rows as written in level_penalty.yml.
	LevelPenalty map[string][]struct {
		Difference int `json:"difference"`
		Rate       int `json:"rate"`
	} `json:"level_penalty"`
	Mobs []MobData `json:"mobs"`
}

// ExpGroup is one group of job_exp.yml. BaseExp[i] is the exp to go from
// level i+1 to i+2. A group may have only base or only job exp.
type ExpGroup struct {
	Jobs         []int   `json:"jobs"`
	MaxBaseLevel *int    `json:"max_base_level"`
	BaseExp      []int64 `json:"base_exp"`
	MaxJobLevel  *int    `json:"max_job_level"`
	JobExp       []int64 `json:"job_exp"`
}

// MobData is a monster's rewards from mob_db.yml.
type MobData struct {
	ID          uint32 `json:"id"`
	MvpExp      int64  `json:"mvp_exp"`
	DamageTaken int    `json:"damage_taken"`
	Drops       []Drop `json:"drops"`
	MvpDrops    []Drop `json:"mvp_drops"`
}

// Drop is one item a monster may drop. Rate is out of 10000.
type Drop struct {
	Item           uint32 `json:"item"`
	Rate           int    `json:"rate"`
	StealProtected bool   `json:"steal_protected"`
	// RandomOptionGroup is mob_db's RandomOptionGroup: the group of
	// random options the dropped copy rolls, or "".
	RandomOptionGroup string `json:"random_option_group,omitempty"`
}

type job struct {
	maxBase, maxJob int
	baseExp, jobExp [MaxLevel]int64
}

// Tables holds the loaded exp tables, status points and level penalty.
type Tables struct {
	Config
	jobs map[int]*job
	// statPoints is the total status points by base level.
	statPoints [MaxLevel + 2]int
	penalty    map[string]*[2*MaxLevel - 1]int
	// items maps an item ID to its type, in lower case, such as "healing".
	items map[uint32]string
}

// New builds the tables from extracted data. items maps item IDs to their
// type from item_db, such as "Healing" or "Card", in any case.
func New(d *Data, items map[uint32]string, cfg Config) *Tables {
	t := &Tables{Config: cfg, jobs: map[int]*job{}, penalty: map[string]*[2*MaxLevel - 1]int{}, items: map[uint32]string{}}
	for id, typ := range items {
		t.items[id] = strings.ToLower(typ)
	}
	t.loadExp(d.Exp)
	t.loadStatPoints(d)
	for typ := range d.LevelPenalty {
		t.penalty[typ] = penaltyTable(d, typ)
	}
	return t
}

// loadExp is JobDatabase's reading of job_exp.yml. Later groups override
// earlier ones. A job with no MaxBaseLevel or MaxJobLevel gets MaxLevel.
func (t *Tables) loadExp(groups []ExpGroup) {
	for _, g := range groups {
		for _, id := range g.Jobs {
			j, exists := t.jobs[id]
			if !exists {
				j = &job{maxBase: MaxLevel, maxJob: MaxLevel}
				t.jobs[id] = j
			}
			if g.MaxBaseLevel != nil {
				j.maxBase = levelCap(*g.MaxBaseLevel)
			}
			for i, exp := range g.BaseExp {
				if level := i + 1; level <= j.maxBase && level <= MaxLevel {
					j.baseExp[i] = exp
				}
			}
			if g.MaxJobLevel != nil {
				j.maxJob = levelCap(*g.MaxJobLevel)
			}
			for i, exp := range g.JobExp {
				if level := i + 1; level <= j.maxJob && level <= MaxLevel {
					j.jobExp[i] = exp
				}
			}
		}
	}
}

// loadStatPoints is PlayerStatPointDatabase::loadingFinished with
// use_statpoint_table on: level 1 has start_status_points, and levels
// missing from the table follow the formula.
func (t *Tables) loadStatPoints(d *Data) {
	have := map[int]int{}
	for _, r := range d.StatPoints {
		have[r.Level] = r.Points
	}
	t.statPoints[1] = startStatusPoints
	for level := 2; level <= MaxLevel; level++ {
		points, ok := have[level]
		if !ok {
			points = t.statPoints[level-1]
			if level <= 200 {
				points += (level - 1 + 15) / 5
			}
		}
		t.statPoints[level] = points
	}
}

// levelCap turns a MaxBaseLevel or MaxJobLevel of 0 or above MaxLevel
// into MaxLevel, as the loader does.
func levelCap(level int) int {
	if level <= 0 || level > MaxLevel {
		return MaxLevel
	}
	return level
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
