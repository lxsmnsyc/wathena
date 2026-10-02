package item

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Equipment positions, as rAthena's equip_pos (EQP_*) bits from mmo.hpp.
// An item's Location holds the positions it can go in, and an inventory
// slot's Equip holds the positions it is equipped in.
const (
	EqpHeadLow        uint32 = 0x000001
	EqpHandR          uint32 = 0x000002
	EqpGarment        uint32 = 0x000004
	EqpAccR           uint32 = 0x000008
	EqpArmor          uint32 = 0x000010
	EqpHandL          uint32 = 0x000020
	EqpShoes          uint32 = 0x000040
	EqpAccL           uint32 = 0x000080
	EqpHeadTop        uint32 = 0x000100
	EqpHeadMid        uint32 = 0x000200
	EqpCostumeHeadTop uint32 = 0x000400
	EqpCostumeHeadMid uint32 = 0x000800
	EqpCostumeHeadLow uint32 = 0x001000
	EqpCostumeGarment uint32 = 0x002000
	EqpAmmo           uint32 = 0x008000
	EqpShadowArmor    uint32 = 0x010000
	EqpShadowWeapon   uint32 = 0x020000
	EqpShadowShield   uint32 = 0x040000
	EqpShadowShoes    uint32 = 0x080000
	EqpShadowAccR     uint32 = 0x100000
	EqpShadowAccL     uint32 = 0x200000
	EqpWeapon                = EqpHandR
	EqpShield                = EqpHandL
	EqpArms                  = EqpHandR | EqpHandL
	EqpHelm                  = EqpHeadLow | EqpHeadMid | EqpHeadTop
	EqpAcc                   = EqpAccL | EqpAccR
	EqpShadowArms            = EqpShadowWeapon | EqpShadowShield
	EqpShadowAcc             = EqpShadowAccR | EqpShadowAccL
	EqpShadowGear            = EqpShadowArmor | EqpShadowWeapon | EqpShadowShield | EqpShadowShoes | EqpShadowAccR | EqpShadowAccL
)

// Equip indexes, as rAthena's equip_index (EQI_*). They are the order in
// which status_calc_pc runs equipment scripts.
const (
	EqiAccL = iota
	EqiAccR
	EqiShoes
	EqiGarment
	EqiHeadLow
	EqiHeadMid
	EqiHeadTop
	EqiArmor
	EqiHandL
	EqiHandR
	EqiCostumeHeadTop
	EqiCostumeHeadMid
	EqiCostumeHeadLow
	EqiCostumeGarment
	EqiAmmo
	EqiShadowArmor
	EqiShadowWeapon
	EqiShadowShield
	EqiShadowShoes
	EqiShadowAccR
	EqiShadowAccL
	EqiMax
)

// EquipBitmask is rAthena's equip_bitmask: the position of each equip
// index.
var EquipBitmask = [EqiMax]uint32{
	EqpAccL, EqpAccR, EqpShoes, EqpGarment, EqpHeadLow, EqpHeadMid, EqpHeadTop, EqpArmor, EqpHandL, EqpHandR,
	EqpCostumeHeadTop, EqpCostumeHeadMid, EqpCostumeHeadLow, EqpCostumeGarment, EqpAmmo,
	EqpShadowArmor, EqpShadowWeapon, EqpShadowShield, EqpShadowShoes, EqpShadowAccR, EqpShadowAccL,
}

// positionNames are the names of positions in item data: the slot names
// of player:get_equipped, and both_hand and both_accessory as rAthena's
// item_db has them.
var positionNames = map[string]uint32{
	"head_low": EqpHeadLow, "right_hand": EqpHandR, "garment": EqpGarment, "right_accessory": EqpAccR,
	"armor": EqpArmor, "left_hand": EqpHandL, "shoes": EqpShoes, "left_accessory": EqpAccL,
	"head_top": EqpHeadTop, "head_middle": EqpHeadMid, "costume_head_top": EqpCostumeHeadTop,
	"costume_head_middle": EqpCostumeHeadMid, "costume_head_low": EqpCostumeHeadLow, "costume_garment": EqpCostumeGarment,
	"ammo": EqpAmmo, "shadow_armor": EqpShadowArmor, "shadow_weapon": EqpShadowWeapon, "shadow_shield": EqpShadowShield,
	"shadow_shoes": EqpShadowShoes, "shadow_right_accessory": EqpShadowAccR, "shadow_left_accessory": EqpShadowAccL,
	"both_hand": EqpArms, "both_accessory": EqpAcc,
}

// Locations is an item's equipment positions. In item data it is a
// number, or a list of names such as [head_top, head_mid].
type Locations uint32

// UnmarshalJSON reads a number or a list of position names.
func (l *Locations) UnmarshalJSON(b []byte) error {
	var n uint32
	if err := json.Unmarshal(b, &n); err == nil {
		*l = Locations(n)
		return nil
	}
	var names []string
	if err := json.Unmarshal(b, &names); err != nil {
		return fmt.Errorf("location is a number or a list of positions, such as [head_top]")
	}
	var v uint32
	for _, name := range names {
		bit, ok := positionNames[strings.ToLower(name)]
		if !ok {
			return fmt.Errorf("location: %q is not a position", name)
		}
		v |= bit
	}
	*l = Locations(v)
	return nil
}

// JobMask is item_data's class_base: one bit per first job (MAPID_FIRSTMASK)
// for base jobs, 2-1 jobs and 2-2 jobs. In item data it is three decimal
// strings, since the values pass 2^53.
type JobMask [3]uint64

