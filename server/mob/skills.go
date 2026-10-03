package mob

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Target is whom or where a monster skill is cast, rAthena's
// e_mob_skill_target (MST_).
type Target int

// Skill targets, in rAthena's order.
const (
	TargetTarget Target = iota
	TargetRandom
	TargetSelf
	TargetFriend
	TargetMaster
	TargetAround5
	TargetAround6
	TargetAround7
	TargetAround8
	TargetAround1
	TargetAround2
	TargetAround3
	TargetAround4
)

// targetNames are the names of data/monsters.yml, in Target order.
var targetNames = []string{"target", "random_target", "self", "friend", "master",
	"around5", "around6", "around7", "around8", "around1", "around2", "around3", "around4"}

// rathenaTargets are mob_skill_db's words, in Target order. "around" is
// around4.
var rathenaTargets = []string{"target", "randomtarget", "self", "friend", "master",
	"around5", "around6", "around7", "around8", "around1", "around2", "around3", "around4"}

func (t Target) String() string {
	if t >= 0 && int(t) < len(targetNames) {
		return targetNames[t]
	}
	return "target"
}

// Around reports whether the target is an area around the monster or its
// target, for ground skills.
func (t Target) Around() bool { return t >= TargetAround5 }

// AroundSize is the half size of the area of an around target, and
// whether the area is around the target rather than the monster:
// around1 to around4 are 3x3 to 9x9 around the monster, and around5 to
// around8 the same around the target.
func (t Target) AroundSize() (int, bool) {
	if t >= TargetAround1 {
		return int(t-TargetAround1) + 1, false
	}
	return int(t-TargetAround5) + 1, true
}

// Condition is when a monster skill may be used, rAthena's
// e_mob_skill_condition (MSC_).
type Condition int

// Skill conditions, in rAthena's order.
const (
	CondAlways Condition = iota
	CondHPBelow
	CondHPBetween
	CondFriendHPBelow
	CondFriendHPBetween
	CondHasStatus
	CondLacksStatus
	CondFriendHasStatus
	CondFriendLacksStatus
	CondAttackersAbove
	CondAttackersAtLeast
	CondSlavesBelow
	CondSlavesAtMost
	CondHitMelee
	CondHitRanged
	CondAfterSkill
	CondHitBySkill
	CondTargetedByCast
	CondHitWhileStuck
	CondMasterHPBelow
	CondMasterAttacked
	CondSummonedByAlchemist
	CondSpawned
	CondMonstersNearbyAbove
	CondHitByGroundSkill
	CondDamageAbove
	CondTargetCasting
)

// conditionNames are the names of review decision 22, in Condition
// order.
var conditionNames = []string{"always", "hp_below", "hp_between", "friend_hp_below", "friend_hp_between",
	"has_status", "lacks_status", "friend_has_status", "friend_lacks_status", "attackers_above",
	"attackers_at_least", "slaves_below", "slaves_at_most", "hit_melee", "hit_ranged", "after_skill",
	"hit_by_skill", "targeted_by_cast", "hit_while_stuck", "master_hp_below", "master_attacked",
	"summoned_by_alchemist", "spawned", "monsters_nearby_above", "hit_by_ground_skill", "damage_above",
	"target_casting"}

// rathenaConditions are mob_skill_db's words, in Condition order.
var rathenaConditions = []string{"always", "myhpltmaxrate", "myhpinrate", "friendhpltmaxrate", "friendhpinrate",
	"mystatuson", "mystatusoff", "friendstatuson", "friendstatusoff", "attackpcgt", "attackpcge", "slavelt",
	"slavele", "closedattacked", "longrangeattacked", "afterskill", "skillused", "casttargeted", "rudeattacked",
	"masterhpltmaxrate", "masterattacked", "alchemist", "onspawn", "mobnearbygt", "groundattacked", "damagedgt",
	"trickcasting"}

func (c Condition) String() string {
	if c >= 0 && int(c) < len(conditionNames) {
		return conditionNames[c]
	}
	return "always"
}

// Event reports whether the condition is an event that mobskill_event or
// mob_spawn passes, rather than a state the AI checks each second.
func (c Condition) Event() bool {
	switch c {
	case CondHitMelee, CondHitRanged, CondHitBySkill, CondTargetedByCast, CondSpawned,
		CondHitByGroundSkill, CondDamageAbove:
		return true
	}
	return false
}

// AnyBadStatus is the condition value "anybad" of mob_skill_db: any of
// the common ailments.
const AnyBadStatus = -1

