package modkit

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
	// The time zone database, for world:set_time on systems without
	// one.
	_ "time/tzdata"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/server/worldevent"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Tests of wactl mod test (mods/README.md, "Testing a mod"):
//   - Test files are tests/*_test.lua. Each returns a function that
//     registers cases with testing.case.
//   - Each case runs in a fresh headless world: the mod, its
//     dependencies and core loaded, a fixed random seed, and no
//     database.
//   - A case fails on an error in its code, and on a warning or error
//     that the server logs about the mod while it runs, such as a
//     handler that failed.

// TestPattern is where test files are, in a mod's folder.
const TestPattern = "tests/*_test.lua"

// Seed is the random seed of every test world.
var Seed = sha256.Sum256([]byte("wactl mod test"))

// CaseResult is the result of one test case.
type CaseResult struct {
	File, Name string
	// Err is why the case failed, or nil.
	Err error
	// Logs are the server's warnings about the mod while the case ran.
	Logs []string
	Took time.Duration
}

// Failed reports whether the case failed.
func (r CaseResult) Failed() bool { return r.Err != nil || len(r.Logs) > 0 }

// TestFiles returns the mod's test files with their source, sorted by
// name.
func TestFiles(m *mod.Mod) (map[string]string, []string, error) {
	fsys, closeFS, err := m.Open()
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = closeFS() }()
	names, err := fs.Glob(fsys, TestPattern)
	if err != nil {
		return nil, nil, err
	}
	slices.Sort(names)
	out := map[string]string{}
	for _, n := range names {
		b, err := fs.ReadFile(fsys, n)
		if err != nil {
			return nil, nil, err
		}
		out[n] = string(b)
	}
	return out, names, nil
}

// RunTests runs every case of every test file of the loaded mod. match,
// when not empty, keeps the cases whose name contains it. Each result
// goes to report as it ends.
func (e *Env) RunTests(ctx context.Context, l *Loaded, match string, report func(CaseResult)) ([]CaseResult, error) {
	files, names, err := TestFiles(l.Mod)
	if err != nil {
		return nil, err
	}
	var out []CaseResult
	add := func(r CaseResult) {
		out = append(out, r)
		report(r)
	}
	for _, file := range names {
		// The first world registers the cases and runs the first one.
		// Each other case gets a world of its own.
		res, cases, ran := e.runCase(l, file, files[file], 0, match)
		if cases == nil {
			res.Name = "(loading the file)"
			add(res)
			continue
		}
		if ran {
			add(res)
		}
		for i := 1; i < len(cases); i++ {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			if match != "" && !strings.Contains(cases[i], match) {
				continue
			}
			res, _, _ := e.runCase(l, file, files[file], i, match)
			add(res)
		}
	}
	return out, nil
}

// runCase makes a world, loads a test file in it, and runs case i when
// its name matches. It returns the names of the file's cases, or nil
// when the file did not load, and whether the case ran.
func (e *Env) runCase(l *Loaded, file, source string, i int, match string) (CaseResult, []string, bool) {
	began := time.Now()
	res := CaseResult{File: file}
	logs := &capture{}
	w, err := e.newWorld(l, slog.New(logs))
	if err != nil {
		res.Err = err
		return res, nil, false
	}
	defer w.h.Close()
	r := w.h.Runtime()
	names, err := r.LoadTests(l.Mod.ID(), file, source)
	if err != nil {
		res.Err = plain(err)
		return res, nil, false
	}
	if names == nil {
		names = []string{}
	}
	if i >= len(names) || match != "" && !strings.Contains(names[i], match) {
		return res, names, false
	}
	res.Name = names[i]
	res.Err = plain(r.RunCase(l.Mod.ID(), i, w))
	res.Logs = logs.about(l.Mod.ID())
	res.Took = time.Since(began)
	return res, names, true
}

