package account

import (
	"bytes"
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// rfcSecret is the SHA-1 secret of RFC 6238's test vectors.
var rfcSecret = []byte("12345678901234567890")

func TestTOTPCodeMatchesRFC6238(t *testing.T) {
	// RFC 6238 appendix B gives 8 digits. The last 6 are the 6-digit code.
	for _, v := range []struct {
		unix int64
		code string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	} {
		if got := totpCode(rfcSecret, totpStep(time.Unix(v.unix, 0))); got != v.code {
			t.Errorf("%d: %s, want %s", v.unix, got, v.code)
		}
	}
}

func TestCheckTOTP(t *testing.T) {
	now := time.Unix(1_800_000_015, 0)
	cur := totpStep(now)
	code := func(step int64) string { return totpCode(rfcSecret, step) }
	for _, c := range []struct {
		name  string
		code  string
		last  int64
		ok    bool
		match int64
	}{
		{"current step", code(cur), 0, true, cur},
		{"one step early (skew)", code(cur - 1), 0, true, cur - 1},
		{"one step late (skew)", code(cur + 1), 0, true, cur + 1},
		{"two steps early", code(cur - 2), 0, false, 0},
		{"two steps late", code(cur + 2), 0, false, 0},
		{"replay of the accepted step", code(cur), cur, false, 0},
		{"older step after a newer one", code(cur - 1), cur, false, 0},
		{"next step after the current one", code(cur + 1), cur, true, cur + 1},
		{"not digits", "12a456", 0, false, 0},
		{"too short", "12345", 0, false, 0},
		{"too long", "1234567", 0, false, 0},
	} {
		step, ok := checkTOTP(rfcSecret, c.code, now, c.last)
		if ok != c.ok || step != c.match {
			t.Errorf("%s: %d %v, want %d %v", c.name, step, ok, c.match, c.ok)
		}
	}
}

func TestNormalizeCode(t *testing.T) {
	if got := normalizeCode(" 123 456 "); got != "123456" {
		t.Errorf("spaces: %q", got)
	}
	if got := normalizeCode("abcd-efgh"); got != "abcdefgh" {
		t.Errorf("dashes: %q", got)
	}
}

func TestTOTPSetupPieces(t *testing.T) {
	secret := newTOTPSecret()
	if len(secret) != 20 || bytes.Equal(secret, newTOTPSecret()) {
		t.Fatalf("secret %x", secret)
	}
	key := formatKey(secret)
	back, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ReplaceAll(key, " ", ""))
	if err != nil || !bytes.Equal(back, secret) || strings.Count(key, " ") != 7 {
		t.Errorf("key %q: %x %v", key, back, err)
	}
	uri, err := totpURI("My Server", "Alice_1", secret)
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"otpauth://totp/My%20Server:Alice_1?", "secret=" + strings.ReplaceAll(key, " ", ""), "issuer=My%20Server", "digits=6", "period=30", "algorithm=SHA1"} {
		if !strings.Contains(uri, part) {
			t.Errorf("URI %q lacks %q", uri, part)
		}
	}
	svg, err := qrSVG(uri)
	if err != nil {
		t.Fatal(err)
	}
	s := string(svg)
	if !strings.HasPrefix(s, "<svg ") || !strings.HasSuffix(s, "</svg>") || strings.Count(s, "h1v1h-1z") < 100 {
		t.Errorf("QR SVG: %.200s", s)
	}
	// The SVG holds only drawing commands, never the secret.
	if strings.Contains(s, strings.ReplaceAll(key, " ", "")) || strings.Contains(s, "otpauth") {
		t.Error("the SVG contains the secret")
	}
}

func TestRecoveryCodes(t *testing.T) {
	codes, hashes := newRecoveryCodes()
	if len(codes) != RecoveryCodes || len(hashes) != RecoveryCodes {
		t.Fatalf("%d codes, %d hashes", len(codes), len(hashes))
	}
	seen := map[string]bool{}
	for i, c := range codes {
		if len(c) != 19 || c[4] != '-' || c[9] != '-' || c[14] != '-' || seen[c] {
			t.Errorf("code %q", c)
		}
		seen[c] = true
		for _, typed := range []string{c, strings.ToUpper(c), strings.ReplaceAll(c, "-", ""), strings.ReplaceAll(c, "-", " ")} {
			h, ok := recoveryHash(typed)
			if !ok || !bytes.Equal(h, hashes[i]) {
				t.Errorf("%q does not match its hash", typed)
			}
		}
	}
	for _, bad := range []string{"", "123456", "abcd-efgh-2345", "abcd-efgh-2345-67ab-cdef", "abcd-efgh-2345-6710", "abcd-efgh-2345-67a!"} {
		if _, ok := recoveryHash(bad); ok {
			t.Errorf("%q passed", bad)
		}
	}
}

