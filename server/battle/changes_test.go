package battle

import "testing"

// TestWatkElementPreRenewal checks pre-renewal's SC_WATK_ELEMENT in
// battle_calc_element_damage: a player's hit gains a percent of the
// damage a normal attack of the status's element would deal.
func TestWatkElementPreRenewal(t *testing.T) {
	player := func(c *Changes) *Status {
		return &Status{
			Level: 99, Hit: 300, Str: 100, Dex: 50, BAtk: 50,
			RHW:     Weapon{Atk: 100, Wlv: 1, Range: 1},
			Player:  &Player{Weapon: WeaponMace, RightWeapon: WeaponMace},
			Changes: c,
		}
	}
	fire := &Changes{AtkPercent: 100, WatkElement: Fire, WatkElementRate: 20}
	earth := func(def, def2 int) *Status {
		return &Status{Level: 1, Size: 1, DefEle: Earth, EleLv: 1, Def: def, Def2: def2}
	}
	fireOnEarth := int64(PreRenewalAttribute(1, Fire, Earth))
	tests := []struct {
		name   string
		src    *Status
		target *Status
		want   int64
	}{
		// Forced rolls give 100. The bonus is 20% of 100 as fire.
		{"plain", player(fire), earth(0, 0), 100 + 100*fireOnEarth/100*20/100},
		// The base damage loses DEF too: 100 * 50% - 40 = 10.
		{"DEF", player(fire), earth(50, 40), 10 + 10*fireOnEarth/100*20/100},
		{"no status", player(nil), earth(0, 0), 100},
	}
	for _, tc := range tests {
		got := AttackPreRenewal(tc.src, tc.target, forcedMin{})
		if got.Damage != tc.want {
			t.Errorf("%s: damage %d, want %d", tc.name, got.Damage, tc.want)
		}
	}
	// Monsters do not get it.
	mob := &Status{Level: 50, RHW: Weapon{Atk: 100, Atk2: 100}, Hit: 300, Changes: fire}
	plain := *mob
	plain.Changes = nil
	if a, b := AttackPreRenewal(mob, earth(0, 0), forcedMin{}), AttackPreRenewal(&plain, earth(0, 0), forcedMin{}); a.Damage != b.Damage {
		t.Errorf("monster damage %d, want %d", a.Damage, b.Damage)
	}
}

// TestSubWeaponProperty checks renewal's SC_SUB_WEAPONPROPERTY in
// battle_calc_damage_parts: weapon ATK gains a percent of itself as
// damage of the status's element.
func TestSubWeaponProperty(t *testing.T) {
	src := &Status{
		Level: 99, Str: 100, Dex: 50, BAtk: 50,
		RHW:    Weapon{Atk: 100, Wlv: 1, Range: 1},
		Player: &Player{Weapon: WeaponMace, RightWeapon: WeaponMace},
	}
	target := &Status{Level: 1, DefEle: Earth, EleLv: 1}
	plain := initializeWeaponData(src, nil)
	calcDamageParts(&plain, src, target, false, forcedMin{})

	with := *src
	with.Changes = &Changes{AtkPercent: 100, SubWeaponElement: Fire, SubWeaponRate: 20}
	wd := initializeWeaponData(&with, nil)
	calcDamageParts(&wd, &with, target, false, forcedMin{})

	// The weapon is neutral, which deals 100% to Earth 1.
	bonus := AttrFix(plain.weaponAtk*20/100, Fire, Earth, 1, 0)
	if wd.weaponAtk != plain.weaponAtk+bonus || bonus == 0 {
		t.Errorf("weapon ATK %d, want %d + %d", wd.weaponAtk, plain.weaponAtk, bonus)
	}
	if wd.statusAtk != plain.statusAtk || wd.equipAtk != plain.equipAtk {
		t.Errorf("other parts changed: %+v", wd)
	}
}

// TestWatkElementRenewal checks renewal's SC_WATK_ELEMENT in
// battle_attack_sc_bonus: it raises the damage through weapon ATK.
func TestWatkElementRenewal(t *testing.T) {
	src := func(c *Changes) *Status {
		return &Status{
			Level: 99, Hit: 300, Str: 100, Dex: 50, BAtk: 50,
			RHW:     Weapon{Atk: 100, Wlv: 1, Range: 1},
			Player:  &Player{Weapon: WeaponMace, RightWeapon: WeaponMace},
			Changes: c,
		}
	}
	target := &Status{Level: 1, DefEle: Neutral, EleLv: 1}
	a := Attack(src(nil), target, forcedMin{})
	b := Attack(src(&Changes{AtkPercent: 100, WatkElement: Fire, WatkElementRate: 100}), target, forcedMin{})
	if b.Damage <= a.Damage {
		t.Errorf("damage %d with the status, %d without", b.Damage, a.Damage)
	}
}
