package item

// Equipping and unequipping, ported from rAthena's pc.cpp: pc_equippoint,
// pc_isequip, pc_equipitem, pc_unequipitem, pc_setequipindex,
// pc_calcweapontype and pc_insert_card. Settings are the defaults of
// conf/battle: ammo_check_weapon and ammo_unequip are on, and
// allow_equip_restricted_item is on.

// Map job ID bits and masks that equipping reads, from rAthena's map.hpp.
const (
	joblSecond1     = 0x100
	joblSecond2     = 0x200
	joblThird       = 0x1000
	joblFourth      = 0x10000
	joblUpper       = 0x100000
	joblBaby        = 0x200000
	mapidFirstMask  = 0xff
	mapidSecondMask = 0xfff
	mapidThief      = 0x6
	mapidNinja      = 0x9
	mapidAssassin   = joblSecond1 | mapidThief
	mapidKagerou    = joblSecond1 | mapidNinja
)

// Skills that equipping reads.
const (
	skillLeftHandMastery = 133  // AS_LEFT
	skillVenomKnife      = 1004 // AS_VENOMKNIFE
)

// Jobs that equipping reads, as rAthena's e_job.
const (
	jobMechanic  = 4058 // JOB_MECHANIC
	jobGenetic   = 4071 // JOB_GENETIC
	jobMechanicT = 4064 // JOB_MECHANIC_T
	jobGeneticT  = 4078 // JOB_GENETIC_T
)

// Wearer is the part of a player that equipping reads.
type Wearer struct {
	// Job is the job ID, and MapID its map job ID (class_).
	Job   int
	MapID uint64
	Male  bool
	// BaseLevel is the base level.
	BaseLevel int
	// Skills maps learned skill IDs to levels.
	Skills map[int]int
	// Renewal picks renewal's rules where the modes differ.
	Renewal bool
}

// EquipResult is rAthena's e_ITEM_EQUIP_ACK.
type EquipResult int

const (
	EquipOK EquipResult = iota
	EquipFail
	// EquipFailLevel is a base level outside the item's range.
	EquipFailLevel
)

// EquipPoint is pc_equippoint_sub: the positions an item goes in for a
// wearer. One-handed swords, daggers and axes go in either hand for
// classes that dual wield.
func EquipPoint(d *Def, w *Wearer) uint32 {
	if d == nil || !d.IsEquipment() {
		return 0
	}
	ep := uint32(d.Location)
	if d.IsWeapon() && (d.Subtype == WeaponDagger || d.Subtype == Weapon1HSword || d.Subtype == Weapon1HAxe) {
		second := w.MapID & mapidSecondMask
		if w.Skills[skillLeftHandMastery] > 0 || second == mapidAssassin || second == mapidKagerou {
			if ep == EqpWeapon {
				return EqpArms
			}
			if ep == EqpShadowWeapon {
				return EqpShadowArms
			}
		}
	}
	return ep
}

// CanEquip is pc_isequip: whether the wearer may equip an item. weapon is
// the wearer's current weapon type (status.weapon), which ammunition
// needs. Status changes and map restrictions are not ported.
func CanEquip(d *Def, w *Wearer, weapon int) EquipResult {
	if d == nil {
		return EquipFail
	}
	if d.MinLevel > 0 && w.BaseLevel < d.MinLevel {
		return EquipFailLevel
	}
	if d.MaxLevel > 0 && w.BaseLevel > d.MaxLevel {
		return EquipFailLevel
	}
	switch d.Gender {
	case "Male":
		if !w.Male {
			return EquipFail
		}
	case "Female":
		if w.Male {
			return EquipFail
		}
	}
	if !AmmoFits(d, w, weapon) {
		return EquipFail
	}
	if !jobCanUse(d, w.MapID) || !classCanUse(d, w.MapID, w.Renewal) {
		return EquipFail
	}
	return EquipOK
}

// AmmoFits is the ammunition check of pc_isequip: arrows need a bow, an
// instrument or a whip, bullets and shells a gun, and so on. Items that
// are not ammunition always fit.
func AmmoFits(d *Def, w *Wearer, weapon int) bool {
	if d.Location&Locations(EqpAmmo) == 0 {
		return true
	}
	switch d.Subtype {
	case AmmoArrow:
		return weapon == WeaponBow || weapon == WeaponMusical || weapon == WeaponWhip
	case AmmoDagger:
		return w.Skills[skillVenomKnife] > 0
	case AmmoBullet, AmmoShell:
		return isGun(weapon, w.Renewal)
	case AmmoGrenade:
		return w.Renewal || weapon == WeaponGrenade
	case AmmoCannonball:
		// Mechanics need a Mado Gear and Geneticists a cart, which do
		// not exist yet.
		switch w.Job {
		case jobMechanic, jobMechanicT, jobGenetic, jobGeneticT:
			return false
		}
	}
	return true
}

