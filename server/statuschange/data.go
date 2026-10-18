// Package statuschange runs status changes as rAthena's status.cpp does:
// starting and ending them, their resistance, their timers, and what
// each one sets up. Stats read the active statuses through
// server/status, and the zone drives the engine with its clock.
//
// This is a port of rAthena's status.cpp (GPL-3.0). Function comments
// name the rAthena functions they port. It covers the data of every
// status in status.yml, the common ailments, and the statuses that items
// start. Statuses that only skills start are stored and shown, and their
// effects come with their skills.
package statuschange

import (
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/lxsmnsyc/wathena/server/constants"
)

// Data is the status.json extract: status.yml as StatusDatabase loads
// it.
type Data struct {
	Mode     string       `json:"mode"`
	Statuses []DataStatus `json:"statuses"`
}

// DataStatus is one status of status.json.
type DataStatus struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	Constant      string   `json:"constant"`
	Icon          int      `json:"icon"`
	DurationSkill int      `json:"duration_skill"`
	Durations     []int    `json:"durations"`
	Durations2    []int    `json:"durations2"`
	States        []string `json:"states"`
	CalcFlags     []string `json:"calc_flags"`
	Opt1          int      `json:"opt1"`
	Opt2          int      `json:"opt2"`
	Opt3          int      `json:"opt3"`
	Options       int      `json:"options"`
	Flags         []string `json:"flags"`
	MinRate       int      `json:"min_rate"`
	MinDuration   int64    `json:"min_duration"`
	Fail          []int    `json:"fail"`
	EndOnStart    []int    `json:"end_on_start"`
	EndReturn     []int    `json:"end_return"`
	EndOnEnd      []int    `json:"end_on_end"`
	Script        string   `json:"script"`
}

// Flag is one of rAthena's e_status_change_flag values: a property of a
// status, such as whether death ends it.
type Flag uint8

// Flags of status.yml, as SCF_ names without the prefix.
const (
	FlagBLEffect Flag = iota
	FlagDisplayPC
	FlagNoClearBuff
	FlagNoRemoveOnDead
	FlagNoDispell
	FlagNoClearance
	FlagNoBanishingBuster
	FlagNoSave
	FlagNoSaveInfinite
	FlagRemoveOnDamaged
	FlagRemoveOnRefresh
	FlagRemoveOnLuxAnima
	FlagStopAttacking
	FlagStopCasting
	FlagStopWalking
	FlagBossResist
	FlagMVPResist
	FlagSetStand
	FlagFailedMado
	FlagDebuff
	FlagRemoveOnChangeMap
	FlagRemoveOnMapWarp
	FlagRemoveChemicalProtect
	FlagOverlapIgnoreLevel
	FlagSendOption
	FlagOnTouch
	FlagUnitMove
	FlagNonPlayer
	FlagSendLook
	FlagDisplayNPC
	FlagRequireWeapon
	FlagRequireShield
	FlagMobLoseTarget
	FlagRemoveElementalOption
	FlagSuperNoviceAngel
	FlagTaekwonAngel
	FlagMadoCancel
	FlagMadoEndCancel
	FlagRestartOnMapWarp
	FlagSpreadEffect
	FlagSendVal1
	FlagSendVal2
	FlagSendVal3
	FlagNoForcedEnd
	FlagNoWarning
	FlagRemoveOnUnequip
	FlagRemoveOnUnequipWeapon
	FlagRemoveOnUnequipArmor
	FlagRemoveOnHermode
	FlagRequireNoWeapon
	FlagRemoveFromHomOnWarp
	FlagRemoveFromHomOnMapWarp
	flagCount
)

// flagNames are the status.yml names of the flags, in Flag order.
var flagNames = [flagCount]string{
	"BLEffect", "DisplayPc", "NoClearbuff", "NoRemoveOnDead", "NoDispell", "NoClearance",
	"NoBanishingBuster", "NoSave", "NoSaveInfinite", "RemoveOnDamaged", "RemoveOnRefresh",
	"RemoveOnLuxAnima", "StopAttacking", "StopCasting", "StopWalking", "BossResist", "MvpResist",
	"SetStand", "FailedMado", "Debuff", "RemoveOnChangeMap", "RemoveOnMapWarp",
	"RemoveChemicalProtect", "OverlapIgnoreLevel", "SendOption", "OnTouch", "UnitMove", "NonPlayer",
	"SendLook", "DisplayNpc", "RequireWeapon", "RequireShield", "MobLoseTarget",
	"RemoveElementalOption", "SuperNoviceAngel", "TaekwonAngel", "MadoCancel", "MadoEndCancel",
	"RestartOnMapWarp", "SpreadEffect", "SendVal1", "SendVal2", "SendVal3", "NoForcedEnd",
	"NoWarning", "RemoveOnUnequip", "RemoveOnUnequipWeapon", "RemoveOnUnequipArmor",
	"RemoveOnHermode", "RequireNoWeapon", "RemoveFromHomOnWarp", "RemoveFromHomOnMapWarp",
}

