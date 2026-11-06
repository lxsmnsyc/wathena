package zone

import (
	"fmt"
	"strconv"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/skill"
)

// Recipes of produce_db.txt (crafting.json's produce), as rAthena's
// skill_can_produce_mix and skill_produce_mix:
//   - Prepare Potion (SkillPreparePotion) lists the potions of item
//     level 22 the player can make, as clif_skill_produce_mix_list.
//   - Create Elemental Converter (SkillCreateElementalConverter) lists
//     the converters of item level 23, as clif_elementalconverter_list.
//   - player:open_produce and player:open_cooking, rAthena's produce and
//     cooking, list the recipes of an item level, as
//     clif_skill_produce_mix_list with no skill and clif_cooking_list.
//   - Twilight Alchemy and Create Deadly Poison make their products
//     without a list.
//
// A pick of a line runs skill_produce_mix. Left out: forging's star
// crumbs and element stones, which the pick cannot name, and the forged
// mark on the product (CARD0_FORGE or CARD0_CREATE with the maker,
// element and star crumbs), which wAthena's copies do not hold. So
// player:open_produce refuses the weapon levels. Fame (forged weapons,
// potion rankings), Instruction Change's bonus from a homunculus, and
// the achievements of a cooked dish wait for those systems.

// Item levels of produce_db.txt (skill_can_produce_mix's trigger): up to
// 10 for weapons, 11 to 20 for cooking, then one level per window.
const (
	produceWeaponLevels = 10
	produceFoodLevels   = 20
	producePotionLevel  = 22
	produceConvLevel    = 23
)

// Items the produce formulas name, as rAthena's ITEMID_ names.
const (
	itemRedPotion            = 501
	itemYellowPotion         = 503
	itemBluePotion           = 505
	itemRedSlimPotion        = 545
	itemYellowSlimPotion     = 546
	itemOrideconAnvil        = 987
	itemGoldenAnvil          = 988
	itemEmperiumAnvil        = 989
	itemStarCrumb            = 1000
	itemManEaterBottle       = 7137
	itemMiniBottle           = 7138
	itemCoatingBottle        = 7139
	itemHomunculusSupplement = 100371
)

// Effects of produce_mix: clif_misceffect's NOTIFYEFFECT_ for potions,
// and the cooking effects.
const (
	effectPharmacyOK   = 305 // EF_PHARMACY_OK
	effectPharmacyFail = 306 // EF_PHARMACY_FAIL
	effectCookingOK    = 608 // EF_COOKING_OK
	effectCookingFail  = 609 // EF_COOKING_FAIL
)

// The answers to a produce, as the client shows clif_produceeffect.
const (
	ProduceDone   = "%s was made."
	ProduceFailed = "Making %s failed."
)

// varCookMastery is rAthena's COOK_MASTERY (SP_COOKMASTERY), 0 to 1999.
var varCookMastery = character.Variable{Mod: "core", Name: "COOK_MASTERY"}

// produceMenu is the menu skill and value that skill_produce_mix reads:
// rAthena's menuskill_id and menuskill_val. skill is 0 for none.
type produceMenu struct {
	skill, val int
}

// recipes are the mode's produce_db.txt rows, in ID order.
func (z *Zone) recipes() []skill.Recipe {
	if z.Skills == nil {
		return nil
	}
	if c := z.Skills.Crafting(); c != nil {
		return c.Produce
	}
	return nil
}

// menuSkill is the skill of the player's open list, or 0.
func (p *Player) menuSkill() int {
	if p.skillMenu == nil {
		return 0
	}
	return p.skillMenu.produce.skill
}

// canProduceMix is skill_can_produce_mix: the index of the recipe for
// qty of a product, or -1. The first row of the product whose skill the
// player has counts; with a menu skill, a row of another skill does not.
// The player needs room for the products, a row of the trigger's item
// level (-1 for any), and the materials.
func (z *Zone) canProduceMix(p *Player, product uint32, trigger, qty, menuSkill int) int {
	if z.Items == nil || p.inventory == nil {
		return -1
	}
	if _, ok := z.Items.Lookup(product); !ok {
		return -1
	}
	recipes := z.recipes()
	idx := -1
	for i := range recipes {
		r := &recipes[i]
		if r.Item != product {
			continue
		}
		if r.Skill > 0 && p.skillLevel(r.Skill) < r.SkillLevel {
			continue
		}
		if r.Skill > 0 && menuSkill > 0 && menuSkill != r.Skill {
			continue
		}
		idx = i
		break
	}
	if product == itemHomunculusSupplement && p.skillLevel(skill.IDBioethics) == 0 {
		return -1
	}
	if idx < 0 || z.overAmount(p, product, qty) {
		return -1
	}
	r := &recipes[idx]
	switch {
	case trigger < 0:
	case trigger > produceFoodLevels:
		if r.ItemLevel != trigger {
			return -1
		}
	case trigger > produceWeaponLevels:
		if r.ItemLevel <= produceWeaponLevels || r.ItemLevel > produceFoodLevels {
			return -1
		}
	default:
		if r.ItemLevel > trigger {
			return -1
		}
	}
	for _, m := range r.Materials {
		if m.Item == 0 {
			continue
		}
		have := z.countItem(p, m.Item)
		if m.Amount == 0 && have == 0 || have < qty*m.Amount {
			return -1
		}
	}
	return idx
}

