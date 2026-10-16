package store_test

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/store"
)

// testLayout is a small layout with a map of each kind.
func testLayout() *layout.Layout {
	return &layout.Layout{
		Maps: []layout.Map{
			{ID: "gen:field:0:0", Kind: "field", Name: "Valheim Fields", Levels: [2]int{12, 21}, Distance: 1, Size: 300, Biome: 2},
			{ID: "gen:town:t1", Kind: "town", Name: "Valheim", Levels: [2]int{1, 10}, Biome: 2, Record: "hamlet", Params: []byte{2, 1, 0, 96}, Town: 1},
			{ID: "gen:dungeon:crypt:1", Kind: "dungeon", Name: "Crypt of Valheim F1", Levels: [2]int{12, 21}, Distance: 2, X: 0, Y: 0, Biome: -1,
				Record: "crypt", Params: []byte{1, 1, 1}, Dungeon: "crypt", Floor: 1, Floors: 1},
			{ID: "gen:home:village", Kind: "home", Name: "Willow Village", Levels: [2]int{1, 10}, Biome: -1, Record: "example.home:village", NoMemo: true},
		},
		Links: []layout.Link{
			{A: layout.End{Map: "gen:home:village", Tag: "portal"}, B: layout.End{Map: "gen:town:t1", Tag: "portal"}},
			{A: layout.End{Map: "gen:field:0:0", Tag: "portal:west"}, B: layout.End{Map: "gen:town:t1", Tag: "portal", Index: 1}},
			{A: layout.End{Map: "gen:field:0:0", Tag: "portal:north"}, B: layout.End{Map: "gen:dungeon:crypt:1", Tag: "entrance"}},
		},
		Home: "gen:home:village",
	}
}

// TestWorldLayout: wactl world new writes the layout with the world, the
// game role reads it and renames maps but changes nothing else, and
// names stay unique in any case (decision 0041).
func TestWorldLayout(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	if _, err := store.LoadLayout(ctx, game); !errors.Is(err, store.ErrNoLayout) {
		t.Fatalf("before: %v", err)
	}
	l := testLayout()
	w := store.World{Name: "P", Ruleset: "renewal", Seed: bytes.Repeat([]byte{3}, 32), Mode: "procedural", GeneratorVersion: 3,
		RNGVersion: "rng-v1", PublicNode: bytes.Repeat([]byte{4}, 32), Layout: l}
	if _, err := store.NewWorld(ctx, admin, w, store.Admin{Name: "Admin1", Hash: "x"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadLayout(ctx, game)
	if err != nil {
		t.Fatal(err)
	}
	if got.Checksum() != l.Checksum() || got.Home != "gen:home:village" {
		t.Errorf("loaded %+v\nwant %+v", got, l)
	}
	// Renames, the town first: the field takes the town's old name.
	if err := store.RenameMaps(ctx, game, []layout.Rename{{Map: "gen:town:t1", Name: "Skarvik"}, {Map: "gen:field:0:0", Name: "Skarvik Fields"}}); err != nil {
		t.Fatal(err)
	}
	got, _ = store.LoadLayout(ctx, game)
	if got.Map("gen:town:t1").Name != "Skarvik" || got.Map("gen:field:0:0").Name != "Skarvik Fields" {
		t.Errorf("renamed %+v", got.Maps)
	}
	// A name another map has, in any case, fails, and the change before
	// it in the same call is undone.
	err = store.RenameMaps(ctx, game, []layout.Rename{{Map: "gen:town:t1", Name: "Eldholm"}, {Map: "gen:field:0:0", Name: "willow village"}})
	if !isCode(err, "23505") {
		t.Errorf("a taken name: %v", err)
	}
	if got, _ = store.LoadLayout(ctx, game); got.Map("gen:town:t1").Name != "Skarvik" {
		t.Errorf("a failed rename changed %+v", got.Map("gen:town:t1"))
	}
	if err := store.RenameMaps(ctx, game, []layout.Rename{{Map: "gen:town:t9", Name: "Nowhere"}}); err == nil {
		t.Error("a rename of no map")
	}
	// The game role changes only display names.
	for _, q := range []string{
		"UPDATE world_maps SET kind = 'town' WHERE id = 'gen:field:0:0'",
		"INSERT INTO world_maps (id, ord, kind, display_name, min_level, max_level, distance) VALUES ('gen:town:t9', 9, 'town', 'X', 1, 1, 0)",
		"DELETE FROM world_links",
	} {
		if _, err := game.Exec(ctx, q); !isCode(err, "42501") {
			t.Errorf("%s: %v", q, err)
		}
	}
}

// TestCharacterMaps: the maps a character entered and its memo points
// are saved and loaded with it, and a save only adds maps.
func TestCharacterMaps(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	s := newStore(t, game, store.Options{})
	id, err := s.DevCharacter(ctx, newCharacter("Walker"))
	if err != nil {
		t.Fatal(err)
	}
	c, sess, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Visited) != 0 || len(c.Memos) != 0 {
		t.Fatalf("a new character has %v and %v", c.Visited, c.Memos)
	}
	next := c.Clone()
	next.Visited = []string{"gen:town:t1", "gen:field:0:0"}
	next.Memos = []character.Memo{{Map: "gen:field:0:0", X: 10, Y: 20}, {Map: "gen:town:t1", X: 5, Y: 6}}
	sess.Save(next, true)
	if err := sess.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, sess, err := s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got.Visited, next.Visited) || !slices.Equal(got.Memos, next.Memos) {
		t.Errorf("loaded %v %v", got.Visited, got.Memos)
	}
	next = got.Clone()
	next.Visited = append(next.Visited, "gen:town:t2")
	next.Memos = next.Memos[1:]
	sess.Save(next, true)
	if err := sess.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got, _, err = s.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Visited) != 3 || !slices.Contains(got.Visited, "gen:town:t2") || !slices.Equal(got.Memos, next.Memos) {
		t.Errorf("after the second save %v %v", got.Visited, got.Memos)
	}
}
