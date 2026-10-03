package gamedata

import (
	"math"
	"math/rand/v2"
	"sort"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/tools/convert"
)

// randomOptionRolls is random_option_rolls.json: rAthena's
// s_random_opt_group::apply on each group with rnd_force_min, and
// statistics of 100,000 seeded calls.
type randomOptionRolls struct {
	reference.Header
	Groups []struct {
		ID        int           `json:"id"`
		Name      string        `json:"name"`
		ForcedMin []recordedOpt `json:"forced_min"`
		Rolls     struct {
			Count          int   `json:"count"`
			OptionsPerItem []int `json:"options_per_item"`
			Slots          [][]struct {
				ID, Count, Min, Max int
				Sum                 int64
			} `json:"slots"`
		} `json:"rolls"`
	} `json:"groups"`
	Drops []struct {
		Monster   uint32        `json:"monster"`
		MVP       bool          `json:"mvp"`
		Index     int           `json:"index"`
		Item      uint32        `json:"item"`
		GroupID   int           `json:"group_id"`
		ForcedMin []recordedOpt `json:"forced_min"`
	} `json:"drops"`
}

type recordedOpt struct {
	ID    int `json:"id"`
	Value int `json:"value"`
	Param int `json:"param"`
}

// lowest is rnd_force_min: every draw is its lowest value, so every
// chance passes.
type lowest struct{}

func (lowest) IntN(int) int { return 0 }

type pcg struct{ r *rand.Rand }

func (p pcg) IntN(n int) int { return p.r.IntN(n) }

// TestRandomOptionParity checks core's random option groups against
// rAthena's in both modes:
//   - With every draw at its lowest, each group and each monster drop
//     with a group gives the options rAthena gave.
//   - Over 100,000 seeded calls per group, the number of options per
//     copy, each option's share at each index, and the values' range and
//     mean match rAthena's statistics within six standard errors.
func TestRandomOptionParity(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			core, err := convert.LoadCore(mode)
			if err != nil {
				t.Fatal(err)
			}
			set, err := mod.Resolve(nil, mod.Config{Core: core.Files})
			if err != nil {
				t.Fatal(err)
			}
			opts, err := set.RandomOptions(core.Names(nil))
			if err != nil {
				t.Fatal(err)
			}
			byID := map[int]string{}
			for _, o := range core.RandomOptions.Options {
				byID[o.ID] = o.Name
			}
			var rec randomOptionRolls
			if err := reference.Load(mode, "random_option_rolls", &rec); err != nil {
				t.Fatal(err)
			}
			if len(rec.Groups) != len(opts.Groups) {
				t.Fatalf("%d groups, rAthena has %d", len(opts.Groups), len(rec.Groups))
			}
			names := func(list []recordedOpt) []item.Option {
				var out []item.Option
				for _, o := range list {
					out = append(out, item.Option{Name: byID[o.ID], Value: o.Value, Param: o.Param})
				}
				return out
			}
			same := func(a, b []item.Option) bool {
				if len(a) != len(b) {
					return false
				}
				for i := range a {
					if a[i] != b[i] {
						return false
					}
				}
				return true
			}
			cells := 0
			for gi, g := range rec.Groups {
				og := opts.Group(g.Name)
				if og == nil {
					t.Errorf("group %s is missing", g.Name)
					continue
				}
				if got, want := og.Apply(lowest{}), names(g.ForcedMin); !same(got, want) {
					t.Errorf("group %s forced: %v, want %v", g.Name, got, want)
				}
				cells += compareStats(t, og, g.Name, uint64(gi), g.Rolls.Count, g.Rolls.OptionsPerItem, g.Rolls.Slots, byID)
			}
			// Drops: core's monsters carry the groups, and each gives
			// rAthena's options.
			tables, data, err := LoadProgression(mode)
			if err != nil {
				t.Fatal(err)
			}
			monsters, err := LoadMonsters(mode, tables, data)
			if err != nil {
				t.Fatal(err)
			}
			groupName := map[int]string{}
			for _, g := range rec.Groups {
				groupName[g.ID] = g.Name
			}
			for _, d := range rec.Drops {
				m := monsters[d.Monster]
				list := m.drops
				if d.MVP {
					list = m.mvpDrops
				}
				if d.Index >= len(list) || list[d.Index].Item != d.Item || list[d.Index].RandomOptionGroup != groupName[d.GroupID] {
					t.Errorf("monster %d drop %d: not item %d with group %s", d.Monster, d.Index, d.Item, groupName[d.GroupID])
					continue
				}
				if got, want := opts.Group(groupName[d.GroupID]).Apply(lowest{}), names(d.ForcedMin); !same(got, want) {
					t.Errorf("monster %d drop %d: %v, want %v", d.Monster, d.Index, got, want)
				}
			}
			t.Logf("%s: %d groups, %d option cells, %d drops", mode, len(rec.Groups), cells, len(rec.Drops))
		})
	}
}

