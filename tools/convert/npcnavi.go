package convert

import (
	"fmt"
	"strings"
)

// Navigation (proposal 0006, section 5; decision 0079):
// navigateto("<map>"{,<x>,<y>,<flag>,<hide window>,<monster ID>{,<char
// ID>}}) becomes player:navigate_to(map, { x, y, services, hide_window,
// monster }). A field the script leaves out stays out of the table, so
// the API's defaults are rAthena's: no cell, NAV_KAFRA_AND_AIRSHIP and a
// hidden window. The flag becomes a constant of wa.navigation. A call for
// another character is refused. F_Navi converts as a whole: its markup
// is text that the client reads.

// naviCommand converts navigateto. ok is false for another name.
func (b *body) naviCommand(name string, args []Expr) (ok bool, err error) {
	if name != "navigateto" {
		return false, nil
	}
	if len(args) < 1 || len(args) > 7 {
		return true, unsupported(name, "arguments")
	}
	if len(args) == 7 {
		return true, unsupported(name, "for another character")
	}
	m, err := b.expr(args[0])
	if err != nil {
		return true, err
	}
	if m.t != tStr {
		return true, unsupported(name, "a map that is not text")
	}
	var opts []string
	for i, field := range []string{"x", "y"} {
		if len(args) <= i+1 {
			break
		}
		v, err := b.expr(args[i+1])
		if err != nil {
			return true, err
		}
		opts = append(opts, fmt.Sprintf("%s = %s", field, v.asInt()))
	}
	if len(args) > 3 {
		code, err := b.constant(args[3], "navigation")
		if err != nil {
			return true, err
		}
		opts = append(opts, "services = "+code)
	}
	if len(args) > 4 {
		v, err := b.expr(args[4])
		if err != nil {
			return true, err
		}
		opts = append(opts, "hide_window = "+v.asBool())
	}
	if len(args) > 5 {
		v, err := b.expr(args[5])
		if err != nil {
			return true, err
		}
		opts = append(opts, "monster = "+v.asInt())
	}
	if len(opts) == 0 {
		b.line("player:navigate_to(%s)", m.code)
		return true, nil
	}
	b.line("player:navigate_to(%s, { %s })", m.code, strings.Join(opts, ", "))
	return true, nil
}
