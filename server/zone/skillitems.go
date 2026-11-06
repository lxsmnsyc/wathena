package zone

import (
	"fmt"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/skill"
	"github.com/lxsmnsyc/wathena/server/status"
)

// The item lists of the skills that ask the player to pick an item, as
// rAthena's menu skills (menuskill_id): the skill's cast sends
// SkillItemList, and SkillItemPick runs the pick.
//   - Arrow Crafting (SkillMakingArrow) lists the items of
//     create_arrow_db.yml the player holds (clif_arrow_create_list), and
//     a pick is skill_arrow_create.
//   - Weapon Repair (SkillWeaponRepair) lists the target's broken items
//     (clif_item_repair_list), and a pick is skill_repairweapon.
//   - Upgrade Weapon (SkillUpgradeWeapon) lists the weapons it can refine
//     (clif_item_refine_list), and a pick is skill_weaponrefine.
//
// rAthena's client sends the item, or its inventory index, back. Here
// the pick names a line of the list, and the server checks it again as
// rAthena does. Left out: the forged weapons' fame at +10
// (pc_addfame), the achievements of a refine, and the hatched pet eggs
// that the lists skip, since fame, achievements and pets do not exist
// yet. produce_item_name_input's bit 4 is off by default, so crafted
// arrows carry no maker.

// skillMenu is the open item list of a menu skill.
type skillMenu struct {
	skill int
	// lv is the cast's level for Weapon Repair's range, and the learned
	// level of Upgrade Weapon, the highest refine it reaches
	// (menuskill_val).
	lv int
	// target is Weapon Repair's target.
	target *entity
	// slots are the inventory slots of the lines, of the target for
	// Weapon Repair, and copies what each slot held when the list was
	// sent. items are Arrow Crafting's source items.
	slots  []int
	copies []listedCopy
	items  []uint32
}

// listedCopy is what a listed slot held: a pick of a slot that holds
// another copy now is refused, so it never acts on an item the player
// did not see.
type listedCopy struct {
	id     uint32
	serial item.Serial
}

// add lists an inventory slot.
func (m *skillMenu) add(slot int, s *item.Slot) {
	m.slots = append(m.slots, slot)
	m.copies = append(m.copies, listedCopy{s.ID, s.Serial})
}

// line returns the slot of line i when inv still holds the listed copy
// there, or -1.
func (m *skillMenu) line(i int, inv *item.Inventory) int {
	if i < 0 || i >= len(m.slots) || inv == nil {
		return -1
	}
	slot := m.slots[i]
	if slot < 0 || slot >= len(inv.Slots) {
		return -1
	}
	if s := &inv.Slots[slot]; s.ID != m.copies[i].id || s.Serial != m.copies[i].serial || s.Amount <= 0 {
		return -1
	}
	return slot
}

// The materials of Weapon Repair and Upgrade Weapon by weapon or armor
// level, as skill_repairweapon and skill_weaponrefine. Level 5 weapons
// and level 2 armors, which exist in renewal only, have none.
var (
	repairWeaponMaterials = []uint32{itemIronOre, itemIron, itemSteel, itemOrideconStone}
	repairArmorMaterials  = []uint32{itemSteel}
	refineMaterials       = []uint32{itemPhracon, itemEmveretarcon, itemOridecon, itemOridecon}
)

// Items the crafting skills use, as rAthena's ITEMID_ names.
const (
	itemOrideconStone = 756
	itemOridecon      = 984
	itemIron          = 998
	itemSteel         = 999
	itemIronOre       = 1002
	itemPhracon       = 1010
	itemEmveretarcon  = 1011
)

// The answers to a refine and a repair, as the client's message table
// shows clif_upgrademessage and clif_item_repaireffect.
const (
	RefineUpgraded   = "%s was upgraded."
	RefineFailed     = "Upgrading %s failed."
	RefineSkillLow   = "Upgrade Weapon's level is too low to upgrade %s further."
	RefineNoMaterial = "You need %s to upgrade the weapon."
	RepairDone       = "The item has been repaired."
	RepairFailed     = "The item could not be repaired."
)

