package admin_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	"github.com/lxsmnsyc/wathena/server/account"
	"github.com/lxsmnsyc/wathena/server/admin"
)

// manyCalls lifts the admin API's rate limit for tests with many calls.
func manyCalls(l *account.Limits) { l.Admin = account.Bucket{Burst: 1000, Every: time.Millisecond} }

func sanctionReq(kind string, character, account int64, minutes int32, reason string) *adminv1.SanctionRequest {
	return adminv1.SanctionRequest_builder{Kind: kind, CharacterId: character, AccountId: account, Minutes: minutes, Reason: reason}.Build()
}

// TestReportCalls: the report queue's filters, the detail, and closing
// with each outcome, with their refusals.
func TestReportCalls(t *testing.T) {
	e := setup(t, options{limits: manyCalls})
	mod := e.withRole("Moddy", "moderator")
	csrf := e.me(mod).GetCsrfToken()

	page := get(e, mod, "reports?status=all&kind=line&target=Bob&limit=10&before_id=99", &adminv1.ReportPage{})
	if len(page.GetReports()) != 1 || page.GetReports()[0].GetTarget() != "Bob" {
		t.Errorf("reports %v", page)
	}
	get(e, mod, "reports", &adminv1.ReportPage{})
	e.world.mu.Lock()
	qs := e.world.reportQueries
	e.world.mu.Unlock()
	if len(qs) != 2 || qs[0] != (admin.ReportQuery{Kind: "line", Target: "Bob", BeforeID: 99, Limit: 10}) ||
		qs[1] != (admin.ReportQuery{Status: "open", Limit: admin.DefaultReportPage}) {
		t.Errorf("report queries %+v", qs)
	}
	for _, q := range []string{"status=gone", "kind=rude", "limit=0", "limit=101", "before_id=x", "target=" + strings.Repeat("a", 25)} {
		e.errorCode(mod, "GET", "reports?"+q, "", nil, http.StatusBadRequest, "bad_request")
	}

	d := get(e, mod, "reports/3", &adminv1.ReportDetail{})
	if d.GetReport().GetId() != 3 || len(d.GetLines()) != 1 || !d.GetLines()[0].GetReported() {
		t.Errorf("report %v", d)
	}
	e.errorCode(mod, "GET", "reports/404", "", nil, http.StatusNotFound, "not_found")
	e.errorCode(mod, "GET", "reports/x", "", nil, http.StatusBadRequest, "bad_request")

	closeReq := func(outcome, note string, sanction int64) *adminv1.CloseReportRequest {
		return adminv1.CloseReportRequest_builder{Outcome: outcome, Note: note, SanctionId: sanction}.Build()
	}
	status, b := e.call(mod, "POST", "reports/3/close", csrf, closeReq("sanctioned", "muted for spam", 5))
	if status != http.StatusOK || !strings.Contains(string(b), `"outcome":"sanctioned"`) {
		t.Errorf("close: %d %s", status, b)
	}
	if status, b := e.call(mod, "POST", "reports/4/close", csrf, closeReq("no_action", "", 0)); status != http.StatusOK {
		t.Errorf("close without action: %d %s", status, b)
	}
	e.errorCode(mod, "POST", "reports/409/close", csrf, closeReq("warned", "", 0), http.StatusConflict, "conflict")
	e.errorCode(mod, "POST", "reports/3/close", csrf, closeReq("banned", "", 0), http.StatusBadRequest, "bad_request")
	e.errorCode(mod, "POST", "reports/3/close", csrf, closeReq("sanctioned", "", 0), http.StatusBadRequest, "bad_request")
	e.errorCode(mod, "POST", "reports/3/close", csrf, closeReq("warned", "", 5), http.StatusBadRequest, "bad_request")
	e.errorCode(mod, "POST", "reports/3/close", csrf, closeReq("warned", strings.Repeat("n", 256), 0), http.StatusBadRequest, "bad_request")
	e.errorCode(mod, "POST", "reports/3/close", "", closeReq("warned", "", 0), http.StatusForbidden, "csrf")
	e.world.mu.Lock()
	closes := e.world.closes
	e.world.mu.Unlock()
	if len(closes) != 3 || closes[0].Outcome != "sanctioned" || closes[0].SanctionID != 5 || closes[0].Note != "muted for spam" ||
		closes[0].AccountName != "Moddy" || closes[0].Role != "moderator" {
		t.Errorf("closes %+v", closes)
	}

	// A role without moderation.reports, and a player, get forbidden.
	player := e.staffer("Player")
	e.errorCode(player, "GET", "reports", "", nil, http.StatusForbidden, "forbidden")
	e.errorCode(player, "GET", "reports/3", "", nil, http.StatusForbidden, "forbidden")
	e.errorCode(player, "POST", "reports/3/close", e.me(player).GetCsrfToken(), closeReq("warned", "", 0), http.StatusForbidden, "forbidden")
	// A role without a second factor gets two_factor.
	weak := e.signup("Weakling")
	if _, err := e.admin.Exec(context.Background(), "INSERT INTO staff_roles (account_id, role) SELECT id, 'moderator' FROM accounts WHERE name = 'Weakling'"); err != nil {
		t.Fatal(err)
	}
	e.errorCode(weak, "GET", "reports", "", nil, http.StatusForbidden, "two_factor")
}

