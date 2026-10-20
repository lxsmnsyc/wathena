package zone

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/status"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
	"github.com/lxsmnsyc/wathena/server/variables"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// presentZone is a zone of map/<name> whose NPCs run src, with every
// permission. Its NPCs start with the first player.
func presentZone(t *testing.T, name, src string, npcs ...NPC) (*Zone, *syncBuffer) {
	t.Helper()
	z := New("map/"+name, open(40), rng.FromKey([32]byte{7}).Stream(), nil)
	z.Items = testItems()
	z.Progress = testTables()
	z.PlayerStatus = battle.Status{MaxHP: 200, MaxSP: 50, Hit: 200, BAtk: 5, ADelay: 600, AMotion: 300, RHW: battle.Weapon{Range: 1}, Player: &battle.Player{}}
	logs := &syncBuffer{}
	z.Mods = &Mods{Limits: testLimits, Scripts: []*mod.Scripts{npcMod(src)}, IDs: []string{"core", "example.test"},
		Log: slog.New(slog.NewTextHandler(logs, nil)), Server: variables.NewServer(nil, nil), Temporary: variables.NewTemporary(),
		Monsters: map[uint32]Spawn{1002: {Mob: 1002, Name: "Poring"}}, ItemIDs: map[string]uint32{"Jellopy": 909}}
	for i := range npcs {
		if npcs[i].Script == "" {
			npcs[i].Script = "example.test:scripts/npc.lua"
		}
	}
	z.SetNPCs(npcs)
	t.Cleanup(func() {
		z.stopScripts()
		if strings.Contains(logs.String(), "level=WARN") || strings.Contains(logs.String(), "level=ERROR") {
			t.Errorf("logs: %s", logs)
		}
	})
	return z, logs
}

// presentWorld is a world of zones for these tests: it finds maps and
// lists its zones, and keeps the NPC directory.
type presentWorld struct {
	zones []*Zone
	dir   *NPCDirectory
}

func newPresentWorld(zones ...*Zone) *presentWorld {
	w := &presentWorld{zones: zones, dir: NewNPCDirectory()}
	for _, z := range zones {
		z.World = w
		w.dir.AddZone(z)
	}
	return w
}

func (w *presentWorld) Zone(name string) *Zone {
	for _, z := range w.zones {
		if z.Map == name {
			return z
		}
	}
	return nil
}
func (w *presentWorld) Announce(string, uint32)             {}
func (w *presentWorld) AnnounceFrom(string, uint32, string) {}
func (w *presentWorld) NPCs() *NPCDirectory                 { return w.dir }
func (w *presentWorld) Zones() []*Zone                      { return w.zones }

// chatLines are a player's chat lines as text:from:name:color, from 0 or 1
// for a speaker the player sees, sorted.
func chatLines(ev []*wathenav1.Event) []string {
	var out []string
	for _, e := range ev {
		if c := e.GetChat(); c != nil {
			from := 0
			if c.GetFrom() != 0 {
				from = 1
			}
			out = append(out, fmt.Sprintf("%s:%d:%s:%x", c.GetText(), from, c.GetFromName(), c.GetColor()))
		}
	}
	slices.Sort(out)
	return out
}

const talkerScript = `
local events = require "wa.events"
local context = require "wa.context"
return function()
  local npc = context:get_instance()
  events.on("click", function(player, conversation)
    npc:say_aloud("area")
    npc:say_aloud("map", { to = "map" })
    npc:say_aloud("world", { to = "world", color = 0xFF0000 })
    npc:say_aloud("self", { to = player })
    player:say_aloud("hi")
    player:say_aloud("me", { to = player })
    player:say_aloud("all", { to = "map" })
  end)
end`

