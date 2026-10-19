//go:build !tinygo

package rng

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"testing"
)

func computeSecret(t *testing.T, v *vectors) {
	for i := range v.ChaCha8 {
		c := &v.ChaCha8[i]
		c.Outputs = outputs(mustKey(t, c.Key).SecretStream(), 4)
	}
	for i := range v.SecretHash {
		h := &v.SecretHash[i]
		h.Value = u64hex(mustKey(t, h.Key).SecretHash(h.X, h.Y))
	}
}

// TestSecretHash checks that a secret node's hash is HMAC-SHA256 of the
// cell keyed by the node, that it differs from the invertible Hash, and
// that another key gives another value (review decision 54).
func TestSecretHash(t *testing.T) {
	n := Node{}
	for i := range n.key {
		n.key[i] = byte(i)
	}
	for _, c := range [][2]int64{{0, 0}, {1, 0}, {0, 1}, {-1, -1}, {123, 456}} {
		got := n.SecretHash(c[0], c[1])
		msg := []byte("wa-secret-hash-v1")
		msg = binary.BigEndian.AppendUint64(msg, uint64(c[0]))
		msg = binary.BigEndian.AppendUint64(msg, uint64(c[1]))
		mac := hmac.New(sha256.New, n.key[:])
		mac.Write(msg)
		if want := binary.BigEndian.Uint64(mac.Sum(nil)); got != want {
			t.Errorf("SecretHash(%d, %d) = %016x, want %016x", c[0], c[1], got, want)
		}
		if old := n.Hash(c[0], c[1], 0); got == old {
			t.Errorf("SecretHash(%d, %d) = %016x, the same as the public Hash", c[0], c[1], got)
		}
		// A key that folds to the same 64 bits gives the same public
		// Hash, so the public Hash depends only on the fold. SecretHash
		// depends on the whole key.
		other := n
		other.key[0] ^= 1
		other.key[8] ^= 1
		if n.Hash(c[0], c[1], 0) != other.Hash(c[0], c[1], 0) {
			t.Fatal("the public Hash of keys with the same fold differs")
		}
		if other.SecretHash(c[0], c[1]) == got {
			t.Errorf("SecretHash(%d, %d) is the same for another key", c[0], c[1])
		}
	}
	if n.SecretHash(0, 1) == n.SecretHash(1, 0) {
		t.Error("SecretHash mixes up x and y")
	}
}
