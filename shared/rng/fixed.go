package rng

// Fixed is a Q16.16 fixed-point number: 16 integer bits and 16 fraction bits.
// Generation uses it instead of floating point, so results match on every
// platform.
type Fixed int32

// One is 1.0.
const One Fixed = 1 << 16

// FromInt returns i as a Fixed. Values outside the int16 range overflow.
func FromInt(i int32) Fixed {
	return Fixed(i << 16)
}

// Int returns the integer part, rounded toward negative infinity.
func (f Fixed) Int() int32 {
	return int32(f >> 16)
}

// Mul returns f * g, rounded toward negative infinity.
// A result outside the Fixed range wraps to 32 bits.
func (f Fixed) Mul(g Fixed) Fixed {
	return Fixed((int64(f) * int64(g)) >> 16) // #nosec G115 -- the spec wraps results to 32 bits.
}

// Div returns f / g, rounded toward zero. It panics if g is 0.
// A result outside the Fixed range wraps to 32 bits.
func (f Fixed) Div(g Fixed) Fixed {
	return Fixed((int64(f) << 16) / int64(g)) // #nosec G115 -- the spec wraps results to 32 bits.
}
