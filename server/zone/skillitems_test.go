package zone

import (
	"fmt"
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
)

// skillLists are the SkillItemLists among events.
func skillLists(ev []*wathenav1.Event) []*wathenav1.SkillItemList {
	var out []*wathenav1.SkillItemList
	for _, e := range ev {
		if l := e.GetSkillItemList(); l != nil {
			out = append(out, l)
		}
	}
	return out
}

// listItems are the item IDs of a SkillItemList.
func listItems(l *wathenav1.SkillItemList) []uint32 {
	var out []uint32
	for _, it := range l.GetItems() {
		out = append(out, it.GetItemId())
	}
	return out
}

// withRefine gives a golden zone its own copy of the job tables with the
// mode's refine.yml, which the shared golden tables lack.
func withRefine(t *testing.T, z *Zone, mode reference.Mode) {
	t.Helper()
	var rd status.RefineData
	if err := reference.LoadFile("extracts/"+string(mode)+"/refine_db.json", &rd); err != nil {
		t.Fatal(err)
	}
	tb := *z.Status
	if err := tb.SetRefine(&rd); err != nil {
		t.Fatal(err)
	}
	z.Status = &tb
}

// pick answers the open item list.
func pick(z *Zone, p *Player, id, index int) {
	z.handle(command{player: p, skillPick: &skillPickCommand{skill: uint32(id), index: uint32(index)}}) // #nosec G115 -- test values.
}

// TestArrowCrafting checks Arrow Crafting in both modes: the list holds
// the create_arrow_db.yml sources the player holds, and a pick uses one
// source for each of its products.
func TestArrowCrafting(t *testing.T) {
	for _, mode := range reference.Modes {
		t.Run(string(mode), func(t *testing.T) {
			z := goldenZone(t, mode)
			p := goldenPlayerIn(t, z, goldenPlayer{Job: 3, Build: "ranged", Sex: 1, JobLevel: 50,
				Skills: [][2]int{{skill.IDMakingArrow, 1}}}, 0, 0)
			// Red Gemstone, Empty Bottle, and an Apple, which makes no arrows.
			give(t, z, p, 716, 2)
			give(t, z, p, 713, 3)
			give(t, z, p, 512, 1)
			sp := p.sp
			cast(z, p, skill.IDMakingArrow, 1, p.entity)
			lists := skillLists(run(z, p, 50, 500))
			if len(lists) != 1 || lists[0].GetSkillId() != skill.IDMakingArrow {
				t.Fatalf("lists %v", lists)
			}
			got := listItems(lists[0])
			idx := map[uint32]int{}
			for i, id := range got {
				idx[id] = i
			}
			if len(got) != 2 || p.sp >= sp {
				t.Fatalf("list %v, SP %d of %d", got, p.sp, sp)
			}
			// Empty_Bottle makes 2 Iron Arrows; Red_Gemstone 1 Curse Arrow,
			// 1 Poison Arrow and 10 Rusty Arrows.
			pick(z, p, skill.IDMakingArrow, idx[713])
			if z.countItem(p, 713) != 2 || z.countItem(p, 1770) != 2 {
				t.Errorf("bottles %d, Iron Arrows %d", z.countItem(p, 713), z.countItem(p, 1770))
			}
			// The pick closed the list.
			pick(z, p, skill.IDMakingArrow, idx[716])
			if z.countItem(p, 716) != 2 {
				t.Errorf("a second pick used a gemstone")
			}
			cast(z, p, skill.IDMakingArrow, 1, p.entity)
			lists = skillLists(run(z, p, 1000, 1500))
			if len(lists) != 1 {
				t.Fatalf("lists %v", lists)
			}
			for i, id := range listItems(lists[0]) {
				idx[id] = i
			}
			pick(z, p, skill.IDMakingArrow, idx[716])
			if z.countItem(p, 716) != 1 || z.countItem(p, 1761) != 1 || z.countItem(p, 1763) != 1 ||
				z.countItem(p, 1762) != 10 {
				t.Errorf("gemstones %d, Curse %d, Poison %d, Rusty %d", z.countItem(p, 716), z.countItem(p, 1761),
					z.countItem(p, 1763), z.countItem(p, 1762))
			}
		})
	}
}

