package store_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/lxsmnsyc/wathena/server/content"
	"github.com/lxsmnsyc/wathena/server/store"
)

// TestWorldContent: wactl world new writes the places, objects and
// quests with the layout, the game role reads them in order but cannot
// change them, and a quest the table refuses fails the whole world
// (decision 0044).
func TestWorldContent(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	if got, err := store.LoadContent(ctx, game); err != nil || len(got.Quests)+len(got.Places)+len(got.Objects) != 0 {
		t.Fatalf("before: %+v %v", got, err)
	}
	want := &content.World{
		Places: []content.Place{
			{Map: "gen:town:t1", X: 30, Y: 31, Tags: []string{"quest_board"}, Source: "core"},
			{Map: "gen:town:t1", X: 40, Y: 41, Tags: []string{"guild:swordman"}, Source: "core"},
			{Map: "gen:field:0:0", X: 5, Y: 6, Tags: []string{"shrine", "shrine:moss"}, Source: "example.shrines"},
		},
		Objects: []content.Object{{Map: "gen:field:0:0", Model: "rock", X: 7, Y: 8, Turn: 3, Source: "example.shrines"}},
		Quests: []content.Quest{
			{ID: 900000, Town: "gen:town:t1", Purpose: "board", Template: "hunt", Kind: "hunt", Title: "Hunt 10 Poring", Monster: "PORING",
				Count: 10, BaseExp: 50, JobExp: 37, Zeny: 50},
			{ID: 900001, Town: "gen:town:t1", Purpose: "board", Template: "collect", Kind: "collect", Title: "Bring 5 Jellopy", Monster: "PORING",
				Item: "Jellopy", Count: 5, DropRate: 3000, BaseExp: 60, JobExp: 45, Zeny: 60},
			{ID: 900002, Town: "gen:town:t1", Purpose: "board", Template: "delivery", Kind: "delivery", Title: "Deliver a parcel to Willow Village",
				Item: "Delivery_Box", To: "gen:home:village", BaseExp: 40, JobExp: 30, Zeny: 40},
			{ID: 900003, Town: "gen:town:t1", Purpose: "guild", Guild: "swordman", Stage: "first", Step: 1, Template: "hunt", Kind: "hunt",
				Title: "Hunt 12 Fabre", Monster: "FABRE", Count: 12},
		},
	}
	w := store.World{Name: "P", Ruleset: "renewal", Seed: bytes.Repeat([]byte{3}, 32), Mode: "procedural", GeneratorVersion: 3,
		RNGVersion: "rng-v1", PublicNode: bytes.Repeat([]byte{4}, 32), Layout: testLayout(),
		Content: &content.World{Quests: []content.Quest{{ID: 900000, Town: "gen:town:t1", Purpose: "guild", Template: "hunt", Kind: "hunt", Title: "x", Monster: "PORING", Count: 1}}}}
	// A guild quest without its guild fails the world: nothing is written.
	if _, err := store.NewWorld(ctx, admin, w, store.Admin{Name: "Admin1", Hash: "x"}); err == nil {
		t.Fatal("a guild quest without a guild was written")
	}
	w.Content = want
	if _, err := store.NewWorld(ctx, admin, w, store.Admin{Name: "Admin1", Hash: "x"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadContent(ctx, game)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Quests, want.Quests) || !reflect.DeepEqual(got.Objects, want.Objects) {
		t.Errorf("loaded %+v\nwant %+v", got, want)
	}
	// Places come by map, then in their order on the map.
	if len(got.Places) != 3 || !reflect.DeepEqual(got.Places[0], want.Places[2]) || !reflect.DeepEqual(got.Places[1:], want.Places[:2]) {
		t.Errorf("places %+v", got.Places)
	}
	for _, q := range []string{
		"UPDATE world_quests SET zeny = 99",
		"DELETE FROM world_places",
		"INSERT INTO world_objects (map_id, ord, model, x, y, source) VALUES ('gen:town:t1', 0, 'rock', 1, 1, 'core')",
	} {
		if _, err := game.Exec(ctx, q); !isCode(err, "42501") {
			t.Errorf("%s: %v", q, err)
		}
	}
}
