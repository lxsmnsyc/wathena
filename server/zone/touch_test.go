package zone

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mod"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/shared/rng"
	"github.com/lxsmnsyc/wathena/tools/convert"
)

// toucher is an NPC script that says who touched it, and when someone
// left its area.
const toucher = `
local context = require "wa.context"
local events = require "wa.events"
return function()
  local npc = context:get_instance()
  events.on("touch", function(player) npc:say_aloud("touch " .. player.name) end)
  events.on("untouch", function(player) npc:say_aloud("untouch") end)
end`

// touchZone is a zone with an NPC at 20,20 whose touch area reaches one
// cell, with the script src of the mod example.test, and a watcher at
// 25,25 who hears the NPC.
func touchZone(t *testing.T, src string) (*Zone, *Player) {
	t.Helper()
	z := New("map/test", open(40), rng.FromKey([32]byte{5}).Stream(), nil)
	z.Items = testItems()
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	logs := &syncBuffer{}
	z.Mods = &Mods{Limits: testLimits, Scripts: []*mod.Scripts{npcMod(src)}, IDs: []string{"core", "example.test"},
		Log: slog.New(slog.NewTextHandler(logs, nil)), Server: variables.NewServer(nil, nil)}
	z.SetNPCs([]NPC{{Name: "example.test:Gate", DisplayName: "Gate", X: 20, Y: 20, Sprite: 45, Script: "example.test:scripts/npc.lua",
		Touch: &TouchArea{XSpan: 1, YSpan: 1}}})
	t.Cleanup(func() {
		z.stopScripts()
		if strings.Contains(logs.String(), "level=WARN") || strings.Contains(logs.String(), "level=ERROR") {
			t.Errorf("logs: %s", logs)
		}
	})
	w := add(z, "watcher")
	w.entity.x, w.entity.y = 25, 25
	z.stepAt(50)
	events(w)
	return z, w
}

// heard is what the NPC said that a player heard.
func heard(p *Player) []string {
	var out []string
	for _, e := range events(p) {
		if c := e.GetChat(); c != nil && (strings.HasPrefix(c.GetText(), "touch") || strings.HasPrefix(c.GetText(), "untouch")) {
			out = append(out, c.GetText())
		}
	}
	return out
}

// walkTo walks a player to a cell, and steps until it stands there.
func walkTo(t *testing.T, z *Zone, p *Player, x, y int) {
	t.Helper()
	z.walk(p.entity, x, y)
	for now := z.now + 50; now <= z.now+10_000; now += 50 {
		z.stepAt(now)
		if p.entity.x == x && p.entity.y == y && len(p.entity.path) == 0 {
			z.stepAt(now + 50)
			return
		}
	}
	t.Fatalf("%s did not reach %d,%d: at %d,%d", p.Name, x, y, p.entity.x, p.entity.y)
}

// joinAt adds a player on a cell, past its first frame.
func joinAt(z *Zone, name string, x, y int) *Player {
	p := add(z, name)
	p.entity.x, p.entity.y = x, y
	z.stepAt(z.now + 50)
	z.stepAt(z.now + 50)
	return p
}

// TestTouchAreaSteps checks the touch and untouch events of a walk, as
// npc_touch_area_allnpc and the player's areanpc list: touch once on the
// step into the area, nothing for steps inside it, and untouch on the
// step out of it.
func TestTouchAreaSteps(t *testing.T) {
	z, w := touchZone(t, toucher)
	a := joinAt(z, "a", 10, 20)
	if got := heard(w); len(got) != 0 {
		t.Fatalf("before the walk: %q", got)
	}
	walkTo(t, z, a, 19, 20)
	if got := strings.Join(heard(w), ","); got != "touch a" {
		t.Fatalf("into the area: %q", got)
	}
	walkTo(t, z, a, 21, 21)
	if got := heard(w); len(got) != 0 {
		t.Fatalf("inside the area: %q", got)
	}
	walkTo(t, z, a, 24, 21)
	if got := strings.Join(heard(w), ","); got != "untouch" {
		t.Fatalf("out of the area: %q", got)
	}
	walkTo(t, z, a, 21, 19)
	if got := strings.Join(heard(w), ","); got != "touch a" {
		t.Fatalf("back into the area: %q", got)
	}
	// A logout in the area leaves it.
	z.leave(a, wathenav1.VanishReason_VANISH_REASON_LOGGED_OUT)
	z.stepAt(z.now + 50)
	if got := strings.Join(heard(w), ","); got != "untouch" {
		t.Fatalf("after the logout: %q", got)
	}
}

// TestTouchAreaArrival checks clif_parse_LoadEndAck: a player who
// arrives on a cell of the area touches it once its first frame went
// out, and a teleport out of the area leaves it.
func TestTouchAreaArrival(t *testing.T) {
	z, w := touchZone(t, toucher)
	a := add(z, "a")
	a.entity.x, a.entity.y = 21, 21
	z.stepAt(z.now + 50)
	z.stepAt(z.now + 50)
	if got := strings.Join(heard(w), ","); got != "touch a" {
		t.Fatalf("arrival: %q", got)
	}
	z.teleport(a, 5, 5)
	for range 3 {
		z.stepAt(z.now + 50)
	}
	if got := strings.Join(heard(w), ","); got != "untouch" {
		t.Fatalf("teleport out: %q", got)
	}
	z.teleport(a, 20, 19)
	for range 3 {
		z.stepAt(z.now + 50)
	}
	if got := strings.Join(heard(w), ","); got != "touch a" {
		t.Fatalf("teleport in: %q", got)
	}
}

