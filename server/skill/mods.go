package skill

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// Mods' skills and skill trees (proposal 0002, review decision 20).
// server/gamedata reads data/skills.yml and data/skill_trees.yml, and
// puts the result into a copy of core's database and trees with these
// functions. mods/README.md lists the fields.

// Limits of the fields of data/skills.yml.
const (
	// MaxModRange is the longest range and splash a mod's skill may have,
	// in cells.
	MaxModRange = 30
	// MaxModMs is the longest cast time, delay or cooldown, in
	// milliseconds: one hour.
	MaxModMs = 3_600_000
	// MaxModCost is the most HP or SP a skill may cost.
	MaxModCost = 100_000
	// MaxModZeny is the most zeny a skill may cost.
	MaxModZeny = 1_000_000_000
	// MaxModHits is the most hits of hit_count, either sign.
	MaxModHits = 30
	// MaxModKnockback is the most cells of knockback.
	MaxModKnockback = 20
	// MaxRequiredItems is the most items a skill may need, as skill_db's
	// ItemCost.
	MaxRequiredItems = 10
)

// SkillFields are the fields of a record of data/skills.yml, besides
// name.
var SkillFields = []string{
	"display_name", "icon", "max_level", "type", "target", "element", "range", "cast_time", "fixed_cast_time",
	"after_cast_delay", "cooldown", "requirements", "script", "hit", "hit_count", "damage_flags", "splash", "knockback",
	"effects",
}

// EffectMoments are the keys of a skill's effects field, as the client's
// skill effects table has them (decision 0033).
var EffectMoments = []string{"caster", "effect", "hit"}

// RequirementFields are the fields of requirements in data/skills.yml.
var RequirementFields = []string{"sp", "hp", "sp_rate", "hp_rate", "zeny", "spirit_spheres", "weapons", "items", "companion"}

// Names finds the names that data/skills.yml writes.
type Names interface {
	// Skill finds a skill by name, such as MG_LIGHTNINGBOLT, or a mod's
	// full name.
	Skill(name string) (int, bool)
	// Item finds an item by AegisName or a mod's full name.
	Item(name string) (int, bool)
	// Element finds an element of wa.elements, such as wind.
	Element(name string) (int, bool)
	// WeaponType finds a weapon type of wa.weapon_types, such as dagger.
	WeaponType(name string) (int, bool)
}

// Clone returns a copy of the database. Put changes the copy only. The
// skills are shared until Put replaces them.
func (db *DB) Clone() *DB {
	return &DB{renewal: db.renewal, byID: maps.Clone(db.byID), byName: maps.Clone(db.byName), order: slices.Clone(db.order)}
}

// Put adds a skill, or replaces the skill with its ID in its place.
func (db *DB) Put(d *Def) {
	old := db.byID[d.ID]
	if old == nil {
		db.add(d)
		return
	}
	delete(db.byName, old.Name)
	db.byID[d.ID] = d
	db.byName[d.Name] = d
	if i := slices.Index(db.order, old); i >= 0 {
		db.order[i] = d
	}
}

// Copy returns a copy of a skill whose fields can change without
// changing the original.
func (d *Def) Copy() *Def {
	c := *d
	c.Flags = slices.Clone(d.Flags)
	c.Requires.Status = slices.Clone(d.Requires.Status)
	c.Requires.Equipment = slices.Clone(d.Requires.Equipment)
	if d.Unit != nil {
		u := *d.Unit
		u.Flags = slices.Clone(d.Unit.Flags)
		c.Unit = &u
	}
	return &c
}

// NewModDef is a new skill of a mod, with skill_db's defaults: no type,
// passive, neutral, no status, and casts that a hit cancels. hit_count
// is 1, so a damage skill hits once.
func NewModDef(id int, name, modID string) *Def {
	d := &Def{ID: id, Name: name, Mod: modID, Status: -1, CastCancel: true, MaxLevel: 1}
	d.HitCount = fill(1)
	return d
}

// fill is a value that is the same at every level.
func fill(n int) Levels {
	var l Levels
	for i := range l {
		l[i] = n
	}
	return l
}

// skillTypes are the values of type.
var skillTypes = map[string]int{"none": 0, "weapon": battle.TypeWeapon, "magic": battle.TypeMagic, "misc": battle.TypeMisc}

// targets are the values of target.
var targets = map[string]Target{
	"passive": TargetPassive, "enemy": TargetEnemy, "ground": TargetGround, "self": TargetSelf, "support": TargetSupport,
}

