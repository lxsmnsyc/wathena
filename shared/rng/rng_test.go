package rng

import (
	"bytes"
	"fmt"
	"runtime"
	"slices"
	"testing"
)

func testRoot(t *testing.T) Node {
	t.Helper()
	n, err := NewRoot([]byte("wathena"))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestEmptySecret(t *testing.T) {
	if _, err := NewRoot(nil); err == nil {
		t.Error("NewRoot accepted an empty secret")
	}
}

// Deriving nodes in any order gives the same keys.
func TestOrderDoesNotMatter(t *testing.T) {
	root := testRoot(t)
	a := root.Child("region", 3, -1)
	b := root.Child("region", 0, 0)
	if root.Child("region", 0, 0) != b || root.Child("region", 3, -1) != a {
		t.Error("child keys depend on derivation order")
	}
	if a == b {
		t.Error("sibling keys are equal")
	}
}

func TestStringHidesKey(t *testing.T) {
	n := testRoot(t)
	key := n.Key()
	for _, s := range []string{fmt.Sprint(n), fmt.Sprintf("%v %+v %#v", n, n, n)} {
		if bytes.Contains([]byte(s), key[:4]) || bytes.Contains([]byte(s), []byte(fmt.Sprintf("%x", key[:4]))) {
			t.Errorf("formatted node shows its key: %s", s)
		}
	}
}

func TestZeroKeyStream(t *testing.T) {
	x := NewXoshiro([32]byte{})
	if x.s == [4]uint64{} {
		t.Fatal("zero key gave a zero state")
	}
}

func TestHelpersPanic(t *testing.T) {
	if runtime.Compiler == "tinygo" && runtime.GOARCH == "wasm" {
		// t.Skip does not stop the test there either, so return instead.
		t.Log("TinyGo on WebAssembly cannot recover from panics")
		return
	}
	r := testRoot(t).Stream()
	for name, f := range map[string]func(){
		"Uint64N(0)":    func() { r.Uint64N(0) },
		"IntN(0)":       func() { r.IntN(0) },
		"Range(1, 0)":   func() { r.Range(1, 0) },
		"Weighted(nil)": func() { r.Weighted(nil) },
		"Weighted(0)":   func() { r.Weighted([]uint32{0, 0}) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s did not panic", name)
				}
			}()
			f()
		}()
	}
}

func TestFullRange(t *testing.T) {
	r := testRoot(t).Stream()
	const lo, hi = -1 << 63, 1<<63 - 1
	seen := map[bool]bool{}
	for range 64 {
		seen[r.Range(lo, hi) < 0] = true
	}
	if len(seen) != 2 {
		t.Error("full range never gave both signs")
	}
}

func TestShufflePermutes(t *testing.T) {
	r := testRoot(t).Stream()
	a := make([]int, 100)
	for i := range a {
		a[i] = i
	}
	r.Shuffle(len(a), func(i, j int) { a[i], a[j] = a[j], a[i] })
	b := slices.Clone(a)
	slices.Sort(b)
	for i := range b {
		if b[i] != i {
			t.Fatal("shuffle lost or duplicated an element")
		}
	}
}

// Uint64N must be close to uniform for a small bound.
func TestUint64NUniform(t *testing.T) {
	r := testRoot(t).Stream()
	const n, draws = 6, 60000
	var counts [n]int
	for range draws {
		counts[r.Uint64N(n)]++
	}
	for i, c := range counts {
		if c < draws/n*9/10 || c > draws/n*11/10 {
			t.Errorf("value %d drawn %d times in %d", i, c, draws)
		}
	}
}

func BenchmarkXoshiro(b *testing.B) {
	x := NewXoshiro(testRootKey())
	for b.Loop() {
		x.Uint64()
	}
}

func BenchmarkHash(b *testing.B) {
	n := FromKey(testRootKey())
	var i int64
	for b.Loop() {
		n.Hash(i, -i, 0)
		i++
	}
}

func BenchmarkChild(b *testing.B) {
	n := FromKey(testRootKey())
	for b.Loop() {
		n.Child("region", 3, -1)
	}
}

func testRootKey() [32]byte {
	var k [32]byte
	for i := range k {
		k[i] = byte(i)
	}
	return k
}
