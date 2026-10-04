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
	t.Logf("%d pick-ups the bots made themselves, %d items came in, %d unmatched", rep.Taken, rep.Added, rep.Unmatched)
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

// TestDuplicationPartyPickUps runs the duplication test with the bots in
// parties of 4 with even item share and shared pick-up, so a pick-up
// gives the item to a random member on the map.
func TestDuplicationPartyPickUps(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: "909:30", Race: true})
	d := dupTest(srv)
	d.Prefix = "Party"
	d.Setup = PartySetup(4, true, true)
	rep := d.Run(t)
	checkDupReport(t, rep)
	// With even item share among 4 members, a pick-up gives the picker
	// the item one time in 4, so each bot's items that came in differ
	// from its own pick-ups. Without sharing they differ only for a
	// connection that closed in the middle of a pick-up.
	t.Logf("%d pick-ups the bots made themselves, %d items came in, %d unmatched", rep.Taken, rep.Added, rep.Unmatched)
	if rep.Unmatched <= rep.Reconnects {
		t.Errorf("%d items came in that differ from the bots' own pick-ups, with %d logins again: the items were not shared", rep.Unmatched, rep.Reconnects)
	}
	srv.CheckLogs()
}

// guildStorageTest is the duplication test with guild storage: 8 bots
// in one guild drop, pick up and disconnect as dupTest's do, and also
// open the guild storage in turn, as an NPC would, and put Jellopy in
// and take it out. Only one bot at a time has the storage open; the
// others get STORAGE_OUTCOME_IN_USE. Every bot stays on the start map,
// whose zone the audit reads the guild storages on.
func guildStorageTest(srv *Server, counts *StorageCounts) DupTest {
	d := dupTest(srv)
	d.Bots = 8
	d.Prefix = "Guild"
	storage := GuildStorageMove(srv, Jellopy, counts)
	storage.Weight = 15
	d.Moves = append(d.Moves, storage)
	d.Settle = SettleStorage(d.Settle)
	d.Setup = GuildStorageSetup("Vault")
	return d
}

// checkStorageReport checks that the bots used the guild storage, and
// that the storage holds what the audit counts in it.
func checkStorageReport(t *testing.T, rep DupReport, counts *StorageCounts) {
	t.Helper()
	t.Logf("guild storage: opened %d times, %d refused as in use, %d puts and %d takes, %d connections closed with it open; it holds %d Jellopy at the end",
		counts.Opened.Load(), counts.InUse.Load(), counts.Put.Load(), counts.Taken.Load(), counts.Dropped.Load(), rep.End.Stored[Jellopy])
	if counts.Opened.Load() == 0 || counts.InUse.Load() == 0 || counts.Put.Load() == 0 || counts.Taken.Load() == 0 {
		t.Errorf("too little happened in the guild storage")
	}
}

// TestDuplicationGuildStorage runs the guild storage duplication test
// without a database.
func TestDuplicationGuildStorage(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: "909:30,714:1", Race: true})
	var counts StorageCounts
	d := guildStorageTest(srv, &counts)
	rep := d.Run(t)
	checkDupReport(t, rep)
	checkStorageReport(t, rep, &counts)
	srv.CheckLogs()
}

// TestDuplicationGuildStorageWithDatabase runs the guild storage
// duplication test with PostgreSQL, then checks the saved inventories
// and guild storage.
func TestDuplicationGuildStorageWithDatabase(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{StartItems: "909:30,714:1", Database: true, Race: true})
	var counts StorageCounts
	d := guildStorageTest(srv, &counts)
	d.Prefix = "SavedGuild"
	rep := d.Run(t)
	checkDupReport(t, rep)
	checkStorageReport(t, rep, &counts)
	d.CheckDatabase(t, rep)
	srv.CheckLogs()
}
