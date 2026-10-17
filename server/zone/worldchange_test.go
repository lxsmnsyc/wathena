package zone

import (
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/server/character"
)

// TestRelocationMoves: the save point and memo points on a removed map
// move to its town's save spot, a character keeps one memo point per
// map, the newest first, and other points stay (decision 0045).
func TestRelocationMoves(t *testing.T) {
	r := &Relocations{}
	r.Add("gen:dungeon:crypt:2026-10-16:1", Relocation{Map: "gen:town:t2", X: 40, Y: 41})
	r.Add("gen:dungeon:crypt:2026-10-16:2", Relocation{Map: "gen:town:t2", X: 40, Y: 41})
	info := character.Info{SaveMap: "gen:dungeon:crypt:2026-10-16:2", SaveX: 9, SaveY: 9}
	memos := []character.Memo{
		{Map: "gen:dungeon:crypt:2026-10-16:1", X: 1, Y: 1},
		{Map: "gen:field:0:0", X: 2, Y: 2},
		{Map: "gen:dungeon:crypt:2026-10-16:2", X: 3, Y: 3},
	}
	out, kept, changed := r.Moves(info, memos)
	want := []character.Memo{{Map: "gen:town:t2", X: 40, Y: 41}, {Map: "gen:field:0:0", X: 2, Y: 2}}
	if !changed || out.SaveMap != "gen:town:t2" || out.SaveX != 40 || out.SaveY != 41 || !slices.Equal(kept, want) {
		t.Errorf("moved %s %d,%d %+v", out.SaveMap, out.SaveX, out.SaveY, kept)
	}
	if memos[0].Map != "gen:dungeon:crypt:2026-10-16:1" {
		t.Error("the memos given changed")
	}
	other := character.Info{SaveMap: "gen:town:t1", SaveX: 5, SaveY: 5}
	if out, kept, changed := r.Moves(other, want); changed || out != other || !slices.Equal(kept, want) {
		t.Errorf("points elsewhere moved: %+v %+v", out, kept)
	}
	var none *Relocations
	if _, ok := none.Lookup("gen:town:t1"); ok {
		t.Error("a nil list moves maps")
	}
}