// hitTypes are the values of hit, as e_damage_type.
var hitTypes = map[string]int{"normal": HitNormal, "single": HitSingle, "multiple": HitMulti}

// specialElements are the elements of skills that the ten elements do not
// name: the weapon's, the endowed one, and a random one.
var specialElements = map[string]int{"weapon": battle.ElementWeapon, "endowed": battle.ElementEndowed, "random": battle.ElementRandom}

// damageFlags are the values of damage_flags, from skill_db's
// DamageFlags in snake case.
var damageFlags = map[string]battle.NK{
	"no_damage": battle.NKNoDamage, "splash": battle.NKSplash, "splash_split": battle.NKSplashSplit,
	"ignore_attack_card": battle.NKIgnoreAtkCard, "ignore_element": battle.NKIgnoreElement,
	"ignore_defense": battle.NKIgnoreDefense, "ignore_flee": battle.NKIgnoreFlee,
	"ignore_defense_card": battle.NKIgnoreDefCard, "critical": battle.NKCritical,
	"ignore_long_card": battle.NKIgnoreLongCard, "simple_defense": battle.NKSimpleDefense,
}

// Apply sets the fields of a record of data/skills.yml on a skill, as
// YAML decodes them. script is left to the caller, which knows the mod
// that set it. Errors name the field.
func Apply(d *Def, fields map[string]any, names Names) error {
	var errs []error
	bad := func(key string, err error) { errs = append(errs, fmt.Errorf("%s: %w", key, err)) }
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		v := fields[key]
		switch key {
		case "script":
		case "display_name":
			s, ok := v.(string)
			if !ok || len(s) > 255 {
				bad(key, errors.New("want text of at most 255 bytes"))
				continue
			}
			d.DisplayName = s
		case "icon":
			s, ok := v.(string)
			if ok && mod.IsArtName(s) {
				if err := mod.CheckArtName(s, "texture"); err != nil {
					bad(key, err)
					continue
				}
				d.Art.Icon = s
				continue
			}
			id, found := names.Skill(s)
			if !ok || !found {
				bad(key, fmt.Errorf("want the name of a skill, such as MG_LIGHTNINGBOLT, and %v is none", v))
				continue
			}
			d.Icon = id
		case "effects":
			m, ok := v.(map[string]any)
			if !ok {
				bad(key, fmt.Errorf("want a table with %s, each a list of effect names", strings.Join(EffectMoments, ", ")))
				continue
			}
			effects := map[string][]string{}
			for _, k := range slices.Sorted(maps.Keys(m)) {
				if !slices.Contains(EffectMoments, k) {
					bad(key, fmt.Errorf("%q is not a key; the keys are %s", k, strings.Join(EffectMoments, ", ")))
					continue
				}
				list, err := mod.ArtList(m[k], "effect")
				if err != nil {
					bad(key, fmt.Errorf("%s: %w", k, err))
					continue
				}
				effects[k] = list
			}
			d.Art.Effects = effects
		case "max_level":
			n, ok := v.(int)
			if !ok || n < 1 || n > MaxLevel {
				bad(key, fmt.Errorf("want 1 to %d", MaxLevel))
				continue
			}
			d.MaxLevel = n
		case "type":
			n, ok := enum(v, skillTypes)
			if !ok {
				bad(key, errors.New("want weapon, magic, misc or none"))
				continue
			}
			d.Type = n
		case "target":
			s, _ := v.(string)
			t, ok := targets[s]
			if !ok {
				bad(key, errors.New("want passive, enemy, ground, self or support"))
				continue
			}
			d.Target = t
		case "element":
			l, err := levelsOf(v, func(x any) (int, error) {
				s, _ := x.(string)
				if n, ok := specialElements[s]; ok {
					return n, nil
				}
				if n, ok := names.Element(s); ok && n >= 0 && n < 10 {
					return n, nil
				}
				return 0, fmt.Errorf("%v is not an element: want a name of wa.elements, such as wind, or weapon, endowed or random", x)
			})
			if err != nil {
				bad(key, err)
				continue
			}
			d.Element = l
		case "range", "splash":
			low := 0
			if key == "splash" {
				// -1 is the whole screen, as skill_db's SplashArea.
				low = -1
			}
			l, err := levelsOf(v, between(low, MaxModRange))
			if err != nil {
				bad(key, err)
				continue
			}
			if key == "range" {
				d.Range = l
			} else {
				d.Splash = l
			}
		case "cast_time", "fixed_cast_time", "after_cast_delay", "cooldown":
			low := 0
			if key == "fixed_cast_time" {
				// A negative fixed cast time is a percent of the cast
				// time in renewal, as skill_db's FixedCastTime.
				low = -100
			}
			l, err := levelsOf(v, between(low, MaxModMs))
			if err != nil {
				bad(key, err)
				continue
			}
			switch key {
			case "cast_time":
				d.CastTime = l
			case "fixed_cast_time":
				d.FixedCastTime = l
				d.fixedCastSet = true
			case "after_cast_delay":
				d.AfterCastActDelay = l
			default:
				d.Cooldown = l
			}
		case "hit":
			n, ok := enum(v, hitTypes)
			if !ok {
				bad(key, errors.New("want normal, single or multiple"))
				continue
			}
			d.Hit = n
		case "hit_count":
			l, err := levelsOf(v, between(-MaxModHits, MaxModHits))
			if err != nil {
				bad(key, err)
				continue
			}
			d.HitCount = l
		case "knockback":
			l, err := levelsOf(v, between(0, MaxModKnockback))
			if err != nil {
				bad(key, err)
				continue
			}
			d.Knockback = l
		case "damage_flags":
			list, ok := v.([]any)
			if !ok {
				bad(key, errors.New("want a list, such as [ignore_flee]"))
				continue
			}
			var nk battle.NK
			for _, x := range list {
				s, _ := x.(string)
				f, ok := damageFlags[s]
				if !ok {
					bad(key, fmt.Errorf("%v is not a flag; the flags are %s", x, strings.Join(slices.Sorted(maps.Keys(damageFlags)), ", ")))
					continue
				}
				nk |= f
			}
			d.NK = nk
		case "requirements":
			if err := applyRequirements(&d.Requires, v, names); err != nil {
				bad(key, err)
			}
		default:
			bad(key, fmt.Errorf("not a field; the fields are name, %s", strings.Join(SkillFields, ", ")))
		}
	}
	return errors.Join(errs...)
}

