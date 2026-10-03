package mob

import "testing"

func TestAIModes(t *testing.T) {
	for ai, want := range map[string]uint32{"01": 0x81, "04": 0x3885, "06": 0, "21": 0x3695, "27": 0x8084, "ABR_PASSIVE": 0x21} {
		if got, ok := AIModes(ai); !ok || got != want {
			t.Errorf("AI %s: %#x %v, want %#x", ai, got, ok, want)
		}
	}
	for _, ai := range []string{"14", "15", "16", "18", "22", "23", "28", "", "4x"} {
		if _, ok := AIModes(ai); ok {
			t.Errorf("AI %q exists", ai)
		}
	}
	// Baphomet: AI 21, Mvp on, boss class.
	if got := Modes(0x3695, []uint32{ModeMvp}, nil, 1); got != 0x3695|ModeMvp|ModeDetector|ModeStatusImmune|ModeKnockbackImmune {
		t.Errorf("Baphomet's modes %#x", got)
	}
	if got := Modes(0x3885, nil, []uint32{ModeAggressive}, 0); got != 0x3885&^ModeAggressive {
		t.Errorf("modes off: %#x", got)
	}
}

// TestSetState checks mob_setstate: attack and chase states turn into
// angry and follow while the monster is aggressive, and other states
// reset aggressive from the angry mode.
func TestSetState(t *testing.T) {
	var a AIState
	a.SetState(StateIdle, ModeAngry)
	if !a.Aggressive || a.State != StateIdle {
		t.Fatalf("idle with angry: %+v", a)
	}
	a.SetState(StateBerserk, ModeAngry)
	if a.State != StateAngry {
		t.Errorf("attack while aggressive is %s, want angry", a.State)
	}
	a.SetState(StateRush, ModeAngry)
	if a.State != StateFollow {
		t.Errorf("chase while aggressive is %s, want follow", a.State)
	}
	a.Aggressive = false
	a.SetState(StateAngry, ModeAngry)
	if a.State != StateBerserk {
		t.Errorf("angry after a hit is %s, want attack", a.State)
	}
	a.SetState(StateWalk, 0)
	if a.Aggressive || a.State != StateWalk {
		t.Errorf("walk without angry: %+v", a)
	}
}

// TestCanChangeTarget checks mob_can_changetarget with monster_ai 0.
func TestCanChangeTarget(t *testing.T) {
	cases := []struct {
		state  State
		mode   uint32
		normal bool
		want   bool
	}{
		{StateBerserk, 0, true, false},
		{StateBerserk, ModeChangeTargetMelee, false, false},
		{StateBerserk, ModeChangeTargetMelee, true, true},
		{StateRush, 0, false, false},
		{StateRush, ModeChangeTargetChase, false, true},
		{StateIdle, 0, false, true},
		{StateWalk, 0, false, true},
		{StateLoot, 0, false, true},
		{StateAngry, 0, false, true},
		{StateFollow, 0, false, true},
		{StateDead, ModeChangeTargetMelee | ModeChangeTargetChase, true, false},
	}
	for _, c := range cases {
		a := AIState{State: c.state}
		if got := a.CanChangeTarget(c.mode, c.normal); got != c.want {
			t.Errorf("%s mode %#x normal %v: %v, want %v", c.state, c.mode, c.normal, got, c.want)
		}
	}
}

func TestStateNames(t *testing.T) {
	for _, w := range StateNames() {
		s, ok := ParseState(w)
		if !ok || s.String() != w {
			t.Errorf("%q reads as %v %v", w, s, ok)
		}
	}
	if _, ok := ParseState("berserk"); ok {
		t.Error("berserk is not a word of mob_skill_db")
	}
	if !IsChasing(StateLoot) || !IsChasing(StateRush) || !IsChasing(StateFollow) || IsChasing(StateBerserk) {
		t.Error("mob_is_chasing")
	}
}
