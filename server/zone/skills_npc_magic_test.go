package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Hand tests of the monster magic, breath and area skills: what the
// recording of mob_skill_use.json does not show, such as areas, ground
// units over time, timer skills and statuses. The zones are the parity
// tests' zones: full data, an open square around parityCenter, and every
// roll at its lowest, so every chance succeeds.

// mobZombie is the undead monster that the tests cast with when a
// Poring's (mobPoring) MATK is too low to deal damage.
const mobZombie = 1015

// npcMagicZone is a parity zone of a mode with a monster of an ID at the
// center, which neither moves nor attacks.
func npcMagicZone(t *testing.T, mode reference.Mode, mob uint32) (*Zone, *entity) {
	t.Helper()
	z, defs := parityZone(t, mode)
	m := parityMob(t, z, defs, mob, parityCenter[0], parityCenter[1], 0, nil)
	z.now = 2000
	return z, m
}

// npcKnight adds a level 99 Knight at an offset from the center.
func npcKnight(t *testing.T, z *Zone, dx, dy int) *entity {
	t.Helper()
	return parityPlayer(t, z, 7, "melee", 99, 50, parityCenter[0]+dx, parityCenter[1]+dy).entity
}

// hpLost is the HP a unit lost from its maximum.
func hpLost(e *entity) int64 { return int64(e.status.MaxHP) - e.hp }

// stepTo steps the zone in 50 ms steps up to a time.
func stepTo(z *Zone, to int64) {
	for now := z.now + 50; now <= to; now += 50 {
		z.stepAt(now)
	}
}

// npcMagicSkill is a skill of the zone's database, which the test needs.
func npcMagicSkill(t *testing.T, z *Zone, id int) *skill.Def {
	t.Helper()
	d := z.Skills.Get(id)
	if d == nil {
		t.Fatalf("skill %d is not in the database", id)
	}
	return d
}

// TestNPCBreathHitsLine checks SkillFireBreath::castendDamageId with
// skill_eightpath_algorithm (map_foreachindir and skill_attack_area):
// the units in the line toward the target are hit, a unit behind the
// caster is not, and SkillIceBreath2::applyAdditionalEffects freezes at
// 100%.
func TestNPCBreathHitsLine(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, m := npcMagicZone(t, mode, mobPoring)
			a, b, c := npcKnight(t, z, 1, 0), npcKnight(t, z, 5, 0), npcKnight(t, z, -3, 0)
			z.castendDamage(m, a, npcMagicSkill(t, z, skill.IDNPCFireBreath), 1, 0)
			if hpLost(a) == 0 || hpLost(b) == 0 {
				t.Errorf("line hits: target lost %d, the unit behind it %d", hpLost(a), hpLost(b))
			}
			if hpLost(c) != 0 {
				t.Errorf("the unit behind the caster lost %d", hpLost(c))
			}
			if d := z.Skills.Get(skill.IDNPCIceBreath2); d != nil {
				z.castendDamage(m, a, d, 1, 0)
				if !a.sc.Has(sc.SCFreeze) || !b.sc.Has(sc.SCFreeze) {
					t.Error("NPC_ICEBREATH2 did not freeze the line")
				}
			}
		})
	}
}

// TestNPCDarkBreathMisses checks SkillDarkBreath::castendDamageId: the
// caster shows ET_ANGER, and on a roll of 0 (rnd() % 2 == 0) the breath
// deals nothing.
func TestNPCDarkBreathMisses(t *testing.T) {
	z, m := npcMagicZone(t, reference.Renewal, mobPoring)
	a := npcKnight(t, z, 1, 0)
	z.castendDamage(m, a, npcMagicSkill(t, z, skill.IDNPCDarkBreath), 1, 0)
	if hpLost(a) != 0 {
		t.Errorf("lost %d, want 0 on the lowest roll", hpLost(a))
	}
	if len(z.emotions) != 1 || z.emotions[0].emotion != etAnger {
		t.Errorf("emotions %v, want ET_ANGER", z.emotions)
	}
}

