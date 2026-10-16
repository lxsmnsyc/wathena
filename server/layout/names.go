package layout

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Town names (strategy section 8.10, proposal 0002, review decision 43):
//
//   - A naming style is a record of data/naming_styles.yml: its biome,
//     its syllables by part, and its patterns, such as
//     "{start}{middle}{end}". Core has one per biome, and mods change
//     them and add their own. The last record for a biome, in load
//     order, is that biome's style.
//   - A town takes the style of its seat's biome. Attempt i draws from
//     the stream of name(i), the child ("name", i) of the town's public
//     node: a pattern, then a syllable for each part.
//   - A name is its syllables joined, in lower case but the first
//     letter. It passes when it has 4 to 12 letters, no other town or
//     the home map has it in any case, and the chat filter blocks none of
//     its words. Otherwise the next index tries, up to MaxNameTries.
//   - Other maps take their names from the nearest town and a template,
//     in English until locale files exist.

// Name rules.
const (
	MinNameLetters = 4
	MaxNameLetters = 12
	// MaxNameTries is how many name(i) a town tries.
	MaxNameTries = 100
	// MaxSyllable is the longest syllable, in letters.
	MaxSyllable = 12
	// MaxPatternParts is the most parts of one pattern.
	MaxPatternParts = 8
)

// NamingStyle is a naming style record.
type NamingStyle struct {
	// Name is the record's full name: a plain name for core's, such as
	// norse, and mod:name for a mod's.
	Name  string
	Biome string
	// Parts are the syllable lists by part name, in sorted order.
	Parts []Part
	// Patterns are the patterns, each a list of part names.
	Patterns [][]string
}

// Part is a named list of syllables.
type Part struct {
	Name      string
	Syllables []string
}

func (s *NamingStyle) part(name string) *Part {
	for i := range s.Parts {
		if s.Parts[i].Name == name {
			return &s.Parts[i]
		}
	}
	return nil
}

// Naming is the naming styles of a world, by biome.
type Naming struct {
	styles []NamingStyle
}

// NewNaming returns the styles in record order. A later style for a
// biome replaces an earlier one.
func NewNaming(styles []NamingStyle) *Naming {
	n := &Naming{}
	for _, s := range styles {
		if i := slices.IndexFunc(n.styles, func(o NamingStyle) bool { return o.Biome == s.Biome }); i >= 0 {
			n.styles[i] = s
			continue
		}
		n.styles = append(n.styles, s)
	}
	return n
}

// Styles lists the style of each biome, in the order the biomes first
// appeared.
func (n *Naming) Styles() []NamingStyle { return n.styles }

// For returns the style of a biome.
func (n *Naming) For(biome string) (*NamingStyle, bool) {
	for i := range n.styles {
		if n.styles[i].Biome == biome {
			return &n.styles[i], true
		}
	}
	return nil, false
}

// isLetter reports whether r is an ASCII letter.
func isLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }

// letters reports whether s is n to m ASCII letters.
func letters(s string, n, m int) bool {
	if len(s) < n || len(s) > m {
		return false
	}
	for _, r := range s {
		if !isLetter(r) {
			return false
		}
	}
	return true
}

// ParsePattern reads a pattern such as "{start}{middle}{end}" into its
// part names.
func ParsePattern(p string) ([]string, error) {
	var out []string
	rest := p
	for rest != "" {
		if rest[0] != '{' {
			return nil, fmt.Errorf("pattern %q: a pattern is part names in braces, such as {start}{end}", p)
		}
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			return nil, fmt.Errorf("pattern %q: a { has no }", p)
		}
		name := rest[1:end]
		if !validPartName(name) {
			return nil, fmt.Errorf("pattern %q: %q is not a part name: 1 to 32 lowercase letters, digits and _", p, name)
		}
		out = append(out, name)
		rest = rest[end+1:]
	}
	if len(out) == 0 || len(out) > MaxPatternParts {
		return nil, fmt.Errorf("pattern %q: a pattern has 1 to %d parts", p, MaxPatternParts)
	}
	return out, nil
}