// compareStats rolls a group as rAthena's recording did and compares the
// statistics. It returns how many option cells it compared.
func compareStats(t *testing.T, g *item.OptionGroup, name string, seed uint64, n int, perItem []int,
	slots [][]struct {
		ID, Count, Min, Max int
		Sum                 int64
	}, byID map[int]string) int {
	t.Helper()
	type stat struct {
		count, lo, hi int
		sum           int64
	}
	r := pcg{rand.New(rand.NewPCG(seed+1, 0x5eed))}
	got := [item.MaxOptions]map[string]*stat{}
	for i := range got {
		got[i] = map[string]*stat{}
	}
	var counts [item.MaxOptions + 1]int
	for range n {
		opts := g.Apply(r)
		counts[len(opts)]++
		for i, o := range opts {
			s := got[i][o.Name]
			if s == nil {
				s = &stat{lo: math.MaxInt, hi: math.MinInt}
				got[i][o.Name] = s
			}
			s.count++
			s.sum += int64(o.Value)
			s.lo, s.hi = min(s.lo, o.Value), max(s.hi, o.Value)
		}
	}
	// share compares two counts out of n each.
	share := func(what string, a, b int) {
		p1, p2 := float64(a)/float64(n), float64(b)/float64(n)
		p := (p1 + p2) / 2
		se := math.Sqrt(p*(1-p)*2/float64(n)) + 1e-12
		if math.Abs(p1-p2) > 6*se+1e-9 {
			t.Errorf("group %s %s: %d of %d, rAthena %d", name, what, a, n, b)
		}
	}
	for k := range counts {
		want := 0
		if k < len(perItem) {
			want = perItem[k]
		}
		share("copies with "+string(rune('0'+k))+" options", counts[k], want)
	}
	cells := 0
	for i := range item.MaxOptions {
		seen := map[string]bool{}
		var rec []struct {
			ID, Count, Min, Max int
			Sum                 int64
		}
		if i < len(slots) {
			rec = slots[i]
		}
		for _, w := range rec {
			opt := byID[w.ID]
			seen[opt] = true
			cells++
			s := got[i][opt]
			if s == nil {
				s = &stat{}
			}
			share("index "+string(rune('1'+i))+" "+opt, s.count, w.Count)
			if s.count >= 1000 && w.Count >= 1000 {
				// Both reach the ends of a range they draw from often
				// enough: 20 times its size.
				if size := w.Max - w.Min + 1; s.count >= 20*size && w.Count >= 20*size && (s.lo != w.Min || s.hi != w.Max) {
					t.Errorf("group %s index %d %s: values %d to %d, rAthena %d to %d", name, i+1, opt, s.lo, s.hi, w.Min, w.Max)
				}
				// The mean, within six standard errors of a uniform draw.
				sd := float64(w.Max-w.Min) / math.Sqrt(12)
				m1, m2 := float64(s.sum)/float64(s.count), float64(w.Sum)/float64(w.Count)
				if se := sd * math.Sqrt(1/float64(s.count)+1/float64(w.Count)); math.Abs(m1-m2) > 6*se+1e-9 {
					t.Errorf("group %s index %d %s: mean %.2f, rAthena %.2f", name, i+1, opt, m1, m2)
				}
			}
		}
		var extra []string
		for opt := range got[i] {
			if !seen[opt] {
				extra = append(extra, opt)
			}
		}
		sort.Strings(extra)
		for _, opt := range extra {
			cells++
			share("index "+string(rune('1'+i))+" "+opt, got[i][opt].count, 0)
		}
	}
	return cells
}

// TestRollLabels checks how the client names rolls: a roll's
// display_name, or the words of the first static bonus that reads it
// with its unit, or the roll's name.
func TestRollLabels(t *testing.T) {
	static := func(entry ...any) *bonus.Bonus {
		b, err := bonus.FromData(entry, &bonus.Tables{})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	d := item.Def{ID: 1, AegisName: "Charm", Rolls: map[string]*item.RollDef{
		"luck":     {Min: 1, Max: 5, Default: 3, DisplayName: "Clover luck"},
		"critical": {Min: 1, Max: 5, Default: 3},
		"unused":   {Min: 1, Max: 5, Default: 3},
	}, Static: []*bonus.Bonus{
		static("luk", map[string]any{"roll": "luck"}),
		static("crit", map[string]any{"roll": "critical"}),
	}}
	data := &item.Data{Items: []item.Def{d}}
	labelRolls(data, &mod.RandomOptions{}, nil)
	rolls := data.Items[0].Rolls
	want, _ := bonus.Describe(d.Static[1], func(string, int64) string { return "" })
	for name, label := range map[string]string{"luck": "Clover luck", "critical": want, "unused": "Unused"} {
		if got := rolls[name].Label; got != label {
			t.Errorf("roll %s: label %q, want %q", name, got, label)
		}
	}
	if _, unit := bonus.Describe(d.Static[0], func(string, int64) string { return "" }); rolls["luck"].Unit != unit {
		t.Errorf("roll luck: unit %q, want the bonus's %q", rolls["luck"].Unit, unit)
	}
}
