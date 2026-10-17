package statuschange_test

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// TestProvokeValues checks status_change_start's SC_PROVOKE: the ATK and
// DEF percents by level, and the level 10 Provoke of a monster.
func TestProvokeValues(t *testing.T) {
	for _, mode := range reference.Modes {
		en, _ := newEngine(t, mode)
		p := testPlayer()
		m := newUnit(sc.KindMonster, battle.Status{Level: 50, MaxHP: 1000})
		if !en.Start(p, m, sc.Start{Type: sc.SCProvoke, Rate: 10000, Val1: 5, Duration: 30000, Flag: sc.NoAvoid}) {
			t.Fatalf("%s: provoke did not start", mode)
		}
		if x := m.list.Get(sc.SCProvoke); x.Val2 != 17 || x.Val3 != 30 {
			t.Errorf("%s: level 5: %+v", mode, x)
		}
		// A monster's level 10 Provoke raises no ATK and takes all DEF.
		q := testPlayer()
		en.Start(m, q, sc.Start{Type: sc.SCProvoke, Rate: 10000, Val1: 10, Duration: 30000, Flag: sc.NoAvoid})
		if x := q.list.Get(sc.SCProvoke); x.Val2 != 0 || x.Val3 != 100 {
			t.Errorf("%s: monster level 10: %+v", mode, x)
		}
	}
}

// TestProvokeEndsSleep checks Provoke's EndOnStart: it wakes the target.
func TestProvokeEndsSleep(t *testing.T) {
	en, _ := newEngine(t, reference.Renewal)
	p := testPlayer()
	m := newUnit(sc.KindMonster, battle.Status{Level: 50, MaxHP: 1000})
	en.Start(p, m, sc.Start{Type: sc.SCSleep, Rate: 10000, Duration: 30000, Flag: sc.NoAvoid})
	if !m.list.Has(sc.SCSleep) {
		t.Fatal("sleep did not start")
	}
	en.Start(p, m, sc.Start{Type: sc.SCProvoke, Rate: 10000, Val1: 1, Duration: 30000, Flag: sc.NoAvoid})
	if m.list.Has(sc.SCSleep) || !m.list.Has(sc.SCProvoke) {
		t.Errorf("sleep %v, provoke %v", m.list.Has(sc.SCSleep), m.list.Has(sc.SCProvoke))
	}
}

// TestAutoBerserk checks SC_AUTOBERSERK's timer: at a quarter of max HP
// or less, Provoke level 10 starts with no end; above, it ends; and it
// ends with Auto Berserk.
func TestAutoBerserk(t *testing.T) {
	en, e := newEngine(t, reference.Renewal)
	p := testPlayer()
	if !en.Start(p, p, sc.Start{Type: sc.SCAutoberserk, Rate: 10000, Val1: 1, Duration: 60000, Flag: sc.NoAvoid}) {
		t.Fatal("auto berserk did not start")
	}
	if x := p.list.Get(sc.SCAutoberserk); x.Remaining(0) != sc.InfiniteTick {
		t.Errorf("auto berserk ends at %d", x.Remaining(0))
	}
	e.now = 100
	en.Advance(e.now)
	if p.list.Has(sc.SCProvoke) {
		t.Fatal("provoke at full HP")
	}
	p.hp = 250
	e.now = 200
	en.Advance(e.now)
	x := p.list.Get(sc.SCProvoke)
	if x == nil || x.Val1 != 10 || x.Val2 != 32 || x.Val3 != 55 || x.Val4 != 1 || x.Remaining(e.now) != sc.InfiniteTick {
		t.Fatalf("provoke %+v", x)
	}
	p.hp = 251
	e.now = 300
	en.Advance(e.now)
	if p.list.Has(sc.SCProvoke) {
		t.Fatal("provoke stayed above a quarter of max HP")
	}
	p.hp = 100
	e.now = 400
	en.Advance(e.now)
	if !p.list.Has(sc.SCProvoke) {
		t.Fatal("provoke did not start again")
	}
	en.End(p, sc.SCAutoberserk, sc.ReasonRemoved)
	if p.list.Has(sc.SCProvoke) {
		t.Error("provoke stayed after auto berserk ended")
	}
	if en.Next() != -1 {
		e.now = 10000
		en.Advance(e.now)
		if p.list.Has(sc.SCProvoke) || p.list.Has(sc.SCAutoberserk) {
			t.Error("a timer started a status again")
		}
	}
}

// TestRemoveMapEndsAutoProvoke checks unit_remove_map's status part: the
// Provoke of Auto Berserk ends, a Provoke from the skill stays, and
// statuses with RemoveOnChangeMap end.
func TestRemoveMapEndsAutoProvoke(t *testing.T) {
	en, _ := newEngine(t, reference.Renewal)
	p := testPlayer()
	en.Start(p, p, sc.Start{Type: sc.SCProvoke, Rate: 10000, Val1: 10, Val4: 1, Duration: sc.InfiniteTick, Flag: sc.NoAvoid})
	en.Start(p, p, sc.Start{Type: sc.SCTrickdead, Rate: 10000, Val1: 1, Duration: 600000, Flag: sc.NoAvoid})
	en.RemoveMap(p)
	if p.list.Has(sc.SCProvoke) || p.list.Has(sc.SCTrickdead) {
		t.Errorf("provoke %v, trick dead %v", p.list.Has(sc.SCProvoke), p.list.Has(sc.SCTrickdead))
	}
	q := testPlayer()
	en.Start(q, q, sc.Start{Type: sc.SCProvoke, Rate: 10000, Val1: 5, Duration: 30000, Flag: sc.NoAvoid})
	en.RemoveMap(q)
	if !q.list.Has(sc.SCProvoke) {
		t.Error("the skill's provoke ended")
	}
}