// TestUpgradeWeapon checks Upgrade Weapon: the list leaves out the
// weapon in hand and weapons without their ore, a pick refines, and a
// pick whose chance is below the draw destroys the weapon.
func TestUpgradeWeapon(t *testing.T) {
	z, p, _ := smithZone(t, reference.Renewal, smithAxe, 0)
	withRefine(t, z, reference.Renewal)
	// A Sword (weapon level 1) with Phracon, and a Tsurugi (level 3)
	// without Oridecon.
	sword := give(t, z, p, 1101, 1)
	give(t, z, p, 1119, 1)
	give(t, z, p, itemPhracon, 2)
	cast(z, p, skill.IDWeaponRefine, 10, p.entity)
	lists := skillLists(run(z, p, 50, 500))
	if len(lists) != 1 || fmt.Sprint(listItems(lists[0])) != "[1101]" {
		t.Fatalf("lists %v", lists)
	}
	pick(z, p, skill.IDWeaponRefine, 0)
	if s := p.inventory.Slots[sword]; s.ID != 1101 || s.Refine != 1 || z.countItem(p, itemPhracon) != 1 {
		t.Fatalf("sword %+v, Phracon %d", s, z.countItem(p, itemPhracon))
	}
	// +9 to +10 is 19% in renewal; at job level 1 the chance is 19 - 24,
	// below every draw.
	p.inventory.Slots[sword].Refine = 9
	p.progress.JobLevel = 1
	cast(z, p, skill.IDWeaponRefine, 10, p.entity)
	lists = skillLists(run(z, p, 1000, 1500))
	if len(lists) != 1 || len(lists[0].GetItems()) != 1 || lists[0].GetItems()[0].GetRefine() != 9 {
		t.Fatalf("lists %v", lists)
	}
	pick(z, p, skill.IDWeaponRefine, 0)
	if z.countItem(p, 1101) != 0 || z.countItem(p, itemPhracon) != 0 {
		t.Errorf("swords %d, Phracon %d", z.countItem(p, 1101), z.countItem(p, itemPhracon))
	}
}

// TestUpgradeWeaponLimit checks that the list leaves out weapons at the
// skill's learned level.
func TestUpgradeWeaponLimit(t *testing.T) {
	z, p, _ := smithZone(t, reference.PreRenewal, smithAxe, 0)
	withRefine(t, z, reference.PreRenewal)
	p.char.Skills[skill.IDWeaponRefine] = 3
	sword := give(t, z, p, 1101, 1)
	give(t, z, p, itemPhracon, 1)
	p.inventory.Slots[sword].Refine = 3
	cast(z, p, skill.IDWeaponRefine, 3, p.entity)
	lists := skillLists(run(z, p, 50, 500))
	if len(lists) != 1 || len(lists[0].GetItems()) != 0 {
		t.Fatalf("lists %v", lists)
	}
	// An empty list opens nothing to pick from.
	pick(z, p, skill.IDWeaponRefine, 0)
	if p.inventory.Slots[sword].Refine != 3 || z.countItem(p, itemPhracon) != 1 {
		t.Error("a pick from an empty list refined")
	}
}

