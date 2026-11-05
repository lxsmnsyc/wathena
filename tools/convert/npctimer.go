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
//     without duplicates. The labels run without a player, as rAthena's
//     npc_timerevent does when nd->u.scr.rid is 0.
//   - A timer attached to the player: initnpctimer and startnpctimer with
//     an attach flag. The timer is the player's own (R.npc_timer's of),
//     and runs its labels over npc:fire_event with the player (review
//     decision 60, C3).
//   - Another NPC's timer commands, over events between NPCs (C4). Every
//     NPC with timers listens to hidden labels (timerHandlers). The
//     command runs after the current step, as every event between NPCs.
//     setnpctimer there takes the tick 0 only.
//   - getnpctimer(0), the timer's tick, getnpctimer(1), whether the next
//     label is due, and getnpctimer(2), the number of labels, of the
//     NPC's own timer. The tick follows the server clock (wa.clock,
//     review decision 58), so it runs while the map has no players, and
//     startnpctimer resumes from the tick where stopnpctimer stopped.
//   - The timer's labels wait on the server clock, so a sleeping map
//     wakes for them (review decision 60, C1).

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

// Hidden labels: an NPC with timers listens to these npc_event labels,
// so that another NPC's timer commands reach it (review decision 60,
// C4). The event's player is the attached player, if any. Only a tick of
// 0 can be set from another NPC.
const (
	timerLabelInit  = "watimerinit"
	timerLabelStart = "watimerstart"
	timerLabelStop  = "watimerstop"
	timerLabelSet0  = "watimerset0"
)

// ownTimer checks the NPC name of a timer command. own is set for the
// NPC's own timer: no name, or the NPC's own name for an NPC without
// duplicates. Another NPC's name gives its code in target, and the
// command reaches it over events. A duplicate runs its source's script,
// where its own name is the source's, so that is another NPC too.
func (b *body) ownTimer(what string, name Expr) (target string, own bool, err error) {
	n := b.n
	if name == nil {
		if n.isFunction {
			return "", false, unsupported(what, "in a function, where the timer is the calling NPC's")
		}
		return "", true, nil
	}
	if s, ok := name.(*Str); ok && s.Value == n.line.Name && !n.c.duplicated[n.line.Name] && !n.isFunction {
		return "", true, nil
	}
	target, _, err = b.npcRef(what, name)
	return target, false, err
}

// timerFlag reads a timer command carries the player: the
// attach flag of initnpctimer and startnpctimer, and the detach flag of
// stopnpctimer. rAthena's other commands take the player of the label
// they run in.
func (b *body) timerFlag(what string, flag Expr) (bool, error) {
	if flag == nil {
		return false, nil
	}
	v, err := b.expr(flag)
	if err != nil {
		return false, err
	}
	if v.static == nil {
		return false, unsupported(what, "a computed attach flag")
	}
	return *v.static != 0, nil
}

// timerCommand writes initnpctimer, startnpctimer, stopnpctimer and
// setnpctimer.
func (b *body) timerCommand(name string, args []Expr) error {
	var npcName, flag, tick Expr
	if name == "setnpctimer" {
		if len(args) < 1 || len(args) > 2 {
			return unsupported(name, "arguments")
		}
		tick = args[0]
		if len(args) == 2 {
			npcName = args[1]
		}
	} else {
		// One argument is the NPC's name when it is a string, and the flag
		// otherwise. Two are the name and the flag.
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
	}
	target, own, err := b.ownTimer(name, npcName)
	if err != nil {
		return err
	}
	attach, err := b.timerFlag(name, flag)
	if err != nil {
		return err
	}
	method := map[string]string{"initnpctimer": "init", "startnpctimer": "start", "stopnpctimer": "stop"}[name]
	if own {
		b.n.timer = true
		if name == "setnpctimer" {
			v, err := b.expr(tick)
			if err != nil {
				return err
			}
			b.line("%s:of(player):set(%s)", timerVar, v.asInt())
			return nil
		}
		// An attach without a player does nothing, as script_rid2sd.
		if attach && name != "stopnpctimer" {
			b.line("%s:of(player, true):%s()", timerVar, method)
		} else {
			b.line("%s:of(player):%s()", timerVar, method)
		}
		return nil
	}
	// Another NPC's timer: the command is an event to the NPC, which
	// lists the hidden labels. A timer attached to the player gets the
	// player, so the NPC must be on the player's map.
	label := map[string]string{"initnpctimer": timerLabelInit, "startnpctimer": timerLabelStart, "stopnpctimer": timerLabelStop}[name]
	if name == "setnpctimer" {
		v, err := b.expr(tick)
		if err != nil {
			return err
		}
		if v.static == nil || *v.static != 0 {
			return unsupported(name, "a tick other than 0 on another NPC's timer")
		}
		label = timerLabelSet0
	}
	if !attach {
		b.line("R.fire(%s, %q)", target, label)
		return nil
	}
	if err := b.sameMapNPC(name, npcName); err != nil {
		return err
	}
	b.line("R.doevent(%s, %q, player)", target, label)
	return nil
}

