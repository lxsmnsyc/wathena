package battle

// preRenewalAttrFixTable is the pre-renewal element table from rAthena's
// db/pre-re/attr_fix.yml, in percent. It is indexed like attrFixTable.
// Unlike renewal, some values are negative: such hits heal the target
// when attribute_recover is on.
var preRenewalAttrFixTable = [maxElementLevel][elementCount][elementCount]int{
	// Level 1.
	{
		{100, 100, 100, 100, 100, 100, 100, 100, 25, 100, 100},
		{100, 25, 100, 150, 50, 100, 75, 100, 100, 100, 100},
		{100, 100, 100, 50, 150, 100, 75, 100, 100, 100, 100},
		{100, 50, 150, 25, 100, 100, 75, 100, 100, 125, 100},
		{100, 175, 50, 100, 25, 100, 75, 100, 100, 100, 100},
		{100, 100, 125, 125, 125, 0, 75, 50, 100, -25, 100},
		{100, 100, 100, 100, 100, 100, 0, 125, 100, 150, 100},
		{100, 100, 100, 100, 100, 50, 125, 0, 100, -25, 100},
		{25, 100, 100, 100, 100, 100, 75, 75, 125, 100, 100},
		{100, 100, 100, 100, 100, 50, 100, 0, 100, 0, 100},
		{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
	},
	// Level 2.
	{
		{100, 100, 100, 100, 100, 100, 100, 100, 25, 100, 100},
		{100, 0, 100, 175, 25, 100, 50, 75, 100, 100, 100},
		{100, 100, 50, 25, 175, 100, 50, 75, 100, 100, 100},
		{100, 25, 175, 0, 100, 100, 50, 75, 100, 150, 100},
		{100, 175, 25, 100, 0, 100, 50, 75, 100, 100, 100},
		{100, 75, 125, 125, 125, 0, 50, 25, 75, -50, 100},
		{100, 100, 100, 100, 100, 100, -25, 150, 100, 175, 100},
		{100, 100, 100, 100, 100, 25, 150, -25, 100, -50, 100},
		{0, 75, 75, 75, 75, 75, 50, 50, 150, 125, 100},
		{100, 75, 75, 75, 75, 25, 125, 0, 100, 0, 100},
		{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
	},
	// Level 3.
	{
		{100, 100, 100, 100, 100, 100, 100, 100, 0, 100, 100},
		{100, -25, 100, 200, 0, 100, 25, 50, 100, 125, 100},
		{100, 100, 0, 0, 200, 100, 25, 50, 100, 75, 100},
		{100, 0, 200, -25, 100, 100, 25, 50, 100, 175, 100},
		{100, 200, 0, 100, -25, 100, 25, 50, 100, 100, 100},
		{100, 50, 100, 100, 100, 0, 25, 0, 50, -75, 100},
		{100, 100, 100, 100, 100, 125, -50, 175, 100, 200, 100},
		{100, 100, 100, 100, 100, 0, 175, -50, 100, -75, 100},
		{0, 50, 50, 50, 50, 50, 25, 25, 175, 150, 100},
		{100, 50, 50, 50, 50, 0, 150, 0, 100, 0, 100},
		{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
	},
	// Level 4.
	{
		{100, 100, 100, 100, 100, 100, 100, 100, 0, 100, 100},
		{100, -50, 100, 200, 0, 75, 0, 25, 100, 150, 100},
		{100, 100, -25, 0, 200, 75, 0, 25, 100, 50, 100},
		{100, 0, 200, -50, 100, 75, 0, 25, 100, 200, 100},
		{100, 200, 0, 100, -50, 75, 0, 25, 100, 100, 100},
		{100, 25, 75, 75, 75, 0, 0, -25, 25, -100, 100},
		{100, 75, 75, 75, 75, 125, -100, 200, 100, 200, 100},
		{100, 75, 75, 75, 75, -25, 200, -100, 100, -100, 100},
		{0, 25, 25, 25, 25, 25, 0, 0, 200, 175, 100},
		{100, 25, 25, 25, 25, -25, 175, 0, 100, 0, 100},
		{100, 100, 100, 100, 100, 100, 100, 100, 100, 100, 100},
	},
}

// PreRenewalAttribute is Attribute with the pre-renewal element table.
func PreRenewalAttribute(level, atkEle, defEle int) int {
	if level < 1 || level > maxElementLevel || !validElement(atkEle) || !validElement(defEle) {
		return 100
	}
	return preRenewalAttrFixTable[level-1][atkEle][defEle]
}

// AttrFixPreRenewal applies the pre-renewal element table to damage, as
// rAthena's pre-renewal battle_attr_fix with no status changes. With
// flag&1 unset, a negative percent counts as 0, because attribute_recover
// is off by default. With it set, the damage can turn negative.
//
// rAthena picks a random attack element when atkEle is not valid.
// AttrFixPreRenewal is only called with valid elements, so it leaves that
// out.
func AttrFixPreRenewal(damage int64, atkEle, defEle, defLv, flag int) int64 {
	// Values outside the table give 100%. rAthena also reports an error.
	ratio := int64(PreRenewalAttribute(defLv, atkEle, defEle))
	if !attrRecover && flag&1 == 0 && ratio < 0 {
		ratio = 0
	}
	// Pre-renewal truncates the result, so a reduction can reach 0.
	return damage * ratio / 100
}
