package status

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// TestStatusPointCost checks PC_STATUS_POINT_COST in both modes at the
// edges of its steps.
func TestStatusPointCost(t *testing.T) {
	for _, c := range []struct {
		low, pre, re int
	}{
		{1, 2, 2}, {9, 2, 2}, {10, 2, 2}, {11, 3, 3}, {20, 3, 3}, {21, 4, 4},
		{90, 10, 10}, {91, 11, 11}, {98, 11, 11}, {99, 11, 11},
		{100, 11, 16}, {101, 12, 16}, {104, 12, 16}, {105, 12, 20}, {110, 12, 24}, {129, 14, 36},
	} {
		if got := statusPointCost(false, c.low); got != c.pre {
			t.Errorf("pre-renewal cost at %d: %d, want %d", c.low, got, c.pre)
		}
		if got := statusPointCost(true, c.low); got != c.re {
			t.Errorf("renewal cost at %d: %d, want %d", c.low, got, c.re)
		}
	}
}

// TestMaxParameter checks the job limits from conf/battle/player.conf.
func TestMaxParameter(t *testing.T) {
	for _, mode := range reference.Modes {
		tb := load(t, mode)
		for _, c := range []struct {
			job, want int
		}{
			{0, 99},     // Novice
			{4008, 99},  // Lord Knight
			{4023, 80},  // Baby Novice
			{4054, 130}, // Rune Knight
			{4060, 130}, // Rune Knight (trans)
			{4096, 117}, // Baby Rune Knight
			{4215, 130}, // Rebellion
			{4211, 130}, // Kagerou
			{4218, 130}, // Summoner
			{4252, 130}, // Dragon Knight
			{4307, 130}, // Hyper Novice
			{4306, 130}, // Night Watch
			{4001, 99},  // High Novice
			{4190, 130}, // Expanded Super Novice
			{4046, 99},  // Taekwon
			{4239, 130}, // Star Emperor
			{4302, 130}, // Sky Emperor
			{4308, 130}, // Spirit Handler
			{4222, 80},  // Baby Ninja
			{4241, 117}, // Baby Star Emperor
			{4220, 80},  // Baby Summoner
		} {
			if _, ok := tb.jobs[c.job]; !ok && mode == reference.PreRenewal {
				// Pre-renewal has no third jobs and later.
				continue
			}
			got, err := tb.MaxParameter(&Character{Job: c.job, Male: true})
			if err != nil {
				t.Errorf("%s job %d: %v", mode, c.job, err)
				continue
			}
			if got != c.want {
				t.Errorf("%s job %d: max %d, want %d", mode, c.job, got, c.want)
			}
		}
	}
}

// TestNeedStatusPoint checks pc_need_status_point.
func TestNeedStatusPoint(t *testing.T) {
	re, pre := load(t, reference.Renewal), load(t, reference.PreRenewal)
	c := Character{Job: 0, Male: true, Stats: Stats{Str: 1, Agi: 99, Vit: 50, Int: 1, Dex: 1, Luk: 1}}
	for _, tc := range []struct {
		tb        *Tables
		st        Stat
		val, want int
	}{
		// Raising a stat from 1 to 99 costs 628 in both modes.
		{re, Str, 98, 628},
		{pre, Str, 98, 628},
		{re, Str, 1, 2},
		{re, Vit, 1, 6},
		// The cost of a stat at its maximum is 0.
		{re, Agi, 1, 0},
		{pre, Agi, 1, 0},
		// A negative value is what the last raises cost.
		{re, Vit, -1, 6},
		{re, Vit, -10, 59},
		{re, Str, 0, 0},
	} {
		if got := tc.tb.NeedStatusPoint(&c, tc.st, tc.val); got != tc.want {
			t.Errorf("renewal=%v stat %d by %d: %d, want %d", tc.tb.renewal, tc.st, tc.val, got, tc.want)
		}
	}
	// Above 99, renewal costs more.
	third := Character{Job: 4054, Male: true, Stats: Stats{Str: 100}}
	if got := re.NeedStatusPoint(&third, Str, 5); got != 80 {
		t.Errorf("renewal 100 to 105: %d, want 80", got)
	}
}

// TestStatusUp checks pc_statusup: the amount, the cost, the maximum, and
// the fail cases.
func TestStatusUp(t *testing.T) {
	for _, mode := range reference.Modes {
		tb := load(t, mode)
		all1 := Stats{1, 1, 1, 1, 1, 1}

		// A new character's 48 points raise STR from 1 to 20: 10 raises
		// at 2 points and 9 at 3, with 1 point left.
		c := Character{Job: 0, Male: true, Stats: all1}
		points := 48
		if !tb.StatusUp(&c, &points, Str, 255) || c.Stats.Str != 20 || points != 1 {
			t.Errorf("%s: STR %d with %d points, want 20 with 1", mode, c.Stats.Str, points)
		}
		// 1 point pays for nothing.
		if tb.StatusUp(&c, &points, Agi, 1) || c.Stats.Agi != 1 || points != 1 {
			t.Errorf("%s: raised AGI with 1 point", mode)
		}

		// One raise at a time costs the same.
		c = Character{Job: 0, Male: true, Stats: all1}
		points = 48
		for tb.StatusUp(&c, &points, Str, 1) {
		}
		if c.Stats.Str != 20 || points != 1 {
			t.Errorf("%s: one at a time, STR %d with %d points", mode, c.Stats.Str, points)
		}

		// A stat stops at the job's maximum, keeping the points left.
		c = Character{Job: 0, Male: true, Stats: Stats{Str: 97, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}}
		points = 100
		if !tb.StatusUp(&c, &points, Str, 10) || c.Stats.Str != 99 || points != 78 {
			t.Errorf("%s: STR %d with %d points, want 99 with 78", mode, c.Stats.Str, points)
		}
		if tb.StatusUp(&c, &points, Str, 1) || c.Stats.Str != 99 || points != 78 {
			t.Errorf("%s: raised STR past 99", mode)
		}
		baby := Character{Job: 4023, Male: true, Stats: Stats{Str: 80, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}}
		if tb.StatusUp(&baby, &points, Str, 1) {
			t.Errorf("%s: raised a baby's STR past 80", mode)
		}

		// Bad requests change nothing.
		c = Character{Job: 0, Male: true, Stats: all1}
		points = 48
		for _, bad := range []struct {
			st  Stat
			inc int
		}{{Str, 0}, {Str, -1}, {Stat(6), 1}, {Stat(-1), 1}} {
			if tb.StatusUp(&c, &points, bad.st, bad.inc) || points != 48 || c.Stats != all1 {
				t.Errorf("%s: stat %d by %d changed something", mode, bad.st, bad.inc)
			}
		}
		unknown := Character{Job: 99999, Stats: all1}
		if tb.StatusUp(&unknown, &points, Str, 1) || points != 48 {
			t.Errorf("%s: raised a stat of an unknown job", mode)
		}
	}
}
