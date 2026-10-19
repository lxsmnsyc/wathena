package convert

import (
	"fmt"
	"regexp"
	"strings"
)

// Whole NPC scripts and functions.

// segments splits a script's top level at its event labels: the part
// before the first is what a click runs, and each label starting with
// On, such as OnInit, starts the part its event runs. Other labels stay
// in the part they are in. As in rAthena, an event's part runs on into
// the next label's part unless it ends with end, close, return or goto.
// The labels it runs into stay in it as labels.
func segments(stmts []Stmt) (click []Stmt, events map[string][]Stmt, order []string) {
	type part struct {
		label *Label
		stmts []Stmt
	}
	var parts []*part
	for _, s := range stmts {
		if l, ok := s.(*Label); ok && strings.HasPrefix(l.Name, "On") {
			parts = append(parts, &part{label: l})
			continue
		}
		if len(parts) == 0 {
			click = append(click, s)
		} else {
			p := parts[len(parts)-1]
			p.stmts = append(p.stmts, s)
		}
	}
	events = map[string][]Stmt{}
	for i, p := range parts {
		name := strings.ToLower(p.label.Name)
		if _, seen := events[name]; seen {
			continue
		}
		body := append([]Stmt{}, p.stmts...)
		for j := i + 1; j < len(parts) && !terminates(body); j++ {
			body = append(body, parts[j].label)
			body = append(body, parts[j].stmts...)
		}
		order = append(order, name)
		events[name] = body
	}
	return click, events, order
}

// terminates reports whether statements end with a jump that leaves
// them: end, close, return or goto.
func terminates(stmts []Stmt) bool {
	if len(stmts) == 0 {
		return false
	}
	switch s := stmts[len(stmts)-1].(type) {
	case *Jump:
		return s.Kind == "end" || s.Kind == "close"
	case *Return, *Goto:
		return true
	}
	return false
}

// clockSchedule returns the schedule of a clock label, as a format for
// the handler, such as schedules.daily("00:00", %s) for OnClock0000. ok
// is false for a label that is not a clock label. err is set for a
// clock label that does not convert. rAthena runs these labels in
// npc_event_do_clock:
//   - OnClockHHMM every day at HH:MM.
//   - OnHourHH when the hour changes to HH.
//   - OnMinuteMM every hour at minute MM.
//   - OnSunHHMM to OnSatHHMM on that day at HH:MM.
//   - OnDayMMDD once a year, which the API has no schedule for.
func clockSchedule(label string) (format string, ok bool, err error) {
	kinds := []struct{ prefix, day string }{
		{"onclock", ""}, {"onhour", ""}, {"onminute", ""}, {"onday", ""},
		{"onsun", "sunday"}, {"onmon", "monday"}, {"ontue", "tuesday"}, {"onwed", "wednesday"},
		{"onthu", "thursday"}, {"onfri", "friday"}, {"onsat", "saturday"},
	}
	name := strings.ToLower(label)
	for _, k := range kinds {
		digits, found := strings.CutPrefix(name, k.prefix)
		if !found || digits == "" || strings.Trim(digits, "0123456789") != "" {
			continue
		}
		bad := unsupported("event "+name, "%s is not a valid time", digits)
		num := func(s string) int {
			n := 0
			for _, c := range s {
				n = n*10 + int(c-'0')
			}
			return n
		}
		switch k.prefix {
		case "onday":
			return "", true, unsupported("event "+name, "a yearly schedule (OnDay) is not in the API")
		case "onhour", "onminute":
			if len(digits) != 2 {
				return "", true, bad
			}
			n := num(digits)
			if k.prefix == "onhour" {
				if n > 23 {
					return "", true, bad
				}
				return fmt.Sprintf(`schedules.daily("%02d:00", %%s)`, n), true, nil
			}
			if n > 59 {
				return "", true, bad
			}
			return fmt.Sprintf("schedules.hourly(%d, %%s)", n), true, nil
		}
		if len(digits) != 4 || num(digits[:2]) > 23 || num(digits[2:]) > 59 {
			return "", true, bad
		}
		at := digits[:2] + ":" + digits[2:]
		if k.day == "" {
			return fmt.Sprintf(`schedules.daily("%s", %%s)`, at), true, nil
		}
		return fmt.Sprintf(`schedules.weekly("%s", "%s", %%s)`, k.day, at), true, nil
	}
	return "", false, nil
}

