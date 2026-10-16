package layout

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/server/chat"
	"github.com/lxsmnsyc/wathena/server/mapfolder"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// Options are the config's procedural.layout, which `wactl world new`
// reads from waserver.yml, and `waserver --dev --layout` too. Left out,
// a value takes its default.
type Options struct {
	// Grid is the grid of fields, [width, height].
	Grid []int `yaml:"grid"`
	// Towns and Dungeons are how many to place.
	Towns    *int `yaml:"towns"`
	Dungeons *int `yaml:"dungeons"`
	// BandWidth, BandStep and MaxLevel shape the level bands.
	BandWidth int `yaml:"band_width"`
	BandStep  int `yaml:"band_step"`
	MaxLevel  int `yaml:"max_level"`
	// Home picks the home map: a mod's map folder as mod:folder, or
	// "none" for the first town. Empty takes the one map tagged home of
	// the loaded mods, or the first town when none has one.
	Home string `yaml:"home"`
}

// HomeNone is Options.Home's value for a world whose home area is its
// first town, even when a mod has a home map.
const HomeNone = "none"

// Config returns the layout's config, with fieldSize the side of each
// field, 0 for the default.
func (o Options) Config(fieldSize int) (Config, error) {
	c := DefaultConfig()
	if len(o.Grid) != 0 {
		if len(o.Grid) != 2 {
			return Config{}, errors.New("procedural.layout.grid is [width, height]")
		}
		c.Width, c.Height = o.Grid[0], o.Grid[1]
	}
	if o.Towns != nil {
		c.Towns = *o.Towns
	}
	if o.Dungeons != nil {
		c.Dungeons = *o.Dungeons
	}
	if o.BandWidth != 0 {
		c.BandWidth = o.BandWidth
	}
	if o.BandStep != 0 {
		c.BandStep = o.BandStep
	} else {
		c.BandStep = min(c.BandStep, c.BandWidth)
	}
	if o.MaxLevel != 0 {
		c.MaxLevel = o.MaxLevel
	}
	if fieldSize != 0 {
		c.FieldSize = fieldSize
	}
	if err := c.Check(); err != nil {
		return Config{}, fmt.Errorf("procedural.layout: %w", err)
	}
	return c, nil
}

// Home is a mod's map folder tagged home.
type Home struct {
	// Record names it, as mod:folder.
	Record string
	Map    *mapfolder.Map
}

// HomeMap is what a layout records of the home map.
func (h *Home) HomeMap() *HomeMap {
	if h == nil {
		return nil
	}
	return &HomeMap{Folder: h.Map.ID, Record: h.Record, Name: h.Map.Name, NoMemo: h.Map.HasFlag(mapfolder.FlagNoMemo)}
}

// FindHomes reads the map folders of the loaded mods and returns those
// tagged home, by mod load order then folder name. A folder that does
// not read, or whose ID is not its folder's name, is an error.
func FindHomes(set *mod.Set) ([]Home, error) {
	var out []Home
	var errs []error
	for _, m := range set.Mods {
		if m.Builtin {
			continue
		}
		fsys, closeFS, err := m.Open()
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", m.ID(), err))
			continue
		}
		dirs, err := mapfolder.Find(fsys)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", m.ID(), err))
		}
		for _, d := range dirs {
			hm, err := mapfolder.Read(fsys, d)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", m.ID(), err))
				continue
			}
			if folder := strings.TrimPrefix(d, mapfolder.Dir+"/"); hm.ID != folder {
				errs = append(errs, fmt.Errorf("%s: %s: map.yaml's id %q is not the folder's name", m.ID(), d, hm.ID))
				continue
			}
			if hm.HasTag(mapfolder.TagHome) {
				out = append(out, Home{Record: m.ID() + ":" + hm.ID, Map: hm})
			}
		}
		_ = closeFS()
	}
	return out, errors.Join(errs...)
}

// PickHome picks the home map a config names (Options.Home): none for
// "none", the named one, or the only one there is. Two home maps and
// no choice is an error.
func PickHome(homes []Home, want string) (*Home, error) {
	names := make([]string, len(homes))
	for i, h := range homes {
		names[i] = h.Record
	}
	switch {
	case want == HomeNone:
		return nil, nil
	case want != "":
		i := slices.Index(names, want)
		if i < 0 {
			return nil, fmt.Errorf("procedural.layout.home: no loaded mod has the home map %q; the home maps are %s", want, list(names))
		}
		return &homes[i], nil
	case len(homes) == 0:
		return nil, nil
	case len(homes) > 1:
		return nil, fmt.Errorf("several mods have a home map (%s): name one in procedural.layout.home, or none", list(names))
	}
	return &homes[0], nil
}

func list(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// LoadFilter builds the chat filter of the loaded mods' data/
// chat_filter.yml records, as the server does, for town names.
func LoadFilter(set *mod.Set) (*chat.Filter, error) {
	o, err := set.Overlay(mod.ChatFilterKind, func(string) bool { return false })
	if err != nil {
		return nil, err
	}
	var words []chat.Word
	var errs []error
	for _, r := range o.Records {
		word := r.Name
		if r.Mod != mod.Core {
			word = mod.Local(r.Name)
		}
		w, err := chat.BuildWord(word, r.Fields)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: chat filter %s: %w", r.Where, r.Name, err))
			continue
		}
		words = append(words, w)
	}
	return chat.NewFilter(words), errors.Join(errs...)
}

// Blocked turns a chat filter into Inputs.Blocked: a name is blocked
// when the filter refuses it as a character name.
func Blocked(f *chat.Filter) func(string) bool {
	if f == nil {
		return nil
	}
	return func(name string) bool { return f.CheckName(name) != nil }
}
