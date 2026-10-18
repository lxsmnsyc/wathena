package store_test

import (
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
)

func utc(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// TestDropOldLogs: drop_old_log_partitions drops a month partition whose
// month ends at the cutoff, keeps the month that ends after it, deletes
// the older rows of the default partitions, and runs as the game role,
// which still cannot drop tables (decision 0049).
func TestDropOldLogs(t *testing.T) {
	admin, game := migrated(t)
	ctx := timeout(t)
	// March and April 2026 get partitions. 2025-01 and 2030-01 have none,
	// so their rows go to the default partitions.
	// ensure_log_partitions refuses a day far from now, so the test makes
	// them itself.
	for _, part := range []string{"item_log", "zeny_log"} {
		for _, m := range []struct{ name, from, to string }{
			{"2026_03", "2026-03-01", "2026-04-01"}, {"2026_04", "2026-04-01", "2026-05-01"},
		} {
			if _, err := admin.Exec(ctx, "CREATE TABLE "+part+"_"+m.name+" PARTITION OF "+part+
				" FOR VALUES FROM ('"+m.from+"T00:00:00Z') TO ('"+m.to+"T00:00:00Z')"); err != nil {
				t.Fatal(err)
			}
		}
	}
	insert := func(at string) {
		t.Helper()
		if _, err := admin.Exec(ctx, `INSERT INTO item_log (at, action, from_kind, from_id, to_kind, to_id, serial, item_id, amount)
			VALUES ($1, 'drop', 'inventory', 1, 'ground', 0, gen_random_uuid(), 501, 1)`, utc(at)); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `INSERT INTO zeny_log (at, action, from_kind, from_id, to_kind, to_id, amount)
			VALUES ($1, 'shop', 'inventory', 1, 'shop', 0, 10)`, utc(at)); err != nil {
			t.Fatal(err)
		}
	}
	for _, at := range []string{"2026-03-01T00:00:00Z", "2026-03-31T23:59:59Z", "2026-04-01T00:00:00Z", "2026-04-30T12:00:00Z",
		"2025-01-10T00:00:00Z", "2030-01-10T00:00:00Z"} {
		insert(at)
	}
	exists := func(table string) bool {
		var ok bool
		if err := admin.QueryRow(ctx, "SELECT to_regclass('public.' || $1) IS NOT NULL", table).Scan(&ok); err != nil {
			t.Fatal(err)
		}
		return ok
	}
	count := func(table string) int {
		var n int
		if err := admin.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	// One second before the end of March: every month ends after it.
	items, zeny, err := store.DropOldLogs(ctx, game, utc("2026-03-31T23:59:59Z"))
	if err != nil {
		t.Fatal(err)
	}
	// Only the default partitions' row of 2025 goes.
	if items.Partitions != 0 || items.Rows != 1 || zeny.Partitions != 0 || zeny.Rows != 1 {
		t.Errorf("before the end of March: items %+v zeny %+v", items, zeny)
	}
	if !exists("item_log_2026_03") || !exists("zeny_log_2026_03") || count("item_log_default") != 1 {
		t.Error("March went before its end")
	}
	// At the end of March: March goes whole, April stays.
	items, zeny, err = store.DropOldLogs(ctx, game, utc("2026-04-01T00:00:00Z"))
	if err != nil {
		t.Fatal(err)
	}
	if items.Partitions != 1 || items.Rows != 2 || zeny.Partitions != 1 || zeny.Rows != 2 {
		t.Errorf("at the end of March: items %+v zeny %+v", items, zeny)
	}
	if exists("item_log_2026_03") || exists("zeny_log_2026_03") || !exists("item_log_2026_04") || !exists("zeny_log_2026_04") {
		t.Error("the partitions after the drop are wrong")
	}
	// The row of 2030 stays in the default partition, and April's rows
	// stay.
	if n := count("item_log"); n != 3 {
		t.Errorf("%d item log rows left, want April's 2 and 2030's", n)
	}
	if n := count("zeny_log_default"); n != 1 {
		t.Errorf("%d zeny rows in the default partition, want 2030's", n)
	}
	// Late in April, April still stays: its month ends after the cutoff.
	if items, _, err = store.DropOldLogs(ctx, game, utc("2026-04-30T23:59:59Z")); err != nil || items.Partitions != 0 || !exists("item_log_2026_04") {
		t.Errorf("late April: %+v %v", items, err)
	}

	// The game role cannot drop a partition itself, or delete rows.
	for _, q := range []string{"DROP TABLE item_log_2026_04", "DELETE FROM item_log", "DROP TABLE zeny_log_default"} {
		if _, err := game.Exec(ctx, q); err == nil {
			t.Errorf("the game role ran %q", q)
		}
	}
	// A cutoff less than 30 days ago is refused (migration 00037). 30
	// days ago works.
	for _, ago := range []time.Duration{time.Hour, 29 * 24 * time.Hour, 30*24*time.Hour - time.Minute} {
		if _, _, err := store.DropOldLogs(ctx, game, time.Now().Add(-ago)); err == nil || !strings.Contains(err.Error(), "less than 30 days ago") {
			t.Errorf("a cutoff %v ago: %v", ago, err)
		}
	}
	// A second more, as the database's clock may run a little behind
	// the test's, such as in a VM.
	if _, _, err := store.DropOldLogs(ctx, game, time.Now().Add(-30*24*time.Hour-time.Second)); err != nil {
		t.Errorf("a cutoff 30 days ago: %v", err)
	}
	// Other roles cannot run it.
	account := storetest.As(t, admin, "wathena_account")
	if _, _, err := store.DropOldLogs(ctx, account, utc("2026-04-01T00:00:00Z")); err == nil {
		t.Error("the account role dropped log partitions")
	}
}

// TestDeleteOldAudit: old audit rows go with their reviews, except rows
// that need a review and have none or one less than 30 days old, and new
// rows stay. Only the game role runs it (decision 0049, review decision
// 51).
func TestDeleteOldAudit(t *testing.T) {
	admin, game := migrated(t)
	account := storetest.As(t, admin, "wathena_account")
	ctx := timeout(t)
	old := time.Now().Add(-800 * 24 * time.Hour)
	// review adds a review of a row, made at a time. A review always
	// gets the database's time, so the test moves it afterwards.
	review := func(id int64, at time.Time) {
		t.Helper()
		if _, err := admin.Exec(ctx, `INSERT INTO staff_audit_reviews (audit_id, reviewer_id, reviewer_name) VALUES ($1, 2, 'Owner')`, id); err != nil {
			t.Fatal(err)
		}
		if _, err := admin.Exec(ctx, `UPDATE staff_audit_reviews SET reviewed_at = $2 WHERE audit_id = $1`, id, at); err != nil {
			t.Fatal(err)
		}
	}
	add := func(name string, at time.Time, needsReview bool, reviewedAt time.Time) int64 {
		t.Helper()
		var id int64
		reason := ""
		if needsReview {
			reason = "restore"
		}
		if err := admin.QueryRow(ctx, `INSERT INTO staff_audit (at, account_id, account_name, role, action, source, reason, needs_review)
			VALUES ($1, 1, $2, 'admin', 'items.restore', 'admin', $3, $4) RETURNING id`, at, name, reason, needsReview).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if !reviewedAt.IsZero() {
			review(id, reviewedAt)
		}
		return id
	}
	add("OldPlain", old, false, time.Time{})
	add("OldReviewed", old, true, old.Add(24*time.Hour))
	pending := add("OldPending", old, true, time.Time{})
	add("OldFreshReview", old, true, time.Now())
	add("NewPending", time.Now(), true, time.Time{})
	add("NewReviewed", time.Now(), true, time.Now())

	n, err := store.DeleteOldAudit(ctx, game, time.Now().Add(-730*24*time.Hour))
	if err != nil || n != 2 {
		t.Fatalf("deleted %d, %v; want 2", n, err)
	}
	var names string
	if err := admin.QueryRow(ctx, "SELECT string_agg(account_name, ' ' ORDER BY id) FROM staff_audit").Scan(&names); err != nil {
		t.Fatal(err)
	}
	if names != "OldPending OldFreshReview NewPending NewReviewed" {
		t.Errorf("rows left: %s", names)
	}
	var reviews int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM staff_audit_reviews").Scan(&reviews); err != nil || reviews != 2 {
		t.Errorf("%d reviews left, %v; want OldFreshReview's and NewReviewed's", reviews, err)
	}
	// A review made now does not free the old pending row at once. Once
	// the review is 30 days old, the row goes.
	review(pending, time.Now())
	if n, err := store.DeleteOldAudit(ctx, game, time.Now().Add(-730*24*time.Hour)); err != nil || n != 0 {
		t.Errorf("after a new review: deleted %d, %v; want 0", n, err)
	}
	if _, err := admin.Exec(ctx, `UPDATE staff_audit_reviews SET reviewed_at = now() - interval '31 days' WHERE audit_id = $1`, pending); err != nil {
		t.Fatal(err)
	}
	if n, err := store.DeleteOldAudit(ctx, game, time.Now().Add(-730*24*time.Hour)); err != nil || n != 1 {
		t.Errorf("after the review aged: deleted %d, %v; want 1", n, err)
	}
	// The account role, which writes the reviews, can neither delete
	// rows itself nor run the function. The game role cannot delete rows
	// itself.
	if _, err := account.Exec(ctx, "DELETE FROM staff_audit"); err == nil {
		t.Error("the account role deleted audit rows")
	}
	if _, err := store.DeleteOldAudit(ctx, account, old); err == nil {
		t.Error("the account role ran the audit deletion")
	}
	if _, err := game.Exec(ctx, "DELETE FROM staff_audit"); err == nil {
		t.Error("the game role deleted audit rows")
	}
	// A cutoff less than 30 days ago is refused (migration 00037). 30
	// days ago works.
	for _, ago := range []time.Duration{0, 29 * 24 * time.Hour} {
		if _, err := store.DeleteOldAudit(ctx, game, time.Now().Add(-ago)); err == nil || !strings.Contains(err.Error(), "less than 30 days ago") {
			t.Errorf("a cutoff %v ago: %v", ago, err)
		}
	}
	// A second more, as the database's clock may run a little behind
	// the test's, such as in a VM.
	if _, err := store.DeleteOldAudit(ctx, game, time.Now().Add(-30*24*time.Hour-time.Second)); err != nil {
		t.Errorf("a cutoff 30 days ago: %v", err)
	}
}

