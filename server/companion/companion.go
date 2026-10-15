// Package companion holds companions, mounts and carts (proposal 0002,
// review decision 25): the records of data/companions.yml and
// data/mounts.yml, the core rules each record acts as, and the option
// bits of rAthena that show them to players.
//
// Core's records are files of this package (core/), which the core mod
// also carries. server/gamedata merges the mods' records over them.
package companion

import (
	"embed"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// CoreFiles are core's data/companions.yml and data/mounts.yml, in
// core/.
//
//go:embed core
var CoreFiles embed.FS

// Rule is the core behavior a record acts as.
type Rule int

// The rules. Each is one of rAthena's option checks.
const (
	RuleNone Rule = iota
	// RuleFalcon is pc_isfalcon (OPTION_FALCON).
	RuleFalcon
	// RuleRiding is pc_isriding (OPTION_RIDING): the Peco Peco, the
	// Grand Peco and the gryphon.
	RuleRiding
	// RuleDragon is pc_isridingdragon (OPTION_DRAGON).
	RuleDragon
	// RuleWugRider is pc_isridingwug (OPTION_WUGRIDER).
	RuleWugRider
	// RuleMadogear is pc_ismadogear (OPTION_MADOGEAR).
	RuleMadogear
)

// rAthena's option bits that show a companion, a mount or a cart.
const (
	OptionFalcon   = 0x0010
	OptionRiding   = 0x0020
	OptionDragon1  = 0x0008_0000
	OptionWug      = 0x0010_0000
	OptionWugRider = 0x0020_0000
	OptionMadogear = 0x0040_0000
	OptionDragon2  = 0x0080_0000
	OptionDragon3  = 0x0100_0000
	OptionDragon4  = 0x0200_0000
	OptionDragon5  = 0x0400_0000
	// OptionDragon is every dragon color.
	OptionDragon = OptionDragon1 | OptionDragon2 | OptionDragon3 | OptionDragon4 | OptionDragon5
	// OptionCart is every cart look, OPTION_CART1 to OPTION_CART5.
	OptionCart = 0x0008 | 0x0080 | 0x0100 | 0x0200 | 0x0400
	// OptionLooks are every bit a companion or a mount shows.
	OptionLooks = OptionFalcon | OptionRiding | OptionDragon | OptionWug | OptionWugRider | OptionMadogear
)

// OptionInvisible is rAthena's OPTION_INVISIBLE, the bit of @hide. The
// character saves it with the other bits, as pc_makesavestatus does, and
// the zone keeps it apart from them while the player plays.
const OptionInvisible = 0x0040

// CartOptions are the option bits of each cart look, as pc_setcart's
// table without NEW_CARTS: 0 is no cart, then OPTION_CART1 to
// OPTION_CART5.
var CartOptions = [...]int{0, 0x0008, 0x0080, 0x0100, 0x0200, 0x0400}

// MaxCart is the highest cart look.
const MaxCart = len(CartOptions) - 1

// CartLook is the cart look of option bits, or 0 for none.
func CartLook(option int) int {
	for i := MaxCart; i > 0; i-- {
		if option&CartOptions[i] != 0 {
			return i
		}
	}
	return 0
}

// CompanionLooks are the companion sprites the client can show, with the
// option bit that shows each.
var CompanionLooks = map[string]int{"falcon": OptionFalcon, "warg": OptionWug}

// MountLooks are the mount sprites the client can show, with the option
// bit that shows each. The Peco Peco, the Grand Peco and the gryphon are
// all OPTION_RIDING: the client shows the mounted sprite of the player's
// job, as rAthena does.
var MountLooks = map[string]int{
	"peco_peco": OptionRiding, "grand_peco": OptionRiding, "gryphon": OptionRiding,
	"dragon": OptionDragon1, "dragon_brown": OptionDragon2, "dragon_gray": OptionDragon3,
	"dragon_blue": OptionDragon4, "dragon_red": OptionDragon5,
	"warg": OptionWugRider, "madogear": OptionMadogear,
}

// companionRules and mountRules are the core records that acts_as may
// name, with their rule.
var (
	companionRules = map[string]Rule{"falcon": RuleFalcon}
	mountRules     = map[string]Rule{
		"peco_peco": RuleRiding, "grand_peco": RuleRiding, "gryphon": RuleRiding,
		"dragon": RuleDragon, "warg": RuleWugRider, "madogear": RuleMadogear,
	}
)

// Requirement is one skill a record needs, at a level.
type Requirement struct {
	Skill, Level int
	// Name is the skill's name, for messages.
	Name string
}

// Record is a companion or a mount.
type Record struct {
	ID int
	// Name is the record's full name: a core name such as falcon, or a
	// mod's such as example.familiars:Owl.
	Name        string
	DisplayName string
	// Look is the sprite, a key of CompanionLooks or MountLooks, or an
	// asset name.
	Look string
	// Art is Look when it names art, such as
	// mod/example.familiars/sprite/companion/Owl (decision 0033).
	Art string
	// ActsAs is the core record whose rules apply, and Rule its rule.
	ActsAs string
	Rule   Rule
	// Requires are the skills a player needs to have it.
	Requires []Requirement
	// Bonuses apply while the player has it, as an item's static bonuses.
	Bonuses []*bonus.Bonus
	// Speed is a mount's walk speed bonus in percent, as rAthena's 25 for
	// a Peco Peco. HasSpeed is false when the record leaves it to its
	// rule.
	Speed    int
	HasSpeed bool
	// Option is the option bit of the look.
	Option int
	// Mod is the mod that added the record, or core.
	Mod string
}

// Meets reports whether a player with the skill levels of level meets
// the record's requires.
func (r *Record) Meets(level func(skill int) int) bool {
	for _, q := range r.Requires {
		if level(q.Skill) < q.Level {
			return false
		}
	}
	return true
}

// Is reports whether the record is the named record or acts as it.
func (r *Record) Is(name string) bool {
	return r != nil && (r.Name == name || r.ActsAs == name)
}

// Table is one kind of record: companions or mounts.
type Table struct {
	byID   map[int]*Record
	byName map[string]*Record
}

// Get returns a record by ID, or nil.
func (t *Table) Get(id int) *Record {
	if t == nil {
		return nil
	}
	return t.byID[id]
}

// ByName returns a record by full name, or nil.
func (t *Table) ByName(name string) *Record {
	if t == nil {
		return nil
	}
	return t.byName[name]
}

// Names are the records' names, sorted.
func (t *Table) Names() []string {
	if t == nil {
		return nil
	}
	return slices.Sorted(maps.Keys(t.byName))
}

func (t *Table) add(r *Record) {
	if t.byID == nil {
		t.byID, t.byName = map[int]*Record{}, map[string]*Record{}
	}
	t.byID[r.ID] = r
	t.byName[r.Name] = r
}

// Set is every companion and mount.
type Set struct {
	Companions, Mounts Table
}

// Kind is companions or mounts.
type Kind struct {
	// Name is "companions" or "mounts", for messages.
	Name string
	// File is the data file, such as data/companions.yml.
	File  string
	looks map[string]int
	rules map[string]Rule
	mount bool
}

// The two kinds.
var (
	Companions = Kind{Name: "companions", File: "data/companions.yml", looks: CompanionLooks, rules: companionRules}
	Mounts     = Kind{Name: "mounts", File: "data/mounts.yml", looks: MountLooks, rules: mountRules, mount: true}
)

// Fields are the fields of a record besides name.
func (k Kind) Fields() []string {
	f := []string{"id", "display_name", "look", "acts_as", "requires", "bonuses"}
	if k.mount {
		f = append(f, "speed")
	}
	return f
}

// MaxSpeed is the highest speed of a mount record.
const MaxSpeed = 50

// Names resolve the names a record writes.
type Names struct {
	// Skill finds a skill by name, such as HT_FALCON.
	Skill func(name string) (int, bool)
	// Bonus resolves the names in bonuses. Nil leaves bonuses out.
	Bonus bonus.Names
}

// Build makes a record from its fields. id is the record's ID: core's
// own, or the one the server gave a mod's record.
func (k Kind) Build(name, modID string, id int, fields map[string]any, names Names) (*Record, error) {
	r := &Record{ID: id, Name: name, Mod: modID, DisplayName: localName(name)}
	var errs []string
	bad := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	for _, f := range slices.Sorted(maps.Keys(fields)) {
		if f != "id" && !slices.Contains(k.Fields(), f) {
			bad("%q is not a field; %s have %s", f, k.Name, strings.Join(k.Fields(), ", "))
		}
	}
	if v, ok := fields["display_name"]; ok {
		s, _ := v.(string)
		if s == "" || len(s) > 64 {
			bad("display_name must be 1 to 64 bytes")
		}
		r.DisplayName = s
	}
	r.Look, _ = fields["look"].(string)
	r.ActsAs, _ = fields["acts_as"].(string)
	if _, ok := fields["acts_as"]; !ok {
		r.ActsAs = name
	}
	if rule, ok := k.rules[r.ActsAs]; ok {
		r.Rule = rule
	} else if _, set := fields["acts_as"]; set {
		bad("acts_as must be a core %s record: %s", strings.TrimSuffix(k.Name, "s"), strings.Join(slices.Sorted(maps.Keys(k.rules)), ", "))
	} else {
		// A new record without acts_as has no core rules.
		r.ActsAs = ""
	}
	switch opt, ok := k.looks[r.Look]; {
	case ok:
		r.Option = opt
	case mod.IsArtName(r.Look):
		// A sprite of art shows with the option bit of the core record
		// it acts as (decision 0033).
		if err := mod.CheckArtName(r.Look, "sprite"); err != nil {
			bad("look: %v", err)
		}
		r.Art = r.Look
		if opt, ok := k.looks[r.ActsAs]; ok {
			r.Option = opt
		} else {
			bad("look: a sprite of art needs acts_as, the core record whose option bit shows it")
		}
	default:
		bad("look must be one of %s, or an art name", strings.Join(slices.Sorted(maps.Keys(k.looks)), ", "))
	}
	if v, ok := fields["requires"]; ok {
		list, ok := v.([]any)
		if !ok {
			bad("requires must be a list of { skill, level }")
		}
		for i, e := range list {
			m, ok := e.(map[string]any)
			s, _ := m["skill"].(string)
			lv, isInt := m["level"].(int)
			if _, has := m["level"]; !has {
				lv, isInt = 1, true
			}
			if !ok || len(m) > 2 || s == "" || !isInt || lv < 1 || lv > 100 {
				bad("requires[%d]: want { skill: HT_FALCON, level: 1 }", i)
				continue
			}
			sid, found := names.Skill(s)
			if !found {
				bad("requires[%d]: there is no skill %q", i, s)
				continue
			}
			r.Requires = append(r.Requires, Requirement{Skill: sid, Level: lv, Name: s})
		}
	}
	if v, ok := fields["bonuses"]; ok && names.Bonus != nil {
		list, ok := v.([]any)
		if !ok {
			bad("bonuses must be a list of bonuses, such as [[str, 2]]")
		}
		for i, e := range list {
			b, err := bonus.FromData(e, names.Bonus)
			if err != nil {
				bad("bonuses[%d]: %v", i, err)
				continue
			}
			if len(b.Rolls()) > 0 {
				bad("bonuses[%d]: a %s bonus cannot read a roll", i, strings.TrimSuffix(k.Name, "s"))
				continue
			}
			r.Bonuses = append(r.Bonuses, b)
		}
	}
	if v, ok := fields["speed"]; ok {
		n, ok := v.(int)
		if !ok || n < 0 || n > MaxSpeed {
			bad("speed must be a whole number from 0 to %d: the walk speed bonus in percent", MaxSpeed)
		}
		r.Speed, r.HasSpeed = n, true
	}
	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	return r, nil
}

// localName is a record's name without its mod prefix.
func localName(name string) string {
	if i := strings.LastIndex(name, ":"); i >= 0 {
		return name[i+1:]
	}
	return name
}

// Add puts a record into the set.
func (s *Set) Add(k Kind, r *Record) {
	if k.mount {
		s.Mounts.add(r)
	} else {
		s.Companions.add(r)
	}
}

// Table returns the set's table of a kind.
func (s *Set) Table(k Kind) *Table {
	if s == nil {
		return nil
	}
	if k.mount {
		return &s.Mounts
	}
	return &s.Companions
}
