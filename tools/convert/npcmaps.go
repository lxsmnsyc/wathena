package convert

import (
	"fmt"
	"strings"
)

// The map, area and monster commands of review decision 58, area B8 of
// proposal 0002b, onto the calls of server/modscript/maps.go (decision
// 0065). A command on another map runs in that map's zone after the
// current step.

// setcellKinds are maps.set_cells' kinds by rAthena's cell_t (map.hpp
// 748), and checkcellKinds maps.check_cell's by cell_chk (map.hpp 764).
var (
	setcellKinds = []string{"walkable", "shootable", "water", "npc", "basilica", "land_protector", "no_vending",
		"no_chat", "maelstrom", "ice_wall", "no_buying_store"}
	checkcellKinds = []string{"", "wall", "water", "cliff", "pass", "reach", "no_pass", "no_reach", "stack", "npc",
		"basilica", "land_protector", "no_vending", "no_chat", "maelstrom", "ice_wall", "no_buying_store"}
)

// cellRefused are the cell kinds the server refuses, and why
// (server/zone/maps.go).
var cellRefused = map[string]string{
	"water":          "water cells are not in the server's map data",
	"basilica":       "wAthena's Basilica keeps no cell flag, and battlegrounds do not exist",
	"land_protector": "Land Protector does not exist",
	"no_chat":        "chat rooms do not exist",
	"maelstrom":      "Maelstrom does not exist",
	"ice_wall":       "wAthena's Ice Wall keeps no cell flag",
	"npc":            "touch areas read their NPCs, not a cell flag",
}

// mapFlagRefused are rAthena's map flags that maps.set_flag refuses,
// grouped by why. nomemo is the flag wAthena has.
var mapFlagRefused = map[string]string{}

func init() {
	refuse := func(why string, flags ...string) {
		for _, f := range flags {
			mapFlagRefused[f] = why
		}
	}
	refuse("PvP is not ported", "pvp", "pvp_noparty", "pvp_noguild", "pvp_nightmaredrop", "pvp_nocalcrank")
	refuse("sieges do not exist", "gvg", "gvg_noparty", "gvg_castle", "gvg_dungeon", "gvg_te", "gvg_te_castle")
	refuse("battlegrounds do not exist", "battleground")
	refuse("the client draws no weather", "snow", "fog", "sakura", "leaves", "rain", "clouds", "clouds2", "fireworks")
	refuse("chat rooms do not exist", "nochat")
	refuse("day and night do not exist", "nightenabled")
}

// mobInfoFields are monsters.get's fields by getmonsterinfo's MOB_ type
// (script.hpp 360).
var mobInfoFields = []string{"", "name", "level", "hp", "sp", "base_exp", "job_exp", "attack_min", "attack_max", "defense",
	"magic_defense", "resistance", "magic_resistance", "str", "agi", "vit", "int", "dex", "luk", "speed", "attack_range",
	"skill_range", "chase_range", "size", "race", "element", "element_level", "modes", "mvp_exp", "id"}

// spawnAIs are spawning.monsters' AIs by rAthena's AI_ value.
var spawnAIs = []string{"none", "attack", "sphere", "flora", "zanzou", "legion", "faw", "guild", "wave_mode", "abr", "bionic"}

