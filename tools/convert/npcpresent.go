package convert

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The presentation and quest icon commands of review decision 58,
// areas B5 and B6 of proposal 0002b, onto the NPC and player methods of
// server/modscript/present.go:
//   - A command on another NPC finds it by name when the script runs,
//     through R.npc_view, and does nothing when it does not exist, as
//     rAthena only logs an error.
//   - A game ID argument converts when it is the attached player
//     (getcharid(3)), the NPC itself (getnpcid(0)), an NPC by a fixed
//     name (getnpcid(0, "<NPC>")), or a variable that only ever holds one
//     of the two getnpcid forms (npcIDVars).
//   - A send target converts when it is fixed.

// rAthena's constants that these commands read.
const (
	bcAll, bcMap, bcSelf      = 0, 1, 3
	sendAllSameMap, sendSelf  = 1, 24
	jtFakeNPC                 = -1
	uswDefaultNPCStop         = 1 | 4 | 8 // USW_FIXPOS | USW_MOVE_FULL_CELL | USW_FORCE_STOP
	uswFixPos, uswFullCell    = 1, 4
	uswMoveOnce               = 2
	umobHP, umobSpeed         = 2, 8
	umobMode, umobDmgImmune   = 9, 31
	unpcSex, unpcClass        = 32, 33
	unpcHairStyle, unpcHairCl = 34, 35
	unpcHeadBottom, unpcHeadM = 36, 37
	unpcHeadTop, unpcCloth    = 38, 39
	sexMale                   = 1
	qtypeNone                 = 9999
)

// unpcLooks are the look fields of setunitdata's UNPC_ types, as
// npc:set_look names them.
var unpcLooks = map[int64]string{
	unpcClass: "sprite", unpcHairStyle: "hair_style", unpcHairCl: "hair_color", unpcHeadBottom: "head_bottom",
	unpcHeadM: "head_middle", unpcHeadTop: "head_top", unpcCloth: "cloth_color",
}

// questinfoIcons are wa.QuestIcon's names by rAthena's QTYPE_ values,
// as questinfo takes them for PACKETVER 20170315 and later.
var questinfoIcons = map[int64]string{
	0: "quest", 1: "quest_question", 2: "job", 3: "job_question", 4: "event", 5: "event_question",
	6: "click_me", 7: "daily_quest", 8: "event3", 9: "job_quest", 10: "jumping_poring",
}

// questMarks are wa.QuestMark's names by rAthena's QMARK_ values. 0 is
// QMARK_NONE.
var questMarks = map[int64]string{1: "yellow", 2: "green", 3: "purple"}

// npcIDVars finds the variables of a script that only ever hold
// getnpcid(0) or getnpcid(0, "<NPC>") with one name: their lower-case
// names, with the NPC's code.
func (b *body) npcIDVars() map[string]string {
	n := b.n
	if n.npcIDs != nil {
		return n.npcIDs
	}
	out, bad := map[string]string{}, map[string]bool{}
	walkStmts(n.all, func(s Stmt) {
		var target *Var
		var value Expr
		switch s := s.(type) {
		case *Assign:
			target, value = s.Target, s.Value
			if s.Op != "=" {
				value = nil
			}
		case *IncDec:
			target = s.Target
		default:
			return
		}
		key := strings.ToLower(target.Name)
		code, ok := b.getnpcidOf(value)
		if !ok || target.Index != nil || out[key] != "" && out[key] != code {
			bad[key] = true
			return
		}
		out[key] = code
	})
	for k := range bad {
		delete(out, k)
	}
	n.npcIDs = out
	return out
}

// getnpcidOf reads getnpcid(0) or getnpcid(0, "<NPC>"): the NPC's code.
func (b *body) getnpcidOf(e Expr) (string, bool) {
	c, ok := e.(*Call)
	if !ok || !strings.EqualFold(c.Name, "getnpcid") || len(c.Args) < 1 || len(c.Args) > 2 {
		return "", false
	}
	if k, ok := c.Args[0].(*Num); !ok || k.Value != 0 {
		return "", false
	}
	if len(c.Args) == 1 {
		return "npc", true
	}
	s, ok := c.Args[1].(*Str)
	if !ok {
		return "", false
	}
	if s.Value == b.n.line.Name && !b.n.isFunction && !b.n.c.duplicated[s.Value] {
		return "npc", true
	}
	return luaString(b.n.c.recordFor(s.Value)), true
}

// gidRef reads a game ID argument: kind is "player" for the attached
// player, or "npc" with the NPC's code.
func (b *body) gidRef(what string, e Expr) (kind, code string, err error) {
	switch e := e.(type) {
	case *Call:
		if strings.EqualFold(e.Name, "getcharid") && len(e.Args) == 1 {
			if k, ok := e.Args[0].(*Num); ok && k.Value == 3 {
				return "player", "player", nil
			}
		}
		if (strings.EqualFold(e.Name, "playerattached") || strings.EqualFold(e.Name, "getattachedrid")) && len(e.Args) == 0 {
			// The attached player's account ID, as st->rid.
			return "player", "player", nil
		}
		if code, ok := b.getnpcidOf(e); ok {
			return "npc", code, nil
		}
		if strings.EqualFold(e.Name, "getnpcid") && len(e.Args) == 2 {
			// getnpcid(0, <name made when the script runs>).
			if k, ok := e.Args[0].(*Num); ok && k.Value == 0 {
				code, _, err := b.npcRef(what, e.Args[1])
				if err != nil {
					return "", "", err
				}
				return "npc", code, nil
			}
		}
	case *Var:
		if code, ok := b.npcIDVars()[strings.ToLower(e.Name)]; ok && e.Index == nil {
			return "npc", code, nil
		}
	case *Num:
		if e.Value == 0 {
			// script_rid2bl: 0 is the attached player.
			return "player", "player", nil
		}
	}
	return "", "", unsupported(what, "a game ID other than the attached player's or an NPC's of getnpcid")
}

