package item

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// seeded is a fixed random stream for distribution tests.
type seeded struct{ r *rand.Rand }

func (s seeded) IntN(n int) int { return s.r.IntN(n) }

func newSeeded(seed uint64) seeded {
	return seeded{rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))}
}

// chiSquare checks samples against a roll's probabilities. Values whose
// expected count is below 5 join one bucket. It fails above the mean
// plus six standard deviations of the statistic, so a fixed seed never
// fails on a right distribution.
func chiSquare(t *testing.T, d *RollDef, counts map[int]int, n int) {
	t.Helper()
	lo, hi := d.Bounds()
	stat, df := 0.0, -1
	var restObs, restExp float64
	for v := lo; v <= hi; v++ {
		exp := d.Probability(v) * float64(n)
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
	if df < 1 {
		return
	}
	if limit := float64(df) + 6*math.Sqrt(2*float64(df)); stat > limit {
		t.Errorf("%s %d..%d: chi-square %.1f over %d degrees of freedom, above %.1f", d.Distribution, lo, hi, stat, df, limit)
	}
}

// TestRollDistributions draws 100,000 values of each distribution: every
// value stays in its range and the counts follow the distribution
// (strategy section 5.4, phase 4 exit criteria).
func TestRollDistributions(t *testing.T) {
	for i, d := range []*RollDef{
		{Min: 15, Max: 35, Default: 25, Distribution: Uniform},
		{Min: 15, Max: 35, Default: 25, Distribution: Triangular},
		{Min: 15, Max: 35, Default: 25},
		{Min: 1, Max: 10, Default: 1, Distribution: Triangular},
		{Min: 1, Max: 10, Default: 10, Distribution: Triangular},
		{Min: 7, Max: 7, Default: 7, Distribution: Triangular},
		{Min: -50, Max: 50, Default: 0, Distribution: Triangular},
		{Default: 5, Distribution: Weighted, Weights: []Weight{{1, 5, 70}, {6, 9, 25}, {10, 10, 5}}},
		{Min: 0, Max: 20, Default: 0, Distribution: Weighted, Weights: []Weight{{0, 0, 1}, {20, 20, 1}}},
	} {
		if err := d.Check(); err != nil {
			t.Fatalf("roll %d: %v", i, err)
		}
		const n = 100_000
		r := newSeeded(uint64(i) + 1)
		lo, hi := d.Bounds()
		counts := map[int]int{}
		for range n {
			v := d.Random(r)
			if v < lo || v > hi {
				t.Fatalf("roll %d: %d outside %d..%d", i, v, lo, hi)
			}
			counts[v]++
		}
		chiSquare(t, d, counts, n)
		sum := 0.0
		for v := lo; v <= hi; v++ {
			sum += d.Probability(v)
		}
		if math.Abs(sum-1) > 1e-9 {
			t.Errorf("roll %d: probabilities sum to %f", i, sum)
		}
	}
}

func TestTriangularShape(t *testing.T) {
	d := &RollDef{Min: 15, Max: 35, Default: 25, Distribution: Triangular}
	// The peak is the default, and the ends are the rarest.
	if !(d.Probability(25) > d.Probability(20) && d.Probability(20) > d.Probability(15)) ||
		d.Probability(15) != d.Probability(35) || d.Probability(14) != 0 {
		t.Errorf("probabilities %f %f %f %f", d.Probability(15), d.Probability(20), d.Probability(25), d.Probability(35))
	}
}

func TestRollCheck(t *testing.T) {
	for _, c := range []struct {
		d    RollDef
		want string
	}{
		{RollDef{Min: 5, Max: 1, Default: 3}, "above max"},
		{RollDef{Min: 1, Max: 5, Default: 9}, "outside"},
		{RollDef{Min: 1, Max: 5, Default: 3, Distribution: "normal"}, "not uniform"},
		{RollDef{Default: 1, Distribution: Weighted}, "needs weights"},
		{RollDef{Min: 1, Max: 5, Default: 1, Distribution: Weighted, Weights: []Weight{{1, 9, 1}}}, "outside min"},
		{RollDef{Default: 1, Distribution: Weighted, Weights: []Weight{{1, 1, 0}}}, "weight is"},
		{RollDef{Min: 1, Max: 5, Default: 1, Weights: []Weight{{1, 1, 1}}}, "only a weighted"},
		{RollDef{Min: 0, Max: 1 << 40, Default: 0}, "values are"},
		{RollDef{Min: 1, Max: 5, Default: 3, DisplayName: strings.Repeat("x", 65)}, "display_name is 1 to 64 bytes"},
	} {
		err := c.d.Check()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: %v, want %q", c.d, err, c.want)
		}
	}
}

func TestRollDisplayName(t *testing.T) {
	var d RollDef
	if err := json.Unmarshal([]byte(`{"min": 1, "max": 5, "default": 3, "display_name": "Clover luck"}`), &d); err != nil {
		t.Fatal(err)
	}
	if err := d.Check(); err != nil || d.DisplayName != "Clover luck" {
		t.Errorf("%+v: %v", d, err)
	}
	d.DisplayName = strings.Repeat("x", 64)
	if err := d.Check(); err != nil {
		t.Errorf("64 bytes: %v", err)
	}
}