// mobLabel converts a monster label of monster, areamonster, killmonster
// and mobcount: "" for none, a label of the NPC itself, or another NPC's
// label, which names its record.
func (b *body) mobLabel(what string, e Expr) (string, error) {
	if s, ok := e.(*Str); ok && s.Value != "" && !b.n.isFunction {
		if i := strings.LastIndex(s.Value, "::"); i > 0 {
			name, label := s.Value[:i], s.Value[i+2:]
			if !labelName.MatchString(label) {
				return "", unsupported(what, "%q is not an event label", s.Value)
			}
			switch {
			case name != b.n.line.Name || b.n.c.duplicated[name]:
				// Another NPC, or the source of duplicates that run this
				// script: the name is the source's, as rAthena reads it.
				return luaString(b.n.c.recordFor(name) + "::" + label), nil
			case !b.n.eventLabels[strings.ToLower(label)]:
				// A label the NPC does not have still names its monsters.
				// Their deaths run nothing, as npc_event finds no label.
				return luaString(label), nil
			}
		}
	}
	if bin, ok := e.(*Binary); ok && bin.Op == "+" && !b.n.isFunction {
		// "<NPC>::OnMobDead" + .@i: a label made when the script runs,
		// whose NPC is fixed.
		if s, ok := bin.X.(*Str); ok {
			if i := strings.LastIndex(s.Value, "::"); i >= 0 && labelName.MatchString(s.Value[i+2:]+"0") {
				rest, err := b.expr(bin.Y)
				if err != nil {
					return "", err
				}
				prefix := s.Value[i+2:]
				switch name := s.Value[:i]; name {
				case "":
					return "", unsupported(what, "a global event label (::%s)", prefix)
				case b.n.line.Name:
				default:
					prefix = b.n.c.recordFor(name) + "::" + prefix
				}
				return luaString(prefix) + " .. " + rest.asText(), nil
			}
		}
	}
	if computedLabel(e) {
		// A label made when the script runs, such as .npc_name$ +
		// "::OnMyMobDead": the server finds its NPC by name, as
		// rAthena's npc_event does. R.label gives nil for "".
		v, err := b.expr(e)
		if err != nil {
			return "", err
		}
		if v.t == tInt || v.t == tBool {
			return "", unsupported(what, "an event label that is not a string")
		}
		return "R.label(" + v.asText() + ")", nil
	}
	label, err := b.ownEvent(what, e)
	if err != nil || label == "" {
		return "", err
	}
	// The label as the script writes it: rAthena compares labels as
	// written (buildin_killmonster_sub).
	return luaString(b.ownLabelCase(e)), nil
}

// computedLabel reports whether a label is made when the script runs,
// other than strnpcinfo(0) or strnpcinfo(3) and a fixed label, which
// ownEvent reads.
func computedLabel(e Expr) bool {
	switch e := e.(type) {
	case *Str:
		return false
	case *Binary:
		c, isCall := e.X.(*Call)
		_, isStr := e.Y.(*Str)
		return e.Op != "+" || !isCall || !isStr || !strings.EqualFold(c.Name, "strnpcinfo")
	}
	return true
}

// ownLabelCase is a label of the NPC itself as the script writes it.
func (b *body) ownLabelCase(e Expr) string {
	switch e := e.(type) {
	case *Str:
		return e.Value[strings.LastIndex(e.Value, "::")+2:]
	case *Binary:
		return e.Y.(*Str).Value[2:]
	}
	return ""
}

// staticInt reads a fixed integer argument.
func (b *body) staticInt(what, field string, e Expr) (int64, error) {
	v, err := b.expr(e)
	if err != nil {
		return 0, err
	}
	if v.static == nil {
		return 0, unsupported(what, "a computed %s", field)
	}
	return *v.static, nil
}

// spawnTail converts monster's and areamonster's event label, size and
// AI into spawning.monsters' fields.
func (b *body) spawnTail(what string, args []Expr) (string, error) {
	var out []string
	if len(args) > 0 {
		label, err := b.mobLabel(what, args[0])
		if err != nil {
			return "", err
		}
		if label != "" {
			out = append(out, "label = "+label)
		}
	}
	if len(args) > 1 {
		size, err := b.staticInt(what, "size", args[1])
		if err != nil {
			return "", err
		}
		switch size {
		case 0:
		case 1:
			out = append(out, `size = "small"`)
		case 2:
			out = append(out, `size = "big"`)
		default:
			// buildin_monster refuses another size.
			return "", unsupported(what, "the size %d, which rAthena refuses", size)
		}
	}
	if len(args) > 2 {
		ai, err := b.staticInt(what, "AI", args[2])
		if err != nil {
			return "", err
		}
		if ai < 0 || int(ai) >= len(spawnAIs) {
			return "", unsupported(what, "the AI %d, which rAthena refuses", ai)
		}
		if ai != 0 {
			out = append(out, "ai = "+luaString(spawnAIs[ai]))
		}
	}
	if len(out) == 0 {
		return "nil", nil
	}
	return "{ " + strings.Join(out, ", ") + " }", nil
}

