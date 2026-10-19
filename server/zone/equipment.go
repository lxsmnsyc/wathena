package zone

import (
	"errors"
	"fmt"
	"slices"

	"github.com/lxsmnsyc/wathena/internal/script"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/modscript"
	"github.com/lxsmnsyc/wathena/server/status"
)

// Equipment: players put on items from their inventory, as rAthena's
// pc_equipitem and pc_unequipitem, and put cards into them, as
// pc_insert_card (server/item). Each equipped slot has a handle for
// scripts, and an equip context for its item's script and one for each
// card's (server/modscript). Stats are computed again from every
// equipped item and every active bonus after each change
// (docs/decisions/0009-stats-as-totals-of-modifiers.md).

type equipCommand struct {
	slot     int
	position uint32
}

type insertCardCommand struct {
	card, equip int
}

// wornSlot is an equipped inventory slot: its handle for scripts, and
// the equip contexts of its item and of each card, by card index. A nil
// script runs nothing.
type wornSlot struct {
	handle int64
	item   *equipScript
	cards  []*equipScript
	// cardHandles are the handles of each card's context. A card's
	// handle reads the item's fields, but its rolls are the card's own.
	cardHandles []int64
}

// equipScript is one equip context and the bonuses its script keeps.
type equipScript struct {
	ctx  *modscript.Context
	sink *bonusSink
}

// bonusSink keeps an equip context's active bonuses. A change, such as a
// timed bonus that starts or ends, marks the wearer's stats. The host
// call that made it computes them before it returns (FlushStats), and
// the end of the tick computes those of changes that no call made.
type bonusSink struct {
	z      *Zone
	p      *Player
	active []*modscript.Bonus
}

func (s *bonusSink) BonusAdded(b *modscript.Bonus) {
	s.active = append(s.active, b)
	s.z.markStats(s.p)
}

func (s *bonusSink) BonusRemoved(b *modscript.Bonus) {
	s.active = slices.DeleteFunc(s.active, func(x *modscript.Bonus) bool { return x == b })
	s.z.markStats(s.p)
}

// equippedRef is what an equipped handle points at.
type equippedRef struct {
	p    *Player
	slot int
	// card is the card's index for a card's handle, or -1.
	card int
}

// throwableGroup is IG_THROWABLE: ammunition whose script does not run
// when it is equipped.
const throwableGroup = "Throwable"

// wearer is the item.Wearer of a player.
func (z *Zone) wearer(p *Player) *item.Wearer {
	w := &item.Wearer{Job: p.char.Job, Male: p.char.Male, BaseLevel: 1, Skills: p.char.Skills, Renewal: true}
	if z.Progress != nil {
		w.BaseLevel = p.progress.BaseLevel
	}
	if z.Status != nil {
		w.MapID, _ = z.Status.MapID(p.char.Job)
		if m, ok := z.Status.EquipMapID(p.char.Job); ok && m != w.MapID {
			w.EquipsAs, w.EquipMapID = true, m
		}
		w.Renewal = z.Status.Renewal()
	}
	return w
}

