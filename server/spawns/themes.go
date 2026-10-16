package spawns

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Theme is a spawn theme, a record of data/spawn_themes.yml (strategy
// section 8.11, review decision 44). A map may draw one: the monsters
// that match it weigh Boost percent of their weight.
type Theme struct {
	Name string
	// Biomes are where the theme may apply. Empty is everywhere.
	Biomes []string
	// Races and Elements match monsters by their data, as names of
	// wa.races and wa.elements. A monster whose tags name the theme
	// matches too.
	Races, Elements []string
	// Boost is the matching monsters' weight in percent, 100 to 1000.
	Boost int
}

// Limits of themes.
const (
	MinBoost = 100
	MaxBoost = 1000
	// ThemeChance is the chance in 3 that a map draws a theme.
	ThemeChance = 1
)

// ThemeFields are the fields of a spawn theme record.
var ThemeFields = []string{"biomes", "races", "elements", "boost"}

// ParseTheme reads a spawn theme record's fields. races and elements
// check the names of wa.races and wa.elements, and may be nil to skip
// the check.
func ParseTheme(name string, f map[string]any, races, elements func(string) bool) (Theme, error) {
	t := Theme{Name: name, Boost: 300}
	var errs []string
	for _, k := range slices.Sorted(maps.Keys(f)) {
		if !slices.Contains(ThemeFields, k) {
			errs = append(errs, fmt.Sprintf("%q is not a field; spawn themes have %s", k, strings.Join(ThemeFields, ", ")))
		}
	}
	list := func(k string, ok func(string) bool, what string) []string {
		v, has := f[k]
		if !has {
			return nil
		}
		l, isList := v.([]any)
		if !isList {
			errs = append(errs, k+" must be a list of names")
			return nil
		}
		var out []string
		for _, e := range l {
			s, _ := e.(string)
			if !ValidName(s) || (ok != nil && !ok(s)) {
				errs = append(errs, fmt.Sprintf("%s: %q is not %s", k, s, what))
				continue
			}
			if !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
		return out
	}
	t.Biomes = list("biomes", nil, "a biome name: 1 to 64 lowercase letters, digits and _")
	t.Races = list("races", races, "a race of wa.races")
	t.Elements = list("elements", elements, "an element of wa.elements")
	if v, ok := f["boost"]; ok {
		n, isInt := v.(int)
		if !isInt || n < MinBoost || n > MaxBoost {
			errs = append(errs, fmt.Sprintf("boost must be a whole number from %d to %d, the matching monsters' weight in percent", MinBoost, MaxBoost))
		} else {
			t.Boost = n
		}
	}
	if !ValidName(localName(name)) {
		errs = append(errs, "a theme's name, without the mod prefix, is 1 to 64 lowercase letters, digits and _")
	}
	if len(errs) > 0 {
		return Theme{}, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return t, nil
}

// localName is a record name without its mod prefix.
func localName(name string) string {
	if _, local, ok := strings.Cut(name, ":"); ok {
		return local
	}
	return name
}

// Applies reports whether the theme may apply in a biome.
func (t Theme) Applies(biome string) bool {
	return len(t.Biomes) == 0 || slices.Contains(t.Biomes, biome)
}

// Matches reports whether a monster matches the theme: its tags name the
// theme, by its full name or without the mod prefix, or its race or
// element is the theme's.
func (t Theme) Matches(m Monster) bool {
	if m.Tags != nil && (slices.Contains(m.Tags.Themes, t.Name) || slices.Contains(m.Tags.Themes, localName(t.Name))) {
		return true
	}
	return slices.Contains(t.Races, m.Race) || slices.Contains(t.Elements, m.Element)
}