// TestAuditReviewRules: the database refuses a review of a row that is
// not marked for review and a review by the row's own actor, and gives
// every review its own time, so the account role cannot forge old
// reviews to free rows for deletion (review decision 51).
func TestAuditReviewRules(t *testing.T) {
	admin, _ := migrated(t)
	account := storetest.As(t, admin, "wathena_account")
	ctx := timeout(t)
	row := func(needsReview bool) int64 {
		t.Helper()
		reason := ""
		if needsReview {
			reason = "restore"
		}
		var id int64
		if err := admin.QueryRow(ctx, `INSERT INTO staff_audit (at, account_id, account_name, role, action, source, reason, needs_review)
			VALUES (now() - interval '800 days', 7, 'Rogue', 'admin', 'items.restore', 'admin', $1, $2) RETURNING id`, reason, needsReview).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	plain, marked := row(false), row(true)
	for _, c := range []struct {
		name string
		sql  string
		args []any
	}{
		{"a row not marked for review", `INSERT INTO staff_audit_reviews (audit_id, reviewer_id, reviewer_name) VALUES ($1, 8, 'Other')`, []any{plain}},
		{"a review of one's own action", `INSERT INTO staff_audit_reviews (audit_id, reviewer_id, reviewer_name) VALUES ($1, 7, 'Rogue')`, []any{marked}},
	} {
		if _, err := account.Exec(ctx, c.sql, c.args...); err == nil {
			t.Errorf("%s was accepted", c.name)
		}
	}
	// A review with a time in the past gets the database's time.
	if _, err := account.Exec(ctx, `INSERT INTO staff_audit_reviews (audit_id, reviewer_id, reviewer_name, reviewed_at)
		VALUES ($1, 8, 'Other', now() - interval '400 days')`, marked); err != nil {
		t.Fatal(err)
	}
	var fresh bool
	if err := admin.QueryRow(ctx, "SELECT reviewed_at > now() - interval '1 minute' FROM staff_audit_reviews WHERE audit_id = $1", marked).Scan(&fresh); err != nil || !fresh {
		t.Errorf("the review kept a forged time: %v %v", fresh, err)
	}
}

// TestDeleteOldLogins: login records older than the cutoff go.
func TestDeleteOldLogins(t *testing.T) {
	admin, _ := migrated(t)
	account := storetest.As(t, admin, "wathena_account")
	ctx := timeout(t)
	var acc int64
	if err := admin.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Logger') RETURNING id").Scan(&acc); err != nil {
		t.Fatal(err)
	}
	for _, days := range []int{100, 91, 89, 1} {
		if _, err := admin.Exec(ctx, "INSERT INTO account_logins (account_id, at, ip) VALUES ($1, now() - make_interval(days => $2), '192.0.2.1')", acc, days); err != nil {
			t.Fatal(err)
		}
	}
	n, err := store.DeleteOldLogins(ctx, account, time.Now().Add(-90*24*time.Hour))
	if err != nil || n != 2 {
		t.Errorf("deleted %d, %v; want 2", n, err)
	}
}

// TestRetentionLock: one session holds the lock at a time.
func TestRetentionLock(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	unlock, ok, err := store.LockRetention(ctx, game)
	if err != nil || !ok {
		t.Fatalf("first lock: %v %v", ok, err)
	}
	if _, ok, err := store.LockRetention(ctx, game); err != nil || ok {
		t.Fatalf("second lock while the first holds: %v %v", ok, err)
	}
	unlock()
	unlock2, ok, err := store.LockRetention(ctx, game)
	if err != nil || !ok {
		t.Fatalf("lock after the unlock: %v %v", ok, err)
	}
	unlock2()
}
