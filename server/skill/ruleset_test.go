package skill

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/rules/rulestest"
	"github.com/lxsmnsyc/wathena/server/status"
)

// Tests of the ruleset flags that skills read (decision 0019): each flag
// at both values on both bases, with expected values from the other
// mode's recording or from rAthena's formulas.

// withRules returns the core rules of a base, changed by set.
func withRules(base bool, set func(*rules.Rules)) *rules.Rules {
	rl := rules.Base(base)
	set(rl)
	return rl
}

// TestCastTimeFlag checks both values of the cast_time flag on both
// bases' skill data:
//   - pre_renewal: skill_castfix with DEX. At 150 DEX every skill that
//     DEX shortens casts at once, and at 75 DEX in half its time. There
//     is no fixed cast time, also with renewal's skill data.
//   - renewal: skill_vfcastfix. At DEX * 2 + INT of 530 only the fixed
//     part is left: FixedCastTime with renewal's data, and the default
//     20% of the cast time with pre-renewal's, which has no
//     FixedCastTime.
//   - With renewal's skill data and pre-renewal's cast time, the
//     pre-renewal recording's cast times come out for every case whose
//     skill has the same cast time in both modes.
func TestCastTimeFlag(t *testing.T) {
	for _, mode := range reference.Modes {
		db := loadDB(t, mode)
		for _, flag := range []bool{true, false} {
			name := fmt.Sprintf("%s data, cast_time renewal %v", mode, flag)
			caster := func(dex, int_ int) *Caster {
				return &Caster{Status: &battle.Status{Dex: dex, Int: int_, Player: &battle.Player{Cast: battle.CastBonus{CastRate: 100}}},
					Renewal: mode == reference.Renewal}
			}
			checked := 0
			for _, d := range db.All() {
				ct := d.CastTime.At(1)
				if ct <= 0 || d.CastTimeFlags&CastIgnoreDex != 0 || d.ID == IDChargeAttack || d.ID == IDFingerOffensive || d.ID == IDExtremityFist {
					continue
				}
				checked++
				if !flag {
					if got := d.CastTimeFor(caster(150, 1), 1, false, true).Milliseconds; got != 0 {
						t.Errorf("%s: %s at 150 DEX casts in %d", name, d.Name, got)
					}
					want := int(float64(ct) * float64(float32(75)) / 150)
					if got := d.CastTimeFor(caster(75, 1), 1, false, true).Milliseconds; got != want {
						t.Errorf("%s: %s at 75 DEX casts in %d, want %d", name, d.Name, got, want)
					}
					continue
				}
				want := d.FixedCastTime.At(1)
				if mode == reference.PreRenewal {
					want = ct * defaultFixedCastrate / 100
				}
				if got := d.CastTimeFor(caster(265, 0), 1, true, true).Milliseconds; got != max(want, 0) {
					t.Errorf("%s: %s at DEX 265 casts in %d, want the fixed %d", name, d.Name, got, want)
				}
			}
			if checked < 100 {
				t.Errorf("%s: only %d skills checked", name, checked)
			}
		}
	}

	// The pre-renewal recording, on renewal's skill data.
	re, pre := loadDB(t, reference.Renewal), loadDB(t, reference.PreRenewal)
	var jobs status.Data
	if err := reference.LoadFile("extracts/pre-renewal/job_stats.json", &jobs); err != nil {
		t.Fatal(err)
	}
	tables, err := status.New(&jobs)
	if err != nil {
		t.Fatal(err)
	}
	var ref struct {
		Players []samplePlayer `json:"players"`
		Cases   []castCase     `json:"cases"`
	}
	if err := reference.Load(reference.PreRenewal, "skill_casting", &ref); err != nil {
		t.Fatal(err)
	}
	learned := map[string]map[int]int{}
	jobLevels := map[string]int{}
	for _, p := range ref.Players {
		key := playerKey(p.Job, p.Build, p.Sex == 1)
		learned[key], jobLevels[key] = learnedMap(p.Skills), p.JobLevel
	}
	matched, total := 0, 0
	for _, c := range ref.Cases {
		rd, pd := re.Get(c.Skill), pre.Get(c.Skill)
		male := c.Sex == nil || *c.Sex == 1
		key := playerKey(c.Job, c.Build, male)
		if rd == nil || pd == nil || rd.CastTime.At(c.Level) != pd.CastTime.At(c.Level) || rd.CastTimeFlags != pd.CastTimeFlags ||
			learned[key] == nil {
			continue
		}
		res, err := tables.Calc(status.Character{Job: c.Job, Male: male, BaseLevel: 99, JobLevel: jobLevels[key], Stats: builds[c.Build], Skills: learned[key]})
		if err != nil {
			continue
		}
		total++
		st := res.Battle
		caster := &Caster{Status: &st, Rules: withRules(true, func(r *rules.Rules) { r.CastTime = false })}
		if got := rd.CastTimeFor(caster, c.Level, false, learned[key][c.Skill] > 0).Milliseconds; got != c.Cast {
			t.Errorf("%s lv %d on renewal data with pre-renewal cast time: %d, want %d", c.SkillName, c.Level, got, c.Cast)
			continue
		}
		matched++
	}
	t.Logf("pre-renewal recording on renewal data: %d of %d cases match", matched, total)
	if total < 1000 {
		t.Errorf("only %d cases share their cast time", total)
	}
}

