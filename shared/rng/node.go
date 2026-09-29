// Package rng implements the seed tree and random streams in docs/specs/rng-v1.md.
// World generation uses it on the server and in the web client, so every
// result must match the spec's test vectors exactly.
package rng

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"math"
)

// Version is the spec version. Each world records it.
const Version = "rng-v1"

const (
	rootKey      = "wa-seed-v1-root"
	childContext = "wa-seed-v1"
)

// Node is one point in the seed tree.
// Its key is secret for secret branches, so String does not print it.
type Node struct {
	key [32]byte
}

// NewRoot derives the root node from a world secret.
func NewRoot(secret []byte) (Node, error) {
	if len(secret) == 0 {
		return Node{}, errors.New("rng: empty world secret")
	}
	mac := hmac.New(sha256.New, []byte(rootKey))
	mac.Write(secret)
	var n Node
	mac.Sum(n.key[:0])
	return n, nil
}

// FromKey returns the node with the given key.
func FromKey(key [32]byte) Node {
	return Node{key: key}
}

// Key returns the node's key.
func (n Node) Key() [32]byte {
	return n.key
}

// String hides the key, so logging a node does not leak it.
func (n Node) String() string {
	return "rng.Node{redacted}"
}

// GoString hides the key from %#v.
func (n Node) GoString() string {
	return n.String()
}

// Child derives a child node with HMAC-SHA256.
// A child key does not reveal its parent key or its siblings' keys.
func (n Node) Child(label string, idx ...int64) Node {
	mac := hmac.New(sha256.New, n.key[:])
	mac.Write(childMessage(label, idx))
	var c Node
	mac.Sum(c.key[:0])
	return c
}

// childMessage encodes the HMAC message for Child.
// The label has a length prefix, so no two inputs give the same message.
func childMessage(label string, idx []int64) []byte {
	if uint64(len(label)) > math.MaxUint32 {
		panic("rng: label too long")
	}
	msg := make([]byte, 0, len(childContext)+4+len(label)+8*len(idx))
	msg = append(msg, childContext...)
	msg = binary.BigEndian.AppendUint32(msg, uint32(len(label))) // #nosec G115 -- the length is checked above.
	msg = append(msg, label...)
	for _, i := range idx {
		msg = binary.BigEndian.AppendUint64(msg, uint64(i)) // #nosec G115 -- the spec encodes indexes as two's complement.
	}
	return msg
}

// Stream returns a xoshiro256** stream for public generation.
func (n Node) Stream() *Rand {
	return New(NewXoshiro(n.key))
}

// Hash constants. All are odd.
const (
	hashP1 = 0x9E3779B97F4A7C15
	hashP2 = 0xC2B2AE3D27D4EB4F
	hashP3 = 0x165667B19E3779F9
)

// Hash returns a stateless value for a coordinate.
// Noise and per-cell choices use it, so any cell can be computed alone.
func (n Node) Hash(x, y, z int64) uint64 {
	h := n.hashKey()
	h = mix64(h ^ uint64(x)*hashP1)    // #nosec G115 -- the spec hashes coordinates as two's complement.
	h = mix64(h ^ uint64(y)*hashP2)    // #nosec G115 -- the spec hashes coordinates as two's complement.
	return mix64(h ^ uint64(z)*hashP3) // #nosec G115 -- the spec hashes coordinates as two's complement.
}

// hashKey folds the key into 64 bits by XOR of its four little-endian words.
func (n Node) hashKey() uint64 {
	var k uint64
	for i := 0; i < 32; i += 8 {
		k ^= binary.LittleEndian.Uint64(n.key[i:])
	}
	return k
}
