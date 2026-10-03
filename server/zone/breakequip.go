package zone

import (
	"github.com/lxsmnsyc/wathena/server/battle"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/skill"
	sc "github.com/lxsmnsyc/wathena/server/statuschange"
)

// Breaking equipment: the part of rAthena's skill_additional_effect that
// breaks weapons and armor on weapon hits, and skill_break_equip. Power
// Thrust and Maximum Power Thrust may break the attacker's own weapon in
// pre-renewal, and Melt Down breaks the target's weapon and armor. A
// broken item of a player comes off and cannot be equipped again until
// it is repaired. A monster gets the strip status of the part instead.

// Settings of conf/battle/battle.conf that breaking reads.
const (
	// equipNaturalBreakRate is equip_natural_break_rate: the chance per
	// 10,000 that any weapon hit breaks the attacker's weapon.
	equipNaturalBreakRate = 0
	// equipSelfBreakRate and equipSkillBreakRate are equip_self_break_rate
	// and equip_skill_break_rate: percents of the chances to break one's
	// own equipment and the target's.
	equipSelfBreakRate  = 100
	equipSkillBreakRate = 100
	// breakMobEquip is break_mob_equip: monsters' equipment breaks only
	// under Melt Down.
	breakMobEquip = false
)

// Statuses that breaking starts or reads, as rAthena's sc_type: the
// strip statuses a monster gets for a broken part, and the Chemical
// Protection statuses that keep a part whole.
const (
	scStripWeapon = 68 // SC_STRIPWEAPON
	scStripShield = 69 // SC_STRIPSHIELD
	scStripArmor  = 70 // SC_STRIPARMOR
	scStripHelm   = 71 // SC_STRIPHELM
	scCPWeapon    = 72 // SC_CP_WEAPON
	scCPShield    = 73 // SC_CP_SHIELD
	scCPArmor     = 74 // SC_CP_ARMOR
	scCPHelm      = 75 // SC_CP_HELM
)

// idTomahawk is ITM_TOMAHAWK, which breaks nothing.
const idTomahawk = 337

// breakParts are skill_break_equip's where_list, scatk and scdef for
// the parts wAthena has: weapon, armor, shield and helm. Accessories
// and shadow gear have no strip or protection status that is ported.
var breakParts = [...]struct {
	where         uint32
	strip, protec int
}{
	{item.EqpWeapon, scStripWeapon, scCPWeapon},
	{item.EqpArmor, scStripArmor, scCPArmor},
	{item.EqpShield, scStripShield, scCPShield},
	{item.EqpHelm, scStripHelm, scCPHelm},
}

// breakOnHit is the equipment breaking of skill_additional_effect for a
// weapon hit with battle flags: the attacker's own weapon, then the
// target's weapon and armor. skillID is 0 for a normal attack.
func (z *Zone) breakOnHit(src, target *entity, skillID, flag int) {
	if flag&battle.FlagWeapon == 0 {
		return
	}
	if src.player != nil && equipSelfBreakRate != 0 {
		// Self weapon breaking. Power Thrust and Maximum Power Thrust add
		// 0.1% each in pre-renewal.
		rate := equipNaturalBreakRate
		if !z.skillRenewal(skill.IDOverThrust) && src.sc.Has(sc.SCOverthrust) {
			rate += 10
		}
		if !z.skillRenewal(skill.IDOverThrustMax) && src.sc.Has(sc.SCMaxoverthrust) {
			rate += 10
		}
		if rate != 0 {
			z.breakEquip(src, src, item.EqpWeapon, rate, bctSelf)
		}
	}
	if equipSkillBreakRate == 0 || skillID == skill.IDCartTermination || skillID == idTomahawk {
		return
	}
	// The target's weapon, then its armor. The item bonuses
	// bBreakWeaponRate and bBreakArmorRate are not in the stats yet,
	// so only Melt Down and SC_WEAPONBREAKER give a chance.
	rate := 0
	if e := src.sc.Get(sc.SCMeltdown); e != nil {
		rate += e.Val2
	}
	if e := src.sc.Get(scWeaponBreaker); e != nil {
		rate += e.Val2
	}
	if rate != 0 {
		z.breakEquip(src, target, item.EqpWeapon, rate, bctEnemy)
	}
	rate = 0
	if e := src.sc.Get(sc.SCMeltdown); e != nil {
		rate += e.Val3
	}
	if rate != 0 {
		z.breakEquip(src, target, item.EqpArmor, rate, bctEnemy)
	}
}

