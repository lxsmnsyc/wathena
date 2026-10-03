package zone

import (
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/progression"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
)

// jobZone is a golden zone of a mode with rAthena's exp tables, and a
// player of a job at the given levels.
func jobZone(t *testing.T, mode reference.Mode, job, base, jobLevel int) (*Zone, *Player) {
	t.Helper()
	z := goldenZone(t, mode)
	var d progression.Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/progression.json", &d); err != nil {
		t.Fatal(err)
	}
	z.Progress = progression.New(&d, map[uint32]string{}, progression.DefaultConfig)
	p := add(z, "jobs")
	p.char.Job, p.char.Male = job, true
	p.char.Stats = status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}
	p.char.Skills = map[int]int{}
	p.progress = progression.Character{Job: job, BaseLevel: base, JobLevel: jobLevel}
	p.Look.Job = uint32(job) // #nosec G115 -- a job ID.
	z.recalc(p)
	events(p)
	return z, p
}

func TestNextJobs(t *testing.T) {
	g := loadGoldenData(t, reference.Renewal)
	for _, c := range []struct {
		job  int
		want []int
	}{
		{0, []int{1, 2, 3, 4, 5, 6, 23}},
		{1, []int{7, 14}},
		{3, []int{11, 19, 20}},
		{4001, []int{4002, 4003, 4004, 4005, 4006, 4007}},
		{4002, []int{4008, 4015}},
		{4004, []int{4012, 4020, 4021}},
		{7, nil},
		{23, nil},
		{4008, nil},
	} {
		if got := NextJobs(g.tables, c.job); !slices.Equal(got, c.want) {
			t.Errorf("next jobs of %d: %v, want %v", c.job, got, c.want)
		}
	}
}

// TestChangeJob checks pc_jobchange's rules: job level 1, learned skills
// kept, only next jobs without force, and the second class's change level.
func TestChangeJob(t *testing.T) {
	z, p := jobZone(t, reference.Renewal, 0, 30, 10)
	p.char.Skills[skill.IDBasic] = 9
	if z.changeJob(p, 7, false) {
		t.Fatal("a Novice became a Knight without force")
	}
	if !z.changeJob(p, 1, false) {
		t.Fatal("a Novice did not become a Swordman")
	}
	if p.char.Job != 1 || p.progress.Job != 1 || p.progress.JobLevel != 1 || p.char.Skills[skill.IDBasic] != 9 {
		t.Fatalf("after the change: job %d, job level %d, skills %v", p.char.Job, p.progress.JobLevel, p.char.Skills)
	}
	if p.Look.Job != 1 {
		t.Errorf("look job %d", p.Look.Job)
	}
	if z.changeJob(p, 1, false) {
		t.Error("changed into the same job")
	}
	p.progress.JobLevel = 43
	if !z.changeJob(p, 7, false) {
		t.Fatal("a Swordman did not become a Knight")
	}
	if p.record.JobChangeLevel != 43 {
		t.Errorf("change level %d, want 43", p.record.JobChangeLevel)
	}
	if z.changeJob(p, 14, false) {
		t.Error("a Knight became a Crusader without force")
	}
	if !z.changeJob(p, 14, true) || p.char.Job != 14 {
		t.Error("force did not change the job")
	}
}

// TestChangeJobSex checks pc_mapid2jobid with the player's sex: a male
// who changes into a Dancer becomes a Bard.
func TestChangeJobSex(t *testing.T) {
	z, p := jobZone(t, reference.Renewal, 3, 50, 40)
	if !z.changeJob(p, 20, false) || p.char.Job != 19 {
		t.Fatalf("a male Archer into a Dancer became %d, want 19", p.char.Job)
	}
	z, p = jobZone(t, reference.Renewal, 3, 50, 40)
	p.char.Male = false
	if !z.changeJob(p, 19, false) || p.char.Job != 20 {
		t.Fatalf("a female Archer into a Bard became %d, want 20", p.char.Job)
	}
}

