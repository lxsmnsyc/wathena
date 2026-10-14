package e2e

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/tools/botclient"
)

// The account page's deletion (decision 0032). The test server runs
// --dev, which has no account pages, so the tests write the request
// as the account service does, and run the job with --dev's POST
// /debug/wathena/deletions.

// requestDeletion asks for the deletion of a character's account, as the
// account page does, 14 days from now on the database's clock.
func (s *Server) requestDeletion(ctx context.Context, name string) {
	s.t.Helper()
	if _, err := s.DB.Exec(ctx, `INSERT INTO account_deletions (account_id, requested_at, delete_after)
		SELECT account_id, now(), now() + interval '14 days' FROM characters WHERE name = $1`, name); err != nil {
		s.t.Fatal(err)
	}
}

// cancelDeletion cancels it, as the account page's button does.
func (s *Server) cancelDeletion(ctx context.Context, name string) {
	s.t.Helper()
	if _, err := s.DB.Exec(ctx, `DELETE FROM account_deletions WHERE account_id = (SELECT account_id FROM characters WHERE name = $1)`, name); err != nil {
		s.t.Fatal(err)
	}
}

// runDeletions runs the deletion job on the server clock, and returns
// how many accounts it deleted.
func (s *Server) runDeletions(ctx context.Context) int {
	s.t.Helper()
	body, err := s.post(ctx, s.Debug+"/debug/wathena/deletions", nil)
	if err != nil {
		s.t.Fatal(err)
	}
	var out struct {
		Deleted int `json:"deleted"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		s.t.Fatal(err)
	}
	return out.Deleted
}

// TestDeletionBlocksPlayUntilCancelled: while an account is set to be
// deleted, a login of its character is refused with the date. Once the
// player cancels, the character plays again.
func TestDeletionBlocksPlayUntilCancelled(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true})
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	p := srv.Join(t, "Leaving")
	p.Say("one last line")
	_ = p.Close()
	srv.requestDeletion(ctx, "Leaving")
	_, err := botclient.Dial(ctx, srv.WS, []byte("Leaving"), botclient.Options{})
	if err == nil || !strings.Contains(err.Error(), "DISCONNECT_REASON_ACCOUNT_DELETION") || !strings.Contains(err.Error(), "set to be deleted on") {
		t.Fatalf("a login while set to be deleted: %v", err)
	}
	// The job does nothing before the date.
	if n := srv.runDeletions(ctx); n != 0 {
		t.Errorf("the job deleted %d accounts before the date", n)
	}
	srv.cancelDeletion(ctx, "Leaving")
	back := srv.Join(t, "Leaving")
	back.Say("I stayed")
	if n := srv.count(t, "SELECT count(*) FROM characters WHERE name = 'Leaving'"); n != 1 {
		t.Errorf("characters named Leaving: %d", n)
	}
	srv.CheckLogs()
}

// TestDeletionRunsAfterTheWait: a party member's account is set to be
// deleted. 15 days later on the server clock, the job deletes the
// account, its character and its chat lines, and the party it led breaks
// up for the member who stays. The item log keeps the character's ID.
func TestDeletionRunsAfterTheWait(t *testing.T) {
	t.Parallel()
	srv := Start(t, Options{Database: true})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	leaver := srv.Join(t, "Goner")
	stayer := srv.Join(t, "Remainer")
	MakeParty("Short Lived", false, false, leaver, stayer)
	leaver.Say("goodbye everyone")
	stayer.Hears("Goner", "goodbye everyone")
	leaver.Use(RedPotion)
	var id int64
	if err := srv.DB.QueryRow(ctx, "SELECT id FROM characters WHERE name = 'Goner'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	// The chat line and the item log line reach the database.
	deadline := time.Now().Add(15 * time.Second)
	for srv.count(t, "SELECT count(*) FROM chat_log WHERE character_id = $1", id) == 0 ||
		srv.count(t, "SELECT count(*) FROM item_log WHERE from_kind = 'inventory' AND from_id = $1", id) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the chat line or the item log line is not saved")
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = leaver.Close()
	<-leaver.Done()
	srv.requestDeletion(ctx, "Goner")
	stayer.Skip()

	if err := srv.AdvanceClock(ctx, 15*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	// The logout's save may still be on its way: the job waits for it.
	deadline = time.Now().Add(15 * time.Second)
	for srv.runDeletions(ctx) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("the job did not delete the account")
		}
		time.Sleep(200 * time.Millisecond)
	}
	if r := stayer.PartyResult(); r.GetOutcome() != wathenav1.PartyOutcome_PARTY_OUTCOME_DISBANDED {
		t.Errorf("the member who stays got %v", r)
	}
	stayer.PartyInfo(func(i *wathenav1.PartyInfo) bool { return i.GetId() == 0 })
	for q, want := range map[string]int64{
		"SELECT count(*) FROM characters WHERE id = $1":              0,
		"SELECT count(*) FROM chat_log WHERE character_id = $1":      0,
		"SELECT count(*) FROM inventory WHERE character_id = $1":     0,
		"SELECT count(*) FROM party_members WHERE character_id = $1": 0,
	} {
		if n := srv.count(t, q, id); n != want {
			t.Errorf("%s: %d rows, want %d", q, n, want)
		}
	}
	for q, want := range map[string]int64{
		"SELECT count(*) FROM accounts WHERE name = 'Goner'":      0,
		"SELECT count(*) FROM account_deletions":                  0,
		"SELECT count(*) FROM parties WHERE name = 'Short Lived'": 0,
		"SELECT count(*) FROM characters WHERE name = 'Remainer'": 1,
		"SELECT count(*) FROM chat_log WHERE name = 'Remainer'":   0,
	} {
		if n := srv.count(t, q); n != want {
			t.Errorf("%s: %d rows, want %d", q, n, want)
		}
	}
	if n := srv.count(t, "SELECT count(*) FROM item_log WHERE from_kind = 'inventory' AND from_id = $1", id); n == 0 {
		t.Error("the item log lost the character's lines")
	}
	// The member who stays still plays.
	stayer.Say("still here")
	srv.CheckLogs()
}
