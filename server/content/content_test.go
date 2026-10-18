//go:build procedural

package content_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/server/content"
	contentcore "github.com/lxsmnsyc/wathena/server/content/core"
	"github.com/lxsmnsyc/wathena/server/fields"
	fieldscore "github.com/lxsmnsyc/wathena/server/fields/core"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// coreTemplates reads core's quest templates.
func coreTemplates(t *testing.T) []content.Template {
	t.Helper()
	b, err := contentcore.Files.ReadFile(contentcore.TemplatesFile)
	if err != nil {
		t.Fatal(err)
	}
	var recs []map[string]any
	if err := yaml.Unmarshal(b, &recs); err != nil {
		t.Fatal(err)
	}
	var out []content.Template
	for _, r := range recs {
		name, _ := r["name"].(string)
		delete(r, "name")
		tp, err := content.ParseTemplate(name, "core", r, func(s string) bool { return s == "Delivery_Box" })
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out = append(out, tp)
	}
	return out
}

// open is a map of open ground with places.
func open(w, h int, places ...gen.Place) *gen.Field {
	f := &gen.Field{Places: places}
	f.Width, f.Height = w, h
	f.Types = make([]uint8, w*h)
	return f
}

// townParams are core's hamlet's params.
func townParams(t *testing.T) []byte {
	t.Helper()
	b, _ := fieldscore.Files.ReadFile("data/town_templates.yml")
	var recs []map[string]any
	if err := yaml.Unmarshal(b, &recs); err != nil {
		t.Fatal(err)
	}
	delete(recs[0], "name")
	tp, err := fields.ParseTemplate(recs[0])
	if err != nil {
		t.Fatal(err)
	}
	p, err := gen.EncodeTown(gen.TownParams{Template: tp})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// smallWorld is two towns and two fields in a row: t1 (levels 1 to 10),
// field 0:0, field 1:0 and t2 (levels 41 to 50). The near field holds
// Porings and Fabres, the far field Orc Warriors.
func smallWorld(t *testing.T, secret string, layers content.Layers) content.Input {
	t.Helper()
	g, ok := gen.Latest()
	if !ok {
		t.Skip("no generator")
	}
	params := townParams(t)
	l := &layout.Layout{Home: "gen:town:t1", Maps: []layout.Map{
		{ID: "gen:field:0:0", Kind: gen.KindField, Name: "Near Fields", Levels: [2]int{1, 10}, Biome: 0},
		{ID: "gen:field:1:0", Kind: gen.KindField, Name: "Far Fields", Levels: [2]int{40, 49}, Biome: 0, X: 1},
		{ID: "gen:town:t1", Kind: gen.KindTown, Name: "Ashby", Levels: [2]int{1, 10}, Town: 1, Params: params},
		{ID: "gen:town:t2", Kind: gen.KindTown, Name: "Bexley", Levels: [2]int{41, 50}, Town: 2, Params: params},
	}, Links: []layout.Link{
		{A: layout.End{Map: "gen:town:t1", Tag: "portal"}, B: layout.End{Map: "gen:field:0:0", Tag: "portal:west"}},
		{A: layout.End{Map: "gen:field:0:0", Tag: "portal:east"}, B: layout.End{Map: "gen:field:1:0", Tag: "portal:west"}},
		{A: layout.End{Map: "gen:field:1:0", Tag: "portal:east"}, B: layout.End{Map: "gen:town:t2", Tag: "portal"}},
	}}
	root, err := rng.NewRoot([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	town := func() *gen.Field {
		return open(64, 64, gen.Place{Tags: []string{"square"}, X: 32, Y: 32}, gen.Place{Tags: []string{"guild"}, X: 20, Y: 20})
	}
	maps := map[string]*gen.Field{
		"gen:field:0:0": open(96, 96, gen.Place{Tags: []string{"clearing"}, X: 40, Y: 40}),
		"gen:field:1:0": open(96, 96),
		"gen:town:t1":   town(),
		"gen:town:t2":   town(),
	}
	maps["gen:field:0:0"].Biome = 0
	monsters := map[string]content.MonsterInfo{
		"PORING":      {Name: "PORING", DisplayName: "Poring", Level: 1, Drops: []content.Drop{{Item: "Jellopy", Rate: 7000}, {Item: "Apple", Rate: 1000}}},
		"FABRE":       {Name: "FABRE", DisplayName: "Fabre", Level: 2, Drops: []content.Drop{{Item: "Fluff", Rate: 6500}}},
		"ORK_WARRIOR": {Name: "ORK_WARRIOR", DisplayName: "Orc Warrior", Level: 44, Drops: []content.Drop{{Item: "Orcish_Voucher", Rate: 5000}}},
		"VAGABOND":    {Name: "VAGABOND", DisplayName: "Vagabond Wolf", Level: 24, Boss: true},
	}
	items := map[string]content.ItemInfo{
		"Jellopy": {DisplayName: "Jellopy", Collectable: true}, "Fluff": {DisplayName: "Fluff", Collectable: true},
		"Orcish_Voucher": {DisplayName: "Orcish Voucher", Collectable: true}, "Apple": {DisplayName: "Apple"},
		"Delivery_Box": {DisplayName: "Delivery Box"},
	}
	return content.Input{
		Generator: g, Layout: l, Maps: maps,
		SpawnMaps: []spawns.Map{{ID: "gen:field:0:0", Kind: gen.KindField, Arrivals: []spawns.Point{{X: 1, Y: 48}}},
			{ID: "gen:field:1:0", Kind: gen.KindField, TownPortals: []spawns.Point{{X: 95, Y: 48}}}},
		Spawns: []spawns.Planned{
			{Map: "gen:field:0:0", Spawns: []spawns.Spawn{{Monster: "PORING", Count: 30, Role: spawns.Common}, {Monster: "FABRE", Count: 10, Role: spawns.Uncommon}}},
			{Map: "gen:field:1:0", Spawns: []spawns.Spawn{{Monster: "ORK_WARRIOR", Count: 40, Role: spawns.Common}}},
		},
		SpawnMonsters: map[string]spawns.Monster{"VAGABOND": {Name: "VAGABOND", Boss: true, Aggressive: true}, "PORING": {Name: "PORING"}},
		Data: content.Data{
			Monster: func(n string) (content.MonsterInfo, bool) { m, ok := monsters[n]; return m, ok },
			Item:    func(n string) (content.ItemInfo, bool) { i, ok := items[n]; return i, ok },
			BaseExp: func(level int) int64 { return int64(level) * 100 },
		},
		Templates: coreTemplates(t), Secret: spawns.SecretBranch(root), Layers: layers,
	}
}

// TestGenerate checks a small world's content against the rules of
// decision 0044.
func TestGenerate(t *testing.T) {
	in := smallWorld(t, "content-test", nil)
	w, err := content.Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	near := map[string][]string{"gen:town:t1": {"PORING", "FABRE", "ORK_WARRIOR"}, "gen:town:t2": {"PORING", "FABRE", "ORK_WARRIOR"}}
	board := map[string]int{}
	var zeny = map[string]int64{}
	for i, q := range w.Quests {
		if q.ID != content.FirstQuestID+uint32(i) { // #nosec G115 -- a few quests.
			t.Errorf("quest %d has the ID %d", i, q.ID)
		}
		switch q.Purpose {
		case content.PurposeBoard:
			board[q.Town]++
			if q.Kind != content.KindDelivery && !slices.Contains(near[q.Town], q.Monster) {
				t.Errorf("%+v names a monster away from its town", q)
			}
			if q.Kind == content.KindDelivery && (q.To == q.Town || q.To == "") {
				t.Errorf("delivery %+v", q)
			}
			if q.BaseExp <= 0 || q.JobExp <= 0 || q.Zeny <= 0 {
				t.Errorf("no reward: %+v", q)
			}
			if q.Template == "hunt" {
				zeny[q.Town] = q.Zeny
			}
		case content.PurposeGuild:
			if q.BaseExp != 0 || q.Zeny != 0 || q.Step < 1 || q.Step > 2 {
				t.Errorf("guild quest %+v", q)
			}
			if lvl := map[string]int{"PORING": 1, "FABRE": 2, "ORK_WARRIOR": 44}[q.Monster]; (q.Stage == content.StageFirst) != (lvl < 20) {
				t.Errorf("the %s test names %s, of level %d", q.Stage, q.Monster, lvl)
			}
		}
		if q.Kind == content.KindCollect && (q.Item == "Apple" || q.DropRate != content.DefaultDropRate) {
			t.Errorf("collect %+v", q)
		}
		if strings.Contains(q.Title, "{") {
			t.Errorf("title %q", q.Title)
		}
	}
	if board["gen:town:t1"] != content.BoardQuests || board["gen:town:t2"] != content.BoardQuests {
		t.Errorf("board quests %v", board)
	}
	if z1, z2 := zeny["gen:town:t1"], zeny["gen:town:t2"]; z1 > 0 && z2 > 0 && z2 <= z1 {
		t.Errorf("the higher town's hunt pays %d, the lower %d", z2, z1)
	}
	tags := map[string][]string{}
	for _, p := range w.Places {
		tags[p.Tags[0]] = append(tags[p.Tags[0]], p.Map)
		if p.Source != content.SourceCore {
			t.Errorf("place %+v", p)
		}
	}
	if !reflect.DeepEqual(tags[content.TagQuestBoard], []string{"gen:town:t1", "gen:town:t2"}) {
		t.Errorf("quest boards %v", tags[content.TagQuestBoard])
	}
	// Only t1's band starts at level 20 or lower, so every guild stands
	// there (decision 0047).
	for _, f := range content.Families {
		if got := tags[content.GuildTag(f)]; len(got) != 1 || got[0] != "gen:town:t1" {
			t.Errorf("the %s guild stands in %v, want gen:town:t1", f, got)
		}
	}
	if got := tags[content.TagRebirth]; len(got) != 1 || got[0] != "gen:town:t2" {
		t.Errorf("rebirth in %v", got)
	}
	// Places keep a cell from each other, and stand on open ground.
	for i, a := range w.Places {
		for _, b := range w.Places[i+1:] {
			if a.Map == b.Map && max(abs(a.X-b.X), abs(a.Y-b.Y)) < 2 {
				t.Errorf("places %+v and %+v touch", a, b)
			}
		}
	}
	again, err := content.Generate(smallWorld(t, "content-test", nil))
	if err != nil || !reflect.DeepEqual(again, w) {
		t.Errorf("a second run differs: %v", err)
	}
	other, err := content.Generate(smallWorld(t, "content-test-2", nil))
	if err != nil || reflect.DeepEqual(other.Quests, w.Quests) {
		t.Errorf("another secret gives the same quests: %v", err)
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// fakeLayers returns fixed results.
type fakeLayers struct {
	results map[string][]content.LayerResult
	seen    []string
}

func (f *fakeLayers) Map(m content.LayerMap) ([]content.LayerResult, []string) {
	f.seen = append(f.seen, m.ID+" "+m.Kind+" "+m.Tileset)
	return f.results[m.ID], nil
}

func (f *fakeLayers) Templates() []content.Template {
	return []content.Template{{Name: "example.a:walk", Mod: "example.a", Kind: content.KindDelivery, Title: "Walk to {town}", Reward: 10, Weight: 1000, Uses: []string{content.UseBoard}}}
}

// TestLayers checks the rules over the layers' results: places move to
// open ground, objects name props of the tileset, bosses join fields'
// lists with their clear cells, and templates join the core's.
func TestLayers(t *testing.T) {
	fl := &fakeLayers{results: map[string][]content.LayerResult{
		"gen:field:0:0": {{Mod: "example.a",
			Places:  []content.LayerPlace{{Tags: []string{"shrine"}, X: 5, Y: 5}, {Tags: []string{"Bad Tag"}, X: 1, Y: 1}},
			Objects: []content.LayerObject{{Model: "tree", X: 6, Y: 5, Turn: 2}, {Model: "fountain", X: 7, Y: 5}, {Model: "tree", X: 500, Y: 5}},
			Bosses: []content.LayerBoss{{Monster: "VAGABOND", Role: "mini_boss", X: 10, Y: 10, HasCell: true},
				{Monster: "VAGABOND", Role: "common"}, {Monster: "NOBODY", Role: "field_boss"}}}},
		"gen:town:t1": {{Mod: "example.a", Bosses: []content.LayerBoss{{Monster: "VAGABOND", Role: "mini_boss"}}}},
	}}
	in := smallWorld(t, "content-test", fl)
	// A wall cell under the shrine moves it to the next open cell.
	f := in.Maps["gen:field:0:0"]
	f.Types[5*f.Width+5] = gen.CellBlocked
	w, err := content.Generate(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(fl.seen) != 4 || fl.seen[2] != "gen:town:t1 town town" || !strings.HasPrefix(fl.seen[0], "gen:field:0:0 field ") {
		t.Errorf("layers ran on %v", fl.seen)
	}
	shrines := slices.IndexFunc(w.Places, func(p content.Place) bool { return p.Tags[0] == "shrine" })
	if shrines < 0 || w.Places[shrines].Source != "example.a" ||
		(w.Places[shrines].X == 5 && w.Places[shrines].Y == 5) {
		t.Errorf("places %+v", w.Places)
	}
	if len(w.Objects) != 1 || w.Objects[0] != (content.Object{Map: "gen:field:0:0", Model: "tree", X: 6, Y: 5, Turn: 2, Source: "example.a"}) {
		t.Errorf("objects %+v", w.Objects)
	}
	list := w.Spawns[0].Spawns
	if len(list) != 3 || list[2].Monster != "VAGABOND" || list[2].Role != spawns.MiniBoss || list[2].Count != 1 ||
		list[2].RespawnMs != content.DefaultMiniBossRespawn || list[2].Area != (spawns.Area{X: 10, Y: 10, W: content.BossReach, H: content.BossReach}) ||
		len(list[2].Avoid) != 1 || list[2].Avoid[0] != (spawns.Avoid{X: 1, Y: 48, R: spawns.ArrivalClear}) {
		t.Errorf("the field's list %+v", list)
	}
	if len(in.Spawns[0].Spawns) != 2 {
		t.Error("Generate changed its input's lists")
	}
	notes := strings.Join(w.Notes, "\n")
	for _, want := range []string{"place 2: tags", `object 2: "fountain" is not a prop`, "object 3: 500,5 is off the map",
		"boss 2: the role must be mini_boss or field_boss", "boss 3: NOBODY does not spawn", "on gen:town:t1: bosses spawn on fields and dungeon floors only"} {
		if !strings.Contains(notes, want) {
			t.Errorf("notes lack %q: %s", want, notes)
		}
	}
	if !slices.ContainsFunc(w.Templates, func(tp content.Template) bool { return tp.Name == "example.a:walk" }) {
		t.Errorf("templates %+v", w.Templates)
	}
	if !slices.ContainsFunc(w.Quests, func(q content.Quest) bool { return q.Template == "example.a:walk" }) {
		t.Error("no quest of the layers' template")
	}
}

// TestParseTemplate checks the fields of a template record.
func TestParseTemplate(t *testing.T) {
	tp, err := content.ParseTemplate("hunt", "core", map[string]any{"kind": "hunt", "title": "Hunt {count} {monster}", "count": []any{2, 5},
		"roles": []any{"rare"}, "uses": []any{"guild"}, "weight": 3}, nil)
	if err != nil || tp.Count != [2]int{2, 5} || !reflect.DeepEqual(tp.Roles, []spawns.Role{spawns.Rare}) || !tp.Has(content.UseGuild) ||
		tp.Has(content.UseBoard) || tp.Weight != 3 || tp.Reward != 100 {
		t.Errorf("%+v %v", tp, err)
	}
	c, err := content.ParseTemplate("c", "core", map[string]any{"kind": "collect", "title": "Bring {item}", "count": 3}, nil)
	if err != nil || c.Count != [2]int{3, 3} || c.DropRate != content.DefaultDropRate || len(c.Roles) != 2 {
		t.Errorf("%+v %v", c, err)
	}
	for _, bad := range []struct {
		fields map[string]any
		want   string
	}{
		{map[string]any{"kind": "talk", "title": "x"}, "kind must be"},
		{map[string]any{"kind": "hunt", "title": "x"}, "needs count"},
		{map[string]any{"kind": "hunt", "title": "x {who}", "count": 1}, "{who} is not one of"},
		{map[string]any{"kind": "hunt", "title": "x", "count": []any{5, 2}}, "count is a whole number or a range"},
		{map[string]any{"kind": "delivery", "title": "x", "count": 2}, "count is for hunt and collect"},
		{map[string]any{"kind": "delivery", "title": "x", "uses": []any{"guild"}}, "serves quest boards only"},
		{map[string]any{"kind": "delivery", "title": "x", "item": "Nothing"}, `item: "Nothing" is not an item`},
		{map[string]any{"kind": "hunt", "title": "x", "count": 1, "item": "Jellopy"}, "item is for delivery"},
		{map[string]any{"kind": "hunt", "title": "x", "count": 1, "roles": []any{"boss"}}, `"boss" is not a spawn role`},
		{map[string]any{"kind": "hunt", "title": "x", "count": 1, "reward": 5000}, "reward must be"},
		{map[string]any{"kind": "hunt", "title": "x", "count": 1, "colour": 1}, `"colour" is not a field`},
	} {
		_, err := content.ParseTemplate("x", "core", bad.fields, func(s string) bool { return s == "Jellopy" })
		if err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%v: %v, want %q", bad.fields, err, bad.want)
		}
	}
}

// TestRebirthTown: the largest town of the highest band, ties to the
// lower number.
func TestRebirthTown(t *testing.T) {
	towns := []*layout.Map{
		{ID: "a", Town: 1, Levels: [2]int{81, 90}}, {ID: "b", Town: 2, Levels: [2]int{89, 98}},
		{ID: "c", Town: 3, Levels: [2]int{89, 98}}, {ID: "d", Town: 4, Levels: [2]int{89, 98}},
	}
	size := map[string]int{"a": 900, "b": 100, "c": 400, "d": 400}
	if got := content.RebirthTown(towns, func(id string) int { return size[id] }); got.ID != "c" {
		t.Errorf("rebirth town %s", got.ID)
	}
	if content.RebirthTown(nil, nil) != nil {
		t.Error("a town without towns")
	}
}

// TestGuildTowns: the guilds stand in the towns of a band that starts
// at level 20 or lower, the first three at most, by band and number, or
// in the lowest town without one (decision 0047).
func TestGuildTowns(t *testing.T) {
	town := func(id string, n int64, low int) *layout.Map {
		return &layout.Map{ID: id, Town: n, Levels: [2]int{low, low + 9}}
	}
	for _, c := range []struct {
		name  string
		towns []*layout.Map
		want  string
	}{
		// The old rule took the third town, of band 41 to 50, here.
		{"two low towns", []*layout.Map{town("h", 1, 1), town("e", 2, 15), town("x", 3, 41), town("y", 4, 61)}, "h e h e h e"},
		{"three of four low", []*layout.Map{town("d", 4, 20), town("c", 3, 11), town("b", 2, 11), town("a", 1, 1)}, "a b c a b c"},
		{"one low town", []*layout.Map{town("x", 2, 21), town("h", 1, 1)}, "h h h h h h"},
		{"no low town", []*layout.Map{town("b", 2, 89), town("a", 1, 81)}, "a a a a a a"},
	} {
		var got []string
		for _, h := range content.GuildTowns(c.towns) {
			got = append(got, h.ID)
		}
		if strings.Join(got, " ") != c.want {
			t.Errorf("%s: guild towns %v, want %s", c.name, got, c.want)
		}
	}
	if content.GuildTowns(nil) != nil {
		t.Error("guild towns without towns")
	}
}
