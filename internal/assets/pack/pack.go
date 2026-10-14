// Package pack reads art sources from a folder: the pack format for
// original art, and loose RO files, as described in docs/specs/assets-v1.md,
// "Pack format for original art" and "Loose RO files". A file's path
// without its extension is its name. Mods build their art/ folder with
// both kinds at once, under the prefix mod/<mod id>/.
package pack

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"

	"go.yaml.in/yaml/v3"
	"golang.org/x/text/unicode/norm"

	"github.com/lxsmnsyc/wathena/internal/assets"
	"github.com/lxsmnsyc/wathena/internal/assets/build"
	"github.com/lxsmnsyc/wathena/internal/assets/model"
	"github.com/lxsmnsyc/wathena/internal/assets/sound"
	"github.com/lxsmnsyc/wathena/internal/assets/texture"
)

// Formats picks which kinds of files a source reads.
type Formats uint8

const (
	// Authoring is the pack format: PNG sprites with YAML frame files,
	// PNG, JPEG and WebP textures, glTF binary models, WAV sounds, Opus
	// in WebM, MP3 music, and JSON tables.
	Authoring Formats = 1 << iota
	// RO is loose RO files: spr and act sprites, BMP and TGA textures,
	// WAV sounds, pal palettes, RSM models, STR effects, and maps as gat,
	// gnd and rsw.
	RO
	// Both is what a mod's art/ folder may hold.
	Both = Authoring | RO
)

// InfoFile is a pack's description, at its root.
const InfoFile = "pack.yaml"

// Converter versions. Changing a converter's output means raising its
// version, so cached outputs from the old converter are not reused.
const (
	spriteConverter  = "pack-sprite-1"
	textureConverter = "pack-texture-1"
	modelConverter   = "pack-model-1"
	soundConverter   = "pack-sound-1"
	tableConverter   = "pack-table-1"
	roConverter      = "pack-ro-1"
)

// Limits of files that are passed through unchanged.
const (
	maxTableBytes = 16 << 20
	maxMusicBytes = 32 << 20
	// maxSourceBytes is the largest source file read, besides music.
	maxSourceBytes = 64 << 20
)

// Info is pack.yaml.
type Info struct {
	// Name is the pack's name, for messages.
	Name string `yaml:"name"`
	// License names the art's license, such as CC-BY-4.0. It is required.
	License string `yaml:"license"`
	// Contract is the asset contract the pack follows, assets-v1 when
	// left out.
	Contract string `yaml:"contract"`
}

// ParseInfo reads and checks pack.yaml. Unknown fields are errors.
func ParseInfo(data []byte) (*Info, error) {
	var in Info
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&in); err != nil {
		return nil, fmt.Errorf("%s: %w", InfoFile, err)
	}
	switch {
	case strings.TrimSpace(in.License) == "" || len(in.License) > 200:
		return nil, fmt.Errorf("%s: license is required, such as CC-BY-4.0", InfoFile)
	case len(in.Name) > 200:
		return nil, fmt.Errorf("%s: name is at most 200 bytes", InfoFile)
	case in.Contract != "" && in.Contract != assets.Contract:
		return nil, fmt.Errorf("%s: contract %q is not %s", InfoFile, in.Contract, assets.Contract)
	}
	return &in, nil
}

// Source is a folder of art sources. It is a build.Source.
type Source struct {
	Dir string
	// Prefix goes before every name, such as mod/example.lantern/. It is
	// empty for a pack in a server's own build.
	Prefix string
	// Formats are the files it reads.
	Formats Formats
	// Info is pack.yaml, which Items reads when the source reads the
	// pack format.
	Info *Info

	rejected []string
	root     *os.Root
}

// Close closes the folder that Items opened.
func (s *Source) Close() error {
	if s.root == nil {
		return nil
	}
	err := s.root.Close()
	s.root = nil
	return err
}

// Name is the source as wactl assets build names it.
func (s *Source) Name() string {
	switch s.Formats {
	case RO:
		return "ro:" + s.Dir
	case Both:
		return "art:" + s.Dir
	}
	return "pack:" + s.Dir
}

// Rejected lists the files Items left out, with the reason.
func (s *Source) Rejected() []string { return s.rejected }

// file is one source file, by its path relative to the folder.
type file struct {
	path string
	// base is the path without its extension, in NFC.
	base string
	ext  string
	size int64
	key  string
}

