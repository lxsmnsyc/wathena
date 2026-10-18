package mod

import (
	"context"
	"strings"
	"testing"
)

// A mod with no permissions must not change prices or drops of core
// records (security review high-4).
func TestOverlayCoreEconomyNeedsPermission(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "evil", map[string]string{
		"mod.yaml":          manifest("example.evil"),
		"data/items.yml":    "- name: Jellopy\n  sell: 2000000000\n",
		"data/monsters.yml": "- name: PORING\n  drops: [{item: Old_Blue_Box, rate: 10000}]\n",
	})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	ids, _ := LoadIDs(context.Background(), &MemoryIDs{})
	if _, err := s.Items(context.Background(), baseItems, ids, testNames); err == nil {
		t.Error("items: a mod with no permissions raised a core item's sell price")
	}
	if _, err := s.Monsters(func(n string) bool { return n == "PORING" }); err == nil {
		t.Error("monsters: a mod with no permissions changed a core monster's drops")
	}
}

// With data.core_override approved, the same changes load, and
// Overlay.Overrides lists them for wactl mod install. A mod's own record
// needs no permission.
func TestOverlayCoreOverrideAllowed(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "rich", map[string]string{
		"mod.yaml":          manifest("example.rich", "permissions: [data.core_override]"),
		"data/items.yml":    "- name: Jellopy\n  buy: 50\n  sell: 5\n- name: example.rich:Gem\n  buy: 100\n  sell: 50\n",
		"data/monsters.yml": "- name: PORING\n  base_exp: 9000\n",
	})
	s, err := resolve(t, dir, Config{Approve: map[string][]string{"example.rich": {CoreOverride}}})
	if err != nil {
		t.Fatal(err)
	}
	ids, _ := LoadIDs(context.Background(), &MemoryIDs{})
	items, err := s.Items(context.Background(), baseItems, ids, testNames)
	if err != nil {
		t.Fatal(err)
	}
	if got := items.Overlay.Overrides("example.rich"); len(got) != 1 || got[0] != "items Jellopy: buy, sell" {
		t.Errorf("item overrides = %q, want [items Jellopy: buy, sell]", got)
	}
	mons, err := s.Monsters(func(n string) bool { return n == "PORING" })
	if err != nil {
		t.Fatal(err)
	}
	if got := mons.Overrides("example.rich"); len(got) != 1 || got[0] != "monsters PORING: base_exp" {
		t.Errorf("monster overrides = %q", got)
	}
}

// Quest drops and chat filter actions of core's records need the
// permission too.
func TestOverlayCoreOverrideQuestsAndChat(t *testing.T) {
	dir := t.TempDir()
	writeMod(t, dir, "evil", map[string]string{
		"mod.yaml":             manifest("example.evil"),
		"data/quests.yml":      "- name: \"7104\"\n  drops: [{monster: PORING, item: Old_Blue_Box, rate: 10000}]\n",
		"data/chat_filter.yml": "- {name: gamemaster, action: replace}\n",
	})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	core := func(n string) bool { return n == "7104" || n == "gamemaster" }
	for _, k := range []Kind{QuestKind, ChatFilterKind} {
		if _, err := s.Overlay(k, core); err == nil || !strings.Contains(err.Error(), CoreOverride) {
			t.Errorf("%s: err = %v, want one that names %s", k.Name, err, CoreOverride)
		}
	}
}

// Prices follow rAthena's itemdb rule: buy / 124 is not below sell / 75.
func TestItemPricesCannotMakeZeny(t *testing.T) {
	for _, c := range []struct {
		buy, sell int
		ok        bool
	}{{0, 0, true}, {100, 60, true}, {124, 75, true}, {124, 76, false}, {0, 500, false}, {10, 10, false}} {
		if got := PricesSafe(c.buy, c.sell); got != c.ok {
			t.Errorf("PricesSafe(%d, %d) = %v, want %v", c.buy, c.sell, got, c.ok)
		}
	}
	dir := t.TempDir()
	writeMod(t, dir, "a", map[string]string{
		"mod.yaml":       manifest("example.a"),
		"data/items.yml": "- name: example.a:Coin\n  buy: 10\n  sell: 10\n",
	})
	s, err := resolve(t, dir, Config{})
	if err != nil {
		t.Fatal(err)
	}
	ids, _ := LoadIDs(context.Background(), &MemoryIDs{})
	if _, err := s.Items(context.Background(), baseItems, ids, testNames); err == nil || !strings.Contains(err.Error(), "sell 10 is too high for buy 10") {
		t.Errorf("err = %v, want the price rule", err)
	}
}