// TestWeaponRepair checks Weapon Repair on another player: the list
// holds the target's broken items, a pick uses the caster's ore and
// mends the item, and a target without broken items makes the skill
// fail.
func TestWeaponRepair(t *testing.T) {
	z, p, _ := smithZone(t, reference.Renewal, smithAxe, 0)
	o := otherPlayer(t, z, goldenPlayer{Job: 1, Build: "melee", Sex: 1, JobLevel: 50}, 21, 20)
	run(z, p, 0, 0)
	sp := p.sp
	cast(z, p, skill.IDRepairWeapon, 1, o.entity)
	if f := skillFailures(run(z, p, 50, 5000)); len(f) != 1 || p.sp >= sp {
		t.Fatalf("failures %v, SP %d of %d", f, p.sp, sp)
	}
	sword := give(t, z, o, 1101, 1)
	o.inventory.Slots[sword].Broken = true
	give(t, z, p, itemIronOre, 1)
	cast(z, p, skill.IDRepairWeapon, 1, o.entity)
	lists := skillLists(run(z, p, 5500, 10000))
	if len(lists) != 1 || fmt.Sprint(listItems(lists[0])) != "[1101]" {
		t.Fatalf("lists %v", lists)
	}
	events(o)
	pick(z, p, skill.IDRepairWeapon, 0)
	if o.inventory.Slots[sword].Broken || z.countItem(p, itemIronOre) != 0 {
		t.Errorf("broken %v, ore %d", o.inventory.Slots[sword].Broken, z.countItem(p, itemIronOre))
	}
	run(z, p, 10050, 10050)
	saw := false
	for _, e := range events(o) {
		if e.GetChat().GetText() == RepairDone {
			saw = true
		}
	}
	if !saw {
		t.Error("the target was not told")
	}
	// Without the ore of the item's level the pick fails.
	o.inventory.Slots[sword].Broken = true
	cast(z, p, skill.IDRepairWeapon, 1, o.entity)
	run(z, p, 10500, 15000)
	pick(z, p, skill.IDRepairWeapon, 0)
	if f := skillFailures(run(z, p, 15050, 15050)); len(f) != 1 || !o.inventory.Slots[sword].Broken {
		t.Errorf("failures %v, broken %v", f, o.inventory.Slots[sword].Broken)
	}
}

// TestSkillPickWhileTrading checks that a pick during a trade fails and
// closes the list.
func TestSkillPickWhileTrading(t *testing.T) {
	z, p, _ := smithZone(t, reference.Renewal, smithAxe, 0)
	withRefine(t, z, reference.Renewal)
	give(t, z, p, 1101, 1)
	give(t, z, p, itemPhracon, 1)
	cast(z, p, skill.IDWeaponRefine, 10, p.entity)
	run(z, p, 50, 500)
	p.trade = &trade{}
	pick(z, p, skill.IDWeaponRefine, 0)
	p.trade = nil
	if f := skillFailures(run(z, p, 550, 550)); len(f) != 1 || z.countItem(p, itemPhracon) != 1 || p.skillMenu != nil {
		t.Errorf("failures %v, Phracon %d", f, z.countItem(p, itemPhracon))
	}
}

// refineList is a smith in a renewal golden zone, with a Sword and
// Phracon, whose Upgrade Weapon list is open.
func refineList(t *testing.T) (*Zone, *Player, int) {
	t.Helper()
	z, p, _ := smithZone(t, reference.Renewal, smithAxe, 0)
	withRefine(t, z, reference.Renewal)
	sword := give(t, z, p, 1101, 1)
	give(t, z, p, itemPhracon, 2)
	cast(z, p, skill.IDWeaponRefine, 10, p.entity)
	lists := skillLists(run(z, p, 50, 500))
	// The Axe in hand is left out.
	if len(lists) != 1 || fmt.Sprint(listItems(lists[0])) != "[1101]" || p.skillMenu == nil {
		t.Fatalf("lists %v", lists)
	}
	return z, p, sword
}

// TestSkillPickDead checks that death closes the list, and that a dead
// player's pick does nothing.
func TestSkillPickDead(t *testing.T) {
	z, p, sword := refineList(t)
	z.die(p.entity, nil)
	if p.skillMenu != nil {
		t.Error("death kept the list")
	}
	pick(z, p, skill.IDWeaponRefine, 0)
	if p.inventory.Slots[sword].Refine != 0 || z.countItem(p, itemPhracon) != 2 {
		t.Error("a dead player refined")
	}
}

