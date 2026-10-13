package admin_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	"github.com/lxsmnsyc/wathena/server/admin"
)

// auditActions lists the audit rows of some actions, as "account action
// target reason".
func (e *env) auditActions(prefix string) []string {
	e.t.Helper()
	rows, err := e.admin.Query(context.Background(),
		"SELECT account_name || ' ' || action || ' ' || target || ' ' || reason FROM staff_audit WHERE action LIKE $1 ORDER BY id", prefix+"%")
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, strings.TrimSpace(s))
	}
	return out
}

// TestWorldCalls: GET /world and POST /announce with their permissions,
// checks and audit rows (decision 0031).
func TestWorldCalls(t *testing.T) {
	e := setup(t, options{limits: manyCalls})
	gm := e.withRole("Gamemaster", "game_master")
	mod := e.withRole("Moddy", "moderator")
	csrf := e.me(gm).GetCsrfToken()

	v := get(e, gm, "world", &adminv1.WorldView{})
	if len(v.GetZones()) != 1 || v.GetZones()[0].GetMap() != "prontera" || v.GetPlayersOnline() != 1 {
		t.Errorf("world %v", v)
	}
	e.errorCode(mod, "GET", "world", "", nil, http.StatusForbidden, "forbidden")
	e.errorCode(e.browser(), "GET", "world", "", nil, http.StatusUnauthorized, "login")

	announce := func(text, m string) *adminv1.AnnounceRequest {
		return adminv1.AnnounceRequest_builder{Text: text, Map: m}.Build()
	}
	for _, r := range []*adminv1.AnnounceRequest{announce("Welcome!", ""), announce(" Market day in town ", "prontera")} {
		if status, b := e.call(gm, "POST", "announce", csrf, r); status != http.StatusOK {
			t.Errorf("announce: %d %s", status, b)
		}
	}
	for _, r := range []*adminv1.AnnounceRequest{announce("", ""), announce("  ", ""), announce(strings.Repeat("a", 256), ""),
		announce("two\nlines", ""), announce("hi", "pront era"), announce("hi", strings.Repeat("m", 65))} {
		e.errorCode(gm, "POST", "announce", csrf, r, http.StatusBadRequest, "bad_request")
	}
	e.errorCode(gm, "POST", "announce", csrf, announce("hi", "nowhere"), http.StatusNotFound, "not_found")
	e.errorCode(gm, "POST", "announce", "", announce("hi", ""), http.StatusForbidden, "csrf")
	e.errorCode(mod, "POST", "announce", e.me(mod).GetCsrfToken(), announce("hi", ""), http.StatusForbidden, "forbidden")
	e.world.mu.Lock()
	said := strings.Join(e.world.announcements, "|")
	e.world.mu.Unlock()
	if said != ": Welcome!|prontera: Market day in town" {
		t.Errorf("announcements %q", said)
	}
	if got := strings.Join(e.auditActions("world.announce"), "|"); got != "Gamemaster world.announce world|Gamemaster world.announce prontera" {
		t.Errorf("audit rows %q", got)
	}
}

