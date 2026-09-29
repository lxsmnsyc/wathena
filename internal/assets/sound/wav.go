package sound

import (
	"encoding/binary"
	"fmt"
	"math"
)

// WAV format tags.
const (
	wavPCM        = 1
	wavMSADPCM    = 2
	wavFloat      = 3
	wavIMAADPCM   = 17
	wavExtensible = 0xfffe
)

// PCM is decoded audio: interleaved samples from -1 to 1.
type PCM struct {
	Rate     int
	Channels int
	Samples  []float32
}

// Frames returns the number of samples per channel.
func (p *PCM) Frames() int { return len(p.Samples) / p.Channels }

type wavFormat struct {
	tag, channels, bits, blockAlign int
	rate                            int
	extra                           []byte
}

// decodeWAV reads a RIFF WAVE file. Chunk sizes past the end of the file
// are cut to what is there, since some game files have wrong sizes.
func decodeWAV(b []byte) (*PCM, error) {
	if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return nil, fmt.Errorf("%w: not a wav file", ErrFormat)
	}
	var f *wavFormat
	var data []byte
	// fact holds the frame count of compressed audio, or -1.
	fact := -1
	for p := 12; p+8 <= len(b); {
		id := string(b[p : p+4])
		n := int(binary.LittleEndian.Uint32(b[p+4:]))
		body := b[p+8:]
		if n < len(body) {
			body = body[:n]
		}
		switch id {
		case "fmt ":
			if len(body) < 16 {
				return nil, fmt.Errorf("%w: wav format chunk", ErrFormat)
			}
			f = &wavFormat{
				tag:        int(binary.LittleEndian.Uint16(body)),
				channels:   int(binary.LittleEndian.Uint16(body[2:])),
				rate:       int(binary.LittleEndian.Uint32(body[4:])),
				blockAlign: int(binary.LittleEndian.Uint16(body[12:])),
				bits:       int(binary.LittleEndian.Uint16(body[14:])),
			}
			if len(body) >= 18 {
				f.extra = body[18:]
			}
			// The extensible format stores the real tag at the start of
			// its sub-format GUID.
			if f.tag == wavExtensible && len(f.extra) >= 8 {
				f.tag = int(binary.LittleEndian.Uint16(f.extra[6:]))
			}
		case "data":
			if data == nil {
				data = body
			}
		case "fact":
			if len(body) >= 4 {
				fact = int(binary.LittleEndian.Uint32(body))
			}
		}
		if n > len(b)-p-8 {
			break
		}
		p += 8 + n + n%2
	}
	if f == nil || data == nil {
		return nil, fmt.Errorf("%w: wav has no format or data", ErrFormat)
	}
	if f.channels < 1 || f.channels > 2 {
		return nil, fmt.Errorf("%w: wav has %d channels", ErrFormat, f.channels)
	}
	if f.rate < MinRate || f.rate > MaxRate {
		return nil, fmt.Errorf("%w: wav rate %d", ErrFormat, f.rate)
	}
	pcm := &PCM{Rate: f.rate, Channels: f.channels}
	var err error
	switch f.tag {
	case wavPCM:
		pcm.Samples, err = decodeLinear(data, f.bits, f.channels)
	case wavFloat:
		pcm.Samples, err = decodeFloat(data, f.bits, f.channels)
	case wavIMAADPCM:
		pcm.Samples, err = decodeIMA(data, f)
	case wavMSADPCM:
		pcm.Samples, err = decodeMSADPCM(data, f)
	default:
		return nil, fmt.Errorf("%w: wav format %d", ErrFormat, f.tag)
	}
	if err != nil {
		return nil, err
	}
	// Compressed audio ends with a partly filled block. The fact chunk
	// gives the real length.
	if (f.tag == wavIMAADPCM || f.tag == wavMSADPCM) && fact > 0 && fact < pcm.Frames() {
		pcm.Samples = pcm.Samples[:fact*pcm.Channels]
	}
	if len(pcm.Samples) == 0 {
		return nil, fmt.Errorf("%w: wav is empty", ErrFormat)
	}
	if pcm.Frames() > MaxFrames(pcm.Rate) {
		return nil, fmt.Errorf("%w: wav is longer than %d seconds", ErrFormat, MaxSeconds)
	}
	return pcm, nil
}

// checkLength checks that data is not too long to decode, before any
// allocation. Samples are at least 4 bits, so this bounds the output.
func checkLength(data []byte) error {
	if len(data) > MaxSeconds*MaxRate*2*4 {
		return fmt.Errorf("%w: wav is longer than %d seconds", ErrFormat, MaxSeconds)
	}
	return nil
}

