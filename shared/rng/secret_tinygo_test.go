//go:build tinygo

package rng

import "testing"

// computeSecret keeps the expected ChaCha8 outputs and secret hashes,
// because TinyGo builds leave out SecretStream and SecretHash.
func computeSecret(t *testing.T, v *vectors) {
	want := loadVectors(t)
	v.ChaCha8 = want.ChaCha8
	v.SecretHash = want.SecretHash
}
