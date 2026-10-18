package mod

import (
	"errors"
	"fmt"
	"sort"
)

// MonsterKind is the monster overlay: data/monsters.yml. A record's name
// is the monster's AegisName, such as PORING, or a mod's full name, such
// as example.halloween:PUMPKIN_PORING. server/gamedata reads the fields.
var MonsterKind = Kind{Name: "monsters", File: "data/monsters.yml", Fixed: []string{"id"},
	Economy: []string{"drops", "mvp_drops", "base_exp", "job_exp", "mvp_exp"}}

// EventKind is the world event overlay: data/events.yml. A name without
// a prefix is the mod's own event. server/worldevent reads the fields.
var EventKind = Kind{Name: "events", File: "data/events.yml", LocalNames: true}

// Monsters merges the mods' monster overlays. core reports whether core
// has a monster of an AegisName.
func (s *Set) Monsters(core func(name string) bool) (*Overlay, error) {
	return s.Overlay(MonsterKind, core)
}

// Events merges the mods' world events. Every mod that adds or changes
// an event needs the events.schedule permission.
func (s *Set) Events() (*Overlay, error) {
	o, err := s.Overlay(EventKind, func(string) bool { return false })
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, r := range o.Records {
		mods := map[string]bool{r.Mod: true}
		for _, m := range r.SetBy {
			mods[m] = true
		}
		var ids []string
		for id := range mods {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			m := s.Mod(id)
			if m != nil && !m.Grants.Has("events.schedule") {
				errs = append(errs, fmt.Errorf("%s: events %s: %s needs the permission events.schedule to add or change world events", r.Where, r.Name, id))
			}
		}
	}
	return o, errors.Join(errs...)
}

// ScriptOf returns the entry script that a record's script field names,
// as <mod>:<path>: the path belongs to the mod that set the field. It
// checks the path's form, not that the file exists.
func ScriptOf(r *Record) (string, error) {
	p, ok := r.Fields["script"].(string)
	if !ok {
		return "", errors.New("script must be a path under scripts/, such as scripts/events/boss.lua")
	}
	if err := checkScriptPath(p); err != nil {
		return "", err
	}
	return r.SetBy["script"] + ":" + p, nil
}

// HasFile reports whether a loaded mod has a file, such as a script
// that a record names.
func (s *Set) HasFile(modID, name string) bool {
	m := s.Mod(modID)
	if m == nil {
		return false
	}
	_, err := m.readFile(name)
	return err == nil
}
