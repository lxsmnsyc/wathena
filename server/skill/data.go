// Package skill holds the skill database, skill trees, and the skills
// themselves, as rAthena's skill.cpp, skill_db.yml, skill_tree.yml and the
// SkillImpl classes of src/map/skills (GPL-3.0). Function comments name the
// rAthena functions they port.
//
// The zone runs the casting itself (unit_skilluse_id and its kin); this
// package gives it the data, the formulas of cast time, delay and costs,
// and each skill's behavior through Impl.
package skill

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/server/battle"
)

// MaxLevel is MAX_SKILL_LEVEL: the levels skill_db stores.
const MaxLevel = 13

// Levels is a value with one entry per level, as skill_db's arrays.
type Levels [MaxLevel]int

// At is skill_get_lv: the value at a level. Levels above MaxLevel follow
// the last three values' trend, and a level below 1 gives 0.
func (l *Levels) At(lv int) int {
	if lv < 1 {
		return 0
	}
	i := min(lv, MaxLevel) - 1
	if lv > MaxLevel && l[i] > 1 && i > 1 {
		a, b, c := l[i-2], l[i-1], l[i]
		return c + (lv-MaxLevel+1)*(b-a)/2 + (lv-MaxLevel)*(c-b)/2
	}
	return l[i]
}

// Target is a skill's target type, as rAthena's e_skill_inf (INF_).
type Target int

// Target types.
const (
	TargetPassive Target = 0
	TargetEnemy   Target = 1
	TargetGround  Target = 2
	TargetSelf    Target = 4
	TargetSupport Target = 16
	TargetTrap    Target = 32
)

// String is the target's name in data/skills.yml.
func (t Target) String() string {
	switch t {
	case TargetEnemy:
		return "enemy"
	case TargetGround:
		return "ground"
	case TargetSelf:
		return "self"
	case TargetSupport:
		return "support"
	case TargetTrap:
		return "trap"
	}
	return "passive"
}

// Hit types of skill_db's Hit, as rAthena's e_damage_type.
const (
	HitNormal = 0
	HitSingle = 6
	HitMulti  = 8
)

// Cast and delay flags of skill_db, as SKILL_CAST_ bits.
const (
	CastIgnoreDex        = 1
	CastIgnoreStatus     = 2
	CastIgnoreItemBonus  = 4
	requireAmmoTypeCount = 9
)

// Required states, as rAthena's e_require_state (ST_).
const (
	StateNone   = 0
	StateHidden = 1
	StateRiding = 2
	StateFalcon = 3
	StateCart   = 4
	StateShield = 5
	StateWater  = 8
)

// Requires is skill_db's Requires: what using a skill costs and needs.
type Requires struct {
	HP, SP, AP, HPRate, SPRate, APRate, MaxHPTrigger, Zeny Levels
	// Weapon is a bit per W_ type (1 << W_), 0 for any. Ammo is a bit per
	// AMMO_ type, 0 for none.
	Weapon, Ammo int
	AmmoAmount   Levels
	// State is an ST_ value.
	State int
	// Status lists SC_ values the caster must have.
	Status       []int
	SpiritSphere Levels
	// Items holds 10 slots of [item, amount], as itemid and amount.
	// ItemLevelDependent is set when an entry named its level: the slot
	// of the cast level is the one used.
	Items              [10][2]int
	ItemLevelDependent bool
	// Equipment lists items the caster must wear.
	Equipment []int
}

// Unit is skill_db's Unit: the ground unit a skill places.
type Unit struct {
	ID, AlternateID int
	Layout, Range   Levels
	Interval        int
	// Target is the BCT_ value of who the unit affects.
	Target int
	Flags  []string
}

// Def is one skill: rAthena's s_skill_db, with the fields of proposal
// 0002, review decision 20, that mods set.
type Def struct {
	ID int
	// Name is the skill's key: skill_db's name, such as SM_BASH, or a
	// mod's full name.
	Name string
	// DisplayName is the name players see, skill_db's Description.
	DisplayName string
	MaxLevel    int
	// Type is battle.TypeWeapon, TypeMagic, TypeMisc, or 0 for none.
	Type   int
	Target Target
	NK     battle.NK
	// Flags are the INF2_ names that are set, such as IsQuest.
	Flags              []string
	Range              Levels
	Hit                int
	HitCount           Levels
	Element            Levels
	Splash             Levels
	ActiveInstance     Levels
	Knockback          Levels
	CastCancel         bool
	CastDefenseRate    int
	CastTime           Levels
	AfterCastActDelay  Levels
	AfterCastWalkDelay Levels
	Duration1          Levels
	Duration2          Levels
	Cooldown           Levels
	FixedCastTime      Levels
	CastTimeFlags      int
	CastDelayFlags     int
	Requires           Requires
	Unit               *Unit
	// Status is the SC_ value of skill_db's Status, or -1.
	Status int
	// NoCast is skill_nocast_db's map type bits.
	NoCast int

	// Mod is the mod that added the skill, or "" for core's.
	Mod string
	// Script is the entry script that runs the skill: "<mod>:<path>", or
	// "" for none.
	Script string
	// Icon is the skill whose icon and animation the skill shows, for a
	// mod's skill. It is 0 for the skill's own.
	Icon int

	// renewal is the mode of the database that holds the skill, for the
	// formulas that differ by mode.
	renewal bool
}

