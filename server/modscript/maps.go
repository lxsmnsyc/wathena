package modscript

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/mob"
)

// Maps, areas and monsters of review decision 58, area B8 of proposal
// 0002b (decision 0065):
//   - spawning.monsters on any map, with a display name, a size, an AI, a
//     label and a rectangle, as rAthena's monster and areamonster.
//   - Monster methods: kill, remove, set_hp, set_damage_immune,
//     set_modes, set_speed, use_skill_at and warp_to.
//   - maps.count_players, count_monsters, remove_monsters, warp_players,
//     set_cells, check_cell, set_flag, get_flag, find_free_cell and
//     announce with a rectangle, and party:warp.
//   - monsters.get, as getmonsterinfo.
//
// A call on another map runs in that map's zone after the current step,
// as the NPC messages of decision 0062. Reads of another map come from
// what its zone shares: they show the state after its last step.

// MapHost runs the calls of area B8. A Host without it raises an error on
// them.
type MapHost interface {
	// CountPlayers counts the players of a map, or of a rectangle of it.
	// ok is false when the map does not run in this world.
	CountPlayers(mapName string, area *Rect) (n int, ok bool)
	// CountMonsters counts the living monsters of a map, or only those of
	// a label when label is not "". ok is false when the map does not
	// run in this world.
	CountMonsters(mapName, label string) (n int, ok bool)
	// RemoveMonsters kills monsters of a map, as killmonster and
	// killmonsterall.
	RemoveMonsters(mapName string, r RemoveMonsters) error
	// WarpPlayers moves the players of a map, as mapwarp and areawarp.
	WarpPlayers(mapName string, w WarpPlayers) error
	// SetCells changes a kind of cell in a rectangle, as setcell.
	SetCells(mapName string, area Rect, kind string, on bool) error
	// CheckCell reads a cell, as checkcell.
	CheckCell(mapName string, x, y int, kind string) (bool, error)
	// SetMapFlag and MapFlag are setmapflag, removemapflag and
	// getmapflag.
	SetMapFlag(mapName, flag string, on bool) error
	MapFlag(mapName, flag string) (bool, error)
	// FindFreeCell is getfreecell's map_search_freecell. ok is false
	// when no try found a cell.
	FindFreeCell(mapName string, q FreeCell) (x, y int, ok bool, err error)
	// AnnounceArea is areaannounce: a line to the players of a rectangle.
	AnnounceArea(mapName, mod, text string, area Rect) error
	// MonsterAct changes or acts with a monster of the zone.
	MonsterAct(monster int64, a MonsterAction) (bool, error)
	// MonsterInfo reads a monster's record, as getmonsterinfo.
	MonsterInfo(monster uint32) (MonsterInfo, bool)
	// PartyWarp moves the members of a party, as warpparty.
	PartyWarp(party int64, mapName string, x, y int) error
	// LabelOwner is the NPC handle of a label's NPC, by name, or 0.
	LabelOwner(name, mod string) int64
}

// Rect is a rectangle of cells, corners included, with X1 <= X2 and
// Y1 <= Y2.
type Rect struct{ X1, Y1, X2, Y2 int }

// Contains reports whether a cell is in the rectangle.
func (r Rect) Contains(x, y int) bool { return x >= r.X1 && x <= r.X2 && y >= r.Y1 && y <= r.Y2 }

// MonsterLabel is the label of spawned monsters, as rAthena's event
// label of monster: its NPC's event runs when each monster dies, and it
// names the monsters for count_monsters and remove_monsters.
type MonsterLabel struct {
	// Key is "<NPC's full record name>::<label>", or the name as given
	// for an NPC that does not exist.
	Key string
	// NPC is the NPC's handle, or 0, and Event the label in lower case.
	NPC   int64
	Event string
}

// RemoveMonsters is what maps.remove_monsters kills.
type RemoveMonsters struct {
	// Label kills only the monsters of a label, as killmonster. Scripted
	// kills only monsters that no spawn line placed, as killmonster's
	// "All". Without both, every monster dies, as killmonsterall.
	Label    string
	Scripted bool
	// RunLabels runs the labels of the monsters that die, as type 1.
	RunLabels bool
}