// TestSayAloudTargets checks who hears npc:say_aloud and
// player:say_aloud, as npctalk's send targets (script.cpp 16193) and
// unittalk's clif_disp_overhead_: AREA is every player who sees the NPC,
// bc_map every player of the map, bc_all every player of every map, and
// bc_self one player. A player's line goes to the players within chat
// range who see it, as AREA_CHAT_WOC, and back to itself.
func TestSayAloudTargets(t *testing.T) {
	z, _ := presentZone(t, "test", talkerScript, NPC{Name: "example.test:Talker", DisplayName: "Talker", X: 20, Y: 20, Sprite: 112})
	other, _ := presentZone(t, "other", talkerScript)
	newPresentWorld(z, other)
	a := joinAt(z, "a", 21, 20)
	// b sees the NPC and a, but is past a's chat range.
	b := joinAt(z, "b", 34, 20)
	// c sees neither.
	c := joinAt(z, "c", 38, 38)
	d := joinAt(other, "d", 5, 5)
	for _, p := range []*Player{a, b, c} {
		events(p)
	}
	events(d)
	clickNPC(t, z, a, npcNamed(z, "Talker"))
	z.stepAt(z.now + 50)
	other.stepAt(other.now + 50)
	want := map[*Player][]string{
		a: {"all:1:a:0", "area:1:Talker:0", "hi:1:a:0", "map:1:Talker:0", "me:1:a:0", "self:1:Talker:0", "world:1:Talker:ff0000"},
		b: {"all:1:a:0", "area:1:Talker:0", "map:1:Talker:0", "world:1:Talker:ff0000"},
		c: {"all:0:a:0", "map:0:Talker:0", "world:0:Talker:ff0000"},
		d: {"world:0:Talker:ff0000"},
	}
	for p, w := range want {
		if got := chatLines(events(p)); !slices.Equal(got, w) {
			t.Errorf("%s heard %q, want %q", p.Name, got, w)
		}
	}
}

const showScript = `
local events = require "wa.events"
local context = require "wa.context"
local maps = require "wa.maps"
return function()
  local npc = context:get_instance()
  events.on("click", function(player, conversation)
    npc:show_effect(10)
    npc:show_effect(11, { to = "map" })
    npc:show_effect(12, { to = player })
    npc:remove_effect(10)
    player:show_effect(13, { to = player })
    player:remove_effect(13)
    player:show_emotion(5)
    npc:show_emotion(6)
    npc:play_sound("effect/ef_hit1")
    npc:play_sound("jupe_warning", { to = "map", ["repeat"] = true })
    player:play_sound("ding")
    player:play_music("30")
    maps.play_music("test", "59")
    maps.play_sound("test", "bell")
    maps.mark_minimap("test", 3, 10, 12, 0x00FF00)
    maps.unmark_minimap("test", 4)
    player:send_message("green", { color = 0x00FF00 })
    player:show_digits(10, "count_down")
    player:show_digits(150, "count_down_to_zero")
    player:open_quest_log()
    npc:show_skill_effect(28, 5)
  end)
end`

// shown are the presentation events of a frame, as short strings.
func shown(t *testing.T, p *Player, npc *entity, ev []*wathenav1.Event) []string {
	t.Helper()
	id, _ := p.netIDs.get(npc)
	me, _ := p.netIDs.get(p.entity)
	who := func(e uint32) string {
		switch e {
		case 0:
			return "-"
		case id:
			return "npc"
		case me:
			return "me"
		}
		return fmt.Sprint(e)
	}
	var out []string
	for _, e := range ev {
		switch {
		case e.HasEffect():
			out = append(out, fmt.Sprintf("effect %d %s", e.GetEffect().GetEffectId(), who(e.GetEffect().GetEntity())))
		case e.HasEffectRemove():
			out = append(out, fmt.Sprintf("remove %d %s", e.GetEffectRemove().GetEffectId(), who(e.GetEffectRemove().GetEntity())))
		case e.HasEmotion():
			out = append(out, fmt.Sprintf("emotion %d %s", e.GetEmotion().GetEmotion(), who(e.GetEmotion().GetEntity())))
		case e.HasSound():
			s := e.GetSound()
			out = append(out, fmt.Sprintf("sound %s %v %s", s.GetName(), s.GetRepeat(), who(s.GetEntity())))
		case e.HasMusic():
			out = append(out, "music "+e.GetMusic().GetName())
		case e.HasMinimapMark():
			m := e.GetMinimapMark()
			out = append(out, fmt.Sprintf("mark %d %d,%d %x %v", m.GetId(), m.GetX(), m.GetY(), m.GetColor(), m.GetRemove()))
		case e.HasChat():
			out = append(out, fmt.Sprintf("chat %s %s %x", e.GetChat().GetChannel(), e.GetChat().GetText(), e.GetChat().GetColor()))
		case e.HasDigits():
			out = append(out, fmt.Sprintf("digits %s %d", e.GetDigits().GetKind(), e.GetDigits().GetValue()))
		case e.HasQuestLogOpen():
			out = append(out, fmt.Sprintf("quest log %d", e.GetQuestLogOpen().GetQuest()))
		case e.HasSkillEffect():
			s := e.GetSkillEffect()
			out = append(out, fmt.Sprintf("skill %d %d %s", s.GetSkillId(), s.GetLevel(), who(s.GetSource())))
		}
	}
	slices.Sort(out)
	return out
}