// testWorld is one test case's world.
type testWorld struct {
	env *Env
	l   *Loaded
	h   *zone.Headless
	// players counts the players, for their names.
	players int
	// events runs the world events of the world's map.
	events *worldevent.Scheduler
}

// newWorld makes a headless world with the loaded mods.
func (e *Env) newWorld(l *Loaded, log *slog.Logger) (*testWorld, error) {
	mods := *l.Zone
	mods.Testing = true
	mods.Log = log
	mods.Server = variables.NewServer(nil, log)
	id := l.Mod.ID()
	if !slices.ContainsFunc(mods.Scripts, func(s *mod.Scripts) bool { return s.Mod == id }) {
		// A mod without Lua files still loads its tests.
		sc, err := l.Set.Scripts(id)
		if err != nil {
			return nil, err
		}
		mods.Scripts = append(slices.Clone(mods.Scripts), sc)
	}
	h, err := zone.NewHeadless(&mods, Seed)
	if err != nil {
		return nil, err
	}
	z := h.Zone()
	z.Items = l.itemTable()
	z.Progress, z.Status, z.Start, z.Statuses = e.Tables, e.Jobs, e.Start, e.Statuses
	z.Skills, z.SkillTrees, z.SkillSettings = l.Skills, l.SkillTrees, skill.Settings{}
	if l.Tables != nil {
		z.Progress = l.Tables
	}
	if l.JobTables != nil {
		z.Status = l.JobTables
	}
	return &testWorld{env: e, l: l, h: h}, nil
}

var tables sync.Map // *Loaded -> *item.Table

func (l *Loaded) itemTable() *item.Table {
	if t, ok := tables.Load(l); ok {
		return t.(*item.Table)
	}
	t := item.New(l.Items.Data)
	tables.Store(l, t)
	return t
}

// DefaultMap is the map of a world whose case calls advance before
// anything names a map.
const DefaultMap = "prontera"

// bind gives the world its map, the first time a call names one, and
// checks that later calls name the same map.
func (w *testWorld) bind(name string) error {
	if name == "" {
		name = DefaultMap
		if m, ok := w.h.Bound(); ok {
			name = strings.TrimPrefix(m, "map/")
		}
	}
	asset := "map/" + strings.TrimPrefix(name, "map/")
	if m, ok := w.h.Bound(); ok {
		if m != asset {
			return fmt.Errorf("a test world runs one map, and this case runs %s, not %s", strings.TrimPrefix(m, "map/"), name)
		}
		return nil
	}
	npcs := w.l.NPCs[asset]
	grid, err := w.env.Grid(asset, npcs)
	if err != nil {
		return err
	}
	if err := w.h.Bind(asset, grid, npcs, w.env.Warps[asset]); err != nil {
		return fmt.Errorf("the mods' entry scripts failed on %s: %w", name, err)
	}
	return w.startEvents(asset)
}

// startEvents starts the world event scheduler for the events of the
// world's map, and of no map, on the world's clock. It runs after
// every tick.
func (w *testWorld) startEvents(asset string) error {
	var defs []worldevent.Def
	for _, d := range w.l.Events {
		if d.Map == "" || d.Map == asset {
			defs = append(defs, d)
		}
	}
	if len(defs) == 0 {
		return nil
	}
	seed := sha256.Sum256(append(Seed[:], "events"...))
	w.events = worldevent.New(defs, worldevent.Options{Runner: headlessRunner{w.h}, Location: w.h.Location(),
		Rand: rng.FromKey(seed).Stream(), Log: w.h.Zone().Mods.Log})
	ctx := context.Background()
	if err := w.events.Load(ctx, w.h.Now()); err != nil {
		return err
	}
	w.h.AfterStep = func() { w.events.Check(ctx, w.h.Now()) }
	w.h.AfterStep()
	return nil
}

// headlessRunner runs world events in a test world's zone.
type headlessRunner struct{ h *zone.Headless }

