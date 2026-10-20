package convert

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/modscript"
)

// The PresentHost of the NPC runtime tests: each call is recorded, and
// an NPC's effect to its area is an effect, as before area B5.
func (h *npcHost) present(format string, args ...any) error {
	h.presents = append(h.presents, fmt.Sprintf(format, args...))
	return nil
}

func (h *npcHost) SayAloud(e modscript.Entity, text string, to modscript.SendTarget, color int64) error {
	return h.present("say %s %d %q %s %d", e.Kind, e.ID, text, to.To, color)
}
func (h *npcHost) PlayerEmotion(_, emotion int64) error {
	return h.present("player emotion %d", emotion)
}
func (h *npcHost) ShowEffectTo(e modscript.Entity, effect int64, to modscript.SendTarget) error {
	if e.Kind == "npc" && to.To == "area" {
		return h.ShowEntityEffect(e, effect)
	}
	return h.present("effect %s %d %d %s", e.Kind, e.ID, effect, to.To)
}
func (h *npcHost) RemoveEffect(e modscript.Entity, effect int64) error {
	return h.present("remove effect %s %d %d", e.Kind, e.ID, effect)
}
func (h *npcHost) SetNPCTitle(id int64, title string) error {
	return h.present("title %d %q", id, title)
}
func (h *npcHost) SetNPCDisplayName(id int64, name string) error {
	return h.present("name %d %q", id, name)
}
func (h *npcHost) SetNPCLook(id int64, l modscript.NPCLook) error {
	out := fmt.Sprintf("look %d", id)
	for _, f := range []struct {
		name string
		v    *int64
	}{{"sprite", l.Sprite}, {"hair_style", l.HairStyle}, {"hair_color", l.HairColor}, {"cloth_color", l.ClothColor},
		{"head_top", l.HeadTop}, {"head_middle", l.HeadMiddle}, {"head_bottom", l.HeadBottom}} {
		if f.v != nil {
			out += fmt.Sprintf(" %s=%d", f.name, *f.v)
		}
	}
	if l.Male != nil {
		out += fmt.Sprintf(" male=%t", *l.Male)
	}
	return h.present("%s", out)
}
func (h *npcHost) MoveNPC(id int64, x, y int) error { return h.present("move %d %d,%d", id, x, y) }
func (h *npcHost) WalkNPC(id int64, x, y int) (bool, error) {
	return true, h.present("walk %d %d,%d", id, x, y)
}
func (h *npcHost) SetNPCSpeed(id int64, ms int) error { return h.present("speed %d %d", id, ms) }
func (h *npcHost) StopNPC(id int64) error             { return h.present("stop %d", id) }
func (h *npcHost) ShowNPCProgress(id, ms int64, color uint32) error {
	return h.present("bar %d %d %06x", id, ms, color)
}
func (h *npcHost) NPCUseSkill(id, skill, level int64, target modscript.Entity, stat, npcLevel int64) (bool, error) {
	return true, h.present("skill %d %d %d %s %d %d", id, skill, level, target.Kind, stat, npcLevel)
}
func (h *npcHost) ShowSkillEffect(e modscript.Entity, skill, level int64) error {
	return h.present("skill effect %s %d %d", e.Kind, skill, level)
}
func (h *npcHost) PlaySound(e modscript.Entity, mapName, name, to string, repeat bool) error {
	return h.present("sound %s %s %s %s %t", e.Kind, mapName, name, to, repeat)
}
func (h *npcHost) PlayMusic(player int64, mapName, name string) error {
	return h.present("music %d %s %s", player, mapName, name)
}
func (h *npcHost) SendColoredMessage(_ int64, text string, color uint32) error {
	return h.present("message %q %06x", text, color)
}
func (h *npcHost) ShowDigits(_ int64, kind int, value int64) error {
	return h.present("digits %d %d", kind, value)
}
func (h *npcHost) MarkMinimapOfMap(m string, id int64, x, y int, color uint32, remove bool) error {
	return h.present("mark %s %d %d,%d %06x %t", m, id, x, y, color, remove)
}
func (h *npcHost) QuestIconsChanged(id int64) error    { return h.present("quest icons %d", id) }
func (h *npcHost) RefreshQuestIcons(int64) error       { return h.present("refresh quest icons") }
func (h *npcHost) OpenQuestLog(_ int64, q int64) error { return h.present("quest log %d", q) }

