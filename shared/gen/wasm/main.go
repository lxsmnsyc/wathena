//go:build tinygo && procedural

// Command wasm is the map generator for the web client, built with TinyGo:
//
//	tinygo build -o client/src/world/gen.wasm -target=wasm-unknown -opt=2 -no-debug -tags procedural ./shared/gen/wasm
//
// The client writes a map's public node key into the buffer key returns,
// calls generate, and reads the grid and its checksum from the buffers
// the other exports return. The buffers stay valid until the next call
// to generate.
package main

import (
	"errors"

	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

var (
	nodeKey [32]byte
	grid    gen.Grid
	field   gen.Field
	sum     [32]byte
	// input holds a dungeon floor's or a town's params, and places the
	// encoding of the map's places.
	input  [gen.MaxParams]byte
	places []byte
)

// keyBuffer returns the 32-byte buffer of the node key.
//
//export key
func keyBuffer() *byte { return &nodeKey[0] }

// rngVersion returns the rng spec version a generator version uses, such
// as 1 for rng-v1, or 0 when this module lacks the version.
//
//export rng_version
func rngVersion(version int32) int32 {
	g, ok := gen.Lookup(int(version))
	if !ok || g.RNG != rng.Version {
		return 0
	}
	return 1
}

// latest returns the newest generator version.
//
//export latest
func latest() int32 {
	g, _ := gen.Latest()
	return int32(g.Version) // #nosec G115 -- versions are small.
}

// biomes returns the number of biomes a generator version has: 0 for a
// version without fields, or -1 for a version this module lacks.
//
//export biomes
func biomes(version int32) int32 {
	g, ok := gen.Lookup(int(version))
	if !ok {
		return -1
	}
	return int32(len(g.Biomes)) // #nosec G115 -- a few biomes.
}

// generate makes a map: a field for a version with fields, else a grid.
// It returns 0, or:
//
//   - -1 for a version this module lacks,
//   - -2 for a size outside 1 to 1024, or below the smallest field,
//   - -3 when no attempt made a valid field,
//   - -4 for a biome the version lacks.
//
//export generate
func generate(version, width, height, biome int32) int32 {
	grid, field, places = gen.Grid{}, gen.Field{}, nil
	g, ok := gen.Lookup(int(version))
	if !ok {
		return -1
	}
	p := gen.Params{Width: int(width), Height: int(height), Biome: int(biome)}
	if g.Field == nil {
		out, err := g.Grid(rng.FromKey(nodeKey), p)
		if err != nil {
			return -2
		}
		grid = out
	} else {
		out, err := g.Field(rng.FromKey(nodeKey), p)
		switch {
		case errors.Is(err, gen.ErrNoField):
			return -3
		case errors.Is(err, gen.ErrBiome):
			return -4
		case err != nil:
			return -2
		}
		field, grid = out, out.Grid
		places = gen.EncodePlaces(field.Places)
	}
	sum = grid.Checksum()
	return 0
}

// paramsBuffer returns the buffer of a dungeon floor's or a town's
// params, gen.MaxParams bytes.
//
//export params
func paramsBuffer() *byte { return &input[0] }

// generateMap makes a dungeon floor (kind 1) or a town (kind 2) from the
// node and the first length bytes of the params buffer. It returns 0,
// or:
//
//   - -1 for a version this module lacks, or one without the kind,
//   - -2 for params that do not decode or check,
//   - -3 when no attempt made a valid map,
//   - -4 for a tileset or prop the version lacks.
//
//export generate_map
func generateMap(version, kind, length int32) int32 {
	grid, field, places = gen.Grid{}, gen.Field{}, nil
	g, ok := gen.Lookup(int(version))
	var k string
	switch kind {
	case 1:
		k = gen.KindDungeon
	case 2:
		k = gen.KindTown
	}
	if !ok || k == "" || !g.Makes(k) {
		return -1
	}
	if length < 0 || int(length) > len(input) {
		return -2
	}
	out, err := g.Map(rng.FromKey(nodeKey), gen.MapParams{Kind: k, Params: input[:length]})
	switch {
	case errors.Is(err, gen.ErrNoMap):
		return -3
	case errors.Is(err, gen.ErrTileset):
		return -4
	case err != nil:
		return -2
	}
	field, grid = out, out.Grid
	places = gen.EncodePlaces(field.Places)
	sum = grid.Checksum()
	return 0
}

// width and height return the size of the last map.
//
//export width
func width() int32 { return int32(grid.Width) } // #nosec G115 -- at most 1024.

//export height
func height() int32 { return int32(grid.Height) } // #nosec G115 -- at most 1024.

// placesBuffer returns the encoding of the map's places,
// places_size bytes (gen.EncodePlaces), or 0 for a map without places.
//
//export places
func placesBuffer() *byte {
	if len(places) == 0 {
		return nil
	}
	return &places[0]
}

//export places_size
func placesSize() int32 { return int32(len(places)) } // #nosec G115 -- a few kilobytes.

// wmap returns the field's WMAP map, wmap_size bytes, or 0 for a grid
// without one.
//
//export wmap
func wmap() *byte {
	if len(field.WMAP) == 0 {
		return nil
	}
	return &field.WMAP[0]
}

// wmapSize returns the length of the field's WMAP map.
//
//export wmap_size
func wmapSize() int32 { return int32(len(field.WMAP)) } // #nosec G115 -- maps are below 64 MiB.

// attempt returns the attempt that made the field: 0 for its node.
//
//export attempt
func attempt() int32 { return int32(field.Attempt) } // #nosec G115 -- below 8.

// types returns the cell types, width × height bytes row by row.
//
//export types
func types() *uint8 { return &grid.Types[0] }

// heights returns the corner heights, 4 Q16.16 values per cell.
//
//export heights
func heights() *rng.Fixed { return &grid.Heights[0] }

// water returns the water level in Q16.16.
//
//export water
func water() int32 { return int32(grid.WaterLevel) }

// checksum returns the 32-byte checksum of the grid.
//
//export checksum
func checksum() *byte { return &sum[0] }

func main() {}
