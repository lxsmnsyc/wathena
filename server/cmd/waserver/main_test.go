package main

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/zone"
)

func TestLoadSpawnsWithRewards(t *testing.T) {
	for _, mode := range reference.Modes {
		tables, data, err := loadProgression(mode)
		if err != nil {
			t.Fatal(err)
		}
		spawns, err := loadSpawns(mode, "prt_fild08", tables, data)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, s := range spawns {
			if s.Mob != 1002 {
				continue
			}
			found = true
			r := s.Reward
			if r == nil || r.Level == 0 || r.BaseExp == 0 || r.JobExp == 0 || len(r.Drops) == 0 {
				t.Fatalf("%s: poring reward %+v", mode, r)
			}
			if s.Status.DamageTaken != 100 {
				t.Errorf("%s: poring DamageTaken %d", mode, s.Status.DamageTaken)
			}
		}
		if !found {
			t.Errorf("%s: no poring on prt_fild08", mode)
		}
	}
}

func TestStartCharacter(t *testing.T) {
	for _, mode := range reference.Modes {
		for build := range builds {
			tables, c, err := startCharacter(mode, build)
			if err != nil {
				t.Fatalf("%s %s: %v", mode, build, err)
			}
			r, err := tables.Calc(c)
			if err != nil || r.Battle.MaxHP == 0 || r.Battle.Player == nil {
				t.Errorf("%s %s: %+v, %v", mode, build, r.Battle, err)
			}
		}
	}
	if _, _, err := startCharacter(reference.Renewal, "nope"); err == nil {
		t.Error("an unknown build loads")
	}
}

func TestLoadItems(t *testing.T) {
	for _, mode := range []reference.Mode{reference.Renewal, reference.PreRenewal} {
		items, err := loadItems(mode)
		if err != nil {
			t.Fatal(err)
		}
		// Red Potion: 7 weight, itemheal rand(45,65),0.
		d := items.Get(501)
		if d.Weight != 70 || d.Heal == nil || d.Heal.HP != [2]int{45, 65} || d.Heal.SP != [2]int{0, 0} {
			t.Errorf("%s: Red Potion %+v", mode, d)
		}
		if want := map[reference.Mode]int{reference.Renewal: 70, reference.PreRenewal: 50}[mode]; items.NaturalHealWeightRate != want {
			t.Errorf("%s: natural_heal_weight_rate %d", mode, items.NaturalHealWeightRate)
		}
	}
}

func TestParseStartItems(t *testing.T) {
	got, err := parseStartItems("501:10,512:2")
	if err != nil || len(got) != 2 || got[0] != (zone.StartItem{ID: 501, Amount: 10}) || got[1] != (zone.StartItem{ID: 512, Amount: 2}) {
		t.Errorf("%v %v", got, err)
	}
	for _, bad := range []string{"501", "0:1", "501:0", "501:30001", "x:1"} {
		if _, err := parseStartItems(bad); err == nil {
			t.Errorf("%q parsed", bad)
		}
	}
}
