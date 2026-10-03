package companion

import (
	"fmt"
	"sync"

	"go.yaml.in/yaml/v3"
)

// coreSkills are the skills core's records need, by name, for Core,
// which runs without the skill database.
var coreSkills = map[string]int{
	"HT_FALCON": 127, "KN_RIDING": 63, "RK_DRAGONTRAINING": 2007, "RA_WUGRIDER": 2241, "NC_MADOLICENCE": 2255,
}

var (
	coreOnce sync.Once
	coreSet  *Set
	coreErr  error
)

// Core returns core's records alone, without bonuses, for a zone that
// runs without mods, such as in tests. The result is shared: do not
// change it.
func Core() *Set {
	coreOnce.Do(func() { coreSet, coreErr = parseCore() })
	if coreErr != nil {
		panic(coreErr)
	}
	return coreSet
}

func parseCore() (*Set, error) {
	s := &Set{}
	names := Names{Skill: func(name string) (int, bool) { id, ok := coreSkills[name]; return id, ok }}
	for _, k := range []Kind{Companions, Mounts} {
		recs, err := CoreRecords(k)
		if err != nil {
			return nil, err
		}
		for _, c := range recs {
			r, err := k.Build(c.Name, "core", c.ID, c.Fields, names)
			if err != nil {
				return nil, fmt.Errorf("companion: core %s %s: %w", k.Name, c.Name, err)
			}
			s.Add(k, r)
		}
	}
	return s, nil
}

// CoreRecord is one record of core's file.
type CoreRecord struct {
	Name   string
	ID     int
	Fields map[string]any
}

// CoreRecords reads core's file of a kind.
func CoreRecords(k Kind) ([]CoreRecord, error) {
	data, err := CoreFiles.ReadFile("core/" + k.File)
	if err != nil {
		return nil, err
	}
	var list []map[string]any
	if err := yaml.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("companion: core %s: %w", k.File, err)
	}
	out := make([]CoreRecord, 0, len(list))
	for _, m := range list {
		name, _ := m["name"].(string)
		id, _ := m["id"].(int)
		delete(m, "name")
		out = append(out, CoreRecord{Name: name, ID: id, Fields: m})
	}
	return out, nil
}
