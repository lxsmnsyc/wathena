package store_test

import (
	"context"
	"slices"
	"strings"
	"testing"
)

// TestDefinerSearchPath: every SECURITY DEFINER function searches
// pg_temp last, so a temporary table never shadows a public one
// (review finding L15).
func TestDefinerSearchPath(t *testing.T) {
	admin, _ := migrated(t)
	ctx := timeout(t)
	rows, err := admin.Query(ctx, `SELECT p.proname, coalesce(array_to_string(p.proconfig, ';'), '')
		FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE p.prosecdef AND n.nspname = 'public'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var name, config string
		if err := rows.Scan(&name, &config); err != nil {
			t.Fatal(err)
		}
		n++
		if !slices.Contains(strings.Split(config, ";"), "search_path=pg_catalog, public, pg_temp") {
			t.Errorf("%s: settings %q do not end the search path with pg_temp", name, config)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("no SECURITY DEFINER functions found")
	}
}

// TestDefinerIgnoresTempShadow: a temporary account_deletions table made
// by the game role does not make delete_account delete an account that
// asked for nothing (review finding L15). The test databases keep the
// default TEMP right of PUBLIC.
func TestDefinerIgnoresTempShadow(t *testing.T) {
	admin, _ := migrated(t)
	ctx := timeout(t)
	victim, _ := seedAccount(t, ctx, admin, "Shadow1", "Shadow One")
	tx, err := admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(ctx, "SET LOCAL ROLE wathena_game"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `CREATE TEMP TABLE account_deletions (account_id bigint, requested_at timestamptz, delete_after timestamptz)`); err != nil {
		t.Skipf("the game role cannot make temporary tables here: %v", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO account_deletions VALUES ($1, now() - interval '30 days', now() - interval '1 day')`, victim); err != nil {
		t.Fatal(err)
	}
	var ok bool
	if err := tx.QueryRow(ctx, "SELECT delete_account($1, now(), '{}')", victim).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("delete_account read the temporary table and deleted the account")
	}
	var left int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM characters WHERE account_id = $1", victim).Scan(&left); err != nil || left != 1 {
		t.Errorf("%d characters left, want 1: %v", left, err)
	}
}

// TestLogPartitionsBounded: the game role cannot make the log partitions
// of a month far from now (review finding L16). Days within 45 days
// still work.
func TestLogPartitionsBounded(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	for _, c := range []struct {
		day string
		ok  bool
	}{
		{"now()", true},
		{"now() + interval '40 days'", true},
		{"now() - interval '40 days'", true},
		{"now() + interval '50 days'", false},
		{"now() - interval '50 days'", false},
		{"'2100-01-01T00:00:00Z'::timestamptz", false},
		{"'1990-01-01T00:00:00Z'::timestamptz", false},
		{"NULL", false},
	} {
		_, err := game.Exec(ctx, "SELECT ensure_log_partitions("+c.day+")")
		if (err == nil) != c.ok {
			t.Errorf("ensure_log_partitions(%s): %v", c.day, err)
		}
	}
}
