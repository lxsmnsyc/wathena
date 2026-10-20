package item

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

// Rolled item values (strategy section 5.4, proposal 0002 review
// decision 23): an item's data declares named rolls, and each new copy
// gets a value for each roll. rAthena's random options are extra rolled
// lines on the same copy (random.go).

// MaxRolls is the most rolls an item declares.
const MaxRolls = 8

// MaxRollDisplayName is the longest display_name of a roll, in bytes.
const MaxRollDisplayName = 64

// Distributions of a roll.
const (
	Uniform    = "uniform"
	Triangular = "triangular"
	Weighted   = "weighted"
)

// RollDef is one roll of an item's data: rolls in data/items.yml.
type RollDef struct {
	Min     int `json:"min"`
	Max     int `json:"max"`
	Default int `json:"default"`
	// Distribution is uniform, triangular or weighted. Empty is
	// triangular.
	Distribution string `json:"distribution"`
	// Weights are the tiers of a weighted roll.
	Weights []Weight `json:"weights"`
	// Hidden rolls show only once the copy is identified.
	Hidden bool `json:"hidden"`
	// DisplayName is the name players see, 1 to 64 bytes. Empty means
	// the label of the first static bonus that reads the roll.
	DisplayName string `json:"display_name,omitempty"`
	// Label and Unit are how the client shows the roll, such as
	// "Physical damage against bosses" and "%". The mod loader sets them:
	// Label is DisplayName when set, and otherwise comes from the first
	// static bonus that reads the roll, as Unit does.
	Label string `json:"-"`
	Unit  string `json:"-"`
}

// Weight is one tier of a weighted roll: a value, or a range from Min to
// Max, picked with Weight over the sum of the weights.
type Weight struct {
	Min, Max int
	Weight   int
}

// UnmarshalJSON reads { value, weight }, where value is a number or
// [min, max].
func (w *Weight) UnmarshalJSON(b []byte) error {
	var raw struct {
		Value  json.RawMessage `json:"value"`
		Weight *int            `json:"weight"`
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&raw); err != nil {
		return errors.New("a weight is { value, weight }, where value is a number or [min, max]")
	}
	if raw.Weight == nil || len(raw.Value) == 0 {
		return errors.New("a weight needs value and weight")
	}
	w.Weight = *raw.Weight
	var n int
	if err := json.Unmarshal(raw.Value, &n); err == nil {
		w.Min, w.Max = n, n
		return nil
	}
	var r []int
	if err := json.Unmarshal(raw.Value, &r); err != nil || len(r) != 2 {
		return errors.New("a weight's value is a whole number or [min, max]")
	}
	w.Min, w.Max = r[0], r[1]
	return nil
}

// MarshalJSON writes the form UnmarshalJSON reads.
func (w Weight) MarshalJSON() ([]byte, error) {
	var v any = w.Min
	if w.Min != w.Max {
		v = []int{w.Min, w.Max}
	}
	return json.Marshal(map[string]any{"value": v, "weight": w.Weight})
}

// Roll is one rolled value of a copy.
type Roll struct {
	Name  string
	Value int
}

// Rolls are a copy's values, sorted by name: the canonical form that
// stacking compares and the database stores.
type Rolls []Roll

// Get returns a roll's value.
func (r Rolls) Get(name string) (int, bool) {
	i, ok := slices.BinarySearchFunc(r, name, func(x Roll, n string) int { return strings.Compare(x.Name, n) })
	if !ok {
		return 0, false
	}
	return r[i].Value, true
}

// With returns the rolls with one value set, as a new list.
func (r Rolls) With(name string, value int) Rolls {
	out := slices.Clone(r)
	i, ok := slices.BinarySearchFunc(out, name, func(x Roll, n string) int { return strings.Compare(x.Name, n) })
	if ok {
		out[i].Value = value
		return out
	}
	return slices.Insert(out, i, Roll{name, value})
}

// Key is the canonical text of the rolls: names in order with their
// values, such as "boss_damage=31,luck=2".
func (r Rolls) Key() string {
	var b strings.Builder
	for i, x := range r {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%s=%d", x.Name, x.Value)
	}
	return b.String()
}

// MarshalJSON writes the rolls as an object, such as {"boss_damage": 31}.
func (r Rolls) MarshalJSON() ([]byte, error) {
	m := make(map[string]int, len(r))
	for _, x := range r {
		m[x.Name] = x.Value
	}
	return json.Marshal(m)
}

// UnmarshalJSON reads an object of whole numbers.
func (r *Rolls) UnmarshalJSON(b []byte) error {
	var m map[string]int
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	*r = RollsOf(m)
	return nil
}

