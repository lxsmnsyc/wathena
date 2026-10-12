package convert

import (
	"strings"
	"testing"
)

// TestRuleReadsConvert checks the commands that read the ruleset (review
// decision 32): basicskillcheck reads its rule with rules.get, and
// checkre reads the flag of its define with rules.is, except checkre(0),
// the base, which the converter's mode fixes.
func TestRuleReadsConvert(t *testing.T) {
	res := eventResult(t)
	for _, c := range []struct {
		script string
		want   []string
	}{
		{`{ if (basicskillcheck()) end; end; }`, []string{`local rules = require "wa.rules"`, `if rules.get("basic_skill_check") then`}},
		{`{ .@n = checkre(0); end; }`, []string{"n = 1"}},
		{`{ if (checkre(1)) end; end; }`, []string{`if rules.is("cast_time") then`}},
		{`{ if (checkre(2)) end; end; }`, []string{`if rules.is("level_penalty_drop") then`}},
		{`{ if (checkre(3)) end; end; }`, []string{`if rules.is("level_penalty_exp") then`}},
		{`{ if (checkre(4)) end; end; }`, []string{`if rules.is("level_damage_mod") then`}},
		{`{ if (checkre(5)) end; end; }`, []string{`if rules.is("aspd") then`}},
		{`{ .@n = checkre(3) ? 10 : 100; end; }`, []string{`rules.is("level_penalty_exp")`}},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		lua, err := n.npcScript()
		if err != nil {
			t.Errorf("%s: %v", c.script, err)
			continue
		}
		for _, w := range c.want {
			if !strings.Contains(lua, w) {
				t.Errorf("%s: got\n%s\nwant %q", c.script, lua, w)
			}
		}
	}
	for _, c := range []struct{ script, want string }{
		{`{ if (checkre(6)) end; end; }`, "checkre: an argument other than 0 to 5"},
		{`{ if (checkre(.@n)) end; end; }`, "checkre: an argument other than 0 to 5"},
		{`{ if (checkre()) end; end; }`, "checkre: arguments"},
	} {
		n := res.conv.newNPCConv(&npcLine{Kind: "script", Name: "X#t", Map: "prontera", Script: c.script})
		_, err := n.npcScript()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.script, err, c.want)
		}
	}
}
