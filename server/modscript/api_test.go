package modscript

import (
	"bytes"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/constants"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/modscript/luaapi"
)

// TestAPIDefinitions checks the runtime against server/modscript/luaapi,
// which the editor definitions and the API reference come from. When it
// fails, change the definitions with the runtime, and regenerate the
// files with go test ./server/modscript/luaapi -update.
func TestAPIDefinitions(t *testing.T) {
	data, err := reference.ReadFile("extracts/renewal/script_constants.json")
	if err != nil {
		t.Fatal(err)
	}
	consts, err := constants.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	r, err := New(Options{Host: newHost(), Mods: []string{"core"}, Constants: consts, Testing: true}, []*mod.Scripts{itemMod()})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	modules, handles, err := r.Describe()
	if err != nil {
		t.Fatal(err)
	}
	tables := consts.Modules()
	specModules := map[string]bool{}
	for _, m := range luaapi.Modules {
		specModules[m.Name] = true
		keys, ok := modules[m.Name]
		if !ok {
			t.Errorf("%s is defined, and the runtime has no such module", m.Name)
			continue
		}
		var want []string
		for _, f := range append(slices.Clone(m.Funcs), m.Methods...) {
			want = append(want, f.Name)
		}
		if m.Constants {
			c := tables[strings.TrimPrefix(m.Name, "wa.")]
			if c == nil {
				t.Errorf("%s is a constant module, and server/constants has none of that name", m.Name)
				continue
			}
			want = append(want, slices.Collect(maps.Keys(c.Values))...)
		}
		slices.Sort(want)
		if !slices.Equal(keys, want) {
			t.Errorf("%s: the runtime has %v, the definitions %v", m.Name, diff(keys, want), diff(want, keys))
		}
	}
	for name := range modules {
		if !specModules[name] {
			t.Errorf("the runtime has %s, which is not defined", name)
		}
	}
	if got, want := constants.ModuleNames(), constantModules(); !sameSet(got, want) {
		t.Errorf("constant modules: server/constants has %v, the definitions %v", got, want)
	}

	byHandle := map[string][]string{}
	for _, c := range luaapi.Classes {
		if c.Handle == "" {
			continue
		}
		byHandle[c.Handle] = append(byHandle[c.Handle], "")
		for _, f := range append(slices.Clone(c.Methods), c.Functions...) {
			byHandle[c.Handle] = append(byHandle[c.Handle], f.Name)
		}
	}
	for name, list := range byHandle {
		list = slices.DeleteFunc(list, func(s string) bool { return s == "" })
		slices.Sort(list)
		list = slices.Compact(list)
		got, ok := handles[name]
		if !ok {
			t.Errorf("handle type %q is defined, and the runtime has none", name)
			continue
		}
		if !slices.Equal(got, list) {
			t.Errorf("handle type %q: the runtime has %v, the definitions %v", name, diff(got, list), diff(list, got))
		}
	}
	for name := range handles {
		if _, ok := byHandle[name]; !ok {
			t.Errorf("the runtime has the handle type %q, which is not defined", name)
		}
	}

	var events []string
	for _, e := range luaapi.Events {
		events = append(events, e.Name)
		if kinds := Events[e.Name]; !slices.Equal(kinds, e.Kinds) {
			t.Errorf("event %s: the runtime's kinds are %v, the definitions' %v", e.Name, kinds, e.Kinds)
		}
	}
	if !sameSet(events, slices.Collect(maps.Keys(Events))) {
		t.Errorf("events: the runtime has %v, the definitions %v", slices.Sorted(maps.Keys(Events)), events)
	}
	if !sameSet(luaapi.EquipSlots, slices.Collect(maps.Keys(equipSlots))) {
		t.Errorf("equipment slots differ: %v", luaapi.EquipSlots)
	}
	for _, a := range luaapi.Aliases() {
		if a.Name == "wa.ContextKind" && !sameSet(a.Values, []string{KindMod, KindNPC, KindItem, KindSkill, KindMonster, KindEvent}) {
			t.Errorf("context kinds differ: %v", a.Values)
		}
	}
}

// TestAPIEventObjects checks the fields of the damage and skill_used
// objects, and of a mod context's instance, against the definitions.
func TestAPIEventObjects(t *testing.T) {
	h := newHost()
	r := newItemRuntime(t, h, "scripts/items/x.lua", `
local events = require "wa.events"
return function()
  local function keys(t)
    local list = {}
    for k in pairs(t) do list[#list + 1] = k end
    table.sort(list)
    return table.concat(list, ",")
  end
  events.on("damage", function(hit) hit.target:send_message(keys(hit)) end)
  events.on("skill_used", function(use) use.target:send_message(keys(use)) end)
end`)
	c, err := r.Equip("core:scripts/items/x.lua", 10, 1, &recorder{r: r})
	if err != nil {
		t.Fatal(err)
	}
	hit := Hit{Source: 2, SourceKind: "player", Target: 1, TargetKind: "player", Kind: "physical", Range: "melee", Skill: 5, Amount: 1}
	if err := r.FireHit(c, hit); err != nil {
		t.Fatal(err)
	}
	if err := r.FireSkillUse(c, SkillUse{Skill: 5, Level: 1, Target: 1}); err != nil {
		t.Fatal(err)
	}
	for i, class := range []string{"wa.Damage", "wa.SkillUse"} {
		if len(h.messages) <= i {
			t.Fatalf("messages %v", h.messages)
		}
		got := strings.Split(strings.TrimPrefix(h.messages[i], "1:"), ",")
		if want := classFields(class); !sameSet(got, want) {
			t.Errorf("%s: the runtime has %v, the definitions %v", class, got, want)
		}
	}

	h2 := newHost()
	r2, logs := newRuntime(t, h2, modScripts("example.a", nil, nil, "scripts/main.lua", `
local events = require "wa.events"
local context = require "wa.context"
return function()
  local keys = {}
  for k in pairs(context:get_instance()) do keys[#keys + 1] = k end
  table.sort(keys)
  local text = table.concat(keys, ",")
  events.on("login", function(player) player:send_message(text) end)
end`))
	r2.Fire("login", Player(1))
	if len(h2.messages) != 1 {
		t.Fatalf("messages %v, logs %s", h2.messages, logs)
	}
	if got, want := strings.Split(strings.TrimPrefix(h2.messages[0], "1:"), ","), classFields("wa.ModInstance"); !sameSet(got, want) {
		t.Errorf("wa.ModInstance: the runtime has %v, the definitions %v", got, want)
	}
}

func classFields(name string) []string {
	for _, c := range luaapi.Classes {
		if c.Name == name {
			var out []string
			for _, f := range c.Fields {
				out = append(out, f.Name)
			}
			return out
		}
	}
	return nil
}

func constantModules() []string {
	var out []string
	for _, m := range luaapi.Modules {
		if m.Constants {
			out = append(out, strings.TrimPrefix(m.Name, "wa."))
		}
	}
	return out
}

func sameSet(a, b []string) bool {
	a, b = slices.Sorted(slices.Values(a)), slices.Sorted(slices.Values(b))
	return slices.Equal(a, b)
}

// diff lists what a has and b lacks.
func diff(a, b []string) []string {
	var out []string
	for _, s := range a {
		if !slices.Contains(b, s) {
			out = append(out, s)
		}
	}
	return out
}
