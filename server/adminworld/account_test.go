package adminworld_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/lxsmnsyc/wathena/server/account"
	"github.com/lxsmnsyc/wathena/server/adminworld"
	"github.com/lxsmnsyc/wathena/server/rules"
	"github.com/lxsmnsyc/wathena/server/social"
)

// accountOf reads a character's account.
func (e *env) accountOf(id int64) int64 {
	e.t.Helper()
	var acc int64
	if err := e.admin.QueryRow(e.ctx, "SELECT account_id FROM characters WHERE id = $1", id).Scan(&acc); err != nil {
		e.t.Fatal(err)
	}
	return acc
}

func (e *env) exec(sql string, args ...any) {
	e.t.Helper()
	if _, err := e.admin.Exec(e.ctx, sql, args...); err != nil {
		e.t.Fatalf("%s: %v", sql, err)
	}
}

// TestAccountPageBlocks lists and changes the block lists of an online
// character, in its zone, and of an offline one, in the database.
func TestAccountPageBlocks(t *testing.T) {
	e := setup(t)
	page := e.aw.AccountPage()
	away := e.character("Away")
	here := e.character("Here")
	e.exec("INSERT INTO character_blocks (character_id, position, name) VALUES ($1, 0, 'Pest'), ($1, 1, 'Troll')", away)
	s := e.login(here)
	defer s.Leave()
	s.Block(1, "Spammer", false)
	var got []account.CharacterBlocks
	deadline := time.Now().Add(5 * time.Second)
	for {
		var err error
		if got, err = page.Blocks(e.ctx, e.accountOf(here)); err != nil {
			t.Fatal(err)
		}
		if len(got) == 1 && len(got[0].Names) == 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(got) != 1 || !got[0].Online || !slices.Equal(got[0].Names, []string{"Spammer"}) {
		t.Fatalf("online blocks %+v", got)
	}
	if ok, err := page.Unblock(e.ctx, e.accountOf(here), here, "spammer"); err != nil || !ok {
		t.Fatalf("online unblock: %v %v", ok, err)
	}
	if got, _ = page.Blocks(e.ctx, e.accountOf(here)); len(got[0].Names) != 0 {
		t.Errorf("after the online unblock %+v", got)
	}
	got, err := page.Blocks(e.ctx, e.accountOf(away))
	if err != nil || len(got) != 1 || got[0].Online || !slices.Equal(got[0].Names, []string{"Pest", "Troll"}) {
		t.Fatalf("offline blocks %+v %v", got, err)
	}
	// Another account's character is not this account's to change.
	if ok, err := page.Unblock(e.ctx, e.accountOf(here), away, "Pest"); err != nil || ok {
		t.Errorf("unblock of another account's character: %v %v", ok, err)
	}
	if ok, err := page.Unblock(e.ctx, e.accountOf(away), away, "PEST"); err != nil || !ok {
		t.Fatalf("offline unblock: %v %v", ok, err)
	}
	if got, _ = page.Blocks(e.ctx, e.accountOf(away)); !slices.Equal(got[0].Names, []string{"Troll"}) {
		t.Errorf("after the offline unblock %+v", got)
	}
}

// TestAccountPageReportsAndSanctions lists the reports a character
// filed, with only whether staff took action, and the sanctions on the
// account and its characters.
func TestAccountPageReportsAndSanctions(t *testing.T) {
	e := setup(t)
	page := e.aw.AccountPage()
	me := e.character("Reporter")
	acc := e.accountOf(me)
	e.exec(`INSERT INTO reports (created_at, reporter_id, reporter_name, target_id, target_name, map, reason, status, closed_at, outcome, note)
		VALUES (now(), $1, 'Reporter', 9, 'Ruffian', 'map/one', 'insults', 'closed', now(), 'warned', 'staff only note'),
		(now(), $1, 'Reporter', 9, 'Bot', 'map/one', 'botting', 'closed', now(), 'no_action', ''),
		(now(), $1, 'Reporter', 0, 'Rude Guild', 'map/one', 'emblem', 'open', NULL, NULL, '')`, me)
	e.exec(`UPDATE reports SET guild_id = 3, emblem_version = 1 WHERE target_name = 'Rude Guild'`)
	e.exec(`INSERT INTO sanctions (character_id, account_id, kind, reason, started_at, ends_at) VALUES ($1, $2, 'mute', 'flood', now() - interval '2 hours', now() - interval '1 hour')`, me, acc)
	e.exec(`INSERT INTO sanctions (account_id, kind, reason, started_at, ends_at) VALUES ($1, 'ban', 'cheat', now(), 'infinity')`, acc)
	reports, err := page.Reports(e.ctx, acc)
	if err != nil || len(reports) != 3 {
		t.Fatalf("reports %+v %v", reports, err)
	}
	if r := reports[0]; r.Kind != account.ReportGuildEmblem || r.Closed {
		t.Errorf("newest report %+v", r)
	}
	if r := reports[1]; !r.Closed || r.ActionTaken || r.Kind != account.ReportPlayer {
		t.Errorf("no action %+v", r)
	}
	if r := reports[2]; !r.Closed || !r.ActionTaken {
		t.Errorf("action taken %+v", r)
	}
	sanctions, err := page.Sanctions(e.ctx, acc)
	if err != nil || len(sanctions) != 2 || sanctions[0].Kind != "ban" || !sanctions[0].Permanent || sanctions[1].Character != "Reporter" ||
		sanctions[1].Active(time.Now()) {
		t.Errorf("sanctions %+v %v", sanctions, err)
	}
}

// TestAccountPageExport reads an offline character's data, and refuses
// while it plays.
func TestAccountPageExport(t *testing.T) {
	e := setup(t)
	page := e.aw.AccountPage()
	id := e.character("Exporter", stack(0, 501, 3))
	acc := e.accountOf(id)
	e.exec("INSERT INTO character_skills (character_id, skill_id, level) VALUES ($1, 2, 10)", id)
	e.exec("INSERT INTO character_blocks (character_id, position, name) VALUES ($1, 0, 'Pest')", id)
	e.exec("INSERT INTO storage_items (serial, holder, account_id, slot, item_id, amount) VALUES (uuidv7(), 'account', $1, 0, 909, 4)", acc)
	e.exec("INSERT INTO mails (id, sender_name, recipient_id, recipient_name, title, body, sent_at) VALUES (900, 'Friend', $1, 'Exporter', 'Hi', 'A gift', now())", id)
	e.exec(`INSERT INTO chat_log (id, at, kind, character_id, name, map, text) VALUES (uuidv7(), now(), 'local', $1, 'Exporter', 'map/one', 'hello all')`, id)
	e.exec(`INSERT INTO item_log (at, action, from_kind, from_id, to_kind, to_id, serial, item_id, amount) VALUES
		(now(), 'storage', 'inventory', $1, 'storage', $2, uuidv7(), 909, 4), (now(), 'pick_up', 'ground', 0, 'inventory', $1, uuidv7(), 501, 3)`, id, acc)
	e.exec(`INSERT INTO zeny_log (at, action, from_kind, from_id, to_kind, to_id, amount) VALUES (now(), 'sell', 'shop', 0, 'inventory', $1, 25)`, id)
	// Mods' variables of the character and the account (decision 0036).
	e.exec(`INSERT INTO mod_character_variables (character_id, mod_id, name, value) VALUES
		($1, 'example.lantern', 'lit', 'true'), ($1, 'example.bank', 'visits', '3')`, id)
	e.exec(`INSERT INTO mod_account_variables (account_id, mod_id, name, value) VALUES ($1, 'example.bank', 'balance', '{"zeny": 500}')`, acc)
	// The quest log (decision 0043), with a quest whose record is gone.
	e.exec(`INSERT INTO character_quests (character_id, quest_id, ord, state, ends_at, count1) VALUES
		($1, 1100, 0, 1, '2026-10-17 04:00:00+00', 2), ($1, 1000500, 1, 2, NULL, 0)`, id)
	ex, err := page.Export(e.ctx, acc)
	if err != nil {
		t.Fatal(err)
	}
	if len(ex.Characters) != 1 {
		t.Fatalf("characters %+v", ex.Characters)
	}
	c := ex.Characters[0]
	if c.Name != "Exporter" || c.Sex != "male" || len(c.Inventory) != 1 || c.Inventory[0].Name != "Red Potion" ||
		len(c.Skills) != 1 || c.Skills[0].Level != 10 || len(c.Mail) != 1 || c.Mail[0].Body != "A gift" ||
		!slices.Equal(c.BlockList, []string{"Pest"}) {
		t.Errorf("character %+v", c)
	}
	if len(c.Variables) != 2 || c.Variables[0].Mod != "example.bank" || c.Variables[0].Name != "visits" || string(c.Variables[0].Value) != "3" ||
		c.Variables[1].Mod != "example.lantern" || string(c.Variables[1].Value) != "true" {
		t.Errorf("character variables %+v", c.Variables)
	}
	if q := c.Quests; len(q) != 2 || q[0].ID != 1100 || q[0].Name != "1100" || q[0].Title != "Porings" || q[0].State != "active" ||
		q[0].EndsAt == nil || !q[0].EndsAt.Equal(time.Date(2026, 10, 17, 4, 0, 0, 0, time.UTC)) || !slices.Equal(q[0].Counts, []int{2}) ||
		q[1].ID != 1000500 || q[1].Name != "" || q[1].State != "complete" || q[1].EndsAt != nil || len(q[1].Counts) != 3 {
		t.Errorf("character quests %+v", c.Quests)
	}
	if len(ex.Variables) != 1 || ex.Variables[0].Name != "balance" || string(ex.Variables[0].Value) != `{"zeny": 500}` {
		t.Errorf("account variables %+v", ex.Variables)
	}
	if len(ex.Storages) != 1 || ex.Storages[0].Kind != "storage" || ex.Storages[0].Items[0].Amount != 4 {
		t.Errorf("storages %+v", ex.Storages)
	}
	if len(ex.Chat) != 1 || ex.Chat[0].Text != "hello all" || len(ex.ItemLog) != 2 || len(ex.ZenyLog) != 1 || len(ex.Truncated) != 0 {
		t.Errorf("logs %+v %+v %+v", ex.Chat, ex.ItemLog, ex.ZenyLog)
	}
	s := e.login(id)
	if _, err := page.Export(e.ctx, acc); !errors.Is(err, account.ErrCharacterOnline) {
		t.Errorf("export while online: %v", err)
	}
	s.Leave()
}

// inbox is a social.Inbox that drops what it gets.
type inbox struct{}

func (inbox) DeliverSocial(social.Notice) {}

// TestDeletionJob: the job waits while a character has a session, and
// disconnects it. Once the clock passes the request's time, it deletes
// the account, and the party it led breaks up in memory.
func TestDeletionJob(t *testing.T) {
	e := setup(t)
	leaver := e.character("Leaver")
	stayer := e.character("Stayer")
	acc, other := e.accountOf(leaver), e.accountOf(stayer)
	e.exec(`INSERT INTO account_deletions (account_id, requested_at, delete_after) VALUES ($1, now(), now() + interval '14 days')`, acc)
	reg := social.New(rules.DefaultLimits(), rules.DefaultParty())
	lp := reg.Online(social.Login{Name: "Leaver", CharID: leaver, AccountID: acc, Inbox: inbox{}})
	sp := reg.Online(social.Login{Name: "Stayer", CharID: stayer, AccountID: other, Inbox: inbox{}})
	party, _ := reg.Create(lp, "Pair", social.Options{})
	reg.Invite(lp, "Stayer")
	reg.Reply(sp, party, true)
	reg.Offline(lp, inbox{})
	if _, ok := reg.GuildLedWithOthers(leaver); ok {
		t.Fatal("no guild yet")
	}

	later := time.Now().Add(15 * 24 * time.Hour)
	cfg := e.cfg
	cfg.Social = reg
	cfg.Clock = func() time.Time { return later }
	aw := adminworld.New(cfg)

	// Not due on the real clock.
	if n, err := e.aw.DeleteDue(e.ctx); err != nil || n != 0 {
		t.Fatalf("early run: %d %v", n, err)
	}
	// Due 15 days later, but the character plays: it is disconnected and
	// the account waits.
	s := e.login(leaver)
	if n, err := aw.DeleteDue(e.ctx); err != nil || n != 0 {
		t.Fatalf("run while online: %d %v", n, err)
	}
	kicks := e.kicker.got()
	if len(kicks) != 1 || !strings.HasPrefix(kicks[0], "DISCONNECT_REASON_ACCOUNT_DELETION This account is set to be deleted") {
		t.Errorf("kicks %v", kicks)
	}
	s.Leave()
	deadline := time.Now().Add(5 * time.Second)
	for e.st.Live(leaver) && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n, err := aw.DeleteDue(e.ctx); err != nil || n != 1 {
		t.Fatalf("run after the logout: %d %v", n, err)
	}
	var left int
	if err := e.admin.QueryRow(e.ctx, "SELECT count(*) FROM accounts WHERE id = $1", acc).Scan(&left); err != nil || left != 0 {
		t.Errorf("the account is still there: %d %v", left, err)
	}
	if err := e.admin.QueryRow(e.ctx, "SELECT count(*) FROM characters WHERE id = $1", stayer).Scan(&left); err != nil || left != 1 {
		t.Errorf("the other character: %d %v", left, err)
	}
	if info, ok := reg.Party(party); ok && !info.Ended {
		t.Errorf("the party still runs: %+v", info)
	}
	if _, _, err := e.st.Load(e.ctx, leaver); err == nil {
		t.Error("the deleted character still loads")
	}
}
