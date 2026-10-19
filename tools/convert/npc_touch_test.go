package convert

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// touchNPCs are NPCs written for the tests of OnTouch and OnTouch_
// (review decisions 55 and 56). Their names are the keys.
var touchNPCs = map[string]string{
	// A warp portal script, as rAthena's warps folders have.
	"Touch Warp#t": `{
	end;
OnTouch_:
	warp "prontera",150,180;
	end;
}`,
	// OnTouch runs for each player, beside a click.
	"Touch Each#t": `{
	mes "[Guard]";
	mes "Halt.";
	close;
OnTouch:
	npctalk "Welcome, " + strcharinfo(0) + ".";
	end;
}`,
	// OnTouch_ wins over OnTouch, as npc_touch_areanpc runs OnTouch only
	// without OnTouch_.
	"Touch Both#t": `{ end; OnTouch: npctalk "each"; end; OnTouch_: npctalk "one"; end; }`,
	// A dialog in OnTouch uses the touch's conversation.
	"Touch Talk#t": `{ end; OnTouch: mes "Hello."; close; }`,
	// So does a function with a dialog. R.touch_one knows of it.
	"Touch Func#t": `{ end; OnTouch_: callfunc "F_TouchTalk"; end; }`,
	// A function without one too.
	"Touch Quiet#t": `{ end; OnTouch_: callfunc "F_TouchQuiet"; end; }`,
	// An NPC without a sprite still has its area: HIDDEN_NPC.
	"Touch Hidden#t": `{ end; OnTouch: npctalk "boo"; end; }`,
	// A span of -1 has no area, so the record has no touch field.
	"Touch No Span#t": `{ end; OnTouch: npctalk "never"; end; }`,
}

// touchFunctions are the functions the touchNPCs call.
var touchFunctions = map[string]string{
	"F_TouchTalk":  `{ mes "Hi."; close; }`,
	"F_TouchQuiet": `{ npctalk "quiet"; return; }`,
}

// touchResult converts the touchNPCs, on prontera.
func touchResult(t *testing.T) *NPCResult {
	t.Helper()
	read := func(name string) []byte {
		b, err := reference.ReadFile("extracts/renewal/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var lines []npcLine
	for _, name := range sortedKeys(touchFunctions) {
		lines = append(lines, npcLine{Kind: "function", Name: name, File: "test.txt", Line: 1, Script: touchFunctions[name]})
	}
	for _, name := range sortedKeys(touchNPCs) {
		l := npcLine{Kind: "script", Name: name, Display: name, File: "test.txt", Line: 1, Map: "prontera",
			X: 150, Y: 150, Sprite: "WARPNPC", XS: 1, YS: 1, Script: touchNPCs[name]}
		switch name {
		case "Touch Each#t":
			l.Sprite, l.XS, l.YS = "4_M_JOB_KNIGHT1", 3, 2
		case "Touch Hidden#t":
			l.Sprite = "-1"
		case "Touch No Span#t":
			l.Sprite, l.XS, l.YS = "4_M_JOB_KNIGHT1", -1, -1
		}
		lines = append(lines, l)
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

func TestNPCTouchConvert(t *testing.T) {
	res := touchResult(t)
	want := map[string]struct{ unsupported, record string }{
		"Touch Warp#t":    {"", `sprite: WARPNPC, display_name: Touch Warp, script: scripts/npcs/Touch_Warp_t.lua, touch: {x_span: 1, y_span: 1}}`},
		"Touch Each#t":    {"", `touch: {x_span: 3, y_span: 2}}`},
		"Touch Both#t":    {"", `touch: {x_span: 1, y_span: 1}}`},
		"Touch Talk#t":    {"", `touch: {x_span: 1, y_span: 1}}`},
		"Touch Func#t":    {"", `touch: {x_span: 1, y_span: 1}}`},
		"Touch Quiet#t":   {"", `touch: {x_span: 1, y_span: 1}}`},
		"Touch Hidden#t":  {"", `sprite: HIDDEN_NPC, display_name: Touch Hidden, script: scripts/npcs/Touch_Hidden_t.lua, touch: {x_span: 1, y_span: 1}}`},
		"Touch No Span#t": {"", `script: scripts/npcs/Touch_No_Span_t.lua}`},
	}
	for _, n := range res.NPCs {
		w := want[n.Source]
		if got := strings.Join(n.Unsupported, "; "); got != w.unsupported {
			t.Errorf("%s: unsupported %q, want %q", n.Source, got, w.unsupported)
		}
		if !strings.HasSuffix(n.record, w.record) || (w.record == "") != (n.record == "") {
			t.Errorf("%s: record %q, want it to end with %q", n.Source, n.record, w.record)
		}
		if n.Touch != strings.Contains(w.record, "touch:") {
			t.Errorf("%s: Touch %v", n.Source, n.Touch)
		}
	}
	for name, wantLine := range map[string]string{
		"Touch Warp#t":  "R.touch_one(event_ontouch_)",
		"Touch Each#t":  `events.on("touch", event_ontouch)`,
		"Touch Both#t":  "R.touch_one(event_ontouch_)",
		"Touch Quiet#t": "R.touch_one(event_ontouch_)",
		"Touch Talk#t":  `events.on("touch", event_ontouch)`,
		"Touch Func#t":  "R.touch_one(event_ontouch_, true)",
	} {
		var n *NPC
		for _, x := range res.NPCs {
			if x.Source == name {
				n = x
			}
		}
		if !strings.Contains(n.Lua, wantLine) {
			t.Errorf("%s: no %q in\n%s", name, wantLine, n.Lua)
		}
		if name == "Touch Both#t" && strings.Contains(n.Lua, "event_ontouch =") {
			t.Errorf("%s: OnTouch converted beside OnTouch_:\n%s", name, n.Lua)
		}
		if n.TouchDialog != (name == "Touch Talk#t" || name == "Touch Func#t") {
			t.Errorf("%s: TouchDialog %v", name, n.TouchDialog)
		}
	}
	for _, name := range []string{"Touch Warp#t", "Touch Each#t", "Touch Talk#t"} {
		var n *NPC
		for _, x := range res.NPCs {
			if x.Source == name {
				n = x
			}
		}
		path := filepath.Join("testdata", "golden", "npcs", "touch_"+strings.NewReplacer("#", "_", " ", "_").Replace(name)+".lua")
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
			t.Errorf("%s changed; run go test ./tools/convert -run TestNPCTouchConvert -update and review the diff:\n%s", path, n.Lua)
		}
	}
}
