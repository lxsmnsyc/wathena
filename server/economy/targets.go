package economy

import (
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/economy/core"
)

// Target is a level band's target rates per hour (strategy section
// 8.13): what the model player of the band's middle level gets on
// rAthena's classic fields of the band, as the median of those fields.
type Target struct {
	// Levels is the band, and Level the model player's level.
	Levels [2]int `yaml:"levels"`
	Level  int    `yaml:"level"`
	// Fields is how many classic fields the median is of.
	Fields   int   `yaml:"fields"`
	BaseExp  int64 `yaml:"base_exp"`
	JobExp   int64 `yaml:"job_exp"`
	Zeny     int64 `yaml:"zeny"`
	Supplies int64 `yaml:"supplies"`
}

// Targets are the bands' targets, from the lowest band.
type Targets []Target

// LoadTargets reads core's targets.
func LoadTargets() (Targets, error) {
	b, err := core.Files.ReadFile(core.TargetsFile)
	if err != nil {
		return nil, err
	}
	var f struct {
		Targets Targets `yaml:"targets"`
	}
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", core.TargetsFile, err)
	}
	if len(f.Targets) == 0 {
		return nil, fmt.Errorf("%s: no targets", core.TargetsFile)
	}
	for i, t := range f.Targets {
		if i > 0 && t.Level <= f.Targets[i-1].Level {
			return nil, fmt.Errorf("%s: the targets must rise by level", core.TargetsFile)
		}
	}
	return f.Targets, nil
}

// Bands are the bands of the targets: 10 levels each from level 1, up to
// 99, as the layout's coverage check counts them (decision 0042).
func Bands() [][2]int {
	var out [][2]int
	for lo := 1; lo <= 91; lo += 10 {
		out = append(out, [2]int{lo, min(lo+9, 99)})
	}
	return out
}

// Middle is a band's model level: the middle of the band, rounded down.
func Middle(band [2]int) int { return (band[0] + band[1]) / 2 }

// At returns the target of a model level: the line between the two
// nearest targets by level, or the nearest one beyond the ends.
func (ts Targets) At(level int) Target {
	if level <= ts[0].Level {
		return withLevel(ts[0], level)
	}
	last := ts[len(ts)-1]
	if level >= last.Level {
		return withLevel(last, level)
	}
	i := 1
	for ts[i].Level < level {
		i++
	}
	a, b := ts[i-1], ts[i]
	span, at := int64(b.Level-a.Level), int64(level-a.Level)
	lerp := func(x, y int64) int64 { return x + (y-x)*at/span }
	return Target{Levels: [2]int{a.Levels[0], b.Levels[1]}, Level: level, Fields: min(a.Fields, b.Fields),
		BaseExp: lerp(a.BaseExp, b.BaseExp), JobExp: lerp(a.JobExp, b.JobExp),
		Zeny: lerp(a.Zeny, b.Zeny), Supplies: lerp(a.Supplies, b.Supplies)}
}

func withLevel(t Target, level int) Target {
	t.Level = level
	return t
}

// Within reports whether a value is within pct percent of a target.
func Within(got, want int64, pct int) bool {
	if want <= 0 {
		return got <= 0
	}
	d := got - want
	if d < 0 {
		d = -d
	}
	return d*100 <= want*int64(pct)
}

// Ratio is got over want in percent, for reports.
func Ratio(got, want int64) int64 {
	if want <= 0 {
		return 0
	}
	return got * 100 / want
}

// Check returns how a field's rates miss a target by more than pct
// percent: base exp and zeny, the rates of the phase 7 exit criteria.
func Check(r Rates, t Target, pct int) []string {
	var out []string
	if !Within(r.BaseExp, t.BaseExp, pct) {
		out = append(out, fmt.Sprintf("base exp %d/h is %d%% of the target %d/h", r.BaseExp, Ratio(r.BaseExp, t.BaseExp), t.BaseExp))
	}
	if !Within(r.Zeny, t.Zeny, pct) {
		out = append(out, fmt.Sprintf("zeny %d/h is %d%% of the target %d/h", r.Zeny, Ratio(r.Zeny, t.Zeny), t.Zeny))
	}
	return out
}

// ClassicField is one of rAthena's field maps with its spawn lines,
// from the spawn extract.
type ClassicField struct {
	Map      string
	Walkable int
	Lines    []ClassicLine
}

// ClassicLine is one spawn line of a classic field.
type ClassicLine struct {
	Mob       uint32
	Count     int
	RespawnMs int
	Boss      bool
}

