package mob

import (
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// testNames resolves a few names for the parse tests.
type testNames struct{}

func (testNames) SkillID(name string) (int, bool) {
	id, ok := map[string]int{"NPC_SUMMONSLAVE": 196, "AL_HEAL": 28, "MG_FIREBOLT": 19, "WZ_METEOR": 83, "NPC_EMOTION": 197}[name]
	return id, ok
}
func (testNames) SkillName(id int) (string, bool) {
	s, ok := map[int]string{196: "NPC_SUMMONSLAVE", 28: "AL_HEAL", 19: "MG_FIREBOLT", 83: "WZ_METEOR", 197: "NPC_EMOTION"}[id]
	return s, ok
}
func (testNames) Ground(id int) bool { return id == 83 }
func (testNames) MonsterID(name string) (uint32, bool) {
	id, ok := map[string]uint32{"PORING": 1002, "DROPS": 1113}[name]
	return id, ok
}
func (testNames) MonsterName(id uint32) (string, bool) {
	s, ok := map[uint32]string{1002: "PORING", 1113: "DROPS"}[id]
	return s, ok
}
func (testNames) StatusID(name string) (int, bool) {
	id, ok := map[string]int{"stone": 0, "silence": 6}[name]
	return id, ok
}
func (testNames) StatusName(id int) (string, bool) {
	s, ok := map[int]string{0: "stone", 6: "silence"}[id]
	return s, ok
}
func (testNames) EmotionID(name string) (int, bool) {
	id, ok := map[string]int{"surprise": 0, "heh": 7}[name]
	return id, ok
}
func (testNames) EmotionName(id int) (string, bool) {
	s, ok := map[int]string{0: "surprise", 7: "heh"}[id]
	return s, ok
}

// TestSkillRoundTrip writes entries as YAML and reads them back.
func TestSkillRoundTrip(t *testing.T) {
	skills := []Skill{
		{ID: 196, Level: 1, State: StateIdle, Rate: 1000, CastTime: 700, Delay: 10000, Target: TargetSelf,
			Condition: CondSlavesAtMost, Value: 3, Summons: []uint32{1002, 1113}, Emotion: -1},
		{ID: 28, Level: 9, State: StateAny, Rate: 10000, Cancelable: true, Target: TargetFriend,
			Condition: CondFriendHPBetween, Value: 10, Value2: 60, Emotion: 7, Chat: `Heal "me"!`},
		{ID: 83, Level: 5, State: StateBerserk, Rate: 500, Target: TargetAround6, Condition: CondHasStatus, Value: AnyBadStatus, Emotion: -1},
		{ID: 19, Level: 3, State: StateRush, Rate: 2000, Condition: CondHitBySkill, Value: 28, Emotion: -1},
	}
	for _, s := range skills {
		line, err := s.YAML(testNames{})
		if err != nil {
			t.Fatal(err)
		}
		m := parseFlow(t, line)
		got, err := ParseSkill(m, testNames{})
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		if g, w := strings.TrimSpace(mustYAML(t, got)), line; g != w {
			t.Errorf("round trip:\n got %s\nwant %s", g, w)
		}
	}
}

func mustYAML(t *testing.T, s Skill) string {
	t.Helper()
	y, err := s.YAML(testNames{})
	if err != nil {
		t.Fatal(err)
	}
	return y
}

func TestParseSkillErrors(t *testing.T) {
	for _, c := range []struct {
		m    map[string]any
		want string
	}{
		{map[string]any{"skill": "NOPE"}, `there is no skill "NOPE"`},
		{map[string]any{"skill": "AL_HEAL", "state": "berserk"}, `state: "berserk"`},
		{map[string]any{"skill": "AL_HEAL", "target": "around1"}, "around1 is for ground skills"},
		{map[string]any{"skill": "AL_HEAL", "condition": "hp_below"}, "hp_below needs a number"},
		{map[string]any{"skill": "AL_HEAL", "condition": "always", "condition_value": 5}, "always takes none"},
		{map[string]any{"skill": "AL_HEAL", "condition": "hp_between", "condition_value": []any{60, 10}}, "[lowest, highest]"},
		{map[string]any{"skill": "AL_HEAL", "summons": []any{"PORING"}}, "only NPC_SUMMONSLAVE"},
		{map[string]any{"skill": "AL_HEAL", "rate": 0}, "rate must be"},
		{map[string]any{"skill": "AL_HEAL", "colour": "red"}, `"colour" is not a field`},
	} {
		_, err := ParseSkill(c.m, testNames{})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%v: %v, want %q", c.m, err, c.want)
		}
	}
}

// TestStateMatches checks the state test of mobskill_use.
func TestStateMatches(t *testing.T) {
	if !StateMatches(StateAny, StateBerserk, false) || StateMatches(StateAny, StateDead, false) {
		t.Error("any")
	}
	if !StateMatches(StateAnyTarget, StateRush, true) || StateMatches(StateAnyTarget, StateRush, false) || StateMatches(StateAnyTarget, StateLoot, true) {
		t.Error("anytarget")
	}
	if !StateMatches(StateDead, StateDead, false) || StateMatches(StateIdle, StateWalk, true) {
		t.Error("exact")
	}
}

// TestDelayEntries checks mobskill_delay: the first entry of the skill
// whose state matches gives the delay.
func TestDelayEntries(t *testing.T) {
	ms := []Skill{
		{ID: 28, State: StateIdle, Delay: 1000},
		{ID: 19, State: StateBerserk, Delay: 2000},
		{ID: 28, State: StateBerserk, Delay: 3000},
		{ID: 28, State: StateAny, Delay: 4000},
	}
	if d := DelayEntries(ms, 0, StateBerserk, true); d != 3000 {
		t.Errorf("delay %d, want 3000", d)
	}
	if d := DelayEntries(ms, 2, StateWalk, false); d != 4000 {
		t.Errorf("delay %d, want 4000", d)
	}
}

// TestFromRawEveryRow converts every mob_skill_db row of both modes.
func TestFromRawEveryRow(t *testing.T) {
	for _, mode := range reference.Modes {
		d, err := Load(mode)
		if err != nil {
			t.Fatal(err)
		}
		ids := map[uint32]bool{}
		for _, m := range d.Monsters {
			ids[m.ID] = true
		}
		rows, lost := 0, 0
		for _, m := range d.Monsters {
			for _, r := range m.Skills {
				s, l, err := FromRaw(r, func(id int) bool { return id == 83 }, func(id uint32) bool { return ids[id] }, d.ChatText)
				if err != nil {
					t.Fatalf("%s %d: %v", mode, m.ID, err)
				}
				if s.Condition.Event() && r.Condition != "closedattacked" && r.Condition != "longrangeattacked" &&
					r.Condition != "skillused" && r.Condition != "casttargeted" && r.Condition != "onspawn" &&
					r.Condition != "groundattacked" && r.Condition != "damagedgt" {
					t.Errorf("%s: %s is not an event", r.Condition, s.Condition)
				}
				rows++
				if len(l) > 0 {
					lost++
				}
			}
		}
		t.Logf("%s: %d rows, %d lose a part", mode, rows, lost)
	}
}
