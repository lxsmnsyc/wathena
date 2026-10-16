//go:build procedural

package gen

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/shared/rng"
)

// checkField checks what every version 2 field must hold:
//
//   - Only the 4 cell types, and the heights the grid has.
//   - Every portal is walkable and reaches every other portal, and every
//     walkable cell reaches them, along the 4 axes.
//   - The walkable share is in the biome's range.
//   - Each prop blocks its cell.
//   - The WMAP has the grid's flags and heights.
func checkField(t *testing.T, f Field) {
	t.Helper()
	b := v2Biomes[f.Biome]
	n := f.Width * f.Height
	if len(f.Types) != n || len(f.Heights) != 4*n {
		t.Fatalf("%d types and %d heights for %d cells", len(f.Types), len(f.Heights), n)
	}
	walk := 0
	for _, c := range f.Types {
		switch c {
		case CellWalkable, CellWater:
			walk++
		case CellBlocked, CellGap:
		default:
			t.Fatalf("cell type %d", c)
		}
	}
	if share := walk * 1000 / n; share < b.MinWalk || share > b.MaxWalk {
		t.Errorf("%s: walkable share %d, want %d to %d", b.Name, share, b.MinWalk, b.MaxWalk)
	}
	if len(f.Portals) != 4 {
		t.Fatalf("%d portals", len(f.Portals))
	}
	// A flood fill from the first portal reaches every walkable cell.
	seen := make([]bool, n)
	p0 := f.Portals[0].Y*f.Width + f.Portals[0].X
	if !walkableType(f.Types[p0]) {
		t.Fatalf("portal %+v is not walkable", f.Portals[0])
	}
	stack := []int{p0}
	seen[p0] = true
	reached := 0
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		reached++
		x, y := c%f.Width, c/f.Width
		for _, d := range [4][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx, ny := x+d[0], y+d[1]
			if nx < 0 || ny < 0 || nx >= f.Width || ny >= f.Height {
				continue
			}
			j := ny*f.Width + nx
			if !seen[j] && walkableType(f.Types[j]) {
				seen[j] = true
				stack = append(stack, j)
			}
		}
	}
	if reached != walk {
		t.Errorf("%s: the first portal reaches %d of %d walkable cells", b.Name, reached, walk)
	}
	for _, p := range f.Portals {
		if !seen[p.Y*f.Width+p.X] {
			t.Errorf("%s: portal %+v is not reached", b.Name, p)
		}
	}
	for _, o := range f.Objects {
		if f.Types[o.Y*f.Width+o.X] != CellBlocked {
			t.Errorf("%s: prop %+v stands on a cell of type %d", b.Name, o, f.Types[o.Y*f.Width+o.X])
		}
	}
	checkWMAP(t, f)
}

// checkWMAP reads the CELL section of the field's WMAP and compares it
// with the grid.
func checkWMAP(t *testing.T, f Field) {
	t.Helper()
	w := f.WMAP
	if len(w) < 10 || string(w[:4]) != "WMAP" || binary.LittleEndian.Uint16(w[4:]) != 1 ||
		int(binary.LittleEndian.Uint16(w[6:])) != f.Width || int(binary.LittleEndian.Uint16(w[8:])) != f.Height {
		t.Fatalf("WMAP header % x", w[:min(len(w), 10)])
	}
	if string(w[10:14]) != "CELL" || int(binary.LittleEndian.Uint32(w[14:])) != 17*f.Width*f.Height {
		t.Fatalf("WMAP cell section %q", w[10:18])
	}
	cells := w[18:]
	for i, c := range f.Types {
		r := cells[17*i:]
		if r[0] != Flags(c) {
			t.Fatalf("cell %d: flags %d, want %d", i, r[0], Flags(c))
		}
		for k := range 4 {
			got := math.Float32frombits(binary.LittleEndian.Uint32(r[1+4*k:]))
			if want := float32(float64(f.Heights[4*i+k]) / 65536); got != want {
				t.Fatalf("cell %d corner %d: %v, want %v", i, k, got, want)
			}
		}
	}
	for _, tag := range []string{"GRND", "LMAP", "LGHT", "OBJS", "SNDS", "EFCT", "DEPS"} {
		if !bytes.Contains(w, []byte(tag)) {
			t.Errorf("WMAP has no %s section", tag)
		}
	}
}

