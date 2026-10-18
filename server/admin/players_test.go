package admin_test

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	adminv1 "github.com/lxsmnsyc/wathena/schema/gen/go/wathena/admin/v1"
	"github.com/lxsmnsyc/wathena/server/admin"
	"github.com/lxsmnsyc/wathena/server/item"
	"github.com/lxsmnsyc/wathena/server/store"
)

// withRole gives an account a role in the database and logs it in with
// TOTP.
func (e *env) withRole(name, role string) *http.Client {
	e.t.Helper()
	c := e.signup(name)
	e.enableTOTP(c, name)
	if _, err := e.admin.Exec(context.Background(), "INSERT INTO staff_roles (account_id, role) SELECT id, $1 FROM accounts WHERE name = $2",
		role, name); err != nil {
		e.t.Fatal(err)
	}
	return e.loginTOTP(name)
}

// character adds a character to an account and returns its ID.
func (e *env) character(account, name string) int64 {
	e.t.Helper()
	var id int64
	err := e.admin.QueryRow(context.Background(), `INSERT INTO characters (account_id, slot, name, job, sex, base_level, job_level,
		str, agi, vit, int, dex, luk, hp, sp, map, x, y)
		SELECT a.id, (SELECT count(*) FROM characters c WHERE c.account_id = a.id), $2, 0, 1, 10, 5, 1, 1, 1, 1, 1, 1, 40, 11, 'map/prontera', 150, 160
		FROM accounts a WHERE a.name = $1 RETURNING id`, account, name).Scan(&id)
	if err != nil {
		e.t.Fatal(err)
	}
	return id
}

