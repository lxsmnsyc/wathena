package bonus

import "testing"

// 0002a has 127 bonus names, plus skill.
func TestSpecCount(t *testing.T) {
	if len(Specs) != 128 || len(byName) != 128 {
		t.Errorf("%d specs, %d names", len(Specs), len(byName))
	}
}

func TestCheck(t *testing.T) {
	ok := []*Bonus{
		{Name: "str", Args: []Num{{Value: 5}}},
		{Name: "max_hp", Args: []Num{{Value: 10}}, Options: Options{Percent: true, Set: map[string]bool{"percent": true}}},
		{Name: "damage", Args: []Num{{Value: 10}}, Options: Options{Kind: "physical", Against: &Target{Key: "race", Value: 7}, Set: map[string]bool{"kind": true, "against": true}}},
		{Name: "unbreakable_weapon"},
	}
	for _, b := range ok {
		if err := Check(b); err != nil {
			t.Errorf("%s: %v", b.Name, err)
		}
	}
	bad := []*Bonus{
		{Name: "str"},
		{Name: "natural_hp_recovery", Args: []Num{{Value: 5}}},
		{Name: "cast", Args: []Num{{Value: 19}, {Value: 3}}},
		{Name: "damage_taken", Args: []Num{{Value: -5}}, Options: Options{Kind: "magic", From: &Target{Key: "race", Value: 7}, Set: map[string]bool{"kind": true, "from": true}}},
	}
	for _, b := range bad {
		if err := Check(b); err == nil {
			t.Errorf("%s %+v passed", b.Name, b)
		}
	}
}
