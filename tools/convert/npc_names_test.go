package convert

import (
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/modscript"
)

// TestNamesConvert checks the converted text of the names and forms of
// converter-names, and the reasons of the forms that stay refused. A want
// that starts with "!" is the reason.
func TestNamesConvert(t *testing.T) {
	res := builtinResult(t, nil)
	for _, c := range []struct{ script, want string }{
		// Names that start with a digit (review decision 60, F1).
		{`{ $@3rdgc_room01 = 4; $2012_x$ = "a"; end; }`, `variables.server_temporary_integer "3rdgc_room01"`},
		{`{ $2012_x$ = "a"; end; }`, `variables.server_string "2012_x_text"`},
		{`{ .@3x = 1; .y3 = 2; end; }`, "v3x = 1"},
		{`{ setd "$3rd_" + .@i, 1; end; }`, `R.setd("server", "$3rd_" .. tostring(i), player, 1)`},
		{`{ mes "" + getd("$3rd_x"); end; }`, `SERVER_N3RD_X:get()`},
		// An assignment used as a value (F2).
		{`{ if ((.@a = 5) == 5) mes "x"; end; }`, "(function() a = 5 return a end)() == 5"},
		{`{ .@d = .@c++; .@e = ++.@c; end; }`, "(function() local wa_old = c c = (c + 1) return wa_old end)()"},
		{`{ .@a[.@i++] = 1; end; }`, "a[(function() local wa_old = i i = (i + 1) return wa_old end)()] = 1"},
		{`{ .@g = .@h = 2; end; }`, "g = (function() h = 2 return h end)()"},
		{`{ $@v = ($@w = 3) + 1; end; }`, "SERVER_TEMP_V:set(((function() SERVER_TEMP_W:set(3) return SERVER_TEMP_W:get() end)() + 1))"},
		// getarg and getargcount outside a function end the script.
		{`{ mes "" + getarg(0); end; }`, "(R.stop() or 0)"},
		{`{ mes "" + getargcount(); end; }`, "(R.stop() or 0)"},
		{`{ mes "" + getarg(); end; }`, "!getarg: arguments"},
		// getd and setd of computed names (F3).
		{`{ setd ".@v" + .@i, 3; mes "" + getd(".@v" + .@i); .@v1 = 1; end; }`, "R.setd_local(locals_"},
		{`{ mes "" + getd(".@v" + .@i); .@v1 = 1; end; }`, `R.getd_local(locals_1, ".@v" .. tostring(locals_1["i"]))`},
		{`{ .@v1 = 1; setd ".@v" + .@i, 3; end; }`, `locals_1["v1"] = 0`},
		{`{ .@s$ = "a"; setd ".@v" + .@i, 3; end; }`, `locals_1["s$"] = ""`},
		{`{ .x = 1; setd ".y" + .@i, 2; end; }`, `local NPC_X = variables.npc_integer "x"`},
		{`{ .x = 1; mes "" + getd(".y" + .@i); end; }`, `R.getd("npc", ".y" .. tostring(i), player, npc)`},
		{`{ setd .@n$, 1; end; }`, `R.setd_any(locals_1["n$"], player, npc, locals_1, 1)`},
		{`{ mes "" + getd("." + .@n$); .x = 1; end; }`, `R.getd_any("." .. locals_1["n$"], player, npc, locals_1)`},
		{`{ mes "" + getd("." + .@n$); .x = 1; end; }`, `local NPC_X = variables.npc_integer "x"`},
		{`{ mes "" + getd("$" + .@n$); end; }`, "R.getd_any("},
		{`{ setd "'x" + .@i, 1; end; }`, "!variables: 'x... is an instance variable"},
		{`{ setd "Zen" + .@i, 1; end; }`, "!setd: a name that may be"},
		// sscanf.
		{`{ sscanf(strnpcinfo(2), "f_boss%d", .@num); end; }`, `scan_1, scanned_2 = R.sscanf(`},
		{`{ .@r = sscanf("a b", "%s %s", .@a$, .@b$); end; }`, `R.sscanf("a b", "%s %s")`},
		{`{ sscanf(.@s$, .@f$, .@num); end; }`, "!sscanf: a computed format"},
		{`{ sscanf("1", "%d", .@s$); end; }`, "!sscanf: a conversion that does not match the type of .@s$"},
		{`{ sscanf("1", "%x", .@n); end; }`, "!sscanf: a conversion other than %d and %s"},
		{`{ sscanf("1", "%d %d", .@n); end; }`, "!sscanf: a different number of conversions and variables"},
		// Labels inside blocks: a jump from outside enters the block.
		{`{ .@m = 2; if (.@m == 1) { mes "a"; L_In: mes "b"; } else if (.@m == 2) { goto L_In; } end; }`, "::wa_enter_l_in::"},
		{`{ .@m = 2; if (.@m == 1) { mes "a"; L_In: mes "b"; } else if (.@m == 2) { goto L_In; } end; }`, "goto wa_enter_l_in"},
		{`{ goto L_In; while (.@a) { L_In: end; } end; }`, "!labels: l_in: a label inside a block, which a jump from outside the block reaches"},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", XS: -1, YS: -1, Script: c.script})
		lua, err := n.npcScript()
		if reason, ok := strings.CutPrefix(c.want, "!"); ok {
			if err == nil || !strings.Contains(err.Error(), reason) {
				t.Errorf("%s: got %v, want %q", c.script, err, reason)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.script, err)
			continue
		}
		if !strings.Contains(lua, c.want) {
			t.Errorf("%s: got\n%s\nwant %q", c.script, lua, c.want)
		}
	}
}

