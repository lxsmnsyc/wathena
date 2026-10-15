package adminworld_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/zone"
)

// kicker records kicks, and kicks the characters that have a session.
type kicker struct {
	st    *store.Store
	mu    sync.Mutex
	kicks []string
}

func (k *kicker) Kick(_ context.Context, ids []int64, reason wathenav1.DisconnectReason, text string) []int64 {
	k.mu.Lock()
	defer k.mu.Unlock()
	var out []int64
	for _, id := range ids {
		if k.st.Live(id) {
			out = append(out, id)
		}
	}
	k.kicks = append(k.kicks, reason.String()+" "+text)
	return out
}

func (k *kicker) got() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]string(nil), k.kicks...)
}

var moderator = admin.Actor{AccountID: 1 << 40, AccountName: "Moddy", Role: "moderator"}

func code(err error) string {
	var we *admin.WorldError
	if errors.As(err, &we) {
		return we.Code
	}
	if errors.Is(err, admin.ErrNotFound) {
		return "not_found"
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

// TestSanctions: a mute and a jail of an online and an offline
// character, lifts, a kick and a ban, with their rows and audit rows
// (decision 0030).
func TestSanctions(t *testing.T) {
	e := setup(t)
	loud := e.character("Loud")
	away := e.character("Away")
	s := e.login(loud)
	defer s.Leave()

	// A mute of an online character starts in its zone.
	m, err := e.aw.Sanction(e.ctx, admin.SanctionOrder{Actor: moderator, Kind: "mute", Character: loud, Minutes: 10, Reason: "spam"})
	if err != nil || m.GetId() == 0 || m.GetCharacter() != "Loud" || !m.GetActive() {
		t.Fatalf("mute %v %v", m, err)
	}
	d, err := e.aw.Character(e.ctx, loud)
	if err != nil || len(d.GetSanctions()) != 1 || d.GetSanctions()[0].GetKind() != "mute" {
		t.Fatalf("details %v %v", d.GetSanctions(), err)
	}
	// The zone's save of the mute is the same row.
	if err := e.st.Drain(e.ctx); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := e.admin.QueryRow(e.ctx, "SELECT count(*) FROM sanctions WHERE character_id = $1", loud).Scan(&rows); err != nil || rows != 1 {
		t.Errorf("%d sanction rows, %v", rows, err)
	}
	// Lifting ends it in the zone too.
	l, err := e.aw.LiftSanction(e.ctx, admin.Lift{Actor: moderator, ID: m.GetId(), Reason: "appealed"})
	if err != nil || l.GetActive() || l.GetLiftReason() != "appealed" {
		t.Errorf("lift %v %v", l, err)
	}
	if d, _ := e.aw.Character(e.ctx, loud); len(d.GetSanctions()) != 0 {
		t.Errorf("after the lift %v", d.GetSanctions())
	}
	if _, err := e.aw.LiftSanction(e.ctx, admin.Lift{Actor: moderator, ID: m.GetId(), Reason: "again"}); code(err) != "conflict" {
		t.Errorf("a second lift: %v", err)
	}

	// A jail of an offline character waits for its login.
	j, err := e.aw.Sanction(e.ctx, admin.SanctionOrder{Actor: moderator, Kind: "jail", Character: away, Minutes: 60, Reason: "botting"})
	if err != nil || j.GetCharacter() != "Away" {
		t.Fatalf("jail %v %v", j, err)
	}
	var ret string
	if err := e.admin.QueryRow(e.ctx, "SELECT return_map FROM sanctions WHERE id = $1", j.GetId()).Scan(&ret); err != nil || ret != "map/one" {
		t.Errorf("return map %q %v", ret, err)
	}
	s2 := e.login(away)
	if d, _ := e.aw.Character(e.ctx, away); len(d.GetSanctions()) != 1 || d.GetSanctions()[0].GetKind() != "jail" {
		t.Errorf("the jail after login %v", d.GetSanctions())
	}

	// A kick needs the character online.
	k, err := e.aw.Sanction(e.ctx, admin.SanctionOrder{Actor: moderator, Kind: "kick", Character: away, Reason: "afk"})
	if err != nil || k.GetActive() || k.GetId() == 0 {
		t.Errorf("kick %v %v", k, err)
	}
	s2.Leave()
	if err := e.st.Drain(e.ctx); err != nil {
		t.Fatal(err)
	}
	waitOffline(t, e, away)
	if _, err := e.aw.Sanction(e.ctx, admin.SanctionOrder{Actor: moderator, Kind: "kick", Character: away, Reason: "afk"}); code(err) != "not_found" {
		t.Errorf("a kick of an offline character: %v", err)
	}

	// A ban kicks every character of the account.
	var account int64
	if err := e.admin.QueryRow(e.ctx, "SELECT account_id FROM characters WHERE id = $1", loud).Scan(&account); err != nil {
		t.Fatal(err)
	}
	// It also ends the account's sessions on the account pages (decision
	// 0036).
	ended := &sessionEnder{}
	e.aw.SetSessions(ended)
	b, err := e.aw.Sanction(e.ctx, admin.SanctionOrder{Actor: moderator, Kind: "ban", Account: account, Minutes: 0, Reason: "scam"})
	if err != nil || !b.GetPermanent() || b.GetAccountId() != account || b.HasEndsAt() {
		t.Fatalf("ban %v %v", b, err)
	}
	if !slices.Equal(ended.accounts, []int64{account}) {
		t.Errorf("the ban ended the sessions of %v, want %d", ended.accounts, account)
	}
	kicks := e.kicker.got()
	if len(kicks) != 3 || !strings.HasPrefix(kicks[2], "DISCONNECT_REASON_BANNED Your account is banned permanently. Reason: scam") ||
		!strings.HasPrefix(kicks[0], "DISCONNECT_REASON_KICKED") {
		t.Errorf("kicks %v", kicks)
	}
	if ban, ok, err := e.st.ActiveBan(e.ctx, loud); err != nil || !ok || ban.ID != b.GetId() {
		t.Errorf("active ban %+v %v %v", ban, ok, err)
	}
	bans, err := e.aw.Bans(e.ctx, []int64{account})
	if err != nil || bans[account].GetId() != b.GetId() {
		t.Errorf("bans %v %v", bans, err)
	}
	if d, _ := e.aw.Character(e.ctx, loud); d.GetBan().GetId() != b.GetId() {
		t.Errorf("the details' ban %v", d.GetBan())
	}
	// No one sanctions their own account.
	self := admin.Actor{AccountID: account, AccountName: "Self", Role: "admin"}
	if _, err := e.aw.Sanction(e.ctx, admin.SanctionOrder{Actor: self, Kind: "mute", Character: loud, Minutes: 1, Reason: "x"}); code(err) != "conflict" {
		t.Errorf("a mute of one's own account: %v", err)
	}
	if _, err := e.aw.Sanction(e.ctx, admin.SanctionOrder{Actor: moderator, Kind: "ban", Account: 1 << 40, Minutes: 5, Reason: "x"}); code(err) != "not_found" {
		t.Errorf("a ban of no account: %v", err)
	}

	// The lists.
	list, err := e.aw.Sanctions(e.ctx, admin.SanctionQuery{Character: loud, Limit: 1})
	if err != nil || len(list.GetSanctions()) != 1 || list.GetNextBeforeId() == 0 || list.GetSanctions()[0].GetKind() != "ban" {
		t.Errorf("list %v %v", list, err)
	}
	active, err := e.aw.Sanctions(e.ctx, admin.SanctionQuery{Kind: "ban", Active: true, Limit: 10})
	if err != nil || len(active.GetSanctions()) != 1 {
		t.Errorf("active bans %v %v", active, err)
	}
	if _, err := e.aw.LiftSanction(e.ctx, admin.Lift{Actor: self, ID: b.GetId(), Reason: "mine"}); code(err) != "conflict" {
		t.Errorf("a lift of one's own ban: %v", err)
	}
	lifted, err := e.aw.LiftSanction(e.ctx, admin.Lift{Actor: moderator, ID: b.GetId(), Reason: "appeal"})
	if err != nil || lifted.GetActive() {
		t.Errorf("lift the ban %v %v", lifted, err)
	}
	if _, err := e.aw.LiftSanction(e.ctx, admin.Lift{Actor: moderator, ID: k.GetId(), Reason: "x"}); code(err) != "bad_request" {
		t.Errorf("lift a kick: %v", err)
	}
	var actions string
	if err := e.admin.QueryRow(e.ctx, "SELECT string_agg(action, ',' ORDER BY id) FROM staff_audit").Scan(&actions); err != nil ||
		actions != "moderation.mute,moderation.lift,moderation.jail,moderation.kick,moderation.ban,moderation.lift" {
		t.Errorf("audit actions %q %v", actions, err)
	}
}

func waitOffline(t *testing.T, e *env, id int64) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for e.st.Live(id) {
		if time.Now().After(deadline) {
			t.Fatal("the character did not log out")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestCommandSanctions: GM commands find the target by name, and answer
// with what they did.
func TestCommandSanctions(t *testing.T) {
	e := setup(t)
	loud := e.character("Loud Mouth")
	s := e.login(loud)
	defer s.Leave()
	actor := moderator
	o := zone.StaffOrder{Kind: "mute", Target: "loud mouth", Minutes: 5, Reason: "spam"}
	o.Actor.AccountID, o.Actor.AccountName, o.Actor.Role = actor.AccountID, actor.AccountName, actor.Role
	text, err := e.aw.Command(e.ctx, o)
	if err != nil || text != "Muted Loud Mouth for 5 minutes." {
		t.Errorf("mute: %q %v", text, err)
	}
	o.Target = "Nobody"
	var ce *zone.CommandError
	if _, err := e.aw.Command(e.ctx, o); !errors.As(err, &ce) || ce.Text != "Character not found." {
		t.Errorf("nobody: %v", err)
	}
	var source string
	if err := e.admin.QueryRow(e.ctx, "SELECT source FROM staff_audit WHERE action = 'moderation.mute'").Scan(&source); err != nil || source != "game" {
		t.Errorf("the command's audit row: %q %v", source, err)
	}
}

// TestReportQueue: the queue, a report's detail, and closing it with a
// sanction of the reported player.
func TestReportQueue(t *testing.T) {
	e := setup(t)
	rude := e.character("Rude")
	other := e.character("Other")
	var report int64
	if err := e.admin.QueryRow(e.ctx, `INSERT INTO reports (created_at, reporter_id, reporter_name, target_id, target_name, reason, map)
		VALUES (now(), $1, 'Other', $2, 'Rude', 'insults', 'map/one') RETURNING id`, other, rude).Scan(&report); err != nil {
		t.Fatal(err)
	}
	page, err := e.aw.Reports(e.ctx, admin.ReportQuery{Status: "open", Limit: 10})
	if err != nil || len(page.GetReports()) != 1 || page.GetReports()[0].GetKind() != "player" || page.GetReports()[0].GetTargetAccountId() == 0 {
		t.Fatalf("queue %v %v", page, err)
	}
	if _, err := e.aw.Report(e.ctx, 1<<40); code(err) != "not_found" {
		t.Errorf("no report: %v", err)
	}
	m, err := e.aw.Sanction(e.ctx, admin.SanctionOrder{Actor: moderator, Kind: "mute", Character: rude, Minutes: 5, Reason: "insults"})
	if err != nil {
		t.Fatal(err)
	}
	wrong, err := e.aw.Sanction(e.ctx, admin.SanctionOrder{Actor: moderator, Kind: "mute", Character: other, Minutes: 5, Reason: "x"})
	if err != nil {
		t.Fatal(err)
	}
	d, err := e.aw.Report(e.ctx, report)
	if err != nil || len(d.GetSanctions()) != 1 || d.GetSanctions()[0].GetId() != m.GetId() {
		t.Errorf("detail %v %v", d, err)
	}
	closeWith := func(sanction int64) error {
		_, err := e.aw.CloseReport(e.ctx, admin.CloseReport{Actor: moderator, ID: report, Outcome: "sanctioned", Note: "muted", SanctionID: sanction})
		return err
	}
	if err := closeWith(wrong.GetId()); code(err) != "bad_request" {
		t.Errorf("a sanction of another player: %v", err)
	}
	if err := closeWith(1 << 40); code(err) != "bad_request" {
		t.Errorf("no sanction: %v", err)
	}
	if err := closeWith(m.GetId()); err != nil {
		t.Fatal(err)
	}
	if err := closeWith(m.GetId()); code(err) != "conflict" {
		t.Errorf("a second close: %v", err)
	}
	closed, err := e.aw.Reports(e.ctx, admin.ReportQuery{Status: "closed", Limit: 10})
	if err != nil || len(closed.GetReports()) != 1 || closed.GetReports()[0].GetSanctionId() != m.GetId() ||
		closed.GetReports()[0].GetClosedBy() != "Moddy" || closed.GetReports()[0].GetOutcome() != "sanctioned" {
		t.Errorf("closed %v %v", closed, err)
	}
}

// sessionEnder records the accounts whose sessions a ban ended.
type sessionEnder struct{ accounts []int64 }

func (s *sessionEnder) EndAccountSessions(_ context.Context, account int64) (int, error) {
	s.accounts = append(s.accounts, account)
	return 1, nil
}
