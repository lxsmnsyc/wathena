package sound

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"testing"
)

// buildWAV writes a WAV file with one format chunk and one data chunk.
func buildWAV(tag, channels, rate, bits, blockAlign int, extra, data []byte, fact int) []byte {
	var fmtBody bytes.Buffer
	w := func(v any) { _ = binary.Write(&fmtBody, binary.LittleEndian, v) }
	w(uint16(tag))
	w(uint16(channels))
	w(uint32(rate))
	w(uint32(rate * blockAlign))
	w(uint16(blockAlign))
	w(uint16(bits))
	if extra != nil {
		w(uint16(len(extra)))
		fmtBody.Write(extra)
	}
	var b bytes.Buffer
	chunk := func(id string, body []byte) {
		b.WriteString(id)
		_ = binary.Write(&b, binary.LittleEndian, uint32(len(body)))
		b.Write(body)
		if len(body)%2 == 1 {
			b.WriteByte(0)
		}
	}
	chunk("fmt ", fmtBody.Bytes())
	if fact >= 0 {
		chunk("fact", binary.LittleEndian.AppendUint32(nil, uint32(fact)))
	}
	chunk("data", data)
	out := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(4+b.Len()))...)
	return append(append(out, "WAVE"...), b.Bytes()...)
}

func TestLinear(t *testing.T) {
	for _, c := range []struct {
		name string
		tag  int
		bits int
		data []byte
	}{
		{"8-bit", wavPCM, 8, []byte{128, 192, 0}},
		{"16-bit", wavPCM, 16, []byte{0, 0, 0, 0x40, 0, 0x80}},
		{"24-bit", wavPCM, 24, []byte{0, 0, 0, 0, 0, 0x40, 0, 0, 0x80}},
		{"32-bit", wavPCM, 32, []byte{0, 0, 0, 0, 0, 0, 0, 0x40, 0, 0, 0, 0x80}},
		{"float", wavFloat, 32, binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint32(nil,
			math.Float32bits(0)), math.Float32bits(0.5)), math.Float32bits(-3))},
	} {
		p, err := decodeWAV(buildWAV(c.tag, 1, 8000, c.bits, c.bits/8, nil, c.data, -1))
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		// Out-of-range floats are clipped to -1.
		if len(p.Samples) != 3 || p.Samples[0] != 0 || p.Samples[1] != 0.5 || p.Samples[2] != -1 {
			t.Errorf("%s: %v", c.name, p.Samples)
		}
	}
}

func TestIMA(t *testing.T) {
	// One mono block: predictor 0, step index 0, then 8 nibbles.
	block := []byte{0, 0, 0, 0, 0x44, 0x0c, 0, 0}
	p, err := decodeWAV(buildWAV(wavIMAADPCM, 1, 8000, 4, 8, []byte{9, 0}, block, 5))
	if err != nil {
		t.Fatal(err)
	}
	// Worked by hand from the IMA step and index tables. The fact chunk
	// cuts the block's 9 frames to 5.
	want := []float32{0, 7, 17, 5, 6}
	if len(p.Samples) != len(want) {
		t.Fatalf("%d samples, want %d", len(p.Samples), len(want))
	}
	for i, v := range want {
		if p.Samples[i] != v/32768 {
			t.Errorf("sample %d = %v, want %v", i, p.Samples[i]*32768, v)
		}
	}
}

func TestMSADPCM(t *testing.T) {
	// Predictor 0 (256, 0), step 16, samples 100 and 50, then nibbles 1, -1.
	block := []byte{0, 16, 0, 100, 0, 50, 0, 0x1f}
	extra := []byte{4, 0, 1, 0, 0, 1, 0, 0} // 4 frames per block, 1 coefficient pair
	p, err := decodeWAV(buildWAV(wavMSADPCM, 1, 8000, 4, 8, extra, block, -1))
	if err != nil {
		t.Fatal(err)
	}
	// The older sample comes first. Then 100 + 16 = 116, and
	// 116 - delta, where delta = max(16, 230 × 16 >> 8) = 16, is 100.
	want := []float32{50, 100, 116, 100}
	for i, v := range want {
		if i >= len(p.Samples) || p.Samples[i] != v/32768 {
			t.Fatalf("samples %v, want %v / 32768", p.Samples, want)
		}
	}
}

func TestRejects(t *testing.T) {
	for name, data := range map[string][]byte{
		"not riff":   []byte("RIFX"),
		"3 channels": buildWAV(wavPCM, 3, 8000, 16, 6, nil, make([]byte, 12), -1),
		"low rate":   buildWAV(wavPCM, 1, 10, 16, 2, nil, make([]byte, 4), -1),
		"12-bit":     buildWAV(wavPCM, 1, 8000, 12, 2, nil, make([]byte, 4), -1),
		"empty":      buildWAV(wavPCM, 1, 8000, 16, 2, nil, nil, -1),
		"mp3":        buildWAV(0x55, 1, 8000, 0, 1, nil, make([]byte, 4), -1),
	} {
		if _, err := decodeWAV(data); err == nil {
			t.Errorf("%s: decoded", name)
		}
	}
}

