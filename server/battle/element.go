package battle

import "github.com/lxsmnsyc/wathena/server/rules"

// Elements, as rAthena's e_element.
const (
	Neutral = iota
	Water
	Earth
	Fire
	Wind
	Poison
	Holy
	Dark
	Ghost
	Undead
	// AllElements is ELE_ALL. Its row and column hold 100%.
	AllElements
	elementCount
)

// maxElementLevel is rAthena's MAX_ELE_LEVEL.
const maxElementLevel = 4

// attrFixTable is the renewal element table from rAthena's
// db/re/attr_fix.yml, in percent. It is indexed by defense element level
// minus one, then attack element, then defense element, as
// AttributeDatabase::getAttribute.
var attrFixTable = [maxElementLevel][elementCount][elementCount]int{
	// Level 1.
	{
		{100, 100, 100, 100, 100, 100, 100, 100, 90, 100, 100},
		{100, 25, 100, 150, 90, 150, 100, 100, 100, 100, 100},
		{100, 100, 25, 90, 150, 150, 100, 100, 100, 100, 100},
		{100, 90, 150, 25, 100, 150, 100, 100, 100, 125, 100},
		{100, 150, 90, 100, 25, 150, 100, 100, 100, 100, 100},
		{100, 150, 150, 150, 150, 0, 75, 75, 75, 75, 100},
		{100, 100, 100, 100, 100, 75, 0, 125, 100, 125, 100},
		{100, 100, 100, 100, 100, 75, 125, 0, 100, 0, 100},
		{90, 100, 100, 100, 100, 75, 90, 90, 125, 100, 100},
		{100, 100, 100, 90, 100, 75, 125, 0, 100, 0, 100},
		{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
	},
	// Level 2.
	{
		{100, 100, 100, 100, 100, 100, 100, 100, 70, 100, 100},
		{100, 0, 100, 175, 80, 150, 100, 100, 100, 100, 100},
		{100, 100, 0, 80, 175, 150, 100, 100, 100, 100, 100},
		{100, 80, 175, 0, 100, 150, 100, 100, 100, 150, 100},
		{100, 175, 80, 100, 0, 150, 100, 100, 100, 100, 100},
		{100, 150, 150, 150, 150, 0, 75, 75, 75, 50, 100},
		{100, 100, 100, 100, 100, 75, 0, 150, 100, 150, 100},
		{100, 100, 100, 100, 100, 75, 150, 0, 100, 0, 100},
		{70, 100, 100, 100, 100, 75, 80, 80, 150, 125, 100},
		{100, 100, 100, 80, 100, 50, 150, 0, 125, 0, 100},
		{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
	},
	// Level 3.
	{
		{100, 100, 100, 100, 100, 100, 100, 100, 50, 100, 100},
		{100, 0, 100, 200, 70, 125, 100, 100, 100, 100, 100},
		{100, 100, 0, 70, 200, 125, 100, 100, 100, 100, 100},
		{100, 70, 200, 0, 100, 125, 100, 100, 100, 175, 100},
		{100, 200, 70, 100, 0, 125, 100, 100, 100, 100, 100},
		{100, 125, 125, 125, 125, 0, 50, 50, 50, 25, 100},
		{100, 100, 100, 100, 100, 50, 0, 175, 100, 175, 100},
		{100, 100, 100, 100, 100, 50, 175, 0, 100, 0, 100},
		{50, 100, 100, 100, 100, 50, 70, 70, 175, 150, 100},
		{100, 100, 100, 70, 100, 25, 175, 0, 150, 0, 100},
		{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
	},
	// Level 4.
	{
		{100, 100, 100, 100, 100, 100, 100, 100, 0, 100, 100},
		{100, 0, 100, 200, 60, 125, 100, 100, 100, 100, 100},
		{100, 100, 0, 60, 200, 125, 100, 100, 100, 100, 100},
		{100, 60, 200, 0, 100, 125, 100, 100, 100, 200, 100},
		{100, 200, 60, 100, 0, 125, 100, 100, 100, 100, 100},
		{100, 125, 125, 125, 125, 0, 50, 50, 50, 0, 100},
		{100, 100, 100, 100, 100, 50, 0, 200, 100, 200, 100},
		{100, 100, 100, 100, 100, 50, 200, 0, 100, 0, 100},
		{0, 100, 100, 100, 100, 50, 60, 60, 200, 175, 100},
		{100, 100, 100, 60, 100, 0, 200, 0, 175, 0, 100},
		{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
	},
}

// Attribute returns the element table's percent for an attack element
// against a defense element and level, as
// AttributeDatabase::getAttribute. It returns 100 for values outside the
// table.
func Attribute(level, atkEle, defEle int) int {
	if level < 1 || level > maxElementLevel || !validElement(atkEle) || !validElement(defEle) {
		return 100
	}
	return attrFixTable[level-1][atkEle][defEle]
}

// validElement is rAthena's CHK_ELEMENT.
func validElement(ele int) bool { return ele >= 0 && ele < elementCount }

// AttrFix applies the element table to damage, as rAthena's renewal
// battle_attr_fix with no status changes. With flag&1 unset, a negative
// percent counts as 0, because attribute_recover is off by default.
//
// rAthena picks a random attack element when atkEle is not valid. AttrFix
// is only called with valid elements, so it leaves that out.
func AttrFix(damage int64, atkEle, defEle, defLv, flag int) int64 {
	// Values outside the table give 100%. rAthena also reports an error.
	ratio := int64(Attribute(defLv, atkEle, defEle))
	if !attrRecover && flag&1 == 0 && ratio < 0 {
		ratio = 0
	}
	// In renewal, reductions are rounded down, so damage only reaches 0
	// when the percent is 0.
	return damage - damage*(100-ratio)/100
}

// attrFix is battle_attr_fix with the statuses of the attacker and the
// target, in renewal: the ground fields the attacker stands on raise the
// percent of their element, and Fiber Lock doubles a fire hit once.
func attrFix(src, target *Status, damage int64, atkEle, defEle, defLv, flag int) int64 {
	ratio := int64(Attribute(defLv, atkEle, defEle))
	ratio += int64(fieldBonus(src, atkEle))
	if spiderWebFire(target, atkEle) {
		ratio += 100
	}
	if !attrRecover && flag&1 == 0 && ratio < 0 {
		ratio = 0
	}
	return damage - damage*(100-ratio)/100
}

// ruleset returns the attack's rules: the world's, or those of its
// mode's core ruleset.
func (wd *damage) ruleset() *rules.Rules {
	if wd.rules != nil {
		return wd.rules
	}
	return rules.Of(!wd.pre)
}

// attrFix is battle_attr_fix as the element_table flag picks it:
// renewal's table and status bonuses, or pre-renewal's. The attack's
// mode does not matter.
func (wd *damage) attrFix(src, target *Status, damage int64, atkEle, defEle, defLv, flag int) int64 {
	if wd.ruleset().ElementTable {
		return attrFix(src, target, damage, atkEle, defEle, defLv, flag)
	}
	return attrFixPre(src, target, damage, atkEle, defEle, defLv, flag)
}

// attrFixPlain is wd.attrFix with no status changes, as AttrFix and
// AttrFixPreRenewal.
func (wd *damage) attrFixPlain(damage int64, atkEle, defEle, defLv, flag int) int64 {
	return ElementFix(wd.ruleset(), damage, atkEle, defEle, defLv, flag)
}

// ElementFix applies the element table that the element_table flag of
// rl picks: AttrFix or AttrFixPreRenewal.
func ElementFix(rl *rules.Rules, damage int64, atkEle, defEle, defLv, flag int) int64 {
	if rl.ElementTable {
		return AttrFix(damage, atkEle, defEle, defLv, flag)
	}
	return AttrFixPreRenewal(damage, atkEle, defEle, defLv, flag)
}

// attrFixPre is attrFix in pre-renewal: the statuses raise the damage
// before the element table.
func attrFixPre(src, target *Status, damage int64, atkEle, defEle, defLv, flag int) int64 {
	ratio := int64(PreRenewalAttribute(defLv, atkEle, defEle))
	if b := fieldBonus(src, atkEle); b != 0 {
		damage += damage * int64(b) / 100
	}
	if spiderWebFire(target, atkEle) {
		damage *= 2
	}
	if !attrRecover && flag&1 == 0 && ratio < 0 {
		ratio = 0
	}
	return damage * ratio / 100
}

// fieldBonus is the percent that the attacker's Volcano, Deluge or
// Violent Gale adds to an attack of its element.
func fieldBonus(src *Status, atkEle int) int {
	if src == nil || src.Changes == nil {
		return 0
	}
	switch atkEle {
	case Fire:
		return src.Changes.VolcanoRate
	case Wind:
		return src.Changes.ViolentGaleRate
	case Water:
		return src.Changes.DelugeRate
	}
	return 0
}

// spiderWebFire reports whether Fiber Lock doubles a fire hit on the
// target, and ends a web when it does.
func spiderWebFire(target *Status, atkEle int) bool {
	return atkEle == Fire && target != nil && target.Changes != nil && target.Changes.SpiderWeb != nil &&
		target.Changes.SpiderWeb()
}
