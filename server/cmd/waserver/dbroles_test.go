package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store/storetest"
)

// loginAs makes a login role, a member of role unless it is "", and
// returns a pool that logs in as it. So the session's role is not the
// test's superuser.
func loginAs(t *testing.T, admin *pgxpool.Pool, role string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	name := "wathena_rolecheck_" + hex.EncodeToString(b)
	password := hex.EncodeToString(b)
	ident := pgx.Identifier{name}.Sanitize()
	// DDL takes no bound parameters. The name and password are random hex.
	sql := "CREATE ROLE " + ident + " LOGIN PASSWORD '" + password + "'"
	if role != "" {
		sql += " IN ROLE " + pgx.Identifier{role}.Sanitize()
	}
	if _, err := admin.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP ROLE "+ident); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(os.Getenv(storetest.Env))
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(name, password)
	u.Path = "/" + admin.Config().ConnConfig.Database
	pool, err := pgxpool.New(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestCheckDatabaseRole(t *testing.T) {
	admin := storetest.Migrated(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	game := loginAs(t, admin, "wathena_game")
	account := loginAs(t, admin, "wathena_account")

	for _, c := range []struct {
		name string
		pool *pgxpool.Pool
		game bool
		want string
	}{
		{"game role", game, true, ""},
		{"account role", account, false, ""},
		{"account role as game", account, true, "can read account_passwords"},
		{"superuser as game", admin, true, "is a superuser"},
		{"superuser as account", admin, false, "is a superuser"},
		// A superuser session that only sets the game role can reset it.
		{"superuser acting as game", storetest.As(t, admin, "wathena_game"), true, "is a superuser"},
	} {
		err := checkDatabaseRole(ctx, c.pool, c.game, false, log)
		if c.want == "" {
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want %q", c.name, err, c.want)
		}
		// --dev only warns.
		if err := checkDatabaseRole(ctx, c.pool, c.game, true, log); err != nil {
			t.Errorf("%s with dev: %v", c.name, err)
		}
	}

	// A role that owns a table is refused, as the migrator would be.
	owner := loginAs(t, admin, "")
	var name string
	if err := owner.QueryRow(ctx, "SELECT current_user::text").Scan(&name); err != nil {
		t.Fatal(err)
	}
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "ALTER TABLE accounts OWNER TO "+ident); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "REASSIGN OWNED BY "+ident+" TO CURRENT_USER") })
	if err := checkDatabaseRole(ctx, owner, false, false, log); err == nil || !strings.Contains(err.Error(), "owns tables") {
		t.Errorf("owner: got %v", err)
	}
}
