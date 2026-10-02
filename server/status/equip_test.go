package status

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/bonus"
	"github.com/lxsmnsyc/wathena/server/item"
)

func loadData(t *testing.T, mode reference.Mode) *Data {
	t.Helper()
	var d Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", &d); err != nil {
		t.Fatal(err)
	}
	return &d
}

// opts makes the options of a bonus.
func opts(set map[string]any) *bonus.Options {
	o := &bonus.Options{Set: map[string]bool{}}
	for k, v := range set {
		o.Set[k] = true
		switch k {
		case "percent":
			o.Percent = true
		case "stacking":
			o.Stacking = v.(string)
		case "priority":
			o.Priority = int64(v.(int))
		}
	}
	return o
}

// wearing puts items with bonuses in equip indexes.
func wearing(items map[int]*Equipped) *Equipment {
	e := &Equipment{MapType: item.MapNormal}
	for i := range e.Worn.Index {
		e.Worn.Index[i] = -1
	}
	for eqi, it := range items {
		e.Items[eqi] = it
		e.Worn.Index[eqi] = eqi
	}
	return e
}

func armor(bonuses ...Bonus) *Equipped {
	return &Equipped{Def: &item.Def{Type: "Armor", Location: item.Locations(item.EqpArmor)}, Position: item.EqpArmor, Bonuses: bonuses}
}

func TestStacking(t *testing.T) {
	tb := load(t, reference.Renewal)
	base := Character{Job: 0, Male: true, BaseLevel: 50, JobLevel: 10, Stats: builds["melee"]}
	calc := func(eq *Equipment) Result {
		t.Helper()
		c := base
		c.Equipment = eq
		r, err := tb.Calc(c)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	// Two "highest" speed bonuses keep the larger, and a "sum" one adds:
	// 20% + 5% faster.
	speed := func(v int64, stacking string) Bonus {
		o := map[string]any{"percent": true}
		if stacking != "" {
			o["stacking"] = stacking
		}
		return Bonus{Name: "move_speed", Args: []int64{v}, Options: opts(o)}
	}
	r := calc(wearing(map[int]*Equipped{
		item.EqiArmor:   armor(speed(10, "highest"), speed(20, "highest")),
		item.EqiGarment: armor(speed(5, "")),
	}))
	if r.Battle.Speed != 150*75/100 {
		t.Errorf("speed %d, want %d", r.Battle.Speed, 150*75/100)
	}
	// Of two "lowest" STR bonuses, the smaller counts.
	str := func(v int64) Bonus {
		return Bonus{Name: "str", Args: []int64{v}, Options: opts(map[string]any{"stacking": "lowest"})}
	}
	plain := calc(nil).Battle.Str
	if got := calc(wearing(map[int]*Equipped{item.EqiArmor: armor(str(3), str(7))})).Battle.Str; got != plain+3 {
		t.Errorf("STR %d, want %d", got, plain+3)
	}
	// One-value bonuses: the last in rAthena's order wins, unless a
	// priority is higher. Shoes (EQI_SHOES) run before armor.
	ele := func(e int64, o map[string]any) Bonus {
		return Bonus{Name: "defense_element", Args: []int64{e}, Options: opts(o)}
	}
	r = calc(wearing(map[int]*Equipped{
		item.EqiShoes: armor(ele(battle.Fire, nil)),
		item.EqiArmor: armor(ele(battle.Water, nil)),
	}))
	if r.Battle.DefEle != battle.Water {
		t.Errorf("defense element %d, want the armor's water", r.Battle.DefEle)
	}
	r = calc(wearing(map[int]*Equipped{
		item.EqiShoes: armor(ele(battle.Fire, map[string]any{"stacking": "priority", "priority": 5})),
		item.EqiArmor: armor(ele(battle.Water, nil)),
	}))
	if r.Battle.DefEle != battle.Fire {
		t.Errorf("defense element %d, want the shoes' fire by priority", r.Battle.DefEle)
	}
}

func TestMapRestrictedScripts(t *testing.T) {
	tb := load(t, reference.Renewal)
	c := Character{Job: 0, Male: true, BaseLevel: 50, JobLevel: 10, Stats: builds["melee"]}
	plain, err := tb.Calc(c)
	if err != nil {
		t.Fatal(err)
	}
	a := armor(Bonus{Name: "str", Args: []int64{5}, Options: opts(nil)})
	a.Def.NoEquip = item.MapNormal
	c.Equipment = wearing(map[int]*Equipped{item.EqiArmor: a})
	r, err := tb.Calc(c)
	if err != nil {
		t.Fatal(err)
	}
	if r.Battle.Str != plain.Battle.Str {
		t.Errorf("a restricted item's script ran: STR %d", r.Battle.Str)
	}
	c.Equipment.MapType = item.MapPvP
	if r, _ = tb.Calc(c); r.Battle.Str != plain.Battle.Str+5 {
		t.Errorf("the script did not run on a PvP map: STR %d", r.Battle.Str)
	}
}

// TestASPDFlag checks both values of the aspd ruleset flag on each base:
// the ASPD of a dagger follows the flag's formula and table.
func TestASPDFlag(t *testing.T) {
	re, pre := loadData(t, reference.Renewal), loadData(t, reference.PreRenewal)
	dagger := func() *Equipment {
		e := wearing(map[int]*Equipped{item.EqiHandR: {
			Def:      &item.Def{Type: "Weapon", Subtype: weaponDagger, Location: item.Locations(item.EqpHandR), WeaponLevel: 1},
			Position: item.EqpHandR,
		}})
		e.Worn.WeaponType1, e.Worn.Weapon = weaponDagger, weaponDagger
		return e
	}
	c := Character{Job: 0, Male: true, BaseLevel: 50, JobLevel: 10, Stats: builds["melee"]}
	c.Equipment = dagger()
	for _, tc := range []struct {
		name        string
		base        *Data
		flag        bool
		other       *Data
		flagTablesD *Data
	}{
		{"renewal base, renewal aspd", re, true, nil, re},
		{"renewal base, pre-renewal aspd", re, false, pre, pre},
		{"pre-renewal base, pre-renewal aspd", pre, false, nil, pre},
		{"pre-renewal base, renewal aspd", pre, true, re, re},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tb, err := New(tc.base)
			if err != nil {
				t.Fatal(err)
			}
			if err := tb.SetASPD(tc.flag, tc.other); err != nil {
				t.Fatal(err)
			}
			r, err := tb.Calc(c)
			if err != nil {
				t.Fatal(err)
			}
			// The same character on tables of the flag's own mode.
			own, err := New(tc.flagTablesD)
			if err != nil {
				t.Fatal(err)
			}
			want, err := own.Calc(c)
			if err != nil {
				t.Fatal(err)
			}
			// AGI and DEX are the same on both bases for a Novice, so the
			// ASPD must be the flag mode's.
			if r.Battle.Agi != want.Battle.Agi || r.Battle.Dex != want.Battle.Dex {
				t.Skipf("AGI or DEX differ between the bases: %d/%d and %d/%d", r.Battle.Agi, r.Battle.Dex, want.Battle.Agi, want.Battle.Dex)
			}
			if r.Battle.AMotion != want.Battle.AMotion {
				t.Errorf("amotion %d, want %d", r.Battle.AMotion, want.Battle.AMotion)
			}
		})
	}
	tb, _ := New(re)
	if err := tb.SetASPD(false, nil); err == nil {
		t.Error("the pre-renewal aspd flag was set without its job table")
	}
}