// tone is a deterministic test signal: a triangle wave whose period
// shrinks over time, plus a little noise. It uses only integer math, so
// the input is the same on every platform, and each sample converts to
// float exactly.
func tone(rate, channels int, seconds float64) *PCM {
	n := int(float64(rate) * seconds)
	p := &PCM{Rate: rate, Channels: channels, Samples: make([]float32, n*channels)}
	period := rate / 200
	phase, noise := 0, uint32(1)
	for i := range n {
		if i%(rate/20) == 0 && period > 20 {
			period -= period / 8
		}
		phase = (phase + 1) % period
		v := 4*16000*phase/period - 16000
		if phase >= period/2 {
			v = 3*16000 - 4*16000*phase/period
		}
		noise ^= noise << 13
		noise ^= noise >> 17
		noise ^= noise << 5
		v += int(noise%512) - 256
		for c := range channels {
			p.Samples[i*channels+c] = float32(v*(c+1)/channels) / 32768
		}
	}
	return p
}

// element is one EBML element found by walk.
type element struct {
	id   uint32
	body []byte
}

func readVint(b []byte, keepMarker bool) (uint64, int) {
	if len(b) == 0 || b[0] == 0 {
		return 0, 0
	}
	l := 1
	for b[0]&(0x80>>(l-1)) == 0 {
		l++
	}
	if l > len(b) {
		return 0, 0
	}
	v := uint64(b[0])
	if !keepMarker {
		v &= uint64(0xff >> l)
	}
	for _, x := range b[1:l] {
		v = v<<8 | uint64(x)
	}
	return v, l
}

// walk lists the elements in b, going into the containers in open.
func walk(t *testing.T, b []byte, open map[uint32]bool) []element {
	t.Helper()
	var out []element
	for len(b) > 0 {
		id, l := readVint(b, true)
		size, sl := readVint(b[l:], false)
		if l == 0 || sl == 0 || uint64(len(b)-l-sl) < size {
			t.Fatalf("bad element at %x", b[:min(len(b), 8)])
		}
		body := b[l+sl : l+sl+int(size)]
		id32 := uint32(id)
		out = append(out, element{id32, body})
		if open[id32] {
			out = append(out, walk(t, body, open)...)
		}
		b = b[l+sl+int(size):]
	}
	return out
}

func TestEncode(t *testing.T) {
	// The hashes pin the encoder's output. Builds are stored by content
	// hash, so the same input must give the same bytes on every platform.
	// A new opus.wasm that changes them needs a new converter version.
	for _, c := range []struct {
		pcm  *PCM
		hash string
	}{
		{tone(22050, 1, 0.5), "edf720703b0f5690277eb777130a33106249181775d789903047e05f6fab566b"},
		{tone(44100, 2, 0.3), "7fd01841a2d1efed06387017d90ef45b4471f28936da6f067700de789b075878"},
		{tone(48000, 1, 0.02), "92e8049407be1167b654038eedd1ad0d375e00c78055f57b773be8a28748e680"},
	} {
		w, err := Encode(c.pcm)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(w)
		t.Logf("%d Hz, %d channels: %d bytes, %s", c.pcm.Rate, c.pcm.Channels, len(w), hex.EncodeToString(sum[:]))
		els := walk(t, w, map[uint32]bool{
			idEBML: false, idSegment: true, idTracks: true, idTrackEntry: true,
			idCluster: true, idBlockGroup: true,
		})
		var delay, discard uint64
		var head []byte
		blocks := 0
		for _, e := range els {
			switch e.id {
			case idCodecDelay:
				delay, _ = readUint(e.body)
			case idDiscardPadding:
				discard, _ = readUint(e.body)
			case idCodecPrivate:
				head = e.body
			case idSimpleBlock, idBlock:
				blocks++
			}
		}
		if len(head) != 19 || string(head[:8]) != "OpusHead" || int(head[9]) != c.pcm.Channels {
			t.Fatalf("OpusHead %x", head)
		}
		preSkip := int(binary.LittleEndian.Uint16(head[10:]))
		if delay != samplesToNs(preSkip) {
			t.Errorf("codec delay %d ns, pre-skip %d", delay, preSkip)
		}
		// The decoded length, after the pre-skip and the discarded end, is
		// the input's length at 48 kHz.
		want := (c.pcm.Frames()*48000 + c.pcm.Rate/2) / c.pcm.Rate
		got := blocks*frameSamples - preSkip - int((discard*48000+500_000_000)/1_000_000_000)
		if got != want {
			t.Errorf("%d Hz: decodes to %d frames, want %d", c.pcm.Rate, got, want)
		}
		if hex.EncodeToString(sum[:]) != c.hash {
			t.Errorf("%d Hz: output changed: %x", c.pcm.Rate, sum)
		}
	}
}

func readUint(b []byte) (uint64, bool) {
	var v uint64
	for _, x := range b {
		v = v<<8 | uint64(x)
	}
	return v, len(b) <= 8
}

func FuzzDecodeWAV(f *testing.F) {
	f.Add(buildWAV(wavPCM, 1, 8000, 16, 2, nil, []byte{1, 2, 3, 4}, -1))
	f.Add(buildWAV(wavIMAADPCM, 2, 8000, 4, 16, []byte{9, 0}, make([]byte, 20), 3))
	f.Add(buildWAV(wavMSADPCM, 1, 8000, 4, 8, []byte{4, 0, 1, 0, 0, 1, 0, 0}, []byte{0, 16, 0, 100, 0, 50, 0, 0x1f}, -1))
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := decodeWAV(data)
		if err != nil {
			return
		}
		if p.Channels < 1 || p.Channels > 2 || len(p.Samples)%p.Channels != 0 || p.Frames() > MaxFrames(p.Rate) {
			t.Fatalf("decoded %d channels, %d samples", p.Channels, len(p.Samples))
		}
		for _, v := range p.Samples {
			if !(v >= -1 && v <= 1) {
				t.Fatalf("sample %v", v)
			}
		}
	})
}
