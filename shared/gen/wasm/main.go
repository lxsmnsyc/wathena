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
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

var (
	nodeKey [32]byte
	grid    gen.Grid
	sum     [32]byte
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

// generate makes the grid of a map. It returns 0, -1 for a version this
// module lacks, or -2 for a size outside 1 to 1024.
//
//export generate
func generate(version, width, height int32) int32 {
	grid = gen.Grid{}
	g, ok := gen.Lookup(int(version))
	if !ok {
		return -1
	}
	out, err := g.Grid(rng.FromKey(nodeKey), gen.Params{Width: int(width), Height: int(height)})
	if err != nil {
		return -2
	}
	grid = out
	sum = grid.Checksum()
	return 0
}

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