// overAmount is pc_checkadditem's CHKADDITEM_OVERAMOUNT: qty of an item
// would not fit in its stack.
func (z *Zone) overAmount(p *Player, id uint32, qty int) bool {
	if qty > item.MaxAmount {
		return true
	}
	d := z.Items.Get(id)
	if !d.Stackable {
		return false
	}
	if d.StackLimit > 0 && qty > d.StackLimit {
		return true
	}
	for i, s := range p.inventory.Slots {
		if s.ID != id {
			continue
		}
		if qty > item.MaxAmount-s.Amount || d.StackLimit > 0 && qty > d.StackLimit-s.Amount {
			return true
		}
		return i >= p.inventory.Size
	}
	return false
}

// produceMix is skill_produce_mix: the materials of qty products are
// used, then each product succeeds at make_per in 10,000. skillID 0 is
// the recipe's skill. idx is the recipe, or -1 to look it up with any
// item level. menu is the open list's, which picks the cooking formula.
// It reports whether any product was made. A full inventory loses the
// products (skill_drop_items_full is off).
func (z *Zone) produceMix(p *Player, skillID int, product uint32, qty, idx int, menu produceMenu) bool {
	recipes := z.recipes()
	if idx < 0 {
		if idx = z.canProduceMix(p, product, -1, qty, menu.skill); idx < 0 {
			return false
		}
	}
	if idx >= len(recipes) {
		return false
	}
	r := &recipes[idx]
	qty = max(qty, 1)
	if skillID == 0 {
		skillID = r.Skill
	}
	num := -1 // the materials, less the guide
	for _, m := range r.Materials {
		if _, ok := z.Items.Lookup(m.Item); !ok {
			continue
		}
		num++
		if z.countItem(p, m.Item) == 0 {
			return false
		}
		z.takeItems(p, m.Item, qty*m.Amount)
	}
	st := p.entity.status
	def, ok := z.Items.Lookup(product)
	if st == nil || !ok {
		return false
	}
	jobLv := z.unit(p.entity).JobLevel()
	baby := false
	if z.Status != nil {
		if m, ok := z.Status.MapID(p.char.Job); ok && m&jcBaby != 0 {
			baby = true
		}
	}
	equip := def.IsEquipment()
	wlv := 0
	if equip && def.IsWeapon() {
		wlv = def.WeaponLevel
	}
	cooking := r.ItemLevel > produceWeaponLevels && r.ItemLevel <= produceFoodLevels
	var makePer int
	switch {
	case equip:
		// Weapon forging, without star crumbs or element stones. potion
		// and weapon_produce_rate are 100%.
		makePer = jobLv*20 + st.Dex*10 + st.Luk*10 + z.rand.IntN(100)*10 + 10
		if wlv > 0 && wlv < 4 {
			makePer += 4 / wlv * 1000
		}
		makePer += p.skillLevel(skillID)*500 + p.skillLevel(skill.IDWeaponResearch)*100
		switch {
		case firstSlot(p, itemEmperiumAnvil) >= 0:
			makePer += 1000
		case firstSlot(p, itemGoldenAnvil) >= 0:
			makePer += 500
		case firstSlot(p, itemOrideconAnvil) >= 0:
			makePer += 250
		}
		// A plain Anvil adds nothing.
		if baby {
			makePer = makePer * 70 / 100
		}
	case skillID == skill.IDIron || skillID == skill.IDSteel || skillID == skill.IDEnchantedStone:
		lv := p.skillLevel(skillID)
		makePer = jobLv*20 + st.Dex*10 + st.Luk*10 + z.rand.IntN(100)*10 + 10
		switch product {
		case itemIron:
			makePer += 4000 + lv*500
		case itemSteel:
			makePer += 3000 + lv*500
		case itemStarCrumb:
			makePer = 100000
		default:
			makePer += 1000 + lv*500
		}
	case skillID == skill.IDCreateDeadly:
		makePer = 2000 + 40*st.Dex + 20*st.Luk
	case skillID == skill.IDHolyWater || skillID == skill.IDCreateCon:
		makePer = 100000
	case skillID == skill.IDPharmacy || skillID == skill.IDTwilight1 || skillID == skill.IDTwilight2 ||
		skillID == skill.IDTwilight3:
		makePer = p.skillLevel(skill.IDLearningPotion)*50 + p.skillLevel(skill.IDPharmacy)*300 + jobLv*20 +
			st.Int/2*10 + st.Dex*10 + st.Luk*10
		switch product {
		case itemRedPotion, itemYellowPotion, itemWhitePotion:
			makePer += (1+z.rand.IntN(100))*10 + 2000
		case itemAlcohol:
			makePer += (1+z.rand.IntN(100))*10 + 1000
		case itemFireBottle, itemAcidBottle, itemManEaterBottle, itemMiniBottle:
			makePer += (1 + z.rand.IntN(100)) * 10
		case itemYellowSlimPotion:
			makePer -= (1 + z.rand.IntN(50)) * 10
		case itemWhiteSlimPotion, itemCoatingBottle:
			makePer -= (1 + z.rand.IntN(100)) * 10
		}
		if baby {
			makePer = makePer * 70 / 100
		}
	case menu.skill == skill.IDPharmacy && menu.val > produceWeaponLevels && menu.val <= produceFoodLevels:
		// A cooking kit's dish. The Legendary Cooking Set (level 15 and
		// up) always works.
		if menu.val >= 15 {
			makePer = 10000
			break
		}
		cm := z.cookMastery(p)
		lo := 6 + cm/80
		makePer = 1200*(menu.val-10) + 20*(p.progress.BaseLevel+1) + 20*(st.Dex+1) +
			100*(z.rand.IntN(max(30+5*(cm/400)-lo, 1))+lo) - 400*(r.ItemLevel-11+1) - 10*(100-st.Luk+1) -
			500*(num-1) - 100*(z.rand.IntN(4)+1)
	default:
		makePer = 5000
	}
	makePer = max(makePer, 1)
	name := def.Name
	if name == "" {
		name = def.AegisName
	}
	made := 0
	if qty > 1 || z.rand.IntN(10000) < makePer {
		for range qty {
			if qty == 1 || z.rand.IntN(10000) < makePer {
				made++
			}
		}
	}
	if made > 0 {
		if r := z.addItem(p, product, made, skillOrigin(skillID)); r != item.AddSuccess {
			p.pending = append(p.pending, pickUpFailed(0, pickUpFailure(r)))
		}
		switch {
		case equip, skillID == skill.IDIron || skillID == skill.IDSteel || skillID == skill.IDEnchantedStone:
			z.produceAnswer(p, ProduceDone, name, effectRefineOK)
		case skillID == skill.IDPharmacy || skillID == skill.IDTwilight1 || skillID == skill.IDTwilight2 ||
			skillID == skill.IDTwilight3 || skillID == skill.IDCreateDeadly:
			z.produceAnswer(p, ProduceDone, name, effectPharmacyOK)
		case cooking && skillID == 0:
			z.produceAnswer(p, ProduceDone, name, effectCookingOK)
			z.setCookMastery(p, z.cookMastery(p)+(1<<((r.ItemLevel-11)/2))*5)
		}
		return true
	}
	switch {
	case equip, skillID == skill.IDIron || skillID == skill.IDSteel || skillID == skill.IDEnchantedStone:
		z.produceAnswer(p, ProduceFailed, name, effectRefineFail)
	case skillID == skill.IDCreateDeadly:
		// status_percent_damage of 25% of max HP, which can kill.
		scEnv{z}.Damage(z.unit(p.entity), int64(st.MaxHP)*25/100, 0, true, false)
		z.produceAnswer(p, ProduceFailed, name, effectPharmacyFail)
	case skillID == skill.IDPharmacy || skillID == skill.IDTwilight1 || skillID == skill.IDTwilight2 ||
		skillID == skill.IDTwilight3:
		z.produceAnswer(p, ProduceFailed, name, effectPharmacyFail)
	case cooking && skillID == 0:
		z.produceAnswer(p, ProduceFailed, name, effectCookingFail)
		b := 1 << ((r.ItemLevel - 11) / 2)
		z.setCookMastery(p, z.cookMastery(p)-b-(b>>1)*3)
	}
	return false
}

