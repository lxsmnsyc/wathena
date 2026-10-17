package layout

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/schedule"
	"github.com/lxsmnsyc/wathena/shared/gen"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// TestParseRotation: a style's rotation reads as { every, at, weekday },
// at 00:00 when left out, and refuses bad values.
func TestParseRotation(t *testing.T) {
	r, err := ParseRotation(map[string]any{"every": "weekly", "weekday": "friday", "at": "20:00"})
	if err != nil || r.String() != "weekly on friday at 20:00" {
		t.Fatalf("weekly: %+v %v", r, err)
	}
	if s, _ := r.Spec(); s.Kind != schedule.Weekly || s.Weekday != time.Friday || s.Hour != 20 {
		t.Errorf("spec %+v", s)
	}
	if r, err := ParseRotation(map[string]any{"every": "daily"}); err != nil || r.At != "00:00" || r.String() != "daily at 00:00" {
		t.Errorf("daily: %+v %v", r, err)
	}
	for _, bad := range []any{
		"daily",
		map[string]any{"every": "hourly"},
		map[string]any{"every": "daily", "at": "25:00"},
		map[string]any{"every": "daily", "weekday": "monday"},
		map[string]any{"every": "weekly"},
		map[string]any{"every": "weekly", "weekday": "someday"},
		map[string]any{"every": "daily", "at": 4},
		map[string]any{"every": "daily", "when": "now"},
	} {
		if _, err := ParseRotation(bad); err == nil {
			t.Errorf("%v parsed", bad)
		}
	}
}

// TestFloorIDs: a rotating floor's map ID carries the date, and parses
// back, as a plain floor's does.
func TestFloorIDs(t *testing.T) {
	if id := FloorID("crypt-2", "2026-10-31", 3); id != "gen:dungeon:crypt-2:2026-10-31:3" {
		t.Errorf("id %s", id)
	}
	for id, want := range map[string][3]string{
		"gen:dungeon:crypt:2026-10-31:2": {"crypt", "2026-10-31", "2"},
		"gen:dungeon:crypt:2":            {"crypt", "", "2"},
	} {
		d, date, f, ok := ParseFloorID(id)
		if !ok || d != want[0] || date != want[1] || f != int(want[2][0]-'0') {
			t.Errorf("%s: %s %s %d %v", id, d, date, f, ok)
		}
	}
	for _, bad := range []string{"gen:dungeon:crypt:2026-02-30:1", "gen:dungeon:crypt:2026-10-31:0", "gen:dungeon:Crypt:2026-10-31:1",
		"gen:dungeon:crypt:2026-10-31:01", "gen:dungeon:crypt:x:1", "gen:town:t1", "gen:dungeon:crypt:2026-10-31"} {
		if _, _, _, ok := ParseFloorID(bad); ok {
			t.Errorf("%s parsed", bad)
		}
	}
	if DateNumber("2026-10-31") != 20261031 || DateNumber("nope") != 0 {
		t.Error("date numbers")
	}
	if DateOf(time.Date(2026, 10, 31, 23, 30, 0, 0, time.UTC), time.FixedZone("UTC+8", 8*3600)) != "2026-11-01" {
		t.Error("a date in the server's time zone")
	}
}

// rotationLayout is townLayout(3) with a two-floor crypt off the last
// field.
func rotationLayout() *Layout {
	l := townLayout(3)
	for f := 1; f <= 2; f++ {
		l.Maps = append(l.Maps, Map{ID: gen.DungeonMapID("crypt", f), Kind: gen.KindDungeon, X: 2, Biome: -1, Record: "crypt",
			Dungeon: "crypt", Floor: f, Floors: 2})
	}
	l.Links = append(l.Links,
		Link{End{gen.FieldID(2, 0), FieldPortal(gen.EdgeEast), 0}, End{gen.DungeonMapID("crypt", 1), TagEntrance, 0}},
		Link{End{gen.DungeonMapID("crypt", 1), TagExit, 0}, End{gen.DungeonMapID("crypt", 2), TagEntrance, 0}})
	for i, n := range []string{"Valheim", "Skarvik", "Eldholm"} {
		l.Maps[3+i].Name = n
	}
	DeriveNames(l)
	return l
}