// equipItem is clif_parse_EquipItem and pc_equipitem.
func (z *Zone) equipItem(p *Player, slot int, position uint32) {
	if z.Items == nil || slot < 0 || slot >= len(p.inventory.Slots) {
		return
	}
	s := &p.inventory.Slots[slot]
	if s.ID == 0 || s.Amount <= 0 {
		p.pending = append(p.pending, equipFailed(slot, wathenav1.EquipFailure_EQUIP_FAILURE_EMPTY_SLOT))
		return
	}
	d := z.Items.Get(s.ID)
	// pc_isequip: what a strip took off cannot go back on.
	if stripBlocksEquip(p.entity.sc, d) {
		p.pending = append(p.pending, equipFailed(slot, wathenav1.EquipFailure_EQUIP_FAILURE_CANNOT_EQUIP))
		return
	}
	if position == 0 {
		// The client asks for every position the item takes.
		position = uint32(d.Location)
	}
	w := z.wearer(p)
	weapon := p.inventory.Worn(z.Items).Weapon
	ch, r := p.inventory.Equip(z.Items, slot, position, w)
	if r == item.EquipFail && !item.AmmoFits(d, w, weapon) {
		// pc_isequip names the weapon the ammunition needs.
		p.pending = append(p.pending, equipFailed(slot, wathenav1.EquipFailure_EQUIP_FAILURE_WRONG_AMMUNITION))
		return
	}
	switch r {
	case item.EquipOK:
	case item.EquipFailLevel:
		p.pending = append(p.pending, equipFailed(slot, wathenav1.EquipFailure_EQUIP_FAILURE_LEVEL))
		return
	default:
		p.pending = append(p.pending, equipFailed(slot, wathenav1.EquipFailure_EQUIP_FAILURE_CANNOT_EQUIP))
		return
	}
	for _, u := range ch.Unequipped {
		z.unwear(p, u.Slot, true)
		z.unequipStatuses(p, u.Slot, u.Position)
		p.pending = append(p.pending, itemUnequipped(u.Slot, u.Position))
	}
	z.wear(p, slot)
	p.pending = append(p.pending, itemEquipped(slot, ch.Position))
	z.equipmentChanged(p)
}

// unequipItem is clif_parse_UnequipItem and pc_unequipitem.
func (z *Zone) unequipItem(p *Player, slot int) {
	if z.Items == nil || slot < 0 || slot >= len(p.inventory.Slots) {
		return
	}
	ch, ok := p.inventory.Unequip(z.Items, slot)
	if !ok {
		return
	}
	for _, u := range ch.Unequipped {
		z.unwear(p, u.Slot, true)
		z.unequipStatuses(p, u.Slot, u.Position)
		p.pending = append(p.pending, itemUnequipped(u.Slot, u.Position))
	}
	z.equipmentChanged(p)
}

// insertCard is clif_parse_InsertCard and pc_insert_card.
func (z *Zone) insertCard(p *Player, cardSlot, equipSlot int) {
	if z.Items == nil {
		return
	}
	inv := p.inventory
	fail := func() {
		p.pending = append(p.pending, wathenav1.Event_builder{InsertCardFailed: wathenav1.InsertCardFailed_builder{
			CardSlot:  uint32(max(cardSlot, 0)),  // #nosec G115 -- checked below MaxInventory by the gateway.
			EquipSlot: uint32(max(equipSlot, 0)), // #nosec G115 -- as above.
		}.Build()}.Build())
	}
	if cardSlot < 0 || cardSlot >= len(p.inventory.Slots) || equipSlot < 0 || equipSlot >= len(p.inventory.Slots) {
		fail()
		return
	}
	id := inv.Slots[cardSlot].ID
	if !inv.InsertCard(z.Items, cardSlot, equipSlot, z.maxCardSlots()) {
		fail()
		return
	}
	p.pending = append(p.pending, wathenav1.Event_builder{InventoryRemove: wathenav1.InventoryRemove_builder{
		Slot:   uint32(cardSlot), // #nosec G115 -- below MaxInventory.
		Amount: 1,
		Reason: wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_CARD_INSERTED,
	}.Build()}.Build())
	p.pending = append(p.pending, wathenav1.Event_builder{CardInserted: wathenav1.CardInserted_builder{
		CardSlot:  uint32(cardSlot),  // #nosec G115 -- below MaxInventory.
		EquipSlot: uint32(equipSlot), // #nosec G115 -- below MaxInventory.
		CardId:    id,
	}.Build()}.Build())
	z.updateWeight(p)
}

// maxCardSlots is MAX_SLOTS, the most cards an item holds.
func (z *Zone) maxCardSlots() int {
	if z.MaxCardSlots > 0 {
		return z.MaxCardSlots
	}
	return DefaultMaxCardSlots
}

// DefaultMaxCardSlots is rAthena's MAX_SLOTS.
const DefaultMaxCardSlots = 4

