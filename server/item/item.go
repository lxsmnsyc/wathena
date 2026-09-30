// Package item ports a player's inventory from rAthena: slots, stacking,
// weight, and the heal of healing items.
//
// This is a port of parts of rAthena's pc.cpp, script.cpp and status.cpp
// (GPL-3.0). Function comments name the rAthena functions they port. The
// settings are the defaults from rAthena's conf/battle files.
package item

import "strings"

// Limits from rAthena's mmo.hpp for recent clients.
const (
	// MaxInventory is MAX_INVENTORY: INVENTORY_BASE_SIZE plus
	// INVENTORY_EXPANSION_SIZE.
	MaxInventory = 200
	// BaseSlots is INVENTORY_BASE_SIZE, the inventory_slots a new
	// character has. Slots from it on are for inventory expansions.
	BaseSlots = 100
	// MaxAmount is MAX_AMOUNT, the most of one item a slot holds.
	MaxAmount = 30000
)

// Rand is the random source heal amounts draw from. IntN returns a number
// in [0, n). It must draw once even when n is 1, as rnd_value does.
type Rand interface {
	IntN(n int) int
}

// Config holds the battle settings this package reads.
type Config struct {
	// NaturalHealWeightRate is natural_heal_weight_rate: from this
	// percent of the maximum weight, natural HP and SP regen stops.
	NaturalHealWeightRate int
	// MajorOverweightRate is major_overweight_rate: from this percent,
	// the player cannot attack or use skills.
	MajorOverweightRate int
	// OpenBoxWeightRate is open_box_weight_rate: from this percent, cash
	// items cannot be used.
	OpenBoxWeightRate int
	// ItemUseInterval is item_use_interval, the wait between two item
	// uses, in milliseconds.
	ItemUseInterval int64
}

// DefaultConfig returns rAthena's defaults for a mode. Renewal stops
// regen at 70% and opens boxes up to 90%. Pre-renewal uses 50% and 70%.
func DefaultConfig(renewal bool) Config {
	c := Config{NaturalHealWeightRate: 50, MajorOverweightRate: 90, OpenBoxWeightRate: 70, ItemUseInterval: 100}
	if renewal {
		c.NaturalHealWeightRate, c.OpenBoxWeightRate = 70, 90
	}
	return c
}

// Data is the part of an extracted items.json this package reads.
type Data struct {
	Mode  string `json:"mode"`
	Items []Def  `json:"items"`
}

// Def is one item from rAthena's item_db.
type Def struct {
	ID        uint32 `json:"id"`
	AegisName string `json:"aegis_name"`
	// Type is the item_db type, such as "Healing" or "Etc".
	Type string `json:"type"`
	// Weight is in tenths, as rAthena's.
	Weight    int  `json:"weight"`
	Stackable bool `json:"stackable"`
	// StackLimit is Stack.Amount when Stack.Inventory is set, or 0.
	StackLimit int  `json:"stack_limit"`
	UniqueID   bool `json:"unique_id"`
	NoConsume  bool `json:"no_consume"`
	NoDrop     bool `json:"no_drop"`
	// DelayMs is the item's reuse delay. With DelayStatus, the delay
	// belongs to that status and is shared by every item that names it.
	DelayMs     int64  `json:"delay_ms"`
	DelayStatus string `json:"delay_status"`
	// MinLevel and MaxLevel are EquipLevelMin and EquipLevelMax, or 0.
	MinLevel int `json:"min_level"`
	MaxLevel int `json:"max_level"`
	// Gender is "Male", "Female", or empty for both.
	Gender string `json:"gender"`
	// JobRestricted is set when Jobs or Classes leave some jobs out.
	JobRestricted bool `json:"job_restricted"`
	// Heal is the item's script when it only heals.
	Heal *Heal `json:"heal"`
}

// Heal is an item script that is only itemheal or percentheal. HP and SP
// are [min, max]. When they differ, the script draws with rand.
type Heal struct {
	Command string `json:"command"`
	HP      [2]int `json:"hp"`
	SP      [2]int `json:"sp"`
}

// Table holds the item database and the settings.
type Table struct {
	Config
	items map[uint32]*Def
}

// New builds the table. Data from a renewal extract gets renewal's
// settings.
func New(d *Data) *Table {
	t := &Table{Config: DefaultConfig(d.Mode == "renewal"), items: make(map[uint32]*Def, len(d.Items))}
	for i := range d.Items {
		t.items[d.Items[i].ID] = &d.Items[i]
	}
	return t
}

// unknown is itemdb's dummy item for IDs the database lacks: an Etc item
// with no weight.
var unknown = Def{Type: "Etc", Stackable: true}

// Get is itemdb_search: the item, or the dummy item. A nil table knows no
// items.
func (t *Table) Get(id uint32) *Def {
	if t != nil {
		if d, ok := t.items[id]; ok {
			return d
		}
	}
	d := unknown
	d.ID = id
	return &d
}

// Consumable is the type check of pc_isUseitem: only healing, usable and
// cash items can be used.
func (d *Def) Consumable() bool {
	switch strings.ToLower(d.Type) {
	case "healing", "usable", "cash", "delayconsume":
		return true
	}
	return false
}

// stacks is whether pc_additem adds to a slot with the same item:
// itemdb_isstackable2, and no unique ID, since each new copy of an item
// with UniqueId gets its own.
func (d *Def) stacks() bool { return d.Stackable && !d.UniqueID }
