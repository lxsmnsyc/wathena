package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/andybalholm/brotli"
	"golang.org/x/text/encoding/korean"

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
		raw, err := readJSON(root.FS(), h)
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
		{Name: `data\sprite\npc\4_f_kafra1.spr`, Data: spr},
		{Name: `data\sprite\npc\4_f_kafra1.act`, Data: act},
		{Name: `data\luafiles514\lua files\datainfo\jobname.lub`, Data: jobNameLub(114, "4_F_KAFRA1")},
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
	for _, name := range []string{"placeholder/sprite", "sprite/npc/114"} {
		e, ok := all[name]
		if !ok {
			t.Fatalf("%s missing from manifest", name)
		}
		if _, err := wspr.Decode(readAsset(t, out, e.H, "wspr")); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// jobNameLub builds a compiled Lua 5.1 chunk for
// JobNameTable = { [id] = name }.
func jobNameLub(id float64, name string) []byte {
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.Write([]byte{0x1b, 'L', 'u', 'a', 0x51, 0, 1, 4, 4, 4, 8, 0})
	w([]uint32{0, 0, 0})         // source name, line defined, last line defined
	b.Write([]byte{0, 0, 2, 10}) // upvalues, params, vararg, max stack
	const rkK = 0x100
	w([]uint32{
		4,                             // instructions
		10,                            // NEWTABLE 0
		9 | (rkK|1)<<23 | (rkK|2)<<14, // SETTABLE 0 K1 K2
		7,                             // SETGLOBAL 0 K0
		30 | 1<<23,                    // RETURN 0 1
	})
	w(uint32(3)) // constants
	b.WriteByte(4)
	w(uint32(len("JobNameTable") + 1))
	b.WriteString("JobNameTable\x00")
	b.WriteByte(3)
	w(math.Float64bits(id))
	b.WriteByte(4)
	w(uint32(len(name) + 1))
	b.WriteString(name + "\x00")
	w([]uint32{0, 0, 0, 0}) // protos, line info, locals, upvalue names
	return b.Bytes()
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

// TestItemTextTables builds the item text tables from a synthetic client
// in CP949, and checks that the code page is part of the cache key.
func TestItemTextTables(t *testing.T) {
	dir := t.TempDir()
	enc := func(s string) []byte {
		b, err := korean.EUCKR.NewEncoder().Bytes([]byte(s))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var buf bytes.Buffer
	if err := grf.Write(&buf, []grf.WriteEntry{
		{Name: `data\idnum2itemdisplaynametable.txt`, Data: enc("501#빨간_포션#\r\n1201#나이프#\r\n")},
		{Name: `data\num2itemdisplaynametable.txt`, Data: enc("1201#단검#\r\n")},
		{Name: `data\itemslotcounttable.txt`, Data: enc("1201#3#\r\n")},
		{Name: `data\idnum2itemdesctable.txt`, Data: enc("501#\r\n^000088HP^000000 회복\r\n#\r\n")},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.grf"), buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "DATA.INI"), []byte("[Data]\n1=data.grf\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	build := func(cp string) map[string]entry {
		g := &GRF{Dir: dir, CodePage: cp}
		defer func() { _ = g.Close() }()
		rep, err := Run(Options{Out: out, Brotli: 5, Sources: []Source{g}})
		if err != nil {
			t.Fatal(err)
		}
		if len(rep.Rejected) != 0 {
			t.Fatalf("rejected %v", rep.Rejected)
		}
		_, all := readManifest(t, out)
		return all
	}
	all := build("")
	names, desc := all[ItemNamesTable], all[ItemDescTable]
	if names.T != "table" || desc.T != "table" {
		t.Fatalf("entries %+v %+v", names, desc)
	}
	want := `{"1201":{"n":"나이프","u":"단검","s":3},"501":{"n":"빨간 포션"}}`
	if got := string(readAsset(t, out, names.H, "json")); got != want {
		t.Errorf("names %s, want %s", got, want)
	}
	want = `{"501":["^000088HP^000000 회복"]}`
	if got := string(readAsset(t, out, desc.H, "json")); got != want {
		t.Errorf("descriptions %s, want %s", got, want)
	}
	rep, err := Check(out, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Broken) != 0 {
		t.Errorf("broken %v", rep.Broken)
	}
	// Read as CP1252, the same bytes are different text, not a cached copy.
	if again := build("cp1252"); again[ItemNamesTable].H == names.H {
		t.Error("the code page did not change the output")
	}
}