// Flags is a set of flags.
type Flags uint64

// Has reports whether the set holds a flag.
func (f Flags) Has(x Flag) bool { return f&(1<<x) != 0 }

// With returns the set with a flag added.
func (f Flags) With(x Flag) Flags { return f | 1<<x }

// State is one of rAthena's e_scs_flag values: something a status keeps
// its unit from doing.
type State uint8

// States of status.yml, as SCS_ names without the prefix. The
// conditional states of rAthena have no status that this package ports.
const (
	StateNoMove State = iota
	StateNoPickItem
	StateNoDropItem
	StateNoCast
	StateNoChat
	StateNoEquipItem
	StateNoUnequipItem
	StateNoConsumeItem
	StateNoAttack
	StateNoWarp
	StateNoDeathPenalty
	StateNoInteract
	stateCount
)

var stateNames = [stateCount]string{
	"NoMove", "NoPickItem", "NoDropItem", "NoCast", "NoChat", "NoEquipItem", "NoUnequipItem",
	"NoConsumeItem", "NoAttack", "NoWarp", "NoDeathPenalty", "NoInteract",
}

// States is a set of states.
type States uint32

// Has reports whether the set holds a state.
func (s States) Has(x State) bool { return s&(1<<x) != 0 }

// Opt1 values, as rAthena's e_sc_opt1: the body state a status gives.
const (
	Opt1None      = 0
	Opt1Stone     = 1
	Opt1Freeze    = 2
	Opt1Stun      = 3
	Opt1Sleep     = 4
	Opt1StoneWait = 6
	Opt1Burning   = 7
	Opt1Imprison  = 8
)

// Option bits that this package reads, as rAthena's e_option.
const (
	OptionSight   = 0x1
	OptionHide    = 0x2
	OptionCloak   = 0x4
	OptionRuwach  = 0x2000
	OptionWedding = 0x1000
)

// maxSkillLevel is MAX_SKILL_LEVEL, the levels skill_db keeps.
const maxSkillLevel = 13

// Def is one status: s_status_change_db, and for a status from a mod,
// what statuses.define gave it.
type Def struct {
	ID int
	// Name is the status's name in wa.statuses, such as stone, or the
	// full name of a mod's status, such as example.glow:glow.
	Name string
	// Constant is rAthena's name, such as SC_STONE. A mod's status has
	// none.
	Constant string
	// Icon is the EFST_ value of the status icon, or -1 for none.
	Icon int
	// DurationSkill is the DurationLookup skill, and Durations and
	// Durations2 its Duration1 and Duration2 for levels 1 to 13.
	DurationSkill         int
	Durations, Durations2 [maxSkillLevel]int
	States                States
	// CondStates is set when the status has conditional states, which
	// status_calc_state_sub checks in calcState.
	CondStates bool
	// CalcAll is set when the status changes every stat (SCB_ALL).
	CalcAll bool
	// Calc are the SCB_ names of what the status changes, lower case.
	Calc                []string
	Opt1                int
	Opt2, Opt3, Options int
	Flags               Flags
	MinRate             int
	MinDuration         int64
	// Fail, EndOnStart, EndReturn and EndOnEnd are statuses, in file
	// order.
	Fail, EndOnStart, EndReturn, EndOnEnd []int
	// Script is the status's script in status.yml, and Bonuses the
	// bonuses it converts to. A status with a script changes stats
	// through its bonuses.
	Script  string
	Bonuses []Template
	// Mod is the mod that defined the status, or "" for core's.
	Mod string
	// TickMs is a mod status's tick_milliseconds: how often its
	// status_tick event fires, or 0.
	TickMs int64
}

// Changes reports whether the status changes stats, so its start and
// end need a stat calculation. A mod status may have bonuses from its
// definition's bonuses function.
func (d *Def) Changes() bool {
	return d.CalcAll || len(d.Calc) > 0 || len(d.Bonuses) > 0 || d.Mod != ""
}

// Calcs reports whether status_calc_bl recomputes a stat for the
// status, by its SCB_ name in lower case, such as aspd. SCB_ALL, a
// script's bonuses and a mod status recompute every stat.
func (d *Def) Calcs(name string) bool {
	return d.CalcAll || len(d.Bonuses) > 0 || d.Mod != "" || slices.Contains(d.Calc, name)
}