// TestSanctionCalls: each kind needs its permission, a ban needs a
// recent login, and a permanent ban moderation.ban_permanent.
func TestSanctionCalls(t *testing.T) {
	e := setup(t, options{limits: manyCalls, roles: map[string][]string{
		"admin":       {"staff.*", "audit.*", "players.*", "items.*", "moderation.*", "commands.*"},
		"game_master": {"players.read", "moderation.reports", "moderation.mute", "moderation.kick", "moderation.jail", "moderation.ban"},
		"moderator":   {"players.read", "moderation.reports", "moderation.mute", "moderation.kick", "moderation.jail"},
		"muter":       {"moderation.mute"},
		"reader":      {"players.read"},
	}})
	adm := e.withRole("Admin", "admin")
	gm := e.withRole("Gamemaster", "game_master")
	mod := e.withRole("Moddy", "moderator")
	muter := e.withRole("Muter", "muter")
	reader := e.withRole("Reader", "reader")
	csrf := func(c *http.Client) string { return e.me(c).GetCsrfToken() }

	// Every kind, by a role that has it.
	for _, c := range []struct {
		who  *http.Client
		req  *adminv1.SanctionRequest
		want string
	}{
		{mod, sanctionReq("mute", 7, 0, 10, "spam"), "mute"},
		{mod, sanctionReq("jail", 7, 0, 60, "botting"), "jail"},
		{mod, sanctionReq("kick", 7, 0, 0, "afk in the way"), "kick"},
		{gm, sanctionReq("ban", 0, 9, 1440, "scam"), "ban"},
		{adm, sanctionReq("ban", 0, 9, 0, "real money trade"), "ban"},
		{muter, sanctionReq("mute", 7, 0, 525600, "spam"), "mute"},
	} {
		status, b := e.call(c.who, "POST", "sanctions", csrf(c.who), c.req)
		if status != http.StatusOK || !strings.Contains(string(b), `"kind":"`+c.want+`"`) {
			t.Errorf("%s: %d %s", c.want, status, b)
		}
	}
	e.world.mu.Lock()
	orders := e.world.orders
	e.world.mu.Unlock()
	if len(orders) != 6 || orders[0] != (admin.SanctionOrder{Actor: admin.Actor{AccountID: e.accountID("Moddy"), AccountName: "Moddy",
		Role: "moderator"}, Kind: "mute", Character: 7, Minutes: 10, Reason: "spam"}) || orders[4].Minutes != 0 || orders[4].Account != 9 {
		t.Errorf("orders %+v", orders)
	}

	// Permissions.
	e.errorCode(mod, "POST", "sanctions", csrf(mod), sanctionReq("ban", 0, 9, 60, "x"), http.StatusForbidden, "forbidden")
	e.errorCode(gm, "POST", "sanctions", csrf(gm), sanctionReq("ban", 0, 9, 0, "x"), http.StatusForbidden, "forbidden")
	e.errorCode(muter, "POST", "sanctions", csrf(muter), sanctionReq("jail", 7, 0, 5, "x"), http.StatusForbidden, "forbidden")
	e.errorCode(muter, "POST", "sanctions", csrf(muter), sanctionReq("kick", 7, 0, 0, "x"), http.StatusForbidden, "forbidden")
	e.errorCode(reader, "POST", "sanctions", csrf(reader), sanctionReq("mute", 7, 0, 5, "x"), http.StatusForbidden, "forbidden")
	e.errorCode(reader, "GET", "sanctions", "", nil, http.StatusForbidden, "forbidden")

	// Bad bodies.
	for _, r := range []*adminv1.SanctionRequest{
		sanctionReq("warn", 7, 0, 5, "x"),
		sanctionReq("mute", 7, 0, 5, ""),
		sanctionReq("mute", 7, 0, 5, strings.Repeat("r", 256)),
		sanctionReq("mute", 7, 0, 0, "x"),
		sanctionReq("mute", 7, 0, 525601, "x"),
		sanctionReq("jail", 0, 0, 5, "x"),
		sanctionReq("mute", 7, 9, 5, "x"),
		sanctionReq("kick", 7, 0, 5, "x"),
		sanctionReq("ban", 7, 0, 5, "x"),
		sanctionReq("ban", 0, 9, -1, "x"),
		sanctionReq("ban", 0, 9, 5256001, "x"),
	} {
		e.errorCode(adm, "POST", "sanctions", csrf(adm), r, http.StatusBadRequest, "bad_request")
	}
	e.errorCode(adm, "POST", "sanctions", "", sanctionReq("mute", 7, 0, 5, "x"), http.StatusForbidden, "csrf")
	// A kick of a character that is not online.
	e.errorCode(mod, "POST", "sanctions", csrf(mod), sanctionReq("kick", 404, 0, 0, "x"), http.StatusNotFound, "not_found")

	// A ban needs a login within 10 minutes. A mute does not.
	if _, err := e.admin.Exec(context.Background(), "UPDATE account_sessions SET authenticated_at = now() - interval '11 minutes'"); err != nil {
		t.Fatal(err)
	}
	if got := e.errorCode(gm, "POST", "sanctions", csrf(gm), sanctionReq("ban", 0, 9, 60, "x"), http.StatusForbidden, "reauth"); got.GetReauthUrl() == "" {
		t.Error("no reauth URL")
	}
	if status, b := e.call(mod, "POST", "sanctions", csrf(mod), sanctionReq("mute", 7, 0, 5, "x")); status != http.StatusOK {
		t.Errorf("a stale mute: %d %s", status, b)
	}

	// Lifting needs the kind's permission, a reason, and for a ban a
	// recent login.
	e.world.mu.Lock()
	e.world.sanctionKinds = map[int64]string{1: "mute", 2: "jail", 3: "ban", 4: "kick"}
	e.world.mu.Unlock()
	status, b := e.call(muter, "DELETE", "sanctions/1?reason="+url.QueryEscape("appealed"), csrf(muter), nil)
	if status != http.StatusOK || !strings.Contains(string(b), "appealed") {
		t.Errorf("lift: %d %s", status, b)
	}
	e.errorCode(muter, "DELETE", "sanctions/2?reason=x", csrf(muter), nil, http.StatusForbidden, "forbidden")
	e.errorCode(muter, "DELETE", "sanctions/1", csrf(muter), nil, http.StatusBadRequest, "bad_request")
	e.errorCode(muter, "DELETE", "sanctions/9?reason=x", csrf(muter), nil, http.StatusNotFound, "not_found")
	e.errorCode(adm, "DELETE", "sanctions/4?reason=x", csrf(adm), nil, http.StatusBadRequest, "bad_request")
	e.errorCode(gm, "DELETE", "sanctions/3?reason=x", csrf(gm), nil, http.StatusForbidden, "reauth")
	e.errorCode(muter, "DELETE", "sanctions/1?reason=x", "", nil, http.StatusForbidden, "csrf")
	fresh := e.loginTOTP("Gamemaster")
	if status, b := e.call(fresh, "DELETE", "sanctions/3?reason=x", csrf(fresh), nil); status != http.StatusOK {
		t.Errorf("lift a ban: %d %s", status, b)
	}
	e.world.mu.Lock()
	lifts := e.world.lifts
	e.world.mu.Unlock()
	if len(lifts) != 2 || lifts[0].ID != 1 || lifts[0].Reason != "appealed" || lifts[1].AccountName != "Gamemaster" {
		t.Errorf("lifts %+v", lifts)
	}

	// The list: filters and refusals.
	list := get(e, muter, "sanctions?character=7&kind=mute&active=true&limit=5&before_id=10", &adminv1.SanctionList{})
	if len(list.GetSanctions()) != 1 {
		t.Errorf("list %v", list)
	}
	get(e, mod, "sanctions?account=9", &adminv1.SanctionList{})
	e.world.mu.Lock()
	sq := e.world.sanctionQueries
	e.world.mu.Unlock()
	if len(sq) != 2 || sq[0] != (admin.SanctionQuery{Character: 7, Kind: "mute", Active: true, BeforeID: 10, Limit: 5}) ||
		sq[1] != (admin.SanctionQuery{Account: 9, Limit: admin.DefaultSanctions}) {
		t.Errorf("sanction queries %+v", sq)
	}
	for _, q := range []string{"character=x", "account=-1", "kind=warn", "active=yes", "limit=101", "before_id=0"} {
		e.errorCode(mod, "GET", "sanctions?"+q, "", nil, http.StatusBadRequest, "bad_request")
	}
}