// TestSkillStatusStarts checks the start values of the other statuses
// that first-class skills start.
func TestSkillStatusStarts(t *testing.T) {
	en, _ := newEngine(t, reference.Renewal)
	p := testPlayer()
	en.Start(p, p, sc.Start{Type: sc.SCTrickdead, Rate: 10000, Val1: 1, Duration: 600000, Flag: sc.NoAvoid})
	if x := p.list.Get(sc.SCTrickdead); x == nil || x.Remaining(0) != sc.InfiniteTick {
		t.Errorf("trick dead %+v", x)
	}
	en.Start(p, p, sc.Start{Type: sc.SCConcentrate, Rate: 10000, Val1: 10, Val3: 7, Val4: 9, Duration: 240000, Flag: sc.NoAvoid})
	if x := p.list.Get(sc.SCConcentrate); x == nil || x.Val2 != 12 || x.Val3 != 0 || x.Val4 != 0 {
		t.Errorf("concentrate %+v", x)
	}
	for _, typ := range []int{sc.SCLoud, sc.SCEnergycoat, sc.SCSafetywall, sc.SCPneuma} {
		if !en.Start(p, p, sc.Start{Type: typ, Rate: 10000, Val1: 1, Duration: 10000, Flag: sc.NoAvoid}) {
			t.Errorf("status %d did not start", typ)
		}
	}
	en.Start(p, p, sc.Start{Type: sc.SCSubWeaponPro, Rate: 10000, Val1: 3, Val2: 20, Val3: 7, Duration: 10000, Flag: sc.NoAvoid})
	if x := p.list.Get(sc.SCSubWeaponPro); x == nil || x.Val1 != 3 || x.Val2 != 20 || x.Remaining(0) != 10000 {
		t.Errorf("sub weapon property %+v", x)
	}
	// Casting Magnum Break again replaces the bonus (EndOnStart).
	en.Start(p, p, sc.Start{Type: sc.SCSubWeaponPro, Rate: 10000, Val1: 3, Val2: 20, Val3: 7, Duration: 5000, Flag: sc.NoAvoid})
	if x := p.list.Get(sc.SCSubWeaponPro); x == nil || x.Remaining(0) != 5000 {
		t.Errorf("sub weapon property again %+v", x)
	}

	pre, _ := newEngine(t, reference.PreRenewal)
	q := testPlayer()
	if !pre.Start(q, q, sc.Start{Type: sc.SCWatkElement, Rate: 10000, Val1: 3, Val2: 20, Duration: 10000, Flag: sc.NoAvoid}) {
		t.Fatal("watk element did not start")
	}
	if x := q.list.Get(sc.SCWatkElement); x.Val1 != 3 || x.Val2 != 20 {
		t.Errorf("watk element %+v", x)
	}
}

// TestSubWeaponPropertyEDP checks that Research New Poison makes the
// SC_SUB_WEAPONPROPERTY of Enchant Deadly Poison last longer.
func TestSubWeaponPropertyEDP(t *testing.T) {
	en, _ := newEngine(t, reference.Renewal)
	p := testPlayer()
	p.skills = map[int]int{2024: 2}
	en.Start(p, p, sc.Start{Type: sc.SCSubWeaponPro, Rate: 10000, Val1: 5, Val2: 10, Val3: 378, Duration: 10000, Flag: sc.NoAvoid})
	if x := p.list.Get(sc.SCSubWeaponPro); x == nil || x.Remaining(0) != 10000+30000+30000 {
		t.Errorf("sub weapon property %+v", x)
	}
}

// TestMonsterCloakingEnds checks SC_CLOAKING and SC_MAXIMIZEPOWER on a
// monster, which cannot consume SP: each ends at its first timer, 10
// seconds in, as rAthena's status_damage takes no SP from it. Before,
// the timer charged the monster and ran again at once, forever, with
// a status_tick each time.
func TestMonsterCloakingEnds(t *testing.T) {
	for _, typ := range []int{sc.SCCloaking, sc.SCMaximizepower} {
		en, e := newEngine(t, reference.Renewal)
		m := newUnit(sc.KindMonster, battle.Status{Level: 50, MaxHP: 1000, MaxSP: 10})
		if !en.Start(m, m, sc.Start{Type: typ, Rate: 10000, Val1: 5, Duration: 30000, Flag: sc.NoAvoid}) {
			t.Fatalf("status %d did not start", typ)
		}
		e.now = 9999
		en.Advance(e.now)
		if !m.list.Has(typ) {
			t.Fatalf("status %d ended early", typ)
		}
		e.now = 10000
		en.Advance(e.now)
		if m.list.Has(typ) || en.Next() >= 0 && en.Next() <= e.now {
			t.Errorf("status %d: still on %v, next timer %d", typ, m.list.Has(typ), en.Next())
		}
		if len(e.ticked) > 1 {
			t.Errorf("status %d ticked %d times", typ, len(e.ticked))
		}
	}
}
