package status

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// loadRefine loads the tables of a mode with refine_db.json.
func loadRefine(t *testing.T, mode reference.Mode) *Tables {
	t.Helper()
	tb := load(t, mode)
	var rd RefineData
	if err := reference.LoadFile("extracts/"+string(mode)+"/refine_db.json", &rd); err != nil {
		t.Fatal(err)
	}
	if err := tb.SetRefine(&rd); err != nil {
		t.Fatal(err)
	}
	return tb
}

// TestRefineCost checks RefineCost against refine.yml. The cost of going
// from +r to +r+1 is the Normal chance of the yml's refine Level r+1.
func TestRefineCost(t *testing.T) {
	cases := []struct {
		mode          reference.Mode
		weapon        bool
		level, refine int
		want          RefineCost
		ok            bool
	}{
		// Renewal weapon level 1, Level 1.
		{reference.Renewal, true, 1, 0, RefineCost{10000, 50, "Phracon", 0}, true},
		// Renewal weapon level 1, Level 10.
		{reference.Renewal, true, 1, 9, RefineCost{1900, 50, "Phracon", 10000}, true},
		// Renewal armor level 1, Level 11 uses Carnium.
		{reference.Renewal, false, 1, 10, RefineCost{800, 100000, "Carnium", 2000}, true},
		// Renewal weapon level 4, Level 20 is the last refine.
		{reference.Renewal, true, 4, 19, RefineCost{500, 100000, "Bradium", 2000}, true},
		// MAX_REFINE is 20 in renewal.
		{reference.Renewal, true, 1, 20, RefineCost{}, false},
		// Pre-renewal armor from +4 to +5 is Level 5.
		{reference.PreRenewal, false, 1, 4, RefineCost{6000, 2000, "Elunium", 10000}, true},
		// Pre-renewal weapon level 3, Level 10.
		{reference.PreRenewal, true, 3, 9, RefineCost{2000, 5000, "Oridecon", 10000}, true},
		// MAX_REFINE is 10 in pre-renewal.
		{reference.PreRenewal, false, 1, 10, RefineCost{}, false},
		// Pre-renewal has no armor level 2.
		{reference.PreRenewal, false, 2, 0, RefineCost{}, false},
		// A negative refine has no cost.
		{reference.PreRenewal, true, 1, -1, RefineCost{}, false},
	}
	tables := map[reference.Mode]*Tables{}
	for _, c := range cases {
		tb := tables[c.mode]
		if tb == nil {
			tb = loadRefine(t, c.mode)
			tables[c.mode] = tb
		}
		got, ok := tb.RefineCost(c.weapon, c.level, c.refine)
		if ok != c.ok || got != c.want {
			t.Errorf("%s weapon=%v level %d +%d: got %+v %v, want %+v %v", c.mode, c.weapon, c.level, c.refine, got, ok, c.want, c.ok)
		}
	}
}

// TestRefineCostWithoutTable checks that tables without refine_db have no
// costs.
func TestRefineCostWithoutTable(t *testing.T) {
	if _, ok := load(t, reference.Renewal).RefineCost(true, 1, 0); ok {
		t.Error("RefineCost without a refine table returned a cost")
	}
}
