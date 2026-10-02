package zone

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// highPriest is the recorder's High Priest: every skill of the tree.
var highPriest = goldenPlayer{Job: 4009, Build: "magic", Sex: 1, JobLevel: 50, Skills: [][2]int{
	{1, 9}, {9, 10}, {12, 10}, {22, 10}, {23, 10}, {24, 1}, {25, 1}, {26, 2}, {27, 4}, {28, 10}, {29, 10}, {30, 10},
	{31, 1}, {32, 10}, {33, 10}, {34, 10}, {35, 1}, {54, 4}, {65, 10}, {66, 5}, {67, 3}, {68, 5}, {69, 5}, {70, 10},
	{71, 4}, {72, 1}, {73, 10}, {74, 5}, {75, 5}, {76, 10}, {77, 10}, {78, 1}, {79, 10}, {361, 5}, {362, 5},
	{363, 10}, {481, 5}, {1014, 1},
}}

// priestZone is a golden zone of a mode with a High Priest at (20, 20)
// and the given monsters from (20, 22) east. The monsters neither move
// nor fight back.
func priestZone(t *testing.T, mode reference.Mode, mobs ...uint32) (*Zone, *Player, []*entity) {
	t.Helper()
	z := goldenZone(t, mode, mobs...)
	p := goldenPlayerIn(t, z, highPriest, 0, 0)
	var ms []*entity
	for _, id := range mobs {
		m := monsterOf(z, id)
		m.status.Mode &^= modeCanAttack
		ms = append(ms, m)
	}
	return z, p, ms
}

// step runs the zone from one time to another in steps of 50 ms.
func step(z *Zone, from, to int64) {
	for now := from; now <= to; now += 50 {
		z.stepAt(now)
	}
}

// Sanctuary heals those on its cells once a second, and hits the undead
// among the caster's enemies, ending after level + 3 hits
// (skill_unitsetting, UNT_SANCTUARY in skill_unit_onplace_timer and
// skill_unit_timer_sub).
func TestSanctuaryHealsAndHitsUndead(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, ms := priestZone(t, mode, 1015)
			zombie := ms[0]
			zombie.status.MaxHP, zombie.hp = 1000000, 1000000
			zombie.status.Mode |= modeKnockbackImm
			me := p.entity
			me.hp = 10
			d := z.Skills.Get(skill.IDSanctuary)
			z.castendPos(me, 20, 21, d, 2, 0)
			if len(z.groups) != 1 {
				t.Fatalf("%d groups, want 1", len(z.groups))
			}
			var g *unitGroup
			for _, x := range z.groups {
				g = x
			}
			if len(g.units) != 21 || g.val1 != 5 {
				t.Fatalf("%d cells and %d hits, want 21 and 5", len(g.units), g.val1)
			}
			heal := skill.CalcHeal(me.status, me.status, skill.IDSanctuary, 2, true, z.renewal(), z.rand)
			step(z, z.now+50, z.now+50)
			if me.hp != 10+heal {
				t.Errorf("HP %d after the first tick, want %d", me.hp, 10+heal)
			}
			// The next heal comes a second later, at the unit timer's
			// next pass of 100 ms.
			hp := me.hp
			step(z, z.now+50, z.now+900)
			if me.hp != hp {
				t.Errorf("HP %d before a second, want %d", me.hp, hp)
			}
			step(z, z.now+50, z.now+200)
			if me.hp != hp+heal {
				t.Errorf("HP %d after a second, want %d", me.hp, hp+heal)
			}
			// Five hits on the zombie, then the group ends before its
			// time.
			step(z, z.now+50, z.now+6000)
			if g.val1 != 0 || !g.deleted {
				t.Errorf("hits left %d, deleted %v", g.val1, g.deleted)
			}
			if zombie.hp >= 1000000 {
				t.Error("the zombie took no damage")
			}
		})
	}
}

// Magnus Exorcismus hits every enemy on its cells each 3 seconds in
// renewal, and only the undead and demons in pre-renewal (UNT_MAGNUS).
func TestMagnusWaves(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, ms := priestZone(t, mode, 1002, 1015)
			poring, zombie := ms[0], ms[1]
			for _, m := range ms {
				m.status.MaxHP, m.hp = 1000000, 1000000
			}
			d := z.Skills.Get(skill.IDMagnus)
			z.castendPos(p.entity, 20, 22, d, 1, 0)
			var zombieHits, poringHits int
			last := map[*entity]int64{poring: poring.hp, zombie: zombie.hp}
			end := z.now + int64(d.Duration1.At(1))
			for now := z.now + 50; now <= end; now += 50 {
				z.stepAt(now)
				if zombie.hp != last[zombie] {
					zombieHits++
				}
				if poring.hp != last[poring] {
					poringHits++
				}
				last[poring], last[zombie] = poring.hp, zombie.hp
			}
			if zombieHits != 2 {
				t.Errorf("zombie hit %d times, want 2", zombieHits)
			}
			want := 2
			if mode == reference.PreRenewal {
				want = 0
			}
			if poringHits != want {
				t.Errorf("poring hit %d times, want %d", poringHits, want)
			}
		})
	}
}

