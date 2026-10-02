package luaapi

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/server/constants"
)

// LuaLS returns the Lua Language Server definitions: one ---@meta file
// per module under wa/, named as require finds it, and wa/types.lua with
// the handle types and named types. Paths use forward slashes. consts
// fills the constant modules. Without it they have no fields.
func LuaLS(consts *constants.Data) map[string]string {
	out := map[string]string{}
	var tables map[string]*constants.Module
	if consts != nil {
		tables = consts.Modules()
	}
	for _, m := range Modules {
		out["wa/"+strings.TrimPrefix(m.Name, "wa.")+".lua"] = luaModule(m, tables[strings.TrimPrefix(m.Name, "wa.")])
	}
	out["wa/types.lua"] = luaTypes()
	return out
}

const generatedNote = "-- Generated from server/modscript/luaapi. Do not edit: wactl mod types writes it again.\n"

// comment writes text as --- lines.
func comment(b *strings.Builder, text string) {
	if text == "" {
		return
	}
	for line := range strings.SplitSeq(text, "\n") {
		b.WriteString("---" + line + "\n")
	}
}

// funcDoc is a function's text, with its permission and status.
func funcDoc(f Func) string {
	doc := f.Doc
	if f.Permission != "" {
		doc += "\n\nNeeds the permission `" + f.Permission + "`."
	}
	if f.Provisional {
		doc += "\n\nProvisional: not reviewed in proposal 0002 yet."
	}
	return doc
}

// param writes a ---@param line. A type that ends in ? makes the
// parameter optional.
func param(b *strings.Builder, p Param) {
	name, typ := p.Name, p.Type
	if strings.HasSuffix(typ, "?") && name != "..." {
		name += "?"
		typ = strings.TrimSuffix(typ, "?")
	}
	fmt.Fprintf(b, "---@param %s %s", name, typ)
	if p.Doc != "" {
		b.WriteString(" " + p.Doc)
	}
	b.WriteString("\n")
}

// function writes one function or method with its annotations.
func function(b *strings.Builder, owner, sep string, f Func) {
	comment(b, funcDoc(f))
	var names []string
	for _, p := range f.Params {
		param(b, p)
		names = append(names, p.Name)
	}
	if f.Returns != "" {
		fmt.Fprintf(b, "---@return %s\n", f.Returns)
	}
	for _, o := range f.Overloads {
		fmt.Fprintf(b, "---@overload %s\n", o)
	}
	fmt.Fprintf(b, "function %s%s%s(%s) end\n\n", owner, sep, f.Name, strings.Join(names, ", "))
}

// signature is a function's type, as fun(...): ...
func signature(params []Param, returns string) string {
	var ps []string
	for _, p := range params {
		name, typ := p.Name, p.Type
		if strings.HasSuffix(typ, "?") && name != "..." {
			name += "?"
			typ = strings.TrimSuffix(typ, "?")
		}
		ps = append(ps, name+": "+typ)
	}
	s := "fun(" + strings.Join(ps, ", ") + ")"
	if returns != "" {
		s += ": " + returns
	}
	return s
}

func luaModule(m Module, consts *constants.Module) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---@meta %s\n%s\n", m.Name, generatedNote)
	doc := m.Doc
	if m.Testing {
		doc += "\n\nIt exists only in tests that wactl mod test runs."
	}
	comment(&b, doc)
	fmt.Fprintf(&b, "---@class %s\n", m.Name)
	if m.Call != nil {
		fmt.Fprintf(&b, "---@overload %s\n", signature(m.Call.Params, m.Call.Returns))
	}
	if consts != nil {
		for _, name := range slices.Sorted(maps.Keys(consts.Values)) {
			fmt.Fprintf(&b, "---@field %s integer %d, rAthena's %s.\n", name, consts.Values[name], consts.Origins[name])
		}
	}
	fmt.Fprintf(&b, "local %s = {}\n\n", m.Local)
	for _, f := range m.Funcs {
		if m.Name == "wa.events" && f.Name == "on" {
			f.Overloads = eventOverloads()
		}
		function(&b, m.Local, ".", f)
	}
	for _, f := range m.Methods {
		function(&b, m.Local, ":", f)
	}
	fmt.Fprintf(&b, "return %s\n", m.Local)
	return b.String()
}

// eventOverloads are the signatures of events.on, one per event.
func eventOverloads() []string {
	var out []string
	for _, e := range Events {
		out = append(out, fmt.Sprintf("fun(name: %q, handler: %s): wa.EventHandle", e.Name, signature(e.Params, "")))
	}
	return out
}

func luaTypes() string {
	var b strings.Builder
	b.WriteString("---@meta _\n" + generatedNote + "\n")
	for _, a := range Aliases() {
		comment(&b, a.Doc)
		if a.Type != "" {
			fmt.Fprintf(&b, "---@alias %s %s\n\n", a.Name, a.Type)
			continue
		}
		fmt.Fprintf(&b, "---@alias %s\n", a.Name)
		for _, v := range a.Values {
			fmt.Fprintf(&b, "---| %q\n", v)
		}
		b.WriteString("\n")
	}
	for _, c := range Classes {
		doc := c.Doc
		if c.Testing {
			doc += "\n\nIt exists only in tests that wactl mod test runs."
		}
		if c.Provisional {
			doc += "\n\nProvisional: not reviewed in proposal 0002 yet."
		}
		comment(&b, doc)
		fmt.Fprintf(&b, "---@class %s\n", c.Name)
		for _, f := range c.Fields {
			fmt.Fprintf(&b, "---@field %s %s", f.Name, f.Type)
			if f.Doc != "" {
				b.WriteString(" " + f.Doc)
			}
			b.WriteString("\n")
		}
		local := localName(c.Name)
		fmt.Fprintf(&b, "local %s = {}\n\n", local)
		for _, f := range c.Methods {
			function(&b, local, ":", f)
		}
		for _, f := range c.Functions {
			function(&b, local, ".", f)
		}
	}
	return b.String()
}

// localName is the local that holds a class's methods in types.lua.
func localName(class string) string {
	return strings.ReplaceAll(class, ".", "_")
}
