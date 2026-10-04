package zone

import (
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/rules/rulestest"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Tests of the ruleset flags that the zone reads (decision 0019), each
// at both values on both bases where the loader allows it.

// otherMode is the rAthena mode that is not mode.
func otherMode(mode reference.Mode) reference.Mode {
	if mode == reference.Renewal {
		return reference.PreRenewal
	}
	return reference.Renewal
}

// baseMode is a base's mode.
func baseMode(renewal bool) reference.Mode {
	if renewal {
		return reference.Renewal
	}
	return reference.PreRenewal
}

// zoneRules gives a zone a world's rules, as gamedata.LoadMods does: the
// zone's rules, job tables with the flags of player stats, and the skill
// records that a flag set to the other mode takes from that mode. The
// zone's tables and skills become copies, as golden zones share theirs.
func zoneRules(t *testing.T, z *Zone, rl *rules.Rules) {
	t.Helper()
	if z.Mods == nil {
		z.Mods = &Mods{}
	}
	z.Mods.Rules = rl
	if z.Status != nil {
		tb := *z.Status
		var other *status.Data
		if rl.ASPD != tb.Renewal() {
			other = jobStatsOf(t, baseMode(rl.ASPD))
		}
		if err := tb.SetASPD(rl.ASPD, other); err != nil {
			t.Fatal(err)
		}
		tb.SetSizeTable(!rl.SizeTable)
		tb.SetRules(rl)
		z.Status = &tb
	}
	if z.Skills != nil {
		otherDB := readOnlySkills(t, otherMode(baseMode(z.Skills.Renewal())))
		db := z.Skills.Clone()
		for id, v := range rl.Skills {
			if v == rl.Renewal {
				continue
			}
			if d, o := db.Get(id), otherDB.Get(id); d != nil && o != nil && d.Name == o.Name {
				db.Put(o.Copy())
			}
		}
		z.Skills = db
	}
	// Players' stats follow the new tables.
	for e := range z.entities.all() {
		if e.player != nil {
			z.recalc(e.player)
		}
	}
}

// readOnly holds the job tables that zoneRules reads and never
// changes, loaded once per test binary. Loading them and the skill
// database for each zone made TestMixedRulesetsZone take most of the
// package's time.
var readOnly sync.Map

// jobStatsOf returns a mode's job_stats.json. Callers must not change
// it.
func jobStatsOf(t *testing.T, mode reference.Mode) *status.Data {
	t.Helper()
	key := "job_stats " + string(mode)
	if v, ok := readOnly.Load(key); ok {
		return v.(*status.Data)
	}
	d := &status.Data{}
	if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", d); err != nil {
		t.Fatal(err)
	}
	v, _ := readOnly.LoadOrStore(key, d)
	return v.(*status.Data)
}

// readOnlySkills returns a mode's skill database. Callers must not
// change it or its skills: zoneRules only reads and copies them.
func readOnlySkills(t *testing.T, mode reference.Mode) *skill.DB {
	t.Helper()
	return loadSkillSource(t, mode).db
}

// with returns the core rules of a base, changed by set.
func with(base bool, set func(*rules.Rules)) *rules.Rules {
	rl := rules.Base(base)
	set(rl)
	return rl
}

// TestHealFormulaFlagZone checks both values of heal_formula on both
// bases in a cast of Heal: the HP healed is skill_calc_heal of the
// flag's mode.
func TestHealFormulaFlagZone(t *testing.T) {
	priest := goldenPlayer{Job: 8, Build: "magic", Sex: 1, JobLevel: 50, Skills: [][2]int{{skill.IDHeal, 10}}}
	for _, mode := range reference.Modes {
		healed := map[bool]int64{}
		for _, flag := range []bool{true, false} {
			z := goldenZone(t, mode)
			p := goldenPlayerIn(t, z, priest, 0, 0)
			zoneRules(t, z, with(mode == reference.Renewal, func(r *rules.Rules) { r.HealFormula = flag }))
			me := p.entity
			me.hp = 1
			z.castendNoDamage(me, me, z.Skills.Get(skill.IDHeal), 10, 0)
			want := min(skill.CalcHeal(me.status, me.status, skill.IDHeal, 10, true, flag, minRoll{}), int64(me.status.MaxHP)-1)
			if got := me.hp - 1; got != want {
				t.Errorf("%s base, heal_formula renewal %v: healed %d, want %d", mode, flag, got, want)
			}
			healed[flag] = me.hp - 1
		}
		if healed[true] == healed[false] {
			t.Errorf("%s base: both formulas healed %d", mode, healed[true])
		}
	}
}