// JobCanUse is pc_job_can_use_item and pc_isItemClass: whether a job may
// use or wear an item.
func JobCanUse(d *Def, mapID uint64, renewal bool) bool {
	return jobCanUse(d, mapID) && classCanUse(d, mapID, renewal)
}

// isGun reports whether bullets and shells fit a weapon type. In renewal
// grenade launchers take them too.
func isGun(weapon int, renewal bool) bool {
	switch weapon {
	case WeaponRevolver, WeaponRifle, WeaponGatling, WeaponShotgun:
		return true
	case WeaponGrenade:
		return renewal
	}
	return false
}

// jobCanUse is pc_job_can_use_item.
func jobCanUse(d *Def, mapID uint64) bool {
	if d.Jobs == nil {
		return true
	}
	job := uint64(1) << (mapID & mapidFirstMask)
	index := 0
	switch {
	case mapID&joblSecond1 != 0:
		index = 1
	case mapID&joblSecond2 != 0:
		index = 2
	}
	return d.Jobs[index]&job != 0
}

// classCanUse is pc_isItemClass. Classes 0 means every class.
func classCanUse(d *Def, mapID uint64, renewal bool) bool {
	c := d.Classes
	if c == 0 {
		return true
	}
	if c&ItemjNormal != 0 && mapID&(joblUpper|joblBaby|joblThird|joblFourth) == 0 {
		return true
	}
	if !renewal {
		return c&ItemjUpper != 0 && mapID&(joblUpper|joblThird) != 0 ||
			c&ItemjBaby != 0 && mapID&joblBaby != 0 ||
			c&(ItemjThird|ItemjThirdUpper|ItemjThirdBaby) != 0 && mapID&joblThird != 0
	}
	third, upper, baby, fourth := mapID&joblThird != 0, mapID&joblUpper != 0, mapID&joblBaby != 0, mapID&joblFourth != 0
	return c&ItemjUpper != 0 && upper && !third ||
		c&ItemjBaby != 0 && baby && !third ||
		c&ItemjThird != 0 && third && !upper && !baby && !fourth ||
		c&ItemjThirdUpper != 0 && third && upper && !fourth ||
		c&ItemjThirdBaby != 0 && third && baby && !fourth ||
		c&ItemjFourth != 0 && fourth
}

// Worn is what pc_setequipindex derives from the inventory: the slot
// in each equip index, the weapon types and the shield.
type Worn struct {
	// Index is equip_index: the inventory slot in each equip index, or
	// -1.
	Index [EqiMax]int
	// WeaponType1 and WeaponType2 are each hand's weapon type, as
	// weapontype1 and weapontype2. A shield or an empty hand is
	// WeaponFist.
	WeaponType1, WeaponType2 int
	// Weapon is status.weapon, from pc_calcweapontype: the hand's type, or
	// a W_DOUBLE_ type for a weapon in each hand.
	Weapon int
	// Shield is status.shield: the shield's view, or 0.
	Shield int
}

// Worn is pc_setequipindex followed by pc_calcweapontype.
func (inv *Inventory) Worn(t *Table) Worn {
	var w Worn
	for i := range w.Index {
		w.Index[i] = -1
	}
	for i := range inv.Slots {
		s := &inv.Slots[i]
		if s.ID == 0 || s.Equip == 0 {
			continue
		}
		for j, bit := range EquipBitmask {
			if s.Equip&bit != 0 {
				w.Index[j] = i
			}
		}
		d, ok := t.Lookup(s.ID)
		if s.Equip&EqpHandR != 0 {
			w.WeaponType1 = WeaponFist
			if ok {
				w.WeaponType1 = d.Subtype
			}
		}
		if s.Equip&EqpHandL != 0 {
			w.WeaponType2 = WeaponFist
			if ok && d.IsWeapon() {
				w.WeaponType2 = d.Subtype
			} else if ok && d.IsArmor() {
				w.Shield = d.View
			}
		}
	}
	w.Weapon = CalcWeaponType(w.WeaponType1, w.WeaponType2)
	return w
}

