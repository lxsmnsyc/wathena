package store

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Staff roles and the GM audit log on the game role (decision 0028).

// LoadStaffRoles returns the roles the admin screens gave, by account
// ID, for the world's staff registry.
func LoadStaffRoles(ctx context.Context, pool *pgxpool.Pool) (map[int64]string, error) {
	rows, err := db.New(pool).AllStaffRoles(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]string, len(rows))
	for _, r := range rows {
		out[r.AccountID] = r.Role
	}
	return out, nil
}

// AuditRow is one row of the GM audit log.
type AuditRow = staff.AuditRow

// WriteStaffAudit adds a row to the GM audit log. The game role may add
// rows, but not read, change or delete them.
func WriteStaffAudit(ctx context.Context, pool *pgxpool.Pool, row AuditRow) error {
	return writeAudit(ctx, db.New(pool), row)
}

// writeAudit adds an audit row with q, such as in a commit's
// transaction.
func writeAudit(ctx context.Context, q *db.Queries, row AuditRow) error {
	args := row.Arguments
	if args == nil {
		args = map[string]any{}
	}
	b, err := json.Marshal(args)
	if err != nil {
		return err
	}
	return q.WriteStaffAudit(ctx, db.WriteStaffAuditParams{
		AccountID: row.AccountID, AccountName: row.AccountName, Role: row.Role, Action: row.Action, Source: row.Source,
		Target: row.Target, TargetAccountID: pgtype.Int8{Int64: row.TargetAccount, Valid: row.TargetAccount > 0},
		Arguments: b, Reason: row.Reason, NeedsReview: row.NeedsReview,
	})
}