// equipmentChanged computes a player's stats and look again after its
// equipment changed, and sends what changed.
func (z *Zone) equipmentChanged(p *Player) {
	z.recalc(p)
	z.checkAllowSkill(p)
	if s := z.battleStats(p); s != nil {
		p.pending = append(p.pending, s)
	}
	z.updateLook(p)
}

// wear starts an equipped slot: its handle, and the equip contexts of its
// item and cards. Throwing weapons run no script, as status_calc_pc.
// Stats wait for the caller, which computes them once.
func (z *Zone) wear(p *Player, slot int) {
	z.holdStats++
	defer func() { z.holdStats-- }()
	s := &p.inventory.Slots[slot]
	if p.worn == nil {
		p.worn = map[int]*wornSlot{}
	}
	if _, ok := p.worn[slot]; ok {
		return
	}
	z.nextEquipped++
	ws := &wornSlot{handle: z.nextEquipped}
	if z.equippedRefs == nil {
		z.equippedRefs = map[int64]equippedRef{}
	}
	z.equippedRefs[ws.handle] = equippedRef{p, slot, -1}
	for i := range s.Cards {
		z.nextEquipped++
		ws.cardHandles = append(ws.cardHandles, z.nextEquipped)
		z.equippedRefs[z.nextEquipped] = equippedRef{p, slot, i}
	}
	p.worn[slot] = ws
	z.startSlotScripts(p, slot, ws)
}

// startSlotScripts starts the equip contexts of a worn slot's item and
// cards.
func (z *Zone) startSlotScripts(p *Player, slot int, ws *wornSlot) {
	s := &p.inventory.Slots[slot]
	d := z.Items.Get(s.ID)
	throwable := d.IsAmmo() && z.Mods != nil && z.Mods.Groups.Contains(throwableGroup, d.ID)
	if !throwable {
		ws.item = z.startEquipScript(p, ws.handle, d)
	}
	ws.cards = make([]*equipScript, len(s.Cards))
	for i, c := range s.Cards {
		if cd, ok := z.Items.Lookup(c); ok && i < len(ws.cardHandles) {
			ws.cards[i] = z.startEquipScript(p, ws.cardHandles[i], cd)
		}
	}
}

// restartSlot ends a worn slot's equip contexts without their unequip
// handlers and starts them again with the same handles, after its rolls
// changed. Stats wait for the caller.
func (z *Zone) restartSlot(p *Player, slot int) {
	ws := p.worn[slot]
	if ws == nil {
		return
	}
	z.holdStats++
	defer func() { z.holdStats-- }()
	if r := z.scripts; r != nil {
		for _, es := range append([]*equipScript{ws.item}, ws.cards...) {
			if es != nil {
				r.EndEquip(es.ctx)
			}
		}
	}
	ws.item, ws.cards = nil, nil
	z.startSlotScripts(p, slot, ws)
	z.markStats(p)
}

// startEquipScript runs an item's script in a new equip context, or
// returns nil when the item has no script or the zone runs none.
func (z *Zone) startEquipScript(p *Player, handle int64, d *item.Def) *equipScript {
	if d.Script == "" {
		return nil
	}
	r := z.itemScripts()
	if r == nil {
		return nil
	}
	sink := &bonusSink{z: z, p: p}
	ctx, err := r.Equip(d.Script, handle, int64(p.charID), sink)
	if err != nil {
		if !quietOff(err) && z.Mods != nil && z.Mods.Log != nil {
			z.Mods.Log.Warn("equip script failed", "map", z.Map, "item", d.AegisName, "script", d.Script, "err", err)
		}
		return nil
	}
	return &equipScript{ctx: ctx, sink: sink}
}