// emptyScript reports whether statements do nothing: only end.
func emptyScript(stmts []Stmt) bool {
	for _, s := range stmts {
		if j, ok := s.(*Jump); !ok || j.Kind != "end" && j.Kind != "close" {
			return false
		}
	}
	return true
}

// writeBody converts statements into a Lua function body, with the
// local functions of the labels it calls with callsub.
func (n *npcConv) writeBody(stmts []Stmt, indent int, main, hasArgs bool) ([]string, error) {
	b := n.newBody(indent)
	b.main, b.hasArgs = main, hasArgs
	if hasArgs {
		b.line("local args = table.pack(...)")
	}
	if err := b.block(stmts); err != nil {
		return nil, err
	}
	if err := checkGotos(b); err != nil {
		return nil, err
	}
	// The labels callsub calls become local functions, each running from
	// its label to the end of the top level. They may call each other.
	done := map[string]bool{}
	var subLines []string
	var names []string
	for {
		var next []string
		for _, l := range sortedKeys(b.subs) {
			if !done[l] {
				next = append(next, l)
			}
		}
		if len(next) == 0 {
			break
		}
		for _, label := range next {
			done[label] = true
			start := -1
			for i, s := range stmts {
				if l, ok := s.(*Label); ok && strings.EqualFold(l.Name, label) {
					start = i
				}
			}
			if start < 0 {
				return nil, unsupported("callsub", "%s is not a label of this part of the script", label)
			}
			sb := n.newBody(indent + 1)
			sb.hasArgs = true
			sb.subs = b.subs
			sb.line("local args = table.pack(...)")
			if err := sb.block(stmts[start:]); err != nil {
				return nil, err
			}
			if err := checkGotos(sb); err != nil {
				return nil, err
			}
			fn := b.subs[label]
			names = append(names, fn)
			lines, ok := sb.text(nil)
			if !ok {
				return nil, unsupported("script", "more than 150 variables in one function")
			}
			subLines = append(subLines, strings.Repeat("  ", indent)+fn+" = function(player, conversation, npc, ...)")
			subLines = append(subLines, lines...)
			subLines = append(subLines, strings.Repeat("  ", indent)+"end")
		}
	}
	if len(names) > 0 {
		subLines = append([]string{strings.Repeat("  ", indent) + "local " + strings.Join(names, ", ")}, subLines...)
	}
	lines, ok := b.text(subLines)
	if !ok {
		return nil, unsupported("script", "more than 150 variables in one function")
	}
	return lines, nil
}

// checkGotos checks that every goto finds its label in the same Lua
// function.
func checkGotos(b *body) error {
	for _, g := range b.gotos {
		if !b.labels[g] {
			return unsupported("goto", "%s is in another part of the script", g)
		}
	}
	return nil
}