// TestPresentTargets checks who sees the effects, emotions, sounds,
// music, digits, marks and lines of area B5: specialeffect's AREA,
// ALL_SAMEMAP and SELF, removespecialeffect, soundeffect and
// soundeffectall, playBGM and playBGMall, viewpointmap, dispbottom's
// color, showdigit's values and open_quest_ui.
func TestPresentTargets(t *testing.T) {
	z, _ := presentZone(t, "test", showScript, NPC{Name: "example.test:Shower", DisplayName: "Shower", X: 20, Y: 20, Sprite: 112})
	z.Skills, _ = testSkills(t, reference.Renewal)
	a := joinAt(z, "a", 21, 20)
	c := joinAt(z, "c", 38, 38)
	events(a)
	events(c)
	n := npcNamed(z, "Shower")
	clickNPC(t, z, a, n)
	z.stepAt(z.now + 50)
	wantA := []string{
		"chat CHAT_CHANNEL_SYSTEM green ff00", "digits DIGITS_KIND_COUNT_DOWN -10", "digits DIGITS_KIND_COUNT_DOWN_TO_ZERO 99",
		"effect 10 npc", "effect 11 npc", "effect 12 npc", "effect 13 me", "emotion 5 me", "emotion 6 npc",
		"mark 3 10,12 ff00 false", "mark 4 0,0 0 true", "music music/30", "music music/59", "quest log 0",
		"remove 10 npc", "remove 13 me", "skill 28 5 npc", "sound sound/bell false -", "sound sound/ding false -",
		"sound sound/effect/ef_hit1 false npc", "sound sound/jupe_warning true -",
	}
	if got := shown(t, a, n, events(a)); !slices.Equal(got, wantA) {
		t.Errorf("a saw\n%q\nwant\n%q", got, wantA)
	}
	// c sees neither the NPC nor a: only what goes to the whole map.
	wantC := []string{"mark 3 10,12 ff00 false", "mark 4 0,0 0 true", "music music/59", "sound sound/bell false -", "sound sound/jupe_warning true -"}
	if got := shown(t, c, n, events(c)); !slices.Equal(got, wantC) {
		t.Errorf("c saw\n%q\nwant\n%q", got, wantC)
	}
}

const lookScript = `
local events = require "wa.events"
local context = require "wa.context"
return function()
  local npc = context:get_instance()
  events.on("npc_event", function(event)
    npc:set_title("<Juno Society>")
    npc:set_display_name("Renamed")
  end, { name = "OnTitle" })
  events.on("npc_event", function(event) npc:set_sprite(1002) end, { name = "OnMonster" })
  events.on("npc_event", function(event)
    npc:set_look { sprite = 4056, hair_style = 13, hair_color = 7, cloth_color = 1, head_top = 1523, sex = "male" }
  end, { name = "OnJob" })
end`

// appears are the NPC's appearances in a frame.
func appears(ev []*wathenav1.Event) []*wathenav1.EntityAppear {
	var out []*wathenav1.EntityAppear
	for _, e := range ev {
		if a := e.GetAppear(); a != nil && a.GetKind() == wathenav1.EntityKind_ENTITY_KIND_NPC {
			out = append(out, a)
		}
	}
	return out
}