// unwear ends an equipped slot's contexts. With unequip, their unequip
// handlers run first, as rAthena's OnUnequipScript. Without it, as when
// the player leaves the map, they end quietly.
func (z *Zone) unwear(p *Player, slot int, unequip bool) {
	ws := p.worn[slot]
	if ws == nil {
		return
	}
	z.holdStats++
	defer func() { z.holdStats-- }()
	delete(p.worn, slot)
	delete(z.equippedRefs, ws.handle)
	for _, h := range ws.cardHandles {
		delete(z.equippedRefs, h)
	}
	r := z.scripts
	for _, es := range append([]*equipScript{ws.item}, ws.cards...) {
		if es == nil || r == nil {
			continue
		}
		if unequip {
			if err := r.Unequip(es.ctx); err != nil && z.Mods != nil && z.Mods.Log != nil {
				z.Mods.Log.Warn("unequip handler failed", "map", z.Map, "err", err)
			}
		} else {
			r.EndEquip(es.ctx)
		}
	}
	z.markStats(p)
}

// wearAll starts every equipped slot of a player who enters the zone.
func (z *Zone) wearAll(p *Player) {
	for i := range p.inventory.Slots {
		if s := &p.inventory.Slots[i]; s.ID != 0 && s.Equip != 0 {
			z.wear(p, i)
		}
	}
}

// unwearAll ends every equipped slot's contexts of a player who leaves
// the zone. The items stay equipped.
func (z *Zone) unwearAll(p *Player) {
	for _, slot := range wornSlots(p) {
		z.unwear(p, slot, false)
	}
}

// equipment is a player's equipment for status.Calc: every equipped
// item with its refine, cards, static bonuses, and the bonuses of its
// scripts whose condition holds, with computed values run now.
func (z *Zone) equipment(p *Player) *status.Equipment {
	src := func(slot, card int, d *item.Def) []status.Bonus {
		s := &p.inventory.Slots[slot]
		rolls := s.Rolls
		if card >= 0 {
			rolls = s.CardRoll(card)
		}
		out := status.StaticBonuses(d, rolls)
		ws := p.worn[slot]
		if ws == nil {
			return out
		}
		es := ws.item
		if card >= 0 {
			es = nil
			if card < len(ws.cards) {
				es = ws.cards[card]
			}
		}
		if es == nil || z.scripts == nil {
			return out
		}
		for _, b := range es.sink.active {
			args, chance, active, err := z.scripts.Values(b)
			if err != nil {
				if z.Mods != nil && z.Mods.Log != nil {
					z.Mods.Log.Warn("computed bonus failed", "map", z.Map, "item", d.AegisName, "bonus", b.Name, "err", err)
				}
				continue
			}
			if active {
				out = append(out, status.Bonus{Name: b.Name, Args: args, Chance: chance, Options: &b.Options})
			}
		}
		return out
	}
	var find func(string) *item.RandomOption
	if z.Mods != nil && z.Mods.RandomOptions != nil {
		find = z.Mods.RandomOptions.Option
	}
	return status.WearOptions(p.inventory, z.Items, src, find)
}

// markStats marks a player's stats for flushStats.
func (z *Zone) markStats(p *Player) {
	p.keepAspd, p.keepTempSkills = false, false
	if !p.statsDirty {
		p.statsDirty = true
		z.dirtyStats = append(z.dirtyStats, p)
	}
}

// statsChanged computes a player's stats again at once, as rAthena's
// status_calc_pc, unless a change of equipment or a join holds them.
func (z *Zone) statsChanged(p *Player) {
	z.markStats(p)
	z.flushStats()
}

// flushStats computes again the stats of the players marked, and sends
// what changed. Computed bonuses and when conditions run in Lua, as
// nested calls when a script's host call got here (internal/script,
// "Nested calls").
func (z *Zone) flushStats() {
	for z.holdStats == 0 && len(z.dirtyStats) > 0 {
		p := z.dirtyStats[0]
		z.dirtyStats = z.dirtyStats[1:]
		if !p.statsDirty || p.entity == nil || z.players[p.charID] != p {
			continue
		}
		p.statsDirty = false
		e := p.entity
		speed := e.speed
		z.recalc(p)
		if s := z.battleStats(p); s != nil {
			p.pending = append(p.pending, s)
		}
		z.speedChanged(e, speed)
	}
	if len(z.dirtyStats) == 0 {
		z.dirtyStats = nil
	}
}

