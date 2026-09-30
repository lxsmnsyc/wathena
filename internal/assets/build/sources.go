package build

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"sync"

	"golang.org/x/text/unicode/norm"

	clientdata "github.com/lxsmnsyc/wathena/client/data"
	"github.com/lxsmnsyc/wathena/internal/assets/effect"
	"github.com/lxsmnsyc/wathena/internal/assets/grfimport"
	"github.com/lxsmnsyc/wathena/internal/assets/mapimport"
	"github.com/lxsmnsyc/wathena/internal/assets/model"
	"github.com/lxsmnsyc/wathena/internal/assets/placeholder"
	"github.com/lxsmnsyc/wathena/internal/assets/sound"
	"github.com/lxsmnsyc/wathena/internal/assets/texture"
	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
	"github.com/lxsmnsyc/wathena/internal/assets/wpal"
	"github.com/lxsmnsyc/wathena/internal/grf"
)

// Converter versions. Changing a converter's output means raising its
// version, so cached outputs from the old converter are not reused.
const (
	spriteConverter      = "grf-sprite-1"
	paletteConverter     = "grf-palette-1"
	textureConverter     = "grf-texture-1"
	soundConverter       = "grf-sound-1"
	modelConverter       = "grf-model-4"
	mapConverter         = "grf-map-4"
	effectConverter      = "grf-effect-2"
	musicConverter       = "grf-music-1"
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
	}, {
		Name:     "placeholder/texture",
		Ext:      "webp",
		CacheKey: fmt.Sprintf("%s|texture|%d", placeholderConverter, texture.Level),
		Produce:  func() ([]byte, error) { return texture.Encode(placeholder.Texture()) },
	}}, nil
}

// GRF imports from a game client folder.
type GRF struct {
	Dir    string
	client *grf.Client
	root   *os.Root
}

func (g *GRF) Name() string { return "grf:" + g.Dir }

