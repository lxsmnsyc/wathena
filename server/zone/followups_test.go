package zone

import (
	"log/slog"
	"strings"
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/social"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// followupScript runs the calls of review decision 59 on npc_event
// labels, and notes results in the variable seen.
const followupScript = `
local events = require "wa.events"
local maps = require "wa.maps"
local world = require "wa.world"
local spawning = require "wa.spawning"
local variables = require "wa.variables"
local context = require "wa.context"
local seen = variables.server_temporary_string("seen")
local function note(s) seen:set(seen:get() .. s .. ";") end
local kept
return function()
  local npc = context:get_instance()
  local function on(name, f) events.on("npc_event", f, { name = name }) end
  on("OnSprite", function() npc:set_sprite(1002, { keep = false }) end)
  on("OnSpriteJob", function() npc:set_sprite(7, { keep = false }) end)
  on("OnSpriteBad", function()
    local ok, err = pcall(npc.set_sprite, npc, 1002, { to = "area" })
    note("bad " .. tostring(ok) .. " " .. tostring(tostring(err):match("needs keep = false") ~= nil))
  end)
  on("OnMove", function() npc:move_to(24, 20, 2) end)
  on("OnWalk", function() note("walk " .. tostring(npc:walk_to(23, 20, { label = "OnArrive" }))) end)
  on("OnArrive", function(event) note("arrived " .. npc.x .. "," .. npc.y .. " " .. tostring(event.player)) end)
  on("OnWalkFar", function() npc:walk_to(35, 20) end)
  on("OnStop", function() npc:stop({ send_cell = false }) end)
  on("OnMusic", function() maps.play_music("test", "08", { rectangle = { { 0, 0 }, { 21, 21 } } }) end)
  on("OnWorldMusic", function() world.play_music("09") end)
  on("OnMonster", function()
    kept = spawning.monsters { monster = 1002, x = 30, y = 30 }[1]
    local found = maps.find_monster(kept.game_id)
    note("found " .. tostring(found == kept) .. " " .. tostring(maps.find_monster(kept.game_id + 1000)))
  end)
  local kept_id
  on("OnKillKept", function()
    kept_id = kept.game_id
    kept:kill()
  end)
  on("OnAfterKill", function() note("after " .. tostring(maps.find_monster(kept_id))) end)
  on("OnPartyWarp", function()
    local p = maps.find_player("a")
    p.party:warp("test", 10, 10, { range_x = 2, range_y = 2 })
  end)
  on("OnLeader", function() maps.find_player("a").party:warp("leader") end)
  on("OnSavePoint", function()
    local ok, err = pcall(function() maps.find_player("a").party:warp("save_point") end)
    note("save " .. tostring(ok) .. " " .. tostring(tostring(err):match("needs the player option") ~= nil))
  end)
  on("OnRandomWarp", function() maps.warp_players("test", "random", 0, 0) end)
  on("OnMapWarp", function() maps.warp_players("test", "test", 5, 6) end)
end`

// followupZone is mapsZone with followupScript.
func followupZone(t *testing.T) (*Zone, *Player, *entity) {
	t.Helper()
	z := New("map/test", open(40), rng.FromKey([32]byte{5}).Stream(), nil)
	z.Items = testItems()
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	logs := &syncBuffer{}
	st := &battle.Status{Level: 3, MaxHP: 30, Hit: 200, ADelay: 1000, AMotion: 500, DMotion: 300, Mode: 0x81, RHW: battle.Weapon{Atk: 10, Atk2: 10, Range: 1}}
	z.Mods = &Mods{Limits: testLimits, Scripts: []*mod.Scripts{npcMod(followupScript)}, IDs: []string{"core", "example.test"},
		Log: slog.New(slog.NewTextHandler(logs, nil)), Server: variables.NewServer(nil, nil), Temporary: variables.NewTemporary(),
		Monsters: map[uint32]Spawn{1002: {Mob: 1002, Name: "Poring", Amount: 1, Speed: 400, Status: st}}}
	z.SetNPCs([]NPC{{Name: "example.test:Guide", DisplayName: "Guide", X: 20, Y: 20, Sprite: 112, Script: "example.test:scripts/npc.lua"}})
	t.Cleanup(func() {
		z.stopScripts()
		if strings.Contains(logs.String(), "level=WARN") || strings.Contains(logs.String(), "level=ERROR") {
			t.Errorf("logs: %s", logs)
		}
	})
	p := add(z, "a")
	p.entity.x, p.entity.y = 21, 20
	z.stepAt(50)
	events(p)
	n := npcNamed(z, "Guide")
	if n == nil || n.npc.script == nil {
		t.Fatalf("the NPC did not start: %s", logs)
	}
	return z, p, n
}

// appearsOf are the appear events of an entity's ID.
func appearsOf(ev []*wathenav1.Event, id uint32) []*wathenav1.EntityAppear {
	var out []*wathenav1.EntityAppear
	for _, e := range ev {
		if a := e.GetAppear(); a != nil && a.GetId() == id {
			out = append(out, a)
		}
	}
	return out
}

// TestClassChangeNotKept checks npc:set_sprite with keep = false, as
// classchange (script.cpp 15163): the players who see the NPC see the
// view, the NPC keeps its own for players who come later, and a job's
// view shows nothing (clif.cpp 1477).
func TestClassChangeNotKept(t *testing.T) {
	z, p, n := followupZone(t)
	id, _ := p.netIDs.get(n)
	fire(t, z, n, "OnSprite")
	z.stepAt(z.now + 50)
	got := appearsOf(events(p), id)
	if len(got) != 1 || got[0].GetLook().GetBody() != 1002 {
		t.Fatalf("appears %v, want one with the monster's view", got)
	}
	if n.npc.Sprite != 112 {
		t.Errorf("the NPC kept %d", n.npc.Sprite)
	}
	q := add(z, "b")
	q.entity.x, q.entity.y = 22, 20
	z.stepAt(z.now + 50)
	qid, _ := q.netIDs.get(n)
	if got := appearsOf(events(q), qid); len(got) != 1 || got[0].GetLook().GetBody() != 112 {
		t.Errorf("a later player saw %v, want the NPC's own view", got)
	}
	fire(t, z, n, "OnSpriteJob")
	z.stepAt(z.now + 50)
	if got := appearsOf(events(p), id); len(got) != 0 {
		t.Errorf("a job's view showed %v", got)
	}
	fire(t, z, n, "OnSpriteBad")
	if got := seen(t, z); got != "bad false true;" {
		t.Errorf("got %q", got)
	}
}

// TestNPCMoveDirWalkLabelStop checks movenpc's direction (script.cpp
// 16161), unitwalk's label, which runs with no player when the walk ends
// on its cell (unit.cpp 650), and unitstopwalk's USW_NONE, which stops
// the NPC with no packet (unit.cpp 1725).
func TestNPCMoveDirWalkLabelStop(t *testing.T) {
	z, p, n := followupZone(t)
	fire(t, z, n, "OnMove")
	if n.x != 24 || n.y != 20 || n.dir != facing(2) {
		t.Fatalf("the NPC is at %d,%d facing %d", n.x, n.y, n.dir)
	}
	z.stepAt(z.now + 50)
	id, _ := p.netIDs.get(n)
	if got := appearsOf(events(p), id); len(got) != 1 || got[0].GetDir() != uint32(facing(2)) {
		t.Errorf("appears %v, want the NPC facing west", got)
	}
	fire(t, z, n, "OnWalk")
	for range 40 {
		z.stepAt(z.now + 50)
		z.runScripts()
	}
	if got := seen(t, z); got != "walk true;arrived 23,20 nil;" {
		t.Fatalf("got %q", got)
	}
	events(p)
	fire(t, z, n, "OnWalkFar")
	z.stepAt(z.now + 250)
	events(p)
	fire(t, z, n, "OnStop")
	at := n.x
	z.stepAt(z.now + 50)
	for _, ev := range events(p) {
		if ev.GetStop() != nil {
			t.Errorf("the stop sent the cell: %v", ev)
		}
	}
	z.stepAt(z.now + 1000)
	if n.x != at || len(n.path) != 0 {
		t.Errorf("the NPC walked on to %d", n.x)
	}
}

// TestSoundsAndMusicOfDecision59 checks player:play_sound's to = "area",
// as soundeffectall around the attached player (script.cpp 15315), and
// playBGMall of a rectangle and of the world (script.cpp 15253).
func TestSoundsAndMusicOfDecision59(t *testing.T) {
	z, p, n := followupZone(t)
	q := add(z, "b")
	q.entity.x, q.entity.y = 30, 30
	far := add(z, "c")
	far.entity.x, far.entity.y = 5, 5
	z.stepAt(z.now + 50)
	events(p)
	events(q)
	events(far)
	h := scriptHost{z}
	if err := h.PlaySound(modscript.Entity{Kind: "player", ID: int64(p.charID)}, "", "effect/x", "area", false); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	from, _ := q.netIDs.get(p.entity)
	sounds := func(pl *Player) []*wathenav1.SoundPlay {
		var out []*wathenav1.SoundPlay
		for _, ev := range events(pl) {
			if s := ev.GetSound(); s != nil {
				out = append(out, s)
			}
		}
		return out
	}
	if got := sounds(q); len(got) != 1 || got[0].GetEntity() != from {
		t.Errorf("a player who sees a got %v, want the sound from a", got)
	}
	if got := sounds(far); len(got) != 0 {
		t.Errorf("a player out of sight got %v", got)
	}
	music := func(pl *Player) []string {
		var out []string
		for _, ev := range events(pl) {
			if m := ev.GetMusic(); m != nil {
				out = append(out, m.GetName())
			}
		}
		return out
	}
	events(p)
	fire(t, z, n, "OnMusic")
	z.stepAt(z.now + 50)
	if a, b, c := music(p), music(q), music(far); len(a) != 1 || len(b) != 0 || len(c) != 1 {
		t.Errorf("the rectangle's music went to %v %v %v", a, b, c)
	}
	fire(t, z, n, "OnWorldMusic")
	z.stepAt(z.now + 50)
	if a, b, c := music(p), music(q), music(far); len(a) != 1 || len(b) != 1 || len(c) != 1 || a[0] != "music/09" {
		t.Errorf("the world's music went to %v %v %v", a, b, c)
	}
}

// TestMonsterGameIDs checks monster.game_id and maps.find_monster, as
// rAthena's GID and map_id2md: a living monster is found, and a dead one
// is not.
func TestMonsterGameIDs(t *testing.T) {
	z, _, n := followupZone(t)
	fire(t, z, n, "OnMonster")
	if got := seen(t, z); got != "found true nil;" {
		t.Fatalf("got %q", got)
	}
	fire(t, z, n, "OnKillKept")
	fire(t, z, n, "OnAfterKill")
	if got := seen(t, z); got != "after nil;" {
		t.Fatalf("got %q", got)
	}
}

// TestWarpsOfDecision59 checks areawarp's "Random" (script.cpp 5670),
// that map warps move dead players (buildin_areawarp_sub and mapwarp
// check no pc_isdead, script.cpp 5659 and 14340), that warpparty skips
// them (script.cpp 5870), its range and its Leader target.
func TestWarpsOfDecision59(t *testing.T) {
	z, p, n := followupZone(t)
	q := add(z, "b")
	q.entity.x, q.entity.y = 30, 30
	formParty(t, z, social.Options{}, p, q)
	// A dead member stays on party:warp.
	z.die(q.entity, nil)
	fire(t, z, n, "OnPartyWarp")
	z.stepAt(z.now + 50)
	if e := p.entity; e.x < 8 || e.x > 12 || e.y < 8 || e.y > 12 {
		t.Errorf("the living member is at %d,%d, want within 2 of 10,10", e.x, e.y)
	}
	if q.entity.x != 30 || q.entity.y != 30 {
		t.Errorf("the dead member moved to %d,%d", q.entity.x, q.entity.y)
	}
	// mapwarp moves the dead too, who stay dead.
	fire(t, z, n, "OnMapWarp")
	z.stepAt(z.now + 50)
	if q.entity.x != 5 || q.entity.y != 6 || !q.entity.dead {
		t.Errorf("the dead member is at %d,%d, dead %v, want 5,6 and dead", q.entity.x, q.entity.y, q.entity.dead)
	}
	// After its wait, it returns to its save point alive.
	z.stepAt(z.now + reviveMs + 50)
	if q.entity.dead {
		t.Error("the dead member did not come back")
	}
	// Leader: every member but the leader goes to the leader's cell.
	p.entity.x, p.entity.y = 15, 16
	fire(t, z, n, "OnLeader")
	z.stepAt(z.now + 50)
	if q.entity.x != 15 || q.entity.y != 16 {
		t.Errorf("the member is at %d,%d, want the leader's 15,16", q.entity.x, q.entity.y)
	}
	fire(t, z, n, "OnSavePoint")
	if got := seen(t, z); got != "save false true;" {
		t.Errorf("got %q", got)
	}
	// "random" moves each player to a random cell of its own map.
	fire(t, z, n, "OnRandomWarp")
	z.stepAt(z.now + 50)
	if p.entity.x == 15 && p.entity.y == 16 && q.entity.x == 15 && q.entity.y == 16 {
		t.Error("random moved nobody")
	}
}

// TestRefineFailureEffect checks equipped:destroy with refine_failure,
// as failedrefitem (script.cpp 9802): the copy goes, and the wearer
// shows REFINE_FAILURE.
func TestRefineFailureEffect(t *testing.T) {
	z, a, _ := equipZone(t)
	h := scriptHost{z}
	slot := give(t, z, a, 1101, 1)
	equip(z, a, slot)
	a.inventory.Slots[slot].Refine = 4
	handle, ok, err := h.Equipped(int64(a.charID), "right_hand")
	if err != nil || !ok {
		t.Fatalf("not worn: %v", err)
	}
	z.effects = z.effects[:0]
	if err := h.EquippedDestroy(handle, true); err != nil || a.inventory.Slots[slot].ID != 0 {
		t.Fatalf("destroy: %v", err)
	}
	found := false
	for _, ef := range z.effects {
		found = found || ef.e == a.entity && ef.id == effectRefineFail
	}
	if !found {
		t.Error("no refine failure effect")
	}
}

// followWorld is presentWorld with the world's player directory.
type followWorld struct {
	*presentWorld
	players *PlayerDirectory
}

func (w *followWorld) Players() *PlayerDirectory { return w.players }

func newFollowWorld(zones ...*Zone) *followWorld {
	w := &followWorld{presentWorld: newPresentWorld(zones...), players: NewPlayerDirectory()}
	for _, z := range zones {
		z.World = w
	}
	return w
}

// hunterScript spawns monsters with its label on map b, reads one from
// map a, acts on another, and notes each label's run.
const hunterScript = `
local events = require "wa.events"
local maps = require "wa.maps"
local spawning = require "wa.spawning"
local variables = require "wa.variables"
local seen = variables.server_temporary_string("seen")
local function note(s) seen:set(seen:get() .. s .. ";") end
local list
return function()
  local function on(name, f) events.on("npc_event", f, { name = name }) end
  on("OnSpawnB", function()
    list = spawning.monsters { map = "b", monster = 1002, amount = 2, x = 10, y = 10, label = "OnMyMobDead" }
    note("spawned " .. #list .. " " .. list[1].map)
  end)
  on("OnRead", function()
    local m = list[1]
    note("read " .. m.name .. " " .. m.x .. "," .. m.y .. " " .. tostring(m.is_alive) .. " " .. tostring(maps.find_monster(m.game_id) == m))
    note("entities " .. #maps.find_entities { map = "b", x = 10, y = 10, radius = 3, kinds = { "monster", "player" } })
    note("hp " .. tostring(list[2]:set_hp(5)))
    local ok = pcall(function() return m.target end)
    note("target " .. tostring(ok))
  end)
  on("OnMyMobDead", function(event)
    local who = event.player and (event.player.name .. "@" .. event.player.map) or "-"
    note("dead " .. who .. " " .. tostring(event.conversation) .. " " .. maps.count_monsters("b", { label = "OnMyMobDead" }))
  end)
end`

// TestMonstersAndLabelsAcrossZones checks review decision 59 between
// two zones: a spawn on another map returns handles at once, as monster
// fills $@mobid; another zone reads the monsters from what their zone
// shares, finds them by game ID and with maps.find_entities, and acts on
// them by message; and a monster label of an NPC of another map runs
// with the player that killed it, as mob_dead's npc_event (mob.cpp
// 3600), without a conversation.
func TestMonstersAndLabelsAcrossZones(t *testing.T) {
	a, _ := presentZone(t, "a", hunterScript, NPC{Name: "example.test:Hunter", DisplayName: "Hunter", Script: "example.test:scripts/npc.lua", Floating: true})
	b, _ := presentZone(t, "b", hunterScript)
	b.Mods = a.Mods
	st := &battle.Status{Level: 3, MaxHP: 30, Hit: 200, ADelay: 1000, AMotion: 500, DMotion: 300, Mode: 0x81, RHW: battle.Weapon{Atk: 10, Atk2: 10, Range: 1}}
	a.Mods.Monsters = map[uint32]Spawn{1002: {Mob: 1002, Name: "Poring", Amount: 1, Speed: 400, Status: st}}
	newFollowWorld(a, b)
	k := joinAt(b, "k", 10, 12)
	a.startNPCs()
	hunter := npcNamed(a, "Hunter")
	step := func() {
		for range 3 {
			a.runScripts()
			b.runScripts()
		}
		b.stepAt(b.now + 50)
	}
	fire(t, a, hunter, "OnSpawnB")
	step()
	if got := seen(t, a); got != "spawned 2 b;" {
		t.Fatalf("got %q", got)
	}
	fire(t, a, hunter, "OnRead")
	step()
	if got := seen(t, a); got != "read Poring 10,10 true true;entities 3;hp true;target false;" {
		t.Fatalf("got %q", got)
	}
	mobs := monstersOf(b, 1002)
	if len(mobs) != 2 || mobs[1].hp != 5 {
		t.Fatalf("monsters %d, the second's HP %d, want 5", len(mobs), mobs[1].hp)
	}
	b.die(mobs[0], k.entity)
	step()
	if got := seen(t, a); got != "dead k@b nil 1;" {
		t.Fatalf("got %q", got)
	}
}

// TestMapsControlPermission checks review decision 59's maps.control: a
// mod without it cannot kill a map's monsters or change its cells, and
// npc.create no longer allows them.
func TestMapsControlPermission(t *testing.T) {
	z, _, _ := followupZone(t)
	grants := mod.Grants{}
	for k, v := range z.Mods.Scripts[0].Grants {
		grants[k] = v
	}
	delete(grants, "maps.control")
	src := `
local maps = require "wa.maps"
local variables = require "wa.variables"
local seen = variables.server_temporary_string("seen")
return function()
  local ok1, e1 = pcall(maps.remove_monsters, "test")
  local ok2, e2 = pcall(maps.set_cells, "test", 1, 1, 2, 2, "walkable", false)
  seen:set(tostring(ok1) .. " " .. tostring(tostring(e1):match("needs the permission maps.control") ~= nil) .. " " ..
    tostring(ok2) .. " " .. tostring(tostring(e2):match("needs the permission maps.control") ~= nil))
end`
	sc := &mod.Scripts{Mod: "example.test", Files: map[string]string{"scripts/npc.lua": src}, Grants: grants, Visible: []string{"example.test"},
		Entries: []string{"scripts/npc.lua"}}
	if err := z.scripts.Reload(sc); err != nil {
		t.Fatal(err)
	}
	if got := seen(t, z); got != "false true false true" {
		t.Fatalf("got %q", got)
	}
}

// TestDeadPlayerWarpsToAnotherMap checks that a map warp of a dead
// player to another map keeps it dead, as pc_setpos keeps pc_isdead,
// and that it returns to its save point after the rest of its wait.
func TestDeadPlayerWarpsToAnotherMap(t *testing.T) {
	a, b := twoMaps()
	p := onMap(a, "Fallen", 10, 10, &saves{})
	a.stepAt(50)
	a.die(p.entity, nil)
	a.stepAt(a.now + 1000)
	a.scriptWarpDead(p, b, 7, 8)
	q := arrive(t, b)
	if !q.entity.dead || q.entity.hp != 0 || q.entity.x != 7 || q.entity.y != 8 {
		t.Fatalf("arrived at %d,%d, dead %v with %d HP, want dead at 7,8", q.entity.x, q.entity.y, q.entity.dead, q.entity.hp)
	}
	b.stepAt(b.now + reviveMs - 1000 - 100)
	if !q.entity.dead {
		t.Fatal("it came back before the rest of its wait")
	}
}
