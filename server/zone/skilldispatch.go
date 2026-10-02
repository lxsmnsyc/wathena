package zone

import (
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/skill"
)

// Dispatch at cast end: skill_castend_damage_id, skill_castend_nodamage_id
// and skill_castend_pos2. Each runs the skill's behavior: a mod's script
// when the skill has one, or the Go behavior of skillimpls.go. The top
// level call then uses the ammunition and the items the skill costs.

// castendDamage is skill_castend_damage_id.
func (z *Zone) castendDamage(src, target *entity, d *skill.Def, lv, flag int) {
	if src.dead || target == nil || !z.entities.has(target) || target.dead {
		return
	}
	if !z.runSkillScript(src, target, d, lv, 0, 0, false) {
		if impl, ok := skillImpls[d.ID].(castendDamager); ok {
			impl.castendDamage(z, src, target, d, lv, &flag)
		}
	}
	if flag&1 == 0 {
		z.afterSkill(src, target, d, lv, 0, 0, flag, true)
	}
}

// castendNoDamage is skill_castend_nodamage_id.
func (z *Zone) castendNoDamage(src, target *entity, d *skill.Def, lv, flag int) {
	if src.dead || target == nil || !z.entities.has(target) || target.dead && d.ID != skill.IDResurrection {
		return
	}
	// A player's Heal on an undead target is a magic attack.
	if src.player != nil && d.ID == skill.IDHeal && target.status != nil && battle.IsUndead(target.status.Race, target.status.DefEle) {
		if !isEnemy(src, target) {
			z.skillFail(src, d.ID, wathenav1.SkillFailure_SKILL_FAILURE_LEVEL)
			return
		}
		z.castendDamage(src, target, d, lv, flag)
		return
	}
	// A status skill of an element above neutral cast on another unit
	// fails silently on a target that takes no damage from the element.
	if d.Status >= 0 && src != target && target.status != nil {
		if ele := d.Element.At(lv); ele > eleNeutral && ele < battle.ElementWeapon &&
			z.attrFix(100, ele, target.status.DefEle, target.status.EleLv) <= 0 {
			return
		}
	}
	if !z.runSkillScript(src, target, d, lv, 0, 0, false) {
		if impl, ok := skillImpls[d.ID].(castendNoDamager); ok {
			impl.castendNoDamage(z, src, target, d, lv, &flag)
		}
	}
	if flag&1 == 0 {
		z.afterSkill(src, target, d, lv, 0, 0, flag, true)
		// skill_onskillusage once for a no-damage skill.
		if src.player != nil && !src.dead {
			z.skillUsage(src, target, d, lv)
		}
	}
}

// castendPos is skill_castend_pos2.
func (z *Zone) castendPos(src *entity, x, y int, d *skill.Def, lv, flag int) {
	if src.dead {
		return
	}
	z.skillPosEffect(src, d, lv, x, y)
	if !z.runSkillScript(src, nil, d, lv, x, y, true) {
		if impl, ok := skillImpls[d.ID].(castendPoser); ok {
			impl.castendPos(z, src, x, y, d, lv, &flag)
		}
	}
	z.afterSkill(src, nil, d, lv, x, y, flag, flag&1 == 0)
	// skill_onskillusage without a target for a ground skill.
	if src.player != nil && !src.dead {
		z.skillUsage(src, nil, d, lv)
	}
}

// afterSkill is the end of the castend functions for the top level call:
// the ammunition (when ammo is set), then the items, unless the skill
// kept them. It also fires the skill_used event.
func (z *Zone) afterSkill(src, target *entity, d *skill.Def, lv, x, y, flag int, ammo bool) {
	if p := src.player; p != nil {
		if ammo && z.arrowAttack(src, d) {
			z.consumeAmmo(p, d, lv)
		}
		if flag&skillNoConsumeReq == 0 {
			z.consumeRequirement(src, d, lv, 2)
		}
	}
	z.skillUsed(src, target, d, lv, x, y)
}

