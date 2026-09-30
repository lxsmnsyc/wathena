package path

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"testing"
)

// The reference records rAthena's results on six maps, for each mode.
// testdata holds those maps' cells, from rAthena's map caches (GPL-3.0):
// db/map_cache.dat, with db/re/map_cache.dat or db/pre-re/map_cache.dat
// over it. Each file is "CELL", the width and height as u16, then the
// zlib-compressed GAT cell types, row by row.

type reference struct {
	Maps []struct {
		Map          string `json:"map"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		WalkableHash uint64 `json:"walkable_hash"`
		Walk         []struct {
			From  [2]int `json:"from"`
			To    [2]int `json:"to"`
			Flag  int    `json:"flag"`
			Found bool   `json:"found"`
			Path  []Dir  `json:"path"`
		} `json:"walk"`
		Shoot []struct {
			From  [2]int `json:"from"`
			To    [2]int `json:"to"`
			Clear bool   `json:"clear"`
			Len   int    `json:"len"`
		} `json:"shoot"`
	} `json:"maps"`
}

// gatFlags maps GAT cell types to flags, as rAthena's map_gat2cell does.
var gatFlags = map[byte]uint8{0: Walkable | Shootable, 2: Walkable | Shootable, 3: Walkable | Shootable, 4: Walkable | Shootable, 5: Shootable, 6: Walkable | Shootable}

func loadGrid(t *testing.T, name string) *Grid {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name + ".cells")
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 8 || string(b[:4]) != "CELL" {
		t.Fatalf("%s: bad header", name)
	}
	g := &Grid{Width: int(binary.LittleEndian.Uint16(b[4:])), Height: int(binary.LittleEndian.Uint16(b[6:]))}
	r, err := zlib.NewReader(bytes.NewReader(b[8:]))
	if err != nil {
		t.Fatal(err)
	}
	types, err := io.ReadAll(r)
	if err != nil || len(types) != g.Width*g.Height {
		t.Fatalf("%s: %d cells, %v", name, len(types), err)
	}
	g.Cells = make([]uint8, len(types))
	for i, c := range types {
		g.Cells[i] = gatFlags[c]
	}
	return g
}

// walkableHash is the recorder's FNV-1a hash of walkability, which shows
// the cells are the ones rAthena used.
func walkableHash(g *Grid) uint64 {
	h := uint64(0xCBF29CE484222325)
	for y := range g.Height {
		for x := range g.Width {
			var w uint64
			if !g.NoPass(x, y) {
				w = 1
			}
			h ^= w
			h *= 0x100000001B3
		}
	}
	return h
}

func TestReference(t *testing.T) {
	checked := 0
	defer func() {
		t.Logf("%d queries match rAthena", checked)
		if checked < 9000 {
			t.Errorf("only %d queries checked", checked)
		}
	}()
	for _, mode := range []string{"renewal", "pre-renewal"} {
		b, err := os.ReadFile("../../reference/rathena/" + mode + "/pathfinding.json")
		if err != nil {
			t.Fatal(err)
		}
		var ref reference
		if err := json.Unmarshal(b, &ref); err != nil {
			t.Fatal(err)
		}
		for _, m := range ref.Maps {
			g := loadGrid(t, mode+"-"+m.Map)
			if g.Width != m.Width || g.Height != m.Height || walkableHash(g) != m.WalkableHash {
				t.Fatalf("%s %s: cells differ from the reference", mode, m.Map)
			}
			bad := 0
			for _, w := range m.Walk {
				path, found := Search(g, w.From[0], w.From[1], w.To[0], w.To[1], w.Flag&1 != 0)
				if found != w.Found || fmt.Sprint(path) != fmt.Sprint(w.Path) && found {
					if bad++; bad <= 5 {
						t.Errorf("%s %s walk %v→%v flag %d: got %v %v, want %v %v", mode, m.Map, w.From, w.To, w.Flag, found, path, w.Found, w.Path)
					}
				}
			}
			for _, s := range m.Shoot {
				clear, n := Shoot(g, s.From[0], s.From[1], s.To[0], s.To[1])
				if clear != s.Clear || n != s.Len {
					if bad++; bad <= 5 {
						t.Errorf("%s %s shoot %v→%v: got %v %d, want %v %d", mode, m.Map, s.From, s.To, clear, n, s.Clear, s.Len)
					}
				}
			}
			if bad > 0 {
				t.Errorf("%s %s: %d mismatches of %d", mode, m.Map, bad, len(m.Walk)+len(m.Shoot))
			}
			checked += len(m.Walk) + len(m.Shoot)
		}
	}
}

func TestTime(t *testing.T) {
	if StepTime(North, 150) != 150 || StepTime(NorthEast, 150) != 210 || StepTime(SouthWest, 151) != 211 {
		t.Error("step times")
	}
	if PathTime([]Dir{North, NorthEast, East}, 150) != 510 {
		t.Error("path time")
	}
}

// FuzzSearch checks that searches on any grid end without panicking and
// return paths that walk only on open cells to the goal.
func FuzzSearch(f *testing.F) {
	f.Add(uint8(8), uint8(8), []byte{1, 1, 0, 1, 1, 1, 1, 1}, uint8(0), uint8(0), uint8(5), uint8(6), false)
	f.Fuzz(func(t *testing.T, w, h uint8, cells []byte, x0, y0, x1, y1 uint8, easy bool) {
		g := &Grid{Width: int(w%40) + 1, Height: int(h%40) + 1}
		g.Cells = make([]uint8, g.Width*g.Height)
		for i := range g.Cells {
			if i < len(cells) {
				g.Cells[i] = cells[i] & (Walkable | Shootable)
			} else {
				g.Cells[i] = Walkable | Shootable
			}
		}
		sx, sy, ex, ey := int(x0%45), int(y0%45), int(x1%45), int(y1%45)
		steps, found := Search(g, sx, sy, ex, ey, easy)
		Shoot(g, sx, sy, ex, ey)
		if !found {
			return
		}
		x, y := sx, sy
		for i, d := range steps {
			dx, dy := d.Step()
			x += dx
			y += dy
			if i < len(steps)-1 && g.NoPass(x, y) && !easy {
				t.Fatalf("step %d onto a blocked cell (%d, %d)", i, x, y)
			}
		}
		if (x != ex || y != ey) && len(steps) > 0 {
			t.Fatalf("path ends at (%d, %d), not (%d, %d)", x, y, ex, ey)
		}
	})
}