// WarpPlayers is what maps.warp_players moves.
type WarpPlayers struct {
	To   string
	X, Y int
	// Area limits the players to a rectangle, as areawarp. ToArea picks
	// a random cell of a rectangle of the target, as areawarp's second
	// area.
	Area, ToArea *Rect
	// Party and Guild limit the players to a party or a guild, as
	// mapwarp's types 2 and 1.
	Party, Guild int64
}

// FreeCell is a free cell search of maps.find_free_cell.
type FreeCell struct {
	// X and Y are the center, and RangeX and RangeY how far from it, or
	// -1 for the whole map.
	X, Y, RangeX, RangeY int
	// From is set for reachable_from: the cell must be reachable by
	// walking from the entity.
	From *Entity
}

// MonsterAction is a monster method of area B8.
type MonsterAction struct {
	// Op is kill, remove, set_hp, set_damage_immune, set_modes,
	// set_speed, use_skill_at or warp_to.
	Op     string
	Value  int64
	On     bool
	Modes  uint32
	Skill  int64
	Level  int64
	X, Y   int
	Labels bool
}

// MonsterInfo is a monster's record, as getmonsterinfo reads it.
type MonsterInfo struct {
	ID                                     uint32
	Name                                   string
	Level, HP, SP                          int
	BaseExp, JobExp, MvpExp                int64
	AttackMin, AttackMax                   int
	Defense, MagicDefense                  int
	Resistance, MagicResistance            int
	Str, Agi, Vit, Int, Dex, Luk           int
	Speed, AttackRange, SkillRange, Chase  int
	Size, Race, Element, ElementLevel      int
	Modes                                  uint32
	MagicAttackMin, MagicAttackMax, Damage int
}

// Sizes and AIs of spawning.monsters, as rAthena's SZ_ and AI_ values.
var (
	SpawnSizes = map[string]int{"normal": 0, "small": 1, "big": 2}
	SpawnAIs   = map[string]int{"none": 0, "attack": 1, "sphere": 2, "flora": 3, "zanzou": 4, "legion": 5,
		"faw": 6, "guild": 7, "wave_mode": 8, "abr": 9, "bionic": 10}
)

// Cell kinds of maps.set_cells and maps.check_cell.
var (
	// SetCellKinds are the kinds set_cells takes, as setcell's cell_
	// types.
	SetCellKinds = []string{"walkable", "shootable", "water", "npc", "basilica", "land_protector", "no_vending",
		"no_chat", "maelstrom", "ice_wall", "no_buying_store"}
	// CheckCellKinds are the kinds check_cell takes, as checkcell's
	// cell_chk types.
	CheckCellKinds = []string{"wall", "water", "cliff", "pass", "reach", "no_pass", "no_reach", "stack", "npc",
		"basilica", "land_protector", "no_vending", "no_chat", "maelstrom", "ice_wall", "no_buying_store"}
)

func (r *Runtime) mapHost(what string) (MapHost, error) {
	h, ok := r.host.(MapHost)
	if !ok {
		return nil, fmt.Errorf("%s: maps cannot be reached here", what)
	}
	return h, nil
}

// checkMap reads a map name argument.
func checkMap(what string, v script.Value) (string, error) {
	m, ok := v.AsString()
	if !ok || m == "" || len(m) > 64 {
		return "", errors.New(what + ": want a map name, such as prontera")
	}
	return strings.TrimPrefix(m, "map/"), nil
}

// readRect reads a rectangle, { { x1, y1 }, { x2, y2 } }, in any corner
// order.
func readRect(what, field string, v script.Value) (Rect, error) {
	bad := fmt.Errorf("%s: %s must be two corners, such as { { 100, 100 }, { 110, 105 } }", what, field)
	t, ok := v.AsTable()
	if !ok || len(t.Fields) != 2 {
		return Rect{}, bad
	}
	var c [2][2]int
	for i := range 2 {
		ct, ok := t.Index(int64(i + 1)).AsTable()
		if !ok || len(ct.Fields) != 2 {
			return Rect{}, bad
		}
		x, ok1 := ct.Index(1).AsInt()
		y, ok2 := ct.Index(2).AsInt()
		if !ok1 || !ok2 || x < -1<<15 || y < -1<<15 || x > 1<<15 || y > 1<<15 {
			return Rect{}, bad
		}
		c[i] = [2]int{int(x), int(y)}
	}
	return Rect{X1: min(c[0][0], c[1][0]), Y1: min(c[0][1], c[1][1]), X2: max(c[0][0], c[1][0]), Y2: max(c[0][1], c[1][1])}, nil
}

