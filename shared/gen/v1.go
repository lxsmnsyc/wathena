//go:build procedural

package gen

import "github.com/lxsmnsyc/wathena/shared/rng"

// Generator version 1 is a small grid that proves the pipeline and its
// determinism. Phase 7 step 2 adds biome terrain as a new version.
//
// It derives three children of the node it gets:
//
//   - "height": two octaves of value noise give each corner's height.
//   - "water": the first value of its stream sets the water level.
//   - "objects": a per-cell hash places rocks.

// Lattice spacings, as powers of two: 32 cells and 8 cells.
const (
	v1CoarseShift = 5
	v1FineShift   = 3
)

// gridV1 generates the grid of version 1.
func gridV1(node rng.Node, p Params) (Grid, error) {
	if err := p.Check(); err != nil {
		return Grid{}, err
	}
	height := node.Child("height")
	objects := node.Child("objects")
	level := rng.Fixed(node.Child("water").Stream().Range(-int64(rng.One), int64(rng.One/2))) // #nosec G115 -- the range fits in 32 bits.

	// Corner heights are shared between cells, so they are computed once
	// per vertex: (width + 1) × (height + 1) of them.
	vw := p.Width + 1
	vertex := make([]rng.Fixed, vw*(p.Height+1))
	for y := 0; y <= p.Height; y++ {
		for x := 0; x <= p.Width; x++ {
			vertex[y*vw+x] = v1Noise(height, x, y, v1CoarseShift, 0, 4*rng.One) +
				v1Noise(height, x, y, v1FineShift, 1, rng.One)
		}
	}

	g := Grid{
		Width:      p.Width,
		Height:     p.Height,
		WaterLevel: level,
		Types:      make([]uint8, p.Width*p.Height),
		Heights:    make([]rng.Fixed, 4*p.Width*p.Height),
	}
	for y := range p.Height {
		for x := range p.Width {
			i := y*p.Width + x
			c := [4]rng.Fixed{vertex[y*vw+x], vertex[y*vw+x+1], vertex[(y+1)*vw+x], vertex[(y+1)*vw+x+1]}
			copy(g.Heights[4*i:4*i+4], c[:])
			g.Types[i] = v1Type(c, level, objects.Hash(int64(x), int64(y), 0))
		}
	}
	return g, nil
}

// v1Type picks a cell's type from its corners:
//
//   - Water more than 1 cell deep, by the corners' mean, is blocked.
//   - Shallower water is walkable water.
//   - A cell whose corners differ by more than half a cell is a gap.
//   - One cell in 64, by the objects hash, is a rock and blocked.
//   - Any other cell is walkable.
func v1Type(c [4]rng.Fixed, level rng.Fixed, rock uint64) uint8 {
	lo, hi := c[0], c[0]
	var sum int64
	for _, h := range c {
		lo = min(lo, h)
		hi = max(hi, h)
		sum += int64(h)
	}
	mean := rng.Fixed(sum >> 2) // #nosec G115 -- the mean of four Fixed values is a Fixed value.
	switch {
	case mean < level-rng.One:
		return CellBlocked
	case mean < level:
		return CellWater
	case hi-lo > rng.One/2:
		return CellGap
	case rock%64 == 0:
		return CellBlocked
	}
	return CellWalkable
}

// v1Lattice returns a lattice point's value, in [-amp, amp).
func v1Lattice(n rng.Node, lx, ly, octave int64, amp rng.Fixed) rng.Fixed {
	// The hash's top 17 bits are a Q16.16 value in [0, 2).
	unit := rng.Fixed(n.Hash(lx, ly, octave) >> 47) // #nosec G115 -- 17 bits fit in 32.
	return unit.Mul(amp) - amp
}

// v1Noise interpolates the lattice of spacing 1 << shift bilinearly at a
// vertex. The lattice point (lx, ly) sits at vertex (lx << shift, ly << shift).
func v1Noise(n rng.Node, x, y int, shift uint, octave int64, amp rng.Fixed) rng.Fixed {
	lx, ly := int64(x>>shift), int64(y>>shift)
	mask := 1<<shift - 1
	tx := rng.Fixed((x & mask) << (16 - shift)) // #nosec G115 -- below 1.0 in Q16.16.
	ty := rng.Fixed((y & mask) << (16 - shift)) // #nosec G115 -- below 1.0 in Q16.16.
	a := v1Lattice(n, lx, ly, octave, amp)
	b := v1Lattice(n, lx+1, ly, octave, amp)
	c := v1Lattice(n, lx, ly+1, octave, amp)
	d := v1Lattice(n, lx+1, ly+1, octave, amp)
	south := a + (b - a).Mul(tx)
	north := c + (d - c).Mul(tx)
	return south + (north - south).Mul(ty)
}
