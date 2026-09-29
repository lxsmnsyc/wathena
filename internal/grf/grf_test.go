package grf

import (
	"bytes"
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"
)

func sample() []WriteEntry {
	r := rand.New(rand.NewPCG(1, 2))
	noise := make([]byte, 5000)
	for i := range noise {
		noise[i] = byte(r.Uint32())
	}
	return []WriteEntry{
		{Name: `data\prontera.gat`, Data: bytes.Repeat([]byte("GRAT"), 1000)},
		{Name: `data\sprite\몬스터\poring.spr`, Data: []byte("SP poring")},
		{Name: `data\texture\유저인터페이스\login.bmp`, Data: noise},
		{Name: `data\mixed.bin`, Data: noise, Crypt: FlagMixedCrypt},
		{Name: `data\header.bin`, Data: noise[:300], Crypt: FlagHeaderCrypt},
		{Name: `data\empty.txt`, Data: nil},
	}
}

func build(t testing.TB, entries []WriteEntry) *Archive {
	t.Helper()
	var buf bytes.Buffer
	if err := Write(&buf, entries); err != nil {
		t.Fatal(err)
	}
	a, err := Open(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestRoundTrip(t *testing.T) {
	entries := sample()
	a := build(t, entries)
	if len(a.Entries()) != len(entries) {
		t.Fatalf("%d entries, want %d", len(a.Entries()), len(entries))
	}
	if a.BadNames != 0 {
		t.Errorf("%d bad names", a.BadNames)
	}
	for _, w := range entries {
		e, ok := a.Lookup(w.Name)
		if !ok {
			t.Fatalf("%q not found", w.Name)
		}
		if e.Name != w.Name {
			t.Errorf("name %q, want %q", e.Name, w.Name)
		}
		got, err := a.ReadFile(e)
		if err != nil {
			t.Fatalf("%s: %v", w.Name, err)
		}
		if !bytes.Equal(got, w.Data) && len(got)+len(w.Data) != 0 {
			t.Errorf("%s: data differs", w.Name)
		}
	}
}

func TestLookupIgnoresCaseAndSlashes(t *testing.T) {
	a := build(t, sample())
	if _, ok := a.Lookup("DATA/Prontera.GAT"); !ok {
		t.Error("lookup with other case and slashes failed")
	}
}

// Encryption must change the stored bytes, or the crypt tests prove nothing.
func TestCryptChangesData(t *testing.T) {
	data := bytes.Repeat([]byte{1, 2, 3, 4, 5, 6, 7, 8}, 400)
	for _, flag := range []uint8{FlagMixedCrypt, FlagHeaderCrypt} {
		enc := append([]byte(nil), data...)
		codeEntry(enc, flag, uint32(len(enc)), true)
		if bytes.Equal(enc, data) {
			t.Errorf("flag %#x: encryption left data unchanged", flag)
		}
		codeEntry(enc, flag, uint32(len(enc)), false)
		if !bytes.Equal(enc, data) {
			t.Errorf("flag %#x: decryption did not restore data", flag)
		}
	}
}

func TestMixedCycle(t *testing.T) {
	for _, c := range []struct {
		size uint32
		want int
	}{{9, 1}, {99, 1}, {100, 4}, {9999, 5}, {10000, 14}, {999999, 15}, {1000000, 22}, {1 << 31, 25}} {
		if got := mixedCycle(c.size); got != c.want {
			t.Errorf("mixedCycle(%d) = %d, want %d", c.size, got, c.want)
		}
	}
}

func TestRejects(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, sample()); err != nil {
		t.Fatal(err)
	}
	good := buf.Bytes()
	for name, mutate := range map[string]func([]byte) []byte{
		"short":         func(b []byte) []byte { return b[:20] },
		"magic":         func(b []byte) []byte { b[0] = 'X'; return b },
		"version":       func(b []byte) []byte { b[43] = 3; return b },
		"table offset":  func(b []byte) []byte { b[33] = 0x7f; return b },
		"truncated end": func(b []byte) []byte { return b[:len(b)-10] },
	} {
		b := mutate(append([]byte(nil), good...))
		if _, err := Open(bytes.NewReader(b), int64(len(b))); err == nil {
			t.Errorf("%s: Open accepted a bad file", name)
		}
	}
	b := append([]byte(nil), good...)
	b[43] = 3
	if _, err := Open(bytes.NewReader(b), int64(len(b))); !errors.Is(err, ErrVersion) {
		t.Errorf("version error is %v", err)
	}
}

// An entry whose recorded compressed size is too small still reads, and
// is counted as repaired.
func TestShortCompressedSize(t *testing.T) {
	var buf bytes.Buffer
	data := []byte("RIFF....WAVEfmt some sound data that is long enough to matter")
	if err := Write(&buf, []WriteEntry{{Name: "a.wav", Data: data}, {Name: "b.txt", Data: []byte("next")}}); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	a, err := Open(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	e, _ := a.Lookup("a.wav")
	// Record 6 fewer compressed bytes, as the damaged official entry does.
	e.CompressedSize -= 6
	got, err := a.ReadFile(e)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Error("lenient read returned wrong data")
	}
	if a.Repaired() != 1 {
		t.Errorf("repaired = %d, want 1", a.Repaired())
	}
	// A wrong uncompressed size still fails.
	e.Size++
	if _, err := a.ReadFile(e); err == nil {
		t.Error("read with a wrong size succeeded")
	}
}

func FuzzOpen(f *testing.F) {
	// A small seed keeps the fuzzer fast when it shrinks new inputs.
	var buf bytes.Buffer
	if err := Write(&buf, []WriteEntry{
		{Name: `data\a.txt`, Data: []byte("hello")},
		{Name: `data\몬스터.spr`, Data: []byte("SP"), Crypt: FlagHeaderCrypt},
	}); err != nil {
		f.Fatal(err)
	}
	f.Add(buf.Bytes())
	f.Fuzz(func(t *testing.T, b []byte) {
		a, err := Open(bytes.NewReader(b), int64(len(b)))
		if err != nil {
			return
		}
		for _, e := range a.Entries() {
			if e.IsFile() {
				_, _ = a.ReadFile(e)
			}
		}
	})
}

// TestRealClient reads a real GRF when WATHENA_GRF names one. Real game
// files are never in the repository or CI.
func TestRealClient(t *testing.T) {
	path := os.Getenv("WATHENA_GRF")
	if path == "" {
		t.Skip("set WATHENA_GRF to a GRF file to run")
	}
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	a, err := Open(f, st.Size())
	if err != nil {
		t.Fatal(err)
	}
	var files, crypt, failed int
	for _, e := range a.Entries() {
		if !e.IsFile() {
			continue
		}
		files++
		if e.Flags&(FlagMixedCrypt|FlagHeaderCrypt) != 0 {
			crypt++
			if _, err := a.ReadFile(e); err != nil {
				failed++
				t.Errorf("encrypted %s: %v", e.Name, err)
			}
		}
	}
	// Read a spread of entries, not all, to keep the test fast.
	step := max(files/2000, 1)
	for i, e := range a.Entries() {
		if e.IsFile() && i%step == 0 {
			if _, err := a.ReadFile(e); err != nil {
				failed++
				t.Errorf("%s: %v", e.Name, err)
			}
		}
	}
	t.Logf("%d entries, %d files, %d encrypted, %d bad names, %d read errors, %d repaired", len(a.Entries()), files, crypt, a.BadNames, failed, a.Repaired())
}
