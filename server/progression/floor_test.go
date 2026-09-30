package progression

import (
	"slices"
	"testing"
)

func TestNextDropDir(t *testing.T) {
	d := DirCenter
	var got []Dir
	for range 5 {
		d = NextDropDir(d)
		got = append(got, d)
	}
	if want := []Dir{DirSouthEast, DirWest, DirNorth, DirSouthEast, DirWest}; !slices.Equal(got, want) {
		t.Errorf("%v", got)
	}
}

func TestPlaceItem(t *testing.T) {
	all := func(x, y int) bool { return true }
	none := func(x, y int) bool { return false }

	// The first item lies on the monster's cell, with no search. The
	// place inside the cell is 3, 6, 9 or 12 on each axis.
	r := &scripted{vals: []int{0, 3}}
	if got := PlaceItem(none, 50, 60, DirCenter, r); got != (Placement{50, 60, 3, 12}) {
		t.Errorf("center: %+v", got)
	}
	if !slices.Equal(r.drawn, []int{4, 4}) {
		t.Errorf("center draws %v", r.drawn)
	}

	// South-east is x+1, y-1 when that cell is free.
	r = &scripted{vals: []int{1, 2}}
	if got := PlaceItem(all, 50, 60, DirSouthEast, r); got != (Placement{51, 59, 6, 9}) {
		t.Errorf("free: %+v", got)
	}

	// When the cell is taken, the search draws two mirror flags, then two
	// distances for each direction it tries.
	west := func(x, y int) bool { return x == 49 && y == 60 }
	free := func(x, y int) bool { return !west(x, y) && x == 48 && y == 61 }
	r = &scripted{vals: []int{0, 1}}
	// Mirrors: x as is, y flipped. The first try is i = -1, j = -1:
	// (49-1, 60+1).
	if got := PlaceItem(free, 50, 60, DirWest, r); got.X != 48 || got.Y != 61 {
		t.Errorf("search: %+v", got)
	}
	if !slices.Equal(r.drawn, []int{2, 2, 1, 1, 4, 4}) {
		t.Errorf("search draws %v", r.drawn)
	}

	// With no free cell, the item stays on the direction's cell, after
	// all 8 tries.
	r = &scripted{}
	if got := PlaceItem(none, 50, 60, DirNorth, r); got.X != 50 || got.Y != 61 {
		t.Errorf("full: %+v", got)
	}
	if len(r.drawn) != 2+16+2 {
		t.Errorf("full draws %v", r.drawn)
	}
}

func TestRights(t *testing.T) {
	tb := New(&Data{}, nil, DefaultConfig)
	r := tb.NewRights(1, 2, 3, 1000, BossNone)
	if r.FirstTick != 4000 || r.SecondTick != 6000 || r.ThirdTick != 8000 {
		t.Fatalf("%+v", r)
	}
	for _, c := range []struct {
		id   uint32
		now  int64
		want bool
	}{
		{1, 1000, true}, {2, 3999, false}, {2, 4000, true}, {3, 5999, false}, {3, 6000, true},
		{4, 7999, false}, {4, 8000, true},
	} {
		if got := r.CanTake(c.id, c.now); got != c.want {
			t.Errorf("player %d at %d: %v", c.id, c.now, got)
		}
	}
	// Without a first looter anyone may take it.
	if !(Rights{}).CanTake(9, 0) {
		t.Error("no rights")
	}
	// Boss drops are protected for longer.
	r = tb.NewRights(1, 0, 0, 0, BossMini)
	if r.FirstTick != 10000 || r.SecondTick != 20000 || r.ThirdTick != 22000 {
		t.Errorf("boss: %+v", r)
	}
	// Without second and third looters, others still wait for all three
	// times.
	if r.CanTake(2, 21999) || !r.CanTake(2, 22000) {
		t.Error("others")
	}
}
