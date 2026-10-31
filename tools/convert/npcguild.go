package convert

import (
	"fmt"
	"strings"
)

// Guild commands and reads of NPC scripts, converted to player.guild and
// wa.guilds (proposal 0002, review decision 28).
//
// rAthena names a guild by its ID. getcharid(2) is the attached player's
// guild ID, so a guild argument of getcharid(2) becomes player.guild. Any
// other ID becomes guilds.get(id), which is nil when no guild has the ID.
// The converted code checks for nil where rAthena checks guild_search.
//
// guildgetexp needs the guild.manage permission. Core is built in and
// holds every permission, so the converted files need no manifest entry.
//
// guildopenstorage converts to R.guild_storage, which calls
// player:open_guild_storage and returns rAthena's GSTORAGE_ value.
// guildopenstorage_log converts to R.guild_storage_log, which uses core's
// wa.internal module, which is not API, and says that the log is not
// available elsewhere.

// The permission bits of guild_has_permission, as rAthena's
// e_guild_permission, and the fields of a guild position they read.
var guildPermissions = []struct {
	bit   int64
	field string
}{
	{0x001, "can_invite"},
	{0x010, "can_expel"},
	{0x100, "can_use_storage"},
}

// ownGuild reports whether an expression is getcharid(2).
func ownGuild(e Expr) bool {
	c, ok := e.(*Call)
	if !ok || !strings.EqualFold(c.Name, "getcharid") || len(c.Args) != 1 {
		return false
	}
	n, ok := static(c.Args[0])
	return ok && n == 2
}

// guildHandle converts a guild ID argument to a guild handle or nil. own
// is set for getcharid(2), which is player.guild.
func (b *body) guildHandle(e Expr) (code string, own bool, err error) {
	if ownGuild(e) {
		return "player.guild", true, nil
	}
	v, err := b.expr(e)
	if err != nil {
		return "", false, err
	}
	b.n.file.need("guilds")
	return "guilds.get(" + v.asInt() + ")", false, nil
}

// guildField reads a field of a guild, or a default without one.
func (b *body) guildField(e Expr, field, def string) (val, error) {
	g, own, err := b.guildHandle(e)
	if err != nil {
		return val{}, err
	}
	if own {
		return val{code: fmt.Sprintf("(player.guild and player.guild.%s or %s)", field, luaString(def)), t: tStr}, nil
	}
	return val{code: fmt.Sprintf("R.guild_field(%s, %s, %s)", g, luaString(field), luaString(def)), t: tStr}, nil
}

// guildRead converts the guild reads in expressions. ok is false for a
// name that is not one.
func (b *body) guildRead(name string, args []Expr) (v val, ok bool, err error) {
	switch name {
	case "getguildname":
		// rAthena returns "null" for a guild that does not exist.
		if len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		v, err := b.guildField(args[0], "name", "null")
		return v, true, err
	case "getguildmaster":
		// The master's name, and "null" for a guild that does not exist.
		if len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		v, err := b.guildField(args[0], "master_name", "null")
		return v, true, err
	case "getgdskilllv":
		v, err := b.getgdskilllv(args)
		return v, true, err
	case "is_guild_leader":
		// is_guild_leader({<guild id>}) is true when the attached player
		// is the guild's master, by default of the player's own guild.
		if len(args) > 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		if len(args) == 0 || ownGuild(args[0]) {
			return val{code: "(player.guild ~= nil and player.guild.master_name == player.name)", t: tBool}, true, nil
		}
		v, err := b.guildField(args[0], "master_name", "")
		if err != nil {
			return val{}, true, err
		}
		return val{code: "(" + v.code + " == player.name)", t: tBool}, true, nil
	case "guild_has_permission":
		v, err := b.guildHasPermission(args)
		return v, true, err
	case "getguildmember":
		count, err := b.getguildmember(args)
		return val{code: count, t: tInt}, true, err
	case "guildopenstorage":
		// guildopenstorage returns rAthena's GSTORAGE_ value.
		if len(args) != 0 {
			return val{}, true, unsupported(name, "arguments")
		}
		return val{code: "R.guild_storage(conversation, player)", t: tInt}, true, nil
	case "guildgetexp", "guildopenstorage_log":
		return val{}, true, unsupported(name, "its result")
	case "getcastlename":
		if len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		m, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		return val{code: "R.castle_name(" + m.asText() + ")", t: tStr}, true, nil
	case "getcastledata":
		if len(args) != 2 {
			return val{}, true, unsupported(name, "arguments")
		}
		code, err := b.castleArgs(args)
		return val{code: "R.castle_data(" + code + ")", t: tInt}, true, err
	}
	if why := guildUnsupported(name); why != "" {
		return val{}, true, unsupported(name, "%s", why)
	}
	return val{}, false, nil
}

