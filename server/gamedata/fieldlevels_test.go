package gamedata

import (
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/worldevent"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// TestFieldLevelsGenerated checks that the fields of a procedural world
// (gen:field:x:y) have levels, as rAthena's *_fild maps do, so a typed
// event with level_band picks them (review decision 62, I4).
func TestFieldLevelsGenerated(t *testing.T) {
	list := func(level int) []zone.Spawn {
		return []zone.Spawn{{Mob: 1, Amount: 4, Reward: &progression.Mob{Level: level}}}
	}
	spawns := map[string][]zone.Spawn{
		gen.FieldID(0, 0):   list(5),
		gen.FieldID(-2, 3):  list(40),
		"prt_fild08":        list(12),
		"prt_dun01":         list(30),
		"gen:dungeon:crypt": list(50),
	}
	got := FieldLevels(spawns)
	want := map[string]int{gen.FieldID(0, 0): 5, gen.FieldID(-2, 3): 40, "prt_fild08": 12}
	if len(got) != len(want) {
		t.Fatalf("FieldLevels %v, want %v", got, want)
	}
	for id, l := range want {
		if got[id] != l {
			t.Errorf("FieldLevels[%s] = %d, want %d (all %v)", id, got[id], l, got)
		}
	}
}

// TestLevelBandPicksGeneratedFields checks that a boss event with
// level_band picks the generated fields of a procedural world. The
// levels are keyed as waserver keys them: "map/" and the field's ID.
func TestLevelBandPicksGeneratedFields(t *testing.T) {
	list := func(level int) []zone.Spawn {
		return []zone.Spawn{{Mob: 1, Amount: 3, Reward: &progression.Mob{Level: level}}}
	}
	levels := FieldLevels(map[string][]zone.Spawn{
		gen.FieldID(0, 0):   list(8),
		gen.FieldID(1, 0):   list(35),
		gen.FieldID(0, 1):   list(12),
		"gen:dungeon:crypt": list(10),
	})
	keyed := map[string]int{}
	for id, l := range levels {
		keyed["map/"+id] = l
	}
	got := worldevent.MapsOf(&worldevent.Band{Min: 5, Max: 15}, nil, keyed)
	want := []string{"map/" + gen.FieldID(0, 0), "map/" + gen.FieldID(0, 1)}
	if !slices.Equal(got, want) {
		t.Errorf("MapsOf = %v, want %v", got, want)
	}
}
