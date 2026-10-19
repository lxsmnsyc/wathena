package gamedata

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// TestSpawnDelays checks npc_parse_mob's reading of a spawn line's
// delays: a delay below mob_respawn_time is 5 seconds, only boss lines
// keep their variance (mob_spawn_variance 1), and a delay above
// 0xfffffff skips the line.
func TestSpawnDelays(t *testing.T) {
	for _, c := range []struct {
		delay, variance int
		boss            bool
		want, wantVar   int
		ok              bool
	}{
		{0, 0, false, 5000, 0, true},
		{999, 0, true, 5000, 0, true},
		{1000, 0, false, 1000, 0, true},
		{60000, 30000, false, 60000, 0, true},
		{3600000, 600000, true, 3600000, 600000, true},
		{0x10000000, 0, true, 0, 0, false},
		{5000, 0x10000000, true, 0, 0, false},
	} {
		got, gotVar, ok := spawnDelays(c.delay, c.variance, c.boss)
		if got != c.want || gotVar != c.wantVar || ok != c.ok {
			t.Errorf("%d, %d, boss %v: got %d, %d, %v; want %d, %d, %v", c.delay, c.variance, c.boss, got, gotVar, ok, c.want, c.wantVar, c.ok)
		}
	}
}

// TestSpawnsKeepBoss checks that spawns keep rAthena's boss flag and
// respawn variance from spawns.json, with the delays npc_parse_mob
// reads.
func TestSpawnsKeepBoss(t *testing.T) {
	monsters, err := LoadMonsters(reference.Renewal, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	all, err := SpawnsOf(reference.Renewal, monsters)
	if err != nil {
		t.Fatal(err)
	}
	find := func(m string, mob uint32) bool {
		for _, s := range all[m] {
			if s.Mob != mob {
				continue
			}
			switch mob {
			case 1150: // Moonlight Flower: boss_monster at a fixed cell.
				if !s.Boss || s.RespawnMs != 3600000 || s.RespawnVarianceMs != 600000 || !s.FixedCell || s.X != 120 || s.Y != 115 {
					t.Errorf("Moonlight Flower %+v", s)
				}
			case 1871: // Fallen Bishop: boss_monster in an area.
				if !s.Boss || s.RespawnMs != 7200000 || s.RespawnVarianceMs != 600000 || s.FixedCell {
					t.Errorf("Fallen Bishop %+v", s)
				}
			case 1068: // Hydra: monster with a variance, which it loses.
				if s.Boss || s.RespawnMs != 60000 || s.RespawnVarianceMs != 0 || s.FixedCell {
					t.Errorf("Hydra %+v", s)
				}
			}
			return true
		}
		return false
	}
	for _, c := range []struct {
		m   string
		mob uint32
	}{{"pay_dun04", 1150}, {"abbey02", 1871}, {"ama_fild01", 1068}} {
		if !find(c.m, c.mob) {
			t.Errorf("no spawn of %d on %s", c.mob, c.m)
		}
	}
	bosses, delays := 0, 0
	for _, list := range all {
		for _, s := range list {
			if s.Boss {
				bosses++
			}
			if s.RespawnMs < 1000 {
				delays++
			}
		}
	}
	if bosses < 50 || delays != 0 {
		t.Errorf("%d boss spawns, %d below 1 second", bosses, delays)
	}
}