// TestRebirth checks the rebirth NPC's rules: a second class at 99/50
// becomes a High Novice at 1/1 with 100 status points and the platinum
// skills, and then stays in its line.
func TestRebirth(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p := jobZone(t, mode, 7, 99, 49)
			if z.rebirth(p) {
				t.Fatal("rebirth at job level 49")
			}
			p.progress.JobLevel = 50
			p.char.Stats.Str = 80
			p.char.Skills[skill.IDBasic] = 9
			p.char.Skills[skill.IDBash] = 10
			if !z.rebirth(p) {
				t.Fatal("rebirth refused")
			}
			if p.char.Job != 4001 || p.progress.BaseLevel != 1 || p.progress.JobLevel != 1 || p.progress.StatusPoints != 100 {
				t.Fatalf("job %d, levels %d/%d, status points %d", p.char.Job, p.progress.BaseLevel, p.progress.JobLevel, p.progress.StatusPoints)
			}
			if p.char.Stats.Str != 1 || p.char.Skills[skill.IDBash] != 0 || p.char.Skills[skill.IDBasic] != 0 {
				t.Errorf("stats and skills not reset: %+v %v", p.char.Stats, p.char.Skills)
			}
			if p.char.Skills[skill.IDFirstAid] != 1 || p.char.Skills[skill.IDTrickDead] != 1 {
				t.Errorf("no platinum skills: %v", p.char.Skills)
			}
			if p.record.RebirthJob != 7 {
				t.Errorf("rebirth job %d", p.record.RebirthJob)
			}
			if z.rebirth(p) {
				t.Error("a High Novice was reborn")
			}
			if z.changeJob(p, 4003, false) {
				t.Error("a reborn Knight became a High Mage")
			}
			points := p.progress.StatusPoints
			if !z.changeJob(p, 4002, false) {
				t.Fatal("a reborn Knight did not become a High Swordman")
			}
			if p.progress.StatusPoints != points {
				t.Errorf("status points %d, want %d: both jobs are transcendent", p.progress.StatusPoints, points)
			}
			if z.changeJob(p, 4015, false) {
				t.Error("a reborn Knight became a Paladin")
			}
			if !z.changeJob(p, 4008, false) {
				t.Error("a reborn Knight did not become a Lord Knight")
			}
		})
	}
}

// TestTranscendentStatusPoints checks pc_jobchange's
// transcendent_status_points: 52 more for a change into a transcendent
// job, and a stat reset when a change out of one finds them spent.
func TestTranscendentStatusPoints(t *testing.T) {
	z, p := jobZone(t, reference.Renewal, 1, 60, 30)
	p.progress.StatusPoints = 10
	if !z.changeJob(p, 4002, true) || p.progress.StatusPoints != 62 {
		t.Fatalf("status points %d, want 62", p.progress.StatusPoints)
	}
	p.progress.StatusPoints = 5
	p.char.Stats.Str = 50
	if !z.changeJob(p, 1, true) {
		t.Fatal("force refused")
	}
	if p.char.Stats.Str != 1 || p.progress.StatusPoints != z.Progress.StatusPoints(60) {
		t.Errorf("str %d, points %d, want a reset to %d", p.char.Stats.Str, p.progress.StatusPoints, z.Progress.StatusPoints(60))
	}
}

// TestResetSkills checks pc_resetskill: points come back, quest skills
// and a non-Novice's Basic Skill stay.
func TestResetSkills(t *testing.T) {
	z, p := jobZone(t, reference.Renewal, 7, 99, 50)
	p.char.Skills = map[int]int{skill.IDBasic: 9, skill.IDBash: 10, skill.IDMagnumBreak: 10, 1001: 1, skill.IDMovingRecovery: 1}
	p.progress.SkillPoints = 3
	z.resetSkills(p)
	if p.progress.SkillPoints != 23 {
		t.Errorf("skill points %d, want 23", p.progress.SkillPoints)
	}
	want := map[int]int{skill.IDBasic: 9, 1001: 1, skill.IDMovingRecovery: 1}
	if len(p.char.Skills) != len(want) {
		t.Fatalf("skills %v, want %v", p.char.Skills, want)
	}
	for id, lv := range want {
		if p.char.Skills[id] != lv {
			t.Errorf("skill %d at %d, want %d", id, p.char.Skills[id], lv)
		}
	}
	// A Novice's Basic Skill resets.
	z, p = jobZone(t, reference.Renewal, 0, 10, 10)
	p.char.Skills = map[int]int{skill.IDBasic: 9}
	z.resetSkills(p)
	if p.char.Skills[skill.IDBasic] != 0 || p.progress.SkillPoints != 9 {
		t.Errorf("Novice: skills %v, points %d", p.char.Skills, p.progress.SkillPoints)
	}
}