// RollsOf makes sorted rolls from a map. An empty map is nil.
func RollsOf(m map[string]int) Rolls {
	if len(m) == 0 {
		return nil
	}
	out := make(Rolls, 0, len(m))
	for k, v := range m {
		out = append(out, Roll{k, v})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Option is one random option of a copy: the option's name in
// data/random_options.yml, its value and its param.
type Option struct {
	Name  string `json:"option"`
	Value int    `json:"value"`
	Param int    `json:"param"`
}

// Extra is what makes copies of one item differ besides refine and
// cards: rolls, random options and identification. The zero Extra is an
// identified copy with nothing rolled.
type Extra struct {
	Rolls Rolls
	// CardRolls are the rolls of each card, in the order of the slot's
	// Cards. It may be shorter than Cards: missing entries have none.
	CardRolls []Rolls
	Options   []Option
	// Unidentified copies show their hidden rolls only once identified.
	Unidentified bool
	// Bound is who the copy is bound to, as rAthena's item.bound. Bound
	// and unbound copies never stack.
	Bound Bound
	// Broken is rAthena's item.attribute of 1: skill_break_equip or a
	// script broke the item, and it cannot be equipped until it is
	// repaired.
	Broken bool
	// Expires is a rental's end, as rAthena's item.expire_time: Unix
	// seconds, or 0 for an item that is not rented. Rentals never stack.
	Expires int64
	// SignedBy is the character a script named the copy for, as
	// getnameditem's CARD0_CREATE, or 0. SignedName is that character's
	// name, which players see before the item's name.
	SignedBy   uint32
	SignedName string
}

// Equal reports whether two extras are the same.
func (e *Extra) Equal(o *Extra) bool {
	return slices.Equal(e.Rolls, o.Rolls) && slices.Equal(e.Options, o.Options) && e.Unidentified == o.Unidentified && e.Bound == o.Bound &&
		e.Broken == o.Broken && e.Expires == o.Expires && e.SignedBy == o.SignedBy && e.SignedName == o.SignedName &&
		slices.EqualFunc(trimRolls(e.CardRolls), trimRolls(o.CardRolls), func(a, b Rolls) bool { return slices.Equal(a, b) })
}

// trimRolls drops the empty entries at the end of card rolls, so a short
// list equals a padded one.
func trimRolls(r []Rolls) []Rolls {
	for len(r) > 0 && len(r[len(r)-1]) == 0 {
		r = r[:len(r)-1]
	}
	return r
}

// Clone copies an extra.
func (e Extra) Clone() Extra {
	out := Extra{Rolls: slices.Clone(e.Rolls), Options: slices.Clone(e.Options), Unidentified: e.Unidentified, Bound: e.Bound,
		Broken: e.Broken, Expires: e.Expires, SignedBy: e.SignedBy, SignedName: e.SignedName}
	for _, r := range e.CardRolls {
		out.CardRolls = append(out.CardRolls, slices.Clone(r))
	}
	return out
}

// CardRoll returns the rolls of card i.
func (e *Extra) CardRoll(i int) Rolls {
	if i < 0 || i >= len(e.CardRolls) {
		return nil
	}
	return e.CardRolls[i]
}

// HasHidden reports whether an item has a hidden roll.
func (d *Def) HasHidden() bool {
	for _, r := range d.Rolls {
		if r.Hidden {
			return true
		}
	}
	return false
}

// RollNames are an item's roll names, sorted.
func (d *Def) RollNames() []string {
	out := make([]string, 0, len(d.Rolls))
	for n := range d.Rolls {
		out = append(out, n)
	}
	slices.Sort(out)
	return out
}

// Bounds are the lowest and highest value of a roll. A weighted roll
// without min and max spans its tiers.
func (r *RollDef) Bounds() (lo, hi int) {
	if r.Distribution != Weighted || r.Min != 0 || r.Max != 0 {
		return r.Min, r.Max
	}
	lo, hi = math.MaxInt, math.MinInt
	for _, w := range r.Weights {
		lo, hi = min(lo, w.Min), max(hi, w.Max)
	}
	return lo, hi
}

// Clamp keeps a value inside a roll's range.
func (r *RollDef) Clamp(v int) int {
	lo, hi := r.Bounds()
	return min(max(v, lo), hi)
}

// maxRollValue keeps values inside int32, which the protocol and
// rAthena's random options use.
const maxRollValue = math.MaxInt32

// Check checks a roll's data. It returns the first problem.
func (r *RollDef) Check() error {
	switch r.Distribution {
	case "", Uniform, Triangular:
		if len(r.Weights) > 0 {
			return errors.New("only a weighted roll has weights")
		}
		if r.Min > r.Max {
			return fmt.Errorf("min %d is above max %d", r.Min, r.Max)
		}
	case Weighted:
		if len(r.Weights) == 0 {
			return errors.New("a weighted roll needs weights")
		}
		if len(r.Weights) > 64 {
			return errors.New("a weighted roll has at most 64 weights")
		}
		for i, w := range r.Weights {
			if w.Weight <= 0 || w.Weight > 1_000_000 {
				return fmt.Errorf("weight %d: weight is 1 to 1000000", i+1)
			}
			if w.Min > w.Max {
				return fmt.Errorf("weight %d: %d is above %d", i+1, w.Min, w.Max)
			}
		}
		lo, hi := math.MaxInt, math.MinInt
		for _, w := range r.Weights {
			lo, hi = min(lo, w.Min), max(hi, w.Max)
		}
		if (r.Min != 0 || r.Max != 0) && (lo < r.Min || hi > r.Max) {
			return fmt.Errorf("the weights reach %d to %d, outside min %d and max %d", lo, hi, r.Min, r.Max)
		}
	default:
		return fmt.Errorf("distribution %q is not uniform, triangular or weighted", r.Distribution)
	}
	if len(r.DisplayName) > MaxRollDisplayName {
		return fmt.Errorf("display_name is 1 to %d bytes", MaxRollDisplayName)
	}
	lo, hi := r.Bounds()
	if lo < -maxRollValue || hi > maxRollValue {
		return fmt.Errorf("values are %d to %d", -maxRollValue, maxRollValue)
	}
	if r.Default < lo || r.Default > hi {
		return fmt.Errorf("default %d is outside %d to %d", r.Default, lo, hi)
	}
	return nil
}

// Random draws a roll's value from r.
//
//   - uniform: every value from min to max is equally likely.
//   - triangular: a value's weight falls in a straight line from the
//     default to one past each end, so values near the default are
//     common and min and max are rare.
//   - weighted: a tier is picked by weight, then a value of its range,
//     each equally likely.
func (d *RollDef) Random(r Rand) int {
	switch d.Distribution {
	case Uniform:
		return d.Min + r.IntN(d.Max-d.Min+1)
	case Weighted:
		total := 0
		for _, w := range d.Weights {
			total += w.Weight
		}
		n := r.IntN(total)
		for _, w := range d.Weights {
			if n < w.Weight {
				return w.Min + r.IntN(w.Max-w.Min+1)
			}
			n -= w.Weight
		}
		return d.Default
	}
	return triangular(d.Min, d.Max, d.Default, r)
}

// triangular draws from the discrete triangle over lo to hi with its peak
// at mode. A value v at or below mode weighs (v-lo+1)*(hi-mode+1), and
// one above it (hi-v+1)*(mode-lo+1), so both sides meet at the peak.
func triangular(lo, hi, mode int, r Rand) int {
	left, right := int64(mode-lo+1), int64(hi-mode+1)
	// The left side, lo to mode, weighs right * left(left+1)/2. The right
	// side, mode+1 to hi, weighs left * (right-1)right/2.
	leftTotal := right * left * (left + 1) / 2
	total := leftTotal + left*(right-1)*right/2
	n := int64(r.IntN(int(total)))
	if n < leftTotal {
		// The cumulative weight of lo..lo+k-1 is right * k(k+1)/2.
		k := sort.Search(int(left), func(k int) bool { return right*int64(k+1)*int64(k+2)/2 > n })
		return lo + k
	}
	n -= leftTotal
	// From mode+1, step j weighs left * (right-1-j).
	k := sort.Search(int(right-1), func(j int) bool {
		j64 := int64(j + 1)
		return left*(j64*(right-1)-j64*(j64-1)/2) > n
	})
	return mode + 1 + k
}

// Probability is the chance of a value, for tests.
func (d *RollDef) Probability(v int) float64 {
	switch d.Distribution {
	case Uniform:
		if v < d.Min || v > d.Max {
			return 0
		}
		return 1 / float64(d.Max-d.Min+1)
	case Weighted:
		total := 0
		for _, w := range d.Weights {
			total += w.Weight
		}
		p := 0.0
		for _, w := range d.Weights {
			if v >= w.Min && v <= w.Max {
				p += float64(w.Weight) / float64(total) / float64(w.Max-w.Min+1)
			}
		}
		return p
	}
	lo, hi, mode := d.Min, d.Max, d.Default
	left, right := float64(mode-lo+1), float64(hi-mode+1)
	total := right*left*(left+1)/2 + left*(right-1)*right/2
	switch {
	case v < lo || v > hi:
		return 0
	case v <= mode:
		return float64(v-lo+1) * right / total
	}
	return float64(hi-v+1) * left / total
}

// Defaults are an item's rolls at their defaults.
func (d *Def) Defaults() Rolls {
	if len(d.Rolls) == 0 {
		return nil
	}
	m := make(map[string]int, len(d.Rolls))
	for n, r := range d.Rolls {
		m[n] = r.Default
	}
	return RollsOf(m)
}

// FixRolls checks a copy's rolls against its item's data, as the server
// does when it loads a copy. Values outside a roll's range are clamped,
// names the item no longer has are dropped, and rolls the item gained
// get their default. It returns the fixed rolls and what it changed.
func (d *Def) FixRolls(r Rolls) (Rolls, []string) {
	var notes []string
	m := make(map[string]int, len(d.Rolls))
	for _, x := range r {
		def, ok := d.Rolls[x.Name]
		if !ok {
			notes = append(notes, fmt.Sprintf("roll %s dropped: the item has no such roll", x.Name))
			continue
		}
		v := def.Clamp(x.Value)
		if v != x.Value {
			notes = append(notes, fmt.Sprintf("roll %s clamped from %d to %d", x.Name, x.Value, v))
		}
		m[x.Name] = v
	}
	for n, def := range d.Rolls {
		if _, ok := m[n]; !ok {
			m[n] = def.Default
			if len(r) > 0 {
				notes = append(notes, fmt.Sprintf("roll %s added at its default", n))
			}
		}
	}
	return RollsOf(m), notes
}