// spawnMonster converts the monster argument of monster and areamonster:
// an ID or an AegisName.
func (b *body) spawnMonster(what string, e Expr) (string, error) {
	c := b.n.c
	mob, err := b.expr(e)
	if err != nil {
		return "", err
	}
	var id int64
	switch {
	case mob.static != nil:
		id = *mob.static
	case mob.str != nil:
		var ok bool
		if id, ok = c.monsterIDs[strings.ToUpper(*mob.str)]; !ok {
			return "", unsupported(what, "%q is not a monster", *mob.str)
		}
	default:
		return mob.asInt(), nil
	}
	info, ok := c.monsters[id]
	if !ok {
		return "", unsupported(what, "%d is not a monster, such as a random monster", id)
	}
	return b.n.file.record("monsters.id", info.sprite), nil
}

// monster writes rAthena's monster command: monster "<map>",<x>,<y>,
// "<name>",<monster>,<amount>{,"<event label>"{,<size>{,<ai>}}}
// (script.cpp 11238), through R.monster.
func (b *body) monster(args []Expr) error {
	const what = "monster"
	if len(args) < 6 || len(args) > 9 {
		return unsupported(what, "arguments")
	}
	vals, err := b.args(args[:6])
	if err != nil {
		return err
	}
	if vals[0].str != nil && *vals[0].str == "this" {
		return unsupported(what, "on the attached player's map (\"this\")")
	}
	code, err := b.spawnMonster(what, args[4])
	if err != nil {
		return err
	}
	tail, err := b.spawnTail(what, args[6:])
	if err != nil {
		return err
	}
	b.line("R.monster(%s, %s, %s, %s, %s, %s, %s)", vals[0].asText(), vals[1].asInt(), vals[2].asInt(), vals[3].asText(), code, vals[5].asInt(), tail)
	return nil
}

// areamonster writes areamonster "<map>",<x1>,<y1>,<x2>,<y2>,"<name>",
// <monster>,<amount>{,"<event label>"{,<size>{,<ai>}}} (script.cpp
// 11359), through R.areamonster.
func (b *body) areamonster(args []Expr) error {
	const what = "areamonster"
	if len(args) < 8 || len(args) > 11 {
		return unsupported(what, "arguments")
	}
	vals, err := b.args(args[:8])
	if err != nil {
		return err
	}
	if vals[0].str != nil && *vals[0].str == "this" {
		return unsupported(what, "on the attached player's map (\"this\")")
	}
	code, err := b.spawnMonster(what, args[6])
	if err != nil {
		return err
	}
	tail, err := b.spawnTail(what, args[8:])
	if err != nil {
		return err
	}
	b.line("R.areamonster(%s, %s, %s, %s, %s, %s, %s, %s, %s)", vals[0].asText(), vals[1].asInt(), vals[2].asInt(), vals[3].asInt(),
		vals[4].asInt(), vals[5].asText(), code, vals[7].asInt(), tail)
	return nil
}

