package gamedata

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/rules/rulestest"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
)

// Sampled mixed rulesets (strategy section 3.6, "Testing", and decision
// 0019): the mixes of rulestest.Mixes load through the mod loader and
// become the same rules, and the flags that change data (skill
// cooldowns, skill records and the level penalty) give the other mode's
// values. Each package runs the recordings that apply to the same mixes:
// TestMixedRulesetsBattle, TestMixedRulesetsSkills,
// TestMixedRulesetsStatus, TestMixedRulesetsStatusEffects,
// TestMixedRulesetsLevelPenalty and TestMixedRulesetsZone.

// rulesetFixture is the data of one mode that every ruleset of its base
// shares.
type rulesetFixture struct {
	mode     reference.Mode
	items    *item.Table
	monsters map[uint32]Monster
	tables   *progression.Tables
	skills   *skill.DB
	// jobs and refine are the job and refine tables' data, which each
	// ruleset's job tables start from.
	jobs   status.Data
	refine status.RefineData
}

func loadRulesetFixture(t *testing.T, mode reference.Mode) *rulesetFixture {
	t.Helper()
	data, err := LoadItemData(mode)
	if err != nil {
		t.Fatal(err)
	}
	tables, prog, err := LoadProgression(mode)
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := LoadMonsters(mode, tables, prog)
	if err != nil {
		t.Fatal(err)
	}
	skills, _, err := LoadSkills(mode)
	if err != nil {
		t.Fatal(err)
	}
	f := &rulesetFixture{mode: mode, items: item.New(data), monsters: monsters, tables: tables, skills: skills}
	if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", &f.jobs); err != nil {
		t.Fatal(err)
	}
	if err := reference.LoadFile("extracts/"+string(mode)+"/refine_db.json", &f.refine); err != nil {
		t.Fatal(err)
	}
	return f
}

// sampleWearer is a character of the fingerprint with its equipment.
type sampleWearer struct {
	job   int
	items []uint32
	// skill is a skill it casts, at level lv.
	skill, lv int
}

// sampleWearers are a Novice with a Knife, a Monk with Waghnakh, which
// the size table changes, and a Swordman with a Sword and a Guard, whose
// shield the aspd flag reads.
var sampleWearers = []sampleWearer{
	{job: 0, items: []uint32{1201}, skill: 28, lv: 1},
	{job: 15, items: []uint32{1801}, skill: 28, lv: 10},
	{job: 1, items: []uint32{1101, 2101}, skill: 19, lv: 5},
}

// sampleTargets are a Poring (medium), a Golem (large), Baphomet (large
// boss) and Whisper (ghost 3, which a neutral hit deals 50% to with
// renewal's element table and none with pre-renewal's).
var sampleTargets = []uint32{1002, 1040, 1039, 1179}

// minRand is rnd_force_min: every draw is the lowest.
type minRand struct{}

func (minRand) IntN(int) int { return 0 }

