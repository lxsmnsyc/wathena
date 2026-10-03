package modkit

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/script"
	"github.com/lxsmnsyc/wathena/server/mod"
)

// TestLuckyClover runs the Lucky Clover sample mod's tests, then checks
// in Go what wa.testing cannot read: with the item_rolls flag on, the
// charms that Clover Porings drop roll their luck through the mod's hook
// and get a random option of the mod's group, and the smith rerolls a
// worn charm.
func TestLuckyClover(t *testing.T) {
	e := env(t)
	l, err := e.Load(context.Background(), "../../mods/examples/lucky-clover", "", mod.Renewal, nil)
	if err != nil {
		t.Fatal(err)
	}
	results, err := e.RunTests(context.Background(), l, "", func(CaseResult) {})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if r.Failed() {
			t.Errorf("%s: %v %v", r.Name, r.Err, r.Logs)
		}
	}

	l.Zone.ItemRolls = true
	logs := &capture{}
	w, err := e.newWorld(l, slog.New(logs))
	if err != nil {
		t.Fatal(err)
	}
	defer w.h.Close()
	vals, err := w.Do("new_player", []script.Value{script.NewTable(script.KV("map", script.String("prontera")),
		script.KV("x", script.Int(156)), script.KV("y", script.Int(191)), script.KV("zeny", script.Int(100000)))})
	if err != nil {
		t.Fatal(err)
	}
	player, _ := vals[0].AsInt()
	charm := l.Items.IDs["example.lucky-clover:Clover_Charm"]
	poring := l.Zone.MonsterIDs["EXAMPLE.LUCKY-CLOVER:CLOVER_PORING"]
	for range 40 {
		m, err := w.Do("spawn_monster", []script.Value{script.NewTable(script.KV("monster", script.Int(poring)),
			script.KV("x", script.Int(157)), script.KV("y", script.Int(191)))})
		if err != nil {
			t.Fatal(err)
		}
		id, _ := m[0].AsInt()
		if err := w.h.Kill(player, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.h.TakeFloorItems(player); err != nil {
		t.Fatal(err)
	}
	copies, err := w.h.Copies(player, charm)
	if err != nil {
		t.Fatal(err)
	}
	if len(copies) != 40 {
		t.Fatalf("%d charms", len(copies))
	}
	lucks := map[int]int{}
	equipSlot := -1
	for slot, s := range copies {
		luck, _ := s.Rolls.Get("luck")
		lucks[luck]++
		if luck < 2 || luck > 5 {
			t.Errorf("slot %d: luck %d, want 2 to 5 from the hook", slot, luck)
		}
		// The fortune roll is hidden: dropped charms are unidentified.
		if !s.Unidentified {
			t.Errorf("slot %d: an identified charm", slot)
		}
		if len(s.Options) != 1 || s.Options[0].Name != "max_hp" && s.Options[0].Name != "max_sp" {
			t.Errorf("slot %d: options %v", slot, s.Options)
		}
		equipSlot = slot
	}
	if len(lucks) < 3 {
		t.Errorf("lucks %v: the drops did not roll", lucks)
	}
	if err := w.h.EquipItem(player, equipSlot); err != nil {
		t.Fatal(err)
	}
	said, err := w.h.Talk(player, "Clover Smith#prontera", l.Mod.ID(), []script.Value{script.Int(1)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(said, "Now it is") {
		t.Errorf("the smith said %q; logs %v", said, logs.about("lucky-clover"))
	}
	if lines := logs.about("lucky-clover"); len(lines) > 0 {
		t.Errorf("logs: %v", lines)
	}
}
