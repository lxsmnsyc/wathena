package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// Hand tests of the Wizard and High Wizard skills: the areas, hits over
// time and statuses of their ground units, Ice Wall's cells, Quagmire,
// Meteor Storm's timing and the timer skills. The zones are the golden
// tests' zones: full data and every roll at its lowest, so every chance
// succeeds and Meteor Storm's random cells are the corner of its area.

// wizardSkills are the skills the test High Wizard learned.
var wizardSkills = [][2]int{
	{skill.IDBasic, 9}, {skill.IDSight, 1}, {skill.IDFireBolt, 10}, {skill.IDFirePillar, 10}, {skill.IDSightRasher, 10},
	{skill.IDMeteor, 10}, {skill.IDJupitel, 10}, {skill.IDVermilion, 10}, {skill.IDWaterBall, 5}, {skill.IDIceWall, 10},
	{skill.IDFrostNova, 10}, {skill.IDStormGust, 10}, {skill.IDEarthSpike, 5}, {skill.IDHeavenDrive, 5},
	{skill.IDQuagmire, 5}, {skill.IDEstimation, 1}, {skill.IDSoulDrain, 10}, {skill.IDMagicCrasher, 1},
	{skill.IDMagicPower, 10}, {skill.IDNapalmVulcan, 5}, {skill.IDGanbantein, 1}, {skill.IDGravitation, 5},
	{skill.IDSightBlaster, 1},
}

// wizardZone is a golden zone of a mode with a High Wizard at (20, 20)
// and the given monsters from (20, 22) on, which neither move nor
// attack and have plenty of HP.
func wizardZone(t *testing.T, mode reference.Mode, mobs ...uint32) (*Zone, *Player, []*entity) {
	t.Helper()
	z := goldenZone(t, mode, mobs...)
	p := goldenPlayerIn(t, z, goldenPlayer{Job: 4010, Build: "magic", Sex: 1, JobLevel: 70, Skills: wizardSkills}, 0, 0)
	var ms []*entity
	for e := range z.entities.all() {
		if e.monster != nil {
			e.monster.spawn.CanMove = false
			// Each spawn has its own copy of the status.
			e.monster.spawn.Status.Mode &^= modeCanAttack
			e.status = e.monster.spawn.Status
			e.hp = 10_000_000
			ms = append(ms, e)
		}
	}
	z.stepAt(50)
	events(p)
	return z, p, ms
}

// hitTimes steps the zone in 50 ms steps and returns the times at which
// an entity lost HP.
func hitTimes(z *Zone, e *entity, from, to int64) []int64 {
	var out []int64
	hp := e.hp
	for now := from; now <= to; now += 50 {
		z.stepAt(now)
		if e.hp < hp {
			out = append(out, now)
		}
		hp = e.hp
	}
	return out
}

// TestFirePillarRisesAndHits checks SkillFirePillar and the
// UNT_FIREPILLAR_WAITING and UNT_FIREPILLAR_ACTIVE cases of
// skill_unit_onplace_timer: the pillar waits until an enemy comes within
// a cell, rises, hits every enemy in its splash once with the skill's
// hits, stops their walking (applyAdditionalEffects), and ends 1.5
// seconds later.
func TestFirePillarRisesAndHits(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, ms := wizardZone(t, mode, 1040, 1040)
			a, b := ms[0], ms[1]
			b.x, b.y = 30, 30
			d := z.Skills.Get(skill.IDFirePillar)
			z.castendPos(p.entity, 20, 24, d, 5, 0)
			if len(p.entity.groups) != 1 || p.entity.groups[0].unitID != untFirePillarWaiting {
				t.Fatalf("groups %v", p.entity.groups)
			}
			// The Golem two cells away does not set it off.
			if got := hitTimes(z, a, 100, 3000); len(got) != 0 {
				t.Fatalf("hit at %v while waiting", got)
			}
			a.y = 23
			b.x, b.y = 21, 25
			hp := a.hp
			if got := hitTimes(z, a, 3050, 3500); len(got) != 1 {
				t.Fatalf("hits at %v, want one", got)
			}
			if b.hp == 10_000_000 {
				t.Error("the second Golem in the splash was not hit")
			}
			if a.canMove <= 3050 {
				t.Error("no walk delay after the hit")
			}
			if a.hp >= hp {
				t.Error("no damage")
			}
			// The used pillar ends 1.5 seconds after it went off.
			hitTimes(z, a, 3550, 5500)
			if len(z.groups) != 0 {
				t.Errorf("%d groups left", len(z.groups))
			}
		})
	}
}

