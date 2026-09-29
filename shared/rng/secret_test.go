//go:build !tinygo

package rng

import "testing"

func computeSecret(t *testing.T, v *vectors) {
	for i := range v.ChaCha8 {
		c := &v.ChaCha8[i]
		c.Outputs = outputs(mustKey(t, c.Key).SecretStream(), 4)
	}
}