// Skill is one skill of a monster: an entry of a monster record's skills
// list (proposal 0002, review decision 22), from mob_skill_db.
type Skill struct {
	// ID is the skill, and Level its level.
	ID, Level int
	// State is the state the monster must be in. StateAny matches every
	// state but dead, and StateAnyTarget every state with a target.
	State State
	// Rate is the chance per 10,000 that the skill is used when its
	// condition holds.
	Rate int
	// CastTime is the cast time in milliseconds, which replaces the
	// skill's own. Delay is how long the monster waits before it uses the
	// skill again.
	CastTime, Delay int
	// Cancelable is set when hits can stop the cast.
	Cancelable bool
	Target     Target
	Condition  Condition
	// Value is condition_value: a percent, a count, a status, a skill or
	// an amount, by condition. Value2 is the upper bound of hp_between
	// and friend_hp_between.
	Value, Value2 int
	// Summons are the monsters of NPC_SUMMONSLAVE, NPC_SUMMONMONSTER,
	// NPC_METAMORPHOSIS and NPC_TRANSFORMATION, by ID.
	Summons []uint32
	// Emotions are shown over the monster, in turn, when the skill takes
	// effect: none, one, or two. NPC_EMOTION's val1 is the second when
	// the row's Emotion column shows one first.
	Emotions []int
	// HasModes is set when the entry changes the monster's modes, which
	// only NPC_EMOTION and NPC_EMOTION_ON do, through SC_MODECHANGE.
	// NPC_EMOTION sets the modes of ModeMask to Modes, or with no Modes
	// ends the change, so the monster has its own modes again.
	// NPC_EMOTION_ON adds Modes, and takes aggressive away when Modes
	// lacks it.
	HasModes bool
	Modes    uint32
	// Chat is a line the monster says when it uses the skill, or "".
	// ChatColor is its color as 0xRRGGBB, or 0 for DefaultChatColor.
	Chat      string
	ChatColor uint32
}

// DefaultChatColor is the color of a chat line without chat_color, as
// mob_chat_db.yml's default Color.
const DefaultChatColor uint32 = 0xFF0000

// ModeMask is rAthena's MD_MASK: the modes that NPC_EMOTION sets.
const ModeMask uint32 = 0xFFFF

// ModeChange is the mode change of an NPC_EMOTION or NPC_EMOTION_ON
// entry as rAthena's val2 to val5 of the skill: SC_MODECHANGE's set,
// added and removed modes, and whether the skill ends an earlier change
// first.
func (s *Skill) ModeChange() (set, add, del uint32, end bool) {
	if !s.HasModes {
		return 0, 0, 0, false
	}
	switch s.ID {
	case IDEmotion:
		if s.Modes == 0 {
			return 0, 0, 0, true
		}
		return s.Modes, 0, 0, false
	case IDEmotionOn:
		// mob_parse_row_mobskilldb: the modes are added, and a list
		// without aggressive takes it away.
		if s.Modes&ModeAggressive == 0 {
			del = ModeAggressive
		}
		return 0, s.Modes, del, false
	}
	return 0, 0, 0, false
}

// ChatLineColor is the color the chat line is sent with.
func (s *Skill) ChatLineColor() uint32 {
	if s.ChatColor == 0 {
		return DefaultChatColor
	}
	return s.ChatColor
}

// Skills with values that mob_skill_db keeps in val1 to val5.
const (
	IDMetamorphosis  = 193 // NPC_METAMORPHOSIS
	IDSummonSlave    = 196 // NPC_SUMMONSLAVE
	IDEmotion        = 197 // NPC_EMOTION
	IDTransformation = 198 // NPC_TRANSFORMATION
	IDSummonMonster  = 209 // NPC_SUMMONMONSTER
	IDEmotionOn      = 474 // NPC_EMOTION_ON
	IDDeathSummon    = 718 // NPC_DEATHSUMMON, renewal only
	IDRandomMove     = 331 // NPC_RANDOMMOVE
	IDSpeedUp        = 332 // NPC_SPEEDUP
	IDCallSlave      = 352 // NPC_CALLSLAVE
)

// SummonSkill reports whether a skill reads Summons.
func SummonSkill(id int) bool {
	switch id {
	case IDSummonSlave, IDSummonMonster, IDTransformation, IDMetamorphosis, IDDeathSummon:
		return true
	}
	return false
}

