package account

import (
	"bytes"
	"strings"
	"testing"
)

func TestTokens(t *testing.T) {
	a, ha := newToken()
	b, hb := newToken()
	if len(a) != TokenLen || len(b) != TokenLen || a == b || bytes.Equal(ha, hb) || len(ha) != 32 {
		t.Fatalf("tokens %q %q", a, b)
	}
	got, ok := tokenHash(a)
	if !ok || !bytes.Equal(got, ha) {
		t.Error("a token's hash does not match")
	}
	for _, bad := range []string{"", a[:42], a + "A", strings.Repeat("!", 43), "Alice", a[:42] + "="} {
		if _, ok := tokenHash(bad); ok {
			t.Errorf("%q is a token", bad)
		}
	}
}

func FuzzTokenHash(f *testing.F) {
	tok, _ := newToken()
	f.Add(tok)
	f.Add("Alice")
	f.Fuzz(func(t *testing.T, s string) {
		h, ok := tokenHash(s)
		if !ok {
			return
		}
		if len(s) != TokenLen || len(h) != 32 {
			t.Fatalf("%q accepted", s)
		}
		// Only one text maps to each token, so a token cannot be sent in
		// two spellings.
		b, _ := tokenEncoding.Strict().DecodeString(s)
		if tokenEncoding.EncodeToString(b) != s {
			t.Fatalf("%q is not canonical", s)
		}
	})
}