// Duration is skill_get_time of the status's DurationLookup skill at a
// level.
func (d *Def) Duration(level int) int { return d.skillLevel(&d.Durations, level) }

// Duration2 is skill_get_time2 of the DurationLookup skill at a level.
func (d *Def) Duration2(level int) int { return d.skillLevel(&d.Durations2, level) }

// skillLevel is skill_get_lv: levels above 13 continue the last two
// levels' trend.
func (d *Def) skillLevel(arr *[maxSkillLevel]int, level int) int {
	if d.DurationSkill == 0 || level <= 0 {
		return 0
	}
	idx := min(level, maxSkillLevel) - 1
	if level > maxSkillLevel && arr[idx] > 1 && idx > 1 {
		a, b, c := arr[idx-2], arr[idx-1], arr[idx]
		return c + (level-maxSkillLevel+1)*(b-a)/2 + (level-maxSkillLevel)*(c-b)/2
	}
	return arr[idx]
}

// DB is the status database of one mode: core's statuses from
// status.json, and the statuses mods define. Zones share it, so a mod
// status keeps one ID in every zone.
type DB struct {
	renewal bool
	core    []*Def
	byName  map[string]*Def

	mu      sync.RWMutex
	mods    map[int]*Def
	modName map[string]*Def
	nextMod int
}

// ModStart is the first ID of statuses that mods define, above every
// SC_ value.
const ModStart = 100_000

// Load builds a mode's status database from status.json.
func Load(d *Data) (*DB, error) {
	db := &DB{byName: map[string]*Def{}, mods: map[int]*Def{}, modName: map[string]*Def{}, nextMod: ModStart}
	switch d.Mode {
	case "renewal":
		db.renewal = true
	case "pre-renewal":
	default:
		return nil, fmt.Errorf("statuschange: unknown mode %q", d.Mode)
	}
	maxID := 0
	for i := range d.Statuses {
		maxID = max(maxID, d.Statuses[i].ID)
	}
	db.core = make([]*Def, maxID+1)
	for i := range d.Statuses {
		s := &d.Statuses[i]
		if s.ID < 0 || s.ID >= ModStart {
			return nil, fmt.Errorf("statuschange: status %s has ID %d", s.Name, s.ID)
		}
		def, err := newDef(s)
		if err != nil {
			return nil, err
		}
		db.core[s.ID] = def
		if def.Name != "" {
			db.byName[def.Name] = def
		}
	}
	return db, nil
}

func newDef(s *DataStatus) (*Def, error) {
	def := &Def{
		ID: s.ID, Constant: s.Constant, Icon: s.Icon, DurationSkill: s.DurationSkill,
		Opt1: s.Opt1, Opt2: s.Opt2, Opt3: s.Opt3, Options: s.Options,
		MinRate: s.MinRate, MinDuration: s.MinDuration,
		Fail: s.Fail, EndOnStart: s.EndOnStart, EndReturn: s.EndReturn, EndOnEnd: s.EndOnEnd,
		Script: s.Script,
	}
	def.Name = constants.Readable(s.Constant, "SC_")
	copy(def.Durations[:], s.Durations)
	copy(def.Durations2[:], s.Durations2)
	for _, n := range s.States {
		i := slices.IndexFunc(stateNames[:], func(x string) bool { return strings.EqualFold(x, n) })
		if i < 0 {
			// Conditional states (NoMoveCond and the like) have no ported
			// condition, as status_calc_state_sub reports for them.
			if strings.HasSuffix(strings.ToLower(n), "cond") {
				def.CondStates = true
				continue
			}
			return nil, fmt.Errorf("statuschange: %s: unknown state %s", s.Name, n)
		}
		def.States |= 1 << i
	}
	for _, n := range s.CalcFlags {
		if strings.EqualFold(n, "All") {
			def.CalcAll = true
			continue
		}
		def.Calc = append(def.Calc, strings.ToLower(n))
	}
	for _, n := range s.Flags {
		i := slices.IndexFunc(flagNames[:], func(x string) bool { return strings.EqualFold(x, n) })
		if i < 0 {
			return nil, fmt.Errorf("statuschange: %s: unknown flag %s", s.Name, n)
		}
		def.Flags = def.Flags.With(Flag(i)) // #nosec G115 -- i indexes flagNames, which has flagCount entries.
	}
	return def, nil
}

// Renewal reports whether the database is renewal's.
func (db *DB) Renewal() bool { return db.renewal }

