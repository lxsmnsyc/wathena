package progression

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

func TestExpTables(t *testing.T) {
	for _, mode := range reference.Modes {
		d, _ := load(t, mode)
		// Each base or job table covers its levels, from 1 up.
		for _, g := range d.Exp {
			if g.MaxBaseLevel != nil && len(g.BaseExp) != *g.MaxBaseLevel {
				t.Errorf("%s %v: %d base exp rows for max level %d", mode, g.Jobs, len(g.BaseExp), *g.MaxBaseLevel)
			}
			if g.MaxJobLevel != nil && len(g.JobExp) != *g.MaxJobLevel {
				t.Errorf("%s %v: %d job exp rows for max level %d", mode, g.Jobs, len(g.JobExp), *g.MaxJobLevel)
			}
		}
	}

	re := tables(t, reference.Renewal)
	pre := tables(t, reference.PreRenewal)
	novice := re.NewCharacter(0)
	if novice.BaseLevel != 1 || novice.JobLevel != 1 || novice.StatusPoints != 48 || novice.SkillPoints != 0 {
		t.Errorf("new novice %+v", novice)
	}
	for _, c := range []struct {
		name      string
		got, want int64
	}{
		{"renewal novice base 1", re.NextBaseExp(&novice), 548},
		{"renewal novice job 1", re.NextJobExp(&novice), 10},
		{"pre-renewal novice base 1", pre.NextBaseExp(&novice), 9},
		{"renewal novice base 98", re.BaseExpAt(0, 98), 1252761},
		{"novice max base", int64(re.MaxBaseLevel(0)), 99},
		{"novice max job", int64(re.MaxJobLevel(0)), 10},
		{"swordman max job", int64(re.MaxJobLevel(1)), 50},
		{"unknown job", int64(re.MaxBaseLevel(99999)), 0},
		{"at max base", re.NextBaseExp(&Character{BaseLevel: 99, JobLevel: 1}), MaxLevelBaseExp},
		{"at max job", re.NextJobExp(&Character{BaseLevel: 1, JobLevel: 10}), MaxLevelJobExp},
		{"points at 1", int64(re.StatusPoints(1)), 48},
		{"points at 100", int64(re.StatusPoints(100)), 1295},
		{"points 1 to 2", int64(re.StatusPointGain(1)), 3},
		{"points 5 to 6", int64(re.StatusPointGain(5)), 4},
		{"points past the table", int64(re.StatusPointGain(MaxLevel)), 0},
	} {
		if c.got != c.want {
			t.Errorf("%s: %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestStatPointFormula(t *testing.T) {
	// Levels missing from statpoint.yml follow the loader's formula.
	tb := New(&Data{}, nil, DefaultConfig)
	for _, c := range [][2]int{{1, 48}, {2, 51}, {5, 60}, {6, 64}, {201, tbPoints(200)}} {
		if got := tb.StatusPoints(c[0]); got != c[1] {
			t.Errorf("level %d: %d, want %d", c[0], got, c[1])
		}
	}
}

// tbPoints is the formula's total at a level up to 200.
func tbPoints(level int) int {
	p := 48
	for l := 2; l <= level; l++ {
		p += (l - 1 + 15) / 5
	}
	return p
}

func TestLevelUpCarriesOver(t *testing.T) {
	tb := tables(t, reference.Renewal)
	c := tb.NewCharacter(0)
	g := tb.GainExp(&c, 548+100, 10+5)
	if g.BaseLevels != 1 || c.BaseLevel != 2 || c.BaseExp != 100 || c.StatusPoints != 51 {
		t.Errorf("base: %+v, %+v", g, c)
	}
	if g.JobLevels != 1 || c.JobLevel != 2 || c.JobExp != 5 || c.SkillPoints != 1 {
		t.Errorf("job: %+v, %+v", g, c)
	}

	// Without multi_level_up, one kill gives one level, and the exp left
	// is at most the finished level's exp minus 1.
	c = tb.NewCharacter(0)
	g = tb.GainExp(&c, 100000, 1000)
	if g.BaseLevels != 1 || c.BaseLevel != 2 || c.BaseExp != 547 {
		t.Errorf("base: %+v, %+v", g, c)
	}
	if g.JobLevels != 1 || c.JobLevel != 2 || c.JobExp != 9 {
		t.Errorf("job: %+v, %+v", g, c)
	}

	// With it, levels keep coming.
	multi := DefaultConfig
	multi.MultiLevelUp = true
	tm := New(mustData(t), nil, multi)
	c = tm.NewCharacter(0)
	g = tm.GainExp(&c, 548+894+10, 0)
	if g.BaseLevels != 2 || c.BaseLevel != 3 || c.BaseExp != 10 || c.StatusPoints != 54 {
		t.Errorf("multi: %+v, %+v", g, c)
	}
}

func mustData(t *testing.T) *Data {
	t.Helper()
	d, _ := load(t, reference.Renewal)
	return d
}

func TestMaxLevel(t *testing.T) {
	tb := tables(t, reference.Renewal)
	// Reaching the maximum job level keeps the exp left, up to the cap.
	c := Character{Job: 0, BaseLevel: 1, JobLevel: 9}
	g := tb.GainExp(&c, 0, 340+50)
	if g.JobLevels != 1 || c.JobLevel != 10 || c.JobExp != 50 || c.SkillPoints != 1 {
		t.Errorf("to max job: %+v, %+v", g, c)
	}
	// At the maximum, exp still adds up to MAX_LEVEL_JOB_EXP.
	g = tb.GainExp(&c, 0, 100)
	if g.JobExp != 100 || c.JobExp != 150 || c.JobLevel != 10 {
		t.Errorf("at max job: %+v, %+v", g, c)
	}
	c.JobExp = MaxLevelJobExp - 10
	if g = tb.GainExp(&c, 0, 100); g.JobExp != 10 || c.JobExp != MaxLevelJobExp {
		t.Errorf("job cap: %+v, %+v", g, c)
	}
	if g = tb.GainExp(&c, 0, 100); g.JobExp != 0 || c.JobExp != MaxLevelJobExp {
		t.Errorf("job full: %+v, %+v", g, c)
	}

	c = Character{Job: 0, BaseLevel: 98, JobLevel: 1}
	tb.GainExp(&c, 1252761*3, 0)
	if c.BaseLevel != 99 || c.BaseExp != 1252761-1 {
		t.Errorf("to max base: %+v", c)
	}
	c.BaseExp = MaxLevelBaseExp - 1
	if g = tb.GainExp(&c, MaxExp, 0); g.BaseExp != 1 || c.BaseExp != MaxLevelBaseExp {
		t.Errorf("base cap: %+v, %+v", g, c)
	}
}

func TestMaxExpGainRate(t *testing.T) {
	cfg := DefaultConfig
	cfg.MaxExpGainRate = 500
	tb := New(mustData(t), nil, cfg)
	c := tb.NewCharacter(0)
	// At most half of the 548 for level 1.
	if g := tb.GainExp(&c, 1000, 0); g.BaseExp != 274 || c.BaseExp != 274 || c.BaseLevel != 1 {
		t.Errorf("%+v, %+v", g, c)
	}
}

func TestMobExp(t *testing.T) {
	if got := MobExp(169, 100); got != 169 {
		t.Error(got)
	}
	if got := MobExp(169, 250); got != 422 {
		t.Error(got)
	}
}
