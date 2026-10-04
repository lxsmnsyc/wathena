package skill

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/battle"
)

// Hand tests of the Monk's formulas that the recordings, made with no
// spirit spheres, do not cover.

// TestFingerOffensiveCastTimePerSphere checks unit_skilluse_id2's
// pre-renewal rule: Finger Offensive takes its cast time once more for
// each sphere it throws, up to its level.
func TestFingerOffensiveCastTimePerSphere(t *testing.T) {
	d := loadDB(t, reference.PreRenewal).Get(IDFingerOffensive)
	c := &Caster{Status: &battle.Status{Dex: 0, Player: &battle.Player{Cast: battle.CastBonus{CastRate: 100}}}}
	base := d.CastTimeFor(c, 5, false, true).Milliseconds
	if base != 1000 {
		t.Fatalf("cast time %d with no DEX, want 1000", base)
	}
	c.SpiritBalls = 3
	if got := d.CastTimeFor(c, 5, false, true).Milliseconds; got != 4000 {
		t.Errorf("cast time %d with 3 spheres, want 4000", got)
	}
	c.SpiritBalls = 9
	if got := d.CastTimeFor(c, 2, false, true).Milliseconds; got != 3000 {
		t.Errorf("cast time %d at level 2 with 9 spheres, want 3000", got)
	}
	// Renewal has no such rule. MO_FINGEROFFENSIVE's skill_rebalance
	// flag picks it, which follows the caster's base here.
	r := loadDB(t, reference.Renewal).Get(IDFingerOffensive)
	c.Renewal = true
	c.SpiritBalls = 0
	want := r.CastTimeFor(c, 5, true, true).Milliseconds
	c.SpiritBalls = 3
	if got := r.CastTimeFor(c, 5, true, true).Milliseconds; got != want {
		t.Errorf("renewal cast time %d with spheres, want %d", got, want)
	}
}

// TestSpheresInDamage checks SkillThrowSpiritSphere::modifyDamageData
// (pre-renewal hits are the spheres thrown) and
// battle_get_spiritball_damage (3 damage per sphere).
func TestSpheresInDamage(t *testing.T) {
	testrace.SkipUnderRace(t, "it loads the recorded damage data and runs on one goroutine")
	f := loadDamageFixture(t, reference.PreRenewal)
	var fo *skillCase
	for i := range f.cases {
		c := &f.cases[i]
		if c.SkillName == "MO_FINGEROFFENSIVE" && c.Level == 5 && c.Target == 1002 {
			fo = c
		}
	}
	if fo == nil {
		t.Fatal("no Finger Offensive case")
	}
	def := f.db.Get(fo.Skill)
	src, err := f.attacker(t, fo, def)
	if err != nil {
		t.Fatal(err)
	}
	target := f.mobs[fo.Target]
	sk := def.Battle(src, 5, 0, 1)
	src.Player.SpiritBallOld = 3
	got := attack(reference.PreRenewal, def.Type, src, target, sk, forcedMin{})
	if got.Hits != 3 || got.Damage <= 0 {
		t.Fatalf("Finger Offensive with 3 spheres: %+v, want 3 hits", got)
	}
	// Spheres left over add 3 damage each, after the element table.
	src.Player.SpiritBall = 2
	more := attack(reference.PreRenewal, def.Type, src, target, sk, forcedMin{})
	if more.Damage <= got.Damage {
		t.Errorf("damage %d with 2 more spheres, was %d", more.Damage, got.Damage)
	}

	// Renewal: a normal attack's mastery ATK gains 3 per sphere.
	r := loadDamageFixture(t, reference.Renewal)
	var tri *skillCase
	for i := range r.cases {
		c := &r.cases[i]
		if c.SkillName == "MO_TRIPLEATTACK" && c.Target == 1002 {
			tri = c
			break
		}
	}
	rd := r.db.Get(tri.Skill)
	rsrc, err := r.attacker(t, tri, rd)
	if err != nil {
		t.Fatal(err)
	}
	none := battle.Attack(rsrc, r.mobs[1002], forcedMin{})
	rsrc.Player.SpiritBall = 5
	five := battle.Attack(rsrc, r.mobs[1002], forcedMin{})
	if five.Damage <= none.Damage {
		t.Errorf("normal attack %d with 5 spheres, %d without", five.Damage, none.Damage)
	}
}
