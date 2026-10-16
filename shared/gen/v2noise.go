//go:build procedural

package gen

import "github.com/lxsmnsyc/wathena/shared/rng"

// Gradient noise in Q16.16 for generator version 2.
//
// A lattice has one gradient per point, picked by the node's hash from
// 8 directions whose parts are -1, 0 or 1. Every point of an octave is
// hashed once, so a sample costs a few additions and multiplications.

// v2Grad holds the gradient directions: 4 axes, then 4 diagonals.
var v2Grad = [8][2]int64{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {1, 1}, {-1, 1}, {1, -1}, {-1, -1}}

// v2Lattice is one octave's lattice over a range of cells.
type v2Lattice struct {
	// shift is the spacing, 1 << shift cells.
	shift uint
	// x0 and y0 are the lattice indexes of the first point stored.
	x0, y0 int64
	lw, lh int64
	grad   []uint8
}

// newV2Lattice hashes the lattice that covers cells from (minX, minY) to
// (maxX, maxY), both included.
func newV2Lattice(n rng.Node, octave int64, shift uint, minX, minY, maxX, maxY int64) v2Lattice {
	l := v2Lattice{shift: shift, x0: minX >> shift, y0: minY >> shift}
	l.lw = maxX>>shift - l.x0 + 2
	l.lh = maxY>>shift - l.y0 + 2
	l.grad = make([]uint8, l.lw*l.lh)
	for j := range l.lh {
		for i := range l.lw {
			l.grad[j*l.lw+i] = uint8(n.Hash(l.x0+i, l.y0+j, octave) >> 61) // #nosec G115 -- the top 3 bits.
		}
	}
	return l
}

// v2Fade is the quintic 6t^5 - 15t^4 + 10t^3 for t in [0, 1].
func v2Fade(t int64) int64 {
	t3 := t * t >> 16 * t >> 16
	inner := (6*t-15*int64(rng.One))*t>>16 + 10*int64(rng.One)
	return t3 * inner >> 16
}

// sample returns the noise at a point given in Q16.16 cells, about -1 to
// 1. Points outside the lattice use its nearest points.
func (l *v2Lattice) sample(px, py int64) int64 {
	ix := px>>(16+l.shift) - l.x0
	iy := py>>(16+l.shift) - l.y0
	fx := px >> l.shift & 0xffff
	fy := py >> l.shift & 0xffff
	if ix < 0 {
		ix, fx = 0, 0
	} else if ix > l.lw-2 {
		ix, fx = l.lw-2, int64(rng.One)
	}
	if iy < 0 {
		iy, fy = 0, 0
	} else if iy > l.lh-2 {
		iy, fy = l.lh-2, int64(rng.One)
	}
	i := iy*l.lw + ix
	one := int64(rng.One)
	g00, g10 := v2Grad[l.grad[i]], v2Grad[l.grad[i+1]]
	g01, g11 := v2Grad[l.grad[i+l.lw]], v2Grad[l.grad[i+l.lw+1]]
	d00 := g00[0]*fx + g00[1]*fy
	d10 := g10[0]*(fx-one) + g10[1]*fy
	d01 := g01[0]*fx + g01[1]*(fy-one)
	d11 := g11[0]*(fx-one) + g11[1]*(fy-one)
	u, v := v2Fade(fx), v2Fade(fy)
	south := d00 + (d10-d00)*u>>16
	north := d01 + (d11-d01)*u>>16
	return south + (north-south)*v>>16
}

// v2Value is value noise for slow changes, such as density and patches:
// a lattice of hashed values in [0, 1) with bilinear steps.
type v2Value struct {
	n      rng.Node
	shift  uint
	octave int64
}

// at returns the value at cell (x, y), in [0, 1) as Q16.16.
func (v v2Value) at(x, y int64) int64 {
	lx, ly := x>>v.shift, y>>v.shift
	mask := int64(1)<<v.shift - 1
	tx := (x & mask) << (16 - v.shift)
	ty := (y & mask) << (16 - v.shift)
	p := func(i, j int64) int64 { return int64(v.n.Hash(lx+i, ly+j, v.octave) >> 48) } // #nosec G115 -- 16 bits.
	south := p(0, 0) + (p(1, 0)-p(0, 0))*tx>>16
	north := p(0, 1) + (p(1, 1)-p(0, 1))*tx>>16
	return south + (north-south)*ty>>16
}

// grid writes the value of every cell of a w × h map into out, row by
// row, hashing each lattice point once.
func (v v2Value) grid(w, h int, out []rng.Fixed) {
	s := int64(1) << v.shift
	lw, lh := int64(w)>>v.shift+2, int64(h)>>v.shift+2
	lat := make([]int64, lw*lh)
	for j := range lh {
		for i := range lw {
			lat[j*lw+i] = int64(v.n.Hash(i, j, v.octave) >> 48) // #nosec G115 -- 16 bits.
		}
	}
	for y := range int64(h) {
		ly, ty := y>>v.shift, (y&(s-1))<<(16-v.shift)
		for x := range int64(w) {
			lx, tx := x>>v.shift, (x&(s-1))<<(16-v.shift)
			i := ly*lw + lx
			south := lat[i] + (lat[i+1]-lat[i])*tx>>16
			north := lat[i+lw] + (lat[i+lw+1]-lat[i+lw])*tx>>16
			out[y*int64(w)+x] = rng.Fixed(south + (north-south)*ty>>16) // #nosec G115 -- below 1.0.
		}
	}
}
