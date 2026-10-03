package gamedata

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
)

// modeConstants reads a mode's script constants.
func modeConstants(t *testing.T, mode reference.Mode) *constants.Data {
	t.Helper()
	data, err := reference.ReadFile("extracts/" + string(mode) + "/script_constants.json")
	if err != nil {
		t.Fatal(err)
	}
	c, err := constants.Read(strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestCoreJobRecordsMatchRAthena checks that the job tables and exp
// tables built from core's job records are rAthena's: every job's stats
// at several levels and builds, its maximum stats, and its exp tables.
func TestCoreJobRecordsMatchRAthena(t *testing.T) {
	for _, mode := range []reference.Mode{reference.Renewal, reference.PreRenewal} {
		t.Run(string(mode), func(t *testing.T) {
			var d status.Data
			if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", &d); err != nil {
				t.Fatal(err)
			}
			direct, err := status.New(&d)
			if err != nil {
				t.Fatal(err)
			}
			exp, _, err := LoadProgression(mode)
			if err != nil {
				t.Fatal(err)
			}
			records, names, err := CoreJobs(mode, modeConstants(t, mode))
			if err != nil {
				t.Fatal(err)
			}
			jobs := &Jobs{Records: records, names: names}
			built := direct.WithJobs(jobs.StatusData())
			builtExp := exp.WithJobs(jobs.ExpTables())
			compared := 0
			for _, j := range d.Jobs {
				id := j.ID
				m1, _ := direct.MapID(id)
				m2, _ := built.MapID(id)
				if m1 != m2 || direct.MaxJobLevel(id) != built.MaxJobLevel(id) {
					t.Errorf("job %d: map job ID %#x, want %#x", id, m2, m1)
				}
				for _, male := range []bool{true, false} {
					for _, lv := range [][2]int{{1, 1}, {50, 10}, {j.MaxBaseLevel, j.MaxJobLevel}} {
						for _, b := range []string{"all_1", "melee", "magic"} {
							c := status.Character{Job: id, Male: male, BaseLevel: lv[0], JobLevel: lv[1], Stats: Builds[b]}
							want, err1 := direct.Calc(c)
							got, err2 := built.Calc(c)
							if (err1 == nil) != (err2 == nil) || !reflect.DeepEqual(want, got) {
								t.Fatalf("job %d %v %s: %+v %v, want %+v %v", id, lv, b, got.Battle, err2, want.Battle, err1)
							}
							compared++
						}
					}
					c := status.Character{Job: id, Male: male}
					p1, err1 := direct.MaxParameter(&c)
					p2, err2 := built.MaxParameter(&c)
					if p1 != p2 || (err1 == nil) != (err2 == nil) {
						t.Errorf("job %d: max stats %d, want %d", id, p2, p1)
					}
				}
				e1, ok1 := exp.ExpOf(id)
				e2, ok2 := builtExp.ExpOf(id)
				if ok1 != ok2 || !reflect.DeepEqual(e1, e2) {
					t.Errorf("job %d: exp tables differ", id)
				}
				if !slices.Equal(direct.NextJobs(id), built.NextJobs(id)) {
					t.Errorf("job %d: next jobs %v, want %v", id, built.NextJobs(id), direct.NextJobs(id))
				}
			}
			t.Logf("%s: %d jobs, %d stat calculations match", mode, len(d.Jobs), compared)
		})
	}
}

// TestCoreJobRecords checks the record fields of a few core jobs.
func TestCoreJobRecords(t *testing.T) {
	records, names, err := CoreJobs(reference.Renewal, testConstants(t))
	if err != nil {
		t.Fatal(err)
	}
	knight := records[names["knight"]].Data.Info
	if names["knight"] != 7 || knight.DisplayName != "Knight" || knight.Tier != status.TierSecond || knight.Transcendent ||
		knight.BaseJob != 7 || !slices.Equal(knight.Inherits, []int{0, 1}) || len(knight.ChangesFrom) != 1 || knight.ChangesFrom[0].Job != 1 {
		t.Errorf("knight %+v", knight)
	}
	lk := records[names["lord_knight"]].Data.Info
	if lk.DisplayName != "Lord Knight" || !lk.Transcendent || lk.BaseJob != 7 || lk.ChangesFrom[0].Job != names["swordman_high"] {
		t.Errorf("lord knight %+v", lk)
	}
	if h := records[names["swordman_high"]].Data.Info; h.DisplayName != "High Swordman" || h.Tier != status.TierFirst {
		t.Errorf("high swordman %+v", h)
	}
	if tk := records[names["taekwon"]].Data.Info; tk.Tier != status.TierExpanded || len(tk.ChangesFrom) != 0 {
		t.Errorf("taekwon %+v", tk)
	}
	if rk := records[names["rune_knight"]].Data.Info; rk.Tier != status.TierThird {
		t.Errorf("rune knight %+v", rk)
	}
}

// jobSet loads the mods' jobs and skills of a set, over renewal's core.
func jobSet(t *testing.T, set *mod.Set) (*Jobs, *status.Tables, *skill.Trees, error) {
	t.Helper()
	ids, err := mod.LoadIDs(context.Background(), &mod.MemoryIDs{})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := modJobs(context.Background(), set, reference.Renewal, ids, testConstants(t))
	if err != nil {
		return nil, nil, nil, err
	}
	tables, _, err := StartCharacter(reference.Renewal, "all_1")
	if err != nil {
		t.Fatal(err)
	}
	scripts, err := set.AllScripts()
	if err != nil {
		t.Fatal(err)
	}
	_, trees, _, err := modSkills(context.Background(), set, reference.Renewal, ids, testConstants(t), map[string]uint32{}, scripts, jobs)
	if err != nil {
		return nil, nil, nil, err
	}
	return jobs, tables.WithJobs(jobs.StatusData()), trees, nil
}

func TestModJobs(t *testing.T) {
	set := skillSet(t, "", map[string]string{
		"data/jobs.yml": `
- name: example.a:Desperado
  display_name: Desperado
  look: gunslinger
  tier: second
  changes_from: [{job: gunslinger, job_level: 40}]
  inherits: [novice, gunslinger]
  max_job_level: 50
  max_stats: 120
  hp: {factor: 120, increase: 500}
  aspd: {revolver: 60}
  bonus_stats: [{level: 1, dex: 2}, {level: 10, agi: 1}]
- name: example.a:Pistolero
  look: example.a:Desperado
  transcendent: true
  changes_from: []
- name: knight
  display_name: Sir Knight
  max_weight: 30000
`,
		"data/skill_trees.yml": `
- job: example.a:Desperado
  skill: GS_GLITTERING
  max_level: 5
- job: gunslinger
  skill: SM_BASH
  max_level: 3
`,
	})
	jobs, tables, trees, err := jobSet(t, set)
	if err != nil {
		t.Fatal(err)
	}
	id, ok := jobs.ID("example.a:Desperado")
	if !ok || id != 10_000 {
		t.Fatalf("Desperado has ID %d", id)
	}
	gs, _ := jobs.ID("gunslinger")
	info, _ := tables.Info(id)
	if info.DisplayName != "Desperado" || info.Look != gs || info.EquipsAs != gs || info.Mod != "example.a" || info.Tier != status.TierSecond {
		t.Errorf("Desperado %+v", info)
	}
	// Its class is the Rebellion's, a second job of the Gunslinger line.
	if m, _ := tables.MapID(id); m != 0x108 {
		t.Errorf("map job ID %#x", m)
	}
	if next := tables.NextJobs(gs); !slices.Contains(next, id) {
		t.Errorf("next jobs of the Gunslinger %v", next)
	}
	r, err := tables.Calc(status.Character{Job: id, Male: true, BaseLevel: 50, JobLevel: 10, Stats: Builds["all_1"]})
	if err != nil {
		t.Fatal(err)
	}
	if r.Base.Dex != 3 || r.Base.Agi != 2 {
		t.Errorf("job bonuses: dex %d agi %d", r.Base.Dex, r.Base.Agi)
	}
	if p, _ := tables.MaxParameter(&status.Character{Job: id}); p != 120 {
		t.Errorf("max stats %d", p)
	}
	// The tree has the inherited Gunslinger skills, its own entry, and the
	// entry a mod added to the Gunslinger, which it inherits.
	tree := trees.Job(id)
	for _, skillID := range []int{512, 1, 5} { // GS_GLITTERING, NV_BASIC, SM_BASH
		if tree.Skill(skillID) == nil {
			t.Errorf("Desperado's tree lacks skill %d", skillID)
		}
	}
	if trees.Job(gs).Skill(5) == nil {
		t.Error("the Gunslinger's tree lacks the mod's entry")
	}
	// A job whose look is a mod job starts from that job.
	pid, _ := jobs.ID("example.a:Pistolero")
	pinfo, _ := tables.Info(pid)
	if pinfo.Look != gs || pinfo.Tier != status.TierSecond || !pinfo.Transcendent || len(pinfo.ChangesFrom) != 0 || pinfo.MaxStats != 120 {
		t.Errorf("Pistolero %+v", pinfo)
	}
	if m, _ := tables.MapID(pid); m != 0x100108 {
		t.Errorf("Pistolero's map job ID %#x", m)
	}
	if tables.DisplayName(7) != "Sir Knight" {
		t.Errorf("knight's display name %q", tables.DisplayName(7))
	}
}

func TestModJobErrors(t *testing.T) {
	cases := []struct {
		yml  string
		want []string
	}{
		{"- name: example.a:X\n  display_name: X\n", []string{"a new job needs look"}},
		{"- name: example.a:X\n  look: knight\n  tier: fifth\n  colour: red\n", []string{"tier must be one of", `"colour" is not a field`}},
		{"- name: knight\n  tier: third\n", []string{"tier comes from rAthena for a core job"}},
		{"- name: example.a:X\n  look: knight\n  changes_from: [{job: pirate}]\n", []string{"pirate is not a job"}},
		{"- name: example.a:X\n  look: knight\n  max_job_level: 70\n", []string{"max_job_level 70 need"}},
		{"- name: example.a:X\n  look: knight\n  hp_table: [1, 2]\n", []string{"hp_table must list one whole number from 1 per base level"}},
		{"- name: example.a:X\n  look: example.a:Y\n- name: example.a:Y\n  look: example.a:X\n", []string{"leads back to itself"}},
	}
	for _, c := range cases {
		set := skillSet(t, "", map[string]string{"data/jobs.yml": c.yml})
		_, _, _, err := jobSet(t, set)
		if err == nil {
			t.Errorf("%s: no error", c.yml)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(err.Error(), w) {
				t.Errorf("%s: %v, want %q", c.yml, err, w)
			}
		}
	}
}

// TestItemJobNames checks the jobs field as a list of names: a core job
// sets rAthena's Jobs bits of its class, and a mod job is listed by ID.
func TestItemJobNames(t *testing.T) {
	set := skillSet(t, "", map[string]string{
		"data/jobs.yml": "- name: example.a:Desperado\n  look: gunslinger\n  tier: second\n",
		"data/items.yml": `
- name: example.a:Badge
  type: armor
  location: 16
  jobs: [knight, example.a:Desperado]
`,
	})
	ids, err := mod.LoadIDs(context.Background(), &mod.MemoryIDs{})
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := modJobs(context.Background(), set, reference.Renewal, ids, testConstants(t))
	if err != nil {
		t.Fatal(err)
	}
	base, err := LoadItemData(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	items, err := set.Items(context.Background(), base, ids, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := itemJobs(items, jobs); err != nil {
		t.Fatal(err)
	}
	d := item.New(items.Data).Get(items.IDs["example.a:Badge"])
	desperado, _ := jobs.ID("example.a:Desperado")
	if *d.Jobs != (item.JobMask{0, 1 << 1, 0}) || !slices.Equal(d.JobIDs, []int{desperado}) || !d.JobRestricted {
		t.Fatalf("badge jobs %v %v", d.Jobs, d.JobIDs)
	}
	tables, _, err := StartCharacter(reference.Renewal, "all_1")
	if err != nil {
		t.Fatal(err)
	}
	tables = tables.WithJobs(jobs.StatusData())
	for job, want := range map[int]bool{7: true, 4008: true, 1: false, 24: false, desperado: true} {
		m, _ := tables.MapID(job)
		e, _ := tables.EquipMapID(job)
		w := &item.Wearer{Job: job, MapID: m, EquipsAs: e != m, EquipMapID: e, Male: true, BaseLevel: 99, Renewal: true}
		if got := item.CanEquip(d, w, 0) == item.EquipOK; got != want {
			t.Errorf("job %d wears the badge: %v, want %v", job, got, want)
		}
	}
}