// FromRaw converts a mob_skill_db row as rAthena reads it.
//   - ground reports whether a skill is a ground skill: rAthena turns an
//     around target into target for other skills.
//   - exists reports whether a monster ID exists, for summons.
//   - chat finds a mob_chat_db line, or the zero Chat.
//
// mode is the monster's mode, which an NPC_EMOTION row compares its mode
// with. lost lists what the row does that a Skill cannot hold: modes
// that ModeNames does not name, and the added and removed modes of
// val3 to val5, which no row of mob_skill_db sets.
func FromRaw(r RawSkill, mode uint32, ground func(id int) bool, exists func(id uint32) bool, chat func(id int) Chat) (s Skill, lost []string, err error) {
	st, ok := ParseState(r.State)
	if !ok {
		return s, nil, fmt.Errorf("unknown state %q", r.State)
	}
	var tgt Target
	if r.Target != "around" {
		i := slices.Index(rathenaTargets, r.Target)
		if i < 0 {
			return s, nil, fmt.Errorf("unknown target %q", r.Target)
		}
		tgt = Target(i)
	} else {
		tgt = TargetAround4
	}
	// mob_parse_row_mobskilldb: an area target is only for ground skills.
	if tgt > TargetMaster && !ground(r.Skill) {
		tgt = TargetTarget
	}
	ci := slices.Index(rathenaConditions, r.Condition)
	if ci < 0 {
		return s, nil, fmt.Errorf("unknown condition %q", r.Condition)
	}
	s = Skill{
		ID: r.Skill, Level: r.Level, State: st, Rate: r.Rate, CastTime: r.CastTime, Delay: r.Delay,
		Cancelable: r.Cancelable, Target: tgt, Condition: Condition(ci), Value: r.ConditionValue,
	}
	if r.Emotion >= 0 {
		s.Emotions = []int{r.Emotion}
	}
	switch s.Condition {
	case CondHPBetween, CondFriendHPBetween:
		s.Value2 = r.Values[0]
	}
	if SummonSkill(r.ID()) {
		// mob_summonslave stops at the first ID that does not exist.
		for _, v := range r.Values {
			if v <= 0 || !exists(uint32(v)) { // #nosec G115 -- checked above.
				break
			}
			s.Summons = append(s.Summons, uint32(v)) // #nosec G115 -- as above.
		}
	}
	switch r.Skill {
	case IDEmotion, IDEmotionOn:
		// val1 is the emotion the skill shows, after the Emotion
		// column's emotion when the row has one.
		s.Emotions = append(s.Emotions, r.Values[0])
		// val2 is a mode. NPC_EMOTION sets it, or returns the monster to
		// its own mode when it is the monster's mode, and NPC_EMOTION_ON
		// adds it (mob_parse_row_mobskilldb).
		if v := uint32(r.Values[1]); v != 0 { // #nosec G115 -- a mode's bits.
			s.HasModes = true
			if r.Skill == IDEmotion && v == mode {
				v = 0
			}
			s.Modes = v
			if _, rest := ModeList(v); rest != 0 {
				lost = append(lost, "modes without a name in the mode change of "+r.SkillName)
			}
		}
		if r.Values[2] != 0 || r.Values[3] != 0 || r.Values[4] != 0 {
			lost = append(lost, "the added or removed modes of "+r.SkillName)
		}
	}
	if r.Chat > 0 {
		c := chat(r.Chat)
		s.Chat = c.Text
		if c.Color != DefaultChatColor {
			s.ChatColor = c.Color
		}
	}
	return s, lost, nil
}

// ModeList names the modes of bits, in bit order, and returns the bits
// that ModeNames does not name.
func ModeList(bits uint32) (names []string, rest uint32) {
	rest = bits
	for b := uint32(1); b != 0; b <<= 1 {
		if bits&b == 0 {
			continue
		}
		for n, v := range ModeNames {
			if v == b {
				names = append(names, n)
				rest &^= b
				break
			}
		}
	}
	return names, rest
}

// ID is the row's skill ID.
func (r RawSkill) ID() int { return r.Skill }

// Names finds the names that data/monsters.yml uses for skills,
// monsters, statuses and emotions, and their IDs.
type Names interface {
	SkillID(name string) (int, bool)
	SkillName(id int) (string, bool)
	// Ground reports whether a skill is a ground skill.
	Ground(id int) bool
	MonsterID(name string) (uint32, bool)
	MonsterName(id uint32) (string, bool)
	// StatusID and StatusName use the names of wa.statuses, such as
	// stone.
	StatusID(name string) (int, bool)
	StatusName(id int) (string, bool)
	// EmotionID and EmotionName use the names of wa.emotions.
	EmotionID(name string) (int, bool)
	EmotionName(id int) (string, bool)
}

