package status

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/rules"
)

// Raised stats of the recorder's PC_SAMPLE_BUILDS.
var builds = map[string]Stats{
	"all_1":    {1, 1, 1, 1, 1, 1},
	"melee":    {90, 50, 30, 1, 40, 1},
	"critical": {70, 90, 1, 1, 40, 50},
	"magic":    {1, 1, 30, 99, 80, 1},
	"ranged":   {30, 40, 1, 1, 99, 30},
	"tank":     {60, 1, 99, 1, 30, 1},
}

func load(t *testing.T, mode reference.Mode) *Tables {
	t.Helper()
	var d Data
	if err := reference.LoadFile("extracts/"+string(mode)+"/job_stats.json", &d); err != nil {
		t.Fatal(err)
	}
	tb, err := New(&d)
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

// TestRecordedPlayers checks every player in pc_status.json. The recorder
// makes male characters with no equipment.
func TestRecordedPlayers(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			checkRecordedPlayers(t, mode, nil)
		})
	}
}

// checkRecordedPlayers checks every player of a mode's pc_status.json,
// with the tables' rules set to rl when it is not nil.
func checkRecordedPlayers(t *testing.T, mode reference.Mode, rl *rules.Rules) {
	{
		{
			tb := load(t, mode)
			if rl != nil {
				tb.SetRules(rl)
			}
			var pcs struct {
				Players []struct {
					Job       int             `json:"job"`
					BaseLevel int             `json:"base_level"`
					JobLevel  int             `json:"job_level"`
					Build     string          `json:"build"`
					AllSkills bool            `json:"all_skills"`
					MaxWeight int             `json:"max_weight"`
					Skills    [][2]int        `json:"skills"`
					Base      json.RawMessage `json:"base_status"`
					Battle    json.RawMessage `json:"battle_status"`
				} `json:"players"`
			}
			if err := reference.Load(mode, "pc_status", &pcs); err != nil {
				t.Fatal(err)
			}
			matched := 0
			for _, rec := range pcs.Players {
				name := fmt.Sprintf("job %d lv %d/%d %s skills=%v", rec.Job, rec.BaseLevel, rec.JobLevel, rec.Build, rec.AllSkills)
				stats, ok := builds[rec.Build]
				if !ok {
					t.Fatalf("%s: unknown build", name)
				}
				skills := map[int]int{}
				for _, s := range rec.Skills {
					skills[s[0]] = s[1]
				}
				res, err := tb.Calc(Character{Job: rec.Job, Male: true, BaseLevel: rec.BaseLevel, JobLevel: rec.JobLevel, Stats: stats, Skills: skills})
				if err != nil {
					t.Errorf("%s: %v", name, err)
					continue
				}
				diffs := diff("base", rec.Base, res.Base)
				diffs = append(diffs, diff("battle", rec.Battle, res.Battle)...)
				if res.MaxWeight != rec.MaxWeight {
					diffs = append(diffs, fmt.Sprintf("max_weight %d, want %d", res.MaxWeight, rec.MaxWeight))
				}
				if len(diffs) > 0 {
					t.Errorf("%s: %v", name, diffs)
					continue
				}
				matched++
			}
			t.Logf("%s: %d of %d players match", mode, matched, len(pcs.Players))
		}
	}
}

// diff compares every field the reference recorded.
func diff(what string, want json.RawMessage, got any) []string {
	var w, g map[string]any
	dec := json.NewDecoder(bytes.NewReader(want))
	dec.UseNumber()
	if err := dec.Decode(&w); err != nil {
		return []string{err.Error()}
	}
	b, err := json.Marshal(got)
	if err != nil {
		return []string{err.Error()}
	}
	dec = json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&g); err != nil {
		return []string{err.Error()}
	}
	var out []string
	var walk func(prefix string, w, g map[string]any)
	walk = func(prefix string, w, g map[string]any) {
		for k, wv := range w {
			gv, ok := g[k]
			if wm, isMap := wv.(map[string]any); isMap {
				gm, _ := gv.(map[string]any)
				walk(prefix+k+".", wm, gm)
				continue
			}
			if !ok || fmt.Sprint(gv) != fmt.Sprint(wv) {
				out = append(out, fmt.Sprintf("%s.%s%s %v, want %v", what, prefix, k, gv, wv))
			}
		}
	}
	walk("", w, g)
	return out
}

func TestCalcErrors(t *testing.T) {
	tb := load(t, reference.Renewal)
	for _, c := range []Character{
		{Job: 99999, BaseLevel: 1, JobLevel: 1},
		{Job: 0, BaseLevel: 1, JobLevel: 11},
		{Job: 0, BaseLevel: 0, JobLevel: 1},
		{Job: 0, BaseLevel: 276, JobLevel: 1},
		{Job: 4218, BaseLevel: 1, JobLevel: 1},
	} {
		if _, err := tb.Calc(c); err == nil {
			t.Errorf("%+v: no error", c)
		}
	}
}