// TestSkillPickWarp checks that a warp within the map closes the list.
func TestSkillPickWarp(t *testing.T) {
	z, p, sword := refineList(t)
	z.teleport(p, 25, 25)
	pick(z, p, skill.IDWeaponRefine, 0)
	if p.skillMenu != nil || p.inventory.Slots[sword].Refine != 0 {
		t.Error("the list stayed open after a warp")
	}
}

// TestSkillPickBusy checks that a pick while talking to an NPC or with
// an open shop fails, as pc_istrading.
func TestSkillPickBusy(t *testing.T) {
	for name, set := range map[string]func(*Player, bool){
		"talk":    func(p *Player, on bool) { p.talk = map[bool]*talk{true: {}}[on] },
		"vending": func(p *Player, on bool) { p.vend = map[bool]*vendShop{true: {}}[on] },
		"buying":  func(p *Player, on bool) { p.buying = map[bool]*buyStore{true: {}}[on] },
	} {
		t.Run(name, func(t *testing.T) {
			z, p, sword := refineList(t)
			set(p, true)
			pick(z, p, skill.IDWeaponRefine, 0)
			set(p, false)
			if f := skillFailures(run(z, p, 550, 550)); len(f) != 1 || p.inventory.Slots[sword].Refine != 0 || p.skillMenu != nil {
				t.Errorf("failures %v, refine %d", f, p.inventory.Slots[sword].Refine)
			}
		})
	}
}

// TestSkillPickChangedSlot checks that a pick of a slot that holds
// another copy than the listed one does nothing.
func TestSkillPickChangedSlot(t *testing.T) {
	z, p, sword := refineList(t)
	z.deleteItem(p, sword, 1, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED)
	if s := give(t, z, p, 1104, 1); s != sword {
		t.Fatalf("the Falchion went to slot %d, not %d", s, sword)
	}
	pick(z, p, skill.IDWeaponRefine, 0)
	if p.inventory.Slots[sword].Refine != 0 || z.countItem(p, itemPhracon) != 2 {
		t.Error("the pick refined another copy")
	}
}

// TestSkillPickMisses checks a cancel, an index past the list and a pick
// naming another skill: none refines. Cancel and the index close the
// list; another skill leaves it open.
func TestSkillPickMisses(t *testing.T) {
	z, p, sword := refineList(t)
	pick(z, p, skill.IDMakingArrow, 0)
	if p.skillMenu == nil {
		t.Fatal("a pick of another skill closed the list")
	}
	pick(z, p, skill.IDWeaponRefine, 5)
	if p.skillMenu != nil {
		t.Fatal("an index past the list kept it open")
	}
	z, p, sword = refineList(t)
	z.handle(command{player: p, skillPick: &skillPickCommand{skill: skill.IDWeaponRefine, cancel: true}})
	if p.skillMenu != nil {
		t.Fatal("cancel kept the list open")
	}
	if p.inventory.Slots[sword].Refine != 0 || z.countItem(p, itemPhracon) != 2 {
		t.Error("a missed pick refined")
	}
}

// TestUpgradeWeaponNotRefineable checks that a weapon refine.yml cannot
// refine is listed, as clif_item_refine_list does not check, and that
// its pick fails and keeps the ore.
func TestUpgradeWeaponNotRefineable(t *testing.T) {
	z, p, _ := smithZone(t, reference.Renewal, smithAxe, 0)
	withRefine(t, z, reference.Renewal)
	give(t, z, p, 1177, 1)
	give(t, z, p, itemPhracon, 1)
	cast(z, p, skill.IDWeaponRefine, 10, p.entity)
	if lists := skillLists(run(z, p, 50, 500)); len(lists) != 1 || fmt.Sprint(listItems(lists[0])) != "[1177]" {
		t.Fatalf("lists %v", lists)
	}
	pick(z, p, skill.IDWeaponRefine, 0)
	if f := skillFailures(run(z, p, 550, 550)); len(f) != 1 || z.countItem(p, itemPhracon) != 1 {
		t.Errorf("failures %v, Phracon %d", f, z.countItem(p, itemPhracon))
	}
}
