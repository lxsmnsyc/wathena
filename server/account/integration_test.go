package account_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"

	"github.com/lxsmnsyc/wathena/schema"
	wathenav1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/v1"
	"github.com/lxsmnsyc/wathena/server/account"
	"github.com/lxsmnsyc/wathena/server/character"
	"github.com/lxsmnsyc/wathena/server/gateway"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/status"
	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
	"github.com/lxsmnsyc/wathena/server/zone"
	"github.com/lxsmnsyc/wathena/shared/path"
	"github.com/lxsmnsyc/wathena/shared/rng"
)

const (
	clientOrigin = "http://client.test"
	playURL      = clientOrigin + "/play"
)

// fastParams keep tests quick. They are far below DefaultParams.
var fastParams = account.Params{Memory: 64, Time: 1, Threads: 1, SaltLen: 16, KeyLen: 32}

type env struct {
	t       *testing.T
	admin   *pgxpool.Pool
	account *pgxpool.Pool
	game    *pgxpool.Pool
	store   *store.Store
	svc     *account.Service
	srv     *httptest.Server
	c       *http.Client
}

func newCharacter(name string, look account.Look) character.Character {
	return character.Character{
		Info: character.Info{
			Name: name, Male: look.Male, Hair: look.Hair, HairColor: look.HairColor,
			BaseLevel: 1, JobLevel: 1, StatusPoints: 48,
			Stats: status.Stats{Str: 1, Agi: 1, Vit: 1, Int: 1, Dex: 1, Luk: 1},
			HP:    40, SP: 11, Map: "map/test", X: 5, Y: 5, InventorySize: 100,
		},
		Items: []character.Item{{Slot: 0, ID: 501, Amount: 5, Serial: item.NewSerial()}},
	}
}

func setup(t *testing.T, change func(*account.Config)) *env {
	t.Helper()
	e := &env{t: t}
	e.admin = storetest.Migrated(t)
	e.account = storetest.As(t, e.admin, "wathena_account")
	e.game = storetest.As(t, e.admin, "wathena_game")
	e.store = store.New(e.game, store.Options{Log: slog.New(slog.DiscardHandler)})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = e.store.Close(ctx)
	})
	cfg := account.Config{
		Pool: e.account, ClientOrigins: []string{clientOrigin}, PlayURL: playURL,
		NewCharacter: newCharacter, Hasher: account.NewHasher(fastParams, 2),
		Log: slog.New(slog.DiscardHandler),
	}
	if change != nil {
		change(&cfg)
	}
	svc, err := account.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	e.svc = svc
	e.srv = httptest.NewServer(svc)
	t.Cleanup(e.srv.Close)
	e.c = e.newClient()
	return e
}

// newClient is a browser: it keeps cookies and does not follow
// redirects, so tests can see them.
func (e *env) newClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

var csrfField = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

// page loads a page and returns its body and CSRF token.
func (e *env) page(c *http.Client, path string) (string, string) {
	e.t.Helper()
	resp, err := c.Get(e.srv.URL + path)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	m := csrfField.FindStringSubmatch(string(b))
	if m == nil {
		return string(b), ""
	}
	return string(b), m[1]
}

// submit loads a page for its CSRF token, then posts a form to path.
func (e *env) submit(c *http.Client, from, path string, form url.Values) (*http.Response, string) {
	e.t.Helper()
	_, token := e.page(c, from)
	if token == "" {
		e.t.Fatalf("%s has no form", from)
	}
	form.Set("csrf", token)
	resp, err := c.PostForm(e.srv.URL+path, form)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp, string(b)
}

