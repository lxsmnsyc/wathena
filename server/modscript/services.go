package modscript

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/internal/script"
	modpkg "github.com/lxsmnsyc/wathena/server/mod"
)

// The player calls of NPC services: zeny, items, save points, shops and
// minimap marks, and items.name, text, and announcements.

// MaxZeny is rAthena's MAX_ZENY.
const MaxZeny = 2_147_483_647

// zenyAmount reads an amount of zeny: 0 to MAX_ZENY.
func zenyAmount(c *script.Call, i int, what string) (int64, error) {
	n, ok := c.Arg(i).AsInt()
	if !ok || n < 0 || n > MaxZeny {
		return 0, fmt.Errorf("%s: the amount must be 0 to %d", what, MaxZeny)
	}
	return n, nil
}

func (r *Runtime) playerPayZeny(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	if err := r.mutating("player:pay_zeny"); err != nil {
		return err
	}
	n, err := zenyAmount(c, 1, "player:pay_zeny")
	if err != nil {
		return err
	}
	ok, err := h.PayZeny(id, n)
	if err != nil {
		return err
	}
	c.Return(script.Bool(ok))
	return nil
}

func (r *Runtime) playerGiveZeny(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	if err := r.mutating("player:give_zeny"); err != nil {
		return err
	}
	if err := r.need("player:give_zeny", "player.give_zeny"); err != nil {
		return err
	}
	n, err := zenyAmount(c, 1, "player:give_zeny")
	if err != nil {
		return err
	}
	return h.GiveZeny(id, n)
}

// itemAmount reads an item ID and an amount from least to 30000.
func itemAmount(c *script.Call, what string, least int64) (uint32, int, error) {
	item, ok := c.Arg(1).AsInt()
	amount := int64(1)
	if !c.Arg(2).IsNil() {
		var okAmount bool
		amount, okAmount = c.Arg(2).AsInt()
		ok = ok && okAmount
	}
	if !ok || item <= 0 || item > 1<<32-1 || amount < least || amount > 30000 {
		return 0, 0, fmt.Errorf("%s: want an item ID from items.id and an amount from %d to 30000", what, least)
	}
	return uint32(item), int(amount), nil // #nosec G115 -- checked above.
}

func (r *Runtime) playerTakeItem(c *script.Call) error {
	if !c.Arg(3).IsNil() {
		return r.playerStorageTake(c)
	}
	id, err := playerID(c)
	if err != nil {
		return err
	}
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	if err := r.mutating("player:take_item"); err != nil {
		return err
	}
	// An amount of 0 is an error, so a script's bug shows instead of
	// taking nothing (proposal 0002, decision 14).
	item, amount, err := itemAmount(c, "player:take_item", 1)
	if err != nil {
		return err
	}
	ok, err := h.TakeItem(id, item, amount)
	if err != nil {
		return err
	}
	c.Return(script.Bool(ok))
	return nil
}

func (r *Runtime) playerCanCarry(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	if err := r.need("player:can_carry", "game.read"); err != nil {
		return err
	}
	item, amount, err := itemAmount(c, "player:can_carry", 1)
	if err != nil {
		return err
	}
	ok, err := h.CanCarry(id, item, amount)
	if err != nil {
		return err
	}
	c.Return(script.Bool(ok))
	return nil
}

func (r *Runtime) playerSetSavePoint(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	if err := r.mutating("player:set_save_point"); err != nil {
		return err
	}
	if err := r.need("player:set_save_point", "player.warp"); err != nil {
		return err
	}
	m, ok := c.Arg(1).AsString()
	x, okX := c.Arg(2).AsInt()
	y, okY := c.Arg(3).AsInt()
	if !ok || !okX || !okY || m == "" || len(m) > 64 || x < 0 || y < 0 || x > 1023 || y > 1023 {
		return errors.New("player:set_save_point: want a map name, such as prontera, and a cell")
	}
	saved, err := h.SetSavePoint(id, m, int(x), int(y))
	if err != nil {
		return err
	}
	c.Return(script.Bool(saved))
	return nil
}