// killmonster writes killmonster "<map>","<event label>"{,<type>}
// (script.cpp 11480): the label's monsters, or "All", every monster
// that no spawn line placed.
func (b *body) killmonster(args []Expr) error {
	const what = "killmonster"
	if len(args) < 2 || len(args) > 3 {
		return unsupported(what, "arguments")
	}
	m, err := b.expr(args[0])
	if err != nil {
		return err
	}
	var opts []string
	run, err := b.runLabels(what, args[2:])
	if err != nil {
		return err
	}
	if s, ok := args[1].(*Str); ok && s.Value == "All" {
		opts = append(opts, "scripted = true")
	} else {
		label, err := b.mobLabel(what, args[1])
		if err != nil {
			return err
		}
		if label == "" {
			return unsupported(what, "without an event label")
		}
		if strings.HasPrefix(label, "R.label(") {
			// A computed label may be "All" or "".
			b.line("R.killmonster(%s, %s, %s)", m.asText(), strings.TrimSuffix(strings.TrimPrefix(label, "R.label("), ")"), cmpOr(run, "false"))
			return nil
		}
		opts = append(opts, "label = "+label)
	}
	opts = append(opts, run...)
	b.n.file.need("maps")
	b.line("R.on_map(%s, maps.remove_monsters, { %s })", m.asText(), strings.Join(opts, ", "))
	return nil
}

// runLabels reads killmonster's and killmonsterall's type: 1 runs the
// labels.
func (b *body) runLabels(what string, args []Expr) ([]string, error) {
	if len(args) == 0 {
		return nil, nil
	}
	t, err := b.expr(args[0])
	if err != nil {
		return nil, err
	}
	if t.static == nil {
		return []string{"run_labels = " + t.asInt() + " == 1"}, nil
	}
	if *t.static == 1 {
		return []string{"run_labels = true"}, nil
	}
	return nil, nil
}

// cmpOr is run_labels' value of runLabels, or def.
func cmpOr(run []string, def string) string {
	if len(run) == 0 {
		return def
	}
	return strings.TrimPrefix(run[0], "run_labels = ")
}

// killmonsterall writes killmonsterall "<map>"{,<type>} (script.cpp
// 11521): every monster, those of spawn lines too, which come back.
func (b *body) killmonsterall(args []Expr) error {
	const what = "killmonsterall"
	if len(args) < 1 || len(args) > 2 {
		return unsupported(what, "arguments")
	}
	m, err := b.expr(args[0])
	if err != nil {
		return err
	}
	run, err := b.runLabels(what, args[1:])
	if err != nil {
		return err
	}
	opts := "nil"
	if len(run) > 0 {
		opts = "{ " + strings.Join(run, ", ") + " }"
	}
	b.n.file.need("maps")
	b.line("R.on_map(%s, maps.remove_monsters, %s)", m.asText(), opts)
	return nil
}

// mobcount converts mobcount("<map>","<event label>") (script.cpp
// 14378): the label's living monsters, every monster for "all", and -1
// for a map that does not run.
func (b *body) mobcount(args []Expr) (val, error) {
	const what = "mobcount"
	if len(args) != 2 {
		return val{}, unsupported(what, "arguments")
	}
	m, err := b.expr(args[0])
	if err != nil {
		return val{}, err
	}
	if m.str != nil && *m.str == "this" {
		return val{}, unsupported(what, "on the attached player's map (\"this\")")
	}
	b.n.file.need("maps")
	if s, ok := args[1].(*Str); ok && s.Value == "all" {
		return val{code: fmt.Sprintf("(maps.count_monsters(%s) or -1)", m.asText()), t: tInt}, nil
	}
	label, err := b.mobLabel(what, args[1])
	if err != nil {
		return val{}, err
	}
	if label == "" {
		return val{}, unsupported(what, "without an event label")
	}
	if strings.HasPrefix(label, "R.label(") {
		// A computed label may be "all" or "".
		return val{code: fmt.Sprintf("R.mobcount(%s, %s)", m.asText(), strings.TrimSuffix(strings.TrimPrefix(label, "R.label("), ")")), t: tInt}, nil
	}
	return val{code: fmt.Sprintf("(maps.count_monsters(%s, { label = %s }) or -1)", m.asText(), label), t: tInt}, nil
}