// FlushStats computes the stats that a script's bonuses changed, before
// its host call returns (modscript.StatsHost).
func (h scriptHost) FlushStats() { h.z.flushStats() }

// updateLook sets a player's look from its equipment, as
// clif_changelook, and marks it for viewers when it changed.
func (z *Zone) updateLook(p *Player) {
	l := z.lookOf(p)
	if l != p.Look {
		p.Look = l
		p.entity.lookChanged = true
	}
}

// lookOf is a player's look: its job and hair, and the view_data of its
// equipment, with costumes over the items they cover
// (pc_set_costume_view).
func (z *Zone) lookOf(p *Player) Look {
	l := p.Look
	l.Weapon, l.Shield, l.HeadTop, l.HeadMid, l.HeadBottom, l.Robe, l.WeaponView = 0, 0, 0, 0, 0, 0, 0
	l.Companion, l.Mount = companionLookIDs(p)
	if z.Items == nil || p.inventory == nil {
		return l
	}
	worn := p.inventory.Worn(z.Items)
	l.Weapon, l.Shield = uint32(max(worn.Weapon, 0)), uint32(max(worn.Shield, 0)) // #nosec G115 -- types and views are small.
	view := func(eqi int) uint32 {
		if i := worn.Index[eqi]; i >= 0 {
			return uint32(max(z.Items.Get(p.inventory.Slots[i].ID).View, 0)) // #nosec G115 -- views are small.
		}
		return 0
	}
	// A weapon whose record names a view_sprite shows it, as headgear
	// does. Weapon stays the weapon type of attacks.
	if v := view(item.EqiHandR); item.ArtView(int(v)) {
		l.WeaponView = v
	}
	same := func(a, b int) bool { return worn.Index[a] >= 0 && worn.Index[a] == worn.Index[b] }
	l.HeadTop = view(item.EqiHeadTop)
	if !same(item.EqiHeadMid, item.EqiHeadTop) {
		l.HeadMid = view(item.EqiHeadMid)
	}
	if !same(item.EqiHeadLow, item.EqiHeadMid) && !same(item.EqiHeadLow, item.EqiHeadTop) {
		l.HeadBottom = view(item.EqiHeadLow)
	}
	l.Robe = view(item.EqiGarment)
	if v := view(item.EqiCostumeHeadTop); v != 0 {
		l.HeadTop = v
	}
	if v := view(item.EqiCostumeHeadMid); v != 0 {
		l.HeadMid = v
	}
	if v := view(item.EqiCostumeHeadLow); v != 0 {
		l.HeadBottom = v
	}
	if v := view(item.EqiCostumeGarment); v != 0 {
		l.Robe = v
	}
	return l
}

// slotPositions are the positions of the slot names of
// player:get_equipped.
var slotPositions = map[string]uint32{
	"head_top": item.EqpHeadTop, "head_middle": item.EqpHeadMid, "head_low": item.EqpHeadLow,
	"armor": item.EqpArmor, "left_hand": item.EqpHandL, "right_hand": item.EqpHandR,
	"garment": item.EqpGarment, "shoes": item.EqpShoes, "left_accessory": item.EqpAccL,
	"right_accessory": item.EqpAccR, "ammo": item.EqpAmmo,
	"costume_head_top": item.EqpCostumeHeadTop, "costume_head_middle": item.EqpCostumeHeadMid,
	"costume_head_low": item.EqpCostumeHeadLow, "costume_garment": item.EqpCostumeGarment,
	"shadow_armor": item.EqpShadowArmor, "shadow_weapon": item.EqpShadowWeapon,
	"shadow_shield": item.EqpShadowShield, "shadow_shoes": item.EqpShadowShoes,
	"shadow_right_accessory": item.EqpShadowAccR, "shadow_left_accessory": item.EqpShadowAccL,
}

