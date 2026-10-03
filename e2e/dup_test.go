package e2e

import (
	"testing"
	"time"
)

// dupTest is the duplication test of the moves that exist in phase 5
// step 1: 16 bots drop parts of their Jellopy stacks and pick up each
// other's for 6 seconds, then pick up what is left. About one move in
// 60 closes the bot's connection in the middle of a drop or a pick-up,
// and the bot logs in again.
func dupTest(srv *Server) DupTest {
	drop, pickUp, disconnect := DropMove(Jellopy), PickUpMove(), DisconnectMove(Jellopy)
	drop.Weight, pickUp.Weight, disconnect.Weight = 20, 40, 1
	return DupTest{
		Server:   srv,
		Bots:     16,
		Duration: 6 * time.Second,
		Seed:     1,
		Moves:    []Move{drop, pickUp, disconnect},
		Settle:   SettlePickUps(12, 12),
	}
}

// checkDupReport checks that enough happened, and that every drop was
// picked up again.
func checkDupReport(t *testing.T, rep DupReport) {
	t.Helper()
	t.Logf("%d drops and %d pick-ups seen, moves %v, %d audits and %d with a bot away, %d logins again; %d Jellopy",
		rep.Dropped, rep.Added, rep.Moves, rep.Audits, rep.Partial, rep.Reconnects, rep.End.Items[Jellopy])
	if rep.Moves["drop"] == 0 || rep.Moves["pick_up"] == 0 || rep.Moves["disconnect"] == 0 || rep.Audits < 5 {
		t.Errorf("too little happened: %v moves, %d audits", rep.Moves, rep.Audits)
	}
	// A move sent just before a connection closed can happen without
	// its bot seeing it.
	if diff := rep.Dropped - rep.Added; diff > rep.Reconnects || -diff > rep.Reconnects || rep.End.Ground != 0 {
		t.Errorf("%d drops and %d pick-ups seen with %d logins again, %d items left on the ground", rep.Dropped, rep.Added, rep.Reconnects, rep.End.Ground)
	}
}

// TestDuplicationDropsAndPickUps runs the duplication test without a
// database.
func TestDuplicationDropsAndPickUps(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: "909:30", Race: true})
	d := dupTest(srv)
	rep := d.Run(t)
	checkDupReport(t, rep)
	srv.CheckLogs()
}

// TestDuplicationWithDatabase runs the duplication test with PostgreSQL,
// then checks the saved inventories and the item log.
func TestDuplicationWithDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: "909:30", Database: true, Race: true})
	d := dupTest(srv)
	d.Prefix = "Saved"
	rep := d.Run(t)
	checkDupReport(t, rep)
	d.CheckDatabase(t, rep)
	srv.CheckLogs()
}
