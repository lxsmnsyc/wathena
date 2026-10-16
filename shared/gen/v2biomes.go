//go:build procedural

package gen

import (
	"slices"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// The biome table of generator version 2 (strategy section 8.5, "Field
// steps"). A biome's index is part of the version: a field records it,
// and the server sends it to clients.

// Ground materials. Each names the texture texture/gen/<tileset>/<name>.
const (
	matGrass = iota
	matDirt
	matSand
	matRock
	matSnow
	matMud
	matCount
)

// v2Materials names the materials by index.
var v2Materials = [matCount]string{"grass", "dirt", "sand", "rock", "snow", "mud"}

// v2Prop is a prop of a biome with its weight among the biome's props.
type v2Prop struct {
	Name   string
	Weight uint64
}

// v2Biome holds a biome's rules. Lengths and heights are Q16.16 cells.
type v2Biome struct {
	Name string
	// Tileset names the textures and models: texture/gen/<tileset>/...
	// and model/gen/<tileset>/....
	Tileset string

	// Height: Octaves of gradient noise, the first with a spacing of
	// 1 << Shift cells and amplitude Amp, each next one with half of
	// both. Ridged noise makes crests, as dunes. Warp is the amplitude
	// of the domain warp. Basin flattens heights below -Amp/4. Ramp is
	// the drop toward one edge of a beach.
	Octaves int
	Shift   uint
	Amp     rng.Fixed
	Ridged  bool
	Warp    rng.Fixed
	Basin   bool
	Ramp    rng.Fixed
	// Terrace is the height of a terrace step, or 0 for none. Each step
	// rises over an eighth of its height, which makes cliffs between
	// flat levels.
	Terrace rng.Fixed

	// Water: WaterShare is the share of cells below the water level, in
	// thousandths. Water up to Shallow deep is walkable. Rivers is the
	// most rivers.
	WaterShare int
	Shallow    rng.Fixed
	Rivers     int

	// Cliffs: a cell whose corners differ by more than Slope becomes
	// Cliff, a gap or blocked.
	Slope rng.Fixed
	Cliff uint8

	// Validation: the walkable share, water included, in thousandths.
	MinWalk, MaxWalk int

	// Objects: Poisson-disk points Spacing cells apart, of which Density
	// in a thousand become props where the density noise is middling.
	Spacing int
	Density int64
	Props   []v2Prop
	// Thickets: Thicket cells in a thousand, where a slow noise is
	// highest, are blocked, with ThicketProp every 3 cells or so.
	Thicket     int
	ThicketProp string

	// Materials: Ground is the base, Patch shows where the patch noise
	// is above 0.6, Shore lines water, Road covers roads.
	Ground, Patch, Shore, Road int

	// Light: the diffuse and ambient colors.
	Diffuse, Ambient [3]rng.Fixed
}

// Biome indexes of version 2.
const (
	biomeGrassland = iota
	biomeForest
	biomeDesert
	biomeSnow
	biomeSwamp
	biomeBeach
)

// v2Biomes is the biome table of version 2, by index.
var v2Biomes = []v2Biome{
	biomeGrassland: {
		Name: "grassland", Tileset: "grassland",
		Octaves: 5, Shift: 6, Amp: 3 * rng.One, Warp: 6 * rng.One,
		WaterShare: 60, Shallow: rng.One * 3 / 4, Rivers: 1,
		Slope: rng.One * 3 / 5, Cliff: CellGap,
		MinWalk: 550, MaxWalk: 850,
		Spacing: 4, Density: 120, Terrace: 2 * rng.One, Thicket: 120, ThicketProp: "tree",
		Props:  []v2Prop{{"tree", 3}, {"bush", 5}, {"rock", 2}},
		Ground: matGrass, Patch: matDirt, Shore: matSand, Road: matDirt,
		Diffuse: [3]rng.Fixed{rng.One, rng.One, rng.One},
		Ambient: [3]rng.Fixed{rng.One * 35 / 100, rng.One * 35 / 100, rng.One * 35 / 100},
	},
	biomeForest: {
		Name: "forest", Tileset: "forest",
		Octaves: 5, Shift: 6, Amp: 5 * rng.One, Warp: 8 * rng.One,
		WaterShare: 50, Shallow: rng.One * 3 / 4, Rivers: 2,
		Slope: rng.One * 7 / 10, Cliff: CellGap,
		MinWalk: 450, MaxWalk: 800,
		Spacing: 4, Density: 200, Terrace: 3 * rng.One, Thicket: 250, ThicketProp: "tree",
		Props:  []v2Prop{{"tree", 6}, {"pine", 2}, {"bush", 2}},
		Ground: matGrass, Patch: matDirt, Shore: matMud, Road: matDirt,
		Diffuse: [3]rng.Fixed{rng.One * 85 / 100, rng.One * 9 / 10, rng.One * 8 / 10},
		Ambient: [3]rng.Fixed{rng.One * 3 / 10, rng.One * 33 / 100, rng.One * 3 / 10},
	},
	biomeDesert: {
		Name: "desert", Tileset: "desert",
		Octaves: 4, Shift: 6, Amp: 4 * rng.One, Ridged: true, Warp: 10 * rng.One, Basin: true,
		Slope: rng.One * 7 / 10, Cliff: CellGap,
		MinWalk: 550, MaxWalk: 950,
		Spacing: 6, Density: 80, Terrace: 0, Thicket: 40, ThicketProp: "rock",
		Props:  []v2Prop{{"cactus", 5}, {"rock", 5}},
		Ground: matSand, Patch: matDirt, Shore: matSand, Road: matDirt,
		Diffuse: [3]rng.Fixed{rng.One, rng.One * 95 / 100, rng.One * 85 / 100},
		Ambient: [3]rng.Fixed{rng.One * 4 / 10, rng.One * 38 / 100, rng.One * 33 / 100},
	},
	biomeSnow: {
		Name: "snow", Tileset: "snow",
		Octaves: 6, Shift: 6, Amp: 9 * rng.One, Warp: 8 * rng.One,
		WaterShare: 30, Shallow: rng.One / 2, Rivers: 1,
		Slope: rng.One * 9 / 10, Cliff: CellGap,
		MinWalk: 400, MaxWalk: 800,
		Spacing: 4, Density: 150, Terrace: 3 * rng.One, Thicket: 100, ThicketProp: "pine",
		Props:  []v2Prop{{"pine", 7}, {"rock", 3}},
		Ground: matSnow, Patch: matRock, Shore: matDirt, Road: matDirt,
		Diffuse: [3]rng.Fixed{rng.One * 9 / 10, rng.One * 95 / 100, rng.One},
		Ambient: [3]rng.Fixed{rng.One * 4 / 10, rng.One * 42 / 100, rng.One * 45 / 100},
	},
	biomeSwamp: {
		Name: "swamp", Tileset: "swamp",
		Octaves: 4, Shift: 5, Amp: rng.One, Warp: 4 * rng.One,
		WaterShare: 280, Shallow: rng.One * 3 / 2,
		Slope: rng.One / 2, Cliff: CellBlocked,
		MinWalk: 600, MaxWalk: 950,
		Spacing: 4, Density: 200, Terrace: 0, Thicket: 80, ThicketProp: "deadtree",
		Props:  []v2Prop{{"deadtree", 4}, {"reed", 6}},
		Ground: matMud, Patch: matGrass, Shore: matMud, Road: matDirt,
		Diffuse: [3]rng.Fixed{rng.One * 75 / 100, rng.One * 8 / 10, rng.One * 7 / 10},
		Ambient: [3]rng.Fixed{rng.One * 3 / 10, rng.One * 32 / 100, rng.One * 28 / 100},
	},
	biomeBeach: {
		Name: "beach", Tileset: "beach",
		Octaves: 5, Shift: 6, Amp: rng.One * 3 / 2, Warp: 6 * rng.One, Ramp: 4 * rng.One,
		WaterShare: 300, Shallow: rng.One * 3 / 4,
		Slope: rng.One * 3 / 5, Cliff: CellGap,
		MinWalk: 450, MaxWalk: 850,
		Spacing: 5, Density: 100, Terrace: 0, Thicket: 60, ThicketProp: "palm",
		Props:  []v2Prop{{"palm", 6}, {"rock", 4}},
		Ground: matSand, Patch: matGrass, Shore: matSand, Road: matDirt,
		Diffuse: [3]rng.Fixed{rng.One, rng.One, rng.One * 95 / 100},
		Ambient: [3]rng.Fixed{rng.One * 4 / 10, rng.One * 4 / 10, rng.One * 38 / 100},
	},
}

// v2BiomeNames returns the biome names by index.
func v2BiomeNames() []string {
	out := make([]string, len(v2Biomes))
	for i, b := range v2Biomes {
		out[i] = b.Name
	}
	return out
}

// v2Climate is the biome table by temperature (rows, cold to hot) and
// moisture (columns, dry to wet), in thirds.
var v2Climate = [3][3]int{
	{biomeSnow, biomeSnow, biomeForest},
	{biomeGrassland, biomeForest, biomeSwamp},
	{biomeDesert, biomeGrassland, biomeBeach},
}

// climateV2 picks the biome of field (x, y): two value noises over field
// coordinates, 4 fields apart, give temperature and moisture.
func climateV2(public rng.Node, x, y int64) int {
	n := public.Child("climate")
	third := func(v int64) int { return min(int(v*3>>16), 2) }
	t := v2Value{n: n, shift: 2, octave: 0}.at(x, y)
	m := v2Value{n: n, shift: 2, octave: 1}.at(x, y)
	return v2Climate[third(t)][third(m)]
}

// v2Tilesets lists each biome's tileset with the slots its fields can
// name: its materials, rock, water, and its props.
func v2Tilesets() []Tileset {
	out := make([]Tileset, 0, len(v2Biomes))
	for _, b := range v2Biomes {
		t := Tileset{Name: b.Tileset}
		for _, m := range []int{b.Ground, b.Patch, b.Shore, b.Road, matRock} {
			if !slices.Contains(t.Textures, v2Materials[m]) {
				t.Textures = append(t.Textures, v2Materials[m])
			}
		}
		t.Textures = append(t.Textures, "water")
		for _, p := range b.Props {
			t.Models = append(t.Models, p.Name)
		}
		if b.ThicketProp != "" && !slices.Contains(t.Models, b.ThicketProp) {
			t.Models = append(t.Models, b.ThicketProp)
		}
		slices.Sort(t.Textures)
		slices.Sort(t.Models)
		out = append(out, t)
	}
	return out
}
