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
	for _, n := range []string{"sprite/body/novice/f", "sound/attack_bow1", "effect/pong3", "effect/angel", "sound/levelup"} {
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
		"no parts":  "effects:\n  - {id: 1, constant: A, parts: []}\n",
		"both":      "effects:\n  - {id: 1, constant: A, parts: [{effect: effect/a, variants: [effect/b]}]}\n",
		"bad name":  "effects:\n  - {id: 1, constant: A, parts: [{effect: texture/a}]}\n",
		"sound too": "effects:\n  - {id: 1, constant: A, parts: [{effect: effect/a, sound: sound/b}]}\n",
		"bad sound": "effects:\n  - {id: 1, constant: A, parts: [{sound: effect/b}]}\n",
	} {
		if _, err := ParseEffects([]byte(y)); err == nil {
			t.Errorf("effects, %s: accepted", name)
		}
	}
}
