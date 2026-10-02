package zone

import (
	"maps"
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/modscript/luaapi"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestAPIFields checks that the zone reads every field that
// server/modscript/luaapi defines for players, monsters and NPCs, and
// no other. Equipped items wait for equipment (phase 4).
func TestAPIFields(t *testing.T) {
	z := New("map/test", open(40), rng.FromKey([32]byte{3}).Stream(), nil)
	z.Items = testItems()
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	z.SetNPCs([]NPC{{Name: "example.test:Guide", DisplayName: "Guide", X: 20, Y: 20, Sprite: 112}})
	p := add(z, "a")
	st := &battle.Status{MaxHP: 50}
	e := z.spawnOne(&Spawn{Mob: 1002, Name: "Poring", Amount: 1, Status: st})
	z.monsterHandles = map[int64]*entity{1: e}
	h := scriptHost{z}
	names := map[string][]string{
		"wa.Player":  keys(playerFields),
		"wa.Monster": keys(monsterFields),
		"wa.Npc":     keys(npcFields),
	}
	reads := map[string]func(field string) error{
		"wa.Player":  func(f string) error { _, err := h.PlayerField(int64(p.charID), f); return err },
		"wa.Monster": func(f string) error { _, err := h.MonsterField(1, f); return err },
		"wa.Npc":     func(f string) error { _, err := h.NPCField(1, f); return err },
	}
	for _, c := range luaapi.Classes {
		read := reads[c.Name]
		if read == nil {
			continue
		}
		for _, f := range c.Fields {
			if err := read(f.Name); err != nil {
				t.Errorf("%s.%s is defined, and the zone says: %v", c.Name, f.Name, err)
			}
		}
		if err := read("no_such_field"); err == nil {
			t.Errorf("%s reads a field that does not exist", c.Name)
		}
		var defined []string
		for _, f := range c.Fields {
			defined = append(defined, f.Name)
		}
		slices.Sort(defined)
		if !slices.Equal(defined, names[c.Name]) {
			t.Errorf("%s: the zone has %v, the definitions %v", c.Name, names[c.Name], defined)
		}
		delete(reads, c.Name)
	}
	for name := range reads {
		t.Errorf("%s is not defined", name)
	}
}

func keys[V any](m map[string]V) []string { return slices.Sorted(maps.Keys(m)) }
