package convert

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/status"
	"go.yaml.in/yaml/v3"
)

var update = flag.Bool("update", false, "write the golden files and reports")

var cores = map[reference.Mode]func() (*Core, error){
	reference.Renewal:    sync.OnceValues(func() (*Core, error) { return LoadCore(reference.Renewal) }),
	reference.PreRenewal: sync.OnceValues(func() (*Core, error) { return LoadCore(reference.PreRenewal) }),
}

func loadCore(t *testing.T, mode reference.Mode) *Core {
	t.Helper()
	c, err := cores[mode]()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// testHost is a zone with one player, 1, whose stats tests set, and
// equipped copies by handle ID. It records nothing: the tests read the
// bonuses.
type testHost struct {
	job, baseLevel, jobLevel int
	stats                    status.Stats
	jobs                     *status.Tables
	items                    map[string]uint32
	groups                   map[string]int64
	monsters                 map[string]int64
	equipped                 map[int64]uint32
}

func (h *testHost) Map() string { return "prontera" }
func (h *testHost) PlayerField(id int64, field string) (script.Value, error) {
	switch field {
	case "base_level":
		return script.Int(int64(h.baseLevel)), nil
	case "job_level":
		return script.Int(int64(h.jobLevel)), nil
	case "job":
		return script.Int(int64(h.job)), nil
	case "base_str":
		return script.Int(int64(h.stats.Str)), nil
	case "base_agi":
		return script.Int(int64(h.stats.Agi)), nil
	case "base_vit":
		return script.Int(int64(h.stats.Vit)), nil
	case "base_int":
		return script.Int(int64(h.stats.Int)), nil
	case "base_dex":
		return script.Int(int64(h.stats.Dex)), nil
	case "base_luk":
		return script.Int(int64(h.stats.Luk)), nil
	case "base_pow", "base_sta", "base_wis", "base_spl", "base_con", "base_crt", "hp", "sp", "zeny":
		return script.Int(0), nil
	case "max_hp", "max_sp":
		return script.Int(1000), nil
	case "job_class":
		m, _ := h.jobs.MapID(h.job)
		return script.Int(int64(m)), nil
	case "name":
		return script.String("Tester"), nil
	}
	return script.Nil, fmt.Errorf("player has no field %q", field)
}
func (h *testHost) MonsterField(int64, string) (script.Value, error) {
	return script.Nil, errors.New("no monsters")
}
func (h *testHost) SendMessage(int64, string) error                    { return nil }
func (h *testHost) GiveItem(int64, uint32, int, int64) (bool, error)   { return true, nil }
func (h *testHost) GiveGroup(int64, int64) error                       { return nil }
func (h *testHost) CountItem(int64, uint32) (int, error)               { return 0, nil }
func (h *testHost) Heal(int64, bool, [2]int, [2]int) error             { return nil }
func (h *testHost) Warp(int64, string, string, int, int) (bool, error) { return true, nil }
func (h *testHost) ShowEffect(int64, int64, bool) error                { return nil }
func (h *testHost) SkillLevel(int64, int64) (int, error)               { return 0, nil }
func (h *testHost) Equipped(int64, string) (int64, bool, error)        { return 0, false, nil }
func (h *testHost) RandomN(int64) int64                                { return 0 }
func (h *testHost) Now() int64                                         { return 0 }
func (h *testHost) ItemID(name string) (uint32, bool)                  { id, ok := h.items[name]; return id, ok }
func (h *testHost) ItemGroupID(name string) (int64, bool)              { id, ok := h.groups[name]; return id, ok }
func (h *testHost) MonsterID(name string) (int64, bool)                { id, ok := h.monsters[name]; return id, ok }
func (h *testHost) EquippedField(id int64, field string) (script.Value, error) {
	item, ok := h.equipped[id]
	if !ok {
		return script.Nil, errors.New("not equipped")
	}
	switch field {
	case "item":
		return script.Int(int64(item)), nil
	case "wearer":
		return script.Int(1), nil
	case "refine", "enchant_grade", "weapon_level", "armor_level", "weapon_type":
		return script.Int(0), nil
	}
	return script.Nil, fmt.Errorf("equipped item has no field %q", field)
}

// recorder keeps the bonuses an equip context has.
type recorder struct{ active []*modscript.Bonus }

func (s *recorder) BonusAdded(b *modscript.Bonus) { s.active = append(s.active, b) }
func (s *recorder) BonusRemoved(b *modscript.Bonus) {
	s.active = slices.DeleteFunc(s.active, func(x *modscript.Bonus) bool { return x == b })
}

// testRuntime makes a runtime with core's converted scripts of a mode.
func testRuntime(t *testing.T, mode reference.Mode) (*modscript.Runtime, *testHost, *Core) {
	t.Helper()
	core := loadCore(t, mode)
	var jd status.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", &jd); err != nil {
		t.Fatal(err)
	}
	jobs, err := status.New(&jd)
	if err != nil {
		t.Fatal(err)
	}
	h := &testHost{jobs: jobs, baseLevel: 99, jobLevel: 50, stats: status.Stats{Str: 90, Agi: 50, Vit: 30, Int: 1, Dex: 40, Luk: 1},
		items: map[string]uint32{}, groups: map[string]int64{}, monsters: core.Monsters, equipped: map[int64]uint32{}}
	for id, name := range itemNames(t, mode) {
		h.items[name] = id
	}
	for _, g := range core.Groups.Groups {
		h.groups[g.Name] = g.ID
	}
	sc := &mod.Scripts{Mod: mod.Core, Files: map[string]string{}, Grants: mod.Grants{}, Visible: []string{mod.Core}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	err = fs.WalkDir(core.Files, "scripts", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(core.Files, p)
		sc.Files[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := modscript.New(modscript.Options{Host: h, Mods: []string{mod.Core}, Constants: core.Constants, Jobs: jobs}, []*mod.Scripts{sc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r, h, core
}

func itemNames(t *testing.T, mode reference.Mode) map[uint32]string {
	t.Helper()
	var d struct {
		Items []struct {
			ID        uint32 `json:"id"`
			AegisName string `json:"aegis_name"`
		} `json:"items"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/items.json", &d); err != nil {
		t.Fatal(err)
	}
	out := map[uint32]string{}
	for _, it := range d.Items {
		out[it.ID] = it.AegisName
	}
	return out
}

// scriptPaths maps item IDs to their generated script, as Files does.
func scriptPaths(res *Result) map[uint32]string {
	items := append([]*Item(nil), res.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	used := map[string]bool{}
	out := map[uint32]string{}
	for _, it := range items {
		if it.Lua != "" {
			out[it.ID] = "core:" + ScriptPath(it.AegisName, it.ID, used)
		}
	}
	return out
}

// TestRunConvertedScripts loads every converted script and runs it:
// usable items once, and equipment through equip, its computed bonuses,
// a hit dealt, a hit taken, a skill use, and unequip, against a
// recording bonus sink. Static bonuses are read as item data.
func TestRunConvertedScripts(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			r, h, core := testRuntime(t, mode)
			names := core.Names(h.items)
			paths := scriptPaths(core.Result)
			failures := 0
			fail := func(it *Item, err error) {
				failures++
				if failures <= 20 {
					t.Errorf("%s (%d): %v", it.AegisName, it.ID, err)
				}
			}
			var ran, equipped, static int
			for _, it := range core.Result.Items {
				for i, entry := range it.Bonuses {
					var v any
					if err := yamlUnmarshal(entry, &v); err != nil {
						fail(it, err)
						continue
					}
					if _, err := bonus.FromData(v, names); err != nil {
						fail(it, fmt.Errorf("bonuses[%d] %s: %w", i, entry, err))
					}
				}
				if it.Bonuses != nil {
					static++
				}
				path := paths[it.ID]
				if path == "" {
					continue
				}
				if it.Kind != KindEquip {
					ran++
					if err := r.UseItem(1, it.ID, path); err != nil {
						fail(it, err)
					}
					continue
				}
				equipped++
				h.equipped[int64(it.ID)] = it.ID
				sink := &recorder{}
				c, err := r.Equip(path, int64(it.ID), 1, sink)
				if err != nil {
					fail(it, err)
					continue
				}
				hit := modscript.Hit{Source: 1, SourceKind: "player", Target: 1, TargetKind: "player", Kind: "physical", Range: "melee", Amount: 10, WearerIsSource: true}
				for _, err := range []error{
					r.FireHit(c, hit),
					func() error { hit.WearerIsSource = false; return r.FireHit(c, hit) }(),
					r.FireSkillUse(c, modscript.SkillUse{Skill: 28, Level: 10}),
				} {
					if err != nil {
						fail(it, err)
					}
				}
				for _, b := range sink.active {
					if _, _, _, err := r.Values(b); err != nil {
						fail(it, fmt.Errorf("%s: %w", b.Name, err))
					}
				}
				if err := r.Unequip(c); err != nil {
					fail(it, err)
				}
				if len(sink.active) != 0 {
					fail(it, fmt.Errorf("%d bonuses after unequip", len(sink.active)))
				}
			}
			t.Logf("%s: %d usable scripts ran, %d equip scripts ran, %d static items read, %d failures", mode, ran, equipped, static, failures)
		})
	}
}

// TestReportsCurrent checks the generated coverage reports in
// docs/generated.
func TestReportsCurrent(t *testing.T) {
	for _, mode := range reference.Modes {
		path := "../../docs/generated/item-scripts-" + string(mode) + ".md"
		got := loadCore(t, mode).Result.Report()
		if *update {
			if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != got {
			t.Errorf("%s is not current: run go run ./tools/wactl convert items --mode %s --report %s", path, mode, strings.TrimPrefix(path, "../../"))
		}
	}
}

func yamlUnmarshal(s string, v any) error { return yaml.Unmarshal([]byte(s), v) }

// baseStats are the six stats item_bonuses.json compares: the status
// fields it records, and the bonus names that add to them.
var baseStats = []struct{ field, name string }{
	{"status.str", "str"}, {"status.agi", "agi"}, {"status.vit", "vit"},
	{"status.int", "int"}, {"status.dex", "dex"}, {"status.luk", "luk"},
}

// TestItemBonusesParity compares the base stats that converted equipment
// gives with rAthena's item_bonuses.json, which records each equipment
// item, and each card in a base item, equipped alone on a sample player
// (Novice or the first job that can wear it, base level from the record,
// the melee build, refine 0). Only items whose scripts convert fully are
// compared. The counts and the mismatches are a golden file, so a change
// shows in review.
func TestItemBonusesParity(t *testing.T) {
	var data struct {
		Items []struct {
			Item    uint32         `json:"item"`
			Job     int            `json:"job"`
			Level   int            `json:"level"`
			Changes map[string]any `json:"changes"`
		} `json:"items"`
		Cards []struct {
			Card    uint32         `json:"card"`
			Job     int            `json:"job"`
			Level   int            `json:"level"`
			Changes map[string]any `json:"changes"`
		} `json:"cards"`
	}
	if err := reference.Load(reference.Renewal, "item_bonuses", &data); err != nil {
		t.Fatal(err)
	}
	r, h, core := testRuntime(t, reference.Renewal)
	names := core.Names(h.items)
	byID := map[uint32]*Item{}
	for _, it := range core.Result.Items {
		byID[it.ID] = it
	}
	paths := scriptPaths(core.Result)

	type record struct {
		id         uint32
		job, level int
		changes    map[string]any
	}
	var records []record
	for _, it := range data.Items {
		records = append(records, record{it.Item, it.Job, it.Level, it.Changes})
	}
	for _, c := range data.Cards {
		records = append(records, record{c.Card, c.Job, c.Level, c.Changes})
	}
	var compared, matched, noScript, notFull, changing int
	var mismatches []string
	for _, rec := range records {
		it := byID[rec.id]
		if it == nil {
			noScript++
			continue
		}
		if it.Status != Full {
			notFull++
			continue
		}
		h.job, h.baseLevel = rec.job, rec.level
		predicted := map[string]int64{}
		add := func(name string, v int64) {
			switch name {
			case "all_stats":
				for _, s := range baseStats {
					predicted[s.name] += v
				}
			case "str", "agi", "vit", "int", "dex", "luk":
				predicted[name] += v
			}
		}
		for _, entry := range it.Bonuses {
			var v any
			if err := yamlUnmarshal(entry, &v); err != nil {
				t.Fatal(err)
			}
			b, err := bonus.FromData(v, names)
			if err != nil {
				t.Fatal(err)
			}
			if len(b.Args) > 0 {
				add(b.Name, b.Args[0].Value)
			}
		}
		if path := paths[rec.id]; path != "" {
			h.equipped[int64(rec.id)] = rec.id
			sink := &recorder{}
			c, err := r.Equip(path, int64(rec.id), 1, sink)
			if err != nil {
				t.Fatalf("%s: %v", it.AegisName, err)
			}
			for _, b := range sink.active {
				args, _, active, err := r.Values(b)
				if err != nil {
					t.Fatalf("%s: %v", it.AegisName, err)
				}
				if active && len(args) > 0 {
					add(b.Name, args[0])
				}
			}
			if err := r.Unequip(c); err != nil {
				t.Fatal(err)
			}
		}
		compared++
		for _, s := range baseStats {
			if _, ok := rec.changes[s.field]; ok {
				changing++
				break
			}
		}
		var diffs []string
		for _, s := range baseStats {
			var recorded int64
			want := predicted[s.name]
			if pair, ok := rec.changes[s.field].([]any); ok && len(pair) == 2 {
				before, _ := pair[0].(float64)
				after, _ := pair[1].(float64)
				recorded = int64(after - before)
				// status_calc_pc keeps a stat at 0 or more.
				if int64(before)+want < 0 {
					want = -int64(before)
				}
			}
			if recorded != want {
				diffs = append(diffs, fmt.Sprintf("%s %d, rAthena %d", s.name, want, recorded))
			}
		}
		if len(diffs) == 0 {
			matched++
			continue
		}
		mismatches = append(mismatches, fmt.Sprintf("- %s (%d): %s", it.AegisName, it.ID, strings.Join(diffs, "; ")))
	}
	sort.Strings(mismatches)
	var b strings.Builder
	fmt.Fprintf(&b, "# Base stats of converted equipment against rAthena's item_bonuses.json (renewal)\n\n")
	fmt.Fprintf(&b, "Records: %d. Without a script: %d. Not fully converted: %d.\n", len(records), noScript, notFull)
	fmt.Fprintf(&b, "Compared: %d, of which %d change a base stat in rAthena. Same STR, AGI, VIT, INT, DEX and LUK: %d. Different: %d.\n\n", compared, changing, matched, len(mismatches))
	b.WriteString("A stat that a bonus would take below 0 stays at 0, as status_calc_pc keeps it.\n\n")
	b.WriteString("Different:\n\n")
	for _, m := range mismatches {
		b.WriteString(m + "\n")
	}
	got := b.String()
	path := "testdata/item_bonuses_parity.md"
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != got {
		t.Errorf("%s changed; run go test ./tools/convert -run TestItemBonusesParity -update and review the diff:\n%s", path, got[:min(len(got), 600)])
	}
}
