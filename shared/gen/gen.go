// Package gen holds the map generators of procedural worlds and their
// version registry (docs/specs/generator-v1.md). The server and the web
// client run the same code, so a generator must give the same grid on
// every platform:
//
//   - Only integer and Q16.16 fixed-point math. No floating point.
//   - No Go maps, so no iteration order can change a result.
//   - A generator never changes within a version. A changed generator
//     gets a new version, and old versions stay callable.
//
// TestGuard checks the first two rules on the package's source.
//
// Builds with the build tag `procedural` contain the generators (strategy
// section 8.1). Builds without it contain only the classic world mode,
// the types and the checksum, and Lookup finds no generator.
package gen

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// World modes, chosen when a world is made (strategy section 8.1).
const (
	// ModeClassic runs rAthena's fixed maps.
	ModeClassic = "classic"
	// ModeProcedural generates maps from the world seed.
	ModeProcedural = "procedural"
)

// Modes returns the world modes this build can run, classic first.
func Modes() []string {
	if Procedural {
		return []string{ModeClassic, ModeProcedural}
	}
	return []string{ModeClassic}
}

// DefaultMode is the mode of a new world: procedural in builds with the
// tag, classic in builds without it.
func DefaultMode() string {
	if Procedural {
		return ModeProcedural
	}
	return ModeClassic
}

// HasMode reports whether this build can run the mode.
func HasMode(mode string) bool {
	for _, m := range Modes() {
		if m == mode {
			return true
		}
	}
	return false
}

// GAT cell types, as rAthena's map_gat2cell reads them (strategy section
// 8.5).
const (
	// CellWalkable is open ground.
	CellWalkable uint8 = 0
	// CellBlocked blocks walking and projectiles.
	CellBlocked uint8 = 1
	// CellWater is walkable water.
	CellWater uint8 = 3
	// CellGap blocks walking but not projectiles, such as a cliff.
	CellGap uint8 = 5
)

// WMAP cell flags (docs/specs/assets-v1.md, "Map (WMAP)").
const (
	FlagWalkable  uint8 = 1
	FlagShootable uint8 = 2
	FlagWater     uint8 = 4
)

// Flags returns the WMAP flags of a cell type. Unknown types are blocked.
func Flags(cellType uint8) uint8 {
	switch cellType {
	case CellWalkable:
		return FlagWalkable | FlagShootable
	case CellWater:
		return FlagWalkable | FlagShootable | FlagWater
	case CellGap:
		return FlagShootable
	default:
		return 0
	}
}

// MaxSize is the largest width or height of a grid, as for WMAP maps.
const MaxSize = 1024

// Params are a generator's inputs besides its node.
type Params struct {
	// Width and Height are the map's size in cells, 1 to MaxSize.
	Width, Height int
	// Biome is the index of the field's biome in the generator's biome
	// table, from 0. Version 1 has no biomes and ignores it.
	Biome int
}

// ErrSize is the error for a size outside 1 to MaxSize.
var ErrSize = errors.New("gen: the width and height must be 1 to 1024")

// Check returns ErrSize for a size outside 1 to MaxSize.
func (p Params) Check() error {
	if p.Width < 1 || p.Height < 1 || p.Width > MaxSize || p.Height > MaxSize {
		return ErrSize
	}
	return nil
}

// Grid is a generated cell grid. Cells go row by row from y = 0, the
// south edge, as in WMAP.
type Grid struct {
	Width, Height int
	// WaterLevel is the map's water level, in cells, up positive.
	WaterLevel rng.Fixed
	// Types holds one GAT cell type per cell.
	Types []uint8
	// Heights holds 4 corner heights per cell, in cells, up positive, in
	// WMAP's order: (x, y), (x+1, y), (x, y+1), (x+1, y+1).
	Heights []rng.Fixed
}

// checksumContext starts the checksum's input, so it never equals a hash
// of other data.
const checksumContext = "wa-grid-v1"

