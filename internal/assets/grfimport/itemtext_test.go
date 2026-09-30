package grfimport

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"golang.org/x/text/encoding/korean"

	"github.com/lxsmnsyc/wathena/internal/grf"
)

func cp949(t *testing.T, s string) []byte {
	t.Helper()
	b, err := korean.EUCKR.NewEncoder().Bytes([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// itemInfoChunk builds an iteminfo.lub chunk:
//
//	tbl = { [501] = { identifiedDisplayName = name, unidentifiedDisplayName = un,
//	                  slotCount = 2, identifiedDescriptionName = { line1, line2 } } }
func itemInfoChunk(name, un, line1, line2 string) []byte {
	return chunk([]uint32{
		iABC(opNewTable, 0, 0, 0),
		iABC(opNewTable, 1, 0, 0),
		iABC(opSetTable, 1, rkK|2, rkK|3),
		iABC(opSetTable, 1, rkK|4, rkK|5),
		iABC(opSetTable, 1, rkK|6, rkK|7),
		iABC(opNewTable, 2, 0, 0),
		iABx(opLoadK, 3, 9),
		iABx(opLoadK, 4, 10),
		iABC(opSetList, 2, 2, 1),
		iABC(opSetTable, 1, rkK|8, 2),
		iABC(opSetTable, 0, rkK|1, 1),
		iABx(opSetGlob, 0, 0),
		iABC(opReturn, 0, 1, 0),
	}, []any{
		"tbl", 501.0,
		"identifiedDisplayName", name,
		"unidentifiedDisplayName", un,
		"slotCount", 2.0,
		"identifiedDescriptionName", line1, line2,
	})
}

func textClient(t *testing.T, entries []grf.WriteEntry) *grf.Client {
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

func TestItemTexts(t *testing.T) {
	c := textClient(t, []grf.WriteEntry{
		{Name: itemNamePath, Data: cp949(t, "// comment\r\n501#빨간_포션#\r\n502#주황_포션#\r\n1201#나이프#\r\n")},
		{Name: itemUnnamePath, Data: cp949(t, "502#포션#\r\n1201#단검#\r\n")},
		{Name: itemSlotsPath, Data: cp949(t, "1201#3#\r\n1202#9#\r\n")},
		{Name: itemDescPath, Data: cp949(t, "502#\r\n주황색 ^000088포션^000000\r\n무게 : 10\r\n\r\n#\r\n1201#\r\n짧은 단검\r\n#\r\n")},
		{Name: itemInfoPath, Data: itemInfoChunk(
			string(cp949(t, "빨간포션")), string(cp949(t, "포션")),
			string(cp949(t, "체력 ^000088회복^000000")), string(cp949(t, "무게 : 7")),
		)},
	})
	d, err := NewTextDecoder("cp949")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ItemTexts(c, d)
	if err != nil {
		t.Fatal(err)
	}
	want := map[int]ItemText{
		// iteminfo.lub replaces the text tables.
		501: {Name: "빨간포션", Unidentified: "포션", Slots: 2, Desc: []string{"체력 ^000088회복^000000", "무게 : 7"}},
		// "_" in the text tables is a space, and trailing empty lines go.
		502:  {Name: "주황 포션", Unidentified: "포션", Desc: []string{"주황색 ^000088포션^000000", "무게 : 10"}},
		1201: {Name: "나이프", Unidentified: "단검", Slots: 3, Desc: []string{"짧은 단검"}},
		// 1202 has only a slot count, so it has no text and is left out.
	}
	if len(got) != len(want) {
		t.Fatalf("got %d items, want %d: %v", len(got), len(want), got)
	}
	for id, w := range want {
		g := got[id]
		if g == nil || g.Name != w.Name || g.Unidentified != w.Unidentified || g.Slots != w.Slots || !slices.Equal(g.Desc, w.Desc) {
			t.Errorf("item %d: %+v, want %+v", id, g, w)
		}
	}
}

func TestTextDecoder(t *testing.T) {
	for _, tc := range []struct {
		cp, in, want string
	}{
		{"cp949", string(cp949(t, "포링")), "포링"},
		{"CP1252", "Caf\xe9 \x80", "Café €"},
		{"cp1251", "\xcf\xee\xf0\xe8\xed\xe3", "Поринг"},
		{"utf-8", "ok \xff", "ok "},
		// A decomposed Hangul syllable is composed, as NFC.
		{"utf-8", "포", "포"},
	} {
		d, err := NewTextDecoder(tc.cp)
		if err != nil {
			t.Fatal(err)
		}
		if got := d.Decode(tc.in); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.cp, got, tc.want)
		}
	}
	if _, err := NewTextDecoder("ebcdic"); err == nil {
		t.Error("unknown code page accepted")
	}
}

func TestParseDescTable(t *testing.T) {
	// A missing closing "#" drops the entry, and junk between entries is
	// skipped.
	got := parseDescTable("junk\n7#\na\nb\n#\n8#\nnever closed\n")
	if len(got) != 1 || !slices.Equal(got[7], []string{"a", "b"}) {
		t.Errorf("got %v", got)
	}
}

func FuzzItemTextTables(f *testing.F) {
	f.Add("501#\nline\n#\n502#x#\n")
	f.Fuzz(func(t *testing.T, s string) {
		_ = parseDescTable(s)
		_ = parseTextTable(s)
	})
}

// TestRealItemTexts reads a real client's item text when WATHENA_CLIENT
// names a client folder.
func TestRealItemTexts(t *testing.T) {
	dir := os.Getenv("WATHENA_CLIENT")
	if dir == "" {
		t.Skip("set WATHENA_CLIENT to a client folder to run")
	}
	c, err := grf.OpenClient(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	d, _ := NewTextDecoder(DefaultCodePage)
	texts, err := ItemTexts(c, d)
	if err != nil {
		t.Fatal(err)
	}
	if texts[501] == nil || texts[501].Name == "" {
		t.Error("no name for item 501")
	}
	t.Logf("%d items, 501 is %q", len(texts), texts[501].Name)
}
