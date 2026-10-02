package zone

import (
	"math/bits"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

func init() {
	// The recorder's Golem stood within Eternal Chaos's 4 cells, so its
	// DEF fell to 0 for the attack after the song. The golden test's
	// Golem stands apart, as Frost Nova needs. TestEternalChaosGolem
	// checks the song on a Golem in range.
	statusGaps["BD_ETERNALCHAOS"] = "the recorder's Golem stood in the song's area"
}

// The recorder's performers, with every skill of their trees.
var (
	bardSkills = [][2]int{{1, 9}, {43, 10}, {44, 10}, {45, 10}, {46, 10}, {47, 10}, {304, 1}, {305, 1}, {306, 1},
		{307, 5}, {308, 1}, {309, 5}, {310, 5}, {311, 1}, {312, 1}, {313, 5}}
	bard = goldenPlayer{Job: 19, Build: "ranged", Sex: 1, JobLevel: 50, Skills: append(append([][2]int{}, bardSkills...),
		[2]int{315, 10}, [2]int{316, 5}, [2]int{317, 5}, [2]int{318, 5}, [2]int{319, 10}, [2]int{320, 10},
		[2]int{321, 10}, [2]int{322, 10})}
	dancer = goldenPlayer{Job: 20, Build: "ranged", Sex: 0, JobLevel: 50, Skills: append(append([][2]int{}, bardSkills...),
		[2]int{323, 10}, [2]int{324, 5}, [2]int{325, 5}, [2]int{326, 5}, [2]int{327, 10}, [2]int{328, 10},
		[2]int{329, 10}, [2]int{330, 10})}
	clown = goldenPlayer{Job: 4020, Build: "ranged", Sex: 1, JobLevel: 50, Skills: append(append([][2]int{}, bard.Skills...),
		[2]int{394, 10}, [2]int{395, 5}, [2]int{396, 1}, [2]int{487, 5}, [2]int{488, 5}, [2]int{489, 5},
		[2]int{5068, 1})}
	knightPlayer = goldenPlayer{Job: 7, Build: "melee", Sex: 1, JobLevel: 50}
)

// Weapons of the performers: a Violin and a Rope.
const (
	itemViolin = 1901
	itemRope   = 1950
)

// bardZone is a golden zone of a mode with a performer at (20, 20),
// holding its weapon, and the given monsters from (20, 22) east. The
// monsters neither move nor fight back.
func bardZone(t *testing.T, mode reference.Mode, who goldenPlayer, mobs ...uint32) (*Zone, *Player, []*entity) {
	t.Helper()
	z := goldenZone(t, mode, mobs...)
	weapon := uint32(itemViolin)
	if who.Sex == 0 {
		weapon = itemRope
	}
	p := goldenPlayerIn(t, z, who, weapon, 0)
	var ms []*entity
	for _, id := range mobs {
		m := monsterOf(z, id)
		m.status.Mode &^= modeCanAttack
		ms = append(ms, m)
	}
	return z, p, ms
}

// otherPlayer adds a level 99 player of a sample at a cell.
func otherPlayer(t *testing.T, z *Zone, who goldenPlayer, x, y int) *Player {
	t.Helper()
	p := goldenPlayerIn(t, z, who, 0, 0)
	p.entity.x, p.entity.y = x, y
	return p
}

// onlyGroup is the zone's one unit group.
func onlyGroup(t *testing.T, z *Zone) *unitGroup {
	t.Helper()
	if len(z.groups) != 1 {
		t.Fatalf("%d groups, want 1", len(z.groups))
	}
	for _, g := range z.groups {
		return g
	}
	return nil
}

// Renewal's songs are statuses that skill_castend_song starts on the
// caster's party, which is the caster alone without a party, and the
// harmful songs on the enemies around (skill_castend_song,
// skill_apply_songs).
func TestRenewalSongs(t *testing.T) {
	z, p, ms := bardZone(t, reference.Renewal, bard, 1002)
	poring := ms[0]
	poring.status.MaxHP, poring.hp = 100000, 100000
	knight := otherPlayer(t, z, knightPlayer, 21, 20)
	me := p.entity
	d := z.Skills.Get(skill.IDWhistle)
	sp := p.sp
	if !z.skillUseID(me, me, d, 10) {
		t.Fatal("the cast did not start")
	}
	step(z, z.now+50, z.now+2000)
	e := me.sc.Get(sc.SCWhistle)
	if e == nil || e.Val1 != 10 || e.Val2 != 38 || e.Val3 != 50 {
		t.Fatalf("SC_WHISTLE %+v, want level 10 with 38 FLEE and 50 perfect dodge", e)
	}
	if knight.entity.sc.Has(sc.SCWhistle) || poring.sc.Has(sc.SCWhistle) {
		t.Error("the song reached a unit outside the caster's party")
	}
	if want := int64(z.requirement(me, d, 10).SP); sp-p.sp != want {
		t.Errorf("SP %d less, want %d", sp-p.sp, want)
	}
	// Dissonance hits the enemies around, and no one else.
	knightHP := knight.entity.hp
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDDissonance), 1, 0)
	if poring.hp >= 100000 || knight.entity.hp != knightHP {
		t.Errorf("Poring HP %d, Knight HP %d of %d", poring.hp, knight.entity.hp, knightHP)
	}
}

