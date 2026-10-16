package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/lxsmnsyc/wathena/server/mob"
	"github.com/lxsmnsyc/wathena/server/spawns"
)

// TestCommitted: the committed spawn tags are what the tool derives from
// rAthena's spawn files today.
func TestCommitted(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	old, err := os.ReadFile(filepath.Join("..", "..", Out))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(old, d.YAML()) {
		t.Errorf("%s is not current; run go run ./tools/spawntags", Out)
	}
}

// TestFamily: map names find their family's biome, and others none.
func TestFamily(t *testing.T) {
	for name, want := range map[string]string{
		"prt_fild08a": "grassland", "moc_fild12": "desert", "gef_dun02": "crypt", "gl_chyard_": "crypt",
		"c_tower2_": "tower", "prontera": "", "gld_dun01": "", "prt_fild": "grassland", "prt_fild_long_name": "",
	} {
		if got := Family(name); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
}

// TestDerive: weights are shares per 1,000 of a biome's monsters, roles
// follow the lines, and bosses become mini or field bosses.
func TestDerive(t *testing.T) {
	x := extract{Maps: []extractMap{
		{Map: "prt_fild01", Walkable: 10000, Total: 51, Spawns: []extractLine{
			{Mob: 1, Amount: 40, RespawnMs: 5000}, {Mob: 2, Amount: 10, RespawnMs: 5000}, {Mob: 3, Amount: 1, RespawnMs: 3600000, Boss: true},
		}},
		{Map: "pay_dun00", Walkable: 10000, Total: 10, Spawns: []extractLine{{Mob: 1, Amount: 10, RespawnMs: 5000}}},
	}}
	defs := map[uint32]*mob.Def{
		1: {ID: 1, AegisName: "A", AI: "02"},
		2: {ID: 2, AegisName: "B", AI: "06"},
		3: {ID: 3, AegisName: "C", AI: "21", Class: 1, Mode: mob.ModeMvp},
	}
	d, err := Derive(x, defs)
	if err != nil {
		t.Fatal(err)
	}
	if d.FieldDensity != 51 || d.DungeonDensity != 10 || len(d.Monsters) != 3 {
		t.Fatalf("%+v", d)
	}
	a, b, c := d.Monsters[0], d.Monsters[1], d.Monsters[2]
	if a.Role != spawns.Common || a.Biomes["grassland"] != 784 || a.Biomes["cave"] != 1000 {
		t.Errorf("A %+v", a)
	}
	if b.Role != spawns.Plant || c.Role != spawns.FieldBoss || c.Biomes["grassland"] != 20 {
		t.Errorf("B %+v, C %+v", b, c)
	}
}
