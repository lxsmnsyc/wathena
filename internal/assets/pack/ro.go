package pack

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/lxsmnsyc/wathena/internal/assets/effect"
	"github.com/lxsmnsyc/wathena/internal/assets/grfimport"
	"github.com/lxsmnsyc/wathena/internal/assets/mapimport"
	"github.com/lxsmnsyc/wathena/internal/assets/model"
	"github.com/lxsmnsyc/wathena/internal/assets/texture"
	"github.com/lxsmnsyc/wathena/internal/assets/wpal"
	"github.com/lxsmnsyc/wathena/internal/grf"
)

// Loose RO files reuse the GRF importer's parsers and converters. Names
// come from the file's path in the folder, as for packs. References
// inside the files, such as a model's textures, find the folder's own
// files first, and otherwise get the names the GRF importer gives:
//   - A model's or map's texture path p, relative to data\texture, is the
//     folder's texture/<p>, else core's texture/<p>.
//   - A map's model path p, relative to data\model, is model/<p>.
//   - A sound path p, relative to data\wav, is sound/<p>.
//   - An STR effect at effect/<path>.str finds its textures in
//     texture/effect/<folder of path>/, as RO keeps them in
//     data\texture\effect.
// Paths are compared without regard to case, as RO does.

// roFiles adds the sprites, palettes, models, effects and maps.
func (l *lister) roFiles(used map[*file]bool) {
	for _, f := range l.files {
		if used[f] {
			continue
		}
		switch f.ext {
		case ".spr":
			act := l.has(f.base, ".act")
			if act == nil {
				continue
			}
			used[f], used[act] = true, true
			l.roSprite(f, act)
		case ".pal":
			used[f] = true
			l.roPalette(f)
		case ".rsm", ".rsm2":
			used[f] = true
			l.roModel(f)
		case ".str":
			used[f] = true
			l.roEffect(f)
		case ".rsw":
			gnd, gat := l.has(f.base, ".gnd"), l.has(f.base, ".gat")
			if gnd == nil || gat == nil {
				continue
			}
			used[f], used[gnd], used[gat] = true, true, true
			l.roMap(f, gnd, gat)
		}
	}
	for _, f := range l.files {
		if used[f] {
			continue
		}
		switch f.ext {
		case ".spr", ".act":
			used[f] = true
			l.src.reject(f.path, "a sprite needs both its .spr and .act files")
		case ".rsw", ".gnd", ".gat":
			used[f] = true
			l.src.reject(f.path, "a map needs its .rsw, .gnd and .gat files")
		}
	}
}

// lowerNames maps the source's own names, without the prefix and in
// lower case, to the full names. Converters call it once every name is
// known.
func (l *lister) lowerNames() map[string]string {
	l.lowerOnce.Do(func() {
		l.lower = make(map[string]string, len(l.names))
		for n := range l.names {
			l.lower[strings.ToLower(strings.TrimPrefix(n, l.src.Prefix))] = n
		}
	})
	return l.lower
}

// roPath turns an RO path, with backslashes and any case, into a name's
// form: lower case with slashes and no extension.
func roPath(p string) string {
	p = norm.NFC.String(grf.Normalize(p))
	if i := strings.LastIndexByte(p, '.'); i > strings.LastIndexByte(p, '/') {
		p = p[:i]
	}
	return p
}

// resolvers returns the name lookups for models and maps.
func (l *lister) resolvers() mapimport.Names {
	coreTexture := grfimport.TextureResolver(nil)
	coreModel := grfimport.ModelResolver(nil)
	return mapimport.Names{
		Texture: func(p string) string {
			if n, ok := l.lowerNames()["texture/"+roPath(p)]; ok {
				return n
			}
			return coreTexture(p)
		},
		Model: func(p string) string {
			if n, ok := l.lowerNames()["model/"+roPath(p)]; ok {
				return n
			}
			return coreModel(p)
		},
		Sound: func(p string) string {
			if n, ok := l.lowerNames()["sound/"+roPath(p)]; ok {
				return n
			}
			return grfimport.SoundResolver(p)
		},
	}
}

func (l *lister) roSprite(spr, act *file) {
	name, ok := l.name(spr, spr.base, "sprite")
	if !ok {
		return
	}
	eventName := func(ev string) string {
		n := grfimport.EventName(ev)
		if strings.HasPrefix(n, "sound/") {
			if own, ok := l.lowerNames()[n]; ok {
				return own
			}
		}
		return n
	}
	l.add(name, outSprite, roConverter+"|sprite|"+spr.key+"|"+act.key+"|"+l.soundListKey(), func() ([]byte, error) {
		sd, err := l.read(spr, maxSourceBytes)
		if err != nil {
			return nil, err
		}
		ad, err := l.read(act, maxSourceBytes)
		if err != nil {
			return nil, err
		}
		s, err := grfimport.ConvertSprite(sd, ad, eventName)
		if err != nil {
			return nil, err
		}
		return s.Encode()
	})
}