// TestNPCLooks checks setunittitle, setnpcdisplay's name and class, and
// setunitdata's UNPC_ looks: the players who see the NPC see it appear
// again, as unit_refresh and clif_name_area, with its title, name and
// view. A monster or a job view keeps its kind, as status_set_viewdata.
func TestNPCLooks(t *testing.T) {
	z, _ := presentZone(t, "test", lookScript, NPC{Name: "example.test:Looker", DisplayName: "Looker", X: 20, Y: 20, Sprite: 112})
	a := joinAt(z, "a", 21, 20)
	c := joinAt(z, "c", 38, 38)
	n := npcNamed(z, "Looker")
	h := scriptHost{z}
	fire := func(label string) []*wathenav1.EntityAppear {
		t.Helper()
		events(a)
		events(c)
		if _, err := h.FireNPCEvent(n.npc.handle, strings.ToLower(label), 0); err != nil {
			t.Fatal(err)
		}
		z.stepAt(z.now + 50)
		if got := appears(events(c)); len(got) != 0 {
			t.Errorf("%s: c, who does not see the NPC, saw %v", label, got)
		}
		return appears(events(a))
	}
	got := fire("OnTitle")
	if len(got) != 1 || got[0].GetTitle() != "<Juno Society>" || got[0].GetName() != "Renamed" {
		t.Fatalf("title and name: %v", got)
	}
	if name, _ := h.NPCField(n.npc.handle, "name"); str(name) != "Renamed" {
		t.Errorf("npc.name is %q", str(name))
	}
	got = fire("OnMonster")
	if len(got) != 1 || got[0].GetLook().GetBody() != 1002 || got[0].GetLook().GetNpcView() != wathenav1.NpcView_NPC_VIEW_MONSTER {
		t.Fatalf("monster view: %v", got)
	}
	got = fire("OnJob")
	l := got[0].GetLook()
	if len(got) != 1 || l.GetBody() != 4056 || l.GetNpcView() != wathenav1.NpcView_NPC_VIEW_JOB || l.GetHair() != 13 || l.GetHairColor() != 7 ||
		l.GetClothColor() != 1 || l.GetHeadTop() != 1523 || l.GetSex() != 1 {
		t.Fatalf("job view: %v", got)
	}
	// A player who comes later sees the NPC as it is now.
	c.entity.x, c.entity.y = 22, 22
	z.stepAt(z.now + 50)
	if got := appears(events(c)); len(got) != 1 || got[0].GetLook().GetBody() != 4056 || got[0].GetTitle() != "<Juno Society>" {
		t.Errorf("a later viewer saw %v", got)
	}
}

const walkerScript = `
local events = require "wa.events"
local context = require "wa.context"
return function()
  local npc = context:get_instance()
  events.on("npc_event", function(event)
    local results = {}
    results[#results + 1] = tostring(npc:walk_to(26, 20))
    results[#results + 1] = tostring(npc:walk_to(60, 20))
    npc:set_title(table.concat(results, ","))
  end, { name = "OnWalk" })
  events.on("npc_event", function(event) npc:set_speed(100) end, { name = "OnFast" })
  events.on("npc_event", function(event) npc:stop() end, { name = "OnStop" })
  events.on("npc_event", function(event) npc:move_to(5, 70) end, { name = "OnMove" })
  events.on("touch", function(player) npc:set_title("touched " .. player.name) end)
end`

// TestNPCWalk checks npcwalkto, npcspeed, npcstop and movenpc: an NPC
// walks at DEFAULT_NPC_WALK_SPEED, a cell no walk reaches fails, npcspeed
// changes the time of the next steps, npcstop past half a step finishes
// the cell, the players it walks into touch it, and movenpc caps the cell
// to the map and makes the NPC vanish and appear again.
func TestNPCWalk(t *testing.T) {
	z, _ := presentZone(t, "test", walkerScript, NPC{Name: "example.test:Walker", DisplayName: "Walker", X: 20, Y: 20, Sprite: 112,
		Touch: &TouchArea{XSpan: 0, YSpan: 0}})
	a := joinAt(z, "a", 24, 21)
	n := npcNamed(z, "Walker")
	h := scriptHost{z}
	fire := func(label string) {
		t.Helper()
		if _, err := h.FireNPCEvent(n.npc.handle, strings.ToLower(label), 0); err != nil {
			t.Fatal(err)
		}
	}
	events(a)
	fire("OnWalk")
	// The event runs after the walks of the step: the walk starts then.
	z.stepAt(z.now + 50)
	start := z.now
	if n.npc.title != "true,false" {
		t.Fatalf("walk_to results %q", n.npc.title)
	}
	var moved bool
	for _, e := range events(a) {
		if m := e.GetMove(); m != nil {
			moved = true
		}
	}
	if !moved {
		t.Fatal("a saw no walk")
	}
	// One cell takes 200 ms.
	z.stepAt(start + 190)
	if n.x != 20 {
		t.Fatalf("at 190 ms the NPC is at %d", n.x)
	}
	z.stepAt(start + 210)
	if n.x != 21 {
		t.Fatalf("at 210 ms the NPC is at %d", n.x)
	}
	// The new speed counts from the step after the one under way: the
	// step to 22 ends at 400 ms, and the step to 23 takes 100 ms.
	fire("OnFast")
	z.stepAt(start + 400)
	z.stepAt(start + 500)
	if n.x != 23 {
		t.Fatalf("at 500 ms the NPC is at %d", n.x)
	}
	// Past half of the step to 24, npcstop finishes the cell.
	z.stepAt(start + 560)
	fire("OnStop")
	z.stepAt(start + 570)
	if n.x != 24 || len(n.path) != 0 {
		t.Fatalf("stopped at %d with %d steps left", n.x, len(n.path))
	}
	var stopped bool
	for _, e := range events(a) {
		stopped = stopped || e.GetStop() != nil
	}
	if !stopped {
		t.Error("a saw no stop")
	}
	// The player stands on 24,21: a walk of the NPC onto 24,21 touches it.
	if _, err := h.WalkNPC(n.npc.handle, 24, 21); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10 && n.npc.title != "touched a"; i++ {
		z.stepAt(z.now + 100)
	}
	if n.npc.title != "touched a" {
		t.Fatalf("the walk did not touch a: title %q, NPC at %d,%d", n.npc.title, n.x, n.y)
	}
	events(a)
	fire("OnMove")
	z.stepAt(z.now + 50)
	if n.x != 5 || n.y != 39 {
		t.Fatalf("movenpc to 5,70 put the NPC at %d,%d, want the map's edge 5,39", n.x, n.y)
	}
	var vanished bool
	for _, e := range events(a) {
		vanished = vanished || e.GetVanish() != nil
	}
	if !vanished {
		t.Error("a, far from the new cell, saw no vanish")
	}
}

