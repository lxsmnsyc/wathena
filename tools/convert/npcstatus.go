package convert

import (
	"fmt"
	"strings"
)

// scStart converts sc_start, sc_start2 and sc_start4 in an NPC script
// (buildin_sc_start). An NPC's flag defaults to SCSTART_NOAVOID.
func (b *body) scStart(name string, args []Expr) error {
	n := map[string]int{"sc_start": 1, "sc_start2": 2, "sc_start4": 4}[name]
	fixed := 3 + map[int]int{1: 0, 2: 1, 4: 3}[n]
	if len(args) < fixed {
		return unsupported(name, "arguments")
	}
	if len(args) > fixed+2 {
		return unsupported(name, "on another character")
	}
	st, err := b.constant(args[0], "statuses")
	if err != nil {
		return err
	}
	vals, err := b.args(args[1:])
	if err != nil {
		return err
	}
	var opts []string
	tick := vals[0]
	switch {
	case tick.static == nil:
		opts = append(opts, "milliseconds = "+tick.asInt())
	case *tick.static == -1:
		opts = append(opts, `milliseconds = "forever"`)
	case *tick.static == 0:
		// The server looks the duration up for the level.
	case *tick.static < 0:
		return unsupported(name, "with a negative duration")
	default:
		opts = append(opts, "milliseconds = "+tick.asInt())
	}
	if lv := vals[1]; lv.static == nil || *lv.static != 1 {
		opts = append(opts, "level = "+lv.asInt())
	}
	if n > 1 {
		var vs []string
		for _, v := range vals[2 : fixed-1] {
			vs = append(vs, v.asInt())
		}
		for len(vs) > 0 && vs[len(vs)-1] == "0" {
			vs = vs[:len(vs)-1]
		}
		if len(vs) > 0 {
			opts = append(opts, "values = { "+strings.Join(vs, ", ")+" }")
		}
	}
	if len(vals) > fixed-1 {
		r := vals[fixed-1]
		switch {
		case r.static == nil:
			opts = append(opts, "chance = math.max(0, math.min("+r.asInt()+", 10000))")
		case *r.static < 10000:
			opts = append(opts, fmt.Sprintf("chance = %d", max(*r.static, 0)))
		}
	}
	flag := int64(1)
	if len(vals) > fixed {
		f := vals[fixed]
		if f.static == nil {
			return unsupported(name, "with a computed flag")
		}
		if *f.static&^(1|2|8|16) != 0 {
			return unsupported(name, "with flag %d", *f.static)
		}
		flag = *f.static
	}
	for _, o := range scStartOptions {
		if flag&o.bit != 0 {
			opts = append(opts, o.key+" = true")
		}
	}
	if len(opts) == 0 {
		b.line("player:start_status(%s)", st)
	} else {
		b.line("player:start_status(%s, { %s })", st, strings.Join(opts, ", "))
	}
	return nil
}

// getstatus converts getstatus (buildin_getstatus): whether a status is
// active, one of its values, or the time it has left.
func (b *body) getstatus(args []Expr) (val, error) {
	if len(args) < 1 || len(args) > 2 {
		return val{}, unsupported("getstatus", "of another character")
	}
	st, err := b.constant(args[0], "statuses")
	if err != nil {
		return val{}, err
	}
	typ := int64(0)
	if len(args) == 2 {
		v, err := b.expr(args[1])
		if err != nil {
			return val{}, err
		}
		if v.static == nil {
			return val{}, unsupported("getstatus", "a computed type")
		}
		typ = *v.static
	}
	get := "player:get_status(" + st + ")"
	switch typ {
	case 1:
		return val{code: "((" + get + " or { level = 0 }).level)", t: tInt}, nil
	case 2, 3, 4:
		return val{code: fmt.Sprintf("((%s or { values = { 0, 0, 0 } }).values[%d])", get, typ-1), t: tInt}, nil
	case 5:
		// rAthena gives -1 for a status without a timer, and 0 when the
		// status is not active.
		return val{code: "(function(s) if s == nil then return 0 end if s.remaining_milliseconds == \"forever\" then return -1 end return s.remaining_milliseconds end)(" + get + ")", t: tInt}, nil
	}
	return val{code: "player:has_status(" + st + ")", t: tBool}, nil
}
