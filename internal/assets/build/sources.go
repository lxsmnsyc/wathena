package build

import (
	"fmt"
	"path"
	"strings"

	clientdata "github.com/lxsmnsyc/wathena/client/data"
	"github.com/lxsmnsyc/wathena/internal/assets/grfimport"
	"github.com/lxsmnsyc/wathena/internal/assets/placeholder"
	"github.com/lxsmnsyc/wathena/internal/assets/texture"
	"github.com/lxsmnsyc/wathena/internal/assets/wpal"
	"github.com/lxsmnsyc/wathena/internal/grf"
)

// Converter versions. Changing a converter's output means raising its
// version, so cached outputs from the old converter are not reused.
const (
	spriteConverter      = "grf-sprite-1"
	paletteConverter     = "grf-palette-1"
	textureConverter     = "grf-texture-1"
	placeholderConverter = "placeholder-1"
)

// Placeholder is the built-in placeholder pack.
type Placeholder struct{}

func (Placeholder) Name() string { return "placeholder" }

func (Placeholder) Items() ([]Item, error) {
	return []Item{{
		Name:     "placeholder/sprite",
		Ext:      "wspr",
		CacheKey: placeholderConverter + "|sprite",
		Produce:  func() ([]byte, error) { return placeholder.Sprite().Encode() },
	}}, nil
}

// GRF imports from a game client folder.
type GRF struct {
	Dir    string
	client *grf.Client
}

func (g *GRF) Name() string { return "grf:" + g.Dir }

// Close closes the client's GRF files.
func (g *GRF) Close() error {
	if g.client == nil {
		return nil
	}
	return g.client.Close()
}

func (g *GRF) Items() ([]Item, error) {
	c, err := grf.OpenClient(g.Dir)
	if err != nil {
		return nil, err
	}
	g.client = c
	jobs, err := clientdata.LoadJobs()
	if err != nil {
		return nil, err
	}
	tables, err := grfimport.LoadTables(c, jobs)
	if err != nil {
		return nil, err
	}

	// Sprites come in spr and act pairs with the same path and names.
	type pair struct {
		spr, act grf.Source
		names    []string
	}
	pairs := map[string]*pair{}
	var items []Item
	files := c.Files()
	paths := make([]string, len(files))
	for i, s := range files {
		paths[i] = s.Entry.Name
	}
	textures := grfimport.TextureNames(paths)
	for _, s := range files {
		if name, ok := textures[s.Entry.Name]; ok {
			f, _ := texture.FormatOf(path.Ext(s.Entry.Key))
			items = append(items, Item{
				Name:     name,
				Ext:      "webp",
				CacheKey: fmt.Sprintf("%s|%d|%s|%s", textureConverter, texture.Level, sourceKey(s), name),
				Produce: func() ([]byte, error) {
					b, err := c.ReadFile(s)
					if err != nil {
						return nil, err
					}
					return texture.Convert(f, b)
				},
			})
			continue
		}
		if names := tables.PaletteNames(s.Entry.Name); len(names) > 0 {
			produce := func() ([]byte, error) {
				b, err := c.ReadFile(s)
				if err != nil {
					return nil, err
				}
				p, err := wpal.FromRO(b)
				if err != nil {
					return nil, err
				}
				return p.Encode(), nil
			}
			for _, name := range names {
				items = append(items, Item{Name: name, Ext: "wpal", CacheKey: paletteConverter + "|" + sourceKey(s) + "|" + name, Produce: produce})
			}
			continue
		}
		names := tables.SpriteNames(s.Entry.Name)
		if len(names) == 0 {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(s.Entry.Key, ".spr"), ".act")
		p := pairs[base]
		if p == nil {
			p = &pair{names: names}
			pairs[base] = p
		}
		if strings.HasSuffix(s.Entry.Key, ".spr") {
			p.spr = s
		} else {
			p.act = s
		}
	}
	for _, p := range pairs {
		if p.spr.Entry.Key == "" || p.act.Entry.Key == "" {
			continue
		}
		produce := func() ([]byte, error) {
			sprData, err := c.ReadFile(p.spr)
			if err != nil {
				return nil, err
			}
			actData, err := c.ReadFile(p.act)
			if err != nil {
				return nil, err
			}
			s, err := grfimport.ConvertSprite(sprData, actData, grfimport.EventName)
			if err != nil {
				return nil, err
			}
			return s.Encode()
		}
		key := spriteConverter + "|" + sourceKey(p.spr) + "|" + sourceKey(p.act)
		for _, name := range p.names {
			items = append(items, Item{Name: name, Ext: "wspr", CacheKey: key + "|" + name, Produce: produce})
		}
	}
	return items, nil
}

// sourceKey identifies a GRF entry's content without reading it: which
// archive, where it is stored, and its sizes.
func sourceKey(s grf.Source) string {
	return fmt.Sprintf("%s:%s:%d:%d:%d", s.Archive, s.Entry.Key, s.Entry.Offset, s.Entry.CompressedSize, s.Entry.Size)
}