// A song needs an instrument or a whip (skill_check_condition_castbegin's
// weapon check).
func TestSongNeedsInstrument(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z := goldenZone(t, mode)
			p := goldenPlayerIn(t, z, bard, 0, 0)
			if z.skillUseID(p.entity, p.entity, z.Skills.Get(skill.IDWhistle), 1) {
				t.Fatal("a bare-handed Bard sang")
			}
			if f := skillFailures(p.pending); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_THIS_WEAPON {
				t.Errorf("failures %v", f)
			}
		})
	}
}

// Pre-renewal's songs place a 7x7 unit around the performer, who gets
// SC_DANCING and pays SP every few seconds. Players in the unit get the
// song with the group's values, keep it 20 seconds after they leave, and
// the unit follows its performer (skill_unitsetting, skill_unit_onplace,
// skill_unit_onleft, skill_unit_move_unit_group, status_change_timer's
// SC_DANCING).
func TestPreRenewalSongUnit(t *testing.T) {
	z, p, ms := bardZone(t, reference.PreRenewal, bard, 1002)
	poring := ms[0]
	knight := otherPlayer(t, z, knightPlayer, 22, 20)
	me := p.entity
	d := z.Skills.Get(skill.IDWhistle)
	z.castendNoDamage(me, me, d, 10, 0)
	g := onlyGroup(t, z)
	if len(g.units) != 49 {
		t.Fatalf("%d cells, want 49", len(g.units))
	}
	dancing := me.sc.Get(sc.SCDancing)
	if dancing == nil || dancing.Val1 != skill.IDWhistle|10<<16 || dancing.Val2 != int(g.id) || dancing.Val4 != 0 {
		t.Fatalf("SC_DANCING %+v, want Whistle level 10 of group %d", dancing, g.id)
	}
	if me.sc.Has(sc.SCWhistle) {
		t.Error("the performer got its own song")
	}
	if poring.sc.Has(sc.SCWhistle) {
		t.Error("the song reached a monster (UF_NOMOB)")
	}
	st := me.status
	ml := p.skillLevel(skill.IDMusicalLesson)
	val1, val2 := 10+st.Agi/10+ml/2, (10+1)/2+st.Luk/30+ml/5
	e := knight.entity.sc.Get(sc.SCWhistle)
	if e == nil || e.Val1 != 10 || e.Val2 != val1 || e.Val3 != val2 {
		t.Fatalf("SC_WHISTLE %+v, want %d and %d", e, val1, val2)
	}
	if e.Timer() != z.now+g.limit+unitTimerInterval {
		t.Errorf("timer at %d, want %d", e.Timer(), z.now+g.limit+unitTimerInterval)
	}
	// The Knight walks out: the song lingers 20 seconds.
	knight.entity.x = 30
	z.skillUnitMove(knight.entity)
	if e := knight.entity.sc.Get(sc.SCWhistle); e == nil || e.Val4 != 1 || e.Timer() != z.now+20000 {
		t.Fatalf("SC_WHISTLE after leaving %+v", e)
	}
	// The performer steps east: the unit follows.
	me.x = 21
	z.skillUnitMove(me)
	if g.x != 21 || z.unitOnCell(knight.entity, 24, 20, skill.IDWhistle, false) == nil ||
		z.unitOnCell(knight.entity, 17, 20, skill.IDWhistle, false) != nil {
		t.Errorf("the unit did not follow: center %d", g.x)
	}
	// Whistle costs 1 SP every 5 seconds.
	sp := p.sp
	step(z, z.now+50, z.now+5000)
	if sp-p.sp != 1 {
		t.Errorf("SP %d less after 5 seconds, want 1", sp-p.sp)
	}
	// The lingering song ends.
	step(z, z.now+50, z.now+20000)
	if knight.entity.sc.Has(sc.SCWhistle) {
		t.Error("the song still lingers")
	}
}