// mapsRead converts the commands of area B8 that give a value. ok is
// false for another command.
func (b *body) mapsRead(name string, args []Expr) (v val, ok bool, err error) {
	f := b.n.file
	switch name {
	case "mobcount":
		v, err := b.mobcount(args)
		return v, true, err
	case "getmapusers":
		if len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		m, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		f.need("maps")
		return val{code: "(maps.count_players(" + m.asText() + ") or -1)", t: tInt}, true, nil
	case "getareausers":
		if len(args) != 5 {
			return val{}, true, unsupported(name, "arguments")
		}
		vals, err := b.args(args)
		if err != nil {
			return val{}, true, err
		}
		f.need("maps")
		return val{code: fmt.Sprintf("(maps.count_players(%s, { rectangle = { { %s, %s }, { %s, %s } } }) or -1)",
			vals[0].asText(), vals[1].asInt(), vals[2].asInt(), vals[3].asInt(), vals[4].asInt()), t: tInt}, true, nil
	case "checkcell":
		if len(args) != 4 {
			return val{}, true, unsupported(name, "arguments")
		}
		vals, err := b.args(args[:3])
		if err != nil {
			return val{}, true, err
		}
		kind, err := b.staticInt(name, "cell type", args[3])
		if err != nil {
			return val{}, true, err
		}
		if kind <= 0 || int(kind) >= len(checkcellKinds) {
			return val{}, true, unsupported(name, "the cell type %d: cell_gettype gives a gat type, which the server does not keep", kind)
		}
		k := checkcellKinds[kind]
		if why, refused := cellRefused[k]; refused && k != "npc" {
			return val{}, true, unsupported(name, "%s cells: %s", k, why)
		}
		f.need("maps")
		return val{code: fmt.Sprintf("maps.check_cell(%s, %s, %s, %s)", vals[0].asText(), vals[1].asInt(), vals[2].asInt(), luaString(k)), t: tBool}, true, nil
	case "getmapflag":
		if len(args) < 2 || len(args) > 3 {
			return val{}, true, unsupported(name, "arguments")
		}
		m, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		flag, err := b.mapFlag(name, args[1])
		if err != nil {
			return val{}, true, err
		}
		if _, refused := mapFlagRefused[flag]; refused || flag != "nomemo" {
			// No map has a flag that set_flag refuses.
			return intVal(0), true, nil
		}
		f.need("maps")
		return val{code: fmt.Sprintf("maps.get_flag(%s, %s)", m.asText(), luaString(flag)), t: tBool}, true, nil
	case "getmonsterinfo":
		if len(args) != 2 {
			return val{}, true, unsupported(name, "arguments")
		}
		mob, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		kind, err := b.expr(args[1])
		if err != nil {
			return val{}, true, err
		}
		t := tAny
		if kind.static != nil {
			if *kind.static <= 0 || int(*kind.static) >= len(mobInfoFields) {
				return val{}, true, unsupported(name, "the type %d", *kind.static)
			}
			t = tInt
			if *kind.static == 1 {
				t = tStr
			}
		}
		code := mob.asInt()
		if mob.t == tStr {
			code = mob.code
		}
		return val{code: fmt.Sprintf("R.monster_info(%s, %s)", code, kind.asInt()), t: t}, true, nil
	case "checkwall":
		if len(args) != 1 {
			return val{}, true, unsupported(name, "arguments")
		}
		n, err := b.expr(args[0])
		if err != nil {
			return val{}, true, err
		}
		return val{code: "R.checkwall(" + n.asText() + ")", t: tBool}, true, nil
	}
	return val{}, false, nil
}

// mapFlag reads a map flag argument, such as mf_nomemo: rAthena's name
// without mf_.
func (b *body) mapFlag(what string, e Expr) (string, error) {
	id, ok := e.(*Ident)
	if !ok || !strings.HasPrefix(strings.ToLower(id.Name), "mf_") {
		return "", unsupported(what, "a computed map flag")
	}
	return strings.ToLower(id.Name)[3:], nil
}

