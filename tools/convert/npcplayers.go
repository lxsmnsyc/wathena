package convert

// Players across zones (review decision 59): rAthena's commands that
// act on another character, and attachrid, onto player handles of any
// map. A handle of a player of another map sends its actions to the
// player's zone, which runs them after the current step.

// otherTarget is the last argument of a command that names another
// character: its place, and how rAthena finds the player, as
// script_accid2sd and script_mapid2sd (an account ID), script_charid2sd
// (a character ID), script_nick2sd (a name) or map_id2bl (a game ID).
type otherTarget struct {
	at   int
	find string
}

// otherCharacter are the commands whose last argument names another
// character, by rAthena's signatures (doc/script_commands.txt) and
// lookups (script.cpp).
var otherCharacter = map[string]otherTarget{
	"getitem": {2, "account"}, "getitembound": {3, "account"}, "getitem2": {9, "account"}, "getitembound2": {10, "account"},
	"delitem": {2, "account"}, "delitem2": {9, "account"}, "rentitem": {2, "account"}, "rentitem2": {9, "account"},
	"setpcblock":  {2, "account"},
	"percentheal": {2, "char"}, "heal": {2, "char"}, "itemheal": {2, "char"}, "warp": {3, "char"}, "warpchar": {3, "char"},
	"savepoint": {3, "char"}, "dispbottom": {2, "char"}, "getexp": {2, "char"}, "getexp2": {2, "char"},
	"setlook": {2, "char"}, "changelook": {2, "char"}, "jobchange": {2, "char"}, "setquest": {1, "char"},
	"completequest": {1, "char"}, "erasequest": {1, "char"}, "changequest": {2, "char"}, "resetlvl": {1, "char"},
	"resetstatus": {0, "char"}, "resetskill": {0, "char"}, "statusup": {1, "char"}, "statusup2": {2, "char"},
	"nude": {0, "char"}, "repair": {1, "char"}, "repairall": {0, "char"}, "successrefitem": {2, "char"},
	"downrefitem": {2, "char"}, "failedrefitem": {1, "char"}, "delequip": {1, "char"}, "getgroupitem": {2, "char"},
	"clearitem": {0, "char"},
	"sc_start":  {5, "gid"}, "sc_start2": {6, "gid"}, "sc_start4": {8, "gid"}, "sc_end": {1, "gid"},
	"specialeffect2": {2, "name"},
}

// findPlayer is the R function that finds the player of a lookup.
var findPlayer = map[string]string{"account": "R.by_account", "char": "R.by_char", "gid": "R.by_gid", "name": "R.by_name"}

// ownValue reports whether an argument is the attached player's own ID
// or name for a lookup: the command acts on the attached player.
func ownValue(v val, find string) bool {
	switch find {
	case "account", "gid":
		return v.code == "player.account_id"
	case "char":
		return v.code == "player.character_id"
	case "name":
		return v.code == "player.name"
	}
	return false
}

// onOtherCharacter converts a command that names another character in
// its last argument: the command acts on that player, on any map, in a
// block where player is its handle, and does nothing for a player who is
// offline, as rAthena's commands only log an error. ok is false for a
// command that does not name one.
func (b *body) onOtherCharacter(name string, args []Expr) (ok bool, err error) {
	if name == "message" && len(args) == 2 {
		// message "<name>","<text>" (script.cpp 16170): the line of
		// dispbottom to a player by name.
		who, err := b.expr(args[0])
		if err != nil {
			return true, err
		}
		if ownValue(who, "name") {
			return false, nil
		}
		return true, b.onPlayerOf("R.by_name("+who.asText()+")", "dispbottom", args[1:])
	}
	t, found := otherCharacter[name]
	if !found {
		return false, nil
	}
	at := t.at
	if name == "savepoint" && len(args) == 6 {
		// savepoint's range comes before the character.
		at = 5
	}
	if name == "warpchar" {
		// warpchar is warp with a character ID (BUILDIN_DEF2).
		name = "warp"
	}
	if len(args) != at+1 {
		return false, nil
	}
	who, err := b.expr(args[at])
	if err != nil {
		return true, err
	}
	rest := args[:at]
	if ownValue(who, t.find) {
		return true, b.commandErr(name, rest)
	}
	code := who.asInt()
	if t.find == "name" {
		code = who.asText()
	}
	return true, b.onPlayerOf(findPlayer[t.find]+"("+code+")", name, rest)
}

// onPlayerOf writes a command in a block where player is the handle that
// find gives, which does nothing when it is nil.
func (b *body) onPlayerOf(find, name string, args []Expr) error {
	mark := len(b.lines)
	b.line("do")
	b.indent++
	b.line("local player = %s", find)
	b.line("if player ~= nil then")
	b.indent++
	err := b.commandErr(name, args)
	b.indent -= 2
	if err != nil {
		b.lines = b.lines[:mark]
		return err
	}
	b.line("  end")
	b.line("end")
	return nil
}

// attachrid converts attachrid(<account ID>) as a statement (script.cpp
// 13507): the online player of the account, on any map, is the script's
// player from then on, and the player before stays when there is none.
// Dialogs still go to the conversation's player.
func (b *body) attachrid(args []Expr) error {
	const what = "attachrid"
	if b.n.isFunction {
		return unsupported(what, "in a function, whose caller keeps the player it attached")
	}
	if len(args) == 2 {
		f, err := b.fixed(what, "force flag", args[1])
		if err != nil {
			return err
		}
		if f == 0 {
			return unsupported(what, "without force, which needs to know whether the player talks to an NPC")
		}
	} else if len(args) != 1 {
		return unsupported(what, "arguments")
	}
	v, err := b.expr(args[0])
	if err != nil {
		return err
	}
	if v.t == tStr {
		return unsupported(what, "a name")
	}
	b.n.file.need("maps")
	b.line("player = R.attachrid(player, %s)", v.asInt())
	return nil
}

// detachrid converts detachrid (script.cpp 13535): the script has no
// player from then on.
func (b *body) detachrid(args []Expr) error {
	if len(args) != 0 {
		return unsupported("detachrid", "arguments")
	}
	if b.n.isFunction {
		return unsupported("detachrid", "in a function, whose caller keeps its player")
	}
	b.line("player = nil")
	return nil
}
