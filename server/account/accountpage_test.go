package account_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/account"
)

// fakeGame is the game side of the account page for these tests.
type fakeGame struct {
	mu        sync.Mutex
	blocks    map[int64][]string
	names     map[int64]string
	online    bool
	leader    string
	reports   []account.FiledReport
	sanctions []account.AccountSanction
	deleting  []string
	unblocked []string
}

func newFakeGame() *fakeGame {
	return &fakeGame{blocks: map[int64][]string{7: {"Spammer", "Rude One"}}, names: map[int64]string{7: "Alice Hero"}}
}

func (g *fakeGame) Blocks(_ context.Context, acc int64) ([]account.CharacterBlocks, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	var out []account.CharacterBlocks
	for id, n := range g.blocks {
		out = append(out, account.CharacterBlocks{CharacterID: id, Character: g.names[id], Names: n})
	}
	return out, nil
}

func (g *fakeGame) Unblock(_ context.Context, acc, char int64, name string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	list := g.blocks[char]
	for i, n := range list {
		if strings.EqualFold(n, name) {
			g.blocks[char] = append(list[:i:i], list[i+1:]...)
			g.unblocked = append(g.unblocked, name)
			return true, nil
		}
	}
	return false, nil
}

func (g *fakeGame) Reports(context.Context, int64) ([]account.FiledReport, error) {
	return g.reports, nil
}

func (g *fakeGame) Sanctions(context.Context, int64) ([]account.AccountSanction, error) {
	return g.sanctions, nil
}

func (g *fakeGame) Export(context.Context, int64) (*account.GameExport, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.online {
		return nil, account.ErrCharacterOnline
	}
	// Mod variables are data a mod chose to save. Their names are not
	// fields of the format, even when one sounds like a secret.
	return &account.GameExport{
		Variables: []account.ExportVariable{{Mod: "example.bank", Name: "session_count", Value: json.RawMessage(`2`)}},
		Characters: []account.ExportCharacter{{ID: 7, Name: "Alice Hero", BlockList: g.blocks[7],
			Variables: []account.ExportVariable{{Mod: "example.lantern", Name: "lit", Value: json.RawMessage(`true`)}}}},
		Chat: []account.ExportChatLine{{At: time.Now(), Character: "Alice Hero", Kind: "local", Map: "map/test", Text: "hello"}}}, nil
}

func (g *fakeGame) LeadsGuild(context.Context, int64) (string, string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.leader == "" {
		return "", "", nil
	}
	return "Alice Hero", g.leader, nil
}

func (g *fakeGame) Deleting(_ context.Context, _ int64, text string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.deleting = append(g.deleting, text)
}

func withGame(g *fakeGame) func(*account.Config) {
	return func(c *account.Config) {
		c.Game = g
		c.Privacy = account.Privacy{Contact: "privacy@example.com", Text: "We run this server for fun.\n\nAsk us <anything>.", ChatLogDays: 45}
	}
}

// post posts a form with the CSRF token of the page from, and returns
// the answer with its body.
func (e *env) post(c *http.Client, from, path string, form url.Values) (*http.Response, string) {
	e.t.Helper()
	return e.submit(c, from, path, form)
}

