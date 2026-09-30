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
