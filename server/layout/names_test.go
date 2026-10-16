package layout

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// style is a naming style for tests.
func style(biome string, start, end []string) NamingStyle {
	return NamingStyle{Name: "test", Biome: biome,
		Parts:    []Part{{Name: "end", Syllables: end}, {Name: "start", Syllables: start}},
		Patterns: [][]string{{"start", "end"}}}
}

// TestParseNamingStyle: a record's fields, and the checks.
func TestParseNamingStyle(t *testing.T) {
	ok := map[string]any{
		"biome":     "grassland",
		"syllables": map[string]any{"start": []any{"Val", "hrim"}, "end": []any{"heim"}},
		"patterns":  []any{"{start}{end}"},
	}
	s, err := ParseNamingStyle("norse", ok)
	if err != nil || s.Biome != "grassland" || len(s.Parts) != 2 || s.Parts[0].Name != "end" || !slices.Equal(s.Patterns[0], []string{"start", "end"}) {
		t.Fatalf("%+v %v", s, err)
	}
	for name, fields := range map[string]map[string]any{
		"no biome":       {"syllables": ok["syllables"], "patterns": ok["patterns"]},
		"unknown field":  {"biome": "grassland", "syllables": ok["syllables"], "patterns": ok["patterns"], "moon": 1},
		"bad pattern":    {"biome": "grassland", "syllables": ok["syllables"], "patterns": []any{"start{end}"}},
		"missing part":   {"biome": "grassland", "syllables": ok["syllables"], "patterns": []any{"{start}{middle}"}},
		"bad syllable":   {"biome": "grassland", "syllables": map[string]any{"start": []any{"Val!"}, "end": []any{"x"}}, "patterns": ok["patterns"]},
		"too short":      {"biome": "grassland", "syllables": map[string]any{"start": []any{"a"}, "end": []any{"b"}}, "patterns": ok["patterns"]},
		"too long":       {"biome": "grassland", "syllables": map[string]any{"start": []any{"abcdefghijk"}, "end": []any{"abcdefghijk"}}, "patterns": ok["patterns"]},
		"no patterns":    {"biome": "grassland", "syllables": ok["syllables"], "patterns": []any{}},
		"empty part":     {"biome": "grassland", "syllables": map[string]any{"start": []any{}, "end": []any{"heim"}}, "patterns": ok["patterns"]},
		"part not lists": {"biome": "grassland", "syllables": map[string]any{"start": "val"}, "patterns": ok["patterns"]},
	} {
		if _, err := ParseNamingStyle("x", fields); err == nil {
			t.Errorf("%s was taken", name)
		}
	}
	if err := CheckNaming([]string{"forest"}, []NamingStyle{s}); err == nil || !strings.Contains(err.Error(), `no biome "grassland"`) {
		t.Errorf("a style of an unknown biome: %v", err)
	}
}

// TestNaming: a later style of a biome replaces an earlier one.
func TestNaming(t *testing.T) {
	a, b, c := style("grassland", []string{"val"}, []string{"heim"}), style("desert", []string{"al"}, []string{"abad"}), style("grassland", []string{"sky"}, []string{"hold"})
	c.Name = "example.mod:sky"
	n := NewNaming([]NamingStyle{a, b, c})
	if got, _ := n.For("grassland"); got.Name != "example.mod:sky" || len(n.Styles()) != 2 {
		t.Errorf("grassland's style is %s", got.Name)
	}
	if _, ok := n.For("snow"); ok {
		t.Error("snow has a style")
	}
}

// townLayout is a layout of n towns in a row of fields, for tests
// without a generator: field i links east to field i+1, and town i
// links to field i-1's south portal and field i-1's north portal...
// each town stands on its own field.
func townLayout(n int) *Layout {
	l := &Layout{}
	for i := range n {
		l.Maps = append(l.Maps, Map{ID: gen.FieldID(int64(i), 0), Kind: gen.KindField, X: int64(i), Biome: 0})
		if i > 0 {
			l.Links = append(l.Links, Link{End{gen.FieldID(int64(i-1), 0), FieldPortal(gen.EdgeEast), 0}, End{gen.FieldID(int64(i), 0), FieldPortal(gen.EdgeWest), 0}})
		}
	}
	for i := range n {
		id := gen.TownMapID(int64(i + 1))
		l.Maps = append(l.Maps, Map{ID: id, Kind: gen.KindTown, X: int64(i), Biome: 0, Town: int64(i + 1)})
		l.Links = append(l.Links, Link{End{gen.FieldID(int64(i), 0), FieldPortal(gen.EdgeSouth), 0}, End{id, TagPortal, 0}})
	}
	l.Home = HomeOf(l)
	return l
}