// TestRestartCalls: POST and DELETE /restart need world.restart and a
// recent login, and write audit rows.
func TestRestartCalls(t *testing.T) {
	e := setup(t, options{limits: manyCalls})
	adm := e.withRole("Admin", "admin")
	gm := e.withRole("Gamemaster", "game_master")
	csrf := e.me(adm).GetCsrfToken()
	at := time.Now().Add(30 * time.Minute).Truncate(time.Second)
	plan := func(at time.Time, reason, message string) *adminv1.RestartRequest {
		r := adminv1.RestartRequest_builder{Reason: reason, Message: message}.Build()
		if !at.IsZero() {
			r.SetAt(timestamppb.New(at))
		}
		return r
	}
	status, b := e.call(adm, "POST", "restart", csrf, plan(at, "maintenance", "New maps arrive!"))
	if status != http.StatusOK || !strings.Contains(string(b), `"plannedBy":"Admin"`) {
		t.Fatalf("plan: %d %s", status, b)
	}
	e.errorCode(adm, "POST", "restart", csrf, plan(at, "again", ""), http.StatusConflict, "conflict")
	for _, r := range []*adminv1.RestartRequest{plan(time.Time{}, "x", ""), plan(at, "", ""), plan(at, "  ", ""),
		plan(at, strings.Repeat("r", 256), ""), plan(at, "x", strings.Repeat("m", 256)), plan(at, "x", "two\nlines")} {
		e.errorCode(adm, "POST", "restart", csrf, r, http.StatusBadRequest, "bad_request")
	}
	if v := get(e, adm, "world", &adminv1.WorldView{}); v.GetRestart().GetReason() != "maintenance" {
		t.Errorf("the world view has no restart: %v", v)
	}
	e.errorCode(gm, "POST", "restart", e.me(gm).GetCsrfToken(), plan(at, "x", ""), http.StatusForbidden, "forbidden")
	e.errorCode(gm, "DELETE", "restart", e.me(gm).GetCsrfToken(), nil, http.StatusForbidden, "forbidden")
	e.errorCode(adm, "DELETE", "restart", "", nil, http.StatusForbidden, "csrf")
	e.errorCode(adm, "DELETE", "restart?reason="+strings.Repeat("r", 256), csrf, nil, http.StatusBadRequest, "bad_request")
	if status, b := e.call(adm, "DELETE", "restart?reason="+url.QueryEscape("done early"), csrf, nil); status != http.StatusOK {
		t.Fatalf("cancel: %d %s", status, b)
	}
	e.errorCode(adm, "DELETE", "restart", csrf, nil, http.StatusNotFound, "not_found")
	// The world refuses a time too soon.
	e.errorCode(adm, "POST", "restart", csrf, plan(time.Now(), "x", ""), http.StatusBadRequest, "bad_request")

	// Both need a login within 10 minutes.
	if _, err := e.admin.Exec(context.Background(), "UPDATE account_sessions SET authenticated_at = now() - interval '11 minutes'"); err != nil {
		t.Fatal(err)
	}
	if got := e.errorCode(adm, "POST", "restart", csrf, plan(at, "x", ""), http.StatusForbidden, "reauth"); got.GetReauthUrl() == "" {
		t.Error("no reauth URL")
	}
	e.errorCode(adm, "DELETE", "restart", csrf, nil, http.StatusForbidden, "reauth")
	if got := strings.Join(e.auditActions("world.restart"), "|"); got != "Admin world.restart world maintenance|Admin world.restart_cancel world done early" {
		t.Errorf("audit rows %q", got)
	}
}

// TestEventCalls: GET /events, the start, stop and reset of an event,
// and PATCH /events/{name}, with events.control.
func TestEventCalls(t *testing.T) {
	e := setup(t, options{limits: manyCalls})
	gm := e.withRole("Gamemaster", "game_master")
	mod := e.withRole("Moddy", "moderator")
	csrf := e.me(gm).GetCsrfToken()
	list := get(e, gm, "events", &adminv1.EventList{})
	if len(list.GetEvents()) != 1 || list.GetEvents()[0].GetTriggerText() != "daily at 20:00" {
		t.Errorf("events %v", list)
	}
	action := func(reason string) *adminv1.EventAction { return adminv1.EventAction_builder{Reason: reason}.Build() }
	name := url.PathEscape("core:Field_Boss")
	for _, a := range []string{"start", "stop", "reset"} {
		if status, b := e.call(gm, "POST", "events/"+name+"/"+a, csrf, action("weekend fun")); status != http.StatusOK {
			t.Errorf("%s: %d %s", a, status, b)
		}
	}
	trigger, err := structpb.NewStruct(map[string]any{"weekly": map[string]any{"day": "saturday", "time": "21:00"}})
	if err != nil {
		t.Fatal(err)
	}
	change := adminv1.EventChange_builder{On: new(bool), Trigger: trigger, DurationMinutes: 90, Reason: "later this week"}.Build()
	if status, b := e.call(gm, "PATCH", "events/"+name, csrf, change); status != http.StatusOK {
		t.Errorf("change: %d %s", status, b)
	}
	e.world.mu.Lock()
	orders := e.world.eventOrders
	e.world.mu.Unlock()
	if len(orders) != 4 || orders[0].Action != admin.EventStart || orders[1].Action != admin.EventStop || orders[2].Action != admin.EventReset ||
		orders[0].Reason != "weekend fun" || orders[0].AccountName != "Gamemaster" {
		t.Fatalf("orders %+v", orders)
	}
	if c := orders[3]; c.Action != admin.EventChange || c.On == nil || *c.On || c.DurationMinutes != 90 ||
		c.Trigger["weekly"].(map[string]any)["day"] != "saturday" {
		t.Errorf("change %+v", c)
	}
	for _, c := range []*adminv1.EventChange{
		adminv1.EventChange_builder{}.Build(),
		adminv1.EventChange_builder{DurationMinutes: 10081}.Build(),
		adminv1.EventChange_builder{DurationMinutes: -1}.Build(),
		adminv1.EventChange_builder{DurationMinutes: 5, Reason: strings.Repeat("r", 256)}.Build(),
	} {
		e.errorCode(gm, "PATCH", "events/"+name, csrf, c, http.StatusBadRequest, "bad_request")
	}
	e.errorCode(gm, "POST", "events/"+name+"/start", csrf, action(strings.Repeat("r", 256)), http.StatusBadRequest, "bad_request")
	e.errorCode(gm, "POST", "events/"+url.PathEscape("example.none:Nothing")+"/start", csrf, action(""), http.StatusNotFound, "not_found")
	e.errorCode(gm, "POST", "events/"+name+"/start", "", action(""), http.StatusForbidden, "csrf")
	e.errorCode(mod, "GET", "events", "", nil, http.StatusForbidden, "forbidden")
	e.errorCode(mod, "POST", "events/"+name+"/stop", e.me(mod).GetCsrfToken(), action(""), http.StatusForbidden, "forbidden")
}

