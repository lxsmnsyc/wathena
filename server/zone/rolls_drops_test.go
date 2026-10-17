package zone

import (
	"math"
	"testing"

	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/shared/gen"
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

// TestDropRollsByBand: on a map of the atlas, a drop's random rolls get
// one more draw for each full Mods.DropRollLevels levels of the band
// past level 1, and keep the best (decision 0046). Values stay in the
// roll's range. A band from level 1, a map outside the atlas, and a
// setting of 0 keep one draw.
func TestDropRollsByBand(t *testing.T) {
	z, _, _ := rollZone(t, "return function() end")
	d := z.Items.Get(rollHat)
	d.Static = nil
	d.Rolls = map[string]*item.RollDef{"even": {Min: 1, Max: 100, Default: 50, Distribution: item.Uniform}}
	mean := func() float64 {
		sum := 0
		for range 20_000 {
			v, _ := z.dropCopy(rollHat, "", nil).extra.Rolls.Get("even")
			if v < 1 || v > 100 {
				t.Fatalf("%d outside 1..100", v)
			}
			sum += v
		}
		return float64(sum) / 20_000
	}
	atlas := func(lo int) *layout.Atlas {
		return layout.NewAtlas(&layout.Layout{Maps: []layout.Map{{ID: z.Map, Kind: gen.KindField, Levels: [2]int{lo, lo + 9}}}, Home: z.Map}, layout.AtlasOptions{})
	}
	z.Mods.DropRollLevels = 30
	for _, c := range []struct {
		lo     int
		want   float64
		levels int
	}{
		// The mean of the best of k uniform draws of 1 to 100 is about
		// 100k/(k+1).
		{1, 50.5, 30}, {31, 67, 30}, {61, 75.5, 30}, {91, 80.5, 30}, {91, 50.5, 0},
	} {
		z.Mods.Atlas, z.Mods.DropRollLevels = atlas(c.lo), c.levels
		if got := mean(); math.Abs(got-c.want) > 1.5 {
			t.Errorf("band from %d, every %d levels: mean %.1f, want about %.1f", c.lo, c.levels, got, c.want)
		}
	}
	z.Mods.Atlas, z.Mods.DropRollLevels = nil, 30
	if got := mean(); math.Abs(got-50.5) > 1.5 {
		t.Errorf("no atlas: mean %.1f", got)
	}
}