// TestFieldV2 generates fields of every biome from many seeds and checks
// each, and that the same node gives the same field.
func TestFieldV2(t *testing.T) {
	g, ok := Lookup(2)
	if !ok || g.Field == nil || len(g.Biomes) != len(v2Biomes) {
		t.Fatalf("generator 2: %+v %v", g, ok)
	}
	root, err := rng.NewRoot([]byte("field-test"))
	if err != nil {
		t.Fatal(err)
	}
	public := PublicBranch(root)
	retried := 0
	for b := range g.Biomes {
		for i := range 8 {
			node := FieldNode(public, int64(i), int64(b))
			p := Params{Width: 96 + 16*i, Height: 128 - 8*i, Biome: b}
			f, err := g.Field(node, p)
			if err != nil {
				t.Errorf("%s %d: %v", g.Biomes[b], i, err)
				continue
			}
			if f.Biome != b || f.Width != p.Width || f.Height != p.Height {
				t.Errorf("field %d %d×%d, want %d %d×%d", f.Biome, f.Width, f.Height, b, p.Width, p.Height)
			}
			if f.Attempt > 0 {
				retried++
			}
			checkField(t, f)
			again, err := g.Field(node, p)
			if err != nil || again.Checksum() != f.Checksum() || !bytes.Equal(again.WMAP, f.WMAP) {
				t.Errorf("%s %d: a second run differs: %v", g.Biomes[b], i, err)
			}
			if grid, err := g.Grid(node, p); err != nil || grid.Checksum() != f.Checksum() {
				t.Errorf("%s %d: Grid differs from Field: %v", g.Biomes[b], i, err)
			}
		}
	}
	t.Logf("%d of %d fields needed a retry", retried, 8*len(g.Biomes))
}

// TestFieldV2Retries: an attempt that fails validation is made again from
// attempt(n), at most v2Attempts times, and then the field is an error
// that lists each attempt.
func TestFieldV2Retries(t *testing.T) {
	saved := v2Biomes[biomeGrassland]
	defer func() { v2Biomes[biomeGrassland] = saved }()
	node := rng.FromKey([32]byte{9})
	p := Params{Width: 64, Height: 64, Biome: biomeGrassland}

	// No field can be walkable and not walkable at once.
	v2Biomes[biomeGrassland].MinWalk, v2Biomes[biomeGrassland].MaxWalk = 1000, 0
	_, err := fieldV2(node, p)
	if !errors.Is(err, ErrNoField) {
		t.Fatalf("error %v, want ErrNoField", err)
	}
	if got := strings.Count(err.Error(), "; attempt ") + 1; got != v2Attempts {
		t.Errorf("%d attempts in %q, want %d", got, err, v2Attempts)
	}

	// An attempt that passes is the field the attempt's node gives.
	v2Biomes[biomeGrassland] = saved
	for k := range 32 {
		n := rng.FromKey([32]byte{byte(k), 1})
		f, err := fieldV2(n, Params{Width: 32, Height: 32, Biome: biomeGrassland})
		if err != nil || f.Attempt == 0 {
			continue
		}
		m := newV2Map(&v2Biomes[biomeGrassland], 32, 32, new(v2Buffers))
		m.generate(n.Child("attempt", int64(f.Attempt)))
		if got := m.field(); m.validate() != "" || got.Checksum() != f.Checksum() {
			t.Errorf("attempt %d does not match the field", f.Attempt)
		}
		return
	}
	t.Error("no 32 × 32 grassland needed a retry")
}

// TestFieldV2Inputs: sizes below the smallest field, unknown biomes and
// sizes over 1024 are errors.
func TestFieldV2Inputs(t *testing.T) {
	g, _ := Lookup(2)
	node := rng.FromKey([32]byte{3})
	for _, c := range []struct {
		p   Params
		err error
	}{
		{Params{Width: 31, Height: 64}, ErrFieldSize},
		{Params{Width: 64, Height: 1}, ErrFieldSize},
		{Params{Width: 1025, Height: 64}, ErrSize},
		{Params{Width: 64, Height: 64, Biome: -1}, ErrBiome},
		{Params{Width: 64, Height: 64, Biome: len(v2Biomes)}, ErrBiome},
	} {
		if _, err := g.Field(node, c.p); !errors.Is(err, c.err) {
			t.Errorf("%+v: %v, want %v", c.p, err, c.err)
		}
	}
	if i, ok := g.BiomeIndex("desert"); !ok || g.Biomes[i] != "desert" {
		t.Errorf("BiomeIndex(desert) = %d, %v", i, ok)
	}
	if _, ok := g.BiomeIndex("moon"); ok {
		t.Error("BiomeIndex found moon")
	}
}

