package zone

import (
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestPreparePotion checks Prepare Potion in both modes: the list holds
// the level 22 potions of Prepare Potion's rows the player has the
// materials for, each with its materials, and a pick uses one product's
// materials but keeps the guide.
func TestPreparePotion(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z := goldenZone(t, mode)
			p := alchemistIn(t, z, 18, 0, map[int]int{skill.IDPharmacy: 10, skill.IDLearningPotion: 10})
			// Medicine Bowls, the Potion Creation Guide, Red Herbs, a White
			// Herb and Empty Potion Bottles.
			for _, it := range [][2]int{{7134, 2}, {7144, 1}, {507, 2}, {509, 1}, {1093, 2}} {
				give(t, z, p, uint32(it[0]), it[1])
			}
			cast(z, p, skill.IDPharmacy, 10, p.entity)
			lists := skillLists(run(z, p, 50, 500))
			if len(lists) != 1 || lists[0].GetSkillId() != skill.IDPharmacy || lists[0].GetItemLevel() != producePotionLevel {
				t.Fatalf("lists %v", lists)
			}
			if got := listItems(lists[0]); !slices.Equal(got, []uint32{itemRedPotion, itemWhitePotion}) {
				t.Fatalf("list %v", got)
			}
			var mats [][2]uint32
			for _, m := range lists[0].GetItems()[0].GetMaterials() {
				mats = append(mats, [2]uint32{m.GetItemId(), m.GetAmount()})
			}
			if !slices.Equal(mats, [][2]uint32{{7144, 0}, {507, 1}, {1093, 1}}) {
				t.Errorf("Red Potion's materials %v", mats)
			}
			if z.countItem(p, 7134) != 1 {
				t.Errorf("Medicine Bowls %d, want 1", z.countItem(p, 7134))
			}
			pick(z, p, skill.IDPharmacy, 0)
			if z.countItem(p, itemRedPotion) != 1 || z.countItem(p, 507) != 1 || z.countItem(p, 1093) != 1 ||
				z.countItem(p, 7144) != 1 {
				t.Errorf("Red Potions %d, herbs %d, bottles %d, guide %d", z.countItem(p, itemRedPotion),
					z.countItem(p, 507), z.countItem(p, 1093), z.countItem(p, 7144))
			}
			// The pick closed the list.
			pick(z, p, skill.IDPharmacy, 1)
			if z.countItem(p, itemWhitePotion) != 0 {
				t.Error("a second pick made a White Potion")
			}
		})
	}
}

// TestPreparePotionFails checks a failed potion: the materials go, and
// nothing comes.
func TestPreparePotionFails(t *testing.T) {
	z := goldenZone(t, reference.Renewal)
	p := alchemistIn(t, z, 18, 0, map[int]int{skill.IDPharmacy: 1})
	for _, it := range [][2]int{{7134, 1}, {7133, 1}, {itemWhitePotion, 1}, {1092, 1}, {1061, 1}} {
		give(t, z, p, uint32(it[0]), it[1])
	}
	cast(z, p, skill.IDPharmacy, 1, p.entity)
	lists := skillLists(run(z, p, 50, 500))
	if len(lists) != 1 || !slices.Equal(listItems(lists[0]), []uint32{itemWhiteSlimPotion}) {
		t.Fatalf("lists %v", lists)
	}
	z.rand = rng.New(maxSource{})
	pick(z, p, skill.IDPharmacy, 0)
	if z.countItem(p, itemWhiteSlimPotion) != 0 || z.countItem(p, itemWhitePotion) != 0 || z.countItem(p, 7133) != 1 {
		t.Errorf("White Slim Potions %d, White Potions %d, guide %d", z.countItem(p, itemWhiteSlimPotion),
			z.countItem(p, itemWhitePotion), z.countItem(p, 7133))
	}
	if a := answers(p); len(a) == 0 || a[len(a)-1] != "Making White_Slim_Potion failed." {
		t.Errorf("answers %q", a)
	}
}

// TestCreateElementalConverter checks Create Elemental Converter in both
// modes: the list holds the converters of item level 23, and a pick
// always makes one.
func TestCreateElementalConverter(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z := goldenZone(t, mode)
			p := goldenPlayerIn(t, z, goldenPlayer{Job: 16, Build: "magic", Sex: 1, JobLevel: 50,
				Skills: [][2]int{{skill.IDCreateCon, 1}}}, 0, 0)
			// A Blank Scroll and the fire converter's material: one Red
			// Blood in renewal, three Scorpion Tails before.
			give(t, z, p, 7433, 1)
			material, amount := uint32(990), 1
			if mode == reference.PreRenewal {
				material, amount = 904, 3
			}
			give(t, z, p, material, amount)
			cast(z, p, skill.IDCreateCon, 1, p.entity)
			lists := skillLists(run(z, p, 50, 500))
			if len(lists) != 1 || lists[0].GetSkillId() != skill.IDCreateCon ||
				!slices.Equal(listItems(lists[0]), []uint32{12114}) {
				t.Fatalf("lists %v", lists)
			}
			pick(z, p, skill.IDCreateCon, 0)
			if z.countItem(p, 12114) != 1 || z.countItem(p, 7433) != 0 || z.countItem(p, material) != 0 {
				t.Errorf("converters %d, scrolls %d, material %d", z.countItem(p, 12114), z.countItem(p, 7433),
					z.countItem(p, material))
			}
		})
	}
}

