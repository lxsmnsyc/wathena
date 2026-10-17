package store_test

import (
	"bytes"
	"slices"
	"testing"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/store"
)

// TestWorldChanges: wactl world new writes the rotating dungeon and the
// derived keys, the game role reads them, and a change records the new
// date or season with the moves of the offline characters' maps, save
// points and memo points in one transaction; the game role changes
// nothing else (decision 0045).
func TestWorldChanges(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	rotKey, seasonKey := bytes.Repeat([]byte{5}, 32), bytes.Repeat([]byte{6}, 32)
	w := store.World{Name: "P", Ruleset: "renewal", Seed: bytes.Repeat([]byte{3}, 32), Mode: "procedural", GeneratorVersion: 3,
		RNGVersion: "rng-v1", PublicNode: bytes.Repeat([]byte{4}, 32), Layout: testLayout(),
		Rotations: []store.WorldRotation{{Rotation: layout.Rotation{Dungeon: "crypt", Style: "crypt",
			Rule: layout.RotationRule{Every: "weekly", Weekday: "friday", At: "20:00"}}, Key: rotKey}},
		SeasonKey: seasonKey}
	if _, err := store.NewWorld(ctx, admin, w, store.Admin{Name: "Admin1", Hash: "x"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadWorldChanges(ctx, game)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rotations) != 1 || got.Rotations[0].Rule.String() != "weekly on friday at 20:00" || !bytes.Equal(got.Rotations[0].Key, rotKey) ||
		got.Rotations[0].ActiveDate != "" || !bytes.Equal(got.SeasonKey, seasonKey) || got.SeasonsStarted != "" || len(got.Regions) != 0 {
		t.Fatalf("loaded %+v", got)
	}
	// Characters: one inside the old floor, one with its save point and
	// a memo point there, and one with memo points on the floor and on
	// the town already.
	s := newStore(t, game, store.Options{})
	const old = "gen:dungeon:crypt:2026-10-16:1"
	add := func(name string, f func(c *character.Character)) int64 {
		c := newCharacter(name)
		f(c)
		id, err := s.DevCharacter(ctx, c)
		if err != nil {
			t.Fatal(err)
		}
		loaded, sess, err := s.Load(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		next := loaded.Clone()
		f(next)
		sess.Save(next, true)
		if err := sess.Flush(ctx); err != nil {
			t.Fatal(err)
		}
		return id
	}
	inside := add("Inside", func(c *character.Character) { c.Map, c.X, c.Y = old, 7, 8 })
	saved := add("Saver", func(c *character.Character) {
		c.SaveMap, c.SaveX, c.SaveY = old, 3, 4
		c.Memos = []character.Memo{{Map: "gen:field:0:0", X: 1, Y: 1}, {Map: old, X: 2, Y: 2}}
	})
	both := add("Both", func(c *character.Character) {
		c.Memos = []character.Memo{{Map: old, X: 2, Y: 2}, {Map: "gen:town:t1", X: 5, Y: 5}}
	})
	moves := []store.Move{{From: old, To: "gen:town:t1", X: 40, Y: 41}}
	if err := store.CommitWorldChange(ctx, game, store.WorldChange{Dungeon: "crypt", Date: "2026-10-23", Moves: moves}); err != nil {
		t.Fatal(err)
	}
	load := func(id int64) *character.Character {
		c, _, err := s.Load(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	if c := load(inside); c.Map != "gen:town:t1" || c.X != 40 || c.Y != 41 {
		t.Errorf("inside: %s %d,%d", c.Map, c.X, c.Y)
	}
	if c := load(saved); c.SaveMap != "gen:town:t1" || c.SaveX != 40 || c.SaveY != 41 ||
		!slices.Equal(c.Memos, []character.Memo{{Map: "gen:field:0:0", X: 1, Y: 1}, {Map: "gen:town:t1", X: 40, Y: 41}}) {
		t.Errorf("saver: %s %d,%d %v", c.SaveMap, c.SaveX, c.SaveY, c.Memos)
	}
	if c := load(both); !slices.Equal(c.Memos, []character.Memo{{Map: "gen:town:t1", X: 5, Y: 5}}) {
		t.Errorf("both: %v", c.Memos)
	}
	// Seasons and a region's change.
	if err := store.StartSeasons(ctx, game, "2026-10-01"); err != nil {
		t.Fatal(err)
	}
	if err := store.StartSeasons(ctx, game, "2026-10-09"); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitWorldChange(ctx, game, store.WorldChange{Region: "t1", Season: 1, SeasonEnds: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.CommitWorldChange(ctx, game, store.WorldChange{Region: "t1", Season: 2}); err != nil {
		t.Fatal(err)
	}
	got, _ = store.LoadWorldChanges(ctx, game)
	if got.Rotations[0].ActiveDate != "2026-10-23" || got.SeasonsStarted != "2026-10-01" || got.SeasonEnds != 1 || got.Regions["t1"] != 2 {
		t.Errorf("after the changes %+v", got)
	}
	// A failed part leaves the whole change out.
	if err := store.CommitWorldChange(ctx, game, store.WorldChange{Dungeon: "nope", Date: "2026-10-30",
		Moves: []store.Move{{From: "gen:town:t1", To: "gen:field:0:0", X: 1, Y: 1}}}); err == nil {
		t.Error("a change of no dungeon")
	}
	if c := load(inside); c.Map != "gen:town:t1" {
		t.Errorf("a failed change moved %s", c.Map)
	}
	// The game role changes nothing else.
	for _, q := range []string{
		"UPDATE world_rotations SET key = '\\x00'",
		"UPDATE world_rotations SET every = 'daily', weekday = ''",
		"UPDATE world SET season_key = NULL",
		"SELECT seed FROM world",
		"DELETE FROM world_regions",
	} {
		if _, err := game.Exec(ctx, q); !isCode(err, "42501") {
			t.Errorf("%s: %v", q, err)
		}
	}
}
