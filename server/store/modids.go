package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/mod"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// ModIDs keeps the IDs of the records mods add in the mod_ids table
// (decision 0006). It is a mod.IDStore.
type ModIDs struct {
	pool *pgxpool.Pool
}

// NewModIDs returns the ID store of a database. waserver uses the game
// role's pool.
func NewModIDs(pool *pgxpool.Pool) *ModIDs { return &ModIDs{pool: pool} }

// ModIDs returns every ID given so far.
func (m *ModIDs) ModIDs(ctx context.Context) ([]mod.AssignedID, error) {
	rows, err := db.New(m.pool).ModIDs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]mod.AssignedID, len(rows))
	for i, r := range rows {
		out[i] = mod.AssignedID{Kind: r.Kind, Name: r.Name, ID: uint32(r.ID), Mod: r.ModID} // #nosec G115 -- the table's CHECK keeps IDs within 32 bits.
	}
	return out, nil
}

// AssignModID gives a name the next free ID of its kind, or returns the
// one it has. A clash with another server that assigned at the same
// moment is tried again.
func (m *ModIDs) AssignModID(ctx context.Context, kind, name, modID string, start uint32) (uint32, error) {
	q := db.New(m.pool)
	for range 50 {
		id, err := q.AssignModID(ctx, db.AssignModIDParams{Kind: kind, Name: name, Start: int64(start), ModID: modID})
		if errors.Is(err, pgx.ErrNoRows) {
			// The name has an ID already.
			id, err = q.ModID(ctx, db.ModIDParams{Kind: kind, Name: name})
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			continue
		}
		if err != nil {
			return 0, err
		}
		return uint32(id), nil // #nosec G115 -- the table's CHECK keeps IDs within 32 bits.
	}
	return 0, fmt.Errorf("store: could not assign an ID to %s %s", kind, name)
}
