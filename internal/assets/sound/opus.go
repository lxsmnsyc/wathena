package sound

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/lxsmnsyc/wathena/internal/wasmhost"
)

// opusWasm is libopus's encoder and the speexdsp resampler, built by
// wasm/build.sh.
//
//go:embed wasm/opus.wasm
var opusWasm []byte

var encoder = wasmhost.New(opusWasm)

// frameSamples is the samples per channel in one 20 ms packet at 48 kHz.
const frameSamples = 960

// opusStream is encoded audio.
type opusStream struct {
	packets [][]byte
	// preSkip is how many samples at 48 kHz the decoder drops at the start.
	preSkip int
	// frames is the length at 48 kHz. The last packet holds padding past it.
	frames int
}

func encodeOpus(p *PCM, bitrate int) (*opusStream, error) {
	in, err := encoder.Get()
	if err != nil {
		return nil, err
	}
	s, err := runOpus(in, p, bitrate)
	if err != nil {
		in.Close()
		return nil, err
	}
	encoder.Put(in)
	return s, nil
}

func runOpus(in *wasmhost.Instance, p *PCM, bitrate int) (*opusStream, error) {
	n := len(p.Samples) * 4
	// One buffer holds the samples, the output pointer, and 2 info values.
	buf, err := in.Alloc(n + 12)
	if err != nil {
		return nil, err
	}
	defer in.Free(buf)
	raw := make([]byte, n)
	for i, v := range p.Samples {
		binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(v))
	}
	if err := in.Write(buf, raw); err != nil {
		return nil, err
	}
	outPtr := buf + uint32(n)                                                                                                                                // #nosec G115 -- at most 1 GiB.
	res, err := in.Call("wa_encode", uint64(buf), uint64(p.Frames()), uint64(p.Channels), uint64(p.Rate), uint64(bitrate), uint64(outPtr), uint64(outPtr+4)) // #nosec G115 -- small positive values.
	if err != nil {
		return nil, err
	}
	size := int32(uint32(res[0])) // #nosec G115 -- a C int.
	if size <= 0 {
		return nil, fmt.Errorf("sound: opus encoder failed with code %d", size)
	}
	head, err := in.Read(outPtr, 12)
	if err != nil {
		return nil, err
	}
	ptr := binary.LittleEndian.Uint32(head)
	defer in.Free(ptr)
	data, err := in.Read(ptr, uint32(size)) // #nosec G115 -- positive.
	if err != nil {
		return nil, err
	}
	s := &opusStream{
		preSkip: int(binary.LittleEndian.Uint32(head[4:])),
		frames:  int(binary.LittleEndian.Uint32(head[8:])),
	}
	for len(data) >= 2 {
		l := int(binary.LittleEndian.Uint16(data))
		if l > len(data)-2 {
			return nil, fmt.Errorf("sound: opus encoder output is malformed")
		}
		s.packets = append(s.packets, data[2:2+l])
		data = data[2+l:]
	}
	return s, nil
}
