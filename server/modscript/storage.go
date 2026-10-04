package modscript

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/item"
)

// Storage, cart and bound items (proposal 0002, review decision 29):
//   - player:open_storage(name) opens the account's storage, or the extra
//     storage of data/storages.yml that name names. A name without a mod
//     prefix is the calling mod's record first, then core's. It returns
//     false when the player has a storage open, or another character of
//     the account has this one open.
//   - player:open_guild_storage() opens the guild storage. It returns
//     true, or false and why: "no_guild", "no_storage",
//     "no_permission", "in_use" or "already_open".
//   - player:count_item, give_item and take_item take { from } or
//     { to }: "storage", "cart" or a storage's name. count_item returns
//     nil when the storage cannot be read, as rAthena's -1: no cart, or a
//     storage open.
//   - give_item takes { bound = "account" | "character" | "party" |
//     "guild" }.
//   - guild:count_item(item) reads the guild storage.
//
// Opening needs no permission: scripts charge any fee. The item calls
// need the permissions they need without the option.

// StorageHost is what the storage calls need from the zone.
type StorageHost interface {
	// OpenStorage opens the account's storage for name "", or an extra
	// storage. mod is the calling mod, for names without a prefix.
	OpenStorage(player int64, name, mod string) (bool, error)
	// OpenGuildStorageFor opens the guild storage. reason says why it
	// did not open.
	OpenGuildStorageFor(player int64) (ok bool, reason string, err error)
	// StorageCount is the amount of an item in a storage. ok is false
	// when the storage cannot be read.
	StorageCount(player int64, where, mod string, item uint32) (n int, ok bool, err error)
	// StorageGive gives an amount of an item to the inventory (where
	// "") or a storage, bound as bound. It returns false when it does
	// not fit or may not go there.
	StorageGive(player int64, where, mod string, item uint32, amount int, roll GiveRoll, bound item.Bound) (bool, error)
	// StorageTake takes an amount of an item from a storage. It returns
	// false, and takes nothing, when the storage holds less.
	StorageTake(player int64, where, mod string, item uint32, amount int) (bool, error)
	// GuildCountItem is the amount of an item in a guild's storage.
	GuildCountItem(guild int64, item uint32) (int, error)
}

func (r *Runtime) storageHost(what string) (StorageHost, error) {
	h, ok := r.host.(StorageHost)
	if !ok {
		return nil, errors.New(what + ": storage does not exist here")
	}
	return h, nil
}

// whereArg reads the from or to option: "", "storage", "cart" or a
// storage's name.
func whereArg(what string, v script.Value) (string, error) {
	if v.IsNil() {
		return "", nil
	}
	s, ok := v.AsString()
	if !ok || s == "" || len(s) > 128 {
		return "", fmt.Errorf(`%s: the place is "storage", "cart" or a storage's name`, what)
	}
	return s, nil
}

// playerOpenStorage is player:open_storage(name).
func (r *Runtime) playerOpenStorage(c *script.Call) error {
	const what = "player:open_storage"
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.storageHost(what)
	if err != nil {
		return err
	}
	name := ""
	if !c.Arg(1).IsNil() {
		s, ok := c.Arg(1).AsString()
		if !ok || s == "" || len(s) > 128 {
			return errors.New(what + ": the name is a storage of data/storages.yml, or nothing for the account's storage")
		}
		name = s
	}
	ok, err := h.OpenStorage(id, name, r.actor())
	if err != nil {
		return err
	}
	c.Return(script.Bool(ok))
	return nil
}

// playerOpenGuildStorage is player:open_guild_storage().
func (r *Runtime) playerOpenGuildStorage(c *script.Call) error {
	const what = "player:open_guild_storage"
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.storageHost(what)
	if err != nil {
		return err
	}
	ok, reason, err := h.OpenGuildStorageFor(id)
	if err != nil {
		return err
	}
	if ok {
		c.Return(script.Bool(true))
		return nil
	}
	c.Return(script.Bool(false), script.String(reason))
	return nil
}

// playerStorageCount is player:count_item(item, { from }) with a place.
func (r *Runtime) playerStorageCount(c *script.Call) error {
	const what = "player:count_item"
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	itemID := c.Int(1)
	if itemID <= 0 || itemID > 1<<32-1 {
		return errors.New(what + ": want an item ID from items.id")
	}
	where, err := whereArg(what, c.Arg(2))
	if err != nil {
		return err
	}
	h, err := r.storageHost(what)
	if err != nil {
		return err
	}
	n, ok, err := h.StorageCount(id, where, r.actor(), uint32(itemID)) // #nosec G115 -- checked above.
	if err != nil {
		return err
	}
	if !ok {
		c.Return(script.Nil)
		return nil
	}
	c.Return(script.Int(int64(n)))
	return nil
}

// playerStorageTake is player:take_item(item, amount, { from }) with a
// place.
func (r *Runtime) playerStorageTake(c *script.Call) error {
	const what = "player:take_item"
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if _, err := r.npcHost(); err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	itemID, amount, err := itemAmount(c, what, 1)
	if err != nil {
		return err
	}
	where, err := whereArg(what, c.Arg(3))
	if err != nil {
		return err
	}
	h, err := r.storageHost(what)
	if err != nil {
		return err
	}
	ok, err := h.StorageTake(id, where, r.actor(), itemID, amount)
	if err != nil {
		return err
	}
	c.Return(script.Bool(ok))
	return nil
}

// giveOptions reads the to and bound options of player:give_item, the
// arguments after the roll.
func giveOptions(what string, to, bound script.Value) (string, item.Bound, error) {
	where, err := whereArg(what, to)
	if err != nil {
		return "", 0, err
	}
	b := item.BoundNone
	if !bound.IsNil() {
		s, _ := bound.AsString()
		var ok bool
		if b, ok = item.ParseBound(s); !ok {
			return "", 0, errors.New(what + `: bound is "account", "character", "party" or "guild"`)
		}
	}
	return where, b, nil
}

// guildCountItem is guild:count_item(item).
func (r *Runtime) guildCountItem(c *script.Call, id int64) error {
	const what = "guild:count_item"
	itemID, ok := c.Arg(2).AsInt()
	if !ok || itemID <= 0 || itemID > 1<<32-1 {
		return errors.New(what + ": want an item ID from items.id")
	}
	h, err := r.storageHost(what)
	if err != nil {
		return err
	}
	n, err := h.GuildCountItem(id, uint32(itemID)) // #nosec G115 -- checked above.
	if err != nil {
		return err
	}
	c.Return(script.Int(int64(n)))
	return nil
}
