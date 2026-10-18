package convert

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/variables"
)

func TestClockSchedule(t *testing.T) {
	for _, c := range []struct {
		label, want string
		ok          bool
		err         string
	}{
		{"OnClock0000", `schedules.daily("00:00", f)`, true, ""},
		{"OnClock2359", `schedules.daily("23:59", f)`, true, ""},
		{"OnHour06", `schedules.daily("06:00", f)`, true, ""},
		{"OnMinute30", `schedules.hourly(30, f)`, true, ""},
		{"OnMinute00", `schedules.hourly(0, f)`, true, ""},
		{"OnSun0000", `schedules.weekly("sunday", "00:00", f)`, true, ""},
		{"OnMon1230", `schedules.weekly("monday", "12:30", f)`, true, ""},
		{"OnTue0101", `schedules.weekly("tuesday", "01:01", f)`, true, ""},
		{"OnWed0101", `schedules.weekly("wednesday", "01:01", f)`, true, ""},
		{"OnThu0101", `schedules.weekly("thursday", "01:01", f)`, true, ""},
		{"OnFri0101", `schedules.weekly("friday", "01:01", f)`, true, ""},
		{"OnSat2200", `schedules.weekly("saturday", "22:00", f)`, true, ""},
		{"OnDay0101", "", true, "a yearly schedule (OnDay) is not in the API"},
		{"OnClock2400", "", true, "not a valid time"},
		{"OnClock000", "", true, "not a valid time"},
		{"OnHour24", "", true, "not a valid time"},
		{"OnMinute60", "", true, "not a valid time"},
		{"OnMinute5", "", true, "not a valid time"},
		{"OnInit", "", false, ""},
		{"OnMonsterDead", "", false, ""},
		{"OnHourly", "", false, ""},
		{"OnTimer1000", "", false, ""},
	} {
		format, ok, err := clockSchedule(c.label)
		got := ""
		if format != "" {
			got = strings.Replace(format, "%s", "f", 1)
		}
		if got != c.want || ok != c.ok || (err == nil) != (c.err == "") || err != nil && !strings.Contains(err.Error(), c.err) {
			t.Errorf("%s: got %q, %v, %v; want %q, %v, %q", c.label, got, ok, err, c.want, c.ok, c.err)
		}
	}
}

// TestSegmentsFallThrough checks that an event's part runs on into the
// next label until an end.
func TestSegmentsFallThrough(t *testing.T) {
	stmts, err := ParseNPC("{ mes \"a\"; close; OnInit: OnClock0000: .x = 1; OnSat2200: .y = 2; end; OnMinute10: .z = 3; }")
	if err != nil {
		t.Fatal(err)
	}
	_, events, order := segments(stmts.Stmts)
	if strings.Join(order, ",") != "oninit,onclock0000,onsat2200,onminute10" {
		t.Fatalf("order %v", order)
	}
	for label, want := range map[string]int{"oninit": 5, "onclock0000": 4, "onsat2200": 2, "onminute10": 1} {
		if got := len(events[label]); got != want {
			t.Errorf("%s: %d statements, want %d", label, got, want)
		}
	}
}