func TestWeightJSON(t *testing.T) {
	var d RollDef
	if err := json.Unmarshal([]byte(`{"default": 3, "distribution": "weighted", "weights": [{"value": 3, "weight": 9}, {"value": [4, 6], "weight": 1}]}`), &d); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(d.Weights, []Weight{{3, 3, 9}, {4, 6, 1}}) {
		t.Errorf("weights %v", d.Weights)
	}
	b, _ := json.Marshal(d.Weights)
	if string(b) != `[{"value":3,"weight":9},{"value":[4,6],"weight":1}]` {
		t.Errorf("json %s", b)
	}
	for _, bad := range []string{`{"value": 3}`, `{"value": [1], "weight": 1}`, `{"value": 1, "weight": 1, "tier": 2}`} {
		var w Weight
		if err := json.Unmarshal([]byte(bad), &w); err == nil {
			t.Errorf("%s: no error", bad)
		}
	}
}

func TestRolls(t *testing.T) {
	r := RollsOf(map[string]int{"luck": 2, "boss_damage": 31})
	if r.Key() != "boss_damage=31,luck=2" {
		t.Errorf("key %q", r.Key())
	}
	if v, ok := r.Get("luck"); !ok || v != 2 {
		t.Errorf("luck %d %v", v, ok)
	}
	w := r.With("crit", 4).With("luck", 5)
	if w.Key() != "boss_damage=31,crit=4,luck=5" || r.Key() != "boss_damage=31,luck=2" {
		t.Errorf("with %q, original %q", w.Key(), r.Key())
	}
	b, _ := json.Marshal(r)
	var back Rolls
	if err := json.Unmarshal(b, &back); err != nil || !slices.Equal(back, r) {
		t.Errorf("round trip %s: %v %v", b, back, err)
	}
}

func TestFixRolls(t *testing.T) {
	d := &Def{Rolls: map[string]*RollDef{
		"boss_damage": {Min: 15, Max: 35, Default: 25},
		"luck":        {Min: 1, Max: 5, Default: 3},
	}}
	got, notes := d.FixRolls(RollsOf(map[string]int{"boss_damage": 99, "gone": 1}))
	if got.Key() != "boss_damage=35,luck=3" || len(notes) != 3 {
		t.Errorf("fixed %q, notes %v", got.Key(), notes)
	}
	// A copy saved before the item had rolls gets the defaults quietly.
	if got, notes := d.FixRolls(nil); got.Key() != "boss_damage=25,luck=3" || len(notes) != 0 {
		t.Errorf("fixed %q, notes %v", got.Key(), notes)
	}
}

func TestStackingByRolls(t *testing.T) {
	card := &Def{ID: 4140, Type: "Card", Stackable: true, Rolls: map[string]*RollDef{"boss_damage": {Min: 15, Max: 35, Default: 25}}}
	hidden := &Def{ID: 4141, Type: "Card", Stackable: true, Rolls: map[string]*RollDef{"luck": {Min: 1, Max: 5, Default: 3, Hidden: true}}}
	inv := NewInventory()
	add := func(d *Def, x Extra) int {
		slot, r := inv.AddExtra(d, 1, 1<<30, x)
		if r != AddSuccess {
			t.Fatalf("add: %d", r)
		}
		return slot
	}
	r31 := Extra{Rolls: RollsOf(map[string]int{"boss_damage": 31})}
	r29 := Extra{Rolls: RollsOf(map[string]int{"boss_damage": 29})}
	// Two cards with 31% stack. A card with 29% takes its own slot.
	if a, b, c := add(card, r31), add(card, r31), add(card, r29); a != 0 || b != 0 || c != 1 {
		t.Errorf("slots %d %d %d", a, b, c)
	}
	if inv.Slots[0].Amount != 2 || inv.Slots[1].Amount != 1 {
		t.Errorf("amounts %d %d", inv.Slots[0].Amount, inv.Slots[1].Amount)
	}
	// Unidentified copies with hidden rolls never stack, even when equal.
	h := Extra{Rolls: RollsOf(map[string]int{"luck": 3}), Unidentified: true}
	if a, b := add(hidden, h), add(hidden, h); a == b {
		t.Errorf("unidentified hidden copies stacked in slot %d", a)
	}
	// Identified ones do.
	h.Unidentified = false
	if a, b := add(hidden, h), add(hidden, h); a != b {
		t.Errorf("identified copies in slots %d and %d", a, b)
	}
	// Random options take part in the key.
	o1 := Extra{Rolls: r31.Rolls, Options: []Option{{Name: "max_hp", Value: 100}}}
	if a := add(card, o1); a == 0 {
		t.Error("a card with options stacked with one without")
	}
}

func TestInsertCardKeepsRolls(t *testing.T) {
	tb := New(&Data{Mode: "renewal", Items: []Def{
		{ID: 1201, Type: "Weapon", Slots: 2, Location: Locations(EqpHandR)},
		{ID: 4140, Type: "Card", Stackable: true, Location: Locations(EqpHandR), Rolls: map[string]*RollDef{"boss_damage": {Min: 15, Max: 35, Default: 25}}},
	}})
	inv := NewInventory()
	w, _ := inv.Add(tb.Get(1201), 1, 1<<30)
	c, _ := inv.AddExtra(tb.Get(4140), 1, 1<<30, Extra{Rolls: RollsOf(map[string]int{"boss_damage": 31})})
	if !inv.InsertCard(tb, c, w, 4) {
		t.Fatal("insert failed")
	}
	if s := inv.Slots[w]; len(s.Cards) != 1 || s.CardRoll(0).Key() != "boss_damage=31" {
		t.Errorf("weapon %+v", s)
	}
}
