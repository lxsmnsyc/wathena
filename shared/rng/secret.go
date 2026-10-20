//go:build !tinygo

package rng

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"math/rand/v2"
)

// SecretStream returns a ChaCha8 stream for secret branches.
// Players cannot recover its state from outcomes they see.
// It is left out of TinyGo builds, because only the server uses it.
func (n Node) SecretStream() *Rand {
	return New(rand.NewChaCha8(n.key))
}

// secretHashContext starts the HMAC message of SecretHash. It differs from
// childContext, so a hash message is never a child message.
const secretHashContext = "wa-secret-hash-v1" // #nosec G101 -- a label of the HMAC message, not a secret.

// SecretHash returns a stateless value for a cell of a secret node: the
// first 8 bytes of HMAC-SHA256 of the cell, keyed by the node, read
// big-endian (review decision 54). Unlike Hash, it cannot be inverted to
// recover the key, and it cannot be computed without the key.
// It is left out of TinyGo builds, because only the server uses it.
func (n Node) SecretHash(x, y int64) uint64 {
	mac := hmac.New(sha256.New, n.key[:])
	mac.Write(secretHashMessage(x, y))
	var sum [sha256.Size]byte
	return binary.BigEndian.Uint64(mac.Sum(sum[:0]))
}

// secretHashMessage encodes the HMAC message for SecretHash.
func secretHashMessage(x, y int64) []byte {
	msg := make([]byte, 0, len(secretHashContext)+16)
	msg = append(msg, secretHashContext...)
	msg = binary.BigEndian.AppendUint64(msg, uint64(x)) // #nosec G115 -- the spec encodes coordinates as two's complement.
	msg = binary.BigEndian.AppendUint64(msg, uint64(y)) // #nosec G115 -- the spec encodes coordinates as two's complement.
	return msg
}
