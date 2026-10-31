package modscript

import (
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/server/castle"
)

func castleRegistry() *castle.Registry {
	return castle.New([]*castle.Record{
		{ID: 15, Name: "Kriemhild", DisplayName: "Kriemhild", Map: "prtg_cas01", Edition: "fe", NPC: "Agit#prtg_cas01",
			Teleport: &castle.Teleport{X: 107, Y: 180, Cost: 100, SiegeCost: 100000}},
		{ID: 10000, Name: "example.a:keep", DisplayName: "keep", Map: "keep_cas", Edition: "se", NPC: "example.a:steward"},
	}, map[string]castle.State{"Kriemhild": {Owner: 7, Economy: 30, Guardians: 1 << 2}})
}

// TestCastles reads castles and changes them from a mod with
// castles.manage: its own castle, and core's with npcs.control.
func TestCastles(t *testing.T) {
	reg := castleRegistry()
	src := `local castles = require "wa.castles"
return function()
	local k = castles.get("prtg_cas01")
	assert(k ~= nil and k == castles.get("Kriemhild"), "get by map and by name")
	assert(k.id == 15 and k.display_name == "Kriemhild" and k.edition == "fe" and k.npc == "Agit#prtg_cas01")
	assert(k.owner_id == 7 and k.economy == 30 and k.kafra == false)
	assert(k.teleport.x == 107 and k.teleport.siege_cost == 100000)
	assert(k:get_guardian(2).installed and not k:get_guardian(3).installed)
	local keep = castles.get("keep")
	assert(keep ~= nil and keep.name == "example.a:keep" and keep.teleport == nil and keep.owner == nil)
	assert(#castles.list() == 2 and #castles.list { edition = "se" } == 1 and #castles.list { owner = false } == 1)
	keep:set { economy = 5, kafra = true }
	keep:set_guardian(7, true)
	assert(keep.economy == 5 and keep.kafra and keep:get_guardian(7).installed)
	assert(k:set_owner(nil) and not k:set_owner(nil))
	assert(not pcall(keep.set, keep, { economy = -1 }), "negative")
	assert(not pcall(keep.set, keep, { owner = 1 }), "unknown value")
	assert(not pcall(castles.list, { edition = "woe" }), "edition")
end`
	r, logs := newRuntimeOpts(t, newHost(), Options{Castles: reg},
		modScripts("example.a", []string{"castles.manage", "npcs.control"}, nil, "scripts/main.lua", src))
	if r.Context("example.a") == nil {
		t.Fatalf("logs: %s", logs)
	}
	if s, _ := reg.State("example.a:keep"); s.Economy != 5 || !s.Kafra || !s.Installed(7) {
		t.Errorf("keep %+v", s)
	}
	if s, _ := reg.State("Kriemhild"); s.Owner != 0 {
		t.Errorf("Kriemhild %+v", s)
	}
}

// TestCastlePermissions checks castles.manage, and npcs.control on
// another mod's castle.
func TestCastlePermissions(t *testing.T) {
	for _, c := range []struct {
		grants []string
		src    string
		want   string
	}{
		{nil, `castles.get("keep"):set { economy = 1 }`, "needs the permission castles.manage"},
		{[]string{"castles.manage"}, `castles.get("keep"):set { economy = 1 }`, ""},
		{[]string{"castles.manage"}, `castles.get("Kriemhild"):set_guardian(0, true)`, "needs the permission npcs.control"},
	} {
		src := "local castles = require \"wa.castles\"\nreturn function()\n" + c.src + "\nend"
		r, logs := newRuntimeOpts(t, newHost(), Options{Castles: castleRegistry()},
			modScripts("example.a", c.grants, nil, "scripts/main.lua", src))
		if c.want == "" {
			if r.Context("example.a") == nil {
				t.Errorf("%s: logs %s", c.src, logs)
			}
			continue
		}
		if r.Context("example.a") != nil || !strings.Contains(logs.String(), c.want) {
			t.Errorf("%s: logs %s, want %q", c.src, logs, c.want)
		}
	}
}

// TestCastleChange runs castle_change with a castle handle.
func TestCastleChange(t *testing.T) {
	reg := castleRegistry()
	src := `local events = require "wa.events"
local castles = require "wa.castles"
return function()
	events.on("castle_change", function(change)
		assert(change.castle == castles.get("Kriemhild") and change.previous_owner_id == 7)
		seen = (seen or 0) + 1
		castles.get("keep"):set { economy = seen }
	end)
end`
	r, logs := newRuntimeOpts(t, newHost(), Options{Castles: reg},
		modScripts("example.a", []string{"castles.manage"}, nil, "scripts/main.lua", src))
	if r.Context("example.a") == nil {
		t.Fatalf("logs: %s", logs)
	}
	r.Fire("castle_change", CastleChangeArg(CastleChange{Castle: 15, Previous: 7}))
	if s, _ := reg.State("example.a:keep"); s.Economy != 1 {
		t.Errorf("the handler did not run: %+v, logs %s", s, logs)
	}
}
