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