// skillFields are the fields of a skills entry, in the order YAML writes
// them.
var skillFields = []string{"skill", "level", "state", "rate", "cast_time", "delay", "cancelable", "target",
	"condition", "condition_value", "summons", "emotion", "modes", "chat", "chat_color"}

// valueKind is what condition_value holds for a condition.
func (c Condition) valueKind() string {
	switch c {
	case CondAlways, CondHitMelee, CondHitRanged, CondTargetedByCast, CondHitWhileStuck, CondMasterAttacked,
		CondSummonedByAlchemist, CondSpawned, CondHitByGroundSkill, CondTargetCasting:
		return ""
	case CondHPBetween, CondFriendHPBetween:
		return "range"
	case CondHasStatus, CondLacksStatus, CondFriendHasStatus, CondFriendLacksStatus:
		return "status"
	case CondHitBySkill, CondAfterSkill:
		return "skill"
	}
	return "number"
}

// YAML writes the entry as a flow mapping, leaving out fields at their
// defaults: level 1, state any, rate 10000, no cast time or delay, not
// cancelable, target target and condition always.
func (s Skill) YAML(n Names) (string, error) {
	var b strings.Builder
	name, ok := n.SkillName(s.ID)
	if !ok {
		return "", fmt.Errorf("skill %d has no name", s.ID)
	}
	b.WriteString("{skill: " + name)
	add := func(k, v string) { b.WriteString(", " + k + ": " + v) }
	if s.Level != 1 {
		add("level", fmt.Sprint(s.Level))
	}
	if s.State != StateAny {
		add("state", s.State.String())
	}
	if s.Rate != 10000 {
		add("rate", fmt.Sprint(s.Rate))
	}
	if s.CastTime != 0 {
		add("cast_time", fmt.Sprint(s.CastTime))
	}
	if s.Delay != 0 {
		add("delay", fmt.Sprint(s.Delay))
	}
	if s.Cancelable {
		add("cancelable", "true")
	}
	if s.Target != TargetTarget {
		add("target", s.Target.String())
	}
	if s.Condition != CondAlways {
		add("condition", s.Condition.String())
	}
	switch s.Condition.valueKind() {
	case "number":
		add("condition_value", fmt.Sprint(s.Value))
	case "range":
		add("condition_value", fmt.Sprintf("[%d, %d]", s.Value, s.Value2))
	case "status":
		if s.Value == AnyBadStatus {
			add("condition_value", "any")
		} else if st, ok := n.StatusName(s.Value); ok {
			add("condition_value", st)
		} else {
			return "", fmt.Errorf("status %d has no name", s.Value)
		}
	case "skill":
		if s.Value != 0 {
			sk, ok := n.SkillName(s.Value)
			if !ok {
				return "", fmt.Errorf("skill %d has no name", s.Value)
			}
			add("condition_value", sk)
		}
	}
	if len(s.Summons) > 0 {
		var names []string
		for _, id := range s.Summons {
			m, ok := n.MonsterName(id)
			if !ok {
				return "", fmt.Errorf("monster %d has no name", id)
			}
			names = append(names, m)
		}
		add("summons", "["+strings.Join(names, ", ")+"]")
	}
	var emotions []string
	for _, em := range s.Emotions {
		e, ok := n.EmotionName(em)
		if !ok {
			return "", fmt.Errorf("emotion %d has no name", em)
		}
		emotions = append(emotions, e)
	}
	switch len(emotions) {
	case 0:
	case 1:
		add("emotion", emotions[0])
	default:
		add("emotion", "["+strings.Join(emotions, ", ")+"]")
	}
	if s.HasModes {
		names, rest := ModeList(s.Modes)
		if rest != 0 {
			return "", fmt.Errorf("modes 0x%X have no name", rest)
		}
		add("modes", "["+strings.Join(names, ", ")+"]")
	}
	if s.Chat != "" {
		add("chat", quote(s.Chat))
		if s.ChatColor != 0 && s.ChatColor != DefaultChatColor {
			add("chat_color", fmt.Sprintf("0x%06X", s.ChatColor))
		}
	}
	b.WriteString("}")
	return b.String(), nil
}

