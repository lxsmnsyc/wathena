package skill

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

func init() {
	// Renewal's recording uses the Taurus Sword (1100), whose script adds
	// bBaseAtk 50 + BaseLevel. The fixture of TestSkillDamage runs no
	// item scripts. TestSpiralPierceGolden checks every case with the
	// bonus.
	damageGaps["LK_SPIRALPIERCE"] = "the renewal weapon's bBaseAtk comes from its script; TestSpiralPierceGolden checks it"
}

// taurusSword is Taurus_Sword_J, the first one-handed sword in renewal.
const taurusSword = 1100

// TestSpiralPierceGolden checks every LK_SPIRALPIERCE case of
// skill_damage.json in both modes, with the equipment ATK that the
// Taurus Sword's script gives the renewal attacker (bonus bBaseAtk,
// 50 + BaseLevel). It covers the pre-renewal weight formula and the
// renewal one of battle_calc_skill_base_damage.
func TestSpiralPierceGolden(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			f := loadDamageFixture(t, mode)
			n := 0
			for i := range f.cases {
				c := &f.cases[i]
				if c.Skill != IDSpiralPierce {
					continue
				}
				n++
				def := f.db.Get(c.Skill)
				src, err := f.attacker(t, c, def)
				if err != nil {
					t.Fatal(err)
				}
				if c.Weapon == taurusSword {
					src.EAtk += 50 + 99
				}
				target := f.mobs[c.Target]
				sk := def.Battle(src, c.Level, c.Flag, 1)
				got := attack(mode, def.Type, src, target, sk, forcedMin{})
				want := c.ForcedMin
				if got.Damage != want.Damage+want.Damage2 || got.Hits != want.Div || got.Flag != want.Flag {
					t.Errorf("%s: got %+v, want %+v", c.name(), got, want)
					continue
				}
				if msg := checkRolls(mode, src, target, sk, def.Type, f.rolls, c.Rolls); msg != "" {
					t.Errorf("%s: %s", c.name(), msg)
				}
			}
			if n == 0 {
				t.Fatal("no Spiral Pierce cases")
			}
		})
	}
}

// knightAttacker is a level 99 Knight of the melee build with every
// tree skill and a weapon, for the hand tests.
func knightAttacker(t *testing.T, mode reference.Mode, job int, weapon uint32) (*damageFixture, *battle.Status) {
	t.Helper()
	f := loadDamageFixture(t, mode)
	sex := 1
	c := &skillCase{Job: job, Build: "melee", Weapon: weapon, Sex: &sex}
	src, err := f.attacker(t, c, f.db.Get(IDPierce))
	if err != nil {
		t.Fatal(err)
	}
	return f, src
}

// TestPierceHitsBySize checks SkillPierce::modifyDamageData: one hit per
// size step of the target.
func TestPierceHitsBySize(t *testing.T) {
	f, src := knightAttacker(t, reference.PreRenewal, 7, 1401)
	def := f.db.Get(IDPierce)
	for size, want := range []int{1, 2, 3} {
		target := *f.mobs[1040]
		target.Size = size
		got := attack(reference.PreRenewal, def.Type, src, &target, def.Battle(src, 10, 0, 1), forcedMin{})
		if got.Hits != want {
			t.Errorf("size %d: %d hits, want %d", size, got.Hits, want)
		}
	}
}

// TestBrandishSpearRows checks SkillBrandishSpear::calculateSkillRatio in
// pre-renewal: at level 10 the nearest rows (flag 0) deal 100% + 20% per
// level plus its half, quarter and eighth, and the farthest (flag 3)
// the base ratio.
func TestBrandishSpearRows(t *testing.T) {
	ratio := func(renewal bool, lv, flag, str int) int {
		r := 100
		brandishSpear{}.SkillRatio(&battle.SkillContext{Src: &battle.Status{Str: str},
			Skill: &battle.Skill{Level: lv}, Flag: flag, Renewal: renewal}, &r)
		return r
	}
	for _, c := range []struct{ lv, flag, want int }{
		{10, 0, 300 + 150 + 75 + 37}, {10, 1, 300 + 150 + 75}, {10, 2, 300 + 150}, {10, 3, 300},
		{7, 0, 240 + 120 + 60}, {7, 1, 240 + 120}, {4, 0, 180 + 90}, {3, 0, 160},
	} {
		if got := ratio(false, c.lv, c.flag, 0); got != c.want {
			t.Errorf("pre-renewal lv %d flag %d: %d%%, want %d%%", c.lv, c.flag, got, c.want)
		}
	}
	// Renewal: 400% + 100% per level + 3% per STR.
	if got := ratio(true, 10, 0, 100); got != 400+1000+300 {
		t.Errorf("renewal lv 10: %d%%", got)
	}
}