// UnmarshalJSON reads three decimal strings.
func (m *JobMask) UnmarshalJSON(b []byte) error {
	var parts [3]string
	if err := json.Unmarshal(b, &parts); err != nil {
		return fmt.Errorf("jobs is three decimal strings")
	}
	for i, p := range parts {
		v, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return fmt.Errorf("jobs: %w", err)
		}
		m[i] = v
	}
	return nil
}

// MarshalJSON writes three decimal strings.
func (m JobMask) MarshalJSON() ([]byte, error) {
	return json.Marshal([3]string{strconv.FormatUint(m[0], 10), strconv.FormatUint(m[1], 10), strconv.FormatUint(m[2], 10)})
}

// Item classes, as rAthena's e_item_job (ITEMJ_*).
const (
	ItemjNormal     uint32 = 0x01
	ItemjUpper      uint32 = 0x02
	ItemjBaby       uint32 = 0x04
	ItemjThird      uint32 = 0x08
	ItemjThirdUpper uint32 = 0x10
	ItemjThirdBaby  uint32 = 0x20
	ItemjFourth     uint32 = 0x40
)

// Weapon types that equipping reads, as rAthena's weapon_type.
const (
	WeaponFist     = 0
	WeaponDagger   = 1
	Weapon1HSword  = 2
	Weapon1HAxe    = 6
	WeaponBow      = 11
	WeaponMusical  = 13
	WeaponWhip     = 14
	WeaponRevolver = 17
	WeaponRifle    = 18
	WeaponGatling  = 19
	WeaponShotgun  = 20
	WeaponGrenade  = 21
	// The dual-wield types of pc_calcweapontype.
	WeaponDoubleDD = 25
	WeaponDoubleSS = 26
	WeaponDoubleAA = 27
	WeaponDoubleDS = 28
	WeaponDoubleDA = 29
	WeaponDoubleSA = 30
)

// Ammunition types, as rAthena's e_ammo_type.
const (
	AmmoArrow      = 1
	AmmoDagger     = 2
	AmmoBullet     = 3
	AmmoShell      = 4
	AmmoGrenade    = 5
	AmmoShuriken   = 6
	AmmoKunai      = 7
	AmmoCannonball = 8
	AmmoThrow      = 9
)

// EquipData is equipment.json, which reference/rathena extracted from
// rAthena's item_db. LoadItemData merges it into the items.
type EquipData struct {
	Mode  string      `json:"mode"`
	Items []EquipInfo `json:"items"`
}

// EquipInfo is one item of equipment.json.
type EquipInfo struct {
	ID          uint32    `json:"id"`
	Subtype     int       `json:"subtype"`
	Location    Locations `json:"location"`
	Atk         int       `json:"atk"`
	Matk        int       `json:"matk"`
	Def         int       `json:"def"`
	Range       int       `json:"range"`
	Slots       int       `json:"slots"`
	WeaponLevel int       `json:"weapon_level"`
	ArmorLevel  int       `json:"armor_level"`
	MinLevel    int       `json:"min_level"`
	MaxLevel    int       `json:"max_level"`
	Refineable  bool      `json:"refineable"`
	View        int       `json:"view"`
	Jobs        *JobMask  `json:"jobs"`
	Classes     *uint32   `json:"classes"`
	Sex         *int      `json:"sex"`
	NoEquip     uint32    `json:"no_equip"`
}

// MergeEquipment sets the equipment fields of items from equipment.json.
// Items it does not list keep theirs.
func (d *Data) MergeEquipment(e *EquipData) {
	byID := make(map[uint32]*Def, len(d.Items))
	for i := range d.Items {
		byID[d.Items[i].ID] = &d.Items[i]
	}
	for i := range e.Items {
		x := &e.Items[i]
		def := byID[x.ID]
		if def == nil {
			continue
		}
		def.Subtype, def.Location = x.Subtype, x.Location
		def.Atk, def.Matk, def.Defense, def.Range, def.Slots = x.Atk, x.Matk, x.Def, x.Range, x.Slots
		def.WeaponLevel, def.ArmorLevel, def.Refineable, def.View = x.WeaponLevel, x.ArmorLevel, x.Refineable, x.View
		def.MinLevel, def.MaxLevel = x.MinLevel, x.MaxLevel
		def.Jobs, def.NoEquip = x.Jobs, x.NoEquip
		if x.Classes != nil {
			def.Classes = *x.Classes
		}
		if x.Sex != nil {
			// The sex after ItemDatabase::defaultGender: instruments are
			// male-only and whips female-only.
			def.Gender = map[int]string{0: "Female", 1: "Male"}[*x.Sex]
		}
	}
}

// IsEquipment is itemdb_isequip2: weapons, armor, ammunition and shadow
// gear go in equipment positions.
func (d *Def) IsEquipment() bool {
	switch strings.ToLower(d.Type) {
	case "weapon", "armor", "ammo", "shadowgear":
		return true
	}
	return false
}

// IsWeapon reports whether the item is a weapon (IT_WEAPON).
func (d *Def) IsWeapon() bool { return strings.EqualFold(d.Type, "weapon") }

// IsArmor reports whether the item is armor (IT_ARMOR).
func (d *Def) IsArmor() bool { return strings.EqualFold(d.Type, "armor") }

// IsCard reports whether the item is a card (IT_CARD).
func (d *Def) IsCard() bool { return strings.EqualFold(d.Type, "card") }

// IsAmmo reports whether the item is ammunition (IT_AMMO).
func (d *Def) IsAmmo() bool { return strings.EqualFold(d.Type, "ammo") }

// IsShadowGear reports whether the item is shadow gear (IT_SHADOWGEAR).
func (d *Def) IsShadowGear() bool { return strings.EqualFold(d.Type, "shadowgear") }
