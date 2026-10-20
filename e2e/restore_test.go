package e2e

import (
	"context"
	"testing"
	"time"
)

// TestDuplicationRestoresWithDatabase runs the trade duplication test
// with PostgreSQL while restores take random serials from the bots, their
// storages, their carts and the mails, and give them to random bots by
// mail (decision 0029). The holders trade, move items between their
// storages and carts, and lose their connections in the middle of it.
// The totals, the serials held once and the saved rows must hold as in
// every duplication test: a restore only moves a serial, and never
// copies one.
func TestDuplicationRestoresWithDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: "909:30", StartJob: MerchantJob, Database: true, Race: true})
	if srv.DB == nil {
		t.Skip("restores need a database: set WATHENA_TEST_DATABASE_URL")
	}
	var trades TradeCounts
	var personal PersonalCounts
	var restores RestoreCounts
	d := tradeTest(srv, "Restored", &trades, &personal)
	restore := RestoreMove(srv, d.Prefix, d.Bots, &restores)
	restore.Weight = 6
	d.Moves = append(d.Moves, restore)
	d.Duration = 10 * time.Second
	rep := d.Run(t)
	t.Logf("restores: %d moved, %d gone, %d gave up; %d storage and cart moves found their stack restored; trades %d completed; moves %v; %d logins again",
		restores.Moved.Load(), restores.Gone.Load(), restores.Busy.Load(), personal.Restored.Load(), trades.Completed.Load(), rep.Moves, rep.Reconnects)
	if restores.Moved.Load() == 0 || trades.Completed.Load() == 0 || rep.Reconnects == 0 {
		t.Errorf("too little happened: %d restores, %d trades, %d logins again", restores.Moved.Load(), trades.Completed.Load(), rep.Reconnects)
	}
	d.CheckDatabase(t, rep)
	// Each restore wrote its audit row, marked for review, in its
	// commit.
	var rows int64
	if err := srv.DB.QueryRow(context.Background(), `SELECT count(*) FROM staff_audit WHERE action = 'items.restore' AND needs_review`).
		Scan(&rows); err != nil || rows != restores.Moved.Load() {
		t.Errorf("database: %d restore audit rows, %d restores, %v", rows, restores.Moved.Load(), err)
	}
	srv.CheckLogs()
}

// TestDuplicationCutRestoresWithDatabase is the restore duplication test
// on a machine that stalls. Bots leave for up to a second. Restores take
// serials from the saved inventories of the bots that are away, stall
// the start zone for 400 ms, and give up on their requests after 100 to
// 200 ms, as a closed browser or a proxy's timeout. A restore then often
// gives up while its zone works on it. The server must make each
// restore whole or not at all (review 2026-10). Before the fix, the
// restore let go of its offline characters and storages when its
// request ended, the zone moved their items afterwards, and the items
// were in two places.
func TestDuplicationCutRestoresWithDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: "909:30", StartJob: MerchantJob, Database: true, Race: true})
	if srv.DB == nil {
		t.Skip("restores need a database: set WATHENA_TEST_DATABASE_URL")
	}
	var trades TradeCounts
	var personal PersonalCounts
	var restores RestoreCounts
	d := tradeTest(srv, "Cut", &trades, &personal)
	restore := RestoreMove(srv, d.Prefix, d.Bots, &restores)
	cut := CutRestoreMove(srv, d.Prefix, d.Bots, &restores, 200*time.Millisecond)
	leave := LeaveMove(time.Second)
	restore.Weight, cut.Weight, leave.Weight = 3, 12, 4
	d.Moves = append(d.Moves, restore, cut, leave)
	d.Duration = 10 * time.Second
	rep := d.Run(t)
	t.Logf("restores: %d moved, %d cut, %d gone, %d gave up; trades %d completed; moves %v; %d logins again",
		restores.Moved.Load(), restores.Cut.Load(), restores.Gone.Load(), restores.Busy.Load(), trades.Completed.Load(), rep.Moves, rep.Reconnects)
	if restores.Cut.Load() == 0 || rep.Reconnects == 0 {
		t.Errorf("too little happened: %d restores cut, %d logins again", restores.Cut.Load(), rep.Reconnects)
	}
	d.CheckDatabase(t, rep)
	// A cut restore wrote its audit row or not.
	var rows int64
	if err := srv.DB.QueryRow(context.Background(), `SELECT count(*) FROM staff_audit WHERE action = 'items.restore' AND needs_review`).
		Scan(&rows); err != nil || rows < restores.Moved.Load() || rows > restores.Moved.Load()+restores.Cut.Load() {
		t.Errorf("database: %d restore audit rows, %d restores and %d cut, %v", rows, restores.Moved.Load(), restores.Cut.Load(), err)
	}
	srv.CheckLogs()
}
