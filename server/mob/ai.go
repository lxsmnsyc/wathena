package mob

import "slices"

// Mode bits of rAthena's e_mode (MD_).
const (
	ModeCanMove           uint32 = 0x1
	ModeLooter            uint32 = 0x2
	ModeAggressive        uint32 = 0x4
	ModeAssist            uint32 = 0x8
	ModeCastSensorIdle    uint32 = 0x10
	ModeNoRandomWalk      uint32 = 0x20
	ModeNoCast            uint32 = 0x40
	ModeCanAttack         uint32 = 0x80
	ModeCastSensorChase   uint32 = 0x200
	ModeChangeChase       uint32 = 0x400
	ModeAngry             uint32 = 0x800
	ModeChangeTargetMelee uint32 = 0x1000
	ModeChangeTargetChase uint32 = 0x2000
	ModeTargetWeak        uint32 = 0x4000
	ModeRandomTarget      uint32 = 0x8000
	ModeIgnoreMelee       uint32 = 0x10000
	ModeIgnoreMagic       uint32 = 0x20000
	ModeIgnoreRanged      uint32 = 0x40000
	ModeMvp               uint32 = 0x80000
	ModeIgnoreMisc        uint32 = 0x100000
	ModeKnockbackImmune   uint32 = 0x200000
	ModeTeleportBlock     uint32 = 0x400000
	ModeFixedItemDrop     uint32 = 0x1000000
	ModeDetector          uint32 = 0x2000000
	ModeStatusImmune      uint32 = 0x4000000
	ModeSkillImmune       uint32 = 0x8000000
)

// ModeNames are the names of modes in data/monsters.yml, from rAthena's
// MD_ names.
var ModeNames = map[string]uint32{
	"can_move": ModeCanMove, "looter": ModeLooter, "aggressive": ModeAggressive, "assist": ModeAssist,
	"cast_sensor_idle": ModeCastSensorIdle, "no_random_walk": ModeNoRandomWalk, "no_cast": ModeNoCast,
	"can_attack": ModeCanAttack, "cast_sensor_chase": ModeCastSensorChase, "change_chase": ModeChangeChase,
	"angry": ModeAngry, "change_target_melee": ModeChangeTargetMelee, "change_target_chase": ModeChangeTargetChase,
	"target_weak": ModeTargetWeak, "random_target": ModeRandomTarget, "ignore_melee": ModeIgnoreMelee,
	"ignore_magic": ModeIgnoreMagic, "ignore_ranged": ModeIgnoreRanged, "mvp": ModeMvp, "ignore_misc": ModeIgnoreMisc,
	"knockback_immune": ModeKnockbackImmune, "teleport_block": ModeTeleportBlock, "fixed_item_drop": ModeFixedItemDrop,
	"detector": ModeDetector, "status_immune": ModeStatusImmune, "skill_immune": ModeSkillImmune,
}

// AITypes are the modes of each Aegis AI type, as rAthena's
// e_aegis_monstertype (MONSTER_TYPE_). Types 14, 15, 16, 18, 22 and 23 do
// not exist.
var AITypes = map[int]uint32{
	1: 0x81, 2: 0x83, 3: 0x1089, 4: 0x3885, 5: 0x2085, 6: 0, 7: 0x108B, 8: 0x7085, 9: 0x3095,
	10: 0x84, 11: 0x84, 12: 0x2085, 13: 0x308D, 17: 0x91, 19: 0x3095, 20: 0x3295, 21: 0x3695,
	24: 0xA1, 25: 0x1, 26: 0xB695, 27: 0x8084,
}

// specialAITypes are the AI types of rAthena that no number names.
var specialAITypes = map[string]uint32{"ABR_PASSIVE": 0x21, "ABR_OFFENSIVE": 0xA5}

// AIModes returns the modes of an AI type of monsters.json, such as "04"
// or "ABR_PASSIVE", and whether it exists.
func AIModes(ai string) (uint32, bool) {
	if m, ok := specialAITypes[ai]; ok {
		return m, true
	}
	n := 0
	for _, c := range ai {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
		if n > 99 {
			return 0, false
		}
	}
	m, ok := AITypes[n]
	return m, ok && ai != ""
}

