package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store/db"
)

// roleProblems lists what is wrong with the role of a pool. game is true
// for the game pool and false for the account pool. A pool must connect
// as the low-rights role of deploy/postgres/roles.sql that it is meant
// for (store query DatabaseRole):
//   - The session's role and the current role are not superusers.
//   - The current role cannot make roles or databases, skip row security,
//     or replicate.
//   - The current role owns no object in the public schema, so it is not
//     the migrator.
//   - The game pool cannot read account_passwords, so it is not the
//     account role.
func roleProblems(ctx context.Context, pool *pgxpool.Pool, game bool) (string, []string, error) {
	r, err := db.New(pool).DatabaseRole(ctx)
	if err != nil {
		return "", nil, errors.Join(errors.New("reading the database role"), err)
	}
	var problems []string
	add := func(bad bool, what string) {
		if bad {
			problems = append(problems, what)
		}
	}
	add(r.Super, "is a superuser")
	add(r.CreateRole, "may make roles")
	add(r.CreateDb, "may make databases")
	add(r.BypassRls, "bypasses row security")
	add(r.Replication, "may replicate")
	add(r.Owns, "owns tables, as the migrator does")
	add(game && r.ReadsPasswords, "can read account_passwords, as the account role does")
	return r.Name, problems, nil
}

// checkDatabaseRole refuses a pool whose role has more rights than the
// role it is meant for. With dev it only logs a warning.
func checkDatabaseRole(ctx context.Context, pool *pgxpool.Pool, game, dev bool, log *slog.Logger) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	name, problems, err := roleProblems(ctx, pool, game)
	if err != nil || len(problems) == 0 {
		return err
	}
	which, want := "account database URL", "wathena_account"
	if game {
		which, want = "game database URL", "wathena_game"
	}
	msg := "the " + which + " connects as " + name + ", which " + strings.Join(problems, ", ") +
		"; use the " + want + " role of deploy/postgres/roles.sql"
	if dev {
		log.Warn(msg)
		return nil
	}
	return errors.New(msg)
}
