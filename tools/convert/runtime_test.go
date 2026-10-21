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
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/companion"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/status"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
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
	// refine and defs give the refine and item data of equipped copies,
	// and slots the copy in each equipment slot.
	refine map[int64]int
	defs   map[uint32]*item.Def
	slots  map[string]int64
	// statuses is the mode's status data; started counts status starts.
	statuses *sc.DB
	started  int
	// companion, mount and cart are what the companion calls set, by
	// kind, with 0 for none.
	companions map[string]int
}

// CompanionID finds core's records, with the IDs of their files.
func (h *testHost) CompanionID(kind, name string) (int, bool) {
	set := companion.Core()
	t := &set.Companions
	if kind == "mount" {
		t = &set.Mounts
	}
	if r := t.ByName(name); r != nil {
		return r.ID, true
	}
	return 0, false
}

func (h *testHost) SetCompanion(_ int64, kind string, id int) (bool, error) {
	if h.companions == nil {
		h.companions = map[string]int{}
	}
	h.companions[kind] = id
	return true, nil
}

func (h *testHost) SetCart(_ int64, look int) (bool, error) {
	return h.SetCompanion(0, "cart", look)
}

func (h *testHost) status(id int64) error {
	if h.statuses.Get(int(id)) == nil {
		return fmt.Errorf("there is no status %d", id)
	}
	return nil
}

func (h *testHost) StartStatus(_ modscript.Entity, s modscript.StatusStart) (bool, error) {
	h.started++
	return true, h.status(s.Status)
}

func (h *testHost) PreviewStatus(_ modscript.Entity, s modscript.StatusStart) (modscript.StatusPreview, bool, error) {
	return modscript.StatusPreview{Chance: s.Chance, Milliseconds: s.Milliseconds}, true, h.status(s.Status)
}

func (h *testHost) EndStatus(_ modscript.Entity, id int64) (bool, error) { return false, h.status(id) }

func (h *testHost) Statuses(modscript.Entity, int64) ([]modscript.StatusInfo, error) { return nil, nil }

func (h *testHost) DefineStatus(modscript.StatusDefinition) (int64, error) {
	return 0, fmt.Errorf("core defines no statuses")
}

func (h *testHost) AddTimedBonuses(int64, modscript.TimedBonuses) (bool, error) { return true, nil }

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
	case "companion", "mount", "cart":
		if id := h.companions[field]; id != 0 {
			return script.Int(int64(id)), nil
		}
		return script.Nil, nil
	case "job_class":
		m, _ := h.jobs.MapID(h.job)
		return script.Int(int64(m)), nil
	case "name":
		return script.String("Tester"), nil
	case "sex":
		return script.String("male"), nil
	case "map":
		return script.String("prontera"), nil
	case "character_id", "account_id", "x", "y":
		return script.Int(1), nil
	case "party", "guild":
		return script.Nil, nil
	case "weight", "skill_points", "staff_level":
		return script.Int(0), nil
	case "max_weight":
		return script.Int(80000), nil
	case "look":
		return script.NewTable(script.KV("hair_style", script.Int(1)), script.KV("hair_color", script.Int(0)),
			script.KV("cloth_color", script.Int(0)), script.KV("head_top", script.Int(0)), script.KV("head_middle", script.Int(0)),
			script.KV("head_bottom", script.Int(0)), script.KV("weapon", script.Int(0)), script.KV("shield", script.Int(0))), nil
	}
	return script.Nil, fmt.Errorf("player has no field %q", field)
}
func (h *testHost) MonsterField(id int64, field string) (script.Value, error) {
	if field == "game_id" {
		// The test hosts' handles are their game IDs (mapshost_test.go).
		return script.Int(id), nil
	}
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
func (h *testHost) Equipped(_ int64, slot string) (int64, bool, error) {
	id, ok := h.slots[slot]
	return id, ok, nil
}
func (h *testHost) RandomN(int64) int64                   { return 0 }
func (h *testHost) Now() int64                            { return 0 }
func (h *testHost) ItemID(name string) (uint32, bool)     { id, ok := h.items[name]; return id, ok }
func (h *testHost) ItemGroupID(name string) (int64, bool) { id, ok := h.groups[name]; return id, ok }
func (h *testHost) MonsterID(name string) (int64, bool)   { id, ok := h.monsters[name]; return id, ok }
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
	case "refine":
		return script.Int(int64(h.refine[id])), nil
	case "weapon_level", "armor_level", "weapon_type":
		d := h.defs[item]
		if d == nil {
			return script.Int(0), nil
		}
		v := map[string]int{"weapon_level": d.WeaponLevel, "armor_level": d.ArmorLevel, "weapon_type": d.Subtype}[field]
		if field == "weapon_type" && !d.IsWeapon() {
			v = 0
		}
		return script.Int(int64(v)), nil
	case "enchant_grade":
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
	statuses, _, err := core.LoadStatuses(mode)
	if err != nil {
		t.Fatal(err)
	}
	h := &testHost{jobs: jobs, statuses: statuses, baseLevel: 99, jobLevel: 50, stats: status.Stats{Str: 90, Agi: 50, Vit: 30, Int: 1, Dex: 40, Luk: 1},
		items: map[string]uint32{}, groups: map[string]int64{}, monsters: core.Monsters, equipped: map[int64]uint32{},
		refine: map[int64]int{}, defs: map[uint32]*item.Def{}, slots: map[string]int64{}}
	for id, name := range itemNames(t, mode) {
		h.items[name] = id
	}
	for _, g := range core.Groups.Groups {
		h.groups[g.Name] = g.ID
	}
	r, err := modscript.New(modscript.Options{Host: h, Mods: []string{mod.Core}, Constants: core.Constants, Jobs: jobs}, coreScripts(t, core))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r, h, core
}

// coreScripts are core's converted files as a mod's scripts, with every
// permission.
func coreScripts(t *testing.T, core *Core) []*mod.Scripts {
	t.Helper()
	sc := &mod.Scripts{Mod: mod.Core, Files: map[string]string{}, Grants: mod.Grants{}, Visible: []string{mod.Core}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	err := fs.WalkDir(core.Files, "scripts", func(p string, d fs.DirEntry, err error) error {
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
	return []*mod.Scripts{sc}
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
	testrace.SkipUnderRace(t, "it converts or runs all of core's scripts on one goroutine")
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
