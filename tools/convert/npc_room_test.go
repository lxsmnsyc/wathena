package convert

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// fakeRoom is a waiting room of the test host.
type fakeRoom struct {
	spec              modscript.RoomSpec
	disabled          bool
	enabled, disables int
	kicked            []string
	allKicked         int
	warps             []string
}

// roomHost records waiting rooms for npcHost.
type roomHost struct {
	rooms map[int64]*fakeRoom
	byNPC map[int64]int64
}

func (h *npcHost) rooms() *roomHost {
	if h.room.rooms == nil {
		h.room.rooms, h.room.byNPC = map[int64]*fakeRoom{}, map[int64]int64{}
	}
	return &h.room
}

func (h *npcHost) OpenChatRoom(npc int64, s modscript.RoomSpec) (int64, error) {
	rh := h.rooms()
	if _, ok := rh.byNPC[npc]; ok {
		return 0, nil
	}
	id := int64(len(rh.rooms) + 1)
	rh.rooms[id], rh.byNPC[npc] = &fakeRoom{spec: s}, id
	return id, nil
}

func (h *npcHost) CloseChatRoom(npc int64) (bool, error) {
	rh := h.rooms()
	id, ok := rh.byNPC[npc]
	delete(rh.rooms, id)
	delete(rh.byNPC, npc)
	return ok, nil
}

func (h *npcHost) NPCChatRoom(npc int64) (int64, error) { return h.rooms().byNPC[npc], nil }

func (h *npcHost) ChatRoomField(id int64, field string) (script.Value, error) {
	r := h.rooms().rooms[id]
	if r == nil {
		return script.Nil, errors.New("the chat room is gone")
	}
	switch field {
	case "title":
		return script.String(r.spec.Title), nil
	case "limit":
		return script.Int(int64(r.spec.Limit)), nil
	case "trigger":
		if r.spec.Trigger < 0 {
			return script.Int(int64(r.spec.Limit)), nil
		}
		return script.Int(int64(r.spec.Trigger)), nil
	case "trigger_disabled":
		return script.Bool(r.disabled), nil
	case "event":
		return script.String(r.spec.Event), nil
	case "users":
		return script.Int(2), nil
	case "password":
		return script.String(""), nil
	case "full":
		return script.Bool(true), nil
	case "triggered":
		return script.Bool(false), nil
	}
	return script.Nil, errors.New("no field " + field)
}

func (h *npcHost) ChatRoomMembers(int64) ([]int64, error) { return []int64{1}, nil }

func (h *npcHost) ChatRoomEvent(id int64, enable bool) error {
	r := h.rooms().rooms[id]
	r.disabled = !enable
	if enable {
		r.enabled++
	} else {
		r.disables++
	}
	return nil
}

func (h *npcHost) ChatRoomKick(id int64, name string) (bool, error) {
	r := h.rooms().rooms[id]
	r.kicked = append(r.kicked, name)
	return true, nil
}

func (h *npcHost) ChatRoomKickAll(id int64) (int, error) {
	h.rooms().rooms[id].allKicked++
	return 2, nil
}

func (h *npcHost) ChatRoomWarp(id int64, mapName string, x, y, n int) ([]int64, error) {
	r := h.rooms().rooms[id]
	r.warps = append(r.warps, fmt.Sprintf("%s,%d,%d,%d", mapName, x, y, n))
	return []int64{1}, nil
}

func (h *npcHost) OpenPlayerRoom(int64, string, string, int, bool, []int64) (int64, error) {
	return 0, errors.New("not in this test")
}

// TestWaitingRoomConverts checks the converted text of the waiting room
// commands and their refusals.
func TestWaitingRoomConverts(t *testing.T) {
	res := shopResult(t)
	for _, c := range []struct {
		script string
		want   string
	}{
		{`{ waitingroom "Arena",10; end; }`, `npc:open_chat_room({ title = "Arena", limit = 10 })`},
		{`{ waitingroom "Arena",10,"Arena Room::OnStart",2,100,50,99; end; }`,
			`event = "Arena Room::OnStart", trigger = 2, zeny = 100, min_level = 50, max_level = 99 })`},
		{`{ waitingroom "Arena",10,"",5; end; }`, `npc:open_chat_room({ title = "Arena", limit = 10, trigger = 5 })`},
		{`{ delwaitingroom; end; }`, `R.room_close(npc)`},
		{`{ delwaitingroom "Red"; end; }`, `R.room_close("Red")`},
		{`{ kickwaitingroomall "Red"; end; }`, `R.room_kick_all("Red")`},
		{`{ waitingroomkick "Red","Bob"; end; }`, `R.room_kick("Red", "Bob")`},
		{`{ enablewaitingroomevent; end; }`, `R.room_event(npc, true)`},
		{`{ disablewaitingroomevent "Red"; end; }`, `R.room_event("Red", false)`},
		{`{ warpwaitingpc "prt_are01",150,150; end; }`, `R.warpwaitingpc(npc, "prt_are01", 150, 150, nil)`},
		{`{ warpwaitingpc "job_duncer",69,110,1; end; }`, `R.warpwaitingpc(npc, "job_duncer", 69, 110, 1)`},
		{`{ .@n = getwaitingroomstate(0,"Red"); end; }`, `R.waitingroomstate(0, "Red")`},
		{`{ .@t$ = getwaitingroomstate(4); end; }`, `R.waitingroomstate(4, npc)`},
		{`{ if (checkchatting()) end; end; }`, `player.in_chat_room`},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "Arena Room#t", Map: "prontera", Script: c.script})
		lua, err := n.npcScript()
		if err != nil {
			t.Errorf("%s: %v", c.script, err)
			continue
		}
		if !strings.Contains(lua, c.want) {
			t.Errorf("%s: got\n%s\nwant %q", c.script, lua, c.want)
		}
	}
	for _, c := range []struct{ script, want string }{
		{`{ waitingroom "Arena"; end; }`, "waitingroom: arguments"},
		{`{ waitingroom 5,10; end; }`, "waitingroom: a title that is not text"},
		{`{ waitingroom "Arena",10,"NoLabel"; end; }`, "is not an event label NPC::OnX"},
		{`{ .@e$ = "A::OnX"; waitingroom "Arena",10,.@e$; end; }`, "waitingroom: a computed event label"},
		{`{ warpwaitingpc "x",1; end; }`, "warpwaitingpc: arguments"},
		{`{ waitingroomkick "Red"; end; }`, "waitingroomkick: arguments"},
		{`{ .@n = checkchatting(150000); end; }`, "checkchatting: another character"},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		_, err := n.npcScript()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.script, err, c.want)
		}
	}
}

