package convert

import (
	"fmt"
	"strings"
)

// Storage, cart and bound item commands of NPC scripts, converted to the
// storage calls of proposal 0002, review decision 29:
//   - openstorage and openstorage2 0 become player:open_storage().
//   - storagecountitem and cartcountitem become player:count_item with
//     from. It returns nil where rAthena returns -1, so the converted code
//     reads nil as -1.
//   - guildstoragecountitem becomes guild:count_item on the attached
//     player's guild, and -1 without a guild.
//   - storagedelitem and cartdelitem become player:take_item with from.
//   - getitembound and the plain form of getitembound2 become
//     player:give_item with bound.
//
// The commands that name another account or character convert only for
// the attached player. The forms that match refine, cards and other
// fields of a copy do not convert, as the API matches the item only.

// storageFrom are the from and to values of the commands of a storage.
var storageFrom = map[string]string{
	"storagecountitem": "storage", "storagedelitem": "storage",
	"cartcountitem": "cart", "cartdelitem": "cart",
}

// boundTypes are the bound values of give_item, by rAthena's BOUND_
// value. BOUND_NONE, 0, gives a plain item.
var boundTypes = map[int64]string{1: "account", 2: "guild", 3: "party", 4: "character"}

// isPlayerAccount reports whether an account ID argument is the attached
// player's, getcharid(3).
func isPlayerAccount(v val) bool { return v.code == "player.account_id" }

// ownAccount checks an optional account ID argument: only the attached
// player's converts.
func (b *body) ownAccount(name string, args []Expr, at int) error {
	if len(args) <= at {
		return nil
	}
	v, err := b.expr(args[at])
	if err != nil {
		return err
	}
	if !isPlayerAccount(v) {
		return unsupported(name, "for another account")
	}
	return nil
}

// storageRead converts the storage reads in expressions. ok is false for
// a name that is not one.
func (b *body) storageRead(name string, args []Expr) (v val, ok bool, err error) {
	switch name {
	case "openstorage2":
		// rAthena returns 1 when it asked for the storage, and 0 when it
		// could not, such as with a storage already open.
		if err := b.openstorage2(args); err != nil {
			return val{}, true, err
		}
		return val{code: "player:open_storage()", t: tBool}, true, nil
	case "storagecountitem", "cartcountitem":
		// <item>{,<account id>}. rAthena returns -1 when it cannot read
		// the storage: without a cart, or with a storage window open.
		if len(args) < 1 || len(args) > 2 {
			return val{}, true, unsupported(name, "arguments")
		}
		if err := b.ownAccount(name, args, 1); err != nil {
			return val{}, true, err
		}
		it, err := b.item(args[0])
		if err != nil {
			return val{}, true, err
		}
		return val{code: fmt.Sprintf("(player:count_item(%s, { from = %s }) or -1)", it, luaString(storageFrom[name])), t: tInt}, true, nil
	case "guildstoragecountitem":
		// <item>{,<account id>}. rAthena returns -1 without a guild.
		if len(args) < 1 || len(args) > 2 {
			return val{}, true, unsupported(name, "arguments")
		}
		if err := b.ownAccount(name, args, 1); err != nil {
			return val{}, true, err
		}
		it, err := b.item(args[0])
		if err != nil {
			return val{}, true, err
		}
		return val{code: fmt.Sprintf("(player.guild and player.guild:count_item(%s) or -1)", it), t: tInt}, true, nil
	}
	if why := storageUnsupported(name); why != "" {
		return val{}, true, unsupported(name, "%s", why)
	}
	return val{}, false, nil
}

// storageCommand converts the storage commands written as statements. ok
// is false for a name that is not one.
func (b *body) storageCommand(name string, args []Expr) (ok bool, err error) {
	switch name {
	case "openstorage":
		if len(args) != 0 {
			return true, unsupported(name, "arguments")
		}
		b.line("player:open_storage()")
		return true, nil
	case "openstorage2":
		if err := b.openstorage2(args); err != nil {
			return true, err
		}
		b.line("player:open_storage()")
		return true, nil
	case "storagedelitem", "cartdelitem":
		// <item>,<amount>{,<account id>}.
		if len(args) < 2 || len(args) > 3 {
			return true, unsupported(name, "arguments")
		}
		if err := b.ownAccount(name, args, 2); err != nil {
			return true, err
		}
		it, err := b.item(args[0])
		if err != nil {
			return true, err
		}
		n, err := b.expr(args[1])
		if err != nil {
			return true, err
		}
		b.giveTake(name, "take_item", it, n, fmt.Sprintf("{ from = %s }", luaString(storageFrom[name])))
		return true, nil
	case "getitembound":
		// <item>,<amount>,<bound type>{,<account id>}.
		if len(args) < 3 || len(args) > 4 {
			return true, unsupported(name, "arguments")
		}
		return true, b.getitembound(name, args[0], args[1], args[2], args, 3)
	case "getitembound2":
		// <item>,<amount>,<identify>,<refine>,<attribute>,<card1>,<card2>,
		// <card3>,<card4>,<bound type>{,<account id>}. Only an identified
		// item with no refine, attribute or cards converts.
		if len(args) < 10 || len(args) > 11 {
			return true, unsupported(name, "arguments")
		}
		for i, want := range []int64{1, 0, 0, 0, 0, 0, 0} {
			if n, ok := static(args[2+i]); !ok || n != want {
				return true, unsupported(name, "an item that is not identified, or has a refine, an attribute or cards, which give_item does not set")
			}
		}
		return true, b.getitembound(name, args[0], args[1], args[9], args, 10)
	}
	if why := storageUnsupported(name); why != "" {
		return true, unsupported(name, "%s", why)
	}
	return false, nil
}