// sameMapNPC checks that an NPC named by a string is on the map of the
// NPC whose script runs, as doevent needs for its player.
func (b *body) sameMapNPC(what string, e Expr) error {
	n := b.n
	if n.isFunction {
		return unsupported(what, "a timer attached to the player, in a function, which may run on a map other than the NPC's")
	}
	s, ok := e.(*Str)
	if !ok {
		return unsupported(what, "a timer attached to the player, of an NPC named by a computed name")
	}
	m, known := n.c.npcMap[s.Value]
	switch {
	case !known:
		return unsupported(what, "the NPC %s is not in the extract", s.Value)
	case m == "" || m != n.line.Map:
		return unsupported(what, "a timer attached to the player, of an NPC on another map than the player's, or without a map")
	}
	return nil
}

// playerTimer converts addtimer and deltimer (script.cpp 12050, pc.cpp
// pc_addeventtimer): the timer belongs to the attached player and runs
// the label "NPC::OnX" for the player (review decision 60, C2).
func (b *body) playerTimer(name string, args []Expr) error {
	want := 2
	if name == "deltimer" {
		want = 1
	}
	if len(args) != want {
		return unsupported(name, "arguments")
	}
	if s, ok := args[want-1].(*Str); ok && !strings.Contains(s.Value, "::") {
		// check_event only warns, and the event finds no NPC.
		b.line("-- %s without \"::\" finds no event, as in rAthena.", name)
		return nil
	}
	target, label, computed, err := b.eventLabel(name, args[want-1])
	if err != nil {
		return err
	}
	var event string
	switch s, isStr := args[want-1].(*Str); {
	case computed:
		event = target
	case target == "":
		return unsupported(name, "an event label without an NPC name")
	case isStr:
		i := strings.LastIndex(s.Value, "::")
		event = luaString(b.n.c.recordFor(s.Value[:i]) + "::" + label)
	default:
		event = target + " .. " + luaString("::"+label)
	}
	if name == "deltimer" {
		b.line("player:remove_timer(%s)", event)
		return nil
	}
	ms, err := b.expr(args[0])
	if err != nil {
		return err
	}
	b.line("R.add_timer(player, %s, %s)", ms.asInt(), event)
	return nil
}

// timerHandlers writes the hidden npc_event handlers of an NPC with
// timers.
func (n *npcConv) timerHandlers() {
	f := n.file
	f.need("events")
	handler := func(label, body string) {
		f.line(`events.on("npc_event", function(event) %s end, { name = %q })`, body, label)
	}
	handler(timerLabelInit, timerVar+":of(event.player, event.player ~= nil):init()")
	handler(timerLabelStart, timerVar+":of(event.player, event.player ~= nil):start()")
	handler(timerLabelStop, timerVar+":of(event.player):stop()")
	handler(timerLabelSet0, timerVar+":of(event.player):set(0)")
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
	if _, own, err := b.ownTimer(what, npcName); err != nil {
		return val{}, err
	} else if !own {
		return val{}, unsupported(what, "the timer of another NPC")
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
		return val{code: timerVar + ":of(player):tick()", t: tInt}, nil
	case 1:
		return val{code: timerVar + ":of(player):due()", t: tInt}, nil
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
		parts[i] = fmt.Sprintf("{ %d, %s, %q }", l.tick, n.eventFns[l.label], l.label)
	}
	return fmt.Sprintf("%s = R.npc_timer({ %s }, npc)", timerVar, strings.Join(parts, ", "))
}
