package account_test

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/account"
	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

// gameEnv is an account service with a gateway, whose Ended closes the
// game connections of ended sessions.
func gameEnv(t *testing.T, change func(*account.Config)) (*env, string) {
	t.Helper()
	g := &path.Grid{Width: 20, Height: 20, Cells: make([]uint8, 400)}
	for i := range g.Cells {
		g.Cells[i] = path.Walkable | path.Shootable
	}
	z := zone.New("map/test", g, rng.FromKey([32]byte{}).Stream(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go z.Run(ctx)
	gw := &gateway.Gateway{World: z, Origins: []string{clientOrigin}}
	e := setup(t, func(c *account.Config) {
		c.Ended = gw
		if change != nil {
			change(c)
		}
	})
	gw.Sessions = account.GameTickets{Store: e.store}
	gw.Characters = storeCharacters{e.store}
	return e, newGateway(t, gw)
}

// loggedOut reports whether a browser's session ended: the characters
// page sends it to the login page.
func (e *env) loggedOut(c *http.Client) bool {
	e.t.Helper()
	resp, err := c.Get(e.srv.URL + "/account/characters")
	if err != nil {
		e.t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusSeeOther && strings.HasPrefix(resp.Header.Get("Location"), "/account/login")
}

// TestLogOutEverywhere: the settings page's button ends every other
// session of the account and closes the game connections they opened.
// The session that pressed it stays. It needs a recent login, so a
// stolen cookie cannot use it (review decision 51).
func TestLogOutEverywhere(t *testing.T) {
	e, gw := gameEnv(t, nil)
	thief := e.c
	e.signup(thief, "Victim_1", password)
	e.makeCharacter(thief, "Victim Hero", "0")
	e.play(thief, "0")
	_, ticket := e.ticket(thief, clientOrigin)
	game := gameConn(t, gw, ticket)

	owner := e.newClient()
	if resp, _ := e.login(owner, "Victim_1", password, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: %d", resp.StatusCode)
	}
	if body, _ := e.page(owner, "/account/settings"); !strings.Contains(body, "1 other session is logged in.") {
		t.Errorf("settings page: %s", body)
	}

	// A session whose login is old is sent to the reauth page first.
	e.ageSessions()
	resp, _ := e.submit(thief, "/account/settings", "/account/settings/sessions/end", url.Values{})
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/account/reauth") {
		t.Fatalf("stale end: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if e.loggedOut(owner) {
		t.Fatal("a stale session logged the owner out")
	}

	// The owner proves who they are and ends the thief's session.
	if resp, body := e.submit(owner, "/account/reauth", "/account/reauth", url.Values{"password": {password}}); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("reauth: %d %s", resp.StatusCode, body)
	}
	resp, body := e.submit(owner, "/account/settings", "/account/settings/sessions/end", url.Values{})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/settings?notice=sessions-ended" {
		t.Fatalf("end: %d %s %s", resp.StatusCode, resp.Header.Get("Location"), body)
	}
	if r := disconnectReason(t, game); r != wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_OUT {
		t.Errorf("the thief's game ended with %v", r)
	}
	if !e.loggedOut(thief) {
		t.Error("the thief's session still works")
	}
	if e.loggedOut(owner) {
		t.Error("the owner's own session ended")
	}
	if body, _ := e.page(owner, "/account/settings"); !strings.Contains(body, "No other session is logged in.") {
		t.Errorf("settings page after: %s", body)
	}
}

// TestPasswordChangeEndsOtherSessions: a new password needs the current
// one, and ends the other sessions. The old password stops working.
func TestPasswordChangeEndsOtherSessions(t *testing.T) {
	e, _ := gameEnv(t, nil)
	owner := e.c
	e.signup(owner, "Changer_1", password)
	other := e.newClient()
	if resp, _ := e.login(other, "Changer_1", password, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: %d", resp.StatusCode)
	}
	const newPassword = "another fine long password"
	for _, c := range []struct {
		name string
		form url.Values
		want int
	}{
		{"wrong current password", url.Values{"password": {"not the password"}, "new_password": {newPassword}, "confirm": {newPassword}}, http.StatusBadRequest},
		{"different confirm", url.Values{"password": {password}, "new_password": {newPassword}, "confirm": {newPassword + "x"}}, http.StatusBadRequest},
		{"short", url.Values{"password": {password}, "new_password": {"short"}, "confirm": {"short"}}, http.StatusBadRequest},
	} {
		if resp, _ := e.submit(owner, "/account/settings", "/account/settings/password", c.form); resp.StatusCode != c.want {
			t.Errorf("%s: %d", c.name, resp.StatusCode)
		}
	}
	if e.loggedOut(other) {
		t.Fatal("a refused change ended the other session")
	}
	resp, body := e.submit(owner, "/account/settings", "/account/settings/password",
		url.Values{"password": {password}, "new_password": {newPassword}, "confirm": {newPassword}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/settings?notice=password-changed" {
		t.Fatalf("change: %d %s %s", resp.StatusCode, resp.Header.Get("Location"), body)
	}
	if !e.loggedOut(other) {
		t.Error("the other session survived the password change")
	}
	if e.loggedOut(owner) {
		t.Error("the session that changed the password ended")
	}
	fresh := e.newClient()
	if resp, _ := e.login(fresh, "Changer_1", password, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("the old password still logs in: %d", resp.StatusCode)
	}
	if resp, _ := e.login(fresh, "Changer_1", newPassword, nil); resp.StatusCode != http.StatusSeeOther {
		t.Errorf("the new password does not log in: %d", resp.StatusCode)
	}
}

// TestLoginMethodChangesEndOtherSessions: turning TOTP on, new recovery
// codes and turning TOTP off each end the other sessions.
func TestLoginMethodChangesEndOtherSessions(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	e, _ := gameEnv(t, twoFactor(t, clock))
	owner := e.c
	e.signup(owner, "Methods_1", password)
	second := func() *http.Client {
		t.Helper()
		c := e.newClient()
		if resp, _ := e.login(c, "Methods_1", password, nil); resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("login: %d", resp.StatusCode)
		}
		return c
	}
	other := second()
	secret, _ := e.enableTOTP(owner, clock)
	if !e.loggedOut(other) {
		t.Error("the other session survived turning TOTP on")
	}

	// The second browser now needs a code too.
	other = e.newClient()
	if resp, _ := e.login(other, "Methods_1", password, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: %d", resp.StatusCode)
	}
	clock.step()
	if resp, body := e.sendCode(other, code(secret, clock.Now())); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("code: %d %s", resp.StatusCode, body)
	}
	resp, body := e.submit(owner, "/account/settings", "/account/settings/recovery", url.Values{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("new recovery codes: %d %s", resp.StatusCode, body)
	}
	var codes []string
	for _, m := range recoveryField.FindAllStringSubmatch(body, -1) {
		codes = append(codes, m[1])
	}
	if !e.loggedOut(other) {
		t.Error("the other session survived new recovery codes")
	}

	other = e.newClient()
	if resp, _ := e.login(other, "Methods_1", password, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: %d", resp.StatusCode)
	}
	if resp, body := e.sendCode(other, codes[0]); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("recovery code: %d %s", resp.StatusCode, body)
	}
	clock.step()
	resp, body = e.submit(owner, "/account/settings", "/account/settings/totp/disable",
		url.Values{"password": {password}, "code": {code(secret, clock.Now())}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("disable: %d %s", resp.StatusCode, body)
	}
	if !e.loggedOut(other) {
		t.Error("the other session survived turning TOTP off")
	}
	if e.loggedOut(owner) {
		t.Error("the session that made the changes ended")
	}
}

// TestTicketRecordsRecentLogin: a ticket made within the reauth window
// of the session's login says so, for the GM commands that need a
// recent login. A ticket of an older login does not (review decision
// 51).
func TestTicketRecordsRecentLogin(t *testing.T) {
	e := setup(t, nil)
	e.signup(e.c, "Recent_1", password)
	e.makeCharacter(e.c, "Recent Hero", "0")
	e.play(e.c, "0")
	_, ticket := e.ticket(e.c, clientOrigin)
	l, err := e.redeem(ticket)
	if err != nil || !l.Staff.Recent {
		t.Fatalf("fresh ticket: %+v %v", l.Staff, err)
	}
	e.ageSessions()
	_, ticket = e.ticket(e.c, clientOrigin)
	l, err = e.redeem(ticket)
	if err != nil || l.Staff.Recent {
		t.Errorf("ticket of an old login: %+v %v", l.Staff, err)
	}
}
