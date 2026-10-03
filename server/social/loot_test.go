package social

import (
	"fmt"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/progression"
)

// lootCases is the loot part of party_share.json (rAthena fork commit
// 50df331ca): pc_takeitem and party_share_loot for pickers in and out
// of parties, with the random draws rAthena made.
type lootCases struct {
	LootCases []struct {
		Name      string `json:"name"`
		PartyItem int    `json:"party_item"`
		ShareType int    `json:"share_type"`
		Players   []struct {
			Party         int    `json:"party"`
			Slot          int    `json:"slot"`
			Map           string `json:"map"`
			Dead          bool   `json:"dead"`
			InventoryFull bool   `json:"inventory_full"`
		} `json:"players"`
		Picker      int   `json:"picker"`
		First       int   `json:"first"`
		Second      int   `json:"second"`
		Third       int   `json:"third"`
		FirstTick   int64 `json:"first_tick"`
		SecondTick  int64 `json:"second_tick"`
		ThirdTick   int64 `json:"third_tick"`
		At          int64 `json:"at"`
		ItemCBefore int   `json:"itemc_before"`
		ItemCAfter  int   `json:"itemc_after"`
		Taken       bool  `json:"taken"`
		Cleared     bool  `json:"floor_cleared"`
		Receiver    int   `json:"receiver"`
		Drawn       []int `json:"drawn"`
	} `json:"loot_cases"`
}

// TestLootShareMatchesReference replays every recorded pick-up through
// CanTakeParty, SharesLoot, RoundRobin and RandomPick, in both modes.
func TestLootShareMatchesReference(t *testing.T) {
	for _, mode := range []reference.Mode{reference.Renewal, reference.PreRenewal} {
		var ref lootCases
		if err := reference.Load(mode, "party_share", &ref); err != nil {
			t.Fatal(err)
		}
		passed := 0
		for _, c := range ref.LootCases {
			id := func(i int) uint32 {
				if i < 0 {
					return 0
				}
				return uint32(i + 1) // #nosec G115 -- small indexes.
			}
			r := progression.Rights{First: id(c.First), Second: id(c.Second), Third: id(c.Third),
				FirstTick: c.FirstTick, SecondTick: c.SecondTick, ThirdTick: c.ThirdTick}
			pk := c.Players[c.Picker]
			looters := []int{c.First, c.Second, c.Third}
			sameParty := func(i int) bool {
				l := looters[i]
				return l >= 0 && pk.Party != 0 && c.Players[l].Party == pk.Party
			}
			shared := pk.Party != 0 && c.PartyItem&1 != 0
			taken := r.CanTakeParty(id(c.Picker), c.At, shared, sameParty)
			receiver, itemc := -1, c.ItemCBefore
			if taken && pk.Party != 0 && SharesLoot(Options{ItemEven: c.PartyItem&2 != 0}, c.ShareType, c.First >= 0) {
				bySlot := map[int]int{}
				size := 0
				for i, p := range c.Players {
					if p.Party == pk.Party && p.Slot >= 0 {
						bySlot[p.Slot] = i
						size = max(size, p.Slot+1)
					}
				}
				eligible := func(slot int) bool {
					i, ok := bySlot[slot]
					return ok && c.Players[i].Map == pk.Map && !c.Players[i].Dead
				}
				give := func(slot int) bool { return !c.Players[bySlot[slot]].InventoryFull }
				if c.ShareType&ShareRoundRobin != 0 {
					// rAthena wraps at MAX_PARTY, 12.
					if slot, ok := RoundRobin(c.ItemCBefore, max(size, 12), eligible, give); ok {
						receiver, itemc = bySlot[slot], slot
					}
				} else {
					var candidates []int
					for s := range max(size, 12) {
						if eligible(s) {
							candidates = append(candidates, s)
						}
					}
					draws := c.Drawn
					draw := func(n int) int {
						if len(draws) == 0 {
							t.Errorf("%s %s: more draws than rAthena made", mode, c.Name)
							return 0
						}
						d := draws[0]
						draws = draws[1:]
						return d
					}
					if slot, ok := RandomPick(candidates, draw, give); ok {
						receiver = bySlot[slot]
					}
				}
			}
			if taken && receiver < 0 && !pk.InventoryFull {
				receiver = c.Picker
			}
			got := fmt.Sprintf("taken %v cleared %v receiver %d itemc %d", taken, receiver >= 0, receiver, itemc)
			wantItemc := c.ItemCAfter
			if pk.Party == 0 {
				wantItemc = c.ItemCBefore
			}
			want := fmt.Sprintf("taken %v cleared %v receiver %d itemc %d", c.Taken, c.Cleared, c.Receiver, wantItemc)
			if got != want {
				t.Errorf("%s %s: got %s, want %s", mode, c.Name, got, want)
				continue
			}
			passed++
		}
		t.Logf("%s: %d/%d loot cases match", mode, passed, len(ref.LootCases))
		if passed == 0 {
			t.Errorf("%s: no loot cases", mode)
		}
	}
}