// npcTarget reads an NPC name argument: "" or the NPC's own name is the
// NPC itself.
func (b *body) npcTarget(what string, e Expr) (string, error) {
	if s, ok := e.(*Str); ok && s.Value == "" {
		return "npc", nil
	}
	if s, ok := e.(*Str); ok && s.Value == b.n.line.Name && !b.n.isFunction && !b.n.c.duplicated[s.Value] {
		return "npc", nil
	}
	code, _, err := b.npcRef(what, e)
	return code, err
}

// selfOrNPC reads the NPC name argument of npctalk and classchange, for
// which an empty name, even a computed one, is the NPC itself.
func (b *body) selfOrNPC(what string, e Expr) (string, error) {
	target, err := b.npcTarget(what, e)
	if err != nil || target == "npc" || strings.HasPrefix(target, `"`) {
		return target, err
	}
	return "R.or_self(" + target + ", npc)", nil
}

// onNPC writes a method call on an NPC: on the handle, or through
// R.npc_view for an NPC by name.
func (b *body) onNPC(target, method string, args ...string) string {
	if target == "npc" {
		return fmt.Sprintf("npc:%s(%s)", method, strings.Join(args, ", "))
	}
	return fmt.Sprintf("R.npc_view(%s)", strings.Join(append([]string{target, luaString(method)}, args...), ", "))
}

// options writes an options table, or "" for none.
func options(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	return "{ " + strings.Join(parts, ", ") + " }"
}

// withOptions appends an options table to arguments.
func withOptions(args []string, parts []string) []string {
	if o := options(parts); o != "" {
		return append(args, o)
	}
	return args
}

// fixed reads a fixed integer argument.
func (b *body) fixed(what, arg string, e Expr) (int64, error) {
	v, err := b.expr(e)
	if err != nil {
		return 0, err
	}
	if v.static == nil {
		return 0, unsupported(what, "a computed %s", arg)
	}
	return *v.static, nil
}