// TestClickOnTouchConvert checks the click fallback of review decision
// 60, E2: an NPC with a touch area and no touch label clicks on touch,
// unless it has a label, no view, no area, or a duplicate's area.
func TestClickOnTouchConvert(t *testing.T) {
	res := builtinResult(t, nil)
	for _, c := range []struct {
		line npcLine
		want bool
	}{
		{npcLine{Name: "A#t", Sprite: "45", XS: 2, YS: 2, Script: `{ mes "hi"; close; }`}, true},
		{npcLine{Name: "B#t", Sprite: "45", XS: 0, YS: 0, Script: `{ mes "hi"; close; }`}, true},
		{npcLine{Name: "C#t", Sprite: "45", XS: -1, YS: -1, Script: `{ mes "hi"; close; }`}, false},
		{npcLine{Name: "D#t", Sprite: "-1", XS: 2, YS: 2, Script: `{ mes "hi"; close; }`}, false},
		{npcLine{Name: "E#t", Sprite: "45", XS: 2, YS: 2, Script: `{ mes "hi"; close; OnTouch: mes "t"; end; }`}, false},
		{npcLine{Name: "F#t", Sprite: "45", XS: 2, YS: 2, Script: `{ end; }`}, false},
		{npcLine{Name: "G#t", Sprite: "45", XS: 2, YS: 2, Script: `{ end; OnInit: end; }`}, false},
	} {
		c.line.Kind, c.line.Map = "script", "prontera"
		n := res.conv.newNPCConv(&c.line)
		lua, err := n.npcScript()
		if err != nil {
			t.Errorf("%s: %v", c.line.Name, err)
			continue
		}
		if got := n.clickTouch; got != c.want {
			t.Errorf("%s: click on touch %v, want %v", c.line.Name, got, c.want)
		}
		has := strings.Contains(lua, `events.on("touch", function(player, conversation, waited) if not waited and npc.state == "shown" then click(player, conversation) end end)`)
		if has != c.want {
			t.Errorf("%s: the touch handler is %v, want %v:\n%s", c.line.Name, has, c.want, lua)
		}
	}
	// A duplicate's touch area makes its source click on touch.
	res.conv.dupSpan["H#t"] = true
	defer delete(res.conv.dupSpan, "H#t")
	n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "H#t", Map: "prontera", Sprite: "45", XS: -1, YS: -1, Script: `{ mes "hi"; close; }`})
	if _, err := n.npcScript(); err != nil || !n.clickTouch {
		t.Errorf("a source with a duplicate that has an area: %v %v", n.clickTouch, err)
	}
}

// namesRun converts the scripts as NPCs, clicks each in turn, and
// returns what each showed with dispbottom, joined by "|", by name.
func namesRun(t *testing.T, scripts map[string]string) map[string]string {
	t.Helper()
	res := builtinResult(t, scripts)
	npcs := map[int64]*NPC{}
	for i, n := range res.NPCs {
		if n.Status != Full {
			t.Fatalf("%s: %s %v", n.Source, n.Status, n.Unsupported)
		}
		npcs[int64(i+1)] = n
	}
	_, base, _ := testRuntime(t, reference.Renewal)
	h := &eventHost{npcHost: &npcHost{testHost: base, npcs: npcs, steps: map[int64]modscript.Step{},
		ended: map[int64]bool{}, vars: map[modscript.VariableKey]string{}}}
	logs := &logRecorder{}
	rt := builtinRuntime(t, res, h, logs)
	out := map[string]string{}
	for i, n := range res.NPCs {
		npc, err := rt.StartNPC(int64(i+1), "core:"+n.Script)
		if err != nil {
			t.Fatal(err)
		}
		h.said = nil
		if _, err := rt.Click(npc, 1); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, s := range h.said {
			got = append(got, strings.TrimPrefix(s, "message: "))
		}
		out[n.Source] = strings.Join(got, "|")
	}
	for _, m := range logs.msgs {
		t.Error(m)
	}
	return out
}

