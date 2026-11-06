package modscript

import (
	"errors"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Recipe lists (proposal 0002, review decision 66):
//   - player:open_produce(level) opens the list of the produce_db
//     recipes of an item level the player can make, as rAthena's produce.
//   - player:open_cooking(level) opens a cooking kit's list, as
//     cooking: levels 11 to 20 list every dish of those levels.
//
// Weapon forging's levels, 1 to 10, raise an error: the pick cannot add
// star crumbs or element stones yet. Opening needs no permission: the
// pick checks the skills and materials, as rAthena's does.

// ProduceHost is what the recipe lists need from the zone.
type ProduceHost interface {
	// OpenProduce sends the list of an item level's recipes; cooking
	// makes it a cooking kit's list.
	OpenProduce(player int64, level int, cooking bool) error
}

// playerOpenProduce is player:open_produce(level) and, with true as the
// third argument, player:open_cooking(level).
func (r *Runtime) playerOpenProduce(c *script.Call) error {
	cooking := c.Bool(2)
	what := "player:open_produce"
	if cooking {
		what = "player:open_cooking"
	}
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, ok := r.host.(ProduceHost)
	if !ok {
		return errors.New(what + ": recipe lists do not exist here")
	}
	level, ok := c.Arg(1).AsInt()
	if !ok || level < 1 || level > 1000 {
		return errors.New(what + ": want an item level of produce_db, 11 to 1000")
	}
	return h.OpenProduce(id, int(level), cooking)
}
