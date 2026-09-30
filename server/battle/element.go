package battle

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
