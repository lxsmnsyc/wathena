package gamedata

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/progression"
)

// overlaySet makes a set of core and one mod with a monsters.yml.
func overlaySet(t *testing.T, monsters string) *mod.Set {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"a/mod.yaml":          "id: example.a\nname: A\nversion: 1.0.0\napi: experimental\n",
		"a/data/monsters.yml": monsters,
	}
	for p, s := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	found, err := mod.Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	set, err := mod.Resolve(found, mod.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func baseMonsters() map[uint32]Monster {
	st := &battle.Status{Level: 1, MaxHP: 50, Mode: 0x81, RHW: battle.Weapon{Atk: 7, Atk2: 10, Range: 1}}
	return map[uint32]Monster{1002: {Status: st, AegisName: "PORING", Name: "Poring",
		Reward: &progression.Mob{ID: 1002, Level: 1, MaxHP: 50, BaseExp: 2, JobExp: 1, Drops: []progression.Drop{{Item: 909, Rate: 7000}}}}}
}

func TestMonsterOverlay(t *testing.T) {
	set := overlaySet(t, `
- name: example.a:BIG_PORING
  sprite: PORING
  display_name: Big Poring
  level: 20
  hp: 900
  attack: [30, 40]
  class: boss
  modes: [can_move]
  base_exp: 100
  drops: [{item: Jellopy, rate: 5000}]
- name: PORING
  hp: 80
`)
	ids, _ := mod.LoadIDs(context.Background(), &mod.MemoryIDs{})
	tables := progression.New(&progression.Data{}, map[uint32]string{909: "Etc"}, progression.DefaultConfig)
	base := baseMonsters()
	out, names, _, err := monsterOverlay(context.Background(), set, base, nil, map[string]uint32{"Jellopy": 909}, tables, ids)
	if err != nil {
		t.Fatal(err)
	}
	big := out[100_000]
	if names["EXAMPLE.A:BIG_PORING"] != 100_000 || big.Name != "Big Poring" || big.Sprite != 1002 || big.Status.MaxHP != 900 || big.Status.RHW.Atk2 != 40 {
		t.Fatalf("big poring %+v %+v", big, big.Status)
	}
	if big.Status.Class != 1 || big.Status.Mode != 0x1|modeDetector|modeStatusImmune|modeKnockbackImmune {
		t.Errorf("class %d mode %#x", big.Status.Class, big.Status.Mode)
	}
	if r := big.Reward; r.ID != 100_000 || r.Level != 20 || r.BaseExp != 100 || r.JobExp != 0 || len(r.Drops) != 1 || r.Drops[0].Rate != 5000 {
		t.Errorf("reward %+v", r)
	}
	if out[1002].Status.MaxHP != 80 || base[1002].Status.MaxHP != 50 {
		t.Errorf("the core poring has %d HP, and the original %d", out[1002].Status.MaxHP, base[1002].Status.MaxHP)
	}
}

func TestMonsterOverlayErrors(t *testing.T) {
	set := overlaySet(t, `
- name: example.a:NO_SPRITE
  hp: 10
- name: example.a:BAD
  sprite: PORING
  hp: -1
  attack: [9, 3]
  class: king
  modes: [flying]
  colour: red
  drops: [{item: Gold_Brick, rate: 10}]
`)
	ids, _ := mod.LoadIDs(context.Background(), &mod.MemoryIDs{})
	tables := progression.New(&progression.Data{}, nil, progression.DefaultConfig)
	_, _, _, err := monsterOverlay(context.Background(), set, baseMonsters(), nil, map[string]uint32{}, tables, ids)
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{"a new monster needs sprite", "hp must be", "attack must be", `class: "king"`, `"flying" is not a mode`, `"colour" is not a field`, `there is no item "Gold_Brick"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}