// TestMeteorStormTiming checks SkillMeteorStorm::castendPos2 and the
// meteor parts of skill_unit_timer_sub: one meteor per second of the
// skill's time, each falling 700 ms after its effect starts, and a stun
// on the target.
func TestMeteorStormTiming(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, ms := wizardZone(t, mode, 1040)
			m := ms[0]
			d := z.Skills.Get(skill.IDMeteor)
			z.stepAt(1000)
			// Every roll is 0: each meteor falls on (x - 3, y - 3).
			m.x, m.y = 22, 22
			z.castendPos(p.entity, 25, 25, d, 10, 0)
			if len(p.entity.groups) != 7 {
				t.Fatalf("%d meteors, want 7", len(p.entity.groups))
			}
			// Unstun the Golem before each meteor so the hits stay.
			var got []int64
			hp := m.hp
			for now := int64(1050); now <= 9000; now += 50 {
				z.stepAt(now)
				if m.hp < hp {
					got = append(got, now)
					if !m.sc.Has(sc.SCStun) {
						t.Errorf("no stun after the meteor at %d", now)
					}
					z.scEnd(m, sc.SCStun)
				}
				hp = m.hp
			}
			if len(got) != 7 {
				t.Fatalf("meteor hits at %v, want 7", got)
			}
			for i, at := range got {
				want := int64(1000 + 700 + 1000*i)
				if at < want || at >= want+150 {
					t.Errorf("meteor %d hit at %d, want %d", i+1, at, want)
				}
			}
			if len(z.groups) != 0 {
				t.Errorf("%d groups left", len(z.groups))
			}
		})
	}
}

// TestStormGustFreezeCounter checks pre-renewal's Storm Gust counter
// (WZ_STORMGUST in skill_unit_onplace_timer and
// SkillStormGust::applyAdditionalEffects): the third hit in a row
// freezes, and frozen units take no more hits (status_check_skilluse).
// In renewal the freeze is a chance on each hit.
func TestStormGustFreezeCounter(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, ms := wizardZone(t, mode, 1040)
			m := ms[0]
			m.monster.spawn.Status.Mode |= modeKnockbackImm
			d := z.Skills.Get(skill.IDStormGust)
			z.castendPos(p.entity, m.x, m.y, d, 10, 0)
			hits := 0
			hp := m.hp
			for now := int64(100); now <= 1500 && !m.sc.Has(sc.SCFreeze); now += 50 {
				z.stepAt(now)
				if m.hp < hp {
					hits++
				}
				hp = m.hp
			}
			want := 3
			if mode == reference.Renewal {
				want = 1
			}
			if hits != want || !m.sc.Has(sc.SCFreeze) {
				t.Fatalf("%d hits before the freeze (frozen %v), want %d", hits, m.sc.Has(sc.SCFreeze), want)
			}
			if mode == reference.PreRenewal && m.sgCounter != 0 {
				t.Errorf("counter %d after the freeze", m.sgCounter)
			}
			// A frozen Golem takes no hits.
			if got := hitTimes(z, m, 1550, 3000); len(got) != 0 {
				t.Errorf("hits on the frozen Golem at %v", got)
			}
		})
	}
}

// TestHeavenDriveArea checks Heaven's Drive's unit: a 5x5 layout of
// range 0 cells that hits each unit on it once.
func TestHeavenDriveArea(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, ms := wizardZone(t, mode, 1040, 1040, 1040)
			ms[0].x, ms[0].y = 18, 22
			ms[1].x, ms[1].y = 22, 24
			ms[2].x, ms[2].y = 23, 22
			d := z.Skills.Get(skill.IDHeavenDrive)
			z.castendPos(p.entity, 20, 22, d, 5, 0)
			ev := run(z, p, 100, 2000)
			for i, m := range ms {
				n := 0
				for _, h := range skillDamage(ev, skill.IDHeavenDrive) {
					if h.GetTarget() == p.netID(m) {
						n++
					}
				}
				want := 1
				if i == 2 {
					want = 0
				}
				if n != want {
					t.Errorf("Golem %d at (%d, %d): %d hits, want %d", i, m.x, m.y, n, want)
				}
			}
		})
	}
}

