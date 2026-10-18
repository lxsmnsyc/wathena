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
// advisory lock, and a run that finds it taken does nothing.
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

// MinDays and MaxDays bound each period.
const (
	MinDays = 1
	MaxDays = 3650
)

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
// chat lines and old log partitions and holds the lock. Account is the
// account role's pool, which deletes login records and audit rows. A
// nil Account skips those two, as with --dev.
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
	// before the first.
	LastRun, LastSuccess time.Time
	// NextRun is when the next run starts, or zero when the job does
	// not run.
	NextRun time.Time
	// LastError is the last run's error, or "".
	LastError string
	// Skipped is set when the last run found the lock taken: another
	// process ran the job.
	Skipped bool
	// Last counts the rows the last run deleted, by kind. Total counts
	// them since the process started.
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

// Status returns a copy of the job's status.
func (j *Job) Status() Status {
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
	if j.Account != nil {
		n, err := store.DeleteOldAudit(ctx, j.Account, cutoff(j.Periods.AuditDays))
		res.Deleted[KindAudit] = n
		if err != nil {
			fail(KindAudit, err)
		}
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
	return res, runErr
}

// finish records a run in the status.
func (j *Job) finish(start time.Time, res Result, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	s := &j.status
	s.LastRun, s.Skipped, s.LastError = start, res.Skipped, ""
	if err != nil {
		s.LastError = err.Error()
	}
	if res.Skipped {
		return
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
//   - wathena_retention_deleted_rows_total{kind}: the rows deleted since
//     the process started.
//   - wathena_retention_last_success_timestamp_seconds: the start of the
//     last run that deleted everything it had to, as Unix seconds, or 0
//     before the first.
func (j *Job) WritePrometheus(w io.Writer) error {
	s := j.Status()
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