// TestResetSkillDependents checks player:reset_skill: a skill that
// another learned skill needs is not lowered without with_dependents,
// and with it both go and both refund.
func TestResetSkillDependents(t *testing.T) {
	z, p := jobZone(t, reference.Renewal, 1, 50, 40)
	p.char.Skills = map[int]int{skill.IDBasic: 9, skill.IDBash: 10, skill.IDMagnumBreak: 3}
	if z.resetSkill(p, skill.IDBash, 4, false) {
		t.Fatal("Bash went below Magnum Break's need of 5")
	}
	if p.char.Skills[skill.IDBash] != 10 || p.progress.SkillPoints != 0 {
		t.Fatal("a refused reset changed something")
	}
	if !z.resetSkill(p, skill.IDBash, 5, false) || p.char.Skills[skill.IDBash] != 5 || p.progress.SkillPoints != 5 {
		t.Fatalf("Bash to 5: level %d, points %d", p.char.Skills[skill.IDBash], p.progress.SkillPoints)
	}
	if !z.resetSkill(p, skill.IDBash, 0, true) {
		t.Fatal("with_dependents refused")
	}
	if p.char.Skills[skill.IDBash] != 0 || p.char.Skills[skill.IDMagnumBreak] != 0 || p.progress.SkillPoints != 13 {
		t.Errorf("skills %v, points %d, want both gone and 13 points", p.char.Skills, p.progress.SkillPoints)
	}
	// Quest skills and Basic Skill outside the Novice jobs stay.
	p.char.Skills[skill.IDMovingRecovery] = 1
	if z.resetSkill(p, skill.IDMovingRecovery, 0, false) || z.resetSkill(p, skill.IDBasic, 0, true) {
		t.Error("a quest skill or Basic Skill was lowered")
	}
}

// TestResetStat checks the refund of player:reset_stat against rAthena's
// PC_STATUS_POINT_COST in both modes.
func TestResetStat(t *testing.T) {
	// Raising from 1 to 50 costs the sum of the costs of 1 to 49.
	cost := func(renewal bool, low int) int {
		if !renewal {
			return 1 + (low+9)/10
		}
		return 2 + (low-1)/10
	}
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z, p := jobZone(t, mode, 1, 99, 50)
			p.char.Stats.Str = 50
			p.progress.StatusPoints = 0
			want := 0
			for low := 10; low < 50; low++ {
				want += cost(mode == reference.Renewal, low)
			}
			if !z.resetStat(p, status.Str, 10) || p.char.Stats.Str != 10 || p.progress.StatusPoints != want {
				t.Fatalf("str %d, points %d, want 10 and %d", p.char.Stats.Str, p.progress.StatusPoints, want)
			}
			if z.resetStat(p, status.Str, 10) || z.resetStat(p, status.Str, 0) {
				t.Error("a reset that lowers nothing passed")
			}
		})
	}
}

// TestResetStats checks pc_resetstate: the table's points at the base
// level, with 52 more for a transcendent job.
func TestResetStats(t *testing.T) {
	z, p := jobZone(t, reference.Renewal, 4008, 99, 50)
	p.char.Stats = status.Stats{Str: 99, Agi: 50, Vit: 30, Int: 1, Dex: 40, Luk: 1}
	z.resetStats(p)
	if p.char.Stats != (status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}) {
		t.Errorf("stats %+v", p.char.Stats)
	}
	if want := z.Progress.StatusPoints(99) + 52; p.progress.StatusPoints != want {
		t.Errorf("status points %d, want %d", p.progress.StatusPoints, want)
	}
}