// TestVermilionWaves checks Lord of Vermilion's unit: NoOverlap keeps one
// hit per interval of 1250 ms over every cell, through its time (four
// waves in pre-renewal, one in renewal), and the hits blind.
func TestVermilionWaves(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, ms := wizardZone(t, mode, 1040)
			m := ms[0]
			d := z.Skills.Get(skill.IDVermilion)
			z.castendPos(p.entity, m.x, m.y, d, 10, 0)
			got := hitTimes(z, m, 100, 6000)
			want := 4
			if mode == reference.Renewal {
				want = 1
			}
			if len(got) != want {
				t.Errorf("hits at %v, want %d", got, want)
			}
			if !m.sc.Has(sc.SCBlind) {
				t.Error("no blind")
			}
		})
	}
}

// TestQuagmire checks UNT_QUAGMIRE in skill_unit_onplace and WZ_QUAGMIRE
// in skill_unit_onleft: a unit on the field gets SC_QUAGMIRE for the
// field's time, with AGI and DEX lower (10 per level on monsters, 5 on
// players) and a slower walk. A player loses it when it leaves the
// field; a monster keeps it.
func TestQuagmire(t *testing.T) {
	z, p, ms := wizardZone(t, reference.Renewal, 1040)
	m := ms[0]
	agi, speed := m.status.Agi, m.status.Speed
	d := z.Skills.Get(skill.IDQuagmire)
	z.castendPos(p.entity, m.x, m.y, d, 5, 0)
	e := m.sc.Get(sc.SCQuagmire)
	if e == nil {
		t.Fatal("no Quagmire on the Golem")
	}
	if e.Val2 != 50 || m.status.Agi != max(agi-50, 0) {
		t.Errorf("val2 %d, AGI %d -> %d", e.Val2, agi, m.status.Agi)
	}
	if m.status.Speed != speed*150/100 {
		t.Errorf("speed %d -> %d", speed, m.status.Speed)
	}
	// The monster keeps it when it leaves the field.
	m.x, m.y = 30, 30
	z.skillUnitMove(m)
	if !m.sc.Has(sc.SCQuagmire) {
		t.Error("the Golem lost Quagmire when it left")
	}

	// A monster's Quagmire on the player: it ends when the player leaves.
	z.unitSetting(m, d, 5, p.entity.x, p.entity.y)
	if pe := p.entity.sc.Get(sc.SCQuagmire); pe == nil || pe.Val2 != 25 {
		t.Fatalf("player Quagmire %+v", pe)
	}
	p.entity.x += 3
	z.skillUnitMove(p.entity)
	if p.entity.sc.Has(sc.SCQuagmire) {
		t.Error("the player kept Quagmire off the field")
	}
}

// TestIceWallBlocksWalking checks SkillIceWall and its cells
// (skill_unitsetting, skill_initunit and skill_delunit): five cells
// across the way from the caster that units cannot walk on but spells
// pass, with 200 + 200 x level HP that falls by 50 every second
// (skill_unit_timer_sub), after which the cells open again.
func TestIceWallBlocksWalking(t *testing.T) {
	z, p, _ := wizardZone(t, reference.Renewal)
	shared := z.grid
	d := z.Skills.Get(skill.IDIceWall)
	// North of the caster: a wall along x.
	z.castendPos(p.entity, 20, 25, d, 1, 0)
	if len(p.entity.groups) != 1 || len(p.entity.groups[0].units) != 5 {
		t.Fatalf("groups %v", p.entity.groups)
	}
	for x := 18; x <= 22; x++ {
		if !z.grid.NoPass(x, 25) {
			t.Errorf("cell (%d, 25) open", x)
		}
	}
	if shared.NoPass(20, 25) {
		t.Error("the shared grid changed")
	}
	if ok, _ := path.Shoot(z.grid, 20, 20, 20, 30); !ok {
		t.Error("the wall stops spells")
	}
	steps, ok := path.Search(z.grid, 20, 24, 20, 26, false)
	if !ok || len(steps) <= 2 {
		t.Errorf("walk through the wall: %v %v", steps, ok)
	}
	// A second wall cannot stand on the first.
	z.castendPos(p.entity, 20, 25, d, 1, 0)
	if len(p.entity.groups) != 1 {
		t.Errorf("%d groups after a second wall on the first", len(p.entity.groups))
	}
	// Level 1 has 400 HP: eight seconds.
	run(z, p, 100, 7000)
	if !z.grid.NoPass(20, 25) {
		t.Fatal("the wall melted early")
	}
	run(z, p, 7050, 9000)
	for x := 18; x <= 22; x++ {
		if z.grid.NoPass(x, 25) {
			t.Errorf("cell (%d, 25) still blocked", x)
		}
	}
	if len(z.groups) != 0 {
		t.Errorf("%d groups left", len(z.groups))
	}
}

