package convert

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// npcHost is testHost with NPCs: one NPC handle per placed NPC, a player
// with zeny, the last step each conversation waits on, and the monsters
// scripts spawn.
type npcHost struct {
	*testHost
	npcs  map[int64]*NPC
	steps map[int64]modscript.Step
	ended map[int64]bool
	vars  map[modscript.VariableKey]string
	zeny  int64
	// spawned are the monsters that have not died yet, by handle ID.
	spawned     map[int64]uint32
	nextMonster int64
}

func (h *npcHost) SpawnMonsters(_ int64, s modscript.SpawnSpec) ([]int64, error) {
	if h.spawned == nil {
		h.spawned = map[int64]uint32{}
	}
	ids := make([]int64, s.Amount)
	for i := range ids {
		h.nextMonster++
		ids[i] = h.nextMonster
		h.spawned[ids[i]] = s.Monster
	}
	return ids, nil
}
func (h *npcHost) Despawn(int64) {}

// testClock is the clock of the NPC runtime: schedules start from it.
var testClock = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func (h *npcHost) PlayerField(id int64, field string) (script.Value, error) {
	if field == "zeny" {
		return script.Int(h.zeny), nil
	}
	return h.testHost.PlayerField(id, field)
}

func (h *npcHost) NPCField(id int64, field string) (script.Value, error) {
	n := h.npcs[id]
	switch field {
	case "name":
		return script.String(displayName(n.Source)), nil
	case "map":
		return script.String(n.Map), nil
	case "x", "y":
		return script.Int(1), nil
	}
	return script.Nil, fmt.Errorf("npc has no field %q", field)
}
func (h *npcHost) NPCEmotion(int64, int64) error { return nil }
func (h *npcHost) NPCSay(int64, string) error    { return nil }
func (h *npcHost) Converse(c *modscript.Conversation, s modscript.Step) error {
	if s.Kind.Waits() {
		h.steps[c.ID] = s
	}
	return nil
}
func (h *npcHost) ConversationEnded(c *modscript.Conversation) { h.ended[c.ID] = true }
func (h *npcHost) PayZeny(_, amount int64) (bool, error) {
	if h.zeny < amount {
		return false, nil
	}
	h.zeny -= amount
	return true, nil
}
func (h *npcHost) GiveZeny(_, amount int64) error                         { h.zeny += amount; return nil }
func (h *npcHost) TakeItem(int64, uint32, int) (bool, error)              { return true, nil }
func (h *npcHost) CanCarry(int64, uint32, int) (bool, error)              { return true, nil }
func (h *npcHost) SetSavePoint(int64, string, int, int) (bool, error)     { return true, nil }
func (h *npcHost) MarkMinimap(int64, int64, int, int, uint32, bool) error { return nil }
func (h *npcHost) OpenShop(int64, string) error                           { return nil }
func (h *npcHost) Variable(_ int64, v modscript.VariableKey) (string, bool, error) {
	s, ok := h.vars[v]
	return s, ok, nil
}
func (h *npcHost) SetVariable(_ int64, v modscript.VariableKey, value string) error {
	if value == "" {
		delete(h.vars, v)
	} else {
		h.vars[v] = value
	}
	return nil
}
func (h *npcHost) ItemName(id uint32) (string, bool) { return fmt.Sprintf("Item %d", id), true }
func (h *npcHost) Announce(string, string) error     { return nil }

// logRecorder keeps the warnings a runtime logs, which are the errors of
// its handlers.
type logRecorder struct {
	mu   sync.Mutex
	msgs []string
}

func (l *logRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (l *logRecorder) Handle(_ context.Context, r slog.Record) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	var err string
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "err" {
			err = a.Value.String()
		}
		return true
	})
	l.msgs = append(l.msgs, r.Message+": "+err)
	return nil
}
func (l *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return l }
func (l *logRecorder) WithGroup(string) slog.Handler      { return l }

