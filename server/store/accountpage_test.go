package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lxsmnsyc/wathena/server/store"
	"github.com/lxsmnsyc/wathena/server/store/storetest"
)

// seedAccount makes an account with characters, as the admin role.
func seedAccount(t *testing.T, ctx context.Context, admin *pgxpool.Pool, name string, chars ...string) (int64, []int64) {
	t.Helper()
	var acc int64
	if err := admin.QueryRow(ctx, "INSERT INTO accounts (name) VALUES ($1) RETURNING id", name).Scan(&acc); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for i, c := range chars {
		var id int64
		err := admin.QueryRow(ctx, `INSERT INTO characters (account_id, slot, name, job, sex, base_level, job_level,
			str, agi, vit, int, dex, luk, hp, sp, map, x, y)
			VALUES ($1, $2, $3, 0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 40, 11, 'map/one', 5, 5) RETURNING id`, acc, i, c).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return acc, ids
}

// TestDeleteAccountEveryTable fills every table that names an account or
// a character, for an account set to be deleted and for another one,
// then runs the deletion as the game role. The deleted account's
// personal data and characters go. The logs, reports, mails and groups
// of other players stay, with the character IDs and without the account.
func TestDeleteAccountEveryTable(t *testing.T) {
	admin, game := migrated(t)
	s := newStore(t, game, store.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	a, la := seedAccount(t, ctx, admin, "Leaver1", "Leaver One", "Leaver Two")
	b, sb := seedAccount(t, ctx, admin, "Stayer1", "Stayer One")
	l1, l2, s1 := la[0], la[1], sb[0]
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	// The account service's tables.
	for _, acc := range []int64{a, b} {
		exec(`INSERT INTO account_passwords (account_id, hash) VALUES ($1, '$argon2id$x')`, acc)
		exec(`INSERT INTO account_sessions (token_hash, account_id, expires_at, ip) VALUES (sha256($1::bigint::text::bytea), $1::bigint, now() + interval '1 day', '10.0.0.1')`, acc)
		exec(`INSERT INTO account_logins (account_id, ip) VALUES ($1, '10.0.0.1')`, acc)
	}
	exec(`INSERT INTO game_tickets (token_hash, session_id, account_id, character_id, expires_at)
		SELECT sha256('ticket'), id, account_id, $2, now() + interval '1 minute' FROM account_sessions WHERE account_id = $1`, a, l1)
	exec(`INSERT INTO account_totp (account_id, secret, enabled_at) VALUES ($1, decode(repeat('ab', 40), 'hex'), now())`, a)
	exec(`INSERT INTO account_recovery_codes (account_id, code_hash) VALUES ($1, sha256('code'))`, a)
	exec(`INSERT INTO account_pending_logins (token_hash, account_id, factor, expires_at) VALUES (sha256('pending'), $1, 'password', now() + interval '5 minutes')`, a)
	exec(`INSERT INTO account_webauthn_users (account_id, user_handle) VALUES ($1, sha256('handle'))`, a)
	exec(`INSERT INTO account_passkeys (account_id, rp_id, credential_id, public_key, flags, name)
		VALUES ($1, 'example.com', decode(repeat('cd', 16), 'hex'), decode('a1b2', 'hex'), 5, 'Phone')`, a)
	exec(`INSERT INTO account_webauthn_ceremonies (token_hash, kind, session_id, data, expires_at)
		SELECT sha256('ceremony'), 'register', id, '{}', now() + interval '5 minutes' FROM account_sessions WHERE account_id = $1`, a)
	exec(`INSERT INTO account_exports (account_id, exported_at) VALUES ($1, now())`, a)
	exec(`INSERT INTO account_deletions (account_id, requested_at, delete_after) VALUES ($1, now() - interval '15 days', now() - interval '1 day')`, a)
	exec(`INSERT INTO account_deletions (account_id, requested_at, delete_after) VALUES ($1, now(), now() + interval '14 days')`, b)
	exec(`INSERT INTO staff_roles (account_id, role, given_by) VALUES ($1, 'moderator', $2), ($2, 'admin', $1)`, a, b)
	exec(`INSERT INTO staff_audit (account_id, account_name, role, action, source, target, target_account_id)
		VALUES ($2, 'Stayer1', 'admin', 'moderation.ban', 'admin', 'Leaver1', $1), ($1, 'Leaver1', 'moderator', 'moderation.mute', 'game', 'Stayer One', $2)`, a, b)
	exec(`INSERT INTO event_overrides (event, enabled, changed_by, changed_by_id) VALUES ('core:Treasure', false, 'Leaver1', $1)`, a)

	// The game's tables.
	exec(`INSERT INTO mod_account_variables (account_id, mod_id, name, value) VALUES ($1, 'm', 'v', '1'), ($2, 'm', 'v', '1')`, a, b)
	for _, c := range []int64{l1, s1} {
		exec(`INSERT INTO mod_character_variables (character_id, mod_id, name, value) VALUES ($1, 'm', 'v', '1')`, c)
		exec(`INSERT INTO character_statuses (character_id, name, val1, val2, val3, val4, tick) VALUES ($1, 'SC_BLESSING', 1, 0, 0, 0, -1)`, c)
		exec(`INSERT INTO character_timed_bonuses (character_id, position, key, remaining_ms, bonuses) VALUES ($1, 0, 'k', 1000, '{}')`, c)
		exec(`INSERT INTO character_skills (character_id, skill_id, level) VALUES ($1, 1, 9)`, c)
		exec(`INSERT INTO inventory (character_id, slot, item_id, amount) VALUES ($1, 0, 501, 5)`, c)
		exec(`INSERT INTO character_currencies (character_id, currency, amount) VALUES ($1, 1, 10)`, c)
		exec(`INSERT INTO character_maps (character_id, map_id) VALUES ($1, 'gen:town:t1')`, c)
		exec(`INSERT INTO character_memos (character_id, slot, map_id, x, y) VALUES ($1, 0, 'gen:town:t1', 5, 6)`, c)
		exec(`INSERT INTO character_quests (character_id, quest_id, ord, state, count1) VALUES ($1, 1100, 0, 1, 3)`, c)
	}
	exec(`INSERT INTO character_blocks (character_id, position, name) VALUES ($1, 0, 'Stayer One'), ($2, 0, 'Leaver One')`, l1, s1)
	exec(`INSERT INTO character_friends (character_id, friend_id) VALUES ($1, $2), ($2, $1)`, l1, s1)
	exec(`INSERT INTO account_currencies (account_id, currency, amount) VALUES ($1, 2, 10), ($2, 2, 10)`, a, b)
	exec(`INSERT INTO storage_items (serial, holder, account_id, slot, item_id, amount) VALUES (uuidv7(), 'account', $1, 0, 909, 3), (uuidv7(), 'account', $2, 0, 909, 3)`, a, b)
	exec(`INSERT INTO storage_items (serial, holder, character_id, slot, item_id, amount) VALUES (uuidv7(), 'cart', $1, 0, 909, 3), (uuidv7(), 'character', $1, 0, 909, 3)`, l1)
	exec(`INSERT INTO storage_sizes (holder, account_id, slots) VALUES ('account', $1, 700)`, a)
	exec(`INSERT INTO storage_sizes (holder, character_id, slots) VALUES ('cart', $1, 120)`, l1)
	exec(`INSERT INTO stores (character_id, buying, title, map, x, y, autotrade) VALUES ($1, false, 'Cheap', 'map/one', 5, 5, true)`, l1)
	exec(`INSERT INTO store_items (character_id, buying, idx, item_id, cart_serial, amount, price) VALUES ($1, false, 0, 909, uuidv7(), 1, 10)`, l1)
	// Mail to the leaver and mail from it.
	exec(`INSERT INTO mails (id, sender_id, sender_name, recipient_id, recipient_name, title, sent_at) VALUES
		(1, $2, 'Stayer One', $1, 'Leaver One', 'Hi', now()), (2, $1, 'Leaver One', $2, 'Stayer One', 'Bye', now())`, l1, s1)
	exec(`INSERT INTO mail_items (serial, mail_id, slot, item_id, amount) VALUES (uuidv7(), 1, 0, 909, 1), (uuidv7(), 2, 0, 909, 1)`)
	// Sanctions on both accounts, and a ban of the leaver.
	exec(`INSERT INTO sanctions (character_id, account_id, kind, reason, started_at, ends_at) VALUES
		($1, $2, 'mute', 'spam', now(), now() + interval '1 hour'), ($3, $4, 'mute', 'spam', now(), now() + interval '1 hour')`, l1, a, s1, b)
	exec(`INSERT INTO sanctions (account_id, kind, reason, started_at, ends_at) VALUES ($1, 'ban', 'cheat', now(), 'infinity')`, a)
	// Chat lines of both, a whisper to the leaver, and reports each way.
	exec(`INSERT INTO chat_log (id, at, kind, character_id, name, map, text) VALUES
		('00000000-0000-7000-8000-000000000001', now(), 'local', $1, 'Leaver One', 'map/one', 'mine'),
		('00000000-0000-7000-8000-000000000002', now(), 'local', $2, 'Stayer One', 'map/one', 'theirs')`, l1, s1)
	exec(`INSERT INTO chat_log (id, at, kind, character_id, name, target, map, text) VALUES
		('00000000-0000-7000-8000-000000000003', now(), 'whisper', $1, 'Stayer One', 'Leaver One', 'map/one', 'to the leaver')`, s1)
	exec(`INSERT INTO reports (created_at, reporter_id, reporter_name, target_id, target_name, line_id, map, status, closed_at, outcome, sanction_id)
		SELECT now(), $2, 'Stayer One', $1, 'Leaver One', '00000000-0000-7000-8000-000000000001', 'map/one', 'closed', now(), 'sanctioned', id
		FROM sanctions WHERE character_id = $1`, l1, s1)
	exec(`INSERT INTO reports (created_at, reporter_id, reporter_name, target_id, target_name, map) VALUES (now(), $1, 'Leaver One', $2, 'Stayer One', 'map/one')`, l1, s1)
	exec(`INSERT INTO report_lines (report_id, line_id) SELECT id, '00000000-0000-7000-8000-000000000001' FROM reports WHERE reporter_id = $1`, s1)
	exec(`INSERT INTO report_lines (report_id, line_id) SELECT id, '00000000-0000-7000-8000-000000000002' FROM reports WHERE reporter_id = $1`, l1)
	// A party the leaver leads with the stayer, one it leads alone, a
	// guild it leads alone, and the stayer's guild with a leaver member.
	exec(`INSERT INTO parties (id, name, leader_id) VALUES (1, 'Shared', $1), (2, 'Alone', $2)`, l1, l2)
	exec(`INSERT INTO party_members (party_id, character_id, slot) VALUES (1, $1, 0), (1, $2, 1), (2, $3, 0)`, l1, s1, l2)
	exec(`INSERT INTO guilds (id, name, positions) VALUES (1, 'Solo', '[{},{}]'), (2, 'Others', '[{},{}]')`)
	exec(`INSERT INTO guild_members (guild_id, character_id, slot) VALUES (1, $1, 0), (2, $2, 0), (2, $3, 1)`, l1, s1, l2)
	exec(`INSERT INTO guild_storage (serial, guild_id, slot, item_id, amount) VALUES (uuidv7(), 1, 0, 909, 2)`)
	exec(`INSERT INTO guild_emblems (guild_id, version, png, uploaded_by) VALUES (2, 1, 'x', $1)`, a)
	// Item and zeny logs: the leaver's inventory, the account's storage
	// and its extra storage 5, and the stayer's inventory.
	exec(`INSERT INTO item_log (at, action, from_kind, from_id, to_kind, to_id, serial, item_id, amount, to_storage) VALUES
		(now(), 'storage', 'inventory', $1, 'storage', $2, uuidv7(), 909, 1, 0),
		(now(), 'storage', 'inventory', $1, 'storage', $2, uuidv7(), 909, 1, 5),
		(now(), 'trade', 'inventory', $1, 'inventory', $3, uuidv7(), 909, 1, 0),
		(now(), 'storage', 'inventory', $3, 'storage', $4, uuidv7(), 909, 1, 0)`, l1, a, s1, b)
	exec(`INSERT INTO zeny_log (at, action, from_kind, from_id, to_kind, to_id, amount, currency) VALUES
		(now(), 'currency', 'script', 0, 'storage', $1, 5, 2), (now(), 'trade', 'inventory', $2, 'inventory', $3, 50, 0)`, a, l1, s1)

	count := func(sql string, args ...any) int {
		t.Helper()
		var n int
		if err := admin.QueryRow(ctx, sql, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
		return n
	}

	// Not due yet at a time before delete_after, and never for the
	// stayer, whose request waits.
	if ok, err := s.DeleteAccount(ctx, a, time.Now().Add(-2*24*time.Hour), []int{5}); err != nil || ok {
		t.Fatalf("early deletion: %v %v", ok, err)
	}
	if ok, err := s.DeleteAccount(ctx, b, time.Now(), nil); err != nil || ok {
		t.Fatalf("deletion of an account whose time has not come: %v %v", ok, err)
	}
	// The database clock decides, not the caller's: a time 15 days ahead
	// neither lists nor deletes the stayer (review decision 51).
	ahead := time.Now().Add(15 * 24 * time.Hour)
	if ok, err := s.DeleteAccount(ctx, b, ahead, nil); err != nil || ok {
		t.Fatalf("deletion with the caller's clock ahead: %v %v", ok, err)
	}
	if due, err := s.DueDeletions(ctx, ahead, 10); err != nil || len(due) != 1 || due[0] != a {
		t.Fatalf("due with the caller's clock ahead: %v %v", due, err)
	}
	if n := count("SELECT count(*) FROM characters"); n != 3 {
		t.Fatalf("characters after refused deletions: %d", n)
	}
	due, err := s.DueDeletions(ctx, time.Now(), 10)
	if err != nil || len(due) != 1 || due[0] != a {
		t.Fatalf("due %v %v", due, err)
	}
	if ok, err := s.DeleteAccount(ctx, a, time.Now(), []int{5}); err != nil || !ok {
		t.Fatalf("deletion: %v %v", ok, err)
	}

	// Gone: everything of the account and its characters.
	for _, q := range []string{
		"SELECT count(*) FROM accounts WHERE id = $1",
		"SELECT count(*) FROM account_passwords WHERE account_id = $1",
		"SELECT count(*) FROM account_sessions WHERE account_id = $1",
		"SELECT count(*) FROM game_tickets WHERE account_id = $1",
		"SELECT count(*) FROM account_totp WHERE account_id = $1",
		"SELECT count(*) FROM account_recovery_codes WHERE account_id = $1",
		"SELECT count(*) FROM account_pending_logins WHERE account_id = $1",
		"SELECT count(*) FROM account_webauthn_users WHERE account_id = $1",
		"SELECT count(*) FROM account_passkeys WHERE account_id = $1",
		"SELECT count(*) FROM account_logins WHERE account_id = $1",
		"SELECT count(*) FROM account_exports WHERE account_id = $1",
		"SELECT count(*) FROM account_deletions WHERE account_id = $1",
		"SELECT count(*) FROM staff_roles WHERE account_id = $1",
		"SELECT count(*) FROM mod_account_variables WHERE account_id = $1",
		"SELECT count(*) FROM account_currencies WHERE account_id = $1",
		"SELECT count(*) FROM storage_items WHERE account_id = $1",
		"SELECT count(*) FROM storage_sizes WHERE account_id = $1",
		"SELECT count(*) FROM sanctions WHERE account_id = $1",
		"SELECT count(*) FROM characters WHERE account_id = $1",
		"SELECT count(*) FROM item_log WHERE (from_kind = 'storage' AND from_id = $1) OR (to_kind = 'storage' AND to_id = $1)",
		"SELECT count(*) FROM zeny_log WHERE (from_kind = 'storage' AND from_id = $1) OR (to_kind = 'storage' AND to_id = $1)",
		"SELECT count(*) FROM guild_emblems WHERE uploaded_by = $1",
	} {
		if n := count(q, a); n != 0 {
			t.Errorf("%s: %d rows left", q, n)
		}
	}
	if n := count("SELECT count(*) FROM account_webauthn_ceremonies"); n != 0 {
		t.Errorf("passkey ceremonies left: %d", n)
	}
	for _, q := range []string{
		"SELECT count(*) FROM inventory WHERE character_id = ANY($1)",
		"SELECT count(*) FROM mod_character_variables WHERE character_id = ANY($1)",
		"SELECT count(*) FROM character_statuses WHERE character_id = ANY($1)",
		"SELECT count(*) FROM character_timed_bonuses WHERE character_id = ANY($1)",
		"SELECT count(*) FROM character_skills WHERE character_id = ANY($1)",
		"SELECT count(*) FROM character_currencies WHERE character_id = ANY($1)",
		"SELECT count(*) FROM character_maps WHERE character_id = ANY($1)",
		"SELECT count(*) FROM character_memos WHERE character_id = ANY($1)",
		"SELECT count(*) FROM character_quests WHERE character_id = ANY($1)",
		"SELECT count(*) FROM character_blocks WHERE character_id = ANY($1)",
		"SELECT count(*) FROM character_friends WHERE character_id = ANY($1) OR friend_id = ANY($1)",
		"SELECT count(*) FROM storage_items WHERE character_id = ANY($1)",
		"SELECT count(*) FROM storage_sizes WHERE character_id = ANY($1)",
		"SELECT count(*) FROM stores WHERE character_id = ANY($1)",
		"SELECT count(*) FROM store_items WHERE character_id = ANY($1)",
		"SELECT count(*) FROM mails WHERE recipient_id = ANY($1) OR sender_id = ANY($1)",
		"SELECT count(*) FROM party_members WHERE character_id = ANY($1)",
		"SELECT count(*) FROM guild_members WHERE character_id = ANY($1)",
		"SELECT count(*) FROM chat_log WHERE character_id = ANY($1)",
	} {
		if n := count(q, la); n != 0 {
			t.Errorf("%s: %d rows left", q, n)
		}
	}
	if n := count("SELECT count(*) FROM mail_items WHERE mail_id = 1"); n != 0 {
		t.Errorf("the items of the mail to the leaver: %d", n)
	}
	if n := count("SELECT count(*) FROM parties WHERE id = 2") + count("SELECT count(*) FROM guilds WHERE id = 1") +
		count("SELECT count(*) FROM guild_storage WHERE guild_id = 1"); n != 0 {
		t.Errorf("the leaver's own party and guild: %d rows left", n)
	}

	// Kept: the other account, and what other players need.
	for q, want := range map[string]int{
		"SELECT count(*) FROM accounts WHERE id = $1":                                      1,
		"SELECT count(*) FROM account_passwords WHERE account_id = $1":                     1,
		"SELECT count(*) FROM account_sessions WHERE account_id = $1":                      1,
		"SELECT count(*) FROM account_logins WHERE account_id = $1":                        1,
		"SELECT count(*) FROM account_deletions WHERE account_id = $1":                     1,
		"SELECT count(*) FROM staff_roles WHERE account_id = $1 AND given_by IS NULL":      1,
		"SELECT count(*) FROM mod_account_variables WHERE account_id = $1":                 1,
		"SELECT count(*) FROM account_currencies WHERE account_id = $1":                    1,
		"SELECT count(*) FROM storage_items WHERE account_id = $1":                         1,
		"SELECT count(*) FROM sanctions WHERE account_id = $1":                             1,
		"SELECT count(*) FROM staff_audit WHERE target_account_id = $1 OR account_id = $1": 2,
		"SELECT count(*) FROM item_log WHERE to_kind = 'storage' AND to_id = $1":           1,
	} {
		if n := count(q, b); n != want {
			t.Errorf("%s: %d rows, want %d", q, n, want)
		}
	}
	for q, want := range map[string]int{
		"SELECT count(*) FROM character_blocks WHERE character_id = $1 AND name = 'Leaver One'":                              1,
		"SELECT count(*) FROM inventory WHERE character_id = $1":                                                             1,
		"SELECT count(*) FROM character_quests WHERE character_id = $1":                                                      1,
		"SELECT count(*) FROM party_members WHERE character_id = $1 AND party_id = 1":                                        1,
		"SELECT count(*) FROM guild_members WHERE character_id = $1 AND guild_id = 2":                                        1,
		"SELECT count(*) FROM mails WHERE id = 2 AND recipient_id = $1 AND sender_id IS NULL AND sender_name = 'Leaver One'": 1,
		"SELECT count(*) FROM chat_log WHERE character_id = $1":                                                              2,
		"SELECT count(*) FROM reports WHERE reporter_id = $1 AND sanction_id IS NULL":                                        1,
	} {
		if n := count(q, s1); n != want {
			t.Errorf("%s: %d rows, want %d", q, n, want)
		}
	}
	if n := count("SELECT count(*) FROM mail_items WHERE mail_id = 2"); n != 1 {
		t.Errorf("the items of the mail from the leaver: %d", n)
	}
	// The reports keep both character IDs. The report of the leaver's line
	// lost the line, and the leaver's report keeps the stayer's line.
	if n := count("SELECT count(*) FROM reports WHERE target_id = $1 OR reporter_id = $1", l1); n != 2 {
		t.Errorf("reports naming the leaver: %d", n)
	}
	if n := count("SELECT count(*) FROM report_lines"); n != 1 {
		t.Errorf("report lines: %d", n)
	}
	// The staff audit keeps its target IDs, and the actor's.
	if n := count("SELECT count(*) FROM staff_audit WHERE target_account_id = $1 OR account_id = $1", a); n != 2 {
		t.Errorf("audit rows naming the leaver: %d", n)
	}
	if n := count("SELECT count(*) FROM event_overrides WHERE changed_by_id = $1", a); n != 1 {
		t.Errorf("event override: %d", n)
	}
	// The logs keep the character IDs, and name storage 0 for the
	// account's storages.
	if n := count("SELECT count(*) FROM item_log WHERE from_kind = 'inventory' AND from_id = $1", l1); n != 3 {
		t.Errorf("item log lines of the leaver's inventory: %d", n)
	}
	if n := count("SELECT count(*) FROM item_log WHERE to_kind = 'storage' AND to_id = 0"); n != 2 {
		t.Errorf("item log lines of the account's storages: %d", n)
	}
	if n := count("SELECT count(*) FROM zeny_log WHERE (to_kind = 'storage' AND to_id = 0) OR from_id = $1", l1); n != 2 {
		t.Errorf("zeny log lines: %d", n)
	}
	if n := count("SELECT count(*) FROM guild_emblems WHERE guild_id = 2 AND uploaded_by = 0"); n != 1 {
		t.Errorf("emblem: %d", n)
	}
	// The party the leaver led with the stayer stays: the server moved
	// it in memory first, which the registry's tests cover.
	if n := count("SELECT count(*) FROM parties WHERE id = 1") + count("SELECT count(*) FROM guilds WHERE id = 2"); n != 2 {
		t.Errorf("the stayer's party and guild: %d", n)
	}
	// A second run finds nothing to do.
	if ok, err := s.DeleteAccount(ctx, a, time.Now(), nil); err != nil || ok {
		t.Errorf("second deletion: %v %v", ok, err)
	}
}

// TestDeletionOfCharacter reads a character's deletion request, which
// refuses its game logins.
func TestDeletionOfCharacter(t *testing.T) {
	admin, game := migrated(t)
	s := newStore(t, game, store.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	a, ids := seedAccount(t, ctx, admin, "Leaver1", "Leaver One")
	if _, ok, err := s.DeletionOf(ctx, ids[0]); err != nil || ok {
		t.Fatalf("before a request: %v %v", ok, err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO account_deletions (account_id, requested_at, delete_after) VALUES ($1, now(), now() + interval '14 days')`, a); err != nil {
		t.Fatal(err)
	}
	d, ok, err := s.DeletionOf(ctx, ids[0])
	if err != nil || !ok || d.After.Sub(d.Requested) != 14*24*time.Hour {
		t.Errorf("request %+v %v %v", d, ok, err)
	}
}

// TestUnblockOffline takes a name off an offline character's block
// list, and leaves a character with a session alone.
func TestUnblockOffline(t *testing.T) {
	admin, game := migrated(t)
	s := newStore(t, game, store.Options{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, ids := seedAccount(t, ctx, admin, "Blocker1", "Blocker One")
	if _, err := admin.Exec(ctx, `INSERT INTO character_blocks (character_id, position, name) VALUES ($1, 0, 'Pest'), ($1, 1, 'Other')`, ids[0]); err != nil {
		t.Fatal(err)
	}
	if removed, live, err := s.UnblockOffline(ctx, ids[0], "PEST"); err != nil || !removed || live {
		t.Fatalf("unblock: %v %v %v", removed, live, err)
	}
	blocks, err := s.BlocksOf(ctx, ids)
	if err != nil || len(blocks[ids[0]]) != 1 || blocks[ids[0]][0] != "Other" {
		t.Errorf("blocks %v %v", blocks, err)
	}
	// A character with a session keeps its list in its zone.
	c, x, err := s.Load(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if removed, live, err := s.UnblockOffline(ctx, ids[0], "Other"); err != nil || removed || !live {
		t.Errorf("unblock of a live character: %v %v %v", removed, live, err)
	}
	x.Save(c, true)
}

// TestAccountPageRights: only the game role runs delete_account, and it
// cannot write deletion requests or delete accounts itself. The account
// role writes requests and exports, and cannot run the deletion.
func TestAccountPageRights(t *testing.T) {
	admin, _ := migrated(t)
	ctx := timeout(t)
	acc, _ := seedAccount(t, ctx, admin, "Rights1", "Rights One")
	for _, c := range []struct {
		role, sql string
		ok        bool
	}{
		{"wathena_account", "INSERT INTO account_deletions (account_id, requested_at, delete_after) VALUES ($1, now(), now() + interval '14 days')", false},
		{"wathena_account", "SELECT request_account_deletion($1, interval '13 days')", false},
		{"wathena_account", "SELECT request_account_deletion($1, interval '14 days')", true},
		{"wathena_account", "DELETE FROM account_deletions WHERE account_id = $1", true},
		{"wathena_account", "INSERT INTO account_exports (account_id, exported_at) VALUES ($1, now())", true},
		{"wathena_account", "SELECT delete_account($1, now(), '{}')", false},
		{"wathena_account", "DELETE FROM accounts WHERE id = $1", false},
		{"wathena_game", "SELECT count(*) FROM account_deletions WHERE account_id = $1", true},
		{"wathena_game", "SELECT delete_account($1, now(), '{}')", true},
		{"wathena_game", "INSERT INTO account_deletions (account_id, requested_at, delete_after) VALUES ($1, now(), now())", false},
		{"wathena_game", "SELECT request_account_deletion($1, interval '14 days')", false},
		{"wathena_game", "DELETE FROM accounts WHERE id = $1", false},
		{"wathena_game", "DELETE FROM characters WHERE account_id = $1", false},
		{"wathena_game", "SELECT count(*) FROM account_exports WHERE account_id = $1", false},
	} {
		tx, err := admin.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, "SET LOCAL ROLE "+c.role); err != nil {
			t.Fatal(err)
		}
		_, err = tx.Exec(ctx, c.sql, acc)
		if (err == nil) != c.ok {
			t.Errorf("%s: %s: %v", c.role, c.sql, err)
		}
		_ = tx.Rollback(context.Background())
	}
}

// TestDeletionWaitInDatabase: the account role cannot write a request
// whose wait is short or already over, and delete_account refuses one
// that someone else wrote, so a compromised account role cannot make
// the game role's job delete accounts at once (review decision 51).
func TestDeletionWaitInDatabase(t *testing.T) {
	admin, game := migrated(t)
	s := newStore(t, game, store.Options{})
	ctx := timeout(t)
	account := storetest.As(t, admin, "wathena_account")
	a, _ := seedAccount(t, ctx, admin, "Short1", "Short One")
	b, _ := seedAccount(t, ctx, admin, "Asked1", "Asked One")
	// The function sets both times from the database clock.
	var ok bool
	if err := account.QueryRow(ctx, "SELECT request_account_deletion($1, interval '14 days')", b).Scan(&ok); err != nil || !ok {
		t.Fatalf("request: %v %v", ok, err)
	}
	if err := account.QueryRow(ctx, "SELECT request_account_deletion($1, interval '14 days')", b).Scan(&ok); err != nil || ok {
		t.Errorf("second request: %v %v", ok, err)
	}
	var wait int64
	var fresh bool
	if err := admin.QueryRow(ctx, `SELECT extract(epoch FROM delete_after - requested_at)::bigint, requested_at > now() - interval '1 minute'
		FROM account_deletions WHERE account_id = $1`, b).Scan(&wait, &fresh); err != nil || wait != 14*24*3600 || !fresh {
		t.Errorf("request times: %v %v %v", wait, fresh, err)
	}
	// A row with a wait shorter than 14 days, such as one the account
	// role could write before, is never due.
	if _, err := admin.Exec(ctx, `INSERT INTO account_deletions (account_id, requested_at, delete_after)
		VALUES ($1, now() - interval '1 day', now() - interval '1 minute')`, a); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.DeleteAccount(ctx, a, time.Now(), nil); err != nil || ok {
		t.Errorf("deletion of a short request: %v %v", ok, err)
	}
	if ok, err := s.DeleteAccount(ctx, b, time.Now().Add(15*24*time.Hour), nil); err != nil || ok {
		t.Errorf("deletion with the caller's clock ahead: %v %v", ok, err)
	}
	var left int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM accounts WHERE id IN ($1, $2)", a, b).Scan(&left); err != nil || left != 2 {
		t.Errorf("accounts left: %d %v", left, err)
	}
}