// walkOptions walks an options table: each key must be one of known.
func walkOptions(what string, v script.Value, known []string, f func(k string, v script.Value) error) error {
	if v.IsNil() {
		return nil
	}
	t, ok := v.AsTable()
	if !ok {
		return fmt.Errorf("%s: the options must be a table", what)
	}
	for _, fl := range t.Fields {
		k, _ := fl.Key.AsString()
		if !slices.Contains(known, k) {
			return fmt.Errorf("%s: there is no option %q; the options are %s", what, k, strings.Join(known, ", "))
		}
		if err := f(k, fl.Value); err != nil {
			return err
		}
	}
	return nil
}

// monsterLabel resolves a label, as rAthena's event label "NPC::OnX": a
// plain label names the context's own NPC. An NPC that does not exist
// keeps the name as given, and its monsters run no event, as rAthena
// only logs an error.
func (r *Runtime) monsterLabel(what string, v script.Value, h MapHost, control bool) (MonsterLabel, error) {
	s, ok := v.AsString()
	if !ok || s == "" || len(s) > 128 {
		return MonsterLabel{}, fmt.Errorf("%s: label must be an event label, such as OnMyMobDead, or \"<NPC>::OnMyMobDead\"", what)
	}
	name, label, other := strings.Cut(s, "::")
	if !other {
		name, label = "", s
	}
	if i := strings.LastIndex(s, "::"); i >= 0 {
		name, label = s[:i], s[i+2:]
	}
	if !labelName.MatchString(label) {
		return MonsterLabel{}, fmt.Errorf("%s: %q is not an event label: 1 to 50 letters, digits and _", what, label)
	}
	var npc int64
	if !other {
		ctx := r.current
		if ctx == nil || ctx.npc == 0 {
			return MonsterLabel{}, fmt.Errorf("%s: a label without an NPC's name needs an NPC context; write \"<NPC>::%s\"", what, label)
		}
		npc = ctx.npc
	} else {
		npc = h.LabelOwner(name, r.actor())
	}
	if npc == 0 {
		return MonsterLabel{Key: name + "::" + label}, nil
	}
	w, err := r.npcWorld(what)
	if err != nil {
		return MonsterLabel{}, err
	}
	key, err := w.NPCKey(npc)
	if err != nil {
		return MonsterLabel{}, fmt.Errorf("%s: %w", what, err)
	}
	if control {
		if err := r.controls(what, w, npc); err != nil {
			return MonsterLabel{}, err
		}
	}
	return MonsterLabel{Key: key + "::" + label, NPC: npc, Event: strings.ToLower(label)}, nil
}

