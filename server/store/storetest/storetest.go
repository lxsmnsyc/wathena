// Package storetest makes throwaway PostgreSQL databases for integration
// tests. It is only for tests.
//
// The tests need a PostgreSQL 18 server and a role that can create
// databases and roles, named by WATHENA_TEST_DATABASE_URL. Without it,
// they are skipped.
package storetest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store"
)

// Env names the PostgreSQL URL for integration tests.
const Env = "WATHENA_TEST_DATABASE_URL"

// Roles are the service roles of deploy/postgres/roles.sql. Tests make
// them without LOGIN and act as them with SET ROLE.
var Roles = []string{"wathena_game", "wathena_account"}

// Empty makes a new, empty database for one test and drops it
// afterwards. It returns a pool with the admin role.
func Empty(t testing.TB) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv(Env)
	if url == "" {
		t.Skip(Env + " is not set")
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
	// The roles are shared by every test database, as roles belong to
	// the whole server.
	for _, role := range Roles {
		if _, err := admin.Exec(ctx, `DO $$ BEGIN
			CREATE ROLE `+pgx.Identifier{role}.Sanitize()+` NOLOGIN;
		EXCEPTION WHEN duplicate_object OR unique_violation THEN NULL;
		END $$`); err != nil {
			t.Fatal(err)
		}
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

// Migrated makes a test database with every migration applied. It
// returns the admin pool.
func Migrated(t testing.TB) *pgxpool.Pool {
	t.Helper()
	admin := Empty(t)
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
	return admin
}

// As returns a pool on the same database whose connections act as a
// role, such as wathena_game.
func As(t testing.TB, admin *pgxpool.Pool, role string) *pgxpool.Pool {
	t.Helper()
	cfg := admin.Config().Copy()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE "+pgx.Identifier{role}.Sanitize())
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}
