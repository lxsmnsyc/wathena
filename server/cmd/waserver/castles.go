package main

import (
	"context"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/castle"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/shared/path"
)

// castleRegistry makes the world's castle registry (proposal 0004,
// section 2) from the loaded records and the saved rows. A record whose
// map does not run, or whose teleport cell is not walkable, is left out
// with a warning. names and grids are the world's maps and their grids,
// in the same order. Without a pool, castles start at zeros and nothing
// is saved.
func castleRegistry(ctx context.Context, pool *pgxpool.Pool, recs []*castle.Record, names []string, grids []*path.Grid,
	log *slog.Logger) (*castle.Registry, error) {
	byMap := map[string]*path.Grid{}
	for i, n := range names {
		if i < len(grids) {
			byMap[strings.TrimPrefix(n, "map/")] = grids[i]
		}
	}
	var kept []*castle.Record
	for _, r := range recs {
		g := byMap[r.Map]
		switch {
		case g == nil:
			log.Warn("castle left out: its map does not run", "castle", r.Name, "map", r.Map)
		case r.Teleport != nil && g.NoPass(r.Teleport.X, r.Teleport.Y):
			log.Warn("castle left out: its teleport cell is not walkable", "castle", r.Name, "map", r.Map,
				"x", r.Teleport.X, "y", r.Teleport.Y)
		default:
			kept = append(kept, r)
		}
	}
	var saved map[string]castle.State
	if pool != nil {
		var err error
		if saved, err = store.LoadCastles(ctx, pool); err != nil {
			return nil, err
		}
	}
	reg := castle.New(kept, saved)
	reg.Log = log
	if len(kept) > 0 {
		log.Info("castles loaded", "castles", len(kept), "saved", len(saved))
	}
	return reg, nil
}