// TestNPCHellJudgementSplash checks SkillHellsJudgement: castendNoDamageId
// starts the splash of SkillImplRecursiveDamageSplash on the caster, so
// every enemy in its splash is hit and cursed (applyAdditionalEffects),
// and the caster is not.
func TestNPCHellJudgementSplash(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, m := npcMagicZone(t, mode, mobPoring)
			a, b := npcKnight(t, z, 1, 0), npcKnight(t, z, -6, 4)
			hp := m.hp
			z.castendNoDamage(m, m, npcMagicSkill(t, z, skill.IDNPCHellJudgement), 1, 0)
			for _, e := range []*entity{a, b} {
				if hpLost(e) == 0 || !e.sc.Has(sc.SCCurse) {
					t.Errorf("unit at (%d, %d): lost %d, cursed %v", e.x, e.y, hpLost(e), e.sc.Has(sc.SCCurse))
				}
			}
			if m.hp != hp {
				t.Errorf("the caster lost %d", hp-m.hp)
			}
		})
	}
}

// TestNPCMagicalAttackStatus checks SkillDemonShockAttack and
// SC_MAGICALATTACK's part of battle_weapon_attack: after the skill, the
// caster's normal attack is NPC_MAGICALATTACK.
func TestNPCMagicalAttackStatus(t *testing.T) {
	z, m := npcMagicZone(t, reference.Renewal, mobPoring)
	a := npcKnight(t, z, 1, 0)
	d := npcMagicSkill(t, z, skill.IDNPCMagicalAttack)
	z.castendDamage(m, a, d, 1, 0)
	if !m.sc.Has(scMagicalAttack) {
		t.Fatal("no SC_MAGICALATTACK on the caster")
	}
	z.hits = z.hits[:0]
	z.strike(m, a)
	if len(z.hits) != 1 || z.hits[0].skill != d {
		t.Fatalf("hits %v, want one of NPC_MAGICALATTACK", z.hits)
	}
}

// TestNPCGrandDarknessCross checks SkillGrandCrossOfDarkness: the cross
// of Grand Cross (skill_init_unit_layout), 3 hits per cell
// (Duration1 / interval, gx_allhit off), blindness at 100%, and no
// damage to a monster caster on its own cells.
func TestNPCGrandDarknessCross(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, m := npcMagicZone(t, mode, mobPoring)
			on, off := npcKnight(t, z, 2, 0), npcKnight(t, z, 2, 2)
			hp := m.hp
			z.castendNoDamage(m, m, npcMagicSkill(t, z, skill.IDNPCGrandDarkness), 5, 0)
			if len(m.groups) != 1 || len(m.groups[0].units) != len(grandCrossLayout) {
				t.Fatalf("groups %v", m.groups)
			}
			times := hitTimes(z, on, 2050, 3500)
			if len(times) != 3 {
				t.Errorf("hits at %v, want 3", times)
			}
			if !on.sc.Has(sc.SCBlind) {
				t.Error("not blinded")
			}
			if hpLost(off) != 0 {
				t.Errorf("a unit off the cross lost %d", hpLost(off))
			}
			if m.hp != hp {
				t.Errorf("the caster lost %d", hp-m.hp)
			}
		})
	}
}

// TestNPCEvilLandHurtsAndHeals checks SkillEvilLand and UNT_EVILLAND in
// skill_unit_onplace_timer: a player takes skill_calc_heal's 100 per
// level, not halved, as a misc hit, and an undead monster is healed by
// the same amount.
func TestNPCEvilLandHurtsAndHeals(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, m := npcMagicZone(t, mode, mobZombie)
			a := npcKnight(t, z, 1, 0)
			m.hp -= 500
			hp := m.hp
			z.castendPos(m, m.x+1, m.y, npcMagicSkill(t, z, skill.IDNPCEvilLand), 1, 0)
			stepTo(z, 2150)
			if hpLost(a) != 100 {
				t.Errorf("player lost %d, want 100", hpLost(a))
			}
			if m.hp != hp+100 {
				t.Errorf("undead caster healed %d, want 100", m.hp-hp)
			}
		})
	}
}