// TestTouchAreaHiddenAndWarps checks how rAthena treats hidden players
// and warps: OnTouch fires for a hidden player (npc_ontouch2_event does
// not check hiding), and a step onto a warp that takes the player checks
// no touch area, since warps come first.
func TestTouchAreaHiddenAndWarps(t *testing.T) {
	z, w := touchZone(t, toucher)
	a := joinAt(z, "a", 10, 20)
	a.entity.sc = sc.NewList()
	a.entity.sc.Option |= sc.OptionHide
	walkTo(t, z, a, 19, 21)
	if got := strings.Join(heard(w), ","); got != "touch a" {
		t.Fatalf("hidden player: %q", got)
	}
	// A warp needs a session to take the player.
	z.SetWarps([]Warp{{Name: "w", X: 21, Y: 21, To: "map/test", ToX: 5, ToY: 5}})
	c := onMap(z, "c", 24, 21, &saves{})
	z.stepAt(z.now + 50)
	z.stepAt(z.now + 50)
	heard(w)
	z.walk(c.entity, 21, 21)
	for range 40 {
		z.stepAt(z.now + 50)
	}
	if got := heard(w); len(got) != 0 {
		t.Fatalf("a warp in the area touched it: %q", got)
	}
	if c.entity.x != 5 || c.entity.y != 5 {
		t.Fatalf("the warp did not take the player: %d,%d", c.entity.x, c.entity.y)
	}
}

// convertedTouch converts an rAthena NPC with an OnTouch_ label, as the
// warps folders have them, and returns its Lua file.
func convertedTouch(t *testing.T, script string) string {
	t.Helper()
	read := func(name string) []byte {
		b, err := reference.ReadFile("extracts/renewal/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	data, err := json.Marshal(map[string]any{"mode": "renewal", "npcs": []map[string]any{{
		"kind": "script", "name": "Gate#t", "display": "Gate#t", "file": "test.txt", "line": 1, "map": "test", "x": 20, "y": 20,
		"sprite": "WARPNPC", "xs": 1, "ys": 1, "state": "enabled", "script": script,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := convert.ConvertNPCs(convert.NPCInput{NPCs: bytes.NewReader(data), Constants: bytes.NewReader(read("script_constants")),
		ClientTables: bytes.NewReader(read("client_tables"))})
	if err != nil {
		t.Fatal(err)
	}
	n := res.NPCs[0]
	if !n.Touch || n.Lua == "" {
		t.Fatalf("not converted: %v", n.Unsupported)
	}
	return n.Lua
}

// TestTouchOneConverted runs a converted OnTouch_ label in the zone, with
// core's R.touch_one: it runs for one player in the area at a time
// (touching_id), not for a hiding player, and for the next player in
// the area when that player leaves (npc_touchnext_areanpc).
func TestTouchOneConverted(t *testing.T) {
	lua := convertedTouch(t, `{ end; OnTouch_: npctalk "touch " + strcharinfo(0); end; }`)
	z, w := touchZone(t, "return function() end")
	z.Statuses = testStatuses(t)
	coreMods(t, z)
	z.Mods.Log = slog.New(slog.DiscardHandler)
	z.Mods.Server = variables.NewServer(nil, nil)
	z.Mods.Scripts[0].Files["scripts/npcs/Gate_t.lua"] = lua
	z.stopScripts()
	z.npcsStarted = false
	z.SetNPCs([]NPC{{Name: "core:Gate#t", DisplayName: "Gate", X: 20, Y: 20, Sprite: 45, Script: "core:scripts/npcs/Gate_t.lua",
		Touch: &TouchArea{XSpan: 1, YSpan: 1}}})
	z.startScripts()
	z.startNPCs()
	a := joinAt(z, "a", 10, 20)
	b := joinAt(z, "b", 10, 21)
	walkTo(t, z, a, 19, 20)
	if got := strings.Join(heard(w), ","); got != "touch a" {
		t.Fatalf("first player: %q", got)
	}
	walkTo(t, z, b, 19, 21)
	if got := heard(w); len(got) != 0 {
		t.Fatalf("second player while the first is in: %q", got)
	}
	walkTo(t, z, a, 10, 20)
	if got := strings.Join(heard(w), ","); got != "touch b" {
		t.Fatalf("after the first player left: %q", got)
	}
	// A hiding player does not start it, and stays in the area. Hiding
	// stops walks, so the player arrives in the area hidden.
	walkTo(t, z, b, 10, 21)
	heard(w)
	c := add(z, "c")
	c.entity.x, c.entity.y = 20, 21
	z.statusEngine().Start(z.unit(c.entity), z.unit(c.entity), sc.Start{Type: sc.SCHiding, Rate: 10000, Val1: 10,
		Duration: 600_000, Flag: sc.NoAvoid, Reason: sc.ReasonScript})
	if !c.entity.sc.Has(sc.SCHiding) {
		t.Fatal("no hiding")
	}
	for range 3 {
		z.stepAt(z.now + 50)
	}
	if got := heard(w); len(got) != 0 {
		t.Fatalf("a hiding player started it: %q", got)
	}
	walkTo(t, z, b, 19, 21)
	if got := strings.Join(heard(w), ","); got != "touch b" {
		t.Fatalf("with a hiding player in the area: %q", got)
	}
}
