package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/internal/secretenv"
	"github.com/lxsmnsyc/wathena/server/store"
)

// migrateEnv holds the migrator role's database URL when --database is
// not given. migrateEnv_FILE may name a file that holds it instead.
const migrateEnv = "WATHENA_MIGRATE_DATABASE_URL"

// migratorURL returns the migrator role's URL: --database when given,
// else migrateEnv or the file that migrateEnv_FILE names.
func migratorURL(flagValue string) (string, error) {
	if flagValue != "" {
		return flagValue, nil
	}
	url, err := secretenv.Get(migrateEnv)
	if err != nil {
		return "", err
	}
	if url == "" {
		return "", errors.New("give --database or set " + migrateEnv + " or " + migrateEnv + "_FILE")
	}
	return url, nil
}

// connect opens a pool on a database URL. Its errors never quote the
// URL, which holds a password.
func connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		// pgx's parse errors can quote the URL, password included.
		return nil, errors.New("the database URL is not valid")
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = "wactl"
	return pgxpool.NewWithConfig(ctx, cfg)
}

// commandContext stops on Ctrl+C or SIGTERM, and after limit.
func commandContext(limit time.Duration) (context.Context, context.CancelFunc) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	ctx, cancel := context.WithTimeout(ctx, limit)
	return ctx, func() { cancel(); stop() }
}

// migrate lists or applies the schema migrations. It needs a role that
// may change the schema, such as wathena_migrator from
// deploy/postgres/roles.sql, not the game role.
func migrate(args []string, up bool) error {
	name := "migrate status"
	if up {
		name = "migrate up"
	}
	fs := newFlags(name)
	database := fs.String("database", "", "PostgreSQL URL of the migrator role; prefer the "+migrateEnv+" environment variable")
	if err := fs.Parse(args); err != nil {
		return err
	}
	url, err := migratorURL(*database)
	if err != nil {
		return err
	}
	ctx, cancel := commandContext(5 * time.Minute)
	defer cancel()
	pool, err := connect(ctx, url)
	if err != nil {
		return err
	}
	defer pool.Close()
	if up {
		return migrateUp(ctx, pool)
	}
	m, err := store.NewMigrator(pool)
	if err != nil {
		return err
	}
	defer func() { _ = m.Close() }()
	list, err := m.Status(ctx)
	if err != nil {
		return err
	}
	for _, s := range list {
		state := "pending"
		if s.Applied {
			state = "applied"
		}
		fmt.Printf("%05d  %-8s %s\n", s.Version, state, s.Name)
	}
	return nil
}

// migrateUp applies pending migrations and prints them.
func migrateUp(ctx context.Context, pool *pgxpool.Pool) error {
	m, err := store.NewMigrator(pool)
	if err != nil {
		return err
	}
	defer func() { _ = m.Close() }()
	applied, err := m.Up(ctx)
	for _, v := range applied {
		fmt.Printf("applied %05d\n", v)
	}
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		fmt.Println("no pending migrations")
	}
	return nil
}