// TestNPCEarthquakeSplits checks SkillEarthquake and UNT_EARTHQUAKE in
// skill_unit_onplace_timer: each hit splits its damage among the
// enemies in the splash (skill_area_sub_count and NK_SPLASHSPLIT in
// battle_calc_magic_attack), and the unit hits 3 times in its 910 ms.
func TestNPCEarthquakeSplits(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			first := func(n int) (int64, []int64) {
				z, m := npcMagicZone(t, mode, mobPoring)
				var es []*entity
				for i := range n {
					es = append(es, npcKnight(t, z, 1+i, 0))
				}
				z.castendNoDamage(m, m, npcMagicSkill(t, z, skill.IDNPCEarthquake), 1, 0)
				hp := es[0].hp
				z.stepAt(2050)
				dmg := hp - es[0].hp
				return dmg, hitTimes(z, es[0], 2100, 3200)
			}
			one, times := first(1)
			two, _ := first(2)
			if one <= 0 || two <= 0 || two < one/2-1 || two > one/2+1 {
				t.Errorf("damage %d alone, %d with two targets", one, two)
			}
			if len(times) != 2 {
				t.Errorf("later hits at %v, want 2", times)
			}
		})
	}
}

// TestNPCCometDistance checks SkillComet2: skill_unitsetting keeps the
// comet's cell on the caster (sc->comet_x and comet_y), and
// calculateSkillRatio lowers the damage with the distance from it.
func TestNPCCometDistance(t *testing.T) {
	z, m := npcMagicZone(t, reference.Renewal, mobZombie)
	near, far := npcKnight(t, z, 3, 0), npcKnight(t, z, 3, 5)
	z.castendPos(m, m.x+3, m.y, npcMagicSkill(t, z, skill.IDNPCComet), 1, 0)
	if m.comet == nil || *m.comet != [2]int{m.x + 3, m.y} {
		t.Fatalf("comet %v", m.comet)
	}
	z.stepAt(2050)
	if hpLost(near) <= hpLost(far) || hpLost(far) <= 0 {
		t.Errorf("lost %d at the center and %d 5 cells away", hpLost(near), hpLost(far))
	}
	if !near.sc.Has(scBurning) {
		t.Error("no SC_BURNING")
	}
}

// TestNPCRainOfMeteorCount checks SkillRainOfMeteor::castendPos2: two
// meteors in the first interval and one in each after, and the meteor
// parts of skill_unit_timer_sub: each falls one interval after it was
// placed.
func TestNPCRainOfMeteorCount(t *testing.T) {
	z, m := npcMagicZone(t, reference.Renewal, mobPoring)
	a := npcKnight(t, z, 1, 0)
	z.castendPos(m, a.x, a.y, npcMagicSkill(t, z, skill.IDNPCRainOfMeteor), 3, 0)
	if len(m.groups) != 4 {
		t.Fatalf("%d meteors, want 4", len(m.groups))
	}
	if got := hitTimes(z, a, 2050, 2800); len(got) != 0 {
		t.Errorf("hits at %v before the first meteor fell", got)
	}
	if got := hitTimes(z, a, 2850, 3200); len(got) == 0 {
		t.Error("the first meteors did not hit")
	}
}

// TestNPCCloudKillOnePerCaster checks skill_unitsetting's
// skill_clear_group with flag 4: a new NPC_CLOUD_KILL ends the
// caster's last one. UNT_CLOUD_KILL then hits from the caster.
func TestNPCCloudKillOnePerCaster(t *testing.T) {
	z, m := npcMagicZone(t, reference.Renewal, mobZombie)
	a := npcKnight(t, z, 1, 0)
	d := npcMagicSkill(t, z, skill.IDNPCCloudKill)
	z.castendPos(m, a.x+5, a.y, d, 1, 0)
	z.castendPos(m, a.x, a.y, d, 1, 0)
	if len(m.groups) != 1 || m.groups[0].x != a.x {
		t.Fatalf("groups %v, want the last cloud only", m.groups)
	}
	if got := hitTimes(z, a, 2050, 2200); len(got) != 1 {
		t.Errorf("hits at %v, want one", got)
	}
}

