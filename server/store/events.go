package store

import (
	"context"
	"time"

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

// RecordEventRun records a run, in place of the event's last one, and
// deletes the progress of the event's older runs.
func (e *EventRuns) RecordEventRun(ctx context.Context, r worldevent.Run) error {
	q := db.New(e.pool)
	at := pgtype.Timestamptz{Time: r.Scheduled, Valid: true}
	if err := q.RecordEventRun(ctx, db.RecordEventRunParams{
		Event:       r.Event,
		ScheduledAt: at,
		StartedAt:   pgtype.Timestamptz{Time: r.Started, Valid: true},
		EndsAt:      pgtype.Timestamptz{Time: r.Ends, Valid: true},
	}); err != nil {
		return err
	}
	return q.DeleteOldEventProgress(ctx, db.DeleteOldEventProgressParams{Event: r.Event, ScheduledAt: at})
}

// EventProgress returns the units of a run's progress (decision 0025).
func (e *EventRuns) EventProgress(ctx context.Context, event string, scheduled time.Time) ([]string, error) {
	return db.New(e.pool).EventProgress(ctx, db.EventProgressParams{Event: event, ScheduledAt: pgtype.Timestamptz{Time: scheduled, Valid: true}})
}

// RecordEventProgress records a unit of a run's progress.
func (e *EventRuns) RecordEventProgress(ctx context.Context, event string, scheduled time.Time, unit string) error {
	return db.New(e.pool).RecordEventProgress(ctx, db.RecordEventProgressParams{Event: event,
		ScheduledAt: pgtype.Timestamptz{Time: scheduled, Valid: true}, Unit: unit})
}

// EventUnit is a unit of a run's progress that a commit writes with the
// rewards it gave.
type EventUnit struct {
	Event     string
	Scheduled time.Time
	Unit      string
}

// writeEventUnits writes the units of progress of a commit.
func writeEventUnits(ctx context.Context, q *db.Queries, units []EventUnit) error {
	for _, u := range units {
		if err := q.RecordEventProgress(ctx, db.RecordEventProgressParams{Event: u.Event,
			ScheduledAt: pgtype.Timestamptz{Time: u.Scheduled, Valid: true}, Unit: u.Unit}); err != nil {
			return err
		}
	}
	return nil
}