// mapsCommand converts the commands of area B8 that are statements. ok
// is false for another command.
func (b *body) mapsCommand(name string, args []Expr) (ok bool, err error) {
	f := b.n.file
	switch name {
	case "monster":
		return true, b.monster(args)
	case "areamonster":
		return true, b.areamonster(args)
	case "killmonster":
		return true, b.killmonster(args)
	case "killmonsterall":
		return true, b.killmonsterall(args)
	case "mapwarp":
		// mapwarp "<from>","<to>",<x>,<y>{,<type>,<ID>} (script.cpp
		// 14317).
		if len(args) != 4 && len(args) != 6 {
			return true, unsupported(name, "arguments")
		}
		vals, err := b.args(args)
		if err != nil {
			return true, err
		}
		opts := "nil"
		if len(args) == 6 {
			if vals[4].static == nil {
				return true, unsupported(name, "a computed type")
			}
			switch *vals[4].static {
			case 1:
				opts = "{ guild = " + vals[5].asInt() + " }"
			case 2:
				opts = "{ party = " + vals[5].asInt() + " }"
			}
		}
		f.need("maps")
		b.line("R.warp_players(%s, %s, %s, %s, %s)", vals[0].asText(), vals[1].asText(), vals[2].asInt(), vals[3].asInt(), opts)
	case "areawarp":
		// areawarp "<from>",<x1>,<y1>,<x2>,<y2>,"<to>",<x3>,<y3>{,<x4>,<y4>}
		// (script.cpp 5695).
		if len(args) != 8 && len(args) != 10 {
			return true, unsupported(name, "arguments")
		}
		vals, err := b.args(args)
		if err != nil {
			return true, err
		}
		if vals[5].str != nil && *vals[5].str == "Random" {
			return true, unsupported(name, "to a random cell of each player's map (\"Random\")")
		}
		area := fmt.Sprintf("rectangle = { { %s, %s }, { %s, %s } }", vals[1].asInt(), vals[2].asInt(), vals[3].asInt(), vals[4].asInt())
		if len(args) == 10 {
			b.line("R.areawarp(%s, %s, %s, %s, %s, %s, { %s })", vals[0].asText(), vals[5].asText(), vals[6].asInt(), vals[7].asInt(),
				vals[8].asInt(), vals[9].asInt(), area)
			break
		}
		b.line("R.warp_players(%s, %s, %s, %s, { %s })", vals[0].asText(), vals[5].asText(), vals[6].asInt(), vals[7].asInt(), area)
	case "warpparty":
		// warpparty "<to>",<x>,<y>,<party ID> (script.cpp 5781).
		if len(args) != 4 {
			return true, unsupported(name, "from one map or with a range, which party:warp does not take")
		}
		vals, err := b.args(args)
		if err != nil {
			return true, err
		}
		if s := vals[0].str; s != nil {
			switch *s {
			case "Random", "SavePointAll", "SavePoint", "Leader", "RandomAll":
				return true, unsupported(name, "to %q, which party:warp does not take", *s)
			}
		} else {
			return true, unsupported(name, "to a computed map, which may be Random, SavePoint or Leader")
		}
		b.line("R.warpparty(%s, %s, %s, %s)", vals[3].asInt(), vals[0].asText(), vals[1].asInt(), vals[2].asInt())
	case "setcell":
		// setcell "<map>",<x1>,<y1>,<x2>,<y2>,<type>,<flag> (script.cpp
		// 20631).
		if len(args) != 7 {
			return true, unsupported(name, "arguments")
		}
		vals, err := b.args(args)
		if err != nil {
			return true, err
		}
		kind, err := b.staticInt(name, "cell type", args[5])
		if err != nil {
			return true, err
		}
		if kind < 0 || int(kind) >= len(setcellKinds) {
			return true, unsupported(name, "the cell type %d", kind)
		}
		k := setcellKinds[kind]
		if why, refused := cellRefused[k]; refused {
			return true, unsupported(name, "%s cells: %s", k, why)
		}
		b.line("R.on_map(%s, maps.set_cells, %s, %s, %s, %s, %s, %s)", vals[0].asText(), vals[1].asInt(), vals[2].asInt(),
			vals[3].asInt(), vals[4].asInt(), luaString(k), vals[6].asBool())
		f.need("maps")
	case "setwall":
		// setwall "<map>",<x>,<y>,<size>,<dir>,<shootable>,"<name>"
		// (script.cpp 14633).
		if len(args) != 7 {
			return true, unsupported(name, "arguments")
		}
		vals, err := b.args(args)
		if err != nil {
			return true, err
		}
		b.line("R.setwall(%s, %s, %s, %s, %s, %s, %s)", vals[0].asText(), vals[1].asInt(), vals[2].asInt(), vals[3].asInt(),
			vals[4].asInt(), vals[5].asBool(), vals[6].asText())
	case "delwall":
		if len(args) != 1 {
			return true, unsupported(name, "arguments")
		}
		n, err := b.expr(args[0])
		if err != nil {
			return true, err
		}
		b.line("R.delwall(%s)", n.asText())
	case "setmapflag", "removemapflag":
		if len(args) < 2 || len(args) > 3 || name == "removemapflag" && len(args) > 2 {
			return true, unsupported(name, "arguments")
		}
		m, err := b.expr(args[0])
		if err != nil {
			return true, err
		}
		flag, err := b.mapFlag(name, args[1])
		if err != nil {
			return true, err
		}
		if flag != "nomemo" {
			why, refused := mapFlagRefused[flag]
			if !refused {
				why = "its effect is not built yet"
			}
			return true, unsupported(name, "the map flag %s: %s", flag, why)
		}
		f.need("maps")
		b.line("R.on_map(%s, maps.set_flag, %q, %t)", m.asText(), flag, name == "setmapflag")
	case "getfreecell":
		return true, b.getfreecell(args)
	case "areaannounce":
		// areaannounce "<map>",<x1>,<y1>,<x2>,<y2>,"<text>",<flag>{,...}
		// (script.cpp 12050). The flag's colors and the font, as
		// mapannounce's, do not convert.
		if len(args) < 7 {
			return true, unsupported(name, "arguments")
		}
		vals, err := b.args(args[:6])
		if err != nil {
			return true, err
		}
		b.line("R.areaannounce(%s, %s, %s, %s, %s, %s)", vals[0].asText(), vals[1].asInt(), vals[2].asInt(), vals[3].asInt(),
			vals[4].asInt(), vals[5].asText())
	default:
		return false, nil
	}
	return true, nil
}