// Pre-renewal Basilica places a 5x5 area on the caster: the caster
// cannot move or be hit, enemies on it are pushed out, and casting it
// again cancels it (SkillBasilica, UNT_BASILICA, battle_status_block_damage
// and status_change_end).
func TestBasilicaPreRenewal(t *testing.T) {
	z, p, ms := priestZone(t, reference.PreRenewal, 1002)
	poring := ms[0]
	poring.x, poring.y = 30, 30
	me := p.entity
	d := z.Skills.Get(skill.IDBasilica)
	if r := (basilica{}).castBegin(z, me, d, 5); r != wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
		t.Fatalf("cast start refused: %v", r)
	}
	z.castendNoDamage(me, me, d, 5, 0)
	b := me.sc.Get(sc.SCBasilica)
	if b == nil || len(z.groups) != 1 {
		t.Fatalf("Basilica status %v, %d groups", b, len(z.groups))
	}
	if int64(b.Val4) != int64(p.charID) || !me.cant().Move {
		t.Errorf("caster val4 %d, cannot move %v", b.Val4, me.cant().Move)
	}
	// A hit from a monster that is not a boss is blocked.
	res := battle.AttackPreRenewal(poring.status, z.defender(me), z.rand)
	if res.Damage != 0 {
		t.Errorf("hit for %d through Basilica", res.Damage)
	}
	// An enemy stepping on the area is pushed back the way it faces.
	poring.x, poring.y, poring.dir = me.x+1, me.y, 2
	z.skillUnitMove(poring)
	if poring.x == me.x+1 && poring.y == me.y {
		t.Error("the poring was not pushed")
	}
	// A second cast cancels: the status and the area end.
	z.castendNoDamage(me, me, d, 5, 0)
	if me.sc.Has(sc.SCBasilica) || len(z.groups) != 0 {
		t.Errorf("after the cancel: status %v, %d groups", me.sc.Has(sc.SCBasilica), len(z.groups))
	}
	// The cast start needs an area free of other units.
	poring.x, poring.y = me.x+2, me.y
	if r := (basilica{}).castBegin(z, me, d, 5); r == wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
		t.Error("cast start allowed with a unit in the area")
	}
}

// Kyrie Eleison absorbs weapon hits until its HP or its hits run out
// (battle_status_block_damage).
func TestKyrieAbsorbsHits(t *testing.T) {
	z, p, ms := priestZone(t, reference.Renewal, 1023)
	orc := ms[0]
	me := p.entity
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDKyrie), 1, 0)
	k := me.sc.Get(sc.SCKyrie)
	if k == nil {
		t.Fatal("no Kyrie")
	}
	barrier, hits := k.Val2, k.Val3
	// The hits land: no flee and no lucky dodge.
	me.status.Flee, me.status.Flee2 = 0, 0
	res := battle.Attack(orc.status, z.defender(me), z.rand)
	if res.Damage != 0 {
		t.Errorf("first hit %+v", res)
	}
	if k.Val3 != hits-1 || k.Val2 >= barrier {
		t.Errorf("barrier %d/%d hits %d/%d after a hit", k.Val2, barrier, k.Val3, hits)
	}
	for range hits - 1 {
		battle.Attack(orc.status, z.defender(me), z.rand)
	}
	if me.sc.Has(sc.SCKyrie) {
		t.Error("Kyrie stayed after its last hit")
	}
	// A hit beyond the barrier deals the rest.
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDKyrie), 1, 0)
	k = me.sc.Get(sc.SCKyrie)
	full := battle.Attack(orc.status, z.defender(orc), z.rand).Damage
	k.Val2 = 1
	if got := z.kyrieBlock(me, battle.FlagWeapon, 0, full); got != full-1 || me.sc.Has(sc.SCKyrie) {
		t.Errorf("hit of %d over a barrier of 1 dealt %d, Kyrie on %v", full, got, me.sc.Has(sc.SCKyrie))
	}
}