// CalcWeaponType is pc_calcweapontype: the weapon type of attacks.
func CalcWeaponType(w1, w2 int) int {
	if w2 == WeaponFist {
		return w1
	}
	if w1 == WeaponFist {
		return w2
	}
	weapon := WeaponFist
	switch w1 {
	case WeaponDagger:
		weapon = map[int]int{WeaponDagger: WeaponDoubleDD, Weapon1HSword: WeaponDoubleDS, Weapon1HAxe: WeaponDoubleDA}[w2]
	case Weapon1HSword:
		weapon = map[int]int{WeaponDagger: WeaponDoubleDS, Weapon1HSword: WeaponDoubleSS, Weapon1HAxe: WeaponDoubleSA}[w2]
	case Weapon1HAxe:
		weapon = map[int]int{WeaponDagger: WeaponDoubleDA, Weapon1HSword: WeaponDoubleSA, Weapon1HAxe: WeaponDoubleAA}[w2]
	}
	if weapon == WeaponFist {
		return w1
	}
	return weapon
}

// Change lists what an equip or unequip changed.
type Change struct {
	// Equipped is the slot that was equipped, and Position where.
	Equipped int
	Position uint32
	// Unequipped are slots that were taken off, with the positions they
	// had, in the order they came off.
	Unequipped []Unequipped
}

// Unequipped is one slot that came off.
type Unequipped struct {
	Slot     int
	Position uint32
}

// Equip is the part of pc_equipitem that changes the inventory: it
// checks the item with CanEquip, picks its positions from reqPos, takes
// off what is there, and puts the item on. Items it takes off are
// unequipped with flags 1|2|4, so ammunition stays until the new weapon
// is known. With ammo_unequip, ammunition that does not fit a new weapon
// comes off.
func (inv *Inventory) Equip(t *Table, slot int, reqPos uint32, w *Wearer) (Change, EquipResult) {
	ch := Change{Equipped: -1}
	if slot < 0 || slot >= MaxInventory {
		return ch, EquipFail
	}
	s := &inv.Slots[slot]
	d, ok := t.Lookup(s.ID)
	if s.ID == 0 || s.Amount <= 0 || !ok {
		return ch, EquipFail
	}
	pos := EquipPoint(d, w)
	worn := inv.Worn(t)
	if r := CanEquip(d, w, worn.Weapon); r != EquipOK {
		return ch, r
	}
	// pc_isequip: a broken item cannot be equipped.
	if s.Broken {
		return ch, EquipFail
	}
	if pos&reqPos == 0 || s.Equip != 0 {
		return ch, EquipFail
	}
	idx := worn.Index
	switch {
	case pos == EqpAcc:
		// Accessories go in one of the two positions.
		pos = reqPos & EqpAcc
		if pos == EqpAcc {
			pos = EqpAccR
			if idx[EqiAccR] >= 0 && idx[EqiAccL] < 0 {
				pos = EqpAccL
			}
		}
		// A card can force the position.
		for _, c := range s.Cards {
			if cd, ok := t.Lookup(c); ok {
				if cp := uint32(cd.Location); cp == EqpAccL || cp == EqpAccR {
					pos = cp
					break
				}
			}
		}
	case pos == EqpArms && uint32(d.Location) == EqpHandR:
		// A weapon that can go in either hand.
		pos = reqPos & EqpArms
		if pos == EqpArms {
			pos = EqpHandR
			if w.Renewal {
				if idx[EqiHandR] >= 0 && idx[EqiHandL] < 0 {
					pos = EqpHandL
				}
			} else if idx[EqiHandR] >= 0 {
				pos = EqpHandL
			}
		}
	case pos == EqpShadowAcc:
		pos = reqPos & EqpShadowAcc
		if pos == EqpShadowAcc {
			pos = EqpShadowAccL
			if idx[EqiShadowAccL] >= 0 && idx[EqiShadowAccR] < 0 {
				pos = EqpShadowAccR
			}
		}
	case pos == EqpShadowArms && uint32(d.Location) == EqpShadowWeapon:
		pos = reqPos & EqpShadowArms
		if pos == EqpShadowArms {
			pos = EqpShadowWeapon
			if idx[EqiShadowWeapon] >= 0 {
				pos = EqpShadowShield
			}
		}
	}
	for i, bit := range EquipBitmask {
		if pos&bit == 0 || idx[i] < 0 {
			continue
		}
		if o := idx[i]; inv.Slots[o].Equip != 0 {
			ch.Unequipped = append(ch.Unequipped, Unequipped{Slot: o, Position: inv.Slots[o].Equip})
			inv.Slots[o].Equip = 0
		}
	}
	s.Equip = pos
	ch.Equipped, ch.Position = slot, pos

	// ammo_unequip: ammunition that does not fit the new weapon.
	if pos&EqpArms != 0 && d.IsWeapon() {
		if a := inv.Worn(t).Index[EqiAmmo]; a >= 0 {
			if ad, ok := t.Lookup(inv.Slots[a].ID); ok && !ammoFits(ad.Subtype, d.Subtype, w.Renewal) {
				ch.Unequipped = append(ch.Unequipped, Unequipped{Slot: a, Position: inv.Slots[a].Equip})
				inv.Slots[a].Equip = 0
			}
		}
	}
	return ch, EquipOK
}

