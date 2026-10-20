package convert

import (
	"fmt"
	"strings"
)

// Party commands and reads of NPC scripts, converted to player.party and
// wa.parties (proposal 0002, review decision 27).
//
// rAthena names a party by its ID. getcharid(1) is the attached player's
// party ID, so a party argument of getcharid(1) becomes player.party. Any
// other ID becomes parties.get(id), which is nil when no party has the ID.
// The converted code checks for nil where rAthena checks party_search.
//
// The write commands convert as statements only. Their result codes do
// not convert. The API needs the party.manage permission for them. Core
// is built in and holds every permission, so the converted files need no
// manifest entry.

// ownParty reports whether an expression is getcharid(1).
func ownParty(e Expr) bool {
	c, ok := e.(*Call)
	if !ok || !strings.EqualFold(c.Name, "getcharid") || len(c.Args) != 1 {
		return false
	}
	n, ok := static(c.Args[0])
	return ok && n == 1
}

// partyHandle converts a party ID argument to a party handle or nil. own
// is set for getcharid(1), which is player.party.
func (b *body) partyHandle(e Expr) (code string, own bool, err error) {
	if ownParty(e) {
		return "player.party", true, nil
	}
	v, err := b.expr(e)
	if err != nil {
		return "", false, err
	}
	b.n.file.need("parties")
	return "parties.get(" + v.asInt() + ")", false, nil
}

// partyLocal writes a party handle into a local for a write command, and
// returns the local. The attached player's own party needs none.
func (b *body) partyLocal(e Expr) (string, bool, error) {
	code, own, err := b.partyHandle(e)
	if err != nil || own {
		return code, own, err
	}
	local := b.hide("party")
	b.line("%s = %s", local, code)
	return local, false, nil
}

// isPlayerID reports whether a character ID argument is the attached
// player's, getcharid(0).
func isPlayerID(v val) bool { return v.code == "player.character_id" }

// partyFlag writes an rAthena option flag as one of two API values: on
// for a flag other than 0.
func partyFlag(v val, on, off string) string {
	if v.static != nil {
		if *v.static != 0 {
			return luaString(on)
		}
		return luaString(off)
	}
	return fmt.Sprintf("(%s and %s or %s)", v.asBool(), luaString(on), luaString(off))
}

// partyCommand converts the party write commands.
func (b *body) partyCommand(name string, args []Expr) error {
	switch name {
	case "party_create":
		return b.partyCreate(args)
	case "party_addmember":
		// party_addmember <party id>,<char id>. rAthena skips a player who
		// has a party, and a party that does not exist.
		if len(args) != 2 {
			return unsupported(name, "arguments")
		}
		who, err := b.expr(args[1])
		if err != nil {
			return err
		}
		if !isPlayerID(who) {
			return unsupported(name, "another character")
		}
		if ownParty(args[0]) {
			b.line("-- party_addmember to the player's own party does nothing in rAthena.")
			return nil
		}
		p, _, err := b.partyLocal(args[0])
		if err != nil {
			return err
		}
		b.line("if %s and player.party == nil then", p)
		b.line("  %s:add(player)", p)
		b.line("end")
	case "party_delmember":
		// party_delmember {<char id>,<party id>}. With the attached player,
		// rAthena removes the player from the player's own party and
		// ignores the party ID.
		if len(args) > 2 {
			return unsupported(name, "arguments")
		}
		if len(args) > 0 {
			who, err := b.expr(args[0])
			if err != nil {
				return err
			}
			if !isPlayerID(who) {
				return unsupported(name, "another character")
			}
		}
		b.line("if player.party then")
		b.line("  player.party:remove(player)")
		b.line("end")
	case "party_changeleader":
		// party_changeleader <party id>,<char id>. rAthena skips a player
		// who is not in the party. A player in the party has it as
		// player.party, so the party ID is compared with it.
		if len(args) != 2 {
			return unsupported(name, "arguments")
		}
		who, err := b.expr(args[1])
		if err != nil {
			return err
		}
		if !isPlayerID(who) {
			return unsupported(name, "another character")
		}
		if ownParty(args[0]) {
			b.line("if player.party then")
		} else {
			id, err := b.expr(args[0])
			if err != nil {
				return err
			}
			b.line("if player.party and player.party.id == %s then", id.asInt())
		}
		b.line("  player.party:set_leader(player)")
		b.line("end")
	case "party_changeoption":
		// party_changeoption <party id>,<option>,<flag>. Option 0 is the exp
		// share, 1 the item pick-up and 2 the item share. A flag other
		// than 0 is even or shared.
		if len(args) != 3 {
			return unsupported(name, "arguments")
		}
		opt, err := b.expr(args[1])
		if err != nil {
			return err
		}
		if opt.static == nil {
			return unsupported(name, "a computed option")
		}
		flag, err := b.expr(args[2])
		if err != nil {
			return err
		}
		var field string
		switch *opt.static {
		case 0:
			field = "exp_share = " + partyFlag(flag, "even", "each")
		case 1:
			field = "item_pickup = " + partyFlag(flag, "shared", "each")
		case 2:
			field = "item_share = " + partyFlag(flag, "even", "each")
		default:
			return unsupported(name, "an option other than 0, 1 and 2")
		}
		p, _, err := b.partyLocal(args[0])
		if err != nil {
			return err
		}
		b.line("if %s then", p)
		b.line("  %s:set_options { %s }", p, field)
		b.line("end")
	case "party_destroy":
		if len(args) != 1 {
			return unsupported(name, "arguments")
		}
		p, _, err := b.partyLocal(args[0])
		if err != nil {
			return err
		}
		b.line("if %s then", p)
		b.line("  %s:disband()", p)
		b.line("end")
	}
	return nil
}