// Adaptation ends the dance and its unit, and Encore sings it again at
// half its SP (SkillAmp, SkillEncore, skill_get_requirement).
func TestAdaptationAndEncore(t *testing.T) {
	z, p, _ := bardZone(t, reference.PreRenewal, bard)
	me := p.entity
	d := z.Skills.Get(skill.IDWhistle)
	z.castendNoDamage(me, me, d, 10, 0)
	onlyGroup(t, z)
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDAdaptation), 1, 0)
	if me.sc.Has(sc.SCDancing) || len(z.groups) != 0 {
		t.Fatal("Adaptation did not end the dance")
	}
	full := z.requirement(me, d, 10).SP
	sp := p.sp
	if !z.skillUseID(me, me, z.Skills.Get(skill.IDEncore), 1) {
		t.Fatalf("Encore did not start: %v", skillFailures(p.pending))
	}
	step(z, z.now+50, z.now+500)
	if !me.sc.Has(sc.SCDancing) || me.sc.Get(sc.SCDancing).Val1&0xFFFF != skill.IDWhistle {
		t.Fatal("Encore did not sing Whistle again")
	}
	if sp-p.sp != int64(full/2) {
		t.Errorf("Encore cost %d SP, want %d", sp-p.sp, full/2)
	}
	if p.perform.skillOld != 0 {
		t.Error("Encore's half cost was not used up")
	}
}

// Ensembles need a partner of the other performer job in the caster's
// party (skill_check_pc_partner). Without parties they fail before they
// start, even with a Dancer next to the Bard.
func TestEnsemblesNeedPartner(t *testing.T) {
	ensembles := []int{skill.IDLullaby, skill.IDRichmanKim, skill.IDEternalChaos, skill.IDDrumBattlefield,
		skill.IDRingNibelungen, skill.IDRokisWeil, skill.IDIntoAbyss, skill.IDSiegfried, skill.IDMoonlit}
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, _ := bardZone(t, mode, clown)
			partner := goldenPlayerIn(t, z, dancer, itemRope, 0)
			partner.entity.x = 21
			me := p.entity
			sp := p.sp
			for _, id := range ensembles {
				p.pending = nil
				if z.skillUseID(me, me, z.Skills.Get(id), 1) {
					t.Errorf("%d started without a party", id)
				}
				if f := skillFailures(p.pending); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_LEVEL {
					t.Errorf("%d: failures %v", id, f)
				}
			}
			if p.sp != sp || me.sc.Has(sc.SCDancing) {
				t.Errorf("SP %d of %d, dancing %v", p.sp, sp, me.sc.Has(sc.SCDancing))
			}
		})
	}
}

