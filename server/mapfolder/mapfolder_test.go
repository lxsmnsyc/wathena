package mapfolder

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestSampleHomeMap: the Home Village sample's map reads, passes the
// checks, and writes a WMAP that decodes to its cells.
func TestSampleHomeMap(t *testing.T) {
	dir := filepath.Join("..", "..", "mods", "examples", "home-village")
	m, err := Read(os.DirFS(dir), "maps/village")
	if err != nil {
		t.Fatal(err)
	}
	if p := m.Check(); len(p) > 0 {
		t.Fatal(p)
	}
	if m.ID != "village" || !m.HasTag(TagHome) || m.Width != 48 || len(m.Places) != 3 || len(m.Objects) != 7 {
		t.Errorf("map %s %v %d %d %d", m.ID, m.Tags, m.Width, len(m.Places), len(m.Objects))
	}
	b, err := m.WMAP()
	if err != nil {
		t.Fatal(err)
	}
	w, err := wmap.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if w.Width != 48 || w.Height != 48 || w.Ground.Width != 24 || len(w.Objects) != 7 || len(w.Ground.Textures) != 2 {
		t.Errorf("wmap %d × %d, ground %d, %d objects, textures %v", w.Width, w.Height, w.Ground.Width, len(w.Objects), w.Ground.Textures)
	}
	if w.Cells[0].Flags != 0 || w.Cells[48+1].Flags != gen.Flags(gen.CellWalkable) {
		t.Error("cell flags")
	}
	found, err := Find(os.DirFS(dir))
	if err != nil || len(found) != 1 || found[0] != "maps/village" {
		t.Errorf("find %v %v", found, err)
	}
	g, again := m.Grid(), m.Grid()
	if g.Checksum() != again.Checksum() || len(g.Types) != 48*48 {
		t.Error("grid")
	}
}

// folder is a valid 8 × 8 map folder: walkable but a blocked wall at
// x 4 with a gap at y 7, a portal at 1,1 and a start at 6,1.
func folder() fstest.MapFS {
	const n = 8
	types := make([]uint8, n*n)
	for y := range 7 {
		types[y*n+4] = gen.CellBlocked
	}
	heights := make([]rng.Fixed, 4*n*n)
	zero := make([]uint8, n*n)
	return fstest.MapFS{
		"m/map.yaml":      {Data: []byte("format: 1\nid: m\nname: Test Map\nsize: [8, 8]\ntags: [home]\nflags: [nomemo]\nmaterials: [texture/gen/grassland/grass]\n")},
		"m/cells.bin":     {Data: EncodeCells(n, n, types, heights)},
		"m/materials.bin": {Data: EncodeMaterials(n, n, zero, zero, zero)},
		"m/objects.yaml":  {Data: []byte("- {model: model/gen/grassland/tree, x: 2.5, y: 6.5}\n")},
		"m/places.yaml":   {Data: []byte("- {tags: [portal], x: 1, y: 1}\n- {tags: [start], x: 6, y: 1}\n")},
	}
}

