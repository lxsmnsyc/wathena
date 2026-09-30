package grfimport

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"golang.org/x/text/encoding/korean"

	"github.com/lxsmnsyc/wathena/internal/grf"
)

// itemClient writes a client folder with item tables and files.
func itemClient(t *testing.T) *grf.Client {
	t.Helper()
	cp949 := func(s string) []byte {
		b, err := korean.EUCKR.NewEncoder().Bytes([]byte(s))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var buf bytes.Buffer
	err := grf.Write(&buf, []grf.WriteEntry{
		{Name: `data\idnum2itemresnametable.txt`, Data: cp949("// comment\n501#빨간포션#\n502#주황포션#\n4001#이름없는카드#\nbad line\n")},
		{Name: `data\num2cardillustnametable.txt`, Data: cp949("4001#포링카드#\n")},
		{Name: `data\texture\유저인터페이스\item\빨간포션.bmp`, Data: []byte("x")},
		{Name: `data\texture\유저인터페이스\collection\빨간포션.bmp`, Data: []byte("x")},
		{Name: `data\texture\유저인터페이스\item\이름없는카드.bmp`, Data: []byte("x")},
		{Name: `data\texture\유저인터페이스\cardbmp\포링카드.bmp`, Data: []byte("x")},
		{Name: `data\sprite\아이템\빨간포션.spr`, Data: []byte("x")},
		{Name: `data\sprite\아이템\빨간포션.act`, Data: []byte("x")},
	})
	if err != nil {
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

func TestItemAssets(t *testing.T) {
	c := itemClient(t)
	textures, sprites, err := ItemAssets(c)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tx := range textures {
		names = append(names, tx.Name)
	}
	sort.Strings(names)
	// 502 has no files, and the card's icon is the shared card icon.
	want := []string{"icon/item/4001", "icon/item/501", "image/card/4001", "image/item/501"}
	if len(names) != len(want) {
		t.Fatalf("textures %v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("textures %v, want %v", names, want)
		}
	}
	if len(sprites) != 1 || sprites[0].Name != "sprite/item/501" {
		t.Errorf("sprites %+v", sprites)
	}
	// Item images are named by ID only, not by path.
	if n := TextureNames([]string{`data\texture\유저인터페이스\item\빨간포션.bmp`}); len(n) != 0 {
		t.Errorf("path names %v", n)
	}
}