func TestAccountPagesNeedALogin(t *testing.T) {
	e := setup(t, withGame(newFakeGame()))
	for _, path := range []string{"/account/safety", "/account/data"} {
		resp, err := e.c.Get(e.srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/login" {
			t.Errorf("%s without a session: %d %s", path, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	// A post without a session goes to the login page too, once its CSRF
	// token is right.
	for _, path := range []string{"/account/safety/unblock", "/account/data/export", "/account/data/delete", "/account/data/cancel"} {
		resp, _ := e.submit(e.c, "/account/login", path, url.Values{"character": {"7"}, "name": {"x"}})
		if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/login" {
			t.Errorf("%s without a session: %d %s", path, resp.StatusCode, resp.Header.Get("Location"))
		}
	}
	// The privacy notice needs no login.
	body, _ := e.page(e.c, "/account/privacy")
	if !strings.Contains(body, "privacy@example.com") {
		t.Errorf("privacy page: %s", body)
	}
}

func TestAccountPostsNeedCSRF(t *testing.T) {
	g := newFakeGame()
	e := setup(t, withGame(g))
	e.signup(e.c, "Alice_1", password)
	for _, path := range []string{"/account/safety/unblock", "/account/data/export", "/account/data/delete", "/account/data/cancel"} {
		resp, err := e.c.PostForm(e.srv.URL+path, url.Values{"character": {"7"}, "name": {"Spammer"}, "csrf": {"forged"}})
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s with a forged token: %d", path, resp.StatusCode)
		}
	}
	if len(g.unblocked) != 0 || len(g.deleting) != 0 {
		t.Errorf("a forged post changed something: %v %v", g.unblocked, g.deleting)
	}
}

func TestSafetyPage(t *testing.T) {
	g := newFakeGame()
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	g.reports = []account.FiledReport{
		{ID: 3, At: at, Character: "Alice Hero", Target: "Ruffian", Kind: account.ReportLine, Reason: "insults", Closed: true, ActionTaken: true},
		{ID: 2, At: at, Character: "Alice Hero", Target: "Bot1", Kind: account.ReportPlayer, Reason: "botting", Closed: true},
		{ID: 1, At: at, Character: "Alice Hero", Target: "Ugly Guild", Kind: account.ReportGuildEmblem, Reason: "rude emblem"},
	}
	g.sanctions = []account.AccountSanction{
		{ID: 9, Kind: "mute", Character: "Alice Hero", Reason: "flooding", Start: at, End: at.Add(time.Hour)},
		{ID: 8, Kind: "ban", Reason: "repeat offender", Start: at, Permanent: true, Lifted: at.Add(2 * time.Hour)},
	}
	e := setup(t, withGame(g))
	e.signup(e.c, "Alice_1", password)
	body, _ := e.page(e.c, "/account/safety")
	for _, want := range []string{"Spammer", "Rude One", "Unblock", "Chat line: Ruffian", "Action taken", "No action", "Open",
		"Guild emblem: Ugly Guild", "flooding", "Mute", "Ended", "Never", "Lifted 2026-10-01 14:00 UTC", "The account"} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q", want)
		}
	}
	// Unblock a name, in another case.
	resp, _ := e.post(e.c, "/account/safety", "/account/safety/unblock", url.Values{"character": {"7"}, "name": {"spammer"}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/safety?notice=unblocked" {
		t.Fatalf("unblock: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	body, _ = e.page(e.c, "/account/safety?notice=unblocked")
	if strings.Contains(body, ">Spammer<") || !strings.Contains(body, "The name is off the block list.") {
		t.Errorf("after the unblock: %s", body)
	}
	// A name the list does not hold.
	resp, _ = e.post(e.c, "/account/safety", "/account/safety/unblock", url.Values{"character": {"7"}, "name": {"Nobody"}})
	if resp.Header.Get("Location") != "/account/safety?notice=not-blocked" {
		t.Errorf("unblock of a name not blocked: %s", resp.Header.Get("Location"))
	}
	resp, _ = e.post(e.c, "/account/safety", "/account/safety/unblock", url.Values{"character": {"x"}, "name": {"Nobody"}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unblock without a character: %d", resp.StatusCode)
	}
}

func TestAccountPageRateLimit(t *testing.T) {
	g := newFakeGame()
	e := setup(t, func(c *account.Config) {
		withGame(g)(c)
		c.Limits = account.DefaultLimits
		c.Limits.Account = account.Bucket{Burst: 1, Every: time.Hour}
	})
	e.signup(e.c, "Alice_1", password)
	resp, _ := e.post(e.c, "/account/safety", "/account/safety/unblock", url.Values{"character": {"7"}, "name": {"Spammer"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("first unblock: %d", resp.StatusCode)
	}
	resp, body := e.post(e.c, "/account/safety", "/account/safety/unblock", url.Values{"character": {"7"}, "name": {"Rude One"}})
	if resp.StatusCode != http.StatusTooManyRequests || !strings.Contains(body, "Too many changes") {
		t.Errorf("second unblock: %d %s", resp.StatusCode, body)
	}
	resp, _ = e.post(e.c, "/account/data", "/account/data/delete", url.Values{"name": {"Alice_1"}})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("a deletion request over the limit: %d", resp.StatusCode)
	}
	if len(g.unblocked) != 1 || len(g.deleting) != 0 {
		t.Errorf("unblocked %v, deleting %v", g.unblocked, g.deleting)
	}
}

// download posts the export form and returns the answer's status,
// headers and body.
func (e *env) download(c *http.Client) (*http.Response, []byte) {
	e.t.Helper()
	_, token := e.page(c, "/account/data")
	resp, err := c.PostForm(e.srv.URL+"/account/data/export", url.Values{"csrf": {token}})
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func TestExport(t *testing.T) {
	g := newFakeGame()
	e := setup(t, withGame(g))
	e.signup(e.c, "Alice_1", password)

	// An old login is asked for the password first.
	e.ageSessions()
	resp, _ := e.download(e.c)
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/account/reauth?") {
		t.Fatalf("export with an old login: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	resp, body := e.submit(e.c, "/account/reauth?next=/account/data", "/account/reauth", url.Values{"password": {password}, "next": {"/account/data"}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/data" {
		t.Fatalf("reauth: %d %s", resp.StatusCode, body)
	}

	// A character in the game refuses it, and the try is not used up.
	g.online = true
	resp, b := e.download(e.c)
	if resp.StatusCode != http.StatusConflict || !bytes.Contains(b, []byte("Log out every character")) {
		t.Fatalf("export while online: %d %s", resp.StatusCode, b)
	}
	g.online = false

	resp, b = e.download(e.c)
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") ||
		!strings.Contains(resp.Header.Get("Content-Disposition"), `attachment; filename="wathena-Alice_1-`) {
		t.Fatalf("export: %d %v %s", resp.StatusCode, resp.Header, b)
	}
	var ex map[string]any
	if err := json.Unmarshal(b, &ex); err != nil {
		t.Fatal(err)
	}
	if ex["format"] != account.ExportFormat || ex["version"] != float64(account.ExportVersion) {
		t.Errorf("format %v version %v", ex["format"], ex["version"])
	}
	acc, _ := ex["account"].(map[string]any)
	if acc["name"] != "Alice_1" || acc["two_factor"] != false || acc["email"] != nil {
		t.Errorf("account %v", acc)
	}
	if logins, _ := ex["logins"].([]any); len(logins) == 0 {
		t.Errorf("no login records: %v", ex["logins"])
	}
	if chars, _ := ex["characters"].([]any); len(chars) != 1 {
		t.Errorf("characters %v", ex["characters"])
	}
	if vars, _ := ex["variables"].([]any); len(vars) != 1 || vars[0].(map[string]any)["value"] != float64(2) {
		t.Errorf("account variables %v", ex["variables"])
	}
	if chars, _ := ex["characters"].([]any); len(chars) == 1 {
		if vars, _ := chars[0].(map[string]any)["variables"].([]any); len(vars) != 1 || vars[0].(map[string]any)["value"] != true {
			t.Errorf("character variables %v", chars[0])
		}
	}
	for _, k := range []string{"storages", "currencies", "variables", "chat", "item_log", "zeny_log", "reports", "sanctions", "truncated"} {
		if _, ok := ex[k].([]any); !ok {
			t.Errorf("%s is %v, not a list", k, ex[k])
		}
	}

	// Once a day.
	resp, b = e.download(e.c)
	if resp.StatusCode != http.StatusTooManyRequests || !bytes.Contains(b, []byte("once a day")) {
		t.Errorf("second export: %d %s", resp.StatusCode, b)
	}
	body, _ = e.page(e.c, "/account/data")
	if !strings.Contains(body, "The next download is possible after") {
		t.Errorf("data page after an export: %s", body)
	}
	// A day later it works again.
	if _, err := e.admin.Exec(context.Background(), "UPDATE account_exports SET exported_at = now() - interval '25 hours'"); err != nil {
		t.Fatal(err)
	}
	if resp, b = e.download(e.c); resp.StatusCode != http.StatusOK {
		t.Errorf("export a day later: %d %s", resp.StatusCode, b)
	}
}

// TestExportHasNoSecrets: the export of an account with a password,
// TOTP, recovery codes, a passkey, sessions and a game ticket holds none
// of them.
func TestExportHasNoSecrets(t *testing.T) {
	clock := &fakeClock{now: time.Now()}
	e := setup(t, func(c *account.Config) {
		twoFactor(t, clock)(c)
		withGame(newFakeGame())(c)
	})
	c := e.c
	e.signup(c, "Alice_1", password)
	_, codes := e.enableTOTP(c, clock)
	e.makeCharacter(c, "Alice Hero", "0")
	e.play(c, "0")
	if status, _ := e.ticket(c, clientOrigin); status != http.StatusOK {
		t.Fatalf("ticket: %d", status)
	}
	ctx := context.Background()
	var id int64
	if err := e.admin.QueryRow(ctx, "SELECT id FROM accounts WHERE name = 'Alice_1'").Scan(&id); err != nil {
		t.Fatal(err)
	}
	credential := bytes.Repeat([]byte{0xAB, 0x13}, 16)
	key := bytes.Repeat([]byte{0xC4, 0x5E, 0x91}, 30)
	if _, err := e.admin.Exec(ctx, `INSERT INTO account_webauthn_users (account_id, user_handle) VALUES ($1, $2)`,
		id, bytes.Repeat([]byte{0x77}, 32)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.admin.Exec(ctx, `INSERT INTO account_passkeys (account_id, rp_id, credential_id, public_key, flags, name)
		VALUES ($1, 'example.com', $2, $3, 5, 'Phone')`, id, credential, key); err != nil {
		t.Fatal(err)
	}

	resp, b := e.download(c)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("export: %d %s", resp.StatusCode, b)
	}
	var secrets [][]byte
	var hash string
	if err := e.admin.QueryRow(ctx, "SELECT hash FROM account_passwords WHERE account_id = $1", id).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	secrets = append(secrets, []byte(hash), []byte(password))
	for _, code := range codes {
		secrets = append(secrets, []byte(code), []byte(strings.ReplaceAll(code, "-", "")))
	}
	var totp []byte
	if err := e.admin.QueryRow(ctx, "SELECT secret FROM account_totp WHERE account_id = $1", id).Scan(&totp); err != nil {
		t.Fatal(err)
	}
	binary := [][]byte{totp, credential, key, bytes.Repeat([]byte{0x77}, 32)}
	for _, q := range []string{
		"SELECT code_hash FROM account_recovery_codes WHERE account_id = $1",
		"SELECT token_hash FROM account_sessions WHERE account_id = $1",
		"SELECT token_hash FROM game_tickets WHERE account_id = $1",
	} {
		rows, err := e.admin.Query(ctx, q, id)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var v []byte
			if err := rows.Scan(&v); err != nil {
				t.Fatal(err)
			}
			binary = append(binary, v)
		}
		rows.Close()
	}
	for _, v := range binary {
		secrets = append(secrets, []byte(hex.EncodeToString(v)), []byte(base64.StdEncoding.EncodeToString(v)),
			[]byte(base64.RawURLEncoding.EncodeToString(v)))
	}
	// The session cookie's token.
	u, _ := url.Parse(e.srv.URL)
	for _, ck := range c.Jar.Cookies(u) {
		secrets = append(secrets, []byte(ck.Value))
	}
	for _, s := range secrets {
		if len(s) >= 8 && bytes.Contains(b, s) {
			t.Errorf("the export holds a secret: %s", s)
		}
	}
	// No field of the format names a secret. The check reads the keys of
	// the JSON objects, so data such as a mod variable's name may hold
	// these words (decision 0036).
	var tree any
	if err := json.Unmarshal(b, &tree); err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	var walk func(v any)
	walk = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, x := range v {
				keys[strings.ToLower(k)] = true
				walk(x)
			}
		case []any:
			for _, x := range v {
				walk(x)
			}
		}
	}
	walk(tree)
	if !keys["variables"] || !keys["mod"] {
		t.Errorf("the export has no variables: %v", keys)
	}
	for k := range keys {
		for _, field := range []string{"hash", "secret", "recovery", "passkey", "token", "credential", "public_key", "session"} {
			if strings.HasPrefix(k, field) {
				t.Errorf("the export has the field %q, which starts with %q", k, field)
			}
		}
	}
	var ex account.Export
	if err := json.Unmarshal(b, &ex); err != nil {
		t.Fatal(err)
	}
	if !ex.Account.TwoFactor {
		t.Error("two_factor is false with TOTP on")
	}
}

func TestDeletion(t *testing.T) {
	g := newFakeGame()
	e := setup(t, withGame(g))
	c := e.c
	e.signup(c, "Alice_1", password)
	e.makeCharacter(c, "Alice Hero", "0")
	e.play(c, "0")
	ctx := context.Background()
	pending := func() bool {
		var n int
		if err := e.admin.QueryRow(ctx, "SELECT count(*) FROM account_deletions").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n > 0
	}

	// An old login is asked for the password first.
	e.ageSessions()
	resp, _ := e.post(c, "/account/data", "/account/data/delete", url.Values{"name": {"Alice_1"}})
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/account/reauth?") {
		t.Fatalf("delete with an old login: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	e.submit(c, "/account/reauth?next=/account/data", "/account/reauth", url.Values{"password": {password}, "next": {"/account/data"}})

	// The wrong name.
	resp, body := e.post(c, "/account/data", "/account/data/delete", url.Values{"name": {"Alice_2"}})
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(body, "Type your account name") {
		t.Errorf("wrong name: %d %s", resp.StatusCode, body)
	}
	// A guild master with members.
	g.leader = "Knights"
	resp, body = e.post(c, "/account/data", "/account/data/delete", url.Values{"name": {"alice_1"}})
	if resp.StatusCode != http.StatusConflict || !strings.Contains(body, "Alice Hero leads the guild Knights") {
		t.Errorf("guild master: %d %s", resp.StatusCode, body)
	}
	if pending() || len(g.deleting) != 0 {
		t.Fatal("a refused request was recorded")
	}
	g.leader = ""

	resp, body = e.post(c, "/account/data", "/account/data/delete", url.Values{"name": {"alice_1"}})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/data?notice=deletion-requested" {
		t.Fatalf("delete: %d %s", resp.StatusCode, body)
	}
	if !pending() || len(g.deleting) != 1 || !strings.Contains(g.deleting[0], "set to be deleted on") {
		t.Fatalf("after the request: %v %v", pending(), g.deleting)
	}
	var after time.Time
	if err := e.admin.QueryRow(ctx, "SELECT delete_after FROM account_deletions").Scan(&after); err != nil {
		t.Fatal(err)
	}
	if d := time.Until(after); d < 14*24*time.Hour-time.Minute || d > 14*24*time.Hour+time.Minute {
		t.Errorf("deleted in %v", d)
	}

	// Play is blocked: no ticket, no Play button, and the date shows.
	if status, msg := e.ticket(c, clientOrigin); status != http.StatusForbidden || msg != "deleting" {
		t.Errorf("ticket while deleting: %d %s", status, msg)
	}
	resp, body = e.post(c, "/account/characters", "/account/play", url.Values{"slot": {"0"}})
	if resp.StatusCode != http.StatusForbidden || !strings.Contains(body, "set to be deleted on") {
		t.Errorf("play while deleting: %d %s", resp.StatusCode, body)
	}
	// A new login shows the date and the cancel button.
	e.logout(c)
	e.login(c, "Alice_1", password, nil)
	body, _ = e.page(c, "/account/characters")
	if !strings.Contains(body, "Your account is set to be deleted on <strong>"+after.UTC().Format("2006-01-02")) ||
		!strings.Contains(body, `action="/account/data/cancel"`) || strings.Contains(body, ">Play<") {
		t.Errorf("characters page while deleting: %s", body)
	}

	// Cancel.
	resp, _ = e.post(c, "/account/characters", "/account/data/cancel", url.Values{})
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/account/data?notice=deletion-cancelled" {
		t.Fatalf("cancel: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if pending() {
		t.Error("the request is still there")
	}
	e.play(c, "0")
	if status, _ := e.ticket(c, clientOrigin); status != http.StatusOK {
		t.Errorf("ticket after the cancel: %d", status)
	}
}

// TestBannedAccountMayAskForDeletion: a ban refuses logins of the game,
// not the account pages of a session that has one.
func TestBannedAccountMayAskForDeletion(t *testing.T) {
	g := newFakeGame()
	e := setup(t, withGame(g))
	e.signup(e.c, "Alice_1", password)
	ctx := context.Background()
	if _, err := e.admin.Exec(ctx, `INSERT INTO sanctions (account_id, kind, reason, started_at, ends_at)
		SELECT id, 'ban', 'cheating', now(), 'infinity' FROM accounts WHERE name = 'Alice_1'`); err != nil {
		t.Fatal(err)
	}
	resp, body := e.post(e.c, "/account/data", "/account/data/delete", url.Values{"name": {"Alice_1"}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Errorf("a banned account's request: %d %s", resp.StatusCode, body)
	}
}

func TestPrivacyPage(t *testing.T) {
	e := setup(t, withGame(newFakeGame()))
	body, _ := e.page(e.c, "/account/privacy")
	for _, want := range []string{"privacy@example.com", "We run this server for fun.", "Ask us &lt;anything&gt;.", "45 days",
		"90 days", "180 days", "2 years", "14 days"} {
		if !strings.Contains(body, want) {
			t.Errorf("the privacy page lacks %q: %s", want, body)
		}
	}
	if strings.Contains(body, "<anything>") {
		t.Error("the owner's text is not escaped")
	}
}

func TestAccountPagesWithoutGame(t *testing.T) {
	e := setup(t, nil)
	e.signup(e.c, "Alice_1", password)
	body, _ := e.page(e.c, "/account/safety")
	if !strings.Contains(body, "not available on this server") {
		t.Errorf("safety page without a game: %s", body)
	}
	resp, b := e.download(e.c)
	var ex account.Export
	if resp.StatusCode != http.StatusOK || json.Unmarshal(b, &ex) != nil || ex.Account.Name != "Alice_1" || ex.Characters == nil {
		t.Errorf("export without a game: %d %s", resp.StatusCode, b)
	}
}

// TestOwnerCannotAskForDeletion: an account that staff.owners names
// keeps its name, so no new account can take it and become an owner.
func TestOwnerCannotAskForDeletion(t *testing.T) {
	g := newFakeGame()
	e := setup(t, func(c *account.Config) {
		withGame(g)(c)
		c.Owners = []string{"alice_1"}
	})
	e.signup(e.c, "Alice_1", password)
	resp, body := e.post(e.c, "/account/data", "/account/data/delete", url.Values{"name": {"Alice_1"}})
	if resp.StatusCode != http.StatusConflict || !strings.Contains(body, "staff.owners") || len(g.deleting) != 0 {
		t.Errorf("an owner's request: %d %s", resp.StatusCode, body)
	}
}