// fingerprint computes, with a world's rules (the parity harness's
// defaults when rl is nil), each sample character's battle status, its
// forced-minimum damage against each target, its cast time, and the exp
// and drops of 300 seeded Poring kills by a level 10 player.
func (f *rulesetFixture) fingerprint(t *testing.T, rl *rules.Rules) string {
	t.Helper()
	// The job and refine tables of StartCharacter, built from data
	// loaded once.
	jobs, err := status.New(&f.jobs)
	if err != nil {
		t.Fatal(err)
	}
	if err := jobs.SetRefine(&f.refine); err != nil {
		t.Fatal(err)
	}
	start := status.Character{Male: true, Stats: Builds["melee"]}
	renewal := f.mode == reference.Renewal
	attack := battle.Attack
	if !renewal {
		attack = battle.AttackPreRenewal
	}
	castFlag := renewal
	tables := f.tables
	// Without rules, the tables keep their mode's defaults, as the
	// parity tests build them.
	if rl != nil {
		if err := ApplyRuleset(jobs, rl); err != nil {
			t.Fatal(err)
		}
		attack = func(a, d *battle.Status, r battle.Rand) battle.Result { return battle.AttackRules(a, d, rl, r) }
		castFlag = rl.CastTime
		if tables, err = levelPenalty(f.tables, f.mode, rl); err != nil {
			t.Fatal(err)
		}
	}
	var b strings.Builder
	for _, w := range sampleWearers {
		c := start
		c.Job, c.BaseLevel, c.JobLevel = w.job, 90, min(40, jobs.MaxJobLevel(w.job))
		mapID, _ := jobs.MapID(w.job)
		wearer := &item.Wearer{Job: w.job, MapID: mapID, Male: true, BaseLevel: c.BaseLevel, Renewal: renewal}
		inv := item.NewInventory()
		for i, id := range w.items {
			inv.Slots[i] = item.Slot{ID: id, Amount: 1}
			d, ok := f.items.Lookup(id)
			if !ok {
				t.Fatalf("item %d is missing", id)
			}
			if _, res := inv.Equip(f.items, i, item.EquipPoint(d, wearer), wearer); res != item.EquipOK {
				t.Fatalf("job %d cannot equip %d: %d", w.job, id, res)
			}
		}
		c.Equipment = status.Wear(inv, f.items, func(_, _ int, d *item.Def) []status.Bonus { return status.StaticBonuses(d, nil) })
		res, err := jobs.Calc(c)
		if err != nil {
			t.Fatal(err)
		}
		st := res.Battle
		shown := st
		shown.Player = nil
		fmt.Fprintf(&b, "job %d: %+v\n  player %+v\n", w.job, shown, *st.Player)
		for _, id := range sampleTargets {
			src := st
			hit := attack(&src, f.monsters[id].Status, minRand{})
			fmt.Fprintf(&b, "  on %d: %+v\n", id, hit)
		}
		caster := &skill.Caster{Status: &st, Renewal: renewal, Rules: rl}
		cast := f.skills.Get(w.skill).CastTimeFor(caster, w.lv, castFlag, true)
		fmt.Fprintf(&b, "  cast %d lv %d: %+v\n", w.skill, w.lv, cast)
	}
	poring := f.monsters[1002].Reward
	rnd := rand.New(rand.NewPCG(1, 2)) // #nosec G404 -- a fixed seed for a repeatable test.
	var exp [2]int64
	drops := map[uint32]int{}
	for range 300 {
		var log progression.DamageLog
		log.Add(1, int64(poring.MaxHP), 0)
		k := tables.Kill(poring, &log, 1, func(uint32) progression.Attacker {
			return progression.Attacker{Online: true, BaseLevel: 10}
		})
		exp[0] += k.Rewards[0].BaseExp
		exp[1] += k.Rewards[0].JobExp
		for _, d := range tables.RollDrops(poring, k.DropModifier, &progression.Killer{Player: true, Luk: 1}, true, rnd) {
			drops[d.Item]++
		}
	}
	fmt.Fprintf(&b, "exp %v drops %v\n", exp, drops)
	return b.String()
}

