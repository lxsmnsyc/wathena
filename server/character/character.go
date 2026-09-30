// Package character holds a saved character: what the zone loads when a
// player enters and what it hands to the persistence layer to save. It is
// plain data, so the zone and the store do not depend on each other.
package character

import (
	"slices"

	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/status"
)

// Character is one character's saved state.
type Character struct {
	Info
	// Items are the filled inventory slots, in slot order.
	Items []Item
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
}

// Item is one filled inventory slot.
type Item struct {
	Slot   int
	ID     uint32
	Amount int
	Serial item.Serial
}

// Equal reports whether two characters hold the same data.
func (c *Character) Equal(o *Character) bool {
	return c.Info == o.Info && slices.Equal(c.Items, o.Items)
}

// Sex is the protocol's and rAthena's sex: 1 for male, 0 for female.
func (c *Character) Sex() uint32 {
	if c.Male {
		return 1
	}
	return 0
}
