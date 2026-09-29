package rng

import (
	"bytes"
	"slices"
	"testing"
)

// Different inputs to Child must give different HMAC messages.
func FuzzChildMessage(f *testing.F) {
	f.Add("ab", int64(0), "a", int64(0), true)
	f.Add("region", int64(3), "region", int64(-1), false)
	f.Fuzz(func(t *testing.T, l1 string, i1 int64, l2 string, i2 int64, noIdx bool) {
		idx1, idx2 := []int64{i1}, []int64{i1, i2}
		if noIdx {
			idx1 = nil
		}
		m1, m2 := childMessage(l1, idx1), childMessage(l2, idx2)
		same := l1 == l2 && slices.Equal(idx1, idx2)
		if bytes.Equal(m1, m2) != same {
			t.Errorf("(%q, %v) and (%q, %v): equal messages = %v", l1, idx1, l2, idx2, !same)
		}
	})
}

func FuzzUint64N(f *testing.F) {
	f.Add(uint64(1), uint64(6))
	f.Add(uint64(7), uint64(1<<63+1))
	f.Fuzz(func(t *testing.T, seed, n uint64) {
		if n == 0 {
			return
		}
		var key [32]byte
		key[0] = byte(seed)
		key[8] = byte(seed >> 8)
		a, b := FromKey(key).Stream(), FromKey(key).Stream()
		for range 4 {
			v := a.Uint64N(n)
			if v >= n {
				t.Fatalf("Uint64N(%d) = %d", n, v)
			}
			if w := b.Uint64N(n); w != v {
				t.Fatalf("Uint64N is not deterministic: %d and %d", v, w)
			}
		}
	})
}

func FuzzWeighted(f *testing.F) {
	f.Add([]byte{0, 5, 0, 3})
	f.Fuzz(func(t *testing.T, raw []byte) {
		weights := make([]uint32, len(raw))
		var total uint32
		for i, w := range raw {
			weights[i] = uint32(w)
			total += uint32(w)
		}
		if total == 0 {
			return
		}
		r := FromKey([32]byte{1}).Stream()
		for range 8 {
			i := r.Weighted(weights)
			if weights[i] == 0 {
				t.Fatalf("Weighted picked index %d with weight 0", i)
			}
		}
	})
}