// TestHealFormulaFlag checks both values of the heal_formula flag on both
// bases: skill_calc_heal follows the flag, so each mode's recorded Heal
// amounts come out on either base, and Heal's damage to the undead in a
// SkillContext reads the flag, not the base.
func TestHealFormulaFlag(t *testing.T) {
	for _, mode := range reference.Modes {
		var jobs status.Data
		if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", &jobs); err != nil {
			t.Fatal(err)
		}
		tables, err := status.New(&jobs)
		if err != nil {
			t.Fatal(err)
		}
		var ref struct {
			Players []samplePlayer `json:"players"`
			Cases   []castCase     `json:"cases"`
		}
		if err := reference.Load(mode, "skill_casting", &ref); err != nil {
			t.Fatal(err)
		}
		learned := map[string]map[int]int{}
		jobLevels := map[string]int{}
		for _, p := range ref.Players {
			key := playerKey(p.Job, p.Build, p.Sex == 1)
			learned[key], jobLevels[key] = learnedMap(p.Skills), p.JobLevel
		}
		flag := mode == reference.Renewal
		for _, base := range []bool{true, false} {
			rl := withRules(base, func(r *rules.Rules) { r.HealFormula = flag })
			heals := 0
			for _, c := range ref.Cases {
				male := c.Sex == nil || *c.Sex == 1
				key := playerKey(c.Job, c.Build, male)
				if c.Heal == nil || learned[key] == nil {
					continue
				}
				res, err := tables.Calc(status.Character{Job: c.Job, Male: male, BaseLevel: 99, JobLevel: jobLevels[key], Stats: builds[c.Build], Skills: learned[key]})
				if err != nil {
					t.Fatal(err)
				}
				st := res.Battle
				heals++
				if got := CalcHeal(&st, &st, c.Skill, c.Level, true, rl.HealFormula, forcedMin{}); got != c.Heal.ForcedMin {
					t.Errorf("base renewal %v, heal_formula %s: %s lv %d heals %d, want %d", base, mode, c.SkillName, c.Level, got, c.Heal.ForcedMin)
				}
				a := &battle.SkillContext{Src: &st, Target: &st, Skill: &battle.Skill{ID: IDHeal, Level: c.Level}, Renewal: base, Rules: rl}
				if got, want := (heal{}).BaseDamage(a, forcedMin{}), CalcHeal(&st, &st, IDHeal, c.Level, false, flag, forcedMin{}); got != want {
					t.Errorf("base renewal %v, heal_formula %s: Heal's damage %d, want %d", base, mode, got, want)
				}
			}
			if heals == 0 {
				t.Errorf("%s: no Heal cases", mode)
			}
		}
	}
}