const barScript = `
local events = require "wa.events"
local context = require "wa.context"
local variables = require "wa.variables"
local order = variables.server_temporary_string("order")
return function()
  local npc = context:get_instance()
  events.on("click", function(player, conversation)
    npc:show_progress(2, { color = 0xFF0000 })
    npc:set_title("done")
  end)
  events.on("npc_event", function(event)
    local ok, err = pcall(npc.show_progress, npc, 1)
    order:set((tostring(err):gsub("^.-:%d+: ", "")))
  end, { name = "OnBar" })
end`

// bars are the NPC progress bars of a frame, as duration:color.
func bars(ev []*wathenav1.Event) []string {
	var out []string
	for _, e := range ev {
		if b := e.GetNpcProgressBar(); b != nil {
			out = append(out, fmt.Sprintf("%d:%x", b.GetDurationMs(), b.GetColor()))
		}
	}
	return out
}

// TestNPCProgressBar checks progressbar_npc: the bar shows over the NPC
// to the players who see it, a player who comes later sees the time
// left (clif_progressbar_npc), and the conversation waits until it is
// full. Without a conversation, it raises an error.
func TestNPCProgressBar(t *testing.T) {
	z, _ := presentZone(t, "test", barScript, NPC{Name: "example.test:Barred", DisplayName: "Barred", X: 20, Y: 20, Sprite: 112})
	a := joinAt(z, "a", 21, 20)
	c := joinAt(z, "c", 38, 38)
	n := npcNamed(z, "Barred")
	events(a)
	clickNPC(t, z, a, n)
	start := z.now
	z.stepAt(z.now + 50)
	if got := bars(events(a)); len(got) != 1 || got[0] != "2000:ff0000" {
		t.Fatalf("a saw the bars %v", got)
	}
	if got := bars(events(c)); len(got) != 0 {
		t.Fatalf("c saw the bars %v", got)
	}
	c.entity.x, c.entity.y = 22, 22
	z.stepAt(start + 500)
	if got := bars(events(c)); len(got) != 1 || got[0] != "1500:ff0000" {
		t.Fatalf("c, who came later, saw %v", got)
	}
	if n.npc.title == "done" {
		t.Fatal("the conversation did not wait")
	}
	for z.now < start+2100 {
		z.stepAt(z.now + 100)
	}
	if n.npc.title != "done" {
		t.Fatalf("after the bar the title is %q", n.npc.title)
	}
	h := scriptHost{z}
	if _, err := h.FireNPCEvent(n.npc.handle, "onbar", 0); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	if got, want := order(t, z), "npc:show_progress waits, so it runs only in a conversation's handler, as a click's"; got != want {
		t.Errorf("without a conversation: %q", got)
	}
}