// storageUnsupported is the reason a storage command does not convert,
// or "" for a name that is not one of them.
func storageUnsupported(name string) string {
	switch name {
	case "storagecountitem2", "cartcountitem2", "guildstoragecountitem2",
		"storagedelitem2", "cartdelitem2", "guildstoragedelitem2":
		return "count_item and take_item match the item only, not its refine, attribute and cards"
	case "guildstoragedelitem":
		return "the guild API cannot take items out of the guild storage"
	case "getitembound3", "getitembound4":
		return "give_item cannot set random options, refine and cards"
	case "countbound":
		return "the player API cannot list the bound items of the inventory for @bound_items and @bound_amount"
	}
	return ""
}

// openstorage2 checks openstorage2 <storage id>{,<mode>{,<account id>}}.
// rAthena's storage IDs come from its inter_server.yml, where ID 0 is the
// main storage. wAthena names extra storages in data/storages.yml, and
// core has none, so only ID 0 converts. player:open_storage opens a
// storage to take and to store, so only the mode STOR_MODE_ALL converts.
func (b *body) openstorage2(args []Expr) error {
	const name = "openstorage2"
	if len(args) < 1 || len(args) > 3 {
		return unsupported(name, "arguments")
	}
	id, err := b.expr(args[0])
	if err != nil {
		return err
	}
	switch {
	case id.static == nil:
		return unsupported(name, "a computed storage ID")
	case *id.static != 0:
		return unsupported(name, "storage ID %d: rAthena's IDs come from inter_server.yml, and core has no extra storages in data/storages.yml", *id.static)
	}
	if len(args) >= 2 {
		mode, err := b.expr(args[1])
		if err != nil {
			return err
		}
		if mode.static == nil || *mode.static != 3 {
			return unsupported(name, "a mode other than STOR_MODE_ALL, as player:open_storage cannot open a storage to take or to store only")
		}
	}
	return b.ownAccount(name, args, 2)
}

// getitembound writes getitembound for an item, an amount and a bound
// type. account is the index of the optional account ID argument.
func (b *body) getitembound(name string, item, amount, bound Expr, args []Expr, account int) error {
	if err := b.ownAccount(name, args, account); err != nil {
		return err
	}
	t, err := b.expr(bound)
	if err != nil {
		return err
	}
	if t.static == nil {
		return unsupported(name, "a computed bound type")
	}
	opts := ""
	if *t.static != 0 {
		kind, ok := boundTypes[*t.static]
		if !ok {
			return unsupported(name, "bound type %d, which rAthena rejects", *t.static)
		}
		opts = fmt.Sprintf("{ bound = %s }", luaString(kind))
	}
	it, err := b.item(item)
	if err != nil {
		return err
	}
	n, err := b.expr(amount)
	if err != nil {
		return err
	}
	b.giveTake(name, "give_item", it, n, opts)
	return nil
}

// giveTake writes a give_item or take_item call with options, or none
// for "". rAthena's commands do nothing for an amount of 0 or less.
// wAthena's calls raise an error then, so such a call is left out, and a
// computed amount goes through R, which checks it.
func (b *body) giveTake(name, method, it string, n val, opts string) {
	args := []string{it, n.asInt()}
	if opts != "" {
		args = append(args, opts)
	}
	switch {
	case n.static != nil && *n.static <= 0:
		b.n.c.zeroAmounts[b.n.line.Name] = true
		b.line("-- %s with an amount of %d does nothing in rAthena.", name, *n.static)
	case n.static != nil:
		b.line("player:%s(%s)", method, strings.Join(args, ", "))
	default:
		b.n.c.checkedAmounts[b.n.line.Name] = true
		b.line("R.%s(player, %s)", method, strings.Join(args, ", "))
	}
}