// TestNPCStormGust2SkipsFrozen checks status_check_skilluse: like Storm
// Gust, NPC_STORMGUST2 does not hit a frozen unit.
func TestNPCStormGust2SkipsFrozen(t *testing.T) {
	z, m := npcMagicZone(t, reference.Renewal, mobPoring)
	a, b := npcKnight(t, z, 1, 0), npcKnight(t, z, 2, 0)
	z.scStart(m, a, sc.SCFreeze, 100, 1, 60000)
	if !a.sc.Has(sc.SCFreeze) {
		t.Fatal("no freeze")
	}
	z.castendPos(m, a.x, a.y, npcMagicSkill(t, z, skill.IDNPCStormGust2), 1, 0)
	stepTo(z, 2200)
	if hpLost(a) != 0 || hpLost(b) == 0 {
		t.Errorf("frozen lost %d, other lost %d", hpLost(a), hpLost(b))
	}
}

// TestNPCMagmaEruption checks SkillNpcMagmaEruption: the slam hits and
// stuns at once (applyAdditionalEffects), the eruption unit comes two
// attack motions later (skill_timerskill), and UNT_MAGMA_ERUPTION deals
// NC_MAGMA_ERUPTION_DOTDAMAGE's 800 + 200 per level and
// NPC_MAGMA_ERUPTION_DOTDAMAGE's 1000 per level.
func TestNPCMagmaEruption(t *testing.T) {
	z, m := npcMagicZone(t, reference.Renewal, mobPoring)
	a := npcKnight(t, z, 1, 0)
	z.castendPos(m, a.x, a.y, npcMagicSkill(t, z, skill.IDNPCMagmaEruption), 3, 0)
	slam := hpLost(a)
	if slam == 0 || !a.sc.Has(sc.SCStun) {
		t.Fatalf("slam: lost %d, stunned %v", slam, a.sc.Has(sc.SCStun))
	}
	if len(m.groups) != 0 || len(z.timerSkills) != 1 || z.timerSkills[0].at != 2000+int64(m.status.AMotion)*2 {
		t.Fatalf("groups %v, timer skills %v", m.groups, z.timerSkills)
	}
	stepTo(z, z.timerSkills[0].at+100)
	if len(m.groups) != 1 {
		t.Fatalf("%d eruption units", len(m.groups))
	}
	if got := hpLost(a) - slam; got != 1400+3000 {
		t.Errorf("eruption dealt %d, want %d", got, 1400+3000)
	}
}

// TestNPCPropertyWalk checks SkillNpcFireWalk and SC_PROPERTYWALK's part
// of map_moveblock: each cell the caster walks from gets a unit, up to
// the skill's count (skill_get_maxcount), and the next move ends the
// status. A unit hits a player on its cell.
func TestNPCPropertyWalk(t *testing.T) {
	z, m := npcMagicZone(t, reference.Renewal, mobPoring)
	d := npcMagicSkill(t, z, skill.IDNPCFireWalk)
	z.castendNoDamage(m, m, d, 1, 0)
	if !m.sc.Has(scPropertyWalk) {
		t.Fatal("no SC_PROPERTYWALK")
	}
	start := [2]int{m.x, m.y}
	n := d.ActiveInstance.At(1)
	for i := range n {
		x0, y0 := m.x, m.y
		m.x++
		z.propertyWalkMoved(m, x0, y0)
		if len(m.groups) != i+1 {
			t.Fatalf("after %d cells, %d units", i+1, len(m.groups))
		}
	}
	x0, y0 := m.x, m.y
	m.x++
	z.propertyWalkMoved(m, x0, y0)
	if m.sc.Has(scPropertyWalk) || len(m.groups) != n {
		t.Errorf("status %v, %d units after the count", m.sc.Has(scPropertyWalk), len(m.groups))
	}
	a := npcKnight(t, z, start[0]-parityCenter[0], start[1]-parityCenter[1])
	if got := hitTimes(z, a, 2050, 2200); len(got) != 1 {
		t.Errorf("hits at %v on a fire cell, want one", got)
	}
}

// TestNPCReverberationGoesOff checks SkillReverberation2 and
// UNT_REVERBERATION: an enemy on the trap sets it off, each enemy in its
// splash takes NPC_REVERBERATION_ATK 50 ms later (skill_trap_splash and
// skill_timerskill), and the used trap ends a second later.
func TestNPCReverberationGoesOff(t *testing.T) {
	z, m := npcMagicZone(t, reference.Renewal, mobPoring)
	a, b := npcKnight(t, z, 2, 0), npcKnight(t, z, 3, 1)
	z.castendPos(m, a.x, a.y, npcMagicSkill(t, z, skill.IDNPCReverberation), 1, 0)
	stepTo(z, 2050)
	if len(m.groups) != 1 || m.groups[0].unitID != untUsedTraps {
		t.Fatalf("groups %v, want a used trap", m.groups)
	}
	if hpLost(a) != 0 {
		t.Error("hit before the timer skill")
	}
	stepTo(z, 2150)
	if hpLost(a) == 0 || hpLost(b) == 0 {
		t.Errorf("lost %d and %d", hpLost(a), hpLost(b))
	}
	stepTo(z, 3200)
	if len(m.groups) != 0 {
		t.Errorf("%d groups left", len(m.groups))
	}
}

