package content

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/spawns"
)

// Kinds of quest templates (review decision 46).
const (
	// KindHunt asks for kills of a monster.
	KindHunt = "hunt"
	// KindCollect asks for items that a monster drops.
	KindCollect = "collect"
	// KindDelivery carries an item, or a message, to another town's
	// quest board.
	KindDelivery = "delivery"
)

// Kinds lists the template kinds.
var Kinds = []string{KindHunt, KindCollect, KindDelivery}

// Uses of quest templates: the quest boards of towns, and the tests of
// job guilds.
const (
	UseBoard = "board"
	UseGuild = "guild"
)

// Uses lists the uses.
var Uses = []string{UseBoard, UseGuild}

// Limits of templates.
const (
	// MaxTitle is the most characters of a title, as a quest's.
	MaxTitle = 255
	// MaxCount is the most kills or items a quest asks for.
	MaxCount = 1000
	// MaxReward is the highest reward percent.
	MaxReward = 1000
	// MaxWeight is the highest weight.
	MaxWeight = 1000
	// MaxDropRate is the highest drop rate, out of 10,000.
	MaxDropRate = 10000
	// MaxTemplates is the most templates a world may have.
	MaxTemplates = 1000
	// DefaultDropRate is a collect template's drop rate without one.
	DefaultDropRate = 3000
)

// Template is a quest template, a record of data/quest_templates.yml or
// of generation.quest_template (review decision 46).
type Template struct {
	// Name is the full name: core's plain name, such as hunt, or a
	// mod's, such as example.shrines:pilgrimage.
	Name string
	// Mod is the mod that added the record.
	Mod  string
	Kind string
	// Title is the quest's title, with {count}, {monster}, {item} and
	// {town} replaced when a world makes the quest.
	Title string
	// Count is the range of kills or items a hunt or collect quest asks
	// for, both ends included.
	Count [2]int
	// Roles are the spawn roles of the monsters a hunt or collect quest
	// may name: common and uncommon by default.
	Roles []spawns.Role
	// Item is the item a delivery carries, or "" for a message. A
	// collect template has none: it takes a drop of its monster.
	Item string
	// DropRate is a collect quest's drop rate, out of 10,000: each kill
	// of its monster gives the item at that rate while the quest is in
	// the log (decision 0043, "Kills").
	DropRate int
	// Reward is the reward in percent of the band's reward, 0 to
	// MaxReward. Guild tests give none.
	Reward int
	// Weight is the template's chance among the templates of a use.
	Weight int
	// Uses are where the template serves: board, guild or both.
	Uses []string
}

// Has reports whether the template serves a use.
func (t Template) Has(use string) bool { return slices.Contains(t.Uses, use) }

// TemplateFields are the fields of a template record.
var TemplateFields = []string{"kind", "title", "count", "roles", "item", "drop_rate", "reward", "weight", "uses"}

// placeholders are the parts a title may name.
var placeholders = []string{"{count}", "{monster}", "{item}", "{town}"}