// arrowAttack is sd->state.arrow_atk for a skill: it uses ammunition.
func (z *Zone) arrowAttack(src *entity, d *skill.Def) bool {
	if d.Requires.Ammo == 0 || src.player == nil {
		return false
	}
	_, ok := z.equippedAmmo(src.player)
	return ok
}

// consumeAmmo is battle_consume_ammo: the skill's ammunition count, at
// least 1, leaves the equipped stack (arrow_decrement).
func (z *Zone) consumeAmmo(p *Player, d *skill.Def, lv int) {
	slot, ok := z.equippedAmmo(p)
	if !ok {
		return
	}
	n := max(d.Requires.AmmoAmount.At(lv), 1)
	z.deleteItem(p, slot, min(n, p.inventory.Slots[slot].Amount), wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED)
}

// equippedAmmo is the inventory slot of the player's ammunition.
func (z *Zone) equippedAmmo(p *Player) (int, bool) {
	if p.inventory == nil || z.Items == nil {
		return 0, false
	}
	slot := p.inventory.Worn(z.Items).Index[item.EqiAmmo]
	if slot < 0 || p.inventory.Slots[slot].Amount <= 0 {
		return 0, false
	}
	return slot, true
}

// wears reports whether the player has an item equipped.
func (z *Zone) wears(p *Player, id uint32) bool {
	if p.inventory == nil {
		return false
	}
	for _, s := range p.inventory.Slots {
		if s.ID == id && s.Equip != 0 {
			return true
		}
	}
	return false
}

// countItem is pc_search_inventory's count: how many of an item the
// player has.
func (z *Zone) countItem(p *Player, id uint32) int {
	if p.inventory == nil {
		return 0
	}
	return p.inventory.Count(id)
}

// takeItems is pc_delitem over the first slots that hold an item. A
// player with too few loses nothing.
func (z *Zone) takeItems(p *Player, id uint32, amount int) bool {
	if amount <= 0 {
		return true
	}
	if z.countItem(p, id) < amount {
		return false
	}
	for slot := range p.inventory.Slots {
		if amount == 0 {
			break
		}
		s := p.inventory.Slots[slot]
		if s.ID != id || s.Amount <= 0 {
			continue
		}
		n := min(s.Amount, amount)
		z.deleteItem(p, slot, n, wathenav1.InventoryRemoveReason_INVENTORY_REMOVE_REASON_USED)
		amount -= n
	}
	return true
}

// hasCart is pc_iscarton. Carts do not exist yet, so only SC_PUSH_CART,
// which nothing starts, gives one.
func (z *Zone) hasCart(p *Player) bool { return p.entity.sc.Has(scPushCart) }

// scPushCart is SC_PUSH_CART.
const scPushCart = 518

// healHP is status_heal for HP, not forced: the unit gains HP up to its
// maximum.
func (z *Zone) healHP(e *entity, hp int64) {
	if hp <= 0 || e.dead || e.status == nil {
		return
	}
	if p := e.player; p != nil {
		z.statusHeal(p, hp, 0)
		return
	}
	e.hp = min(e.hp+hp, int64(e.status.MaxHP))
	e.hpChanged = true
}

// attrFix is battle_attr_fix in the zone's mode.
func (z *Zone) attrFix(damage int64, atkEle, defEle, defLv int) int64 {
	if z.renewal() {
		return battle.AttrFix(damage, atkEle, defEle, defLv, 0)
	}
	return battle.AttrFixPreRenewal(damage, atkEle, defEle, defLv, 0)
}

// moveTo is unit_warp within the map for a unit that is not a player:
// viewers see it vanish and appear at the new cell.
func (z *Zone) moveTo(e *entity, x, y int) {
	e.path = e.path[:0]
	e.x, e.y = x, y
	e.target = nil
	z.castCancel(e, false)
	z.removeMap(e)
	z.forget(e)
	for o := range z.entities.all() {
		if o.player == nil || o == e {
			continue
		}
		if id, ok := o.player.netIDs.get(e); ok {
			o.player.netIDs.del(e)
			z.queue(o.player, frame(z.tick, o.player.ackSeq, vanish(id, wathenav1.VanishReason_VANISH_REASON_TELEPORTED)))
		}
	}
	z.skillUnitMove(e)
}