// Frost Joker freezes, and Scream stuns, every enemy and party member
// around the caster 3 seconds after the cast, but not the caster, and
// not other players without a party (skill_timerskill,
// skill_frostjoke_scream, applyAdditionalEffects).
func TestFrostJokerAndScream(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			for _, c := range []struct {
				who     goldenPlayer
				id, typ int
			}{{bard, skill.IDFrostJoker, sc.SCFreeze}, {dancer, skill.IDScream, sc.SCStun}} {
				z, p, ms := bardZone(t, mode, c.who, 1002)
				poring := ms[0]
				knight := otherPlayer(t, z, knightPlayer, 19, 20)
				me := p.entity
				z.castendNoDamage(me, me, z.Skills.Get(c.id), 5, 0)
				if poring.sc.Has(c.typ) {
					t.Fatalf("%d acted at once", c.id)
				}
				step(z, z.now+50, z.now+3000)
				if !poring.sc.Has(c.typ) {
					t.Errorf("%d: the Poring has no status %d", c.id, c.typ)
				}
				if me.sc.Has(c.typ) || knight.entity.sc.Has(c.typ) {
					t.Errorf("%d reached the caster or a player outside its party", c.id)
				}
			}
		})
	}
}

// A monster's Frost Joker reaches the monsters around too, its party
// (battle_check_target with BCT_PARTY), but not the caster.
func TestFrostJokerOnParty(t *testing.T) {
	z, _, _ := bardZone(t, reference.Renewal, bard, 1002, 1002)
	var porings []*entity
	for e := range z.entities.all() {
		if e.monster != nil && e.monster.spawn.Mob == 1002 {
			porings = append(porings, e)
		}
	}
	if len(porings) != 2 {
		t.Fatalf("%d Porings", len(porings))
	}
	a, b := porings[0], porings[1]
	z.castendNoDamage(a, a, z.Skills.Get(skill.IDFrostJoker), 5, 0)
	step(z, z.now+50, z.now+3000)
	if !b.sc.Has(sc.SCFreeze) {
		t.Error("the other monster did not freeze")
	}
	if a.sc.Has(sc.SCFreeze) {
		t.Error("the caster froze")
	}
}

// scripted is a source whose first values make IntN return chosen
// numbers, then rolls the lowest, as rnd_force_min.
type scripted struct {
	vals []uint64
	i    int
}

func (s *scripted) Uint64() uint64 {
	if s.i < len(s.vals) {
		s.i++
		return s.vals[s.i-1]
	}
	return 1
}

// roll is the source value for which IntN(n) returns k: the middle of
// k's share of 2^64.
func roll(k, n int) uint64 {
	q, _ := bits.Div64(uint64(2*k+1), 0, uint64(2*n)) // #nosec G115 -- small test values.
	return q
}

// tarotCheck checks a card's effect on the Orc.
type tarotCheck func(t *testing.T, orc *entity)

// tarotHas checks a status with its val1.
func tarotHas(typ, val1 int) tarotCheck {
	return func(t *testing.T, orc *entity) {
		t.Helper()
		if e := orc.sc.Get(typ); e == nil || e.Val1 != val1 {
			t.Errorf("status %d %+v, want val1 %d", typ, e, val1)
		}
	}
}

// tarotLost checks the HP the Orc lost.
func tarotLost(lost int64) tarotCheck {
	return func(t *testing.T, orc *entity) {
		t.Helper()
		if got := int64(orc.status.MaxHP) - orc.hp; got != lost {
			t.Errorf("HP lost %d, want %d", got, lost)
		}
	}
}

