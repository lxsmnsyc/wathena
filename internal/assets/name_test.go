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

func TestModNames(t *testing.T) {
	good := []string{"mod/example.lantern/icon/item/Paper_Lantern", "mod/a/sprite/mob/X", "mod/a-b.c_d/texture/x"}
	bad := []string{"mod/Example/icon/item/x", "mod/example.lantern/Icon/item/x", "mod/example.lantern/icon", "mod/example.lantern",
		"mod/.x/icon/a", "mod/x/icon/a:b", "Mod/x/icon/a"}
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
	if id, rest, ok := SplitMod("mod/example.lantern/icon/item/Paper_Lantern"); !ok || id != "example.lantern" || rest != "icon/item/Paper_Lantern" {
		t.Errorf("SplitMod: %q %q %v", id, rest, ok)
	}
	if _, _, ok := SplitMod("icon/item/501"); ok {
		t.Error("SplitMod accepted a core name")
	}
	for name, want := range map[string]string{
		"mod/example.lantern/icon/item/Paper_Lantern": "texture",
		"mod/x/sprite/mob/Ghost":                      "sprite",
		"sprite/mob/1002":                             "sprite",
		"lightmap/prontera":                           "texture",
		"placeholder/sprite":                          "",
	} {
		if got := TypeOf(name); got != want {
			t.Errorf("TypeOf(%q) = %q, want %q", name, got, want)
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
