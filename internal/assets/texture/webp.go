package texture

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"image"

	"github.com/lxsmnsyc/wathena/internal/wasmhost"
)

// webpWasm is libwebp's encoder, built by wasm/build.sh.
//
//go:embed wasm/webp.wasm
var webpWasm []byte

var encoder = wasmhost.New(webpWasm)

// Level is libwebp's lossless effort, 0 to 9. Level 6 is within 4% of
// level 9 on RO textures, at a small fraction of the time.
const Level = 6

// encodeWebP encodes an image with libwebp at the given level.
func encodeWebP(img *image.NRGBA, level int) ([]byte, error) {
	in, err := encoder.Get()
	if err != nil {
		return nil, err
	}
	out, err := runWebP(in, img, level)
	if err != nil {
		in.Close()
		return nil, err
	}
	encoder.Put(in)
	return out, nil
}

func runWebP(in *wasmhost.Instance, img *image.NRGBA, level int) ([]byte, error) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	n := w * h * 4
	pix := img.Pix
	if img.Stride != w*4 {
		pix = make([]byte, 0, n)
		for y := range h {
			pix = append(pix, img.Pix[y*img.Stride:y*img.Stride+w*4]...)
		}
	}
	// One buffer holds the pixels and 4 bytes for the output pointer.
	buf, err := in.Alloc(n + 4)
	if err != nil {
		return nil, err
	}
	defer in.Free(buf)
	if err := in.Write(buf, pix[:n]); err != nil {
		return nil, err
	}
	outPtr := buf + uint32(n)                                                                                   // #nosec G115 -- at most 64 MiB.
	res, err := in.Call("wa_encode_lossless", uint64(buf), uint64(w), uint64(h), uint64(level), uint64(outPtr)) // #nosec G115 -- small positive values.
	if err != nil {
		return nil, err
	}
	size := uint32(res[0]) // #nosec G115 -- a C int, 0 on error.
	ptr, err := in.Read(outPtr, 4)
	if err != nil {
		return nil, err
	}
	p := binary.LittleEndian.Uint32(ptr)
	if size == 0 || p == 0 {
		return nil, fmt.Errorf("texture: libwebp failed to encode a %d × %d image", w, h)
	}
	defer in.Free(p)
	return in.Read(p, size)
}