// Lex Aeterna doubles the next hit and ends (battle_calc_damage).
func TestLexAeternaDoublesOnce(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, ms := priestZone(t, mode, 1023)
			orc := ms[0]
			d := z.Skills.Get(skill.IDHolyLight)
			base := z.calcSkillAttack(battle.TypeMagic, p.entity, p.entity, orc, d, 1, 0, nil).Damage
			z.castendNoDamage(p.entity, orc, z.Skills.Get(skill.IDLexAeterna), 1, 0)
			if !orc.sc.Has(sc.SCAeterna) {
				t.Fatal("no Lex Aeterna")
			}
			if got := z.calcSkillAttack(battle.TypeMagic, p.entity, p.entity, orc, d, 1, 0, nil).Damage; got != 2*base {
				t.Errorf("damage %d with Lex Aeterna, want %d", got, 2*base)
			}
			if orc.sc.Has(sc.SCAeterna) {
				t.Error("Lex Aeterna stayed after the hit")
			}
			if got := z.calcSkillAttack(battle.TypeMagic, p.entity, p.entity, orc, d, 1, 0, nil).Damage; got != base {
				t.Errorf("damage %d after Lex Aeterna, want %d", got, base)
			}
		})
	}
}

// Resurrection revives a dead player with a percent of its HP by level,
// and hits the undead (SkillResurrection and status_revive).
func TestResurrection(t *testing.T) {
	z, p, ms := priestZone(t, reference.Renewal, 1015)
	zombie := ms[0]
	other := goldenPlayerIn(t, z, highPriest, 0, 0)
	o := other.entity
	o.x, o.y = 21, 20
	o.hp = 0
	z.die(o, nil)
	if !o.dead {
		t.Fatal("the player did not die")
	}
	d := z.Skills.Get(skill.IDResurrection)
	if !z.skillUseID(p.entity, o, d, 4) {
		t.Fatal("Resurrection did not start on a dead player")
	}
	// The cast takes longer than a dead player waits to return, so it
	// ends here.
	z.castCancel(p.entity, false)
	z.castendNoDamage(p.entity, o, d, 4, 0)
	if o.dead || o.hp != int64(o.status.MaxHP)*80/100 {
		t.Errorf("dead %v, HP %d of %d", o.dead, o.hp, o.status.MaxHP)
	}
	// A living player who is not undead cannot be the target.
	if z.skillUseID(p.entity, o, d, 4) {
		t.Error("Resurrection started on a living player")
	}
	// On the undead it is a magic hit.
	hp := zombie.hp
	z.castendNoDamage(p.entity, zombie, d, 4, 0)
	if zombie.hp >= hp {
		t.Error("Resurrection did not hit the zombie")
	}
}

// Turn Undead hits only the undead, and with every roll forced to
// succeed it kills at once (battle_calc_magic_attack and
// skill_castend_id_check).
func TestTurnUndead(t *testing.T) {
	z, p, ms := priestZone(t, reference.Renewal, 1015, 1002)
	zombie, poring := ms[0], ms[1]
	d := z.Skills.Get(skill.IDTurnUndead)
	if r := z.castEndIDCheck(p.entity, poring, d); r != skillFailSilent {
		t.Errorf("check on a poring %v, want a silent failure", r)
	}
	z.castendDamage(p.entity, poring, d, 10, 0)
	if poring.hp != int64(poring.status.MaxHP) {
		t.Error("Turn Undead hit a poring")
	}
	z.castendDamage(p.entity, zombie, d, 10, 0)
	if !zombie.dead {
		t.Errorf("zombie HP %d after Turn Undead", zombie.hp)
	}
}

// Lex Divina silences an enemy a second after the cast, and cures a
// silenced unit (SkillLexDivina and skill_timerskill).
func TestLexDivinaSilencesLater(t *testing.T) {
	z, p, ms := priestZone(t, reference.Renewal, 1023)
	orc := ms[0]
	d := z.Skills.Get(skill.IDLexDivina)
	z.castendNoDamage(p.entity, orc, d, 1, 0)
	if orc.sc.Has(sc.SCSilence) {
		t.Fatal("silenced at once")
	}
	step(z, z.now+50, z.now+1000)
	if !orc.sc.Has(sc.SCSilence) {
		t.Fatal("not silenced after a second")
	}
	z.castendNoDamage(p.entity, orc, d, 1, 0)
	if orc.sc.Has(sc.SCSilence) {
		t.Error("a second cast did not cure the silence")
	}
	// An ally needs to be silenced.
	if r := z.castEndIDCheck(p.entity, p.entity, d); r != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
		t.Errorf("check on a free ally %v", r)
	}
	startOn(z, p.entity, sc.SCSilence, 1, 10000)
	if r := z.castEndIDCheck(p.entity, p.entity, d); r != wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
		t.Errorf("check on a silenced ally %v", r)
	}
}