// TestRunWaitingRoom runs converted waiting room commands against the
// real runtime and checks what the host got, and the variables
// warpwaitingpc fills.
func TestRunWaitingRoom(t *testing.T) {
	testrace.SkipUnderRace(t, "it converts or runs all of core's scripts on one goroutine")
	res := shopResult(t)
	n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "Run#t", Map: "prontera", Script: `{
	waitingroom "Arena",10,"Run::OnGo",2,100,5,60;
	waitingroom "again",3;
	$@r0 = getwaitingroomstate(0);
	$@r1 = getwaitingroomstate(1);
	$@r2 = getwaitingroomstate(2);
	$@r3 = getwaitingroomstate(3);
	$@r32 = getwaitingroomstate(32);
	$@r99 = getwaitingroomstate(99);
	$@rno = getwaitingroomstate(0,"Nobody");
	if (getwaitingroomstate(4) == "Arena") $@r4 = 1;
	enablewaitingroomevent;
	disablewaitingroomevent;
	waitingroomkick "Run#t","Bob";
	kickwaitingroomall;
	warpwaitingpc "prt_are01",150,151;
	warpwaitingpc "Random",0,0,1;
	end;
}`})
	lua, err := n.npcScript()
	if err != nil {
		t.Fatal(err)
	}
	_, base, core := testRuntime(t, reference.Renewal)
	h := &npcHost{testHost: base, npcs: map[int64]*NPC{1: {Source: "Run#t", Map: "prontera"}}, steps: map[int64]modscript.Step{},
		ended: map[int64]bool{}, vars: map[modscript.VariableKey]string{}, byName: map[string]int64{"Run#t": 1}}
	sc := &mod.Scripts{Mod: mod.Core, Files: map[string]string{"scripts/npcs/Run_t.lua": lua, RathenaLib: rathenaLua}, Grants: mod.Grants{}, Visible: []string{mod.Core}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	logs := &logRecorder{}
	srv, temp := variables.NewServer(nil, nil), variables.NewTemporary()
	rt, err := modscript.New(modscript.Options{Host: h, Mods: []string{mod.Core}, Constants: core.Constants, Jobs: base.jobs,
		Log: slog.New(logs), Server: srv, Temporary: temp, Clock: func() time.Time { return testClock }, Location: time.UTC},
		[]*mod.Scripts{sc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	npc, err := rt.StartNPC(1, "core:scripts/npcs/Run_t.lua")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.Click(npc, 1); err != nil {
		t.Fatal(err)
	}
	for _, m := range logs.msgs {
		t.Error(m)
	}
	r := h.rooms().rooms[1]
	if r == nil || len(h.rooms().rooms) != 1 {
		t.Fatalf("rooms %+v: a second waitingroom is ignored", h.rooms().rooms)
	}
	want := modscript.RoomSpec{Title: "Arena", Limit: 10, Event: "Run::OnGo", Mod: "core", Trigger: 2, Zeny: 100, MinLevel: 5, MaxLevel: 60}
	if r.spec != want {
		t.Errorf("spec %+v, want %+v", r.spec, want)
	}
	if r.enabled != 1 || r.disables != 1 || len(r.kicked) != 1 || r.kicked[0] != "Bob" || r.allKicked != 1 {
		t.Errorf("event %d/%d kicks %v all %d", r.enabled, r.disables, r.kicked, r.allKicked)
	}
	// warpwaitingpc warps the trigger's number of players: 2 here.
	if len(r.warps) != 2 || r.warps[0] != "prt_are01,150,151,2" || r.warps[1] != "Random,0,0,1" {
		t.Errorf("warps %v", r.warps)
	}
	// The answers, and $@warpwaitingpc and $@warpwaitingpcnum.
	for name, want := range map[string]string{"r0": "2", "r1": "10", "r2": "2", "r3": "", "r4": "1", "r32": "1", "r99": "-1", "rno": "-1",
		"warpwaitingpcnum": "1"} {
		k := variables.TempKey{Mod: "core", Name: name}
		if v, _ := temp.Get(k); v != want {
			t.Errorf("$@%s = %q, want %q", name, v, want)
		}
	}
}