// materialAt returns a material of a table by level from 1, or 0.
func materialAt(table []uint32, level int) uint32 {
	if level < 1 || level > len(table) {
		return 0
	}
	return table[level-1]
}

// firstSlot is pc_search_inventory: the first slot that holds an item,
// or -1.
func firstSlot(p *Player, id uint32) int {
	if id == 0 || p.inventory == nil {
		return -1
	}
	for i, s := range p.inventory.Slots {
		if s.ID == id && s.Amount > 0 {
			return i
		}
	}
	return -1
}

// listLine is a line of a SkillItemList for an inventory slot.
func listLine(s *item.Slot) *wathenav1.SkillListItem {
	return wathenav1.SkillListItem_builder{ItemId: s.ID, Refine: uint32(max(s.Refine, 0)), // #nosec G115 -- refines are small.
		Cards: s.Cards}.Build()
}

// openSkillMenu sends a skill's item list. A list with lines replaces the
// open one; an empty list leaves it.
func (p *Player) openSkillMenu(m *skillMenu, lines []*wathenav1.SkillListItem) {
	p.pending = append(p.pending, wathenav1.Event_builder{SkillItemList: wathenav1.SkillItemList_builder{
		SkillId: uint32(m.skill), Items: lines}.Build()}.Build()) // #nosec G115 -- skill IDs are positive.
	if len(lines) > 0 {
		p.skillMenu = m
	}
}

// makingArrow is SkillMakingArrow: the list of the player's items that
// make arrows, as clif_arrow_create_list. An item that is worn or not
// identified is left out.
type makingArrow struct{}

func (makingArrow) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	p := src.player
	if p == nil {
		return
	}
	m := &skillMenu{skill: d.ID}
	var lines []*wathenav1.SkillListItem
	var arrows []skill.ArrowSource
	if c := z.Skills.Crafting(); c != nil {
		arrows = c.Arrows
	}
	for _, a := range arrows {
		if _, ok := z.Items.Lookup(a.Source); !ok {
			continue
		}
		i := firstSlot(p, a.Source)
		if i < 0 {
			continue
		}
		if s := &p.inventory.Slots[i]; s.Equip != 0 || s.Unidentified {
			continue
		}
		m.items = append(m.items, a.Source)
		lines = append(lines, wathenav1.SkillListItem_builder{ItemId: a.Source}.Build())
	}
	p.openSkillMenu(m, lines)
	z.skillEffect(src, target, d, lv, int64(lv), true)
}

// arrowCreate is skill_arrow_create: one source item goes, and each of
// its products comes. A product that does not fit is lost, as
// skill_drop_items_full is off, and the player sees why.
func (z *Zone) arrowCreate(p *Player, source uint32) {
	a := z.Skills.Crafting().Arrow(source)
	if a == nil || len(a.Make) == 0 {
		return
	}
	if _, ok := z.Items.Lookup(source); !ok {
		return
	}
	i := firstSlot(p, source)
	if i < 0 {
		return
	}
	z.deleteItem(p, i, 1, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED)
	for _, m := range a.Make {
		if _, ok := z.Items.Lookup(m.Item); !ok || m.Amount <= 0 {
			continue
		}
		if r := z.addItem(p, m.Item, m.Amount, skillOrigin(skill.IDMakingArrow)); r != item.AddSuccess {
			p.pending = append(p.pending, pickUpFailed(0, pickUpFailure(r)))
		}
	}
}

// weaponRepair is SkillWeaponRepair: the list of a player target's
// broken items, as clif_item_repair_list. A target without one makes
// the skill fail.
type weaponRepair struct{}