// eventNPCs are NPCs written for the tests of clock labels and the
// monster command. Their names are the keys.
var eventNPCs = map[string]string{
	"Clock Test#t": `{
	mes "[Clock]";
	mes "Hello.";
	close;
OnInit:
OnClock0000:
	announce "Midnight or start", bc_all;
	end;
OnHour06:
	announce "Six", bc_all;
	end;
OnMinute30:
	announce "Half past", bc_all;
	end;
OnSat2200:
	announce "Saturday night", bc_all;
	end;
}`,
	"Mob Test#t": `{
	mes "[Mobs]";
	mes "There are " + mobcount("prontera", "Mob Test#t::OnMyMobDead") + " left.";
	close;
OnInit:
	monster "prontera",0,0,"--ja--",1002,2,"Mob Test#t::OnMyMobDead";
	monster "prontera",150,160,"Poring","PORING",1,strnpcinfo(3)+"::OnMyMobDead";
	monster "prontera",150,160,"",1002,1;
	end;
OnMyMobDead:
	if (mobcount("prontera", "Mob Test#t::OnMyMobDead") < 1) {
		announce "All dead", bc_all;
		monster "prontera",0,0,"--en--",1002,1,"Mob Test#t::OnOther";
	}
	end;
OnOther:
	dispbottom "Killed the last one";
	end;
}`,
	"Day Test#t":       `{ end; OnDay0101: announce "New year", bc_all; end; }`,
	"Bad Clock#t":      `{ end; OnClock2460: end; }`,
	"Other Map#t":      `{ monster "geffen",0,0,"--ja--",1002,1; end; }`,
	"Own Name#t":       `{ monster "prontera",0,0,"Bob",1002,1; end; }`,
	"Other NPC#t":      `{ monster "prontera",0,0,"--ja--",1002,1,"Someone::OnMyMobDead"; end; OnMyMobDead: end; }`,
	"Global Label#t":   `{ monster "prontera",0,0,"--ja--",1002,1,"::OnMyMobDead"; end; OnMyMobDead: end; }`,
	"No Label#t":       `{ monster "prontera",0,0,"--ja--",1002,1,"No Label#t::OnGone"; end; }`,
	"Big Size#t":       `{ monster "prontera",0,0,"--ja--",1002,1,"",2; end; }`,
	"Kill Test#t":      `{ killmonster "prontera","Kill Test#t::OnMyMobDead"; end; OnMyMobDead: end; }`,
	"Random Monster#t": `{ monster "prontera",0,0,"--ja--",-1,1; end; }`,
}

