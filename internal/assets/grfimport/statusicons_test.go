package grfimport

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/grf"
)

// efstChunk is efstids.lub in small:
//
//	EFST_IDs = { EFST_A = 10, EFST_B = 20 }
//	setmetatable(EFST_IDs, EFST_IDs)
func efstChunk() []byte {
	return chunk([]uint32{
		iABC(opNewTable, 0, 0, 0),
		iABC(opSetTable, 0, rkK|1, rkK|2),
		iABC(opSetTable, 0, rkK|3, rkK|4),
		iABx(opSetGlob, 0, 0),
		iABx(opGetGlob, 0, 5),
		iABx(opGetGlob, 1, 0),
		iABx(opGetGlob, 2, 0),
		iABC(opCall, 0, 3, 1),
		iABC(opReturn, 0, 1, 0),
	}, []any{"EFST_IDs", "EFST_A", 10.0, "EFST_B", 20.0, "setmetatable"})
}

// iconChunk is stateiconimginfo.lub in small:
//
//	StateIconImgList = {
//	  [1] = { [EFST_IDs.EFST_A] = "IconA.TGA", [EFST_IDs.EFST_B] = "gone.tga" },
//	  [0] = { [EFST_IDs.EFST_B] = "b.bmp" },
//	}
func iconChunk() []byte {
	return chunk([]uint32{
		iABC(opNewTable, 0, 0, 0),
		iABC(opNewTable, 1, 0, 0),
		iABx(opGetGlob, 2, 0),
		iABC(opGetTable, 3, 2, rkK|1),
		iABC(opSetTable, 1, 3, rkK|2),
		iABC(opGetTable, 3, 2, rkK|3),
		iABC(opSetTable, 1, 3, rkK|4),
		iABC(opSetTable, 0, rkK|5, 1),
		iABC(opNewTable, 1, 0, 0),
		iABC(opGetTable, 3, 2, rkK|3),
		iABC(opSetTable, 1, 3, rkK|6),
		iABC(opSetTable, 0, rkK|7, 1),
		iABx(opSetGlob, 0, 8),
		iABC(opReturn, 0, 1, 0),
	}, []any{"EFST_IDs", "EFST_A", "IconA.TGA", "EFST_B", "gone.tga", 1.0, "b.bmp", 0.0, "StateIconImgList"})
}

// statusClient writes a client folder with the given files.
func statusClient(t *testing.T, entries []grf.WriteEntry) *grf.Client {
	t.Helper()
	var buf bytes.Buffer
	if err := grf.Write(&buf, entries); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "data.grf"), buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "DATA.INI"), []byte("[Data]\n1=data.grf\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := grf.OpenClient(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestStatusIconAssets(t *testing.T) {
	c := statusClient(t, []grf.WriteEntry{
		{Name: efstIDsPath, Data: efstChunk()},
		{Name: stateIconImgPath, Data: iconChunk()},
		{Name: `data\texture\effect\icona.tga`, Data: []byte("a")},
		// b.bmp is listed, but only a TGA exists.
		{Name: `data\texture\effect\b.tga`, Data: []byte("b")},
	})
	icons, err := StatusIconAssets(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(icons) != 2 {
		t.Fatalf("icons %+v", icons)
	}
	if icons[0].Name != "icon/status/10" || icons[0].Source.Entry.Key != "data/texture/effect/icona.tga" {
		t.Errorf("first icon %+v", icons[0])
	}
	// Priority 0 wins over priority 1, whose file is missing anyway.
	if icons[1].Name != "icon/status/20" || icons[1].Source.Entry.Key != "data/texture/effect/b.tga" {
		t.Errorf("second icon %+v", icons[1])
	}
}

func TestStatusIconAssetsWithoutTables(t *testing.T) {
	c := statusClient(t, []grf.WriteEntry{{Name: `data\texture\effect\icona.tga`, Data: []byte("a")}})
	icons, err := StatusIconAssets(c)
	if err != nil || len(icons) != 0 {
		t.Errorf("icons %+v, err %v", icons, err)
	}
}

func TestEvalLubSetMetatable(t *testing.T) {
	g := map[string]any{}
	if err := evalLub(efstChunk(), g); err != nil {
		t.Fatal(err)
	}
	tb, _ := g["EFST_IDs"].(*LubTable)
	if tb == nil || tb.M["EFST_A"] != 10.0 || tb.M["EFST_B"] != 20.0 {
		t.Errorf("globals %v", g)
	}
	// setmetatable on a non-table, or a global the chunk set itself
	// under that name, is still refused.
	for name, code := range map[string][]uint32{
		"non-table": {
			iABx(opGetGlob, 0, 0),
			iABx(opLoadK, 1, 1),
			iABC(opCall, 0, 2, 1),
		},
		"redefined": {
			iABC(opNewTable, 0, 0, 0),
			iABx(opSetGlob, 0, 0),
			iABx(opGetGlob, 0, 0),
			iABC(opNewTable, 1, 0, 0),
			iABC(opCall, 0, 2, 1),
		},
	} {
		if err := evalLub(chunk(code, []any{"setmetatable", 1.0}), map[string]any{}); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
