package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store"
)

// migrateEnv holds the migrator role's database URL when --database is
// not given.
const migrateEnv = "WATHENA_MIGRATE_DATABASE_URL"

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
	url := *database
	if url == "" {
		url = os.Getenv(migrateEnv)
	}
	if url == "" {
		return errors.New("give --database or set " + migrateEnv)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		// pgx's parse errors can quote the URL, password included.
		return errors.New("the database URL is not valid")
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = "wactl"
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return err
	}
	defer pool.Close()
	m, err := store.NewMigrator(pool)
	if err != nil {
		return err
	}
	defer func() { _ = m.Close() }()
	if up {
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