// Tarot Card of Fate: the roll of 8% per level, then one of 14 cards
// with the official chances, each as skill_tarotcard.
func TestTarotCards(t *testing.T) {
	cases := []struct {
		name  string
		rolls [][2]int
		check []tarotCheck
	}{
		{"magician", [][2]int{{10, 100}}, []tarotCheck{tarotHas(sc.SCIncmatkrate, -50)}},
		{"chariot", [][2]int{{30, 100}}, []tarotCheck{tarotLost(1000)}},
		{"strength", [][2]int{{40, 100}}, []tarotCheck{tarotHas(sc.SCIncatkrate, -50)}},
		{"hanged man ankle", [][2]int{{63, 100}, {0, 3}}, []tarotCheck{tarotHas(sc.SCAnkle, 5)}},
		{"hanged man freeze", [][2]int{{63, 100}, {1, 3}}, []tarotCheck{tarotHas(sc.SCFreeze, 5)}},
		{"death", [][2]int{{69, 100}}, []tarotCheck{tarotHas(sc.SCCurse, 5), tarotHas(sc.SCPoison, 5), func(t *testing.T, orc *entity) {
			if orc.hp != 1 {
				t.Errorf("HP %d after Coma, want 1", orc.hp)
			}
		}}},
		{"temperance", [][2]int{{74, 100}}, []tarotCheck{tarotHas(sc.SCConfusion, 5)}},
		{"devil", [][2]int{{82, 100}}, []tarotCheck{tarotLost(6666), tarotHas(sc.SCIncatkrate, -50),
			tarotHas(sc.SCIncmatkrate, -50), tarotHas(sc.SCCurse, 100)}},
		{"tower", [][2]int{{83, 100}}, []tarotCheck{tarotLost(4444)}},
		{"star", [][2]int{{85, 100}}, []tarotCheck{tarotHas(sc.SCStun, 5)}},
		{"sun", [][2]int{{90, 100}}, []tarotCheck{tarotHas(sc.SCIncatkrate, -20), tarotHas(sc.SCIncmatkrate, -20),
			tarotHas(sc.SCInchitrate, -20), tarotHas(sc.SCIncfleerate, -20), tarotHas(sc.SCIncdefrate, -20)}},
		// The wheel draws the Chariot, which rolls for an armor to break,
		// and the Tower.
		{"wheel of fortune", [][2]int{{62, 100}, {30, 100}, {0, 3}, {83, 100}}, []tarotCheck{tarotLost(5444)}},
		{"lovers", [][2]int{{47, 100}}, []tarotCheck{func(t *testing.T, orc *entity) {
			if orc.x == 20 && orc.y == 22 {
				t.Error("the Orc did not move")
			}
		}}},
	}
	for _, mode := range reference.Modes {
		for _, c := range cases {
			t.Run(string(mode)+" "+c.name, func(t *testing.T) {
				z, p, ms := bardZone(t, mode, clown, 1023)
				orc := ms[0]
				orc.status.MaxHP, orc.hp = 100000, 100000
				vals := []uint64{roll(0, 100)}
				for _, r := range c.rolls {
					vals = append(vals, roll(r[0], r[1]))
				}
				z.rand = rng.New(&scripted{vals: vals})
				me := p.entity
				sp := p.sp
				d := z.Skills.Get(skill.IDTarotCard)
				z.castendNoDamage(me, orc, d, 5, 0)
				for _, ch := range c.check {
					ch(t, orc)
				}
				if sp-p.sp != int64(d.Requires.SP.At(5)) {
					t.Errorf("SP %d less, want %d", sp-p.sp, d.Requires.SP.At(5))
				}
				if c.name == "sun" && orc.sc.Has(sc.SCTarotcard) != (mode == reference.Renewal) {
					t.Errorf("SC_TAROTCARD %v", orc.sc.Has(sc.SCTarotcard))
				}
			})
		}
	}
}

// A failed roll keeps the SP and tells the caster, and renewal's Sun
// keeps every later card away (SkillTarotCardOfFate).
func TestTarotFailsAndSunImmunity(t *testing.T) {
	z, p, ms := bardZone(t, reference.Renewal, clown, 1023)
	orc := ms[0]
	me := p.entity
	d := z.Skills.Get(skill.IDTarotCard)
	// 41 is above 8% of level 5.
	z.rand = rng.New(&scripted{vals: []uint64{roll(41, 100)}})
	sp := p.sp
	flag := 0
	tarotCard{}.castendNoDamage(z, me, orc, d, 5, &flag)
	if flag&skillNoConsumeReq == 0 || p.sp != sp || len(skillFailures(p.pending)) != 1 || !orc.sc.Empty() {
		t.Fatalf("flag %#x, SP %d of %d, statuses %v", flag, p.sp, sp, orc.sc.Sorted())
	}
	z.scStart(me, orc, sc.SCTarotcard, 100, 5, 30000)
	z.rand = rng.New(&scripted{vals: []uint64{roll(0, 100), roll(10, 100)}})
	flag = 0
	tarotCard{}.castendNoDamage(z, me, orc, d, 5, &flag)
	if flag&skillNoConsumeReq == 0 || orc.sc.Has(sc.SCIncmatkrate) {
		t.Error("a card passed the Sun's immunity")
	}
}

