package layout

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/server/mod"
)

// namingFields are the fields of a naming style record.
var namingFields = []string{"biome", "syllables", "patterns"}

// ParseNamingStyle reads a naming style from its record's fields and
// checks it.
func ParseNamingStyle(name string, fields map[string]any) (NamingStyle, error) {
	for _, k := range slices.Sorted(maps.Keys(fields)) {
		if !slices.Contains(namingFields, k) {
			return NamingStyle{}, fmt.Errorf("%q is not a field; the fields are %s", k, strings.Join(namingFields, ", "))
		}
	}
	s := NamingStyle{Name: name}
	var ok bool
	if s.Biome, ok = fields["biome"].(string); !ok {
		return NamingStyle{}, errors.New("biome is required: a biome's name, such as grassland")
	}
	syl, ok := fields["syllables"].(map[string]any)
	if !ok {
		return NamingStyle{}, errors.New("syllables is required: lists of syllables by part name, such as {start: [val, hrim], end: [heim]}")
	}
	for _, part := range slices.Sorted(maps.Keys(syl)) {
		list, ok := syl[part].([]any)
		if !ok {
			return NamingStyle{}, fmt.Errorf("syllables: %s is a list of syllables", part)
		}
		p := Part{Name: part}
		for _, v := range list {
			y, ok := v.(string)
			if !ok {
				return NamingStyle{}, fmt.Errorf("syllables: %s is a list of syllables", part)
			}
			p.Syllables = append(p.Syllables, y)
		}
		s.Parts = append(s.Parts, p)
	}
	pats, ok := fields["patterns"].([]any)
	if !ok {
		return NamingStyle{}, errors.New("patterns is required: a list of patterns, such as [\"{start}{end}\"]")
	}
	for _, v := range pats {
		text, ok := v.(string)
		if !ok {
			return NamingStyle{}, errors.New("patterns is a list of patterns, such as [\"{start}{end}\"]")
		}
		p, err := ParsePattern(text)
		if err != nil {
			return NamingStyle{}, err
		}
		s.Patterns = append(s.Patterns, p)
	}
	return s, s.Check()
}

// LoadNaming merges the mods' naming styles over core's and checks
// them. Styles keep the overlay's order: core's in its file's order,
// then the mods' in load order.
func LoadNaming(set *mod.Set) ([]NamingStyle, *mod.Overlay, error) {
	o, err := set.Overlay(mod.NamingStyleKind, func(string) bool { return false })
	if err != nil {
		return nil, nil, err
	}
	var out []NamingStyle
	var errs []error
	for _, r := range o.Records {
		s, err := ParseNamingStyle(r.Name, r.Fields)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: naming styles %s: %w", r.Where, r.Name, err))
			continue
		}
		out = append(out, s)
	}
	return out, o, errors.Join(errs...)
}

// CheckNaming checks the styles against the generator's biomes.
func CheckNaming(biomes []string, styles []NamingStyle) error {
	var errs []error
	for _, s := range styles {
		if !slices.Contains(biomes, s.Biome) {
			errs = append(errs, fmt.Errorf("naming styles %s: no biome %q; the biomes are %s", s.Name, s.Biome, strings.Join(biomes, ", ")))
		}
	}
	return errors.Join(errs...)
}