const healerScript = `
local events = require "wa.events"
local context = require "wa.context"
return function()
  local npc = context:get_instance()
  events.on("click", function(player, conversation)
    local healed = npc:use_skill(28, 10, player, { stat = 50, npc_level = 99 })
    local blessed = npc:use_skill(34, 10, player)
    npc:set_title(tostring(healed) .. "," .. tostring(blessed))
  end)
end`

// TestNPCUseSkill checks npcskill: the NPC casts at once, with no range
// check, with the status of status_calc_npc from the stat and the level,
// and the next cast keeps them.
func TestNPCUseSkill(t *testing.T) {
	z, _ := presentZone(t, "test", healerScript, NPC{Name: "example.test:Healer", DisplayName: "Healer", X: 20, Y: 20, Sprite: 112})
	z.Status, z.Start = jobTables(t), status.Character{Male: true, Stats: status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1}}
	z.Statuses = testStatuses(t)
	z.Skills, z.SkillTrees = testSkills(t, reference.Renewal)
	// The player stands farther than Heal's range of 9 cells.
	a := joinAt(z, "a", 31, 20)
	a.entity.hp = 1
	events(a)
	n := npcNamed(z, "Healer")
	clickNPC(t, z, a, n)
	z.stepAt(z.now + 50)
	if n.npc.title != "true,true" {
		t.Fatalf("use_skill returned %q", n.npc.title)
	}
	if n.npc.skillLevel != 99 || n.npc.skillStat != 50 || n.status == nil || n.status.Int != 50 || n.status.Level != 99 {
		t.Fatalf("the NPC's status: level %d, stat %d, %+v", n.npc.skillLevel, n.npc.skillStat, n.status)
	}
	if a.entity.hp <= 1 {
		t.Errorf("Heal left the player at %d HP", a.entity.hp)
	}
	if !a.entity.sc.Has(sc.SCBlessing) {
		t.Error("Blessing did not start on the player")
	}
	var effects []uint32
	id, _ := a.netIDs.get(n)
	for _, e := range events(a) {
		if s := e.GetSkillEffect(); s != nil && s.GetSource() == id {
			effects = append(effects, s.GetSkillId())
		}
	}
	if !slices.Contains(effects, 28) || !slices.Contains(effects, 34) {
		t.Errorf("a saw the NPC's skill effects %v", effects)
	}
}

const questGiverScript = `
local context = require "wa.context"
return function()
  local npc = context:get_instance()
  npc:add_quest_icon("quest", { mark = "yellow", condition = function(p) return p:count_item(909) == 0 end })
  npc:add_quest_icon("daily_quest", { condition = function(p) return p:count_item(909) < 3 end })
end`

// icons are the quest icons of a frame, as icon/mark.
func icons(ev []*wathenav1.Event) []string {
	var out []string
	for _, e := range ev {
		if q := e.GetQuestIcon(); q != nil {
			out = append(out, q.GetIcon()+"/"+q.GetMark())
		}
	}
	return out
}

// TestQuestInfo checks questinfo and pc_show_questinfo: at map entry a
// player sees the first icon whose condition holds, an item gained shows
// the next one, the icon goes when none holds, nothing is sent while
// nothing changes, and an NPC that comes into view brings its icon.
func TestQuestInfo(t *testing.T) {
	z, _ := presentZone(t, "test", questGiverScript, NPC{Name: "example.test:Giver", DisplayName: "Giver", X: 20, Y: 20, Sprite: 112})
	a := joinAt(z, "a", 21, 20)
	a.maxWeight = maxWeight
	n := npcNamed(z, "Giver")
	if got := icons(events(a)); !slices.Equal(got, []string{"quest/yellow"}) {
		t.Fatalf("at map entry: %v", got)
	}
	z.addItem(a, 909, 1, scriptOrigin)
	z.stepAt(z.now + 50)
	if got := icons(events(a)); !slices.Equal(got, []string{"daily_quest/"}) {
		t.Fatalf("with a Jellopy: %v", got)
	}
	h := scriptHost{z}
	if err := h.RefreshQuestIcons(int64(a.charID)); err != nil {
		t.Fatal(err)
	}
	z.stepAt(z.now + 50)
	if got := icons(events(a)); len(got) != 0 {
		t.Fatalf("a refresh with nothing changed sent %v", got)
	}
	z.addItem(a, 909, 2, scriptOrigin)
	z.stepAt(z.now + 50)
	if got := icons(events(a)); !slices.Equal(got, []string{"/"}) {
		t.Fatalf("with 3 Jellopies: %v", got)
	}
	// Out of view and back, with the daily icon again.
	for slot := range a.inventory.Slots {
		if a.inventory.Slots[slot].ID == 909 {
			z.deleteItem(a, slot, 1, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_TAKEN)
		}
	}
	a.entity.x = 38
	z.stepAt(z.now + 50)
	events(a)
	a.entity.x = 22
	z.stepAt(z.now + 50)
	if got := icons(events(a)); !slices.Equal(got, []string{"daily_quest/"}) {
		t.Fatalf("on coming back: %v", got)
	}
	if !slices.Contains(z.questInfoNPCs, n) {
		t.Error("the NPC is not in the map's list")
	}
}

