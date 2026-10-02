package gamedata

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/skill"
)

// Skills and skill trees (proposal 0002, review decision 20): rAthena's
// skill_db.yml and skill_tree.yml from the extracts, with the mods'
// data/skills.yml and data/skill_trees.yml over them. mods/README.md,
// "Skills", lists the fields.

// skillData caches the parsed extracts of each mode. Nothing changes
// them: overlays work on copies.
var skillData sync.Map // reference.Mode -> *skillExtracts

type skillExtracts struct {
	skills *skill.Data
	trees  *skill.TreeData
	once   sync.Once
	err    error
}

// LoadSkillData reads a mode's skills.json and skill_trees.json. The
// result is shared: do not change it.
func LoadSkillData(mode reference.Mode) (*skill.Data, *skill.TreeData, error) {
	v, _ := skillData.LoadOrStore(mode, &skillExtracts{})
	e := v.(*skillExtracts)
	e.once.Do(func() {
		e.skills, e.trees = &skill.Data{}, &skill.TreeData{}
		dir := "extracts/" + string(mode) + "/"
		if err := reference.LoadFile(dir+"skills.json", e.skills); err != nil {
			e.err = err
			return
		}
		if err := reference.LoadFile(dir+"skill_trees.json", e.trees); err != nil {
			e.err = err
		}
	})
	return e.skills, e.trees, e.err
}

// LoadSkills builds a mode's skill database and skill trees, without
// mods.
func LoadSkills(mode reference.Mode) (*skill.DB, *skill.Trees, error) {
	d, t, err := LoadSkillData(mode)
	if err != nil {
		return nil, nil, err
	}
	db, err := skill.Load(d)
	if err != nil {
		return nil, nil, err
	}
	return db, skill.LoadTrees(t), nil
}

// skillNames finds the names of data/skills.yml and
// data/skill_trees.yml.
type skillNames struct {
	db     *skill.DB
	items  map[string]uint32
	consts map[string]*constants.Module
}

func (n *skillNames) Skill(name string) (int, bool) {
	if d := n.db.ByName(name); d != nil {
		return d.ID, true
	}
	return 0, false
}

func (n *skillNames) Item(name string) (int, bool) {
	id, ok := n.items[name]
	return int(id), ok
}

func (n *skillNames) constant(module, name string) (int, bool) {
	m := n.consts[module]
	if m == nil {
		return 0, false
	}
	v, ok := m.Values[name]
	return int(v), ok
}

func (n *skillNames) Element(name string) (int, bool) { return n.constant("elements", name) }

func (n *skillNames) WeaponType(name string) (int, bool) { return n.constant("weapon_types", name) }

// job finds a job by its name in wa.jobs, such as mage, or its ID.
func (n *skillNames) job(name string) (int, bool) {
	if id, err := strconv.Atoi(name); err == nil {
		return id, id >= 0
	}
	return n.constant("jobs", name)
}

// modSkills applies the mods' skill overlays and skill trees over core's.
// New skills get IDs in the mod range from ids. items finds required
// items by name. Every script a skill names must be a file of the mod
// that set it.
func modSkills(ctx context.Context, set *mod.Set, mode reference.Mode, ids *mod.IDs, consts *constants.Data,
	items map[string]uint32, scripts []*mod.Scripts) (*skill.DB, *skill.Trees, *mod.Overlay, error) {
	base, treeData, err := LoadSkillData(mode)
	if err != nil {
		return nil, nil, nil, err
	}
	core, err := skill.Load(base)
	if err != nil {
		return nil, nil, nil, err
	}
	o, err := set.Skills(func(name string) bool { return !strings.Contains(name, ":") && core.ByName(name) != nil })
	if err != nil {
		return nil, nil, nil, err
	}
	trees := skill.LoadTrees(treeData)
	entries, err := set.SkillTrees()
	if err != nil {
		return nil, nil, nil, err
	}
	if len(o.Records) == 0 && len(entries) == 0 {
		return core, trees, o, nil
	}
	db := core.Clone()
	names := &skillNames{db: db, items: items}
	if consts != nil {
		names.consts = consts.Modules()
	}
	files := map[string]bool{}
	for _, sc := range scripts {
		for p := range sc.Files {
			files[sc.Mod+":"+p] = true
		}
	}
	// New skills first, so fields such as icon can name any skill.
	defs := make([]*skill.Def, len(o.Records))
	for i, r := range o.Records {
		if d := core.ByName(r.Name); d != nil && !strings.Contains(r.Name, ":") {
			defs[i] = d.Copy()
			continue
		}
		id, err := ids.ID(ctx, mod.SkillIDs, r.Name, r.Mod)
		if err != nil {
			return nil, nil, nil, err
		}
		defs[i] = skill.NewModDef(int(id), r.Name, r.Mod)
		db.Put(defs[i])
	}
	var errs []error
	for i, r := range o.Records {
		d := defs[i]
		if err := skill.Apply(d, r.Fields, names); err != nil {
			for line := range strings.SplitSeq(err.Error(), "\n") {
				errs = append(errs, fmt.Errorf("%s: skills %s: %s", r.Where, r.Name, line))
			}
			continue
		}
		if _, ok := r.Fields["script"]; ok {
			entry, err := mod.ScriptOf(r)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: skills %s: script: %w", r.Where, r.Name, err))
				continue
			}
			if !files[entry] {
				errs = append(errs, fmt.Errorf("%s: skills %s: script %s does not exist", r.Where, r.Name, entry))
				continue
			}
			d.Script = entry
		}
		db.Put(d)
	}
	if len(errs) > 0 {
		return nil, nil, nil, errors.Join(errs...)
	}
	if len(entries) > 0 {
		trees = trees.Clone()
		for _, e := range entries {
			ts, job, err := treeSkill(e, db, names)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: skill_trees: %w", e.Where, err))
				continue
			}
			trees.SetInherited(job, ts, treeData.Inheritors(job))
		}
	}
	if len(errs) > 0 {
		return nil, nil, nil, errors.Join(errs...)
	}
	return db, trees, o, nil
}

// treeSkill resolves one entry of data/skill_trees.yml.
func treeSkill(e mod.TreeEntry, db *skill.DB, names *skillNames) (skill.TreeSkill, int, error) {
	job, ok := names.job(e.Job)
	if !ok {
		return skill.TreeSkill{}, 0, fmt.Errorf("%s is not a job: want a name of wa.jobs, such as mage, or a job ID", e.Job)
	}
	d := db.ByName(e.Skill)
	if d == nil {
		return skill.TreeSkill{}, 0, fmt.Errorf("there is no skill %s", e.Skill)
	}
	if e.MaxLevel > d.MaxLevel {
		return skill.TreeSkill{}, 0, fmt.Errorf("%s: max_level %d is above the skill's max_level %d", e.Skill, e.MaxLevel, d.MaxLevel)
	}
	ts := skill.TreeSkill{Skill: d.ID, Name: d.Name, MaxLevel: e.MaxLevel}
	for _, r := range e.Requires {
		need := db.ByName(r.Skill)
		if need == nil {
			return skill.TreeSkill{}, 0, fmt.Errorf("%s: requires: there is no skill %s", e.Skill, r.Skill)
		}
		if r.Level > need.MaxLevel {
			return skill.TreeSkill{}, 0, fmt.Errorf("%s: requires %s level %d, above its max_level %d", e.Skill, r.Skill, r.Level, need.MaxLevel)
		}
		ts.Requires = append(ts.Requires, [2]int{need.ID, r.Level})
	}
	return ts, job, nil
}
