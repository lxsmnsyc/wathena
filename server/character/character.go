// Package character holds a saved character: what the zone loads when a
// player enters and what it hands to the persistence layer to save. It is
// plain data, so the zone and the store do not depend on each other.
package character

import (
	"maps"
	"slices"
	"time"

	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/status"
)

// Character is one character's saved state.
type Character struct {
	Info
	// Items are the filled inventory slots, in slot order.
	Items []Item
	// Variables are the mods' saved variables of the character and of
	// its account (proposal 0002, "Variables"), as JSON. A variable at
	// its default value is not in the map.
	Variables map[Variable]string
	// Statuses are the statuses that rAthena saves (sc_data), in status
	// order.
	Statuses []Status
	// TimedBonuses are the timed bonuses of player:add_timed_bonuses
	// (rAthena's bonus_script), in the order they started.
	TimedBonuses []TimedBonus
	// Skills are the skills the character learned with skill points, by
	// skill ID, with their level.
	Skills map[int]int
	// Blocks are the character names the character blocks, in the order
	// they were added (strategy section 7.10). rAthena keeps its ignore
	// list for the session only; wAthena saves it.
	Blocks []string
	// Sanctions are the character's mutes and jails that have not ended.
	Sanctions []Sanction
}

// Kinds of sanctions.
const (
	SanctionMute = "mute"
	SanctionJail = "jail"
)

// Sanction is a mute or a jail (strategy section 7.10). The database
// keeps every sanction, ended ones too, for the staff tools.
type Sanction struct {
	// Kind is SanctionMute or SanctionJail.
	Kind   string
	Reason string
	// Start and End are when it started and ends, on the server clock.
	Start, End time.Time
	// By names who gave it: a mod's ID, or a staff member.
	By string
	// ReturnMap, ReturnX and ReturnY are where a jailed character goes
	// back to when the jail ends, as SC_JAILED's val3 and val4.
	ReturnMap        string
	ReturnX, ReturnY int
}

// Equal reports whether two sanctions are the same.
func (s *Sanction) Equal(o *Sanction) bool {
	return s.Kind == o.Kind && s.Reason == o.Reason && s.Start.Equal(o.Start) && s.End.Equal(o.End) && s.By == o.By &&
		s.ReturnMap == o.ReturnMap && s.ReturnX == o.ReturnX && s.ReturnY == o.ReturnY
}

// Status is one saved status.
type Status struct {
	// Name is the status's name in wa.statuses, such as blessing, or a
	// mod status's full name, such as example.glow:glow.
	Name string
	// Values are val1 to val4.
	Values [4]int
	// Tick is the time to the status's next timer, in milliseconds, as
	// chrif_save_scdata saves it, or -1 for a status without an end.
	Tick int64
	// End is the time to the end of a mod status that ticks, or 0.
	End int64
}

// TimedBonus is one saved timed bonus.
type TimedBonus struct {
	// Key names it. A character may have several with one key, as the
	// on_repeat mode "add" of player:add_timed_bonuses gives.
	Key string
	// Remaining is the time left, in milliseconds.
	Remaining int64
	// Bonuses are the bonus entries as JSON: a list of [name, args...,
	// options], as player:add_timed_bonuses takes them.
	Bonuses string
	// EndsOn are the names of what ends it, such as death.
	EndsOn []string
	// Icon is the EFST_ icon, or -1 for none. Kind is "buff", "debuff"
	// or "" for neither.
	Icon int
	Kind string
}

// Equal reports whether two timed bonuses are the same.
func (t *TimedBonus) Equal(o *TimedBonus) bool {
	return t.Key == o.Key && t.Remaining == o.Remaining && t.Bonuses == o.Bonuses && slices.Equal(t.EndsOn, o.EndsOn) &&
		t.Icon == o.Icon && t.Kind == o.Kind
}

// Variable names one saved variable of a mod.
type Variable struct {
	Mod, Name string
	// Account is set for an account variable, which every character of
	// the account shares.
	Account bool
}