// labelScript is a script that jumps into a block: m is the value of
// the mode that picks the branch.
func labelScript(m string) string {
	return `{
	.@mode = ` + m + `;
	if (.@mode == 1) {
		dispbottom "one";
	L_In:
		dispbottom "in";
		.@mode = 9;
	} else if (.@mode == 2) {
		dispbottom "two";
		goto L_In;
	}
	dispbottom "after" + .@mode;
	end;
}`
}

// TestRunNames runs converted scripts that use names, assignments as
// values, computed names, sscanf and jumps into blocks, and checks each
// value against rAthena's.
func TestRunNames(t *testing.T) {
	got := namesRun(t, map[string]string{
		"Assign#t": `{
	$@3rdgc_room01 = 4;
	$2012_x$ = "ab";
	dispbottom "" + $@3rdgc_room01 + $2012_x$;
	.@a = 1;
	.@b = (.@a = 5) + 1;
	dispbottom "" + .@a + .@b;
	.@c = 7;
	.@d = .@c++;
	.@e = ++.@c;
	dispbottom "" + .@c + .@d + .@e;
	if ((.@f = 3) > 2)
		dispbottom "f" + .@f;
	.@g = .@h = 2;
	dispbottom "" + .@g + .@h;
	.@s$ = "x";
	.@t$ = (.@s$ += "y") + "z";
	dispbottom .@t$ + .@s$;
	.@i = 0;
	.@arr[.@i++] = 10;
	.@arr[.@i++] = 20;
	dispbottom "" + .@arr[0] + .@arr[1] + .@i;
	end;
}`,
		"Dyn#t": `{
	setd ".@v" + 1, 3;
	dispbottom "" + getd(".@v" + 1) + .@v1;
	.@n$ = "$dn_k";
	setd .@n$, 8;
	dispbottom "" + getd("$dn_k") + getd(.@n$);
	.@m$ = ".@v1";
	dispbottom "" + getd(.@m$);
	.z = 4;
	setd "." + "z", 5;
	dispbottom "" + .z + getd(".z");
	setd ".@w" + 2 + "[3]", 6;
	dispbottom "" + getd(".@w2[3]") + getd(".@nothing" + 1);
	setd "$3rd_" + 7, 9;
	dispbottom "" + getd("$3rd_7");
	end;
}`,
		"Scan#t": `{
	.@r = sscanf("boss12x", "boss%d", .@num);
	dispbottom "" + .@r + " " + .@num;
	.@r = sscanf("abc 5", "%s %d", .@s$, .@k);
	dispbottom "" + .@r + .@s$ + .@k;
	.@r = sscanf("abc x", "%s %d", .@s2$, .@k2);
	dispbottom "" + .@r + .@s2$ + .@k2;
	sscanf("7", "%d", .@q);
	dispbottom "" + .@q;
	dispbottom "" + sscanf("", "%d", .@z) + sscanf("x", "y%d", .@z) + sscanf("5 %", "%d %%", .@z);
	end;
}`,
		"NoFrame#t": `{
	dispbottom "a";
	dispbottom "" + getarg(0);
	dispbottom "b";
	end;
}`,
		"Label#1": labelScript("1"),
		"Label#2": labelScript("2"),
		"Label#3": labelScript("3"),
		"Switch#t": `{
	.@mode = 3;
	if (.@mode == 3) goto L_C;
	switch (.@mode) {
	case 1:
		dispbottom "c1";
	L_C:
		dispbottom "c1b";
		break;
	case 2:
		dispbottom "c2";
	}
	dispbottom "end";
	end;
}`,
	})
	for name, want := range map[string]string{
		"Assign#t":  "4ab|56|979|f3|22|xyzxy|10202",
		"Dyn#t":     "33|88|3|55|60|9",
		"Scan#t":    "1 12|2abc5|-1abc0|7|-1-11",
		"NoFrame#t": "a",
		"Label#1":   "one|in|after9",
		"Label#2":   "two|in|after9",
		"Label#3":   "after3",
		"Switch#t":  "c1b|end",
	} {
		if got[name] != want {
			t.Errorf("%s: got %q, want %q", name, got[name], want)
		}
	}
}