// getfreecell writes getfreecell "<map>",<rX>,<rY>{,<x>,<y>,<rangeX>,
// <rangeY>,<flag>} (script.cpp 20657): rX and rY get the cell, or x and
// y when no try found one, as map_search_freecell leaves them.
func (b *body) getfreecell(args []Expr) error {
	const what = "getfreecell"
	if len(args) < 3 || len(args) > 8 {
		return unsupported(what, "arguments")
	}
	var targets [2]*Var
	for i := range targets {
		v, ok := asVar(args[1+i])
		if ok {
			if _, _, str := varName(v.Name); str {
				ok = false
			}
		}
		if !ok {
			return unsupported(what, "into values that are not number variables")
		}
		targets[i] = v
	}
	m, err := b.expr(args[0])
	if err != nil {
		return err
	}
	if m.str != nil && *m.str == "this" {
		return unsupported(what, "on the attached unit's map (\"this\")")
	}
	opt := []string{"0", "0", "-1", "-1", "1"}
	for i := 3; i < len(args); i++ {
		v, err := b.expr(args[i])
		if err != nil {
			return err
		}
		opt[i-3] = v.asInt()
	}
	x, y := b.hide("freecell_x"), b.hide("freecell_y")
	b.line("%s, %s = R.getfreecell(%s, %s, %s, %s, %s, %s, player)", x, y, m.asText(), opt[0], opt[1], opt[2], opt[3], opt[4])
	for i, v := range []string{x, y} {
		if err := b.writeVar(targets[i], val{code: v, t: tInt}); err != nil {
			return err
		}
	}
	return nil
}
