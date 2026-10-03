package modscript

import (
	"errors"
	"fmt"

	"github.com/lxsmnsyc/wathena/internal/script"
)

// Companions, mounts and carts (proposal 0002, review decision 25):
// companions.id, mounts.id, player:set_companion, player:set_mount and
// player:set_cart. The fields player.companion, player.mount and
// player.cart are player fields of the zone.

// CompanionHost is what the companion calls need of the zone.
type CompanionHost interface {
	// CompanionID finds a record of kind "companion" or "mount" by its
	// full name.
	CompanionID(kind, name string) (int, bool)
	// SetCompanion gives the player a record of the kind, or removes it
	// when id is 0. It returns false when the player does not meet the
	// record's requires.
	SetCompanion(player int64, kind string, id int) (bool, error)
	// SetCart gives the player a cart look from 1, or removes it with 0.
	// It returns false without Pushcart, as pc_setcart.
	SetCart(player int64, look int) (bool, error)
}

func (r *Runtime) companionHost() (CompanionHost, error) {
	h, ok := r.host.(CompanionHost)
	if !ok {
		return nil, errors.New("companions do not run here")
	}
	return h, nil
}

// companionKinds are the kinds of companions.id and mounts.id, by the
// call's module.
var companionKinds = map[string]string{"companions": "companion", "mounts": "mount"}

// companionsID is companions.id and mounts.id. Arguments: the module
// and the name.
func (r *Runtime) companionsID(c *script.Call) error {
	module := c.String(0)
	kind, ok := companionKinds[module]
	if !ok {
		return fmt.Errorf("no module %q", module)
	}
	what := module + ".id"
	if err := r.need(what, "game.read"); err != nil {
		return err
	}
	name := c.String(1)
	h, err := r.companionHost()
	if err != nil {
		return err
	}
	id, ok := h.CompanionID(kind, name)
	if !ok {
		return fmt.Errorf("%s: there is no %s %q", what, kind, name)
	}
	c.Return(script.Int(int64(id)))
	return nil
}

// playerSetCompanion is player:set_companion and player:set_mount.
// Arguments: the player, the kind, then the record's ID or nil.
func (r *Runtime) playerSetCompanion(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	kind := c.String(1)
	what := "player:set_" + kind
	if _, ok := companionKinds[kind+"s"]; !ok {
		return fmt.Errorf("no call %s", what)
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	rec := int64(0)
	if !c.Arg(2).IsNil() {
		n, ok := c.Arg(2).AsInt()
		if !ok || n <= 0 || n > maxJobID {
			return fmt.Errorf("%s: want a %s from %ss.id, or nil to remove it", what, kind, kind)
		}
		rec = n
	}
	h, err := r.companionHost()
	if err != nil {
		return err
	}
	done, err := h.SetCompanion(id, kind, int(rec))
	if err != nil {
		return err
	}
	c.Return(script.Bool(done))
	return nil
}

// maxCartLook bounds set_cart's argument. The zone checks its own range.
const maxCartLook = 100

// playerSetCart is player:set_cart(look).
func (r *Runtime) playerSetCart(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	if err := r.mutating("player:set_cart"); err != nil {
		return err
	}
	look := int64(0)
	if !c.Arg(1).IsNil() {
		n, ok := c.Arg(1).AsInt()
		if !ok || n < 1 || n > maxCartLook {
			return errors.New("player:set_cart: want a cart look from 1, or nil to remove the cart")
		}
		look = n
	}
	h, err := r.companionHost()
	if err != nil {
		return err
	}
	done, err := h.SetCart(id, int(look))
	if err != nil {
		return err
	}
	c.Return(script.Bool(done))
	return nil
}
