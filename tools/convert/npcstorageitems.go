package convert

import (
	"fmt"
	"strings"
)

// Item and equipment commands of review decision 60 (decision 0072):
//   - jobname(<job>) becomes jobs.name(job) (E3).
//   - delitem2 becomes player:take_item with refine, cards and
//     signed_by_character (G5). countitem2 reads its cards the same way.
//   - isequippedcnt becomes player:count_equipped (G12).
//   - getequiprandomoption reads equipped.random_options (G13).

// roaFields are the fields of wa.RandomOption by rAthena's ROA_ value.
var roaFields = map[int64]string{0: "id", 1: "value", 2: "param"}

// itemsRead converts the item and equipment reads of decision 0072. ok
// is false for another name.
func (b *body) itemsRead(name string, args []Expr) (v val, ok bool, err error) {
	switch name {
	case "jobname":
		if len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		job, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		b.n.file.need("jobs")
		return val{code: "jobs.name(" + job.asInt() + ")", t: tStr}, true, nil
	case "isequippedcnt":
		v, err := b.isequippedcnt(args)
		return v, true, err
	case "getequiprandomoption":
		v, err := b.getequiprandomoption(args)
		return v, true, err
	}
	return val{}, false, nil
}

// itemsCommand converts the item commands of decision 0072 written as
// statements. ok is false for another name.
func (b *body) itemsCommand(name string, args []Expr) (ok bool, err error) {
	if name != "delitem2" {
		return false, nil
	}
	return true, b.delitem2(args)
}

// isequippedcnt converts isequippedcnt(<item>{,<item>...}): the sum of
// player:count_equipped over the items. rAthena skips IDs of 0 or less
// and an ID given twice (script.cpp 16735).
func (b *body) isequippedcnt(args []Expr) (val, error) {
	const name = "isequippedcnt"
	if len(args) == 0 {
		return val{}, unsupported(name, "arguments")
	}
	if len(args) == 1 {
		if n, ok := static(args[0]); ok && n > 0 {
			it, err := b.item(args[0])
			if err != nil {
				return val{}, err
			}
			return val{code: "player:count_equipped(" + it + ")", t: tInt}, nil
		}
	}
	parts := make([]string, len(args))
	for i, a := range args {
		v, err := b.expr(a)
		if err != nil {
			return val{}, err
		}
		if v.str != nil || v.static != nil && *v.static > 0 {
			it, err := b.item(a)
			if err != nil {
				return val{}, err
			}
			parts[i] = it
			continue
		}
		parts[i] = v.asInt()
	}
	return val{code: "R.count_equipped(player, " + strings.Join(parts, ", ") + ")", t: tInt}, nil
}

// getequiprandomoption converts getequiprandomoption(<slot>, <index>,
// <type>): a field of the nth random option of the copy in an EQI_ slot.
// The type must be a fixed ROA_ value.
func (b *body) getequiprandomoption(args []Expr) (val, error) {
	const name = "getequiprandomoption"
	if len(args) != 3 {
		return val{}, unsupported(name, "arguments, or for another character")
	}
	vals, err := b.args(args)
	if err != nil {
		return val{}, err
	}
	if vals[2].static == nil {
		return val{}, unsupported(name, "a computed ROA_ type")
	}
	field, ok := roaFields[*vals[2].static]
	if !ok {
		// rAthena warns and returns 0 for another type, after the checks
		// of the slot and index, which R.equip_option makes too.
		field = ""
	}
	return val{code: fmt.Sprintf("R.equip_option(player, %s, %s, %s)", vals[0].asInt(), vals[1].asInt(), luaString(field)), t: tInt}, nil
}

// delitem2 converts delitem2 <item>,<amount>,<identify>,<refine>,
// <attribute>,<card1>,<card2>,<card3>,<card4>: player:take_item of the
// copies that match exactly. Only identified copies that are not broken
// convert, as count_item's and take_item's matches have.
func (b *body) delitem2(args []Expr) error {
	const name = "delitem2"
	if len(args) != 9 {
		return unsupported(name, "arguments, or for another character")
	}
	it, err := b.item(args[0])
	if err != nil {
		return err
	}
	vals, err := b.args(args[1:])
	if err != nil {
		return err
	}
	if vals[1].static == nil || *vals[1].static == 0 || vals[3].static == nil || *vals[3].static != 0 {
		return unsupported(name, "unidentified or broken copies")
	}
	match, err := matchOptions(name, vals[2], vals[4:8])
	if err != nil {
		return err
	}
	// rAthena ends the script, and closes the dialog, when the player
	// has too few (script.cpp 8771). Plain delitem goes on (decision
	// 0072).
	n := vals[0]
	switch {
	case n.static != nil && *n.static <= 0:
		b.n.c.zeroAmounts[b.n.line.Name] = true
		b.line("-- %s with an amount of %d does nothing in rAthena.", name, *n.static)
	case n.static != nil:
		b.line("if not player:take_item(%s, %s, %s) then R.stop() end", it, n.asInt(), match)
	default:
		b.n.c.checkedAmounts[b.n.line.Name] = true
		b.line("if not R.take_item(player, %s, %s, %s) then R.stop() end", it, n.asInt(), match)
	}
	return nil
}

// matchOptions writes the options of count_item and take_item that
// match countitem2's and delitem2's refine and four cards. card1
// CARD0_CREATE is a copy named for the character in cards 3 and 4, as
// getnameditem makes it. A forged or pet item's mark does not convert.
// Computed cards go through R.match, which reads them when the script
// runs.
func matchOptions(name string, refine val, cards []val) (string, error) {
	computed := false
	for _, c := range cards {
		if c.static == nil {
			computed = true
		}
	}
	if computed {
		all := make([]string, len(cards))
		for i, c := range cards {
			all[i] = c.asInt()
		}
		return fmt.Sprintf("R.match(%s, %s)", refine.asInt(), strings.Join(all, ", ")), nil
	}
	if *cards[0].static == card0Create {
		// A named copy's card 2 is 0 (getnameditem); rAthena compares
		// all four cards, so another card 2 matches no copy.
		if *cards[1].static != 0 {
			return "", unsupported(name, "a named item's mark with a card 2 other than 0")
		}
		id := *cards[2].static&0xFFFF | (*cards[3].static&0xFFFF)<<16
		if id <= 0 {
			return "", unsupported(name, "a copy named for no character")
		}
		return fmt.Sprintf("{ refine = %s, signed_by_character = %d }", refine.asInt(), id), nil
	}
	list, err := cardList(name, cards)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("{ refine = %s, cards = %s }", refine.asInt(), list), nil
}
