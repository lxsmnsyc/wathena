package rng

import "math/bits"

// Source is a stream of 64-bit values.
type Source interface {
	Uint64() uint64
}

// Rand adds the spec's helpers to a Source.
// The helpers work the same on public and secret streams.
type Rand struct {
	src Source
}

// New returns a Rand that reads from src.
func New(src Source) *Rand {
	return &Rand{src: src}
}

// Uint64 returns the next value from the source.
func (r *Rand) Uint64() uint64 {
	return r.src.Uint64()
}

// Uint64N returns a value in [0, n) with Lemire's multiply-and-reject method.
// A product whose low half is below (2^64 - n) mod n is rejected, and each
// rejection reads one more value. It panics if n is 0.
func (r *Rand) Uint64N(n uint64) uint64 {
	if n == 0 {
		panic("rng: Uint64N with n = 0")
	}
	hi, lo := bits.Mul64(r.src.Uint64(), n)
	if lo < n {
		threshold := -n % n
		for lo < threshold {
			hi, lo = bits.Mul64(r.src.Uint64(), n)
		}
	}
	return hi
}

// IntN returns a value in [0, n). It panics if n <= 0.
func (r *Rand) IntN(n int) int {
	if n <= 0 {
		panic("rng: IntN with n <= 0")
	}
	return int(r.Uint64N(uint64(n))) // #nosec G115 -- n > 0, and the result is below n.
}

// Range returns a value in [lo, hi]. It panics if lo > hi.
func (r *Rand) Range(lo, hi int64) int64 {
	if lo > hi {
		panic("rng: Range with lo > hi")
	}
	span := uint64(hi) - uint64(lo) + 1 // #nosec G115 -- the span is computed with 64-bit wrapping.
	if span == 0 {
		// The range covers every int64 value.
		return int64(r.src.Uint64()) // #nosec G115 -- every int64 value is in range.
	}
	return lo + int64(r.Uint64N(span)) // #nosec G115 -- lo plus an offset below the span wraps into [lo, hi].
}

// Shuffle runs Fisher-Yates from the last element down.
// Element i is swapped with element Uint64N(i+1).
func (r *Rand) Shuffle(n int, swap func(i, j int)) {
	for i := n - 1; i > 0; i-- {
		swap(i, int(r.Uint64N(uint64(i)+1))) // #nosec G115 -- i >= 1, and the result is at most i.
	}
}

// Weighted returns an index with probability weight/total.
// It draws Uint64N(total) and returns the first index whose cumulative
// weight is above the draw. It panics if the total is 0.
func (r *Rand) Weighted(weights []uint32) int {
	var total uint64
	for _, w := range weights {
		total += uint64(w)
	}
	v := r.Uint64N(total)
	var sum uint64
	for i, w := range weights {
		sum += uint64(w)
		if v < sum {
			return i
		}
	}
	panic("rng: unreachable")
}

// Chance returns true with probability p, where p is a Q16.16 value
// from 0 (never) to One (always). It draws Uint64N(One).
func (r *Rand) Chance(p Fixed) bool {
	return r.Uint64N(uint64(One)) < uint64(max(p, 0))
}