// TestBansInSearch: the search and the character details show the
// account's ban.
func TestBansInSearch(t *testing.T) {
	e := setup(t, options{})
	adm := e.withRole("Admin", "admin")
	e.signup("Banned")
	e.character("Banned", "Badguy")
	e.world.mu.Lock()
	e.world.banned = map[int64]bool{e.accountID("Banned"): true}
	e.world.mu.Unlock()
	p := get(e, adm, "players?name=Badguy", &adminv1.PlayerSearch{})
	if len(p.GetAccounts()) != 1 || p.GetAccounts()[0].GetBan().GetKind() != "ban" {
		t.Errorf("search %v", p)
	}
}

// TestBannedLogin: a banned account cannot log in or get a game ticket,
// and sees the reason and the end time. A lifted or ended ban lets it
// in again.
func TestBannedLogin(t *testing.T) {
	e := setup(t, options{})
	ctx := context.Background()
	c := e.signup("Banned")
	e.character("Banned", "Badguy")
	if _, err := e.admin.Exec(ctx, "UPDATE account_sessions SET character_id = (SELECT id FROM characters WHERE name = 'Badguy')"); err != nil {
		t.Fatal(err)
	}
	ticket := func(c *http.Client) (int, string) {
		req, _ := http.NewRequest("POST", e.srv.URL+"/account/ticket", nil)
		req.Header.Set("Origin", "http://client.test")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	if status, body := ticket(c); status != http.StatusOK {
		t.Fatalf("ticket before the ban: %d %s", status, body)
	}
	end := time.Now().Add(48 * time.Hour).UTC()
	var banID int64
	if err := e.game.QueryRow(ctx, `INSERT INTO sanctions (account_id, kind, reason, started_at, ends_at, issued_by)
		SELECT id, 'ban', 'scamming players', now(), $1, 'Admin' FROM accounts WHERE name = 'Banned' RETURNING id`, end).Scan(&banID); err != nil {
		t.Fatal(err)
	}
	status, body := ticket(c)
	if status != http.StatusForbidden || !strings.Contains(body, `"error":"banned"`) || !strings.Contains(body, "scamming players") ||
		!strings.Contains(body, end.Format("2006-01-02 15:04")) {
		t.Errorf("ticket of a banned account: %d %s", status, body)
	}
	status, body = e.submit(e.browser(), "/account/login", "/account/login", url.Values{"name": {"Banned"}, "password": {password}})
	if status != http.StatusForbidden || !strings.Contains(body, "scamming players") || !strings.Contains(body, end.Format("2006-01-02 15:04")) {
		t.Errorf("login of a banned account: %d %s", status, body)
	}
	// A wrong password says nothing of the ban.
	status, body = e.submit(e.browser(), "/account/login", "/account/login", url.Values{"name": {"Banned"}, "password": {"wrong password!!"}})
	if status != http.StatusBadRequest || strings.Contains(body, "scamming") {
		t.Errorf("wrong password of a banned account: %d %s", status, body)
	}
	// With TOTP on, the ban shows once the password is right, before the
	// code.
	e.enableTOTP(c, "Banned")
	b := e.browser()
	if status, body := e.submit(b, "/account/login", "/account/login", url.Values{"name": {"Banned"}, "password": {password}}); status != http.StatusForbidden {
		t.Errorf("TOTP login of a banned account: %d %s", status, body)
	}
	// A permanent ban says so.
	if _, err := e.game.Exec(ctx, "UPDATE sanctions SET ends_at = 'infinity' WHERE id = $1", banID); err != nil {
		t.Fatal(err)
	}
	if status, body := ticket(c); status != http.StatusForbidden || !strings.Contains(body, "banned permanently") || !strings.Contains(body, `"ends_at":""`) {
		t.Errorf("ticket of a permanent ban: %d %s", status, body)
	}
	// A lifted ban lets the account in.
	if _, err := e.game.Exec(ctx, "UPDATE sanctions SET lifted_at = now(), lifted_by = 'Admin', lift_reason = 'appeal' WHERE id = $1", banID); err != nil {
		t.Fatal(err)
	}
	if status, body := ticket(c); status != http.StatusOK {
		t.Errorf("ticket after the lift: %d %s", status, body)
	}
	// The account role reads bans, and nothing else of sanctions.
	acct, err := e.admin.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = acct.Rollback(ctx) }()
	if _, err := acct.Exec(ctx, "SET LOCAL ROLE wathena_account"); err != nil {
		t.Fatal(err)
	}
	if _, err := acct.Exec(ctx, "SELECT issued_by FROM sanctions"); err == nil {
		t.Error("the account role read issued_by")
	}
}