// TestJobChangeSkillTree checks that the skill list follows the job, and
// that the saved change level decides when the second class's tree
// shows, as pc_calc_skilltree_normalize_job.
func TestJobChangeSkillTree(t *testing.T) {
	z, p := jobZone(t, reference.Renewal, 1, 60, 40)
	p.char.Skills = map[int]int{skill.IDBasic: 9}
	for _, id := range []int{skill.IDBash, skill.IDSwordMastery, skill.IDRecovery} {
		p.char.Skills[id] = 10
	}
	p.char.Skills[skill.IDProvoke] = 9 // 39 first class points.
	if !z.changeJob(p, 7, false) {
		t.Fatal("job change refused")
	}
	tree := func() bool {
		entries, err := z.SkillTrees.List(z.Skills, z.Status, z.learner(p), z.SkillSettings)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.Skill == 55 { // KN_SPEARMASTERY.
				return true
			}
		}
		return false
	}
	// Changed at job level 40: 39 points open the Knight tree.
	if !tree() {
		t.Error("the Knight tree is hidden after spending the 39 points of job level 40")
	}
	p.record.JobChangeLevel = 50
	if tree() {
		t.Error("the Knight tree shows before 49 points at a change level of 50")
	}
}

// withModJob adds a mod job to a zone's job tables: a second job of the
// Gunslinger line that shows the Gunslinger's sprites, and that a
// Gunslinger at job level 40 may change into.
func withModJob(t *testing.T, z *Zone) int {
	t.Helper()
	const id = 10_000
	var d status.Data
	if err := reference.LoadFile("extracts/renewal/job_stats.json", &d); err != nil {
		t.Fatal(err)
	}
	jobs := status.CoreJobData(&d)
	for _, j := range jobs {
		if j.ID == 24 {
			j.ID, j.MapID = id, 0x108
			j.Info = status.JobInfo{Name: "example.a:Desperado", DisplayName: "Desperado", Mod: "example.a", Look: 24, EquipsAs: 24, BaseJob: id,
				Tier: status.TierSecond, ChangesFrom: []status.JobChange{{Job: 24, JobLevel: 40}}}
			jobs = append(jobs, j)
			break
		}
	}
	z.Status = z.Status.WithJobs(jobs)
	return id
}

// TestChangeIntoModJob checks a change into a mod job: changes_from and
// its levels, the look job's sprites, and the job's name for the client.
func TestChangeIntoModJob(t *testing.T) {
	z, p := jobZone(t, reference.Renewal, 24, 60, 39)
	id := withModJob(t, z)
	if !slices.Equal(NextJobs(z.Status, 24), []int{id}) {
		t.Errorf("next jobs of the Gunslinger: %v", NextJobs(z.Status, 24))
	}
	if z.changeJob(p, id, false) {
		t.Fatal("a Gunslinger at job level 39 changed into the mod job")
	}
	p.progress.JobLevel = 40
	if !z.changeJob(p, id, false) || p.char.Job != id || p.progress.JobLevel != 1 {
		t.Fatalf("job %d, job level %d", p.char.Job, p.progress.JobLevel)
	}
	if p.Look.Job != 24 {
		t.Errorf("the mod job shows sprites of job %d", p.Look.Job)
	}
	z.stepAt(z.now + 50)
	var name string
	for _, e := range events(p) {
		if n := e.GetJobName(); n != nil && n.GetJob() == uint32(id) {
			name = n.GetName()
		}
	}
	if name != "Desperado" {
		t.Errorf("job name %q", name)
	}
	// Back to a core job only with force.
	if z.changeJob(p, 24, false) || !z.changeJob(p, 24, true) || p.char.Job != 24 {
		t.Errorf("job %d after the change back", p.char.Job)
	}
}