// applyRequirements sets the fields of requirements.
func applyRequirements(q *Requires, v any, names Names) error {
	m, ok := v.(map[string]any)
	if !ok {
		return errors.New("want a mapping, such as { sp: 10 }")
	}
	var errs []error
	bad := func(key string, err error) { errs = append(errs, fmt.Errorf("%s: %w", key, err)) }
	for _, key := range slices.Sorted(maps.Keys(m)) {
		v := m[key]
		switch key {
		case "sp", "hp":
			l, err := levelsOf(v, between(0, MaxModCost))
			if err != nil {
				bad(key, err)
				continue
			}
			if key == "sp" {
				q.SP = l
			} else {
				q.HP = l
			}
		case "sp_rate", "hp_rate":
			// A positive rate is a percent of the current value, and a
			// negative one of the maximum, as skill_db's rate costs.
			l, err := levelsOf(v, between(-100, 100))
			if err != nil {
				bad(key, err)
				continue
			}
			if key == "sp_rate" {
				q.SPRate = l
			} else {
				q.HPRate = l
			}
		case "zeny":
			l, err := levelsOf(v, between(0, MaxModZeny))
			if err != nil {
				bad(key, err)
				continue
			}
			q.Zeny = l
		case "spirit_spheres":
			l, err := levelsOf(v, between(0, 15))
			if err != nil {
				bad(key, err)
				continue
			}
			q.SpiritSphere = l
		case "weapons":
			list, ok := v.([]any)
			if !ok {
				bad(key, errors.New("want a list of weapon types, such as [dagger, one_handed_sword]; an empty list allows any"))
				continue
			}
			bits := 0
			for _, x := range list {
				s, _ := x.(string)
				n, ok := names.WeaponType(s)
				if !ok || n < 0 || n >= 31 {
					bad(key, fmt.Errorf("%v is not a weapon type of wa.weapon_types", x))
					continue
				}
				bits |= 1 << n
			}
			q.Weapon = bits
		case "items":
			list, ok := v.([]any)
			if !ok || len(list) > MaxRequiredItems {
				bad(key, fmt.Errorf("want a list of at most %d { item, amount }", MaxRequiredItems))
				continue
			}
			q.Items = [10][2]int{}
			q.ItemLevelDependent = false
			for i, x := range list {
				e, ok := x.(map[string]any)
				name, _ := e["item"].(string)
				amount, isInt := e["amount"].(int)
				if _, has := e["amount"]; !has {
					amount, isInt = 1, true
				}
				id, found := names.Item(name)
				if !ok || len(e) > 2 || !found || !isInt || amount < 1 || amount > 30000 {
					bad(key, fmt.Errorf("entry %d: want { item: Red_Gemstone, amount: 1 } with an item that exists and an amount of 1 to 30000", i+1))
					continue
				}
				q.Items[i] = [2]int{id, amount}
			}
		case "companion":
			// server/gamedata checks the name once the companions load.
			s, ok := v.(string)
			if !ok || s == "" {
				bad(key, errors.New("want a companion's name, such as falcon"))
				continue
			}
			q.Companion = s
		default:
			bad(key, fmt.Errorf("not a requirement; the requirements are %s", strings.Join(RequirementFields, ", ")))
		}
	}
	return errors.Join(errs...)
}

