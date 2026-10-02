package mod

import (
	"errors"
	"fmt"
	"io/fs"
	"strconv"

	"go.yaml.in/yaml/v3"
)

// SkillKind is the skill overlay: data/skills.yml (proposal 0002, review
// decision 20). A record's name is skill_db's name, such as
// MG_LIGHTNINGBOLT, or a mod's full name, such as
// example.chain-lightning:CHAIN_LIGHTNING. server/gamedata reads the
// fields.
var SkillKind = Kind{Name: "skills", File: "data/skills.yml", Fixed: []string{"id"}}

// SkillTreeFile is the skill tree file of a mod.
const SkillTreeFile = "data/skill_trees.yml"

// Skills merges the mods' skill overlays. core reports whether core has a
// skill of a name. A mod that sets script on a skill it did not add, such
// as a core skill, replaces that skill's behavior, and needs the
// permission rules.override.
func (s *Set) Skills(core func(name string) bool) (*Overlay, error) {
	o, err := s.Overlay(SkillKind, core)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, r := range o.Records {
		by, ok := r.SetBy["script"]
		if !ok || by == r.Mod {
			continue
		}
		if m := s.Mod(by); m != nil && !m.Grants.Has("rules.override") {
			errs = append(errs, fmt.Errorf("%s: skills %s: %s sets script on a skill it did not add, which replaces the skill's behavior and needs the permission rules.override", r.Where, r.Name, by))
		}
	}
	return o, errors.Join(errs...)
}

// TreeEntry is one entry of a mod's data/skill_trees.yml, as written:
// names are resolved by server/gamedata.
type TreeEntry struct {
	// Job is a job's name in wa.jobs, such as mage, or its ID.
	Job string
	// Skill is a skill's name.
	Skill    string
	MaxLevel int
	// Requires lists the skills the entry needs, with their levels.
	Requires []TreeRequirement
	// Mod is the mod whose file has the entry, and Where its file and
	// line.
	Mod, Where string
}

// TreeRequirement is one skill a tree entry needs.
type TreeRequirement struct {
	Skill string
	Level int
}

// treeFields are the fields of an entry of data/skill_trees.yml.
var treeFields = map[string]bool{"job": true, "skill": true, "max_level": true, "requires": true}

// SkillTrees reads every loaded mod's data/skill_trees.yml, in load
// order. A later entry for the same job and skill replaces an earlier
// one.
func (s *Set) SkillTrees() ([]TreeEntry, error) {
	var out []TreeEntry
	var errs []error
	for _, m := range s.Mods {
		data, err := m.readFile(SkillTreeFile)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", m.file(SkillTreeFile), err)
		}
		list, err := readTrees(m, data)
		if err != nil {
			errs = append(errs, err)
		}
		out = append(out, list...)
	}
	return out, errors.Join(errs...)
}

// readTrees reads one mod's data/skill_trees.yml.
func readTrees(m *Mod, data []byte) ([]TreeEntry, error) {
	file := m.file(SkillTreeFile)
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	if root.Kind == 0 {
		return nil, nil
	}
	doc := root.Content[0]
	if doc.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s: line %d: want a list of entries, each with a job and a skill", file, doc.Line)
	}
	var out []TreeEntry
	var errs []error
	bad := func(line int, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%s: line %d: %s", file, line, fmt.Sprintf(format, args...)))
	}
	for _, n := range doc.Content {
		if n.Kind != yaml.MappingNode {
			bad(n.Line, "want an entry: a mapping with job, skill and max_level")
			continue
		}
		e := TreeEntry{Mod: m.ID(), Where: fmt.Sprintf("%s: line %d", file, n.Line)}
		ok := true
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i].Value, n.Content[i+1]
			if !treeFields[key] {
				bad(val.Line, "%s is not a field; the fields are job, skill, max_level and requires", key)
				ok = false
				continue
			}
			switch key {
			case "job", "skill":
				if val.Kind != yaml.ScalarNode || val.Value == "" {
					bad(val.Line, "%s must be a name", key)
					ok = false
					continue
				}
				if key == "job" {
					e.Job = val.Value
				} else {
					e.Skill = val.Value
				}
			case "max_level":
				lv, err := strconv.Atoi(val.Value)
				if val.Kind != yaml.ScalarNode || err != nil || lv < 1 {
					bad(val.Line, "max_level must be a whole number from 1")
					ok = false
					continue
				}
				e.MaxLevel = lv
			case "requires":
				if val.Kind != yaml.SequenceNode {
					bad(val.Line, "requires must be a list of { skill, level }")
					ok = false
					continue
				}
				for _, rn := range val.Content {
					var req TreeRequirement
					if err := rn.Decode(&req); err != nil || rn.Kind != yaml.MappingNode || len(rn.Content) != 4 || req.Skill == "" || req.Level < 1 {
						bad(rn.Line, "want { skill: MG_LIGHTNINGBOLT, level: 3 }")
						ok = false
						continue
					}
					e.Requires = append(e.Requires, req)
				}
			}
		}
		if e.Job == "" || e.Skill == "" || e.MaxLevel == 0 {
			bad(n.Line, "an entry needs job, skill and max_level")
			ok = false
		}
		if ok {
			out = append(out, e)
		}
	}
	return out, errors.Join(errs...)
}