func inputs(n *Naming, blocked func(string) bool) Inputs {
	root, _ := rng.NewRoot([]byte("names"))
	return Inputs{Public: root.Child("public"), Naming: n, Blocked: blocked, Generator: gen.Generator{Biomes: []string{"grassland"}}}
}

// TestTownNames: names follow the style, stay unique, skip blocked
// words, and retry with the next index. A style that cannot pass fails.
func TestTownNames(t *testing.T) {
	l := townLayout(3)
	in := inputs(NewNaming([]NamingStyle{style("grassland", []string{"Val", "Thor"}, []string{"heim", "gard"})}), nil)
	if err := nameTowns(l, in); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tw := range l.Towns() {
		names = append(names, tw.Name)
		if !ValidTownName(tw.Name) || tw.Name[:1] != strings.ToUpper(tw.Name[:1]) || tw.Name[1:] != strings.ToLower(tw.Name[1:]) {
			t.Errorf("name %q", tw.Name)
		}
	}
	if len(slices.Compact(slices.Sorted(slices.Values(names)))) != 3 {
		t.Errorf("names %v are not unique", names)
	}
	// The same seed gives the same names; a blocked word moves a town to
	// a later index.
	again := townLayout(3)
	_ = nameTowns(again, in)
	for i, tw := range again.Towns() {
		if tw.Name != names[i] {
			t.Errorf("town %d: %s then %s", i, names[i], tw.Name)
		}
	}
	blocked := townLayout(3)
	in.Blocked = func(name string) bool { return strings.EqualFold(name, names[0]) }
	if err := nameTowns(blocked, in); err != nil {
		t.Fatal(err)
	}
	if blocked.Towns()[0].Name == names[0] {
		t.Error("a blocked name was taken")
	}
	// Two syllables make one name: the second town finds none.
	one := townLayout(2)
	in = inputs(NewNaming([]NamingStyle{style("grassland", []string{"Val"}, []string{"heim"})}), nil)
	if err := nameTowns(one, in); !errors.Is(err, ErrNoName) {
		t.Errorf("one possible name for two towns: %v", err)
	}
	// A town whose biome has no style fails.
	in = inputs(NewNaming([]NamingStyle{style("desert", []string{"Al"}, []string{"abad"})}), nil)
	if err := nameTowns(townLayout(1), in); !errors.Is(err, ErrNoName) {
		t.Errorf("no style for grassland: %v", err)
	}
	// A home map's name is taken too.
	home := townLayout(1)
	home.Maps = append(home.Maps, Map{ID: HomeMapID("v"), Kind: KindHome, Name: "Valheim"})
	in = inputs(NewNaming([]NamingStyle{style("grassland", []string{"Val"}, []string{"heim", "gard"})}), nil)
	if err := nameTowns(home, in); err != nil || home.Towns()[0].Name != "Valgard" {
		t.Errorf("a town beside the home map Valheim: %v %v", home.Towns()[0].Name, err)
	}
}

