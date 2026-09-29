package rng

import (
	"encoding/binary"
	"math/bits"
)

// SplitMix64 is the SplitMix64 generator.
// Xoshiro uses it to replace an all-zero seed.
type SplitMix64 struct {
	state uint64
}

// Uint64 returns the next value.
func (s *SplitMix64) Uint64() uint64 {
	s.state += 0x9E3779B97F4A7C15
	return mix64(s.state)
}

// mix64 is the SplitMix64 finalizer. It is a bijection on 64-bit values.
func mix64(z uint64) uint64 {
	z = (z ^ z>>30) * 0xBF58476D1CE4E5B9
	z = (z ^ z>>27) * 0x94D049BB133111EB
	return z ^ z>>31
}

// Xoshiro is the xoshiro256** generator.
type Xoshiro struct {
	s [4]uint64
}

// NewXoshiro seeds a generator with four little-endian words from key.
// An all-zero key is replaced by the first four outputs of SplitMix64
// with state 0, because xoshiro cannot use a zero state.
func NewXoshiro(key [32]byte) *Xoshiro {
	x := &Xoshiro{}
	zero := true
	for i := range x.s {
		x.s[i] = binary.LittleEndian.Uint64(key[8*i:])
		if x.s[i] != 0 {
			zero = false
		}
	}
	if zero {
		var sm SplitMix64
		for i := range x.s {
			x.s[i] = sm.Uint64()
		}
	}
	return x
}

// Uint64 returns the next value.
func (x *Xoshiro) Uint64() uint64 {
	s := &x.s
	result := bits.RotateLeft64(s[1]*5, 7) * 9
	t := s[1] << 17
	s[2] ^= s[0]
	s[3] ^= s[1]
	s[1] ^= s[2]
	s[0] ^= s[3]
	s[2] ^= t
	s[3] = bits.RotateLeft64(s[3], 45)
	return result
}