// fingerprintLines keeps the lines of a fingerprint that start with
// prefix.
func fingerprintLines(fp, prefix string) string {
	var out []string
	for _, l := range strings.Split(fp, "\n") {
		if strings.HasPrefix(l, prefix) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// rulesetMods writes a ruleset mod, example.mixed, that extends a core
// ruleset with flags, and finds it.
func rulesetMods(t *testing.T, base string, flags map[string]string) []*mod.Mod {
	t.Helper()
	dir := t.TempDir()
	var y strings.Builder
	y.WriteString("id: example.mixed\nname: Mixed\nversion: 1.0.0\napi: 1.0\nprovides:\n  ruleset:\n    extends: core.ruleset." + base + "\n    flags:\n")
	for _, k := range slices.Sorted(maps.Keys(flags)) {
		fmt.Fprintf(&y, "      %s: %q\n", k, flags[k])
	}
	if err := os.MkdirAll(filepath.Join(dir, "mixed"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mixed", "mod.yaml"), []byte(y.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	found, err := mod.Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// rulesetMod resolves example.mixed as a world's ruleset.
func rulesetMod(t *testing.T, base string, flags map[string]string) (*mod.Set, error) {
	t.Helper()
	return mod.Resolve(rulesetMods(t, base, flags), mod.Config{Ruleset: "example.mixed", Base: base})
}

// readFlags are the flags the fingerprint reads: rl with every other
// flag at its base's value.
func readFlags(rl *rules.Rules) *rules.Rules {
	p := rules.Base(rl.Renewal)
	p.ASPD, p.SizeTable, p.CastTime, p.ElementTable = rl.ASPD, rl.SizeTable, rl.CastTime, rl.ElementTable
	p.LevelPenaltyExp, p.LevelPenaltyDrop = rl.LevelPenaltyExp, rl.LevelPenaltyDrop
	return p
}

// sameRules reports whether two rules are equal, an empty Skills map
// counting as none.
func sameRules(a, b *rules.Rules) bool {
	x, y := *a, *b
	x.Skills, y.Skills = nil, nil
	return reflect.DeepEqual(x, y) && maps.Equal(a.Skills, b.Skills)
}

// TestSampledMixedRulesets checks, for each base:
//   - The core ruleset gives what the parity tests compute with the
//     mode's tables and formulas alone.
//   - Each flag that the fingerprint reads changes what it should: aspd
//     the stats, size_table the Monk's damage with a knuckle,
//     element_table the damage to Whisper, cast_time the cast times,
//     and the level penalty flags the exp and drops.
//   - Each mix of rulestest.Mixes loads through the mod loader and
//     RulesOf gives the same rules, and its fingerprint equals that of
//     the flags the fingerprint reads, so the other flags do not leak.
func TestSampledMixedRulesets(t *testing.T) {
	testrace.SkipUnderRace(t, "it checks sampled mixed rulesets on one goroutine")
	mixes := rulestest.Mixes()
	for _, base := range []string{mod.Renewal, mod.PreRenewal} {
		t.Run(base, func(t *testing.T) {
			mode := reference.Mode(mod.BaseMode(base))
			f := loadRulesetFixture(t, mode)
			own, err := mod.Resolve(nil, mod.Config{Base: base})
			if err != nil {
				t.Fatal(err)
			}
			coreRules, _, err := RulesOf(own.Ruleset, f.skills)
			if err != nil {
				t.Fatal(err)
			}
			if !sameRules(coreRules, rules.Base(base == mod.Renewal)) {
				t.Errorf("core.ruleset.%s gives %+v", base, coreRules)
			}
			core := f.fingerprint(t, coreRules)
			if harness := f.fingerprint(t, nil); core != harness {
				t.Errorf("core.ruleset.%s differs from the parity harness's defaults:\n%s\nwant\n%s", base, core, harness)
			}
			for _, c := range []struct {
				flag, lines string
				set         func(*rules.Rules)
			}{
				{"aspd", "job ", func(r *rules.Rules) { r.ASPD = !r.ASPD }},
				{"size_table", "  on 1040", func(r *rules.Rules) { r.SizeTable = !r.SizeTable }},
				{"element_table", "  on 1179", func(r *rules.Rules) { r.ElementTable = !r.ElementTable }},
				{"cast_time", "  cast", func(r *rules.Rules) { r.CastTime = !r.CastTime }},
				{"level_penalty_exp", "exp", func(r *rules.Rules) { r.LevelPenaltyExp = !r.LevelPenaltyExp }},
				{"level_penalty_drop", "exp", func(r *rules.Rules) { r.LevelPenaltyDrop = !r.LevelPenaltyDrop }},
			} {
				rl := rules.Base(base == mod.Renewal)
				c.set(rl)
				if fingerprintLines(f.fingerprint(t, rl), c.lines) == fingerprintLines(core, c.lines) {
					t.Errorf("the %s flag changed nothing", c.flag)
				}
			}
			n := 0
			for i, mix := range mixes {
				if mix.Renewal != (base == mod.Renewal) {
					continue
				}
				n++
				s, err := rulesetMod(t, base, rulestest.Flags(mix))
				if err != nil {
					t.Fatalf("mix %d: %v", i, err)
				}
				rl, _, err := RulesOf(s.Ruleset, f.skills)
				if err != nil {
					t.Fatalf("mix %d: %v", i, err)
				}
				if !sameRules(rl, mix) {
					t.Errorf("mix %d: the loader gives %+v, want %+v", i, rl, mix)
				}
				if got, want := f.fingerprint(t, rl), f.fingerprint(t, readFlags(rl)); got != want {
					t.Errorf("mix %d %s: differs from its read flags alone:\n%s\nwant\n%s", i, s.Ruleset, got, want)
				}
				away, skills := rulestest.Away(mix)
				t.Logf("mix %d: %d flags and %d skill_rebalance flags away from the base", i, away, skills)
			}
			if n == 0 {
				t.Error("no mix of this base")
			}
		})
	}
}

// skillsWith returns core's skill database of a mode with the flags that
// change skill data applied.
func skillsWith(t *testing.T, mode reference.Mode, rl *rules.Rules) *skill.DB {
	t.Helper()
	db, _, err := LoadSkills(mode)
	if err != nil {
		t.Fatal(err)
	}
	if err := applySkillFlags(db, mode, rl); err != nil {
		t.Fatal(err)
	}
	return db
}

// TestSkillCooldownFlag checks both values of the skill_cooldown flag on
// both bases: every skill of the base that the flag's mode also has gets
// the cooldown recorded in that mode's skill_casting.json.
func TestSkillCooldownFlag(t *testing.T) {
	for _, mode := range reference.Modes {
		var ref struct {
			Cases []struct {
				Skill     int    `json:"skill"`
				SkillName string `json:"skill_name"`
				Level     int    `json:"level"`
				Cooldown  int    `json:"cooldown"`
			} `json:"cases"`
		}
		if err := reference.Load(mode, "skill_casting", &ref); err != nil {
			t.Fatal(err)
		}
		for _, base := range reference.Modes {
			rl := rules.Base(base == reference.Renewal)
			rl.SkillCooldown = mode == reference.Renewal
			db := skillsWith(t, base, rl)
			caster := &skill.Caster{Status: &battle.Status{Player: &battle.Player{}}}
			matched, total := 0, 0
			for _, c := range ref.Cases {
				d := db.Get(c.Skill)
				if d == nil || d.Name != c.SkillName {
					continue
				}
				total++
				if got := d.CooldownFor(caster, c.Level); got != c.Cooldown {
					t.Errorf("%s base, skill_cooldown of %s: %s lv %d cooldown %d, want %d", base, mode, c.SkillName, c.Level, got, c.Cooldown)
					continue
				}
				matched++
			}
			t.Logf("%s base, skill_cooldown of %s: %d of %d cases match", base, mode, matched, total)
			if total < 1000 {
				t.Errorf("%s base: only %d cases", base, total)
			}
		}
	}
}

// TestSkillRecordFlags checks that ensemble_songs and a skill_rebalance
// flag set to the other mode take the skill's skill_db record from that
// mode, with the base's cooldown while skill_cooldown is the base's:
// Whistle and Basilica are ground units in pre-renewal and no units in
// renewal.
func TestSkillRecordFlags(t *testing.T) {
	for _, base := range reference.Modes {
		renewal := base == reference.Renewal
		other := otherMode(base)
		plain, _, err := LoadSkills(base)
		if err != nil {
			t.Fatal(err)
		}
		otherDB, _, err := LoadSkills(other)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range []struct {
			name string
			set  func(*rules.Rules)
		}{
			{"BA_WHISTLE", func(r *rules.Rules) {
				r.EnsembleSongs = !renewal
				r.Skills = map[int]bool{}
				for _, id := range rules.SongSkills {
					r.Skills[id] = !renewal
				}
			}},
			{"HP_BASILICA", func(r *rules.Rules) { r.Skills = map[int]bool{rules.RebalancedSkills["HP_BASILICA"]: !renewal} }},
		} {
			rl := rules.Base(renewal)
			c.set(rl)
			db := skillsWith(t, base, rl)
			got, want, own := db.ByName(c.name), otherDB.ByName(c.name), plain.ByName(c.name)
			if (got.Unit == nil) != (want.Unit == nil) || got.Splash != want.Splash || got.Duration1 != want.Duration1 {
				t.Errorf("%s base: %s keeps its own record", base, c.name)
			}
			if got.Cooldown != own.Cooldown {
				t.Errorf("%s base: %s cooldown %v, want the base's %v", base, c.name, got.Cooldown, own.Cooldown)
			}
			if (own.Unit == nil) == (want.Unit == nil) {
				t.Errorf("%s: the modes' records do not differ in their unit", c.name)
			}
		}
	}
}

// TestRulesOfErrors checks the skill_rebalance flags RulesOf refuses: an
// unknown skill and a song. A skill whose versions the server does not
// tell apart gets a note.
func TestRulesOfErrors(t *testing.T) {
	db, _, err := LoadSkills(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ flag, want string }{
		{"skill_rebalance.NO_SUCH_SKILL", "there is no core skill NO_SUCH_SKILL"},
		{"skill_rebalance.BA_WHISTLE", "songs and ensembles follow ensemble_songs"},
	} {
		s, err := rulesetMod(t, mod.Renewal, map[string]string{c.flag: mod.PreRenewal})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := RulesOf(s.Ruleset, db); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.flag, err, c.want)
		}
	}
	s, err := rulesetMod(t, mod.Renewal, map[string]string{"skill_rebalance.SM_BASH": mod.PreRenewal})
	if err != nil {
		t.Fatal(err)
	}
	rl, notes, err := RulesOf(s.Ruleset, db)
	if err != nil || len(notes) != 1 || rl.Skill(5) {
		t.Errorf("SM_BASH: %v %v %v", rl, notes, err)
	}
}