// accountID is an account's ID.
func (e *env) accountID(name string) int64 {
	e.t.Helper()
	var id int64
	if err := e.admin.QueryRow(context.Background(), "SELECT id FROM accounts WHERE name = $1", name).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// get calls a GET and decodes its answer.
func get[T proto.Message](e *env, c *http.Client, path string, m T) T {
	e.t.Helper()
	status, b := e.call(c, "GET", path, "", nil)
	if status != http.StatusOK {
		e.t.Fatalf("GET %s: %d %s", path, status, b)
	}
	if err := protojson.Unmarshal(b, m); err != nil {
		e.t.Fatal(err)
	}
	return m
}

func accountNames(p *adminv1.PlayerSearch) string {
	var out []string
	for _, a := range p.GetAccounts() {
		out = append(out, a.GetAccount())
	}
	return strings.Join(out, ",")
}

// actions are the audit rows' actions and targets, oldest first.
func (e *env) actions() []string {
	e.t.Helper()
	rows, err := e.admin.Query(context.Background(), "SELECT action || ' ' || target FROM staff_audit ORDER BY id")
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

// TestPlayerSearch finds accounts by name, ID, serial and IP range,
// with pages, permissions and the audit rows of IP searches (review
// decision 34).
func TestPlayerSearch(t *testing.T) {
	e := setup(t, options{})
	owner := e.staffer("Owner")
	e.signup("Alice")
	e.signup("Alfred")
	e.signup("Bob_x")
	alice := e.character("Alice", "Wonder")
	bob := e.character("Bob_x", "Alchemy")
	e.world.online = map[int64]string{bob: "geffen"}

	// A name prefix, in any case, of an account or of a character.
	p := get(e, owner, "players?name=AL", &adminv1.PlayerSearch{})
	if got := accountNames(p); got != "Alice,Alfred,Bob_x" {
		t.Errorf("name al: %q", got)
	}
	// Bob's character is online where the world says; Alice's is where
	// it was saved.
	for _, a := range p.GetAccounts() {
		for _, c := range a.GetCharacters() {
			switch c.GetId() {
			case bob:
				if !c.GetOnline() || c.GetPosition().GetMap() != "geffen" {
					t.Errorf("bob's character %v", c)
				}
			case alice:
				if c.GetOnline() || c.GetPosition().GetMap() != "prontera" || c.GetPosition().GetX() != 150 || c.GetName() != "Wonder" ||
					c.GetBaseLevel() != 10 || c.GetJobName() != "Novice" {
					t.Errorf("alice's character %v", c)
				}
			}
		}
		if a.GetLastLogin() == nil {
			t.Errorf("%s has no last login", a.GetAccount())
		}
	}
	if got := accountNames(get(e, owner, "players?name=won", &adminv1.PlayerSearch{})); got != "Alice" {
		t.Errorf("name won: %q", got)
	}
	// LIKE's wildcards are plain characters.
	if got := accountNames(get(e, owner, "players?name=%25", &adminv1.PlayerSearch{})); got != "" {
		t.Errorf("name %%: %q", got)
	}
	if got := accountNames(get(e, owner, "players?name=bob_", &adminv1.PlayerSearch{})); got != "Bob_x" {
		t.Errorf("name bob_: %q", got)
	}
	// Pages of one.
	p = get(e, owner, "players?name=al&limit=1", &adminv1.PlayerSearch{})
	if accountNames(p) != "Alice" || p.GetNextCursor() == "" {
		t.Fatalf("page 1: %q %q", accountNames(p), p.GetNextCursor())
	}
	p = get(e, owner, "players?name=al&limit=2&cursor="+p.GetNextCursor(), &adminv1.PlayerSearch{})
	if accountNames(p) != "Alfred,Bob_x" || p.GetNextCursor() != "" {
		t.Errorf("page 2: %q %q", accountNames(p), p.GetNextCursor())
	}
	// An ID of an account, or of a character.
	if got := accountNames(get(e, owner, "players?id="+strconv.FormatInt(e.accountID("Alfred"), 10), &adminv1.PlayerSearch{})); got != "Alfred" {
		t.Errorf("account ID: %q", got)
	}
	if got := accountNames(get(e, owner, "players?id="+strconv.FormatInt(bob, 10), &adminv1.PlayerSearch{})); !strings.Contains(got, "Bob_x") {
		t.Errorf("character ID: %q", got)
	}
	// A serial finds its holder's account.
	serial := item.NewSerial().String()
	e.world.holder = adminv1.Owner_builder{Kind: "inventory", Id: alice, Name: "Wonder", CharacterId: alice, AccountId: e.accountID("Alice")}.Build()
	p = get(e, owner, "players?serial="+serial, &adminv1.PlayerSearch{})
	if accountNames(p) != "Alice" || p.GetHolder().GetName() != "Wonder" {
		t.Errorf("serial: %q %v", accountNames(p), p.GetHolder())
	}
	e.world.holder = nil
	if p := get(e, owner, "players?serial="+serial, &adminv1.PlayerSearch{}); len(p.GetAccounts()) != 0 || p.GetHolder() != nil {
		t.Errorf("gone serial: %v", p)
	}
	// Name and ID searches write no audit row. A serial search shows
	// who holds an item, so it writes one (review decision 51).
	if got := e.actions(); !slices.Equal(got, []string{"players.search_serial " + serial, "players.search_serial " + serial}) {
		t.Errorf("audit rows after searches: %q", got)
	}
	// The test server's logins come from 127.0.0.1.
	p = get(e, owner, "players?ip=127.0.0.0/8", &adminv1.PlayerSearch{})
	if got := accountNames(p); got != "Owner,Alice,Alfred,Bob_x" {
		t.Errorf("ip: %q", got)
	}
	if l := p.GetAccounts()[1].GetLogins(); len(l) == 0 || l[0].GetIp() != "127.0.0.1" || l[0].GetAt() == nil {
		t.Errorf("logins %v", l)
	}
	if got := accountNames(get(e, owner, "players?ip=203.0.113.7", &adminv1.PlayerSearch{})); got != "" {
		t.Errorf("other range: %q", got)
	}
	if got := e.actions(); len(got) < 2 || !slices.Equal(got[2:], []string{"players.search_ip 127.0.0.0/8", "players.search_ip 203.0.113.7/32"}) {
		t.Errorf("audit rows of IP searches: %q", got)
	}
	for _, q := range []string{"", "?name=a&id=1", "?id=x", "?id=0", "?serial=nope", "?ip=300.1.1.1", "?name=a&limit=0",
		"?name=a&limit=51", "?name=a&cursor=x", "?name=" + strings.Repeat("a", 25)} {
		e.errorCode(owner, "GET", "players"+q, "", nil, http.StatusBadRequest, "bad_request")
	}
	// Every default role reads players. Only admins search by IP.
	mod := e.withRole("Moddy", "moderator")
	get(e, mod, "players?name=al", &adminv1.PlayerSearch{})
	e.errorCode(mod, "GET", "players?ip=127.0.0.1", "", nil, http.StatusForbidden, "forbidden")
	gm := e.withRole("Gamem", "game_master")
	e.errorCode(gm, "GET", "players?ip=127.0.0.1", "", nil, http.StatusForbidden, "forbidden")
	e.errorCode(e.signup("Nobody"), "GET", "players?name=al", "", nil, http.StatusForbidden, "forbidden")
	e.errorCode(e.browser(), "GET", "players?name=al", "", nil, http.StatusUnauthorized, "login")
}

// TestCharacterDetail reads a character through the world, with its
// account's name, and writes an audit row.
func TestCharacterDetail(t *testing.T) {
	e := setup(t, options{})
	e.signup("Alice")
	id := e.character("Alice", "Wonder")
	mod := e.withRole("Moddy", "moderator")
	d := get(e, mod, "characters/"+strconv.FormatInt(id, 10), &adminv1.CharacterDetail{})
	if d.GetId() != id || d.GetAccount() != "Alice" || d.GetName() != "from the world" {
		t.Errorf("detail %v", d)
	}
	if got := e.actions(); !slices.Equal(got, []string{"players.view Wonder"}) {
		t.Errorf("audit rows %q", got)
	}
	e.errorCode(mod, "GET", "characters/999999", "", nil, http.StatusNotFound, "not_found")
	e.errorCode(mod, "GET", "characters/x", "", nil, http.StatusBadRequest, "bad_request")
	e.errorCode(mod, "GET", "characters/0", "", nil, http.StatusBadRequest, "bad_request")
	e.errorCode(e.signup("Nobody"), "GET", "characters/"+strconv.FormatInt(id, 10), "", nil, http.StatusForbidden, "forbidden")
}

// TestLogsAndHistory checks the item history and the character logs
// calls: items.history, their queries and their errors.
func TestLogsAndHistory(t *testing.T) {
	e := setup(t, options{})
	gm := e.withRole("Gamem", "game_master")
	mod := e.withRole("Moddy", "moderator")
	serial := item.NewSerial().String()
	e.errorCode(mod, "GET", "items/"+serial+"/history", "", nil, http.StatusForbidden, "forbidden")
	e.errorCode(mod, "GET", "characters/5/logs", "", nil, http.StatusForbidden, "forbidden")
	e.errorCode(gm, "GET", "items/"+serial+"/history", "", nil, http.StatusNotFound, "not_found")
	e.world.history = adminv1.ItemHistory_builder{Moves: []*adminv1.ItemMove{adminv1.ItemMove_builder{Action: "drop"}.Build()}}.Build()
	if h := get(e, gm, "items/"+strings.ToUpper(serial)+"/history", &adminv1.ItemHistory{}); h.GetSerial() != serial || len(h.GetMoves()) != 1 {
		t.Errorf("history %v", h)
	}
	e.errorCode(gm, "GET", "items/nope/history", "", nil, http.StatusBadRequest, "bad_request")
	l := get(e, gm, "characters/5/logs?limit=10&cursor=abc&since=2026-01-01&until=2026-02-01T00:00:00Z", &adminv1.CharacterLogs{})
	if l.GetNextCursor() != "next" {
		t.Errorf("logs %v", l)
	}
	q := e.world.logs[0]
	if q.Limit != 10 || q.Cursor != "abc" || q.Since.Year() != 2026 || q.Until.Month() != 2 {
		t.Errorf("log query %+v", q)
	}
	get(e, gm, "characters/5/logs", &adminv1.CharacterLogs{})
	if q := e.world.logs[1]; q.Limit != admin.DefaultLogPage || !q.Since.IsZero() {
		t.Errorf("default log query %+v", q)
	}
	e.errorCode(gm, "GET", "characters/404/logs", "", nil, http.StatusNotFound, "not_found")
	for _, q := range []string{"?limit=0", "?limit=101", "?since=soon"} {
		e.errorCode(gm, "GET", "characters/5/logs"+q, "", nil, http.StatusBadRequest, "bad_request")
	}
	e.errorCode(gm, "GET", "characters/x/logs", "", nil, http.StatusBadRequest, "bad_request")
	// Each history read, and the first page of each log search, writes
	// an audit row (review decision 51). A later page, which comes with
	// a cursor, and a failed read write none.
	if got := e.actions(); !slices.Equal(got, []string{"items.history " + serial, "characters.logs 5"}) {
		t.Errorf("audit rows of the reads: %q", got)
	}
}

func restoreReq(character int64, reason string, zeny int64, serials ...string) *adminv1.RestoreRequest {
	r := adminv1.RestoreRequest_builder{CharacterId: character, Reason: reason, Zeny: zeny}
	for _, s := range serials {
		r.Items = append(r.Items, adminv1.RestoreItem_builder{Serial: s}.Build())
	}
	return r.Build()
}

// TestRestoreCall checks POST /restore: items.restore, items.create for
// zeny, the CSRF token, a recent login, the body's checks, and the
// world's errors.
func TestRestoreCall(t *testing.T) {
	e := setup(t, options{roles: map[string][]string{"admin": {"staff.*", "audit.*", "players.*", "items.*"},
		"restorer": {"players.read", "items.history", "items.restore"}, "game_master": {"players.read", "items.history"}}})
	adm := e.withRole("Admin1", "admin")
	csrf := e.me(adm).GetCsrfToken()
	s1, s2 := item.NewSerial().String(), item.NewSerial().String()
	ok := restoreReq(42, "stolen by a scam", 0, s1, s2)
	e.errorCode(adm, "POST", "restore", "", ok, http.StatusForbidden, "csrf")
	status, b := e.call(adm, "POST", "restore", csrf, ok)
	res := &adminv1.RestoreResult{}
	_ = protojson.Unmarshal(b, res)
	if status != http.StatusOK || res.GetMailId() != 7 {
		t.Fatalf("restore: %d %s", status, b)
	}
	r := e.world.restores[0]
	if r.Character != 42 || r.Reason != "stolen by a scam" || !r.MayCreate || len(r.Items) != 2 || r.Items[0].Serial.String() != s1 ||
		r.AccountName != "Admin1" || r.Role != "admin" {
		t.Errorf("restore passed %+v", r)
	}
	for _, bad := range []*adminv1.RestoreRequest{
		restoreReq(42, "", 0, s1),
		restoreReq(42, "  ", 0, s1),
		restoreReq(0, "x", 0, s1),
		restoreReq(42, "x", 0),
		restoreReq(42, "x", 0, "nope"),
		restoreReq(42, "x", 0, s1, s1),
		restoreReq(42, "x", -1, s1),
		restoreReq(42, strings.Repeat("r", 256), 0, s1),
		restoreReq(42, "x", 0, func() []string {
			var out []string
			for range admin.MaxRestoreItems + 1 {
				out = append(out, item.NewSerial().String())
			}
			return out
		}()...),
	} {
		e.errorCode(adm, "POST", "restore", csrf, bad, http.StatusBadRequest, "bad_request")
	}
	// The world's errors keep their codes.
	e.world.restoreErr = &admin.WorldError{Code: "conflict", Message: "The items kept moving."}
	e.errorCode(adm, "POST", "restore", csrf, ok, http.StatusConflict, "conflict")
	e.world.restoreErr = &admin.WorldError{Code: "forbidden", Message: "A copy needs items.create."}
	e.errorCode(adm, "POST", "restore", csrf, ok, http.StatusForbidden, "forbidden")
	e.world.restoreErr = nil
	// Without items.create, no zeny, and the world refuses copies.
	restorer := e.withRole("Restorer", "restorer")
	rc := e.me(restorer).GetCsrfToken()
	e.errorCode(restorer, "POST", "restore", rc, restoreReq(42, "x", 100), http.StatusForbidden, "forbidden")
	if status, b := e.call(restorer, "POST", "restore", rc, restoreReq(42, "x", 0, s1)); status != http.StatusOK {
		t.Errorf("restorer: %d %s", status, b)
	}
	if r := e.world.restores[len(e.world.restores)-1]; r.MayCreate {
		t.Errorf("restorer may create")
	}
	// No items.restore.
	gm := e.withRole("Gamem", "game_master")
	e.errorCode(gm, "POST", "restore", e.me(gm).GetCsrfToken(), ok, http.StatusForbidden, "forbidden")
	// A login older than 10 minutes needs a reauth.
	if _, err := e.admin.Exec(context.Background(), "UPDATE account_sessions SET authenticated_at = now() - interval '11 minutes'"); err != nil {
		t.Fatal(err)
	}
	e.errorCode(adm, "POST", "restore", csrf, ok, http.StatusForbidden, "reauth")
	if n := len(e.world.restores); n != 4 {
		t.Errorf("%d restores reached the world, want 4", n)
	}
}

// TestReviews checks the review queue and POST /audit/{id}/review: no
// self review, a note for a flag, one review per row, and an audit row
// for each review.
func TestReviews(t *testing.T) {
	e := setup(t, options{})
	ctx := context.Background()
	first := e.withRole("Admin1", "admin")
	second := e.withRole("Admin2", "admin")
	gm := e.withRole("Gamem", "game_master")
	id1 := e.accountID("Admin1")
	for _, r := range []store.AuditRow{
		{AccountID: id1, AccountName: "Admin1", Role: "admin", Action: "items.restore", Source: "admin", Target: "Hero",
			Reason: "theft", NeedsReview: true},
		{AccountID: id1, AccountName: "Admin1", Role: "admin", Action: "players.view", Source: "admin", Target: "Hero"},
		{AccountID: id1, AccountName: "Admin1", Role: "admin", Action: "items.restore", Source: "admin", Target: "Other",
			Reason: "scam", NeedsReview: true},
	} {
		if err := store.WriteStaffAudit(ctx, e.game, r); err != nil {
			t.Fatal(err)
		}
	}
	var ids []int64
	rows, _ := e.admin.Query(ctx, "SELECT id FROM staff_audit ORDER BY id")
	for rows.Next() {
		var id int64
		_ = rows.Scan(&id)
		ids = append(ids, id)
	}
	rows.Close()
	queue := func() []int64 {
		p := get(e, second, "audit?needs_review=true&review=pending", &adminv1.AuditPage{})
		var out []int64
		for _, en := range p.GetEntries() {
			out = append(out, en.GetId())
		}
		return out
	}
	if got := queue(); !slices.Equal(got, []int64{ids[2], ids[0]}) {
		t.Fatalf("queue %v, want %v", got, []int64{ids[2], ids[0]})
	}
	path := func(id int64) string { return "audit/" + strconv.FormatInt(id, 10) + "/review" }
	ok := adminv1.ReviewRequest_builder{Verdict: "ok"}.Build()
	csrf2 := e.me(second).GetCsrfToken()
	e.errorCode(second, "POST", path(ids[0]), "", ok, http.StatusForbidden, "csrf")
	e.errorCode(first, "POST", path(ids[0]), e.me(first).GetCsrfToken(), ok, http.StatusConflict, "conflict")
	e.errorCode(gm, "POST", path(ids[0]), e.me(gm).GetCsrfToken(), ok, http.StatusForbidden, "forbidden")
	e.errorCode(second, "POST", path(ids[1]), csrf2, ok, http.StatusConflict, "conflict")
	e.errorCode(second, "POST", path(999999), csrf2, ok, http.StatusNotFound, "not_found")
	e.errorCode(second, "POST", "audit/x/review", csrf2, ok, http.StatusBadRequest, "bad_request")
	e.errorCode(second, "POST", path(ids[0]), csrf2, adminv1.ReviewRequest_builder{Verdict: "flag"}.Build(), http.StatusBadRequest, "bad_request")
	e.errorCode(second, "POST", path(ids[0]), csrf2, adminv1.ReviewRequest_builder{Verdict: "maybe"}.Build(), http.StatusBadRequest, "bad_request")
	// A review needs no recent login.
	if _, err := e.admin.Exec(ctx, "UPDATE account_sessions SET authenticated_at = now() - interval '11 minutes'"); err != nil {
		t.Fatal(err)
	}
	status, b := e.call(second, "POST", path(ids[0]), csrf2, adminv1.ReviewRequest_builder{Verdict: "flag", Note: "no ticket"}.Build())
	en := &adminv1.AuditEntry{}
	_ = protojson.Unmarshal(b, en)
	if status != http.StatusOK || en.GetVerdict() != "flag" || en.GetReviewNote() != "no ticket" || en.GetReviewedBy() != "Admin2" ||
		en.GetAccountId() != id1 {
		t.Fatalf("review: %d %s", status, b)
	}
	e.errorCode(second, "POST", path(ids[0]), csrf2, ok, http.StatusConflict, "conflict")
	if got := queue(); !slices.Equal(got, []int64{ids[2]}) {
		t.Errorf("queue after review %v", got)
	}
	p := get(e, second, "audit?needs_review=false", &adminv1.AuditPage{})
	if len(p.GetEntries()) != 2 || p.GetEntries()[0].GetAction() != "audit.review" || p.GetEntries()[1].GetAction() != "players.view" {
		t.Errorf("needs_review=false: %v", p)
	}
	e.errorCode(second, "GET", "audit?needs_review=yes", "", nil, http.StatusBadRequest, "bad_request")
	if got := e.actions(); got[len(got)-1] != "audit.review "+strconv.FormatInt(ids[0], 10) {
		t.Errorf("review audit row %q", got)
	}
}

// TestIPSearchLookback: an IP search finds the logins of the last
// login_ip_days, below and above the default of 90 (decision 0049).
func TestIPSearchLookback(t *testing.T) {
	for _, c := range []struct {
		days int
		want string
	}{
		{0, "Days60"},
		{30, ""},
		{120, "Days60,Days100"},
	} {
		e := setup(t, options{loginIPDays: c.days})
		owner := e.staffer("Owner")
		// Days60 signs up first, so it has the lower account ID.
		for _, l := range []struct {
			name string
			days int
		}{{"Days60", 60}, {"Days100", 100}} {
			name, days := l.name, l.days
			e.signup(name)
			if _, err := e.admin.Exec(context.Background(), `INSERT INTO account_logins (account_id, at, ip)
				SELECT id, now() - make_interval(days => $2), '198.51.100.7' FROM accounts WHERE name = $1`, name, days); err != nil {
				t.Fatal(err)
			}
		}
		p := get(e, owner, "players?ip=198.51.100.0/24", &adminv1.PlayerSearch{})
		if got := accountNames(p); got != c.want {
			t.Errorf("login_ip_days %d: %q, want %q", c.days, got, c.want)
		}
		for _, a := range p.GetAccounts() {
			if len(a.GetLogins()) != 1 {
				t.Errorf("login_ip_days %d: %s has logins %v", c.days, a.GetAccount(), a.GetLogins())
			}
		}
	}
}