func (r *Runtime) playerMarkMinimap(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	if err := r.mutating("player:mark_minimap"); err != nil {
		return err
	}
	mark, ok := c.Arg(1).AsInt()
	if !ok || mark < 0 || mark > 1<<16 {
		return errors.New("player:mark_minimap: the mark ID must be 0 to 65536")
	}
	if c.NArgs() <= 2 || c.Arg(2).IsNil() {
		return h.MarkMinimap(id, mark, 0, 0, 0, true)
	}
	x, okX := c.Arg(2).AsInt()
	y, okY := c.Arg(3).AsInt()
	color, okC := c.Arg(4).AsInt()
	if c.Arg(4).IsNil() {
		color, okC = 0xFFFFFF, true
	}
	if !okX || !okY || x < 0 || y < 0 || x > 1023 || y > 1023 || !okC || color < 0 || color > 0xFFFFFF {
		return errors.New("player:mark_minimap: want a mark ID, a cell, and a color as 0xRRGGBB")
	}
	return h.MarkMinimap(id, mark, int(x), int(y), uint32(color), false) // #nosec G115 -- checked above.
}

func (r *Runtime) playerOpenShop(c *script.Call) error {
	id, err := playerID(c)
	if err != nil {
		return err
	}
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	if err := r.mutating("player:open_shop"); err != nil {
		return err
	}
	name, ok := c.Arg(1).AsString()
	if !ok || name == "" {
		return errors.New("player:open_shop: want a shop's name from data/npcs.yml")
	}
	if !strings.Contains(name, ":") {
		// A name without a mod ID is the running mod's own shop.
		name = r.actor() + ":" + name
	}
	return h.OpenShop(id, name)
}

func (r *Runtime) itemsName(c *script.Call) error {
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	if err := r.need("items.name", "game.read"); err != nil {
		return err
	}
	item := c.Int(0)
	if item <= 0 || item > 1<<32-1 {
		return errors.New("items.name: want an item from items.id")
	}
	name, ok := h.ItemName(uint32(item)) // #nosec G115 -- checked above.
	if !ok {
		return fmt.Errorf("items.name: there is no item %d", item)
	}
	c.Return(script.String(name))
	return nil
}

// textItemLink is text.item_link: a link the client draws as the item's
// name, in the RO client's form.
func (r *Runtime) textItemLink(c *script.Call) error {
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	item := c.Int(0)
	if item <= 0 || item > 1<<32-1 {
		return errors.New("text.item_link: want an item from items.id")
	}
	name, ok := h.ItemName(uint32(item)) // #nosec G115 -- checked above.
	if !ok {
		return fmt.Errorf("text.item_link: there is no item %d", item)
	}
	name = strings.NewReplacer("<", "", ">", "").Replace(name)
	c.Return(script.String(fmt.Sprintf("<ITEMLINK>%s<INFO>%d</INFO></ITEMLINK>", name, item)))
	return nil
}

// text is text "key": the line of the running mod's English locale.
// Players' languages come with a later step, so it is always English.
func (r *Runtime) text(c *script.Call) error {
	key := c.String(0)
	mod := r.actor()
	sc := r.scripts[mod]
	if sc != nil {
		if s, ok := sc.Texts[key]; ok {
			c.Return(script.String(s))
			return nil
		}
	}
	return fmt.Errorf("text: %s has no text %q in %s", mod, key, modpkg.LocaleFile)
}

func (r *Runtime) announce(c *script.Call) error {
	h, err := r.npcHost()
	if err != nil {
		return err
	}
	what := "world.announce"
	m, _ := c.Arg(0).AsString()
	if !c.Arg(0).IsNil() {
		what = "maps.announce"
		if m == "" {
			return errors.New("maps.announce: want a map name, such as prontera")
		}
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	// A world announcement reaches every player, so the owner approves
	// it. A map announcement needs no permission.
	if what == "world.announce" {
		if err := r.need(what, "world.announce"); err != nil {
			return err
		}
	}
	text, ok := c.Arg(1).AsString()
	if !ok || text == "" || len(text) > MaxMessage || !utf8.ValidString(text) {
		return fmt.Errorf("%s: the text must be 1 to %d bytes", what, MaxMessage)
	}
	return h.Announce(m, r.sender(), text)
}
