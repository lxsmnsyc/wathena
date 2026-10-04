package zone

import (
	"fmt"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/storage"
	"github.com/lxsmnsyc/wathena/server/transfer"
)

// The storage calls of scripts (proposal 0002, review decision 29):
// player:open_storage, player:open_guild_storage, the from and to
// options of count_item, give_item and take_item, and guild:count_item.

// place is a storage that a script names: "storage", "cart" or an extra
// storage's name.
type place struct {
	st   *storage.Storage
	rec  *storage.Record
	cart bool
}

// placeOf finds the storage a script names. mod is the calling mod.
func (z *Zone) placeOf(p *Player, where, mod string) (place, error) {
	switch where {
	case "storage":
		return place{st: z.findStorage(p, nil)}, nil
	case "cart":
		return place{st: z.cartOf(p), cart: true}, nil
	}
	rec := z.storageRecord(where, mod)
	if rec == nil {
		return place{}, fmt.Errorf("there is no storage %q; the places are \"storage\", \"cart\" and the records of data/storages.yml", where)
	}
	return place{st: z.findStorage(p, rec), rec: rec}, nil
}

// prepare locks a place's storage and sets up its box. The caller
// unlocks it.
func (z *Zone) prepare(pl place) *transfer.ListBox {
	pl.st.Lock()
	switch {
	case pl.cart:
		return z.prepareCart(pl.st)
	case pl.rec != nil:
		return pl.st.Prepare(z.Items, pl.rec.Slots, 0)
	}
	return pl.st.Prepare(z.Items, z.limits().StorageSlots, 0)
}

// shownKind is the kind of a place the player sees: its open storage or
// its cart. ok is false when the player does not see the place.
func (z *Zone) shownKind(p *Player, pl place) (wathenav1.StorageKind, bool) {
	switch {
	case pl.cart && z.hasCart(p):
		return wathenav1.StorageKind_STORAGE_KIND_CART, true
	case !pl.cart && p.storage != nil && p.storage.st == pl.st:
		return wathenav1.StorageKind_STORAGE_KIND_PERSONAL, true
	}
	return 0, false
}

// OpenStorage is player:open_storage.
func (h scriptHost) OpenStorage(player int64, name, mod string) (bool, error) {
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	var rec *storage.Record
	if name != "" {
		if rec = h.z.storageRecord(name, mod); rec == nil {
			return false, fmt.Errorf("player:open_storage: there is no storage %q in data/storages.yml", name)
		}
	}
	return h.z.openPersonalStorage(p, rec), nil
}

// StorageCount is player:count_item with from, as storagecountitem and
// cartcountitem.
func (h scriptHost) StorageCount(player int64, where, mod string, itemID uint32) (int, bool, error) {
	p, err := h.player(player)
	if err != nil {
		return 0, false, err
	}
	pl, err := h.z.placeOf(p, where, mod)
	if err != nil {
		return 0, false, fmt.Errorf("player:count_item: %w", err)
	}
	n := h.z.countStorage(p, pl.st, pl.cart, itemID)
	return n, n >= 0, nil
}

