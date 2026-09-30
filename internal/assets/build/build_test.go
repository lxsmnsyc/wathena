package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/andybalholm/brotli"

	"github.com/lxsmnsyc/wathena/internal/assets"
	"github.com/lxsmnsyc/wathena/internal/assets/wspr"
	"github.com/lxsmnsyc/wathena/internal/grf"
)

// fakeSource provides fixed items.
type fakeSource struct {
	name  string
	items []Item
}

func (f fakeSource) Name() string           { return f.name }
func (f fakeSource) Items() ([]Item, error) { return f.items, nil }

func item(name, body string) Item {
	return Item{Name: name, Ext: "wspr", CacheKey: "fake|" + name + "|" + body, Produce: func() ([]byte, error) { return []byte(body), nil }}
}

func readManifest(t *testing.T, out string) (Manifest, map[string]entry) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	all := map[string]entry{}
	for i, h := range m.Shards {
		root, err := os.OpenRoot(out)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := readJSON(root, h)
		_ = root.Close()
		if err != nil {
			t.Fatalf("shard %d: %v", i, err)
		}
		var sh map[string]entry
		if err := json.Unmarshal(raw, &sh); err != nil {
			t.Fatal(err)
		}
		for n, e := range sh {
			if ShardOf(n) != i {
				t.Errorf("%s is in shard %d, want %d", n, i, ShardOf(n))
			}
			all[n] = e
		}
	}
	return m, all
}

func TestLayeringAndOutput(t *testing.T) {
	out := t.TempDir()
	rep, err := Run(Options{Out: out, Brotli: 5, Sources: []Source{
		fakeSource{"a", []Item{item("sprite/mob/a", "A1"), item("sprite/mob/b", "B1")}},
		fakeSource{"b", []Item{item("sprite/mob/b", "B2"), item("Bad/Name", "x")}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Assets != 2 || rep.Sources[1].Replaced != 1 || len(rep.Rejected) != 1 {
		t.Fatalf("report %+v", rep)
	}
	m, all := readManifest(t, out)
	if m.Contract != assets.Contract {
		t.Errorf("contract %q", m.Contract)
	}
	e := all["sprite/mob/b"]
	if data := readAsset(t, out, e.H, "wspr"); string(data) != "B2" {
		t.Errorf("sprite/mob/b is %q, want the later source's B2", data)
	}

	// A second build takes everything from the cache.
	rep, err = Run(Options{Out: out, Brotli: 5, Sources: []Source{
		fakeSource{"a", []Item{item("sprite/mob/a", "A1"), item("sprite/mob/b", "B1")}},
		fakeSource{"b", []Item{item("sprite/mob/b", "B2")}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Converted != 0 || rep.Cached != 2 {
		t.Errorf("second build converted %d, cached %d", rep.Converted, rep.Cached)
	}
}

// TestPlaceholderAndGRF builds from the placeholder pack and a synthetic
// client folder, the same path an operator's build takes.
func TestPlaceholderAndGRF(t *testing.T) {
	dir := t.TempDir()
	spr, act := testSprite()
	var buf bytes.Buffer
	if err := grf.Write(&buf, []grf.WriteEntry{
		{Name: `data\sprite\npc\poring.spr`, Data: spr},
		{Name: `data\sprite\npc\poring.act`, Data: act},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.grf"), buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "DATA.INI"), []byte("[Data]\n1=data.grf\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	g := &GRF{Dir: dir}
	defer func() { _ = g.Close() }()
	out := t.TempDir()
	rep, err := Run(Options{Out: out, Brotli: 5, Sources: []Source{Placeholder{}, g}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Rejected) != 0 {
		t.Fatalf("rejected %v", rep.Rejected)
	}
	_, all := readManifest(t, out)
	for _, name := range []string{"placeholder/sprite", "sprite/npc/poring"} {
		e, ok := all[name]
		if !ok {
			t.Fatalf("%s missing from manifest", name)
		}
		if _, err := wspr.Decode(readAsset(t, out, e.H, "wspr")); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// readAsset reads a stored asset, decompresses it, and checks its hash.
func readAsset(t *testing.T, out, h, ext string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(out, StoredPath(h, ext)))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(brotli.NewReader(bytes.NewReader(raw)))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != h {
		t.Fatalf("asset %s does not match its hash", h)
	}
	return data
}

// testSprite returns a minimal spr 2.1 and act 2.5 pair.
func testSprite() ([]byte, []byte) {
	var s bytes.Buffer
	s.WriteString("SP")
	s.Write([]byte{1, 2})
	_ = binary.Write(&s, binary.LittleEndian, []uint16{1, 0, 2, 1, 2})
	s.Write([]byte{5, 6})
	s.Write(make([]byte, 1024))
	var a bytes.Buffer
	a.WriteString("AC")
	a.Write([]byte{5, 2})
	_ = binary.Write(&a, binary.LittleEndian, uint16(1))
	a.Write(make([]byte, 10))
	_ = binary.Write(&a, binary.LittleEndian, uint32(1))
	a.Write(make([]byte, 32))
	_ = binary.Write(&a, binary.LittleEndian, uint32(1))
	_ = binary.Write(&a, binary.LittleEndian, []int32{0, 0, 0, 0})
	a.Write([]byte{255, 255, 255, 255})
	_ = binary.Write(&a, binary.LittleEndian, []float32{1, 1})
	// Rotation, image kind, width, height, event, and anchor count.
	_ = binary.Write(&a, binary.LittleEndian, []int32{0, 0, 0, 0, -1, 0})
	_ = binary.Write(&a, binary.LittleEndian, uint32(0))  // event names
	_ = binary.Write(&a, binary.LittleEndian, float32(4)) // frame interval
	return s.Bytes(), a.Bytes()
}
