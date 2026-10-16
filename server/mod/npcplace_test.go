package mod

import (
	"strings"
	"testing"
)

// TestNPCPlaces: a record may give place in place of map, x and y
// (review decision 42), and the place is checked.
func TestNPCPlaces(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{
		"mod.yaml": manifest("example.a"),
		"data/npcs.yml": `
- name: Smith Helper
  place: {scope: "town:any", tag: blacksmith}
  sprite: 4_F_KAFRA1
  direction: east
- name: Crypt Keeper
  place: {scope: "dungeon:crypt", tag: entrance}
  sprite: 4_F_KAFRA1
  shop: [{item: Red_Potion}]
`,
	})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	sprites := func(name string) (uint32, bool) { return 112, name == "4_F_KAFRA1" }
	npcs, _, err := s.NPCs(sprites)
	if err != nil || len(npcs) != 2 {
		t.Fatalf("npcs %+v, %v", npcs, err)
	}
	if n := npcs[0]; n.Place == nil || n.Place.Scope.String() != "town:any" || n.Place.Tag != "blacksmith" || n.Map != "" || !n.Placed || n.Direction != 6 {
		t.Errorf("smith helper %+v", n)
	}
	if n := npcs[1]; n.Place == nil || n.Place.Scope.String() != "dungeon:crypt" || n.Shop == nil || !n.Placed {
		t.Errorf("crypt keeper %+v", n)
	}
	for _, c := range []struct{ record, want string }{
		{"place: {scope: \"town:any\", tag: blacksmith}\n  map: prontera", "map does not go with place"},
		{"place: {scope: \"town:any\", tag: blacksmith}\n  x: 3", "x does not go with place"},
		{"place: {scope: field:any, tag: blacksmith}", "a scope is"},
		{"place: {scope: \"town:any\", tag: Black Smith}", "is not 1 to 64 lowercase"},
		{"place: {scope: \"town:any\"}", "place is {scope, tag}"},
		{"place: town:any", "place is {scope, tag}"},
		{"place: {scope: \"town:any\", tag: smith, x: 1}", "place is {scope, tag}"},
	} {
		dir := t.TempDir()
		writeMod(t, dir, "a", map[string]string{
			"mod.yaml":      manifest("example.a"),
			"data/npcs.yml": "- name: Bad\n  sprite: 4_F_KAFRA1\n  " + c.record + "\n",
		})
		s, err := resolve(t, dir, Config{})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.NPCs(sprites); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: %v, want %q", c.record, err, c.want)
		}
	}
}