// TestRunConvertedNPCs starts every placed NPC of both modes and talks to
// it four times: answering each menu with its first option, its second,
// its third, and its last one, each number with its minimum, and text
// with "abc". Then every monster the NPCs spawned dies, killed by the
// player, and every schedule runs once. A part of a function that did
// not convert may fail when reached. Any other error fails the test.
func TestRunConvertedNPCs(t *testing.T) {
	testrace.SkipUnderRace(t, "it converts or runs all of core's scripts on one goroutine")
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			r, base, core := testRuntime(t, mode)
			_ = r
			logs := &logRecorder{}
			h := &npcHost{testHost: base, npcs: map[int64]*NPC{}, steps: map[int64]modscript.Step{}, ended: map[int64]bool{},
				vars: map[modscript.VariableKey]string{}, zeny: 1_000_000}
			rt := npcRuntime(t, core, h, logs)
			var started, clicks, steps int
			for i, n := range core.NPCs.NPCs {
				if !n.Placed || n.Script == "" {
					continue
				}
				id := int64(i + 1)
				h.npcs[id] = n
				npc, err := rt.StartNPC(id, "core:"+n.Script)
				if err != nil {
					t.Errorf("%s: %v", n.Name, err)
					continue
				}
				started++
				if !npc.Clickable() {
					continue
				}
				for _, pick := range []int{1, 2, 3, -1} {
					clicks++
					conv, err := rt.Click(npc, 1)
					if err != nil {
						t.Errorf("%s: click: %v", n.Name, err)
						break
					}
					for range 300 {
						if conv == nil || conv.Done {
							break
						}
						steps++
						s := h.steps[conv.ID]
						var answer script.Value
						switch s.Kind {
						case modscript.StepMenu:
							// Empty options are hidden, so the zone
							// refuses them.
							var shown []int
							for i, o := range s.Options {
								if o != "" {
									shown = append(shown, i+1)
								}
							}
							k := pick
							if k < 0 || k > len(shown) {
								k = len(shown)
							}
							answer = script.Int(int64(shown[k-1]))
						case modscript.StepNumber:
							answer = script.Int(s.Min)
						case modscript.StepText:
							answer = script.String("abc")
						}
						rt.ResumeConversation(conv, answer)
					}
					if conv != nil && !conv.Done {
						rt.CloseConversation(conv)
					}
				}
			}
			// Each spawned monster dies, killed by the player, and each
			// schedule runs once.
			deaths := 0
			for len(h.spawned) > 0 && deaths < 10_000 {
				for _, id := range sortedIDs(h.spawned) {
					mob := h.spawned[id]
					delete(h.spawned, id)
					deaths++
					rt.FireDeath(modscript.Death{Victim: id, VictimKind: "monster", Monster: int64(mob), Killer: 1, KillerKind: "player"})
				}
			}
			rt.RunSchedules(testClock.Add(8 * 24 * time.Hour))
			bad := 0
			for _, m := range logs.msgs {
				if strings.Contains(m, "which wAthena does not have yet") {
					continue
				}
				bad++
				if bad <= 20 {
					t.Error(m)
				}
			}
			t.Logf("%d NPCs started, %d clicks, %d steps, %d monster deaths, %d errors", started, clicks, steps, deaths, bad)
		})
	}
}

// npcRuntime makes a runtime with core's files and an NPC host.
func npcRuntime(t *testing.T, core *Core, h *npcHost, logs *logRecorder) *modscript.Runtime {
	t.Helper()
	sc := coreScripts(t, core)
	r, err := modscript.New(modscript.Options{Host: h, Mods: []string{"core"}, Constants: core.Constants, Jobs: h.jobs,
		Log: slog.New(logs), Server: variables.NewServer(nil, nil),
		Clock: func() time.Time { return testClock }, Location: time.UTC}, sc)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(r.Close)
	return r
}

func sortedIDs(m map[int64]uint32) []int64 {
	out := make([]int64, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}
