package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// Rentals that expire (review decision 58, area B9, decision 0064), as
// rAthena's pc_inventory_rentals (pc.cpp 1200):
//   - A rental's end is in Unix seconds of the server clock, so it runs
//     while the player is offline: a rental that ended is gone at login.
//   - The zone checks its players' inventories once a second. An expired
//     rental comes off if worn and is deleted, and the player is told,
//     as clif_rental_expired.
//   - Rentals in a storage or a cart do not expire there, as rAthena
//     checks the inventory alone. They expire once back in it.

// rentalCheckEvery is how often the zone checks rentals, in zone
// milliseconds.
const rentalCheckEvery = 1000

// expireRentals checks the rentals of every player, once a second.
func (z *Zone) expireRentals() {
	if z.now < z.nextRentalCheck {
		return
	}
	z.nextRentalCheck = z.now + rentalCheckEvery
	now := z.wallSeconds()
	for _, p := range z.playersInOrder() {
		z.expirePlayerRentals(p, now)
	}
}

// expirePlayerRentals deletes a player's rentals that ended by now.
func (z *Zone) expirePlayerRentals(p *Player, now int64) {
	if p.inventory == nil || p.entity == nil {
		return
	}
	for i := range p.inventory.Slots {
		s := &p.inventory.Slots[i]
		if s.ID == 0 || s.Amount <= 0 || s.Expires == 0 || s.Expires > now {
			continue
		}
		z.deleteItem(p, i, s.Amount, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_EXPIRED)
	}
}