func (r headlessRunner) Start(_ context.Context, d worldevent.Def, run worldevent.Run) error {
	return r.h.StartEvent(zone.EventRun{ID: run.ID, Name: d.Name, Entry: d.Script})
}

func (r headlessRunner) End(_ context.Context, _ worldevent.Def, run worldevent.Run) error {
	r.h.EndEvent(run.ID)
	return nil
}

func (r headlessRunner) Condition(_ context.Context, d worldevent.Def) (bool, error) {
	return r.h.EventCondition(d.Mod, d.Trigger.Module, d.Trigger.Function)
}

// timeLayout is how world:set_time and world:get_time write times.
const timeLayout = "2006-01-02 15:04"

// Do runs a world method of a test case.
func (w *testWorld) Do(op string, args []script.Value) ([]script.Value, error) {
	switch op {
	case "new_player":
		return w.newPlayer(args[0])
	case "talk":
		var answers []script.Value
		if t, ok := args[2].AsTable(); ok {
			for i := int64(1); ; i++ {
				v := t.Index(i)
				if v.IsNil() {
					break
				}
				answers = append(answers, v)
			}
			if len(answers) != len(t.Fields) {
				return nil, errors.New("world:talk: the answers must be a list, such as { 1, \"Bob\" }")
			}
		}
		id, _ := args[0].AsInt()
		name, _ := args[1].AsString()
		text, err := w.h.Talk(id, name, w.l.Mod.ID(), answers)
		if err != nil {
			return nil, fmt.Errorf("world:talk: %w", err)
		}
		return []script.Value{script.String(text)}, nil
	case "use_item":
		id, _ := args[0].AsInt()
		it, _ := args[1].AsInt()
		if it <= 0 || it > 1<<32-1 {
			return nil, errors.New("world:use_item: want an item from items.id")
		}
		if err := w.h.UseItem(id, uint32(it)); err != nil {
			return nil, fmt.Errorf("world:use_item: %w", err)
		}
		return nil, nil
	case "equip":
		id, _ := args[0].AsInt()
		it, _ := args[1].AsInt()
		if it <= 0 || it > 1<<32-1 {
			return nil, errors.New("world:equip: want an item from items.id")
		}
		rollArg := script.Nil
		if len(args) > 2 {
			rollArg = args[2]
		}
		roll, err := modscript.ReadGiveRoll("world:equip", rollArg)
		if err != nil {
			return nil, err
		}
		h, err := w.h.Equip(id, uint32(it), roll)
		if err != nil {
			return nil, fmt.Errorf("world:equip: %w", err)
		}
		return []script.Value{script.Int(h)}, nil
	case "advance":
		ms, _ := args[0].AsInt()
		if _, ok := w.h.Bound(); !ok {
			if err := w.bind(""); err != nil {
				return nil, fmt.Errorf("world:advance: %w", err)
			}
		}
		w.h.Advance(ms)
		return nil, nil
	case "spawn_monster":
		return w.spawnMonster(args[0])
	case "set_time":
		return nil, w.setTime(args)
	case "get_time":
		return []script.Value{script.String(w.h.Now().In(w.h.Location()).Format(timeLayout))}, nil
	case "get_monsters":
		var id int64
		if len(args) > 0 && !args[0].IsNil() {
			id, _ = args[0].AsInt()
			if id <= 0 || id > 1<<32-1 {
				return nil, errors.New("world:get_monsters: want a monster from monsters.id")
			}
		}
		if _, ok := w.h.Bound(); !ok {
			return []script.Value{script.List()}, nil
		}
		var list []script.Value
		for _, h := range w.h.Monsters(uint32(id)) {
			list = append(list, script.Int(h))
		}
		return []script.Value{script.List(list...)}, nil
	case "get_announcements":
		var list []script.Value
		for _, a := range w.h.Announcements() {
			list = append(list, script.String(a))
		}
		return []script.Value{script.List(list...)}, nil
	case "count_floor_items":
		id, _ := args[0].AsInt()
		if id <= 0 || id > 1<<32-1 {
			return nil, errors.New("world:count_floor_items: want an item from items.id")
		}
		if _, ok := w.h.Bound(); !ok {
			return []script.Value{script.Int(0)}, nil
		}
		return []script.Value{script.Int(int64(w.h.FloorItems(uint32(id))))}, nil
	case "chat":
		p, _ := args[0].AsInt()
		text, _ := args[1].AsString()
		channel, _ := args[2].AsString()
		to, _ := args[3].AsString()
		if err := w.h.Say(p, channel, text, to); err != nil {
			return nil, fmt.Errorf("world:chat: %w", err)
		}
		return nil, nil
	case "trade":
		a, _ := args[0].AsInt()
		b, _ := args[1].AsInt()
		offers := [2]zone.TradeOffer{}
		for i := range offers {
			t, ok := args[2+i].AsTable()
			if !ok {
				continue
			}
			offers[i].Items = map[uint32]int{}
			if items, ok := t.Get("items").AsTable(); ok {
				for _, f := range items.Fields {
					id, okID := f.Key.AsInt()
					n, okN := f.Value.AsInt()
					if !okID || !okN || id <= 0 || id > 1<<32-1 || n < 1 || n > 30000 {
						return nil, errors.New("world:trade: items maps item IDs to amounts from 1 to 30000")
					}
					offers[i].Items[uint32(id)] += int(n)
				}
			}
			if z, ok := t.Get("zeny").AsInt(); ok {
				offers[i].Zeny = z
			}
		}
		ok, why, err := w.h.Trade(a, b, offers[0], offers[1])
		if err != nil {
			return nil, fmt.Errorf("world:trade: %w", err)
		}
		if ok {
			return []script.Value{script.Bool(true)}, nil
		}
		return []script.Value{script.Bool(false), script.String(why)}, nil
	case "close_storage":
		p, _ := args[0].AsInt()
		if err := w.h.CloseStorage(p); err != nil {
			return nil, fmt.Errorf("world:close_storage: %w", err)
		}
		return nil, nil
	case "get_chat":
		p, _ := args[0].AsInt()
		var list []script.Value
		for _, l := range w.h.Heard(p) {
			list = append(list, script.NewTable(script.KV("channel", script.String(l.Channel)),
				script.KV("from", script.String(l.From)), script.KV("text", script.String(l.Text))))
		}
		return []script.Value{script.List(list...)}, nil
	case "kill":
		p, _ := args[0].AsInt()
		m, _ := args[1].AsInt()
		if err := w.h.Kill(p, m); err != nil {
			return nil, fmt.Errorf("world:kill: %w", err)
		}
		return nil, nil
	}
	return nil, fmt.Errorf("the test world has no method %q", op)
}

