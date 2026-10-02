package convert

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// rAthena's SCSTART_ flags of sc_start, and the options they become
// (proposal 0002, review decision 19).
var scStartOptions = []struct {
	bit int64
	key string
}{
	{1, "cannot_be_avoided"}, // SCSTART_NOAVOID
	{2, "keep_duration"},     // SCSTART_NOTICKDEF
	{8, "keep_chance"},       // SCSTART_NORATEDEF
	{16, "hide_icon"},        // SCSTART_NOICON
}

// scStart converts sc_start, sc_start2 and sc_start4 (buildin_sc_start)
// to player:start_status. item says the script runs from an item, whose
// flag defaults to SCSTART_NOTICKDEF; other scripts default to
// SCSTART_NOAVOID.
func (t *imperative) scStart(name string, args []Expr, item bool) error {
	n := map[string]int{"sc_start": 1, "sc_start2": 2, "sc_start4": 4}[name]
	// sc_start <type>,<tick>,<val1>{,<val2>...}{,<rate>{,<flag>{,<GID>}}}
	fixed := 3 + map[int]int{1: 0, 2: 1, 4: 3}[n]
	if len(args) < fixed {
		return unsupported(name, "arguments")
	}
	if len(args) > fixed+2 {
		return unsupported(name, "on another character")
	}
	st, err := t.constArg(args[0], "statuses")
	if err != nil {
		return err
	}
	var opts []string
	if v, ok := t.staticNum(args[1]); ok {
		switch {
		case v == -1:
			opts = append(opts, `milliseconds = "forever"`)
		case v == 0:
			// No duration: the server looks it up for the level, from
			// the status's skill.
		case v < 0:
			return unsupported(name, "with a negative duration")
		default:
			opts = append(opts, fmt.Sprintf("milliseconds = %d", v))
		}
	} else {
		d, err := t.num(args[1])
		if err != nil {
			return err
		}
		opts = append(opts, "milliseconds = "+d)
	}
	if v, ok := t.staticNum(args[2]); !ok || v != 1 {
		lv, err := t.num(args[2])
		if err != nil {
			return err
		}
		opts = append(opts, "level = "+lv)
	}
	if n > 1 {
		var vals []string
		for _, a := range args[3:fixed] {
			v, err := t.num(a)
			if err != nil {
				return err
			}
			vals = append(vals, v)
		}
		for len(vals) > 0 && vals[len(vals)-1] == "0" {
			vals = vals[:len(vals)-1]
		}
		if len(vals) > 0 {
			opts = append(opts, "values = { "+strings.Join(vals, ", ")+" }")
		}
	}
	if len(args) > fixed {
		if v, ok := t.staticNum(args[fixed]); ok {
			if v < 10000 {
				opts = append(opts, fmt.Sprintf("chance = %d", max(v, 0)))
			}
		} else {
			r, err := t.num(args[fixed])
			if err != nil {
				return err
			}
			opts = append(opts, "chance = math.max(0, math.min("+r+", 10000))")
		}
	}
	flag := int64(1)
	if item {
		flag = 2
	}
	if len(args) > fixed+1 {
		v, ok := t.staticNum(args[fixed+1])
		if !ok {
			return unsupported(name, "with a computed flag")
		}
		if v&^(1|2|8|16) != 0 {
			return unsupported(name, "with flag %d", v)
		}
		flag = v
	}
	for _, o := range scStartOptions {
		if flag&o.bit != 0 {
			opts = append(opts, o.key+" = true")
		}
	}
	if len(opts) == 0 {
		t.line("player:start_status(%s)", st)
	} else {
		t.line("player:start_status(%s, { %s })", st, strings.Join(opts, ", "))
	}
	return nil
}

// rAthena's BSF_REM_ON_ flags of bonus_script, and the ends_on names
// they become.
var bonusScriptEnds = []struct {
	bit  int64
	name string
}{
	{0x001, "death"}, {0x002, "dispell"}, {0x004, "clearance"}, {0x008, "logout"},
	{0x010, "banishing_buster"}, {0x020, "refresh"}, {0x040, "lux_anima"},
	{0x080, "magic_gear_change"}, {0x100, "damage"},
}