// TestModCalls: GET /mods with mods.read, and turning a mod off and on
// with mods.disable.
func TestModCalls(t *testing.T) {
	e := setup(t, options{limits: manyCalls})
	adm := e.withRole("Admin", "admin")
	gm := e.withRole("Gamemaster", "game_master")
	csrf := e.me(adm).GetCsrfToken()
	list := get(e, adm, "mods", &adminv1.ModList{})
	if len(list.GetMods()) != 2 || list.GetMods()[1].GetErrors() != 2 {
		t.Errorf("mods %v", list)
	}
	sw := func(m, reason string) *adminv1.ModSwitchRequest {
		return adminv1.ModSwitchRequest_builder{Map: m, Reason: reason}.Build()
	}
	if status, b := e.call(adm, "POST", "mods/example.a/disable", csrf, sw("prontera", "quest gives too much")); status != http.StatusOK {
		t.Errorf("disable: %d %s", status, b)
	}
	if status, b := e.call(adm, "POST", "mods/example.a/enable", csrf, sw("", "")); status != http.StatusOK {
		t.Errorf("enable: %d %s", status, b)
	}
	e.world.mu.Lock()
	orders := e.world.modOrders
	e.world.mu.Unlock()
	if len(orders) != 2 || !orders[0].Off || orders[0].Map != "prontera" || orders[0].Reason != "quest gives too much" ||
		orders[1].Off || orders[1].Map != "" || orders[0].AccountName != "Admin" {
		t.Errorf("orders %+v", orders)
	}
	e.errorCode(adm, "POST", "mods/example.a/disable", csrf, sw("", ""), http.StatusBadRequest, "bad_request")
	e.errorCode(adm, "POST", "mods/core/disable", csrf, sw("", "x"), http.StatusBadRequest, "bad_request")
	e.errorCode(adm, "POST", "mods/Not!A!Mod/disable", csrf, sw("", "x"), http.StatusBadRequest, "bad_request")
	e.errorCode(adm, "POST", "mods/example.a/disable", csrf, sw("a b", "x"), http.StatusBadRequest, "bad_request")
	e.errorCode(adm, "POST", "mods/example.a/enable", csrf, sw("", strings.Repeat("r", 256)), http.StatusBadRequest, "bad_request")
	e.errorCode(adm, "POST", "mods/example.b/disable", csrf, sw("", "x"), http.StatusNotFound, "not_found")
	e.errorCode(adm, "POST", "mods/example.a/disable", "", sw("", "x"), http.StatusForbidden, "csrf")
	e.errorCode(gm, "GET", "mods", "", nil, http.StatusForbidden, "forbidden")
	e.errorCode(gm, "POST", "mods/example.a/disable", e.me(gm).GetCsrfToken(), sw("", "x"), http.StatusForbidden, "forbidden")
}

// TestToolsRateLimit: the tools' calls count toward the admin rate
// limit, as every admin call does.
func TestToolsRateLimit(t *testing.T) {
	e := setup(t, options{})
	adm := e.withRole("Admin", "admin")
	limited := false
	for range 200 {
		status, _ := e.call(adm, "GET", "world", "", nil)
		if status == http.StatusTooManyRequests {
			limited = true
			break
		}
	}
	if !limited {
		t.Error("GET /world was never limited")
	}
}
