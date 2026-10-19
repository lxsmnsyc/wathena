package admin_test

import (
	"bytes"
	"context"
	"encoding/base32"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	"github.com/lxsmnsyc/wathena/server/account"
	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/staff"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/db"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
)

const password = "a fine long password"

// clock is the time TOTP codes are checked against. Each login moves it
// on by a step, since a code works once.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) step() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(30 * time.Second)
}

// world records the role changes the API passes on, and answers the
// game calls of step 2 with what a test set.
type world struct {
	mu      sync.Mutex
	changes []string
	// online are the characters online, with their maps.
	online map[int64]string
	// holder is where FindSerial says a serial is.
	holder *adminv1.Owner
	// history is what ItemHistory answers, nil for not found.
	history *adminv1.ItemHistory
	// logs are the LogQuery calls, and restores the Restore calls.
	logs     []admin.LogQuery
	restores []admin.Restore
	// restoreErr is what Restore fails with.
	restoreErr error
	// The moderation calls, and the sanctions SanctionByID knows by ID,
	// and the banned accounts of Bans.
	reportQueries   []admin.ReportQuery
	closes          []admin.CloseReport
	orders          []admin.SanctionOrder
	lifts           []admin.Lift
	sanctionQueries []admin.SanctionQuery
	sanctionKinds   map[int64]string
	banned          map[int64]bool
	// The tools' calls of step 4 (decision 0031): announcements as
	// "map: text", the planned restart, and the event and mod orders.
	announcements []string
	restart       *admin.RestartOrder
	eventOrders   []admin.EventOrder
	changeOrders  []admin.ChangeOrder
	modOrders     []admin.ModOrder
}

func (w *world) StaffChanged(_ context.Context, _ int64, role string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.changes = append(w.changes, role)
	return nil
}

func (w *world) got() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.changes...)
}

type env struct {
	t       *testing.T
	admin   *pgxpool.Pool
	game    *pgxpool.Pool
	clock   *clock
	world   *world
	srv     *httptest.Server
	secrets map[string][]byte
	handler *admin.Handler
}

type options struct {
	roles  map[string][]string
	owners []string
	limits func(*account.Limits)
	// retention is GET /retention's job, or nil for none.
	retention admin.Retention
	// loginIPDays is retention.login_ip_days, or 0 for the default.
	loginIPDays int
	// founder makes the account Founder before the handler, with the
	// password, as the world's owner by account ID (review decision 54).
	founder bool
}

func setup(t *testing.T, o options) *env {
	t.Helper()
	e := &env{t: t, clock: &clock{now: time.Now()}, world: &world{}, secrets: map[string][]byte{}}
	e.admin = storetest.Migrated(t)
	acct := storetest.As(t, e.admin, "wathena_account")
	e.game = storetest.As(t, e.admin, "wathena_game")
	sealer, err := account.NewSealer(bytes.Repeat([]byte{7}, account.SecretKeyLen))
	if err != nil {
		t.Fatal(err)
	}
	limits := account.DefaultLimits
	limits.Login = account.Bucket{Burst: 1000, Every: time.Millisecond}
	limits.Signup = limits.Login
	if o.limits != nil {
		o.limits(&limits)
	}
	hasher := account.NewHasher(account.Params{Memory: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}, 2)
	var ownerAccount int64
	if o.founder {
		hash, err := hasher.Hash(context.Background(), password)
		if err != nil {
			t.Fatal(err)
		}
		if ownerAccount, err = db.New(e.admin).CreateAdminAccount(context.Background(), db.CreateAdminAccountParams{Name: "Founder", Hash: hash}); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := account.New(account.Config{
		Pool: acct, ClientOrigins: []string{"http://client.test"}, Hasher: hasher, OwnerAccount: ownerAccount,
		NewCharacter: func(name string, look account.Look) character.Character {
			return character.Character{Info: character.Info{Name: name, Male: look.Male, Hair: look.Hair, HairColor: look.HairColor,
				BaseLevel: 1, JobLevel: 1, Stats: status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1},
				HP: 40, SP: 11, Map: "map/test", X: 5, Y: 5, InventorySize: 100}}
		},
		Log: slog.New(slog.DiscardHandler), Sealer: sealer, Clock: e.clock.Now, Limits: limits,
	})
	if err != nil {
		t.Fatal(err)
	}
	roles, err := staff.ParseRoles(o.roles)
	if err != nil {
		t.Fatal(err)
	}
	owners := o.owners
	if owners == nil {
		owners = []string{"Owner"}
	}
	h, err := admin.New(admin.Config{Pool: acct, Auth: svc, World: e.world, Roles: roles, Owners: owners, OwnerAccount: ownerAccount, Retention: o.retention,
		LoginIPDays: o.loginIPDays, Log: slog.New(slog.DiscardHandler)})
	if err != nil {
		t.Fatal(err)
	}
	e.handler = h
	mux := http.NewServeMux()
	mux.Handle(account.Prefix, svc)
	mux.Handle(admin.Prefix, h)
	e.srv = httptest.NewServer(mux)
	t.Cleanup(e.srv.Close)
	return e
}