// produceAnswer shows a produce's effect around the player and tells it
// what was made, as clif_produceeffect and clif_misceffect.
func (z *Zone) produceAnswer(p *Player, format, name string, ef uint32) {
	z.effects = append(z.effects, effect{p.entity, ef})
	z.answer(p, fmt.Sprintf(format, name))
}

// cookMastery is the player's SP_COOKMASTERY.
func (z *Zone) cookMastery(p *Player) int {
	n, _ := strconv.Atoi(p.varMap("character")[varCookMastery])
	return n
}

// setCookMastery is pc_setparam(SP_COOKMASTERY): 0 to 1999, saved with
// the character.
func (z *Zone) setCookMastery(p *Player, v int) {
	v = min(max(v, 0), 1999)
	m := p.varMap("character")
	if v == 0 {
		delete(m, varCookMastery)
		return
	}
	m[varCookMastery] = strconv.Itoa(v)
}

// openRecipes sends the list of the recipes the player can make: those
// of an item level, of the skill's rows only when only is set. menuSkill
// is rAthena's menuskill_id for the pick: the skill, Prepare Potion for
// a cooking kit, or 0 for a script's produce.
func (z *Zone) openRecipes(p *Player, listSkill, menuSkill, trigger int, only, cooking bool) {
	m := &skillMenu{skill: listSkill, produce: produceMenu{skill: menuSkill, val: trigger}, cooking: cooking}
	var lines []*wathenav1.SkillListItem
	recipes := z.recipes()
	for i := range recipes {
		r := &recipes[i]
		idx := z.canProduceMix(p, r.Item, trigger, 1, 0)
		if idx < 0 || only && r.Skill != listSkill {
			continue
		}
		m.items = append(m.items, r.Item)
		lines = append(lines, recipeLine(&recipes[idx]))
	}
	if listSkill == skill.IDCreateCon {
		// clif_elementalconverter_list keeps the count as menuskill_val.
		m.produce.val = len(lines)
	}
	p.pending = append(p.pending, wathenav1.Event_builder{SkillItemList: wathenav1.SkillItemList_builder{
		SkillId: uint32(listSkill), Items: lines, // #nosec G115 -- skill IDs are positive.
		ItemLevel: uint32(max(trigger, 0)), Cooking: cooking}.Build()}.Build()) // #nosec G115 -- small.
	if len(lines) > 0 {
		p.skillMenu = m
	}
}

