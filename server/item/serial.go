package item

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"time"
)

// Serial identifies one item stack or instance, so item logs can follow
// it and duplicated or stolen items can be found (strategy section 3.5).
// It is a UUID version 7, as PostgreSQL's uuidv7(). The zero Serial means
// none.
type Serial [16]byte

// NewSerial makes a serial from the current time and random bits.
func NewSerial() Serial {
	return serialAt(time.Now())
}

func serialAt(t time.Time) Serial {
	var s Serial
	if _, err := rand.Read(s[6:]); err != nil {
		panic(err)
	}
	var ms [8]byte
	binary.BigEndian.PutUint64(ms[:], uint64(t.UnixMilli())) // #nosec G115 -- the clock is after 1970.
	copy(s[:6], ms[2:])
	s[6] = s[6]&0x0f | 0x70 // version 7
	s[8] = s[8]&0x3f | 0x80 // RFC 9562 variant
	return s
}

// IsZero reports whether s is the zero Serial.
func (s Serial) IsZero() bool { return s == Serial{} }

// String is the UUID's text form.
func (s Serial) String() string {
	var b [36]byte
	hex.Encode(b[0:8], s[0:4])
	b[8] = '-'
	hex.Encode(b[9:13], s[4:6])
	b[13] = '-'
	hex.Encode(b[14:18], s[6:8])
	b[18] = '-'
	hex.Encode(b[19:23], s[8:10])
	b[23] = '-'
	hex.Encode(b[24:], s[10:])
	return string(b[:])
}

// ParseSerial reads a serial in the UUID's text form, in any case. ok
// is false for any other text and for the zero UUID.
func ParseSerial(text string) (s Serial, ok bool) {
	if len(text) != 36 || text[8] != '-' || text[13] != '-' || text[18] != '-' || text[23] != '-' {
		return Serial{}, false
	}
	hexText := text[0:8] + text[9:13] + text[14:18] + text[19:23] + text[24:]
	if _, err := hex.Decode(s[:], []byte(hexText)); err != nil {
		return Serial{}, false
	}
	return s, !s.IsZero()
}