// Status Recovery ends stun on the living and blinds the undead a
// second later (SkillStatusRecovery).
func TestStatusRecovery(t *testing.T) {
	z, p, ms := priestZone(t, reference.Renewal, 1015)
	zombie := ms[0]
	d := z.Skills.Get(skill.IDStatusRecovery)
	startOn(z, p.entity, sc.SCStun, 1, 10000)
	z.castendNoDamage(p.entity, p.entity, d, 1, 0)
	if p.entity.sc.Has(sc.SCStun) {
		t.Error("stun stayed")
	}
	z.castendNoDamage(p.entity, zombie, d, 1, 0)
	step(z, z.now+50, z.now+1000)
	if !zombie.sc.Has(sc.SCBlind) {
		t.Error("the zombie is not blind")
	}
}

// Suffragium lowers the next cast time: in pre-renewal it ends with that
// cast, in renewal it stays (skill_castfix_sc and skill_vfcastfix).
func TestSuffragiumCastTime(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, _ := priestZone(t, mode)
			me := p.entity
			d := z.Skills.Get(skill.IDHolyLight)
			before := d.CastTimeFor(z.caster(me), 1, z.renewal(), true)
			z.castendNoDamage(me, me, z.Skills.Get(skill.IDSuffragium), 3, 0)
			after := d.CastTimeFor(z.caster(me), 1, z.renewal(), true)
			if after.Milliseconds >= before.Milliseconds {
				t.Errorf("cast %d ms with Suffragium, %d without", after.Milliseconds, before.Milliseconds)
			}
			if after.EndSuffragium != (mode == reference.PreRenewal) {
				t.Errorf("ends Suffragium %v", after.EndSuffragium)
			}
		})
	}
}

// Magnificat halves the wait of natural SP regen (status_calc_regen_rate
// and status_natural_heal).
func TestMagnificatRegen(t *testing.T) {
	z, p, _ := priestZone(t, reference.Renewal)
	p.sp = 1
	z.castendNoDamage(p.entity, p.entity, z.Skills.Get(skill.IDMagnificat), 5, 0)
	start := z.now
	step(z, start+50, start+4000)
	if p.sp <= 1 {
		t.Errorf("SP %d after 4 seconds with Magnificat", p.sp)
	}
}

// B.S. Sacramenti needs two Acolyte class players beside the caster,
// who pay 10 SP each, and gives holy armor to the players on its area
// (skill_check_pc_partner and SkillBenedictioSanctissimiSacramenti).
func TestBenedictioNeedsPartners(t *testing.T) {
	z, p, _ := priestZone(t, reference.Renewal)
	me := p.entity
	me.dir = 0
	d := z.Skills.Get(skill.IDBenedictio)
	if r := (benedictio{}).castBegin(z, me, d, 5); r != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
		t.Fatalf("cast start without partners: %v", r)
	}
	a := goldenPlayerIn(t, z, highPriest, 0, 0)
	b := goldenPlayerIn(t, z, highPriest, 0, 0)
	a.entity.x, a.entity.y = me.x-1, me.y
	b.entity.x, b.entity.y = me.x+1, me.y
	if r := (benedictio{}).castBegin(z, me, d, 5); r != wathenav1.SkillFailure_SKILL_FAILURE_UNSPECIFIED {
		t.Fatalf("cast start with partners: %v", r)
	}
	spA := a.sp
	z.chargePartners()
	if a.sp != spA-10 {
		t.Errorf("partner SP %d, want %d", a.sp, spA-10)
	}
	z.castendPos(me, me.x, me.y, d, 5, 0)
	for _, e := range []*entity{me, a.entity, b.entity} {
		if !e.sc.Has(sc.SCBenedictio) {
			t.Error("no holy armor")
		}
	}
}

// Redemptio fails for a player without a party (SkillRedemptio).
func TestRedemptioNeedsParty(t *testing.T) {
	z, p, _ := priestZone(t, reference.Renewal)
	events(p)
	z.castendNoDamage(p.entity, p.entity, z.Skills.Get(skill.IDRedemptio), 1, 0)
	z.stepAt(z.now + 50)
	if f := skillFailures(events(p)); len(f) != 1 {
		t.Errorf("failures %v, want one", f)
	}
}

// A player cannot give a monster Aspersio or Assumptio.
func TestAspersioAndAssumptioNotOnMonsters(t *testing.T) {
	z, p, ms := priestZone(t, reference.Renewal, 1002)
	z.castendNoDamage(p.entity, ms[0], z.Skills.Get(skill.IDAspersio), 5, 0)
	z.castendNoDamage(p.entity, ms[0], z.Skills.Get(skill.IDAssumptio), 5, 0)
	if ms[0].sc.Has(sc.SCAspersio) || ms[0].sc.Has(sc.SCAssumptio) {
		t.Error("a monster got a Priest buff")
	}
}