// recipeLine is the list line of a recipe: the product and its
// materials.
func recipeLine(r *skill.Recipe) *wathenav1.SkillListItem {
	var mats []*wathenav1.SkillListMaterial
	for _, m := range r.Materials {
		mats = append(mats, wathenav1.SkillListMaterial_builder{ItemId: m.Item,
			Amount: uint32(max(m.Amount, 0))}.Build()) // #nosec G115 -- small.
	}
	return wathenav1.SkillListItem_builder{ItemId: r.Item, Materials: mats}.Build()
}

// recipePick runs a pick of a recipe list, as clif_parse_ProduceMix,
// clif_parse_Cooking and clif_parse_SelectArrow for Create Elemental
// Converter.
func (z *Zone) recipePick(p *Player, m *skillMenu, i int) {
	if i < 0 || i >= len(m.items) {
		return
	}
	product := m.items[i]
	if m.skill == skill.IDCreateCon {
		z.produceMix(p, skill.IDCreateCon, product, 1, -1, m.produce)
		return
	}
	if idx := z.canProduceMix(p, product, m.produce.val, 1, m.produce.skill); idx >= 0 {
		z.produceMix(p, 0, product, 1, idx, m.produce)
	}
}

// preparePotion is SkillPreparePotion: the list of the potions of item
// level 22 the player can make with Prepare Potion's rows.
type preparePotion struct{}

func (preparePotion) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if p := src.player; p != nil {
		z.openRecipes(p, d.ID, d.ID, producePotionLevel, true, false)
		z.skillEffect(src, target, d, lv, int64(lv), true)
	}
}

// createConverter is SkillCreateElementalConverter: the list of the
// converters of item level 23 the player can make.
type createConverter struct{}

func (createConverter) castendNoDamage(z *Zone, src, target *entity, d *skill.Def, lv int, _ *int) {
	if p := src.player; p != nil {
		z.openRecipes(p, d.ID, d.ID, produceConvLevel, false, false)
		z.skillEffect(src, target, d, lv, int64(lv), true)
	}
}

// OpenProduce is player:open_produce and player:open_cooking, rAthena's
// produce and cooking: the list of an item level's recipes. A cooking
// kit's list is Prepare Potion's menu, as clif_cooking_list. Weapon
// forging's levels are refused: the pick cannot add star crumbs or
// element stones, and copies hold no forged mark.
func (h scriptHost) OpenProduce(player int64, level int, cooking bool) error {
	what := "player:open_produce"
	if cooking {
		what = "player:open_cooking"
	}
	p, err := h.player(player)
	if err != nil {
		return err
	}
	if level <= produceWeaponLevels || level > 1000 {
		return fmt.Errorf("%s: the item level is 11 to 1000; weapon forging (1 to 10) is not ported", what)
	}
	menuSkill := 0
	if cooking {
		menuSkill = skill.IDPharmacy
	}
	h.z.openRecipes(p, 0, menuSkill, level, false, cooking)
	return nil
}