// ParseTemplate reads a template record's fields. item, when not nil,
// reports whether an item name exists.
func ParseTemplate(name, modID string, f map[string]any, item func(string) bool) (Template, error) {
	t := Template{Name: name, Mod: modID, Reward: 100, Weight: 1, Uses: []string{UseBoard}}
	var errs []string
	bad := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	for _, k := range slices.Sorted(maps.Keys(f)) {
		if !slices.Contains(TemplateFields, k) {
			bad("%q is not a field; quest templates have %s", k, strings.Join(TemplateFields, ", "))
		}
	}
	t.Kind, _ = f["kind"].(string)
	if !slices.Contains(Kinds, t.Kind) {
		bad("kind must be %s", strings.Join(Kinds, ", "))
	}
	t.Title, _ = f["title"].(string)
	if t.Title == "" || utf8.RuneCountInString(t.Title) > MaxTitle {
		bad("title is text of 1 to %d characters", MaxTitle)
	}
	if err := checkTitle(t.Title); err != nil {
		bad("%v", err)
	}
	number := func(k string, lo, hi int, out *int) {
		v, ok := f[k]
		if !ok {
			return
		}
		n, isInt := v.(int)
		if !isInt || n < lo || n > hi {
			bad("%s must be a whole number from %d to %d", k, lo, hi)
			return
		}
		*out = n
	}
	number("reward", 0, MaxReward, &t.Reward)
	number("weight", 1, MaxWeight, &t.Weight)
	counted := t.Kind == KindHunt || t.Kind == KindCollect
	if v, ok := f["count"]; ok {
		lo, hi, isPair := countRange(v)
		switch {
		case !counted:
			bad("count is for hunt and collect templates")
		case !isPair || lo < 1 || hi < lo || hi > MaxCount:
			bad("count is a whole number or a range [lowest, highest] from 1 to %d", MaxCount)
		default:
			t.Count = [2]int{lo, hi}
		}
	} else if counted {
		bad("a %s template needs count: a whole number or a range [lowest, highest]", t.Kind)
	}
	if v, ok := f["roles"]; ok {
		list, isList := v.([]any)
		if !counted || !isList || len(list) == 0 {
			bad("roles is a list of spawn roles, for hunt and collect templates")
		}
		for _, e := range list {
			s, _ := e.(string)
			r, ok := spawns.ParseRole(s)
			if !ok {
				bad("roles: %q is not a spawn role", s)
				continue
			}
			if !slices.Contains(t.Roles, r) {
				t.Roles = append(t.Roles, r)
			}
		}
	} else if counted {
		t.Roles = []spawns.Role{spawns.Common, spawns.Uncommon}
	}
	if v, ok := f["item"]; ok {
		s, _ := v.(string)
		switch {
		case t.Kind != KindDelivery:
			bad("item is for delivery templates; a collect template takes a drop of its monster")
		case s == "" || item != nil && !item(s):
			bad("item: %q is not an item", s)
		default:
			t.Item = s
		}
	}
	if t.Kind == KindCollect {
		t.DropRate = DefaultDropRate
	}
	if _, ok := f["drop_rate"]; ok {
		if t.Kind != KindCollect {
			bad("drop_rate is for collect templates")
		} else {
			number("drop_rate", 0, MaxDropRate, &t.DropRate)
		}
	}
	if v, ok := f["uses"]; ok {
		list, isList := v.([]any)
		if !isList || len(list) == 0 {
			bad("uses is a list of %s", strings.Join(Uses, " and "))
		}
		t.Uses = nil
		for _, e := range list {
			s, _ := e.(string)
			if !slices.Contains(Uses, s) {
				bad("uses: %q is not %s", s, strings.Join(Uses, " or "))
				continue
			}
			if !slices.Contains(t.Uses, s) {
				t.Uses = append(t.Uses, s)
			}
		}
	}
	if t.Kind == KindDelivery && t.Has(UseGuild) {
		bad("a delivery template serves quest boards only: a guild test hunts and collects")
	}
	if len(errs) > 0 {
		return Template{}, errors.New(strings.Join(errs, "; "))
	}
	return t, nil
}

// countRange reads a count: a whole number, or [lowest, highest].
func countRange(v any) (lo, hi int, ok bool) {
	switch c := v.(type) {
	case int:
		return c, c, true
	case []any:
		if len(c) != 2 {
			return 0, 0, false
		}
		a, okA := c[0].(int)
		b, okB := c[1].(int)
		return a, b, okA && okB
	}
	return 0, 0, false
}

// checkTitle checks a title's placeholders: each {...} must be one of
// {count}, {monster}, {item} and {town}.
func checkTitle(title string) error {
	rest := title
	for {
		i := strings.IndexByte(rest, '{')
		if i < 0 {
			return nil
		}
		j := strings.IndexByte(rest[i:], '}')
		if j < 0 {
			return errors.New("title: a { has no }")
		}
		if p := rest[i : i+j+1]; !slices.Contains(placeholders, p) {
			return fmt.Errorf("title: %s is not one of %s", p, strings.Join(placeholders, ", "))
		}
		rest = rest[i+j+1:]
	}
}

// LoadTemplates reads the quest templates of a mod set: core's
// data/quest_templates.yml with the mods' records. item reports whether
// an item name exists, or is nil to skip the check.
func LoadTemplates(set *mod.Set, item func(string) bool) ([]Template, *mod.Overlay, error) {
	o, err := set.Overlay(mod.QuestTemplateKind, func(string) bool { return false })
	if err != nil {
		return nil, nil, err
	}
	var out []Template
	var errs []error
	for _, r := range o.Records {
		t, err := ParseTemplate(r.Name, r.Mod, r.Fields, item)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: quest templates %s: %w", r.Where, r.Name, err))
			continue
		}
		out = append(out, t)
	}
	if len(out) > MaxTemplates {
		errs = append(errs, fmt.Errorf("quest templates: %d records, more than %d", len(out), MaxTemplates))
	}
	return out, o, errors.Join(errs...)
}
