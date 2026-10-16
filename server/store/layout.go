package store

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// The world layout (decision 0041): `wactl world new` writes a
// procedural world's maps and links with the world row, and waserver
// reads them. Display names change with @renametown. Each character's
// entered maps and memo points are saved with it.

// ErrNoLayout is LoadLayout's error for a world without a layout, such
// as a classic world or one made before phase 7 step 4.
var ErrNoLayout = errors.New("store: the world has no layout")

// insertLayout writes a layout's maps and links in a transaction.
func insertLayout(ctx context.Context, q *db.Queries, l *layout.Layout) error {
	for i, m := range l.Maps {
		params := m.Params
		if params == nil {
			params = []byte{}
		}
		err := q.InsertWorldMap(ctx, db.InsertWorldMapParams{
			ID: m.ID, Ord: i, Kind: m.Kind, DisplayName: m.Name, MinLevel: m.Levels[0], MaxLevel: m.Levels[1],
			Distance: m.Distance, X: int(m.X), Y: int(m.Y), Size: m.Size, Biome: m.Biome, Record: m.Record, Params: params,
			Town: int(m.Town), Dungeon: m.Dungeon, Floor: m.Floor, Floors: m.Floors, Nomemo: m.NoMemo,
		})
		if err != nil {
			return fmt.Errorf("map %s: %w", m.ID, err)
		}
	}
	for i, k := range l.Links {
		err := q.InsertWorldLink(ctx, db.InsertWorldLinkParams{
			Ord: i, AMap: k.A.Map, ATag: k.A.Tag, AIndex: k.A.Index, BMap: k.B.Map, BTag: k.B.Tag, BIndex: k.B.Index,
		})
		if err != nil {
			return fmt.Errorf("link %s to %s: %w", k.A, k.B, err)
		}
	}
	return nil
}

// LoadLayout reads the world's layout. The game role runs it. It
// returns ErrNoLayout when the world has none.
func LoadLayout(ctx context.Context, pool *pgxpool.Pool) (*layout.Layout, error) {
	q := db.New(pool)
	rows, err := q.WorldMaps(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNoLayout
	}
	links, err := q.WorldLinks(ctx)
	if err != nil {
		return nil, err
	}
	l := &layout.Layout{}
	for _, r := range rows {
		l.Maps = append(l.Maps, layout.Map{
			ID: r.ID, Kind: r.Kind, Name: r.DisplayName, Levels: [2]int{r.MinLevel, r.MaxLevel}, Distance: r.Distance,
			X: int64(r.X), Y: int64(r.Y), Size: r.Size, Biome: r.Biome, Record: r.Record, Params: r.Params,
			Town: int64(r.Town), Dungeon: r.Dungeon, Floor: r.Floor, Floors: r.Floors, NoMemo: r.Nomemo,
		})
	}
	for _, k := range links {
		l.Links = append(l.Links, layout.Link{
			A: layout.End{Map: k.AMap, Tag: k.ATag, Index: k.AIndex},
			B: layout.End{Map: k.BMap, Tag: k.BTag, Index: k.BIndex},
		})
	}
	l.Home = layout.HomeOf(l)
	return l, nil
}

// RenameMaps writes new display names in one transaction. The names
// must stay unique in any case.
func RenameMaps(ctx context.Context, pool *pgxpool.Pool, names []layout.Rename) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		for _, n := range names {
			c, err := q.RenameWorldMap(ctx, db.RenameWorldMapParams{ID: n.Map, DisplayName: n.Name})
			if err != nil {
				return err
			}
			if c == 0 {
				return fmt.Errorf("store: no map %s", n.Map)
			}
		}
		return nil
	})
}

// loadMaps reads the maps a character entered and its memo points.
func loadMaps(ctx context.Context, q *db.Queries, c *character.Character) error {
	visited, err := q.CharacterMaps(ctx, c.ID)
	if err != nil {
		return err
	}
	c.Visited = visited
	memos, err := q.CharacterMemos(ctx, c.ID)
	if err != nil {
		return err
	}
	c.Memos = nil
	for _, m := range memos {
		c.Memos = append(c.Memos, character.Memo{Map: m.MapID, X: m.X, Y: m.Y})
	}
	return nil
}

// writeMaps writes the maps a character entered since base, and its
// memo points when they changed.
func writeMaps(ctx context.Context, q *db.Queries, id int64, base, c *character.Character) error {
	var added []string
	for _, m := range c.Visited {
		if !slices.Contains(base.Visited, m) {
			added = append(added, m)
		}
	}
	if len(added) > 0 {
		if err := q.InsertCharacterMaps(ctx, db.InsertCharacterMapsParams{CharacterID: id, MapIds: added}); err != nil {
			return err
		}
	}
	if slices.Equal(base.Memos, c.Memos) {
		return nil
	}
	if err := q.DeleteCharacterMemos(ctx, id); err != nil {
		return err
	}
	if len(c.Memos) == 0 {
		return nil
	}
	p := db.InsertCharacterMemosParams{CharacterID: id}
	for i, m := range c.Memos {
		p.Slots, p.MapIds, p.Xs, p.Ys = append(p.Slots, i), append(p.MapIds, m.Map), append(p.Xs, m.X), append(p.Ys, m.Y)
	}
	return q.InsertCharacterMemos(ctx, p)
}