// kinds of output, by stored extension.
const (
	outSprite  = "wspr"
	outTexture = "webp"
	outModel   = "glb"
	outSound   = "webm"
	outMusic   = "mp3"
	outTable   = "json"
	outEffect  = "weff"
	outMap     = "wmap"
	outPalette = "wpal"
)

// Items lists every asset the folder provides. Files it cannot use are
// left out and listed by Rejected.
func (s *Source) Items() ([]build.Item, error) {
	s.rejected = nil
	_ = s.Close()
	root, err := os.OpenRoot(s.Dir)
	if err != nil {
		return nil, err
	}
	fsys := root.FS()
	if s.Formats&Authoring != 0 {
		data, err := fs.ReadFile(fsys, InfoFile)
		if err != nil {
			_ = root.Close()
			return nil, fmt.Errorf("%s: a pack needs %s with its license: %w", s.Dir, InfoFile, err)
		}
		if s.Info, err = ParseInfo(data); err != nil {
			_ = root.Close()
			return nil, err
		}
	}
	files, err := s.list(fsys)
	if err != nil {
		_ = root.Close()
		return nil, err
	}
	l := &lister{src: s, fsys: fsys, files: files, byBase: map[string][]*file{}, names: map[string]string{}}
	for _, f := range files {
		l.byBase[f.base] = append(l.byBase[f.base], f)
	}
	items := l.list()
	// The root stays open while the build reads the files, until Close.
	s.root = root
	sort.Strings(s.rejected)
	return items, nil
}

// list walks the folder. Names that start with "." and the pack's
// description are left out. Symbolic links are refused.
func (s *Source) list(fsys fs.FS) ([]*file, error) {
	var out []*file
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == "." {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if p == InfoFile && s.Formats&Authoring != 0 {
			return nil
		}
		if !d.Type().IsRegular() {
			s.reject(p, "is not a regular file")
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		ext := strings.ToLower(path.Ext(p))
		base := norm.NFC.String(strings.TrimSuffix(p, path.Ext(p)))
		out = append(out, &file{path: p, base: base, ext: ext, size: info.Size(),
			key: fmt.Sprintf("%s|%d|%d", p, info.Size(), info.ModTime().UnixNano())})
		return nil
	})
	return out, err
}

func (s *Source) reject(p, why string) {
	s.rejected = append(s.rejected, fmt.Sprintf("%s: %s: %s", s.Name(), p, why))
}

// lister turns files into items.
type lister struct {
	src    *Source
	fsys   fs.FS
	files  []*file
	byBase map[string][]*file
	// names maps each name given so far to the file that has it.
	names map[string]string
	out   []build.Item

	lowerOnce sync.Once
	lower     map[string]string
}

// read reads a source file, up to limit bytes.
func (l *lister) read(f *file, limit int64) ([]byte, error) {
	if f.size > limit {
		return nil, fmt.Errorf("%s is %d bytes, over the limit of %d", f.path, f.size, limit)
	}
	data, err := fs.ReadFile(l.fsys, f.path)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is over the limit of %d bytes", f.path, limit)
	}
	return data, nil
}

// name is the asset name of a path without its extension. ok is false,
// with the file rejected, when the name breaks the rules or its
// namespace does not hold the type.
func (l *lister) name(f *file, base, t string) (string, bool) {
	n := l.src.Prefix + base
	if err := assets.ValidName(n); err != nil {
		why := "is not a valid asset name"
		if l.src.Prefix == "" && strings.ToLower(base) != base {
			why += ": names are in lower case"
		}
		l.src.reject(f.path, why)
		return "", false
	}
	if got := assets.TypeOf(n); got != t {
		l.src.reject(f.path, fmt.Sprintf("a %s must be under a %s folder, such as %s", t, t, exampleFolder[t]))
		return "", false
	}
	if other, ok := l.names[n]; ok {
		l.src.reject(f.path, fmt.Sprintf("has the name %s, which %s has", n, other))
		return "", false
	}
	l.names[n] = f.path
	return n, true
}

var exampleFolder = map[string]string{
	"sprite": "sprite/mob/", "texture": "texture/, icon/ or image/", "model": "model/", "sound": "sound/",
	"music": "music/", "table": "table/", "effect": "effect/", "map": "map/", "palette": "palette/",
}