// npcScript converts an NPC's script into its entry script. It returns
// "" for a script that does nothing. Its setup function does this:
//   - It makes a local function for each event label that a schedule
//     or a monster's death runs. The function runs with the player that
//     killed the monster, or with no player.
//   - It adds the schedules of clock labels, the touch handler of
//     OnTouch or OnTouch_, and the click handler.
//   - It runs OnInit last, so an end there does not skip the rest.
func (n *npcConv) npcScript() (string, error) {
	stmts, err := ParseNPC(n.line.Script)
	if err != nil {
		return "", unsupported("parse", "%v", err)
	}
	click, events, order := segments(stmts.Stmts)
	n.scanArrays(stmts.Stmts)
	for _, e := range order {
		n.eventLabels[e] = true
	}
	mobEvents := n.mobEvents(stmts.Stmts)
	schedules := map[string]string{}
	// OnTouch_ wins over OnTouch: rAthena runs OnTouch only for an NPC
	// without OnTouch_ (npc_touch_areanpc).
	touch := ""
	for _, label := range []string{"ontouch_", "ontouch"} {
		if _, ok := events[label]; ok && touch == "" {
			touch = label
		}
	}
	for _, e := range order {
		if e == "oninit" || e == "ontouch" && touch == "ontouch_" {
			continue
		}
		if e == touch {
			n.eventFn(e)
			continue
		}
		format, ok, err := clockSchedule(e)
		switch {
		case ok && err == nil:
			schedules[e] = format
			n.eventFn(e)
		case mobEvents[e]:
			// The function comes when the monster command converts.
		case err != nil:
			return "", err
		default:
			return "", unsupported("event "+e, "")
		}
	}
	var setup []string
	if len(events["oninit"]) > 0 {
		if setup, err = n.writeBody(events["oninit"], 2, true, false); err != nil {
			return "", err
		}
	}
	var handler []string
	if !emptyScript(click) {
		if handler, err = n.writeBody(click, 3, true, false); err != nil {
			return "", err
		}
	}
	bodies := map[string][]string{}
	touchDialog := false
	// A body may name more labels, with a monster command.
	for i := 0; i < len(n.eventOrder); i++ {
		e := n.eventOrder[i]
		if e == touch {
			n.bodyCalls = map[string]bool{}
		}
		bodies[e], err = n.writeBody(events[e], 3, true, false)
		if e == touch {
			calls := n.bodyCalls
			n.bodyCalls = nil
			if err == nil {
				touchDialog = n.touchHasDialog(bodies[e], calls)
			}
		}
		if err != nil {
			return "", err
		}
		n.ok("event " + e)
	}
	n.touch, n.touchDialog = touch != "", touchDialog
	if len(handler) == 0 && len(setup) == 0 && len(n.eventOrder) == 0 {
		return "", nil
	}
	f := n.file
	f.need("context")
	f.line("return function()")
	f.indent++
	f.line("local npc = context:get_instance()")
	for _, k := range n.varOrder {
		v := n.vars[k]
		if v.scope != "npc" {
			continue
		}
		init := "0"
		switch {
		case v.array:
			init = "{}"
		case v.str:
			init = `""`
		}
		f.line("local %s = %s", v.local, init)
	}
	if len(n.eventOrder) > 0 {
		names := make([]string, len(n.eventOrder))
		for i, e := range n.eventOrder {
			names[i] = n.eventFns[e]
		}
		f.line("local %s", strings.Join(names, ", "))
		for _, e := range n.eventOrder {
			f.line("%s = function(player, conversation)", n.eventFns[e])
			f.line("  R.run(function()")
			f.indent = 0
			for _, l := range bodies[e] {
				f.line("%s", l)
			}
			f.indent = 1
			f.line("  end)")
			f.line("end")
		}
	}
	for _, e := range order {
		if format, ok := schedules[e]; ok {
			f.need("schedules")
			f.line(format, n.eventFns[e])
		}
	}
	switch {
	case touch == "ontouch_" && touchDialog:
		f.line("R.touch_one(%s, true)", n.eventFns[touch])
	case touch == "ontouch_":
		f.line("R.touch_one(%s)", n.eventFns[touch])
	case touch == "ontouch":
		f.need("events")
		f.line(`events.on("touch", %s)`, n.eventFns[touch])
	}
	if len(handler) > 0 {
		f.need("events")
		f.line(`events.on("click", function(player, conversation)`)
		f.line("  R.run(function()")
		f.indent = 0
		for _, l := range handler {
			f.line("%s", l)
		}
		f.indent = 1
		f.line("  end)")
		f.line("end)")
	}
	if len(setup) > 0 {
		f.line("R.run(function()")
		f.indent = 0
		for _, l := range setup {
			f.line("%s", l)
		}
		f.indent = 1
		f.line("end)")
	}
	f.indent = 0
	f.line("end")
	return n.finish(n.header()), nil
}

// dialogCode matches converted code that needs the conversation: a
// dialog call, or a helper that shows lines. A call to a converted
// function passes the conversation on after the player, which does not
// match.
var dialogCode = regexp.MustCompile(`conversation:|\(conversation[,)]`)