// assetName turns a file name of rAthena's sound and music commands
// into an asset name: the path under data\wav or BGM, in lower case,
// without its extension.
func assetName(file, ext string) (string, bool) {
	s := strings.ToLower(strings.ReplaceAll(file, `\`, "/"))
	s = strings.TrimSuffix(s, ext)
	if s == "" || strings.ContainsAny(s, " .") {
		return "", false
	}
	return s, true
}

// presentCommand converts the commands of areas B5 and B6. ok is false
// for another command.
func (b *body) presentCommand(name string, args []Expr) (ok bool, err error) {
	switch name {
	case "npctalk":
		return true, b.npctalk(args)
	case "unittalk":
		return true, b.unittalk(args)
	case "emotion":
		return true, b.emotion(args)
	case "specialeffect2":
		return true, b.specialeffect2(args)
	case "removespecialeffect", "removespecialeffect2":
		return true, b.removespecialeffect(name, args)
	case "setunittitle":
		return true, b.setunittitle(args)
	case "setnpcdisplay":
		v, err := b.setnpcdisplay(args)
		if err == nil {
			// R.npc_display's result is not read.
			b.line("%s", v.code)
		}
		return true, err
	case "classchange":
		return true, b.classchange(args)
	case "setunitdata":
		return true, b.setunitdata(args)
	case "unitwalk":
		call, err := b.unitwalkCall(args)
		if err == nil {
			b.line("%s", call)
		}
		return true, err
	case "unitwarp":
		return true, b.unitwarp(args)
	case "unitkill":
		return true, b.unitkill(args)
	case "unitskillusepos":
		return true, b.unitskillusepos(args)
	case "unitstopwalk":
		return true, b.unitstopwalk(args)
	case "npcspeed", "npcwalkto", "npcstop", "movenpc":
		return true, b.npcMove(name, args)
	case "progressbar_npc":
		return true, b.progressbarNPC(args)
	case "npcskill":
		return true, b.npcskill(args)
	case "skilleffect":
		return true, b.skilleffect(args)
	case "soundeffect", "soundeffectall":
		return true, b.soundeffect(name, args)
	case "playbgm", "playbgmall":
		return true, b.playbgm(name, args)
	case "showdigit":
		return true, b.showdigit(args)
	case "viewpointmap":
		return true, b.viewpointmap(args)
	case "questinfo":
		return true, b.questinfo(args)
	case "questinfo_refresh":
		if err := b.questPlayer(name, args, 0); err != nil {
			return true, err
		}
		b.line("player:refresh_quest_icons()")
		return true, nil
	case "open_quest_ui":
		return true, b.openQuestUI(args)
	}
	return false, nil
}

// presentRead converts the commands of area B5 that give a value. ok is
// false for another function.
func (b *body) presentRead(name string, args []Expr) (v val, ok bool, err error) {
	switch name {
	case "unitwalk":
		v, err := b.unitwalk(args)
		return v, true, err
	case "setnpcdisplay":
		v, err := b.setnpcdisplay(args)
		return v, true, err
	case "getnpcid":
		return val{}, true, unsupported(name, "the game ID of an NPC, which only the unit commands take")
	}
	return val{}, false, nil
}

// stripSpeaker drops strcharinfo(0) + " : " from the start of a line
// that a player says aloud: the protocol carries the speaker's name, and
// the client shows it before the line.
func stripSpeaker(e Expr) Expr {
	var parts []Expr
	var flatten func(Expr) bool
	flatten = func(e Expr) bool {
		if bin, ok := e.(*Binary); ok && bin.Op == "+" {
			return flatten(bin.X) && flatten(bin.Y)
		}
		parts = append(parts, e)
		return true
	}
	flatten(e)
	for len(parts) > 0 {
		if s, ok := parts[0].(*Str); ok && s.Value == "" {
			parts = parts[1:]
			continue
		}
		break
	}
	if len(parts) < 2 {
		return e
	}
	c, ok := parts[0].(*Call)
	s, isStr := parts[1].(*Str)
	if !ok || !isStr || !strings.EqualFold(c.Name, "strcharinfo") || len(c.Args) != 1 {
		return e
	}
	if k, ok := c.Args[0].(*Num); !ok || k.Value != 0 {
		return e
	}
	rest, cut := strings.CutPrefix(strings.TrimLeft(s.Value, " "), ":")
	if !cut {
		return e
	}
	parts[1] = &Str{Value: strings.TrimLeft(rest, " ")}
	out := parts[1]
	for _, p := range parts[2:] {
		out = &Binary{Op: "+", X: out, Y: p}
	}
	return out
}

// npctalk converts npctalk "<text>"{,"<NPC>"{,<flag>{,<color>}}}
// (script.cpp 16193): bc_all, bc_map, bc_self, or the area.
func (b *body) npctalk(args []Expr) error {
	const what = "npctalk"
	if len(args) < 1 || len(args) > 4 {
		return unsupported(what, "arguments")
	}
	text, err := b.expr(args[0])
	if err != nil {
		return err
	}
	target := "npc"
	if len(args) >= 2 {
		if target, err = b.selfOrNPC(what, args[1]); err != nil {
			return err
		}
	}
	var opts []string
	self := false
	if len(args) >= 3 {
		flag, err := b.fixed(what, "flag", args[2])
		if err != nil {
			return err
		}
		switch flag {
		case bcAll:
			opts = append(opts, `to = "world"`)
		case bcMap:
			opts = append(opts, `to = "map"`)
		case bcSelf:
			opts, self = append(opts, "to = player"), true
		}
	}
	if len(args) == 4 {
		c, err := b.expr(args[3])
		if err != nil {
			return err
		}
		if c.static != nil && (*c.static < 0 || *c.static > 0xFFFFFF) {
			return unsupported(what, "a color past 0xFFFFFF")
		}
		opts = append(opts, "color = "+c.asInt())
	}
	call := b.onNPC(target, "say_aloud", withOptions([]string{text.asText()}, opts)...)
	switch {
	case text.str != nil && *text.str == "":
		b.line("-- npctalk of an empty line: the API sends no empty line.")
		return nil
	case text.str == nil:
		// A computed line may be empty, which rAthena sends and the API
		// refuses: R.say_aloud leaves it out.
		call = fmt.Sprintf("R.say_aloud(%s)", strings.Join(withOptions([]string{target, text.asText()}, opts), ", "))
	}
	if self {
		// buildin_npctalk fails without an attached player.
		b.line("if player ~= nil then %s end", call)
		return nil
	}
	b.line("%s", call)
	return nil
}

// unittalk converts unittalk <game ID>,"<text>"{,<flag>} (script.cpp
// 20216) for the attached player: a line of local chat, or bc_self.
func (b *body) unittalk(args []Expr) error {
	const what = "unittalk"
	if len(args) < 2 || len(args) > 3 {
		return unsupported(what, "arguments")
	}
	if mob, ok, err := b.mobGID(what, args[0]); ok || err != nil {
		// Another game ID: a player by account ID on any map, or a
		// monster (review decision 59). The player's line goes to its own
		// chat range.
		if err != nil {
			return err
		}
		if len(args) == 3 {
			return unsupported(what, "a flag with a game ID other than the attached player's")
		}
		text, err := b.expr(args[1])
		if err != nil {
			return err
		}
		gid := strings.TrimSuffix(strings.TrimPrefix(mob, "R.unit_monster("), ")")
		b.line("R.unittalk(%s, %s)", gid, text.asText())
		return nil
	}
	kind, _, err := b.gidRef(what, args[0])
	if err != nil {
		return err
	}
	if kind != "player" {
		return unsupported(what, "an NPC's line, which reaches the chat range as AREA_CHAT_WOC, and npc:say_aloud reaches the NPC's view")
	}
	text, err := b.expr(stripSpeaker(args[1]))
	if err != nil {
		return err
	}
	var opts []string
	if len(args) == 3 {
		flag, err := b.fixed(what, "flag", args[2])
		if err != nil {
			return err
		}
		if flag == bcSelf {
			opts = append(opts, "to = player")
		}
	}
	b.line("player:say_aloud(%s)", strings.Join(withOptions([]string{text.asText()}, opts), ", "))
	return nil
}

// emotion converts emotion <emotion>{,<game ID>} (script.cpp 13818).
func (b *body) emotion(args []Expr) error {
	const what = "emotion"
	if len(args) < 1 || len(args) > 2 {
		return unsupported(what, "arguments")
	}
	e, err := b.constant(args[0], "emotions")
	if err != nil {
		return err
	}
	if len(args) == 1 {
		b.line("npc:show_emotion(%s)", e)
		return nil
	}
	kind, code, err := b.gidRef(what, args[1])
	if err != nil {
		return err
	}
	if kind == "player" {
		b.line("player:show_emotion(%s)", e)
		return nil
	}
	b.line("%s", b.onNPC(code, "show_emotion", e))
	return nil
}

// effectTarget reads a send target of specialeffect: AREA,
// ALL_SAMEMAP or SELF.
func (b *body) effectTarget(what string, e Expr) ([]string, error) {
	t, err := b.fixed(what, "send target", e)
	if err != nil {
		return nil, err
	}
	switch t {
	case sendAREA:
		return nil, nil
	case sendAllSameMap:
		return []string{`to = "map"`}, nil
	case sendSelf:
		return []string{"to = player"}, nil
	}
	return nil, unsupported(what, "a send target other than AREA, ALL_SAMEMAP and SELF")
}

// fixedEffect reads a fixed EF_ effect.
func (b *body) fixedEffect(what string, e Expr) (string, error) {
	v, err := b.expr(e)
	if err != nil {
		return "", err
	}
	name, ok := "", false
	if v.static != nil {
		name, ok = b.n.c.modules["effects"].Names[*v.static]
	}
	if !ok {
		// rAthena does nothing for an effect outside EF_NONE to EF_MAX.
		return "", unsupported(what, "an effect that is not a fixed EF_ effect")
	}
	return b.n.file.need("effects") + "." + name, nil
}

// specialeffect converts specialeffect <effect>{,<send target>{,"<NPC>"}}
// (script.cpp 15596): on the NPC, or on another NPC, to the area, the
// map or the attached player.
func (b *body) specialeffect(args []Expr) error {
	const what = "specialeffect"
	if len(args) < 1 || len(args) > 3 {
		return unsupported(what, "arguments")
	}
	var opts []string
	if len(args) >= 2 {
		var err error
		if opts, err = b.effectTarget(what, args[1]); err != nil {
			return err
		}
	}
	target := "npc"
	if len(args) == 3 {
		var err error
		if target, err = b.npcTarget(what, args[2]); err != nil {
			return err
		}
	}
	e, err := b.fixedEffect(what, args[0])
	if err != nil {
		return err
	}
	call := b.onNPC(target, "show_effect", withOptions([]string{e}, opts)...)
	if len(opts) == 1 && opts[0] == "to = player" {
		// buildin_specialeffect fails without an attached player.
		b.line("if player ~= nil then %s end", call)
		return nil
	}
	b.line("%s", call)
	return nil
}

// specialeffect2 converts specialeffect2 <effect>{,<send target>{,
// "<player>"}} (script.cpp 15634) on the attached player.
func (b *body) specialeffect2(args []Expr) error {
	const what = "specialeffect2"
	if len(args) < 1 || len(args) > 3 {
		return unsupported(what, "arguments")
	}
	if len(args) == 3 {
		p, err := b.expr(args[2])
		if err != nil {
			return err
		}
		if p.code != "player.name" {
			return unsupported(what, "on another character")
		}
	}
	e, err := b.constant(args[0], "effects")
	if err != nil {
		return err
	}
	var opts []string
	if len(args) >= 2 {
		if opts, err = b.effectTarget(what, args[1]); err != nil {
			return err
		}
	}
	b.line("player:show_effect(%s)", strings.Join(withOptions([]string{e}, opts), ", "))
	return nil
}

// removespecialeffect converts removespecialeffect <effect>{,AREA{,
// "<NPC>"}} and removespecialeffect2 <effect>{,AREA{,"<player>"}}
// (script.cpp 15652).
func (b *body) removespecialeffect(what string, args []Expr) error {
	if len(args) < 1 || len(args) > 3 {
		return unsupported(what, "arguments")
	}
	if len(args) >= 2 {
		t, err := b.fixed(what, "send target", args[1])
		if err != nil {
			return err
		}
		if t != sendAREA {
			return unsupported(what, "a send target other than AREA")
		}
	}
	e, err := b.fixedEffect(what, args[0])
	if err != nil {
		return err
	}
	if what == "removespecialeffect2" {
		if len(args) == 3 {
			p, err := b.expr(args[2])
			if err != nil {
				return err
			}
			if p.code != "player.name" {
				return unsupported(what, "on another character")
			}
		}
		b.line("player:remove_effect(%s)", e)
		return nil
	}
	target := "npc"
	if len(args) == 3 {
		if target, err = b.npcTarget(what, args[2]); err != nil {
			return err
		}
	}
	b.line("%s", b.onNPC(target, "remove_effect", e))
	return nil
}

// npcGID reads a game ID argument that must be an NPC's.
func (b *body) npcGID(what string, e Expr) (string, error) {
	kind, code, err := b.gidRef(what, e)
	if err != nil {
		return "", err
	}
	if kind != "npc" {
		return "", unsupported(what, "on a player")
	}
	return code, nil
}

// setunittitle converts setunittitle <game ID>,"<title>" (script.cpp
// 19944) for an NPC.
func (b *body) setunittitle(args []Expr) error {
	const what = "setunittitle"
	if len(args) != 2 {
		return unsupported(what, "arguments")
	}
	target, err := b.npcGID(what, args[0])
	if err != nil {
		return err
	}
	t, err := b.expr(args[1])
	if err != nil {
		return err
	}
	// safestrncpy cuts the title to NAME_LENGTH - 1 bytes.
	title := "R.title(" + t.asText() + ")"
	if t.str != nil {
		title = luaString(cutBytes(*t.str, maxTitle))
	}
	b.line("%s", b.onNPC(target, "set_title", title))
	return nil
}

// maxTitle is the most bytes of a unit's title: NAME_LENGTH - 1.
const maxTitle = 23

// cutBytes cuts a string to at most n bytes, at a character's start.
func cutBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// setnpcdisplay converts setnpcdisplay "<NPC>","<name>"{,<class>{,
// <size>}} and setnpcdisplay "<NPC>",<class> (script.cpp 17869). It
// gives 0, or 1 for an NPC that does not exist.
func (b *body) setnpcdisplay(args []Expr) (val, error) {
	const what = "setnpcdisplay"
	if len(args) < 2 || len(args) > 4 {
		return val{}, unsupported(what, "arguments")
	}
	target, err := b.npcTarget(what, args[0])
	if err != nil {
		return val{}, err
	}
	if len(args) == 4 {
		size, err := b.fixed(what, "size", args[3])
		if err != nil {
			return val{}, err
		}
		if size != -1 {
			return val{}, unsupported(what, "a size, which the API does not change")
		}
	}
	second, err := b.expr(args[1])
	if err != nil {
		return val{}, err
	}
	name, class := "nil", "nil"
	switch second.t {
	case tStr:
		name = second.code
		if len(args) >= 3 {
			c, err := b.expr(args[2])
			if err != nil {
				return val{}, err
			}
			class = c.asInt()
			if c.static != nil && *c.static == jtFakeNPC {
				class = "nil"
			}
		}
	case tInt:
		class = second.code
		if len(args) >= 3 {
			return val{}, unsupported(what, "arguments")
		}
	default:
		return val{}, unsupported(what, "a name or class that may be either")
	}
	return val{code: fmt.Sprintf("R.npc_display(%s, %s, %s)", target, name, class), t: tInt}, nil
}

// classchange converts classchange <class>{,"<NPC>"{,<flag>}} (script.cpp
// 15163): the class shows to the players near, or with BC_SELF to the
// attached player, and the NPC keeps its own, as npc:set_sprite with
// keep = false (review decision 59). BC_SELF without a player does
// nothing, as rAthena fails.
func (b *body) classchange(args []Expr) error {
	const what = "classchange"
	if len(args) < 1 || len(args) > 3 {
		return unsupported(what, "arguments")
	}
	self := false
	if len(args) == 3 {
		f, err := b.fixed(what, "flag", args[2])
		if err != nil {
			return err
		}
		self = f == bcSelf
	}
	target := "npc"
	if len(args) >= 2 {
		var err error
		if target, err = b.selfOrNPC(what, args[1]); err != nil {
			return err
		}
	}
	c, err := b.expr(args[0])
	if err != nil {
		return err
	}
	if self {
		b.line("if player ~= nil then %s end", b.onNPC(target, "set_sprite", c.asInt(), "{ keep = false, to = player }"))
		return nil
	}
	b.line("%s", b.onNPC(target, "set_sprite", c.asInt(), "{ keep = false }"))
	return nil
}

// setunitdata converts setunitdata <game ID>,UNPC_<look>,<value>
// (script.cpp 19398) for an NPC's class and looks.
func (b *body) setunitdata(args []Expr) error {
	const what = "setunitdata"
	if len(args) != 3 {
		return unsupported(what, "arguments")
	}
	if mob, ok, err := b.mobGID(what, args[0]); ok || err != nil {
		if err != nil {
			return err
		}
		return b.setunitdataMob(mob, args)
	}
	target, err := b.npcGID(what, args[0])
	if err != nil {
		return err
	}
	t, err := b.fixed(what, "type", args[1])
	if err != nil {
		return err
	}
	v, err := b.expr(args[2])
	if err != nil {
		return err
	}
	field := unpcLooks[t]
	value := v.asInt()
	switch {
	case t == unpcSex:
		field = "sex"
		value = fmt.Sprintf(`(%s == %d and "male" or "female")`, v.asInt(), sexMale)
		if v.static != nil {
			value = map[bool]string{true: `"male"`, false: `"female"`}[*v.static == sexMale]
		}
	case field == "":
		return unsupported(what, "a type other than UNPC_CLASS, UNPC_SEX and the UNPC_ looks")
	}
	b.line("%s", b.onNPC(target, "set_look", fmt.Sprintf("{ %s = %s }", field, value)))
	return nil
}

// mobGID reads a game ID of a monster: a value that is not the attached
// player's or an NPC's, which R.unit_monster finds on any map when the
// script runs (review decision 59). ok is false for a player's or an
// NPC's game ID.
func (b *body) mobGID(what string, e Expr) (code string, ok bool, err error) {
	if _, _, gerr := b.gidRef(what, e); gerr == nil {
		return "", false, nil
	}
	v, err := b.expr(e)
	if err != nil {
		return "", true, err
	}
	if v.t == tStr {
		return "", true, unsupported(what, "a game ID that is a name")
	}
	return "R.unit_monster(" + v.asInt() + ")", true, nil
}

// setunitdataMob converts setunitdata on a monster's game ID with the
// UMOB_ types that the API has (script.cpp 19447 to 19493).
func (b *body) setunitdataMob(mob string, args []Expr) error {
	const what = "setunitdata"
	t, err := b.fixed(what, "type", args[1])
	if err != nil {
		return err
	}
	kind := map[int64]string{umobHP: "hp", umobSpeed: "speed", umobMode: "modes", umobDmgImmune: "immune"}[t]
	if kind == "" {
		return unsupported(what, "a monster's UMOB_ type other than UMOB_HP, UMOB_SPEED, UMOB_MODE and UMOB_DMGIMMUNE")
	}
	v, err := b.expr(args[2])
	if err != nil {
		return err
	}
	gid := strings.TrimSuffix(strings.TrimPrefix(mob, "R.unit_monster("), ")")
	b.line("R.umob_set(%s, %q, %s)", gid, kind, v.asInt())
	return nil
}

// unitkill converts unitkill <game ID> (script.cpp 20061) on a monster:
// it dies with no exp and no drops, and its label runs.
func (b *body) unitkill(args []Expr) error {
	const what = "unitkill"
	if len(args) != 1 {
		return unsupported(what, "arguments")
	}
	mob, ok, err := b.mobGID(what, args[0])
	if err != nil {
		return err
	}
	if !ok {
		return unsupported(what, "on a player or an NPC")
	}
	b.line("do local m = %s if m ~= nil then m:kill() end end", mob)
	return nil
}

// unitskillusepos converts unitskillusepos <game ID>,<skill>,<level>,
// <x>,<y> (script.cpp 20305) on a monster, with the skill's own cast
// time.
func (b *body) unitskillusepos(args []Expr) error {
	const what = "unitskillusepos"
	if len(args) != 5 {
		return unsupported(what, "a cast time or a cancel flag of its own")
	}
	mob, ok, err := b.mobGID(what, args[0])
	if err != nil {
		return err
	}
	if !ok {
		return unsupported(what, "on a player or an NPC")
	}
	skill, err := b.skillArg(what, args[1])
	if err != nil {
		return err
	}
	vals, err := b.args(args[2:])
	if err != nil {
		return err
	}
	b.line("do local m = %s if m ~= nil then m:use_skill_at(%s, %s, %s, %s) end end", mob, skill, vals[0].asInt(), vals[1].asInt(), vals[2].asInt())
	return nil
}

// unitwalk converts unitwalk <game ID>,<x>,<y> (script.cpp 19996) for an
// NPC: whether it can walk there.
func (b *body) unitwalk(args []Expr) (val, error) {
	call, err := b.unitwalkCall(args)
	if err != nil {
		return val{}, err
	}
	return val{code: call, t: tInt}, nil
}

// unitwalkCall is unitwalk's R.unitwalk: the 50 ms timer and the check
// of a forced walk (script.cpp 20015, 20032).
func (b *body) unitwalkCall(args []Expr) (string, error) {
	const what = "unitwalk"
	if len(args) != 3 && len(args) != 4 {
		return "", unsupported(what, "arguments")
	}
	target, err := b.npcGID(what, args[0])
	if err != nil {
		return "", err
	}
	x, err := b.expr(args[1])
	if err != nil {
		return "", err
	}
	y, err := b.expr(args[2])
	if err != nil {
		return "", err
	}
	if len(args) == 4 {
		// The event label of the walk's end (unit.cpp 650), as
		// npc:walk_to's label (review decision 59).
		label, err := b.mobLabel(what, args[3])
		if err != nil {
			return "", err
		}
		if label == "" {
			label = "nil"
		}
		return fmt.Sprintf("R.unitwalk(%s, %s, %s, %s)", target, x.asInt(), y.asInt(), label), nil
	}
	return fmt.Sprintf("R.unitwalk(%s, %s, %s)", target, x.asInt(), y.asInt()), nil
}

// unitwarp converts unitwarp <game ID>,"<map>",<x>,<y> (script.cpp 20075)
// for the attached player, and for an NPC on its own map.
func (b *body) unitwarp(args []Expr) error {
	const what = "unitwarp"
	if len(args) != 4 {
		return unsupported(what, "arguments")
	}
	if mob, ok, err := b.mobGID(what, args[0]); ok || err != nil {
		if err != nil {
			return err
		}
		// A monster on its own map, "this" or its map's name.
		m, err := b.expr(args[1])
		if err != nil {
			return err
		}
		vals, err := b.args(args[2:])
		if err != nil {
			return err
		}
		cond := "true"
		if m.str == nil || *m.str != "this" {
			cond = "m.map == " + m.asText()
		}
		b.line("do local m = %s if m ~= nil and %s then m:warp_to(%s, %s) end end", mob, cond, vals[0].asInt(), vals[1].asInt())
		return nil
	}
	kind, target, err := b.gidRef(what, args[0])
	if err != nil {
		return err
	}
	m, err := b.expr(args[1])
	if err != nil {
		return err
	}
	x, err := b.expr(args[2])
	if err != nil {
		return err
	}
	y, err := b.expr(args[3])
	if err != nil {
		return err
	}
	if kind == "player" {
		if m.str != nil && *m.str == "this" {
			b.line("player:warp(player.map, %s, %s)", x.asInt(), y.asInt())
			return nil
		}
		b.line("player:warp(%s, %s, %s)", m.asText(), x.asInt(), y.asInt())
		return nil
	}
	if m.str == nil || *m.str != "this" && (*m.str != b.n.line.Map || target != "npc") {
		return unsupported(what, "an NPC to another map")
	}
	b.line("%s", b.onNPC(target, "move_to", x.asInt(), y.asInt()))
	return nil
}

// unitstopwalk converts unitstopwalk <game ID>{,<flag>} (script.cpp
// 20183) for an NPC.
func (b *body) unitstopwalk(args []Expr) error {
	const what = "unitstopwalk"
	if len(args) < 1 || len(args) > 2 {
		return unsupported(what, "arguments")
	}
	target, err := b.npcGID(what, args[0])
	if err != nil {
		return err
	}
	send := false
	if len(args) == 2 {
		if send, err = b.stopFlag(what, args[1]); err != nil {
			return err
		}
	}
	// rAthena's default USW_NONE stops at once and sends no fixpos
	// (unit.cpp 1725, 1732), as npc:stop's send_cell = false.
	b.line("R.npc_stop(%s, %t)", target, send)
	return nil
}

// stopFlag reads a unit_stop_walking flag of npcstop and unitstopwalk
// (unit.cpp 1695): without USW_FIXPOS, USW_MOVE_ONCE and
// USW_MOVE_FULL_CELL the unit stops at once with no packet, as npc:stop's
// send_cell = false; USW_FORCE_STOP and USW_RELEASE_TARGET change only
// what the command returns and the chase target, which an NPC has not.
// USW_FIXPOS with USW_MOVE_FULL_CELL finishes the cell and sends it, as
// npc:stop. Other flags do not convert.
func (b *body) stopFlag(what string, e Expr) (bool, error) {
	f, err := b.fixed(what, "flag", e)
	if err != nil {
		return false, err
	}
	switch {
	case f&(uswFixPos|uswMoveOnce|uswFullCell) == 0:
		return false, nil
	case f&uswFixPos != 0 && f&uswFullCell != 0 && f&uswMoveOnce == 0:
		return true, nil
	}
	return false, unsupported(what, "a flag with USW_MOVE_ONCE, or with only one of USW_FIXPOS and USW_MOVE_FULL_CELL")
}

// npcMove converts npcspeed, npcwalkto, npcstop and movenpc (script.cpp
// 16146 to 16337).
func (b *body) npcMove(what string, args []Expr) error {
	arity := map[string][2]int{"npcspeed": {1, 2}, "npcwalkto": {2, 3}, "npcstop": {0, 2}, "movenpc": {3, 4}}[what]
	if len(args) < arity[0] || len(args) > arity[1] {
		return unsupported(what, "arguments")
	}
	target := "npc"
	pick := func(i int) error {
		if len(args) > i {
			var err error
			target, err = b.npcTarget(what, args[i])
			return err
		}
		return nil
	}
	switch what {
	case "npcspeed":
		if err := pick(1); err != nil {
			return err
		}
		v, err := b.expr(args[0])
		if err != nil {
			return err
		}
		b.line("%s", b.onNPC(target, "set_speed", v.asInt()))
	case "npcwalkto":
		if err := pick(2); err != nil {
			return err
		}
		x, err := b.expr(args[0])
		if err != nil {
			return err
		}
		y, err := b.expr(args[1])
		if err != nil {
			return err
		}
		b.line("%s", b.onNPC(target, "walk_to", x.asInt(), y.asInt()))
	case "npcstop":
		if err := pick(0); err != nil {
			return err
		}
		send := true
		if len(args) == 2 {
			var err error
			if send, err = b.stopFlag(what, args[1]); err != nil {
				return err
			}
		}
		// USW_FORCE_STOP ends a forced walk too.
		b.line("R.npc_stop(%s, %t)", target, send)
	case "movenpc":
		if err := pick(0); err != nil {
			return err
		}
		x, err := b.expr(args[1])
		if err != nil {
			return err
		}
		y, err := b.expr(args[2])
		if err != nil {
			return err
		}
		if len(args) == 4 {
			// movenpc's direction, dir % 8 (script.cpp 16161).
			d, err := b.expr(args[3])
			if err != nil {
				return err
			}
			b.line("%s", b.onNPC(target, "move_to", x.asInt(), y.asInt(), "("+d.asInt()+") % 8"))
			break
		}
		b.line("%s", b.onNPC(target, "move_to", x.asInt(), y.asInt()))
	}
	return nil
}

// strtol is C's strtol with base 0 on a whole string, as
// progressbar_npc reads its color: "0x" is hex, a leading 0 octal, and
// the digits stop at the first that does not fit.
func strtol(s string) int64 {
	s = strings.TrimSpace(s)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimLeft(s, "+-")
	base := 10
	switch {
	case strings.HasPrefix(strings.ToLower(s), "0x"):
		base, s = 16, s[2:]
	case strings.HasPrefix(s, "0"):
		base = 8
	}
	end := 0
	for end < len(s) {
		if _, err := strconv.ParseInt(s[end:end+1], base, 64); err != nil {
			break
		}
		end++
	}
	n, _ := strconv.ParseInt(s[:end], base, 64)
	if neg {
		n = -n
	}
	return n
}

// progressbarNPC converts progressbar_npc "<color>",<seconds>{,"<NPC>"}
// (script.cpp 22546). rAthena reads the color with strtol and base 0,
// so "FFFF00" is 0.
func (b *body) progressbarNPC(args []Expr) error {
	const what = "progressbar_npc"
	if len(args) < 2 || len(args) > 3 {
		return unsupported(what, "arguments")
	}
	c, ok := args[0].(*Str)
	if !ok {
		return unsupported(what, "a computed color")
	}
	color := strtol(c.Value) & 0xFFFFFF
	s, err := b.expr(args[1])
	if err != nil {
		return err
	}
	target := "npc"
	if len(args) == 3 {
		if target, err = b.npcTarget(what, args[2]); err != nil {
			return err
		}
	}
	b.line("%s", b.onNPC(target, "show_progress", s.asInt(), fmt.Sprintf("{ color = 0x%06X }", color)))
	return nil
}

// skillArg reads a skill argument: a name, or a fixed ID.
func (b *body) skillArg(what string, e Expr) (string, error) {
	v, err := b.expr(e)
	if err != nil {
		return "", err
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
		return "", unsupported(what, "a computed skill")
	}
	return b.n.file.record("skills.id", skill), nil
}

// npcskill converts npcskill <skill>,<level>,<stat>,<NPC level>
// (script.cpp 23286): the NPC casts on the attached player.
func (b *body) npcskill(args []Expr) error {
	const what = "npcskill"
	if len(args) != 4 {
		return unsupported(what, "arguments")
	}
	sk, err := b.skillArg(what, args[0])
	if err != nil {
		return err
	}
	vals, err := b.args(args[1:])
	if err != nil {
		return err
	}
	b.line("npc:use_skill(%s, %s, player, { stat = %s, npc_level = %s })", sk, vals[0].asInt(), vals[1].asInt(), vals[2].asInt())
	return nil
}

// skilleffect converts skilleffect <skill>,<level>{,<game ID>}
// (script.cpp 15519).
func (b *body) skilleffect(args []Expr) error {
	const what = "skilleffect"
	if len(args) < 2 || len(args) > 3 {
		return unsupported(what, "arguments")
	}
	sk, err := b.skillArg(what, args[0])
	if err != nil {
		return err
	}
	lv, err := b.expr(args[1])
	if err != nil {
		return err
	}
	if len(args) == 2 {
		b.line("player:show_skill_effect(%s, %s)", sk, lv.asInt())
		return nil
	}
	kind, code, err := b.gidRef(what, args[2])
	if err != nil {
		return err
	}
	if kind == "player" {
		b.line("player:show_skill_effect(%s, %s)", sk, lv.asInt())
		return nil
	}
	b.line("%s", b.onNPC(code, "show_skill_effect", sk, lv.asInt()))
	return nil
}

// soundeffect converts soundeffect "<file>",<type> (script.cpp 15281)
// and soundeffectall "<file>",<type>{,"<map>"} (15308). Type 1 repeats,
// and type 2, which stops a sound, does not convert.
func (b *body) soundeffect(what string, args []Expr) error {
	max := map[string]int{"soundeffect": 2, "soundeffectall": 3}[what]
	if what == "soundeffectall" && len(args) == 7 {
		// rAthena reads a part of a map and plays nothing (script.cpp
		// 15330).
		b.line("-- soundeffectall of a part of a map plays nothing in rAthena")
		return nil
	}
	if len(args) < 2 || len(args) > max {
		return unsupported(what, "arguments")
	}
	f, ok := args[0].(*Str)
	if !ok {
		return unsupported(what, "a computed file")
	}
	file, ok := assetName(f.Value, ".wav")
	if !ok {
		return unsupported(what, "the file %q", f.Value)
	}
	t, err := b.fixed(what, "type", args[1])
	if err != nil {
		return err
	}
	var opts []string
	switch t {
	case 0:
	case 1:
		opts = append(opts, `["repeat"] = true`)
	default:
		return unsupported(what, "type %d, which stops a sound", t)
	}
	switch {
	case what == "soundeffect":
		b.line("player:play_sound(%s)", strings.Join(withOptions([]string{luaString(file)}, opts), ", "))
	case len(args) == 3:
		m, err := b.expr(args[2])
		if err != nil {
			return err
		}
		b.n.file.need("maps")
		b.line("maps.play_sound(%s)", strings.Join(withOptions([]string{m.asText(), luaString(file)}, opts), ", "))
	default:
		// Around the attached player when there is one, from it, else
		// around the NPC (script.cpp 15315).
		withTo := append([]string{"to = \"area\""}, opts...)
		b.line("if player ~= nil then player:play_sound(%s) else npc:play_sound(%s) end",
			strings.Join(withOptions([]string{luaString(file)}, withTo), ", "), strings.Join(withOptions([]string{luaString(file)}, opts), ", "))
	}
	return nil
}

// playbgm converts playBGM "<file>" and playBGMall "<file>",
// "<map>" (script.cpp 15226 and 15253).
func (b *body) playbgm(what string, args []Expr) error {
	if len(args) == 0 {
		return unsupported(what, "arguments")
	}
	f, ok := args[0].(*Str)
	if !ok {
		return unsupported(what, "a computed file")
	}
	file, ok := assetName(f.Value, ".mp3")
	if !ok {
		return unsupported(what, "the file %q", f.Value)
	}
	if what == "playbgm" {
		if len(args) != 1 {
			return unsupported(what, "arguments")
		}
		b.line("player:play_music(%s)", luaString(file))
		return nil
	}
	switch len(args) {
	case 1:
		// Every player of the server (map_foreachpc).
		b.n.file.need("world")
		b.line("world.play_music(%s)", luaString(file))
		return nil
	case 2, 6:
	default:
		return unsupported(what, "arguments")
	}
	vals, err := b.args(args[1:])
	if err != nil {
		return err
	}
	b.n.file.need("maps")
	if len(args) == 6 {
		// A part of a map (map_foreachinallarea).
		b.line("R.on_map(%s, maps.play_music, %s, { rectangle = { { %s, %s }, { %s, %s } } })", vals[0].asText(), luaString(file),
			vals[1].asInt(), vals[2].asInt(), vals[3].asInt(), vals[4].asInt())
		return nil
	}
	b.line("maps.play_music(%s, %s)", vals[0].asText(), luaString(file))
	return nil
}

// showdigit converts showdigit <value>{,<type>} (script.cpp 22745).
func (b *body) showdigit(args []Expr) error {
	const what = "showdigit"
	if len(args) < 1 || len(args) > 2 {
		return unsupported(what, "arguments")
	}
	v, err := b.expr(args[0])
	if err != nil {
		return err
	}
	kind := int64(0)
	if len(args) == 2 {
		if kind, err = b.fixed(what, "type", args[1]); err != nil {
			return err
		}
	}
	names := []string{"show", "count_up", "count_down", "count_down_to_zero"}
	if kind < 0 || kind >= int64(len(names)) {
		return unsupported(what, "type %d", kind)
	}
	b.line("player:show_digits(%s, %s)", v.asInt(), luaString(names[kind]))
	return nil
}

// viewpointmap converts viewpointmap "<map>",<type>,<x>,<y>,<id>,<color>
// (script.cpp 7013): type 1 marks, and type 2 removes.
func (b *body) viewpointmap(args []Expr) error {
	const what = "viewpointmap"
	if len(args) != 6 {
		return unsupported(what, "arguments")
	}
	t, err := b.fixed(what, "type", args[1])
	if err != nil {
		return err
	}
	vals, err := b.args(args)
	if err != nil {
		return err
	}
	b.n.file.need("maps")
	switch t {
	case 1:
		b.line("maps.mark_minimap(%s, %s, %s, %s, %s)", vals[0].asText(), vals[4].asInt(), vals[2].asInt(), vals[3].asInt(), vals[5].asInt())
	case 2:
		b.line("maps.unmark_minimap(%s, %s)", vals[0].asText(), vals[4].asInt())
	default:
		return unsupported(what, "type %d", t)
	}
	return nil
}

// questinfo converts questinfo <icon>{,<mark>{,"<condition>"}}
// (script.cpp 20949). The condition is rAthena script that
// achievement_check_condition runs: it converts to a function of the
// player.
func (b *body) questinfo(args []Expr) error {
	const what = "questinfo"
	if len(args) < 1 || len(args) > 3 {
		return unsupported(what, "arguments")
	}
	if b.n.isFunction {
		return unsupported(what, "in a function")
	}
	icon, err := b.fixed(what, "icon", args[0])
	if err != nil {
		return err
	}
	name, ok := questinfoIcons[icon]
	if !ok {
		// questinfo turns an unknown icon into QTYPE_NONE, which shows
		// nothing when its condition holds.
		return unsupported(what, "the icon %d, QTYPE_NONE", icon)
	}
	var opts []string
	if len(args) >= 2 {
		m, err := b.fixed(what, "mark", args[1])
		if err != nil {
			return err
		}
		if mark, ok := questMarks[m]; ok {
			opts = append(opts, "mark = "+luaString(mark))
		}
	}
	if len(args) == 3 {
		s, ok := args[2].(*Str)
		if !ok {
			return unsupported(what, "a computed condition")
		}
		cond, err := parseCondition(s.Value)
		if err != nil {
			return unsupported(what, "the condition %q: %v", s.Value, err)
		}
		v, err := b.expr(cond)
		if err != nil {
			return err
		}
		opts = append(opts, "condition = function(player) return "+v.asBool()+" end")
	}
	b.line("npc:add_quest_icon(%s)", strings.Join(withOptions([]string{luaString(name)}, opts), ", "))
	return nil
}

// parseCondition parses a condition of questinfo, which rAthena wraps in
// achievement_condition( ... ).
func parseCondition(src string) (Expr, error) {
	block, err := ParseNPC("if (" + src + ") end;")
	if err != nil {
		return nil, err
	}
	if len(block.Stmts) != 1 {
		return nil, fmt.Errorf("not one expression")
	}
	s, ok := block.Stmts[0].(*If)
	if !ok {
		return nil, fmt.Errorf("not an expression")
	}
	return s.Cond, nil
}

// openQuestUI converts open_quest_ui {<quest ID>{,<char ID>}}
// (script.cpp 27055).
func (b *body) openQuestUI(args []Expr) error {
	const what = "open_quest_ui"
	if len(args) > 2 {
		return unsupported(what, "arguments")
	}
	if err := b.questPlayer(what, args, 1); err != nil {
		return err
	}
	if len(args) == 0 {
		b.line("player:open_quest_log()")
		return nil
	}
	id, known, err := b.questID(what, args[0])
	if err != nil {
		return err
	}
	if id.static == nil {
		return unsupported(what, "a computed quest")
	}
	if *id.static == 0 || !known {
		// open_quest_ui only warns for a quest the player lacks.
		b.line("player:open_quest_log()")
		return nil
	}
	b.line("player:open_quest_log(%s)", id.asInt())
	return nil
}
