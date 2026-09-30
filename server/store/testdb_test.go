package store_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
)

// emptyDatabase makes a new, empty database for one test and drops it
// afterwards. It returns a pool with the admin role.
func emptyDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	return storetest.Empty(t)
}

// migrated makes a test database with every migration applied, and
// returns the admin pool and a pool whose connections act as the game
// role, as waserver does.
func migrated(t *testing.T) (admin, game *pgxpool.Pool) {
	t.Helper()
	admin = storetest.Migrated(t)
	return admin, storetest.As(t, admin, "wathena_game")
}

// newStore makes a store on the game role's pool.
func newStore(t *testing.T, game *pgxpool.Pool, opt store.Options) *store.Store {
	t.Helper()
	if opt.Log == nil {
		opt.Log = slog.New(slog.DiscardHandler)
	}
	s := store.New(game, opt)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return s
}
