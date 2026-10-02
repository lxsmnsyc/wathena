package account_test

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"google.golang.org/protobuf/proto"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/account"
	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

const password = "a fine long password"

// fakeClock is the time TOTP codes are checked against. Tests move it
// on by whole steps.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) step() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(30 * time.Second)
}

// twoFactor sets a sealer, a fake clock and loose address limits, so a
// test can post many codes.
func twoFactor(t *testing.T, clock *fakeClock) func(*account.Config) {
	sealer, err := account.NewSealer(bytes.Repeat([]byte{7}, account.SecretKeyLen))
	if err != nil {
		t.Fatal(err)
	}
	return func(c *account.Config) {
		c.Sealer = sealer
		c.Clock = clock.Now
		c.Limits = account.DefaultLimits
		c.Limits.Login = account.Bucket{Burst: 1000, Every: time.Millisecond}
	}
}

func code(secret []byte, at time.Time) string {
	s, err := totp.GenerateCodeCustom(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(secret), at, totp.ValidateOpts{
		Period: 30, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		panic(err)
	}
	return s
}

var (
	keyField      = regexp.MustCompile(`<p class="key"><code>([A-Z2-7 ]+)</code>`)
	recoveryField = regexp.MustCompile(`<li><code>([a-z2-7]{4}-[a-z2-7]{4}-[a-z2-7]{4}-[a-z2-7]{4})</code></li>`)
	dataCSRF      = regexp.MustCompile(`data-csrf="([^"]+)"`)
)

// enableTOTP turns TOTP on in settings, as a player with an app does,
// and returns the secret and the recovery codes.
func (e *env) enableTOTP(c *http.Client, clock *fakeClock) ([]byte, []string) {
	e.t.Helper()
	resp, body := e.submit(c, "/account/settings", "/account/settings/totp/start", url.Values{})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/settings/totp" {
		e.t.Fatalf("start: %d %s", resp.StatusCode, body)
	}
	page, _ := e.page(c, "/account/settings/totp")
	m := keyField.FindStringSubmatch(page)
	if m == nil || !strings.Contains(page, "<svg ") {
		e.t.Fatalf("setup page: %s", page)
	}
	secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ReplaceAll(m[1], " ", ""))
	if err != nil {
		e.t.Fatal(err)
	}
	// A wrong code does not turn it on.
	resp, _ = e.submit(c, "/account/settings/totp", "/account/settings/totp", url.Values{"code": {code(secret, clock.Now().Add(-time.Hour))}})
	if resp.StatusCode != http.StatusBadRequest {
		e.t.Fatalf("wrong first code: %d", resp.StatusCode)
	}
	resp, body = e.submit(c, "/account/settings/totp", "/account/settings/totp", url.Values{"code": {code(secret, clock.Now())}})
	codes := recoveryField.FindAllStringSubmatch(body, -1)
	if resp.StatusCode != http.StatusOK || len(codes) != account.RecoveryCodes {
		e.t.Fatalf("confirm: %d %s", resp.StatusCode, body)
	}
	var out []string
	for _, m := range codes {
		out = append(out, m[1])
	}
	return secret, out
}

func (e *env) logout(c *http.Client) {
	e.t.Helper()
	if resp, body := e.submit(c, "/account/characters", "/account/logout", url.Values{}); resp.StatusCode != http.StatusSeeOther {
		e.t.Fatalf("logout: %d %s", resp.StatusCode, body)
	}
}

// sendCode posts a code on the second login step.
func (e *env) sendCode(c *http.Client, code string) (*http.Response, string) {
	e.t.Helper()
	return e.submit(c, "/account/login/code", "/account/login/code", url.Values{"code": {code}})
}

// liveFactors returns the factors of the live sessions, newest first.
func (e *env) liveFactors() [][]string {
	e.t.Helper()
	rows, err := e.admin.Query(context.Background(), "SELECT factors FROM account_sessions WHERE revoked_at IS NULL ORDER BY id DESC")
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	var out [][]string
	for rows.Next() {
		var f []string
		if err := rows.Scan(&f); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, f)
	}
	return out
}

// ageSessions makes every session's last login an hour old, so pages
// that need a recent login ask again.
func (e *env) ageSessions() {
	e.t.Helper()
	if _, err := e.admin.Exec(context.Background(), "UPDATE account_sessions SET authenticated_at = now() - interval '1 hour'"); err != nil {
		e.t.Fatal(err)
	}
}

