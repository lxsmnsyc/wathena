package spawns

import (
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/server/spawns/core"
)

// CoreTags are core's spawn tags by monster name, and rAthena's
// densities, from core/data/spawn_tags.yml.
type CoreTags struct {
	Tags      map[string]*Tags
	Densities Densities
}

// LoadCore reads core's spawn tags.
func LoadCore() (*CoreTags, error) {
	b, err := core.Files.ReadFile(core.TagsFile)
	if err != nil {
		return nil, err
	}
	var f struct {
		Density struct {
			Field   int `yaml:"field"`
			Dungeon int `yaml:"dungeon"`
		} `yaml:"density"`
		Monsters []struct {
			Name   string         `yaml:"name"`
			Role   string         `yaml:"role"`
			Biomes map[string]int `yaml:"biomes"`
		} `yaml:"monsters"`
	}
	if err := yaml.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", core.TagsFile, err)
	}
	out := &CoreTags{Tags: make(map[string]*Tags, len(f.Monsters)), Densities: Densities{Field: f.Density.Field, Dungeon: f.Density.Dungeon}}
	for _, m := range f.Monsters {
		r, ok := ParseRole(m.Role)
		if !ok {
			return nil, fmt.Errorf("%s: %s: role %q", core.TagsFile, m.Name, m.Role)
		}
		out.Tags[m.Name] = &Tags{Biomes: m.Biomes, Role: r}
	}
	if out.Densities.Field < 1 || out.Densities.Dungeon < 1 {
		return nil, fmt.Errorf("%s: no density", core.TagsFile)
	}
	return out, nil
}
