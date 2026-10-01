// Package constants gives rAthena's script constants the readable names
// of the Lua API (proposal 0002, "Constants and IDs"). It reads the
// script_constants.json extract. The converter uses it to turn rAthena's
// names into wAthena's, and the Lua runtime builds its constant modules
// from it.
//
// Readable names are made by a fixed rule (proposal 0002, decision 12):
//   - The rAthena prefix is removed, such as RC_ or SC_.
//   - The rest is written in lower case. Underscores stay.
//   - A few names get a word split or a spelled-out word, listed in
//     renames, such as RC_DEMIHUMAN to demi_human and W_1HSWORD to
//     one_handed_sword.
package constants

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Data is the script_constants.json extract.
type Data struct {
	Mode string `json:"mode"`
	// Constants and Parameters map rAthena's names to their values.
	// rAthena matches them without case.
	Constants  map[string]int64 `json:"constants"`
	Parameters map[string]int64 `json:"parameters"`
	// Skills maps skill_db names, such as SM_BASH, to skill IDs.
	Skills map[string]int64 `json:"skills"`

	lowerConsts, lowerParams, lowerSkills map[string]int64
	skillNames                            map[int64]string
}

// Read decodes a script_constants.json extract.
func Read(r io.Reader) (*Data, error) {
	var d Data
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		return nil, fmt.Errorf("script_constants.json: %w", err)
	}
	if len(d.Constants) == 0 {
		return nil, fmt.Errorf("script_constants.json: no constants")
	}
	d.index()
	return &d, nil
}

// Module is one constant module, such as wa.races.
type Module struct {
	// Name is the module's name after wa., such as races.
	Name string
	// Values maps readable names to values.
	Values map[string]int64
	// Names maps values to the readable name the converter writes. When
	// two names share a value, the one listed first in sorted order wins.
	Names map[int64]string
}

// moduleSpec says which rAthena constants make a module.
type moduleSpec struct {
	name   string
	prefix string
	// skip are rAthena names that are not values, such as RC2_MAX.
	skip []string
}

var moduleSpecs = []moduleSpec{
	{"races", "RC_", nil},
	{"monster_groups", "RC2_", []string{"RC2_NONE", "RC2_MAX"}},
	{"elements", "ELE_", []string{"ELE_WEAPON", "ELE_ENDOWED", "ELE_RANDOM"}},
	{"sizes", "Size_", nil},
	{"monster_classes", "CLASS_", []string{"CLASS_NONE", "CLASS_MAX"}},
	{"weapon_types", "W_", nil},
	{"statuses", "SC_", []string{"SC_NONE", "SC_MAX"}},
	{"effects", "EF_", []string{"EF_NONE", "EF_MAX"}},
	// Headgear effects (hateffect) have IDs of their own.
	{"headgear_effects", "HAT_EF_", []string{"HAT_EF_MAX"}},
	{"jobs", "JOB_", []string{"JOB_MAX", "JOB_MAX_BASIC"}},
}

// renames are readable names that the rule alone does not give.
var renames = map[string]string{
	"RC_DEMIHUMAN": "demi_human",
	"W_1HSWORD":    "one_handed_sword",
	"W_2HSWORD":    "two_handed_sword",
	"W_1HSPEAR":    "one_handed_spear",
	"W_2HSPEAR":    "two_handed_spear",
	"W_1HAXE":      "one_handed_axe",
	"W_2HAXE":      "two_handed_axe",
	"W_2HMACE":     "two_handed_mace",
	"W_2HSTAFF":    "two_handed_staff",
	"W_DOUBLE_DD":  "dual_daggers",
	"W_DOUBLE_SS":  "dual_swords",
	"W_DOUBLE_AA":  "dual_axes",
	"W_DOUBLE_DS":  "dagger_and_sword",
	"W_DOUBLE_DA":  "dagger_and_axe",
	"W_DOUBLE_SA":  "sword_and_axe",
}

// Modules builds the constant modules, by module name.
func (d *Data) Modules() map[string]*Module {
	out := map[string]*Module{}
	for _, spec := range moduleSpecs {
		m := &Module{Name: spec.name, Values: map[string]int64{}, Names: map[int64]string{}}
		var names []string
		for n := range d.Constants {
			if strings.HasPrefix(n, spec.prefix) && !contains(spec.skip, n) {
				names = append(names, n)
			}
		}
		sort.Strings(names)
		for _, n := range names {
			readable := Readable(n, spec.prefix)
			if readable == "" {
				continue
			}
			v := d.Constants[n]
			if _, dup := m.Values[readable]; dup {
				continue
			}
			m.Values[readable] = v
			if _, ok := m.Names[v]; !ok {
				m.Names[v] = readable
			}
		}
		out[spec.name] = m
	}
	return out
}

// Readable is the readable name of an rAthena constant in a module.
func Readable(name, prefix string) string {
	if r, ok := renames[name]; ok {
		return r
	}
	s := strings.ToLower(strings.TrimPrefix(name, prefix))
	if s == "" || !isIdentifier(s) {
		return ""
	}
	if s[0] >= '0' && s[0] <= '9' {
		// A Lua name cannot start with a digit: SC_2011RWC_SCROLL is
		// statuses.n2011rwc_scroll.
		s = "n" + s
	}
	return s
}

func isIdentifier(s string) bool {
	for _, c := range s {
		if c != '_' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Lookup finds an rAthena constant or parameter without case, as
// rAthena's script engine does. param reports a parameter, such as
// BaseLevel.
func (d *Data) Lookup(name string) (value int64, param, ok bool) {
	d.index()
	if v, ok := d.lowerParams[strings.ToLower(name)]; ok {
		return v, true, true
	}
	v, ok := d.lowerConsts[strings.ToLower(name)]
	return v, false, ok
}

// Skill finds a skill by its skill_db name, without case.
func (d *Data) Skill(name string) (int64, bool) {
	d.index()
	v, ok := d.lowerSkills[strings.ToLower(name)]
	return v, ok
}

// SkillName is the skill_db name of a skill ID.
func (d *Data) SkillName(id int64) (string, bool) {
	d.index()
	n, ok := d.skillNames[id]
	return n, ok
}

// index builds the lookup maps once. Read calls it, so a Data from Read
// is safe to share between goroutines.
func (d *Data) index() {
	if d.lowerConsts != nil {
		return
	}
	d.lowerConsts = make(map[string]int64, len(d.Constants))
	for k, v := range d.Constants {
		d.lowerConsts[strings.ToLower(k)] = v
	}
	d.lowerParams = make(map[string]int64, len(d.Parameters))
	for k, v := range d.Parameters {
		d.lowerParams[strings.ToLower(k)] = v
	}
	d.lowerSkills = make(map[string]int64, len(d.Skills))
	d.skillNames = make(map[int64]string, len(d.Skills))
	for k, v := range d.Skills {
		d.lowerSkills[strings.ToLower(k)] = v
		d.skillNames[v] = k
	}
}
