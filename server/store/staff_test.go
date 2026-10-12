package store_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
)

// TestStaffAuditRights checks the rights of decision 0028: the game and
// account roles add audit rows, and neither can change or delete one.
// Only the account role reads them. The game role reads the given
// roles and cannot change them.
func TestStaffAuditRights(t *testing.T) {
	admin, game := migrated(t)
	acct := storetest.As(t, admin, "wathena_account")
	ctx := context.Background()
	var id int64
	if err := admin.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Staffer') RETURNING id").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, "INSERT INTO staff_roles (account_id, role) VALUES ($1, 'moderator')", id); err != nil {
		t.Fatal(err)
	}
	row := store.AuditRow{AccountID: id, AccountName: "Staffer", Role: "game_master", Action: "item.give", Source: "game",
		Target: "Hero", Arguments: map[string]any{"item": 501, "amount": 5}, Reason: "event prize", NeedsReview: true}
	for _, pool := range []*pgxpool.Pool{game, acct} {
		if err := store.WriteStaffAudit(ctx, pool, row); err != nil {
			t.Fatalf("adding a row: %v", err)
		}
	}
	var n int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM staff_audit WHERE needs_review AND arguments->>'item' = '501'").Scan(&n); err != nil || n != 2 {
		t.Fatalf("%d rows, %v", n, err)
	}
	denied := func(pool *pgxpool.Pool, role, sql string) {
		t.Helper()
		_, err := pool.Exec(ctx, sql)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Errorf("%s: %q gave %v, want permission denied", role, sql, err)
		}
	}
	for _, sql := range []string{
		"UPDATE staff_audit SET reason = 'changed'",
		"UPDATE staff_audit SET needs_review = false",
		"DELETE FROM staff_audit",
		"TRUNCATE staff_audit",
		"UPDATE staff_audit_reviews SET note = 'x'",
		"DELETE FROM staff_audit_reviews",
	} {
		denied(game, "game", sql)
		denied(acct, "account", sql)
	}
	for _, sql := range []string{
		"SELECT * FROM staff_audit",
		"SELECT * FROM staff_audit_reviews",
		"INSERT INTO staff_audit_reviews (audit_id, reviewer_id, reviewer_name) VALUES (1, 1, 'x')",
		"UPDATE staff_roles SET role = 'admin'",
		"INSERT INTO staff_roles (account_id, role) VALUES (1, 'admin')",
		"DELETE FROM staff_roles",
	} {
		denied(game, "game", sql)
	}
	roles, err := store.LoadStaffRoles(ctx, game)
	if err != nil || roles[id] != "moderator" || len(roles) != 1 {
		t.Errorf("roles %v, %v", roles, err)
	}
	// A row that needs review needs a reason, and the owner role is
	// never in staff_roles.
	row.Reason = ""
	if err := store.WriteStaffAudit(ctx, game, row); err == nil {
		t.Error("a row for review without a reason was added")
	}
	if _, err := acct.Exec(ctx, "INSERT INTO staff_roles (account_id, role) VALUES ($1, 'owner') ON CONFLICT (account_id) DO UPDATE SET role = 'owner'", id); err == nil {
		t.Error("the owner role was given in the database")
	}
}
