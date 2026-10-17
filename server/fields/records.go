package fields

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// Dungeon styles and town templates (proposal 0002, review decision 42):
// records of data/dungeon_styles.yml and data/town_templates.yml. Core's
// records are the files of server/fields/core, which the core mod
// carries. Mods change them by name and add their own.

// Style is a dungeon style record.
type Style struct {
	// Name is the full name: a plain name for core's, such as crypt, and
	// mod:name for a mod's.
	Name  string
	Mod   string
	Style gen.DungeonStyle
	// Rotation is the style's rotation, or nil (decision 0045).
	Rotation *layout.RotationRule
}

// Template is a town template record.
type Template struct {
	Name     string
	Mod      string
	Template gen.TownTemplate
}

// Fields of the records.
var (
	styleFields    = []string{"method", "floors", "floor_size", "room_size", "tileset", "level_step", "rotation"}
	templateFields = []string{"size", "gates", "walls", "tileset", "buildings", "decorations"}
	buildingFields = []string{"model", "footprint", "count", "places"}
)

// unknownField returns an error for the first field that is not one of
// allowed, or nil.
func unknownField(fields map[string]any, allowed []string) error {
	for _, k := range slices.Sorted(maps.Keys(fields)) {
		if !slices.Contains(allowed, k) {
			return fmt.Errorf("%q is not a field; the fields are %s", k, strings.Join(allowed, ", "))
		}
	}
	return nil
}

// pair reads a list of two whole numbers.
func pair(v any) (int, int, bool) {
	l, ok := v.([]any)
	if !ok || len(l) != 2 {
		return 0, 0, false
	}
	a, okA := l[0].(int)
	b, okB := l[1].(int)
	return a, b, okA && okB
}

// strs reads a list of strings.
func strs(v any) ([]string, bool) {
	l, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(l))
	for _, e := range l {
		s, ok := e.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// ParseStyleRotation reads a style record's rotation, or nil when it has
// none (decision 0045).
func ParseStyleRotation(fields map[string]any) (*layout.RotationRule, error) {
	v, set := fields["rotation"]
	if !set {
		return nil, nil
	}
	return layout.ParseRotation(v)
}

// ParseStyle reads a dungeon style from its record's fields and checks
// it. room_size may be left out for caves. The rotation is
// ParseStyleRotation's.
func ParseStyle(fields map[string]any) (gen.DungeonStyle, error) {
	if err := unknownField(fields, styleFields); err != nil {
		return gen.DungeonStyle{}, err
	}
	var s gen.DungeonStyle
	var errs []string
	s.Method, _ = fields["method"].(string)
	var ok bool
	if s.Floors, ok = fields["floors"].(int); !ok {
		errs = append(errs, "floors is required: a whole number")
	}
	if s.Width, s.Height, ok = pair(fields["floor_size"]); !ok {
		errs = append(errs, "floor_size is required: [width, height] in cells")
	}
	if v, set := fields["room_size"]; set {
		if s.RoomMin, s.RoomMax, ok = pair(v); !ok {
			errs = append(errs, "room_size is [smallest, largest] in cells")
		}
	} else if s.Method == gen.MethodCave {
		s.RoomMin, s.RoomMax = gen.MinRoomSide, gen.MinRoomSide
	} else {
		errs = append(errs, "room_size is required for rooms: [smallest, largest] in cells")
	}
	s.Tileset, _ = fields["tileset"].(string)
	if s.LevelStep, ok = fields["level_step"].(int); !ok {
		errs = append(errs, "level_step is required: a whole number")
	}
	if len(errs) > 0 {
		return gen.DungeonStyle{}, errors.New(strings.Join(errs, "; "))
	}
	return s, s.Check()
}

// ParseTemplate reads a town template from its record's fields and
// checks it. walls is false and decorations empty when left out.
func ParseTemplate(fields map[string]any) (gen.TownTemplate, error) {
	if err := unknownField(fields, templateFields); err != nil {
		return gen.TownTemplate{}, err
	}
	var t gen.TownTemplate
	var errs []string
	var ok bool
	if t.Size, ok = fields["size"].(int); !ok {
		errs = append(errs, "size is required: a whole number of cells")
	}
	if t.Gates, ok = fields["gates"].(int); !ok {
		errs = append(errs, "gates is required: 2 to 4")
	}
	if v, set := fields["walls"]; set {
		if t.Walls, ok = v.(bool); !ok {
			errs = append(errs, "walls is true or false")
		}
	}
	t.Tileset, _ = fields["tileset"].(string)
	if v, set := fields["decorations"]; set {
		if t.Decorations, ok = strs(v); !ok {
			errs = append(errs, "decorations is a list of props, such as [tree, well]")
		}
	}
	list, ok := fields["buildings"].([]any)
	if !ok {
		errs = append(errs, "buildings is required: a list of {model, footprint, count, places}")
	}
	for i, e := range list {
		b, err := parseBuilding(e)
		if err != nil {
			errs = append(errs, fmt.Sprintf("buildings[%d]: %v", i, err))
			continue
		}
		t.Buildings = append(t.Buildings, b)
	}
	if len(errs) > 0 {
		return gen.TownTemplate{}, errors.New(strings.Join(errs, "; "))
	}
	return t, t.Check()
}

func parseBuilding(v any) (gen.Building, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return gen.Building{}, errors.New("a building is {model, footprint, count, places}")
	}
	if err := unknownField(m, buildingFields); err != nil {
		return gen.Building{}, err
	}
	b := gen.Building{Count: 1}
	if b.Model, ok = m["model"].(string); !ok {
		return gen.Building{}, errors.New("model is required: a model of the tileset, such as smithy")
	}
	if b.Width, b.Depth, ok = pair(m["footprint"]); !ok {
		return gen.Building{}, errors.New("footprint is required: [width, depth] in cells")
	}
	if v, set := m["count"]; set {
		if b.Count, ok = v.(int); !ok {
			return gen.Building{}, errors.New("count is a whole number")
		}
	}
	if v, set := m["places"]; set {
		if b.Places, ok = strs(v); !ok {
			return gen.Building{}, errors.New("places is a list of tags, such as [blacksmith]")
		}
	}
	return b, nil
}

