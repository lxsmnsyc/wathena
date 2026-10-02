package status

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// scUnit is a unit for starting statuses in tests.
type scUnit struct {
	kind sc.Kind
	st   battle.Status
	list *sc.List
}

func (u *scUnit) Changes() *sc.List      { return u.list }
func (u *scUnit) Kind() sc.Kind          { return u.kind }
func (u *scUnit) Status() *battle.Status { return &u.st }
func (u *scUnit) Base() *battle.Status   { return &u.st }
func (u *scUnit) Level() int             { return 50 }
func (u *scUnit) JobLevel() int          { return 1 }
func (u *scUnit) HP() int64              { return 1000 }
func (u *scUnit) SP() int64              { return 100 }
func (u *scUnit) Dead() bool             { return false }
func (u *scUnit) ResEff(int) int         { return 0 }
func (u *scUnit) Skill(int) int          { return 0 }
func (u *scUnit) AlwaysEndure() bool     { return false }
func (u *scUnit) Ref() sc.Source         { return sc.Source{Kind: u.kind} }

// scEnv is an engine environment that does nothing.
type scEnv struct{}

func (scEnv) Now() int64                                                { return 0 }
func (scEnv) RandN(int) int                                             { return 0 }
func (scEnv) Recalc(sc.Unit)                                            {}
func (scEnv) Icon(sc.Unit, int, bool, int64, int, int, int)             {}
func (scEnv) OptionChanged(sc.Unit)                                     {}
func (scEnv) StopWalking(sc.Unit)                                       {}
func (scEnv) StopAttacking(sc.Unit)                                     {}
func (scEnv) Damage(sc.Unit, int64, int64, bool, bool) int64            { return 0 }
func (scEnv) Heal(sc.Unit, int64, int64)                                {}
func (scEnv) Started(sc.Unit, *sc.Entry)                                {}
func (scEnv) Ended(sc.Unit, *sc.Entry, sc.Reason)                       {}
func (scEnv) Ticked(sc.Unit, *sc.Entry)                                 {}
func (scEnv) Chance(sc.Unit, *sc.Source, int, int, int) int             { return 10000 }
func (scEnv) Duration(_ sc.Unit, _ *sc.Source, _, _ int, d int64) int64 { return d }

// withStatuses starts statuses on a unit of a kind, as a player would
// start them, and returns its list.
func withStatuses(t *testing.T, mode reference.Mode, kind sc.Kind, starts ...sc.Start) *sc.List {
	t.Helper()
	var d sc.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/status.json", &d); err != nil {
		t.Fatal(err)
	}
	db, err := sc.Load(&d)
	if err != nil {
		t.Fatal(err)
	}
	en := sc.New(db, scEnv{})
	u := &scUnit{kind: kind, list: sc.NewList()}
	src := &scUnit{kind: sc.KindPlayer, list: sc.NewList()}
	for _, s := range starts {
		s.Rate, s.Flag = 10000, sc.NoAvoid
		if s.Duration == 0 {
			s.Duration = 60000
		}
		if !en.Start(src, u, s) {
			t.Fatalf("status %d did not start", s.Type)
		}
	}
	return u.list
}

// TestProvokeStats checks SC_PROVOKE in status_calc_def, status_calc_def2
// and battle_get_atkpercent: monsters lose hard and soft DEF, players
// only soft DEF.
func TestProvokeStats(t *testing.T) {
	for _, mode := range reference.Modes {
		renewal := mode == reference.Renewal
		l := withStatuses(t, mode, sc.KindMonster, sc.Start{Type: sc.SCProvoke, Val1: 5})
		// Level 5: 17% more ATK, 30% less DEF.
		m := CalcMonster(&battle.Status{Level: 50, Def: 50, Def2: 40, MaxHP: 100}, l, renewal)
		if m.Def != 35 || m.Def2 != 28 {
			t.Errorf("%s: monster DEF %d + %d, want 35 + 28", mode, m.Def, m.Def2)
		}
		if m.Changes == nil || m.Changes.AtkPercent != 117 {
			t.Errorf("%s: monster changes %+v", mode, m.Changes)
		}

		tb := load(t, mode)
		c := Character{Job: 1, Male: true, BaseLevel: 50, JobLevel: 10, Stats: builds["tank"]}
		plain, err := tb.Calc(c)
		if err != nil {
			t.Fatal(err)
		}
		c.Changes = withStatuses(t, mode, sc.KindPlayer, sc.Start{Type: sc.SCProvoke, Val1: 10})
		r, err := tb.Calc(c)
		if err != nil {
			t.Fatal(err)
		}
		if r.Battle.Def != plain.Battle.Def {
			t.Errorf("%s: player DEF %d, want %d", mode, r.Battle.Def, plain.Battle.Def)
		}
		if want := plain.Battle.Def2 - plain.Battle.Def2*55/100; r.Battle.Def2 != want {
			t.Errorf("%s: player DEF2 %d, want %d", mode, r.Battle.Def2, want)
		}
		if r.Battle.Changes == nil || r.Battle.Changes.AtkPercent != 132 {
			t.Errorf("%s: player changes %+v", mode, r.Battle.Changes)
		}
	}
}

