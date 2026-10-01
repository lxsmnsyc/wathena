package main

import (
	"context"
	"log/slog"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// TestLoadSampleMod loads mods/examples over the renewal reference data,
// as waserver does at startup.
func TestLoadSampleMod(t *testing.T) {
	data, err := loadItemData(reference.Renewal)
	if err != nil {
		t.Fatal(err)
	}
	m, err := loadMods(context.Background(), modOptions{Dir: "../../../mods/examples"}, mod.Renewal, reference.Renewal, data, &mod.MemoryIDs{}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	cookie := m.items.IDs["example.greeter:Welcome_Cookie"]
	if d := item.New(m.items.Data).Get(cookie); cookie != 2_000_000 || d.Type != "Healing" || d.Heal == nil {
		t.Errorf("cookie %d: %+v", cookie, d)
	}
	npcs := m.npcs["map/prt_fild08"]
	if len(npcs) != 1 || npcs[0].DisplayName != "Greeter" || npcs[0].Sprite == 0 {
		t.Errorf("npcs %+v", npcs)
	}
	if len(m.zone.Scripts) != 1 || m.zone.ItemIDs["Red_Potion"] != 501 {
		t.Errorf("zone mods %+v", m.zone)
	}
	if m.set.Ruleset.Mod != mod.CoreRenewal {
		t.Errorf("ruleset %s", m.set.Ruleset)
	}
}
