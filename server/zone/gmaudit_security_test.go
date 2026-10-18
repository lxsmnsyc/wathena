package zone

import (
	"testing"
	"time"
)

// endSession ends a session as a closed connection does: the player is
// in no zone and the session has ended.
func endSession(s *Session) {
	s.mu.Lock()
	s.end, s.zone, s.player = endLeave, nil, nil
	s.mu.Unlock()
}

// TestRecallAuditSurvivesLogout checks that @recall writes its audit row
// when the staff member disconnects right after typing it.
func TestRecallAuditSurvivesLogout(t *testing.T) {
	w := newGMWorld()
	gm := w.player(w.a, 10, 2, "Gamemaster", 10, 10, true)
	target := w.player(w.b, 11, 9, "Target Name", 25, 26, true)
	say(w.a, gm, "@recall Target Name")
	endSession(gm.sess)
	w.pump(t, func() bool { z, _ := target.sess.where(); return z == w.a })
	for _, row := range w.b.txAudit {
		if row.Action == "commands.recall" && row.Target == "Target Name" && row.AccountID == 2 && row.AccountName == "GamemasterAccount" {
			return
		}
	}
	t.Errorf("no audit row of the recall: %+v %+v", w.a.txAudit, w.b.txAudit)
}

// TestRenameTownAuditSurvivesLogout checks that @renametown writes its
// audit row when the staff member disconnects right after typing it.
func TestRenameTownAuditSurvivesLogout(t *testing.T) {
	w := newLayoutWorld(t)
	gm := w.join(w.t1, "Admin", 5, 5)
	// The zone takes posted functions on its control channel, as a
	// running zone does, so the test runs them on its own goroutine.
	w.t1.startSender()
	defer w.t1.stopSender()
	say(w.t1, gm, "@renametown Valheim Newheim")
	endSession(gm.sess)
	deadline := time.After(5 * time.Second)
	for {
		for _, row := range w.t1.txAudit {
			if row.Action == "world.rename" && row.Arguments["to"] == "Newheim" && row.AccountName == "AdminAccount" {
				return
			}
		}
		select {
		case f := <-w.t1.control:
			f()
		case <-deadline:
			n, _ := w.atlas.Name(t1ID)
			t.Fatalf("no audit row of the rename to %q: %+v", n, w.t1.txAudit)
		}
	}
}
