package texture

import (
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// webpWasm is libwebp's encoder, built by wasm/build.sh.
//
//go:embed wasm/webp.wasm
var webpWasm []byte

// Level is libwebp's lossless effort, 0 to 9. Level 6 is within 2% of
// level 9 on RO textures, at a fraction of the time.
const Level = 6

// maxMemoryPages limits each encoder's memory to 1 GiB, enough for a
// 4096 × 4096 image and libwebp's working buffers.
const maxMemoryPages = 1 << 14

var (
	encoderOnce sync.Once
	encoderErr  error
	wasmRuntime wazero.Runtime
	compiled    wazero.CompiledModule
	// idle holds encoder instances between calls. Each instance runs one
	// encode at a time.
	idle = make(chan *encoder, 64)
)

type encoder struct {
	mod    api.Module
	alloc  api.Function
	free   api.Function
	encode api.Function
}

func setup() error {
	encoderOnce.Do(func() {
		ctx := context.Background()
		wasmRuntime = wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithMemoryLimitPages(maxMemoryPages))
		if _, err := wasi_snapshot_preview1.Instantiate(ctx, wasmRuntime); err != nil {
			encoderErr = err
			return
		}
		compiled, encoderErr = wasmRuntime.CompileModule(ctx, webpWasm)
	})
	return encoderErr
}

func getEncoder() (*encoder, error) {
	select {
	case e := <-idle:
		return e, nil
	default:
	}
	if err := setup(); err != nil {
		return nil, err
	}
	// Each instance is anonymous, so many can exist at once. The module is a
	// WASI reactor, which is set up by calling _initialize.
	cfg := wazero.NewModuleConfig().WithName("").WithStartFunctions("_initialize")
	mod, err := wasmRuntime.InstantiateModule(context.Background(), compiled, cfg)
	if err != nil {
		return nil, err
	}
	e := &encoder{
		mod:    mod,
		alloc:  mod.ExportedFunction("wa_alloc"),
		free:   mod.ExportedFunction("wa_free"),
		encode: mod.ExportedFunction("wa_encode_lossless"),
	}
	if e.alloc == nil || e.free == nil || e.encode == nil {
		_ = mod.Close(context.Background())
		return nil, errors.New("texture: webp.wasm is missing exports")
	}
	return e, nil
}

func putEncoder(e *encoder) {
	select {
	case idle <- e:
	default:
		_ = e.mod.Close(context.Background())
	}
}

// encodeWebP encodes an image with libwebp at the given level.
func encodeWebP(img *image.NRGBA, level int) ([]byte, error) {
	w, h := img.Rect.Dx(), img.Rect.Dy()
	e, err := getEncoder()
	if err != nil {
		return nil, err
	}
	out, err := e.run(img, w, h, level)
	if err != nil {
		// A failed instance may be in a bad state, so it is not reused.
		_ = e.mod.Close(context.Background())
		return nil, err
	}
	putEncoder(e)
	return out, nil
}

func (e *encoder) run(img *image.NRGBA, w, h, level int) ([]byte, error) {
	ctx := context.Background()
	mem := e.mod.Memory()
	n := w * h * 4
	// One call allocates the pixels and 4 bytes for the output pointer.
	res, err := e.alloc.Call(ctx, uint64(n+4)) // #nosec G115 -- at most 64 MiB.
	if err != nil {
		return nil, err
	}
	in := uint32(res[0]) // #nosec G115 -- a wasm32 pointer.
	if in == 0 {
		return nil, errors.New("texture: webp encoder is out of memory")
	}
	defer func() { _, _ = e.free.Call(ctx, uint64(in)) }()
	pix := img.Pix
	if img.Stride != w*4 {
		pix = make([]byte, 0, n)
		for y := range h {
			pix = append(pix, img.Pix[y*img.Stride:y*img.Stride+w*4]...)
		}
	}
	outPtr := in + uint32(n) // #nosec G115 -- at most 64 MiB.
	if !mem.Write(in, pix[:n]) {
		return nil, errors.New("texture: webp encoder memory")
	}
	res, err = e.encode.Call(ctx, uint64(in), uint64(w), uint64(h), uint64(level), uint64(outPtr)) // #nosec G115 -- small positive values.
	if err != nil {
		return nil, err
	}
	size := uint32(res[0]) // #nosec G115 -- a C int, 0 on error.
	ptrBytes, ok := mem.Read(outPtr, 4)
	if !ok {
		return nil, errors.New("texture: webp encoder memory")
	}
	ptr := binary.LittleEndian.Uint32(ptrBytes)
	if size == 0 || ptr == 0 {
		return nil, fmt.Errorf("texture: libwebp failed to encode a %d × %d image", w, h)
	}
	defer func() { _, _ = e.free.Call(ctx, uint64(ptr)) }()
	data, ok := mem.Read(ptr, size)
	if !ok {
		return nil, errors.New("texture: webp encoder memory")
	}
	// Read returns a view of wasm memory, which the next call can change.
	return append([]byte(nil), data...), nil
}