// partyCreate converts party_create "<name>"{,<char id>{,<item pick-up>
// {,<item share>}}}.
//   - The leader is the attached player. rAthena's default is the same.
//   - rAthena cuts the name to 23 bytes and trims spaces. A fixed name is
//     cut and trimmed here. An empty name does nothing in rAthena.
//   - parties.create returns nil when the name is taken or the player has
//     a party, where rAthena returns -3 and -2.
//   - The item pick-up flag sets item_pickup to "shared", and the item
//     share flag item_share to "even". The exp share starts as "each".
func (b *body) partyCreate(args []Expr) error {
	const name = "party_create"
	if len(args) < 1 || len(args) > 4 {
		return unsupported(name, "arguments")
	}
	vals, err := b.args(args)
	if err != nil {
		return err
	}
	if len(vals) > 1 && !isPlayerID(vals[1]) {
		return unsupported(name, "for another character")
	}
	partyName := vals[0].asText()
	if s := vals[0].str; s != nil {
		cut := *s
		if len(cut) > 23 {
			cut = cut[:23]
		}
		cut = strings.TrimSpace(cut)
		if cut == "" {
			return unsupported(name, "an empty name")
		}
		partyName = luaString(cut)
	}
	var opts []string
	if len(vals) > 2 && (vals[2].static == nil || *vals[2].static != 0) {
		opts = append(opts, "item_pickup = "+partyFlag(vals[2], "shared", "each"))
	}
	if len(vals) > 3 && (vals[3].static == nil || *vals[3].static != 0) {
		opts = append(opts, "item_share = "+partyFlag(vals[3], "even", "each"))
	}
	f := b.n.file
	f.need("parties")
	if len(opts) == 0 {
		b.line("parties.create(player, %s)", partyName)
		return nil
	}
	p := b.hide("party")
	b.line("%s = parties.create(player, %s)", p, partyName)
	b.line("if %s then", p)
	b.line("  %s:set_options { %s }", p, strings.Join(opts, ", "))
	b.line("end")
	return nil
}

// partyRead converts the party reads in expressions. ok is false for a
// name that is not one.
func (b *body) partyRead(name string, args []Expr) (v val, ok bool, err error) {
	switch name {
	case "getpartyname":
		// rAthena returns "null" for a party that does not exist.
		if len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		v, err := b.partyField(args[0], "name", "null")
		return v, true, err
	case "getpartyleader":
		// getpartyleader <party id>{,<type>}. Type 0 is the leader's name,
		// and "null" for a party that does not exist.
		if len(args) < 1 || len(args) > 2 {
			return val{}, true, unsupported(name, "arguments")
		}
		if len(args) == 2 {
			t, err := b.expr(args[1])
			if err != nil {
				return val{}, true, err
			}
			if t.static == nil {
				return val{}, true, unsupported(name, "a computed type")
			}
			if *t.static != 0 {
				what, known := map[int64]string{1: "account ID", 2: "character ID", 3: "job", 4: "map", 5: "level"}[*t.static]
				if !known {
					return val{}, true, unsupported(name, "type %d", *t.static)
				}
				return val{}, true, unsupported(name, "the leader's %s (type %d), which the party API has only while the leader is online", what, *t.static)
			}
		}
		v, err := b.partyField(args[0], "leader_name", "null")
		return v, true, err
	case "is_party_leader":
		// is_party_leader({<party id>}) is true when the attached player
		// leads the party, by default the player's own.
		if len(args) > 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		if len(args) == 0 || ownParty(args[0]) {
			return val{code: "(player.party ~= nil and player.party.leader_name == player.name)", t: tBool}, true, nil
		}
		v, err := b.partyField(args[0], "leader_name", "")
		if err != nil {
			return val{}, true, err
		}
		return val{code: "(" + v.code + " == player.name)", t: tBool}, true, nil
	case "getpartymember":
		count, err := b.getpartymember(args)
		return val{code: count, t: tInt}, true, err
	case "party_create", "party_addmember", "party_delmember", "party_changeleader", "party_changeoption", "party_destroy":
		return val{}, true, unsupported(name, "its result code")
	}
	return val{}, false, nil
}

