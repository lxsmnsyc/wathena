package convert

import (
	"fmt"
	"regexp"
	"slices"
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
// them: end, close, return or goto. A local function's body after it
// does not count, as rAthena jumps over it. With returns, it also counts
// these, whose every way out leaves:
//   - callsub of a label whose part never returns (returns says which);
//   - if with an else whose branches both leave;
//   - a block that ends so;
//   - a switch whose cases cover every value and each leave, by the
//     end of its statements or through the next case. A switch on
//     select of fixed options gets 1 to their count: a player who
//     cancels the menu ends the script.
func terminates(stmts []Stmt) bool { return endsFlow(stmts, nil) }

func endsFlow(stmts []Stmt, returns func(label string) bool) bool {
	for len(stmts) > 0 {
		if _, ok := stmts[len(stmts)-1].(*FuncDecl); !ok {
			break
		}
		stmts = stmts[:len(stmts)-1]
	}
	if len(stmts) == 0 {
		return false
	}
	switch s := stmts[len(stmts)-1].(type) {
	case *Jump:
		return s.Kind == "end" || s.Kind == "close"
	case *Return, *Goto:
		return true
	}
	if returns == nil {
		return false
	}
	return stmtEnds(stmts[len(stmts)-1], returns)
}

// stmtEnds reports whether every way out of a statement leaves, as
// endsFlow says.
func stmtEnds(s Stmt, returns func(string) bool) bool {
	switch s := s.(type) {
	case *Jump:
		return s.Kind == "end" || s.Kind == "close"
	case *Return, *Goto:
		return true
	case *Block:
		return endsFlow(s.Stmts, returns)
	case *If:
		return s.Else != nil && stmtEnds(s.Then, returns) && stmtEnds(s.Else, returns)
	case *Command:
		switch strings.ToLower(s.Name) {
		case "close3":
			return true
		case "callsub":
			if len(s.Args) > 0 {
				if id, ok := s.Args[0].(*Ident); ok {
					return !returns(strings.ToLower(id.Name))
				}
			}
		}
	case *Switch:
		return switchEnds(s, returns)
	}
	return false
}

// switchEnds reports whether every case of a switch covers the values
// and leaves.
func switchEnds(s *Switch, returns func(string) bool) bool {
	covered := map[int64]bool{}
	def := false
	for _, cs := range s.Cases {
		def = def || cs.Default
		for _, v := range cs.Values {
			n, ok := v.(*Num)
			if !ok {
				return false
			}
			covered[n.Value] = true
		}
	}
	if !def {
		count := selectOptions(s.Value)
		if count <= 0 {
			return false
		}
		for i := int64(1); i <= int64(count); i++ {
			if !covered[i] {
				return false
			}
		}
	}
	// A case leaves when its statements do, or when it has no break and
	// runs on into a case that leaves.
	next := false
	for i := len(s.Cases) - 1; i >= 0; i-- {
		body := s.Cases[i].Body
		switch {
		case breaks(body):
			next = false
		case endsFlow(body, returns):
			next = true
		case i == len(s.Cases)-1:
			next = false
		}
		if !next {
			return false
		}
	}
	return true
}

// selectOptions is the number of options of select("A:B", "C") with
// fixed text, or 0.
func selectOptions(e Expr) int {
	c, ok := e.(*Call)
	if !ok || !strings.EqualFold(c.Name, "select") {
		return 0
	}
	n := 0
	for _, a := range c.Args {
		s, ok := a.(*Str)
		if !ok {
			return 0
		}
		n += strings.Count(s.Value, ":") + 1
	}
	return n
}

// breaks reports whether statements hold a break or a continue that
// leaves the switch they are in: one outside the loops and switches in
// them.
func breaks(stmts []Stmt) bool {
	found := false
	var walk func(list []Stmt)
	walk = func(list []Stmt) {
		for _, s := range list {
			switch s := s.(type) {
			case *Jump:
				if s.Kind == "break" || s.Kind == "continue" {
					found = true
				}
			case *Block:
				walk(s.Stmts)
			case *If:
				walk([]Stmt{s.Then})
				if s.Else != nil {
					walk([]Stmt{s.Else})
				}
			}
		}
	}
	walk(stmts)
	return found
}

// subReturns reports whether the part that callsub runs from a label can
// return to its caller: its reachable statements hold a return. A part
// without one ends the script, as end does or as the script's end does.
// A label that is not found, or a part that is being checked, may
// return.
func (n *npcConv) subReturns(label string) bool {
	if r, ok := n.returns[label]; ok {
		return r
	}
	n.returns[label] = true
	start := -1
	for i, s := range n.all {
		if l, ok := s.(*Label); ok && strings.EqualFold(l.Name, label) {
			start = i
		}
	}
	if start < 0 {
		return true
	}
	r := false
	walkStmts(n.reachable(n.all[start:]), func(s Stmt) {
		if _, ok := s.(*Return); ok {
			r = true
		}
	})
	n.returns[label] = r
	return r
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
// local functions of the labels it calls with callsub and of the local
// functions it calls. Statements it cannot reach are left out
// (reachable).
func (n *npcConv) writeBody(stmts []Stmt, indent int, main, hasArgs bool) ([]string, error) {
	b := n.newBody(indent)
	b.main, b.hasArgs = main, hasArgs
	if hasArgs {
		b.line("local args = table.pack(...)")
	}
	if n.isFunction && hasArgs {
		// The body of a global function reads its reference arguments.
		b.refArgs = n.refArgs
	}
	if err := b.block(n.reachable(stmts)); err != nil {
		return nil, err
	}
	if err := checkGotos(b); err != nil {
		return nil, err
	}
	// The labels callsub calls become local functions, each running from
	// its label on, as rAthena's callsub runs the script from the label's
	// position. A local function (function Name { }) becomes one too.
	// rAthena calls both with a new scope for .@ variables
	// (buildin_callsub, script.cpp 5465). They may call each other.
	done := map[string]bool{}
	var subLines []string
	var names []string
	all := n.all
	if all == nil {
		all = stmts
	}
	for {
		var next []string
		for _, l := range sortedKeys(b.subs) {
			if !done["sub "+l] {
				next = append(next, "sub "+l)
			}
		}
		for _, f := range sortedKeys(b.funcs) {
			if !done["func "+f] {
				next = append(next, "func "+f)
			}
		}
		if len(next) == 0 {
			break
		}
		for _, key := range next {
			done[key] = true
			kind, name, _ := strings.Cut(key, " ")
			var part []Stmt
			fn := b.subs[name]
			if kind == "func" {
				fn = b.funcs[name]
				part = n.localFuncs[name].Body.Stmts
			} else {
				start := -1
				for i, s := range all {
					if l, ok := s.(*Label); ok && strings.EqualFold(l.Name, name) {
						start = i
					}
				}
				if start < 0 {
					return nil, unsupported("callsub", "%s is not a label of the script's top level", name)
				}
				part = all[start:]
			}
			sb := n.newBody(indent + 1)
			sb.hasArgs = true
			sb.subs, sb.funcs = b.subs, b.funcs
			sb.line("local args = table.pack(...)")
			if err := sb.block(n.reachable(part)); err != nil {
				return nil, err
			}
			if err := checkGotos(sb); err != nil {
				return nil, err
			}
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

// reachable leaves out the statements of the top level that the flow
// cannot reach from the first one. The top level splits at its labels.
// A part is reached when the part before it is reached and does not end
// with end, close, return or goto, or when a goto or a menu of a reached
// part names its label. A part that only callsub reaches, after an end,
// is left out, as rAthena never runs it there: its getarg would fail.
func (n *npcConv) reachable(stmts []Stmt) []Stmt {
	type part struct {
		stmts   []Stmt
		targets []string
	}
	var parts []*part
	index := map[string]int{}
	for _, s := range stmts {
		if l, ok := s.(*Label); ok || len(parts) == 0 {
			if ok {
				index[strings.ToLower(l.Name)] = len(parts)
			}
			parts = append(parts, &part{})
		}
		p := parts[len(parts)-1]
		p.stmts = append(p.stmts, s)
	}
	if len(parts) <= 1 {
		return stmts
	}
	for _, p := range parts {
		walkStmts(p.stmts, func(s Stmt) {
			switch s := s.(type) {
			case *Goto:
				p.targets = append(p.targets, strings.ToLower(s.Label))
			case *Command:
				if strings.EqualFold(s.Name, "menu") {
					for i := 1; i < len(s.Args); i += 2 {
						if id, ok := s.Args[i].(*Ident); ok && id.Name != "-" {
							p.targets = append(p.targets, strings.ToLower(id.Name))
						}
					}
				}
			}
		})
	}
	reach := make([]bool, len(parts))
	reach[0] = true
	for changed := true; changed; {
		changed = false
		mark := func(i int) {
			if !reach[i] {
				reach[i], changed = true, true
			}
		}
		for i, p := range parts {
			if !reach[i] {
				continue
			}
			if i+1 < len(parts) && !endsFlow(p.stmts, n.subReturns) {
				mark(i + 1)
			}
			for _, t := range p.targets {
				if j, ok := index[t]; ok {
					mark(j)
				}
			}
		}
	}
	var out []Stmt
	for i, p := range parts {
		if reach[i] {
			out = append(out, p.stmts...)
		}
	}
	return out
}

// checkGotos checks that every goto finds its label in the same Lua
// function. A label inside a block is seen only from that block and
// the blocks in it, as Lua's goto sees labels. rAthena allows a jump to
// any label, so a goto from outside the block does not convert.
func checkGotos(b *body) error {
	for _, g := range b.gotos {
		if b.labels[g.label] {
			continue
		}
		path, ok := b.deepLabels[g.label]
		if !ok {
			return unsupported("goto", "%s is in another part of the script", g.label)
		}
		if len(path) > len(g.path) || !slices.Equal(path, g.path[:len(path)]) {
			return unsupported("labels", "%s: a label inside a block, which a jump from outside the block reaches", g.label)
		}
	}
	return nil
}

// npcScript converts an NPC's script into its entry script. It returns
// "" for a script that does nothing. Its setup function does this:
//   - It makes a local function for each event label. The function runs
//     with the player of the event, such as the player that killed a
//     monster, or with no player.
//   - Each label is an npc_event handler, which npc:fire_event and
//     world.fire_npc_event run, as rAthena's ev_db lists every label
//     that starts with On (review decision 58). OnTouchNPC is the
//     monster_touch handler too.
//   - It adds the schedules of clock labels, the touch handler of
//     OnTouch or OnTouch_, and the click handler.
//   - It runs OnInit last, so an end there does not skip the rest.
func (n *npcConv) npcScript() (string, error) {
	stmts, err := ParseNPC(n.line.Script)
	if err != nil {
		return "", unsupported("parse", "%v", err)
	}
	click, events, order := segments(stmts.Stmts)
	n.prepare(stmts.Stmts)
	// The labels as the script writes them, for npc_event's name.
	labels := map[string]string{}
	for _, st := range stmts.Stmts {
		if l, ok := st.(*Label); ok && strings.HasPrefix(l.Name, "On") {
			if _, seen := labels[strings.ToLower(l.Name)]; !seen {
				labels[strings.ToLower(l.Name)] = l.Name
			}
		}
	}
	for _, e := range order {
		n.eventLabels[e] = true
	}
	n.timerLabels = timerLabels(stmts.Stmts)
	timed := map[string]bool{}
	for _, l := range n.timerLabels {
		timed[l.label] = true
	}
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
		if err := labelError(e); err != nil {
			return "", err
		}
		format, ok, err := clockSchedule(e)
		switch {
		case ok && err == nil:
			schedules[e] = format
		case err != nil:
			return "", err
		}
		// OnInit, the touch and timer labels, the labels of monsters'
		// deaths and every other label: each is an event function, which
		// npc_event runs too.
		n.eventFn(e)
	}
	if len(n.eventOrder) > 150 {
		return "", unsupported("script", "more than 150 event labels")
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
		playerEvent := playerLabels[e] != ""
		if e == touch || e == "oninit" || playerEvent {
			n.bodyCalls = map[string]bool{}
		}
		n.inEvent = e != touch
		bodies[e], err = n.writeBody(events[e], 3, true, false)
		n.inEvent = false
		if e == touch || e == "oninit" || playerEvent {
			calls := n.bodyCalls
			n.bodyCalls = nil
			if err == nil && e == touch {
				touchDialog = n.touchHasDialog(bodies[e], calls)
			}
			if err == nil && e == "oninit" && n.touchHasDialog(bodies[e], calls) {
				// OnInit runs at the setup, with no player: a wait or a
				// dialog there has no conversation.
				err = unsupported("event oninit", "it waits or shows a dialog, and OnInit runs without a player")
			}
			if err == nil && playerEvent && n.touchHasDialog(bodies[e], calls) {
				// The player events have a player and no conversation.
				err = unsupported("event "+e, "it waits or shows a dialog, and the player events have no conversation")
			}
		}
		if err != nil {
			return "", err
		}
		n.ok("event " + e)
	}
	n.touch, n.touchDialog = touch != "", touchDialog
	if len(handler) == 0 && len(n.eventOrder) == 0 {
		return "", nil
	}
	f := n.file
	f.need("context")
	f.line("return function()")
	f.indent++
	f.line("local npc = context:get_instance()")
	if n.sourceShared {
		// The duplicates and the source share the source's .x variables.
		f.need("world")
		f.line("local %s = world.get_npc(%s) or npc", sourceVar, luaString(n.c.recordFor(n.line.Name)))
	}
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
	timer := n.timer || len(n.timerLabels) > 0
	if timer {
		f.line("local %s", timerVar)
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
	if timer {
		f.line("%s", n.timerSetup())
	}
	if len(n.eventOrder) > 0 {
		f.need("events")
	}
	for _, e := range n.eventOrder {
		name := labels[e]
		if name == "" {
			name = e
		}
		f.line(`events.on("npc_event", function(event) %s(event.player, event.conversation) end, { name = %s })`, n.eventFns[e], luaString(name))
		if e == "ontouchnpc" {
			// OnTouchNPC runs with the monster attached in rAthena, which a
			// converted script cannot read.
			f.line(`events.on("monster_touch", function() %s() end)`, n.eventFns[e])
		}
		// The labels of one player's events run with the player, for the
		// players of every map (npc_script_event, review decision 59).
		switch ev := playerLabels[e]; ev {
		case "":
		case "death":
			// OnPCDieEvent: a player's death (status.cpp 1786).
			f.line(`events.on("death", function(death) %s(death.victim, nil) end, { kind = "player" })`, n.eventFns[e])
		case "map_enter":
			// OnPCLoadMapEvent runs only on maps with the loadevent flag
			// (clif.cpp 11148).
			f.need("maps")
			f.line(`events.on("map_enter", function(player) if maps.get_flag(player.map, "loadevent") then %s(player, nil) end end)`, n.eventFns[e])
		default:
			f.line(`events.on(%q, function(player) %s(player, nil) end)`, ev, n.eventFns[e])
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
	if fn, ok := n.eventFns["oninit"]; ok {
		f.line("%s()", fn)
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
	n.refArgs = c.refArgsOf(key)
	stmts, err := ParseNPC(l.Script)
	if err != nil {
		f.Err = "it does not parse: " + err.Error()
		f.Path = ""
		return nil, unsupported("callfunc "+l.Name, "%s", f.Err)
	}
	n.prepare(stmts.Stmts)
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