func (weaponRepair) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	p, t := src.player, target.player
	if p == nil || t == nil {
		return
	}
	m := &skillMenu{skill: d.ID, lv: lv, target: target}
	var lines []*wathenav1.SkillListItem
	for _, i := range brokenSlots(t) {
		m.add(i, &t.inventory.Slots[i])
		lines = append(lines, listLine(&t.inventory.Slots[i]))
	}
	if len(lines) == 0 {
		z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	p.openSkillMenu(m, lines)
}

// repairWeapon is skill_repairweapon: the caster pays one ore of the
// item's level and the item is mended. Another player must be in the
// skill's range.
func (z *Zone) repairWeapon(p *Player, m *skillMenu, slot int) {
	target := m.target
	if target == nil || target.player == nil || !z.entities.has(target) {
		return
	}
	t := target.player
	if slot < 0 || slot >= len(t.inventory.Slots) {
		return
	}
	s := &t.inventory.Slots[slot]
	if s.ID == 0 || s.Amount <= 0 || !s.Broken {
		return
	}
	d := z.Skills.Get(m.skill)
	if d == nil {
		return
	}
	if t != p && !z.checkRange(p.entity, target.x, target.y, d.RangeFor(p.entity.status, m.lv)) {
		z.answer(p, RepairFailed)
		return
	}
	var material uint32
	if def, ok := z.Items.Lookup(s.ID); ok {
		switch {
		case def.IsWeapon():
			material = materialAt(repairWeaponMaterials, def.WeaponLevel)
		case def.IsArmor():
			material = materialAt(repairArmorMaterials, def.ArmorLevel)
		}
	}
	ore := firstSlot(p, material)
	if ore < 0 {
		z.skillFail(p.entity, m.skill, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	z.skillEffect(p.entity, target, d, 1, 1, true)
	z.repair(t, slot)
	z.deleteItem(p, ore, 1, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED)
	z.answer(p, RepairDone)
	if t != p {
		z.answer(t, RepairDone)
	}
}

// upgradeWeapon is SkillUpgradeWeapon: the list of the weapons the
// player can refine, as clif_item_refine_list: identified, below the
// skill's learned level, with the ore of their weapon level held, and
// not worn in a hand. rAthena sends the list even when it is empty.
type upgradeWeapon struct{}

func (upgradeWeapon) castendNoDamage(z *Zone, src, _ *entity, d *skill.Def, _ int, _ *int) {
	p := src.player
	if p == nil {
		return
	}
	m := &skillMenu{skill: d.ID, lv: p.skillLevel(d.ID)}
	var lines []*wathenav1.SkillListItem
	for i := range p.inventory.Slots {
		s := &p.inventory.Slots[i]
		if s.ID == 0 || s.Amount <= 0 || s.Refine >= m.lv || s.Unidentified || s.Equip&item.EqpArms != 0 {
			continue
		}
		def, ok := z.Items.Lookup(s.ID)
		if !ok || !def.IsWeapon() || def.WeaponLevel < 1 || firstSlot(p, materialAt(refineMaterials, def.WeaponLevel)) < 0 {
			continue
		}
		m.add(i, s)
		lines = append(lines, listLine(s))
	}
	p.openSkillMenu(m, lines)
}

// weaponRefine is skill_weaponrefine: the ore of the weapon's level
// goes, then a draw below refine.yml's Normal chance in percent, plus
// half the job levels above 50 (10 for third classes), raises the
// refine by 1. Any other draw destroys the weapon.
func (z *Zone) weaponRefine(p *Player, m *skillMenu, slot int) {
	if slot < 0 || slot >= len(p.inventory.Slots) {
		return
	}
	s := &p.inventory.Slots[slot]
	def, ok := z.Items.Lookup(s.ID)
	if s.ID == 0 || s.Amount <= 0 || !ok || !def.IsWeapon() {
		return
	}
	name := def.Name
	if !def.Refineable || def.WeaponLevel < 1 {
		z.skillFail(p.entity, m.skill, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	if s.Refine >= m.lv || s.Refine >= 10 {
		z.answer(p, fmt.Sprintf(RefineSkillLow, name))
		return
	}
	material := materialAt(refineMaterials, def.WeaponLevel)
	ore := firstSlot(p, material)
	if ore < 0 {
		z.answer(p, fmt.Sprintf(RefineNoMaterial, z.Items.Get(material).Name))
		return
	}
	var c status.RefineCost
	found := false
	if z.Status != nil {
		c, found = z.Status.RefineCost(true, def.WeaponLevel, s.Refine)
	}
	if want, ok := z.Items.ByName(c.Material); !found || !ok || want.ID != material {
		// The refine database's ore must be the hardcoded one.
		z.skillFail(p.entity, m.skill, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	per := c.Rate / 100
	if z.Status != nil {
		if mapID, found := z.Status.MapID(p.char.Job); found && mapID&jcThird != 0 {
			per += 10
		} else {
			per += (z.unit(p.entity).JobLevel() - 50) / 2
		}
	}
	z.deleteItem(p, ore, 1, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED)
	s = &p.inventory.Slots[slot]
	if per > z.rand.IntN(100) {
		position := s.Equip
		if position != 0 {
			z.unequipItem(p, slot)
		}
		p.inventory.Slots[slot].Refine++
		p.pending = append(p.pending, wathenav1.Event_builder{InventoryRemove: wathenav1.InventoryRemove_builder{
			Slot: uint32(slot), Amount: 1, // #nosec G115 -- below MaxInventory.
			Reason: wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED,
		}.Build()}.Build())
		z.sendAdd(p, slot, 1)
		z.answer(p, fmt.Sprintf(RefineUpgraded, name))
		if position != 0 {
			z.equipItem(p, slot, position)
		}
		z.effects = append(z.effects, effect{p.entity, effectRefineOK})
		return
	}
	s.Refine = 0
	if s.Equip != 0 {
		z.unequipItem(p, slot)
	}
	z.answer(p, fmt.Sprintf(RefineFailed, name))
	z.deleteItem(p, slot, 1, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED)
	z.effects = append(z.effects, effect{p.entity, effectRefineFail})
	z.emotions = append(z.emotions, npcEmotion{p.entity, emotionHuk})
}

// skillItemPick runs a pick of the open item list, as
// clif_parse_SelectArrow, clif_parse_RepairItem and
// clif_parse_WeaponRefine. The list closes either way. A dead player's
// list is gone (pc_dead clears it). A player who talks to an NPC, vends,
// buys or trades sees the skill fail, as rAthena refuses the pick then.
// A listed slot that holds another copy now does nothing.
func (z *Zone) skillItemPick(p *Player, c *skillPickCommand) {
	m := p.skillMenu
	if m == nil || m.skill != int(c.skill) {
		return
	}
	p.skillMenu = nil
	if c.cancel {
		return
	}
	if p.entity.dead {
		return
	}
	if p.trade != nil || p.talk != nil || p.vend != nil || p.buying != nil {
		// pc_istrading: an NPC talk, a vending shop, a buying store or a
		// trade.
		z.skillFail(p.entity, m.skill, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
		return
	}
	i := int(c.index)
	switch m.skill {
	case skill.IDMakingArrow:
		if i < len(m.items) {
			z.arrowCreate(p, m.items[i])
		}
	case skill.IDRepairWeapon:
		if t := m.target; t != nil && t.player != nil {
			if slot := m.line(i, t.player.inventory); slot >= 0 {
				z.repairWeapon(p, m, slot)
			}
		}
	case skill.IDWeaponRefine:
		if slot := m.line(i, p.inventory); slot >= 0 {
			z.weaponRefine(p, m, slot)
		}
	}
}

// skillPickCommand is a SkillItemPick.
type skillPickCommand struct {
	skill, index uint32
	cancel       bool
}

// SkillItemPick answers the open item list of a skill.
func (s *Session) SkillItemPick(seq, skillID, index uint32, cancel bool) bool {
	return s.send(command{seq: seq, skillPick: &skillPickCommand{skill: skillID, index: index, cancel: cancel}})
}
