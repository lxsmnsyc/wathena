package zone

import (
	"errors"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/storage"
)

// A player's limits (proposal 0002, review decision 32): player:expand
// changes the limits one player holds, within each limit's base and max.
//   - inventory_slots is the inventory's size, saved with the character
//     (characters.inventory_size).
//   - cart_slots is the cart's size, saved with the cart.
//   - storage_slots is the account storage's size, saved with the
//     account's storage, which the account's characters share.
//   - A value is clamped to the limit's range when it is read, so a
//     ruleset that changed the range applies at once. Lowering deletes
//     nothing: slots past the size keep their items and take no new one.
//   - The player gets PlayerLimits at login and after each change.

var _ modscript.LimitHost = scriptHost{}

// limitRange is a limit's base and max in this world.
func (z *Zone) limitRange(name string) rules.Range {
	r, _ := z.limits().RangeOf(name)
	return r
}

// playerLimit is a player's value of a limit it holds. ok is false when
// the player's storage of the limit is not loaded.
func (z *Zone) playerLimit(p *Player, name string) (int, bool) {
	switch name {
	case "inventory_slots":
		return p.inventory.Size, true
	case "cart_slots":
		if st := z.cartOf(p); st != nil {
			st.Lock()
			defer st.Unlock()
			return z.cartSize(st), true
		}
	case "storage_slots":
		if st := z.findStorage(p, nil); st != nil {
			st.Lock()
			defer st.Unlock()
			return z.storageSize(st, nil), true
		}
	}
	return 0, false
}

// sendPlayerLimits sends the player its values of the limits it holds.
func (z *Zone) sendPlayerLimits(p *Player) {
	var list []*wathenav1.PlayerLimit
	for _, name := range rules.ExpandableLimits {
		if v, ok := z.playerLimit(p, name); ok {
			list = append(list, wathenav1.PlayerLimit_builder{Name: name, Value: uint32(max(v, 0))}.Build()) // #nosec G115 -- at most a ceiling.
		}
	}
	p.pending = append(p.pending, wathenav1.Event_builder{PlayerLimits: wathenav1.PlayerLimits_builder{Limits: list}.Build()}.Build())
}

// commitPlayer puts a player's character in the next commit, for a change
// that no transfer made.
func (z *Zone) commitPlayer(p *Player) {
	if !p.transferred {
		p.transferred = true
		z.txPlayers = append(z.txPlayers, p)
	}
}

// expandLimit changes one of a player's limits by amount, within the
// limit's range, and returns the new value. The change joins the next
// commit.
func (z *Zone) expandLimit(p *Player, name string, amount int) (int, error) {
	r := z.limitRange(name)
	var n int
	switch name {
	case "inventory_slots":
		cur := r.Clamp(p.inventory.Size)
		n = r.Clamp(cur + amount)
		if n != p.inventory.Size {
			p.inventory.Size = n
			p.inventory.Grow(n)
			z.commitPlayer(p)
		}
	case "cart_slots", "storage_slots":
		var st *storage.Storage
		if name == "cart_slots" {
			st = z.cartOf(p)
		} else {
			st = z.findStorage(p, nil)
		}
		if st == nil {
			return 0, errors.New("player:expand: the player's " + name + " is not loaded")
		}
		st.Lock()
		cur := r.Clamp(st.Size())
		n = r.Clamp(cur + amount)
		// The base is saved as no expansion, so a later ruleset with a
		// higher base raises it.
		saved := n
		if n == r.Base {
			saved = 0
		}
		if st.Size() != saved {
			st.SetSize(saved)
			z.personalChanged(st)
		}
		st.Unlock()
		if name == "cart_slots" && n != cur {
			z.sendCart(p)
		}
	default:
		return 0, errors.New("player:expand: " + name + " is not a limit a player holds")
	}
	z.sendPlayerLimits(p)
	return n, nil
}

// ExpandLimit is player:expand.
func (h scriptHost) ExpandLimit(id int64, name string, amount int) (int, error) {
	p, err := h.player(id)
	if err != nil {
		return 0, err
	}
	return h.z.expandLimit(p, name, amount)
}