// ClassicFields reads the field maps of rAthena's spawn files of a mode:
// the maps whose name holds _fild, from spawns.json.
func ClassicFields(mode reference.Mode) ([]ClassicField, error) {
	var d struct {
		Maps []struct {
			Map      string `json:"map"`
			Walkable int    `json:"walkable_cells"`
			Spawns   []struct {
				Mob       uint32 `json:"mob"`
				Amount    int    `json:"amount"`
				RespawnMs int    `json:"respawn_ms"`
				Boss      bool   `json:"boss"`
			} `json:"spawns"`
		} `json:"maps"`
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/spawns.json", &d); err != nil {
		return nil, err
	}
	var out []ClassicField
	for _, m := range d.Maps {
		if !strings.Contains(m.Map, "_fild") || m.Walkable == 0 {
			continue
		}
		f := ClassicField{Map: m.Map, Walkable: m.Walkable}
		for _, s := range m.Spawns {
			f.Lines = append(f.Lines, ClassicLine{Mob: s.Mob, Count: s.Amount, RespawnMs: s.RespawnMs, Boss: s.Boss})
		}
		out = append(out, f)
	}
	slices.SortFunc(out, func(a, b ClassicField) int { return strings.Compare(a.Map, b.Map) })
	return out, nil
}

// HuntLevel is the level of a field's hunt: the median level of the
// monsters of its lines that respawn within a minute, by count, without
// bosses. level finds a monster's level, and false leaves it out. It
// returns 0 for a field without such monsters.
func HuntLevel(lines []ClassicLine, level func(mob uint32) (int, bool)) int {
	var levels []int
	for _, l := range lines {
		lv, ok := level(l.Mob)
		if l.Boss || l.RespawnMs > 60_000 || !ok {
			continue
		}
		for range l.Count {
			levels = append(levels, lv)
		}
	}
	if len(levels) == 0 {
		return 0
	}
	slices.Sort(levels)
	return levels[(len(levels)-1)/2]
}

// Derived is one classic field's result, for reports.
type Derived struct {
	Map   string
	Level int
	Band  [2]int
	Rates Rates
}

// DeriveTargets computes each band's target from the classic fields
// (strategy section 8.13, decision 0046): each field whose hunt level
// falls in a band is simulated for the band's model player, and the
// target is the median of each rate. name finds a monster's name and
// level by ID; a monster it does not know is left out.
func (w *World) DeriveTargets(fields []ClassicField, name func(mob uint32) (string, int, bool)) (Targets, []Derived, error) {
	var out Targets
	var all []Derived
	for _, band := range Bands() {
		level := Middle(band)
		var list []Rates
		for _, f := range fields {
			hl := HuntLevel(f.Lines, func(id uint32) (int, bool) {
				_, lv, ok := name(id)
				return lv, ok
			})
			if hl < band[0] || hl > band[1] {
				continue
			}
			var hunt Field
			hunt.Walkable = f.Walkable
			for _, l := range f.Lines {
				n, _, ok := name(l.Mob)
				if !ok || l.Boss {
					continue
				}
				hunt.Lines = append(hunt.Lines, Line{Monster: n, Count: l.Count, RespawnMs: l.RespawnMs})
			}
			r, err := w.Simulate(level, hunt)
			if err != nil {
				return nil, nil, err
			}
			list = append(list, r)
			all = append(all, Derived{Map: f.Map, Level: hl, Band: band, Rates: r})
		}
		if len(list) == 0 {
			return nil, nil, fmt.Errorf("economy: no classic field in the band %v", band)
		}
		median := func(get func(Rates) int64) int64 {
			v := make([]int64, len(list))
			for i, r := range list {
				v[i] = get(r)
			}
			slices.Sort(v)
			if len(v)%2 == 1 {
				return v[len(v)/2]
			}
			return (v[len(v)/2-1] + v[len(v)/2]) / 2
		}
		out = append(out, Target{Levels: band, Level: level, Fields: len(list),
			BaseExp:  median(func(r Rates) int64 { return r.BaseExp }),
			JobExp:   median(func(r Rates) int64 { return r.JobExp }),
			Zeny:     median(func(r Rates) int64 { return r.Zeny }),
			Supplies: median(func(r Rates) int64 { return r.Supplies })})
	}
	return out, all, nil
}

// TargetsFile writes targets as core's targets file.
func TargetsFile(ts Targets) []byte {
	var b strings.Builder
	b.WriteString(`# The target rates per hour of each level band (strategy section 8.13,
# decision 0046). Do not edit: the economy package's TestTargets
# derives them from rAthena's renewal classic fields with the simulator,
# and go test -run TestTargets ./server/economy -update writes them.
#
# Each band's target is what the model player of its middle level gets
# on the classic fields whose hunt level falls in the band, as the
# median of those fields. fields is how many there are.
targets:
`)
	for _, t := range ts {
		fmt.Fprintf(&b, "  - { levels: [%d, %d], level: %d, fields: %d, base_exp: %d, job_exp: %d, zeny: %d, supplies: %d }\n",
			t.Levels[0], t.Levels[1], t.Level, t.Fields, t.BaseExp, t.JobExp, t.Zeny, t.Supplies)
	}
	return []byte(b.String())
}