func decodeLinear(data []byte, bits, channels int) ([]float32, error) {
	if bits != 8 && bits != 16 && bits != 24 && bits != 32 {
		return nil, fmt.Errorf("%w: wav with %d bits", ErrFormat, bits)
	}
	if err := checkLength(data); err != nil {
		return nil, err
	}
	size := bits / 8
	n := len(data) / size / channels * channels
	out := make([]float32, n)
	for i := range out {
		s := data[i*size:]
		switch bits {
		case 8:
			out[i] = float32(int(s[0])-128) / 128
		case 16:
			out[i] = float32(int16(binary.LittleEndian.Uint16(s))) / 32768 // #nosec G115 -- reinterprets the bits.
		case 24:
			v := int32(uint32(s[0])<<8|uint32(s[1])<<16|uint32(s[2])<<24) >> 8 // #nosec G115 -- sign-extends 24 bits.
			out[i] = float32(v) / (1 << 23)
		case 32:
			out[i] = float32(int32(binary.LittleEndian.Uint32(s))) / (1 << 31) // #nosec G115 -- reinterprets the bits.
		}
	}
	return out, nil
}

func decodeFloat(data []byte, bits, channels int) ([]float32, error) {
	if bits != 32 && bits != 64 {
		return nil, fmt.Errorf("%w: float wav with %d bits", ErrFormat, bits)
	}
	if err := checkLength(data); err != nil {
		return nil, err
	}
	size := bits / 8
	n := len(data) / size / channels * channels
	out := make([]float32, n)
	for i := range out {
		var v float64
		if bits == 32 {
			v = float64(math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:])))
		} else {
			v = math.Float64frombits(binary.LittleEndian.Uint64(data[i*8:]))
		}
		// Out-of-range and NaN samples are clipped, since the encoder
		// expects -1 to 1.
		switch {
		case v != v: // NaN
			v = 0
		case v > 1:
			v = 1
		case v < -1:
			v = -1
		}
		out[i] = float32(v)
	}
	return out, nil
}

var imaSteps = [89]int32{
	7, 8, 9, 10, 11, 12, 13, 14, 16, 17, 19, 21, 23, 25, 28, 31, 34, 37, 41, 45,
	50, 55, 60, 66, 73, 80, 88, 97, 107, 118, 130, 143, 157, 173, 190, 209, 230,
	253, 279, 307, 337, 371, 408, 449, 494, 544, 598, 658, 724, 796, 876, 963,
	1060, 1166, 1282, 1411, 1552, 1707, 1878, 2066, 2272, 2499, 2749, 3024, 3327,
	3660, 4026, 4428, 4871, 5358, 5894, 6484, 7132, 7845, 8630, 9493, 10442,
	11487, 12635, 13899, 15289, 16818, 18500, 20350, 22385, 24623, 27086, 29794,
	32767,
}

var imaIndex = [8]int32{-1, -1, -1, -1, 2, 4, 6, 8}

type imaState struct {
	pred, index int32
}

func (s *imaState) next(n byte) float32 {
	step := imaSteps[s.index]
	diff := step >> 3
	if n&1 != 0 {
		diff += step >> 2
	}
	if n&2 != 0 {
		diff += step >> 1
	}
	if n&4 != 0 {
		diff += step
	}
	if n&8 != 0 {
		s.pred -= diff
	} else {
		s.pred += diff
	}
	s.pred = max(-32768, min(32767, s.pred))
	s.index = max(0, min(88, s.index+imaIndex[n&7]))
	return float32(s.pred) / 32768
}

// samplesPerBlock returns the frames per block from the format's extra
// bytes, if they are there and no more than max.
func samplesPerBlock(f *wavFormat, max int) int {
	if len(f.extra) >= 2 {
		if n := int(binary.LittleEndian.Uint16(f.extra)); n > 0 && n <= max {
			return n
		}
	}
	return max
}

