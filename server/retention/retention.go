// Package retention is the retention job (strategy section 7.10,
// proposal 0002 review decision 49, decision 0049). Once a day it
// deletes what the server keeps only for a time:
//   - login records with their IP addresses, after login_ip_days,
//   - chat lines, after chat_days, except the lines that an open report
//     holds,
//   - the item and zeny logs, after item_log_days: whole month
//     partitions once their month is older, and rows of the default
//     partitions by date,
//   - the GM audit log, after audit_days, with the reviews, except rows
//     that wait for a review.
//
// Only one process runs the job at a time: each run takes a PostgreSQL
// advisory lock, and a run that finds it taken does nothing. A run that
// holds the lock records itself in the table retention_status, so the
// status shows the last run of any process, and survives a restart.
package retention

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store"
)

// Periods are the days each kind of data is kept.
type Periods struct {
	LoginIPDays int
	ChatDays    int
	ItemLogDays int
	AuditDays   int
}

// Defaults are the periods of strategy section 7.10.
var Defaults = Periods{LoginIPDays: 90, ChatDays: 30, ItemLogDays: 180, AuditDays: 730}

// MinDays and MaxDays bound each period. The item and zeny logs and the
// audit log have MinLogDays instead of MinDays: drop_old_log_partitions
// and delete_old_staff_audit refuse a cutoff later than 30 days ago.
const (
	MinDays    = 1
	MinLogDays = 30
	MaxDays    = 3650
)

// MinDaysOf returns the shortest period of a kind.
func MinDaysOf(kind string) int {
	switch kind {
	case KindItemLog, KindZenyLog, KindAudit:
		return MinLogDays
	}
	return MinDays
}

// Every is how often the job runs.
const Every = 24 * time.Hour

// Kinds of data, as the logs, the metrics and the admin API name them.
const (
	KindLoginIPs = "login_ips"
	KindChat     = "chat"
	KindItemLog  = "item_log"
	KindZenyLog  = "zeny_log"
	KindAudit    = "audit"
)

// Kinds lists every kind in a fixed order.
var Kinds = []string{KindLoginIPs, KindChat, KindItemLog, KindZenyLog, KindAudit}

// Days returns the period of a kind. The item and zeny logs share one.
func (p Periods) Days(kind string) int {
	switch kind {
	case KindLoginIPs:
		return p.LoginIPDays
	case KindChat:
		return p.ChatDays
	case KindItemLog, KindZenyLog:
		return p.ItemLogDays
	case KindAudit:
		return p.AuditDays
	}
	return 0
}

// Key returns the config key of a kind's period.
func Key(kind string) string {
	switch kind {
	case KindLoginIPs:
		return "retention.login_ip_days"
	case KindChat:
		return "retention.chat_days"
	case KindItemLog, KindZenyLog:
		return "retention.item_log_days"
	case KindAudit:
		return "retention.audit_days"
	}
	return ""
}

// Warnings lists the periods that are longer than the defaults, one
// line each. The privacy policy of strategy section 7.10 names the
// defaults.
func (p Periods) Warnings() []string {
	var out []string
	for _, k := range []string{KindLoginIPs, KindChat, KindItemLog, KindAudit} {
		if d, def := p.Days(k), Defaults.Days(k); d > def {
			out = append(out, Key(k)+" is "+strconv.Itoa(d)+" days, longer than the default of "+strconv.Itoa(def)+" days of the privacy policy")
		}
	}
	return out
}

// Job runs the deletes. Game is the game role's pool, which deletes
// chat lines, old log partitions and old audit rows, and holds the
// lock. Account is the account role's pool, which deletes login
// records. A nil Account skips those, as with --dev.
type Job struct {
	Game    *pgxpool.Pool
	Account *pgxpool.Pool
	Periods Periods
	Log     *slog.Logger
	// Clock is time.Now when nil.
	Clock func() time.Time

	mu     sync.Mutex
	status Status
}

