package mob

import (
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// testNames resolves a few names for the parse tests.
type testNames struct{}

func (testNames) SkillID(name string) (int, bool) {
	id, ok := map[string]int{"NPC_SUMMONSLAVE": 196, "AL_HEAL": 28, "MG_FIREBOLT": 19, "WZ_METEOR": 83, "NPC_EMOTION": 197,
		"NPC_EMOTION_ON": 474}[name]
	return id, ok
}
func (testNames) SkillName(id int) (string, bool) {
	s, ok := map[int]string{196: "NPC_SUMMONSLAVE", 28: "AL_HEAL", 19: "MG_FIREBOLT", 83: "WZ_METEOR", 197: "NPC_EMOTION",
		474: "NPC_EMOTION_ON"}[id]
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
			Condition: CondSlavesAtMost, Value: 3, Summons: []uint32{1002, 1113}},
		{ID: 28, Level: 9, State: StateAny, Rate: 10000, Cancelable: true, Target: TargetFriend,
			Condition: CondFriendHPBetween, Value: 10, Value2: 60, Emotions: []int{7}, Chat: `Heal "me"!`},
		{ID: 83, Level: 5, State: StateBerserk, Rate: 500, Target: TargetAround6, Condition: CondHasStatus, Value: AnyBadStatus},
		{ID: 19, Level: 3, State: StateRush, Rate: 2000, Condition: CondHitBySkill, Value: 28, Chat: "Burn!", ChatColor: 0xFF8000},
		{ID: IDEmotion, Level: 1, State: StateBerserk, Rate: 10000, Target: TargetSelf, Emotions: []int{0, 7},
			HasModes: true, Modes: ModeCanMove | ModeAggressive | ModeCanAttack},
		{ID: IDEmotion, Level: 1, State: StateIdle, Rate: 10000, Target: TargetSelf, Emotions: []int{7}, HasModes: true},
		{ID: IDEmotionOn, Level: 1, State: StateIdle, Rate: 10000, Target: TargetSelf, HasModes: true, Modes: ModeAngry},
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
		{map[string]any{"skill": "AL_HEAL", "emotion": []any{"heh"}}, "a list of two"},
		{map[string]any{"skill": "AL_HEAL", "emotion": []any{"heh", "nope"}}, `"nope" is not in wa.emotions`},
		{map[string]any{"skill": "AL_HEAL", "modes": []any{"aggressive"}}, "only NPC_EMOTION and NPC_EMOTION_ON"},
		{map[string]any{"skill": "NPC_EMOTION", "modes": []any{"angrry"}}, `"angrry" is not a mode`},
		{map[string]any{"skill": "NPC_EMOTION", "modes": "aggressive"}, "modes must be a list"},
		{map[string]any{"skill": "NPC_EMOTION_ON", "modes": []any{}}, "needs a mode to add"},
		{map[string]any{"skill": "AL_HEAL", "chat_color": 0xFF8000}, "chat_color needs chat"},
		{map[string]any{"skill": "AL_HEAL", "chat": "Hi", "chat_color": 0x1000000}, "chat_color must be"},
		{map[string]any{"skill": "AL_HEAL", "chat": "Hi", "chat_color": "#FF8000"}, "chat_color must be"},
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
				s, l, err := FromRaw(r, m.Mode, func(id int) bool { return id == 83 }, func(id uint32) bool { return ids[id] }, d.ChatLine)
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

// TestParseSkillFields reads the emotion list, the modes and the chat
// color from YAML, as a monster record writes them.
func TestParseSkillFields(t *testing.T) {
	m := parseFlow(t, "{skill: NPC_EMOTION, target: self, emotion: [surprise, heh], modes: [can_move, aggressive, can_attack], chat: Grr, chat_color: 0xFF8000}")
	s, err := ParseSkill(m, testNames{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.Emotions, []int{0, 7}) {
		t.Errorf("emotions %v, want [0 7]", s.Emotions)
	}
	if want := ModeCanMove | ModeAggressive | ModeCanAttack; !s.HasModes || s.Modes != want {
		t.Errorf("modes %v 0x%X, want 0x%X", s.HasModes, s.Modes, want)
	}
	if set, add, del, end := s.ModeChange(); set != s.Modes || add != 0 || del != 0 || end {
		t.Errorf("mode change %X %X %X %v", set, add, del, end)
	}
	if s.Chat != "Grr" || s.ChatColor != 0xFF8000 || s.ChatLineColor() != 0xFF8000 {
		t.Errorf("chat %q 0x%X", s.Chat, s.ChatColor)
	}
	// One emotion, an empty modes list and no chat color.
	s, err = ParseSkill(parseFlow(t, "{skill: NPC_EMOTION, emotion: heh, modes: [], chat: Hi}"), testNames{})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.Emotions, []int{7}) || s.ChatLineColor() != DefaultChatColor {
		t.Errorf("emotions %v, color 0x%X", s.Emotions, s.ChatLineColor())
	}
	if _, _, _, end := s.ModeChange(); !s.HasModes || !end {
		t.Error("an empty modes list does not end the mode change")
	}
	// NPC_EMOTION_ON adds its modes and takes aggressive away.
	s, err = ParseSkill(parseFlow(t, "{skill: NPC_EMOTION_ON, modes: [can_move, can_attack]}"), testNames{})
	if err != nil {
		t.Fatal(err)
	}
	if set, add, del, end := s.ModeChange(); set != 0 || add != ModeCanMove|ModeCanAttack || del != ModeAggressive || end {
		t.Errorf("NPC_EMOTION_ON: %X %X %X %v", set, add, del, end)
	}
}

// TestFromRawEmotion converts NPC_EMOTION and NPC_EMOTION_ON rows as
// mob_parse_row_mobskilldb reads them.
func TestFromRawEmotion(t *testing.T) {
	never := func(int) bool { return false }
	none := func(uint32) bool { return false }
	chats := func(id int) Chat {
		return map[int]Chat{1: {ID: 1, Color: 0xFF0000, Text: "Red"}, 2: {ID: 2, Color: 0x00FF00, Text: "Green"}}[id]
	}
	const own = 0x3885
	for _, c := range []struct {
		name     string
		r        RawSkill
		emotions []int
		hasModes bool
		modes    uint32
		chat     string
		color    uint32
	}{
		{"one emotion", RawSkill{Skill: IDEmotion, Emotion: -1, Values: [5]int{7}}, []int{7}, false, 0, "", 0},
		{"two emotions", RawSkill{Skill: IDEmotion, Emotion: 0, Values: [5]int{7}}, []int{0, 7}, false, 0, "", 0},
		{"the same twice", RawSkill{Skill: IDEmotion, Emotion: 7, Values: [5]int{7}}, []int{7, 7}, false, 0, "", 0},
		{"set modes", RawSkill{Skill: IDEmotion, Emotion: -1, Values: [5]int{7, 0x81}}, []int{7}, true, 0x81, "", 0},
		{"own modes", RawSkill{Skill: IDEmotion, Emotion: -1, Values: [5]int{7, own}}, []int{7}, true, 0, "", 0},
		{"add modes", RawSkill{Skill: IDEmotionOn, Emotion: -1, Values: [5]int{1, own}}, []int{1}, true, own, "", 0},
		{"default color", RawSkill{Skill: 28, Emotion: -1, Chat: 1}, nil, false, 0, "Red", 0},
		{"color", RawSkill{Skill: 28, Emotion: -1, Chat: 2}, nil, false, 0, "Green", 0x00FF00},
	} {
		c.r.State, c.r.Target, c.r.Condition, c.r.Level = "any", "self", "always", 1
		s, lost, err := FromRaw(c.r, own, never, none, chats)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(lost) > 0 {
			t.Errorf("%s: lost %v", c.name, lost)
		}
		if !slices.Equal(s.Emotions, c.emotions) || s.HasModes != c.hasModes || s.Modes != c.modes ||
			s.Chat != c.chat || s.ChatColor != c.color {
			t.Errorf("%s: emotions %v, modes %v 0x%X, chat %q 0x%X", c.name, s.Emotions, s.HasModes, s.Modes, s.Chat, s.ChatColor)
		}
	}
	// A mode without a name is lost.
	r := RawSkill{Skill: IDEmotion, State: "any", Target: "self", Condition: "always", Emotion: -1, Values: [5]int{7, 0x100}}
	if _, lost, err := FromRaw(r, own, never, none, chats); err != nil || len(lost) != 1 {
		t.Errorf("lost %v, %v", lost, err)
	}
}