var _ modscript.PresentHost = (*npcHost)(nil)

// TestPresentConvert checks the converted text of the commands of review
// decision 58, areas B5 and B6, and the reasons of the forms that do not
// convert.
func TestPresentConvert(t *testing.T) {
	res := questResult(t)
	for _, c := range []struct {
		script string
		want   []string
	}{
		{`{ npctalk "Hi"; end; }`, []string{`npc:say_aloud("Hi")`}},
		{`{ npctalk "Hi","Other#t",bc_map; end; }`, []string{`R.npc_view("Other#t", "say_aloud", "Hi", { to = "map" })`}},
		{`{ npctalk "Hi","",bc_all,0xFF0000; end; }`, []string{`npc:say_aloud("Hi", { to = "world", color = 16711680 })`}},
		{`{ npctalk "Hi","",bc_self; end; }`, []string{`if player ~= nil then npc:say_aloud("Hi", { to = player }) end`}},
		{`{ npctalk "Hi",.@n$; end; }`, []string{`R.npc_view(R.or_self(n, npc), "say_aloud", "Hi")`}},
		{`{ npctalk .@m$; end; }`, []string{`R.say_aloud(npc, m)`}},
		{`{ npctalk "Hi","",bc_area,0x1000000; end; }`, []string{"!npctalk: a color past 0xFFFFFF"}},
		{`{ unittalk getcharid(3),"" + strcharinfo(0) + " : Hello",bc_self; end; }`, []string{`player:say_aloud("Hello", { to = player })`}},
		{`{ unittalk getcharid(3),strcharinfo(0) + " : Hi " + .@n$; end; }`, []string{`player:say_aloud("Hi " .. n)`}},
		{`{ unittalk getnpcid(0),"Hi"; end; }`, []string{"!unittalk: an NPC's line"}},
		{`{ emotion ET_OK, playerattached(); emotion ET_HNG, getnpcid(0, "Guard#" + strnpcinfo(2)); end; }`, []string{
			"player:show_emotion(emotions.ok)", `R.npc_view("Guard#" .. npc.hidden_name, "show_emotion", emotions.hng)`,
		}},
		{`{ unittalk .@aid,"Hi"; end; }`, []string{"!unittalk: a game ID other than"}},
		{`{ emotion ET_SMILE, getnpcid(0, "Other#t"); emotion ET_OK, getcharid(3); emotion ET_HNG, getnpcid(0); end; }`, []string{
			`R.npc_view("Other#t", "show_emotion", emotions.smile)`, "player:show_emotion(emotions.ok)", "npc:show_emotion(emotions.hng)",
		}},
		{`{ specialeffect EF_HEAL,ALL_SAMEMAP,"Other#t"; end; }`, []string{`R.npc_view("Other#t", "show_effect", effects.heal, { to = "map" })`}},
		{`{ specialeffect2 EF_HEAL,SELF; end; }`, []string{`player:show_effect(effects.heal, { to = player })`}},
		{`{ removespecialeffect EF_HEAL; removespecialeffect EF_HEAL,AREA,"Other#t"; removespecialeffect2 EF_HEAL; end; }`, []string{
			"npc:remove_effect(effects.heal)", `R.npc_view("Other#t", "remove_effect", effects.heal)`, "player:remove_effect(effects.heal)",
		}},
		{`{ removespecialeffect EF_HEAL,SELF; end; }`, []string{"!removespecialeffect: a send target other than AREA"}},
		{`{ setunittitle getnpcid(0), "<Society>"; end; }`, []string{`npc:set_title("<Society>")`}},
		{`{ setunittitle getcharid(3), "x"; end; }`, []string{"!setunittitle: on a player"}},
		{`{ setunittitle getnpcid(0), "<Subspace Vending Machine>"; setunittitle getnpcid(0), .@t$; end; }`, []string{
			`npc:set_title("<Subspace Vending Machi")`, "npc:set_title(R.title(t))",
		}},
		{`{ setnpcdisplay "Other#t","Renamed",4_F_KAFRA1; setnpcdisplay("Other#t", CLEAR_NPC); setnpcdisplay "Other#t","Same",-1; end; }`, []string{
			`R.npc_display("Other#t", "Renamed", `, `R.npc_display("Other#t", nil, 844)`, `R.npc_display("Other#t", "Same", nil)`,
		}},
		{`{ setnpcdisplay "Other#t","x",1,2; end; }`, []string{"!setnpcdisplay: a size"}},
		{`{ classchange 1002; classchange 1002,"Other#t"; end; }`, []string{"npc:set_sprite(1002)", `R.npc_view("Other#t", "set_sprite", 1002)`}},
		{`{ classchange 1002,"",bc_self; end; }`, []string{"!classchange: for one player"}},
		{`{ .@npc_id = getnpcid(0); setunitdata .@npc_id, UNPC_HAIRSTYLE, 13; setunitdata .@npc_id, UNPC_SEX, SEX_MALE; setunitdata getnpcid(0, "Other#t"), UNPC_CLASS, 4056; end; }`, []string{
			"-- .@npc_id holds the game ID of an NPC.", "npc:set_look({ hair_style = 13 })", `npc:set_look({ sex = "male" })`,
			`R.npc_view("Other#t", "set_look", { sprite = 4056 })`,
		}},
		{`{ .@npc_id = getnpcid(0); mes "" + .@npc_id; end; }`, []string{"!getnpcid: the game ID of an NPC"}},
		{`{ setunitdata getnpcid(0), UNPC_LEVEL, 13; end; }`, []string{"!setunitdata: a type other than"}},
		{`{ if (unitwalk(getnpcid(0), 10, 12)) end; unitwalk getnpcid(0,"Other#t"),1,2; end; }`, []string{
			"if (npc:walk_to(10, 12) == true) then", "\n" + `      R.npc_view("Other#t", "walk_to", 1, 2)` + "\n",
		}},
		{`{ unitwalk getnpcid(0),1,2,"X::OnDone"; end; }`, []string{"!unitwalk: an event label at the walk's end"}},
		{`{ unitwarp getnpcid(0),"this",3,4; unitwarp 0,"prontera",5,6; unitstopwalk getnpcid(0); end; }`, []string{
			"npc:move_to(3, 4)", `player:warp("prontera", 5, 6)`, "npc:stop()",
		}},
		{`{ unitwarp getnpcid(0),"geffen",3,4; end; }`, []string{"!unitwarp: an NPC to another map"}},
		{`{ npcspeed 300; npcwalkto 5,6,"Other#t"; npcstop; movenpc "Other#t",7,8; end; }`, []string{
			"npc:set_speed(300)", `R.npc_view("Other#t", "walk_to", 5, 6)`, "npc:stop()", `R.npc_view("Other#t", "move_to", 7, 8)`,
		}},
		{`{ movenpc "Other#t",7,8,2; end; }`, []string{"!movenpc: a direction"}},
		{`{ npcstop "",USW_NONE; end; }`, []string{"!npcstop: a flag other than the default"}},
		{`{ progressbar_npc "FFFF00",2; progressbar_npc "0xFF0000",3,"Other#t"; end; }`, []string{
			"npc:show_progress(2, { color = 0x000000 })", `R.npc_view("Other#t", "show_progress", 3, { color = 0xFF0000 })`,
		}},
		{`{ npcskill "AL_HEAL",10,50,99; end; }`, []string{"npc:use_skill(AL_HEAL, 10, player, { stat = 50, npc_level = 99 })"}},
		{`{ skilleffect "AL_HEAL",5; skilleffect 28,1,getnpcid(0); end; }`, []string{"player:show_skill_effect(AL_HEAL, 5)", "npc:show_skill_effect(AL_HEAL, 1)"}},
		{`{ soundeffect "jupe_warning.wav",1; soundeffectall "_curse.wav",0; soundeffectall "Earth_Quake.wav",0,"prontera"; end; }`, []string{
			`player:play_sound("jupe_warning", { ["repeat"] = true })`, `npc:play_sound("_curse")`, `maps.play_sound("prontera", "earth_quake")`,
		}},
		{`{ soundeffect "a.wav",2; end; }`, []string{"!soundeffect: type 2, which stops a sound"}},
		{`{ soundeffectall "a.wav",0,"prontera",1,1,5,5; end; }`, []string{"!soundeffectall: a part of a map"}},
		{`{ playbgm "30.mp3"; playbgmall "59","prontera"; end; }`, []string{`player:play_music("30")`, `maps.play_music("prontera", "59")`}},
		{`{ playbgmall "59"; end; }`, []string{"!playbgmall: every map of the server"}},
		{`{ dispbottom "Hi",0xFF0000; dispbottom "Me",0x00FF00,getcharid(0); end; }`, []string{
			`player:send_message("Hi", { color = 16711680 })`, `player:send_message("Me", { color = 65280 })`,
		}},
		{`{ showdigit 10; showdigit 30,2; end; }`, []string{`player:show_digits(10, "show")`, `player:show_digits(30, "count_down")`}},
		{`{ viewpointmap "prontera",1,10,12,3,0xFF0000; viewpointmap "prontera",2,0,0,3,0; end; }`, []string{
			`maps.mark_minimap("prontera", 3, 10, 12, 16711680)`, `maps.unmark_minimap("prontera", 3)`,
		}},
		{`{ end; OnInit: questinfo QTYPE_DAILYQUEST,QMARK_YELLOW,"BaseLevel >= 100 && countitem(909) < 1"; questinfo QTYPE_QUEST; end; }`, []string{
			`npc:add_quest_icon("daily_quest", { mark = "yellow", condition = function(player) return ((player.base_level >= 100) and (player:count_item(JELLOPY) < 1)) end })`,
			`npc:add_quest_icon("quest")`,
		}},
		{`{ end; OnInit: questinfo QTYPE_NONE; end; }`, []string{"!questinfo: the icon 9999, QTYPE_NONE"}},
		{`{ questinfo_refresh; open_quest_ui; open_quest_ui 1100; end; }`, []string{
			"player:refresh_quest_icons()", "player:open_quest_log()", "player:open_quest_log(1100)",
		}},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		lua, err := n.npcScript()
		for _, want := range c.want {
			if reason, ok := strings.CutPrefix(want, "!"); ok {
				if err == nil || !strings.Contains(err.Error(), reason) {
					t.Errorf("%s: got %v, want %q", c.script, err, reason)
				}
				continue
			}
			if err != nil || !strings.Contains(lua, want) {
				t.Errorf("%s: %v, want %q in\n%s", c.script, err, want, lua)
			}
		}
	}
}