// add adds an item.
func (l *lister) add(name, ext, key string, produce func() ([]byte, error)) {
	l.out = append(l.out, build.Item{Name: name, Ext: ext, CacheKey: key + "|" + name, Produce: produce})
}

// has reports whether the folder has a file with a base and extension.
func (l *lister) has(base, ext string) *file {
	for _, f := range l.byBase[base] {
		if f.ext == ext {
			return f
		}
	}
	return nil
}

func (l *lister) list() []build.Item {
	formats := l.src.Formats
	used := map[*file]bool{}
	// Textures first, so models, effects and maps can find them, and in
	// order of preference when one name has several formats.
	l.textures(used)
	l.sounds(used)
	for _, f := range l.files {
		if used[f] {
			continue
		}
		switch {
		case formats&Authoring != 0 && f.ext == ".png" && assets.Namespace(l.src.Prefix+f.base) == "sprite":
			used[f] = true
			l.packSprite(f, used)
		case formats&Authoring != 0 && f.ext == ".glb":
			used[f] = true
			l.glb(f)
		case formats&Authoring != 0 && f.ext == ".mp3":
			used[f] = true
			l.music(f)
		case formats&Authoring != 0 && f.ext == ".json":
			used[f] = true
			l.table(f)
		case formats&RO != 0 && (f.ext == ".spr" || f.ext == ".act" || f.ext == ".pal" || f.ext == ".rsm" || f.ext == ".rsm2" ||
			f.ext == ".str" || f.ext == ".rsw" || f.ext == ".gnd" || f.ext == ".gat"):
			// Below, once every texture and sound has its name.
		}
	}
	if formats&RO != 0 {
		l.roFiles(used)
	}
	for _, f := range l.files {
		if !used[f] {
			l.src.reject(f.path, "is not a file this source reads")
		}
	}
	return l.out
}

// textureFormats are the texture inputs in order of preference, as the
// GRF importer orders them. When one name has several, the first gets the
// plain name and the others get "~" and their format appended.
func (l *lister) textureFormats() []string {
	var out []string
	if l.src.Formats&RO != 0 {
		out = append(out, ".bmp", ".tga")
	}
	if l.src.Formats&Authoring != 0 {
		out = append(out, ".png", ".jpg", ".jpeg", ".webp")
	}
	return out
}

func (l *lister) textures(used map[*file]bool) {
	order := l.textureFormats()
	bases := make([]string, 0, len(l.byBase))
	for b := range l.byBase {
		bases = append(bases, b)
	}
	sort.Strings(bases)
	for _, base := range bases {
		if assets.TypeOf(l.src.Prefix+base) != "texture" {
			continue
		}
		first := true
		for _, ext := range order {
			f := l.has(base, ext)
			if f == nil {
				continue
			}
			used[f] = true
			nb := base
			if !first {
				nb += "~" + strings.TrimPrefix(ext, ".")
			}
			first = false
			name, ok := l.name(f, nb, "texture")
			if !ok {
				continue
			}
			format, _ := texture.FormatOf(ext)
			l.add(name, outTexture, fmt.Sprintf("%s|%d|%s", textureConverter, texture.Level, f.key), func() ([]byte, error) {
				data, err := l.read(f, maxSourceBytes)
				if err != nil {
					return nil, err
				}
				return texture.Convert(format, data)
			})
		}
	}
}

func (l *lister) sounds(used map[*file]bool) {
	for _, f := range l.files {
		ns := assets.Namespace(l.src.Prefix + f.base)
		switch {
		case f.ext == ".wav":
			used[f] = true
			name, ok := l.name(f, f.base, "sound")
			if !ok {
				continue
			}
			l.add(name, outSound, soundConverter+"|"+f.key, func() ([]byte, error) {
				data, err := l.read(f, maxSourceBytes)
				if err != nil {
					return nil, err
				}
				return sound.ConvertWAV(data)
			})
		case f.ext == ".webm" && l.src.Formats&Authoring != 0 && (ns == "sound" || ns == "music"):
			used[f] = true
			name, ok := l.name(f, f.base, assets.Types[ns])
			if !ok {
				continue
			}
			limit := int64(sound.MaxBytes)
			if ns == "music" {
				limit = maxMusicBytes
			}
			l.add(name, outSound, soundConverter+"|"+f.key, func() ([]byte, error) {
				data, err := l.read(f, limit)
				if err != nil {
					return nil, err
				}
				if err := sound.CheckWebM(data); err != nil {
					return nil, err
				}
				return data, nil
			})
		case (f.ext == ".ogg" || f.ext == ".opus" || (f.ext == ".mp3" && ns == "sound")) && l.src.Formats&Authoring != 0:
			used[f] = true
			l.src.reject(f.path, "sounds from "+strings.TrimPrefix(f.ext, ".")+" are not converted yet; use WAV, or Opus in WebM")
		}
	}
}

