// Package battle computes attacks as rAthena's src/map/battle.cpp does in
// its renewal mode, so results match the recorded reference in
// reference/rathena/renewal.
//
// This is a port of rAthena's battle.cpp (GPL-3.0). It covers a normal
// weapon attack by a monster, with no skill, status changes, cards or
// equipment. Function comments name the rAthena functions they port.
package battle

// Weapon is a unit's weapon attack, as rAthena's weapon_atk.
type Weapon struct {
	Atk   int `json:"atk"`
	Atk2  int `json:"atk2"`
	Range int `json:"range"`
	Ele   int `json:"ele"`
	Matk  int `json:"matk"`
	Wlv   int `json:"wlv"`
}

// Status is a unit's battle status, as rAthena's status_data. The field
// names and JSON keys follow the reference's JSON, so a "status" object
// from mob_status.json decodes into it. Level and DamageTaken are not in
// that object.
type Status struct {
	Level   int    `json:"level"`
	MaxHP   int    `json:"max_hp"`
	MaxSP   int    `json:"max_sp"`
	Str     int    `json:"str"`
	Agi     int    `json:"agi"`
	Vit     int    `json:"vit"`
	Int     int    `json:"int"`
	Dex     int    `json:"dex"`
	Luk     int    `json:"luk"`
	BAtk    int    `json:"batk"`
	WAtk    int    `json:"watk"`
	WAtk2   int    `json:"watk2"`
	MAtkMin int    `json:"matk_min"`
	MAtkMax int    `json:"matk_max"`
	Speed   int    `json:"speed"`
	AMotion int    `json:"amotion"`
	ADelay  int    `json:"adelay"`
	DMotion int    `json:"dmotion"`
	Mode    uint32 `json:"mode"`
	Hit     int    `json:"hit"`
	Flee    int    `json:"flee"`
	Cri     int    `json:"cri"`
	Flee2   int    `json:"flee2"`
	Def     int    `json:"def"`
	MDef    int    `json:"mdef"`
	Def2    int    `json:"def2"`
	MDef2   int    `json:"mdef2"`
	Res     int    `json:"res"`
	MRes    int    `json:"mres"`
	DefEle  int    `json:"def_ele"`
	EleLv   int    `json:"ele_lv"`
	Size    int    `json:"size"`
	Race    int    `json:"race"`
	Class   int    `json:"class"`
	RHW     Weapon `json:"rhw"`
	// LHW is the left-hand weapon. Monsters have none.
	LHW Weapon `json:"lhw"`
	// DamageTaken is the percent of damage a monster takes, from
	// DamageTaken in rAthena's mob_db.yml. 0 counts as 100.
	DamageTaken int `json:"damage_taken"`
}

// Monster modes that attacks read, as rAthena's e_mode.
const (
	ModeIgnoreMelee  = 0x0010000
	ModeIgnoreMagic  = 0x0020000
	ModeIgnoreRanged = 0x0040000
	ModeIgnoreMisc   = 0x0100000
)

// hasMode is rAthena's status_has_mode: every bit of md must be set.
func (s *Status) hasMode(md uint32) bool { return s.Mode&md == md }

// Kind is what an attack did.
type Kind int

const (
	Hit Kind = iota
	Critical
	Miss
	LuckyDodge
)

// Result is one attack's outcome.
type Result struct {
	Kind Kind
	// Damage is the total damage of both hands.
	Damage int64
	// Left is the part of Damage dealt by the left hand, as rAthena's
	// damage2. It is 0 for monsters.
	Left int64
	// Hits is how many hits the damage shows as, such as 2 for a double
	// attack.
	Hits int
	// Flag holds rAthena's e_battle_flag bits, such as BF_WEAPON and
	// BF_SHORT.
	Flag int
}

// Rand is the random source attacks draw from. IntN returns a number in
// [0, n), as rAthena's rnd() % n.
type Rand interface {
	IntN(n int) int
}

// Attack is a normal weapon attack by a monster on a target, as
// battle_calc_attack with BF_WEAPON, no skill and no status changes.
func Attack(attacker, target *Status, r Rand) Result {
	wd := calcWeaponAttack(attacker, target, r)

	// The rest of battle_calc_attack.
	if wd.damage+wd.damage2 < 1 {
		// A hit that deals no damage counts as a miss.
		if wd.dmgLv == atkDef {
			wd.dmgLv = atkMiss
		}
	} else {
		// Some damage lands even when the attack was dodged.
		wd.dmgLv = atkDef
	}
	return wd.result()
}

// result converts rAthena's Damage fields to a Result.
func (wd *damage) result() Result {
	res := Result{Damage: wd.damage + wd.damage2, Left: wd.damage2, Hits: wd.div, Flag: wd.flag}
	switch {
	case wd.dmgLv == atkLucky:
		res.Kind = LuckyDodge
	case wd.dmgLv != atkDef:
		res.Kind = Miss
	case wd.typ == dmgCritical || wd.typ == dmgMultiHitCritical:
		res.Kind = Critical
	default:
		res.Kind = Hit
	}
	return res
}