// enum reads a name of a set.
func enum(v any, set map[string]int) (int, bool) {
	s, ok := v.(string)
	if !ok {
		return 0, false
	}
	n, ok := set[s]
	return n, ok
}

// between checks a whole number in a range.
func between(low, high int) func(any) (int, error) {
	return func(x any) (int, error) {
		n, ok := x.(int)
		if !ok || n < low || n > high {
			return 0, fmt.Errorf("%v is not a whole number from %d to %d", x, low, high)
		}
		return n, nil
	}
}

// levelsOf reads a value of data/skills.yml: one value for every level,
// or a list with one value per level from level 1. Levels past the end
// of a list take its last value.
func levelsOf(v any, one func(any) (int, error)) (Levels, error) {
	var l Levels
	list, isList := v.([]any)
	if !isList {
		n, err := one(v)
		if err != nil {
			return l, err
		}
		return fill(n), nil
	}
	if len(list) == 0 || len(list) > MaxLevel {
		return l, fmt.Errorf("a list has 1 to %d values, one per level", MaxLevel)
	}
	for i := range l {
		x := list[min(i, len(list)-1)]
		n, err := one(x)
		if err != nil {
			return l, fmt.Errorf("level %d: %w", min(i, len(list)-1)+1, err)
		}
		l[i] = n
	}
	return l, nil
}

// Clone returns a copy of the trees. Set changes the copy only.
func (t *Trees) Clone() *Trees {
	out := &Trees{byJob: make(map[int]*Tree, len(t.byJob))}
	for job, tree := range t.byJob {
		c := &Tree{Job: job, bySkill: make(map[int]*TreeSkill, len(tree.Skills))}
		for _, s := range tree.Skills {
			e := *s
			e.Requires = slices.Clone(s.Requires)
			c.Skills = append(c.Skills, &e)
			c.bySkill[e.Skill] = &e
		}
		out.byJob[job] = c
	}
	return out
}

// Jobs returns the jobs that have a tree, in order.
func (t *Trees) Jobs() []int {
	out := slices.Collect(maps.Keys(t.byJob))
	sort.Ints(out)
	return out
}

// Inheritors returns the jobs whose trees take a job's tree through
// skill_tree.yml's Inherit, directly or through other jobs, in order.
func (d *TreeData) Inheritors(job int) []int {
	from := map[int][]int{}
	for _, tr := range d.Trees {
		for _, p := range tr.Inherit {
			from[p] = append(from[p], tr.Job)
		}
	}
	seen := map[int]bool{job: true}
	var out []int
	queue := []int{job}
	for len(queue) > 0 {
		j := queue[0]
		queue = queue[1:]
		for _, c := range from[j] {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
				queue = append(queue, c)
			}
		}
	}
	sort.Ints(out)
	return out
}

// SetInherited sets a job's tree entry, as Set, and the entry of every
// job in inheritors whose tree has no entry for the skill, or one equal
// to the job's entry before the change: a copy that Inherit made. A job
// that lists the skill itself keeps its own entry, as in rAthena.
func (t *Trees) SetInherited(job int, s TreeSkill, inheritors []int) {
	var old *TreeSkill
	if e := t.Job(job).Skill(s.Skill); e != nil {
		c := *e
		old = &c
	}
	t.Set(job, s)
	for _, j := range inheritors {
		e := t.Job(j).Skill(s.Skill)
		if e == nil || old != nil && sameEntry(e, old) {
			c := s
			c.Requires = slices.Clone(s.Requires)
			t.Set(j, c)
		}
	}
}

func sameEntry(a, b *TreeSkill) bool {
	return a.Skill == b.Skill && a.MaxLevel == b.MaxLevel && a.BaseLevel == b.BaseLevel && a.JobLevel == b.JobLevel &&
		a.Exclude == b.Exclude && slices.Equal(a.Requires, b.Requires)
}