// ammoFits is the ammo_unequip check of pc_equipitem: whether ammunition
// of a type stays on with a new weapon of a type.
func ammoFits(ammo, weapon int, renewal bool) bool {
	switch ammo {
	case AmmoArrow:
		return weapon == WeaponBow || weapon == WeaponMusical || weapon == WeaponWhip
	case AmmoBullet, AmmoShell:
		return isGun(weapon, renewal)
	case AmmoGrenade:
		return renewal || weapon == WeaponGrenade
	}
	return true
}

// Unequip is the part of pc_unequipitem that changes the inventory: it
// takes a slot off. When a bow, instrument, whip or gun comes off, the
// ammunition comes off too (ammo_unequip). It returns false when the
// slot is not equipped.
func (inv *Inventory) Unequip(t *Table, slot int) (Change, bool) {
	ch := Change{Equipped: -1}
	if slot < 0 || slot >= MaxInventory || inv.Slots[slot].Equip == 0 {
		return ch, false
	}
	s := &inv.Slots[slot]
	pos := s.Equip
	ch.Unequipped = append(ch.Unequipped, Unequipped{Slot: slot, Position: pos})
	s.Equip = 0
	if d, ok := t.Lookup(s.ID); ok && pos&EqpArms != 0 && d.IsWeapon() {
		switch d.Subtype {
		case WeaponBow, WeaponMusical, WeaponWhip, WeaponRevolver, WeaponRifle, WeaponGatling, WeaponShotgun, WeaponGrenade:
			if a := inv.Worn(t).Index[EqiAmmo]; a >= 0 {
				ch.Unequipped = append(ch.Unequipped, Unequipped{Slot: a, Position: inv.Slots[a].Equip})
				inv.Slots[a].Equip = 0
			}
		}
	}
	return ch, true
}

// CanInsertCard is the check of pc_insert_card: whether a card fits into
// an item. maxSlots is the most card slots an item may have (MAX_SLOTS).
func CanInsertCard(card, equip *Def, s *Slot, maxSlots int) bool {
	if card == nil || equip == nil || s.ID == 0 || s.Amount < 1 {
		return false
	}
	if !equip.IsWeapon() && !equip.IsArmor() {
		return false
	}
	if !card.IsCard() {
		return false
	}
	ce, ee := uint32(card.Location), uint32(equip.Location)
	if ee&ce == 0 {
		return false
	}
	if equip.IsWeapon() && ce == EqpShield {
		// A shield card on a left-hand weapon.
		return false
	}
	if equip.IsArmor() && ce&EqpAcc != 0 && ce&EqpAcc != EqpAcc && ee&EqpAcc != ce&EqpAcc {
		// A card for one accessory position.
		return false
	}
	if s.Equip != 0 {
		return false
	}
	return len(s.Cards) < min(equip.Slots, maxSlots)
}

// InsertCard is pc_insert_card: a card from one slot goes into the first
// free card slot of the item in another. The card slot loses one card.
func (inv *Inventory) InsertCard(t *Table, cardSlot, equipSlot, maxSlots int) bool {
	if cardSlot < 0 || cardSlot >= MaxInventory || equipSlot < 0 || equipSlot >= MaxInventory || cardSlot == equipSlot {
		return false
	}
	cs, es := &inv.Slots[cardSlot], &inv.Slots[equipSlot]
	card, ok1 := t.Lookup(cs.ID)
	equip, ok2 := t.Lookup(es.ID)
	if !ok1 || !ok2 || cs.Amount < 1 || !CanInsertCard(card, equip, es, maxSlots) {
		return false
	}
	id := cs.ID
	if !inv.Delete(t, cardSlot, 1) {
		return false
	}
	es.Cards = append(es.Cards, id)
	return true
}
