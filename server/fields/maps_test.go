//go:build procedural

package fields

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/places"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// coreStyles and coreTemplates are core's records, parsed.
func coreStyles(t *testing.T) []Style {
	t.Helper()
	var out []Style
	for i, r := range coreRecords(t, "dungeon_styles.yml") {
		s, err := ParseStyle(r)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, Style{Name: []string{"cave", "crypt", "tower", "ruins"}[i], Mod: "core", Style: s})
	}
	return out
}

func coreTemplates(t *testing.T) []Template {
	t.Helper()
	var out []Template
	for i, r := range coreRecords(t, "town_templates.yml") {
		tp, err := ParseTemplate(r)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, Template{Name: []string{"hamlet", "village", "city"}[i], Mod: "core", Template: tp})
	}
	return out
}

// mapSet is two fields, two towns and two dungeons of the latest
// generator.
func mapSet(t *testing.T) (*Set, rng.Node) {
	t.Helper()
	g, _ := gen.Latest()
	public := gen.PublicBranch(rng.FromKey([32]byte{7}))
	fs, err := Generate(g, public, []Spec{{X: 0, Y: 0}, {X: 1, Y: 0}}, 64)
	if err != nil {
		t.Fatal(err)
	}
	styles, templates := coreStyles(t), coreTemplates(t)
	towns := []TownSpec{{N: 1, Template: templates[0]}, {N: 2, Template: templates[1]}}
	dungeons := []DungeonSpec{
		{ID: "crypt", Style: styles[1], Floors: 2, Levels: [2]int{1, 10}},
		{ID: "deep-cave", Style: styles[0], Floors: 1, Levels: [2]int{20, 30}},
	}
	ms, err := GenerateMaps(g, public, towns, dungeons)
	if err != nil {
		t.Fatal(err)
	}
	tilesets, err := LoadTilesets(g, "", templates...)
	if err != nil {
		t.Fatal(err)
	}
	return NewSet(append(fs, ms...), tilesets.JSON()), public
}

