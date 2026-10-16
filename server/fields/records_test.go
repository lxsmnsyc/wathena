package fields

import (
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/server/fields/core"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// coreRecords reads one of core's files as records.
func coreRecords(t *testing.T, file string) []map[string]any {
	t.Helper()
	b, err := core.Files.ReadFile("data/" + file)
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	if err := yaml.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	for _, r := range out {
		delete(r, "name")
	}
	return out
}

// TestCoreRecords: core's styles and templates pass their checks, and a
// model has one footprint in every template.
func TestCoreRecords(t *testing.T) {
	styles := coreRecords(t, "dungeon_styles.yml")
	if len(styles) != 4 {
		t.Errorf("%d core styles", len(styles))
	}
	for i, r := range styles {
		if _, err := ParseStyle(r); err != nil {
			t.Errorf("style %d: %v", i, err)
		}
	}
	templates := coreRecords(t, "town_templates.yml")
	if len(templates) != 3 {
		t.Errorf("%d core templates", len(templates))
	}
	feet := map[string][2]int{}
	for i, r := range templates {
		tp, err := ParseTemplate(r)
		if err != nil {
			t.Errorf("template %d: %v", i, err)
			continue
		}
		for _, b := range tp.Buildings {
			if f, ok := feet[b.Model]; ok && f != [2]int{b.Width, b.Depth} {
				t.Errorf("%s has two footprints", b.Model)
			}
			feet[b.Model] = [2]int{b.Width, b.Depth}
		}
	}
}

// TestParseRecords: defaults, and the errors that name the field.
func TestParseRecords(t *testing.T) {
	cave, err := ParseStyle(map[string]any{"method": "cave", "floors": 2, "floor_size": []any{64, 80}, "tileset": "cave", "level_step": 3})
	if err != nil || cave.RoomMin != gen.MinRoomSide || cave.Width != 64 || cave.Height != 80 {
		t.Errorf("cave %+v, %v", cave, err)
	}
	for _, c := range []struct {
		fields map[string]any
		want   string
	}{
		{map[string]any{"method": "rooms", "floors": 2, "floor_size": []any{64, 64}, "tileset": "crypt", "level_step": 3}, "room_size is required"},
		{map[string]any{"method": "maze", "floors": 2, "floor_size": []any{64, 64}, "room_size": []any{6, 8}, "tileset": "crypt", "level_step": 3}, "method is cave or rooms"},
		{map[string]any{"method": "cave", "floors": "two", "floor_size": []any{64}, "tileset": "cave", "level_step": 3}, "floors is required"},
		{map[string]any{"method": "cave", "floors": 2, "floor_size": []any{64, 64}, "tileset": "cave", "level_step": 3, "boss": "x"}, `"boss" is not a field`},
	} {
		if _, err := ParseStyle(c.fields); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.fields, err, c.want)
		}
	}
	town, err := ParseTemplate(map[string]any{"size": 64, "gates": 2, "tileset": "town",
		"buildings": []any{map[string]any{"model": "hut", "footprint": []any{4, 4}}}})
	if err != nil || town.Walls || len(town.Buildings) != 1 || town.Buildings[0].Count != 1 {
		t.Errorf("town %+v, %v", town, err)
	}
	for _, c := range []struct {
		fields map[string]any
		want   string
	}{
		{map[string]any{"size": 64, "gates": 2, "tileset": "town"}, "buildings is required"},
		{map[string]any{"size": 64, "gates": 2, "tileset": "town", "walls": "yes", "buildings": []any{}}, "walls is true or false"},
		{map[string]any{"size": 64, "gates": 2, "tileset": "town", "buildings": []any{map[string]any{"model": "hut"}}}, "buildings[0]: footprint is required"},
		{map[string]any{"size": 64, "gates": 2, "tileset": "town", "buildings": []any{map[string]any{"model": "hut", "footprint": []any{4, 4}, "door": "s"}}}, `"door" is not a field`},
		{map[string]any{"size": 64, "gates": 2, "tileset": "town", "buildings": []any{map[string]any{"model": "hut", "footprint": []any{4, 4}, "places": "inn"}}}, "places is a list of tags"},
		{map[string]any{"size": 65, "gates": 2, "tileset": "town", "buildings": []any{map[string]any{"model": "hut", "footprint": []any{4, 4}}}}, "size is an even number"},
	} {
		if _, err := ParseTemplate(c.fields); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.fields, err, c.want)
		}
	}
}

// TestParseDungeonSpec reads the forms of --dungeons.
func TestParseDungeonSpec(t *testing.T) {
	for _, c := range []struct {
		in     string
		style  string
		floors int
	}{
		{"crypt", "crypt", 0},
		{"crypt:3", "crypt", 3},
		{"example.mod:sewer", "example.mod:sewer", 0},
		{"example.mod:sewer:2", "example.mod:sewer", 2},
	} {
		s, f, err := ParseDungeonSpec(c.in)
		if err != nil || s != c.style || f != c.floors {
			t.Errorf("%s: %s %d %v", c.in, s, f, err)
		}
	}
	for _, bad := range []string{"", ":3", "crypt:0", "crypt:100"} {
		if _, _, err := ParseDungeonSpec(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
	if DungeonID("example.mod:sewer") != "sewer" || DungeonID("crypt") != "crypt" {
		t.Error(DungeonID("example.mod:sewer"))
	}
}
