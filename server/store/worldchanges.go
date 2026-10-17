package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/store/db"
)

// Rotating dungeons and world changes (decision 0045): `wactl world new`
// writes the rotating dungeon and the derived keys with the world, and
// waserver reads the state and records each change in one transaction
// with the moves of the points of the maps it removed.

// WorldRotation is a world's rotating dungeon, as world_rotations keeps
// it.
type WorldRotation struct {
	layout.Rotation
	// Key is child(child(secret, "rotation"), dungeon), which gives the
	// spawns of the dungeon's floors.
	Key []byte
	// ActiveDate is the date of the floors that run, or "" before the
	// server first started the world.
	ActiveDate string
}

// WorldChanges is the state of a world's rotations and seasons.
type WorldChanges struct {
	Rotations []WorldRotation
	// SeasonKey is child(secret, "season"), or nil for a world made
	// before migration 00035.
	SeasonKey []byte
	// SeasonsStarted is the date seasons started, or "" while they are
	// off, and SeasonEnds how many season ends changed a region.
	SeasonsStarted string
	SeasonEnds     int
	// Regions are each changed region's season, by town ID.
	Regions map[string]int
}

// Move is where the points on a removed map go: a town's save spot.
type Move struct {
	From, To string
	X, Y     int
}

// WorldChange is what CommitWorldChange records: a rotation, a region's
// change, or both none and only moves.
type WorldChange struct {
	// Dungeon and Date are a rotation's dungeon and its new date.
	Dungeon, Date string
	// Region and Season are a region's change, and SeasonEnds the count
	// of season ends after it, or 0 for a change staff forced.
	Region     string
	Season     int
	SeasonEnds int
	// Moves move the characters, save points and memo points on the
	// removed or replaced maps.
	Moves []Move
}

func pgDate(s string) (pgtype.Date, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return pgtype.Date{}, fmt.Errorf("store: bad date %q", s)
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

func dateString(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format("2006-01-02")
}

// insertWorldChanges writes a new world's rotating dungeons and season
// key in its transaction.
func insertWorldChanges(ctx context.Context, q *db.Queries, rotations []WorldRotation, seasonKey []byte) error {
	for _, r := range rotations {
		err := q.InsertWorldRotation(ctx, db.InsertWorldRotationParams{Dungeon: r.Dungeon, Style: r.Style,
			Every: r.Rule.Every, At: r.Rule.At, Weekday: r.Rule.Weekday, Key: r.Key})
		if err != nil {
			return fmt.Errorf("the rotating dungeon %s: %w", r.Dungeon, err)
		}
	}
	if seasonKey != nil {
		return q.SetSeasonKey(ctx, seasonKey)
	}
	return nil
}

// LoadWorldChanges reads the state of the world's rotations and seasons.
// The game role runs it.
func LoadWorldChanges(ctx context.Context, pool *pgxpool.Pool) (WorldChanges, error) {
	q := db.New(pool)
	var out WorldChanges
	rows, err := q.WorldRotations(ctx)
	if err != nil {
		return out, err
	}
	for _, r := range rows {
		out.Rotations = append(out.Rotations, WorldRotation{
			Rotation: layout.Rotation{Dungeon: r.Dungeon, Style: r.Style, Rule: layout.RotationRule{Every: r.Every, At: r.At, Weekday: r.Weekday}},
			Key:      r.Key, ActiveDate: dateString(r.ActiveDate),
		})
	}
	if out.SeasonKey, err = q.SeasonKey(ctx); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	seasons, err := q.WorldSeasons(ctx)
	if err != nil {
		return out, err
	}
	for _, s := range seasons {
		out.SeasonsStarted, out.SeasonEnds = dateString(s.Started), s.Ends
	}
	regions, err := q.WorldRegions(ctx)
	if err != nil {
		return out, err
	}
	out.Regions = map[string]int{}
	for _, r := range regions {
		out.Regions[r.Town] = r.Season
	}
	return out, nil
}

// StartSeasons records the date seasons started, once.
func StartSeasons(ctx context.Context, pool *pgxpool.Pool, date string) error {
	d, err := pgDate(date)
	if err != nil {
		return err
	}
	return db.New(pool).StartSeasons(ctx, d)
}

// CommitWorldChange records a change and moves the points on the maps it
// removed, in one transaction (strategy section 8.10): characters saved
// there move to their town's save spot, and so do save points and memo
// points. A character keeps one memo point per map. A change that is not
// recorded happens again at the next start.
func CommitWorldChange(ctx context.Context, pool *pgxpool.Pool, c WorldChange) error {
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		for _, m := range c.Moves {
			if _, err := q.MoveCharacterMaps(ctx, db.MoveCharacterMapsParams{FromMap: m.From, ToMap: m.To, X: m.X, Y: m.Y}); err != nil {
				return fmt.Errorf("characters on %s: %w", m.From, err)
			}
			if _, err := q.MoveSavePoints(ctx, db.MoveSavePointsParams{FromMap: m.From, ToMap: m.To, X: m.X, Y: m.Y}); err != nil {
				return fmt.Errorf("save points on %s: %w", m.From, err)
			}
			if _, err := q.DropDoubleMemos(ctx, db.DropDoubleMemosParams{FromMap: m.From, ToMap: m.To}); err != nil {
				return fmt.Errorf("memo points on %s: %w", m.From, err)
			}
			if _, err := q.MoveMemos(ctx, db.MoveMemosParams{FromMap: m.From, ToMap: m.To, X: m.X, Y: m.Y}); err != nil {
				return fmt.Errorf("memo points on %s: %w", m.From, err)
			}
		}
		if c.Dungeon != "" {
			d, err := pgDate(c.Date)
			if err != nil {
				return err
			}
			n, err := q.SetRotationDate(ctx, db.SetRotationDateParams{Dungeon: c.Dungeon, ActiveDate: d})
			if err != nil {
				return err
			}
			if n == 0 {
				return fmt.Errorf("store: no rotating dungeon %s", c.Dungeon)
			}
		}
		if c.Region != "" {
			if err := q.SetRegionSeason(ctx, db.SetRegionSeasonParams{Town: c.Region, Season: c.Season}); err != nil {
				return fmt.Errorf("the region %s: %w", c.Region, err)
			}
		}
		if c.SeasonEnds > 0 {
			n, err := q.SetSeasonEnds(ctx, c.SeasonEnds)
			if err != nil {
				return err
			}
			if n == 0 {
				return errors.New("store: seasons have not started")
			}
		}
		return nil
	})
}
