package skill

import (
	"testing"

	"github.com/lxsmnsyc/wathena/internal/reference"
)

// TestCrafting checks that crafting.json loads into the database of each
// mode, and that Arrow finds create_arrow_db.yml's entries.
func TestCrafting(t *testing.T) {
	for _, mode := range reference.Modes {
		var d Data
		if err := reference.LoadFile("extracts/"+string(mode)+"/skills.json", &d); err != nil {
			t.Fatal(err)
		}
		d.Crafting = &Crafting{}
		if err := reference.LoadFile("extracts/"+string(mode)+"/crafting.json", d.Crafting); err != nil {
			t.Fatal(err)
		}
		db, err := Load(&d)
		if err != nil {
			t.Fatal(err)
		}
		c := db.Clone().Crafting()
		if c == nil || c.Mode != string(mode) || len(c.Produce) == 0 {
			t.Fatalf("%s: crafting %v", mode, c)
		}
		// Empty_Bottle makes 2 Iron_Arrow.
		a := c.Arrow(713)
		if a == nil || len(a.Make) != 1 || a.Make[0] != (ItemAmount{Item: 1770, Amount: 2}) {
			t.Errorf("%s: Empty Bottle %+v", mode, a)
		}
		if c.Arrow(512) != nil {
			t.Errorf("%s: an Apple makes arrows", mode)
		}
	}
	var none *Crafting
	if none.Arrow(713) != nil {
		t.Error("nil crafting found an arrow")
	}
}
