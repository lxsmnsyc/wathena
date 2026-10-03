package zone

import (
	"math"
	"testing"

	"github.com/lxsmnsyc/wathena/server/item"
)

// TestRolledDropDistributions makes 100,000 monster drops of an item with
// a uniform, a triangular and a weighted roll through dropCopy, which
// reward and mvpReward call for every drop, from the zone's secret
// random stream with item_rolls on (strategy section 5.4, phase 4 exit
// criteria). Every value stays in its range, and the counts of each
// roll pass a chi-square test against its declared distribution. With
// item_rolls off, every drop gets the defaults.
func TestRolledDropDistributions(t *testing.T) {
	z, _, _ := rollZone(t, "return function() end")
	d := z.Items.Get(rollHat)
	d.Static = nil
	d.Rolls = map[string]*item.RollDef{
		"even":   {Min: 15, Max: 35, Default: 25, Distribution: item.Uniform},
		"peaked": {Min: 1, Max: 10, Default: 3, Distribution: item.Triangular},
		"tiered": {Default: 5, Distribution: item.Weighted, Weights: []item.Weight{{Min: 1, Max: 5, Weight: 70}, {Min: 6, Max: 9, Weight: 25}, {Min: 10, Max: 10, Weight: 5}}},
	}
	for name, r := range d.Rolls {
		if err := r.Check(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
	const n = 100_000
	counts := map[string]map[int]int{}
	for name := range d.Rolls {
		counts[name] = map[int]int{}
	}
	for range n {
		x := z.dropCopy(rollHat, "", nil).extra
		if len(x.Rolls) != len(d.Rolls) {
			t.Fatalf("rolls %v", x.Rolls)
		}
		for name, r := range d.Rolls {
			v, ok := x.Rolls.Get(name)
			lo, hi := r.Bounds()
			if !ok || v < lo || v > hi {
				t.Fatalf("%s: %d outside %d..%d", name, v, lo, hi)
			}
			counts[name][v]++
		}
	}
	for name, r := range d.Rolls {
		stat, df := dropChiSquare(r, counts[name], n)
		limit := float64(df) + 6*math.Sqrt(2*float64(df))
		t.Logf("%s (%s): chi-square %.1f over %d degrees of freedom, limit %.1f", name, r.Distribution, stat, df, limit)
		if df < 1 || stat > limit {
			t.Errorf("%s: chi-square %.1f over %d degrees of freedom, above %.1f", name, stat, df, limit)
		}
	}

	z.Mods.ItemRolls = false
	for range 1000 {
		x := z.dropCopy(rollHat, "", nil).extra
		for name, r := range d.Rolls {
			if v, _ := x.Rolls.Get(name); v != r.Default {
				t.Fatalf("%s: %d with item_rolls off, want the default %d", name, v, r.Default)
			}
		}
		if x.Unidentified {
			t.Fatal("a copy with defaults is unidentified")
		}
	}
}

// dropChiSquare is the chi-square statistic of counts against a roll's
// probabilities, with values whose expected count is below 5 in one
// bucket, and its degrees of freedom.
func dropChiSquare(r *item.RollDef, counts map[int]int, n int) (float64, int) {
	lo, hi := r.Bounds()
	stat, df := 0.0, -1
	var restObs, restExp float64
	for v := lo; v <= hi; v++ {
		exp := r.Probability(v) * float64(n)
		obs := float64(counts[v])
		if exp < 5 {
			restObs += obs
			restExp += exp
			continue
		}
		stat += (obs - exp) * (obs - exp) / exp
		df++
	}
	if restExp >= 5 {
		stat += (restObs - restExp) * (restObs - restExp) / restExp
		df++
	}
	return stat, df
}
