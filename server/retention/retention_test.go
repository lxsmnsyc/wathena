package retention_test

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/retention"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
)

func TestWarnings(t *testing.T) {
	if w := retention.Defaults.Warnings(); len(w) != 0 {
		t.Errorf("the defaults warn: %v", w)
	}
	p := retention.Defaults
	p.ChatDays, p.AuditDays, p.LoginIPDays = 31, 365, 90
	w := p.Warnings()
	if len(w) != 1 || !strings.Contains(w[0], "retention.chat_days is 31 days") || !strings.Contains(w[0], "default of 30 days") {
		t.Errorf("warnings %v", w)
	}
}

// TestJob: a run deletes each kind, keeps chat lines that an open
// report holds, skips while another session holds the lock, and
// records its counts for the status and the metrics.
func TestJob(t *testing.T) {
	admin := storetest.Migrated(t)
	game := storetest.As(t, admin, "wathena_game")
	account := storetest.As(t, admin, "wathena_account")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	var acc int64
	if err := admin.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ('Alice') RETURNING id").Scan(&acc); err != nil {
		t.Fatal(err)
	}
	exec("INSERT INTO account_logins (account_id, at) VALUES ($1, now() - interval '100 days'), ($1, now())", acc)
	// Three old lines: one held by an open report, one by a closed one,
	// one by none. And a new line.
	exec(`INSERT INTO chat_log (id, at, kind, name, map, text) VALUES
		('00000000-0000-0000-0000-000000000001', now() - interval '40 days', 'local', 'Bob', 'prontera', 'held'),
		('00000000-0000-0000-0000-000000000002', now() - interval '40 days', 'local', 'Bob', 'prontera', 'closed'),
		('00000000-0000-0000-0000-000000000003', now() - interval '40 days', 'local', 'Bob', 'prontera', 'old'),
		('00000000-0000-0000-0000-000000000004', now(), 'local', 'Bob', 'prontera', 'new')`)
	exec(`INSERT INTO reports (created_at, reporter_name, target_name, map) VALUES (now() - interval '40 days', 'Carol', 'Bob', 'prontera')`)
	exec(`INSERT INTO reports (created_at, reporter_name, target_name, map, status, closed_at)
		VALUES (now() - interval '40 days', 'Carol', 'Bob', 'prontera', 'closed', now())`)
	exec(`INSERT INTO report_lines (report_id, line_id)
		SELECT id, CASE status WHEN 'open' THEN '00000000-0000-0000-0000-000000000001'::uuid ELSE '00000000-0000-0000-0000-000000000002'::uuid END FROM reports`)
	// A zeny row in the default partition, from before every partition.
	exec(`INSERT INTO zeny_log (at, action, from_kind, from_id, to_kind, to_id, amount) VALUES ('2020-01-10T00:00:00Z', 'shop', 'inventory', 1, 'shop', 0, 10)`)
	exec(`INSERT INTO staff_audit (at, account_id, account_name, role, action, source) VALUES (now() - interval '800 days', 1, 'Owner', 'owner', 'staff.give', 'admin')`)

	j := &retention.Job{Game: game, Account: account, Periods: retention.Defaults, Log: slog.New(slog.DiscardHandler)}

	// While another session holds the lock, a run does nothing.
	unlock, ok, err := store.LockRetention(ctx, game)
	if err != nil || !ok {
		t.Fatalf("lock: %v %v", ok, err)
	}
	res, err := j.RunOnce(ctx)
	if err != nil || !res.Skipped || len(res.Deleted) != 0 {
		t.Fatalf("a run while the lock is held: %+v %v", res, err)
	}
	// A skip is not a run: there is no last run yet.
	if s, err := j.Status(ctx); err != nil || !s.Skipped || !s.LastRun.IsZero() || !s.LastSuccess.IsZero() {
		t.Errorf("status after a skip: %+v %v", s, err)
	}
	unlock()

	res, err = j.RunOnce(ctx)
	if err != nil || res.Skipped {
		t.Fatalf("run: %+v %v", res, err)
	}
	want := map[string]int64{retention.KindLoginIPs: 1, retention.KindChat: 2, retention.KindItemLog: 0, retention.KindZenyLog: 1, retention.KindAudit: 1}
	for k, n := range want {
		if res.Deleted[k] != n {
			t.Errorf("%s: deleted %d, want %d", k, res.Deleted[k], n)
		}
	}
	var texts string
	if err := admin.QueryRow(ctx, "SELECT string_agg(text, ' ' ORDER BY id) FROM chat_log").Scan(&texts); err != nil {
		t.Fatal(err)
	}
	if texts != "held new" {
		t.Errorf("chat lines left: %q, want the open report's and the new one", texts)
	}
	s, err := j.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if s.Skipped || s.LastSuccess.IsZero() || s.LastError != "" || s.Total[retention.KindChat] != 2 || s.Last[retention.KindAudit] != 1 {
		t.Errorf("status %+v", s)
	}
	ran := s.LastRun

	// Another process, or this one after a restart, shows the run from
	// retention_status, with no counts of its own.
	other := &retention.Job{Game: game, Account: account, Periods: retention.Defaults, Log: slog.New(slog.DiscardHandler)}
	if o, err := other.Status(ctx); err != nil || !o.LastRun.Equal(ran) || !o.LastSuccess.Equal(ran) || o.Last[retention.KindChat] != 2 ||
		o.Last[retention.KindAudit] != 1 || o.Total[retention.KindChat] != 0 || o.Skipped {
		t.Errorf("status in another process: %+v %v", o, err)
	}
	// A run that skips records nothing.
	unlock, ok, err = store.LockRetention(ctx, game)
	if err != nil || !ok {
		t.Fatalf("lock: %v %v", ok, err)
	}
	if res, err := other.RunOnce(ctx); err != nil || !res.Skipped {
		t.Fatalf("a skip in another process: %+v %v", res, err)
	}
	unlock()
	if o, err := other.Status(ctx); err != nil || !o.Skipped || !o.LastRun.Equal(ran) || o.Last[retention.KindChat] != 2 {
		t.Errorf("status after a skip in another process: %+v %v", o, err)
	}

	// A second run deletes nothing more, and the totals stay.
	if res, err := j.RunOnce(ctx); err != nil || res.Deleted[retention.KindChat] != 0 {
		t.Errorf("second run: %+v %v", res, err)
	}
	var b strings.Builder
	if err := j.WritePrometheus(ctx, &b); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		`wathena_retention_deleted_rows_total{kind="chat"} 2`,
		`wathena_retention_deleted_rows_total{kind="login_ips"} 1`,
		`# TYPE wathena_retention_last_success_timestamp_seconds gauge`,
	} {
		if !slices.Contains(strings.Split(b.String(), "\n"), line) {
			t.Errorf("metrics lack %q:\n%s", line, b.String())
		}
	}

	// The game role only reads and writes the row, and the account role
	// has no rights on it.
	if _, err := game.Exec(ctx, "DELETE FROM retention_status"); err == nil {
		t.Error("the game role deleted the retention status")
	}
	if _, err := account.Exec(ctx, "SELECT * FROM retention_status"); err == nil {
		t.Error("the account role read the retention status")
	}

	// A run with errors keeps the last success, and records the error:
	// here the item and zeny logs with 29 days, which the function
	// refuses.
	p := retention.Defaults
	p.ItemLogDays = 29
	short := &retention.Job{Game: game, Account: account, Periods: p, Log: slog.New(slog.DiscardHandler)}
	if _, err := short.RunOnce(ctx); err == nil || !strings.Contains(err.Error(), "less than 30 days ago") {
		t.Errorf("a run with 29 days of logs: %v", err)
	}
	if o, err := other.Status(ctx); err != nil || !strings.Contains(o.LastError, "less than 30 days ago") || o.LastRun.Equal(o.LastSuccess) ||
		o.LastSuccess.IsZero() {
		t.Errorf("status after a failed run: %+v %v", o, err)
	}

	// Without the account pool, the login records are left. The game
	// role deletes old audit rows, so they still go (review decision
	// 51).
	dev := &retention.Job{Game: game, Periods: retention.Defaults, Log: slog.New(slog.DiscardHandler)}
	if res, err := dev.RunOnce(ctx); err != nil {
		t.Errorf("dev run: %v", err)
	} else if _, ok := res.Deleted[retention.KindLoginIPs]; ok {
		t.Errorf("a run without the account pool touched the login records: %+v", res)
	} else if _, ok := res.Deleted[retention.KindAudit]; !ok {
		t.Errorf("a run without the account pool skipped the audit log: %+v", res)
	}
}
