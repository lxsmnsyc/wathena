// Package mapfolder reads maps in the project's map folder format
// (strategy section 8.12, docs/specs/map-folder-v1.md): a folder with
// map.yaml, cells.bin, materials.bin, objects.yaml and places.yaml. A
// mod ships its home map this way, under maps/<id>/ (proposal 0002,
// review decision 43).
//
//   - Read reads a folder and checks its files.
//   - Check runs the generator's checks on the map: places on walkable
//     cells, every place reached from the first one, and a home map's
//     portal.
//   - WMAP writes the map as WMAP for clients, Grid as the zone's cells.
package mapfolder

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"

	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// Files of a map folder.
const (
	MapFile       = "map.yaml"
	CellsFile     = "cells.bin"
	MaterialsFile = "materials.bin"
	ObjectsFile   = "objects.yaml"
	PlacesFile    = "places.yaml"
)

// Format is the version of the format this package reads.
const Format = 1

// Dir is the folder of a mod that holds its map folders.
const Dir = "maps"

// Tags and flags.
const (
	// TagHome marks a home map: new characters start there, and the
	// layout links it to the first town.
	TagHome = "home"
	// FlagNoMemo forbids Warp Portal memo points on the map.
	FlagNoMemo = "nomemo"
)

// Flags lists the map flags the format knows.
var Flags = []string{FlagNoMemo}

// Limits of the format.
const (
	MinSize      = 8
	MaxSize      = gen.MaxSize
	MaxMaterials = 255
	MaxObjects   = 4096
	MaxPlaces    = 256
	MaxPlaceTags = 8
	MaxName      = 64
	// MaxHeight is the largest corner height, up or down, in cells.
	MaxHeight = 256
	// maxYAML is the largest YAML file read.
	maxYAML = 4 << 20
)

// Binary headers: a 4-byte magic, a version byte, and the width and the
// height as big-endian u16 values.
const (
	cellsMagic     = "WCEL"
	materialsMagic = "WMTL"
	headerSize     = 9
	cellRecord     = 17
	materialRecord = 3
)

// Map is a map folder, read.
type Map struct {
	ID string
	// Name is the display name.
	Name          string
	Width, Height int
	// Biome is a biome of the generator, or empty.
	Biome string
	// WaterLevel is the water level in cells, when HasWater is set.
	WaterLevel rng.Fixed
	HasWater   bool
	// Music is the background music's asset name, or empty.
	Music string
	Tags  []string
	Flags []string
	// Materials are the ground textures' asset names, by index.
	Materials []string
	// Types holds one GAT cell type per cell, row by row from the south
	// edge, and Heights 4 corner heights per cell, as gen.Grid.
	Types   []uint8
	Heights []rng.Fixed
	// Material, Blend and Weight are each cell's material, the material
	// it blends toward, and the blend's weight from 0 to 255.
	Material, Blend, Weight []uint8
	Objects                 []Object
	Places                  []gen.Place
}

// Object is a placed model.
type Object struct {
	Model string
	// X and Y are its point in cells, Rotation its turn about the up
	// axis in degrees, and Scale its size.
	X, Y, Rotation, Scale float64
}

// HasTag reports whether the map has a tag.
func (m *Map) HasTag(tag string) bool { return slices.Contains(m.Tags, tag) }

// HasFlag reports whether the map has a flag.
func (m *Map) HasFlag(flag string) bool { return slices.Contains(m.Flags, flag) }

// ValidID reports whether a map folder's ID is 1 to 32 lowercase
// letters, digits, _ and -.
func ValidID(id string) bool { return gen.ValidDungeonID(id) }

// mapYAML is map.yaml.
type mapYAML struct {
	Format     int      `yaml:"format"`
	ID         string   `yaml:"id"`
	Name       string   `yaml:"name"`
	Size       []int    `yaml:"size"`
	Biome      string   `yaml:"biome"`
	WaterLevel *float64 `yaml:"water_level"`
	Music      string   `yaml:"music"`
	Tags       []string `yaml:"tags"`
	Flags      []string `yaml:"flags"`
	Materials  []string `yaml:"materials"`
}

type objectYAML struct {
	Model    string   `yaml:"model"`
	X        float64  `yaml:"x"`
	Y        float64  `yaml:"y"`
	Rotation float64  `yaml:"rotation"`
	Scale    *float64 `yaml:"scale"`
}

type placeYAML struct {
	Tags []string `yaml:"tags"`
	X    int      `yaml:"x"`
	Y    int      `yaml:"y"`
}

// ErrFormat is the error of a folder that does not follow the format.
var ErrFormat = errors.New("mapfolder: not a map folder")

