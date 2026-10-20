package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// TestSkillCommand checks the conversion of rAthena's skill and
// addtoskill to player:give_skill (review decision 57).
func TestSkillCommand(t *testing.T) {
	cases := []struct{ src, want string }{
		{`skill "SM_MOVINGRECOVERY",1,SKILL_PERM;`, `player:give_skill(SM_MOVINGRECOVERY, 1, "permanent")`},
		{`skill "NV_TRICKDEAD",0,SKILL_PERM;`, `player:give_skill(NV_TRICKDEAD, 0, "permanent")`},
		{`skill 19,3;`, `player:give_skill(MG_FIREBOLT, 3, "temporary")`},
		{`skill "AL_HEAL",2,SKILL_TEMPLEVEL;`, `player:give_skill(AL_HEAL, 2, "temporary_add")`},
		{`addtoskill "AL_HEAL",2;`, `player:give_skill(AL_HEAL, 2, "temporary_add")`},
		{`skill "AL_HEAL",BaseLevel/10,SKILL_TEMP;`, `player:give_skill(AL_HEAL, integers.divide_truncated(player.base_level, 10), "temporary")`},
		{`skill "AL_HEAL",1,SKILL_PERM_GRANT;`, `player:give_skill(AL_HEAL, 1, "permanent_grant")`},
		{`skill .@s,1;`, "skill: a computed skill"},
		{`skill "AL_HEAL",1,.@f;`, "skill: a computed flag"},
	}
	read := func(name string) []byte {
		b, err := reference.ReadFile("extracts/renewal/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var lines []npcLine
	for i, c := range cases {
		name := fmt.Sprintf("Skill %d#s", i)
		lines = append(lines, npcLine{Kind: "script", Name: name, Display: name, File: "test.txt", Line: 1, Map: "prontera",
			X: 150, Y: 150, Sprite: "4_M_JOB_KNIGHT1", XS: -1, YS: -1, Script: "{\n\t" + c.src + "\n\tend;\n}"})
	}
	data, err := json.Marshal(map[string]any{"mode": "renewal", "npcs": lines})
	if err != nil {
		t.Fatal(err)
	}
	res, err := ConvertNPCs(NPCInput{NPCs: bytes.NewReader(data), Constants: bytes.NewReader(read("script_constants")),
		ClientTables: bytes.NewReader(read("client_tables"))})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range res.NPCs {
		var i int
		if _, err := fmt.Sscanf(n.Source, "Skill %d#s", &i); err != nil {
			t.Fatal(err)
		}
		c := cases[i]
		if strings.HasPrefix(c.want, "skill: ") {
			if got := strings.Join(n.Unsupported, "; "); got != c.want {
				t.Errorf("%s: unsupported %q, want %q", c.src, got, c.want)
			}
			continue
		}
		if len(n.Unsupported) > 0 || !strings.Contains(n.Lua, c.want) {
			t.Errorf("%s: %v\n%s", c.src, n.Unsupported, n.Lua)
		}
	}
}

// jobChangeHost is npcHost with a player's job, learned skills and job
// changes, for a converted job change NPC.
type jobChangeHost struct {
	*npcHost
	learned map[int]int
	changes []int
}

func (h *jobChangeHost) SkillLevel(_ int64, skill int64) (int, error) {
	return h.learned[int(skill)], nil
}
func (h *jobChangeHost) NextJobs(int) []int { return nil }
func (h *jobChangeHost) ChangeJob(_ int64, job int, force bool) (bool, error) {
	if !force {
		return false, fmt.Errorf("a change to %d without force", job)
	}
	h.job, h.jobLevel = job, 1
	h.changes = append(h.changes, job)
	return true, nil
}
func (h *jobChangeHost) Rebirth(int64) (bool, error)                    { return false, nil }
func (h *jobChangeHost) ResetSkills(int64) error                        { return nil }
func (h *jobChangeHost) ResetStats(int64) error                         { return nil }
func (h *jobChangeHost) ResetSkill(int64, int, int, bool) (bool, error) { return false, nil }
func (h *jobChangeHost) ResetStat(int64, int, int) (bool, error)        { return false, nil }
func (h *jobChangeHost) GiveSkill(_ int64, skill, level int, kind modscript.SkillKind) (bool, error) {
	if kind != modscript.SkillPermanent {
		return false, fmt.Errorf("skill %d: kind %d", skill, kind)
	}
	if level == 0 {
		delete(h.learned, skill)
	} else {
		h.learned[skill] = level
	}
	return true, nil
}

// TestConvertedFirstJobChange runs the renewal Swordman job change of
// npc/re/jobs/1-1/swordman.txt in Izlude, which the skill command placed
// (review decision 57): a Novice at job level 10 with Basic Skill 9 asks
// to become a Swordman, and a High Novice who was a Lord Knight gets the
// Swordman's quest skills with the change.
func TestConvertedFirstJobChange(t *testing.T) {
	_, base, core := testRuntime(t, reference.Renewal)
	i := slices.IndexFunc(core.NPCs.NPCs, func(n *NPC) bool { return n.Source == "Swordman#swd" })
	if i < 0 {
		t.Fatal("no Swordman#swd")
	}
	n := core.NPCs.NPCs[i]
	if !n.Placed || n.Map != "izlude_in" {
		t.Fatalf("Swordman#swd: placed %t on %q, %v", n.Placed, n.Map, n.Unsupported)
	}
	id := func(name string) int {
		v, ok := core.Constants.Skill(name)
		if !ok {
			t.Fatalf("no skill %s", name)
		}
		return int(v)
	}
	job := func(name string) int {
		v, _, ok := core.Constants.Lookup(name)
		if !ok {
			t.Fatalf("no constant %s", name)
		}
		return int(v)
	}
	for _, c := range []struct {
		name    string
		job     int
		advjob  string
		answers []int64
		want    int
		skills  map[int]int
	}{
		{"novice", job("Job_Novice"), "", []int64{2, 1}, job("Job_Swordman"),
			map[int]int{id("NV_BASIC"): 9, id("NV_TRICKDEAD"): 1}},
		{"high novice", job("Job_Novice_High"), fmt.Sprint(job("Job_Lord_Knight")), nil, job("Job_Swordman_High"),
			map[int]int{id("NV_BASIC"): 9, id("SM_MOVINGRECOVERY"): 1, id("SM_FATALBLOW"): 1, id("SM_AUTOBERSERK"): 1}},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := *base
			b.job, b.jobLevel = c.job, 10
			h := &jobChangeHost{npcHost: &npcHost{testHost: &b, npcs: map[int64]*NPC{}, steps: map[int64]modscript.Step{}, ended: map[int64]bool{},
				vars: map[modscript.VariableKey]string{}, zeny: 0, market: marketHost{shops: coreShops(core)}},
				learned: map[int]int{id("NV_BASIC"): 9, id("NV_TRICKDEAD"): 1}}
			if c.advjob != "" {
				h.vars[modscript.VariableKey{Mod: "core", Name: "advjob", Scope: "character"}] = c.advjob
			}
			logs := &logRecorder{}
			rt, err := modscript.New(modscript.Options{Host: h, Mods: []string{"core"}, Constants: core.Constants, Jobs: h.jobs,
				Log: slog.New(logs), Server: variables.NewServer(nil, nil), Quests: coreQuests(t, core),
				Clock: func() time.Time { return testClock }, Location: time.UTC}, coreScripts(t, core))
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Close()
			h.npcs[1] = n
			npc, err := rt.StartNPC(1, "core:"+n.Script)
			if err != nil {
				t.Fatal(err)
			}
			conv, err := rt.Click(npc, 1)
			if err != nil {
				t.Fatal(err)
			}
			answers := c.answers
			for range 50 {
				if conv == nil || conv.Done {
					break
				}
				var answer script.Value
				if s := h.steps[conv.ID]; s.Kind == modscript.StepMenu {
					if len(answers) == 0 {
						t.Fatalf("an unexpected menu: %q", s.Options)
					}
					answer, answers = script.Int(answers[0]), answers[1:]
				}
				rt.ResumeConversation(conv, answer)
			}
			if conv != nil && !conv.Done {
				t.Fatal("the conversation did not end")
			}
			for _, m := range logs.msgs {
				t.Error(m)
			}
			if !slices.Equal(h.changes, []int{c.want}) {
				t.Errorf("job changes %v, want [%d]", h.changes, c.want)
			}
			if fmt.Sprint(h.learned) != fmt.Sprint(c.skills) {
				t.Errorf("skills %v, want %v", h.learned, c.skills)
			}
		})
	}
}