// TestApply: the layout that runs takes the dated floors, with nomemo
// and their stored IDs, the links follow, and a region's season gives
// its fields one new biome; the stored layout stays as it was.
func TestApply(t *testing.T) {
	l := rotationLayout()
	root, _ := rng.NewRoot([]byte("apply"))
	public := root.Child("public")
	if r, ok := PickRotation(l, []StyleRecord{{Name: "crypt", Rotation: &RotationRule{Every: EveryDaily, At: "04:00"}}}); !ok || r.Dungeon != "crypt" {
		t.Errorf("pick %+v %v", r, ok)
	}
	if _, ok := PickRotation(l, []StyleRecord{{Name: "crypt"}}); ok {
		t.Error("a style without a rotation rotates")
	}
	run := l.Apply(public, 6, State{Dates: map[string]string{"crypt": "2026-10-31"}})
	f2 := run.Map(FloorID("crypt", "2026-10-31", 2))
	if f2 == nil || !f2.NoMemo || f2.Base != gen.DungeonMapID("crypt", 2) || f2.Date != "2026-10-31" || f2.Name != "Crypt of Eldholm F2" {
		t.Fatalf("floor 2 %+v", f2)
	}
	if l.Map(gen.DungeonMapID("crypt", 2)) == nil || l.Map(f2.ID) != nil {
		t.Error("the stored layout changed")
	}
	if !slices.ContainsFunc(run.Links, func(k Link) bool { return k.A.Map == FloorID("crypt", "2026-10-31", 1) && k.B.Map == f2.ID }) {
		t.Errorf("links %v", run.Links)
	}
	if MapNode(public, *f2).Key() == MapNode(public, *l.Map(gen.DungeonMapID("crypt", 2))).Key() {
		t.Error("a dated floor has its stored floor's node")
	}
	regions := Regions(l)
	if len(regions) != 3 || regions[0].Town != "t1" || !slices.Equal(regions[0].Fields, []string{gen.FieldID(0, 0)}) {
		t.Fatalf("regions %+v", regions)
	}
	if r, ok := RegionOf(regions, 2, 0); !ok || r != "t3" {
		t.Errorf("region of 2,0: %s", r)
	}
	s1 := l.Apply(public, 6, State{Seasons: map[string]int{"t3": 1}})
	s2 := l.Apply(public, 6, State{Seasons: map[string]int{"t3": 2}})
	f, g := s1.Map(gen.FieldID(2, 0)), s2.Map(gen.FieldID(2, 0))
	if f.Season != 1 || f.Biome == 0 || g.Season != 2 || g.Biome == f.Biome || s1.Map(gen.FieldID(1, 0)).Season != 0 {
		t.Errorf("seasons %+v %+v", f, g)
	}
	if SeasonNode(public, 2, 0, 1).Key() == gen.FieldNode(public, 2, 0).Key() || SeasonNode(public, 2, 0, 0).Key() != gen.FieldNode(public, 2, 0).Key() {
		t.Error("season nodes")
	}
}

// TestAtlasReplace: an atlas runs a new layout with its renames, and a
// rename saves a rotating floor's name by its stored ID.
func TestAtlasReplace(t *testing.T) {
	l := rotationLayout()
	var saved []Rename
	a := NewAtlas(l, AtlasOptions{Save: func(_ context.Context, names []Rename) error {
		saved = append(saved, names...)
		return nil
	}})
	root, _ := rng.NewRoot([]byte("apply"))
	a.Replace(a.Base().Apply(root.Child("public"), 6, State{Dates: map[string]string{"crypt": "2026-10-31"}}))
	id := FloorID("crypt", "2026-10-31", 1)
	if m, ok := a.Map(id); !ok || m.Name != "Crypt of Eldholm F1" {
		t.Fatalf("the dated floor %+v", m)
	}
	if _, ok := a.Map(gen.DungeonMapID("crypt", 1)); ok {
		t.Error("the stored floor still runs")
	}
	if tw, ok := a.NearestTown(id); !ok || tw.ID != "gen:town:t3" {
		t.Errorf("nearest town %+v", tw)
	}
	if _, err := a.Rename(context.Background(), "t3", "Newholm"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(saved, Rename{gen.DungeonMapID("crypt", 1), "Crypt of Newholm F1"}) {
		t.Errorf("saved %v", saved)
	}
	// The next rotation keeps the rename.
	a.Replace(a.Base().Apply(root.Child("public"), 6, State{Dates: map[string]string{"crypt": "2026-11-01"}}))
	if n, _ := a.Name(FloorID("crypt", "2026-11-01", 2)); !strings.HasPrefix(n, "Crypt of Newholm") {
		t.Errorf("after the rename: %q", n)
	}
}