// TestOpenProduce checks player:open_produce, rAthena's produce: level
// 21 lists the ores the player's smithing skills make, a pick runs the
// skill's formula, and weapon forging's levels are refused.
func TestOpenProduce(t *testing.T) {
	z := goldenZone(t, reference.Renewal)
	p := goldenPlayerIn(t, z, goldenPlayer{Job: 10, Build: "melee", Sex: 1, JobLevel: 50,
		Skills: [][2]int{{skill.IDIron, 5}}}, 0, 0)
	give(t, z, p, itemIronOre, 2)
	h := scriptHost{z}
	if err := h.OpenProduce(int64(p.charID), 1, false); err == nil {
		t.Error("weapon forging opened")
	}
	if err := h.OpenProduce(int64(p.charID), 21, false); err != nil {
		t.Fatal(err)
	}
	lists := skillLists(p.pending)
	if len(lists) != 1 || lists[0].GetSkillId() != 0 || lists[0].GetItemLevel() != 21 || lists[0].GetCooking() ||
		!slices.Equal(listItems(lists[0]), []uint32{itemIron}) {
		t.Fatalf("lists %v", lists)
	}
	pick(z, p, 0, 0)
	if z.countItem(p, itemIron) != 1 || z.countItem(p, itemIronOre) != 1 {
		t.Errorf("Iron %d, ore %d", z.countItem(p, itemIron), z.countItem(p, itemIronOre))
	}
}

// TestOpenCooking checks player:open_cooking, rAthena's cooking: a kit
// of level 11 lists every dish of levels 11 to 20 the player has the
// materials for, and a dish raises COOK_MASTERY on success and lowers it
// on failure.
func TestOpenCooking(t *testing.T) {
	z := goldenZone(t, reference.Renewal)
	p := goldenPlayerIn(t, z, goldenPlayer{Job: 0, Build: "melee", Sex: 1, JobLevel: 10}, 0, 0)
	// produce_db row 52 (level 11): the Cookbook, 5 Grasshopper's Legs,
	// a Hot Sauce and a Cooking Oil, twice.
	for _, it := range [][2]int{{7472, 1}, {940, 10}, {7031, 2}, {7457, 2}} {
		give(t, z, p, uint32(it[0]), it[1])
	}
	h := scriptHost{z}
	if err := h.OpenProduce(int64(p.charID), 11, true); err != nil {
		t.Fatal(err)
	}
	lists := skillLists(p.pending)
	if len(lists) != 1 || !lists[0].GetCooking() || !slices.Equal(listItems(lists[0]), []uint32{12041}) {
		t.Fatalf("lists %v", lists)
	}
	pick(z, p, 0, 0)
	if z.countItem(p, 12041) != 1 || z.countItem(p, 940) != 5 || z.countItem(p, 7472) != 1 || z.cookMastery(p) != 5 {
		t.Errorf("dishes %d, legs %d, cookbook %d, mastery %d", z.countItem(p, 12041), z.countItem(p, 940),
			z.countItem(p, 7472), z.cookMastery(p))
	}
	if err := h.OpenProduce(int64(p.charID), 11, true); err != nil {
		t.Fatal(err)
	}
	z.rand = rng.New(maxSource{})
	pick(z, p, 0, 0)
	if z.countItem(p, 12041) != 1 || z.countItem(p, 940) != 0 || z.cookMastery(p) != 4 {
		t.Errorf("after a failure: dishes %d, legs %d, mastery %d", z.countItem(p, 12041), z.countItem(p, 940),
			z.cookMastery(p))
	}
}

const produceScript = `
local events = require "wa.events"
return function()
  events.on("click", function(player, conversation)
    player:open_produce(21)
    player:open_cooking(15)
    local ok = pcall(function() player:open_produce(1) end)
    player:send_message(ok and "forging opened" or "forging refused")
  end)
end
`

// TestOpenProduceFromLua checks player:open_produce and
// player:open_cooking from a script: each sends its list, and weapon
// forging's levels raise an error.
func TestOpenProduceFromLua(t *testing.T) {
	z, _ := presentZone(t, "test", produceScript, NPC{Name: "example.test:Cook", DisplayName: "Cook", X: 20, Y: 20, Sprite: 112})
	z.Skills, _ = testSkills(t, reference.Renewal)
	a := joinAt(z, "a", 21, 20)
	events(a)
	clickNPC(t, z, a, npcNamed(z, "Cook"))
	z.stepAt(z.now + 50)
	ev := events(a)
	lists := skillLists(ev)
	if len(lists) != 2 || lists[0].GetItemLevel() != 21 || lists[0].GetCooking() || lists[1].GetItemLevel() != 15 ||
		!lists[1].GetCooking() {
		t.Fatalf("lists %v", lists)
	}
	if got := chatLines(ev); !slices.Contains(got, "forging refused:0::0") {
		t.Errorf("chat %q", got)
	}
}