// breakEquip is skill_break_equip: each part in where breaks with rate
// per 10,000. A player's broken item comes off; a monster gets the
// part's strip status. flag is bctEnemy for the target's equipment and
// bctSelf for one's own. It returns the parts that broke.
func (z *Zone) breakEquip(src, target *entity, where uint32, rate, flag int) uint32 {
	if !breakMobEquip && target.player == nil && !src.sc.Has(sc.SCMeltdown) {
		return 0
	}
	p := target.player
	if p != nil && where&item.EqpWeapon != 0 && p.inventory != nil && z.Items != nil {
		// Bare hands, axes, maces, staves, books and Huuma shuriken do
		// not break, nor axes in a dual wield.
		switch p.inventory.Worn(z.Items).Weapon {
		case battle.WeaponFist, battle.Weapon1HAxe, battle.Weapon2HAxe, battle.WeaponMace, battle.Weapon2HMace,
			battle.WeaponStaff, battle.Weapon2HStaff, battle.WeaponBook, battle.WeaponHuuma,
			item.WeaponDoubleAA, item.WeaponDoubleDA, item.WeaponDoubleSA:
			where &^= item.EqpWeapon
		}
	}
	switch {
	case flag&bctEnemy != 0:
		if equipSkillBreakRate != 100 {
			rate = rate * equipSkillBreakRate / 100
		}
	case flag&(bctParty|bctSelf) != 0:
		if equipSelfBreakRate != 100 {
			rate = rate * equipSelfBreakRate / 100
		}
	}
	for _, part := range breakParts {
		if where&part.where == 0 {
			continue
		}
		switch {
		case target.sc.Has(part.protec):
			where &^= part.where
		case z.rand.IntN(10000) >= rate:
			where &^= part.where
		case p == nil:
			// A monster's part is stripped instead.
			z.scStart(src, target, part.strip, 100, 0, int64(z.stripDuration(part.strip)))
		}
	}
	if where == 0 || p == nil {
		return where
	}
	z.breakItems(p, where)
	return where
}

// stripDuration is skill_get_time of a strip status's skill at level 1.
func (z *Zone) stripDuration(typ int) int {
	if z.Statuses == nil {
		return 0
	}
	if d := z.Statuses.Get(typ); d != nil {
		return d.Duration(1)
	}
	return 0
}

// breakItems is the player part of skill_break_equip: each equipped item
// in a part of where breaks and comes off, as attribute 1 and
// pc_unequipitem. A hand breaks for a weapon when it holds a weapon and
// for a shield when it holds armor.
func (z *Zone) breakItems(p *Player, where uint32) {
	if p.inventory == nil || z.Items == nil {
		return
	}
	worn := p.inventory.Worn(z.Items)
	for i, slot := range worn.Index {
		if slot < 0 || p.inventory.Slots[slot].Broken {
			continue
		}
		d, ok := z.Items.Lookup(p.inventory.Slots[slot].ID)
		if !ok {
			continue
		}
		var hit bool
		switch i {
		case item.EqiHeadTop:
			hit = where&item.EqpHelm != 0
		case item.EqiArmor:
			hit = where&item.EqpArmor != 0
		case item.EqiHandR, item.EqiHandL:
			hit = where&item.EqpWeapon != 0 && d.IsWeapon() || where&item.EqpShield != 0 && d.IsArmor()
		case item.EqiShoes:
			hit = where&item.EqpShoes != 0
		case item.EqiGarment:
			hit = where&item.EqpGarment != 0
		case item.EqiAccL:
			hit = where&item.EqpAccL != 0
		case item.EqiAccR:
			hit = where&item.EqpAccR != 0
		}
		if !hit || p.inventory.Slots[slot].Broken {
			continue
		}
		p.inventory.Slots[slot].Broken = true
		z.unequipItem(p, slot)
	}
}
