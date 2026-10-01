package clientdata

import "testing"

func TestLoadTables(t *testing.T) {
	w, err := LoadWeapons()
	if err != nil {
		t.Fatal(err)
	}
	if len(w.Weapons) != 24 || w.Weapons[11].Constant != "W_BOW" || len(w.Weapons[11].Attack) != 2 {
		t.Errorf("weapons %+v", w.Weapons)
	}
	e, err := LoadEffects()
	if err != nil {
		t.Fatal(err)
	}
	if len(e.Effects) == 0 || e.Effects[0].ID != 109 {
		t.Errorf("effects %+v", e.Effects)
	}
	refs, err := References()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, r := range refs {
		seen[r.Name] = true
	}
	for _, n := range []string{"sprite/body/novice/f", "sound/attack_bow1", "effect/pong3", "effect/angel", "sound/levelup", "texture/effect/ring_blue"} {
		if !seen[n] {
			t.Errorf("references lack %s", n)
		}
	}
}

func TestTablesReject(t *testing.T) {
	for name, y := range map[string]string{
		"unknown key": "weapons:\n  - {id: 1, constant: A, view: 1, colour: red}\n",
		"bad sound":   "weapons:\n  - {id: 1, constant: A, view: 1, attack: [attack_sword]}\n",
		"repeated id": "weapons:\n  - {id: 1, constant: A, view: 1}\n  - {id: 1, constant: B, view: 1}\n",
	} {
		if _, err := ParseWeapons([]byte(y)); err == nil {
			t.Errorf("weapons, %s: accepted", name)
		}
	}
	for name, y := range map[string]string{
		"no parts":         "effects:\n  - {id: 1, constant: A, parts: []}\n",
		"both":             "effects:\n  - {id: 1, constant: A, parts: [{effect: effect/a, variants: [effect/b]}]}\n",
		"bad name":         "effects:\n  - {id: 1, constant: A, parts: [{effect: texture/a}]}\n",
		"sound too":        "effects:\n  - {id: 1, constant: A, parts: [{effect: effect/a, sound: sound/b}]}\n",
		"bad sound":        "effects:\n  - {id: 1, constant: A, parts: [{sound: effect/b}]}\n",
		"cylinder texture": "effects:\n  - {id: 1, constant: A, parts: [{cylinder: {texture: effect/a, duration_ms: 1, copies: 1}}]}\n",
		"cylinder time":    "effects:\n  - {id: 1, constant: A, parts: [{cylinder: {texture: texture/a, duration_ms: 0, copies: 1}}]}\n",
		"cylinder color":   "effects:\n  - {id: 1, constant: A, parts: [{cylinder: {texture: texture/a, duration_ms: 1, copies: 1, color: [2, 0, 0]}}]}\n",
		"cylinder too":     "effects:\n  - {id: 1, constant: A, parts: [{sound: sound/a, cylinder: {texture: texture/a, duration_ms: 1, copies: 1}}]}\n",
	} {
		if _, err := ParseEffects([]byte(y)); err == nil {
			t.Errorf("effects, %s: accepted", name)
		}
	}
}

func TestNPCSprites(t *testing.T) {
	n, err := LoadNPCSprites()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range n.Sprites {
		if s.ID == 112 && s.Sprite == "4_f_kafra6" {
			found = true
		}
	}
	if len(n.Sprites) < 1000 || !found {
		t.Errorf("%d sprites, 112 found: %v", len(n.Sprites), found)
	}
	if _, err := ParseNPCSprites([]byte("sprites:\n  - {id: 1, sprite: a}\n  - {id: 1, sprite: b}\n")); err == nil {
		t.Error("a repeated ID passed")
	}
	if _, err := ParseNPCSprites([]byte("sprites:\n  - {id: 1, sprite: \"A B\"}\n")); err == nil {
		t.Error("an invalid name passed")
	}
}
