package main

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
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