// eventResult converts the eventNPCs, on prontera.
func eventResult(t *testing.T) *NPCResult {
	t.Helper()
	read := func(name string) []byte {
		b, err := reference.ReadFile("extracts/renewal/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var lines []npcLine
	for _, name := range sortedKeys(eventNPCs) {
		lines = append(lines, npcLine{Kind: "script", Name: name, Display: name, File: "test.txt", Line: 1, Map: "prontera",
			X: 150, Y: 150, Sprite: "4_F_KAFRA1", Script: eventNPCs[name]})
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
	return res
}

func TestNPCEventsConvert(t *testing.T) {
	res := eventResult(t)
	want := map[string]string{
		"Clock Test#t":     "",
		"Mob Test#t":       "",
		"Day Test#t":       "event onday0101: a yearly schedule (OnDay) is not in the API",
		"Bad Clock#t":      "event onclock2460: 2460 is not a valid time",
		"Other Map#t":      "monster: on another map",
		"Own Name#t":       "monster: a name of its own",
		"Other NPC#t":      "event onmymobdead",
		"Global Label#t":   "event onmymobdead",
		"No Label#t":       "monster: OnGone is not an event label of this NPC",
		"Big Size#t":       "monster: a size other than the default",
		"Kill Test#t":      "event onmymobdead",
		"Random Monster#t": "monster: -1 is not a monster, such as a random monster",
	}
	for _, n := range res.NPCs {
		got := strings.Join(n.Unsupported, "; ")
		if got != want[n.Source] {
			t.Errorf("%s: unsupported %q, want %q", n.Source, got, want[n.Source])
		}
	}
	// The labels of other NPCs fail in the monster command, once the
	// label is not an event of this NPC.
	for _, c := range []struct{ script, want string }{
		{`{ monster "prontera",0,0,"--ja--",1002,1,"Someone::OnMyMobDead"; end; }`, "an event label of another NPC (Someone)"},
		{`{ monster "prontera",0,0,"--ja--",1002,1,"::OnMyMobDead"; end; }`, "a global event label (::OnMyMobDead)"},
		{`{ killmonster "prontera","All"; end; }`, "the API cannot remove spawned monsters"},
		{`{ mes "" + mobcount("prontera", "all"); end; }`, `"all" is not an event label`},
	} {
		c2 := res.conv
		n := c2.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		_, err := n.npcScript()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.script, err, c.want)
		}
	}
	for _, name := range []string{"Clock Test#t", "Mob Test#t"} {
		var n *NPC
		for _, x := range res.NPCs {
			if x.Source == name {
				n = x
			}
		}
		path := filepath.Join("testdata", "golden", "npcs", "events_"+strings.NewReplacer("#", "_", " ", "_").Replace(name)+".lua")
		if *update {
			if err := os.WriteFile(path, []byte(n.Lua), 0o600); err != nil {
				t.Fatal(err)
			}
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != n.Lua {
			t.Errorf("%s changed; run go test ./tools/convert -run TestNPCEventsConvert -update and review the diff:\n%s", path, n.Lua)
		}
	}
}

// eventHost records announcements and messages.
type eventHost struct {
	*npcHost
	said []string
}

func (h *eventHost) Announce(_, _, text string) error { h.said = append(h.said, text); return nil }
func (h *eventHost) SendMessage(_ int64, text string) error {
	h.said = append(h.said, "message: "+text)
	return nil
}

// TestRunNPCEvents runs the converted eventNPCs: schedules at their
// times, and monsters that die.
func TestRunNPCEvents(t *testing.T) {
	testrace.SkipUnderRace(t, "it converts or runs all of core's scripts on one goroutine")
	res := eventResult(t)
	_, base, core := testRuntime(t, reference.Renewal)
	h := &eventHost{npcHost: &npcHost{testHost: base, npcs: map[int64]*NPC{}, steps: map[int64]modscript.Step{}, ended: map[int64]bool{},
		vars: map[modscript.VariableKey]string{}}}
	sc := &mod.Scripts{Mod: mod.Core, Files: map[string]string{}, Grants: mod.Grants{}, Visible: []string{mod.Core}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	for p, content := range res.Files() {
		if strings.HasPrefix(p, "scripts/") {
			sc.Files[p] = content
		}
	}
	logs := &logRecorder{}
	clock := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC) // A Friday.
	rt, err := modscript.New(modscript.Options{Host: h, Mods: []string{mod.Core}, Constants: core.Constants, Jobs: base.jobs,
		Log: slog.New(logs), Server: variables.NewServer(nil, nil), Clock: func() time.Time { return clock }, Location: time.UTC},
		[]*mod.Scripts{sc})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Close)
	start := func(name string) {
		for i, n := range res.NPCs {
			if n.Source == name {
				h.npcs[int64(i+1)] = n
				if _, err := rt.StartNPC(int64(i+1), "core:"+n.Script); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	check := func(what string, want ...string) {
		t.Helper()
		if strings.Join(h.said, "|") != strings.Join(want, "|") {
			t.Errorf("%s: said %q, want %q", what, h.said, want)
		}
		h.said = nil
		for _, m := range logs.msgs {
			t.Error(m)
		}
		logs.msgs = nil
	}

	start("Clock Test#t")
	check("OnInit", "Midnight or start")
	rt.RunSchedules(time.Date(2026, 1, 2, 12, 30, 0, 0, time.UTC))
	check("12:30", "Half past")
	rt.RunSchedules(time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC))
	// Half past runs every hour, and once for the hours it missed.
	check("midnight", "Half past", "Midnight or start")
	rt.RunSchedules(time.Date(2026, 1, 3, 6, 0, 0, 0, time.UTC))
	check("06:00", "Half past", "Six")
	rt.RunSchedules(time.Date(2026, 1, 3, 22, 0, 0, 0, time.UTC))
	check("Saturday 22:00", "Half past", "Saturday night")

	start("Mob Test#t")
	check("spawn")
	if len(h.spawned) != 4 {
		t.Fatalf("spawned %v, want 4 monsters", h.spawned)
	}
	kill := func(id int64) {
		mob := h.spawned[id]
		delete(h.spawned, id)
		rt.FireDeath(modscript.Death{Victim: id, VictimKind: "monster", Monster: int64(mob), Killer: 1, KillerKind: "player"})
	}
	// Monster 4 has no label: its death runs nothing.
	kill(4)
	kill(1)
	kill(2)
	check("two of three")
	kill(3)
	check("the last labeled one", "All dead")
	if len(h.spawned) != 1 {
		t.Fatalf("spawned %v, want 1 monster", h.spawned)
	}
	kill(5)
	check("the one of OnOther", "message: Killed the last one")
}