// TestSizeTableFlag checks both values of the size_table ruleset flag.
func TestSizeTableFlag(t *testing.T) {
	tb := load(t, reference.Renewal)
	c := Character{Job: 0, Male: true, BaseLevel: 50, JobLevel: 10, Stats: builds["melee"]}
	for _, pre := range []bool{false, true} {
		tb.SetSizeTable(pre)
		r, err := tb.Calc(c)
		if err != nil {
			t.Fatal(err)
		}
		if r.Battle.Player.PreRenewalSizes != pre {
			t.Errorf("size_table pre_renewal=%v: player has %v", pre, r.Battle.Player.PreRenewalSizes)
		}
	}
	if battle.SizeFix(battle.WeaponKnuckle, battle.SizeLarge) != 75 || battle.PreRenewalSizeFix(battle.WeaponKnuckle, battle.SizeLarge) != 50 ||
		battle.PreRenewalSizeFix(battle.WeaponKnuckle, battle.SizeMedium) != 75 || battle.PreRenewalSizeFix(battle.WeaponWhip, battle.SizeLarge) != 50 ||
		battle.PreRenewalSizeFix(battle.WeaponDagger, battle.SizeLarge) != battle.SizeFix(battle.WeaponDagger, battle.SizeLarge) {
		t.Error("size tables")
	}
}

// BenchmarkCalcEquipped measures one stat calculation of a player who
// wears ten items, each with three bonuses.
func BenchmarkCalcEquipped(b *testing.B) {
	var d Data
	if err := reference.LoadFile("extracts/renewal/job_stats.json", &d); err != nil {
		b.Fatal(err)
	}
	tb, err := New(&d)
	if err != nil {
		b.Fatal(err)
	}
	items := map[int]*Equipped{}
	for _, eqi := range []int{item.EqiAccL, item.EqiAccR, item.EqiShoes, item.EqiGarment, item.EqiHeadLow, item.EqiHeadMid, item.EqiHeadTop, item.EqiArmor, item.EqiHandL, item.EqiHandR} {
		items[eqi] = armor(
			Bonus{Name: "str", Args: []int64{1}, Options: opts(nil)},
			Bonus{Name: "max_hp", Args: []int64{5}, Options: opts(map[string]any{"percent": true})},
			Bonus{Name: "aspd", Args: []int64{2}, Options: opts(map[string]any{"percent": true})},
		)
	}
	c := Character{Job: 0, Male: true, BaseLevel: 50, JobLevel: 10, Stats: builds["melee"], Equipment: wearing(items)}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tb.Calc(c); err != nil {
			b.Fatal(err)
		}
	}
}
