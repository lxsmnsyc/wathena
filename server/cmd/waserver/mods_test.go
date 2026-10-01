package main

import (
	"context"
	"log/slog"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/tools/convert"
)

// TestLoadSampleMod loads mods/examples over the renewal reference data,
// as waserver does at startup.
func TestLoadSampleMod(t *testing.T) {
	data, err := loadItemData(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	core, err := convert.LoadCore(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	m, err := loadMods(context.Background(), modOptions{Dir: "../../../mods/examples"}, mod.Renewal, reference.Renewal, data, &mod.MemoryIDs{}, core, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	table := item.New(m.items.Data)
	cookie := m.items.IDs["example.greeter:Welcome_Cookie"]
	if d := table.Get(cookie); cookie != 2_000_000 || d.Type != "Healing" || d.Script != "example.greeter:scripts/items/welcome_cookie.lua" {
		t.Errorf("cookie %d: %+v", cookie, d)
	}
	// Core's converted scripts: a heal, a box, a static card, and an
	// item refused for what it needs.
	if d := table.Get(501); d.Script != "core:scripts/items/Red_Potion.lua" {
		t.Errorf("Red Potion %+v", d)
	}
	if d := table.Get(603); d.Script != "core:scripts/items/Old_Blue_Box.lua" {
		t.Errorf("Old Blue Box %+v", d)
	}
	if d := table.Get(4001); len(d.Bonuses) != 2 || d.Script != "" {
		t.Errorf("Poring Card %+v", d)
	}
	if d := table.Get(506); d.Unsupported != "status effects" {
		t.Errorf("Green Potion %+v", d)
	}
	if m.zone.Groups == nil || m.zone.Constants == nil || m.zone.MonsterIDs["PORING"] != 1002 {
		t.Error("zone mods lack the groups, constants or monsters")
	}
	npcs := m.npcs["map/prt_fild08"]
	if len(npcs) != 1 || npcs[0].DisplayName != "Greeter" || npcs[0].Sprite == 0 {
		t.Errorf("npcs %+v", npcs)
	}
	if len(m.zone.Scripts) != 2 || m.zone.ItemIDs["Red_Potion"] != 501 {
		t.Errorf("zone mods %+v", m.zone)
	}
	if m.set.Ruleset.Mod != mod.CoreRenewal {
		t.Errorf("ruleset %s", m.set.Ruleset)
	}
}