// Renewal's Eternal Chaos takes the DEF of the enemies around to 0
// (SC_ETERNALCHAOS in status_calc_def and status_calc_def2): with the
// recorder's Golem in range, the normal attack after the song deals
// what the recording shows.
func TestEternalChaosGolem(t *testing.T) {
	z, p, _ := bardZone(t, reference.Renewal, bard, 1040)
	golem := monsterOf(z, 1040)
	golem.x, golem.y = 22, 20
	me := p.entity
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDEternalChaos), 1, 0)
	if !golem.sc.Has(sc.SCEternalchaos) || me.sc.Has(sc.SCEternalchaos) {
		t.Fatalf("Golem %v, caster %v", golem.sc.Has(sc.SCEternalchaos), me.sc.Has(sc.SCEternalchaos))
	}
	if golem.status.Def != 0 || golem.status.Def2 != 0 {
		t.Errorf("DEF %d and %d, want 0", golem.status.Def, golem.status.Def2)
	}
	if got := goldenAttack(z, me, golem); got.Damage != 575 {
		t.Errorf("attack %+v, want 575", got)
	}
}

// Marionette Control lends half the caster's own stats to the target, at
// most to 99, and the second cast ends both statuses. A performer of the
// caster's sex cannot be controlled (SkillMarionetteControl,
// SC_MARIONETTE and SC_MARIONETTE2).
func TestMarionette(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p, _ := bardZone(t, mode, clown)
			knight := otherPlayer(t, z, knightPlayer, 21, 20)
			me := p.entity
			d := z.Skills.Get(skill.IDMarionette)
			before, mine := knight.entity.status.Str, me.status.Str
			give := min(p.char.Stats.Str/2, 99-before)
			z.castendNoDamage(me, knight.entity, d, 1, 0)
			z.flushStats()
			if !me.sc.Has(sc.SCMarionette) || !knight.entity.sc.Has(sc.SCMarionette2) {
				t.Fatal("no Marionette statuses")
			}
			if knight.entity.status.Str != before+give || me.status.Str != mine-p.char.Stats.Str/2 {
				t.Errorf("STR %d and %d, want %d and %d", knight.entity.status.Str, me.status.Str, before+give,
					mine-p.char.Stats.Str/2)
			}
			z.castendNoDamage(me, knight.entity, d, 1, 0)
			if me.sc.Has(sc.SCMarionette) || knight.entity.sc.Has(sc.SCMarionette2) {
				t.Error("the second cast did not end it")
			}
			other := otherPlayer(t, z, bard, 19, 20)
			p.pending = nil
			z.castendNoDamage(me, other.entity, d, 1, 0)
			if other.entity.sc.Has(sc.SCMarionette2) || len(skillFailures(p.pending)) != 1 {
				t.Error("a Bard of the caster's sex was controlled")
			}
		})
	}
}

// Wand of Hermode turns magic on its target into a miss
// (status_isimmune in skill_attack).
func TestHermodeBlocksMagic(t *testing.T) {
	z, p, ms := bardZone(t, reference.Renewal, clown, 1002)
	me := p.entity
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDHermode), 5, 0)
	if !me.sc.Has(sc.SCHermode) {
		t.Fatal("no SC_HERMODE")
	}
	hp := me.hp
	if dmg := z.skillAttack(battle.TypeMagic, ms[0], ms[0], me, z.Skills.Get(skill.IDFireBolt), 5, 0); dmg != 0 || me.hp != hp {
		t.Errorf("Fire Bolt dealt %d", dmg)
	}
}

