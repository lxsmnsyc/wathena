package convert

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// NPC timers: rAthena's initnpctimer, startnpctimer, stopnpctimer,
// setnpctimer and getnpctimer, and the OnTimer<n> labels they run
// (script.cpp 11710 to 11890, npc.cpp 1431 to 1735). R.npc_timer in
// rathena.lua keeps the timer's state over timers.after.
//
// These forms convert:
//   - The NPC's own timer: no NPC name, or the NPC's own name for an NPC
//     without duplicates. A duplicate runs its source's script, where the
//     name is the source's. Another NPC's timer needs events between
//     NPCs.
//   - A timer without a player: initnpctimer and startnpctimer with an
//     attach flag of 0. The labels run without a player, as rAthena's
//     npc_timerevent does when nd->u.scr.rid is 0.
//   - getnpctimer(0), the timer's tick, getnpctimer(1), whether the next
//     label is due, and getnpctimer(2), the number of labels. The tick
//     follows the server clock (wa.clock, review decision 58), so it runs
//     while the map has no players, and startnpctimer resumes from the
//     tick where stopnpctimer stopped.

// timerLabel is an OnTimer<n> label: its tick in milliseconds, and its
// name in lower case.
type timerLabel struct {
	tick  int64
	label string
}

// timerLabels finds the OnTimer<n> labels of a script's top level, by
// tick. rAthena's npc_timerevent_export matches "OnTimer" with case and
// reads the digits after it with sscanf. A label of the same tick as an
// earlier one goes after it.
func timerLabels(stmts []Stmt) []timerLabel {
	var out []timerLabel
	seen := map[string]bool{}
	for _, s := range stmts {
		l, ok := s.(*Label)
		if !ok {
			continue
		}
		digits, ok := strings.CutPrefix(l.Name, "OnTimer")
		if !ok || digits == "" || strings.Trim(digits, "0123456789") != "" || seen[strings.ToLower(l.Name)] {
			continue
		}
		tick, err := strconv.ParseInt(digits, 10, 32)
		if err != nil {
			continue
		}
		seen[strings.ToLower(l.Name)] = true
		out = append(out, timerLabel{tick: tick, label: strings.ToLower(l.Name)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].tick < out[j].tick })
	return out
}

// timerVar is the Lua local of the NPC's timer. luaName makes lower-case
// names, so it cannot clash with a converted name.
const timerVar = "NpcTimer"

// ownTimer checks the NPC name of a timer command: none, or the NPC's
// own name in a string.
func (b *body) ownTimer(what string, name Expr) error {
	n := b.n
	if n.isFunction {
		return unsupported(what, "in a function, where the timer is the calling NPC's")
	}
	if name == nil {
		return nil
	}
	s, ok := name.(*Str)
	switch {
	case !ok:
		return unsupported(what, "the timer of an NPC named by a computed name")
	case s.Value != n.line.Name:
		return unsupported(what, "the timer of another NPC (%s)", s.Value)
	case n.c.duplicated[n.line.Name]:
		return unsupported(what, "the timer of an NPC named by its name, in a script that its duplicates run")
	}
	return nil
}

// timerCommand writes initnpctimer, startnpctimer, stopnpctimer and
// setnpctimer.
func (b *body) timerCommand(name string, args []Expr) error {
	if name == "setnpctimer" {
		if len(args) < 1 || len(args) > 2 {
			return unsupported(name, "arguments")
		}
		var npcName Expr
		if len(args) == 2 {
			npcName = args[1]
		}
		if err := b.ownTimer(name, npcName); err != nil {
			return err
		}
		v, err := b.expr(args[0])
		if err != nil {
			return err
		}
		b.n.timer = true
		b.line("%s:set(%s)", timerVar, v.asInt())
		return nil
	}
	// One argument is the NPC's name when it is a string, and the flag
	// otherwise. Two are the name and the flag.
	var npcName, flag Expr
	switch len(args) {
	case 0:
	case 1:
		v, err := b.expr(args[0])
		if err != nil {
			return err
		}
		if v.t == tStr {
			npcName = args[0]
		} else {
			flag = args[0]
		}
	case 2:
		npcName, flag = args[0], args[1]
	default:
		return unsupported(name, "arguments")
	}
	if err := b.ownTimer(name, npcName); err != nil {
		return err
	}
	if flag != nil && name != "stopnpctimer" {
		// stopnpctimer's flag detaches the player, and no converted timer
		// has one.
		v, err := b.expr(flag)
		if err != nil {
			return err
		}
		if v.static == nil || *v.static != 0 {
			return unsupported(name, "a timer attached to the player")
		}
	}
	method := map[string]string{"initnpctimer": "init", "startnpctimer": "start", "stopnpctimer": "stop"}[name]
	b.n.timer = true
	b.line("%s:%s()", timerVar, method)
	return nil
}

// getnpctimer writes getnpctimer(1) and getnpctimer(2).
func (b *body) getnpctimer(args []Expr) (val, error) {
	const what = "getnpctimer"
	if len(args) < 1 || len(args) > 2 {
		return val{}, unsupported(what, "arguments")
	}
	var npcName Expr
	if len(args) == 2 {
		npcName = args[1]
	}
	if err := b.ownTimer(what, npcName); err != nil {
		return val{}, err
	}
	t, err := b.expr(args[0])
	if err != nil {
		return val{}, err
	}
	if t.static == nil {
		return val{}, unsupported(what, "a computed type")
	}
	b.n.timer = true
	switch *t.static {
	case 0:
		return val{code: timerVar + ":tick()", t: tInt}, nil
	case 1:
		return val{code: timerVar + ":due()", t: tInt}, nil
	case 2:
		return intVal(int64(len(b.n.timerLabels))), nil
	}
	// rAthena returns 0 for any other type.
	return intVal(0), nil
}

// timerSetup is the line that makes the NPC's timer, with its labels'
// functions.
func (n *npcConv) timerSetup() string {
	parts := make([]string, len(n.timerLabels))
	for i, l := range n.timerLabels {
		parts[i] = fmt.Sprintf("{ %d, %s }", l.tick, n.eventFns[l.label])
	}
	return fmt.Sprintf("%s = R.npc_timer({ %s })", timerVar, strings.Join(parts, ", "))
}
