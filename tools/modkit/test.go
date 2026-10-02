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

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/server/zone"
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
	z.Progress, z.Status, z.Start = e.Tables, e.Jobs, e.Start
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
	return nil
}

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
	s, err := newSpec(what, v, "map", "x", "y", "job", "base_level", "job_level", "zeny", "items")
	if err != nil {
		return nil, err
	}
	tp := zone.TestPlayer{
		X: int(s.int("x", 0, true)), Y: int(s.int("y", 0, true)),
		Job: int(s.int("job", 0, false)), BaseLevel: int(s.int("base_level", 1, false)), JobLevel: int(s.int("job_level", 1, false)),
		Zeny: s.int("zeny", 0, false),
	}
	mapName := s.str("map")
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
	m, ok := w.env.Monsters[uint32(id)] // #nosec G115 -- a missing ID is refused below.
	if id <= 0 || id > 1<<32-1 || !ok {
		return nil, fmt.Errorf("%s: %d is not a monster; use monsters.id", what, id)
	}
	if err := w.bind(mapName); err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	name := ""
	for n, mid := range w.env.Core.Monsters {
		if mid == id && (name == "" || n < name) {
			name = n
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