func validPartName(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

// Check returns the style's first problem, or nil. Every pattern's part
// must have syllables, and some pattern must be able to make a name of
// 4 to 12 letters.
func (s *NamingStyle) Check() error {
	if s.Biome == "" {
		return errors.New("biome is required: the biome whose towns take the style")
	}
	if len(s.Parts) == 0 {
		return errors.New("syllables is required: lists of syllables by part name")
	}
	for _, p := range s.Parts {
		if !validPartName(p.Name) {
			return fmt.Errorf("syllables: %q is not a part name: 1 to 32 lowercase letters, digits and _", p.Name)
		}
		if len(p.Syllables) == 0 || len(p.Syllables) > 256 {
			return fmt.Errorf("syllables: %s has 1 to 256 syllables", p.Name)
		}
		for _, y := range p.Syllables {
			if !letters(y, 1, MaxSyllable) {
				return fmt.Errorf("syllables: %s: %q is not 1 to %d ASCII letters", p.Name, y, MaxSyllable)
			}
		}
	}
	if len(s.Patterns) == 0 || len(s.Patterns) > 32 {
		return errors.New("patterns is required: 1 to 32 patterns, such as {start}{end}")
	}
	fits := false
	for _, pat := range s.Patterns {
		lo, hi := 0, 0
		for _, name := range pat {
			p := s.part(name)
			if p == nil {
				return fmt.Errorf("patterns: no syllables for the part %s", name)
			}
			short, long := MaxSyllable, 0
			for _, y := range p.Syllables {
				short, long = min(short, len(y)), max(long, len(y))
			}
			lo, hi = lo+short, hi+long
		}
		if lo <= MaxNameLetters && hi >= MinNameLetters {
			fits = true
		}
	}
	if !fits {
		return fmt.Errorf("patterns: no pattern can make a name of %d to %d letters", MinNameLetters, MaxNameLetters)
	}
	return nil
}

// Make draws a name from a stream: a pattern, then a syllable for each
// of its parts. The name is in lower case but its first letter.
func (s *NamingStyle) Make(r *rng.Rand) string {
	pat := s.Patterns[r.IntN(len(s.Patterns))]
	var b strings.Builder
	for _, name := range pat {
		p := s.part(name)
		b.WriteString(p.Syllables[r.IntN(len(p.Syllables))])
	}
	name := strings.ToLower(b.String())
	if name == "" {
		return ""
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// NameNode returns the node of a town's name attempt i: the child
// ("name", i) of the town's public node.
func NameNode(public rng.Node, town int64, i int) rng.Node {
	return gen.TownNode(public, town).Child("name", int64(i))
}

// ValidTownName reports whether a name has 4 to 12 ASCII letters.
func ValidTownName(name string) bool { return letters(name, MinNameLetters, MaxNameLetters) }

// nameTowns names each town, by number.
func nameTowns(l *Layout, in Inputs) error {
	var taken []string
	for _, m := range l.Maps {
		if m.Kind == KindHome {
			taken = append(taken, strings.ToLower(m.Name))
		}
	}
	for _, t := range l.Towns() {
		biome := ""
		if t.Biome >= 0 && t.Biome < len(in.Generator.Biomes) {
			biome = in.Generator.Biomes[t.Biome]
		}
		style, ok := in.Naming.For(biome)
		if !ok {
			return fmt.Errorf("%w: town %s stands in %s, and no naming style has that biome", ErrNoName, gen.TownID(t.Town), biome)
		}
		name := ""
		for i := range MaxNameTries {
			try := style.Make(NameNode(in.Public, t.Town, i).Stream())
			if ValidTownName(try) && !slices.Contains(taken, strings.ToLower(try)) && (in.Blocked == nil || !in.Blocked(try)) {
				name = try
				break
			}
		}
		if name == "" {
			return fmt.Errorf("%w: town %s with the style %s after %d tries", ErrNoName, gen.TownID(t.Town), style.Name, MaxNameTries)
		}
		t.Name = name
		taken = append(taken, strings.ToLower(name))
	}
	return nil
}

// Directions of fields from their town, by the larger of the grid
// steps north and east. A tie goes north or south.
func direction(dx, dy int64) string {
	ax, ay := dx, dy
	if ax < 0 {
		ax = -ax
	}
	if ay < 0 {
		ay = -ay
	}
	switch {
	case dx == 0 && dy == 0:
		return ""
	case ay >= ax && dy > 0:
		return "North"
	case ay >= ax:
		return "South"
	case dx > 0:
		return "East"
	}
	return "West"
}

// StyleWord is the word of a dungeon style in its name: the style's
// name without its mod prefix, with _ and - as spaces, each word with a
// capital, such as Crypt or Sunken Hall.
func StyleWord(style string) string {
	words := strings.FieldsFunc(DungeonID(style), func(r rune) bool { return r == '_' || r == '-' || r == ' ' })
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}

// DeriveNames names the maps that are not towns or home maps from the
// nearest town by warps, ties to the lower number:
//
//   - A field is "<direction> <town> Fields", such as "North Valheim
//     Fields", with no direction on the town's own seat. Fields that
//     share a name get a number in map order, such as "North Valheim
//     Fields 2".
//   - A dungeon is "<style> of <town>", such as "Crypt of Skarvik",
//     numbered as fields when two share it, and each floor adds
//     " F<floor>".
//
// A map that no town reaches keeps its name. Renaming a town calls it
// again.
func DeriveNames(l *Layout) {
	at := index(l)
	adj := adjacency(l)
	towns := l.Towns()
	nearest := make([]int, len(l.Maps))
	best := make([]int, len(l.Maps))
	for i := range nearest {
		nearest[i], best[i] = -1, -1
	}
	for ti, t := range towns {
		d := distances(adj, at[t.ID])
		for i := range l.Maps {
			if d[i] >= 0 && (best[i] < 0 || d[i] < best[i]) {
				nearest[i], best[i] = ti, d[i]
			}
		}
	}
	base := make([]string, len(l.Maps))
	for i, m := range l.Maps {
		if nearest[i] < 0 {
			continue
		}
		t := towns[nearest[i]]
		switch m.Kind {
		case gen.KindField:
			base[i] = strings.TrimSpace(direction(m.X-t.X, m.Y-t.Y) + " " + t.Name + " Fields")
		case gen.KindDungeon:
			base[i] = StyleWord(m.Record) + " of " + t.Name
		}
	}
	// Dungeons share a base by their first floor, so every floor of one
	// dungeon gets one number.
	count := map[string]int{}
	firsts := map[string]string{}
	for i, m := range l.Maps {
		if base[i] == "" || m.Kind == gen.KindDungeon && m.Floor != 1 {
			continue
		}
		count[m.Kind+"|"+base[i]]++
	}
	seen := map[string]int{}
	for i, m := range l.Maps {
		if base[i] == "" {
			continue
		}
		key := m.Kind + "|" + base[i]
		name := base[i]
		switch {
		case m.Kind == gen.KindDungeon && m.Floor != 1:
			name = firsts[m.Dungeon]
		case count[key] > 1:
			seen[key]++
			name += " " + strconv.Itoa(seen[key])
		}
		if m.Kind == gen.KindDungeon {
			if m.Floor == 1 {
				firsts[m.Dungeon] = name
			}
			name += " F" + strconv.Itoa(m.Floor)
		}
		l.Maps[i].Name = name
	}
}