// touchHasDialog reports whether an OnTouch or OnTouch_ part shows a
// dialog, or calls a function that does. The touch event has a
// conversation (review decision 56), so such a part converts. Only
// R.touch_one needs to know: it cannot show the dialog to the next
// player in the area when the first one leaves (rathena.lua). calls
// are the functions the part calls itself.
func (n *npcConv) touchHasDialog(lines []string, calls map[string]bool) bool {
	for _, l := range lines {
		if dialogCode.MatchString(l) {
			return true
		}
	}
	direct := make([]string, 0, len(calls))
	for k := range calls {
		direct = append(direct, k)
	}
	for _, name := range n.c.functionsOf(direct) {
		if f := n.c.converted[name]; f != nil && dialogCode.MatchString(f.Lua) {
			return true
		}
	}
	return false
}

// eventFn returns the local function of an event label, and adds it.
func (n *npcConv) eventFn(label string) string {
	if fn, ok := n.eventFns[label]; ok {
		return fn
	}
	fn := "event_" + luaName(label)
	n.names[fn] = true
	n.eventFns[label] = fn
	n.eventOrder = append(n.eventOrder, label)
	return fn
}

// finish writes the file: the header, the requires, the shared helpers,
// the records, the variables, and the body.
func (n *npcConv) finish(header []string) string {
	f := n.file
	var decls []string
	for _, k := range n.varOrder {
		if v := n.vars[k]; v.scope != "npc" {
			decls = append(decls, v.declaration())
		}
	}
	if len(decls) > 0 {
		f.need("variables")
	}
	f.records = append(append([]string{`local R = require "core.lib.rathena"`}, f.records...), decls...)
	return f.String(header)
}

// header is the comment at the top of an NPC's file: where it came from,
// and its rAthena script.
func (n *npcConv) header() []string {
	l := n.line
	what := l.Display
	if l.Kind == "function" {
		what = l.Name
	} else if l.Name != l.Display {
		what = fmt.Sprintf("%s (%s)", l.Display, l.Name)
	}
	out := []string{fmt.Sprintf("%s: %s, line %d. Generated by wactl convert npcs; do not edit.", what, l.File, l.Line), "rAthena script:"}
	for _, line := range strings.Split(strings.TrimSpace(l.Script), "\n") {
		out = append(out, "  "+strings.TrimRight(line, " \t\r"))
	}
	return out
}

// function converts an rAthena function once, on first call. A function
// that does not parse cannot be called; parts that do not convert fail
// when a script reaches them.
func (c *npcConverter) function(key string) (*Function, error) {
	if f, ok := c.converted[key]; ok {
		if f.Err != "" {
			return nil, unsupported("callfunc "+f.Name, "%s", f.Err)
		}
		return f, nil
	}
	l := c.functions[key]
	if l == nil {
		return nil, unsupported("callfunc "+key, "the function is not in the extract")
	}
	f := &Function{Name: l.Name, File: l.File, Line: l.Line, Features: map[string]bool{}}
	c.converted[key] = f
	f.Path = ScriptPathIn("scripts/functions/", l.Name, c.paths)
	n := c.newNPCConv(l)
	n.isFunction, n.allowStubs = true, true
	stmts, err := ParseNPC(l.Script)
	if err != nil {
		f.Err = "it does not parse: " + err.Error()
		f.Path = ""
		return nil, unsupported("callfunc "+l.Name, "%s", f.Err)
	}
	n.scanArrays(stmts.Stmts)
	lines, err := n.writeBody(stmts.Stmts, 1, false, true)
	if err != nil {
		f.Err = err.Error()
		f.Path = ""
		return nil, unsupported("callfunc "+l.Name, "%s", f.Err)
	}
	n.file.line("return function(player, conversation, npc, ...)")
	n.file.indent = 0
	for _, x := range lines {
		n.file.line("%s", x)
	}
	n.file.line("end")
	f.Lua = n.finish(n.header())
	f.Stubs, f.Features = n.stubs, n.features
	for k := range n.calls {
		f.Calls = append(f.Calls, k)
	}
	sortStrings(f.Calls)
	return f, nil
}