// TestNPCMandragora checks SkillNpcHowlingOfMandragora: a player in the
// splash gets SC_MANDRAGORA and loses 25% + 5% per level of its max SP.
func TestNPCMandragora(t *testing.T) {
	z, m := npcMagicZone(t, reference.Renewal, mobPoring)
	a := npcKnight(t, z, 2, 0)
	d := npcMagicSkill(t, z, skill.IDNPCMandragora)
	sp := a.player.sp
	z.castendNoDamage(m, m, d, 1, 0)
	if !a.sc.Has(d.Status) {
		t.Fatal("no SC_MANDRAGORA")
	}
	if want := sp - int64(a.status.MaxSP)*30/100; a.player.sp != want {
		t.Errorf("SP %d, want %d", a.player.sp, want)
	}
}

// TestNPCPoisonBuster checks SkillNpcPoisonBuster::castendDamageId: only
// a poisoned target is hit, and the hit ends its poison.
func TestNPCPoisonBuster(t *testing.T) {
	z, m := npcMagicZone(t, reference.Renewal, mobPoring)
	a := npcKnight(t, z, 1, 0)
	d := npcMagicSkill(t, z, skill.IDNPCPoisonBuster)
	z.castendDamage(m, a, d, 1, 0)
	if hpLost(a) != 0 {
		t.Fatalf("an unpoisoned target lost %d", hpLost(a))
	}
	z.scStart(m, a, sc.SCPoison, 100, 1, 60000)
	z.castendDamage(m, a, d, 1, 0)
	if hpLost(a) == 0 || a.sc.Has(sc.SCPoison) {
		t.Errorf("lost %d, poisoned %v", hpLost(a), a.sc.Has(sc.SCPoison))
	}
}

// TestNPCFatalMenaceWarps checks SkillNpcFatalMenace::castendDamageId:
// the enemies around the target are hit, and a timer skill warps the
// caster 800 ms later.
func TestNPCFatalMenaceWarps(t *testing.T) {
	z, m := npcMagicZone(t, reference.Renewal, mobPoring)
	a, b := npcKnight(t, z, 1, 0), npcKnight(t, z, 1, 1)
	z.castendDamage(m, a, npcMagicSkill(t, z, skill.IDNPCFatalMenace), 1, 0)
	if hpLost(a) == 0 || hpLost(b) == 0 {
		t.Errorf("lost %d and %d", hpLost(a), hpLost(b))
	}
	if len(z.timerSkills) != 1 || z.timerSkills[0].at != 2800 || z.timerSkills[0].src != m {
		t.Errorf("timer skills %v, want the warp at 2800", z.timerSkills)
	}
}

// TestNPCJackFrostFreezing checks SkillJackFrost2: every enemy around
// the target is hit, and calculateSkillRatio deals more to a target
// with SC_FREEZING.
func TestNPCJackFrostFreezing(t *testing.T) {
	hit := func(freezing bool) int64 {
		z, m := npcMagicZone(t, reference.Renewal, mobZombie)
		a := npcKnight(t, z, 1, 0)
		if freezing {
			z.scStart4(m, a, scFreezing, 100, 1, 1000, 0, 0, 60000)
			if !a.sc.Has(scFreezing) {
				t.Fatal("no SC_FREEZING")
			}
		}
		z.castendNoDamage(m, m, npcMagicSkill(t, z, skill.IDNPCJackFrost), 1, 0)
		return hpLost(a)
	}
	plain, frozen := hit(false), hit(true)
	if plain <= 0 || frozen <= plain {
		t.Errorf("lost %d, and %d with SC_FREEZING", plain, frozen)
	}
}