func (l *lister) music(f *file) {
	name, ok := l.name(f, f.base, "music")
	if !ok {
		return
	}
	l.add(name, outMusic, soundConverter+"|"+f.key, func() ([]byte, error) {
		data, err := l.read(f, maxMusicBytes)
		if err != nil {
			return nil, err
		}
		if !sound.IsMP3(data) {
			return nil, fmt.Errorf("%s is not an mp3 file", f.path)
		}
		return data, nil
	})
}

func (l *lister) table(f *file) {
	name, ok := l.name(f, f.base, "table")
	if !ok {
		return
	}
	l.add(name, outTable, tableConverter+"|"+f.key, func() ([]byte, error) {
		data, err := l.read(f, maxTableBytes)
		if err != nil {
			return nil, err
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(data, &obj); err != nil {
			return nil, fmt.Errorf("a table is a JSON object: %w", err)
		}
		return data, nil
	})
}

// packSprite converts a PNG sprite with its optional frame file.
func (l *lister) packSprite(f *file, used map[*file]bool) {
	spec := l.has(f.base, ".yaml")
	if spec == nil {
		spec = l.has(f.base, ".yml")
	}
	key := spriteConverter + "|" + f.key
	if spec != nil {
		used[spec] = true
		key += "|" + spec.key
	}
	name, ok := l.name(f, f.base, "sprite")
	if !ok {
		return
	}
	key += "|" + l.soundListKey()
	l.add(name, outSprite, key, func() ([]byte, error) {
		img, err := l.read(f, maxSourceBytes)
		if err != nil {
			return nil, err
		}
		var sp *SpriteSpec
		if spec != nil {
			data, err := l.read(spec, MaxSpriteYAML)
			if err != nil {
				return nil, err
			}
			if sp, err = ParseSpriteSpec(data); err != nil {
				return nil, fmt.Errorf("%s: %w", spec.path, err)
			}
		}
		s, err := ConvertSprite(img, sp, l.eventName)
		if err != nil {
			return nil, err
		}
		return s.Encode()
	})
}

// eventName maps a sprite event to its stored name: a sound the source
// provides gets the prefix, so a mod's sprite can say sound/effect/x for
// its own sound.
func (l *lister) eventName(ev string) string {
	if strings.Contains(ev, "/") && l.src.Prefix != "" {
		if _, ok := l.names[l.src.Prefix+ev]; ok {
			return l.src.Prefix + ev
		}
	}
	return ev
}

// soundListKey identifies the sounds the source provides, which sprite
// events refer to.
func (l *lister) soundListKey() string {
	var list []string
	for n := range l.names {
		if assets.TypeOf(n) == "sound" {
			list = append(list, n)
		}
	}
	slices.Sort(list)
	return hashStrings(list)
}

// glb checks a model against the profile and points its texture names at
// the source's own textures where it has them.
func (l *lister) glb(f *file) {
	name, ok := l.name(f, f.base, "model")
	if !ok {
		return
	}
	resolve := l.ownName
	l.add(name, outModel, modelConverter+"|"+f.key+"|"+l.textureListKey(), func() ([]byte, error) {
		data, err := l.read(f, model.MaxBytes)
		if err != nil {
			return nil, err
		}
		return model.RenameTextures(data, resolve)
	})
}

// ownName returns the source's own name for a name it provides without
// the prefix, or the name as it is.
func (l *lister) ownName(n string) string {
	if l.src.Prefix != "" {
		if _, ok := l.names[l.src.Prefix+n]; ok {
			return l.src.Prefix + n
		}
	}
	return n
}

// textureListKey identifies the textures the source provides, which
// models, effects and maps refer to.
func (l *lister) textureListKey() string {
	var list []string
	for n := range l.names {
		if assets.TypeOf(n) == "texture" {
			list = append(list, n)
		}
	}
	slices.Sort(list)
	return hashStrings(list)
}