// Has reports whether an INF2_ flag is set, by name, such as IsQuest.
func (d *Def) Has(flag string) bool { return slices.Contains(d.Flags, flag) }

// Passive reports whether the skill is passive.
func (d *Def) Passive() bool { return d.Target == TargetPassive }

// Data is skills.json: skill_db.yml as SkillDatabase loads it.
type Data struct {
	Mode   string     `json:"mode"`
	Skills []RawSkill `json:"skills"`
}

// RawSkill is one skill of skills.json.
type RawSkill struct {
	ID                 int      `json:"id"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	MaxLevel           int      `json:"max_level"`
	Type               int      `json:"type"`
	Target             int      `json:"target"`
	DamageFlags        []string `json:"damage_flags"`
	Flags              []string `json:"flags"`
	Range              Levels   `json:"range"`
	Hit                int      `json:"hit"`
	HitCount           Levels   `json:"hit_count"`
	Element            Levels   `json:"element"`
	Splash             Levels   `json:"splash"`
	ActiveInstance     Levels   `json:"active_instance"`
	Knockback          Levels   `json:"knockback"`
	CastCancel         bool     `json:"cast_cancel"`
	CastDefenseRate    int      `json:"cast_defense_reduction"`
	CastTime           Levels   `json:"cast_time"`
	AfterCastActDelay  Levels   `json:"after_cast_act_delay"`
	AfterCastWalkDelay Levels   `json:"after_cast_walk_delay"`
	Duration1          Levels   `json:"duration1"`
	Duration2          Levels   `json:"duration2"`
	Cooldown           Levels   `json:"cooldown"`
	FixedCastTime      Levels   `json:"fixed_cast_time"`
	CastTimeFlags      int      `json:"cast_time_flags"`
	CastDelayFlags     int      `json:"cast_delay_flags"`
	Requires           struct {
		HP                 Levels   `json:"hp"`
		SP                 Levels   `json:"sp"`
		AP                 Levels   `json:"ap"`
		HPRate             Levels   `json:"hp_rate"`
		SPRate             Levels   `json:"sp_rate"`
		APRate             Levels   `json:"ap_rate"`
		MaxHPTrigger       Levels   `json:"max_hp_trigger"`
		Zeny               Levels   `json:"zeny"`
		Weapon             int      `json:"weapon"`
		Ammo               int      `json:"ammo"`
		AmmoAmount         Levels   `json:"ammo_amount"`
		State              int      `json:"state"`
		Status             []int    `json:"status"`
		SpiritSphere       Levels   `json:"spirit_sphere"`
		Items              [][2]int `json:"items"`
		ItemLevelDependent bool     `json:"item_level_dependent"`
		Equipment          []int    `json:"equipment"`
	} `json:"requires"`
	Unit *struct {
		ID          int      `json:"id"`
		AlternateID int      `json:"alternate_id"`
		Layout      Levels   `json:"layout"`
		Range       Levels   `json:"range"`
		Interval    int      `json:"interval"`
		Target      int      `json:"target"`
		Flags       []string `json:"flags"`
	} `json:"unit"`
	Status int `json:"status"`
	NoCast int `json:"nocast"`
}

// nkNames maps skill_db's DamageFlags to battle.NK.
var nkNames = map[string]battle.NK{
	"NoDamage": battle.NKNoDamage, "Splash": battle.NKSplash, "SplashSplit": battle.NKSplashSplit,
	"IgnoreAtkCard": battle.NKIgnoreAtkCard, "IgnoreElement": battle.NKIgnoreElement,
	"IgnoreDefense": battle.NKIgnoreDefense, "IgnoreFlee": battle.NKIgnoreFlee,
	"IgnoreDefCard": battle.NKIgnoreDefCard, "Critical": battle.NKCritical,
	"IgnoreLongCard": battle.NKIgnoreLongCard, "SimpleDefense": battle.NKSimpleDefense,
}

// newDef makes a Def from skills.json.
func newDef(r *RawSkill) (*Def, error) {
	d := &Def{
		ID: r.ID, Name: r.Name, DisplayName: r.Description, MaxLevel: r.MaxLevel, Type: r.Type, Target: Target(r.Target),
		Flags: r.Flags, Range: r.Range, Hit: r.Hit, HitCount: r.HitCount, Element: r.Element, Splash: r.Splash,
		ActiveInstance: r.ActiveInstance, Knockback: r.Knockback, CastCancel: r.CastCancel,
		CastDefenseRate: r.CastDefenseRate, CastTime: r.CastTime, AfterCastActDelay: r.AfterCastActDelay,
		AfterCastWalkDelay: r.AfterCastWalkDelay, Duration1: r.Duration1, Duration2: r.Duration2,
		Cooldown: r.Cooldown, FixedCastTime: r.FixedCastTime, CastTimeFlags: r.CastTimeFlags,
		CastDelayFlags: r.CastDelayFlags, Status: r.Status, NoCast: r.NoCast,
	}
	for _, n := range r.DamageFlags {
		f, ok := nkNames[n]
		if !ok {
			return nil, fmt.Errorf("skill %s: unknown damage flag %q", r.Name, n)
		}
		d.NK |= f
	}
	q := &r.Requires
	d.Requires = Requires{
		HP: q.HP, SP: q.SP, AP: q.AP, HPRate: q.HPRate, SPRate: q.SPRate, APRate: q.APRate,
		MaxHPTrigger: q.MaxHPTrigger, Zeny: q.Zeny, Weapon: q.Weapon, Ammo: q.Ammo, AmmoAmount: q.AmmoAmount,
		State: q.State, Status: q.Status, SpiritSphere: q.SpiritSphere, ItemLevelDependent: q.ItemLevelDependent,
		Equipment: q.Equipment,
	}
	for i, it := range q.Items {
		if i < len(d.Requires.Items) {
			d.Requires.Items[i] = it
		}
	}
	if u := r.Unit; u != nil {
		d.Unit = &Unit{ID: u.ID, AlternateID: u.AlternateID, Layout: u.Layout, Range: u.Range,
			Interval: u.Interval, Target: u.Target, Flags: u.Flags}
	}
	return d, nil
}

// DB is the skill database of one mode, with mods' skills.
type DB struct {
	renewal bool
	byID    map[int]*Def
	byName  map[string]*Def
	order   []*Def
}

// Load builds a mode's skill database from skills.json.
func Load(d *Data) (*DB, error) {
	db := &DB{byID: map[int]*Def{}, byName: map[string]*Def{}}
	switch d.Mode {
	case "renewal":
		db.renewal = true
	case "pre-renewal":
	default:
		return nil, fmt.Errorf("skill: unknown mode %q", d.Mode)
	}
	var errs []error
	for i := range d.Skills {
		def, err := newDef(&d.Skills[i])
		if err != nil {
			errs = append(errs, err)
			continue
		}
		db.add(def)
	}
	return db, errors.Join(errs...)
}

func (db *DB) add(d *Def) {
	d.renewal = db.renewal
	if old := db.byID[d.ID]; old != nil {
		delete(db.byName, old.Name)
		db.order = slices.DeleteFunc(db.order, func(x *Def) bool { return x == old })
	}
	db.byID[d.ID] = d
	db.byName[d.Name] = d
	db.order = append(db.order, d)
}

// Renewal reports whether the database is renewal's.
func (db *DB) Renewal() bool { return db.renewal }

// Get returns a skill by ID, or nil.
func (db *DB) Get(id int) *Def {
	if db == nil {
		return nil
	}
	return db.byID[id]
}

// ByName returns a skill by name, such as SM_BASH, or nil. Core names
// match without case, as skill_name2id.
func (db *DB) ByName(name string) *Def {
	if db == nil {
		return nil
	}
	if d := db.byName[name]; d != nil {
		return d
	}
	if !strings.Contains(name, ":") {
		return db.byName[strings.ToUpper(name)]
	}
	return nil
}

// All returns every skill in the order it was added.
func (db *DB) All() []*Def { return slices.Clone(db.order) }

// CastType is rAthena's e_cast_type: the path a skill takes when its
// cast ends.
type CastType int

const (
	CastDamage CastType = iota
	CastNoDamage
	CastGround
)

// CastType is skill_get_casttype: ground skills go to the ground path;
// support skills, and self skills without NoTargetSelf, and skills with
// the NoDamage flag go to the no-damage path; the rest deal damage.
func (d *Def) CastType() CastType {
	switch {
	case d.Target == TargetGround:
		return CastGround
	case d.Target == TargetSupport:
		return CastNoDamage
	case d.Target == TargetSelf:
		if d.Has("NoTargetSelf") {
			return CastDamage
		}
		return CastNoDamage
	case d.NK.Has(battle.NKNoDamage):
		return CastNoDamage
	}
	return CastDamage
}

// areaSize is battle_config.area_size, which a negative splash area
// stands for.
const areaSize = 14

// SplashFor is skill_get_splash: the splash area at a level, with
// area_size for a negative value.
func (d *Def) SplashFor(lv int) int {
	if s := d.Splash.At(lv); s >= 0 {
		return s
	}
	return areaSize
}

// AutoSpellSelf reports whether an autospell of a skill with no target
// option casts on the player itself, as pc_bonus_autospell picks it:
// support skills, and self skills without NoTargetSelf.
func (db *DB) AutoSpellSelf(id int) bool {
	d := db.Get(id)
	return d != nil && (d.Target == TargetSupport || d.Target == TargetSelf && !d.Has("NoTargetSelf"))
}