// Get returns a status by ID, or nil.
func (db *DB) Get(id int) *Def {
	if id >= 0 && id < len(db.core) {
		return db.core[id]
	}
	if id < ModStart {
		return nil
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.mods[id]
}

// ByName returns a status by its name in wa.statuses, or by a mod
// status's full name.
func (db *DB) ByName(name string) *Def {
	if d := db.byName[name]; d != nil {
		return d
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.modName[name]
}

// Core returns core's statuses in ID order.
func (db *DB) Core() []*Def {
	out := make([]*Def, 0, len(db.core))
	for _, d := range db.core {
		if d != nil {
			out = append(out, d)
		}
	}
	return out
}

// SetBonuses sets the bonuses a core status's script converts to.
func (db *DB) SetBonuses(id int, b []Template) error {
	d := db.Get(id)
	if d == nil || d.Mod != "" {
		return fmt.Errorf("statuschange: no core status %d", id)
	}
	d.Bonuses = b
	return nil
}

// MaxModStatuses is the most statuses one mod may define. Defining a
// status again, such as in another zone, does not count (review decision
// 52).
const MaxModStatuses = 256

// ModSpec is what statuses.define gives a mod's status.
type ModSpec struct {
	// Mod is the mod's ID, and Name the status's name in wa.statuses.
	Mod, Name string
	Icon      int
	// Debuff is set for kind = "debuff".
	Debuff bool
	// EndsOn are the names of ends_on: death, dispell, clearance,
	// logout, banishing_buster, refresh, lux_anima, magic_gear_change
	// and damage.
	EndsOn []string
	TickMs int64
}

// EndsOnNames are the names that ends_on takes, for statuses.define and
// player:add_timed_bonuses (0002a, "bonus_script").
var EndsOnNames = []string{"death", "dispell", "clearance", "logout", "banishing_buster", "refresh", "lux_anima", "magic_gear_change", "damage"}

// Define adds a mod's status, or returns the one the mod defined before
// under the name, such as in another zone. Another mod's status, or a
// core status, with the same name is an error.
func (db *DB) Define(s ModSpec) (*Def, error) {
	full := s.Mod + ":" + s.Name
	if db.byName[s.Name] != nil {
		return nil, fmt.Errorf("statuses.define: statuses.%s is a status of the game already", s.Name)
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	own := 0
	for _, d := range db.mods {
		if d.Mod != s.Mod && strings.TrimPrefix(d.Name, d.Mod+":") == s.Name {
			return nil, fmt.Errorf("statuses.define: %s defined a status named %s already", d.Mod, s.Name)
		}
		if d.Mod == s.Mod {
			own++
		}
	}
	if db.modName[full] == nil && own >= MaxModStatuses {
		return nil, fmt.Errorf("statuses.define: %s has defined %d statuses, the most a mod may", s.Mod, MaxModStatuses)
	}
	def := &Def{Name: full, Icon: s.Icon, Mod: s.Mod, TickMs: s.TickMs, MinDuration: 1}
	// A mod status ends on what ends_on names, and on nothing else, so
	// the flags that keep a status are set for the rest.
	ends := map[string]bool{}
	for _, e := range s.EndsOn {
		if !slices.Contains(EndsOnNames, e) {
			return nil, fmt.Errorf("statuses.define: ends_on has no %q; it takes %s", e, strings.Join(EndsOnNames, ", "))
		}
		ends[e] = true
	}
	def.Flags = EndsOnFlags(ends)
	if s.Debuff {
		def.Flags = def.Flags.With(FlagDebuff)
	}
	if old := db.modName[full]; old != nil {
		// A reload or another zone: keep the ID, take the new settings.
		def.ID = old.ID
	} else {
		def.ID = db.nextMod
		db.nextMod++
	}
	db.mods[def.ID] = def
	db.modName[full] = def
	return def, nil
}

// EndsOnFlags turns ends_on names into status flags, as pc_bonus_script
// flags map to removals: what is not named keeps the status.
func EndsOnFlags(ends map[string]bool) Flags {
	var f Flags
	if !ends["death"] {
		f = f.With(FlagNoRemoveOnDead)
	}
	if !ends["dispell"] {
		f = f.With(FlagNoDispell)
	}
	if !ends["clearance"] {
		f = f.With(FlagNoClearance)
	}
	if !ends["banishing_buster"] {
		f = f.With(FlagNoBanishingBuster)
	}
	if ends["logout"] {
		f = f.With(FlagNoSave)
	}
	if ends["refresh"] {
		f = f.With(FlagRemoveOnRefresh)
	}
	if ends["lux_anima"] {
		f = f.With(FlagRemoveOnLuxAnima)
	}
	if ends["magic_gear_change"] {
		f = f.With(FlagMadoCancel)
	}
	if ends["damage"] {
		f = f.With(FlagRemoveOnDamaged)
	}
	return f
}