// TestJupitelDelayAndKnockback checks SkillJupitelThunder: the hit comes
// TIMERSKILL_INTERVAL after the cast (skill_timerskill) and pushes the
// target by the skill's knockback.
func TestJupitelDelayAndKnockback(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, ms := wizardZone(t, mode, 1040)
			m := ms[0]
			d := z.Skills.Get(skill.IDJupitel)
			z.stepAt(1000)
			z.castendDamage(p.entity, m, d, 10, 0)
			got := hitTimes(z, m, 1050, 2000)
			if len(got) != 1 || got[0] != 1150 {
				t.Fatalf("hits at %v, want 1150", got)
			}
			if dist := chebyshev(p.entity, m); dist != 2+7 {
				t.Errorf("Golem %d cells away, want 9", dist)
			}
		})
	}
}

// TestWaterBall checks SkillWaterBall and its timer skill. A player needs
// water (ST_WATER), which no map has yet. A monster needs none: its balls
// come from the cells of the layout, one each 150 ms.
func TestWaterBall(t *testing.T) {
	z, p, ms := wizardZone(t, reference.Renewal, 1040)
	m := ms[0]
	cast(z, p, skill.IDWaterBall, 1, m)
	ev := run(z, p, 100, 300)
	if f := skillFailures(ev); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
		t.Fatalf("failures %v", f)
	}
	d := z.Skills.Get(skill.IDWaterBall)
	p.entity.hp = 10_000_000
	z.stepAt(1000)
	// Level 2: a 3x3 layout, nine balls.
	z.castendDamage(m, p.entity, d, 2, 0)
	if len(m.groups) != 1 || len(m.groups[0].units) != 9 {
		t.Fatalf("groups %v", m.groups)
	}
	got := hitTimes(z, p.entity, 1050, 3500)
	if len(got) != 9 {
		t.Errorf("%d balls at %v, want 9", len(got), got)
	}
	if len(z.groups) != 0 {
		t.Errorf("%d groups left", len(z.groups))
	}
}

// TestSightRasherNeedsSight checks SkillSightRasher and its SC_SIGHT
// requirement, which skill_check_condition_castend checks: without
// Sight it fails; with it, Sight ends and the enemies around are hit and
// pushed.
func TestSightRasherNeedsSight(t *testing.T) {
	z, p, ms := wizardZone(t, reference.Renewal, 1040)
	m := ms[0]
	cast(z, p, skill.IDSightRasher, 10, nil)
	ev := run(z, p, 100, 1500)
	if f := skillFailures(ev); len(f) != 1 {
		t.Fatalf("failures %v", f)
	}
	cast(z, p, skill.IDSight, 1, nil)
	run(z, p, 1550, 2500)
	if !p.entity.sc.Has(sc.SCSight) {
		t.Fatal("no Sight")
	}
	before := chebyshev(p.entity, m)
	cast(z, p, skill.IDSightRasher, 10, nil)
	ev = run(z, p, 2550, 5000)
	if len(skillDamage(ev, skill.IDSightRasher)) != 1 {
		t.Fatalf("Sight Rasher hits %v", ev)
	}
	if p.entity.sc.Has(sc.SCSight) {
		t.Error("Sight did not end")
	}
	if after := chebyshev(p.entity, m); after != before+5 {
		t.Errorf("Golem pushed from %d to %d cells", before, after)
	}
}

