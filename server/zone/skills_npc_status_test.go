package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// npcStatusZone is a golden zone of a mode with a Poring at (20, 22)
// that does not fight back, and a Knight at (20, 20). Every roll is at
// its lowest, so every status chance succeeds.
func npcStatusZone(t *testing.T, mode reference.Mode) (*Zone, *entity, *Player) {
	t.Helper()
	z := goldenZone(t, mode, 1002)
	m := monsterOf(z, 1002)
	m.status.Mode &^= modeCanAttack
	p := goldenPlayerIn(t, z, knightPlayer, 0, 0)
	return z, m, p
}

// npcSkill is a skill of the zone's mode, or a skip when the mode has
// no such skill.
func npcSkill(t *testing.T, z *Zone, id int) *skill.Def {
	t.Helper()
	d := z.Skills.Get(id)
	if d == nil || d.Target == skill.TargetPassive {
		t.Skip("the skill is not in this mode's skill_db")
	}
	return d
}

// NPC_STOP stops the target and the monster, each SC_STOP naming the
// other, and the end of one ends the other (SkillStop, and SC_STOP in
// status_change_end).
func TestNPCStopHoldsBoth(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, m, p := npcStatusZone(t, mode)
			d := npcSkill(t, z, idNPCStop)
			z.castendNoDamage(m, p.entity, d, 1, 0)
			pe, me := p.entity.sc.Get(sc.SCStop), m.sc.Get(sc.SCStop)
			if pe == nil || me == nil {
				t.Fatalf("SC_STOP on player %v and monster %v, want both", pe != nil, me != nil)
			}
			if pe.Val2 != int(z.unit(m).Ref().ID) || me.Val2 != int(z.unit(p.entity).Ref().ID) {
				t.Errorf("val2 %d and %d, want each other's IDs", pe.Val2, me.Val2)
			}
			if got := pe.End - z.now; got != int64(d.Duration1.At(1)) {
				t.Errorf("player's SC_STOP lasts %d, want Duration1 %d", got, d.Duration1.At(1))
			}
			z.scEnd(p.entity, sc.SCStop)
			if m.sc.Has(sc.SCStop) {
				t.Error("the monster's SC_STOP outlived the player's")
			}
		})
	}
}

// The wide status skills hit every enemy in the splash around the
// monster with the skill's status, val2 the caster, and not its allies (SkillWideCurse, map_foreachinallrange with
// BCT_ENEMY).
func TestNPCWideCurseArea(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, m, p := npcStatusZone(t, mode)
			d := npcSkill(t, z, idNPCWideCurse)
			far := otherPlayer(t, z, knightPlayer, 20, 25)
			z.castendNoDamage(m, m, d, 1, 0)
			e := p.entity.sc.Get(sc.SCCurse)
			if e == nil {
				t.Fatal("the player 2 cells away is not cursed")
			}
			if e.Val2 != int(z.unit(m).Ref().ID) {
				t.Errorf("val2 %d, want the monster's ID", e.Val2)
			}
			if e.End <= z.now {
				t.Errorf("curse ends at %d, want after %d", e.End, z.now)
			}
			if far.entity.sc.Has(sc.SCCurse) || m.sc.Has(sc.SCCurse) {
				t.Error("the player 3 cells away or the monster is cursed")
			}
		})
	}
}

// NPC_WIDESTONE starts SC_STONEWAIT, which turns to SC_STONE after
// Duration1, with the caster's ID as val2 (SkillWidePetrify: sc_start2
// with a delay, and SC_STONEWAIT's end).
func TestNPCWideStoneWaits(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, m, p := npcStatusZone(t, mode)
			d := npcSkill(t, z, idNPCWideStone)
			z.castendNoDamage(m, m, d, 5, 0)
			if e := p.entity.sc.Get(sc.SCStonewait); e == nil || e.Val2 != int(z.unit(m).Ref().ID) {
				t.Fatalf("SC_STONEWAIT %+v, want val2 the monster's ID", e)
			}
			step(z, z.now+50, z.now+50+int64(d.Duration1.At(5)))
			if !p.entity.sc.Has(sc.SCStone) {
				t.Error("no SC_STONE after the wait")
			}
		})
	}
}

// NPC_DRAGONFEAR picks a status at random, and its duration is
// Duration2 at the pick's place in the list, not at the skill's level
// (SkillDragonFear). The lowest roll picks SC_STUN, the first.
func TestNPCDragonFearDuration(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, m, p := npcStatusZone(t, mode)
			d := npcSkill(t, z, idNPCDragonFear)
			z.castendNoDamage(m, m, d, 5, 0)
			e := p.entity.sc.Get(sc.SCStun)
			if e == nil {
				t.Fatal("no SC_STUN")
			}
			if left := e.End - z.now; left <= 0 || left > int64(d.Duration2.At(1)) {
				t.Errorf("stun lasts %d, want up to Duration2 at level 1, %d", left, d.Duration2.At(1))
			}
		})
	}
}