// TestConcentrateStats checks SC_CONCENTRATE: AGI and DEX rise by a
// percent that leaves out the AGI and DEX of cards, which the stat
// calculation keeps in val3 and val4.
func TestConcentrateStats(t *testing.T) {
	for _, mode := range reference.Modes {
		tb := load(t, mode)
		card := Card{Def: &item.Def{Type: "Card"}, Bonuses: []Bonus{
			{Name: "agi", Args: []int64{5}, Options: opts(nil)},
			{Name: "dex", Args: []int64{7}, Options: opts(nil)},
		}}
		a := armor()
		a.Cards = []Card{card}
		c := Character{Job: 3, Male: true, BaseLevel: 50, JobLevel: 10, Stats: builds["ranged"],
			Equipment: wearing(map[int]*Equipped{item.EqiArmor: a})}
		plain, err := tb.Calc(c)
		if err != nil {
			t.Fatal(err)
		}
		c.Changes = withStatuses(t, mode, sc.KindPlayer, sc.Start{Type: sc.SCConcentrate, Val1: 10})
		r, err := tb.Calc(c)
		if err != nil {
			t.Fatal(err)
		}
		e := c.Changes.Get(sc.SCConcentrate)
		if e.Val3 != 5 || e.Val4 != 7 {
			t.Errorf("%s: val3 %d, val4 %d, want 5 and 7", mode, e.Val3, e.Val4)
		}
		agi, dex := plain.Battle.Agi, plain.Battle.Dex
		if want := agi + (agi-5)*12/100; r.Battle.Agi != want {
			t.Errorf("%s: AGI %d, want %d", mode, r.Battle.Agi, want)
		}
		if want := dex + (dex-7)*12/100; r.Battle.Dex != want {
			t.Errorf("%s: DEX %d, want %d", mode, r.Battle.Dex, want)
		}
	}
}

// TestLoudStats checks SC_LOUD: STR + 4, and renewal's BATK + 30.
func TestLoudStats(t *testing.T) {
	for _, mode := range reference.Modes {
		tb := load(t, mode)
		c := Character{Job: 5, Male: true, BaseLevel: 50, JobLevel: 10, Stats: builds["melee"]}
		plain, err := tb.Calc(c)
		if err != nil {
			t.Fatal(err)
		}
		c.Changes = withStatuses(t, mode, sc.KindPlayer, sc.Start{Type: sc.SCLoud, Val1: 1})
		r, err := tb.Calc(c)
		if err != nil {
			t.Fatal(err)
		}
		if r.Battle.Str != plain.Battle.Str+4 {
			t.Errorf("%s: STR %d, want %d", mode, r.Battle.Str, plain.Battle.Str+4)
		}
		// The base ATK of 4 more STR, then renewal's flat 30.
		c.Changes = nil
		c.Stats.Str += 4
		strong, err := tb.Calc(c)
		if err != nil {
			t.Fatal(err)
		}
		want := strong.Battle.BAtk
		if mode == reference.Renewal {
			want += 30
		}
		if r.Battle.BAtk != want {
			t.Errorf("%s: BATK %d, want %d", mode, r.Battle.BAtk, want)
		}
	}
}

// TestMagnumBreakChanges checks that BattleChanges carries the fire bonus
// of Magnum Break.
func TestMagnumBreakChanges(t *testing.T) {
	l := withStatuses(t, reference.Renewal, sc.KindPlayer, sc.Start{Type: sc.SCSubWeaponPro, Val1: battle.Fire, Val2: 20, Val3: 7})
	if c := BattleChanges(l); c.SubWeaponElement != battle.Fire || c.SubWeaponRate != 20 || c.WatkElementRate != 0 {
		t.Errorf("renewal changes %+v", c)
	}
	l = withStatuses(t, reference.PreRenewal, sc.KindPlayer, sc.Start{Type: sc.SCWatkElement, Val1: battle.Fire, Val2: 20})
	if c := BattleChanges(l); c.WatkElement != battle.Fire || c.WatkElementRate != 20 || c.SubWeaponRate != 0 {
		t.Errorf("pre-renewal changes %+v", c)
	}
}
