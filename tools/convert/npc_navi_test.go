package convert

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/internal/testrace"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/variables"
)

// Navigate records the navigations of player:navigate_to.
func (h *npcHost) Navigate(_ int64, to modscript.NaviTarget) (bool, error) {
	h.navis = append(h.navis, to)
	return true, nil
}

// TestNavigatetoConverts checks the converted text of navigateto: the
// arguments in rAthena's order, the flag as a constant of wa.navigation,
// and the refusals.
func TestNavigatetoConverts(t *testing.T) {
	res := shopResult(t)
	for _, c := range []struct {
		script string
		want   string
	}{
		{`{ navigateto "prontera"; end; }`, `player:navigate_to("prontera")`},
		{`{ navigateto "prontera",150,160; end; }`, `player:navigate_to("prontera", { x = 150, y = 160 })`},
		{`{ navigateto "prontera",150,160,NAV_ALL; end; }`, `player:navigate_to("prontera", { x = 150, y = 160, services = navigation.all })`},
		{`{ navigateto "geffen",120,100,NAV_KAFRA_AND_AIRSHIP,0; end; }`, `{ x = 120, y = 100, services = navigation.kafra_and_airship, hide_window = false }`},
		{`{ navigateto "geffen",0,0,0,1,1002; end; }`, `{ x = 0, y = 0, services = navigation.none, hide_window = true, monster = 1002 }`},
		{`{ navigateto "geffen",0,0,100; end; }`, `services = navigation.kafra_only`},
		{`{ navigateto "geffen",0,0,7; end; }`, `services = 7`},
		{`{ .@h = 1; navigateto "geffen",1,2,0,.@h; end; }`, `hide_window = (h ~= 0)`},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
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
		{`{ navigateto "prontera",1,2,0,1,0,150000; end; }`, "navigateto: for another character"},
		{`{ navigateto; end; }`, "navigateto: arguments"},
		{`{ navigateto 5; end; }`, "navigateto: a map that is not text"},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		_, err := n.npcScript()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.script, err, c.want)
		}
	}
}

// TestRunNavigateto runs converted navigateto calls against the real
// runtime and checks what the host got, including the defaults.
func TestRunNavigateto(t *testing.T) {
	testrace.SkipUnderRace(t, "it converts or runs all of core's scripts on one goroutine")
	res := shopResult(t)
	n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "Run#t", Map: "prontera", Script: `{
	navigateto "prontera";
	navigateto "geffen",120,100,NAV_ALL,0;
	navigateto "payon",0,0,NAV_NONE,1,1002;
	end;
}`})
	lua, err := n.npcScript()
	if err != nil {
		t.Fatal(err)
	}
	_, base, core := testRuntime(t, reference.Renewal)
	h := &npcHost{testHost: base, npcs: map[int64]*NPC{1: {Source: "Run#t", Map: "prontera"}}, steps: map[int64]modscript.Step{},
		ended: map[int64]bool{}, vars: map[modscript.VariableKey]string{}}
	sc := &mod.Scripts{Mod: mod.Core, Files: map[string]string{"scripts/npcs/Run_t.lua": lua, RathenaLib: rathenaLua}, Grants: mod.Grants{}, Visible: []string{mod.Core}}
	for _, p := range mod.Permissions {
		sc.Grants[p.Name] = true
	}
	logs := &logRecorder{}
	rt, err := modscript.New(modscript.Options{Host: h, Mods: []string{mod.Core}, Constants: core.Constants, Jobs: base.jobs,
		Log: slog.New(logs), Server: variables.NewServer(nil, nil), Clock: func() time.Time { return testClock }, Location: time.UTC},
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
	want := []modscript.NaviTarget{
		{Map: "prontera", Services: 101, HideWindow: true},
		{Map: "geffen", X: 120, Y: 100, Services: 111},
		{Map: "payon", Services: 0, Monster: 1002, HideWindow: true},
	}
	if len(h.navis) != len(want) {
		t.Fatalf("navigations %+v, want %+v", h.navis, want)
	}
	for i := range want {
		if h.navis[i] != want[i] {
			t.Errorf("navigation %d: %+v, want %+v", i, h.navis[i], want[i])
		}
	}
}
