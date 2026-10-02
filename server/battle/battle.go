// Package battle computes attacks as rAthena's src/map/battle.cpp does, so
// results match the recorded reference in reference/rathena. Attack and
// SkillAttack follow the renewal mode and AttackPreRenewal the pre-renewal
// mode.
//
// This is a port of rAthena's battle.cpp (GPL-3.0). It covers a normal
// weapon attack by a monster or a player, with no skill. Of status
// changes, it reads what Changes carries: the ATK percent of Curse and
// other statuses, sleep's doubled critical rate, the sure hit on a
// stoned, frozen, stunned or sleeping target, and SC_DEF_RATE. Function comments
// name the rAthena functions they port.
package battle

// Weapon is a unit's weapon attack, as rAthena's weapon_atk. For a
// monster, Atk and Atk2 are its lowest and highest attack. For a player in
// pre-renewal, Atk is the weapon's ATK and Atk2 its refine bonus. Matk and
// Wlv only exist in renewal's weapon_atk. AttackPreRenewal still reads Wlv
// as the weapon level of the item in the hand, which rAthena reads from
// the item.
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
// from mob_status.json decodes into it. Level, EAtk, DamageTaken and
// Player are not in that object.
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
	// EAtk is the equipment ATK from bonus bBaseAtk, as status_data's
	// eatk. Only players have it.
	EAtk int `json:"eatk"`
	// DamageTaken is the percent of damage a monster takes, from
	// DamageTaken in rAthena's mob_db.yml. 0 counts as 100.
	DamageTaken int `json:"damage_taken"`
	// ID is status_get_class: a monster's ID, or a player's job.
	ID int `json:"-"`
	// Race2 are a monster's race groups (RC2_), as status_get_race2.
	Race2 []int `json:"-"`
	// CRate is a player's C.Rate trait stat: percent more critical damage.
	// PAtk is its P.Atk: percent more damage. SMatk is its S.Matk:
	// percent more magic damage.
	CRate int `json:"-"`
	PAtk  int `json:"-"`
	SMatk int `json:"-"`
	// Spl is the Spell trait stat, and HPlus the H.Plus trait stat: percent
	// more healing.
	Spl   int `json:"-"`
	HPlus int `json:"-"`
	// Player holds what only players have. It is nil for monsters.
	Player *Player `json:"-"`
	// Changes are what attacks read of the unit's status changes, or nil
	// for a unit without them.
	Changes *Changes `json:"-"`
}

// Changes are what attacks read of a unit's status changes.
type Changes struct {
	// Opt1 is the unit's body state (OPT1_). An attack on a unit with
	// one always hits, except during Stone Curse's wait and Burning.
	Opt1 int
	// Sleep doubles the critical chance of attacks on the unit.
	Sleep bool
	// AtkPercent is battle_get_atkpercent: the unit's attacks deal this
	// percent of their damage, from Curse, Provoke, Concentration and
	// SC_INCATKRATE.
	AtkPercent int
	// DefRate is SC_DEF_RATE's percent: a player takes that much less
	// physical damage.
	DefRate int
	// Block is battle_status_block_damage: it reports whether a status
	// blocks a hit with these battle flags, such as Safety Wall against a
	// melee hit or Pneuma against a ranged one, and counts the hit
	// against the status. It may be nil.
	Block func(flag, skill int, damage int64) bool
	// EnergyCoat is set while Energy Coat is on: it charges the SP and
	// returns the percent of damage it takes away.
	EnergyCoat func() int
	// WatkElement and WatkElementRate are SC_WATK_ELEMENT's val1 and
	// val2: an element and a percent. A rate of 0 means no status.
	//   - Pre-renewal: a player's physical hits deal that percent of the
	//     damage a normal attack of that element would deal.
	//   - Renewal: weapon ATK rises by that percent.
	WatkElement, WatkElementRate int
	// SubWeaponElement and SubWeaponRate are SC_SUB_WEAPONPROPERTY's val1
	// and val2: in renewal, a player's weapon ATK gains that percent of
	// itself as damage of that element. A rate of 0 means no status.
	SubWeaponElement, SubWeaponRate int
}

// Opt1 values that do not make attacks always hit, as rAthena's
// OPT1_STONEWAIT and OPT1_BURNING.
const (
	opt1StoneWait = 6
	opt1Burning   = 7
)

// atkPercent is battle_get_atkpercent for a normal attack.
func (s *Status) atkPercent() int64 {
	if s.Changes == nil {
		return 100
	}
	return int64(min(max(s.Changes.AtkPercent, 0), 0xffff))
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
	// damage2. Players have it with a katar or a weapon in the left hand.
	Left int64
	// Hits is how many hits the damage shows as, such as 2 for a double
	// attack.
	Hits int
	// Flag holds rAthena's e_battle_flag bits, such as BF_WEAPON and
	// BF_SHORT.
	Flag int
	// Knockback is the number of cells the hit pushes the target, as the
	// Damage's blewcount.
	Knockback int
	// Blocked is set when a status of the target blocked the hit
	// (ATK_BLOCK), such as Safety Wall. Kind is then Miss.
	Blocked bool
}

// Rand is the random source attacks draw from. IntN returns a number in
// [0, n), as rAthena's rnd() % n.
type Rand interface {
	IntN(n int) int
}

// Attack is a normal weapon attack on a target, as battle_calc_attack
// with BF_WEAPON and no skill. The attacker is a player when its Player
// is set, and a monster otherwise.
func Attack(attacker, target *Status, r Rand) Result {
	wd := calcWeaponAttack(attacker, target, nil, r)
	return finishAttack(&wd, attacker)
}

// result converts rAthena's Damage fields to a Result.
func (wd *damage) result() Result {
	res := Result{Damage: wd.damage + wd.damage2, Left: wd.damage2, Hits: wd.div, Flag: wd.flag,
		Knockback: wd.blewcount, Blocked: wd.dmgLv == atkBlock}
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