const callerScript = `
local events = require "wa.events"
local context = require "wa.context"
local world = require "wa.world"
local variables = require "wa.variables"
local order = variables.server_temporary_string("order")
return function()
  local npc = context:get_instance()
  events.on("click", function(player, conversation)
    local far = world.get_npc("Far")
    far:say_aloud("hello", { to = "map" })
    far:say_aloud("psst", { to = player })
    far:show_emotion(1)
    far:show_effect(3)
    far:set_title("Remote")
    order:set(tostring(far:walk_to(25, 5)))
  end)
  events.on("npc_event", function(event)
    local far = world.get_npc("Far")
    order:set(far.x .. "," .. far.y)
  end, { name = "OnWhere" })
end`

// TestPresentAcrossZones checks review decision 58's calls on an NPC of
// another map: they run in its zone after the current step, for the
// players of that map, and a line to one player comes with the NPC's
// name alone. The directory follows the NPC's walk.
func TestPresentAcrossZones(t *testing.T) {
	za, _ := presentZone(t, "a", callerScript, NPC{Name: "example.test:Caller", DisplayName: "Caller", X: 20, Y: 20, Sprite: 112})
	zb, _ := presentZone(t, "b", callerScript, NPC{Name: "example.test:Far", DisplayName: "Far", X: 5, Y: 5, Sprite: 112})
	newPresentWorld(za, zb)
	a := joinAt(za, "a", 21, 20)
	b := joinAt(zb, "b", 6, 5)
	events(a)
	events(b)
	clickNPC(t, za, a, npcNamed(za, "Caller"))
	za.stepAt(za.now + 50)
	if got := order(t, za); got != "true" {
		t.Fatalf("walk_to of another map's NPC returned %q", got)
	}
	if got := chatLines(events(a)); !slices.Equal(got, []string{"psst:0:Far:0"}) {
		t.Errorf("a heard %q", got)
	}
	zb.stepAt(zb.now + 50)
	far := npcNamed(zb, "Far")
	ev := events(b)
	if got := chatLines(ev); !slices.Equal(got, []string{"hello:1:Far:0"}) {
		t.Errorf("b heard %q", got)
	}
	got := shown(t, b, far, ev)
	if !slices.Contains(got, "emotion 1 npc") || !slices.Contains(got, "effect 3 npc") {
		t.Errorf("b saw %q", got)
	}
	if a := appears(ev); len(a) != 1 || a[0].GetTitle() != "Remote" {
		t.Errorf("b saw the appearances %v", a)
	}
	for i := 0; i < 60 && far.x != 25; i++ {
		zb.stepAt(zb.now + 100)
	}
	if far.x != 25 {
		t.Fatalf("Far walked to %d,%d", far.x, far.y)
	}
	h := scriptHost{za}
	if _, err := h.FireNPCEvent(npcNamed(za, "Caller").npc.handle, "onwhere", 0); err != nil {
		t.Fatal(err)
	}
	za.stepAt(za.now + 50)
	if got := order(t, za); got != "25,5" {
		t.Errorf("Far's cell from map a: %q", got)
	}
}

const checksScript = `
local events = require "wa.events"
local context = require "wa.context"
local variables = require "wa.variables"
local order = variables.server_temporary_string("order")
return function()
  local npc = context:get_instance()
  npc:add_quest_icon("quest", { condition = function(p) p:give_zeny(1); return true end })
  events.on("click", function(player, conversation)
    local function try(f, ...)
      local ok, err = pcall(f, ...)
      order:set(order:get() .. (tostring(err):gsub("^.-:%d+: ", "")) .. "|")
    end
    try(npc.say_aloud, npc, "x", { to = "guild" })
    try(npc.say_aloud, npc, "x", { colour = 1 })
    try(npc.set_speed, npc, 10)
    try(npc.set_title, npc, string.rep("t", 24))
    try(npc.set_look, npc, { wings = 1 })
    try(npc.add_quest_icon, npc, "star")
    try(npc.show_effect, npc, 1, { to = "world" })
    try(player.show_digits, player, 1, "count_sideways")
    try(npc.use_skill, npc, 28, 1, player, { stat = 131 })
  end)
end`