// TestGenerateMaps: each town and floor comes from its public node, a
// client that generates from the node and the params gets the same map,
// and floors' level bands rise by the style's step.
func TestGenerateMaps(t *testing.T) {
	s, public := mapSet(t)
	g, _ := gen.Latest()
	want := []string{"gen:dungeon:crypt:1", "gen:dungeon:crypt:2", "gen:dungeon:deep-cave:1", "gen:field:0:0", "gen:field:1:0", "gen:town:t1", "gen:town:t2"}
	var got []string
	for _, f := range s.Fields() {
		got = append(got, f.ID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("maps %v", got)
	}
	for _, f := range s.Fields() {
		if f.Kind == gen.KindField {
			continue
		}
		var node rng.Node
		if f.Kind == gen.KindTown {
			node = gen.TownNode(public, f.Town)
		} else {
			node = gen.FloorNode(gen.DungeonNode(public, f.Dungeon), f.Floor)
		}
		if f.Key != node.Key() {
			t.Errorf("%s: the key is not its node's", f.ID)
		}
		client, err := g.Map(rng.FromKey(f.Key), gen.MapParams{Kind: f.Kind, Params: f.Params})
		if err != nil || client.Checksum() != f.Checksum || sha256.Sum256(client.WMAP) != f.WMAPSum {
			t.Errorf("%s: the client's map differs: %v", f.ID, err)
		}
	}
	c1, c2 := s.Get("gen:dungeon:crypt:1"), s.Get("gen:dungeon:crypt:2")
	if c1.Levels != [2]int{1, 10} || c2.Levels != [2]int{7, 16} || c2.Floors != 2 || c1.Name != "crypt" {
		t.Errorf("crypt bands %v and %v", c1.Levels, c2.Levels)
	}
	if lv := s.Get("gen:dungeon:deep-cave:1").Levels; lv != [2]int{20, 30} {
		t.Errorf("cave band %v", lv)
	}
	if _, ok := placeAt(c2, "boss_room", 0); !ok {
		t.Error("the crypt's last floor has no boss room")
	}
	if _, ok := placeAt(c1, "exit", 0); !ok {
		t.Error("the crypt's first floor has no exit")
	}
	if _, err := GenerateMaps(g, public, nil, []DungeonSpec{{ID: "Bad", Style: coreStyles(t)[0], Floors: 1}}); err == nil {
		t.Error("a bad dungeon ID was taken")
	}
	v2, _ := gen.Lookup(2)
	if _, err := GenerateMaps(v2, public, []TownSpec{{N: 1, Template: coreTemplates(t)[0]}}, nil); err == nil || !strings.Contains(err.Error(), "makes no towns") {
		t.Errorf("generator 2 made a town: %v", err)
	}
}

// devLayout is the dev layout of mapSet's maps, as waserver --dev makes
// it for --fields 0:0,1:0 --towns 2 --dungeons crypt:2,deep-cave.
func devLayout(t *testing.T) *layout.Layout {
	t.Helper()
	styles, templates := coreStyles(t), coreTemplates(t)
	towns, recs := LayoutOf(styles, templates)
	spec := layout.DevSpec{FieldSize: 64,
		Fields:   []layout.DevField{{X: 0, Y: 0}, {X: 1, Y: 0}},
		Towns:    []layout.DevTown{{N: 1, Record: towns[0]}, {N: 2, Record: towns[1]}},
		Dungeons: []layout.DevDungeon{{ID: "crypt", Style: recs[1], Floors: 2}, {ID: "deep-cave", Style: recs[0], Floors: 1}},
	}
	l, unlinked, err := layout.Dev(spec, layout.Inputs{})
	if err != nil || len(unlinked) != 0 {
		t.Fatalf("dev layout: %v, unlinked %v", err, unlinked)
	}
	return l
}

// TestLayoutWarps: the dev links of step 3 hold as a layout: town t1
// opens from the first free portal, its other gate leads to the next
// town, dungeons open from towns' spare gates, and floors link in order.
// Every warp stands on its place's walkable cell and lands on a walkable
// cell that no warp covers.
func TestLayoutWarps(t *testing.T) {
	s, _ := mapSet(t)
	l := devLayout(t)
	warps, err := s.LayoutWarps(l)
	if err != nil {
		t.Fatal(err)
	}
	links := map[string][]string{}
	for from, list := range warps {
		for _, w := range list {
			links[from] = append(links[from], w.To)
			f, dst := s.Get(from), s.Get(w.To)
			if f.Grid().NoPass(w.X, w.Y) || dst.Grid().NoPass(w.ToX, w.ToY) {
				t.Errorf("%s %s: %d,%d to %s %d,%d", from, w.Name, w.X, w.Y, w.To, w.ToX, w.ToY)
			}
			for _, o := range warps[w.To] {
				if w.ToX >= o.X-o.XS && w.ToX <= o.X+o.XS && w.ToY >= o.Y-o.YS && w.ToY <= o.Y+o.YS {
					t.Errorf("%s %s lands in the warp %s", from, w.Name, o.Name)
				}
			}
		}
	}
	for _, c := range [][2]string{
		{"gen:field:0:0", "gen:field:1:0"},
		{"gen:field:1:0", "gen:field:0:0"},
		{"gen:field:0:0", "gen:town:t1"},
		{"gen:town:t1", "gen:town:t2"},
		{"gen:town:t2", "gen:dungeon:crypt:1"},
		{"gen:town:t2", "gen:dungeon:deep-cave:1"},
		{"gen:dungeon:crypt:1", "gen:dungeon:crypt:2"},
		{"gen:dungeon:crypt:2", "gen:dungeon:crypt:1"},
	} {
		if !slices.Contains(links[c[0]], c[1]) {
			t.Errorf("no warp from %s to %s: %v", c[0], c[1], links[c[0]])
		}
	}
	// A link to a place a map lacks is an error.
	bad := &layout.Layout{Links: []layout.Link{{A: layout.End{Map: "gen:field:0:0", Tag: "moon"}, B: layout.End{Map: "gen:town:t1", Tag: "portal"}}}}
	if _, err := s.LayoutWarps(bad); err == nil || !strings.Contains(err.Error(), "no such place") {
		t.Errorf("a missing place: %v", err)
	}
}

// TestSetPlaces: the index holds every map's places, and nearest follows
// the warps.
func TestSetPlaces(t *testing.T) {
	s, _ := mapSet(t)
	warps, err := s.LayoutWarps(devLayout(t))
	if err != nil {
		t.Fatal(err)
	}
	idx := s.Places(warps)
	sc, _ := places.ParseScope("town:any")
	if got := idx.Find(sc, "square"); len(got) != 2 {
		t.Errorf("%d squares", len(got))
	}
	dsc, _ := places.ParseScope("dungeon:crypt")
	if got := idx.Find(dsc, "entrance"); len(got) != 2 {
		t.Errorf("%d crypt entrances", len(got))
	}
	// From field 0:0, the nearest blacksmith is in t1, one warp away.
	i, ok := idx.Nearest("gen:field:0:0", 32, 32, "blacksmith")
	if !ok || idx.Place(i).Map != "gen:town:t1" {
		t.Errorf("nearest blacksmith from the field: %v %v", i, ok)
	}
	fsc, _ := places.ParseScope("map:gen:field:0:0")
	if len(idx.Find(fsc, "portal")) != 4 {
		t.Error("field 0:0 lacks its portal places")
	}
}

// TestServeMaps: a town's and a floor's info carry the kind, the params,
// and the floor's band.
func TestServeMaps(t *testing.T) {
	s, _ := mapSet(t)
	srv := httptest.NewServer(s)
	defer srv.Close()
	for _, id := range []string{"gen:town:t1", "gen:dungeon:crypt:2", "gen:field:0:0"} {
		res, err := http.Get(srv.URL + "/gen/maps/" + id + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var info Info
		if err := json.NewDecoder(res.Body).Decode(&info); err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		f := s.Get(id)
		if info.Kind != f.Kind || info.Params != hex.EncodeToString(f.Params) || info.Name != f.Name || info.Version != f.Version ||
			len(info.Places) != len(f.Field.Places) || len(info.Places) == 0 || info.Places[0].X != f.Field.Places[0].X {
			t.Errorf("%s: %+v", id, info)
		}
		switch f.Kind {
		case gen.KindDungeon:
			if !slices.Equal(info.Levels, []int{7, 16}) || info.Floor != 2 || info.Dungeon != "crypt" {
				t.Errorf("%s: %+v", id, info)
			}
		case gen.KindTown:
			if info.Town != 1 || info.Levels != nil || info.Params == "" {
				t.Errorf("%s: %+v", id, info)
			}
		default:
			if info.Params != "" || info.Kind != "field" {
				t.Errorf("%s: %+v", id, info)
			}
		}
	}
}

// TestTilesetBuildings: a template's buildings join its tileset with a
// placeholder of their footprint, and a tileset table may map them.
func TestTilesetBuildings(t *testing.T) {
	g, _ := gen.Latest()
	templates := coreTemplates(t)
	ts, err := LoadTilesets(g, "", templates...)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"model/gen/town/smithy":    "placeholder/building/8x6",
		"model/gen/town/chapel":    "placeholder/building/8x10",
		"model/gen/town/fountain":  "placeholder/prop/fountain",
		"texture/gen/town/paving":  "placeholder/ground/paving",
		"model/gen/crypt/pillar":   "placeholder/prop/pillar",
		"texture/gen/cave/rock":    "placeholder/ground/rock",
		"model/gen/grassland/tree": "placeholder/prop/tree",
	} {
		if ts.Aliases[name] != want {
			t.Errorf("%s: %q, want %s", name, ts.Aliases[name], want)
		}
	}
	p := filepath.Join(t.TempDir(), "tilesets.yml")
	if err := os.WriteFile(p, []byte("tilesets:\n  town:\n    models: {smithy: model/my/smithy}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if ts, err := LoadTilesets(g, p, templates...); err != nil || ts.Aliases["model/gen/town/smithy"] != "model/my/smithy" {
		t.Errorf("a mapped building: %v", err)
	}
	if _, err := LoadTilesets(g, p); err == nil {
		t.Error("a building slot without its template was taken")
	}
	if err := CheckRecords(g, coreStyles(t), templates); err != nil {
		t.Error(err)
	}
	bad := templates[0]
	bad.Template.Tileset = "moon"
	badStyle := coreStyles(t)[0]
	badStyle.Style.Tileset = "town"
	if err := CheckRecords(g, []Style{badStyle}, []Template{bad}); err == nil ||
		!strings.Contains(err.Error(), `no dungeon tileset "town"`) || !strings.Contains(err.Error(), `no town tileset "moon"`) {
		t.Errorf("bad records: %v", err)
	}
}
