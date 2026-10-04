package status

import (
	"fmt"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/rules/rulestest"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Tests of the ruleset flags that player stats read (decision 0019),
// each at both values on both bases.

// ruled returns a mode's tables with the core rules of their base,
// changed by set.
func ruled(t *testing.T, mode reference.Mode, set func(*rules.Rules)) *Tables {
	t.Helper()
	tb := load(t, mode)
	rl := rules.Base(mode == reference.Renewal)
	set(rl)
	tb.SetRules(rl)
	return tb
}

// TestStatPointCostFlag checks both values of the stat_point_cost flag
// on both bases, against PC_STATUS_POINT_COST of each mode:
//   - Renewal: 2 + (low - 1) / 10 below 100, and 16 + 4 per 5 above.
//   - Pre-renewal: 1 + (low + 9) / 10.
//
// The two formulas agree below 100, so they only differ for the third
// jobs of a renewal base, whose stats go to 130.
func TestStatPointCostFlag(t *testing.T) {
	for _, mode := range reference.Modes {
		for _, flag := range []bool{true, false} {
			tb := ruled(t, mode, func(r *rules.Rules) { r.StatPointCost = flag })
			job, top := 1, 99
			if mode == reference.Renewal {
				job, top = 4054, 130 // Rune Knight.
			}
			c := Character{Job: job, Male: true, BaseLevel: 99, JobLevel: 10}
			for low := 1; low < top; low++ {
				c.Stats.Str = low
				want := 1 + (low+9)/10
				if flag {
					want = 2 + (low-1)/10
					if low >= 100 {
						want = 16 + 4*((low-100)/5)
					}
				}
				if got := tb.NeedStatusPoint(&c, Str, 1); got != want {
					t.Errorf("%s base, stat_point_cost renewal %v: STR %d to %d costs %d, want %d", mode, flag, low, low+1, got, want)
				}
			}
		}
	}
}

// TestFourthJobTraitsFlag checks both values of the fourth_job_traits
// flag on a renewal base: a Dragon Knight with trait stats from its job
// and a +30 trait stat item has P.ATK, RES and its CON's HIT with the
// flag on, and none of them with it off. A pre-renewal base, where the
// loader refuses the flag, has no trait stats either way.
func TestFourthJobTraitsFlag(t *testing.T) {
	item := armor(Bonus{Name: "all_trait_stats", Args: []int64{30}, Options: opts(nil)})
	for _, mode := range reference.Modes {
		for _, flag := range []bool{true, false} {
			tb := ruled(t, mode, func(r *rules.Rules) { r.FourthJobTraits = flag })
			job := 4252 // Dragon Knight.
			if mode == reference.PreRenewal {
				job = 7 // Knight.
			}
			c := Character{Job: job, Male: true, BaseLevel: 99, JobLevel: 50, Stats: builds["melee"], Equipment: wearing(map[int]*Equipped{2: item})}
			res, err := tb.Calc(c)
			if err != nil {
				t.Fatal(err)
			}
			b := res.Battle
			has := b.PAtk > 0 && b.Res > 0 && b.MRes > 0 && b.SMatk > 0 && b.HPlus > 0 && b.CRate > 0
			none := b.PAtk == 0 && b.Res == 0 && b.MRes == 0 && b.SMatk == 0 && b.HPlus == 0 && b.CRate == 0
			switch {
			case mode == reference.Renewal && flag && !has:
				t.Errorf("renewal, on: trait stats missing: %+v", b)
			case (mode == reference.PreRenewal || !flag) && !none:
				t.Errorf("%s, on %v: trait stats %d %d %d %d %d %d", mode, flag, b.PAtk, b.Res, b.MRes, b.SMatk, b.HPlus, b.CRate)
			}
		}
	}
	// HIT and FLEE count 2 per CON with the flag on.
	on := ruled(t, reference.Renewal, func(r *rules.Rules) { r.FourthJobTraits = true })
	off := ruled(t, reference.Renewal, func(r *rules.Rules) { r.FourthJobTraits = false })
	c := Character{Job: 4252, Male: true, BaseLevel: 99, JobLevel: 50, Stats: builds["melee"], Equipment: wearing(map[int]*Equipped{2: item})}
	a, _ := on.Calc(c)
	b, _ := off.Calc(c)
	if a.Battle.Hit-b.Battle.Hit < 60 || a.Battle.Flee-b.Battle.Flee < 60 {
		t.Errorf("CON's HIT and FLEE: on %d/%d, off %d/%d", a.Battle.Hit, a.Battle.Flee, b.Battle.Hit, b.Battle.Flee)
	}
}

// TestCastBonusFlag checks both values of the cast_time flag in pc_bonus
// on both bases (RENEWAL_CAST): bCastrate -10 is renewal's variable cast
// reduction of 10, or pre-renewal's cast rate of 90. bFixedCastrate only
// counts with renewal's cast time.
func TestCastBonusFlag(t *testing.T) {
	items := map[int]*Equipped{2: armor(
		Bonus{Name: "variable_cast_time", Args: []int64{-10}, Options: opts(map[string]any{"percent": true})},
		Bonus{Name: "fixed_cast_time", Args: []int64{-20}, Options: opts(map[string]any{"percent": true})},
	)}
	for _, mode := range reference.Modes {
		for _, flag := range []bool{true, false} {
			tb := ruled(t, mode, func(r *rules.Rules) { r.CastTime = flag })
			res, err := tb.Calc(Character{Job: 0, Male: true, BaseLevel: 50, JobLevel: 10, Stats: builds["magic"], Equipment: wearing(items)})
			if err != nil {
				t.Fatal(err)
			}
			cb := res.Battle.Player.Cast
			if flag && (cb.VarCastRate != 10 || cb.CastRate != 100 || cb.FixCastRate != -20) ||
				!flag && (cb.VarCastRate != 0 || cb.CastRate != 90 || cb.FixCastRate != 0) {
				t.Errorf("%s base, cast_time renewal %v: %+v", mode, flag, cb)
			}
		}
	}
}

// TestEnsembleSongsStats checks both values of the ensemble_songs flag in
// the stats on both bases: Siegfried resists 4 elements in renewal and 9
// in pre-renewal, so a player under it resists poison only with the
// pre-renewal songs.
func TestEnsembleSongsStats(t *testing.T) {
	for _, mode := range reference.Modes {
		l := withStatuses(t, mode, sc.KindPlayer, sc.Start{Type: sc.SCSiegfried, Val1: 5, Val2: 30})
		for _, flag := range []bool{true, false} {
			tb := ruled(t, mode, func(r *rules.Rules) { r.EnsembleSongs = flag })
			res, err := tb.Calc(Character{Job: 0, Male: true, BaseLevel: 50, JobLevel: 10, Stats: builds["melee"], Changes: l})
			if err != nil {
				t.Fatal(err)
			}
			sub := res.Battle.Player.SubEle
			if sub[eleWater] == 0 || (sub[elePoison] != 0) != !flag {
				t.Errorf("%s base, ensemble_songs renewal %v: element resistance %v", mode, flag, sub)
			}
		}
	}
}

// TestRebalancedStatusEffects checks a skill_rebalance flag in the
// stats on both bases: renewal's Blessing adds 2 HIT per level, and
// pre-renewal's none.
func TestRebalancedStatusEffects(t *testing.T) {
	for _, mode := range reference.Modes {
		l := withStatuses(t, mode, sc.KindPlayer, sc.Start{Type: sc.SCBlessing, Val1: 10})
		hit := map[bool]int{}
		for _, flag := range []bool{true, false} {
			tb := ruled(t, mode, func(r *rules.Rules) { r.Skills = map[int]bool{skillBlessing: flag} })
			res, err := tb.Calc(Character{Job: 0, Male: true, BaseLevel: 50, JobLevel: 10, Stats: builds["melee"], Changes: l})
			if err != nil {
				t.Fatal(err)
			}
			hit[flag] = res.Battle.Hit
		}
		if hit[true]-hit[false] != 20 {
			t.Errorf("%s base: HIT %d with renewal's Blessing and %d with pre-renewal's, want 20 apart", mode, hit[true], hit[false])
		}
	}
}

// TestMixedRulesetsStatus runs pc_status.json of the base for each
// sampled mixed ruleset (rulestest.Mixes) whose aspd and ensemble_songs
// flags are the base's: renewal's Bards and Dancers get SP from their
// lesson with the renewal songs. The other flags must not change the
// recorded players, who have no equipment, statuses or trait stats.
func TestMixedRulesetsStatus(t *testing.T) {
	testrace.SkipUnderRace(t, "it checks sampled mixed rulesets on one goroutine")
	ran := 0
	for i, rl := range rulestest.Mixes() {
		if rl.ASPD != rl.Renewal || rl.EnsembleSongs != rl.Renewal {
			continue
		}
		mode := reference.PreRenewal
		if rl.Renewal {
			mode = reference.Renewal
		}
		t.Run(fmt.Sprintf("%s mix %d", mode, i), func(t *testing.T) {
			checkRecordedPlayers(t, mode, rl)
		})
		ran++
	}
	if ran == 0 {
		t.Error("no mix ran pc_status.json")
	}
}