// TestPresentChecks checks the errors of bad arguments, and that a quest
// icon's condition only reads: a call that changes the game fails, and
// the icon does not show.
func TestPresentChecks(t *testing.T) {
	z, logs := presentZone(t, "test", checksScript, NPC{Name: "example.test:Checker", DisplayName: "Checker", X: 20, Y: 20, Sprite: 112})
	a := joinAt(z, "a", 21, 20)
	if got := icons(events(a)); len(got) != 0 {
		t.Errorf("a condition that changes the game showed %v", got)
	}
	if !strings.Contains(logs.String(), "player:give_zeny cannot run in a computed bonus, a hook or a condition") {
		t.Errorf("logs: %s", logs)
	}
	logs.mu.Lock()
	logs.b.Reset()
	logs.mu.Unlock()
	clickNPC(t, z, a, npcNamed(z, "Checker"))
	z.stepAt(z.now + 50)
	want := []string{
		`npc:say_aloud: to is one of ["area" "map" "world" "player"], or a player handle`,
		"npc:say_aloud: there is no option colour",
		"npc:set_speed: the speed is 20 to 1000 milliseconds per cell",
		"npc:set_title: the title is at most 23 bytes of UTF-8",
		`npc:set_look: there is no field "wings"; the fields are ["sprite" "hair_style" "hair_color" "cloth_color" "head_top" "head_middle" "head_bottom" "sex"]`,
		"npc:add_quest_icon: the icon is one of wa.QuestIcon, such as quest",
		`npc:show_effect: to is one of ["area" "map" "player"], or a player handle`,
		`player:show_digits: the kind is one of ["show" "count_up" "count_down" "count_down_to_zero"]`,
		"npc:use_skill: stat is 0 to 130, as max_third_parameter",
	}
	got := strings.Split(strings.TrimSuffix(strings.ReplaceAll(order(t, z), `\"`, `"`), "|"), "|")
	if !slices.Equal(got, want) {
		t.Errorf("errors:\n%q\nwant\n%q", got, want)
	}
}

// TestNPCStopEarly checks that npcstop before half of a step leaves the
// NPC on its cell, as USW_MOVE_FULL_CELL needs half of the step.
func TestNPCStopEarly(t *testing.T) {
	z, _ := presentZone(t, "test", walkerScript, NPC{Name: "example.test:Walker", DisplayName: "Walker", X: 20, Y: 20, Sprite: 112})
	joinAt(z, "a", 24, 21)
	n := npcNamed(z, "Walker")
	h := scriptHost{z}
	if ok, err := h.WalkNPC(n.npc.handle, 26, 20); err != nil || !ok {
		t.Fatal(ok, err)
	}
	z.stepAt(z.now + 50)
	if err := h.StopNPC(n.npc.handle); err != nil {
		t.Fatal(err)
	}
	if n.x != 20 || len(n.path) != 0 || !n.stopped {
		t.Errorf("stopped at %d with %d steps left", n.x, len(n.path))
	}
}

const showeventScript = `
local events = require "wa.events"
local context = require "wa.context"
return function()
  local npc = context:get_instance()
  events.on("click", function(player, conversation)
    npc:show_quest_icon(player, "job", { mark = "green" })
    npc:show_quest_icon(player, "daily_quest", { mark = "purple" })
  end)
end`

// TestShowQuestIconMark checks showevent's mark color, and that
// showevent turns QTYPE_DAILYQUEST into QTYPE_NONE (script.cpp 21195):
// the icon goes, and the mark still goes out.
func TestShowQuestIconMark(t *testing.T) {
	z, _ := presentZone(t, "test", showeventScript, NPC{Name: "example.test:Shower", DisplayName: "Shower", X: 20, Y: 20, Sprite: 112})
	a := joinAt(z, "a", 21, 20)
	events(a)
	clickNPC(t, z, a, npcNamed(z, "Shower"))
	z.stepAt(z.now + 50)
	if got := icons(events(a)); !slices.Equal(got, []string{"job/green", "/purple"}) {
		t.Errorf("icons %v", got)
	}
}
