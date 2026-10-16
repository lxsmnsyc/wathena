package gen

import (
	"errors"
	"strconv"
	"strings"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// The seed tree of a procedural world's maps (decision 0039):
//
//	root (secret, the world row's seed)
//	├── public                    PublicBranch: the game role reads it
//	│   ├── climate               the biome of each field, server only
//	│   └── field(x, y)           FieldNode: sent to clients
//	│       ├── terrain, warp, rivers, portals, objects, density, materials
//	│       └── attempt(n)        a retry, with the same children
//	└── secret                    spawns, loot and other server branches
//
// A field's node does not reveal the public branch, another field's node
// or any secret branch, since each child key is one-way.

// PublicBranch returns the public branch of a world's root. The server's
// game role holds it instead of the root.
func PublicBranch(root rng.Node) rng.Node { return root.Child("public") }

// FieldNode returns the public node of field (x, y) under the public
// branch. The server sends it to clients.
func FieldNode(public rng.Node, x, y int64) rng.Node { return public.Child("field", x, y) }

// fieldPrefix starts every field's map ID.
const fieldPrefix = "gen:field:"

// FieldID returns the map ID of field (x, y), such as gen:field:3:-1
// (strategy section 8.10).
func FieldID(x, y int64) string {
	return fieldPrefix + strconv.FormatInt(x, 10) + ":" + strconv.FormatInt(y, 10)
}

// ParseFieldID reads a field's coordinates from its map ID.
func ParseFieldID(id string) (x, y int64, ok bool) {
	rest, found := strings.CutPrefix(id, fieldPrefix)
	if !found {
		return 0, 0, false
	}
	xs, ys, found := strings.Cut(rest, ":")
	if !found {
		return 0, 0, false
	}
	x, errX := strconv.ParseInt(xs, 10, 64)
	y, errY := strconv.ParseInt(ys, 10, 64)
	if errX != nil || errY != nil || FieldID(x, y) != id {
		return 0, 0, false
	}
	return x, y, true
}

// IsGenerated reports whether a map name is a generated map's ID, which
// starts with "gen:". Asset names never contain ":".
func IsGenerated(name string) bool { return strings.HasPrefix(name, "gen:") }

// Edges of a map, for portals.
const (
	EdgeSouth = 0
	EdgeEast  = 1
	EdgeNorth = 2
	EdgeWest  = 3
)

// Portal is a place on an edge of a field where a warp can go. The cells
// around it are flat and walkable, and every portal of a field reaches
// every other one.
type Portal struct {
	Edge int
	X, Y int
}

// Object is a prop placed on one cell, which it blocks.
type Object struct {
	// Prop is the prop's name in its tileset, such as tree or rock.
	Prop string
	X, Y int
	// Turn is the rotation about the up axis, in 16ths of a turn.
	Turn int
	// Scale is the model's scale.
	Scale rng.Fixed
}

// Field is a generated field: its cell grid and what a server and a
// client need besides it.
type Field struct {
	Grid
	// Biome is the biome's index in the generator's biome table.
	Biome int
	// Attempt is the attempt that passed validation: 0 for the field's
	// node, n for its child attempt(n).
	Attempt int
	Portals []Portal
	Objects []Object
	// WMAP is the field as a WMAP map (docs/specs/assets-v1.md).
	WMAP []byte
}

// Errors of field generation.
var (
	// ErrFieldSize is the error for a field below the generator's
	// smallest size.
	ErrFieldSize = errors.New("gen: the field is too small")
	// ErrBiome is the error for a biome the generator lacks.
	ErrBiome = errors.New("gen: unknown biome")
	// ErrNoField is the error when every attempt failed validation.
	ErrNoField = errors.New("gen: no attempt made a valid field")
)