// NPC_DRAGONFEAR tries the next status when one fails: a stunned target
// gets SC_SILENCE (SkillDragonFear's loop, and status_change_start's
// refusal to stack SC_STUN).
func TestNPCDragonFearNextStatus(t *testing.T) {
	z, m, p := npcStatusZone(t, reference.Renewal)
	d := npcSkill(t, z, idNPCDragonFear)
	z.statusEngine().Start(z.unit(m), z.unit(p.entity), sc.Start{Type: sc.SCStun, Rate: 10000, Val1: 1,
		Duration: 60000, Flag: sc.NoAvoid, Reason: sc.ReasonSkill})
	f := 1
	npcDragonFear{}.castendNoDamage(z, m, p.entity, d, 1, &f)
	if !p.entity.sc.Has(sc.SCSilence) {
		t.Error("a stunned target got no SC_SILENCE")
	}
}

// NPC_LICK takes 100 SP and may stun (SkillLick: status_zap).
func TestNPCLickTakesSP(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, m, p := npcStatusZone(t, mode)
			d := npcSkill(t, z, idNPCLick)
			sp := p.sp
			z.castendNoDamage(m, p.entity, d, 1, 0)
			if p.sp != sp-100 {
				t.Errorf("SP %d, want %d", p.sp, sp-100)
			}
			if !p.entity.sc.Has(sc.SCStun) {
				t.Error("no SC_STUN")
			}
		})
	}
}

// NPC_WIDESOULDRAIN takes 20% of the current SP per level, from 1 to 5
// (SkillWideSoulDrain: status_percent_damage with a positive rate).
func TestNPCWideSoulDrain(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, m, p := npcStatusZone(t, mode)
			d := npcSkill(t, z, idNPCWideSoulDrain)
			sp := p.sp
			z.castendNoDamage(m, m, d, 2, 0)
			if want := sp - sp*40/100; p.sp != want {
				t.Errorf("SP %d, want %d", p.sp, want)
			}
		})
	}
}

// NPC_LEX_AETERNA casts Lex Aeterna level 1 on every enemy in the
// square around the cell (SkillLexAeterna2).
func TestNPCLexAeternaSquare(t *testing.T) {
	z, m, p := npcStatusZone(t, reference.Renewal)
	d := npcSkill(t, z, idNPCLexAeterna)
	corner := otherPlayer(t, z, knightPlayer, 22, 22)
	out := otherPlayer(t, z, knightPlayer, 23, 20)
	z.castendPos(m, 20, 20, d, 1, 0)
	if !p.entity.sc.Has(sc.SCAeterna) || !corner.entity.sc.Has(sc.SCAeterna) {
		t.Error("a player in the square has no SC_AETERNA")
	}
	if out.entity.sc.Has(sc.SCAeterna) {
		t.Error("a player out of the square has SC_AETERNA")
	}
}

// NPC_GRADUAL_GRAVITY's status takes 10% of max HP per level every
// second, and cannot be resisted (SkillIncreasedGravity, SC_GRADUAL_GRAVITY
// in status_change_start and status_change_timer).
func TestNPCGradualGravityTicks(t *testing.T) {
	z, m, p := npcStatusZone(t, reference.Renewal)
	d := npcSkill(t, z, idNPCGradualGravity)
	z.castendNoDamage(m, p.entity, d, 1, 0)
	e := p.entity.sc.Get(sc.SCGradualGravity)
	if e == nil || e.Val2 != 10 {
		t.Fatalf("SC_GRADUAL_GRAVITY %+v, want val2 10", e)
	}
	maxHP := int64(p.entity.status.MaxHP)
	hp := p.entity.hp
	step(z, z.now+50, z.now+1000)
	if want := hp - maxHP*10/100; p.entity.hp != want {
		t.Errorf("HP %d after a second, want %d", p.entity.hp, want)
	}
}

