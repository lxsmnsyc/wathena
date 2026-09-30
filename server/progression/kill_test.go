package progression

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// scripted returns set values and records each draw's n.
type scripted struct {
	vals  []int
	drawn []int
}

func (s *scripted) IntN(n int) int {
	s.drawn = append(s.drawn, n)
	if len(s.vals) == 0 {
		return 0
	}
	v := s.vals[0]
	s.vals = s.vals[1:]
	if v >= n {
		panic("scripted value out of range")
	}
	return v
}

func online(level int) Attacker { return Attacker{Online: true, BaseLevel: level} }

func TestDamageLog(t *testing.T) {
	var l DamageLog
	l.Add(1, 10, 0)
	l.Add(1, 5, 3)
	l.Add(0, 5, 0)
	l.Add(2, -1, 0)
	if len(l.Entries) != 1 || l.Entries[0] != (DamageEntry{CharID: 1, Damage: 15, Tanked: 3}) {
		t.Fatalf("%+v", l.Entries)
	}
	// A miss still adds the player.
	l.Add(2, 0, 0)
	for id := uint32(3); id <= DamageLogSize+2; id++ {
		l.Add(id, 1, 0)
	}
	// The log keeps the newest 20: the first two players left.
	if len(l.Entries) != DamageLogSize || l.Entries[0].CharID != 3 || l.Entries[DamageLogSize-1].CharID != DamageLogSize+2 {
		t.Errorf("%+v", l.Entries)
	}
}

func TestLoggedDamage(t *testing.T) {
	tb := New(&Data{}, nil, DefaultConfig)
	if got := tb.LoggedDamage(500, 20); got != 500 {
		t.Errorf("type 0: %d", got)
	}
	tb.ExpCalcType = 1
	if got := tb.LoggedDamage(500, 20); got != 20 {
		t.Errorf("type 1: %d", got)
	}
}

func TestKillExp(t *testing.T) {
	tb := tables(t, reference.Renewal)
	// Scorpion: level 16, 169 base and 115 job exp.
	scorpion := &Mob{ID: 1001, Level: 16, MaxHP: 136, BaseExp: 169, JobExp: 115}

	// One attacker at level 16 gets all of it.
	var l DamageLog
	l.Add(7, 136, 0)
	k := tb.Kill(scorpion, &l, 7, func(uint32) Attacker { return online(16) })
	if len(k.Rewards) != 1 || k.Rewards[0] != (Reward{7, 169, 115}) || k.First != 7 || k.Second != 0 || k.DropModifier != 100 {
		t.Errorf("%+v", k)
	}
	// At level 1, 15 levels below, renewal gives 115% exp and 60% drops.
	k = tb.Kill(scorpion, &l, 7, func(uint32) Attacker { return online(1) })
	if k.Rewards[0] != (Reward{7, 169 * 115 / 100, 115 * 115 / 100}) || k.DropModifier != 60 {
		t.Errorf("%+v", k)
	}

	// Two attackers share by damage, each with 25% more for the second
	// attacker. The first keeps loot priority with the 30% bonus.
	l = DamageLog{}
	l.Add(1, 30, 0)
	l.Add(2, 10, 0)
	pre := tables(t, reference.PreRenewal)
	mob := &Mob{Level: 10, MaxHP: 40, BaseExp: 100, JobExp: 60}
	k = pre.Kill(mob, &l, 2, func(uint32) Attacker { return online(10) })
	// 100 * 0.75 * 1.25 = 93.75 and 100 * 0.25 * 1.25 = 31.25.
	if len(k.Rewards) != 2 || k.Rewards[0] != (Reward{1, 93, 56}) || k.Rewards[1] != (Reward{2, 31, 18}) {
		t.Errorf("%+v", k.Rewards)
	}
	if k.First != 1 || k.Second != 2 || k.Third != 0 {
		t.Errorf("loot %d %d %d", k.First, k.Second, k.Third)
	}

	// The first attacker's bonus is 30% of the total: 20 + 13 beats 25,
	// but 20 + 18 does not beat 40.
	l = DamageLog{}
	l.Add(1, 20, 0)
	l.Add(2, 25, 0)
	if k = pre.Kill(mob, &l, 2, func(uint32) Attacker { return online(10) }); k.First != 1 || k.Second != 2 {
		t.Errorf("bonus: %d %d", k.First, k.Second)
	}
	l = DamageLog{}
	l.Add(1, 20, 0)
	l.Add(2, 40, 0)
	if k = pre.Kill(mob, &l, 2, func(uint32) Attacker { return online(10) }); k.First != 2 || k.Second != 1 {
		t.Errorf("no bonus: %d %d", k.First, k.Second)
	}

	// A player who left still counts in the total damage, but not as an
	// attacker. A dead player counts as an attacker but gets nothing.
	l = DamageLog{}
	l.Add(1, 20, 0)
	l.Add(2, 20, 0)
	l.Add(3, 40, 0)
	who := map[uint32]Attacker{1: {}, 2: {Online: true, Dead: true, BaseLevel: 10}, 3: online(10)}
	k = pre.Kill(mob, &l, 3, func(id uint32) Attacker { return who[id] })
	// Player 3: 0.5 * 1.25 = 0.625 of the exp.
	if len(k.Rewards) != 1 || k.Rewards[0] != (Reward{3, 62, 37}) || k.First != 3 || k.Second != 0 {
		t.Errorf("%+v", k)
	}

	// Only misses: shares use max HP, and each gets at least 1.
	l = DamageLog{}
	l.Add(1, 0, 0)
	k = pre.Kill(mob, &l, 1, func(uint32) Attacker { return online(10) })
	if len(k.Rewards) != 1 || k.Rewards[0] != (Reward{1, 1, 1}) {
		t.Errorf("misses: %+v", k.Rewards)
	}
	// No exp for monsters without any.
	k = pre.Kill(&Mob{Level: 1, MaxHP: 1}, &l, 1, func(uint32) Attacker { return online(10) })
	if len(k.Rewards) != 0 {
		t.Errorf("no exp: %+v", k.Rewards)
	}
}

func TestKillExpCalcTypes(t *testing.T) {
	cfg := DefaultConfig
	cfg.ExpBonusAttacker = 0
	var l DamageLog
	l.Add(1, 10, 0)
	l.Add(2, 30, 0)
	mob := &Mob{Level: 1, MaxHP: 100, BaseExp: 1000, JobExp: 1000}
	all := func(uint32) Attacker { return online(1) }

	cfg.ExpCalcType = 1
	k := New(&Data{}, nil, cfg).Kill(mob, &l, 2, all)
	if k.Rewards[0].BaseExp != 100 || k.Rewards[1].BaseExp != 300 {
		t.Errorf("type 1: %+v", k.Rewards)
	}
	// Type 2 counts the first attacker's damage twice: 20 of 50.
	cfg.ExpCalcType = 2
	k = New(&Data{}, nil, cfg).Kill(mob, &l, 2, all)
	if k.Rewards[0].BaseExp != 400 || k.Rewards[1].BaseExp != 600 {
		t.Errorf("type 2: %+v", k.Rewards)
	}
	if l.Entries[0].Damage != 10 {
		t.Error("Kill changed the log")
	}
}
