package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/server/fields"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// worldConfig is the part of waserver.yml that a new world's layout
// reads: the mods, which give the town templates, dungeon styles,
// naming styles, chat filter and home maps, and procedural.layout.
// Other keys are left alone.
type worldConfig struct {
	Mods struct {
		Dir     string              `yaml:"dir"`
		Ruleset string              `yaml:"ruleset"`
		Strict  bool                `yaml:"strict"`
		Approve map[string][]string `yaml:"approve"`
		Disable []string            `yaml:"disable"`
	} `yaml:"mods"`
	Procedural struct {
		FieldSize int            `yaml:"field_size"`
		Layout    layout.Options `yaml:"layout"`
	} `yaml:"procedural"`
}

// readWorldConfig reads waserver.yml, or nothing for "".
func readWorldConfig(path string) (worldConfig, error) {
	var c worldConfig
	if path == "" {
		return c, nil
	}
	b, err := os.ReadFile(path) // #nosec G304 -- the file the operator named.
	if err != nil {
		return c, fmt.Errorf("--config: %w", err)
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return c, fmt.Errorf("--config %s: %w", path, err)
	}
	return c, nil
}

// worldLayout makes a procedural world's layout from the public branch,
// with the mods and procedural.layout of the config file, or core's
// records and the defaults without one (decision 0041).
func worldLayout(configPath, ruleset string, publicKey []byte, version int) (*layout.Layout, error) {
	g, ok := gen.Lookup(version)
	if !ok || !g.Makes(gen.KindTown) {
		return nil, fmt.Errorf("map generator %d makes no towns; a world layout needs generator 3 or later", version)
	}
	c, err := readWorldConfig(configPath)
	if err != nil {
		return nil, err
	}
	var found []*mod.Mod
	if c.Mods.Dir != "" {
		if found, err = mod.Discover(c.Mods.Dir); err != nil {
			return nil, fmt.Errorf("mods.dir: %w", err)
		}
	}
	core, err := fields.LayoutCore()
	if err != nil {
		return nil, err
	}
	set, err := mod.Resolve(found, mod.Config{Core: core, Base: ruleset, Ruleset: c.Mods.Ruleset, Approve: c.Mods.Approve,
		Disable: c.Mods.Disable, Strict: c.Mods.Strict})
	if err != nil {
		return nil, fmt.Errorf("mods: %w", err)
	}
	recs, err := fields.LoadLayoutRecords(set, g)
	if err != nil {
		return nil, fmt.Errorf("mods: %w", err)
	}
	if len(publicKey) != 32 {
		return nil, errors.New("the world has no public node")
	}
	l, _, err := fields.BuildLayout(recs, g, rng.FromKey([32]byte(publicKey)), c.Procedural.Layout, c.Procedural.FieldSize)
	if err != nil {
		return nil, fmt.Errorf("the world layout: %w", err)
	}
	return l, nil
}

// layoutText describes a layout for the operator.
func layoutText(l *layout.Layout) string {
	var fieldCount, floors int
	var towns, dungeons []string
	for _, m := range l.Maps {
		switch m.Kind {
		case gen.KindField:
			fieldCount++
		case gen.KindTown:
			towns = append(towns, fmt.Sprintf("%s (%s)", m.Name, gen.TownID(m.Town)))
		case gen.KindDungeon:
			floors++
			if m.Floor == 1 {
				dungeons = append(dungeons, m.Dungeon)
			}
		}
	}
	homeName := l.Home
	if h := l.Map(l.Home); h != nil {
		homeName = h.Name + " (" + h.ID + ")"
	}
	return fmt.Sprintf("the layout: fields %d, towns %d, dungeons %d with %d floors, links %d\n"+
		"towns: %s\ndungeons: %s\nhome area: %s\n",
		fieldCount, len(towns), len(dungeons), floors, len(l.Links), strings.Join(towns, ", "), strings.Join(dungeons, ", "), homeName)
}