// decodeIMA reads IMA ADPCM. Each block starts with a header per channel.
// Mono data follows as nibbles, low nibble first. Stereo data alternates
// 4 bytes, 8 samples, per channel. The last block can be short.
func decodeIMA(data []byte, f *wavFormat) ([]float32, error) {
	ch := f.channels
	if f.bits != 4 || f.blockAlign <= 4*ch || (f.blockAlign-4*ch)%(4*ch) != 0 {
		return nil, fmt.Errorf("%w: ima adpcm layout", ErrFormat)
	}
	if err := checkLength(data); err != nil {
		return nil, err
	}
	perBlock := samplesPerBlock(f, (f.blockAlign-4*ch)*2/ch+1)
	var out []float32
	for len(data) >= 4*ch {
		block := data[:min(f.blockAlign, len(data))]
		data = data[len(block):]
		states := make([]imaState, ch)
		body := block[4*ch:]
		groups := len(body) / (4 * ch)
		frames := make([]float32, (1+groups*8)*ch)
		for c := range ch {
			h := block[4*c:]
			states[c] = imaState{pred: int32(int16(binary.LittleEndian.Uint16(h))), index: int32(min(h[2], 88))} // #nosec G115 -- reinterprets the bits.
			frames[c] = float32(states[c].pred) / 32768
		}
		for g := range groups {
			for c := range ch {
				chunk := body[(g*ch+c)*4 : (g*ch+c)*4+4]
				for i, v := range chunk {
					f0 := 1 + g*8 + i*2
					frames[f0*ch+c] = states[c].next(v & 0x0f)
					frames[(f0+1)*ch+c] = states[c].next(v >> 4)
				}
			}
		}
		out = append(out, frames[:min(len(frames), perBlock*ch)]...)
	}
	return out, nil
}

var msAdapt = [16]int32{230, 230, 230, 230, 307, 409, 512, 614, 768, 614, 512, 409, 307, 230, 230, 230}

var msCoefs = [][2]int32{{256, 0}, {512, -256}, {0, 0}, {192, 64}, {240, 0}, {460, -208}, {392, -232}}

// decodeMSADPCM reads Microsoft ADPCM. Each block starts with a predictor,
// a step size, and two samples per channel. Nibbles follow, high nibble
// first, alternating channels.
func decodeMSADPCM(data []byte, f *wavFormat) ([]float32, error) {
	ch := f.channels
	if f.bits != 4 || f.blockAlign < 7*ch {
		return nil, fmt.Errorf("%w: ms adpcm layout", ErrFormat)
	}
	if err := checkLength(data); err != nil {
		return nil, err
	}
	perBlock := samplesPerBlock(f, 2+(f.blockAlign-7*ch)*2/ch)
	coefs := msCoefs
	// The format chunk can list its own coefficients, after the samples
	// per block and their count.
	if len(f.extra) >= 4 {
		n := int(binary.LittleEndian.Uint16(f.extra[2:]))
		if n > 0 && n <= 256 && len(f.extra) >= 4+4*n {
			coefs = make([][2]int32, n)
			for i := range coefs {
				c := f.extra[4+4*i:]
				coefs[i] = [2]int32{int32(int16(binary.LittleEndian.Uint16(c))), int32(int16(binary.LittleEndian.Uint16(c[2:])))} // #nosec G115 -- reinterprets the bits.
			}
		}
	}
	type state struct{ c1, c2, delta, s1, s2 int32 }
	var out []float32
	for len(data) >= 7*ch {
		block := data[:min(f.blockAlign, len(data))]
		data = data[len(block):]
		start := len(out)
		st := make([]state, ch)
		for c := range ch {
			i := int(block[c])
			if i >= len(coefs) {
				return nil, fmt.Errorf("%w: ms adpcm predictor %d", ErrFormat, i)
			}
			i16 := func(off int) int32 {
				return int32(int16(binary.LittleEndian.Uint16(block[off:]))) // #nosec G115 -- reinterprets the bits.
			}
			st[c] = state{c1: coefs[i][0], c2: coefs[i][1], delta: i16(ch + 2*c), s1: i16(3*ch + 2*c), s2: i16(5*ch + 2*c)}
		}
		// The two header samples come first, the older one first.
		for c := range ch {
			out = append(out, float32(st[c].s2)/32768)
		}
		for c := range ch {
			out = append(out, float32(st[c].s1)/32768)
		}
		c := 0
		for _, v := range block[7*ch:] {
			for _, n := range [2]byte{v >> 4, v & 0x0f} {
				s := &st[c]
				// 64-bit math, since crafted coefficients and step sizes
				// can overflow 32 bits.
				pred := (int64(s.s1)*int64(s.c1) + int64(s.s2)*int64(s.c2)) >> 8
				signed := int64(n)
				if signed >= 8 {
					signed -= 16
				}
				pred = max(-32768, min(32767, pred+signed*int64(s.delta)))
				s.s2, s.s1 = s.s1, int32(pred) // #nosec G115 -- clamped to 16 bits.
				// Step sizes are kept small enough that the next update
				// cannot overflow, as other decoders do.
				s.delta = max(16, min(math.MaxInt32/768, (msAdapt[n]*s.delta)>>8))
				out = append(out, float32(pred)/32768)
				c = (c + 1) % ch
			}
		}
		out = out[:start+min(len(out)-start, perBlock*ch)]
	}
	return out, nil
}