func TestSealer(t *testing.T) {
	k1, k2 := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32)
	s1, err := NewSealer(k1)
	if err != nil {
		t.Fatal(err)
	}
	secret := []byte("a totp secret of 20 b")
	sealed := s1.seal(secret, totpAD(7))
	if bytes.Contains(sealed, secret) || len(sealed) != sealHeader+len(secret)+16 {
		t.Fatalf("sealed %x", sealed)
	}
	if again := s1.seal(secret, totpAD(7)); bytes.Equal(again, sealed) {
		t.Error("two seals of one value are equal; the nonce is not random")
	}
	got, current, err := s1.open(sealed, totpAD(7))
	if err != nil || !current || !bytes.Equal(got, secret) {
		t.Fatalf("round trip: %q %v %v", got, current, err)
	}
	// Another account's row, a wrong key, and a changed byte all fail.
	if _, _, err := s1.open(sealed, totpAD(8)); err == nil {
		t.Error("opened with another account's data")
	}
	s2, _ := NewSealer(k2)
	if _, _, err := s2.open(sealed, totpAD(7)); err == nil {
		t.Error("opened with the wrong key")
	}
	for i := range sealed {
		bad := bytes.Clone(sealed)
		bad[i] ^= 1
		if _, _, err := s1.open(bad, totpAD(7)); err == nil {
			t.Errorf("opened with byte %d changed", i)
		}
	}
	if _, _, err := s1.open(sealed[:10], totpAD(7)); err == nil {
		t.Error("opened a short value")
	}
	// After a key change the old key still opens old values, and says
	// they need sealing again.
	rotated, err := NewSealer(k2, k1)
	if err != nil {
		t.Fatal(err)
	}
	got, current, err = rotated.open(sealed, totpAD(7))
	if err != nil || current || !bytes.Equal(got, secret) {
		t.Errorf("old key: %q %v %v", got, current, err)
	}
	if _, err := NewSealer(k1[:16]); err == nil {
		t.Error("a 16-byte key was taken")
	}
}

func TestParseSecretKey(t *testing.T) {
	want := bytes.Repeat([]byte{0xfb}, 32)
	for _, text := range []string{
		"+/v7+/v7+/v7+/v7+/v7+/v7+/v7+/v7+/v7+/v7+/s=",
		"+/v7+/v7+/v7+/v7+/v7+/v7+/v7+/v7+/v7+/v7+/s",
		"-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_v7-_s",
		" +/v7+/v7+/v7+/v7+/v7+/v7+/v7+/v7+/v7+/v7+/s=\n",
	} {
		got, err := ParseSecretKey(text)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("%q: %x %v", text, got, err)
		}
	}
	for _, text := range []string{"", "c2hvcnQ=", "not base64 at all!!", strings.Repeat("A", 60)} {
		_, err := ParseSecretKey(text)
		if err == nil {
			t.Errorf("%q passed", text)
			continue
		}
		if text != "" && strings.Contains(err.Error(), text) {
			t.Errorf("the error quotes the key: %v", err)
		}
	}
}

func TestRecent(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	window := 10 * time.Minute
	for _, c := range []struct {
		at time.Time
		ok bool
	}{
		{now, true},
		{now.Add(-9 * time.Minute), true},
		{now.Add(-10 * time.Minute), true},
		{now.Add(-10*time.Minute - time.Second), false},
		{now.Add(time.Hour), false},
		{time.Time{}, false},
	} {
		if got := recent(c.at, now, window); got != c.ok {
			t.Errorf("%v: %v", now.Sub(c.at), got)
		}
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"/account/settings":                    "/account/settings",
		"/account/settings?notice=x":           "/account/settings?notice=x",
		"":                                     "/account/settings",
		"https://evil.test/account/":           "/account/settings",
		"//evil.test/account/settings":         "/account/settings",
		"/other":                               "/account/settings",
		"/account/\\evil":                      "/account/settings",
		"javascript:alert(1)":                  "/account/settings",
		"/account/" + strings.Repeat("a", 600): "/account/settings",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}

func TestPasskeyName(t *testing.T) {
	for in, want := range map[string]string{
		"":                      "Passkey",
		"  Phone  ":             "Phone",
		"Laptop Touch ID":       "Laptop Touch ID",
		"Téléphone":             "Téléphone",
		"bad\x00name":           "",
		"new\nline":             "",
		strings.Repeat("x", 65): "",
	} {
		if got := passkeyName(in); got != want {
			t.Errorf("%q: %q", in, got)
		}
	}
}
