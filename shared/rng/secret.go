//go:build !tinygo

package rng

import "math/rand/v2"

// SecretStream returns a ChaCha8 stream for secret branches.
// Players cannot recover its state from outcomes they see.
// It is left out of TinyGo builds, because only the server uses it.
func (n Node) SecretStream() *Rand {
	return New(rand.NewChaCha8(n.key))
}
