package zone

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/content"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestSpawnClearCells: a stored spawn line places its monsters in its
// area, never within its clear cells, and keeps them clear when they
// come back (decision 0042).
func TestSpawnClearCells(t *testing.T) {
	spawns := []Spawn{
		{Mob: 1002, Name: "Poring", Amount: 150, RespawnMs: 1000, Avoid: []Clear{{X: 20, Y: 20, R: 15}, {X: 55, Y: 5, R: 4}}},
		{Mob: 1004, Name: "Hornet", Amount: 30, X: 30, Y: 30, W: 8, H: 8, RespawnMs: 1000, Avoid: []Clear{{X: 30, Y: 30, R: 3}}},
	}
	z := New("map/test", open(60), rng.FromKey([32]byte{7}).Stream(), spawns)
	counts := map[uint32]int{}
	for e := range z.entities.all() {
		if e.monster == nil {
			continue
		}
		counts[e.monster.spawn.Mob]++
		for _, c := range e.monster.spawn.Avoid {
			if max(abs(e.x-c.X), abs(e.y-c.Y)) <= c.R {
				t.Errorf("%s at %d,%d is within %d of %d,%d", e.monster.spawn.Name, e.x, e.y, c.R, c.X, c.Y)
			}
		}
		if s := e.monster.spawn; s.Mob == 1004 && (abs(e.x-30) > 8 || abs(e.y-30) > 8) {
			t.Errorf("a Hornet at %d,%d is outside its area", e.x, e.y)
		}
	}
	if counts[1002] != 150 || counts[1004] != 30 {
		t.Errorf("counts %v", counts)
	}
	// A line whose clear cells cover the whole map places nothing.
	z = New("map/test", open(10), rng.FromKey([32]byte{7}).Stream(), []Spawn{{Mob: 1002, Amount: 3, Avoid: []Clear{{X: 5, Y: 5, R: 10}}}})
	if z.entities.len() != 0 {
		t.Errorf("%d monsters on a cleared map", z.entities.len())
	}
}

// TestSpawnHooksSecretStream: the spawn_table hooks draw from the secret
// ChaCha8 stream of their node, not the xoshiro stream whose state is
// the node's key (review finding L36).
func TestSpawnHooksSecretStream(t *testing.T) {
	seed := [32]byte{9, 8, 7}
	got, want := spawnHooksStream(seed), rng.FromKey(seed).SecretStream()
	for i := range 4 {
		if got, w := got.Uint64(), want.Uint64(); got != w {
			t.Fatalf("draw %d: %#x, want the secret stream's %#x", i, got, w)
		}
	}
}

// TestMonstersGetInHooksAndLayers checks that monsters.get works in a
// spawn_table hook and in a generation layer, which run in the spawn
// hooks' state, a zone with no map (review decision 62, C1).
func TestMonstersGetInHooksAndLayers(t *testing.T) {
	src := `
local hooks = require "wa.hooks"
local generation = require "wa.generation"
local monsters = require "wa.monsters"
return function()
  hooks.on("spawn_table", function(map, list, next)
    local m = monsters.get(1002)
    if m == nil or m.name ~= "Poring" or m.level ~= 3 then error("monsters.get in the hook gave " .. tostring(m)) end
    return { { monster = 1002, count = m.level, respawn = 5000, role = "common" } }
  end)
  generation.on("map", function(map, node)
    local m = monsters.get(1002)
    if m == nil or m.level ~= 3 then error("monsters.get in the layer gave " .. tostring(m)) end
    return { bosses = { { monster = 1002, role = "mini_boss", x = m.level, y = 4 } } }
  end)
end`
	sc := npcMod(src)
	sc.Entries = []string{"scripts/npc.lua"}
	logs := &syncBuffer{}
	st := &battle.Status{Level: 3, MaxHP: 30}
	mods := &Mods{Limits: testLimits, Scripts: []*mod.Scripts{sc}, IDs: []string{"core", "example.test"},
		Log:      slog.New(slog.NewTextHandler(logs, nil)),
		Monsters: map[uint32]Spawn{1002: {Mob: 1002, Name: "Poring", Amount: 1, Status: st}}}
	h, err := NewSpawnHooks(mods, [32]byte{3})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	idx := SpawnIndex{IDs: map[string]int64{"PORING": 1002}, Names: map[int64]string{1002: "PORING"},
		Monsters: map[string]spawns.Monster{"PORING": {}}}
	out := h.Run(spawns.Map{ID: "gen:field:0:0", Kind: "field"}, []spawns.Spawn{{Monster: "PORING", Count: 9, RespawnMs: 5000, Role: spawns.Common}}, idx, nil)
	if len(out) != 1 || out[0].Count != 3 {
		t.Errorf("hook result %+v, logs: %s", out, logs)
	}
	secret, _ := rng.NewRoot([]byte("monsters-get"))
	res, notes := h.Layers(secret, idx).Map(content.LayerMap{ID: "gen:field:0:0", Kind: gen.KindField, Width: 64, Height: 48})
	if len(res) != 1 || len(res[0].Bosses) != 1 || res[0].Bosses[0].X != 3 {
		t.Errorf("layer result %+v, notes %v, logs: %s", res, notes, logs)
	}
	if strings.Contains(logs.String(), "level=ERROR") || strings.Contains(logs.String(), "level=WARN") {
		t.Errorf("logs: %s", logs)
	}
}
