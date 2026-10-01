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
	if err != nil || got.Name != "Test" || got.Ruleset != "renewal" || got.Seed != nil {
		t.Errorf("load: %+v, %v", got, err)
	}
	var staff bool
	if err := admin.QueryRow(ctx, "SELECT staff FROM accounts WHERE id = $1", id).Scan(&staff); err != nil || !staff {
		t.Errorf("staff: %v, %v", staff, err)
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
	// a staff account or a world.
	if _, err := game.Exec(ctx, "SELECT seed FROM world"); !isCode(err, "42501") {
		t.Errorf("game reads the seed: %v", err)
	}
	for role, pool := range map[string]*pgxpool.Pool{"game": game, "account": account} {
		if _, err := pool.Exec(ctx, "INSERT INTO accounts (name, staff) VALUES ('Sneaky1', true)"); !isCode(err, "42501") {
			t.Errorf("%s makes a staff account: %v", role, err)
		}
		if _, err := pool.Exec(ctx, "UPDATE accounts SET staff = true"); !isCode(err, "42501") {
			t.Errorf("%s makes staff: %v", role, err)
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