// guildCommand converts the guild commands written as statements. ok is
// false for a name that is not one.
func (b *body) guildCommand(name string, args []Expr) (ok bool, err error) {
	switch name {
	case "guildgetexp":
		return true, b.guildgetexp(args)
	case "guildopenstorage":
		if len(args) != 0 {
			return true, unsupported(name, "arguments")
		}
		b.line("R.guild_storage(conversation, player)")
		return true, nil
	case "guildopenstorage_log":
		// guildopenstorage_log {<char id>}.
		if len(args) > 1 {
			return true, unsupported(name, "arguments")
		}
		if len(args) == 1 {
			who, err := b.expr(args[0])
			if err != nil {
				return true, err
			}
			if !isPlayerID(who) {
				return true, unsupported(name, "another character")
			}
		}
		b.line("R.guild_storage_log(conversation, player)")
		return true, nil
	case "getguildmember":
		_, err := b.getguildmember(args)
		return true, err
	case "setcastledata":
		// setcastledata "<map>",<index>,<value>.
		if len(args) != 3 {
			return true, unsupported(name, "arguments")
		}
		code, err := b.castleArgs(args)
		if err != nil {
			return true, err
		}
		b.line("R.set_castle_data(%s)", code)
		return true, nil
	case "flagemblem":
		// flagemblem <guild id> changes the running NPC (st->oid).
		if len(args) != 1 {
			return true, unsupported(name, "arguments")
		}
		if b.n.isFunction {
			return true, unsupported(name, "in a function")
		}
		id, err := b.expr(args[0])
		if err != nil {
			return true, err
		}
		b.line("R.flag_emblem(npc, %s)", id.asInt())
		return true, nil
	}
	if why := guildUnsupported(name); why != "" {
		return true, unsupported(name, "%s", why)
	}
	return false, nil
}

// guildUnsupported is the reason a guild command does not convert, or ""
// for a name that is not one of them.
func guildUnsupported(name string) string {
	switch name {
	case "guildskill":
		return "the guild API cannot raise a guild skill"
	case "guildchangegm":
		return "it names the new master by name, and guild:set_master takes a player handle"
	case "getguildmasterid":
		return "the guild API does not give character IDs"
	case "getguildalliance":
		return "rAthena's relation codes, with -2 and -1 for a guild that does not exist, have no helper yet over guild:is_allied and guild:is_enemy"
	case "getguildinfo":
		return "the guild API does not give the guild's emblem, castles and other details"
	case "warpguild":
		return "the guild API cannot warp a guild"
	case "requestguildinfo":
		return "the guild API has no call to load a guild from the character server"
	case "getmapguildusers":
		return "the guild API cannot count a guild's members on a map"
	case "maprespawnguildid":
		return "the guild API cannot warp a guild's members off a map or remove its monsters"
	case "gvgon", "gvgoff", "gvgon3", "gvgoff3",
		"agitstart", "agitend", "agitcheck", "agitstart2", "agitend2", "agitcheck2", "agitstart3", "agitend3", "agitcheck3",
		"guardian", "guardianinfo", "getguildemblem", "setwall", "delwall", "checkwall":
		return "castles, sieges and guardians are not in the API yet"
	}
	return ""
}

// castleArgs converts a castle command's map and the rest of its
// arguments, which are numbers: getcastledata's index, and
// setcastledata's index and value.
func (b *body) castleArgs(args []Expr) (string, error) {
	m, err := b.expr(args[0])
	if err != nil {
		return "", err
	}
	parts := []string{m.asText()}
	for _, a := range args[1:] {
		v, err := b.expr(a)
		if err != nil {
			return "", err
		}
		parts = append(parts, v.asInt())
	}
	return strings.Join(parts, ", "), nil
}

// getgdskilllv converts getgdskilllv(<guild id>,<skill id or "name">).
// rAthena returns -1 for a guild that does not exist.
func (b *body) getgdskilllv(args []Expr) (val, error) {
	const name = "getgdskilllv"
	if len(args) != 2 {
		return val{}, unsupported(name, "arguments")
	}
	v, err := b.expr(args[1])
	if err != nil {
		return val{}, err
	}
	skill := ""
	switch {
	case v.str != nil:
		skill = *v.str
	case v.static != nil:
		if s, ok := b.n.c.consts.SkillName(*v.static); ok {
			skill = s
		}
	}
	if skill == "" {
		return val{}, unsupported(name, "a computed skill")
	}
	g, own, err := b.guildHandle(args[0])
	if err != nil {
		return val{}, err
	}
	id := b.n.file.record("skills.id", skill)
	if own {
		return val{code: fmt.Sprintf("(player.guild and player.guild:get_skill_level(%s) or -1)", id), t: tInt}, nil
	}
	return val{code: fmt.Sprintf("R.guild_skill(%s, %s)", g, id), t: tInt}, nil
}

