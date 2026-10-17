package store

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/content"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// The procedural content of a procedural world (decision 0044): `wactl
// world new` writes the places, objects and quests with the layout, and
// waserver reads them. The game role reads the rows only.

// insertContent writes a world's content in a transaction. nil writes
// nothing.
func insertContent(ctx context.Context, q *db.Queries, w *content.World) error {
	if w == nil {
		return nil
	}
	ords := map[string]int{}
	for _, p := range w.Places {
		ord := ords[p.Map]
		ords[p.Map]++
		if err := q.InsertWorldPlace(ctx, db.InsertWorldPlaceParams{MapID: p.Map, Ord: ord, X: p.X, Y: p.Y, Tags: p.Tags, Source: p.Source}); err != nil {
			return fmt.Errorf("map %s, place %v: %w", p.Map, p.Tags, err)
		}
	}
	ords = map[string]int{}
	for _, o := range w.Objects {
		ord := ords[o.Map]
		ords[o.Map]++
		if err := q.InsertWorldObject(ctx, db.InsertWorldObjectParams{MapID: o.Map, Ord: ord, Model: o.Model, X: o.X, Y: o.Y, Turn: o.Turn, Source: o.Source}); err != nil {
			return fmt.Errorf("map %s, object %s: %w", o.Map, o.Model, err)
		}
	}
	for _, x := range w.Quests {
		arg := db.InsertWorldQuestParams{
			ID: int(x.ID), Town: x.Town, Purpose: x.Purpose, Guild: x.Guild, Stage: x.Stage, Step: x.Step,
			Template: x.Template, Kind: x.Kind, Title: x.Title, Monster: x.Monster, Item: x.Item, Count: x.Count,
			DropRate: x.DropRate, BaseExp: x.BaseExp, JobExp: x.JobExp, Zeny: x.Zeny,
		}
		if x.To != "" {
			arg.ToTown = pgtype.Text{String: x.To, Valid: true}
		}
		if err := q.InsertWorldQuest(ctx, arg); err != nil {
			return fmt.Errorf("quest %d (%s): %w", x.ID, x.Title, err)
		}
	}
	return nil
}

// LoadContent reads a world's content: its places, objects and quests,
// without the spawns, which LoadSpawns reads. The game role runs it. A
// classic world has none.
func LoadContent(ctx context.Context, pool *pgxpool.Pool) (*content.World, error) {
	q := db.New(pool)
	w := &content.World{}
	places, err := q.WorldPlaces(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range places {
		w.Places = append(w.Places, content.Place{Map: p.MapID, X: p.X, Y: p.Y, Tags: slices.Clone(p.Tags), Source: p.Source})
	}
	objects, err := q.WorldObjects(ctx)
	if err != nil {
		return nil, err
	}
	for _, o := range objects {
		w.Objects = append(w.Objects, content.Object{Map: o.MapID, Model: o.Model, X: o.X, Y: o.Y, Turn: o.Turn, Source: o.Source})
	}
	quests, err := q.WorldQuests(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range quests {
		if r.ID < int(content.FirstQuestID) || r.ID > int(content.LastQuestID) {
			return nil, fmt.Errorf("store: world quest %d is outside %d to %d", r.ID, content.FirstQuestID, content.LastQuestID)
		}
		w.Quests = append(w.Quests, content.Quest{
			ID: uint32(r.ID), Town: r.Town, Purpose: r.Purpose, Guild: r.Guild, Stage: r.Stage, Step: r.Step, // #nosec G115 -- checked above.
			Template: r.Template, Kind: r.Kind, Title: r.Title, Monster: r.Monster, Item: r.Item, Count: r.Count,
			DropRate: r.DropRate, To: r.ToTown.String, BaseExp: r.BaseExp, JobExp: r.JobExp, Zeny: r.Zeny,
		})
	}
	return w, nil
}