// LoadRecords merges the mods' dungeon styles and town templates over
// core's, and checks them. Records keep the overlay's order: core's in
// its file's order, then the mods' in load order. A building model has
// one footprint in every template, since its art is made to fit it.
func LoadRecords(set *mod.Set) (styles []Style, templates []Template, overlays []*mod.Overlay, err error) {
	none := func(string) bool { return false }
	so, err := set.Overlay(mod.DungeonStyleKind, none)
	if err != nil {
		return nil, nil, nil, err
	}
	to, err := set.Overlay(mod.TownTemplateKind, none)
	if err != nil {
		return nil, nil, nil, err
	}
	var errs []error
	for _, r := range so.Records {
		s, err := ParseStyle(r.Fields)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: dungeon styles %s: %w", r.Where, r.Name, err))
			continue
		}
		rot, err := ParseStyleRotation(r.Fields)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: dungeon styles %s: %w", r.Where, r.Name, err))
			continue
		}
		styles = append(styles, Style{Name: r.Name, Mod: r.Mod, Style: s, Rotation: rot})
	}
	type footprint struct {
		w, d     int
		template string
	}
	models := map[string]footprint{}
	for _, r := range to.Records {
		t, err := ParseTemplate(r.Fields)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: town templates %s: %w", r.Where, r.Name, err))
			continue
		}
		for _, b := range t.Buildings {
			key := t.Tileset + "/" + b.Model
			if f, seen := models[key]; seen && (f.w != b.Width || f.d != b.Depth) {
				errs = append(errs, fmt.Errorf("%s: town templates %s: the model %s is %d × %d here and %d × %d in %s; a model has one footprint",
					r.Where, r.Name, b.Model, b.Width, b.Depth, f.w, f.d, f.template))
				continue
			}
			models[key] = footprint{b.Width, b.Depth, r.Name}
		}
		templates = append(templates, Template{Name: r.Name, Mod: r.Mod, Template: t})
	}
	if len(errs) > 0 {
		return nil, nil, nil, errors.Join(errs...)
	}
	return styles, templates, []*mod.Overlay{so, to}, nil
}

// CheckRecords checks the styles and templates against a generator: a
// style's tileset must be one of its dungeon tilesets, and a template's
// tileset one of its town tilesets, with its decorations among the
// tileset's props and no building named as a prop.
func CheckRecords(g gen.Generator, styles []Style, templates []Template) error {
	var errs []error
	for _, s := range styles {
		if _, ok := g.Tileset(gen.KindDungeon, s.Style.Tileset); !ok {
			errs = append(errs, fmt.Errorf("dungeon styles %s: generator %d has no dungeon tileset %q; it has %s",
				s.Name, g.Version, s.Style.Tileset, strings.Join(tilesetNames(g, gen.KindDungeon), ", ")))
		}
	}
	for _, t := range templates {
		ts, ok := g.Tileset(gen.KindTown, t.Template.Tileset)
		if !ok {
			errs = append(errs, fmt.Errorf("town templates %s: generator %d has no town tileset %q; it has %s",
				t.Name, g.Version, t.Template.Tileset, strings.Join(tilesetNames(g, gen.KindTown), ", ")))
			continue
		}
		for _, d := range t.Template.Decorations {
			if !slices.Contains(ts.Models, d) {
				errs = append(errs, fmt.Errorf("town templates %s: the tileset %s has no prop %q; its props are %s", t.Name, ts.Name, d, strings.Join(ts.Models, ", ")))
			}
		}
		for _, b := range t.Template.Buildings {
			if slices.Contains(ts.Models, b.Model) {
				errs = append(errs, fmt.Errorf("town templates %s: the building model %s is a prop of the tileset %s; name it otherwise", t.Name, b.Model, ts.Name))
			}
		}
	}
	return errors.Join(errs...)
}

// tilesetNames lists a generator's tilesets of a kind.
func tilesetNames(g gen.Generator, kind string) []string {
	var out []string
	for _, t := range g.Tilesets {
		if t.Kind == kind {
			out = append(out, t.Name)
		}
	}
	return out
}