// Pre-renewal's Dissonance hits the enemies in its unit every 3 seconds,
// as a misc attack from the unit, but not the performer (UNT_DISSONANCE).
func TestDissonanceUnit(t *testing.T) {
	z, p, ms := bardZone(t, reference.PreRenewal, bard, 1002)
	poring := ms[0]
	poring.status.MaxHP, poring.hp = 100000, 100000
	me := p.entity
	hp := me.hp
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDDissonance), 5, 0)
	step(z, z.now+50, z.now+200)
	first := poring.hp
	if first >= 100000 {
		t.Fatal("no hit at once")
	}
	step(z, z.now+50, z.now+2500)
	if poring.hp != first {
		t.Error("a second hit before 3 seconds")
	}
	step(z, z.now+50, z.now+700)
	if poring.hp >= first {
		t.Error("no second hit after 3 seconds")
	}
	if me.hp != hp {
		t.Error("the performer was hit")
	}
}

// Longing for Freedom lets an ensemble's performer walk and attack in
// pre-renewal (SC_DANCING's conditions in status_calc_state).
func TestLongingForFreedom(t *testing.T) {
	z, p, _ := bardZone(t, reference.PreRenewal, clown)
	me := p.entity
	z.scStart4(me, me, sc.SCDancing, 100, skill.IDSiegfried, 0, 1, bctSelf, 60000)
	if c := me.cant(); !c.Move || !c.Attack {
		t.Fatalf("an ensemble's performer may move %v or attack %v", !c.Move, !c.Attack)
	}
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDLongingFreedom), 5, 0)
	if !me.sc.Has(sc.SCLonging) {
		t.Fatal("no SC_LONGING")
	}
	if c := me.cant(); c.Move || c.Attack {
		t.Errorf("with Longing for Freedom: cannot move %v, cannot attack %v", c.Move, c.Attack)
	}
	// It does not start without an ensemble.
	z.scEnd(me, sc.SCLonging)
	z.scEnd(me, sc.SCDancing)
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDLongingFreedom), 5, 0)
	if me.sc.Has(sc.SCLonging) {
		t.Error("Longing for Freedom without a dance")
	}
}

// Moonlit Water Mill's unit pushes out whoever is not performing it
// (UNT_MOONLIT in skill_unit_onplace).
func TestMoonlitPushes(t *testing.T) {
	z, p, ms := bardZone(t, reference.Renewal, clown, 1002)
	poring := ms[0]
	me := p.entity
	z.unitSetting(me, z.Skills.Get(skill.IDMoonlit), 1, me.x, me.y)
	if abs(poring.x-me.x) <= 4 && abs(poring.y-me.y) <= 4 && poring.x == 20 && poring.y == 22 {
		t.Errorf("the Poring stays at (%d, %d)", poring.x, poring.y)
	}
	if me.x != 20 || me.y != 20 {
		t.Error("the performer was pushed")
	}
}

// Bosses cannot be charmed (unit_skilluse_id2).
func TestCharmsFailOnBosses(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			for _, c := range []struct {
				who goldenPlayer
				id  int
			}{{bard, skill.IDPangVoice}, {dancer, skill.IDWinkCharm}} {
				z, p, ms := bardZone(t, mode, c.who, 1039)
				p.char.Skills[c.id] = 1
				z.recalc(p)
				if z.skillUseID(p.entity, ms[0], z.Skills.Get(c.id), 1) {
					t.Errorf("%d started on Baphomet", c.id)
				}
				if f := skillFailures(p.pending); len(f) != 1 || f[0] != wathenav1.SkillFailure_SKILL_FAILURE_TARGET {
					t.Errorf("%d: failures %v", c.id, f)
				}
			}
		})
	}
}

// Taking off the instrument ends the dance (pc_checkallowskill with
// dancing_weaponswitch_fix).
func TestUnequipEndsDance(t *testing.T) {
	z, p, _ := bardZone(t, reference.PreRenewal, bard)
	me := p.entity
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDWhistle), 1, 0)
	if !me.sc.Has(sc.SCDancing) {
		t.Fatal("not dancing")
	}
	for slot, s := range p.inventory.Slots {
		if s.ID == itemViolin {
			z.unequipItem(p, slot)
		}
	}
	if me.sc.Has(sc.SCDancing) || len(z.groups) != 0 {
		t.Error("the dance goes on without the Violin")
	}
}