// TestRead: a folder that breaks the format is refused with the file and
// the reason.
func TestRead(t *testing.T) {
	m, err := Read(folder(), "m")
	if err != nil {
		t.Fatal(err)
	}
	if !m.HasFlag(FlagNoMemo) || len(m.Check()) != 0 {
		t.Errorf("flags %v, problems %v", m.Flags, m.Check())
	}
	for name, change := range map[string]func(f fstest.MapFS){
		"format 2": func(f fstest.MapFS) {
			f["m/map.yaml"].Data = []byte(strings.Replace(string(f["m/map.yaml"].Data), "format: 1", "format: 2", 1))
		},
		"unknown key": func(f fstest.MapFS) { f["m/map.yaml"].Data = append(f["m/map.yaml"].Data, "moon: 1\n"...) },
		"unknown flag": func(f fstest.MapFS) {
			f["m/map.yaml"].Data = []byte(strings.Replace(string(f["m/map.yaml"].Data), "nomemo", "nofly", 1))
		},
		"bad id": func(f fstest.MapFS) {
			f["m/map.yaml"].Data = []byte(strings.Replace(string(f["m/map.yaml"].Data), "id: m", "id: M!", 1))
		},
		"size": func(f fstest.MapFS) {
			f["m/map.yaml"].Data = []byte(strings.Replace(string(f["m/map.yaml"].Data), "[8, 8]", "[9, 8]", 1))
		},
		"no cells":       func(f fstest.MapFS) { delete(f, "m/cells.bin") },
		"short cells":    func(f fstest.MapFS) { f["m/cells.bin"].Data = f["m/cells.bin"].Data[:20] },
		"cell type":      func(f fstest.MapFS) { f["m/cells.bin"].Data[9] = 2 },
		"material":       func(f fstest.MapFS) { f["m/materials.bin"].Data[9] = 5 },
		"bad magic":      func(f fstest.MapFS) { f["m/materials.bin"].Data[0] = 'X' },
		"object outside": func(f fstest.MapFS) { f["m/objects.yaml"].Data = []byte("- {model: model/a/b, x: 20, y: 1}\n") },
		"bad model":      func(f fstest.MapFS) { f["m/objects.yaml"].Data = []byte("- {model: tree, x: 1, y: 1}\n") },
		"bad tag":        func(f fstest.MapFS) { f["m/places.yaml"].Data = []byte("- {tags: [Portal], x: 1, y: 1}\n") },
		"place outside":  func(f fstest.MapFS) { f["m/places.yaml"].Data = []byte("- {tags: [portal], x: 8, y: 1}\n") },
	} {
		f := folder()
		for k, v := range f {
			f[k] = &fstest.MapFile{Data: append([]byte(nil), v.Data...)}
		}
		change(f)
		if _, err := Read(f, "m"); !errors.Is(err, ErrFormat) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// TestCheck: places stand on walkable cells and are reached from the
// first, and a home map has a portal.
func TestCheck(t *testing.T) {
	m, err := Read(folder(), "m")
	if err != nil {
		t.Fatal(err)
	}
	// Close the gap in the wall: the start is cut off.
	m.Types[7*8+4] = gen.CellBlocked
	if p := m.Check(); len(p) != 1 || !strings.Contains(p[0], "not reached") {
		t.Errorf("a wall: %v", p)
	}
	m.Types[7*8+4] = gen.CellWalkable
	// A diagonal step needs both cells beside it.
	m.Types[6*8+3], m.Types[7*8+5] = gen.CellBlocked, gen.CellBlocked
	if p := m.Check(); len(p) != 1 {
		t.Errorf("a diagonal squeeze: %v", p)
	}
	m.Types[6*8+3], m.Types[7*8+5] = gen.CellWalkable, gen.CellWalkable
	m.Places[1].X = 4
	if p := m.Check(); len(p) != 1 || !strings.Contains(p[0], "not walkable") {
		t.Errorf("a place on the wall: %v", p)
	}
	m.Places = m.Places[1:]
	m.Places[0].X = 6
	if p := m.Check(); len(p) != 1 || !strings.Contains(p[0], "portal") {
		t.Errorf("a home map without a portal: %v", p)
	}
}

// FuzzRead: any map.yaml, cells.bin and places.yaml read without a panic,
// and a folder that reads passes Check and WMAP without one either.
func FuzzRead(f *testing.F) {
	base := folder()
	f.Add(base["m/map.yaml"].Data, base["m/cells.bin"].Data, base["m/places.yaml"].Data)
	f.Fuzz(func(t *testing.T, mapYAML, cells, placesYAML []byte) {
		fsys := folder()
		fsys["m/map.yaml"] = &fstest.MapFile{Data: mapYAML}
		fsys["m/cells.bin"] = &fstest.MapFile{Data: cells}
		fsys["m/places.yaml"] = &fstest.MapFile{Data: placesYAML}
		m, err := Read(fsys, "m")
		if err != nil {
			return
		}
		_ = m.Check()
		_, _ = m.WMAP()
	})
}
