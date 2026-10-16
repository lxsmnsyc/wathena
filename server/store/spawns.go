package store

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/spawns"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// The spawns of a procedural world (decision 0042): `wactl world new`
// writes each field's and floor's spawn list with the layout, and
// waserver reads them. The game role reads the rows only.

// insertSpawns writes the spawn lists in a transaction.
func insertSpawns(ctx context.Context, q *db.Queries, list []spawns.Planned) error {
	for _, p := range list {
		for i, s := range p.Spawns {
			arg := db.InsertWorldSpawnParams{
				MapID: p.Map, Ord: i, Monster: s.Monster, Count: s.Count, RespawnMs: s.RespawnMs, Role: string(s.Role),
				AreaX: s.Area.X, AreaY: s.Area.Y, AreaW: s.Area.W, AreaH: s.Area.H,
				AvoidX: []int{}, AvoidY: []int{}, AvoidR: []int{},
			}
			for _, a := range s.Avoid {
				arg.AvoidX, arg.AvoidY, arg.AvoidR = append(arg.AvoidX, a.X), append(arg.AvoidY, a.Y), append(arg.AvoidR, a.R)
			}
			if err := q.InsertWorldSpawn(ctx, arg); err != nil {
				return fmt.Errorf("map %s, spawn %d (%s): %w", p.Map, i, s.Monster, err)
			}
		}
	}
	return nil
}

// LoadSpawns reads the spawn lists of the world's maps, by map ID, in
// their order. The game role runs it. A classic world has none.
func LoadSpawns(ctx context.Context, pool *pgxpool.Pool) (map[string][]spawns.Spawn, error) {
	rows, err := db.New(pool).WorldSpawns(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string][]spawns.Spawn{}
	for _, r := range rows {
		role, ok := spawns.ParseRole(r.Role)
		if !ok {
			return nil, fmt.Errorf("store: map %s: the role %q", r.MapID, r.Role)
		}
		s := spawns.Spawn{Monster: r.Monster, Count: r.Count, RespawnMs: r.RespawnMs, Role: role,
			Area: spawns.Area{X: r.AreaX, Y: r.AreaY, W: r.AreaW, H: r.AreaH}}
		if len(r.AvoidY) != len(r.AvoidX) || len(r.AvoidR) != len(r.AvoidX) {
			return nil, fmt.Errorf("store: map %s: the clear cells of %s", r.MapID, r.Monster)
		}
		for i := range r.AvoidX {
			s.Avoid = append(s.Avoid, spawns.Avoid{X: r.AvoidX[i], Y: r.AvoidY[i], R: r.AvoidR[i]})
		}
		out[r.MapID] = append(out[r.MapID], s)
	}
	return out, nil
}
