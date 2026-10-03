package battle

import "testing"

// TestSizeTableFlagBothBases checks both values of the size_table
// ruleset flag with each base's attack: a knuckle against a large target
// does 75% with renewal's table and 50% with pre-renewal's, and a dagger,
// which both tables treat alike, does the same either way.
func TestSizeTableFlagBothBases(t *testing.T) {
	attacker := func(weapon int, pre bool) *Status {
		return &Status{
			Level: 99, Hit: 500, Str: 100, Dex: 50, BAtk: 50,
			RHW:    Weapon{Atk: 100, Wlv: 1, Range: 1},
			Player: &Player{Weapon: weapon, RightWeapon: weapon, PreRenewalSizes: pre},
		}
	}
	target := &Status{Level: 1, MaxHP: 1000, Size: SizeLarge, EleLv: 1}
	for _, base := range []struct {
		name   string
		attack attackFunc
	}{{"renewal", Attack}, {"pre-renewal", AttackPreRenewal}} {
		re := base.attack(attacker(WeaponKnuckle, false), target, forcedMin{}).Damage
		pre := base.attack(attacker(WeaponKnuckle, true), target, forcedMin{}).Damage
		if re <= pre || re == 0 {
			t.Errorf("%s base: knuckle on a large target %d with renewal's table, %d with pre-renewal's; want renewal's higher", base.name, re, pre)
		}
		if a, b := base.attack(attacker(WeaponDagger, false), target, forcedMin{}).Damage, base.attack(attacker(WeaponDagger, true), target, forcedMin{}).Damage; a != b {
			t.Errorf("%s base: dagger %d and %d, want equal", base.name, a, b)
		}
	}
}
