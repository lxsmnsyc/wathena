package admin_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/retention"
)

// fakeRetention is a retention job's status.
type fakeRetention struct {
	status retention.Status
	err    error
}

func (f fakeRetention) Status(context.Context) (retention.Status, error) { return f.status, f.err }

// TestRetentionCall: GET /retention needs world.read, with the session,
// the role and the two-factor rules of every call, and shows the
// periods, the last run, the counts and the warnings (decision 0049).
func TestRetentionCall(t *testing.T) {
	ran := time.Date(2026, 10, 18, 3, 0, 0, 0, time.UTC)
	p := retention.Defaults
	p.ChatDays = 45
	job := fakeRetention{status: retention.Status{Periods: p, LastRun: ran, LastSuccess: ran, NextRun: ran.Add(24 * time.Hour),
		Last: map[string]int64{retention.KindChat: 12, retention.KindAudit: 1}, Total: map[string]int64{retention.KindChat: 40},
		Partitions: 2, Warnings: p.Warnings()}}
	e := setup(t, options{limits: manyCalls, retention: job})
	gm := e.withRole("Gamemaster", "game_master")
	mod := e.withRole("Moddy", "moderator")

	v := get(e, gm, "retention", &adminv1.RetentionStatus{})
	if len(v.GetKinds()) != len(retention.Kinds) || !v.GetLastSuccess().AsTime().Equal(ran) || v.GetPartitionsDropped() != 2 {
		t.Errorf("retention %v", v)
	}
	for _, k := range v.GetKinds() {
		if k.GetKind() == retention.KindChat && (k.GetDays() != 45 || k.GetDefaultDays() != 30 || k.GetLastDeleted() != 12 || k.GetDeletedSinceStart() != 40) {
			t.Errorf("chat %v", k)
		}
		if k.GetKind() == retention.KindItemLog && k.GetDays() != 180 {
			t.Errorf("item log %v", k)
		}
	}
	if len(v.GetWarnings()) != 1 || v.GetLastError() != "" || v.GetSkipped() {
		t.Errorf("warnings %v", v)
	}

	// A role without world.read, a player, no session, and a staff
	// member without a two-factor login are refused.
	e.errorCode(mod, "GET", "retention", "", nil, http.StatusForbidden, "forbidden")
	e.errorCode(e.signup("Player"), "GET", "retention", "", nil, http.StatusForbidden, "forbidden")
	e.errorCode(e.browser(), "GET", "retention", "", nil, http.StatusUnauthorized, "login")
	e.errorCode(e.signup("Owner"), "GET", "retention", "", nil, http.StatusForbidden, "two_factor")
	// It is read-only: there is no other call at the path.
	e.errorCode(gm, "POST", "retention", e.me(gm).GetCsrfToken(), nil, http.StatusNotFound, "not_found")

	// The error of a failed run shows as a fixed text. The database's
	// text stays in the server log.
	e = setup(t, options{limits: manyCalls, retention: fakeRetention{status: retention.Status{Periods: p, LastRun: ran,
		LastError: `chat: ERROR: function delete_old_chat(timestamp) does not exist (SQLSTATE 42883)`}}})
	if v := get(e, e.withRole("Gamemaster", "game_master"), "retention", &adminv1.RetentionStatus{}); v.GetLastError() != admin.RetentionFailed {
		t.Errorf("last error %q", v.GetLastError())
	}

	// Without a job, or when the job's status cannot be read, it answers
	// unavailable.
	e = setup(t, options{limits: manyCalls})
	e.errorCode(e.withRole("Gamemaster", "game_master"), "GET", "retention", "", nil, http.StatusServiceUnavailable, "unavailable")
	e = setup(t, options{limits: manyCalls, retention: fakeRetention{err: errors.New("the database is gone")}})
	e.errorCode(e.withRole("Gamemaster", "game_master"), "GET", "retention", "", nil, http.StatusServiceUnavailable, "unavailable")
}