// TestDeriveNames: fields and dungeons take the nearest town's name, a
// direction and a number when they share it.
func TestDeriveNames(t *testing.T) {
	l := &Layout{}
	for _, xy := range [][2]int64{{0, 0}, {0, 1}, {0, 2}, {1, 1}, {-1, 1}} {
		l.Maps = append(l.Maps, Map{ID: gen.FieldID(xy[0], xy[1]), Kind: gen.KindField, X: xy[0], Y: xy[1]})
	}
	l.Maps = append(l.Maps, Map{ID: "gen:town:t1", Kind: gen.KindTown, Town: 1, Name: "Valheim", Y: 0})
	for f := 1; f <= 2; f++ {
		l.Maps = append(l.Maps, Map{ID: gen.DungeonMapID("crypt", f), Kind: gen.KindDungeon, Dungeon: "crypt", Floor: f, Floors: 2, Record: "crypt"})
	}
	l.Maps = append(l.Maps, Map{ID: gen.DungeonMapID("sunken_hall", 1), Kind: gen.KindDungeon, Dungeon: "sunken_hall", Floor: 1, Floors: 1, Record: "example.mod:sunken_hall"})
	link := func(a, b string) { l.Links = append(l.Links, Link{End{a, "x", 0}, End{b, "y", 0}}) }
	link("gen:town:t1", gen.FieldID(0, 0))
	link(gen.FieldID(0, 0), gen.FieldID(0, 1))
	link(gen.FieldID(0, 1), gen.FieldID(0, 2))
	link(gen.FieldID(0, 1), gen.FieldID(1, 1))
	link(gen.FieldID(0, 1), gen.FieldID(-1, 1))
	link(gen.FieldID(0, 2), gen.DungeonMapID("crypt", 1))
	link(gen.DungeonMapID("crypt", 1), gen.DungeonMapID("crypt", 2))
	link(gen.FieldID(1, 1), gen.DungeonMapID("sunken_hall", 1))
	DeriveNames(l)
	want := map[string]string{
		gen.FieldID(0, 0):                  "Valheim Fields",
		gen.FieldID(0, 1):                  "North Valheim Fields 1",
		gen.FieldID(0, 2):                  "North Valheim Fields 2",
		gen.FieldID(1, 1):                  "North Valheim Fields 3",
		gen.FieldID(-1, 1):                 "North Valheim Fields 4",
		gen.DungeonMapID("crypt", 1):       "Crypt of Valheim F1",
		gen.DungeonMapID("crypt", 2):       "Crypt of Valheim F2",
		gen.DungeonMapID("sunken_hall", 1): "Sunken Hall of Valheim F1",
	}
	for _, m := range l.Maps {
		if w, ok := want[m.ID]; ok && m.Name != w {
			t.Errorf("%s: %q, want %q", m.ID, m.Name, w)
		}
	}
	if direction(3, 1) != "East" || direction(-3, 2) != "West" || direction(0, -1) != "South" || direction(2, 2) != "North" {
		t.Error("directions")
	}
}

// TestAtlas: lookups, distances, the nearest town, renames and the
// discovered part of the graph.
func TestAtlas(t *testing.T) {
	l := townLayout(3)
	for i, n := range []string{"Valheim", "Skarvik", "Eldholm"} {
		l.Maps[3+i].Name = n
	}
	DeriveNames(l)
	var saved [][]Rename
	fail := false
	a := NewAtlas(l, AtlasOptions{
		Blocked: func(n string) bool { return strings.Contains(strings.ToLower(n), "bad") },
		Save: func(_ context.Context, names []Rename) error {
			if fail {
				return errors.New("down")
			}
			saved = append(saved, names)
			return nil
		},
	})
	if a.Home() != "gen:town:t1" {
		t.Errorf("home %s", a.Home())
	}
	if m, ok := a.Find("skarvik"); !ok || m.ID != "gen:town:t2" {
		t.Errorf("find by name: %+v", m)
	}
	if m, ok := a.Find("East Valheim Fields"); ok {
		t.Errorf("a name no map has: %+v", m)
	}
	if m, ok := a.Find("Skarvik Fields"); !ok || m.ID != gen.FieldID(1, 0) {
		t.Errorf("a field by name: %+v %v", m, ok)
	}
	if d := a.Distance("gen:town:t1", "gen:town:t3"); d != 4 {
		t.Errorf("t1 to t3: %d warps", d)
	}
	if d := a.Distance(gen.FieldID(0, 0), gen.FieldID(1, 0)); d != -1 {
		t.Errorf("two fields: %d", d)
	}
	if n, ok := a.NearestTown(gen.FieldID(2, 0)); !ok || n.ID != "gen:town:t3" {
		t.Errorf("nearest town of field 2: %+v", n)
	}
	if tw, ok := a.Town("t2"); !ok || tw.Name != "Skarvik" {
		t.Errorf("town t2 %+v", tw)
	}
	for name, want := range map[string]error{"Ab": ErrTownName, "Skarvik": ErrNameTaken, "Badtown": ErrNameFilter, "Val heim": ErrTownName} {
		if err := a.CheckRename("t1", name); !errors.Is(err, want) {
			t.Errorf("%q: %v, want %v", name, err, want)
		}
	}
	if err := a.CheckRename("t9", "Newtown"); !errors.Is(err, ErrNotTown) {
		t.Errorf("t9: %v", err)
	}
	changes, err := a.Rename(context.Background(), "t1", "Newheim")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || changes[0] != (Rename{"gen:town:t1", "Newheim"}) || changes[1] != (Rename{gen.FieldID(0, 0), "Newheim Fields"}) || len(saved) != 1 {
		t.Errorf("changes %v", changes)
	}
	if n, _ := a.Name(gen.FieldID(0, 0)); n != "Newheim Fields" {
		t.Errorf("the field is %q", n)
	}
	fail = true
	if _, err := a.Rename(context.Background(), "t1", "Oldheim"); err == nil {
		t.Error("a failed save renamed")
	}
	if n, _ := a.Name("gen:town:t1"); n != "Newheim" {
		t.Errorf("a failed save left %q", n)
	}
	nodes, links := a.Discovered([]string{"gen:town:t1"})
	if len(nodes) != 2 || !nodes[1].Visited || nodes[0].Visited || nodes[0].Name != "" || len(links) != 1 ||
		links[0] != [2]string{gen.FieldID(0, 0), "gen:town:t1"} {
		t.Errorf("discovered %+v %v", nodes, links)
	}
	if n, l := (*Atlas)(nil).Discovered(nil); n != nil || l != nil || len((*Atlas)(nil).Towns()) != 0 {
		t.Error("a nil atlas")
	}
}