// setTime is world:set_time(time, time_zone): the server time and time
// zone the world starts at.
func (w *testWorld) setTime(args []script.Value) error {
	const what = "world:set_time"
	if _, ok := w.h.Bound(); ok {
		return errors.New(what + ": call it before the world has a map: before new_player, spawn_monster and advance")
	}
	text, _ := args[0].AsString()
	loc := w.h.Location()
	if len(args) > 1 && !args[1].IsNil() {
		name, _ := args[1].AsString()
		l, err := time.LoadLocation(name)
		if err != nil || name == "" || name == "Local" {
			return fmt.Errorf("%s: %q is not an IANA time zone name, such as Asia/Manila", what, name)
		}
		loc = l
	}
	t, err := time.ParseInLocation(timeLayout, text, loc)
	if err != nil {
		return fmt.Errorf("%s: %q is not a time such as \"2026-10-31 19:58\"", what, text)
	}
	return w.h.SetClock(t, loc)
}

// spec reads a table of named fields, and refuses fields it does not
// know.
type spec struct {
	what string
	t    *script.Table
	errs []string
}

func newSpec(what string, v script.Value, allowed ...string) (*spec, error) {
	t, ok := v.AsTable()
	if !ok {
		return nil, fmt.Errorf("%s: want a table", what)
	}
	for _, f := range t.Fields {
		k, ok := f.Key.AsString()
		if !ok || !slices.Contains(allowed, k) {
			return nil, fmt.Errorf("%s: %s is not a field; the fields are %s", what, f.Key, strings.Join(allowed, ", "))
		}
	}
	return &spec{what: what, t: t}, nil
}

