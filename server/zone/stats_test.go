package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

func jobTables(t *testing.T) *status.Tables {
	t.Helper()
	var d status.Data
	if err := reference.LoadFile("extracts/renewal/job_stats.json", &d); err != nil {
		t.Fatal(err)
	}
	tb, err := status.New(&d)
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

func TestStatsFollowLevels(t *testing.T) {
	tb := jobTables(t)
	start := status.Character{Job: 0, Male: true, Stats: status.Stats{Str: 90, Agi: 50, Vit: 30, Int: 1, Dex: 40, Luk: 1}}
	calc := func(base, job int) status.Result {
		c := start
		c.BaseLevel, c.JobLevel = base, job
		r, err := tb.Calc(c)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	z := New("map/test", open(40), rng.FromKey([32]byte{4}).Stream(), nil)
	z.Progress = testTables()
	z.Status, z.Start = tb, start
	p := add(z, "a")
	z.stepAt(50)

	lv1 := calc(1, 1)
	got := stats(events(p))
	want := map[wathenav1.StatId]int64{
		wathenav1.StatId_STAT_ID_HP:         int64(lv1.Battle.MaxHP),
		wathenav1.StatId_STAT_ID_MAX_HP:     int64(lv1.Battle.MaxHP),
		wathenav1.StatId_STAT_ID_SP:         int64(lv1.Battle.MaxSP),
		wathenav1.StatId_STAT_ID_MAX_SP:     int64(lv1.Battle.MaxSP),
		wathenav1.StatId_STAT_ID_STR:        90,
		wathenav1.StatId_STAT_ID_LUK:        1,
		wathenav1.StatId_STAT_ID_ATK:        int64(lv1.Battle.BAtk),
		wathenav1.StatId_STAT_ID_MATK:       int64(lv1.Battle.MAtkMin),
		wathenav1.StatId_STAT_ID_DEF:        int64(lv1.Battle.Def + lv1.Battle.Def2),
		wathenav1.StatId_STAT_ID_MDEF:       int64(lv1.Battle.MDef + lv1.Battle.MDef2),
		wathenav1.StatId_STAT_ID_HIT:        int64(lv1.Battle.Hit),
		wathenav1.StatId_STAT_ID_FLEE:       int64(lv1.Battle.Flee),
		wathenav1.StatId_STAT_ID_CRITICAL:   int64(lv1.Battle.Cri / 10),
		wathenav1.StatId_STAT_ID_ASPD:       int64((2000 - lv1.Battle.AMotion) / 10),
		wathenav1.StatId_STAT_ID_MAX_WEIGHT: 20000 + 90*300,
	}
	for id, v := range want {
		if g, ok := got[id]; !ok || g != v {
			t.Errorf("join %v: %d (sent %v), want %d", id, g, ok, v)
		}
	}
	if p.entity.status.Player == nil {
		t.Error("the battle status has no Player")
	}

	// A base and job level up: the stats follow the new levels, and HP
	// and SP are full.
	p.entity.hp, p.sp = 10, 0
	z.gainExp(p, 10, 5)
	z.stepAt(100)
	lv2 := calc(2, 2)
	if lv2.Battle.MaxHP == lv1.Battle.MaxHP || lv2.Battle.Luk != 2 {
		t.Fatalf("level 2 changes nothing to test: %+v", lv2.Battle)
	}
	got = stats(events(p))
	for id, v := range map[wathenav1.StatId]int64{
		wathenav1.StatId_STAT_ID_BASE_LEVEL: 2,
		wathenav1.StatId_STAT_ID_JOB_LEVEL:  2,
		wathenav1.StatId_STAT_ID_HP:         int64(lv2.Battle.MaxHP),
		wathenav1.StatId_STAT_ID_MAX_HP:     int64(lv2.Battle.MaxHP),
		wathenav1.StatId_STAT_ID_SP:         int64(lv2.Battle.MaxSP),
		wathenav1.StatId_STAT_ID_LUK:        2,
	} {
		if g, ok := got[id]; !ok || g != v {
			t.Errorf("level 2 %v: %d (sent %v), want %d", id, g, ok, v)
		}
	}
	if _, ok := got[wathenav1.StatId_STAT_ID_STR]; ok {
		t.Error("STR sent again without a change")
	}
	if got, want := withoutPlayer(*p.entity.status), withoutPlayer(lv2.Battle); got != want {
		t.Errorf("status %+v, want %+v", got, want)
	}

	// A job level up alone does not heal.
	p.entity.hp = 10
	z.gainExp(p, 0, 5)
	if p.progress.JobLevel != 3 || p.entity.hp != 10 {
		t.Errorf("job level %d, HP %d", p.progress.JobLevel, p.entity.hp)
	}
	if p.entity.status.Dex != calc(2, 3).Battle.Dex {
		t.Errorf("DEX %d after job level 3", p.entity.status.Dex)
	}
}

// withoutPlayer drops a status's Player pointer, so statuses compare by
// value.
func withoutPlayer(s battle.Status) battle.Status {
	s.Player = nil
	return s
}

func TestStatsClampHP(t *testing.T) {
	tb := jobTables(t)
	z := New("map/test", open(40), rng.FromKey([32]byte{5}).Stream(), nil)
	z.Progress = testTables()
	z.Status, z.Start = tb, status.Character{Job: 0, Stats: status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}}
	p := add(z, "a")
	// HP over the maximum drops to it, as status_calc_bl_main.
	p.entity.hp, p.sp = 99999, 99999
	z.recalc(p)
	if p.entity.hp != int64(p.entity.status.MaxHP) || p.sp != int64(p.entity.status.MaxSP) {
		t.Errorf("HP %d of %d, SP %d of %d", p.entity.hp, p.entity.status.MaxHP, p.sp, p.entity.status.MaxSP)
	}
}
