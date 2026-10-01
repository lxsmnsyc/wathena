package zone

import (
	"testing"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
)

// An Old Blue Box opens IG_BlueBox: its one Random subgroup gives one
// item, and the box is spent.
func TestUseItemBox(t *testing.T) {
	z, a, _ := invZone(t, StartItem{603, 2})
	before := a.inventory.Weight
	use(z, a, 0)
	z.stepAt(z.now + 50)
	ev := events(a)
	if a.inventory.Count(603) != 1 {
		t.Fatalf("boxes %d", a.inventory.Count(603))
	}
	var added []*wathenav1.InventoryAdd
	for _, e := range ev {
		if add := e.GetInventoryAdd(); add != nil {
			added = append(added, add)
		}
	}
	if len(added) != 1 || added[0].GetAmount() != 1 {
		t.Fatalf("added %v", added)
	}
	if got := a.inventory.Weight; got == before {
		t.Errorf("weight %d did not change", got)
	}
	// Ten more boxes draw from the zone's stream: they give items too.
	for range 10 {
		a.inventory.Add(z.Items.Get(603), 1, maxWeight*100)
	}
	a.maxWeight = maxWeight * 100
	for range 11 {
		z.stepAt(z.now + 150)
		use(z, a, 0)
	}
	if a.inventory.Count(603) != 0 {
		t.Errorf("%d boxes left", a.inventory.Count(603))
	}
}

// A Fly Wing warps to a random cell of the map and is spent after.
func TestUseItemFlyWing(t *testing.T) {
	z, a, b := invZone(t, StartItem{601, 2})
	x, y := a.entity.x, a.entity.y
	use(z, a, 0)
	z.stepAt(z.now + 50)
	if a.inventory.Count(601) != 1 {
		t.Errorf("wings %d", a.inventory.Count(601))
	}
	if a.entity.x == x && a.entity.y == y {
		t.Error("the player did not move")
	}
	if z.grid.NoPass(a.entity.x, a.entity.y) {
		t.Errorf("landed on a wall at %d,%d", a.entity.x, a.entity.y)
	}
	// The other player saw it leave.
	gone := false
	for _, e := range events(b) {
		if v := e.GetVanish(); v != nil {
			gone = true
		}
	}
	if !gone {
		t.Error("b did not see a leave")
	}
}

// A Butterfly Wing goes to the save point: the start map's start cell.
func TestUseItemButterflyWing(t *testing.T) {
	z, a, _ := invZone(t, StartItem{602, 1})
	a.entity.x, a.entity.y = 3, 3
	use(z, a, 0)
	z.stepAt(z.now + 50)
	sx, sy := z.StartCell()
	if a.entity.x != sx || a.entity.y != sy || a.inventory.Count(602) != 0 {
		t.Errorf("at %d,%d, start %d,%d, wings %d", a.entity.x, a.entity.y, sx, sy, a.inventory.Count(602))
	}
}

// A dead player cannot warp: the wing is not spent.
func TestUseItemWingWhenDead(t *testing.T) {
	z, a, _ := invZone(t, StartItem{601, 1})
	a.entity.dead = true
	use(z, a, 0)
	if a.inventory.Count(601) != 1 {
		t.Error("a dead player used a wing")
	}
}

// A zone whose mods only have item scripts makes its Lua state on the
// first use, not when it starts.
func TestItemScriptsStartLazily(t *testing.T) {
	z, a, _ := invZone(t, StartItem{501, 1})
	z.startScripts()
	if z.scripts != nil {
		t.Fatal("the zone made a Lua state before any item use")
	}
	a.entity.hp = 10
	use(z, a, 0)
	if z.scripts == nil || a.entity.hp == 10 {
		t.Errorf("state %v, HP %d", z.scripts != nil, a.entity.hp)
	}
	z.stopScripts()
}