func (e *env) signup(c *http.Client, name, password string) {
	e.t.Helper()
	resp, body := e.submit(c, "/account/signup", "/account/signup", url.Values{"name": {name}, "password": {password}, "confirm": {password}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/characters" {
		e.t.Fatalf("signup: %d %s", resp.StatusCode, body)
	}
}

func (e *env) login(c *http.Client, name, password string, extra url.Values) (*http.Response, string) {
	e.t.Helper()
	form := url.Values{"name": {name}, "password": {password}}
	for k, v := range extra {
		form[k] = v
	}
	return e.submit(c, "/account/login", "/account/login", form)
}

func (e *env) makeCharacter(c *http.Client, name string, slot string) {
	e.t.Helper()
	resp, body := e.submit(c, "/account/characters", "/account/characters", url.Values{
		"name": {name}, "slot": {slot}, "sex": {"f"}, "hair": {"3"}, "hair_color": {"2"},
	})
	if resp.StatusCode != http.StatusSeeOther {
		e.t.Fatalf("make character: %d %s", resp.StatusCode, body)
	}
}

func (e *env) play(c *http.Client, slot string) {
	e.t.Helper()
	resp, body := e.submit(c, "/account/characters", "/account/play", url.Values{"slot": {slot}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != playURL {
		e.t.Fatalf("play: %d %s %s", resp.StatusCode, resp.Header.Get("Location"), body)
	}
}

// ticket fetches a game ticket as the web client does, from the client
// origin with the session cookie.
func (e *env) ticket(c *http.Client, origin string) (int, string) {
	e.t.Helper()
	req, _ := http.NewRequest("POST", e.srv.URL+"/account/ticket", nil)
	req.Header.Set("Origin", origin)
	resp, err := c.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Ticket string `json:"ticket"`
		Error  string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if resp.StatusCode == http.StatusOK && resp.Header.Get("Access-Control-Allow-Origin") != origin {
		e.t.Errorf("ticket: Access-Control-Allow-Origin %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
	if resp.StatusCode == http.StatusOK && resp.Header.Get("Cache-Control") != "no-store" {
		e.t.Errorf("ticket: Cache-Control %q", resp.Header.Get("Cache-Control"))
	}
	if body.Ticket != "" {
		return resp.StatusCode, body.Ticket
	}
	return resp.StatusCode, body.Error
}

func (e *env) redeem(ticket string) (gateway.Login, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return account.GameTickets{Store: e.store}.Check(ctx, []byte(ticket))
}

func TestSignupPlayLogout(t *testing.T) {
	e := setup(t, nil)
	c := e.c
	e.signup(c, "Alice_1", "a fine long password")
	body, _ := e.page(c, "/account/characters")
	if !strings.Contains(body, "Logged in as <strong>Alice_1</strong>") || !strings.Contains(body, "no characters yet") {
		t.Fatalf("characters page: %s", body)
	}
	// No character chosen yet.
	if code, msg := e.ticket(c, clientOrigin); code != http.StatusConflict || msg != "character" {
		t.Errorf("ticket before Play: %d %s", code, msg)
	}
	e.makeCharacter(c, "Alice Hero", "0")
	body, _ = e.page(c, "/account/characters")
	if !strings.Contains(body, "Alice Hero") || !strings.Contains(body, "base level 1") {
		t.Fatalf("new character not listed: %s", body)
	}
	e.play(c, "0")

	code, ticket := e.ticket(c, clientOrigin)
	if code != http.StatusOK || len(ticket) != account.TokenLen {
		t.Fatalf("ticket: %d %q", code, ticket)
	}
	login, err := e.redeem(ticket)
	if err != nil || login.CharacterID == 0 || login.Name != "Alice Hero" || login.Look.Hair != 3 || login.Look.HairColor != 2 {
		t.Fatalf("redeem: %+v %v", login, err)
	}
	// A ticket works once.
	if _, err := e.redeem(ticket); !errors.Is(err, gateway.ErrSessionInvalid) {
		t.Errorf("second use: %v", err)
	}
	// The character loads with its start items.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ch, sess, err := e.store.Load(ctx, login.CharacterID)
	if err != nil || len(ch.Items) != 1 || ch.Items[0].ID != 501 {
		t.Fatalf("load: %+v %v", ch, err)
	}
	sess.Save(ch, true)

	// Tickets from an untrusted origin are refused.
	if code, _ := e.ticket(c, "http://evil.test"); code != http.StatusForbidden {
		t.Errorf("untrusted origin: %d", code)
	}
	// Another browser without the cookie gets no ticket.
	if code, msg := e.ticket(e.newClient(), clientOrigin); code != http.StatusUnauthorized || msg != "login" {
		t.Errorf("no session: %d %s", code, msg)
	}

	// A ticket made before logout dies with the session.
	_, unused := e.ticket(c, clientOrigin)
	resp, _ := e.submit(c, "/account/characters", "/account/logout", url.Values{})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/login" {
		t.Fatalf("logout: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if _, err := e.redeem(unused); !errors.Is(err, gateway.ErrSessionInvalid) {
		t.Errorf("ticket after logout: %v", err)
	}
	if code, _ := e.ticket(c, clientOrigin); code != http.StatusUnauthorized {
		t.Errorf("ticket after logout: %d", code)
	}
	// The revoked session's cookie no longer opens the characters page.
	resp, _ = c.Get(e.srv.URL + "/account/characters")
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/account/login") {
		t.Errorf("characters after logout: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	_ = resp.Body.Close()

	// Logging in again, in another case, finds the same account and
	// character.
	resp, body = e.login(c, "alice_1", "a fine long password", nil)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: %d %s", resp.StatusCode, body)
	}
	e.play(c, "0")
	_, ticket = e.ticket(c, clientOrigin)
	if again, err := e.redeem(ticket); err != nil || again.CharacterID != login.CharacterID {
		t.Errorf("after login again: %+v %v", again, err)
	}
}

func TestSignupRules(t *testing.T) {
	e := setup(t, nil)
	e.signup(e.c, "Taken", "a fine long password")
	c := e.newClient()
	for _, tc := range []struct {
		name, password, confirm string
		status                  int
		message                 string
	}{
		{"taken", "another long password", "another long password", http.StatusConflict, "That account name is taken."},
		{"ab", "another long password", "another long password", http.StatusBadRequest, "Account names are"},
		{"Newbie", "short", "short", http.StatusBadRequest, "at least 8"},
		{"Newbie", "password123", "password123", http.StatusBadRequest, "too common"},
		{"Newbie", "a fine long password", "a different password", http.StatusBadRequest, "not the same"},
	} {
		resp, body := e.submit(c, "/account/signup", "/account/signup", url.Values{"name": {tc.name}, "password": {tc.password}, "confirm": {tc.confirm}})
		if resp.StatusCode != tc.status || !strings.Contains(body, tc.message) {
			t.Errorf("%s/%s: %d, want %d %q", tc.name, tc.password, resp.StatusCode, tc.status, tc.message)
		}
	}
}

func TestCharacterRules(t *testing.T) {
	e := setup(t, nil)
	e.signup(e.c, "Maker", "a fine long password")
	e.makeCharacter(e.c, "Unique Name", "0")
	other := e.newClient()
	e.signup(other, "Other", "a fine long password")
	for _, tc := range []struct {
		c       *http.Client
		form    url.Values
		status  int
		message string
	}{
		{other, url.Values{"name": {"unique name"}, "slot": {"0"}, "sex": {"m"}, "hair": {"1"}, "hair_color": {"0"}}, http.StatusConflict, "name is taken"},
		{e.c, url.Values{"name": {"Second"}, "slot": {"0"}, "sex": {"m"}, "hair": {"1"}, "hair_color": {"0"}}, http.StatusConflict, "slot is taken"},
		{e.c, url.Values{"name": {"x"}, "slot": {"1"}, "sex": {"m"}, "hair": {"1"}, "hair_color": {"0"}}, http.StatusBadRequest, "Character names are"},
		{e.c, url.Values{"name": {"Second"}, "slot": {"9"}, "sex": {"m"}, "hair": {"1"}, "hair_color": {"0"}}, http.StatusBadRequest, "free slot"},
		{e.c, url.Values{"name": {"Second"}, "slot": {"1"}, "sex": {"x"}, "hair": {"1"}, "hair_color": {"0"}}, http.StatusBadRequest, "female or male"},
		{e.c, url.Values{"name": {"Second"}, "slot": {"1"}, "sex": {"m"}, "hair": {"99"}, "hair_color": {"0"}}, http.StatusBadRequest, "hair style"},
	} {
		resp, body := e.submit(tc.c, "/account/characters", "/account/characters", tc.form)
		if resp.StatusCode != tc.status || !strings.Contains(body, tc.message) {
			t.Errorf("%v: %d, want %d %q", tc.form, resp.StatusCode, tc.status, tc.message)
		}
	}
	// Play only reaches the session's own characters.
	resp, _ := e.submit(other, "/account/characters", "/account/play", url.Values{"slot": {"0"}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("play with an empty slot: %d", resp.StatusCode)
	}
}

func TestLoginFailuresAndCaptcha(t *testing.T) {
	e := setup(t, func(c *account.Config) { c.Captcha = account.DevCaptcha{} })
	e.signup(e.c, "Victim", "the right password")
	c := e.newClient()
	// A wrong password and an unknown name get the same answer.
	_, wrong := e.login(c, "Victim", "a wrong password", nil)
	_, unknown := e.login(c, "Nobody", "a wrong password", nil)
	for _, body := range []string{wrong, unknown} {
		if !strings.Contains(body, "The account name or password is wrong.") {
			t.Errorf("failure message: %s", body)
		}
	}
	e.login(c, "Victim", "a wrong password", nil)
	e.login(c, "Victim", "a wrong password", nil)
	// Three failures: the form now asks for the captcha.
	resp, body := e.login(c, "Victim", "the right password", nil)
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "Complete the check") || !strings.Contains(body, `name="dev_captcha"`) {
		t.Fatalf("no captcha asked: %d %s", resp.StatusCode, body)
	}
	e.login(c, "Victim", "a wrong password", url.Values{"dev_captcha": {"yes"}})
	e.login(c, "Victim", "a wrong password", url.Values{"dev_captcha": {"yes"}})
	// Five failures: the name must wait, even with the right password.
	resp, body = e.login(c, "Victim", "the right password", url.Values{"dev_captcha": {"yes"}})
	if resp.StatusCode != http.StatusTooManyRequests || !strings.Contains(body, "Try again in 1 second") {
		t.Fatalf("no wait: %d %s", resp.StatusCode, body)
	}
	time.Sleep(1100 * time.Millisecond)
	resp, body = e.login(c, "Victim", "the right password", url.Values{"dev_captcha": {"yes"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("right password after the wait: %d %s", resp.StatusCode, body)
	}
}

func TestLoginRateLimitPerAddress(t *testing.T) {
	e := setup(t, func(c *account.Config) {
		c.Limits = account.DefaultLimits
		c.Limits.Login = account.Bucket{Burst: 3, Every: time.Hour}
	})
	c := e.newClient()
	for i := range 3 {
		if resp, _ := e.login(c, "Name"+string(rune('a'+i)), "a wrong password", nil); resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("attempt %d: %d", i+1, resp.StatusCode)
		}
	}
	resp, body := e.login(c, "Named", "a wrong password", nil)
	if resp.StatusCode != http.StatusTooManyRequests || !strings.Contains(body, "from your network") {
		t.Errorf("fourth attempt: %d %s", resp.StatusCode, body)
	}
}

func TestRehashOnLogin(t *testing.T) {
	e := setup(t, nil)
	e.signup(e.c, "Rehash", "a fine long password")
	// The server's parameters change after signup.
	newer := fastParams
	newer.Time = 2
	e2 := *e
	svc, err := account.New(account.Config{
		Pool: e.account, ClientOrigins: []string{clientOrigin}, NewCharacter: newCharacter,
		Hasher: account.NewHasher(newer, 1), Log: slog.New(slog.DiscardHandler),
	})
	if err != nil {
		t.Fatal(err)
	}
	e2.srv = httptest.NewServer(svc)
	defer e2.srv.Close()
	if resp, body := e2.login(e2.newClient(), "Rehash", "a fine long password", nil); resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login: %d %s", resp.StatusCode, body)
	}
	var hash string
	ctx := context.Background()
	if err := e.admin.QueryRow(ctx, "SELECT hash FROM account_passwords").Scan(&hash); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(hash, "$m=64,t=2,p=1$") {
		t.Errorf("hash not renewed: %s", hash)
	}
	// The old parameters still log in, and now match the new hash.
	if resp, _ := e2.login(e2.newClient(), "Rehash", "a fine long password", nil); resp.StatusCode != http.StatusSeeOther {
		t.Errorf("second login: %d", resp.StatusCode)
	}
}

func TestTicketExpires(t *testing.T) {
	e := setup(t, func(c *account.Config) { c.TicketLifetime = time.Second })
	e.signup(e.c, "Slowpoke", "a fine long password")
	e.makeCharacter(e.c, "Slow Hero", "0")
	e.play(e.c, "0")
	_, ticket := e.ticket(e.c, clientOrigin)
	time.Sleep(1200 * time.Millisecond)
	if _, err := e.redeem(ticket); !errors.Is(err, gateway.ErrSessionInvalid) {
		t.Errorf("expired ticket: %v", err)
	}
	// The expired ticket was deleted when it was tried.
	var n int
	if err := e.admin.QueryRow(context.Background(), "SELECT count(*) FROM game_tickets").Scan(&n); err != nil || n != 0 {
		t.Errorf("tickets left: %d %v", n, err)
	}
	// Tokens that are not tickets are refused without a query.
	for _, bad := range []string{"", "Alice", strings.Repeat("A", 43)} {
		if _, err := e.redeem(bad); !errors.Is(err, gateway.ErrSessionInvalid) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestCleanup(t *testing.T) {
	e := setup(t, func(c *account.Config) { c.TicketLifetime = time.Second })
	e.signup(e.c, "Cleaner", "a fine long password")
	e.makeCharacter(e.c, "Clean Hero", "0")
	e.play(e.c, "0")
	e.ticket(e.c, clientOrigin)
	other := e.newClient()
	e.signup(other, "Leaver", "a fine long password")
	e.submit(other, "/account/characters", "/account/logout", url.Values{})
	time.Sleep(1200 * time.Millisecond)
	e.svc.Cleanup(context.Background())
	var sessions, tickets int
	ctx := context.Background()
	_ = e.admin.QueryRow(ctx, "SELECT count(*) FROM account_sessions").Scan(&sessions)
	_ = e.admin.QueryRow(ctx, "SELECT count(*) FROM game_tickets").Scan(&tickets)
	// The revoked session and the expired ticket went. The live session
	// stays.
	if sessions != 1 || tickets != 0 {
		t.Errorf("after cleanup: %d sessions, %d tickets", sessions, tickets)
	}
}

// TestRoles checks what each role may do, with SET ROLE on the pools
// that waserver would use.
func TestRoles(t *testing.T) {
	e := setup(t, nil)
	e.signup(e.c, "Roles", "a fine long password")
	e.makeCharacter(e.c, "Role Hero", "0")
	ctx := context.Background()
	denied := func(pool *pgxpool.Pool, role, sql string) {
		t.Helper()
		_, err := pool.Exec(ctx, sql)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "42501" {
			t.Errorf("%s: %q gave %v, want permission denied", role, sql, err)
		}
	}
	for _, sql := range []string{
		"SELECT hash FROM account_passwords",
		"SELECT token_hash FROM account_sessions",
		"UPDATE account_passwords SET hash = 'x'",
		"INSERT INTO game_tickets (token_hash, session_id, account_id, character_id, expires_at) VALUES ('\\x00', 1, 1, 1, now())",
		"DROP TABLE game_tickets",
	} {
		denied(e.game, "game", sql)
	}
	for _, sql := range []string{
		"UPDATE characters SET zeny = 1000000",
		"DELETE FROM characters",
		"DELETE FROM accounts",
		"UPDATE inventory SET amount = 30000",
		"DELETE FROM inventory",
		"DROP TABLE account_passwords",
	} {
		denied(e.account, "account", sql)
	}
	// The game role can still read what it needs.
	if _, err := e.game.Exec(ctx, "SELECT token_hash FROM game_tickets"); err != nil {
		t.Errorf("game role cannot read tickets: %v", err)
	}
}

// storeCharacters loads characters through the store, as waserver
// does.
type storeCharacters struct{ s *store.Store }

func (s storeCharacters) Find(context.Context, string, zone.Look) (int64, error) {
	return 0, errors.New("tickets name the character")
}

func (s storeCharacters) Load(ctx context.Context, id int64) (*character.Character, zone.Saver, error) {
	c, sess, err := s.s.Load(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return c, sess, nil
}

// TestGatewayRedeemsTicket opens the game with a ticket, over the real
// gateway, and checks that the same ticket cannot open it again.
func TestGatewayRedeemsTicket(t *testing.T) {
	e := setup(t, nil)
	e.signup(e.c, "Gamer", "a fine long password")
	e.makeCharacter(e.c, "Gamer Hero", "0")
	e.play(e.c, "0")
	_, ticket := e.ticket(e.c, clientOrigin)

	g := &path.Grid{Width: 20, Height: 20, Cells: make([]uint8, 400)}
	for i := range g.Cells {
		g.Cells[i] = path.Walkable | path.Shootable
	}
	z := zone.New("map/test", g, rng.FromKey([32]byte{}).Stream(), nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go z.Run(ctx)
	gw := httptest.NewServer(&gateway.Gateway{
		Zone: z, Sessions: account.GameTickets{Store: e.store}, Characters: storeCharacters{e.store},
		Origins: []string{clientOrigin},
	})
	t.Cleanup(gw.Close)
	url := "ws" + strings.TrimPrefix(gw.URL, "http")

	first := hello(t, url, ticket)
	if !first.HasWelcome() {
		t.Fatalf("first use: %v", first)
	}
	second := hello(t, url, ticket)
	if second.GetDisconnect().GetReason() != wathenav1.DisconnectReason_DISCONNECT_REASON_SESSION_INVALID {
		t.Errorf("second use: %v", second)
	}
	// A page from an untrusted origin cannot open the socket.
	dctx, dcancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer dcancel()
	_, resp, err := websocket.Dial(dctx, url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {"http://evil.test"}}})
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Errorf("untrusted origin: %v", err)
	}
	if _, _, err := websocket.Dial(dctx, url, &websocket.DialOptions{HTTPHeader: http.Header{"Origin": {clientOrigin}}}); err != nil {
		t.Errorf("trusted origin: %v", err)
	}
}

// hello opens a connection, sends Hello with a token, and returns the
// first message back.
func hello(t *testing.T, url, token string) *wathenav1.ServerMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.CloseNow() })
	data, _ := proto.Marshal(wathenav1.ClientMessage_builder{Seq: 1, Hello: wathenav1.Hello_builder{
		ProtocolVersion: schema.Version, SessionToken: []byte(token),
	}.Build()}.Build())
	if err := c.Write(ctx, websocket.MessageBinary, data); err != nil {
		t.Fatal(err)
	}
	_, data, err = c.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m := &wathenav1.ServerMessage{}
	if err := proto.Unmarshal(data, m); err != nil {
		t.Fatal(err)
	}
	return m
}
