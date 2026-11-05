package convert

import (
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/modscript"
)

// timerRuntime converts scripts and starts the last NPC.
func timerRuntime(t *testing.T, scripts map[string]string) (*eventHost, *modscript.Runtime, *modscript.NPC, *logRecorder) {
	t.Helper()
	res := builtinResult(t, scripts)
	n := res.NPCs[len(res.NPCs)-1]
	if n.Status != Full {
		t.Fatalf("%s: %s %v", n.Source, n.Status, n.Unsupported)
	}
	_, base, _ := testRuntime(t, "renewal")
	h := &eventHost{npcHost: &npcHost{testHost: base, npcs: map[int64]*NPC{1: n}, steps: map[int64]modscript.Step{},
		ended: map[int64]bool{}, vars: map[modscript.VariableKey]string{}}}
	logs := &logRecorder{}
	rt := builtinRuntime(t, res, h, logs)
	inst, err := rt.StartNPC(1, "core:"+n.Script)
	if err != nil {
		t.Fatal(err)
	}
	return h, rt, inst, logs
}

// moveTo moves the zone clock, and the server clock with it, and runs
// both kinds of timers.
func moveTo(h *eventHost, rt *modscript.Runtime, ms int64) {
	h.now = ms
	rt.Advance(ms)
	rt.RunServerTimers(testClock.Add(time.Duration(ms) * time.Millisecond))
}

// TestRunSleepWithoutPlayer runs the waits of labels that run without a
// player (review decision 60, A4): OnInit, a timer label, and a label
// that donpcevent sends.
func TestRunSleepWithoutPlayer(t *testing.T) {
	h, rt, n, logs := timerRuntime(t, map[string]string{"Waits#t": `{
	end;
OnInit:
	announce "init", bc_all;
	sleep 1000;
	announce "init later", bc_all;
	initnpctimer;
	end;
OnTimer500:
	announce "t500", bc_all;
	sleep2 700;
	announce "t500 later", bc_all;
	end;
OnGo:
	announce "go", bc_all;
	sleep 100;
	announce "go later", bc_all;
	end;
}`})
	moveTo(h, rt, 0)
	if got := strings.Join(h.said, "|"); got != "init" {
		t.Fatalf("said %q", got)
	}
	moveTo(h, rt, 999)
	moveTo(h, rt, 1000)
	if got := strings.Join(h.said, "|"); got != "init|init later" {
		t.Fatalf("said %q", got)
	}
	moveTo(h, rt, 1500)
	if got := strings.Join(h.said, "|"); got != "init|init later|t500" {
		t.Fatalf("said %q", got)
	}
	moveTo(h, rt, 2200)
	if got := strings.Join(h.said, "|"); got != "init|init later|t500|t500 later" {
		t.Fatalf("said %q", got)
	}
	if _, _, err := rt.FireNPCEvent(n, "ongo", 0); err != nil {
		t.Fatal(err)
	}
	moveTo(h, rt, 2300)
	if got := strings.Join(h.said, "|"); got != "init|init later|t500|t500 later|go|go later" {
		t.Fatalf("said %q", got)
	}
	for _, m := range logs.msgs {
		t.Error(m)
	}
}

// TestRunTimerCommandsOnAnotherNPC runs another NPC's timer commands
// over the hidden labels (review decision 60, C4): a timer starts and
// stops, with no player, and a tick of 0 is set.
func TestRunTimerCommandsOnAnotherNPC(t *testing.T) {
	h, rt, n, logs := timerRuntime(t, map[string]string{"Target#t": `{
	end;
OnTimer1000:
	announce "tick", bc_all;
	end;
OnTimer3000:
	announce "tick3", bc_all;
	end;
}`})
	if _, ran, err := rt.FireNPCEvent(n, "watimerinit", 0); err != nil || !ran {
		t.Fatalf("init: %v %v", ran, err)
	}
	moveTo(h, rt, 1000)
	if got := strings.Join(h.said, "|"); got != "tick" {
		t.Fatalf("said %q", got)
	}
	// Setting the tick to 0 starts the count again, and a stop ends it.
	if _, _, err := rt.FireNPCEvent(n, "watimerset0", 0); err != nil {
		t.Fatal(err)
	}
	moveTo(h, rt, 1900)
	if _, _, err := rt.FireNPCEvent(n, "watimerstop", 0); err != nil {
		t.Fatal(err)
	}
	moveTo(h, rt, 5000)
	if got := strings.Join(h.said, "|"); got != "tick" {
		t.Fatalf("said %q after a stop", got)
	}
	for _, m := range logs.msgs {
		t.Error(m)
	}
}

// TestRunTimerAttachedToPlayer runs a timer attached to the player
// (review decision 60, C3): its labels go out as events with the player,
// and the NPC's own timer stays idle.
func TestRunTimerAttachedToPlayer(t *testing.T) {
	h, rt, n, logs := timerRuntime(t, map[string]string{"Mine#t": `{
	end;
OnTimer1000:
	announce "tick", bc_all;
	end;
}`})
	if _, _, err := rt.FireNPCEvent(n, "watimerinit", 0); err != nil {
		t.Fatal(err)
	}
	moveTo(h, rt, 1000)
	if got := strings.Join(h.said, "|"); got != "tick" {
		t.Fatalf("said %q", got)
	}
	h.fired = nil
	if _, _, err := rt.FireNPCEvent(n, "watimerinit", 1); err != nil {
		t.Fatal(err)
	}
	// The attached timer is the player's: it sends its label to the NPC
	// with the player.
	moveTo(h, rt, 2000)
	moveTo(h, rt, 2500)
	got := h.fired
	if len(got) != 1 || got[0].label != "ontimer1000" || got[0].player != 1 {
		t.Fatalf("fired %+v", got)
	}
	for _, m := range logs.msgs {
		t.Error(m)
	}
}

// TestRunOnInitCallsubWaits checks an OnInit whose callsub target waits.
func TestRunOnInitCallsubWaits(t *testing.T) {
	h, rt, _, logs := timerRuntime(t, map[string]string{"Sub#t": `{
	end;
OnInit:
	callsub L_Setup;
	end;
L_Setup:
	announce "a", bc_all;
	sleep 1000;
	announce "b", bc_all;
	return;
}`})
	moveTo(h, rt, 0)
	moveTo(h, rt, 1000)
	if got := strings.Join(h.said, "|"); got != "a|b" {
		t.Fatalf("said %q", got)
	}
	for _, m := range logs.msgs {
		t.Error(m)
	}
}

// TestOnInitCallfuncWaitsRefused checks that an OnInit that calls a
// function which waits is refused: the function's wait has no
// conversation, and only OnInit's own waits are wrapped.
func TestOnInitCallfuncWaitsRefused(t *testing.T) {
	builtinFunctions["Nap"] = `{
	sleep 1000;
	return;
}`
	defer delete(builtinFunctions, "Nap")
	res := builtinResult(t, map[string]string{"Fn#t": `{
	end;
OnInit:
	callfunc "Nap";
	end;
}`})
	n := res.NPCs[len(res.NPCs)-1]
	if n.Status == Full || len(n.Unsupported) == 0 || !strings.Contains(n.Unsupported[0], "OnInit runs without a player") {
		t.Fatalf("%s %v", n.Status, n.Unsupported)
	}
}