// TestSightBlaster checks SkillSightBlaster and SC_SIGHTBLASTER's
// status_change_timer_sub: the first enemy next to the caster is hit and
// the status ends.
func TestSightBlaster(t *testing.T) {
	z, p, ms := wizardZone(t, reference.Renewal, 1040)
	m := ms[0]
	m.x, m.y = 30, 30
	d := z.Skills.Get(skill.IDSightBlaster)
	z.castendNoDamage(p.entity, p.entity, d, 1, 0)
	if e := p.entity.sc.Get(sc.SCSightblaster); e == nil || e.Val3 != 1 {
		t.Fatalf("status %+v", e)
	}
	z.stepAt(100)
	m.x, m.y = 21, 20
	if got := hitTimes(z, m, 150, 400); len(got) != 1 {
		t.Fatalf("hits at %v", got)
	}
	if p.entity.sc.Has(sc.SCSightblaster) {
		t.Error("the status did not end")
	}
}

// TestFrostNova checks SkillFrostNova: enemies in the splash are hit and
// frozen, but not one on the caster's own cell.
func TestFrostNova(t *testing.T) {
	z, p, ms := wizardZone(t, reference.PreRenewal, 1040, 1040)
	ms[1].x, ms[1].y = p.entity.x, p.entity.y
	d := z.Skills.Get(skill.IDFrostNova)
	z.castendNoDamage(p.entity, p.entity, d, 10, 0)
	if !ms[0].sc.Has(sc.SCFreeze) || ms[0].hp == 10_000_000 {
		t.Error("the Golem in the splash was not hit and frozen")
	}
	if ms[1].hp != 10_000_000 {
		t.Error("the Golem on the caster's cell was hit")
	}
}

// TestNapalmVulcanCurses checks SkillNapalmVulcan: a splash whose hits
// may curse.
func TestNapalmVulcanCurses(t *testing.T) {
	z, p, ms := wizardZone(t, reference.Renewal, 1040, 1040)
	d := z.Skills.Get(skill.IDNapalmVulcan)
	z.castendDamage(p.entity, ms[0], d, 5, 0)
	for i, m := range ms {
		if !m.sc.Has(sc.SCCurse) || m.hp == 10_000_000 {
			t.Errorf("Golem %d not hit and cursed", i)
		}
	}
}

// TestSense checks SkillSense: it succeeds on a monster and costs SP.
func TestSense(t *testing.T) {
	z, p, ms := wizardZone(t, reference.Renewal, 1040)
	sp := p.sp
	cast(z, p, skill.IDEstimation, 1, ms[0])
	ev := run(z, p, 100, 1000)
	ok := false
	for _, e := range ev {
		if s := e.GetSkillEffect(); s != nil && s.GetSkillId() == skill.IDEstimation {
			ok = true
		}
	}
	if !ok || p.sp != sp-10 {
		t.Errorf("effect %v, SP %d -> %d", ok, sp, p.sp)
	}
}

// TestSoulDrain checks HW_SOULDRAIN in skill_counter_additional_effect:
// a targeted spell that kills gives SP by the target's level.
func TestSoulDrain(t *testing.T) {
	z, p, ms := wizardZone(t, reference.Renewal, 1002)
	m := ms[0]
	m.hp = 1
	p.sp = 0
	d := z.Skills.Get(skill.IDFireBolt)
	z.castendDamage(p.entity, m, d, 1, 0)
	if !m.dead {
		t.Fatal("the Poring lived")
	}
	want := int64(m.status.Level * (95 + 15*10) / 100)
	if p.sp != want {
		t.Errorf("SP %d, want %d", p.sp, want)
	}
}