// Info is a character's state other than its items.
type Info struct {
	// ID and AccountID are database IDs. They are 0 for a character
	// that only lives in memory. They are never sent to clients.
	ID, AccountID int64
	// Slot is the character's place on the account's character list.
	Slot int
	Name string
	Job  int
	Male bool
	// Hair, HairColor and ClothColor are look IDs from the client data
	// tables.
	Hair, HairColor, ClothColor int
	BaseLevel, JobLevel         int
	BaseExp, JobExp             int64
	StatusPoints, SkillPoints   int
	// Stats are the raised stats, without job bonuses.
	Stats status.Stats
	// HP is 0 for a character that logged out dead.
	HP, SP int64
	// Map is the zone's map name, such as map/prontera.
	Map  string
	X, Y int
	Zeny int64
	// InventorySize is inventory_slots, the slots the character may fill.
	InventorySize int
	// SaveMap is the map of the save point, as rAthena's save_point,
	// such as map/prontera. Empty is the world's start point.
	SaveMap      string
	SaveX, SaveY int
	// JobChangeLevel is rAthena's change_level_2nd (the JOBCHANGE2ND_VAR
	// variable): the job level at the change from a first class to a
	// second class, or 0.
	JobChangeLevel int
	// RebirthJob is the second class a transcendent character had before
	// its rebirth, which picks its transcendent line, or 0.
	RebirthJob int
	// Options are the saved bits of rAthena's status.option: the cart,
	// and the looks of the companion and the mount, such as the falcon
	// (OPTION_FALCON) and the Peco Peco (OPTION_RIDING, 0x20).
	Options int
	// Companion and Mount are the full names of the character's
	// companion and mount records, or empty for none. A character saved
	// with empty names and look bits in Options has core's.
	Companion, Mount string
	// CreatedAt is when the character was made. Chat limits are lower in
	// its first hour (strategy section 7.10).
	CreatedAt time.Time
}

// Item is one filled inventory slot.
type Item struct {
	Slot   int
	ID     uint32
	Amount int
	Serial item.Serial
	// Equip holds the positions the item is equipped in, as EQP_ bits, or
	// 0.
	Equip uint32
	// Refine is the refine level.
	Refine int
	// Cards are the cards in the item's slots, in slot order.
	Cards []uint32
	// Extra holds the rolls, the cards' rolls, the random options and
	// whether the copy is identified (strategy section 5.4).
	item.Extra
}

// Equal reports whether two items are the same.
func (it *Item) Equal(o *Item) bool {
	return it.Slot == o.Slot && it.ID == o.ID && it.Amount == o.Amount && it.Serial == o.Serial &&
		it.Equip == o.Equip && it.Refine == o.Refine && slices.Equal(it.Cards, o.Cards) && it.Extra.Equal(&o.Extra)
}

// Equal reports whether two characters hold the same data.
func (c *Character) Equal(o *Character) bool {
	return c.equal(&o.Info) && slices.EqualFunc(c.Items, o.Items, func(a, b Item) bool { return a.Equal(&b) }) &&
		maps.Equal(c.Variables, o.Variables) && slices.Equal(c.Statuses, o.Statuses) &&
		slices.EqualFunc(c.TimedBonuses, o.TimedBonuses, func(a, b TimedBonus) bool { return a.Equal(&b) }) &&
		maps.Equal(c.Skills, o.Skills) && slices.Equal(c.Blocks, o.Blocks) &&
		slices.EqualFunc(c.Sanctions, o.Sanctions, func(a, b Sanction) bool { return a.Equal(&b) })
}

// Clone returns a copy of c that shares no slices or maps with it.
func (c *Character) Clone() *Character {
	out := &Character{Info: c.Info, Variables: maps.Clone(c.Variables), Statuses: slices.Clone(c.Statuses),
		Skills: maps.Clone(c.Skills), Blocks: slices.Clone(c.Blocks), Sanctions: slices.Clone(c.Sanctions)}
	for _, it := range c.Items {
		it.Cards = slices.Clone(it.Cards)
		it.Extra = it.Clone()
		out.Items = append(out.Items, it)
	}
	for _, b := range c.TimedBonuses {
		b.EndsOn = slices.Clone(b.EndsOn)
		out.TimedBonuses = append(out.TimedBonuses, b)
	}
	return out
}

// equal reports whether two infos are the same. Times compare as
// instants, without their monotonic readings.
func (i *Info) equal(o *Info) bool {
	a, b := *i, *o
	a.CreatedAt, b.CreatedAt = time.Time{}, time.Time{}
	return a == b && i.CreatedAt.Equal(o.CreatedAt)
}

// Sex is the protocol's and rAthena's sex: 1 for male, 0 for female.
func (c *Character) Sex() uint32 {
	if c.Male {
		return 1
	}
	return 0
}

// MaxZeny is MAX_ZENY.
const MaxZeny = 2_147_483_647