// Close closes the client's GRF files.
func (g *GRF) Close() error {
	if g.root != nil {
		_ = g.root.Close()
	}
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
	textureName := grfimport.TextureResolver(textures)
	textureListKey := hashNames(textures)
	models := grfimport.ModelNames(paths)
	modelName := grfimport.ModelResolver(models)
	names := mapimport.Names{Texture: textureName, Model: modelName, Sound: grfimport.SoundResolver}
	// Map outputs depend on the texture and model names they refer to.
	namesKey := textureListKey + "|" + hashNames(models)
	byKey := make(map[string]grf.Source, len(files))
	for _, s := range files {
		byKey[s.Entry.Key] = s
	}
	for _, s := range files {
		if name, ok := grfimport.SoundName(s.Entry.Name); ok {
			items = append(items, Item{
				Name:     name,
				Ext:      "webm",
				CacheKey: soundConverter + "|" + sourceKey(s) + "|" + name,
				Produce: func() ([]byte, error) {
					b, err := c.ReadFile(s)
					if err != nil {
						return nil, err
					}
					return sound.ConvertWAV(b)
				},
			})
			continue
		}
		if name, ok := models[s.Entry.Name]; ok {
			items = append(items, Item{
				Name: name,
				Ext:  "glb",
				// Texture names are part of the output, so the key covers
				// the converter's view of them through the texture list.
				CacheKey: modelConverter + "|" + sourceKey(s) + "|" + name + "|" + textureListKey,
				Produce: func() ([]byte, error) {
					b, err := c.ReadFile(s)
					if err != nil {
						return nil, err
					}
					m, err := model.Parse(b)
					if err != nil {
						return nil, err
					}
					return model.Convert(m, textureName)
				},
			})
			continue
		}
		if name, folder, ok := grfimport.EffectName(s.Entry.Name); ok {
			items = append(items, Item{
				Name:     name,
				Ext:      "weff",
				CacheKey: effectConverter + "|" + sourceKey(s) + "|" + name + "|" + textureListKey,
				Produce: func() ([]byte, error) {
					b, err := c.ReadFile(s)
					if err != nil {
						return nil, err
					}
					st, err := effect.ParseSTR(b)
					if err != nil {
						return nil, err
					}
					e, err := effect.Bake(st, func(p string) string { return textureName(folder + `\` + p) })
					if err != nil {
						return nil, err
					}
					return e.Encode()
				},
			})
			continue
		}
		if name, ok := grfimport.MapName(s.Entry.Name); ok {
			base := strings.TrimSuffix(s.Entry.Key, ".rsw")
			gat, ok1 := byKey[base+".gat"]
			gnd, ok2 := byKey[base+".gnd"]
			if !ok1 || !ok2 {
				continue
			}
			rsw := s
			lightmap := grfimport.LightmapName(name)
			mapBase := strings.TrimPrefix(name, "map/")
			toolKey := "none"
			if tool, ok := grfimport.EmitterSource(c, mapBase); ok {
				toolKey = sourceKey(tool)
			}
			convert := func() (*wmap.Map, *image.NRGBA, error) {
				var data [3][]byte
				for i, src := range []grf.Source{gat, gnd, rsw} {
					b, err := c.ReadFile(src)
					if err != nil {
						return nil, nil, err
					}
					data[i] = b
				}
				ga, err := mapimport.ParseGAT(data[0])
				if err != nil {
					return nil, nil, err
				}
				gn, err := mapimport.ParseGND(data[1])
				if err != nil {
					return nil, nil, err
				}
				rw, err := mapimport.ParseRSW(data[2])
				if err != nil {
					return nil, nil, err
				}
				emitters, err := grfimport.MapEmitters(c, mapBase)
				if err != nil {
					return nil, nil, err
				}
				return mapimport.Convert(ga, gn, rw, names, lightmap, emitters)
			}
			key := mapConverter + "|" + sourceKey(gat) + "|" + sourceKey(gnd) + "|" + sourceKey(rsw) + "|" + toolKey + "|" + namesKey
			items = append(items,
				Item{Name: name, Ext: "wmap", CacheKey: key + "|" + name, Produce: func() ([]byte, error) {
					m, _, err := convert()
					if err != nil {
						return nil, err
					}
					return m.Encode()
				}},
				Item{Name: lightmap, Ext: "webp", CacheKey: fmt.Sprintf("%s|%d|%s", key, texture.Level, lightmap), Produce: func() ([]byte, error) {
					_, atlas, err := convert()
					if err != nil {
						return nil, err
					}
					// The atlas is data, so its pixels are kept exactly:
					// no color key and no bleeding.
					return texture.Encode(atlas)
				}},
			)
			continue
		}
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
	itemItems, err := g.items(c)
	if err != nil {
		return nil, err
	}
	items = append(items, itemItems...)
	music, err := g.music()
	if err != nil {
		return nil, err
	}
	return append(items, music...), nil
}

// items lists item images and dropped-item sprites, named by item ID.
// Many items share one file, such as the icon every card uses, so each
// file converts once however many names use it.
func (g *GRF) items(c *grf.Client) ([]Item, error) {
	textures, sprites, err := grfimport.ItemAssets(c)
	if err != nil {
		return nil, err
	}
	var items []Item
	converted := map[string]func() ([]byte, error){}
	once := func(key string, f func() ([]byte, error)) func() ([]byte, error) {
		if p, ok := converted[key]; ok {
			return p
		}
		p := sync.OnceValues(f)
		converted[key] = p
		return p
	}
	for _, t := range textures {
		f, _ := texture.FormatOf(path.Ext(t.Source.Entry.Key))
		key := fmt.Sprintf("%s|%d|%s", textureConverter, texture.Level, sourceKey(t.Source))
		items = append(items, Item{
			Name:     t.Name,
			Ext:      "webp",
			CacheKey: key + "|" + t.Name,
			Produce: once(key, func() ([]byte, error) {
				b, err := c.ReadFile(t.Source)
				if err != nil {
					return nil, err
				}
				return texture.Convert(f, b)
			}),
		})
	}
	for _, sp := range sprites {
		key := spriteConverter + "|" + sourceKey(sp.Spr) + "|" + sourceKey(sp.Act)
		items = append(items, Item{
			Name:     sp.Name,
			Ext:      "wspr",
			CacheKey: key + "|" + sp.Name,
			Produce: once(key, func() ([]byte, error) {
				sprData, err := c.ReadFile(sp.Spr)
				if err != nil {
					return nil, err
				}
				actData, err := c.ReadFile(sp.Act)
				if err != nil {
					return nil, err
				}
				s, err := grfimport.ConvertSprite(sprData, actData, grfimport.EventName)
				if err != nil {
					return nil, err
				}
				return s.Encode()
			}),
		})
	}
	return items, nil
}

// music lists the MP3 files in the client's BGM folder, such as BGM\01.mp3,
// which become music/01. They are passed through unchanged.
func (g *GRF) music() ([]Item, error) {
	root, err := os.OpenRoot(g.Dir)
	if err != nil {
		return nil, err
	}
	entries, err := fs.ReadDir(root.FS(), "BGM")
	if err != nil {
		_ = root.Close()
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	g.root = root
	var items []Item
	for _, e := range entries {
		base, ok := strings.CutSuffix(strings.ToLower(e.Name()), ".mp3")
		info, err := e.Info()
		if !ok || !e.Type().IsRegular() || err != nil {
			continue
		}
		name := norm.NFC.String("music/" + base)
		file := "BGM/" + e.Name()
		items = append(items, Item{
			Name:     name,
			Ext:      "mp3",
			CacheKey: fmt.Sprintf("%s|%s|%d|%d|%s", musicConverter, file, info.Size(), info.ModTime().UnixNano(), name),
			Produce: func() ([]byte, error) {
				b, err := root.ReadFile(file)
				if err != nil {
					return nil, err
				}
				if !sound.IsMP3(b) {
					return nil, fmt.Errorf("%s is not an mp3 file", file)
				}
				return b, nil
			},
		})
	}
	return items, nil
}

// hashNames returns a short hash of a path-to-name map.
func hashNames(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		_, _ = fmt.Fprintf(h, "%s\x00%s\x00", k, m[k])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// sourceKey identifies a GRF entry's content without reading it: which
// archive, where it is stored, and its sizes.
func sourceKey(s grf.Source) string {
	return fmt.Sprintf("%s:%s:%d:%d:%d", s.Archive, s.Entry.Key, s.Entry.Offset, s.Entry.CompressedSize, s.Entry.Size)
}