// NPC_ALL_STAT_DOWN lowers every stat by 20 per level, 10 less below
// level 5 (SC_ALL_STAT_DOWN in status_change_start and status_calc_str
// and the others).
func TestNPCAllStatDown(t *testing.T) {
	z, m, p := npcStatusZone(t, reference.Renewal)
	d := npcSkill(t, z, idNPCAllStatDown)
	st := *p.entity.status
	z.castendNoDamage(m, p.entity, d, 2, 0)
	got := p.entity.status
	// Stats stop at 0.
	if got.Str != st.Str-30 || got.Agi != st.Agi-30 || got.Vit != st.Vit-30 || got.Int != max(st.Int-30, 0) ||
		got.Dex != st.Dex-30 || got.Luk != max(st.Luk-30, 0) {
		t.Errorf("stats %d %d %d %d %d %d from %d %d %d %d %d %d, want 30 less", got.Str, got.Agi, got.Vit, got.Int,
			got.Dex, got.Luk, st.Str, st.Agi, st.Vit, st.Int, st.Dex, st.Luk)
	}
}

// NPC_HALLUCINATIONWALK gives 50 FLEE per level, and a chance of 10% per
// level to evade a magic hit (SC_NPC_HALLUCINATIONWALK in
// status_calc_flee and skill_attack).
func TestNPCHallucinationWalk(t *testing.T) {
	z, m, p := npcStatusZone(t, reference.Renewal)
	d := npcSkill(t, z, idNPCHallucinationWalk)
	holy := z.Skills.Get(skill.IDHolyLight)
	if z.skillAttack(battle.TypeMagic, p.entity, p.entity, m, holy, 1, 0) <= 0 {
		t.Fatal("Holy Light dealt no damage before the status")
	}
	m.hp = int64(m.status.MaxHP)
	flee := m.status.Flee
	z.castendNoDamage(m, m, d, 1, 0)
	if m.status.Flee != flee+50 {
		t.Errorf("FLEE %d, want %d", m.status.Flee, flee+50)
	}
	if got := z.skillAttack(battle.TypeMagic, p.entity, p.entity, m, holy, 1, 0); got != 0 {
		t.Errorf("Holy Light dealt %d under the status, want a miss", got)
	}
}

// Pre-renewal has no SC_NPC_HALLUCINATIONWALK: the skill does nothing
// (StatusSkillImpl with SC_NONE).
func TestNPCHallucinationWalkPreRenewal(t *testing.T) {
	z, m, _ := npcStatusZone(t, reference.PreRenewal)
	d := z.Skills.Get(idNPCHallucinationWalk)
	if d == nil || d.Status >= 0 {
		t.Skip("the skill has a status in pre-renewal")
	}
	z.castendNoDamage(m, m, d, 1, 0)
	if !m.sc.Empty() {
		t.Error("the monster has a status")
	}
}

// NPC_WIDEWEB halves FLEE (SC_WIDEWEB in status_calc_flee).
func TestNPCWideWebHalvesFlee(t *testing.T) {
	z, m, p := npcStatusZone(t, reference.Renewal)
	d := npcSkill(t, z, idNPCWideWeb)
	flee := p.entity.status.Flee
	z.castendNoDamage(m, m, d, 1, 0)
	if !p.entity.sc.Has(scWideWebTest) {
		t.Fatal("no SC_WIDEWEB")
	}
	if p.entity.status.Flee != flee-flee*50/100 {
		t.Errorf("FLEE %d, want half of %d", p.entity.status.Flee, flee)
	}
}

// scWideWebTest is SC_WIDEWEB.
const scWideWebTest = 770

// NPC_PETRIFYATTACK hits, then starts SC_STONEWAIT with the caster's ID
// as val2 (SkillPetrifyAttack: sc_start2 with a delay).
func TestNPCPetrifyAttack(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, m, p := npcStatusZone(t, mode)
			d := npcSkill(t, z, idNPCPetrifyAttack)
			hp := p.entity.hp
			z.castendDamage(m, p.entity, d, 1, 0)
			if p.entity.hp >= hp {
				t.Error("no damage")
			}
			e := p.entity.sc.Get(sc.SCStonewait)
			if e == nil || e.Val2 != int(z.unit(m).Ref().ID) {
				t.Errorf("SC_STONEWAIT %+v, want val2 the monster's ID", e)
			}
		})
	}
}

// NPC_WIDESUCK places its skill's ground unit (SkillWideSuck:
// skill_unitsetting). skill_db.yml gives it no unit, so nothing is
// placed and nothing is hit.
func TestNPCWideSuckPlacesNoUnit(t *testing.T) {
	z, m, p := npcStatusZone(t, reference.Renewal)
	d := npcSkill(t, z, idNPCWideSuck)
	hp := p.entity.hp
	z.castendPos(m, 20, 20, d, 1, 0)
	step(z, z.now+50, z.now+1000)
	if len(z.groups) != 0 || p.entity.hp != hp {
		t.Errorf("%d groups and HP %d, want none and %d", len(z.groups), p.entity.hp, hp)
	}
}
