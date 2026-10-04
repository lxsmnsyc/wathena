package emblem

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"testing"

	"golang.org/x/image/bmp"
)

func square(n int, c color.Color) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		for x := range n {
			m.Set(x, y, c)
		}
	}
	return m
}

func TestProcess(t *testing.T) {
	m := square(24, color.RGBA{10, 20, 30, 255})
	m.Set(0, 0, color.RGBA{255, 0, 255, 255})
	var b bytes.Buffer
	if err := bmp.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	out, err := Process(b.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	got, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := got.At(0, 0).RGBA(); a != 0 {
		t.Error("magenta is not transparent")
	}
	if r, g, bl, a := got.At(5, 5).RGBA(); r>>8 != 10 || g>>8 != 20 || bl>>8 != 30 || a>>8 != 255 {
		t.Errorf("pixel %d %d %d %d", r>>8, g>>8, bl>>8, a>>8)
	}
	if bytes.Equal(out, b.Bytes()) {
		t.Error("the upload was kept")
	}
	var g bytes.Buffer
	if err := gif.Encode(&g, square(24, color.White), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Process(g.Bytes()); err != nil {
		t.Errorf("gif: %v", err)
	}
	b.Reset()
	_ = png.Encode(&b, square(32, color.White))
	if _, err := Process(b.Bytes()); err != ErrSize {
		t.Errorf("32 px: %v", err)
	}
	if _, err := Process([]byte("<svg></svg>")); err != ErrFormat {
		t.Errorf("svg: %v", err)
	}
	if _, err := Process(make([]byte, MaxUpload+1)); err != ErrTooLarge {
		t.Errorf("large: %v", err)
	}
}

func FuzzProcess(f *testing.F) {
	var b bytes.Buffer
	_ = png.Encode(&b, square(24, color.White))
	f.Add(b.Bytes())
	f.Fuzz(func(t *testing.T, data []byte) {
		out, err := Process(data)
		if err != nil {
			return
		}
		m, err := png.Decode(bytes.NewReader(out))
		if err != nil || m.Bounds().Dx() != Size || m.Bounds().Dy() != Size {
			t.Fatalf("bad output: %v", err)
		}
	})
}