// Checksum returns the SHA-256 of the grid's canonical encoding:
//
//	"wa-grid-v1" || BE16(width) || BE16(height) || BE32(water level)
//	|| for each cell: type || BE32(4 corner heights)
//
// The server sends it with a map, and a client compares its own grid's
// checksum with it (strategy section 8.4).
func (g *Grid) Checksum() [32]byte {
	h := sha256.New()
	buf := make([]byte, 0, len(checksumContext)+8)
	buf = append(buf, checksumContext...)
	buf = binary.BigEndian.AppendUint16(buf, uint16(g.Width))      // #nosec G115 -- the size is at most MaxSize.
	buf = binary.BigEndian.AppendUint16(buf, uint16(g.Height))     // #nosec G115 -- the size is at most MaxSize.
	buf = binary.BigEndian.AppendUint32(buf, uint32(g.WaterLevel)) // #nosec G115 -- heights are hashed as two's complement.
	h.Write(buf)
	cell := make([]byte, 17)
	for i, t := range g.Types {
		cell[0] = t
		for c := range 4 {
			binary.BigEndian.PutUint32(cell[1+4*c:], uint32(g.Heights[4*i+c])) // #nosec G115 -- heights are hashed as two's complement.
		}
		h.Write(cell)
	}
	var sum [32]byte
	h.Sum(sum[:0])
	return sum
}

// Generator is one version of the map generator.
type Generator struct {
	// Version is the number each world records.
	Version int
	// RNG is the version of the rng spec it uses, such as "rng-v1".
	RNG string
	// Grid generates a map's cell grid from its public node.
	Grid func(node rng.Node, p Params) (Grid, error)
	// Field generates a field from its public node, or is nil for a
	// version without fields. Its grid is the one Grid returns.
	Field func(node rng.Node, p Params) (Field, error)
	// Biomes names the biomes of the biome table, by index. It is empty
	// for a version without fields.
	Biomes []string
	// Tilesets lists each tileset's texture and model slots, which the
	// field's WMAP names as texture/gen/<tileset>/<slot> and
	// model/gen/<tileset>/<slot>. It is empty for a version without
	// fields.
	Tilesets []Tileset
	// Climate picks the biome of field (x, y) from the world's public
	// branch, or is nil for a version without fields. Only the server
	// runs it, since clients never get the public branch.
	Climate func(public rng.Node, x, y int64) int
	// Map generates a dungeon floor or a town from its public node and
	// its params, or is nil for a version without them. Kinds lists the
	// kinds it makes, KindDungeon and KindTown.
	Map   func(node rng.Node, p MapParams) (Field, error)
	Kinds []string
}

// Tileset is a tileset's slots: ground textures, with water, and props.
type Tileset struct {
	Name string
	// Kind is the kind of map that uses it: KindField, KindDungeon or
	// KindTown.
	Kind     string
	Textures []string
	Models   []string
}

// Tileset returns a tileset of a kind by name.
func (g Generator) Tileset(kind, name string) (Tileset, bool) {
	for _, t := range g.Tilesets {
		if t.Name == name && (t.Kind == kind || t.Kind == "" && kind == KindField) {
			return t, true
		}
	}
	return Tileset{}, false
}

// Makes reports whether the generator makes maps of a kind.
func (g Generator) Makes(kind string) bool {
	if kind == KindField {
		return g.Field != nil
	}
	for _, k := range g.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// TextureName and ModelName are the asset names a field's WMAP uses for
// a tileset's slots. A tileset table maps them to other assets.
func TextureName(tileset, slot string) string { return "texture/gen/" + tileset + "/" + slot }

// ModelName is the asset name of a tileset's prop.
func ModelName(tileset, slot string) string { return "model/gen/" + tileset + "/" + slot }

// BiomeIndex returns a biome's index by name.
func (g Generator) BiomeIndex(name string) (int, bool) {
	for i, b := range g.Biomes {
		if b == name {
			return i, true
		}
	}
	return 0, false
}

// Versions returns the generator versions this build has, oldest first.
// It is empty in builds without the procedural tag.
func Versions() []int {
	out := make([]int, 0, len(registry))
	for _, g := range registry {
		out = append(out, g.Version)
	}
	return out
}

// Lookup returns a generator by version.
func Lookup(version int) (Generator, bool) {
	for _, g := range registry {
		if g.Version == version {
			return g, true
		}
	}
	return Generator{}, false
}

// Latest returns the newest generator. New worlds use it. ok is false in
// builds without the procedural tag.
func Latest() (g Generator, ok bool) {
	if len(registry) == 0 {
		return Generator{}, false
	}
	return registry[len(registry)-1], true
}