func (s *spec) int(key string, def int64, required bool) int64 {
	v := s.t.Get(key)
	if v.IsNil() {
		if required {
			s.errs = append(s.errs, key+" is required")
		}
		return def
	}
	n, ok := v.AsInt()
	if !ok {
		s.errs = append(s.errs, key+" must be an integer")
	}
	return n
}

func (s *spec) str(key string) string {
	v := s.t.Get(key)
	if v.IsNil() {
		return ""
	}
	str, ok := v.AsString()
	if !ok {
		s.errs = append(s.errs, key+" must be a string")
	}
	return str
}

func (s *spec) err() error {
	if len(s.errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %s", s.what, strings.Join(s.errs, "; "))
}

func (w *testWorld) newPlayer(v script.Value) ([]script.Value, error) {
	const what = "world:new_player"
	s, err := newSpec(what, v, "map", "x", "y", "job", "base_level", "job_level", "zeny", "items", "sex", "skills", "stats",
		"skill_points", "status_points")
	if err != nil {
		return nil, err
	}
	tp := zone.TestPlayer{
		X: int(s.int("x", 0, true)), Y: int(s.int("y", 0, true)),
		Job: int(s.int("job", 0, false)), BaseLevel: int(s.int("base_level", 1, false)), JobLevel: int(s.int("job_level", 1, false)),
		Zeny: s.int("zeny", 0, false),
	}
	mapName := s.str("map")
	switch sex := s.str("sex"); sex {
	case "", "male":
	case "female":
		tp.Female = true
	default:
		return nil, fmt.Errorf(`%s: sex must be "male" or "female"`, what)
	}
	for _, f := range []struct {
		name string
		dst  **int
	}{{"skill_points", &tp.SkillPoints}, {"status_points", &tp.StatusPoints}} {
		if v := s.t.Get(f.name); !v.IsNil() {
			n, ok := v.AsInt()
			if !ok || n < 0 || n > 1<<20 {
				return nil, fmt.Errorf("%s: %s must be an integer from 0", what, f.name)
			}
			x := int(n)
			*f.dst = &x
		}
	}
	if skills := s.t.Get("skills"); !skills.IsNil() {
		list, ok := skills.AsTable()
		if !ok {
			return nil, fmt.Errorf("%s: skills must be a list of { skill, level }", what)
		}
		tp.Skills = map[int]int{}
		for i := int64(1); i <= int64(len(list.Fields)); i++ {
			pair, ok := list.Index(i).AsTable()
			if !ok {
				return nil, fmt.Errorf("%s: skills[%d] must be { skill, level }", what, i)
			}
			id, ok1 := pair.Index(1).AsInt()
			lv, ok2 := pair.Index(2).AsInt()
			if !ok1 || !ok2 || id <= 0 || id > 1<<20 || lv < 1 || lv > 1000 {
				return nil, fmt.Errorf("%s: skills[%d] must be { skill, level }, with a skill from skills.id and a level from 1", what, i)
			}
			tp.Skills[int(id)] = int(lv)
		}
	}
	if stats := s.t.Get("stats"); !stats.IsNil() {
		t, ok := stats.AsTable()
		if !ok {
			return nil, fmt.Errorf("%s: stats must be a table such as { str = 50 }", what)
		}
		st := status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}
		for _, f := range []struct {
			name string
			dst  *int
		}{{"str", &st.Str}, {"agi", &st.Agi}, {"vit", &st.Vit}, {"int", &st.Int}, {"dex", &st.Dex}, {"luk", &st.Luk}} {
			if v := t.Get(f.name); !v.IsNil() {
				n, ok := v.AsInt()
				if !ok || n < 1 || n > 1000 {
					return nil, fmt.Errorf("%s: stats.%s must be an integer from 1", what, f.name)
				}
				*f.dst = int(n)
			}
		}
		tp.Stats = &st
	}
	if items := s.t.Get("items"); !items.IsNil() {
		list, ok := items.AsTable()
		if !ok {
			return nil, fmt.Errorf("%s: items must be a list of { item, amount }", what)
		}
		for i := int64(1); i <= int64(len(list.Fields)); i++ {
			pair, ok := list.Index(i).AsTable()
			if !ok {
				return nil, fmt.Errorf("%s: items[%d] must be { item, amount }", what, i)
			}
			id, ok1 := pair.Index(1).AsInt()
			amount, ok2 := pair.Index(2).AsInt()
			if !ok1 || !ok2 || id <= 0 || id > 1<<32-1 || amount < 1 || amount > item.MaxAmount {
				return nil, fmt.Errorf("%s: items[%d] must be { item, amount }, with an item from items.id and an amount from 1 to %d", what, i, item.MaxAmount)
			}
			tp.Items = append(tp.Items, zone.StartItem{ID: uint32(id), Amount: int(amount)})
		}
	}
	if err := s.err(); err != nil {
		return nil, err
	}
	if err := w.bind(mapName); err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	w.players++
	tp.Name = fmt.Sprintf("Tester %d", w.players)
	id, err := w.h.AddPlayer(tp)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return []script.Value{script.Int(id)}, nil
}