// TestChargeAttackDistance checks SkillChargeAttack::calculateSkillRatio
// in pre-renewal (100% more per 3 cells, at most 500%) and the cast time
// of unit_skilluse_id2, which grows with the distance.
func TestChargeAttackDistance(t *testing.T) {
	for _, c := range []struct{ dist, want int }{{1, 100}, {3, 100}, {4, 200}, {7, 300}, {13, 500}, {20, 500}} {
		r := 100
		chargeAttack{}.SkillRatio(&battle.SkillContext{Skill: &battle.Skill{Level: 1}, Flag: c.dist}, &r)
		if r != c.want {
			t.Errorf("distance %d: %d%%, want %d%%", c.dist, r, c.want)
		}
	}
	db := loadDB(t, reference.PreRenewal)
	d := db.Get(IDChargeAttack)
	st := &battle.Status{Dex: 0}
	base := d.CastTimeFor(&Caster{Status: st, Monster: true}, 1, false, true).Milliseconds
	for _, c := range []struct{ dist, k int }{{1, 1}, {4, 2}, {7, 3}, {14, 3}} {
		got := d.CastTimeFor(&Caster{Status: st, Monster: true, TargetDistance: c.dist}, 1, false, true).Milliseconds
		if got != base*c.k {
			t.Errorf("distance %d: cast %d ms, want %d", c.dist, got, base*c.k)
		}
	}
}

// TestJointBeatNeck checks SkillVitalStrike::calculateSkillRatio: a hit
// that breaks the neck, or a hit on a broken neck, deals double.
func TestJointBeatNeck(t *testing.T) {
	ratio := func(flag, broken int) int {
		r := 100
		target := &battle.Status{Changes: &battle.Changes{JointBeat: broken}}
		jointBeat{}.SkillRatio(&battle.SkillContext{Target: target, Skill: &battle.Skill{Level: 10}, Flag: flag}, &r)
		return r
	}
	if got := ratio(sc.BreakAnkle, 0); got != 150 {
		t.Errorf("ankle: %d%%, want 150%%", got)
	}
	if got := ratio(sc.BreakNeck, 0); got != 300 {
		t.Errorf("neck: %d%%, want 300%%", got)
	}
	if got := ratio(sc.BreakAnkle, sc.BreakNeck); got != 300 {
		t.Errorf("broken neck: %d%%, want 300%%", got)
	}
}

// TestKnightStatusDamage checks the battle effects of the Knight
// statuses in pre-renewal: Frenzy doubles a normal attack's ratio
// (battle_calc_attack_skill_ratio), Aura Blade adds 20 per level after
// DEF but not to Spiral Pierce (battle_calc_attack_post_defense), and a
// waist broken by Joint Beat takes 25% of the ATK percent
// (battle_get_atkpercent).
func TestKnightStatusDamage(t *testing.T) {
	f, src := knightAttacker(t, reference.PreRenewal, 4008, 1401)
	target := f.mobs[1040]
	plain := battle.AttackPreRenewal(src, target, forcedMin{}).Damage

	berserk := *src
	berserk.Changes = &battle.Changes{AtkPercent: 100, Berserk: true}
	if got := battle.AttackPreRenewal(&berserk, target, forcedMin{}).Damage; got <= plain {
		t.Errorf("Frenzy: %d, plain %d", got, plain)
	}

	aura := *src
	aura.Changes = &battle.Changes{AtkPercent: 100, AuraBlade: 5}
	if got := battle.AttackPreRenewal(&aura, target, forcedMin{}).Damage; got != plain+100 {
		t.Errorf("Aura Blade 5: %d, want %d", got, plain+100)
	}
	def := f.db.Get(IDSpiralPierce)
	sp := attack(reference.PreRenewal, def.Type, src, target, def.Battle(src, 5, 0, 1), forcedMin{}).Damage
	if got := attack(reference.PreRenewal, def.Type, &aura, target, def.Battle(&aura, 5, 0, 1), forcedMin{}).Damage; got != sp {
		t.Errorf("Aura Blade on Spiral Pierce: %d, want %d", got, sp)
	}
}

// TestAutoCounterCritical checks is_attack_critical with auto_counter_type
// and SkillCounterAttack::modifyDamageData: the counter is critical and
// counts as a normal attack.
func TestAutoCounterCritical(t *testing.T) {
	for _, mode := range reference.Modes {
		f, src := knightAttacker(t, mode, 7, 0)
		def := f.db.Get(IDAutoCounter)
		got := attack(mode, def.Type, src, f.mobs[1040], def.Battle(src, 5, 0, 1), forcedMin{})
		if got.Kind != battle.Critical || got.Flag&battle.FlagNormal == 0 || got.Flag&battle.FlagSkill != 0 {
			t.Errorf("%s: %+v", mode, got)
		}
	}
}

// TestRidingSpear checks the riding parts of battle_addmastery and
// status_calc_pc: Spear Mastery adds 5 per level instead of 4, and a
// spear deals to medium targets what it deals to large ones.
func TestRidingSpear(t *testing.T) {
	f, src := knightAttacker(t, reference.PreRenewal, 7, 1401)
	target := *f.mobs[1002]
	target.Size = battle.SizeMedium
	walk := battle.AttackPreRenewal(src, &target, forcedMin{}).Damage
	ride := *src
	pl := *src.Player
	pl.Riding = true
	ride.Player = &pl
	got := battle.AttackPreRenewal(&ride, &target, forcedMin{}).Damage
	if got == walk {
		t.Errorf("riding with a spear on a medium target: %d, same as walking", got)
	}
	large := target
	large.Size = battle.SizeLarge
	big := battle.AttackPreRenewal(&ride, &large, forcedMin{}).Damage
	if got != big {
		t.Errorf("riding: medium %d, large %d, want the same", got, big)
	}
}
