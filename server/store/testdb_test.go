package store_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store"
)

// testDatabaseEnv names a PostgreSQL 18 URL for integration tests. The
// role must be able to create databases and roles, such as the postgres
// superuser of a throwaway container. Without it, the tests are skipped.
const testDatabaseEnv = "WATHENA_TEST_DATABASE_URL"

// emptyDatabase makes a new, empty database for one test and drops it
// afterwards. It returns a pool with the admin role.
func emptyDatabase(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv(testDatabaseEnv)
	if url == "" {
		t.Skip(testDatabaseEnv + " is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(ctx) }()
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	name := "wathena_test_" + hex.EncodeToString(b)
	// DDL cannot take bound parameters. The name is made here from
	// random hex, and quoted as an identifier.
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		t.Fatal(err)
	}
	// The game role is shared by every test database, as roles belong to
	// the whole server.
	if _, err := admin.Exec(ctx, `DO $$ BEGIN
		CREATE ROLE wathena_game NOLOGIN;
	EXCEPTION WHEN duplicate_object OR unique_violation THEN NULL;
	END $$`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		c, err := pgx.Connect(ctx, url)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = c.Close(ctx) }()
		if _, err := c.Exec(ctx, "DROP DATABASE "+ident+" WITH (FORCE)"); err != nil {
			t.Error(err)
		}
	})
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.Database = name
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// migrated makes a test database with every migration applied, and
// returns the admin pool and a pool whose connections act as the game
// role, as waserver does.
func migrated(t *testing.T) (admin, game *pgxpool.Pool) {
	t.Helper()
	admin = emptyDatabase(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	m, err := store.NewMigrator(admin)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = m.Close() }()
	if _, err := m.Up(ctx); err != nil {
		t.Fatal(err)
	}
	cfg := admin.Config().Copy()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE wathena_game")
		return err
	}
	game, err = pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(game.Close)
	return admin, game
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
