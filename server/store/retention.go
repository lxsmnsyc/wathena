package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store/db"
)

// The deletes of the retention job (decision 0049). server/retention
// runs them once a day. The game role deletes chat lines and drops old
// log partitions. The account role deletes login records and audit
// rows.

// retentionBatch is the most rows one delete of chat lines or login
// records takes. The queries say 10,000 too.
const retentionBatch = 10000

// retentionTimeout bounds each statement of the job. A month partition
// is counted before it is dropped, which takes longer than a save.
const retentionTimeout = 5 * time.Minute

// LockRetention takes the retention job's advisory lock on a connection
// of pool, so only one process runs the job at a time. It returns false
// when another session holds the lock. unlock lets the lock and the
// connection go. A connection whose unlock fails is closed, which ends
// its session and so its lock.
func LockRetention(ctx context.Context, pool *pgxpool.Pool) (unlock func(), ok bool, err error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, false, err
	}
	q := db.New(conn)
	locked, err := q.TryRetentionLock(ctx)
	if err != nil || !locked {
		conn.Release()
		return nil, false, err
	}
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if unlocked, err := q.RetentionUnlock(ctx); err != nil || !unlocked {
			_ = conn.Conn().Close(ctx)
		}
		conn.Release()
	}, true, nil
}

// DeleteOldChatLines deletes the chat lines from before a time, except
// those of open reports, in batches, and returns how many went.
func DeleteOldChatLines(ctx context.Context, pool *pgxpool.Pool, before time.Time) (int64, error) {
	q := db.New(pool)
	var total int64
	for {
		ctx, cancel := context.WithTimeout(ctx, retentionTimeout)
		n, err := q.DeleteOldChatLines(ctx, timestamp(before))
		cancel()
		total += n
		if err != nil || n < retentionBatch {
			return total, err
		}
	}
}

// DeleteOldLogins deletes the login records from before a time, with
// their IP addresses, in batches, and returns how many went. pool is the
// account role's.
func DeleteOldLogins(ctx context.Context, pool *pgxpool.Pool, before time.Time) (int64, error) {
	q := db.New(pool)
	var total int64
	for {
		ctx, cancel := context.WithTimeout(ctx, retentionTimeout)
		n, err := q.DeleteOldLogins(ctx, timestamp(before))
		cancel()
		total += n
		if err != nil || n < retentionBatch {
			return total, err
		}
	}
}

// LogRetention is what DropOldLogs removed from one log.
type LogRetention struct {
	// Partitions are the month partitions dropped. Rows counts their
	// rows and the rows deleted from the default partition.
	Partitions int
	Rows       int64
}

// DropOldLogs drops the month partitions of the item and zeny logs whose
// whole month ends at or before before, and deletes the older rows of
// their default partitions. pool is the game role's.
func DropOldLogs(ctx context.Context, pool *pgxpool.Pool, before time.Time) (items, zeny LogRetention, err error) {
	ctx, cancel := context.WithTimeout(ctx, retentionTimeout)
	defer cancel()
	rows, err := db.New(pool).DropOldLogPartitions(ctx, timestamp(before))
	if err != nil {
		return items, zeny, err
	}
	for _, r := range rows {
		l := LogRetention{Partitions: r.Partitions, Rows: r.Deleted}
		switch r.Kind {
		case "item_log":
			items = l
		case "zeny_log":
			zeny = l
		}
	}
	return items, zeny, nil
}

// DeleteOldAudit deletes the rows of the GM audit log from before a
// time, with their reviews, except rows that wait for a review. pool is
// the account role's.
func DeleteOldAudit(ctx context.Context, pool *pgxpool.Pool, before time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, retentionTimeout)
	defer cancel()
	return db.New(pool).DeleteOldStaffAudit(ctx, timestamp(before))
}
