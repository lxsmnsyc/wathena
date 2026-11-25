package convert

import (
	"fmt"
	"strings"
)

// Chat rooms (proposal 0006, section 2; decision 0080): rAthena's waiting
// rooms. waitingroom "<title>",<limit>{,"<event>"{,<trigger>{,<zeny>{,
// <min level>{,<max level>}}}}} becomes npc:open_chat_room{...} on the
// NPC itself. The event is "<NPC>::<label>" with the NPC's record name,
// or "::<label>" for the NPC itself. The other commands name an NPC or
// act on the NPC itself, and go through R's helpers, which find the NPC's
// room and do nothing for an NPC without one, as rAthena.
// getwaitingroomusers is not converted: no script uses it.

// roomCommand converts the waiting room commands that do something. ok
// is false for another name.
func (b *body) roomCommand(name string, args []Expr) (ok bool, err error) {
	switch name {
	case "waitingroom":
		return true, b.waitingroom(args)
	case "delwaitingroom", "kickwaitingroomall", "enablewaitingroomevent", "disablewaitingroomevent":
		if len(args) > 1 {
			return true, unsupported(name, "arguments")
		}
		target := "npc"
		if len(args) == 1 {
			if target, _, err = b.npcRef(name, args[0]); err != nil {
				return true, err
			}
		}
		call := map[string]string{
			"delwaitingroom": "R.room_close(%s)", "kickwaitingroomall": "R.room_kick_all(%s)",
			"enablewaitingroomevent": "R.room_event(%s, true)", "disablewaitingroomevent": "R.room_event(%s, false)",
		}[name]
		b.line(call, target)
		return true, nil
	case "waitingroomkick":
		if len(args) != 2 {
			return true, unsupported(name, "arguments")
		}
		target, _, err := b.npcRef(name, args[0])
		if err != nil {
			return true, err
		}
		who, err := b.expr(args[1])
		if err != nil {
			return true, err
		}
		b.line("R.room_kick(%s, %s)", target, who.asText())
		return true, nil
	case "warpwaitingpc":
		return true, b.warpwaitingpc(args)
	}
	return false, nil
}

// roomEvent converts the event name of waitingroom to the string the API
// takes: "<record name>::<label>", or "::<label>" for the NPC itself.
func (b *body) roomEvent(e Expr) (string, error) {
	const what = "waitingroom"
	switch e := e.(type) {
	case *Str:
		if e.Value == "" {
			return `""`, nil
		}
		i := strings.LastIndex(e.Value, "::")
		if i < 0 || !labelName.MatchString(e.Value[i+2:]) {
			return "", unsupported(what, "%q is not an event label NPC::OnX", e.Value)
		}
		if i == 0 {
			return luaString(e.Value), nil
		}
		return luaString(b.n.c.recordFor(e.Value[:i]) + e.Value[i:]), nil
	case *Binary:
		target, label, computed, err := b.eventLabel(what, e)
		if err != nil {
			return "", err
		}
		if computed || target != "npc" {
			return "", unsupported(what, "a computed event label")
		}
		return luaString("::" + label), nil
	}
	return "", unsupported(what, "a computed event label")
}

// waitingroom converts the command itself.
func (b *body) waitingroom(args []Expr) error {
	const what = "waitingroom"
	if len(args) < 2 || len(args) > 8 {
		return unsupported(what, "arguments")
	}
	title, err := b.expr(args[0])
	if err != nil {
		return err
	}
	if title.t == tInt || title.t == tBool {
		return unsupported(what, "a title that is not text")
	}
	limit, err := b.expr(args[1])
	if err != nil {
		return err
	}
	opts := []string{"title = " + title.asText(), "limit = " + limit.asInt()}
	if len(args) > 2 {
		ev, err := b.roomEvent(args[2])
		if err != nil {
			return err
		}
		if ev != `""` {
			opts = append(opts, "event = "+ev)
		}
	}
	for i, field := range []string{"trigger", "zeny", "min_level", "max_level"} {
		if len(args) <= i+3 {
			break
		}
		v, err := b.expr(args[i+3])
		if err != nil {
			return err
		}
		opts = append(opts, field+" = "+v.asInt())
	}
	b.line("npc:open_chat_room({ %s })", strings.Join(opts, ", "))
	return nil
}

// warpwaitingpc converts warpwaitingpc "<map>",<x>,<y>{,<n>} on the NPC
// itself. The helper warps the room's players and writes $@warpwaitingpc
// and $@warpwaitingpcnum.
func (b *body) warpwaitingpc(args []Expr) error {
	const what = "warpwaitingpc"
	if len(args) < 3 || len(args) > 4 {
		return unsupported(what, "arguments")
	}
	m, err := b.expr(args[0])
	if err != nil {
		return err
	}
	if m.t == tInt || m.t == tBool {
		return unsupported(what, "a map that is not text")
	}
	x, err := b.expr(args[1])
	if err != nil {
		return err
	}
	y, err := b.expr(args[2])
	if err != nil {
		return err
	}
	n := "nil"
	if len(args) == 4 {
		v, err := b.expr(args[3])
		if err != nil {
			return err
		}
		n = v.asInt()
	}
	b.line("R.warpwaitingpc(npc, %s, %s, %s, %s)", m.asText(), x.asInt(), y.asInt(), n)
	return nil
}

// roomValue converts the waiting room functions that return a value. ok
// is false for another name.
func (b *body) roomValue(name string, args []Expr) (v val, ok bool, err error) {
	switch name {
	case "getwaitingroomstate":
		if len(args) < 1 || len(args) > 2 {
			return val{}, true, unsupported(name, "arguments")
		}
		t, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		target := "npc"
		if len(args) == 2 {
			if target, _, err = b.npcRef(name, args[1]); err != nil {
				return val{}, true, err
			}
		}
		typ := tInt
		switch {
		case t.static == nil:
			typ = tAny
		case *t.static == 4 || *t.static == 5 || *t.static == 16:
			typ = tStr
		}
		return val{code: fmt.Sprintf("R.waitingroomstate(%s, %s)", t.asInt(), target), t: typ}, true, nil
	case "checkchatting":
		if len(args) > 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		if len(args) == 1 && !b.attachedChar(args[0]) {
			return val{}, true, unsupported(name, "another character")
		}
		return val{code: "(player.in_chat_room and 1 or 0)", t: tInt}, true, nil
	}
	return val{}, false, nil
}