// quote writes a YAML double-quoted string.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Limits of a skills entry.
const (
	// MaxSkills is MAX_MOBSKILL: the most skills a monster has.
	MaxSkills = 50
	// MaxSkillLevel is MAX_MOBSKILL_LEVEL.
	MaxSkillLevel = 100
	// MaxDelay is MOB_MAX_DELAY: a day.
	MaxDelay = 24 * 3600 * 1000
	// MaxSummons is the most monsters an entry summons, as val1 to val5.
	MaxSummons = 5
	// MaxChat is the longest chat line, CHAT_SIZE_MAX less one.
	MaxChat = 255
)

// ParseSkills reads the skills list of a monster record.
func ParseSkills(v any, n Names) ([]Skill, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, errors.New("want a list of { skill, state, rate, ... } entries")
	}
	if len(list) > MaxSkills {
		return nil, fmt.Errorf("a monster has at most %d skills", MaxSkills)
	}
	out := make([]Skill, 0, len(list))
	var errs []string
	for i, e := range list {
		m, ok := e.(map[string]any)
		if !ok {
			errs = append(errs, fmt.Sprintf("skill %d: want { skill: NAME, ... }", i+1))
			continue
		}
		s, err := ParseSkill(m, n)
		if err != nil {
			errs = append(errs, fmt.Sprintf("skill %d: %v", i+1, err))
			continue
		}
		out = append(out, s)
	}
	if len(errs) > 0 {
		return nil, errors.New(strings.Join(errs, "; "))
	}
	return out, nil
}

