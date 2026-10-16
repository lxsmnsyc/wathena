//go:build procedural

package spawns_test

import (
	"bytes"
	"sync"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/fields"
	fieldscore "github.com/lxsmnsyc/wathena/server/fields/core"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/layout"
	layoutcore "github.com/lxsmnsyc/wathena/server/layout/core"
	"github.com/lxsmnsyc/wathena/server/spawns"
	spawncore "github.com/lxsmnsyc/wathena/server/spawns/core"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// records reads one of core's record files.
func records(t testing.TB, b []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := yaml.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

var (
	coreOnce     sync.Once
	coreMonsters []spawns.Monster
	coreThemes   []spawns.Theme
	coreDensity  spawns.Densities
	coreErr      error
)

// corePool is core's renewal monsters with core's spawn tags and themes.
func corePool(t testing.TB) (*spawns.Pool, spawns.Resolved) {
	t.Helper()
	coreOnce.Do(func() {
		b, err := reference.ReadFile("extracts/renewal/script_constants.json")
		if err != nil {
			coreErr = err
			return
		}
		var consts *constants.Data
		if consts, coreErr = constants.Read(bytes.NewReader(b)); coreErr != nil {
			return
		}
		monsters, err := gamedata.LoadMonsters(reference.Renewal, nil, nil)
		if err != nil {
			coreErr = err
			return
		}
		coreMonsters = gamedata.SpawnMonsters(monsters, consts)
		ct, err := spawns.LoadCore()
		if err != nil {
			coreErr = err
			return
		}
		coreDensity = ct.Densities
		tb, _ := spawncore.Files.ReadFile(spawncore.ThemesFile)
		var recs []map[string]any
		if coreErr = yaml.Unmarshal(tb, &recs); coreErr != nil {
			return
		}
		for _, r := range recs {
			name, _ := r["name"].(string)
			delete(r, "name")
			th, err := spawns.ParseTheme(name, r, nil, nil)
			if err != nil {
				coreErr = err
				return
			}
			coreThemes = append(coreThemes, th)
		}
	})
	if coreErr != nil {
		t.Fatal(coreErr)
	}
	st, err := spawns.Settings{}.Resolve(coreDensity)
	if err != nil {
		t.Fatal(err)
	}
	return spawns.NewPool(coreMonsters, coreThemes), st
}

// world is a generated world's layout, maps and spawn inputs.
type world struct {
	layout *layout.Layout
	set    *fields.Set
	maps   []spawns.Map
	secret rng.Node
	public rng.Node
}

// coreWorld builds a world of core's records from a secret, with fields
// of size cells.
func coreWorld(t testing.TB, secret string, cfg layout.Config) world {
	t.Helper()
	g, _ := gen.Latest()
	root, err := rng.NewRoot([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	public := gen.PublicBranch(root)
	in := layout.Inputs{Generator: g, Public: public}
	b, _ := fieldscore.Files.ReadFile("data/town_templates.yml")
	for _, r := range records(t, b) {
		name, _ := r["name"].(string)
		delete(r, "name")
		tp, err := fields.ParseTemplate(r)
		if err != nil {
			t.Fatal(err)
		}
		in.Towns = append(in.Towns, layout.TownRecord{Name: name, Template: tp})
	}
	b, _ = fieldscore.Files.ReadFile("data/dungeon_styles.yml")
	for _, r := range records(t, b) {
		name, _ := r["name"].(string)
		delete(r, "name")
		st, err := fields.ParseStyle(r)
		if err != nil {
			t.Fatal(err)
		}
		in.Styles = append(in.Styles, layout.StyleRecord{Name: name, Style: st})
	}
	b, _ = layoutcore.Files.ReadFile("data/naming_styles.yml")
	var styles []layout.NamingStyle
	for _, r := range records(t, b) {
		name, _ := r["name"].(string)
		delete(r, "name")
		s, err := layout.ParseNamingStyle(name, r)
		if err != nil {
			t.Fatal(err)
		}
		styles = append(styles, s)
	}
	in.Naming = layout.NewNaming(styles)
	words, _ := chat.CoreRecords(chat.FilterFile)
	var list []chat.Word
	for _, w := range words {
		cw, err := chat.BuildWord(w.Name, w.Fields)
		if err != nil {
			t.Fatal(err)
		}
		list = append(list, cw)
	}
	in.Blocked = layout.Blocked(chat.NewFilter(list))
	l, err := layout.Build(cfg, in)
	if err != nil {
		t.Fatal(err)
	}
	made, err := fields.GenerateLayout(g, public, l, nil)
	if err != nil {
		t.Fatal(err)
	}
	set := fields.NewSet(made, nil)
	warps, err := set.LayoutWarps(l)
	if err != nil {
		t.Fatal(err)
	}
	maps, err := set.SpawnMaps(l, warps)
	if err != nil {
		t.Fatal(err)
	}
	return world{layout: l, set: set, maps: maps, secret: spawns.SecretBranch(root), public: public}
}
