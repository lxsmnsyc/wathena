package fields

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/internal/assets"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// TilesetFile is a tileset table in YAML. It maps each tileset's slots
// to asset names in the operator's build:
//
//	tilesets:
//	  grassland:
//	    textures: {grass: texture/my_fields/grass01, water: texture/my_fields/water}
//	    models: {tree: model/my_fields/tree01}
//
// Slots it leaves out show the placeholder pack's art:
// placeholder/ground/<slot> and placeholder/prop/<slot>.
type TilesetFile struct {
	Tilesets map[string]TilesetNames `yaml:"tilesets"`
}

// TilesetNames are one tileset's asset names by slot.
type TilesetNames struct {
	Textures map[string]string `yaml:"textures"`
	Models   map[string]string `yaml:"models"`
}

// Tilesets is what /gen/tilesets.json sends: the asset each of the
// generator's names stands for.
type Tilesets struct {
	// Version is the format version, 1.
	Version int `json:"version"`
	// Aliases maps texture/gen/... and model/gen/... names to assets.
	Aliases map[string]string `json:"aliases"`
}

// LoadTilesets builds the tileset table of a generator from a YAML file,
// or from the placeholders alone when file is "". A tileset or slot the
// generator lacks, or a name that is not an asset name, is an error.
//
// The town templates add their buildings' models to their tilesets'
// slots. A building's placeholder is placeholder/building/<w>x<d>, a
// house of its footprint (decision 0040).
func LoadTilesets(g gen.Generator, file string, templates ...Template) (Tilesets, error) {
	t := Tilesets{Version: 1, Aliases: map[string]string{}}
	g.Tilesets = slices.Clone(g.Tilesets)
	for _, tp := range templates {
		i := slices.IndexFunc(g.Tilesets, func(ts gen.Tileset) bool {
			return ts.Name == tp.Template.Tileset && ts.Kind == gen.KindTown
		})
		if i < 0 {
			continue
		}
		ts := &g.Tilesets[i]
		for _, b := range tp.Template.Buildings {
			if !slices.Contains(ts.Models, b.Model) {
				ts.Models = append(slices.Clip(ts.Models), b.Model)
				t.Aliases[gen.ModelName(ts.Name, b.Model)] = BuildingPlaceholder(b.Width, b.Depth)
			}
		}
	}
	for _, ts := range g.Tilesets {
		for _, s := range ts.Textures {
			t.Aliases[gen.TextureName(ts.Name, s)] = "placeholder/ground/" + s
		}
		for _, s := range ts.Models {
			if _, building := t.Aliases[gen.ModelName(ts.Name, s)]; !building {
				t.Aliases[gen.ModelName(ts.Name, s)] = "placeholder/prop/" + s
			}
		}
	}
	if file == "" {
		return t, nil
	}
	data, err := os.ReadFile(file) // #nosec G304 -- the file the operator named.
	if err != nil {
		return Tilesets{}, fmt.Errorf("tilesets: %w", err)
	}
	var f TilesetFile
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return Tilesets{}, fmt.Errorf("tilesets: %s: %w", file, err)
	}
	var problems []string
	set := func(kind, name, slot, to string, slots []string, alias string) {
		switch {
		case !slices.Contains(slots, slot):
			problems = append(problems, fmt.Sprintf("tileset %s has no %s %q; it has %s", name, kind, slot, strings.Join(slots, ", ")))
		case assets.ValidName(to) != nil:
			problems = append(problems, fmt.Sprintf("tileset %s %s %s: %v", name, kind, slot, assets.ValidName(to)))
		default:
			t.Aliases[alias] = to
		}
	}
	for _, name := range sortedKeys(f.Tilesets) {
		i := slices.IndexFunc(g.Tilesets, func(ts gen.Tileset) bool { return ts.Name == name })
		if i < 0 {
			problems = append(problems, fmt.Sprintf("generator %d has no tileset %q", g.Version, name))
			continue
		}
		ts := g.Tilesets[i]
		names := f.Tilesets[name]
		for _, slot := range sortedKeys(names.Textures) {
			set("texture", name, slot, names.Textures[slot], ts.Textures, gen.TextureName(name, slot))
		}
		for _, slot := range sortedKeys(names.Models) {
			set("model", name, slot, names.Models[slot], ts.Models, gen.ModelName(name, slot))
		}
	}
	if len(problems) > 0 {
		return Tilesets{}, fmt.Errorf("tilesets: %s:\n  %s", file, strings.Join(problems, "\n  "))
	}
	return t, nil
}

// BuildingPlaceholder is the placeholder model of a building's
// footprint, in cells.
func BuildingPlaceholder(w, d int) string {
	return "placeholder/building/" + strconv.Itoa(w) + "x" + strconv.Itoa(d)
}

// JSON returns the table as /gen/tilesets.json sends it.
func (t Tilesets) JSON() []byte {
	b, _ := json.Marshal(t) // A map of strings always encodes.
	return b
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// Warps links the portals of fields that touch: the east portal of
// field (x, y) leads to the west portal of field (x+1, y), and the north
// portal to the south portal of field (x, y+1). A player arrives 3 cells
// inside the other portal, off its touch area. The map graph of phase 7
// step 4 replaces this.
func (s *Set) Warps() map[string][]zone.Warp {
	out := map[string][]zone.Warp{}
	for _, f := range s.fields {
		for _, p := range f.Field.Portals {
			dx, dy := edgeStep(p.Edge)
			to := s.Get(gen.FieldID(f.X+dx, f.Y+dy))
			if to == nil {
				continue
			}
			q := to.Field.Portals[(p.Edge+2)%4]
			ix, iy := edgeStep(q.Edge)
			out[f.ID] = append(out[f.ID], zone.Warp{
				Name: fmt.Sprintf("gen_%s_%d", f.ID, p.Edge),
				X:    p.X, Y: p.Y, XS: 1, YS: 1,
				To: to.ID, ToX: q.X - 3*int(ix), ToY: q.Y - 3*int(iy),
			})
		}
	}
	return out
}

// edgeStep is the field step toward an edge's neighbor.
func edgeStep(edge int) (dx, dy int64) {
	switch edge {
	case gen.EdgeSouth:
		return 0, -1
	case gen.EdgeEast:
		return 1, 0
	case gen.EdgeNorth:
		return 0, 1
	default:
		return -1, 0
	}
}