func TestTOTPLogin(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	e := setup(t, twoFactor(t, clock))
	c := e.c
	e.signup(c, "Twofa", password)
	secret, codes := e.enableTOTP(c, clock)

	// The database holds the secret sealed, never as it is.
	var sealed []byte
	if err := e.admin.QueryRow(context.Background(), "SELECT secret FROM account_totp").Scan(&sealed); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(sealed, secret) || len(sealed) < 57 {
		t.Errorf("stored secret %x", sealed)
	}
	body, _ := e.page(c, "/account/settings")
	if !strings.Contains(body, "10 of 10 recovery codes left") {
		t.Errorf("settings: %s", body)
	}

	// The password alone now leads to the code page, without a session.
	e.logout(c)
	resp, _ := e.login(c, "Twofa", password, nil)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/login/code" {
		t.Fatalf("password step: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if code, _ := e.ticket(c, clientOrigin); code != http.StatusUnauthorized {
		t.Errorf("ticket with only the password: %d", code)
	}
	if resp, body := e.sendCode(c, code(secret, clock.Now().Add(-time.Hour))); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "wrong") {
		t.Errorf("wrong code: %d %s", resp.StatusCode, body)
	}
	// The code that turned TOTP on was used, so it is refused.
	if resp, _ := e.sendCode(c, code(secret, clock.Now())); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("reused code: %d", resp.StatusCode)
	}
	clock.step()
	next := code(secret, clock.Now())
	resp, body = e.sendCode(c, next)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/characters" {
		t.Fatalf("right code: %d %s", resp.StatusCode, body)
	}
	if f := e.liveFactors(); len(f) != 1 || !slices.Equal(f[0], []string{"password", "totp"}) {
		t.Errorf("factors: %v", f)
	}
	// The same code in another browser within its step is refused.
	other := e.newClient()
	e.login(other, "Twofa", password, nil)
	if resp, _ := e.sendCode(other, next); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("replay in another browser: %d", resp.StatusCode)
	}

	// A recovery code works once.
	e.logout(c)
	e.login(c, "Twofa", password, nil)
	if resp, body := e.sendCode(c, codes[0]); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("recovery code: %d %s", resp.StatusCode, body)
	}
	if f := e.liveFactors(); !slices.Equal(f[0], []string{"password", "recovery_code"}) {
		t.Errorf("factors: %v", f)
	}
	e.logout(c)
	e.login(c, "Twofa", password, nil)
	if resp, _ := e.sendCode(c, codes[0]); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("recovery code used twice: %d", resp.StatusCode)
	}
	// Typed in capitals without dashes, another one works.
	if resp, _ := e.sendCode(c, strings.ToUpper(strings.ReplaceAll(codes[1], "-", ""))); resp.StatusCode != http.StatusSeeOther {
		t.Errorf("second recovery code: %d", resp.StatusCode)
	}
	body, _ = e.page(c, "/account/settings")
	if !strings.Contains(body, "8 of 10 recovery codes left") || !strings.Contains(body, "with password and recovery code") {
		t.Errorf("settings: %s", body)
	}

	// Turning it off needs the password and a current code.
	clock.step()
	for _, form := range []url.Values{
		{"password": {"a wrong password"}, "code": {code(secret, clock.Now())}},
		{"password": {password}, "code": {"000000"}},
		{"password": {password}},
	} {
		if resp, _ := e.submit(c, "/account/settings", "/account/settings/totp/disable", form); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("disable with %v: %d", form, resp.StatusCode)
		}
	}
	resp, _ = e.submit(c, "/account/settings", "/account/settings/totp/disable", url.Values{"password": {password}, "code": {code(secret, clock.Now())}})
	if resp.StatusCode != http.StatusSeeOther || !strings.Contains(resp.Header.Get("Location"), "totp-off") {
		t.Fatalf("disable: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	var left int
	_ = e.admin.QueryRow(context.Background(), "SELECT (SELECT count(*) FROM account_totp) + (SELECT count(*) FROM account_recovery_codes)").Scan(&left)
	if left != 0 {
		t.Errorf("%d TOTP rows left", left)
	}
	e.logout(c)
	if resp, _ := e.login(c, "Twofa", password, nil); resp.Header.Get("Location") != "/account/characters" {
		t.Errorf("login after turning it off: %s", resp.Header.Get("Location"))
	}
}

func TestPendingLoginLimits(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	e := setup(t, func(c *account.Config) {
		twoFactor(t, clock)(c)
		c.MaxCodeAttempts = 3
	})
	e.signup(e.c, "Pending", password)
	secret, _ := e.enableTOTP(e.c, clock)
	e.logout(e.c)
	clock.step()

	// Three attempts, then the pending login is gone, even for the right
	// code.
	c := e.newClient()
	e.login(c, "Pending", password, nil)
	wrong := code(secret, clock.Now().Add(-time.Hour))
	for i := 1; i <= 2; i++ {
		if resp, body := e.sendCode(c, wrong); resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "tries left") {
			t.Fatalf("attempt %d: %d %s", i, resp.StatusCode, body)
		}
	}
	resp, _ := e.sendCode(c, wrong)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/login?notice=expired" {
		t.Errorf("third wrong code: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	// The code page is gone, so the form's token comes from the login
	// page. It depends only on the cookies.
	resp, _ = e.submit(c, "/account/login", "/account/login/code", url.Values{"code": {code(secret, clock.Now())}})
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/account/login?") {
		t.Errorf("right code after the attempts: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}

	// A pending login expires. The test moves its expiry into the past
	// instead of sleeping, since a short PendingLifetime could run out
	// during the attempts above on a busy machine.
	c = e.newClient()
	e.login(c, "Pending", password, nil)
	if _, err := e.admin.Exec(context.Background(), "UPDATE account_pending_logins SET created_at = now() - interval '2 seconds', expires_at = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	resp, _ = e.submit(c, "/account/login", "/account/login/code", url.Values{"code": {code(secret, clock.Now())}})
	if resp.StatusCode != http.StatusSeeOther || !strings.Contains(resp.Header.Get("Location"), "notice=expired") {
		t.Errorf("expired: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	// Without the pending cookie the code page sends the player back.
	resp, err := e.newClient().Get(e.srv.URL + "/account/login/code")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/account/login?") {
		t.Errorf("code page without a pending login: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	// Cleanup deletes expired pending logins.
	e.svc.Cleanup(context.Background())
	var n int
	_ = e.admin.QueryRow(context.Background(), "SELECT count(*) FROM account_pending_logins").Scan(&n)
	if n != 0 {
		t.Errorf("%d pending logins left", n)
	}
}

func TestReauth(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	e := setup(t, twoFactor(t, clock))
	c := e.c
	e.signup(c, "Reauth", password)
	e.ageSessions()
	// An old login must prove itself again before changing login
	// methods.
	resp, _ := e.submit(c, "/account/settings", "/account/settings/totp/start", url.Values{})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/reauth?next=%2Faccount%2Fsettings" {
		t.Fatalf("start with an old login: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if resp, _ := e.submit(c, "/account/reauth", "/account/reauth", url.Values{"password": {"a wrong password"}, "next": {"/account/settings"}}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("wrong password: %d", resp.StatusCode)
	}
	// next cannot leave the service.
	resp, _ = e.submit(c, "/account/reauth", "/account/reauth", url.Values{"password": {password}, "next": {"https://evil.test/"}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/settings" {
		t.Fatalf("reauth: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	secret, _ := e.enableTOTP(c, clock)

	// With TOTP on, the password alone is not enough.
	e.ageSessions()
	clock.step()
	if resp, _ := e.submit(c, "/account/reauth", "/account/reauth", url.Values{"password": {password}}); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("reauth without a code: %d", resp.StatusCode)
	}
	if resp, _ := e.submit(c, "/account/settings", "/account/settings/recovery", url.Values{}); !strings.HasPrefix(resp.Header.Get("Location"), "/account/reauth") {
		t.Errorf("new codes with an old login: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp, _ = e.submit(c, "/account/reauth", "/account/reauth", url.Values{"password": {password}, "code": {code(secret, clock.Now())}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("reauth with a code: %d", resp.StatusCode)
	}
	resp, body := e.submit(c, "/account/settings", "/account/settings/recovery", url.Values{})
	if resp.StatusCode != http.StatusOK || len(recoveryField.FindAllString(body, -1)) != account.RecoveryCodes {
		t.Errorf("new codes: %d %s", resp.StatusCode, body)
	}
}

// postJSON posts to a passkey endpoint as passkey.js does.
func (e *env) postJSON(c *http.Client, path, csrf string, body []byte) (int, []byte) {
	e.t.Helper()
	req, _ := http.NewRequest("POST", e.srv.URL+path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// pageCSRF returns the data-csrf of a page's passkey button.
func (e *env) pageCSRF(c *http.Client, path string) string {
	e.t.Helper()
	body, _ := e.page(c, path)
	m := dataCSRF.FindStringSubmatch(body)
	if m == nil {
		e.t.Fatalf("%s has no passkey button: %s", path, body)
	}
	return m[1]
}

// passkeyLogin logs in with a passkey in a new browser, and returns the
// browser, the status and the answer.
func (e *env) passkeyLogin(key *softKey, query string) (*http.Client, int, map[string]string) {
	e.t.Helper()
	c := e.newClient()
	csrf := e.pageCSRF(c, "/account/login")
	status, options := e.postJSON(c, "/account/passkeys/login/begin", csrf, []byte(`{"purpose":"login"}`))
	if status != http.StatusOK {
		e.t.Fatalf("login begin: %d %s", status, options)
	}
	status, b := e.postJSON(c, "/account/passkeys/login/finish"+query, csrf, key.get(options))
	var out map[string]string
	_ = json.Unmarshal(b, &out)
	return c, status, out
}

const passkeyOrigin = "http://localhost"

func TestPasskeys(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	e := setup(t, func(c *account.Config) {
		twoFactor(t, clock)(c)
		// Passkeys need a domain name. The test client reaches the
		// server by IP address, which does not matter to the server.
		c.PublicURL = passkeyOrigin
	})
	c := e.c
	e.signup(c, "Passer", password)
	csrf := e.pageCSRF(c, "/account/settings")

	// Registration.
	if status, _ := e.postJSON(c, "/account/passkeys/register/begin", "", []byte(`{"name":"Test key"}`)); status != http.StatusForbidden {
		t.Errorf("begin without the CSRF token: %d", status)
	}
	status, options := e.postJSON(c, "/account/passkeys/register/begin", csrf, []byte(`{"name":"Test key"}`))
	if status != http.StatusOK {
		t.Fatalf("register begin: %d %s", status, options)
	}
	var opts struct {
		PublicKey struct {
			RP   struct{ ID string } `json:"rp"`
			User struct{ Name string }
			Sel  struct {
				ResidentKey      string `json:"residentKey"`
				UserVerification string `json:"userVerification"`
			} `json:"authenticatorSelection"`
		} `json:"publicKey"`
	}
	_ = json.Unmarshal(options, &opts)
	if o := opts.PublicKey; o.RP.ID != "localhost" || o.User.Name != "Passer" || o.Sel.ResidentKey != "required" || o.Sel.UserVerification != "preferred" {
		t.Errorf("options: %s", options)
	}
	key := newSoftKey(t, passkeyOrigin)
	status, b := e.postJSON(c, "/account/passkeys/register/finish", csrf, key.create(options))
	if status != http.StatusOK || !strings.Contains(string(b), "passkey-added") {
		t.Fatalf("register finish: %d %s", status, b)
	}
	body, _ := e.page(c, "/account/settings")
	if !strings.Contains(body, "Test key") || !strings.Contains(body, "last used never") {
		t.Errorf("settings: %s", body)
	}
	// A key from another site's origin is refused.
	status, options = e.postJSON(c, "/account/passkeys/register/begin", csrf, []byte(`{"name":"Phished"}`))
	if status != http.StatusOK {
		t.Fatalf("second begin: %d", status)
	}
	phished := newSoftKey(t, "http://evil.test")
	if status, _ := e.postJSON(c, "/account/passkeys/register/finish", csrf, phished.create(options)); status != http.StatusBadRequest {
		t.Errorf("wrong origin: %d", status)
	}
	// Adding a passkey needs a recent login.
	e.ageSessions()
	status, b = e.postJSON(c, "/account/passkeys/register/begin", csrf, []byte(`{}`))
	if status != http.StatusForbidden || !strings.Contains(string(b), `"reauth"`) {
		t.Errorf("begin with an old login: %d %s", status, b)
	}
	e.logout(c)

	// Login without a name.
	c, status, out := e.passkeyLogin(key, "?return="+url.QueryEscape(playURL))
	if status != http.StatusOK || out["next"] != "/account/characters?return="+url.QueryEscape(playURL) {
		t.Fatalf("passkey login: %d %v", status, out)
	}
	body, _ = e.page(c, "/account/characters")
	if !strings.Contains(body, "Logged in as <strong>Passer</strong>") {
		t.Fatalf("characters after passkey login: %s", body)
	}
	if f := e.liveFactors(); !slices.Equal(f[0], []string{"passkey"}) {
		t.Errorf("factors: %v", f)
	}
	var count int64
	var used bool
	_ = e.admin.QueryRow(context.Background(), "SELECT sign_count, last_used_at IS NOT NULL FROM account_passkeys").Scan(&count, &used)
	if count != 1 || !used {
		t.Errorf("after use: count %d, used %v", count, used)
	}
	// An answer cannot be used twice: the challenge went with the first.
	csrf = e.pageCSRF(c, "/account/login")
	_, options = e.postJSON(c, "/account/passkeys/login/begin", csrf, []byte(`{}`))
	answer := key.get(options)
	if status, _ := e.postJSON(c, "/account/passkeys/login/finish", csrf, answer); status != http.StatusOK {
		t.Errorf("fresh answer: %d", status)
	}
	csrf = e.pageCSRF(c, "/account/login")
	if status, _ := e.postJSON(c, "/account/passkeys/login/finish", csrf, answer); status != http.StatusBadRequest {
		t.Errorf("answer replayed: %d", status)
	}
	e.postJSON(c, "/account/passkeys/login/begin", csrf, []byte(`{}`))
	if status, _ := e.postJSON(c, "/account/passkeys/login/finish", csrf, answer); status != http.StatusBadRequest {
		t.Errorf("answer replayed on a new challenge: %d", status)
	}

	// A passkey that skipped user verification counts as one factor, so
	// with TOTP on the code is still asked.
	secret, _ := e.enableTOTP(c, clock)
	clock.step()
	key.uv = false
	c2, status, out := e.passkeyLogin(key, "")
	if status != http.StatusOK || out["next"] != "/account/login/code" {
		t.Fatalf("passkey without UV: %d %v", status, out)
	}
	if resp, body := e.sendCode(c2, code(secret, clock.Now())); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("code after the passkey: %d %s", resp.StatusCode, body)
	}
	if f := e.liveFactors(); !slices.Equal(f[0], []string{"passkey", "totp"}) {
		t.Errorf("factors: %v", f)
	}
	key.uv = true

	// Reauth with the passkey.
	e.ageSessions()
	csrf = e.pageCSRF(c2, "/account/reauth")
	status, options = e.postJSON(c2, "/account/passkeys/login/begin", csrf, []byte(`{"purpose":"reauth"}`))
	if status != http.StatusOK || !strings.Contains(string(options), "allowCredentials") {
		t.Fatalf("reauth begin: %d %s", status, options)
	}
	status, b = e.postJSON(c2, "/account/passkeys/login/finish?next=%2Faccount%2Fsettings", csrf, key.get(options))
	if status != http.StatusOK || !strings.Contains(string(b), `"/account/settings"`) {
		t.Fatalf("reauth finish: %d %s", status, b)
	}
	if status, b := e.postJSON(c2, "/account/passkeys/register/begin", e.pageCSRF(c2, "/account/settings"), []byte(`{}`)); status != http.StatusOK {
		t.Errorf("begin after reauth: %d %s", status, b)
	}

	// A counter that does not go up marks the passkey and blocks it.
	key.count = 0
	if _, status, out := e.passkeyLogin(key, ""); status != http.StatusForbidden || out["error"] != "clone" {
		t.Errorf("counter back to 1: %d %v", status, out)
	}
	key.count = 100
	if _, status, out := e.passkeyLogin(key, ""); status != http.StatusForbidden || out["error"] != "clone" {
		t.Errorf("after the clone warning: %d %v", status, out)
	}

	// Removing it, with a recent login, stops it from working.
	var id string
	_ = e.admin.QueryRow(context.Background(), "SELECT id::text FROM account_passkeys").Scan(&id)
	resp, _ := e.submit(c2, "/account/settings", "/account/settings/passkeys/remove", url.Values{"id": {id}})
	if resp.StatusCode != http.StatusSeeOther || !strings.Contains(resp.Header.Get("Location"), "passkey-removed") {
		t.Fatalf("remove: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if _, status, _ := e.passkeyLogin(key, ""); status != http.StatusBadRequest {
		t.Errorf("removed passkey: %d", status)
	}
}

// TestNewTablesRoles checks that only the account role can read the
// two-factor tables.
func TestNewTablesRoles(t *testing.T) {
	e := setup(t, nil)
	ctx := context.Background()
	for _, table := range []string{
		"account_totp", "account_recovery_codes", "account_pending_logins",
		"account_webauthn_users", "account_passkeys", "account_webauthn_ceremonies",
	} {
		_, err := e.game.Exec(ctx, "SELECT * FROM "+table)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Errorf("game role reading %s: %v, want permission denied", table, err)
		}
		if _, err := e.account.Exec(ctx, "SELECT * FROM "+table); err != nil {
			t.Errorf("account role reading %s: %v", table, err)
		}
		_, err = e.account.Exec(ctx, "DROP TABLE "+table)
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Errorf("account role dropping %s: %v", table, err)
		}
	}
}

// gameConn opens the game with a ticket and waits for Welcome.
func gameConn(t *testing.T, url, ticket string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	data, _ := proto.Marshal(wathenav1.ClientMessage_builder{Seq: 1, Hello: wathenav1.Hello_builder{
		ProtocolVersion: schema.Version, SessionToken: []byte(ticket),
	}.Build()}.Build())
	if err := c.Write(ctx, websocket.MessageBinary, data); err != nil {
		t.Fatal(err)
	}
	for {
		m := next(t, c)
		if m.HasWelcome() {
			return c
		}
		if m.HasDisconnect() {
			t.Fatalf("disconnected: %v", m)
		}
	}
}

func next(t *testing.T, c *websocket.Conn) *wathenav1.ServerMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m := &wathenav1.ServerMessage{}
	if err := proto.Unmarshal(data, m); err != nil {
		t.Fatal(err)
	}
	return m
}

// disconnectReason reads until Disconnect and returns its reason.
func disconnectReason(t *testing.T, c *websocket.Conn) wathenav1.DisconnectReason {
	t.Helper()
	for {
		if m := next(t, c); m.HasDisconnect() {
			_ = c.CloseNow()
			return m.GetDisconnect().GetReason()
		}
	}
}

// TestLogoutClosesGame checks, over the real gateway, that ending a
// login session closes the game connection it opened, and only that
// one.
func TestLogoutClosesGame(t *testing.T) {
	g := &path.Grid{Width: 20, Height: 20, Cells: make([]uint8, 400)}
	for i := range g.Cells {
		g.Cells[i] = path.Walkable | path.Shootable
	}
	z := zone.New("map/test", g, rng.FromKey([32]byte{}).Stream(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go z.Run(ctx)
	gw := &gateway.Gateway{World: z, Origins: []string{clientOrigin}}
	e := setup(t, func(c *account.Config) { c.Ended = gw })
	gw.Sessions = account.GameTickets{Store: e.store}
	gw.Characters = storeCharacters{e.store}
	srv := newGateway(t, gw)

	play := func(c *http.Client, name, hero string) *websocket.Conn {
		e.signup(c, name, password)
		e.makeCharacter(c, hero, "0")
		e.play(c, "0")
		_, ticket := e.ticket(c, clientOrigin)
		return gameConn(t, srv, ticket)
	}
	alice := play(e.c, "Alice", "Alice Hero")
	bobClient := e.newClient()
	bob := play(bobClient, "Bobby", "Bobby Hero")

	// Alice logs out in another tab of the same browser.
	e.logout(e.c)
	if r := disconnectReason(t, alice); r != wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_OUT {
		t.Errorf("Alice's game ended with %v", r)
	}
	// Bob plays on.
	data, _ := proto.Marshal(wathenav1.ClientMessage_builder{Seq: 2, Ping: wathenav1.Ping_builder{Nonce: 9}.Build()}.Build())
	if err := bob.Write(context.Background(), websocket.MessageBinary, data); err != nil {
		t.Fatal(err)
	}
	for {
		m := next(t, bob)
		if m.HasDisconnect() {
			t.Fatalf("Bob was disconnected: %v", m)
		}
		if m.HasPong() {
			break
		}
	}

	// A new login in Bob's browser replaces his session, and ends the
	// game it opened.
	if resp, _ := e.login(bobClient, "Bobby", password, nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login again: %d", resp.StatusCode)
	}
	if r := disconnectReason(t, bob); r != wathenav1.DisconnectReason_DISCONNECT_REASON_LOGGED_OUT {
		t.Errorf("Bob's game ended with %v", r)
	}
}

// newGateway serves a gateway and returns its WebSocket URL.
func newGateway(t *testing.T, gw *gateway.Gateway) string {
	t.Helper()
	srv := httptest.NewServer(gw)
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}