// TestLevelDamageModFlag checks both values of the level_damage_mod flag
// on both bases: RE_LVL_DMOD(100) scales the renewal version of Sharp
// Shooting by base level / 100 above level 99 when the flag is on. The
// pre-renewal version and a level 99 caster do not change.
func TestLevelDamageModFlag(t *testing.T) {
	ratio := func(base, mod, skillRenewal bool, level int) int {
		rl := withRules(base, func(r *rules.Rules) {
			r.LevelDamageMod = mod
			r.Skills = map[int]bool{IDSharpShooting: skillRenewal}
		})
		a := &battle.SkillContext{Src: &battle.Status{Level: level, Player: &battle.Player{}}, Target: &battle.Status{},
			Skill: &battle.Skill{ID: IDSharpShooting, Level: 5}, Renewal: rl.Skill(IDSharpShooting), Rules: rl}
		r := 100
		sharpShooting{}.SkillRatio(a, &r)
		return r
	}
	for _, base := range []bool{true, false} {
		// Renewal's ratio at level 5: 300% + 300% per level.
		off := ratio(base, false, true, 150)
		if off != 1800 {
			t.Errorf("base renewal %v: ratio %d, want 1800", base, off)
		}
		if on := ratio(base, true, true, 150); on != off*150/100 {
			t.Errorf("base renewal %v: on at level 150: %d, want %d", base, on, off*150/100)
		}
		if on := ratio(base, true, true, 99); on != off {
			t.Errorf("base renewal %v: on at level 99: %d, want %d", base, on, off)
		}
		if a, b := ratio(base, true, false, 150), ratio(base, false, false, 150); a != b {
			t.Errorf("base renewal %v: the pre-renewal version changed: %d and %d", base, a, b)
		}
	}
}

// ratioContext is a SkillContext that the ratio formulas can read.
func ratioContext(id int, rl *rules.Rules) *battle.SkillContext {
	src := &battle.Status{Level: 99, Str: 80, Agi: 50, Vit: 40, Int: 60, Dex: 70, Luk: 30, MaxHP: 8000, MaxSP: 800, SP: 400,
		Player: &battle.Player{Skills: map[int]int{}, Weapon: battle.WeaponKatar, RightWeapon: battle.WeaponKatar, SpiritBall: 5, SP: 400}}
	tgt := &battle.Status{Level: 80, MaxHP: 20000, HP: 10000, Str: 40, Vit: 40, Int: 40, Dex: 40, Race: battle.RaceDemon, DefEle: battle.Dark, EleLv: 1}
	return &battle.SkillContext{Src: src, Target: tgt, Skill: &battle.Skill{ID: id, Level: 5}, Renewal: rl.Skill(id), Rules: rl}
}