// TestMagicPowerToggle checks SC_MAGICPOWER: in renewal its MATK applies
// for its time; in pre-renewal skill_toggle_magicpower turns it on when
// the next spell starts and ends it at the one after.
func TestMagicPowerToggle(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, ms := wizardZone(t, mode, 1040)
			base := p.entity.status.MAtkMin
			cast(z, p, skill.IDMagicPower, 10, nil)
			run(z, p, 100, 1500)
			e := p.entity.sc.Get(sc.SCMagicpower)
			if e == nil {
				t.Fatal("no SC_MAGICPOWER")
			}
			boosted := base + base*50/100
			if mode == reference.Renewal {
				if p.entity.status.MAtkMin <= base {
					t.Errorf("MATK %d -> %d", base, p.entity.status.MAtkMin)
				}
				return
			}
			if p.entity.status.MAtkMin != base {
				t.Errorf("MATK %d before a spell, want %d", p.entity.status.MAtkMin, base)
			}
			cast(z, p, skill.IDFireBolt, 1, ms[0])
			if e.Val4 != 1 || p.entity.status.MAtkMin != boosted {
				t.Errorf("val4 %d, MATK %d at the spell's start, want %d", e.Val4, p.entity.status.MAtkMin, boosted)
			}
			run(z, p, 1550, 4000)
			cast(z, p, skill.IDFireBolt, 1, ms[0])
			if p.entity.sc.Has(sc.SCMagicpower) || p.entity.status.MAtkMin != base {
				t.Error("SC_MAGICPOWER did not end at the next spell")
			}
		})
	}
}

// TestGanbanteinRemovesUnits checks SkillGanbantein and
// skill_cell_overlap's HW_GANBANTEIN case: the units in the 3x3 square
// around the cell end.
func TestGanbanteinRemovesUnits(t *testing.T) {
	z, p, _ := wizardZone(t, reference.Renewal)
	z.castendPos(p.entity, 25, 25, z.Skills.Get(skill.IDQuagmire), 5, 0)
	if len(z.groups) != 1 {
		t.Fatal("no Quagmire")
	}
	z.castendPos(p.entity, 24, 24, z.Skills.Get(skill.IDGanbantein), 1, 0)
	g := p.entity.groups[0]
	left := 0
	for _, u := range g.units {
		if u.x >= 23 && u.x <= 25 && u.y >= 23 && u.y <= 25 {
			t.Errorf("unit at (%d, %d) stayed", u.x, u.y)
		}
		left++
	}
	if left != 25-9 {
		t.Errorf("%d units left, want 16", left)
	}
}

// TestGravitationField checks SkillGravitationField. In renewal it hits
// the units in its range once, from the caster. In pre-renewal the
// caster cannot move, the enemies on the field get SC_GRAVITATION and a
// hit every second, and damage to the caster ends the field.
func TestGravitationField(t *testing.T) {
	t.Run("renewal", func(t *testing.T) {
		z, p, ms := wizardZone(t, reference.Renewal, 1040)
		m := ms[0]
		z.castendPos(p.entity, m.x, m.y, z.Skills.Get(skill.IDGravitation), 5, 0)
		if got := hitTimes(z, m, 100, 3000); len(got) != 1 {
			t.Errorf("hits at %v, want one", got)
		}
	})
	t.Run("pre-renewal", func(t *testing.T) {
		z, p, ms := wizardZone(t, reference.PreRenewal, 1040)
		m := ms[0]
		aspd := m.status.AMotion
		z.castendPos(p.entity, m.x, m.y, z.Skills.Get(skill.IDGravitation), 5, 0)
		self := p.entity.sc.Get(scGravitation)
		if self == nil || self.Val3 != bctSelf || !p.entity.cant().Move {
			t.Fatalf("caster status %+v", self)
		}
		if e := m.sc.Get(scGravitation); e == nil || e.Val3 != bctEnemy || m.status.AMotion <= aspd {
			t.Errorf("Golem status %+v, amotion %d -> %d", e, aspd, m.status.AMotion)
		}
		if got := hitTimes(z, m, 100, 2950); len(got) != 3 {
			t.Errorf("hits at %v, want 3", got)
		}
		// The caster's other skills deal no damage.
		hp := m.hp
		z.castendDamage(p.entity, m, z.Skills.Get(skill.IDFireBolt), 1, 0)
		if m.hp != hp {
			t.Error("Fire Bolt hit under the caster's field")
		}
		z.damaged(p.entity, 1, true)
		if len(z.groups) != 0 || p.entity.sc.Has(scGravitation) {
			t.Error("damage did not end the field")
		}
	})
}
