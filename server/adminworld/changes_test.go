package adminworld_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/adminworld"
	"github.com/lxsmnsyc/wathena/server/layout"
	"github.com/lxsmnsyc/wathena/server/rotation"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
)

// fakeChanges is a rotation.Manager for the events tool.
type fakeChanges struct {
	status rotation.Status
	forced []string
}

func (f *fakeChanges) Status() rotation.Status { return f.status }

func (f *fakeChanges) Force(_ context.Context, dungeon, region string, audit func(rotation.Change) error) (rotation.Change, error) {
	due := time.Date(2026, 10, 17, 12, 10, 0, 0, time.UTC)
	switch {
	case dungeon == "ahead":
		return rotation.Change{}, rotation.ErrAhead
	case dungeon == "crypt":
		c := rotation.Change{Kind: rotation.KindRotation, Dungeon: dungeon, Date: "2026-10-23", Due: due, Forced: true}
		if err := audit(c); err != nil {
			return rotation.Change{}, err
		}
		f.forced = append(f.forced, dungeon)
		f.status.Rotations[0].Next = &c
		return c, nil
	case region == "t1":
		c := rotation.Change{Kind: rotation.KindRegion, Region: region, Season: 1, Due: due, Forced: true}
		if err := audit(c); err != nil {
			return rotation.Change{}, err
		}
		f.forced = append(f.forced, region)
		return c, nil
	case dungeon != "":
		return rotation.Change{}, rotation.ErrNoDungeon
	}
	return rotation.Change{}, rotation.ErrNoRegion
}

// TestForceChange: the events tool lists the rotating dungeon and the
// regions, and staff force a rotation and a region's change with an
// audit row each (decision 0045).
func TestForceChange(t *testing.T) {
	adminPool := storetest.Migrated(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	game := storetest.As(t, adminPool, "wathena_game")
	st := store.New(game, store.Options{Log: slog.New(slog.DiscardHandler)})
	fc := &fakeChanges{status: rotation.Status{
		Rotations:  []rotation.RotationStatus{{Dungeon: "crypt", Rule: layout.RotationRule{Every: "weekly", Weekday: "friday", At: "20:00"}, Date: "2026-10-16"}},
		SeasonDays: 30, Started: "2026-10-01", Regions: []rotation.RegionStatus{{Region: "t1", Season: 2}},
	}}
	aw := adminworld.New(adminworld.Config{Store: st, Log: slog.New(slog.DiscardHandler), Tools: adminworld.Tools{Changes: fc}})
	actor := admin.Actor{AccountID: 7, AccountName: "Admin1", Role: "admin"}
	list, err := aw.Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.GetRotations()) != 1 || list.GetRotations()[0].GetRotation() != "weekly on friday at 20:00" || list.GetRotations()[0].GetDate() != "2026-10-16" ||
		list.GetSeasons().GetDays() != 30 || len(list.GetSeasons().GetRegions()) != 1 || list.GetSeasons().GetRegions()[0].GetSeason() != 2 {
		t.Fatalf("events %v", list)
	}
	list, err = aw.ForceChange(ctx, admin.ChangeOrder{Actor: actor, Dungeon: "crypt", Reason: "the event"})
	if err != nil {
		t.Fatal(err)
	}
	if n := list.GetRotations()[0].GetNext(); n.GetDate() != "2026-10-23" || !n.GetForced() {
		t.Errorf("after the force %v", n)
	}
	if _, err := aw.ForceChange(ctx, admin.ChangeOrder{Actor: actor, Region: "t1"}); err != nil {
		t.Fatal(err)
	}
	var we *admin.WorldError
	if _, err := aw.ForceChange(ctx, admin.ChangeOrder{Actor: actor, Dungeon: "cave"}); !errors.As(err, &we) || we.Code != "not_found" {
		t.Errorf("an unknown dungeon: %v", err)
	}
	if _, err := aw.ForceChange(ctx, admin.ChangeOrder{Actor: actor, Region: "t9"}); !errors.As(err, &we) || we.Code != "not_found" {
		t.Errorf("an unknown region: %v", err)
	}
	rows, err := adminPool.Query(ctx, "SELECT action || ' ' || target || ' ' || reason || ' ' || arguments::text FROM staff_audit ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		got = append(got, s)
	}
	if len(got) != 2 || got[0] != `events.rotate crypt the event {"at": "2026-10-17T12:10:00Z", "date": "2026-10-23"}` ||
		got[1] != `events.change_region t1  {"at": "2026-10-17T12:10:00Z", "season": 1}` {
		t.Errorf("audit rows %q", got)
	}
	none := adminworld.New(adminworld.Config{Store: st, Log: slog.New(slog.DiscardHandler)})
	if _, err := none.ForceChange(ctx, admin.ChangeOrder{Actor: actor, Dungeon: "crypt"}); !errors.As(err, &we) || we.Code != "not_found" {
		t.Errorf("a world without changes: %v", err)
	}
}

// TestForceChangeAuditFails: when the audit row cannot be written, the
// change is not planned, and a dungeon ahead of its schedule is refused
// (review finding L39).
func TestForceChangeAuditFails(t *testing.T) {
	adminPool := storetest.Migrated(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	game := storetest.As(t, adminPool, "wathena_game")
	st := store.New(game, store.Options{Log: slog.New(slog.DiscardHandler)})
	// A closed pool makes every audit write fail.
	game.Close()
	fc := &fakeChanges{status: rotation.Status{
		Rotations: []rotation.RotationStatus{{Dungeon: "crypt", Rule: layout.RotationRule{Every: "weekly", Weekday: "friday", At: "20:00"}, Date: "2026-10-16"}},
	}}
	aw := adminworld.New(adminworld.Config{Store: st, Log: slog.New(slog.DiscardHandler), Tools: adminworld.Tools{Changes: fc}})
	actor := admin.Actor{AccountID: 7, AccountName: "Admin1", Role: "admin"}
	if _, err := aw.ForceChange(ctx, admin.ChangeOrder{Actor: actor, Dungeon: "crypt"}); err == nil {
		t.Error("the force went on without its audit row")
	}
	if _, err := aw.ForceChange(ctx, admin.ChangeOrder{Actor: actor, Region: "t1"}); err == nil {
		t.Error("the region's change went on without its audit row")
	}
	if len(fc.forced) != 0 {
		t.Errorf("planned without an audit row: %v", fc.forced)
	}
	var we *admin.WorldError
	if _, err := aw.ForceChange(ctx, admin.ChangeOrder{Actor: actor, Dungeon: "ahead"}); !errors.As(err, &we) || we.Code != "conflict" {
		t.Errorf("a dungeon ahead of its schedule: %v", err)
	}
}