// TestSkillRebalanceFlag checks the skill_rebalance flags of the skills
// with a ratio of their own: on either base, a skill's flag picks the
// ratio of its mode, the same ratio a context of that mode alone gives.
// The flags of other skills change nothing.
func TestSkillRebalanceFlag(t *testing.T) {
	differ := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(rules.RebalancedSkills)) {
		id := rules.RebalancedSkills[name]
		impl := registry[id]
		if impl == nil {
			continue
		}
		for _, base := range []bool{true, false} {
			for _, flag := range []bool{true, false} {
				rl := withRules(base, func(r *rules.Rules) { r.Skills = map[int]bool{id: flag, 1: !flag} })
				got, want := 100, 100
				impl.SkillRatio(ratioContext(id, rl), &got)
				impl.SkillRatio(ratioContext(id, rules.Of(flag)), &want)
				if got != want {
					t.Errorf("%s on base renewal %v with its flag renewal %v: ratio %d, want %d", name, base, flag, got, want)
				}
			}
		}
		re, pre := 100, 100
		impl.SkillRatio(ratioContext(id, rules.Of(true)), &re)
		impl.SkillRatio(ratioContext(id, rules.Of(false)), &pre)
		if re != pre {
			differ[name] = true
		}
	}
	t.Logf("%d skills have a ratio that differs between modes: %v", len(differ), slices.Sorted(maps.Keys(differ)))
	if len(differ) < 25 {
		t.Errorf("only %d rebalanced skills have a ratio that differs", len(differ))
	}
	// Through the battle code: Fire Ball at level 10 deals 240% with its
	// renewal version and 70% with its pre-renewal version, on either
	// base.
	src := &battle.Status{Level: 99, Int: 99, Dex: 99, MAtkMin: 1000, MAtkMax: 1000, Hit: 999, EleLv: 1}
	tgt := &battle.Status{Level: 1, MaxHP: 100000, DefEle: battle.Neutral, EleLv: 1, Size: battle.SizeMedium}
	db := loadDB(t, reference.Renewal)
	for _, base := range []bool{true, false} {
		dmg := map[bool]int64{}
		for _, flag := range []bool{true, false} {
			sk := db.Get(IDFireBall).Battle(src, 10, 0, 1)
			sk.Rules = withRules(base, func(r *rules.Rules) { r.Skills = map[int]bool{IDFireBall: flag} })
			dmg[flag] = battle.SkillAttackRules(battle.TypeMagic, src, tgt, sk, forcedMin{}).Damage
		}
		if dmg[true] <= dmg[false] || dmg[false] <= 0 {
			t.Errorf("base renewal %v: Fire Ball %d with renewal's ratio and %d with pre-renewal's", base, dmg[true], dmg[false])
		}
	}
}

// TestMixedRulesetsSkills runs the skill recordings that apply to each
// sampled mixed ruleset (rulestest.Mixes) of their mode's base:
//   - skill_casting.json, each part when the flags it reads keep the
//     mode's value (castingApplies).
//   - The forced minimum of every golden case of skill_damage.json whose
//     skill keeps its mode's version, when element_table is the base's,
//     and heal_formula for Heal's damage.
func TestMixedRulesetsSkills(t *testing.T) {
	fixtures := map[reference.Mode]*damageFixture{}
	cases := 0
	for i, rl := range rulestest.Mixes() {
		mode := reference.PreRenewal
		if rl.Renewal {
			mode = reference.Renewal
		}
		t.Run(fmt.Sprintf("%s mix %d", mode, i), func(t *testing.T) {
			checkCasting(t, mode, rl)
			if rl.ElementTable != rl.Renewal {
				return
			}
			if fixtures[mode] == nil {
				fixtures[mode] = loadDamageFixture(t, mode)
			}
			f := fixtures[mode]
			for j := range f.cases {
				c := &f.cases[j]
				if !goldenDamage(c.SkillName, c.Skill) || rl.Skill(c.Skill) != rl.Renewal ||
					(c.Skill == IDHeal || c.Skill == IDSanctuary || c.Skill == IDBenedictio) && rl.HealFormula != rl.Renewal {
					continue
				}
				def := f.db.Get(c.Skill)
				src, err := f.attacker(t, c, def)
				if err != nil {
					t.Fatal(err)
				}
				sk := def.Battle(src, c.Level, c.Flag, 1)
				sk.Rules = rl
				got := battle.SkillAttackRules(def.Type, src, f.mobs[c.Target], sk, forcedMin{})
				want := c.ForcedMin
				if got.Damage != want.Damage+want.Damage2 || got.Left != want.Damage2 || got.Kind != kindOf(want) ||
					got.Hits != want.Div || got.Flag != want.Flag {
					t.Errorf("%s: got %+v, want damage %d+%d", c.name(), got, want.Damage, want.Damage2)
				}
				cases++
			}
		})
	}
	t.Logf("%d skill_damage cases ran under mixed rulesets", cases)
	if cases == 0 {
		t.Error("no skill_damage case ran")
	}
}
