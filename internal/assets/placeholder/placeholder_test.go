package placeholder

import (
	"github.com/lxsmnsyc/wathena/internal/assets/wmap"
	"image/png"
	"os"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/assets/wspr"
)

func TestSprite(t *testing.T) {
	b, err := Sprite().Encode()
	if err != nil {
		t.Fatal(err)
	}
	s, err := wspr.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	img := s.RenderFrame(0, 0, 64)
	if img.NRGBAAt(32, 12).A == 0 {
		t.Error("the shape is not drawn above the origin")
	}
	if img.NRGBAAt(32, 31).A == 0 {
		t.Error("the shadow is not drawn at the origin")
	}
	if p := os.Getenv("WATHENA_PREVIEW"); p != "" {
		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := png.Encode(f, s.RenderFrame(0, 0, 64)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTexture(t *testing.T) {
	img := Texture()
	if img.Rect.Dx() != 64 || img.NRGBAAt(0, 0) == img.NRGBAAt(8, 0) || img.NRGBAAt(0, 0) != img.NRGBAAt(63, 63) {
		t.Error("texture is not a tiling checkerboard")
	}
}

func TestMap(t *testing.T) {
	m, lm := Map()
	b, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	got, err := wmap.Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	// The ground and the cells agree on heights, and the hill is in the
	// middle.
	center := got.Cells[12*mapSize+12].Heights[0]
	surface := got.Ground.Surfaces[6*mapSize/2+6].Heights[0]
	if center != surface || center < 1 || got.Cells[0].Heights[0] > 0.1 {
		t.Errorf("heights: center %v, surface %v, corner %v", center, surface, got.Cells[0].Heights[0])
	}
	if lm.Rect.Dx() != 8 || lm.NRGBAAt(3, 3).A != 255 {
		t.Error("lightmap is not one neutral block")
	}
}