// ClassModes are the modes MobDatabase::loadingFinished adds for a
// monster class (CLASS_).
func ClassModes(class int) uint32 {
	switch class {
	case 1: // CLASS_BOSS
		return ModeDetector | ModeStatusImmune | ModeKnockbackImmune
	case 2: // CLASS_GUARDIAN
		return ModeStatusImmune
	case 4: // CLASS_BATTLEFIELD
		return ModeStatusImmune | ModeSkillImmune
	case 5: // CLASS_EVENT
		return ModeFixedItemDrop
	}
	return 0
}

// Modes is a monster's mode as mob_db reads it: the AI type's modes,
// with on set and off cleared, and the class's modes.
func Modes(ai uint32, on, off []uint32, class int) uint32 {
	m := ai
	for _, b := range on {
		m |= b
	}
	for _, b := range off {
		m &^= b
	}
	return m | ClassModes(class)
}

// State is a monster's skill state, rAthena's MobSkillState (MSS_).
type State int

// Skill states.
const (
	StateAny State = iota - 1
	StateIdle
	StateWalk
	StateLoot
	StateDead
	// StateBerserk is mob_skill_db's "attack": the monster attacks.
	StateBerserk
	// StateAngry is "angry": an aggressive monster attacks first.
	StateAngry
	// StateRush is "chase": the monster chases a target that hit it.
	StateRush
	// StateFollow is "follow": an aggressive monster chases a target.
	StateFollow
	StateAnyTarget
)

// stateNames are rAthena's words for the states, which data/monsters.yml
// keeps.
var stateNames = []string{"any", "idle", "walk", "loot", "dead", "attack", "angry", "chase", "follow", "anytarget"}

// String is the state's word, as monster.state reads it.
func (s State) String() string {
	if i := int(s) + 1; i >= 0 && i < len(stateNames) {
		return stateNames[i]
	}
	return "idle"
}

// ParseState reads a state's word.
func ParseState(s string) (State, bool) {
	i := slices.Index(stateNames, s)
	return State(i - 1), i >= 0
}

// StateNames lists the words of the states.
func StateNames() []string { return slices.Clone(stateNames) }

// IsChasing is mob_is_chasing: a monster that walks after a target or an
// item.
func IsChasing(s State) bool { return s == StateLoot || s == StateRush || s == StateFollow }

// AIState is the part of rAthena's mob_data that the AI rules read and
// change.
type AIState struct {
	// State is state.skillstate.
	State State
	// Aggressive is state.aggressive: the monster attacked first and was
	// not hit yet, so it is angry rather than berserk.
	Aggressive bool
}

// SetState is mob_setstate. Attack and chase states become angry and
// follow while the monster is aggressive. Other states reset aggressive
// from the angry mode.
func (a *AIState) SetState(s State, mode uint32) {
	switch s {
	case StateBerserk, StateAngry:
		if a.Aggressive {
			a.State = StateAngry
		} else {
			a.State = StateBerserk
		}
	case StateRush, StateFollow:
		if a.Aggressive {
			a.State = StateFollow
		} else {
			a.State = StateRush
		}
	default:
		a.Aggressive = mode&ModeAngry != 0
		a.State = s
	}
}

// CanChangeTarget is mob_can_changetarget with rAthena's default
// monster_ai of 0: whether a monster in its state may take a new
// target. normAttacked is set when the new target is the one that hit
// the monster with a normal attack (norm_attacked_id).
func (a *AIState) CanChangeTarget(mode uint32, normAttacked bool) bool {
	switch a.State {
	case StateBerserk:
		return mode&ModeChangeTargetMelee != 0 && normAttacked
	case StateRush:
		return mode&ModeChangeTargetChase != 0
	case StateFollow, StateAngry, StateIdle, StateWalk, StateLoot:
		return true
	}
	return false
}