func (w *testWorld) spawnMonster(v script.Value) ([]script.Value, error) {
	const what = "world:spawn_monster"
	s, err := newSpec(what, v, "monster", "map", "x", "y")
	if err != nil {
		return nil, err
	}
	id := s.int("monster", 0, true)
	x, y := int(s.int("x", 0, true)), int(s.int("y", 0, true))
	mapName := s.str("map")
	if err := s.err(); err != nil {
		return nil, err
	}
	monsters := w.l.Monsters
	if monsters == nil {
		monsters = w.env.Monsters
	}
	m, ok := monsters[uint32(id)] // #nosec G115 -- a missing ID is refused below.
	if id <= 0 || id > 1<<32-1 || !ok {
		return nil, fmt.Errorf("%s: %d is not a monster; use monsters.id", what, id)
	}
	if err := w.bind(mapName); err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	name := m.Name
	if name == "" {
		for n, mid := range w.env.Core.Monsters {
			if mid == id && (name == "" || n < name) {
				name = n
			}
		}
	}
	h, err := w.h.SpawnMonster(m.Spawn(uint32(id), name, x, y))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	return []script.Value{script.Int(h)}, nil
}

// capture keeps the warnings and errors the server logs.
type capture struct {
	mu      sync.Mutex
	records []string
}

func (c *capture) Enabled(_ context.Context, l slog.Level) bool { return l >= slog.LevelWarn }

func (c *capture) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, "; %s: %s", a.Key, a.Value)
		return true
	})
	c.mu.Lock()
	c.records = append(c.records, b.String())
	c.mu.Unlock()
	return nil
}

func (c *capture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *capture) WithGroup(string) slog.Handler      { return c }

// about returns the records that name a mod.
func (c *capture) about(id string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, r := range c.records {
		if strings.Contains(r, id) {
			out = append(out, r)
		}
	}
	return out
}

// plain returns a script error as its Lua message, which starts with
// the file and line. Limits keep their own text.
func plain(err error) error {
	var se *script.Error
	if errors.As(err, &se) && !errors.Is(err, script.ErrCPULimit) && !errors.Is(err, script.ErrMemoryLimit) && !errors.Is(err, script.ErrTimeLimit) {
		return errors.New(se.Message)
	}
	return err
}