// TestClimateV2: every biome occurs, and a field's biome does not depend
// on the order of the calls.
func TestClimateV2(t *testing.T) {
	g, _ := Lookup(2)
	public := PublicBranch(rng.FromKey([32]byte{5}))
	seen := make([]int, len(g.Biomes))
	for y := int64(-40); y < 40; y++ {
		for x := int64(-40); x < 40; x++ {
			seen[g.Climate(public, x, y)]++
		}
	}
	for b, n := range seen {
		if n == 0 {
			t.Errorf("no field of biome %s", g.Biomes[b])
		}
	}
	first := g.Climate(public, 3, -1)
	g.Climate(public, -9, 4)
	if again := g.Climate(public, 3, -1); again != first {
		t.Errorf("the climate of 3,-1 changed from %d to %d", first, again)
	}
}

// TestF32Bits compares the integer encoding with the hardware's.
func TestF32Bits(t *testing.T) {
	vals := []int64{0, 1, -1, 65536, -65536, 46341, 1 << 40, -(1 << 40) + 7, 0x7fffffff, 0x123456789, 3 << 30}
	for i := range int64(2000) {
		vals = append(vals, i*i*i*7919-1<<35, i*104729)
	}
	for _, v := range vals {
		for _, frac := range []uint{0, 1, 16, 32} {
			want := math.Float32bits(float32(float64(v) / float64(uint64(1)<<frac)))
			if got := f32bits(v, frac); got != want {
				t.Errorf("f32bits(%d, %d) = %#x, want %#x", v, frac, got, want)
			}
		}
	}
}

// TestFieldIDs: a field's map ID names its coordinates.
func TestFieldIDs(t *testing.T) {
	for _, c := range [][2]int64{{0, 0}, {3, -1}, {-1 << 40, 1 << 40}} {
		id := FieldID(c[0], c[1])
		x, y, ok := ParseFieldID(id)
		if !ok || x != c[0] || y != c[1] || !IsGenerated(id) {
			t.Errorf("%s: %d %d %v", id, x, y, ok)
		}
	}
	for _, id := range []string{"gen:field:1", "gen:field:a:1", "gen:field:01:1", "map/prontera", "gen:field:1:2:3", "gen:field:+1:2"} {
		if _, _, ok := ParseFieldID(id); ok {
			t.Errorf("%s parsed", id)
		}
	}
	if IsGenerated("map/prontera") || IsGenerated("placeholder/map") {
		t.Error("an asset name is generated")
	}
}

// FuzzFieldV2 generates fields from any key, size and biome. Each is a
// valid field or ErrNoField, never a panic or a hang.
func FuzzFieldV2(f *testing.F) {
	f.Add([]byte{1}, uint16(32), uint16(32), uint8(0))
	f.Add([]byte("seed"), uint16(100), uint16(40), uint8(3))
	f.Add(make([]byte, 32), uint16(64), uint16(200), uint8(5))
	f.Fuzz(func(t *testing.T, key []byte, w, h uint16, biome uint8) {
		var k [32]byte
		copy(k[:], key)
		p := Params{
			Width:  v2MinSize + int(w)%160,
			Height: v2MinSize + int(h)%160,
			Biome:  int(biome) % len(v2Biomes),
		}
		field, err := fieldV2(rng.FromKey(k), p)
		if err != nil {
			if !errors.Is(err, ErrNoField) {
				t.Fatalf("%+v: %v", p, err)
			}
			return
		}
		checkField(t, field)
	})
}

// BenchmarkFieldV2 times a 400 × 400 field of each biome.
func BenchmarkFieldV2(b *testing.B) {
	g, _ := Lookup(2)
	for b.Loop() {
		for bi := range g.Biomes {
			if _, err := g.Field(rng.FromKey([32]byte{1, byte(bi)}), Params{Width: 400, Height: 400, Biome: bi}); err != nil {
				b.Fatal(err)
			}
		}
	}
}
