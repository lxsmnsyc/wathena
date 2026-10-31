package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/castle"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Castles (proposal 0004, section 2): one row per castle record, written
// on each change through the SocialStore's queue, so a castle's owner
// saves in order with the guild writes.

// LoadCastles reads every saved castle's state, by record name.
func LoadCastles(ctx context.Context, pool *pgxpool.Pool) (map[string]castle.State, error) {
	rows, err := db.New(pool).LoadCastles(ctx)
	if err != nil {
		return nil, err
	}
	out := make(map[string]castle.State, len(rows))
	for _, r := range rows {
		out[r.Castle] = castle.State{Owner: r.OwnerID, Economy: r.Economy, Defense: r.Defense,
			InvestedEconomy: r.InvestedEconomy, InvestedDefense: r.InvestedDefense, NextTime: r.NextTime,
			PayTime: r.PayTime, CreateTime: r.CreateTime, Kafra: r.Kafra,
			Guardians: uint8(r.Guardians & 0xff)} // #nosec G115 -- the column holds 0 to 255.
	}
	return out, nil
}

// SaveCastle queues a write of every field of a castle.
func (s *SocialStore) SaveCastle(name string, c castle.State) {
	s.guildWrite(func(ctx context.Context, q *db.Queries) error {
		return q.UpsertCastle(ctx, db.UpsertCastleParams{Castle: name, OwnerID: c.Owner, Economy: c.Economy, Defense: c.Defense,
			InvestedEconomy: c.InvestedEconomy, InvestedDefense: c.InvestedDefense, NextTime: c.NextTime,
			PayTime: c.PayTime, CreateTime: c.CreateTime, Kafra: c.Kafra, Guardians: int(c.Guardians)})
	})
}
