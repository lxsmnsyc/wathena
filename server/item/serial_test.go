package item

import (
	"testing"
	"time"
)

func TestSerial(t *testing.T) {
	at := time.UnixMilli(0x0123456789ab)
	s := serialAt(at)
	if s[0] != 0x01 || s[5] != 0xab {
		t.Errorf("timestamp bytes % x", s[:6])
	}
	if s[6]>>4 != 7 || s[8]>>6 != 2 {
		t.Errorf("version or variant wrong: %s", s)
	}
	if got := s.String(); len(got) != 36 || got[:13] != "01234567-89ab" || got[14] != '7' {
		t.Errorf("String() = %s", got)
	}
	if s.IsZero() || !(Serial{}).IsZero() {
		t.Error("IsZero is wrong")
	}
	if serialAt(at) == s {
		t.Error("two serials in the same millisecond are equal")
	}
}

func TestParseSerial(t *testing.T) {
	s := NewSerial()
	got, ok := ParseSerial(s.String())
	if !ok || got != s {
		t.Fatalf("ParseSerial(%s) = %s, %v", s, got, ok)
	}
	upper, ok := ParseSerial("0192F0A8-1234-7ABC-8DEF-0123456789AB")
	if !ok || upper.String() != "0192f0a8-1234-7abc-8def-0123456789ab" {
		t.Errorf("upper case: %s, %v", upper, ok)
	}
	for _, bad := range []string{"", "x", "00000000-0000-0000-0000-000000000000", "0192f0a8-1234-7abc-8def-0123456789ag",
		"0192f0a8x1234-7abc-8def-0123456789ab", "0192f0a8-1234-7abc-8def-0123456789abc"} {
		if _, ok := ParseSerial(bad); ok {
			t.Errorf("ParseSerial(%q) is ok", bad)
		}
	}
}
