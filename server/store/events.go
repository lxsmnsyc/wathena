package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store/db"
	"github.com/lxsmnsyc/wathena/server/worldevent"
)

// EventRuns keeps the last run of each world event in the
// world_event_runs table. It is a worldevent.Store.
type EventRuns struct {
	pool *pgxpool.Pool
}

// NewEventRuns returns the event run store of a database. waserver uses
// the game role's pool.
func NewEventRuns(pool *pgxpool.Pool) *EventRuns { return &EventRuns{pool: pool} }

// EventRuns returns the last run of each event.
func (e *EventRuns) EventRuns(ctx context.Context) ([]worldevent.Run, error) {
	rows, err := db.New(e.pool).EventRuns(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]worldevent.Run, len(rows))
	for i, r := range rows {
		out[i] = worldevent.Run{Event: r.Event, Scheduled: r.ScheduledAt.Time, Started: r.StartedAt.Time, Ends: r.EndsAt.Time}
	}
	return out, nil
}

// RecordEventRun records a run, in place of the event's last one.
func (e *EventRuns) RecordEventRun(ctx context.Context, r worldevent.Run) error {
	return db.New(e.pool).RecordEventRun(ctx, db.RecordEventRunParams{
		Event:       r.Event,
		ScheduledAt: pgtype.Timestamptz{Time: r.Scheduled, Valid: true},
		StartedAt:   pgtype.Timestamptz{Time: r.Started, Valid: true},
		EndsAt:      pgtype.Timestamptz{Time: r.Ends, Valid: true},
	})
}