// browser keeps cookies and does not follow redirects.
func (e *env) browser() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

var (
	csrfField = regexp.MustCompile(`name="csrf" value="([^"]+)"`)
	keyField  = regexp.MustCompile(`<p class="key"><code>([A-Z2-7 ]+)</code>`)
)

// submit loads a page for its CSRF token, then posts a form.
func (e *env) submit(c *http.Client, from, path string, form url.Values) (int, string) {
	e.t.Helper()
	resp, err := c.Get(e.srv.URL + from)
	if err != nil {
		e.t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	m := csrfField.FindStringSubmatch(string(b))
	if m == nil {
		e.t.Fatalf("%s has no form: %s", from, b)
	}
	form.Set("csrf", m[1])
	resp, err = c.PostForm(e.srv.URL+path, form)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ = io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Location") + " " + string(b)
}

func code(secret []byte, at time.Time) string {
	s, err := totp.GenerateCodeCustom(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), at,
		totp.ValidateOpts{Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		panic(err)
	}
	return s
}

// signup makes an account and returns its browser, logged in with the
// password alone.
func (e *env) signup(name string) *http.Client {
	e.t.Helper()
	c := e.browser()
	if status, body := e.submit(c, "/account/signup", "/account/signup", url.Values{"name": {name}, "password": {password}, "confirm": {password}}); status != http.StatusSeeOther {
		e.t.Fatalf("signup %s: %d %s", name, status, body)
	}
	return c
}

// enableTOTP turns TOTP on for the browser's account.
func (e *env) enableTOTP(c *http.Client, name string) {
	e.t.Helper()
	if status, body := e.submit(c, "/account/settings", "/account/settings/totp/start", url.Values{}); status != http.StatusSeeOther {
		e.t.Fatalf("totp start: %d %s", status, body)
	}
	resp, err := c.Get(e.srv.URL + "/account/settings/totp")
	if err != nil {
		e.t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	m := keyField.FindStringSubmatch(string(b))
	if m == nil {
		e.t.Fatalf("totp page: %s", b)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ReplaceAll(m[1], " ", ""))
	if err != nil {
		e.t.Fatal(err)
	}
	e.clock.step()
	if status, body := e.submit(c, "/account/settings/totp", "/account/settings/totp", url.Values{"code": {code(secret, e.clock.Now())}}); status != http.StatusOK {
		e.t.Fatalf("totp confirm: %d %s", status, body)
	}
	e.secrets[name] = secret
}

// loginTOTP logs in with the password and a TOTP code in a new browser.
func (e *env) loginTOTP(name string) *http.Client {
	e.t.Helper()
	c := e.browser()
	if status, body := e.submit(c, "/account/login", "/account/login", url.Values{"name": {name}, "password": {password}}); status != http.StatusSeeOther || !strings.HasPrefix(body, "/account/login/code") {
		e.t.Fatalf("login %s: %d %s", name, status, body)
	}
	e.clock.step()
	if status, body := e.submit(c, "/account/login/code", "/account/login/code", url.Values{"code": {code(e.secrets[name], e.clock.Now())}}); status != http.StatusSeeOther {
		e.t.Fatalf("code %s: %d %s", name, status, body)
	}
	return c
}

// staffer makes an account with TOTP on and returns a browser logged in
// with it.
func (e *env) staffer(name string) *http.Client {
	e.t.Helper()
	e.enableTOTP(e.signup(name), name)
	return e.loginTOTP(name)
}

// call calls the admin API. csrf "" sends no token.
func (e *env) call(c *http.Client, method, path, csrf string, body proto.Message) (int, []byte) {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := protojson.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, e.srv.URL+admin.Prefix+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if csrf != "" {
		req.Header.Set(admin.CSRFHeader, csrf)
	}
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if ct := resp.Header.Get("Content-Type"); resp.StatusCode != http.StatusNoContent && ct != "application/json" {
		e.t.Errorf("%s %s: Content-Type %q", method, path, ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		e.t.Errorf("%s %s: Cache-Control %q", method, path, cc)
	}
	return resp.StatusCode, b
}

// me calls GET /me.
func (e *env) me(c *http.Client) *adminv1.Me {
	e.t.Helper()
	status, b := e.call(c, "GET", "me", "", nil)
	if status != http.StatusOK {
		e.t.Fatalf("me: %d %s", status, b)
	}
	m := &adminv1.Me{}
	if err := protojson.Unmarshal(b, m); err != nil {
		e.t.Fatal(err)
	}
	return m
}

// errorCode checks a failed call's status and error code.
func (e *env) errorCode(c *http.Client, method, path, csrf string, body proto.Message, status int, want string) *adminv1.Error {
	e.t.Helper()
	got, b := e.call(c, method, path, csrf, body)
	m := &adminv1.Error{}
	_ = protojson.Unmarshal(b, m)
	if got != status || m.GetCode() != want {
		e.t.Errorf("%s %s: %d %s, want %d %s", method, path, got, b, status, want)
	}
	return m
}

func give(account, role, reason string) *adminv1.GiveRoleRequest {
	return adminv1.GiveRoleRequest_builder{Account: account, Role: role, Reason: reason}.Build()
}

// auditRows returns the audit rows' action, target and role argument,
// oldest first.
func (e *env) auditRows() []string {
	e.t.Helper()
	rows, err := e.admin.Query(context.Background(), "SELECT account_name || ' ' || role || ' ' || action || ' ' || target || ' ' || (arguments->>'role') || ' ' || reason FROM staff_audit ORDER BY id")
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
		out = append(out, s)
	}
	return out
}

// TestTwoFactorRule checks who /me says has a role, and that every
// call but /me refuses a role without a second factor (review decision
// 33).
// TestMissingOwners checks that the names of staff.owners without an
// account are found, so the server warns at startup (review decision
// 51).
func TestMissingOwners(t *testing.T) {
	e := setup(t, options{owners: []string{"Owner", "Second"}})
	ctx := context.Background()
	missing, err := e.handler.MissingOwners(ctx)
	if err != nil || !slices.Equal(missing, []string{"Owner", "Second"}) {
		t.Fatalf("before signup: %v %v", missing, err)
	}
	e.signup("owner")
	missing, err = e.handler.MissingOwners(ctx)
	if err != nil || !slices.Equal(missing, []string{"Second"}) {
		t.Fatalf("after signup: %v %v", missing, err)
	}
}

// TestOwnerAccount: the world's owner by account ID has the owner role,
// with a second factor, though staff.owners does not name it. Its role
// cannot change, and it is listed first (review decision 54).
func TestOwnerAccount(t *testing.T) {
	e := setup(t, options{founder: true})
	c := e.browser()
	if status, body := e.submit(c, "/account/login", "/account/login", url.Values{"name": {"Founder"}, "password": {password}}); status != http.StatusSeeOther {
		t.Fatalf("login: %d %s", status, body)
	}
	if m := e.me(c); m.GetRole() != "" || m.GetAssignedRole() != staff.Owner {
		t.Errorf("owner account without TOTP: %v", m)
	}
	e.enableTOTP(c, "Founder")
	founder := e.loginTOTP("Founder")
	if m := e.me(founder); m.GetRole() != staff.Owner || !m.GetTwoFactor() {
		t.Fatalf("owner account with TOTP: %v", m)
	}
	status, b := e.call(founder, "GET", "staff", "", nil)
	list := &adminv1.StaffList{}
	_ = protojson.Unmarshal(b, list)
	if status != http.StatusOK || len(list.GetMembers()) != 2 || list.GetMembers()[0].GetAccount() != "Founder" ||
		list.GetMembers()[0].GetRole() != staff.Owner || !list.GetMembers()[0].GetTwoFactor() {
		t.Fatalf("list: %d %s", status, b)
	}
	// Another admin cannot change the owner's role.
	adm := e.staffer("Admin1")
	csrf := e.me(founder).GetCsrfToken()
	if status, b := e.call(founder, "POST", "staff", csrf, give("Admin1", "admin", "")); status != http.StatusOK {
		t.Fatalf("give admin: %d %s", status, b)
	}
	e.errorCode(adm, "POST", "staff", e.me(adm).GetCsrfToken(), give("Founder", "moderator", ""), http.StatusConflict, "conflict")
	// The account cannot ask for its own deletion.
	status, body := e.submit(founder, "/account/data", "/account/data/delete", url.Values{"name": {"Founder"}})
	if status != http.StatusConflict || !strings.Contains(body, "owner of the world") {
		t.Errorf("owner's deletion: %d %s", status, body)
	}
}

func TestTwoFactorRule(t *testing.T) {
	e := setup(t, options{})
	anon := e.browser()
	e.errorCode(anon, "GET", "me", "", nil, http.StatusUnauthorized, "login")
	e.errorCode(anon, "GET", "staff", "", nil, http.StatusUnauthorized, "login")
	e.errorCode(anon, "GET", "audit", "", nil, http.StatusUnauthorized, "login")
	e.errorCode(anon, "GET", "nothing", "", nil, http.StatusNotFound, "not_found")

	// A player has no role.
	player := e.signup("Player")
	if m := e.me(player); m.GetAccount() != "Player" || m.GetRole() != "" || m.GetAssignedRole() != "" || len(m.GetPermissions()) != 0 || m.GetCsrfToken() == "" {
		t.Errorf("player me %v", m)
	}
	e.errorCode(player, "GET", "staff", "", nil, http.StatusForbidden, "forbidden")
	e.errorCode(player, "GET", "audit", "", nil, http.StatusForbidden, "forbidden")

	// The owner without two-factor login has the role assigned, and it
	// does not count.
	owner := e.signup("Owner")
	if m := e.me(owner); m.GetRole() != "" || m.GetAssignedRole() != staff.Owner || m.GetTwoFactor() || !m.GetRecentLogin() {
		t.Errorf("owner without TOTP: %v", m)
	}
	e.errorCode(owner, "GET", "staff", "", nil, http.StatusForbidden, "two_factor")
	// TOTP on, but the session logged in with the password alone.
	e.enableTOTP(owner, "Owner")
	e.errorCode(owner, "GET", "audit", "", nil, http.StatusForbidden, "two_factor")
	// A login with the code counts.
	owner = e.loginTOTP("Owner")
	m := e.me(owner)
	if m.GetRole() != staff.Owner || !m.GetTwoFactor() || len(m.GetPermissions()) != len(staff.Catalog) ||
		m.GetReauthUrl() != "/account/reauth?next=%2Fadmin" {
		t.Errorf("owner with TOTP: %v", m)
	}
	if status, b := e.call(owner, "GET", "staff", "", nil); status != http.StatusOK {
		t.Errorf("staff: %d %s", status, b)
	}
	// A role given in the database to a player with TOTP counts only
	// after a login with it, and a role the config lacks never counts.
	e.enableTOTP(player, "Player")
	if _, err := e.admin.Exec(context.Background(), "INSERT INTO staff_roles (account_id, role) SELECT id, 'retired' FROM accounts WHERE name = 'Player'"); err != nil {
		t.Fatal(err)
	}
	if m := e.me(e.loginTOTP("Player")); m.GetRole() != "" || m.GetAssignedRole() != "" {
		t.Errorf("a role the config lacks: %v", m)
	}
}

// TestStaffChanges gives and removes roles, with the CSRF token, the
// recent login, the audit rows and the checks on who may change what.
func TestStaffChanges(t *testing.T) {
	e := setup(t, options{})
	owner := e.staffer("Owner")
	e.signup("Bobby")
	csrf := e.me(owner).GetCsrfToken()

	// A change needs the CSRF token.
	e.errorCode(owner, "POST", "staff", "", give("Bobby", "game_master", ""), http.StatusForbidden, "csrf")
	e.errorCode(owner, "POST", "staff", "not the token", give("Bobby", "game_master", ""), http.StatusForbidden, "csrf")
	e.errorCode(owner, "DELETE", "staff/Bobby", "", nil, http.StatusForbidden, "csrf")
	// A token from another browser does not work.
	e.errorCode(owner, "POST", "staff", e.me(e.signup("Other")).GetCsrfToken(), give("Bobby", "game_master", ""), http.StatusForbidden, "csrf")
	if rows := e.auditRows(); len(rows) != 0 {
		t.Fatalf("refused calls wrote %q", rows)
	}

	status, b := e.call(owner, "POST", "staff", csrf, give("bobby", "game_master", "new GM"))
	sm := &adminv1.StaffMember{}
	_ = protojson.Unmarshal(b, sm)
	if status != http.StatusOK || sm.GetAccount() != "Bobby" || sm.GetRole() != "game_master" {
		t.Fatalf("give: %d %s", status, b)
	}
	// The same role again changes nothing and writes no row.
	if status, _ := e.call(owner, "POST", "staff", csrf, give("Bobby", "game_master", "")); status != http.StatusOK {
		t.Errorf("give again: %d", status)
	}
	e.errorCode(owner, "POST", "staff", csrf, give("Bobby", "owner", ""), http.StatusBadRequest, "bad_request")
	e.errorCode(owner, "POST", "staff", csrf, give("Bobby", "wizard", ""), http.StatusBadRequest, "bad_request")
	e.errorCode(owner, "POST", "staff", csrf, give("Nobody", "moderator", ""), http.StatusNotFound, "not_found")
	e.errorCode(owner, "POST", "staff", csrf, give("x y", "moderator", ""), http.StatusBadRequest, "bad_request")
	e.errorCode(owner, "POST", "staff", csrf, give("Owner", "moderator", ""), http.StatusConflict, "conflict")
	e.errorCode(owner, "POST", "staff", csrf, give("Bobby", "moderator", strings.Repeat("a", 256)), http.StatusBadRequest, "bad_request")

	// The list has the owner from the config and Bobby.
	status, b = e.call(owner, "GET", "staff", "", nil)
	list := &adminv1.StaffList{}
	_ = protojson.Unmarshal(b, list)
	if status != http.StatusOK || len(list.GetMembers()) != 2 || len(list.GetRoles()) != 3 {
		t.Fatalf("list: %d %s", status, b)
	}
	if o := list.GetMembers()[0]; o.GetAccount() != "Owner" || !o.GetFromConfig() || !o.GetTwoFactor() || o.GetRole() != staff.Owner {
		t.Errorf("owner row %v", o)
	}
	if m := list.GetMembers()[1]; m.GetAccount() != "Bobby" || m.GetGivenBy() != "Owner" || m.GetTwoFactor() || m.GetFromConfig() {
		t.Errorf("Bobby's row %v", m)
	}

	// Bobby, a game master with TOTP, reads the audit log but cannot
	// change roles.
	e.enableTOTP(e.loginPassword("Bobby"), "Bobby")
	bobby := e.loginTOTP("Bobby")
	if status, b := e.call(bobby, "GET", "audit", "", nil); status != http.StatusOK || !strings.Contains(string(b), "staff.give") {
		t.Errorf("Bobby's audit: %d %s", status, b)
	}
	e.errorCode(bobby, "GET", "staff", "", nil, http.StatusForbidden, "forbidden")
	e.errorCode(bobby, "POST", "staff", e.me(bobby).GetCsrfToken(), give("Other", "moderator", ""), http.StatusForbidden, "forbidden")

	// A stale login must log in again first.
	if _, err := e.admin.Exec(context.Background(), "UPDATE account_sessions SET authenticated_at = now() - interval '11 minutes'"); err != nil {
		t.Fatal(err)
	}
	if m := e.me(owner); m.GetRecentLogin() {
		t.Error("an old login is recent")
	}
	if got := e.errorCode(owner, "POST", "staff", csrf, give("Other", "admin", ""), http.StatusForbidden, "reauth"); got.GetReauthUrl() != "/account/reauth?next=%2Fadmin" {
		t.Errorf("reauth URL %q", got.GetReauthUrl())
	}
	e.errorCode(owner, "DELETE", "staff/Bobby", csrf, nil, http.StatusForbidden, "reauth")
	// The reauth page goes back to the admin screens.
	e.clock.step()
	if status, loc := e.submit(owner, "/account/reauth?next=/admin", "/account/reauth", url.Values{
		"password": {password}, "code": {code(e.secrets["Owner"], e.clock.Now())}, "next": {"/admin"},
	}); status != http.StatusSeeOther || !strings.HasPrefix(loc, "/admin ") {
		t.Fatalf("reauth: %d %s", status, loc)
	}
	if status, b := e.call(owner, "POST", "staff", csrf, give("Other", "admin", "")); status != http.StatusOK {
		t.Fatalf("give admin: %d %s", status, b)
	}

	// Other, an admin, removes Bobby's role, but cannot change the owner
	// or itself.
	e.enableTOTP(e.loginPassword("Other"), "Other")
	other := e.loginTOTP("Other")
	oc := e.me(other).GetCsrfToken()
	e.errorCode(other, "DELETE", "staff/Owner", oc, nil, http.StatusConflict, "conflict")
	e.errorCode(other, "DELETE", "staff/Other", oc, nil, http.StatusConflict, "conflict")
	e.errorCode(other, "DELETE", "staff/Player", oc, nil, http.StatusNotFound, "not_found")
	if status, b := e.call(other, "DELETE", "staff/bobby?reason=left+the+team", oc, nil); status != http.StatusNoContent {
		t.Fatalf("remove: %d %s", status, b)
	}
	e.errorCode(other, "DELETE", "staff/Bobby", oc, nil, http.StatusNotFound, "not_found")
	if m := e.me(bobby); m.GetRole() != "" {
		t.Errorf("Bobby still has %q", m.GetRole())
	}
	want := []string{"Owner owner staff.give Bobby game_master new GM", "Owner owner staff.give Other admin ", "Other admin staff.remove Bobby game_master left the team"}
	if got := e.auditRows(); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("audit rows %q, want %q", got, want)
	}
	if got := e.world.got(); strings.Join(got, ",") != "game_master,admin," {
		t.Errorf("the world got %q", got)
	}
}

// loginPassword logs in with the password alone, for an account without
// TOTP.
func (e *env) loginPassword(name string) *http.Client {
	e.t.Helper()
	c := e.browser()
	if status, body := e.submit(c, "/account/login", "/account/login", url.Values{"name": {name}, "password": {password}}); status != http.StatusSeeOther {
		e.t.Fatalf("login %s: %d %s", name, status, body)
	}
	return c
}

// TestRolesCover checks that a caller gives and takes only roles whose
// permissions it holds.
func TestRolesCover(t *testing.T) {
	e := setup(t, options{roles: map[string][]string{
		"lead": {"staff.assign", "audit.read"}, "helper": {"audit.read"}, "gm": {"audit.read", "moderation.*"},
	}})
	owner := e.staffer("Owner")
	csrf := e.me(owner).GetCsrfToken()
	e.signup("Lead")
	e.signup("Gamer")
	e.signup("Helper")
	for _, g := range [][2]string{{"Lead", "lead"}, {"Gamer", "gm"}} {
		if status, b := e.call(owner, "POST", "staff", csrf, give(g[0], g[1], "")); status != http.StatusOK {
			t.Fatalf("give %v: %d %s", g, status, b)
		}
	}
	e.enableTOTP(e.loginPassword("Lead"), "Lead")
	lead := e.loginTOTP("Lead")
	lc := e.me(lead).GetCsrfToken()
	if status, b := e.call(lead, "POST", "staff", lc, give("Helper", "helper", "")); status != http.StatusOK {
		t.Errorf("lead gives helper: %d %s", status, b)
	}
	e.errorCode(lead, "POST", "staff", lc, give("Helper", "gm", ""), http.StatusForbidden, "forbidden")
	e.errorCode(lead, "DELETE", "staff/Gamer", lc, nil, http.StatusForbidden, "forbidden")
	e.errorCode(lead, "POST", "staff", lc, give("Gamer", "helper", ""), http.StatusForbidden, "forbidden")
}

// TestAuditSearch checks GET /audit's filters, paging and errors.
func TestAuditSearch(t *testing.T) {
	e := setup(t, options{})
	owner := e.staffer("Owner")
	ctx := context.Background()
	for i, r := range []store.AuditRow{
		{AccountID: 1, AccountName: "Gm", Role: "game_master", Action: "item.give", Source: "game", Target: "Hero",
			Arguments: map[string]any{"item": 501}, Reason: "prize", NeedsReview: true},
		{AccountID: 1, AccountName: "Gm", Role: "game_master", Action: "player.warp", Source: "game", Target: "Hero"},
		{AccountID: 2, AccountName: "Mod", Role: "moderator", Action: "player.mute", Source: "game", Target: "Spammer", Reason: "spam"},
		{AccountID: 1, AccountName: "Gm", Role: "game_master", Action: "item.give", Source: "game", Target: "Other",
			Arguments: map[string]any{"item": 607}, Reason: "event", NeedsReview: true},
	} {
		if err := store.WriteStaffAudit(ctx, e.game, r); err != nil {
			t.Fatalf("row %d: %v", i, err)
		}
	}
	if _, err := e.admin.Exec(ctx, "INSERT INTO staff_audit_reviews (audit_id, reviewer_id, reviewer_name) SELECT min(id), 9, 'Owner' FROM staff_audit"); err != nil {
		t.Fatal(err)
	}
	search := func(query string) (*adminv1.AuditPage, []string) {
		t.Helper()
		status, b := e.call(owner, "GET", "audit"+query, "", nil)
		if status != http.StatusOK {
			t.Fatalf("%s: %d %s", query, status, b)
		}
		p := &adminv1.AuditPage{}
		if err := protojson.Unmarshal(b, p); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, en := range p.GetEntries() {
			out = append(out, en.GetAction()+" "+en.GetTarget())
		}
		return p, out
	}
	for _, c := range []struct{ query, want string }{
		{"", "item.give Other,player.mute Spammer,player.warp Hero,item.give Hero"},
		{"?account=gm", "item.give Other,player.warp Hero,item.give Hero"},
		{"?action=item.give", "item.give Other,item.give Hero"},
		{"?target=hero", "player.warp Hero,item.give Hero"},
		{"?review=pending", "item.give Other"},
		{"?review=reviewed", "item.give Hero"},
		{"?since=2000-01-01&until=2100-01-01T00:00:00Z", "item.give Other,player.mute Spammer,player.warp Hero,item.give Hero"},
		{"?until=2000-01-01", ""},
	} {
		if _, got := search(c.query); strings.Join(got, ",") != c.want {
			t.Errorf("%s: %q, want %q", c.query, got, c.want)
		}
	}
	// Entries carry their arguments, reason and review.
	p, _ := search("?review=reviewed")
	if en := p.GetEntries()[0]; en.GetArguments().GetFields()["item"].GetNumberValue() != 501 || en.GetReason() != "prize" ||
		!en.GetNeedsReview() || en.GetReviewedBy() != "Owner" || en.GetReviewedAt() == nil || en.GetSource() != "game" {
		t.Errorf("entry %v", en)
	}
	// Pages of 3, then the rest.
	p, got := search("?limit=3")
	if len(got) != 3 || p.GetNextBeforeId() == 0 {
		t.Fatalf("first page %q, next %d", got, p.GetNextBeforeId())
	}
	p, got = search("?limit=3&before_id=" + strconv.FormatInt(p.GetNextBeforeId(), 10))
	if strings.Join(got, ",") != "item.give Hero" || p.GetNextBeforeId() != 0 {
		t.Errorf("second page %q, next %d", got, p.GetNextBeforeId())
	}
	for _, q := range []string{"?limit=0", "?limit=101", "?before_id=x", "?review=all", "?since=yesterday", "?account=" + strings.Repeat("a", 25)} {
		e.errorCode(owner, "GET", "audit"+q, "", nil, http.StatusBadRequest, "bad_request")
	}
}

// TestRateLimit checks that admin calls share a bucket per address.
func TestRateLimit(t *testing.T) {
	e := setup(t, options{limits: func(l *account.Limits) { l.Admin = account.Bucket{Burst: 2, Every: time.Hour} }})
	c := e.signup("Player")
	e.me(c)
	e.me(c)
	e.errorCode(c, "GET", "me", "", nil, http.StatusTooManyRequests, "rate")
	e.errorCode(e.browser(), "GET", "staff", "", nil, http.StatusTooManyRequests, "rate")
}

// TestBadBodies checks that a change needs a JSON body of known fields.
func TestBadBodies(t *testing.T) {
	e := setup(t, options{})
	owner := e.staffer("Owner")
	csrf := e.me(owner).GetCsrfToken()
	for _, c := range []struct {
		ctype, body string
		status      int
	}{
		{"text/plain", `{"account":"Bobby","role":"admin"}`, http.StatusUnsupportedMediaType},
		{"application/json", `{"account":"Bobby","role":"admin","extra":1}`, http.StatusBadRequest},
		{"application/json", `not json`, http.StatusBadRequest},
		{"application/json", `{"account":"` + strings.Repeat("a", 20000) + `"}`, http.StatusBadRequest},
	} {
		req, _ := http.NewRequest("POST", e.srv.URL+admin.Prefix+"staff", strings.NewReader(c.body))
		req.Header.Set("Content-Type", c.ctype)
		req.Header.Set(admin.CSRFHeader, csrf)
		resp, err := owner.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != c.status {
			t.Errorf("%s %.40s: %d, want %d", c.ctype, c.body, resp.StatusCode, c.status)
		}
	}
	// A change from another site is refused even with the token.
	req, _ := http.NewRequest("POST", e.srv.URL+admin.Prefix+"staff", strings.NewReader(`{"account":"Bobby","role":"admin"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(admin.CSRFHeader, csrf)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	resp, err := owner.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site change: %d", resp.StatusCode)
	}
}

// TestTicketCarriesTwoFactor checks that a game ticket records whether
// its session logged in with a second factor, for the game's staff
// role.
func TestTicketCarriesTwoFactor(t *testing.T) {
	e := setup(t, options{})
	st := store.New(e.game, store.Options{Log: slog.New(slog.DiscardHandler)})
	t.Cleanup(func() { _ = st.Close(context.Background()) })
	login := func(c *http.Client, name string) (string, bool) {
		t.Helper()
		if status, body := e.submit(c, "/account/characters", "/account/characters", url.Values{
			"name": {name + " Hero"}, "slot": {"0"}, "sex": {"f"}, "hair": {"3"}, "hair_color": {"2"},
		}); status != http.StatusSeeOther && status != http.StatusConflict {
			t.Fatalf("character: %d %s", status, body)
		}
		if status, body := e.submit(c, "/account/characters", "/account/play", url.Values{"slot": {"0"}}); status != http.StatusOK && status != http.StatusSeeOther {
			t.Fatalf("play: %d %s", status, body)
		}
		req, _ := http.NewRequest("POST", e.srv.URL+"/account/ticket", nil)
		req.Header.Set("Origin", "http://client.test")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		m := regexp.MustCompile(`"ticket":"([^"]+)"`).FindSubmatch(b)
		if m == nil {
			t.Fatalf("ticket: %d %s", resp.StatusCode, b)
		}
		l, err := account.GameTickets{Store: st}.Check(context.Background(), m[1])
		if err != nil {
			t.Fatal(err)
		}
		return l.Staff.AccountName, l.Staff.TwoFactor
	}
	owner := e.signup("Owner")
	if name, two := login(owner, "Owner"); name != "Owner" || two {
		t.Errorf("password login: %s %v", name, two)
	}
	e.enableTOTP(owner, "Owner")
	if _, two := login(owner, "Owner"); two {
		t.Error("TOTP on, but the session logged in with the password")
	}
	if name, two := login(e.loginTOTP("Owner"), "Owner"); name != "Owner" || !two {
		t.Errorf("TOTP login: %s %v", name, two)
	}
}
