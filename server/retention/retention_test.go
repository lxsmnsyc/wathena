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
	if s := j.Status(); !s.Skipped || s.LastRun.IsZero() || !s.LastSuccess.IsZero() {
		t.Errorf("status after a skip: %+v", s)
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
	s := j.Status()
	if s.Skipped || s.LastSuccess.IsZero() || s.LastError != "" || s.Total[retention.KindChat] != 2 || s.Last[retention.KindAudit] != 1 {
		t.Errorf("status %+v", s)
	}
	// A second run deletes nothing more, and the totals stay.
	if res, err := j.RunOnce(ctx); err != nil || res.Deleted[retention.KindChat] != 0 {
		t.Errorf("second run: %+v %v", res, err)
	}
	var b strings.Builder
	if err := j.WritePrometheus(&b); err != nil {
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

	// Without the account pool, the login records and the audit log are
	// left.
	dev := &retention.Job{Game: game, Periods: retention.Defaults, Log: slog.New(slog.DiscardHandler)}
	if res, err := dev.RunOnce(ctx); err != nil {
		t.Errorf("dev run: %v", err)
	} else if _, ok := res.Deleted[retention.KindAudit]; ok {
		t.Errorf("a run without the account pool touched the audit log: %+v", res)
	}
}
