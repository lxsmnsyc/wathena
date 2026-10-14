package clientdata

import (
	_ "embed"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

//go:embed skills.yml
var skillsYAML []byte

// Mod client data tables (proposal 0002, review decision 38, and
// docs/client-data.md): a mod may ship client/<table>.yml with entries
// that override core's entries of the same key, and new entries. The
// server merges core's tables with every mod's, in load order, and
// serves one set.

// Table is one client data table: its file and the lists it holds, each
// with the field that keys its entries.
type Table struct {
	File  string
	Lists map[string]string
	// check parses and checks a whole table.
	check func([]byte) error
	core  []byte
}

// Tables are the tables a mod may override, by file name.
var Tables = map[string]*Table{
	"jobs.yml":    {File: "jobs.yml", Lists: map[string]string{"looks": "look", "jobs": "id"}, check: func(b []byte) error { _, err := ParseJobs(b); return err }, core: jobsYAML},
	"weapons.yml": {File: "weapons.yml", Lists: map[string]string{"weapons": "id"}, check: func(b []byte) error { _, err := ParseWeapons(b); return err }, core: weaponsYAML},
	"effects.yml": {File: "effects.yml", Lists: map[string]string{"effects": "id"}, check: func(b []byte) error { _, err := ParseEffects(b); return err }, core: effectsYAML},
	"skills.yml":  {File: "skills.yml", Lists: map[string]string{"skills": "id", "units": "id", "auras": "id"}, check: func(b []byte) error { _, err := ParseSkills(b); return err }, core: skillsYAML},
}

// MaxTableBytes is the largest table file a mod may ship.
const MaxTableBytes = 4 << 20

// Override is one mod's table file.
type Override struct {
	// Mod is the mod's ID, and File the table's file name, such as
	// jobs.yml.
	Mod, File string
	Data      []byte
}

// Merged is the merged tables, by file name, in the form the web client
// reads: each table's lists, as in its YAML file.
type Merged map[string]map[string]any

// Merge applies overrides, in order, over core's tables, and checks each
// merged table against its schema. Errors name the mod and the file.
func Merge(overrides []Override) (Merged, error) {
	out := Merged{}
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(Tables)) {
		doc, err := decode(Tables[name].core)
		if err != nil {
			return nil, fmt.Errorf("client/data/%s: %w", name, err)
		}
		out[name] = doc
	}
	for _, o := range overrides {
		where := o.Mod + ": client/" + o.File
		t, ok := Tables[o.File]
		if !ok {
			errs = append(errs, fmt.Errorf("%s: not a client data table; the tables are %s", where, strings.Join(slices.Sorted(maps.Keys(Tables)), ", ")))
			continue
		}
		if len(o.Data) > MaxTableBytes {
			errs = append(errs, fmt.Errorf("%s: %d bytes, over the limit of %d", where, len(o.Data), MaxTableBytes))
			continue
		}
		doc, err := decode(o.Data)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", where, err))
			continue
		}
		if err := apply(out[o.File], doc, t); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", where, err))
			continue
		}
		// The table must still pass its checks with the mod's entries.
		b, err := yaml.Marshal(out[o.File])
		if err == nil {
			err = t.check(b)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", where, err))
		}
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return out, nil
}

// decode reads a table as YAML. An empty file is an empty table.
// Aliases are refused: a few of them can stand for a table too large to
// write out.
func decode(data []byte) (map[string]any, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if hasAlias(&root) {
		return nil, errors.New("YAML aliases are not allowed in client data tables")
	}
	doc := map[string]any{}
	if err := root.Decode(&doc); err != nil && root.Kind != 0 {
		return nil, err
	}
	return doc, nil
}

func hasAlias(n *yaml.Node) bool {
	if n.Kind == yaml.AliasNode {
		return true
	}
	for _, c := range n.Content {
		if hasAlias(c) {
			return true
		}
	}
	return false
}

// apply puts a mod's entries into a table: an entry with the key of an
// entry there replaces it, and others are added at the end.
func apply(into, doc map[string]any, t *Table) error {
	for _, list := range slices.Sorted(maps.Keys(doc)) {
		key, ok := t.Lists[list]
		if !ok {
			return fmt.Errorf("%q is not a list of %s; it has %s", list, t.File, strings.Join(slices.Sorted(maps.Keys(t.Lists)), ", "))
		}
		entries, ok := doc[list].([]any)
		if !ok {
			return fmt.Errorf("%s is a list", list)
		}
		have, _ := into[list].([]any)
		index := map[string]int{}
		for i, e := range have {
			if m, ok := e.(map[string]any); ok {
				index[fmt.Sprint(m[key])] = i
			}
		}
		for i, e := range entries {
			m, ok := e.(map[string]any)
			if !ok || m[key] == nil {
				return fmt.Errorf("%s[%d] needs %s", list, i, key)
			}
			k := fmt.Sprint(m[key])
			if j, ok := index[k]; ok {
				have[j] = m
				continue
			}
			index[k] = len(have)
			have = append(have, m)
		}
		into[list] = have
	}
	return nil
}

// SkillEffect is one skill of skills.yml.
type SkillEffect struct {
	ID       int    `yaml:"id"`
	Constant string `yaml:"constant"`
	Caster   []int  `yaml:"caster"`
	Effect   []int  `yaml:"effect"`
	Hit      []int  `yaml:"hit"`
	Aura     *bool  `yaml:"aura"`
}

// UnitEffect is a skill unit's effect.
type UnitEffect struct {
	ID       int    `yaml:"id"`
	Constant string `yaml:"constant"`
	Effect   *int   `yaml:"effect"`
}

// Aura is an element's cast aura.
type Aura struct {
	ID       int    `yaml:"id"`
	Constant string `yaml:"constant"`
	Effect   int    `yaml:"effect"`
}

// Skills is skills.yml.
type Skills struct {
	Skills []SkillEffect `yaml:"skills"`
	Units  []UnitEffect  `yaml:"units"`
	Auras  []Aura        `yaml:"auras"`
}

// ParseSkills parses and checks a skill effects table.
func ParseSkills(data []byte) (*Skills, error) {
	var s Skills
	if err := strictUnmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("skills.yml: %w", err)
	}
	for _, list := range []struct {
		name string
		ids  []int
	}{{"skill", ids(s.Skills, func(e SkillEffect) (int, string) { return e.ID, e.Constant })},
		{"unit", ids(s.Units, func(e UnitEffect) (int, string) { return e.ID, e.Constant })},
		{"aura", ids(s.Auras, func(e Aura) (int, string) { return e.ID, e.Constant })}} {
		seen := map[int]bool{}
		for _, id := range list.ids {
			if id < 0 || seen[id] {
				return nil, fmt.Errorf("skills.yml: %s %d is invalid or listed twice", list.name, id)
			}
			seen[id] = true
		}
	}
	return &s, nil
}

// ids lists the IDs of a list's entries, with -1 for an entry without a
// constant.
func ids[T any](list []T, f func(T) (int, string)) []int {
	out := make([]int, len(list))
	for i, e := range list {
		id, c := f(e)
		if c == "" {
			id = -1
		}
		out[i] = id
	}
	return out
}