func (l *lister) roPalette(f *file) {
	name, ok := l.name(f, f.base, "palette")
	if !ok {
		return
	}
	l.add(name, outPalette, roConverter+"|palette|"+f.key, func() ([]byte, error) {
		data, err := l.read(f, 4096)
		if err != nil {
			return nil, err
		}
		p, err := wpal.FromRO(data)
		if err != nil {
			return nil, err
		}
		return p.Encode(), nil
	})
}

func (l *lister) roModel(f *file) {
	base := f.base
	if f.ext == ".rsm2" && l.has(f.base, ".rsm") != nil {
		base += "~rsm2"
	}
	name, ok := l.name(f, base, "model")
	if !ok {
		return
	}
	names := l.resolvers()
	l.add(name, outModel, roConverter+"|model|"+f.key+"|"+l.textureListKey(), func() ([]byte, error) {
		data, err := l.read(f, maxSourceBytes)
		if err != nil {
			return nil, err
		}
		m, err := model.Parse(data)
		if err != nil {
			return nil, err
		}
		return model.Convert(m, names.Texture)
	})
}

func (l *lister) roEffect(f *file) {
	name, ok := l.name(f, f.base, "effect")
	if !ok {
		return
	}
	// effect/<folder>/<file> keeps its textures in texture/effect/<folder>.
	folder := ""
	if rest, ok := strings.CutPrefix(f.base, "effect/"); ok {
		if i := strings.LastIndexByte(rest, '/'); i >= 0 {
			folder = rest[:i]
		}
	}
	resolve := l.resolvers().Texture
	l.add(name, outEffect, roConverter+"|effect|"+f.key+"|"+l.textureListKey(), func() ([]byte, error) {
		data, err := l.read(f, maxSourceBytes)
		if err != nil {
			return nil, err
		}
		st, err := effect.ParseSTR(data)
		if err != nil {
			return nil, err
		}
		e, err := effect.Bake(st, func(p string) string {
			if folder == "" {
				return resolve(`effect\` + p)
			}
			return resolve(`effect\` + folder + `\` + p)
		})
		if err != nil {
			return nil, err
		}
		return e.Encode()
	})
}

func (l *lister) roMap(rsw, gnd, gat *file) {
	name, ok := l.name(rsw, rsw.base, "map")
	if !ok {
		return
	}
	lightBase := "lightmap/" + strings.TrimPrefix(rsw.base, "map/")
	lightName, ok := l.name(rsw, lightBase, "texture")
	if !ok {
		return
	}
	names := l.resolvers()
	convert := func() (*mapResult, error) {
		var data [3][]byte
		for i, f := range []*file{gat, gnd, rsw} {
			b, err := l.read(f, maxSourceBytes)
			if err != nil {
				return nil, err
			}
			data[i] = b
		}
		ga, err := mapimport.ParseGAT(data[0])
		if err != nil {
			return nil, err
		}
		gn, err := mapimport.ParseGND(data[1])
		if err != nil {
			return nil, err
		}
		rw, err := mapimport.ParseRSW(data[2])
		if err != nil {
			return nil, err
		}
		m, atlas, err := mapimport.Convert(ga, gn, rw, names, lightName, mapimport.Extras{})
		if err != nil {
			return nil, err
		}
		return &mapResult{m.Encode, atlas}, nil
	}
	key := roConverter + "|map|" + gat.key + "|" + gnd.key + "|" + rsw.key + "|" + l.textureListKey()
	l.add(name, outMap, key, func() ([]byte, error) {
		r, err := convert()
		if err != nil {
			return nil, err
		}
		return r.encode()
	})
	l.add(lightName, outTexture, fmt.Sprintf("%s|%d", key, texture.Level), func() ([]byte, error) {
		r, err := convert()
		if err != nil {
			return nil, err
		}
		// The atlas is data, so its pixels are kept exactly: no color key
		// and no bleeding.
		return texture.Encode(r.atlas)
	})
}

type mapResult struct {
	encode func() ([]byte, error)
	atlas  *image.NRGBA
}

// hashStrings returns a short hash of a sorted list.
func hashStrings(list []string) string {
	h := sha256.New()
	for _, s := range list {
		_, _ = h.Write([]byte(s))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