// Equipped finds the copy a player has in an equipment slot, as
// getequipid.
func (h scriptHost) Equipped(id int64, slot string) (int64, bool, error) {
	p, err := h.player(id)
	if err != nil {
		return 0, false, err
	}
	bit := slotPositions[slot]
	for _, i := range wornSlots(p) {
		if p.inventory.Slots[i].Equip&bit != 0 {
			return p.worn[i].handle, true, nil
		}
	}
	return 0, false, nil
}

// EquippedField reads a field of an equipped copy (proposal 0002,
// "wa.EquippedItem").
func (h scriptHost) EquippedField(id int64, field string) (script.Value, error) {
	ref, ok := h.z.equippedRefs[id]
	if !ok || ref.p.gone() {
		return script.Nil, errNotEquipped
	}
	s := &ref.p.inventory.Slots[ref.slot]
	d := h.z.Items.Get(s.ID)
	switch field {
	case "item":
		return script.Int(int64(s.ID)), nil
	case "refine":
		return script.Int(int64(s.Refine)), nil
	case "enchant_grade":
		// Enchant grades do not exist yet.
		return script.Int(0), nil
	case "wearer":
		return script.Int(int64(ref.p.charID)), nil
	case "weapon_type":
		if !d.IsWeapon() {
			return script.Int(0), nil
		}
		return script.Int(int64(d.Subtype)), nil
	case "weapon_level":
		return script.Int(int64(d.WeaponLevel)), nil
	case "armor_level":
		return script.Int(int64(d.ArmorLevel)), nil
	case "rolls":
		return h.z.equippedRolls(ref.p, ref.slot, ref.card), nil
	case "cards":
		return h.z.equippedCards(ref.p, ref.slot), nil
	case "random_options":
		return equippedOptions(s), nil
	}
	return script.Nil, errNoField(field)
}

func equipFailed(slot int, reason wathenav1.EquipFailure) *wathenav1.Event {
	return wathenav1.Event_builder{EquipFailed: wathenav1.EquipFailed_builder{
		Slot:   uint32(slot), // #nosec G115 -- below MaxInventory.
		Reason: reason,
	}.Build()}.Build()
}

func itemEquipped(slot int, position uint32) *wathenav1.Event {
	return wathenav1.Event_builder{ItemEquipped: wathenav1.ItemEquipped_builder{
		Slot:     uint32(slot), // #nosec G115 -- below MaxInventory.
		Position: position,
	}.Build()}.Build()
}

func itemUnequipped(slot int, position uint32) *wathenav1.Event {
	return wathenav1.Event_builder{ItemUnequipped: wathenav1.ItemUnequipped_builder{
		Slot:     uint32(slot), // #nosec G115 -- below MaxInventory.
		Position: position,
	}.Build()}.Build()
}

// lookChange is the event that tells a viewer an entity's look changed.
func lookChange(id uint32, p *Player) *wathenav1.Event {
	return wathenav1.Event_builder{LookChange: wathenav1.LookChange_builder{Id: id, Look: p.Look.message()}.Build()}.Build()
}

var errNotEquipped = errors.New("the item is no longer equipped")

func errNoField(field string) error { return fmt.Errorf("equipped item has no field %q", field) }

// wornSlots returns a player's equipped slots in slot order, so handlers
// run in the same order every time.
func wornSlots(p *Player) []int {
	out := make([]int, 0, len(p.worn))
	for slot := range p.worn {
		out = append(out, slot)
	}
	slices.Sort(out)
	return out
}

// fireHit runs the "damage" handlers of a player's equipped items for a
// hit it dealt or took (0002a, "Item events").
func (z *Zone) fireHit(p *Player, h modscript.Hit) {
	r := z.scripts
	if r == nil || len(p.worn) == 0 {
		return
	}
	for _, slot := range wornSlots(p) {
		ws := p.worn[slot]
		for _, es := range append([]*equipScript{ws.item}, ws.cards...) {
			if es != nil && r.Listens(es.ctx, "damage") {
				if err := r.FireHit(es.ctx, h); err != nil && z.Mods != nil && z.Mods.Log != nil {
					z.Mods.Log.Warn("damage handler failed", "map", z.Map, "err", err)
				}
			}
		}
	}
}