// Status is what the job did, for the admin API and the metrics.
type Status struct {
	Periods Periods
	// LastRun is when the last run started, and LastSuccess when the
	// last run that deleted everything it had to started. Both are zero
	// before the first. They are of the last run of any process.
	LastRun, LastSuccess time.Time
	// NextRun is when this process runs next, or zero when the job does
	// not run.
	NextRun time.Time
	// LastError is the last run's error, or "".
	LastError string
	// Skipped is set when this process's last run found the lock taken:
	// another process ran the job.
	Skipped bool
	// Last counts the rows the last run of any process deleted, by
	// kind. Total counts the rows this process deleted since it
	// started.
	Last, Total map[string]int64
	// Partitions are the month partitions the last run dropped.
	Partitions int
	// Warnings are Periods.Warnings.
	Warnings []string
}

func (j *Job) now() time.Time {
	if j.Clock != nil {
		return j.Clock()
	}
	return time.Now()
}

func (j *Job) log() *slog.Logger {
	if j.Log != nil {
		return j.Log
	}
	return slog.Default()
}

// Status returns the job's status. The last run, its counts and its
// errors come from retention_status, so they show a run of another
// process, and a run from before a restart. NextRun, Skipped and Total
// are this process's. On a database error, Status returns the status of
// this process's runs alone, with the error.
func (j *Job) Status(ctx context.Context) (Status, error) {
	s := j.local()
	run, ok, err := store.LoadRetentionRun(ctx, j.Game)
	if err != nil || !ok {
		return s, err
	}
	s.LastRun, s.LastSuccess, s.LastError, s.Partitions = run.Start, run.LastSuccess, run.Error, run.Partitions
	s.Last = map[string]int64{KindLoginIPs: run.LoginIPs, KindChat: run.Chat, KindItemLog: run.ItemLog,
		KindZenyLog: run.ZenyLog, KindAudit: run.Audit}
	return s, nil
}

// local returns a copy of the status of this process's runs.
func (j *Job) local() Status {
	j.mu.Lock()
	defer j.mu.Unlock()
	s := j.status
	s.Periods, s.Warnings = j.Periods, j.Periods.Warnings()
	s.Last, s.Total = copyCounts(j.status.Last), copyCounts(j.status.Total)
	return s
}

func copyCounts(m map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(Kinds))
	for _, k := range Kinds {
		out[k] = m[k]
	}
	return out
}

// Run runs the job at once, then every Every, until ctx ends.
func (j *Job) Run(ctx context.Context) {
	t := time.NewTicker(Every)
	defer t.Stop()
	for {
		j.mu.Lock()
		j.status.NextRun = j.now().Add(Every)
		j.mu.Unlock()
		if _, err := j.RunOnce(ctx); err != nil && ctx.Err() == nil {
			j.log().Error("the retention job failed; it runs again in 24 hours", "err", err)
		}
		select {
		case <-t.C:
		case <-ctx.Done():
			return
		}
	}
}

// Result is one run's deletes.
type Result struct {
	// Skipped is set when another process held the lock.
	Skipped bool
	// Deleted counts the rows deleted, by kind.
	Deleted map[string]int64
	// Partitions are the month partitions dropped.
	Partitions int
}

