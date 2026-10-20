package mod

import (
	"strings"
	"testing"
)

// TestNPCStatesAndFloating: a record may start disabled, hidden or
// cloaked, have no sprite, or have no map when it has a script (review
// decision 58).
func TestNPCStatesAndFloating(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{
		"mod.yaml":        manifest("example.a"),
		"scripts/npc.lua": "return function() end",
		"data/npcs.yml": `
- name: Door
  map: prontera
  x: 10
  y: 10
  sprite: 4_F_KAFRA1
  state: disabled
  script: scripts/npc.lua
- name: Trap
  map: prontera
  x: 11
  y: 10
  sprite: none
  touch: {x_span: 1, y_span: 1}
  script: scripts/npc.lua
- name: Ghost#g1
  display_name: Ghost
  script: scripts/npc.lua
`,
	})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	sprites := func(name string) (uint32, bool) { return 112, name == "4_F_KAFRA1" }
	npcs, _, err := s.NPCs(sprites)
	if err != nil || len(npcs) != 3 {
		t.Fatalf("npcs %+v, %v", npcs, err)
	}
	if n := npcs[0]; n.State != "disabled" || n.NoSprite || n.Floating {
		t.Errorf("door %+v", n)
	}
	if n := npcs[1]; !n.NoSprite || n.Sprite != 0 || n.Touch == nil {
		t.Errorf("trap %+v", n)
	}
	if n := npcs[2]; !n.Floating || !n.Placed || n.Map != "" || n.Script != "scripts/npc.lua" || n.DisplayName != "Ghost" {
		t.Errorf("ghost %+v", n)
	}
	for _, c := range []struct{ record, want string }{
		{"map: prontera\n  x: 1\n  y: 1\n  sprite: 4_F_KAFRA1\n  state: gone", "state is disabled, hidden or cloaked"},
		{"script: scripts/npc.lua\n  x: 3", "x needs a map: an NPC without a map has no place"},
		{"script: scripts/npc.lua\n  sprite: 4_F_KAFRA1", "sprite needs a map"},
		{"script: scripts/npc.lua\n  state: hidden", "state needs a map"},
		{"map: prontera\n  x: 1\n  y: 1", "sprite is required"},
	} {
		dir := t.TempDir()
		writeMod(t, dir, "a", map[string]string{
			"mod.yaml":        manifest("example.a"),
			"scripts/npc.lua": "return function() end",
			"data/npcs.yml":   "- name: Bad\n  " + c.record + "\n",
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
