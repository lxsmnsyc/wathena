// Command testdb makes and drops throwaway PostgreSQL databases for the
// browser checks (client/scripts/exit-check.ts), as storetest does for Go
// tests. It reads the server from WATHENA_TEST_DATABASE_URL, a role that
// can create databases and roles.
//
//	testdb create   makes a database and the service roles, and prints
//	                the database's name
//	testdb login ROLE
//	                lets a service role log in with a new random
//	                password, and prints the password
//	testdb drop NAME
//	                drops a database that create made
//
// The service roles of deploy/postgres/roles.sql are made without LOGIN,
// as storetest makes them. A --dev waserver may act as them with the URL
// option "-c role=wathena_game". A waserver without --dev refuses a
// superuser that acts as a role (review 2026-10), so it logs in as the
// role itself, with the password of testdb login.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// prefix starts the name of every database testdb makes, so drop never
// drops another.
const prefix = "wathena_check_"

// env names the server, as for the Go tests (server/store/storetest).
const env = "WATHENA_TEST_DATABASE_URL"

// roles are the service roles that waserver acts as.
var roles = []string{"wathena_game", "wathena_account"}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "testdb:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	url := os.Getenv(env)
	if url == "" {
		return errors.New(env + " is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close(ctx) }()
	switch {
	case len(args) == 1 && args[0] == "create":
		b := make([]byte, 8)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		name := prefix + hex.EncodeToString(b)
		// DDL takes no bound parameters. The name is made here from
		// random hex, and quoted as an identifier.
		if _, err := conn.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			return err
		}
		for _, role := range roles {
			if _, err := conn.Exec(ctx, `DO $$ BEGIN
				CREATE ROLE `+pgx.Identifier{role}.Sanitize()+` NOLOGIN;
			EXCEPTION WHEN duplicate_object OR unique_violation THEN NULL;
			END $$`); err != nil {
				return err
			}
		}
		fmt.Println(name)
		return nil
	case len(args) == 2 && args[0] == "login" && slices.Contains(roles, args[1]):
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		password := hex.EncodeToString(b)
		// DDL takes no bound parameters. The role is one of roles, and
		// the password is random hex.
		if _, err := conn.Exec(ctx, "ALTER ROLE "+pgx.Identifier{args[1]}.Sanitize()+
			" LOGIN PASSWORD '"+password+"'"); err != nil {
			return err
		}
		fmt.Println(password)
		return nil
	case len(args) == 2 && args[0] == "drop" && strings.HasPrefix(args[1], prefix):
		_, err := conn.Exec(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{args[1]}.Sanitize()+" WITH (FORCE)")
		return err
	default:
		return errors.New("usage: testdb create, testdb login wathena_game|wathena_account, or testdb drop " + prefix + "<hex>")
	}
}