// guildHasPermission converts guild_has_permission(<permission>{,<char
// id>}) for the attached player. It is true when the player's position
// has every bit of the permission. A computed permission does not
// convert, and neither do 0 and bits that rAthena rejects.
func (b *body) guildHasPermission(args []Expr) (val, error) {
	const name = "guild_has_permission"
	if len(args) < 1 || len(args) > 2 {
		return val{}, unsupported(name, "arguments")
	}
	if len(args) == 2 {
		who, err := b.expr(args[1])
		if err != nil {
			return val{}, err
		}
		if !isPlayerID(who) {
			return val{}, unsupported(name, "for another character")
		}
	}
	p, err := b.expr(args[0])
	if err != nil {
		return val{}, err
	}
	if p.static == nil {
		return val{}, unsupported(name, "a computed permission")
	}
	perm := *p.static
	var fields []string
	known := int64(0)
	for _, gp := range guildPermissions {
		known |= gp.bit
		if perm&gp.bit != 0 {
			fields = append(fields, luaString(gp.field))
		}
	}
	if perm == 0 || perm&^known != 0 {
		return val{}, unsupported(name, "permission %d, which is not a combination of GUILD_PERM_INVITE, GUILD_PERM_EXPEL and GUILD_PERM_STORAGE", perm)
	}
	return val{code: "R.guild_permission(player, " + strings.Join(fields, ", ") + ")", t: tBool}, nil
}

// guildgetexp converts guildgetexp <amount> for the attached player's
// guild. rAthena stops the script for an amount of 0 or less and for a
// player without a guild. The converted code skips a player without a
// guild, and guild:give_exp raises an error for a computed amount out of
// its range.
func (b *body) guildgetexp(args []Expr) error {
	const name = "guildgetexp"
	if len(args) != 1 {
		return unsupported(name, "arguments")
	}
	v, err := b.expr(args[0])
	if err != nil {
		return err
	}
	if v.static != nil && *v.static <= 0 {
		return unsupported(name, "an amount of 0 or less, which rAthena rejects")
	}
	b.line("if player.guild then")
	b.line("  player.guild:give_exp(%s)", v.asInt())
	b.line("end")
	return nil
}

// getguildmember converts getguildmember <guild id>{,<type>}. rAthena
// sets $@guildmembername$[] to every member's name and $@guildmembercount
// to their count, and returns the count. These become locals of the
// function, which reads of the two variables after it use. It returns
// the count's local.
//   - Type 1, character IDs, and type 2, account IDs, do not convert. The
//     guild API gives names only.
//   - An array argument of the script's own does not convert.
func (b *body) getguildmember(args []Expr) (string, error) {
	const name = "getguildmember"
	if len(args) < 1 || len(args) > 3 {
		return "", unsupported(name, "arguments")
	}
	if len(args) >= 2 {
		t, err := b.expr(args[1])
		if err != nil {
			return "", err
		}
		switch {
		case t.static == nil:
			return "", unsupported(name, "a computed type")
		case *t.static == 1:
			return "", unsupported(name, "type 1, character IDs, which the guild API does not give")
		case *t.static == 2:
			return "", unsupported(name, "type 2, account IDs, which the guild API does not give")
		}
	}
	if len(args) == 3 {
		return "", unsupported(name, "into an array of the script's own")
	}
	g, _, err := b.guildHandle(args[0])
	if err != nil {
		return "", err
	}
	if b.guildNames == "" {
		b.guildNames = b.hide("guild_member_names")
		b.guildCount = b.hide("guild_member_count")
	}
	b.line("%s, %s = R.guild_members(%s)", b.guildNames, b.guildCount, g)
	return b.guildCount, nil
}

// guildMemberVar reads $@guildmembercount, $@guildmembername$[i] and the
// size of $@guildmembername$ after a getguildmember of the same function.
// ok is false for other variables.
func (b *body) guildMemberVar(name, idx string, size bool) (v val, ok bool, err error) {
	key := strings.ToLower(name)
	if key != "$@guildmembercount" && key != "$@guildmembername$" {
		return val{}, false, nil
	}
	if b.guildNames == "" {
		return val{}, true, unsupported("variables", "%s outside a function that calls getguildmember", name)
	}
	switch {
	case size && key == "$@guildmembercount":
		return val{}, true, unsupported("variables", "the size of %s", name)
	case size, key == "$@guildmembercount":
		return val{code: b.guildCount, t: tInt}, true, nil
	}
	return val{code: fmt.Sprintf("(%s[%s] or \"\")", b.guildNames, idx), t: tStr}, true, nil
}
