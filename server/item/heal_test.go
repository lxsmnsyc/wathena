package item

import (
	"slices"
	"testing"
)

// There is no recording of item heals, so these values come from reading
// rAthena's pc_itemheal, pc_percentheal and status_percent_change.

// fixedRand returns the largest value and records each draw's range.
type fixedRand struct{ draws []int }

func (r *fixedRand) IntN(n int) int {
	r.draws = append(r.draws, n)
	return n - 1
}

func TestRoll(t *testing.T) {
	r := &fixedRand{}
	h := Heal{Command: "itemheal", HP: [2]int{325, 405}, SP: [2]int{40, 60}}
	hp, sp := h.Roll(r)
	// HP draws first, from 81 values, then SP from 21.
	if hp != 405 || sp != 60 || !slices.Equal(r.draws, []int{81, 21}) {
		t.Errorf("%d, %d, draws %v", hp, sp, r.draws)
	}
	// Fixed amounts draw nothing.
	r = &fixedRand{}
	h = Heal{Command: "percentheal", HP: [2]int{100, 100}, SP: [2]int{0, 0}}
	if hp, sp := h.Roll(r); hp != 100 || sp != 0 || len(r.draws) != 0 {
		t.Errorf("%d, %d, draws %v", hp, sp, r.draws)
	}
}

func TestItemHeal(t *testing.T) {
	for _, c := range []struct {
		name              string
		hp, sp, vit, int_ int
		skills            map[int]int
		wantHP, wantSP    int
	}{
		// 100% + 2% per VIT: 50 × 160 / 100.
		{"vit", 50, 0, 30, 1, nil, 80, 0},
		// VIT 0 and no skills: the heal is unchanged.
		{"plain", 50, 30, 0, 0, nil, 50, 30},
		// Increase HP Recovery 10 adds 100%, Learning Potion 5 adds 25%
		// to both.
		{"skills", 100, 40, 1, 99, map[int]int{SkillIncHPRecovery: 10, SkillLearningPotion: 5}, 100 * 227 / 100, 40 * 323 / 100},
		// Increase SP Recovery only helps SP.
		{"sp skill", 10, 10, 0, 0, map[int]int{SkillIncSPRecovery: 3}, 10, 13},
		// A heal of 0 stays 0.
		{"zero", 0, 0, 99, 99, nil, 0, 0},
	} {
		hp, sp := ItemHeal(c.hp, c.sp, c.vit, c.int_, c.skills)
		if hp != c.wantHP || sp != c.wantSP {
			t.Errorf("%s: %d, %d; want %d, %d", c.name, hp, sp, c.wantHP, c.wantSP)
		}
	}
}

func TestPercentHeal(t *testing.T) {
	for _, c := range []struct {
		hpRate, spRate, maxHP, maxSP int
		wantHP, wantSP               int
	}{
		{100, 100, 1234, 321, 1234, 321},
		{150, 0, 1234, 321, 1234, 0},
		{5, 5, 1000, 30, 50, 1},
		// A heal below 1 heals 1.
		{1, 0, 50, 10, 1, 0},
		// apply_rate divides first above 100000.
		{5, 0, 150001, 0, 150001 / 100 * 5, 0},
	} {
		hp, sp := PercentHeal(c.hpRate, c.spRate, c.maxHP, c.maxSP)
		if hp != c.wantHP || sp != c.wantSP {
			t.Errorf("%+v: %d, %d", c, hp, sp)
		}
	}
}

func TestCap(t *testing.T) {
	for _, c := range []struct{ heal, cur, maxValue, want int64 }{
		{50, 10, 100, 50}, {50, 80, 100, 20}, {50, 100, 100, 0}, {-5, 10, 100, 0},
	} {
		if got := Cap(c.heal, c.cur, c.maxValue); got != c.want {
			t.Errorf("%+v: %d", c, got)
		}
	}
}