// StorageGive is player:give_item with to or bound. Items for a storage
// follow its restrictions and bound rules, as storage_additem.
func (h scriptHost) StorageGive(player int64, where, mod string, itemID uint32, amount int, roll modscript.GiveRoll, bound item.Bound) (bool, error) {
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	z := h.z
	if where == "" {
		ok, err := z.giveRolled(p, itemID, amount, roll, bound)
		if err != nil {
			return false, fmt.Errorf("player:give_item: %w", err)
		}
		return ok, nil
	}
	pl, err := z.placeOf(p, where, mod)
	if err != nil {
		return false, fmt.Errorf("player:give_item: %w", err)
	}
	if pl.st == nil || pl.cart && !z.hasCart(p) {
		return false, nil
	}
	d := z.Items.Get(itemID)
	limit, at := item.NoStorage, item.PlaceStorage
	if pl.cart {
		limit, at = item.NoCart, item.PlaceCart
	}
	if d.RestrictionMask()&limit != 0 || !item.BoundAllows(bound, at, false) {
		return false, nil
	}
	if roll.Mode == modscript.RollSet {
		for name := range roll.Values {
			if d.Rolls[name] == nil {
				return false, fmt.Errorf("player:give_item: item %s has no roll %q", d.AegisName, name)
			}
		}
	}
	step := amount
	if !d.Stackable {
		step = 1
	}
	box := z.prepare(pl)
	defer pl.st.Unlock()
	tx := transfer.New(transfer.ActionGive)
	if z.Clock != nil {
		tx.Now = z.Clock()
	}
	for given := 0; given < amount; given += step {
		x := z.newExtra(d, roll.Mode == modscript.RollRandom, rollSource{kind: sourceScript}, "")
		if roll.Mode == modscript.RollSet {
			for i := range x.Rolls {
				if v, ok := roll.Values[x.Rolls[i].Name]; ok {
					x.Rolls[i].Value = d.Rolls[x.Rolls[i].Name].Clamp(v)
				}
			}
		}
		x.Bound = bound
		tx.Create(scriptOrigin.from, item.Slot{ID: itemID, Extra: x}, step, box, item.Serial{})
	}
	kind, shown := z.shownKind(p, pl)
	logs, err := tx.Apply(func(c transfer.Change) {
		if shown {
			z.storageChange(p, c, &end{}, &end{ok: true, kind: kind, box: box, st: pl.st})
		}
	})
	if err != nil {
		return false, nil
	}
	z.recordTransfer(p, logs)
	z.personalChanged(pl.st)
	if pl.cart {
		// The cart's lock is held, so its weight comes from the box.
		z.setCartWeight(p, box.Weight)
	}
	return true, nil
}

// StorageTake is player:take_item with from, as storagedelitem and
// cartdelitem: it takes nothing when the storage holds less.
func (h scriptHost) StorageTake(player int64, where, mod string, itemID uint32, amount int) (bool, error) {
	p, err := h.player(player)
	if err != nil {
		return false, err
	}
	z := h.z
	pl, err := z.placeOf(p, where, mod)
	if err != nil {
		return false, fmt.Errorf("player:take_item: %w", err)
	}
	if pl.st == nil || pl.cart && !z.hasCart(p) {
		return false, nil
	}
	box := z.prepare(pl)
	defer pl.st.Unlock()
	have := 0
	for _, s := range box.Slots {
		if s.ID == itemID {
			have += s.Amount
		}
	}
	if have < amount {
		return false, nil
	}
	tx := transfer.New(transfer.ActionTake)
	if z.Clock != nil {
		tx.Now = z.Clock()
	}
	left := amount
	for i := range box.Slots {
		s := &box.Slots[i]
		if left == 0 {
			break
		}
		if s.ID != itemID || s.Amount <= 0 {
			continue
		}
		n := min(s.Amount, left)
		tx.Destroy(box, i, s.Serial, n, transfer.End(transfer.KindScript))
		left -= n
	}
	kind, shown := z.shownKind(p, pl)
	logs, err := tx.Apply(func(c transfer.Change) {
		if shown {
			z.storageChange(p, c, &end{ok: true, kind: kind, box: box, st: pl.st}, &end{})
		}
	})
	if err != nil {
		return false, nil
	}
	z.recordTransfer(p, logs)
	z.personalChanged(pl.st)
	if pl.cart {
		// The cart's lock is held, so its weight comes from the box.
		z.setCartWeight(p, box.Weight)
	}
	return true, nil
}

// GuildCountItem is guild:count_item, as guildstoragecountitem.
func (h scriptHost) GuildCountItem(guild int64, itemID uint32) (int, error) {
	if !h.z.socialHub().GuildRuns(guild) {
		return 0, fmt.Errorf("guild:count_item: there is no guild %d", guild)
	}
	return h.z.socialHub().GuildStorageCount(guild, itemID), nil
}