// readFile reads a file of at most limit bytes.
func readFile(fsys fs.FS, name string, limit int64) ([]byte, error) {
	f, err := fsys.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s is over %d bytes", path.Base(name), limit)
	}
	return b, nil
}

// decodeYAML decodes a YAML file strictly: unknown keys are errors.
func decodeYAML(b []byte, v any) error {
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(v); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// validAsset reports whether an asset name is 1 to 255 bytes of
// printable UTF-8 with a /, such as texture/gen/grassland/grass.
func validAsset(s string) bool {
	if s == "" || len(s) > 255 || !utf8.ValidString(s) || !strings.Contains(s, "/") || strings.HasPrefix(s, "/") {
		return false
	}
	for _, r := range s {
		if !unicode.IsPrint(r) || r == ' ' {
			return false
		}
	}
	return true
}

// Read reads the map folder dir of fsys and checks its files against
// the format. It does not run the generator's checks: Check does.
func Read(fsys fs.FS, dir string) (*Map, error) {
	wrap := func(file string, err error) error {
		return fmt.Errorf("%w: %s: %w", ErrFormat, path.Join(dir, file), err)
	}
	b, err := readFile(fsys, path.Join(dir, MapFile), maxYAML)
	if err != nil {
		return nil, wrap(MapFile, err)
	}
	var my mapYAML
	if err := decodeYAML(b, &my); err != nil {
		return nil, wrap(MapFile, err)
	}
	m := &Map{ID: my.ID, Name: my.Name, Biome: my.Biome, Music: my.Music, Tags: my.Tags, Flags: my.Flags, Materials: my.Materials}
	switch {
	case my.Format != Format:
		return nil, wrap(MapFile, fmt.Errorf("format %d is not known; this server reads format %d", my.Format, Format))
	case !ValidID(my.ID):
		return nil, wrap(MapFile, errors.New("id is 1 to 32 lowercase letters, digits, _ and -"))
	case my.Name == "" || utf8.RuneCountInString(my.Name) > MaxName || strings.TrimSpace(my.Name) != my.Name || strings.ContainsFunc(my.Name, unicode.IsControl):
		return nil, wrap(MapFile, fmt.Errorf("name is 1 to %d characters without control characters or spaces at the ends", MaxName))
	case len(my.Size) != 2 || my.Size[0] < MinSize || my.Size[1] < MinSize || my.Size[0] > MaxSize || my.Size[1] > MaxSize:
		return nil, wrap(MapFile, fmt.Errorf("size is [width, height], each %d to %d cells", MinSize, MaxSize))
	case my.Music != "" && !validAsset(my.Music):
		return nil, wrap(MapFile, errors.New("music is an asset name, such as music/town"))
	case len(my.Materials) == 0 || len(my.Materials) > MaxMaterials:
		return nil, wrap(MapFile, fmt.Errorf("materials is 1 to %d texture asset names", MaxMaterials))
	}
	m.Width, m.Height = my.Size[0], my.Size[1]
	if my.WaterLevel != nil {
		w := *my.WaterLevel
		if w < -MaxHeight || w > MaxHeight {
			return nil, wrap(MapFile, fmt.Errorf("water_level is %d to %d cells", -MaxHeight, MaxHeight))
		}
		m.WaterLevel, m.HasWater = rng.Fixed(w*float64(rng.One)), true
	}
	for _, t := range my.Tags {
		if !gen.ValidTag(t) {
			return nil, wrap(MapFile, fmt.Errorf("tag %q: a tag is 1 to 64 characters of lowercase letters, digits, _, -, . and : only", t))
		}
	}
	for _, f := range my.Flags {
		if !slices.Contains(Flags, f) {
			return nil, wrap(MapFile, fmt.Errorf("flag %q is not known; the flags are %s", f, strings.Join(Flags, ", ")))
		}
	}
	for _, t := range my.Materials {
		if !validAsset(t) {
			return nil, wrap(MapFile, fmt.Errorf("material %q is not an asset name, such as texture/gen/grassland/grass", t))
		}
	}
	n := m.Width * m.Height
	if b, err = readFile(fsys, path.Join(dir, CellsFile), int64(headerSize+cellRecord*n)); err != nil {
		return nil, wrap(CellsFile, err)
	}
	if err := m.readCells(b); err != nil {
		return nil, wrap(CellsFile, err)
	}
	if b, err = readFile(fsys, path.Join(dir, MaterialsFile), int64(headerSize+materialRecord*n)); err != nil {
		return nil, wrap(MaterialsFile, err)
	}
	if err := m.readMaterials(b); err != nil {
		return nil, wrap(MaterialsFile, err)
	}
	if err := m.readObjects(fsys, dir); err != nil {
		return nil, wrap(ObjectsFile, err)
	}
	if err := m.readPlaces(fsys, dir); err != nil {
		return nil, wrap(PlacesFile, err)
	}
	return m, nil
}

// header checks a binary file's header and length.
func (m *Map) header(b []byte, magic string, record int) error {
	switch {
	case len(b) < headerSize || string(b[:4]) != magic:
		return fmt.Errorf("the file does not start with %s", magic)
	case b[4] != Format:
		return fmt.Errorf("version %d is not known; this server reads version %d", b[4], Format)
	}
	w, h := int(binary.BigEndian.Uint16(b[5:])), int(binary.BigEndian.Uint16(b[7:]))
	if w != m.Width || h != m.Height {
		return fmt.Errorf("the header says %d × %d cells, and map.yaml %d × %d", w, h, m.Width, m.Height)
	}
	if len(b) != headerSize+record*w*h {
		return fmt.Errorf("the file has %d bytes; %d × %d cells take %d", len(b), w, h, headerSize+record*w*h)
	}
	return nil
}

func (m *Map) readCells(b []byte) error {
	if err := m.header(b, cellsMagic, cellRecord); err != nil {
		return err
	}
	n := m.Width * m.Height
	m.Types, m.Heights = make([]uint8, n), make([]rng.Fixed, 4*n)
	for i := range n {
		r := b[headerSize+cellRecord*i:]
		t := r[0]
		if t != gen.CellWalkable && t != gen.CellBlocked && t != gen.CellWater && t != gen.CellGap {
			return fmt.Errorf("cell %d,%d has the type %d; the types are 0 walkable, 1 blocked, 3 walkable water and 5 gap", i%m.Width, i/m.Width, t)
		}
		m.Types[i] = t
		for c := range 4 {
			h := rng.Fixed(int32(binary.BigEndian.Uint32(r[1+4*c:]))) // #nosec G115 -- two's complement.
			if h < -MaxHeight*rng.One || h > MaxHeight*rng.One {
				return fmt.Errorf("cell %d,%d has a corner at %d cells; corners are %d to %d", i%m.Width, i/m.Width, h.Int(), -MaxHeight, MaxHeight)
			}
			m.Heights[4*i+c] = h
		}
	}
	return nil
}

func (m *Map) readMaterials(b []byte) error {
	if err := m.header(b, materialsMagic, materialRecord); err != nil {
		return err
	}
	n := m.Width * m.Height
	m.Material, m.Blend, m.Weight = make([]uint8, n), make([]uint8, n), make([]uint8, n)
	for i := range n {
		r := b[headerSize+materialRecord*i:]
		if int(r[0]) >= len(m.Materials) || int(r[1]) >= len(m.Materials) {
			return fmt.Errorf("cell %d,%d names material %d or %d; map.yaml lists %d", i%m.Width, i/m.Width, r[0], r[1], len(m.Materials))
		}
		m.Material[i], m.Blend[i], m.Weight[i] = r[0], r[1], r[2]
	}
	return nil
}

func (m *Map) readObjects(fsys fs.FS, dir string) error {
	b, err := readFile(fsys, path.Join(dir, ObjectsFile), maxYAML)
	if err != nil {
		return err
	}
	var list []objectYAML
	if err := decodeYAML(b, &list); err != nil {
		return err
	}
	if len(list) > MaxObjects {
		return fmt.Errorf("%d objects; the most is %d", len(list), MaxObjects)
	}
	for i, o := range list {
		scale := 1.0
		if o.Scale != nil {
			scale = *o.Scale
		}
		switch {
		case !validAsset(o.Model):
			return fmt.Errorf("object %d: model is an asset name, such as model/tree/pine", i)
		case o.X < 0 || o.Y < 0 || o.X > float64(m.Width) || o.Y > float64(m.Height):
			return fmt.Errorf("object %d: %g,%g is outside the map", i, o.X, o.Y)
		case scale <= 0 || scale > 16:
			return fmt.Errorf("object %d: scale is above 0 and at most 16", i)
		case o.Rotation < -360 || o.Rotation > 360:
			return fmt.Errorf("object %d: rotation is -360 to 360 degrees", i)
		}
		m.Objects = append(m.Objects, Object{Model: o.Model, X: o.X, Y: o.Y, Rotation: o.Rotation, Scale: scale})
	}
	return nil
}

func (m *Map) readPlaces(fsys fs.FS, dir string) error {
	b, err := readFile(fsys, path.Join(dir, PlacesFile), maxYAML)
	if err != nil {
		return err
	}
	var list []placeYAML
	if err := decodeYAML(b, &list); err != nil {
		return err
	}
	if len(list) > MaxPlaces {
		return fmt.Errorf("%d places; the most is %d", len(list), MaxPlaces)
	}
	for i, p := range list {
		if len(p.Tags) == 0 || len(p.Tags) > MaxPlaceTags {
			return fmt.Errorf("place %d: a place has 1 to %d tags", i, MaxPlaceTags)
		}
		for _, t := range p.Tags {
			if !gen.ValidTag(t) {
				return fmt.Errorf("place %d: tag %q: a tag is 1 to 64 characters of lowercase letters, digits, _, -, . and : only", i, t)
			}
		}
		if p.X < 0 || p.Y < 0 || p.X >= m.Width || p.Y >= m.Height {
			return fmt.Errorf("place %d: %d,%d is outside the map", i, p.X, p.Y)
		}
		m.Places = append(m.Places, gen.Place{Tags: p.Tags, X: p.X, Y: p.Y})
	}
	return nil
}

// walkable reports whether a cell is walkable ground or water.
func (m *Map) walkable(x, y int) bool {
	if x < 0 || y < 0 || x >= m.Width || y >= m.Height {
		return false
	}
	t := m.Types[y*m.Width+x]
	return t == gen.CellWalkable || t == gen.CellWater
}

// Check runs the generator's checks on the map (strategy section 8.5,
// "Validation and retries") and returns every problem:
//
//   - Every place stands on a walkable cell.
//   - Every place is reached from the first place, by walks of 8
//     directions where a diagonal step needs both cells beside it
//     walkable, as rAthena's path search.
//   - A home map has a portal place, where the layout links it to the
//     first town.
func (m *Map) Check() []string {
	var out []string
	if m.HasTag(TagHome) && !slices.ContainsFunc(m.Places, func(p gen.Place) bool { return p.HasTag("portal") }) {
		out = append(out, "a home map needs a place tagged portal, where its warp to the first town stands")
	}
	for i, p := range m.Places {
		if !m.walkable(p.X, p.Y) {
			out = append(out, fmt.Sprintf("place %d (%s) stands on %d,%d, which is not walkable", i, p.Tags[0], p.X, p.Y))
		}
	}
	if len(m.Places) == 0 || !m.walkable(m.Places[0].X, m.Places[0].Y) {
		return out
	}
	seen := m.reach(m.Places[0].X, m.Places[0].Y)
	for i, p := range m.Places[1:] {
		if m.walkable(p.X, p.Y) && !seen[p.Y*m.Width+p.X] {
			out = append(out, fmt.Sprintf("place %d (%s) at %d,%d is not reached from place 0 (%s)", i+1, p.Tags[0], p.X, p.Y, m.Places[0].Tags[0]))
		}
	}
	return out
}

// reach marks the cells a walk reaches from a cell.
func (m *Map) reach(x, y int) []bool {
	seen := make([]bool, m.Width*m.Height)
	seen[y*m.Width+x] = true
	q := []int{y*m.Width + x}
	for len(q) > 0 {
		c := q[0]
		q = q[1:]
		cx, cy := c%m.Width, c/m.Width
		for dy := -1; dy <= 1; dy++ {
			for dx := -1; dx <= 1; dx++ {
				nx, ny := cx+dx, cy+dy
				if dx == 0 && dy == 0 || !m.walkable(nx, ny) || seen[ny*m.Width+nx] {
					continue
				}
				if dx != 0 && dy != 0 && (!m.walkable(cx+dx, cy) || !m.walkable(cx, cy+dy)) {
					continue
				}
				seen[ny*m.Width+nx] = true
				q = append(q, ny*m.Width+nx)
			}
		}
	}
	return seen
}

// Grid returns the map's cell grid, with its checksum's water level.
func (m *Map) Grid() gen.Grid {
	level := m.WaterLevel
	if !m.HasWater {
		level = -MaxHeight * rng.One
	}
	return gen.Grid{Width: m.Width, Height: m.Height, WaterLevel: level, Types: slices.Clone(m.Types), Heights: slices.Clone(m.Heights)}
}

// Find lists the map folders of a mod's files: each folder under maps/
// with a map.yaml, by name.
func Find(fsys fs.FS) ([]string, error) {
	entries, err := fs.ReadDir(fsys, Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if _, err := fs.Stat(fsys, path.Join(Dir, e.Name(), MapFile)); err == nil {
			out = append(out, path.Join(Dir, e.Name()))
		}
	}
	return out, nil
}
