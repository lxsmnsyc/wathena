package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store/db"
)

// WorldCounts are the row counts a backup's manifest records (decision
// 0048). `wactl world verify-backup` counts them again in the restored
// copy and compares.
type WorldCounts struct {
	Accounts    int64 `json:"accounts"`
	Characters  int64 `json:"characters"`
	ItemSerials int64 `json:"item_serials"`
	Zeny        int64 `json:"zeny"`
}

// CountWorld counts the accounts, characters, item serials and zeny of
// a database.
func CountWorld(ctx context.Context, pool *pgxpool.Pool) (WorldCounts, error) {
	return countWorld(ctx, db.New(pool))
}

func countWorld(ctx context.Context, q *db.Queries) (WorldCounts, error) {
	r, err := q.WorldCounts(ctx)
	return WorldCounts(r), err
}

// Snapshot is an open read-only transaction whose snapshot pg_dump can
// share with --snapshot. Its counts are of that snapshot, so they match
// the dump while the game writes. Close it after pg_dump ends.
type Snapshot struct {
	ID     string
	Counts WorldCounts
	tx     pgx.Tx
}

// ExportSnapshot opens a repeatable read, read-only transaction, names
// its snapshot and counts the world in it.
func ExportSnapshot(ctx context.Context, pool *pgxpool.Pool) (*Snapshot, error) {
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	q := db.New(tx)
	id, err := q.ExportSnapshot(ctx)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	counts, err := countWorld(ctx, q)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return &Snapshot{ID: id, Counts: counts, tx: tx}, nil
}

// Close ends the snapshot's transaction.
func (s *Snapshot) Close(ctx context.Context) error {
	return s.tx.Rollback(ctx)
}