// mapsCall is the maps functions of area B8: Arg(0) is the function's
// name, and the rest its arguments.
func (r *Runtime) mapsCall(c *script.Call) error {
	op := c.String(0)
	what := "maps." + op
	arg := func(i int) script.Value { return c.Arg(1 + i) }
	h, err := r.mapHost(what)
	if err != nil {
		return err
	}
	read := op == "count_players" || op == "count_monsters" || op == "check_cell" || op == "get_flag" || op == "find_free_cell"
	if read {
		if err := r.need(what, "game.read"); err != nil {
			return err
		}
	} else if err := r.mutating(what); err != nil {
		return err
	}
	m, err := checkMap(what, arg(0))
	if err != nil {
		return err
	}
	switch op {
	case "count_players":
		var area *Rect
		err := walkOptions(what, arg(1), []string{"rectangle"}, func(_ string, v script.Value) error {
			rc, err := readRect(what, "rectangle", v)
			area = &rc
			return err
		})
		if err != nil {
			return err
		}
		if n, ok := h.CountPlayers(m, area); ok {
			c.Return(script.Int(int64(n)))
		}
		return nil
	case "count_monsters":
		var label string
		err := walkOptions(what, arg(1), []string{"label"}, func(_ string, v script.Value) error {
			l, err := r.monsterLabel(what, v, h, false)
			label = l.Key
			return err
		})
		if err != nil {
			return err
		}
		if n, ok := h.CountMonsters(m, label); ok {
			c.Return(script.Int(int64(n)))
		}
		return nil
	case "remove_monsters":
		if err := r.need(what, "npc.create"); err != nil {
			return err
		}
		var rm RemoveMonsters
		err := walkOptions(what, arg(1), []string{"label", "scripted", "run_labels"}, func(k string, v script.Value) error {
			switch k {
			case "label":
				l, err := r.monsterLabel(what, v, h, false)
				rm.Label = l.Key
				return err
			case "scripted", "run_labels":
				b, ok := v.AsBool()
				if !ok {
					return fmt.Errorf("%s: %s must be true or false", what, k)
				}
				if k == "scripted" {
					rm.Scripted = b
				} else {
					rm.RunLabels = b
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
		if rm.Label != "" && rm.Scripted {
			return errors.New(what + ": give label or scripted, not both")
		}
		return wrap(what, h.RemoveMonsters(m, rm))
	case "warp_players":
		return r.mapsWarpPlayers(c, h, m)
	case "set_cells":
		return r.mapsSetCells(c, h, m)
	case "check_cell":
		x, y, err := checkCell(what, arg(1), arg(2))
		if err != nil {
			return err
		}
		kind, ok := arg(3).AsString()
		if !ok || !slices.Contains(CheckCellKinds, kind) {
			return fmt.Errorf("%s: the kind must be one of %s", what, strings.Join(CheckCellKinds, ", "))
		}
		on, err := h.CheckCell(m, x, y, kind)
		if err != nil {
			return wrap(what, err)
		}
		c.Return(script.Bool(on))
		return nil
	case "set_flag", "get_flag":
		flag, ok := arg(1).AsString()
		if !ok || flag == "" || len(flag) > 64 {
			return fmt.Errorf("%s: want a map flag, such as nomemo", what)
		}
		if op == "get_flag" {
			on, err := h.MapFlag(m, flag)
			if err != nil {
				return wrap(what, err)
			}
			c.Return(script.Bool(on))
			return nil
		}
		on, ok := arg(2).AsBool()
		if !ok {
			return fmt.Errorf("%s: the value must be true or false", what)
		}
		return wrap(what, h.SetMapFlag(m, flag, on))
	case "find_free_cell":
		return r.mapsFindFreeCell(c, h, m)
	case "announce_area":
		text, ok := arg(1).AsString()
		if !ok || text == "" || len(text) > MaxMessage || !utf8.ValidString(text) {
			return fmt.Errorf("maps.announce: the text must be 1 to %d bytes", MaxMessage)
		}
		area, err := readRect("maps.announce", "rectangle", arg(2))
		if err != nil {
			return err
		}
		return wrap("maps.announce", h.AnnounceArea(m, r.sender(), text, area))
	}
	return fmt.Errorf("maps has no function %q", op)
}

func (r *Runtime) mapsWarpPlayers(c *script.Call, h MapHost, m string) error {
	const what = "maps.warp_players"
	arg := func(i int) script.Value { return c.Arg(1 + i) }
	if err := r.need(what, "player.warp"); err != nil {
		return err
	}
	to, err := checkMap(what, arg(1))
	if err != nil {
		return err
	}
	x, y, err := checkCell(what, arg(2), arg(3))
	if err != nil {
		return err
	}
	w := WarpPlayers{To: to, X: x, Y: y}
	err = walkOptions(what, arg(4), []string{"rectangle", "to_rectangle", "party", "guild"}, func(k string, v script.Value) error {
		switch k {
		case "rectangle", "to_rectangle":
			rc, err := readRect(what, k, v)
			if err != nil {
				return err
			}
			if k == "rectangle" {
				w.Area = &rc
			} else {
				w.ToArea = &rc
			}
		case "party", "guild":
			n, ok := v.AsInt()
			if !ok || n <= 0 {
				return fmt.Errorf("%s: %s must be a %s's ID, such as player.%s.id", what, k, k, k)
			}
			if k == "party" {
				w.Party = n
			} else {
				w.Guild = n
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if w.Party != 0 && w.Guild != 0 {
		return errors.New(what + ": give party or guild, not both")
	}
	return wrap(what, h.WarpPlayers(m, w))
}

func (r *Runtime) mapsSetCells(c *script.Call, h MapHost, m string) error {
	const what = "maps.set_cells"
	arg := func(i int) script.Value { return c.Arg(1 + i) }
	if err := r.need(what, "npc.create"); err != nil {
		return err
	}
	x1, y1, err := checkCell(what, arg(1), arg(2))
	if err != nil {
		return err
	}
	x2, y2, err := checkCell(what, arg(3), arg(4))
	if err != nil {
		return err
	}
	kind, ok := arg(5).AsString()
	if !ok || !slices.Contains(SetCellKinds, kind) {
		return fmt.Errorf("%s: the kind must be one of %s", what, strings.Join(SetCellKinds, ", "))
	}
	on, ok := arg(6).AsBool()
	if !ok {
		return fmt.Errorf("%s: on must be true or false", what)
	}
	area := Rect{X1: min(x1, x2), Y1: min(y1, y2), X2: max(x1, x2), Y2: max(y1, y2)}
	return wrap(what, h.SetCells(m, area, kind, on))
}

func (r *Runtime) mapsFindFreeCell(c *script.Call, h MapHost, m string) error {
	const what = "maps.find_free_cell"
	q := FreeCell{RangeX: -1, RangeY: -1}
	err := walkOptions(what, c.Arg(2), []string{"x", "y", "range_x", "range_y", "reachable_from"}, func(k string, v script.Value) error {
		if k == "reachable_from" {
			e, ok := parseEntityKey(v)
			if !ok {
				return fmt.Errorf("%s: reachable_from must be a player, monster or NPC handle", what)
			}
			q.From = &e
			return nil
		}
		n, ok := v.AsInt()
		if !ok || n < -1 || n > 1023 {
			return fmt.Errorf("%s: %s must be an integer from -1 to 1023", what, k)
		}
		switch k {
		case "x":
			q.X = int(n)
		case "y":
			q.Y = int(n)
		case "range_x":
			q.RangeX = int(n)
		case "range_y":
			q.RangeY = int(n)
		}
		return nil
	})
	if err != nil {
		return err
	}
	x, y, ok, err := h.FindFreeCell(m, q)
	if err != nil {
		return wrap(what, err)
	}
	if ok {
		c.Return(script.Int(int64(x)), script.Int(int64(y)))
	}
	return nil
}

// monsterCall8 is the monster methods of area B8: Arg(0) is the
// method's name, Arg(1) the monster, and the rest its arguments.
func (r *Runtime) monsterCall8(c *script.Call) error {
	op := c.String(0)
	what := "monster:" + op
	id, ok := c.Arg(1).AsInt()
	if !ok {
		return fmt.Errorf("%s: not a monster: call the method with a colon", what)
	}
	if err := r.mutating(what); err != nil {
		return err
	}
	h, err := r.mapHost(what)
	if err != nil {
		return err
	}
	arg := func(i int) script.Value { return c.Arg(2 + i) }
	a := MonsterAction{Op: op}
	switch op {
	case "kill", "remove":
	case "set_hp":
		n, ok := arg(0).AsInt()
		if !ok || n < 0 || n > 1<<31-1 {
			return fmt.Errorf("%s: want an HP from 0", what)
		}
		a.Value = n
	case "set_damage_immune":
		b, ok := arg(0).AsBool()
		if !ok {
			return fmt.Errorf("%s: want true or false", what)
		}
		a.On = b
	case "set_modes":
		t, ok := arg(0).AsTable()
		if !ok {
			return fmt.Errorf("%s: want a list of modes, such as { \"can_move\", \"aggressive\" }", what)
		}
		for i := int64(1); i <= int64(len(t.Fields)); i++ {
			s, ok := t.Index(i).AsString()
			bit, known := mob.ModeNames[s]
			if !ok || !known {
				return fmt.Errorf("%s: %q is not a monster mode; the modes are the names of data/monsters.yml, such as can_move", what, s)
			}
			a.Modes |= bit
		}
	case "set_speed":
		n, ok := arg(0).AsInt()
		if !ok || n < 20 || n > 1000 {
			return fmt.Errorf("%s: want milliseconds per cell from 20 to 1000", what)
		}
		a.Value = n
	case "use_skill_at":
		skill, ok1 := arg(0).AsInt()
		level, ok2 := arg(1).AsInt()
		if !ok1 || !ok2 || skill <= 0 || level < 1 || level > MaxMonsterSkillLevel {
			return fmt.Errorf("%s: want a skill from skills.id, a level from 1 to %d, and a cell", what, MaxMonsterSkillLevel)
		}
		x, y, err := checkCell(what, arg(2), arg(3))
		if err != nil {
			return err
		}
		a.Skill, a.Level, a.X, a.Y = skill, level, x, y
	case "warp_to":
		x, y, err := checkCell(what, arg(0), arg(1))
		if err != nil {
			return err
		}
		a.X, a.Y = x, y
	default:
		return fmt.Errorf("monster has no method %q", op)
	}
	done, err := h.MonsterAct(id, a)
	if err != nil {
		return wrap(what, err)
	}
	if op == "set_hp" || op == "use_skill_at" || op == "warp_to" {
		c.Return(script.Bool(done))
	}
	return nil
}

// monstersGet is monsters.get, as getmonsterinfo.
func (r *Runtime) monstersGet(c *script.Call) error {
	const what = "monsters.get"
	id, ok := c.Arg(0).AsInt()
	if !ok || id <= 0 || id > 1<<32-1 {
		return errors.New(what + ": want a monster from monsters.id")
	}
	h, err := r.mapHost(what)
	if err != nil {
		return err
	}
	in, found := h.MonsterInfo(uint32(id)) // #nosec G115 -- checked above.
	if !found {
		return nil
	}
	var modes []script.Value
	names := make([]string, 0, len(mob.ModeNames))
	for n := range mob.ModeNames {
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b string) int { return int(mob.ModeNames[a]) - int(mob.ModeNames[b]) })
	for _, n := range names {
		if in.Modes&mob.ModeNames[n] != 0 {
			modes = append(modes, script.String(n))
		}
	}
	i := func(n int) script.Value { return script.Int(int64(n)) }
	c.Return(script.NewTable(
		script.KV("id", script.Int(int64(in.ID))), script.KV("name", script.String(in.Name)),
		script.KV("level", i(in.Level)), script.KV("hp", i(in.HP)), script.KV("sp", i(in.SP)),
		script.KV("base_exp", script.Int(in.BaseExp)), script.KV("job_exp", script.Int(in.JobExp)), script.KV("mvp_exp", script.Int(in.MvpExp)),
		script.KV("attack_min", i(in.AttackMin)), script.KV("attack_max", i(in.AttackMax)),
		script.KV("magic_attack_min", i(in.MagicAttackMin)), script.KV("magic_attack_max", i(in.MagicAttackMax)),
		script.KV("defense", i(in.Defense)), script.KV("magic_defense", i(in.MagicDefense)),
		script.KV("resistance", i(in.Resistance)), script.KV("magic_resistance", i(in.MagicResistance)),
		script.KV("str", i(in.Str)), script.KV("agi", i(in.Agi)), script.KV("vit", i(in.Vit)),
		script.KV("int", i(in.Int)), script.KV("dex", i(in.Dex)), script.KV("luk", i(in.Luk)),
		script.KV("speed", i(in.Speed)), script.KV("attack_range", i(in.AttackRange)),
		script.KV("skill_range", i(in.SkillRange)), script.KV("chase_range", i(in.Chase)),
		script.KV("size", i(in.Size)), script.KV("race", i(in.Race)), script.KV("element", i(in.Element)),
		script.KV("element_level", i(in.ElementLevel)), script.KV("damage_taken", i(in.Damage)),
		script.KV("modes", script.List(modes...)),
	))
	return nil
}

// partyWarp is party:warp, as warpparty to a map and a cell.
func (r *Runtime) partyWarp(c *script.Call) error {
	const what = "party:warp"
	if err := r.mutating(what); err != nil {
		return err
	}
	if err := r.need(what, "player.warp"); err != nil {
		return err
	}
	id, ok := c.Arg(0).AsInt()
	if !ok {
		return errors.New(what + ": not a party: call the method with a colon")
	}
	m, err := checkMap(what, c.Arg(1))
	if err != nil {
		return err
	}
	x, y, err := checkCell(what, c.Arg(2), c.Arg(3))
	if err != nil {
		return err
	}
	h, err := r.mapHost(what)
	if err != nil {
		return err
	}
	return wrap(what, h.PartyWarp(id, m, x, y))
}
