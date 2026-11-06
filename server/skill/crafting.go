package skill

// Crafting is crafting.json: rAthena's create_arrow_db.yml, which Arrow
// Crafting reads (skill_arrow_create), and produce_db.txt, which
// skill_produce_mix reads. The extract holds them as rAthena loads them
// for the mode.
type Crafting struct {
	Mode    string        `json:"mode"`
	Arrows  []ArrowSource `json:"arrows"`
	Produce []Recipe      `json:"produce"`
}

// ArrowSource is one entry of create_arrow_db.yml: Arrow Crafting uses
// one Source and makes each product's amount.
type ArrowSource struct {
	Source uint32       `json:"source"`
	Make   []ItemAmount `json:"make"`
}

// ItemAmount is an item and an amount.
type ItemAmount struct {
	Item   uint32 `json:"item"`
	Amount int    `json:"amount"`
}

// Recipe is one row of produce_db.txt: the product, the item level that
// picks the window or script making it (skill_can_produce_mix's
// trigger), the skill and level it needs (0 for none), and the
// materials. A material of amount 0 must be held but is not used.
type Recipe struct {
	ID         int          `json:"id"`
	Item       uint32       `json:"item"`
	ItemLevel  int          `json:"item_level"`
	Skill      int          `json:"skill"`
	SkillLevel int          `json:"skill_level"`
	Materials  []ItemAmount `json:"materials"`
}

// Arrow returns the arrow crafting entry of a source item, or nil.
func (c *Crafting) Arrow(source uint32) *ArrowSource {
	if c == nil {
		return nil
	}
	for i := range c.Arrows {
		if c.Arrows[i].Source == source {
			return &c.Arrows[i]
		}
	}
	return nil
}

// Crafting returns the mode's crafting data, or nil when it was not
// loaded.
func (db *DB) Crafting() *Crafting { return db.crafting }
