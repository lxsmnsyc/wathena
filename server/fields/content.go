package fields

import (
	"fmt"
	"slices"

	"github.com/lxsmnsyc/wathena/server/content"
	"github.com/lxsmnsyc/wathena/shared/gen"
)

// AllPlaces returns the map's generated places, then those that content
// and generation layers added (decision 0044).
func (f *Field) AllPlaces() []gen.Place {
	if len(f.Added) == 0 {
		return f.Field.Places
	}
	return append(slices.Clone(f.Field.Places), f.Added...)
}

// Tileset returns the tileset whose props the map's objects name: a
// field's biome, or a town's or a floor's tileset.
func (f *Field) Tileset() (string, error) {
	switch f.Kind {
	case gen.KindTown:
		p, err := gen.DecodeTown(f.Params)
		if err != nil {
			return "", err
		}
		return p.Template.Tileset, nil
	case gen.KindDungeon:
		p, err := gen.DecodeDungeon(f.Params)
		if err != nil {
			return "", err
		}
		return p.Style.Tileset, nil
	}
	return f.BiomeName, nil
}

// AddContent puts a world's content places and objects on the set's
// maps (decision 0044). It returns the maps it does not hold, such as
// the maps of a removed home map's mod, whose content is left out.
func (s *Set) AddContent(w *content.World) ([]string, error) {
	if w == nil {
		return nil, nil
	}
	var missing []string
	miss := func(id string) {
		if !slices.Contains(missing, id) {
			missing = append(missing, id)
		}
	}
	for _, p := range w.Places {
		f := s.Get(p.Map)
		if f == nil {
			miss(p.Map)
			continue
		}
		f.Added = append(f.Added, gen.Place{Tags: slices.Clone(p.Tags), X: p.X, Y: p.Y})
	}
	for _, o := range w.Objects {
		f := s.Get(o.Map)
		if f == nil {
			miss(o.Map)
			continue
		}
		ts, err := f.Tileset()
		if err != nil {
			return nil, fmt.Errorf("the map %s: %w", f.ID, err)
		}
		f.Objects = append(f.Objects, Object{Model: gen.ModelName(ts, o.Model), X: o.X, Y: o.Y, Turn: o.Turn})
	}
	return missing, nil
}