// TestBands: the home area and the nearest fields are levels 1 to 10,
// each further warp adds 8 levels, floors build on their entrance by
// their step, and bands stop at the top band (review decision 44).
func TestBands(t *testing.T) {
	l := townLayout(3)
	l.Maps = append(l.Maps, Map{ID: gen.DungeonMapID("d", 1), Kind: gen.KindDungeon, Dungeon: "d", Floor: 1, Floors: 2, Record: "deep"},
		Map{ID: gen.DungeonMapID("d", 2), Kind: gen.KindDungeon, Dungeon: "d", Floor: 2, Floors: 2, Record: "deep"})
	l.Links = append(l.Links, Link{End{gen.FieldID(2, 0), FieldPortal(gen.EdgeNorth), 0}, End{gen.DungeonMapID("d", 1), TagEntrance, 0}},
		Link{End{gen.DungeonMapID("d", 1), TagExit, 0}, End{gen.DungeonMapID("d", 2), TagEntrance, 0}})
	Bands(l, BandConfig{Width: 10, Step: 8, MaxLevel: 99}, []StyleRecord{{Name: "deep", Style: gen.DungeonStyle{LevelStep: 80}}})
	got := map[string][2]int{}
	for _, m := range l.Maps {
		got[m.ID] = m.Levels
	}
	// t1 0, field 0 1, field 1 2, field 2 3, t3 4 warps away.
	for id, want := range map[string][2]int{
		"gen:town:t1": {1, 10}, gen.FieldID(0, 0): {1, 10}, gen.FieldID(1, 0): {9, 18}, gen.FieldID(2, 0): {17, 26}, "gen:town:t3": {25, 34},
		gen.DungeonMapID("d", 1): {17, 26}, gen.DungeonMapID("d", 2): {90, 99},
	} {
		if got[id] != want {
			t.Errorf("%s: %v, want %v", id, got[id], want)
		}
	}
}

// TestOptions: procedural.layout's defaults, values and checks, and the
// choice of the home map.
func TestOptions(t *testing.T) {
	c, err := Options{}.Config(0)
	if err != nil || c != DefaultConfig() {
		t.Errorf("defaults %+v %v", c, err)
	}
	zero, two := 0, 2
	c, err = Options{Grid: []int{3, 2}, Towns: &two, Dungeons: &zero, BandWidth: 5, MaxLevel: 50}.Config(128)
	if err != nil || c.Width != 3 || c.Height != 2 || c.Towns != 2 || c.Dungeons != 0 || c.BandWidth != 5 || c.MaxLevel != 50 || c.FieldSize != 128 {
		t.Errorf("config %+v %v", c, err)
	}
	if _, err := (Options{Grid: []int{3}}).Config(0); err == nil {
		t.Error("a grid of one number")
	}
	if _, err := (Options{Towns: &zero}).Config(0); err == nil {
		t.Error("no towns")
	}
	homes := []Home{{Record: "a:one"}, {Record: "b:two"}}
	if h, err := PickHome(homes, "b:two"); err != nil || h.Record != "b:two" {
		t.Errorf("named home %v %v", h, err)
	}
	if h, err := PickHome(homes, HomeNone); err != nil || h != nil {
		t.Errorf("none %v %v", h, err)
	}
	if _, err := PickHome(homes, ""); err == nil {
		t.Error("two homes without a choice")
	}
	if _, err := PickHome(homes, "c:three"); err == nil {
		t.Error("a home that no mod has")
	}
	if h, err := PickHome(homes[:1], ""); err != nil || h.Record != "a:one" {
		t.Errorf("the only home %v %v", h, err)
	}
	if h, err := PickHome(nil, ""); err != nil || h != nil {
		t.Errorf("no homes %v %v", h, err)
	}
}