// RunOnce runs the job once. Each kind runs even when another failed,
// and the errors are joined.
func (j *Job) RunOnce(ctx context.Context) (Result, error) {
	start := j.now()
	res := Result{Deleted: map[string]int64{}}
	unlock, ok, err := store.LockRetention(ctx, j.Game)
	if err != nil {
		j.finish(start, res, err)
		return res, fmt.Errorf("retention lock: %w", err)
	}
	if !ok {
		res.Skipped = true
		j.log().Info("the retention job runs in another process; this run skips")
		j.finish(start, res, nil)
		return res, nil
	}
	defer unlock()
	cutoff := func(days int) time.Time { return start.Add(-time.Duration(days) * 24 * time.Hour) }
	var errs []string
	fail := func(kind string, err error) {
		errs = append(errs, kind+": "+err.Error())
	}
	if j.Account != nil {
		n, err := store.DeleteOldLogins(ctx, j.Account, cutoff(j.Periods.LoginIPDays))
		res.Deleted[KindLoginIPs] = n
		if err != nil {
			fail(KindLoginIPs, err)
		}
	}
	n, err := store.DeleteOldChatLines(ctx, j.Game, cutoff(j.Periods.ChatDays))
	res.Deleted[KindChat] = n
	if err != nil {
		fail(KindChat, err)
	}
	items, zeny, err := store.DropOldLogs(ctx, j.Game, cutoff(j.Periods.ItemLogDays))
	res.Deleted[KindItemLog], res.Deleted[KindZenyLog] = items.Rows, zeny.Rows
	res.Partitions = items.Partitions + zeny.Partitions
	if err != nil {
		fail("item and zeny logs", err)
	}
	// The game role deletes old audit rows. The account role writes
	// the reviews, so it may not also delete the rows they release
	// (review decision 51).
	n, err = store.DeleteOldAudit(ctx, j.Game, cutoff(j.Periods.AuditDays))
	res.Deleted[KindAudit] = n
	if err != nil {
		fail(KindAudit, err)
	}
	var runErr error
	if len(errs) > 0 {
		runErr = errors.New(strings.Join(errs, "; "))
	}
	args := []any{"took", j.now().Sub(start).Round(time.Millisecond), "partitions_dropped", res.Partitions}
	for _, k := range Kinds {
		args = append(args, k, res.Deleted[k])
	}
	if runErr != nil {
		args = append(args, "err", runErr)
		j.log().Warn("the retention job deleted old data, with errors", args...)
	} else {
		j.log().Info("the retention job deleted old data", args...)
	}
	j.finish(start, res, runErr)
	// The run is recorded while the lock still holds, so two processes
	// never write it at once.
	run := store.RetentionRun{Start: start, LoginIPs: res.Deleted[KindLoginIPs], Chat: res.Deleted[KindChat],
		ItemLog: res.Deleted[KindItemLog], ZenyLog: res.Deleted[KindZenyLog], Audit: res.Deleted[KindAudit], Partitions: res.Partitions}
	if runErr != nil {
		run.Error = runErr.Error()
	}
	if err := store.SaveRetentionRun(context.WithoutCancel(ctx), j.Game, run); err != nil {
		j.log().Warn("the retention job could not record its run", "err", err)
	}
	return res, runErr
}

// finish records a run in the status.
func (j *Job) finish(start time.Time, res Result, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	s := &j.status
	// A skip is not a run: the run of the other process shows instead.
	s.Skipped = res.Skipped
	if res.Skipped {
		return
	}
	s.LastRun, s.LastError = start, ""
	if err != nil {
		s.LastError = err.Error()
	}
	if s.Total == nil {
		s.Total = map[string]int64{}
	}
	for k, n := range res.Deleted {
		s.Total[k] += n
	}
	s.Last, s.Partitions = res.Deleted, res.Partitions
	if err == nil {
		s.LastSuccess = start
	}
}

// WritePrometheus writes the job's metrics in the Prometheus text
// format:
//   - wathena_retention_deleted_rows_total{kind}: the rows this process
//     deleted since it started.
//   - wathena_retention_last_success_timestamp_seconds: the start of the
//     last run of any process that deleted everything it had to, as Unix
//     seconds, or 0 before the first. When retention_status cannot be
//     read, it is this process's last success.
func (j *Job) WritePrometheus(ctx context.Context, w io.Writer) error {
	s, _ := j.Status(ctx)
	var b strings.Builder
	b.WriteString("# HELP wathena_retention_deleted_rows_total Rows the retention job deleted since the process started, by kind.\n")
	b.WriteString("# TYPE wathena_retention_deleted_rows_total counter\n")
	for _, k := range Kinds {
		fmt.Fprintf(&b, "wathena_retention_deleted_rows_total{kind=%q} %d\n", k, s.Total[k])
	}
	b.WriteString("# HELP wathena_retention_last_success_timestamp_seconds When the last successful retention run started, as Unix seconds.\n")
	b.WriteString("# TYPE wathena_retention_last_success_timestamp_seconds gauge\n")
	last := int64(0)
	if !s.LastSuccess.IsZero() {
		last = s.LastSuccess.Unix()
	}
	b.WriteString("wathena_retention_last_success_timestamp_seconds " + strconv.FormatInt(last, 10) + "\n")
	_, err := io.WriteString(w, b.String())
	return err
}