// partyField reads a field of a party, or a default without one.
func (b *body) partyField(e Expr, field, def string) (val, error) {
	p, own, err := b.partyHandle(e)
	if err != nil {
		return val{}, err
	}
	if own {
		return val{code: fmt.Sprintf("(player.party and player.party.%s or %s)", field, luaString(def)), t: tStr}, nil
	}
	return val{code: fmt.Sprintf("R.party_field(%s, %s, %s)", p, luaString(field), luaString(def)), t: tStr}, nil
}

// getpartymember converts getpartymember <party id>{,<type>}. rAthena
// sets $@partymembername$[] to every member's name, $@partymembercid[]
// to their character IDs for type 1, or $@partymemberaid[] to their
// account IDs for type 2, and $@partymembercount to their count, and
// returns the count. These become locals of the function, which reads of
// the variables after it use. It returns the count's local. Types 1 and
// 2 read party:get_member_list (review decision 58).
//   - An array argument of the script's own does not convert.
func (b *body) getpartymember(args []Expr) (string, error) {
	const name = "getpartymember"
	if len(args) < 1 || len(args) > 3 {
		return "", unsupported(name, "arguments")
	}
	field := "name"
	if len(args) >= 2 {
		t, err := b.expr(args[1])
		if err != nil {
			return "", err
		}
		switch {
		case t.static == nil:
			return "", unsupported(name, "a computed type")
		case *t.static == 1:
			field = "character_id"
		case *t.static == 2:
			field = "account_id"
		}
	}
	if len(args) == 3 {
		return "", unsupported(name, "into an array of the script's own")
	}
	p, _, err := b.partyHandle(args[0])
	if err != nil {
		return "", err
	}
	if b.partyCount == "" {
		b.partyCount = b.hide("party_member_count")
	}
	list := &b.partyNames
	switch field {
	case "character_id":
		list = &b.partyCIDs
	case "account_id":
		list = &b.partyAIDs
	}
	if *list == "" {
		*list = b.hide("party_member_" + map[string]string{"name": "names", "character_id": "cids", "account_id": "aids"}[field])
	}
	if field == "name" {
		b.line("%s, %s = R.party_members(%s)", *list, b.partyCount, p)
	} else {
		b.line("%s, %s = R.party_members(%s, %s)", *list, b.partyCount, p, luaString(field))
	}
	return b.partyCount, nil
}

// partyMemberVar reads $@partymembercount, $@partymembername$[i] and the
// size of $@partymembername$ after a getpartymember of the same function.
// ok is false for other variables.
func (b *body) partyMemberVar(name, idx string, size bool) (v val, ok bool, err error) {
	key := strings.ToLower(name)
	list := map[string]string{"$@partymembername$": b.partyNames, "$@partymembercid": b.partyCIDs, "$@partymemberaid": b.partyAIDs}
	l, isList := list[key]
	if key != "$@partymembercount" && !isList {
		return val{}, false, nil
	}
	if b.partyCount == "" || isList && l == "" {
		return val{}, true, unsupported("variables", "%s outside a function that calls getpartymember", name)
	}
	switch {
	case size && key == "$@partymembercount":
		return val{}, true, unsupported("variables", "the size of %s", name)
	case size, key == "$@partymembercount":
		return val{code: b.partyCount, t: tInt}, true, nil
	case key == "$@partymembername$":
		return val{code: fmt.Sprintf("(%s[%s] or \"\")", l, idx), t: tStr}, true, nil
	}
	return val{code: fmt.Sprintf("(%s[%s] or 0)", l, idx), t: tInt}, true, nil
}
