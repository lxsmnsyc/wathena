package luaapi

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/server/constants"
)

// cell escapes text for a Markdown table cell.
func cell(s string) string {
	s = strings.ReplaceAll(s, "|", `\|`)
	return strings.ReplaceAll(s, "\n", " ")
}

func code(s string) string { return "`" + s + "`" }

// call is how a function is written in the reference, such as
// events.on(name, handler).
func call(owner, sep string, f Func) string {
	var names []string
	for _, p := range f.Params {
		n := p.Name
		if strings.HasSuffix(p.Type, "?") && n != "..." {
			n += "?"
		}
		names = append(names, n)
	}
	s := owner + sep + f.Name + "(" + strings.Join(names, ", ") + ")"
	if f.Returns != "" {
		s += " → " + f.Returns
	}
	return s
}

// funcSection writes one function: its call, text and parameters.
func funcSection(b *strings.Builder, owner, sep string, f Func) {
	fmt.Fprintf(b, "#### %s\n\n", code(call(owner, sep, f)))
	if f.Doc != "" {
		b.WriteString(f.Doc + "\n\n")
	}
	if f.Permission != "" {
		fmt.Fprintf(b, "Permission: %s.\n\n", code(f.Permission))
	}
	if f.Provisional {
		b.WriteString("Provisional: not reviewed in proposal 0002 yet.\n\n")
	}
	if len(f.Params) > 0 {
		b.WriteString("| Parameter | Type | Meaning |\n|---|---|---|\n")
		for _, p := range f.Params {
			fmt.Fprintf(b, "| %s | %s | %s |\n", code(p.Name), code(cell(p.Type)), cell(p.Doc))
		}
		b.WriteString("\n")
	}
}

// Markdown returns the API reference, for docs/generated/lua-api.md.
func Markdown() string {
	var b strings.Builder
	b.WriteString("# Lua API reference\n\n")
	b.WriteString("Generated from [server/modscript/luaapi](../../server/modscript/luaapi) with `go test ./server/modscript/luaapi -update`. Do not edit it by hand.\n\n")
	b.WriteString("- The API is [proposal 0002](../proposals/0002-lua-api.md), version 1.0. A mod's manifest says `api: 1.0`.\n")
	b.WriteString("- Tests check these definitions against the runtime, so this page lists what the server has today.\n")
	b.WriteString("- Editors get the same definitions from `wactl mod types` ([mods/README.md](../../mods/README.md), \"Tools\").\n")
	b.WriteString("- The constants are in [lua-constants.md](lua-constants.md).\n\n")

	b.WriteString("## Modules\n\n| Module | Gives |\n|---|---|\n")
	for _, m := range Modules {
		fmt.Fprintf(&b, "| [%s](#%s) | %s |\n", code(m.Name), anchor(m.Name), cell(m.Doc))
	}
	b.WriteString("\n")
	for _, m := range Modules {
		fmt.Fprintf(&b, "### %s\n\n", m.Name)
		fmt.Fprintf(&b, "```lua\nlocal %s = require %q\n```\n\n", m.Local, m.Name)
		b.WriteString(m.Doc + "\n\n")
		if m.Testing {
			b.WriteString("It exists only in tests that `wactl mod test` runs.\n\n")
		}
		if m.Call != nil {
			f := *m.Call
			fmt.Fprintf(&b, "#### %s\n\n%s\n\n", code(call("", "", Func{Name: m.Local, Params: f.Params, Returns: f.Returns})), f.Doc)
		}
		if m.Constants {
			fmt.Fprintf(&b, "Constants: [lua-constants.md](lua-constants.md#%s).\n\n", anchor(m.Name))
		}
		for _, f := range m.Funcs {
			funcSection(&b, m.Local, ".", f)
		}
		for _, f := range m.Methods {
			funcSection(&b, m.Local, ":", f)
		}
	}

	b.WriteString("## Handles and tables\n\n")
	b.WriteString("Handles are read-only. Writing a field raises an error. Changes are method calls.\n\n")
	for _, c := range Classes {
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", c.Name, c.Doc)
		if c.Testing {
			b.WriteString("It exists only in tests that `wactl mod test` runs.\n\n")
		}
		if len(c.Fields) > 0 {
			b.WriteString("| Field | Type | Meaning |\n|---|---|---|\n")
			for _, f := range c.Fields {
				fmt.Fprintf(&b, "| %s | %s | %s |\n", code(f.Name), code(cell(f.Type)), cell(f.Doc))
			}
			b.WriteString("\n")
		}
		local := lowerFirst(strings.TrimPrefix(c.Name, "wa."))
		for _, f := range c.Methods {
			funcSection(&b, local, ":", f)
		}
		for _, f := range c.Functions {
			funcSection(&b, local, ".", f)
		}
	}

	b.WriteString("## Events\n\n| Event | Contexts | Handler arguments | When |\n|---|---|---|---|\n")
	for _, e := range Events {
		var args []string
		for _, p := range e.Params {
			args = append(args, p.Name+": "+p.Type)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", code(e.Name), strings.Join(e.Kinds, ", "), code(cell(strings.Join(args, ", "))), cell(e.Doc))
	}
	b.WriteString("\n## Named types\n\n")
	for _, a := range Aliases() {
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", a.Name, a.Doc)
		if a.Type != "" {
			fmt.Fprintf(&b, "%s\n\n", code(a.Type))
			continue
		}
		var vals []string
		for _, v := range a.Values {
			vals = append(vals, code(v))
		}
		b.WriteString(strings.Join(vals, ", ") + "\n\n")
	}
	return b.String()
}

// ConstantsMarkdown returns the constant modules with rAthena's names,
// for docs/generated/lua-constants.md.
func ConstantsMarkdown(consts *constants.Data) string {
	var b strings.Builder
	b.WriteString("# Lua constants\n\n")
	b.WriteString("Generated from [server/modscript/luaapi](../../server/modscript/luaapi) and the renewal script constants of reference/rathena, with `go test ./server/modscript/luaapi -update`. Do not edit it by hand.\n\n")
	b.WriteString("Each readable name comes from rAthena's name by a fixed rule ([0002a](../proposals/0002a-bonus-names.md), \"Constants used in arguments\"). The converter uses the same table.\n\n")
	tables := consts.Modules()
	for _, m := range Modules {
		if !m.Constants {
			continue
		}
		c := tables[strings.TrimPrefix(m.Name, "wa.")]
		fmt.Fprintf(&b, "## %s\n\n%s\n\n| Name | Value | rAthena |\n|---|---|---|\n", m.Name, m.Doc)
		for _, name := range slices.Sorted(maps.Keys(c.Values)) {
			fmt.Fprintf(&b, "| %s | %d | %s |\n", code(name), c.Values[name], code(c.Origins[name]))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// anchor is GitHub's heading anchor for a heading of one word with dots.
func anchor(s string) string { return strings.ReplaceAll(strings.ToLower(s), ".", "") }

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
