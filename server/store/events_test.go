package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/schedule"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/worldevent"
)

type runner struct{ started int }

func (r *runner) Start(context.Context, worldevent.Def, worldevent.Run) error {
	r.started++
	return nil
}
func (r *runner) End(context.Context, worldevent.Def, worldevent.Run) error { return nil }
func (r *runner) Condition(context.Context, worldevent.Def) (bool, error)   { return false, nil }

// TestEventRuns checks that a run recorded in PostgreSQL keeps a
// restarted scheduler from running the same time again.
func TestEventRuns(t *testing.T) {
	_, game := migrated(t)
	ctx := timeout(t)
	runs := store.NewEventRuns(game)
	def := worldevent.Def{Name: "example.bosses:Night", Duration: time.Hour,
		Trigger: worldevent.Trigger{Kind: worldevent.Clock, Spec: schedule.Spec{Kind: schedule.Daily, Hour: 20}}}
	at := func(h, m int) time.Time { return time.Date(2026, 10, 31, h, m, 0, 0, time.UTC) }
	r := &runner{}
	s := worldevent.New([]worldevent.Def{def}, worldevent.Options{Runner: r, Store: runs})
	if err := s.Load(ctx, at(19, 59)); err != nil {
		t.Fatal(err)
	}
	s.Check(ctx, at(20, 0))
	if r.started != 1 {
		t.Fatalf("started %d runs", r.started)
	}
	list, err := runs.EventRuns(ctx)
	if err != nil || len(list) != 1 || !list[0].Scheduled.Equal(at(20, 0)) || !list[0].Ends.Equal(at(21, 0)) {
		t.Fatalf("runs %+v, %v", list, err)
	}
	again := worldevent.New([]worldevent.Def{def}, worldevent.Options{Runner: r, Store: runs})
	if err := again.Load(ctx, at(20, 0).Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	again.Check(ctx, at(20, 1))
	if r.started != 1 {
		t.Fatalf("a restart ran the event again: %d runs", r.started)
	}
}
