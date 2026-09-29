package sound

import (
	"encoding/binary"
	"math"
)

// EBML element IDs used in a WebM audio file.
const (
	idEBML               = 0x1a45dfa3
	idEBMLVersion        = 0x4286
	idEBMLReadVersion    = 0x42f7
	idEBMLMaxIDLength    = 0x42f2
	idEBMLMaxSizeLength  = 0x42f3
	idDocType            = 0x4282
	idDocTypeVersion     = 0x4287
	idDocTypeReadVersion = 0x4285
	idSegment            = 0x18538067
	idInfo               = 0x1549a966
	idTimestampScale     = 0x2ad7b1
	idDuration           = 0x4489
	idMuxingApp          = 0x4d80
	idWritingApp         = 0x5741
	idTracks             = 0x1654ae6b
	idTrackEntry         = 0xae
	idTrackNumber        = 0xd7
	idTrackUID           = 0x73c5
	idTrackType          = 0x83
	idFlagLacing         = 0x9c
	idCodecID            = 0x86
	idCodecPrivate       = 0x63a2
	idCodecDelay         = 0x56aa
	idSeekPreRoll        = 0x56bb
	idAudio              = 0xe1
	idSamplingFrequency  = 0xb5
	idChannels           = 0x9f
	idCluster            = 0x1f43b675
	idTimestamp          = 0xe7
	idSimpleBlock        = 0xa3
	idBlockGroup         = 0xa0
	idBlock              = 0xa1
	idDiscardPadding     = 0x75a2
)

const (
	// Timestamps are in milliseconds.
	timestampScale = 1_000_000
	// packetsPerCluster starts a new cluster every 5 seconds.
	packetsPerCluster = 250
	// seekPreRoll is the 80 ms the Opus spec asks decoders to decode
	// before a seek point.
	seekPreRoll = 80_000_000
)

func vint(n uint64) []byte {
	// Sizes use the shortest form. All ones is reserved for unknown sizes.
	for l := 1; l <= 8; l++ {
		if n < 1<<(7*l)-1 {
			b := make([]byte, l)
			for i := l - 1; i >= 0; i-- {
				b[i] = byte(n) // #nosec G115 -- one byte at a time.
				n >>= 8
			}
			b[0] |= 0x80 >> (l - 1)
			return b
		}
	}
	panic("webm: element too large")
}

func elem(id uint32, body ...[]byte) []byte {
	var b []byte
	switch {
	case id >= 1<<24:
		b = binary.BigEndian.AppendUint32(nil, id)
	case id >= 1<<16:
		b = []byte{byte(id >> 16), byte(id >> 8), byte(id)} // #nosec G115 -- 3-byte ID.
	case id >= 1<<8:
		b = binary.BigEndian.AppendUint16(nil, uint16(id)) // #nosec G115 -- 2-byte ID.
	default:
		b = []byte{byte(id)} // #nosec G115 -- 1-byte ID.
	}
	n := 0
	for _, p := range body {
		n += len(p)
	}
	b = append(b, vint(uint64(n))...) // #nosec G115 -- a length.
	for _, p := range body {
		b = append(b, p...)
	}
	return b
}

func uintElem(id uint32, v uint64) []byte {
	b := binary.BigEndian.AppendUint64(nil, v)
	for len(b) > 1 && b[0] == 0 {
		b = b[1:]
	}
	return elem(id, b)
}

func floatElem(id uint32, v float64) []byte {
	return elem(id, binary.BigEndian.AppendUint64(nil, math.Float64bits(v)))
}

// samplesToNs converts samples at 48 kHz to nanoseconds, rounded.
func samplesToNs(n int) uint64 {
	return (uint64(n)*1_000_000_000 + 24_000) / 48_000 // #nosec G115 -- a sample count.
}

// opusHead is the Ogg Opus identification header, which WebM stores as the
// track's codec private data.
func opusHead(channels, preSkip, inputRate int) []byte {
	b := []byte("OpusHead")
	b = append(b, 1, byte(channels))                           // #nosec G115 -- 1 or 2.
	b = binary.LittleEndian.AppendUint16(b, uint16(preSkip))   // #nosec G115 -- the encoder's lookahead.
	b = binary.LittleEndian.AppendUint32(b, uint32(inputRate)) // #nosec G115 -- at most MaxRate.
	b = binary.LittleEndian.AppendUint16(b, 0)                 // output gain
	return append(b, 0)                                        // channel mapping family
}

// muxWebM writes Opus packets as a WebM file. The track's codec delay
// holds the pre-skip, and the last block's discard padding trims the end,
// so the decoded length matches the input.
func muxWebM(s *opusStream, channels, inputRate int) []byte {
	header := elem(idEBML,
		uintElem(idEBMLVersion, 1),
		uintElem(idEBMLReadVersion, 1),
		uintElem(idEBMLMaxIDLength, 4),
		uintElem(idEBMLMaxSizeLength, 8),
		elem(idDocType, []byte("webm")),
		uintElem(idDocTypeVersion, 4),
		uintElem(idDocTypeReadVersion, 2),
	)
	info := elem(idInfo,
		uintElem(idTimestampScale, timestampScale),
		floatElem(idDuration, float64(s.frames)/48),
		elem(idMuxingApp, []byte("wathena")),
		elem(idWritingApp, []byte("wathena")),
	)
	tracks := elem(idTracks, elem(idTrackEntry,
		uintElem(idTrackNumber, 1),
		uintElem(idTrackUID, 1),
		uintElem(idTrackType, 2),
		uintElem(idFlagLacing, 0),
		elem(idCodecID, []byte("A_OPUS")),
		elem(idCodecPrivate, opusHead(channels, s.preSkip, inputRate)),
		uintElem(idCodecDelay, samplesToNs(s.preSkip)),
		uintElem(idSeekPreRoll, seekPreRoll),
		elem(idAudio,
			floatElem(idSamplingFrequency, 48000),
			uintElem(idChannels, uint64(channels)), // #nosec G115 -- 1 or 2.
		),
	))

	// The decoded stream is all packets, less the pre-skip at the start.
	// What remains past the input's length is discarded at the end.
	pad := len(s.packets)*frameSamples - s.preSkip - s.frames
	var clusters []byte
	for start := 0; start < len(s.packets); start += packetsPerCluster {
		end := min(start+packetsPerCluster, len(s.packets))
		clusterMs := start * frameSamples / 48
		parts := [][]byte{uintElem(idTimestamp, uint64(clusterMs))} // #nosec G115 -- positive.
		for i := start; i < end; i++ {
			rel := i*frameSamples/48 - clusterMs
			block := append([]byte{0x81, byte(rel >> 8), byte(rel)}, 0x80) // #nosec G115 -- under 5 seconds.
			if i == len(s.packets)-1 && pad > 0 {
				block[3] = 0
				disc := binary.BigEndian.AppendUint64(nil, samplesToNs(pad))
				for len(disc) > 1 && disc[0] == 0 && disc[1] < 0x80 {
					disc = disc[1:]
				}
				parts = append(parts, elem(idBlockGroup,
					elem(idBlock, block, s.packets[i]),
					elem(idDiscardPadding, disc),
				))
				continue
			}
			parts = append(parts, elem(idSimpleBlock, block, s.packets[i]))
		}
		clusters = append(clusters, elem(idCluster, parts...)...)
	}
	return append(header, elem(idSegment, info, tracks, clusters)...)
}