// TestNPCViewSprites checks that an NPC line's view may be a monster or
// a job, as rAthena takes any class (review decision 58): npcs.yml gets
// its number. A view that is no class is refused.
func TestNPCViewSprites(t *testing.T) {
	read := func(name string) []byte {
		b, err := reference.ReadFile("extracts/renewal/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var lines []npcLine
	for _, v := range [][2]string{{"Mob#v", "1002"}, {"Named#v", "PORING"}, {"Job#v", "4056"}, {"None#v", "6002"}} {
		lines = append(lines, npcLine{Kind: "script", Name: v[0], Display: v[0], File: "test.txt", Line: 1, Map: "prontera",
			X: 150, Y: 150, Sprite: v[1], XS: -1, YS: -1, Script: `{ mes "Hi"; close; }`})
	}
	data, err := json.Marshal(map[string]any{"mode": "renewal", "npcs": lines})
	if err != nil {
		t.Fatal(err)
	}
	res, err := ConvertNPCs(NPCInput{NPCs: bytes.NewReader(data), Constants: bytes.NewReader(read("script_constants")),
		ClientTables: bytes.NewReader(read("client_tables"))})
	if err != nil {
		t.Fatal(err)
	}
	yml := res.Files()["data/npcs.yml"]
	for _, w := range []string{`name: "Mob#v", map: prontera, x: 150, y: 150, direction: north, sprite: "1002"`, `name: "Named#v", map: prontera, x: 150, y: 150, direction: north, sprite: "1002"`,
		`name: "Job#v", map: prontera, x: 150, y: 150, direction: north, sprite: "4056"`} {
		if !strings.Contains(yml, w) {
			t.Errorf("no %q in\n%s", w, yml)
		}
	}
	for _, n := range res.NPCs {
		if n.Source == "None#v" && !strings.Contains(strings.Join(n.Unsupported, "; "), "sprite: 6002 is not an NPC sprite") {
			t.Errorf("None#v: %v", n.Unsupported)
		}
	}
}
