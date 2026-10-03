// Package item ports a player's inventory from rAthena: slots, stacking,
// weight, and the heal of healing items.
//
// This is a port of parts of rAthena's pc.cpp, script.cpp and status.cpp
// (GPL-3.0). Function comments name the rAthena functions they port. The
// settings are the defaults from rAthena's conf/battle files.
package item

import (
	"strings"

	"github.com/lxsmnsyc/wathena/server/bonus"
)

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
	// Script is the item's entry script, as <mod ID>:<path>, such as
	// core:scripts/items/Red_Potion.lua. The mod loader sets it from the
	// script field of an item overlay.
	Script string `json:"script"`
	// Bonuses are static bonuses of an equipment item, as item data
	// entries: a bonus name followed by its arguments (server/bonus).
	Bonuses []any `json:"bonuses"`
	// Static are Bonuses read and checked, in order. The mod loader sets
	// them.
	Static []*bonus.Bonus `json:"-"`
	// Rolls are the item's rolled values (strategy section 5.4), by
	// name. Each new copy gets a value for each.
	Rolls map[string]*RollDef `json:"rolls"`
	// Unsupported says why the server cannot run the item's script yet.
	// Using such an item is refused before it is spent.
	Unsupported string `json:"unsupported"`
	// Name is the item's name in the server's language, from item_db's
	// Name. Scripts read it with items.name.
	Name string `json:"name"`
	// Buy and Sell are the prices NPC shops sell and buy the item for,
	// in zeny, before Discount and Overcharge (item_prices.json).
	Buy  int `json:"buy"`
	Sell int `json:"sell"`
	// NoSell is Trade.NoSell: NPC shops do not buy the item.
	NoSell bool `json:"no_sell"`

	// The equipment fields come from equipment.json (MergeEquipment).

	// Subtype is the weapon type (W_*) of a weapon, the ammunition type
	// (AMMO_*) of ammunition, and the card type of a card.
	Subtype int `json:"subtype"`
	// Location is item_data's equip: the positions the item goes in. A
	// card's location is where it can be compounded.
	Location Locations `json:"location"`
	// Atk, Matk and Defense are the weapon ATK, the weapon MATK (renewal)
	// and the armor DEF. Range is a weapon's attack range in cells.
	Atk     int `json:"atk"`
	Matk    int `json:"matk"`
	Defense int `json:"def"`
	Range   int `json:"range"`
	// Slots is how many cards the item holds.
	Slots       int  `json:"slots"`
	WeaponLevel int  `json:"weapon_level"`
	ArmorLevel  int  `json:"armor_level"`
	Refineable  bool `json:"refineable"`
	// View is the sprite the item shows on a character, as item_data's
	// look.
	View int `json:"view"`
	// Jobs is item_data's class_base. nil is every job.
	Jobs *JobMask `json:"jobs"`
	// Classes is item_data's class_upper: the ITEMJ_ bits of the classes
	// that may use the item. 0 is every class.
	Classes uint32 `json:"classes"`
	// NoEquip is item_data's flag.no_equip, from item_noequip.txt: the
	// map types where the item's script does not run (MapNormal and the
	// others).
	NoEquip uint32 `json:"no_equip"`
}

// Map types of NoEquip, as item_noequip.txt's flags.
const (
	MapNormal       uint32 = 1
	MapPvP          uint32 = 2
	MapGvG          uint32 = 4
	MapBattleground uint32 = 8
	MapWoETE        uint32 = 16
)

// Heal is the amounts of an itemheal or percentheal: HP and SP are
// [min, max]. When they differ, the script draws with rand.
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

// MaxZeny is MAX_ZENY, the most zeny a character holds, and the most a
// price may be.
const MaxZeny = 2_147_483_647

// Lookup returns an item the database has.
func (t *Table) Lookup(id uint32) (*Def, bool) {
	if t == nil {
		return nil, false
	}
	d, ok := t.items[id]
	return d, ok
}

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