// Pre-renewal's Apple of Idun heals the units in it every 6 seconds,
// but not its performer (UNT_APPLEIDUN).
func TestAppleOfIdunHeals(t *testing.T) {
	z, p, _ := bardZone(t, reference.PreRenewal, bard)
	knight := otherPlayer(t, z, knightPlayer, 21, 20)
	knight.entity.hp = 100
	me := p.entity
	me.hp = 100
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDAppleIdun), 10, 0)
	step(z, z.now+50, z.now+200)
	heal := skill.CalcHeal(me.status, knight.entity.status, skill.IDAppleIdun, 10, true, false, z.rand)
	if knight.entity.hp != 100+heal {
		t.Errorf("Knight HP %d, want %d", knight.entity.hp, 100+heal)
	}
	if me.hp != 100 {
		t.Errorf("the performer healed to %d", me.hp)
	}
	if !knight.entity.sc.Has(sc.SCAppleidun) {
		t.Error("no SC_APPLEIDUN on the Knight")
	}
}

// Renewal's Adaptation takes 20% off the SP of songs, and Into the Abyss
// waives gemstones (skill_get_requirement).
func TestPerformanceRequirements(t *testing.T) {
	z, p, _ := bardZone(t, reference.Renewal, bard)
	me := p.entity
	song := z.Skills.Get(skill.IDWhistle)
	base := z.requirement(me, song, 10).SP
	z.castendNoDamage(me, me, z.Skills.Get(skill.IDAdaptation), 1, 0)
	if !me.sc.Has(sc.SCAdaptation) {
		t.Fatal("no SC_ADAPTATION")
	}
	if got := z.requirement(me, song, 10).SP; got != base-base*20/100 {
		t.Errorf("Adaptation: %d SP, want %d", got, base-base*20/100)
	}
	gem := z.Skills.Get(skill.IDSafetyWall)
	if len(z.requirement(me, gem, 1).Items) == 0 {
		t.Fatal("Safety Wall needs no gem")
	}
	z.scStart(me, me, sc.SCIntoabyss, 100, 1, 60000)
	if items := z.requirement(me, gem, 1).Items; len(items) != 0 {
		t.Errorf("Into the Abyss kept the gems: %v", items)
	}
}

// Two songs on one cell turn it into Dissonance: it hits the enemies on
// it, a song on it lingers as when leaving, and it turns back when one
// song ends
// (skill_dance_overlap, skill_dance_switch).
func TestSongOverlapDissonance(t *testing.T) {
	z, p, ms := bardZone(t, reference.PreRenewal, bard, 1002)
	poring := ms[0]
	poring.status.MaxHP, poring.hp = 100000, 100000
	poring.x, poring.y = 23, 20
	other := goldenPlayerIn(t, z, bard, itemViolin, 0)
	other.entity.x = 26
	knight := otherPlayer(t, z, knightPlayer, 23, 21)
	d := z.Skills.Get(skill.IDWhistle)
	z.castendNoDamage(p.entity, p.entity, d, 10, 0)
	z.castendNoDamage(other.entity, other.entity, d, 10, 0)
	cell := z.unitOnCell(knight.entity, 23, 21, skill.IDWhistle, false)
	if cell == nil || !cell.unit.overlap || z.shownUnitID(cell) == cell.unit.group.unitID {
		t.Fatal("the shared cell does not overlap")
	}
	// The Knight got the first song, which lingers once the cell
	// overlaps, as on leaving it.
	if e := knight.entity.sc.Get(sc.SCWhistle); e == nil || e.Val4 != 1 {
		t.Errorf("SC_WHISTLE on the overlapped cell %+v, want it lingering", e)
	}
	step(z, z.now+50, z.now+200)
	if poring.hp >= 100000 {
		t.Error("the overlapped cell did not hit as Dissonance")
	}
	// One performer stops: the other's cell is a song again.
	z.scEnd(other.entity, sc.SCDancing)
	if cell := z.unitOnCell(knight.entity, 23, 21, skill.IDWhistle, false); cell == nil || cell.unit.overlap {
		t.Fatal("the cell did not turn back")
	}
	if !knight.entity.sc.Has(sc.SCWhistle) {
		t.Error("the song did not come back")
	}
}