// TestCastTimeFlagZone checks both values of cast_time on both bases in
// a cast of Fire Bolt: the cast lasts what skill_castfix of the flag's
// mode gives, and the two values differ.
func TestCastTimeFlagZone(t *testing.T) {
	wizard := goldenPlayer{Job: 9, Build: "magic", Sex: 1, JobLevel: 50, Skills: [][2]int{{skill.IDFireBolt, 10}}}
	for _, mode := range reference.Modes {
		ms := map[bool]int64{}
		for _, flag := range []bool{true, false} {
			z := goldenZone(t, mode, 1002)
			p := goldenPlayerIn(t, z, wizard, 0, 0)
			zoneRules(t, z, with(mode == reference.Renewal, func(r *rules.Rules) { r.CastTime = flag }))
			me, target := p.entity, monsterOf(z, 1002)
			d := z.Skills.Get(skill.IDFireBolt)
			want := d.CastTimeFor(z.caster(me), 10, flag, true).Milliseconds
			if !z.skillUseID(me, target, d, 10) || me.cast == nil {
				t.Fatalf("%s: the cast did not start", mode)
			}
			if got := me.cast.end - z.now; got != int64(want) {
				t.Errorf("%s base, cast_time renewal %v: cast %d ms, want %d", mode, flag, got, want)
			}
			ms[flag] = me.cast.end - z.now
		}
		if ms[true] == ms[false] {
			t.Errorf("%s base: both cast times are %d", mode, ms[true])
		}
	}
}

// TestBossProtectionFlag checks both values of boss_protection on both
// bases:
//   - renewal: Ankle Snare does not catch a boss.
//   - pre_renewal: it does, for a fifth of its time, at least 30 ms per
//     caster level plus 100.
//
// On a renewal base, Blade Stop catches a boss's attack for 2 seconds
// with renewal's protection, and not at all with pre-renewal's.
func TestBossProtectionFlag(t *testing.T) {
	const baphomet = 1039
	for _, mode := range reference.Modes {
		for _, flag := range []bool{true, false} {
			z, p := hunterZone(t, mode, 11, baphomet)
			zoneRules(t, z, with(mode == reference.Renewal, func(r *rules.Rules) { r.BossProtection = flag }))
			m := monsterOf(z, baphomet)
			m.x, m.y = 35, 35
			g := placeTrap(t, z, p, skill.IDAnkleSnare, 5)
			trap := g.units[0]
			m.x, m.y = trap.x+1, trap.y+1
			run(z, p, z.now+50, z.now+100)
			s := m.sc.Get(sc.SCAnkle)
			switch {
			case flag && s != nil:
				t.Errorf("%s base, renewal protection: Ankle Snare caught a boss", mode)
			case !flag && s == nil:
				t.Errorf("%s base, pre-renewal protection: Ankle Snare missed a boss", mode)
			case !flag:
				sec := int64(20000 / 5)
				want := max(sec*int64(m.status.Agi)/-200+sec, int64(30*(99+100)))
				if got := s.Remaining(z.now); got > want || got < want-200 {
					t.Errorf("%s base: held %d ms, want %d", mode, got, want)
				}
			}
		}
	}
	for _, flag := range []bool{true, false} {
		z, p, m := monkZone(t)
		zoneRules(t, z, with(true, func(r *rules.Rules) { r.BossProtection = flag }))
		m.status.Class = battle.ClassBoss
		e := p.entity
		giveSpheres(z, p, 1)
		cast(z, p, skill.IDBladeStop, 5, nil)
		run(z, p, 100, 100)
		m.target, m.repeat = e, true
		m.nextAttack = 0
		run(z, p, 150, 150)
		caught := m.sc.Get(sc.SCBladestop)
		switch {
		case flag && (caught == nil || caught.Remaining(z.now) > 2000):
			t.Errorf("renewal protection: the boss was not held for 2 seconds: %+v", caught)
		case !flag && caught != nil:
			t.Error("pre-renewal protection: Blade Stop caught a boss")
		}
	}
}

// TestEnsembleSongsFlag checks both values of ensemble_songs on both
// bases with Whistle: renewal's song is a status on the caster, and
// pre-renewal's a 7x7 unit around it, with the skill record of the
// flag's mode.
func TestEnsembleSongsFlag(t *testing.T) {
	for _, mode := range reference.Modes {
		for _, flag := range []bool{true, false} {
			z, p, _ := bardZone(t, mode, bard, 1002)
			zoneRules(t, z, with(mode == reference.Renewal, func(r *rules.Rules) {
				r.EnsembleSongs = flag
				r.Skills = map[int]bool{}
				for _, id := range rules.SongSkills {
					r.Skills[id] = flag
				}
			}))
			me := p.entity
			z.castendNoDamage(me, me, z.Skills.Get(skill.IDWhistle), 10, 0)
			e := me.sc.Get(sc.SCWhistle)
			switch {
			case flag && (len(z.groups) != 0 || e == nil || e.Val2 != 38):
				t.Errorf("%s base, renewal songs: %d groups, SC_WHISTLE %+v", mode, len(z.groups), e)
			case !flag && (len(z.groups) != 1 || e != nil || !me.sc.Has(sc.SCDancing)):
				t.Errorf("%s base, pre-renewal songs: %d groups, SC_WHISTLE %+v", mode, len(z.groups), e)
			case !flag:
				if g := onlyGroup(t, z); len(g.units) != 49 {
					t.Errorf("%s base: %d cells, want 49", mode, len(g.units))
				}
			}
		}
	}
}