// bonusScript converts bonus_script (buildin_bonus_script) to
// player:add_timed_bonuses. The key is made from the script's text, as
// rAthena tells copies apart by the script's text. The flags of a second
// use become the option on_repeat, as pc_bonus_script_add reads them:
//   - None keeps the first copy, the API's default.
//   - BSF_FORCE_REPLACE (0x400) becomes on_repeat = "extend".
//   - BSF_FORCE_DUPLICATE (0x800) becomes on_repeat = "add".
func (t *imperative) bonusScript(args []Expr) error {
	const name = "bonus_script"
	if len(args) < 2 || len(args) > 5 {
		return unsupported(name, "on another character")
	}
	str, ok := args[0].(*Str)
	if !ok {
		return unsupported(name, "a computed bonus script")
	}
	secs, ok := t.staticNum(args[1])
	if !ok {
		return unsupported(name, "a computed duration")
	}
	if secs <= 0 {
		return unsupported(name, "a duration of %d", secs)
	}
	var flag, kind, icon int64
	if len(args) > 2 {
		if flag, ok = t.staticNum(args[2]); !ok {
			return unsupported(name, "a computed flag")
		}
	}
	if len(args) > 3 {
		if kind, ok = t.staticNum(args[3]); !ok || kind < 0 || kind > 2 {
			return unsupported(name, "a computed or unknown type")
		}
	}
	if len(args) > 4 {
		if icon, ok = t.staticNum(args[4]); !ok {
			return unsupported(name, "a computed icon")
		}
	}
	src := strings.TrimSpace(str.Value)
	effects, err := t.c.equipScriptEnv(strings.Trim(src, "{}"), nil)
	if err != nil {
		return err
	}
	var entries []string
	for _, e := range effects {
		cmd := strings.ToLower(e.cmd.Name)
		if !strings.HasPrefix(cmd, "bonus") || e.guard != nil {
			return unsupported(name, "with %s", cmd)
		}
		n := 1
		if cmd != "bonus" {
			if len(cmd) != 6 || cmd[5] < '2' || cmd[5] > '5' {
				return unsupported(name, "with %s", cmd)
			}
			n = int(cmd[5] - '0')
		}
		calls, err := t.c.mapBonus(n, e.args)
		if err != nil {
			return err
		}
		for _, w := range calls {
			if !w.isStatic() {
				return unsupported(name, "with computed bonuses")
			}
			l, err := t.c.luaBonus(w, nil)
			if err != nil {
				return err
			}
			entries = append(entries, "{ "+strings.TrimSuffix(strings.TrimPrefix(l, "bonuses.add("), ")")+" }")
		}
	}
	if len(entries) == 0 {
		return unsupported(name, "with no bonuses")
	}
	var opts []string
	var ends []string
	for _, b := range bonusScriptEnds {
		if flag&b.bit != 0 {
			ends = append(ends, luaString(b.name))
		}
	}
	if len(ends) > 0 {
		opts = append(opts, "ends_on = { "+strings.Join(ends, ", ")+" }")
	}
	// BSF_FORCE_REPLACE comes first when both are set, as in
	// pc_bonus_script_add.
	switch {
	case flag&0x400 != 0:
		opts = append(opts, `on_repeat = "extend"`)
	case flag&0x800 != 0:
		opts = append(opts, `on_repeat = "add"`)
	}
	if icon > 0 {
		// wAthena has no constants for icons: the EFST_ ID stays a
		// number.
		opts = append(opts, fmt.Sprintf("icon = %d", icon))
	}
	switch kind {
	case 1:
		opts = append(opts, `kind = "buff"`)
	case 2:
		opts = append(opts, `kind = "debuff"`)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(src)) // A hash's Write never fails.
	key := fmt.Sprintf("bonus_script:%08x", h.Sum32())
	t.line("player:add_timed_bonuses(%q, %d, {", key, secs*1000)
	for _, e := range entries {
		t.line("  %s,", e)
	}
	if len(opts) == 0 {
		t.line("})")
	} else {
		t.line("}, { %s })", strings.Join(opts, ", "))
	}
	return nil
}
