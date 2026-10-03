package e2e

import (
	"testing"
	"time"
)

// dupTest is the duplication test of the moves that exist in phase 5
// step 1: 16 bots drop parts of their Jellopy stacks and pick up each
// other's for 6 seconds, then pick up what is left.
func dupTest(srv *Server) DupTest {
	return DupTest{
		Server:   srv,
		Bots:     16,
		Duration: 6 * time.Second,
		Seed:     1,
		Moves:    []Move{DropMove(Jellopy), PickUpMove(), PickUpMove()},
		Settle:   SettlePickUps(12, 12),
	}
}

// TestDuplicationDropsAndPickUps runs the duplication test without a
// database.
func TestDuplicationDropsAndPickUps(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: "909:30", Race: true})
	d := dupTest(srv)
	rep := d.Run(t)
	t.Logf("%d drops and %d pick-ups seen, moves %v, %d audits; %d Jellopy", rep.Dropped, rep.Added, rep.Moves, rep.Audits, rep.End.Items[Jellopy])
	if rep.Moves["drop"] == 0 || rep.Moves["pick_up"] == 0 || rep.Audits < 5 {
		t.Errorf("too little happened: %v moves, %d audits", rep.Moves, rep.Audits)
	}
	// Every drop was picked up again.
	if rep.Dropped != rep.Added || rep.End.Ground != 0 {
		t.Errorf("%d drops and %d pick-ups, %d items left on the ground", rep.Dropped, rep.Added, rep.End.Ground)
	}
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
	t.Logf("%d drops and %d pick-ups seen, moves %v, %d audits", rep.Dropped, rep.Added, rep.Moves, rep.Audits)
	d.CheckDatabase(t, rep)
	srv.CheckLogs()
}