// TestBusyRestrictionsFlag checks both values of busy_restrictions on
// both bases: with it on, an item or skill refused during a conversation
// and an NPC click during a cast tell the player that it is busy; with it
// off they fail silently, as pre-renewal.
func TestBusyRestrictionsFlag(t *testing.T) {
	busy := func(p *Player) bool {
		defer func() { p.pending = nil }()
		for _, e := range p.pending {
			if c := e.GetChat(); c != nil && strings.Contains(c.GetText(), "busy") {
				return true
			}
		}
		return false
	}
	for _, mode := range reference.Modes {
		for _, flag := range []bool{true, false} {
			z := goldenZone(t, mode)
			p := goldenPlayerIn(t, z, knightPlayer, 0, 0)
			zoneRules(t, z, with(mode == reference.Renewal, func(r *rules.Rules) { r.BusyRestrictions = flag }))
			p.pending = nil
			p.talk = &talk{}
			slot := 0
			z.handle(command{player: p, use: &slot})
			if got := busy(p); got != flag {
				t.Errorf("%s base, busy_restrictions %v: item during a talk: busy %v", mode, flag, got)
			}
			z.handle(command{player: p, useSkill: &useSkillCommand{skill: skill.IDBash, level: 1}})
			if got := busy(p); got != flag {
				t.Errorf("%s base, busy_restrictions %v: skill during a talk: busy %v", mode, flag, got)
			}
			p.talk = nil
			p.entity.cast = &casting{def: z.Skills.Get(skill.IDBash), lv: 1}
			z.click(p, 12345)
			if got := busy(p); got != flag {
				t.Errorf("%s base, busy_restrictions %v: NPC click during a cast: busy %v", mode, flag, got)
			}
		}
	}
}

// TestSkillRebalanceZone checks a skill_rebalance flag on both bases in
// the zone: renewal's Back Stab stuns its target, which the forced
// minimum roll always does, and pre-renewal's does not.
func TestSkillRebalanceZone(t *testing.T) {
	rogue := goldenPlayer{Job: 17, Build: "melee", Sex: 1, JobLevel: 50, Skills: [][2]int{{skill.IDBackStab, 10}}}
	for _, mode := range reference.Modes {
		for _, flag := range []bool{true, false} {
			z := goldenZone(t, mode, 1002)
			p := goldenPlayerIn(t, z, rogue, 0, 0)
			zoneRules(t, z, with(mode == reference.Renewal, func(r *rules.Rules) { r.Skills = map[int]bool{skill.IDBackStab: flag} }))
			m := monsterOf(z, 1002)
			backStab{}.additionalEffect(z, p.entity, m, z.Skills.Get(skill.IDBackStab), 10)
			if got := m.sc.Has(sc.SCStun); got != flag {
				t.Errorf("%s base, RG_BACKSTAP renewal %v: stunned %v", mode, flag, got)
			}
		}
	}
}

// TestMixedRulesetsZone runs skill_status.json of the base for each
// sampled mixed ruleset (rulestest.Mixes), each part of a case when the
// flags it reads keep the base's value (statusParts). Each case builds
// its own zone, so it runs the first two mixes of each base that run
// cases.
func TestMixedRulesetsZone(t *testing.T) {
	testrace.SkipUnderRace(t, "it checks sampled mixed rulesets on one goroutine")
	if testing.Short() {
		t.Skip("the golden skill cases run once per mix")
	}
	const limit = 2
	ran := 0
	done := map[bool]int{}
	for i, rl := range rulestest.Mixes() {
		mode := baseMode(rl.Renewal)
		if done[rl.Renewal] >= limit {
			continue
		}
		t.Run(fmt.Sprintf("%s mix %d", mode, i), func(t *testing.T) {
			n := checkSkillStatusGolden(t, mode, rl)
			ran += n
			if n > 0 {
				done[rl.Renewal]++
			}
		})
	}
	t.Logf("%d case parts ran under mixed rulesets", ran)
	if ran == 0 {
		t.Error("no case ran")
	}
}

// minRoll is rnd_force_min for the skill formulas.
type minRoll struct{}

func (minRoll) IntN(int) int { return 0 }
