package main

import (
	"context"
	"log/slog"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/gamedata"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/tools/convert"
)

// sampleApprovals approves what the sample mods request.
var sampleApprovals = map[string][]string{
	"example.halloween":    {"player.give_items", "world.announce", "events.schedule", "rules.override"},
	"example.wolf-pack":    {"rules.override"},
	"example.lucky-clover": {"rules.override", "items.modify_rolls", "player.give_items"},
}

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
	tables, prog, err := loadProgression(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := gamedata.LoadMonsters(reference.Renewal, tables, prog)
	if err != nil {
		t.Fatal(err)
	}
	m, err := loadMods(context.Background(), modOptions{Dir: "../../../mods/examples", Approve: sampleApprovals}, mod.Renewal, reference.Renewal, data, monsters, tables,
		&mod.MemoryIDs{}, core, slog.New(slog.DiscardHandler))
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
	if d := table.Get(619); d.Unsupported != "pets" {
		t.Errorf("Unripe Apple %+v", d)
	}
	if m.zone.Groups == nil || m.zone.Constants == nil || m.zone.MonsterIDs["PORING"] != 1002 {
		t.Error("zone mods lack the groups, constants or monsters")
	}
	// Core's converted NPCs and the mod's NPC share the map.
	var greeter *zone.NPC
	for i, n := range m.npcs["map/prt_fild08"] {
		if n.Name == "example.greeter:Greeter#prt_fild08" {
			greeter = &m.npcs["map/prt_fild08"][i]
		}
	}
	if greeter == nil || greeter.DisplayName != "Greeter" || greeter.Sprite == 0 || greeter.Script != "example.greeter:scripts/npc/greeter.lua" {
		t.Errorf("greeter %+v", greeter)
	}
	// A converted Kafra with her script, and a converted shop. The
	// cookie stand sells Red Potions at their buy price, 10 in renewal.
	var kafra, shop *zone.NPC
	for i, n := range m.npcs["map/prontera"] {
		switch n.Name {
		case "core:kaf_prontera":
			kafra = &m.npcs["map/prontera"][i]
		case "core:Fruit Gardener#prt":
			shop = &m.npcs["map/prontera"][i]
		}
	}
	if kafra == nil || kafra.Script != "core:scripts/npcs/kaf_prontera.lua" || kafra.DisplayName != "Kafra Employee" {
		t.Errorf("kafra %+v", kafra)
	}
	if shop == nil || shop.Shop == nil || len(shop.Shop.Items) != 2 || shop.Shop.Items[0].Price != 15 || !shop.Shop.Discount {
		t.Errorf("fruit shop %+v", shop)
	}
	if s := m.zone.Shops["example.greeter:Cookie Stand"]; s == nil || len(s.Items) != 2 || s.Items[1].Price != 10 {
		t.Errorf("cookie stand %+v", s)
	}
	// core and the seven sample mods.
	if len(m.zone.Scripts) != 8 || m.zone.ItemIDs["Red_Potion"] != 501 {
		t.Errorf("zone mods: %d scripts", len(m.zone.Scripts))
	}
	// The Halloween mod's monsters, with the IDs of the mod range, and
	// its world events.
	poring := m.zone.MonsterIDs["EXAMPLE.HALLOWEEN:PUMPKIN_PORING"]
	king := m.zone.MonsterIDs["EXAMPLE.HALLOWEEN:PUMPKIN_KING"]
	if poring != 100_000 || king != 100_001 {
		t.Errorf("monster IDs %d %d", poring, king)
	}
	if s := m.zone.Monsters[100_000]; s.Name != "Pumpkin Poring" || s.Sprite != 1002 || s.Reward == nil || s.Reward.Level != 12 || len(s.Reward.Drops) != 3 {
		t.Errorf("pumpkin poring %+v %+v", s, s.Reward)
	}
	if s := m.zone.Monsters[100_001]; s.Status.Class != 1 || s.Status.Mode&0x4000000 == 0 || s.Sprite != 1130 {
		t.Errorf("pumpkin king %+v", s.Status)
	}
	if len(m.all.Events) != 2 || m.all.Events[0].Map != "map/prt_fild08" || m.all.Events[0].Script != "example.halloween:scripts/events/pumpkin_king.lua" {
		t.Errorf("events %+v", m.all.Events)
	}
	// The Chain Lightning mod's skill, which Mages learn.
	chain := m.all.Skills.ByName("example.chain-lightning:CHAIN_LIGHTNING")
	if chain == nil || chain.ID != 20_000 || chain.Script != "example.chain-lightning:scripts/skills/chain_lightning.lua" || chain.MaxLevel != 5 {
		t.Errorf("chain lightning %+v", chain)
	}
	if chain != nil {
		if e := m.all.SkillTrees.Job(2).Skill(chain.ID); e == nil || e.MaxLevel != 5 {
			t.Errorf("mage tree entry %+v", e)
		}
	}
	if m.set.Ruleset.Mod != mod.CoreRenewal {
		t.Errorf("ruleset %s", m.set.Ruleset)
	}
}
