package texture

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"

	"golang.org/x/image/webp"
)

// pixels is a 2 × 2 test image, top row first.
var pixels = [4][4]byte{
	{255, 0, 0, 255}, {0, 255, 0, 255},
	{0, 0, 255, 255}, {255, 0, 255, 255},
}

func checkPixels(t *testing.T, img *image.NRGBA, want [4][4]byte) {
	t.Helper()
	if img.Rect.Dx() != 2 || img.Rect.Dy() != 2 {
		t.Fatalf("size %v", img.Rect)
	}
	for i, w := range want {
		got := [4]byte(img.Pix[(i/2)*img.Stride+(i%2)*4:])
		if got != w {
			t.Errorf("pixel %d = %v, want %v", i, got, w)
		}
	}
}

// buildBMP writes a 2 × 2 bottom-up BMP with the given bits per pixel.
func buildBMP(bpp int, palette [][4]byte, rows [2][]byte, comp uint32, masks []uint32) []byte {
	var hdr bytes.Buffer
	w := func(v any) { _ = binary.Write(&hdr, binary.LittleEndian, v) }
	w(uint32(40))
	w(int32(2))
	w(int32(2))
	w(uint16(1))
	w(uint16(bpp))
	w(comp)
	w(uint32(0))
	w(int32(0))
	w(int32(0))
	w(uint32(len(palette)))
	w(uint32(0))
	for _, m := range masks {
		w(m)
	}
	for _, c := range palette {
		hdr.Write([]byte{c[2], c[1], c[0], 0})
	}
	var b bytes.Buffer
	b.WriteString("BM")
	_ = binary.Write(&b, binary.LittleEndian, uint32(0))
	_ = binary.Write(&b, binary.LittleEndian, uint32(0))
	_ = binary.Write(&b, binary.LittleEndian, uint32(14+hdr.Len()))
	b.Write(hdr.Bytes())
	// Rows are stored bottom first, each padded to 4 bytes.
	for _, r := range []int{1, 0} {
		row := append([]byte(nil), rows[r]...)
		for len(row)%4 != 0 {
			row = append(row, 0)
		}
		b.Write(row)
	}
	return b.Bytes()
}

func TestBMP(t *testing.T) {
	pal := [][4]byte{pixels[0], pixels[1], pixels[2]}
	for name, data := range map[string][]byte{
		"8-bit": buildBMP(8, pal, [2][]byte{{0, 1}, {2, 3}}, biRGB, nil),
		"4-bit": buildBMP(4, pal, [2][]byte{{0x01}, {0x23}}, biRGB, nil),
		"24-bit": buildBMP(24, nil, [2][]byte{
			{0, 0, 255, 0, 255, 0},
			{255, 0, 0, 255, 0, 255},
		}, biRGB, nil),
		"16-bit": buildBMP(16, nil, [2][]byte{
			{0x00, 0x7c, 0xe0, 0x03},
			{0x1f, 0x00, 0x1f, 0x7c},
		}, biRGB, nil),
		"32-bit bitfields": buildBMP(32, nil, [2][]byte{
			{0, 0, 0xff, 0, 0, 0xff, 0, 0},
			{0xff, 0, 0, 0, 0xff, 0, 0xff, 0},
		}, biBitfields, []uint32{0xff0000, 0xff00, 0xff}),
	} {
		img, err := Decode(BMP, data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		want := pixels
		if name == "8-bit" || name == "4-bit" {
			// Index 3 is past the 3-color palette, so it is black.
			want[3] = [4]byte{0, 0, 0, 255}
		}
		checkPixels(t, img, want)
	}
}

// buildTGA writes a 2 × 2 32-bit TGA. rle stores each row as one raw
// packet and one repeated packet.
func buildTGA(rle, topDown bool) []byte {
	b := []byte{0, 0, tgaTrueColor, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 2, 0, 32, 8}
	if rle {
		b[2] = tgaRLETrueColor
	}
	if topDown {
		b[17] |= 0x20
	}
	bgra := func(c [4]byte) []byte { return []byte{c[2], c[1], c[0], c[3]} }
	order := []int{2, 3, 0, 1}
	if topDown {
		order = []int{0, 1, 2, 3}
	}
	for _, i := range order {
		if rle {
			b = append(b, 0)
		}
		b = append(b, bgra(pixels[i])...)
	}
	return b
}

func TestTGA(t *testing.T) {
	for _, rle := range []bool{false, true} {
		for _, top := range []bool{false, true} {
			img, err := Decode(TGA, buildTGA(rle, top))
			if err != nil {
				t.Fatal(err)
			}
			checkPixels(t, img, pixels)
		}
	}
	// A run packet repeats one pixel.
	run := []byte{0, 0, tgaRLEGray, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 2, 0, 8, 0x20, 0x83, 7}
	img, err := Decode(TGA, run)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 7 || img.Pix[i+3] != 255 {
			t.Fatalf("gray run %v", img.Pix)
		}
	}
	if _, err := Decode(TGA, run[:len(run)-1]); err == nil {
		t.Error("truncated tga decoded")
	}
}

func TestSizeLimit(t *testing.T) {
	b := buildTGA(false, false)
	binary.LittleEndian.PutUint16(b[12:], MaxSize+1)
	if _, err := Decode(TGA, b); err == nil {
		t.Error("oversized tga decoded")
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewGray(image.Rect(0, 0, MaxSize+1, 1)))
	if _, err := Decode(PNG, buf.Bytes()); err == nil {
		t.Error("oversized png decoded")
	}
}

func TestConvert(t *testing.T) {
	pal := [][4]byte{pixels[0], pixels[1], pixels[2], pixels[3]}
	w, err := Convert(BMP, buildBMP(8, pal, [2][]byte{{0, 1}, {2, 3}}, biRGB, nil))
	if err != nil {
		t.Fatal(err)
	}
	img, err := webp.Decode(bytes.NewReader(w))
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range pixels {
		got := color.NRGBAModel.Convert(img.At(i%2, i/2)).(color.NRGBA)
		if i == 3 {
			// Magenta is keyed out, and takes the average of its neighbors.
			if got.A != 0 || got != (color.NRGBA{85, 85, 85, 0}) {
				t.Errorf("keyed pixel %v", got)
			}
			continue
		}
		if got != (color.NRGBA{want[0], want[1], want[2], want[3]}) {
			t.Errorf("pixel %d = %v, want %v", i, got, want)
		}
	}
	// Encoding is deterministic, since assets are stored by content hash.
	again, err := Convert(BMP, buildBMP(8, pal, [2][]byte{{0, 1}, {2, 3}}, biRGB, nil))
	if err != nil || !bytes.Equal(w, again) {
		t.Error("encoding is not deterministic")
	}
}

func FuzzDecode(f *testing.F) {
	pal := [][4]byte{pixels[0], pixels[1]}
	f.Add(uint8(BMP), buildBMP(8, pal, [2][]byte{{0, 1}, {1, 0}}, biRGB, nil))
	f.Add(uint8(BMP), buildBMP(16, nil, [2][]byte{{0, 0x7c, 0, 0}, {0, 0, 0, 0}}, biRGB, nil))
	f.Add(uint8(TGA), buildTGA(true, false))
	f.Add(uint8(TGA), buildTGA(false, true))
	f.Fuzz(func(t *testing.T, format uint8, data []byte) {
		img, err := Decode(Format(format%2), data)
		if err != nil {
			return
		}
		if img.Rect.Dx() > MaxSize || img.Rect.Dy() > MaxSize {
			t.Fatalf("decoded %v", img.Rect)
		}
	})
}
