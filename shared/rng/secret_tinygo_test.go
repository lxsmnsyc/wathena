//go:build tinygo

package rng

import "testing"

// computeSecret keeps the expected ChaCha8 outputs, because TinyGo builds
// leave out SecretStream.
func computeSecret(t *testing.T, v *vectors) {
	want := loadVectors(t)
	v.ChaCha8 = want.ChaCha8
}
