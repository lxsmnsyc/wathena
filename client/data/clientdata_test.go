package clientdata

import "testing"

func TestLoadJobs(t *testing.T) {
	j, err := LoadJobs()
	if err != nil {
		t.Fatal(err)
	}
	if len(j.Jobs) < 180 || len(j.Looks) < 160 {
		t.Errorf("%d jobs and %d looks", len(j.Jobs), len(j.Looks))
	}
	byID := map[int]Job{}
	for _, job := range j.Jobs {
		byID[job.ID] = job
	}
	if byID[1].Look != "swordman" || byID[1].Mount == "" {
		t.Errorf("swordman is %+v", byID[1])
	}
}

func TestParseJobsRejects(t *testing.T) {
	for name, y := range map[string]string{
		"missing look": "looks: []\njobs:\n  - {id: 1, constant: A, look: x}\n",
		"bad race":     "looks:\n  - {look: x, grf_sprite: y, race: elf}\njobs: []\n",
		"bad name":     "looks:\n  - {look: X, grf_sprite: y, race: human}\njobs: []\n",
	} {
		if _, err := ParseJobs([]byte(y)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestSexes(t *testing.T) {
	j, err := LoadJobs()
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range j.Looks {
		switch l.Look {
		case "bard":
			if s := l.Sexes(); len(s) != 1 || s[0] != "m" {
				t.Errorf("bard sexes %v", s)
			}
		case "novice":
			if s := l.Sexes(); len(s) != 2 {
				t.Errorf("novice sexes %v", s)
			}
		}
	}
	if _, err := ParseJobs([]byte("looks:\n  - look: x\n    grf_sprite: x\n    race: human\n    sex: q\n")); err == nil {
		t.Error("parsed sex q")
	}
}
