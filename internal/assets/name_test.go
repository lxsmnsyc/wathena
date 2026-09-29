package assets

import "testing"

func TestValidName(t *testing.T) {
	good := []string{"sprite/mob/poring", "texture/prontera/ground01", "texture/유저인터페이스/login", "music/08"}
	bad := []string{"", "Sprite/mob/poring", "sprite//poring", "sprite/../x", "sprite/./x", "/sprite", "sprite/", `a\b`, "a:b", "a\x00b", "a\nb",
		"texture/가", // decomposed Hangul, not NFC
		string(make([]byte, 256))}
	for _, n := range good {
		if err := ValidName(n); err != nil {
			t.Errorf("%q rejected", n)
		}
	}
	for _, n := range bad {
		if ValidName(n) == nil {
			t.Errorf("%q accepted", n)
		}
	}
}

func FuzzValidName(f *testing.F) {
	f.Add("sprite/mob/poring")
	f.Fuzz(func(t *testing.T, s string) {
		if ValidName(s) != nil {
			return
		}
		// A valid name never has a segment that moves up a folder.
		for _, seg := range []string{"/../", "/./"} {
			if contains("/"+s+"/", seg) {
				t.Fatalf("%q accepted with %q", s, seg)
			}
		}
	})
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
