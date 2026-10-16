package store_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/server/store"
)

// TestWorldSpawns: wactl world new writes the spawn lists with the
// layout, the game role reads them in order but cannot change them or
// read the seed, and a bad line fails the whole world (decision 0042).
func TestWorldSpawns(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	if got, err := store.LoadSpawns(ctx, game); err != nil || len(got) != 0 {
		t.Fatalf("before: %v %v", got, err)
	}
	planned := []spawns.Planned{
		{Map: "gen:field:0:0", Spawns: []spawns.Spawn{
			{Monster: "PORING", Count: 40, RespawnMs: 5000, Role: spawns.Common},
			{Monster: "example.wolf-pack:ALPHA_WOLF", Count: 3, RespawnMs: 30000, Role: spawns.Uncommon,
				Avoid: []spawns.Avoid{{X: 10, Y: 20, R: 15}, {X: 290, Y: 150, R: 15}}},
			{Monster: "RED_PLANT", Count: 5, RespawnMs: 600000, Role: spawns.Plant, Area: spawns.Area{X: 100, Y: 120, W: 8, H: 8}},
			{Monster: "ECLIPSE", Count: 1, RespawnMs: 3600000, Role: spawns.MiniBoss, Avoid: []spawns.Avoid{{X: 0, Y: 150, R: 30}}},
		}},
		{Map: "gen:dungeon:crypt:1", Spawns: []spawns.Spawn{{Monster: "ZOMBIE", Count: 20, RespawnMs: 5000, Role: spawns.Common}}},
	}
	// A count the table refuses fails the world: nothing is written.
	bad := []spawns.Planned{{Map: "gen:field:0:0", Spawns: []spawns.Spawn{{Monster: "PORING", Count: 0, RespawnMs: 5000, Role: spawns.Common}}}}
	w := store.World{Name: "P", Ruleset: "renewal", Seed: bytes.Repeat([]byte{3}, 32), Mode: "procedural", GeneratorVersion: 3,
		RNGVersion: "rng-v1", PublicNode: bytes.Repeat([]byte{4}, 32), Layout: testLayout(), Spawns: bad}
	if _, err := store.NewWorld(ctx, admin, w, store.Admin{Name: "Admin1", Hash: "x"}); err == nil {
		t.Fatal("a count of 0 was written")
	}
	if _, err := store.LoadWorld(ctx, game); err == nil {
		t.Fatal("a failed world was kept")
	}
	w.Spawns = planned
	if _, err := store.NewWorld(ctx, admin, w, store.Admin{Name: "Admin1", Hash: "x"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadSpawns(ctx, game)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !reflect.DeepEqual(got["gen:field:0:0"], planned[0].Spawns) || !reflect.DeepEqual(got["gen:dungeon:crypt:1"], planned[1].Spawns) {
		t.Errorf("loaded %+v\nwant %+v", got, planned)
	}
	for _, q := range []string{
		"UPDATE world_spawns SET count = 99",
		"INSERT INTO world_spawns (map_id, ord, monster, count, respawn_ms, role) VALUES ('gen:field:0:0', 9, 'PORING', 1, 5000, 'common')",
		"DELETE FROM world_spawns",
		"SELECT seed FROM world",
	} {
		if _, err := game.Exec(ctx, q); !isCode(err, "42501") {
			t.Errorf("%s: %v", q, err)
		}
	}
}
