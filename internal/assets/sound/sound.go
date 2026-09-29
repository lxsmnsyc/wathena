// Package sound converts audio into assets-v1 sounds: Opus in WebM at
// 48 kHz, as defined in docs/specs/assets-v1.md, "Sound and music".
package sound

import (
	"bytes"
	"errors"
	"fmt"
)

var ErrFormat = errors.New("sound: invalid audio")

// Input limits.
const (
	MinRate    = 1000
	MaxRate    = 384000
	MaxSeconds = 600
	// MaxBytes is the largest encoded sound.
	MaxBytes = 8 << 20
)

// MaxFrames returns the most samples per channel a sound can have at rate.
func MaxFrames(rate int) int { return MaxSeconds * rate }

// Bitrate returns the Opus bitrate for a channel count: 32 kbit/s per
// channel. At this rate, 22 kHz game sounds have no audible loss.
func Bitrate(channels int) int { return 32000 * channels }

// DecodeWAV reads a WAV file.
func DecodeWAV(data []byte) (*PCM, error) { return decodeWAV(data) }

// Encode encodes audio as Opus in WebM.
func Encode(p *PCM) ([]byte, error) {
	enc, err := encodeOpus(p, Bitrate(p.Channels))
	if err != nil {
		return nil, err
	}
	out := muxWebM(enc, p.Channels, p.Rate)
	if len(out) > MaxBytes {
		return nil, fmt.Errorf("%w: encoded sound is %d bytes", ErrFormat, len(out))
	}
	return out, nil
}

// ConvertWAV converts a WAV file to Opus in WebM.
func ConvertWAV(data []byte) ([]byte, error) {
	p, err := decodeWAV(data)
	if err != nil {
		return nil, err
	}
	return Encode(p)
}

// IsMP3 reports whether data starts like an MP3 file: an ID3 tag or an
// MPEG audio frame header.
func IsMP3(data []byte) bool {
	if len(data) >= 3 && string(data[:3]) == "ID3" {
		return true
	}
	return len(data) >= 2 && data[0] == 0xff && data[1]&0xe0 == 0xe0
}

// CheckWebM checks that data is a WebM file with an Opus track, as the
// build writes them.
func CheckWebM(data []byte) error {
	if len(data) > MaxBytes {
		return fmt.Errorf("%w: %d bytes", ErrFormat, len(data))
	}
	if len(data) < 4 || string(data[:4]) != "\x1a\x45\xdf\xa3" {
		return fmt.Errorf("%w: not an ebml file", ErrFormat)
	}
	head := data[:min(len(data), 64)]
	if !bytes.Contains(head, []byte("webm")) {
		return fmt.Errorf("%w: not a webm file", ErrFormat)
	}
	if !bytes.Contains(data[:min(len(data), 4096)], []byte("A_OPUS")) {
		return fmt.Errorf("%w: no opus track", ErrFormat)
	}
	return nil
}