// ParseSkill reads one entry of a skills list.
func ParseSkill(m map[string]any, n Names) (Skill, error) {
	s := Skill{Level: 1, State: StateAny, Rate: 10000}
	var errs []string
	bad := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	for k := range m {
		if !slices.Contains(skillFields, k) {
			bad("%q is not a field; skills entries have %s", k, strings.Join(skillFields, ", "))
		}
	}
	num := func(k string, lo, hi int, to *int) {
		v, ok := m[k]
		if !ok {
			return
		}
		x, ok := v.(int)
		if !ok || x < lo || x > hi {
			bad("%s must be a whole number from %d to %d", k, lo, hi)
			return
		}
		*to = x
	}
	name, _ := m["skill"].(string)
	id, ok := n.SkillID(name)
	if !ok {
		return s, fmt.Errorf("skill: there is no skill %q", name)
	}
	s.ID = id
	num("level", 1, MaxSkillLevel, &s.Level)
	if v, ok := m["state"]; ok {
		w, _ := v.(string)
		st, ok := ParseState(w)
		if !ok {
			bad("state: %q is not one of %s", w, strings.Join(stateNames, ", "))
		}
		s.State = st
	}
	num("rate", 1, 10000, &s.Rate)
	num("cast_time", 0, MaxDelay, &s.CastTime)
	num("delay", 0, MaxDelay, &s.Delay)
	if v, ok := m["cancelable"]; ok {
		b, ok := v.(bool)
		if !ok {
			bad("cancelable must be true or false")
		}
		s.Cancelable = b
	}
	if v, ok := m["target"]; ok {
		w, _ := v.(string)
		i := slices.Index(targetNames, w)
		if i < 0 {
			bad("target: %q is not one of %s", w, strings.Join(targetNames, ", "))
		}
		s.Target = Target(max(i, 0))
		if s.Target.Around() && !n.Ground(s.ID) {
			bad("target: %s is for ground skills; %s is not one", w, name)
		}
	}
	if v, ok := m["condition"]; ok {
		w, _ := v.(string)
		i := slices.Index(conditionNames, w)
		if i < 0 {
			bad("condition: %q is not one of %s", w, strings.Join(conditionNames, ", "))
		}
		s.Condition = Condition(max(i, 0))
	}
	v, has := m["condition_value"]
	switch kind := s.Condition.valueKind(); kind {
	case "":
		if has {
			bad("condition_value: %s takes none", s.Condition)
		}
	case "number":
		if !has {
			bad("condition_value: %s needs a number", s.Condition)
		}
		num("condition_value", 0, 1<<31-1, &s.Value)
	case "range":
		list, _ := v.([]any)
		lo, ok1 := listInt(list, 0)
		hi, ok2 := listInt(list, 1)
		if len(list) != 2 || !ok1 || !ok2 || lo < 0 || hi > 100 || lo > hi {
			bad("condition_value: %s needs [lowest, highest] percents from 0 to 100", s.Condition)
		}
		s.Value, s.Value2 = lo, hi
	case "status":
		w, _ := v.(string)
		if w == "any" {
			s.Value = AnyBadStatus
		} else if st, ok := n.StatusID(w); ok {
			s.Value = st
		} else {
			bad("condition_value: %s needs a status from wa.statuses, such as stone, or any", s.Condition)
		}
	case "skill":
		if has {
			w, _ := v.(string)
			sk, ok := n.SkillID(w)
			if !ok {
				bad("condition_value: there is no skill %q", w)
			}
			s.Value = sk
		}
	}
	if v, ok := m["summons"]; ok {
		list, ok := v.([]any)
		if !ok || len(list) == 0 || len(list) > MaxSummons {
			bad("summons must be a list of 1 to %d monsters", MaxSummons)
		}
		for _, e := range list {
			w, _ := e.(string)
			id, ok := n.MonsterID(w)
			if !ok {
				bad("summons: there is no monster %q", w)
				continue
			}
			s.Summons = append(s.Summons, id)
		}
		if !SummonSkill(s.ID) {
			bad("summons: only NPC_SUMMONSLAVE, NPC_SUMMONMONSTER, NPC_METAMORPHOSIS, NPC_TRANSFORMATION and NPC_DEATHSUMMON summon")
		}
	}
	if v, ok := m["emotion"]; ok {
		list, isList := v.([]any)
		if !isList {
			list = []any{v}
		} else if len(list) != 2 {
			bad("emotion must be one emotion or a list of two, such as [surprise, heh]")
			list = nil
		}
		for _, x := range list {
			w, _ := x.(string)
			e, ok := n.EmotionID(w)
			if !ok {
				bad("emotion: %q is not in wa.emotions", w)
				continue
			}
			s.Emotions = append(s.Emotions, e)
		}
	}
	if v, ok := m["modes"]; ok {
		list, isList := v.([]any)
		if !isList {
			bad("modes must be a list, such as [can_move, aggressive]")
		}
		for _, x := range list {
			w, _ := x.(string)
			bit, ok := ModeNames[w]
			if !ok {
				bad("modes: %q is not a mode; the modes are %s", w, strings.Join(slices.Sorted(maps.Keys(ModeNames)), ", "))
				continue
			}
			s.Modes |= bit
		}
		s.HasModes = true
		switch {
		case s.ID != IDEmotion && s.ID != IDEmotionOn:
			bad("modes: only NPC_EMOTION and NPC_EMOTION_ON change modes")
		case s.ID == IDEmotionOn && isList && len(list) == 0:
			bad("modes: NPC_EMOTION_ON needs a mode to add")
		}
	}
	if v, ok := m["chat"]; ok {
		w, _ := v.(string)
		if w == "" || len(w) > MaxChat {
			bad("chat must be 1 to %d bytes", MaxChat)
		}
		s.Chat = w
	}
	if v, ok := m["chat_color"]; ok {
		c, ok := v.(int)
		if !ok || c < 1 || c > 0xFFFFFF {
			bad("chat_color must be a color from 0x000001 to 0xFFFFFF, such as 0xFF8000")
			c = 0
		}
		if s.Chat == "" {
			bad("chat_color needs chat")
		}
		s.ChatColor = uint32(max(c, 0)) // #nosec G115 -- checked above.
	}
	if len(errs) > 0 {
		return s, errors.New(strings.Join(errs, "; "))
	}
	return s, nil
}

func listInt(list []any, i int) (int, bool) {
	if i >= len(list) {
		return 0, false
	}
	n, ok := list[i].(int)
	return n, ok
}

// StateMatches is the state check of mobskill_use: an entry's state is
// the monster's, or any (except while dead), or anytarget while the
// monster has a target and does not loot.
func StateMatches(entry, cur State, hasTarget bool) bool {
	if entry == cur {
		return true
	}
	if cur == StateDead {
		return false
	}
	return entry == StateAny || entry == StateAnyTarget && hasTarget && cur != StateLoot
}

// DelayEntries is mobskill_delay with rAthena's default monster_ai: after
// the monster used entry used, the delay of the first entry of the same
// skill whose state matches the monster's state now applies to every
// entry of that skill. It returns the delay and whether one was found.
func DelayEntries(skills []Skill, used int, cur State, hasTarget bool) int {
	id := skills[used].ID
	for _, s := range skills {
		if s.ID != id {
			continue
		}
		if s.State == cur || s.State == StateAny || s.State == StateAnyTarget && hasTarget && cur != StateLoot {
			return s.Delay
		}
	}
	return 0
}
