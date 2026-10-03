package gamedata

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
)

// Sampled mixed rulesets (strategy section 3.6, "Testing", and the
// phase 4 exit criteria): rulesets with several flags away from their
// base at once load, and the stats, damage, cast times, exp and drops
// they give are those of the flags that the server reads. Today these
// are aspd and size_table (ApplyRuleset). item_rolls goes to the zone
// through LoadMods, and TestRolledDropDistributions in server/zone
// checks both of its values on drops. The other flags are accepted and kept, and change nothing yet.

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

// sampleTargets are a Poring (medium), a Golem (large) and Baphomet
// (large boss).
var sampleTargets = []uint32{1002, 1040, 1039}

// minRand is rnd_force_min: every draw is the lowest.
type minRand struct{}

func (minRand) IntN(int) int { return 0 }

// fingerprint computes, with a ruleset's job tables (the mode's
// defaults when r is nil), each sample
// character's battle status, its forced-minimum damage against each
// target, its cast time, and the exp and drops of 300 seeded Poring
// kills.
func (f *rulesetFixture) fingerprint(t *testing.T, r *mod.Ruleset) string {
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
	// Without a ruleset, the tables keep their mode's defaults, as the
	// parity tests build them.
	if r != nil {
		if err := ApplyRuleset(jobs, r); err != nil {
			t.Fatal(err)
		}
	}
	renewal := f.mode == reference.Renewal
	attack := battle.Attack
	if !renewal {
		attack = battle.AttackPreRenewal
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
		cast := f.skills.Get(w.skill).CastTimeFor(&skill.Caster{Status: &st, Renewal: renewal}, w.lv, renewal, true)
		fmt.Fprintf(&b, "  cast %d lv %d: %+v\n", w.skill, w.lv, cast)
	}
	poring := f.monsters[1002].Reward
	rnd := rand.New(rand.NewPCG(1, 2)) // #nosec G404 -- a fixed seed for a repeatable test.
	var exp [2]int64
	drops := map[uint32]int{}
	for range 300 {
		var log progression.DamageLog
		log.Add(1, int64(poring.MaxHP), 0)
		k := f.tables.Kill(poring, &log, 1, func(uint32) progression.Attacker {
			return progression.Attacker{Online: true, BaseLevel: 10}
		})
		exp[0] += k.Rewards[0].BaseExp
		exp[1] += k.Rewards[0].JobExp
		for _, d := range f.tables.RollDrops(poring, k.DropModifier, &progression.Killer{Player: true, Luk: 1}, true, rnd) {
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

// TestSampledMixedRulesets draws 8 mixed rulesets per base from a fixed
// seed, each with every flag set to a random value, and checks that:
//   - the loader accepts each one and keeps every value;
//   - stats, damage, cast times, exp and drops equal those of the ruleset
//     that sets only aspd and size_table as the mix does, so the other
//     flags do not leak into them;
//   - aspd and size_table change what they should: the characters'
//     stats, and the Monk's damage with a knuckle;
//   - the core ruleset gives what the parity tests compute with the
//     mode's job tables alone;
func TestSampledMixedRulesets(t *testing.T) {
	core, err := mod.Resolve(nil, mod.Config{})
	if err != nil {
		t.Fatal(err)
	}
	names := slices.Sorted(maps.Keys(core.Ruleset.Flags))
	values := func(name string) []string {
		if v := core.Ruleset.Flags[name]; v == mod.Renewal || v == mod.PreRenewal {
			return []string{mod.Renewal, mod.PreRenewal}
		}
		return []string{"on", "off"}
	}
	const mixes = 8
	rnd := rand.New(rand.NewPCG(20261003, 7)) // #nosec G404 -- a fixed seed for a repeatable test.
	for _, base := range []string{mod.Renewal, mod.PreRenewal} {
		t.Run(base, func(t *testing.T) {
			mode := reference.Mode(mod.BaseMode(base))
			f := loadRulesetFixture(t, mode)
			own, err := mod.Resolve(nil, mod.Config{Base: base})
			if err != nil {
				t.Fatal(err)
			}
			// The rulesets that set only the flags the server reads.
			only := map[string]string{}
			for _, aspd := range values("aspd") {
				for _, size := range values("size_table") {
					s, err := rulesetMod(t, base, map[string]string{"aspd": aspd, "size_table": size})
					if err != nil {
						t.Fatal(err)
					}
					only[aspd+"/"+size] = f.fingerprint(t, s.Ruleset)
				}
			}
			coreKey := base + "/" + base
			// The core ruleset gives what the parity tests compute with
			// the mode's tables alone.
			if got, harness := f.fingerprint(t, own.Ruleset), f.fingerprint(t, nil); got != harness || got != only[coreKey] {
				t.Errorf("core.ruleset.%s differs from the parity harness's defaults:\n%s\nwant\n%s", base, got, harness)
			}
			for _, other := range []string{mod.Renewal, mod.PreRenewal} {
				if other == base {
					continue
				}
				// aspd changes the characters' stats, and size_table the
				// damage of the Monk's knuckle.
				if fingerprintLines(only[other+"/"+base], "job ") == fingerprintLines(only[coreKey], "job ") {
					t.Error("the aspd flag changed no stats")
				}
				if fingerprintLines(only[base+"/"+other], "  on ") == fingerprintLines(only[coreKey], "  on ") {
					t.Error("the size_table flag changed no damage")
				}
			}
			awayMin, awayMax := len(names), 0
			for i := range mixes {
				flags := map[string]string{}
				for _, name := range names {
					v := values(name)[rnd.IntN(2)]
					if name == "fourth_job_traits" && base == mod.PreRenewal {
						v = "off"
					}
					flags[name] = v
				}
				away := 0
				for name, v := range flags {
					if v != own.Ruleset.Flags[name] {
						away++
					}
				}
				awayMin, awayMax = min(awayMin, away), max(awayMax, away)
				s, err := rulesetMod(t, base, flags)
				if err != nil {
					t.Fatalf("mix %d %v: %v", i, flags, err)
				}
				for name, v := range flags {
					if got := s.Ruleset.Flag(name); got != v {
						t.Errorf("mix %d: %s is %q, want %q", i, name, got, v)
					}
				}
				got := f.fingerprint(t, s.Ruleset)
				if want := only[flags["aspd"]+"/"+flags["size_table"]]; got != want {
					t.Errorf("mix %d %s: differs from aspd=%s size_table=%s alone:\n%s\nwant\n%s", i, s.Ruleset, flags["aspd"], flags["size_table"], got, want)
				}
			}
			t.Logf("%s: %d mixes with %d to %d of %d flags away from the base match the rulesets of their aspd and size_table", base, mixes, awayMin, awayMax, len(names))
			if awayMin < 2 {
				t.Errorf("a mix has only %d flags away from the base", awayMin)
			}
		})
	}
}
