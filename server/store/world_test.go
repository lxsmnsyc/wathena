package store_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
)

func TestNewWorld(t *testing.T) {
	admin, game := migrated(t)
	account := storetest.As(t, admin, "wathena_account")
	ctx := timeout(t)

	if _, err := store.LoadWorld(ctx, game); !errors.Is(err, store.ErrNoWorld) {
		t.Fatalf("before: %v, want ErrNoWorld", err)
	}
	seed := bytes.Repeat([]byte{7}, 32)
	w := store.World{Name: "Test", Ruleset: "renewal", Seed: seed}
	id, err := store.NewWorld(ctx, admin, w, store.Admin{Name: "Admin1", Hash: "$argon2id$test"})
	if err != nil || id == 0 {
		t.Fatalf("new world: %d, %v", id, err)
	}
	got, err := store.LoadWorld(ctx, game)
	if err != nil || got.Name != "Test" || got.Ruleset != "renewal" || got.Seed != nil ||
		got.Mode != "classic" || got.GeneratorVersion != 0 || got.RNGVersion != "" {
		t.Errorf("load: %+v, %v", got, err)
	}
	var name string
	if err := admin.QueryRow(ctx, "SELECT name FROM accounts WHERE id = $1", id).Scan(&name); err != nil || name != "Admin1" {
		t.Errorf("account: %q, %v", name, err)
	}
	// accounts.staff is gone (decision 0036). Roles come from the config
	// and staff_roles.
	var cols int
	if err := admin.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'accounts' AND column_name = 'staff'`).Scan(&cols); err != nil || cols != 0 {
		t.Errorf("accounts.staff columns: %d, %v", cols, err)
	}

	// A second world, or a taken name, changes nothing.
	if _, err := store.NewWorld(ctx, admin, w, store.Admin{Name: "Admin2", Hash: "x"}); !errors.Is(err, store.ErrWorldExists) {
		t.Errorf("second world: %v", err)
	}
	var n int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM accounts").Scan(&n); err != nil || n != 1 {
		t.Errorf("accounts after a second world: %d, %v", n, err)
	}

	// The game role cannot read the seed. Neither service role can make
	// a world.
	if _, err := game.Exec(ctx, "SELECT seed FROM world"); !isCode(err, "42501") {
		t.Errorf("game reads the seed: %v", err)
	}
	for role, pool := range map[string]*pgxpool.Pool{"game": game, "account": account} {
		if _, err := pool.Exec(ctx, "INSERT INTO world (name, ruleset, seed) VALUES ('W', 'renewal', $1)", seed); !isCode(err, "42501") {
			t.Errorf("%s makes a world: %v", role, err)
		}
		if _, err := pool.Exec(ctx, "DELETE FROM world"); !isCode(err, "42501") {
			t.Errorf("%s deletes the world: %v", role, err)
		}
	}
	// They still make plain accounts.
	if _, err := account.Exec(ctx, "INSERT INTO accounts (name) VALUES ('Plain1')"); err != nil {
		t.Errorf("account role: %v", err)
	}
	if _, err := game.Exec(ctx, "INSERT INTO accounts (name, dev) VALUES ('dev one', true)"); err != nil {
		t.Errorf("game role: %v", err)
	}
}

// TestWorldMode: a procedural world records its generator and rng
// versions, and the table refuses a mode without them (decision 0038).
func TestWorldMode(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	seed := bytes.Repeat([]byte{9}, 32)
	for _, bad := range []store.World{
		{Name: "W", Ruleset: "renewal", Seed: seed, Mode: "procedural"},
		{Name: "W", Ruleset: "renewal", Seed: seed, Mode: "procedural", GeneratorVersion: 1},
		{Name: "W", Ruleset: "renewal", Seed: seed, Mode: "classic", GeneratorVersion: 1, RNGVersion: "rng-v1"},
		{Name: "W", Ruleset: "renewal", Seed: seed, Mode: "other"},
		{Name: "W", Ruleset: "renewal", Seed: seed, Mode: "procedural", GeneratorVersion: 1, RNGVersion: "rng-1"},
	} {
		if _, err := store.NewWorld(ctx, admin, bad, store.Admin{Name: "Admin1", Hash: "x"}); !isCode(err, "23514") {
			t.Errorf("%+v: %v, want a check violation", bad, err)
		}
	}
	w := store.World{Name: "Gen", Ruleset: "pre-renewal", Seed: seed, Mode: "procedural", GeneratorVersion: 1, RNGVersion: "rng-v1"}
	if _, err := store.NewWorld(ctx, admin, w, store.Admin{Name: "Admin1", Hash: "x"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.LoadWorld(ctx, game)
	if err != nil || got.Name != w.Name || got.Ruleset != w.Ruleset || got.Seed != nil ||
		got.Mode != w.Mode || got.GeneratorVersion != w.GeneratorVersion || got.RNGVersion != w.RNGVersion {
		t.Errorf("load: %+v, %v, want %+v", got, err, w)
	}
}
