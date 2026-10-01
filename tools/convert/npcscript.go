package convert

import (
	"fmt"
	"strings"
)

// Whole NPC scripts and functions.

// segments splits a script's top level at its event labels: the part
// before the first is what a click runs, and each label starting with
// On, such as OnInit, starts the part its event runs. Other labels stay
// in the part they are in.
func segments(stmts []Stmt) (click []Stmt, events map[string][]Stmt, order []string) {
	events = map[string][]Stmt{}
	cur := ""
	for _, s := range stmts {
		if l, ok := s.(*Label); ok && strings.HasPrefix(l.Name, "On") {
			cur = strings.ToLower(l.Name)
			if _, seen := events[cur]; !seen {
				order = append(order, cur)
				events[cur] = []Stmt{}
			}
			continue
		}
		if cur == "" {
			click = append(click, s)
		} else {
			events[cur] = append(events[cur], s)
		}
	}
	return click, events, order
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
// "" for a script that does nothing.
func (n *npcConv) npcScript() (string, error) {
	stmts, err := ParseNPC(n.line.Script)
	if err != nil {
		return "", unsupported("parse", "%v", err)
	}
	click, events, order := segments(stmts.Stmts)
	n.scanArrays(stmts.Stmts)
	for _, e := range order {
		if e != "oninit" {
			return "", unsupported("event "+e, "")
		}
	}
	setup, err := n.writeBody(events["oninit"], 2, true, false)
	if err != nil {
		return "", err
	}
	var handler []string
	if !emptyScript(click) {
		if handler, err = n.writeBody(click, 3, true, false); err != nil {
			return "", err
		}
	}
	if len(handler) == 0 && len(events["oninit"]) == 0 {
		return "", nil
	}
	f := n.file
	f.need("context")
	f.line("return function()")
	f.indent++
	f.line("local npc = context:get_instance()")
	var npcVars []string
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
		npcVars = append(npcVars, fmt.Sprintf("local %s = %s", v.local, init))
	}
	for _, l := range npcVars {
		f.line("%s", l)
	}
	f.indent = 0
	for _, l := range setup {
		f.line("%s", l)
	}
	f.indent = 1
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
	f.indent = 0
	f.line("end")
	return n.finish(n.header()), nil
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
